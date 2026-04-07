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
	PublishSessionUpdate(ctx context.Context, target im.SendTarget, params acp.SessionUpdateParams) error
	PublishPromptResult(ctx context.Context, target im.SendTarget, result acp.SessionPromptResult) error
	PublishPermissionRequest(ctx context.Context, target im.SendTarget, requestID int64, params acp.PermissionRequestParams) error
	SystemNotify(ctx context.Context, target im.SendTarget, payload im.SystemPayload) error
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

func (c *Client) HandleIMPrompt(ctx context.Context, source im.ChatRef, params acp.SessionPromptParams) error {
	return c.handleIMText(ctx, source, promptTextFromBlocks(params.Prompt))
}

func (c *Client) HandleIMCommand(ctx context.Context, source im.ChatRef, cmd im.Command) error {
	if source.ChannelID == "" || source.ChatID == "" {
		return fmt.Errorf("client im: invalid source")
	}
	return c.handleIMCommand(ctx, source, cmd.Name, cmd.Args)
}

func (c *Client) HandleIMPermissionResponse(_ context.Context, source im.ChatRef, requestID int64, result acp.PermissionResponse) error {
	source = im.ChatRef{ChannelID: strings.TrimSpace(source.ChannelID), ChatID: strings.TrimSpace(source.ChatID)}
	if source.ChannelID == "" || source.ChatID == "" {
		return fmt.Errorf("client im: invalid source")
	}
	routeKey := imRouteKey(source)

	c.mu.Lock()
	sessID := c.routeMap[routeKey]
	sess := c.sessions[sessID]
	all := make([]*Session, 0, len(c.sessions))
	for _, s := range c.sessions {
		all = append(all, s)
	}
	c.mu.Unlock()

	if sess != nil && sess.permRouter != nil && sess.permRouter.resolve(requestID, result) {
		return nil
	}
	for _, candidate := range all {
		if candidate == nil || candidate == sess || candidate.permRouter == nil {
			continue
		}
		if candidate.permRouter.resolve(requestID, result) {
			return nil
		}
	}
	return nil
}

// HandleIMInbound preserves the previous text-based entrypoint for tests and
// any legacy callers while routing into the new prompt/command handlers.
func (c *Client) HandleIMInbound(ctx context.Context, event im.InboundEvent) error {
	source := im.ChatRef{ChannelID: strings.TrimSpace(event.ChannelID), ChatID: strings.TrimSpace(event.ChatID)}
	if source.ChannelID == "" || source.ChatID == "" {
		return fmt.Errorf("client im: invalid source")
	}
	if cmd, ok := im.ParseCommand(event.Text); ok {
		return c.HandleIMCommand(ctx, source, cmd)
	}
	return c.HandleIMPrompt(ctx, source, acp.SessionPromptParams{
		Prompt: []acp.ContentBlock{{Type: acp.ContentBlockTypeText, Text: event.Text}},
	})
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
	return router.SystemNotify(ctx, im.SendTarget{ChannelID: source.ChannelID, ChatID: source.ChatID}, im.SystemPayload{
		Kind: "message",
		Body: text,
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

func promptTextFromBlocks(blocks []acp.ContentBlock) string {
	for _, block := range blocks {
		if block.Type == acp.ContentBlockTypeText {
			return block.Text
		}
	}
	return ""
}

func (c *Client) handleIMText(ctx context.Context, source im.ChatRef, text string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	routeKey := imRouteKey(source)

	if cmd, ok := im.ParseCommand(text); ok {
		return c.handleIMCommand(ctx, source, cmd.Name, cmd.Args)
	}

	sess := c.resolveOrCreateIMSession(ctx, source, routeKey)
	if sess == nil {
		return nil
	}
	sess.setIMSource(source)
	sess.handlePrompt(text)
	return nil
}

func (c *Client) handleIMCommand(ctx context.Context, source im.ChatRef, cmd, args string) error {
	routeKey := imRouteKey(source)

	if cmd == "/list" {
		body, err := c.formatSessionList("")
		if err != nil {
			body = fmt.Sprintf("List error: %v", err)
		}
		return c.sendIMDirect(ctx, source, body)
	}
	if cmd == "/new" {
		sess := c.ClientNewSession(routeKey)
		if err := c.bindIM(ctx, source, sess.ID); err != nil {
			return err
		}
		sess.setIMSource(source)
		sess.reply(fmt.Sprintf("Created new session: %s", sess.ID))
		return nil
	}
	if cmd == "/load" {
		loaded, err := c.loadSessionForIM(ctx, source, routeKey, args)
		if err != nil {
			return c.sendIMDirect(ctx, source, fmt.Sprintf("Load error: %v", err))
		}
		loaded.setIMSource(source)
		loaded.reply(fmt.Sprintf("Loaded session: %s", loaded.ID))
		return nil
	}

	sess := c.resolveOrCreateIMSession(ctx, source, routeKey)
	if sess == nil {
		return nil
	}
	sess.setIMSource(source)
	c.handleCommand(sess, routeKey, cmd, args)
	return nil
}

func (c *Client) resolveOrCreateIMSession(ctx context.Context, source im.ChatRef, routeKey string) *Session {
	c.mu.Lock()
	sessID := c.routeMap[routeKey]
	c.mu.Unlock()
	if sessID != "" {
		return c.resolveSession(routeKey)
	}
	sess := c.ClientNewSession(routeKey)
	if err := c.bindIM(ctx, source, sess.ID); err != nil {
		return nil
	}
	return sess
}
