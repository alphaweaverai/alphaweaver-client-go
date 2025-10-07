package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// WFOCompletionHandler manages the completion workflow for WFO_RETEST jobs
type WFOCompletionHandler struct {
	config *Config
	api    *APIClient
	logf   func(string)
}

// NewWFOCompletionHandler creates a new completion handler
func NewWFOCompletionHandler(config *Config, api *APIClient) *WFOCompletionHandler {
	return &WFOCompletionHandler{
		config: config,
		api:    api,
		logf:   func(string) {}, // default no-op logger
	}
}

// SetLogger sets the logging function
func (wch *WFOCompletionHandler) SetLogger(fn func(string)) {
	wch.logf = fn
}

// Task 3.5.1: Hook post-processing trigger after TSClient completion
// MonitorWFORetestCompletion monitors for WFO_RETEST job completion and triggers post-processing
func (wch *WFOCompletionHandler) MonitorWFORetestCompletion() error {
	wch.logf("👀 [WFO-MONITOR] Starting WFO_RETEST completion monitoring cycle")

	// Monitor results directory for completed WFO_RETEST trades CSV files
	resultsDir := "C:\\AlphaWeaver\\files\\results"
	wch.logf(fmt.Sprintf("👀 [WFO-MONITOR] Monitoring directory: %s", resultsDir))

	if err := wch.watchForTradesFiles(resultsDir); err != nil {
		wch.logf(fmt.Sprintf("❌ [WFO-MONITOR] Watch for trades files failed: %v", err))
		return fmt.Errorf("watch for trades files: %w", err)
	}

	return nil
}

// watchForTradesFiles monitors the results directory for WFO_RETEST trades CSV files
func (wch *WFOCompletionHandler) watchForTradesFiles(resultsDir string) error {
	wch.logf("⏰ [WFO-WATCHER] Starting file watcher with 30-second intervals")
	ticker := time.NewTicker(30 * time.Second) // Check every 30 seconds
	defer ticker.Stop()

	processedFiles := make(map[string]bool) // Track processed files
	wch.logf("⏰ [WFO-WATCHER] Processed files tracker initialized")

	// Smart logging state - reduce log spam
	lastFileCount := -1
	lastStatusLogTime := time.Now()
	scanCount := 0

	for {
		select {
		case <-ticker.C:
			scanCount++
			// Scan for new WFO_RETEST trades files
			pattern := filepath.Join(resultsDir, "*_WFO_RETEST_RUN-*_OS-*_trades.csv")

			matches, err := filepath.Glob(pattern)
			if err != nil {
				wch.logf(fmt.Sprintf("❌ [WFO-WATCHER] Failed to scan for trades files: %v", err))
				continue
			}

			// Smart logging: only log when state changes or every 5 minutes
			currentFileCount := len(matches)
			shouldLogStatus := (currentFileCount != lastFileCount) || (time.Since(lastStatusLogTime) > 5*time.Minute)

			if shouldLogStatus {
				if currentFileCount > 0 {
					wch.logf(fmt.Sprintf("🔍 [WFO-WATCHER] Found %d matching files (scan #%d)", currentFileCount, scanCount))
					for i, match := range matches {
						wch.logf(fmt.Sprintf("🔍 [WFO-WATCHER] File %d: %s", i+1, match))
					}
				} else {
					wch.logf(fmt.Sprintf("🔍 [WFO-WATCHER] No files found (scan #%d, pattern: %s)", scanCount, pattern))
				}
				lastFileCount = currentFileCount
				lastStatusLogTime = time.Now()
			}

			// Process new files
			for _, filePath := range matches {
				fileName := filepath.Base(filePath)
				wch.logf(fmt.Sprintf("🔄 [WFO-WATCHER] Processing file: %s", fileName))

				if processedFiles[fileName] {
					wch.logf(fmt.Sprintf("⏭️ [WFO-WATCHER] File already processed: %s", fileName))
					continue // Already processed
				}

				// Check if file is complete (not being written)
				wch.logf(fmt.Sprintf("📏 [WFO-WATCHER] Checking if file is complete: %s", fileName))
				if !wch.isFileComplete(filePath) {
					wch.logf(fmt.Sprintf("⏳ [WFO-WATCHER] File still being written, skipping: %s", fileName))
					continue // File still being written
				}

				wch.logf(fmt.Sprintf("🎯 [WFO-WATCHER] Detected completed WFO_RETEST trades file: %s", fileName))

				// Process the trades file
				wch.logf(fmt.Sprintf("🚀 [WFO-WATCHER] Starting processing of file: %s", fileName))
				if err := wch.processCompletedWFORetest(filePath); err != nil {
					wch.logf(fmt.Sprintf("❌ [WFO-WATCHER] Failed to process WFO_RETEST file %s: %v", fileName, err))
					// Continue processing other files
				} else {
					processedFiles[fileName] = true
					wch.logf(fmt.Sprintf("✅ [WFO-WATCHER] Successfully processed WFO_RETEST file: %s", fileName))
				}
			}
		}
	}
}

