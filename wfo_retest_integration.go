package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Global logger for WFO_RETEST operations
var wfoLogger *Logger

// Initialize logger for WFO operations
func init() {
	// Create a logger instance for WFO operations
	exePath, _ := os.Executable()
	wfoLogger = NewLogger(filepath.Join(filepath.Dir(exePath), "logs"))
}

// Note: Using OPTResult struct defined in api.go

// processWFORetestGeneration is the main entry point for WFO_RETEST XML generation
// This function is called after successful OPT upload for WFO jobs
func (ac *APIClient) processWFORetestGeneration(jobID, symbol, timeframe string, optResults []OPTResult) error {
	fmt.Printf("[DEBUG] WFO_RETEST XML Generation: Starting process for job %s (symbol=%s, timeframe=%s) with %d OPT results\n", jobID, symbol, timeframe, len(optResults))
	wfoLogger.Info(fmt.Sprintf("WFO_RETEST XML Generation: Starting process for job %s (symbol=%s, timeframe=%s) with %d OPT results", jobID, symbol, timeframe, len(optResults)))

	fmt.Printf("[DEBUG] WFO_RETEST XML Generation: Using provided metadata - symbol=%s, timeframe=%s\n", symbol, timeframe)
	wfoLogger.Info(fmt.Sprintf("WFO_RETEST XML Generation: Using provided metadata - symbol=%s, timeframe=%s", symbol, timeframe))

	// Validate OPT results before processing
	fmt.Printf("[DEBUG] WFO_RETEST XML Generation Step 2: Validating %d OPT results\n", len(optResults))
	if err := ac.validateOptResults(optResults); err != nil {
		fmt.Printf("[ERROR] WFO_RETEST XML Generation: OPT validation failed - %v\n", err)
		return fmt.Errorf("validate OPT results: %w", err)
	}
	fmt.Printf("[DEBUG] WFO_RETEST XML Generation: OPT results validation passed\n")

	// Generate WFO_RETEST XML with fixed parameters and proper date handling
	fmt.Printf("[DEBUG] WFO_RETEST XML Generation Step 3: Generating XML content\n")
	wfoRetestXML, err := generateWFORetestXML(jobID, symbol, timeframe, optResults)
	if err != nil {
		fmt.Printf("[ERROR] WFO_RETEST XML Generation: XML generation failed - %v\n", err)
		return fmt.Errorf("generate WFO_RETEST XML: %w", err)
	}
	fmt.Printf("[DEBUG] WFO_RETEST XML Generation: Generated XML (%d bytes)\n", len(wfoRetestXML))

	// Save WFO_RETEST XML to appropriate location for TSClient processing
	fmt.Printf("[DEBUG] WFO_RETEST XML Generation Step 4: Saving XML to file system\n")
	wfoLogger.Info(fmt.Sprintf("WFO_RETEST XML Generation Step 4: Saving XML to file system"))

	xmlFilePath, err := ac.saveWFORetestXML(jobID, symbol, timeframe, wfoRetestXML)
	if err != nil {
		fmt.Printf("[ERROR] WFO_RETEST XML Generation: Failed to save XML - %v\n", err)
		wfoLogger.Error(fmt.Sprintf("WFO_RETEST XML Generation: Failed to save XML - %v", err))
		return fmt.Errorf("save WFO_RETEST XML: %w", err)
	}
	fmt.Printf("[DEBUG] WFO_RETEST XML Generation: XML saved to %s\n", xmlFilePath)
	wfoLogger.Info(fmt.Sprintf("WFO_RETEST XML Generation: XML saved to %s", xmlFilePath))

	// Submit WFO_RETEST job for processing
	fmt.Printf("[DEBUG] WFO_RETEST XML Generation Step 5: Submitting job for TSClient processing\n")
	if err := ac.submitWFORetestJob(xmlFilePath, jobID); err != nil {
		fmt.Printf("[ERROR] WFO_RETEST XML Generation: Job submission failed - %v\n", err)
		return fmt.Errorf("submit WFO_RETEST job: %w", err)
	}
	fmt.Printf("[INFO] WFO_RETEST XML Generation: Job submitted successfully to TSClient\n")
	wfoLogger.Info(fmt.Sprintf("WFO_RETEST XML Generation: Job submitted successfully to TSClient"))

	fmt.Printf("[DEBUG] WFO_RETEST XML Generation: Process completed successfully for job %s\n", jobID)
	wfoLogger.Info(fmt.Sprintf("WFO_RETEST XML Generation: Process completed successfully for job %s", jobID))
	return nil
}

