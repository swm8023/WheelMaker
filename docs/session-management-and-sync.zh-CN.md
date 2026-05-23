# Session 对话管理与同步

本文是 session 对话链路的基础协议、存储说明、turn-first 同步、状态和显示重构的主文档。旧的 `session_prompts`、`turns_json`、`promptIndex` 游标和一次性迁移工具都已移除；运行期协议只暴露 session 级全局 `turnIndex`。

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

服务端、app source store、IndexedDB 都使用同一个 raw turn shape：

```ts
type RegistrySessionTurn = {
  turnIndex: number;
  content: string;
  finished: boolean;
};
```

实时 `session.message` 事件 payload 统一为：

```json
{
  "sessionId": "sess-1",
  "turn": {
    "turnIndex": 12,
    "content": "{\"method\":\"agent_message_chunk\",\"param\":{\"text\":\"...\"}}",
    "finished": false
  }
}
```

规则：

- 不再有 `promptIndex`、`turnId`、`updateIndex`。
- `sessionId` 只在 payload 顶层；`turn` 内不重复 `sessionId`。
- 不兼容旧的扁平 `{sessionId, turnIndex, content, finished}` payload。
- `content` 是 IM turn JSON，包含 `method` 和 `param`。
- source store 和 IndexedDB 原样保存 `content`，不 parse-normalize 后重新 stringify。
- 服务端对客户端暴露的 `turnIndex` 必须连续；语义上为空的 turn 也必须返回一条非空 JSON `content`，不能跳过 index。
- `prompt_request.param` 会写入 `createdAt` 和当前 `modelName`。
- `prompt_done.param` 会写入 `completedAt` 和 `stopReason`。
- 实时消息和 `session.read` 返回都使用 `finished`；旧的 `done` 字段不参与解析。
- tool call 按 tool call id 合并到同一个 turn。
- 连续 `agent_message_chunk` 或连续 `agent_thought_chunk` 合并到同一个 turn。
- 文本/思考流式 turn 可以先以 `finished=false` 发布；服务端必须保证一个 session 最多只有当前尾部 turn 是 `finished=false`，不能出现中间 unfinished。
- 当下一个 turn 或 `prompt_done` 到来时，服务端用同一个 `turnIndex` 重发完整内容并标记 `finished=true`，再发布更大的 turn。
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
  "sessionId": "sess-1",
  "latestTurnIndex": 132,
  "session": {
    "sessionId": "sess-1",
    "latestTurnIndex": 132,
    "running": false,
    "lastDoneTurnIndex": 132,
    "lastDoneSuccess": true,
    "lastReadTurnIndex": 128
  },
  "turns": [
    {"turnIndex":129,"content":"...","finished":true}
  ]
}
```

读取顺序：

- response 顶层必须带 `sessionId`，客户端必须校验它与请求 session 一致。
- `turns[]` 内不带 `sessionId`。
- `session` summary 随 read 返回，方便激活 session 时一次同步 turns 和列表状态。
- `afterTurnIndex < latestPersistedTurnIndex` 时，从 turn 文件读取持久化增量。
- 如果当前 session 有 live prompt state，再追加内存中 `turnIndex > afterTurnIndex` 且尚未持久化的 turns。
- 返回内容按 `turnIndex` 升序排序，并覆盖 `afterTurnIndex+1..latestTurnIndex` 的连续区间。
- 语义缺失的 durable slot 在 read projection 中合成 `session/gap` turn，不写回 WMT2。
- `turns` 为空只允许在 `latestTurnIndex <= afterTurnIndex`。
- read response 最多包含一个 `finished=false` turn，且只能是返回区间的最后一条。

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

- 本地 raw turn identity 是 `sessionId:turnIndex`，但 wire/read 的 turn body 内不携带 `sessionId`。
- 本地持久缓存只保存 raw finished prefix：`1..Finished Cursor`。
- cursor 是本地已连续缓存 finished turn 的最大 `turnIndex`；如果本地缓存出现缺口，cursor 必须回退到缺口前。
- IndexedDB 保存 raw `turnsJson` 和 `cursorJson`；旧 `messagesJson` 或旧 schema/version 检测失败时，不做兼容迁移，除 token / 认证凭据外清空本地持久缓存并重建表，再由 `session.read` 重新补。
- finished prefix 变更后 5 秒 debounce 持久化；切换 session、切后台、断连前需要 flush。
- app/web 不维护有容量上限的运行集合。收到 `session.message` 的 session、被 hydrate 的 session、被 read 的 session、以及用户选择的 session 都进入内存 runtime store，直到页面生命周期结束或显式清除/reload。
- 用户打开 session 时，先 hydrate 本地 finished prefix；如果内存缺失或需要恢复可见内容，再调用 `session.read(after=Finished Cursor)`。
- 用户切回内存中已有 raw store 的 session 时，直接用内存 raw store 重建显示视图；必要的补读由 read repair / 可见恢复逻辑触发。
- 收到已知项目的实时 `session.message` 后，校验顶层 `sessionId` 和 `turn` shape，然后 upsert raw turn；未知 session 同时触发 project session list refresh，但不丢弃该条消息。
- 只有连续的 `finished=true` turn 推进 Finished Cursor。
- 如果 incoming `turnIndex > cursor.turnIndex + 1`，说明漏收，客户端用当前 cursor 调 `session.read` 补读；因此 `cursor=10` 收到 `12/false` 会 read，收到 `11/false` 不 read，之后收到 `12/true` 仍会因为 `12 > 10+1` read。
- 同一 session 最多一个 read in flight；等待期间新 gap 只设置 dirty flag，当前 read 返回后如仍不连续再读一次。
- `session.read` 返回的 turns 视为服务端权威结果，覆盖响应区间，并和等待期间收到的实时消息 reconcile，避免旧缓存覆盖新流式内容。
- `session.message` 只更新 turn store，不更新 title、preview、running、done、read、unread 状态。
- UI 列表状态只看 Session Summary：`running=true` 显示进行中；否则当 `lastDoneTurnIndex > lastReadTurnIndex` 时，`lastDoneSuccess=false` 显示失败未查看，其他情况显示完成未查看。
- 选中 session 的显示视图由 raw source store 派生完整轻量 Display Index，再由 `react-virtuoso` 只挂载 visible + overscan items。上滑/下滑只改变 virtualizer range，不触发 server read。
- 尾部锁定时新 turn 和 streaming 高度增长跟随到底；用户离开底部后保持当前锚点并显示回到底部 affordance。

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

对外协议暴露 `session.archive` 和 `session.delete`：

- turn 总数 `< 3`：直接删除 `sessions` / `route_bindings` 和原 `db/session/<projectName>/<sessionId>` 目录，不写归档 pack、manifest 或 tombstone。
- turn 总数 `>= 3`：先写归档 pack 和 manifest，成功后删除 `sessions` / `route_bindings`，再删除原 `db/session/<projectName>/<sessionId>` 目录。

`session.delete` 是硬删除协议：不写归档 pack、manifest 或 tombstone，直接删除 `sessions` / `route_bindings`、原 `db/session/<projectName>/<sessionId>` 目录和相关 artifacts。

`session.archive`、`session.delete`、`session.reload` 都必须拒绝运行中的 session。running 判定以服务端内存态为准：如果 session 仍有 active prompt 或 recorder 中存在未 terminal 的 prompt state，则返回错误；客户端的 `running` 字段只用于禁用按钮。

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
      "agentType": "codex",
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

---

## 8. Chat Turn 同步、状态与显示重构设计

本文定义 WheelMaker app/web 与 server 的第一阶段 chat session 同步目标模型。目标是让 session 状态、流式更新、客户端缓存、窗口渲染在断线、重连、漏收、乱序、隐藏 tool、长会话等场景下稳定正确。

本文是待 review 设计，不代表当前代码已经全部实现。

### 1. 设计目标

第一阶段目标：

- session 列表中的进行中、完成、失败未查看状态必须正确。
- 当前 session 的 streaming turn 必须稳定更新，同一 turn 原地覆盖，不产生重复或断裂。
- 进入内存 runtime store 的 session 可以全量加载 turns 到内存，简化同步模型。
- React/DOM 不能全量渲染历史，只挂载 `react-virtuoso` 的 visible + overscan items。
- 上滑/下滑只调整窗口，不触发 server read。
- 用户在底部时，新 turn 自动跟随；用户上翻后，新 turn 不强制跳底。
- 服务端和 app/web 同步升级，不保留旧 message/read wire 兼容。
- IndexedDB / 本地持久 cache schema 不做兼容迁移；版本不匹配或旧格式检测失败时，除 token / 认证凭据外清空本地持久缓存并重建表，由 server read 重新补。

非目标：

- 不做 `session.read` 分页。
- 不做 IndexedDB chunk cache。
- 不做 per-turn IndexedDB entry。
- 不做 WMT2 turn 正文精简或大内容外置。
- 不做多 viewer read cursor。

### 2. 核心原则

服务端提供权威 raw turn 流与 **Session Summary**。客户端维护内存 runtime store 中 session 的 raw turn source store、durable finished prefix、串行 read repair 和派生显示视图。

关键边界：

- `Finished Cursor` 是唯一 read cursor。
- `Live Turn Buffer` 可以显示，但不能推进 cursor。
- `Session Summary` 是 session list 状态的唯一来源。
- `session.message` 只更新 turn store，不更新 title、preview、running、done、read。
- `Display Index` 与 virtualized view 是派生显示，不是缓存源。

### 3. Wire 格式

#### 3.1 Raw Turn

服务端、app source store、IndexedDB 都使用同一个 raw turn shape：

```ts
type RegistrySessionTurn = {
  turnIndex: number;
  content: string;
  finished: boolean;
};
```

JSON 使用 camelCase：

```json
{
  "turnIndex": 12,
  "content": "{\"method\":\"agent_message_chunk\",\"param\":{\"text\":\"hello\"}}",
  "finished": false
}
```

规则：

- `turnIndex` 从 1 开始。
- `content` 是非空 IM turn JSON 字符串，包含 `method` 和 `param`。
- source store 和 IndexedDB 原样保存 `content`，不 parse-normalize 后重新 stringify。
- decode 只发生在 render、copy、prompt status、selected prompt_done markRead 等消费点。
- 如果 decode 失败，显示层可 fallback 为 system message；sync/cache 不因 decode 失败丢 raw turn。

#### 3.2 Realtime session.message

实时事件 payload：

```json
{
  "sessionId": "sess-1",
  "turn": {
    "turnIndex": 12,
    "content": "{\"method\":\"agent_message_chunk\",\"param\":{\"text\":\"hello\"}}",
    "finished": false
  }
}
```

规则：

- `sessionId` 只在 payload 顶层。
- `turn` 内不再重复 `sessionId`。
- 不兼容旧 `{sessionId, turnIndex, content, finished}` payload。
- 高频 `session.message` 不带 Session Summary。

#### 3.3 session.read

请求仍使用 Finished Cursor：

```json
{
  "sessionId": "sess-1",
  "afterTurnIndex": 128
}
```

响应：

```json
{
  "sessionId": "sess-1",
  "latestTurnIndex": 132,
  "session": {
    "sessionId": "sess-1",
    "title": "Build sync",
    "preview": "Build sync",
    "updatedAt": "2026-05-18T12:00:00Z",
    "messageCount": 132,
    "running": true,
    "latestTurnIndex": 132,
    "lastDoneTurnIndex": 120,
    "lastDoneSuccess": true,
    "lastReadTurnIndex": 120
  },
  "turns": [
    {
      "turnIndex": 129,
      "content": "{\"method\":\"agent_message_chunk\",\"param\":{\"text\":\"...\"}}",
      "finished": true
    }
  ]
}
```

规则：

- response 顶层必须带 `sessionId`，app 必须校验它与请求 session 一致。
- `turns[]` 内不带 `sessionId`。
- `session` summary 随 read 返回，方便激活 session 时一次同步 turns 和 list state。
- 当前迭代不做分页，返回 `afterTurnIndex` 后服务端已有的完整连续范围。
- `turns` 为空只允许在 `latestTurnIndex <= afterTurnIndex`。

### 4. 服务端不变量

#### 4.1 Turn 连续性

- 服务端对外暴露的 turn index 必须连续。
- WMT2 正常写入必须强拒绝洞：`startTurnIndex == latestPersistedTurnIndex + 1`。
- 空 content 必须拒绝。
- 语义缺失的 durable slot 在 read projection 中合成 `session/gap`，不写回 WMT2。

Hot gap raw turn：

```json
{
  "turnIndex": 42,
  "content": "{\"method\":\"session/gap\",\"param\":{\"reason\":\"missing_turn\",\"turnIndex\":42}}",
  "finished": true
}
```

#### 4.2 Unfinished Tail

服务端对外最多允许一个 unfinished turn：

- `finished:false` 只能用于最新 text streaming turn。
- `finished:false` 只用于 `agent_message_chunk` / `agent_thought_chunk`。
- `tool_call`、`agent_plan`、`prompt_request`、`prompt_done` 默认都是 `finished:true`。
- tool running 状态通过 `param.status` 表达，不用 `finished:false`。
- 发布更大 `turnIndex` 前，服务端必须先以同 index 发布前一个 text turn 的 `finished:true` seal。
- `session.read` 可以返回 finished prefix + 最多一个 unfinished tail，且 unfinished tail 必须是最后一个 turn。

#### 4.3 prompt_done 发布顺序

完成 prompt 时的发布顺序：

1. 持久化 prompt turns。
2. 发布必要的 sealed text turn。
3. 发布 `prompt_done` raw turn。
4. 发布 `session.updated` summary。

这样 selected session 可以先收到完整 turn stream，再由 summary/markRead response 收敛列表状态。

### 5. 客户端 Source Store

#### 5.1 类型分层

```ts
type RegistrySessionTurn = {
  turnIndex: number;
  content: string;
  finished: boolean;
};

