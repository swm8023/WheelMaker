# CodexApp Phase 1 实现计划

> **给 agentic workers：** 必须使用子技能：`superpowers:subagent-driven-development`（推荐）或 `superpowers:executing-plans`，按任务逐项执行本计划。步骤使用 checkbox（`- [ ]`）语法跟踪。

**目标：** 新增可选择的 `codexapp` agent。它在 `Conn` 层伪装成 ACP agent，对上层完全透明，对内连接 `codex app-server --listen stdio://`，完成 Phase 1 文本聊天、session load/list/cancel、approval request 和 config options 同步。

**架构：** 不新增 app-server-native 上层，也不绕过现有 `instance.go`。`codexapp` 实现一个 `agent.Conn`：上层仍走 `NewInstance("codexapp", conn)` 和现有 ACP-shaped `Instance` 方法；`codexappConn` 在 `Send/Notify` 内把 ACP method 转成 app-server request，并把 app-server notification/server request 转回 ACP `session/update` 与 `session/request_permission`。

**Tech Stack:** Go、现有 `agent.Conn`/`ACPProcess`、app-server JSON-RPC over JSONL stdio、`server/internal/hub/agent` 单包测试。

**当前实现基线：** Phase 1 已落地的是基础文本聊天、session new/load/list、config options、cancel、按 thread routing、approval request skeleton。`resource_link` 输入、`session/load` history replay、tool/plan streaming 仍是后续任务；本文中涉及这些能力的条目应按 follow-up 解读，不能声明已完成。

---

## 已确认决策

- Agent/provider 名称全链路无横杠：`codexapp`。
- 用户侧使用 `/new codexapp`。
- `protocol.ACPProviderCodexApp` 的值是 `"codexapp"`。
- 代码文件统一使用 `codexapp_xx.go` 命名。
- 测试不新增多个 test 文件，全部合并到 `server/internal/hub/agent/agent_test.go`。
- 修改聚焦 agent 层；不改 `server/internal/hub/client`、IM、registry 协议或 recorder schema。
- 保留现有 `instance.go`。`codexapp` 通过 `Conn` 接口伪装 ACP，而不是实现一个特殊 `Instance`。
- 不直接复用现有 `ownedConn` 实现，因为它假设对端是真 ACP wire；但保留 owned connection 语义，并实现 `codexappConn`。
- 可复用 `ACPProcess`，因为它只是 JSONL stdio subprocess transport。
- Phase 1 可以每个 `codexappConn` 一个 app-server process，但结构必须先适配未来多 session shared app-server。
- shared-ready 设计要求 runtime/conn 分层：runtime 共享 transport 和 model list cache，conn 保存 session-local state。
- Phase 1 不支持图片、audio、embedded resource、非空 MCP servers；必须不声明或明确拒绝。

## 文件分工

只新增两个实现文件：

- `server/internal/hub/agent/codexapp_agent.go`
  - `codexapp` provider launch
  - factory creator
  - `codexappConn` 实现 `agent.Conn`
  - `codexappRuntime` 与 lightweight app-server JSON-RPC matching
  - process lifecycle、routing、config state、prompt wait、approval wait

- `server/internal/hub/agent/codexapp_convert.go`
  - app-server 最小私有 structs
  - ACP in -> app-server in 转换
  - app-server out -> ACP out 转换
  - config options、approval preset、sandbox policy、tool lifecycle、plan、stopReason 映射

只修改这些现有文件：

- `server/internal/protocol/acp_const.go`
  - 加 `ACPProviderCodexApp = "codexapp"`
  - 加 config 常量：`approval_preset`、`reasoning_effort`、`_approval_preset`

- `server/internal/hub/agent/factory.go`
  - 注册 `codexapp` native creator
  - creator 返回 `NewInstance("codexapp", codexappConn)`
  - 不走 `providerInstanceCreator`

- `server/internal/hub/agent/agent_test.go`
  - 合并所有 `codexapp` tests

