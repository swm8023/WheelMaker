# WheelMaker â€” Server

Go å®ˆæŠ¤è¿›ç¨‹ï¼šè¿žæŽ¥æœ¬åœ° AI CLIï¼ˆCodexã€Claude ç­‰ï¼‰ä¸Žè¿œç«¯ IMï¼ˆé£žä¹¦ã€ç§»åŠ¨ App ç­‰ï¼‰ã€‚

## æž¶æž„å±‚æ¬¡

```
Hub (internal/hub/)             â€” å¤š project ç”Ÿå‘½å‘¨æœŸç®¡ç†ï¼Œè¯» config.json
  â”œâ”€ im/forwarder.Forwarder     â€” IM åŒ…è£…å±‚ï¼šæ¶ˆæ¯è·¯ç”±ã€å†³ç­–è¯·æ±‚ç®¡ç†ã€HelpResolver
  â”‚    â””â”€ console / feishu / mobile  â€” åº•å±‚ IM é€‚é…å™¨
  â””â”€ client.Client              â€” å• project ä¸»æŽ§ï¼šå‘½ä»¤è·¯ç”±ã€ä¼šè¯ç®¡ç†ã€çŠ¶æ€æŒä¹…åŒ–
       â””â”€ acp.Forwarder         â€” ACP åè®®å°è£…ï¼šç±»åž‹åŒ–å‡ºç«™æ–¹æ³•ã€ClientCallbacks åˆ†å‘
            â””â”€ acp.Conn         â€” JSON-RPC 2.0 over stdio â†’ CLI binary
```

## åŒ…èŒè´£

| åŒ… | èŒè´£ |
|----|------|
| `internal/hub/` | è¯» `~/.wheelmaker/config.json`ï¼Œä¸ºæ¯ä¸ª project åˆ›å»º Client + IM |
| `internal/client/` | ä¸»æŽ§ï¼šå‘½ä»¤è·¯ç”±ã€ä¼šè¯ç”Ÿå‘½å‘¨æœŸï¼ˆensureReady/promptStream/switchAgentï¼‰ã€terminal ç®¡ç†ã€å®žçŽ° acp.ClientCallbacks |
| `internal/acp/` | çº¯ä¼ è¾“å±‚ï¼šConnï¼ˆå­è¿›ç¨‹ stdioï¼‰ã€Forwarderï¼ˆç±»åž‹åŒ– ACP æ–¹æ³• + ClientCallbacks åˆ†å‘ï¼‰ã€åè®®ç±»åž‹ |
| `internal/agent/claude/` | å¯åŠ¨ claude-agent-acp å­è¿›ç¨‹ï¼Œè¿”å›ž `*acp.Conn`ï¼›NormalizeParams/HandlePermission é’©å­ |
| `internal/agent/codex/` | å¯åŠ¨ codex-acp å­è¿›ç¨‹ï¼Œè¿”å›ž `*acp.Conn`ï¼›åŒä¸Š |
| `internal/im/forwarder/` | IM åŒ…è£…å±‚ï¼šæ¶ˆæ¯åŽ»é‡/è¿‡æ»¤ã€å†³ç­–è¯·æ±‚ï¼ˆpending decisionsï¼‰ã€HelpResolver æ³¨å…¥ |
| `internal/im/console/` | Console IM é€‚é…å™¨ï¼šè¯» stdinï¼Œdebug æ¨¡å¼æ‰“å°æ‰€æœ‰ ACP JSON |
| `internal/im/feishu/` | é£žä¹¦ Bot IM é€‚é…å™¨ |
| `internal/im/mobile/` | WebSocket IM é€‚é…å™¨ï¼ˆä¾›ç§»åŠ¨ç«¯ App è¿žæŽ¥ï¼‰ |
| `internal/tools/` | å·¥å…·äºŒè¿›åˆ¶è·¯å¾„è§£æžï¼ˆ`bin/{GOOS}_{GOARCH}/`ï¼‰ |

## é…ç½®æ–‡ä»¶

- `~/.wheelmaker/config.json` â€” é¡¹ç›®é…ç½®ï¼ˆIM ç±»åž‹ã€agentã€å·¥ä½œç›®å½•ï¼‰
- `~/.wheelmaker/state.json` â€” è¿è¡Œæ—¶çŠ¶æ€æŒä¹…åŒ–ï¼ˆsession IDã€agent å…ƒæ•°æ®ã€session çŠ¶æ€ï¼‰

config.json æ ¼å¼ï¼š
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

## Mobile WebSocket åè®®

App é€šè¿‡ WebSocket è¿žæŽ¥åˆ° `ws://<host>:<port>/ws`ï¼Œæ¡æ‰‹æµç¨‹ï¼š

