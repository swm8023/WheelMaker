# Session Finished Cursor 与文件化历史设计

状态：设计草案，作为后续补充和执行的主设计文档  
日期：2026-05-13  
范围：app chat session 同步协议、服务端 session history 持久化、前端重连/cache 语义

相关文档：

- [app-chat-recorder-sync-protocol.zh-CN.md](./app-chat-recorder-sync-protocol.zh-CN.md)
- [session-persistence-sqlite.md](./session-persistence-sqlite.md)
- [session-recorder-record-event.md](./session-recorder-record-event.md)
- [superpowers spec](./superpowers/specs/2026-05-13-session-finished-cursor-file-history-design.md)
- [superpowers implementation plan](./superpowers/plans/2026-05-13-session-finished-cursor-file-history.md)

## 1. 目标

将 session prompt 历史从 SQLite `session_prompts.turns_json` 迁移到文件，同时让断线重连不需要频繁读取 prompt 文件。

核心目标：

- `sessions` 表继续作为热索引。
- prompt 正文历史从 `session_prompts` 迁移到文件。
- 启动时执行一次全量迁移，迁移后 runtime 不再读写 `session_prompts`。
- `sessions` 增加一个小型同步投影字段，供 `session.list` 和重连判断使用。
- 所有 turn 使用统一的 `finished` 字段。
- `prompt_done` 作为真实 turn 存入历史。
- 前端只缓存 `finished: true` 的 turn。
- 前端重连时只上报自己已缓存的最新 finished cursor。
- `session.read(P, T)` 返回 finished cursor 之后的增量，而不是返回完整 prompt snapshot。
- App 发送 prompt 时不本地写入 `prompt_request`，只接受服务端回放的权威 turn。

非目标：

- 不把 `sessions`、`route_bindings`、`projects`、`agent_preferences` 文件化。
- 不逐 turn 重写 prompt 文件。
- 第一阶段不引入 JSONL/WAL。
- 不保证断线后恢复 tool/plan 的所有中间状态。
- 不立即删除旧 `session_prompts` 表。

## 2. 核心决策

### 2.1 `sessions` 保留为热索引

`sessions` 继续保存：

- session id
- project name
- agent type/json
- title
- created/last active time
- status

新增一个 JSON 字段：

```sql
session_sync_json TEXT NOT NULL DEFAULT '{}'
```

字段值只保存最小同步投影：

```json
{
  "promptIndex": 12,
  "turnIndex": 5,
  "finished": true
}
```

语义：

- `promptIndex`：服务端当前最新 prompt。
- `turnIndex`：该 prompt 内服务端当前最新 turn。
- `finished`：该最新 turn 是否可以作为 cache/retransmission cursor。

这个字段不保存正文，不保存文件路径。prompt 文件路径从 `projectName + sessionId + promptIndex` 推导。

### 2.2 prompt 历史按文件存

文件布局：

```text
session/
  <project-key>/
    <session-id>/
      manifest.json
      prompts/
        p000001.json
        p000002.json
```

一个 prompt 一个 snapshot 文件。不要一个 session 一个大文件。

原因：

- session 会 append 新 prompt，单大文件会重写历史。
- `session.read(sessionId, promptIndex, 0)` 是 prompt-local repair，单 prompt 文件天然匹配。
- 文件损坏时影响范围更小。
- prompt 完成时只序列化当前 prompt 一次。

### 2.3 文件只保存完成后的 prompt snapshot

运行中 prompt 的 source of truth 仍然是内存 `sessionPromptState`。

写入时机：

1. streaming 期间只更新内存。
2. `prompt_done` 到达时封口未完成 text/thought turn。
3. 将 `prompt_done` 加入 turn 列表。
4. 将完整 prompt snapshot 写到临时文件。
5. 原子 rename 到正式 prompt 文件。
6. 更新 `session_sync_json`。
7. 发布 `prompt_done` realtime turn。

要求：`prompt_done` 必须在 prompt 文件写成功后发布。

这样前端收到 `prompt_done` 后如果立即补读，服务端已经能从文件返回完整结果。

### 2.4 prompt request 的权威来源

App 发送用户输入时只清空 composer 并调用 `session.send`。本地不乐观插入、不持久化 `prompt_request`。