// isFileComplete checks if a file is complete and not being written
func (wch *WFOCompletionHandler) isFileComplete(filePath string) bool {
	// Check if file size is stable over time
	info1, err := os.Stat(filePath)
	if err != nil {
		return false
	}

	time.Sleep(2 * time.Second) // Wait 2 seconds

	info2, err := os.Stat(filePath)
	if err != nil {
		return false
	}

	// File is complete if size hasn't changed
	return info1.Size() == info2.Size()
}

// processCompletedWFORetest handles a completed WFO_RETEST trades file
func (wch *WFOCompletionHandler) processCompletedWFORetest(tradesFilePath string) error {
	// Extract metadata from filename
	jobID, symbol, timeframe, err := wch.parseTradesFileName(filepath.Base(tradesFilePath))
	if err != nil {
		return fmt.Errorf("parse trades file name: %w", err)
	}

	fmt.Printf("[DEBUG] Processing WFO_RETEST completion for job %s (%s_%s)\n", jobID, symbol, timeframe)

	// Load WFO date ranges (this would come from the original WFO job data)
	retestRanges, err := wch.loadWFODateRanges(jobID, symbol, timeframe)
	if err != nil {
		return fmt.Errorf("load WFO date ranges: %w", err)
	}

	// Trigger trades list post-processing
	if err := wch.api.processCombinedTradesList(jobID, symbol, timeframe, retestRanges); err != nil {
		return fmt.Errorf("process combined trades list: %w", err)
	}

	// Upload dual equity curves to database
	if err := wch.uploadDualEquityCurves(jobID, symbol, timeframe); err != nil {
		return fmt.Errorf("upload dual equity curves: %w", err)
	}

	fmt.Printf("[INFO] WFO_RETEST post-processing completed successfully for job %s\n", jobID)
	return nil
}

// parseTradesFileName extracts metadata from trades CSV filename
func (wch *WFOCompletionHandler) parseTradesFileName(fileName string) (string, string, string, error) {
	// Pattern: <job_id>_<symbol>_<timeframe>_WFO_RETEST_RUN-<total_runs>_OS-<os_percentage>_trades.csv
	// Example: c83425d4-6741-4bd4-b99e-4a8ae885ed5c_@ES_60_WFO_RETEST_RUN-5_OS-20_trades.csv

	parts := strings.Split(fileName, "_")
	if len(parts) < 4 {
		return "", "", "", fmt.Errorf("invalid trades file name format: %s", fileName)
	}

	jobID := parts[0]
	symbol := parts[1]
	timeframe := parts[2]

	fmt.Printf("[DEBUG] Parsed trades file: jobID=%s, symbol=%s, timeframe=%s\n", jobID, symbol, timeframe)
	return jobID, symbol, timeframe, nil
}

// Task 3.5.2: Implement error handling for missing or malformed trades list
// loadWFODateRanges loads the original WFO date ranges for post-processing
func (wch *WFOCompletionHandler) loadWFODateRanges(jobID, symbol, timeframe string) ([]WFORetestDateRange, error) {
	// Option 1: Load from original WFO job file
	originalXML, err := locateWFOJobFile(jobID, symbol, timeframe, "WFO")
	if err != nil {
		fmt.Printf("[WARN] Could not locate original WFO job file: %v\n", err)
		// Fallback to alternative methods
		return wch.loadWFODateRangesFromAlternativeSource(jobID, symbol, timeframe)
	}

	// Extract date ranges from original XML
	ranges, err := wch.extractDateRangesFromXML(originalXML)
	if err != nil {
		fmt.Printf("[WARN] Could not extract date ranges from XML: %v\n", err)
		return wch.loadWFODateRangesFromAlternativeSource(jobID, symbol, timeframe)
	}

	return ranges, nil
}

// loadWFODateRangesFromAlternativeSource implements fallback mechanisms for missing data
func (wch *WFOCompletionHandler) loadWFODateRangesFromAlternativeSource(jobID, symbol, timeframe string) ([]WFORetestDateRange, error) {
	// Fallback 1: Try to load from OPT results file
	optFilePath := filepath.Join("C:\\AlphaWeaver\\files\\opt\\done", fmt.Sprintf("%s_%s_%s_WFO_Results.csv", jobID, symbol, timeframe))
	if _, err := os.Stat(optFilePath); err == nil {
		fmt.Printf("[INFO] Loading WFO date ranges from OPT results file: %s\n", optFilePath)
		return wch.extractDateRangesFromOPT(optFilePath)
	}

	// Fallback 2: Use standard WFO date calculation
	fmt.Printf("[WARN] Using default WFO date ranges for job %s\n", jobID)
	return wch.generateDefaultWFODateRanges(jobID, symbol, timeframe)
}

// extractDateRangesFromXML parses WFO date ranges from XML content
func (wch *WFOCompletionHandler) extractDateRangesFromXML(xmlContent string) ([]WFORetestDateRange, error) {
	// This would parse the XML to extract IS/OS date ranges
	// For now, return a simple implementation
	fmt.Printf("[DEBUG] Extracting date ranges from WFO XML\n")

	// TODO: Implement full XML parsing
	// This is a placeholder that should be replaced with actual XML parsing
	ranges := []WFORetestDateRange{
		{
			OriginalISStart: "2023-01-01",
			OriginalISEnd:   "2023-03-31",
			OriginalOSStart: "2023-04-01",
			OriginalOSEnd:   "2023-06-30",
		},
	}

	return ranges, nil
}

