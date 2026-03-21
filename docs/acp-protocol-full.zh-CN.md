# ACP 协议文档（v1，稳定版）

> 基于 ACP（Agent Client Protocol）官方规范，面向快速实现的中文参考。
> 废弃 API 与 unstable 草案见附录；官方链接见附录 C。
> 更新：2026-03-21

---

## 目录

1. [概述与方法清单](#1-概述与方法清单)
2. [消息格式（JSON-RPC 2.0）](#2-消息格式json-rpc-20)
3. [初始化](#3-初始化)
4. [会话管理](#4-会话管理)
5. [Prompt 回合](#5-prompt-回合)
6. [内容块（ContentBlock）](#6-内容块contentblock)
7. [工具调用](#7-工具调用)
8. [文件系统](#8-文件系统)
9. [终端](#9-终端)
10. [会话配置项（Session Config Options）](#10-会话配置项session-config-options)
11. [斜线命令](#11-斜线命令)
12. [Agent 计划](#12-agent-计划)
13. [扩展性](#13-扩展性)
14. [传输层](#14-传输层)
15. [错误码](#15-错误码)
16. [实现要点速查](#16-实现要点速查)

附录 A：[废弃 API（Session Modes / SSE）](#附录-a废弃-api)
附录 B：[Unstable 草案](#附录-bunstable-草案)
附录 C：[官方文档链接](#附录-c官方文档链接)

---

## 1. 概述与方法清单

**角色：**
- **Agent**：使用生成式 AI 自主操作代码的程序，通常作为 Client 的子进程运行。
- **Client**：提供用户与 Agent 接口的程序（IDE/编辑器），管理环境、处理用户交互、控制资源访问。

**通信：** JSON-RPC 2.0，双向调用，支持请求-响应（方法）和单向通知。

**全局约束：所有文件路径必须是绝对路径；行号从 1 开始（1-based）。**

### Agent 方法（Client → Agent）

| 方法 | 说明 | 前置能力 |
|------|------|----------|
| `initialize` | 版本与能力协商 | — |
| `authenticate` | 认证 | — |
| `session/new` | 创建会话 | — |
| `session/prompt` | 发送 prompt | — |
| `session/load` | 加载已有会话 | `agentCapabilities.loadSession` |
| `session/list` | 列举会话 | `agentCapabilities.sessionCapabilities.list` |
| `session/set_config_option` | 修改配置项 | — |
| `session/cancel` | 取消当前操作（通知，无响应） | — |
| `session/set_mode` | 切换模式（**已废弃**，见附录 A） | — |

### Client 方法（Agent → Client）

| 方法 | 说明 | 前置能力 |
|------|------|----------|
| `session/request_permission` | 请求用户授权工具调用 | — |
| `fs/read_text_file` | 读取文件 | `clientCapabilities.fs.readTextFile` |
| `fs/write_text_file` | 写入文件 | `clientCapabilities.fs.writeTextFile` |
| `terminal/create` | 创建终端 | `clientCapabilities.terminal` |
| `terminal/output` | 获取终端输出 | `clientCapabilities.terminal` |
| `terminal/wait_for_exit` | 等待命令退出 | `clientCapabilities.terminal` |
| `terminal/kill` | 终止命令 | `clientCapabilities.terminal` |
| `terminal/release` | 释放终端资源 | `clientCapabilities.terminal` |
| `session/update` | 会话流式更新（通知，无响应） | — |

### 基线要求（所有实现必须支持）

- **Agent**：`session/new`、`session/prompt`、`session/cancel`、`session/update`、`ContentBlock::Text`、`ContentBlock::ResourceLink`、stdio MCP 传输
- **Client**：`session/request_permission`

---

## 2. 消息格式（JSON-RPC 2.0）

所有消息必须 UTF-8 编码，以换行符 `\n` 分隔，消息内**必须不**包含嵌入换行。

```jsonc
// Request
{ "jsonrpc": "2.0", "id": 1, "method": "session/new", "params": { ... } }

// Success Response
{ "jsonrpc": "2.0", "id": 1, "result": { ... } }

// Error Response
{ "jsonrpc": "2.0", "id": 1, "error": { "code": -32602, "message": "Invalid params" } }

// Notification（无 id，无响应）
{ "jsonrpc": "2.0", "method": "session/update", "params": { ... } }
```

---

## 3. 初始化

`initialize` **必须** 在任何 `session/*` 方法之前完成。

### 3.1 initialize 请求（Client → Agent）

**必填：** `protocolVersion`（Client 支持的最新 MAJOR 版本）、`clientCapabilities`  
**应提供：** `clientInfo`

```json
{
  "jsonrpc": "2.0", "id": 0, "method": "initialize",
  "params": {
    "protocolVersion": 1,
    "clientCapabilities": {
      "fs": { "readTextFile": true, "writeTextFile": true },
      "terminal": true
    },
    "clientInfo": { "name": "my-client", "title": "My Client", "version": "1.0.0" }
  }
}
```

### 3.2 initialize 响应（Agent → Client）

**必填：** `protocolVersion`（选定版本）、`agentCapabilities`  
**应提供：** `agentInfo`

```json
{
  "jsonrpc": "2.0", "id": 0,
  "result": {
    "protocolVersion": 1,
    "agentCapabilities": {
      "loadSession": true,
      "promptCapabilities": { "image": true, "audio": true, "embeddedContext": true },
      "mcpCapabilities": { "http": true },
      "sessionCapabilities": { "list": {} }
    },
    "agentInfo": { "name": "my-agent", "title": "My Agent", "version": "1.0.0" },
    "authMethods": []
  }
}
```

### 3.3 能力表

**Client Capabilities：**

| 字段 | 说明 |
|------|------|
| `fs.readTextFile` | 解锁 `fs/read_text_file` |
| `fs.writeTextFile` | 解锁 `fs/write_text_file` |
| `terminal` | 解锁所有 `terminal/*` |

**Agent Capabilities：**

| 字段 | 默认值 | 说明 |
|------|--------|------|
| `loadSession` | false | 解锁 `session/load` |
| `promptCapabilities.image` | false | prompt 可含 Image 块 |
| `promptCapabilities.audio` | false | prompt 可含 Audio 块 |
| `promptCapabilities.embeddedContext` | false | prompt 可含嵌入 Resource 块 |
| `mcpCapabilities.http` | false | 支持 HTTP MCP 服务器 |
| `mcpCapabilities.sse` | false | 已废弃，见附录 A |
| `sessionCapabilities.list` | 不存在 | 解锁 `session/list` |

### 3.4 版本协商与能力规则

- `protocolVersion` 是单整数 MAJOR 版本，仅在破坏性变更时递增。
- Agent 支持请求的版本时 **必须** 以相同版本响应；否则以自身最新版本响应。
- Client 若不支持 Agent 响应的版本，**应该** 关闭连接并通知用户。
- 所有省略的 capabilities **必须** 被视为不支持。

`clientInfo` / `agentInfo` 字段：`name`（逻辑名）、`title`（UI 展示，优先级高于 `name`）、`version`。

---

## 4. 会话管理

初始化完成后，Client **必须** 先建立会话再发送 prompt。

### 4.1 创建会话（session/new）

```json
{
  "jsonrpc": "2.0", "id": 1, "method": "session/new",
  "params": {
    "cwd": "/home/user/project",
    "mcpServers": [
      { "name": "filesystem", "command": "/path/to/mcp-server", "args": ["--stdio"], "env": [] }
    ]
  }
}
```

响应：

```json
{ "result": { "sessionId": "sess_abc123" } }
```

可选响应字段：`configOptions`（见第 10 节）、`modes`（已废弃，见附录 A）。

`cwd` 必须是绝对路径，**必须** 作为会话文件操作边界。

### 4.2 MCP Servers

所有 Agent **必须** 支持 stdio 传输；HTTP 传输需 `mcpCapabilities.http`。

**stdio：**
```json
{ "name": "...", "command": "/abs/path/mcp-server", "args": ["--stdio"], "env": [{ "name": "KEY", "value": "val" }] }
```

**HTTP（需 `mcpCapabilities.http`）：**
```json
{ "type": "http", "name": "...", "url": "https://...", "headers": [{ "name": "Authorization", "value": "Bearer ..." }] }
```

SSE 传输已废弃，见附录 A。

### 4.3 加载已有会话（session/load）

**前置：** `agentCapabilities.loadSession: true`，否则 **必须不** 调用。

```json
{
  "jsonrpc": "2.0", "id": 1, "method": "session/load",
  "params": { "sessionId": "sess_789", "cwd": "/home/user/project", "mcpServers": [] }
}
```

流程：Agent **必须** 先通过多条 `session/update` 通知将完整对话历史重播给 Client，全部发送完毕后响应 `null`。之后 Client 可继续发送 prompt。

```
Agent                    Client
  |   session/load         |
  |<-----------------------|
  |  session/update (历史) |
  |----------------------->|（多条，重播历史）
  |  session/load response |
  |----------------------->|（null）
  |   (Ready for prompts)  |
```

### 4.4 列举会话（session/list）

**前置：** `agentCapabilities.sessionCapabilities.list` 存在，否则 **必须不** 调用。

```json
{
  "method": "session/list",
  "params": { "cwd": "/home/user/project", "cursor": "opaque-token" }
}
```

响应：

```json
{
  "result": {
    "sessions": [
      { "sessionId": "sess_abc", "cwd": "/...", "title": "Implement auth", "updatedAt": "2025-10-29T14:22:15Z" }
    ],
    "nextCursor": "next-opaque-token"
  }
}
```

- `cwd`（可选，按工作目录过滤）、`cursor`（可选分页，不透明令牌，**必须不** 解析或持久化）
- `nextCursor` 缺失表示无更多结果

`SessionInfo` 字段：`sessionId`（必填）、`cwd`（必填）、`title?`、`updatedAt?`（ISO 8601）、`_meta?`

Agent 可通过 `session_info_update` 通知实时推送会话元数据变更：

```json
{ "sessionUpdate": "session_info_update", "title": "Implement user authentication", "updatedAt": "2025-10-29T15:00:00Z" }
```

### 4.5 取消（session/cancel，通知）

```json
{ "method": "session/cancel", "params": { "sessionId": "sess_abc" } }
```

- Client **应该** 预先标记所有未完成工具调用为 `cancelled`
- Client **必须** 以 `cancelled` 响应所有 pending 的权限请求
- Agent **应该** 尽快停止所有 LLM 请求和工具调用
- Agent **必须** 以 `cancelled` stopReason 响应原 `session/prompt` 请求，**不得** 返回 error 响应
- Agent **可以** 在收到 cancel 后继续发送 `session/update`，但 **必须** 在响应 prompt 之前完成

---

## 5. Prompt 回合

### 5.1 发送 Prompt（session/prompt）

```json
{
  "jsonrpc": "2.0", "id": 2, "method": "session/prompt",
  "params": {
    "sessionId": "sess_abc",
    "prompt": [
      { "type": "text", "text": "分析这段代码" },
      { "type": "resource_link", "uri": "file:///src/main.py", "name": "main.py" }
    ]
  }
}
```

内容类型需符合 Prompt Capabilities（见第 6 节）。

### 5.2 回合生命周期

```
Agent                                    Client
  |        session/prompt (用户消息)       |
  |<---------------------------------------|
  |   session/update (plan?)               |
  |--------------------------------------->|
  |   session/update (agent_message_chunk) |（流式文本）
  |--------------------------------------->|
  |   session/update (tool_call)           |
  |--------------------------------------->|
  |     session/request_permission?        |（可选）
  |--------------------------------------->|
  |              permission response       |
  |<---------------------------------------|
  |   session/update (tool_call_update)    |（in_progress / completed）
  |--------------------------------------->|
  |             [用户取消时]               |
  |              session/cancel            |
  |<---------------------------------------|
  |   session/prompt response (cancelled)  |
  |--------------------------------------->|
  |                                        |
  |   session/prompt response (stopReason) |（正常结束）
  |--------------------------------------->|
```

### 5.3 流式更新（session/update 通知）

| `sessionUpdate` 值 | 说明 |
|-------------------|------|
| `agent_message_chunk` | 文本块，含 `content: ContentBlock` |
| `tool_call` | 新工具调用（见第 7 节） |
| `tool_call_update` | 工具调用状态/内容更新 |
| `plan` | 完整计划列表（见第 12 节） |
| `available_commands_update` | 可用斜线命令列表（见第 11 节） |
| `config_option_update` | 配置项列表变更（见第 10 节） |
| `session_info_update` | 会话元数据更新（`title`、`updatedAt`） |
| `current_mode_update` | 模式变更（**已废弃**，见附录 A） |

### 5.4 回合结束（session/prompt 响应）

Agent **必须** 在所有 `session/update` 发送完毕后响应：

```json
{ "result": { "stopReason": "end_turn" } }
```

**StopReason 枚举：**

| 值 | 说明 |
|----|------|
| `end_turn` | LLM 完成响应 |
| `max_tokens` | 达到 token 上限 |
| `max_turn_requests` | 单回合 LLM 请求次数超限 |
| `refusal` | Agent 拒绝继续 |
| `cancelled` | Client 发送了 cancel |

### 5.5 权限请求（session/request_permission，Agent → Client）

Agent 在执行工具前 **可以** 请求权限：

```json
{
  "jsonrpc": "2.0", "id": 5, "method": "session/request_permission",
  "params": {
    "sessionId": "sess_abc",
    "toolCall": { "toolCallId": "call_001", "title": "Write config.json", "status": "pending" },
    "options": [
      { "optionId": "allow-once", "name": "允许本次", "kind": "allow_once" },
      { "optionId": "reject", "name": "拒绝", "kind": "reject_once" }
    ]
  }
}
```

Client 响应（用户选择）：
```json
{ "result": { "outcome": { "outcome": "selected", "optionId": "allow-once" } } }
```

Client 响应（回合被取消）：
```json
{ "result": { "outcome": { "outcome": "cancelled" } } }
```

**PermissionOptionKind：** `allow_once`、`allow_always`、`reject_once`、`reject_always`

Client 可根据用户设置自动响应权限请求。回合被取消时，Client **必须** 以 `cancelled` 响应所有 pending 权限请求。

---

## 6. 内容块（ContentBlock）

内容块出现在：`session/prompt` 的 `prompt` 字段、`session/update` 通知、工具调用内容中。

### Text（所有 Agent 必须支持）

```json
{ "type": "text", "text": "...", "annotations?": {} }
```

### Image（需 `promptCapabilities.image`）

```json
{ "type": "image", "mimeType": "image/png", "data": "<base64>", "uri?": "...", "annotations?": {} }
```

### Audio（需 `promptCapabilities.audio`）

```json
{ "type": "audio", "mimeType": "audio/wav", "data": "<base64>", "annotations?": {} }
```

### Resource（嵌入资源，需 `promptCapabilities.embeddedContext`）

在 prompt 中嵌入文件内容的**首选**方式（适用于 Agent 无法直接访问的来源）：

```json
{
  "type": "resource",
  "resource": {
    "uri": "file:///src/main.py",
    "mimeType": "text/x-python",
    "text": "def hello(): ..."
  }
}
```

`resource` 字段为 Text Resource（含 `text`）或 Blob Resource（含 `blob: <base64>`）。

### ResourceLink（所有 Agent 必须支持）

Agent 可直接访问引用资源（无需内容嵌入）：

```json
{
  "type": "resource_link",
  "uri": "file:///doc.pdf",
  "name": "doc.pdf",
  "mimeType?": "application/pdf",
  "title?": "...",
  "description?": "...",
  "size?": 1024000
}
```

---

## 7. 工具调用

工具调用通过 `session/update` 通知报告，生命周期：**`pending` → `in_progress` → `completed`/`failed`**（不跳过中间状态）。

### 7.1 创建工具调用（tool_call 通知）

```json
{
  "sessionUpdate": "tool_call",
  "toolCallId": "call_001",
  "title": "读取配置文件",
  "kind": "read",
  "status": "pending"
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `toolCallId` | string（必填） | 会话内唯一 ID |
| `title` | string（必填） | 人类可读描述 |
| `kind` | ToolKind（可选） | `read`/`write`/`execute`/`other`，供 Client 展示用 |
| `status` | ToolCallStatus（默认 `pending`） | 当前状态 |
| `content` | ToolCallContent[]（可选） | 产生的内容 |
| `locations` | `{ path, line? }`[]（可选） | 影响的文件位置 |
| `rawInput` | object（可选） | 发送给工具的原始输入 |
| `rawOutput` | object（可选） | 工具的原始输出 |

### 7.2 更新工具调用（tool_call_update 通知）

```json
{
  "sessionUpdate": "tool_call_update",
  "toolCallId": "call_001",
  "status": "completed",
  "content": [
    { "type": "content", "content": { "type": "text", "text": "分析完毕" } }
  ]
}
```

除 `toolCallId` 外所有字段均可选，只传变更字段。

**ToolCallStatus：** `pending`、`in_progress`、`completed`、`failed`

### 7.3 工具调用内容类型（ToolCallContent）

| type | 说明 | 关键字段 |
|------|------|----------|
| `content` | 常规内容块 | `content: ContentBlock` |
| `diff` | 文件差异 | `path`（绝对路径）、`oldText`（null=新文件）、`newText`（必填） |
| `terminal` | 终端引用 | `terminalId`（Client 实时显示输出） |

---

## 8. 文件系统

Agent **必须** 在使用前验证对应 capability 存在（`clientCapabilities.fs.*`）。

### fs/read_text_file

```json
{
  "method": "fs/read_text_file",
  "params": { "sessionId": "...", "path": "/abs/path/file.py", "line?": 10, "limit?": 50 }
}
```

响应：`{ "content": "..." }`（包含编辑器中未保存的更改）

### fs/write_text_file

```json
{
  "method": "fs/write_text_file",
  "params": { "sessionId": "...", "path": "/abs/path/file.json", "content": "..." }
}
```

响应：`null`。文件不存在时 Client **必须** 创建它。

---

## 9. 终端

Agent **必须** 在使用前验证 `clientCapabilities.terminal: true`。Agent **必须** 在不再需要时调用 `terminal/release`。

### terminal/create

```json
{
  "method": "terminal/create",
  "params": {
    "sessionId": "...", "command": "npm", "args": ["test"],
    "env?": [{ "name": "NODE_ENV", "value": "test" }],
    "cwd?": "/abs/path",
    "outputByteLimit?": 1048576
  }
}
```

立即返回（不等待命令完成）：`{ "terminalId": "term_xyz" }`

在工具调用中嵌入终端（Client 实时显示输出）：

```json
{
  "sessionUpdate": "tool_call",
  "toolCallId": "call_002", "title": "Running tests", "kind": "execute", "status": "in_progress",
  "content": [{ "type": "terminal", "terminalId": "term_xyz" }]
}
```

### terminal/output

获取当前输出（不等待命令完成）：

```json
{ "params": { "sessionId": "...", "terminalId": "..." } }
```

响应：`{ "output": "...", "truncated": false, "exitStatus?": { "exitCode": 0, "signal": null } }`

`exitStatus` 仅在命令已退出时存在。

### terminal/wait_for_exit

阻塞等待命令完成，返回：`{ "exitCode": 0, "signal": null }`

### terminal/kill

终止命令但不释放终端（仍可继续查询输出）。之后 **仍然必须** 调用 `terminal/release`。

### terminal/release

终止仍在运行的命令并释放所有资源。释放后终端 ID 对所有 `terminal/*` 无效；Client **应该** 在释放后继续显示已嵌入的终端输出。

**超时模式：**

```
1. terminal/create
2. 并发等待：超时计时器 OR terminal/wait_for_exit
3. 若计时器先到 → terminal/kill → terminal/output（获取最终输出）
4. 最后调用 terminal/release
```

---

## 10. 会话配置项（Session Config Options）

会话级配置的**首选方式**，取代废弃的 Session Modes。

### 10.1 初始状态

在 `session/new` 或 `session/load` 响应中返回：

```json
{
  "configOptions": [
    {
      "id": "mode", "name": "Session Mode", "category": "mode",
      "type": "select", "currentValue": "ask",
      "options": [
        { "value": "ask", "name": "Ask", "description": "变更前请求权限" },
        { "value": "code", "name": "Code", "description": "完整工具权限" }
      ]
    },
    {
      "id": "model", "name": "Model", "category": "model",
      "type": "select", "currentValue": "model-1",
      "options": [
        { "value": "model-1", "name": "Model 1", "description": "最快" },
        { "value": "model-2", "name": "Model 2", "description": "最强" }
      ]
    }
  ]
}
```

**ConfigOption 字段：**

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | string（必填） | 唯一标识符，`set_config_option` 时使用 |
| `name` | string（必填） | 人类可读标签 |
| `description?` | string | 可选说明 |
| `category?` | string | 语义分类（见下），仅供 UX 使用 |
| `type` | string（必填） | 目前仅支持 `select` |
| `currentValue` | string（必填） | 当前选中的值 |
| `options` | `{ value, name, description? }`[]（必填） | 可用值列表 |

**Category：** `mode`、`model`、`thought_level`；以 `_` 开头可自定义。  
**分类仅供 UX，不得作为功能依赖。** `configOptions` 数组顺序代表 Agent 优先级，Client **应该** 尊重。

### 10.2 Client 修改（session/set_config_option）

```json
{
  "method": "session/set_config_option",
  "params": { "sessionId": "...", "configId": "mode", "value": "code" }
}
```

Agent **必须** 响应**所有**配置项的完整列表（含更新后的 `currentValue`，允许反映级联变更）。

### 10.3 Agent 主动更新（config_option_update 通知）

```json
{ "sessionUpdate": "config_option_update", "configOptions": [/* 完整列表 */] }
```

Agent **必须** 始终为每个配置选项提供默认值，确保即使 Client 不支持 `configOptions` 也能正常运行。

---

## 11. 斜线命令

### 11.1 发布命令（available_commands_update 通知）

会话建立后 Agent **可以** 发送：

```json
{
  "sessionUpdate": "available_commands_update",
  "availableCommands": [
    { "name": "web", "description": "搜索网络", "input": { "hint": "查询词" } },
    { "name": "test", "description": "运行测试" }
  ]
}
```

Agent 可在会话期间任意时刻发送新列表来更新可用命令（增/删/改）。

### 11.2 调用命令

命令通过普通 prompt 消息调用，Client 在文本前加 `/` 前缀：

```json
{ "method": "session/prompt", "params": { "sessionId": "...", "prompt": [{ "type": "text", "text": "/web agent client protocol" }] } }
```

---

## 12. Agent 计划

### 12.1 发布/更新计划（plan 通知）

```json
{
  "sessionUpdate": "plan",
  "entries": [
    { "content": "分析代码结构", "priority": "high", "status": "pending" },
    { "content": "重构关键模块", "priority": "high", "status": "in_progress" },
    { "content": "补充单元测试", "priority": "medium", "status": "pending" }
  ]
}
```

**PlanEntry 字段：**

| 字段 | 类型 | 说明 |
|------|------|------|
| `content` | string（必填） | 人类可读描述 |
| `priority` | `high`/`medium`/`low`（必填） | 相对重要性 |
| `status` | `pending`/`in_progress`/`completed`（必填） | 当前执行状态 |

**Agent 必须** 每次发送**完整**条目列表，Client **必须** 完全替换当前计划（非增量合并）。计划可在执行过程中动态演变（增删改条目均可）。

---

## 13. 扩展性

### 13.1 `_meta` 字段

所有类型均含 `_meta: { [key: string]: unknown }`，用于附加自定义信息（请求/响应/通知/内容块/工具调用等均适用）。

实现者 **必须不** 在规范类型根级别添加任何自定义字段（所有名称均保留给协议未来版本）。

`_meta` 中 `traceparent`、`tracestate`、`baggage` 保留给 W3C Trace Context。

### 13.2 自定义方法

所有以 `_` 开头的方法名保留给自定义扩展：

```jsonc
// 自定义请求
{ "method": "_zed.dev/workspace/buffers", "params": { "language": "rust" } }

// 自定义通知
{ "method": "_zed.dev/file_opened", "params": { "path": "..." } }
```

接收端不识别时：请求 → 返回 `-32601 Method not found`；通知 → **应该** 忽略。

**发布自定义能力：** 在 `agentCapabilities._meta` 中声明，供双方在初始化期间协商：

```json
{ "agentCapabilities": { "_meta": { "zed.dev": { "workspace": true } } } }
```

---

## 14. 传输层

### stdio（推荐，所有实现应支持）

- Client 以子进程方式启动 Agent
- 消息通过 `stdin`/`stdout` 交换，`\n` 分隔，消息内**必须不**包含嵌入换行
- `stderr` 用于日志，Client 可捕获/转发/忽略
- `stdin`/`stdout` **必须不** 写入非 ACP 消息内容

### 自定义传输

协议传输无关，可在任意双向通信通道上实现，但 **必须** 保留 JSON-RPC 消息格式和生命周期要求。

---

## 15. 错误码

| 错误码 | 含义 |
|--------|------|
| `-32700` | Parse error |
| `-32600` | Invalid request |
| `-32601` | Method not found |
| `-32602` | Invalid params |
| `-32603` | Internal error |
| `-32000` | Authentication required |
| `-32002` | Resource not found |
| 其他 `int32` | 实现自定义错误 |

---

## 16. 实现要点速查

1. **Capability 驱动**：以 `initialize` 返回的 capability 为唯一开关源，省略的 capability 必须视为不支持，不得硬编码功能假设。
2. **三段式 Prompt 回合**：`session/prompt` 请求 → 多条 `session/update` 通知 → 最终含 `stopReason` 的响应。
3. **工具调用生命周期完整**：`pending` → `in_progress` → `completed`/`failed`，不跳过中间状态。
4. **取消时稳定返回**：Agent **必须** 捕获所有异常，以 `cancelled` stopReason 响应 prompt，不得返回 error 响应。
5. **权限请求支持 `cancelled`**：回合被取消时 Client **必须** 以 `cancelled` 响应所有 pending 权限请求，避免悬挂。
6. **计划全量替换**：每次 `plan` 通知发完整列表，Client 完全替换（非增量追加）。
7. **Session Config Options 优先**：`configOptions` 是首选 API；`modes`（废弃）仅为向后兼容，应保持两者同步。
8. **路径绝对、行号 1-based**。
9. **`_meta` 原样透传**，不过滤，预留给未来扩展。
10. **传输首选 stdio**；新 Agent 也应考虑支持 HTTP MCP 传输。

---

## 附录 A：废弃 API

### A.1 Session Modes（已被 Session Config Options 取代）

> `session/set_mode`、`modes` 字段、`current_mode_update` 通知将在协议未来版本中移除。向后兼容时可同时提供，但应保持与 `configOptions` 同步。支持 `configOptions` 的 Client **应该** 忽略 `modes`。

**session/new 响应中的 modes 字段：**

```json
{
  "modes": {
    "currentModeId": "ask",
    "availableModes": [
      { "id": "ask", "name": "Ask", "description": "变更前请求权限" },
      { "id": "code", "name": "Code", "description": "完整工具权限" }
    ]
  }
}
```

**session/set_mode（Client → Agent）：**
```json
{ "method": "session/set_mode", "params": { "sessionId": "...", "modeId": "code" } }
```

**current_mode_update（Agent → Client 通知）：**
```json
{ "sessionUpdate": "current_mode_update", "modeId": "code" }
```

### A.2 SSE MCP 传输（已被 HTTP 取代）

需 `mcpCapabilities.sse`（已废弃，MCP 规范已移除此传输方式，新实现应使用 HTTP）。

```json
{ "type": "sse", "name": "api", "url": "https://...", "headers": [] }
```

---

## 附录 B：Unstable 草案

> 截至 2026-03-21，均为草案，**不保证兼容性**，需通过 feature flag 控制并在握手时显式声明。

| 新增方法/通知 | 说明 |
|--------------|------|
| `session/close` | 关闭会话 |
| `session/fork` | 派生会话 |
| `session/resume` | 恢复会话 |
| `session/set_model` | 设置模型 |
| `$/cancel_request` | 协议级取消请求通知 |

---

## 附录 C：官方文档链接

| 类别 | URL |
|------|-----|
| 协议概述 | https://agentclientprotocol.com/protocol/overview |
| 各协议页面 | https://agentclientprotocol.com/protocol/{section} |
| 稳定 Schema | https://raw.githubusercontent.com/agentclientprotocol/agent-client-protocol/main/schema/schema.json |
| Unstable Schema | https://raw.githubusercontent.com/agentclientprotocol/agent-client-protocol/main/schema/schema.unstable.json |
