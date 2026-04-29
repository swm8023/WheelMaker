# session.go 职责梳理与冗余审计

本文梳理 `server/internal/hub/client/session.go` 当前全部函数职责、关键流程，以及可见冗余点。

## 1. 文件职责总览

`Session` 是单会话运行时核心，负责：

- ACP 连接生命周期：实例创建、初始化、恢复加载、重连。
- Prompt 生命周期：发起 prompt、流式转发、取消、异常恢复。
- 会话状态落盘：session record、agent preference。
- IM 与 SessionView 侧输出：系统消息、流式事件、超时告警。

## 2. 全函数清单与作用

### 2.1 构造与状态辅助

- `newSession(id, cwd)`：创建 Session，校验 session id 非空并初始化锁/超时限制器。
- `cloneAgentInfo(src)`：浅拷贝 `AgentInfo` 指针对象。
- `cloneSessionAgentState(src)`：深拷贝 `SessionAgentState`（切片与指针字段）。
- `agentStateLocked(name)`：在持锁上下文获取当前 agent state，并在 `agentType` 为空时补齐。
- `currentAgentNameLocked()`：优先实例名，其次 `agentType`。
- `shortSessionID(id)`：用于展示的短 session id。
- `sessionInfoLine()`：输出会话摘要（session/agent/mode/model）。
- `renderUnknown(v)`：空字符串渲染为 `unknown`。

### 2.2 消息与上下文

- `reply(text)`：系统消息快捷入口。
- `replyWithTitle(title, body)`：发送系统消息到 IM 或 stdout，并写入 SessionView。
- `setIMSource(source)`：设置会话 IM 源。
- `imContext()`：读取 IM router/source 上下文。
- `recordSessionViewEvent(event)`：补齐默认字段并写入 view sink。

### 2.3 ACP 连接与就绪

- `ensureInstance(ctx)`：懒创建 agent instance，处理 provider fallback，绑定 callbacks。
- `emptyMCPServers()`：`session/new|load` 的 MCP server 占位。
- `ensureReady(ctx)`：执行 `initialize + session/load + 配置回放 + 状态写回`。
- `ensureReadyAndNotify(ctx)`：`ensureReady` 成功后仅在“从未就绪 -> 就绪”时发 ready 提示并持久化。

### 2.4 配置回放与合并

- `findConfigOptionID(options, id, category)`：按 id/category 查配置项 id。
- `mergeConfigOptions(current, updated)`：按 id/category 合并配置列表。
- `replayableTargetsFromSnapshot(snap)`：将 snapshot 展开为可回放目标（mode/model/thought_level）。
- `currentReplayableValue(label, snap)`：从 snapshot 读取单字段值。
- `applyReplayableConfigBaseline(ctx, projectName, inst, sessionID, current, desired)`：调用 `SessionSetConfigOption` 回放缺失配置。

### 2.5 Prompt 流与取消

- `promptStream(ctx, blocks)`：启动一次 prompt，转成内部更新事件流并桥接最终结果。
- `cancelPrompt()`：取消当前 prompt context，并在可取消条件下调用 `SessionCancel`。
- `normalizeSessionPromptBlocks(blocks)`：清洗输入 block（类型和文本）。
- `handlePrompt(text)`：文本 prompt 快捷入口。
- `handlePromptBlocks(blocks)`：完整 prompt 主流程（连接、重试、流转发、落盘、恢复）。
- `extractTextChunk(raw)`：从 update content 提取文本。
- `extractTextFromAny(v)`：递归提取嵌套文本片段。

### 2.6 持久化与恢复

- `toRecord()`：将运行态 Session 转 SessionRecord。
- `sessionFromRecord(rec, cwd)`：从 SessionRecord 恢复 Session。
- `persistSession(ctx)`：持久化当前 session。
- `persistSessionBestEffort()`：后台容错持久化。
- `loadAgentPreferenceState(agentName)`：读取项目级 agent 偏好。
- `persistAgentPreferenceState(agentName, configOptions, commands)`：保存项目级 agent 偏好。
- `Suspend(ctx)`：取消进行中的 prompt，关闭实例，改为 suspended 并落盘。

### 2.7 ACP 回调处理

- `SessionUpdate(params)`：处理 ACP session/update；更新本地状态、持久化、并转发给 prompt 通道。
- `SessionRequestPermission(ctx, requestID, params)`：权限请求策略（优先持久允许，其次一次性允许）。

### 2.8 观测、可用性与重连

