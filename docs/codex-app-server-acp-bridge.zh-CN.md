# Codex App-Server 与 ACP 转换文档

日期：2026-05-12
状态：Phase 1 修订版

本文是 WheelMaker `codexapp` agent 的协议转换说明。`codexapp` 对上必须表现为 ACP agent，对下连接 `codex app-server`。转换逻辑只能落在 agent 层，不能污染 `client.Session`、IM、registry、recorder 或 ACP 基础 transport。

## 资料来源

- OpenAI Codex app-server README：`https://github.com/openai/codex/blob/main/codex-rs/app-server/README.md`
- 当前本机运行时 schema：`codex-cli 0.129.0` 通过 `codex app-server generate-ts` 与 `generate-json-schema` 生成。
- WheelMaker ACP 文档：`docs/acp-protocol-full.zh-CN.md`
- 当前实现：`server/internal/hub/agent/codexapp_agent.go`、`server/internal/hub/agent/codexapp_convert.go`

重要约束：GitHub `main` 文档可能领先于本机 Codex 版本。代码实现以本机生成 schema 为准，文档用 OpenAI README 校验生命周期和流程语义。

## 官方 App-Server 落地总结

### 传输与握手

- app-server 使用 JSON-RPC 2.0 语义，但 wire 上省略 `jsonrpc: "2.0"`。
- `stdio://` 是 JSONL；同一条 stdio stream 可以承载多个 thread。
- 每条连接必须先发送一次 `initialize` request，再发送 `initialized` notification。
- 初始化前发送其他请求会失败；重复初始化会失败。
- 客户端请求、服务端响应、服务端 notification、服务端反向 request 共用同一条 stream。

### 核心对象

- Thread：Codex 对话。一个 thread 包含多个 turn。
- Turn：一次用户请求及其后续智能体工作。一个 turn 包含多个 item。
- Item：turn 内的输入或输出单元，例如 `userMessage`、`agentMessage`、`reasoning`、`commandExecution`、`fileChange`、`mcpToolCall`、`webSearch`、`imageView`。

### 生命周期

1. `initialize` request。
2. `initialized` notification。
3. `thread/start` 创建新 thread，或 `thread/resume` 恢复已有 thread。
4. `turn/start` 追加用户输入并启动一次 turn。
5. 服务端通过 `turn/*`、`item/*`、`thread/*` notification 流式输出。
6. `turn/completed` 表示本次 turn 结束。
7. `turn/interrupt` 请求取消正在运行的 turn，最终仍以 `turn/completed.status = interrupted` 收尾。

### 官方字段形状

`model/list` 返回：

```json
{
  "data": [
    {
      "id": "gpt-5.4",
      "model": "gpt-5.4",
      "displayName": "GPT-5.4",
      "supportedReasoningEfforts": [
        { "reasoningEffort": "low", "description": "Lower latency" }
      ],
      "defaultReasoningEffort": "medium",
      "inputModalities": ["text", "image"],
      "isDefault": true
    }
  ],
  "nextCursor": null
}
```

`thread/start` / `thread/resume` 的 thread 级 sandbox 字段是字符串 `sandbox`：

```json
{
  "cwd": "D:/Code/WheelMaker",
  "model": "gpt-5.4",
  "approvalPolicy": "on-request",
  "sandbox": "workspace-write",
  "serviceName": "wheelmaker"
}
```

`turn/start` 的 turn 级 sandbox 字段是对象 `sandboxPolicy`，不能发送 thread 级 `sandbox`：

```json
{
  "threadId": "thr_123",
  "input": [
    { "type": "text", "text": "Run tests", "text_elements": [] }
  ],
  "model": "gpt-5.4",
  "effort": "medium",
  "approvalPolicy": "on-request",
  "sandboxPolicy": {
    "type": "workspaceWrite",
    "writableRoots": ["D:/Code/WheelMaker"],
    "networkAccess": false,
    "excludeTmpdirEnvVar": false,
    "excludeSlashTmp": false
  }
}
```

`UserInput` 是 discriminated union：

| app-server input | 字段 |
|---|---|
| text | `type`, `text`, `text_elements` |
| image URL | `type=image`, `url` |
| local image | `type=localImage`, `path` |
| skill | `type=skill`, `name`, `path` |
| mention | `type=mention`, `name`, `path` |

