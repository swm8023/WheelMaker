# WheelMaker Registry Protocol 2.1

本文定义 WheelMaker Registry 2.1 协议，覆盖 Registry、Hub、Client 的连接、认证、路由、同步与数据传输行为。

## 1. 目标与范围

- 以 `project` 为同步单元，支持多 Hub、多 Project。
- 客户端只刷新可见范围（展开目录、打开/Pin 文件、当前 Git 视图）。
- 同步模型：push hint + pull data，保障最终一致。
- `fs.list`：一次性返回 `{name, kind}` 极简条目，不分页。
- `fs.info`：查询路径元信息（文件类型、大小、总行数等），客户端据此决定 `fs.read` 的寻址语义。
- `fs.read`：统一字段 `offset`/`count`，文本按行寻址（行号/行数），二进制按字节偏移寻址，语义由 `isBinary` 决定；分段仅为传输优化。
- `hash` 协商（conditional GET）减少重复传输；Git 列表按版本触发，不做 hash 协商。
- 协议仅描述 2.1 语义，不含旧版本兼容。

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

所有使用 project 对象的方法必须包含完整字段，不允许部分省略。Client 侧响应额外附加 `projectId`（格式 `hubId:projectName`，客户端从中即可解析 Hub 归属）。

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

消息体不含 `version` 字段。协议版本通过 `connect.init.protocolVersion` 唯一协商。

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

- `role` 必填，仅允许 `hub`、`client`、`monitor`。
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
| `client` | `project.list`、`project.syncCheck`、`fs.*`、`git.*`、`batch` |
| `monitor` | `project.list`、`monitor.listHub`、`monitor.status`、`monitor.log`、`monitor.db`、`monitor.action`、`batch` |

- 方法与角色不匹配返回 `FORBIDDEN`。
- **事件方法**（`project.changed`、`git.workspace.changed`、`project.offline`、`project.online`等）是服务端→客户端推送，不受白名单约束。

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

目录列表一次性返回全部直接子项，不做分页。目录规模有限，无需分块传输。

#### 5.4.1 请求

```json
{
  "requestId": 1,
  "type": "request",
  "method": "fs.list",
  "projectId": "local-hub:WheelMaker",
  "payload": {
    "path": "docs",
    "knownHash": "sha256:prev-docs-hash"
  }
}
```

- `knownHash`：可选。客户端持有该目录的 entries 缓存时携带，服务端校验 hash 未变则快速返回 `notModified`。
- 无缓存或缓存已驱逐时**不传** `knownHash`，服务端直接返回完整数据。

#### 5.4.2 响应 — 未变化

```json
{
  "requestId": 1,
  "type": "response",
  "method": "fs.list",
  "projectId": "local-hub:WheelMaker",
  "payload": {
    "path": "docs",
    "hash": "sha256:prev-docs-hash",
    "notModified": true
  }
}
```

#### 5.4.3 响应 — 有变化

```json
{
  "requestId": 1,
  "type": "response",
  "method": "fs.list",
  "projectId": "local-hub:WheelMaker",
  "payload": {
    "path": "docs",
    "hash": "sha256:new-docs-hash",
    "notModified": false,
    "entries": [
      { "name": "registry-protocol.md", "kind": "file" },
      { "name": "superpowers", "kind": "dir" }
    ]
  }
}
```

#### 5.4.4 字段说明

| 字段 | 请求 | 响应 |
|------|------|------|
| `path` | 必传 | 必返回 |
| `knownHash` | 可选（有缓存时） | — |
| `hash` | — | 必返回 |
| `notModified` | — | 必返回 |
| `entries` | — | `notModified=false` 时返回 |

> **目录 `hash`** 仅覆盖直接子项的名称与类型（`kind|name`），不反映文件内容变化，也不递归反映子目录内部变化。文件内容的变化通过 `project.changed` 事件的 `changedPaths` 推送，客户端对已打开文件单独发起 `fs.read`。

### 5.5 `fs.info`（路径元信息查询）

客户端在打开文件或展开目录前，通过 `fs.info` 查询路径的元信息。对于文件，返回类型（文本/二进制）、MIME、大小等，客户端据此决定 `fs.read` 中 `offset`/`count` 的寻址语义。

#### 5.5.1 请求

```json
{
  "requestId": 1,
  "type": "request",
  "method": "fs.info",
  "projectId": "local-hub:WheelMaker",
  "payload": {
    "path": "docs/registry-protocol.md"
  }
}
```

