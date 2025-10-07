package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"time"
)

// Config holds all configuration settings
type Config struct {
	Supabase     SupabaseConfig     `json:"supabase"`
	Auth         AuthConfig         `json:"auth"`
	Download     DownloadConfig     `json:"download"`
	Poll         PollConfig         `json:"poll"`
	BurstPolling BurstPollingConfig `json:"burst_polling"`
	Logging      LoggingConfig      `json:"logging"`
	Folders      FolderConfig       `json:"folders"`
}

// SupabaseConfig holds Supabase connection settings
type SupabaseConfig struct {
	URL       string `json:"url"`
	AnonKey   string `json:"anon_key"`
	ProjectID string `json:"project_id"`
}

// AuthConfig holds authentication settings
type AuthConfig struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// DownloadConfig holds download settings
type DownloadConfig struct {
	Folder        string `json:"folder"`
	MaxConcurrent int    `json:"max_concurrent"`
	RetryAttempts int    `json:"retry_attempts"`
	RetryDelay    int    `json:"retry_delay"`
	UploadFolder  string `json:"upload_folder"`
}

// PollConfig holds polling settings
type PollConfig struct {
	Limit                  int                      `json:"limit"`
	Interval               int                      `json:"interval"`
	MaxInterval            int                      `json:"max_interval"`
	MinInterval            int                      `json:"min_interval"`
	MaxJobs                int                      `json:"max_jobs"`
	Adaptive               bool                     `json:"adaptive"`
	RemainingJobsThreshold int                      `json:"remaining_jobs_threshold"`
	ExponentialBackoff     ExponentialBackoffConfig `json:"exponential_backoff"`
}

// ExponentialBackoffConfig holds exponential backoff settings
type ExponentialBackoffConfig struct {
	Enabled       bool    `json:"enabled"`
	Factor        float64 `json:"factor"`
	MaxEmptyPolls int     `json:"max_empty_polls"`
}

// BurstPollingConfig holds burst polling settings
type BurstPollingConfig struct {
	Enabled              bool          `json:"enabled"`                // Enable burst polling
	WaitAfterUpload      time.Duration `json:"wait_after_upload"`      // Wait time after upload (30s)
	JobThreshold         int           `json:"job_threshold"`          // Job count threshold to trigger burst
	BurstPollTimeout     time.Duration `json:"burst_poll_timeout"`     // Max time for burst poll
	EnableOptTrigger     bool          `json:"enable_opt_trigger"`     // Enable for OPT uploads
	EnableSummaryTrigger bool          `json:"enable_summary_trigger"` // Enable for summary uploads
}

// LoggingConfig holds logging settings
type LoggingConfig struct {
	Level string `json:"level"`
	File  string `json:"file"`
}

// FolderConfig holds the new folder structure settings
type FolderConfig struct {
	Files FilesConfig `json:"files"`
}

// FilesConfig holds job and results folder paths
type FilesConfig struct {
	Jobs    JobsConfig    `json:"jobs"`
	Results ResultsConfig `json:"results"`
	Opt     OptConfig     `json:"opt"`
}

// JobsConfig holds job status folder paths
type JobsConfig struct {
	ToDo       string `json:"to_do"`
	InProgress string `json:"in_progress"`
	Done       string `json:"done"`
	Error      string `json:"error"`
}

// ResultsConfig holds result-related folder paths
type ResultsConfig struct {
	Temp   string `json:"temp"`
	CSV    string `json:"csv"`
	ToDo   string `json:"to_do"`
	Done   string `json:"done"`
	Trades string `json:"trades"`
}

// OptConfig holds optimization artifact folder paths (.opt files)
type OptConfig struct {
	In      string `json:"in"`
	Done    string `json:"done"`
	Error   string `json:"error"`   // Failed uploads
	Summary string `json:"summary"`  // Daily summary JSON files from TSClient
}

