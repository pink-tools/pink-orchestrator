# pink-orchestrator

Service manager for [pink-tools](https://github.com/pink-tools). Downloads, updates, and manages lifecycle of all services. Works as system tray app on desktop or headless on servers.

## Install

Download binary from [Releases](https://github.com/pink-tools/pink-orchestrator/releases).

## Usage

```bash
pink-orchestrator                              # Start (tray if GUI available, headless otherwise)

# Service management
pink-orchestrator --service-download NAME      # Download service from GitHub
pink-orchestrator --service-start NAME         # Start service
pink-orchestrator --service-stop NAME          # Stop service
pink-orchestrator --service-restart NAME       # Restart service
pink-orchestrator --service-update NAME        # Check and install update
pink-orchestrator --service-reinstall NAME     # Remove and re-download

# Updates
pink-orchestrator --update                     # Update orchestrator itself
pink-orchestrator --update-all                 # Update all downloaded services

# Info
pink-orchestrator --services                   # List downloaded services (JSON)
pink-orchestrator --registry                   # List all available services
pink-orchestrator --version                    # Show version
pink-orchestrator --health                     # Check health
```

On desktop, right-click tray icon to manage services via GUI.

## Services

| Service | Type | Description |
|---------|------|-------------|
| pink-agent | daemon | Telegram bot for Claude Code sessions |
| pink-transcriber | cli | Speech-to-text via whisper.cpp |
| pink-whisper | daemon | whisper.cpp TCP server with auto-setup |
| pink-voice | daemon | Voice input with configurable hotkey |
| pink-elevenlabs | cli | Text-to-speech via ElevenLabs API |

## Server Deployment

```bash
# Download and start on a headless server
pink-orchestrator --service-download pink-agent
pink-orchestrator --service-start pink-agent
```

Install as systemd service for auto-restart on boot.

## Paths

| Item | Path |
|------|------|
| Services | `~/pink-tools/{service}/` |
| State | `~/pink-tools/pink-orchestrator/` |
| Registry | `~/pink-tools/pink-orchestrator/registry.yaml` |

## Build from Source

```bash
git clone https://github.com/pink-tools/pink-orchestrator.git
cd pink-orchestrator
go build .
```
