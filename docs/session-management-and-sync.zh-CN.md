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
- `sessions.session_sync_json`：同步投影，保存服务端内部落盘进度和会话级 read/done cursor：

```json
{
  "latestPersistedTurnIndex": 132,
  "lastDoneTurnIndex": 132,
  "lastDoneSuccess": true,
  "lastReadTurnIndex": 128
}
```

- `latestPersistedTurnIndex` 表示已经写入 turn 文件的最大 turn。它不直接暴露给 app/web。
- `lastDoneTurnIndex` 表示最近一个 `prompt_done` turn。
- `lastDoneSuccess` 由 `prompt_done.param.stopReason !== "failed"` 推导。
- `lastReadTurnIndex` 表示该 session 已被任意客户端查看到的最大 done turn。当前没有 viewer 概念，因此是 session 级全局 read cursor。
- 服务端列 session 时会再叠加内存中的 live turn，得到返回给客户端的 `latestTurnIndex`。

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
- 服务端对客户端暴露的 `turnIndex` 必须连续；语义上为空的 turn 也必须返回一条非空 JSON `content`，不能跳过 index。
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
~/.wheelmaker/db/session/<projectName>/<sessionId>/turns/t000000.bin
```

文件格式：

- 当前写入格式是 `WMT2` v2，每个文件保存 256 个 turn。
- 文件头固定 8 字节：

```text
0..3  magic = "WMT2"
4..5  version = 2
6     chunkSizeCode = 0
7     reserved = 0
```

- `chunkSizeCode=0` 表示 256 turns/file；后续如需扩展，按 `256 << chunkSizeCode` 推导容量。
- 当前 session turn 存储只接受 256 turns/file，不再兼容旧的 v1/128 文件。
- header 后跟 256 个 slot。
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

`session.markRead` 请求：

```json
{"sessionId":"sess-1","lastReadTurnIndex":132}
```

服务端把 `lastReadTurnIndex` 按 `max(old, incoming)` 写入 `session_sync_json`，并返回更新后的 session summary。客户端只在用户打开 session，或当前可见 session 收到 `prompt_done` 后调用；列表刷新和后台事件不能清 read cursor。

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
- cursor 是本地已连续缓存 finished turn 的最大 `turnIndex`；如果本地缓存出现缺口，cursor 必须回退到缺口前。
- 收到实时 `session.message` 后按 `sessionId:turnIndex` upsert。
- 只有 `finished=true` 的 turn 推进 cursor。
- 如果 incoming `turnIndex > cursor.turnIndex + 1`，说明漏收，客户端用当前 cursor 调 `session.read` 补读。
- `session.read` 返回的 turns 会和等待期间收到的实时消息 reconcile，避免旧缓存覆盖新流式内容。
- UI 可以按 `prompt_request` / `prompt_done` 临时分组展示对话，但这只是渲染派生状态，不参与协议和缓存 cursor。
- UI 列表状态只看 session summary：`running=true` 显示进行中；否则当 `lastDoneTurnIndex > lastReadTurnIndex` 时，`lastDoneSuccess=false` 显示失败未查看，其他情况显示完成未查看。

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

## 7. Session 归档

`session.archive` 用于把非运行中的普通 session 移出常规会话系统。服务端按 turn 总数决定后续处理：`latestPersistedTurnIndex < 3` 的短会话直接永久删除；`latestPersistedTurnIndex >= 3` 的会话把已完成正文保留到冷归档文件。归档后 v1 不支持恢复、不提供归档列表/读取 API、不进入 monitor，也不提供 Feishu `/archive` 命令。

对外协议只暴露 `session.archive`：

- turn 总数 `< 3`：直接删除 `sessions` / `route_bindings` 和原 `db/session/<projectName>/<sessionId>` 目录，不写归档 pack、manifest 或 tombstone。
- turn 总数 `>= 3`：先写归档 pack 和 manifest，成功后删除 `sessions` / `route_bindings`，再删除原 `db/session/<projectName>/<sessionId>` 目录。

`session.delete` 不是对外协议；旧客户端发送该 method 会得到 unsupported method。内部仍可复用删除 helper 清理 active index、turn 文件和 session artifacts。

`session.archive`、`session.reload` 都必须拒绝运行中的 session。running 判定以服务端内存态为准：如果 session 仍有 active prompt 或 recorder 中存在未 terminal 的 prompt state，则返回错误；客户端的 `running` 字段只用于禁用按钮。

归档目录：

```text
~/.wheelmaker/db/session-archive/<projectName>/
  archive.pack
  manifest.json
