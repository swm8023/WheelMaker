# WheelMaker MVP å®žçŽ°è®¡åˆ’

> ç‰ˆæœ¬ï¼šv0.1
> æ—¥æœŸï¼š2026-03-14
> é˜¶æ®µï¼šPhase 1 â€” é¡¹ç›®åŸºç¡€ + ACP å®¢æˆ·ç«¯

## MVP ç›®æ ‡

1. å»ºç«‹å®Œæ•´é¡¹ç›®éª¨æž¶ï¼ˆç›®å½•ã€go.modã€CLAUDE.mdã€AGENTS.mdï¼‰
2. å®žçŽ° ACP å®¢æˆ·ç«¯ï¼ˆGo è°ƒç”¨ codex-acp.exe via stdio JSON-RPCï¼‰
3. æ•´ç†åè®®æ–‡æ¡£åˆ° docs/ï¼ˆACP + Feishu Bot + codex-acpï¼‰

**ä¸åœ¨ MVP å†…**ï¼šFeishu WebSocket æŽ¥å…¥ï¼ˆPhase 2ï¼‰

## å®žçŽ°æ­¥éª¤

### Step 1ï¼šé¡¹ç›®è„šæ‰‹æž¶

**ç›®æ ‡æ–‡ä»¶**ï¼š
- `go.mod`
- `.gitignore`
- `CLAUDE.md`
- `AGENTS.md`
- å…¨éƒ¨ç›®å½•åŠ `.gitkeep` æ–‡ä»¶

**ä»»åŠ¡**ï¼š

1. åˆå§‹åŒ– Go moduleï¼š
   ```bash
   cd d:/Code/WheelMaker
   go mod init github.com/swm8023/wheelmaker
   ```

2. åˆ›å»º `.gitignore`ï¼š
   ```gitignore
   # å·¥å…·äºŒè¿›åˆ¶ï¼ˆé€šè¿‡ scripts/install-tools.sh ä¸‹è½½ï¼‰
   bin/**/*
   !bin/**/.gitkeep

   # çŠ¶æ€æ–‡ä»¶
   .wheelmaker/

   # Go
   *.exe
   *.test
   /vendor/
   ```

3. åˆ›å»ºç›®å½•éª¨æž¶ï¼ˆå« `.gitkeep`ï¼‰ï¼š
   ```
   cmd/wheelmaker/
   internal/acp/
   internal/agent/codex/
   internal/im/feishu/
   internal/hub/
   internal/tools/
   bin/windows_amd64/
   bin/darwin_arm64/
   bin/darwin_amd64/
   bin/linux_amd64/
   bin/linux_arm64/
   scripts/
   ```

4. åˆ›å»º `CLAUDE.md` å’Œ `AGENTS.md`ï¼ˆè§ä¸‹æ–¹å†…å®¹è§„åˆ’ï¼‰

5. åˆ›å»º `scripts/install-tools.sh` å’Œ `scripts/install-tools.ps1`

### Step 2ï¼šç±»åž‹ä¸ŽæŽ¥å£å®šä¹‰

**ç›®æ ‡æ–‡ä»¶**ï¼š
- `internal/agent/agent.go`
- `internal/im/im.go`
- `internal/hub/state.go`
- `internal/hub/store.go`

çº¯æŽ¥å£å’Œç»“æž„ä½“å®šä¹‰ï¼Œæ— å…·ä½“å®žçŽ°é€»è¾‘ã€‚è¯¦è§è®¾è®¡è§„èŒƒ Â§4ã€‚

### Step 3ï¼šACP ä¼ è¾“å±‚

**ç›®æ ‡æ–‡ä»¶**ï¼š
- `internal/acp/types.go` â€” JSON-RPC ç»“æž„ä½“
- `internal/acp/client.go` â€” ACPClient å®žçŽ°

**å…³é”®å®žçŽ°ç‚¹**ï¼š

```go
// types.go
type Request struct {
    JSONRPC string `json:"jsonrpc"`
    ID      int64  `json:"id,omitempty"`
    Method  string `json:"method"`
    Params  any    `json:"params,omitempty"`
}

type Response struct {
    JSONRPC string          `json:"jsonrpc"`
    ID      int64           `json:"id"`
    Result  json.RawMessage `json:"result,omitempty"`
    Error   *RPCError       `json:"error,omitempty"`
}

type Notification struct {
    JSONRPC string `json:"jsonrpc"`
    Method  string `json:"method"`
    Params  any    `json:"params,omitempty"`
}

// client.go æ ¸å¿ƒæ–¹æ³•
type Client struct { ... }

func New(exePath string, env []string) *Client
func (c *Client) Start() error
func (c *Client) Send(ctx context.Context, method string, params any, result any) error
func (c *Client) Subscribe(handler func(Notification)) (cancel func())
func (c *Client) Close() error
```

