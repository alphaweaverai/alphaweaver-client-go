package main

import (
	"compress/zlib"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// UploadEvent represents a file upload completion event
type UploadEvent struct {
	EventType string    // "opt_upload" or "daily_summary_upload"
	FileName  string    // Uploaded file name
	JobID     string    // Associated job ID
	Timestamp time.Time // Upload completion time
}

// Global channel for upload events (buffered to prevent blocking)
var UploadEventChan = make(chan UploadEvent, 100)

// CSVUploadManager handles automatic CSV file uploads
type CSVUploadManager struct {
	config    *Config
	api       *APIClient
	fileMgr   *FileManager
	isRunning bool
	stopCh    chan bool
	mutex     sync.Mutex
	logf      func(string)
}

func NewCSVUploadManager(cfg *Config, api *APIClient) *CSVUploadManager {
	return &CSVUploadManager{
		config:  cfg,
		api:     api,
		fileMgr: NewFileManager(cfg),
		stopCh:  make(chan bool),
		logf:    func(string) {},
	}
}

func (cum *CSVUploadManager) SetLogger(fn func(string)) {
	if fn != nil {
		cum.logf = fn
	}
}

// Start begins monitoring the CSV upload folder
func (cum *CSVUploadManager) Start() error {
	cum.mutex.Lock()
	defer cum.mutex.Unlock()

	if cum.isRunning {
		return fmt.Errorf("CSV upload manager is already running")
	}

	cum.isRunning = true
	cum.stopCh = make(chan bool)
	cum.logf("CSV monitoring started")

	go cum.monitorFolder()
	return nil
}

// Stop stops monitoring the CSV upload folder
func (cum *CSVUploadManager) Stop() {
	cum.mutex.Lock()
	defer cum.mutex.Unlock()

	if !cum.isRunning {
		return
	}

	cum.isRunning = false
	close(cum.stopCh)
	cum.logf("CSV monitoring stopped")
}

// IsRunning returns whether the upload manager is currently running
func (cum *CSVUploadManager) IsRunning() bool {
	cum.mutex.Lock()
	defer cum.mutex.Unlock()
	return cum.isRunning
}

// monitorFolder continuously monitors the Results/To Do folder for CSV files
func (cum *CSVUploadManager) monitorFolder() {
	ticker := time.NewTicker(10 * time.Second) // Check every 10 seconds
	defer ticker.Stop()

	for {
		select {
		case <-cum.stopCh:
			return
		case <-ticker.C:
			cum.processCSVFiles()
		}
	}
}

// processCSVFiles processes all CSV files in the To Do folder
func (cum *CSVUploadManager) processCSVFiles() {
	files, err := cum.fileMgr.GetCSVFiles()
	if err != nil {
		cum.logf(fmt.Sprintf("Error getting CSV files: %v", err))
		return
	}

	for _, fileName := range files {
		if err := cum.uploadCSVFile(fileName); err != nil {
			cum.logf(fmt.Sprintf("CSV upload failed for %s: %v", fileName, err))
			// Move to error folder or leave in place for retry
			continue
		}

		// Move successful upload to Done folder
		if err := cum.fileMgr.MoveCSVFile(fileName); err != nil {
			cum.logf(fmt.Sprintf("Error moving CSV file %s to Done: %v", fileName, err))
		} else {
			cum.logf(fmt.Sprintf("CSV file moved to Done: %s", fileName))
		}
	}
}

// uploadCSVFile uploads a single CSV file
func (cum *CSVUploadManager) uploadCSVFile(fileName string) error {
	// Extract symbol and timeframe from filename
	// Expected format: symbol_timeframe_*.csv or similar
	symbol, timeframe, err := cum.extractSymbolAndTimeframe(fileName)
	if err != nil {
		return fmt.Errorf("failed to extract symbol and timeframe from filename %s: %w", fileName, err)
	}

	filePath := filepath.Join(cum.config.Folders.Files.Results.ToDo, fileName)
	cum.logf(fmt.Sprintf("Uploading CSV %s (Symbol: %s, Timeframe: %s)", fileName, symbol, timeframe))

	// Upload the file
	resp, err := cum.api.UploadCSV(filePath, symbol, timeframe)
	if err != nil {
		return fmt.Errorf("upload failed for %s: %w", fileName, err)
	}

	if !resp.Success {
		return fmt.Errorf("upload failed for %s: %s", fileName, resp.Message)
	}

	cum.logf(fmt.Sprintf("CSV upload success: jobId=%s", resp.JobID))
	fmt.Printf("Successfully uploaded %s (Symbol: %s, Timeframe: %s)\n", fileName, symbol, timeframe)
	return nil
}

// extractSymbolAndTimeframe extracts symbol and timeframe from CSV filename
// This is a simple implementation - you may need to adjust based on your naming convention
func (cum *CSVUploadManager) extractSymbolAndTimeframe(fileName string) (string, string, error) {
	// Remove .csv extension
	name := strings.TrimSuffix(fileName, ".csv")

	// Split by underscore to get parts
	parts := strings.Split(name, "_")
	if len(parts) < 2 {
		return "", "", fmt.Errorf("filename does not contain symbol and timeframe: %s", fileName)
	}

	// Assume first part is symbol and second part is timeframe
	symbol := parts[0]
	timeframe := parts[1]

	// Validate symbol format (should start with @)
	if !strings.HasPrefix(symbol, "@") {
		symbol = "@" + symbol
	}

	return symbol, timeframe, nil
}

// GetUploadStats returns statistics about CSV uploads
func (cum *CSVUploadManager) GetUploadStats() (int, int, error) {
	toDoFiles, err := cum.fileMgr.GetCSVFiles()
	if err != nil {
		return 0, 0, err
	}

	doneFiles, err := os.ReadDir(cum.config.Folders.Files.Results.Done)
	if err != nil {
		return len(toDoFiles), 0, nil
	}

	doneCount := 0
	for _, entry := range doneFiles {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".csv" {
			doneCount++
		}
	}

	return len(toDoFiles), doneCount, nil
}

// OptUploadManager handles automatic .opt file uploads
type OptUploadManager struct {
	config    *Config
	api       *APIClient
	fileMgr   *FileManager
	isRunning bool
	stopCh    chan bool
	mutex     sync.Mutex
	logf      func(string)
}

func NewOptUploadManager(cfg *Config, api *APIClient) *OptUploadManager {
	return &OptUploadManager{
		config:  cfg,
		api:     api,
		fileMgr: NewFileManager(cfg),
		stopCh:  make(chan bool),
		logf:    func(string) {},
	}
}

func (oum *OptUploadManager) SetLogger(fn func(string)) {
	if fn != nil {
		oum.logf = fn
	}
}

// Start begins monitoring the Opt/In folder for .opt files
func (oum *OptUploadManager) Start() error {
	oum.mutex.Lock()
	defer oum.mutex.Unlock()

	if oum.isRunning {
		return fmt.Errorf("OPT upload manager is already running")
	}

	oum.isRunning = true
	oum.stopCh = make(chan bool)
	oum.logf("OPT monitoring started")

	go oum.monitorOptFolder()
	return nil
}

// Stop stops monitoring the Opt/In folder
func (oum *OptUploadManager) Stop() {
	oum.mutex.Lock()
	defer oum.mutex.Unlock()

	if !oum.isRunning {
		return
	}

	oum.isRunning = false
	close(oum.stopCh)
	oum.logf("OPT monitoring stopped")
}

// monitorOptFolder continuously monitors the Opt/In folder for .opt files
func (oum *OptUploadManager) monitorOptFolder() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-oum.stopCh:
			return
		case <-ticker.C:
			oum.processOptFiles()
		}
	}
}

