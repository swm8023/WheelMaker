# IM2 Router Multi-Session Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build an isolated IM2 router package with multi-session chat bindings, watch fanout, normalized history hooks, a Feishu channel implementation, and an App channel stub while keeping IM 1.0 as the default runtime.

**Architecture:** `server/internal/im2` owns protocol types, bindings, routing, and history. IM channels implement a minimal `Channel` contract; Feishu adapts existing Feishu rendering through composition with the IM1 Feishu channel, while App is a stub for future multi-chat support. No default `hub.buildClient` wiring changes are made.

**Tech Stack:** Go 1.22+, standard library concurrency/error handling, existing `server/internal/hub/im/feishu` package for Feishu rendering reuse, `go test ./...`.

---

## Scope Check

The spec covers one subsystem: an isolated IM2 package and channel implementations. Client and Hub production wiring are deferred; this plan only adds contracts, router behavior, channel packages, tests, and docs.

## File Structure

| File | Action | Responsibility |
|---|---|---|
| `server/internal/im2/protocol.go` | Create | Shared IM2 types: `ChatRef`, inbound/outbound events, binding options, send target, channel interface |
| `server/internal/im2/history.go` | Create | History interfaces and in-memory store |
| `server/internal/im2/router.go` | Create | Channel registry, chat binding, inbound dispatch, send fanout |
| `server/internal/im2/router_test.go` | Create | Router binding, send, watch, and isolation behavior tests |
| `server/internal/im2/history_test.go` | Create | In-memory history tests |
| `server/internal/im2/app/app.go` | Create | App channel stub implementing `im2.Channel` |
| `server/internal/im2/app/app_test.go` | Create | App stub contract and multi-chat router tests |
| `server/internal/im2/feishu/feishu.go` | Create | Feishu channel adapter implementing `im2.Channel` |
| `server/internal/im2/feishu/feishu_test.go` | Create | Feishu contract and mapping tests |
| `docs/superpowers/plans/2026-04-07-im2-router-multisession-implementation.md` | Create | This implementation plan |

## Task 1: Router Protocol And Core Binding Flow

**Files:**
- Create: `server/internal/im2/protocol.go`
- Create: `server/internal/im2/router.go`
- Create: `server/internal/im2/router_test.go`

- [ ] **Step 1: Write failing router tests**

Add `server/internal/im2/router_test.go`:

```go
package im2

import (
	"context"
	"testing"
)

type captureInboundClient struct {
	events []InboundEvent
}

func (c *captureInboundClient) HandleIM2Inbound(_ context.Context, event InboundEvent) error {
	c.events = append(c.events, event)
	return nil
}

type captureChannel struct {
	id    string
	sent  []sentEvent
	onMsg func(context.Context, string, string) error
}

type sentEvent struct {
	chatID string
	event  OutboundEvent
}

func (c *captureChannel) ID() string { return c.id }
func (c *captureChannel) OnMessage(fn func(context.Context, string, string) error) {
	c.onMsg = fn
}
func (c *captureChannel) Send(_ context.Context, chatID string, event OutboundEvent) error {
	c.sent = append(c.sent, sentEvent{chatID: chatID, event: event})
	return nil
}
func (c *captureChannel) Run(context.Context) error { return nil }

func TestHandleInbound_UnboundChatReachesClientWithoutSession(t *testing.T) {
	ctx := context.Background()
	client := &captureInboundClient{}
	router := NewRouter(client, nil)
	ch := &captureChannel{id: "feishu"}
	if err := router.RegisterChannel(ch); err != nil {
		t.Fatalf("RegisterChannel: %v", err)
	}

	if err := ch.onMsg(ctx, "chat-a", "hello"); err != nil {
		t.Fatalf("onMsg: %v", err)
	}

	if len(client.events) != 1 {
		t.Fatalf("events=%d, want 1", len(client.events))
	}
	got := client.events[0]
	if got.ChannelID != "feishu" || got.ChatID != "chat-a" || got.Text != "hello" || got.SessionID != "" {
		t.Fatalf("event=%+v", got)
	}
}

func TestBind_CausesLaterInboundToCarrySessionID(t *testing.T) {
	ctx := context.Background()
	client := &captureInboundClient{}
	router := NewRouter(client, nil)
	ch := &captureChannel{id: "app"}
	_ = router.RegisterChannel(ch)
	if err := router.Bind(ctx, ChatRef{ChannelID: "app", ChatID: "chat-a"}, "session-1", BindOptions{}); err != nil {
		t.Fatalf("Bind: %v", err)
	}

	if err := ch.onMsg(ctx, "chat-a", "hello"); err != nil {
		t.Fatalf("onMsg: %v", err)
	}

	if got := client.events[0].SessionID; got != "session-1" {
		t.Fatalf("SessionID=%q, want session-1", got)
	}
}
```

