package im

import (
	"context"
	"errors"
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
		return fmt.Errorf("im: channel is nil")
	}
	id := normalize(ch.ID())
	if id == "" {
		return fmt.Errorf("im: channel id is empty")
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
		return fmt.Errorf("im: invalid binding")
	}
	r.mu.Lock()
	r.bindings[chat] = binding{sessionID: sessionID, watch: opts.Watch}
	r.mu.Unlock()
	return nil
}

func (r *Router) Unbind(_ context.Context, chat ChatRef) error {
	chat = normalizeChat(chat)
	if chat.ChannelID == "" || chat.ChatID == "" {
		return fmt.Errorf("im: invalid chat")
	}
	r.mu.Lock()
	delete(r.bindings, chat)
	r.mu.Unlock()
	return nil
}

func (r *Router) HandleInbound(ctx context.Context, event InboundEvent) error {
	chat := normalizeChat(ChatRef{ChannelID: event.ChannelID, ChatID: event.ChatID})
	if chat.ChannelID == "" || chat.ChatID == "" {
		return fmt.Errorf("im: inbound chat is invalid")
	}
	if strings.TrimSpace(event.Text) == "" {
		return nil
	}
	r.mu.RLock()
	b := r.bindings[chat]
	r.mu.RUnlock()
	event.ChannelID = chat.ChannelID
	event.ChatID = chat.ChatID
	event.SessionID = b.sessionID
	_ = r.history.Append(ctx, HistoryEvent{SessionID: event.SessionID, Direction: HistoryInbound, Source: &chat, Text: event.Text})
	if r.client == nil {
		return nil
	}
	return r.client.HandleIMInbound(ctx, event)
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

func (r *Router) RequestDecision(ctx context.Context, target SendTarget, req DecisionRequest) (DecisionResult, error) {
	sessionID := strings.TrimSpace(target.SessionID)
	if sessionID == "" {
		return DecisionResult{Outcome: "invalid"}, fmt.Errorf("im: decision session is empty")
	}
	if target.Source == nil {
		return DecisionResult{Outcome: "invalid"}, fmt.Errorf("im: decision source is empty")
	}
	source := normalizeChat(*target.Source)
	if source.ChannelID == "" || source.ChatID == "" {
		return DecisionResult{Outcome: "invalid"}, fmt.Errorf("im: decision source is invalid")
	}
	ch, err := r.channel(source.ChannelID)
	if err != nil {
		return DecisionResult{Outcome: "invalid"}, err
	}
	req.SessionID = sessionID
	res, err := ch.RequestDecision(ctx, source.ChatID, req)
	_ = r.history.Append(ctx, HistoryEvent{
		SessionID: sessionID,
		Direction: HistoryOutbound,
		Source:    &source,
		Targets:   []ChatRef{source},
		Kind:      OutboundSystem,
		Payload:   req,
	})
	return res, err
}

func (r *Router) Run(ctx context.Context) error {
	r.mu.RLock()
	channels := make([]Channel, 0, len(r.channels))
	for _, ch := range r.channels {
		channels = append(channels, ch)
	}
	r.mu.RUnlock()
	if len(channels) == 0 {
		return fmt.Errorf("im: no channels registered")
	}

	var wg sync.WaitGroup
	errCh := make(chan error, len(channels))
	for _, ch := range channels {
		ch := ch
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := ch.Run(ctx); err != nil && !errors.Is(err, context.Canceled) && ctx.Err() == nil {
				errCh <- err
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *Router) recipients(target SendTarget) ([]ChatRef, error) {
	sessionID := strings.TrimSpace(target.SessionID)
	if sessionID == "" {
		chat := normalizeChat(ChatRef{ChannelID: target.ChannelID, ChatID: target.ChatID})
		if chat.ChannelID == "" || chat.ChatID == "" {
			return nil, fmt.Errorf("im: direct send target is invalid")
		}
		return []ChatRef{chat}, nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if target.Source != nil {
		source := normalizeChat(*target.Source)
		if source.ChannelID == "" || source.ChatID == "" {
			return nil, fmt.Errorf("im: reply source is invalid")
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
		return nil, fmt.Errorf("im: no chats bound to session %q", sessionID)
	}
	return out, nil
}

func (r *Router) channel(channelID string) (Channel, error) {
	id := normalize(channelID)
	r.mu.RLock()
	ch := r.channels[id]
	r.mu.RUnlock()
	if ch == nil {
		return nil, fmt.Errorf("im: channel %q is not registered", id)
	}
	return ch, nil
}

func normalizeChat(chat ChatRef) ChatRef {
	return ChatRef{ChannelID: normalize(chat.ChannelID), ChatID: strings.TrimSpace(chat.ChatID)}
}

func normalize(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}
