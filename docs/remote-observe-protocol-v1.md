# WheelMaker 远端查看架构与协议设计（V1 草案）

## 1. 目标与范围

本设计用于支持客户端远端查看 WheelMaker 项目状态，重点覆盖：

- 文件系统只读浏览：
  - 当前目录列表
  - 点选目录后继续展开
  - 点选文件后读取文件内容
- Git 只读浏览：
  - 分支列表
  - 指定分支提交记录
  - 指定提交改动文件列表
  - 按文件查看该提交 diff

约束：

- 仍以 `client`（project）为单位组织数据。
- 不走 IM 转发链路，改为网络连接到 WheelMaker 服务进程。
- 可连接非本机 WheelMaker（远端地址）。
- 客户端按点击触发请求，每次仅传必要信息。
- 客户端实现暂缓（后续可为 App 或 Web）。
- 安全方案先预留接口与扩展点。

## 2. 总体架构

### 2.1 组件

- `WheelMaker Server (Remote Observe Mode)`
  - 独立进程启动
  - 监听 TCP 端口（建议支持 TLS）
  - 暴露只读查询协议
- `Project Runtime (existing client model)`
  - 每个项目仍对应一个 client 上下文
  - 提供文件/Git查询能力
- `Remote Client`
  - 只负责按需请求与展示
  - 不持有业务状态源，只缓存 UI 所需数据

### 2.2 连接模型

- 长连接协议建议：`WebSocket + JSON`
- 单连接可访问多个 project（通过 `projectId` 显式指定）
- 请求-响应模型，支持并发请求（`requestId` 关联）
- 服务端可选推送（V1 可不启用，仅保留扩展）

## 3. 协议原则

- 最小必要数据：只返回当前 UI 需要的数据。
- 分页优先：提交记录和大目录必须支持分页/游标。
- 显式上下文：除全局发现类接口（如 `project.list`、`project.listFull`）和握手阶段外，每个请求必须带 `projectId`。
- 只读语义：V1 不提供写操作。
- 可演进：所有消息包含 `version`，接口可向后兼容扩展字段。

## 4. 传输与消息封装

### 4.1 通用消息结构

```json
{
  "version": "1.0",
  "requestId": "uuid-or-seq",
  "type": "request|response|error|event",
  "method": "fs.list|git.log|...",
  "projectId": "project-name",
  "payload": {}
}
```

### 4.2 错误结构

```json
{
  "version": "1.0",
  "requestId": "same-as-request",
  "type": "error",
  "error": {
    "code": "UNAUTHORIZED|NOT_FOUND|INVALID_ARGUMENT|INTERNAL|RATE_LIMITED",
    "message": "human readable",
    "details": {}
  }
}
```

## 5. 握手与会话（V1）

### 5.1 `hello`

请求：

```json
{
  "type": "request",
  "method": "hello",
  "payload": {
    "clientName": "wm-web",
    "clientVersion": "0.1.0",
    "protocolVersion": "1.0"
  }
}
```

响应：

```json
{
  "type": "response",
  "method": "hello",
  "payload": {
    "serverVersion": "x.y.z",
    "protocolVersion": "1.0",
    "features": {
      "fs": true,
      "git": true,
      "push": false
    }
  }
}
```

### 5.2 `auth`（预留）

- V1 可先支持匿名/单 token 模式。
- 后续扩展 mTLS / OAuth2 / 短期签名 token。

### 5.3 项目发现协议

### 5.3.1 项目列表：`project.list`

请求：

```json
{
  "type": "request",
  "method": "project.list",
  "payload": {}
}
```

响应：

```json
{
  "type": "response",
  "method": "project.list",
  "payload": {
    "projects": [
      { "projectId": "server", "name": "server", "online": true },
      { "projectId": "app", "name": "app", "online": true }
    ]
  }
}
```

### 5.3.2 申请所有 project 完整信息：`project.listFull`

用于一次性拉取所有 project 的基础元信息和能力信息，便于客户端初始化项目选择器与能力缓存。

请求：

```json
{
  "type": "request",
  "method": "project.listFull",
  "payload": {
    "includeStats": true
  }
}
```

响应：

```json
{
  "type": "response",
  "method": "project.listFull",
  "payload": {
    "projects": [
      {
        "projectId": "server",
        "name": "server",
        "cwd": "D:/Code/WheelMaker/server",
        "online": true,
        "capabilities": { "fs": true, "git": true },
        "git": { "currentBranch": "main" },
        "stats": { "lastActiveAt": "2026-03-26T09:20:00Z" }
      },
      {
        "projectId": "app",
        "name": "app",
        "cwd": "D:/Code/WheelMaker/app",
        "online": true,
        "capabilities": { "fs": true, "git": true },
        "git": { "currentBranch": "main" },
        "stats": { "lastActiveAt": "2026-03-26T09:10:00Z" }
      }
    ]
  }
}
```

说明：

- `project.listFull` 为全局接口，请求中不需要 `projectId`。
- `includeStats` 可选，默认 `false`；为 `true` 时允许返回轻量统计字段（如 `lastActiveAt`）。
- 响应中的 `cwd` 可按权限策略脱敏或隐藏。

## 6. 文件浏览协议（只读）

### 6.1 列目录：`fs.list`

请求：