#### 5.5.2 响应 — 文件

```json
{
  "requestId": 1,
  "type": "response",
  "method": "fs.info",
  "projectId": "local-hub:WheelMaker",
  "payload": {
    "path": "docs/registry-protocol.md",
    "kind": "file",
    "size": 21459,
    "isBinary": false,
    "mimeType": "text/markdown",
    "totalLines": 380,
    "hash": "sha256:..."
  }
}
```

#### 5.5.3 响应 — 目录

```json
{
  "requestId": 1,
  "type": "response",
  "method": "fs.info",
  "projectId": "local-hub:WheelMaker",
  "payload": {
    "path": "docs",
    "kind": "dir",
    "entryCount": 5,
    "hash": "sha256:..."
  }
}
```

#### 5.5.4 字段说明

| 字段 | 文件 | 目录 | 说明 |
|------|------|------|------|
| `path` | 必返回 | 必返回 | 请求路径原样回显 |
| `kind` | `"file"` | `"dir"` | 路径类型 |
| `size` | 必返回 | — | 文件字节大小 |
| `isBinary` | 必返回 | — | 是否为二进制文件 |
| `mimeType` | 必返回 | — | MIME 类型 |
| `totalLines` | 文本文件必返回 | — | 文件总行数（`isBinary=false` 时） |
| `entryCount` | — | 必返回 | 直接子项数量 |
| `hash` | 必返回 | 必返回 | 实体 hash |

> 客户端根据 `isBinary` 决定后续 `fs.read` 的寻址语义：文本文件 `offset`/`count` 按**行**寻址，二进制文件按**字节**寻址。

### 5.6 `fs.read`（文件）

#### 5.6.1 Hash 协商语义

`hash` 始终是**整个文件**的 hash（`sha256(raw bytes)`），与读取范围无关。

`knownHash` 协商逻辑：

- **有缓存、不确定是否过期** → 携带 `knownHash`。服务端校验：hash 未变则快速返回 `notModified=true`，hash 变了则返回新数据 + 新 `hash`。
- **无缓存**（首次打开、缓存已驱逐） → 不传 `knownHash`。服务端直接返回完整数据。
- 文件级 hash 未变 ⟹ 所有字节未变 ⟹ 任意范围的缓存均有效。
- 文件 hash 变化后，客户端应失效该文件所有已缓存的范围。

#### 5.6.2 统一寻址：`offset` / `count`

文本文件和二进制文件使用相同的请求字段 `offset`/`count`，语义由文件类型决定（客户端通过 `fs.info` 提前获知）：

| | 文本文件（`isBinary=false`） | 二进制文件（`isBinary=true`） |
|---|---|---|
| `offset` 含义 | 起始行号（从 1 开始） | 字节偏移（从 0 开始） |
| `count` 含义 | 请求行数 | 请求字节数 |
| 编码 | `utf-8` | `base64`（< 2MiB）或 `none`（>= 2MiB，仅元信息） |
| 缓存粒度 | `{path, offset, count, hash}` | `{path, offset, count, hash}` |

响应使用对应的统一字段：

| 响应字段 | 说明 |
|---------|------|
| `offset` | 回显请求的起始位置 |
| `returned` | 实际返回的数量（行数或字节数） |
| `total` | 总量（总行数或总字节数） |
| `hasMore` | 是否还有后续数据 |

#### 5.6.3 文本文件读取

**请求**：

```json
{
  "requestId": 1,
  "type": "request",
  "method": "fs.read",
  "projectId": "local-hub:WheelMaker",
  "payload": {
    "path": "docs/registry-protocol.md",
    "knownHash": "sha256:old-content-hash",
    "offset": 1,
    "count": 500
  }
}
```

省略 `offset` 和 `count` 时默认从第 1 行开始，服务端返回默认行数。

**响应 — 未变化**：

```json
{
  "requestId": 1,
  "type": "response",
  "method": "fs.read",
  "projectId": "local-hub:WheelMaker",
  "payload": {
    "path": "docs/registry-protocol.md",
    "hash": "sha256:old-content-hash",
    "notModified": true
  }
}
```

**响应 — 有变化（小文件一次读完）**：

```json
{
  "requestId": 1,
  "type": "response",
  "method": "fs.read",
  "projectId": "local-hub:WheelMaker",
  "payload": {
    "path": "docs/registry-protocol.md",
    "hash": "sha256:new-content-hash",
    "notModified": false,
    "isBinary": false,
    "mimeType": "text/markdown",
    "encoding": "utf-8",
    "content": "line1\nline2\n...",
    "size": 21459,
    "total": 380,
    "offset": 1,
    "returned": 380,
    "hasMore": false
  }
}
```

