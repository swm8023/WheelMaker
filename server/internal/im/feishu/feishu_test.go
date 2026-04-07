package feishu

import (
	"context"
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
