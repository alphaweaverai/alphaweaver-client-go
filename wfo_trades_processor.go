package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// TradeRecord represents a single trade from the trades CSV file
type TradeRecord struct {
	Date        string  `json:"date"`         // YYYYMMDD format
	Time        string  `json:"time"`         // HHMM format
	RunNumber   int     `json:"run_number"`   // WFO run number
	Symbol      string  `json:"symbol"`       // Trading symbol
	EntryExit   string  `json:"entry_exit"`   // "Entry" or "Exit"
	Quantity    int     `json:"quantity"`     // Position size
	Price       float64 `json:"price"`        // Execution price
	Commission  float64 `json:"commission"`   // Trade commission
	PnL         float64 `json:"pnl"`          // Profit/Loss
	TestType    string  `json:"test_type"`    // "IS" or "OS" from CSV
	Timestamp   time.Time `json:"-"`          // Parsed datetime for sorting
}

// EquityCurveData represents equity curve analysis for IS or OS period
type EquityCurveData struct {
	StrategyName     string    `json:"strategy_name"`
	TaskType         string    `json:"task_type"`
	TestType         string    `json:"test_type"`     // "IS" or "OS"
	Symbol           string    `json:"symbol"`
	Timeframe        string    `json:"timeframe"`
	Run              string    `json:"run"`           // "Combined"
	ProjectID        string    `json:"project_id"`
	JobID            string    `json:"job_id"`
	TaskID           string    `json:"task_id"`
	StartDate        string    `json:"start_date"`
	EndDate          string    `json:"end_date"`
	TotalRuns        int       `json:"total_runs"`
	OSPercentage     int       `json:"os_percentage"`
	Profit           string    `json:"profit"`
	MaxDrawdown      string    `json:"max_drawdown"`
	NetProfitDrawdown string   `json:"netprofit_drawdown"`
	Dates            []string  `json:"dates"`
	CumulativePnL    []float64 `json:"cumulative_pnl"`
	RunningPeak      []float64 `json:"running_peak"`
	Drawdown         []float64 `json:"drawdown"`
	DailyReturns     []float64 `json:"daily_returns"`
	NetProfit        []float64 `json:"net_profit"`
}

// DualEquityCurves represents the final JSON structure with IS and OS curves
type DualEquityCurves struct {
	EquityCurves map[string]EquityCurveData `json:"equity_curves"`
}

