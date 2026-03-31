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
Guardian (-d)
  ├── Hub Worker
  │     ├── Project A  [IM: Feishu]   [Agent: Claude]
  │     ├── Project B  [IM: Mobile]   [Agent: Copilot]
  │     └── ...
  └── Registry Worker (optional)
        └── WebSocket server for app/web clients
```

## Quick Start

### 1. Pull, Build, Install & Restart

Requires **Go 1.22+** and **Node.js 22+**.

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/refresh_server.ps1
```

The refresh script will:
- Pull the latest code with `git pull --ff-only` when the worktree is clean
- Install ACP CLI dependencies (`codex-acp`, `claude-agent-acp`) if missing
- Build `server\bin\windows_amd64\wheelmaker.exe`
- Install the binary to `~/.wheelmaker\bin\`
- Preserve an existing `~\.wheelmaker\config.json`, or create one from `server\config.example.json`
- Generate `start.bat` / `stop.bat` / `restart.bat`
- Restart the daemon after install

If `config.json` is created for the first time, the script stops before restart so you can edit it safely, then rerun the same command.

### 2. Configure

Copy the example config and fill in your credentials:

```powershell
notepad ~\.wheelmaker\config.json
```

<details>
<summary>Config reference</summary>

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

</details>

### 3. Run

Double-click or run from terminal:

```powershell
~\.wheelmaker\start.bat     # start
~\.wheelmaker\stop.bat      # stop
~\.wheelmaker\restart.bat   # restart
```

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
- `deploy.bat` — one-click manual deploy
- `deploy_everyday.bat` — register nightly auto-update (3 AM)
- `scripts\refresh_server.ps1` — core deploy orchestrator (git pull + build + install + restart)
- `scripts\delay_restart_server.ps1` — delayed restart (used by CI completion gate)
- `scripts\auto_update.ps1` — git update check + deploy; also manages scheduled task (`-Setup` / `-Uninstall`)

## License

Private — all rights reserved.
