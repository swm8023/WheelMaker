# WheelMaker 架构重设计规范

> 日期：2026-03-14
> 状态：已批准

## 1. 背景与问题

当前实现的层次划分和命名存在以下问题：

1. `agent.Agent` 是 interface，`internal/agent/codex/provider.go` 是其实现——文件名叫 adapter，概念名叫 Agent，层次语义混乱。
2. `hub` 承担了协调职责，但 "hub" 不是 ACP 协议术语；按 ACP 规范，WheelMaker 整体是 "Client"。
3. `acp.Client` 是低层 JSON-RPC 传输，但名字 "Client" 和系统级的 "我们是 ACP Client" 产生歧义。
4. fs/terminal/permission 回调混在 codex 包里，切换 CLI 后端时逻辑无法复用。
5. 没有明确的 Adapter 抽象，无法干净地支持多 CLI（codex / claude / 未来其他）。

## 2. 设计目标

- 命名与 ACP 协议文档对齐。
- Agent 是 ACP 协议的具体封装，包含所有出站调用和入站回调处理。
- Adapter 是纯粹的连接管道抽象，只负责"启动 binary，返回连接"。
- 支持运行时切换 Adapter（用户选择是否延续上下文）。
- Permission 策略可注入，MVP auto-allow，Phase 2 路由到 IM。
- Client 层通过窄接口（Session）依赖 Agent，保持可测试性。

## 3. 数据流

```
IM (飞书等)
    ↓ im.Message
client.Client          ← 协调层：路由命令、管理 adapter 池、状态持久化
    ↓ Session interface
agent.Agent            ← ACP 协议封装：会话、prompt、fs/terminal/permission 回调
    ↓ provider.Connect() → *acp.Conn
provider.Provider        ← 连接工厂：启动 binary，返回 Conn（可多次调用）
    ↓
agent/acp.Conn         ← 低层传输：JSON-RPC 2.0 over stdio，拥有子进程生命周期
    ↓
CLI binary             ← codex-acp / claude-acp / ...
```

## 4. 命名变更

| 旧名 | 新名 | 说明 |
|------|------|------|
| `internal/hub` | `internal/client` | WheelMaker 是 ACP Client |
| `hub.Hub` | `client.Client` | 同上 |
| `internal/acp/` | `internal/agent/provider/` | acp 是 agent 层内部传输细节，移入 agent 子包 |
| `acp.Client` | `acp.Conn` | 低层传输，名副其实 |
| `internal/agent/codex/` | `internal/agent/provider/codex/` | Adapter 归到顶层 adapter 包 |
| `agent.Agent`（interface） | `agent.Session`（窄接口）+ `agent.Agent`（concrete struct） | Agent 是具体概念；Session 是 Client 用的可测试接口 |
| `hub.State.ACPSessionIDs` | `State.SessionIDs` | 字段改名，JSON tag 同步更新，**state.json 需迁移** |

## 5. 包结构

```
internal/
  agent/
    acp/               ← 低层传输（从 internal/acp/ 移入，是 agent 的内部细节）
      connect.go          ← Conn struct（原 client.go rename）
      conn_test.go     ← 原 client_test.go rename
      protocl.go         ← 不变

    agent.go           ← Agent struct + Session interface + 对外方法
    session.go         ← ACP 生命周期（initialize / session/new / session/load）
    prompt.go          ← session/prompt + session/update → Update channel
    callbacks.go       ← fs/* / terminal/* / permission 回调
    terminal.go        ← terminalManager
    permission.go      ← PermissionHandler interface + AutoAllowHandler
    update.go          ← Update / UpdateType 类型定义

  adapter/
    provider.go         ← Adapter interface（连接工厂，Connect 返回 *acp.Conn）
    codex/
      provider.go       ← CodexAdapter implements Adapter

  client/
    client.go          ← Client struct（原 hub.go），依赖 agent.Session 接口
    store.go           ← 不变
    state.go           ← State 字段调整（见第 8 节）

  im/                  ← 不变
  tools/               ← 不变
```

## 6. 接口定义

### 6.1 Adapter（连接工厂）

