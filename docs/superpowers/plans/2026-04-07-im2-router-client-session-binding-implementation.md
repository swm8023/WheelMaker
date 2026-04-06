# IM2 Router RouteKey Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在不修改 `server/internal/hub/im/*` 的前提下，实现 IM2 独立路由：Router 注册 IM 通道、统一生成 `routeKey=imType:chatID`、入站下发给 Client、普通回复按 routeKey 定点回发。

**Architecture:** `internal/im2` 完整承担路由与绑定逻辑；`client` 消费/发布标准事件；`hub` 通过新增 adapter 把现有 IM 输入桥接到 IM2 Router。`clientSessionId` 仅用于绑定与广播分组，普通回复默认使用当前会话绑定的 routeKey。

**Tech Stack:** Go 1.22+, `modernc.org/sqlite`, `internal/im2`, `internal/hub/client`, `internal/hub/hub`.

---

## Scope Check

本计划只覆盖 IM2 路由链路，不包含：

1. `server/internal/hub/im/*` 改动
2. ACP 协议/agent runtime 改动
3. registry 同步逻辑改动

## File Structure

| File | Action | Responsibility |
|---|---|---|
| `server/internal/im2/protocol.go` | Modify | 定义 `Channel` 与 routeKey helper |
| `server/internal/im2/router.go` | Modify | channel 注册、inbound/outbound、rebind |
| `server/internal/im2/state.go` | Modify | 显式 `RebindActiveChat` |
| `server/internal/im2/router_test.go` | Modify | routeKey + target/broadcast + rebind 测试 |
| `server/internal/im2/state_test.go` | Modify | rebind 持久化与重启一致性 |
| `server/internal/im2/integration_test.go` | Create | IM2 端到端路由行为测试 |
| `server/internal/hub/client/im2_bridge.go` | Create | Client <-> IM2 Router 接口 |
| `server/internal/hub/client/client.go` | Modify | inbound 接 IM2、routeKey 绑定、session id allocator |
| `server/internal/hub/client/session.go` | Modify | reply 走 routeKey 定点回发 |
| `server/internal/hub/client/commands.go` | Modify | `/new` `/load` 后 rebind 当前 routeKey |
| `server/internal/hub/client/client_internal_test.go` | Modify | client+im2 bridge 回归 |
| `server/internal/hub/im2_adapter.go` | Create | 旧 IM 输入桥接到 IM2（不改 IM 包） |
| `server/internal/hub/hub.go` | Modify | buildClient 装配 IM2 state/router/adapter |
| `server/internal/hub/hub_test.go` | Modify/Create | Hub wiring 回归 |
| `docs/im-protocol-2.0.md` | Modify | 同步 routeKey 入站/回包行为 |
| `docs/architecture-3.0.md` | Modify | 补充新链路图 |

---

### Task 1: 固化 IM2 routeKey 与 Channel 合约

**Files:**
- Modify: `server/internal/im2/protocol.go`
- Modify: `server/internal/im2/router.go`
- Modify: `server/internal/im2/router_test.go`

- [ ] **Step 1: 写失败测试**

```go
func TestBuildRouteKey_UsesIMTypeAndChatID(t *testing.T) {
	got, err := BuildRouteKey("feishu", "oc_123")
	if err != nil {
		t.Fatalf("BuildRouteKey: %v", err)
	}
	if got != "feishu:oc_123" {
		t.Fatalf("routeKey=%q, want feishu:oc_123", got)
	}
}

func TestRouter_RegisterChannelAndTargetReply(t *testing.T) {
	st := newFakeState()
	_ = st.BindActiveChat(context.Background(), "feishu:oc_123", "feishu", "oc_123", "sess-1", true)
	ch := &fakeChannel{}
	r, err := NewRouter("proj", st, func(context.Context) (string, error) { return "sess-1", nil }, nil)
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	if err := r.RegisterChannel("feishu", ch); err != nil {
		t.Fatalf("RegisterChannel: %v", err)
	}
	if err := r.Publish(context.Background(), OutboundEvent{
		Kind:               OutboundMessage,
		ClientSessionID:    "sess-1",
		TargetActiveChatID: "feishu:oc_123",
		Text:               "hello",
	}); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if ch.count() != 1 {
		t.Fatalf("channel count=%d, want 1", ch.count())
	}
}
```

- [ ] **Step 2: 运行失败测试**

Run: `cd server; go test ./internal/im2 -run "TestBuildRouteKey_UsesIMTypeAndChatID|TestRouter_RegisterChannelAndTargetReply" -v`
Expected: FAIL。

- [ ] **Step 3: 实现最小代码**

