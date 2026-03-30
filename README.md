# WheelMaker

WheelMaker 是一个将本地 AI Coding CLI（Codex / Claude 等）与 IM（如飞书）连接起来的守护进程。你可以在手机端远程发指令，让本地项目由 AI 执行开发任务。

## 功能介绍

- 多项目管理：单进程管理多个项目，每个项目独立配置 IM 与 Agent。
- 多通道交互：支持 `console`（本地调试）与 `feishu`（线上使用）。
- 会话恢复：重启后可恢复历史会话状态。
- 懒启动与空闲回收：仅在收到请求时启动 Agent，空闲后自动回收。
- Registry 扩展（可选）：
  - 默认模式下 WheelMaker 可独立运行，不依赖 Registry。
  - 开启 Registry 后，支持远程项目发现、文件/Git 只读查询、状态同步事件等扩展能力。

## 安装及配置

### 1. 环境准备

- Go 1.22+
- Node.js（用于安装 ACP 相关 CLI）

可直接用 `npm` 安装 ACP CLI（推荐）：

```bash
npm install -g @zed-industries/codex-acp @zed-industries/claude-agent-acp
```

也可以交给安装脚本自动检查并补装缺失依赖（见下方 `install_server.ps1`）。

### 2. 配置文件

参考 `server/config.example.json`（默认使用飞书通道）：

```json
{
  "projects": [
    {
      "name": "local-dev",
      "debug": true,
      "path": "/path/to/your/project",
      "yolo": true,
      "im": { "type": "console" },
      "client": { "agent": "claude" }
    },
    {
      "name": "feishu-prod",
      "path": "/path/to/other/project",
      "yolo": false,
      "im": {
        "type": "feishu",
        "appID": "cli_xxx",
        "appSecret": "yyy"
      },
      "client": { "agent": "claude" }
    }
  ],
  "registry": {
    "listen": true,
    "port": 9630,
    "server": "127.0.0.1",
    "token": "",
    "hubId": "dev-hub"
  },
  "log": {
    "level": "warn"
  }
}
```

说明：

- `projects[]`：项目配置（必填）。
  - `im.type`：`console` 用于本地调试，`feishu` 用于线上飞书通道（需填入 `appID` / `appSecret`）。
- `registry`：Registry 扩展项（可选）。
  - `listen=true`：启动内置 Registry 服务。
  - `server/token/hubId`：用于 Hub 与 Registry 的连接与鉴权。

### 3. 一键构建、安装、拉起（Windows PowerShell）

仓库已提供脚本（位于 `scripts/`）：

- `build_server.ps1`：编译 `server/cmd/wheelmaker` 到 `server/bin/windows_amd64/wheelmaker.exe`
- `install_server.ps1`：安装/更新 `~/.wheelmaker/bin/wheelmaker.exe`，可自动安装 ACP 依赖
- `delay_restart_server.ps1`：延迟执行 build + install，并以守护模式重启 wheelmaker

推荐流程：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/build_server.ps1
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/install_server.ps1
# 首次安装：拷贝示例配置并编辑（填入飞书 appID/appSecret 等）
Copy-Item server\config.example.json ~\.wheelmaker\config.json
notepad ~\.wheelmaker\config.json
# 启动
~\.wheelmaker\start.ps1
```

安装脚本会在 `~/.wheelmaker/` 下生成以下便捷脚本：

| 脚本 | 功能 |
|------|------|
| `start.ps1` | 后台启动守护进程（已运行则跳过） |
| `stop.ps1` | 停止所有 wheelmaker 进程 |
| `restart.ps1` | 先 stop 再 start |

日常使用：

```powershell
~\.wheelmaker\start.ps1     # 启动
~\.wheelmaker\stop.ps1      # 停止
~\.wheelmaker\restart.ps1   # 重启
```

说明：

- 安装脚本会在 `~/.wheelmaker/config.json` 不存在时自动生成默认配置；如需自定义，建议先手动拷贝 example 并编辑。
- `-d` 为守护模式，会拉起 guardian/hub/registry worker 进程。
- 如果你希望“延迟重启并自动替换二进制”，可执行：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/delay_restart_server.ps1
```

### 4. 开发调试（可选）

- 直接运行（非守护）：

```bash
cd server
go run ./cmd/wheelmaker
```

- 测试与构建：

```bash
cd server
go test ./...
go build ./cmd/wheelmaker
```

- Web 开发服务重启脚本（默认端口 8080）：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File app/scripts/restart_web.ps1 -Port 8080
```

## 架构设计

### 核心链路

```text
IM (Feishu/Console)
  -> Hub (WheelMaker)
    -> Agent Adapter (Codex/Claude ACP)
      -> Local Project Workspace
```

守护模式下（`wheelmaker.exe -d`）会由 guardian 进程管理 Hub 与 Registry worker 生命周期。

### Registry 扩展链路（可选）

```text
Hub (project runtime + git/fs providers)
  <-> Registry (routing, auth, project index)
  <-> Client (web/mobile observer)
```

### 模块分层

- `server/internal/hub/`：Hub 领域模型与调度。
- `server/internal/hub/client/`：单项目生命周期、消息路由、会话管理。
- `server/internal/hub/acp/`：ACP 会话与 JSON-RPC 传输。
- `server/internal/hub/im/`：IM 适配层（console / feishu）。
- `server/internal/registry/`：Registry 服务与转发。
- `server/internal/shared/`：共享协议、配置、日志等基础组件。

### 协议文档

- 主协议（2.0）：`docs/registry-protocol.md`
- 历史 1.0：`docs/registry-server-remote-observe-protocol-v1.md`
