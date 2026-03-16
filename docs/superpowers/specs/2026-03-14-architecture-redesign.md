# WheelMaker æž¶æž„é‡è®¾è®¡è§„èŒƒ

> æ—¥æœŸï¼š2026-03-14
> çŠ¶æ€ï¼šå·²æ‰¹å‡†

## 1. èƒŒæ™¯ä¸Žé—®é¢˜

å½“å‰å®žçŽ°çš„å±‚æ¬¡åˆ’åˆ†å’Œå‘½åå­˜åœ¨ä»¥ä¸‹é—®é¢˜ï¼š

1. `agent.Agent` æ˜¯ interfaceï¼Œ`internal/agent/codex/provider.go` æ˜¯å…¶å®žçŽ°â€”â€”æ–‡ä»¶åå« adapterï¼Œæ¦‚å¿µåå« Agentï¼Œå±‚æ¬¡è¯­ä¹‰æ··ä¹±ã€‚
2. `hub` æ‰¿æ‹…äº†åè°ƒèŒè´£ï¼Œä½† "hub" ä¸æ˜¯ ACP åè®®æœ¯è¯­ï¼›æŒ‰ ACP è§„èŒƒï¼ŒWheelMaker æ•´ä½“æ˜¯ "Client"ã€‚
3. `acp.Client` æ˜¯ä½Žå±‚ JSON-RPC ä¼ è¾“ï¼Œä½†åå­— "Client" å’Œç³»ç»Ÿçº§çš„ "æˆ‘ä»¬æ˜¯ ACP Client" äº§ç”Ÿæ­§ä¹‰ã€‚
4. fs/terminal/permission å›žè°ƒæ··åœ¨ codex åŒ…é‡Œï¼Œåˆ‡æ¢ CLI åŽç«¯æ—¶é€»è¾‘æ— æ³•å¤ç”¨ã€‚
5. æ²¡æœ‰æ˜Žç¡®çš„ Adapter æŠ½è±¡ï¼Œæ— æ³•å¹²å‡€åœ°æ”¯æŒå¤š CLIï¼ˆcodex / claude / æœªæ¥å…¶ä»–ï¼‰ã€‚

## 2. è®¾è®¡ç›®æ ‡

- å‘½åä¸Ž ACP åè®®æ–‡æ¡£å¯¹é½ã€‚
- Agent æ˜¯ ACP åè®®çš„å…·ä½“å°è£…ï¼ŒåŒ…å«æ‰€æœ‰å‡ºç«™è°ƒç”¨å’Œå…¥ç«™å›žè°ƒå¤„ç†ã€‚
- Adapter æ˜¯çº¯ç²¹çš„è¿žæŽ¥ç®¡é“æŠ½è±¡ï¼Œåªè´Ÿè´£"å¯åŠ¨ binaryï¼Œè¿”å›žè¿žæŽ¥"ã€‚
- æ”¯æŒè¿è¡Œæ—¶åˆ‡æ¢ Adapterï¼ˆç”¨æˆ·é€‰æ‹©æ˜¯å¦å»¶ç»­ä¸Šä¸‹æ–‡ï¼‰ã€‚
- Permission ç­–ç•¥å¯æ³¨å…¥ï¼ŒMVP auto-allowï¼ŒPhase 2 è·¯ç”±åˆ° IMã€‚
- Client å±‚é€šè¿‡çª„æŽ¥å£ï¼ˆSessionï¼‰ä¾èµ– Agentï¼Œä¿æŒå¯æµ‹è¯•æ€§ã€‚

## 3. æ•°æ®æµ

```
IM (é£žä¹¦ç­‰)
    â†“ im.Message
client.Client          â† åè°ƒå±‚ï¼šè·¯ç”±å‘½ä»¤ã€ç®¡ç† adapter æ± ã€çŠ¶æ€æŒä¹…åŒ–
    â†“ Session interface
agent.Agent            â† ACP åè®®å°è£…ï¼šä¼šè¯ã€promptã€fs/terminal/permission å›žè°ƒ
    â†“ provider.Connect() â†’ *acp.Conn
provider.Provider        â† è¿žæŽ¥å·¥åŽ‚ï¼šå¯åŠ¨ binaryï¼Œè¿”å›ž Connï¼ˆå¯å¤šæ¬¡è°ƒç”¨ï¼‰
    â†“
agent/acp.Conn         â† ä½Žå±‚ä¼ è¾“ï¼šJSON-RPC 2.0 over stdioï¼Œæ‹¥æœ‰å­è¿›ç¨‹ç”Ÿå‘½å‘¨æœŸ
    â†“
CLI binary             â† codex-acp / claude-acp / ...
```

## 4. å‘½åå˜æ›´

