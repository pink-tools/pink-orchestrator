package services

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/pink-tools/pink-core/log"
	"github.com/pink-tools/pink-orchestrator/internal/config"
	"github.com/pink-tools/pink-orchestrator/internal/registry"
	"golang.org/x/mod/semver"
)

type State struct {
	RunningServices []string `json:"running_services"`
}

var (
	stateMu     sync.Mutex
	state       = &State{}
	stateLoaded bool
)

func loadState() {
	if stateLoaded {
		return
	}
	data, err := os.ReadFile(config.StateFile())
	if err != nil {
		stateLoaded = true
		return
	}
	if err := json.Unmarshal(data, state); err != nil {
		log.Warn(context.Background(), "failed to parse state file", log.Attr{K: "error", V: err.Error()})
	}
	stateLoaded = true
}

func SaveState() error {
	services, err := registry.ListServices()
	if err != nil {
		return fmt.Errorf("failed to list services: %w", err)
	}

	var running []string
	for _, svc := range services {
		if GetStatus(svc.Name).Status == StatusRunning {
			running = append(running, svc.Name)
		}
	}

	stateMu.Lock()
	state.RunningServices = running
	data, err := json.MarshalIndent(state, "", "  ")
	stateMu.Unlock()

	if err != nil {
		return err
	}

	// Atomic write: write to .tmp then rename
	stateFile := config.StateFile()
	tmpFile := stateFile + ".tmp"

	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write state: %w", err)
	}

	if err := os.Rename(tmpFile, stateFile); err != nil {
		os.Remove(tmpFile)
		return fmt.Errorf("failed to save state: %w", err)
	}

	return nil
}

func RestoreState() {
	stateMu.Lock()
	loadState()
	toStart := make([]string, len(state.RunningServices))
	copy(toStart, state.RunningServices)
	stateMu.Unlock()

	for _, name := range toStart {
		if err := Start(name); err != nil {
			log.Warn(context.Background(), "failed to restore service", log.Attr{K: "service", V: name}, log.Attr{K: "error", V: err.Error()})
		}
	}
}

type GitHubRelease struct {
	Name    string `json:"name"`
	TagName string `json:"tag_name"`
}

func getLatestRelease(repo string) (*GitHubRelease, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GitHub API error: %d", resp.StatusCode)
	}

	var release GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, err
	}
	return &release, nil
}

func GetLatestVersion(repo string) (string, error) {
	release, err := getLatestRelease(repo)
	if err != nil {
		return "", err
	}

	// Name format: "pink-xxx vX.Y.Z" - extract version
	parts := strings.Split(release.Name, " ")
	if len(parts) >= 2 {
		return parts[len(parts)-1], nil
	}
	return release.Name, nil
}

func GetLatestReleaseTag(repo string) (string, error) {
	release, err := getLatestRelease(repo)
	if err != nil {
		return "", err
	}
	return release.TagName, nil
}

func GetInstalledVersion(name string) string {
	binary := config.ServiceBinary(name)
	if _, err := os.Stat(binary); err != nil {
		return ""
	}

	cmd := exec.Command(binary, "--version")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}

	// Parse "pink-xxx v1.2.3" → "v1.2.3"
	parts := strings.Fields(strings.TrimSpace(string(output)))
	if len(parts) >= 2 {
		return parts[1]
	}
	return ""
}

// isNewer returns true if latest version is newer than installed
// If installed is legacy date format (YYYYMMDD.HHMM) — always update
func isNewer(latest, installed string) bool {
	// Legacy detection: date format YYYYMMDD.HHMM (major version > 10000)
	isLegacy := func(v string) bool {
		v = strings.TrimPrefix(v, "v")
		parts := strings.Split(v, ".")
		if len(parts) > 0 && len(parts[0]) > 4 {
			return true
		}
		return false
	}

	// Installed is legacy? → always update
	if isLegacy(installed) {
		return true
	}

	// Latest is legacy? → don't update (shouldn't happen)
	if isLegacy(latest) {
		return false
	}

	// Normal semver comparison
	latestV := latest
	installedV := installed
	if !strings.HasPrefix(latestV, "v") {
		latestV = "v" + latestV
	}
	if !strings.HasPrefix(installedV, "v") {
		installedV = "v" + installedV
	}

	if !semver.IsValid(latestV) || !semver.IsValid(installedV) {
		return false
	}

	return semver.Compare(latestV, installedV) > 0
}

func CheckUpdate(name string) (hasUpdate bool, installed, latest string, err error) {
	svc, err := registry.GetService(name)
	if err != nil {
		return false, "", "", err
	}

	installed = GetInstalledVersion(name)
	if installed == "" {
		return false, "", "", nil
	}

	latest, err = GetLatestVersion(svc.Repo)
	if err != nil {
		return false, installed, "", err
	}

	return isNewer(latest, installed), installed, latest, nil
}
