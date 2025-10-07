package main

import (
	"fmt"
)

// DateBuffer represents calculated buffer dates for WFO_RETEST processing
type DateBuffer struct {
	OriginalStart string // Original date from WFO job
	OriginalEnd   string // Original date from WFO job
	BufferedStart string // Start date with buffer applied
	BufferedEnd   string // End date with buffer applied
	BufferDays    int    // Number of buffer days calculated
}

// WFORetestDateRange contains both original and buffered dates for a WFO run
type WFORetestDateRange struct {
	// Original dates from WFO job (for trade filtering)
	OriginalISStart string
	OriginalISEnd   string
	OriginalOSStart string
	OriginalOSEnd   string

	// Buffered dates (for TSClient processing)
	BufferedISStart string
	BufferedISEnd   string
	BufferedOSStart string
	BufferedOSEnd   string

	// Buffer metadata
	ISBufferDays int
	OSBufferDays int
}

// calculateDateBuffer implements 0.25% duration buffer calculation
// This ensures proper trade capture at period boundaries for TSClient processing
func calculateDateBuffer(startDate, endDate string) (*DateBuffer, error) {
	// Parse input dates
	start, err := parseDate(startDate)
	if err != nil {
		return nil, fmt.Errorf("parse start date '%s': %w", startDate, err)
	}

	end, err := parseDate(endDate)
	if err != nil {
		return nil, fmt.Errorf("parse end date '%s': %w", endDate, err)
	}

	// Calculate total duration in days
	duration := end.Sub(start)
	totalDays := int(duration.Hours() / 24)

	// Calculate 0.25% buffer (minimum 1 day, maximum 30 days)
	bufferDays := int(float64(totalDays) * 0.0025)
	if bufferDays < 1 {
		bufferDays = 1 // Minimum buffer of 1 day
	}
	if bufferDays > 30 {
		bufferDays = 30 // Maximum buffer of 30 days to prevent excessive data processing
	}

	// Apply buffer to dates
	bufferedStart := start.AddDate(0, 0, -bufferDays)
	bufferedEnd := end.AddDate(0, 0, bufferDays)

	fmt.Printf("[DEBUG] Date buffer calculation: %s to %s (%d days) -> buffer: %d days\n",
		startDate, endDate, totalDays, bufferDays)
	fmt.Printf("[DEBUG] Buffered range: %s to %s\n",
		formatDate(bufferedStart), formatDate(bufferedEnd))

	return &DateBuffer{
		OriginalStart: startDate,
		OriginalEnd:   endDate,
		BufferedStart: formatDate(bufferedStart),
		BufferedEnd:   formatDate(bufferedEnd),
		BufferDays:    bufferDays,
	}, nil
}

// calculateWFORetestDateRanges processes all WFO runs and applies date buffers
// while preserving original date ranges for trade filtering
func calculateWFORetestDateRanges(optResults []OPTResult) ([]WFORetestDateRange, error) {
	var retestRanges []WFORetestDateRange

	for i, result := range optResults {
		fmt.Printf("[DEBUG] Processing WFO_RETEST date ranges for run %d\n", i+1)

		// Calculate IS period buffer
		isBuffer, err := calculateDateBuffer(result.ISStartDate, result.ISEndDate)
		if err != nil {
			return nil, fmt.Errorf("calculate IS buffer for run %d: %w", i+1, err)
		}

		// Calculate OS period buffer (if OS dates exist)
		var osBuffer *DateBuffer
		if result.OSStartDate != "" && result.OSEndDate != "" {
			osBuffer, err = calculateDateBuffer(result.OSStartDate, result.OSEndDate)
			if err != nil {
				return nil, fmt.Errorf("calculate OS buffer for run %d: %w", i+1, err)
			}
		}

		// Create WFO_RETEST date range
		retestRange := WFORetestDateRange{
			// Preserve original dates for trade filtering
			OriginalISStart: result.ISStartDate,
			OriginalISEnd:   result.ISEndDate,
			OriginalOSStart: result.OSStartDate,
			OriginalOSEnd:   result.OSEndDate,

			// Apply buffers for TSClient processing
			BufferedISStart: isBuffer.BufferedStart,
			BufferedISEnd:   isBuffer.BufferedEnd,
			ISBufferDays:    isBuffer.BufferDays,
		}

		// Add OS buffer if OS period exists
		if osBuffer != nil {
			retestRange.BufferedOSStart = osBuffer.BufferedStart
			retestRange.BufferedOSEnd = osBuffer.BufferedEnd
			retestRange.OSBufferDays = osBuffer.BufferDays
		}

		retestRanges = append(retestRanges, retestRange)

		fmt.Printf("[DEBUG] Run %d - IS: %s to %s (buffer: ±%d days)\n",
			i+1, result.ISStartDate, result.ISEndDate, isBuffer.BufferDays)
		if osBuffer != nil {
			fmt.Printf("[DEBUG] Run %d - OS: %s to %s (buffer: ±%d days)\n",
				i+1, result.OSStartDate, result.OSEndDate, osBuffer.BufferDays)
		}
	}

	return retestRanges, nil
}

// validateDateRanges ensures calculated date ranges are logically correct
func validateDateRanges(ranges []WFORetestDateRange) error {
	for i, r := range ranges {
		// Validate IS dates
		isStart, err := parseDate(r.OriginalISStart)
		if err != nil {
			return fmt.Errorf("invalid IS start date for run %d: %w", i+1, err)
		}

		isEnd, err := parseDate(r.OriginalISEnd)
		if err != nil {
			return fmt.Errorf("invalid IS end date for run %d: %w", i+1, err)
		}

		if isEnd.Before(isStart) {
			return fmt.Errorf("IS end date before start date for run %d", i+1)
		}

		// Validate OS dates if they exist
		if r.OriginalOSStart != "" && r.OriginalOSEnd != "" {
			osStart, err := parseDate(r.OriginalOSStart)
			if err != nil {
				return fmt.Errorf("invalid OS start date for run %d: %w", i+1, err)
			}

			osEnd, err := parseDate(r.OriginalOSEnd)
			if err != nil {
				return fmt.Errorf("invalid OS end date for run %d: %w", i+1, err)
			}

			if osEnd.Before(osStart) {
				return fmt.Errorf("OS end date before start date for run %d", i+1)
			}

			// OS should start after IS ends
			if osStart.Before(isEnd) {
				return fmt.Errorf("OS start date before IS end date for run %d", i+1)
			}
		}
	}

	fmt.Printf("[DEBUG] Validated %d WFO_RETEST date ranges successfully\n", len(ranges))
	return nil
}