| æ—§å | æ–°å | è¯´æ˜Ž |
|------|------|------|
| `internal/hub` | `internal/client` | WheelMaker æ˜¯ ACP Client |
| `hub.Hub` | `client.Client` | åŒä¸Š |
| `internal/acp/` | `internal/agent/acp/` | acp æ˜¯ agent å±‚å†…éƒ¨ä¼ è¾“ç»†èŠ‚ï¼Œç§»å…¥ agent å­åŒ… |
| `acp.Client` | `acp.Conn` | ä½Žå±‚ä¼ è¾“ï¼Œåå‰¯å…¶å®ž |
| `internal/agent/codex/` | `internal/provider/codex/` | Adapter å½’åˆ°é¡¶å±‚ adapter åŒ… |
| `agent.Agent`ï¼ˆinterfaceï¼‰ | `agent.Session`ï¼ˆçª„æŽ¥å£ï¼‰+ `agent.Agent`ï¼ˆconcrete structï¼‰ | Agent æ˜¯å…·ä½“æ¦‚å¿µï¼›Session æ˜¯ Client ç”¨çš„å¯æµ‹è¯•æŽ¥å£ |
| `hub.State.ACPSessionIDs` | `State.SessionIDs` | å­—æ®µæ”¹åï¼ŒJSON tag åŒæ­¥æ›´æ–°ï¼Œ**state.json éœ€è¿ç§»** |

## 5. åŒ…ç»“æž„

```
internal/
  agent/
    acp/               â† ä½Žå±‚ä¼ è¾“ï¼ˆä»Ž internal/acp/ ç§»å…¥ï¼Œæ˜¯ agent çš„å†…éƒ¨ç»†èŠ‚ï¼‰
      conn.go          â† Conn structï¼ˆåŽŸ client.go renameï¼‰
      conn_test.go     â† åŽŸ client_test.go rename
      types.go         â† ä¸å˜

    agent.go           â† Agent struct + Session interface + å¯¹å¤–æ–¹æ³•
    session.go         â† ACP ç”Ÿå‘½å‘¨æœŸï¼ˆinitialize / session/new / session/loadï¼‰
    prompt.go          â† session/prompt + session/update â†’ Update channel
    callbacks.go       â† fs/* / terminal/* / permission å›žè°ƒ
    terminal.go        â† terminalManager
    permission.go      â† PermissionHandler interface + AutoAllowHandler
    update.go          â† Update / UpdateType ç±»åž‹å®šä¹‰

  adapter/
    provider.go         â† Adapter interfaceï¼ˆè¿žæŽ¥å·¥åŽ‚ï¼ŒConnect è¿”å›ž *acp.Connï¼‰
    codex/
      provider.go       â† CodexAdapter implements Adapter

  client/
    client.go          â† Client structï¼ˆåŽŸ hub.goï¼‰ï¼Œä¾èµ– agent.Session æŽ¥å£
    store.go           â† ä¸å˜
    state.go           â† State å­—æ®µè°ƒæ•´ï¼ˆè§ç¬¬ 8 èŠ‚ï¼‰

  im/                  â† ä¸å˜
  tools/               â† ä¸å˜
```

## 6. æŽ¥å£å®šä¹‰

### 6.1 Adapterï¼ˆè¿žæŽ¥å·¥åŽ‚ï¼‰

```go
// adapter/provider.go
Package provider

import (
    "context"
    "github.com/swm8023/wheelmaker/internal/agent/acp"
)

// Adapter æŠ½è±¡ä¸€ä¸ª ACP å…¼å®¹çš„ CLI åŽç«¯ã€‚
// æ˜¯ä¸€ä¸ªæ— çŠ¶æ€çš„è¿žæŽ¥å·¥åŽ‚ï¼šConnect() æ¯æ¬¡è°ƒç”¨éƒ½å¯åŠ¨ä¸€ä¸ªæ–°çš„ binary å­è¿›ç¨‹å¹¶è¿”å›žå…¶ Connã€‚
// å­è¿›ç¨‹çš„ç”Ÿå‘½å‘¨æœŸç”±è¿”å›žçš„ *acp.Conn æŒæœ‰ï¼›provider.Close() ä»…ç”¨äºŽ Connect() å¤±è´¥æ—¶çš„æ¸…ç†ã€‚
// Connect() æˆåŠŸåŽè°ƒç”¨ provider.Close() æ˜¯ no-opï¼ˆå­è¿›ç¨‹å·²ç”± Conn ç®¡ç†ï¼‰ã€‚
// Connect() å†…éƒ¨è°ƒç”¨ Conn.Start() åŽè¿”å›žï¼Œå³è°ƒç”¨å®Œæˆæ—¶å­è¿›ç¨‹å·²è¿è¡Œï¼Œä¸éœ€è¦å†æ¬¡è°ƒç”¨ Start()ã€‚
type Adapter interface {
    Name() string
    Connect(ctx context.Context) (*acp.Conn, error)
    Close() error
}
```

### 6.2 Sessionï¼ˆClient ä½¿ç”¨çš„çª„æŽ¥å£ï¼Œä¿éšœå¯æµ‹è¯•æ€§ï¼‰

