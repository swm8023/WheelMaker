# WheelMaker 设计规范

> 版本：v0.1
> 日期：2026-03-14
> 状态：已批准

## 1. 项目目标

WheelMaker 是一个 Go 编写的长期驻留守护进程，让开发者通过手机 IM（初期为飞书）远程控制本地 AI 编程 CLI（Codex、Claude、Copilot 等）。

**核心问题**：离开电脑后无法与本地 AI 编程助手交互。
**解决方案**：在本地机器运行一个桥接守护进程，连接 IM 平台与本地 AI CLI 工具。

## 2. 整体架构

```
┌─────────────┐    WebSocket      ┌──────────────────────────────────┐
│  飞书 App    │ ◄───────────────► │          WheelMaker              │
│  (手机端)    │   (go-lark SDK)   │                                  │
└─────────────┘                   │  ┌──────────────────────────┐    │
                                  │  │         Hub              │    │
                                  │  │  (调度 / 状态 / 持久化)  │    │
                                  │  └────────────┬─────────────┘    │
                                  │               │                  │
                                  │  ┌────────────▼─────────────┐    │
                                  │  │      Agent Interface      │    │
                                  │  │  ┌──────────────────────┐ │    │
                                  │  │  │   Codex Adapter      │ │    │
                                  │  │  │  (ACP JSON-RPC)      │ │    │
                                  │  │  └──────────┬───────────┘ │    │
                                  │  └─────────────┼─────────────┘    │
                                  │                │ stdin/stdout     │
                                  │  ┌─────────────▼───────────┐     │
                                  │  │     codex-acp.exe        │     │
                                  │  │  (子进程，Rust 编写)     │     │
                                  │  └─────────────────────────┘     │
                                  └──────────────────────────────────┘
```

### 数据流

```
1. 飞书消息 → WebSocket → im/feishu Adapter → im.Message
2. Hub.HandleMessage(msg) → 解析命令 or 转发 Prompt
3. Agent.Prompt(ctx, text) → acp.Client.Send(session/prompt)
4. codex-acp.exe → session/update notifications (流式)
5. Update stream → Hub → im.Adapter.SendText() → 飞书
```

## 3. 目录结构

```
wheelmaker/
├── cmd/
│   └── wheelmaker/
│       └── main.go              # 启动入口
├── internal/
│   ├── acp/
│   │   ├── client.go            # JSON-RPC stdio 传输层
│   │   └── types.go             # 消息结构体
│   ├── agent/
│   │   ├── agent.go             # Agent interface + Update 类型
│   │   └── codex/
│   │       └── adapter.go       # Codex ACP 适配器
│   ├── im/
│   │   ├── im.go                # IM Adapter interface
│   │   └── feishu/
│   │       └── adapter.go       # 飞书 WebSocket 适配
│   ├── hub/
│   │   ├── hub.go               # 核心调度器
│   │   ├── store.go             # Store interface + JSONStore
│   │   └── state.go             # 持久化状态结构体
│   └── tools/
│       └── resolve.go           # 工具二进制路径解析
├── bin/
│   ├── windows_amd64/.gitkeep
│   ├── darwin_arm64/.gitkeep
│   ├── darwin_amd64/.gitkeep
│   ├── linux_amd64/.gitkeep
│   └── linux_arm64/.gitkeep
├── scripts/
│   ├── install-tools.sh         # Linux/macOS 安装脚本
│   └── install-tools.ps1        # Windows 安装脚本
├── docs/
│   ├── specs/                   # 设计规范（本文件）
│   ├── plan/                    # 实现计划
│   ├── acp-protocol-full.zh-CN.md
│   ├── feishu-bot.md
│   └── codex-acp.md
├── CLAUDE.md
├── AGENTS.md
├── go.mod
└── .gitignore
```

## 4. 核心接口定义

### 4.1 Agent 接口

```go
// internal/agent/agent.go

// Update 表示 agent 返回的一个流式更新单元
type Update struct {
    Type    string // "text" | "tool_call" | "thought" | "error"
    Content string
    Done    bool
    Err     error
}

// Agent 表示一个可交互的 AI 编程助手
type Agent interface {
    Name() string
    // Prompt 发送 prompt，返回流式 Update channel，调用方读完 channel 直到 Done=true
    Prompt(ctx context.Context, text string) (<-chan Update, error)
    Cancel() error
    SetMode(modeID string) error
    Close() error
}
```

### 4.2 IM Adapter 接口

```go
// internal/im/im.go

// Message 表示来自 IM 平台的一条消息
type Message struct {
    ChatID    string
    MessageID string
    UserID    string
    Text      string
}

// Card 表示富文本消息卡片（飞书交互卡片格式）
type Card map[string]any

// Adapter 表示一个 IM 平台适配器
type Adapter interface {
    // OnMessage 注册消息处理函数
    OnMessage(handler func(Message))
    SendText(chatID, text string) error
    SendCard(chatID string, card Card) error
    SendReaction(messageID, emoji string) error
    // Run 启动事件循环（阻塞直到 ctx 取消）
    Run(ctx context.Context) error
}
```

### 4.3 Hub 状态与持久化