// processOptFiles processes all .opt files
func (oum *OptUploadManager) processOptFiles() {
	files, err := oum.fileMgr.GetOptFiles()
	if err != nil {
		oum.logf(fmt.Sprintf("Error getting OPT files: %v", err))
		return
	}
	for _, fileName := range files {
		if err := oum.uploadOptFile(fileName); err != nil {
			oum.logf(fmt.Sprintf("OPT upload failed for %s: %v", fileName, err))
			continue
		}
		if err := oum.fileMgr.MoveOptFile(fileName); err != nil {
			oum.logf(fmt.Sprintf("Error moving OPT file %s to Done: %v", fileName, err))
		} else {
			oum.logf(fmt.Sprintf("OPT file moved to Done: %s", fileName))
		}
	}
}

// uploadOptFile uploads a single .opt file
func (oum *OptUploadManager) uploadOptFile(fileName string) error {
	jobID, _, _, err := oum.extractMetadata(fileName)
	if err != nil {
		return fmt.Errorf("failed to extract metadata from filename %s: %w", fileName, err)
	}
	filePath := filepath.Join(oum.config.Folders.Files.Opt.In, fileName)
	oum.logf(fmt.Sprintf("Uploading OPT %s for job %s", fileName, jobID))
	resp, err := oum.api.UploadOpt(filePath, jobID, "performance")
	if err != nil {
		return fmt.Errorf("upload failed for %s: %w", fileName, err)
	}
	oum.logf(fmt.Sprintf("OPT upload response: jobId=%s status=%s", resp.JobID, resp.Status))

	// Emit upload event for burst polling
	select {
	case UploadEventChan <- UploadEvent{
		EventType: "opt_upload",
		FileName:  fileName,
		JobID:     jobID,
		Timestamp: time.Now(),
	}:
		oum.logf(fmt.Sprintf("OPT upload event emitted for job %s", jobID))
	default:
		// Channel full, skip event (non-blocking)
		oum.logf("Upload event channel full, skipping OPT upload event")
	}

	// Only check for WFO processing if the filename suggests it might be a WFO job
	// This prevents unnecessary database queries and processing for regular OPT files
	if oum.shouldCheckForWFO(fileName) {
		if err := oum.checkAndTriggerCombinedWFO(fileName, jobID); err != nil {
			oum.logf(fmt.Sprintf("Warning: Combined WFO generation check failed for job %s: %v", jobID, err))
			// Don't fail the OPT upload if combined generation fails
		}
	} else {
		// Regular OPT file - skip WFO processing
		oum.logf(fmt.Sprintf("Non-WFO OPT file detected: %s - skipping WFO processing", fileName))
	}

	// After successful OPT upload, trigger daily summary folder scan after 30 seconds
	// This scans the entire Summary folder for ANY *_Daily.rep files and uploads them
	// This approach is flexible and works for all task types (RETEST, OOS, MM, MTF, etc.)
	go oum.scanAndUploadDailySummaries(30 * time.Second)

	return nil
}

// scanAndUploadDailySummaries scans the Summary folder for all *_Daily.rep files and uploads them
// This flexible approach works for all task types: RETEST, OOS, MM, MTF, etc.
// It handles complex filenames like:
//   - 5b856adb-5107-4fc9-907d-b8570bdf3f6e_@ES_60_RETEST_Daily.rep
//   - 9b739066-53d2-4062-b0e0-050120b11862_@ES_60-120-240_MTF_MTF_Daily.rep
//   - 19974dd6-c233-467b-8135-e58302bf1c99_@ES-@NQ_60_MM_MM_Daily.rep
func (oum *OptUploadManager) scanAndUploadDailySummaries(initialDelay time.Duration) {
	// Wait for files to be generated
	time.Sleep(initialDelay)
	
	oum.logf("[DAILY-SUMMARY-SCAN] Starting scan of Summary folder for *_Daily.rep files")
	
	summaryFolder := oum.config.Folders.Files.Opt.Summary
	files, err := os.ReadDir(summaryFolder)
	if err != nil {
		oum.logf(fmt.Sprintf("[DAILY-SUMMARY-SCAN] Error reading Summary folder: %v", err))
		return
	}
	
	var repFiles []string
	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(file.Name(), "_Daily.rep") {
			repFiles = append(repFiles, file.Name())
		}
	}
	
	if len(repFiles) == 0 {
		oum.logf("[DAILY-SUMMARY-SCAN] No *_Daily.rep files found in Summary folder")
		return
	}
	
	oum.logf(fmt.Sprintf("[DAILY-SUMMARY-SCAN] Found %d *_Daily.rep files to upload", len(repFiles)))
	
	// Upload each daily summary file
	for _, repFileName := range repFiles {
		// Extract jobID from filename (first part before underscore)
		jobID := oum.extractJobIDFromDailySummary(repFileName)
		if jobID == "" {
			oum.logf(fmt.Sprintf("[DAILY-SUMMARY-SCAN] Could not extract jobID from %s, skipping", repFileName))
			continue
		}
		
		oum.logf(fmt.Sprintf("[DAILY-SUMMARY-SCAN] Processing %s (job: %s)", repFileName, jobID))
		
		// Wait for backtest row to exist before uploading
		exists, err := oum.api.WaitForBacktestByJob(jobID, 60*time.Second)
		if err != nil {
			oum.logf(fmt.Sprintf("[DAILY-SUMMARY-SCAN] Error waiting for backtest row for job %s: %v", jobID, err))
			continue
		}
		if !exists {
			oum.logf(fmt.Sprintf("[DAILY-SUMMARY-SCAN] Backtest row not ready for job %s, skipping %s", jobID, repFileName))
			continue
		}
		
		// Upload the daily summary
		repPath := filepath.Join(summaryFolder, repFileName)
		resp, err := oum.api.UploadDailySummary(repPath, jobID)
		if err != nil {
			oum.logf(fmt.Sprintf("[DAILY-SUMMARY-SCAN] Upload failed for %s: %v", repFileName, err))
			_ = oum.fileMgr.MoveDailySummaryFileToError(repFileName)
			continue
		}
		
		oum.logf(fmt.Sprintf("[DAILY-SUMMARY-SCAN] ✅ Upload success for %s (job: %s, path: %s)", repFileName, jobID, resp.Path))
		_ = oum.fileMgr.MoveDailySummaryFile(repFileName)
	}
	
	oum.logf(fmt.Sprintf("[DAILY-SUMMARY-SCAN] Completed - processed %d files", len(repFiles)))
}