type RegistryChatMessage = {
  sessionId: string;
  turnIndex: number;
  method: string;
  param: Record<string, unknown>;
  finished: boolean;
};
```

- `RegistrySessionTurn` 是 wire/cache/source store 类型。
- `RegistryChatMessage` 是 decoded view model，用于 render/copy/status。
- sync、cursor、cache 逻辑只依赖 raw turn 的 `turnIndex` 和 `finished`。

#### 5.2 Finished Store

- 每个进入内存的 session 保留全量 finished raw turns。
- Finished Store 可短期包含 cursor 之后的 finished turns，例如 `1,2,4`。
- Finished Cursor 只取连续 finished prefix，因此 `1,2,4` 的 cursor 是 2。
- 同 index finished turn 覆盖 live turn。

#### 5.3 Live Turn Buffer

- 保存内存中 session 的 unfinished raw turn。
- 正常情况下每个 session 最多一个 live tail。
- Live turn 可以参与 selected session 的显示。
- Live turn 不能推进 Finished Cursor，不能写 Durable Turn Cache。

#### 5.4 Same-Index 覆盖

同一个 `turnIndex` 的多次更新按 raw turn upsert：

- `finished:false` 覆盖之前的同 index `finished:false`。
- `finished:true` 覆盖同 index live，并从 Live Turn Buffer 移入 Finished Store。
- 普通 realtime `finished:false` 不覆盖已有 `finished:true`。
- `session.read` 返回的 covered range 是权威范围，可以覆盖该范围内的旧 raw turns。

### 6. In-Memory Runtime Store

客户端不再维护有容量上限的运行集合。任何收到 `session.message`、被 hydrate、被 read、或被用户选择的 session 都进入内存 runtime store，并保留到页面生命周期结束或用户显式清除/reload。

规则：

- 不按 session 数淘汰内存中的 Finished Store / Live Turn Buffer / Finished Cursor。
- 收到已知项目的 `session.message` 后，无论该 session 是否 selected，都完整消费消息、维护 raw source store、触发 gap Read Repair、并写 Durable Cache。
- 如果收到未知 session 的 `session.message`，客户端先触发该 project 的 session list refresh，同时仍将该消息写入内存 runtime store，避免丢失 realtime turn。
- selected session 在完整同步之外，额外维护 Display Index、virtualized view、Tail Lock、selected prompt_done markRead。
- 重连后需要补读的 session 从内存 runtime store keys 推导，并额外加入当前 selected session。
- Dirty finished prefix 仍通过 5 秒 debounce 写 Durable Cache；不再存在容量淘汰前 flush。

### 7. Read 触发与 Cursor

Read cursor 永远是该 session 的 Finished Cursor：

```text
afterTurnIndex = Finished Cursor
```

不能使用：

- Live Turn Buffer 最大 turnIndex。
- virtualized view 的 latest visible item。
- Session Summary.latestTurnIndex。
- Background hint。

触发时机：

1. **首次进入内存 runtime store**：hydrate local durable cache 后，按可见恢复/选择流程调用 `session.read(after=Finished Cursor)`。
2. **内存 session 实时 gap**：如果收到 `incoming.turnIndex > Finished Cursor + 1`，触发 read。
3. **重连后**：对内存 runtime store keys 中的 session 执行 `session.read(after=Finished Cursor)`，并额外包含当前 selected session。
4. **显式刷新 / reload**：普通 refresh 用当前 Finished Cursor；reload 清 cursor 后从 0 read。

例子：

```text
cursor = 10
receive turn 11 finished:false
```

不 read，因为这是连续 live tail。

```text
cursor = 10
receive turn 12 finished:false
```

触发 read，因为 turn 11 的 seal 或内容可能漏收。

```text
cursor = 10
live = 11 finished:false
receive turn 12 finished:true
```

仍触发 read，因为 live 11 不能推进 cursor，服务端按协议应先发布 11 finished seal。

### 8. Read Repair

同一 session 只允许一个 read in flight。

规则：

- read in flight 时再次触发 gap，只设置 Repair Pending Flag。
- read 完成后，如果 pending=true，按当前 store 重新检查是否仍有 gap；需要时再发一次 read。
- 不按触发次数排队。

应用 read response：

- response 必须连续。
- response 最多一个 `finished:false`，且只能是最后一个 turn。
- read 覆盖范围是 `[afterTurnIndex + 1, responseLastTurnIndex]`。
- 覆盖范围内先删除旧 finished/live raw turns，再用 response turns 重建。
- 覆盖范围外保留，随后用 gap check 判断是否继续 read。
- 如果 response 为空且 `latestTurnIndex <= afterTurnIndex`，接受为空。
- 如果 response 为空但 `latestTurnIndex > afterTurnIndex`，丢弃 response，不推进 cursor。
- 如果 response 中间出现 `finished:false` 后还有更大 turn，视为协议异常，不推进异常后的 turns；保留旧 store 并等待后续 read。
- 如果 `latestTurnIndex < Finished Cursor`，本地 cache stale，清本地该 session cache 后从 0 read。

### 9. Durable Turn Cache

#### 9.1 IndexedDB 格式

第一阶段保持 session content blob，不做 chunk：

```text
wm_chat_session_index
  k
  projectId
  sessionId
  sessionJson
  cursorJson
  updatedAt