服务端收到请求后生成 `prompt_request finished=true` turn，通过 `session.message` 下发。客户端如果漏收，后续 reconnect 或 `session.read(P, T)` 会补回同一个权威 turn。这样可以避免本地乐观消息和服务端消息重复。

## 3. Turn Envelope 协议

所有 app chat message 都使用统一 envelope：

```typescript
interface RegistrySessionMessage {
  sessionId: string;
  promptIndex: number;
  turnIndex: number;
  method: string;
  param: Record<string, unknown>;
  finished: boolean;
}
```

服务端 wire payload 仍然保留 `content` 字符串：

```json
{
  "sessionId": "sess-1",
  "promptIndex": 12,
  "turnIndex": 5,
  "finished": true,
  "content": "{\"method\":\"prompt_done\",\"param\":{\"stopReason\":\"end_turn\"}}"
}
```

前端 decode 后得到 `method` 和 `param`。

### 3.1 `finished` 语义

`finished` 表示：这个 turn 可以进入客户端 cache，并可作为重传 cursor。

它不是严格的“永不变化”承诺。对 tool/plan，后续可能还有同 turn 覆盖更新，但断线后不保证恢复这些中间状态。

规则：

| method | `finished` |
| --- | --- |
| `prompt_request` | `true` |
| `user_message_chunk` | `true` |
| `tool_call` | `true` |
| `plan` | `true` |
| `system` | `true` |
| `agent_message_chunk` | streaming 时 `false`，封口后 `true` |
| `agent_thought_chunk` | streaming 时 `false`，封口后 `true` |
| `prompt_done` | `true` |

取舍：

- tool/plan 中间状态主要用于 UI，即时显示即可。
- text/thought 内容是主要回复内容，未封口前不能进 cache。
- prompt 完成后，最终 prompt snapshot 会包含最终可恢复内容。

### 3.2 `prompt_done` 是真实 turn

`prompt_done` 不再只是 realtime terminal marker，而是 prompt 内最后一个真实 turn。

示例：

```text
turn 1: prompt_request       finished=true
turn 2: agent_thought_chunk  finished=true
turn 3: tool_call            finished=true
turn 4: agent_message_chunk  finished=true
turn 5: prompt_done          finished=true
```

这样 cursor 规则变简单：

- `prompt_done` 推进 cursor。
- `session.read(P, T)` 只需要返回 cursor 之后的 turns。
- 不再需要 `prompt_done.turnIndex - 1` watermark。
- 不再需要“prompt_done 不推进读取游标”的特殊规则。

### 3.3 Prompt terminal invariant

服务端必须保证：只要一个 prompt 已经产生任何 turn，它最终就有一个 terminal turn。terminal turn 统一使用 `prompt_done finished=true` 表示，`stopReason` 区分来源：

- `end_turn`：模型正常结束。
- `cancelled`：用户取消。
- `error`：服务端或 agent 返回错误。
- `interrupted`：服务端准备进入下一个 prompt 时发现上一个 prompt 尚未 terminal。
- `server_restart`：服务端启动恢复时发现历史中最后一个 prompt 非 terminal。
- `reload_recovered`：reload/resume 过程中恢复了未闭合状态。

因此服务端如果开始处理 prompt 2，必须先保证 prompt 1 已经 terminal。客户端如果只收到 prompt 2 的 turn，却没有收到 prompt 1 的 `prompt_done`，调用 `session.read(p1, t)` 必须能读回这个合成 terminal turn。

## 4. `session.read` 协议

请求：

```json
{
  "sessionId": "sess-1",
  "promptIndex": 12,
  "turnIndex": 3
}
```

语义：客户端已缓存 finished turns 到 `(promptIndex=12, turnIndex=3)`。

`session.read(P, T)` 是 finished cursor 之后的增量补读，不是“读取 prompt P 的完整快照”。只有 `(0, 0)` 或 `(P, 0)` 这类 cursor 自然覆盖完整范围时，响应才会包含完整 prompt 内容。

响应：