- [ ] **Step 2: Run the tests and verify RED**

Run: `cd server; go test ./internal/im2 -run "TestHandleInbound_UnboundChatReachesClientWithoutSession|TestBind_CausesLaterInboundToCarrySessionID" -v`

Expected: FAIL because `server/internal/im2` and its types do not exist.

- [ ] **Step 3: Implement minimal protocol and router**

Create `server/internal/im2/protocol.go`:

```go
package im2

import "context"

type ChatRef struct {
	ChannelID string
	ChatID    string
}

type BindOptions struct {
	Watch bool
}

type InboundEvent struct {
	ChannelID string
	ChatID    string
	Text      string
	SessionID string
}

type OutboundKind string

const (
	OutboundMessage OutboundKind = "message"
	OutboundACP     OutboundKind = "acp"
	OutboundSystem  OutboundKind = "system"
)

type OutboundEvent struct {
	Kind    OutboundKind
	Payload any
}

type SendTarget struct {
	ChannelID string
	ChatID    string
	SessionID string
	Source    *ChatRef
}

type Channel interface {
	ID() string
	OnMessage(func(ctx context.Context, chatID string, text string) error)
	Send(ctx context.Context, chatID string, event OutboundEvent) error
	Run(ctx context.Context) error
}

type InboundHandler interface {
	HandleIM2Inbound(ctx context.Context, event InboundEvent) error
}
```

Create `server/internal/im2/router.go`:

```go
package im2

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

type binding struct {
	sessionID string
	watch     bool
}

type Router struct {
	mu       sync.RWMutex
	client   InboundHandler
	history  SessionHistoryStore
	channels map[string]Channel
	bindings map[ChatRef]binding
}

func NewRouter(client InboundHandler, history SessionHistoryStore) *Router {
	if history == nil {
		history = NoopHistoryStore{}
	}
	return &Router{
		client:   client,
		history:  history,
		channels: map[string]Channel{},
		bindings: map[ChatRef]binding{},
	}
}

func (r *Router) RegisterChannel(ch Channel) error {
	if ch == nil {
		return fmt.Errorf("im2: channel is nil")
	}
	id := normalize(ch.ID())
	if id == "" {
		return fmt.Errorf("im2: channel id is empty")
	}
	r.mu.Lock()
	r.channels[id] = ch
	r.mu.Unlock()
	ch.OnMessage(func(ctx context.Context, chatID string, text string) error {
		return r.HandleInbound(ctx, InboundEvent{ChannelID: id, ChatID: chatID, Text: text})
	})
	return nil
}

func (r *Router) Bind(_ context.Context, chat ChatRef, sessionID string, opts BindOptions) error {
	chat = normalizeChat(chat)
	sessionID = strings.TrimSpace(sessionID)
	if chat.ChannelID == "" || chat.ChatID == "" || sessionID == "" {
		return fmt.Errorf("im2: invalid binding")
	}
	r.mu.Lock()
	r.bindings[chat] = binding{sessionID: sessionID, watch: opts.Watch}
	r.mu.Unlock()
	return nil
}

func (r *Router) Unbind(_ context.Context, chat ChatRef) error {
	chat = normalizeChat(chat)
	if chat.ChannelID == "" || chat.ChatID == "" {
		return fmt.Errorf("im2: invalid chat")
	}
	r.mu.Lock()
	delete(r.bindings, chat)
	r.mu.Unlock()
	return nil
}

func (r *Router) HandleInbound(ctx context.Context, event InboundEvent) error {
	chat := normalizeChat(ChatRef{ChannelID: event.ChannelID, ChatID: event.ChatID})
	text := strings.TrimSpace(event.Text)
	if chat.ChannelID == "" || chat.ChatID == "" {
		return fmt.Errorf("im2: inbound chat is invalid")
	}
	if text == "" {
		return nil
	}
	r.mu.RLock()
	b := r.bindings[chat]
	r.mu.RUnlock()
	event.ChannelID = chat.ChannelID
	event.ChatID = chat.ChatID
	event.Text = text
	event.SessionID = b.sessionID
	_ = r.history.Append(ctx, HistoryEvent{SessionID: event.SessionID, Direction: HistoryInbound, Source: &chat, Text: text})
	if r.client == nil {
		return nil
	}
	return r.client.HandleIM2Inbound(ctx, event)
}

func normalizeChat(chat ChatRef) ChatRef {
	return ChatRef{ChannelID: normalize(chat.ChannelID), ChatID: strings.TrimSpace(chat.ChatID)}
}

func normalize(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}
```

