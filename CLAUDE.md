# WheelMaker

## 项目目标

本地 AI 编程 CLI（Codex、Claude、Copilot 等）的远程控制桥接守护进程。
通过飞书等 IM 平台，让开发者在手机上远程操控本地 AI 编程助手。

## 整体架构

```
飞书 App (手机)
    ↕ WebSocket 长连接 (go-lark SDK)
internal/im/feishu    — IM 适配层
    ↕ im.Message / im.Adapter
internal/hub          — 核心调度（agent 切换、持久化、命令解析）
    ↕ agent.Agent
internal/agent/codex  — Codex ACP 适配器
    ↕ stdin/stdout JSON-RPC 2.0
codex-acp.exe         — 子进程（ACP 协议实现）
```

## 包职责

| 包 | 职责 |
|----|------|
| `internal/acp` | 低层 ACP JSON-RPC stdio 传输（spawn 子进程、读写 stdin/stdout） |
| `internal/agent` | Agent 接口定义 + Update 类型 |
| `internal/agent/codex` | Codex ACP 适配器（实现 Agent 接口） |
| `internal/im` | IM Adapter 接口定义 + Message 类型 |
| `internal/im/feishu` | 飞书 WebSocket 适配器（实现 Adapter 接口） |
| `internal/hub` | 核心调度器：agent 切换、命令解析、状态持久化 |
| `internal/tools` | 工具二进制路径解析（跨平台） |
| `cmd/wheelmaker` | 启动入口，组装各层 |

## 开发约定

- **接口优先**：所有跨层依赖通过接口，不依赖具体实现
- **不过度抽象**：先让它工作，再考虑扩展
- **懒加载**：agent 子进程在首次 Prompt 时才 spawn
- 工具二进制放 `bin/{GOOS}_{GOARCH}/`，通过 `internal/tools` 解析

## 关键协议文档

- ACP 协议：[docs/acp-protocol-full.zh-CN.md](docs/acp-protocol-full.zh-CN.md)
- codex-acp：[docs/codex-acp.md](docs/codex-acp.md)
- 飞书 Bot：[docs/feishu-bot.md](docs/feishu-bot.md)
- 设计规范：[docs/specs/2026-03-14-wheelmaker-design.md](docs/specs/2026-03-14-wheelmaker-design.md)
- MVP 计划：[docs/plan/2026-03-14-wheelmaker-mvp-plan.md](docs/plan/2026-03-14-wheelmaker-mvp-plan.md)

## 本地开发

```bash
# 安装工具二进制
./scripts/install-tools.sh       # Linux/macOS
./scripts/install-tools.ps1      # Windows

# 设置环境变量
export OPENAI_API_KEY=sk-...

# 运行（stdin 模拟模式）
go run ./cmd/wheelmaker/

# 测试
go test ./internal/acp/...
go test ./internal/agent/codex/...
go test ./internal/hub/...

# 构建
go build ./cmd/wheelmaker/
```

## 特殊命令（发送给 Bot 的命令）

| 命令 | 说明 |
|------|------|
| `/use <agent>` | 切换 agent（如 `/use codex`） |
| `/cancel` | 取消当前请求 |
| `/status` | 查看当前状态 |
