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

func (r *Router) Send(ctx context.Context, target SendTarget, event OutboundEvent) error {
	recipients, err := r.recipients(target)
	if err != nil {
		return err
	}
	var firstErr error
	for _, chat := range recipients {
		ch, err := r.channel(chat.ChannelID)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if err := ch.Send(ctx, chat.ChatID, event); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	_ = r.history.Append(ctx, HistoryEvent{
		SessionID: strings.TrimSpace(target.SessionID),
		Direction: HistoryOutbound,
		Source:    target.Source,
		Targets:   recipients,
		Kind:      event.Kind,
		Payload:   event.Payload,
	})
	return firstErr
}

func (r *Router) recipients(target SendTarget) ([]ChatRef, error) {
	sessionID := strings.TrimSpace(target.SessionID)
	if sessionID == "" {
		chat := normalizeChat(ChatRef{ChannelID: target.ChannelID, ChatID: target.ChatID})
		if chat.ChannelID == "" || chat.ChatID == "" {
			return nil, fmt.Errorf("im2: direct send target is invalid")
		}
		return []ChatRef{chat}, nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if target.Source != nil {
		source := normalizeChat(*target.Source)
		if source.ChannelID == "" || source.ChatID == "" {
			return nil, fmt.Errorf("im2: reply source is invalid")
		}
		out := []ChatRef{source}
		for chat, b := range r.bindings {
			if b.sessionID == sessionID && b.watch && chat != source {
				out = append(out, chat)
			}
		}
		return out, nil
	}
	var out []ChatRef
	for chat, b := range r.bindings {
		if b.sessionID == sessionID {
			out = append(out, chat)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("im2: no chats bound to session %q", sessionID)
	}
	return out, nil
}

func (r *Router) channel(channelID string) (Channel, error) {
	id := normalize(channelID)
	r.mu.RLock()
	ch := r.channels[id]
	r.mu.RUnlock()
	if ch == nil {
		return nil, fmt.Errorf("im2: channel %q is not registered", id)
	}
	return ch, nil
}

func normalizeChat(chat ChatRef) ChatRef {
	return ChatRef{ChannelID: normalize(chat.ChannelID), ChatID: strings.TrimSpace(chat.ChatID)}
}

func normalize(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}
