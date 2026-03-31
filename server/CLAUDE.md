# WheelMaker � Server

Go daemon bridging local AI CLIs (Codex, Claude) to remote IM channels (Feishu, console), with independent registry sync.

## Architecture

```
Hub
 +- im/forwarder.Forwarder -- console | feishu   (IM layer)
 +- registry.Reporter --? registry server (project snapshot sync)
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

## Config Files

- `~/.wheelmaker/config.json` � project config (IM type, agent, working dir, yolo)
- `~/.wheelmaker/state.json` � runtime state (session IDs, agent metadata)

```json
{
  "projects": [
    { "name": "local", "debug": true, "path": "/your/project", "yolo": true,
      "im": { "type": "console" }, "client": { "agent": "claude" } },
    { "name": "prod", "path": "/your/project", "yolo": false,
      "im": { "type": "feishu", "appID": "cli_xxx", "appSecret": "yyy" },
      "client": { "agent": "claude" } }
  ],
  "registry": { "listen": true, "port": 9630, "server": "127.0.0.1" }
}
```

## state.go Design

```
FileState
  +- Projects map[name]*ProjectState
       +- ActiveAgent string
       +- Connection *ConnectionConfig
       +- Agents map[name]*AgentState
            +- LastSessionID
            +- ProtocolVersion / AgentCapabilities / AgentInfo / AuthMethods
            +- Session *SessionState
            +- Sessions []SessionSummary
```

## Dev Conventions

- Interfaces first: `acp.Session`, `agent.Agent`, `im.Channel`
- Agent subprocess is lazy: created on first message (`ensureForwarder`)
- Slash commands: `/use` `/cancel` `/status` `/mode` `/model` `/list` `/new` `/load` `/debug`
- Code comments and identifiers: **English only**
- **After every change: `git add` ? `git commit` ? `git push`**
- Completion gate is defined in repo root `CLAUDE.md`

## Local Dev

```bash
# in server/
go run ./cmd/wheelmaker/            # requires ~/.wheelmaker/config.json
go test ./...
go build -o bin/windows_amd64/wheelmaker.exe ./cmd/wheelmaker/
GOOS=linux  GOARCH=amd64 go build -o bin/linux_amd64/wheelmaker  ./cmd/wheelmaker/
GOOS=darwin GOARCH=arm64 go build -o bin/darwin_arm64/wheelmaker ./cmd/wheelmaker/

# Root-level helper scripts
powershell -NoProfile -ExecutionPolicy Bypass -File ../scripts/refresh_server.ps1           # full deploy
powershell -NoProfile -ExecutionPolicy Bypass -File ../scripts/delay_restart_server.ps1      # delayed restart
powershell -NoProfile -ExecutionPolicy Bypass -File ../scripts/auto_update.ps1               # git check + deploy
powershell -NoProfile -ExecutionPolicy Bypass -File ../scripts/auto_update.ps1 -Setup        # register scheduled task
```

## Key Invariants (do not break)

| # | Invariant |
|---|-----------|
| 1 | `acp.Conn` is pure transport � no business logic inside |
| 2 | `client.Client` is the single owner of session state; IM adapters never mutate it directly |
| 3 | Agent subprocess is created lazily � never at startup |
| 4 | All cross-layer deps injected via interfaces (`acp.Session`, `agent.Agent`, `im.Channel`) |
| 5 | `state.json` is the source of truth for runtime state |
| 6 | Registry sync is independent of IM mode (`registry.listen=true` local, otherwise remote connect) |

## Key Protocol Docs

- ACP protocol: [../docs/acp-protocol-full.zh-CN.md](../docs/acp-protocol-full.zh-CN.md)
- Feishu Bot: [../docs/feishu-bot.md](../docs/feishu-bot.md)
- Remote Observe / Registry: [../docs/registry-server-remote-observe-protocol-v1.md](../docs/registry-server-remote-observe-protocol-v1.md)

