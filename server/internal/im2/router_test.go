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
