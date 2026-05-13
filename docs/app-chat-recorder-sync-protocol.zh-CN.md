# Registry <-> App Chat 协议（当前实现）

## 1. 范围

本文档描述 web 客户端（`role=client`）与 registry/hub 运行时之间的 **当前** app chat 协议。
协议基于以下代码路径：

- `server/internal/registry/server.go`
- `server/internal/hub/reporter.go`
- `server/internal/hub/client/session_recorder.go`
- `server/internal/hub/client/session.go`
- `server/internal/hub/client/client.go`
- `app/web/src/services/registryClient.ts`
- `app/web/src/services/registryRepository.ts`
- `app/web/src/types/registry.ts`
- `app/web/src/main.tsx`

本文档中的协议为 app chat 当前使用的协议，不依赖旧的 chat 载荷格式。

## 2. 传输层与消息封装

- 传输层：WebSocket
- 消息封装：

```json
{
  "requestId": 1,
  "type": "request|response|error|event",
  "method": "...",
  "projectId": "hubId:projectName",
  "payload": {}
}
```

规则：

- `requestId` 在 `request`/`response`/`error` 消息中必填，类型为正整数（`>= 1`），同一连接内递增不重复。
- `event` 消息无 `requestId`。
- App chat 监听 `event.method = session.updated | session.message`。
- `projectId` 格式为 `hubId:projectName`，业务方法必填。

## 3. 握手（`connect.init`）

App 以 `role=client`、协议版本 `2.2` 连接。

请求：

```json
{
  "requestId": 1,
  "type": "request",
  "method": "connect.init",
  "payload": {
    "clientName": "wheelmaker-web",
    "clientVersion": "0.1.0",
    "protocolVersion": "2.2",
    "role": "client",
    "token": "<token-or-empty>"
  }
}
```

注意事项：

- Registry 精确校验 `protocolVersion`，必须为 `"2.2"`。
- 若 registry 配置了 token，`payload.token` 必须携带且匹配。
- 认证失败返回 `UNAUTHORIZED`，随后立即断连。
- 成功响应返回 `{ok, principal, serverInfo, features, hashAlgorithms}`。

## 4. App Chat 请求 API

App chat 的读写路径统一使用 `session.*` 族方法。所有方法均通过 registry 按 `projectId` 转发到对应 hub，hub 响应原样返回。

### 4.1 `session.list`

用途：进入 chat 页面时加载侧栏会话列表。

请求：

```json
{
  "requestId": 10,
  "type": "request",
  "method": "session.list",
  "projectId": "local-hub:WheelMaker",
  "payload": {}
}
```

响应载荷（`RegistrySessionSummary`）：

```json
{
  "sessions": [
    {
      "sessionId": "sess-1",
      "title": "Fix sync bug",
      "preview": "latest message preview...",
      "updatedAt": "2026-04-30T10:00:00Z",
      "messageCount": 42,
      "unreadCount": 3,
      "agentType": "codex",
      "configOptions": [
        {
          "id": "model",
          "name": "Model",
          "currentValue": "claude-sonnet-4-6",
          "options": [{"value": "claude-sonnet-4-6", "name": "Sonnet 4.6"}]
        }
      ]
    }
  ]
}
```

字段说明：

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `sessionId` | `string` | 是 | 会话唯一标识 |
| `title` | `string` | 是 | 会话标题（默认为 sessionId） |
| `preview` | `string` | 是 | 最新一条消息预览文本 |
| `updatedAt` | `string` | 是 | 最后活跃时间（RFC3339） |
| `messageCount` | `number` | 是 | 消息总数（实际是 prompt 数） |
| `unreadCount` | `number` | 否 | 未读消息数 |
| `agentType` | `string` | 否 | 代理类型 |
| `configOptions` | `ConfigOption[]` | 否 | 会话配置选项列表 |

### 4.2 `session.read`

用途：从检查点 `(promptIndex, turnIndex)` 拉取增量 prompt/turn。

请求载荷：

```json
{
  "sessionId": "sess-1",
  "promptIndex": 12,
  "turnIndex": 3
}
```

- `promptIndex` 和 `turnIndex` 可选。首次读取时不传或传 `0`，拉取全量消息。
- 传入 `promptIndex > 0` 或 `turnIndex > 0` 时执行增量读取。
- 传入 `promptIndex=N, turnIndex=0` 可读取第 N 个 prompt 的全部 turns，用于 prompt 局部补读。

