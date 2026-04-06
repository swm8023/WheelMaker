# IM2 Router ClientSession Binding Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 落地 IM2 协议与客户端集成，确保路由键固定为 `imType:chatID`、`/new` 仅重绑当前 chat，并统一使用 `channel` 命名（仅命名对齐，不继承 IM 1.0 实现细节）。

**Architecture:** 以 `internal/im2` 为路由与绑定核心，`Client` 通过显式桥接调用 IM2，`channel` 继续负责平台收发。路由主键直接使用 `routeKey = IMActiveChatID = imType:chatID`，`clientSessionId` 作为 chat 绑定目标。IM2 仅持久化 chat 投影，不接管 client session 生命周期。

**Tech Stack:** Go 1.22+, `modernc.org/sqlite`, existing `internal/hub/client`, `internal/hub/im`, `internal/im2`.

---

## Scope Check

本计划只覆盖一个子系统：`client + im2 + im channel` 路由集成。ACP 协议、agent 运行时、registry 同步不在本计划范围。

## File Structure

| File | Action | Responsibility |
|---|---|---|
| `server/internal/im2/protocol.go` | Modify | 将 `Publisher` 明确为 `Channel`，补齐 routeKey 语义定义 |
| `server/internal/im2/router.go` | Modify | `RegisterChannel`/`UnregisterChannel`、路由分发、显式 Rebind API |
| `server/internal/im2/state.go` | Modify | 增加显式重绑入口（语义上区分 Bind vs Rebind） |
| `server/internal/im2/router_test.go` | Modify | 覆盖 routeKey、broadcast、targeted、rebind 语义 |
| `server/internal/im2/state_test.go` | Modify | 覆盖重绑持久化和 reload 一致性 |
| `server/internal/im2/integration_test.go` | Create | IM2 端到端路由行为测试 |
| `server/internal/hub/im/im.go` | Modify | 增加统一 routeKey helper（`imType:chatID`） |
| `server/internal/hub/im/console/console.go` | Modify | inbound message routeKey 改为 `console:<chatID>` |
| `server/internal/hub/im/feishu/feishu.go` | Modify | inbound message routeKey 改为 `feishu:<chatID>` |
| `server/internal/hub/im/mobile/mobile.go` | Modify | inbound message routeKey 改为 `mobile:<chatID>` |
| `server/internal/hub/im/route_key_test.go` | Create | routeKey helper 和 channel 路由键回归测试 |
| `server/internal/hub/client/im2_bridge.go` | Create | Client->IM2 最小桥接接口 |
| `server/internal/hub/client/client.go` | Modify | HandleMessage 接入 IM2 inbound；暴露 session id allocator |
| `server/internal/hub/client/commands.go` | Modify | `/new` `/load` 后调用 IM2 rebind，保证只影响当前 routeKey |
| `server/internal/hub/client/client_internal_test.go` | Modify | fake IM2 router + 命令重绑回归测试 |
| `server/internal/hub/hub.go` | Modify | 初始化 IM2 router + IM2 state（复用同一个 sqlite 文件） |
| `server/internal/hub/im2_wiring.go` | Create | 提供 IM2 sqlite 路径和装配函数 |
| `server/internal/hub/hub_test.go` | Modify/Create | Hub 层 IM2 wiring 回归测试 |
| `docs/im-protocol-2.0.md` | Modify | 同步实现完成后的行为细节 |
| `docs/architecture-3.0.md` | Modify | 记录 client -> im2 router -> channel 路由关系 |

---

### Task 1: 固化 IM2 的 `channel` 与 `routeKey` 协议边界

**Files:**
- Modify: `server/internal/im2/protocol.go`
- Modify: `server/internal/im2/router.go`
- Modify: `server/internal/im2/router_test.go`

- [ ] **Step 1: 先写失败测试（routeKey 与 channel API）**

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