- [ ] **Step 4: Run router tests and verify GREEN**

Run: `cd server; go test ./internal/im2 -run "TestHandleInbound_UnboundChatReachesClientWithoutSession|TestBind_CausesLaterInboundToCarrySessionID" -v`

Expected: PASS.

- [ ] **Step 5: Commit**

Run:

```bash
git add server/internal/im2/protocol.go server/internal/im2/router.go server/internal/im2/router_test.go
git commit -m "feat(im2): add router binding core"
```

## Task 2: History Store

**Files:**
- Create: `server/internal/im2/history.go`
- Create: `server/internal/im2/history_test.go`
- Modify: `server/internal/im2/router.go`

- [ ] **Step 1: Write failing history tests**

Add `server/internal/im2/history_test.go`:

```go
package im2

import (
	"context"
	"testing"
)

func TestMemoryHistoryStore_ListFiltersBySession(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryHistoryStore()
	if err := store.Append(ctx, HistoryEvent{SessionID: "s1", Direction: HistoryInbound, Text: "a"}); err != nil {
		t.Fatalf("Append s1: %v", err)
	}
	if err := store.Append(ctx, HistoryEvent{SessionID: "s2", Direction: HistoryInbound, Text: "b"}); err != nil {
		t.Fatalf("Append s2: %v", err)
	}

	got, err := store.List(ctx, "s1", HistoryQuery{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 || got[0].Text != "a" {
		t.Fatalf("events=%+v", got)
	}
}
```

- [ ] **Step 2: Run the test and verify RED**

Run: `cd server; go test ./internal/im2 -run TestMemoryHistoryStore_ListFiltersBySession -v`

Expected: FAIL because `NewMemoryHistoryStore` is missing.

- [ ] **Step 3: Implement history store**

Create `server/internal/im2/history.go`:

```go
package im2

import (
	"context"
	"sync"
	"time"
)

const (
	HistoryInbound  = "inbound"
	HistoryOutbound = "outbound"
)

type HistoryEvent struct {
	SessionID string
	Direction string
	Source    *ChatRef
	Targets   []ChatRef
	Kind      OutboundKind
	Payload   any
	Text      string
	CreatedAt time.Time
}

type HistoryQuery struct {
	Limit int
}

type SessionHistoryStore interface {
	Append(ctx context.Context, event HistoryEvent) error
	List(ctx context.Context, sessionID string, query HistoryQuery) ([]HistoryEvent, error)
}

type NoopHistoryStore struct{}

func (NoopHistoryStore) Append(context.Context, HistoryEvent) error { return nil }
func (NoopHistoryStore) List(context.Context, string, HistoryQuery) ([]HistoryEvent, error) {
	return nil, nil
}

type MemoryHistoryStore struct {
	mu     sync.Mutex
	events []HistoryEvent
}

func NewMemoryHistoryStore() *MemoryHistoryStore {
	return &MemoryHistoryStore{}
}

func (s *MemoryHistoryStore) Append(_ context.Context, event HistoryEvent) error {
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now()
	}
	s.mu.Lock()
	s.events = append(s.events, event)
	s.mu.Unlock()
	return nil
}

func (s *MemoryHistoryStore) List(_ context.Context, sessionID string, query HistoryQuery) ([]HistoryEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []HistoryEvent
	for _, event := range s.events {
		if event.SessionID == sessionID {
			out = append(out, event)
		}
	}
	if query.Limit > 0 && len(out) > query.Limit {
		out = out[len(out)-query.Limit:]
	}
	return append([]HistoryEvent(nil), out...), nil
}
```

