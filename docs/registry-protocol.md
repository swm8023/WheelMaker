# WheelMaker Registry Protocol 2.0

本文定义 WheelMaker Registry 2.0 协议，覆盖 Registry、Hub、Client 的连接、认证、路由与同步行为。

## 1. 目标与范围

- 以 `project` 为同步单元，支持多 Hub、多 Project。
- 客户端只刷新可见范围（展开目录、打开/Pin 文件、当前 Git 视图）。
- 文件系统接口支持 hash 协商，减少重复传输。
- Git 列表按版本触发刷新，不做 hash 协商。
- 协议仅描述 2.0 语义，不包含旧版本兼容分支。

## 2. 核心模型

### 2.1 标识规则

- Hub 上报主键：`projects[].name`。
- 对客户端暴露：`projectId = hubId + ":" + projectName`。
- 客户端后续查询仅使用 `projectId`。

### 2.2 版本状态

每个 project 维护：

- `projectRev`：项目聚合版本戳（当前仅由 Git 侧状态派生）。
- `gitRev`：Git 版本戳（建议 `branch + headSha + dirty` 归一后哈希）。
- `headSha`：当前分支 HEAD。
- `dirty`：是否存在未提交改动。
- `worktreeRev`：工作区状态版本戳（由 `git status --porcelain` 归一生成）。

## 3. 通用连接与握手认证（Hub/Client 共享）

### 3.1 通用消息封装

```json
{
  "version": "1.0",
  "requestId": "req-123",
  "type": "request|response|error|event",
  "method": "connect.init|...",
  "projectId": "optional",
  "payload": {}
}
```

### 3.2 `connect.init`（握手与认证合并）

请求：

```json
{
  "version": "1.0",
  "requestId": "req-init",
  "type": "request",
  "method": "connect.init",
  "payload": {
    "clientName": "wm-web",
    "clientVersion": "0.1.0",
    "protocolVersion": "1.0",
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
  "version": "1.0",
  "requestId": "req-init",
  "type": "response",
  "method": "connect.init",
  "payload": {
    "ok": true,
    "principal": {
      "role": "client",
      "hubId": "local-hub"
    },
    "serverInfo": {
      "serverVersion": "x.y.z",
      "protocolVersion": "1.0"
    },
    "features": {
      "hubReportProjects": true,
      "pushHint": true,
      "pingPong": true,
      "supportsHashNegotiation": true
    },
    "hashAlgorithms": ["sha256"]
  }
}
```

### 3.3 校验与安全约束

- `role` 必填，仅允许 `hub` 或 `client`。
- `role=hub` 时 `hubId` 必填；`role=client` 时可省略，由 token 作用域绑定。
- `token` 必填，必须在 `connect.init` 中携带。
- 所有业务方法必须在 `connect.init.ok=true` 后调用。
- 失败响应统一为 `UNAUTHORIZED`，且认证失败后立即断连。
- 建议 `connect.init` 使用 `ts + nonce`，服务端做时间窗校验与 nonce 去重。

### 3.4 方法白名单

- `role=hub`：`registry.reportProjects`、`registry.updateProject`、`hub.ping`。
- `role=client`：`project.list`、`fs.*`、`git.*`。
- 方法与角色不匹配返回 `FORBIDDEN`。

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

## 4. Hub 侧协议

### 4.1 Hub 连接生命周期

1. 建立 WebSocket（`ws://host:port/ws` 或 `wss://...`）。
2. 调用 `connect.init`。
3. 调用 `registry.reportProjects` 发送全量快照。
4. steady 状态下按需调用 `registry.updateProject`。
5. 连接失活后自动重连，重连成功后重新发送全量快照。

### 4.2 `registry.reportProjects`（全量覆盖）

请求：

