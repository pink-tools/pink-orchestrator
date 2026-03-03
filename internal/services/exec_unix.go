//go:build !windows

package services

import (
	"os"
	"os/exec"
	"path/filepath"
)

// ExecRestart launches a new instance of the binary and returns.
// The caller must exit after this. We use exec.Command instead of
// syscall.Exec because macOS systray (NSStatusBar) needs a fresh
// process — exec replaces in-place and loses the WindowServer connection.
func ExecRestart() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	exe, _ = filepath.EvalSymlinks(exe)
	cmd := exec.Command(exe, os.Args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()
	return cmd.Start()
}

// ExecPath launches the binary at path as a new process.
func ExecPath(path string) error {
	cmd := exec.Command(path, os.Args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()
	return cmd.Start()
}
