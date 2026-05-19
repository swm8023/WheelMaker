# Chat Turn 同步、状态与显示重构设计

本文定义 WheelMaker app/web 与 server 的第一阶段 chat session 同步目标模型。目标是让 session 状态、流式更新、客户端缓存、窗口渲染在断线、重连、漏收、乱序、隐藏 tool、长会话等场景下稳定正确。

本文是待 review 设计，不代表当前代码已经全部实现。

## 1. 设计目标

第一阶段目标：

- session 列表中的进行中、完成、失败未查看状态必须正确。
- 当前 session 的 streaming turn 必须稳定更新，同一 turn 原地覆盖，不产生重复或断裂。
- 进入内存 runtime store 的 session 可以全量加载 turns 到内存，简化同步模型。
- React/DOM 不能全量渲染历史，只挂载 `react-virtuoso` 的 visible + overscan items。
- 上滑/下滑只调整窗口，不触发 server read。
- 用户在底部时，新 turn 自动跟随；用户上翻后，新 turn 不强制跳底。
- 服务端和 app/web 同步升级，不保留旧 message/read wire 兼容。
- IndexedDB / 本地持久 cache schema 不做兼容迁移；版本不匹配或旧格式检测失败时，除 token / 认证凭据外清空本地持久缓存并重建表，由 server read 重新补。

非目标：

- 不做 `session.read` 分页。
- 不做 IndexedDB chunk cache。
- 不做 per-turn IndexedDB entry。
- 不做 WMT2 turn 正文精简或大内容外置。
- 不做多 viewer read cursor。

## 2. 核心原则

服务端提供权威 raw turn 流与 **Session Summary**。客户端维护内存 runtime store 中 session 的 raw turn source store、durable finished prefix、串行 read repair 和派生显示视图。

关键边界：

- `Finished Cursor` 是唯一 read cursor。
- `Live Turn Buffer` 可以显示，但不能推进 cursor。
- `Session Summary` 是 session list 状态的唯一来源。
- `session.message` 只更新 turn store，不更新 title、preview、running、done、read。
- `Display Index` 与 virtualized view 是派生显示，不是缓存源。

## 3. Wire 格式

### 3.1 Raw Turn

服务端、app source store、IndexedDB 都使用同一个 raw turn shape：

```ts
type RegistrySessionTurn = {
  turnIndex: number;
  content: string;
  finished: boolean;
};
```

JSON 使用 camelCase：

```json
{
  "turnIndex": 12,
  "content": "{\"method\":\"agent_message_chunk\",\"param\":{\"text\":\"hello\"}}",
  "finished": false
}
```

规则：

- `turnIndex` 从 1 开始。
- `content` 是非空 IM turn JSON 字符串，包含 `method` 和 `param`。
- source store 和 IndexedDB 原样保存 `content`，不 parse-normalize 后重新 stringify。
- decode 只发生在 render、copy、prompt status、selected prompt_done markRead 等消费点。
- 如果 decode 失败，显示层可 fallback 为 system message；sync/cache 不因 decode 失败丢 raw turn。

### 3.2 Realtime session.message

实时事件 payload：

```json
{
  "sessionId": "sess-1",
  "turn": {
    "turnIndex": 12,
    "content": "{\"method\":\"agent_message_chunk\",\"param\":{\"text\":\"hello\"}}",
    "finished": false
  }
}
```

规则：

- `sessionId` 只在 payload 顶层。
- `turn` 内不再重复 `sessionId`。
- 不兼容旧 `{sessionId, turnIndex, content, finished}` payload。
- 高频 `session.message` 不带 Session Summary。

### 3.3 session.read

请求仍使用 Finished Cursor：

```json
{
  "sessionId": "sess-1",
  "afterTurnIndex": 128
}
```

响应：

```json
{
  "sessionId": "sess-1",
  "latestTurnIndex": 132,
  "session": {
    "sessionId": "sess-1",
    "title": "Build sync",
    "preview": "Build sync",
    "updatedAt": "2026-05-18T12:00:00Z",
    "messageCount": 132,
    "running": true,
    "latestTurnIndex": 132,
    "lastDoneTurnIndex": 120,
    "lastDoneSuccess": true,
    "lastReadTurnIndex": 120
  },
  "turns": [
    {
      "turnIndex": 129,
      "content": "{\"method\":\"agent_message_chunk\",\"param\":{\"text\":\"...\"}}",
      "finished": true
    }
  ]
}
```

