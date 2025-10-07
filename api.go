package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type APIClient struct {
	config     *Config
	auth       *AuthManager
	httpClient *http.Client
}

type Job struct {
	ID             string `json:"id"`
	WorkflowID     string `json:"workflow_id"`
	WorkflowTaskID string `json:"workflow_task_id"`
	Status         string `json:"status"`
	XMLURL         string `json:"xmlUrl"`
	Symbol         string `json:"symbol"`
	Timeframe      string `json:"timeframe"`
	TaskType       string `json:"task_type"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
	Redownload     bool   `json:"redownload"`
}

type PollJobsResponse struct {
	Jobs []Job `json:"jobs"`
}
type PollJobsRequest struct {
	Limit int `json:"limit"`
}

type UploadCSVResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	JobID   string `json:"job_id,omitempty"`
}

type UploadOptResponse struct {
	JobID   string `json:"jobId"`
	Status  string `json:"status"`
	Path    string `json:"path"`
	Message string `json:"message,omitempty"`
}

type UploadDailySummaryResponse struct {
	JobID   string `json:"jobId"`
	Status  string `json:"status"`
	Path    string `json:"path"`
	Message string `json:"message,omitempty"`
}

func NewAPIClient(cfg *Config, am *AuthManager) *APIClient {
	return &APIClient{
		config:     cfg,
		auth:       am,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

func (ac *APIClient) PollJobs(limit int) (*PollJobsResponse, error) {
	if err := ac.auth.EnsureValidToken(); err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/functions/v1/poll-jobs", ac.config.Supabase.URL)
	body, _ := json.Marshal(PollJobsRequest{Limit: limit})
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("create poll request: %w", err)
	}
	for k, v := range ac.auth.GetAuthHeaders() {
		req.Header.Set(k, v)
	}

	resp, err := ac.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("poll request failed: %w", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("poll failed: http %d", resp.StatusCode)
	}

	var pr PollJobsResponse
	if err := json.Unmarshal(data, &pr); err != nil {
		return nil, fmt.Errorf("parse poll response: %w", err)
	}
	return &pr, nil
}

func (ac *APIClient) UploadCSV(filePath, symbol, timeframe string) (*UploadCSVResponse, error) {
	if err := ac.auth.EnsureValidToken(); err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/functions/v1/ingest-trades-csv", ac.config.Supabase.URL)

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	f, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("open file: %w", err)
	}
	defer f.Close()
	part, err := mw.CreateFormFile("file", filepath.Base(filePath))
	if err != nil {
		return nil, fmt.Errorf("form file: %w", err)
	}
	if _, err := io.Copy(part, f); err != nil {
		return nil, fmt.Errorf("copy file: %w", err)
	}
	mw.WriteField("symbol", symbol)
	mw.WriteField("timeframe", timeframe)
	mw.Close()

	req, err := http.NewRequest("POST", url, &buf)
	if err != nil {
		return nil, fmt.Errorf("create upload request: %w", err)
	}
	for k, v := range ac.auth.GetAuthHeaders() {
		req.Header.Set(k, v)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())

	resp, err := ac.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("upload request failed: %w", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("upload failed: http %d", resp.StatusCode)
	}

	var ur UploadCSVResponse
	if err := json.Unmarshal(data, &ur); err != nil {
		return nil, fmt.Errorf("parse upload response: %w", err)
	}
	return &ur, nil
}

func (ac *APIClient) UploadOpt(filePath, jobID, resultType string) (*UploadOptResponse, error) {
	if err := ac.auth.EnsureValidToken(); err != nil {
		return nil, err
	}
	if resultType == "" {
		resultType = "performance"
	}
	url := fmt.Sprintf("%s/functions/v1/upload-opt-results", ac.config.Supabase.URL)

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	f, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("open file: %w", err)
	}
	defer f.Close()
	part, err := mw.CreateFormFile("file", filepath.Base(filePath))
	if err != nil {
		return nil, fmt.Errorf("form file: %w", err)
	}
	if _, err := io.Copy(part, f); err != nil {
		return nil, fmt.Errorf("copy file: %w", err)
	}
	mw.WriteField("job_id", jobID)
	mw.WriteField("type", resultType)
	mw.Close()

	req, err := http.NewRequest("POST", url, &buf)
	if err != nil {
		return nil, fmt.Errorf("create upload request: %w", err)
	}
	for k, v := range ac.auth.GetAuthHeaders() {
		req.Header.Set(k, v)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())

	resp, err := ac.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("upload request failed: %w", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("upload failed: http %d - %s", resp.StatusCode, string(data))
	}

	var ur UploadOptResponse
	if err := json.Unmarshal(data, &ur); err != nil {
		return nil, fmt.Errorf("parse upload response: %w", err)
	}
	return &ur, nil
}

func (ac *APIClient) UploadDailySummary(filePath, jobID string) (*UploadDailySummaryResponse, error) {
	if err := ac.auth.EnsureValidToken(); err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/functions/v1/upload-daily-summary", ac.config.Supabase.URL)

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	f, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("open file: %w", err)
	}
	defer f.Close()
	part, err := mw.CreateFormFile("file", filepath.Base(filePath))
	if err != nil {
		return nil, fmt.Errorf("form file: %w", err)
	}
	if _, err := io.Copy(part, f); err != nil {
		return nil, fmt.Errorf("copy file: %w", err)
	}
	if err := mw.WriteField("jobId", jobID); err != nil {
		return nil, fmt.Errorf("write jobId field: %w", err)
	}
	if err := mw.WriteField("projectId", ac.config.Supabase.ProjectID); err != nil {
		return nil, fmt.Errorf("write projectId field: %w", err)
	}
	if err := mw.Close(); err != nil {
		return nil, fmt.Errorf("close multipart writer: %w", err)
	}

	req, err := http.NewRequest("POST", url, &buf)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", ac.auth.accessToken))

	resp, err := ac.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("upload failed: http %d - %s", resp.StatusCode, string(data))
	}

	var ur UploadDailySummaryResponse
	if err := json.Unmarshal(data, &ur); err != nil {
		return nil, fmt.Errorf("parse upload response: %w", err)
	}
return &ur, nil
}

// BacktestExistsForJob checks if a strategy_backtests record exists for the given source_job_id
func (ac *APIClient) BacktestExistsForJob(jobID string) (bool, error) {
	if err := ac.auth.EnsureValidToken(); err != nil {
		return false, err
	}
	url := fmt.Sprintf("%s/rest/v1/strategy_backtests?select=id&source_job_id=eq.%s&limit=1", ac.config.Supabase.URL, jobID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil { return false, fmt.Errorf("create request: %w", err) }
	req.Header.Set("apikey", ac.config.Supabase.AnonKey)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", ac.auth.accessToken))
	resp, err := ac.httpClient.Do(req)
	if err != nil { return false, fmt.Errorf("do request: %w", err) }
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return false, fmt.Errorf("rest query failed: http %d - %s", resp.StatusCode, string(b))
	}
	b, _ := io.ReadAll(resp.Body)
	// If empty array, not found
	if strings.TrimSpace(string(b)) == "[]" { return false, nil }
	return true, nil
}

// WaitForBacktestByJob polls until a backtest exists for the job or timeout occurs
func (ac *APIClient) WaitForBacktestByJob(jobID string, timeout time.Duration) (bool, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		exists, err := ac.BacktestExistsForJob(jobID)
		if err != nil { return false, err }
		if exists { return true, nil }
		time.Sleep(2 * time.Second)
	}
	return false, nil
}

func (ac *APIClient) TestConnection() error {
	url := fmt.Sprintf("%s/rest/v1/", ac.config.Supabase.URL)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("create test request: %w", err)
	}
	req.Header.Set("apikey", ac.config.Supabase.AnonKey)
	req.Header.Set("Authorization", "Bearer "+ac.config.Supabase.AnonKey)
	resp, err := ac.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("connection test failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("connection test failed: http %d", resp.StatusCode)
	}
	return nil
}

// extractFilenameFromXML extracts the filename element from XML content
func extractFilenameFromXML(xmlContent string) (string, error) {
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

func (ac *APIClient) DownloadFile(url, filePath string) error {
	fmt.Printf("[DEBUG] Starting XML download from URL: %s\n", url)
	fmt.Printf("[DEBUG] Target file path: %s\n", filePath)
	
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("create download request: %w", err)
	}
	resp, err := ac.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("download request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("[ERROR] Download failed with status %d: %s\n", resp.StatusCode, string(body))
		return fmt.Errorf("download failed: http %d - %s", resp.StatusCode, string(body))
	}

	// Read the XML content from the response
	xmlContent, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response body: %w", err)
	}

	// Enhanced logging for MM task debugging
	fmt.Printf("[DEBUG] Raw XML content length: %d bytes\n", len(xmlContent))
	fmt.Printf("[DEBUG] Raw XML content preview (first 500 chars):\n%s\n", string(xmlContent[:min(500, len(xmlContent))]))
	
	// Count job elements in raw XML
	jobCount := bytes.Count(xmlContent, []byte("<Job>"))
	fmt.Printf("[DEBUG] Number of <Job> elements found in raw XML: %d\n", jobCount)

	// Extract filename from XML content for proper raw file naming
	var rawFilePath string
	if extractedFilename, err := extractFilenameFromXML(string(xmlContent)); err == nil {
		// Use extracted filename with _temp suffix instead of _raw
		baseName := strings.TrimSuffix(extractedFilename, filepath.Ext(extractedFilename))
		rawFilePath = filepath.Join(filepath.Dir(filePath), fmt.Sprintf("%s_temp.xml", baseName))
	} else {
		// Fallback to original naming if extraction fails
		fmt.Printf("[WARNING] Could not extract filename from XML, using fallback naming: %v\n", err)
		rawFilePath = filepath.Join(filepath.Dir(filePath),
			fmt.Sprintf("%s_temp.xml",
				filepath.Base(filePath[:len(filePath)-len(filepath.Ext(filePath))])))
	}

	rawOut, err := os.Create(rawFilePath)
	if err != nil {
		fmt.Printf("[WARNING] Could not create temp XML file %s: %v\n", rawFilePath, err)
	} else {
		defer rawOut.Close()
		_, err = rawOut.Write(xmlContent)
		if err != nil {
			fmt.Printf("[WARNING] Could not write temp XML file: %v\n", err)
		} else {
			fmt.Printf("[DEBUG] Saved temp XML to: %s\n", rawFilePath)
		}
	}

	// Create output file
	out, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("create file %s: %w", filePath, err)
	}
	defer out.Close()

	// Check if this is an MM job with symbols, MTF job with timeframes, or WFO job that need expansion
	symbolsPattern := []byte("<symbols>")
	timeframesPattern := []byte("<timeframes>")
	oosRunsPattern := []byte("<oos_runs>")
	mmJobPattern := []byte("<task_type>MM</task_type>")
	mtfJobPattern := []byte("<task_type>MTF</task_type>")
	wfoJobPattern := []byte("<task_type>WFO</task_type>")
	wfmJobPattern := []byte("<task_type>WFM</task_type>")
	dwfmJobPattern := []byte("<task_type>DWFM</task_type>")

	isMMJob := bytes.Contains(xmlContent, mmJobPattern)
	isMTFJob := bytes.Contains(xmlContent, mtfJobPattern)
	isWFOJob := bytes.Contains(xmlContent, wfoJobPattern) || bytes.Contains(xmlContent, wfmJobPattern) || bytes.Contains(xmlContent, dwfmJobPattern)
	hasSymbolsTag := bytes.Contains(xmlContent, symbolsPattern)
	hasTimeframesTag := bytes.Contains(xmlContent, timeframesPattern)
	hasOOSRunsTag := bytes.Contains(xmlContent, oosRunsPattern)

	fmt.Printf("[DEBUG] MM Job detected: %v, MTF Job detected: %v, WFO Job detected: %v, Has symbols tag: %v, Has timeframes tag: %v, Has OOS runs tag: %v\n",
		isMMJob, isMTFJob, isWFOJob, hasSymbolsTag, hasTimeframesTag, hasOOSRunsTag)

	var finalContent string

	if isMMJob && hasSymbolsTag {
		// Process MM job to generate multiple job elements for each symbol
		finalContent = processMMJob(string(xmlContent))
		fmt.Printf("[DEBUG] Processed MM job - generated multiple job elements\n")
	} else if isMTFJob && hasTimeframesTag {
		// Process MTF job to generate multiple job elements for each timeframe
		finalContent = processMTFJob(string(xmlContent))
		fmt.Printf("[DEBUG] Processed MTF job - generated multiple job elements\n")
	} else if isWFOJob && hasOOSRunsTag {
		// Process WFO job to generate multiple job elements for each run
		finalContent = processWFOJob(string(xmlContent))
		fmt.Printf("[DEBUG] Processed WFO job - generated multiple job elements\n")
	} else {
		// Regular single job
		finalContent = fmt.Sprintf("<root>\n%s\n</root>", string(xmlContent))
	}
	
	_, err = out.WriteString(finalContent)
	if err != nil {
		return fmt.Errorf("write file: %w", err)
	}
	
	// Count job elements in final wrapped XML
	finalJobCount := bytes.Count([]byte(finalContent), []byte("<Job>"))
	fmt.Printf("[DEBUG] Final wrapped XML job count: %d\n", finalJobCount)
	fmt.Printf("[DEBUG] Successfully saved wrapped XML to: %s\n", filePath)
	
	return nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// processMMJob takes a single MM job XML and generates multiple job elements for each symbol
func processMMJob(xmlContent string) string {
	fmt.Printf("[DEBUG] Processing MM job XML for symbol expansion\n")
	
	// Extract symbols from <symbols>tag</symbols>
	symbolsStart := strings.Index(xmlContent, "<symbols>")
	symbolsEnd := strings.Index(xmlContent, "</symbols>")
	
	if symbolsStart == -1 || symbolsEnd == -1 {
		fmt.Printf("[DEBUG] No valid symbols tag found, treating as regular job\n")
		return fmt.Sprintf("<root>\n%s\n</root>", xmlContent)
	}
	
	symbolsValue := xmlContent[symbolsStart+9 : symbolsEnd] // +9 for "<symbols>"
	symbols := strings.Split(symbolsValue, ",")
	
	fmt.Printf("[DEBUG] Extracted symbols: %v\n", symbols)
	
	if len(symbols) <= 1 {
		fmt.Printf("[DEBUG] Only one symbol found, treating as regular job\n")
		return fmt.Sprintf("<root>\n%s\n</root>", xmlContent)
	}
	
	// Generate job elements for each symbol
	var jobElements []string
	
	for i, symbol := range symbols {
		symbol = strings.TrimSpace(symbol)
		if symbol == "" {
			continue
		}
		
		// Create modified job XML for this symbol
		jobXML := xmlContent
		
		// Replace the original <Symbol> tag with the current symbol
		jobXML = replaceXMLTag(jobXML, "Symbol", symbol)
		
		// Remove the <symbols> tag from individual job elements
		jobXML = removeXMLTag(jobXML, "symbols")
		
		fmt.Printf("[DEBUG] Generated job element %d for symbol: %s\n", i+1, symbol)
		jobElements = append(jobElements, jobXML)
	}
	
	// Wrap all job elements in root
	finalContent := fmt.Sprintf("<root>\n%s\n</root>", strings.Join(jobElements, "\n"))
	
	fmt.Printf("[DEBUG] Generated %d job elements for MM task\n", len(jobElements))
	return finalContent
}

// processMTFJob takes a single MTF job XML and generates multiple job elements for each timeframe
func processMTFJob(xmlContent string) string {
	fmt.Printf("[DEBUG] Processing MTF job XML for timeframe expansion\n")

	// Extract timeframes from <timeframes>tf1,tf2,tf3</timeframes>
	timeframesStart := strings.Index(xmlContent, "<timeframes>")
	timeframesEnd := strings.Index(xmlContent, "</timeframes>")

	if timeframesStart == -1 || timeframesEnd == -1 {
		fmt.Printf("[DEBUG] No valid timeframes tag found, treating as regular job\n")
		return fmt.Sprintf("<root>\n%s\n</root>", xmlContent)
	}

	timeframesValue := xmlContent[timeframesStart+12 : timeframesEnd] // +12 for "<timeframes>"
	timeframes := strings.Split(timeframesValue, ",")

	fmt.Printf("[DEBUG] Extracted timeframes: %v\n", timeframes)

	if len(timeframes) <= 1 {
		fmt.Printf("[DEBUG] Only one timeframe found, treating as regular job\n")
		return fmt.Sprintf("<root>\n%s\n</root>", xmlContent)
	}

	// Generate job elements for each timeframe
	var jobElements []string

	for i, timeframe := range timeframes {
		timeframe = strings.TrimSpace(timeframe)
		if timeframe == "" {
			continue
		}

		// Create modified job XML for this timeframe
		jobXML := xmlContent

		// Replace the original <Timeframe> tag with the current timeframe
		jobXML = replaceXMLTag(jobXML, "Timeframe", timeframe)

		// Remove the <timeframes> tag from individual job elements
		jobXML = removeXMLTag(jobXML, "timeframes")

		fmt.Printf("[DEBUG] Generated job element %d for timeframe: %s\n", i+1, timeframe)
		jobElements = append(jobElements, jobXML)
	}

	// Wrap all job elements in root
	finalContent := fmt.Sprintf("<root>\n%s\n</root>", strings.Join(jobElements, "\n"))

	fmt.Printf("[DEBUG] Generated %d job elements for MTF task\n", len(jobElements))
	return finalContent
}

// replaceXMLTag replaces the content of an XML tag
func replaceXMLTag(xml, tagName, newValue string) string {
	startTag := fmt.Sprintf("<%s>", tagName)
	endTag := fmt.Sprintf("</%s>", tagName)
	
	start := strings.Index(xml, startTag)
	end := strings.Index(xml, endTag)
	
	if start == -1 || end == -1 {
		return xml
	}
	
	before := xml[:start]
	after := xml[end+len(endTag):]
	replacement := fmt.Sprintf("%s%s%s", startTag, newValue, endTag)
	
	return before + replacement + after
}

// removeXMLTag removes an XML tag and its content
func removeXMLTag(xml, tagName string) string {
	startTag := fmt.Sprintf("<%s>", tagName)
	endTag := fmt.Sprintf("</%s>", tagName)
	
	start := strings.Index(xml, startTag)
	end := strings.Index(xml, endTag)
	
	if start == -1 || end == -1 {
		return xml
	}
	
	before := xml[:start]
	after := xml[end+len(endTag):]
	
	// Also remove any surrounding whitespace/newlines
	before = strings.TrimRightFunc(before, func(r rune) bool {
		return r == ' ' || r == '\t' || r == '\n' || r == '\r'
	})
	after = strings.TrimLeftFunc(after, func(r rune) bool {
		return r == ' ' || r == '\t' || r == '\n' || r == '\r'
	})
	
	return before + "\n" + after
}

// processWFOJob takes a single WFO job XML and generates multiple job elements for each run
func processWFOJob(xmlContent string) string {
	fmt.Printf("[DEBUG] Processing WFO job XML for run expansion\n")

	// Extract OOS parameters
	oosRuns, err := extractXMLTagValue(xmlContent, "oos_runs")
	if err != nil {
		fmt.Printf("[DEBUG] Could not extract oos_runs: %v, treating as regular job\n", err)
		return fmt.Sprintf("<root>\n%s\n</root>", xmlContent)
	}

	oosPercent, err := extractXMLTagValue(xmlContent, "oos_percent")
	if err != nil {
		fmt.Printf("[DEBUG] Could not extract oos_percent: %v, treating as regular job\n", err)
		return fmt.Sprintf("<root>\n%s\n</root>", xmlContent)
	}

	startDate, err := extractXMLTagValue(xmlContent, "startDate")
	if err != nil {
		fmt.Printf("[DEBUG] Could not extract startDate: %v, treating as regular job\n", err)
		return fmt.Sprintf("<root>\n%s\n</root>", xmlContent)
	}

	endDate, err := extractXMLTagValue(xmlContent, "endDate")
	if err != nil {
		fmt.Printf("[DEBUG] Could not extract endDate: %v, treating as regular job\n", err)
		return fmt.Sprintf("<root>\n%s\n</root>", xmlContent)
	}

	// Parse OOS parameters
	runs, err := strconv.Atoi(oosRuns)
	if err != nil {
		fmt.Printf("[DEBUG] Invalid oos_runs value: %s, treating as regular job\n", oosRuns)
		return fmt.Sprintf("<root>\n%s\n</root>", xmlContent)
	}

	oosPercentFloat, err := strconv.ParseFloat(oosPercent, 64)
	if err != nil {
		fmt.Printf("[DEBUG] Invalid oos_percent value: %s, treating as regular job\n", oosPercent)
		return fmt.Sprintf("<root>\n%s\n</root>", xmlContent)
	}

	fmt.Printf("[DEBUG] WFO Parameters - Runs: %d, OOS Percent: %.1f%%, Start: %s, End: %s\n",
		runs, oosPercentFloat, startDate, endDate)

	// Calculate date ranges for all runs
	dateRanges, err := calculateWFORuns(startDate, endDate, runs, oosPercentFloat)
	if err != nil {
		fmt.Printf("[DEBUG] Error calculating WFO runs: %v, treating as regular job\n", err)
		return fmt.Sprintf("<root>\n%s\n</root>", xmlContent)
	}

	// Generate job elements for each run
	var jobElements []string

	for i, dateRange := range dateRanges {
		runNumber := i + 1

		// Create modified job XML for this run
		jobXML := xmlContent

		// Update run-specific parameters
		jobXML = addXMLTag(jobXML, "run", strconv.Itoa(runNumber))
		jobXML = replaceXMLTag(jobXML, "startDate", dateRange.ISStartDate)
		jobXML = replaceXMLTag(jobXML, "endDate", dateRange.GetEndDate())

		// Add IS/OS date tags
		jobXML = addXMLTag(jobXML, "is_start_date", dateRange.ISStartDate)
		jobXML = addXMLTag(jobXML, "is_end_date", dateRange.ISEndDate)

		// Handle OOS period (final extra run has no OOS, matches DLL logic)
		if runNumber == len(dateRanges) {
			// Final extra run: IS-only for future parameter optimization, no OOS period
			jobXML = replaceXMLTag(jobXML, "oos_percent", "0.0")
			// No OS dates for final run
		} else {
			// Regular runs (including second-to-last): add OOS dates and maintain OOS percentage
			jobXML = addXMLTag(jobXML, "os_start_date", dateRange.OSStartDate)
			jobXML = addXMLTag(jobXML, "os_end_date", dateRange.OSEndDate)
			// Ensure oos_percent is preserved for CSV output
			jobXML = replaceXMLTag(jobXML, "oos_percent", oosPercent)
		}

		// Remove the oos_runs tag from individual job elements (not needed per job)
		jobXML = removeXMLTag(jobXML, "oos_runs")

		fmt.Printf("[DEBUG] Generated WFO job element %d: IS(%s to %s)", runNumber, dateRange.ISStartDate, dateRange.ISEndDate)
		if runNumber < len(dateRanges) {
			fmt.Printf(", OS(%s to %s)", dateRange.OSStartDate, dateRange.OSEndDate)
		} else {
			fmt.Printf(" [IS-only, final extra run for future parameter optimization]")
		}
		fmt.Printf("\n")

		jobElements = append(jobElements, jobXML)
	}

	// Wrap all job elements in root
	finalContent := fmt.Sprintf("<root>\n%s\n</root>", strings.Join(jobElements, "\n"))

	fmt.Printf("[DEBUG] Generated %d WFO job elements\n", len(jobElements))
	return finalContent
}

// DateRange represents the date boundaries for a WFO run
type DateRange struct {
	ISStartDate string
	ISEndDate   string
	OSStartDate string
	OSEndDate   string
}

// GetEndDate returns the appropriate end date (OS end for regular runs, IS end for final run)
func (dr *DateRange) GetEndDate() string {
	if dr.OSEndDate != "" {
		return dr.OSEndDate
	}
	return dr.ISEndDate
}

// calculateWFORuns implements the WFO date calculation algorithm from TSClient DLL
func calculateWFORuns(startDate, endDate string, runs int, oosPercent float64) ([]DateRange, error) {
	fmt.Printf("[DEBUG] Calculating WFO runs: %s to %s, %d runs, %.1f%% OOS\n",
		startDate, endDate, runs, oosPercent)

	// Parse dates
	startTime, err := parseDate(startDate)
	if err != nil {
		return nil, fmt.Errorf("parse start date: %w", err)
	}

	endTime, err := parseDate(endDate)
	if err != nil {
		return nil, fmt.Errorf("parse end date: %w", err)
	}

	// Calculate time allocation
	oosSamplePct := oosPercent / 100.0
	isSamplePct := 1.0 - oosSamplePct

	// Calculate total days and days per run
	totalDays := int(endTime.Sub(startTime).Hours() / 24)
	daysPerRun := float64(totalDays) / (float64(runs)*oosSamplePct + isSamplePct)

	isDays := int(daysPerRun * isSamplePct)
	osDays := int(daysPerRun * oosSamplePct)

	fmt.Printf("[DEBUG] WFO Calculation - Total days: %d, Days per run: %.1f, IS days: %d, OS days: %d\n",
		totalDays, daysPerRun, isDays, osDays)

	var dateRanges []DateRange

	// Generate runs + 1 total runs (matches DLL logic)
	// The extra run provides final IS optimization for future parameter use
	for run := 0; run < runs+1; run++ {
		var dr DateRange

		if run == 0 {
			// First run starts at the beginning
			dr.ISStartDate = formatDate(startTime)
		} else {
			// Subsequent runs start IS period before previous OOS end to create overlap
			prevOSEnd, err := parseDate(dateRanges[run-1].OSEndDate)
			if err != nil {
				return nil, fmt.Errorf("parse previous OS end date: %w", err)
			}
			dr.ISStartDate = formatDate(prevOSEnd.AddDate(0, 0, -isDays))
		}

		// Calculate IS end date
		isStart, err := parseDate(dr.ISStartDate)
		if err != nil {
			return nil, fmt.Errorf("parse IS start date: %w", err)
		}
		dr.ISEndDate = formatDate(isStart.AddDate(0, 0, isDays))

		// Calculate OS dates
		isEnd, err := parseDate(dr.ISEndDate)
		if err != nil {
			return nil, fmt.Errorf("parse IS end date: %w", err)
		}
		dr.OSStartDate = formatDate(isEnd.AddDate(0, 0, 1))

		// Handle final run termination (matches DLL logic)
		if run == runs-1 {
			// Second-to-last run: OS extends to specified end date
			dr.OSEndDate = endDate
		} else {
			// Regular run: normal OOS period calculation
			dr.OSEndDate = formatDate(isEnd.AddDate(0, 0, 1+osDays))
		}

		dateRanges = append(dateRanges, dr)
	}

	fmt.Printf("[DEBUG] Generated %d date ranges for WFO runs\n", len(dateRanges))
	return dateRanges, nil
}

// parseDate converts string date (YYYY-MM-DD) to time.Time
func parseDate(dateStr string) (time.Time, error) {
	return time.Parse("2006-01-02", dateStr)
}

// formatDate converts time.Time to string date (YYYY-MM-DD)
func formatDate(t time.Time) string {
	return t.Format("2006-01-02")
}

// extractXMLTagValue extracts the value between XML tags
func extractXMLTagValue(xmlContent, tagName string) (string, error) {
	startTag := fmt.Sprintf("<%s>", tagName)
	endTag := fmt.Sprintf("</%s>", tagName)

	start := strings.Index(xmlContent, startTag)
	end := strings.Index(xmlContent, endTag)

	if start == -1 || end == -1 {
		return "", fmt.Errorf("tag <%s> not found", tagName)
	}

	return strings.TrimSpace(xmlContent[start+len(startTag) : end]), nil
}

// addXMLTag adds a new XML tag to the content (before closing </Job> tag)
func addXMLTag(xmlContent, tagName, value string) string {
	insertPoint := strings.LastIndex(xmlContent, "</Job>")
	if insertPoint == -1 {
		return xmlContent
	}

	newTag := fmt.Sprintf("  <%s>%s</%s>\n", tagName, value, tagName)
	return xmlContent[:insertPoint] + newTag + xmlContent[insertPoint:]
}

func (ac *APIClient) ForceRegenerateXML(jobID string) error {
	if err := ac.auth.EnsureValidToken(); err != nil {
		return err
	}
	url := fmt.Sprintf("%s/functions/v1/download-job-xml?job_id=%s&force=true", ac.config.Supabase.URL, jobID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("create force regenerate request: %w", err)
	}
	for k, v := range ac.auth.GetAuthHeaders() {
		req.Header.Set(k, v)
	}

	resp, err := ac.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("force regenerate request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		data, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("force regenerate failed: http %d - %s", resp.StatusCode, string(data))
	}

	return nil
}

// OPTResult represents a single row from the OPT CSV file
type OPTResult struct {
	Run           int                    `json:"run"`
	ParametersJSON string                `json:"parameters_json"`
	ISStartDate   string                 `json:"is_start_date"`
	ISEndDate     string                 `json:"is_end_date"`
	OSStartDate   string                 `json:"os_start_date"`
	OSEndDate     string                 `json:"os_end_date"`
	AllNetProfit  float64               `json:"all_net_profit"`
	Parameters    map[string]interface{} `json:"parsed_parameters"`
}

// generateCombinedWFOXML creates a secondary XML job with fixed parameters for combined daily summary generation
func generateCombinedWFOXML(originalXML string, optResults []OPTResult) (string, error) {
	fmt.Printf("[DEBUG] Generating combined WFO XML with %d optimization results\n", len(optResults))

	if len(optResults) == 0 {
		return "", fmt.Errorf("no optimization results provided")
	}

	// Parse parameters_json from each OPT result
	var parsedResults []OPTResult
	for _, result := range optResults {
		var params map[string]interface{}
		if err := json.Unmarshal([]byte(result.ParametersJSON), &params); err != nil {
			fmt.Printf("[WARNING] Failed to parse parameters_json for run %d: %v\n", result.Run, err)
			continue
		}
		result.Parameters = params
		parsedResults = append(parsedResults, result)
	}

	if len(parsedResults) == 0 {
		return "", fmt.Errorf("no valid parameters found in OPT results")
	}

	fmt.Printf("[DEBUG] Successfully parsed %d OPT results with valid parameters\n", len(parsedResults))

	// Generate job elements for each run
	var jobElements []string

	for i, result := range parsedResults {
		runNumber := i + 1

		// Create modified job XML for this run
		jobXML := originalXML

		// Add no_opt_file attribute to prevent OPT CSV generation
		jobXML = addNoOptFileAttribute(jobXML)

		// Convert optimizable parameters to fixed parameters
		jobXML = convertToFixedParameters(jobXML, result.Parameters)

		// Update run-specific information
		jobXML = addXMLTag(jobXML, "run", strconv.Itoa(runNumber))
		jobXML = replaceXMLTag(jobXML, "stage", "CombinedDailySummary")

		// Set date ranges based on run number
		if runNumber == 1 {
			// Run 1: Full IS + OS period
			jobXML = replaceXMLTag(jobXML, "startDate", result.ISStartDate)
			jobXML = replaceXMLTag(jobXML, "endDate", result.OSEndDate)
			jobXML = addXMLTag(jobXML, "is_start_date", result.ISStartDate)
			jobXML = addXMLTag(jobXML, "is_end_date", result.ISEndDate)
			jobXML = addXMLTag(jobXML, "os_start_date", result.OSStartDate)
			jobXML = addXMLTag(jobXML, "os_end_date", result.OSEndDate)
		} else {
			// Runs 2-N: OS period only
			jobXML = replaceXMLTag(jobXML, "startDate", result.OSStartDate)
			jobXML = replaceXMLTag(jobXML, "endDate", result.OSEndDate)
			jobXML = addXMLTag(jobXML, "os_start_date", result.OSStartDate)
			jobXML = addXMLTag(jobXML, "os_end_date", result.OSEndDate)
		}

		// Remove WFO-specific tags that are not needed for combined processing
		jobXML = removeXMLTag(jobXML, "oos_runs")
		jobXML = removeXMLTag(jobXML, "oos_percent")
		jobXML = removeXMLTag(jobXML, "optimizableParameters")

		fmt.Printf("[DEBUG] Generated combined WFO job element %d with fixed parameters from run %d\n", runNumber, result.Run)
		jobElements = append(jobElements, jobXML)
	}

	// Wrap all job elements in root
	finalContent := fmt.Sprintf("<root>\n%s\n</root>", strings.Join(jobElements, "\n"))

	fmt.Printf("[DEBUG] Generated combined WFO XML with %d job elements for continuous equity calculation\n", len(jobElements))
	return finalContent, nil
}

// addNoOptFileAttribute adds the no_opt_file="true" attribute to the Job tag
func addNoOptFileAttribute(xmlContent string) string {
	// Find the <Job> tag and add the no_opt_file attribute
	jobTagStart := strings.Index(xmlContent, "<Job>")
	if jobTagStart == -1 {
		return xmlContent
	}

	// Replace <Job> with <Job no_opt_file="true">
	before := xmlContent[:jobTagStart]
	after := xmlContent[jobTagStart+5:] // +5 for "<Job>"
	return before + `<Job no_opt_file="true">` + after
}

// convertToFixedParameters converts optimizable parameters to fixed values
func convertToFixedParameters(xmlContent string, fixedParams map[string]interface{}) string {
	// Find the <parameters> section
	paramsStart := strings.Index(xmlContent, "<parameters>")
	paramsEnd := strings.Index(xmlContent, "</parameters>")

	if paramsStart == -1 || paramsEnd == -1 {
		fmt.Printf("[WARNING] No parameters section found in XML\n")
		return xmlContent
	}

	// Extract the parameters section
	beforeParams := xmlContent[:paramsStart]
	afterParams := xmlContent[paramsEnd+13:] // +13 for "</parameters>"

	// Build new fixed parameters section
	var newParamsContent strings.Builder
	newParamsContent.WriteString("<parameters>\n")

	for paramName, paramValue := range fixedParams {
		// Convert parameter value to string
		valueStr := fmt.Sprintf("%v", paramValue)

		// Create fixed parameter XML
		newParamsContent.WriteString(fmt.Sprintf("  <%s>\n", paramName))
		newParamsContent.WriteString(fmt.Sprintf("    <value>%s</value>\n", valueStr))
		newParamsContent.WriteString(fmt.Sprintf("    <param_type>Fixed</param_type>\n"))
		newParamsContent.WriteString(fmt.Sprintf("  </%s>\n", paramName))
	}

	newParamsContent.WriteString("</parameters>")

	// Reconstruct the XML with fixed parameters
	result := beforeParams + newParamsContent.String() + afterParams

	fmt.Printf("[DEBUG] Converted %d parameters from optimizable to fixed values\n", len(fixedParams))
	return result
}

// processCombinedWFOGeneration triggers combined daily summary generation after OPT completion
func (ac *APIClient) processCombinedWFOGeneration(jobID string, optResults []OPTResult) error {
	fmt.Printf("[DEBUG] Processing combined WFO generation for job %s with %d optimization results\n", jobID, len(optResults))

	// This function would be called after successful OPT CSV upload
	// Implementation would:
	// 1. Fetch original job XML from the database/API
	// 2. Generate combined WFO XML using generateCombinedWFOXML()
	// 3. Submit the new XML job to TradeStation processing
	// 4. Monitor completion and upload resulting combined daily summary

	// For now, return placeholder - full implementation would integrate with existing job processing workflow
	fmt.Printf("[DEBUG] Combined WFO generation processing initiated for job %s\n", jobID)
	return nil
}


func (ac *APIClient) GetJobByID(jobID string) (*Job, error) {
	if err := ac.auth.EnsureValidToken(); err != nil {
		return nil, err
	}

	// Use the same poll-jobs endpoint but filter for specific job
	// This ensures we use the existing API pattern and security
	url := fmt.Sprintf("%s/functions/v1/poll-jobs", ac.config.Supabase.URL)
	body, _ := json.Marshal(PollJobsRequest{Limit: 1000}) // Use high limit to ensure we find the job
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("create job request: %w", err)
	}

	for k, v := range ac.auth.GetAuthHeaders() {
		req.Header.Set(k, v)
	}

	resp, err := ac.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("job request failed: %w", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("job request failed: http %d", resp.StatusCode)
	}

	var pr PollJobsResponse
	if err := json.Unmarshal(data, &pr); err != nil {
		return nil, fmt.Errorf("parse job response: %w", err)
	}

	// Find the specific job by ID
	for _, job := range pr.Jobs {
		if job.ID == jobID {
			return &job, nil
		}
	}

	return nil, fmt.Errorf("job with ID %s not found", jobID)
}
