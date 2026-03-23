package im

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
)

type stubAdapter struct {
	onMsg      MessageHandler
	onAction   func(CardActionEvent)
	lastChatID string
	lastText   string
	textCount  int
	cardSent   bool
	runCalled  bool
}

type toolCardStub struct {
	stubAdapter
	toolCalls []ToolCallUpdate
}

func (s *toolCardStub) SendToolCall(_ string, update ToolCallUpdate) error {
	s.toolCalls = append(s.toolCalls, update)
	return nil
}

func (s *stubAdapter) OnMessage(h MessageHandler) { s.onMsg = h }
func (s *stubAdapter) SendText(chatID, text string) error {
	s.lastChatID = chatID
	s.lastText = text
	s.textCount++
	return nil
}
func (s *stubAdapter) SendCard(_ string, _ Card) error { return nil }
func (s *stubAdapter) SendReaction(_, _ string) error  { return nil }
func (s *stubAdapter) Run(_ context.Context) error {
	s.runCalled = true
	return nil
}
func (s *stubAdapter) OnCardAction(h func(CardActionEvent)) { s.onAction = h }

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
	f.OnMessage(func(m Message) { got = m.Text })
	ad.onMsg(Message{Text: "ping"})

	if got != "ping" {
		t.Fatalf("bridged text %q, want %q", got, "ping")
	}
}

func TestForwarder_RequestDecision_TextReply(t *testing.T) {
	ad := &stubAdapter{}
	f := New(ad)
	f.OnMessage(func(_ Message) {})

	done := make(chan DecisionResult, 1)
	go func() {
		res, _ := f.RequestDecision(context.Background(), DecisionRequest{
			Kind:   DecisionSingle,
			ChatID: "chat-1",
			Title:  "pick one",
			Options: []DecisionOption{
				{ID: "allow", Label: "Allow"},
				{ID: "reject", Label: "Reject"},
			},
		})
		done <- res
	}()

	time.Sleep(20 * time.Millisecond)
	ad.onMsg(Message{ChatID: "chat-1", Text: "1"})

	res := <-done
	if res.Outcome != "selected" || res.OptionID != "allow" {
		t.Fatalf("decision result = %#v, want selected allow", res)
	}
}

func TestForwarder_RequestDecision_DuplicateCardActionIsIgnored(t *testing.T) {
	ad := &stubAdapter{}
	f := New(ad)
	var buf bytes.Buffer
	f.SetDebugLogger(&buf)

	done := make(chan DecisionResult, 1)
	go func() {
		res, _ := f.RequestDecision(context.Background(), DecisionRequest{
			Kind:   DecisionPermission,
			ChatID: "chat-1",
			Title:  "Permission request",
			Options: []DecisionOption{
				{ID: "allow", Label: "Allow"},
				{ID: "reject", Label: "Reject"},
			},
		})
		done <- res
	}()

	time.Sleep(20 * time.Millisecond)
	ad.onAction(CardActionEvent{
		UserID: "u-1",
		Value: map[string]string{
			"kind":        "decision",
			"decision_id": "dec-1",
			"option_id":   "allow",
			"value":       "allow_once",
		},
	})

	res := <-done
	if res.Outcome != "selected" || res.OptionID != "allow" {
		t.Fatalf("decision result = %#v, want selected allow", res)
	}

	// Simulate Feishu duplicate callback delivery for the same decision click.
	ad.onAction(CardActionEvent{
		UserID: "u-1",
		Value: map[string]string{
			"kind":        "decision",
			"decision_id": "dec-1",
			"option_id":   "allow",
			"value":       "allow_once",
		},
	})
	time.Sleep(10 * time.Millisecond)

	logs := buf.String()
	if !containsAll(logs, "event=card_action_accept", "decision_id=\"dec-1\"") {
		t.Fatalf("missing card_action_accept logs: %q", logs)
	}
	if !containsAll(logs, "event=card_action_ignore", "decision_already_closed") {
		t.Fatalf("missing duplicate-ignore logs: %q", logs)
	}
}