wm_chat_session_content
  k
  projectId
  sessionId
  turnsJson
  updatedAt
```

`turnsJson` 保存 raw turns：

```json
[
  {
    "turnIndex": 1,
    "content": "{\"method\":\"prompt_request\",\"param\":{\"contentBlocks\":[]}}",
    "finished": true
  }
]
```

规则：

- `turnsJson` 只保存 `1..Finished Cursor` 的连续 finished prefix。
- `cursorJson.turnIndex` 是 Durable Cache 的 prefix cursor。
- hydrate 时校验 `cursorJson` 和 `turnsJson`：
  - 计算 `turnsJson` 实际连续 finished prefix。
  - 最终 cursor 取 `min(cursorJson.turnIndex, actualPrefix)`.
  - 丢弃 cursor 之后的 durable turns。
  - 修正 IndexedDB 为一致状态。
- DB schema 不兼容旧 `messagesJson` decoded message 格式；检测到旧版本或旧格式时，不做表级迁移，除 token / 认证凭据外删除本地持久缓存并重建 IndexedDB 表。

#### 9.2 Persist

内存 runtime store 中的 finished cursor 更新后：

- 内存 Finished Store / Finished Cursor 立即更新。
- UI 立即更新。
- IndexedDB 延迟 5 秒 debounce persist。

必须 flush：

- 页面 hidden / beforeunload。
- reload / archive / delete 前。
- selected session 收到 `prompt_done` 后。

flush 失败：

- 不阻止 UI。
- 下次该 session 进入 read/repair 流程时用旧 cursor 调 server read 修复。

### 10. Session 状态同步

Session list 状态只由服务端 Session Summary 更新：

- `session.list`
- `session.updated`
- `session.read` response 的 `session`
- `session.markRead` response 的 `session`

`session.message` 不直接更新：

- title
- preview
- running
- lastDoneTurnIndex
- lastDoneSuccess
- lastReadTurnIndex
- unread/completed flags

状态规则：

- `running === true`：显示进行中。
- `running !== true && lastDoneTurnIndex > lastReadTurnIndex && lastDoneSuccess === false`：失败未查看。
- `running !== true && lastDoneTurnIndex > lastReadTurnIndex`：完成未查看。
- 其他：idle。

`prompt_done`：

- 只有 selected session decode 出 `prompt_done` 后调用 `session.markRead(promptDoneTurn.turnIndex)`。
- 非 selected session 即使收到并写入内存，也不 markRead。

### 11. Display Index 与虚拟列表策略

第一阶段 full selected session raw turns 已在内存，滚动策略只控制 React/DOM 渲染和可见 projection。不能为了滚动条或高度计算提前 decode、markdown render 或挂载全量 turns。

本迭代使用 `react-virtuoso` 封装 `ChatVirtuosoTurnList`。不再维护手写 raw turn range 作为主要滚动状态；raw `turnIndex` 仍作为 Display Item metadata、copy range、gap/cursor 判断和 scroll-to-item 定位边界。

#### 11.1 Source、Display Index 与 View

```text
Source turns =
  merge(Finished Store raw turns, Live Turn Buffer raw turns)
  -> same index finished wins
  -> sorted by turnIndex

