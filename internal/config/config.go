package config

import (
	"fmt"
	"os"
	"os/exec"
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

func HomeDir() string {
	home, _ := os.UserHomeDir()
	return home
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

func ServiceBinary(name string) string {
	bin := name
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	return filepath.Join(core.ServiceDir(name), bin)
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

// ChownDirs recursively chowns pink-tools directories to the given user.
// Called after EnsureDirs when running as root via sudo.
func ChownDirs(username string) error {
	if username == "" {
		return nil
	}
	dirs := []string{core.PinkToolsDir()}
	for _, dir := range dirs {
		cmd := exec.Command("chown", "-R", username+":staff", dir)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("chown %s: %w", dir, err)
		}
	}
	return nil
}

// MigrateStateDir moves state files from old /Users/.pink-orchestrator/ to new location.
func MigrateStateDir() {
	oldDir := filepath.Join(core.BaseDir(), ".pink-orchestrator")
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

// AgentClaudeDir returns agent's .claude directory (/Users/.claude).
func AgentClaudeDir() string {
	return filepath.Join(core.BaseDir(), ".claude")
}

// AgentClaudePinkToolsDir returns agent's pink-tools directory.
func AgentClaudePinkToolsDir() string {
	return filepath.Join(AgentClaudeDir(), "pink-tools")
}

// AgentClaudeServiceDir returns agent's service directory.
func AgentClaudeServiceDir(name string) string {
	return filepath.Join(AgentClaudePinkToolsDir(), name)
}

// AgentClaudeServiceMd returns path to service CLAUDE.md.
func AgentClaudeServiceMd(name string) string {
	return filepath.Join(AgentClaudeServiceDir(name), "CLAUDE.md")
}

// AgentClaudeProjectsMd returns path to agent's PROJECTS.md.
func AgentClaudeProjectsMd() string {
	return filepath.Join(AgentClaudeDir(), "PROJECTS.md")
}

