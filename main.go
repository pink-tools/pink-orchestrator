package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"

	"github.com/pink-tools/pink-otel"
	"github.com/pink-tools/pink-orchestrator/internal/api"
	"github.com/pink-tools/pink-orchestrator/internal/config"
	"github.com/pink-tools/pink-orchestrator/internal/registry"
	"github.com/pink-tools/pink-orchestrator/internal/services"
	"github.com/pink-tools/pink-orchestrator/internal/tray"
)

var version = "dev"

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "--version", "-V":
			fmt.Printf("pink-orchestrator v%s\n", version)
			return
		case "--health":
			fmt.Println("OK")
			return
		case "--help", "-h":
			printUsage()
			return
		case "--update":
			if len(os.Args) < 3 {
				fmt.Println("Usage: pink-orchestrator --update <version>")
				os.Exit(1)
			}
			targetVersion := os.Args[2]
			if err := services.SelfUpdate(targetVersion, func(msg string) {
				fmt.Println(msg)
			}); err != nil {
				fmt.Printf("Update failed: %v\n", err)
				os.Exit(1)
			}
			os.Exit(0)
		case "--service-update", "--service-restart", "--service-stop", "--service-start":
			if len(os.Args) < 3 {
				fmt.Printf("Usage: pink-orchestrator %s <service-name>\n", os.Args[1])
				os.Exit(1)
			}
			cmd := os.Args[1][len("--service-"):]
			serviceName := os.Args[2]
			msg, err := api.Send(cmd, serviceName)
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				os.Exit(1)
			}
			fmt.Println(msg)
			os.Exit(0)
		case "--update-all":
			updateAllServices()
			os.Exit(0)
		}
	}

	// On Unix, require root privileges for service management
	if runtime.GOOS != "windows" && os.Getuid() != 0 {
		home := os.Getenv("HOME")
		cmd := exec.Command("sudo", "env", fmt.Sprintf("HOME=%s", home), os.Args[0])
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to elevate privileges: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	otel.Init("pink-orchestrator", version)
	otel.SetServiceNameWidth(registry.MaxServiceNameLen())

	if err := config.EnsureDirs(); err != nil {
		otel.Error(context.Background(), "failed to create directories", otel.Attr{"error", err.Error()})
		os.Exit(1)
	}

	if err := services.AcquireLock(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer services.ReleaseLock()

	services.SetOrchestratorBinaryVersion(version)

	otel.Info(context.Background(), "started "+version, otel.Attr{"port", config.Port()})

	apiServer, err := api.NewServer()
	if err != nil {
		otel.Error(context.Background(), "api server failed", otel.Attr{"error", err.Error()})
		os.Exit(1)
	}
	go apiServer.Start()

	t := tray.New()
	t.Run()
}

func printUsage() {
	fmt.Printf(`pink-orchestrator v%s - System tray manager for pink-tools services

Usage:
  pink-orchestrator                             Start in system tray
  pink-orchestrator --health                    Check health
  pink-orchestrator --version                   Show version
  pink-orchestrator --update-all                Update all installed services
  pink-orchestrator --service-update <name>     Update a service
  pink-orchestrator --service-restart <name>    Restart a service
  pink-orchestrator --service-stop <name>       Stop a service
  pink-orchestrator --service-start <name>      Start a service

Environment:
  ORCHESTRATOR_PORT    API port (default: %d)
`, version, config.DefaultPort)
}

func updateAllServices() {
	otel.Init("pink-orchestrator", version)

	svcs, err := registry.ListServices()
	if err != nil {
		fmt.Printf("Failed to list services: %v\n", err)
		return
	}

	var updated, failed, skipped int
	for _, svc := range svcs {
		if !services.IsInstalled(svc.Name) {
			fmt.Printf("⊘ %s (not installed)\n", svc.Name)
			skipped++
			continue
		}

		fmt.Printf("→ %s: checking...\n", svc.Name)

		err := services.Update(svc.Name, func(msg string) {
			fmt.Printf("  %s\n", msg)
		})

		if err != nil {
			fmt.Printf("✗ %s: %v\n", svc.Name, err)
			failed++
		} else {
			fmt.Printf("✓ %s\n", svc.Name)
			updated++
		}
	}

	fmt.Printf("\nDone: %d updated, %d failed, %d skipped\n", updated, failed, skipped)
}
