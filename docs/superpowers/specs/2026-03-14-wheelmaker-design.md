# WheelMaker è®¾è®¡è§„èŒƒ

> ç‰ˆæœ¬ï¼šv0.1
> æ—¥æœŸï¼š2026-03-14
> çŠ¶æ€ï¼šå·²æ‰¹å‡†

## 1. é¡¹ç›®ç›®æ ‡

WheelMaker æ˜¯ä¸€ä¸ª Go ç¼–å†™çš„é•¿æœŸé©»ç•™å®ˆæŠ¤è¿›ç¨‹ï¼Œè®©å¼€å‘è€…é€šè¿‡æ‰‹æœº IMï¼ˆåˆæœŸä¸ºé£žä¹¦ï¼‰è¿œç¨‹æŽ§åˆ¶æœ¬åœ° AI ç¼–ç¨‹ CLIï¼ˆCodexã€Claudeã€Copilot ç­‰ï¼‰ã€‚

**æ ¸å¿ƒé—®é¢˜**ï¼šç¦»å¼€ç”µè„‘åŽæ— æ³•ä¸Žæœ¬åœ° AI ç¼–ç¨‹åŠ©æ‰‹äº¤äº’ã€‚
**è§£å†³æ–¹æ¡ˆ**ï¼šåœ¨æœ¬åœ°æœºå™¨è¿è¡Œä¸€ä¸ªæ¡¥æŽ¥å®ˆæŠ¤è¿›ç¨‹ï¼Œè¿žæŽ¥ IM å¹³å°ä¸Žæœ¬åœ° AI CLI å·¥å…·ã€‚

## 2. æ•´ä½“æž¶æž„

```
â”Œâ”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”    WebSocket      â”Œâ”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”
â”‚  é£žä¹¦ App    â”‚ â—„â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â–º â”‚          WheelMaker              â”‚
â”‚  (æ‰‹æœºç«¯)    â”‚   (go-lark SDK)   â”‚                                  â”‚
â””â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”˜                   â”‚  â”Œâ”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”    â”‚
                                  â”‚  â”‚         Hub              â”‚    â”‚
                                  â”‚  â”‚  (è°ƒåº¦ / çŠ¶æ€ / æŒä¹…åŒ–)  â”‚    â”‚
                                  â”‚  â””â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”¬â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”˜    â”‚
                                  â”‚               â”‚                  â”‚
                                  â”‚  â”Œâ”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â–¼â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”    â”‚
                                  â”‚  â”‚      Agent Interface      â”‚    â”‚
                                  â”‚  â”‚  â”Œâ”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â” â”‚    â”‚
                                  â”‚  â”‚  â”‚   Codex Adapter      â”‚ â”‚    â”‚
                                  â”‚  â”‚  â”‚  (ACP JSON-RPC)      â”‚ â”‚    â”‚
                                  â”‚  â”‚  â””â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”¬â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”˜ â”‚    â”‚
                                  â”‚  â””â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”¼â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”˜    â”‚
                                  â”‚                â”‚ stdin/stdout     â”‚
                                  â”‚  â”Œâ”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â–¼â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”     â”‚
                                  â”‚  â”‚     codex-acp.exe        â”‚     â”‚
                                  â”‚  â”‚  (å­è¿›ç¨‹ï¼ŒRust ç¼–å†™)     â”‚     â”‚
                                  â”‚  â””â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”˜     â”‚
                                  â””â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”˜
```

### æ•°æ®æµ

```
1. é£žä¹¦æ¶ˆæ¯ â†’ WebSocket â†’ im/feishu Adapter â†’ im.Message
2. Hub.HandleMessage(msg) â†’ è§£æžå‘½ä»¤ or è½¬å‘ Prompt
3. Agent.Prompt(ctx, text) â†’ acp.Client.Send(session/prompt)
4. codex-acp.exe â†’ session/update notifications (æµå¼)
5. Update stream â†’ Hub â†’ im.provider.SendText() â†’ é£žä¹¦
```

## 3. ç›®å½•ç»“æž„