- [ ] **Step 4: Run history tests and verify GREEN**

Run: `cd server; go test ./internal/im2 -run TestMemoryHistoryStore_ListFiltersBySession -v`

Expected: PASS.

- [ ] **Step 5: Commit**

Run:

```bash
git add server/internal/im2/history.go server/internal/im2/history_test.go server/internal/im2/router.go
git commit -m "feat(im2): add session history store"
```

## Task 3: Unified Send Fanout

**Files:**
- Modify: `server/internal/im2/router.go`
- Modify: `server/internal/im2/router_test.go`

- [ ] **Step 1: Write failing send tests**

Append these tests to `server/internal/im2/router_test.go`:

```go
func TestSend_DirectChatSendsOnlyTarget(t *testing.T) {
	ctx := context.Background()
	router := NewRouter(nil, nil)
	ch := &captureChannel{id: "feishu"}
	_ = router.RegisterChannel(ch)

	err := router.Send(ctx, SendTarget{ChannelID: "feishu", ChatID: "chat-a"}, OutboundEvent{Kind: OutboundSystem, Payload: "choose a session"})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if len(ch.sent) != 1 || ch.sent[0].chatID != "chat-a" {
		t.Fatalf("sent=%+v", ch.sent)
	}
}

func TestSend_ReplyFansOutToWatchChatsOnly(t *testing.T) {
	ctx := context.Background()
	router := NewRouter(nil, nil)
	ch := &captureChannel{id: "app"}
	_ = router.RegisterChannel(ch)
	_ = router.Bind(ctx, ChatRef{ChannelID: "app", ChatID: "a"}, "s1", BindOptions{})
	_ = router.Bind(ctx, ChatRef{ChannelID: "app", ChatID: "b"}, "s1", BindOptions{Watch: true})
	_ = router.Bind(ctx, ChatRef{ChannelID: "app", ChatID: "c"}, "s1", BindOptions{})
	source := ChatRef{ChannelID: "app", ChatID: "a"}

	err := router.Send(ctx, SendTarget{SessionID: "s1", Source: &source}, OutboundEvent{Kind: OutboundMessage, Payload: "hello"})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	if len(ch.sent) != 2 {
		t.Fatalf("sent count=%d, want 2: %+v", len(ch.sent), ch.sent)
	}
	if ch.sent[0].chatID != "a" || ch.sent[1].chatID != "b" {
		t.Fatalf("sent=%+v", ch.sent)
	}
}

func TestSend_SessionBroadcastSendsAllBoundChats(t *testing.T) {
	ctx := context.Background()
	router := NewRouter(nil, nil)
	ch := &captureChannel{id: "app"}
	_ = router.RegisterChannel(ch)
	_ = router.Bind(ctx, ChatRef{ChannelID: "app", ChatID: "a"}, "s1", BindOptions{})
	_ = router.Bind(ctx, ChatRef{ChannelID: "app", ChatID: "b"}, "s1", BindOptions{Watch: true})

	err := router.Send(ctx, SendTarget{SessionID: "s1"}, OutboundEvent{Kind: OutboundSystem, Payload: "broadcast"})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if len(ch.sent) != 2 {
		t.Fatalf("sent count=%d, want 2: %+v", len(ch.sent), ch.sent)
	}
}
```

- [ ] **Step 2: Run the tests and verify RED**

Run: `cd server; go test ./internal/im2 -run "TestSend_" -v`

Expected: FAIL because `Router.Send` is missing.