func TestRouter_RegisterChannelAndTargetPublish(t *testing.T) {
	st := newFakeState()
	_ = st.BindActiveChat(context.Background(), "feishu:oc_123", "feishu", "oc_123", "s1", true)
	ch := &fakeChannel{}
	r, err := NewRouter("proj", st, func(context.Context) (string, error) { return "s1", nil }, nil)
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	if err := r.RegisterChannel("feishu", ch); err != nil {
		t.Fatalf("RegisterChannel: %v", err)
	}
	if err := r.Publish(context.Background(), OutboundEvent{
		Kind:               OutboundMessage,
		ClientSessionID:    "s1",
		TargetActiveChatID: "feishu:oc_123",
		Text:               "hello",
	}); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if ch.count() != 1 {
		t.Fatalf("channel publish count=%d, want 1", ch.count())
	}
}
```

- [ ] **Step 2: 运行测试，确认失败**

Run: `cd server; go test ./internal/im2 -run "TestBuildRouteKey_UsesIMTypeAndChatID|TestRouter_RegisterChannelAndTargetPublish" -v`
Expected: FAIL（`BuildRouteKey`/`RegisterChannel` 尚未定义或行为不匹配）。

- [ ] **Step 3: 实现协议命名与接口收敛**

```go
// protocol.go
// Channel naming is unified; runtime behavior remains IM2-specific.
type Channel interface {
	PublishToChat(ctx context.Context, chatID string, event OutboundEvent) error
}

// BuildRouteKey is the canonical route identity: <imType>:<chatID>.
func BuildRouteKey(imType, chatID string) (string, error) {
	return BuildActiveChatID(imType, chatID)
}
```

```go
// router.go
func (r *Router) RegisterChannel(imType string, ch Channel) error { /* ... */ }
func (r *Router) UnregisterChannel(imType string) { /* ... */ }
```

- [ ] **Step 4: 运行 IM2 单测**

Run: `cd server; go test ./internal/im2 -v`
Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add server/internal/im2/protocol.go server/internal/im2/router.go server/internal/im2/router_test.go
git commit -m "refactor(im2): align channel terminology and routeKey contract"
```

### Task 2: 增加显式 Rebind 语义（支持 `/new` 仅影响当前 chat）

**Files:**
- Modify: `server/internal/im2/router.go`
- Modify: `server/internal/im2/state.go`
- Modify: `server/internal/im2/router_test.go`
- Modify: `server/internal/im2/state_test.go`

- [ ] **Step 1: 先写失败测试（重绑只影响目标 chat）**

```go
func TestRouter_RebindActiveChat_OnlyAffectsTarget(t *testing.T) {
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

	sidA, okA, _ := st.ResolveClientSessionID(context.Background(), "feishu:chat-a")
	sidB, okB, _ := st.ResolveClientSessionID(context.Background(), "feishu:chat-b")
	if !okA || sidA != "s-new" {
		t.Fatalf("chat-a sid=%q, want s-new", sidA)
	}
	if !okB || sidB != "s-old" {
		t.Fatalf("chat-b sid=%q, want s-old", sidB)
	}
}
```

- [ ] **Step 2: 运行测试，确认失败**

Run: `cd server; go test ./internal/im2 -run TestRouter_RebindActiveChat_OnlyAffectsTarget -v`
Expected: FAIL（`RebindActiveChat` 尚未实现）。

- [ ] **Step 3: 增加显式重绑 API（critical sync）**

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

- [ ] **Step 4: 跑 IM2 回归测试**

Run: `cd server; go test ./internal/im2 -v`
Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add server/internal/im2/router.go server/internal/im2/state.go server/internal/im2/router_test.go server/internal/im2/state_test.go
git commit -m "feat(im2): add explicit active-chat rebind semantics"
```

### Task 3: 统一所有 IM channel 的 routeKey 生成规则

**Files:**
- Modify: `server/internal/hub/im/im.go`
- Modify: `server/internal/hub/im/console/console.go`
- Modify: `server/internal/hub/im/feishu/feishu.go`
- Modify: `server/internal/hub/im/mobile/mobile.go`
- Create: `server/internal/hub/im/route_key_test.go`

- [ ] **Step 1: 先写失败测试（routeKey helper）**

```go
func TestBuildRouteKey(t *testing.T) {
	got := BuildRouteKey("feishu", "oc_xxx")
	if got != "feishu:oc_xxx" {
		t.Fatalf("routeKey=%q, want feishu:oc_xxx", got)
	}
}