```
wheelmaker/
â”œâ”€â”€ cmd/
â”‚   â””â”€â”€ wheelmaker/
â”‚       â””â”€â”€ main.go              # å¯åŠ¨å…¥å£
â”œâ”€â”€ internal/
â”‚   â”œâ”€â”€ acp/
â”‚   â”‚   â”œâ”€â”€ client.go            # JSON-RPC stdio ä¼ è¾“å±‚
â”‚   â”‚   â””â”€â”€ protocl.go             # æ¶ˆæ¯ç»“æž„ä½“
â”‚   â”œâ”€â”€ agent/
â”‚   â”‚   â”œâ”€â”€ agent.go             # Agent interface + Update ç±»åž‹
â”‚   â”‚   â””â”€â”€ codex/
â”‚   â”‚       â””â”€â”€ provider.go       # Codex ACP é€‚é…å™¨
â”‚   â”œâ”€â”€ im/
â”‚   â”‚   â”œâ”€â”€ im.go                # IM Adapter interface
â”‚   â”‚   â””â”€â”€ feishu/
â”‚   â”‚       â””â”€â”€ provider.go       # é£žä¹¦ WebSocket é€‚é…
â”‚   â”œâ”€â”€ hub/
â”‚   â”‚   â”œâ”€â”€ hub.go               # æ ¸å¿ƒè°ƒåº¦å™¨
â”‚   â”‚   â”œâ”€â”€ store.go             # Store interface + JSONStore
â”‚   â”‚   â””â”€â”€ state.go             # æŒä¹…åŒ–çŠ¶æ€ç»“æž„ä½“
â”‚   â””â”€â”€ tools/
â”‚       â””â”€â”€ resolve.go           # å·¥å…·äºŒè¿›åˆ¶è·¯å¾„è§£æž
â”œâ”€â”€ bin/
â”‚   â”œâ”€â”€ windows_amd64/.gitkeep
â”‚   â”œâ”€â”€ darwin_arm64/.gitkeep
â”‚   â”œâ”€â”€ darwin_amd64/.gitkeep
â”‚   â”œâ”€â”€ linux_amd64/.gitkeep
â”‚   â””â”€â”€ linux_arm64/.gitkeep
â”œâ”€â”€ scripts/
â”‚   â”œâ”€â”€ install-tools.sh         # Linux/macOS å®‰è£…è„šæœ¬
â”‚   â””â”€â”€ install-tools.ps1        # Windows å®‰è£…è„šæœ¬
â”œâ”€â”€ docs/
â”‚   â”œâ”€â”€ specs/                   # è®¾è®¡è§„èŒƒï¼ˆæœ¬æ–‡ä»¶ï¼‰
â”‚   â”œâ”€â”€ plan/                    # å®žçŽ°è®¡åˆ’
â”‚   â”œâ”€â”€ acp-protocol-full.zh-CN.md
â”‚   â”œâ”€â”€ feishu-bot.md
â”‚   â””â”€â”€ codex-acp.md
â”œâ”€â”€ CLAUDE.md
â”œâ”€â”€ AGENTS.md
â”œâ”€â”€ go.mod
â””â”€â”€ .gitignore
```

## 4. æ ¸å¿ƒæŽ¥å£å®šä¹‰

### 4.1 Agent æŽ¥å£

```go
// internal/agent/agent.go

// Update è¡¨ç¤º agent è¿”å›žçš„ä¸€ä¸ªæµå¼æ›´æ–°å•å…ƒ
type Update struct {
    Type    string // "text" | "tool_call" | "thought" | "error"
    Content string
    Done    bool
    Err     error
}

// Agent è¡¨ç¤ºä¸€ä¸ªå¯äº¤äº’çš„ AI ç¼–ç¨‹åŠ©æ‰‹
type Agent interface {
    Name() string
    // Prompt å‘é€ promptï¼Œè¿”å›žæµå¼ Update channelï¼Œè°ƒç”¨æ–¹è¯»å®Œ channel ç›´åˆ° Done=true
    Prompt(ctx context.Context, text string) (<-chan Update, error)
    Cancel() error
    SetMode(modeID string) error
    Close() error
}
```

### 4.2 IM Adapter æŽ¥å£

```go
// internal/im/im.go

// Message è¡¨ç¤ºæ¥è‡ª IM å¹³å°çš„ä¸€æ¡æ¶ˆæ¯
type Message struct {
    ChatID    string
    MessageID string
    UserID    string
    Text      string
}

// Card è¡¨ç¤ºå¯Œæ–‡æœ¬æ¶ˆæ¯å¡ç‰‡ï¼ˆé£žä¹¦äº¤äº’å¡ç‰‡æ ¼å¼ï¼‰
type Card map[string]any

// Adapter è¡¨ç¤ºä¸€ä¸ª IM å¹³å°é€‚é…å™¨
type Adapter interface {
    // OnMessage æ³¨å†Œæ¶ˆæ¯å¤„ç†å‡½æ•°
    OnMessage(handler func(Message))
    SendText(chatID, text string) error
    SendCard(chatID string, card Card) error
    SendReaction(messageID, emoji string) error
    // Run å¯åŠ¨äº‹ä»¶å¾ªçŽ¯ï¼ˆé˜»å¡žç›´åˆ° ctx å–æ¶ˆï¼‰
    Run(ctx context.Context) error
}
```

### 4.3 Hub çŠ¶æ€ä¸ŽæŒä¹…åŒ–

