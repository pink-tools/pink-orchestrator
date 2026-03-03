//go:build windows

package services

import (
	"os"
	"os/exec"
	"path/filepath"
)

// ExecRestart starts a new instance of the binary and exits.
func ExecRestart() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	exe, _ = filepath.EvalSymlinks(exe)
	return execAndExit(exe)
}

// ExecPath starts the binary at path and exits.
func ExecPath(path string) error {
	return execAndExit(path)
}

func execAndExit(path string) error {
	cmd := exec.Command(path, os.Args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	os.Exit(0)
	return nil
}
