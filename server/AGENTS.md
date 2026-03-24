# WheelMaker Server — AI Agent Guide

Go daemon that connects local AI CLIs (via ACP / JSON-RPC 2.0 over stdio) to IM channels
(Feishu, mobile WebSocket, console).  One daemon can serve multiple projects simultaneously.

## Architecture at a Glance

```
Hub
 ├─ im/forwarder.Forwarder ─── console | feishu | mobile   (IM layer)
 └─ client.Client ─── acp.Forwarder ─── acp.Conn ─── CLI subprocess  (agent layer)
```

## Key Packages (quick map)

| Path | Role |
|------|------|
| `internal/hub/` | Reads config, spawns Client + IM per project |
| `internal/client/` | Command routing, session lifecycle, state persistence |
| `internal/acp/` | JSON-RPC 2.0 transport over stdio |
| `internal/agent/` | `claude` / `codex` subprocess launchers |
| `internal/im/` | `forwarder` / `console` / `feishu` / `mobile` adapters |
| `internal/tools/` | Binary path resolver (`bin/{GOOS}_{GOARCH}/`) |

## Server-specific Constraints

- Interfaces first: cross-layer deps via `acp.Session`, `agent.Agent`, `im.Channel`
- Agent subprocess is lazy: created on first message (`ensureForwarder`)
- Supported slash commands: `/use` `/cancel` `/status` `/mode` `/model` `/list` `/new` `/load` `/debug`
- Other `/`-prefixed text is forwarded as plain message

## Dev Quick Start

```bash
# in server/
go run ./cmd/wheelmaker/            # requires ~/.wheelmaker/config.json
go test ./...
go build -o bin/windows_amd64/wheelmaker.exe ./cmd/wheelmaker/
```

## Mandatory Preflight (every session)

1. Read [`CLAUDE.md`](../CLAUDE.md) — global conventions.
2. Read [`server/CLAUDE.md`](CLAUDE.md) — full architecture, state design, WebSocket protocol, build matrix.
3. Confirm before acting: `READ_OK: CLAUDE.md + server/CLAUDE.md`

Do not skip the above even if context seems sufficient.

