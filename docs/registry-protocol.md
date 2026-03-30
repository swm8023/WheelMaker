# WheelMaker Registry Protocol 2.1

本文定义 WheelMaker Registry 2.1 协议，覆盖 Registry、Hub、Client 的连接、认证、路由、同步与数据传输行为。

## 1. 目标与范围

- 以 `project` 为同步单元，支持多 Hub、多 Project。
- 客户端只刷新可见范围（展开目录、打开/Pin 文件、当前 Git 视图）。
- 文件系统接口支持 hash 协商（conditional GET），减少重复传输。
- 分块读（目录分页、文件区块）引入 `snapshotToken` 保证一致性。
- Git 列表按版本触发刷新，不做 hash 协商。
- 协议仅描述 2.1 语义，不包含旧版本兼容分支。

## 2. 核心模型

### 2.1 标识规则

- Hub 上报主键：`projects[].name`。
- 对客户端暴露：`projectId = hubId + ":" + projectName`。
- 客户端后续查询仅使用 `projectId`。

### 2.2 版本状态

每个 project 维护：

| 字段 | 含义 |
|------|------|
| `projectRev` | 项目聚合版本戳（当前仅由 Git 侧状态派生） |
| `gitRev` | Git 版本戳（由 `branch + headSha + dirty` 归一后哈希） |
| `headSha` | 当前分支 HEAD |
| `dirty` | 是否存在未提交改动 |
| `worktreeRev` | 工作区状态版本戳（由 `git status --porcelain` 归一生成） |

### 2.3 共享 Project 对象（`ProjectObject`）

Hub 上报（`reportProjects`、`updateProject`）和 Client 查询（`project.list`）共用同一结构：

```json
{
  "name": "WheelMaker",
  "path": "D:/Code/WheelMaker",
  "online": true,
  "agent": "codex",
  "imType": "feishu",
  "projectRev": "sha256:...",
  "git": {
    "branch": "main",
    "headSha": "abc123...",
    "dirty": true,
    "gitRev": "sha256:...",
    "worktreeRev": "sha256:..."
  }
}
```

所有使用 project 对象的方法必须包含完整字段，不允许部分省略。Client 侧响应额外附加 `projectId` 和 `hubId`。

## 3. 通用连接与握手认证（Hub/Client 共享）

### 3.1 通用消息封装

```json
{
  "requestId": 1,
  "type": "request|response|error|event",
  "method": "connect.init|...",
  "projectId": "optional, 业务方法必填",
  "payload": {}
}
```

> **变更说明**：2.1 移除了消息体中的 `version` 字段。协议版本通过 `connect.init` 握手中的 `protocolVersion` 唯一协商，消息帧层面不再携带冗余版本号。

### 3.1.1 `requestId` 规则

- `requestId` 类型为正整数（`int`），取值 `>= 1`。
- 每次连接建立后，发送方从 `1` 开始递增（`1,2,3...`）。
- 同一连接内不允许重复使用同一 `requestId`。
- **`type=event` 时不携带 `requestId`**。事件是服务端单向推送，无请求-响应对应关系。
- Registry 校验规则：
  - 非整数或 `<1`：返回 `INVALID_ARGUMENT`。
  - 已处理过的 `requestId`（重放/重复）：返回 `CONFLICT`，丢弃该请求。
- 连接重建后 `requestId` 重新从 `1` 计数。

### 3.2 `connect.init`（握手与认证合并）

请求：

```json
{
  "requestId": 1,
  "type": "request",
  "method": "connect.init",
  "payload": {
    "clientName": "wm-web",
    "clientVersion": "0.1.0",
    "protocolVersion": "2.1",
    "role": "client",
    "hubId": "local-hub",
    "token": "******",
    "ts": 1777777777,
    "nonce": "a1b2c3d4"
  }
}
```

成功响应：

```json
{
  "requestId": 1,
  "type": "response",
  "method": "connect.init",
  "payload": {
    "ok": true,
    "principal": {
      "role": "client",
      "hubId": "local-hub",
      "connectionEpoch": 42
    },
    "serverInfo": {
      "serverVersion": "x.y.z",
      "protocolVersion": "2.1"
    },
    "features": {
      "hubReportProjects": true,
      "pushHint": true,
      "pingPong": true,
      "supportsHashNegotiation": true,
      "supportsBatch": true
    },
    "hashAlgorithms": ["sha256"]
  }
}
```

> **`connectionEpoch`**：由 Registry 分配的全局单调递增整数，在 `connect.init` 响应中返回。Hub 必须在后续 `reportProjects`、`updateProject` 中回传此值。详见 4.2、4.3。

### 3.3 校验与安全约束

- `role` 必填，仅允许 `hub` 或 `client`。
- `role=hub` 时 `hubId` 必填。
- `role=client` 时 `hubId` 可选：
  - 携带 `hubId`：连接绑定到该 hub，`project.list` 仅返回该 hub 的项目。
  - 省略 `hubId`：全局范围，`project.list` 返回 token 有权访问的所有 hub 的项目。
- `token` 必填，必须在 `connect.init` 中携带。
- 所有业务方法必须在 `connect.init.ok=true` 后调用。
- 失败响应统一为 `UNAUTHORIZED`，认证失败后立即断连。
- 建议 `connect.init` 使用 `ts + nonce`，服务端做时间窗校验与 nonce 去重。

### 3.4 方法白名单

| 角色 | 允许的请求方法 |
|------|---------------|
| `hub` | `registry.reportProjects`、`registry.updateProject`、`hub.ping` |
| `client` | `project.list`、`project.syncCheck`、`fs.*`、`git.*`、`agent.*`、`batch`、`subscribe`、`unsubscribe` |

- 方法与角色不匹配返回 `FORBIDDEN`。
- **事件方法**（`project.changed`、`git.workspace.changed`、`project.offline`、`project.online`、`agent.activity`等）是服务端→客户端推送，不受白名单约束。

### 3.5 projectId 反查与路由

Registry 维护：

- `projectToHub[projectId] = hubId`
- `hubPeers[hubId] = peerConn`

规则：

