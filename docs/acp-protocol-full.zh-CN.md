# ACP 完整协议文档（整理版）

> 目标：提供一份可直接查阅的 ACP（Agent Client Protocol）完整中文整理。
> 范围：基于官方协议页面实际内容（https://agentclientprotocol.com/protocol/）进行全面整理。
> 日期：2026-03-15（根据官方页面最新内容全面更新）

---

## 目录

1. [规范边界与优先级](#1-规范边界与优先级)
2. [协议概述](#2-协议概述)
3. [JSON-RPC 消息结构](#3-json-rpc-消息结构)
4. [初始化（Initialization）](#4-初始化initialization)
5. [会话建立（Session Setup）](#5-会话建立session-setup)
6. [会话列表（Session List）](#6-会话列表session-list)
7. [Prompt 回合（Prompt Turn）](#7-prompt-回合prompt-turn)
8. [内容块（Content）](#8-内容块content)
9. [工具调用（Tool Calls）](#9-工具调用tool-calls)
10. [文件系统（File System）](#10-文件系统file-system)
11. [终端（Terminals）](#11-终端terminals)
12. [Agent 计划（Agent Plan）](#12-agent-计划agent-plan)
13. [会话模式（Session Modes）](#13-会话模式session-modes)
14. [会话配置项（Session Config Options）](#14-会话配置项session-config-options)
15. [斜线命令（Slash Commands）](#15-斜线命令slash-commands)
16. [扩展性（Extensibility）](#16-扩展性extensibility)
17. [传输层（Transports）](#17-传输层transports)
18. [错误码（Error Codes）](#18-错误码error-codes)
19. [unstable 草案差异](#19-unstable-草案差异)
20. [落地实现建议](#20-落地实现建议)

---

## 1. 规范边界与优先级

1. 最高权威：`schema/schema.json`（稳定协议，机器可校验）
2. 说明文档：`/protocol/*` 页面（概念、流程、实现建议）
3. 草案扩展：`schema/schema.unstable.json` + `/protocol/draft/*`（不保证兼容）

实现建议：
- 生产端只承诺稳定 schema。
- unstable 字段和方法必须走 feature flag，并在握手里显式声明。

---

## 2. 协议概述

### 2.1 角色定义

**Agent**：使用生成式 AI 自主操作代码的程序，通常作为 Client 的子进程运行。

**Client**：提供用户与 Agent 之间接口的程序，通常是代码编辑器（IDE、文本编辑器），负责管理环境、处理用户交互和控制资源访问。

### 2.2 通信模型

ACP 基于 [JSON-RPC 2.0](https://www.jsonrpc.org/specification) 规范，支持两类消息：

- **方法（Methods）**：请求-响应对，期望得到 result 或 error。
- **通知（Notifications）**：单向消息，不期望响应。

双向调用关系：

| 方向 | 描述 |
|------|------|
| Client → Agent | 会话管理、prompt 执行、模式切换、配置修改等 |
| Agent → Client | 文件读写、终端执行、权限请求、会话流式更新 |

### 2.3 典型消息流

```
1. 初始化阶段
   Client → Agent: initialize（建立连接，协商版本和能力）
   Client → Agent: authenticate（如果 Agent 要求认证）

2. 会话建立
   Client → Agent: session/new（创建新会话）
   或
   Client → Agent: session/load（恢复已有会话，若 Agent 支持）

3. Prompt 回合
   Client → Agent: session/prompt（发送用户消息）
   Agent → Client: session/update（流式进度更新通知）
   Agent → Client: 文件操作或权限请求（按需）
   Client → Agent: session/cancel（如需中断，可随时发送）
   回合结束时 Agent 返回包含 stopReason 的 session/prompt 响应
```

### 2.4 参数要求

- 协议中所有文件路径 **必须** 是绝对路径。
- 行号从 **1** 开始计数（1-based）。

### 2.5 Agent 方法汇总

#### 基础方法（所有 Agent 必须实现）

| 方法 | 描述 |
|------|------|
| `initialize` | 协商版本与交换能力 |
| `authenticate` | 向 Agent 认证（如果需要） |
| `session/new` | 创建新会话 |
| `session/prompt` | 向 Agent 发送用户 prompt |

#### 可选方法

| 方法 | 描述 | 前置能力 |
|------|------|----------|
| `session/load` | 加载已有会话 | `loadSession` capability |
| `session/list` | 列举已有会话 | `sessionCapabilities.list` capability |
| `session/set_mode` | 切换 Agent 操作模式 | — |
| `session/set_config_option` | 修改会话配置项 | — |

#### 通知（Client → Agent，无响应）

| 通知 | 描述 |
|------|------|
| `session/cancel` | 取消正在进行的操作 |

### 2.6 Client 方法汇总

#### 基础方法（所有 Client 必须实现）

| 方法 | 描述 |
|------|------|
| `session/request_permission` | 请求用户授权 tool call |

#### 可选方法

| 方法 | 描述 | 前置能力 |
|------|------|----------|
| `fs/read_text_file` | 读取文件内容 | `fs.readTextFile` capability |
| `fs/write_text_file` | 写入文件内容 | `fs.writeTextFile` capability |
| `terminal/create` | 创建新终端 | `terminal` capability |
| `terminal/output` | 获取终端输出和退出状态 | `terminal` capability |
| `terminal/wait_for_exit` | 等待终端命令完成 | `terminal` capability |
| `terminal/kill` | 终止终端命令 | `terminal` capability |
| `terminal/release` | 释放终端资源 | `terminal` capability |

#### 通知（Agent → Client，无响应）

| 通知 | 描述 |
|------|------|
| `session/update` | 发送会话更新（消息块、工具调用、计划、命令、模式变更） |

---

## 3. JSON-RPC 消息结构

### 3.1 Request（请求）

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "session/new",
  "params": { "cwd": "/home/user/project", "mcpServers": [] }
}
```

### 3.2 Response（成功响应）

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": { "sessionId": "sess_abc123def456" }
}
```

### 3.3 Response（错误响应）

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "error": { "code": -32602, "message": "Invalid params" }
}
```

### 3.4 Notification（通知，无响应）

```json
{
  "jsonrpc": "2.0",
  "method": "session/update",
  "params": {
    "sessionId": "sess_abc123def456",
    "update": {
      "sessionUpdate": "agent_message_chunk",
      "content": { "type": "text", "text": "..." }
    }
  }
}
```

---

## 4. 初始化（Initialization）

初始化阶段允许 Client 和 Agent 协商协议版本、能力和认证方式。在创建会话之前，Client **必须** 先完成初始化。

### 4.1 初始化流程时序

```
Agent           Client
  |               |
  |  initialize   |
  |<--------------|
  |               |
  |  initialize   |
  |   response    |
  |-------------->|
  |               |
  | (Ready for session setup)
```

### 4.2 initialize 请求

Client **必须** 发送：
- `protocolVersion`：Client 支持的最新协议版本
- `clientCapabilities`：Client 支持的能力

Client **应该** 提供：
- `clientInfo`：Client 名称和版本

```json
{
  "jsonrpc": "2.0",
  "id": 0,
  "method": "initialize",
  "params": {
    "protocolVersion": 1,
    "clientCapabilities": {
      "fs": {
        "readTextFile": true,
        "writeTextFile": true
      },
      "terminal": true
    },
    "clientInfo": {
      "name": "my-client",
      "title": "My Client",
      "version": "1.0.0"
    }
  }
}
```

### 4.3 initialize 响应

Agent **必须** 响应选定的协议版本和它支持的能力。Agent **应该** 也提供自身信息：

```json
{
  "jsonrpc": "2.0",
  "id": 0,
  "result": {
    "protocolVersion": 1,
    "agentCapabilities": {
      "loadSession": true,
      "promptCapabilities": {
        "image": true,
        "audio": true,
        "embeddedContext": true
      },
      "mcp": {
        "http": true,
        "sse": true
      }
    },
    "agentInfo": {
      "name": "my-agent",
      "title": "My Agent",
      "version": "1.0.0"
    },
    "authMethods": []
  }
}
```

### 4.4 协议版本协商

- 协议版本是单个整数（MAJOR 版本），仅在引入破坏性变更时递增。
- Client 在 `initialize` 请求中 **必须** 包含它支持的最新协议版本。
- 如果 Agent 支持请求的版本，它 **必须** 以相同版本响应；否则以它支持的最新版本响应。
- 如果 Client 不支持 Agent 响应中指定的版本，Client **应该** 关闭连接并通知用户。
- 新增 capabilities 不视为破坏性变更。

### 4.5 能力协商（Capabilities）

所有 capabilities 都是可选的。Client 和 Agent **必须** 将 `initialize` 请求中省略的所有 capabilities 视为不支持。

#### Client Capabilities

| 字段 | 类型 | 说明 |
|------|------|------|
| `fs.readTextFile` | boolean | `fs/read_text_file` 方法可用 |
| `fs.writeTextFile` | boolean | `fs/write_text_file` 方法可用 |
| `terminal` | boolean | 所有 `terminal/*` 方法可用 |

#### Agent Capabilities

| 字段 | 类型 | 说明 |
|------|------|------|
| `loadSession` | boolean（默认 false） | `session/load` 方法可用 |
| `promptCapabilities.image` | boolean（默认 false） | prompt 可包含 Image 内容块 |
| `promptCapabilities.audio` | boolean（默认 false） | prompt 可包含 Audio 内容块 |
| `promptCapabilities.embeddedContext` | boolean（默认 false） | prompt 可包含 Resource 内容块 |
| `mcpCapabilities.http` | boolean（默认 false） | 支持通过 HTTP 连接 MCP 服务器 |
| `mcpCapabilities.sse` | boolean（默认 false） | 支持通过 SSE 连接 MCP 服务器（已废弃） |
| `sessionCapabilities.list` | 对象（可选） | `session/list` 方法可用 |

#### 基线要求

所有 Agent **必须** 支持：
- `session/new`、`session/prompt`、`session/cancel`、`session/update`
- `ContentBlock::Text` 和 `ContentBlock::ResourceLink` 类型的 prompt 内容

### 4.6 实现信息（Implementation Information）

Client 和 Agent **应该** 在 `clientInfo` 和 `agentInfo` 字段中提供实现信息：

| 字段 | 说明 |
|------|------|
| `name` | 程序化/逻辑名称，无 `title` 时也可用于展示 |
| `title` | UI 展示用的人类可读名称（优先级高于 `name`） |
| `version` | 实现版本，可展示给用户或用于调试 |

---

## 5. 会话建立（Session Setup）

会话代表 Client 和 Agent 之间的一个对话或线程。每个会话维护自己的上下文、对话历史和状态，允许同一 Agent 进行多个独立交互。在创建会话之前，Client **必须** 先完成初始化阶段。

### 5.1 会话建立流程时序

`session/new` 时序：

```
Agent           Client
  |               |
  |  session/new  |
  |<--------------|
  |               |
  | Create session|
  | context       |
  | Connect MCP   |
  |               |
  | session/new   |
  | response      |
  | (sessionId)   |
  |-------------->|
  |               |
  | (Ready for prompts)
```

`session/load` 时序：

```
Agent           Client
  |               |
  | session/load  |
  |<--------------|
  |               |
  | Restore ctx   |
  | Connect MCP   |
  | Replay history|
  |               |
  | session/update|  (重播历史消息，多条)
  |-------------->|
  |               |
  | session/load  |
  | response      |
  |-------------->|
```

### 5.2 创建会话（session/new）

Client 通过调用 `session/new` 方法创建新会话，需提供：
- `cwd`：会话的工作目录
- `mcpServers`：Agent 应连接的 MCP 服务器列表

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "session/new",
  "params": {
    "cwd": "/home/user/project",
    "mcpServers": [
      {
        "name": "filesystem",
        "command": "/path/to/mcp-server",
        "args": ["--stdio"],
        "env": []
      }
    ]
  }
}
```

Agent **必须** 响应唯一的 Session ID：

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "sessionId": "sess_abc123def456"
  }
}
```

Agent 还可以在响应中返回可选字段：`modes`（模式列表）、`configOptions`（配置项列表）。

### 5.3 加载已有会话（session/load）

#### 检查支持

在尝试加载会话之前，Client **必须** 验证 Agent 是否支持此能力：

```json
{
  "jsonrpc": "2.0",
  "id": 0,
  "result": {
    "protocolVersion": 1,
    "agentCapabilities": {
      "loadSession": true
    }
  }
}
```

如果 `loadSession` 为 `false` 或不存在，Client **必须不** 尝试调用 `session/load`。

#### 加载请求

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "session/load",
  "params": {
    "sessionId": "sess_789xyz",
    "cwd": "/home/user/project",
    "mcpServers": [
      {
        "name": "filesystem",
        "command": "/path/to/mcp-server",
        "args": ["--mode", "filesystem"],
        "env": []
      }
    ]
  }
}
```

Agent **必须** 通过 `session/update` 通知将完整的对话历史重播给 Client（包括历史用户消息和 Agent 响应），历史内容全部流式发送后，Agent **必须** 响应原始 `session/load` 请求：

```json
{ "jsonrpc": "2.0", "id": 1, "result": null }
```

之后 Client 可以继续发送 prompt，如同会话从未中断。

### 5.4 Session ID

`session/new` 返回的 session ID 是对话上下文的唯一标识符。Client 使用此 ID 来：
- 通过 `session/prompt` 发送 prompt 请求
- 通过 `session/cancel` 取消正在进行的操作
- 通过 `session/load` 加载之前的会话（如果 Agent 支持 `loadSession` 能力）

### 5.5 工作目录（Working Directory）

`cwd` 参数为会话建立文件系统上下文：
- **必须** 是绝对路径
- 无论 Agent 子进程在哪里启动，**必须** 在会话中使用此目录
- **应该** 作为文件系统工具操作的边界

### 5.6 MCP Servers

MCP（Model Context Protocol）允许 Agent 访问外部工具和数据源。创建会话时，Client 可以包含 Agent 应连接的 MCP 服务器的连接详情。

所有 Agent **必须** 支持 stdio 传输；HTTP 和 SSE 传输是可选能力。新 Agent **应该** 支持 HTTP 传输。

#### stdio 传输（所有 Agent 必须支持）

| 参数 | 类型 | 是否必填 | 说明 |
|------|------|----------|------|
| `name` | string | 必填 | 服务器的人类可读标识符 |
| `command` | string | 必填 | MCP 服务器可执行文件的绝对路径 |
| `args` | array | 必填 | 传递给服务器的命令行参数 |
| `env` | EnvVariable[] | 可选 | 启动服务器时设置的环境变量 |

```json
{
  "name": "filesystem",
  "command": "/path/to/mcp-server",
  "args": ["--stdio"],
  "env": [{ "name": "API_KEY", "value": "secret123" }]
}
```

#### HTTP 传输（需要 `mcpCapabilities.http`）

| 参数 | 类型 | 是否必填 | 说明 |
|------|------|----------|------|
| `type` | string | 必填 | 必须为 `"http"` |
| `name` | string | 必填 | 服务器的人类可读标识符 |
| `url` | string | 必填 | MCP 服务器的 URL |
| `headers` | HttpHeader[] | 必填 | 请求中包含的 HTTP 头 |

```json
{
  "type": "http",
  "name": "api-server",
  "url": "https://api.example.com/mcp",
  "headers": [
    { "name": "Authorization", "value": "Bearer token123" }
  ]
}
```

#### SSE 传输（需要 `mcpCapabilities.sse`，已被 MCP 规范废弃）

| 参数 | 类型 | 是否必填 | 说明 |
|------|------|----------|------|
| `type` | string | 必填 | 必须为 `"sse"` |
| `name` | string | 必填 | 服务器的人类可读标识符 |
| `url` | string | 必填 | SSE 端点的 URL |
| `headers` | HttpHeader[] | 必填 | 建立 SSE 连接时包含的 HTTP 头 |

---

## 6. 会话列表（Session List）

### 6.1 检查支持

在尝试列出会话之前，Client **必须** 验证 Agent 是否支持此能力：

```json
{
  "agentCapabilities": {
    "sessionCapabilities": { "list": {} }
  }
}
```

如果 `sessionCapabilities.list` 不存在，Client **必须不** 尝试调用 `session/list`。

### 6.2 列举会话（session/list）

```json
{
  "jsonrpc": "2.0",
  "id": 2,
  "method": "session/list",
  "params": {
    "cwd": "/home/user/project",
    "cursor": "eyJwYWdlIjogMn0="
  }
}
```

| 参数 | 类型 | 说明 |
|------|------|------|
| `cwd` | string（可选） | 按工作目录过滤，必须是绝对路径 |
| `cursor` | string（可选） | 上一个响应 `nextCursor` 字段的游标令牌，用于分页 |

空 `params` 对象返回第一页会话。

```json
{
  "jsonrpc": "2.0",
  "id": 2,
  "result": {
    "sessions": [
      {
        "sessionId": "sess_abc123def456",
        "cwd": "/home/user/project",
        "title": "Implement session list API",
        "updatedAt": "2025-10-29T14:22:15Z"
      },
      {
        "sessionId": "sess_xyz789ghi012",
        "cwd": "/home/user/another-project",
        "title": "Debug authentication flow",
        "updatedAt": "2025-10-28T16:45:30Z"
      }
    ],
    "nextCursor": "eyJwYWdlIjogM30="
  }
}
```

`SessionInfo` 字段：

| 字段 | 类型 | 说明 |
|------|------|------|
| `sessionId` | string（必填） | 会话唯一 ID |
| `cwd` | string（必填） | 会话工作目录 |
| `title` | string（可选） | 会话的人类可读标题 |
| `updatedAt` | string（可选） | 最后活动时间（ISO 8601） |
| `_meta` | object（可选） | Agent 特定的扩展元数据 |

### 6.3 分页

`session/list` 使用基于游标（cursor-based）的分页：
- Client **必须** 将缺失的 `nextCursor` 视为结果结束
- Client **必须** 将游标视为不透明令牌，不得解析、修改或持久化
- Agent **应该** 在游标无效时返回错误
- Agent **应该** 在内部强制限制合理的页面大小

### 6.4 实时更新会话元数据

Agent 可以通过 `session_info_update` 通知实时更新会话元数据（不需要 Client 轮询）：

```json
{
  "jsonrpc": "2.0",
  "method": "session/update",
  "params": {
    "sessionId": "sess_abc123def456",
    "update": {
      "sessionUpdate": "session_info_update",
      "title": "Implement user authentication"
    }
  }
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `title` | string \| null（可选） | 会话标题，设为 `null` 可清除 |
| `updatedAt` | string \| null（可选） | 最后活动时间（ISO 8601），设为 `null` 可清除 |

`sessionId` 和 `cwd` 不包含在更新中（`sessionId` 已在通知的 `params` 中，`cwd` 在创建时设定且不可变）。

### 6.5 与其他会话方法的交互

`session/list` 仅是发现机制，不恢复或修改会话：
1. Client 调用 `session/list` 发现可用会话
2. 用户从列表中选择一个会话
3. Client 调用 `session/load` 并传入所选 `sessionId` 来恢复对话

---

## 7. Prompt 回合（Prompt Turn）

Prompt 回合代表 Client 和 Agent 之间的完整交互周期，从用户消息开始，直到 Agent 完成响应。这可能涉及与语言模型的多次交换和工具调用。

在发送 prompt 之前，Client **必须** 先完成初始化阶段和会话建立。

### 7.1 Prompt 回合生命周期

```
Agent                                    Client
  |                                        |
  |         session/prompt (user msg)      |
  |<---------------------------------------|
  |                                        |
  | Process with LLM                       |
  |                                        |
  |   session/update (plan)                |
  |--------------------------------------->|
  |                                        |
  |   session/update (agent_message_chunk) |
  |--------------------------------------->|
  |                                        |
  |   session/update (tool_call)           |
  |--------------------------------------->|
  |                                        |
  |     session/request_permission         |  [可选，需权限时]
  |--------------------------------------->|
  |                   Permission response  |
  |<---------------------------------------|
  |                                        |
  |   session/update (tool_call in_progr.) |
  |--------------------------------------->|
  |                                        |
  |   session/update (tool_call completed) |
  |--------------------------------------->|
  |                                        |
  |           [用户取消时]                  |
  |              session/cancel            |
  |<---------------------------------------|
  |   session/prompt resp (cancelled)      |
  |--------------------------------------->|
  |                                        |
  |   session/prompt response (stopReason) |
  |--------------------------------------->|
```

### 7.2 步骤详解

#### 步骤 1：发送用户消息

Client 发送 `session/prompt`：

```json
{
  "jsonrpc": "2.0",
  "id": 2,
  "method": "session/prompt",
  "params": {
    "sessionId": "sess_abc123def456",
    "prompt": [
      {
        "type": "text",
        "text": "Can you analyze this code for potential issues?"
      },
      {
        "type": "resource",
        "resource": {
          "uri": "file:///home/user/project/main.py",
          "mimeType": "text/x-python",
          "text": "def process_data(items):\n    for item in items:\n        print(item)"
        }
      }
    ]
  }
}
```

| 参数 | 类型 | 说明 |
|------|------|------|
| `sessionId` | SessionId（必填） | 要发送消息的会话 ID |
| `prompt` | ContentBlock[]（必填） | 用户消息内容（文本、图片、文件等） |

Client **必须** 根据初始化阶段建立的 Prompt Capabilities 限制内容类型。

#### 步骤 2：Agent 处理

Agent 接收到 prompt 请求后，将用户消息发送给语言模型，模型可能响应文本内容、工具调用或两者兼有。

#### 步骤 3：Agent 报告输出

Agent 通过 `session/update` 通知将模型的输出报告给 Client。

可选地报告执行计划：

```json
{
  "jsonrpc": "2.0",
  "method": "session/update",
  "params": {
    "sessionId": "sess_abc123def456",
    "update": {
      "sessionUpdate": "plan",
      "entries": [
        { "content": "Check for syntax errors", "priority": "high", "status": "pending" },
        { "content": "Identify potential type issues", "priority": "medium", "status": "pending" }
      ]
    }
  }
}
```

报告文本响应：

```json
{
  "jsonrpc": "2.0",
  "method": "session/update",
  "params": {
    "sessionId": "sess_abc123def456",
    "update": {
      "sessionUpdate": "agent_message_chunk",
      "content": { "type": "text", "text": "I'll analyze your code..." }
    }
  }
}
```

报告工具调用（pending 状态）：

```json
{
  "jsonrpc": "2.0",
  "method": "session/update",
  "params": {
    "sessionId": "sess_abc123def456",
    "update": {
      "sessionUpdate": "tool_call",
      "toolCallId": "call_001",
      "title": "Analyzing Python code",
      "kind": "other",
      "status": "pending"
    }
  }
}
```

#### 步骤 4：检查是否完成

如果没有待处理的工具调用，回合结束，Agent **必须** 响应原始 `session/prompt` 请求并附带 `StopReason`：

```json
{
  "jsonrpc": "2.0",
  "id": 2,
  "result": { "stopReason": "end_turn" }
}
```

Agent 可以在任何时候通过返回对应的 StopReason 来停止回合。

#### 步骤 5：工具调用与状态报告

在执行之前，Agent 可以通过 `session/request_permission` 向 Client 请求权限。

权限批准后，Agent **应该** 调用工具并报告状态更新（标记为 `in_progress`）：

```json
{
  "jsonrpc": "2.0",
  "method": "session/update",
  "params": {
    "sessionId": "sess_abc123def456",
    "update": {
      "sessionUpdate": "tool_call_update",
      "toolCallId": "call_001",
      "status": "in_progress"
    }
  }
}
```

工具完成后，发送包含最终状态和内容的更新：

```json
{
  "jsonrpc": "2.0",
  "method": "session/update",
  "params": {
    "sessionId": "sess_abc123def456",
    "update": {
      "sessionUpdate": "tool_call_update",
      "toolCallId": "call_001",
      "status": "completed",
      "content": [
        {
          "type": "content",
          "content": {
            "type": "text",
            "text": "Analysis complete:\n- No syntax errors found\n- Consider adding type hints"
          }
        }
      ]
    }
  }
}
```

#### 步骤 6：继续对话

Agent 将工具结果发送回语言模型作为新请求。循环回到步骤 2，持续直到语言模型完成响应（不再请求额外的工具调用）或回合被 Agent 停止 / Client 取消。

### 7.3 停止原因（StopReason）

| 值 | 说明 |
|----|------|
| `end_turn` | 语言模型完成响应，无需更多工具 |
| `max_tokens` | 达到最大 token 限制 |
| `max_turn_requests` | 单个回合内模型请求次数超过最大值 |
| `refusal` | Agent 拒绝继续 |
| `cancelled` | Client 取消了回合 |

### 7.4 取消（Cancellation）

Client 可以在任何时候通过发送 `session/cancel` 通知取消正在进行的 prompt 回合：

```json
{
  "jsonrpc": "2.0",
  "method": "session/cancel",
  "params": { "sessionId": "sess_abc123def456" }
}
```

**Client 的职责：**
- **应该** 在发送 `session/cancel` 后将当前回合所有未完成的工具调用预先标记为 `cancelled`
- **必须** 以 `cancelled` 结果响应所有待处理的 `session/request_permission` 请求
- **应该** 仍然接受在发送 `session/cancel` 之后收到的工具调用更新

**Agent 的职责：**
- **应该** 尽快停止所有语言模型请求和工具调用
- 所有正在进行的操作成功中止并发送待处理更新后，**必须** 以 `cancelled` 停止原因响应原始 `session/prompt` 请求
- **必须** 捕获所有异常并返回语义明确的 `cancelled` 停止原因（不应返回 error 响应）
- 在接收到 `session/cancel` 通知后，**可以** 继续发送 `session/update` 通知，但 **必须** 确保在响应 `session/prompt` 请求之前完成

---

## 8. 内容块（Content）

内容块（ContentBlock）代表通过 ACP 流动的可展示信息，与 MCP 使用相同的 `ContentBlock` 结构，使 Agent 能够无需转换地直接转发 MCP 工具输出的内容。

内容块出现在：
- 通过 `session/prompt` 发送的用户 prompt
- 通过 `session/update` 通知流式传输的语言模型输出
- 工具调用的进度更新和结果

### 8.1 文本内容（Text）

所有 Agent **必须** 支持文本内容块。

```json
{ "type": "text", "text": "What's the weather like today?" }
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `text` | string（必填） | 要显示的文本内容 |
| `annotations` | Annotations（可选） | 关于内容如何使用或展示的可选元数据 |

### 8.2 图片内容（Image）

需要 Agent 的 `image` prompt capability。

```json
{
  "type": "image",
  "mimeType": "image/png",
  "data": "iVBORw0KGgoAAAANSUhEUgAAAAEAAAAB..."
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `data` | string（必填） | Base64 编码的图片数据 |
| `mimeType` | string（必填） | 图片 MIME 类型（如 "image/png", "image/jpeg"） |
| `uri` | string（可选） | 图片来源的可选 URI 引用 |
| `annotations` | Annotations（可选） | 内容展示元数据 |

### 8.3 音频内容（Audio）

需要 Agent 的 `audio` prompt capability。

```json
{
  "type": "audio",
  "mimeType": "audio/wav",
  "data": "UklGRiQAAABXQVZFZm10IBAAAAABAAEAQB8AAAB..."
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `data` | string（必填） | Base64 编码的音频数据 |
| `mimeType` | string（必填） | 音频 MIME 类型（如 "audio/wav", "audio/mp3"） |
| `annotations` | Annotations（可选） | 内容展示元数据 |

### 8.4 嵌入资源（Embedded Resource）

需要 Agent 的 `embeddedContext` prompt capability。这是在 prompt 中包含上下文（如文件内容 @-mentions）的**首选**方式，Client 可以嵌入 Agent 无法直接访问的来源内容。

```json
{
  "type": "resource",
  "resource": {
    "uri": "file:///home/user/script.py",
    "mimeType": "text/x-python",
    "text": "def hello():\n    print('Hello, world!')"
  }
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `resource` | EmbeddedResourceResource（必填） | 嵌入式资源内容（Text Resource 或 Blob Resource） |
| `annotations` | Annotations（可选） | 内容展示元数据 |

**Text Resource** 字段：`uri`（string）、`mimeType`（string）、`text`（string）

**Blob Resource** 字段：`uri`（string）、`mimeType`（string）、`blob`（string，Base64 编码）

### 8.5 资源链接（Resource Link）

所有 Agent **必须** 支持资源链接内容块，Agent 可以访问引用的资源。

```json
{
  "type": "resource_link",
  "uri": "file:///home/user/document.pdf",
  "name": "document.pdf",
  "mimeType": "application/pdf",
  "size": 1024000
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `uri` | string（必填） | 资源的 URI |
| `name` | string（必填） | 资源的人类可读名称 |
| `mimeType` | string（可选） | 资源的 MIME 类型 |
| `title` | string（可选） | 可选的展示标题 |
| `description` | string（可选） | 资源内容的可选描述 |
| `size` | integer（可选） | 资源大小（字节） |
| `annotations` | Annotations（可选） | 内容展示元数据 |

---

## 9. 工具调用（Tool Calls）

工具调用代表语言模型在 prompt 回合中请求 Agent 执行的操作。Agent 通过 `session/update` 通知报告工具调用，允许 Client 向用户显示实时进度和结果。

### 9.1 创建工具调用

当语言模型请求工具调用时，Agent **应该** 将其报告给 Client：

```json
{
  "jsonrpc": "2.0",
  "method": "session/update",
  "params": {
    "sessionId": "sess_abc123def456",
    "update": {
      "sessionUpdate": "tool_call",
      "toolCallId": "call_001",
      "title": "Reading configuration file",
      "kind": "read",
      "status": "pending"
    }
  }
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `toolCallId` | ToolCallId（必填） | 会话内此工具调用的唯一标识符 |
| `title` | string（必填） | 描述工具正在做什么的人类可读标题 |
| `kind` | ToolKind（可选） | 工具类别，帮助 Client 选择合适的图标和展示方式 |
| `status` | ToolCallStatus（可选，默认 `pending`） | 当前执行状态 |
| `content` | ToolCallContent[]（可选） | 工具调用产生的内容 |
| `locations` | ToolCallLocation[]（可选） | 此工具调用影响的文件位置 |
| `rawInput` | object（可选） | 发送给工具的原始输入参数 |
| `rawOutput` | object（可选） | 工具返回的原始输出 |

### 9.2 更新工具调用

工具执行时，Agent 发送进度和结果更新（使用 `tool_call_update`）：

```json
{
  "jsonrpc": "2.0",
  "method": "session/update",
  "params": {
    "sessionId": "sess_abc123def456",
    "update": {
      "sessionUpdate": "tool_call_update",
      "toolCallId": "call_001",
      "status": "in_progress",
      "content": [
        { "type": "content", "content": { "type": "text", "text": "Found 3 configuration files..." } }
      ]
    }
  }
}
```

除 `toolCallId` 外，所有字段在更新中都是可选的，只需包含正在更改的字段。

### 9.3 工具调用状态（ToolCallStatus）

| 状态 | 说明 |
|------|------|
| `pending` | 工具调用尚未开始（输入正在流式传输或等待批准） |
| `in_progress` | 工具调用正在运行 |
| `completed` | 工具调用成功完成 |
| `failed` | 工具调用失败 |

### 9.4 请求权限（session/request_permission）

Agent **可以** 在执行工具调用之前通过 `session/request_permission` 方法向用户请求权限：

```json
{
  "jsonrpc": "2.0",
  "id": 5,
  "method": "session/request_permission",
  "params": {
    "sessionId": "sess_abc123def456",
    "toolCall": { "toolCallId": "call_001" },
    "options": [
      { "optionId": "allow-once", "name": "Allow once", "kind": "allow_once" },
      { "optionId": "reject-once", "name": "Reject", "kind": "reject_once" }
    ]
  }
}
```

| 参数 | 类型 | 说明 |
|------|------|------|
| `sessionId` | SessionId（必填） | 请求的会话 ID |
| `toolCall` | ToolCallUpdate（必填） | 包含操作详情的工具调用更新 |
| `options` | PermissionOption[]（必填） | 供用户选择的可用权限选项 |

`PermissionOptionKind` 值：

| 值 | 说明 |
|----|------|
| `allow_once` | 仅允许此次操作 |
| `allow_always` | 允许此操作并记住选择 |
| `reject_once` | 仅拒绝此次操作 |
| `reject_always` | 拒绝此操作并记住选择 |

**Client 响应（用户已选择）：**

```json
{
  "jsonrpc": "2.0",
  "id": 5,
  "result": {
    "outcome": { "outcome": "selected", "optionId": "allow-once" }
  }
}
```

**Client 响应（prompt 回合被取消）：**

```json
{
  "jsonrpc": "2.0",
  "id": 5,
  "result": { "outcome": { "outcome": "cancelled" } }
}
```

Client **可以** 根据用户设置自动允许或拒绝权限请求。如果当前 prompt 回合被取消，Client **必须** 以 `cancelled` 结果响应所有待处理的权限请求。

### 9.5 工具调用内容类型

工具调用可以产生三种类型的内容：

#### 常规内容（Regular Content）

```json
{ "type": "content", "content": { "type": "text", "text": "Analysis complete." } }
```

#### 差异（Diffs）

以 diff 格式显示文件修改：

```json
{
  "type": "diff",
  "path": "/home/user/project/src/config.json",
  "oldText": "{\n  \"debug\": false\n}",
  "newText": "{\n  \"debug\": true\n}"
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `path` | string（必填） | 正在修改的绝对文件路径 |
| `oldText` | string | 原始内容（新文件时为 null） |
| `newText` | string（必填） | 修改后的新内容 |

#### 终端输出（Terminals）

```json
{ "type": "terminal", "terminalId": "term_xyz789" }
```

当终端嵌入工具调用时，Client 实时显示输出，即使终端释放后也继续显示。

### 9.6 跟踪 Agent 位置（Following the Agent）

工具调用可以报告它们正在操作的文件位置，使 Client 能够实现实时跟踪功能：

```json
{ "path": "/home/user/project/src/main.py", "line": 42 }
```

---

## 10. 文件系统（File System）

文件系统方法允许 Agent 在 Client 的环境中读写文本文件，使 Agent 能够访问编辑器中未保存的状态，并允许 Client 跟踪代理执行期间的文件修改。

### 10.1 检查支持

Agent **必须** 在尝试使用文件系统方法之前验证 Client 是否支持这些能力：

```json
{
  "clientCapabilities": {
    "fs": { "readTextFile": true, "writeTextFile": true }
  }
}
```

如果 `readTextFile` 或 `writeTextFile` 为 `false` 或不存在，Agent **必须不** 尝试调用对应的文件系统方法。

### 10.2 读取文件（fs/read_text_file）

`fs/read_text_file` 允许 Agent 从 Client 的文件系统读取文本文件内容，包括编辑器中未保存的更改：

```json
{
  "jsonrpc": "2.0",
  "id": 3,
  "method": "fs/read_text_file",
  "params": {
    "sessionId": "sess_abc123def456",
    "path": "/home/user/project/src/main.py",
    "line": 10,
    "limit": 50
  }
}
```

| 参数 | 类型 | 说明 |
|------|------|------|
| `sessionId` | SessionId（必填） | 请求的会话 ID |
| `path` | string（必填） | 要读取的文件的绝对路径 |
| `line` | number（可选） | 开始读取的行号（1-based） |
| `limit` | number（可选） | 最大读取行数 |

```json
{
  "jsonrpc": "2.0",
  "id": 3,
  "result": { "content": "def hello_world():\n    print('Hello, world!')\n" }
}
```

### 10.3 写入文件（fs/write_text_file）

`fs/write_text_file` 允许 Agent 在 Client 的文件系统中写入或更新文本文件：

```json
{
  "jsonrpc": "2.0",
  "id": 4,
  "method": "fs/write_text_file",
  "params": {
    "sessionId": "sess_abc123def456",
    "path": "/home/user/project/config.json",
    "content": "{\n  \"debug\": true,\n  \"version\": \"1.0.0\"\n}"
  }
}
```

| 参数 | 类型 | 说明 |
|------|------|------|
| `sessionId` | SessionId（必填） | 请求的会话 ID |
| `path` | string（必填） | 要写入的文件的绝对路径，Client **必须** 在文件不存在时创建它 |
| `content` | string（必填） | 要写入文件的文本内容 |

Client 成功时响应空结果：`{ "result": null }`

---

## 11. 终端（Terminals）

终端方法允许 Agent 在 Client 的环境中执行 shell 命令，提供实时输出流和进程控制。

### 11.1 检查支持

Agent **必须** 先验证 Client 是否支持终端能力（`clientCapabilities.terminal: true`）。如果不支持，Agent **必须不** 调用任何终端方法。

### 11.2 创建终端（terminal/create）

```json
{
  "jsonrpc": "2.0",
  "id": 5,
  "method": "terminal/create",
  "params": {
    "sessionId": "sess_abc123def456",
    "command": "npm",
    "args": ["test", "--coverage"],
    "env": [{ "name": "NODE_ENV", "value": "test" }],
    "cwd": "/home/user/project",
    "outputByteLimit": 1048576
  }
}
```

| 参数 | 类型 | 说明 |
|------|------|------|
| `sessionId` | SessionId（必填） | 请求的会话 ID |
| `command` | string（必填） | 要执行的命令 |
| `args` | string[]（可选） | 命令参数数组 |
| `env` | EnvVariable[]（可选） | 命令的环境变量（每个变量有 `name` 和 `value`） |
| `cwd` | string（可选） | 命令的工作目录（绝对路径） |
| `outputByteLimit` | number（可选） | 最大保留输出字节数（超过后从头截断，保证字符边界对齐） |

Client 不等待命令完成，立即返回：

```json
{ "jsonrpc": "2.0", "id": 5, "result": { "terminalId": "term_xyz789" } }
```

Agent **必须** 在不再需要时使用 `terminal/release` 释放终端。

### 11.3 在工具调用中嵌入终端

终端可以直接嵌入工具调用中，向用户提供实时输出：

```json
{
  "sessionUpdate": "tool_call",
  "toolCallId": "call_002",
  "title": "Running tests",
  "kind": "execute",
  "status": "in_progress",
  "content": [{ "type": "terminal", "terminalId": "term_xyz789" }]
}
```

### 11.4 获取输出（terminal/output）

获取当前终端输出，不等待命令完成：

```json
{
  "jsonrpc": "2.0",
  "id": 6,
  "method": "terminal/output",
  "params": { "sessionId": "sess_abc123def456", "terminalId": "term_xyz789" }
}
```

```json
{
  "jsonrpc": "2.0",
  "id": 6,
  "result": {
    "output": "Running tests...\n✓ All tests passed (42 total)\n",
    "truncated": false,
    "exitStatus": { "exitCode": 0, "signal": null }
  }
}
```

| 响应字段 | 类型 | 说明 |
|----------|------|------|
| `output` | string（必填） | 到目前为止捕获的终端输出 |
| `truncated` | boolean（必填） | 输出是否因字节限制而被截断 |
| `exitStatus` | TerminalExitStatus（可选） | 仅当命令已退出时存在，包含 `exitCode` 和 `signal` |

### 11.5 等待退出（terminal/wait_for_exit）

命令完成后返回退出状态：

```json
{
  "jsonrpc": "2.0",
  "id": 7,
  "method": "terminal/wait_for_exit",
  "params": { "sessionId": "sess_abc123def456", "terminalId": "term_xyz789" }
}
```

```json
{ "jsonrpc": "2.0", "id": 7, "result": { "exitCode": 0, "signal": null } }
```

### 11.6 终止命令（terminal/kill）

终止命令而不释放终端（终端 ID 仍然有效，可继续查询输出/状态）：

```json
{
  "jsonrpc": "2.0",
  "id": 8,
  "method": "terminal/kill",
  "params": { "sessionId": "sess_abc123def456", "terminalId": "term_xyz789" }
}
```

Agent **仍然必须** 在使用完后调用 `terminal/release`。

#### 实现超时的模式

```
1. 使用 terminal/create 创建终端
2. 为所需超时时长启动计时器
3. 并发等待：计时器到期 OR terminal/wait_for_exit 返回
4. 如果计时器先到期：
   - 调用 terminal/kill 终止命令
   - 调用 terminal/output 获取最终输出
   - 将输出包含在对模型的响应中
5. 最后调用 terminal/release
```

### 11.7 释放终端（terminal/release）

终止仍在运行的命令并释放所有资源：

```json
{
  "jsonrpc": "2.0",
  "id": 9,
  "method": "terminal/release",
  "params": { "sessionId": "sess_abc123def456", "terminalId": "term_xyz789" }
}
```

释放后，终端 ID 对所有其他 `terminal/*` 方法无效。如果终端已添加到工具调用中，Client **应该** 在释放后继续显示其输出。

---

## 12. Agent 计划（Agent Plan）

计划是复杂任务（需要多个步骤）的执行策略。Agent 可以通过 `session/update` 通知与 Client 分享计划，提供对其思考和进度的实时可见性。

### 12.1 创建计划

```json
{
  "jsonrpc": "2.0",
  "method": "session/update",
  "params": {
    "sessionId": "sess_abc123def456",
    "update": {
      "sessionUpdate": "plan",
      "entries": [
        { "content": "Analyze the existing codebase structure", "priority": "high", "status": "pending" },
        { "content": "Identify components that need refactoring", "priority": "high", "status": "pending" },
        { "content": "Create unit tests for critical functions", "priority": "medium", "status": "pending" }
      ]
    }
  }
}
```

### 12.2 计划条目（PlanEntry）字段

| 字段 | 类型 | 说明 |
|------|------|------|
| `content` | string（必填） | 描述此任务要完成什么的人类可读描述 |
| `priority` | PlanEntryPriority（必填） | 相对重要性：`high`、`medium`、`low` |
| `status` | PlanEntryStatus（必填） | 此任务当前的执行状态：`pending`、`in_progress`、`completed` |

### 12.3 更新计划

Agent 在推进计划时，**应该** 通过发送更多 `session/update` 通知来报告更新。

Agent **必须** 在每次更新中发送**完整的**所有计划条目列表及其当前状态。Client **必须** 完全替换当前计划（不是增量更新）。

### 12.4 动态规划

计划可以在执行过程中演变。Agent **可以** 根据发现的新需求或完成的任务来添加、删除或修改计划条目。

---

## 13. 会话模式（Session Modes）

> **注意**：会话模式 API 已被 [Session Config Options](#14-会话配置项session-config-options) 取代。专用会话模式方法将在协议未来版本中移除。在此期间，Agent 可以同时提供两者以向后兼容。

Agent 可以提供一组它可以运行的模式，模式通常影响使用的系统提示、工具的可用性以及是否在运行前请求权限。

### 13.1 初始状态

在会话建立期间，Agent **可以** 在 `session/new` 或 `session/load` 的响应中返回模式列表：

```json
{
  "sessionId": "sess_abc123def456",
  "modes": {
    "currentModeId": "ask",
    "availableModes": [
      { "id": "ask", "name": "Ask", "description": "Request permission before making any changes" },
      { "id": "architect", "name": "Architect", "description": "Design and plan without implementation" },
      { "id": "code", "name": "Code", "description": "Write and modify code with full tool access" }
    ]
  }
}
```

`SessionMode` 字段：

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | SessionModeId（必填） | 此模式的唯一标识符 |
| `name` | string（必填） | 模式的人类可读名称 |
| `description` | string（可选） | 描述此模式功能的可选说明 |

### 13.2 从 Client 设置模式（session/set_mode）

```json
{
  "jsonrpc": "2.0",
  "id": 2,
  "method": "session/set_mode",
  "params": {
    "sessionId": "sess_abc123def456",
    "modeId": "code"
  }
}
```

### 13.3 从 Agent 设置模式

Agent 通过 `current_mode_update` 会话通知告知 Client 模式变更：

```json
{
  "jsonrpc": "2.0",
  "method": "session/update",
  "params": {
    "sessionId": "sess_abc123def456",
    "update": { "sessionUpdate": "current_mode_update", "modeId": "code" }
  }
}
```

---

## 14. 会话配置项（Session Config Options）

Session Config Options 是暴露会话级配置的**首选方式**，取代旧的 Session Modes API。Agent 可以为会话提供任意配置选项列表（如模型、模式、推理级别等）。

### 14.1 初始状态

在会话建立期间，Agent **可以** 在响应中返回配置选项列表：

```json
{
  "sessionId": "sess_abc123def456",
  "configOptions": [
    {
      "id": "mode",
      "name": "Session Mode",
      "description": "Controls how the agent requests permission",
      "category": "mode",
      "type": "select",
      "currentValue": "ask",
      "options": [
        { "value": "ask", "name": "Ask", "description": "Request permission before changes" },
        { "value": "code", "name": "Code", "description": "Full tool access" }
      ]
    },
    {
      "id": "model",
      "name": "Model",
      "category": "model",
      "type": "select",
      "currentValue": "model-1",
      "options": [
        { "value": "model-1", "name": "Model 1", "description": "The fastest model" },
        { "value": "model-2", "name": "Model 2", "description": "The most powerful model" }
      ]
    }
  ]
}
```

### 14.2 ConfigOption 字段

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | string（必填） | 此配置选项的唯一标识符，设置值时使用 |
| `name` | string（必填） | 选项的人类可读标签 |
| `description` | string（可选） | 描述此选项控制内容的可选说明 |
| `category` | ConfigOptionCategory（可选） | 语义分类，帮助 Client 提供一致的 UX |
| `type` | ConfigOptionType（必填） | 输入控件类型，目前仅支持 `select` |
| `currentValue` | string（必填） | 此选项当前选择的值 |
| `options` | ConfigOptionValue[]（必填） | 此选项的可用值列表 |

### 14.3 选项分类（Category）

| 分类 | 说明 |
|------|------|
| `mode` | 会话模式选择器 |
| `model` | 模型选择器 |
| `thought_level` | 思考 / 推理级别选择器 |

以 `_` 开头的分类名可自由用于自定义用途。分类是仅供 UX 使用的元数据，**不得** 要求其存在才能正常工作。

`configOptions` 数组的顺序代表 Agent 的优先级，Client **应该** 尊重此顺序。

### 14.4 从 Client 设置配置项（session/set_config_option）

```json
{
  "jsonrpc": "2.0",
  "id": 2,
  "method": "session/set_config_option",
  "params": {
    "sessionId": "sess_abc123def456",
    "configId": "mode",
    "value": "code"
  }
}
```

| 参数 | 类型 | 说明 |
|------|------|------|
| `sessionId` | SessionId（必填） | 会话 ID |
| `configId` | string（必填） | 要更改的配置选项的 `id` |
| `value` | string（必填） | 要设置的新值，必须是 `options` 数组中的值之一 |

Agent **必须** 以**所有**配置选项及其当前值的完整列表响应（允许反映依赖变更，如更换模型后 reasoning 选项随之变化）。

### 14.5 从 Agent 设置配置项

Agent 可以通过 `config_option_update` 通知主动更改配置：

```json
{
  "sessionUpdate": "config_option_update",
  "configOptions": [/* 完整配置列表 */]
}
```

常见原因：完成规划阶段后切换模式、因速率限制回退到不同模型、根据上下文调整可用选项。

### 14.6 与 Session Modes 的关系

同时提供 `configOptions` 和 `modes` 时：
- 支持 configOptions 的 Client **应该** 仅使用 `configOptions`，忽略 `modes`
- 不支持的 Client **应该** 回退到 `modes`
- Agent **应该** 保持两者同步

Agent **必须** 始终为每个配置选项提供默认值，确保即使 Client 不支持 configOptions 也能正常运行。

---

## 15. 斜线命令（Slash Commands）

Agent 可以向用户发布一组可调用的斜线命令，提供对特定 Agent 能力和工作流的快速访问。

### 15.1 发布命令

创建会话后，Agent **可以** 通过 `available_commands_update` 通知发送可用命令列表：

```json
{
  "jsonrpc": "2.0",
  "method": "session/update",
  "params": {
    "sessionId": "sess_abc123def456",
    "update": {
      "sessionUpdate": "available_commands_update",
      "availableCommands": [
        {
          "name": "web",
          "description": "Search the web for information",
          "input": { "hint": "query to search for" }
        },
        { "name": "test", "description": "Run tests for the current project" },
        {
          "name": "plan",
          "description": "Create a detailed implementation plan",
          "input": { "hint": "description of what to plan" }
        }
      ]
    }
  }
}
```

`AvailableCommand` 字段：

| 字段 | 类型 | 说明 |
|------|------|------|
| `name` | string（必填） | 命令名称（如 "web"、"test"、"plan"） |
| `description` | string（必填） | 命令功能的人类可读描述 |
| `input.hint` | string（可选） | 未提供输入时显示的提示文本 |

### 15.2 动态更新

Agent 可以在会话期间的任意时刻通过发送另一个 `available_commands_update` 通知来更新可用命令列表（根据上下文添加/删除/修改命令）。

### 15.3 运行命令

命令作为常规用户消息包含在 prompt 请求中，Client 在 prompt 文本前加上 `/` 前缀：

```json
{
  "jsonrpc": "2.0",
  "id": 3,
  "method": "session/prompt",
  "params": {
    "sessionId": "sess_abc123def456",
    "prompt": [{ "type": "text", "text": "/web agent client protocol" }]
  }
}
```

Agent 识别命令前缀并进行相应处理。命令可以与其他任何用户消息内容类型（图片、音频等）一起使用。

---

## 16. 扩展性（Extensibility）

ACP 提供内置的扩展机制，允许实现者在保持与核心协议兼容的同时添加自定义功能。

### 16.1 `_meta` 字段

协议中的所有类型都包含 `_meta` 字段（类型为 `{ [key: string]: unknown }`），可以附加自定义信息。这包括请求、响应、通知，甚至内容块、工具调用、计划条目和能力对象等嵌套类型。

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "session/prompt",
  "params": {
    "sessionId": "sess_abc123def456",
    "prompt": [{ "type": "text", "text": "Hello, world!" }],
    "_meta": {
      "traceparent": "00-80e1afed08e019fc1110464cfa66635c-7a085853722dc6d2-01",
      "zed.dev/debugMode": true
    }
  }
}
```

`_meta` 根级键中以下内容 **应该** 保留给 W3C Trace Context：
- `traceparent`、`tracestate`、`baggage`

实现者 **必须不** 在规范类型的根级别添加任何自定义字段（所有可能的名称都保留给未来的协议版本）。

### 16.2 扩展方法

协议将所有以下划线（`_`）开头的方法名称保留给自定义扩展。

#### 自定义请求（Custom Requests）

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "_zed.dev/workspace/buffers",
  "params": { "language": "rust" }
}
```

如果接收端不识别自定义方法名称，应以标准 "Method not found" 错误响应（code: `-32601`）。

#### 自定义通知（Custom Notifications）

```json
{
  "jsonrpc": "2.0",
  "method": "_zed.dev/file_opened",
  "params": { "path": "/home/user/project/src/editor.rs" }
}
```

与自定义请求不同，实现者 **应该** 忽略未识别的通知。

### 16.3 发布自定义能力

实现者 **应该** 在 `initialize` 响应的 `agentCapabilities._meta` 字段中发布对扩展和其方法的支持，允许双方在初始化期间协商自定义功能：

```json
{
  "agentCapabilities": {
    "loadSession": true,
    "_meta": {
      "zed.dev": {
        "workspace": true,
        "fileNotifications": true
      }
    }
  }
}
```

---

## 17. 传输层（Transports）

ACP 使用 JSON-RPC 编码消息，所有消息 **必须** 使用 UTF-8 编码。

### 17.1 stdio 传输（推荐）

所有 Agent 和 Client **应该** 在可能的情况下支持 stdio。

**规则：**
- Client 将 Agent 作为子进程启动
- Agent 从 `stdin` 读取 JSON-RPC 消息，将消息发送到 `stdout`
- 消息以换行符（`\n`）分隔，**必须不** 包含嵌入式换行符
- Agent **可以** 将 UTF-8 字符串写入 `stderr` 用于日志记录，Client **可以** 捕获、转发或忽略此输出
- Agent **必须不** 向 `stdout` 写入任何不是有效 ACP 消息的内容
- Client **必须不** 向 Agent 的 `stdin` 写入任何不是有效 ACP 消息的内容

```
Client                       Agent Process
  |                               |
  |  Launch subprocess            |
  |  Write to stdin  ----------->|
  |                  Write to stdout
  |<-----------------------------|
  |        (Optional logs on stderr)
  |<-----------------------------|
  |  Close stdin, terminate ----->|
```

### 17.2 Streamable HTTP

目前处于讨论阶段，草案提案正在进行中。

### 17.3 自定义传输

Agent 和 Client **可以** 实现额外的自定义传输机制。协议是传输无关的，可以在任何支持双向消息交换的通信通道上实现。

自定义传输实现者 **必须** 确保保留 ACP 定义的 JSON-RPC 消息格式和生命周期要求，并 **应该** 记录其特定的连接建立和消息交换模式。

---

## 18. 错误码（Error Codes）

标准 JSON-RPC 2.0 + ACP 扩展：

| 错误码 | 含义 |
|--------|------|
| `-32700` | Parse error（解析错误） |
| `-32600` | Invalid request（无效请求） |
| `-32601` | Method not found（方法未找到） |
| `-32602` | Invalid params（无效参数） |
| `-32603` | Internal error（内部错误） |
| `-32000` | Authentication required（需要认证） |
| `-32002` | Resource not found（资源未找到） |
| 其他 `int32` | 实现自定义错误 |

---

## 19. unstable 草案差异

相对于稳定版新增/扩展（截至 2026-03-15，均为草案，不保证兼容性）：

### 19.1 新增会话方法

| 方法 | 说明 |
|------|------|
| `session/close` | 关闭会话 |
| `session/fork` | 派生会话 |
| `session/resume` | 恢复会话 |
| `session/set_model` | 设置模型 |

### 19.2 协议级取消通知

- `$/cancel_request`：协议级取消请求通知

---

## 20. 落地实现建议

1. **以 `initialize` 返回的 capability 作为唯一开关源**，不要硬编码功能假设；Client 和 Agent 都必须将所有省略的 capabilities 视为不支持。

2. **`session/prompt` 必须是三段式**：请求 → 流式 `session/update` 通知 → 最终 `stopReason` 响应。

3. **权限请求必须支持 `cancelled` 结果**，避免 prompt 取消时悬挂请求。

4. **所有对象保留 `_meta` 原样透传**，兼容未来扩展。

5. **stable 与 unstable 的处理路径分离**，便于升级和回滚。

6. **优先使用 Session Config Options**（而非 Session Modes），`configOptions` 是未来主导 API，`modes` 即将废弃。如需兼容旧 Client，两者并存并保持同步。

7. **传输层优先选择 stdio**；对于新 Agent 也应考虑支持 HTTP transport 以兼容现代 MCP 服务器。

8. **工具调用生命周期必须完整**：`pending` → `in_progress` → `completed`/`failed`，不要跳过中间状态。

9. **文件路径必须是绝对路径**，行号必须从 1 开始（1-based）。

10. **Agent 取消时必须捕获所有异常**，以语义明确的 `cancelled` stopReason 响应，而非 error 响应，否则 Client 会将其展示为错误。

11. **计划（Plan）更新必须发完整列表**，Client 完全替换当前计划，不是增量合并。

12. **动态斜线命令**：Agent 可以运行中随时更新可用命令；命令通过普通 `/text` 消息调用，无需特殊协议支持。

---

## 附录：官方文档链接

### 协议说明页面

| 页面 | URL |
|------|-----|
| Overview | https://agentclientprotocol.com/protocol/overview |
| Initialization | https://agentclientprotocol.com/protocol/initialization |
| Session Setup | https://agentclientprotocol.com/protocol/session-setup |
| Session List | https://agentclientprotocol.com/protocol/session-list |
| Prompt Turn | https://agentclientprotocol.com/protocol/prompt-turn |
| Content | https://agentclientprotocol.com/protocol/content |
| Tool Calls | https://agentclientprotocol.com/protocol/tool-calls |
| File System | https://agentclientprotocol.com/protocol/file-system |
| Terminals | https://agentclientprotocol.com/protocol/terminals |
| Agent Plan | https://agentclientprotocol.com/protocol/agent-plan |
| Session Modes | https://agentclientprotocol.com/protocol/session-modes |
| Session Config Options | https://agentclientprotocol.com/protocol/session-config-options |
| Slash Commands | https://agentclientprotocol.com/protocol/slash-commands |
| Extensibility | https://agentclientprotocol.com/protocol/extensibility |
| Transports | https://agentclientprotocol.com/protocol/transports |

### Schema（机器可读）

| 名称 | URL |
|------|-----|
| 稳定版 schema | https://raw.githubusercontent.com/agentclientprotocol/agent-client-protocol/main/schema/schema.json |
| unstable schema | https://raw.githubusercontent.com/agentclientprotocol/agent-client-protocol/main/schema/schema.unstable.json |
