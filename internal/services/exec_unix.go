//go:build !windows

package services

import (
	"os"
	"path/filepath"
	"syscall"
)

// ExecRestart replaces the current process with a fresh instance of the binary.
func ExecRestart() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	exe, _ = filepath.EvalSymlinks(exe)
	return syscall.Exec(exe, os.Args, os.Environ())
}

// ExecPath replaces the current process with the binary at path.
func ExecPath(path string) error {
	return syscall.Exec(path, os.Args, os.Environ())
}
