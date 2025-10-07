package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

type DownloadManager struct {
	config *Config
	api    *APIClient
	sem    chan struct{}
	logf   func(string)
}

type DownloadResult struct {
	JobID     string
	FilePath  string
	Success   bool
	Error     error
	StartTime time.Time
	EndTime   time.Time
}

type DownloadStats struct {
	Total      int
	Successful int
	Failed     int
	StartTime  time.Time
	EndTime    time.Time
}

func NewDownloadManager(cfg *Config, api *APIClient) *DownloadManager {
	return &DownloadManager{
		config: cfg,
		api:    api,
		sem:    make(chan struct{}, cfg.Download.MaxConcurrent),
		logf:   func(string) {}, // default no-op logger
	}
}

// SetLogger sets the logging function
func (dm *DownloadManager) SetLogger(fn func(string)) {
	if fn != nil {
		dm.logf = fn
	}
}

func (dm *DownloadManager) DownloadJobs(jobs []Job) *DownloadStats {
	stats := &DownloadStats{Total: len(jobs), StartTime: time.Now()}
	if len(jobs) == 0 {
		stats.EndTime = time.Now()
		return stats
	}
	var wg sync.WaitGroup
	results := make(chan DownloadResult, len(jobs))
	for _, j := range jobs {
		wg.Add(1)
		go func(job Job) {
			defer wg.Done()
			results <- dm.downloadJob(job)
		}(j)
	}
	go func() { wg.Wait(); close(results) }()
	for r := range results {
		if r.Success {
			stats.Successful++
		} else {
			stats.Failed++
		}
	}
	stats.EndTime = time.Now()
	return stats
}

// extractFilenameFromXML extracts the filename element from XML content and converts .job to .xml
func (dm *DownloadManager) extractFilenameFromXML(xmlContent string) (string, error) {
	re := regexp.MustCompile(`<filename>([^<]+)</filename>`)
	matches := re.FindStringSubmatch(xmlContent)

	if len(matches) < 2 {
		return "", fmt.Errorf("filename element not found in XML")
	}
	filename := matches[1]

	// Convert .job extension to .xml for the file we're creating
	if strings.HasSuffix(filename, ".job") {
		filename = strings.TrimSuffix(filename, ".job") + ".xml"
	}

	return filename, nil
}

func (dm *DownloadManager) downloadJob(job Job) DownloadResult {
	res := DownloadResult{JobID: job.ID, StartTime: time.Now()}
	dm.sem <- struct{}{}
	defer func() { <-dm.sem }()

	// Log if this is a redownload
	if job.Redownload {
		fmt.Printf("🔄 Redownloading job %s (marked for redownload)\n", job.ID)
	}

	// Download to temporary file first to extract the correct filename from XML
	tempName := fmt.Sprintf("%s_temp.xml", job.ID)
	tempPath := filepath.Join(dm.config.Folders.Files.Jobs.ToDo, tempName)

	var lastErr error
	var finalPath string

	for attempt := 1; attempt <= dm.config.Download.RetryAttempts; attempt++ {
		if err := dm.api.DownloadFile(job.XMLURL, tempPath); err != nil {
			lastErr = fmt.Errorf("attempt %d: %w", attempt, err)
			// Log detailed error for debugging
			fmt.Printf("Download failed for job %s (attempt %d): %v\n", job.ID, attempt, err)
			if attempt < dm.config.Download.RetryAttempts {
				time.Sleep(dm.config.GetRetryDelay())
				continue
			}
		} else {
			// Read the XML content to extract the filename
			xmlContent, err := os.ReadFile(tempPath)
			if err != nil {
				lastErr = fmt.Errorf("failed to read downloaded XML: %w", err)
				break
			}

			// Extract filename from XML content
			correctFilename, err := dm.extractFilenameFromXML(string(xmlContent))
			if err != nil {
				// Fallback to old naming if filename extraction fails
				fmt.Printf("Warning: Could not extract filename from XML for job %s, using fallback naming: %v\n", job.ID, err)
				safeSymbol := strings.ReplaceAll(job.Symbol, ",", "-")
				safeTimeframe := strings.ReplaceAll(job.Timeframe, ",", "-")
				correctFilename = fmt.Sprintf("%s_%s_%s_%s.xml", job.ID, safeSymbol, safeTimeframe, job.TaskType)
			}

			// Move temp file to correct filename
			finalPath = filepath.Join(dm.config.Folders.Files.Jobs.ToDo, correctFilename)

			// Remove existing file if it exists
			if _, err := os.Stat(finalPath); err == nil {
				os.Remove(finalPath)
			}

			if err := os.Rename(tempPath, finalPath); err != nil {
				lastErr = fmt.Errorf("failed to rename temp file to correct filename: %w", err)
				// Clean up temp file
				os.Remove(tempPath)
				break
			}

			fmt.Printf("✅ Downloaded job %s with correct filename: %s\n", job.ID, correctFilename)

			// Check and fix XML file if it has empty data streams
			if err := dm.CheckAndFixXMLFile(finalPath, job.ID); err != nil {
				fmt.Printf("Warning: Failed to check/fix XML file for job %s: %v\n", job.ID, err)
				// Continue with the download even if fix fails
			}

			// Compress the downloaded XML file to .job format
			compressedPath, err := CompressXMLFile(finalPath, true) // Delete original XML after compression
			if err != nil {
				lastErr = fmt.Errorf("download successful but compression failed: %w", err)
				fmt.Printf("Compression failed for job %s: %v\n", job.ID, err)
				// Clean up the uncompressed file
				os.Remove(finalPath)
				if attempt < dm.config.Download.RetryAttempts {
					time.Sleep(dm.config.GetRetryDelay())
					continue
				}
			} else {
				res.Success = true
				res.FilePath = compressedPath // Return the compressed file path
				if job.Redownload {
					fmt.Printf("✅ Successfully redownloaded job %s with updated XML\n", job.ID)
				}
				break
			}
		}
	}
	if !res.Success {
		res.Error = lastErr
		// Clean up temp file if it exists
		if _, err := os.Stat(tempPath); err == nil {
			os.Remove(tempPath)
		}
	}
	res.EndTime = time.Now()
	return res
}