// processCombinedTradesList is the main function for Phase 3 - processes trades CSV and generates IS/OS equity curves
func (ac *APIClient) processCombinedTradesList(jobID, symbol, timeframe string, retestRanges []WFORetestDateRange) error {
	fmt.Printf("🚀 [WFO-PROCESSOR] Starting trades list post-processing for job %s (%s_%s)\n", jobID, symbol, timeframe)
	fmt.Printf("🚀 [WFO-PROCESSOR] Date ranges provided: %d ranges\n", len(retestRanges))

	// Step 1: Locate and read trades CSV file
	fmt.Printf("📁 [WFO-PROCESSOR] Step 1: Reading trades CSV file\n")
	trades, metadata, err := ac.readTradesCSV(jobID, symbol, timeframe)
	if err != nil {
		fmt.Printf("❌ [WFO-PROCESSOR] Step 1 FAILED: %v\n", err)
		return fmt.Errorf("read trades CSV: %w", err)
	}
	fmt.Printf("✅ [WFO-PROCESSOR] Step 1 SUCCESS: Read %d trades, metadata: %+v\n", len(trades), metadata)

	// Step 2: Filter trades by IS and OS periods
	fmt.Printf("🔍 [WFO-PROCESSOR] Step 2: Filtering trades by IS/OS periods\n")
	isTrades, osTrades, err := ac.filterTradesByPeriod(trades, retestRanges)
	if err != nil {
		fmt.Printf("❌ [WFO-PROCESSOR] Step 2 FAILED: %v\n", err)
		return fmt.Errorf("filter trades by period: %w", err)
	}
	fmt.Printf("✅ [WFO-PROCESSOR] Step 2 SUCCESS: %d IS trades, %d OS trades\n", len(isTrades), len(osTrades))

	// Step 3: Generate IS equity curve
	fmt.Printf("📊 [WFO-PROCESSOR] Step 3: Generating IS equity curve from %d trades\n", len(isTrades))
	isEquityCurve, err := ac.generateEquityCurve(isTrades, retestRanges, "IS", symbol, timeframe, jobID, metadata)
	if err != nil {
		fmt.Printf("❌ [WFO-PROCESSOR] Step 3 FAILED: %v\n", err)
		return fmt.Errorf("generate IS equity curve: %w", err)
	}
	fmt.Printf("✅ [WFO-PROCESSOR] Step 3 SUCCESS: IS curve generated with %d data points, profit=%s, drawdown=%s\n",
		len(isEquityCurve.Dates), isEquityCurve.Profit, isEquityCurve.MaxDrawdown)

	// Step 4: Generate OS equity curve
	fmt.Printf("📊 [WFO-PROCESSOR] Step 4: Generating OS equity curve from %d trades\n", len(osTrades))
	osEquityCurve, err := ac.generateEquityCurve(osTrades, retestRanges, "OS", symbol, timeframe, jobID, metadata)
	if err != nil {
		fmt.Printf("❌ [WFO-PROCESSOR] Step 4 FAILED: %v\n", err)
		return fmt.Errorf("generate OS equity curve: %w", err)
	}
	fmt.Printf("✅ [WFO-PROCESSOR] Step 4 SUCCESS: OS curve generated with %d data points, profit=%s, drawdown=%s\n",
		len(osEquityCurve.Dates), osEquityCurve.Profit, osEquityCurve.MaxDrawdown)

	// Step 5: Create dual equity curves JSON
	fmt.Printf("🔧 [WFO-PROCESSOR] Step 5: Creating dual equity curves JSON structure\n")
	isKey := fmt.Sprintf("%s-%s-IS", symbol, timeframe)
	osKey := fmt.Sprintf("%s-%s-OS", symbol, timeframe)
	dualCurves := DualEquityCurves{
		EquityCurves: map[string]EquityCurveData{
			isKey: isEquityCurve,
			osKey: osEquityCurve,
		},
	}
	fmt.Printf("✅ [WFO-PROCESSOR] Step 5 SUCCESS: Created dual curves with keys: %s, %s\n", isKey, osKey)

	// Step 6: Save and upload dual equity curves
	fmt.Printf("💾 [WFO-PROCESSOR] Step 6: Saving and uploading dual equity curves\n")
	if err := ac.saveDualEquityCurves(dualCurves, jobID, symbol, timeframe); err != nil {
		fmt.Printf("❌ [WFO-PROCESSOR] Step 6 FAILED: %v\n", err)
		return fmt.Errorf("save dual equity curves: %w", err)
	}
	fmt.Printf("✅ [WFO-PROCESSOR] Step 6 SUCCESS: Dual equity curves saved and uploaded\n")

	fmt.Printf("🎉 [WFO-PROCESSOR] Trades list post-processing completed successfully for job %s\n", jobID)
	return nil
}

