# WheelMaker

A self-hosted daemon that turns your phone into a remote AI coding assistant.
Stop reinventing the wheel, start making your own.

```
Feishu / Mobile App ──► WheelMaker ──► Claude / Codex / Copilot ──► your codebase
```

## Features

- **Multi-project** — manage multiple projects in a single daemon, each with its own IM channel and agent.
- **Multiple agents** — supports Claude Code, OpenAI Codex, and GitHub Copilot. Switch at runtime via `/use <agent>`.
- **Multiple IM channels** — Feishu/Lark bot (rich cards, streaming, decision prompts), console (local debug), and mobile app (WebSocket).
- **Session persistence** — sessions survive daemon restarts.
- **Lazy start & idle reclaim** — agent processes spawn on first message and are reclaimed when idle.
- **Registry (optional)** — WebSocket-based project discovery and file/git proxy for the web dashboard and mobile app.

## Architecture

```
Windows Services
  ├── WheelMaker (guardian)
  │     ├── Hub Worker
  │     └── Registry Worker (optional)
  └── WheelMakerMonitor
```

## Quick Start

### 1. Pull, Build, Install & Restart

Requires **Go 1.22+** and **Node.js 22+**.

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/refresh_server.ps1
```

The refresh script will:
- Pull latest code with `git pull --ff-only` when worktree is clean
- Install ACP CLI dependencies (`codex-acp`, `claude-agent-acp`) if missing
- Build `wheelmaker.exe` and `wheelmaker-monitor.exe`
- Install binaries to `~/.wheelmaker\bin\`
- Preserve `~/.wheelmaker\config.json`, or create one from `server\config.example.json`
- Generate `start.bat` / `stop.bat` / `restart.bat` / `update-restart.bat` service wrappers
- Register or update Windows services: `WheelMaker`, `WheelMakerMonitor`
- Start services (auto-start enabled)

If `config.json` is created for the first time, the script stops before restart so you can edit it safely, then rerun the same command.

### 2. Configure

Copy the example config and fill in your credentials:

```powershell
notepad ~/.wheelmaker/config.json
```

### 3. Service Operations

```powershell
~/.wheelmaker/start.bat     # start services
~/.wheelmaker/stop.bat      # stop services
~/.wheelmaker/restart.bat   # restart services
~/.wheelmaker/update-restart.bat  # update + build + deploy + restart
```

Default refresh flow: `update -> build -> stop -> deploy -> restart`.

You can skip each stage with:
- `-SkipUpdate`
- `-SkipBuild`
- `-SkipStop`
- `-SkipDeploy`
- `-SkipRestart`

## Chat Commands

| Command | Description |
|---------|-------------|
| `/use <agent>` | Switch AI agent (claude, codex, copilot) |
| `/new` | Start a new session |
| `/list` | List saved sessions |
| `/load <id>` | Resume a saved session |
| `/cancel` | Cancel the current agent operation |
| `/status` | Show project and agent status |
| `/mode` | Toggle YOLO mode |
| `/model` | Switch agent model |
| `/help` | Show all commands |

## Repository Structure

```
WheelMaker/
  server/   — Go daemon (hub, agent adapters, IM adapters, registry)
  app/      — React Native mobile app + web dashboard
  docs/     — Protocol specs and design docs
  scripts/  — Build, install, and service management scripts
```

## Development

```bash
cd server
go run ./cmd/wheelmaker    # run in foreground
go test ./...              # run tests
```

Scripts overview:
- `scripts\refresh_server.ps1` — service-first deploy (build + install + service registration)
- `scripts\delay_restart_server.ps1` — delayed refresh + service restarts

## License

Private — all rights reserved.
