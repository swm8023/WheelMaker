package client

import (
	"context"
	"fmt"
	"strings"

	"github.com/swm8023/wheelmaker/internal/im"
	acp "github.com/swm8023/wheelmaker/internal/protocol"
)

type IMRouter interface {
	Bind(ctx context.Context, chat im.ChatRef, sessionID string, opts im.BindOptions) error
	Send(ctx context.Context, target im.SendTarget, event im.OutboundEvent) error
	RequestDecision(ctx context.Context, target im.SendTarget, req im.DecisionRequest) (im.DecisionResult, error)
	Run(ctx context.Context) error
}

func (c *Client) SetIMRouter(router IMRouter) {
	c.mu.Lock()
	c.imRouter = router
	for _, sess := range c.sessions {
		sess.imRouter = router
	}
	c.mu.Unlock()
}

func (c *Client) HasIMRouter() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.imRouter != nil
}

func (c *Client) HandleIMInbound(ctx context.Context, event im.InboundEvent) error {
	text := strings.TrimSpace(event.Text)
	if text == "" {
		return nil
	}
	source := im.ChatRef{ChannelID: strings.TrimSpace(event.ChannelID), ChatID: strings.TrimSpace(event.ChatID)}
	if source.ChannelID == "" || source.ChatID == "" {
		return fmt.Errorf("client im: invalid source")
	}
	routeKey := imRouteKey(source)

	cmd, args, isCommand := parseCommand(text)
	if event.SessionID == "" && isCommand && cmd == "/list" {
		body, err := c.formatSessionList("")
		if err != nil {
			body = fmt.Sprintf("List error: %v", err)
		}
		return c.sendIMDirect(ctx, source, body)
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
		if err := c.bindIM(ctx, source, sess.ID); err != nil {
			return err
		}
		sess.setIMSource(source)
		sess.reply(fmt.Sprintf("Created new session: %s", sess.ID))
		return nil
	case isCommand && cmd == "/load":
		loaded, err := c.loadSessionForIM(ctx, source, routeKey, args)
		if err != nil {
			return c.sendIMDirect(ctx, source, fmt.Sprintf("Load error: %v", err))
		}
		loaded.setIMSource(source)
		loaded.reply(fmt.Sprintf("Loaded session: %s", loaded.ID))
		return nil
	default:
		sess = c.ClientNewSession(routeKey)
		if err := c.bindIM(ctx, source, sess.ID); err != nil {
			return err
		}
	}

	sess.setIMSource(source)
	if isCommand {
		c.handleCommand(sess, routeKey, cmd, args)
		return nil
	}
	sess.handlePrompt(text)
	return nil
}

func (c *Client) bindIM(ctx context.Context, source im.ChatRef, sessionID string) error {
	c.mu.Lock()
	router := c.imRouter
	c.mu.Unlock()
	if router == nil {
		return nil
	}
	return router.Bind(ctx, source, sessionID, im.BindOptions{})
}

func (c *Client) sendIMDirect(ctx context.Context, source im.ChatRef, text string) error {
	c.mu.Lock()
	router := c.imRouter
	c.mu.Unlock()
	if router == nil {
		return nil
	}
	return router.Send(ctx, im.SendTarget{ChannelID: source.ChannelID, ChatID: source.ChatID}, im.OutboundEvent{
		Kind:    im.OutboundSystem,
		Payload: im.TextPayload{Text: text},
	})
}

func (c *Client) loadSessionForIM(ctx context.Context, source im.ChatRef, routeKey, args string) (*Session, error) {
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
	return loaded, c.bindIM(ctx, source, loaded.ID)
}

func imRouteKey(source im.ChatRef) string {
	return "im:" + strings.ToLower(strings.TrimSpace(source.ChannelID)) + ":" + strings.TrimSpace(source.ChatID)
}

func imDecisionOptions(options []acp.PermissionOption) []im.DecisionOption {
	out := make([]im.DecisionOption, 0, len(options))
	for _, opt := range options {
		out = append(out, im.DecisionOption{ID: opt.OptionID, Label: opt.Name, Value: opt.Kind})
	}
	return out
}
