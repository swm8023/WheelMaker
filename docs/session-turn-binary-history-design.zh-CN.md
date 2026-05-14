# Session Turn 二进制持久化设计

## 目标

会话历史以全局 `turnIndex` 为唯一顺序，不再暴露 `promptIndex`。`prompt_request`、普通 agent/tool/thought 消息、`prompt_done` 都是 turn。客户端断线补读只用最后一个已缓存 finished turn：

```json
{"sessionId":"...","afterTurnIndex":128}
```

服务端响应：

```json
{"latestTurnIndex":132,"turns":[{"sessionId":"...","turnIndex":129,"finished":true,"content":"..."}]}
```

`session.message` 实时事件 payload 只包含：

```json
{"sessionId":"...","turnIndex":129,"finished":false,"content":"..."}
```

## SQLite

`sessions` 是热索引表，字段为：

- `id`
- `project_name`
- `status`
- `agent_type`
- `agent_json`
- `session_sync_json`
- `title`
- `created_at`
- `updated_at`

`session_sync_json` 只保存持久化游标：

```json
{"latestPersistedTurnIndex":384}
```

`session_prompts` 不再属于目标 schema。启动只做 schema 校验和安全字段迁移；旧表删除由一次性迁移工具完成。

## 文件格式

根目录：`~/.wheelmaker/session/<project>/<session>/turns/`

文件名：`t000000.bin`、`t000001.bin`，每个文件最多 128 个 turn。

Header：

- magic: 4 bytes, `WMT2`
- version: uint16 little endian, `1`
- reserved: uint16
- 128 个 slot，每个 slot 为 `{offset uint32, length uint32}`

Body 只存 `content` bytes。`turnIndex` 由文件号和 slot 推导，已持久化 turn 视为 `finished=true`。

写入顺序：

1. 写 body。
2. `fsync` body。
3. Patch header slot。
4. `fsync` header。
5. 更新 `sessions.session_sync_json`。

Header slot 为 0 表示 turn 不存在；孤儿 body 在下次写入前截断。

## 迁移

临时脚本：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/migrate_session_turn_store.ps1
```

迁移行为：

- 检测 `wheelmaker*` 进程，运行中则失败。
- 备份 `client.sqlite3`。
- 将已有 `session/<project>/<session>/prompts/p*.json` 转成 `turns/t*.bin`。
- 注入旧 prompt 文件顶层元数据：
  - `prompt_request.param.modelName`
  - `prompt_request.param.createdAt`
  - `prompt_done.param.completedAt`
- 更新 `session_sync_json`。
- `DROP TABLE session_prompts`。
- `prompts` 重命名为 `prompts-legacy`。

运行期不再从旧 prompt 表读写正文。
