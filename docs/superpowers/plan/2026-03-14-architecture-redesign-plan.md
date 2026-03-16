# WheelMaker 架构重设计

## 目标描述

重构现有 WheelMaker 代码库，使包命名与 ACP 协议语义对齐，并引入更清晰的抽象层次。核心变更如下：(1) 将 `internal/acp/` 传输层重命名为 `internal/agent/provider/`，`Client` 改为 `Conn`；(2) 将 `agent.Agent` 接口替换为具体的 `agent.Agent` struct 以及窄接口 `agent.Session`；(3) 引入新的 `internal/agent/provider/` 层作为无状态连接工厂；(4) 将 `internal/hub/` 重命名为 `internal/client/`，`Hub` 改为 `Client`，Client 同时持有 `session agent.Session`（用于日常操作，可 mock）和 `*agent.Agent`（用于 `Switch` 等）；(5) 迁移状态持久化 JSON key，保持向后兼容；(6) 将 stdin 循环从 `main.go` 移入 `client.Client.Run`。

所有现有行为必须保留：ACP JSON-RPC 通信、session 生命周期、fs/terminal/permission 回调、命令解析、状态持久化，以及已有的 16 个 `acp/client` 单元测试（重命名后继续通过）。

## 验收标准

遵循 TDD 理念，每条标准均包含正向测试和负向测试，以便确定性验证。

- AC-1: 包结构与规范一致（§5、§12）：新文件存在，旧文件已删除，Go 构建成功。
  - 正向测试（期望 PASS）：
    - `go build ./...` 在重构后无错误完成。
    - `go vet ./...` 无报告问题。
    - `internal/agent/provider/connect.go`、`internal/agent/provider/provider.go`、`internal/agent/provider/codex/provider.go`、`internal/client/client.go` 均存在。
    - `internal/agent/update.go`、`internal/agent/permission.go`、`internal/agent/terminal.go`、`internal/agent/callbacks.go`、`internal/agent/session.go`、`internal/agent/prompt.go` 均存在。
  - 负向测试（期望 FAIL）：
    - 旧目录 `internal/acp/`、`internal/hub/`、`internal/agent/codex/` 不再存在（任何对它们的文件引用均导致构建错误）。

- AC-2: `acp.Conn`（从 `acp.Client` 重命名）通过全部 16 个现有单元测试，迁移至 `internal/agent/provider/conn_test.go`。
  - 正向测试（期望 PASS）：
    - `go test ./internal/agent/provider/...` 运行，全部 16 个测试通过。
    - 自引用 mock 模式（`GO_ACP_MOCK=1`）在重命名后的 `Conn` 类型下正常工作。
    - 所有已测试行为正常：请求/响应、并发请求、订阅、取消、`OnRequest` 处理器、进程退出时解除阻塞。
  - 负向测试（期望 FAIL）：
    - 向已关闭的 `Conn` 发送消息返回 error（不 panic 或挂起）。

- AC-3: `agent.Session` 接口是 `client.Client` 依赖的窄契约；`agent.Agent` struct 实现该接口。
  - 正向测试（期望 PASS）：
    - 实现了 `agent.Session` 的 mock struct 可以注入 `client.Client` 而不引发类型断言 panic。
    - `agent.Agent` struct 作为 `agent.Session` 的实现可编译通过（通过 `var _ agent.Session = (*agent.Agent)(nil)` 验证）。
    - `agent.Agent.AdapterName()` 和 `agent.Agent.SessionID()` 返回正确的值。
  - 负向测试（期望 FAIL）：
    - 缺少六个 `Session` 方法（`Prompt`、`Cancel`、`SetMode`、`AdapterName`、`SessionID`、`Close`）中任意一个的 struct 无法作为 `agent.Session` 编译通过。