- [ ] **Step 3: Implement send fanout**

Add to `server/internal/im2/router.go`:

```go
func (r *Router) Send(ctx context.Context, target SendTarget, event OutboundEvent) error {
	recipients, err := r.recipients(target)
	if err != nil {
		return err
	}
	var firstErr error
	for _, chat := range recipients {
		ch, err := r.channel(chat.ChannelID)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if err := ch.Send(ctx, chat.ChatID, event); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	_ = r.history.Append(ctx, HistoryEvent{SessionID: strings.TrimSpace(target.SessionID), Direction: HistoryOutbound, Source: target.Source, Targets: recipients, Kind: event.Kind, Payload: event.Payload})
	return firstErr
}

func (r *Router) recipients(target SendTarget) ([]ChatRef, error) {
	sessionID := strings.TrimSpace(target.SessionID)
	if sessionID == "" {
		chat := normalizeChat(ChatRef{ChannelID: target.ChannelID, ChatID: target.ChatID})
		if chat.ChannelID == "" || chat.ChatID == "" {
			return nil, fmt.Errorf("im2: direct send target is invalid")
		}
		return []ChatRef{chat}, nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if target.Source != nil {
		source := normalizeChat(*target.Source)
		if source.ChannelID == "" || source.ChatID == "" {
			return nil, fmt.Errorf("im2: reply source is invalid")
		}
		out := []ChatRef{source}
		for chat, b := range r.bindings {
			if b.sessionID == sessionID && b.watch && chat != source {
				out = append(out, chat)
			}
		}
		return out, nil
	}
	var out []ChatRef
	for chat, b := range r.bindings {
		if b.sessionID == sessionID {
			out = append(out, chat)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("im2: no chats bound to session %q", sessionID)
	}
	return out, nil
}

func (r *Router) channel(channelID string) (Channel, error) {
	id := normalize(channelID)
	r.mu.RLock()
	ch := r.channels[id]
	r.mu.RUnlock()
	if ch == nil {
		return nil, fmt.Errorf("im2: channel %q is not registered", id)
	}
	return ch, nil
}
```

- [ ] **Step 4: Run send tests and verify GREEN**

Run: `cd server; go test ./internal/im2 -run "TestSend_" -v`

Expected: PASS.

- [ ] **Step 5: Commit**

Run:

```bash
git add server/internal/im2/router.go server/internal/im2/router_test.go
git commit -m "feat(im2): add unified send fanout"
```

## Task 4: App Channel Stub

**Files:**
- Create: `server/internal/im2/app/app.go`
- Create: `server/internal/im2/app/app_test.go`

- [ ] **Step 1: Write failing App tests**

Add `server/internal/im2/app/app_test.go`:

```go
package app

import (
	"testing"

	"github.com/swm8023/wheelmaker/internal/im2"
)

func TestChannelImplementsIM2Channel(t *testing.T) {
	var _ im2.Channel = New()
}

func TestChannelIDIsApp(t *testing.T) {
	if got := New().ID(); got != "app" {
		t.Fatalf("ID=%q, want app", got)
	}
}
```

- [ ] **Step 2: Run tests and verify RED**

Run: `cd server; go test ./internal/im2/app -v`

Expected: FAIL because package `internal/im2/app` does not exist.

- [ ] **Step 3: Implement App stub**

Create `server/internal/im2/app/app.go`:

```go
package app

import (
	"context"
	"errors"

	"github.com/swm8023/wheelmaker/internal/im2"
)

var ErrNotImplemented = errors.New("im2 app channel: not implemented")

type Channel struct {
	handler func(context.Context, string, string) error
}

func New() *Channel {
	return &Channel{}
}

func (c *Channel) ID() string { return "app" }

func (c *Channel) OnMessage(handler func(context.Context, string, string) error) {
	c.handler = handler
}

func (c *Channel) Send(context.Context, string, im2.OutboundEvent) error {
	return ErrNotImplemented
}

func (c *Channel) Run(context.Context) error {
	return ErrNotImplemented
}
```

- [ ] **Step 4: Run App tests and verify GREEN**

Run: `cd server; go test ./internal/im2/app -v`