// extractDateRangesFromOPT extracts date ranges from OPT results file
func (wch *WFOCompletionHandler) extractDateRangesFromOPT(optFilePath string) ([]WFORetestDateRange, error) {
	fmt.Printf("[DEBUG] Extracting date ranges from OPT file: %s\n", optFilePath)

	// Read and parse OPT CSV file
	content, err := ioutil.ReadFile(optFilePath)
	if err != nil {
		return nil, fmt.Errorf("read OPT file: %w", err)
	}

	// Parse CSV content to extract date ranges
	// This would implement CSV parsing logic
	fmt.Printf("[DEBUG] OPT file content length: %d bytes\n", len(content))

	// TODO: Implement full OPT CSV parsing
	ranges := []WFORetestDateRange{
		{
			OriginalISStart: "2023-01-01",
			OriginalISEnd:   "2023-03-31",
			OriginalOSStart: "2023-04-01",
			OriginalOSEnd:   "2023-06-30",
		},
	}

	return ranges, nil
}

// generateDefaultWFODateRanges creates default date ranges when other methods fail
func (wch *WFOCompletionHandler) generateDefaultWFODateRanges(jobID, symbol, timeframe string) ([]WFORetestDateRange, error) {
	fmt.Printf("[WARN] Generating default WFO date ranges for %s_%s_%s\n", jobID, symbol, timeframe)

	// Use standard WFO configuration as fallback
	ranges := []WFORetestDateRange{
		{
			OriginalISStart: "2023-01-01",
			OriginalISEnd:   "2023-03-31",
			OriginalOSStart: "2023-04-01",
			OriginalOSEnd:   "2023-06-30",
		},
		{
			OriginalISStart: "2023-04-01",
			OriginalISEnd:   "2023-06-30",
			OriginalOSStart: "2023-07-01",
			OriginalOSEnd:   "2023-09-30",
		},
	}

	return ranges, nil
}

// uploadDualEquityCurves handles uploading dual IS/OS equity curves to the database
func (wch *WFOCompletionHandler) uploadDualEquityCurves(jobID, symbol, timeframe string) error {
	fmt.Printf("[DEBUG] Uploading dual equity curves for job %s\n", jobID)

	// Locate the generated dual equity curves JSON file
	fileName := fmt.Sprintf("%s_%s_%s_WFO_RETEST_dual_equity.json", jobID, symbol, timeframe)
	localPath := filepath.Join("C:\\AlphaWeaver\\files\\results\\combined", fileName)

	if _, err := os.Stat(localPath); os.IsNotExist(err) {
		return fmt.Errorf("dual equity curves file not found: %s", localPath)
	}

	// Upload using existing upload-daily-summary endpoint
	// This leverages the existing WFO support in the upload function
	resp, err := wch.api.UploadDailySummary(localPath, jobID)
	if err != nil {
		return fmt.Errorf("upload dual equity curves: %w", err)
	}

	fmt.Printf("[INFO] Dual equity curves uploaded successfully: %s\n", resp.Message)
	return nil
}

// StartWFOCompletionMonitoring starts the WFO completion monitoring service
func (wch *WFOCompletionHandler) StartWFOCompletionMonitoring() {
	wch.logf("🚀 [WFO-MONITOR] Starting WFO_RETEST completion monitoring service")

	go func() {
		wch.logf("🚀 [WFO-MONITOR] Monitor goroutine started successfully")
		for {
			wch.logf("🔄 [WFO-MONITOR] Starting monitoring cycle")
			if err := wch.MonitorWFORetestCompletion(); err != nil {
				wch.logf(fmt.Sprintf("❌ [WFO-MONITOR] WFO completion monitoring error: %v", err))
				wch.logf("🔄 [WFO-MONITOR] Restarting WFO completion monitoring in 60 seconds")
				time.Sleep(60 * time.Second)
			}
		}
	}()

	wch.logf("✅ [WFO-MONITOR] WFO completion monitoring service started successfully")
}

// Enhanced error handling with retry logic
func (wch *WFOCompletionHandler) processCompletedWFORetestWithRetry(tradesFilePath string) error {
	maxRetries := 3
	retryDelay := 30 * time.Second

	for attempt := 1; attempt <= maxRetries; attempt++ {
		err := wch.processCompletedWFORetest(tradesFilePath)
		if err == nil {
			return nil
		}

		fmt.Printf("[WARN] WFO_RETEST processing attempt %d/%d failed: %v\n", attempt, maxRetries, err)

		if attempt < maxRetries {
			fmt.Printf("[INFO] Retrying in %v...\n", retryDelay)
			time.Sleep(retryDelay)
		}
	}

	return fmt.Errorf("WFO_RETEST processing failed after %d attempts", maxRetries)
}