- AC-4: `provider.Provider` 接口和 `provider/codex.CodexAdapter` 作为无状态连接工厂正确工作。
  - 正向测试（期望 PASS）：
    - `CodexAdapter.Connect(ctx)` 返回的 `*acp.Conn` 子进程已在运行（调用方无需再次调用 `Start()`）。
    - 两次调用 `CodexAdapter.Connect(ctx)` 产生两个独立的 `*acp.Conn` 实例，各自有独立子进程。
    - `Connect` 成功后调用 `CodexAdapter.Close()` 是 no-op（不会杀死已转移所有权的子进程）。
  - 负向测试（期望 FAIL）：
    - 找不到二进制文件时，`CodexAdapter.Connect(ctx)` 返回 error。

- AC-5: `client.Client` 以新设计替换 `hub.Hub`；命令路由和流式回复端到端正常工作。
  - 正向测试（期望 PASS）：
    - `Client.HandleMessage` 处理 `/use codex` 时触发 `switchAdapter`，调用 `provider.Connect()` 并替换活跃 session。
    - `Client.HandleMessage` 处理 `/use codex --continue` 时以 `SwitchWithContext` 模式触发 `switchAdapter`。
    - `Client.HandleMessage` 处理 `/cancel` 时调用 `session.Cancel()`。
    - `Client.HandleMessage` 处理 `/status` 时返回包含 adapter 名称和 session ID 的字符串。
    - `Client.Run(ctx)` 在无 IM adapter 时驱动 stdin 读循环，并将每行转发为 `im.Message`。
    - `Client.Start(ctx)` 立即调用 `provider.Connect()`（非惰性），首条 prompt 前子进程已就绪。
    - `Client.Close()` 将当前 session ID 保存到 store。
  - 负向测试（期望 FAIL）：
    - `/use unknown-adapter` 返回指示 adapter 未注册的错误信息。
    - 注入 `client.Client` 的 mock `agent.Session` 在调用 `switchAdapter` 时不引发类型断言 panic（因为 `Switch` 通过 `c.agent *agent.Agent` 调用，而非 `Session` 接口）。

- AC-6: `agent.Agent.Switch` 实现 §7.4 中规定的并发安全约定。
  - 正向测试（期望 PASS）：
    - `switchAdapter`（Client 侧）在调用 `c.agent.Switch(...)` 前先调用 `c.session.Cancel()`，再排干 `c.currentPromptCh`。
    - `Switch` 后，旧 `*acp.Conn` 已关闭（旧子进程终止），`c.agent` 持有新 `*acp.Conn`。
    - `SwitchWithContext` 且 `lastReply` 非空时，将其作为引导 prompt 发送至新 session；消费 goroutine 已启动且不泄漏。
    - `SwitchWithContext` 且 `lastReply` 为空时，静默降级为 `SwitchClean` 行为（不发送 prompt，不返回 error）。
    - `SwitchWithContext` 中引导 `Prompt` 调用失败时，`Switch` 返回 `nil`（而非 error），并记录 warning。
  - 负向测试（期望 FAIL）：
    - `Agent.Switch` 内部不等待飞行中的 `Prompt` 完成——这是调用方的文档化职责，以防死锁。

- AC-7: 状态持久化透明迁移旧 JSON key。
  - 正向测试（期望 PASS）：
    - `client.JSONStore.Load()` 读取含 `"active_agent"` 和 `"acp_session_ids"` 的状态文件时，正确映射到加载后 `State` 的 `ActiveAdapter` 和 `SessionIDs`。
    - `client.JSONStore.Save()` 仅写入新 key（`"activeAdapter"`、`"session_ids"`），不写旧 key。
    - 使用旧格式状态文件（仅含旧 key）启动的进程能正确恢复 session ID 和活跃 adapter。
  - 负向测试（期望 FAIL）：
    - 仅含旧 key（`active_agent`、`acp_session_ids`）、不含新 key 的状态文件在 `Load()` 后不会导致 `ActiveAdapter` 为空。

## 路径边界

路径边界定义了实现质量和选择的可接受范围。

