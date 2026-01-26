//go:build windows

package services

import (
	"os/exec"
	"strconv"

	"github.com/pink-tools/pink-orchestrator/internal/registry"
)

func signalProcess(pid int) {
	// /T kills entire process tree (including children like cloudflared)
	exec.Command("taskkill", "/F", "/T", "/PID", strconv.Itoa(pid)).Run()
}

func Shutdown() {
	// Stop services one by one (same as Unix)
	svcs, _ := registry.ListServices()
	for _, svc := range svcs {
		if GetStatus(svc.Name).Status == StatusRunning {
			Stop(svc.Name)
		}
	}
}