// extractJobIDFromDailySummary extracts the jobID from a daily summary filename
// Handles various formats:
//   - 5b856adb-5107-4fc9-907d-b8570bdf3f6e_@ES_60_RETEST_Daily.rep -> jobID
//   - 9b739066-53d2-4062-b0e0-050120b11862_@ES_60-120-240_MTF_MTF_Daily.rep -> jobID
//   - 19974dd6-c233-467b-8135-e58302bf1c99_@ES-@NQ_60_MM_MM_Daily.rep -> jobID
func (oum *OptUploadManager) extractJobIDFromDailySummary(fileName string) string {
	// JobID is always the first part before the first underscore
	parts := strings.Split(fileName, "_")
	if len(parts) > 0 {
		return parts[0]
	}
	return ""
}

// waitForFile polls for file existence until timeout
func waitForFile(path string, timeout, interval time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return true
		}
		time.Sleep(interval)
	}
	return false
}

// shouldCheckForWFO determines if a filename indicates a potential WFO job
// This helps avoid unnecessary database queries for regular OPT files
func (oum *OptUploadManager) shouldCheckForWFO(fileName string) bool {
	// Convert to uppercase for case-insensitive matching
	fileNameUpper := strings.ToUpper(fileName)
	
	// Check for WFO-related patterns in filename
	// Common patterns: *_WFO_Results.opt, *_WFM_Results.opt, *_DWFM_Results.opt
	if strings.Contains(fileNameUpper, "_WFO_") || 
	   strings.Contains(fileNameUpper, "_WFM_") || 
	   strings.Contains(fileNameUpper, "_DWFM_") {
		return true
	}
	
	// If filename contains OPT but not WFO patterns, it's likely a regular OPT file
	if strings.Contains(fileNameUpper, "_OPT_") {
		return false
	}
	
	// For ambiguous cases, default to checking (safer approach)
	// This maintains backward compatibility but should be rare
	return false
}

// extractMetadata extracts job_id, symbol, timeframe from filename
func (oum *OptUploadManager) extractMetadata(fileName string) (string, string, string, error) {
	name := strings.TrimSuffix(fileName, filepath.Ext(fileName))
	parts := strings.Split(name, "_")
	if len(parts) < 3 {
		return "", "", "", fmt.Errorf("filename does not contain expected parts: %s", fileName)
	}
	jobID := parts[0]
	symbol := parts[1]
	timeframe := parts[2]
	if !strings.HasPrefix(symbol, "@") {
		symbol = "@" + symbol
	}
	return jobID, symbol, timeframe, nil
}

// extractTaskTypeFromFilename extracts the task type from OPT result filename
// Expected format: <jobId>_<symbol>_<timeframe>_<TASK>_Results.opt
// Examples: 
//   - 5b856adb-5107-4fc9-907d-b8570bdf3f6e_@ES_60_RETEST_Results.opt -> "RETEST"
//   - 5b856adb-5107-4fc9-907d-b8570bdf3f6e_@ES_60_OPT_Results.opt -> "OPT"
func (oum *OptUploadManager) extractTaskTypeFromFilename(fileName string) string {
	name := strings.TrimSuffix(fileName, filepath.Ext(fileName))
	parts := strings.Split(name, "_")
	
	// Look for the task type pattern: it comes before "Results"
	// Format: <jobId>_<symbol>_<timeframe>_<TASK>_Results
	for i, part := range parts {
		if part == "Results" && i > 0 {
			taskType := parts[i-1]
			// Validate it's a known task type
			taskTypeUpper := strings.ToUpper(taskType)
			if taskTypeUpper == "OPT" || taskTypeUpper == "RETEST" || taskTypeUpper == "OOS" || 
			   taskTypeUpper == "MM" || taskTypeUpper == "MTF" || taskTypeUpper == "WFO" {
				return taskTypeUpper
			}
		}
	}
	
	// Fallback: return empty string if no task type found
	return ""
}