### 上界（最大可接受范围）
实现包含 §12 中所有包重命名/迁移、§6 中所有新接口和具体类型、`agent.Agent` 内部拆分为多文件（session.go、prompt.go、callbacks.go、terminal.go、permission.go、update.go）、`agent.Agent` 单元测试（使用 §11 中的 mock Conn 子进程模式）和 `client.Client` 单元测试（使用 mock `agent.Session`）、`provider/codex` 的集成测试（`//go:build integration`），以及完整的向后兼容状态迁移。

### 下界（最小可接受范围）
实现完成所有指定的包重命名/迁移，`go build ./...` 通过，全部 16 个迁移后的 `acp/conn` 单元测试通过，stdin 循环端到端可用（`go run ./cmd/wheelmaker/`）。`agent.Agent` 和 `client.Client` 的单元测试可推迟，前提是代码结构具备可测试性（Session 接口可注入 Client）。

### 允许的选择
> **注意**：这是高度确定性的重构，几乎所有结构性决策均由规范固定。仅剩少量实现层面的选择。

- 可以：在 `internal/agent/` 内部以任意方式拆分文件，只要公共 API 与 §6 完全匹配。
- 可以：单独创建 `conn_integration_test.go`，或在同一文件中使用 `//go:build integration` 标签。
- 可以：在 `agent.Agent` 实现文件内使用任意内部辅助命名。
- 不可以：任何未将 `internal/agent/provider/` 作为独立包并引入 `Adapter` 接口的架构。
- 不可以：任何将 `agent.Agent` 保留为接口的架构（必须改为具体 struct；`Session` 才是窄接口）。
- 不可以：在 `client.Client` 中保留对 `*codex.Agent` 的类型断言以获取 `SessionID()` 或其他字段。
- 所有公共接口方法签名、struct 字段和 JSON tag 均按 §6 和 §8 固定，不可更改。

## 可行性提示与建议

> **注意**：本节仅供参考理解，这些是概念性建议，不是强制要求。

### 概念实现路径

重构可按依赖顺序进行，确保每一步代码始终可编译：

```
传输层迁移：
  - 将 internal/acp/ 复制到 internal/agent/provider/
  - 将类型 Client→Conn，更新所有方法接收者
  - 将 client_test.go 重命名为 conn_test.go，更新类型引用
  - 保留 internal/acp/ 直到所有依赖方更新完毕

Agent 核心（可与传输层迁移并行）：
  - update.go:     新的 UpdateType 枚举 + 更丰富的 Update struct
  - permission.go: PermissionHandler 接口 + AutoAllowHandler（始终 allow_once）
  - terminal.go:   从 codex/handlers.go 抽取 terminalManager
  - callbacks.go:  8 个 ACP 回调处理器（fs/terminal/permission）
  - session.go:    ensureReady（initialize → session/load 或 session/new）
  - prompt.go:     Prompt goroutine + sessionUpdateHandler channel 映射
  - agent.go:      Session 接口、SwitchMode、Agent struct、New、Switch

Adapter 层 + Client（依赖以上两部分完成）：
  - internal/agent/provider/provider.go:       Adapter 接口
  - internal/agent/provider/codex/provider.go: CodexAdapter（ResolveBinary + Conn.Start）
  - internal/client/state.go:          带迁移逻辑的 State（见 Load()）
  - internal/client/store.go:          包声明从 hub 改为 client（内容不变）
  - internal/client/client.go:         Client struct + 所有方法

接线 + 清理：
  - cmd/wheelmaker/main.go: 使用 client.New、RegisterProvider、Start、Run
  - 删除 internal/acp/、internal/hub/、internal/agent/codex/
  - go build ./... 和 go test ./internal/agent/provider/...
```