1. Client 请求携带 `projectId`。
2. Registry 从 `projectId` 解析 `hubId`。
3. 与 `projectToHub[projectId]` 做一致性校验。
4. 使用 `hubPeers[hubId]` 转发。

错误：

- `NOT_FOUND`：`projectId` 不存在。
- `UNAVAILABLE`：project 存在但 hub 不在线。
- `FORBIDDEN`：token 无权访问该 hub/project。

### 3.6 标准错误响应结构

所有 `type=error` 的消息使用统一结构：

```json
{
  "requestId": 1,
  "type": "error",
  "method": "fs.read",
  "payload": {
    "code": "NOT_FOUND",
    "message": "file not found: docs/missing.md",
    "details": {}
  }
}
```

错误码表：

| 错误码 | 含义 | 是否可重试 | 备注 |
|--------|------|-----------|------|
| `INVALID_ARGUMENT` | 请求参数非法 | 否 | |
| `UNAUTHORIZED` | 认证失败 | 否 | 立即断连 |
| `FORBIDDEN` | 无权限 | 否 | |
| `NOT_FOUND` | 资源不存在 | 否 | |
| `CONFLICT` | requestId 重复 | 否 | |
| `UNAVAILABLE` | Hub 离线 | 是 | 等待 Hub 重连 |
| `RATE_LIMITED` | 限流 | 是 | 按 backoff 重试 |
| `TIMEOUT` | 处理超时 | 是 | |
| `SNAPSHOT_EXPIRED` | 分块读中途源数据变化 | 是 | 从首块重新开始 |
| `INTERNAL` | 服务端内部错误 | 是 | |

## 4. Hub 侧协议

### 4.1 Hub 连接生命周期

1. 建立 WebSocket（`ws://host:port/ws` 或 `wss://...`）。
2. 调用 `connect.init`，从响应获取 `connectionEpoch`。
3. 调用 `registry.reportProjects` 发送全量快照。
4. steady 状态下按需调用 `registry.updateProject`。
5. 连接失活后自动重连，重连成功后重新获取新 `connectionEpoch` 并发送全量快照。

### 4.2 `registry.reportProjects`（全量覆盖）

请求：

```json
{
  "requestId": 1,
  "type": "request",
  "method": "registry.reportProjects",
  "payload": {
    "hubId": "local-hub",
    "connectionEpoch": 42,
    "projects": [
      {
        "name": "WheelMaker",
        "path": "D:/Code/WheelMaker",
        "online": true,
        "agent": "codex",
        "imType": "feishu",
        "projectRev": "sha256:...",
        "git": {
          "branch": "main",
          "headSha": "abc123...",
          "dirty": true,
          "gitRev": "sha256:...",
          "worktreeRev": "sha256:..."
        }
      }
    ]
  }
}
```

约束：

- 语义为"当前全量覆盖"，不是 patch。
- 仅接受已认证 hub 连接调用。
- `connectionEpoch` 必填，取自 `connect.init` 响应。
- Registry 拒绝 `connectionEpoch < 当前已知 epoch` 的请求（防止旧连接覆盖新数据）。
- 必须按连接实例区分同 `hubId`，防止旧连接断开误删新映射。

### 4.3 `registry.updateProject`（单项目刷新）

说明：用于某一个 project 状态变化时定点刷新；其 `project` 对象使用 2.3 节定义的完整 `ProjectObject`。

请求：

```json
{
  "requestId": 2,
  "type": "request",
  "method": "registry.updateProject",
  "payload": {
    "hubId": "local-hub",
    "connectionEpoch": 42,
    "seq": 1024,
    "project": {
      "name": "WheelMaker",
      "path": "D:/Code/WheelMaker",
      "online": true,
      "agent": "codex",
      "imType": "feishu",
      "projectRev": "sha256:...",
      "git": {
        "branch": "main",
        "headSha": "def456...",
        "dirty": true,
        "gitRev": "sha256:...",
        "worktreeRev": "sha256:..."
      }
    },
    "changedDomains": ["git", "worktree"],
    "updatedAt": "2026-03-30T10:01:23Z"
  }
}
```

判新规则（幂等）：

```text
key = hubId + "/" + project.name
old = lastSeen[key]  // {connectionEpoch, seq}
new = incoming       // {connectionEpoch, seq}

if new.connectionEpoch > old.connectionEpoch:
  accept
else if new.connectionEpoch == old.connectionEpoch and new.seq > old.seq:
  accept
else:
  ignore as stale_project_update
```

### 4.4 `hub.ping` / `hub.pong`（仅 Hub 保活）

- 仅 Hub 连接要求 ping/pong。
- 建议 Hub 每 `15s` 发送 ping，`45s` 无 pong 判定失活。
- 失活后断开并触发重连。

### 4.5 Hub 侧可观测性

建议指标：

- `reconnect_count`
- `last_report_at`
- `heartbeat_timeout_count`
- `report_ack_latency_ms`

## 5. Client 侧协议

### 5.1 Client 连接行为

- Client 不要求 ping/pong。
- Registry 对空闲 Client 连接做超时清理（建议 5 分钟无任何请求/事件交互）。清理前发送 `connection.closing` 事件 `reason: "idle_timeout"`。
- 发起业务请求前若连接不可用，先重连并重新执行 `connect.init`。
- 重连后使用 `project.syncCheck` 高效恢复状态（见 5.3）。

### 5.2 `project.list`

前置条件：已完成 `connect.init`。返回范围取决于连接作用域（见 3.3）。

请求：

```json
{
  "requestId": 1,
  "type": "request",
  "method": "project.list",
  "payload": {}
}
```

响应：

```json
{
  "requestId": 1,
  "type": "response",
  "method": "project.list",
  "payload": {
    "projects": [
      {
        "projectId": "local-hub:WheelMaker",
        "name": "WheelMaker",
        "path": "D:/Code/WheelMaker",
        "hubId": "local-hub",
        "online": true,
        "agent": "codex",
        "imType": "feishu",
        "projectRev": "sha256:...",
        "git": {
          "branch": "main",
          "headSha": "abc123...",
          "dirty": true,
          "gitRev": "sha256:...",
          "worktreeRev": "sha256:..."
        }
      }
    ]
  }
}
```

