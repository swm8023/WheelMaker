package im2

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type fakeState struct {
	mu sync.Mutex

	bindings map[string]IMRouteBinding
}

func newFakeState() *fakeState {
	return &fakeState{bindings: map[string]IMRouteBinding{}}
}

func (s *fakeState) Load(context.Context) error { return nil }

func (s *fakeState) ResolveClientSessionID(_ context.Context, routeKey string) (string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	binding, ok := s.bindings[routeKey]
	if !ok {
		return "", false, nil
	}
	return binding.ClientSessionID, true, nil
}

func (s *fakeState) BindRouteKey(_ context.Context, routeKey, imType, chatID, clientSessionID string, _ bool) error {
	s.mu.Lock()
	s.bindings[routeKey] = IMRouteBinding{
		RouteKey:        routeKey,
		IMType:          imType,
		ChatID:          chatID,
		ClientSessionID: clientSessionID,
		Online:          true,
		UpdatedAt:       time.Now(),
	}
	s.mu.Unlock()
	return nil
}

func (s *fakeState) RebindRouteKey(ctx context.Context, routeKey, clientSessionID string) error {
	t, c, ok := ParseRouteKey(routeKey)
	if !ok {
		return errors.New("invalid route key")
	}
	return s.BindRouteKey(ctx, routeKey, t, c, clientSessionID, true)
}

func (s *fakeState) SetRouteKeyOnline(_ context.Context, routeKey string, online bool, _ bool) error {
	s.mu.Lock()
	binding := s.bindings[routeKey]
	binding.Online = online
	binding.UpdatedAt = time.Now()
	s.bindings[routeKey] = binding
	s.mu.Unlock()
	return nil
}

func (s *fakeState) ListSessionRouteBindings(_ context.Context, clientSessionID string, onlineOnly bool) ([]IMRouteBinding, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]IMRouteBinding, 0, 4)
	for _, binding := range s.bindings {
		if binding.ClientSessionID != clientSessionID {
			continue
		}
		if onlineOnly && !binding.Online {
			continue
		}
		out = append(out, binding)
	}
	return out, nil
}

func (s *fakeState) Close() error { return nil }

var _ State = (*fakeState)(nil)

type capturedPublish struct {
	chatID string
	event  OutboundEvent
}

type fakeChannel struct {
	mu      sync.Mutex
	records []capturedPublish
	err     error
}

func (ch *fakeChannel) PublishToChat(_ context.Context, chatID string, event OutboundEvent) error {
	if ch.err != nil {
		return ch.err
	}
	ch.mu.Lock()
	ch.records = append(ch.records, capturedPublish{chatID: chatID, event: event})
	ch.mu.Unlock()
	return nil
}

func (ch *fakeChannel) count() int {
	ch.mu.Lock()
	defer ch.mu.Unlock()
	return len(ch.records)
}

func TestBuildRouteKey_UsesIMTypeAndChatID(t *testing.T) {
	got, err := BuildRouteKey("feishu", "oc_123")
	if err != nil {
		t.Fatalf("BuildRouteKey: %v", err)
	}
	if got != "feishu:oc_123" {
		t.Fatalf("routeKey=%q, want feishu:oc_123", got)
	}
}

func TestRouterHandleInboundCreatesBinding(t *testing.T) {
	state := newFakeState()
	var got InboundEvent
	r, err := NewRouter("proj", state, func(context.Context) (string, error) {
		return "session-1", nil
	}, func(_ context.Context, event InboundEvent) error {
		got = event
		return nil
	})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	if err := r.HandleInbound(context.Background(), InboundEvent{
		Kind:   InboundPrompt,
		IMType: "feishu",
		ChatID: "chat-1",
		Text:   "hello",
	}); err != nil {
		t.Fatalf("HandleInbound: %v", err)
	}

	if got.ClientSessionID != "session-1" {
		t.Fatalf("clientSessionId=%q, want session-1", got.ClientSessionID)
	}
	if got.RouteKey != "feishu:chat-1" {
		t.Fatalf("routeKey=%q, want feishu:chat-1", got.RouteKey)
	}
}

func TestRouterPublishBroadcastsToOnlineChats(t *testing.T) {
	state := newFakeState()
	_ = state.BindRouteKey(context.Background(), "feishu:chat-1", "feishu", "chat-1", "session-1", true)
	_ = state.BindRouteKey(context.Background(), "feishu:chat-2", "feishu", "chat-2", "session-1", true)

	ch := &fakeChannel{}
	r, err := NewRouter("proj", state, func(context.Context) (string, error) {
		return "", errors.New("not used")
	}, nil)
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	if err := r.RegisterChannel("feishu", ch); err != nil {
		t.Fatalf("RegisterChannel: %v", err)
	}

	if err := r.Publish(context.Background(), OutboundEvent{
		Kind:            OutboundMessage,
		ClientSessionID: "session-1",
		Text:            "hello all",
	}); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	if ch.count() != 2 {
		t.Fatalf("publish count=%d, want 2", ch.count())
	}
}

func TestRouter_RegisterChannelAndTargetReply(t *testing.T) {
	state := newFakeState()
	_ = state.BindRouteKey(context.Background(), "feishu:oc_123", "feishu", "oc_123", "sess-1", true)

	ch := &fakeChannel{}
	r, err := NewRouter("proj", state, func(context.Context) (string, error) {
		return "", errors.New("not used")
	}, nil)
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	if err := r.RegisterChannel("feishu", ch); err != nil {
		t.Fatalf("RegisterChannel: %v", err)
	}

	if err := r.Publish(context.Background(), OutboundEvent{
		Kind:            OutboundCommandReply,
		ClientSessionID: "sess-1",
		TargetRouteKey:  "feishu:oc_123",
		Text:            "only one",
	}); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	if ch.count() != 1 {
		t.Fatalf("publish count=%d, want 1", ch.count())
	}
	if ch.records[0].chatID != "oc_123" {
		t.Fatalf("target chat=%q, want oc_123", ch.records[0].chatID)
	}
}

func TestRouter_RebindRouteKey_OnlyAffectsTargetRouteKey(t *testing.T) {
	st := newFakeState()
	_ = st.BindRouteKey(context.Background(), "feishu:chat-a", "feishu", "chat-a", "s-old", true)
	_ = st.BindRouteKey(context.Background(), "feishu:chat-b", "feishu", "chat-b", "s-old", true)

	r, err := NewRouter("proj", st, func(context.Context) (string, error) { return "unused", nil }, nil)
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	if err := r.RebindRouteKey(context.Background(), "feishu:chat-a", "s-new"); err != nil {
		t.Fatalf("RebindRouteKey: %v", err)
	}

	a, _, _ := st.ResolveClientSessionID(context.Background(), "feishu:chat-a")
	b, _, _ := st.ResolveClientSessionID(context.Background(), "feishu:chat-b")
	if a != "s-new" || b != "s-old" {
		t.Fatalf("got a=%q b=%q", a, b)
	}
}
