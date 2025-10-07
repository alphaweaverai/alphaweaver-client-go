package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

// UI-safe helpers to ensure all widget operations run on the main thread
func (g *GUI) onMain(fn func()) {
	if g.app == nil {
		fn()
		return
	}
	// In Fyne v2.4+, direct execution is safe for UI operations
	fn()
}

func (g *GUI) setLabel(l *widget.Label, text string) {
	g.onMain(func() { l.SetText(text) })
}

func (g *GUI) setLogText(text string) {
	g.onMain(func() { if g.logText != nil { g.logText.SetText(text) } })
}

func (g *GUI) enableButton(b *widget.Button)  { g.onMain(func() { b.Enable() }) }
func (g *GUI) disableButton(b *widget.Button) { g.onMain(func() { b.Disable() }) }

type GUI struct {
	app         fyne.App
	mainWindow  fyne.Window
	config      *Config
	auth        *AuthManager
	api         *APIClient
	downloader  *DownloadManager
	polling     *PollingOptimizer
	fileMgr     *FileManager
	csvUploader *CSVUploadManager
	logger      *Logger
	optUploader *OptUploadManager
	dailySummaryUploader *DailySummaryUploadManager
	wfoCompletionHandler *WFOCompletionHandler

	emailEntry    *widget.Entry
	passwordEntry *widget.Entry
	loginButton   *widget.Button
	logoutButton  *widget.Button
	statusLabel   *widget.Label
	logText       *widget.TextGrid
	logBuffer     string

	daemonButton *widget.Button
	stopButton   *widget.Button
	statsLabel   *widget.Label

	// New UI elements for folder management
	folderStatsLabel *widget.Label

	isPolling      bool
	stopCh         chan bool
	wfoStarted     bool
}

func NewGUI() *GUI {
	g := &GUI{app: app.New()}
	cfg, _ := LoadConfig("") // No config file needed
	g.config = cfg
	g.auth = NewAuthManager(cfg)
	g.api = NewAPIClient(cfg, g.auth)
	g.downloader = NewDownloadManager(cfg, g.api)
	g.polling = NewPollingOptimizer(cfg)
	g.fileMgr = NewFileManager(cfg)
	g.csvUploader = NewCSVUploadManager(cfg, g.api)
	g.optUploader = NewOptUploadManager(cfg, g.api)
	g.dailySummaryUploader = NewDailySummaryUploadManager(g.api, g.fileMgr, cfg)
	g.wfoCompletionHandler = NewWFOCompletionHandler(cfg, g.api)
	// Bridge downloader logs into GUI log
	g.downloader.SetLogger(func(msg string) { g.log(msg) })
	// Bridge opt uploader logs into GUI log
	g.optUploader.SetLogger(func(msg string) { g.log(msg) })
	// Bridge daily summary uploader logs into GUI log
	g.dailySummaryUploader.SetLogger(func(msg string) { g.log(msg) })
	// Bridge CSV uploader logs into GUI log
	g.csvUploader.SetLogger(func(msg string) { g.log(msg) })
	// Bridge polling logs into GUI log
	g.polling.SetLogger(func(msg string) { g.log(msg) })
	// Bridge WFO completion handler logs into GUI log
	g.wfoCompletionHandler.SetLogger(func(msg string) { g.log(msg) })

	// Start upload event monitoring for burst polling
	go g.monitorUploadEvents()

	// Get executable path for logger
	exePath, _ := os.Executable()
	g.logger = NewLogger(filepath.Join(filepath.Dir(exePath), "logs"))
	g.mainWindow = g.app.NewWindow("Alpha Weaver Client v1.0")
	g.mainWindow.Resize(fyne.NewSize(500, 800)) // Reduced width by 50%
	g.mainWindow.CenterOnScreen()               // Center the main window on screen
	g.buildUI()

	// Note: WFO completion monitoring will start after successful login
	// See onLogin() method for WFO monitoring initialization

	return g
}

func (g *GUI) buildUI() {
	// Create main content without margins for full width usage
	content := container.NewVBox(
		g.authSection(),
		widget.NewSeparator(),
		g.daemonSection(),
		widget.NewSeparator(),
		g.folderManagementSection(),
		widget.NewSeparator(),
		g.logSection(),
	)

	// Set content without scroll - only log section will have scroll
	g.mainWindow.SetContent(content)

	// Initialize folder stats
	go g.updateFolderStats()
}

