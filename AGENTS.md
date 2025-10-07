# AGENTS.md

Scope: Go-based ClientGUI app for local job download, orchestration, and result handling.

What this folder owns
- Downloading job XML/JO B files from Supabase APIs
- Generating WFO_RETEST XML, wrapping XML with <root> tags (required by TSClient)
- Uploading results (CSV/JSON) back to Supabase functions
- GUI and background polling / processing

Build (Windows)
- go mod tidy
- go vet ./...
- go test ./...           # where tests exist
- go build -ldflags "-H windowsgui" -o alpha-weaver-gui.exe

Critical implementation rules
- XML wrapping: Do not remove the <root>...</root> wrapping used in DownloadFile (required for TS client)
- Filename conventions: Keep in sync with TSClient (WFO_RETEST naming, MM consolidated outputs)
- Polling: Use adaptive intervals (longer when idle, shorter when active) per config
- Storage: Use local folders for in_progress/to_do/completed/error in line with TSClient expectations
- Config & logging: Use config.go and logger.go; avoid hardcoded paths
- task_type is authoritative; never branch on stage
- Logging standards (from senior-developer): use function separators, step numbering, and parameter logging in development builds; avoid leaking PII and secrets

Impact analysis checklist
- Job XML: any structural change requires updates in both poll-jobs and download-job-xml functions
- WFO_RETEST: verify OS percentage extraction from original XML remains correct
- Performance: UI responsiveness, goroutine lifecycle, cleanup of temp files
- File paths: compatible with TSClient watchers and supabase upload functions
- Security: ensure HTTPS endpoints; do not log secrets
- Rollback: preserve previous binary and config for fallback

PR checklist
- [ ] Build passes with -H windowsgui
- [ ] XML root wrapping intact
- [ ] task_type routing intact
- [ ] Polling intervals documented
- [ ] Filenames/paths synchronized with TSClient & supabase
- [ ] Logging is structured and free of secrets/PII
