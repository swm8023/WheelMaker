# Codex App Server Phase 1 实现计划

> **给 agentic workers：** 必须使用子技能：`superpowers:subagent-driven-development`（推荐）或 `superpowers:executing-plans`，按任务逐项执行本计划。步骤使用 checkbox（`- [ ]`）语法跟踪。

**目标：** 新增可选择的 `codex-app` agent，懒启动 `codex app-server --listen stdio://`，并提供符合 ACP 行为的文本聊天和 config option 同步。

**架构：** 在 `agent.Instance` 之下实现原生 Go adapter。WheelMaker 的 `Session`、IM、registry 和 recorder 继续保持 ACP-shaped；adapter 负责把 ACP 调用转换成 app-server JSON-RPC，并把 app-server notification/server request 转回 ACP update/callback。

**技术栈：** Go、JSONL stdio、不带 `jsonrpc` 字段的 Codex app-server JSON-RPC、现有 `server/internal/hub/agent` 与 `server/internal/protocol` 包、基于 fake transport 的单元测试。

---

## Phase 1 已锁定决策

- Agent 名称与 provider id：`codex-app`。
- Session 身份：`ACP sessionId == app-server threadId == WheelMaker SessionRecord.ID`。
- 进程启动：`codex app-server --listen stdio://`，由 `Session.ensureInstance` 懒启动。
- 不复用 `ownedConn`：app-server wire 不带 `jsonrpc`，而且有 app-server 专属 server request。
- `codex-app` 暴露的 config options：`approval_preset`、`model`、`reasoning_effort`。
- 默认 `approval_preset`：`ask`，除非 app-server 返回的有效配置能精确映射到某个已知 preset。
- `ask` 含义：app-server `approvalPolicy=on-request`，thread `sandbox=workspace-write`，turn `sandboxPolicy.type=workspaceWrite`。
- Phase 1 声明 `promptCapabilities.image=false`、`audio=false`、`embeddedContext=false`。
- Phase 1 对非空 `mcpServers` 在 `thread/start` 或 `thread/resume` 前明确报错，不能静默忽略。
- Schema 修正：`thread/start` 和 `thread/resume` 接收 `sandbox`，值是 `SandboxMode` 字符串；`turn/start` 接收 `sandboxPolicy`，值是 `SandboxPolicy` 对象。不要把 `sandbox` 发送给 `turn/start`。

## 文件分工

- 修改 `server/internal/protocol/acp_const.go`：provider 常量、解析逻辑、provider 名称列表、config id/category 常量。
- 修改 `server/internal/hub/agent/factory.go`：用原生 `InstanceCreator` 注册 `codex-app`。
- 新建 `server/internal/hub/agent/appserver_provider.go`：解析 `codex`，启动 `app-server --listen stdio://`。
- 新建 `server/internal/hub/agent/appserver_client.go`：JSON-RPC request matching、notification、server request、close/liveness。
- 新建 `server/internal/hub/agent/appserver_types.go`：Phase 1 使用的最小 app-server struct。
- 新建 `server/internal/hub/agent/appserver_config.go`：config option 状态与 approval/sandbox 映射。
- 新建 `server/internal/hub/agent/appserver_convert.go`：纯 ACP/app-server 转换函数。
- 新建 `server/internal/hub/agent/appserver_instance.go`：原生 `agent.Instance` 实现。
- 修改 `server/internal/hub/client/session.go`：推荐让状态展示包含 `approval_preset`。
- 修改 `server/internal/hub/client/commands.go`：推荐让 config update 展示包含 `approval_preset`。
- 测试：扩展 `server/internal/hub/agent/agent_test.go`；新增 `appserver_client_test.go`、`appserver_config_test.go`、`appserver_convert_test.go`、`appserver_instance_test.go`；扩展 `server/internal/hub/client/client_test.go` 做显示兼容测试。

## Task 1：Provider 常量与懒加载 Factory 注册

**文件：** `server/internal/protocol/acp_const.go`、`server/internal/hub/agent/factory.go`、`server/internal/hub/agent/appserver_provider.go`、`server/internal/hub/agent/agent_test.go`

- [ ] 在 `agent_test.go` 添加失败测试：

