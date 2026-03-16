# Agent Plugin Architecture & Config Option Fix

**Date**: 2026-03-17
**Status**: Approved

---

## Problem Statement

当前代码存在三个关联问题：

1. **Validator 逻辑 bug**：`claudeConfigOptionsValidator` / `codexConfigOptionsValidator` 要求 config option update 中同时包含 `mode` 和 `model`。但 `session/update` 的 `config_option_update` 和 `SetConfigOption` 的响应都可能是 partial update（只包含变化的那一项），导致合法更新被静默丢弃。同样，`session.go:164` 在 `session/new` 返回后以同样错误的逻辑验证 `newResult.ConfigOptions`，可能导致初始连接失败。

2. **存盘时机错误**：`handlePrompt` 收到 `UpdateConfigOption` 后只通知用户，但不存盘。存盘操作推迟到整个 prompt 结束后（`saveAgentState(ag)`）。如果进程中途崩溃，config option 变更会丢失。

3. **Setter 注入架构粗暴**：`acp.Agent` 通过 `SetConfigOptionsValidator` / `SetPermissionHandler` 两个 setter 接收差异化行为，各 agent 包通过 duck-typed `configOptionsValidatorProvider` 接口向 client 暴露 validator。这种模式导致：
   - 差异化逻辑分散在 `internal/acp/` 和各 agent 包之间；
   - 每次新增扩展点都需要同时改 acp 层 + client 层；
   - 三份几乎相同的 validator 实现（acp、claude、codex）；
   - `NormalizeParams`（旧协议转新协议）等未来扩展点无处放置。

---

## Design

### 核心思路

- `acp.Agent` 保持为共享核心实现，通过 `AgentPlugin` 接口接收差异化行为。
- 各 agent 包（claude、codex）各自实现 plugin，嵌入 `DefaultPlugin` 后只覆盖需要的方法。
- `agent.Agent` 工厂接口增加 `Plugin()` 方法，client 在构造 `acp.Agent` 时传入 plugin。
- `Switch` 方法增加 `plugin AgentPlugin` 参数，确保切换 agent 后 plugin 也同步更新。
- 删除所有 setter 和 duck-typed 辅助接口。

---

### 1. `acp/plugin.go`（新文件）

```go
// AgentPlugin is the per-agent customization interface for acp.Agent.
// Implementations embed DefaultPlugin and override only the methods they need.
type AgentPlugin interface {
    // ValidateConfigOptions validates options from a config_option_update notification,
    // SetConfigOption response, or session/new result. Only validate fields that are
    // present — partial updates are allowed. Return non-nil to reject and log;
    // the update is dropped (or in the session/new case, connection fails).
    ValidateConfigOptions(opts []ConfigOption) error

    // HandlePermission decides how to respond to an incoming session/request_permission
    // callback. Signature matches the existing PermissionHandler interface so it can
    // replace it directly.
    HandlePermission(ctx context.Context, params PermissionRequestParams) (PermissionResult, error)

    // NormalizeParams is called before acp processes each incoming session/update
    // notification (both in the Prompt subscriber and the session/load replay subscriber).
    // Implementations may translate legacy protocol fields to modern format.
    // Return params unchanged for pass-through (current and default behavior).
    NormalizeParams(method string, params json.RawMessage) json.RawMessage
}

// DefaultPlugin is the zero-value implementation of AgentPlugin.
// All methods are no-ops or auto-allow. Embed this in agent-specific plugins
// so adding new extension points in future does not require updating all implementations.
type DefaultPlugin struct{}

func (DefaultPlugin) ValidateConfigOptions(_ []ConfigOption) error { return nil }

// HandlePermission auto-selects allow_once (matching existing AutoAllowHandler behaviour).
func (DefaultPlugin) HandlePermission(_ context.Context, params PermissionRequestParams) (PermissionResult, error) {
    optionID := ""
    for _, opt := range params.Options {
        if opt.Kind == "allow_once" {
            optionID = opt.OptionID
            break
        }
    }
    if optionID == "" && len(params.Options) > 0 {
        optionID = params.Options[0].OptionID
    }
    return PermissionResult{Outcome: "selected", OptionID: optionID}, nil
}

func (DefaultPlugin) NormalizeParams(_ string, params json.RawMessage) json.RawMessage { return params }
```