// checkAndTriggerCombinedWFO checks if the uploaded OPT file is from a WFO job and triggers combined daily summary generation
func (oum *OptUploadManager) checkAndTriggerCombinedWFO(fileName, jobID string) error {
	fmt.Printf("[2025-09-23 15:34:46] INFO: Checking if job %s is WFO for combined daily summary generation\n", jobID)
	oum.logf(fmt.Sprintf("Checking if job %s is WFO for combined daily summary generation", jobID))

	// Get job information from database to check task_type
	// If job not found, it might be an older job or from a different workflow
	job, err := oum.api.GetJobByID(jobID)
	if err != nil {
		fmt.Printf("[WARN] Failed to fetch job information for %s: %v\n", jobID, err)
		oum.logf(fmt.Sprintf("WARN: Failed to fetch job information for %s: %v", jobID, err))
		
		// If job not found, we can still try to parse the OPT file to determine if it's WFO
		// This handles cases where jobs are cleaned up but OPT files remain
		fmt.Printf("[INFO] Job %s not found in database, will attempt to detect WFO from filename and file content\n", jobID)
		oum.logf(fmt.Sprintf("Job %s not found in database, will attempt to detect WFO from filename and file content", jobID))
		
		// Use unknown task type - parseOPTFile will determine WFO status from file content
		job = &Job{TaskType: "UNKNOWN", ID: jobID}
	}

	fmt.Printf("[DEBUG] Job task_type for %s: %s\n", jobID, job.TaskType)
	oum.logf(fmt.Sprintf("Job task_type for %s: %s", jobID, job.TaskType))

	// Read and analyze the OPT file to determine if it's from a WFO job
	filePath := filepath.Join(oum.config.Folders.Files.Opt.Done, fileName)

	// Check if file was moved to Done folder (successful upload)
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		// File might still be in In folder if move failed
		filePath = filepath.Join(oum.config.Folders.Files.Opt.In, fileName)
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			return fmt.Errorf("OPT file not found in In or Done folders: %s", fileName)
		}
	}

	fmt.Printf("[2025-09-23 15:34:46] INFO: Parsing OPT file: %s\n", filePath)
	oum.logf(fmt.Sprintf("Parsing OPT file: %s", filePath))

	optResults, isWFO, err := oum.parseOPTFile(filePath, job.TaskType)

	// Simplified parsing results logging (remove excessive debug output)
	fmt.Printf("[INFO] parseOPTFile Results: %d runs found, isWFO=%v\n", len(optResults), isWFO)
	oum.logf(fmt.Sprintf("parseOPTFile Results: %d runs found, isWFO=%v", len(optResults), isWFO))

	if err != nil {
		fmt.Printf("[ERROR] Failed to parse OPT file: %v\n", err)
		oum.logf(fmt.Sprintf("ERROR: Failed to parse OPT file: %v", err))
		return fmt.Errorf("failed to parse OPT file: %w", err)
	}

	// Remove redundant debug logging

	if !isWFO {
		fmt.Printf("[INFO] Job %s is not a WFO job, skipping combined daily summary generation\n", jobID)
		oum.logf(fmt.Sprintf("Job %s is not a WFO job, skipping combined daily summary generation", jobID))
		return nil
	}

	fmt.Printf("[INFO] WFO job detected for %s with %d runs, initiating combined daily summary generation\n", jobID, len(optResults))
	oum.logf(fmt.Sprintf("WFO job detected for %s with %d runs, initiating combined daily summary generation", jobID, len(optResults)))

	// Show summary of WFO runs found (limit verbose output)
	fmt.Printf("[INFO] WFO_RETEST Generation: Processing %d runs\n", len(optResults))
	oum.logf(fmt.Sprintf("WFO_RETEST Generation: Processing %d runs", len(optResults)))

	// Show first few runs for debugging, but don't spam logs with hundreds of entries
	maxRunsToLog := 3
	if len(optResults) <= maxRunsToLog {
		maxRunsToLog = len(optResults)
	}

	if maxRunsToLog > 0 {
		fmt.Printf("[INFO] WFO_RETEST Generation: Sample runs (showing first %d of %d):\n", maxRunsToLog, len(optResults))
		oum.logf(fmt.Sprintf("WFO_RETEST Generation: Sample runs (showing first %d of %d)", maxRunsToLog, len(optResults)))

		for i := 0; i < maxRunsToLog; i++ {
			result := optResults[i]
			fmt.Printf("[INFO] WFO_RETEST Generation:   Run %d: Parameters JSON = %.80s...\n", i+1, result.ParametersJSON)
			oum.logf(fmt.Sprintf("WFO_RETEST Generation: Run %d: Parameters JSON = %.80s...", i+1, result.ParametersJSON))
		}

		if len(optResults) > maxRunsToLog {
			fmt.Printf("[INFO] WFO_RETEST Generation:   ... and %d more runs (details suppressed to reduce log size)\n", len(optResults)-maxRunsToLog)
			oum.logf(fmt.Sprintf("WFO_RETEST Generation: ... and %d more runs (details suppressed to reduce log size)", len(optResults)-maxRunsToLog))
		}
	}

	// Trigger WFO_RETEST generation process
	fmt.Printf("[INFO] WFO_RETEST Generation: Starting complete XML generation process\n")
	oum.logf(fmt.Sprintf("WFO_RETEST Generation: Starting complete XML generation process"))

	fmt.Printf("[INFO] WFO_RETEST Generation: About to call triggerWFORetestGeneration with jobID=%s and %d optResults\n", jobID, len(optResults))
	oum.logf(fmt.Sprintf("WFO_RETEST Generation: About to call triggerWFORetestGeneration with jobID=%s and %d optResults", jobID, len(optResults)))

	err = oum.triggerWFORetestGeneration(jobID, optResults)
	if err != nil {
		fmt.Printf("[ERROR] WFO_RETEST Generation: triggerWFORetestGeneration returned error: %v\n", err)
		fmt.Printf("[ERROR] WFO_RETEST Generation: Error type: %T\n", err)
		fmt.Printf("[ERROR] WFO_RETEST Generation: Full error details: %+v\n", err)
		oum.logf(fmt.Sprintf("ERROR: WFO_RETEST Generation: triggerWFORetestGeneration returned error: %v", err))
		oum.logf(fmt.Sprintf("ERROR: WFO_RETEST Generation: Error type: %T", err))
		return fmt.Errorf("failed to trigger WFO_RETEST generation: %w", err)
	}

	fmt.Printf("[INFO] WFO_RETEST Generation: triggerWFORetestGeneration completed successfully\n")
	oum.logf(fmt.Sprintf("WFO_RETEST Generation: triggerWFORetestGeneration completed successfully"))

	fmt.Printf("[INFO] WFO_RETEST Generation: Process completed successfully\n")
	oum.logf(fmt.Sprintf("WFO_RETEST Generation: Process completed successfully"))

	fmt.Printf("[INFO] Combined WFO generation triggered successfully for job %s\n", jobID)
	oum.logf(fmt.Sprintf("Combined WFO generation triggered successfully for job %s", jobID))
	return nil
}

