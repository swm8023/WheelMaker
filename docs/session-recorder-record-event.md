# SessionRecorder `RecordEvent` 流程梳理

更新时间：2026-04-24
状态：Draft
**类别：内部实现分析**（非对外协议文档）

> 本文档描述 Go 服务端内部实现逻辑。对外协议契约参见 [app-chat-recorder-sync-protocol.zh-CN.md](./app-chat-recorder-sync-protocol.zh-CN.md) 和 [registry-protocol.md](./registry-protocol.md)。

## 1. 范围

本文只描述当前实现里 `server/internal/hub/client/session_recorder.go` 的 `RecordEvent` 链路，覆盖：

- 事件从哪里来
- `SessionViewEvent` 如何被解析
- ACP / system 消息如何转成中间类型与 IM 形状
- turn merge 如何判定与落库
- SQLite 落库点
- `publish` 的 payload 长什么样
- `session.read` 读到的最终内容与 push 的差异

本文不讨论 UI 渲染策略，也不讨论未来应该如何重构；这里只记录当前代码的真实行为，方便后续直接改流程与中间类型。

## 2. 总调用链

```text
Client.HandleSessionRequest("session.new")
  -> Client.RecordEvent(...)
  -> SessionRecorder.RecordEvent(...)

Session.handlePromptBlocks / Session.replyWithTitle / Session.decidePermission
  -> Session.recordSessionViewEvent(...)
  -> SessionViewSink.RecordEvent(...)
  -> SessionRecorder.RecordEvent(...)

SessionRecorder.RecordEvent(...)
  -> parseSessionViewEvent(...)
  -> one of:
     - upsertSessionProjection(...)
     - handlePromptStartedLocked(...)
     - handlePromptFinishedLocked(...)
     - appendACPEventMessageLocked(...)
  -> Store.SaveSession / UpsertSessionPrompt / UpsertSessionTurn
  -> publishSessionUpdated / publishSessionTurn
  -> registry.session.updated / registry.session.message
  -> Registry Server rebroadcast as session.updated / session.message
```

相关代码：

- [session_recorder.go](../server/internal/hub/client/session_recorder.go)
- [session.go](../server/internal/hub/client/session.go)
- [permission.go](../server/internal/hub/client/permission.go)
- [client.go](../server/internal/hub/client/client.go)
- [sqlite_store.go](../server/internal/hub/client/sqlite_store.go)
- [server.go](../server/internal/registry/server.go)

## 3. 入口事件从哪里产生

当前 `RecordEvent` 主要有 3 组调用源。

| 来源 | 入口函数 | `SessionViewEvent.Type` | `Content` 形状 |
| --- | --- | --- | --- |
| 新建 session | `Client.HandleSessionRequest("session.new")` | `acp` | `buildACPMethodParamsContent(session.new, ...)` |
| prompt 开始 / 流式 update / prompt 结束 | `Session.handlePromptBlocks` | `acp` | `session.prompt params` / `session.update params` / `session.prompt result` |
| system 文本 | `Session.replyWithTitle` | `system` | 纯文本 |

辅助构造函数都在 [session_recorder.go](../server/internal/hub/client/session_recorder.go) 底部：

- `buildACPMethodParamsContent`
- `buildACPMethodResultContent`
- `buildACPMethodRequestContent`
- `buildACPMethodResponseContent`

这些函数会把内容包成统一 JSON，例如：

```json
{
  "method": "session.update",
  "params": { ... }
}
```

permission 不再进入 recorder，所以这里不会再出现 `request_permission` 形状的事件文档。

`Session.recordSessionViewEvent` 还会补齐外层字段：

- 如果 `event.SessionID` 为空，则回填为 `s.ID`
- 如果 `event.UpdatedAt` 为空，则回填 `time.Now().UTC()`
- 如果当前 session 绑定了 IM chat，则补 `SourceChannel` 与 `SourceChatID`

注意：`RecordEvent` 的路由 key 始终使用外层 `event.SessionID`。内层 ACP payload 自带的 `sessionId` 不参与最终落库 key 决策。现有测试 [client_test.go](../server/internal/hub/client/client_test.go) 中的 `TestSessionRecorderUsesClientSessionIDWhenACPEventCarriesDifferentSessionID` 已验证这一点。

## 4. 核心中间类型

### 4.1 `SessionViewEvent`