### 相关参考
- `internal/acp/client.go` — 待迁移的完整 `Conn` 实现（不需要重写，逻辑不变）
- `internal/acp/client_test.go` — 16 个测试，重命名/迁移至 `conn_test.go`
- `internal/agent/codex/provider.go` — `ensureSession`、`Prompt`、`Cancel`、`SetMode` 逻辑，拆分至 `session.go`、`prompt.go`、`agent.go`
- `internal/agent/codex/handlers.go` — 所有回调处理器 + `terminalManager`，迁移至 `callbacks.go` + `terminal.go`
- `internal/hub/hub.go` — `HandleMessage`、命令分发、回复流式处理，迁移至 `client.go`
- `internal/hub/state.go` — `State` struct，需更新并加入迁移逻辑
- `internal/tools/resolve.go` — `CodexAdapter.Connect` 用于查找二进制路径

## 依赖与执行顺序

### 里程碑

1. **传输层迁移**：迁移并重命名 ACP JSON-RPC 传输层。
   - 步骤 A：复制 `internal/acp/` 内容，创建 `internal/agent/provider/`。
   - 步骤 B：将 `Client` 重命名为 `Conn`，更新新包内所有引用。
   - 步骤 C：更新 `conn_test.go` 使用 `Conn` 类型；验证 `go test ./internal/agent/provider/...` 全部 16 个测试通过。
   - 步骤 D：保留 `internal/acp/` 直到所有依赖方完成迁移。

2. **Agent 核心重构**：构建新的 `agent.Agent` 具体 struct 和 `agent.Session` 接口。
   - 步骤 A：编写 `update.go`，包含更丰富的 `UpdateType` 枚举和 `Update` struct（替换原来的简单 `Update` string 类型）。
   - 步骤 B：编写 `permission.go` 和 `terminal.go`（从 `codex/handlers.go` 抽取）。
   - 步骤 C：编写 `callbacks.go`（全部 8 个 ACP 回调处理器，使用 `agent/acp` 的 `acp.Conn`）。
   - 步骤 D：编写 `session.go`（`ensureReady` 生命周期：initialize → session/load 或 session/new）。
   - 步骤 E：编写 `prompt.go`（`Prompt` goroutine 和 `sessionUpdateHandler` 更新类型映射）。
   - 步骤 F：编写 `agent.go`，定义 `Session` 接口、`SwitchMode`、`Agent` struct、`New`、`Switch` 以及六个 `Session` 接口方法实现。

3. **Adapter 层 + Client 重构**：创建 Adapter 抽象，以 Client 替换 Hub。
   - 步骤 A：创建 `internal/agent/provider/provider.go`，包含 `Adapter` 接口。
   - 步骤 B：创建 `internal/agent/provider/codex/provider.go`，包含 `CodexAdapter`（使用 `tools.ResolveBinary`，创建 `acp.Conn`，调用 `Conn.Start()`，返回已启动的 `Conn`）。
   - 步骤 C：创建 `internal/client/state.go`，包含更新后的 `State` struct 和向后兼容的 `Load()`。
   - 步骤 D：创建 `internal/client/store.go`（包声明从 `hub` 改为 `client`，内容不变）。
   - 步骤 E：创建 `internal/client/client.go`，包含 `Client` struct、`RegisterProvider`、`Start`、`Run`、`HandleMessage`、`switchAdapter`、`Close`。

4. **清理与收尾**：接线 `main.go` 并删除旧包。
   - 步骤 A：更新 `cmd/wheelmaker/main.go`，使用 `client.New`、`client.RegisterProvider`、`c.Start`、`c.Run`。
   - 步骤 B：删除 `internal/acp/` 整个目录。
   - 步骤 C：删除 `internal/hub/` 整个目录。
   - 步骤 D：删除 `internal/agent/codex/` 目录及旧的 `internal/agent/agent.go` 接口文件。
   - 步骤 E：验证 `go build ./...`、`go vet ./...` 和 `go test ./internal/agent/provider/...` 全部通过。

里程碑 1 和 2 可并行推进（无相互依赖）。里程碑 3 依赖 1 和 2 均完成。里程碑 4 依赖里程碑 3。

## 实现备注