```go
// protocol.go
type Channel interface {
	PublishToChat(ctx context.Context, chatID string, event OutboundEvent) error
}

func BuildRouteKey(imType, chatID string) (string, error) {
	return BuildActiveChatID(imType, chatID)
}

// router.go
func (r *Router) RegisterChannel(imType string, ch Channel) error {
	key := strings.ToLower(strings.TrimSpace(imType))
	if key == "" || ch == nil {
		return fmt.Errorf("im2 router: invalid channel registration")
	}
	r.mu.Lock()
	r.channels[key] = ch
	r.mu.Unlock()
	return nil
}

func (r *Router) UnregisterChannel(imType string) {
	key := strings.ToLower(strings.TrimSpace(imType))
	if key == "" {
		return
	}
	r.mu.Lock()
	delete(r.channels, key)
	r.mu.Unlock()
}
```

- [ ] **Step 4: 跑 IM2 测试**

Run: `cd server; go test ./internal/im2 -v`
Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add server/internal/im2/protocol.go server/internal/im2/router.go server/internal/im2/router_test.go
git commit -m "refactor(im2): define routeKey-first channel contract"
```

### Task 2: 实现 rebind 语义（/new 只影响当前 routeKey）

**Files:**
- Modify: `server/internal/im2/router.go`
- Modify: `server/internal/im2/state.go`
- Modify: `server/internal/im2/router_test.go`
- Modify: `server/internal/im2/state_test.go`

- [ ] **Step 1: 写失败测试**

```go
func TestRouter_RebindActiveChat_OnlyAffectsTargetRouteKey(t *testing.T) {
	st := newFakeState()
	_ = st.BindActiveChat(context.Background(), "feishu:chat-a", "feishu", "chat-a", "s-old", true)
	_ = st.BindActiveChat(context.Background(), "feishu:chat-b", "feishu", "chat-b", "s-old", true)

	r, err := NewRouter("proj", st, func(context.Context) (string, error) { return "unused", nil }, nil)
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	if err := r.RebindActiveChat(context.Background(), "feishu:chat-a", "s-new"); err != nil {
		t.Fatalf("RebindActiveChat: %v", err)
	}

	a, _, _ := st.ResolveClientSessionID(context.Background(), "feishu:chat-a")
	b, _, _ := st.ResolveClientSessionID(context.Background(), "feishu:chat-b")
	if a != "s-new" || b != "s-old" {
		t.Fatalf("got a=%q b=%q", a, b)
	}
}
```

- [ ] **Step 2: 运行失败测试**

Run: `cd server; go test ./internal/im2 -run TestRouter_RebindActiveChat_OnlyAffectsTargetRouteKey -v`
Expected: FAIL。

- [ ] **Step 3: 实现 rebind**

```go
// state.go
func (s *sqliteState) RebindActiveChat(ctx context.Context, activeChatID, clientSessionID string) error {
	imType, chatID, ok := ParseActiveChatID(activeChatID)
	if !ok {
		return fmt.Errorf("im2 state: invalid activeChatID %q", activeChatID)
	}
	return s.BindActiveChat(ctx, activeChatID, imType, chatID, clientSessionID, true)
}

// router.go
func (r *Router) RebindActiveChat(ctx context.Context, activeChatID, clientSessionID string) error {
	return r.state.RebindActiveChat(ctx, strings.TrimSpace(activeChatID), strings.TrimSpace(clientSessionID))
}
```

- [ ] **Step 4: 跑 IM2 测试**

Run: `cd server; go test ./internal/im2 -v`
Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add server/internal/im2/router.go server/internal/im2/state.go server/internal/im2/router_test.go server/internal/im2/state_test.go
git commit -m "feat(im2): add routeKey rebind semantics"
```

### Task 3: Client 回包按 routeKey 定点

**Files:**
- Create: `server/internal/hub/client/im2_bridge.go`
- Modify: `server/internal/hub/client/client.go`
- Modify: `server/internal/hub/client/session.go`
- Modify: `server/internal/hub/client/client_internal_test.go`

- [ ] **Step 1: 写失败测试**

```go
func TestSessionReply_UsesBoundRouteKeyViaIM2(t *testing.T) {
	c := New(&noopStore{}, nil, "proj", "/tmp")
	fake := &fakeIM2Router{}
	c.SetIM2Router(fake)

	c.HandleMessage(im.Message{ChatID: "oc_1", RouteKey: "feishu:oc_1", Text: "hello"})
	c.activeSession.reply("world")

	if len(fake.outbound) == 0 {
		t.Fatal("expected outbound via IM2")
	}
	if fake.outbound[0].TargetActiveChatID != "feishu:oc_1" {
		t.Fatalf("target=%q", fake.outbound[0].TargetActiveChatID)
	}
}
```

- [ ] **Step 2: 运行失败测试**