`turn/started` / `turn/completed` 的正式 notification 参数是：

```json
{ "threadId": "thr_123", "turn": { "id": "turn_456", "status": "inProgress", "items": [] } }
```

`thread/name/updated` 的正式字段是：

```json
{ "threadId": "thr_123", "threadName": "Bug bash notes" }
```

### 事件模型

- `item/started` 发出完整 item，用于立即渲染。
- item 运行过程中可能出现 delta，例如 `item/agentMessage/delta`、`item/reasoning/summaryTextDelta`、`item/commandExecution/outputDelta`、`item/fileChange/patchUpdated`。
- `item/completed` 发出最终 item，是权威最终状态。
- `turn/plan/updated` 是完整计划快照，不是增量。
- `turn/diff/updated` 是 turn 聚合 diff 快照。
- `thread/tokenUsage/updated` 是用量更新。

### 审批模型

app-server 审批是服务端反向 JSON-RPC request：

| app-server request | 客户端响应 |
|---|---|
| `item/commandExecution/requestApproval` | `{ "decision": "accept" | "acceptForSession" | "decline" | "cancel" | ... }` |
| `item/fileChange/requestApproval` | `{ "decision": "accept" | "acceptForSession" | "decline" | "cancel" }` |
| `item/permissions/requestApproval` | `{ "permissions": <granted subset>, "scope": "turn" | "session" }` |
| `mcpServer/elicitation/request` | `{ "action": "accept" | "decline" | "cancel", "content": ..., "_meta": ... }` |
| `item/tool/requestUserInput` | `{ "answers": { ... } }` |
| `item/tool/call` | dynamic tool result |

`serverRequest/resolved` 会通知反向 request 已被响应或清理。

## ACP 对照

### ACP 必须保持的流程

- `initialize` 之后才允许 `session/*`。
- `session/new` 创建会话并返回 `sessionId`，可携带完整 `configOptions`。
- `session/load` 必须先通过 `session/update` 重播完整历史，再返回 load 结果。
- `session/prompt` 必须先流出所有 `session/update`，最后返回 `stopReason`。
- `session/cancel` 是 notification；正常取消必须让原 `session/prompt` 返回 `stopReason=cancelled`，不能返回 error。
- 工具生命周期必须是 `pending -> in_progress -> completed/failed`，不能跳过 `tool_call pending`。
- `plan` update 必须是完整列表替换。
- `session/set_config_option` 必须返回完整 config option 列表，不是 patch。
- Agent 未声明的 prompt capability 必须拒绝对应 content block。

### 身份模型修正

旧文档写过：

```text
ACP sessionId == app-server threadId == WheelMaker SessionRecord.ID
```

这个规则不成立，必须废弃。

正确模型：

```text
WheelMaker SessionRecord.ID / ACP sessionId  稳定上层会话 ID
codexapp runtimeThreadId                     当前 app-server thread.id
codexapp internal map                        acpSessionId -> runtimeThreadId
```

通常 `acpSessionId == runtimeThreadId`。但 app-server 存在一个关键行为：仅 `thread/start` 不一定产生可恢复 rollout；如果还没有任何 `turn/start` 就重启 app-server，后续 `thread/resume` 可能返回 `no rollout found for thread id`。因此 bridge 必须允许内部 runtime thread 重建，而对上继续使用原 ACP sessionId。

规则：

- 上层永远只看到 ACP sessionId。
- app-server request 使用当前 runtimeThreadId。
- app-server notification/request 用 runtimeThreadId 路由到 conn。
- 转回 ACP 时，`SessionUpdateParams.sessionId` 必须写回 ACP sessionId。
- 若 `thread/resume(acpSessionId)` 因未 materialize 的 rollout 失败，agent 层可以 `thread/start` 新 runtime thread，并保存 `acpSessionId -> newThreadId` 映射；不得修改 `client.Session` 或 `instance.go`。

## 实现框架

```text
client.Session
  -> agent.Instance                    // existing ACP-shaped interface
    -> codexappConn                    // implements agent.Conn, per WheelMaker session
      -> codexappRuntime               // app-server JSON-RPC matching + thread routing
        -> ACPProcess                  // JSONL stdio subprocess transport only
          -> codex app-server
```