### 代码风格要求
- 所有实现代码和注释必须使用英文（项目约定：代码和注释中不得出现中文）。
- 实现代码和注释中不得包含计划专用术语，如 "AC-"、"里程碑"、"步骤"、"阶段" 或类似的工作流标记。
- 这些术语仅用于计划文档，不应出现在最终代码库中。
- 代码中使用描述性、领域相关的命名（如 `ensureReady`、`switchAdapter`、`handleCallback`），而非进度标记。

### 规范中的关键实现约束
- `provider.Connect()` 必须在内部调用 `Conn.Start()` 后返回，调用方无需再次调用 `Start()`。
- `Client.Start()` 必须立即（非惰性）调用 `provider.Connect()`，确保首条消息到来之前子进程已就绪。
- `Agent.Switch()` 必须在互斥锁内调用 `killAllTerminals()`，在互斥锁外调用 `oldConn.Close()`（避免 `Close` 阻塞时持锁死锁）。
- `SwitchWithContext` 引导 prompt 的 goroutine 必须完整消费返回的 channel（`for range ch {}`）；省略此操作会导致 `Prompt` goroutine 泄漏。
- `Client.switchAdapter()` 必须在调用 `c.agent.Switch()` 前排干 `c.currentPromptCh`（上次 `Prompt` 调用返回的 channel）。
- `Client` 同时持有 `session agent.Session` 和 `agent *agent.Agent` 指向同一对象。`Switch` 始终通过 `c.agent`（具体类型）调用，而非 `c.session`（接口），以避免 mock 注入时的类型断言 panic。
- 状态 `Load()` 同时处理旧 key（`active_agent`、`acp_session_ids`）和新 key（`activeAdapter`、`session_ids`）；`Save()` 仅写入新 key。

--- Original Design Draft Start ---

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

// Adapter abstracts an ACP-compatible CLI backend.
// It is a stateless connection factory: Connect() spawns a new binary subprocess each time and returns its Conn.
// The subprocess lifecycle is owned by the returned *acp.Conn; provider.Close() is only for cleanup on Connect() failure.
// Calling provider.Close() after a successful Connect() is a no-op (subprocess is managed by the Conn).
// Connect() calls Conn.Start() internally; the subprocess is running when Connect() returns.
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

// Session is the narrow interface that client.Client depends on.
// Only the methods Client needs for day-to-day operations are included.
// agent.Agent struct implements this interface; tests can inject mocks.
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

// PermissionHandler decides how to respond to CLI permission requests.
// MVP: AutoAllowHandler auto-selects allow_once.
// Phase 2: IMPermissionHandler routes to IM (requires Hub to provide chatID context;
//           interface will need extension or context injection via closure).
type PermissionHandler interface {
    RequestPermission(ctx context.Context,
        params acp.PermissionRequestParams) (acp.PermissionResult, error)
}

// AutoAllowHandler: stateless, always selects allow_once.
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
    UpdateDone       UpdateType = "done"        // prompt ended, Content = stopReason
    UpdateError      UpdateType = "error"       // error, Err != nil
)

// Update is a streaming update unit sent from Agent to Client.
// Raw is populated only for structured types (tool_call, plan);
// for plain text types (text, thought), Raw is nil.
// For unknown sessionUpdate types, Type uses the raw string value and Raw holds the full params JSON.
type Update struct {
    Type    UpdateType
    Content string // text content (text / thought / plan / stopReason)
    Raw     []byte // raw JSON for structured content (tool_call / plan / unknown types)
    Done    bool
    Err     error
}
```

### 6.5 Agent（concrete struct）

```go
// agent/agent.go
package agent

// Agent is the complete ACP protocol encapsulation.
// It holds an active *acp.Conn, handles outbound ACP calls and inbound CLI callbacks.
// It does not hold an Adapter: the Adapter is used only during Connect by the Client;
// after that the Conn owns the subprocess lifecycle.
type Agent struct {
    name       string                // current adapter name (for identification)
    conn       *acp.Conn             // active ACP connection (owns the subprocess)
    caps       acp.AgentCapabilities // capabilities declared by initialize response
    sessionID  string
    cwd        string
    mcpServers []acp.MCPServer

    permission PermissionHandler     // injectable, defaults to AutoAllowHandler
    terminals  *terminalManager

    lastReply  string   // most recent complete agent reply, used for SwitchWithContext
    mu         sync.Mutex
    ready      bool
}