### 5.3 `project.syncCheck`（重连后状态恢复）

客户端重连后，携带本地缓存的版本戳，快速判断哪些领域需要刷新。

请求：

```json
{
  "requestId": 2,
  "type": "request",
  "method": "project.syncCheck",
  "projectId": "local-hub:WheelMaker",
  "payload": {
    "knownProjectRev": "sha256:...",
    "knownGitRev": "sha256:...",
    "knownWorktreeRev": "sha256:..."
  }
}
```

响应：

```json
{
  "requestId": 2,
  "type": "response",
  "method": "project.syncCheck",
  "projectId": "local-hub:WheelMaker",
  "payload": {
    "projectRev": "sha256:...",
    "gitRev": "sha256:...",
    "worktreeRev": "sha256:...",
    "staleDomains": ["git", "worktree"]
  }
}
```

客户端按 `staleDomains` 进行增量刷新，逻辑与 `project.changed` 事件处理相同（见第 8 节）。

### 5.4 `fs.list`（目录）

#### 5.4.1 Hash 协商语义

`knownHash` 是 **conditional GET** — 仅当客户端持有该目录/页的完整 entries 缓存时才可发送。若客户端仅知道 hash 值但无缓存数据（如 hash 来源于事件推送、或缓存已被驱逐），则**不得发送 `knownHash`**。

#### 5.4.2 基本请求

```json
{
  "requestId": 1,
  "type": "request",
  "method": "fs.list",
  "projectId": "local-hub:WheelMaker",
  "payload": {
    "path": "docs",
    "knownHash": "sha256:prev-docs-hash",
    "limit": 200,
    "cursor": ""
  }
}
```

#### 5.4.3 响应 — 未变化（knownHash 命中）

```json
{
  "requestId": 1,
  "type": "response",
  "method": "fs.list",
  "projectId": "local-hub:WheelMaker",
  "payload": {
    "path": "docs",
    "snapshotHash": "sha256:prev-docs-hash",
    "notModified": true
  }
}
```

#### 5.4.4 响应 — 有变化（首页）

```json
{
  "requestId": 1,
  "type": "response",
  "method": "fs.list",
  "projectId": "local-hub:WheelMaker",
  "payload": {
    "path": "docs",
    "snapshotHash": "sha256:new-docs-hash",
    "notModified": false,
    "pageHash": "sha256:page-1-hash",
    "snapshotToken": "dt_17119_x9y8z7",
    "entries": [
      { "name": "registry-protocol.md", "path": "docs/registry-protocol.md", "kind": "file", "size": 21459, "mtime": "2026-03-30T09:20:00Z", "dataHash": "sha256:..." },
      { "name": "superpowers", "path": "docs/superpowers", "kind": "dir", "size": 0, "mtime": "2026-03-30T09:10:00Z", "dataHash": "" }
    ],
    "nextCursor": "after:200"
  }
}
```

#### 5.4.5 后续页请求（分页续读）

```json
{
  "requestId": 2,
  "type": "request",
  "method": "fs.list",
  "projectId": "local-hub:WheelMaker",
  "payload": {
    "path": "docs",
    "snapshotToken": "dt_17119_x9y8z7",
    "knownPageHash": "sha256:cached-page-2-hash",
    "cursor": "after:200",
    "limit": 200
  }
}
```

后续页响应 — 页未变化：

```json
{
  "requestId": 2,
  "type": "response",
  "method": "fs.list",
  "projectId": "local-hub:WheelMaker",
  "payload": {
    "path": "docs",
    "pageHash": "sha256:cached-page-2-hash",
    "notModified": true,
    "nextCursor": ""
  }
}
```

后续页响应 — 页有变化：

```json
{
  "requestId": 2,
  "type": "response",
  "method": "fs.list",
  "projectId": "local-hub:WheelMaker",
  "payload": {
    "path": "docs",
    "pageHash": "sha256:new-page-2-hash",
    "notModified": false,
    "entries": [...],
    "nextCursor": ""
  }
}
```

#### 5.4.6 `fs.list` 字段总结

| 字段 | 首页请求 | 首页响应 | 后续页请求 | 后续页响应 |
|------|---------|---------|-----------|-----------|
| `knownHash` | 可选（有整页缓存时） | — | — | — |
| `snapshotHash` | — | 必返回 | — | — |
| `knownPageHash` | — | — | 可选（有本页缓存时） | — |
| `pageHash` | — | 必返回 | — | 必返回 |
| `notModified` | — | 必返回 | — | 必返回 |
| `snapshotToken` | — | 有后续页时必返回 | 必传 | — |
| `cursor` | `""` | — | 必传 | — |
| `nextCursor` | — | 必返回 | — | 必返回 |
| `entries` | — | `notModified=false` 时 | — | `notModified=false` 时 |

> **`snapshotHash` 仅覆盖当前目录的直接子项**（一级），不递归反映子目录内部变化。客户端必须对每个已展开目录独立发起 `fs.list` 协商；父目录 `notModified=true` 不意味着子目录未变化。

### 5.5 `fs.read`（文件）

#### 5.5.1 Hash 协商语义

与 `fs.list` 相同：`knownHash` 是 conditional GET。

- `contentHash` 始终是**整个文件**的 hash（`sha256(raw bytes)`），与 `offset/limit` 无关。
- 仅当客户端持有本次请求范围 `[offset, offset+limit)` 的缓存且知道当时的 `contentHash` 时才可发送 `knownHash`。
- 若客户端仅从 `fs.list` 的 `dataHash` 获知文件 hash 但从未读过内容，**不得发送 `knownHash`**。
- 文件级 hash 未变 ⟹ 所有字节未变 ⟹ 任意范围的缓存均有效。

#### 5.5.2 二进制文件处理

服务端通过内容检测判定文件是否为二进制。响应中增加：

