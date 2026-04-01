# WheelMaker - Server

Go daemon bridging local AI CLIs (Codex, Claude) to remote IM channels (Feishu, console), with independent registry sync.

## Architecture

```
Hub
 +- im/forwarder.Forwarder -- console | feishu   (IM layer)
 +- registry.Reporter -- registry server (project snapshot sync)
 +- client.Client -- acp.Forwarder -- acp.Conn -- CLI subprocess  (agent layer)
```

## Package Map

| Package | Role |
|---------|------|
| `internal/hub/` | Hub process domain (orchestration + per-project runtime) |
| `internal/hub/client/` | Command routing, session lifecycle, state persistence |
| `internal/hub/acp/` | JSON-RPC 2.0 transport over stdio |
| `internal/hub/agent/claude/` | Launches claude-agent-acp subprocess |
| `internal/hub/agent/codex/` | Launches codex-acp subprocess |
| `internal/hub/im/` | IM adapters and forwarder |
| `internal/registry/` | Registry server and hub reporter |
| `internal/hub/tools/` | Binary path resolver (`bin/{GOOS}_{GOARCH}/`) |
| `internal/shared/` | Shared config, logging, and registry protocol helpers |
| `cmd/wheelmaker-updater/` | Daily auto-update Windows service |

## Config Files

- `~/.wheelmaker/config.json` - project config (IM type, agent, working dir, yolo)
- `~/.wheelmaker/state.json` - runtime state (session IDs, agent metadata)

## Dev Conventions

- Interfaces first: `acp.Session`, `agent.Agent`, `im.Channel`
- Agent subprocess is lazy: created on first message (`ensureForwarder`)
- Slash commands: `/use` `/cancel` `/status` `/mode` `/model` `/list` `/new` `/load`
- Code comments and identifiers: **English only**
- Completion gate is defined in repo root `CLAUDE.md`

## Local Dev

```bash
# in server/
go run ./cmd/wheelmaker/            # requires ~/.wheelmaker/config.json
go test ./...
go build -o bin/windows_amd64/wheelmaker.exe ./cmd/wheelmaker/
go build -o bin/windows_amd64/wheelmaker-monitor.exe ./cmd/wheelmaker-monitor/
go build -o bin/windows_amd64/wheelmaker-updater.exe ./cmd/wheelmaker-updater/

# Root-level helper scripts
powershell -NoProfile -ExecutionPolicy Bypass -File ../scripts/refresh_server.ps1                      # full deploy (services)
powershell -NoProfile -ExecutionPolicy Bypass -File ../scripts/delay_restart_server.ps1                 # delayed refresh + service restart
powershell -NoProfile -ExecutionPolicy Bypass -File ../scripts/refresh_server.ps1 -AutoUpdate on -UpdateTime 03:00      # enable updater service
powershell -NoProfile -ExecutionPolicy Bypass -File ../scripts/refresh_server.ps1 -AutoUpdate off                       # disable updater service
~/.wheelmaker/bin/wheelmaker-updater.exe --repo D:\Code\WheelMaker --install-dir ~/.wheelmaker/bin --time 03:00 --once # run updater one-shot
```

## Key Invariants (do not break)

| # | Invariant |
|---|-----------|
| 1 | `acp.Conn` is pure transport - no business logic inside |
| 2 | `client.Client` is the single owner of session state; IM adapters never mutate it directly |
| 3 | Agent subprocess is created lazily - never at startup |
| 4 | All cross-layer deps injected via interfaces (`acp.Session`, `agent.Agent`, `im.Channel`) |
| 5 | `state.json` is the source of truth for runtime state |
| 6 | Registry sync is independent of IM mode (`registry.listen=true` local, otherwise remote connect) |

## Key Protocol Docs

- ACP protocol: [../docs/acp-protocol-full.zh-CN.md](../docs/acp-protocol-full.zh-CN.md)
- Feishu Bot: [../docs/feishu-bot.md](../docs/feishu-bot.md)
- Remote Observe / Registry: [../docs/registry-server-remote-observe-protocol-v1.md](../docs/registry-server-remote-observe-protocol-v1.md)
