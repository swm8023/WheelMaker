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

可选安装 ACP CLI：

```bash
npm install -g @zed-industries/codex-acp @zed-industries/claude-agent-acp
```

### 2. 配置文件

参考 `server/config.example.json`：

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
- `registry`：Registry 扩展项（可选）。
  - `listen=true`：启动内置 Registry 服务。
  - `server/token/hubId`：用于 Hub 与 Registry 的连接与鉴权。

### 3. 启动

```bash
cd server
go run ./cmd/wheelmaker
```

常用验证：

```bash
go test ./...
go build ./cmd/wheelmaker
```

## 架构设计

### 核心链路

```text
IM (Feishu/Console)
  -> Hub (WheelMaker)
    -> Agent Adapter (Codex/Claude ACP)
      -> Local Project Workspace
```

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
