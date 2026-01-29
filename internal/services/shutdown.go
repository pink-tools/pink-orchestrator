package services

import "github.com/pink-tools/pink-orchestrator/internal/registry"

// Shutdown stops all running services
func Shutdown() {
	svcs, _ := registry.ListServices()
	for _, svc := range svcs {
		if GetStatus(svc.Name).Status == StatusRunning {
			Stop(svc.Name)
		}
	}
}