func (g *GUI) authSection() fyne.CanvasObject {
	g.emailEntry = widget.NewEntry()
	g.emailEntry.SetPlaceHolder("Enter your email")
	g.emailEntry.Text = g.config.Auth.Email
	g.passwordEntry = widget.NewPasswordEntry()
	g.passwordEntry.SetPlaceHolder("Enter your password")
	g.loginButton = widget.NewButton("Sign In", g.onLogin)
	g.logoutButton = widget.NewButton("Sign Out", g.onLogout)
	g.logoutButton.Disable()
	g.statusLabel = widget.NewLabel("Not authenticated")

	// Add Enter key support for login
	g.emailEntry.OnSubmitted = func(string) {
		g.passwordEntry.FocusGained()
	}
	g.passwordEntry.OnSubmitted = func(string) {
		g.onLogin()
	}

	// Modern card-style layout
	title := widget.NewLabel("Authentication")
	title.TextStyle = fyne.TextStyle{Bold: true}

	grid := container.NewGridWithColumns(2,
		widget.NewLabel("Email:"), g.emailEntry,
		widget.NewLabel("Password:"), g.passwordEntry,
	)

	buttonRow := container.NewHBox(g.loginButton, g.logoutButton)

	return container.NewVBox(
		title,
		container.NewPadded(grid),
		container.NewPadded(buttonRow),
		container.NewPadded(g.statusLabel),
	)
}

func (g *GUI) daemonSection() fyne.CanvasObject {
	g.daemonButton = widget.NewButton("Start Monitoring", g.onStartDaemon)
	g.daemonButton.Disable()
	g.stopButton = widget.NewButton("Stop Monitoring", g.onStopDaemon)
	g.stopButton.Disable()
	g.statsLabel = widget.NewLabel("No activity yet")

	// Modern card-style layout
	title := widget.NewLabel("Monitoring")
	title.TextStyle = fyne.TextStyle{Bold: true}

	buttonRow := container.NewHBox(g.daemonButton, g.stopButton)

	return container.NewVBox(
		title,
		container.NewPadded(buttonRow),
		container.NewPadded(g.statsLabel),
	)
}

func (g *GUI) folderManagementSection() fyne.CanvasObject {
	g.folderStatsLabel = widget.NewLabel("Loading folder statistics...")
	refreshBtn := widget.NewButton("Refresh", g.onRefreshFolderStats)

	// Modern card-style layout
	title := widget.NewLabel("Folder Status")
	title.TextStyle = fyne.TextStyle{Bold: true}

	return container.NewVBox(
		title,
		container.NewPadded(g.folderStatsLabel),
		container.NewPadded(refreshBtn),
	)
}

func (g *GUI) logSection() fyne.CanvasObject {
	g.logText = widget.NewTextGrid()
	scroll := container.NewScroll(g.logText)
	scroll.SetMinSize(fyne.NewSize(0, 400)) // Increased height for better visibility
	clearBtn := widget.NewButton("Clear Activity Log", func() {
		g.logBuffer = ""
		g.logText.SetText("")
	})

	// Modern card-style layout
	title := widget.NewLabel("Activity Log")
	title.TextStyle = fyne.TextStyle{Bold: true}

	return container.NewVBox(
		title,
		container.NewPadded(scroll),
		container.NewPadded(clearBtn),
	)
}

func (g *GUI) onLogin() {
	email, pass := g.emailEntry.Text, g.passwordEntry.Text
	if email == "" || pass == "" {
		g.log("Error: enter email and password")
		g.statusLabel.SetText("Login failed: missing email or password")
		dialog.ShowError(fmt.Errorf("Please enter email and password"), g.mainWindow)
		return
	}
	g.loginButton.Disable()
	g.setLabel(g.statusLabel, "Authenticating...")
	go func() {
		if err := g.auth.Authenticate(email, pass); err != nil {
			msg := fmt.Sprintf("Login failed: %v", err)
			g.log(msg)
			g.setLabel(g.statusLabel, msg)
			g.showModernErrorDialog("Authentication Failed", err.Error())
			g.enableButton(g.loginButton)
			return
		}
		if err := g.api.TestConnection(); err != nil {
			msg := fmt.Sprintf("Connection test failed: %v", err)
			g.log(msg)
			g.setLabel(g.statusLabel, msg)
			g.showModernErrorDialog("Connection Failed", err.Error())
			g.enableButton(g.loginButton)
			return
		}
		g.setLabel(g.statusLabel, "✅ Authenticated - Ready to monitor")
		g.disableButton(g.loginButton)
		g.enableButton(g.logoutButton)
		g.enableButton(g.daemonButton)
		g.log("Authentication successful - monitoring features enabled")

		// Start WFO completion monitoring now that user is authenticated (only once)
		if !g.wfoStarted {
			g.wfoStarted = true
			go g.wfoCompletionHandler.StartWFOCompletionMonitoring()
		}
	}()
}

