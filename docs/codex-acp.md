# codex-acp 使用摘要

> 整理日期：2026-03-14
> 仓库：https://github.com/zed-industries/codex-acp
> 协议：Apache-2.0

## 1. 概述

`codex-acp` 是 Zed 官方提供的 ACP 适配器，将 OpenAI Codex CLI 包装为 ACP（Agent Client Protocol）兼容的 agent。

任何 ACP 兼容客户端（包括 WheelMaker）可以通过 stdin/stdout 以 JSON-RPC 2.0 协议与它通信。

## 2. 安装

### 方式一：npm/npx（推荐，跨平台）

```bash
# 全局安装
npm install -g @zed-industries/codex-acp

# 或直接 npx 运行（首次自动安装）
npx @zed-industries/codex-acp
```

### 方式二：GitHub Releases 下载预编译二进制

从 https://github.com/zed-industries/codex-acp/releases 下载对应平台：

| 平台 | 文件名 |
|------|--------|
| Windows x64 | `codex-acp-windows-amd64.exe` |
| macOS Apple Silicon | `codex-acp-darwin-arm64` |
| macOS Intel | `codex-acp-darwin-amd64` |
| Linux x64 | `codex-acp-linux-amd64` |

下载后放置到 `bin/{platform}/codex-acp[.exe]`（WheelMaker 的 `tools.ResolveBinary` 会自动找到）。

## 3. 认证

启动前需通过环境变量提供 API Key：

```bash
# 方式一：OpenAI API Key（推荐）
export OPENAI_API_KEY=sk-...

# 方式二：Codex 专属 Key
export CODEX_API_KEY=...

# 方式三：ChatGPT Plus 订阅（仅本地项目有效）
# 无需 Key，但功能受限
```

## 4. 启动方式

```bash
# 直接运行（client 通过 stdin/stdout 通信）
OPENAI_API_KEY=sk-... codex-acp

# 或通过 npx
OPENAI_API_KEY=sk-... npx @zed-industries/codex-acp
```

启动后，进程会阻塞等待来自 stdin 的 JSON-RPC 消息。

## 5. ACP 通信协议

### 5.1 消息格式

每条消息是一行 JSON（`\n` 换行符分隔），消息内部不含换行：

```
→ stdin:  {"jsonrpc":"2.0","id":1,"method":"...","params":{...}}\n
← stdout: {"jsonrpc":"2.0","id":1,"result":{...}}\n
← stdout: {"jsonrpc":"2.0","method":"session/update","params":{...}}\n  ← notification
```

### 5.2 完整握手时序

```
1. initialize（能力协商）
→ {"id":1,"method":"initialize","params":{"protocolVersion":"0.1","clientCapabilities":{"fs":{"readTextFile":true,"writeTextFile":true},"terminal":true},"clientInfo":{"name":"wheelmaker","version":"0.1"}}}
← {"id":1,"result":{"protocolVersion":"0.1","agentCapabilities":{...},"agentInfo":{"name":"codex-acp"}}}

2. session/new（创建会话）
→ {"id":2,"method":"session/new","params":{"cwd":"/path/to/project","mcpServers":[]}}
← {"id":2,"result":{"sessionId":"sess_abc123","modes":[...]}}

3. session/prompt（发送 prompt，流式响应）
→ {"id":3,"method":"session/prompt","params":{"sessionId":"sess_abc123","prompt":"解释一下 main.go"}}
← {"method":"session/update","params":{"sessionId":"sess_abc123","update":{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"这个文件..."}}}}
← {"method":"session/update","params":{"sessionId":"sess_abc123","update":{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"包含..."}}}}
← {"id":3,"result":{"stopReason":"end_turn"}}

4. （可选）session/load（恢复会话）
→ {"id":4,"method":"session/load","params":{"sessionId":"sess_abc123","cwd":"/path","mcpServers":[]}}

5. session/cancel（取消进行中的 prompt）
→ {"method":"session/cancel","params":{"sessionId":"sess_abc123"}}   ← notification，无 id
```

### 5.3 session/update 类型

| sessionUpdate | 说明 |
|--------------|------|
| `agent_message_chunk` | AI 回复文本块（流式） |
| `user_message_chunk` | 用户消息回显 |
| `agent_thought_chunk` | 思考过程（可选显示） |
| `tool_call` | 工具调用（文件读写/终端执行） |
| `tool_call_update` | 工具调用状态更新 |
| `plan` | agent 执行计划 |

### 5.4 权限请求（session/request_permission）

当 Codex 需要执行文件操作或终端命令时，会向 Client 发起权限请求：

```
← {"id":10,"method":"session/request_permission","params":{
    "sessionId":"sess_abc123",
    "toolCall":{"name":"shell","args":{"command":"npm install"}},
    "options":[
      {"id":"allow_once","label":"允许一次","kind":"allow_once"},
      {"id":"allow_always","label":"总是允许","kind":"allow_always"},
      {"id":"reject","label":"拒绝","kind":"reject_once"}
    ]
  }}
→ {"id":10,"result":{"outcome":"selected","optionId":"allow_once"}}
```

WheelMaker 在 MVP 阶段可以默认 `allow_once`，Phase 2 通过飞书消息让用户确认。

## 6. 支持的 Slash 命令

| 命令 | 说明 |
|------|------|
| `/review` | Review 当前工作区变更 |
| `/review-branch` | Review 当前分支 |
| `/review-commit` | Review 最新 commit |
| `/init` | 初始化项目（生成 AGENTS.md 等） |
| `/compact` | 压缩会话历史 |
| `/logout` | 登出认证 |

## 7. WheelMaker 中的集成要点

### 子进程管理

```go
cmd := exec.CommandContext(ctx, exePath)
cmd.Env = append(os.Environ(), "OPENAI_API_KEY="+apiKey)
cmd.Stdin = stdinPipe
cmd.Stdout = stdoutPipe
cmd.Stderr = os.Stderr  // 日志输出到主进程 stderr
cmd.Start()
```

### 并发读取

- 单独 goroutine 持续用 `bufio.Scanner` 读取 stdout
- 按 `id` 分发 Response，广播 Notification
- 使用 `context.WithCancel` 控制生命周期

### Session 恢复

- 成功建立 session 后，将 `sessionId` 持久化到 `~/.wheelmaker/state.json`
- 下次启动时先尝试 `session/load`，失败则 `session/new`

## 8. 已知限制

- codex-acp 使用 OpenAI Codex，需要 OpenAI API Key 或 ChatGPT Plus
- ChatGPT 订阅方式不支持远程项目（仅本地）
- 工具调用需要 Client 实现 `fs/read_text_file`、`fs/write_text_file`、`terminal/*` 方法响应
  - WheelMaker MVP 阶段可以默认允许并在本地执行这些操作

