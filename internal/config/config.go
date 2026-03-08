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

// MigrateClaudeDirs removes all obsolete .claude/ directories from pink-tools locations.
// NEVER touches ~/.claude/ — that's user's personal config.
func MigrateClaudeDirs() {
	// /Users/.claude/
	os.RemoveAll(filepath.Join(legacyBaseDir(), ".claude"))

	// ~/pink-tools/.claude/ and ~/pink-tools/*/.claude/
	cleanClaudeDirs(core.PinkToolsDir())

	// /Users/pink-tools/.claude/ and /Users/pink-tools/*/.claude/
	oldPinkTools := filepath.Join(legacyBaseDir(), "pink-tools")
	cleanClaudeDirs(oldPinkTools)
}

func cleanClaudeDirs(base string) {
	if info, err := os.Stat(base); err != nil || !info.IsDir() {
		return
	}
	os.RemoveAll(filepath.Join(base, ".claude"))
	entries, _ := os.ReadDir(base)
	for _, e := range entries {
		if e.IsDir() {
			os.RemoveAll(filepath.Join(base, e.Name(), ".claude"))
		}
	}
}

// MigrateWhisperModel moves whisper model from old ServiceDir to AppDataDir.
func MigrateWhisperModel() {
	newDir := core.AppDataDir("pink-whisper")
	modelName := "ggml-large-v3.bin"
	newPath := filepath.Join(newDir, modelName)

	if fileExists(newPath) {
		return
	}

	oldPaths := []string{
		filepath.Join(core.ServiceDir("pink-whisper"), modelName),
		filepath.Join(filepath.Dir(core.HomeDir()), "pink-tools", "pink-whisper", modelName),
	}

	for _, old := range oldPaths {
		if fileExists(old) {
			os.MkdirAll(newDir, 0755)
			if os.Rename(old, newPath) == nil {
				return
			}
		}
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
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