文件边界：

| 文件 | 职责 |
|---|---|
| `server/internal/hub/agent/codexapp_agent.go` | provider launch、runtime request matching、conn lifecycle、notification/request 转发 |
| `server/internal/hub/agent/codexapp_convert.go` | app-server 最小 schema、ACP/app-server 字段转换、config/preset 映射 |
| `server/internal/hub/agent/agent_test.go` | 复用现有 agent 包测试文件，不新增独立 codexapp test 包 |

禁止事项：

- 不改 `server/internal/hub/client` 来保存 app-server threadId。
- 不改 ACP 基础 `ownedConn` 来兼容 app-server。
- 不把旧错误字段作为长期兼容路径，例如 `model/list.result.models`、flat `turnId/status`、`thread/name/updated.name`。

## Config Options

`codexapp` 自己封装并暴露三个 ACP config options：

| ACP config id | category | 来源 | app-server 落点 |
|---|---|---|---|
| `approval_preset` | `_approval_preset` | adapter-local | `approvalPolicy` + `sandbox` / `sandboxPolicy` |
| `model` | `model` | `model/list.data` | `thread/start.model`、`thread/resume.model`、`turn/start.model` |
| `reasoning_effort` | `thought_level` | selected model 的 `supportedReasoningEfforts` | `turn/start.effort` |

Approval preset 映射：

| preset | approvalPolicy | thread sandbox | turn sandboxPolicy | 含义 |
|---|---|---|---|---|
| `read_only` | `on-request` | `read-only` | `readOnly` | 只读分析，写入必须审批或失败 |
| `ask` | `on-request` | `workspace-write` | `workspaceWrite` | 默认交互确认，工作区写入，敏感动作询问 |
| `auto` | `on-failure` | `workspace-write` | `workspaceWrite` | 工作区内自动尝试，失败或升级时再询问 |
| `full` | `never` | `danger-full-access` | `dangerFullAccess` | 无沙箱、无审批，高信任场景 |

规则：

- `ask` 和 `auto` 必须是两个不同选项，不能把 `ask` canonicalize 成 `auto`。
- `session/new`、`session/load`、`session/set_config_option` 都必须返回完整 `configOptions`。
- `model` 变化后必须重新计算 `reasoning_effort.options`；旧 effort 不再支持时落到该模型默认值。
- `model/list` 正式字段是 `data`，不是 `models`。

## ACP In -> App-Server In

### initialize

ACP `initialize` 转 app-server：

```json
{
  "method": "initialize",
  "params": {
    "clientInfo": { "name": "wheelmaker", "title": "WheelMaker", "version": "0.1.0" }
  }
}
```

随后发送：

```json
{ "method": "initialized", "params": {} }
```

ACP result：

- `agentInfo.name = codexapp`
- `loadSession = true`
- `sessionCapabilities.list = {}`
- Phase 1 `promptCapabilities.image/audio/embeddedContext = false`
- 不声明 HTTP MCP

### session/new

ACP `session/new`：

- `mcpServers` Phase 1 必须为空；非空返回 invalid params 风格错误。
- 调用 `model/list` 刷新模型和 reasoning effort。
- 调用 `thread/start`。
- 返回 ACP `sessionId`。初始情况下它等于 app-server `thread.id`。
- 保存 `acpSessionId -> runtimeThreadId`。
- 返回完整 config options。

### session/load

ACP `session/load`：

1. 用 ACP sessionId 查本地 `runtimeThreadId` 映射；没有映射时先假设二者相同。
2. 调用 `thread/resume(runtimeThreadId)`。
3. 如果 app-server 返回 `no rollout found for thread id`，说明该 thread 尚未 materialize。agent 层应 `thread/start` 创建新 runtime thread，保存 `acpSessionId -> newThreadId`，对上仍认为 load 成功。
4. 如果 `thread/resume` 返回 `thread.turns`，必须把历史 turns 转成 ACP `session/update`，重播完成后再返回。
5. 返回完整 config options。

历史 replay 映射：

