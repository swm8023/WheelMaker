# WheelMaker

WheelMaker 是一个自托管守护进程，让你可以在手机或浏览器里远程使用本地代码仓库上的 AI 编程能力。

> Feishu / Mobile App / Web UI → WheelMaker → Claude / Codex / Copilot → your codebase

![双机拓扑示意](docs/readme-assets/topology-dual-machine.svg)

## 使用

### 适用场景

下面这套示例按你当前的部署形态组织：

- **机器 A**
  - 运行一个 hub
  - 打开 registry 服务
  - 打开 monitor 服务
  - 承载 Web 静态资源
  - 前置 Nginx，统一暴露 HTTPS 入口
- **机器 B**
  - 运行另一个 hub
  - 通过 registry 接入机器 A
- **访问方**
  - 浏览器 / 手机统一访问机器 A 的 HTTPS 地址

这个模式适合：

- 一台机器集中暴露 Web、Monitor、Registry；
- 另一台机器只负责挂项目和 agent；
- 外部只需要记住一个 HTTPS 入口。

### 当前示例拓扑

![当前 Nginx 与服务路由](docs/readme-assets/nginx-routing.svg)

### 入口与端口

| 位置 | 入口 | 当前用途 |
| --- | --- | --- |
| 机器 A / Nginx | `https://<host>:28800/` | Web UI 首页 |
| 机器 A / Nginx | `wss://<host>:28800/ws` | Registry WebSocket 入口 |
| 机器 A / Nginx | `https://<host>:28800/monitor/` | Monitor 页面 |
| 机器 A / 内部 | `127.0.0.1:8080` | Web 静态资源 (`C:\Users\suweimin\.wheelmaker\web`) |
| 机器 A / 内部 | `127.0.0.1:9630` | Registry 监听 |
| 机器 A / 内部 | `127.0.0.1:9631` | Monitor 监听 |
| 机器 A / Nginx | `https://<host>:33006/` | 当前另一组 HTTPS 反代，代理到 `127.0.0.1:3006` |

### 1. 刷新、构建、安装服务

需要：

- **Go 1.22+**
- **Node.js 22+**
- Windows 管理员终端

一键刷新：

```bat
deploy.bat
```

或者直接执行：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/refresh_server.ps1
```

这个脚本会：

- 在工作区干净时 `git pull --ff-only`
- 安装 ACP CLI 依赖（如缺失）
- 构建 `wheelmaker.exe`、`wheelmaker-monitor.exe`、`wheelmaker-updater.exe`
- 部署到 `~\.wheelmaker\bin\`
- 保留或初始化 `~\.wheelmaker\config.json`
- 生成 `start.bat` / `stop.bat`
- 注册或更新 Windows 服务：
  - `WheelMaker`
  - `WheelMakerMonitor`
  - `WheelMakerUpdater`
- 启动服务并设置自动启动

如果第一次创建 `config.json`，脚本会在重启前停下，让你先把配置填完整。

### 2. 配置机器 A（hub + registry + monitor + web）

编辑：

```powershell
notepad ~/.wheelmaker/config.json
```

机器 A 示例：

```json
{
  "projects": [
    {
      "name": "WheelMaker",
      "path": "D:\\Code\\WheelMaker",
      "feishu": {
        "app_id": "cli_xxx",
        "app_secret": "yyy"
      }
    }
  ],
  "registry": {
    "listen": true,
    "port": 9630,
    "server": "127.0.0.1",
    "token": "replace-with-shared-token",
    "hubId": "hub-a"
  },
  "monitor": {
    "port": 9631
  },
  "log": {
    "level": "warn"
  }
}
```

说明：

- `registry.listen: true` 表示机器 A 自己承载 registry 服务。
- `registry.port` 是 registry 内部监听端口。
- `registry.token` 是给其他 hub 与 Web 客户端共用的认证 token。
- `registry.hubId` 建议填稳定、可识别的名称，例如 `hub-a`。
- `monitor.port` 是 monitor 内部监听端口，通常由 Nginx 转发出去。

### 3. 配置机器 B（仅 hub，接入机器 A 的 registry）

机器 B 不承担公网入口，只把自己的项目汇报给机器 A 的 registry。

```json
{
  "projects": [
    {
      "name": "Project-B",
      "path": "D:\\Code\\Project-B",
      "feishu": {
        "app_id": "cli_xxx",
        "app_secret": "yyy"
      }
    }
  ],
  "registry": {
    "listen": false,
    "port": 9630,
    "server": "https://machine-a.example.com:28800",
    "token": "replace-with-shared-token",
    "hubId": "hub-b"
  },
  "monitor": {
    "port": 9631
  },
  "log": {
    "level": "warn"
  }
}
```

说明：

- `registry.server` 可以写 `https://machine-a.example.com:28800`。WheelMaker 会自动转换为 `wss://.../ws`。
- `registry.token` 必须和机器 A 一致。
- `hubId` 必须全局唯一，例如 `hub-b`。
- `listen: false` 表示机器 B 不自己对外承载 registry。