- `isBinary: boolean` — 是否为二进制文件。
- `mimeType: string` — MIME 类型（如 `"image/png"`、`"application/octet-stream"`）。
- 当 `isBinary=true` 时：
  - 小于 2MiB：`encoding: "base64"`，`content` 为 base64 编码。
  - 大于等于 2MiB：`encoding: "none"`，`content: null`，仅返回元信息。
- 当 `isBinary=false` 时：`encoding: "utf-8"`。

#### 5.5.3 基本请求/响应

首块或全量请求：

```json
{
  "requestId": 1,
  "type": "request",
  "method": "fs.read",
  "projectId": "local-hub:WheelMaker",
  "payload": {
    "path": "docs/registry-protocol.md",
    "knownHash": "sha256:old-content-hash",
    "offset": 0,
    "limit": 65536
  }
}
```

响应 — 未变化：

```json
{
  "requestId": 1,
  "type": "response",
  "method": "fs.read",
  "projectId": "local-hub:WheelMaker",
  "payload": {
    "path": "docs/registry-protocol.md",
    "contentHash": "sha256:old-content-hash",
    "notModified": true
  }
}
```

响应 — 有变化（小文件一块读完）：

```json
{
  "requestId": 1,
  "type": "response",
  "method": "fs.read",
  "projectId": "local-hub:WheelMaker",
  "payload": {
    "path": "docs/registry-protocol.md",
    "contentHash": "sha256:new-content-hash",
    "notModified": false,
    "isBinary": false,
    "mimeType": "text/markdown",
    "content": "...file content...",
    "encoding": "utf-8",
    "size": 21459,
    "eof": true
  }
}
```

#### 5.5.4 分块读取流程

推荐默认块大小 `64KiB`，单次上限 `1MiB`。

首块响应（有后续块）：

```json
{
  "requestId": 2,
  "type": "response",
  "method": "fs.read",
  "projectId": "local-hub:WheelMaker",
  "payload": {
    "path": "docs/big-file.md",
    "contentHash": "sha256:whole-file-hash",
    "notModified": false,
    "isBinary": false,
    "mimeType": "text/markdown",
    "content": "...chunk-1...",
    "encoding": "utf-8",
    "size": 200000,
    "eof": false,
    "nextOffset": 65536,
    "snapshotToken": "ft_17119_a3b2c1"
  }
}
```

后续块请求：

```json
{
  "requestId": 3,
  "type": "request",
  "method": "fs.read",
  "projectId": "local-hub:WheelMaker",
  "payload": {
    "path": "docs/big-file.md",
    "snapshotToken": "ft_17119_a3b2c1",
    "offset": 65536,
    "limit": 65536
  }
}
```

后续块响应：

```json
{
  "requestId": 3,
  "type": "response",
  "method": "fs.read",
  "projectId": "local-hub:WheelMaker",
  "payload": {
    "path": "docs/big-file.md",
    "content": "...chunk-2...",
    "encoding": "utf-8",
    "eof": true,
    "nextOffset": 200000,
    "snapshotToken": "ft_17119_a3b2c1"
  }
}
```

#### 5.5.5 随机范围读取

客户端可以从文件任意位置开始读取（如大日志文件滚动到中间）：

```json
{
  "requestId": 4,
  "type": "request",
  "method": "fs.read",
  "projectId": "local-hub:WheelMaker",
  "payload": {
    "path": "logs/server.log",
    "knownHash": "sha256:file-hash-when-cached",
    "offset": 5242880,
    "limit": 65536
  }
}
```

规则：

- `contentHash` 始终是整文件 hash，不随 offset 变化。
- `knownHash` 表示"我有 `[offset, offset+limit)` 的缓存，且缓存时文件 hash 为此值"。
- 文件 hash 未变 → 该范围必然未变 → `notModified=true`。
- 文件 hash 变了 → 返回新数据 + 新 `contentHash`，客户端应失效该文件的所有范围缓存。

#### 5.5.6 `fs.read` 字段总结

| 字段 | 首块/范围请求 | 首块响应 | 后续块请求 | 后续块响应 |
|------|-------------|---------|-----------|-----------|
| `knownHash` | 可选（有对应范围缓存时） | — | 不传 | — |
| `contentHash` | — | 必返回 | — | 不返回 |
| `notModified` | — | 必返回 | — | 不返回 |
| `snapshotToken` | 不传 | 有后续块时必返回 | 必传 | 回显 |
| `offset` / `limit` | 必传 | — | 必传 | — |
| `eof` | — | 必返回 | — | 必返回 |
| `nextOffset` | — | `eof=false` 时 | — | 必返回 |
| `isBinary` | — | 首块必返回 | — | 不返回 |
| `mimeType` | — | 首块必返回 | — | 不返回 |
| `size` | — | 首块必返回 | — | 不返回 |

### 5.6 `snapshotToken` 一致性机制

`fs.list`（分页）和 `fs.read`（分块）共享同一套一致性保障规则：

1. 服务端在首块/首页响应中返回 `snapshotToken`（不透明字符串），当且仅当存在后续块/页（`eof=false` 或 `nextCursor!=""`）。
2. 客户端在所有后续请求中必须回传 `snapshotToken`。
3. 服务端在处理后续块/页前校验源数据是否变化：
   - 未变化：正常返回。
   - 已变化：返回 `SNAPSHOT_EXPIRED` 错误。
4. 客户端收到 `SNAPSHOT_EXPIRED` 后丢弃已读取的所有部分数据，从首块/首页重新开始。
5. `snapshotToken` 有服务端 TTL（建议 60 秒）。过期同样返回 `SNAPSHOT_EXPIRED`。
6. 单块/单页完成（`eof=true` / `nextCursor=""`）的场景不需要 `snapshotToken`。

服务端实现建议（轻量级，不需要 COW）：

```go
type snapshotToken struct {
    path  string
    mtime int64
    size  int64
    hash  string
}
// 后续块请求时：if file.mtime != token.mtime || file.size != token.size → SNAPSHOT_EXPIRED
```

### 5.7 `fs.search`（文件名模糊查找）

首版仅支持文件名模糊匹配，不做内容检索（内容检索见 5.8 `fs.grep`）。

请求：