**注意**：`DefaultPlugin.HandlePermission` 的签名与现有 `PermissionHandler.RequestPermission` 完全相同，`permission.go` 中的 `PermissionHandler` 接口和 `AutoAllowHandler` 类型可以删除，`callbacks.go` 中对 `a.permission.RequestPermission(...)` 的调用改为 `a.plugin.HandlePermission(...)`。

**实现约束**：任何 `AgentPlugin` 实现在 `ValidateConfigOptions` 内部**不得**尝试获取 `acp.Agent` 的内部锁（`mu` 或 `configOptsMu`），因为 `setConfigOptions` 本身在持有 `configOptsMu` 时调用该方法，嵌套获取 `mu` 会导致死锁。

---

### 2. `acp/agent.go` 变更

**删除字段**：
- `validator ConfigOptionsValidator`
- `permission PermissionHandler`

**新增字段**：
- `plugin AgentPlugin`

**`New` / `NewWithSessionID` 签名变更**：

```go
func New(name string, conn *agent.Conn, cwd string, plugin AgentPlugin) *Agent
func NewWithSessionID(name string, conn *agent.Conn, cwd string, sessionID string, plugin AgentPlugin) *Agent
```

`plugin` 为 nil 时自动使用 `DefaultPlugin{}`。`pluginOrDefault` 为包内辅助函数：

```go
func pluginOrDefault(p AgentPlugin) AgentPlugin {
    if p == nil {
        return DefaultPlugin{}
    }
    return p
}
```

`New` 和 `NewWithSessionID` 内部均调用 `pluginOrDefault(plugin)` 赋值到 `ag.plugin`。

**`Switch` 签名变更**（增加 `plugin` 参数）：

```go
func (a *Agent) Switch(ctx context.Context, name string, newConn *agent.Conn, mode SwitchMode, savedSessionID string, plugin AgentPlugin) error
```

在 `Switch` 内部，与其他字段一起在 `a.mu.Lock()` 范围内更新：
```go
a.plugin = pluginOrDefault(plugin)
```

这样切换 agent 后 plugin 也同步替换，不会残留上一个 agent 的 plugin。

**删除方法**：
- `SetConfigOptionsValidator`
- `SetPermissionHandler`
- `validateConfigOptions`（逻辑内联到各调用处）

**`setConfigOptions` 变更**：

```go
func (a *Agent) setConfigOptions(opts []ConfigOption) {
    a.configOptsMu.Lock()
    defer a.configOptsMu.Unlock()
    if err := a.plugin.ValidateConfigOptions(opts); err != nil {
        log.Printf("agent: ignore invalid configOptions update: %v", err)
        return
    }
    a.mu.Lock()
    a.sessionMeta.ConfigOptions = opts
    a.mu.Unlock()
}
```

---

### 3. `acp/session.go` 变更

**session/load 回放订阅者（line 114）**：

```go
case "config_option_update":
    if len(p.Update.ConfigOptions) > 0 {
        if err := a.plugin.ValidateConfigOptions(p.Update.ConfigOptions); err == nil {
            replayMeta.ConfigOptions = p.Update.ConfigOptions
        }
    }
```

**`NormalizeParams` 调用（session/load 回放订阅者）**：在 `json.Unmarshal(n.Params, &p)` 之前：

```go
normalized := a.plugin.NormalizeParams(n.Method, n.Params)
var p SessionUpdateParams
if err := json.Unmarshal(normalized, &p); err != nil || p.SessionID != savedSessionID {
    return
}
```

**session/new 验证（line 164）**：

```go
if err := a.plugin.ValidateConfigOptions(newResult.ConfigOptions); err != nil {
    notifyDone()
    return fmt.Errorf("ensureReady: invalid configOptions: %w", err)
}
```