规则：

- response 顶层必须带 `sessionId`，app 必须校验它与请求 session 一致。
- `turns[]` 内不带 `sessionId`。
- `session` summary 随 read 返回，方便激活 session 时一次同步 turns 和 list state。
- 当前迭代不做分页，返回 `afterTurnIndex` 后服务端已有的完整连续范围。
- `turns` 为空只允许在 `latestTurnIndex <= afterTurnIndex`。

## 4. 服务端不变量

### 4.1 Turn 连续性

- 服务端对外暴露的 turn index 必须连续。
- WMT2 正常写入必须强拒绝洞：`startTurnIndex == latestPersistedTurnIndex + 1`。
- 空 content 必须拒绝。
- 语义缺失的 durable slot 在 read projection 中合成 `session/gap`，不写回 WMT2。

Hot gap raw turn：

```json
{
  "turnIndex": 42,
  "content": "{\"method\":\"session/gap\",\"param\":{\"reason\":\"missing_turn\",\"turnIndex\":42}}",
  "finished": true
}
```

### 4.2 Unfinished Tail

服务端对外最多允许一个 unfinished turn：

- `finished:false` 只能用于最新 text streaming turn。
- `finished:false` 只用于 `agent_message_chunk` / `agent_thought_chunk`。
- `tool_call`、`agent_plan`、`prompt_request`、`prompt_done` 默认都是 `finished:true`。
- tool running 状态通过 `param.status` 表达，不用 `finished:false`。
- 发布更大 `turnIndex` 前，服务端必须先以同 index 发布前一个 text turn 的 `finished:true` seal。
- `session.read` 可以返回 finished prefix + 最多一个 unfinished tail，且 unfinished tail 必须是最后一个 turn。

### 4.3 prompt_done 发布顺序

完成 prompt 时的发布顺序：

1. 持久化 prompt turns。
2. 发布必要的 sealed text turn。
3. 发布 `prompt_done` raw turn。
4. 发布 `session.updated` summary。

这样 selected session 可以先收到完整 turn stream，再由 summary/markRead response 收敛列表状态。

## 5. 客户端 Source Store

### 5.1 类型分层

```ts
type RegistrySessionTurn = {
  turnIndex: number;
  content: string;
  finished: boolean;
};

type RegistryChatMessage = {
  sessionId: string;
  turnIndex: number;
  method: string;
  param: Record<string, unknown>;
  finished: boolean;
};
```

- `RegistrySessionTurn` 是 wire/cache/source store 类型。
- `RegistryChatMessage` 是 decoded view model，用于 render/copy/status。
- sync、cursor、cache 逻辑只依赖 raw turn 的 `turnIndex` 和 `finished`。

### 5.2 Finished Store

- 每个进入内存的 session 保留全量 finished raw turns。
- Finished Store 可短期包含 cursor 之后的 finished turns，例如 `1,2,4`。
- Finished Cursor 只取连续 finished prefix，因此 `1,2,4` 的 cursor 是 2。
- 同 index finished turn 覆盖 live turn。

### 5.3 Live Turn Buffer

- 保存内存中 session 的 unfinished raw turn。
- 正常情况下每个 session 最多一个 live tail。
- Live turn 可以参与 selected session 的显示。
- Live turn 不能推进 Finished Cursor，不能写 Durable Turn Cache。

### 5.4 Same-Index 覆盖

同一个 `turnIndex` 的多次更新按 raw turn upsert：

- `finished:false` 覆盖之前的同 index `finished:false`。
- `finished:true` 覆盖同 index live，并从 Live Turn Buffer 移入 Finished Store。
- 普通 realtime `finished:false` 不覆盖已有 `finished:true`。
- `session.read` 返回的 covered range 是权威范围，可以覆盖该范围内的旧 raw turns。

## 6. In-Memory Runtime Store

客户端不再维护有容量上限的运行集合。任何收到 `session.message`、被 hydrate、被 read、或被用户选择的 session 都进入内存 runtime store，并保留到页面生命周期结束或用户显式清除/reload。

规则：