```json
{
  "requestId": 1,
  "type": "request",
  "method": "fs.search",
  "projectId": "local-hub:WheelMaker",
  "payload": {
    "query": "registry protocol",
    "root": ".",
    "mode": "fuzzy",
    "caseSensitive": false,
    "limit": 50,
    "cursor": ""
  }
}
```

响应：

```json
{
  "requestId": 1,
  "type": "response",
  "method": "fs.search",
  "projectId": "local-hub:WheelMaker",
  "payload": {
    "results": [
      { "path": "docs/registry-protocol.md", "name": "registry-protocol.md", "kind": "file", "score": 0.97 },
      { "path": "docs/superpowers", "name": "superpowers", "kind": "dir", "score": 0.42 }
    ],
    "nextCursor": ""
  }
}
```

### 5.8 `fs.grep`（文件内容搜索）

请求：

```json
{
  "requestId": 1,
  "type": "request",
  "method": "fs.grep",
  "projectId": "local-hub:WheelMaker",
  "payload": {
    "pattern": "UNAUTHORIZED",
    "root": ".",
    "isRegex": false,
    "caseSensitive": false,
    "includeGlob": "*.go",
    "contextLines": 2,
    "limit": 50,
    "cursor": ""
  }
}
```

响应：

```json
{
  "requestId": 1,
  "type": "response",
  "method": "fs.grep",
  "projectId": "local-hub:WheelMaker",
  "payload": {
    "results": [
      {
        "path": "internal/registry/auth.go",
        "matches": [
          {
            "line": 42,
            "content": "  return errors.New(\"UNAUTHORIZED\")",
            "contextBefore": ["func checkToken(t string) error {", "  if t == \"\" {"],
            "contextAfter": ["  }"]
          }
        ]
      }
    ],
    "totalMatches": 7,
    "nextCursor": ""
  }
}
```

### 5.9 Git 只读接口

#### 5.9.1 `git.refs`（分支与标签）

请求：

```json
{
  "requestId": 1,
  "type": "request",
  "method": "git.refs",
  "projectId": "local-hub:WheelMaker",
  "payload": {}
}
```

响应：

```json
{
  "requestId": 1,
  "type": "response",
  "method": "git.refs",
  "projectId": "local-hub:WheelMaker",
  "payload": {
    "current": "main",
    "branches": ["main", "feature/x"],
    "tags": [
      { "name": "v1.0.0", "sha": "abc123..." },
      { "name": "v1.1.0", "sha": "def456..." }
    ]
  }
}
```

#### 5.9.2 `git.log`

请求：

```json
{
  "requestId": 1,
  "type": "request",
  "method": "git.log",
  "projectId": "local-hub:WheelMaker",
  "payload": {
    "ref": "main",
    "limit": 50,
    "cursor": ""
  }
}
```

响应：

```json
{
  "requestId": 1,
  "type": "response",
  "method": "git.log",
  "projectId": "local-hub:WheelMaker",
  "payload": {
    "commits": [
      {
        "sha": "abc123...",
        "author": "John Doe",
        "email": "john@example.com",
        "time": "2026-03-30T09:00:00Z",
        "title": "feat: add registry protocol v2"
      },
      {
        "sha": "def456...",
        "author": "Jane Smith",
        "email": "jane@example.com",
        "time": "2026-03-29T18:30:00Z",
        "title": "fix: handle connection timeout"
      }
    ],
    "nextCursor": "after:def456"
  }
}
```

#### 5.9.3 `git.commit.files`

请求：

```json
{
  "requestId": 1,
  "type": "request",
  "method": "git.commit.files",
  "projectId": "local-hub:WheelMaker",
  "payload": {
    "sha": "abc123"
  }
}
```

响应：

```json
{
  "requestId": 1,
  "type": "response",
  "method": "git.commit.files",
  "projectId": "local-hub:WheelMaker",
  "payload": {
    "sha": "abc123",
    "files": [
      { "path": "docs/registry-protocol.md", "status": "M", "additions": 120, "deletions": 15 },
      { "path": "internal/registry/hub.go", "status": "A", "additions": 85, "deletions": 0 }
    ]
  }
}
```

#### 5.9.4 `git.commit.fileDiff`

请求：

```json
{
  "requestId": 1,
  "type": "request",
  "method": "git.commit.fileDiff",
  "projectId": "local-hub:WheelMaker",
  "payload": {
    "sha": "abc123",
    "path": "docs/registry-protocol.md",
    "contextLines": 3
  }
}
```

响应：

```json
{
  "requestId": 1,
  "type": "response",
  "method": "git.commit.fileDiff",
  "projectId": "local-hub:WheelMaker",
  "payload": {
    "sha": "abc123",
    "path": "docs/registry-protocol.md",
    "isBinary": false,
    "diff": "@@ -1,5 +1,7 @@\n-old line\n+new line\n...",
    "truncated": false
  }
}
```

#### 5.9.5 `git.diff`（任意两个 ref 之间比较）

请求：

```json
{
  "requestId": 1,
  "type": "request",
  "method": "git.diff",
  "projectId": "local-hub:WheelMaker",
  "payload": {
    "base": "main",
    "head": "feature/x",
    "limit": 100,
    "cursor": ""
  }
}
```

响应：

```json
{
  "requestId": 1,
  "type": "response",
  "method": "git.diff",
  "projectId": "local-hub:WheelMaker",
  "payload": {
    "base": "main",
    "head": "feature/x",
    "files": [
      { "path": "src/main.go", "status": "M", "additions": 15, "deletions": 3 },
      { "path": "src/new-feature.go", "status": "A", "additions": 120, "deletions": 0 }
    ],
    "nextCursor": ""
  }
}
```

查看具体文件 diff：

```json
{
  "requestId": 2,
  "type": "request",
  "method": "git.diff.fileDiff",
  "projectId": "local-hub:WheelMaker",
  "payload": {
    "base": "main",
    "head": "feature/x",
    "path": "src/main.go",
    "contextLines": 3
  }
}
```

响应格式与 `git.commit.fileDiff` 一致（`isBinary`, `diff`, `truncated`）。