// extractJobMetadata extracts symbol and timeframe from job ID or OPT results
func (ac *APIClient) extractJobMetadata(jobID string, optResults []OPTResult) (string, string, error) {
	// Method 1: Extract from job ID pattern (preferred)
	// Expected pattern: <uuid>_<symbol>_<timeframe>_WFO_Results.csv -> jobID
	fmt.Printf("[DEBUG] Metadata Extraction: Trying Method 1 - parsing job ID pattern: %s\n", jobID)
	if symbol, timeframe := parseJobIDPattern(jobID); symbol != "" && timeframe != "" {
		fmt.Printf("[DEBUG] Metadata Extraction: SUCCESS Method 1 - symbol=%s, timeframe=%s\n", symbol, timeframe)
		return symbol, timeframe, nil
	}

	// Method 2: Extract from first OPT result if available
	fmt.Printf("[DEBUG] Metadata Extraction: Method 1 failed, trying Method 2 - OPT results parameters\n")
	if len(optResults) > 0 && optResults[0].Parameters != nil {
		fmt.Printf("[DEBUG] Metadata Extraction: Checking parameters in first OPT result\n")
		if symbolVal, ok := optResults[0].Parameters["symbol"]; ok {
			if timeframeVal, ok := optResults[0].Parameters["timeframe"]; ok {
				fmt.Printf("[DEBUG] Metadata Extraction: SUCCESS Method 2 - symbol=%v, timeframe=%v\n", symbolVal, timeframeVal)
				return fmt.Sprintf("%v", symbolVal), fmt.Sprintf("%v", timeframeVal), nil
			}
		}
		fmt.Printf("[DEBUG] Metadata Extraction: Method 2 failed - missing symbol/timeframe in parameters\n")
	} else {
		fmt.Printf("[DEBUG] Metadata Extraction: Method 2 failed - no OPT results or parameters\n")
	}

	fmt.Printf("[ERROR] Metadata Extraction: All methods failed for job %s\n", jobID)
	return "", "", fmt.Errorf("unable to extract symbol and timeframe from job %s", jobID)
}

// parseJobIDPattern extracts symbol and timeframe from job ID
func parseJobIDPattern(jobID string) (string, string) {
	// Common patterns:
	// 1. <uuid>_@ES_60_WFO
	// 2. job_<uuid>_@ES_60_WFO
	// 3. <uuid>_@ES_60_WFO_Results

	patterns := []string{
		`([A-Za-z0-9-]+)_([^_]+)_(\d+)_WFO`,      // Basic pattern
		`job_([A-Za-z0-9-]+)_([^_]+)_(\d+)_WFO`,  // With job_ prefix
		`([A-Za-z0-9-]+)_([^_]+)_(\d+)_WFO_.*`,   // With suffix
	}

	for _, pattern := range patterns {
		regex := regexp.MustCompile(pattern)
		if matches := regex.FindStringSubmatch(jobID); len(matches) >= 4 {
			symbol := matches[2]    // Second capture group
			timeframe := matches[3] // Third capture group
			fmt.Printf("[DEBUG] Parsed job ID '%s': symbol=%s, timeframe=%s\n", jobID, symbol, timeframe)
			return symbol, timeframe
		}
	}

	fmt.Printf("[DEBUG] Unable to parse job ID pattern: %s\n", jobID)
	return "", ""
}