- 不按 session 数淘汰内存中的 Finished Store / Live Turn Buffer / Finished Cursor。
- 收到已知项目的 `session.message` 后，无论该 session 是否 selected，都完整消费消息、维护 raw source store、触发 gap Read Repair、并写 Durable Cache。
- 如果收到未知 session 的 `session.message`，客户端先触发该 project 的 session list refresh，同时仍将该消息写入内存 runtime store，避免丢失 realtime turn。
- selected session 在完整同步之外，额外维护 Display Index、virtualized view、Tail Lock、selected prompt_done markRead。
- 重连后需要补读的 session 从内存 runtime store keys 推导，并额外加入当前 selected session。
- Dirty finished prefix 仍通过 5 秒 debounce 写 Durable Cache；不再存在容量淘汰前 flush。

## 7. Read 触发与 Cursor

Read cursor 永远是该 session 的 Finished Cursor：

```text
afterTurnIndex = Finished Cursor
```

不能使用：

- Live Turn Buffer 最大 turnIndex。
- virtualized view 的 latest visible item。
- Session Summary.latestTurnIndex。
- Background hint。

触发时机：

1. **首次进入内存 runtime store**：hydrate local durable cache 后，按可见恢复/选择流程调用 `session.read(after=Finished Cursor)`。
2. **内存 session 实时 gap**：如果收到 `incoming.turnIndex > Finished Cursor + 1`，触发 read。
3. **重连后**：对内存 runtime store keys 中的 session 执行 `session.read(after=Finished Cursor)`，并额外包含当前 selected session。
4. **显式刷新 / reload**：普通 refresh 用当前 Finished Cursor；reload 清 cursor 后从 0 read。

例子：

```text
cursor = 10
receive turn 11 finished:false
```

不 read，因为这是连续 live tail。

```text
cursor = 10
receive turn 12 finished:false
```

触发 read，因为 turn 11 的 seal 或内容可能漏收。

```text
cursor = 10
live = 11 finished:false
receive turn 12 finished:true
```

仍触发 read，因为 live 11 不能推进 cursor，服务端按协议应先发布 11 finished seal。

## 8. Read Repair

同一 session 只允许一个 read in flight。

规则：

- read in flight 时再次触发 gap，只设置 Repair Pending Flag。
- read 完成后，如果 pending=true，按当前 store 重新检查是否仍有 gap；需要时再发一次 read。
- 不按触发次数排队。

应用 read response：

- response 必须连续。
- response 最多一个 `finished:false`，且只能是最后一个 turn。
- read 覆盖范围是 `[afterTurnIndex + 1, responseLastTurnIndex]`。
- 覆盖范围内先删除旧 finished/live raw turns，再用 response turns 重建。
- 覆盖范围外保留，随后用 gap check 判断是否继续 read。
- 如果 response 为空且 `latestTurnIndex <= afterTurnIndex`，接受为空。
- 如果 response 为空但 `latestTurnIndex > afterTurnIndex`，丢弃 response，不推进 cursor。
- 如果 response 中间出现 `finished:false` 后还有更大 turn，视为协议异常，不推进异常后的 turns；保留旧 store 并等待后续 read。
- 如果 `latestTurnIndex < Finished Cursor`，本地 cache stale，清本地该 session cache 后从 0 read。

## 9. Durable Turn Cache

### 9.1 IndexedDB 格式

第一阶段保持 session content blob，不做 chunk：

```text
wm_chat_session_index
  k
  projectId
  sessionId
  sessionJson
  cursorJson
  updatedAt

wm_chat_session_content
  k
  projectId
  sessionId
  turnsJson
  updatedAt
```

`turnsJson` 保存 raw turns：

```json
[
  {
    "turnIndex": 1,
    "content": "{\"method\":\"prompt_request\",\"param\":{\"contentBlocks\":[]}}",
    "finished": true
  }
]
```

规则：

- `turnsJson` 只保存 `1..Finished Cursor` 的连续 finished prefix。
- `cursorJson.turnIndex` 是 Durable Cache 的 prefix cursor。
- hydrate 时校验 `cursorJson` 和 `turnsJson`：
  - 计算 `turnsJson` 实际连续 finished prefix。
  - 最终 cursor 取 `min(cursorJson.turnIndex, actualPrefix)`.
  - 丢弃 cursor 之后的 durable turns。
  - 修正 IndexedDB 为一致状态。
