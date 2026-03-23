# WheelMaker — Server

Go 守护进程：连接本地 AI CLI（Codex、Claude 等）与远端 IM（飞书、移动 App 等）。

## 架构层次

```
Hub (internal/hub/)             — 多 project 生命周期管理，读 config.json
  ├─ im/forwarder.Forwarder     — IM 包装层：消息路由、决策请求管理、HelpResolver
  │    └─ console / feishu / mobile  — 底层 IM 适配器
  └─ client.Client              — 单 project 主控：命令路由、会话管理、状态持久化
       └─ acp.Forwarder         — ACP 协议封装：类型化出站方法、ClientCallbacks 分发
            └─ acp.Conn         — JSON-RPC 2.0 over stdio → CLI binary
```

## 包职责

| 包 | 职责 |
|----|------|
| `internal/hub/` | 读 `~/.wheelmaker/config.json`，为每个 project 创建 Client + IM |
| `internal/client/` | 主控：命令路由、会话生命周期（ensureReady/promptStream/switchAgent）、terminal 管理、实现 acp.ClientCallbacks |
| `internal/acp/` | 纯传输层：Conn（子进程 stdio）、Forwarder（类型化 ACP 方法 + ClientCallbacks 分发）、协议类型 |
| `internal/agent/claude/` | 启动 claude-agent-acp 子进程，返回 `*acp.Conn`；NormalizeParams/HandlePermission 钩子 |
| `internal/agent/codex/` | 启动 codex-acp 子进程，返回 `*acp.Conn`；同上 |
| `internal/im/forwarder/` | IM 包装层：消息去重/过滤、决策请求（pending decisions）、HelpResolver 注入 |
| `internal/im/console/` | Console IM 适配器：读 stdin，debug 模式打印所有 ACP JSON |
| `internal/im/feishu/` | 飞书 Bot IM 适配器 |
| `internal/im/mobile/` | WebSocket IM 适配器（供移动端 App 连接） |
| `internal/tools/` | 工具二进制路径解析（`bin/{GOOS}_{GOARCH}/`） |

## 配置文件

- `~/.wheelmaker/config.json` — 项目配置（IM 类型、agent、工作目录）
- `~/.wheelmaker/state.json` — 运行时状态持久化（session ID、agent 元数据、session 状态）

config.json 格式：
```json
{
  "projects": [
    { "name": "local", "debug": true, "im": { "type": "console" },
      "client": { "agent": "claude", "path": "/your/project" } },
    { "name": "mobile", "im": { "type": "mobile", "mobile": { "port": 9527, "token": "change-me" } },
      "client": { "agent": "claude", "path": "/your/project" } }
  ]
}
```

## Mobile WebSocket 协议

App 通过 WebSocket 连接到 `ws://<host>:<port>/ws`，握手流程：

```
Server → { "type": "auth_required" }          (若配置了 token)
Client → { "type": "auth", "token": "..." }
Server → { "type": "ready", "chatId": "..." }
```

入站消息类型：`auth` / `message` / `option` / `ping`  
出站消息类型：`text` / `card` / `options` / `debug` / `pong` / `error` / `ready` / `auth_required`

决策流：`options` 携带 `decisionId` → App 选择后发 `{type:"option", decisionId, optionId}` → Bridge 解析

## state.go 设计

```
FileState
  └─ Projects map[name]*ProjectState
       ├─ ActiveAgent string
       ├─ Connection *ConnectionConfig
       └─ Agents map[name]*AgentState
            ├─ LastSessionID
            ├─ ProtocolVersion / AgentCapabilities / AgentInfo / AuthMethods
            ├─ Session *SessionState  (Modes/Models/ConfigOptions/AvailableCommands/Title/UpdatedAt)
            └─ Sessions []SessionSummary  (懒加载)
```

## 开发约定

- **接口优先**：跨层依赖通过接口（`acp.Session`、`agent.Agent`、`im.Channel`）
- **懒加载**：agent 子进程在首条消息时才创建（`ensureForwarder`）
- **支持的命令**：`/use`、`/cancel`、`/status`、`/mode`、`/model`、`/list`、`/new`、`/load`、`/debug`；其他 `/` 开头文本当普通消息处理
- 代码注释和标识符用英文
- **每次改完自动 commit + push**

## 本地开发

```bash
# 在 server/ 目录下执行
export OPENAI_API_KEY=sk-...
go run ./cmd/wheelmaker/                                    # 需先创建 ~/.wheelmaker/config.json
go test ./...
go build -o bin/windows_amd64/wheelmaker.exe ./cmd/wheelmaker/   # Windows
GOOS=linux  GOARCH=amd64 go build -o bin/linux_amd64/wheelmaker  ./cmd/wheelmaker/
GOOS=darwin GOARCH=arm64 go build -o bin/darwin_arm64/wheelmaker  ./cmd/wheelmaker/

# ACP 工具安装（首次）
pwsh scripts/install-tools.ps1   # Windows
bash scripts/install-tools.sh    # macOS / Linux
```

## Process Rule

每次代码变更后：终止所有已有 wheelmaker 进程，用 `go run ./cmd/wheelmaker/` 重启，确认只有一个进程运行。

## 关键协议文档

- ACP 协议：[../docs/acp-protocol-full.zh-CN.md](../docs/acp-protocol-full.zh-CN.md)
- 飞书 Bot：[../docs/feishu-bot.md](../docs/feishu-bot.md)