func TestBuildRouteKey_EmptyInput(t *testing.T) {
	if got := BuildRouteKey("", "chat"); got != "" {
		t.Fatalf("routeKey=%q, want empty", got)
	}
}
```

- [ ] **Step 2: 运行测试，确认失败**

Run: `cd server; go test ./internal/hub/im -run TestBuildRouteKey -v`
Expected: FAIL（helper 尚未定义）。

- [ ] **Step 3: 实现 helper 并替换各 channel 出口**

```go
// im.go
func BuildRouteKey(imType, chatID string) string {
	imType = strings.TrimSpace(strings.ToLower(imType))
	chatID = strings.TrimSpace(chatID)
	if imType == "" || chatID == "" {
		return ""
	}
	return imType + ":" + chatID
}
```

```go
// console/console.go
RouteKey: im.BuildRouteKey("console", c.projectName)

// feishu/feishu.go
RouteKey: im.BuildRouteKey("feishu", *msg.ChatId)

// mobile/mobile.go
RouteKey: im.BuildRouteKey("mobile", chatID)
```

- [ ] **Step 4: 运行 IM 包测试**

Run: `cd server; go test ./internal/hub/im/... -v`
Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add server/internal/hub/im/im.go server/internal/hub/im/console/console.go server/internal/hub/im/feishu/feishu.go server/internal/hub/im/mobile/mobile.go server/internal/hub/im/route_key_test.go
git commit -m "refactor(im): standardize routeKey as imType:chatID across channels"
```

### Task 4: 在 Client 增加 IM2 路由桥接边界

**Files:**
- Create: `server/internal/hub/client/im2_bridge.go`
- Modify: `server/internal/hub/client/client.go`
- Modify: `server/internal/hub/client/client_internal_test.go`

- [ ] **Step 1: 先写失败测试（Client 能接 IM2 router）**

```go
type fakeIM2Router struct {
	inbound []string
	rebindCalls []struct {
		activeChatID    string
		clientSessionID string
	}
}

func (f *fakeIM2Router) HandleInbound(_ context.Context, e im2.InboundEvent) error {
	f.inbound = append(f.inbound, e.ActiveChatID+"|"+e.ClientSessionID)
	return nil
}

func (f *fakeIM2Router) RebindActiveChat(_ context.Context, activeChatID, clientSessionID string) error {
	f.rebindCalls = append(f.rebindCalls, struct {
		activeChatID    string
		clientSessionID string
	}{activeChatID: activeChatID, clientSessionID: clientSessionID})
	return nil
}

func TestClient_HandleMessage_IM2BridgeNormalizesInbound(t *testing.T) {
	c := New(&noopStore{}, nil, "proj", "/tmp")
	r := &fakeIM2Router{}
	c.SetIM2Router(r)

	c.HandleMessage(im.Message{ChatID: "oc_123", RouteKey: "feishu:oc_123", Text: "hello"})
	if len(r.inbound) != 1 {
		t.Fatalf("inbound calls=%d, want 1", len(r.inbound))
	}
	if !strings.Contains(r.inbound[0], "feishu:oc_123") {
		t.Fatalf("inbound payload=%q", r.inbound[0])
	}
}
```

- [ ] **Step 2: 运行测试，确认失败**

Run: `cd server; go test ./internal/hub/client -run TestClient_HandleMessage_IM2BridgeNormalizesInbound -v`
Expected: FAIL（`SetIM2Router` 与 bridge 未实现）。

- [ ] **Step 3: 增加 Client->IM2 最小桥接接口**

```go
// im2_bridge.go
type IM2Router interface {
	HandleInbound(ctx context.Context, event im2.InboundEvent) error
	RebindActiveChat(ctx context.Context, activeChatID, clientSessionID string) error
}

func (c *Client) SetIM2Router(r IM2Router) {
	c.mu.Lock()
	c.im2Router = r
	c.mu.Unlock()
}
```

```go
// client.go (HandleMessage 开头)
if c.im2Router != nil {
	_ = c.im2Router.HandleInbound(context.Background(), im2.InboundEvent{
		Kind:         im2.InboundPrompt,
		ActiveChatID: strings.TrimSpace(msg.RouteKey),
		Text:         strings.TrimSpace(msg.Text),
	})
}
```

- [ ] **Step 4: 跑 client 测试**

