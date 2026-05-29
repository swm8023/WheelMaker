# WheelMaker - Server

Go daemon bridging local AI CLIs (Codex, Claude) to Workspace App sessions through the Registry.

## Architecture

App-only session runtime with per-agent isolation:

```text
Hub
 +- registry.Reporter -- registry server (project snapshot + session event sync)
 +- client.Client -- Session by sessionId
       +- Session -- AgentInstance -- AgentConn -- acp.Forwarder -- acp.Conn -- CLI subprocess
       +- SessionStore (SQLite) -- persist/restore sessions
```

Key types in `internal/hub/client/`:

| Type | File | Role |
|------|------|------|
| Session | session_type.go, session.go | Business session: lifecycle, prompt, agent switching |
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
| `internal/hub/client/` | Session lifecycle, ACP prompt handling, state persistence |
| `internal/hub/agent/` | Unified ACP agent layer: provider, process, conn (owned/shared), instance, factory |
| `internal/registry/` | Registry server and hub reporter |
| `internal/shared/` | Shared config, logging, and registry protocol helpers |

## Config Files

- `~/.wheelmaker/config.json` - project config (agent, working dir, registry/monitor settings)
- `~/.wheelmaker/state.json` - runtime state (session IDs, agent metadata)

## Dev Conventions

- Interfaces first: ACP transport interfaces and agent factories stay injected across layers
- Agent subprocess is lazy: created on first message (`ensureInstance`)
- App conversations use `projectId + sessionId`; session control goes through `session.*` Registry methods
- Code comments and identifiers: **English only**
- Prefer extending existing `*_test.go` files instead of adding new test files unless separation is clearly justified
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
../deploy.bat                    # build temporary wheelmaker-deploy and run full deploy
../deploy.sh                     # macOS/Linux full deploy
# deployed wrappers under ~/.wheelmaker: start/stop/restart/status .bat and .sh
```

## Key Invariants (do not break)

| # | Invariant |
|---|-----------|
| 1 | `acp.Conn` is pure transport - no business logic inside |
| 2 | `client.Client` owns session lookup by sessionId; Session owns agent lifecycle and state |
| 3 | Agent subprocess is created lazily - never at startup (`ensureInstance`) |
| 4 | Session output flows through `SessionRecorder` to Registry `session.*` events |
| 5 | `state.json` is the source of truth for runtime state; `SQLiteSessionStore` for session persistence |
| 6 | Registry sync is the only App conversation transport |

## Key Protocol Docs

- ACP protocol: [../docs/acp-protocol-full.zh-CN.md](../docs/acp-protocol-full.zh-CN.md)
- Remote Observe / Registry: [../docs/registry-server-remote-observe-protocol-v1.md](../docs/registry-server-remote-observe-protocol-v1.md)