```go
func TestParseACPProviderCodexApp(t *testing.T) {
	provider, ok := protocol.ParseACPProvider("codex-app")
	if !ok || provider != protocol.ACPProviderCodexApp {
		t.Fatalf("provider=%q ok=%v", provider, ok)
	}
}

func TestCodexAppProviderLaunchUsesAppServerStdio(t *testing.T) {
	p := NewCodexAppServerProvider()
	p.lookPath = func(file string) (string, error) {
		if file != "codex" { t.Fatalf("file=%q", file) }
		return "C:\\bin\\codex.exe", nil
	}
	exe, args, env, err := p.Launch()
	if err != nil { t.Fatalf("Launch: %v", err) }
	if exe != "C:\\bin\\codex.exe" { t.Fatalf("exe=%q", exe) }
	want := []string{"app-server", "--listen", "stdio://"}
	if !reflect.DeepEqual(args, want) { t.Fatalf("args=%#v want=%#v", args, want) }
	if len(env) != 0 { t.Fatalf("env=%#v", env) }
}
```

- [ ] 运行失败测试：`cd server; go test ./internal/hub/agent -run "TestParseACPProviderCodexApp|TestCodexAppProviderLaunchUsesAppServerStdio" -count=1`。
- [ ] 添加 `protocol.ACPProviderCodexApp = "codex-app"`，加入 `acpProviders`，并添加 `ParseACPProvider` 分支。
- [ ] 添加 config 常量：`ConfigOptionIDApprovalPreset`、`ConfigOptionIDReasoningEffort`、`ConfigOptionCategoryApprovalPreset`。
- [ ] 创建 `NewCodexAppServerProvider()`，其 `Launch()` 返回 `codex app-server --listen stdio://`。
- [ ] 在 `newACPFactoryWithDefaults` 注册 native creator；`codex-app` 不允许走 `providerInstanceCreator`。
- [ ] 再次运行 focused test。预期：Task 6 添加 instance constructor 后通过。
- [ ] 提交：`git add server/internal/protocol/acp_const.go server/internal/hub/agent/factory.go server/internal/hub/agent/appserver_provider.go server/internal/hub/agent/agent_test.go && git commit -m "feat: register codex app-server provider"`。

## Task 2：App-Server JSON-RPC Client

**文件：** `server/internal/hub/agent/appserver_client.go`、`server/internal/hub/agent/appserver_types.go`、`server/internal/hub/agent/appserver_client_test.go`

- [ ] 添加 fake transport 测试，证明 outbound request 不包含 `jsonrpc`、response 按 `id` 匹配、app-server server request 能收到 result/error、`Close` 会让 pending request 失败。
- [ ] 运行失败测试：`cd server; go test ./internal/hub/agent -run TestAppServerClient -count=1`。
- [ ] 实现 `appServerRequestID` 为 raw JSON，确保 string 和 number id 都能 round-trip。
- [ ] 实现 `appServerClient.Request(ctx, method, params, out)`、`Notify(method, params)`、`OnRequest`、`OnNotification`、`Alive`、`Close`。
- [ ] 实现 request 分类：

```go
switch {
case msg.ID != nil && msg.Method != "":
	go c.handleIncomingRequest(*msg.ID, msg.Method, msg.Params)
case msg.ID != nil:
	c.resolvePending(*msg.ID, msg.Result, msg.Error)
case msg.Method != "":
	c.notifyH(c.ctx, msg.Method, msg.Params)
}
```

- [ ] 默认 server request 行为必须返回 app-server error `{code:-32601,message:"method not found: <method>"}`。
- [ ] 运行：`cd server; go test ./internal/hub/agent -run TestAppServerClient -count=1`。预期：PASS。
- [ ] 提交：`git add server/internal/hub/agent/appserver_client.go server/internal/hub/agent/appserver_types.go server/internal/hub/agent/appserver_client_test.go && git commit -m "feat: add codex app-server jsonrpc client"`。

## Task 3：最小 App-Server 类型

**文件：** `server/internal/hub/agent/appserver_types.go`、`server/internal/hub/agent/appserver_convert_test.go`

