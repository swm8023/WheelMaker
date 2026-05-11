# Codex App Server Agent 设计

日期：2026-05-11
状态：待评审草案

## 背景

WheelMaker 当前通过 `codex-acp` 与 Codex 通信，`codex-acp` 把 Codex 包装成 ACP agent。这条路径目前不稳定。Codex 现在提供 `codex app-server`，这是 Codex VS Code extension 等富客户端使用的同一套 JSON-RPC 接口。app-server 原生提供 thread history、turn、streamed item events、approvals、model listing、config、filesystem helpers 和 Codex 自有持久化等概念。

WheelMaker 上层已经依赖 `agent.Instance` 暴露的 ACP 语义。因此干净的迁移路径不是替换 WheelMaker session 或 registry 协议，而是新增一个 `codex-app` agent type。这个类型用 app-server 作为后端，但继续向上实现现有 `agent.Instance` 的 ACP-shaped 接口。

主要信息来源：

- Codex app-server docs：https://www.codex-docs.com/automation/app-server/
- OpenAI Codex app-server README：https://github.com/openai/codex/tree/main/codex-rs/app-server
- 本地 schema 来源：`codex-cli 0.129.0`，通过 `codex app-server generate-ts` 生成
- WheelMaker ACP 文档：`docs/acp-protocol-full.zh-CN.md`、`docs/acp-compat.zh-CN.md`
- WheelMaker agent 边界：`server/internal/hub/agent/instance.go`

## 当前 WheelMaker 事实

- `Session` 只依赖 `agent.Instance`，并发送 ACP-shaped 调用：`Initialize`、`SessionLoad`、`SessionPrompt`、`SessionCancel`、`SessionSetConfigOption` 和可选 `SessionList`。
- `Session` 把 `SessionRecord.ID` 持久化为当前协议 session id。当前代码把这个 id 当作 ACP session id 使用。
- `Instance` 通过 `Callbacks.SessionUpdate` 把 agent updates 分发给 `Session`。
- `Session` 把 ACP `session/update` 和 `session/prompt` result 转换成 recorder events 与 IM events。
- 当前 `codex` provider 启动 `npx --yes @zed-industries/codex-acp`。
- Architecture 3.0 文档说一个 business Session 可以持有多个 ACP session id，但当前代码已经把 runtime identity 收敛为一个 `agentType` 和一个 `acpSessionID`。新的 `codex-app` 设计应跟随当前代码，而不是旧文档中的宽泛表述。

## 目标

- 新增可独立选择的 `codex-app` agent type。
- 保持 WheelMaker 上层协议稳定：Registry、IM、recorder、Web/App clients 继续看到 ACP-shaped session events。
- 新 agent 路径完全避开 `codex-acp`。
- 使用 app-server 原生 thread id 作为 WheelMaker 的 `codex-app` session id。
- 以 ACP `configOptions` 形式保留当前 session history、prompt streaming、cancellation、permissions、model selection、approval preset selection 和 reasoning-effort selection。
- 保留 ACP 行为，而不只是保留像 ACP 的 payload：load replay ordering、prompt completion ordering、cancellation semantics、permission resolution 和 tool lifecycle ordering 必须符合 `docs/acp-protocol-full.zh-CN.md`。
- 协议转换必须能在不启动真实 Codex 进程的情况下测试。

## 非目标

- 替换所有 ACP agents。
- 修改 WheelMaker registry session 协议。
- 在本阶段向 WheelMaker clients 暴露完整 app-server API。
- 在 WheelMaker 里实现 browser-style app-server filesystem APIs；Codex 可以通过 app-server 直接操作本地文件。
- 修改当前 recorder schema。

## 交付阶段

### Phase 1：文本聊天与 Config Sync 基线

Phase 1 必须产出一个可用于日常文本会话的 `codex-app` agent。它不是协议 spike，而是必须满足当前 WheelMaker session 所需的 ACP lifecycle 规则。

必需行为：

