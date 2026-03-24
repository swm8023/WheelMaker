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
	lastCard   Card
	lastUpdatedMessageID string
	textCount  int
	cardSent   bool
	cardUpdateCount int
	cardSendCount int
	runCalled  bool
	doneChatID string
	doneCount  int
	streamOps  []bool
	streamChat []string
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
func (s *stubAdapter) SendCard(chatID string, card Card) error {
	s.lastChatID = chatID
	s.lastCard = card
	s.cardSent = true
	s.cardSendCount++
	return nil
}
func (s *stubAdapter) UpdateCard(chatID, messageID string, card Card) error {
	s.lastChatID = chatID
	s.lastUpdatedMessageID = messageID
	s.lastCard = card
	s.cardUpdateCount++
	return nil
}
func (s *stubAdapter) SendReaction(_, _ string) error  { return nil }
func (s *stubAdapter) SetStreaming(chatID string, active bool) error {
	s.streamChat = append(s.streamChat, chatID)
	s.streamOps = append(s.streamOps, active)
	return nil
}
func (s *stubAdapter) MarkDone(chatID string) error {
	s.doneChatID = chatID
	s.doneCount++
	return nil
}
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
	if ad.doneCount != 1 || ad.doneChatID != "chat-1" {
		t.Fatalf("done marker (%d,%q), want (1,%q)", ad.doneCount, ad.doneChatID, "chat-1")
	}
}