响应载荷（后端线格式）：

```json
{
  "session": {
    "sessionId": "sess-1",
    "title": "Fix sync bug",
    "updatedAt": "2026-04-30T10:00:00Z",
    "agentType": "codex",
    "configOptions": [...]
  },
  "messages": [
    {
      "sessionId": "sess-1",
      "promptIndex": 12,
      "turnIndex": 4,
      "finished": true,
      "content": "{\"method\":\"agent_message_chunk\",\"param\":{\"text\":\"...\"}}"
    }
  ]
}
```

服务端过滤规则：

- 跳过 `promptIndex < request.promptIndex` 的 prompt。
- 同一 prompt 内，跳过 `turnIndex <= request.turnIndex` 的 turn。
- `session.read(P, T)` 返回客户端 finished 游标之后的增量，不返回完整 prompt 快照。
- `prompt_done` 是普通 turn，并推进 finished 游标。

> **前端处理**：`session.read` 返回的 `messages[]` 在前端经 `decodeSessionMessageFromEventPayload` 解码后，按 `promptIndex` 分组为 `prompts` 数组。最终返回 `{session, prompts, messages}` 供 UI 使用。

### 4.3 `session.new`

用途：创建新会话，通常在未选中会话时首次发送前调用。

请求载荷：

```json
{
  "agentType": "codex",
  "title": "optional title"
}
```

- `agentType`：必填，非空字符串。
- `title`：可选，会话标题。

响应载荷：

```json
{
  "ok": true,
  "session": {
    "sessionId": "sess-1",
    "title": "optional title",
    "updatedAt": "2026-04-30T10:00:00Z",
    "agentType": "codex",
    "configOptions": [
      {
        "id": "model",
        "name": "Model",
        "description": "选择 AI 模型",
        "category": "Model",
        "type": "enum",
        "currentValue": "claude-sonnet-4-6",
        "options": [
          {"value": "claude-sonnet-4-6", "name": "Sonnet 4.6"}
        ]
      }
    ]
  }
}
```

> **实现细节**：服务端 `CreateSession` 执行以下步骤：
> 1. Agent `Initialize` + `SessionNew` → 获取初始 configOptions
> 2. 应用已持久化的 agent 偏好覆盖
> 3. 连线 session state、持久化存储
> 4. 发布 `registry.session.updated` 事件
>
> 前端 `createChatSession` 调用后立即设置 `selectedChatId`、初始化消息存储和同步游标。

### 4.4 `session.send`

用途：向选中会话发送文本/图片块。

请求载荷：

```json
{
  "sessionId": "sess-1",
  "text": "hello",
  "blocks": [
    {"type": "text", "text": "hello"},
    {"type": "image", "mimeType": "image/png", "data": "<base64>"}
  ]
}
```

- `sessionId`：必填。
- `text`：可选，纯文本字符串（与 `blocks` 中的 text 块语义等价）。
- `blocks`：可选，`RegistryChatContentBlock[]`，每项 `{type, text?, mimeType?, data?}`。
- `type` 取值：`"text"` | `"image"`。

响应载荷：

```json
{
  "ok": true,
  "sessionId": "sess-1"
}
```

> **与 `chat.send` 的关系**：`chat.send` 存在于 registry/hub 路由中，但走的是 IM app channel 路径（`server/internal/im/app/app.go`），用于 slash command 处理（`/new`、`/list`、`/model` 等）。App chat UI 使用 `session.send` 路径，不调用 `chat.send`。

### 4.5 `session.setConfig`

用途：设置会话配置选项（如模型切换）。

请求载荷：

```json
{
  "sessionId": "sess-1",
  "configId": "model",
  "value": "claude-opus-4-6"
}
```

- `sessionId`：必填。
- `configId`：必填，非空字符串，配置项 ID（如 `"model"`、`"mode"`、`"thought_level"`）。
- `value`：必填，非空字符串，目标值。

响应载荷：

```json
{
  "ok": true,
  "sessionId": "sess-1",
  "configOptions": [
    {
      "id": "model",
      "name": "Model",
      "description": "选择 AI 模型",
      "category": "Model",
      "type": "enum",
      "currentValue": "claude-opus-4-6",
      "options": [
        {"value": "claude-sonnet-4-6", "name": "Sonnet 4.6"},
        {"value": "claude-opus-4-6", "name": "Opus 4.6"}
      ]
    }
  ]
}
```

