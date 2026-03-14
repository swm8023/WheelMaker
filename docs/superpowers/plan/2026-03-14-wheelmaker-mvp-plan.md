# WheelMaker MVP 实现计划

> 版本：v0.1
> 日期：2026-03-14
> 阶段：Phase 1 — 项目基础 + ACP 客户端

## MVP 目标

1. 建立完整项目骨架（目录、go.mod、CLAUDE.md、AGENTS.md）
2. 实现 ACP 客户端（Go 调用 codex-acp.exe via stdio JSON-RPC）
3. 整理协议文档到 docs/（ACP + Feishu Bot + codex-acp）

**不在 MVP 内**：Feishu WebSocket 接入（Phase 2）

## 实现步骤

### Step 1：项目脚手架

**目标文件**：
- `go.mod`
- `.gitignore`
- `CLAUDE.md`
- `AGENTS.md`
- 全部目录及 `.gitkeep` 文件

**任务**：

1. 初始化 Go module：
   ```bash
   cd d:/Code/WheelMaker
   go mod init github.com/swm8023/wheelmaker
   ```

2. 创建 `.gitignore`：
   ```gitignore
   # 工具二进制（通过 scripts/install-tools.sh 下载）
   bin/**/*
   !bin/**/.gitkeep

   # 状态文件
   .wheelmaker/

   # Go
   *.exe
   *.test
   /vendor/
   ```

3. 创建目录骨架（含 `.gitkeep`）：
   ```
   cmd/wheelmaker/
   internal/acp/
   internal/agent/codex/
   internal/im/feishu/
   internal/hub/
   internal/tools/
   bin/windows_amd64/
   bin/darwin_arm64/
   bin/darwin_amd64/
   bin/linux_amd64/
   bin/linux_arm64/
   scripts/
   ```

4. 创建 `CLAUDE.md` 和 `AGENTS.md`（见下方内容规划）

5. 创建 `scripts/install-tools.sh` 和 `scripts/install-tools.ps1`

### Step 2：类型与接口定义

**目标文件**：
- `internal/agent/agent.go`
- `internal/im/im.go`
- `internal/hub/state.go`
- `internal/hub/store.go`

纯接口和结构体定义，无具体实现逻辑。详见设计规范 §4。

### Step 3：ACP 传输层

**目标文件**：
- `internal/acp/types.go` — JSON-RPC 结构体
- `internal/acp/client.go` — ACPClient 实现

**关键实现点**：

```go
// types.go
type Request struct {
    JSONRPC string `json:"jsonrpc"`
    ID      int64  `json:"id,omitempty"`
    Method  string `json:"method"`
    Params  any    `json:"params,omitempty"`
}

type Response struct {
    JSONRPC string          `json:"jsonrpc"`
    ID      int64           `json:"id"`
    Result  json.RawMessage `json:"result,omitempty"`
    Error   *RPCError       `json:"error,omitempty"`
}

type Notification struct {
    JSONRPC string `json:"jsonrpc"`
    Method  string `json:"method"`
    Params  any    `json:"params,omitempty"`
}

// client.go 核心方法
type Client struct { ... }

func New(exePath string, env []string) *Client
func (c *Client) Start() error
func (c *Client) Send(ctx context.Context, method string, params any, result any) error
func (c *Client) Subscribe(handler func(Notification)) (cancel func())
func (c *Client) Close() error
```

**readLoop 实现要点**：
- `bufio.Scanner` 逐行读 stdout
- 按是否有 `id` 字段区分 Response vs Notification
- Response：写入 `pending[id]` channel
- Notification：广播给所有 subscriber（用 goroutine 调用，避免阻塞）

### Step 4：Codex Agent 适配器

**目标文件**：
- `internal/agent/codex/adapter.go`