func (g *GUI) onLogout() {
	g.auth.Logout()
	g.statusLabel.SetText("Not authenticated")
	g.loginButton.Enable()
	g.logoutButton.Disable()
	g.daemonButton.Disable()
	g.stopButton.Disable()
	g.isPolling = false
	g.log("Logged out")
}

func (g *GUI) onStartDaemon() {
	if g.isPolling {
		g.log("Monitoring already running")
		return
	}
	if err := g.auth.EnsureValidToken(); err != nil {
		g.log(fmt.Sprintf("Error: authentication failed - %v", err))
		return
	}
	// Use default limit from config (server-controlled)
	limit := g.config.Poll.Limit
	g.isPolling = true
	g.stopCh = make(chan bool)
	g.disableButton(g.daemonButton)
	g.enableButton(g.stopButton)
	g.log("Starting job and CSV monitoring...")

	// Start polling and all upload monitoring
	go g.runDaemon(limit)
	go g.startCSVMonitoring()
	go g.startOptMonitoring()
	// Daily summary uploads for RETEST are now coupled to OPT upload; independent monitoring disabled
}

func (g *GUI) onStopDaemon() {
	if !g.isPolling {
		return
	}
	g.isPolling = false
	close(g.stopCh)
	g.enableButton(g.daemonButton)
	g.disableButton(g.stopButton)

	// Stop CSV upload monitoring
	g.csvUploader.Stop()
	// Stop OPT upload monitoring
	g.optUploader.Stop()
	// Stop daily summary upload monitoring
	g.dailySummaryUploader.Stop()

	g.log("Monitoring stopped")
}

func (g *GUI) startCSVMonitoring() {
	if err := g.csvUploader.Start(); err != nil {
		g.log(fmt.Sprintf("Failed to start CSV monitoring: %v", err))
		return
	}
	g.log("CSV monitoring started")
}

func (g *GUI) startOptMonitoring() {
	if err := g.optUploader.Start(); err != nil {
		g.log(fmt.Sprintf("Failed to start OPT monitoring: %v", err))
		return
	}
	g.log("OPT monitoring started")
}

func (g *GUI) startDailySummaryMonitoring() {
	if err := g.dailySummaryUploader.Start(); err != nil {
		g.log(fmt.Sprintf("Failed to start daily summary monitoring: %v", err))
		return
	}
	g.log("Daily summary monitoring started")
}

func (g *GUI) runDaemon(limit int) {
	iter := 0
	remaining := 0
	for g.isPolling {
		select {
		case <-g.stopCh:
			return
		default:
		}
		iter++
		g.log(fmt.Sprintf("Daemon iteration %d", iter))
		if err := g.auth.EnsureValidToken(); err != nil {
			g.log(fmt.Sprintf("Token refresh failed: %v", err))
			time.Sleep(30 * time.Second)
			continue
		}
		resp, err := g.api.PollJobs(limit)
		if err != nil {
			g.log(fmt.Sprintf("Poll failed: %v", err))
			time.Sleep(30 * time.Second)
			continue
		}
		if len(resp.Jobs) > 0 {
			remaining = max(0, remaining-len(resp.Jobs))
			g.log(fmt.Sprintf("Remaining jobs: %d", remaining))
		} else {
			remaining = 0
			g.log("Remaining jobs: 0 (no jobs found)")
		}
		if len(resp.Jobs) > 0 {
			g.log(fmt.Sprintf("Downloading %d jobs...", len(resp.Jobs)))
			stats := g.downloader.DownloadJobs(resp.Jobs)
			g.log(fmt.Sprintf("Download complete: %d successful, %d failed", stats.Successful, stats.Failed))
			g.updateFolderStats()
		}
		g.polling.UpdateMetrics(len(resp.Jobs))
		next := g.polling.CalculateOptimalInterval(len(resp.Jobs) > 0, remaining, g.config.Folders.Files.Jobs.ToDo)
		g.log(g.polling.LogPollingDecision(len(resp.Jobs) > 0, next, remaining))
		cnt, size, _ := g.downloader.GetDownloadStats()
	g.setLabel(g.statsLabel, fmt.Sprintf("Files: %d, Size: %s", cnt, FormatFileSize(size)))

		// If next interval is 0, stop polling and wait for user to resume
		if next == 0 {
			g.log("Polling stopped - waiting for downloads folder to have ≤3 jobs")
			// Wait for user to stop or for downloads folder to change
			select {
			case <-g.stopCh:
				return
			case <-time.After(30 * time.Second): // Check every 30 seconds if we can resume polling
				continue
			}
		}

		g.log(fmt.Sprintf("Waiting %s...", FormatDuration(next)))
		select {
		case <-time.After(next):
			continue
		case <-g.stopCh:
			return
		case <-g.polling.GetBurstPollChannel():
			g.log("Burst poll triggered - checking for jobs immediately")
			continue
		}
	}
}

