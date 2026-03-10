package services

import (
	"log"

	"github.com/pink-tools/pink-orchestrator/internal/registry"
)

// Shutdown stops all running services
func Shutdown() {
	svcs, err := registry.ListServices()
	if err != nil {
		log.Printf("shutdown: failed to list services: %v", err)
	}
	for _, svc := range svcs {
		if GetStatus(svc.Name).Status == StatusRunning {
			Stop(svc.Name)
		}
	}
}
