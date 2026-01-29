# pink-orchestrator

`~/pink-tools/pink-orchestrator` | Go

System tray manager for pink-tools services.

```bash
pink-orchestrator          # start in system tray
pink-orchestrator --health # check health
pink-orchestrator --version
```

Right-click tray icon to:
- Install/uninstall services
- Start/stop/restart daemons
- Edit .env files
- Check for updates

## Structure

```
~/.pink-orchestrator/
├── state.json           # running services, installed versions
├── registry.yaml        # cached service definitions
└── orchestrator.sock    # IPC socket

~/pink-tools/
├── pink-transcriber/
├── pink-voice/
├── pink-elevenlabs/
└── pink-agent/
```

## Build

```bash
cd ~/Desktop/_claude/pink-orchestrator
go build .
```