这是 `RecordEvent` 的原始输入。

```go
type SessionViewEvent struct {
    Type      SessionViewEventType
    SessionID string
    Content   string

    SourceChannel string
    SourceChatID  string
    UpdatedAt     time.Time
}
```

其中：

- `Type=acp` 表示 `Content` 需要先解 JSON-RPC 风格的 ACP 文档
- `Type=system` 表示 `Content` 就是一段纯文本 system message

### 4.2 `sessionViewACPContentDoc`

这是 `Content` 被 decode 后的统一 ACP 文档。

```go
type sessionViewACPContentDoc struct {
    ID     int64           `json:"id,omitempty"`
    Method string          `json:"method"`
    Params json.RawMessage `json:"params,omitempty"`
    Result json.RawMessage `json:"result,omitempty"`
}
```

### 4.3 `sessionViewParsedEvent`

`parseSessionViewEvent` 会先把输入归一成下面这组字段，再由 `RecordEvent` 分发：

```go
type sessionViewParsedEvent struct {
    event            SessionViewEvent
    skip             bool
    method           string
    sessionTitle     string
    hasPromptResult  bool
    promptStopReason string
    promptParams     acp.SessionPromptParams
    hasTurnMessage   bool
    turnMessage      sessionViewTurnMessage
}
```

### 4.4 `sessionViewTurnMessage`

这是所有可持久化 turn 的统一中间形状。

```go
type sessionViewTurnMessage struct {
    IMMessage acp.IMMessage
    MergeKey  sessionTurnKey
}

type sessionTurnKey struct {
    ToolCallID          string
    PermissionRequestID int64
}
```

也就是说，`RecordEvent` 真正落库的不是原始 ACP 文档，而是被转换后的 `IMMessage`。

### 4.5 `sessionPromptState`

这是 merge 行为的核心状态，完全驻留内存。

```go
type sessionPromptState struct {
    promptIndex   int64
    nextTurnIndex int64

    turns                     map[int64]SessionTurnRecord
    toolTurnByToolCallID      map[string]int64
    permissionTurnByRequestID map[int64]int64
}
```

用途：

- 跟踪当前 prompt 编号
- 给新 turn 分配 `turnIndex`
- 记录某个 `toolCallId` 对应哪条 turn
- 记录某个 permission `requestId` 对应哪条 turn
- 记录最后一条 turn 的 `method`，用于文本 chunk merge

### 4.6 最终持久化类型

```go
type SessionRecord struct {
    ID           string
    ProjectName  string
    Status       SessionStatus
    ACPSessionID string
    AgentsJSON   string
    Title        string
    Agent        string
    CreatedAt    time.Time
    LastActiveAt time.Time
    InMemory     bool
}

type SessionPromptRecord struct {
    SessionID   string
    PromptIndex int64
    Title       string
    StopReason  string
    UpdatedAt   time.Time
}

type SessionTurnRecord struct {
    SessionID   string
    PromptIndex int64
    TurnIndex   int64
    UpdateIndex int64
    UpdateJSON  string
    ExtraJSON   string
}
```

## 5. 协议类型映射

### 5.1 ACP 侧输入类型

当前转换链最关键的 ACP 类型有：

```go
type SessionPromptParams struct {
    SessionID string
    Prompt    []ContentBlock
}

type SessionPromptResult struct {
    StopReason string
}

type SessionUpdateParams struct {
    SessionID string
    Update    SessionUpdate
}

type SessionUpdate struct {
    SessionUpdate string
    Content       json.RawMessage
    ToolCallID    string
    Title         string
    Kind          string
    Status        string
    Entries       []PlanEntry
    RawInput      json.RawMessage
    RawOutput     json.RawMessage
}

type PermissionRequestParams struct {
    SessionID string
    ToolCall  ToolCallRef
    Options   []PermissionOption
}

type PermissionResponse struct {
    Outcome PermissionResult
}
```

定义见 [acp.go](../server/internal/protocol/acp.go)。

### 5.2 IM 侧中间类型

所有最终 turn 都会先转成 `IMMessage`：

