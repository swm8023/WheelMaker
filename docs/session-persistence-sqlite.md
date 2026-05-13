# Session 持久化（SQLite）草案

更新时间：2026-04-03  
状态：Draft

## 1. 范围

本方案 **只做 WheelMaker Session 持久化**，不包含其他状态域。

包含：

- Session 基本信息（WheelMaker SessionID）
- `route -> session` 绑定
- `session -> instance` 绑定（共享/独享）
- `session -> acpSessionId` 映射

不包含（本阶段）：

- ActiveAgent 全局状态
- Agent initialize 元数据（capabilities/info/auth）
- 其他非 Session 域状态

## 2. 存储位置

建议按项目独立数据库：

- `~/.wheelmaker/projects/<project>/session.db`

这样表结构无需 `project_name` 复合主键，读写路径也更清晰。

当前实现使用统一数据库与文件历史组合：

- SQLite：`~/.wheelmaker/db/client.sqlite3`
- prompt 文件历史：`~/.wheelmaker/session-history/`

SQLite 保留热索引、会话元数据、路由绑定与迁移兼容表。完整 prompt turn 正文迁移到文件：

```text
session-history/<project-key>/<session-id>/manifest.json
session-history/<project-key>/<session-id>/prompts/p000001.json
```

`session_prompts.turns_json` 作为旧数据迁移来源保留；启动时全量导出到 prompt 文件，运行期 finished prompt 正文从文件读取，active prompt turns 从内存读取。

## 3. 表结构（DDL）

```sql
PRAGMA foreign_keys = ON;
PRAGMA journal_mode = WAL;
PRAGMA synchronous = NORMAL;

CREATE TABLE IF NOT EXISTS wm_sessions (
  id              TEXT PRIMARY KEY,             -- WheelMaker SessionID
  route_id        TEXT NOT NULL,
  agent_type      TEXT NOT NULL,
  acp_session_id  TEXT,                         -- nullable until ready
  instance_mode   TEXT NOT NULL,                -- shared | dedicated
  created_at      TEXT NOT NULL DEFAULT (datetime('now')),
  updated_at      TEXT NOT NULL DEFAULT (datetime('now')),
  CHECK(instance_mode IN ('shared', 'dedicated'))
);

CREATE TABLE IF NOT EXISTS wm_route_bindings (
  route_id        TEXT PRIMARY KEY,
  session_id      TEXT NOT NULL,
  created_at      TEXT NOT NULL DEFAULT (datetime('now')),
  updated_at      TEXT NOT NULL DEFAULT (datetime('now')),
  FOREIGN KEY (session_id) REFERENCES wm_sessions(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS wm_instance_bindings (
  session_id       TEXT PRIMARY KEY,
  instance_key     TEXT NOT NULL,               -- shared key or dedicated key
  agent_type       TEXT NOT NULL,
  mode             TEXT NOT NULL,               -- shared | dedicated
  created_at       TEXT NOT NULL DEFAULT (datetime('now')),
  updated_at       TEXT NOT NULL DEFAULT (datetime('now')),
  FOREIGN KEY (session_id) REFERENCES wm_sessions(id) ON DELETE CASCADE,
  CHECK(mode IN ('shared', 'dedicated'))
);
```

当前统一 SQLite schema 的 `sessions` 表包含断线重连投影：

```sql
sessions.session_sync_json TEXT NOT NULL DEFAULT '{}'
```

`session_sync_json` 保存最新可重传 finished cursor 的 JSON，例如：

```json
{"promptIndex":12,"turnIndex":5,"finished":true}
```

客户端以自己的 finished cursor 调用 `session.read(P, T)`；服务端只返回该 cursor 之后的增量。

## 4. 索引与约束

```sql
-- 同一 ACP Session 只能映射到一个 WheelMaker Session（空值除外）
CREATE UNIQUE INDEX IF NOT EXISTS idx_wm_sessions_acp_sid
  ON wm_sessions(acp_session_id)
  WHERE acp_session_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_wm_sessions_route
  ON wm_sessions(route_id);

CREATE INDEX IF NOT EXISTS idx_wm_sessions_agent
  ON wm_sessions(agent_type);

CREATE INDEX IF NOT EXISTS idx_wm_instance_bindings_instance_key
  ON wm_instance_bindings(instance_key);
```

## 5. 写入模型（事务）

凡是改变路由/会话/实例关系的操作，必须单事务提交：

1. upsert `wm_sessions`
2. upsert `wm_route_bindings`
3. upsert `wm_instance_bindings`
4. commit

这样可避免崩溃后出现“路由指向不存在 Session”或“Session 无实例绑定”的半状态。

## 6. 最小查询模型

- 路由入站：`route_id -> session_id`（`wm_route_bindings`）
- ACP 回调分发：`acp_session_id -> session_id`（`wm_sessions`）
- Session 执行：`session_id -> instance_key/mode`（`wm_instance_bindings`）

## 7. Store 接口建议（Phase 1）

保持现有 `Store` 不动，新增 Session 域存储接口：

```go
type SessionStore interface {
  SaveSession(s SessionRecord) error
  BindRoute(routeID, sessionID string) error
  BindInstance(sessionID, instanceKey, agentType, mode string) error
  GetByRoute(routeID string) (*SessionRecord, error)
  GetByACPSessionID(acpSessionID string) (*SessionRecord, error)
}
```

## 8. 边界说明

- SQLite 是 Session 域唯一持久化来源。
- 本文不定义任何协议字段变更。
- 当后续扩展到其他状态域时，再单独增表，不回退本模型。
