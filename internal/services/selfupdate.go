package services

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/pink-tools/pink-orchestrator/internal/config"
)

const orchestratorRepo = "pink-tools/pink-orchestrator"

var orchestratorVersion string // Set by main package
var pendingRestart bool

func PendingRestart() bool {
	return pendingRestart
}

func SetOrchestratorBinaryVersion(v string) {
	orchestratorVersion = v
}

func GetOrchestratorLatestVersion() (string, error) {
	return GetLatestVersion(orchestratorRepo)
}

func CheckOrchestratorUpdate() (hasUpdate bool, current, latest string, err error) {
	current = orchestratorVersion

	// dev version always needs update
	if current == "" || current == "dev" {
		return true, current, "latest", nil
	}

	latest, err = GetOrchestratorLatestVersion()
	if err != nil {
		return false, current, "", err
	}

	return isNewer(latest, current), current, latest, nil
}

func SelfUpdate(targetVersion string, progress func(string)) error {
	progress("Downloading new version...")

	binaryName := "pink-orchestrator-" + config.Platform()
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}

	downloadURL := fmt.Sprintf("https://github.com/%s/releases/latest/download/%s", orchestratorRepo, binaryName)

	currentBinary, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get current binary path: %w", err)
	}
	currentBinary, _ = filepath.EvalSymlinks(currentBinary)

	tmpBinary := currentBinary + ".update"
	if err := downloadFile(downloadURL, tmpBinary, progress); err != nil {
		return fmt.Errorf("failed to download: %w", err)
	}

	if err := os.Chmod(tmpBinary, 0755); err != nil {
		os.Remove(tmpBinary)
		return fmt.Errorf("failed to chmod: %w", err)
	}

	// Get version from downloaded binary
	newVersion := getVersionFromBinary(tmpBinary)
	if newVersion != "" && newVersion == orchestratorVersion {
		os.Remove(tmpBinary)
		progress("Already up to date")
		return nil
	}

	progress("Installing update...")

	if runtime.GOOS == "windows" {
		// Windows allows renaming a running .exe but not modifying or deleting it.
		// Swap files in-process so failures surface here, before we exit — otherwise
		// a deferred move-after-exit can silently fail (Defender lock etc.) and a
		// later `start` re-launches the OLD binary, looping forever.
		oldBinary := currentBinary + ".old"
		os.Remove(oldBinary)
		if err := os.Rename(currentBinary, oldBinary); err != nil {
			os.Remove(tmpBinary)
			return fmt.Errorf("failed to rename current binary: %w", err)
		}
		if err := os.Rename(tmpBinary, currentBinary); err != nil {
			os.Rename(oldBinary, currentBinary)
			os.Remove(tmpBinary)
			return fmt.Errorf("failed to install new binary: %w", err)
		}
		if err := scheduleWindowsRespawn(currentBinary, oldBinary, os.Getpid()); err != nil {
			os.Rename(currentBinary, tmpBinary)
			os.Rename(oldBinary, currentBinary)
			return fmt.Errorf("failed to schedule restart: %w", err)
		}
		pendingRestart = true
		progress("Update complete. Restarting...")
		return nil
	}

	// Unix: replace binary directly (macOS/Linux allow replacing a running binary's file)
	if err := os.Rename(tmpBinary, currentBinary); err != nil {
		os.Remove(tmpBinary)
		return fmt.Errorf("failed to replace binary: %w", err)
	}
	os.Chmod(currentBinary, 0755)

	// Chown to real user when running as root via sudo
	if sudoUser := os.Getenv("SUDO_USER"); sudoUser != "" {
		if u, err := user.Lookup(sudoUser); err == nil {
			uid, _ := strconv.Atoi(u.Uid)
			gid, _ := strconv.Atoi(u.Gid)
			os.Chown(currentBinary, uid, gid)
		}
	}

	progress(fmt.Sprintf("Updated to %s. Exiting.", newVersion))
	os.Exit(0)
	return nil
}

func getVersionFromBinary(path string) string {
	cmd := exec.Command(path, "--version")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	// Output: "pink-orchestrator vYYYYMMDD.HHMM"
	s := strings.TrimSpace(string(out))
	if strings.HasPrefix(s, "pink-orchestrator v") {
		return strings.TrimPrefix(s, "pink-orchestrator v")
	}
	return ""
}

// scheduleWindowsRespawn writes a batch script that waits for the current
// process to exit, deletes the renamed-aside old binary, and starts the new
// one. The binary swap itself happens in-process before this is called.
func scheduleWindowsRespawn(targetPath, oldBinary string, pid int) error {
	script := fmt.Sprintf(`@echo off
:wait
tasklist /FI "PID eq %d" | find "%d" >nul
if not errorlevel 1 (
    timeout /t 1 /nobreak >nul
    goto wait
)
del /F /Q "%s"
start "" "%s"
del "%%~f0"
`, pid, pid, oldBinary, targetPath)

	scriptPath := filepath.Join(os.TempDir(), "pink-orchestrator-updater.bat")
	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		return err
	}

	cmd := exec.Command("cmd", "/C", scriptPath)
	return cmd.Start()
}