> **实现细节**：
> 1. 服务端 `SetConfigOption` 持有 `promptMu` 锁，确保与 prompt 处理串行化。
> 2. 延迟初始化 agent 实例（`ensureInstance` + `ensureReady`）。
> 3. 通过 ACP 方法 `session/set_config_option` 下发到 agent。
> 4. 将返回的 configOptions 合并到 session state，持久化 agent 偏好。
> 5. 前端收到响应后，通过 `applyChatSessionConfigOptions` 将返回的 `configOptions` 合并到本地 `chatSessions` 状态中。

### 4.6 ConfigOption 类型定义

```typescript
interface RegistrySessionConfigOptionValue {
  value: string;
  name?: string;
  description?: string;
}

interface RegistrySessionConfigOption {
  id: string;
  name?: string;
  description?: string;
  category?: string;
  type?: string;
  currentValue?: string;
  options?: RegistrySessionConfigOptionValue[];
}
```

- `type` 取值示例：`"enum"`、`"string"`、`"boolean"`。
- 当 `options` 非空时，前端渲染为下拉选择器；否则渲染为文本输入框。
- 已知 config ID：`"model"`、`"mode"`、`"thought_level"`。

### 4.7 `session.markRead`

Registry 转发 `session.markRead`，但当前 hub session handler **未实现该方法**。App chat 流程中不可依赖此方法。调用会返回 unsupported 错误。

## 5. 实时事件

Hub 通过以下方法向 registry 发布事件：

- `registry.session.updated`
- `registry.session.message`

Registry 按 project 作用域向 app client 转推：

- `event.method = session.updated`
- `event.method = session.message`

### 5.1 `session.updated`

事件封装：

```json
{
  "type": "event",
  "method": "session.updated",
  "projectId": "local-hub:WheelMaker",
  "payload": {
    "session": {
      "sessionId": "sess-1",
      "title": "Fix sync bug",
      "updatedAt": "2026-04-30T10:00:00Z",
      "agentType": "codex",
      "configOptions": [...]
    }
  }
}
```

服务端 `publishSessionUpdated` 发送的 `sessionViewSummary` 结构：

| 字段 | 来源 |
|------|------|
| `sessionId` | session ID |
| `title` | `firstNonEmpty(title, sessionId)` |
| `updatedAt` | `lastActiveAt.UTC().Format(time.RFC3339)` |
| `agentType` | 创建时的 agentType |
| `configOptions` | 来自 `SessionRecord.AgentJSON` 中持久化的 config |

客户端行为：

- 通过 `mergeChatSession(prev, payload.session)` 合并/更新侧栏中的会话摘要。
- **不在此处**更新消息正文。

### 5.2 `session.message`

事件封装：

```json
{
  "type": "event",
  "method": "session.message",
  "projectId": "local-hub:WheelMaker",
  "payload": {
    "sessionId": "sess-1",
    "promptIndex": 12,
    "turnIndex": 4,
    "finished": true,
    "content": "{\"method\":\"agent_message_chunk\",\"param\":{\"text\":\"...\"}}"
  }
}
```

`RegistryChatMessageEventPayload`（TypeScript 类型）：

```typescript
interface RegistrySessionMessageEventPayload {
  sessionId: string;
  promptIndex: number;
  turnIndex: number;
  finished?: boolean;
  content: string;  // JSON 字符串
}
```

> **`content` 字符串**由服务端 `buildIMContentJSON` 生成，格式为 `json.Marshal(IMTurnMessage{Method, Param})`。
> **`finished` 是 turn envelope 状态**：`finished=true` 表示该 turn 已封口且可缓存。`content.param.text` 始终是当前 turn 的完整文本快照。

客户端消息键：

- `messageId = sessionId:promptIndex:turnIndex`

覆盖规则：

- 相同键 → 用最新内容覆盖（同 turn 增量合并）。

客户端事件处理完整流程：

1. 校验 `sessionId` 非空、`promptIndex > 0`、`turnIndex > 0`。
2. 生成 `messageId`。
3. 更新 `chatSessions` 侧栏（preview、time、title）。
4. 通过 `(promptIndex, turnIndex)` 游标去重；只有 `finished=true` 的 turn 推进读取游标。
5. 将消息 upsert 到 `chatMessageStoreRef`。
6. 若 `sessionId === chatSelectedIdRef.current`（当前查看的会话），更新 `chatMessages` 触发 UI 刷新。
7. 如果 incoming turn 跳过 prompt/turn，或在本地没有前一个 prompt 的 `prompt_done` 时进入下一个 prompt，则用最后一个 finished 游标调用 `session.read` 补读。