```go
// agent/agent.go
package agent

import "context"

// Session æ˜¯ client.Client ä¾èµ–çš„çª„æŽ¥å£ï¼Œä»…åŒ…å« Client éœ€è¦è°ƒç”¨çš„æ–¹æ³•ã€‚
// agent.Agent struct å®žçŽ°æ­¤æŽ¥å£ï¼›æµ‹è¯•ä¸­å¯æ³¨å…¥ mockã€‚
type Session interface {
    Prompt(ctx context.Context, text string) (<-chan Update, error)
    Cancel() error
    SetMode(ctx context.Context, modeID string) error
    AdapterName() string
    SessionID() string
    Close() error
}
```

### 6.3 PermissionHandler

```go
// agent/permission.go
package agent

// PermissionHandler å†³å®šå¦‚ä½•å“åº” CLI çš„æƒé™è¯·æ±‚ã€‚
// MVPï¼šAutoAllowHandler è‡ªåŠ¨é€‰æ‹© allow_onceã€‚
// Phase 2ï¼šIMPermissionHandler è·¯ç”±åˆ° IMï¼ˆéœ€è¦ Hub æä¾› chatID ç­‰ä¸Šä¸‹æ–‡ï¼Œ
//           å±Šæ—¶æŽ¥å£éœ€æ‰©å±•æˆ–é€šè¿‡é—­åŒ…æ³¨å…¥ä¸Šä¸‹æ–‡ï¼‰ã€‚
type PermissionHandler interface {
    RequestPermission(ctx context.Context,
        params acp.PermissionRequestParams) (acp.PermissionResult, error)
}

// AutoAllowHandlerï¼šæ— çŠ¶æ€ï¼Œè‡ªåŠ¨é€‰æ‹© allow_onceã€‚
type AutoAllowHandler struct{}
```

### 6.4 Update

```go
// agent/update.go
package agent

type UpdateType string

const (
    UpdateText       UpdateType = "text"        // agent_message_chunk
    UpdateThought    UpdateType = "thought"     // agent_thought_chunk
    UpdateToolCall   UpdateType = "tool_call"   // tool_call / tool_call_update
    UpdatePlan       UpdateType = "plan"        // plan
    UpdateModeChange UpdateType = "mode_change" // current_mode_update
    UpdateDone       UpdateType = "done"        // prompt ç»“æŸï¼ŒContent = stopReason
    UpdateError      UpdateType = "error"       // é”™è¯¯ï¼ŒErr != nil
)

// Update æ˜¯ Agent å‘ Client å‘é€çš„æµå¼æ›´æ–°å•å…ƒã€‚
// Raw ä»…åœ¨å·²çŸ¥çš„ç»“æž„åŒ–ç±»åž‹ï¼ˆtool_callã€planï¼‰ä¸­å¡«å……åŽŸå§‹ JSONï¼›
// å¯¹çº¯æ–‡æœ¬ç±»åž‹ï¼ˆtextã€thoughtï¼‰ï¼ŒRaw ä¸º nilã€‚
// å¯¹æœªçŸ¥ sessionUpdate ç±»åž‹ï¼ŒType ä½¿ç”¨åŽŸå§‹å­—ç¬¦ä¸²å€¼ï¼ŒRaw å¡«å……å®Œæ•´ params JSONã€‚
type Update struct {
    Type    UpdateType
    Content string // æ–‡æœ¬å†…å®¹ï¼ˆtext / thought / plan / stopReasonï¼‰
    Raw     []byte // ç»“æž„åŒ–å†…å®¹çš„åŽŸå§‹ JSONï¼ˆtool_call / plan / æœªçŸ¥ç±»åž‹ï¼‰
    Done    bool
    Err     error
}
```

### 6.5 Agentï¼ˆconcrete structï¼‰