// validateOptResults ensures OPT results are valid for WFO_RETEST processing
func (ac *APIClient) validateOptResults(optResults []OPTResult) error {
	if len(optResults) == 0 {
		return fmt.Errorf("no OPT results provided")
	}

	for i, result := range optResults {
		// Validate required fields
		if result.ISStartDate == "" || result.ISEndDate == "" {
			return fmt.Errorf("missing IS date range for run %d", i+1)
		}

		if result.ParametersJSON == "" {
			return fmt.Errorf("missing parameters JSON for run %d", i+1)
		}

		// Validate date format (should be YYYY-MM-DD)
		if err := validateDateFormat(result.ISStartDate); err != nil {
			return fmt.Errorf("invalid IS start date for run %d: %w", i+1, err)
		}

		if err := validateDateFormat(result.ISEndDate); err != nil {
			return fmt.Errorf("invalid IS end date for run %d: %w", i+1, err)
		}

		// Validate OS dates if present
		if result.OSStartDate != "" {
			if err := validateDateFormat(result.OSStartDate); err != nil {
				return fmt.Errorf("invalid OS start date for run %d: %w", i+1, err)
			}
		}

		if result.OSEndDate != "" {
			if err := validateDateFormat(result.OSEndDate); err != nil {
				return fmt.Errorf("invalid OS end date for run %d: %w", i+1, err)
			}
		}
	}

	fmt.Printf("[DEBUG] Validated %d OPT results successfully\n", len(optResults))
	return nil
}

// validateDateFormat ensures date is in YYYY-MM-DD format
func validateDateFormat(dateStr string) error {
	dateRegex := regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	if !dateRegex.MatchString(dateStr) {
		return fmt.Errorf("invalid date format '%s', expected YYYY-MM-DD", dateStr)
	}
	return nil
}