## 6. IM 消息内容（`payload.content`）

### 6.1 内容格式

`session.message.payload.content` 是一个 JSON 字符串：

```json
{
  "method": "...",
  "param": {...}
}
```

`IMTurnMessage` 结构（Go）：

```go
type IMTurnMessage struct {
    Method string          `json:"method"`
    Param  json.RawMessage `json:"param,omitempty"`
}
```

### 6.2 当前方法列表

Recorder 路径发出的全部内容方法：

| 线格式方法名 | Go 常量 | 对应 ACP 来源 | 含义 |
|-------------|---------|-------------|------|
| `prompt_request` | `IMMethodPromptRequest` | `session/prompt` (params) | 用户发起一次 prompt |
| `prompt_done` | `IMMethodPromptDone` | `session/prompt` (result) | prompt 完成 |
| `user_message_chunk` | `SessionUpdateUserMessageChunk` | `user_message_chunk` | 用户消息流式文本 |
| `agent_message_chunk` | `IMMethodAgentMessage` | `agent_message_chunk` | Agent 消息流式文本 |
| `agent_thought_chunk` | `IMMethodAgentThought` | `agent_thought_chunk` | Agent 思考流式文本 |
| `tool_call` | `IMMethodToolCall` | `tool_call` / `tool_call_update` | 工具调用及其更新 |
| `plan` | `IMMethodAgentPlan` | `plan` | Agent 计划 |
| `system` | `IMMethodSystem` | 系统事件 | 系统消息 |

> **注意事项**：
> - 线格式上 `tool_call_update` 被映射为 `tool_call`（同一方法，通过 `turnKey = ToolCallID` 合并）。
> - 线格式上 `agent_plan` 的 method 为 **`"plan"`**（不是 `"agent_plan"`）。

### 6.3 各方法参数结构

#### `prompt_request` 参数

```json
{
  "contentBlocks": [
    {"type": "text", "text": "hello"},
    {"type": "image", "mimeType": "image/png", "data": "<base64>"}
  ]
}
```

`contentBlocks` 为 `RegistrySessionContentBlock[]`，每项 `{type, text?, mimeType?, data?}`。

#### `prompt_done` 参数

```json
{
  "stopReason": "end_turn"
}
```

`stopReason` 取值：`"end_turn"` | `"max_tokens"` | 等。

#### text chunk 类参数（`user_message_chunk` / `agent_message_chunk` / `agent_thought_chunk`）

```json
{
  "text": "partial response..."
}
```

或 `param` 可以是纯字符串 `"partial response..."`（向后兼容）。

#### `tool_call` 参数

```json
{
  "cmd": "bash",
  "kind": "execute_command",
  "status": "running",
  "output": "stdout content..."
}
```

| 字段 | 说明 |
|------|------|
| `cmd` | 执行的命令 |
| `kind` | 工具类型标识 |
| `status` | 状态：`"running"` / `"done"` / `"error"` / `"need_action"` |
| `output` | 工具输出 |

同一次工具调用通过 `turnKey = params.Update.ToolCallID` 合并增量更新（状态、输出变化）。

#### `plan` 参数（agent_plan）

```json
{
  "content": "plan description",
  "status": "streaming"
}
```

**注意**：`param` 在 Go 侧是 `[]IMPlanResult` 数组，线格式上是**单个对象**（取决于序列化方式）。前端用 `extractTextFromIMParam` 统一处理。

#### `system` 参数

```json
{
  "text": "system notification message"
}
```

### 6.4 前端解码映射（`decodeSessionMessageFromEventPayload`）

