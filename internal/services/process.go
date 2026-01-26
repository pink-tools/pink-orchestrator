package services

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/pink-tools/pink-otel"
	"github.com/pink-tools/pink-orchestrator/internal/config"
	"github.com/pink-tools/pink-orchestrator/internal/registry"
)

func Start(name string) error {
	if !IsInstalled(name) {
		return fmt.Errorf("service not installed: %s", name)
	}

	ClearLastError(name)

	status := GetStatus(name)
	if status.Status == StatusRunning {
		otel.Info(context.Background(), "already running", map[string]any{"service": name})
		return nil
	}

	svc, err := registry.GetService(name)
	if err != nil {
		return err
	}

	for _, dep := range svc.Dependencies {
		depStatus := GetStatus(dep)
		if depStatus.Status != StatusRunning {
			otel.Info(context.Background(), "starting dependency", map[string]any{"service": dep})
			if err := Start(dep); err != nil {
				return fmt.Errorf("failed to start dependency %s: %w", dep, err)
			}
		}
	}

	// Kill any existing process with same name (from previous session)
	killExisting(name)

	otel.Info(context.Background(), "starting", map[string]any{"service": name})
	binary := config.ServiceBinary(name)

	var cmd *exec.Cmd
	// On Unix, if running as root, drop privileges to original user
	if runtime.GOOS != "windows" && os.Getuid() == 0 {
		sudoUser := os.Getenv("SUDO_USER")
		if sudoUser != "" {
			cmd = exec.Command("sudo", "-u", sudoUser, binary)
		} else {
			cmd = exec.Command(binary)
		}
	} else {
		cmd = exec.Command(binary)
	}
	cmd.Dir = config.ServiceDir(name)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	cmd.Env = loadServiceEnv(name)

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start service: %w", err)
	}

	pid := cmd.Process.Pid
	info := &processInfo{
		process: cmd.Process,
		done:    make(chan struct{}),
	}

	mu.Lock()
	runningProcesses[name] = info
	mu.Unlock()

	go captureOutput(name, stdout, false)
	go captureOutput(name, stderr, true)

	go func() {
		err := cmd.Wait()
		mu.Lock()
		delete(runningProcesses, name)
		mu.Unlock()
		close(info.done)
		if err != nil {
			otel.Warn(context.Background(), "exited with error", map[string]any{"service": name, "error": err.Error()})
		} else {
			otel.Info(context.Background(), "exited", map[string]any{"service": name})
		}
	}()

	otel.Info(context.Background(), "started", map[string]any{"service": name, "pid": pid})
	return nil
}

func Stop(name string) error {
	mu.Lock()
	info, ok := runningProcesses[name]
	mu.Unlock()

	if !ok {
		return nil
	}

	otel.Info(context.Background(), "stopping", map[string]any{"service": name, "pid": info.process.Pid})

	signalProcess(info.process.Pid)

	select {
	case <-info.done:
	case <-time.After(3 * time.Second):
		otel.Info(context.Background(), "force killing", map[string]any{"service": name})
		info.process.Kill()
		<-info.done
	}

	mu.Lock()
	delete(serviceLogs, name)
	mu.Unlock()

	otel.Info(context.Background(), "stopped", map[string]any{"service": name})
	return nil
}

func Restart(name string) error {
	otel.Info(context.Background(), "restarting", map[string]any{"service": name})
	if err := Stop(name); err != nil {
		return err
	}
	return Start(name)
}

func captureOutput(name string, r io.Reader, isStderr bool) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		parsed := parseOtelLine(line)
		if parsed != "" {
			otel.Info(context.Background(), parsed, map[string]any{"source": name})
			updateServiceLog(name, parsed, isStderr)
		}
	}
}

func parseOtelLine(line string) string {
	if strings.HasPrefix(line, "{") {
		var otel struct {
			ResourceLogs []struct {
				ScopeLogs []struct {
					LogRecords []struct {
						Body struct {
							StringValue string `json:"stringValue"`
						} `json:"body"`
						Attributes []struct {
							Key   string `json:"key"`
							Value struct {
								StringValue string `json:"stringValue"`
								IntValue    string `json:"intValue"`
							} `json:"value"`
						} `json:"attributes"`
					} `json:"logRecords"`
				} `json:"scopeLogs"`
			} `json:"resourceLogs"`
		}
		if err := json.Unmarshal([]byte(line), &otel); err == nil {
			if len(otel.ResourceLogs) > 0 && len(otel.ResourceLogs[0].ScopeLogs) > 0 {
				logs := otel.ResourceLogs[0].ScopeLogs[0].LogRecords
				if len(logs) > 0 {
					msg := logs[0].Body.StringValue
					var attrs []string
					for _, attr := range logs[0].Attributes {
						val := attr.Value.StringValue
						if val == "" {
							val = attr.Value.IntValue
						}
						if val != "" {
							attrs = append(attrs, attr.Key+"="+val)
						}
					}
					if len(attrs) > 0 {
						msg += " [" + strings.Join(attrs, ", ") + "]"
					}
					return msg
				}
			}
		}
		return ""
	}
	return line
}

func killExisting(name string) {
	if runtime.GOOS == "windows" {
		exec.Command("taskkill", "/F", "/IM", name+".exe").Run()
	} else if runtime.GOOS == "darwin" {
		exec.Command("killall", "-9", name).Run()
	} else {
		exec.Command("pkill", "-9", "-x", name).Run()
	}
}
