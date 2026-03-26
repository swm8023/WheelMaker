package im

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
)

type stubAdapter struct {
	onMsg                MessageHandler
	onAction             func(CardActionEvent)
	lastChatID           string
	lastText             string
	lastCard             Card
	lastUpdatedMessageID string
	textCount            int
	cardSent             bool
	cardUpdateCount      int
	cardSendCount        int
	runCalled            bool
	doneChatID           string
	doneCount            int
}

type toolCardStub struct {
	stubAdapter
	toolCalls []ToolCallUpdate
}

func (s *toolCardStub) SendCard(chatID, messageID string, card Card) error {
	if tc, ok := card.(ToolCallCard); ok {
		s.toolCalls = append(s.toolCalls, tc.Update)
		return nil
	}
	return s.stubAdapter.SendCard(chatID, messageID, card)
}

func (s *stubAdapter) OnMessage(h MessageHandler) { s.onMsg = h }
func (s *stubAdapter) Send(chatID, text string, _ TextKind) error {
	s.lastChatID = chatID
	s.lastText = text
	s.textCount++
	return nil
}
func (s *stubAdapter) SendCard(chatID, messageID string, card Card) error {
	s.lastChatID = chatID
	s.lastCard = card
	switch c := card.(type) {
	case OptionsCard:
		return s.Send(chatID, "options", TextNormal)
	case ToolCallCard:
		if msg := RenderToolCallMessage(c.Update); msg != "" {
			return s.Send(chatID, msg, TextNormal)
		}
		return nil
	}
	if messageID != "" {
		s.lastUpdatedMessageID = messageID
		s.cardUpdateCount++
	} else {
		s.cardSent = true
		s.cardSendCount++
	}
	return nil
}
func (s *stubAdapter) SendReaction(_, _ string) error { return nil }
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
	header, ok := card["header"].(map[string]any)
	if !ok {
		t.Fatalf("help card header missing: %#v", card["header"])
	}
	if _, ok := header["extra"]; ok {
		t.Fatalf("header extra should be removed, got: %#v", header["extra"])
	}

	elements, ok := card["elements"].([]map[string]any)
	if !ok || len(elements) == 0 {
		t.Fatalf("help card elements missing: %#v", card["elements"])
	}
	hasBackInElements := false
	lastIsBack := false
	for _, el := range elements {
		actions, _ := el["actions"].([]map[string]any)
		for _, act := range actions {
			textMap, _ := act["text"].(map[string]any)
			valueMap, _ := act["value"].(map[string]any)
			if textMap["content"] == "Back" && valueMap["kind"] == "help_menu" && valueMap["menu_id"] == "root" && act["type"] == "primary" {
				hasBackInElements = true
			}
		}
	}
	if !hasBackInElements {
		t.Fatalf("expected fallback back button in elements, got: %#v", elements)
	}
	if len(elements) > 0 {
		if actions, ok := elements[len(elements)-1]["actions"].([]map[string]any); ok && len(actions) == 1 {
			textMap, _ := actions[0]["text"].(map[string]any)
			valueMap, _ := actions[0]["value"].(map[string]any)
			lastIsBack = textMap["content"] == "Back" && valueMap["kind"] == "help_menu" && valueMap["menu_id"] == "root"
		}
	}
	if !lastIsBack {
		t.Fatalf("back button should be in the bottom action row, got: %#v", elements[len(elements)-1])
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

func TestForwarder_HelpOptionCardAction_RefreshesRootWithLatestState(t *testing.T) {
	ad := &stubAdapter{}
	f := New(ad)
	currentAgent := "claude"
	f.SetHelpResolver(func(_ context.Context, _ string) (HelpModel, error) {
		return HelpModel{
			Title:    "help",
			Body:     "root",
			RootMenu: "root",
			Options: []HelpOption{
				{Label: "Agent: " + currentAgent, MenuID: "menu:agents"},
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
		}, nil
	})
	f.OnMessage(func(m Message) {
		if strings.TrimSpace(m.Text) == "/use codex" {
			currentAgent = "codex"
		}
	})

	ad.onMsg(Message{ChatID: "chat-1", Text: "/help"})
	if ad.cardSendCount != 1 {
		t.Fatalf("initial /help should send one card, got %d", ad.cardSendCount)
	}

	ad.onAction(CardActionEvent{
		ChatID:    "chat-1",
		MessageID: "msg-help-1",
		Value: map[string]string{
			"kind":    "help_option",
			"chat_id": "chat-1",
			"menu_id": "menu:agents",
			"command": "/use",
			"value":   "codex",
		},
	})

	if ad.cardUpdateCount != 1 {
		t.Fatalf("help option action should update existing card, update_count=%d", ad.cardUpdateCount)
	}
	if ad.lastUpdatedMessageID != "msg-help-1" {
		t.Fatalf("updated message id = %q, want %q", ad.lastUpdatedMessageID, "msg-help-1")
	}
	card, ok := ad.lastCard.(RawCard)
	if !ok {
		t.Fatalf("updated card type=%T, want RawCard", ad.lastCard)
	}
	elements, _ := card["elements"].([]map[string]any)
	if len(elements) < 2 {
		t.Fatalf("unexpected root card elements: %#v", elements)
	}
	actions, _ := elements[1]["actions"].([]map[string]any)
	if len(actions) == 0 {
		t.Fatalf("root actions missing after refresh: %#v", elements[1])
	}
	textMap, _ := actions[0]["text"].(map[string]any)
	if textMap["content"] != "Agent: codex" {
		t.Fatalf("root did not refresh latest agent label, got=%#v", textMap["content"])
	}
}

func TestForwarder_HelpOptionCardAction_FallbacksToRootWhenPostResolveFails(t *testing.T) {
	ad := &stubAdapter{}
	f := New(ad)
	resolveCalls := 0
	f.SetHelpResolver(func(_ context.Context, _ string) (HelpModel, error) {
		resolveCalls++
		model := HelpModel{
			Title:    "help",
			Body:     "root",
			RootMenu: "root",
			Options: []HelpOption{
				{Label: "Agent: claude", MenuID: "menu:agents"},
			},
			Menus: map[string]HelpMenu{
				"menu:agents": {
					Title:  "Agent Switch",
					Body:   "Choose",
					Parent: "root",
					Options: []HelpOption{
						{Label: "Agent: copilot", Command: "/use", Value: "copilot"},
					},
				},
			},
		}
		if resolveCalls >= 3 {
			return HelpModel{}, context.DeadlineExceeded
		}
		return model, nil
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
			"kind":    "help_option",
			"chat_id": "chat-1",
			"menu_id": "menu:agents",
			"command": "/use",
			"value":   "copilot",
		},
	})

	if ad.cardUpdateCount != 1 {
		t.Fatalf("help option action should still update card to root fallback, update_count=%d", ad.cardUpdateCount)
	}
	if ad.lastUpdatedMessageID != "msg-help-1" {
		t.Fatalf("updated message id = %q, want %q", ad.lastUpdatedMessageID, "msg-help-1")
	}
	card, ok := ad.lastCard.(RawCard)
	if !ok {
		t.Fatalf("updated card type=%T, want RawCard", ad.lastCard)
	}
	header, _ := card["header"].(map[string]any)
	title, _ := header["title"].(map[string]any)
	if title["content"] != "help" {
		t.Fatalf("fallback should navigate to root help title, got=%#v", title["content"])
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