Display Index =
  source raw turns
  -> shallow parse content envelope only
  -> map each renderable unit to DisplayItem
  -> apply hide/show filters
  -> keep lightweight item metadata and height estimates

Virtualized Chat View =
  Display Index
  -> react-virtuoso computes visible + overscan items
  -> decode/render only mounted items
  -> measure mounted item heights
```

`Display Index` 是显示索引，不是第三份消息数据。它只保存轻量元数据，不保存完整 decoded message，也不保存 markdown render 结果。

```ts
type ChatDisplayItem = {
  key: string;
  turnIndex: number;
  kind:
    | "text"
    | "tool"
    | "thought"
    | "plan"
    | "prompt_request"
    | "prompt_done"
    | "gap"
    | "system";
  affordance?: "option_replies" | "confirmation_reply";
  finished: boolean;
  contentRevision: string;
  estimatedHeight: number;
  measuredHeight?: number;
};
```

约束：

- Source stores 仍只保存 raw turns。
- React/DOM 只挂载 Virtuoso 当前 visible + overscan 的 items。
- `Display Index` 覆盖 selected session 的完整 source turns，但每个 item 必须轻量。
- `content` 全量 JSON parse 只允许用于 shallow envelope：识别 `method`、`param.type`、`status`、是否 copyable、是否隐藏等。
- Markdown、代码块、复杂组件 props、复制文本等重 decode 只能发生在 visible + overscan items。

#### 11.2 DisplayItem 与 raw turnIndex

- 不维护手写 `ChatTurnWindow` 作为 React 渲染窗口。
- `turnIndex` 仍是每个 `ChatDisplayItem` 的 raw 坐标。
- `turnIndex` 不参与 virtualizer 的滚动高度估算；高度由 item estimate/measure 决定。
- 隐藏 tool/thought 不进入 `Display Index`，因此不贡献可见滚动高度。
- `session/gap` 必须进入 `Display Index`，并以 `kind: "gap"` 渲染为不可恢复占位。
- `prompt_request` / `prompt_done` / tool / plan / gap 等协议类型由 `ChatDisplayItem.kind` 决定，不能在 React 组件里临时猜测。
- A/B/C 选项和中文确认不是独立 Display Item；它们是最新 eligible `kind: "text"` item 的 `affordance`，由 shallow/parser 层或 render selector 标记，仍由同一个 text item 渲染。

如果现有协议在 `content.param` 或 assistant text 中表达 ABC、确认按钮、计划状态、tool 状态等特殊 UI，shallow parser / render selector 必须投影成稳定的 `kind` 或 `affordance`。Virtualizer 重挂组件时，展开、确认、选择、复制、输入态等 UI 状态不能只存在 DOM 组件本地，必须按 `sessionId + item.key` 存在 session UI store 中，或者完全由 raw turn / Session Summary 推导。

#### 11.3 Scrollbar 与高度估算

右侧 scrollbar 必须由 `react-virtuoso` 基于 `Display Index` 的逻辑总高度维护，而不是由已挂载 DOM 节点总高度自然决定。

`ChatVirtuosoTurnList` 封装边界：

```ts
type ChatVirtuosoTurnListProps = {
  scrollRef: React.RefObject<HTMLElement | null>;
  displayIndex: ChatDisplayIndex;
  runtimeKey: string;
  renderItem: (item: ChatDisplayIndexItem) => React.ReactNode;
  onAtBottomChange: (atBottom: boolean) => void;
  shouldAutoscroll: () => boolean;
};
```

实现约束：

- 使用 `Virtuoso` 组件，不保留手写 range、padding spacer 或旧虚拟列表代码。
- `data = displayIndex.items`。
- `computeItemKey = item.key`。
- `defaultItemHeight` / `heightEstimates` 由 `DisplayIndexItem.estimatedHeight` 提供给 Virtuoso。
- `customScrollParent` 指向 chat scroll container。
- `atBottomStateChange`、`totalListHeightChanged`、`scrollToIndex({index: "LAST"})` 和 `autoscrollToBottom()` 都封装在 wrapper 内。
- `main.tsx` 不直接操作 Virtuoso API；只传 `Display Index`、renderer 和滚动意图。

高度策略：

- 未渲染 item 使用 `estimatedHeight`。
- visible + overscan item 挂载后的真实 DOM 高度由 Virtuoso 内部测量与缓存。
- App 不维护第二套高度缓存或手写 range height cache。
- Virtuoso 使用内部测量值和传入估算值计算逻辑总高度和 scrollbar thumb。
- 流式 tail item 的高度更新用 `requestAnimationFrame` 或节流合并，避免每个 chunk 都触发布局抖动。
- 禁止为了获得精确 scrollbar 而提前 decode/render 全量 turns。

估算高度不要求绝对精确。远距离拖动 scrollbar 时，未测量区域可能在进入视口后被校准；校准必须通过 anchor restore 保持当前可见内容不跳。

#### 11.4 Anchor 与 Tail Lock 不变量

滚动正确性依赖两个互斥模式：

- **Viewport Anchor**：用户不在底部时，window 扩展、裁剪、未测量高度修正、流式内容变高，都必须保持当前锚点 item 在 viewport 内的位置不变。
- **Tail Lock**：用户在底部时，latest item 变化后必须保持滚动到底部。

Anchor 记录：

```ts
type ScrollAnchor = {
  itemKey: string;
  turnIndex: number;
  offsetFromViewportTop: number;
};
```

更新流程：

1. 在修改 display index、Virtuoso measurement state 或 tail 内容前记录 anchor。
2. 应用数据变化并让 Virtuoso 完成布局。
3. 如果处于 Tail Lock，滚动到底部。
4. 否则恢复 anchor 的 viewport offset。

#### 11.5 初始定位

打开 selected session 或切换到已在内存中的 session：

1. 计算 latest source turn index。
2. 构建完整轻量 `Display Index`。
3. `ChatVirtuosoTurnList` 初始定位到最后一个 `DisplayItem`，`align: "end"`。
4. 初始状态进入 Tail Lock。

默认参数：

- Overscan：通过 `increaseViewportBy` / `minOverscanItemCount` 控制，按接近一屏到两屏内容调优。
- Tail bottom threshold：由 `atBottomThreshold` 控制，默认 80px。
- Estimate defaults 由 `ChatDisplayItem.kind` 决定，长文本可用 raw `content.length` 粗估，但不得 markdown render。

#### 11.6 上滑

当用户向上滚动：

1. 不移动手写 raw turn range。
2. `react-virtuoso` 根据 scroll offset 计算 visible + overscan items。
3. mounted item 才 decode/render/measure。
4. 不触发 server read，因为该 session 的 source turns 已全量在内存。
5. 不裁剪 `Display Index`；DOM bounded 由 Virtuoso 保证。

#### 11.7 下滑

当用户向下滚动：

1. 不移动手写 raw turn range。
2. `react-virtuoso` 根据 scroll offset 计算 visible + overscan items。
3. mounted item 才 decode/render/measure。
4. 如果 scroll container 到达底部阈值，进入 Tail Lock。
5. 不触发 server read。

#### 11.8 新 turn 与流式增长

如果 selected session 在 Tail Lock：

- 新 turn 进入 source store 后，增量更新 `Display Index`。
- Virtuoso 滚动到最后一个 item，保持底部。

如果 selected session 不在 Tail Lock：

- source store 更新。
- `Display Index` 增量更新。
- 当前 viewport anchor 保持稳定。
- 显示 scroll-to-bottom affordance。
- 用户点击或下滑到 latest 后恢复 Tail Lock。

流式 chunk 更新同一 tail turn 时：

- `contentRevision` 更新。
- 如果该 item 已挂载，重新渲染并测量。
- 如果该 item 未挂载，只更新 source 与 item revision，不提前 render。

#### 11.9 Copy

`prompt_done` copy 使用该 session 的 full source store，不受当前 window 限制：

1. 从 `prompt_done.turnIndex` 向前找最近 `prompt_request`。
2. 要求 raw range 在 Finished Store 中连续。
3. range 含 `session/gap` 时禁用 copy。
4. 只复制 copyable decoded message，例如 `agent_message_chunk`。

### 12. 落地代码结构迭代

这次不应继续把同步、缓存、状态和显示窗口逻辑堆在 `main.tsx`。落地时优先做结构拆分，再填充行为：

- `registry.ts`：只定义 wire types，包括 `RegistrySessionTurn`、`session.message` payload、`session.read` response、Session Summary。
- `chatWire.ts`：做 raw turn shape 校验和旧 payload 拒绝，不 decode 业务内容。
- `chatTurnStores.ts`：维护 Finished Store、Live Turn Buffer、Finished Cursor、read response reconcile、gap repair 判定。
- `main.tsx` / 后续可抽出的 runtime store 模块：维护无容量上限的内存 turn stores，并基于 store keys 处理重连后 read trigger。
- `workspacePersistence.ts` / `workspaceStore.ts`：只负责 raw `turnsJson`、`cursorJson`、schema/version 检查、除 token 外的全量本地缓存清理、重建表、5 秒 debounce persist 和 flush。
- `chatDisplayIndex.ts`：从 raw source store 派生轻量 Display Index，只做浅分类和 metadata，不保存完整 decoded message。
- `ChatVirtuosoTurnList.tsx`：封装 `react-virtuoso`，拥有 visible + overscan、内部高度测量、tail lock、scroll-to-bottom；`main.tsx` 不直接操作 Virtuoso API。
- `main.tsx`：保留项目/session 选择、事件分发和 React 状态编排，不再直接实现 read repair、IndexedDB raw turn 合并、显示索引和滚动窗口。

服务端侧也需要收敛边界：

- `session_recorder.go` 负责 raw turn shape、unfinished tail invariant、prompt_done publish order。
- `session_turn_files.go` 负责 WMT2 连续读取和 read-time `session/gap` projection。
- `client.go` / hub request handler 负责 `session.read` response envelope 和 Session Summary 返回。

旧 `chatTurnWindow.ts` 作为主滚动/显示状态应删除或退役；如果短期保留，只能作为迁移中的测试对照，不参与运行时路径。

### 13. 大内容风险

第一阶段不改变 WMT2 turn 正文格式，不做 DB turn 精简。

后续需要单独设计：

- image content block 不应长期以内联 base64 存在 turn 正文中。
- 超大 tool output / thought / plan 应有 inline size limit。
- 超限内容应转 artifact/ref，UI 显示摘要和展开入口。
