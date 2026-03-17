package registry

import (
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/pink-tools/pink-orchestrator/internal/config"
	"gopkg.in/yaml.v3"
)

type Registry struct {
	Version  int       `yaml:"version"`
	Services []Service `yaml:"services"`
}

type Service struct {
	Name         string      `yaml:"name"`
	Repo         string      `yaml:"repo"`
	Type         string      `yaml:"type"`
	ReleaseTag   string      `yaml:"release_tag,omitempty"`
	HasSetup     bool        `yaml:"has_setup,omitempty"`
	Dependencies []string    `yaml:"dependencies,omitempty"`
	SystemDeps   []SystemDep `yaml:"system_deps,omitempty"`
	EnvVars      []EnvVar    `yaml:"env_vars,omitempty"`
	ExtraAssets  []Asset     `yaml:"extra_assets,omitempty"`
}

type EnvVar struct {
	Name        string `yaml:"name"`
	Default     string `yaml:"default,omitempty"`
	Required    bool   `yaml:"required,omitempty"`
	Description string `yaml:"description,omitempty"`
}

type Asset struct {
	URL  string `yaml:"url"`
	Path string `yaml:"path"`
	Size int64  `yaml:"size,omitempty"`
}

type SystemDep struct {
	Name       string `yaml:"name"`
	Brew       string `yaml:"brew,omitempty"`
	Apt        string `yaml:"apt,omitempty"`
	Winget     string `yaml:"winget,omitempty"`
	UnixScript string `yaml:"unix_script,omitempty"`
	WinScript  string `yaml:"win_script,omitempty"`
}

var (
	cacheMu sync.RWMutex
	cached  *Registry
)

func Load() (*Registry, error) {
	cacheMu.RLock()
	if cached != nil {
		defer cacheMu.RUnlock()
		return cached, nil
	}
	cacheMu.RUnlock()

	cacheMu.Lock()
	defer cacheMu.Unlock()

	if cached != nil {
		return cached, nil
	}

	return refreshLocked()
}

func Refresh() (*Registry, error) {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	return refreshLocked()
}

func refreshLocked() (*Registry, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(config.RegistryURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch registry: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("registry fetch failed: %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read registry: %w", err)
	}

	var reg Registry
	if err := yaml.Unmarshal(data, &reg); err != nil {
		return nil, fmt.Errorf("failed to parse registry: %w", err)
	}

	cached = &reg
	return cached, nil
}

func GetService(name string) (*Service, error) {
	reg, err := Load()
	if err != nil {
		return nil, err
	}

	for _, svc := range reg.Services {
		if svc.Name == name {
			return &svc, nil
		}
	}

	return nil, fmt.Errorf("service not found: %s", name)
}

func ListServices() ([]Service, error) {
	reg, err := Load()
	if err != nil {
		return nil, err
	}
	return reg.Services, nil
}

func IsDaemon(name string) bool {
	svc, err := GetService(name)
	if err != nil {
		return false
	}
	return svc.Type == "daemon"
}

// MaxServiceNameLen returns the length of the longest service name
func MaxServiceNameLen() int {
	maxLen := len("pink-orchestrator") // orchestrator logs too but not in registry

	reg, err := Load()
	if err != nil {
		return maxLen
	}
	for _, svc := range reg.Services {
		if len(svc.Name) > maxLen {
			maxLen = len(svc.Name)
		}
	}
	return maxLen
}