**readLoop å®žçŽ°è¦ç‚¹**ï¼š
- `bufio.Scanner` é€è¡Œè¯» stdout
- æŒ‰æ˜¯å¦æœ‰ `id` å­—æ®µåŒºåˆ† Response vs Notification
- Responseï¼šå†™å…¥ `pending[id]` channel
- Notificationï¼šå¹¿æ’­ç»™æ‰€æœ‰ subscriberï¼ˆç”¨ goroutine è°ƒç”¨ï¼Œé¿å…é˜»å¡žï¼‰

### Step 4ï¼šCodex Agent é€‚é…å™¨

**ç›®æ ‡æ–‡ä»¶**ï¼š
- `internal/agent/codex/provider.go`

```go
type CodexAgent struct {
    name   string
    client *acp.Client
    sessID string  // ACP sessionIdï¼Œç©ºè¡¨ç¤ºæœªåˆå§‹åŒ–
    mu     sync.Mutex
}

func New(cfg hub.AgentConfig) *CodexAgent

// æ‡’åˆå§‹åŒ–ï¼šé¦–æ¬¡è°ƒç”¨æ—¶ Start client â†’ initialize â†’ session/new or session/load
func (a *CodexAgent) ensureSession(ctx context.Context) error

func (a *CodexAgent) Prompt(ctx context.Context, text string) (<-chan agent.Update, error)
```

**Prompt å®žçŽ°è¦ç‚¹**ï¼š
1. `ensureSession(ctx)` ç¡®ä¿ ACP è¿žæŽ¥å’Œ session å°±ç»ª
2. è®¢é˜… Notificationï¼ˆ`Subscribe`ï¼‰
3. å‘é€ `session/prompt`ï¼ˆå¼‚æ­¥ï¼Œä¸ç­‰ resultï¼‰
4. å°† `session/update` notification è½¬æ¢ä¸º `agent.Update` å¹¶å†™å…¥ channel
5. æ”¶åˆ° `session/prompt` result åŽï¼Œå†™å…¥ `Update{Done:true}` å…³é—­ channel
6. å–æ¶ˆè®¢é˜…

### Step 5ï¼šHub

**ç›®æ ‡æ–‡ä»¶**ï¼š
- `internal/hub/hub.go`

```go
type Hub struct {
    store  Store
    state  *State
    agents map[string]agent.Agent  // å·²åˆå§‹åŒ–çš„ agent å®žä¾‹
    im     im.Adapter              // å¯ä¸º nilï¼ˆMVP é˜¶æ®µï¼‰
    mu     sync.Mutex
}

func New(store Store, im im.Adapter) *Hub
func (h *Hub) Start(ctx context.Context) error
func (h *Hub) HandleMessage(msg im.Message)
func (h *Hub) Close() error
```

**å‘½ä»¤è§£æž**ï¼š
- `/use <agent>` â†’ åˆ‡æ¢ `state.ActiveAgent`ï¼Œä¿å­˜ state
- `/cancel` â†’ è°ƒç”¨å½“å‰ agent.Cancel()
- `/status` â†’ è¿”å›žçŠ¶æ€å­—ç¬¦ä¸²
- å…¶ä»– â†’ è½¬å‘ç»™å½“å‰ agent.Prompt()

### Step 6ï¼šå·¥å…·è·¯å¾„è§£æž

**ç›®æ ‡æ–‡ä»¶**ï¼š
- `internal/tools/resolve.go`

```go
func ResolveBinary(name string, configPath string) (string, error) {
    // 1. ä½¿ç”¨é…ç½®è·¯å¾„
    if configPath != "" {
        if _, err := os.Stat(configPath); err == nil {
            return configPath, nil
        }
    }
    // 2. æŸ¥æ‰¾ bin/{GOOS}_{GOARCH}/
    exe := name
    if runtime.GOOS == "windows" {
        exe += ".exe"
    }
    binPath := filepath.Join("bin", runtime.GOOS+"_"+runtime.GOARCH, exe)
    if _, err := os.Stat(binPath); err == nil {
        return filepath.Abs(binPath)
    }
    // 3. æŸ¥æ‰¾ PATH
    return exec.LookPath(name)
}
```

### Step 7ï¼šå…¥å£

**ç›®æ ‡æ–‡ä»¶**ï¼š
- `cmd/wheelmaker/main.go`

