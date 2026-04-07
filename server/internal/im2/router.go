package im2

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// Router is the single IM 2.0 ingress/egress boundary for one client.
// One client owns one Router; router maps many routeKeys to many client sessions.
type Router struct {
	projectName        string
	state              State
	newClientSessionID func(context.Context) (string, error)

	mu             sync.RWMutex
	inboundHandler InboundHandler
	channels       map[string]Channel // key: imType
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
		channels:           map[string]Channel{},
	}, nil
}

// SetInboundHandler updates inbound event callback.
func (r *Router) SetInboundHandler(handler InboundHandler) {
	r.mu.Lock()
	r.inboundHandler = handler
	r.mu.Unlock()
}

// RegisterChannel binds one imType to one outbound channel.
func (r *Router) RegisterChannel(imType string, ch Channel) error {
	key := strings.ToLower(strings.TrimSpace(imType))
	if key == "" {
		return fmt.Errorf("im2 router: empty imType")
	}
	if ch == nil {
		return fmt.Errorf("im2 router: nil channel")
	}
	r.mu.Lock()
	r.channels[key] = ch
	r.mu.Unlock()
	return nil
}

// UnregisterChannel removes channel binding for imType.
func (r *Router) UnregisterChannel(imType string) {
	key := strings.ToLower(strings.TrimSpace(imType))
	if key == "" {
		return
	}
	r.mu.Lock()
	delete(r.channels, key)
	r.mu.Unlock()
}

// HandleInbound normalizes one inbound IM event, resolves/creates clientSessionId,
// persists route binding, then dispatches to Client via inbound handler.
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
// If TargetRouteKey is empty it broadcasts to all online chats in clientSessionId.
func (r *Router) Publish(ctx context.Context, event OutboundEvent) error {
	clientSessionID := strings.TrimSpace(event.ClientSessionID)
	if clientSessionID == "" {
		return fmt.Errorf("im2 router: empty clientSessionId")
	}

	target := strings.TrimSpace(event.TargetRouteKey)
	if target != "" {
		return r.publishToRouteKey(ctx, target, event)
	}

	bindings, err := r.state.ListSessionRouteBindings(ctx, clientSessionID, true)
	if err != nil {
		return fmt.Errorf("im2 router: list route bindings for %q: %w", clientSessionID, err)
	}
	if len(bindings) == 0 {
		return nil
	}

	var firstErr error
	for _, binding := range bindings {
		if err := r.publishToRouteKey(ctx, binding.RouteKey, event); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// RebindRouteKey updates one routeKey to a new client session binding.
func (r *Router) RebindRouteKey(ctx context.Context, routeKey, clientSessionID string) error {
	return r.state.RebindRouteKey(ctx, strings.TrimSpace(routeKey), strings.TrimSpace(clientSessionID))
}

// MarkRouteKeyOffline updates runtime state for disconnected chat endpoints.
func (r *Router) MarkRouteKeyOffline(ctx context.Context, routeKey string, critical bool) error {
	return r.state.SetRouteKeyOnline(ctx, routeKey, false, critical)
}

func (r *Router) normalizeInbound(ctx context.Context, event InboundEvent) (InboundEvent, error) {
	ev := event
	if ev.ReceivedAt.IsZero() {
		ev.ReceivedAt = time.Now().UTC()
	}

	routeKey := strings.TrimSpace(ev.RouteKey)
	imType := strings.ToLower(strings.TrimSpace(ev.IMType))
	chatID := strings.TrimSpace(ev.ChatID)

	if routeKey != "" {
		parsedType, parsedChatID, ok := ParseRouteKey(routeKey)
		if !ok {
			return InboundEvent{}, fmt.Errorf("im2 router: invalid routeKey %q", routeKey)
		}
		if imType == "" {
			imType = parsedType
		}
		if chatID == "" {
			chatID = parsedChatID
		}
	} else {
		id, err := BuildRouteKey(imType, chatID)
		if err != nil {
			return InboundEvent{}, fmt.Errorf("im2 router: build routeKey: %w", err)
		}
		routeKey = id
	}

	clientSessionID, ok, err := r.state.ResolveClientSessionID(ctx, routeKey)
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
		if err := r.state.BindRouteKey(ctx, routeKey, imType, chatID, clientSessionID, true); err != nil {
			return InboundEvent{}, fmt.Errorf("im2 router: bind new routeKey: %w", err)
		}
	} else {
		if err := r.state.BindRouteKey(ctx, routeKey, imType, chatID, clientSessionID, false); err != nil {
			return InboundEvent{}, fmt.Errorf("im2 router: touch route binding: %w", err)
		}
	}

	ev.RouteKey = routeKey
	ev.IMType = imType
	ev.ChatID = chatID
	ev.ClientSessionID = clientSessionID
	return ev, nil
}

func (r *Router) publishToRouteKey(ctx context.Context, routeKey string, event OutboundEvent) error {
	imType, chatID, ok := ParseRouteKey(routeKey)
	if !ok {
		return fmt.Errorf("im2 router: invalid routeKey %q", routeKey)
	}

	r.mu.RLock()
	ch := r.channels[imType]
	r.mu.RUnlock()
	if ch == nil {
		return fmt.Errorf("im2 router: channel not registered for imType=%q", imType)
	}

	ev := event
	ev.TargetRouteKey = routeKey
	if err := ch.PublishToChat(ctx, chatID, ev); err != nil {
		return fmt.Errorf("im2 router: publish to %s:%s failed: %w", imType, chatID, err)
	}
	return nil
}
