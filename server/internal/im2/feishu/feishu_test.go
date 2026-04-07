package feishu

import (
	"context"
	"encoding/json"
	"testing"
	"time"

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

type fakeTransport struct {
	onMsg    MessageHandler
	onAction func(CardActionEvent)
	sends    []fakeSend
	cards    []fakeCard
	done     []string
}

type fakeSend struct {
	chatID string
	text   string
	kind   TextKind
}

type fakeCard struct {
	chatID string
	card   Card
}

func (f *fakeTransport) OnMessage(h MessageHandler) {
	f.onMsg = h
}

func (f *fakeTransport) OnCardAction(h func(CardActionEvent)) {
	f.onAction = h
}

func (f *fakeTransport) Send(chatID, text string, kind TextKind) error {
	f.sends = append(f.sends, fakeSend{chatID: chatID, text: text, kind: kind})
	return nil
}

func (f *fakeTransport) SendCard(chatID, _ string, card Card) error {
	f.cards = append(f.cards, fakeCard{chatID: chatID, card: card})
	return nil
}

func (f *fakeTransport) SendReaction(_, _ string) error { return nil }
func (f *fakeTransport) MarkDone(chatID string) error {
	f.done = append(f.done, chatID)
	return nil
}
func (f *fakeTransport) Run(context.Context) error { return nil }

func TestSend_ACPPayloadRendersByUpdateType(t *testing.T) {
	ft := &fakeTransport{}
	ch := newWithTransport(ft)
	rawTool, _ := json.Marshal(ToolCallUpdate{SessionUpdate: "tool_call_update", ToolCallID: "tc-1", Title: "Read", Status: "completed"})

	tests := []struct {
		name string
		ev   im2.OutboundEvent
	}{
		{name: "text", ev: im2.OutboundEvent{Kind: im2.OutboundACP, Payload: im2.ACPPayload{UpdateType: "text", Text: "hello"}}},
		{name: "thought", ev: im2.OutboundEvent{Kind: im2.OutboundACP, Payload: im2.ACPPayload{UpdateType: "thought", Text: "thinking"}}},
		{name: "tool", ev: im2.OutboundEvent{Kind: im2.OutboundACP, Payload: im2.ACPPayload{UpdateType: "tool_call_update", Raw: rawTool}}},
		{name: "done", ev: im2.OutboundEvent{Kind: im2.OutboundACP, Payload: im2.ACPPayload{UpdateType: "done"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ch.Send(context.Background(), "chat-a", tt.ev); err != nil {
				t.Fatalf("Send: %v", err)
			}
		})
	}

	if len(ft.sends) != 2 {
		t.Fatalf("sends=%+v, want text and thought", ft.sends)
	}
	if ft.sends[0].kind != TextNormal || ft.sends[1].kind != TextThought {
		t.Fatalf("sends=%+v", ft.sends)
	}
	if len(ft.cards) != 1 {
		t.Fatalf("cards=%+v, want one tool card", ft.cards)
	}
	if _, ok := ft.cards[0].card.(ToolCallCard); !ok {
		t.Fatalf("card type=%T, want ToolCallCard", ft.cards[0].card)
	}
	if len(ft.done) != 1 || ft.done[0] != "chat-a" {
		t.Fatalf("done=%+v", ft.done)
	}
}

func TestRequestDecision_CardActionResolvesOnce(t *testing.T) {
	ft := &fakeTransport{}
	ch := newWithTransport(ft)
	done := make(chan im2.DecisionResult, 1)
	go func() {
		res, _ := ch.RequestDecision(context.Background(), "chat-a", im2.DecisionRequest{
			Kind:    im2.DecisionPermission,
			Title:   "Allow?",
			Options: []im2.DecisionOption{{ID: "allow", Label: "Allow", Value: "allow_once"}},
		})
		done <- res
	}()

	waitForCard(t, ft)
	card := ft.cards[0].card.(OptionsCard)
	decisionID := card.Meta["decision_id"]
	ft.onAction(CardActionEvent{ChatID: "chat-a", Value: map[string]string{
		"kind": "decision", "decision_id": decisionID, "option_id": "allow", "value": "allow_once",
	}})
	ft.onAction(CardActionEvent{ChatID: "chat-a", Value: map[string]string{
		"kind": "decision", "decision_id": decisionID, "option_id": "deny", "value": "deny",
	}})

	select {
	case res := <-done:
		if res.OptionID != "allow" || res.Value != "allow_once" || res.Source != "card_action" {
			t.Fatalf("result=%+v", res)
		}
	case <-time.After(time.Second):
		t.Fatal("decision did not resolve")
	}
}

func TestRequestDecision_TextFallbackResolves(t *testing.T) {
	ft := &fakeTransport{}
	ch := newWithTransport(ft)
	done := make(chan im2.DecisionResult, 1)
	go func() {
		res, _ := ch.RequestDecision(context.Background(), "chat-a", im2.DecisionRequest{
			Kind:    im2.DecisionPermission,
			Title:   "Allow?",
			Options: []im2.DecisionOption{{ID: "allow", Label: "Allow", Value: "allow_once"}},
		})
		done <- res
	}()

	waitForCard(t, ft)
	ft.onMsg(Message{ChatID: "chat-a", Text: "1"})
	select {
	case res := <-done:
		if res.OptionID != "allow" || res.Source != "text_reply" {
			t.Fatalf("result=%+v", res)
		}
	case <-time.After(time.Second):
		t.Fatal("decision did not resolve")
	}
}

func waitForCard(t *testing.T, ft *fakeTransport) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if len(ft.cards) > 0 {
			if _, ok := ft.cards[0].card.(OptionsCard); !ok {
				t.Fatalf("card type=%T, want OptionsCard", ft.cards[0].card)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timed out waiting for card")
}