- DB schema 不兼容旧 `messagesJson` decoded message 格式；检测到旧版本或旧格式时，不做表级迁移，除 token / 认证凭据外删除本地持久缓存并重建 IndexedDB 表。

### 9.2 Persist

内存 runtime store 中的 finished cursor 更新后：

- 内存 Finished Store / Finished Cursor 立即更新。
- UI 立即更新。
- IndexedDB 延迟 5 秒 debounce persist。

必须 flush：

- 页面 hidden / beforeunload。
- reload / archive / delete 前。
- selected session 收到 `prompt_done` 后。

flush 失败：

- 不阻止 UI。
- 下次该 session 进入 read/repair 流程时用旧 cursor 调 server read 修复。

## 10. Session 状态同步

Session list 状态只由服务端 Session Summary 更新：

- `session.list`
- `session.updated`
- `session.read` response 的 `session`
- `session.markRead` response 的 `session`

`session.message` 不直接更新：

- title
- preview
- running
- lastDoneTurnIndex
- lastDoneSuccess
- lastReadTurnIndex
- unread/completed flags

状态规则：

- `running === true`：显示进行中。
- `running !== true && lastDoneTurnIndex > lastReadTurnIndex && lastDoneSuccess === false`：失败未查看。
- `running !== true && lastDoneTurnIndex > lastReadTurnIndex`：完成未查看。
- 其他：idle。

`prompt_done`：

- 只有 selected session decode 出 `prompt_done` 后调用 `session.markRead(promptDoneTurn.turnIndex)`。
- 非 selected session 即使收到并写入内存，也不 markRead。

## 11. Display Index 与虚拟列表策略

第一阶段 full selected session raw turns 已在内存，滚动策略只控制 React/DOM 渲染和可见 projection。不能为了滚动条或高度计算提前 decode、markdown render 或挂载全量 turns。

本迭代使用 `react-virtuoso` 封装 `ChatVirtuosoTurnList`。不再维护手写 raw turn range 作为主要滚动状态；raw `turnIndex` 仍作为 Display Item metadata、copy range、gap/cursor 判断和 scroll-to-item 定位边界。

### 11.1 Source、Display Index 与 View

```text
Source turns =
  merge(Finished Store raw turns, Live Turn Buffer raw turns)
  -> same index finished wins
  -> sorted by turnIndex

Display Index =
  source raw turns
  -> shallow parse content envelope only
  -> map each renderable unit to DisplayItem
  -> apply hide/show filters
  -> keep lightweight item metadata and height estimates

Virtualized Chat View =
  Display Index
  -> react-virtuoso computes visible + overscan items
  -> decode/render only mounted items
  -> measure mounted item heights
```

`Display Index` 是显示索引，不是第三份消息数据。它只保存轻量元数据，不保存完整 decoded message，也不保存 markdown render 结果。

```ts
type ChatDisplayItem = {
  key: string;
  turnIndex: number;
  kind:
    | "text"
    | "tool"
    | "thought"
    | "plan"
    | "prompt_request"
    | "prompt_done"
    | "gap"
    | "system";
  affordance?: "option_replies" | "confirmation_reply";
  finished: boolean;
  contentRevision: string;
  estimatedHeight: number;
  measuredHeight?: number;
};
```

约束：

- Source stores 仍只保存 raw turns。
- React/DOM 只挂载 Virtuoso 当前 visible + overscan 的 items。
- `Display Index` 覆盖 selected session 的完整 source turns，但每个 item 必须轻量。
- `content` 全量 JSON parse 只允许用于 shallow envelope：识别 `method`、`param.type`、`status`、是否 copyable、是否隐藏等。
- Markdown、代码块、复杂组件 props、复制文本等重 decode 只能发生在 visible + overscan items。

### 11.2 DisplayItem 与 raw turnIndex

- 不维护手写 `ChatTurnWindow` 作为 React 渲染窗口。
- `turnIndex` 仍是每个 `ChatDisplayItem` 的 raw 坐标。
- `turnIndex` 不参与 virtualizer 的滚动高度估算；高度由 item estimate/measure 决定。
- 隐藏 tool/thought 不进入 `Display Index`，因此不贡献可见滚动高度。
- `session/gap` 必须进入 `Display Index`，并以 `kind: "gap"` 渲染为不可恢复占位。
- `prompt_request` / `prompt_done` / tool / plan / gap 等协议类型由 `ChatDisplayItem.kind` 决定，不能在 React 组件里临时猜测。
- A/B/C 选项和中文确认不是独立 Display Item；它们是最新 eligible `kind: "text"` item 的 `affordance`，由 shallow/parser 层或 render selector 标记，仍由同一个 text item 渲染。

