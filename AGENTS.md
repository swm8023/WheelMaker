# AI Agent 开发规范

本文档规定 WheelMaker 项目的代码规范，AI 代理在修改此项目时必须遵守。

## 代码风格

- 使用 `gofmt` / `goimports` 格式化，不手动调整空格
- 函数长度不超过 60 行，超出则拆分为更小的函数
- 不使用 `init()`，依赖显式初始化
- 导出类型/函数必须有注释

## 包职责边界（严格遵守）

- `im/` 层只处理消息收发格式，不包含业务逻辑
- `agent/` 层只处理与 CLI 工具的通信，不处理 IM 消息格式
- `hub/` 是唯一可以同时引用 `im` 和 `agent` 的层
- `acp/` 是纯传输层，不了解 Codex 业务

## 错误处理

- 使用 `fmt.Errorf("context: %w", err)` 包装错误
- 使用 `errors.Is` / `errors.As` 判断错误类型，不做字符串匹配
- 不使用 `panic`（除非是明确的程序员错误，如 nil interface 初始化）
- 向上层暴露有意义的错误信息，不要裸露底层错误

## 并发

- 共享状态用 `sync.Mutex` 保护，锁的粒度尽量小
- channel 优先用于单向数据流（如 Update stream）
- 使用 `context.Context` 控制 goroutine 生命周期，不用全局变量

## 禁止事项

- 不硬编码 API Key、路径、端口等配置
- 不在代码中存储凭证（仅通过环境变量或配置文件）
- 不在 `im/` 层直接引用 `agent/` 层
- 不绕过 `Agent` / `Adapter` 接口直接访问具体实现
- 不在 `bin/` 目录提交实际二进制文件（只提交 `.gitkeep`）

## 测试

- 新增功能必须有对应的单元测试
- ACP 传输层测试使用 mock 子进程（fake exe），不依赖真实 codex-acp
- 测试文件命名：`xxx_test.go`，包名用 `xxx_test`（黑盒测试）

## 提交规范

- 提交信息格式：`<type>(<scope>): <description>`
- type: `feat` / `fix` / `refactor` / `docs` / `test` / `chore`
- 示例：`feat(acp): implement JSON-RPC stdio client`
