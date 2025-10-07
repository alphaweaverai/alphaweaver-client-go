package main

import (
	"compress/zlib"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// decompressJobFile decompresses a zlib-compressed job file and returns XML content
func decompressJobFile(filePath string) (string, error) {
	fmt.Printf("[DEBUG] Job File Decompression: Starting zlib decompression of file: %s\n", filePath)
	wfoLogger.Info(fmt.Sprintf("Job File Decompression: Starting zlib decompression of file: %s", filePath))

	file, err := os.Open(filePath)
	if err != nil {
		fmt.Printf("[ERROR] Job File Decompression: Failed to open file: %v\n", err)
		wfoLogger.Error(fmt.Sprintf("Job File Decompression: Failed to open file: %v", err))
		return "", fmt.Errorf("open compressed job file: %w", err)
	}
	defer file.Close()

	reader, err := zlib.NewReader(file)
	if err != nil {
		fmt.Printf("[ERROR] Job File Decompression: Failed to create zlib reader: %v\n", err)
		wfoLogger.Error(fmt.Sprintf("Job File Decompression: Failed to create zlib reader: %v", err))
		return "", fmt.Errorf("create zlib reader for job file (file may not be zlib compressed): %w", err)
	}
	defer reader.Close()

	var xmlBuffer strings.Builder
	bytesRead, err := io.Copy(&xmlBuffer, reader)
	if err != nil {
		fmt.Printf("[ERROR] Job File Decompression: Failed to decompress data: %v\n", err)
		wfoLogger.Error(fmt.Sprintf("Job File Decompression: Failed to decompress data: %v", err))
		return "", fmt.Errorf("decompress job file data: %w", err)
	}

	xmlContent := xmlBuffer.String()
	fmt.Printf("[DEBUG] Job File Decompression: Successfully decompressed %d bytes\n", bytesRead)
	wfoLogger.Info(fmt.Sprintf("Job File Decompression: Successfully decompressed %d bytes", bytesRead))
	return xmlContent, nil
}

// generateWFORetestXML creates WFO_RETEST XML with fixed parameters and proper date handling
// This preserves original IS/OS date ranges for trade filtering while applying buffers for TSClient
func generateWFORetestXML(jobID, symbol, timeframe string, optResults []OPTResult) (string, error) {
	fmt.Printf("[DEBUG] XML Generation: Starting WFO_RETEST XML creation for job %s (%s_%s) with %d runs\n", jobID, symbol, timeframe, len(optResults))

	// Step 1: Locate original WFO job file
	fmt.Printf("[DEBUG] XML Generation Step 1: Locating original WFO job file for %s_%s_%s\n", jobID, symbol, timeframe)
	originalXML, err := locateWFOJobFile(jobID, symbol, timeframe, "WFO")
	if err != nil {
		fmt.Printf("[ERROR] XML Generation: Failed to locate WFO job file - %v\n", err)
		return "", fmt.Errorf("locate WFO job file: %w", err)
	}
	fmt.Printf("[DEBUG] XML Generation: Found original WFO job file (%d bytes)\n", len(originalXML))

	// Step 2: Calculate date ranges with buffers
	fmt.Printf("[DEBUG] XML Generation Step 2: Calculating date ranges with buffers for %d runs\n", len(optResults))
	retestRanges, err := calculateWFORetestDateRanges(optResults)
	if err != nil {
		fmt.Printf("[ERROR] XML Generation: Failed to calculate date ranges - %v\n", err)
		return "", fmt.Errorf("calculate WFO_RETEST date ranges: %w", err)
	}
	fmt.Printf("[DEBUG] XML Generation: Calculated %d retest date ranges\n", len(retestRanges))

	// Step 3: Validate date ranges
	fmt.Printf("[DEBUG] XML Generation Step 3: Validating calculated date ranges\n")
	if err := validateDateRanges(retestRanges); err != nil {
		fmt.Printf("[ERROR] XML Generation: Date range validation failed - %v\n", err)
		return "", fmt.Errorf("validate date ranges: %w", err)
	}
	fmt.Printf("[DEBUG] XML Generation: Date range validation passed\n")

	// Step 4: Extract OS percentage from original WFO XML (more accurate than calculating from date ranges)
	totalRuns := len(optResults)
	var osPercentage int

	oosPercentStr, err := extractXMLTagValue(originalXML, "oos_percent")
	if err != nil {
		fmt.Printf("[WARN] XML Generation: Could not extract oos_percent from original XML, falling back to calculation: %v\n", err)
		osPercentage = calculateOSPercentage(retestRanges)
		fmt.Printf("[DEBUG] XML Generation: Using calculated OS percentage: %d%%\n", osPercentage)
	} else {
		// Parse the extracted oos_percent and convert to integer
		oosPercentFloat, parseErr := strconv.ParseFloat(oosPercentStr, 64)
		if parseErr != nil {
			fmt.Printf("[WARN] XML Generation: Could not parse oos_percent '%s', falling back to calculation: %v\n", oosPercentStr, parseErr)
			osPercentage = calculateOSPercentage(retestRanges)
			fmt.Printf("[DEBUG] XML Generation: Using calculated OS percentage: %d%%\n", osPercentage)
		} else {
			osPercentage = int(oosPercentFloat)
			fmt.Printf("[DEBUG] XML Generation: Extracted OS percentage from original XML: %s -> %d%%\n", oosPercentStr, osPercentage)
		}
	}

	// Step 5: Generate WFO_RETEST XML for all runs with filename metadata
	fmt.Printf("[DEBUG] XML Generation Step 5: Building complete WFO_RETEST XML structure with filename metadata\n")
	wfoRetestXML, err := buildWFORetestXML(originalXML, optResults, retestRanges, jobID, symbol, timeframe, totalRuns, osPercentage)
	if err != nil {
		fmt.Printf("[ERROR] XML Generation: Failed to build WFO_RETEST XML - %v\n", err)
		return "", fmt.Errorf("build WFO_RETEST XML: %w", err)
	}
	fmt.Printf("[DEBUG] XML Generation: Built complete XML structure (%d bytes)\n", len(wfoRetestXML))

	fmt.Printf("[DEBUG] XML Generation Step 5: Generated WFO_RETEST XML with %d runs, %d%% OS periods\n", totalRuns, osPercentage)
	fmt.Printf("[INFO] XML Generation: WFO_RETEST XML creation completed successfully\n")

	return wfoRetestXML, nil
}

// locateWFOJobFile finds and reads the original WFO job XML file
func locateWFOJobFile(jobID, symbol, timeframe, taskType string) (string, error) {
	// Construct job file pattern: <job_id>_<symbol>_<timeframe>_<task_type>.job
	jobFileName := fmt.Sprintf("%s_%s_%s_%s.job", jobID, symbol, timeframe, taskType)
	jobFilePath := filepath.Join("C:\\AlphaWeaver\\files\\jobs\\Completed", jobFileName)

	fmt.Printf("[DEBUG] Job File Location: Constructed filename=%s\n", jobFileName)
	fmt.Printf("[DEBUG] Job File Location: Looking for WFO job file at: %s\n", jobFilePath)
	wfoLogger.Info(fmt.Sprintf("Job File Location: Constructed filename=%s", jobFileName))
	wfoLogger.Info(fmt.Sprintf("Job File Location: Looking for WFO job file at: %s", jobFilePath))

	// Check if file exists first
	if _, err := os.Stat(jobFilePath); os.IsNotExist(err) {
		fmt.Printf("[ERROR] Job File Location: Job file does not exist at path: %s\n", jobFilePath)
		wfoLogger.Error(fmt.Sprintf("Job File Location: Job file does not exist at path: %s", jobFilePath))
		return "", fmt.Errorf("job file not found: %s", jobFilePath)
	}
	fmt.Printf("[DEBUG] Job File Location: Job file exists at: %s\n", jobFilePath)
	wfoLogger.Info(fmt.Sprintf("Job File Location: Job file exists at: %s", jobFilePath))

	// Read and decompress job file content (job files are zlib compressed like OPT files)
	fmt.Printf("[DEBUG] Job File Location: Reading and decompressing job file content\n")
	xmlStr, err := decompressJobFile(jobFilePath)
	if err != nil {
		fmt.Printf("[ERROR] Job File Location: Failed to decompress job file - %v\n", err)
		wfoLogger.Error(fmt.Sprintf("Job File Location: Failed to decompress job file - %v", err))
		return "", fmt.Errorf("decompress job file '%s': %w", jobFilePath, err)
	}
	fmt.Printf("[DEBUG] Job File Location: Decompressed %d bytes from job file\n", len(xmlStr))
	wfoLogger.Info(fmt.Sprintf("Job File Location: Decompressed %d bytes from job file", len(xmlStr)))

	// Save decompressed content to debug file for examination
	debugDir := "C:\\AlphaWeaver\\debug"
	os.MkdirAll(debugDir, 0755)
	debugFile := filepath.Join(debugDir, fmt.Sprintf("job_decompressed_%s.xml", filepath.Base(jobFilePath)))
	err = os.WriteFile(debugFile, []byte(xmlStr), 0644)
	if err == nil {
		fmt.Printf("[DEBUG] Job File Location: Saved decompressed content to: %s\n", debugFile)
		wfoLogger.Info(fmt.Sprintf("Job File Location: Saved decompressed content to: %s", debugFile))
	}

	// Log first 500 characters of decompressed content for debugging
	if len(xmlStr) > 500 {
		fmt.Printf("[DEBUG] Job File Location: First 500 chars of XML: %s\n", xmlStr[:500])
		wfoLogger.Info(fmt.Sprintf("Job File Location: First 500 chars of XML: %s", xmlStr[:500]))
	} else {
		fmt.Printf("[DEBUG] Job File Location: Complete XML content: %s\n", xmlStr)
		wfoLogger.Info(fmt.Sprintf("Job File Location: Complete XML content: %s", xmlStr))
	}

	fmt.Printf("[DEBUG] Job File Location: Validating XML content structure\n")

	// Check for <Job tag (uppercase - based on actual XML structure)
	hasJobTag := strings.Contains(xmlStr, "<Job>") || strings.Contains(xmlStr, "<job")
	fmt.Printf("[DEBUG] Job File Location: Contains '<Job>' or '<job' tag: %t\n", hasJobTag)
	wfoLogger.Info(fmt.Sprintf("Job File Location: Contains '<Job>' or '<job' tag: %t", hasJobTag))

	// Check for WFO indicators in the actual XML structure
	// Since there's no task_type, look for WFO in the filename or other indicators
	hasWFOIndicator := strings.Contains(xmlStr, "WFO") || strings.Contains(xmlStr, "wfo")
	fmt.Printf("[DEBUG] Job File Location: Contains WFO indicator: %t\n", hasWFOIndicator)
	wfoLogger.Info(fmt.Sprintf("Job File Location: Contains WFO indicator: %t", hasWFOIndicator))

	// Check if it has the basic job structure we need
	hasRequiredElements := strings.Contains(xmlStr, "<Id>") && strings.Contains(xmlStr, "<Symbol>") && strings.Contains(xmlStr, "<Timeframe>")
	fmt.Printf("[DEBUG] Job File Location: Contains required elements (Id, Symbol, Timeframe): %t\n", hasRequiredElements)
	wfoLogger.Info(fmt.Sprintf("Job File Location: Contains required elements (Id, Symbol, Timeframe): %t", hasRequiredElements))

	if !hasJobTag || !hasRequiredElements {
		fmt.Printf("[ERROR] Job File Location: Invalid job XML content - missing required elements\n")
		fmt.Printf("[ERROR] Job File Location: Has job tag: %t, Has required elements: %t\n", hasJobTag, hasRequiredElements)
		wfoLogger.Error(fmt.Sprintf("Job File Location: Invalid job XML content - missing required elements"))
		wfoLogger.Error(fmt.Sprintf("Job File Location: Has job tag: %t, Has required elements: %t", hasJobTag, hasRequiredElements))
		return "", fmt.Errorf("invalid job XML content in file '%s' - missing Job tag or required elements", jobFilePath)
	}

	// Log success with WFO indicator status
	fmt.Printf("[DEBUG] Job File Location: Job XML validation passed (WFO indicator: %t)\n", hasWFOIndicator)
	wfoLogger.Info(fmt.Sprintf("Job File Location: Job XML validation passed (WFO indicator: %t)", hasWFOIndicator))

	fmt.Printf("[DEBUG] Job File Location: Successfully located and validated WFO job file\n")
	return xmlStr, nil
}

// buildWFORetestXML constructs the complete WFO_RETEST XML from original XML and optimization results
func buildWFORetestXML(originalXML string, optResults []OPTResult, retestRanges []WFORetestDateRange, jobID, symbol, timeframe string, totalRuns, osPercentage int) (string, error) {
	fmt.Printf("[DEBUG] XML Building: Starting construction of %d job elements with filename metadata (runs=%d, os=%d%%)\n", len(optResults), totalRuns, osPercentage)

	// Extract ALL <Job> elements from the original WFO XML so we preserve per-run fields (e.g., <run>)
	jobRegex := regexp.MustCompile(`(?s)<Job>(.*?)</Job>`)
	jobMatches := jobRegex.FindAllStringSubmatch(originalXML, -1)
	if len(jobMatches) == 0 {
		fmt.Printf("[ERROR] XML Building: No <Job> elements found in original XML\n")
		return "", fmt.Errorf("no <Job> elements found in original XML")
	}

	fmt.Printf("[DEBUG] XML Building: Found %d job elements in original XML\n", len(jobMatches))

	limit := len(optResults)
	if len(jobMatches) < limit {
		limit = len(jobMatches)
	}
	if len(retestRanges) < limit {
		limit = len(retestRanges)
	}
	if limit == 0 {
		return "", fmt.Errorf("no job elements can be built: mismatched inputs")
	}

	var jobElements []string

	for i := 0; i < limit; i++ {
		result := optResults[i]
		retestRange := retestRanges[i]
		runNumber := i + 1

		fmt.Printf("[DEBUG] XML Building: Creating job element for run %d (IS: %s to %s, OS: %s to %s)\n",
			runNumber, retestRange.OriginalISStart, retestRange.OriginalISEnd,
			retestRange.OriginalOSStart, retestRange.OriginalOSEnd)

		// Use the corresponding original <Job> as the template to preserve <run> and other run-specific fields
		templateJobXML := fmt.Sprintf("<Job>%s</Job>", jobMatches[i][1])

		jobXML, err := createWFORetestJobElement(templateJobXML, result, retestRange, runNumber, jobID, symbol, timeframe, totalRuns, osPercentage)
		if err != nil {
			fmt.Printf("[ERROR] XML Building: Failed to create job element for run %d - %v\n", runNumber, err)
			return "", fmt.Errorf("create job element for run %d: %w", runNumber, err)
		}

		fmt.Printf("[DEBUG] XML Building: Created job element for run %d (%d bytes)\n", runNumber, len(jobXML))
		jobElements = append(jobElements, jobXML)
	}

	// Wrap all job elements in root
	wfoRetestXML := fmt.Sprintf("<root>\n%s\n</root>", strings.Join(jobElements, "\n"))

	fmt.Printf("[DEBUG] XML Building: Wrapped %d job elements in root structure\n", len(jobElements))
	fmt.Printf("[DEBUG] XML Building: Generated complete WFO_RETEST XML with %d job elements (%d total bytes)\n", len(jobElements), len(wfoRetestXML))
	return wfoRetestXML, nil
}

// createWFORetestJobElement creates a single job element with fixed parameters and proper date handling
func createWFORetestJobElement(originalXML string, result OPTResult, retestRange WFORetestDateRange, runNumber int, jobID, symbol, timeframe string, totalRuns, osPercentage int) (string, error) {
	fmt.Printf("[DEBUG] Job Element Creation: Starting creation for run %d with filename metadata\n", runNumber)
	// Start with original XML content (copy everything except parameters)
	jobXML := originalXML

	// Step 1: Change task_type from WFO to WFO_RETEST
	fmt.Printf("[DEBUG] Job Element Creation Step 1: Changing task_type from WFO to WFO_RETEST\n")
	jobXML = replaceXMLTag(jobXML, "task_type", "WFO_RETEST")

	// Step 2: Update filename element with WFO_RETEST format including RUN and OS suffixes
	wfoRetestFilename := fmt.Sprintf("%s_%s_%s_WFO_RETEST_RUN-%d_OS-%d.job", jobID, symbol, timeframe, totalRuns, osPercentage)
	fmt.Printf("[DEBUG] Job Element Creation Step 2: Updating filename from WFO to WFO_RETEST format: %s\n", wfoRetestFilename)
	jobXML = replaceXMLTag(jobXML, "filename", wfoRetestFilename)

	// Step 3: Preserve existing <run> element from the original WFO XML (no attribute injection)
	// Note: We do NOT add run_number to <Job> and we do NOT modify the <run> tag.

	// Step 4: Keep original startDate and endDate from the WFO job (no buffering)
	fmt.Printf("[DEBUG] Job Element Creation Step 4: Preserving original startDate and endDate (no buffering)\n")

	// Step 5: IS/OS date ranges already exist in the original XML - no need to add them
	fmt.Printf("[DEBUG] Job Element Creation Step 5: IS/OS date ranges already exist in original XML (no action needed)\n")

	// Step 6: Replace parameters section with fixed parameters
	fmt.Printf("[DEBUG] Job Element Creation Step 6: Replacing parameters with fixed values\n")
	fmt.Printf("[DEBUG] Job Element Creation: Parameters JSON for run %d: %s\n", runNumber, result.ParametersJSON)
	jobXML, err := replaceParametersWithFixed(jobXML, result.ParametersJSON)
	if err != nil {
		fmt.Printf("[ERROR] Job Element Creation: Failed to replace parameters for run %d - %v\n", runNumber, err)
		return "", fmt.Errorf("replace parameters for run %d: %w", runNumber, err)
	}

	fmt.Printf("[DEBUG] Job Element Creation: Successfully created job element for run %d with fixed parameters and updated filename\n", runNumber)
	return jobXML, nil
}

// replaceParametersWithFixed replaces optimization parameters with fixed values from OPT results
func replaceParametersWithFixed(xmlStr, parametersJSON string) (result string, err error) {
	// Add crash protection
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("[FATAL] Parameter Replacement: Panic occurred - %v\n", r)
			wfoLogger.Error(fmt.Sprintf("Parameter Replacement: Panic occurred - %v", r))
			err = fmt.Errorf("panic during parameter replacement: %v", r)
			result = ""
		}
	}()

	fmt.Printf("[DEBUG] Parameter Replacement: Starting parameter transformation\n")
	fmt.Printf("[DEBUG] Parameter Replacement: Raw parameters JSON: %s\n", parametersJSON)
	wfoLogger.Info(fmt.Sprintf("Parameter Replacement: Raw parameters JSON: %s", parametersJSON))

	// Fix double-escaped quotes in the JSON string
	// Convert "" to " (double quotes to single quotes)
	cleanedJSON := strings.ReplaceAll(parametersJSON, `""`, `"`)

	// Remove outer quotes if the entire string is wrapped in quotes
	if len(cleanedJSON) >= 2 && cleanedJSON[0] == '"' && cleanedJSON[len(cleanedJSON)-1] == '"' {
		cleanedJSON = cleanedJSON[1 : len(cleanedJSON)-1]
		fmt.Printf("[DEBUG] Parameter Replacement: Removed outer quotes from JSON\n")
		wfoLogger.Info(fmt.Sprintf("Parameter Replacement: Removed outer quotes from JSON"))
	}

	fmt.Printf("[DEBUG] Parameter Replacement: Final cleaned JSON: %s\n", cleanedJSON)
	wfoLogger.Info(fmt.Sprintf("Parameter Replacement: Final cleaned JSON: %s", cleanedJSON))

	// Parse optimized parameters from JSON
	fmt.Printf("[DEBUG] Parameter Replacement: About to parse JSON with Unmarshal\n")
	var optimizedParams map[string]interface{}
	if err := json.Unmarshal([]byte(cleanedJSON), &optimizedParams); err != nil {
		fmt.Printf("[ERROR] Parameter Replacement: Failed to parse parameters JSON - %v\n", err)
		fmt.Printf("[ERROR] Parameter Replacement: Original JSON: %s\n", parametersJSON)
		fmt.Printf("[ERROR] Parameter Replacement: Cleaned JSON: %s\n", cleanedJSON)
		wfoLogger.Error(fmt.Sprintf("Parameter Replacement: Failed to parse parameters JSON - %v", err))
		wfoLogger.Error(fmt.Sprintf("Parameter Replacement: Original JSON: %s", parametersJSON))
		wfoLogger.Error(fmt.Sprintf("Parameter Replacement: Cleaned JSON: %s", cleanedJSON))
		return "", fmt.Errorf("parse parameters JSON: %w", err)
	}
	fmt.Printf("[DEBUG] Parameter Replacement: JSON parsing successful\n")
	fmt.Printf("[DEBUG] Parameter Replacement: Parsed %d optimized parameters\n", len(optimizedParams))
	for key, value := range optimizedParams {
		fmt.Printf("[DEBUG] Parameter Replacement: - %s = %v\n", key, value)
	}

	// Find and replace the parameters section
	fmt.Printf("[DEBUG] Parameter Replacement: Locating parameters section in XML\n")
	fmt.Printf("[DEBUG] Parameter Replacement: XML string length: %d bytes\n", len(xmlStr))
	parametersRegex := regexp.MustCompile(`(?s)<parameters>(.*?)</parameters>`)
	fmt.Printf("[DEBUG] Parameter Replacement: Regex compiled successfully\n")
	match := parametersRegex.FindStringSubmatch(xmlStr)
	fmt.Printf("[DEBUG] Parameter Replacement: Regex search completed, found %d matches\n", len(match))
	if len(match) < 2 {
		fmt.Printf("[ERROR] Parameter Replacement: Parameters section not found in XML\n")
		fmt.Printf("[DEBUG] Parameter Replacement: Searching for '<parameters>' in XML: %t\n", strings.Contains(xmlStr, "<parameters>"))
		snippetLength := 1000
		if len(xmlStr) < snippetLength {
			snippetLength = len(xmlStr)
		}
		fmt.Printf("[DEBUG] Parameter Replacement: XML snippet (first %d chars): %s\n", snippetLength, xmlStr[:snippetLength])
		return "", fmt.Errorf("parameters section not found in XML")
	}

	originalParameters := match[1]
	fmt.Printf("[DEBUG] Parameter Replacement: Found parameters section (%d bytes)\n", len(originalParameters))

	// Transform each parameter from OptRange to Fixed
	fmt.Printf("[DEBUG] Parameter Replacement: Transforming OptRange parameters to Fixed\n")
	newParameters, err := transformParametersToFixed(originalParameters, optimizedParams)
	if err != nil {
		fmt.Printf("[ERROR] Parameter Replacement: Parameter transformation failed - %v\n", err)
		return "", fmt.Errorf("transform parameters: %w", err)
	}
	fmt.Printf("[DEBUG] Parameter Replacement: Transformed parameters (%d bytes)\n", len(newParameters))

	// Replace parameters section
	result = parametersRegex.ReplaceAllString(xmlStr, fmt.Sprintf("<parameters>%s</parameters>", newParameters))

	fmt.Printf("[DEBUG] Parameter Replacement: Successfully replaced parameters section with %d fixed parameters\n", len(optimizedParams))
	fmt.Printf("[DEBUG] Parameter Replacement: Final XML size: %d bytes\n", len(result))
	return result, nil
}
// transformParametersToFixed converts OptRange parameters to Fixed parameters using optimized values
// IMPORTANT: Only processes top-level parameter nodes under <parameters> and preserves their wrappers.
func transformParametersToFixed(originalParams string, optimizedValues map[string]interface{}) (string, error) {
	// Wrap with a <parameters> root so we can use an XML decoder safely
	wrapped := "<parameters>" + originalParams + "</parameters>"
	dec := xml.NewDecoder(strings.NewReader(wrapped))

	type paramBlock struct {
		Name     string
		Children map[string]string
	}

	var (
		blocks            []paramBlock
		depth             int
		current           *paramBlock
		currentChildName  string
		currentChildValue strings.Builder
	)

	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("XML decode parameters: %w", err)
		}

		switch t := tok.(type) {
		case xml.StartElement:
			name := t.Name.Local
			if name == "parameters" {
				continue
			}
			depth++
			if depth == 1 {
				current = &paramBlock{Name: name, Children: make(map[string]string)}
			} else if depth == 2 {
				currentChildName = name
				currentChildValue.Reset()
			}

		case xml.CharData:
			if depth == 2 && currentChildName != "" {
				currentChildValue.Write([]byte(t))
			}

		case xml.EndElement:
			name := t.Name.Local
			if name == "parameters" {
				continue
			}
			if depth == 2 && currentChildName != "" {
				current.Children[currentChildName] = strings.TrimSpace(currentChildValue.String())
				currentChildName = ""
			}
			if depth == 1 && current != nil {
				blocks = append(blocks, *current)
				current = nil
			}
			depth--
		}
	}

	// Now render new <parameters> content per block
	var out strings.Builder
	for _, p := range blocks {
		paramType := p.Children["param_type"]
		dataType := p.Children["data_type"]
		optimizable := strings.EqualFold(p.Children["optimizable_ind"], "true")
		value := p.Children["value"]

		if paramType == "OptRange" && optimizable {
			// Use optimized value if available; otherwise keep existing <value>
			if ov, ok := optimizedValues[p.Name]; ok {
				value = fmt.Sprintf("%v", ov)
				fmt.Printf("[DEBUG] Parameter Transformation: Using optimized value for %s: %s\n", p.Name, value)
			} else {
				fmt.Printf("[DEBUG] Parameter Transformation: No optimized value for %s, keeping current value: %s\n", p.Name, value)
			}

			// Determine Fixed vs FixedString vs FixedBool by data_type
			fixedType := "Fixed"
			if strings.EqualFold(dataType, "string") {
				fixedType = "FixedString"
			} else if strings.EqualFold(dataType, "bool") || strings.EqualFold(dataType, "boolean") {
				fixedType = "FixedBool"
			}

			out.WriteString(fmt.Sprintf("<%s>\n", p.Name))
			out.WriteString(fmt.Sprintf("  <value>%s</value>\n", value))
			out.WriteString(fmt.Sprintf("  <param_type>%s</param_type>\n", fixedType))
			if dataType != "" {
				out.WriteString(fmt.Sprintf("  <data_type>%s</data_type>\n", dataType))
			}
			out.WriteString("  <optimizable_ind>false</optimizable_ind>\n")
			out.WriteString(fmt.Sprintf("</%s>\n", p.Name))
		} else {
			// Keep as Fixed/FixedString/FixedBool. Re-render common fields only.
			out.WriteString(fmt.Sprintf("<%s>\n", p.Name))
			if v := p.Children["value"]; v != "" {
				out.WriteString(fmt.Sprintf("  <value>%s</value>\n", v))
			}
			if pt := p.Children["param_type"]; pt != "" {
				out.WriteString(fmt.Sprintf("  <param_type>%s</param_type>\n", pt))
			}
			if dt := p.Children["data_type"]; dt != "" {
				out.WriteString(fmt.Sprintf("  <data_type>%s</data_type>\n", dt))
			}
			if oi := p.Children["optimizable_ind"]; oi != "" {
				out.WriteString(fmt.Sprintf("  <optimizable_ind>%s</optimizable_ind>\n", oi))
			}
			out.WriteString(fmt.Sprintf("</%s>\n", p.Name))
		}
	}

	return out.String(), nil
}