// New creates an Agent and immediately registers callback handlers on the Conn.
// The caller (Client) is responsible for providing a new Conn during Switch.
func New(name string, conn *acp.Conn, cwd string) *Agent

// --- Session interface implementation ---
func (a *Agent) Prompt(ctx context.Context, text string) (<-chan Update, error)
func (a *Agent) Cancel() error
func (a *Agent) SetMode(ctx context.Context, modeID string) error
func (a *Agent) AdapterName() string
func (a *Agent) SessionID() string
func (a *Agent) Close() error

// --- Extended methods (Client calls directly, not through Session interface) ---
func (a *Agent) SetConfigOption(ctx context.Context, configID, value string) error
func (a *Agent) Switch(ctx context.Context, name string, conn *acp.Conn, mode SwitchMode) error
func (a *Agent) SetPermissionHandler(h PermissionHandler)
```

### 6.6 SwitchMode

```go
// agent/agent.go
type SwitchMode int

const (
    // SwitchClean: discard the current session; new Conn is lazily initialized on next Prompt.
    SwitchClean SwitchMode = iota
    // SwitchWithContext: send the current lastReply as initial context to the new session.
    // If context transfer fails (lastReply is empty or Prompt errors), falls back to SwitchClean behavior with a warning.
    SwitchWithContext
)
```

### 6.7 Client（原 Hub）

```go
// client/client.go
package client

// Client is the top-level coordinator of WheelMaker.
// Holds an Adapter pool (stateless factories); holds two references to the Agent:
//   - session agent.Session: narrow interface for Prompt/Cancel/SetMode etc., mockable.
//   - agent  *agent.Agent:   concrete type pointer for Switch and other extended methods, can be nil in tests.
// When switching adapters, Client:
//   1. Calls provider.Connect() to get a new Conn
//   2. Calls c.agent.Switch(newConn, ...) to replace the connection (via concrete type, not Session interface)
//   3. The old Adapter's Close() is a no-op (Conn already owns the subprocess lifecycle)
type Client struct {
    adapters map[string]provider.Provider // "codex" → CodexAdapter (stateless factory)
    session  agent.Session              // current active session (narrow interface, mockable)
    agent    *agent.Agent               // same Agent as concrete type pointer, for Switch and extended methods
    store    Store
    state    *State
    im       im.Adapter                 // nil in CLI/test mode
}

func New(store Store, im im.Adapter) *Client
func (c *Client) RegisterProvider(a provider.Provider)
// Start loads persisted state, creates the initial Agent, and calls Connect() eagerly (non-lazy, subprocess ready immediately).
func (c *Client) Start(ctx context.Context) error
func (c *Client) Run(ctx context.Context) error  // blocking: drives IM event loop or stdin loop
func (c *Client) HandleMessage(msg im.Message)
func (c *Client) Close() error
```

### 6.8 acp.Conn（原 acp.Client）

```go
// agent/acp/connect.go — moved into agent/acp/, renamed, logic unchanged
package acp

// Conn manages the full lifecycle of an ACP-compatible subprocess (stdio JSON-RPC).
// After Connect, Conn exclusively owns the subprocess; Close() closes stdin and waits for process exit.
type Conn struct { /* same fields as original Client */ }

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
  1. conn.Send("initialize", InitializeParams{...}) → get caps
     InitializeParams declares fs.readTextFile=true, fs.writeTextFile=true, terminal=true
  2. if caps.LoadSession && sessionID != "" → conn.Send("session/load", ...)
  3. else → conn.Send("session/new", {cwd, mcpServers}) → store a.sessionID
  4. conn.OnRequest(a.handleCallback)  ← register all inbound callbacks