MVP é˜¶æ®µæä¾›ç®€å•çš„ stdin æµ‹è¯•æ¨¡å¼ï¼š

```go
func main() {
    store := hub.NewJSONStore(".wheelmaker/state.json")
    h := hub.New(store, nil)  // æš‚æ—  IM
    ctx := context.Background()
    h.Start(ctx)

    // ä»Ž stdin è¯»å–æµ‹è¯•æ¶ˆæ¯
    scanner := bufio.NewScanner(os.Stdin)
    for scanner.Scan() {
        h.HandleMessage(im.Message{
            ChatID: "cli",
            Text:   scanner.Text(),
        })
    }
    h.Close()
}
```

### Step 8ï¼šå®‰è£…è„šæœ¬

**`scripts/install-tools.sh`**ï¼š
```bash
#!/usr/bin/env bash
# ä¸‹è½½ codex-acp åˆ° bin/{platform}/
GOOS=$(go env GOOS)
GOARCH=$(go env GOARCH)
DEST="bin/${GOOS}_${GOARCH}"
mkdir -p "$DEST"
# é€šè¿‡ npx èŽ·å–åŽå¤åˆ¶ï¼Œæˆ–ç›´æŽ¥ä»Ž GitHub releases ä¸‹è½½
npx --yes @zed-industries/codex-acp --version  # è§¦å‘å®‰è£…
NPXBIN=$(npx --yes which codex-acp 2>/dev/null || true)
if [ -n "$NPXBIN" ]; then
    cp "$NPXBIN" "$DEST/codex-acp"
    chmod +x "$DEST/codex-acp"
fi
```

**`scripts/install-tools.ps1`**ï¼š
```powershell
$dest = "bin\windows_amd64"
New-Item -ItemType Directory -Force -Path $dest
# é€šè¿‡ npx å®‰è£…åŽèŽ·å–è·¯å¾„
npx --yes @zed-industries/codex-acp --version
$npxBin = (npx --yes which codex-acp 2>$null)
if ($npxBin) {
    Copy-Item $npxBin "$dest\codex-acp.exe"
}
```

### Step 9ï¼šåè®®æ–‡æ¡£æ•´ç†

- `docs/feishu-bot.md`ï¼šé£žä¹¦ Bot åè®®æ‘˜è¦
- `docs/codex-acp.md`ï¼šcodex-acp ä½¿ç”¨æ‘˜è¦

## æ–‡æ¡£å†…å®¹è§„åˆ’

### CLAUDE.md

```markdown
# WheelMaker

## é¡¹ç›®ç›®æ ‡
æœ¬åœ° AI ç¼–ç¨‹ CLIï¼ˆCodex ç­‰ï¼‰çš„è¿œç¨‹æŽ§åˆ¶æ¡¥æŽ¥å™¨ï¼Œé€šè¿‡é£žä¹¦ç­‰ IM è¿œç¨‹æ“ä½œã€‚

## æž¶æž„
- cmd/wheelmaker/  â€” å…¥å£
- internal/acp/    â€” ACP JSON-RPC stdio ä¼ è¾“
- internal/agent/  â€” Agent æŽ¥å£ + å„ CLI é€‚é…å™¨
- internal/im/     â€” IM æŽ¥å£ + é£žä¹¦é€‚é…å™¨
- internal/hub/    â€” æ ¸å¿ƒè°ƒåº¦ï¼Œç®¡ç† agent åˆ‡æ¢å’ŒæŒä¹…åŒ–
- internal/tools/  â€” å·¥å…·äºŒè¿›åˆ¶è·¯å¾„è§£æž
- bin/{platform}/  â€” ç¬¬ä¸‰æ–¹å·¥å…·äºŒè¿›åˆ¶ï¼ˆ.gitignoredï¼‰
- scripts/         â€” å®‰è£…è„šæœ¬

## å¼€å‘çº¦å®š
- æŽ¥å£ä¼˜å…ˆï¼šæ‰€æœ‰è·¨å±‚ä¾èµ–é€šè¿‡æŽ¥å£
- ä¸è¿‡åº¦æŠ½è±¡ï¼šå…ˆè®©å®ƒå·¥ä½œï¼Œå†è€ƒè™‘æ‰©å±•
- ACP åè®®å‚è€ƒï¼šdocs/acp-protocol-full.zh-CN.md

## æœ¬åœ°æµ‹è¯•
go run ./cmd/wheelmaker/  # ä»Ž stdin è¾“å…¥æµ‹è¯•æ¶ˆæ¯
go test ./internal/acp/...
```

### AGENTS.md

