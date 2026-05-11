# CodexApp Server 与 ACP 协议转换文档

日期：2026-05-11
状态：Draft for review

本文描述新的 `codexapp` agent 如何在 WheelMaker 内部把 ACP 语义转换为 Codex app-server 协议，并把 app-server 输出转换回 ACP 输出。

## 术语

| 术语 | 含义 |
|---|---|
| ACP in | WheelMaker `Session` 调用 `agent.Instance` 的 ACP-shaped 方法 |
| ACP out | `agent.Instance` 回调 `SessionUpdate`、`SessionRequestPermission`，或返回 ACP 方法结果 |
| app-server in | `codex app-server` 接收的 JSON-RPC request/notification |
| app-server out | `codex app-server` 发出的 JSON-RPC response/notification/request |
| ACP sessionId | WheelMaker 现有 session id 字段 |
| app-server threadId | Codex app-server Thread id |
| app-server turnId | 一次用户请求对应的 Turn id |

`codexapp` 的核心规则：`ACP sessionId == app-server threadId == WheelMaker SessionRecord.ID`。

## 分阶段范围

### Phase 1：基础聊天与 Config Options 同步

Phase 1 的目标是让 `codexapp` 成为可日常试用的文本聊天 agent，而不是只完成 transport demo。当前实现保证 WheelMaker 上层看到的是基础文本聊天所需的 ACP-shaped 行为；完整 ACP 覆盖仍依赖下列 follow-up。

Phase 1 基线必须支持：

- `initialize`
- `session/new`
- `session/load`
- `session/list`
- `session/prompt`
- `session/cancel`
- `session/set_config_option`
- 文本输入 `ContentBlock{type:"text"}`
- app-server assistant delta -> ACP `agent_message_chunk`
- `approval_preset`、`model`、`reasoning_effort` 三个 config options 的初始化、修改、同步和偏好回放

Phase 1 后续补齐项：

- `session/load` 的 history replay
- app-server plan -> ACP `plan`
- app-server command/file/tool item -> ACP `tool_call` / `tool_call_update`
- app-server command/file approval 的 fail-closed、cancel/process close pending cleanup 等加固；当前仅实现 approval request skeleton

Phase 1 不支持但必须明确拒绝或不声明：

- `promptCapabilities.image=false`
- `promptCapabilities.audio=false`
- `promptCapabilities.embeddedContext=false`
- 非空 `mcpServers` 返回 ACP `Invalid params`，不能静默忽略
- `resource_link`、`resource`、`audio`、`image` 等非文本 prompt content 返回 ACP 错误
- base64 图片不处理
- HTTP MCP 不声明

Phase 1 完成标准：

- 新建 `codexapp` 会话后可以完成多轮文本聊天。
- `session/load` 能 resume 已知 thread 并返回 config options；history replay 是 follow-up。
- `/model`、`/config` 或 registry `session.setConfig` 修改后，adapter 返回完整 config option 列表，并在下一次 `turn/start` 生效。
- 切换 `model` 时同步更新 `reasoning_effort` 可选值；旧 effort 不再支持时自动落到该模型默认值。
- `approval_preset` 能展开到 app-server `approvalPolicy` + `sandbox`。
- 基础 approval request skeleton 和取消流程不悬挂。
- 工具事件 streaming 与 ACP `pending -> in_progress -> completed/failed` 生命周期是 follow-up。

### Phase 1.5：URI 图片

Phase 1.5 才开启 `promptCapabilities.image=true`。支持范围：

- ACP `image.uri=file:///absolute/path` -> app-server `{type:"localImage", path}`
- ACP `image.uri=http(s)://...` -> app-server `{type:"image", url}`

不支持 `data` base64。收到 base64-only image 时返回 ACP `Invalid params`。

### Phase 2：Base64 图片与 MCP Materialization

Phase 2 支持：

- ACP `image.data` base64 -> 校验 MIME/大小 -> 写入 WheelMaker temp file -> app-server `{type:"localImage", path}`
- 非空 stdio `mcpServers` -> materialize 到本次 `codexapp` 实例可见的 Codex config -> 调用 app-server MCP reload
- HTTP MCP 仅在实现并声明 `agentCapabilities.mcp.http=true` 后接受

## 实现框架