```go
// agent/agent.go
package agent

// Agent æ˜¯ ACP åè®®çš„å®Œæ•´å°è£…ã€‚
// å®ƒæŒæœ‰ä¸€ä¸ªæ´»è·ƒçš„ *acp.Connï¼Œå¤„ç†å‡ºç«™ ACP è°ƒç”¨å’Œå…¥ç«™ CLI å›žè°ƒã€‚
// ä¸æŒæœ‰ Adapterï¼šAdapter ä»…åœ¨ Connect æ—¶ç”± Client è°ƒç”¨ï¼Œä¹‹åŽç”± Conn ç®¡ç†ç”Ÿå‘½å‘¨æœŸã€‚
type Agent struct {
    name       string                // å½“å‰ adapter åï¼ˆæ ‡è¯†ç”¨ï¼‰
    conn       *acp.Conn             // æ´»è·ƒçš„ ACP è¿žæŽ¥ï¼ˆæ‹¥æœ‰å­è¿›ç¨‹ï¼‰
    caps       acp.AgentCapabilities // initialize è¿”å›žçš„èƒ½åŠ›å£°æ˜Ž
    sessionID  string
    cwd        string
    mcpServers []acp.MCPServer

    permission PermissionHandler     // å¯æ³¨å…¥ï¼Œé»˜è®¤ AutoAllowHandler
    terminals  *terminalManager

    lastReply  string   // æœ€è¿‘ä¸€æ¬¡å®Œæ•´ agent å›žå¤ï¼Œç”¨äºŽ SwitchWithContext
    mu         sync.Mutex
    ready      bool
}

// New åˆ›å»º Agent å¹¶ç«‹å³æ³¨å†Œ Conn ä¸Šçš„å›žè°ƒå¤„ç†å™¨ã€‚
// è°ƒç”¨è€…ï¼ˆClientï¼‰è´Ÿè´£åœ¨ Switch æ—¶æä¾›æ–°çš„ Connã€‚
func New(name string, conn *acp.Conn, cwd string) *Agent

// --- Session interface å®žçŽ° ---
func (a *Agent) Prompt(ctx context.Context, text string) (<-chan Update, error)
func (a *Agent) Cancel() error
func (a *Agent) SetMode(ctx context.Context, modeID string) error
func (a *Agent) AdapterName() string
func (a *Agent) SessionID() string
func (a *Agent) Close() error

// --- æ‰©å±•æ–¹æ³•ï¼ˆClient ç›´æŽ¥è°ƒç”¨ï¼Œä¸é€šè¿‡ Session interfaceï¼‰---
func (a *Agent) SetConfigOption(ctx context.Context, configID, value string) error
func (a *Agent) Switch(ctx context.Context, name string, conn *acp.Conn, mode SwitchMode) error
func (a *Agent) SetPermissionHandler(h PermissionHandler)
```

### 6.6 SwitchMode

```go
// agent/agent.go
type SwitchMode int

const (
    // SwitchCleanï¼šä¸¢å¼ƒå½“å‰ sessionï¼Œæ–° Conn åœ¨ä¸‹æ¬¡ Prompt æ—¶æƒ°æ€§åˆå§‹åŒ–ã€‚
    SwitchClean SwitchMode = iota
    // SwitchWithContextï¼šå°†å½“å‰ lastReply ä½œä¸ºåˆå§‹ä¸Šä¸‹æ–‡ä¼ å…¥æ–° sessionã€‚
    // è‹¥ä¸Šä¸‹æ–‡ä¼ é€’å¤±è´¥ï¼ˆlastReply ä¸ºç©ºã€Prompt å‡ºé”™ï¼‰ï¼Œé™çº§ä¸º SwitchClean è¡Œä¸ºå¹¶è¿”å›žè­¦å‘Šã€‚
    SwitchWithContext
)
```

### 6.7 Clientï¼ˆåŽŸ Hubï¼‰

```go
// client/client.go
package client

// Client æ˜¯ WheelMaker çš„é¡¶å±‚åè°ƒå™¨ã€‚
// æŒæœ‰ Adapter æ± ï¼ˆæ— çŠ¶æ€å·¥åŽ‚ï¼‰ï¼›æŒæœ‰ä¸¤ä¸ªå¯¹ Agent çš„å¼•ç”¨ï¼š
//   - session agent.Sessionï¼šçª„æŽ¥å£ï¼Œç”¨äºŽ Prompt/Cancel/SetMode ç­‰æ—¥å¸¸æ“ä½œï¼Œå¯ mock æ›¿æ¢ã€‚
//   - agent  *agent.Agentï¼šå…·ä½“ç±»åž‹æŒ‡é’ˆï¼Œç”¨äºŽ Switch ç­‰æ‰©å±•æ–¹æ³•ï¼Œæµ‹è¯•æ—¶å¯ä¸º nilï¼ˆä¸è§¦å‘ Switchï¼‰ã€‚
// åˆ‡æ¢ Adapter æ—¶ï¼ŒClient è´Ÿè´£ï¼š
//   1. è°ƒç”¨ provider.Connect() èŽ·å–æ–° Conn
//   2. è°ƒç”¨ c.agent.Switch(newConn, ...) æ›¿æ¢è¿žæŽ¥ï¼ˆç›´æŽ¥ä½¿ç”¨å…·ä½“ç±»åž‹ï¼Œä¸ç»è¿‡çª„æŽ¥å£ï¼‰
//   3. å°†æ—§ Adapter çš„ Close() ç½®ä¸º no-opï¼ˆConn å·²ç®¡ç†å­è¿›ç¨‹ç”Ÿå‘½å‘¨æœŸï¼‰
type Client struct {
    adapters map[string]provider.Provider // "codex" â†’ CodexAdapterï¼ˆæ— çŠ¶æ€å·¥åŽ‚ï¼‰
    session  agent.Session              // å½“å‰æ´»è·ƒ sessionï¼ˆçª„æŽ¥å£ï¼Œå¯ mockï¼‰
    agent    *agent.Agent               // åŒä¸€ Agent çš„å…·ä½“ç±»åž‹æŒ‡é’ˆï¼Œç”¨äºŽ Switch ç­‰æ‰©å±•æ–¹æ³•
    store    Store
    state    *State
    im       im.Adapter                 // nil in CLI/test mode
}

func New(store Store, im im.Adapter) *Client
func (c *Client) RegisterProvider(a provider.Provider)
// Start åŠ è½½æŒä¹…åŒ–çŠ¶æ€ï¼Œåˆ›å»ºåˆå§‹ Agent å¹¶è°ƒç”¨ Connect() å®Œæˆè¿žæŽ¥ï¼ˆç«‹å³å¯åŠ¨å­è¿›ç¨‹ï¼Œéžæƒ°æ€§ï¼‰ã€‚
// æ­¤åŽé¦–æ¬¡æ”¶åˆ° Prompt æ—¶ç›´æŽ¥å¯ç”¨ï¼Œæ— é¡»ç­‰å¾…å­è¿›ç¨‹å†·å¯åŠ¨ã€‚
func (c *Client) Start(ctx context.Context) error
func (c *Client) Run(ctx context.Context) error  // é˜»å¡žï¼šé©±åŠ¨ IM äº‹ä»¶å¾ªçŽ¯æˆ– stdin å¾ªçŽ¯
func (c *Client) HandleMessage(msg im.Message)
func (c *Client) Close() error
```