Run: `cd server; go test ./internal/hub/client -v`
Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add server/internal/hub/client/im2_bridge.go server/internal/hub/client/client.go server/internal/hub/client/client_internal_test.go
git commit -m "feat(client): add IM2 router bridge boundary"
```

### Task 5: 将 `/new` 与 `/load` 重绑到 IM2（仅当前 routeKey）

**Files:**
- Modify: `server/internal/hub/client/commands.go`
- Modify: `server/internal/hub/client/client_internal_test.go`

- [ ] **Step 1: 先写失败测试（/new 重绑当前 route）**

```go
func TestHandleNewCommand_RebindsOnlyCurrentRoute(t *testing.T) {
	c := New(&noopStore{}, nil, "proj", "/tmp")
	fake := &fakeIM2Router{}
	c.SetIM2Router(fake)

	msg := im.Message{ChatID: "oc_1", RouteKey: "feishu:oc_1", Text: "/new"}
	c.HandleMessage(msg)

	if len(fake.rebindCalls) != 1 {
		t.Fatalf("rebind calls=%d, want 1", len(fake.rebindCalls))
	}
	if fake.rebindCalls[0].activeChatID != "feishu:oc_1" {
		t.Fatalf("activeChatID=%q", fake.rebindCalls[0].activeChatID)
	}
	if strings.TrimSpace(fake.rebindCalls[0].clientSessionID) == "" {
		t.Fatal("clientSessionID should not be empty")
	}
}
```

- [ ] **Step 2: 运行测试，确认失败**

Run: `cd server; go test ./internal/hub/client -run TestHandleNewCommand_RebindsOnlyCurrentRoute -v`
Expected: FAIL（命令路径尚未调用 IM2 rebind）。

- [ ] **Step 3: 在命令处理路径接入 rebind**

```go
// commands.go - handleNewCommand
newSess := c.ClientNewSession(routeKey)
if c.im2Router != nil {
	_ = c.im2Router.RebindActiveChat(context.Background(), routeKey, newSess.ID)
}

// commands.go - handleLoadCommand
loaded, err := c.ClientLoadSession(routeKey, idx)
if err == nil && c.im2Router != nil {
	_ = c.im2Router.RebindActiveChat(context.Background(), routeKey, loaded.ID)
}
```

- [ ] **Step 4: 运行 client 回归**

Run: `cd server; go test ./internal/hub/client -v`
Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add server/internal/hub/client/commands.go server/internal/hub/client/client_internal_test.go
git commit -m "feat(client): rebind current route in IM2 on /new and /load"
```

### Task 6: Hub 层接入 IM2 router + IM2 state（统一 sqlite）

**Files:**
- Modify: `server/internal/hub/hub.go`
- Create: `server/internal/hub/im2_wiring.go`
- Modify/Create: `server/internal/hub/hub_test.go`

- [ ] **Step 1: 先写失败测试（Hub 会创建 IM2 router）**

```go
func TestBuildClient_WiresIM2Router(t *testing.T) {
	cfg := &shared.AppConfig{
		Projects: []shared.ProjectConfig{{
			Name: "proj",
			Path: ".",
			IM:   shared.IMConfig{Type: "console"},
			Client: shared.ClientConf{Agent: "codex"},
		}},
	}
	h := New(cfg, filepath.Join(t.TempDir(), "state.json"))
	c, err := h.buildClient(context.Background(), cfg.Projects[0])
	if err != nil {
		t.Fatalf("buildClient: %v", err)
	}
	if c == nil {
		t.Fatal("client is nil")
	}
	if !c.HasIM2Router() {
		t.Fatal("expected IM2 router to be wired")
	}
}
```

- [ ] **Step 2: 运行测试，确认失败**

Run: `cd server; go test ./internal/hub -run TestBuildClient_WiresIM2Router -v`
Expected: FAIL（尚未 wiring）。

- [ ] **Step 3: 接入 IM2 状态与路由器初始化**

```go
// im2_wiring.go
func buildIM2StatePath(statePath string) string {
	dir := filepath.Dir(statePath)
	return filepath.Join(dir, "state.db")
}

// client.go
func (c *Client) ClientNewSessionID() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.nextSessionID()
}

func (c *Client) HasIM2Router() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.im2Router != nil
}

// hub.go -> buildClient
im2State, err := im2.NewState(buildIM2StatePath(h.statePath), pc.Name)
if err != nil { return nil, err }
router, err := im2.NewRouter(pc.Name, im2State, func(context.Context) (string, error) {
	return c.ClientNewSessionID(), nil
}, nil)
if err != nil { return nil, err }
c.SetIM2Router(router)
```