// triggerWFORetestGeneration handles the detailed WFO_RETEST XML generation process
func (oum *OptUploadManager) triggerWFORetestGeneration(jobID string, optResults []OPTResult) error {
	fmt.Printf("[INFO] WFO_RETEST Trigger: ================== ENTERING FUNCTION ==================\n")
	oum.logf(fmt.Sprintf("WFO_RETEST Trigger: ================== ENTERING FUNCTION =================="))

	fmt.Printf("[INFO] WFO_RETEST Trigger: Starting detailed XML generation for job %s\n", jobID)
	oum.logf(fmt.Sprintf("WFO_RETEST Trigger: Starting detailed XML generation for job %s", jobID))

	fmt.Printf("[INFO] WFO_RETEST Trigger: Received %d optResults for processing\n", len(optResults))
	oum.logf(fmt.Sprintf("WFO_RETEST Trigger: Received %d optResults for processing", len(optResults)))

	// Validate inputs
	if jobID == "" {
		fmt.Printf("[ERROR] WFO_RETEST Trigger: jobID is empty\n")
		oum.logf(fmt.Sprintf("ERROR: WFO_RETEST Trigger: jobID is empty"))
		return fmt.Errorf("jobID cannot be empty")
	}

	if len(optResults) == 0 {
		fmt.Printf("[ERROR] WFO_RETEST Trigger: optResults is empty\n")
		oum.logf(fmt.Sprintf("ERROR: WFO_RETEST Trigger: optResults is empty"))
		return fmt.Errorf("optResults cannot be empty")
	}

	fmt.Printf("[INFO] WFO_RETEST Trigger: Input validation passed\n")
	oum.logf(fmt.Sprintf("WFO_RETEST Trigger: Input validation passed"))

	// Extract metadata from the OPT filename since job ID is just a UUID
	optFileName := fmt.Sprintf("%s_@ES_60_WFO_Results.opt", jobID) // Reconstruct the filename
	fmt.Printf("[INFO] WFO_RETEST Trigger: Using OPT filename for metadata: %s\n", optFileName)
	oum.logf(fmt.Sprintf("WFO_RETEST Trigger: Using OPT filename for metadata: %s", optFileName))

	fmt.Printf("[INFO] WFO_RETEST Trigger: About to call extractMetadata...\n")
	oum.logf(fmt.Sprintf("WFO_RETEST Trigger: About to call extractMetadata..."))

	_, symbol, timeframe, err := oum.extractMetadata(optFileName)
	if err != nil {
		fmt.Printf("[ERROR] WFO_RETEST Trigger: Failed to extract metadata from filename - %v\n", err)
		fmt.Printf("[ERROR] WFO_RETEST Trigger: extractMetadata error type: %T\n", err)
		oum.logf(fmt.Sprintf("ERROR: WFO_RETEST Trigger: Failed to extract metadata from filename - %v", err))
		return fmt.Errorf("extract metadata from filename: %w", err)
	}

	fmt.Printf("[INFO] WFO_RETEST Trigger: extractMetadata completed successfully\n")
	oum.logf(fmt.Sprintf("WFO_RETEST Trigger: extractMetadata completed successfully"))

	fmt.Printf("[INFO] WFO_RETEST Trigger: Extracted symbol=%s, timeframe=%s from filename\n", symbol, timeframe)
	oum.logf(fmt.Sprintf("WFO_RETEST Trigger: Extracted symbol=%s, timeframe=%s from filename", symbol, timeframe))

	// Validate API client
	if oum.api == nil {
		fmt.Printf("[ERROR] WFO_RETEST Trigger: API client is nil\n")
		oum.logf(fmt.Sprintf("ERROR: WFO_RETEST Trigger: API client is nil"))
		return fmt.Errorf("API client is not initialized")
	}

	fmt.Printf("[INFO] WFO_RETEST Trigger: API client validation passed\n")
	oum.logf(fmt.Sprintf("WFO_RETEST Trigger: API client validation passed"))

	// Call the enhanced WFO_RETEST generation process with detailed tracking
	fmt.Printf("[INFO] WFO_RETEST Trigger: About to call processWFORetestGenerationWithFallback...\n")
	oum.logf(fmt.Sprintf("WFO_RETEST Trigger: About to call processWFORetestGenerationWithFallback..."))

	fmt.Printf("[INFO] WFO_RETEST Trigger: Parameters: jobID=%s, optResults count=%d\n", jobID, len(optResults))
	oum.logf(fmt.Sprintf("WFO_RETEST Trigger: Parameters: jobID=%s, optResults count=%d", jobID, len(optResults)))

	err = oum.api.processWFORetestGenerationWithFallback(jobID, symbol, timeframe, optResults)
	if err != nil {
		fmt.Printf("[ERROR] WFO_RETEST Trigger: processWFORetestGenerationWithFallback returned error: %v\n", err)
		fmt.Printf("[ERROR] WFO_RETEST Trigger: Error type: %T\n", err)
		fmt.Printf("[ERROR] WFO_RETEST Trigger: Full error details: %+v\n", err)
		oum.logf(fmt.Sprintf("ERROR: WFO_RETEST Trigger: processWFORetestGenerationWithFallback returned error: %v", err))
		return fmt.Errorf("WFO_RETEST generation: %w", err)
	}

	fmt.Printf("[INFO] WFO_RETEST Trigger: processWFORetestGenerationWithFallback completed successfully\n")
	oum.logf(fmt.Sprintf("WFO_RETEST Trigger: processWFORetestGenerationWithFallback completed successfully"))

	fmt.Printf("[INFO] WFO_RETEST Trigger: XML generation process completed successfully\n")
	oum.logf(fmt.Sprintf("WFO_RETEST Trigger: XML generation process completed successfully"))

	fmt.Printf("[INFO] WFO_RETEST Trigger: ================== EXITING FUNCTION ==================\n")
	oum.logf(fmt.Sprintf("WFO_RETEST Trigger: ================== EXITING FUNCTION =================="))
	return nil
}

