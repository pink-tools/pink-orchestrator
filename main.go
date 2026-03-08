package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"

	"github.com/pink-tools/pink-core"
	"github.com/pink-tools/pink-core/log"
	"github.com/pink-tools/pink-orchestrator/internal/api"
	"github.com/pink-tools/pink-orchestrator/internal/config"
	"github.com/pink-tools/pink-orchestrator/internal/dialog"
	"github.com/pink-tools/pink-orchestrator/internal/registry"
	"github.com/pink-tools/pink-orchestrator/internal/services"
	"github.com/pink-tools/pink-orchestrator/internal/tray"
)

//go:embed context.md
var claudeContext string

var version = "dev"

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "--dialog":
			dialog.FormMain()
			return
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
		case "--claude":
			fmt.Println(claudeContext)
			return
		case "--services":
			printInstalledServices()
			return
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

	autoInstall()

	log.Init("pink-orchestrator", version)
	log.SetServiceNameWidth(registry.MaxServiceNameLen())

	if err := config.EnsureDirs(); err != nil {
		log.Error(context.Background(), "failed to create directories", log.Attr{K: "error", V: err.Error()})
		os.Exit(1)
	}
	config.MigratePinkToolsDir()
	config.MigrateStateDir()
	config.MigrateClaudeDirs()
	config.MigrateWhisperModel()

	if err := services.AcquireLock(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer services.ReleaseLock()

	services.SetOrchestratorBinaryVersion(version)

	log.Info(context.Background(), "started "+version, log.Attr{K: "port", V: config.Port()})

	apiServer, err := api.NewServer()
	if err != nil {
		log.Error(context.Background(), "api server failed", log.Attr{K: "error", V: err.Error()})
		os.Exit(1)
	}
	go apiServer.Start()

	t := tray.New()
	t.Run()
}

func autoInstall() {
	currentPath, err := os.Executable()
	if err != nil {
		return
	}
	currentPath, err = filepath.EvalSymlinks(currentPath)
	if err != nil {
		return
	}

	expectedPath := core.BinaryPath("pink-orchestrator")
	if currentPath == expectedPath {
		return
	}

	targetDir := filepath.Dir(expectedPath)
	fmt.Printf("Installing to %s...\n", targetDir)

	if err := os.MkdirAll(targetDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create directory: %v\n", err)
		os.Exit(1)
	}

	data, err := os.ReadFile(currentPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to read binary: %v\n", err)
		os.Exit(1)
	}

	if err := os.WriteFile(expectedPath, data, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to write binary: %v\n", err)
		os.Exit(1)
	}

	// Chown to real user so the dir isn't stuck as root-only
	if sudoUser := os.Getenv("SUDO_USER"); sudoUser != "" {
		if u, err := user.Lookup(sudoUser); err == nil {
			uid, _ := strconv.Atoi(u.Uid)
			gid, _ := strconv.Atoi(u.Gid)
			os.Chown(core.PinkToolsDir(), uid, gid)
			filepath.Walk(targetDir, func(path string, info os.FileInfo, err error) error {
				if err == nil {
					os.Chown(path, uid, gid)
				}
				return nil
			})
		}
	}

	fmt.Println("Installed. Restarting from new location...")

	if err := services.ExecPath(expectedPath); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to exec: %v\n", err)
		os.Exit(1)
	}
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
  pink-orchestrator --services                  List installed services (JSON)
  pink-orchestrator --claude                    Print agent context

Environment:
  ORCHESTRATOR_PORT    API port (default: %d)
`, version, config.DefaultPort)
}

func printInstalledServices() {
	svcs, err := registry.ListServices()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to list services: %v\n", err)
		os.Exit(1)
	}
	var installed []string
	for _, svc := range svcs {
		if services.IsInstalled(svc.Name) {
			installed = append(installed, svc.Name)
		}
	}
	data, _ := json.Marshal(installed)
	fmt.Println(string(data))
}

func updateAllServices() {
	log.Init("pink-orchestrator", version)

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