- [ ] 添加 generated schema shape 的 decode 测试：`ThreadStartResponse`、`ThreadResumeResponse`、`TurnStartResponse`、`ModelListResponse`、`TurnCompletedNotification`、`ItemStartedNotification`、`CommandExecutionRequestApprovalParams`、`FileChangeRequestApprovalParams`。
- [ ] 运行失败测试：`cd server; go test ./internal/hub/agent -run TestAppServerPhase1TypesDecodeGeneratedShapes -count=1`。
- [ ] 添加 `thread/start`、`thread/resume`、`thread/read`、`thread/list`、`turn/start`、`turn/interrupt`、`model/list`、thread、turn、thread item、notification 和 approval params 的 struct。
- [ ] 不稳定或 Phase 1 不使用的字段用 `json.RawMessage` 或 `any`；不要建模未使用的 app-server API。
- [ ] 关键字段名：

```go
type appServerThreadStartParams struct {
	Model *string `json:"model,omitempty"`
	CWD string `json:"cwd,omitempty"`
	ApprovalPolicy string `json:"approvalPolicy,omitempty"`
	ApprovalsReviewer string `json:"approvalsReviewer,omitempty"`
	Sandbox string `json:"sandbox,omitempty"`
	ServiceName string `json:"serviceName,omitempty"`
}

type appServerTurnStartParams struct {
	ThreadID string `json:"threadId"`
	Input []appServerUserInput `json:"input"`
	CWD string `json:"cwd,omitempty"`
	ApprovalPolicy string `json:"approvalPolicy,omitempty"`
	SandboxPolicy any `json:"sandboxPolicy,omitempty"`
	Model *string `json:"model,omitempty"`
	Effort *string `json:"effort,omitempty"`
}
```

- [ ] 运行：`cd server; go test ./internal/hub/agent -run TestAppServerPhase1TypesDecodeGeneratedShapes -count=1`。预期：PASS。
- [ ] 提交：`git add server/internal/hub/agent/appserver_types.go server/internal/hub/agent/appserver_convert_test.go && git commit -m "feat: add codex app-server phase one types"`。

## Task 4：Config Options 与 Approval Presets

**文件：** `server/internal/hub/agent/appserver_config.go`、`server/internal/hub/agent/appserver_config_test.go`

- [ ] 添加测试：默认 options、从 model 派生 reasoning options、model 切换时重置不支持的 effort、非法 config id/value 拒绝、approval preset 映射。
- [ ] 运行失败测试：`cd server; go test ./internal/hub/agent -run "TestCodexAppConfig|TestApprovalPreset" -count=1`。
- [ ] 实现 `codexAppConfigState`，默认值：`approvalPreset="ask"`，model 来自第一个可见 app-server model，reasoning effort 来自所选 model 的默认值。
- [ ] 返回且只返回三个 ACP options，顺序固定：`approval_preset`、`model`、`reasoning_effort`。
- [ ] `approval_preset` 使用 category `_approval_preset`；`reasoning_effort` 使用 category `thought_level`。
- [ ] 实现 approval profile 映射：

```text
read_only -> approvalPolicy=on-request, thread sandbox=read-only, turn sandboxPolicy.type=readOnly
ask      -> approvalPolicy=on-request, thread sandbox=workspace-write, turn sandboxPolicy.type=workspaceWrite
auto     -> approvalPolicy=on-failure, thread sandbox=workspace-write, turn sandboxPolicy.type=workspaceWrite
full     -> approvalPolicy=never, thread sandbox=danger-full-access, turn sandboxPolicy.type=dangerFullAccess
```

- [ ] 运行：`cd server; go test ./internal/hub/agent -run "TestCodexAppConfig|TestApprovalPreset" -count=1`。预期：PASS。
- [ ] 提交：`git add server/internal/hub/agent/appserver_config.go server/internal/hub/agent/appserver_config_test.go && git commit -m "feat: add codex app config mapping"`。

## Task 5：纯协议转换

**文件：** `server/internal/hub/agent/appserver_convert.go`、`server/internal/hub/agent/appserver_convert_test.go`