**大文件分段 — 首段响应**：

```json
{
  "requestId": 2,
  "type": "response",
  "method": "fs.read",
  "projectId": "local-hub:WheelMaker",
  "payload": {
    "path": "src/big-module.go",
    "hash": "sha256:whole-file-hash",
    "notModified": false,
    "isBinary": false,
    "mimeType": "text/x-go",
    "encoding": "utf-8",
    "content": "package main\n...(500 lines)...",
    "size": 45000,
    "total": 1200,
    "offset": 1,
    "returned": 500,
    "hasMore": true
  }
}
```

**后续段请求**：

```json
{
  "requestId": 3,
  "type": "request",
  "method": "fs.read",
  "projectId": "local-hub:WheelMaker",
  "payload": {
    "path": "src/big-module.go",
    "offset": 501,
    "count": 500
  }
}
```

**后续段响应**：

```json
{
  "requestId": 3,
  "type": "response",
  "method": "fs.read",
  "projectId": "local-hub:WheelMaker",
  "payload": {
    "path": "src/big-module.go",
    "encoding": "utf-8",
    "content": "...(lines 501-1000)...",
    "offset": 501,
    "returned": 500,
    "hasMore": true
  }
}
```

规则：

- `hash` 始终是整文件 hash，不随 offset 变化。
- 有缓存 → 带 `knownHash`，服务端验证后快速返回或返回新数据。
- 无缓存 → 不带 `knownHash`，服务端直接返回数据。
- hash 变了 → 客户端失效该文件所有已缓存的范围。

> **分段一致性**：分段仅为传输优化。若分段过程中文件内容发生变化，客户端可能获得不一致的拼接；下一轮 push hint 事件会触发重新拉取，自动修正。

#### 5.6.4 二进制文件读取

**请求**：

```json
{
  "requestId": 5,
  "type": "request",
  "method": "fs.read",
  "projectId": "local-hub:WheelMaker",
  "payload": {
    "path": "assets/logo.png",
    "offset": 0,
    "count": 65536
  }
}
```

**响应（小于 2MiB）**：

```json
{
  "requestId": 5,
  "type": "response",
  "method": "fs.read",
  "projectId": "local-hub:WheelMaker",
  "payload": {
    "path": "assets/logo.png",
    "hash": "sha256:image-hash",
    "notModified": false,
    "isBinary": true,
    "mimeType": "image/png",
    "encoding": "base64",
    "content": "iVBORw0KGgo...",
    "size": 15360,
    "total": 15360,
    "offset": 0,
    "returned": 15360,
    "hasMore": false
  }
}
```

**响应（>= 2MiB，仅元信息）**：

```json
{
  "requestId": 6,
  "type": "response",
  "method": "fs.read",
  "projectId": "local-hub:WheelMaker",
  "payload": {
    "path": "assets/large-video.mp4",
    "hash": "sha256:video-hash",
    "notModified": false,
    "isBinary": true,
    "mimeType": "video/mp4",
    "encoding": "none",
    "content": null,
    "size": 52428800,
    "total": 52428800,
    "offset": 0,
    "returned": 0,
    "hasMore": false
  }
}
```

#### 5.6.5 `fs.read` 字段总结

| 字段 | 首段请求 | 首段响应 | 后续段请求 | 后续段响应 |
|------|---------|---------|-----------|-----------|
| `knownHash` | 可选 | — | 不传 | — |
| `hash` | — | 必返回 | — | — |
| `notModified` | — | 必返回 | — | — |
| `offset`/`count` | 可选（默认起始位置） | — | 必传 | — |
| `offset` | — | 回显 | — | 回显 |
| `returned` | — | 必返回 | — | 必返回 |
| `total` | — | 必返回 | — | — |
| `hasMore` | — | 必返回 | — | 必返回 |
| `isBinary`/`mimeType`/`size` | — | 必返回 | — | — |

### 5.7 `fs.search`（文件名模糊查找）

文件名模糊匹配。内容检索见 5.8 `fs.grep`。

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

说明：

- `payload.refs` 为可选分支数组；用于一次查询多个分支可见的提交。
- `payload.ref` 保留兼容：
  - 仅传 `ref`：等价于单分支查询。
  - 同时传 `ref` 与 `refs`：服务端会合并并去重（顺序保留）。
