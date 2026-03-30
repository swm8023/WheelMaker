# WheelMaker Registry Protocol 2.0

本文为 2.0 协议规范，描述 Registry、Hub、Client 的统一通信与同步行为。

## 1. 目标

- 以 `project` 为同步单元，支持多 Hub、多 Project。
- Hub 在 Git 分支或版本（HEAD）变化时主动汇报 Registry。
- 客户端仅刷新可见范围（展开目录、打开/Pin 文件、当前 Git 视图）。
- 文件系统接口支持 hash 协商，减少重复传输。
- Git 列表不做 hash 协商：以 `headSha/gitRev` 变化作为拉取触发条件。

## 2. 范围

- 协议版本固定为 `2.0`，不包含旧版本兼容分支。
- 项目列表统一使用 `project.list`，不使用 `project.listFull`。
- 握手与认证合并为 `connect.init` 单次初始化请求。
- Git 列表按版本触发刷新，不做 hash 协商；FS 接口支持 hash 协商。

## 3. 标识与状态模型

标识规则（固定为新版，不做兼容分支）：

- Hub -> Registry 上报：`projects[].name` 作为 Hub 侧项目标识主键。
- Registry -> Client 返回：`projectId = hubId + ":" + projectName`（固定规则）。
- 设计目标：客户端能看到 `projectId -> hubId` 归属关系；后续查询通信只使用 `projectId`。

每个 Project 维护：

- `projectRev`：项目聚合版本戳（当前阶段仅由 Git 侧状态派生，不做全局 FS 主动监测）
- `gitRev`：Git 版本戳（建议 `branch + headSha + dirty` 归一后哈希）
- `headSha`：当前分支 HEAD
- `dirty`：是否有未提交改动
- `worktreeRev`：工作区状态版本戳（由 `git status --porcelain` 归一生成）

## 4. Hub -> Registry 汇报增强

Hub -> Registry 仅定义两类项目上报协议：

- `registry.reportProjects`：全量快照覆盖（用于初始化与重连恢复）
- `registry.updateProject`：单项目更新（用于某个 project 状态变化时的定点刷新）

两类协议的项目基本项保持一致，统一为 `project` 对象：

- `name`（Hub 侧项目名，作为上报主键）
- `path`
- `online`
- `git.branch`
- `git.headSha`
- `git.dirty`
- `git.gitRev`
- `git.worktreeRev`
- `projectRev`

## 4.1 Hub <-> Registry 连接机制（保活与重连）

参考 `docs/feishu-bot.md` 的长连接策略，Hub 与 Registry 采用“长连接 + 心跳 + 自动重连”。

连接流程：

1. Hub 建立 WebSocket 到 Registry（`ws://host:port/ws` 或 `wss://...`）。
2. 发送 `connect.init`（携带 `protocolVersion`、连接身份与统一 token）。
3. 发送 `registry.reportProjects` 全量快照，等待 ACK。
4. 进入 steady 状态：保活 + 按需发送 `registry.updateProject`。

保活规则：

- 仅 Hub 连接要求使用 ping/pong（见 `4.2.3 hub.ping / hub.pong`）。
- 建议 Hub 每 `15s` 发送 ping，`45s` 无 pong 判定连接失活。
- 失活后立即断开并进入重连流程。
- 保活失败不应阻塞本地项目运行，只影响远程观察能力。

重连规则：

- 默认自动重连开启（无限重试）。
- 基础重试间隔：`2s`。
- 抖动：首次重连随机 `0~30s`，后续指数退避上限 `60s`。
- 每次重连成功后，必须重新发送一次全量 `registry.reportProjects` 以重建 Registry 映射。
- 对不可重试错误（如鉴权失败）停止自动重连并上报错误状态。

幂等与一致性：

- `registry.reportProjects` 语义为“当前全量覆盖”，不是增量 patch。
- Registry 以“最后一次成功汇报”为准。
- 旧连接断开不应清理新连接的同 `hubId` 映射（需按连接实例区分）。

可观测性：

- 记录连接状态变更日志：`connected/disconnected/retrying/init_failed`。
- 暴露指标：`reconnect_count`、`last_report_at`、`heartbeat_timeout_count`、`report_ack_latency_ms`。

## 4.2 Hub -> Registry 协议定义（报文级）

