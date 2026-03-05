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

// legacyBaseDir returns the old base directory (parent of home).
// Only used for migrating away from the old layout.
func legacyBaseDir() string {
	return filepath.Dir(core.HomeDir())
}

// MigrateStateDir moves state files from old /Users/.pink-orchestrator/ to new location.
func MigrateStateDir() {
	oldDir := filepath.Join(legacyBaseDir(), ".pink-orchestrator")
	newDir := OrchestratorDir()
	if oldDir == newDir {
		return
	}
	info, err := os.Stat(oldDir)
	if err != nil || !info.IsDir() {
		return
	}
	entries, err := os.ReadDir(oldDir)
	if err != nil {
		return
	}
	os.MkdirAll(newDir, 0755)
	for _, e := range entries {
		old := filepath.Join(oldDir, e.Name())
		dst := filepath.Join(newDir, e.Name())
		os.Rename(old, dst)
	}
	os.Remove(oldDir)
}

// AgentClaudeDir returns agent's .claude directory (~/pink-tools/.claude/).
func AgentClaudeDir() string {
	return filepath.Join(core.PinkToolsDir(), ".claude")
}

// AgentClaudeServiceDir returns agent's per-service .claude directory (~/pink-tools/<name>/.claude/).
func AgentClaudeServiceDir(name string) string {
	return filepath.Join(core.PinkToolsDir(), name, ".claude")
}

// AgentClaudeServiceMd returns path to service CLAUDE.md.
func AgentClaudeServiceMd(name string) string {
	return filepath.Join(AgentClaudeServiceDir(name), "CLAUDE.md")
}

// AgentClaudeProjectsMd returns path to agent's PROJECTS.md.
func AgentClaudeProjectsMd() string {
	return filepath.Join(AgentClaudeDir(), "PROJECTS.md")
}

// MigrateClaudeDir removes old /Users/.claude/ directory if it exists.
func MigrateClaudeDir() {
	oldDir := filepath.Join(legacyBaseDir(), ".claude")
	if _, err := os.Stat(oldDir); err != nil {
		return
	}
	os.RemoveAll(oldDir)
}

// MigratePinkToolsDir moves services from old /Users/pink-tools/ to ~/pink-tools/.
func MigratePinkToolsDir() {
	oldDir := filepath.Join(legacyBaseDir(), "pink-tools")
	newDir := core.PinkToolsDir()
	if oldDir == newDir {
		return
	}
	info, err := os.Stat(oldDir)
	if err != nil || !info.IsDir() {
		return
	}
	os.MkdirAll(newDir, 0755)
	entries, err := os.ReadDir(oldDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		old := filepath.Join(oldDir, e.Name())
		dst := filepath.Join(newDir, e.Name())
		if _, err := os.Stat(dst); err == nil {
			continue // already exists in new location
		}
		os.Rename(old, dst)
	}
	os.RemoveAll(oldDir)
}