```go
// internal/hub/state.go

// AgentConfig å•ä¸ª agent çš„é…ç½®
type AgentConfig struct {
    ExePath string            // å·¥å…·äºŒè¿›åˆ¶è·¯å¾„ï¼ˆç©ºåˆ™ç”± tools.ResolveBinary è‡ªåŠ¨è§£æžï¼‰
    Env     map[string]string // é¢å¤–çŽ¯å¢ƒå˜é‡ï¼ˆå¦‚ OPENAI_API_KEYï¼‰
}

// State æŒä¹…åŒ–åˆ°ç£ç›˜çš„å…¨å±€çŠ¶æ€
type State struct {
    ActiveAgent   string                 // å½“å‰æ´»è·ƒ agent åç§°ï¼Œå¦‚ "codex"
    Agents        map[string]AgentConfig // agent å â†’ é…ç½®
    ACPSessionIDs map[string]string      // agent å â†’ ACP sessionIdï¼ˆç”¨äºŽ session/loadï¼‰
}

// internal/hub/store.go

// Store æŒä¹…åŒ–æŽ¥å£
type Store interface {
    Load() (*State, error)
    Save(s *State) error
}

// JSONStore å°† State å­˜å‚¨åˆ°æœ¬åœ° JSON æ–‡ä»¶
type JSONStore struct {
    Path string
}
```

### 4.4 å·¥å…·äºŒè¿›åˆ¶è§£æž

```go
// internal/tools/resolve.go

// ResolveBinary æŒ‰ä¼˜å…ˆçº§è§£æžå·¥å…·äºŒè¿›åˆ¶è·¯å¾„ï¼š
//   1. configPath éžç©ºæ—¶ç›´æŽ¥ä½¿ç”¨
//   2. æŸ¥æ‰¾ bin/{GOOS}_{GOARCH}/{name}[.exe]
//   3. æŸ¥æ‰¾ PATH
func ResolveBinary(name string, configPath string) (string, error)
```

## 5. ACP ä¼ è¾“å±‚è®¾è®¡

### 5.1 é€šä¿¡æ¨¡åž‹

`acp.Client` å¯åŠ¨ codex-acp å­è¿›ç¨‹ï¼Œé€šè¿‡ stdin/stdout è¿›è¡Œ JSON-RPC 2.0 é€šä¿¡ï¼š

- æ¯æ¡æ¶ˆæ¯æ˜¯ä¸€è¡Œ JSONï¼ˆ`\n` åˆ†éš”ï¼Œæ¶ˆæ¯å†…ä¸å«æ¢è¡Œï¼‰
- è¯·æ±‚å¸¦ `id`ï¼Œå“åº”åŒ¹é…å¯¹åº” `id`
- Notificationï¼ˆ`session/update`ï¼‰æ—  `id`ï¼Œç”±è®¢é˜…è€…å¤„ç†

### 5.2 ACPClient å†…éƒ¨ç»“æž„

```
ACPClient
â”œâ”€â”€ cmd *exec.Cmd              // å­è¿›ç¨‹
â”œâ”€â”€ encoder *json.Encoder      // å†™ stdin
â”œâ”€â”€ pending map[int64]chan Response  // ç­‰å¾…ä¸­çš„è¯·æ±‚
â”œâ”€â”€ mu sync.Mutex
â””â”€â”€ goroutine: readLoop()      // æŒç»­è¯» stdout
    â”œâ”€â”€ æœ‰ id â†’ åˆ†å‘åˆ° pending[id] channel
    â””â”€â”€ æ—  id â†’ å¹¿æ’­ç»™æ‰€æœ‰ subscriber
```

### 5.3 ç”Ÿå‘½å‘¨æœŸæ—¶åº

```
client.Start()
  â†’ exec.Command("codex-acp")
  â†’ readLoop goroutine

client.Send(initialize)
  â†’ {"id":1,"method":"initialize","params":{...}}
  â† {"id":1,"result":{"agentCapabilities":{...}}}

client.Send(session/new)  // æˆ– session/loadï¼ˆè‹¥æœ‰ sessionIdï¼‰
  â†’ {"id":2,"method":"session/new","params":{"cwd":"...","mcpServers":[]}}
  â† {"id":2,"result":{"sessionId":"abc123"}}

client.Send(session/prompt)  // å¼‚æ­¥é€šçŸ¥æµ
  â†’ {"id":3,"method":"session/prompt","params":{"sessionId":"abc123","prompt":"..."}}
  â† {"method":"session/update","params":{...}}  // 0 åˆ° N æ¡
  â† {"id":3,"result":{"stopReason":"end_turn"}}

client.Send(session/cancel)  // å¯é€‰ï¼Œå–æ¶ˆè¿›è¡Œä¸­çš„ prompt
client.Close()
```

## 6. Hub è®¾è®¡

### 6.1 èŒè´£

