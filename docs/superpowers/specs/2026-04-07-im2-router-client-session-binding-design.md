# IM2 Router + ClientSession Binding Design

Updated: 2026-04-07
Status: Draft (Rewrite v2)

## 0. Background

上一版文档存在一个关键误导：把 `channel` 命名对齐描述成了实现对齐，容易导致把 IM2 设计成 IM1 的薄封装。

本版明确改为：

1. `im2` 与 `internal/hub/im` 实现隔离。
2. IM2 是独立路由系统，行为以 IM2 协议为准。
3. 旧 `im` 代码保留但不改动，作为过渡期输入来源。

## 1. Goals

1. `routeKey` 固定为 `imType:chatID`，由 IM2 Router 统一生成。
2. `IMRouter` 负责注册 IM 通道、接收入站消息、归一化后转发给 Client。
3. 入站转发给 Client 时必须携带 `routeKey`。
4. Client 普通回复默认按入站 `routeKey` 定点回发。
5. `clientSessionId` 作为 chat 绑定目标，支持一对多 chat 绑定。
6. IM2 仅持久化 chat 投影（`im_active_chats`）。

## 2. Non-Goals

1. 修改 `server/internal/hub/im/*` 现有实现。
2. 把 IM2 退化为 IM1 的 wrapper。
3. 在 IM2 协议中引入 `userId`。
4. 改动 ACP 协议结构。

## 3. Final Decisions

| Topic | Decision |
|---|---|
| IM2 vs IM1 | 实现隔离，旧 IM 不改 |
| Router ownership | 每个 Client 一个 IMRouter |
| Route identity | `routeKey = <imType>:<chatID>` |
| Route source | 由 Router 从 IM 入站统一生成 |
| Inbound dispatch | Router -> Client 时带 `routeKey` |
| Outbound default | 普通回复按当前会话 `routeKey` 定点发送 |
| Broadcast | 显式无 target 才按 `clientSessionId` 广播 |
| `/new` behavior | 只重绑当前 `routeKey` |
| Persistence | IM2 仅维护 `im_active_chats` |

## 4. Target Architecture

```text
Hub
  -> Client
      -> IM2 Router (one per client)
          -> registered IM channels
      -> Session manager (client domain)
```

职责边界：

1. `IM2 Router`
   - 注册 IM channel。
   - 接收 channel 入站消息，生成 `routeKey`。
   - 解析/创建 `clientSessionId` 绑定。
   - 向 Client 回调标准 inbound event（带 `routeKey`）。
   - 接收 Client outbound，按 `routeKey` 或 session 广播路由到 channel。

2. `Client`
   - 消费 Router inbound event。
   - 根据 `routeKey -> Session` 路由处理消息。
   - 回复时优先按该 Session 当前 `routeKey` 回发。

3. `im2.State`
   - 只管理 `routeKey(activeChatID) <-> clientSessionId` 绑定与在线态。

## 5. Protocol Contract Alignment

协议源文档：`docs/im-protocol-2.0.md`。

强约束：

1. inbound：`prompt | permission_reply | command`
2. outbound：`message | acp_update | command_reply`
3. `routeKey` 必须是 `imType:chatID`
4. inbound 未绑定时必须创建绑定，不可丢弃
5. 普通回复按 `routeKey` 回发

## 6. Core Flows

### 6.1 Inbound (IM -> Router -> Client)

1. IM channel 产生入站消息（至少有 `imType` 和 `chatID`）。
2. Router 生成 `routeKey = imType:chatID`。
3. Router 解析 `routeKey` 对应的 `clientSessionId`，缺失则新建并绑定。
4. Router 向 Client 分发 inbound event，携带 `routeKey` + `clientSessionId`。

### 6.2 Normal Reply (Client -> Router -> IM)

1. Session 处理消息后产生普通回复。
2. Client 按该 Session 当前 `routeKey` 定点调用 Router outbound。
3. Router 定位到对应 channel 并发送。

### 6.3 Broadcast Reply

1. Client outbound 不带 target routeKey。
2. Router 按 `clientSessionId` 找在线 chat 并广播。

### 6.4 `/new` Rebind

1. Client 在当前 `routeKey` 上创建新 session。
2. Router 将该 `routeKey` 重绑到新 `clientSessionId`。
3. 其他 routeKey 绑定保持不变。

## 7. State Boundary

1. Client state manager：管理 client/session 生命周期。
2. IM2 state manager：只管 chat 绑定投影。
3. 可共享同一 sqlite 文件，但表责任严格分离。

## 8. Migration Constraints

1. 本轮迁移禁止改 `server/internal/hub/im/*` 文件。
2. 新逻辑只在 `internal/im2` 与 `hub/client|hub/hub` 增量落地。
3. 旧 IM 路径保留，直到 IM2 全量切换完成。

## 9. Validation Matrix

1. Unit (`internal/im2`)
   - routeKey 生成正确
   - inbound 首次建绑定
   - normal reply 定点路由
   - broadcast 正确 fan-out
   - rebind 仅影响目标 routeKey
2. Integration (`internal/hub/client + internal/im2`)
   - Client 收到 inbound 时含 routeKey
   - Session 普通回复按 routeKey 回发
   - `/new` 后仅当前 routeKey 绑定变化
3. Non-regression
   - 不修改 `internal/hub/im/*`
   - 协议无 `userId`

## 10. Exit Criteria

1. 实现中未改动 `server/internal/hub/im/*`。
2. Router 入站和回包都基于 `routeKey`。
3. `/new` rebind 行为通过测试。
4. IM2 状态仍只保存 chat 投影。