// parseOPTFile reads and parses an OPT file to extract WFO run information
func (oum *OptUploadManager) parseOPTFile(filePath string, jobTaskType string) ([]OPTResult, bool, error) {
	fmt.Printf("[INFO] Parsing OPT file: %s\n", filePath)
	oum.logf(fmt.Sprintf("Parsing OPT file: %s", filePath))

	// Step 1: Decompress the zlib-compressed OPT file
	csvContent, err := oum.decompressOPTFile(filePath)
	if err != nil {
		fmt.Printf("[ERROR] Failed to decompress OPT file - %v\n", err)
		return nil, false, fmt.Errorf("decompress OPT file: %w", err)
	}

	// Step 2: Parse CSV content to extract WFO run information
	optResults, isWFO, err := oum.parseCSVContent(csvContent, jobTaskType)
	if err != nil {
		fmt.Printf("[ERROR] Failed to parse CSV content - %v\n", err)
		return nil, false, fmt.Errorf("parse CSV content: %w", err)
	}

	if !isWFO {
		fmt.Printf("[INFO] No WFO pattern found - not a WFO job\n")
		return nil, false, nil
	}

	fmt.Printf("[INFO] Successfully parsed %d WFO runs\n", len(optResults))
	return optResults, true, nil
}

// decompressOPTFile decompresses a zlib-compressed OPT file and returns CSV content
func (oum *OptUploadManager) decompressOPTFile(filePath string) (string, error) {
	fmt.Printf("[DEBUG] OPT Decompression: Starting zlib decompression of file: %s\n", filePath)

	// Open the compressed file
	file, err := os.Open(filePath)
	if err != nil {
		fmt.Printf("[ERROR] OPT Decompression: Failed to open compressed file - %v\n", err)
		return "", fmt.Errorf("open compressed file: %w", err)
	}
	defer file.Close()

	fmt.Printf("[DEBUG] OPT Decompression: Creating zlib reader\n")

	// Create zlib reader (same as decompress-rep.go script)
	reader, err := zlib.NewReader(file)
	if err != nil {
		fmt.Printf("[ERROR] OPT Decompression: Failed to create zlib reader - %v\n", err)
		return "", fmt.Errorf("create zlib reader (file may not be zlib compressed): %w", err)
	}
	defer reader.Close()

	// Read decompressed content into memory
	fmt.Printf("[DEBUG] OPT Decompression: Reading decompressed CSV content\n")
	var csvBuffer strings.Builder
	bytesRead, err := io.Copy(&csvBuffer, reader)
	if err != nil {
		fmt.Printf("[ERROR] OPT Decompression: Failed to read decompressed data - %v\n", err)
		return "", fmt.Errorf("decompress data: %w", err)
	}

	csvContent := csvBuffer.String()
	fmt.Printf("[DEBUG] OPT Decompression: Successfully decompressed %d bytes to %d bytes of CSV content\n", bytesRead, len(csvContent))

	// Save decompressed content to a fixed debug location for easy access
	debugDir := "C:\\AlphaWeaver\\debug"
	if err := os.MkdirAll(debugDir, 0755); err != nil {
		fmt.Printf("[WARN] OPT Decompression: Failed to create debug directory: %v\n", err)
	}

	baseFileName := filepath.Base(filePath)
	debugFileName := strings.TrimSuffix(baseFileName, ".opt") + "_decompressed.csv"
	debugFilePath := filepath.Join(debugDir, debugFileName)

	fmt.Printf("[DEBUG] OPT Decompression: Temp dir location: %s\n", os.TempDir())
	fmt.Printf("[DEBUG] OPT Decompression: Attempting to save debug file to: %s\n", debugFilePath)

	if err := os.WriteFile(debugFilePath, []byte(csvContent), 0644); err != nil {
		fmt.Printf("[ERROR] OPT Decompression: Failed to save debug file: %v\n", err)
	} else {
		fmt.Printf("[INFO] OPT Decompression: Successfully saved decompressed content to: %s\n", debugFilePath)
	}

	fmt.Printf("[DEBUG] OPT Decompression: First 500 characters of decompressed content:\n%.500s\n", csvContent)

	return csvContent, nil
}