- **å¯åŠ¨æ—¶**ï¼šä»Ž Store åŠ è½½ Stateï¼Œæ ¹æ® `ActiveAgent` åˆå§‹åŒ–å¯¹åº” Agentï¼ˆæ‡’åŠ è½½ï¼‰
- **æ¶ˆæ¯è·¯ç”±**ï¼šè§£æžç‰¹æ®Šå‘½ä»¤ï¼ˆ`/use <agent>`ã€`/cancel`ã€`/status`ï¼‰ï¼Œå…¶ä½™è½¬å‘ç»™ Agent.Prompt()
- **æµå¼è½¬å‘**ï¼šå°† Agent Update stream å®žæ—¶æŽ¨é€åˆ° IMï¼ˆæ¯ä¸ª text chunk æ‹¼æŽ¥åŽç»Ÿä¸€å‘é€æˆ–åˆ†æ®µå‘é€ï¼‰
- **å…³é—­æ—¶**ï¼šä¿å­˜ ACPSessionID åˆ° Store ä¾›ä¸‹æ¬¡ session/load ä½¿ç”¨

### 6.2 ç‰¹æ®Šå‘½ä»¤

| å‘½ä»¤ | è¯´æ˜Ž |
|------|------|
| `/use <name>` | åˆ‡æ¢å½“å‰æ´»è·ƒ agentï¼ˆå¦‚ `/use codex`ã€`/use claude`ï¼‰ |
| `/cancel` | å–æ¶ˆå½“å‰ agent æ­£åœ¨å¤„ç†çš„è¯·æ±‚ |
| `/status` | è¿”å›žå½“å‰çŠ¶æ€ï¼ˆæ´»è·ƒ agentã€ACP session çŠ¶æ€ï¼‰ |

## 7. æŒä¹…åŒ–

- å­˜å‚¨ä½ç½®ï¼š`~/.wheelmaker/state.json`ï¼ˆæˆ–é€šè¿‡ `--state` å‚æ•°æŒ‡å®šï¼‰
- æ ¼å¼ï¼šJSONï¼ˆäººç±»å¯è¯»ï¼Œä¾¿äºŽè°ƒè¯•ï¼‰
- å†™å…¥æ—¶æœºï¼šsession/new æˆåŠŸåŽï¼ˆä¿å­˜ sessionIdï¼‰ã€agent åˆ‡æ¢æ—¶ã€è¿›ç¨‹é€€å‡ºæ—¶

## 8. å¤šå¹³å°å·¥å…·ç®¡ç†

```
bin/
  {GOOS}_{GOARCH}/
    codex-acp[.exe]
    # åŽç»­ï¼šclaude[.exe]ã€copilot[.exe]
```

- `.gitignore` å¿½ç•¥å®žé™…äºŒè¿›åˆ¶ï¼Œä¿ç•™ `.gitkeep`
- `scripts/install-tools.sh`ï¼šè‡ªåŠ¨ä¸‹è½½å¯¹åº”å¹³å°çš„ codex-acp åˆ° `bin/{platform}/`
- `scripts/install-tools.ps1`ï¼šWindows ç‰ˆæœ¬

## 9. ç¬¬äºŒé˜¶æ®µï¼ˆFeishu æŽ¥å…¥ï¼‰

é£žä¹¦é€‚é…å™¨å°†åœ¨ç¬¬äºŒé˜¶æ®µå®žçŽ°ï¼Œä½¿ç”¨ go-lark SDK çš„ WebSocket é•¿è¿žæŽ¥æ¨¡å¼ï¼š

- æ— éœ€å…¬ç½‘ IP
- SDK ä¸»åŠ¨è¿žæŽ¥é£žä¹¦ WebSocket ç½‘å…³
- äº‹ä»¶é€šè¿‡ `EventTypeMessageReceived` å›žè°ƒæŽ¥æ”¶
- å‘æ¶ˆæ¯é€šè¿‡ `bot.PostText()`ã€`bot.PostCard()` ç­‰æ–¹æ³•

## 10. é”™è¯¯å¤„ç†åŽŸåˆ™

- ä½¿ç”¨ `fmt.Errorf("...: %w", err)` åŒ…è£…é”™è¯¯ï¼Œä¿ç•™è°ƒç”¨é“¾
- ä½¿ç”¨ `errors.Is` / `errors.As` åˆ¤æ–­é”™è¯¯ç±»åž‹
- ä¸ä½¿ç”¨ `panic`ï¼ˆé™¤éžæ˜¯çœŸæ­£çš„ç¨‹åºå‘˜é”™è¯¯ï¼‰
- ACP é€šä¿¡é”™è¯¯ï¼šè®°å½•æ—¥å¿—ï¼Œå‘ IM è¿”å›žé”™è¯¯æ¶ˆæ¯ï¼Œä¸å´©æºƒ


