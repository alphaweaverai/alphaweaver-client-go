package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Logger handles daily log file writing
type Logger struct {
	logDir string
}

// NewLogger creates a new logger instance
func NewLogger(logDir string) *Logger {
	return &Logger{
		logDir: logDir,
	}
}

// getLogFileName returns the log file name for today's date
func (l *Logger) getLogFileName() string {
	today := time.Now().Format("2006-01-02")
	return filepath.Join(l.logDir, fmt.Sprintf("client_%s.log", today))
}

// Log writes a message to today's log file
func (l *Logger) Log(level, message string) error {
	logFile := l.getLogFileName()

	// Ensure log directory exists
	if err := os.MkdirAll(l.logDir, 0755); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}

	// Format the log entry
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	logEntry := fmt.Sprintf("[%s] %s: %s\n", timestamp, level, message)

	// Append to log file
	file, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}
	defer file.Close()

	if _, err := file.WriteString(logEntry); err != nil {
		return fmt.Errorf("failed to write to log file: %w", err)
	}

	return nil
}

// Info logs an info level message
func (l *Logger) Info(message string) error {
	return l.Log("INFO", message)
}

// Error logs an error level message
func (l *Logger) Error(message string) error {
	return l.Log("ERROR", message)
}

// Warning logs a warning level message
func (l *Logger) Warning(message string) error {
	return l.Log("WARNING", message)
}

// Debug logs a debug level message
func (l *Logger) Debug(message string) error {
	return l.Log("DEBUG", message)
}