- `codex-app` 作为可选 agent type 出现，并懒启动 `codex app-server --listen stdio://`。
- `initialize` 映射为 app-server `initialize` + `initialized`，并返回保守的 ACP capabilities。
- `session/new` 启动一个 app-server thread，并返回 `thread.id` 作为 ACP `sessionId`。
- `session/load` resume/read 一个 thread，并在返回前 replay history updates。
- `session/prompt` 支持 ACP `text` 和本地文件 `resource_link` 输入，再把 app-server 输出流式映射回 ACP updates。
- `session/cancel` 中断 active turn，并让 prompt 以 `stopReason=cancelled` 完成。
- `session/list` 映射到 app-server `thread/list`。
- `session/set_config_option` 只支持 `approval_preset`、`model` 和 `reasoning_effort`。
- Config options 在四个边界同步：`session/new` 后、`session/load` 后、`session/set_config_option` 后、app-server/model state 在 session 内变化时。
- app-server command/file approval requests 转换成 ACP `session/request_permission`。
- app-server tool-like items 保持 ACP lifecycle 顺序：`pending -> in_progress -> completed/failed`。
- 非空 `mcpServers` 在 MCP materialization 实现前必须以 `Invalid params` fail fast。
- 除非本阶段实现对应转换，否则 `promptCapabilities.image=false`、`audio=false`、`embeddedContext=false`。

Phase 1 验收：

- 用户可以创建 `/new codex-app`，发送文本 prompts，收到流式 assistant text，看到 plans/tool updates，取消 in-flight turn，恢复 prior thread，并通过现有 config surfaces 修改 model/reasoning/approval preset。
- 当前 WheelMaker clients 只看到 ACP-shaped events，不需要 app-server-specific UI changes。
- 对已声明的 capabilities，不存在已知违反 `docs/acp-protocol-full.zh-CN.md` 的 ACP lifecycle 规则。

### Phase 1.5：URI 图片

只增加不需要内容 materialization 的 image references：

- ACP `image.uri=file:///absolute/path` -> app-server `{type:"localImage", path}`
- ACP `image.uri=http(s)://...` -> app-server `{type:"image", url}`

测试通过后再设置 `promptCapabilities.image=true`。

### Phase 2：Base64 图片与 MCP Materialization

增加更重的输入 materialization：

- ACP `image.data` base64 -> validated temp file -> app-server `{type:"localImage", path}`
- 非空 stdio `mcpServers` -> per-instance Codex config materialization + app-server MCP reload
- 只有实现并声明 `agentCapabilities.mcp.http=true` 后，才支持可选 HTTP MCP。

### 延后

- 完整 app-server-native UI。
- 暴露给 WheelMaker clients 的 dynamic app-server tools。
- Audio input。
- local-file `resource_link` 之外的 embedded resource 支持。

## 推荐方案

把 `codex-app` 实现为一个由 app-server JSON-RPC client 支撑的原生 `agent.Instance`。

Adapter 拥有 app-server 进程，执行 app-server `initialize` + `initialized`，把来自 `Session` 的 ACP-shaped calls 转换为 app-server calls，并把 app-server notifications 或 server-initiated approval requests 转换回 ACP-shaped callbacks。

这样所有改动都保持在 `agent.Instance` 边界之下，也避免让 `Session`、IM routing 或 registry 理解 app-server 概念。

## 备选方案

### A. App-server-native 上层

重写 `Session` 和 registry，直接暴露 app-server Thread/Turn/Item。

优点：

- 没有语义损耗。
- 对 Codex-only UI 长期更贴合。

缺点：

- 打破 multi-agent abstraction。
- 需要修改 Web/App 协议。
- 强迫所有非 Codex agents 进入第二条 adapter 路径。

决策：本阶段拒绝。

### B. 外部 app-server-to-ACP 可执行 shim

构建一个独立 shim executable，通过 stdin/stdout 说 ACP，内部再连接 `codex app-server`。

优点：

- 可复用现有 `ACPProcess` 和 `ownedConn`。
- 可独立测试。

缺点：

- 多一层进程边界。
- 重复 lifecycle、logging 和 configuration 逻辑。
- 更难与 WheelMaker session store 和 permission policy 集成。

决策：延后。如果这个 adapter 未来需要在 WheelMaker 外复用，它仍然是一个可抽取方向。

### C. 原生 `agent.Instance` adapter

在 `server/internal/hub/agent` 内用 Go 实现 shim。

优点：

- 上层 diff 最小。
- 可以直接访问 `Callbacks`、session state 和 logging。
- 围绕 conversion functions 的单测更简单。
- 不依赖 app-server 未定义的 `jsonrpc` wire 字段。

缺点：

- 需要在 ACP transport 旁边新增第二套 JSON-RPC transport。
- 必须把 app-server protocol structs 与 ACP protocol structs 严格隔离。

