package services

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pink-tools/pink-core"
	"github.com/pink-tools/pink-orchestrator/internal/config"
	"github.com/pink-tools/pink-orchestrator/internal/registry"
)

func loadServiceEnv(name string) []string {
	env := appendPinkToolsToPath(getSystemEnv())

	// Service name width for log alignment (children output JSON, orchestrator formats)
	env = append(env, fmt.Sprintf("PINK_LOG_WIDTH=%d", registry.MaxServiceNameLen()))

	envFile := config.ServiceEnvFile(name)
	data, err := os.ReadFile(envFile)
	if err != nil {
		return env
	}

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.Contains(line, "=") {
			env = append(env, line)
		}
	}

	return env
}

func appendPinkToolsToPath(env []string) []string {
	pinkToolsDir := core.PinkToolsDir()
	entries, err := os.ReadDir(pinkToolsDir)
	if err != nil {
		return env
	}

	var extraPaths []string
	for _, entry := range entries {
		if entry.IsDir() {
			extraPaths = append(extraPaths, filepath.Join(pinkToolsDir, entry.Name()))
		}
	}

	if len(extraPaths) == 0 {
		return env
	}

	sep := string(os.PathListSeparator)
	extra := strings.Join(extraPaths, sep)
	for i, e := range env {
		if strings.HasPrefix(strings.ToUpper(e), "PATH=") {
			env[i] = e + sep + extra
			return env
		}
	}
	return append(env, "PATH="+extra)
}