| app-server stored item | ACP replay |
|---|---|
| `userMessage.content` | `user_message_chunk` |
| `agentMessage.text` | `agent_message_chunk` |
| `reasoning.summary/content` | `agent_thought_chunk` |
| `plan.text` | `plan` |
| `commandExecution` | `tool_call pending` + `tool_call_update final` |
| `fileChange` | `tool_call pending` + `tool_call_update final diff` |
| `mcpToolCall` / `dynamicToolCall` / `webSearch` / `imageView` | `tool_call pending` + `tool_call_update final` |

### session/prompt

ACP `session/prompt`：

- Resolve ACP sessionId 到 runtimeThreadId。
- 转换 prompt content 为 app-server `input`。
- 调用 `turn/start`，附带当前 `model`、`effort`、`approvalPolicy`、`sandboxPolicy`。
- 等待 matching `turn/completed`。
- 返回 ACP stopReason。

StopReason 映射：

| app-server turn status | ACP stopReason |
|---|---|
| `completed` | `end_turn` |
| `interrupted` | `cancelled` |
| `failed` | `refusal` 或 error，Phase 1 可先返回 `end_turn` 外的失败映射 |

实时 prompt 不需要 echo 用户输入；用户消息已经在 `session/prompt` 请求里。`session/load` replay 必须发 `user_message_chunk`，因为 load 没有对应的 live prompt 请求。

### session/cancel

ACP `session/cancel`：

- Resolve ACP sessionId 到 runtimeThreadId。
- 如果有 active turn，调用 `turn/interrupt`。
- 正在等待的 `session/prompt` 返回 `stopReason=cancelled`。
- pending approval 应向 app-server 返回 `cancel` 或 denied subset。

### session/list

ACP `session/list` 转 `thread/list`。

正式响应字段是 `data`：

| app-server Thread | ACP SessionInfo |
|---|---|
| `id` | `sessionId` |
| `cwd` | `cwd` |
| `name` or `preview` | `title` |
| `updatedAt` Unix seconds | RFC3339 `updatedAt` |

## App-Server Out -> ACP Out

### Notification 映射

| app-server notification | ACP output |
|---|---|
| `item/agentMessage/delta` | `agent_message_chunk` |
| `item/reasoning/textDelta` | `agent_thought_chunk` |
| `item/reasoning/summaryTextDelta` | `agent_thought_chunk` |
| `turn/plan/updated` | `plan`，完整 entries |
| `item/started` tool-like | `tool_call` status `pending`，必要时再发 `tool_call_update in_progress` |
| `item/commandExecution/outputDelta` | `tool_call_update` text content |
| `item/fileChange/patchUpdated` | `tool_call_update` diff content |
| `item/completed` tool-like | `tool_call_update completed/failed` |
| `turn/completed` | 结束等待中的 `session/prompt` |
| `thread/name/updated` | `session_info_update.title` |
| `thread/tokenUsage/updated` | `usage_update`，若 ACP 类型支持 |
| `warning` / `configWarning` / `deprecationNotice` | 先记录日志；只有用户可见时再转 ACP 文本 |

Tool-like item：

| ThreadItem.type | ACP kind | title |
|---|---|---|
| `commandExecution` | `execute` | `command` |
| `fileChange` | `write` | path summary |
| `mcpToolCall` | `other` | `server/tool` |
| `dynamicToolCall` | `other` | `namespace/tool` |
| `webSearch` | `read` | query |
| `imageView` | `read` | path |

Status 映射：

| app-server status | ACP status |
|---|---|
| empty / queued / pending | `pending` 或 `in_progress`，取决于事件阶段 |
| `inProgress` / `running` | `in_progress` |
| `completed` / `success` | `completed` |
| `failed` / `error` / `declined` / `cancelled` | `failed` |

ACP 标准工具状态只有 `pending`、`in_progress`、`completed`、`failed`。不要输出非标准 `cancelled` tool status。

### Server Request 映射