- `reportTimeoutError(stage, kind)`：超时告警日志 + IM 通知（受限流控制）。
- `connectHint()`：给用户返回“如何连接 agent”的提示。
- `preferredAgentName()`：读取首选 agent 名（instance -> agentType -> registry 默认）。
- `agentProcessAlive()`：探测进程是否存活（若实例支持 `Alive()`）。
- `shouldReconnectOnRecoverableErr(err)`：可恢复错误且进程不活时触发重连。
- `isAgentExitError(err)`：识别进程退出相关错误。
- `isAgentRecoverableRuntimeErr(err)`：可恢复运行时错误集合（退出或 sandbox refresh）。
- `isUnsupportedReasoningEffortError(err)`：识别 reasoning_effort 不兼容错误。
- `pickAlternativeModelValue(opt)`：从模型选项中选一个不同于当前值的候选。
- `tryCopilotReasoningFallback(ctx)`：Copilot 场景下自动切模型并持久化。
- `resetDeadConnection(err)`：在进程退出错误时重置实例/提示流状态。
- `forceReconnect()`：强制重置实例/提示流状态。
- `hasSandboxRefreshUpdate(update)`：检测 sandbox refresh 特征更新。
- `isSandboxRefreshErr(err)`：检测 sandbox refresh 特征错误。

## 3. 关键流程

### 3.1 新会话创建到可用

入口：`Client.CreateSession`（位于 `client.go`）

1. `createSessionState`：创建 instance -> initialize -> session/new -> 配置基线回放。
2. 以返回的 `sessionID` 构造 `Session`。
3. 直接注入 instance 与 agentState，标记 `ready=true`。
4. 持久化 session 与项目 agent 偏好。

结论：新会话路径不再经过 `session.ensureReady()` 分配 session id，符合“先有 ACP sid 再有 Session”的不变式。

### 3.2 已有会话恢复到可用

入口：`Client.SessionByID` / 运行时已有 `Session` 但实例丢失

1. `sessionFromRecord` 恢复结构体与 `acpSessionID`。
2. 需要时 `ensureInstance` 建立实例。
3. `ensureReady` 执行 `initialize + session/load`。
4. 回放配置、更新命令与能力，再设 `ready=true`。

结论：恢复路径严格依赖 `LoadSession` 能力；无该能力会失败而非悄悄建新 sid。

### 3.3 Prompt 执行与流转发

入口：`handlePrompt` / `handlePromptBlocks`

1. 标准化输入 blocks。
2. `ensureInstance` + `ensureReadyAndNotify`。
3. `promptStream` 启动 `SessionPrompt`，并接收 `SessionUpdate` 流。
4. 同时写 SessionView、可选转发 IM、本地拼接文本输出。
5. 遇到可恢复错误时按规则重连重试（最多一次）。

## 4. 冗余判断

以下为“当前代码中可见的冗余/重复”。分为两类：

### 4.1 可立即精简（低风险）

1. 注释与实现不一致：
- `cancelPrompt` 注释写了“emits tool_call_cancelled updates”，但当前实现已不做该补发。
- 建议仅更新注释，避免误导。

2. 重复状态清理逻辑：
- `resetDeadConnection` 与 `forceReconnect` 含高度重复字段复位。
- 可提取私有 `resetConnectionLocked/clearPromptStateLocked`，减少重复和漏改风险。

3. `ensureReady` 注释历史残留：
- 已改为“只做 existing sid 的 load”，但旧分步语义容易被误读成含 `session/new`。
- 当前已有简化注释，可继续补充“不会分配新 sid”。

### 4.2 建议谨慎处理（有行为/并发风险）

1. `SessionUpdate` 内状态读写段可继续收敛，但要保留跨锁快照：
- 当前 `changed` 后再 clone 一次 state 再持久化，是为了脱离锁后安全使用切片。
- 直接删 clone 可能引入并发数据竞争。

2. `ensureReady` 内 snapshot + merge + baseline 回放链条看起来长，但承担了“load 结果不完整”修复：
- 若直接压缩逻辑，容易破坏 mode/model/thought_level 的补齐行为。

3. `promptStream` 中 update/result 双通道收敛逻辑较复杂：
- 代码长度高，但它保证了 result 到达后还能 drain 截获通道，避免漏最后几条 update。

## 5. 建议的下一步精简顺序

1. 先做“纯重复抽取”：合并 `resetDeadConnection`/`forceReconnect` 的公共复位段。
2. 再做“注释和命名对齐”：修正 `cancelPrompt` 等历史注释。
3. 最后才做 `SessionUpdate/ensureReady/promptStream` 的结构压缩，并配套并发与回归测试。

---

审计日期：2026-04-29
范围：`server/internal/hub/client/session.go` + 关键入口 `server/internal/hub/client/client.go`
