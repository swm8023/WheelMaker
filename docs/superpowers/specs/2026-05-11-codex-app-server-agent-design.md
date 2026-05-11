# CodexApp Agent 设计

日期：2026-05-11
状态：待评审草案

## 背景

WheelMaker 当前通过 `codex-acp` 与 Codex 通信，这条路径不稳定。Codex 提供 `codex app-server`，它是 Codex 富客户端使用的 JSON-RPC 接口，原生支持 thread、turn、streaming item events、approval、model list、config 与 Codex 自有持久化。

WheelMaker 上层已经依赖 ACP-shaped `agent.Instance`。新的实现不应改 `Session`、IM、registry 或 recorder，而应在 agent 层新增 `codexapp`，把 app-server 伪装成 ACP agent。

## 已确认决策

- Agent/provider id 全链路为 `codexapp`，无横杠。
- 用户侧使用 `/new codexapp`。
- `protocol.ACPProviderCodexApp` 的值是 `"codexapp"`。
- 文件命名使用 `codexapp_xx.go`。
- 测试合并到 `server/internal/hub/agent/agent_test.go`。
- 修改聚焦 agent 层；除 provider 常量和 factory 注册外，不修改上层业务/UI 文件。
- 不绕过现有 `instance.go`；实现一个 `codexappConn`，让 existing `NewInstance("codexapp", conn)` 继续工作。
- 不直接复用现有 `ownedConn` 实现，因为它假设对端说 ACP wire；但保留 owned connection 语义。
- 复用 `ACPProcess` 作为 JSONL stdio subprocess transport。
- Phase 1 可以 owned runtime（一会话一 app-server process），但结构必须先适配未来 shared runtime（一 app-server process 多 thread）。

## 目标

- `codexapp` 可被选择并懒启动 Codex app-server。
- 上层仍只看到 ACP-shaped calls、updates 和 permission requests。
- `ACP sessionId == app-server threadId == WheelMaker SessionRecord.ID`。
- 支持基础文本聊天、session load/list/cancel、approval request skeleton、config options 同步。
- Config options 包括 `approval_preset`、`model`、`reasoning_effort`。
- Phase 1 不声明图片、audio、embedded context；非空 MCP servers 明确拒绝。
- 协议转换和 runtime routing 可单测，不依赖真实 Codex process。
- `resource_link` 输入、history replay、tool/plan streaming 是后续任务，当前实现不能声明这些能力已完成。

## 非目标

- 修改 WheelMaker 上层协议。
- 暴露 app-server native UI/API。
- 改 recorder schema。
- Phase 1 支持图片、audio、embedded resources 或 MCP materialization。
- Phase 1 实现 project-level shared runtime pool；只要求结构 shared-ready。

## 架构

```text
Session
  -> existing agent.Instance (instance.go)
    -> codexappConn                  // implements agent.Conn, per WheelMaker session
      -> codexappRuntime             // app-server runtime, owned now, shared-ready
        -> ACPProcess                // JSONL stdio process transport
          -> codex app-server
```

### `codexappConn`

`codexappConn` 是每个 WheelMaker session 一个的 ACP-shaped connection：

- 实现 `agent.Conn`。
- 对外接收 ACP method：`initialize`、`session/new`、`session/load`、`session/list`、`session/prompt`、`session/set_config_option`、`session/cancel`。
- 对内调用 app-server method：`initialize`、`initialized`、`thread/start`、`thread/resume`、`thread/read`、`thread/list`、`turn/start`、`turn/interrupt`。
- 保存 session-local state：`threadId`、selected `model`、selected `reasoning_effort`、selected `approval_preset`、`activeTurnId`、pending prompt、pending approvals。
- 只处理属于自己 `threadId` 的 app-server events。
- 通过 `OnACPResponse` 向 existing `instance.go` 发 ACP `session/update`。
- 通过 `OnACPRequest` 借 existing `instance.go` 发 ACP `session/request_permission`。

### `codexappRuntime`

`codexappRuntime` 是 app-server runtime 抽象，Phase 1 可以 owned，未来可以 shared：

- 拥有或引用 app-server process。
- 管理 app-server JSON-RPC request/response matching。
- 读取所有 app-server notifications/server requests。
- 用 `threadId -> codexappConn` 路由 events 和 approval requests。
- 提供 `register(threadId, conn)`、`unregister(threadId, conn)`、`request(ctx, method, params, out)`、`notify(method, params)`、`close()`。
- 可以缓存 runtime-level `model/list`。
- 不保存 selected model/reasoning/approval；这些属于 conn-local state。