---

### 4. `acp/prompt.go` 变更

**`NormalizeParams` 调用（Prompt 订阅者）**：在 `json.Unmarshal(n.Params, &p)` 之前：

```go
normalized := a.plugin.NormalizeParams(n.Method, n.Params)
var p SessionUpdateParams
if err := json.Unmarshal(normalized, &p); err != nil {
    return
}
```

---

### 5. `acp/callbacks.go` 变更

`callbackPermission` 中将 `a.permission` 替换为 `a.plugin`：

```go
result, err := a.plugin.HandlePermission(pCtx, p)
```

---

### 6. `internal/agent/factory.go` 变更

```go
type Agent interface {
    Name() string
    Connect(ctx context.Context) (*Conn, error)
    Close() error
    Plugin() acp.AgentPlugin // returns the agent-specific plugin instance
}
```

---

### 7. 各 agent 包变更

#### `internal/agent/claude/plugin.go`（新文件，替代 `config_options_validator.go`）

```go
type claudePlugin struct{ acp.DefaultPlugin }

// ValidateConfigOptions validates only the fields present in the update.
// Partial updates (mode-only or model-only) are explicitly allowed.
// An empty CurrentValue is rejected because Claude requires a non-empty value
// for any mode/model option that is present in the update.
func (claudePlugin) ValidateConfigOptions(opts []acp.ConfigOption) error {
    for _, opt := range opts {
        if (opt.ID == "mode" || opt.Category == "mode") && opt.CurrentValue == "" {
            return fmt.Errorf("claude: mode currentValue is empty")
        }
        if (opt.ID == "model" || opt.Category == "model") && opt.CurrentValue == "" {
            return fmt.Errorf("claude: model currentValue is empty")
        }
    }
    return nil
}
```

`claude_agent.go` 增加：

```go
func (a *ClaudeAgent) Plugin() acp.AgentPlugin { return claudePlugin{} }
```

#### `internal/agent/codex/plugin.go`（新文件，同理）

#### `internal/agent/mock/mock_agent.go` 变更

mock agent 增加 `Plugin()` 方法，返回 `acp.DefaultPlugin{}`：

```go
func (a *MockAgent) Plugin() acp.AgentPlugin { return acp.DefaultPlugin{} }
```

#### 删除文件

- `internal/acp/config_options_validator.go`
- `internal/acp/permission.go`（`PermissionHandler` 和 `AutoAllowHandler` 职责转移到 `DefaultPlugin`）
- `internal/agent/claude/config_options_validator.go`
- `internal/agent/codex/config_options_validator.go`

---

### 8. `internal/client/client.go` 变更

**删除**：
- `configOptionsValidatorProvider` interface
- 所有 `SetConfigOptionsValidator` 调用（共 3 处：`ensureAgent`、`switchAgent` 两分支）
- 所有 `SetPermissionHandler` 调用（如有）

**`ensureAgent` 变更**：

```go
plugin := backend.Plugin()
if savedSID != "" {
    ag = acp.NewWithSessionID(name, conn, c.cwd, savedSID, plugin)
} else {
    ag = acp.New(name, conn, c.cwd, plugin)
}
```

**`switchAgent` 中 `ag.Switch` 调用变更**（传入新 backend 的 plugin）：

```go
if err := ag.Switch(ctx, name, newConn, mode, savedSID, newBackend.Plugin()); err != nil {
    return fmt.Errorf("switch %q: %w", name, err)
}
// 不再需要 SetConfigOptionsValidator
```

**`handlePrompt` 存盘时机修复**：

```go
if u.Type == acp.UpdateConfigOption {
    c.reply(msg.ChatID, formatConfigOptionUpdateMessage(u.Raw))
    c.saveAgentState(ag) // persist immediately on config change
}
```

注意：`saveAgentState` 会在 `config_option_update` 频繁到达时多次写盘（如 session/load 历史回放）。这是可接受的 tradeoff：session/load 发生在 `ensureReady` 阶段而非 `handlePrompt` 阶段，实际上该路径中不会触发此 client 端代码。正常对话中 `config_option_update` 较少，多次写盘不是问题。