- [ ] 添加测试：ACP text input、本地文件 `resource_link`、不支持的 image/audio/resource 拒绝、assistant delta 转换、reasoning delta 转换、plan full replacement、tool lifecycle synthesis、stop reason 映射。
- [ ] 运行失败测试：`cd server; go test ./internal/hub/agent -run "TestConvertACP|TestRejectUnsupported|TestToolLifecycle|TestTurnStatus|TestPlan" -count=1`。
- [ ] 实现 `acpPromptToAppServerInput([]protocol.ContentBlock)`。
- [ ] Text 映射为 `{type:"text", text, text_elements:[]}`。
- [ ] `resource_link` 只接受绝对本地路径或 `file://` URI，映射为 `{type:"mention", name, path}`。
- [ ] Phase 1 对 `image`、`audio`、embedded `resource` 返回 invalid-params 风格错误。
- [ ] 实现 `appServerItemStartedToACPUpdates`；如果 app-server 第一次上报时 tool 已经 running，合成 `tool_call status=pending`，再发 `tool_call_update status=in_progress`。
- [ ] 实现 stored `Turn.items` 到 ACP updates 的 replay 转换，并保证顺序稳定。
- [ ] 实现 `turnStatusToStopReason`：`completed -> end_turn`，`interrupted -> cancelled`，`failed -> refusal`。
- [ ] 运行 conversion tests。预期：PASS。
- [ ] 提交：`git add server/internal/hub/agent/appserver_convert.go server/internal/hub/agent/appserver_convert_test.go && git commit -m "feat: map codex app-server events to acp"`。

## Task 6：原生 Instance 生命周期

**文件：** `server/internal/hub/agent/appserver_instance.go`、`server/internal/hub/agent/appserver_instance_test.go`、`server/internal/hub/agent/factory.go`

- [ ] 添加 fake RPC 测试：`Initialize`、`SessionNew`、`SessionList`、`SessionSetConfigOption`、非空 `mcpServers` 拒绝。
- [ ] 运行失败测试：`cd server; go test ./internal/hub/agent -run "TestCodexAppInitialize|TestCodexAppSessionNew|TestCodexAppRejects|TestCodexAppSessionList|TestCodexAppSetConfig" -count=1`。
- [ ] 实现 `NewCodexAppServerInstance(provider, cwd)`：用 app-server provider 启动 `ACPProcess`，再用 `newAppServerClient` 包装。
- [ ] 实现测试专用 constructor：`newCodexAppServerInstanceWithRPC(rpc, cwd)`。
- [ ] `Initialize` 必须发送 app-server `initialize`，再发送 `initialized`，然后返回 ACP `InitializeResult`，其中 `loadSession=true`、有 `sessionCapabilities.list`、prompt image/audio/embedded 均为 false。
- [ ] `SessionNew` 必要时调用 `model/list`，拒绝非空 MCP，调用 `thread/start`，设置 `activeThreadID`，返回 thread id 和完整 config options。
- [ ] `SessionList` 把 app-server threads 映射成 ACP `SessionInfo`，`updatedAt` 使用 RFC3339。
- [ ] `SessionSetConfigOption` 修改 adapter config state，并返回完整 option list。
- [ ] Phase 1 的 `ListSkills` 可以委托给 `listSkillsForPreset(ctx, CodexACPProviderPreset, cwd)`，保证当前 Codex skill 目录仍可见。
- [ ] 运行 lifecycle tests。预期：PASS。
- [ ] 提交：`git add server/internal/hub/agent/appserver_instance.go server/internal/hub/agent/appserver_instance_test.go server/internal/hub/agent/factory.go && git commit -m "feat: add codex app-server instance lifecycle"`。

## Task 7：Session Load Replay

**文件：** `server/internal/hub/agent/appserver_instance.go`、`server/internal/hub/agent/appserver_convert.go`、`server/internal/hub/agent/appserver_instance_test.go`

