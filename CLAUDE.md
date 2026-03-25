## 仓库结构
```
WheelMaker/
  server/   — Go 守护进程（ACP 桥接、IM 适配器）
  app/      — Flutter 移动端 App（iOS / Android）
  docs/     — 共享协议与设计文档
  scripts/  — 脚本
```

**根据工作区跳转到对应文档：**
- 修改 Go 服务端 → 读 [server/CLAUDE.md](server/CLAUDE.md)
- 修改 Flutter App → 读 [app/CLAUDE.md](app/CLAUDE.md)

## 全局约定
- 代码注释和标识符用英文

## Completion Gate (Highest Priority)
Before the final user-facing completion message in any implementation task, execute this exact tail sequence:
1. `git add -A`
2. `git commit -m "<message>"`
3. `git push origin <branch>`
4. If files under `server/` changed: `powershell -NoProfile -ExecutionPolicy Bypass -File scripts/delay_restart_server.ps1`

If any step fails, report failure and keep working until resolved. Do not claim completion early.