```go
// adapter/provider.go
Package provider

import (
    "context"
    "github.com/swm8023/wheelmaker/internal/agent"
)

// Adapter 抽象一个 ACP 兼容的 CLI 后端。
// 是一个无状态的连接工厂：Connect() 每次调用都启动一个新的 binary 子进程并返回其 Conn。
// 子进程的生命周期由返回的 *acp.Conn 持有；provider.Close() 仅用于 Connect() 失败时的清理。
// Connect() 成功后调用 provider.Close() 是 no-op（子进程已由 Conn 管理）。
// Connect() 内部调用 Conn.Start() 后返回，即调用完成时子进程已运行，不需要再次调用 Start()。
type Adapter interface {
    Name() string
    Connect(ctx context.Context) (*acp.Conn, error)
    Close() error
}
```

### 6.2 Session（Client 使用的窄接口，保障可测试性）

```go
// agent/agent.go
package agent

import "context"

// Session 是 client.Client 依赖的窄接口，仅包含 Client 需要调用的方法。
// agent.Agent struct 实现此接口；测试中可注入 mock。
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

// PermissionHandler 决定如何响应 CLI 的权限请求。
// MVP：AutoAllowHandler 自动选择 allow_once。
// Phase 2：IMPermissionHandler 路由到 IM（需要 Hub 提供 chatID 等上下文，
//           届时接口需扩展或通过闭包注入上下文）。
type PermissionHandler interface {
    RequestPermission(ctx context.Context,
        params acp.PermissionRequestParams) (acp.PermissionResult, error)
}

// AutoAllowHandler：无状态，自动选择 allow_once。
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
    UpdateDone       UpdateType = "done"        // prompt 结束，Content = stopReason
    UpdateError      UpdateType = "error"       // 错误，Err != nil
)

// Update 是 Agent 向 Client 发送的流式更新单元。
// Raw 仅在已知的结构化类型（tool_call、plan）中填充原始 JSON；
// 对纯文本类型（text、thought），Raw 为 nil。
// 对未知 sessionUpdate 类型，Type 使用原始字符串值，Raw 填充完整 params JSON。
type Update struct {
    Type    UpdateType
    Content string // 文本内容（text / thought / plan / stopReason）
    Raw     []byte // 结构化内容的原始 JSON（tool_call / plan / 未知类型）
    Done    bool
    Err     error
}
```

### 6.5 Agent（concrete struct）

```go
// agent/agent.go
package agent

// Agent 是 ACP 协议的完整封装。
// 它持有一个活跃的 *acp.Conn，处理出站 ACP 调用和入站 CLI 回调。
// 不持有 Adapter：Adapter 仅在 Connect 时由 Client 调用，之后由 Conn 管理生命周期。
type Agent struct {
    name       string                // 当前 adapter 名（标识用）
    conn       *acp.Conn             // 活跃的 ACP 连接（拥有子进程）
    caps       acp.AgentCapabilities // initialize 返回的能力声明
    sessionID  string
    cwd        string
    mcpServers []acp.MCPServer

    permission PermissionHandler     // 可注入，默认 AutoAllowHandler
    terminals  *terminalManager

    lastReply  string   // 最近一次完整 agent 回复，用于 SwitchWithContext
    mu         sync.Mutex
    ready      bool
}

// New 创建 Agent 并立即注册 Conn 上的回调处理器。
// 调用者（Client）负责在 Switch 时提供新的 Conn。
func New(name string, conn *acp.Conn, cwd string) *Agent

// --- Session interface 实现 ---
func (a *Agent) Prompt(ctx context.Context, text string) (<-chan Update, error)
func (a *Agent) Cancel() error
func (a *Agent) SetMode(ctx context.Context, modeID string) error
func (a *Agent) AdapterName() string
func (a *Agent) SessionID() string
func (a *Agent) Close() error

// --- 扩展方法（Client 直接调用，不通过 Session interface）---
func (a *Agent) SetConfigOption(ctx context.Context, configID, value string) error
func (a *Agent) Switch(ctx context.Context, name string, conn *acp.Conn, mode SwitchMode) error
func (a *Agent) SetPermissionHandler(h PermissionHandler)
```

### 6.6 SwitchMode

```go
// agent/agent.go
type SwitchMode int

const (
    // SwitchClean：丢弃当前 session，新 Conn 在下次 Prompt 时惰性初始化。
    SwitchClean SwitchMode = iota
    // SwitchWithContext：将当前 lastReply 作为初始上下文传入新 session。
    // 若上下文传递失败（lastReply 为空、Prompt 出错），降级为 SwitchClean 行为并返回警告。
    SwitchWithContext
)
```

### 6.7 Client（原 Hub）