不新增：

- `appserver_client.go`
- `appserver_types.go`
- `appserver_config.go`
- `appserver_instance.go`
- `appserver_*_test.go`
- `codexapp_*_test.go`

## 核心结构

```text
Session
  -> existing agent.Instance (instance.go)
    -> codexappConn                  // implements agent.Conn, per WheelMaker session
      -> codexappRuntime             // app-server runtime, owned now, shared-ready
        -> ACPProcess                // JSONL stdio process transport
          -> codex app-server
```

`codexappConn` 是 per session：

- 实现 `Conn`：`Send`、`Notify`、`OnACPRequest`、`OnACPResponse`、`Close`、`Alive`。
- 保存当前 `threadId`，即 ACP `sessionId`。
- 保存 session-local config：`approval_preset`、`model`、`reasoning_effort`。
- 保存 session-local turn state：`activeTurnId`、prompt completion channel、pending approvals。
- 只处理属于自己 `threadId` 的 app-server events。
- 对上只暴露 ACP-shaped 行为。

`codexappRuntime` 是 shared-ready：

- 拥有 app-server process 或共享 process 引用。
- 负责 app-server JSON-RPC request/response matching。
- 负责读取所有 app-server notifications/server requests。
- 通过 `threadId -> codexappConn` 路由事件。
- 缓存 runtime-level `model/list`，但不保存 selected model。
- 提供：`register(threadId, conn)`、`unregister(threadId, conn)`、`request(ctx, method, params, out)`、`notify(method, params)`、`close()`。

Phase 1 可以先这样创建：

```go
func newCodexappConn(provider *codexappProvider, cwd string) (*codexappConn, error) {
    rt, err := newOwnedCodexappRuntime(provider, cwd)
    if err != nil {
        return nil, err
    }
    return newCodexappConnWithRuntime(rt, cwd), nil
}
```

未来 shared mode 只替换 runtime provider：

```go
func newCodexappConn(provider *codexappProvider, cwd string) (*codexappConn, error) {
    rt := projectCodexappRuntimePool.Get(projectID, cwd)
    return newCodexappConnWithRuntime(rt, cwd), nil
}
```

`codexappConn` 不应该知道 runtime 是 owned 还是 shared。

## stdio 多 Thread 与 Config Options 设计

`stdio://` 是一条 JSONL JSON-RPC stream，不等于只能一个 thread。多 thread 通过两层 id 区分：

- JSON-RPC `id` 区分 request/response。
- payload 中的 `threadId` / `turnId` 区分会话事件。

一条 stdio stream 上可以出现：

```text
thread/start -> thr_1
thread/start -> thr_2
turn/start {threadId: thr_1}
turn/start {threadId: thr_2}
item/agentMessage/delta {threadId: thr_1, turnId: ...}
item/agentMessage/delta {threadId: thr_2, turnId: ...}
```

因此设计必须从 Phase 1 就支持 runtime 按 `threadId` 分发：

```go
type codexappRuntime struct {
    connsByThread map[string]*codexappConn
    pendingByReqID map[string]chan codexappRPCResponse
}
```

`config options` 不属于 stdio connection 级别，而属于 `codexappConn` / thread 级别：

- `model/list` 是 runtime-level cache。
- selected `model` 是 conn-local。
- selected `reasoning_effort` 是 conn-local。
- selected `approval_preset` 是 conn-local。
- app-server `thread/start` / `thread/resume` response 返回的 effective config 只能更新当前 conn，不能影响其他 conn。

每次发 app-server request 时，由 conn 注入自己的 config：

`session/new -> thread/start`：

```json
{
  "method": "thread/start",
  "params": {
    "cwd": "...",
    "model": "gpt-5",
    "approvalPolicy": "on-request",
    "sandbox": "workspace-write",
    "serviceName": "wheelmaker"
  }
}
```

`session/load -> thread/resume`：

