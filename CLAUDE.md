# WheelMaker

本地 AI 编程 CLI（Codex、Claude 等）的远程控制桥接守护进程，通过飞书等 IM 让开发者在手机上远程操控本地 AI 助手。

## 架构层次

```
Hub (internal/hub/)          — 多 project 生命周期管理，读 config.json
  └─ client.Client           — 单 project 协调：命令路由、agent 懒加载、idle 超时、状态持久化
       └─ agent.Agent        — ACP 协议封装：会话、prompt 流、fs/terminal/permission 回调
            └─ adapter.Adapter → acp.Conn  — 连接工厂 → JSON-RPC 2.0 over stdio → CLI binary
```

## 包职责

| 包 | 职责 |
|----|------|
| `internal/hub/` | 读 `~/.wheelmaker/config.json`，为每个 project 创建 Client + IM |
| `internal/client/` | 单 project 协调：命令路由、lazy agent init、idle 30min 超时、state 持久化 |
| `internal/agent/` | ACP 会话生命周期、prompt 流、入站回调（fs/terminal/permission） |
| `internal/agent/acp/` | JSON-RPC 2.0 传输层，管理子进程 stdio |
| `internal/adapter/codex/` | 启动 codex-acp 二进制，返回 `*acp.Conn` |
| `internal/im/console/` | Console IM：读 stdin，debug 模式打印所有 ACP JSON |
| `internal/im/feishu/` | 飞书 Bot IM adapter |
| `internal/tools/` | 工具二进制路径解析（`bin/{GOOS}_{GOARCH}/`） |

## 配置文件

- `~/.wheelmaker/config.json` — 项目配置（IM 类型、adapter、工作目录）
- `~/.wheelmaker/state.json` — 运行时状态（per-project sessionID 持久化）

config.json 格式：
```json
{
  "projects": [
    { "name": "local", "im": { "type": "console", "debug": true },
      "client": { "adapter": "codex", "path": "/your/project" } }
  ]
}
```

## 开发约定

- **接口优先**：跨层依赖通过接口（`agent.Session`、`adapter.Adapter`、`im.Adapter`）
- **懒加载**：agent 子进程在首条消息时才创建；idle 30min 自动关闭并存盘，下次恢复
- **命令判断**：仅 `/use`、`/cancel`、`/status` 是命令，其他 `/` 开头文本当普通消息处理
- 代码注释和标识符用英文

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