```go
type CodexAgent struct {
    name   string
    client *acp.Client
    sessID string  // ACP sessionId，空表示未初始化
    mu     sync.Mutex
}

func New(cfg hub.AgentConfig) *CodexAgent

// 懒初始化：首次调用时 Start client → initialize → session/new or session/load
func (a *CodexAgent) ensureSession(ctx context.Context) error

func (a *CodexAgent) Prompt(ctx context.Context, text string) (<-chan agent.Update, error)
```

**Prompt 实现要点**：
1. `ensureSession(ctx)` 确保 ACP 连接和 session 就绪
2. 订阅 Notification（`Subscribe`）
3. 发送 `session/prompt`（异步，不等 result）
4. 将 `session/update` notification 转换为 `agent.Update` 并写入 channel
5. 收到 `session/prompt` result 后，写入 `Update{Done:true}` 关闭 channel
6. 取消订阅

### Step 5：Hub

**目标文件**：
- `internal/hub/hub.go`

```go
type Hub struct {
    store  Store
    state  *State
    agents map[string]agent.Agent  // 已初始化的 agent 实例
    im     im.Adapter              // 可为 nil（MVP 阶段）
    mu     sync.Mutex
}

func New(store Store, im im.Adapter) *Hub
func (h *Hub) Start(ctx context.Context) error
func (h *Hub) HandleMessage(msg im.Message)
func (h *Hub) Close() error
```

**命令解析**：
- `/use <agent>` → 切换 `state.ActiveAgent`，保存 state
- `/cancel` → 调用当前 agent.Cancel()
- `/status` → 返回状态字符串
- 其他 → 转发给当前 agent.Prompt()

### Step 6：工具路径解析

**目标文件**：
- `internal/tools/resolve.go`

```go
func ResolveBinary(name string, configPath string) (string, error) {
    // 1. 使用配置路径
    if configPath != "" {
        if _, err := os.Stat(configPath); err == nil {
            return configPath, nil
        }
    }
    // 2. 查找 bin/{GOOS}_{GOARCH}/
    exe := name
    if runtime.GOOS == "windows" {
        exe += ".exe"
    }
    binPath := filepath.Join("bin", runtime.GOOS+"_"+runtime.GOARCH, exe)
    if _, err := os.Stat(binPath); err == nil {
        return filepath.Abs(binPath)
    }
    // 3. 查找 PATH
    return exec.LookPath(name)
}
```

### Step 7：入口

**目标文件**：
- `cmd/wheelmaker/main.go`

MVP 阶段提供简单的 stdin 测试模式：

```go
func main() {
    store := hub.NewJSONStore(".wheelmaker/state.json")
    h := hub.New(store, nil)  // 暂无 IM
    ctx := context.Background()
    h.Start(ctx)

    // 从 stdin 读取测试消息
    scanner := bufio.NewScanner(os.Stdin)
    for scanner.Scan() {
        h.HandleMessage(im.Message{
            ChatID: "cli",
            Text:   scanner.Text(),
        })
    }
    h.Close()
}
```

### Step 8：安装脚本

**`scripts/install-tools.sh`**：
```bash
#!/usr/bin/env bash
# 下载 codex-acp 到 bin/{platform}/
GOOS=$(go env GOOS)
GOARCH=$(go env GOARCH)
DEST="bin/${GOOS}_${GOARCH}"
mkdir -p "$DEST"
# 通过 npx 获取后复制，或直接从 GitHub releases 下载
npx --yes @zed-industries/codex-acp --version  # 触发安装
NPXBIN=$(npx --yes which codex-acp 2>/dev/null || true)
if [ -n "$NPXBIN" ]; then
    cp "$NPXBIN" "$DEST/codex-acp"
    chmod +x "$DEST/codex-acp"
fi
```

**`scripts/install-tools.ps1`**：
```powershell
$dest = "bin\windows_amd64"
New-Item -ItemType Directory -Force -Path $dest
# 通过 npx 安装后获取路径
npx --yes @zed-industries/codex-acp --version
$npxBin = (npx --yes which codex-acp 2>$null)
if ($npxBin) {
    Copy-Item $npxBin "$dest\codex-acp.exe"
}
```