// transformOptRangeToFixed converts a single OptRange parameter to Fixed using optimized value
func transformOptRangeToFixed(paramContent string, optimizedValue interface{}) string {
	// Remove OptRange-specific tags (start, end, step)
	content := regexp.MustCompile(`(?s)<start>.*?</start>\s*`).ReplaceAllString(paramContent, "")
	content = regexp.MustCompile(`(?s)<end>.*?</end>\s*`).ReplaceAllString(content, "")
	content = regexp.MustCompile(`(?s)<step>.*?</step>\s*`).ReplaceAllString(content, "")

	// Update value with optimized value
	valueRegex := regexp.MustCompile(`<value>.*?</value>`)
	content = valueRegex.ReplaceAllString(content, fmt.Sprintf("<value>%v</value>", optimizedValue))

	// Determine param_type based on data_type
	if strings.Contains(content, "<data_type>string</data_type>") {
		// String parameters use FixedString
		paramTypeRegex := regexp.MustCompile(`<param_type>OptRange</param_type>`)
		content = paramTypeRegex.ReplaceAllString(content, "<param_type>FixedString</param_type>")
	} else {
		// Numeric parameters use Fixed
		paramTypeRegex := regexp.MustCompile(`<param_type>OptRange</param_type>`)
		content = paramTypeRegex.ReplaceAllString(content, "<param_type>Fixed</param_type>")
	}

	// Change optimizable_ind from true to false
	optimizableRegex := regexp.MustCompile(`<optimizable_ind>true</optimizable_ind>`)
	content = optimizableRegex.ReplaceAllString(content, "<optimizable_ind>false</optimizable_ind>")

	return content
}