### 6.8 acp.Connï¼ˆåŽŸ acp.Clientï¼‰

```go
// agent/acp/conn.go â€” ç§»å…¥ agent/acp/ï¼Œå¹¶é‡å‘½åï¼Œé€»è¾‘ä¸å˜
package acp

// Conn ç®¡ç†ä¸€ä¸ª ACP å…¼å®¹å­è¿›ç¨‹çš„å®Œæ•´ç”Ÿå‘½å‘¨æœŸï¼ˆstdio JSON-RPCï¼‰ã€‚
// Connect åŽç”± Conn ç‹¬å å­è¿›ç¨‹æ‰€æœ‰æƒï¼›Close() å…³é—­ stdinï¼Œç­‰å¾…è¿›ç¨‹é€€å‡ºã€‚
type Conn struct { /* åŽŸ Client å­—æ®µï¼Œä¸å˜ */ }

func New(exePath string, env []string) *Conn
func (c *Conn) Start() error
func (c *Conn) Send(ctx context.Context, method string, params any, result any) error
func (c *Conn) Notify(method string, params any) error
func (c *Conn) Subscribe(handler NotificationHandler) (cancel func())
func (c *Conn) OnRequest(handler RequestHandler)
func (c *Conn) Close() error
```

## 7. Agent å†…éƒ¨èŒè´£åˆ†åŒº

### 7.1 Session ç”Ÿå‘½å‘¨æœŸï¼ˆsession.goï¼‰

```
Agent.ensureReady(ctx):
  1. conn.Send("initialize", InitializeParams{...}) â†’ èŽ·å– caps
     InitializeParams å£°æ˜Ž fs.readTextFile=true, fs.writeTextFile=true, terminal=true
  2. è‹¥ caps.LoadSession && sessionID != "" â†’ conn.Send("session/load", ...)
  3. å¦åˆ™ â†’ conn.Send("session/new", {cwd, mcpServers}) â†’ å­˜ a.sessionID
  4. conn.OnRequest(a.handleCallback)  â† æ³¨å†Œæ‰€æœ‰å…¥ç«™å›žè°ƒ
```

### 7.2 Prompt æµï¼ˆprompt.goï¼‰

```
Agent.Prompt(ctx, text):
  1. ensureReady(ctx)
  2. cancel := conn.Subscribe(sessionUpdateHandler)  â† è¿‡æ»¤æœ¬ sessionID
  3. goroutine:
       conn.Send("session/prompt", {sessionID, text}, &result)
       â†’ æˆåŠŸï¼šå‘ Update{Type: UpdateDone, Content: result.StopReason, Done: true}
       â†’ å¤±è´¥ï¼šå‘ Update{Type: UpdateError, Err: err, Done: true}
       defer cancel(); defer close(updates)
  4. sessionUpdateHandler:
       å°† session/update å­ç±»åž‹è½¬ä¸º Updateï¼ˆè§ä¸‹è¡¨ï¼‰ï¼Œå†™å…¥ channel
       åŒæ—¶å°† agent_message_chunk çš„æ–‡æœ¬è¿½åŠ åˆ° a.lastReplyï¼ˆprompt ç»“æŸæ—¶å›ºåŒ–ï¼‰
```

`session/update` å­ç±»åž‹æ˜ å°„ï¼š

| `sessionUpdate` å€¼ | `UpdateType` | `Content` | `Raw` |
|---|---|---|---|
| `agent_message_chunk` | `text` | text | nil |
| `agent_thought_chunk` | `thought` | text | nil |
| `tool_call` / `tool_call_update` | `tool_call` | â€” | å®Œæ•´ update JSON |
| `plan` | `plan` | â€” | å®Œæ•´ update JSON |
| `current_mode_update` | `mode_change` | â€” | å®Œæ•´ update JSON |
| å…¶ä½™å·²çŸ¥ç±»åž‹ | åŽŸå­—ç¬¦ä¸² | â€” | å®Œæ•´ update JSON |

