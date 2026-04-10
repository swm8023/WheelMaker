package feishu

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/swm8023/wheelmaker/internal/im"
	acp "github.com/swm8023/wheelmaker/internal/protocol"
)

func TestChannelImplementsIMChannel(t *testing.T) {
	var _ im.Channel = New(Config{})
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
	usage    map[string]string
	nextCard int
}

type fakeSend struct {
	chatID string
	text   string
	kind   TextKind
}

type fakeCard struct {
	chatID    string
	messageID string
	card      Card
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

func (f *fakeTransport) SendCard(chatID, messageID string, card Card) (string, error) {
	if messageID == "" {
		f.nextCard++
		messageID = fmt.Sprintf("card-%d", f.nextCard)
	}
	f.cards = append(f.cards, fakeCard{chatID: chatID, messageID: messageID, card: card})
	return messageID, nil
}

func (f *fakeTransport) SendReaction(_, _ string) error { return nil }

func (f *fakeTransport) SetUsage(chatID, usage string) {
	if f.usage == nil {
		f.usage = map[string]string{}
	}
	f.usage[chatID] = usage
}

func (f *fakeTransport) MarkDone(chatID string) error {
	f.done = append(f.done, chatID)
	return nil
}

func (f *fakeTransport) Run(context.Context) error { return nil }

func TestPublishSessionUpdate_RendersByUpdateType(t *testing.T) {
	ft := &fakeTransport{}
	ch := newWithTransport(ft)
	target := im.SendTarget{ChannelID: "feishu", ChatID: "chat-a"}

	textUpdate := acp.SessionUpdateParams{
		SessionID: "s1",
		Update: acp.SessionUpdate{
			SessionUpdate: acp.SessionUpdateAgentMessageChunk,
			Content:       []byte(`{"type":"text","text":"hello"}`),
		},
	}
	thoughtUpdate := acp.SessionUpdateParams{
		SessionID: "s1",
		Update: acp.SessionUpdate{
			SessionUpdate: acp.SessionUpdateAgentThoughtChunk,
			Content:       []byte(`{"type":"text","text":"thinking"}`),
		},
	}
	toolUpdate := acp.SessionUpdateParams{
		SessionID: "s1",
		Update: acp.SessionUpdate{
			SessionUpdate: acp.SessionUpdateToolCallUpdate,
			ToolCallID:    "tc-1",
			Title:         "Read file",
			Status:        acp.ToolCallStatusCompleted,
		},
	}

	if err := ch.PublishSessionUpdate(context.Background(), target, textUpdate); err != nil {
		t.Fatalf("PublishSessionUpdate(text): %v", err)
	}
	if err := ch.PublishSessionUpdate(context.Background(), target, thoughtUpdate); err != nil {
		t.Fatalf("PublishSessionUpdate(thought): %v", err)
	}
	if err := ch.PublishSessionUpdate(context.Background(), target, toolUpdate); err != nil {
		t.Fatalf("PublishSessionUpdate(tool): %v", err)
	}

	if len(ft.sends) != 2 {
		t.Fatalf("sends=%+v, want 2", ft.sends)
	}
	if ft.sends[0].kind != TextNormal || ft.sends[0].text != "hello" {
		t.Fatalf("text send=%+v", ft.sends[0])
	}
	if ft.sends[1].kind != TextThought || ft.sends[1].text != "thinking" {
		t.Fatalf("thought send=%+v", ft.sends[1])
	}
	if len(ft.cards) != 1 {
		t.Fatalf("cards=%+v, want one tool card", ft.cards)
	}
	if _, ok := ft.cards[0].card.(ToolCallCard); !ok {
		t.Fatalf("card type=%T, want ToolCallCard", ft.cards[0].card)
	}
}

func TestPublishSessionUpdate_BlockThoughtAtChannelLevel(t *testing.T) {
	ft := &fakeTransport{}
	ch := newWithTransportConfig(ft, Config{BlockedUpdates: []string{"thought"}})
	target := im.SendTarget{ChannelID: "feishu", ChatID: "chat-a"}

	err := ch.PublishSessionUpdate(context.Background(), target, acp.SessionUpdateParams{
		SessionID: "s1",
		Update: acp.SessionUpdate{
			SessionUpdate: acp.SessionUpdateAgentThoughtChunk,
			Content:       []byte(`{"type":"text","text":"hidden-thought"}`),
		},
	})
	if err != nil {
		t.Fatalf("PublishSessionUpdate(thought): %v", err)
	}
	if len(ft.sends) != 0 {
		t.Fatalf("thought update should be filtered by feishu channel, sends=%+v", ft.sends)
	}
}

func TestPublishSessionUpdate_BlockToolCardsAtChannelLevel(t *testing.T) {
	ft := &fakeTransport{}
	ch := newWithTransportConfig(ft, Config{BlockedUpdates: []string{"tool"}})
	target := im.SendTarget{ChannelID: "feishu", ChatID: "chat-a"}

	err := ch.PublishSessionUpdate(context.Background(), target, acp.SessionUpdateParams{
		SessionID: "s1",
		Update: acp.SessionUpdate{
			SessionUpdate: acp.SessionUpdateToolCallUpdate,
			ToolCallID:    "tc-1",
			Title:         "Read file",
			Status:        acp.ToolCallStatusCompleted,
		},
	})
	if err != nil {
		t.Fatalf("PublishSessionUpdate(tool): %v", err)
	}
	if len(ft.cards) != 0 {
		t.Fatalf("tool update should be filtered by feishu channel, cards=%+v", ft.cards)
	}
}

func TestPublishPromptResult_EndTurnMarksDone(t *testing.T) {
	ft := &fakeTransport{}
	ch := newWithTransport(ft)

	err := ch.PublishPromptResult(context.Background(), im.SendTarget{ChannelID: "feishu", ChatID: "chat-a"}, acp.SessionPromptResult{
		StopReason: acp.StopReasonEndTurn,
	})
	if err != nil {
		t.Fatalf("PublishPromptResult: %v", err)
	}
	if len(ft.done) != 1 || ft.done[0] != "chat-a" {
		t.Fatalf("done=%+v", ft.done)
	}
}

func TestPublishPermissionRequest_StoresRequestIDAndToolCallID(t *testing.T) {
	ft := &fakeTransport{}
	ch := newWithTransport(ft)

	err := ch.PublishPermissionRequest(context.Background(), im.SendTarget{ChannelID: "feishu", ChatID: "chat-a"}, 42, acp.PermissionRequestParams{
		SessionID: "acp-1",
		ToolCall:  acp.ToolCallRef{ToolCallID: "call-1", Title: "Write file"},
		Options: []acp.PermissionOption{
			{OptionID: "allow", Name: "Allow", Kind: "allow_once"},
		},
	})
	if err != nil {
		t.Fatalf("PublishPermissionRequest: %v", err)
	}
	if got := ch.pendingByRequestID[42].ToolCallID; got != "call-1" {
		t.Fatalf("toolCallID=%q, want call-1", got)
	}
	if len(ft.cards) != 1 {
		t.Fatalf("cards=%+v", ft.cards)
	}
	card, ok := ft.cards[0].card.(OptionsCard)
	if !ok {
		t.Fatalf("card type=%T", ft.cards[0].card)
	}
	if card.Meta["request_id"] != "42" || card.Meta["tool_call_id"] != "call-1" {
		t.Fatalf("card meta=%+v", card.Meta)
	}
}

func TestHandleMessage_RoutesPromptAndCommand(t *testing.T) {
	ft := &fakeTransport{}
	ch := newWithTransport(ft)

	var gotPrompt string
	var gotCmd im.Command
	ch.OnPrompt(func(_ context.Context, _ im.ChatRef, params acp.SessionPromptParams) error {
		gotPrompt = params.Prompt[0].Text
		return nil
	})
	ch.OnCommand(func(_ context.Context, _ im.ChatRef, cmd im.Command) error {
		gotCmd = cmd
		return nil
	})

	ft.onMsg(Message{ChatID: "chat-a", Text: "hello"})
	if gotPrompt != "hello" {
		t.Fatalf("gotPrompt=%q", gotPrompt)
	}

	ft.onMsg(Message{ChatID: "chat-a", Text: "/cancel"})
	if gotCmd.Name != "/cancel" {
		t.Fatalf("gotCmd=%+v", gotCmd)
	}
}

func TestHandleMessage_CachesImageThenMergesWithTextPrompt(t *testing.T) {
	ft := &fakeTransport{}
	ch := newWithTransport(ft)

	var gotPrompt []acp.ContentBlock
	ch.OnPrompt(func(_ context.Context, _ im.ChatRef, params acp.SessionPromptParams) error {
		gotPrompt = append([]acp.ContentBlock(nil), params.Prompt...)
		return nil
	})

	ft.onMsg(Message{ChatID: "chat-a", Prompt: []acp.ContentBlock{{Type: acp.ContentBlockTypeImage, MimeType: "image/png", Data: "aGVsbG8="}}})
	if len(gotPrompt) != 0 {
		t.Fatalf("image-only message should be cached, got prompt=%+v", gotPrompt)
	}

	ft.onMsg(Message{ChatID: "chat-a", Text: "describe this image"})
	if len(gotPrompt) != 2 {
		t.Fatalf("gotPrompt=%+v, want 2 blocks", gotPrompt)
	}
	if gotPrompt[0].Type != acp.ContentBlockTypeImage {
		t.Fatalf("first block type=%q, want image", gotPrompt[0].Type)
	}
	if gotPrompt[1].Type != acp.ContentBlockTypeText || gotPrompt[1].Text != "describe this image" {
		t.Fatalf("second block=%+v", gotPrompt[1])
	}
}

func TestHandleMessage_CommandDoesNotConsumeCachedImage(t *testing.T) {
	ft := &fakeTransport{}
	ch := newWithTransport(ft)

	var gotPrompt []acp.ContentBlock
	var gotCmd im.Command
	ch.OnPrompt(func(_ context.Context, _ im.ChatRef, params acp.SessionPromptParams) error {
		gotPrompt = append([]acp.ContentBlock(nil), params.Prompt...)
		return nil
	})
	ch.OnCommand(func(_ context.Context, _ im.ChatRef, cmd im.Command) error {
		gotCmd = cmd
		return nil
	})

	ft.onMsg(Message{ChatID: "chat-a", Prompt: []acp.ContentBlock{{Type: acp.ContentBlockTypeImage, MimeType: "image/png", Data: "aGVsbG8="}}})
	ft.onMsg(Message{ChatID: "chat-a", Text: "/status"})
	if gotCmd.Name != "/status" {
		t.Fatalf("gotCmd=%+v", gotCmd)
	}
	if len(gotPrompt) != 0 {
		t.Fatalf("command should not trigger prompt, got=%+v", gotPrompt)
	}

	ft.onMsg(Message{ChatID: "chat-a", Text: "now answer"})
	if len(gotPrompt) != 2 {
		t.Fatalf("cached image should still be merged, got=%+v", gotPrompt)
	}
}
func TestPermissionCardAction_ResolvesWithRequestID(t *testing.T) {
	ft := &fakeTransport{}
	ch := newWithTransport(ft)

	var gotRequestID int64
	var gotResult acp.PermissionResponse
	ch.OnPermissionResponse(func(_ context.Context, _ im.ChatRef, requestID int64, result acp.PermissionResponse) error {
		gotRequestID = requestID
		gotResult = result
		return nil
	})

	err := ch.PublishPermissionRequest(context.Background(), im.SendTarget{ChannelID: "feishu", ChatID: "chat-a"}, 42, acp.PermissionRequestParams{
		SessionID: "acp-1",
		ToolCall:  acp.ToolCallRef{ToolCallID: "call-1", Title: "Write file"},
		Options: []acp.PermissionOption{
			{OptionID: "allow", Name: "Allow", Kind: "allow_once"},
		},
	})
	if err != nil {
		t.Fatalf("PublishPermissionRequest: %v", err)
	}

	ft.onAction(CardActionEvent{
		ChatID: "chat-a",
		Value: map[string]string{
			"kind":       "permission",
			"request_id": "42",
			"option_id":  "allow",
		},
	})

	if gotRequestID != 42 {
		t.Fatalf("requestID=%d", gotRequestID)
	}
	if gotResult.Outcome.Outcome != "selected" || gotResult.Outcome.OptionID != "allow" {
		t.Fatalf("result=%+v", gotResult)
	}
}

func TestPermissionTextReply_ResolvesWithPendingRequest(t *testing.T) {
	ft := &fakeTransport{}
	ch := newWithTransport(ft)

	var gotRequestID int64
	var gotResult acp.PermissionResponse
	ch.OnPermissionResponse(func(_ context.Context, _ im.ChatRef, requestID int64, result acp.PermissionResponse) error {
		gotRequestID = requestID
		gotResult = result
		return nil
	})

	err := ch.PublishPermissionRequest(context.Background(), im.SendTarget{ChannelID: "feishu", ChatID: "chat-a"}, 7, acp.PermissionRequestParams{
		SessionID: "acp-1",
		ToolCall:  acp.ToolCallRef{ToolCallID: "call-1", Title: "Write file"},
		Options: []acp.PermissionOption{
			{OptionID: "allow", Name: "Allow", Kind: "allow_once"},
		},
	})
	if err != nil {
		t.Fatalf("PublishPermissionRequest: %v", err)
	}

	ft.onMsg(Message{ChatID: "chat-a", Text: "1"})

	if gotRequestID != 7 {
		t.Fatalf("requestID=%d", gotRequestID)
	}
	if gotResult.Outcome.Outcome != "selected" || gotResult.Outcome.OptionID != "allow" {
		t.Fatalf("result=%+v", gotResult)
	}
}

func TestSystemNotify_HelpCardAlwaysCreatesNewMessage(t *testing.T) {
	ft := &fakeTransport{}
	ch := newWithTransport(ft)
	target := im.SendTarget{ChannelID: "feishu", ChatID: "chat-a"}
	payload := im.SystemPayload{
		Kind: "help_card",
		HelpCard: &im.HelpCardPayload{
			Model: im.HelpModel{
				Title: "Help",
				Body:  "body",
			},
		},
	}

	if err := ch.SystemNotify(context.Background(), target, payload); err != nil {
		t.Fatalf("SystemNotify(first): %v", err)
	}
	if err := ch.SystemNotify(context.Background(), target, payload); err != nil {
		t.Fatalf("SystemNotify(second): %v", err)
	}

	if len(ft.cards) != 2 {
		t.Fatalf("cards=%+v, want 2 help sends", ft.cards)
	}
	if ft.cards[0].messageID == "" {
		t.Fatal("first help card should produce a message id")
	}
	if ft.cards[1].messageID == ft.cards[0].messageID {
		t.Fatalf("second help card should create a new message, got reused messageID %q", ft.cards[1].messageID)
	}
}

func TestSystemNotify_TitleKeepsEmojiAndBodyNoDuplicate(t *testing.T) {
	ft := &fakeTransport{}
	ch := newWithTransport(ft)
	target := im.SendTarget{ChannelID: "feishu", ChatID: "chat-a"}
	payload := im.SystemPayload{
		Title: "Switched",
		Body:  "Session Ready",
	}

	if err := ch.SystemNotify(context.Background(), target, payload); err != nil {
		t.Fatalf("SystemNotify: %v", err)
	}
	if len(ft.cards) != 1 {
		t.Fatalf("cards=%+v, want 1", ft.cards)
	}
	raw, ok := ft.cards[0].card.(RawCard)
	if !ok {
		t.Fatalf("card type=%T, want RawCard", ft.cards[0].card)
	}
	header, ok := raw["header"].(map[string]any)
	if !ok {
		t.Fatalf("header missing in card: %+v", raw)
	}
	titleMap, ok := header["title"].(map[string]any)
	if !ok {
		t.Fatalf("title missing in header: %+v", header)
	}
	title, _ := titleMap["content"].(string)
	if !strings.Contains(title, "📣") {
		t.Fatalf("title should keep loudspeaker emoji, got %q", title)
	}
	body, ok := raw["body"].(map[string]any)
	if !ok {
		t.Fatalf("body missing in card: %+v", raw)
	}
	elements, ok := body["elements"].([]map[string]any)
	if !ok || len(elements) == 0 {
		t.Fatalf("body elements missing in card: %+v", raw)
	}
	content, _ := elements[0]["content"].(string)
	if content != "Session Ready" {
		t.Fatalf("content=%q, want %q", content, "Session Ready")
	}
}

func TestPublishSessionUpdate_UsageUpdateDoesNotSyncRealtime(t *testing.T) {
	ft := &fakeTransport{}
	ch := newWithTransport(ft)
	target := im.SendTarget{ChannelID: "feishu", ChatID: "chat-a"}

	size := int64(258400)
	used := int64(183223)
	err := ch.PublishSessionUpdate(context.Background(), target, acp.SessionUpdateParams{
		SessionID: "s1",
		Update: acp.SessionUpdate{
			SessionUpdate: acp.SessionUpdateUsageUpdate,
			Size:          &size,
			Used:          &used,
		},
	})
	if err != nil {
		t.Fatalf("PublishSessionUpdate(usage): %v", err)
	}
	if len(ft.cards) != 0 || len(ft.sends) != 0 {
		t.Fatalf("usage update should not sync in realtime, sends=%+v cards=%+v", ft.sends, ft.cards)
	}
	if got := ft.usage["chat-a"]; got != "Context" {
		t.Fatalf("usage=%q, want Context", got)
	}
}

func TestPublishPromptResult_EndTurnDoesNotAppendUsageText(t *testing.T) {
	ft := &fakeTransport{}
	ch := newWithTransport(ft)
	target := im.SendTarget{ChannelID: "feishu", ChatID: "chat-a"}

	size := int64(258400)
	used := int64(183223)
	err := ch.PublishSessionUpdate(context.Background(), target, acp.SessionUpdateParams{
		SessionID: "s1",
		Update: acp.SessionUpdate{
			SessionUpdate: acp.SessionUpdateUsageUpdate,
			Size:          &size,
			Used:          &used,
		},
	})
	if err != nil {
		t.Fatalf("PublishSessionUpdate(usage): %v", err)
	}
	if err := ch.PublishPromptResult(context.Background(), target, acp.SessionPromptResult{StopReason: acp.StopReasonEndTurn}); err != nil {
		t.Fatalf("PublishPromptResult(end_turn): %v", err)
	}
	if len(ft.sends) != 0 {
		t.Fatalf("end_turn should not append usage text via channel, sends=%+v", ft.sends)
	}
	if len(ft.done) != 1 || ft.done[0] != "chat-a" {
		t.Fatalf("done=%+v", ft.done)
	}
}

func TestPublishSessionUpdate_BlockUsageAtChannelLevel(t *testing.T) {
	ft := &fakeTransport{}
	ch := newWithTransportConfig(ft, Config{BlockedUpdates: []string{"usage"}})
	target := im.SendTarget{ChannelID: "feishu", ChatID: "chat-a"}

	size := int64(258400)
	used := int64(183223)
	err := ch.PublishSessionUpdate(context.Background(), target, acp.SessionUpdateParams{
		SessionID: "s1",
		Update: acp.SessionUpdate{
			SessionUpdate: acp.SessionUpdateUsageUpdate,
			Size:          &size,
			Used:          &used,
		},
	})
	if err != nil {
		t.Fatalf("PublishSessionUpdate(usage): %v", err)
	}
	if len(ft.cards) != 0 || len(ft.sends) != 0 {
		t.Fatalf("usage update should be filtered by feishu channel, sends=%+v cards=%+v", ft.sends, ft.cards)
	}
	if len(ft.usage) != 0 {
		t.Fatalf("usage update should be filtered by feishu channel, usage=%+v", ft.usage)
	}
}