```

### 7.2 Prompt 流（prompt.go）

```
Agent.Prompt(ctx, text):
  1. ensureReady(ctx)
  2. cancel := conn.Subscribe(sessionUpdateHandler)  ← filter by this sessionID
  3. goroutine:
       conn.Send("session/prompt", {sessionID, text}, &result)
       → success: send Update{Type: UpdateDone, Content: result.StopReason, Done: true}
       → failure: send Update{Type: UpdateError, Err: err, Done: true}
       defer cancel(); defer close(updates)
  4. sessionUpdateHandler:
       convert session/update subtypes to Update (see table below)
       also append agent_message_chunk text to a.lastReply (finalized at prompt end)
```

`session/update` subtype mapping:

| `sessionUpdate` value | `UpdateType` | `Content` | `Raw` |
|---|---|---|---|
| `agent_message_chunk` | `text` | text | nil |
| `agent_thought_chunk` | `thought` | text | nil |
| `tool_call` / `tool_call_update` | `tool_call` | — | full update JSON |
| `plan` | `plan` | — | full update JSON |
| `current_mode_update` | `mode_change` | — | full update JSON |
| other known types | raw string | — | full update JSON |

### 7.3 入站回调（callbacks.go）

`conn.OnRequest(a.handleCallback)` registered inside `ensureReady`.
Callbacks use `context.Background()` as TODO: future work can pass session-scoped context via `Conn.OnRequest` to support cancellation.

| ACP Method | Handler Logic |
|----------|----------|
| `fs/read_text_file` | `os.ReadFile(params.Path)` |
| `fs/write_text_file` | `os.MkdirAll` + `os.WriteFile` |
| `terminal/create` | `terminalManager.Create(...)` → return `terminalId` |
| `terminal/output` | `terminalManager.Output(terminalID)` |
| `terminal/wait_for_exit` | `terminalManager.WaitForExit(terminalID)` |
| `terminal/kill` | `terminalManager.Kill(terminalID)` |
| `terminal/release` | `terminalManager.Release(terminalID)` |
| `session/request_permission` | `a.permission.RequestPermission(ctx, params)` |

### 7.4 Adapter 切换（agent.go）

Switching is coordinated by **Client**; Agent only handles connection replacement:

**Concurrency contract**: Before calling `Switch`, the caller (Client) MUST call `Cancel()` and wait for the current Prompt goroutine to finish (i.e., wait for the channel from the last `Prompt` to close), then call `Switch`.
`Agent` itself does NOT wait for Prompt completion inside `Switch` — that is the caller's responsibility to avoid deadlock (Cancel must send a network message; holding a lock while waiting would deadlock).

```
// Client side (client.go):
// Uses c.agent (concrete type) to call Switch, NOT c.session (interface),
// to avoid type assertion panics when a mock is injected.
func (c *Client) switchAdapter(ctx, name, mode):
    // 1. cancel and drain the current prompt (if any)
    c.session.Cancel()
    if c.currentPromptCh != nil:
        for range c.currentPromptCh {}   // wait for Prompt goroutine to exit
        c.currentPromptCh = nil
    // 2. start new binary
    newAdapter = c.adapters[name]
    newConn, err = newAdapter.Connect(ctx)
    if err: reply error
    // 3. replace connection
    c.agent.Switch(ctx, name, newConn, mode)
    // old Conn is closed inside Agent.Switch (via Close)
    // Adapter object stays in c.adapters for future Connect calls (factory semantics)

// Agent side (agent.go):
// Caller guarantees no concurrent Prompt; Switch just needs mu lock for field writes.
func (a *Agent) Switch(ctx, name, newConn, mode):
    a.mu.Lock()
    if mode == SwitchWithContext && a.lastReply != "":
        summary = a.lastReply
    a.killAllTerminals()       // clean up terminals while holding lock
    oldConn = a.conn
    a.conn = newConn
    a.name = name
    a.ready = false
    a.sessionID = ""
    a.lastReply = ""
    a.mu.Unlock()
    oldConn.Close()            // close old Conn outside lock (kills old subprocess), avoids Close blocking with lock held
    if mode == SwitchWithContext && summary != "":
        ch, err = a.Prompt(ctx, "[context] "+summary)
        if err == nil:
            go func() { for range ch {} }()  // must consume channel, otherwise Prompt goroutine leaks
        // Prompt failure does not affect switch success; log warning only
    return nil