```json
{
  "session": {
    "sessionId": "sess-1",
    "title": "Fix sync bug",
    "updatedAt": "2026-05-13T10:01:00Z",
    "agentType": "codex",
    "sync": {
      "promptIndex": 12,
      "turnIndex": 5,
      "finished": true
    }
  },
  "prompts": [
    {
      "sessionId": "sess-1",
      "promptIndex": 12,
      "turnIndex": 5,
      "modelName": "gpt-5",
      "durationMs": 5000,
      "finished": true
    }
  ],
  "messages": [
    {
      "sessionId": "sess-1",
      "promptIndex": 12,
      "turnIndex": 4,
      "finished": true,
      "content": "{\"method\":\"agent_message_chunk\",\"param\":{\"text\":\"...\"}}"
    },
    {
      "sessionId": "sess-1",
      "promptIndex": 12,
      "turnIndex": 5,
      "finished": true,
      "content": "{\"method\":\"prompt_done\",\"param\":{\"stopReason\":\"end_turn\"}}"
    }
  ]
}
```

读取规则：

- `(0, 0)`：返回全量已持久化 prompts，加上 active prompt tail。
- `(P, 0)`：返回 prompt `P` 全部 turns，加上后续 prompts。
- `(P, T)`：返回 prompt `P` 中 `turnIndex > T` 的 turns，加上后续 prompts 的全部 turns。
- finished prompt 从 prompt 文件读取。
- active prompt 从内存读取。
- 返回的 `finished: false` turns 只用于 UI，不进入客户端 cache。

## 5. 前端 cache 与重连

前端维护两层状态：

1. 内存 UI state：包含所有收到的 turn，包括 `finished: false`。
2. IndexedDB cache：只保存 `finished: true` 的 turn。

本地 cursor：

```json
{
  "promptIndex": 12,
  "turnIndex": 5
}
```

这个 cursor 表示“本地已经缓存到的最新 finished turn”。

实时消息处理：

1. decode `session.message`。
2. 按 `sessionId:promptIndex:turnIndex` upsert 到内存 UI。
3. 如果 `finished: true`，写入 IndexedDB 并推进 cursor。
4. 如果 `finished: false`，只显示，不缓存，不推进 cursor。

实时缺口判断：

假设本地 finished cursor 是 `(P, T)`，收到 turn `(Pi, Ti)`：

- `Pi > P + 1`：存在 prompt 缺口，请求 `session.read(sessionId, P, T)`。
- `Pi == P + 1` 且本地 prompt P 没有 terminal turn：前一个 prompt 的 terminal 可能漏收，请求 `session.read(sessionId, P, T)`。
- `Pi == P + 1` 且 `Ti != 1`：新 prompt 起始 turn 漏收，请求 `session.read(sessionId, P, T)`。
- `Pi == P` 且 `Ti > T + 1`：当前 prompt 内有 turn 缺口，请求 `session.read(sessionId, P, T)`。
- `Pi == P + 1` 且本地 prompt P 已 terminal 且 `Ti == 1`：可直接接受。

prompt 是否 terminal 只由 `prompt_done` 判断。客户端漏收 terminal 后，即使后续先收到了下一个 prompt，也应先补读再合并。

重连：

1. 调用 `session.list`。
2. 从 session summary 的 `sync` 投影得到服务端最新 `(promptIndex, turnIndex, finished)`。
3. 比较服务端 sync 和本地 finished cursor。
4. 如果服务端领先，调用 `session.read`。
5. 如果服务端最新 turn 是 `finished: false`，也调用 `session.read`，用于恢复 active tail。
6. 读取结果按实时消息同样规则处理。

## 6. 启动迁移

迁移策略：启动时一次性全量迁移。

流程：

1. 检查全局 store metadata 是否已经是 `prompt_files_v1`。
2. 如果已完成，跳过。
3. 扫描 `sessions`。
4. 对每个 session 读取旧 `session_prompts`。
5. 对每个 prompt decode `turns_json`。
6. 每个旧 turn 转为 `finished: true` 的 prompt file turn。
7. 如果旧 prompt 有 `stop_reason`，追加一个 `prompt_done` turn。
8. 写 `prompts/pNNNNNN.json.tmp`。
9. rename 到正式 prompt 文件。
10. 写 session `manifest.json`。
11. 更新 `sessions.session_sync_json`。
12. 全部成功后写全局 metadata：`prompt_files_v1`。

失败处理：

- 失败时不删除旧 `session_prompts`。
- 失败时不写全局完成标记。
- 下次启动可以重试。
- 迁移完成后 runtime 不应静默 fallback 到旧 `session_prompts`，避免读到 stale history。