- `ref` / `refs` 都为空时，默认按 `HEAD` 查询。
- `cursor` / `nextCursor` 使用偏移量字符串（例如 `"0"`、`"50"`）。

请求：

```json
{
  "requestId": 1,
  "type": "request",
  "method": "git.log",
  "projectId": "local-hub:WheelMaker",
  "payload": {
    "ref": "main",
    "refs": ["main", "release/1.2"],
    "limit": 50,
    "cursor": "0"
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
    "ref": "main",
    "refs": ["main", "release/1.2"],
    "commits": [
      {
        "sha": "abc123...",
        "author": "John Doe",
        "email": "john@example.com",
        "time": "2026-03-30T09:00:00Z",
        "title": "feat: add registry protocol v2"
      }
    ],
    "nextCursor": "50"
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

### 5.10 `batch`（批量请求）

将多个独立请求合并为单次发送，减少移动端高延迟环境下的 RTT 开销。

请求：

```json
{
  "requestId": 1,
  "type": "request",
  "method": "batch",
  "payload": {
    "requests": [
      { "method": "fs.list", "projectId": "local-hub:WheelMaker", "payload": { "path": "src" } },
      { "method": "fs.list", "projectId": "local-hub:WheelMaker", "payload": { "path": "docs" } },
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

### 5.11 Monitor 侧方法（hub 作用域）

`monitor` 角色用于 monitor 页面与 registry 交互。monitor 相关方法全部按 `payload.hubId` 作用到目标 hub。

#### 5.11.1 `monitor.listHub`

返回 monitor 当前可见 hub 列表。

响应 payload 结构：

```json
{
  "hubs": [
    {
      "hubId": "local-hub",
      "online": true
    }
  ]
}
```

#### 5.11.2 `monitor.status` / `monitor.log` / `monitor.db` / `monitor.action`

请求约束：

- `payload.hubId` 必填。
- `projectId` 不参与 monitor 路由。
- 当目标 hub 不在线时，返回 `UNAVAILABLE`。

方法语义：

- `monitor.status`：查询 hub 对应 monitor service/process 状态。
- `monitor.log`：查询 hub monitor 日志（可带 `file`/`level`/`tail`）。
- `monitor.db`：查询 hub monitor db 表快照。
- `monitor.action`：执行 hub monitor 动作（如 `start`/`stop`/`restart`/`update-publish`）。

`project.list` 在 `monitor` 角色下仍可用；用于展示当前选中 hub 下 project 列表（客户端按 `projectId` 的 `hubId:` 前缀过滤）。
## 6. Hash 规范（统一算法）

统一约定：

- 哈希算法：`sha256`。
- 文本拼接编码：`UTF-8`。
- 输出格式：`sha256:<hex-lowercase>`。
- 规范化要求：排序、换行符、路径分隔符必须先归一，再计算哈希。

### 6.1 `hash`（实体哈希）

所有实体统一使用 `hash` 字段名。计算方式按实体类型不同：

**文件**：`hash = sha256(raw bytes)`。仅出现在 `fs.read` 响应中。`fs.list` 不返回单个文件的 hash。

**目录**：对目录下直接子项（一级）生成字符串：`kind|name`。

- 对所有条目先按 `(kind, name)` 升序排序，再用 `\n` 拼接。
- 对拼接结果做 `sha256`，得到 `hash`。
- 目录 hash 仅反映直接子项的**名称和类型**变化（新增/删除/重命名条目），不反映文件内容变化，也不递归反映子目录内部变化。

### 6.2 `worktreeRev`（工作区）

- 输入来源：`git status --porcelain` 输出。
- 归一规则：
  - 行按路径升序。
  - 路径统一为 `/` 分隔。
  - 去除尾随空白。
- 对归一结果做 `sha256` 得到 `worktreeRev`。

### 6.3 `gitRev`（Git 版本）

- 输入串：`branch + "\n" + headSha + "\n" + dirty`。
- `dirty` 使用布尔字符串 `true/false`。
- 对输入串做 `sha256` 得到 `gitRev`。

### 6.4 `projectRev`（项目聚合版本）

- 当前阶段仅由 Git 侧派生，不引入全量 FS watcher。
- 输入串：`gitRev + "\n" + worktreeRev`。
- 对输入串做 `sha256` 得到 `projectRev`。

### 6.5 `knownHash` 协商规则（Conditional GET）

统一规则适用于 `fs.list` 和 `fs.read`：

1. **有缓存、不确定是否过期 → 携带 `knownHash`**。服务端校验 hash：未变则返回 `notModified=true`（不含数据体），变了则返回新数据 + 新 `hash`。
2. **无缓存 → 不传 `knownHash`**。服务端直接返回完整数据。无缓存的场景包括：首次请求、缓存已驱逐、仅从事件推送获知 hash 但从未拉取过数据。
3. 对 `fs.read`：文件级 hash 未变 ⟹ 全部内容未变 ⟹ 任意 `offset`/`count` 范围的缓存均有效。hash 变化后，客户端应失效该文件所有已缓存的范围。
4. 对 `fs.list`：目录 `hash` 仅覆盖 `kind|name`。文件内容变化不影响目录 hash，客户端通过 `changedPaths` 事件判断已打开文件是否需要刷新。

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

- 已展开目录：`fs.list(path, knownHash)` — 目录 hash 仅反映条目名称和类型变化。
- 当前打开文件和 Pin 文件：`fs.read(path, knownHash, offset, count)` — 获取文件内容、hash、size 等详细信息。
- 非可见目录与文件不主动拉取。
- 若 `changedPaths` 存在，仅对交集路径做刷新。
- 文件内容变化不改变父目录 hash，需依赖 `changedPaths` 或 `fs.read` 单独检测。

### 8.4 幂等与去重

- 对同一路径并发请求做合并（同 key 仅保留 1 个在途请求）。
- 响应 `notModified=true` 时仅更新时间戳，不刷新内容缓存。
- 新响应版本戳早于本地已处理版本时丢弃（防止乱序覆盖）。

## 9. 实施建议

1. **协议收敛**：统一 `project.list`；保留 `reportProjects + updateProject` 双上报。
2. **Hub 侧**：接入 `projectRev/gitRev/headSha/dirty/worktreeRev` 即时上报；回传 `connectionEpoch`。
3. **服务端**：实现路由反查、幂等判新、错误码标准化 。
4. **客户端**：按 push hint + pull data 做可见范围增量刷新；实现 conditional GET。
5. **可观测性**：补齐 hash 命中率、Git 刷新耗时、工作区变更推送频率等指标。

## 10. 版本历史

### 2.1 相比 2.0 的改进

1. **消息封装简化**：移除 `version` 字段，版本通过 `connect.init.protocolVersion` 协商；`type=event` 不携带 `requestId`。

2. **`connectionEpoch` 机制**：Registry 在 `connect.init` 响应中分配全局单调递增整数，`reportProjects` 回传防止旧连接覆盖。

3. **错误体系标准化**：统一错误 JSON 结构（`code` + `message` + `details`），补齐完整错误码表。

4. **`fs.list` 极简化**：条目仅返回 `{name, kind}`，不分页；目录 `hash` 基于 `kind|name` 仅反映条目增删，不含文件内容 hash。

5. **`fs.info` + `fs.read` 统一寻址**：新增 `fs.info` 查询路径元信息（文件类型、大小、总行数等）；`fs.read` 请求/响应字段统一为 `offset`/`count`/`returned`/`total`/`hasMore`，文本按行寻址、二进制按字节寻址，语义由 `fs.info` 返回的 `isBinary` 决定。

6. **`knownHash` 协商（Conditional GET）**：有缓存则携带 `knownHash`，无缓存则不传；文件分段仅为传输优化，不引入一致性机制。

7. **共享 ProjectObject**：统一 Hub 上报与 Client 查询的 project 结构，补齐 `agent`/`imType` 字段。

8. **新增方法**：`fs.info`、`git.refs`、`git.diff`/`git.diff.fileDiff`、`git.workingTree.fileDiff`、`git.status` 枚举补全、`project.syncCheck`、`fs.grep`、`batch`。

9. **连接管理增强**：`project.offline`/`project.online`/`connection.closing` 事件；方法白名单；Client hub 作用域模型。

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
   - 目录 hash 规则收敛为 `kind|name`（仅覆盖直接子项名称与类型）。

6. 同步模式标准化
   - 固化方案 C：`push hint + pull data`。
   - 通过 `project.changed` / `git.workspace.changed` 事件提示，再由客户端按可见范围按需拉取。

7. 安全与可观测性补齐
   - 补齐统一错误码规范（含 `FORBIDDEN/NOT_FOUND/UNAVAILABLE/RATE_LIMITED/TIMEOUT`）。
   - 增加连接级限速、重连退避与审计字段要求。
   - 当前版本不强制协议层 `wss`，由后续网络安全专项推进。