```markdown
# AI Agent å¼€å‘è§„èŒƒ

## ä»£ç é£Žæ ¼
- ä½¿ç”¨ gofmt / goimports
- å‡½æ•°é•¿åº¦ä¸è¶…è¿‡ 50 è¡Œï¼Œè¶…å‡ºåˆ™æ‹†åˆ†
- ä¸ä½¿ç”¨ init()

## åŒ…èŒè´£
- æ¯ä¸ªåŒ…åªåšä¸€ä»¶äº‹
- ä¸åœ¨ im/ å±‚å¤„ç† agent é€»è¾‘
- ä¸åœ¨ agent/ å±‚å¤„ç† IM æ ¼å¼

## é”™è¯¯å¤„ç†
- ä½¿ç”¨ fmt.Errorf("context: %w", err) åŒ…è£…
- ä¸ä½¿ç”¨ panicï¼ˆé™¤éžæ˜¯ä¸å¯æ¢å¤çš„ç¨‹åºå‘˜é”™è¯¯ï¼‰
- å‘ä¸Šå±‚æš´éœ²æœ‰æ„ä¹‰çš„é”™è¯¯ä¿¡æ¯

## ç¦æ­¢äº‹é¡¹
- ä¸ç¡¬ç¼–ç  API key æˆ–è·¯å¾„
- ä¸åœ¨ä»£ç ä¸­å­˜å‚¨å‡­è¯
- ä¸ç»•è¿‡ Agent/IM æŽ¥å£ç›´æŽ¥è®¿é—®å®žçŽ°ç»†èŠ‚
```

## éªŒè¯è®¡åˆ’

### ACP è¿žæŽ¥éªŒè¯

```bash
# 1. å®‰è£… codex-acp
npm install -g @zed-industries/codex-acp
# æˆ–é€šè¿‡è„šæœ¬
./scripts/install-tools.sh

# 2. è¿è¡Œå•å…ƒæµ‹è¯•ï¼ˆéœ€è¦ OPENAI_API_KEYï¼‰
export OPENAI_API_KEY=sk-...
go test ./internal/acp/... -v
go test ./internal/agent/codex/... -v -run TestPrompt

# 3. ç«¯åˆ°ç«¯æµ‹è¯•ï¼šè¿è¡Œ mainï¼Œé€šè¿‡ stdin è¾“å…¥
go run ./cmd/wheelmaker/
# è¾“å…¥: /status
# è¾“å…¥: è§£é‡Šä¸€ä¸‹ Go çš„ goroutine
```

### éªŒæ”¶æ ‡å‡†

- [ ] `go build ./...` æ— é”™è¯¯
- [ ] `go vet ./...` æ— è­¦å‘Š
- [ ] ACP client èƒ½æˆåŠŸ spawn codex-acpï¼Œå®Œæˆ initialize + session/new
- [ ] Prompt å‘é€åŽèƒ½æ”¶åˆ°æµå¼æ–‡æœ¬æ›´æ–°
- [ ] `/use` å‘½ä»¤èƒ½åˆ‡æ¢ agentï¼ŒçŠ¶æ€æŒä¹…åŒ–åˆ°æ–‡ä»¶
- [ ] è¿›ç¨‹é‡å¯åŽèƒ½é€šè¿‡ session/load æ¢å¤ session

## ä¾èµ–

```
go.mod é¢„æœŸä¾èµ–ï¼š
ï¼ˆæš‚æ— ç¬¬ä¸‰æ–¹ Go ä¾èµ–ï¼Œä»…æ ‡å‡†åº“ï¼‰

Phase 2 æ·»åŠ ï¼š
github.com/go-lark/lark/v2  # é£žä¹¦ SDK
```

## Phase 2 é¢„è§ˆï¼ˆé£žä¹¦æŽ¥å…¥ï¼‰

Phase 2 å®žçŽ° `internal/im/feishu/provider.go`ï¼š

```go
import "github.com/go-lark/lark/v2"

type FeishuAdapter struct {
    bot     *lark.Bot
    handler func(im.Message)
}

func New(appID, appSecret string) *FeishuAdapter {
    bot := lark.NewChatBot(appID, appSecret)
    return &FeishuAdapter{bot: bot}
}

func (a *FeishuAdapter) Run(ctx context.Context) error {
    // ä½¿ç”¨ WebSocket é•¿è¿žæŽ¥æ¨¡å¼ï¼ˆæ— éœ€å…¬ç½‘ IPï¼‰
    // æ³¨å†Œ EventTypeMessageReceived äº‹ä»¶å¤„ç†
    ...
}
```

