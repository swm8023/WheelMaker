# Agent Plugin Architecture & Config Option Fix

**Date**: 2026-03-17
**Status**: Approved

---

## Problem Statement

当前代码存在三个关联问题：

1. **Validator 逻辑 bug**：`claudeConfigOptionsValidator` / `codexConfigOptionsValidator` 要求 config option update 中同时包含 `mode` 和 `model`。但 `session/update` 的 `config_option_update` 和 `SetConfigOption` 的响应都可能是 partial update（只包含变化的那一项），导致合法更新被静默丢弃。

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
- 删除所有 setter 和 duck-typed 辅助接口。

---

### 1. `acp/plugin.go`（新文件）

```go
// AgentPlugin is the per-agent customization interface for acp.Agent.
// Implementations embed DefaultPlugin and override only the methods they need.
type AgentPlugin interface {
    // ValidateConfigOptions validates options from a config_option_update notification
    // or SetConfigOption response. Partial updates are allowed — only validate fields
    // that are present. Return non-nil to reject and log; the update is silently dropped.
    ValidateConfigOptions(opts []ConfigOption) error

    // HandlePermission decides how to respond to an incoming permission request.
    HandlePermission(ctx context.Context, req PermissionRequest) PermissionResponse

    // NormalizeParams is called before acp processes each incoming session/update
    // notification. Implementations may translate legacy protocol fields to modern
    // format. Return params unchanged for pass-through (current behavior).
    NormalizeParams(method string, params json.RawMessage) json.RawMessage
}

// DefaultPlugin is the zero-value implementation of AgentPlugin.
// All methods are no-ops or auto-allow. Embed this in agent-specific plugins
// so new extension points don't require updating all implementations.
type DefaultPlugin struct{}

func (DefaultPlugin) ValidateConfigOptions(_ []ConfigOption) error { return nil }
func (DefaultPlugin) HandlePermission(_ context.Context, req PermissionRequest) PermissionResponse {
    return autoAllowResponse(req)
}
func (DefaultPlugin) NormalizeParams(_ string, params json.RawMessage) json.RawMessage { return params }
```

`autoAllowResponse` 提取自现有的 `AutoAllowHandler` 逻辑（内部函数，不导出）。

---

### 2. `acp/agent.go` 变更

**删除**：
- 字段 `validator ConfigOptionsValidator`
- 字段 `permission PermissionHandler`（合并到 plugin）
- 方法 `SetConfigOptionsValidator`
- 方法 `SetPermissionHandler`
- 方法 `validateConfigOptions`（内联到 `setConfigOptions`）

**新增**：
- 字段 `plugin AgentPlugin`

**`New` / `NewWithSessionID` 签名变更**：

```go
func New(name string, conn *agent.Conn, cwd string, plugin AgentPlugin) *Agent
func NewWithSessionID(name string, conn *agent.Conn, cwd string, sessionID string, plugin AgentPlugin) *Agent
```

`plugin` 为 nil 时自动使用 `DefaultPlugin{}`。

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

**`NormalizeParams` 调用点**：在 `prompt.go` 的 `conn.Subscribe` 回调中，parse `SessionUpdateParams` 之前先调用：

```go
normalizedParams := a.plugin.NormalizeParams(n.Method, n.Params)
// 后续用 normalizedParams 替代 n.Params
```

**Permission 回调**：`permission.go` / `callbacks.go` 中对 `a.permission.Handle(...)` 的调用改为 `a.plugin.HandlePermission(...)`。

---

### 3. `internal/agent/factory.go` 变更

```go
type Agent interface {
    Name() string
    Connect(ctx context.Context) (*Conn, error)
    Close() error
    Plugin() acp.AgentPlugin // returns the agent-specific plugin instance
}
```

---

### 4. 各 agent 包变更

#### `internal/agent/claude/plugin.go`（新文件，替代 config_options_validator.go）

```go
type claudePlugin struct{ acp.DefaultPlugin }

func (claudePlugin) ValidateConfigOptions(opts []acp.ConfigOption) error {
    // Only validate fields that are present; partial updates are allowed.
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

#### 删除文件

- `internal/acp/config_options_validator.go`
- `internal/agent/claude/config_options_validator.go`
- `internal/agent/codex/config_options_validator.go`

---

### 5. `internal/client/client.go` 变更

**删除**：
- `configOptionsValidatorProvider` interface
- `SetConfigOptionsValidator` 调用（共 3 处：`ensureAgent`、`switchAgent` 两分支）

**`ensureAgent` 变更**：

```go
plugin := backend.Plugin()
if savedSID != "" {
    ag = acp.NewWithSessionID(name, conn, c.cwd, savedSID, plugin)
} else {
    ag = acp.New(name, conn, c.cwd, plugin)
}
// 不再需要 SetConfigOptionsValidator
```

**`switchAgent` 变更**（两处构造 acp.Agent 的地方同理）。

**`handlePrompt` 存盘时机修复**：

```go
if u.Type == acp.UpdateConfigOption {
    c.reply(msg.ChatID, formatConfigOptionUpdateMessage(u.Raw))
    c.saveAgentState(ag) // persist immediately on config change
}
```

---

### 6. `internal/agent/mock/mock_agent.go` 变更

mock agent 需要实现 `Plugin()` 方法，返回 `acp.DefaultPlugin{}`（或测试专用 plugin）。

---

## File Changes Summary

| 操作 | 文件 |
|------|------|
| 新增 | `internal/acp/plugin.go` |
| 新增 | `internal/agent/claude/plugin.go` |
| 新增 | `internal/agent/codex/plugin.go` |
| 修改 | `internal/acp/agent.go` |
| 修改 | `internal/acp/prompt.go` |
| 修改 | `internal/acp/permission.go` / `callbacks.go` |
| 修改 | `internal/agent/factory.go` |
| 修改 | `internal/agent/claude/claude_agent.go` |
| 修改 | `internal/agent/codex/codex_agent.go` |
| 修改 | `internal/agent/mock/mock_agent.go` |
| 修改 | `internal/client/client.go` |
| 删除 | `internal/acp/config_options_validator.go` |
| 删除 | `internal/agent/claude/config_options_validator.go` |
| 删除 | `internal/agent/codex/config_options_validator.go` |

---

## Acceptance Criteria

1. `go test ./...` 全部通过
2. `acp.Agent` 不再有 `SetConfigOptionsValidator` / `SetPermissionHandler` 方法
3. `internal/client/client.go` 不再有 `configOptionsValidatorProvider` 接口
4. partial config option update（只含 mode 或只含 model）能被正确接受并持久化
5. `UpdateConfigOption` 到达时立即触发存盘，不等 prompt 结束
6. `NormalizeParams` 在 session/update 处理链中被调用（当前为直通，但调用点存在）
7. mock agent 实现 `Plugin()` 方法，测试编译通过
