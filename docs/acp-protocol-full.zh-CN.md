# ACP 完整协议文档（整理版）

> 目标：提供一份可直接查阅的 ACP（Agent Client Protocol）完整中文整理。
> 范围：以官方稳定 schema 为准，补充官方 protocol 页面说明，并单列 unstable 草案扩展。
> 日期：2026-03-14

## 1. 规范边界与优先级

1. 最高权威：`schema/schema.json`（稳定协议，机器可校验）
2. 说明文档：`/protocol/*` 页面（概念、流程、实现建议）
3. 草案扩展：`schema/schema.unstable.json` + `/protocol/draft/*`（不保证兼容）

实现建议：
- 生产端只承诺稳定 schema。
- unstable 字段和方法必须走 feature flag，并在握手里显式声明。

## 2. 协议模型（谁调用谁）

ACP 基于 JSON-RPC 2.0，双向调用：

1. Client -> Agent（客户端请求 Agent）
- 会话管理、prompt 执行、模式切换、配置修改等。

2. Agent -> Client（Agent 回调客户端能力）
- 文件读写、终端执行、权限请求、会话流式更新。

3. Notification（无响应）
- 核心通知：`session/update`（Agent -> Client）、`session/cancel`（Client -> Agent）

## 3. JSON-RPC 消息结构

### 3.1 Request

```json
{
  "id": 1,
  "method": "session/new",
  "params": { "cwd": "D:/Code", "mcpServers": [] }
}
```

### 3.2 Response (result)

```json
{
  "id": 1,
  "result": { "sessionId": "..." }
}
```

### 3.3 Response (error)

```json
{
  "id": 1,
  "error": { "code": -32602, "message": "Invalid params" }
}
```

### 3.4 Notification

```json
{
  "method": "session/update",
  "params": { "sessionId": "...", "update": { "sessionUpdate": "agent_message_chunk", "content": { "type": "text", "text": "..." } } }
}
```

## 4. 生命周期（标准时序）

1. `initialize`
- Client 发送 `protocolVersion` + `clientCapabilities` + `clientInfo`
- Agent 返回 `protocolVersion` + `agentCapabilities` + 可选 `authMethods`

2. （可选）`authenticate`
- 如果 Agent 要求认证，则进入认证流程。

3. 会话建立
- `session/new` 或 `session/load`
- 返回 `sessionId`（new）以及可用 `modes`、`configOptions`

4. 交互回合
- Client 调 `session/prompt`
- Agent 持续发 `session/update`（文本流、tool call、计划等）
- 回合结束时 `session/prompt` 返回 `stopReason`

5. 取消
- Client 发 `session/cancel`
- Agent 应尽快停止并让 prompt 以 `stopReason=cancelled` 返回

## 5. 稳定版方法总表（schema.json）

## 5.1 Client -> Agent

1. `initialize`
- request 必填：`protocolVersion`
- request 常用：`clientCapabilities`, `clientInfo`
- response 常用：`agentCapabilities`, `agentInfo`, `authMethods`, `protocolVersion`

2. `authenticate`
- request 必填：`methodId`
- response：按认证方式返回结果

3. `session/new`
- request 必填：`cwd`, `mcpServers`
- response 必填：`sessionId`
- response 可选：`modes`, `configOptions`

4. `session/load`
- request 必填：`sessionId`, `cwd`, `mcpServers`
- response 可选：`modes`, `configOptions`

5. `session/list`
- request 可选：`cursor`, `cwd`
- response 必填：`sessions`
- response 可选：`nextCursor`

6. `session/prompt`
- request 必填：`sessionId`, `prompt`
- response 必填：`stopReason`

7. `session/set_mode`
- request 必填：`sessionId`, `modeId`

8. `session/set_config_option`
- request 必填：`sessionId`, `configId`, `value`
- response 必填：`configOptions`

9. `session/cancel`（notification）
- params 必填：`sessionId`

## 5.2 Agent -> Client

1. `session/update`（notification）
- params 必填：`sessionId`, `update`

2. `session/request_permission`
- request 必填：`sessionId`, `toolCall`, `options`
- response 必填：`outcome`

3. `fs/read_text_file`
- request 必填：`sessionId`, `path`
- response 必填：`content`

4. `fs/write_text_file`
- request 必填：`sessionId`, `path`, `content`

5. `terminal/create`
- request 必填：`sessionId`, `command`
- 可选：`args`, `cwd`, `env`, `outputByteLimit`
- response 必填：`terminalId`

6. `terminal/output`
- request 必填：`sessionId`, `terminalId`
- response 必填：`output`, `truncated`
- response 可选：`exitStatus`

7. `terminal/wait_for_exit`
- request 必填：`sessionId`, `terminalId`
- response：`exitCode`, `signal`（可选）

8. `terminal/kill`
- request 必填：`sessionId`, `terminalId`

9. `terminal/release`
- request 必填：`sessionId`, `terminalId`

## 6. `session/update` 更新类型（稳定）

`SessionUpdate.sessionUpdate` 可取：

1. `user_message_chunk`
2. `agent_message_chunk`
3. `agent_thought_chunk`
4. `tool_call`
5. `tool_call_update`
6. `plan`
7. `available_commands_update`
8. `current_mode_update`
9. `config_option_update`
10. `session_info_update`

