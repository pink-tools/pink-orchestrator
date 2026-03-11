//go:build !windows && !darwin

package services

import (
	"os"
	"os/exec"
)

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