### 7.3 å…¥ç«™å›žè°ƒï¼ˆcallbacks.goï¼‰

`conn.OnRequest(a.handleCallback)` åœ¨ `ensureReady` å†…æ³¨å†Œã€‚
å›žè°ƒä½¿ç”¨ `context.Background()` çš„ TODOï¼šæœªæ¥å¯åœ¨ `Conn.OnRequest` ä¼ å…¥ session-scoped context ä»¥æ”¯æŒå–æ¶ˆã€‚

| ACP æ–¹æ³• | å¤„ç†é€»è¾‘ |
|----------|----------|
| `fs/read_text_file` | `os.ReadFile(params.Path)` |
| `fs/write_text_file` | `os.MkdirAll` + `os.WriteFile` |
| `terminal/create` | `terminalManager.Create(...)` â†’ è¿”å›ž `terminalId` |
| `terminal/output` | `terminalManager.Output(terminalID)` |
| `terminal/wait_for_exit` | `terminalManager.WaitForExit(terminalID)` |
| `terminal/kill` | `terminalManager.Kill(terminalID)` |
| `terminal/release` | `terminalManager.Release(terminalID)` |
| `session/request_permission` | `a.permission.RequestPermission(ctx, params)` |

### 7.4 Adapter åˆ‡æ¢ï¼ˆagent.goï¼‰

åˆ‡æ¢æµç¨‹ç”± **Client** åè°ƒï¼ŒAgent åªè´Ÿè´£è¿žæŽ¥æ›¿æ¢ï¼š

**å¹¶å‘å®‰å…¨çº¦å®š**ï¼š`Switch` è°ƒç”¨å‰è°ƒç”¨æ–¹ï¼ˆClientï¼‰å¿…é¡»å…ˆè°ƒç”¨ `Cancel()` å¹¶ç­‰å¾…å½“å‰ Prompt goroutine
ç»“æŸï¼ˆå³ç­‰å¾…ä¸Šæ¬¡ `Prompt` è¿”å›žçš„ channel å…³é—­ï¼‰ï¼Œå†è°ƒç”¨ `Switch`ã€‚
`Agent` è‡ªèº«ä¸åœ¨ `Switch` å†…éƒ¨ç­‰å¾… Prompt å®Œæˆâ€”â€”è¿™æ˜¯è°ƒç”¨æ–¹çš„èŒè´£ï¼Œå¯é¿å…æ­»é”ï¼ˆCancel éœ€è¦å‘é€
ç½‘ç»œæ¶ˆæ¯ï¼Œè‹¥ Switch æŒé”ç­‰å¾…åˆ™å¯èƒ½æ­»é”ï¼‰ã€‚

```
// Client ä¾§ï¼ˆclient.goï¼‰ï¼š
// ä½¿ç”¨ c.agentï¼ˆå…·ä½“ç±»åž‹ï¼‰è°ƒç”¨ Switchï¼Œä¸ç»è¿‡ c.sessionï¼ˆçª„æŽ¥å£ï¼‰ï¼Œ
// é¿å…å¯¹ agent.Session interface åšç±»åž‹æ–­è¨€ï¼ˆç±»åž‹æ–­è¨€åœ¨ mock æ³¨å…¥æ—¶ä¼š panicï¼‰ã€‚
func (c *Client) switchAdapter(ctx, name, mode):
    // 1. å…ˆå–æ¶ˆå¹¶æŽ’å¹²å½“å‰ promptï¼ˆè‹¥æœ‰ï¼‰
    c.session.Cancel()
    if c.currentPromptCh != nil:
        for range c.currentPromptCh {}   // ç­‰å¾… Prompt goroutine é€€å‡º
        c.currentPromptCh = nil
    // 2. å¯åŠ¨æ–° binary
    newAdapter = c.adapters[name]
    newConn, err = newAdapter.Connect(ctx)
    if err: reply error
    // 3. æ›¿æ¢è¿žæŽ¥
    c.agent.Switch(ctx, name, newConn, mode)
    // æ—§ Conn ç”± Agent.Switch å†…éƒ¨å…³é—­ï¼ˆCloseï¼‰
    // Adapter å¯¹è±¡ä¿ç•™åœ¨ c.adaptersï¼Œå¯å†æ¬¡ Connectï¼ˆå·¥åŽ‚è¯­ä¹‰ï¼‰

// Agent ä¾§ï¼ˆagent.goï¼‰ï¼š
// è°ƒç”¨æ–¹å·²ä¿è¯æ— å¹¶å‘ Promptï¼ŒSwitch åŠ  mu é”ä¿æŠ¤å­—æ®µå†™å…¥å³å¯ã€‚
func (a *Agent) Switch(ctx, name, newConn, mode):
    a.mu.Lock()
    if mode == SwitchWithContext && a.lastReply != "":
        summary = a.lastReply
    a.killAllTerminals()       // åœ¨é”å†…æ¸…ç† terminals
    oldConn = a.conn
    a.conn = newConn
    a.name = name
    a.ready = false
    a.sessionID = ""
    a.lastReply = ""
    a.mu.Unlock()
    oldConn.Close()            // é”å¤–å…³é—­æ—§ Connï¼ˆæ€æ­»æ—§å­è¿›ç¨‹ï¼‰ï¼Œé¿å… Close é˜»å¡žæŒé”
    if mode == SwitchWithContext && summary != "":
        ch, err = a.Prompt(ctx, "[context] "+summary)
        if err == nil:
            go func() { for range ch {} }()  // å¿…é¡»æ¶ˆè´¹ channelï¼Œå¦åˆ™ Prompt å†…éƒ¨ goroutine æ³„æ¼
        // Prompt å¤±è´¥ä¸å½±å“ switch æˆåŠŸï¼Œè®°å½• warning å³å¯
    return nil
```

