# Session 对话管理与同步

本文是当前 session 对话链路的唯一设计说明。旧的 `session_prompts`、`turns_json`、`promptIndex` 游标和一次性迁移工具都已移除；运行期协议只暴露 session 级全局 `turnIndex`。

## 1. 数据模型

SQLite 只保存会话索引和热状态，不保存对话正文：

- `sessions.id`：ACP session id，也是 app/web 侧的 session id。
- `sessions.project_name`：项目名。
- `sessions.status`：会话生命周期状态。
- `sessions.agent_type` / `sessions.agent_json`：agent 类型和运行态快照。
- `sessions.title`：会话标题，通常由最新用户 prompt 更新；服务端和 app 不再用 session id 或消息内容合成 fallback 标题。
- `sessions.created_at` / `sessions.updated_at`：创建时间和最后活动时间。`updated_at` 在 prompt start 和 prompt done 时都会更新，保存时不会被更旧的事件时间回退。
- `sessions.session_sync_json`：同步投影，目前只保存：

```json
{"latestPersistedTurnIndex":132}
```

`latestPersistedTurnIndex` 表示已经写入 turn 文件的最大 turn。服务端列 session 时会再叠加内存中的 live turn，得到返回给客户端的 `latestTurnIndex`。

## 2. Turn 语义

一个 session 内所有消息共享单调递增的 `turnIndex`，从 1 开始。`prompt_request`、agent message、thought、plan、tool call、`prompt_done` 都是普通 turn。

实时事件统一为：

```json
{
  "sessionId": "sess-1",
  "turnIndex": 12,
  "content": "{\"method\":\"agent_message_chunk\",\"param\":{\"text\":\"...\"}}",
  "finished": false
}
```

规则：

- 不再有 `promptIndex`、`turnId`、`updateIndex`。
- `content` 是 IM turn JSON，包含 `method` 和 `param`。
- `prompt_request.param` 会写入 `createdAt` 和当前 `modelName`。
- `prompt_done.param` 会写入 `completedAt` 和 `stopReason`。
- 实时消息和 `session.read` 返回都使用 `finished`；旧的 `done` 字段不参与解析。
- tool call 按 tool call id 合并到同一个 turn。
- 连续 `agent_message_chunk` 或连续 `agent_thought_chunk` 合并到同一个 turn。
- 文本流式 turn 先以 `finished=false` 发布；当下一个 turn 或 `prompt_done` 到来时，服务端用同一个 `turnIndex` 重发完整内容并标记 `finished=true`。
- 如果新 prompt 到来时上一个 prompt 还没有 terminal turn，服务端先合成 `prompt_done(stopReason="interrupted")`，再开始新 prompt。

## 3. 序列化

完成一个 prompt 时，`SessionRecorder` 会把该 prompt 内尚未持久化的 turns 追加写入二进制 turn 文件，然后更新 `sessions.session_sync_json`。

路径：

```text
~/.wheelmaker/session/<projectName>/<sessionId>/turns/t000000.bin
```

文件格式：

- 每个文件保存 128 个 turn。
- 文件头：`WMT2` magic、version、128 个 slot。
- 每个 slot 保存 body 的 `offset` 和 `len`。
- body 保存对应 turn 的 `content` bytes。
- turn index 由文件号和 slot 推导。
- 已写入文件的 turn 视为 `finished=true`。

写入顺序是先 append body，再同步文件，再写 header slot，再同步 header。header 未写成功时 slot 仍为 0，读路径不会把该 turn 视为存在。

## 4. 服务端读写流程

`RecordEvent` 接收 ACP/session 事件后转成 IM turn：

1. `session/new` 更新或创建 `sessions` 投影。
2. `session/prompt` params 生成 `prompt_request` turn。
3. `session/update` 生成 agent/tool/thought/plan/user chunk turn。
4. `session/prompt` result 生成 `prompt_done` turn，并触发本 prompt 的 turn 文件落盘。

`session.read` 请求：

```json
{"sessionId":"sess-1","afterTurnIndex":128}
```

响应：

```json
{
  "latestTurnIndex": 132,
  "turns": [
    {"sessionId":"sess-1","turnIndex":129,"content":"...","finished":true}
  ]
}
```

读取顺序：

- `afterTurnIndex < latestPersistedTurnIndex` 时，从 turn 文件读取持久化增量。
- 如果当前 session 有 live prompt state，再追加内存中 `turnIndex > afterTurnIndex` 且尚未持久化的 turns。
- 返回内容按 `turnIndex` 升序排序。

`session.reload` 会清除该 session 的内存 turn state、删除该 session 的 turn 文件、把 `session_sync_json` 重置为 `latestPersistedTurnIndex=0`，然后从 agent replay 回灌。

## 5. App/Web 同步

app/web 只维护 session 级 finished cursor：

```ts
type Cursor = { turnIndex: number };
```

同步规则：

- 本地消息 identity 是 `sessionId:turnIndex`。
- 服务端返回的每个 turn 必须带 `sessionId`，客户端不会用 read 请求里的 session id 兜底。
- 本地持久缓存只保存 `finished=true` 的消息。
- cursor 是本地已缓存 finished turn 的最大 `turnIndex`。
- 收到实时 `session.message` 后按 `sessionId:turnIndex` upsert。
- 只有 `finished=true` 的 turn 推进 cursor。
- 如果 incoming `turnIndex > cursor.turnIndex + 1`，说明漏收，客户端用当前 cursor 调 `session.read` 补读。
- `session.read` 返回的 turns 会和等待期间收到的实时消息 reconcile，避免旧缓存覆盖新流式内容。
- UI 可以按 `prompt_request` / `prompt_done` 临时分组展示对话，但这只是渲染派生状态，不参与协议和缓存 cursor。

打开项目、切换 tab、切换 session 时，补读流程异步执行，不阻塞 UI 交互。

## 6. 清理后的边界

当前实现不再包含：

- `session_prompts` 表。
- `SessionPromptRecord` store API。
- `turns_json` 正文存储。
- prompt 文件历史 adapter。
- 启动期旧 prompt 迁移代码。
- `promptIndex` 补读协议。
- app/web 侧 `prompts` read response 缓存。

历史迁移已经完成，后续版本启动时只做当前 SQLite schema 严格校验，不再执行旧结构自动迁移。
