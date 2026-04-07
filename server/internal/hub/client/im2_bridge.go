package client

import (
	"context"
	"fmt"
	"strings"

	"github.com/swm8023/wheelmaker/internal/im2"
	acp "github.com/swm8023/wheelmaker/internal/protocol"
)

type IM2Router interface {
	Bind(ctx context.Context, chat im2.ChatRef, sessionID string, opts im2.BindOptions) error
	Send(ctx context.Context, target im2.SendTarget, event im2.OutboundEvent) error
	RequestDecision(ctx context.Context, target im2.SendTarget, req im2.DecisionRequest) (im2.DecisionResult, error)
	Run(ctx context.Context) error
}

func (c *Client) SetIM2Router(router IM2Router) {
	c.mu.Lock()
	c.im2Router = router
	for _, sess := range c.sessions {
		sess.im2Router = router
	}
	c.mu.Unlock()
}

func (c *Client) HasIM2Router() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.im2Router != nil
}

func (c *Client) HandleIM2Inbound(ctx context.Context, event im2.InboundEvent) error {
	text := strings.TrimSpace(event.Text)
	if text == "" {
		return nil
	}
	source := im2.ChatRef{ChannelID: strings.TrimSpace(event.ChannelID), ChatID: strings.TrimSpace(event.ChatID)}
	if source.ChannelID == "" || source.ChatID == "" {
		return fmt.Errorf("client im2: invalid source")
	}
	routeKey := im2RouteKey(source)

	cmd, args, isCommand := parseCommand(text)
	if event.SessionID == "" && isCommand && cmd == "/list" {
		body, err := c.formatSessionList("")
		if err != nil {
			body = fmt.Sprintf("List error: %v", err)
		}
		return c.sendIM2Direct(ctx, source, body)
	}

	if strings.TrimSpace(event.SessionID) == "" {
		c.mu.Lock()
		if existing := c.routeMap[routeKey]; existing != "" {
			event.SessionID = existing
		}
		c.mu.Unlock()
	}

	var sess *Session
	switch {
	case strings.TrimSpace(event.SessionID) != "":
		sessionID := strings.TrimSpace(event.SessionID)
		c.mu.Lock()
		c.routeMap[routeKey] = sessionID
		c.mu.Unlock()
		sess = c.resolveSession(routeKey)
	case isCommand && cmd == "/new":
		sess = c.ClientNewSession(routeKey)
		if err := c.bindIM2(ctx, source, sess.ID); err != nil {
			return err
		}
		sess.setIM2Source(source)
		sess.reply(fmt.Sprintf("Created new session: %s", sess.ID))
		return nil
	case isCommand && cmd == "/load":
		loaded, err := c.loadSessionForIM2(ctx, source, routeKey, args)
		if err != nil {
			return c.sendIM2Direct(ctx, source, fmt.Sprintf("Load error: %v", err))
		}
		loaded.setIM2Source(source)
		loaded.reply(fmt.Sprintf("Loaded session: %s", loaded.ID))
		return nil
	default:
		sess = c.ClientNewSession(routeKey)
		if err := c.bindIM2(ctx, source, sess.ID); err != nil {
			return err
		}
	}

	sess.setIM2Source(source)
	if isCommand {
		c.handleCommand(sess, routeKey, cmd, args)
		return nil
	}
	sess.handlePrompt(text)
	return nil
}

func (c *Client) bindIM2(ctx context.Context, source im2.ChatRef, sessionID string) error {
	c.mu.Lock()
	router := c.im2Router
	c.mu.Unlock()
	if router == nil {
		return nil
	}
	return router.Bind(ctx, source, sessionID, im2.BindOptions{})
}

func (c *Client) sendIM2Direct(ctx context.Context, source im2.ChatRef, text string) error {
	c.mu.Lock()
	router := c.im2Router
	c.mu.Unlock()
	if router == nil {
		return nil
	}
	return router.Send(ctx, im2.SendTarget{ChannelID: source.ChannelID, ChatID: source.ChatID}, im2.OutboundEvent{
		Kind:    im2.OutboundSystem,
		Payload: im2.TextPayload{Text: text},
	})
}

func (c *Client) loadSessionForIM2(ctx context.Context, source im2.ChatRef, routeKey, args string) (*Session, error) {
	idxStr := strings.TrimSpace(args)
	if idxStr == "" {
		return nil, fmt.Errorf("Usage: /load <index>  (see /list)")
	}
	idx, err := parsePositiveIndex(idxStr)
	if err != nil {
		return nil, err
	}
	loaded, err := c.ClientLoadSession(routeKey, idx)
	if err != nil {
		return nil, err
	}
	return loaded, c.bindIM2(ctx, source, loaded.ID)
}

func im2RouteKey(source im2.ChatRef) string {
	return "im2:" + strings.ToLower(strings.TrimSpace(source.ChannelID)) + ":" + strings.TrimSpace(source.ChatID)
}

func im2DecisionOptions(options []acp.PermissionOption) []im2.DecisionOption {
	out := make([]im2.DecisionOption, 0, len(options))
	for _, opt := range options {
		out = append(out, im2.DecisionOption{ID: opt.OptionID, Label: opt.Name, Value: opt.Kind})
	}
	return out
}