如果现有协议在 `content.param` 或 assistant text 中表达 ABC、确认按钮、计划状态、tool 状态等特殊 UI，shallow parser / render selector 必须投影成稳定的 `kind` 或 `affordance`。Virtualizer 重挂组件时，展开、确认、选择、复制、输入态等 UI 状态不能只存在 DOM 组件本地，必须按 `sessionId + item.key` 存在 session UI store 中，或者完全由 raw turn / Session Summary 推导。

### 11.3 Scrollbar 与高度估算

右侧 scrollbar 必须由 `react-virtuoso` 基于 `Display Index` 的逻辑总高度维护，而不是由已挂载 DOM 节点总高度自然决定。

`ChatVirtuosoTurnList` 封装边界：

```ts
type ChatVirtuosoTurnListProps = {
  scrollRef: React.RefObject<HTMLElement | null>;
  displayIndex: ChatDisplayIndex;
  runtimeKey: string;
  renderItem: (item: ChatDisplayIndexItem) => React.ReactNode;
  onAtBottomChange: (atBottom: boolean) => void;
  shouldAutoscroll: () => boolean;
};
```

实现约束：

- 使用 `Virtuoso` 组件，不保留手写 range、padding spacer 或旧虚拟列表代码。
- `data = displayIndex.items`。
- `computeItemKey = item.key`。
- `defaultItemHeight` / `heightEstimates` 由 `DisplayIndexItem.estimatedHeight` 提供给 Virtuoso。
- `customScrollParent` 指向 chat scroll container。
- `atBottomStateChange`、`totalListHeightChanged`、`scrollToIndex({index: "LAST"})` 和 `autoscrollToBottom()` 都封装在 wrapper 内。
- `main.tsx` 不直接操作 Virtuoso API；只传 `Display Index`、renderer 和滚动意图。

高度策略：

- 未渲染 item 使用 `estimatedHeight`。
- visible + overscan item 挂载后的真实 DOM 高度由 Virtuoso 内部测量与缓存。
- App 不维护第二套高度缓存或手写 range height cache。
- Virtuoso 使用内部测量值和传入估算值计算逻辑总高度和 scrollbar thumb。
- 流式 tail item 的高度更新用 `requestAnimationFrame` 或节流合并，避免每个 chunk 都触发布局抖动。
- 禁止为了获得精确 scrollbar 而提前 decode/render 全量 turns。

估算高度不要求绝对精确。远距离拖动 scrollbar 时，未测量区域可能在进入视口后被校准；校准必须通过 anchor restore 保持当前可见内容不跳。

### 11.4 Anchor 与 Tail Lock 不变量

滚动正确性依赖两个互斥模式：

- **Viewport Anchor**：用户不在底部时，window 扩展、裁剪、未测量高度修正、流式内容变高，都必须保持当前锚点 item 在 viewport 内的位置不变。
- **Tail Lock**：用户在底部时，latest item 变化后必须保持滚动到底部。

Anchor 记录：

```ts
type ScrollAnchor = {
  itemKey: string;
  turnIndex: number;
  offsetFromViewportTop: number;
};
```

更新流程：

1. 在修改 display index、Virtuoso measurement state 或 tail 内容前记录 anchor。
2. 应用数据变化并让 Virtuoso 完成布局。
3. 如果处于 Tail Lock，滚动到底部。
4. 否则恢复 anchor 的 viewport offset。

### 11.5 初始定位

打开 selected session 或切换到已在内存中的 session：

1. 计算 latest source turn index。
2. 构建完整轻量 `Display Index`。
3. `ChatVirtuosoTurnList` 初始定位到最后一个 `DisplayItem`，`align: "end"`。
4. 初始状态进入 Tail Lock。

默认参数：

- Overscan：通过 `increaseViewportBy` / `minOverscanItemCount` 控制，按接近一屏到两屏内容调优。
- Tail bottom threshold：由 `atBottomThreshold` 控制，默认 80px。
- Estimate defaults 由 `ChatDisplayItem.kind` 决定，长文本可用 raw `content.length` 粗估，但不得 markdown render。