`codexapp` 不改 WheelMaker 上层协议，也不新增特殊上层 `Instance`。它在 `Conn` 层伪装成 ACP transport，继续复用现有 `instance.go`：

```text
Session
  -> existing agent.Instance (instance.go)
    -> codexappConn                  implements agent.Conn, per WheelMaker session
      -> codexappRuntime             app-server runtime, owned now, shared-ready
        -> ACPProcess                JSONL stdio process transport
          -> codex app-server
```

文件边界：

| 文件 | 职责 |
|---|---|
| `server/internal/hub/agent/codexapp_agent.go` | provider launch、factory creator、`codexappConn`、`codexappRuntime`、lifecycle |
| `server/internal/hub/agent/codexapp_convert.go` | app-server 最小类型、ACP/app-server 转换、config/preset 映射 |
| `server/internal/hub/agent/agent_test.go` | 所有 `codexapp` 单元测试，保持 agent 包一个 test 文件 |
| `server/internal/protocol/acp_const.go` | `ACPProviderCodexApp = "codexapp"` 与 config 常量 |
| `server/internal/hub/agent/factory.go` | 注册 `codexapp` native creator |

不要复用 ACP 的 `ownedConn` 直接发送 app-server 消息。`ownedConn` 假设对端是真 ACP wire，会发送带 `"jsonrpc":"2.0"` 的请求；app-server wire schema 省略该字段，且存在 app-server 专属 server request。`codexapp` 复用 `ACPProcess` 的 JSONL subprocess 能力，但自己做 request matching、thread routing 和协议转换。

`codexappRuntime` 必须从 Phase 1 就按 `threadId -> codexappConn` 路由。Phase 1 可以是 one conn -> one runtime -> one app-server process；未来 shared 模式只替换 runtime 获取方式，`codexappConn` 不应知道 runtime 是 owned 还是 shared。

## 传输差异

| 项 | ACP | app-server |
|---|---|---|
| 消息格式 | JSON-RPC 2.0，通常带 `jsonrpc` | JSON-RPC 2.0 语义，但 wire 上省略 `jsonrpc` |
| stdio 分帧 | JSONL | JSONL |
| 初始化 | `initialize` request | `initialize` request + `initialized` notification |
| 会话单位 | `session` | `thread` |
| 一次请求 | `session/prompt` | `turn/start` |
| 流式输出 | `session/update` notification | `item/*`, `turn/*`, `thread/*` notifications |
| 取消 | `session/cancel` notification | `turn/interrupt` request |
| 权限 | `session/request_permission` server request | `item/*/requestApproval` server request |

## ACP 覆盖审计

本节按 `docs/acp-protocol-full.zh-CN.md` 对照 `codexapp` 的 ACP 行为。结论：bridge 不应只做字段转换，还必须保证 ACP 的生命周期、能力声明、错误策略和事件顺序。

### Agent 方法覆盖

| ACP 方法 | `codexapp` 策略 | 流程要求 |
|---|---|---|
| `initialize` | 支持，映射到 app-server `initialize` + `initialized` | 必须先完成 app-server 握手，再返回 ACP `InitializeResult` |
| `authenticate` | 当前 WheelMaker `agent.Instance` 未暴露该方法；`codexapp` 返回 `authMethods: []` | 如果未来提供外部 ACP wire server，收到 `authenticate` 应返回 `-32601` 或明确的已认证空结果，不能悬挂 |
| `session/new` | 支持，映射到 `thread/start` | 返回 app-server `thread.id` 作为 ACP `sessionId`，并返回完整 `configOptions` |
| `session/load` | 支持，当前映射到 `thread/resume` | 当前返回 config options；`thread/read includeTurns=true` 与历史 `session/update` replay 是 follow-up |
| `session/list` | 支持，映射到 `thread/list` | 只有在 `agentCapabilities.sessionCapabilities.list` 声明后才可调用 |
| `session/prompt` | 支持，映射到 `turn/start` | 必须先产生用户消息/流式更新，最后在所有 update 发完后返回 `stopReason` |
| `session/cancel` | 支持，映射到 `turn/interrupt` | 必须结束当前 prompt，最终以 `stopReason=cancelled` 返回；不能把取消作为 prompt error |
| `session/set_config_option` | 支持，由 adapter 本地状态封装 | 必须返回所有 config options 的完整列表，而不是只返回被修改项 |
| `session/set_mode` | ACP deprecated；`codexapp` 不实现 | 新实现使用 `approval_preset` config option；如果外部 wire 收到 `session/set_mode`，应返回 `-32601` 或转换为 `approval_preset` 仅作兼容路径 |

