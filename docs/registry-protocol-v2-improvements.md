# Registry Protocol 2.0 → 2.1 改进清单与实施指南

本文档记录 v2 协议评审中发现的所有设计缺陷、缺失功能和文档问题。
每项包含优先级、问题分析和具体实施方案。所有改进已写入 `registry-protocol.md`（v2.1）。

---

## 阶段一：核心设计修复（P0）

必须在编码实现前解决，影响协议基本正确性和一致性。

### 1.1 解决 `version` 字段歧义

**问题**：文档标题为"Protocol 2.0"，但所有消息体使用 `"version": "1.0"`。
消息帧格式版本与协议版本混为一谈。

**修复**：
- 从消息封装中移除 `version` 字段（`connect.init` 中已有 `protocolVersion` 协商）。
- `connect.init.protocolVersion` 为协议兼容性的唯一依据。

**状态**：✅ 已实施

### 1.2 明确 `connectionEpoch` 分配机制

**问题**：`registry.updateProject` 依赖 `connectionEpoch` 做旧数据过滤，但文档未说明谁分配、如何递增。

**修复**：
- Registry 在 `connect.init` 响应中为 `role=hub` 分配 `connectionEpoch`。
- 该值为 Registry 管理的全局单调递增整数。
- Hub 需在后续 `reportProjects`、`updateProject` 中回传此值。
- `connectionEpoch` 出现在 `connect.init` 响应的 `principal` 对象中。

**状态**：✅ 已实施

### 1.3 `reportProjects` 添加 `connectionEpoch` 防止旧数据覆盖

**问题**：`reportProjects` 是"全量覆盖"语义但无过期保护。两次快速重连可能导致旧快照覆盖新数据。

**修复**：
- `registry.reportProjects` 请求中新增 `connectionEpoch` 字段。
- Registry 拒绝 `connectionEpoch < 当前已知 epoch` 的请求。

**状态**：✅ 已实施

### 1.4 标准化错误响应结构

**问题**：v1 有清晰的错误结构（code + message + details）；v2 错误码散落在各节，从未定义 JSON 格式。

**修复**：新增统一错误响应规范节：