```go
// client/client.go
package client

// Client 是 WheelMaker 的顶层协调器。
// 持有 Adapter 池（无状态工厂）；持有两个对 Agent 的引用：
//   - session agent.Session：窄接口，用于 Prompt/Cancel/SetMode 等日常操作，可 mock 替换。
//   - agent  *agent.Agent：具体类型指针，用于 Switch 等扩展方法，测试时可为 nil（不触发 Switch）。
// 切换 Adapter 时，Client 负责：
//   1. 调用 provider.Connect() 获取新 Conn
//   2. 调用 c.agent.Switch(newConn, ...) 替换连接（直接使用具体类型，不经过窄接口）
//   3. 将旧 Adapter 的 Close() 置为 no-op（Conn 已管理子进程生命周期）
type Client struct {
    adapters map[string]provider.Provider // "codex" → CodexAdapter（无状态工厂）
    session  agent.Session              // 当前活跃 session（窄接口，可 mock）
    agent    *agent.Agent               // 同一 Agent 的具体类型指针，用于 Switch 等扩展方法
    store    Store
    state    *State
    im       im.Adapter                 // nil in CLI/test mode
}

func New(store Store, im im.Adapter) *Client
func (c *Client) RegisterProvider(a provider.Provider)
// Start 加载持久化状态，创建初始 Agent 并调用 Connect() 完成连接（立即启动子进程，非惰性）。
// 此后首次收到 Prompt 时直接可用，无须等待子进程冷启动。
func (c *Client) Start(ctx context.Context) error
func (c *Client) Run(ctx context.Context) error  // 阻塞：驱动 IM 事件循环或 stdin 循环
func (c *Client) HandleMessage(msg im.Message)
func (c *Client) Close() error
```

### 6.8 acp.Conn（原 acp.Client）

```go
// agent/acp/connect.go — 移入 agent/acp/，并重命名，逻辑不变
package acp

// Conn 管理一个 ACP 兼容子进程的完整生命周期（stdio JSON-RPC）。
// Connect 后由 Conn 独占子进程所有权；Close() 关闭 stdin，等待进程退出。
type Conn struct { /* 原 Client 字段，不变 */ }

func New(exePath string, env []string) *Conn
func (c *Conn) Start() error
func (c *Conn) Send(ctx context.Context, method string, params any, result any) error
func (c *Conn) Notify(method string, params any) error
func (c *Conn) Subscribe(handler NotificationHandler) (cancel func())
func (c *Conn) OnRequest(handler RequestHandler)
func (c *Conn) Close() error
```

## 7. Agent 内部职责分区

### 7.1 Session 生命周期（session.go）

```
Agent.ensureReady(ctx):
  1. conn.Send("initialize", InitializeParams{...}) → 获取 caps
     InitializeParams 声明 fs.readTextFile=true, fs.writeTextFile=true, terminal=true
  2. 若 caps.LoadSession && sessionID != "" → conn.Send("session/load", ...)
  3. 否则 → conn.Send("session/new", {cwd, mcpServers}) → 存 a.sessionID
  4. conn.OnRequest(a.handleCallback)  ← 注册所有入站回调
```

### 7.2 Prompt 流（prompt.go）

```
Agent.Prompt(ctx, text):
  1. ensureReady(ctx)
  2. cancel := conn.Subscribe(sessionUpdateHandler)  ← 过滤本 sessionID
  3. goroutine:
       conn.Send("session/prompt", {sessionID, text}, &result)
       → 成功：发 Update{Type: UpdateDone, Content: result.StopReason, Done: true}
       → 失败：发 Update{Type: UpdateError, Err: err, Done: true}
       defer cancel(); defer close(updates)
  4. sessionUpdateHandler:
       将 session/update 子类型转为 Update（见下表），写入 channel
       同时将 agent_message_chunk 的文本追加到 a.lastReply（prompt 结束时固化）
```

`session/update` 子类型映射：

| `sessionUpdate` 值 | `UpdateType` | `Content` | `Raw` |
|---|---|---|---|
| `agent_message_chunk` | `text` | text | nil |
| `agent_thought_chunk` | `thought` | text | nil |
| `tool_call` / `tool_call_update` | `tool_call` | — | 完整 update JSON |
| `plan` | `plan` | — | 完整 update JSON |
| `current_mode_update` | `mode_change` | — | 完整 update JSON |
| 其余已知类型 | 原字符串 | — | 完整 update JSON |