决策：接受。

## 架构

在 `server/internal/hub/agent` 下新增这些组件：

- `appserver_provider.go`：声明 `codex-app` provider，并启动 `codex app-server --listen stdio://`。
- `appserver_client.go`：基于 JSONL stdio 的 app-server JSON-RPC transport。Outbound 消息省略 `jsonrpc` 字段，Inbound 容忍没有 `jsonrpc`。
- `appserver_instance.go`：实现 `agent.Instance`，把 ACP-shaped calls 转换成 app-server calls。
- `appserver_convert.go`：app-server Thread/Turn/Item/approval events 与 ACP structs 之间的纯转换函数。
- `appserver_types.go`：adapter 使用的最小 app-server request/response/notification structs。只生成或手工复制已使用字段。
- `appserver_instance_test.go` 和 `appserver_convert_test.go`：基于 fake app-server transport 的单元测试。

新增一个 protocol provider 常量：

- `protocol.ACPProviderCodexApp = "codex-app"`

当 PATH 上存在 `codex` 时，在 `newACPFactoryWithDefaults` 注册 `codex-app`。

## 生命周期

### Initialize

`Session.ensureReady` 用 ACP capabilities 调用 `Instance.Initialize`。对 `codex-app`，它必须：

1. 懒启动 `codex app-server --listen stdio://`。
2. 发送 app-server `initialize`：
   - `clientInfo.name = "wheelmaker"`
   - `clientInfo.title = "WheelMaker"`
   - `clientInfo.version = <server version if available>`
   - `capabilities.experimentalApi = false`
3. 发送 app-server `initialized` notification。
4. 返回 ACP `InitializeResult`：
   - `protocolVersion = 1`
   - `agentInfo.name = "codex-app"`
   - `agentInfo.title = "Codex App Server"`
   - `agentCapabilities.loadSession = true`
   - `agentCapabilities.sessionCapabilities.list = {}`
   - Phase 1 `agentCapabilities.promptCapabilities.image = false`
   - Phase 1 `agentCapabilities.promptCapabilities.audio = false`
   - Phase 1 `agentCapabilities.promptCapabilities.embeddedContext = false`

### Session New

`SessionNew` 映射到 app-server `thread/start`。

返回的 app-server `thread.id` 就是 ACP `sessionId`，并存为 WheelMaker session id。

Adapter 返回由 `codex-app` 完全拥有的 synthetic ACP `configOptions`：

- `model`：来自 `model/list`
- `reasoning_effort`：来自所选 model 的 `supportedReasoningEfforts`
- `approval_preset`：WheelMaker 定义的 preset，映射到后续 turn 使用的 app-server `approvalPolicy` 和 sandbox settings

这些都是普通 ACP config options。ACP 允许 agent-defined option ids；`category` 只是 UI hint，不能作为行为依据。为了兼容现有 WheelMaker UI 约定，`reasoning_effort` 使用 category `thought_level`，`approval_preset` 使用自定义 category，例如 `_approval_preset`。

`SessionNew` 不能静默忽略非空 ACP `mcpServers`。当前 WheelMaker 传空列表，但 ACP baseline 要求 stdio MCP 支持。完整 compliance 需要在 `thread/start` 前把非空 MCP server definitions materialize 到 app-server 可见的 Codex config 中；在实现前，非空 `mcpServers` 必须 fail fast 为 `Invalid params`。

### Session Load

`SessionLoad` 映射到 app-server `thread/resume`。

如果 app-server 返回 reconstructed `thread.turns`，adapter 必须在 `SessionLoad` 返回前 replay 到 `Callbacks.SessionUpdate`，满足 ACP `session/load` 的 contract：history 先到，load response 后到。

如果没有返回 turns，adapter 必须 fallback 到 `thread/read includeTurns=true`；本地 `codex-cli 0.129.0` generated schema 中存在这个接口。如果两条路径都不能返回 history，load 仍可成功，但 recorder replay 不完整，必须记录 warning。

### Prompt

`SessionPrompt` 映射到 app-server `turn/start`。

Adapter 必须：

1. 在 app-server user item 到达前或到达时，合成 ACP `user_message_chunk`，保证 recorder/UI 一致。
2. 使用 `threadId = sessionId` 和转换后的 `input` 发送 `turn/start`。
3. 从 response 或 `turn/started` 跟踪每个 thread 的 `activeTurnID`。
4. 把 streaming app-server notifications 转换成 ACP `session/update`。
5. 阻塞直到匹配的 `turn/completed`。
6. 返回 ACP `SessionPromptResult`。