本节定义 Hub 与 Registry 之间的协议报文，便于直接实现与抓包对照。

通用封装：

```json
{
  "version": "1.0",
  "requestId": "req-123",
  "type": "request|response|error|event",
  "method": "connect.init|registry.reportProjects|registry.updateProject|ping",
  "payload": {}
}
```

### 4.2.1 connect.init（必选，握手与认证合并）

连接方请求（Hub/Client 通用）：

```json
{
  "version": "1.0",
  "requestId": "req-init",
  "type": "request",
  "method": "connect.init",
  "payload": {
    "clientName": "wheelmaker-hub",
    "clientVersion": "0.1.0",
    "protocolVersion": "1.0",
    "role": "hub",
    "hubId": "local-hub",
    "token": "******",
    "ts": 1777777777,
    "nonce": "a1b2c3d4"
  }
}
```

Registry 响应：

```json
{
  "version": "1.0",
  "requestId": "req-init",
  "type": "response",
  "method": "connect.init",
  "payload": {
    "ok": true,
    "principal": {
      "role": "hub",
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

connect.init 校验规则（强约束）：

- `role` 必填，取值仅允许 `hub` 或 `client`。
- `role=hub` 时，`hubId` 必填；`role=client` 时可省略，按 token 作用域绑定。
- `token` 必填，必须在 `connect.init` 中携带。
- 所有业务方法必须在 `connect.init.ok=true` 后调用。
- `role=hub` 仅允许调用 `registry.reportProjects`、`registry.updateProject`、`ping`。
- `role=client` 仅允许调用 `project.list`、`fs.*`、`git.*`。
- 若调用方法与 `role` 不匹配，返回 `FORBIDDEN`。

connect.init 失败与防探测约束：

- `connect.init` 失败响应统一为 `UNAUTHORIZED`，避免暴露枚举信息。
- 认证失败后应立即关闭连接，不保留半认证会话。
- 认证失败不返回详细失败原因（例如 token 不存在/过期/签名错误不区分）。

### 4.2.2 registry.reportProjects（核心）

Hub 请求（全量覆盖）：

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
        "agent": "codex",
        "imType": "feishu",
        "git": {
          "branch": "main",
          "headSha": "abc123...",
          "dirty": true,
          "gitRev": "sha256:...",
          "worktreeRev": "sha256:..."
        },
        "projectRev": "sha256:..."
      }
    ]
  }
}
```

Registry ACK：

```json
{
  "version": "1.0",
  "requestId": "req-report-1",
  "type": "response",
  "method": "registry.reportProjects",
  "payload": {
    "hubId": "local-hub",
    "projectCount": 1
  }
}
```

约束：

- `registry.reportProjects` 是“全量快照覆盖”语义，不是 patch。
- Registry 必须以连接实例区分同 `hubId`，防止旧连接断开误删新映射。

### 4.2.3 hub.ping / hub.pong（仅 Hub 保活）

Hub ping：

```json
{
  "version": "1.0",
  "requestId": "req-ping-1",
  "type": "request",
  "method": "ping",
  "payload": {
    "ts": 1777777777
  }
}
```

Registry pong：

```json
{
  "version": "1.0",
  "requestId": "req-ping-1",
  "type": "response",
  "method": "ping",
  "payload": {
    "ts": 1777777777,
    "ok": true
  }
}
```

### 4.2.4 error

统一错误报文：

```json
{
  "version": "1.0",
  "requestId": "req-init",
  "type": "error",
  "error": {
    "code": "UNAUTHORIZED|FORBIDDEN|NOT_FOUND|UNAVAILABLE|INVALID_ARGUMENT|CONFLICT|RATE_LIMITED|TIMEOUT|INTERNAL",
    "message": "human readable",
    "details": {}
  }
}
```

### 4.2.5 registry.updateProject（Hub -> Registry 单项目更新）

用途：当某个 project 状态变化时，仅刷新该 project，避免全量重报。

