package services

import (
	"encoding/json"
	"fmt"
	"os/exec"

	"github.com/pink-tools/pink-orchestrator/internal/config"
)

// minActionsVersion is the minimum service version that supports --actions.
const minActionsVersion = "v1.9.21"

type Action struct {
	Name  string `json:"name"`
	Label string `json:"label"`
	Desc  string `json:"desc"`
}

// supportsActions checks if the installed binary version supports --actions.
func supportsActions(name string) bool {
	v := GetInstalledVersion(name)
	if v == "" {
		return false
	}
	return !isNewer(minActionsVersion, v) // v >= minActionsVersion
}

// GetActions runs {binary} --actions and parses the JSON output.
// Returns nil if the service is not installed or too old.
func GetActions(name string) []Action {
	if !IsInstalled(name) || !supportsActions(name) {
		return nil
	}

	binary := config.ServiceBinary(name)
	cmd := exec.Command(binary, "--actions")
	output, err := cmd.Output()
	if err != nil {
		return nil
	}

	var actions []Action
	if err := json.Unmarshal(output, &actions); err != nil {
		return nil
	}

	// Filter out "install" — it's handled by the orchestrator's own install flow
	filtered := actions[:0]
	for _, a := range actions {
		if a.Name != "install" {
			filtered = append(filtered, a)
		}
	}
	return filtered
}

// DescribeAction runs {binary} {action} --describe and returns the raw JSON FormSpec.
func DescribeAction(name, action string) (json.RawMessage, error) {
	binary := config.ServiceBinary(name)
	cmd := exec.Command(binary, action, "--describe")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("describe %s %s: %w", name, action, err)
	}
	return json.RawMessage(output), nil
}

// ExecuteAction runs {binary} {action} --config '{json}' with the provided values.
func ExecuteAction(name, action string, values map[string]any) error {
	binary := config.ServiceBinary(name)
	data, err := json.Marshal(values)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	cmd := exec.Command(binary, action, "--config", string(data))
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %s", name, action, string(output))
	}
	return nil
}