Run: `cd server; go test ./internal/hub/client -run TestSessionReply_UsesBoundRouteKeyViaIM2 -v`
Expected: FAIL。

- [ ] **Step 3: 实现 bridge + routeKey 绑定**

```go
// im2_bridge.go
type IM2Router interface {
	HandleInbound(ctx context.Context, event im2.InboundEvent) error
	Publish(ctx context.Context, event im2.OutboundEvent) error
	RebindActiveChat(ctx context.Context, activeChatID, clientSessionID string) error
}

// client.go
func (c *Client) SetIM2Router(r IM2Router) {
	c.mu.Lock()
	c.im2Router = r
	c.mu.Unlock()
}

// session.go (reply)
if s.im2Router != nil && strings.TrimSpace(s.boundRouteKey) != "" {
	_ = s.im2Router.Publish(context.Background(), im2.OutboundEvent{
		Kind:               im2.OutboundMessage,
		ClientSessionID:    s.ID,
		TargetActiveChatID: s.boundRouteKey,
		Text:               text,
	})
	return
}
```

- [ ] **Step 4: 跑 client 测试**

Run: `cd server; go test ./internal/hub/client -v`
Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add server/internal/hub/client/im2_bridge.go server/internal/hub/client/client.go server/internal/hub/client/session.go server/internal/hub/client/client_internal_test.go
git commit -m "feat(client): route replies by bound routeKey through IM2"
```

### Task 4: Hub 桥接现有 IM 输入到 IM2（不改 IM 包）

**Files:**
- Create: `server/internal/hub/im2_adapter.go`
- Modify: `server/internal/hub/hub.go`
- Modify/Create: `server/internal/hub/hub_test.go`

- [ ] **Step 1: 写失败测试**

```go
func TestBuildClient_WiresIM2WithoutTouchingIMPackage(t *testing.T) {
	cfg := &shared.AppConfig{Projects: []shared.ProjectConfig{{
		Name: "proj",
		Path: ".",
		IM:   shared.IMConfig{Type: "console"},
	}}}
	h := New(cfg, filepath.Join(t.TempDir(), "state.json"))
	c, err := h.buildClient(context.Background(), cfg.Projects[0])
	if err != nil {
		t.Fatalf("buildClient: %v", err)
	}
	if c == nil || !c.HasIM2Router() {
		t.Fatal("expected client with IM2 router")
	}
}
```

- [ ] **Step 2: 运行失败测试**

Run: `cd server; go test ./internal/hub -run TestBuildClient_WiresIM2WithoutTouchingIMPackage -v`
Expected: FAIL。

- [ ] **Step 3: 实现 adapter 与装配**

```go
// im2_adapter.go
type IM2Bridge struct {
	adapter     *im.ImAdapter
	router      *im2.Router
	defaultType string
}

func (b *IM2Bridge) Start() {
	b.adapter.OnMessage(func(m im.Message) {
		_ = b.router.HandleInbound(context.Background(), im2.InboundEvent{
			Kind:   im2.InboundPrompt,
			IMType: b.defaultType,
			ChatID: m.ChatID,
			Text:   m.Text,
		})
	})
}

// hub.go buildClient
router, err := im2.NewRouter(pc.Name, im2State, func(context.Context) (string, error) {
	return c.ClientNewSessionID(), nil
}, nil)
if err != nil {
	return nil, err
}
c.SetIM2Router(router)
bridge := &IM2Bridge{adapter: imProvider, router: router, defaultType: pc.IM.Type}
bridge.Start()
```

- [ ] **Step 4: 跑 hub 测试**

Run: `cd server; go test ./internal/hub/... -v`
Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add server/internal/hub/im2_adapter.go server/internal/hub/hub.go server/internal/hub/hub_test.go
git commit -m "feat(hub): bridge existing IM input to IM2 router without IM package changes"
```

### Task 5: `/new` `/load` 后重绑当前 routeKey

**Files:**
- Modify: `server/internal/hub/client/commands.go`
- Modify: `server/internal/hub/client/client_internal_test.go`

- [ ] **Step 1: 写失败测试**

```go
func TestHandleNewCommand_RebindsCurrentRouteKeyOnly(t *testing.T) {
	c := New(&noopStore{}, nil, "proj", "/tmp")
	fake := &fakeIM2Router{}
	c.SetIM2Router(fake)

	c.HandleMessage(im.Message{ChatID: "a", RouteKey: "feishu:chat-a", Text: "/new"})

	if len(fake.rebindCalls) != 1 {
		t.Fatalf("rebind calls=%d, want 1", len(fake.rebindCalls))
	}
	if fake.rebindCalls[0].activeChatID != "feishu:chat-a" {
		t.Fatalf("activeChatID=%q", fake.rebindCalls[0].activeChatID)
	}
}
```