## 7. 内容块（ContentBlock）

`ContentBlock.type`：

1. `text`
2. `image`（需 Agent 声明 image capability）
3. `audio`（需 Agent 声明 audio capability）
4. `resource_link`
5. `resource`（嵌入资源，需 embeddedContext capability）

## 8. Tool Call 与权限

### 8.1 ToolCall 状态

`ToolCallStatus`：
1. `pending`
2. `in_progress`
3. `completed`
4. `failed`

### 8.2 权限选项类型

`PermissionOptionKind`：
1. `allow_once`
2. `allow_always`
3. `reject_once`
4. `reject_always`

### 8.3 权限响应结果

`RequestPermissionOutcome`：
1. `outcome=selected` + `optionId`
2. `outcome=cancelled`

## 9. 回合停止原因（Prompt stopReason）

`StopReason`：
1. `end_turn`
2. `max_tokens`
3. `max_turn_requests`
4. `refusal`
5. `cancelled`

## 10. 能力协商（Capabilities）

### 10.1 ClientCapabilities

1. `fs.readTextFile`（bool）
2. `fs.writeTextFile`（bool）
3. `terminal`（bool）

### 10.2 AgentCapabilities

1. `loadSession`（bool）
2. `mcpCapabilities.http/sse`（bool）
3. `promptCapabilities.image/audio/embeddedContext`（bool）
4. `sessionCapabilities.list`（可选对象）

## 11. MCP Server 配置（session/new & session/load）

`mcpServers[]` 支持：

1. `stdio`
- `name`, `command`, `args[]`, `env[]`

2. `http`
- `type=http`, `name`, `url`, `headers[]`

3. `sse`
- `type=sse`, `name`, `url`, `headers[]`

说明：`stdio` 为 baseline；`http/sse` 需 Agent 能力声明支持。

## 12. 错误码（ErrorCode）

标准 JSON-RPC + ACP 扩展：

1. `-32700` Parse error
2. `-32600` Invalid request
3. `-32601` Method not found
4. `-32602` Invalid params
5. `-32603` Internal error
6. `-32000` Authentication required
7. `-32002` Resource not found
8. 其他 `int32` 为实现自定义错误

## 13. 传输与扩展

1. 传输：官方协议支持在 `Transports` 中定义的承载方式（实现通常使用 stdio）
2. 扩展方法：通过 `ExtRequest/ExtResponse/ExtNotification`
3. 元数据扩展：所有对象可带 `_meta`，但实现不得假设未知键语义

## 14. unstable 草案差异（schema.unstable.json）

相对稳定版新增/扩展（截至 2026-03-14）：

1. 新增会话方法
- `session/close`
- `session/fork`
- `session/resume`
- `session/set_model`

2. 协议级取消通知
- `$/cancel_request`

说明：上述均为草案能力，不应默认作为稳定兼容承诺。

## 15. 官方完整链接目录

### 15.1 协议说明页

1. Overview  
https://agentclientprotocol.com/protocol/overview
2. Initialization  
https://agentclientprotocol.com/protocol/initialization
3. Session Setup  
https://agentclientprotocol.com/protocol/session-setup
4. Session List  
https://agentclientprotocol.com/protocol/session-list
5. Prompt Turn  
https://agentclientprotocol.com/protocol/prompt-turn
6. Content  
https://agentclientprotocol.com/protocol/content
7. Tool Calls  
https://agentclientprotocol.com/protocol/tool-calls
8. File System  
https://agentclientprotocol.com/protocol/file-system
9. Terminals  
https://agentclientprotocol.com/protocol/terminals
10. Agent Plan  
https://agentclientprotocol.com/protocol/agent-plan
11. Session Modes  
https://agentclientprotocol.com/protocol/session-modes
12. Session Config Options  
https://agentclientprotocol.com/protocol/session-config-options
13. Slash Commands  
https://agentclientprotocol.com/protocol/slash-commands
14. Extensibility  
https://agentclientprotocol.com/protocol/extensibility
15. Transports  
https://agentclientprotocol.com/protocol/transports

### 15.2 Schema（机器可读）

1. 稳定版 schema  
https://raw.githubusercontent.com/agentclientprotocol/agent-client-protocol/main/schema/schema.json
2. unstable schema  
https://raw.githubusercontent.com/agentclientprotocol/agent-client-protocol/main/schema/schema.unstable.json

### 15.3 草案与提案

1. draft/cancellation  
https://agentclientprotocol.com/protocol/draft/cancellation
2. draft/schema  
https://agentclientprotocol.com/protocol/draft/schema
3. RFDs  
https://agentclientprotocol.com/rfds

---

## 16. 落地实现建议（简版）

1. 把 `initialize` 返回的 capability 作为唯一开关源，不要硬编码假设。
2. `session/prompt` 必须是“请求-流式通知-最终 stopReason”三段式。
3. 权限请求必须支持 `cancelled` 结果，避免悬挂请求。
4. 所有对象保留 `_meta` 原样透传，兼容未来扩展。
5. stable 与 unstable 的处理路径分离，便于升级和回滚。