- [ ] **Step 4: 运行 hub + 相关回归测试**

Run: `cd server; go test ./internal/hub/... -v`
Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add server/internal/hub/hub.go server/internal/hub/im2_wiring.go server/internal/hub/hub_test.go server/internal/hub/client/client.go
git commit -m "feat(hub): wire IM2 router/state with shared sqlite lifecycle"
```

### Task 7: IM2 端到端回归与文档同步

**Files:**
- Create: `server/internal/im2/integration_test.go`
- Modify: `docs/im-protocol-2.0.md`
- Modify: `docs/architecture-3.0.md`

- [ ] **Step 1: 增加端到端集成测试（3 条）**

```go
func TestIM2Integration_FirstInboundCreatesBinding(t *testing.T) {
	st := newFakeState()
	var got InboundEvent
	r, _ := NewRouter("proj", st, func(context.Context) (string, error) { return "s-1", nil }, func(_ context.Context, ev InboundEvent) error {
		got = ev
		return nil
	})
	_ = r.HandleInbound(context.Background(), InboundEvent{Kind: InboundPrompt, IMType: "feishu", ChatID: "chat-a", Text: "hello"})
	if got.ClientSessionID != "s-1" || got.ActiveChatID != "feishu:chat-a" {
		t.Fatalf("got=(%q,%q)", got.ClientSessionID, got.ActiveChatID)
	}
}

func TestIM2Integration_RebindOnlyCurrentChat(t *testing.T) {
	st := newFakeState()
	_ = st.BindActiveChat(context.Background(), "feishu:chat-a", "feishu", "chat-a", "s-old", true)
	_ = st.BindActiveChat(context.Background(), "feishu:chat-b", "feishu", "chat-b", "s-old", true)
	r, _ := NewRouter("proj", st, func(context.Context) (string, error) { return "s-new", nil }, nil)
	_ = r.RebindActiveChat(context.Background(), "feishu:chat-a", "s-new")
	a, _, _ := st.ResolveClientSessionID(context.Background(), "feishu:chat-a")
	b, _, _ := st.ResolveClientSessionID(context.Background(), "feishu:chat-b")
	if a != "s-new" || b != "s-old" {
		t.Fatalf("a=%q b=%q", a, b)
	}
}

func TestIM2Integration_BroadcastFanoutByClientSession(t *testing.T) {
	st := newFakeState()
	_ = st.BindActiveChat(context.Background(), "feishu:chat-a", "feishu", "chat-a", "s-1", true)
	_ = st.BindActiveChat(context.Background(), "feishu:chat-b", "feishu", "chat-b", "s-1", true)
	ch := &fakeChannel{}
	r, _ := NewRouter("proj", st, func(context.Context) (string, error) { return "unused", nil }, nil)
	_ = r.RegisterChannel("feishu", ch)
	_ = r.Publish(context.Background(), OutboundEvent{Kind: OutboundMessage, ClientSessionID: "s-1", Text: "broadcast"})
	if ch.count() != 2 {
		t.Fatalf("channel count=%d, want 2", ch.count())
	}
}
```

- [ ] **Step 2: 运行完整测试集**

Run: `cd server; go test ./...`
Expected: PASS。

- [ ] **Step 3: 同步文档到最终实现行为**

```md
- routeKey source: imType:chatID (channel-generated chatID)
- /new rebind scope: current active chat only
- channel term replaces publisher term in code and docs
```

- [ ] **Step 4: 提交文档与测试收尾**

```bash
git add -A
git commit -m "test+docs(im2): finalize route rebind and channel-based integration"
```

- [ ] **Step 5: 推送与完成门禁**

```bash
git push origin main
```

## Self-Review Notes

1. Spec coverage:
   - `routeKey = imType:chatID`：Task 1 + Task 3。
   - `channel` 术语替换 `publisher`：Task 1 + Task 3 + Task 7。
   - `/new` 仅重绑当前 chat：Task 2 + Task 5。
   - IM2 仅存 chat 投影：Task 2 + Task 6。
   - 协议无 `userId`：Task 1/7 回归。
2. Placeholder scan:
   - 无 `TBD/TODO`，每个任务包含具体文件、测试命令、提交命令。
3. Type consistency:
   - `routeKey/activeChatID/clientSessionId/channel` 命名在所有任务保持一致。