```json
{
  "method": "thread/resume",
  "params": {
    "threadId": "thr_1",
    "cwd": "...",
    "model": "gpt-5",
    "approvalPolicy": "on-request",
    "sandbox": "workspace-write"
  }
}
```

`session/prompt -> turn/start`：

```json
{
  "method": "turn/start",
  "params": {
    "threadId": "thr_1",
    "input": [],
    "model": "gpt-5",
    "effort": "high",
    "approvalPolicy": "on-request",
    "sandboxPolicy": {
      "type": "workspaceWrite",
      "writableRoots": ["..."],
      "networkAccess": false
    }
  }
}
```

同一个 conn/thread Phase 1 只允许一个 active turn。不同 thread 的并发 turn 可以先通过 runtime 全局 mutex 串行化 `turn/start`，但事件路由仍必须按多 thread 设计。

## ACP Method 映射

`codexappConn.Send` 根据 ACP method 分发：

| ACP method | app-server mapping | conn 行为 |
|---|---|---|
| `initialize` | `initialize` + `initialized` | 返回 ACP `InitializeResult`，声明 conservative capabilities |
| `session/new` | `thread/start` | 获取 `thread.id` 后注册 `threadId -> conn`，返回完整 config options |
| `session/load` | `thread/resume` | 当前仅 resume 并返回 config options；`thread/read includeTurns=true` 与 history replay 是 follow-up |
| `session/list` | `thread/list` | 返回 ACP `SessionListResult` |
| `session/prompt` | `turn/start` | 发送 synthetic user chunk，等待 matching `turn/completed`，返回 stopReason |
| `session/set_config_option` | 本地 conn state | 更新 selected config，返回完整 config options |

`codexappConn.Notify` 支持：

| ACP notification | app-server mapping |
|---|---|
| `session/cancel` | `turn/interrupt`，没有 active turn 时 no-op |

`codexappConn.OnACPResponse` 用于向 existing `instance.go` 上报 ACP notification。当前 Phase 1 基线只实现：

- app-server assistant delta -> ACP `session/update` `agent_message_chunk`
- app-server reasoning delta -> ACP `agent_thought_chunk`
- thread name update -> ACP `session_info_update`

后续任务：

- app-server plan -> ACP `plan`
- app-server tool-like item -> ACP `tool_call` / `tool_call_update`
- app-server config changes -> ACP `config_option_update`

`codexappConn.OnACPRequest` 用于借 existing `instance.go` 的 callback path 发起 ACP client request：

- app-server command/file approval -> ACP `session/request_permission`

## Config Options

Phase 1 只暴露三个 options：

| id | category | source | app-server effect |
|---|---|---|---|
| `approval_preset` | `_approval_preset` | conn-local preset | 展开为 `approvalPolicy` + `sandbox` / `sandboxPolicy` |
| `model` | `model` | runtime `model/list` | `thread/start.model`、`thread/resume.model`、`turn/start.model` |
| `reasoning_effort` | `thought_level` | selected model 的 effort list | `turn/start.effort` |

Config list 必须在这些边界返回完整列表：

- `session/new`
- `session/load`
- `session/set_config_option`
- `config_option_update`

切换 `model` 时，conn 必须重新计算 `reasoning_effort.options`。如果旧 effort 不被新 model 支持，选择该 model 默认 effort。

Approval preset 映射：

| preset | approvalPolicy | thread sandbox | turn sandboxPolicy.type |
|---|---|---|---|
| `read_only` | `on-request` | `read-only` | `readOnly` |
| `ask` | `on-request` | `workspace-write` | `workspaceWrite` |
| `auto` | `on-failure` | `workspace-write` | `workspaceWrite` |
| `full` | `never` | `danger-full-access` | `dangerFullAccess` |

Schema 注意：`thread/start` 和 `thread/resume` 使用 `sandbox` 字符串；`turn/start` 使用 `sandboxPolicy` 对象。