```

> **Note**: `Client` must store a `currentPromptCh <-chan Update` field, updated after each `Prompt` call, used in `switchAdapter` for the drain operation.

## 8. 状态持久化

```go
// client/state.go
type AdapterConfig struct {
    ExePath string            `json:"exePath"`
    Env     map[string]string `json:"env"`
}

// State change notes (two breaking changes; Load() must handle both):
//
// 1. Field ACPSessionIDs (json:"acp_session_ids") → SessionIDs (json:"session_ids")
//    Migration: if "session_ids" is empty but "acp_session_ids" is not, copy old value.
//
// 2. Field ActiveAgent (json:"active_agent") → ActiveAdapter (json:"activeAdapter")
//    Migration: if "activeAdapter" is empty but "active_agent" is not, copy old value.
//    (Omitting this causes ActiveAdapter to be empty on startup, silently losing user's adapter selection)
//
// Save writes only new keys; no longer writes old keys.
type State struct {
    ActiveAdapter string                   `json:"activeAdapter"`
    Adapters      map[string]AdapterConfig `json:"adapters"`
    SessionIDs    map[string]string        `json:"session_ids"` // adapter name → ACP sessionId
}

// Load() migration pseudocode:
//   raw := parseJSON(file)
//   if raw["activeAdapter"] == "" && raw["active_agent"] != "":
//       state.ActiveAdapter = raw["active_agent"]
//   if len(raw["session_ids"]) == 0 && len(raw["acp_session_ids"]) > 0:
//       state.SessionIDs = raw["acp_session_ids"]
```

## 9. CLI 命令处理

Client.HandleMessage parses `/`-prefixed commands.
`/use` now immediately starts a new subprocess (no longer lazy); old subprocess is synchronously closed.

| Command | Client Behavior |
|------|-------------|
| `/use <name>` | switchAdapter(name, SwitchClean) |
| `/use <name> --continue` | switchAdapter(name, SwitchWithContext) |
| `/cancel` | session.Cancel() |
| `/status` | session.AdapterName() + session.SessionID() |
| other text | session.Prompt() → streaming reply |

> **Behavior change**: Original `/use` only updated `state.ActiveAgent` without starting the subprocess (lazy).
> New design has `/use` immediately Connect, ensuring the first Prompt after switch is faster, and the old process releases resources immediately.

## 10. cmd/wheelmaker/main.go 变更

```go
func run() error {
    store := client.NewJSONStore(statePath)
    c := client.New(store, nil)
    c.RegisterProvider(codex.NewProvider(codex.Config{...}))

    ctx, stop := signal.NotifyContext(...)
    defer stop()

    if err := c.Start(ctx); err != nil { return err }
    defer c.Close()

    return c.Run(ctx) // Run blocks: CLI mode drives stdin loop
}
```

`Client.Run(ctx)` drives the stdin read loop when there is no IM adapter (original `main.go` for loop logic moves here);
with an IM adapter it calls `im.provider.Run(ctx)`.

## 11. 测试策略

| Layer | Approach |
|----|------|
| `agent/acp.Conn` | mock agent (test binary acts as subprocess), existing 16 unit tests (file rename only) |
| `provider/codex` | integration tests, `//go:build integration`, requires real codex-acp binary |
| `agent.Agent` | test binary acts as mock Conn subprocess, tests session lifecycle / prompt / switch / callbacks |
| `client.Client` | inject mock Session (implements agent.Session interface), tests command parsing and routing |

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

--- Original Design Draft End ---









