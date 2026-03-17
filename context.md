# pink-orchestrator

Service manager for pink-tools. GUI (system tray) on desktop, headless on servers.

Binaries: ~/pink-tools/<name>/<name>

    pink-orchestrator                             Start (tray or headless)
    pink-orchestrator --health                    Check health
    pink-orchestrator --version                   Show version
    pink-orchestrator --update-all                Update all downloaded services
    pink-orchestrator --service-download <name>   Download a service
    pink-orchestrator --service-update <name>     Update a service
    pink-orchestrator --service-restart <name>    Restart a service
    pink-orchestrator --service-stop <name>       Stop a service
    pink-orchestrator --service-start <name>      Start a service
    pink-orchestrator --services                  List downloaded services (JSON)

Environment:
- ORCHESTRATOR_PORT -- API port (default: 7460)
