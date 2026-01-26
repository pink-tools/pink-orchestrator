# Code Review: pink-orchestrator

**Date:** 2026-01-19
**Status:** ACTION ITEMS

## Architecture

System tray app for managing pink-tools services (install/start/stop/update).

**Components:**
- `internal/tray/` — systray menu UI
- `internal/services/` — process lifecycle, install, update
- `internal/api/` — TCP server (port 7460) for CLI commands
- `internal/registry/` — YAML service definitions
- `internal/config/` — paths and constants

## Action Items

### Must Do

**1. Registry refresh on Update Orchestrator**
- **Problem:** `registry.yaml` cached locally, never auto-updates
- **Location:** `internal/tray/tray.go:374` — `updateOrchestrator()` calls `services.SelfUpdate()` but never `registry.Refresh()`
- **Fix:** Add `registry.Refresh()` call in `updateOrchestrator()` after successful self-update

### Should Do

**2. Headless logging**
- **Problem:** When running without terminal (autostart/tray-only), stdout/stderr goes nowhere
- **TTY detection exists:** `internal/services/selfupdate.go:82` — `!term.IsTerminal(int(os.Stdin.Fd()))` but only used for auto-restart decision
- **Fix:** Reuse TTY detection, if headless write logs to `~/.pink-orchestrator/orchestrator.log`

**3. Dead code cleanup**
- **Problem:** `config.ServicePidFile()` defined but never called
- **Location:** `internal/config/config.go:62-64`
- **Evidence:** Zero references in entire codebase, PID managed via in-memory map `runningProcesses` at `services.go:34`
- **Fix:** Remove unused function

## Minor Issues (Optional)

**4. Missing HTTP timeout**
- **File:** `internal/services/install.go`
- Downloads use `&http.Client{}` without timeout
- Low priority: downloads are from GitHub CDN, usually fast

**5. Silent taskkill errors (Windows)**
- **File:** `internal/services/stop_windows.go:19`
- `exec.Command("taskkill", ...).Run()` — error ignored
- Low priority: force kill rarely fails

**6. Lock file race condition**
- **File:** `internal/services/lock_unix.go:37-43`
- No mutex protecting `lockFile` variable during release
- Low priority: single-threaded access in practice

## Not Bugs (Verified)

- Self-update polling loop — necessary for binary replacement
- 3-second graceful shutdown + force kill — standard pattern
- 1-second Windows sleep after stop — file handle release time
- Root privileges on Unix — required for service management
- Single mutex for multiple maps — low contention, acceptable
- String version comparison — works for YYYYMMDD.HHMM format

## Notes

- `ai-docs/CLAUDE.md` is local dev documentation, not deployed
- Orchestrator CLI commands are in pink-agent's `.claude/CLAUDE.md` (deployed to `~/.claude/CLAUDE.md`)
