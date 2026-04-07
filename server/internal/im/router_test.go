package im

import (
	"context"
	"testing"
)

type captureInboundClient struct {
	events []InboundEvent
}

func (c *captureInboundClient) HandleIMInbound(_ context.Context, event InboundEvent) error {
	c.events = append(c.events, event)
	return nil
}

type captureChannel struct {
	id        string
	sent      []sentEvent
	decisions []decisionEvent
	runCalled bool
	onMsg     func(context.Context, string, string) error
}

type sentEvent struct {
	chatID string
	event  OutboundEvent
}

type decisionEvent struct {
	chatID string
	req    DecisionRequest
}

func (c *captureChannel) ID() string { return c.id }
func (c *captureChannel) OnMessage(fn func(context.Context, string, string) error) {
	c.onMsg = fn
}
func (c *captureChannel) Send(_ context.Context, chatID string, event OutboundEvent) error {
	c.sent = append(c.sent, sentEvent{chatID: chatID, event: event})
	return nil
}
func (c *captureChannel) RequestDecision(_ context.Context, chatID string, req DecisionRequest) (DecisionResult, error) {
	c.decisions = append(c.decisions, decisionEvent{chatID: chatID, req: req})
	return DecisionResult{Outcome: "selected", OptionID: "allow", Value: "allow_once", Source: "card_action"}, nil
}
func (c *captureChannel) Run(context.Context) error {
	c.runCalled = true
	return nil
}

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

func TestHandleInbound_PreservesMessageText(t *testing.T) {
	ctx := context.Background()
	client := &captureInboundClient{}
	router := NewRouter(client, nil)

	err := router.HandleInbound(ctx, InboundEvent{ChannelID: "feishu", ChatID: "chat-a", Text: "  hello\n"})
	if err != nil {
		t.Fatalf("HandleInbound: %v", err)
	}
	if got := client.events[0].Text; got != "  hello\n" {
		t.Fatalf("Text=%q, want preserved whitespace", got)
	}
}

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

func TestRequestDecision_RoutesOnlyToSourceChat(t *testing.T) {
	ctx := context.Background()
	router := NewRouter(nil, nil)
	ch := &captureChannel{id: "feishu"}
	_ = router.RegisterChannel(ch)
	source := ChatRef{ChannelID: "feishu", ChatID: "chat-a"}

	res, err := router.RequestDecision(ctx, SendTarget{SessionID: "s1", Source: &source}, DecisionRequest{
		SessionID: "s1",
		Kind:      DecisionPermission,
		Title:     "Allow?",
		Options:   []DecisionOption{{ID: "allow", Label: "Allow", Value: "allow_once"}},
	})
	if err != nil {
		t.Fatalf("RequestDecision: %v", err)
	}
	if res.OptionID != "allow" {
		t.Fatalf("result=%+v", res)
	}
	if len(ch.decisions) != 1 || ch.decisions[0].chatID != "chat-a" {
		t.Fatalf("decisions=%+v", ch.decisions)
	}
}

func TestRun_StartsRegisteredChannels(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	router := NewRouter(nil, nil)
	ch := &captureChannel{id: "feishu"}
	_ = router.RegisterChannel(ch)

	if err := router.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !ch.runCalled {
		t.Fatal("channel Run was not called")
	}
}
