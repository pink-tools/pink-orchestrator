# pink-orchestrator

System tray manager for [pink-tools](https://github.com/pink-tools) services.

## Install

Download binary from [Releases](https://github.com/pink-tools/pink-orchestrator/releases).

## Usage

```bash
pink-orchestrator                         # Start in system tray
pink-orchestrator --health                # Check health
pink-orchestrator --version               # Show version

# CLI service management
pink-orchestrator --service-start NAME    # Start service
pink-orchestrator --service-stop NAME     # Stop service
pink-orchestrator --service-restart NAME  # Restart service
pink-orchestrator --service-update NAME   # Update service

# Self-update
pink-orchestrator --update                # Update orchestrator itself
```

Right-click tray icon to:
- Install/uninstall services
- Start/stop/restart services
- Edit .env configuration
- Check for updates

## Services

| Service | Type | Description |
|---------|------|-------------|
| pink-transcriber | daemon | Speech-to-text via whisper.cpp |
| pink-voice | daemon | Voice input with Ctrl+Q hotkey |
| pink-elevenlabs | cli | Text-to-speech via ElevenLabs API |
| pink-agent | daemon | Telegram bot for Claude Code |

## Paths

| Item | Path |
|------|------|
| Services | `~/pink-tools/{service}/` |
| State | `~/.pink-orchestrator/` |

## Build from Source

```bash
git clone https://github.com/pink-tools/pink-orchestrator.git
cd pink-orchestrator
go build .
```