### owned 与 shared 的关系

Phase 1 owned runtime：

```go
conn -> owned runtime -> one app-server process
```

未来 shared runtime：

```go
conn A -> shared runtime -> one app-server process
conn B -> shared runtime -> same app-server process
```

`codexappConn` 不应该知道 runtime 是 owned 还是 shared。

## stdio 多 Thread

`stdio://` 是一条 JSONL JSON-RPC stream，不限制一个 thread。多 thread 通过 payload 中的 `threadId` / `turnId` 区分：

```text
thread/start -> thr_1
thread/start -> thr_2
turn/start {threadId: thr_1}
turn/start {threadId: thr_2}
item/agentMessage/delta {threadId: thr_1, turnId: ...}
item/agentMessage/delta {threadId: thr_2, turnId: ...}
```

因此 runtime 从第一天就必须按 `threadId` 路由。即使 Phase 1 每个 runtime 只有一个 conn，也不能把 event handling 写死成单 session。

## Config Options

`model/list` 可以 runtime-level cache。用户选择的 `model`、`reasoning_effort`、`approval_preset` 必须 conn-local。

每次发 app-server request 时，由 conn 把自己的 config 注入：

- `thread/start`：`model`、`approvalPolicy`、`sandbox`
- `thread/resume`：`model`、`approvalPolicy`、`sandbox`
- `turn/start`：`model`、`effort`、`approvalPolicy`、`sandboxPolicy`

`thread/start` / `thread/resume` response 返回的 effective config 只能更新当前 conn 的 config options，不能影响其他 conn。

Phase 1 options：

| id | category | scope | app-server effect |
|---|---|---|---|
| `approval_preset` | `_approval_preset` | conn-local | `approvalPolicy` + `sandbox` / `sandboxPolicy` |
| `model` | `model` | conn-local value, runtime-level option source | `model` |
| `reasoning_effort` | `thought_level` | conn-local | `effort` |

Approval preset：

| preset | approvalPolicy | thread sandbox | turn sandboxPolicy.type |
|---|---|---|---|
| `read_only` | `on-request` | `read-only` | `readOnly` |
| `ask` | `on-request` | `workspace-write` | `workspaceWrite` |
| `auto` | `on-failure` | `workspace-write` | `workspaceWrite` |
| `full` | `never` | `danger-full-access` | `dangerFullAccess` |

## 文件结构

只新增：

- `server/internal/hub/agent/codexapp_agent.go`
- `server/internal/hub/agent/codexapp_convert.go`

只修改：

- `server/internal/protocol/acp_const.go`
- `server/internal/hub/agent/factory.go`
- `server/internal/hub/agent/agent_test.go`

不新增多个 `appserver_*` 文件，也不新增多个 test 文件。

## Phase 1 行为

- `initialize` 映射 app-server `initialize` + `initialized`，返回 conservative ACP capabilities。
- `session/new` 映射 `thread/start`，成功后 runtime `register(threadId, conn)`。
- `session/load` 先 register，再 `thread/resume` 后返回 config options；`thread/read includeTurns=true` 与 replay history 是 follow-up。
- `session/list` 映射 `thread/list`。
- `session/prompt` 映射 `turn/start`，注入 conn-local config，等待 matching `turn/completed`。
- `session/cancel` 映射 `turn/interrupt`，没有 active turn 时 no-op。
- app-server command/file approval request 映射 ACP `session/request_permission`。
- tool lifecycle streaming 是 follow-up，后续必须保持 `pending -> in_progress -> completed/failed`。
- `plan` streaming 是 follow-up，后续必须发送 full-plan replacement。
- `config_option_update` 如后续实现，必须发送 full-list replacement。

## 测试要求

所有 `codexapp` tests 合并进 `server/internal/hub/agent/agent_test.go`。

必须覆盖：

- provider parse/name/launch args
- factory creator 返回 existing `NewInstance("codexapp", conn)` 路径
- runtime request/response matching
- string/number request id
- notification 按 thread routing
- server request 按 thread routing
- initialize/new/load/list/config
- text prompt streaming/cancel
- approval mapping/fail closed
- non-empty MCP reject
- unsupported prompt content reject
- shared-ready：两个 conn 注册到同一 fake runtime 时，events 不串线
- follow-up：history replay、`resource_link`、tool lifecycle streaming、plan streaming