```
Server â†’ { "type": "auth_required" }          (è‹¥é…ç½®äº† token)
Client â†’ { "type": "auth", "token": "..." }
Server â†’ { "type": "ready", "chatId": "..." }
```

å…¥ç«™æ¶ˆæ¯ç±»åž‹ï¼š`auth` / `message` / `option` / `ping`  
å‡ºç«™æ¶ˆæ¯ç±»åž‹ï¼š`text` / `card` / `options` / `debug` / `pong` / `error` / `ready` / `auth_required`

å†³ç­–æµï¼š`options` æºå¸¦ `decisionId` â†’ App é€‰æ‹©åŽå‘ `{type:"option", decisionId, optionId}` â†’ Bridge è§£æž

## state.go è®¾è®¡

```
FileState
  â””â”€ Projects map[name]*ProjectState
       â”œâ”€ ActiveAgent string
       â”œâ”€ Connection *ConnectionConfig
       â””â”€ Agents map[name]*AgentState
            â”œâ”€ LastSessionID
            â”œâ”€ ProtocolVersion / AgentCapabilities / AgentInfo / AuthMethods
            â”œâ”€ Session *SessionState  (Modes/Models/ConfigOptions/AvailableCommands/Title/UpdatedAt)
            â””â”€ Sessions []SessionSummary  (æ‡’åŠ è½½)
```

## å¼€å‘çº¦å®š

- **æŽ¥å£ä¼˜å…ˆ**ï¼šè·¨å±‚ä¾èµ–é€šè¿‡æŽ¥å£ï¼ˆ`acp.Session`ã€`agent.Agent`ã€`im.Channel`ï¼‰
- **æ‡’åŠ è½½**ï¼šagent å­è¿›ç¨‹åœ¨é¦–æ¡æ¶ˆæ¯æ—¶æ‰åˆ›å»ºï¼ˆ`ensureForwarder`ï¼‰
- **æ”¯æŒçš„å‘½ä»¤**ï¼š`/use`ã€`/cancel`ã€`/status`ã€`/mode`ã€`/model`ã€`/list`ã€`/new`ã€`/load`ã€`/debug`ï¼›å…¶ä»– `/` å¼€å¤´æ–‡æœ¬å½“æ™®é€šæ¶ˆæ¯å¤„ç†
- ä»£ç æ³¨é‡Šå’Œæ ‡è¯†ç¬¦ç”¨è‹±æ–‡
- **æ¯æ¬¡æ”¹å®Œè‡ªåŠ¨ commit + push**

## æœ¬åœ°å¼€å‘

```bash
# åœ¨ server/ ç›®å½•ä¸‹æ‰§è¡Œ
export OPENAI_API_KEY=sk-...
go run ./cmd/wheelmaker/                                    # éœ€å…ˆåˆ›å»º ~/.wheelmaker/config.json
go test ./...
go build -o bin/windows_amd64/wheelmaker.exe ./cmd/wheelmaker/   # Windows
GOOS=linux  GOARCH=amd64 go build -o bin/linux_amd64/wheelmaker  ./cmd/wheelmaker/
GOOS=darwin GOARCH=arm64 go build -o bin/darwin_arm64/wheelmaker  ./cmd/wheelmaker/

# ACP å·¥å…·å®‰è£…ï¼ˆé¦–æ¬¡ï¼‰
pwsh scripts/install-tools.ps1   # Windows
bash scripts/install-tools.sh    # macOS / Linux
```

## Process Rule

Every code change must restart wheelmaker: stop existing `wheelmaker` processes, run `go run ./cmd/wheelmaker/`, and ensure only one process remains.
During live debugging, send the final user-facing reply first, then trigger delayed restart script `server/scripts/delayed-restart.ps1` (kill + `go run` after 30s) in background so the current chat is not interrupted. Calling that script counts as completion for this turn.
Windows command (returns immediately): `powershell -NoProfile -ExecutionPolicy Bypass -File server/scripts/delayed-restart.ps1`

## Key Invariants (do not break)
| # | Invariant |
|---|-----------|
| 1 | `acp.Conn` is pure transport â€” no business logic inside |
| 2 | `client.Client` is the single owner of session state; IM adapters never mutate it directly |
| 3 | Agent subprocess is created lazily â€” never at startup |
| 4 | All cross-layer deps injected via interfaces (`acp.Session`, `agent.Agent`, `im.Channel`) |
| 5 | `state.json` is the single source of truth for runtime state; never cache project state in memory only |
| 6 | Decision (option) messages carry a `decisionId`; mobile adapter resolves them via pending-decision map |

## å…³é”®åè®®æ–‡æ¡£

- ACP åè®®ï¼š[../docs/acp-protocol-full.zh-CN.md](../docs/acp-protocol-full.zh-CN.md)
- é£žä¹¦ Botï¼š[../docs/feishu-bot.md](../docs/feishu-bot.md)


