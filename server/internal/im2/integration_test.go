package im2

import (
	"context"
	"testing"
)

func TestIM2_E2E_InboundCarriesRouteKeyToClient(t *testing.T) {
	st := newFakeState()
	var got InboundEvent

	r, err := NewRouter("proj", st, func(context.Context) (string, error) { return "s-1", nil }, func(_ context.Context, ev InboundEvent) error {
		got = ev
		return nil
	})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	if err := r.HandleInbound(context.Background(), InboundEvent{Kind: InboundPrompt, IMType: "feishu", ChatID: "chat-a", Text: "hello"}); err != nil {
		t.Fatalf("HandleInbound: %v", err)
	}
	if got.RouteKey != "feishu:chat-a" || got.ClientSessionID != "s-1" {
		t.Fatalf("got routeKey=%q clientSessionID=%q", got.RouteKey, got.ClientSessionID)
	}
}

func TestIM2_E2E_NormalReplyUsesSameRouteKey(t *testing.T) {
	st := newFakeState()
	_ = st.BindRouteKey(context.Background(), "feishu:chat-a", "feishu", "chat-a", "s-1", true)

	ch := &fakeChannel{}
	r, err := NewRouter("proj", st, func(context.Context) (string, error) { return "unused", nil }, nil)
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	if err := r.RegisterChannel("feishu", ch); err != nil {
		t.Fatalf("RegisterChannel: %v", err)
	}
	if err := r.Publish(context.Background(), OutboundEvent{
		Kind:            OutboundMessage,
		ClientSessionID: "s-1",
		TargetRouteKey:  "feishu:chat-a",
		Text:            "reply",
	}); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if ch.count() != 1 {
		t.Fatalf("channel count=%d, want 1", ch.count())
	}
}

func TestIM2_E2E_NewRebindOnlyCurrentRouteKey(t *testing.T) {
	st := newFakeState()
	_ = st.BindRouteKey(context.Background(), "feishu:chat-a", "feishu", "chat-a", "s-old", true)
	_ = st.BindRouteKey(context.Background(), "feishu:chat-b", "feishu", "chat-b", "s-old", true)

	r, err := NewRouter("proj", st, func(context.Context) (string, error) { return "s-new", nil }, nil)
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
