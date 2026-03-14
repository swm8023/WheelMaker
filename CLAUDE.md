# WheelMaker

## 项目目标

本地 AI 编程 CLI（Codex、Claude、Copilot 等）的远程控制桥接守护进程。
通过飞书等 IM 平台，让开发者在手机上远程操控本地 AI 编程助手。

## 整体架构



## 包职责

## 开发约定

- **接口优先**：所有跨层依赖通过接口，不依赖具体实现
- **不过度抽象**：先让它工作，再考虑扩展
- **懒加载**：agent 子进程在首次 Prompt 时才 spawn
- 工具二进制放 `bin/{GOOS}_{GOARCH}/`，通过 `internal/tools` 解析

## 关键协议文档

- ACP 协议：[docs/acp-protocol-full.zh-CN.md](docs/acp-protocol-full.zh-CN.md)
- codex-acp：[docs/codex-acp.md](docs/codex-acp.md)
- 飞书 Bot：[docs/feishu-bot.md](docs/feishu-bot.md)

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
