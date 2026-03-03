package services

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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

func CheckOrchestratorUpdate() (hasUpdate bool, installed, latest string, err error) {
	installed = orchestratorVersion

	// dev version always needs update
	if installed == "" || installed == "dev" {
		return true, installed, "latest", nil
	}

	latest, err = GetOrchestratorLatestVersion()
	if err != nil {
		return false, installed, "", err
	}

	return isNewer(latest, installed), installed, latest, nil
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

	tmpBinary := filepath.Join(os.TempDir(), "pink-orchestrator-update"+binaryExt())
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
		// Windows can't replace a running binary — use background batch script
		if err := runWindowsUpdater(currentBinary, tmpBinary, os.Getpid(), true); err != nil {
			os.Remove(tmpBinary)
			return fmt.Errorf("failed to start updater: %w", err)
		}
		progress("Update complete. Restarting...")
		return nil
	}

	// Unix: replace binary directly (macOS/Linux allow replacing a running binary's file)
	if err := os.Rename(tmpBinary, currentBinary); err != nil {
		os.Remove(tmpBinary)
		return fmt.Errorf("failed to replace binary: %w", err)
	}

	pendingRestart = true

	if runtime.GOOS == "darwin" {
		if newVersion != "" {
			progress(fmt.Sprintf("Updated to %s. Quitting.", newVersion))
		} else {
			progress("Updated. Quitting.")
		}
		return nil
	}

	progress("Update complete. Restarting...")
	return nil
}

func binaryExt() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
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

func runWindowsUpdater(targetPath, newBinary string, pid int, autoRestart bool) error {
	var script string
	if autoRestart {
		script = fmt.Sprintf(`@echo off
:wait
tasklist /FI "PID eq %d" | find "%d" >nul
if not errorlevel 1 (
    timeout /t 1 /nobreak >nul
    goto wait
)
move /Y "%s" "%s"
start "" "%s"
del "%%~f0"
`, pid, pid, newBinary, targetPath, targetPath)
	} else {
		script = fmt.Sprintf(`@echo off
:wait
tasklist /FI "PID eq %d" | find "%d" >nul
if not errorlevel 1 (
    timeout /t 1 /nobreak >nul
    goto wait
)
move /Y "%s" "%s"
del "%%~f0"
`, pid, pid, newBinary, targetPath)
	}

	scriptPath := filepath.Join(os.TempDir(), "pink-orchestrator-updater.bat")
	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		return err
	}

	cmd := exec.Command("cmd", "/C", scriptPath)
	return cmd.Start()
}
