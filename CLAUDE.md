# WheelMaker

本地 AI 编程 CLI（Codex、Claude 等）的远程控制桥接守护进程，通过飞书等 IM 让开发者在手机上远程操控本地 AI 助手。

## 架构层次

```
Hub (internal/hub/)             — 多 project 生命周期管理，读 config.json
  ├─ im/forwarder.Forwarder     — IM 包装层：消息路由、决策请求管理、HelpResolver
  │    └─ console / feishu      — 底层 IM 适配器（stdin / 飞书 Bot）
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
| `internal/tools/` | 工具二进制路径解析（`bin/{GOOS}_{GOARCH}/`） |

## 配置文件

- `~/.wheelmaker/config.json` — 项目配置（IM 类型、agent、工作目录）
- `~/.wheelmaker/state.json` — 运行时状态持久化（session ID、agent 元数据、session 状态）

config.json 格式：
```json
{
  "projects": [
    { "name": "local", "debug": true, "im": { "type": "console" },
      "client": { "agent": "claude", "path": "/your/project" } }
  ]
}
```

## state.go 设计

`internal/client/state.go` 定义序列化结构，用于：
1. 跨进程持久化运行时状态（sessionID、agent 元数据、最近 session 状态）
2. 启动时恢复上次连接（session/load）

```
FileState
  └─ Projects map[name]*ProjectState
       ├─ ActiveAgent string               — 当前激活的 agent 名称
       ├─ Connection *ConnectionConfig    — 最近一次 initialize 时发送的客户端参数
       └─ Agents map[name]*AgentState     — 每个 agent 的持久化元数据
            ├─ LastSessionID              — 下次启动时传给 session/load
            ├─ ProtocolVersion / AgentCapabilities / AgentInfo / AuthMethods
            │                             — initialize 响应的 agent 级别数据
            ├─ Session *SessionState      — 最后使用的 session 状态（非全量）
            │    └─ Modes / Models / ConfigOptions / AvailableCommands / Title / UpdatedAt
            └─ Sessions []SessionSummary  — 已知 session 的轻量列表（按需填充，不自动维护）
```

**设计原则：**
- 每个 agent 只存最后一次使用的 session 状态；切换 session 时会同步更新
- `Sessions` 列表是懒加载的：仅在用户查询历史时写入，不随每次 prompt 自动更新
- `Connection` 在 initialize 握手完成后由 `persistAgentMeta` 写入，所有 agent 共用同一份客户端参数

## 开发约定

- **接口优先**：跨层依赖通过接口（`acp.Session`、`agent.Agent`、`im.Channel`）
- **懒加载**：agent 子进程在首条消息时才创建（`ensureForwarder`）
- **支持的命令**：`/use`、`/cancel`、`/status`、`/mode`、`/model`、`/list`、`/new`、`/load`、`/debug`；其他 `/` 开头文本当普通消息处理
- 代码注释和标识符用英文
- **每次改完自动 commit + push**：每完成一次代码修改后，立即执行 `git add`、`git commit`、`git push`，无需等用户提示

## 本地开发

```bash
export OPENAI_API_KEY=sk-...
go run ./cmd/wheelmaker/   # 需先创建 ~/.wheelmaker/config.json
go test ./...
go build ./cmd/wheelmaker/
```

## 关键协议文档

- ACP 协议：[docs/acp-protocol-full.zh-CN.md](docs/acp-protocol-full.zh-CN.md)
- 飞书 Bot：[docs/feishu-bot.md](docs/feishu-bot.md)
