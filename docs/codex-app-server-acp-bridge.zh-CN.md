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

| 可见 preset | approvalPolicy | thread sandbox | turn sandboxPolicy | 含义 |
|---|---|---|---|---|
| `auto` | `on-request` | `workspace-write` | `workspaceWrite` | 官方 App 默认交互模式；模型可主动请求审批 |
| `read_only` | `on-request` | `read-only` | `readOnly` | 只读分析；写入必须审批或失败 |
| `full` | `never` | `danger-full-access` | `dangerFullAccess` | 无沙箱、无审批，高信任场景 |

规则：

- `approval_preset` 可见选项必须与官方 App 对齐：`Auto`、`Read-only`、`Full Access`。
- `ask` 只作为旧 ACP 配置值的输入兼容，进入 adapter 后 canonicalize 成 `auto`，不得继续作为可见选项返回。
- `on-failure` 虽仍在 app-server `AskForApproval` 枚举中，但官方 CLI 已标记为 deprecated；交互默认使用 `on-request`。
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
4. 如果 `thread/resume.thread.turns` 为空，或任一 turn 的 `itemsView != "full"`，必须调用 `thread/read {includeTurns:true}` 补齐完整历史。
5. 把完整 turns 转成 ACP `session/update`，重播完成后再返回。
6. 返回完整 config options。

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
- app-server notification/request 必须按 runtimeThreadId 进入 per-thread 串行队列；同一 thread 内，`turn/completed` 只能在前序 `session/update` callback 完成后结束 prompt。

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
- WheelMaker 通用 ACP client 流程必须先调用 Agent `session/cancel`，再取消本地 prompt context；否则 prompt 可能错误返回 `context.Canceled`。
- 如果有 active turn，调用 `turn/interrupt`。
- 正常情况下等待 app-server `turn/completed.status=interrupted` 后，让正在等待的 `session/prompt` 返回 `stopReason=cancelled`。
- 如果 app-server 未及时收尾，agent 层允许超时后合成 `cancelled`，避免 prompt 永久悬挂。
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

`FileUpdateChange` 必须按官方 app-server schema 解码：

```ts
type FileUpdateChange = {
  path: string
  kind: { type: "add" } | { type: "delete" } | { type: "update", move_path: string | null }
  diff: string
}
```

不能把 `changes[].kind` 解成字符串；否则 `thread/resume` replay 历史 fileChange 时会在 `session/load` / config update 的 `ensureReady` 阶段失败。

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

Phase 1 只声明文本 prompt capability；`resource_link` 是 ACP 基线内容块，不受 `promptCapabilities.embeddedContext` 控制，必须接受：

| ACP ContentBlock | Phase 1 行为 |
|---|---|
| `text` | 转 `{type:"text", text, text_elements:[]}` |
| `resource_link` file URI | 转 `{type:"mention", name, path}` |
| `resource_link` non-file URI | 转 `{type:"text", text:"Resource link: ...", text_elements:[]}` |
| `image` | 拒绝；不声明 `promptCapabilities.image` |
| `audio` | 拒绝；不声明 `promptCapabilities.audio` |
| `resource` | 拒绝；不声明 `embeddedContext` |

图片支持计划：

| ACP image | app-server input | 阶段 |
|---|---|---|
| `uri=file:///absolute/path.png` | `{type:"localImage", path}` | 图片最小闭环 |
| `uri=http(s)://...` | `{type:"image", url}` | 图片最小闭环 |
| `data=<base64>` | 写 session-scoped temp file 后 `{type:"localImage", path}` | 图片最小闭环 |

只有完整支持 ACP image 语义后才能把 `promptCapabilities.image` 改成 `true`。由于 WheelMaker Web App 与飞书当前都会把图片送成 ACP `image.data` base64，不能单独发布只支持 `uri` 的能力。

图片临时文件生命周期：

- base64 图片由 `codexapp` agent 写入 session-scoped 临时目录。
- 临时目录放在 session 资源目录内，例如 `~/.wheelmaker/db/session/<projectName>/<sessionId>/images/...`。
- `projectName` 与 `sessionId` 作为路径段使用前必须做 path-safe 处理，不能允许路径分隔符或 `..` 逃逸出项目 artifact 目录。
- 临时目录不绑定 `AgentInstance` / `codexappConn` 生命周期；多 session 并存时，一个 session 被 suspend 或 instance close 不代表该 session 已结束。
- `client` 只在真正删除 session 时调用通用 agent artifact cleanup hook，例如 `agent.CleanupSessionArtifacts(projectName, agentType, sessionID)`；`codexapp` 在 agent 包内清理自己的图片目录。
- `turn/start` 后不能立即删除临时图片；app-server 可能异步读取 `localImage.path`。
- `session.archive` 成功移出 active session 时清理该 session 下的临时图片目录；turn 总数 `< 3` 的短会话会直接永久删除并清理同一目录。orphan/TTL 清理作为后续兜底任务，避免长期未归档的历史图片无限累积。