### 7.3 入站回调（callbacks.go）

`conn.OnRequest(a.handleCallback)` 在 `ensureReady` 内注册。
回调使用 `context.Background()` 的 TODO：未来可在 `Conn.OnRequest` 传入 session-scoped context 以支持取消。

| ACP 方法 | 处理逻辑 |
|----------|----------|
| `fs/read_text_file` | `os.ReadFile(params.Path)` |
| `fs/write_text_file` | `os.MkdirAll` + `os.WriteFile` |
| `terminal/create` | `terminalManager.Create(...)` → 返回 `terminalId` |
| `terminal/output` | `terminalManager.Output(terminalID)` |
| `terminal/wait_for_exit` | `terminalManager.WaitForExit(terminalID)` |
| `terminal/kill` | `terminalManager.Kill(terminalID)` |
| `terminal/release` | `terminalManager.Release(terminalID)` |
| `session/request_permission` | `a.permission.RequestPermission(ctx, params)` |

### 7.4 Adapter 切换（agent.go）

切换流程由 **Client** 协调，Agent 只负责连接替换：

**并发安全约定**：`Switch` 调用前调用方（Client）必须先调用 `Cancel()` 并等待当前 Prompt goroutine
结束（即等待上次 `Prompt` 返回的 channel 关闭），再调用 `Switch`。
`Agent` 自身不在 `Switch` 内部等待 Prompt 完成——这是调用方的职责，可避免死锁（Cancel 需要发送
网络消息，若 Switch 持锁等待则可能死锁）。

```
// Client 侧（client.go）：
// 使用 c.agent（具体类型）调用 Switch，不经过 c.session（窄接口），
// 避免对 agent.Session interface 做类型断言（类型断言在 mock 注入时会 panic）。
func (c *Client) switchAdapter(ctx, name, mode):
    // 1. 先取消并排干当前 prompt（若有）
    c.session.Cancel()
    if c.currentPromptCh != nil:
        for range c.currentPromptCh {}   // 等待 Prompt goroutine 退出
        c.currentPromptCh = nil
    // 2. 启动新 binary
    newAdapter = c.adapters[name]
    newConn, err = newAdapter.Connect(ctx)
    if err: reply error
    // 3. 替换连接
    c.agent.Switch(ctx, name, newConn, mode)
    // 旧 Conn 由 Agent.Switch 内部关闭（Close）
    // Adapter 对象保留在 c.adapters，可再次 Connect（工厂语义）

// Agent 侧（agent.go）：
// 调用方已保证无并发 Prompt，Switch 加 mu 锁保护字段写入即可。
func (a *Agent) Switch(ctx, name, newConn, mode):
    a.mu.Lock()
    if mode == SwitchWithContext && a.lastReply != "":
        summary = a.lastReply
    a.killAllTerminals()       // 在锁内清理 terminals
    oldConn = a.conn
    a.conn = newConn
    a.name = name
    a.ready = false
    a.sessionID = ""
    a.lastReply = ""
    a.mu.Unlock()
    oldConn.Close()            // 锁外关闭旧 Conn（杀死旧子进程），避免 Close 阻塞持锁
    if mode == SwitchWithContext && summary != "":
        ch, err = a.Prompt(ctx, "[context] "+summary)
        if err == nil:
            go func() { for range ch {} }()  // 必须消费 channel，否则 Prompt 内部 goroutine 泄漏
        // Prompt 失败不影响 switch 成功，记录 warning 即可
    return nil
```

> **注**：`Client` 需在自身结构体中保存 `currentPromptCh <-chan Update` 字段，
> 每次 `Prompt` 调用后更新，用于 `switchAdapter` 中的排干操作。

## 8. 状态持久化