```go
// internal/hub/state.go

// AgentConfig 单个 agent 的配置
type AgentConfig struct {
    ExePath string            // 工具二进制路径（空则由 tools.ResolveBinary 自动解析）
    Env     map[string]string // 额外环境变量（如 OPENAI_API_KEY）
}

// State 持久化到磁盘的全局状态
type State struct {
    ActiveAgent   string                 // 当前活跃 agent 名称，如 "codex"
    Agents        map[string]AgentConfig // agent 名 → 配置
    ACPSessionIDs map[string]string      // agent 名 → ACP sessionId（用于 session/load）
}

// internal/hub/store.go

// Store 持久化接口
type Store interface {
    Load() (*State, error)
    Save(s *State) error
}

// JSONStore 将 State 存储到本地 JSON 文件
type JSONStore struct {
    Path string
}
```

### 4.4 工具二进制解析

```go
// internal/tools/resolve.go

// ResolveBinary 按优先级解析工具二进制路径：
//   1. configPath 非空时直接使用
//   2. 查找 bin/{GOOS}_{GOARCH}/{name}[.exe]
//   3. 查找 PATH
func ResolveBinary(name string, configPath string) (string, error)
```

## 5. ACP 传输层设计

### 5.1 通信模型

`acp.Client` 启动 codex-acp 子进程，通过 stdin/stdout 进行 JSON-RPC 2.0 通信：

- 每条消息是一行 JSON（`\n` 分隔，消息内不含换行）
- 请求带 `id`，响应匹配对应 `id`
- Notification（`session/update`）无 `id`，由订阅者处理

### 5.2 ACPClient 内部结构

```
ACPClient
├── cmd *exec.Cmd              // 子进程
├── encoder *json.Encoder      // 写 stdin
├── pending map[int64]chan Response  // 等待中的请求
├── mu sync.Mutex
└── goroutine: readLoop()      // 持续读 stdout
    ├── 有 id → 分发到 pending[id] channel
    └── 无 id → 广播给所有 subscriber
```

### 5.3 生命周期时序

```
client.Start()
  → exec.Command("codex-acp")
  → readLoop goroutine

client.Send(initialize)
  → {"id":1,"method":"initialize","params":{...}}
  ← {"id":1,"result":{"agentCapabilities":{...}}}

client.Send(session/new)  // 或 session/load（若有 sessionId）
  → {"id":2,"method":"session/new","params":{"cwd":"...","mcpServers":[]}}
  ← {"id":2,"result":{"sessionId":"abc123"}}

client.Send(session/prompt)  // 异步通知流
  → {"id":3,"method":"session/prompt","params":{"sessionId":"abc123","prompt":"..."}}
  ← {"method":"session/update","params":{...}}  // 0 到 N 条
  ← {"id":3,"result":{"stopReason":"end_turn"}}

client.Send(session/cancel)  // 可选，取消进行中的 prompt
client.Close()
```

## 6. Hub 设计

### 6.1 职责

- **启动时**：从 Store 加载 State，根据 `ActiveAgent` 初始化对应 Agent（懒加载）
- **消息路由**：解析特殊命令（`/use <agent>`、`/cancel`、`/status`），其余转发给 Agent.Prompt()
- **流式转发**：将 Agent Update stream 实时推送到 IM（每个 text chunk 拼接后统一发送或分段发送）
- **关闭时**：保存 ACPSessionID 到 Store 供下次 session/load 使用

### 6.2 特殊命令

| 命令 | 说明 |
|------|------|
| `/use <name>` | 切换当前活跃 agent（如 `/use codex`、`/use claude`） |
| `/cancel` | 取消当前 agent 正在处理的请求 |
| `/status` | 返回当前状态（活跃 agent、ACP session 状态） |

## 7. 持久化

- 存储位置：`~/.wheelmaker/state.json`（或通过 `--state` 参数指定）
- 格式：JSON（人类可读，便于调试）
- 写入时机：session/new 成功后（保存 sessionId）、agent 切换时、进程退出时

## 8. 多平台工具管理

```
bin/
  {GOOS}_{GOARCH}/
    codex-acp[.exe]
    # 后续：claude[.exe]、copilot[.exe]
```

- `.gitignore` 忽略实际二进制，保留 `.gitkeep`
- `scripts/install-tools.sh`：自动下载对应平台的 codex-acp 到 `bin/{platform}/`
- `scripts/install-tools.ps1`：Windows 版本

## 9. 第二阶段（Feishu 接入）

飞书适配器将在第二阶段实现，使用 go-lark SDK 的 WebSocket 长连接模式：

- 无需公网 IP
- SDK 主动连接飞书 WebSocket 网关
- 事件通过 `EventTypeMessageReceived` 回调接收
- 发消息通过 `bot.PostText()`、`bot.PostCard()` 等方法

## 10. 错误处理原则

- 使用 `fmt.Errorf("...: %w", err)` 包装错误，保留调用链
- 使用 `errors.Is` / `errors.As` 判断错误类型
- 不使用 `panic`（除非是真正的程序员错误）
- ACP 通信错误：记录日志，向 IM 返回错误消息，不崩溃
