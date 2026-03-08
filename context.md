# pink-orchestrator

System tray manager for pink-tools services.

    pink-orchestrator                             Start in system tray
    pink-orchestrator --health                    Check health
    pink-orchestrator --version                   Show version
    pink-orchestrator --update-all                Update all installed services
    pink-orchestrator --service-update <name>     Update a service
    pink-orchestrator --service-restart <name>    Restart a service
    pink-orchestrator --service-stop <name>       Stop a service
    pink-orchestrator --service-start <name>      Start a service
    pink-orchestrator --services                  List installed services (JSON)

Environment:
- ORCHESTRATOR_PORT -- API port (default: 7460)