base64 图片落盘规则：

- 文件名使用内容 hash：`sha256-<hex>.<ext>`。
- 扩展名从 `mimeType` 推导：`image/png -> .png`、`image/jpeg -> .jpg`、`image/webp -> .webp`、`image/gif -> .gif`。
- `mimeType` 缺失时，用解码后的 bytes 做内容嗅探；仍不是 `image/*` 则拒绝该 prompt。
- 同一 session 内相同图片复用同一路径，不重复写。
- 不使用用户传入的 `name` / `title` 作为文件名，避免路径注入和非法字符问题。
- 单张 base64 图片解码后最大 `20 MiB`；多图不另设数量上限，但每张独立校验，任一图片超限则该 prompt 明确失败。
- `image` 同时带 `data` 和 `uri` 时优先使用 `data`；只有 `data` 为空才处理 `uri`。
- `file://` 图片不限制在 workspace 内；只校验可转换为本地绝对路径，实际读权限交给 app-server/Codex 的 sandbox 与审批策略处理。
- prompt 中任一图片转换失败时，整条 `session/prompt` 失败；不得静默跳过坏图片后继续发送。

## 当前校准结论

本轮以 `codex app-server generate-ts --experimental` 生成的 0.129.0 官方 schema、CLI help 与 ACP 文档为准，Phase 1 必须维持以下约束：

1. `model/list` 响应使用正式字段 `data`。
2. `approval_preset` 可见选项为官方 App 的三项：`auto`、`read_only`、`full`。
3. 旧配置值 `ask` 不再作为可见选项，但 adapter 需要输入兼容并归一为 `auto`。
4. `auto` 使用交互推荐的 `approvalPolicy=on-request` 与 `workspace-write`；`on-failure` 已 deprecated，不作为默认交互模式。
5. `turn/start.input.text` 必须带 `text_elements: []`。
6. `turn/start.sandboxPolicy.workspaceWrite` 必须带 `networkAccess`、`excludeTmpdirEnvVar`、`excludeSlashTmp`。
7. `FileUpdateChange.kind` 必须是官方对象 union，不能按 string 解码。
8. `session/load` 必须 replay `thread.turns`，并在 turns 为空或 `itemsView != full` 时使用 `thread/read includeTurns=true` 兜底。
9. app-server runtime thread id 与 ACP session id 只能在 agent 内部映射，不污染 client/session 上层。
10. app-server notification/request 必须按 thread 串行转发，保证 ACP update 先于 prompt result。
11. ACP tool lifecycle 必须先 `tool_call pending`，再 `tool_call_update`。
12. ACP 工具状态只能输出 `pending`、`in_progress`、`completed`、`failed`。
13. `resource_link` 是 ACP 基线能力；Phase 1 必须支持 file URI 与 non-file URI 的降级转换。

## Phase 1 范围

Phase 1 只做 agent 层，并保持对上层透明：

- 基础聊天：`initialize`、`session/new`、`session/load`、`session/prompt`、`session/cancel`、`session/list`。
- Config 同步：`approval_preset`、`model`、`reasoning_effort` 在 `session/new`、`session/load`、`session/set_config_option` 中返回完整 options。
- 官方协议字段：`model/list.data`、`UserInput.text_elements`、`SandboxPolicy` required 字段、`FileUpdateChange.kind` 对象 union。
- Session 映射：agent 内部维护 `acpSessionId -> runtimeThreadId`，支持未 materialize thread 的 agent 层重建。
- 历史 replay：基础文本、reasoning、plan、tool-like item、fileChange diff。
- 实时事件：agent message delta、reasoning delta、plan update、fileChange patch update、tool start/completed。
- 权限请求：command/file approval 转 ACP `session/request_permission`，permissions approval 使用 allow-subset / fail-closed。
- 输入内容：text 与 resource_link；image/audio/resource 暂不声明能力。

不在 Phase 1 做：

- 非空 ACP `mcpServers` materialization。
- ACP image capability 开启。
- dynamic tools。
- Apps/skills UI 查询。
- Realtime/audio。
- 完整 token usage UI。