func TestForwarder_EmitFlushOnDone(t *testing.T) {
	ad := &stubAdapter{}
	f := New(ad)
	if err := f.Emit(context.Background(), IMUpdate{ChatID: "chat-1", UpdateType: "text", Text: "hel"}); err != nil {
		t.Fatalf("emit text: %v", err)
	}
	if err := f.Emit(context.Background(), IMUpdate{ChatID: "chat-1", UpdateType: "text", Text: "lo"}); err != nil {
		t.Fatalf("emit text: %v", err)
	}
	if err := f.Emit(context.Background(), IMUpdate{ChatID: "chat-1", UpdateType: "done"}); err != nil {
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
	f.SetHelpResolver(func(_ context.Context, _ string) (HelpModel, error) {
		return HelpModel{
			Title: "help",
			Body:  "select",
			Options: []HelpOption{
				{Label: "Mode Plan", Command: "/mode", Value: "plan"},
			},
		}, nil
	})
	f.OnMessage(func(m Message) { got = m.Text })

	ad.onMsg(Message{ChatID: "chat-1", Text: "/help"})
	if got != "" {
		t.Fatalf("help should be intercepted")
	}

	if ad.onAction == nil {
		t.Fatalf("expected card action handler to be registered")
	}
	ad.onAction(CardActionEvent{
		Value: map[string]string{
			"kind":    "help_option",
			"chat_id": "chat-1",
			"command": "/mode",
			"value":   "plan",
		},
	})
	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) && got != "/mode plan" {
		time.Sleep(5 * time.Millisecond)
	}
	if got != "/mode plan" {
		t.Fatalf("injected command = %q, want %q", got, "/mode plan")
	}
}

func TestParseToolCallUpdate(t *testing.T) {
	raw := []byte(`{"sessionUpdate":"tool_call_update","toolCallId":"call_1","title":"Run tests","status":"in_progress","rawOutput":"ok"}`)
	upd, sig, ok := parseToolCallUpdate(raw)
	if !ok {
		t.Fatalf("parseToolCallUpdate returned not ok")
	}
	if sig == "" || upd.ToolCallID != "call_1" || upd.Title != "Run tests" {
		t.Fatalf("unexpected update=%+v signature=%q", upd, sig)
	}
	msg := renderToolCallMessage(upd)
	if !containsAll(msg, "Run tests", "in_progress", "call_1") {
		t.Fatalf("renderToolCallMessage()=%q", msg)
	}
}

func TestRenderConfigOptionUpdate(t *testing.T) {
	raw := []byte(`{"configOptions":[{"id":"mode","currentValue":"plan"},{"id":"model","currentValue":"gpt-5"}]}`)
	got := renderConfigOptionUpdate(raw)
	if !containsAll(got, "mode=plan", "model=gpt-5") {
		t.Fatalf("renderConfigOptionUpdate()=%q", got)
	}
}

func TestForwarder_EmitToolCall_DedupByStatus(t *testing.T) {
	ad := &stubAdapter{}
	f := New(ad)
	raw := []byte(`{"sessionUpdate":"tool_call_update","toolCallId":"call_1","title":"Run tests","status":"in_progress"}`)
	if err := f.Emit(context.Background(), IMUpdate{ChatID: "chat-1", UpdateType: "tool_call", Raw: raw}); err != nil {
		t.Fatalf("emit tool_call: %v", err)
	}
	if err := f.Emit(context.Background(), IMUpdate{ChatID: "chat-1", UpdateType: "tool_call", Raw: raw}); err != nil {
		t.Fatalf("emit duplicate tool_call: %v", err)
	}
	if ad.textCount != 1 {
		t.Fatalf("tool call message count=%d, want 1", ad.textCount)
	}
}

func TestForwarder_EmitToolCall_SameStatusNewOutputStillStreams(t *testing.T) {
	ad := &toolCardStub{}
	f := New(ad)
	raw1 := []byte(`{"sessionUpdate":"tool_call_update","toolCallId":"call_1","title":"Run tests","status":"in_progress","rawOutput":"step1"}`)
	raw2 := []byte(`{"sessionUpdate":"tool_call_update","toolCallId":"call_1","title":"Run tests","status":"in_progress","rawOutput":"step2"}`)
	if err := f.Emit(context.Background(), IMUpdate{ChatID: "chat-1", UpdateType: "tool_call", Raw: raw1}); err != nil {
		t.Fatalf("emit tool_call #1: %v", err)
	}
	if err := f.Emit(context.Background(), IMUpdate{ChatID: "chat-1", UpdateType: "tool_call", Raw: raw2}); err != nil {
		t.Fatalf("emit tool_call #2: %v", err)
	}
	if len(ad.toolCalls) != 2 {
		t.Fatalf("tool card update count=%d, want 2", len(ad.toolCalls))
	}
	if ad.textCount != 0 {
		t.Fatalf("text fallback should not be used when tool cards are supported")
	}
}

func TestForwarder_DebugLogger_LogsInAndOut(t *testing.T) {
	ad := &stubAdapter{}
	f := New(ad)
	var buf bytes.Buffer
	f.SetDebugLogger(&buf)

	f.OnMessage(func(_ Message) {})
	ad.onMsg(Message{ChatID: "chat-1", MessageID: "m-1", UserID: "u-1", Text: "hello"})
	if err := f.SendText("chat-1", "world"); err != nil {
		t.Fatalf("send text: %v", err)
	}

	logs := buf.String()
	if !containsAll(logs, "<-[im]", "event=message", "chat-1", "hello", "->[im]", "event=send_text", "world") {
		t.Fatalf("unexpected logs: %q", logs)
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