```go
type IMMessage struct {
    Method  string          `json:"method"`
    Index   string          `json:"index,omitempty"`
    Request json.RawMessage `json:"request,omitempty"`
    Result  json.RawMessage `json:"result,omitempty"`
}

type IMPromptRequest struct {
    ContentBlocks []ContentBlock `json:"contentBlocks,omitempty"`
}

type IMPermissionRequest struct {
    ToolCallID string            `json:"toolCallId,omitempty"`
    Options    []IMRequestOption `json:"options,omitempty"`
}

type IMPermissionResult struct {
    ToolCallID string `json:"toolCallId,omitempty"`
    Selected   string `json:"selected,omitempty"`
}

type IMTextResult struct {
    Text string `json:"text"`
}

type IMToolResult struct {
    Cmd    string `json:"cmd,omitempty"`
    Kind   string `json:"kind,omitempty"`
    Status string `json:"status,omitempty"`
    Output string `json:"output,omitempty"`
}
```

定义见 [im_protocol.go](../server/internal/protocol/im_protocol.go)。

## 6. `RecordEvent` 主流程

`RecordEvent` 本身很短，真正复杂度都在它调用的几个 helper 里。

```go
func (r *SessionRecorder) RecordEvent(ctx context.Context, event SessionViewEvent) error {
    parsed, err := parseSessionViewEvent(event)
    ...
    r.writeMu.Lock()
    defer r.writeMu.Unlock()

    switch parsed.method {
    case session.new:
        return r.upsertSessionProjection(...)
    case session.prompt:
        if parsed.hasPromptResult {
            return r.handlePromptFinishedLocked(...)
        }
        return r.handlePromptStartedLocked(...)
    case session.update, system:
        return r.appendACPEventMessageLocked(...)
    default:
        return nil
    }
}
```

这里有两个锁：

- `writeMu`：保护 prompt state 与写入顺序
- `mu`：只保护 `publish` 函数本身的读写

### 6.1 第一步：`parseSessionViewEvent`

`parseSessionViewEvent` 先做统一预处理：

1. trim `event.SessionID`
2. 如果外层 `SessionID` 为空，直接 `skip=true`
3. 如果 `UpdatedAt` 为空，回填 `time.Now().UTC()`
4. 如果 `Type=acp`，先把 `Content` decode 成 `sessionViewACPContentDoc`
5. 用 `sessionViewMethodFromEvent` 决定最终分支 method

`sessionViewMethodFromEvent` 的规则：

- ACP 事件优先使用 `doc.Method`
- 非 ACP 且 `Type=system` 时，method 固定为 `"system"`

然后进入 method-specific parse：

- `session.new`：只解析 params 里的 `title`
- `session.prompt`：先尝试解 result；如果 result 存在，就认为这是 prompt 完成事件，不再解析 params；否则解析 prompt params
- `session.update`：把 ACP update 转成 `sessionViewTurnMessage`
- `system`：把 ACP system 或 legacy system 文本转成 `sessionViewTurnMessage`

`parseSessionViewEvent` 只做“识别和转换”，不直接写库。

## 7. `session.new` 分支

分支入口：`RecordEvent -> upsertSessionProjection`

行为：

1. `store.LoadSession(projectName, sessionID)`
2. 如果记录不存在，创建新的 `SessionRecord`
3. 更新 `Title` 与 `LastActiveAt`
4. 调 `store.SaveSession`
5. 调 `publishSessionUpdated`

`SessionRecord` 的 upsert 语义在 [sqlite_store.go](../server/internal/hub/client/sqlite_store.go) 中是：

- `title` 只有在新值非空时才覆盖旧值
- `last_active_at` 每次都更新
- `created_at` 只在插入时写入

publish payload 形状：

```json
{
  "session": {
    "sessionId": "sess-1",
    "title": "Task",
    "updatedAt": "2026-04-24T10:00:00Z",
    "agent": "claude"
  }
}
```

## 8. `session.prompt` 开始分支

分支入口：`RecordEvent -> handlePromptStartedLocked`

### 8.1 prompt index 如何分配

先调用 `nextPromptStateLocked`：

- 如果内存里已经有当前 session 的 `promptState`，并且对应 prompt 还存在于 store 中，则直接 `current.promptIndex + 1`
- 如果内存态丢了，或者对应 prompt 已不存在，则从 `ListSessionPrompts` 重新计算
- 如果完全没有 prompt，则从 1 开始

这意味着：

- prompt 是否“正式结束”并不是进入下一 prompt 的前置条件
- 即使上一轮没有收到 `session.prompt result`，只要下一次 `session.prompt params` 进来，也会直接开始新 prompt

