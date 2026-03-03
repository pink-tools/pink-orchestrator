//go:build !windows

package services

import (
	"os"
	"path/filepath"
	"syscall"
)

// ExecRestart replaces the current process with a new instance of the binary.
func ExecRestart() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	exe, _ = filepath.EvalSymlinks(exe)
	return syscall.Exec(exe, append([]string{exe}, os.Args[1:]...), os.Environ())
}

// ExecPath replaces the current process with the binary at path.
func ExecPath(path string) error {
	return syscall.Exec(path, append([]string{path}, os.Args[1:]...), os.Environ())
}
