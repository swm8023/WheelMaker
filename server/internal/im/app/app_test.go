package app

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/swm8023/wheelmaker/internal/im"
	acp "github.com/swm8023/wheelmaker/internal/protocol"
)

type chatRequestHandler interface {
	HandleChatRequest(context.Context, string, string, json.RawMessage) (any, error)
}

type eventPublisherSetter interface {
	SetEventPublisher(string, func(string, any) error)
}

func TestChannelHandleChatSendDispatchesPrompt(t *testing.T) {
	ch := New()
	handler, ok := any(ch).(chatRequestHandler)
	if !ok {
		t.Fatal("Channel does not implement chat request handling")
	}

	var gotSource im.ChatRef
	var gotPrompt acp.SessionPromptParams
	ch.OnPrompt(func(_ context.Context, source im.ChatRef, params acp.SessionPromptParams) error {
		gotSource = source
		gotPrompt = params
		return nil
	})

	resp, err := handler.HandleChatRequest(context.Background(), "chat.send", "hub-a:proj1", json.RawMessage(`{"chatId":"chat-1","text":"hello app chat"}`))
	if err != nil {
		t.Fatalf("HandleChatRequest() err = %v", err)
	}
	payload, ok := resp.(map[string]any)
	if !ok || payload["ok"] != true {
		t.Fatalf("response=%#v, want ok=true map", resp)
	}
	if gotSource.ChannelID != "app" || gotSource.ChatID != "chat-1" {
		t.Fatalf("source=%+v", gotSource)
	}
	if len(gotPrompt.Prompt) != 1 || gotPrompt.Prompt[0].Type != acp.ContentBlockTypeText || gotPrompt.Prompt[0].Text != "hello app chat" {
		t.Fatalf("prompt=%+v", gotPrompt.Prompt)
	}
}

func TestChannelPublishSessionUpdateEmitsChatMessageEvent(t *testing.T) {
	ch := New()
	handler, ok := any(ch).(chatRequestHandler)
	if !ok {
		t.Fatal("Channel does not implement chat request handling")
	}
	publisher, ok := any(ch).(eventPublisherSetter)
	if !ok {
		t.Fatal("Channel does not expose event publisher binding")
	}

	var publishedProjectID string
	var publishedPayload map[string]any
	publisher.SetEventPublisher("hub-a:proj1", func(projectID string, payload any) error {
		publishedProjectID = projectID
		mapped, ok := payload.(map[string]any)
		if !ok {
			t.Fatalf("payload type=%T, want map[string]any", payload)
		}
		publishedPayload = mapped
		return nil
	})

	ch.OnPrompt(func(_ context.Context, _ im.ChatRef, _ acp.SessionPromptParams) error {
		return nil
	})
	if _, err := handler.HandleChatRequest(context.Background(), "chat.send", "hub-a:proj1", json.RawMessage(`{"chatId":"chat-1","text":"hello app chat"}`)); err != nil {
		t.Fatalf("HandleChatRequest() err = %v", err)
	}

	err := ch.PublishSessionUpdate(context.Background(), im.SendTarget{ChatID: "chat-1", SessionID: "sess-1"}, acp.SessionUpdateParams{
		SessionID: "sess-1",
		Update: acp.SessionUpdate{
			SessionUpdate: acp.SessionUpdateAgentMessageChunk,
			Content:       acp.MustRaw(acp.ContentBlock{Type: acp.ContentBlockTypeText, Text: "stream hello"}),
		},
	})
	if err != nil {
		t.Fatalf("PublishSessionUpdate() err = %v", err)
	}
	if publishedProjectID != "hub-a:proj1" {
		t.Fatalf("publishedProjectID=%q", publishedProjectID)
	}
	session, _ := publishedPayload["session"].(map[string]any)
	if session["chatId"] != "chat-1" {
		t.Fatalf("session=%v", session)
	}
	message, _ := publishedPayload["message"].(map[string]any)
	if message["role"] != "assistant" || message["text"] != "stream hello" || message["status"] != "streaming" {
		t.Fatalf("message=%v", message)
	}
}

func TestChannelRejectsPermissionResponseMethod(t *testing.T) {
	ch := New()
	handler, ok := any(ch).(chatRequestHandler)
	if !ok {
		t.Fatal("Channel does not implement chat request handling")
	}

	_, err := handler.HandleChatRequest(context.Background(), "chat.permission.respond", "hub-a:proj1", json.RawMessage(`{"chatId":"chat-1","requestId":42,"optionId":"allow"}`))
	if err == nil {
		t.Fatal("HandleChatRequest() error = nil, want unsupported chat method")
	}
	if got := err.Error(); got != "unsupported chat method: chat.permission.respond" {
		t.Fatalf("HandleChatRequest() err = %q, want unsupported chat method", got)
	}
}