现有测试 [client_test.go](../server/internal/hub/client/client_test.go) 中的 `TestSessionViewNextPromptFlushesPreviousWithoutPromptFinished` 已验证这一点。

### 8.2 prompt row 如何落库

`handlePromptStartedLocked` 会先从 `params.Prompt` 里提一个标题：

- 纯 text block：按换行拼起来
- 没有 text 但有 image block：标题固定为 `Sent an image`
- 其他情况：空字符串

然后写入：

```go
SessionPromptRecord{
    SessionID:   event.SessionID,
    PromptIndex: state.promptIndex,
    Title:       promptTitle,
    UpdatedAt:   event.UpdatedAt,
}
```

### 8.3 prompt 本身如何变成第一条 turn

prompt params 不会原样存 ACP JSON，而是立即转换成一条 `IMMessage`：

```go
IMMessage{
    Method:  "prompt",
    Request: IMPromptRequest{ContentBlocks: params.Prompt},
}
```

然后：

1. `withIMTurnIndex(..., promptIndex, 1)` 写入 `index = p{prompt}.t1`
2. `buildSessionTurnRecord(..., turnIndex=1, updateIndex=1, extra={})`
3. `appendSessionTurnLocked`

最终 prompt 第一条消息会被写入 `session_turns`。

### 8.4 prompt 开始会触发哪些 publish

prompt 开始时会有两次对外可见效果：

- `registry.session.message`：prompt turn 本身
- `registry.session.updated`：session summary，`updatedAt` 刷新

其中 `upsertSessionProjection(..., titleIfEmptyOnly=true)` 的语义是：

- session title 只有在原本为空时才会被 prompt title 补上
- `LastActiveAt` 仍然会更新

## 9. `session.prompt` 结束分支

分支入口：`RecordEvent -> handlePromptFinishedLocked`

这里的“结束”只认 `session.prompt` 的 `result.stopReason`，不会写一条独立 turn。

行为：

1. `ensurePromptStateLocked` 确保当前 prompt state 存在
2. `UpsertSessionPrompt` 更新 `stop_reason` 和 `updated_at`
3. `delete(r.promptState, event.SessionID)` 删除该 session 的内存 prompt state

需要注意一个边界行为：

- 如果当前 session 还没有任何 prompt，`ensurePromptStateLocked` 会先补建 `PromptIndex=1` 的 `SessionPromptRecord`
- 也就是说，单独一条 `session.prompt result` 先到达时，不会直接丢弃，而是会先造出 prompt 行再写 `stop_reason`

结果：

- `session_prompts` 行会更新 `stop_reason`
- `session_turns` 不新增记录
- 不会发 `registry.session.message`
- 也不会额外发 `registry.session.updated`

这是一个重要事实：prompt 完成是“更新 prompt 行”，不是“新增 turn”。

## 10. `session.update` / `system` 分支

这两类事件都会进入 `appendACPEventMessageLocked`。

### 10.1 共同前置条件

`appendACPEventMessageLocked` 第一件事是调用 `currentPromptStateLocked`。如果当前 session 没有 active prompt，则整条事件直接丢弃。

所以：

- 没有 prompt 的 `session.update` 会被丢弃
- legacy system text 只有发生在 prompt 生命周期内才会落库

测试覆盖：

- `TestSessionViewUpdateWithoutPromptIsDropped`
- `TestSessionViewSystemMessageIsNotPersisted`

### 10.2 从 ACP / system 转成 `sessionViewTurnMessage`

#### a. 文本 update

支持的 ACP update type：

- `agent_message_chunk`
- `agent_thought_chunk`
- `user_message_chunk`

转换结果：

```go
IMMessage{
    Method: update.SessionUpdate,
    Result: IMTextResult{Text: extractUpdateText(update.Content)},
}
```

#### b. tool update

支持的 ACP update type：

- `tool_call`
- `tool_call_update`

转换结果：

```go
IMMessage{
    Method: "tool_call",
    Result: IMToolResult{
        Cmd:    update.Title,
        Kind:   update.Kind,
        Status: update.Status,
        Output: extractUpdateText(update.Content) or stringifyRawJSON(update.RawOutput),
    },
}
MergeKey.ToolCallID = update.ToolCallID
```