func (dm *DownloadManager) GetDownloadStats() (int, int64, error) {
	// Count files from all job folders
	folders := []string{
		dm.config.Folders.Files.Jobs.ToDo,
		dm.config.Folders.Files.Jobs.InProgress,
		dm.config.Folders.Files.Jobs.Done,
		dm.config.Folders.Files.Jobs.Error,
	}

	cnt := 0
	var total int64

	for _, folder := range folders {
		entries, err := os.ReadDir(folder)
		if err != nil {
			continue // Skip folders that don't exist
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			info, err := os.Stat(filepath.Join(folder, e.Name()))
			if err != nil {
				continue
			}
			cnt++
			total += info.Size()
		}
	}
	return cnt, total, nil
}

func FormatFileSize(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

// FileManager provides utilities for managing files in the new folder structure
type FileManager struct {
	config *Config
}

func NewFileManager(cfg *Config) *FileManager {
	return &FileManager{config: cfg}
}

// MoveJobFile moves a job file from one status folder to another
func (fm *FileManager) MoveJobFile(fileName, fromStatus, toStatus string) error {
	var fromFolder, toFolder string

	switch fromStatus {
	case "to_do":
		fromFolder = fm.config.Folders.Files.Jobs.ToDo
	case "in_progress":
		fromFolder = fm.config.Folders.Files.Jobs.InProgress
	case "done":
		fromFolder = fm.config.Folders.Files.Jobs.Done
	case "error":
		fromFolder = fm.config.Folders.Files.Jobs.Error
	default:
		return fmt.Errorf("invalid from status: %s", fromStatus)
	}

	switch toStatus {
	case "to_do":
		toFolder = fm.config.Folders.Files.Jobs.ToDo
	case "in_progress":
		toFolder = fm.config.Folders.Files.Jobs.InProgress
	case "done":
		toFolder = fm.config.Folders.Files.Jobs.Done
	case "error":
		toFolder = fm.config.Folders.Files.Jobs.Error
	default:
		return fmt.Errorf("invalid to status: %s", toStatus)
	}

	fromPath := filepath.Join(fromFolder, fileName)
	toPath := filepath.Join(toFolder, fileName)

	if err := os.Rename(fromPath, toPath); err != nil {
		return fmt.Errorf("failed to move file %s: %w", fileName, err)
	}

	return nil
}

// GetJobFiles returns a list of job files in a specific status folder
func (fm *FileManager) GetJobFiles(status string) ([]string, error) {
	var folder string

	switch status {
	case "to_do":
		folder = fm.config.Folders.Files.Jobs.ToDo
	case "in_progress":
		folder = fm.config.Folders.Files.Jobs.InProgress
	case "done":
		folder = fm.config.Folders.Files.Jobs.Done
	case "error":
		folder = fm.config.Folders.Files.Jobs.Error
	default:
		return nil, fmt.Errorf("invalid status: %s", status)
	}

	entries, err := os.ReadDir(folder)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory %s: %w", folder, err)
	}

	var files []string
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".job" {
			files = append(files, entry.Name())
		}
	}

	return files, nil
}