#### 5.9.6 `git.status`（工作区未提交改动视图）

请求：

```json
{
  "requestId": 1,
  "type": "request",
  "method": "git.status",
  "projectId": "local-hub:WheelMaker",
  "payload": {}
}
```

响应：

```json
{
  "requestId": 1,
  "type": "response",
  "method": "git.status",
  "projectId": "local-hub:WheelMaker",
  "payload": {
    "dirty": true,
    "worktreeRev": "sha256:...",
    "staged": [{ "path": "docs/registry-protocol.md", "status": "M" }],
    "unstaged": [{ "path": "app/web/src/main.tsx", "status": "M" }],
    "untracked": [{ "path": "tmp/new.txt", "status": "?" }]
  }
}
```

`status` 值对齐 `git status --porcelain`：

| 值 | 含义 |
|----|------|
| `M` | 已修改 (modified) |
| `A` | 新增 (added) |
| `D` | 删除 (deleted) |
| `R` | 重命名 (renamed) |
| `C` | 复制 (copied) |
| `U` | 未合并冲突 (unmerged) |
| `?` | 未跟踪 (untracked) |

#### 5.9.7 `git.workingTree.fileDiff`（查看工作区未提交文件 diff）

请求：

```json
{
  "requestId": 1,
  "type": "request",
  "method": "git.workingTree.fileDiff",
  "projectId": "local-hub:WheelMaker",
  "payload": {
    "path": "src/main.go",
    "scope": "unstaged",
    "contextLines": 3
  }
}
```

`scope` 取值：

| 值 | 含义 | 等效 git 命令 |
|----|------|--------------|
| `staged` | 已暂存的改动 | `git diff --cached -- <path>` |
| `unstaged` | 未暂存的改动 | `git diff -- <path>` |
| `untracked` | 新文件完整内容（以 new-file diff 形式返回） | — |

响应：

```json
{
  "requestId": 1,
  "type": "response",
  "method": "git.workingTree.fileDiff",
  "projectId": "local-hub:WheelMaker",
  "payload": {
    "path": "src/main.go",
    "scope": "unstaged",
    "isBinary": false,
    "diff": "@@ -10,3 +10,5 @@\n-old code\n+new code\n...",
    "truncated": false
  }
}
```

#### 5.9.8 Git 缓存规则

- Git 列表按 `headSha/gitRev` 变化触发刷新。
- `git.commit.files` 以 `sha` 为缓存键。
- `git.commit.fileDiff` 以 `sha+path+contextLines` 为缓存键。
- Git 列表不做 `knownHash/notModified` 协商。

### 5.10 Agent 观测接口

#### 5.10.1 `agent.status`

请求：

```json
{
  "requestId": 1,
  "type": "request",
  "method": "agent.status",
  "projectId": "local-hub:WheelMaker",
  "payload": {}
}
```

响应：

```json
{
  "requestId": 1,
  "type": "response",
  "method": "agent.status",
  "projectId": "local-hub:WheelMaker",
  "payload": {
    "running": true,
    "agent": "codex",
    "currentTask": "Implementing user authentication",
    "startedAt": "2026-03-30T10:00:00Z",
    "lastActivityAt": "2026-03-30T10:05:23Z"
  }
}
```

#### 5.10.2 `agent.activity` 事件

```json
{
  "type": "event",
  "method": "agent.activity",
  "projectId": "local-hub:WheelMaker",
  "payload": {
    "running": true,
    "summary": "Editing src/auth/handler.go",
    "changedFiles": ["src/auth/handler.go", "src/auth/handler_test.go"]
  }
}
```

### 5.11 `batch`（批量请求）

将多个独立请求合并为单次发送，减少移动端高延迟环境下的 RTT 开销。

请求：

```json
{
  "requestId": 1,
  "type": "request",
  "method": "batch",
  "payload": {
    "requests": [
      { "method": "fs.list", "projectId": "local-hub:WheelMaker", "payload": { "path": "src", "limit": 200, "cursor": "" } },
      { "method": "fs.list", "projectId": "local-hub:WheelMaker", "payload": { "path": "docs", "limit": 200, "cursor": "" } },
      { "method": "git.status", "projectId": "local-hub:WheelMaker", "payload": {} }
    ]
  }
}
```

响应：

```json
{
  "requestId": 1,
  "type": "response",
  "method": "batch",
  "payload": {
    "responses": [
      { "index": 0, "type": "response", "method": "fs.list", "payload": { "..." : "..." } },
      { "index": 1, "type": "response", "method": "fs.list", "payload": { "..." : "..." } },
      { "index": 2, "type": "response", "method": "git.status", "payload": { "..." : "..." } }
    ]
  }
}
```

约束：

- 批量中的每个子请求独立执行、独立成功/失败。
- 子响应的 `type` 可以是 `response` 或 `error`。
- 批量内不允许嵌套 `batch`。
- 批量内不允许 `connect.init`。

### 5.12 `subscribe` / `unsubscribe`（路径级推送订阅）

#### 5.12.1 `subscribe`

```json
{
  "requestId": 1,
  "type": "request",
  "method": "subscribe",
  "projectId": "local-hub:WheelMaker",
  "payload": {
    "paths": ["src/", "docs/registry-protocol.md"],
    "domains": ["fs", "git", "agent"]
  }
}
```

响应：

```json
{
  "requestId": 1,
  "type": "response",
  "method": "subscribe",
  "payload": { "ok": true }
}
```

#### 5.12.2 `unsubscribe`

```json
{
  "requestId": 2,
  "type": "request",
  "method": "unsubscribe",
  "projectId": "local-hub:WheelMaker",
  "payload": {
    "paths": ["docs/registry-protocol.md"]
  }
}
```

#### 5.12.3 订阅后的精准事件推送

```json
{
  "type": "event",
  "method": "fs.changed",
  "projectId": "local-hub:WheelMaker",
  "payload": {
    "changedPaths": [
      { "path": "src/main.go", "kind": "file" },
      { "path": "src/auth/", "kind": "dir" }
    ]
  }
}
```

客户端折叠目录或离开页面时应主动 `unsubscribe`，避免不必要的推送。

