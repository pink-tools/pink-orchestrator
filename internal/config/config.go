package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"

	"github.com/pink-tools/pink-core"
)

const (
	RegistryURL = "https://raw.githubusercontent.com/pink-tools/pink-orchestrator/main/registry.yaml"
	GitHubAPI   = "https://api.github.com"
	DefaultPort = 7460
)

func Port() int {
	if p := os.Getenv("ORCHESTRATOR_PORT"); p != "" {
		if port, err := strconv.Atoi(p); err == nil {
			return port
		}
	}
	return DefaultPort
}

func OrchestratorDir() string {
	return filepath.Join(core.PinkToolsDir(), "pink-orchestrator")
}

func StateFile() string {
	return filepath.Join(OrchestratorDir(), "state.json")
}

func RegistryCacheFile() string {
	return filepath.Join(OrchestratorDir(), "registry.yaml")
}

// ServiceBinary returns full path to a service binary.
// Deprecated: use core.BinaryPath instead.
func ServiceBinary(name string) string {
	return core.BinaryPath(name)
}

func ServiceEnvFile(name string) string {
	return filepath.Join(core.ServiceDir(name), ".env")
}

func Platform() string {
	os := runtime.GOOS
	arch := runtime.GOARCH
	return os + "-" + arch
}

func BinaryName(service string) string {
	name := service + "-" + Platform()
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return name
}

func EnsureDirs() error {
	dirs := []string{
		OrchestratorDir(),
		core.PinkToolsDir(),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	return nil
}


