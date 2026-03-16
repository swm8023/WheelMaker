# WheelMaker

æœ¬åœ° AI ç¼–ç¨‹ CLIï¼ˆCodexã€Claude ç­‰ï¼‰çš„è¿œç¨‹æŽ§åˆ¶æ¡¥æŽ¥å®ˆæŠ¤è¿›ç¨‹ï¼Œé€šè¿‡é£žä¹¦ç­‰ IM è®©å¼€å‘è€…åœ¨æ‰‹æœºä¸Šè¿œç¨‹æ“æŽ§æœ¬åœ° AI åŠ©æ‰‹ã€‚

## æž¶æž„å±‚æ¬¡

```
Hub (internal/hub/)          â€” å¤š project ç”Ÿå‘½å‘¨æœŸç®¡ç†ï¼Œè¯» config.json
  â””â”€ client.Client           â€” å• project åè°ƒï¼šå‘½ä»¤è·¯ç”±ã€agent æ‡’åŠ è½½ã€idle è¶…æ—¶ã€çŠ¶æ€æŒä¹…åŒ–
       â””â”€ agent.Agent        â€” ACP åè®®å°è£…ï¼šä¼šè¯ã€prompt æµã€fs/terminal/permission å›žè°ƒ
            â””â”€ provider.Provider â†’ acp.Conn  â€” è¿žæŽ¥å·¥åŽ‚ â†’ JSON-RPC 2.0 over stdio â†’ CLI binary
```

## åŒ…èŒè´£

| åŒ… | èŒè´£ |
|----|------|
| `internal/hub/` | è¯» `~/.wheelmaker/config.json`ï¼Œä¸ºæ¯ä¸ª project åˆ›å»º Client + IM |
| `internal/client/` | å• project åè°ƒï¼šå‘½ä»¤è·¯ç”±ã€lazy agent initã€idle 30min è¶…æ—¶ã€state æŒä¹…åŒ– |
| `internal/agent/` | ACP ä¼šè¯ç”Ÿå‘½å‘¨æœŸã€prompt æµã€å…¥ç«™å›žè°ƒï¼ˆfs/terminal/permissionï¼‰ |
| `internal/agent/provider/acp/` | JSON-RPC 2.0 ä¼ è¾“å±‚ï¼Œç®¡ç†å­è¿›ç¨‹ stdio |
| `internal/agent/provider/codex/` | å¯åŠ¨ codex-acp äºŒè¿›åˆ¶ï¼Œè¿”å›ž `*acp.Conn` |
| `internal/im/console/` | Console IMï¼šè¯» stdinï¼Œdebug æ¨¡å¼æ‰“å°æ‰€æœ‰ ACP JSON |
| `internal/im/feishu/` | é£žä¹¦ Bot IM adapter |
| `internal/tools/` | å·¥å…·äºŒè¿›åˆ¶è·¯å¾„è§£æžï¼ˆ`bin/{GOOS}_{GOARCH}/`ï¼‰ |

## é…ç½®æ–‡ä»¶

- `~/.wheelmaker/config.json` â€” é¡¹ç›®é…ç½®ï¼ˆIM ç±»åž‹ã€adapterã€å·¥ä½œç›®å½•ï¼‰
- `~/.wheelmaker/state.json` â€” è¿è¡Œæ—¶çŠ¶æ€æŒä¹…åŒ–ï¼ˆsession IDã€agent å…ƒæ•°æ®ã€session çŠ¶æ€ï¼‰

config.json æ ¼å¼ï¼š
```json
{
  "projects": [
    { "name": "local", "im": { "type": "console", "debug": true },
      "client": { "adapter": "codex", "path": "/your/project" } }
  ]
}
```

## state.go è®¾è®¡

`internal/client/state.go` å®šä¹‰åºåˆ—åŒ–ç»“æž„ï¼Œç”¨äºŽï¼š
1. è·¨è¿›ç¨‹æŒä¹…åŒ–è¿è¡Œæ—¶çŠ¶æ€ï¼ˆsessionIDã€agent å…ƒæ•°æ®ã€æœ€è¿‘ session çŠ¶æ€ï¼‰
2. å¯åŠ¨æ—¶æ¢å¤ä¸Šæ¬¡è¿žæŽ¥ï¼ˆsession/loadï¼‰