### Step 9：协议文档整理

- `docs/feishu-bot.md`：飞书 Bot 协议摘要
- `docs/codex-acp.md`：codex-acp 使用摘要

## 文档内容规划

### CLAUDE.md

```markdown
# WheelMaker

## 项目目标
本地 AI 编程 CLI（Codex 等）的远程控制桥接器，通过飞书等 IM 远程操作。

## 架构
- cmd/wheelmaker/  — 入口
- internal/acp/    — ACP JSON-RPC stdio 传输
- internal/agent/  — Agent 接口 + 各 CLI 适配器
- internal/im/     — IM 接口 + 飞书适配器
- internal/hub/    — 核心调度，管理 agent 切换和持久化
- internal/tools/  — 工具二进制路径解析
- bin/{platform}/  — 第三方工具二进制（.gitignored）
- scripts/         — 安装脚本

## 开发约定
- 接口优先：所有跨层依赖通过接口
- 不过度抽象：先让它工作，再考虑扩展
- ACP 协议参考：docs/acp-protocol-full.zh-CN.md

## 本地测试
go run ./cmd/wheelmaker/  # 从 stdin 输入测试消息
go test ./internal/acp/...
```

### AGENTS.md

```markdown
# AI Agent 开发规范

## 代码风格
- 使用 gofmt / goimports
- 函数长度不超过 50 行，超出则拆分
- 不使用 init()

## 包职责
- 每个包只做一件事
- 不在 im/ 层处理 agent 逻辑
- 不在 agent/ 层处理 IM 格式

## 错误处理
- 使用 fmt.Errorf("context: %w", err) 包装
- 不使用 panic（除非是不可恢复的程序员错误）
- 向上层暴露有意义的错误信息

## 禁止事项
- 不硬编码 API key 或路径
- 不在代码中存储凭证
- 不绕过 Agent/IM 接口直接访问实现细节
```

## 验证计划

### ACP 连接验证

```bash
# 1. 安装 codex-acp
npm install -g @zed-industries/codex-acp
# 或通过脚本
./scripts/install-tools.sh

# 2. 运行单元测试（需要 OPENAI_API_KEY）
export OPENAI_API_KEY=sk-...
go test ./internal/acp/... -v
go test ./internal/agent/codex/... -v -run TestPrompt

# 3. 端到端测试：运行 main，通过 stdin 输入
go run ./cmd/wheelmaker/
# 输入: /status
# 输入: 解释一下 Go 的 goroutine
```

### 验收标准

- [ ] `go build ./...` 无错误
- [ ] `go vet ./...` 无警告
- [ ] ACP client 能成功 spawn codex-acp，完成 initialize + session/new
- [ ] Prompt 发送后能收到流式文本更新
- [ ] `/use` 命令能切换 agent，状态持久化到文件
- [ ] 进程重启后能通过 session/load 恢复 session

## 依赖

```
go.mod 预期依赖：
（暂无第三方 Go 依赖，仅标准库）

Phase 2 添加：
github.com/go-lark/lark/v2  # 飞书 SDK
```

## Phase 2 预览（飞书接入）

Phase 2 实现 `internal/im/feishu/adapter.go`：

```go
import "github.com/go-lark/lark/v2"

type FeishuAdapter struct {
    bot     *lark.Bot
    handler func(im.Message)
}

func New(appID, appSecret string) *FeishuAdapter {
    bot := lark.NewChatBot(appID, appSecret)
    return &FeishuAdapter{bot: bot}
}

func (a *FeishuAdapter) Run(ctx context.Context) error {
    // 使用 WebSocket 长连接模式（无需公网 IP）
    // 注册 EventTypeMessageReceived 事件处理
    ...
}
```