## 6. Hash 规范（统一算法）

统一约定：

- 哈希算法：`sha256`。
- 文本拼接编码：`UTF-8`。
- 输出格式：`sha256:<hex-lowercase>`。
- 规范化要求：排序、换行符、路径分隔符必须先归一，再计算哈希。

### 6.1 `dataHash`（文件内容） / `contentHash`

- 计算：`sha256(raw bytes)`。
- `dataHash` 出现在目录项的文件条目中。
- `contentHash` 出现在 `fs.read` 响应中。
- 二者对同一文件的值完全相同。

### 6.2 `snapshotHash`（目录快照）

对目录下直接子项（一级）生成字符串：`kind|name|dataHash`。

- `kind=dir` 时 `dataHash` 固定为空字符串。
- `kind=file` 时 `dataHash` 为该文件的 `dataHash`。
- 对所有条目先按 `(kind, name)` 升序排序，再用 `\n` 拼接。
- 对拼接结果做 `sha256`，得到 `snapshotHash`。

> **重要**：`snapshotHash` 仅覆盖直接子项。子目录内部变化不会反映在父目录的 `snapshotHash` 中。客户端必须对每个展开层级独立做 hash 协商。

### 6.3 `pageHash`（目录分页哈希）

计算方式与 `snapshotHash` 相同，但范围从整个目录缩小到**当前页的条目**：

```text
pageHash = sha256( join("\n", sort(page_entries).map(e => "kind|name|dataHash")) )
```

用于分页续读时跳过未变化的页。

### 6.4 `worktreeRev`（工作区）

- 输入来源：`git status --porcelain` 输出。
- 归一规则：
  - 行按路径升序。
  - 路径统一为 `/` 分隔。
  - 去除尾随空白。
- 对归一结果做 `sha256` 得到 `worktreeRev`。

### 6.5 `gitRev`（Git 版本）

- 输入串：`branch + "\n" + headSha + "\n" + dirty`。
- `dirty` 使用布尔字符串 `true/false`。
- 对输入串做 `sha256` 得到 `gitRev`。

### 6.6 `projectRev`（项目聚合版本）

- 当前阶段仅由 Git 侧派生，不引入全量 FS watcher。
- 输入串：`gitRev + "\n" + worktreeRev`。
- 对输入串做 `sha256` 得到 `projectRev`。

### 6.7 `knownHash` 协商规则（Conditional GET）

统一规则适用于 `fs.list` 和 `fs.read`：

1. **`knownHash` 仅在发送方持有对应请求范围的完整缓存数据时才可携带。**
2. 若发送方仅持有 hash 值但无对应数据（如 hash 来源于目录 `dataHash`、事件推送、或缓存已驱逐），**不得发送 `knownHash`**。
3. 服务端收到 `knownHash` 且命中时，`notModified=true` 响应中不返回数据体。
4. 对 `fs.read` 的范围读取：`knownHash` 表示"我有 `[offset, offset+limit)` 的缓存，且缓存时整文件 hash 为此值"。文件级 hash 未变 ⟹ 全部字节未变 ⟹ 任意范围缓存有效。
5. 文件 hash 变化后，客户端应失效该文件所有已缓存的范围。

## 7. 同步策略（Push Hint + Pull Data）

### 7.1 事件定义

#### `project.changed`

```json
{
  "type": "event",
  "method": "project.changed",
  "projectId": "local-hub:WheelMaker",
  "payload": {
    "projectRev": "sha256:...",
    "gitRev": "sha256:...",
    "worktreeRev": "sha256:...",
    "changedDomains": ["git", "worktree"],
    "changedPaths": ["src/auth/", "docs/registry-protocol.md"]
  }
}
```

`changedPaths` 为可选字段。存在时，客户端仅刷新与之交集的可见路径；不存在时，刷新所有可见路径。

#### `git.workspace.changed`

```json
{
  "type": "event",
  "method": "git.workspace.changed",
  "projectId": "local-hub:WheelMaker",
  "payload": {
    "dirty": true,
    "worktreeRev": "sha256:..."
  }
}
```

#### `project.offline` / `project.online`

Hub 断连或重连时，Registry 向已订阅的 Client 推送：

```json
{
  "type": "event",
  "method": "project.offline",
  "projectId": "local-hub:WheelMaker",
  "payload": {}
}
```

```json
{
  "type": "event",
  "method": "project.online",
  "projectId": "local-hub:WheelMaker",
  "payload": {}
}
```

#### `connection.closing`

Registry 即将关闭客户端连接时提前通知：

```json
{
  "type": "event",
  "method": "connection.closing",
  "payload": {
    "reason": "idle_timeout"
  }
}
```

### 7.2 同步约束

- 事件是提示（hint），不是状态真值；客户端必须以拉取结果为准。
- 同一个 `projectId` 上，客户端应仅保留最后一个待处理事件（可合并去抖）。
- 处理超时或失败时可重试，重试失败回退到 `project.list` + 可见范围全量重拉。

## 8. 客户端增量刷新规则

### 8.1 触发源

- 收到 `project.changed` / `git.workspace.changed` / `fs.changed` 事件。
- 或轮询发现 `projectRev/gitRev/worktreeRev` 与本地缓存不一致。
- 或重连后 `project.syncCheck` 返回的 `staleDomains`。

### 8.2 刷新顺序（同一 project）

1. 先刷新 `project.list` 中该 project 的元信息缓存。
2. 按 `changedDomains` / `staleDomains` 分流：
   - 包含 `git`：拉 `git.log`，按当前选中项继续拉 `git.commit.files` / `git.commit.fileDiff`。
   - 包含 `worktree`：拉 `git.status`。
   - 包含 `fs` 或未提供 `changedDomains`：按可见范围执行 `fs.list` / `fs.read`。

### 8.3 可见范围拉取规则（FS）

- 已展开目录：`fs.list(path, knownHash)`。
- 当前打开文件和 Pin 文件：`fs.read(path, knownHash, offset, limit)`。
- 非可见目录与文件不主动拉取。
- 若 `changedPaths` 存在，仅对交集路径做刷新。
- 父目录 `notModified=true` **不代表**子目录未变化（见 6.2），每个展开层级独立协商。