## 7. Prompt 文件格式

示例：

```json
{
  "schemaVersion": 1,
  "sessionId": "sess-1",
  "promptIndex": 2,
  "title": "Fix sync bug",
  "modelName": "gpt-5",
  "startedAt": "2026-05-13T10:00:00Z",
  "updatedAt": "2026-05-13T10:01:00Z",
  "stopReason": "end_turn",
  "turnIndex": 4,
  "turns": [
    {
      "turnIndex": 1,
      "method": "prompt_request",
      "finished": true,
      "content": "{\"method\":\"prompt_request\",\"param\":{\"contentBlocks\":[{\"type\":\"text\",\"text\":\"hello\"}]}}"
    },
    {
      "turnIndex": 2,
      "method": "tool_call",
      "finished": true,
      "content": "{\"method\":\"tool_call\",\"param\":{\"cmd\":\"go test\",\"status\":\"running\"}}"
    },
    {
      "turnIndex": 3,
      "method": "agent_message_chunk",
      "finished": true,
      "content": "{\"method\":\"agent_message_chunk\",\"param\":{\"text\":\"Done\"}}"
    },
    {
      "turnIndex": 4,
      "method": "prompt_done",
      "finished": true,
      "content": "{\"method\":\"prompt_done\",\"param\":{\"stopReason\":\"end_turn\"}}"
    }
  ]
}
```

备注：

- `content` 继续使用 `IMTurnMessage` JSON 字符串，复用现有 decode 逻辑。
- `turnIndex` 应等于 `turns` 中最大 turn index。
- `prompt_done` 必须是完成 prompt 的最后一个 turn。

## 8. Module / Seam

目标 Module：

- `SessionRecorder`：负责 prompt/turn 生命周期、merge、finished 语义、publish 顺序。
- `SQLite Store`：负责 session index、route binding、agent preference、session sync projection。
- `Session History Store`：负责 prompt 文件读写、文件路径、manifest、迁移。
- `App Chat Sync`：负责 finished cursor、cache、重连读取。

重要 Seam：

- `SessionRecorder` 不应该散落文件路径逻辑。
- prompt 文件格式和迁移逻辑集中在 Session History Store Adapter。
- app/web 不应该理解文件存储，只理解 `session.list` / `session.read` 协议。

## 9. 实施顺序

建议按以下顺序执行：

1. 服务端协议测试：`finished` 替代 `done`，`prompt_done` 作为真实 turn。
2. SQLite 增加 `session_sync_json`。
3. 新增 Session History Store 文件 Adapter。
4. 实现启动迁移。
5. 切换 prompt finish 写文件、`session.read` 从文件/内存读取。
6. app/web 改为 finished cursor 和 cache-only-finished。
7. 更新正式协议文档。
8. 完整测试、build、发布。

详细任务见：

- [superpowers implementation plan](./superpowers/plans/2026-05-13-session-finished-cursor-file-history.md)

## 10. 待补充问题

后续设计补充优先在本节增补。

### 10.1 全局迁移 metadata 放在哪里

待确认：

- 复用现有 SQLite schema metadata 机制。
- 新增 store meta 表。
- 写入 `sessions` 之外的单独 metadata 文件。

倾向：SQLite store meta。因为迁移状态是 DB 到文件的全局状态，放在 SQLite 内更容易与旧数据源保持一致。

### 10.2 session 根目录

待确认默认路径：

```text
~/.wheelmaker/session/
```

需要确认是否允许通过 config 覆盖。

### 10.3 project-key 生成规则

待确认：

- 是否直接使用 sanitized project name。
- 是否使用 hash(projectName) 防止路径过长和泄露绝对路径。

倾向：hash + 可读短前缀。

### 10.4 tool/plan 最终态策略

当前决策：tool/plan `finished=true`，中间态可丢。

如果后续需要更严格恢复，有两个方向：

- tool/plan 只有 terminal status 才 `finished=true`。
- tool/plan 每次状态变化分配新 turn。

第一版不采用。

### 10.5 active prompt 崩溃恢复

第一版不做 WAL/JSONL。

如果要支持服务端崩溃后恢复半截 prompt，可加：

```text
active/p000123.jsonl
```

但这会增加 IO 和 replay 复杂度，应独立设计。
