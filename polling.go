package main

import (
	"fmt"
	"os"
	"strings"
	"time"
)

type PollingOptimizer struct {
	config                *Config
	consecutiveEmptyPolls int
	currentInterval       time.Duration
	totalPolls            int
	successfulPolls       int
	burstPollChan         chan bool
	logf                  func(string)
}

func NewPollingOptimizer(cfg *Config) *PollingOptimizer {
	return &PollingOptimizer{
		config:          cfg,
		currentInterval: cfg.GetPollInterval(),
		burstPollChan:   make(chan bool, 1),
		logf:            func(string) {}, // Default no-op logger
	}
}

// SetLogger sets the logging function
func (po *PollingOptimizer) SetLogger(fn func(string)) {
	if fn != nil {
		po.logf = fn
	}
}

// CalculateOptimalInterval returns the optimal polling interval based on the requirements:
// - If more than 3 jobs exist in Files/Jobs/To Do folder, return 0 (stop polling)
// - If jobs are available and remaining jobs ≤ 3, return 5 minutes
// - If 3 consecutive empty polls, return 30 minutes
// - Otherwise, use adaptive logic
func (po *PollingOptimizer) CalculateOptimalInterval(hasJobs bool, remaining int, downloadsFolder string) time.Duration {
	// Check Files/Jobs/To Do folder first - if more than 3 jobs exist, stop polling
	jobCount, err := po.countJobsInNewFolder()
	if err == nil && jobCount > po.config.Poll.RemainingJobsThreshold {
		// More than 3 jobs in To Do folder - stop polling
		return 0
	}

	// If jobs are available and remaining jobs ≤ 3, poll every 5 minutes
	if hasJobs && remaining <= po.config.Poll.RemainingJobsThreshold {
		return po.config.GetMinPollInterval() // 5 minutes
	}

	// If 3 consecutive empty polls, poll every 30 minutes
	if po.consecutiveEmptyPolls >= 3 {
		return po.config.GetMaxPollInterval() // 30 minutes
	}

	// Default adaptive logic for other cases
	interval := po.currentInterval

	if po.config.Poll.Adaptive {
		interval = po.calculateAdaptiveInterval(hasJobs, remaining)
	}

	// Ensure interval is within bounds
	if interval < po.config.GetMinPollInterval() {
		interval = po.config.GetMinPollInterval()
	}
	if interval > po.config.GetMaxPollInterval() {
		interval = po.config.GetMaxPollInterval()
	}

	po.currentInterval = interval
	return interval
}

func (po *PollingOptimizer) calculateAdaptiveInterval(hasJobs bool, remaining int) time.Duration {
	// If jobs are available and remaining jobs ≤ 3, poll every 5 minutes
	if hasJobs && remaining <= po.config.Poll.RemainingJobsThreshold {
		return po.config.GetMinPollInterval()
	}

	// If jobs are found, reduce interval (but not below minimum)
	if hasJobs {
		half := po.currentInterval / 2
		if half < po.config.GetMinPollInterval() {
			return po.config.GetMinPollInterval()
		}
		return half
	}

	// If no jobs, gradually increase interval (but not above maximum)
	more := time.Duration(float64(po.currentInterval) * 1.2)
	if more > po.config.GetMaxPollInterval() {
		return po.config.GetMaxPollInterval()
	}
	return more
}

func (po *PollingOptimizer) UpdateMetrics(jobCount int) {
	po.totalPolls++
	if jobCount > 0 {
		po.successfulPolls++
		po.consecutiveEmptyPolls = 0
	} else {
		po.consecutiveEmptyPolls++
	}
}

func (po *PollingOptimizer) LogPollingDecision(hasJobs bool, interval time.Duration, remaining int) string {
	reasons := []string{}

	// Check Files/Jobs/To Do folder first
	jobCount, err := po.countJobsInNewFolder()
	if err == nil && jobCount > po.config.Poll.RemainingJobsThreshold {
		reasons = append(reasons, fmt.Sprintf("Files/Jobs/To Do folder has %d jobs (>3) - stopping polling", jobCount))
	}

	// If jobs are available and remaining jobs ≤ 3, poll every 5 minutes
	if hasJobs && remaining <= po.config.Poll.RemainingJobsThreshold {
		reasons = append(reasons, fmt.Sprintf("Jobs available and remaining ≤3: %d - polling every 5 minutes", remaining))
	}

	// If 3 consecutive empty polls, poll every 30 minutes
	if po.consecutiveEmptyPolls >= 3 {
		reasons = append(reasons, fmt.Sprintf("3+ consecutive empty polls: %d - polling every 30 minutes", po.consecutiveEmptyPolls))
	}

	// Default adaptive logic
	if po.config.Poll.Adaptive && len(reasons) == 0 {
		if hasJobs {
			reasons = append(reasons, "Adaptive: jobs found")
		} else {
			reasons = append(reasons, "Adaptive: no jobs")
		}
	}

	why := "default"
	if len(reasons) > 0 {
		why = reasons[0]
		for i := 1; i < len(reasons); i++ {
			why += ", " + reasons[i]
		}
	}

	if interval == 0 {
		return fmt.Sprintf("Polling stopped (%s)", why)
	}
	return fmt.Sprintf("Polling interval: %s (%s)", FormatDuration(interval), why)
}

// countJobsInFolder counts the number of job files in the downloads folder
func (po *PollingOptimizer) countJobsInFolder(folderPath string) (int, error) {
	entries, err := os.ReadDir(folderPath)
	if err != nil {
		return 0, err
	}

	count := 0
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".job") {
			count++
		}
	}
	return count, nil
}

// countJobsInNewFolder counts the number of job files in the Files/Jobs/To Do folder
func (po *PollingOptimizer) countJobsInNewFolder() (int, error) {
	return po.countJobsInFolder(po.config.Folders.Files.Jobs.ToDo)
}

// HandleUploadEvent handles upload events for burst polling
func (po *PollingOptimizer) HandleUploadEvent(event UploadEvent) {
	if !po.config.BurstPolling.Enabled {
		return
	}

	// Check if this event type should trigger burst polling
	if event.EventType == "opt_upload" && !po.config.BurstPolling.EnableOptTrigger {
		return
	}
	if event.EventType == "daily_summary_upload" && !po.config.BurstPolling.EnableSummaryTrigger {
		return
	}

	po.logf(fmt.Sprintf("Upload event received: %s for job %s", event.EventType, event.JobID))

	// Wait for server-side job creation
	time.Sleep(po.config.BurstPolling.WaitAfterUpload)

	// Check job count threshold
	jobCount, err := po.countJobsInNewFolder()
	if err != nil {
		po.logf(fmt.Sprintf("Error checking job count for burst polling: %v", err))
		return
	}

	if jobCount <= po.config.BurstPolling.JobThreshold {
		po.logf(fmt.Sprintf("Job count (%d) ≤ threshold (%d), triggering burst poll",
			jobCount, po.config.BurstPolling.JobThreshold))
		po.triggerBurstPoll()
	} else {
		po.logf(fmt.Sprintf("Job count (%d) > threshold (%d), skipping burst poll",
			jobCount, po.config.BurstPolling.JobThreshold))
	}
}

// triggerBurstPoll triggers an immediate poll
func (po *PollingOptimizer) triggerBurstPoll() {
	select {
	case po.burstPollChan <- true:
		po.logf("Burst poll triggered successfully")
	default:
		po.logf("Burst poll channel full, skipping")
	}
}

// GetBurstPollChannel returns the burst poll channel for the main polling loop
func (po *PollingOptimizer) GetBurstPollChannel() <-chan bool {
	return po.burstPollChan
}