```json
{
  "requestId": 1,
  "type": "error",
  "method": "fs.read",
  "payload": {
    "code": "NOT_FOUND",
    "message": "人类可读的描述信息",
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
| `RATE_LIMITED` | 限流 | 是 | 按退避策略重试 |
| `TIMEOUT` | 处理超时 | 是 | |
| `SNAPSHOT_EXPIRED` | 分块读中途源数据变化 | 是 | 从首块重新开始 |
| `INTERNAL` | 服务端内部错误 | 是 | |

**状态**：✅ 已实施

### 1.5 解决 `fs.read` hash 协商与分块读取的语义冲突

**问题**：`knownHash` 和 `offset/limit` 可同时出现，行为未定义。hash 是整文件级别的，
分块是字节范围级别的，两者混在一起会产生多种未定义场景。

**核心分析**：

1. **hash 协商和分块读取是两个不同关注点**——hash 管"要不要读"，分块管"怎么读"。
2. **文件块级 hash 没有意义**——源码文件的改动以插入/删除为主，任一位置的插入会导致后续所有块的字节偏移移位，所有块 hash 全部失效。只有等长替换且不跨块边界时才有效，在源码场景中是小概率事件。
3. **客户端可能有 hash 但没有数据**——例如从 `fs.list` 的 `dataHash` 获知了文件 hash，但从未读过内容。此时若发送 `knownHash`，服务端返回 `notModified=true`，客户端却没有数据可用，是死局。
4. **客户端可能从任意位置开始读**——如大日志文件直接滚到中间，从未读过开头。

**修复**——统一规则：

1. **`contentHash` 始终是整文件 hash**（`sha256(raw bytes)`），在每个 `fs.read` 响应中都返回（无论 offset 为何），不随读取范围变化。
2. **`knownHash` 语义 = conditional GET**：
   - 客户端必须持有本次请求范围 `[offset, offset+limit)` 的缓存数据才可发送 `knownHash`。
   - 若客户端仅知道 hash 值但没有缓存数据（如来源于 `fs.list.dataHash`、事件推送、或缓存已驱逐），**不得发送 `knownHash`**。
   - 文件级 hash 未变 ⟹ 全部字节未变 ⟹ 任意范围的缓存均有效。
   - 文件 hash 变化后，客户端应失效该文件所有已缓存的范围。
3. **无 `knownHash` 时**：服务端始终返回数据，无论文件是否变化。

**状态**：✅ 已实施

### 1.6 引入 `snapshotToken` 保障分块读一致性

**问题**：文件或目录在两次分块请求之间发生变化，客户端拼接出前半旧+后半新的损坏数据。

**修复**：

- 服务端在首块/首页响应中返回 `snapshotToken`（不透明字符串），当且仅当存在后续块/页。
- 客户端在所有后续块/页请求中必须回传 `snapshotToken`。
- 源数据变化时返回 `SNAPSHOT_EXPIRED` 错误；客户端丢弃已读部分，从头重读。
- `snapshotToken` 有服务端 TTL（建议 60 秒），过期同样返回 `SNAPSHOT_EXPIRED`。
- 单块/单页完成的场景不需要 `snapshotToken`。

字段出现规则（`fs.list` 和 `fs.read` 统一）：

| 字段 | 首块请求 | 首块响应 | 后续块请求 | 后续块响应 |
|------|---------|---------|-----------|-----------|
| `knownHash` | 可选 | — | 不传 | — |
| 实体 hash* | — | 必返回 | — | 不返回 |
| `notModified` | — | 必返回 | — | 不返回 |
| `snapshotToken` | 不传 | 有后续块时必返回 | 必传 | 回显 |

\* 实体 hash = `fs.read` 的 `contentHash` / `fs.list` 的 `snapshotHash`。

服务端实现建议（轻量级，不需要 COW）：

```go
type snapshotToken struct {
    path  string
    mtime int64
    size  int64
    hash  string
}
// 后续块请求时校验：if file.mtime != token.mtime || file.size != token.size → SNAPSHOT_EXPIRED
```

**状态**：✅ 已实施

### 1.7 `fs.list` 引入 `pageHash` 实现页级跳过

**问题**：大目录发生变化时，即使只有一页的条目变了，所有页都要重新下载。
目录条目是离散的（一个条目变化不影响其他条目的位置），页级缓存完全可行。

**分析**：目录分页与文件分块的本质区别——
- 目录条目是离散独立的，一个条目变化不影响其他条目 → 页级 hash 有效。
- 文件字节流是连续的，任一位置插入导致后续所有字节偏移移位 → 块级 hash 无效。

因此：**目录分页做 `pageHash`，文件分块不做块级 hash**。

**修复**：
- 服务端在每个 `fs.list` 响应页中返回 `pageHash`。
- `pageHash` 计算方式与 `snapshotHash` 一致，但范围从整个目录缩小到当前页的条目。
- 后续页请求可带 `knownPageHash`；命中则返回 `notModified=true` + `nextCursor`。
- `knownPageHash` 遵循同样的 conditional GET 规则：有本页 entries 缓存才可发送。

**状态**：✅ 已实施

### 1.8 明确 `snapshotHash` 仅覆盖一级

**问题**：`snapshotHash` 中子目录的 `dataHash` 为空字符串，父目录 hash 不会因深层文件变化而改变。
客户端可能因父目录 `notModified` 而跳过子目录刷新。

**修复**：在 6.2 节添加显式说明：
> `snapshotHash` 仅覆盖目录的直接子项。不反映嵌套子目录内部变化。
> 客户端必须对每个已展开目录层级独立做 hash 协商；
> 父目录 `notModified=true` 不意味着子目录未变化。

**状态**：✅ 已实施

### 1.9 明确 `event` 消息不携带 `requestId`

**问题**：消息模板在 `type: "event"` 旁显示了 `requestId`，但事件示例中没有。

**修复**：在 3.1.1 中添加规则：
> `requestId` 对 `type=request/response/error` 必填。
> 对 `type=event` 不携带 `requestId`——事件是单向推送，无请求-响应对应。

**状态**：✅ 已实施

---

## 阶段二：缺失的客户端协议（P1）

### 2.1 `git.workingTree.fileDiff` — 查看未提交改动

**必要性**：`git.status` 返回改动文件列表，但客户端无法查看具体 diff。
核心场景：观察 agent 刚刚修改了什么。没有此接口，`git.status` 页面是死胡同。

`scope` 取值：
- `staged` — `git diff --cached`
- `unstaged` — `git diff`
- `untracked` — 以 new-file diff 形式返回完整内容

**状态**：✅ 已实施

### 2.2 二进制文件处理规则

**必要性**：`fs.read` 未定义二进制文件行为。客户端会收到乱码或崩溃。

**修复**：
- 响应增加 `isBinary`、`mimeType` 字段。
- `isBinary=true` 且 < 2MiB：`encoding: "base64"`。
- `isBinary=true` 且 >= 2MiB：`encoding: "none"`，`content: null`（仅返回元信息）。

**状态**：✅ 已实施

### 2.3 Git 标签支持

**修复**：将 `git.branches` 升级为 `git.refs`，响应包含 `branches` 和 `tags` 数组。

**状态**：✅ 已实施

### 2.4 `git.diff` — 任意两个 ref 之间比较

新增 `git.diff`（文件列表）和 `git.diff.fileDiff`（单文件 diff 详情）。

**状态**：✅ 已实施

### 2.5 `project.syncCheck` — 重连后高效状态恢复

**必要性**：客户端断线重连后需要高效判断哪些数据过时。无此接口只能全量重拉。

客户端携带已知的 `projectRev/gitRev/worktreeRev`，服务端返回 `staleDomains`。

**状态**：✅ 已实施

### 2.6 明确 Client 的 Hub 作用域模型

**问题**：`connect.init` 说 client 的 `hubId` 可省略，`project.list` 又说"绑定 hub 作用域"，自相矛盾。

**修复**：定义两种模式：
- 携带 `hubId` → 绑定单 hub，`project.list` 仅返回该 hub 项目。
- 省略 `hubId` → 全局范围，返回 token 有权访问的所有 hub 项目。

**状态**：✅ 已实施

### 2.7 补全缺失的响应示例

v2 中以下方法只有请求示例没有响应示例：
- `git.log`（commit 对象：sha, author, email, time, title, nextCursor）
- `git.commit.files`（files 数组：path, status, additions, deletions）
- `git.commit.fileDiff`（isBinary, diff, truncated）

**状态**：✅ 已实施

### 2.8 定义 `git.status` 的 status 值枚举

对齐 `git status --porcelain`：
`M`（modified）、`A`（added）、`D`（deleted）、`R`（renamed）、`C`（copied）、`U`（unmerged）、`?`（untracked）。

**状态**：✅ 已实施

### 2.9 `fs.search` 结果增加 `kind` 字段

客户端无法知道搜索结果是文件还是目录，无法正确渲染图标或决定后续操作。

**状态**：✅ 已实施

### 2.10 修复 `project.list` 的 `includeStats` 问题

v2 请求中有 `includeStats: true`，但响应中无 stats 字段。

**修复**：移除 `includeStats`，将 `agent`、`imType` 等元信息直接包含在 project 对象中。

**状态**：✅ 已实施

### 2.11 统一 `reportProjects` 和 `updateProject` 的 project 对象

**问题**：`reportProjects` 有 `agent`、`imType`，`updateProject` 没有。4.3 说"字段保持一致"，但示例不一致。

**修复**：定义共享的 `ProjectObject` 结构（2.3 节），所有使用 project 对象的方法引用同一结构。

**状态**：✅ 已实施

---

## 阶段三：增强功能（P2）

### 3.1 `agent.status` + `agent.activity` 事件

**必要性**：产品核心价值——观察 agent 实时行为。project 对象有 `agent: "codex"` 字段，
但协议无法查询 agent 当前状态或接收活动推送。

**状态**：✅ 已实施

### 3.2 `fs.grep` — 文件内容搜索

**必要性**：`fs.search` 仅支持文件名匹配。用户频繁需要搜索代码内容（如"哪里调用了这个函数"）。

**状态**：✅ 已实施

### 3.3 `project.changed` 增加 `changedPaths` 提示

**问题**：`project.changed` 只有 `changedDomains`，客户端不知道哪些路径变了，
需要对所有已展开目录逐个 `fs.list` 试探。

**修复**：payload 中增加可选 `changedPaths` 数组，客户端仅对交集路径做刷新。

**状态**：✅ 已实施

### 3.4 `project.offline` / `project.online` 事件

Hub 断连/重连时通知客户端，避免客户端对离线项目发起无效请求。

**状态**：✅ 已实施

### 3.5 Client 空闲超时

Registry 对空闲 Client 连接做超时清理（建议 5 分钟），清理前发送 `connection.closing` 事件。

**状态**：✅ 已实施

### 3.6 `batch` 批量请求

**必要性**：移动端高延迟网络（4G）下，逐个 WebSocket 消息 RTT 很慢。
批量合并多个独立请求为单次发送。

**状态**：✅ 已实施

### 3.7 `subscribe` / `unsubscribe` — 路径级推送订阅

**必要性**：`project.changed` 是 project 级粗粒度通知。客户端展开了 `src/` 但没展开 `docs/`，
`docs/` 的变更也会触发无效刷新。

路径级订阅让 push hint 精准到目录/文件级别。

**状态**：✅ 已实施

---

## 阶段四：文档清理（P3）

### 4.1 修复 `fs.read` 重复响应示例

v2 的 5.4 节有两套相互矛盾的响应示例（一套有 `contentHash`/`notModified`，一套没有。
`eof=true` 时仍给出 `nextOffset` 等矛盾）。

**修复**：统一为一套响应结构，按场景（未变化 / 有变化 / 分块首块 / 分块后续块 / 随机范围读）分别给出示例。

**状态**：✅ 已实施

### 4.2 方法白名单完整性

**问题**：事件方法（`project.changed`、`git.workspace.changed` 等）是服务端→客户端推送，
不在白名单中，也没说明不受白名单约束。

**修复**：明确事件不受白名单约束，并枚举所有事件方法。

**状态**：✅ 已实施

### 4.3 版本历史章节更新

更新第 10 节，记录 2.1 相比 2.0 的所有改进。

**状态**：✅ 已实施

---

## Hash 体系总结

经过多轮深入讨论，最终确定的 hash/缓存/分块体系如下：

### 核心原则

```
hash 管"要不要读" → knownHash 协商（conditional GET）
token 管"读的一致" → snapshotToken 一致性保障
块本身不需要 hash（文件场景）/ 页需要 hash（目录场景）
```

### 为什么文件块不做 hash，目录页要做 hash

| 维度 | 目录分页（fs.list） | 文件分块（fs.read） |
|------|--------------------|--------------------|
| 条目独立性 | 高——条目互不影响 | 低——字节流连续，插入导致全移位 |
| 块/页级 hash 命中率 | 高 | 极低（源码场景） |
| 是否值得做 | **值得**（`pageHash`） | **不值得** |

### `knownHash` = Conditional GET

```
knownHash 出现  → "我有这份数据的缓存，hash 是这个，没变就别传了"
knownHash 不传  → "我没有缓存，无论如何把数据给我"
```

**绝对禁止**：客户端仅从 `fs.list.dataHash` 等外部来源获知 hash 但无内容缓存时发送 `knownHash`——
会导致服务端返回 `notModified=true`，客户端无数据可展示的死局。

### 随机范围读取

`contentHash` 始终是整文件 hash，文件级 hash 未变则任意范围缓存均有效。
客户端按 `{path, offset, limit, fileHash}` 组合做缓存键。

### 完整字段视图

**fs.list（目录）**：

```
snapshotHash    → 整个目录的 hash，首页协商用
pageHash        → 当前页条目的 hash，后续页协商用
entry.dataHash  → 单个文件条目的内容 hash
snapshotToken   → 分页一致性令牌
```

**fs.read（文件）**：

```
contentHash     → 整个文件的 hash，首块/范围协商用
snapshotToken   → 分块一致性令牌
（无块级 hash）
```

---

## 实施顺序

所有改进已统一写入 `registry-protocol.md`（版本升级至 2.1）。

1. ✅ **阶段一**（核心修复）：更新消息封装、connectionEpoch、错误结构、hash/分块语义。
2. ✅ **阶段二**（缺失协议）：新增 git 方法、二进制处理、syncCheck、响应示例补全。
3. ✅ **阶段三**（增强功能）：新增 agent、grep、batch、subscribe 等节。
4. ✅ **阶段四**（文档清理）：修复示例、白名单、版本历史。
