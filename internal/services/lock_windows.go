//go:build windows

package services

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/pink-tools/pink-orchestrator/internal/config"
	"golang.org/x/sys/windows"
)

var lockFile *os.File

func AcquireLock() error {
	lockPath := filepath.Join(config.OrchestratorDir(), "orchestrator.lock")

	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return fmt.Errorf("failed to open lock file: %w", err)
	}

	h := windows.Handle(f.Fd())
	err = windows.LockFileEx(h, windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &windows.Overlapped{})
	if err != nil {
		f.Close()
		return fmt.Errorf("another instance is already running")
	}

	f.Truncate(0)
	f.WriteString(strconv.Itoa(os.Getpid()))

	lockFile = f
	return nil
}

func ReleaseLock() {
	if lockFile != nil {
		h := windows.Handle(lockFile.Fd())
		windows.UnlockFileEx(h, 0, 1, 0, &windows.Overlapped{})
		lockFile.Close()
		lockFile = nil
	}
}
