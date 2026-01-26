//go:build !windows

package services

import (
	"os"

	"github.com/pink-tools/pink-orchestrator/internal/registry"
)

func signalProcess(pid int) {
	if p, err := os.FindProcess(pid); err == nil {
		p.Signal(os.Interrupt)
	}
}

func Shutdown() {
	// Stop services one by one
	svcs, _ := registry.ListServices()
	for _, svc := range svcs {
		if GetStatus(svc.Name).Status == StatusRunning {
			Stop(svc.Name)
		}
	}
}