```
FileState
  â””â”€ Projects map[name]*ProjectState
       â”œâ”€ ActiveAdapter string            â€” å½“å‰æ¿€æ´»çš„ adapter åç§°
       â”œâ”€ Connection *ConnectionConfig    â€” æœ€è¿‘ä¸€æ¬¡ initialize æ—¶å‘é€çš„å®¢æˆ·ç«¯å‚æ•°
       â””â”€ Agents map[name]*AgentState     â€” æ¯ä¸ª adapter çš„æŒä¹…åŒ–å…ƒæ•°æ®
            â”œâ”€ LastSessionID              â€” ä¸‹æ¬¡å¯åŠ¨æ—¶ä¼ ç»™ session/load
            â”œâ”€ ProtocolVersion / AgentCapabilities / AgentInfo / AuthMethods
            â”‚                             â€” initialize å“åº”çš„ agent çº§åˆ«æ•°æ®
            â”œâ”€ Session *SessionState      â€” æœ€åŽä½¿ç”¨çš„ session çŠ¶æ€ï¼ˆéžå…¨é‡ï¼‰
            â”‚    â””â”€ Modes / Models / ConfigOptions / AvailableCommands / Title / UpdatedAt
            â””â”€ Sessions []SessionSummary  â€” å·²çŸ¥ session çš„è½»é‡åˆ—è¡¨ï¼ˆæŒ‰éœ€å¡«å……ï¼Œä¸è‡ªåŠ¨ç»´æŠ¤ï¼‰
```

**è®¾è®¡åŽŸåˆ™ï¼š**
- æ¯ä¸ª adapter åªå­˜æœ€åŽä¸€æ¬¡ä½¿ç”¨çš„ session çŠ¶æ€ï¼›åˆ‡æ¢ session æ—¶ä¼šåŒæ­¥æ›´æ–°
- `Sessions` åˆ—è¡¨æ˜¯æ‡’åŠ è½½çš„ï¼šä»…åœ¨ç”¨æˆ·æŸ¥è¯¢åŽ†å²æ—¶å†™å…¥ï¼Œä¸éšæ¯æ¬¡ prompt è‡ªåŠ¨æ›´æ–°
- `Connection` åœ¨ initialize æ¡æ‰‹å®ŒæˆåŽç”± `persistAgentMeta` å†™å…¥ï¼Œæ‰€æœ‰ adapter å…±ç”¨åŒä¸€ä»½å®¢æˆ·ç«¯å‚æ•°

## å¼€å‘çº¦å®š

- **æŽ¥å£ä¼˜å…ˆ**ï¼šè·¨å±‚ä¾èµ–é€šè¿‡æŽ¥å£ï¼ˆ`agent.Session`ã€`provider.Provider`ã€`im.Adapter`ï¼‰
- **æ‡’åŠ è½½**ï¼šagent å­è¿›ç¨‹åœ¨é¦–æ¡æ¶ˆæ¯æ—¶æ‰åˆ›å»ºï¼›idle 30min è‡ªåŠ¨å…³é—­å¹¶å­˜ç›˜ï¼Œä¸‹æ¬¡æ¢å¤
- **å‘½ä»¤åˆ¤æ–­**ï¼šä»… `/use`ã€`/cancel`ã€`/status` æ˜¯å‘½ä»¤ï¼Œå…¶ä»– `/` å¼€å¤´æ–‡æœ¬å½“æ™®é€šæ¶ˆæ¯å¤„ç†
- ä»£ç æ³¨é‡Šå’Œæ ‡è¯†ç¬¦ç”¨è‹±æ–‡
- **æ¯æ¬¡æ”¹å®Œè‡ªåŠ¨ commit + push**ï¼šæ¯å®Œæˆä¸€æ¬¡ä»£ç ä¿®æ”¹åŽï¼Œç«‹å³æ‰§è¡Œ `git add`ã€`git commit`ã€`git push`ï¼Œæ— éœ€ç­‰ç”¨æˆ·æç¤º

## æœ¬åœ°å¼€å‘

```bash
export OPENAI_API_KEY=sk-...
go run ./cmd/wheelmaker/   # éœ€å…ˆåˆ›å»º ~/.wheelmaker/config.json
go test ./...
go build ./cmd/wheelmaker/
```

## å…³é”®åè®®æ–‡æ¡£

- ACP åè®®ï¼š[docs/acp-protocol-full.zh-CN.md](docs/acp-protocol-full.zh-CN.md)
- é£žä¹¦ Botï¼š[docs/feishu-bot.md](docs/feishu-bot.md)




