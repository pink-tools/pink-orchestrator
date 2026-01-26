package services

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/pink-tools/pink-orchestrator/internal/config"
	"golang.org/x/term"
)

const orchestratorRepo = "pink-tools/pink-orchestrator"

var orchestratorVersion string // Set by main package

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

	// Windows: always restart (console window closes, new one opens)
	// Unix TTY: don't restart (output would be lost)
	// Unix non-TTY: restart (daemon mode)
	autoRestart := runtime.GOOS == "windows" || !term.IsTerminal(int(os.Stdin.Fd()))

	if err := runUpdater(currentBinary, tmpBinary, autoRestart); err != nil {
		os.Remove(tmpBinary)
		return fmt.Errorf("failed to start updater: %w", err)
	}

	// NOTE: Version is NOT saved here because the actual file replacement
	// happens in a background script after this process exits.
	// Version will be updated on next startup via InitOrchestratorVersion()

	if autoRestart {
		progress("Update complete. Restarting...")
	} else {
		progress("Update complete. Please restart manually.")
	}
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

func runUpdater(targetPath, newBinary string, autoRestart bool) error {
	pid := os.Getpid()

	if runtime.GOOS == "windows" {
		return runWindowsUpdater(targetPath, newBinary, pid, autoRestart)
	}
	return runUnixUpdater(targetPath, newBinary, pid, autoRestart)
}

func runUnixUpdater(targetPath, newBinary string, pid int, autoRestart bool) error {
	var script string
	if autoRestart {
		script = fmt.Sprintf(`#!/bin/bash
while kill -0 %d 2>/dev/null; do sleep 0.1; done
sudo mv "%s" "%s"
"%s" &
rm "$0"
`, pid, newBinary, targetPath, targetPath)
	} else {
		script = fmt.Sprintf(`#!/bin/bash
while kill -0 %d 2>/dev/null; do sleep 0.1; done
sudo mv "%s" "%s"
rm "$0"
`, pid, newBinary, targetPath)
	}

	scriptPath := filepath.Join(os.TempDir(), "pink-orchestrator-updater.sh")
	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		return err
	}

	cmd := exec.Command("bash", scriptPath)
	return cmd.Start()
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