// GetCSVFiles returns a list of CSV files in the Files/Results/To Do folder
func (fm *FileManager) GetCSVFiles() ([]string, error) {
	entries, err := os.ReadDir(fm.config.Folders.Files.Results.ToDo)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory %s: %w", fm.config.Folders.Files.Results.ToDo, err)
	}

	var files []string
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".csv" {
			files = append(files, entry.Name())
		}
	}

	return files, nil
}

// MoveCSVFile moves a CSV file from To Do to Done folder
func (fm *FileManager) MoveCSVFile(fileName string) error {
	fromPath := filepath.Join(fm.config.Folders.Files.Results.ToDo, fileName)
	toPath := filepath.Join(fm.config.Folders.Files.Results.Done, fileName)

	if err := os.Rename(fromPath, toPath); err != nil {
		return fmt.Errorf("failed to move CSV file %s: %w", fileName, err)
	}

	return nil
}

// GetOptFiles returns a list of .opt files in the Opt/In folder
func (fm *FileManager) GetOptFiles() ([]string, error) {
	entries, err := os.ReadDir(fm.config.Folders.Files.Opt.In)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory %s: %w", fm.config.Folders.Files.Opt.In, err)
	}

	var files []string
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".opt" {
			files = append(files, entry.Name())
		}
	}

	return files, nil
}

// GetDailySummaryFiles returns a list of .rep files in the Opt/Summary folder
func (fm *FileManager) GetDailySummaryFiles() ([]string, error) {
	entries, err := os.ReadDir(fm.config.Folders.Files.Opt.Summary)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory %s: %w", fm.config.Folders.Files.Opt.Summary, err)
	}

	var files []string
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".rep" {
			files = append(files, entry.Name())
		}
	}

	return files, nil
}

// MoveDailySummaryFile moves a daily summary JSON file from Opt/Summary to Opt/Done
func (fm *FileManager) MoveDailySummaryFile(fileName string) error {
	fromPath := filepath.Join(fm.config.Folders.Files.Opt.Summary, fileName)
	toPath := filepath.Join(fm.config.Folders.Files.Opt.Done, fileName)

	if err := os.Rename(fromPath, toPath); err != nil {
		return fmt.Errorf("failed to move daily summary file %s: %w", fileName, err)
	}

	return nil
}

// MoveDailySummaryFileToError moves a daily summary file from Opt/Summary to Opt/Error
func (fm *FileManager) MoveDailySummaryFileToError(fileName string) error {
	fromPath := filepath.Join(fm.config.Folders.Files.Opt.Summary, fileName)
	toPath := filepath.Join(fm.config.Folders.Files.Opt.Error, fileName)

	if err := os.Rename(fromPath, toPath); err != nil {
		return fmt.Errorf("failed to move daily summary file to error folder %s: %w", fileName, err)
	}

	return nil
}

// MoveOptFile moves an .opt file from Opt/In to Opt/Done
func (fm *FileManager) MoveOptFile(fileName string) error {
	fromPath := filepath.Join(fm.config.Folders.Files.Opt.In, fileName)
	toPath := filepath.Join(fm.config.Folders.Files.Opt.Done, fileName)

	if err := os.Rename(fromPath, toPath); err != nil {
		return fmt.Errorf("failed to move OPT file %s: %w", fileName, err)
	}

	return nil
}