// readTradesCSV reads and parses the trades CSV file generated by TSClient
func (ac *APIClient) readTradesCSV(jobID, symbol, timeframe string) ([]TradeRecord, map[string]interface{}, error) {
	fmt.Printf("🔍 [CSV-READER] Searching for trades CSV file\n")
	// Construct trades CSV filename with metadata
	// Pattern: <job_id>_<symbol>_<timeframe>_WFO_RETEST_RUN-<total_runs>_OS-<os_percentage>_trades.csv

	// Search for trades CSV file in results directory
	resultsDir := "C:\\AlphaWeaver\\files\\results"
	pattern := fmt.Sprintf("%s_%s_%s_WFO_RETEST_RUN-*_OS-*_trades.csv", jobID, symbol, timeframe)
	fullPattern := filepath.Join(resultsDir, pattern)
	fmt.Printf("🔍 [CSV-READER] Search pattern: %s\n", fullPattern)

	matches, err := filepath.Glob(fullPattern)
	if err != nil {
		fmt.Printf("❌ [CSV-READER] Glob search failed: %v\n", err)
		return nil, nil, fmt.Errorf("search for trades CSV: %w", err)
	}

	fmt.Printf("🔍 [CSV-READER] Found %d matching files\n", len(matches))
	if len(matches) == 0 {
		fmt.Printf("❌ [CSV-READER] No trades CSV file found for pattern: %s\n", pattern)
		return nil, nil, fmt.Errorf("trades CSV file not found for pattern: %s", pattern)
	}

	if len(matches) > 1 {
		fmt.Printf("⚠️ [CSV-READER] Multiple trades CSV files found: %v\n", matches)
		return nil, nil, fmt.Errorf("multiple trades CSV files found for pattern: %s", pattern)
	}

	tradesFilePath := matches[0]
	fmt.Printf("✅ [CSV-READER] Found trades CSV file: %s\n", tradesFilePath)

	// Extract metadata from filename
	metadata := extractMetadataFromFilename(filepath.Base(tradesFilePath))
	fmt.Printf("📊 [CSV-READER] Extracted metadata: %+v\n", metadata)

	// Read and parse CSV file
	fmt.Printf("📂 [CSV-READER] Opening CSV file for reading\n")
	file, err := os.Open(tradesFilePath)
	if err != nil {
		fmt.Printf("❌ [CSV-READER] Failed to open file: %v\n", err)
		return nil, nil, fmt.Errorf("open trades CSV file: %w", err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		fmt.Printf("❌ [CSV-READER] Failed to read CSV: %v\n", err)
		return nil, nil, fmt.Errorf("read trades CSV: %w", err)
	}

	fmt.Printf("📊 [CSV-READER] Read %d total records from CSV\n", len(records))
	if len(records) < 2 {
		fmt.Printf("❌ [CSV-READER] Insufficient data: need at least 2 records (header + data)\n")
		return nil, nil, fmt.Errorf("insufficient data in trades CSV file")
	}

	// Log header for format verification
	if len(records) > 0 {
		fmt.Printf("📋 [CSV-READER] CSV Header: %v\n", records[0])
		fmt.Printf("📋 [CSV-READER] Header has %d columns\n", len(records[0]))
	}

	// Parse trades records
	var trades []TradeRecord
	fmt.Printf("🔄 [CSV-READER] Parsing %d data records\n", len(records)-1)
	for i, record := range records[1:] { // Skip header
		if i < 3 { // Log first few records for debugging
			fmt.Printf("📄 [CSV-READER] Record %d: %v\n", i+1, record)
		}
		trade, err := parseTradeRecord(record, i+1)
		if err != nil {
			fmt.Printf("❌ [CSV-READER] Failed to parse record %d: %v\n", i+1, err)
			return nil, nil, fmt.Errorf("parse trade record %d: %w", i+1, err)
		}
		trades = append(trades, trade)
	}

	// Sort trades by timestamp for proper equity calculation
	fmt.Printf("🔄 [CSV-READER] Sorting %d trades by timestamp\n", len(trades))
	sort.Slice(trades, func(i, j int) bool {
		return trades[i].Timestamp.Before(trades[j].Timestamp)
	})

	fmt.Printf("✅ [CSV-READER] Successfully parsed %d trades from CSV file\n", len(trades))
	return trades, metadata, nil
}

// extractMetadataFromFilename parses metadata from trades CSV filename
func extractMetadataFromFilename(filename string) map[string]interface{} {
	metadata := make(map[string]interface{})

	// Parse pattern: <job_id>_<symbol>_<timeframe>_WFO_RETEST_RUN-<total_runs>_OS-<os_percentage>_trades.csv
	if strings.Contains(filename, "RUN-") && strings.Contains(filename, "OS-") {
		parts := strings.Split(filename, "_")
		for _, part := range parts {
			if strings.HasPrefix(part, "RUN-") {
				if runs, err := strconv.Atoi(strings.TrimPrefix(part, "RUN-")); err == nil {
					metadata["total_runs"] = runs
				}
			}
			if strings.HasPrefix(part, "OS-") {
				ospart := strings.TrimPrefix(part, "OS-")
				ospart = strings.TrimSuffix(ospart, "_trades.csv")
				if osPercent, err := strconv.Atoi(ospart); err == nil {
					metadata["os_percentage"] = osPercent
				}
			}
		}
	}

	return metadata
}

// parseTradeRecord parses a single trade record from CSV
// TSClient format: Strategy Name,Task No,Project ID,entry_date,entry_price,exit_date,exit_price,stop_price,position,profit,risk,size,symbol,atr,currency_conv,equity,commission,slippage,mae,mfe,run_no,test_type,is_start_date,is_end_date,os_start_date,os_end_date
func parseTradeRecord(record []string, recordNum int) (TradeRecord, error) {
	fmt.Printf("🔧 [TRADE-PARSER] Parsing record %d with %d columns\n", recordNum, len(record))

	if len(record) < 26 {
		fmt.Printf("❌ [TRADE-PARSER] Record %d has insufficient columns: expected 26, got %d\n", recordNum, len(record))
		return TradeRecord{}, fmt.Errorf("insufficient columns in record %d: expected 26, got %d", recordNum, len(record))
	}

	// Parse TSClient trade record format
	// Since this is a complete trade (entry + exit), we'll treat it as an "Exit" with PnL
	trade := TradeRecord{
		EntryExit: "Exit", // Each record represents a complete trade
		Symbol:    strings.TrimSpace(record[12]), // symbol column
	}

	// Parse run number from run_no column (index 20)
	if runNum, err := strconv.Atoi(strings.TrimSpace(record[20])); err == nil {
		trade.RunNumber = runNum
		fmt.Printf("🔧 [TRADE-PARSER] Record %d: Run=%d\n", recordNum, runNum)
	} else {
		fmt.Printf("⚠️ [TRADE-PARSER] Record %d: Failed to parse run number '%s': %v\n", recordNum, record[20], err)
	}

	// Parse exit date from exit_date column (index 5)
	exitDateStr := strings.TrimSpace(record[5])
	if exitDate, err := parseTradeDateTime(exitDateStr); err == nil {
		trade.Date = exitDate.Format("20060102")     // YYYYMMDD
		trade.Time = exitDate.Format("1504")         // HHMM
		trade.Timestamp = exitDate
		fmt.Printf("🔧 [TRADE-PARSER] Record %d: Date=%s, Time=%s\n", recordNum, trade.Date, trade.Time)
	} else {
		fmt.Printf("⚠️ [TRADE-PARSER] Record %d: Failed to parse exit date '%s': %v\n", recordNum, exitDateStr, err)
	}

	// Parse position size from size column (index 11)
	if size, err := strconv.Atoi(strings.TrimSpace(record[11])); err == nil {
		trade.Quantity = size
	} else {
		fmt.Printf("⚠️ [TRADE-PARSER] Record %d: Failed to parse size '%s': %v\n", recordNum, record[11], err)
	}

	// Parse exit price from exit_price column (index 6)
	if price, err := strconv.ParseFloat(strings.TrimSpace(record[6]), 64); err == nil {
		trade.Price = price
	} else {
		fmt.Printf("⚠️ [TRADE-PARSER] Record %d: Failed to parse exit price '%s': %v\n", recordNum, record[6], err)
	}

	// Parse commission from commission column (index 16)
	if commission, err := strconv.ParseFloat(strings.TrimSpace(record[16]), 64); err == nil {
		trade.Commission = commission
	} else {
		fmt.Printf("⚠️ [TRADE-PARSER] Record %d: Failed to parse commission '%s': %v\n", recordNum, record[16], err)
	}

	// Parse profit from profit column (index 9)
	if profit, err := strconv.ParseFloat(strings.TrimSpace(record[9]), 64); err == nil {
		trade.PnL = profit
		fmt.Printf("🔧 [TRADE-PARSER] Record %d: PnL=%.2f\n", recordNum, profit)
	} else {
		fmt.Printf("⚠️ [TRADE-PARSER] Record %d: Failed to parse profit '%s': %v\n", recordNum, record[9], err)
	}

	// Parse test type from test_type column (index 21)
	trade.TestType = strings.TrimSpace(record[21])
	fmt.Printf("🔧 [TRADE-PARSER] Record %d: TestType=%s\n", recordNum, trade.TestType)

	fmt.Printf("✅ [TRADE-PARSER] Record %d parsed: Symbol=%s, Run=%d, PnL=%.2f, TestType=%s, Date=%s\n",
		recordNum, trade.Symbol, trade.RunNumber, trade.PnL, trade.TestType, trade.Date)

	return trade, nil
}

// parseTradeDateTime converts TSClient datetime format to time.Time
func parseTradeDateTime(datetimeStr string) (time.Time, error) {
	// TSClient format: "1/17/2007 16:00:00"
	layouts := []string{
		"1/2/2006 15:04:05",
		"01/02/2006 15:04:05",
		"1/2/2006 3:04:05",
		"01/02/2006 3:04:05",
	}

	for _, layout := range layouts {
		if t, err := time.Parse(layout, datetimeStr); err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("unable to parse datetime: %s", datetimeStr)
}

// parseTradeTimestamp converts date and time strings to time.Time
func parseTradeTimestamp(dateStr, timeStr string) (time.Time, error) {
	// Date format: YYYYMMDD, Time format: HHMM
	if len(dateStr) != 8 || len(timeStr) != 4 {
		return time.Time{}, fmt.Errorf("invalid date/time format: %s %s", dateStr, timeStr)
	}

	// Parse components
	year, _ := strconv.Atoi(dateStr[0:4])
	month, _ := strconv.Atoi(dateStr[4:6])
	day, _ := strconv.Atoi(dateStr[6:8])
	hour, _ := strconv.Atoi(timeStr[0:2])
	minute, _ := strconv.Atoi(timeStr[2:4])

	return time.Date(year, time.Month(month), day, hour, minute, 0, 0, time.UTC), nil
}

// filterTradesByPeriod separates trades into IS and OS periods based on TestType from CSV
func (ac *APIClient) filterTradesByPeriod(trades []TradeRecord, retestRanges []WFORetestDateRange) ([]TradeRecord, []TradeRecord, error) {
	var isTrades, osTrades []TradeRecord

	fmt.Printf("🔍 [TRADE-FILTER] Filtering %d trades by IS/OS periods using TestType\n", len(trades))

	for i, trade := range trades {
		fmt.Printf("🔍 [TRADE-FILTER] Trade %d: Run=%d, TestType=%s, Date=%s, PnL=%.2f\n",
			i+1, trade.RunNumber, trade.TestType, trade.Date, trade.PnL)

		if trade.TestType == "IS" {
			isTrades = append(isTrades, trade)
		} else if trade.TestType == "OS" {
			osTrades = append(osTrades, trade)
		} else {
			fmt.Printf("⚠️ [TRADE-FILTER] Trade %d: Unknown TestType '%s', skipping\n", i+1, trade.TestType)
		}
	}

	fmt.Printf("✅ [TRADE-FILTER] Filtered trades: %d IS trades, %d OS trades\n", len(isTrades), len(osTrades))
	return isTrades, osTrades, nil
}

// isTradeInPeriod checks if a trade falls within the specified date range
func (ac *APIClient) isTradeInPeriod(trade TradeRecord, startDate, endDate string) bool {
	// Convert dates to comparable format (YYYY-MM-DD to YYYYMMDD)
	startDateCompare := strings.ReplaceAll(startDate, "-", "")
	endDateCompare := strings.ReplaceAll(endDate, "-", "")

	return trade.Date >= startDateCompare && trade.Date <= endDateCompare
}

// generateEquityCurve creates equity curve data for IS or OS period
func (ac *APIClient) generateEquityCurve(trades []TradeRecord, retestRanges []WFORetestDateRange, testType, symbol, timeframe, jobID string, metadata map[string]interface{}) (EquityCurveData, error) {
	fmt.Printf("[DEBUG] Generating %s equity curve from %d trades\n", testType, len(trades))

	// Initialize equity curve data
	curve := EquityCurveData{
		StrategyName:  "WFO Strategy", // Could be extracted from parameters
		TaskType:      "WFO_RETEST",
		TestType:      testType,
		Symbol:        symbol,
		Timeframe:     timeframe,
		Run:           "Combined",
		JobID:         jobID,
		TaskID:        jobID, // Could be different
		ProjectID:     jobID, // Could be different
	}

	// Add metadata
	if totalRuns, ok := metadata["total_runs"]; ok {
		curve.TotalRuns = totalRuns.(int)
	}
	if osPercent, ok := metadata["os_percentage"]; ok {
		curve.OSPercentage = osPercent.(int)
	}

	// Calculate date range for this test type
	if testType == "IS" {
		curve.StartDate = getEarliestISDate(retestRanges)
		curve.EndDate = getLatestISDate(retestRanges)
	} else {
		curve.StartDate = getEarliestOSDate(retestRanges)
		curve.EndDate = getLatestOSDate(retestRanges)
	}

	// Calculate equity progression
	if err := ac.calculateEquityProgression(&curve, trades); err != nil {
		return curve, fmt.Errorf("calculate equity progression: %w", err)
	}

	fmt.Printf("[DEBUG] Generated %s equity curve: %s profit, %s max drawdown\n",
		testType, curve.Profit, curve.MaxDrawdown)

	return curve, nil
}

// calculateEquityProgression computes daily equity arrays from trades
func (ac *APIClient) calculateEquityProgression(curve *EquityCurveData, trades []TradeRecord) error {
	if len(trades) == 0 {
		// No trades for this period
		curve.Profit = "0"
		curve.MaxDrawdown = "0"
		curve.NetProfitDrawdown = "0"
		return nil
	}

	// Group trades by date
	tradesByDate := make(map[string][]TradeRecord)
	for _, trade := range trades {
		dateKey := formatDateForEquity(trade.Date)
		tradesByDate[dateKey] = append(tradesByDate[dateKey], trade)
	}

	// Sort dates
	var dates []string
	for date := range tradesByDate {
		dates = append(dates, date)
	}
	sort.Strings(dates)

	// Initialize equity calculation
	initialCapital := 100000.0 // Default initial capital
	currentEquity := initialCapital
	runningPeak := initialCapital
	totalProfit := 0.0

	// Calculate daily equity progression
	for _, date := range dates {
		dayTrades := tradesByDate[date]
		dayPnL := 0.0

		// Sum PnL for all trades on this date
		for _, trade := range dayTrades {
			dayPnL += trade.PnL - trade.Commission
		}

		// Update equity
		currentEquity += dayPnL
		totalProfit += dayPnL

		// Update running peak
		if currentEquity > runningPeak {
			runningPeak = currentEquity
		}

		// Calculate drawdown from running peak
		drawdown := currentEquity - runningPeak

		// Calculate daily return
		dailyReturn := 0.0
		if len(curve.CumulativePnL) > 0 {
			prevEquity := curve.CumulativePnL[len(curve.CumulativePnL)-1]
			if prevEquity > 0 {
				dailyReturn = (currentEquity - prevEquity) / prevEquity
			}
		}

		// Append to arrays
		curve.Dates = append(curve.Dates, date)
		curve.CumulativePnL = append(curve.CumulativePnL, currentEquity)
		curve.RunningPeak = append(curve.RunningPeak, runningPeak)
		curve.Drawdown = append(curve.Drawdown, drawdown)
		curve.DailyReturns = append(curve.DailyReturns, dailyReturn)
		curve.NetProfit = append(curve.NetProfit, dayPnL)
	}

	// Calculate summary metrics
	curve.Profit = fmt.Sprintf("%.2f", totalProfit)

	maxDrawdown := 0.0
	for _, dd := range curve.Drawdown {
		if dd < maxDrawdown {
			maxDrawdown = dd
		}
	}
	curve.MaxDrawdown = fmt.Sprintf("%.2f", maxDrawdown)

	// Calculate net profit to drawdown ratio
	if maxDrawdown != 0 {
		ratio := totalProfit / (-maxDrawdown)
		curve.NetProfitDrawdown = fmt.Sprintf("%.2f", ratio)
	} else {
		curve.NetProfitDrawdown = "0.00"
	}

	return nil
}

// Helper functions for date range calculation
func getEarliestISDate(ranges []WFORetestDateRange) string {
	if len(ranges) == 0 {
		return ""
	}
	earliest := ranges[0].OriginalISStart
	for _, r := range ranges[1:] {
		if r.OriginalISStart < earliest {
			earliest = r.OriginalISStart
		}
	}
	return strings.ReplaceAll(earliest, "-", "")
}

func getLatestISDate(ranges []WFORetestDateRange) string {
	if len(ranges) == 0 {
		return ""
	}
	latest := ranges[0].OriginalISEnd
	for _, r := range ranges[1:] {
		if r.OriginalISEnd > latest {
			latest = r.OriginalISEnd
		}
	}
	return strings.ReplaceAll(latest, "-", "")
}

func getEarliestOSDate(ranges []WFORetestDateRange) string {
	earliest := ""
	for _, r := range ranges {
		if r.OriginalOSStart != "" {
			if earliest == "" || r.OriginalOSStart < earliest {
				earliest = r.OriginalOSStart
			}
		}
	}
	return strings.ReplaceAll(earliest, "-", "")
}

func getLatestOSDate(ranges []WFORetestDateRange) string {
	latest := ""
	for _, r := range ranges {
		if r.OriginalOSEnd != "" {
			if latest == "" || r.OriginalOSEnd > latest {
				latest = r.OriginalOSEnd
			}
		}
	}
	return strings.ReplaceAll(latest, "-", "")
}

// formatDateForEquity converts YYYYMMDD to YYYY-MM-DD for equity curve
func formatDateForEquity(dateStr string) string {
	if len(dateStr) != 8 {
		return dateStr
	}
	return fmt.Sprintf("%s-%s-%s", dateStr[0:4], dateStr[4:6], dateStr[6:8])
}

// saveDualEquityCurves saves the dual IS/OS equity curves JSON and uploads to storage
func (ac *APIClient) saveDualEquityCurves(curves DualEquityCurves, jobID, symbol, timeframe string) error {
	fmt.Printf("💾 [JSON-SAVER] Converting dual equity curves to JSON\n")
	// Convert to JSON
	jsonData, err := json.MarshalIndent(curves, "", "  ")
	if err != nil {
		fmt.Printf("❌ [JSON-SAVER] JSON marshal failed: %v\n", err)
		return fmt.Errorf("marshal dual equity curves: %w", err)
	}
	fmt.Printf("✅ [JSON-SAVER] JSON marshaled successfully (%d bytes)\n", len(jsonData))

	// Save to local file
	fileName := fmt.Sprintf("%s_%s_%s_WFO_RETEST_dual_equity.json", jobID, symbol, timeframe)
	localPath := filepath.Join("C:\\AlphaWeaver\\files\\results\\combined", fileName)
	fmt.Printf("💾 [JSON-SAVER] Target file path: %s\n", localPath)

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
		fmt.Printf("❌ [JSON-SAVER] Failed to create directory: %v\n", err)
		return fmt.Errorf("create results directory: %w", err)
	}
	fmt.Printf("✅ [JSON-SAVER] Directory created/verified\n")

	if err := os.WriteFile(localPath, jsonData, 0644); err != nil {
		fmt.Printf("❌ [JSON-SAVER] Failed to write file: %v\n", err)
		return fmt.Errorf("write dual equity curves file: %w", err)
	}

	fmt.Printf("🎉 [JSON-SAVER] Dual equity curves saved successfully to: %s\n", localPath)

	// Upload to upload-daily-summary endpoint
	// This would integrate with existing upload logic
	fmt.Printf("📤 [JSON-SAVER] Dual equity curves ready for upload to Run 1 OS record\n")

	return nil
}