func TestForwarder_EmitStreamingMarkerLifecycle(t *testing.T) {
	ad := &stubAdapter{}
	f := New(ad)
	if err := f.Emit(context.Background(), IMUpdate{ChatID: "chat-1", UpdateType: IMUpdateText, Text: "hello"}); err != nil {
		t.Fatalf("emit text: %v", err)
	}
	if len(ad.streamOps) != 1 || !ad.streamOps[0] || ad.streamChat[0] != "chat-1" {
		t.Fatalf("stream start = (%v,%v), want ([true],[chat-1])", ad.streamOps, ad.streamChat)
	}
	if err := f.Emit(context.Background(), IMUpdate{ChatID: "chat-1", UpdateType: IMUpdateDone}); err != nil {
		t.Fatalf("emit done: %v", err)
	}
	if len(ad.streamOps) < 2 || ad.streamOps[len(ad.streamOps)-1] {
		t.Fatalf("stream end ops=%v, want trailing false", ad.streamOps)
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

func TestBuildHelpCard_SubmenuHasBackButton(t *testing.T) {
	model := HelpModel{
		Title:    "WheelMaker Help",
		Body:     "root body",
		RootMenu: "root",
		Options: []HelpOption{
			{Label: "Agent Switch", MenuID: "menu:agents"},
		},
		Menus: map[string]HelpMenu{
			"menu:agents": {
				Title:  "Agent Switch",
				Body:   "Choose",
				Parent: "root",
				Options: []HelpOption{
					{Label: "Agent: codex", Command: "/use", Value: "codex"},
				},
			},
		},
	}
	card := buildHelpCard("chat-1", model, "menu:agents", 0)
	elements, ok := card["elements"].([]map[string]any)
	if !ok || len(elements) == 0 {
		t.Fatalf("help card elements missing: %#v", card["elements"])
	}
	last := elements[len(elements)-1]
	actions, ok := last["actions"].([]map[string]any)
	if !ok || len(actions) != 1 {
		t.Fatalf("back action block missing: %#v", last)
	}
	btn := actions[0]
	text, _ := btn["text"].(map[string]any)
	if text["content"] != "Back" {
		t.Fatalf("back button label = %#v, want Back", text["content"])
	}
	value, _ := btn["value"].(map[string]any)
	if value["kind"] != "help_menu" || value["menu_id"] != "root" {
		t.Fatalf("back button value = %#v", value)
	}
}

func TestBuildHelpCard_RootMenuEntryNavigatesSubmenu(t *testing.T) {
	model := HelpModel{
		Title:    "WheelMaker Help",
		Body:     "root body",
		RootMenu: "root",
		Options: []HelpOption{
			{Label: "Config: mode", MenuID: "menu:config:mode"},
		},
	}
	card := buildHelpCard("chat-1", model, "root", 0)
	elements, _ := card["elements"].([]map[string]any)
	if len(elements) < 2 {
		t.Fatalf("unexpected elements: %#v", elements)
	}
	actions, _ := elements[1]["actions"].([]map[string]any)
	if len(actions) != 1 {
		t.Fatalf("unexpected action count: %#v", actions)
	}
	value, _ := actions[0]["value"].(map[string]any)
	if value["kind"] != "help_menu" || value["menu_id"] != "menu:config:mode" {
		t.Fatalf("root menu entry should navigate submenu, got: %#v", value)
	}
}

func TestForwarder_HelpPageCardAction_UpdatesOriginalCard(t *testing.T) {
	ad := &stubAdapter{}
	f := New(ad)
	f.SetHelpResolver(func(_ context.Context, _ string) (HelpModel, error) {
		opts := make([]HelpOption, 0, 10)
		for i := 0; i < 10; i++ {
			opts = append(opts, HelpOption{Label: "Opt", Command: "/status"})
		}
		return HelpModel{
			Title:   "help",
			Body:    "select",
			Options: opts,
		}, nil
	})
	f.OnMessage(func(_ Message) {})

	ad.onMsg(Message{ChatID: "chat-1", Text: "/help"})
	if ad.cardSendCount != 1 {
		t.Fatalf("initial /help should send one card, got %d", ad.cardSendCount)
	}
	ad.onAction(CardActionEvent{
		ChatID:    "chat-1",
		MessageID: "msg-help-1",
		Value: map[string]string{
			"kind":    "help_page",
			"chat_id": "chat-1",
			"page":    "1",
		},
	})
	if ad.cardUpdateCount != 1 {
		t.Fatalf("help page action should update existing card, update_count=%d", ad.cardUpdateCount)
	}
	if ad.lastUpdatedMessageID != "msg-help-1" {
		t.Fatalf("updated message id = %q, want %q", ad.lastUpdatedMessageID, "msg-help-1")
	}
	if ad.cardSendCount != 1 {
		t.Fatalf("help page action should not create new card, send_count=%d", ad.cardSendCount)
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

func TestRenderToolCallMessage_StripsCodeFences(t *testing.T) {
	raw := []byte("{\"sessionUpdate\":\"tool_call_update\",\"toolCallId\":\"call_1\",\"title\":\"Run skill\",\"status\":\"in_progress\",\"toolCallContent\":[{\"type\":\"content\",\"content\":{\"type\":\"text\",\"text\":\"```sh\\necho hi\\n```\"}}]}")
	upd, _, ok := parseToolCallUpdate(raw)
	if !ok {
		t.Fatalf("parseToolCallUpdate returned not ok")
	}
	msg := renderToolCallMessage(upd)
	if strings.Contains(msg, "```") {
		t.Fatalf("renderToolCallMessage should strip code fences, got: %q", msg)
	}
	if !containsAll(msg, "echo hi") {
		t.Fatalf("renderToolCallMessage()=%q", msg)
	}
}

func TestRenderToolCallMessage_UsesLatestToolCallContent(t *testing.T) {
	raw := []byte("{\"sessionUpdate\":\"tool_call_update\",\"toolCallId\":\"call_1\",\"title\":\"Run skill\",\"status\":\"in_progress\",\"toolCallContent\":[{\"type\":\"content\",\"content\":{\"type\":\"text\",\"text\":\"old chunk\"}},{\"type\":\"content\",\"content\":{\"type\":\"text\",\"text\":\"new chunk\"}}]}")
	upd, _, ok := parseToolCallUpdate(raw)
	if !ok {
		t.Fatalf("parseToolCallUpdate returned not ok")
	}
	msg := renderToolCallMessage(upd)
	if strings.Contains(msg, "old chunk") {
		t.Fatalf("renderToolCallMessage should prefer latest content, got: %q", msg)
	}
	if !containsAll(msg, "new chunk") {
		t.Fatalf("renderToolCallMessage()=%q", msg)
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