### 4. 当前 Nginx + HTTPS 示例

你现在的 Nginx 位于：

```text
D:\Nginx\nginx-1.29.5\
```

其中当前核心路由在：

```text
D:\Nginx\nginx-1.29.5\conf\conf.d\proxy-28800.conf
```

对应思路如下：

```nginx
server {
    listen 28800 ssl;
    server_name _;

    ssl_certificate         D:/Nginx/cert/stunnel.pem;
    ssl_certificate_key     D:/Nginx/cert/stunnel.pem;
    ssl_trusted_certificate D:/Nginx/cert/uca.pem;

    ssl_protocols TLSv1.2 TLSv1.3;

    location / {
        proxy_pass http://127.0.0.1:8080/;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto https;
        proxy_http_version 1.1;
    }

    location /ws {
        proxy_pass http://127.0.0.1:9630;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_read_timeout 3600s;
        proxy_send_timeout 3600s;
        proxy_buffering off;
    }

    location = /monitor {
        return 301 /monitor/;
    }

    location ^~ /monitor/ {
        proxy_pass http://127.0.0.1:9631;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_http_version 1.1;
    }
}
```

这份配置的职责是：

- `/` → Web UI
- `/ws` → Registry WebSocket
- `/monitor/` → Monitor

你当前还保留了另一组示例入口：

```nginx
server {
    listen 33006 ssl;
    server_name _;

    location / {
        proxy_pass http://127.0.0.1:3006/;
    }
}
```

它适合作为另一类本地服务的 HTTPS 透传模板。

### 5. 发布 Web UI

Web 构建产物会直接发布到：

```text
C:\Users\<YourUser>\.wheelmaker\web
```

执行：

```powershell
cd app
npm run build:web:release
```

这会：

1. 构建 Web 前端
2. 把静态资源发布到 `~\.wheelmaker\web`
3. 让 Nginx 的 `127.0.0.1:8080` 站点直接读取最新页面

### 6. 服务操作

```powershell
~/.wheelmaker/start.bat
~/.wheelmaker/stop.bat
~/.wheelmaker/refresh_server.ps1
```

默认刷新流程：

```text
update -> build -> stop -> deploy -> restart
```

支持跳过阶段：

- `-SkipUpdate`
- `-SkipBuild`
- `-SkipStop`
- `-SkipDeploy`
- `-SkipRestart`

也可以手动触发 updater：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/signal_update_now.ps1 -DelaySeconds 30
```

### 7. 最短检查清单

部署完成后，先检查这几项：

1. 浏览器打开 `https://<host>:28800/`，能进入 Web UI
2. 浏览器打开 `https://<host>:28800/monitor/`，能进入 Monitor
3. 机器 B 的 `registry.server` 指向机器 A 的 HTTPS 地址
4. 机器 A / B 使用相同的 `registry.token`
5. Web UI 中能看到来自不同 hub 的项目

### 常用聊天命令

| Command | Description |
| --- | --- |
| `/use <agent>` | 切换 AI agent（claude / codex / copilot） |
| `/new` | 新建 session |
| `/list` | 列出 session |
| `/load <id>` | 恢复 session |
| `/cancel` | 取消当前 agent 操作 |
| `/status` | 查看项目与 agent 状态 |
| `/mode` | 切换 YOLO mode |
| `/model` | 切换 agent model |
| `/help` | 显示命令列表 |