### Client 方法覆盖

ACP 中的 Client 方法是 Agent 反向调用 Client 的能力。`codexapp` 背后是 app-server，文件与终端执行由 app-server/Codex 自己完成，因此不应伪造对 ACP `fs/*` 和 `terminal/*` 的依赖。

| ACP Client 方法 | `codexapp` 策略 |
|---|---|
| `session/request_permission` | 支持。由 app-server approval request 转成 ACP permission request |
| `fs/read_text_file` | 不发起。app-server 直接访问本地文件或通过自身 fs API 工作 |
| `fs/write_text_file` | 不发起。文件修改通过 app-server item/fileChange 事件映射成 ACP tool updates 是 follow-up |
| `terminal/create/output/wait_for_exit/kill/release` | 不发起。命令执行通过 app-server item/commandExecution 事件映射成 ACP tool updates 是 follow-up |
| `session/update` | 支持。所有用户可见 app-server turn/item/thread 事件都经此输出 |

### MCP Servers 覆盖

ACP 基线要求 Agent 支持 stdio MCP server 配置。当前 WheelMaker 代码通过 `emptyMCPServers()` 只传空列表，但 bridge 不能静默忽略非空 `mcpServers`，否则不是 ACP-compliant 行为。

`codexapp` 的实现规则：

- `mcpServers=[]`：正常走 app-server 默认 Codex MCP 配置。
- 非空 stdio MCP：必须在启动 app-server 前或 `thread/start/thread/resume` 前把这些 server materialize 到本次 `codexapp` 实例可见的 Codex config 中，并在需要时调用 app-server `config/mcpServer/reload`。
- HTTP MCP：只有在 `agentCapabilities.mcp.http=true` 时才接受；否则返回 ACP `Invalid params`。
- 未实现非空 MCP materialization 前，不得声称完整 ACP compliance；至少必须对非空 `mcpServers` 返回明确错误，不能悄悄丢弃。

### Session Update 类型覆盖

当前 Phase 1 基线只实现文本 prompt 的 synthetic `user_message_chunk`、assistant delta、reasoning delta、thread title update，以及基础 config option 返回。下表中的 history replay、tool lifecycle、plan streaming、usage/config update notification 是 follow-up 要求，不能当作当前已完成功能。

| ACP update | `codexapp` 来源 | 要求 |
|---|---|---|
| `user_message_chunk` | prompt 输入或 app-server `userMessage` item | 每个 prompt/replay 都应有用户消息，保证 recorder 可还原对话 |
| `agent_message_chunk` | `item/agentMessage/delta` 或 `agentMessage` item | delta 可增量输出；replay 可输出完整文本块 |
| `agent_thought_chunk` | reasoning delta/summary item | 仅输出 app-server 可见的 reasoning/summary，不虚构隐藏推理 |
| `tool_call` | app-server tool-like item started/requested | 必须先发 `pending`，不能跳过生命周期 |
| `tool_call_update` | output delta, patch update, item completed | 必须从 `in_progress` 走到 `completed` 或 `failed` |
| `plan` | `turn/plan/updated` | 每次发送完整计划列表，状态从 `inProgress` 转成 `in_progress` |
| `available_commands_update` | `codexapp` 自己封装的 slash command list | 可选；若发送，必须是完整命令列表 |
| `config_option_update` | `model/list` 变化或 `session/set_config_option` 后 | 必须发送完整 config option 列表 |
| `session_info_update` | thread name/title/updatedAt 变化 | 用于 title 和 updatedAt |
| `current_mode_update` | 不发送 | deprecated；用 `config_option_update approval_preset` |

## ACP In -> App-Server In

### `initialize`

ACP input:

```json
{
  "method": "initialize",
  "params": {
    "protocolVersion": 1,
    "clientCapabilities": {
      "fs": {"readTextFile": true, "writeTextFile": true},
      "terminal": true
    },
    "clientInfo": {"name": "wheelmaker", "version": "0.1"}
  }
}
```

App-server request:

```json
{
  "method": "initialize",
  "id": 1,
  "params": {
    "clientInfo": {
      "name": "wheelmaker",
      "title": "WheelMaker",
      "version": "0.1"
    },
    "capabilities": {
      "experimentalApi": false
    }
  }
}
```