| app-server request | ACP callback | app-server response |
|---|---|---|
| `item/commandExecution/requestApproval` | `session/request_permission` kind `execute` | selected -> `accept` / `acceptForSession` / `decline` / `cancel` |
| `item/fileChange/requestApproval` | `session/request_permission` kind `write` | selected -> `accept` / `acceptForSession` / `decline` / `cancel` |
| `item/permissions/requestApproval` | `session/request_permission` kind `other` | allow -> requested permission subset；reject/cancel -> empty permissions |
| `mcpServer/elicitation/request` | Phase 1 不弹复杂表单 | `{ "action": "cancel", "content": null, "_meta": null }` |
| `item/tool/requestUserInput` | Phase 1 不支持 | method not found 或空 answers，不能悬挂 |
| `item/tool/call` | Phase 1 不启用 dynamic tools | method not found |
| `account/chatgptAuthTokens/refresh` | Phase 1 不由 WheelMaker 管理外部 token | method not found |

## Prompt Content Capability

Phase 1 只声明文本能力：

| ACP ContentBlock | Phase 1 行为 |
|---|---|
| `text` | 转 `{type:"text", text, text_elements:[]}` |
| `image` | 拒绝；不声明 `promptCapabilities.image` |
| `audio` | 拒绝；不声明 `promptCapabilities.audio` |
| `resource` | 拒绝；不声明 `embeddedContext` |
| `resource_link` | ACP 基线要求支持，但当前 Codex app-server 无同构输入；Phase 1 必须明确拒绝，后续通过 `mention` 或文本 materialization 实现 |

图片支持计划：

| ACP image | app-server input | 阶段 |
|---|---|---|
| `uri=file:///absolute/path.png` | `{type:"localImage", path}` | Phase 1.5 |
| `uri=http(s)://...` | `{type:"image", url}` | Phase 1.5 |
| `data=<base64>` | 写 temp file 后 `{type:"localImage", path}` | Phase 2 |

只有完整支持 ACP image 语义后才能把 `promptCapabilities.image` 改成 `true`。

## 本次代码审计结论

本次复核官方 schema 与 ACP 文档后，发现并纳入 Phase 1 修复的偏差如下：

1. `model/list` 仍接受旧的 `models` 字段；应只按官方 `data` 解码。
2. `approval_preset` 缺少 `ask` 选项，并且把 `ask` canonicalize 成 `auto`；应拆成两个不同 preset。
3. `auto` 当前映射成 `on-request`；应映射为 `on-failure`。
4. `turn/start.input.text` 缺少官方 `text_elements: []`。
5. `sandboxPolicy` 省略了官方 schema 中 required 的 `networkAccess`、`excludeTmpdirEnvVar`、`excludeSlashTmp` 字段。
6. `session/load` 不 replay `thread.turns`，但 `initialize` 声明了 `loadSession=true`。
7. `item/started` 直接发 `tool_call_update`，跳过 ACP `tool_call pending`。
8. `item/fileChange/patchUpdated` 未映射。
9. `turn/plan/updated` 未映射，`plan` item 也不是完整列表替换语义。
10. `item/permissions/requestApproval` 未处理。
11. app-server runtime thread 与 ACP session id 强绑定，无法正确处理未 materialize rollout。
12. `thread/name/updated`、`turn/completed` 等仍有旧字段兼容路径，应按官方字段收敛。
13. 工具取消输出了非标准 ACP tool status `cancelled`。

## Phase 1 修复范围

本次 Phase 1 修复只做 agent 层：

- 更新转换文档。
- 修正 config options：`read_only`、`ask`、`auto`、`full`。
- 修正 `model/list.data`、`UserInput.text_elements`、`sandboxPolicy` 官方字段。
- 引入 agent 内部 `acpSessionId -> runtimeThreadId`，支持未 materialize thread 的 agent 层重建。
- `session/load` replay `thread.turns` 的基础文本、reasoning、plan、tool-like item。
- 修正 tool lifecycle：`tool_call pending` 后再 `tool_call_update`。
- 映射 `turn/plan/updated` 与 `item/fileChange/patchUpdated`。
- 支持 `item/permissions/requestApproval` 的 fail-closed/allow-subset 转换。
- 移除测试中的旧 app-server 字段假设。

不在本次 Phase 1 做：

- 非空 ACP `mcpServers` materialization。
- ACP image capability 开启。
- dynamic tools。
- Apps/skills UI 查询。
- Realtime/audio。
- 完整 token usage UI。