---

### 9. 测试文件变更

**`internal/acp/agent_config_options_test.go`**：

`recordingConfigValidator` 改为嵌入 `DefaultPlugin` 并只覆盖 `ValidateConfigOptions`，然后在构造 `Agent` 时直接传入：

```go
type recordingConfigValidator struct {
    DefaultPlugin // embed for HandlePermission and NormalizeParams
    active int32
    max    int32
}

func (v *recordingConfigValidator) ValidateConfigOptions(_ []ConfigOption) error {
    // ... same body as before ...
}

func TestSetConfigOptions_ValidatorCallsSerialized(t *testing.T) {
    v := &recordingConfigValidator{}
    ag := New("test", nil, ".", v) // pass as plugin; no SetConfigOptionsValidator
    // ... rest of test unchanged ...
}
```

**`internal/acp/agent_test.go`**：
- `newACPAgent` 辅助函数中 `acp.New(name, conn, t.TempDir())` 改为 `acp.New(name, conn, t.TempDir(), nil)` 或传入 `acp.DefaultPlugin{}`

**`internal/client/client_test.go`**：

文件中三个本地 agent stub 需各加一个 `Plugin()` 方法：

```go
func (a *minimalMockAgent)      Plugin() acp.AgentPlugin { return acp.DefaultPlugin{} }
func (a *contextRejectMockAgent) Plugin() acp.AgentPlugin { return acp.DefaultPlugin{} }
func (a *failConnectAgent)      Plugin() acp.AgentPlugin { return acp.DefaultPlugin{} }
```

同时 `client_test.go` 中对 `ag.Switch(...)` 的调用（如有）需增加第 6 个 `plugin` 参数。

---

## File Changes Summary

| 操作 | 文件 |
|------|------|
| 新增 | `internal/acp/plugin.go` |
| 新增 | `internal/agent/claude/plugin.go` |
| 新增 | `internal/agent/codex/plugin.go` |
| 修改 | `internal/acp/agent.go` |
| 修改 | `internal/acp/prompt.go` |
| 修改 | `internal/acp/session.go` |
| 修改 | `internal/acp/callbacks.go` | <!-- a.permission → a.plugin -->
| 修改 | `internal/agent/factory.go` |
| 修改 | `internal/agent/claude/claude_agent.go` |
| 修改 | `internal/agent/codex/codex_agent.go` |
| 修改 | `internal/agent/mock/mock_agent.go` |
| 修改 | `internal/client/client.go` |
| 修改 | `internal/acp/agent_test.go` |
| 修改 | `internal/acp/agent_config_options_test.go` |
| 修改 | `internal/client/client_test.go` |
| 删除 | `internal/acp/config_options_validator.go` |
| 删除 | `internal/acp/permission.go` |
| 删除 | `internal/agent/claude/config_options_validator.go` |
| 删除 | `internal/agent/codex/config_options_validator.go` |

---

## Acceptance Criteria

1. `go test ./...` 全部通过，无编译错误
2. `acp.Agent` 不再有 `SetConfigOptionsValidator` / `SetPermissionHandler` 方法
3. `acp.Agent` 不再有 `validator` / `permission` 字段，只有 `plugin AgentPlugin`
4. `internal/client/client.go` 不再有 `configOptionsValidatorProvider` 接口
5. partial config option update（只含 mode 或只含 model）能被正确接受并持久化
6. `UpdateConfigOption` 到达时立即触发存盘，不等 prompt 结束
7. `NormalizeParams` 在 Prompt 订阅者和 session/load 回放订阅者两处都被调用（测试：注入 recording plugin，验证调用次数 > 0）
8. `ag.Switch(...)` 调用后 `ag.plugin` 更新为新 backend 的 plugin（不残留旧 agent 的 plugin）
9. mock agent 实现 `Plugin()` 方法，测试编译通过