转换 app-server tool-like items 时，adapter 必须保留 ACP tool lifecycle。如果 app-server 第一次报告 item 时它已经 running，必须先发 ACP `tool_call status=pending`，再立即发 `tool_call_update status=in_progress`；不能直接跳到 `in_progress`。

### Cancel

`SessionCancel` 映射到 app-server `turn/interrupt`。

Adapter 需要 active app-server `turnId`。如果没有 active turn，cancel 是 no-op。如果知道 turn id，发送 `turn/interrupt`，参数包含 `threadId` 和 `turnId`。

ACP cancel 在 WheelMaker interface 中是 notification-style call，而 app-server cancel 是 request/response。使用短 background context 并记录 timeout/failure；不能无限阻塞 caller。

普通取消必须让原 ACP prompt 以 `stopReason=cancelled` 完成。它也必须把 pending app-server approvals resolve 为 `cancel`，不能把普通用户取消暴露成 prompt error。

### Config

`SessionSetConfigOption` 是 adapter 本地状态加可选 app-server request：

- `model`：存储，并在后续 `turn/start` 传为 `model`。
- `reasoning_effort`：存储，并在后续 `turn/start` 传为 `effort`。
- `approval_preset`：存储，并映射到后续 `turn/start` 的 approval/sandbox policy。

因为 app-server settings 主要在 `thread/start`、`thread/resume` 和 `turn/start` 提供，adapter 应把 ACP config options 视为 “next turn effective”，并返回完整当前 synthetic list。

Phase 1 config 同步规则：

- 在 `session/new` 和 `session/load` 阶段构建初始 option list。
- 在 adapter state 中保存当前值，并从 `session/set_config_option` 返回完整列表。
- 把保存的值应用到每次后续 `turn/start`。
- 如果 model list 或 effective defaults 变化，发出携带完整 option list 的 `config_option_update`。
- 当 model 变化时，根据 selected model 重新计算 `reasoning_effort` options。如果旧 effort 不被新 model 支持，选择该 model 默认 effort，并在返回的完整列表里包含这个变化。
- 现有 `applyStoredConfigOptions` 会 replay WheelMaker 持久化的 agent preferences；`codex-app` 必须接受这些调用，并在每次 replay 后返回完整列表。

## Config Options

`codex-app` 必须在 `session/new`、`session/load`、`session/set_config_option`，以及列表变化时的 `config_option_update` 中返回这些 ACP-compliant config options：

| ACP config id | Name | Category | Source | App-server effect |
|---|---|---|---|---|
| `approval_preset` | Approval Preset | `_approval_preset` | `codex-app` preset list | 展开为 `approvalPolicy` + `sandbox` |
| `model` | Model | `model` | app-server `model/list` | `turn/start.model` |
| `reasoning_effort` | Reasoning Effort | `thought_level` | selected model 的 effort list | `turn/start.effort` |

`reasoning_effort` 有意使用 app-server 和当前 Codex models 的用户可见术语。ACP docs 把 category 称为 `thought_level`；使用这个 category 可以保持现有 WheelMaker config UX 兼容，同时不把旧术语泄露到 option id。

## Approval Preset 映射

`ask` 表示 “workspace-write 且可请求用户 approval”。Codex 可以在 workspace sandbox 内编辑文件，但 app-server policy 认为需要 approval 的动作会通过 app-server approval request，再通过 WheelMaker 的 ACP `session/request_permission` callback 询问用户。它不是 read-only，也不是 full auto。

推荐初始 presets：

| ACP `approval_preset` | App-server approvalPolicy | App-server sandbox | 含义 |
|---|---|---|---|
| `read_only` | `on-request` | `read-only` | 最安全的分析模式；除非未来增加显式 escalation，否则不允许写文件 |
| `ask` | `on-request` | `workspace-write` | 默认交互模式；workspace writes 可用，需要 approval 的动作询问用户 |
| `auto` | `on-failure` | `workspace-write` | 更低摩擦；先 sandboxed 执行，失败或需要升级时再询问 |
| `full` | `never` | `danger-full-access` | 高信任模式；不询问且无 sandbox 边界 |

