# WheelMaker Registry Protocol 2.2

本文是 Registry 协议唯一权威文档。

## 1. 版本策略

- 当前协议版本：`2.2`
- 本版本为**硬切换**：不提供 `2.1` 及更早版本兼容。
- `connect.init.payload.protocolVersion` 必须为 `"2.2"`，否则返回 `INVALID_ARGUMENT`。

## 2. 总体模型

- `projectId = hubId + ":" + projectName`
- 业务请求统一通过 `projectId` 路由到目标 hub。
- 会话主模型为 `session.*`，不再使用 `chat.session.*`。
- 同步模型为**客户端主动拉取（pull-only）**，不再依赖 changed 推送。

## 3. 消息封装

```json
{
  "requestId": 1,
  "type": "request|response|error|event",
  "method": "...",
  "projectId": "optional",
  "payload": {}
}
```

约束：

- `type=event` 不带 `requestId`
- `requestId` 必须是 `>=1` 的整数
- 同一连接内 `requestId` 不能重复

## 4. connect.init

请求示例：

```json
{
  "requestId": 1,
  "type": "request",
  "method": "connect.init",
  "payload": {
    "clientName": "wm-web",
    "clientVersion": "0.1.0",
    "protocolVersion": "2.2",
    "role": "client",
    "hubId": "optional",
    "token": "***"
  }
}
```

成功响应关键字段：

- `principal.role / principal.hubId / principal.connectionEpoch`
- `serverInfo.protocolVersion = "2.2"`
- `features.pushHint = false`

## 5. 方法白名单

### 5.1 hub

- `registry.reportProjects`
- `registry.updateProject`
- `registry.session.updated`
- `registry.session.message`
- `hub.ping`

### 5.2 client

- `project.list`
- `project.syncCheck`
- `batch`
- `session.*`
- `chat.send`
- `chat.permission.respond`
- `fs.*`
- `git.*`

说明：

- `chat.session.list` / `chat.session.read` 已移除。
- `registry.chat.message` 已移除。

### 5.3 monitor

- `project.list`
- `monitor.listHub`
- `monitor.status`
- `monitor.log`
- `monitor.db`
- `monitor.action`
- `batch`

## 6. Project 与同步

### 6.1 project.list

返回项目快照（含 `projectRev/gitRev/worktreeRev`）。

### 6.2 project.syncCheck

客户端用本地版本戳探测是否过期：

请求：`knownProjectRev/knownGitRev/knownWorktreeRev`

响应：`projectRev/gitRev/worktreeRev/staleDomains`

`staleDomains` 当前域值：

- `project`
- `git`
- `worktree`

## 7. 同步策略（Pull-Only）

### 7.1 设计原则

- Registry 不再发送 `project.changed` / `git.workspace.changed`。
- 客户端通过 `project.syncCheck` 主动判定是否拉取。
- 推荐客户端周期性轮询（例如 3 秒）+ 关键交互后主动 refresh。

### 7.2 仍保留的事件

- `project.online`
- `project.offline`
- `session.updated`
- `session.message`
- `connection.closing`

## 8. FS 协议

### 8.1 fs.list

- 目录一次性返回，不分页
- 支持 `knownHash/notModified`
- 目录 hash 仅覆盖直接子项 `kind|name`

### 8.2 fs.info

用于读取前探测路径信息：

- 文件：`kind/size/isBinary/mimeType/totalLines/hash`
- 目录：`kind/entryCount/hash`

### 8.3 fs.read（2.2 语义）

- **整文件返回**（文本与二进制都不分页）
- 支持 `knownHash/notModified`
- 文本：`encoding=utf-8`，`content` 为全文
- 二进制：`encoding=base64`，`content` 为完整 base64
- 不再使用流式/分段字段语义（`offset/count/hasMore`）

推荐客户端流程：

1. `fs.info`
2. 对大文件弹确认
3. 用户确认后 `fs.read`

## 9. Session 协议

请求方法：

- `session.list`
- `session.read`
- `session.new`
- `session.send`
- `session.markRead`

事件：

- `session.updated`
- `session.message`

## 10. Git 协议

### 10.1 git.refs

返回：

- `current` 当前分支
- `branches` 本地分支
- `remoteBranches` 远程分支
- `tags` 标签列表

### 10.2 git.log

- 按 `ref/refs` 拉取提交历史
- 与 `git.refs` 不重合：`git.refs` 是引用目录，`git.log` 是提交历史

## 11. 错误码

- `INVALID_ARGUMENT`
- `UNAUTHORIZED`
- `FORBIDDEN`
- `NOT_FOUND`
- `CONFLICT`
- `UNAVAILABLE`
- `TIMEOUT`
- `INTERNAL`

## 12. 2.2 破坏性变更清单

1. 协议版本强制为 `2.2`（无向下兼容）
2. 移除 `registry.chat.message`
3. 移除 `chat.session.list` / `chat.session.read`
4. 移除 `project.changed` / `git.workspace.changed` 推送依赖
5. `fs.read` 统一为整文件返回，二进制不再 `encoding=none`
6. `git.refs` 扩展返回 `remoteBranches`