> **æ³¨**ï¼š`Client` éœ€åœ¨è‡ªèº«ç»“æž„ä½“ä¸­ä¿å­˜ `currentPromptCh <-chan Update` å­—æ®µï¼Œ
> æ¯æ¬¡ `Prompt` è°ƒç”¨åŽæ›´æ–°ï¼Œç”¨äºŽ `switchAdapter` ä¸­çš„æŽ’å¹²æ“ä½œã€‚

## 8. çŠ¶æ€æŒä¹…åŒ–

```go
// client/state.go
type AdapterConfig struct {
    ExePath string            `json:"exePath"`
    Env     map[string]string `json:"env"`
}

// State å˜æ›´è¯´æ˜Žï¼ˆä¸¤å¤„ breaking changeï¼ŒLoad() å‡éœ€å…¼å®¹è¯»ï¼‰ï¼š
//
// 1. å­—æ®µ ACPSessionIDsï¼ˆjson:"acp_session_ids"ï¼‰â†’ SessionIDsï¼ˆjson:"session_ids"ï¼‰
//    è¿ç§»ï¼šLoad() è§£æžæ—¶è‹¥ "session_ids" ä¸ºç©ºè€Œ "acp_session_ids" ä¸ä¸ºç©ºï¼Œå¤åˆ¶æ—§å€¼ã€‚
//
// 2. å­—æ®µ ActiveAgentï¼ˆjson:"active_agent"ï¼‰â†’ ActiveAdapterï¼ˆjson:"activeAdapter"ï¼‰
//    è¿ç§»ï¼šLoad() è§£æžæ—¶è‹¥ "activeAdapter" ä¸ºç©ºè€Œ "active_agent" ä¸ä¸ºç©ºï¼Œå¤åˆ¶æ—§å€¼ã€‚
//    ï¼ˆä¸å¤åˆ¶ä¼šå¯¼è‡´å¯åŠ¨æ—¶ ActiveAdapter ä¸ºç©ºï¼Œsilently ä¸¢å¤±ç”¨æˆ·é€‰æ‹©çš„ adapterï¼‰
//
// å†™å…¥åªå†™æ–° keyï¼Œä¸å†å†™æ—§ keyã€‚
type State struct {
    ActiveAdapter string                   `json:"activeAdapter"`
    Adapters      map[string]AdapterConfig `json:"adapters"`
    SessionIDs    map[string]string        `json:"session_ids"` // adapterå â†’ ACP sessionId
}

// Load() è¿ç§»ä¼ªä»£ç ï¼š
//   raw := parseJSON(file)
//   if raw["activeAdapter"] == "" && raw["active_agent"] != "":
//       state.ActiveAdapter = raw["active_agent"]
//   if len(raw["session_ids"]) == 0 && len(raw["acp_session_ids"]) > 0:
//       state.SessionIDs = raw["acp_session_ids"]
```

## 9. CLI å‘½ä»¤å¤„ç†

Client.HandleMessage è§£æž `/` å‰ç¼€å‘½ä»¤ã€‚
`/use` çŽ°åœ¨ä¼šç«‹å³å¯åŠ¨æ–°å­è¿›ç¨‹ï¼ˆä¸å†æ˜¯æƒ°æ€§ï¼‰ï¼Œæ—§å­è¿›ç¨‹åŒæ­¥å…³é—­ã€‚

| å‘½ä»¤ | Client è¡Œä¸º |
|------|-------------|
| `/use <name>` | switchAdapter(name, SwitchClean) |
| `/use <name> --continue` | switchAdapter(name, SwitchWithContext) |
| `/cancel` | session.Cancel() |
| `/status` | session.AdapterName() + session.SessionID() |
| å…¶ä»–æ–‡æœ¬ | session.Prompt() â†’ æµå¼å›žå¤ |

> **è¡Œä¸ºå˜æ›´è¯´æ˜Ž**ï¼šåŽŸ `/use` ä»…æ›´æ–° state.ActiveAgentï¼Œä¸ç«‹å³å¯åŠ¨å­è¿›ç¨‹ï¼ˆæƒ°æ€§ï¼‰ã€‚
> æ–°è®¾è®¡ä¸­ `/use` ç«‹å³ Connectï¼Œç¡®ä¿åˆ‡æ¢åŽé¦–æ¡ Prompt å“åº”æ›´å¿«ï¼Œä¸”æ—§è¿›ç¨‹ç«‹å³é‡Šæ”¾èµ„æºã€‚

