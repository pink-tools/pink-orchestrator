package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
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

func HomeDir() string {
	home, _ := os.UserHomeDir()
	return home
}

func PinkToolsDir() string {
	return filepath.Join(HomeDir(), "pink-tools")
}

func OrchestratorDir() string {
	return filepath.Join(HomeDir(), ".pink-orchestrator")
}

func StateFile() string {
	return filepath.Join(OrchestratorDir(), "state.json")
}

func RegistryCacheFile() string {
	return filepath.Join(OrchestratorDir(), "registry.yaml")
}

func ServiceDir(name string) string {
	return filepath.Join(PinkToolsDir(), name)
}

func ServiceBinary(name string) string {
	bin := name
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	return filepath.Join(ServiceDir(name), bin)
}

func ServiceEnvFile(name string) string {
	return filepath.Join(ServiceDir(name), ".env")
}

func ServicePidFile(name string) string {
	return filepath.Join(ServiceDir(name), name+".pid")
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
		PinkToolsDir(),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	return nil
}

func ClaudeDir() string {
	return filepath.Join(HomeDir(), ".claude")
}

func ClaudePinkToolsDir() string {
	return filepath.Join(ClaudeDir(), "pink-tools")
}

func ClaudeServiceDir(name string) string {
	return filepath.Join(ClaudePinkToolsDir(), name)
}

func ClaudeServiceMd(name string) string {
	return filepath.Join(ClaudeServiceDir(name), "CLAUDE.md")
}

func ClaudeProjectsMd() string {
	return filepath.Join(ClaudeDir(), "PROJECTS.md")
}
