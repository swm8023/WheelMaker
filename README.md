# WheelMaker

WheelMaker is a self-hosted daemon that bridges local AI coding agents (Claude Code, OpenAI Codex, GitHub Copilot) to instant messaging platforms. Send instructions from Feishu/Lark or the mobile app, and let AI agents execute coding tasks on your local projects remotely.

## Features

- **Multi-project** — manage multiple projects in a single daemon, each with its own IM channel and agent.
- **Multiple agents** — supports Claude Code, OpenAI Codex, and GitHub Copilot. Switch agents at runtime via `/use <agent>`.
- **Multiple IM channels** — Feishu/Lark bot (rich cards, streaming, decision prompts), console (local debug), and mobile app (WebSocket).
- **Session persistence** — sessions survive daemon restarts.
- **Lazy start & idle reclaim** — agent processes spawn on first message and are reclaimed when idle.
- **Registry (optional)** — WebSocket-based project discovery and file/git proxy for the web dashboard and mobile app.

## Architecture

```
IM (Feishu / Console / Mobile App)
  → Hub (WheelMaker daemon)
    → Agent Adapter (Claude / Codex / Copilot via ACP)
      → Local Project Workspace
```

In daemon mode (`-d`), a guardian process supervises hub and registry workers, restarting them if they crash.

```
Guardian (-d)
  ├── Hub Worker (--hub-worker)
  │     ├── Project Client 1  [IM: Feishu]  [Agent: Claude]
  │     ├── Project Client 2  [IM: Mobile]  [Agent: Copilot]
  │     └── ...
  └── Registry Worker (--registry-worker)  (optional)
        └── WebSocket server for app/web clients
```

## Repository Structure

```
WheelMaker/
  server/   — Go daemon (hub, agent adapters, IM adapters, registry)
  app/      — React Native mobile app + web dashboard
  docs/     — Protocol specs and design docs
  scripts/  — Build, install, and service management scripts
```

## Getting Started

### Prerequisites

- **Go 1.22+**
- **Node.js 22+** (for ACP agent CLIs and the web dashboard)

Install the ACP agent CLIs:

```bash
npm install -g @zed-industries/codex-acp @zed-industries/claude-agent-acp
```

Or let the install script handle it automatically.

### Build & Install (Windows)

```powershell
# Build
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/build_server.ps1

# Install (copies binary, installs npm deps, generates config and service scripts)
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/install_server.ps1
```

### Configuration

On first install, copy the example config and edit it:

```powershell
Copy-Item server\config.example.json ~\.wheelmaker\config.json
notepad ~\.wheelmaker\config.json
```

Example `config.json`:

```json
{
  "projects": [
    {
      "name": "my-project",
      "path": "C:\\Code\\my-project",
      "yolo": false,
      "im": {
        "type": "feishu",
        "appID": "cli_xxx",
        "appSecret": "yyy"
      },
      "client": {
        "agent": "claude"
      }
    }
  ],
  "registry": {
    "listen": false,
    "port": 9630,
    "server": "127.0.0.1",
    "token": "",
    "hubId": ""
  },
  "log": {
    "level": "warn"
  }
}
```

| Field | Description |
|-------|-------------|
| `projects[].name` | Project identifier |
| `projects[].path` | Working directory for agent sessions |
| `projects[].yolo` | Auto-approve all tool permissions when `true` |
| `projects[].im.type` | `"feishu"`, `"console"`, or `"mobile"` |
| `projects[].client.agent` | `"claude"`, `"codex"`, or `"copilot"` |
| `registry.listen` | Start the built-in registry server when `true` |
| `log.level` | `"debug"`, `"info"`, `"warn"`, or `"error"` |

### Start / Stop / Restart

The install script generates service scripts in `~/.wheelmaker/` (double-click to run):

| Script | Description |
|--------|-------------|
| `start.bat` | Start daemon in background (skips if already running) |
| `stop.bat` | Stop all wheelmaker processes |
| `restart.bat` | Stop then start |

```powershell
~\.wheelmaker\start.bat     # start
~\.wheelmaker\stop.bat      # stop
~\.wheelmaker\restart.bat   # restart
```

### Development

Run in foreground (no guardian, single hub worker):

```bash
cd server
go run ./cmd/wheelmaker
```

Run tests:

```bash
cd server
go test ./...
```

## Chat Commands

Once connected through an IM channel, the following commands are available:

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
| `/config` | Show current configuration |
| `/debug` | Toggle debug logging |
| `/help` | Show command list |

## File Locations

| File | Path |
|------|------|
| Config | `~/.wheelmaker/config.json` |
| State | `~/.wheelmaker/state.json` |
| Binary | `~/.wheelmaker/bin/wheelmaker.exe` |
| Hub log | `~/.wheelmaker/hub.log` |
| Registry log | `~/.wheelmaker/registry.log` |

## License

Private — all rights reserved.