注意：`tool_call` 和 `tool_call_update` 最终都被折叠成 IM method `tool_call`，真正区分 merge 的是 `toolCallId`。

#### c. plan update

`session.update` 中的 `plan` 会转成：

```go
IMMessage{
    Method: "agent_plan",
    Result: []IMPlanResult{...},
}
```

plan turn 不参与 merge。

#### d. system

两条路都会落成相同的 IM 形状：

- ACP event，`doc.Method == "system"`
- legacy outer event，`event.Type == system`

统一转换成：

```go
IMMessage{
    Method: "system",
    Result: IMTextResult{Text: text},
}
```

测试覆盖：`TestSessionViewStoresSystemMethodFromACPAndLegacyEvents`。

### 10.3 merge 判定规则

`appendACPEventMessageLocked` 不看原始 ACP JSON；它只看已经构造好的 `sessionViewTurnMessage` 和当前 `sessionPromptState`。

`getMergedTurnFromTurnMessage` 的规则如下：

| 类型 | merge key | 命中条件 | 合并函数 |
| --- | --- | --- | --- |
| text chunk | 无显式 key | 上一条 turn 的 IM `method` 与当前一致 | `mergeTextResultMessage` |
| tool | `ToolCallID` | `state.toolTurnByToolCallID[toolCallId]` 命中 | `mergeToolResultMessage` |
| permission | `PermissionRequestID` | `state.permissionTurnByRequestID[requestId]` 命中 | `mergePermissionMessage` |
| plan/system/其他 | 无 | 默认不 merge | 无 |

这里有两个非常重要的前提：

- text merge 只看“上一条 turn 的 method 是否相同”，不是按 ACP `sessionId` 或别的 key
- tool / permission merge 的 key 来自 `ExtraJSON` 反解出的 meta，而 meta 只存在于已经落库过的 turn 上

也就是说，merge 依赖当前 prompt 的内存态，而不是重新扫描整个数据库。

### 10.4 merge 成功时写什么

如果命中已有 turn：

1. 先把本次 incoming message 用已有 `turnIndex` 写上 `Index = pN.tM`
2. 调 `mergeTurnRecord(existing, indexedIncoming, plan)` 得到新的完整 turn snapshot
3. `appendSessionTurnLocked(ctx, mergedTurn, indexedIncomingRaw)`
4. `state.assignTurn(mergedTurn)` 回写 promptState

`mergeTurnRecord` 会把 `UpdateIndex` 递增：

```go
merged.UpdateIndex = max(existing.UpdateIndex, 0) + 1
```

三种 merge 函数的实际行为：

#### text merge

```go
inc.Text = base.Text + inc.Text
```

效果：

- 第二个 chunk 到来后，数据库里保存的是拼好的全文
- `UpdateIndex` 递增

测试覆盖：

- `TestSessionViewAssistantChunksReusePreviousTurnByUpdateType`
- `TestSessionViewBufferedUpdatesReusePreviousTurnByUpdateType`

#### tool merge

规则：

- `Cmd/Kind/Status` 缺失时继承旧值
- `Output` 两边都有时做字符串拼接 `base.Output + inc.Output`
- `Request` 缺失时沿用旧值

测试覆盖：

- `TestSessionViewToolCallAndUpdateMergeByToolCallID`
- `TestSessionViewToolCallTerminalUpdatesRemainSingleTurn`

#### permission merge

规则：

- incoming 没有 `Request` 时沿用旧 request
- incoming 没有 `Result` 时沿用旧 result
- incoming result 里 `ToolCallID` 为空时，先尝试从 incoming request 回填，再从旧 result 回填
- incoming result 里 `Selected` 为空时，继承旧 result 的 `Selected`

测试覆盖：

- `TestSessionViewPermissionRequestResolveUsesSingleTurn`

### 10.5 merge 失败时写什么

如果没有命中已有 turn：

- 普通情况：新建 `TurnIndex = state.nextTurnIndex` 的 turn
- 特例：permission result 如果没有对应 request，会被直接丢弃

permission orphan 丢弃逻辑在这里：

```go
if plan.kind == sessionTurnMergePermission && plan.hasPermissionResult {
    return nil
}
```

测试覆盖：`TestSessionViewDropsOrphanPermissionResult`。

## 11. `UpdateJSON` / `ExtraJSON` 到底存什么