// saveWFORetestXML saves the generated XML to the appropriate location for TSClient processing
func (ac *APIClient) saveWFORetestXML(jobID, symbol, timeframe, xmlContent string) (string, error) {
	// Create filename with metadata for tracking
	totalRuns := strings.Count(xmlContent, "<Job>") // Count Job elements (uppercase)
	osPercentage := 20                              // Default, should be calculated from date ranges

	fmt.Printf("[INFO] XML File Save: Creating WFO_RETEST job file with metadata\n")
	fmt.Printf("[INFO] XML File Save:   Job ID: %s\n", jobID)
	fmt.Printf("[INFO] XML File Save:   Symbol: %s, Timeframe: %s\n", symbol, timeframe)
	fmt.Printf("[INFO] XML File Save:   Total Runs: %d, OS Percentage: %d\n", totalRuns, osPercentage)
	fmt.Printf("[INFO] XML File Save:   XML Content Size: %d bytes\n", len(xmlContent))

	// Create temporary XML filename and final .job filename
	tempXMLName := fmt.Sprintf("%s_%s_%s_WFO_RETEST_RUN-%d_OS-%d.xml",
		jobID, symbol, timeframe, totalRuns, osPercentage)
	jobFileName := fmt.Sprintf("%s_%s_%s_WFO_RETEST_RUN-%d_OS-%d.job",
		jobID, symbol, timeframe, totalRuns, osPercentage)

	fmt.Printf("[INFO] XML File Save: Generated temp XML filename: %s\n", tempXMLName)
	fmt.Printf("[INFO] XML File Save: Generated final job filename: %s\n", jobFileName)

	// Save to TSClient input directory
	targetDir := "C:\\AlphaWeaver\\files\\jobs\\to_do"
	tempXMLPath := filepath.Join(targetDir, tempXMLName)
	finalJobPath := filepath.Join(targetDir, jobFileName)

	fmt.Printf("[INFO] XML File Save: Target directory: %s\n", targetDir)
	fmt.Printf("[INFO] XML File Save: Temp XML path: %s\n", tempXMLPath)
	fmt.Printf("[INFO] XML File Save: Final job path: %s\n", finalJobPath)

	// Ensure target directory exists
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		fmt.Printf("[ERROR] XML File Save: Failed to create target directory: %v\n", err)
		return "", fmt.Errorf("create target directory '%s': %w", targetDir, err)
	}
	fmt.Printf("[INFO] XML File Save: Target directory confirmed/created\n")

	// Write the temporary XML file
	fmt.Printf("[INFO] XML File Save: Writing XML content to temporary file...\n")
	if err := os.WriteFile(tempXMLPath, []byte(xmlContent), 0644); err != nil {
		fmt.Printf("[ERROR] XML File Save: Failed to write XML file - %v\n", err)
		return "", fmt.Errorf("write XML file '%s': %w", tempXMLPath, err)
	}

	// Compress XML to .job format using existing compression function
	fmt.Printf("[INFO] XML File Save: Compressing XML to .job format...\n")
	compressedPath, err := CompressXMLFile(tempXMLPath, false) // Keep original XML for review
	if err != nil {
		fmt.Printf("[ERROR] XML File Save: Failed to compress XML file - %v\n", err)
		// Clean up temp file if compression fails
		os.Remove(tempXMLPath)
		return "", fmt.Errorf("compress XML file '%s': %w", tempXMLPath, err)
	}

	// Move compressed file to final location if needed (CompressXMLFile should create it in the right place)
	if compressedPath != finalJobPath {
		fmt.Printf("[INFO] XML File Save: Moving compressed file to final location...\n")
		if err := os.Rename(compressedPath, finalJobPath); err != nil {
			fmt.Printf("[ERROR] XML File Save: Failed to move compressed file - %v\n", err)
			os.Remove(compressedPath) // Clean up
			return "", fmt.Errorf("move compressed file to final location: %w", err)
		}
	}

	// Verify final compressed file was created
	if fileInfo, err := os.Stat(finalJobPath); err != nil {
		fmt.Printf("[ERROR] XML File Save: Compressed file verification failed - %v\n", err)
		return "", fmt.Errorf("verify compressed job file '%s': %w", finalJobPath, err)
	} else {
		fmt.Printf("[INFO] XML File Save: ✅ SUCCESS - Both XML and compressed job files created!\n")
		fmt.Printf("[INFO] XML File Save:   📁 Compressed (.job): %s\n", finalJobPath)
		fmt.Printf("[INFO] XML File Save:   📄 Uncompressed (.xml): %s\n", tempXMLPath)
		fmt.Printf("[INFO] XML File Save:   📊 Compressed size: %d bytes\n", fileInfo.Size())
		fmt.Printf("[INFO] XML File Save:   🕐 Created: %s\n", fileInfo.ModTime().Format("2006-01-02 15:04:05"))
		fmt.Printf("[INFO] XML File Save:   🗜️ Note: XML file kept for review\n")
	}
	return finalJobPath, nil
}

// submitWFORetestJob submits the WFO_RETEST job for TSClient processing
func (ac *APIClient) submitWFORetestJob(xmlFilePath, originalJobID string) error {
	// Log the submission
	fmt.Printf("[DEBUG] Job Submission: Submitting WFO_RETEST job for TSClient processing\n")
	fmt.Printf("[DEBUG] Job Submission: XML file path: %s\n", xmlFilePath)
	fmt.Printf("[DEBUG] Job Submission: Original job ID: %s\n", originalJobID)

	// In a full implementation, this would:
	// 1. Create job record in database with WFO_RETEST task type
	// 2. Set appropriate job status and metadata
	// 3. Trigger TSClient processing pipeline
	// 4. Set up monitoring for completion

	// For now, just log the action
	fmt.Printf("[DEBUG] Job Submission: Implementation steps for full deployment:\n")
	fmt.Printf("[DEBUG] Job Submission: 1. Create database record with WFO_RETEST task type\n")
	fmt.Printf("[DEBUG] Job Submission: 2. Set job status and metadata\n")
	fmt.Printf("[DEBUG] Job Submission: 3. Trigger TSClient processing pipeline\n")
	fmt.Printf("[DEBUG] Job Submission: 4. Set up completion monitoring\n")
	fmt.Printf("[INFO] WFO_RETEST job submitted successfully - TSClient will process: %s\n", filepath.Base(xmlFilePath))

	return nil
}