// FindOptFileByJobID searches Opt/In and Opt/Done for an .opt file for the given job id
// Returns the absolute file path and the folder name ("in" or "done"). If not found, returns empty strings.
func (fm *FileManager) FindOptFileByJobID(jobID string) (string, string) {
	// Search In folder first
	prefix := jobID + "_"
	inDir := fm.config.Folders.Files.Opt.In
	entries, _ := os.ReadDir(inDir)
	for _, e := range entries {
		if e.IsDir() { continue }
		name := e.Name()
		if strings.HasPrefix(name, prefix) && filepath.Ext(name) == ".opt" {
			return filepath.Join(inDir, name), "in"
		}
	}
	// Then Done folder
	doneDir := fm.config.Folders.Files.Opt.Done
	entries, _ = os.ReadDir(doneDir)
	for _, e := range entries {
		if e.IsDir() { continue }
		name := e.Name()
		if strings.HasPrefix(name, prefix) && filepath.Ext(name) == ".opt" {
			return filepath.Join(doneDir, name), "done"
		}
	}
	return "", ""
}

// DecompressJobFile decompresses a .job file to XML format
func (fm *FileManager) DecompressJobFile(jobFileName, status string) (string, error) {
	var folder string

	switch status {
	case "to_do":
		folder = fm.config.Folders.Files.Jobs.ToDo
	case "in_progress":
		folder = fm.config.Folders.Files.Jobs.InProgress
	case "done":
		folder = fm.config.Folders.Files.Jobs.Done
	case "error":
		folder = fm.config.Folders.Files.Jobs.Error
	default:
		return "", fmt.Errorf("invalid status: %s", status)
	}

	jobPath := filepath.Join(folder, jobFileName)
	xmlPath, err := DecompressJobFile(jobPath)
	if err != nil {
		return "", fmt.Errorf("failed to decompress job file %s: %w", jobFileName, err)
	}

	return xmlPath, nil
}

// CompressJobFile compresses an XML file to .job format
func (fm *FileManager) CompressJobFile(xmlFileName, status string, deleteOriginal bool) (string, error) {
	var folder string

	switch status {
	case "to_do":
		folder = fm.config.Folders.Files.Jobs.ToDo
	case "in_progress":
		folder = fm.config.Folders.Files.Jobs.InProgress
	case "done":
		folder = fm.config.Folders.Files.Jobs.Done
	case "error":
		folder = fm.config.Folders.Files.Jobs.Error
	default:
		return "", fmt.Errorf("invalid status: %s", status)
	}

	xmlPath := filepath.Join(folder, xmlFileName)
	jobPath, err := CompressXMLFile(xmlPath, deleteOriginal)
	if err != nil {
		return "", fmt.Errorf("failed to compress XML file %s: %w", xmlFileName, err)
	}

	return jobPath, nil
}

// CheckAndFixXMLFile checks if an XML file has empty data streams and regenerates it if needed
func (dm *DownloadManager) CheckAndFixXMLFile(filePath string, jobID string) error {
	// Read the XML file
	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read XML file: %w", err)
	}

	xmlContent := string(content)

	// Check for empty data streams issues
	hasEmptyItemTags := strings.Contains(xmlContent, "<item></item>") ||
		strings.Contains(xmlContent, "<item>\n  </item>") ||
		strings.Contains(xmlContent, "<item>\n    </item>")

	hasSelfClosingEmptyTags := strings.Contains(xmlContent, "<market/>") ||
		strings.Contains(xmlContent, "<timeframe/>")

	// If no issues found, return early
	if !hasEmptyItemTags && !hasSelfClosingEmptyTags {
		return nil
	}

	// Log the issue
	fmt.Printf("XML file %s has empty data streams, regenerating...\n", filePath)

	// Force regenerate the XML
	if err := dm.api.ForceRegenerateXML(jobID); err != nil {
		return fmt.Errorf("failed to force regenerate XML: %w", err)
	}

	// Download the regenerated XML
	// Get the job details to get the XML URL
	resp, err := dm.api.PollJobs(1) // This will get the job with regenerated XML
	if err != nil {
		return fmt.Errorf("failed to poll jobs after regeneration: %w", err)
	}

	// Find the specific job
	var targetJob *Job
	for _, job := range resp.Jobs {
		if job.ID == jobID {
			targetJob = &job
			break
		}
	}

	if targetJob == nil {
		return fmt.Errorf("job %s not found after regeneration", jobID)
	}

	// Download the regenerated XML
	if err := dm.api.DownloadFile(targetJob.XMLURL, filePath); err != nil {
		return fmt.Errorf("failed to download regenerated XML: %w", err)
	}

	fmt.Printf("Successfully regenerated XML file %s\n", filePath)
	return nil
}