- [ ] **Step 2: 运行失败测试**

Run: `cd server; go test ./internal/hub/client -run TestHandleNewCommand_RebindsCurrentRouteKeyOnly -v`
Expected: FAIL。

- [ ] **Step 3: 实现 rebind 调用**

```go
// commands.go
newSess := c.ClientNewSession(routeKey)
if c.im2Router != nil {
	_ = c.im2Router.RebindActiveChat(context.Background(), routeKey, newSess.ID)
}

loaded, err := c.ClientLoadSession(routeKey, idx)
if err == nil && c.im2Router != nil {
	_ = c.im2Router.RebindActiveChat(context.Background(), routeKey, loaded.ID)
}
```

- [ ] **Step 4: 跑 client 测试**

Run: `cd server; go test ./internal/hub/client -v`
Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add server/internal/hub/client/commands.go server/internal/hub/client/client_internal_test.go
git commit -m "feat(client): rebind current routeKey on new/load"
```

### Task 6: 端到端与文档收尾

**Files:**
- Create: `server/internal/im2/integration_test.go`
- Modify: `docs/im-protocol-2.0.md`
- Modify: `docs/architecture-3.0.md`

- [ ] **Step 1: 写端到端测试**

```go
func TestIM2_E2E_InboundCarriesRouteKeyToClient(t *testing.T) {
	st := newFakeState()
	var got InboundEvent
	r, _ := NewRouter("proj", st, func(context.Context) (string, error) { return "s-1", nil }, func(_ context.Context, ev InboundEvent) error {
		got = ev
		return nil
	})
	_ = r.HandleInbound(context.Background(), InboundEvent{Kind: InboundPrompt, IMType: "feishu", ChatID: "chat-a", Text: "hello"})
	if got.ActiveChatID != "feishu:chat-a" || got.ClientSessionID != "s-1" {
		t.Fatalf("got activeChatID=%q clientSessionID=%q", got.ActiveChatID, got.ClientSessionID)
	}
}

func TestIM2_E2E_NormalReplyUsesSameRouteKey(t *testing.T) {
	st := newFakeState()
	_ = st.BindActiveChat(context.Background(), "feishu:chat-a", "feishu", "chat-a", "s-1", true)
	ch := &fakeChannel{}
	r, _ := NewRouter("proj", st, func(context.Context) (string, error) { return "unused", nil }, nil)
	_ = r.RegisterChannel("feishu", ch)
	_ = r.Publish(context.Background(), OutboundEvent{
		Kind:               OutboundMessage,
		ClientSessionID:    "s-1",
		TargetActiveChatID: "feishu:chat-a",
		Text:               "reply",
	})
	if ch.count() != 1 {
		t.Fatalf("channel count=%d, want 1", ch.count())
	}
}

func TestIM2_E2E_NewRebindOnlyCurrentRouteKey(t *testing.T) {
	st := newFakeState()
	_ = st.BindActiveChat(context.Background(), "feishu:chat-a", "feishu", "chat-a", "s-old", true)
	_ = st.BindActiveChat(context.Background(), "feishu:chat-b", "feishu", "chat-b", "s-old", true)
	r, _ := NewRouter("proj", st, func(context.Context) (string, error) { return "s-new", nil }, nil)
	_ = r.RebindActiveChat(context.Background(), "feishu:chat-a", "s-new")
	a, _, _ := st.ResolveClientSessionID(context.Background(), "feishu:chat-a")
	b, _, _ := st.ResolveClientSessionID(context.Background(), "feishu:chat-b")
	if a != "s-new" || b != "s-old" {
		t.Fatalf("got a=%q b=%q", a, b)
	}
}
```

- [ ] **Step 2: 运行全量测试**

Run: `cd server; go test ./...`
Expected: PASS。

- [ ] **Step 3: 更新文档**

```md
- im2 与 im 实现隔离，im 包不改
- routeKey 由 Router 统一生成：imType:chatID
- inbound 与 normal reply 均以 routeKey 为主
```

- [ ] **Step 4: 提交收尾**

```bash
git add -A
git commit -m "test+docs(im2): finalize routeKey-driven isolated IM2 flow"
```

- [ ] **Step 5: 推送**

```bash
git push origin main
```

## Self-Review Notes

1. Spec coverage:
   - IM2/IM 隔离：Task 4 + Task 6。
   - routeKey 入站与回包：Task 1 + Task 3。
   - `/new` 仅重绑当前 routeKey：Task 2 + Task 5。
2. Placeholder scan:
   - 无 TBD/TODO；每个任务含具体文件、命令、提交。
3. Type consistency:
   - 统一使用 `routeKey(activeChatID)`、`clientSessionId`、`Channel`。
