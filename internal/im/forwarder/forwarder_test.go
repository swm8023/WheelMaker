package forwarder

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/swm8023/wheelmaker/internal/im"
)

type stubAdapter struct {
	onMsg      im.MessageHandler
	onAction   func(im.CardActionEvent)
	lastChatID string
	lastText   string
	cardSent   bool
	runCalled  bool
}

func (s *stubAdapter) OnMessage(h im.MessageHandler) { s.onMsg = h }
func (s *stubAdapter) SendText(chatID, text string) error {
	s.lastChatID = chatID
	s.lastText = text
	return nil
}
func (s *stubAdapter) SendCard(_ string, _ im.Card) error { return nil }
func (s *stubAdapter) SendReaction(_, _ string) error     { return nil }
func (s *stubAdapter) Run(_ context.Context) error {
	s.runCalled = true
	return nil
}
func (s *stubAdapter) OnCardAction(h func(im.CardActionEvent)) { s.onAction = h }

func TestForwarder_SendTextPassThrough(t *testing.T) {
	ad := &stubAdapter{}
	f := New(ad)
	if err := f.SendText("chat-1", "hello"); err != nil {
		t.Fatalf("SendText error: %v", err)
	}
	if ad.lastChatID != "chat-1" || ad.lastText != "hello" {
		t.Fatalf("adapter got (%q,%q), want (%q,%q)", ad.lastChatID, ad.lastText, "chat-1", "hello")
	}
}

func TestForwarder_OnMessageBridge(t *testing.T) {
	ad := &stubAdapter{}
	f := New(ad)

	got := ""
	f.OnMessage(func(m im.Message) { got = m.Text })
	ad.onMsg(im.Message{Text: "ping"})

	if got != "ping" {
		t.Fatalf("bridged text %q, want %q", got, "ping")
	}
}

func TestForwarder_RequestDecision_TextReply(t *testing.T) {
	ad := &stubAdapter{}
	f := New(ad)
	f.OnMessage(func(_ im.Message) {})

	done := make(chan im.DecisionResult, 1)
	go func() {
		res, _ := f.RequestDecision(context.Background(), im.DecisionRequest{
			Kind:   im.DecisionSingle,
			ChatID: "chat-1",
			Title:  "pick one",
			Options: []im.DecisionOption{
				{ID: "allow", Label: "Allow"},
				{ID: "reject", Label: "Reject"},
			},
		})
		done <- res
	}()

	time.Sleep(20 * time.Millisecond)
	ad.onMsg(im.Message{ChatID: "chat-1", Text: "1"})

	res := <-done
	if res.Outcome != "selected" || res.OptionID != "allow" {
		t.Fatalf("decision result = %#v, want selected allow", res)
	}
}

func TestForwarder_EmitFlushOnDone(t *testing.T) {
	ad := &stubAdapter{}
	f := New(ad)
	if err := f.Emit(context.Background(), im.IMUpdate{ChatID: "chat-1", UpdateType: "text", Text: "hel"}); err != nil {
		t.Fatalf("emit text: %v", err)
	}
	if err := f.Emit(context.Background(), im.IMUpdate{ChatID: "chat-1", UpdateType: "text", Text: "lo"}); err != nil {
		t.Fatalf("emit text: %v", err)
	}
	if err := f.Emit(context.Background(), im.IMUpdate{ChatID: "chat-1", UpdateType: "done"}); err != nil {
		t.Fatalf("emit done: %v", err)
	}
	if ad.lastText != "hello" {
		t.Fatalf("flushed text %q, want %q", ad.lastText, "hello")
	}
}

func TestForwarder_HelpCardActionInjectsCommand(t *testing.T) {
	ad := &stubAdapter{}
	f := New(ad)

	got := ""
	f.SetHelpResolver(func(_ context.Context, _ string) (im.HelpModel, error) {
		return im.HelpModel{
			Title: "help",
			Body:  "select",
			Options: []im.HelpOption{
				{Label: "Mode Plan", Command: "/mode", Value: "plan"},
			},
		}, nil
	})
	f.OnMessage(func(m im.Message) { got = m.Text })

	ad.onMsg(im.Message{ChatID: "chat-1", Text: "/help"})
	if got != "" {
		t.Fatalf("help should be intercepted")
	}

	if ad.onAction == nil {
		t.Fatalf("expected card action handler to be registered")
	}
	ad.onAction(im.CardActionEvent{
		Value: map[string]string{
			"kind":    "help_option",
			"chat_id": "chat-1",
			"command": "/mode",
			"value":   "plan",
		},
	})
	if got != "/mode plan" {
		t.Fatalf("injected command = %q, want %q", got, "/mode plan")
	}
}

func TestRenderToolCallUpdate(t *testing.T) {
	raw := []byte(`{"sessionUpdate":"tool_call_update","toolCallId":"call_1","title":"Run tests","status":"in_progress"}`)
	got := renderToolCallUpdate(raw)
	if got == "" || !containsAll(got, "Run tests", "in_progress", "call_1") {
		t.Fatalf("renderToolCallUpdate()=%q", got)
	}
}

func TestRenderConfigOptionUpdate(t *testing.T) {
	raw := []byte(`{"configOptions":[{"id":"mode","currentValue":"plan"},{"id":"model","currentValue":"gpt-5"}]}`)
	got := renderConfigOptionUpdate(raw)
	if !containsAll(got, "mode=plan", "model=gpt-5") {
		t.Fatalf("renderConfigOptionUpdate()=%q", got)
	}
}

func containsAll(s string, terms ...string) bool {
	for _, t := range terms {
		if !strings.Contains(s, t) {
			return false
		}
	}
	return true
}