```json
{
  "type": "request",
  "method": "fs.list",
  "projectId": "server",
  "payload": {
    "path": ".",
    "cursor": "",
    "limit": 200
  }
}
```

响应：

```json
{
  "type": "response",
  "method": "fs.list",
  "projectId": "server",
  "payload": {
    "path": ".",
    "entries": [
      { "name": "cmd", "path": "cmd", "kind": "dir", "size": 0, "mtime": "..." },
      { "name": "go.mod", "path": "go.mod", "kind": "file", "size": 245, "mtime": "..." }
    ],
    "nextCursor": ""
  }
}
```

说明：

- 客户端点击目录后，重新调用 `fs.list(path=<clickedDir>)`。
- 不返回整棵树，避免大目录一次性传输。

### 6.2 读文件：`fs.read`

请求：

```json
{
  "type": "request",
  "method": "fs.read",
  "projectId": "server",
  "payload": {
    "path": "internal/agent/codex/agent.go",
    "offset": 0,
    "limit": 65536
  }
}
```

响应：

```json
{
  "type": "response",
  "method": "fs.read",
  "projectId": "server",
  "payload": {
    "path": "internal/agent/codex/agent.go",
    "content": "...",
    "encoding": "utf-8",
    "eof": true,
    "nextOffset": 65536
  }
}
```

## 7. Git 浏览协议（只读）

### 7.1 分支列表：`git.branches`

请求：

```json
{
  "type": "request",
  "method": "git.branches",
  "projectId": "server",
  "payload": {}
}
```

响应：

```json
{
  "type": "response",
  "method": "git.branches",
  "projectId": "server",
  "payload": {
    "current": "main",
    "branches": ["main", "feature/x"]
  }
}
```

### 7.2 提交记录：`git.log`

请求：

```json
{
  "type": "request",
  "method": "git.log",
  "projectId": "server",
  "payload": {
    "ref": "main",
    "cursor": "",
    "limit": 50
  }
}
```

响应：

```json
{
  "type": "response",
  "method": "git.log",
  "projectId": "server",
  "payload": {
    "ref": "main",
    "commits": [
      {
        "sha": "abc123",
        "author": "name",
        "email": "x@y.z",
        "time": "...",
        "title": "commit subject"
      }
    ],
    "nextCursor": "opaque-cursor"
  }
}
```

### 7.3 提交文件列表：`git.commit.files`

请求：

```json
{
  "type": "request",
  "method": "git.commit.files",
  "projectId": "server",
  "payload": {
    "sha": "abc123"
  }
}
```

响应：

```json
{
  "type": "response",
  "method": "git.commit.files",
  "projectId": "server",
  "payload": {
    "sha": "abc123",
    "files": [
      { "path": "internal/im/feishu/feishu.go", "status": "M", "additions": 10, "deletions": 2 }
    ]
  }
}
```

### 7.4 单文件 diff：`git.commit.fileDiff`

请求：

```json
{
  "type": "request",
  "method": "git.commit.fileDiff",
  "projectId": "server",
  "payload": {
    "sha": "abc123",
    "path": "internal/im/feishu/feishu.go",
    "contextLines": 3
  }
}
```

响应：

```json
{
  "type": "response",
  "method": "git.commit.fileDiff",
  "projectId": "server",
  "payload": {
    "sha": "abc123",
    "path": "internal/im/feishu/feishu.go",
    "isBinary": false,
    "diff": "@@ ...",
    "truncated": false
  }
}
```

## 8. 服务端执行模型建议

- 按 `projectId` 映射到现有 client/project 上下文（cwd、git repo）。
- 文件查询和 git 查询使用独立只读执行器。
- 每个请求设置超时（如 5s/10s）。
- 对重型请求（大 diff / 超大目录）限制返回大小并给出 `truncated=true`。

## 9. 安全预留（V1 先留口）

### 9.1 认证与授权

- 预留 `auth` 消息（token/mTLS/OAuth2）。
- 每个连接绑定权限范围（可访问 project 列表、只读能力）。
- project 级 ACL：防止跨项目越权读取。

### 9.2 传输安全

- 建议 TLS（公网必须 TLS）。
- 预留证书轮换与指纹固定策略。

### 9.3 数据安全

- 文件读取限制在 project root 下，防目录穿越。
- 禁止读取敏感路径（可配置 denylist）。
- Git diff/文件内容可配置脱敏策略（如密钥模式匹配）。

### 9.4 审计与限流

- 记录请求审计日志：连接、projectId、method、耗时、结果码。
- 对连接和请求速率限流。
- 限制并发请求数，避免资源耗尽。

## 10. 版本演进策略

- `version` 字段用于协议协商。
- 新增字段保持向后兼容（客户端忽略未知字段）。
- 破坏性变更走 `2.x` 并提供过渡期双栈。

## 11. V1 实施顺序建议

1. 传输层：WebSocket + `hello` + 基础错误模型
2. 项目发现：`project.list` + `project.listFull`
3. 文件只读：`fs.list`、`fs.read`
4. Git只读：`git.branches`、`git.log`、`git.commit.files`、`git.commit.fileDiff`
5. 安全最小集：token + root 限制 + 审计日志
6. 性能增强：分页/游标、响应截断、并发控制

---

本稿为协议草案 V1，适合先做后端能力与协议稳定，再接入 App/Web 客户端。