- [ ] 添加测试，证明 `SessionLoad` 在返回 `SessionLoadResult` 前已经通过 callback 发出 replay `SessionUpdate`。
- [ ] 用一个 stored turn 测 replay 顺序：user message、assistant message、plan、command item。
- [ ] 运行失败测试：`cd server; go test ./internal/hub/agent -run TestCodexAppSessionLoadReplaysHistoryBeforeReturn -count=1`。
- [ ] 实现 `SessionLoad` 为 `thread/resume`；如果返回的 thread 没有 turns，则调用 `thread/read {includeTurns:true}`。
- [ ] 当 app-server 返回有效 `model` 和 `reasoningEffort` 时，应用到本地 config state。
- [ ] 在返回前通过 `callbacks.SessionUpdate` replay 历史 updates。
- [ ] `SessionLoadResult` 在 replay 后返回完整 config options。
- [ ] 运行 load tests。预期：PASS。
- [ ] 提交：`git add server/internal/hub/agent/appserver_instance.go server/internal/hub/agent/appserver_convert.go server/internal/hub/agent/appserver_instance_test.go && git commit -m "feat: replay codex app-server thread history"`。

## Task 8：Prompt Streaming 与 Cancel

**文件：** `server/internal/hub/agent/appserver_instance.go`、`server/internal/hub/agent/appserver_types.go`、`server/internal/hub/agent/appserver_convert.go`、`server/internal/hub/agent/appserver_instance_test.go`

- [ ] 添加 fake RPC 测试：text prompt、assistant delta、plan update、tool start/completion、completed turn、interrupted turn、不支持的 prompt content、cancel 发送 `turn/interrupt`。
- [ ] 运行失败测试：`cd server; go test ./internal/hub/agent -run "TestCodexAppSessionPrompt|TestCodexAppSessionCancel" -count=1`。
- [ ] `SessionPrompt` 转换 prompt blocks，发出 synthetic ACP `user_message_chunk`，发送 `turn/start`，跟踪 `activeTurnID`，等待匹配的 `turn/completed`，并返回 ACP `SessionPromptResult`。
- [ ] `turn/start` params 必须包含选中的 `model`、`effort`、`approvalPolicy`，以及由当前 approval preset 派生的 `sandboxPolicy`。
- [ ] Notification handlers 必须覆盖：`turn/started`、`turn/completed`、`turn/plan/updated`、`item/started`、`item/completed`、`item/agentMessage/delta`、`item/reasoning/textDelta`、`item/reasoning/summaryTextDelta`、`item/commandExecution/outputDelta`、`item/fileChange/patchUpdated`。
- [ ] 未知 notification 以 debug 级别记录并忽略。
- [ ] `SessionCancel` 用短 timeout 发送 `turn/interrupt`，包含当前 `threadId` 和 `turnId`；没有 active turn 时 no-op。
- [ ] 如果 prompt context 被取消，调用 `SessionCancel`，并对普通取消返回 `stopReason=cancelled`。
- [ ] 运行 prompt/cancel tests。预期：PASS。
- [ ] 提交：`git add server/internal/hub/agent/appserver_instance.go server/internal/hub/agent/appserver_types.go server/internal/hub/agent/appserver_convert.go server/internal/hub/agent/appserver_instance_test.go && git commit -m "feat: stream codex app-server turns through acp"`。

## Task 9：Permission Requests

**文件：** `server/internal/hub/agent/appserver_instance.go`、`server/internal/hub/agent/appserver_types.go`、`server/internal/hub/agent/appserver_convert.go`、`server/internal/hub/agent/appserver_instance_test.go`

- [ ] 添加测试：`item/commandExecution/requestApproval`、`item/fileChange/requestApproval`、unsupported server request 返回 `-32601`、callback error fail closed、cancel 会把 active approval resolve 为 cancel。
- [ ] 运行失败测试：`cd server; go test ./internal/hub/agent -run "TestCodexApp.*Approval|TestCodexAppUnsupportedServerRequest" -count=1`。
- [ ] 把 command approval 转成 ACP `session/request_permission`，tool kind 为 `execute`，options 为 `allow_once`、`allow_always`、`reject`。
- [ ] 把 file approval 转成 ACP `session/request_permission`，tool kind 为 `write`，options 同上。
- [ ] 保守映射 ACP result：`allow_once -> approve/allow_once`（按 generated schema 确认）、`allow_always -> approve_for_session/allow_always`、`reject -> reject/deny`、cancelled/error -> cancel。
- [ ] 最终确定 decision 字符串前，检查当前安装版本生成的 `CommandExecutionApprovalDecision.ts` 与 `FileChangeApprovalDecision.ts`。
- [ ] 不支持的 request，例如 `item/tool/requestUserInput`，返回 app-server error `-32601`，不能阻塞。
- [ ] `mcpServer/elicitation/request` 返回 cancelled response。
- [ ] 运行 approval tests。预期：PASS。
- [ ] 提交：`git add server/internal/hub/agent/appserver_instance.go server/internal/hub/agent/appserver_types.go server/internal/hub/agent/appserver_convert.go server/internal/hub/agent/appserver_instance_test.go && git commit -m "feat: bridge codex app-server approvals to acp"`。

