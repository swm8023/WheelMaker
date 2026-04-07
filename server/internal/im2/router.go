package im2

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

type binding struct {
	sessionID string
	watch     bool
}

type Router struct {
	mu       sync.RWMutex
	client   InboundHandler
	history  SessionHistoryStore
	channels map[string]Channel
	bindings map[ChatRef]binding
}

func NewRouter(client InboundHandler, history SessionHistoryStore) *Router {
	if history == nil {
		history = NoopHistoryStore{}
	}
	return &Router{
		client:   client,
		history:  history,
		channels: map[string]Channel{},
		bindings: map[ChatRef]binding{},
	}
}

func (r *Router) RegisterChannel(ch Channel) error {
	if ch == nil {
		return fmt.Errorf("im2: channel is nil")
	}
	id := normalize(ch.ID())
	if id == "" {
		return fmt.Errorf("im2: channel id is empty")
	}
	r.mu.Lock()
	r.channels[id] = ch
	r.mu.Unlock()
	ch.OnMessage(func(ctx context.Context, chatID string, text string) error {
		return r.HandleInbound(ctx, InboundEvent{ChannelID: id, ChatID: chatID, Text: text})
	})
	return nil
}

func (r *Router) Bind(_ context.Context, chat ChatRef, sessionID string, opts BindOptions) error {
	chat = normalizeChat(chat)
	sessionID = strings.TrimSpace(sessionID)
	if chat.ChannelID == "" || chat.ChatID == "" || sessionID == "" {
		return fmt.Errorf("im2: invalid binding")
	}
	r.mu.Lock()
	r.bindings[chat] = binding{sessionID: sessionID, watch: opts.Watch}
	r.mu.Unlock()
	return nil
}

func (r *Router) Unbind(_ context.Context, chat ChatRef) error {
	chat = normalizeChat(chat)
	if chat.ChannelID == "" || chat.ChatID == "" {
		return fmt.Errorf("im2: invalid chat")
	}
	r.mu.Lock()
	delete(r.bindings, chat)
	r.mu.Unlock()
	return nil
}

func (r *Router) HandleInbound(ctx context.Context, event InboundEvent) error {
	chat := normalizeChat(ChatRef{ChannelID: event.ChannelID, ChatID: event.ChatID})
	text := strings.TrimSpace(event.Text)
	if chat.ChannelID == "" || chat.ChatID == "" {
		return fmt.Errorf("im2: inbound chat is invalid")
	}
	if text == "" {
		return nil
	}
	r.mu.RLock()
	b := r.bindings[chat]
	r.mu.RUnlock()
	event.ChannelID = chat.ChannelID
	event.ChatID = chat.ChatID
	event.Text = text
	event.SessionID = b.sessionID
	_ = r.history.Append(ctx, HistoryEvent{SessionID: event.SessionID, Direction: HistoryInbound, Source: &chat, Text: text})
	if r.client == nil {
		return nil
	}
	return r.client.HandleIM2Inbound(ctx, event)
}

func normalizeChat(chat ChatRef) ChatRef {
	return ChatRef{ChannelID: normalize(chat.ChannelID), ChatID: strings.TrimSpace(chat.ChatID)}
}

func normalize(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}
