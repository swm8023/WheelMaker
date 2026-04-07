# WheelMaker - Server

Go daemon bridging local AI CLIs (Codex, Claude) to remote IM channels (Feishu, app stub), with independent registry sync.

## Architecture

Architecture 3.0 — multi-session with per-agent isolation:

```
Hub
 +- im.Router -- feishu | app   (formal IM layer)
 +- registry.Reporter -- registry server (project snapshot sync)
 +- client.Client -- routeMap[routeKey] -> Session
       +- Session -- AgentInstance -- AgentConn -- acp.Forwarder -- acp.Conn -- CLI subprocess
       +- SessionStore (SQLite) -- persist/restore sessions
```

Key types in `internal/hub/client/`:

| Type | File | Role |
|------|------|------|
| Session | session_type.go, session.go | Pure business session: lifecycle, prompt, agent switching |
| AgentInstance | agent_instance.go | ACP executor bound to one Session (sole ACP interface visible to Session) |
| AgentConn | agent_conn.go | ACP connection wrapper with ConnOwned/ConnShared modes |
| AgentFactory | agent_factory.go | Creates AgentInstance, selects AgentConn policy |
| SessionStore | session_store.go | Persistence interface for session snapshots |
| SQLiteSessionStore | sqlite_store.go | SQLite-backed SessionStore (modernc.org/sqlite, CGo-free) |

Full design: [../docs/architecture-3.0.md](../docs/architecture-3.0.md)

## Package Map

| Package | Role |
|---------|------|
| `internal/hub/` | Hub process domain (orchestration + per-project runtime) |
| `internal/hub/client/` | Command routing, session lifecycle, state persistence |
| `internal/hub/agent/` | Unified ACP agent layer: provider, process, conn (owned/shared), instance, factory |
| `internal/im/` | Formal IM runtime, router, and channels |
| `internal/registry/` | Registry server and hub reporter |
| `internal/shared/` | Shared config, logging, and registry protocol helpers |

## Config Files

- `~/.wheelmaker/config.json` - project config (IM type, agent, working dir, yolo)
- `~/.wheelmaker/state.json` - runtime state (session IDs, agent metadata)

## Dev Conventions

- Interfaces first: `acp.Session`, `agent.Agent`, `im.Channel`
- Agent subprocess is lazy: created on first message (`ensureInstance`)
- Slash commands: `/use` `/cancel` `/status` `/mode` `/model` `/list` `/new` `/load` `/config`
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
powershell -NoProfile -ExecutionPolicy Bypass -File ../scripts/signal_update_now.ps1 -DelaySeconds 30   # async manual updater trigger
powershell -NoProfile -ExecutionPolicy Bypass -File ../scripts/refresh_server.ps1 -SkipUpdate -SkipBuild -SkipDeploy    # restart services only
powershell -NoProfile -ExecutionPolicy Bypass -File ../scripts/refresh_server.ps1 -SkipUpdate -SkipBuild -SkipDeploy -SkipRestart # stop services only
# deployed wrappers under ~/.wheelmaker: start.bat / stop.bat / refresh_server.ps1
```

## Key Invariants (do not break)

| # | Invariant |
|---|-----------|
| 1 | `acp.Conn` is pure transport - no business logic inside |
| 2 | `client.Client` owns routing (routeKey → Session); Session owns agent lifecycle and state |
| 3 | Agent subprocess is created lazily - never at startup (`ensureInstance`) |
| 4 | All cross-layer deps injected via interfaces (`acp.Session`, `agent.Agent`, `im.Channel`) |
| 5 | `state.json` is the source of truth for runtime state; `SQLiteSessionStore` for session persistence |
| 6 | Registry sync is independent of IM mode (`registry.listen=true` local, otherwise remote connect) |

## Key Protocol Docs

- ACP protocol: [../docs/acp-protocol-full.zh-CN.md](../docs/acp-protocol-full.zh-CN.md)
- Feishu Bot: [../docs/feishu-bot.md](../docs/feishu-bot.md)
- Remote Observe / Registry: [../docs/registry-server-remote-observe-protocol-v1.md](../docs/registry-server-remote-observe-protocol-v1.md)