```

其中 `<projectName>` 使用和普通 session 历史相同的 safe path segment。归档不保留 images；原 session 目录删除时一并删除图片。

### 7.1 Manifest

`manifest.json` 是归档索引的 source of truth，按 session id 做 map upsert。时间字段统一为 UTC RFC3339。

```json
{
  "version": 1,
  "updatedAt": "2026-05-17T12:34:56Z",
  "sessions": {
    "019e...": {
      "sessionId": "019e...",
      "projectName": "WheelMaker",
      "title": "...",
      "agentType": "codexapp",
      "createdAt": "2026-05-12T00:34:13Z",
      "updatedAt": "2026-05-17T12:00:00Z",
      "archivedAt": "2026-05-17T12:34:56Z",
      "turnCount": 932,
      "gapCount": 0,
      "storage": "pack",
      "file": "archive.pack",
      "offset": 123456,
      "length": 9876,
      "uncompressedLength": 45678,
      "codec": "gzip",
      "sha256": "...",
      "uncompressedSha256": "...",
      "wmt2Version": 2,
      "chunkSizeCode": 2
    }
  }
}
```

manifest 只保存索引和元信息，不保存 `agent_json`、`session_sync_json`、route binding 或图片信息。

### 7.2 Pack Segment

归档 pack 是 append-only。每个 session 作为一个独立 gzip segment 追加到 `archive.pack`。v1 不做 compact；manifest 未引用的 orphan bytes 会被忽略。

每个 segment 的外层 header：

```text
0..3   magic = "WMSA"
4..5   version = 1
6      codec = 1  // gzip
7      reserved = 0
8..9   sessionIDLen uint16 little-endian
10..17 payloadLen uint64 little-endian
18..25 uncompressedLen uint64 little-endian
26..   sessionID bytes
...    gzip(WMT2 bytes)
```

manifest 的 `offset` 指向 WMSA segment header 起点，`length` 是整个 segment 长度。

### 7.3 WMT2 聚合

segment 解压后的 payload 是一个标准 WMT2 v2 文件，只是 `chunkSizeCode` 可大于 0。容量公式：

```text
turnCapacity = 256 << chunkSizeCode
headerSize = 8 + turnCapacity * 8
```

归档时按 `turnCount` 选择能容纳所有 turns 的最小 `chunkSizeCode`，上限为 `chunkSizeCode <= 10`，即最多 `262144` turns。普通热 session 写入仍固定使用 `chunkSizeCode=0`、`256` turns/file，不改变热路径效率。

归档读取源是 `1..latestPersistedTurnIndex`。如果某个 turn 文件或 slot 缺失，归档写入非空 gap turn 占位，保持 turn index 连续：

```json
{"method":"session/archive_gap","param":{"reason":"missing_turn"}}
```

manifest 记录 `gapCount`。WMT2 slot 不能使用 `len=0` 表示 gap，因为现有格式把 offset 或 len 为 0 视为不存在。

### 7.4 写入和失败语义

归档写入顺序：

1. 校验 session 非 running。
2. 从 `sessions` 读取元信息和 `latestPersistedTurnIndex`。
3. 如果 `latestPersistedTurnIndex < 3`，直接删除 `route_bindings`、`sessions` 和原 session 目录并结束。
4. 读取普通 turn 文件并生成单 session WMT2 bytes，缺 turn 写 gap turn。
5. gzip 压缩 WMT2 bytes，计算压缩前后 SHA-256。
6. 持 project 级进程内锁 append WMSA segment 到 `archive.pack` 并 fsync。
7. 读取并 upsert `manifest.json`，写 temp 文件后 rename。
8. 删除 `route_bindings` 和 `sessions`。
9. 删除原 `db/session/<projectName>/<sessionId>` 目录。

长会话归档时，1-7 任一步失败都不能删除 active index。8-9 失败时 manifest 已存在；下一次 `session.archive` 对同一 session 应幂等地继续尝试删除 active index 和原目录。短会话删除不写归档痕迹。