### 8.4 幂等与去重

- 对同一路径并发请求做合并（同 key 仅保留 1 个在途请求）。
- 响应 `notModified=true` 时仅更新时间戳，不刷新内容缓存。
- 新响应版本戳早于本地已处理版本时丢弃（防止乱序覆盖）。

## 9. 实施建议

1. **协议收敛**：统一 `project.list`；保留 `reportProjects + updateProject` 双上报。
2. **Hub 侧**：接入 `projectRev/gitRev/headSha/dirty/worktreeRev` 即时上报；回传 `connectionEpoch`。
3. **服务端**：实现路由反查、幂等判新、`snapshotToken` 管理、错误码标准化。
4. **客户端**：按方案 C 做可见范围增量刷新；实现 conditional GET 与分块一致性。
5. **可观测性**：补齐 hash 命中率、snapshotToken 过期率、Git 刷新耗时、工作区变更推送频率等指标。

## 10. 版本历史

### 2.1 相比 2.0 的改进

1. **消息封装简化**
   - 移除消息体中的 `version` 字段，协议版本唯一通过 `connect.init.protocolVersion` 协商。
   - 明确 `type=event` 不携带 `requestId`。

2. **`connectionEpoch` 机制明确**
   - 由 Registry 在 `connect.init` 响应中分配，全局单调递增。
   - `reportProjects` 新增 `connectionEpoch` 字段，防止旧连接覆盖新数据。

3. **错误体系标准化**
   - 定义统一错误响应 JSON 结构（`code` + `message` + `details`）。
   - 补齐完整错误码表（含 `SNAPSHOT_EXPIRED`、`INTERNAL`）。

4. **Hash 协商语义收敛（Conditional GET）**
   - 明确 `knownHash` 仅在持有缓存数据时才可发送。
   - 明确 `contentHash` 始终为整文件 hash，支持任意 offset 范围的缓存验证。
   - 明确 `snapshotHash` 仅覆盖一级子项，客户端必须按层级独立协商。

5. **分块读一致性保障（`snapshotToken`）**
   - `fs.list` 分页和 `fs.read` 分块统一引入 `snapshotToken`。
   - 源数据变化时返回 `SNAPSHOT_EXPIRED`，客户端从头重读。
   - 定义 token TTL（建议 60s）。

6. **目录分页增强（`pageHash`）**
   - 后续页支持 `knownPageHash` 跳过未变化的页，减少传输量。

7. **二进制文件处理**
   - `fs.read` 响应增加 `isBinary`、`mimeType` 字段。
   - 定义 base64 编码和 metadata-only 两种策略。

8. **共享 ProjectObject**
   - 统一 `reportProjects` 和 `updateProject` 的 project 对象结构。
   - `project.list` 响应补齐 `agent`、`imType` 字段。

9. **新增 Git 方法**
   - `git.refs`：合并分支与标签查询（替代 `git.branches`）。
   - `git.diff` / `git.diff.fileDiff`：任意两个 ref 之间的比较。
   - `git.workingTree.fileDiff`：查看未提交改动的 diff（staged/unstaged/untracked）。
   - `git.status` 的 status 值补齐枚举（M/A/D/R/C/U/?）。
   - 补全 `git.log`、`git.commit.files`、`git.commit.fileDiff` 响应示例。

10. **新增 Agent 观测接口**
    - `agent.status`：查询 agent 当前状态。
    - `agent.activity` 事件：实时推送 agent 活动。

11. **新增客户端能力**
    - `project.syncCheck`：重连后高效判断哪些领域需要刷新。
    - `fs.grep`：文件内容搜索。
    - `fs.search` 结果增加 `kind` 字段。
    - `batch`：批量请求支持。
    - `subscribe` / `unsubscribe`：路径级精准推送订阅。
    - Client hub 作用域模型明确（单 hub 绑定 vs 全局）。

12. **连接管理增强**
    - `project.offline` / `project.online` 事件。
    - `connection.closing` 事件（含 idle timeout）。
    - 方法白名单补齐，明确事件不受白名单约束。

### 2.0 相比 1.0 的修正内容

1. 握手与认证模型收敛
   - 从 `hello` + `auth` 双阶段改为单次 `connect.init`。
   - 初始化请求统一携带 `role/hubId/token`，并支持 `ts+nonce` 抗重放。
   - 认证失败响应统一为 `UNAUTHORIZED`，并要求失败后立即断连。

2. 身份与路由语义明确
   - 明确区分 `hub` 与 `client` 连接身份。
   - 统一 `projectId = hubId + ":" + projectName`。
   - Registry 维护 `projectId -> hubId` 反查与转发规则，减少路由歧义。

3. 项目列表与能力视图收敛
   - 统一使用 `project.list`，移除 `project.listFull`。
   - `project.list` 返回 `hubId`、`projectRev` 与 Git 状态字段，便于客户端识别归属与版本。

4. Git 同步策略修正
   - 明确 Git 列表按版本触发，不做 hash 协商。
   - 引入 `gitRev/headSha/dirty/worktreeRev/projectRev` 统一版本语义。
   - 增加 `git.status` 与 `git.workspace.changed`，覆盖未提交改动监控。

5. 文件系统增量协议增强
   - `fs.list` 与 `fs.read` 支持 `knownHash/notModified` 协商，降低重复传输。
   - 目录 hash 规则收敛为 `kind|name|dataHash`（文件有 `dataHash`，目录为空字符串）。

6. 同步模式标准化
   - 固化方案 C：`push hint + pull data`。
   - 通过 `project.changed` / `git.workspace.changed` 事件提示，再由客户端按可见范围按需拉取。

7. 安全与可观测性补齐
   - 补齐统一错误码规范（含 `FORBIDDEN/NOT_FOUND/UNAVAILABLE/RATE_LIMITED/TIMEOUT`）。
   - 增加连接级限速、重连退避与审计字段要求。
   - 当前版本不强制协议层 `wss`，由后续网络安全专项推进。
