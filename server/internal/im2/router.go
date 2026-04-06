package im2

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// Router is the single IM 2.0 ingress/egress boundary for one client.
// One client owns one Router; router maps many IMActiveChats to many client sessions.
type Router struct {
	projectName        string
	state              State
	newClientSessionID func(context.Context) (string, error)

	mu             sync.RWMutex
	inboundHandler InboundHandler
	publishers     map[string]Publisher // key: imType
}

// NewRouter creates an IM router for one client/project.
func NewRouter(
	projectName string,
	state State,
	newClientSessionID func(context.Context) (string, error),
	handler InboundHandler,
) (*Router, error) {
	pn := strings.TrimSpace(projectName)
	if pn == "" {
		return nil, fmt.Errorf("im2 router: empty project name")
	}
	if state == nil {
		return nil, fmt.Errorf("im2 router: nil state")
	}
	if newClientSessionID == nil {
		return nil, fmt.Errorf("im2 router: nil newClientSessionID callback")
	}
	return &Router{
		projectName:        pn,
		state:              state,
		newClientSessionID: newClientSessionID,
		inboundHandler:     handler,
		publishers:         map[string]Publisher{},
	}, nil
}

// SetInboundHandler updates inbound event callback.
func (r *Router) SetInboundHandler(handler InboundHandler) {
	r.mu.Lock()
	r.inboundHandler = handler
	r.mu.Unlock()
}

// RegisterPublisher binds one imType to one outbound publisher.
func (r *Router) RegisterPublisher(imType string, publisher Publisher) error {
	key := strings.ToLower(strings.TrimSpace(imType))
	if key == "" {
		return fmt.Errorf("im2 router: empty imType")
	}
	if publisher == nil {
		return fmt.Errorf("im2 router: nil publisher")
	}
	r.mu.Lock()
	r.publishers[key] = publisher
	r.mu.Unlock()
	return nil
}

// UnregisterPublisher removes publisher binding for imType.
func (r *Router) UnregisterPublisher(imType string) {
	key := strings.ToLower(strings.TrimSpace(imType))
	if key == "" {
		return
	}
	r.mu.Lock()
	delete(r.publishers, key)
	r.mu.Unlock()
}

// HandleInbound normalizes one inbound IM event, resolves/creates clientSessionId,
// persists routing binding, then dispatches to Client via inbound handler.
func (r *Router) HandleInbound(ctx context.Context, event InboundEvent) error {
	ev, err := r.normalizeInbound(ctx, event)
	if err != nil {
		return err
	}

	r.mu.RLock()
	handler := r.inboundHandler
	r.mu.RUnlock()
	if handler == nil {
		return nil
	}
	return handler(ctx, ev)
}

// Publish sends one client event to IM.
// If TargetActiveChatID is empty it broadcasts to all online chats in clientSessionId.
func (r *Router) Publish(ctx context.Context, event OutboundEvent) error {
	clientSessionID := strings.TrimSpace(event.ClientSessionID)
	if clientSessionID == "" {
		return fmt.Errorf("im2 router: empty clientSessionId")
	}

	target := strings.TrimSpace(event.TargetActiveChatID)
	if target != "" {
		return r.publishToActiveChat(ctx, target, event)
	}

	chats, err := r.state.ListSessionActiveChats(ctx, clientSessionID, true)
	if err != nil {
		return fmt.Errorf("im2 router: list active chats for %q: %w", clientSessionID, err)
	}
	if len(chats) == 0 {
		return nil
	}

	var firstErr error
	for _, chat := range chats {
		if err := r.publishToActiveChat(ctx, chat.ActiveChatID, event); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// MarkActiveChatOffline updates runtime state for disconnected chat endpoints.
func (r *Router) MarkActiveChatOffline(ctx context.Context, activeChatID string, critical bool) error {
	return r.state.SetActiveChatOnline(ctx, activeChatID, false, critical)
}

func (r *Router) normalizeInbound(ctx context.Context, event InboundEvent) (InboundEvent, error) {
	ev := event
	if ev.ReceivedAt.IsZero() {
		ev.ReceivedAt = time.Now().UTC()
	}

	activeChatID := strings.TrimSpace(ev.ActiveChatID)
	imType := strings.ToLower(strings.TrimSpace(ev.IMType))
	chatID := strings.TrimSpace(ev.ChatID)

	if activeChatID != "" {
		parsedType, parsedChatID, ok := ParseActiveChatID(activeChatID)
		if !ok {
			return InboundEvent{}, fmt.Errorf("im2 router: invalid activeChatID %q", activeChatID)
		}
		if imType == "" {
			imType = parsedType
		}
		if chatID == "" {
			chatID = parsedChatID
		}
	} else {
		id, err := BuildActiveChatID(imType, chatID)
		if err != nil {
			return InboundEvent{}, fmt.Errorf("im2 router: build active chat id: %w", err)
		}
		activeChatID = id
	}

	clientSessionID, ok, err := r.state.ResolveClientSessionID(ctx, activeChatID)
	if err != nil {
		return InboundEvent{}, fmt.Errorf("im2 router: resolve clientSessionId: %w", err)
	}
	if !ok {
		newID, err := r.newClientSessionID(ctx)
		if err != nil {
			return InboundEvent{}, fmt.Errorf("im2 router: create clientSessionId: %w", err)
		}
		clientSessionID = strings.TrimSpace(newID)
		if clientSessionID == "" {
			return InboundEvent{}, fmt.Errorf("im2 router: empty clientSessionId from generator")
		}
		if err := r.state.BindActiveChat(ctx, activeChatID, imType, chatID, clientSessionID, true); err != nil {
			return InboundEvent{}, fmt.Errorf("im2 router: bind new active chat: %w", err)
		}
	} else {
		if err := r.state.BindActiveChat(ctx, activeChatID, imType, chatID, clientSessionID, false); err != nil {
			return InboundEvent{}, fmt.Errorf("im2 router: touch active chat binding: %w", err)
		}
	}

	ev.ActiveChatID = activeChatID
	ev.IMType = imType
	ev.ChatID = chatID
	ev.ClientSessionID = clientSessionID
	return ev, nil
}

func (r *Router) publishToActiveChat(ctx context.Context, activeChatID string, event OutboundEvent) error {
	imType, chatID, ok := ParseActiveChatID(activeChatID)
	if !ok {
		return fmt.Errorf("im2 router: invalid activeChatID %q", activeChatID)
	}

	r.mu.RLock()
	publisher := r.publishers[imType]
	r.mu.RUnlock()
	if publisher == nil {
		return fmt.Errorf("im2 router: publisher not registered for imType=%q", imType)
	}

	ev := event
	ev.TargetActiveChatID = activeChatID
	if err := publisher.PublishToChat(ctx, chatID, ev); err != nil {
		return fmt.Errorf("im2 router: publish to %s:%s failed: %w", imType, chatID, err)
	}
	return nil
}