Then send notification:

```json
{"method": "initialized"}
```

ACP result:

```json
{
  "protocolVersion": 1,
  "agentCapabilities": {
    "loadSession": true,
    "promptCapabilities": {"image": false},
    "sessionCapabilities": {"list": {}}
  },
  "agentInfo": {
    "name": "codexapp",
    "title": "CodexApp Server"
  }
}
```

### `session/new`

ACP input:

```json
{
  "method": "session/new",
  "params": {
    "cwd": "D:\\Code\\WheelMaker",
    "mcpServers": []
  }
}
```

App-server request:

```json
{
  "method": "thread/start",
  "id": 2,
  "params": {
    "cwd": "D:\\Code\\WheelMaker",
    "approvalPolicy": "on-request",
    "sandbox": "workspace-write",
    "serviceName": "wheelmaker"
  }
}
```

ACP result:

```json
{
  "sessionId": "<thread.id>",
  "configOptions": [
    {"id": "approval_preset", "category": "_approval_preset", "type": "select", "currentValue": "ask", "options": []},
    {"id": "model", "category": "model", "type": "select", "currentValue": "<model>", "options": []},
    {"id": "reasoning_effort", "category": "thought_level", "type": "select", "currentValue": "<effort>", "options": []}
  ]
}
```

Notes:

- Populate `model` and `reasoning_effort` from `model/list` when available.
- Phase 1: `mcpServers` must be empty. Non-empty lists return ACP `Invalid params`.
- Phase 2: non-empty stdio MCP lists require MCP materialization described in [MCP Servers 覆盖](#mcp-servers-覆盖).

### `session/load`

ACP input:

```json
{
  "method": "session/load",
  "params": {
    "sessionId": "thr_123",
    "cwd": "D:\\Code\\WheelMaker",
    "mcpServers": []
  }
}
```

App-server request:

```json
{
  "method": "thread/resume",
  "id": 3,
  "params": {
    "threadId": "thr_123",
    "cwd": "D:\\Code\\WheelMaker"
  }
}
```

ACP behavior:

- Current Phase 1 returns `{"configOptions": [...]}` after `thread/resume`.
- Follow-up: before returning from `session/load`, convert returned `thread.turns` into ACP `session/update` replay events.
- Follow-up replay order must be stable: older turns first, items in the order app-server returns them.
- Follow-up replay must not include a synthetic final `session/prompt` response per historical turn; WheelMaker recorder may synthesize prompt boundaries, but ACP load itself only emits `session/update` notifications then returns.

Replay mapping:

| app-server stored item | ACP replay update |
|---|---|
| `userMessage` | `user_message_chunk` |
| `agentMessage` | `agent_message_chunk` |
| `reasoning` | `agent_thought_chunk` |
| `plan` | `plan` |
| `commandExecution` | `tool_call` + `tool_call_update` |
| `fileChange` | `tool_call` + optional `tool_call_update` with diff |
| `mcpToolCall` | `tool_call` + `tool_call_update` |

### `session/prompt`

ACP input:

```json
{
  "method": "session/prompt",
  "params": {
    "sessionId": "thr_123",
    "prompt": [{"type": "text", "text": "分析这个仓库"}]
  }
}
```

App-server request:

```json
{
  "method": "turn/start",
  "id": 4,
  "params": {
    "threadId": "thr_123",
    "input": [
      {"type": "text", "text": "分析这个仓库", "text_elements": []}
    ],
    "model": "<selected model or null>",
    "effort": "<selected effort or null>",
    "approvalPolicy": "<selected approval_preset approvalPolicy>"
  }
}
```

Required ACP-side events before the app-server response completes:

1. Emit or replay `user_message_chunk` for the prompt input.
2. For every app-server stream notification, emit the mapped `session/update`.
3. If a permission request is active, block the relevant app-server approval response until ACP `session/request_permission` returns.
4. Drain all queued updates for the matching `turnId`.
5. Return `SessionPromptResult` with a valid ACP `stopReason`.

ACP result after matching `turn/completed`:

| app-server turn status | ACP stopReason |
|---|---|
| `completed` | `end_turn` |
| `interrupted` | `cancelled` |
| `failed` | `refusal` or error, depending on `turn.error` |
| transport/process failure | return error |

### `session/cancel`

ACP input:

```json
{
  "method": "session/cancel",
  "params": {"sessionId": "thr_123"}
}
```

App-server request:

```json
{
  "method": "turn/interrupt",
  "id": 5,
  "params": {
    "threadId": "thr_123",
    "turnId": "<active turn id>"
  }
}
```

If no active turn id is known, return success to WheelMaker and log a debug message.

Cancel flow requirements:

- Mark all adapter-tracked pending approvals for this turn as cancelled.
- Respond to app-server approval requests with `cancel`.
- Send `turn/interrupt` when an active `turnId` exists.
- Continue draining app-server notifications for the interrupted turn.
- Complete the original ACP `session/prompt` with `stopReason=cancelled`; do not return a JSON-RPC error for normal cancellation.

### `session/list`

ACP input:

```json
{
  "method": "session/list",
  "params": {"cwd": "D:\\Code\\WheelMaker", "cursor": null}
}
```

App-server request:

```json
{
  "method": "thread/list",
  "id": 6,
  "params": {
    "cwd": "D:\\Code\\WheelMaker",
    "cursor": null
  }
}
```

Schema note: `thread/list` 的正式返回字段是 `result.data`，不是 `result.threads`。adapter 必须只按官方 `data` 字段解码；`threads` 属于之前桥接实现的错误字段，不应保留兼容路径。

ACP result mapping:

| app-server `Thread` | ACP `SessionInfo` |
|---|---|
| `id` | `sessionId` |
| `cwd` | `cwd` |
| `name` or `preview` | `title` |
| `updatedAt` Unix seconds | `updatedAt` RFC3339 |

### `session/set_config_option`

ACP input:

```json
{
  "method": "session/set_config_option",
  "params": {
    "sessionId": "thr_123",
    "configId": "model",
    "value": "gpt-5.4"
  }
}
```

App-server behavior:

- No immediate app-server request is required for Phase 1.
- Store adapter-local config.
- Apply it on subsequent `turn/start`.
- Return the full synthetic ACP `configOptions` list.
- Emit `config_option_update` with the full list when the effective options change outside a direct `session/set_config_option` response.

Phase 1 synchronization rules:

- `session/new` returns a complete list containing `approval_preset`, `model`, and `reasoning_effort`.
- `session/load` returns the same complete list after applying persisted WheelMaker preferences.
- `session/set_config_option` always returns the complete list, even when only one option changes.
- `config_option_update` always carries the complete list, never a patch.
- `model` changes recompute `reasoning_effort.options` from the selected app-server model. If the previous `reasoning_effort.currentValue` is not supported by the new model, use that model's default effort.
- `approval_preset` changes are stored locally and applied to the next `thread/start`, `thread/resume`, or `turn/start` where app-server accepts the relevant fields.
- Stored preferences replayed through existing WheelMaker `applyStoredConfigOptions` must be accepted in any order and must still produce a complete list after each call.

Config mapping:

| ACP config id | Stored adapter state | App-server field on next `turn/start` |
|---|---|---|
| `model` | selected model id | `model` |
| `reasoning_effort` | selected reasoning effort | `effort` |
| `approval_preset=read_only` | approval/sandbox profile | `approvalPolicy=on-request`, sandbox read-only |
| `approval_preset=ask` | approval/sandbox profile | `approvalPolicy=on-request`, sandbox workspace-write |
| `approval_preset=auto` | approval/sandbox profile | `approvalPolicy=on-failure`, sandbox workspace-write |
| `approval_preset=full` | approval/sandbox profile | `approvalPolicy=never`, sandbox danger-full-access |

Schema note: app-server `thread/start` 和 `thread/resume` 使用 `sandbox` 字段，值是 `SandboxMode` 字符串：`read-only`、`workspace-write`、`danger-full-access`。app-server `turn/start` 使用 `sandboxPolicy` 字段，值是 `SandboxPolicy` 对象：`readOnly`、`workspaceWrite` 或 `dangerFullAccess`。adapter 不能把 thread 级别的 `sandbox` 字段发送给 `turn/start`。`turn/start` 的 reasoning 字段按当前官方 schema 是 `effort`，不是 `reasoningEffort`；`reasoning_effort` 只是 ACP-facing config option id。

`approval_preset`、`model`、`reasoning_effort` 都是 `codexapp` 自己封装并暴露的 ACP config options。它们不要求 app-server 原生存在同名字段；agent 内部负责把这些选项展开到 app-server 的 `thread/start`、`thread/resume`、`turn/start` 参数。

`ask` 的含义：

- `sandbox=workspace-write`：Codex 可以在工作区沙箱内读写。
- `approvalPolicy=on-request`：当 app-server 认为某个动作需要用户批准时，会发出 approval request。
- `codexapp` 再把 approval request 转成 ACP `session/request_permission`。
- 因此 `ask` 不是只读，也不是完全自动；它是默认的交互确认模式。

建议初始 preset：

| Preset | approvalPolicy | sandbox | 适用场景 |
|---|---|---|---|
| `read_only` | `on-request` | `read-only` | 只分析、解释、读代码；默认不允许写 |
| `ask` | `on-request` | `workspace-write` | 默认交互模式；需要批准的动作询问用户 |
| `auto` | `on-failure` | `workspace-write` | 沙箱内先执行，失败或需要升级时再询问 |
| `full` | `never` | `danger-full-access` | 高信任本地任务；不询问且无沙箱边界 |

## App-Server Out -> ACP Out

### Response mapping

| app-server response | ACP output |
|---|---|
| `initialize.result` | ACP `InitializeResult` |
| `thread/start.result.thread.id` | ACP `SessionNewResult.sessionId` |
| `thread/resume.result.thread.turns` | Follow-up：ACP replay `session/update` events before `SessionLoadResult` |
| `turn/start.result.turn.id` | store active turn id, no immediate ACP update required |
| `turn/interrupt.result` | no ACP direct output |
| JSON-RPC error | ACP method error |

### Notification mapping

当前实现基线处理 assistant/reasoning text deltas、`turn/started`、matching `turn/completed` 和 thread name updates。工具、计划、用量和配置通知仍属于后续映射范围。

Schema note: `turn/started` 和 `turn/completed` 的正式参数形状是 `params.threadId` 加 `params.turn` 对象；`turn.id` 和 `turn.status` 在嵌套对象内。adapter 可以兼容历史 flat `turnId/status` 测试形状，但真实 app-server 通知必须按嵌套 `turn` 解码。`thread/name/updated` 的正式字段是 `threadName`，不是 `name`。

| app-server notification | ACP update |
|---|---|
| `thread/started` | optional `session_info_update` |
| `thread/name/updated` | `session_info_update.title` |
| `thread/tokenUsage/updated` | `usage_update` |
| `turn/started` | store active turn id |
| `turn/plan/updated` | `plan` |
| `item/started` with `commandExecution` | `tool_call` kind `execute`, status `pending`, then `tool_call_update` status `in_progress` |
| `item/started` with `fileChange` | `tool_call` kind `write`, status `pending`, then `tool_call_update` status `in_progress` |
| `item/started` with `mcpToolCall` | `tool_call` kind `other`, status `pending`, then `tool_call_update` status `in_progress` |
| `item/started` with `webSearch` | `tool_call` kind `read`, status `pending`, then `tool_call_update` status `in_progress` |
| `item/agentMessage/delta` | `agent_message_chunk` |
| `item/reasoning/textDelta` | `agent_thought_chunk` |
| `item/reasoning/summaryTextDelta` | `agent_thought_chunk` |
| `item/commandExecution/outputDelta` | `tool_call_update` with text content |
| `item/fileChange/patchUpdated` | `tool_call_update` with diff content |
| `item/completed` | `tool_call_update` completed/failed, or final full text if needed |
| `turn/completed` | completes pending `SessionPrompt` and maps status to `stopReason` |
| `warning`, `configWarning`, `deprecationNotice` | system/debug log, optionally `agent_thought_chunk` only if user-visible |

### App-server `ThreadItem` to ACP tool call mapping

| `ThreadItem.type` | ACP `toolCallId` | ACP title | ACP kind | ACP status |
|---|---|---|---|---|
| `commandExecution` | `item.id` | command | `execute` | map command status |
| `fileChange` | `item.id` | "File change" or path summary | `write` | map patch status |
| `mcpToolCall` | `item.id` | `server.tool` | `other` | map MCP status |
| `dynamicToolCall` | `item.id` | namespace/tool | `other` | map dynamic status |
| `webSearch` | `item.id` | query | `read` | `pending` -> `in_progress` -> `completed` |

Status mapping:

| app-server status | ACP status |
|---|---|
| queued/pending | `pending` |
| running/inProgress/in_progress | `in_progress` |
| completed/succeeded/success | `completed` |
| failed/error | `failed` |
| interrupted/cancelled/canceled | `failed` or `cancelled` if the UI supports it |

ACP currently defines `cancelled` as `StopReasonCancelled`, but tool status docs only standardize `pending`, `in_progress`, `completed`, and `failed`. Prefer `failed` with raw output showing cancellation unless UI support for cancelled is added.

If app-server first reports a tool only when it is already running, `codexapp` must synthesize two ACP events in order:

1. `tool_call` with `status=pending`
2. `tool_call_update` with `status=in_progress`

This preserves ACP's no-skip lifecycle invariant.

### Text content mapping

App-server `UserInput`:

| ACP `ContentBlock` | app-server `UserInput` |
|---|---|
| `text` | `{type:"text", text}` |
| `resource_link` file URI | Phase 1 reject；后续可转 `{type:"mention", name, path}` |
| `image` with URI | Phase 1 reject；Phase 1.5 可转 `{type:"localImage", path}` 或 `{type:"image", url}` |
| `image` base64 only | Phase 1 reject；Phase 2 写 temp file 后转 `{type:"localImage", path}` |
| `resource` embedded text | Phase 1 reject；后续可 append 到 synthetic text input |

Capability rules:

- Always support ACP `text`.
- Phase 1 rejects `resource_link`, `resource`, `image`, and `audio` because it does not advertise those prompt capabilities.
- Phase 1: do not advertise `promptCapabilities.image`.
- Phase 1: do not advertise `promptCapabilities.embeddedContext`.
- Phase 1.5: advertise `promptCapabilities.image=true` only after URI image conversion is implemented and tested.
- Phase 2: add base64 image support through temp-file materialization.
- Do not advertise `promptCapabilities.audio` unless app-server input support and conversion exist.
- Do not advertise `promptCapabilities.embeddedContext` until embedded `resource` conversion is implemented. If not advertised and the client sends it anyway, return ACP `Invalid params`.

Image mapping once enabled:

| ACP image input | app-server input | Phase |
|---|---|---|
| `uri=file:///absolute/path.png` | `{type:"localImage", path:"/absolute/path.png"}` | 1.5 |
| `uri=D:\absolute\path.png` | `{type:"localImage", path:"D:\absolute\path.png"}` | 1.5 |
| `uri=https://...` | `{type:"image", url:"https://..."}` | 1.5 |
| `data=<base64>` | temp file then `{type:"localImage", path}` | 2 |
| no usable `uri` or `data` | ACP `Invalid params` | 1.5+ |

App-server agent output:

| app-server output | ACP `ContentBlock` |
|---|---|
| agent message delta string | `{type:"text","text":delta}` |
| reasoning delta string | `{type:"text","text":delta}` |
| command output delta string | `ToolCallContent{type:"content", content:{type:"text", text:delta}}` |
| file patch update | `ToolCallContent{type:"diff", path, oldText, newText}` |

## Server Request Mapping

App-server can send server-initiated requests to the client. The adapter must respond to app-server after optionally asking WheelMaker through ACP permission callbacks.

### Command approval

App-server request:

```json
{
  "method": "item/commandExecution/requestApproval",
  "id": 10,
  "params": {
    "threadId": "thr_123",
    "turnId": "turn_456",
    "itemId": "item_789",
    "command": "npm test",
    "cwd": "D:\\Code\\WheelMaker"
  }
}
```

ACP callback:

```json
{
  "sessionId": "thr_123",
  "toolCall": {
    "toolCallId": "item_789",
    "title": "npm test",
    "kind": "execute",
    "status": "pending"
  },
  "options": [
    {"optionId": "allow_once", "name": "Allow once", "kind": "allow_once"},
    {"optionId": "allow_always", "name": "Allow for session", "kind": "allow_always"},
    {"optionId": "reject", "name": "Reject", "kind": "reject_once"}
  ]
}
```

App-server response:

| ACP permission result | App-server decision |
|---|---|
| selected `allow_once` | `accept` |
| selected `allow_always` | `acceptForSession` |
| selected reject option | `decline` |
| cancelled/error | `cancel` |

### File-change approval

Map `item/fileChange/requestApproval` similarly:

| ACP permission result | App-server decision |
|---|---|
| selected `allow_once` | `accept` |
| selected `allow_always` | `acceptForSession` |
| selected reject option | `decline` |
| cancelled/error | `cancel` |

### Unsupported server requests

| app-server request | Phase 1 response |
|---|---|
| `item/tool/requestUserInput` | JSON-RPC `-32601 Method not found`; do not block |
| `mcpServer/elicitation/request` | return cancelled |
| `item/permissions/requestApproval` | fail closed unless mapped later |
| `item/tool/call` dynamic tool | method not found unless dynamic tools are explicitly enabled |
| `account/chatgptAuthTokens/refresh` | method not found; app-server should own auth through Codex |

## Failure And Ordering Rules

- `initialize` must be idempotent at the adapter boundary: one ACP initialize maps to exactly one app-server initialize per process. Repeated ACP initialize should return the cached result or a protocol error consistently.
- `session/new`, `session/load`, `session/prompt`, `session/list`, and `session/set_config_option` must reject calls before initialization.
- `session/prompt` must reject empty or unsupported prompt content with ACP `Invalid params`.
- Only one active prompt per ACP session is allowed. A second prompt while a turn is active must either wait behind the session lock or return a clear busy error; it must not start an overlapping app-server turn for the same thread.
- Follow-up: `session/load` replay must preserve turn order and item order.
- `session/prompt` must not return before all queued updates for the matching turn have been drained.
- `session/prompt` normal cancellation must return `stopReason=cancelled`, not an error.
- Pending permission requests must be resolved as `cancelled`/`cancel` when a prompt is cancelled or the process exits.
- Tool calls must obey ACP lifecycle ordering: `pending` -> `in_progress` -> `completed`/`failed`.
- Follow-up: `plan` updates must be full-list replacements.
- `config_option_update` must be a full-list replacement.
- Paths emitted in tool locations and diffs must be absolute, and line numbers must remain 1-based.
- If app-server emits events for another thread, ignore unless the adapter has subscribed/loaded that thread.
- If app-server emits events for the same thread but unknown turn, log and ignore for prompt completion.
- If `turn/start` response is received but `turn/completed` never arrives, current WheelMaker stream timeout handling should surface the stall.
- If app-server returns overload error `-32001`, retry only safe non-turn-start requests with jittered exponential backoff.

## ACP Compliance Gaps To Close Before Implementation Is Called Complete

The following are not optional if `codexapp` is expected to behave as an ACP-compliant agent rather than a best-effort bridge:

1. Non-empty stdio `mcpServers` must be honored or explicitly rejected before any prompt starts; silent drop is invalid.
2. Tool lifecycle synthesis must be tested so app-server `item/started` never skips ACP `pending`.
3. Prompt cancellation must resolve pending app-server approvals and return ACP `stopReason=cancelled`.
4. `session/load` replay must be tested for ordering and for "updates before load response".
5. `session/set_config_option` must always return the complete option list.
6. `config_option_update` and `plan` must be full-list replacement events.
7. Unsupported prompt content must be gated by `promptCapabilities`.
8. Deprecated `session/set_mode` and missing `authenticate` must have deterministic error/compat behavior if an external ACP wire surface is added.

## Phase 1 Minimum Mapping

Current Phase 1 implementation baseline can ship with:

- `initialize`
- `session/new`
- `session/load`
- `session/prompt`
- `session/cancel`
- `session/list`
- `session/set_config_option` for local synthetic `approval_preset`, `model`, and `reasoning_effort`
- `item/agentMessage/delta`
- `item/reasoning/textDelta`
- `item/reasoning/summaryTextDelta`
- `thread/name/updated`
- command and file approval request skeleton
- config option synchronization for `approval_preset`, `model`, and `reasoning_effort`
- unsupported prompt capability rejection
- non-empty `mcpServers` rejection

Follow-up Phase 1 work before claiming broader ACP coverage:

- `resource_link` prompt input
- `session/load` history replay
- `turn/plan/updated`
- `item/started`
- `item/completed`
- `item/commandExecution/outputDelta`
- `item/fileChange/patchUpdated`
- approval fail-closed cleanup for cancel/process close

Everything else can be logged and ignored until the UI needs it.