```go
// client/state.go
type AdapterConfig struct {
    ExePath string            `json:"exePath"`
    Env     map[string]string `json:"env"`
}

// State 变更说明（两处 breaking change，Load() 均需兼容读）：
//
// 1. 字段 ACPSessionIDs（json:"acp_session_ids"）→ SessionIDs（json:"session_ids"）
//    迁移：Load() 解析时若 "session_ids" 为空而 "acp_session_ids" 不为空，复制旧值。
//
// 2. 字段 ActiveAgent（json:"active_agent"）→ ActiveAdapter（json:"activeAdapter"）
//    迁移：Load() 解析时若 "activeAdapter" 为空而 "active_agent" 不为空，复制旧值。
//    （不复制会导致启动时 ActiveAdapter 为空，silently 丢失用户选择的 adapter）
//
// 写入只写新 key，不再写旧 key。
type State struct {
    ActiveAdapter string                   `json:"activeAdapter"`
    Adapters      map[string]AdapterConfig `json:"adapters"`
    SessionIDs    map[string]string        `json:"session_ids"` // adapter名 → ACP sessionId
}

// Load() 迁移伪代码：
//   raw := parseJSON(file)
//   if raw["activeAdapter"] == "" && raw["active_agent"] != "":
//       state.ActiveAdapter = raw["active_agent"]
//   if len(raw["session_ids"]) == 0 && len(raw["acp_session_ids"]) > 0:
//       state.SessionIDs = raw["acp_session_ids"]
```

## 9. CLI 命令处理

Client.HandleMessage 解析 `/` 前缀命令。
`/use` 现在会立即启动新子进程（不再是惰性），旧子进程同步关闭。

| 命令 | Client 行为 |
|------|-------------|
| `/use <name>` | switchAdapter(name, SwitchClean) |
| `/use <name> --continue` | switchAdapter(name, SwitchWithContext) |
| `/cancel` | session.Cancel() |
| `/status` | session.AdapterName() + session.SessionID() |
| 其他文本 | session.Prompt() → 流式回复 |

> **行为变更说明**：原 `/use` 仅更新 state.ActiveAgent，不立即启动子进程（惰性）。
> 新设计中 `/use` 立即 Connect，确保切换后首条 Prompt 响应更快，且旧进程立即释放资源。

## 10. cmd/wheelmaker/main.go 变更

```go
func run() error {
    store := client.NewJSONStore(statePath)
    // 注册所有可用 adapter
    c := client.New(store, nil)
    c.RegisterProvider(codex.NewProvider(codex.Config{...}))

    ctx, stop := signal.NotifyContext(...)
    defer stop()

    if err := c.Start(ctx); err != nil { return err }
    defer c.Close()

    return c.Run(ctx) // Run 阻塞：CLI 模式下驱动 stdin 循环
}
```

`Client.Run(ctx)` 在无 IM adapter 时驱动 stdin 读循环（原 main.go 的 for 循环逻辑移入此处）；
有 IM adapter 时调用 `im.provider.Run(ctx)`。

## 11. 测试策略

| 层 | 方式 |
|----|------|
| `agent/acp.Conn` | mock agent（测试二进制自身充当子进程），已有 16 个单元测试（文件名 rename 即可复用） |
| `provider/codex` | 集成测试，`//go:build integration`，需真实 codex-acp binary |
| `agent.Agent` | 测试二进制充当 mock Conn 子进程，测试 session lifecycle / prompt / switch / callbacks |
| `client.Client` | 注入 mock Session（实现 agent.Session interface），测试命令解析和路由 |

## 12. 变更范围

**新建：**
- `internal/agent/provider/provider.go`（Adapter interface）
- `internal/agent/provider/codex/provider.go`（CodexAdapter，原 codex adapter + handlers 合并，无 ACP 逻辑）
- `internal/agent/provider/connect.go`（原 `internal/acp/client.go` 移入并重命名）
- `internal/agent/provider/conn_test.go`（原 `internal/acp/client_test.go` 移入并重命名）
- `internal/agent/provider/protocl.go`（原 `internal/acp/protocl.go` 移入）
- `internal/agent/session.go`（ACP 生命周期）
- `internal/agent/prompt.go`（prompt 流）
- `internal/agent/callbacks.go`（入站回调）
- `internal/agent/terminal.go`（terminalManager）
- `internal/agent/permission.go`（PermissionHandler + AutoAllowHandler）
- `internal/agent/update.go`（Update 类型）

**重命名/重写：**
- `internal/hub/` → `internal/client/`（Hub → Client，新增 Session interface 依赖，新增 Run()，新增 currentPromptCh 字段）
- `internal/agent/agent.go`（删除 Agent interface，改为 concrete struct + Session interface）

**删除：**
- `internal/acp/`（整个目录，内容已移入 `internal/agent/provider/`）
- `internal/agent/codex/provider.go`
- `internal/agent/codex/handlers.go`

**不变：**
- `internal/im/im.go`
- `internal/tools/resolve.go`
- `internal/client/store.go`（原 hub/store.go，仅移包）