## 10. cmd/wheelmaker/main.go å˜æ›´

```go
func run() error {
    store := client.NewJSONStore(statePath)
    // æ³¨å†Œæ‰€æœ‰å¯ç”¨ adapter
    c := client.New(store, nil)
    c.RegisterProvider(codex.NewAdapter(codex.Config{...}))

    ctx, stop := signal.NotifyContext(...)
    defer stop()

    if err := c.Start(ctx); err != nil { return err }
    defer c.Close()

    return c.Run(ctx) // Run é˜»å¡žï¼šCLI æ¨¡å¼ä¸‹é©±åŠ¨ stdin å¾ªçŽ¯
}
```

`Client.Run(ctx)` åœ¨æ—  IM adapter æ—¶é©±åŠ¨ stdin è¯»å¾ªçŽ¯ï¼ˆåŽŸ main.go çš„ for å¾ªçŽ¯é€»è¾‘ç§»å…¥æ­¤å¤„ï¼‰ï¼›
æœ‰ IM adapter æ—¶è°ƒç”¨ `im.provider.Run(ctx)`ã€‚

## 11. æµ‹è¯•ç­–ç•¥

| å±‚ | æ–¹å¼ |
|----|------|
| `agent/acp.Conn` | mock agentï¼ˆæµ‹è¯•äºŒè¿›åˆ¶è‡ªèº«å……å½“å­è¿›ç¨‹ï¼‰ï¼Œå·²æœ‰ 16 ä¸ªå•å…ƒæµ‹è¯•ï¼ˆæ–‡ä»¶å rename å³å¯å¤ç”¨ï¼‰ |
| `provider/codex` | é›†æˆæµ‹è¯•ï¼Œ`//go:build integration`ï¼Œéœ€çœŸå®ž codex-acp binary |
| `agent.Agent` | æµ‹è¯•äºŒè¿›åˆ¶å……å½“ mock Conn å­è¿›ç¨‹ï¼Œæµ‹è¯• session lifecycle / prompt / switch / callbacks |
| `client.Client` | æ³¨å…¥ mock Sessionï¼ˆå®žçŽ° agent.Session interfaceï¼‰ï¼Œæµ‹è¯•å‘½ä»¤è§£æžå’Œè·¯ç”± |

## 12. å˜æ›´èŒƒå›´

**æ–°å»ºï¼š**
- `internal/provider/provider.go`ï¼ˆAdapter interfaceï¼‰
- `internal/provider/codex/provider.go`ï¼ˆCodexAdapterï¼ŒåŽŸ codex adapter + handlers åˆå¹¶ï¼Œæ—  ACP é€»è¾‘ï¼‰
- `internal/agent/acp/conn.go`ï¼ˆåŽŸ `internal/acp/client.go` ç§»å…¥å¹¶é‡å‘½åï¼‰
- `internal/agent/acp/conn_test.go`ï¼ˆåŽŸ `internal/acp/client_test.go` ç§»å…¥å¹¶é‡å‘½åï¼‰
- `internal/agent/acp/types.go`ï¼ˆåŽŸ `internal/acp/types.go` ç§»å…¥ï¼‰
- `internal/agent/session.go`ï¼ˆACP ç”Ÿå‘½å‘¨æœŸï¼‰
- `internal/agent/prompt.go`ï¼ˆprompt æµï¼‰
- `internal/agent/callbacks.go`ï¼ˆå…¥ç«™å›žè°ƒï¼‰
- `internal/agent/terminal.go`ï¼ˆterminalManagerï¼‰
- `internal/agent/permission.go`ï¼ˆPermissionHandler + AutoAllowHandlerï¼‰
- `internal/agent/update.go`ï¼ˆUpdate ç±»åž‹ï¼‰

**é‡å‘½å/é‡å†™ï¼š**
- `internal/hub/` â†’ `internal/client/`ï¼ˆHub â†’ Clientï¼Œæ–°å¢ž Session interface ä¾èµ–ï¼Œæ–°å¢ž Run()ï¼Œæ–°å¢ž currentPromptCh å­—æ®µï¼‰
- `internal/agent/agent.go`ï¼ˆåˆ é™¤ Agent interfaceï¼Œæ”¹ä¸º concrete struct + Session interfaceï¼‰

**åˆ é™¤ï¼š**
- `internal/acp/`ï¼ˆæ•´ä¸ªç›®å½•ï¼Œå†…å®¹å·²ç§»å…¥ `internal/agent/acp/`ï¼‰
- `internal/agent/codex/provider.go`
- `internal/agent/codex/handlers.go`

**ä¸å˜ï¼š**
- `internal/im/im.go`
- `internal/tools/resolve.go`
- `internal/client/store.go`ï¼ˆåŽŸ hub/store.goï¼Œä»…ç§»åŒ…ï¼‰