// parseCSVContent parses CSV content to extract WFO run information and parameters
func (oum *OptUploadManager) parseCSVContent(csvContent string, jobTaskType string) ([]OPTResult, bool, error) {
	// CRITICAL FIX: Validate job task_type BEFORE checking CSV structure
	if jobTaskType != "WFO" && jobTaskType != "WFM" {
		fmt.Printf("[INFO] Job task_type '%s' is not WFO/WFM - skipping WFO processing\n", jobTaskType)
		return nil, false, nil
	}

	// Handle CSV content with JSON parameters that contain commas
	records, err := oum.parseCSVWithJSONFields(csvContent)
	if err != nil {
		fmt.Printf("[ERROR] Failed to parse CSV with JSON handling - %v\n", err)
		return nil, false, fmt.Errorf("parse CSV: %w", err)
	}

	if len(records) < 2 { // Need header + at least one data row
		return nil, false, nil
	}

	// Analyze header to determine if this is a WFO file
	header := records[0]

	// Look for WFO-specific columns
	hasRunColumn := false
	hasParametersJSONColumn := false
	runIndex := -1
	parametersJSONIndex := -1
	isStartIndex, isEndIndex, osStartIndex, osEndIndex := -1, -1, -1, -1

	for i, col := range header {
		colLower := strings.ToLower(strings.TrimSpace(col))
		if colLower == "run" || colLower == "run_number" {
			hasRunColumn = true
			runIndex = i
		}
		if colLower == "parameters_json" || colLower == "parameters json" {
			hasParametersJSONColumn = true
			parametersJSONIndex = i
		}
		// Look for IS/OS date range columns (based on actual CSV headers)
		if colLower == "is_start_date" {
			isStartIndex = i
		}
		if colLower == "is_end_date" {
			isEndIndex = i
		}
		if colLower == "os_start_date" {
			osStartIndex = i
		}
		if colLower == "os_end_date" {
			osEndIndex = i
		}
	}

	// Check if this appears to be a WFO file
	if !hasRunColumn || !hasParametersJSONColumn {
		return nil, false, nil
	}

	// Parse WFO runs
	var optResults []OPTResult

	for i, record := range records[1:] { // Skip header
		if len(record) <= runIndex || len(record) <= parametersJSONIndex {
			fmt.Printf("[WARN] CSV Content Parsing: Skipping invalid record %d (insufficient columns)\n", i+1)
			continue
		}

		// Parse run number
		runStr := strings.TrimSpace(record[runIndex])
		run, err := strconv.Atoi(runStr)
		if err != nil {
			fmt.Printf("[WARN] CSV Content Parsing: Skipping record %d - invalid run number: %s\n", i+1, runStr)
			continue
		}

		// Extract parameters JSON
		parametersJSON := strings.TrimSpace(record[parametersJSONIndex])
		if parametersJSON == "" {
			fmt.Printf("[WARN] CSV Content Parsing: Skipping record %d - empty parameters JSON\n", i+1)
			continue
		}

		// Extract date ranges if available
		var isStartDate, isEndDate, osStartDate, osEndDate string

		if isStartIndex >= 0 && isStartIndex < len(record) {
			isStartDate = strings.TrimSpace(record[isStartIndex])
		}
		if isEndIndex >= 0 && isEndIndex < len(record) {
			isEndDate = strings.TrimSpace(record[isEndIndex])
		}
		if osStartIndex >= 0 && osStartIndex < len(record) {
			osStartDate = strings.TrimSpace(record[osStartIndex])
		}
		if osEndIndex >= 0 && osEndIndex < len(record) {
			osEndDate = strings.TrimSpace(record[osEndIndex])
		}

		// Create OPTResult with complete data
		result := OPTResult{
			Run:            run,
			ParametersJSON: parametersJSON,
			ISStartDate:    isStartDate,
			ISEndDate:      isEndDate,
			OSStartDate:    osStartDate,
			OSEndDate:      osEndDate,
		}

		fmt.Printf("[DEBUG] CSV Content Parsing: Extracted WFO run %d with parameters: %.100s...\n", run, parametersJSON)
		fmt.Printf("[DEBUG] CSV Content Parsing:   Run %d dates - IS: %s to %s, OS: %s to %s\n", run, isStartDate, isEndDate, osStartDate, osEndDate)
		optResults = append(optResults, result)
	}

	if len(optResults) == 0 {
		fmt.Printf("[DEBUG] CSV Content Parsing: No valid WFO runs found\n")
		return nil, false, nil
	}

	fmt.Printf("[DEBUG] CSV Content Parsing: Successfully extracted %d WFO runs\n", len(optResults))

	// Clear CSV content from memory after successful processing
	// This ensures sensitive data doesn't persist longer than necessary
	defer func() {
		csvContent = "" // Clear the string content
		fmt.Printf("[DEBUG] CSV Content Parsing: Cleared CSV content from memory after processing\n")
	}()

	return optResults, true, nil
}

// parseCSVWithJSONFields parses CSV content that may contain unquoted JSON fields with commas
func (oum *OptUploadManager) parseCSVWithJSONFields(csvContent string) ([][]string, error) {
	fmt.Printf("[DEBUG] JSON-aware CSV Parsing: Starting specialized parsing for JSON fields\n")

	lines := strings.Split(csvContent, "\n")
	var records [][]string

	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue // Skip empty lines
		}

		// Parse each line handling JSON fields that contain commas
		fields, err := oum.parseCSVLineWithJSON(line)
		if err != nil {
			fmt.Printf("[ERROR] JSON-aware CSV Parsing: Failed to parse line %d: %v\n", i+1, err)
			fmt.Printf("[ERROR] JSON-aware CSV Parsing: Problematic line: %q\n", line)
			return nil, fmt.Errorf("parse line %d: %w", i+1, err)
		}

		records = append(records, fields)

		// Log first few records for debugging with field content
		if i < 3 {
			fmt.Printf("[DEBUG] JSON-aware CSV Parsing: Line %d parsed into %d fields\n", i+1, len(fields))
			for j, field := range fields {
				if strings.Contains(field, "{") {
					fmt.Printf("[DEBUG] JSON-aware CSV Parsing:   Field %d (JSON): %.100s...\n", j, field)
				} else if j < 5 { // Show first 5 non-JSON fields
					fmt.Printf("[DEBUG] JSON-aware CSV Parsing:   Field %d: %s\n", j, field)
				}
			}
		}
	}

	fmt.Printf("[DEBUG] JSON-aware CSV Parsing: Successfully parsed %d lines\n", len(records))
	return records, nil
}

// parseCSVLineWithJSON parses a single CSV line handling JSON fields and parameter fields with commas
func (oum *OptUploadManager) parseCSVLineWithJSON(line string) ([]string, error) {
	// Use a simple approach: split by commas, then merge back fields that were split incorrectly

	// First, do a simple comma split
	parts := strings.Split(line, ",")

	var fields []string
	i := 0

	for i < len(parts) {
		currentField := parts[i]

		// Check if this part starts a JSON field
		if strings.Contains(currentField, "{") && !strings.Contains(currentField, "}") {
			// This is the start of a JSON field that got split - merge until we find the end
			for i+1 < len(parts) && !strings.Contains(currentField, "}") {
				i++
				currentField += "," + parts[i]
			}
		} else if strings.Contains(currentField, "=") && !strings.Contains(currentField, "{") {
			// This might be a parameter field (key=value format)
			// Look ahead to see if the next parts also look like key=value pairs
			originalField := currentField
			tempI := i

			// Check if next few parts also contain '=' (indicating they're part of the same parameter field)
			for tempI+1 < len(parts) && strings.Contains(parts[tempI+1], "=") && !strings.Contains(parts[tempI+1], "{") {
				tempI++
				currentField += "," + parts[tempI]
			}

			// Only merge if we found additional parameter pairs
			if tempI > i {
				i = tempI
			} else {
				currentField = originalField
			}
		}

		fields = append(fields, strings.TrimSpace(currentField))
		i++
	}

	return fields, nil
}

// DailySummaryUploadManager manages uploading daily summary JSON files
type DailySummaryUploadManager struct {
	api        *APIClient
	fileMgr    *FileManager
	config     *Config
	mutex      sync.Mutex
	isRunning  bool
	stopCh     chan struct{}
	logf       func(string)
}

// NewDailySummaryUploadManager creates a new DailySummaryUploadManager
func NewDailySummaryUploadManager(api *APIClient, fileMgr *FileManager, config *Config) *DailySummaryUploadManager {
	return &DailySummaryUploadManager{
		api:     api,
		fileMgr: fileMgr,
		config:  config,
		logf:    func(s string) {}, // default no-op logger
	}
}