Expected: PASS.

- [ ] **Step 5: Commit**

Run:

```bash
git add server/internal/im2/app/app.go server/internal/im2/app/app_test.go
git commit -m "feat(im2): add app channel stub"
```

## Task 5: Feishu Channel Adapter

**Files:**
- Create: `server/internal/im2/feishu/feishu.go`
- Create: `server/internal/im2/feishu/feishu_test.go`

- [ ] **Step 1: Write failing Feishu tests**

Add `server/internal/im2/feishu/feishu_test.go`:

```go
package feishu

import (
	"testing"

	"github.com/swm8023/wheelmaker/internal/im2"
)

func TestChannelImplementsIM2Channel(t *testing.T) {
	var _ im2.Channel = New(Config{})
}

func TestChannelIDIsFeishu(t *testing.T) {
	if got := New(Config{}).ID(); got != "feishu" {
		t.Fatalf("ID=%q, want feishu", got)
	}
}
```

- [ ] **Step 2: Run tests and verify RED**

Run: `cd server; go test ./internal/im2/feishu -v`

Expected: FAIL because package `internal/im2/feishu` does not exist.

- [ ] **Step 3: Implement Feishu adapter through composition**

Create `server/internal/im2/feishu/feishu.go`:

```go
package feishu

import (
	"context"
	"fmt"

	hubfeishu "github.com/swm8023/wheelmaker/internal/hub/im/feishu"
	hubim "github.com/swm8023/wheelmaker/internal/hub/im"
	"github.com/swm8023/wheelmaker/internal/im2"
)

type Config = hubfeishu.Config

type Channel struct {
	inner *hubfeishu.Channel
}

func New(cfg Config) *Channel {
	return &Channel{inner: hubfeishu.New(cfg)}
}

func (c *Channel) ID() string { return "feishu" }

func (c *Channel) OnMessage(handler func(context.Context, string, string) error) {
	c.inner.OnMessage(func(msg hubim.Message) {
		if handler != nil {
			_ = handler(context.Background(), msg.ChatID, msg.Text)
		}
	})
}

func (c *Channel) Send(ctx context.Context, chatID string, event im2.OutboundEvent) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	text, ok := event.Payload.(string)
	if !ok {
		text = fmt.Sprint(event.Payload)
	}
	switch event.Kind {
	case im2.OutboundACP:
		return c.inner.Send(chatID, text, hubim.TextDebug)
	case im2.OutboundSystem:
		return c.inner.Send(chatID, text, hubim.TextSystem)
	default:
		return c.inner.Send(chatID, text, hubim.TextNormal)
	}
}

func (c *Channel) Run(ctx context.Context) error {
	return c.inner.Run(ctx)
}
```

- [ ] **Step 4: Run Feishu tests and verify GREEN**

Run: `cd server; go test ./internal/im2/feishu -v`

Expected: PASS.

- [ ] **Step 5: Commit**

Run:

```bash
git add server/internal/im2/feishu/feishu.go server/internal/im2/feishu/feishu_test.go
git commit -m "feat(im2): add feishu channel adapter"
```

## Task 6: Full Verification And Isolation

**Files:**
- Modify: `docs/superpowers/plans/2026-04-07-im2-router-multisession-implementation.md`

- [ ] **Step 1: Run focused IM2 tests**

Run: `cd server; go test ./internal/im2/... -v`

Expected: PASS.

- [ ] **Step 2: Run server full test suite**

Run: `cd server; go test ./...`

Expected: PASS.

- [ ] **Step 3: Verify Hub default path does not import IM2**

Run: `rg -n "internal/im2|im2" server/internal/hub server/cmd`

Expected: no production Hub/CMD wiring references to `internal/im2`. Test package references under `server/internal/im2` are fine because the command scope excludes that directory.

- [ ] **Step 4: Commit final plan file if not already committed**

Run:

```bash
git add docs/superpowers/plans/2026-04-07-im2-router-multisession-implementation.md
git commit -m "docs(im2): add router multisession implementation plan"
```

If the plan was already committed earlier, this step may report no changes; do not create an empty commit.