### 11.1 `UpdateJSON`

`session_turns.update_json` 存的是“当前 turn 的最新完整快照”，不是原始 ACP 文档。

示例：

- prompt turn：`IMMessage{method:"prompt", request:...}`
- assistant 文本 turn：`IMMessage{method:"agent_message_chunk", result:{text:"hello world"}}`
- tool turn：`IMMessage{method:"tool_call", result:{cmd:"build", status:"completed", ...}}`
- permission turn：`IMMessage{method:"permission", request:{...}, result:{...}}`

每次写入前都会过 `normalizeJSONDoc`：

- 空字符串或非法 JSON 会退化成 `{}`
- 合法 JSON 原样保留

### 11.2 `ExtraJSON`

`session_turns.extra_json` 存的是 merge 元信息：

```go
type sessionTurnMeta struct {
    ToolCallID          string `json:"toolCallId,omitempty"`
    PermissionRequestID int64  `json:"permissionRequestId,omitempty"`
}
```

写入时机：

- prompt turn 走 `marshalIMMessage(...)`，`extra_json` 固定是 `{}`
- 其他从 `sessionViewTurnMessage` 转出来的 turn 走 `marshalConvertedMessage(...)`
- `marshalConvertedMessage` 会把 `MergeKey` 先转成 `sessionTurnMeta`，再序列化进 `ExtraJSON`

它不参与 `session.read` 返回，但参与后续 merge 索引构建。

### 11.3 `Index` 字段格式

IM message 的 `Index` 由 `withIMTurnIndex` 写入，格式是：

- prompt index：`p1`
- turn index：`p1.t1`

具体格式函数在 [sqlite_store.go](../server/internal/hub/client/sqlite_store.go)：

- `formatPromptSeq`
- `formatPromptTurnSeq`

## 12. SQLite 落库点

### 12.1 `sessions`

由 `upsertSessionProjection -> SaveSession` 写入。

关心字段：

- `id`
- `project_name`
- `title`
- `created_at`
- `last_active_at`

### 12.2 `session_prompts`

由：

- `handlePromptStartedLocked`
- `handlePromptFinishedLocked`
- `ensurePromptStateLocked`（补洞时）

写入规则：

- 主键：`(session_id, prompt_index)`
- `title` 只有新值非空时覆盖
- `stop_reason` 只有新值非空时覆盖
- `updated_at` 取较大的时间戳

### 12.3 `session_turns`

由 `appendSessionTurnLocked -> UpsertSessionTurn` 写入。

写入规则：

- 主键：`(session_id, prompt_index, turn_index)`
- `update_index` 只取较大值
- `update_json` 与 `extra_json` 总是被最新值覆盖

这意味着：

- 同一逻辑 turn 的历史 update 不会保留多行
- 数据库里永远只保存“这个 turn 的最新快照”
- `UpdateIndex` 只是告诉你这条 turn 被覆盖过多少次

## 13. publish 行为

### 13.1 `registry.session.updated`

来源：`publishSessionUpdated`

payload：

```json
{
  "session": {
    "sessionId": "sess-1",
    "title": "Task",
    "updatedAt": "2026-04-24T10:00:00Z",
    "agent": "claude"
  }
}
```

触发时机：

- `session.new`
- prompt 开始时的 `upsertSessionProjection`

不会在以下时机触发：

- prompt finish
- 普通 `session.update`
- permission request/result

### 13.2 `registry.session.message`

来源：`publishSessionTurn`

payload：

```json
{
  "sessionId": "sess-1",
  "promptIndex": 1,
  "turnIndex": 2,
  "updateIndex": 2,
  "content": "{...}"
}
```

注意点：

- payload 没有 `turnId`
- `content` 默认取 `message.UpdateJSON`
- 但 merge 命中时，`content` 会改成“本次 incoming 的 indexed raw 内容”，而不是数据库里的 merged 快照

这点很关键。现有测试 `TestSessionViewMergedTurnPublishesIncomingContentWithMergedIndices` 验证了：

- 同一条 turn 第二次 merge 后
- publish 出去的 `turnIndex` 还是老 turn
- `updateIndex` 已递增
- `content` 只包含本次增量文本，比如 `world`

所以 push 语义是：

- 索引是最新的
- 内容是当前增量

而不是：

