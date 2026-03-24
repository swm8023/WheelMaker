# WheelMaker — Server

Go daemon bridging local AI CLIs (Codex, Claude) to remote IM channels (Feishu, mobile WebSocket, console).

## Architecture

```
Hub
 ├─ im/forwarder.Forwarder ── console | feishu | mobile   (IM layer)
 └─ client.Client ── acp.Forwarder ── acp.Conn ── CLI subprocess  (agent layer)
```

## Package Map

| Package | Role |
|---------|------|
| `internal/hub/` | Reads config, spawns Client + IM per project |
| `internal/client/` | Command routing, session lifecycle, state persistence |
| `internal/acp/` | JSON-RPC 2.0 transport over stdio |
| `internal/agent/claude/` | Launches claude-agent-acp subprocess |
| `internal/agent/codex/` | Launches codex-acp subprocess |
| `internal/im/forwarder/` | Message dedup/filter, pending decisions, HelpResolver |
| `internal/im/console/` | Console IM adapter (reads stdin) |
| `internal/im/feishu/` | Feishu Bot IM adapter |
| `internal/im/mobile/` | WebSocket IM adapter for mobile app |
| `internal/tools/` | Binary path resolver (`bin/{GOOS}_{GOARCH}/`) |

## Config Files

- `~/.wheelmaker/config.json` — project config (IM type, agent, working dir, yolo)
- `~/.wheelmaker/state.json` — runtime state (session IDs, agent metadata)

```json
{
  "projects": [
    { "name": "local", "debug": true, "path": "/your/project", "yolo": true,
      "im": { "type": "console" }, "client": { "agent": "claude" } },
    { "name": "mobile", "path": "/your/project", "yolo": false,
      "im": { "type": "mobile", "mobile": { "port": 9527, "token": "change-me" } },
      "client": { "agent": "claude" } }
  ]
}
```

## Mobile WebSocket Protocol

Connect to `ws://<host>:<port>/ws`:

```
Server → { "type": "auth_required" }
Client → { "type": "auth", "token": "..." }
Server → { "type": "ready", "chatId": "..." }
```

Inbound: `auth` / `message` / `option` / `ping`
Outbound: `text` / `card` / `options` / `debug` / `pong` / `error` / `ready` / `auth_required`

Decision flow: `options` carries `decisionId` → client sends `{type:"option", decisionId, optionId}`

## state.go Design

```
FileState
  └─ Projects map[name]*ProjectState
       ├─ ActiveAgent string
       ├─ Connection *ConnectionConfig
       └─ Agents map[name]*AgentState
            ├─ LastSessionID
            ├─ ProtocolVersion / AgentCapabilities / AgentInfo / AuthMethods
            ├─ Session *SessionState
            └─ Sessions []SessionSummary
```

## Dev Conventions

- Interfaces first: `acp.Session`, `agent.Agent`, `im.Channel`
- Agent subprocess is lazy: created on first message (`ensureForwarder`)
- Slash commands: `/use` `/cancel` `/status` `/mode` `/model` `/list` `/new` `/load` `/debug`
- Code comments and identifiers: **English only**
- **After every change: `git add` → `git commit` → `git push`**

## Local Dev

```bash
# in server/
go run ./cmd/wheelmaker/            # requires ~/.wheelmaker/config.json
go test ./...
go build -o bin/windows_amd64/wheelmaker.exe ./cmd/wheelmaker/
GOOS=linux  GOARCH=amd64 go build -o bin/linux_amd64/wheelmaker  ./cmd/wheelmaker/
GOOS=darwin GOARCH=arm64 go build -o bin/darwin_arm64/wheelmaker ./cmd/wheelmaker/

# Install ACP tools (first time)
pwsh scripts/install-tools.ps1   # Windows
bash scripts/install-tools.sh    # macOS / Linux
```

## Process Rule

After every code change: stop existing wheelmaker processes, run `go run ./cmd/wheelmaker/`, confirm only one process is running.
During live debugging: send user-facing reply first, then trigger `server/scripts/delayed-restart.ps1` in background (kills + restarts after 30s).
Windows: `powershell -NoProfile -ExecutionPolicy Bypass -File server/scripts/delayed-restart.ps1`

## Key Invariants (do not break)

| # | Invariant |
|---|-----------|
| 1 | `acp.Conn` is pure transport — no business logic inside |
| 2 | `client.Client` is the single owner of session state; IM adapters never mutate it directly |
| 3 | Agent subprocess is created lazily — never at startup |
| 4 | All cross-layer deps injected via interfaces (`acp.Session`, `agent.Agent`, `im.Channel`) |
| 5 | `state.json` is the source of truth for runtime state |
| 6 | Decision messages carry a `decisionId`; mobile adapter resolves via pending-decision map |

## Key Protocol Docs

- ACP protocol: [../docs/acp-protocol-full.zh-CN.md](../docs/acp-protocol-full.zh-CN.md)
- Feishu Bot: [../docs/feishu-bot.md](../docs/feishu-bot.md)