| `method`（content JSON 内） | `role` | `kind` | `status` | text 来源 |
|---|---|---|---|---|
| `prompt_request` | `user` | `message` | `done` | `extractTextFromACPContent(param.contentBlocks)` |
| `prompt_done` | `system` | `prompt_result` | `done` | `param.stopReason` |
| `user_message_chunk` | `user` | `message` | `streaming` | `extractTextFromIMParam(param)` |
| `agent_message_chunk` | `assistant` | `message` | `streaming` | `extractTextFromIMParam(param)` |
| `agent_thought_chunk` | `assistant` | `thought` | `streaming` | `extractTextFromIMParam(param)` |
| `tool_call` | `system` | `tool` | 从 `param.status` 派生 | `extractTextFromIMParam(param)` |
| `agent_plan` | `assistant` | `thought` | `streaming` | `extractTextFromIMParam(param)` |
| `system` | `system` | `message` | `done` | `extractTextFromIMParam(param)` |
| 其他未知方法 | `assistant` | `message` | `done` | `extractTextFromIMParam(param)` |
| JSON 解析失败 | `assistant` | `message` | `done` | 原始 `content` 字符串 |

### 6.5 `extractTextFromIMParam` 文本提取逻辑

由于 IM channel 历史兼容，`param` 有多种格式：

1. **字符串**：直接返回。
2. **数组**：过滤每项中 `type === 'content'` 的 `content` 字段，拼接。
3. **对象**：按优先级提取：
   - `text`（字符串） → 直接返回
   - `output`（字符串） → 直接返回
   - `cmd`（字符串） → 直接返回
   - `contentBlocks`（数组） → 调用 `extractTextFromACPContent`

### 6.6 `normalizeChatStatus` 状态规范化

前端在解码时通过此函数将服务端状态字符串标准化：

| 线格式值 | 标准化为 |
|---------|---------|
| `"streaming"` / `"running"` / `"in_progress"` | `"streaming"` |
| `"done"` / `"completed"` | `"done"` |
| `"need_action"` / `"needs_action"` | `"needs_action"` |

## 7. App 同步模型

### 7.1 每会话游标

前端为每个会话维护两个游标（存储在 `Ref` 中）：

- `promptIndex`：当前最新的 prompt 序号。
- `turnIndex`：当前 prompt 内最新的 turn 序号。

### 7.2 更新逻辑

- **实时 `session.message` 事件**：upsert 前先从最新 finished 游标检测 prompt/turn gap。无 gap 时按 `(sessionId, promptIndex, turnIndex)` upsert，且仅在 `finished=true` 时推进游标。
- **`session.read` 主动拉取**：从检查点拉增量，替换受影响 prompt 边界之后的消息。
- **缓存规则**：`finished=false` 的 turn 只用于流式展示，不写入客户端 IndexedDB。
- **Prompt 边界**：`prompt_done` 是普通 finished turn，并推进游标。
- **权威 request turn**：server prompt_request is authoritative；app 不持久化乐观本地 `prompt_request`。

### 7.3 `RegistryChatMessage` 类型

```typescript
// 与后端 IMTurnMessage 线格式完全一致。
// role/kind/status/text/blocks 均为纯函数计算，不作为字段存储。
interface RegistrySessionMessage {
  sessionId: string;
  promptIndex: number;
  turnIndex: number;
  method: string;                     // IMTurnMessage.method
  param: Record<string, unknown>;     // IMTurnMessage.param
  finished: boolean;                  // 已封口、可缓存的 turn envelope 状态
}
```

## 8. 前端拉取触发器

当前 app chat 的拉取触发点：

1. **进入 chat tab**（`tab` 变为 `chat`）：仅调用 `session.list`。
2. **用户点击/打开会话**：以当前游标调用 `session.read`。
3. **重连成功且用户仍在 chat tab 且有选中会话**：
   - 静默重连路径：调用 `session.read`（增量）读取选中会话。
4. **全量重连且在 chat tab**：
   - 调用 `session.list`；消息正文不会自动读取，直到用户选择/读取会话。

当前 app 在项目切换时不会主动拉取所有会话消息。

## 9. 前端写入流程

当前 `sendChatMessage` 的完整流程：

```
用户输入 → 点击 Send
  ├─ 同步：捕获 trimmedText / blocks
  ├─ 同步：resetChatComposer() 清空输入框
  ├─ setChatSending(true)
  └─ 分支：
       ├─ !selectedChatId（无选中会话）
       │   ├─ beginNewChatFlow({title, text, blocks})
       │   │   ├─ setPendingNewChatDraft(draft)
       │   │   └─ setNewChatAgentPickerOpen(true)  打开 Agent 选择器
       │   └─ return（等待用户选择 agent）
       │
       └─ selectedChatId 存在
           ├─ service.sendSessionMessage({sessionId, text, blocks})
           └─ 响应后更新 session list / selectedChatId
```