- 索引最新
- 内容也是数据库里的完整 merged 快照

### 13.3 registry server 如何转发

Hub 侧发的是：

- `registry.session.updated`
- `registry.session.message`

Registry server 会在 [server.go](../server/internal/registry/server.go) 中把它们 rebroadcast 成 client event：

- `session.updated`
- `session.message`

project scope 会挂在 websocket envelope 的 `ProjectID` 上，原始 payload 基本原样透传。

## 14. `session.read` 读到的内容

`session.read` 最终走到 `SessionRecorder.ReadSessionMessages`。

这个读取路径有 3 个关键事实：

1. 它读的是 store 里的 `SessionTurnRecord.UpdateJSON`
2. 所以它看到的是“每个 turn 的最新完整快照”
3. 它的增量游标是 `(promptIndex, turnIndex)`，不是 `(promptIndex, turnIndex, updateIndex)`

返回结构：

```go
type sessionViewMessage struct {
    SessionID   string
    PromptIndex int64
    TurnIndex   int64
    UpdateIndex int64
    Content     string
}
```

`Content` 直接来自 `UpdateJSON`。

这和 push 路径的差异是：

- `session.read` 返回完整 merged snapshot
- `session.message` push 在 merge 时返回增量 content

另外，`ReadSessionMessages` 的过滤条件只比较：

- `turn.PromptIndex`
- `turn.TurnIndex`

不会比较 `UpdateIndex`。

当前实现的直接后果是：

- pull 模式按 turn 粒度同步
- push 模式可以在同一 `turnIndex` 上看到更高的 `updateIndex`
- 但纯 pull 模式无法只通过 `(afterPromptIndex, afterTurnIndex)` 精确请求“同一 turn 的新版本”

这也是当前设计里最值得注意的行为差异之一。

## 15. 当前实现的隐含约束与后果

下面这些不是猜测，而是当前代码已经成立的约束：

1. `RecordEvent` 最终落库的是 IM 形状，不是原始 ACP 形状。
2. `session_turns` 一条逻辑 turn 只有一行，后续 update 只能覆盖这行，历史增量不会分行保存。
3. merge 判定依赖内存 `promptState`；`toolCallId` 和 `requestId` 都要先进入 `ExtraJSON` 才能驱动下一次 merge。
4. 外层 `event.SessionID` 才是最终持久化 session key，内层 ACP `sessionId` 只作为内容字段存在。
5. 没有 active prompt 时，`session.update` / `system` 都可能被静默丢弃。
6. permission 不再进入 recorder，因此不会生成 turn，也不存在 permission merge/orphan 逻辑。
7. prompt finish 不会新增 turn，只会更新 `session_prompts.stop_reason`。
8. `session.updated` 不是“所有变化都发”；它只在 session projection 被 upsert 时发。
9. `session.read` 与 `session.message` 的语义不同：前者返回 merged snapshot，后者在 merge 时返回带新索引的增量 content。

如果后续你要改流程，至少要先决定下面几件事是否保留：

- 持久化层是否继续以 IM 形状为中心
- turn 是否继续按覆盖快照模型存储
- pull 游标是否需要纳入 `UpdateIndex`
- prompt finish 是否仍然不落 turn
- session summary 的更新时间是否只在 prompt 开始时刷新

## 16. 现有测试可作为回归基线

下面这些测试已经覆盖了当前行为，后续改流程时可以直接对照：

- `TestBuildConvertedMessageFromSessionUpdateIncludesToolMergeKey`
- `TestBuildConvertedPermissionMessageIncludesRequestMergeKey`
- `TestSessionViewAssistantChunksReusePreviousTurnByUpdateType`
- `TestSessionViewToolCallAndUpdateMergeByToolCallID`
- `TestSessionViewToolCallTerminalUpdatesRemainSingleTurn`
- `TestSessionViewPermissionRequestResolveUsesSingleTurn`
- `TestSessionViewDropsOrphanPermissionResult`
- `TestSessionViewStoresSystemMethodFromACPAndLegacyEvents`
- `TestSessionViewUpdateWithoutPromptIsDropped`
- `TestSessionViewMergedTurnPublishesIncomingContentWithMergedIndices`
- `TestSessionViewNextPromptFlushesPreviousWithoutPromptFinished`

测试文件：

- [client_test.go](../server/internal/hub/client/client_test.go)