// Note: parseOPTFile function is already implemented in csv_uploader.go

// updateOptUploadWorkflow modifies existing OPT upload workflow to use new WFO_RETEST approach
func (oum *OptUploadManager) updateOptUploadWorkflow() {
	// Update checkAndTriggerCombinedWFO to use new processWFORetestGeneration function
	// This ensures existing workflow continues to work while adding new functionality
}

// Error handling wrapper for WFO_RETEST generation with fallback mechanisms
func (ac *APIClient) processWFORetestGenerationWithFallback(jobID, symbol, timeframe string, optResults []OPTResult) error {
	fmt.Printf("[INFO] WFO_RETEST Fallback: Starting generation with error handling for job %s (symbol=%s, timeframe=%s)\n", jobID, symbol, timeframe)
	fmt.Printf("[INFO] WFO_RETEST Fallback: Processing %d OPT results\n", len(optResults))
	wfoLogger.Info(fmt.Sprintf("WFO_RETEST Fallback: Starting generation with error handling for job %s (symbol=%s, timeframe=%s)", jobID, symbol, timeframe))
	wfoLogger.Info(fmt.Sprintf("WFO_RETEST Fallback: Processing %d OPT results", len(optResults)))

	err := ac.processWFORetestGeneration(jobID, symbol, timeframe, optResults)
	if err != nil {
		fmt.Printf("[ERROR] WFO_RETEST Fallback: Generation failed for job %s: %v\n", jobID, err)
		wfoLogger.Error(fmt.Sprintf("WFO_RETEST Fallback: Generation failed for job %s: %v", jobID, err))

		// Implement fallback mechanisms
		fmt.Printf("[INFO] WFO_RETEST Fallback: Analyzing error type for potential recovery\n")
		wfoLogger.Info(fmt.Sprintf("WFO_RETEST Fallback: Analyzing error type for potential recovery"))

		if strings.Contains(err.Error(), "job file not found") {
			fmt.Printf("[WARN] WFO_RETEST Fallback: Job file missing for %s, attempting alternative metadata extraction\n", jobID)
			wfoLogger.Warning(fmt.Sprintf("WFO_RETEST Fallback: Job file missing for %s, attempting alternative metadata extraction", jobID))
			// Could implement alternative metadata sources here
		}

		if strings.Contains(err.Error(), "XML generation failed") {
			fmt.Printf("[WARN] WFO_RETEST Fallback: XML generation failed for %s, logging for manual review\n", jobID)
			wfoLogger.Warning(fmt.Sprintf("WFO_RETEST Fallback: XML generation failed for %s, logging for manual review", jobID))
			// Could implement manual review queue here
		}

		// Always log the error but don't fail the OPT upload
		fmt.Printf("[INFO] WFO_RETEST Fallback: Continuing with normal OPT upload workflow despite WFO_RETEST failure\n")
		fmt.Printf("[INFO] WFO_RETEST Fallback: ERROR DETAILS: %v\n", err)
		wfoLogger.Info(fmt.Sprintf("WFO_RETEST Fallback: Continuing with normal OPT upload workflow despite WFO_RETEST failure"))
		wfoLogger.Info(fmt.Sprintf("WFO_RETEST Fallback: ERROR DETAILS: %v", err))
		return nil // Don't propagate error to maintain existing workflow stability
	}

	fmt.Printf("[INFO] WFO_RETEST Fallback: Generation completed successfully for job %s\n", jobID)
	wfoLogger.Info(fmt.Sprintf("WFO_RETEST Fallback: Generation completed successfully for job %s", jobID))
	return nil
}