Hub 请求（单项目）：

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
    "updatedAt": "2026-03-29T13:00:00Z"
  }
}
```

Registry ACK：

```json
{
  "version": "1.0",
  "requestId": "req-update-1",
  "type": "response",
  "method": "registry.updateProject",
  "payload": {
    "hubId": "local-hub",
    "accepted": true
  }
}
```

约束：

- 仅允许已通过 Hub 认证的连接调用。
- `project.name` 必须属于该 `hubId` 当前映射，否则返回 `INVALID_ARGUMENT`。
- 若 Registry 未命中该 project 映射，可要求 Hub 回退发送一次全量 `registry.reportProjects`。
- 定时心跳周期仍建议发送全量快照做纠偏（例如 30~60s）。
- 每个 `(hubId, project.name)` 维护最新 `(connectionEpoch, seq)`，仅接受更“新”的更新，防止乱序覆盖。
- 当 `seq` 回退或重复时可直接忽略，并在日志记录 `stale_project_update`。

服务端判新规则（伪代码）：

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

## 4.3 连接身份与统一 Token 模型

Registry 必须显式区分两类连接：

- `hub` 连接：Hub -> Registry，负责 `registry.reportProjects`、`registry.updateProject` 与项目请求响应。
- `client` 连接：App/Web -> Registry，负责 `project.list/fs/git` 查询。

必须在 `connect.init` 中声明连接角色并携带 token：

```json
{
  "type": "request",
  "method": "connect.init",
  "payload": {
    "clientName": "wm-web",
    "protocolVersion": "1.0",
    "role": "client",
    "token": "******"
  }
}
```

或：

```json
{
  "type": "request",
  "method": "connect.init",
  "payload": {
    "clientName": "wheelmaker-hub",
    "protocolVersion": "1.0",
    "role": "hub",
    "hubId": "local-hub",
    "token": "******"
  }
}
```

### 4.3.1 统一 Token（Hub 与 Client 共用）

- 用途：统一认证与授权，Hub 与 Client 都使用同一类 token 机制。
- 校验模型：
  - token 绑定 `hubId`（必选）
  - 可选绑定 `projectIds[]`（更细粒度 ACL）
  - 可选过期时间与签名（JWT 或 HMAC）
- 失败结果：`UNAUTHORIZED`，并可终止连接。

示例 claims（概念）：

```json
{
  "sub": "client-user-123",
  "hubId": "local-hub",
  "projectIds": ["local-hub:WheelMaker"],
  "exp": 1777777777
}
```

### 4.3.2 请求鉴权顺序（客户端）

1. Registry 校验 client token 签名/过期。
2. 提取 token 中允许的 `hubId/projectIds`。
3. 根据请求中的 `projectId` 做路由反查（见 4.4）。
4. 若 `projectId` 对应 hub 不在授权范围，返回 `FORBIDDEN`。

强约束：

- client 在完成 `connect.init` 前，不允许调用 `project.list`、`fs.*`、`git.*`。
- client token 必须带明确 `hubId` 作用域；未绑定 hub 的 token 视为无效。

### 4.3.3 防探测与抗重放要求

- 连接级限速：按 `IP + role + hubId` 维度限制握手失败频率，超过阈值返回 `RATE_LIMITED`。
- 重连退避：认证失败场景要求指数退避（建议 `2s -> 4s -> 8s`，上限 `60s`）。
- 抗重放：`connect.init` 请求建议携带 `ts + nonce`，服务端校验时间窗并去重 `nonce`。
- token 约束：必须校验 `exp`、签名与 `hubId/projectIds` 作用域绑定。
- 审计：记录 `UNAUTHORIZED/FORBIDDEN/RATE_LIMITED`，包含 `requestId/remoteAddr/role/hubId`。
- 本阶段不强制协议层 `wss` 要求（由后续网络安全专项统一推进）。

## 4.4 projectId -> hub 反查与路由规则

Registry 维护映射：

- `projectToHub[projectId] = hubId`
- `hubPeers[hubId] = peerConn`

来源：

- 仅由 Hub 的最新一次 `registry.reportProjects` 全量覆盖更新。

路由规则：

1. client 请求带 `projectId`。
2. Registry 先从 `projectId` 按分隔符 `:` 解析出 `hubId`。
3. 再与 `projectToHub[projectId]` 做一致性校验（防伪造/脏数据）。
4. 用 `hubPeers[hubId]` 找在线连接并转发。
5. 若任一步失败，返回：
   - `NOT_FOUND`：projectId 不存在
   - `UNAVAILABLE`：project 存在但 hub 不在线
   - `FORBIDDEN`：client token 无权访问该 hub/project

一致性要求：

- 同 `hubId` 新连接注册后，旧连接断开不得清理新映射（按连接实例校验）。
- `project.list` 必须返回 `hubId`（用于客户端展示项目所属 Hub）；后续数据请求仍只使用 `projectId`。
- `projectName` 不允许包含分隔符 `:`；如需支持特殊字符，必须先做编码（如 URL encode）后再拼接 `projectId`。

## 4.5 错误码规范（统一）

为避免实现分歧，Hub/Client 与 Registry 的错误码统一如下：

- `UNAUTHORIZED`：认证失败或 token 缺失/无效。
- `FORBIDDEN`：已认证，但无权访问目标 hub/project。
- `NOT_FOUND`：`projectId` 不存在（无映射）。
- `UNAVAILABLE`：project 存在，但对应 hub 当前不在线。
- `INVALID_ARGUMENT`：请求参数非法（字段缺失、格式错误、project 不属于该 hub 等）。
- `CONFLICT`：状态冲突（可选，用于版本竞争场景）。
- `RATE_LIMITED`：触发限流。
- `TIMEOUT`：转发或处理超时。
- `INTERNAL`：服务端内部错误。

建议：

- 错误响应 `details` 至少包含 `projectId`、`hubId`（若可确定）与 `requestId`。
- 对 `UNAUTHORIZED/FORBIDDEN/RATE_LIMITED` 进行安全审计日志记录。

## 5. Client <-> Registry 协议

### 5.1 connect.init（客户端视角，权威定义见 4.2.1）

客户端不要求 ping/pong；在发起业务请求前若发现连接不可用，应先重连并重新执行 `connect.init`。

客户端发送 `connect.init`，携带身份与 token。

服务端在 `connect.init` 成功后返回能力细节：

- `pushHintEnabled`
- `hashAlgorithms`（用于 FS 接口）

```json
{
  "type": "request",
  "method": "connect.init",
  "payload": {
    "clientName": "wm-web",
    "protocolVersion": "1.0",
    "role": "client",
    "token": "client-hub-token"
  }
}
```

校验成功后，当前连接获得该 token 对应的 `hubId/projectIds` 访问范围。
若 token 缺失、无效或与目标作用域不匹配，返回 `UNAUTHORIZED`。

### 5.2 project.list（唯一项目列表）

前置条件：必须先完成 `5.1 connect.init`，且当前连接已绑定一个 hub。
返回范围：仅返回该 hub 下可访问的 projects。

请求：

```json
{
  "type": "request",
  "method": "project.list",
  "payload": {
    "includeStats": true
  }
}
```

响应关键字段（示意）：

```json
{
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

### 5.3 fs.list（目录增量）

请求字段：

- `knownHash`
- `visibleOnly`（默认 true）

响应字段：

- `snapshotHash`
- `notModified`

### 5.4 fs.read（文件增量）

请求字段：

- `knownHash`

响应字段：

- `contentHash`
- `notModified`

### 5.5 Git 接口策略（按版本触发，不做 hash 协商）

- `git.log`：当 `headSha` 或 `gitRev` 变化时客户端主动拉取。
- `git.commit.files`：由 `sha` 唯一标识，按 `sha` 缓存。
- `git.commit.fileDiff`：按 `sha+path+contextLines` 缓存。

备注：Git 列表不需要 `knownHash/notModified`。

### 5.6 fs.search（文件名模糊查找）

仅支持文件名字符串模糊匹配（首版不做内容检索）。

请求：

```json
{
  "type": "request",
  "method": "fs.search",
  "projectId": "local-hub:WheelMaker",
  "payload": {
    "query": "reg serv",
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
  "type": "response",
  "method": "fs.search",
  "projectId": "local-hub:WheelMaker",
  "payload": {
    "results": [
      {
        "path": "docs/registry-server-remote-observe-protocol-v1.md",
        "name": "registry-server-remote-observe-protocol-v1.md",
        "score": 0.93
      }
    ],
    "nextCursor": ""
  }
}
```

### 5.7 工作区未提交改动监控

查询接口 `git.status`：

```json
{
  "type": "request",
  "method": "git.status",
  "projectId": "local-hub:WheelMaker",
  "payload": {}
}
```

响应示例：

```json
{
  "type": "response",
  "method": "git.status",
  "projectId": "local-hub:WheelMaker",
  "payload": {
    "branch": "main",
    "headSha": "abc123...",
    "dirty": true,
    "worktreeRev": "sha256:...",
    "staged": [{"path":"server/internal/registry/server.go","status":"M"}],
    "unstaged": [{"path":"docs/registry-server-remote-observe-protocol-v1.md","status":"M"}],
    "untracked": [{"path":"tmp/new.txt","status":"?"}]
  }
}
```

## 6. Hash 规范（按你要求调整）

仅用于文件系统同步。

## 6.1 目录 hash

目录项按 `kind,name,dataHash` 参与计算：

- `kind=dir`：`dataHash` 可为空字符串
- `kind=file`：`dataHash=文件内容 hash`

建议输入串：

`kind|name|dataHash`

目录下所有 entry 按 `kind,name` 排序后拼接，再 `sha256`。

## 6.2 文件 dataHash

`sha256(raw bytes)`。

## 7. 同步策略（采用方案 C，并写入协议）

采用 `push hint + pull data`：

1. 服务端推送事件摘要（不推全量数据）
2. 客户端收到后，仅拉取可见范围明细

事件：

```json
{
  "type": "event",
  "method": "project.changed",
  "projectId": "local-hub:WheelMaker",
  "payload": {
    "projectRev": "sha256:...",
    "gitRev": "sha256:...",
    "worktreeRev": "sha256:...",
    "changedDomains": ["git","worktree"]
  }
}
```

补充事件（专门给未提交改动）：

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

## 8. 客户端增量刷新规则

1. 收到 `project.changed` 或轮询发现 `projectRev/gitRev/worktreeRev` 变化
2. Git 刷新：
- `headSha`/`gitRev` 变化：拉 `git.log`
- 选中 commit 变化：拉 `git.commit.files`
- 选中 diff 变化：拉 `git.commit.fileDiff`
- `worktreeRev` 变化：拉 `git.status`
3. FS 刷新（按需，不主动全盘监测）：
- 对已展开目录调用 `fs.list(path, knownHash)`
- 对打开/Pin 文件调用 `fs.read(path, knownHash)`

## 9. 下一步实施计划（附在文档末尾）

1. 协议收敛
- 下线 `project.listFull`，统一 `project.list`。

2. Hub 汇报增强
- 汇报 `gitRev/headSha/dirty/worktreeRev/projectRev`。
- Git/工作区变化即时上报。
- `projectRev` 由 Git 侧版本派生，不引入全量 FS watcher。

3. 服务端接口（第一阶段必须完成）
- `fs.search`（文件名模糊匹配）
- `git.status`（未提交改动视图）

4. 客户端同步改造
- 采用方案 C：监听 `project.changed` / `git.workspace.changed`。
- Git 用版本触发拉取；FS 用 hash 协商。

5. 可观测性
- 增加指标：FS 按需 hash 命中率、Git 刷新耗时、工作区状态推送频率。

## 10. 2.0 ?? 1.0 ?????

1. ?????????
- ? `hello` + `auth` ??????? `connect.init`?
- ????????? `role/hubId/token`,??? `ts+nonce` ????
- ????????? `UNAUTHORIZED`,???????????

2. ?????????
- ???? `hub` ? `client` ?????
- ?? `projectId = hubId + ":" + projectName`?
- Registry ?? `projectId -> hubId` ???????,???????

3. ???????????
- ???? `project.list`,?? `project.listFull`?
- `project.list` ?? `hubId`?`projectRev` ? Git ????,???????????????

4. Git ??????
- ?? Git ???????,?? hash ???
- ?? `gitRev/headSha/dirty/worktreeRev/projectRev` ???????
- ?? `git.status` ? `git.workspace.changed`,??????????

5. ??????????
- `fs.list` ? `fs.read` ?? `knownHash/notModified` ??,???????
- ?? hash ????? `kind|name|dataHash`(??? `dataHash`,???????)?

6. ???????
- ???? C:`push hint + pull data`?
- ?? `project.changed` / `git.workspace.changed` ????,???????????????

7. ?????????
- ?????????(? `FORBIDDEN/NOT_FOUND/UNAVAILABLE/RATE_LIMITED/TIMEOUT`)?
- ????????????????????
- ?????????? `wss`,????????????