## Task 10：Config Display 兼容

**文件：** `server/internal/hub/client/session.go`、`server/internal/hub/client/commands.go`、`server/internal/hub/client/client_test.go`

- [ ] 添加测试：当存在 `approval_preset` 时，`sessionInfoLine` 显示 `approval: ask`。
- [ ] 添加测试：当 update 携带 `approval_preset` 时，`formatConfigOptionUpdateMessage` 包含 `approval=<value>`。
- [ ] 运行失败测试：`cd server; go test ./internal/hub/client -run "TestSessionInfoLineShowsApprovalPreset|TestFormatConfigOptionUpdateShowsApproval" -count=1`。
- [ ] 更新 status rendering，包含 approval preset，同时保留现有 agents 的 mode/model 输出。
- [ ] 更新 config update formatting，在存在时显示 approval、mode、model。
- [ ] 不要复用 `/mode`；用户通过 `/config approval_preset ask|auto|full|read_only` 和 help menu config options 设置 approval preset。
- [ ] 运行 client tests。预期：PASS。
- [ ] 提交：`git add server/internal/hub/client/session.go server/internal/hub/client/commands.go server/internal/hub/client/client_test.go && git commit -m "feat: show codex app approval preset config"`。

## Task 11：Docs、验证与 Gate

**文件：** `docs/superpowers/specs/2026-05-11-codex-app-server-agent-design.md`、`docs/codex-app-server-acp-bridge.zh-CN.md`、`docs/superpowers/plans/2026-05-11-codex-app-server-phase1.md`

- [ ] 把 schema correction 写入 bridge/spec 两份文档：`thread/start` 和 `thread/resume` 使用 `sandbox`；`turn/start` 使用 `sandboxPolicy`。
- [ ] 运行 focused packages：`cd server; go test ./internal/protocol ./internal/hub/agent ./internal/hub/client -count=1`。预期：PASS。
- [ ] 运行完整 server tests：`cd server; go test ./...`。预期：PASS。
- [ ] 构建 server：`cd server; go build -o bin/windows_amd64/wheelmaker.exe ./cmd/wheelmaker/`。预期：PASS。
- [ ] 提交 docs/verification fixes：`git add docs server && git commit -m "docs: finalize codex app-server phase one plan"`。
- [ ] 实现完成后执行仓库 completion gate：`git add -A`，如有未提交实现变更则 `git commit -m "feat: add codex app-server phase one bridge"`，`git push origin <current-branch>`，然后因为 server 文件变化运行 `powershell -NoProfile -ExecutionPolicy Bypass -File scripts/signal_update_now.ps1 -DelaySeconds 30`。

## 验收清单

- [ ] `/new codex-app` 创建新的 app-server thread，并把 thread id 存为 session id。
- [ ] 文本 prompt 能流式输出 ACP `user_message_chunk`、`agent_message_chunk`、`plan` 和 tool updates。
- [ ] `session/load` 在返回前 replay 历史 updates。
- [ ] `session/list` 展示 app-server threads。
- [ ] `/cancel` 中断 active app-server turn，普通取消的最终 prompt result 是 `cancelled`。
- [ ] `approval_preset`、`model`、`reasoning_effort` 在 new/load/set/update 路径都以完整 option list 返回。
- [ ] Stored config preferences 通过现有 `applyStoredConfigOptions` 按 exact id replay。
- [ ] Tool lifecycle 不会跳过 ACP `pending` 直接进入 `in_progress`。
- [ ] Command/file approvals 通过 ACP permission callback，并且 fail closed。
- [ ] Phase 1 不会静默接受 images、audio、embedded resources 或非空 MCP servers。