// SetLogger sets the logging function
func (dsum *DailySummaryUploadManager) SetLogger(fn func(string)) {
	if fn != nil {
		dsum.logf = fn
	}
}

// Start begins monitoring the Opt/Summary folder for .rep files
func (dsum *DailySummaryUploadManager) Start() error {
	dsum.mutex.Lock()
	defer dsum.mutex.Unlock()

	if dsum.isRunning {
		return nil
	}

	dsum.isRunning = true
	dsum.stopCh = make(chan struct{})
	dsum.logf("Daily summary monitoring started")

	go dsum.monitorSummaryFolder()
	return nil
}

// Stop stops monitoring the Opt/Summary folder
func (dsum *DailySummaryUploadManager) Stop() {
	dsum.mutex.Lock()
	defer dsum.mutex.Unlock()

	if !dsum.isRunning {
		return
	}
	dsum.isRunning = false
	close(dsum.stopCh)
	dsum.logf("Daily summary monitoring stopped")
}

// monitorSummaryFolder continuously monitors the Opt/Summary folder for .rep files
func (dsum *DailySummaryUploadManager) monitorSummaryFolder() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-dsum.stopCh:
			return
		case <-ticker.C:
			if err := dsum.processFiles(); err != nil {
				dsum.logf(fmt.Sprintf("Error processing daily summary files: %v", err))
			}
		}
	}
}

// processFiles processes all .rep files in the Opt/Summary folder
func (dsum *DailySummaryUploadManager) processFiles() error {
	files, err := dsum.fileMgr.GetDailySummaryFiles()
	if err != nil {
		return fmt.Errorf("failed to get daily summary files: %w", err)
	}

	for _, fileName := range files {
		if err := dsum.uploadDailySummaryFile(fileName); err != nil {
			dsum.logf(fmt.Sprintf("Failed to upload daily summary %s: %v", fileName, err))
			
			// Move failed upload to error folder
			if moveErr := dsum.fileMgr.MoveDailySummaryFileToError(fileName); moveErr != nil {
				dsum.logf(fmt.Sprintf("Failed to move daily summary file %s to error folder: %v", fileName, moveErr))
			} else {
				dsum.logf(fmt.Sprintf("Daily summary %s moved to error folder", fileName))
			}
			continue
		}

		// Move to done folder after successful upload
		if err := dsum.fileMgr.MoveDailySummaryFile(fileName); err != nil {
			dsum.logf(fmt.Sprintf("Failed to move daily summary file %s: %v", fileName, err))
		} else {
			dsum.logf(fmt.Sprintf("Daily summary %s moved to done folder", fileName))
		}
	}

	return nil
}

// uploadDailySummaryFile uploads a single daily summary .rep file
func (dsum *DailySummaryUploadManager) uploadDailySummaryFile(fileName string) error {
	jobID, _, _, err := dsum.extractMetadata(fileName)
	if err != nil {
		return fmt.Errorf("failed to extract metadata from filename %s: %w", fileName, err)
	}

	filePath := filepath.Join(dsum.config.Folders.Files.Opt.Summary, fileName)
	dsum.logf(fmt.Sprintf("Uploading daily summary %s for job %s", fileName, jobID))

	resp, err := dsum.api.UploadDailySummary(filePath, jobID)
	if err != nil {
		return fmt.Errorf("upload failed for %s: %w", fileName, err)
	}

	dsum.logf(fmt.Sprintf("Daily summary upload response: jobId=%s status=%s", resp.JobID, resp.Status))

	// Emit upload event for burst polling
	select {
	case UploadEventChan <- UploadEvent{
		EventType: "daily_summary_upload",
		FileName:  fileName,
		JobID:     jobID,
		Timestamp: time.Now(),
	}:
		dsum.logf(fmt.Sprintf("Daily summary upload event emitted for job %s", jobID))
	default:
		// Channel full, skip event (non-blocking)
		dsum.logf("Upload event channel full, skipping daily summary upload event")
	}

	return nil
}

// extractMetadata extracts job_id, symbol, timeframe from daily summary filename
func (dsum *DailySummaryUploadManager) extractMetadata(fileName string) (string, string, string, error) {
	// TSClient daily summary files have these formats:
	// Standard: {job_id}_{symbol}_{timeframe}_Daily.rep
	// Example: 13ee7df8-d999-47af-a9ab-124fc20a0961_@ES_60_Daily.rep
	//
	// Parse the filename to extract job_id, symbol, and timeframe
	// Remove the .rep extension first
	nameWithoutExt := strings.TrimSuffix(fileName, ".rep")
	
	// Remove the _Daily suffix
	if !strings.HasSuffix(nameWithoutExt, "_Daily") {
		return "", "", "", fmt.Errorf("filename %s does not end with _Daily", fileName)
	}
	nameWithoutDaily := strings.TrimSuffix(nameWithoutExt, "_Daily")
	
	// Split by underscore to get parts
	parts := strings.Split(nameWithoutDaily, "_")
	if len(parts) < 3 {
		return "", "", "", fmt.Errorf("filename %s does not have expected format: jobid_symbol_timeframe_Daily.rep", fileName)
	}
	
	// First part is job_id
	jobID := parts[0]
	
	// Second part is symbol (should have @ prefix)
	symbol := parts[1]
	if !strings.HasPrefix(symbol, "@") {
		symbol = "@" + symbol
	}
	
	// Third part is timeframe
	timeframe := parts[2]
	
	return jobID, symbol, timeframe, nil
}


// GetUploadStats returns the count of files to upload and uploaded
func (dsum *DailySummaryUploadManager) GetUploadStats() (int, int, error) {
	toDoFiles, err := dsum.fileMgr.GetDailySummaryFiles()
	if err != nil {
		return 0, 0, err
	}

	doneFiles, err := os.ReadDir(dsum.config.Folders.Files.Opt.Done)
	if err != nil {
		return len(toDoFiles), 0, nil
	}

	doneCount := 0
	for _, entry := range doneFiles {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".rep" && strings.Contains(entry.Name(), "_Daily") {
			doneCount++
		}
	}

	return len(toDoFiles), doneCount, nil
}