// DefaultConfig returns the default configuration
func DefaultConfig() *Config {
	exePath, _ := os.Executable()
	exeDir := filepath.Dir(exePath)

	// Base root for folders: use TS Client locations on Windows
	baseRoot := filepath.Join(exeDir, "files")
	if runtime.GOOS == "windows" {
		baseRoot = `C:\\AlphaWeaver\\Files`
	}

	return &Config{
		Supabase: SupabaseConfig{
			URL:       "https://rnatsdjhwquhavnnybck.supabase.co",
			AnonKey:   "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc3MiOiJzdXBhYmFzZSIsInJlZiI6InJuYXRzZGpod3F1aGF2bm55YmNrIiwicm9sZSI6ImFub24iLCJpYXQiOjE3NTA5MTQ1OTksImV4cCI6MjA2NjQ5MDU5OX0.bvhOWAd0FBgkpof90SECewkl8pgOOeJyztTdrYqa2G8",
			ProjectID: "rnatsdjhwquhavnnybck",
		},
		Auth: AuthConfig{
			Email:    "",
			Password: "",
		},
		Download: DownloadConfig{
			Folder:        filepath.Join(baseRoot, "jobs", "to_do"),
			MaxConcurrent: 3,
			RetryAttempts: 3,
			RetryDelay:    1000,
			UploadFolder:  filepath.Join(baseRoot, "results", "to_do"),
		},
		Poll: PollConfig{
			Limit:                  10,
			Interval:               300000,  // 5 minutes
			MaxInterval:            1800000, // 30 minutes
			MinInterval:            300000,  // 5 minutes
			MaxJobs:                50,
			Adaptive:               true,
			RemainingJobsThreshold: 3,
			ExponentialBackoff: ExponentialBackoffConfig{
				Enabled:       true,
				Factor:        1.5,
				MaxEmptyPolls: 3,
			},
		},
		BurstPolling: BurstPollingConfig{
			Enabled:              true,
			WaitAfterUpload:      30 * time.Second,
			JobThreshold:         3,
			BurstPollTimeout:     60 * time.Second,
			EnableOptTrigger:     true,
			EnableSummaryTrigger: true,
		},
		Logging: LoggingConfig{
			Level: "info",
			File:  filepath.Join(exeDir, "logs", "client.log"),
		},
		Folders: FolderConfig{
			Files: FilesConfig{
				Jobs: JobsConfig{
					ToDo:       filepath.Join(baseRoot, "jobs", "to_do"),
					InProgress: filepath.Join(baseRoot, "jobs", "in_progress"),
					Done:       filepath.Join(baseRoot, "jobs", "done"),
					Error:      filepath.Join(baseRoot, "jobs", "error"),
				},
				Results: ResultsConfig{
					Temp:   filepath.Join(baseRoot, "results", "temp"),
					CSV:    filepath.Join(baseRoot, "results", "csv"),
					ToDo:   filepath.Join(baseRoot, "results", "to_do"),
					Done:   filepath.Join(baseRoot, "results", "done"),
					Trades: filepath.Join(baseRoot, "results", "trades"),
				},
				Opt: OptConfig{
					In:      filepath.Join(baseRoot, "opt", "in"),
					Done:    filepath.Join(baseRoot, "opt", "done"),
					Error:   filepath.Join(baseRoot, "opt", "error"),  // Failed uploads
					Summary: filepath.Join(baseRoot, "opt", "summary"),  // Daily summary JSON files
				},
			},
		},
	}
}

// LoadConfig loads hardcoded configuration (no config file)
func LoadConfig(configPath string) (*Config, error) {
	cfg := DefaultConfig()

	if err := cfg.EnsureDirectories(); err != nil {
		return nil, fmt.Errorf("failed to create directories: %w", err)
	}

	return cfg, nil
}

// SaveConfig is deprecated - configuration is now hardcoded
func SaveConfig(cfg *Config, configPath string) error {
	return fmt.Errorf("configuration is hardcoded and cannot be saved")
}

// EnsureDirectories creates necessary directories
func (c *Config) EnsureDirectories() error {
	dirs := []string{
		filepath.Dir(c.Logging.File),
		// New folder structure
		c.Folders.Files.Jobs.ToDo,
		c.Folders.Files.Jobs.InProgress,
		c.Folders.Files.Jobs.Done,
		c.Folders.Files.Jobs.Error,
		c.Folders.Files.Results.Temp,
		c.Folders.Files.Results.CSV,
		c.Folders.Files.Results.ToDo,
		c.Folders.Files.Results.Done,
		c.Folders.Files.Results.Trades,
		c.Folders.Files.Opt.In,
		c.Folders.Files.Opt.Done,
		c.Folders.Files.Opt.Error,
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", d, err)
		}
	}
	return nil
}

// GetPollInterval returns the current polling interval
func (c *Config) GetPollInterval() time.Duration {
	return time.Duration(c.Poll.Interval) * time.Millisecond
}

// GetMaxPollInterval returns the maximum polling interval
func (c *Config) GetMaxPollInterval() time.Duration {
	return time.Duration(c.Poll.MaxInterval) * time.Millisecond
}

// GetMinPollInterval returns the minimum polling interval
func (c *Config) GetMinPollInterval() time.Duration {
	return time.Duration(c.Poll.MinInterval) * time.Millisecond
}

// GetRetryDelay returns the retry delay
func (c *Config) GetRetryDelay() time.Duration {
	return time.Duration(c.Download.RetryDelay) * time.Millisecond
}

// FormatDuration formats duration for display
func FormatDuration(d time.Duration) string {
	s := int(d.Seconds())
	if s < 60 {
		return fmt.Sprintf("%ds", s)
	}
	if s < 3600 {
		return fmt.Sprintf("%dm", s/60)
	}
	return fmt.Sprintf("%dh", s/3600)
}

// ParseDuration parses duration like 5m, 30s, 1h
func ParseDuration(s string) (time.Duration, error) {
	if s == "" {
		return 0, fmt.Errorf("empty duration")
	}
	last := s[len(s)-1]
	valStr := s[:len(s)-1]
	v, err := strconv.Atoi(valStr)
	if err != nil {
		return 0, fmt.Errorf("invalid value: %s", valStr)
	}
	switch last {
	case 's':
		return time.Duration(v) * time.Second, nil
	case 'm':
		return time.Duration(v) * time.Minute, nil
	case 'h':
		return time.Duration(v) * time.Hour, nil
	default:
		return 0, fmt.Errorf("invalid unit: %c", last)
	}
}