```json
{
  "version": "1.0",
  "requestId": "req-report-1",
  "type": "request",
  "method": "registry.reportProjects",
  "payload": {
    "hubId": "local-hub",
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

- 语义为“当前全量覆盖”，不是 patch。
- 仅接受已认证 hub 连接调用。
- 必须按连接实例区分同 `hubId`，防止旧连接断开误删新映射。

### 4.3 `registry.updateProject`（单项目刷新）

说明：用于某一个 project 状态变化时定点刷新；其 `project` 对象与全量上报中的项目字段保持一致。

请求：

```json
{
  "version": "1.0",
  "requestId": "req-update-1",
  "type": "request",
  "method": "registry.updateProject",
  "payload": {
    "hubId": "local-hub",
    "connectionEpoch": 7,
    "seq": 1024,
    "project": {
      "name": "WheelMaker",
      "path": "D:/Code/WheelMaker",
      "online": true,
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
- 发起业务请求前若连接不可用，先重连并重新执行 `connect.init`。

### 5.2 `project.list`

前置条件：已完成 `connect.init` 且连接已绑定 hub 作用域。

请求：

```json
{
  "version": "1.0",
  "requestId": "req-project-list-1",
  "type": "request",
  "method": "project.list",
  "payload": {
    "includeStats": true
  }
}
```

响应：

```json
{
  "version": "1.0",
  "requestId": "req-project-list-1",
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

### 5.3 `fs.list`（目录）

请求：

```json
{
  "version": "1.0",
  "requestId": "req-fs-list-1",
  "type": "request",
  "method": "fs.list",
  "projectId": "local-hub:WheelMaker",
  "payload": {
    "path": "docs",
    "knownHash": "sha256:prev-docs-hash",
    "visibleOnly": true,
    "limit": 200,
    "cursor": ""
  }
}
```

响应（命中未变化）：

```json
{
  "version": "1.0",
  "requestId": "req-fs-list-1",
  "type": "response",
  "method": "fs.list",
  "projectId": "local-hub:WheelMaker",
  "payload": {
    "path": "docs",
    "snapshotHash": "sha256:prev-docs-hash",
    "notModified": true,
    "entries": [],
    "nextCursor": ""
  }
}
```

响应（有变化）：

```json
{
  "version": "1.0",
  "requestId": "req-fs-list-1",
  "type": "response",
  "method": "fs.list",
  "projectId": "local-hub:WheelMaker",
  "payload": {
    "path": "docs",
    "snapshotHash": "sha256:new-docs-hash",
    "notModified": false,
    "entries": [
      { "name": "registry-protocol.md", "path": "docs/registry-protocol.md", "kind": "file", "size": 21459, "mtime": "2026-03-30T09:20:00Z", "dataHash": "sha256:..." },
      { "name": "superpowers", "path": "docs/superpowers", "kind": "dir", "size": 0, "mtime": "2026-03-30T09:10:00Z", "dataHash": "" }
    ],
    "nextCursor": ""
  }
}
```

### 5.4 `fs.read`（文件）

请求：

```json
{
  "version": "1.0",
  "requestId": "req-fs-read-1",
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

响应：

```json
{
  "version": "1.0",
  "requestId": "req-fs-read-1",
  "type": "response",
  "method": "fs.read",
  "projectId": "local-hub:WheelMaker",
  "payload": {
    "path": "docs/registry-protocol.md",
    "contentHash": "sha256:new-content-hash",
    "notModified": false,
    "content": "...",
    "encoding": "utf-8",
    "eof": true,
    "nextOffset": 65536
  }
}
```

### 5.5 `fs.search`（文件名模糊查找）

- 首版仅支持文件名模糊匹配，不做内容检索。

请求：

```json
{
  "version": "1.0",
  "requestId": "req-fs-search-1",
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
  "version": "1.0",
  "requestId": "req-fs-search-1",
  "type": "response",
  "method": "fs.search",
  "projectId": "local-hub:WheelMaker",
  "payload": {
    "results": [
      { "path": "docs/registry-protocol.md", "name": "registry-protocol.md", "score": 0.97 }
    ],
    "nextCursor": ""
  }
}
```

### 5.6 Git 只读接口

- `git.branches`

```json
{
  "version": "1.0",
  "requestId": "req-git-branches-1",
  "type": "request",
  "method": "git.branches",
  "projectId": "local-hub:WheelMaker",
  "payload": {}
}
```

```json
{
  "version": "1.0",
  "requestId": "req-git-branches-1",
  "type": "response",
  "method": "git.branches",
  "projectId": "local-hub:WheelMaker",
  "payload": {
    "current": "main",
    "branches": ["main", "feature/x"]
  }
}
```

- `git.log`

```json
{
  "version": "1.0",
  "requestId": "req-git-log-1",
  "type": "request",
  "method": "git.log",
  "projectId": "local-hub:WheelMaker",
  "payload": {
    "ref": "main",
    "cursor": "",
    "limit": 50
  }
}
```

- `git.commit.files`

```json
{
  "version": "1.0",
  "requestId": "req-git-files-1",
  "type": "request",
  "method": "git.commit.files",
  "projectId": "local-hub:WheelMaker",
  "payload": {
    "sha": "abc123"
  }
}
```

- `git.commit.fileDiff`

```json
{
  "version": "1.0",
  "requestId": "req-git-diff-1",
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

- `git.status`（工作区未提交改动视图）

```json
{
  "version": "1.0",
  "requestId": "req-git-status-1",
  "type": "request",
  "method": "git.status",
  "projectId": "local-hub:WheelMaker",
  "payload": {}
}
```

```json
{
  "version": "1.0",
  "requestId": "req-git-status-1",
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

规则：

- Git 列表按 `headSha/gitRev` 变化触发刷新。
- `git.commit.files` 以 `sha` 为缓存键。
- `git.commit.fileDiff` 以 `sha+path+contextLines` 为缓存键。
- Git 列表不做 `knownHash/notModified` 协商。

## 6. Hash 规范

### 6.1 目录 hash

- 输入项：`kind|name|dataHash`
- `kind=dir` 时 `dataHash` 为空字符串。
- `kind=file` 时 `dataHash` 为文件内容 hash。
- 目录下 entry 先按 `kind,name` 排序，再拼接后 `sha256`。

### 6.2 文件 dataHash

- `sha256(raw bytes)`

## 7. 同步策略（方案 C）

采用 `push hint + pull data`：

1. 服务端推送事件摘要（不推全量数据）。
2. 客户端收到后按可见范围拉取明细。

事件示例：

`project.changed`：

```json
{
  "version": "1.0",
  "type": "event",
  "method": "project.changed",
  "projectId": "local-hub:WheelMaker",
  "payload": {
    "projectRev": "sha256:...",
    "gitRev": "sha256:...",
    "worktreeRev": "sha256:...",
    "changedDomains": ["git", "worktree"]
  }
}
```

`git.workspace.changed`：

```json
{
  "version": "1.0",
  "type": "event",
  "method": "git.workspace.changed",
  "projectId": "local-hub:WheelMaker",
  "payload": {
    "dirty": true,
    "worktreeRev": "sha256:..."
  }
}
```

同步约束：

- 事件是提示（hint），不是状态真值；客户端必须以拉取结果为准。
- 同一个 `projectId` 上，客户端应仅保留最后一个待处理事件（可合并去抖）。
- 处理超时或失败时可重试，重试失败回退到“按 `project.list` + 可见范围全量重拉”。

## 8. 客户端增量刷新规则

1. 触发源：
- 收到 `project.changed`/`git.workspace.changed`。
- 或轮询发现 `projectRev/gitRev/worktreeRev` 与本地缓存不一致。

2. 刷新顺序（同一 project）：
- 先刷新 `project.list` 中该 project 的元信息缓存。
- 再按 `changedDomains` 分流：
  - 包含 `git`：拉 `git.log`，并按当前选中项继续拉 `git.commit.files`/`git.commit.fileDiff`。
  - 包含 `worktree`：拉 `git.status`。
  - 包含 `fs` 或未提供 `changedDomains`：按可见范围执行 `fs.list/fs.read`。

3. 可见范围拉取规则（FS）：
- 已展开目录：`fs.list(path, knownHash)`。
- 当前打开文件和 Pin 文件：`fs.read(path, knownHash)`。
- 非可见目录与文件不主动拉取。

4. 幂等与去重：
- 对同一路径并发请求做合并（同 key 仅保留 1 个在途请求）。
- 响应 `notModified=true` 时仅更新时间戳，不刷新内容缓存。
- 新响应版本戳早于本地已处理版本时丢弃（防止乱序覆盖）。

## 9. 实施建议

1. 协议收敛：统一 `project.list`；保留 `reportProjects + updateProject` 双上报。
2. Hub 侧：接入 `projectRev/gitRev/headSha/dirty/worktreeRev` 即时上报。
3. 服务端：实现 `fs.search`、`git.status`、路由反查与幂等判新。
4. 客户端：按方案 C 做可见范围增量刷新。
5. 可观测性：补齐 hash 命中率、Git 刷新耗时、工作区变更推送频率等指标。

## 10. 2.0 相比 1.0 的修正内容

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