## Task 1：重命名 Provider 与常量

**文件：** `server/internal/protocol/acp_const.go`、`server/internal/hub/agent/codexapp_agent.go`、`server/internal/hub/agent/agent_test.go`

- [ ] 把 provider value 改成 `codexapp`，没有横杠。
- [ ] `ParseACPProvider("codexapp")` 返回 `ACPProviderCodexApp`。
- [ ] `ACPProviderNames()` 包含 `codexapp`。
- [ ] 添加 config constants：`ConfigOptionIDApprovalPreset`、`ConfigOptionIDReasoningEffort`、`ConfigOptionCategoryApprovalPreset`。
- [ ] 新增 provider launch helper，解析 `codex`，返回 args `app-server --listen stdio://`。
- [ ] 测试合并到 `agent_test.go`：provider parse、provider names、launch args。
- [ ] 运行：`cd server; go test ./internal/protocol ./internal/hub/agent -run "Test.*CodexApp.*Provider|TestParseACPProviderCodexApp" -count=1`。

## Task 2：实现 shared-ready Runtime 和 Conn 骨架

**文件：** `server/internal/hub/agent/codexapp_agent.go`、`server/internal/hub/agent/agent_test.go`

- [ ] 定义 `codexappConn`，实现 `agent.Conn`。
- [ ] 定义 `codexappRuntime`，支持 request id matching、notification dispatch、server request dispatch、`threadId -> conn` registry。
- [ ] runtime outbound request/notification 不包含 `jsonrpc`。
- [ ] runtime inbound request id 支持 string 或 number。
- [ ] notification/server request 必须离开 `ACPProcess` read loop 异步分发，避免 handler re-entry deadlock。
- [ ] `Close` 关闭 runtime 并 fail pending requests。
- [ ] `Alive` 反映 process/runtime 状态。
- [ ] 测试合并到 `agent_test.go`：wire shape、request matching、notification thread routing、unknown server request `-32601`、close pending、alive。
- [ ] 运行：`cd server; go test ./internal/hub/agent -run "TestCodexApp.*Runtime|TestCodexApp.*Conn" -count=1`。

## Task 3：ACP Initialize/New/Load/List/Config 基线

**文件：** `server/internal/hub/agent/codexapp_agent.go`、`server/internal/hub/agent/codexapp_convert.go`、`server/internal/hub/agent/factory.go`、`server/internal/hub/agent/agent_test.go`

- [ ] factory 注册 `codexapp` native creator：`NewInstance("codexapp", conn)`。
- [ ] `initialize` 映射 app-server `initialize` + `initialized`，返回 ACP capabilities。
- [ ] `session/new` 调用 `thread/start`，拿到 `thread.id` 后 runtime `register(threadId, conn)`。
- [ ] `session/load` 先 runtime `register(sessionId, conn)`，再 `thread/resume` 后返回；`thread/read includeTurns=true` 与 replay 是 follow-up。
- [ ] `session/list` 调用 `thread/list`。
- [ ] `session/set_config_option` 修改 conn-local config，返回完整 options。
- [ ] 非空 `mcpServers` 返回 invalid params 风格错误，不能静默忽略。
- [ ] 测试：initialize order、new returns thread id、load/resume mapping、list mapping、config full list、MCP reject、factory creator。History replay before result 是 follow-up 测试。
- [ ] 运行：`cd server; go test ./internal/hub/agent -run "TestCodexApp.*Initialize|TestCodexApp.*SessionNew|TestCodexApp.*SessionLoad|TestCodexApp.*Config|TestCodexApp.*Factory" -count=1`。

## Task 4：Prompt、Streaming、Cancel

**文件：** `server/internal/hub/agent/codexapp_agent.go`、`server/internal/hub/agent/codexapp_convert.go`、`server/internal/hub/agent/agent_test.go`

