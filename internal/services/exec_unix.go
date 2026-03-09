//go:build !windows

package services

import (
	"os"
	"os/exec"
	"path/filepath"
)

// ExecRestart starts a new instance of the binary and exits the current process.
// syscall.Exec leaves a zombie tray icon on macOS because Cocoa state isn't
// cleaned up when the process image is replaced. Spawning a child and exiting
// lets the OS tear down the old NSStatusItem properly.
func ExecRestart() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	exe, _ = filepath.EvalSymlinks(exe)
	return execSpawn(exe, os.Args[1:]...)
}

// ExecPath starts a new instance at the given path and exits the current process.
func ExecPath(path string) error {
	return execSpawn(path, os.Args[1:]...)
}

func execSpawn(path string, args ...string) error {
	cmd := exec.Command(path, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()
	if err := cmd.Start(); err != nil {
		return err
	}
	os.Exit(0)
	return nil
}