func (g *GUI) onRefreshFolderStats() {
	go func() {
		g.updateFolderStats()
	}()
}

func (g *GUI) showModernSuccessDialog(title, message string) {
	g.onMain(func() {
		// Create a custom modern dialog
		dialogWindow := g.app.NewWindow(title)
		dialogWindow.Resize(fyne.NewSize(400, 200))
		dialogWindow.CenterOnScreen()

		// Create content with modern styling
		titleLabel := widget.NewLabel(title)
		titleLabel.TextStyle = fyne.TextStyle{Bold: true}
		titleLabel.Alignment = fyne.TextAlignCenter

		messageLabel := widget.NewLabel(message)
		messageLabel.Alignment = fyne.TextAlignCenter
		messageLabel.Wrapping = fyne.TextWrapWord

		okButton := widget.NewButton("Continue", func() { dialogWindow.Close() })

		// Layout with proper spacing and centered button
		content := container.NewVBox(
			container.NewPadded(titleLabel),
			container.NewPadded(messageLabel),
			container.NewPadded(container.NewCenter(okButton)),
		)

		dialogWindow.SetContent(content)
		dialogWindow.Show()
	})
}

func (g *GUI) showModernErrorDialog(title, message string) {
	g.onMain(func() {
		// Create a custom modern error dialog
		dialogWindow := g.app.NewWindow(title)
		dialogWindow.Resize(fyne.NewSize(400, 200))
		dialogWindow.CenterOnScreen()

		// Create content with modern styling
		titleLabel := widget.NewLabel(title)
		titleLabel.TextStyle = fyne.TextStyle{Bold: true}
		titleLabel.Alignment = fyne.TextAlignCenter

		messageLabel := widget.NewLabel(message)
		messageLabel.Alignment = fyne.TextAlignCenter
		messageLabel.Wrapping = fyne.TextWrapWord

		okButton := widget.NewButton("OK", func() { dialogWindow.Close() })

		// Layout with proper spacing and centered button
		content := container.NewVBox(
			container.NewPadded(titleLabel),
			container.NewPadded(messageLabel),
			container.NewPadded(container.NewCenter(okButton)),
		)

		dialogWindow.SetContent(content)
		dialogWindow.Show()
	})
}

func (g *GUI) updateFolderStats() {
	// Get job file counts
	toDoJobs, _ := g.fileMgr.GetJobFiles("to_do")
	inProgressJobs, _ := g.fileMgr.GetJobFiles("in_progress")
	doneJobs, _ := g.fileMgr.GetJobFiles("done")
	errorJobs, _ := g.fileMgr.GetJobFiles("error")

	// Get CSV file counts
	toDoCSV, doneCSV, _ := g.csvUploader.GetUploadStats()

	// Get OPT file counts
	optInCount := 0
	optDoneCount := 0
	if entries, err := os.ReadDir(g.config.Folders.Files.Opt.In); err == nil {
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".opt") {
				optInCount++
			}
		}
	}
	if entries, err := os.ReadDir(g.config.Folders.Files.Opt.Done); err == nil {
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".opt") {
				optDoneCount++
			}
		}
	}

	stats := fmt.Sprintf("Jobs - To Do: %d, In Progress: %d, Done: %d, Error: %d\nCSV - To Do: %d, Done: %d\nOPT - In: %d, Done: %d",
		len(toDoJobs), len(inProgressJobs), len(doneJobs), len(errorJobs), toDoCSV, doneCSV, optInCount, optDoneCount)

	g.setLabel(g.folderStatsLabel, stats)
}

func (g *GUI) log(msg string) {
	ts := time.Now().Format("15:04:05")
	line := fmt.Sprintf("[%s] %s\n", ts, msg)
	cur := g.logBuffer
	if len(cur) > 10000 {
		parts := strings.Split(cur, "\n")
		if len(parts) > 100 {
			cur = strings.Join(parts[len(parts)-100:], "\n")
		}
	}
	g.logBuffer = cur + line
	// Safety check: only update GUI if logText is initialized
	g.setLogText(g.logBuffer)

	// Also write to daily log file
	if g.logger != nil {
		g.logger.Info(msg)
	}
}

// monitorUploadEvents monitors upload events for burst polling
func (g *GUI) monitorUploadEvents() {
	for event := range UploadEventChan {
		go g.polling.HandleUploadEvent(event)
	}
}

func (g *GUI) Run() { g.mainWindow.ShowAndRun() }

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