// addRunNumberAttribute adds run_number attribute to job element (supports <Job> and <job>)
func addRunNumberAttribute(xmlStr string, runNumber int) string {
	jobRegex := regexp.MustCompile(`<(?i:job)([^>]*)>`) // case-insensitive, preserves match text
	return jobRegex.ReplaceAllStringFunc(xmlStr, func(m string) string {
		if strings.Contains(strings.ToLower(m), "run_number=") {
			return m // already has attribute
		}
		return strings.Replace(m, ">", fmt.Sprintf(" run_number=\"%d\">", runNumber), 1)
	})
}

// calculateOSPercentage determines the OS percentage from date ranges
func calculateOSPercentage(ranges []WFORetestDateRange) int {
	if len(ranges) == 0 {
		return 0
	}

	// Use first range to calculate OS percentage
	firstRange := ranges[0]
	if firstRange.OriginalOSStart == "" || firstRange.OriginalOSEnd == "" {
		return 0
	}

	// Calculate IS and OS durations
	isStart, _ := parseDate(firstRange.OriginalISStart)
	isEnd, _ := parseDate(firstRange.OriginalISEnd)
	osStart, _ := parseDate(firstRange.OriginalOSStart)
	osEnd, _ := parseDate(firstRange.OriginalOSEnd)

	isDuration := isEnd.Sub(isStart).Hours() / 24
	osDuration := osEnd.Sub(osStart).Hours() / 24
	totalDuration := isDuration + osDuration

	if totalDuration == 0 {
		return 0
	}

	return int((osDuration / totalDuration) * 100)
}

// Helper functions for XML manipulation
// Note: replaceXMLTag and addXMLTag functions are already defined in api.go