//go:build !windows

package services

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/pink-tools/pink-orchestrator/internal/config"
)

var lockFile *os.File

func AcquireLock() error {
	lockPath := filepath.Join(config.OrchestratorDir(), "orchestrator.lock")

	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return fmt.Errorf("failed to open lock file: %w", err)
	}

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
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
		syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN)
		lockFile.Close()
		lockFile = nil
	}
}
