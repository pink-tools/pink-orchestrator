package services

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"

	"github.com/pink-tools/pink-core"
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
		otel.Info(context.Background(), name, otel.Attr{"status", "already running"})
		return nil
	}

	svc, err := registry.GetService(name)
	if err != nil {
		return err
	}

	for _, dep := range svc.Dependencies {
		depStatus := GetStatus(dep)
		if depStatus.Status != StatusRunning {
			otel.Info(context.Background(), dep, otel.Attr{"status", "starting dependency"})
			if err := Start(dep); err != nil {
				return fmt.Errorf("failed to start dependency %s: %w", dep, err)
			}
		}
	}

	// Kill any existing process with same name (from previous session)
	killExisting(name)

	otel.Info(context.Background(), name, otel.Attr{"status", "starting"})
	binary := config.ServiceBinary(name)

	var cmd *exec.Cmd
	// On Unix, if running as root, drop privileges to original user
	if runtime.GOOS != "windows" && os.Getuid() == 0 {
		sudoUser := os.Getenv("SUDO_USER")
		if sudoUser != "" {
			cmd = exec.Command("sudo", "-E", "-u", sudoUser, binary)
		} else {
			cmd = exec.Command(binary)
		}
	} else {
		cmd = exec.Command(binary)
	}
	cmd.Dir = core.ServiceDir(name)

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
			otel.Warn(context.Background(), name, otel.Attr{"status", "exited"}, otel.Attr{"error", err.Error()})
		} else {
			otel.Info(context.Background(), name, otel.Attr{"status", "exited"})
		}
	}()

	return nil
}

func Stop(name string) error {
	mu.Lock()
	info, ok := runningProcesses[name]
	mu.Unlock()

	if !ok {
		return nil
	}

	otel.Info(context.Background(), name, otel.Attr{"status", "stopping"})

	// IPC shutdown - no fallback, if it fails something is wrong
	if !sendIPCStop(name) {
		return fmt.Errorf("IPC stop failed for %s", name)
	}

	// Wait for process to exit (if it hangs, it's a bug)
	<-info.done

	mu.Lock()
	delete(serviceLogs, name)
	mu.Unlock()

	return nil
}

func Restart(name string) error {
	otel.Info(context.Background(), name, otel.Attr{"status", "restarting"})
	if err := Stop(name); err != nil {
		return err
	}
	return Start(name)
}

func captureOutput(name string, r io.Reader, isStderr bool) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		if line != "" {
			otel.PrintServiceLog(line)
			updateServiceLog(name, line, isStderr)
		}
	}
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
