//go:build !windows

package services

import "os"

// getSystemEnv returns current environment.
// On Unix, environment changes require shell restart anyway.
func getSystemEnv() []string {
	return os.Environ()
}
