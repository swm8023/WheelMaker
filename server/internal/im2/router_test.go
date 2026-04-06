package im2

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type fakeState struct {
	mu sync.Mutex

	bindings map[string]IMActiveChat
	sessions map[string]struct{}
}

func newFakeState() *fakeState {
	return &fakeState{
		bindings: map[string]IMActiveChat{},
		sessions: map[string]struct{}{},
	}
}

func (s *fakeState) Load(context.Context) error { return nil }

func (s *fakeState) EnsureClientSession(_ context.Context, clientSessionID string, _ bool) error {
	s.mu.Lock()
	s.sessions[clientSessionID] = struct{}{}
	s.mu.Unlock()
	return nil
}

func (s *fakeState) ResolveClientSessionID(_ context.Context, activeChatID string) (string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	chat, ok := s.bindings[activeChatID]
	if !ok {
		return "", false, nil
	}
	return chat.ClientSessionID, true, nil
}

func (s *fakeState) BindActiveChat(_ context.Context, activeChatID, imType, chatID, clientSessionID string, _ bool) error {
	s.mu.Lock()
	s.bindings[activeChatID] = IMActiveChat{
		ActiveChatID:    activeChatID,
		IMType:          imType,
		ChatID:          chatID,
		ClientSessionID: clientSessionID,
		Online:          true,
		UpdatedAt:       time.Now(),
	}
	s.sessions[clientSessionID] = struct{}{}
	s.mu.Unlock()
	return nil
}

func (s *fakeState) SetActiveChatOnline(_ context.Context, activeChatID string, online bool, _ bool) error {
	s.mu.Lock()
	chat := s.bindings[activeChatID]
	chat.Online = online
	chat.UpdatedAt = time.Now()
	s.bindings[activeChatID] = chat
	s.mu.Unlock()
	return nil
}

func (s *fakeState) ListSessionActiveChats(_ context.Context, clientSessionID string, onlineOnly bool) ([]IMActiveChat, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]IMActiveChat, 0, 4)
	for _, chat := range s.bindings {
		if chat.ClientSessionID != clientSessionID {
			continue
		}
		if onlineOnly && !chat.Online {
			continue
		}
		out = append(out, chat)
	}
	return out, nil
}

func (s *fakeState) Close() error { return nil }

var _ State = (*fakeState)(nil)

type capturedPublish struct {
	chatID string
	event  OutboundEvent
}

type fakePublisher struct {
	mu      sync.Mutex
	records []capturedPublish
	err     error
}

func (p *fakePublisher) PublishToChat(_ context.Context, chatID string, event OutboundEvent) error {
	if p.err != nil {
		return p.err
	}
	p.mu.Lock()
	p.records = append(p.records, capturedPublish{chatID: chatID, event: event})
	p.mu.Unlock()
	return nil
}

func (p *fakePublisher) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.records)
}

func TestRouterHandleInboundCreatesBinding(t *testing.T) {
	state := newFakeState()
	var got InboundEvent
	r, err := NewRouter("proj", state, func(context.Context) (string, error) {
		return "session-1", nil
	}, func(_ context.Context, event InboundEvent) error {
		got = event
		return nil
	})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	if err := r.HandleInbound(context.Background(), InboundEvent{
		Kind:   InboundPrompt,
		IMType: "feishu",
		ChatID: "chat-1",
		Text:   "hello",
	}); err != nil {
		t.Fatalf("HandleInbound: %v", err)
	}

	if got.ClientSessionID != "session-1" {
		t.Fatalf("clientSessionId=%q, want session-1", got.ClientSessionID)
	}
	if got.ActiveChatID != "feishu:chat-1" {
		t.Fatalf("activeChatId=%q, want feishu:chat-1", got.ActiveChatID)
	}
}

func TestRouterPublishBroadcastsToOnlineChats(t *testing.T) {
	state := newFakeState()
	_ = state.BindActiveChat(context.Background(), "feishu:chat-1", "feishu", "chat-1", "session-1", true)
	_ = state.BindActiveChat(context.Background(), "feishu:chat-2", "feishu", "chat-2", "session-1", true)

	publisher := &fakePublisher{}
	r, err := NewRouter("proj", state, func(context.Context) (string, error) {
		return "", errors.New("not used")
	}, nil)
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	if err := r.RegisterPublisher("feishu", publisher); err != nil {
		t.Fatalf("RegisterPublisher: %v", err)
	}

	if err := r.Publish(context.Background(), OutboundEvent{
		Kind:            OutboundMessage,
		ClientSessionID: "session-1",
		Text:            "hello all",
	}); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	if publisher.count() != 2 {
		t.Fatalf("publish count=%d, want 2", publisher.count())
	}
}

func TestRouterPublishTargeted(t *testing.T) {
	state := newFakeState()
	_ = state.BindActiveChat(context.Background(), "feishu:chat-1", "feishu", "chat-1", "session-1", true)
	_ = state.BindActiveChat(context.Background(), "feishu:chat-2", "feishu", "chat-2", "session-1", true)

	publisher := &fakePublisher{}
	r, err := NewRouter("proj", state, func(context.Context) (string, error) {
		return "", errors.New("not used")
	}, nil)
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	if err := r.RegisterPublisher("feishu", publisher); err != nil {
		t.Fatalf("RegisterPublisher: %v", err)
	}

	if err := r.Publish(context.Background(), OutboundEvent{
		Kind:               OutboundCommandReply,
		ClientSessionID:    "session-1",
		TargetActiveChatID: "feishu:chat-2",
		Text:               "only one",
	}); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	if publisher.count() != 1 {
		t.Fatalf("publish count=%d, want 1", publisher.count())
	}
	if publisher.records[0].chatID != "chat-2" {
		t.Fatalf("target chat=%q, want chat-2", publisher.records[0].chatID)
	}
}
