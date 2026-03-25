# WheelMaker

> **Quick context:** Go daemon that proxies between local AI CLIs (Codex / Claude via ACP over stdio) and remote IM channels (Feishu / mobile WebSocket). The Flutter app is a thin chat client connecting over WebSocket.

本地 AI 编程 CLI（Codex、Claude 等）的远程控制桥接守护进程，通过飞书等 IM 让开发者在手机上远程操控本地 AI 助手。

## 仓库结构

```
WheelMaker/
  server/   — Go 守护进程（ACP 桥接、IM 适配器）
  app/      — Flutter 移动端 App（iOS / Android）
  docs/     — 共享协议与设计文档
  CLAUDE.md — 本文件：全局视图
  AGENTS.md — AI agent 入口
```

**根据工作区跳转到对应文档：**
- 修改 Go 服务端 → 读 [server/CLAUDE.md](server/CLAUDE.md)
- 修改 Flutter App → 读 [app/CLAUDE.md](app/CLAUDE.md)

## 全局约定

- 代码注释和标识符用英文
- **每次改完自动 commit + push**：每完成一次代码修改后，立即执行 `git add`、`git commit`、`git push`，无需等用户提示
- `docs/` 下的协议文档两端共享，修改需同时评估对 server 和 app 的影响

## Completion Gate (Highest Priority)

Before the final user-facing completion message in any implementation task, execute this exact tail sequence:

1. `git add -A`
2. `git commit -m "<message>"`
3. `git push origin <branch>`
4. If files under `server/` changed: `powershell -NoProfile -ExecutionPolicy Bypass -File scripts/delay_restart_server.ps1`
5. If step 4 ran: check process count and confirm only one `wheelmaker` process remains

If any step fails, report failure and keep working until resolved. Do not claim completion early.

## 关键协议文档

- ACP 协议：[docs/acp-protocol-full.zh-CN.md](docs/acp-protocol-full.zh-CN.md)
- 飞书 Bot：[docs/feishu-bot.md](docs/feishu-bot.md)