### 11.6 上滑

当用户向上滚动：

1. 不移动手写 raw turn range。
2. `react-virtuoso` 根据 scroll offset 计算 visible + overscan items。
3. mounted item 才 decode/render/measure。
4. 不触发 server read，因为该 session 的 source turns 已全量在内存。
5. 不裁剪 `Display Index`；DOM bounded 由 Virtuoso 保证。

### 11.7 下滑

当用户向下滚动：

1. 不移动手写 raw turn range。
2. `react-virtuoso` 根据 scroll offset 计算 visible + overscan items。
3. mounted item 才 decode/render/measure。
4. 如果 scroll container 到达底部阈值，进入 Tail Lock。
5. 不触发 server read。

### 11.8 新 turn 与流式增长

如果 selected session 在 Tail Lock：

- 新 turn 进入 source store 后，增量更新 `Display Index`。
- Virtuoso 滚动到最后一个 item，保持底部。

如果 selected session 不在 Tail Lock：

- source store 更新。
- `Display Index` 增量更新。
- 当前 viewport anchor 保持稳定。
- 显示 scroll-to-bottom affordance。
- 用户点击或下滑到 latest 后恢复 Tail Lock。

流式 chunk 更新同一 tail turn 时：

- `contentRevision` 更新。
- 如果该 item 已挂载，重新渲染并测量。
- 如果该 item 未挂载，只更新 source 与 item revision，不提前 render。

### 11.9 Copy

`prompt_done` copy 使用该 session 的 full source store，不受当前 window 限制：

1. 从 `prompt_done.turnIndex` 向前找最近 `prompt_request`。
2. 要求 raw range 在 Finished Store 中连续。
3. range 含 `session/gap` 时禁用 copy。
4. 只复制 copyable decoded message，例如 `agent_message_chunk`。

## 12. 落地代码结构迭代

这次不应继续把同步、缓存、状态和显示窗口逻辑堆在 `main.tsx`。落地时优先做结构拆分，再填充行为：

- `registry.ts`：只定义 wire types，包括 `RegistrySessionTurn`、`session.message` payload、`session.read` response、Session Summary。
- `chatWire.ts`：做 raw turn shape 校验和旧 payload 拒绝，不 decode 业务内容。
- `chatTurnStores.ts`：维护 Finished Store、Live Turn Buffer、Finished Cursor、read response reconcile、gap repair 判定。
- `main.tsx` / 后续可抽出的 runtime store 模块：维护无容量上限的内存 turn stores，并基于 store keys 处理重连后 read trigger。
- `workspacePersistence.ts` / `workspaceStore.ts`：只负责 raw `turnsJson`、`cursorJson`、schema/version 检查、除 token 外的全量本地缓存清理、重建表、5 秒 debounce persist 和 flush。
- `chatDisplayIndex.ts`：从 raw source store 派生轻量 Display Index，只做浅分类和 metadata，不保存完整 decoded message。
- `ChatVirtuosoTurnList.tsx`：封装 `react-virtuoso`，拥有 visible + overscan、内部高度测量、tail lock、scroll-to-bottom；`main.tsx` 不直接操作 Virtuoso API。
- `main.tsx`：保留项目/session 选择、事件分发和 React 状态编排，不再直接实现 read repair、IndexedDB raw turn 合并、显示索引和滚动窗口。

服务端侧也需要收敛边界：

- `session_recorder.go` 负责 raw turn shape、unfinished tail invariant、prompt_done publish order。
- `session_turn_files.go` 负责 WMT2 连续读取和 read-time `session/gap` projection。
- `client.go` / hub request handler 负责 `session.read` response envelope 和 Session Summary 返回。

旧 `chatTurnWindow.ts` 作为主滚动/显示状态应删除或退役；如果短期保留，只能作为迁移中的测试对照，不参与运行时路径。

## 13. 大内容风险

第一阶段不改变 WMT2 turn 正文格式，不做 DB turn 精简。

后续需要单独设计：

- image content block 不应长期以内联 base64 存在 turn 正文中。
- 超大 tool output / thought / plan 应有 inline size limit。
- 超限内容应转 artifact/ref，UI 显示摘要和展开入口。
