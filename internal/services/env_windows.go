//go:build windows

package services

import (
	"os"
	"strings"

	"golang.org/x/sys/windows/registry"
)

// getSystemEnv returns environment with fresh PATH from Windows registry.
func getSystemEnv() []string {
	env := os.Environ()

	// Read fresh PATH from registry
	freshPath := getRegistryPath()
	if freshPath == "" {
		return env
	}

	// Replace PATH in env
	for i, e := range env {
		if strings.HasPrefix(strings.ToUpper(e), "PATH=") {
			env[i] = "PATH=" + freshPath
			return env
		}
	}

	return append(env, "PATH="+freshPath)
}

func getRegistryPath() string {
	var parts []string

	// System PATH (HKLM)
	if key, err := registry.OpenKey(registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Control\Session Manager\Environment`, registry.QUERY_VALUE); err == nil {
		if val, _, err := key.GetStringValue("Path"); err == nil {
			parts = append(parts, val)
		}
		key.Close()
	}

	// User PATH (HKCU)
	if key, err := registry.OpenKey(registry.CURRENT_USER, `Environment`, registry.QUERY_VALUE); err == nil {
		if val, _, err := key.GetStringValue("Path"); err == nil {
			parts = append(parts, val)
		}
		key.Close()
	}

	if len(parts) == 0 {
		return ""
	}

	return strings.Join(parts, ";")
}