## 功能

### App 界面总览

![App 界面示意](docs/readme-assets/ui-app-workspace.svg)

Web App 不是只有聊天窗口，而是一个把 **项目、会话、文件、Git、渲染与连接状态** 放在同一工作区里的远程控制台。

### 1. 聊天与会话

![统一后的 Session Picker 示意](docs/readme-assets/ui-session-picker.svg)

你可以直接在 App 里完成完整的会话管理：

- 新建 session
- 恢复历史 session
- 会话列表切换
- session 即时 reload
- 多 agent 会话并存
- 会话级配置项展示

聊天区支持：

- 流式消息展示
- 增量同步
- 历史补载
- 图片附件发送
- 从聊天内容里直接跳转到文件路径

### 2. 项目与工作区

App 会把 registry 中的项目聚合成一个工作区视图，适合多 hub、多项目一起挂载：

- 多项目切换
- 展示项目在线状态
- 展示当前 agent
- 支持跨 hub 聚合项目
- 支持从工作区直接切换到 Chat / File / Git

### 3. 文件浏览与阅读

文件能力不只是“打开一个文本框”：

- 目录树浏览
- 文件打开与缓存复用
- `notModified` 命中时避免重复加载
- pinned files
- 文件滚动位置恢复
- 文件链接跳转
- 大文件、二进制文件的保护性处理

这意味着你可以在手机或浏览器里快速定位仓库结构，而不是只能靠 AI 文本回复猜文件位置。

### 4. Git 浏览

Git 视图覆盖了日常排查最常用的几层信息：

- 当前 branch
- dirty 状态
- staged / unstaged / untracked 汇总
- commit 列表
- commit 文件列表
- commit diff
- working tree diff
- branch 过滤与 commit popover

对于移动端或远程查看来说，这一层非常关键：你不需要 SSH 回去看一眼 `git status` 或 `git show`。

### 5. 富内容渲染

聊天内容与代码展示支持丰富的渲染能力：

- Markdown
- 表格
- KaTeX 数学公式
- Mermaid 图
- 代码高亮
- diff 渲染
- 主题、字体、字号、行高、Tab Size 设置

这让 App 不只是“消息窗口”，而更像一个轻量级远程代码工作台。

### 6. 连接恢复与 PWA

App 针对移动网络和页面切后台做了专门处理：

- silent reconnect
- reconnect 期间工作区保持可见
- session / file 按需恢复
- PWA 支持
- 本地通知
- service worker 缓存基础资源

即使网络不稳，也尽量避免每次重连都把当前工作上下文清空。

### 7. 设置与观测

![Monitor 界面示意](docs/readme-assets/ui-monitor.svg)

除了主工作区，WheelMaker 还提供一套观测与配置入口：

- runtime / registry 地址配置
- token provider / token stats（包括 DeepSeek token stats）
- Monitor 页面查看服务状态
- 查看日志
- 查看 hub / registry 项目状态
- 执行 start / stop / restart / update-publish 等运维动作

对多 hub 环境来说，Monitor 是“看系统有没有活着”的第一入口。

## 仓库结构

```text
WheelMaker/
  server/   — Go daemon（hub、agent 适配、registry、monitor）
  app/      — React Native 移动端 + Web dashboard
  docs/     — 协议、设计文档与 README 图示资产
  scripts/  — 构建、部署、更新脚本
```

## 开发

服务端本地运行：

```powershell
cd server
go run ./cmd/wheelmaker
go test ./...
```

Web 侧常用命令：

```powershell
cd app
npm test -- --runInBand
npm run tsc:web
npm run build:web:release
```

脚本总览：

- `scripts\refresh_server.ps1` — 服务优先的构建与部署
- `scripts\signal_update_now.ps1` — 异步触发 updater
- `app\scripts\export_web_release.ps1` — 发布 Web 资源到 `~\.wheelmaker\web`

## License

Private — all rights reserved.