Schema note: app-server `thread/start` 和 `thread/resume` 使用 `sandbox` 字段，值是 `SandboxMode` 字符串：`read-only`、`workspace-write`、`danger-full-access`。app-server `turn/start` 使用 `sandboxPolicy` 字段，值是 `SandboxPolicy` 对象：`readOnly`、`workspaceWrite` 或 `dangerFullAccess`。adapter 不能把 thread 级别的 `sandbox` 字段发送给 `turn/start`。

默认应为 `ask`。

不要在 adapter API 中把这个 option 叫做 `mode`。ACP legacy Session Modes 已 deprecated，而 `approval_preset` 更精确：它命名的是该 option 控制的一组 policy bundle。

## 持久化

对 `codex-app`，WheelMaker session id 就是 app-server thread id。这样符合当前 `SessionRecord.ID` 行为，并避免新增第二张 id mapping 表。

Adapter 不应持久化额外 app-server session id。`agent_json` 中的 adapter-local state 可以包含：

- 当前 synthetic config options
- selected model
- selected reasoning effort
- selected approval preset
- last known thread title

## 错误处理

- app-server JSON-RPC `-32001` overload 对幂等请求可用 jittered exponential backoff 重试，例如 `initialize`、`model/list`、`thread/list`、`thread/read`。不要在 `turn/start` 可能已被接受后自动重试。
- 进程退出应通过当前 `Session.resetDeadConnection` 路径暴露。
- 未知 notifications 以 debug 级别记录并忽略。
- 未知 server requests 应返回 JSON-RPC `-32601 Method not found`。已知但无法映射的 approval request 类型应保守返回 `cancel` 或 `decline`。
- Permission request conversion 必须 fail closed：如果 ACP permission callback 失败，响应 `cancel` 或 `decline`。

## 测试

单元测试：

- ACP `Initialize` 发送 app-server `initialize`，再发送 `initialized`。
- `SessionNew` 返回 app-server `thread.id` 作为 ACP `sessionId`。
- `SessionLoad` 在返回前 replay app-server turn history。
- `SessionPrompt` 把用户 text 转成 `turn/start`，把 streaming deltas 转成 ACP chunks，并在 completed turn 后返回 `end_turn`。
- `SessionPrompt` cancellation 返回 `cancelled`，并在完成前 drain updates。
- Tool item conversion 发出 `pending -> in_progress -> completed/failed`，不能跳过状态。
- `SessionCancel` 带 active turn id 发送 `turn/interrupt`。
- Approval requests 把 ACP allow/reject/cancel outcomes 映射为 app-server decisions。
- Turn 被取消时，pending approval requests 被取消。
- Model list 映射成 ACP `model` 和 `reasoning_effort` config options。
- Approval presets 映射成 app-server `approvalPolicy` 和 `sandbox`。
- 非空 `mcpServers` 要么被 materialize 到 app-server-visible config，要么在 session creation 前被拒绝；绝不能静默忽略。
- 未知 app-server events 不 panic，也不阻塞 pending requests。

集成测试：

- 只在 integration flag 下启动 `codex app-server --listen stdio://`。
- 在临时 project 中启动 thread，发送 prompt，取消 prompt，并恢复 thread。

## Rollout

1. 把 `codex-app` 作为 opt-in provider 添加，同时保持 `codex` 仍映射到 `codex-acp`。
2. 在 `project.agents` 中同时暴露 `codex` 和 `codex-app`。
3. 用 `/new codex-app` dogfood。
4. 稳定后，把 project default preference 从 `codex` 改为 `codex-app`。
5. 在 `codex-app` 通过 recovery、permission、long-running prompt 测试前，保留 `codex` 作为 fallback。

## 待评审问题

1. `codex-app` 默认应该使用 `ask` approval preset，还是继承用户 Codex `config.toml` 的默认值？
   推荐答案：如果 app-server resolved config 能表示为某个已知 preset，则使用它；否则用安全的交互默认值 `ask`。
2. `codex-app` 应使用 `stdio://`，还是通过 `codex app-server proxy` 使用 app-server control socket？
   推荐答案：Phase 1 使用 `stdio://`，因为它符合当前 process ownership。只有需要 multi-client control 时再评估 unix/control socket。
3. WheelMaker 是否应该为每个 app-server setting 都合成 `configOptions`？
   推荐答案：不应该。初始只暴露 `approval_preset`、`model` 和 `reasoning_effort`。