- [ ] `session/prompt` 支持 ACP text；本地文件 `resource_link` 是 follow-up。
- [ ] Phase 1 拒绝 image/audio/embedded resource。
- [ ] prompt 开始时发 synthetic `user_message_chunk`。
- [ ] `turn/start` 注入 conn-local `model`、`effort`、`approvalPolicy`、`sandboxPolicy`。
- [ ] `turn/started` 设置 conn-local `activeTurnId`。
- [ ] `turn/completed` 完成 matching prompt，返回 ACP `stopReason`。
- [ ] `session/cancel` 发送 `turn/interrupt`，普通取消返回 `cancelled`。
- [ ] 同一个 conn/thread 不允许并发 active prompt。
- [ ] 测试：text prompt、unsupported content、assistant delta、completed/interrupted、cancel no-op、cancel active turn。`resource_link`、plan、tool lifecycle 是 follow-up 测试。
- [ ] 运行：`cd server; go test ./internal/hub/agent -run "TestCodexApp.*Prompt|TestCodexApp.*Cancel|TestCodexApp.*Tool|TestCodexApp.*Plan" -count=1`。

## Task 5：Approval 与 Server Requests

**文件：** `server/internal/hub/agent/codexapp_agent.go`、`server/internal/hub/agent/codexapp_convert.go`、`server/internal/hub/agent/agent_test.go`

- [ ] app-server `item/commandExecution/requestApproval` 转 ACP `session/request_permission`。
- [ ] app-server `item/fileChange/requestApproval` 转 ACP `session/request_permission`。
- [ ] ACP selected `allow_once` -> app-server `accept`。
- [ ] ACP selected `allow_always` -> app-server `acceptForSession`。
- [ ] ACP reject -> app-server `decline`。
- [ ] ACP cancelled/error -> app-server `cancel`。
- [ ] unsupported server request 返回 `-32601` 或 fail closed。
- [ ] `mcpServer/elicitation/request` 返回 cancelled response，作为 follow-up server request 支持。
- [ ] cancel/process close 时 pending approvals 必须 fail closed，作为 approval skeleton 的后续加固。
- [ ] 测试：command approval、file approval、reject、cancel、callback error、unsupported request、unknown thread route fail closed。
- [ ] 运行：`cd server; go test ./internal/hub/agent -run "TestCodexApp.*Approval|TestCodexApp.*ServerRequest" -count=1`。

## Task 6：Docs 与验证

**文件：** docs 与 agent 层文件

- [ ] 把 spec/bridge 文档同步为 `codexapp`、Conn 层 proxy、runtime/conn shared-ready 结构。
- [ ] 确认没有残留 `codex-app` 作为 agent id，除非在历史背景说明中明确表示旧称。
- [ ] 确认没有新增 `appserver_*` 实现文件或 test 文件。
- [ ] 运行：`cd server; go test ./internal/protocol ./internal/hub/agent -count=1`。
- [ ] 运行：`cd server; go test ./...`。
- [ ] 构建：`cd server; go build -o bin/windows_amd64/wheelmaker.exe ./cmd/wheelmaker/`。

## 验收清单

- [ ] `/new codexapp` 可创建新 thread，并把 thread id 作为 ACP session id。
- [ ] 上层仍只通过 existing `instance.go` 和 `Conn` 接口工作。
- [ ] `codexappConn` 对上完全 ACP-shaped。
- [ ] `codexappRuntime` 从 Phase 1 起按 `threadId` 路由，适配未来 shared app-server。
- [ ] session-local config 不进入 runtime 全局状态。
- [ ] model list cache 可以 runtime-level 共享。
- [ ] `session/load` 当前完成 resume；history replay 顺序符合 ACP 是 follow-up。
- [ ] `session/prompt` update draining 与 stopReason 符合 ACP。
- [ ] approval skeleton 与 cancel 不悬挂；tool lifecycle streaming 与 approval fail-closed 加固是 follow-up。
- [ ] 不修改 `client/session/commands` 等上层 UI/业务文件。