### Agent 选择器完成流程（`completeNewChatFlow`）

```
用户选择 agent
  ├─ newChatFlowGuardRef 互斥检查（防并发）
  ├─ service.createSession(agentType, title)
  │   ├─ 服务端创建 session → 返回 {session, configOptions}
  │   ├─ setChatSessions 合并新 session
  │   └─ setSelectedChatId(sessionId)
  ├─ setNewChatAgentPickerOpen(false)
  ├─ setPendingNewChatDraft(null)
  └─ 若 draft 有内容：
       └─ service.sendSessionMessage({sessionId, text, blocks})
```

### 配置选项变更流程

```
用户选择 config option
  ├─ handleChatConfigOptionChange(option, value)
  ├─ setChatConfigUpdatingKey(sessionId:configId)
  ├─ 显示 "Applying config..." 反馈
  ├─ service.setSessionConfig({sessionId, configId, value})
  ├─ 成功：applyChatSessionConfigOptions(sessionId, result.configOptions)
  │   └─ 合并到 chatSessions 中对应 session 的 configOptions
  ├─ 失败：显示 "Config update failed: ..." 错误反馈
  └─ 清除 updatingKey
```

## 10. Prompt/Turn 生命周期

### 10.1 时序

```
prompt_request  ──────────────────────────────> prompt_done
  (promptIndex=N)                                  (stopReason)
     │                                                │
     ├─ user_message_chunk (turn 1)                   │
     ├─ agent_thought_chunk (turn 2)                  │
     ├─ tool_call (turn 3) ── status running ── done  │
     ├─ agent_plan (turn 4)                           │
     └─ agent_message_chunk (turn 5)                  │
```

### 10.2 索引

- `promptIndex`：每次用户发起 prompt 递增（从 1 开始）。
- `turnIndex`：每个 prompt 内单调递增（从 1 开始，`tool_call` 和同一 `agent_thought_chunk` 会合并 turn）。
- `tool_call` 合并规则：同一 `ToolCallID` 的多次更新共用一个 `turnIndex`（通过 `turnIndexByKey` 映射）。
- `agent_message_chunk` / `agent_thought_chunk` 合并规则：同一 turn 内的连续相同类型消息拼接文本。
- `agent_message_chunk` / `agent_thought_chunk` 封口规则：当下一个不同 turn 或 `prompt_done` 到达时，服务端重发同一 `(promptIndex, turnIndex)` 的完整文本，并在 envelope 上设置 `finished: true`。

### 10.3 持久化

- `prompt_request` 到达时：创建 `sessionPromptState`，记录 `SessionPromptRecord` 元数据（status=started）。
- `prompt_done` 到达时：先写入 `session-history/<project-key>/<session-id>/prompts/p000001.json` prompt 快照，保存最终 prompt 元数据，再发布 `prompt_done` turn。
- 启动下一个 prompt 前，如果上一个 prompt 仍未终止，服务端合成 stop reason 为 `interrupted` 的 `prompt_done`。
- 启动时：旧 `session_prompts.turns_json` 全量导出一次到 prompt 文件。运行期 finished prompt 正文从文件读取，active prompt turns 从内存读取。

## 11. 兼容性说明

- 本文档为 app chat 当前实现的契约。
- 旧的 `chat.session.*` 格式不属于此协议流。
- `session.markRead` 路由存在但**未实现**，不可依赖。
- `chat.send` 路由存在但走 IM app channel（slash command 路径），**非** app chat UI 使用。
- 任何协议变更需同步更新：
  - 后端 recorder/event 载荷
  - 前端解码与合并逻辑
  - 本文档

## 12. 相关文档

- [registry-protocol.md](./registry-protocol.md) — Registry Protocol 2.2 完整协议（握手认证、路由、fs/git API、同步策略）
- [session-recorder-record-event.md](./session-recorder-record-event.md) — `SessionRecorder.RecordEvent` 内部实现分析（Go 服务端）
- [global-protocol.md](./global-protocol.md) — Global Protocol 封装设计（路由、追踪、ACP 透传）
- [session-persistence-sqlite.md](./session-persistence-sqlite.md) — SQLite 会话持久化 schema
- [app-chat-recorder-sync-protocol.md](./app-chat-recorder-sync-protocol.md) — 英文版本
