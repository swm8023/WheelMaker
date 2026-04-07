package feishu

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/swm8023/wheelmaker/internal/im"
	acp "github.com/swm8023/wheelmaker/internal/protocol"
)

type transport interface {
	OnMessage(MessageHandler)
	OnCardAction(func(CardActionEvent))
	Send(chatID, text string, kind TextKind) error
	SendCard(chatID, messageID string, card Card) error
	SendReaction(messageID, emoji string) error
	MarkDone(chatID string) error
	Run(ctx context.Context) error
}

type PendingPermission struct {
	RequestID  int64
	ChatID     string
	SessionID  string
	ToolCallID string
	Options    []acp.PermissionOption
	CreatedAt  time.Time
}

type Channel struct {
	inner transport

	mu                 sync.Mutex
	blockedUpdates     map[string]struct{}
	onPrompt           func(context.Context, im.ChatRef, acp.SessionPromptParams) error
	onCommand          func(context.Context, im.ChatRef, im.Command) error
	onPermissionResult func(context.Context, im.ChatRef, int64, acp.PermissionResponse) error
	pendingByRequestID map[int64]PendingPermission
	pendingByChat      map[string]PendingPermission
	closed             map[int64]time.Time
}

func New(cfg Config) *Channel {
	return newWithTransportConfig(newTransport(cfg), cfg)
}

func newWithTransport(inner transport) *Channel {
	return newWithTransportConfig(inner, Config{})
}

func newWithTransportConfig(inner transport, cfg Config) *Channel {
	c := &Channel{
		inner:              inner,
		blockedUpdates:     buildBlockedUpdates(cfg.BlockedUpdates),
		pendingByRequestID: map[int64]PendingPermission{},
		pendingByChat:      map[string]PendingPermission{},
		closed:             map[int64]time.Time{},
	}
	inner.OnMessage(c.handleMessage)
	inner.OnCardAction(c.handleCardAction)
	return c
}

func (c *Channel) ID() string { return "feishu" }

func (c *Channel) OnPrompt(handler func(context.Context, im.ChatRef, acp.SessionPromptParams) error) {
	c.mu.Lock()
	c.onPrompt = handler
	c.mu.Unlock()
}

func (c *Channel) OnCommand(handler func(context.Context, im.ChatRef, im.Command) error) {
	c.mu.Lock()
	c.onCommand = handler
	c.mu.Unlock()
}

func (c *Channel) OnPermissionResponse(handler func(context.Context, im.ChatRef, int64, acp.PermissionResponse) error) {
	c.mu.Lock()
	c.onPermissionResult = handler
	c.mu.Unlock()
}

func (c *Channel) PublishSessionUpdate(ctx context.Context, target im.SendTarget, params acp.SessionUpdateParams) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if c.isBlocked("system") {
		return nil
	}
	chatID := resolveChatID(target)
	if chatID == "" {
		return fmt.Errorf("im feishu: chat is empty")
	}
	return c.renderSessionUpdate(chatID, params)
}

func (c *Channel) PublishPromptResult(ctx context.Context, target im.SendTarget, result acp.SessionPromptResult) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	chatID := resolveChatID(target)
	if chatID == "" {
		return fmt.Errorf("im feishu: chat is empty")
	}
	return c.renderPromptResult(chatID, result)
}

func (c *Channel) PublishPermissionRequest(ctx context.Context, target im.SendTarget, requestID int64, params acp.PermissionRequestParams) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	chatID := resolveChatID(target)
	if chatID == "" {
		return fmt.Errorf("im feishu: chat is empty")
	}
	pending := PendingPermission{
		RequestID:  requestID,
		ChatID:     chatID,
		SessionID:  strings.TrimSpace(params.SessionID),
		ToolCallID: strings.TrimSpace(params.ToolCall.ToolCallID),
		Options:    append([]acp.PermissionOption(nil), params.Options...),
		CreatedAt:  time.Now(),
	}
	c.mu.Lock()
	c.pendingByRequestID[requestID] = pending
	c.pendingByChat[chatID] = pending
	c.mu.Unlock()

	if err := c.renderPermissionRequest(chatID, requestID, params); err != nil {
		c.clearPending(chatID, requestID)
		return err
	}
	return nil
}

func (c *Channel) SystemNotify(ctx context.Context, target im.SendTarget, payload im.SystemPayload) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	chatID := resolveChatID(target)
	if chatID == "" {
		return fmt.Errorf("im feishu: chat is empty")
	}
	text := renderSystemText(payload)
	if text == "" {
		return nil
	}
	return c.inner.Send(chatID, text, TextSystem)
}

func (c *Channel) Run(ctx context.Context) error {
	return c.inner.Run(ctx)
}

func (c *Channel) renderSessionUpdate(chatID string, params acp.SessionUpdateParams) error {
	if c.isBlockedSessionUpdate(params.Update.SessionUpdate) {
		return nil
	}
	switch params.Update.SessionUpdate {
	case acp.SessionUpdateAgentMessageChunk:
		text := extractTextContent(params.Update.Content)
		if text == "" {
			return nil
		}
		return c.inner.Send(chatID, text, TextNormal)
	case acp.SessionUpdateAgentThoughtChunk:
		text := extractTextContent(params.Update.Content)
		if text == "" {
			return nil
		}
		return c.inner.Send(chatID, text, TextThought)
	case acp.SessionUpdateToolCall, acp.SessionUpdateToolCallUpdate:
		if upd, ok := parseToolCallUpdate(params.Update); ok {
			return c.inner.SendCard(chatID, "", ToolCallCard{Update: upd})
		}
	case acp.SessionUpdatePlan:
		if msg := renderUpdateSummary("Plan update", params.Update); msg != "" {
			return c.inner.Send(chatID, msg, TextNormal)
		}
	case acp.SessionUpdateConfigOptionUpdate:
		if msg := renderUpdateSummary("Config updated", params.Update); msg != "" {
			return c.inner.Send(chatID, msg, TextNormal)
		}
	case acp.SessionUpdateAvailableCommandsUpdate:
		if msg := renderUpdateSummary("Commands updated", params.Update); msg != "" {
			return c.inner.Send(chatID, msg, TextNormal)
		}
	case acp.SessionUpdateSessionInfoUpdate, acp.SessionUpdateCurrentModeUpdate:
		if msg := renderUpdateSummary("Session updated", params.Update); msg != "" {
			return c.inner.Send(chatID, msg, TextNormal)
		}
	}
	return nil
}

func (c *Channel) renderPromptResult(chatID string, result acp.SessionPromptResult) error {
	if c.isBlocked("prompt_result") {
		return nil
	}
	stopReason := strings.TrimSpace(result.StopReason)
	if stopReason == "" || stopReason == acp.StopReasonEndTurn {
		return c.inner.MarkDone(chatID)
	}
	return c.inner.Send(chatID, renderPromptResultText(result), TextSystem)
}

func (c *Channel) renderPermissionRequest(chatID string, requestID int64, params acp.PermissionRequestParams) error {
	return c.inner.SendCard(chatID, "", buildPermissionCard(chatID, requestID, params))
}

func (c *Channel) handleMessage(m Message) {
	if c.resolvePermissionText(m) {
		return
	}
	text := strings.TrimSpace(m.Text)
	if text == "" {
		return
	}
	source := im.ChatRef{ChannelID: c.ID(), ChatID: strings.TrimSpace(m.ChatID)}
	if source.ChatID == "" {
		return
	}
	if cmd, ok := im.ParseCommand(text); ok {
		c.mu.Lock()
		handler := c.onCommand
		c.mu.Unlock()
		if handler != nil {
			_ = handler(context.Background(), source, cmd)
		}
		return
	}

	c.mu.Lock()
	handler := c.onPrompt
	c.mu.Unlock()
	if handler != nil {
		_ = handler(context.Background(), source, acp.SessionPromptParams{
			Prompt: []acp.ContentBlock{{Type: acp.ContentBlockTypeText, Text: text}},
		})
	}
}

func (c *Channel) handleCardAction(evt CardActionEvent) {
	if strings.TrimSpace(evt.Value["kind"]) != "permission" {
		return
	}
	requestID, err := strconv.ParseInt(strings.TrimSpace(evt.Value["request_id"]), 10, 64)
	if err != nil || requestID <= 0 {
		return
	}

	chatID := strings.TrimSpace(evt.ChatID)
	if chatID == "" {
		chatID = strings.TrimSpace(evt.Value["chat_id"])
	}
	pending, ok := c.takePending(chatID, requestID)
	if !ok {
		return
	}

	source := im.ChatRef{ChannelID: c.ID(), ChatID: pending.ChatID}
	result := acp.PermissionResponse{
		Outcome: acp.PermissionResult{
			Outcome:  "selected",
			OptionID: strings.TrimSpace(evt.Value["option_id"]),
		},
	}
	if result.Outcome.OptionID == "" {
		result.Outcome.Outcome = "cancelled"
	}

	c.mu.Lock()
	handler := c.onPermissionResult
	c.mu.Unlock()
	if handler != nil {
		_ = handler(context.Background(), source, requestID, result)
	}
}

func (c *Channel) resolvePermissionText(m Message) bool {
	chatID := strings.TrimSpace(m.ChatID)
	if chatID == "" {
		return false
	}

	c.mu.Lock()
	pending, ok := c.pendingByChat[chatID]
	if !ok {
		c.mu.Unlock()
		return false
	}
	delete(c.pendingByChat, chatID)
	delete(c.pendingByRequestID, pending.RequestID)
	c.markClosedLocked(pending.RequestID)
	c.mu.Unlock()

	c.mu.Lock()
	handler := c.onPermissionResult
	c.mu.Unlock()
	if handler != nil {
		_ = handler(context.Background(), im.ChatRef{ChannelID: c.ID(), ChatID: chatID}, pending.RequestID, parsePermissionReply(strings.TrimSpace(m.Text), pending.Options))
	}
	return true
}

func (c *Channel) clearPending(chatID string, requestID int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if pending, ok := c.pendingByChat[chatID]; ok && pending.RequestID == requestID {
		delete(c.pendingByChat, chatID)
	}
	delete(c.pendingByRequestID, requestID)
	c.markClosedLocked(requestID)
}

func (c *Channel) takePending(chatID string, requestID int64) (PendingPermission, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	pending, ok := c.pendingByRequestID[requestID]
	if !ok {
		return PendingPermission{}, false
	}
	delete(c.pendingByRequestID, requestID)
	if current, ok := c.pendingByChat[pending.ChatID]; ok && current.RequestID == requestID {
		delete(c.pendingByChat, pending.ChatID)
	}
	if chatID != "" {
		if current, ok := c.pendingByChat[chatID]; ok && current.RequestID == requestID {
			delete(c.pendingByChat, chatID)
		}
	}
	c.markClosedLocked(requestID)
	return pending, true
}

func (c *Channel) markClosedLocked(requestID int64) {
	c.closed[requestID] = time.Now()
}

func resolveChatID(target im.SendTarget) string {
	if target.Source != nil && strings.TrimSpace(target.Source.ChatID) != "" {
		return strings.TrimSpace(target.Source.ChatID)
	}
	return strings.TrimSpace(target.ChatID)
}

func buildBlockedUpdates(values []string) map[string]struct{} {
	blocked := make(map[string]struct{}, len(values))
	for _, value := range values {
		if key := canonicalBlockedUpdate(value); key != "" {
			blocked[key] = struct{}{}
		}
	}
	return blocked
}

func (c *Channel) isBlockedSessionUpdate(updateType string) bool {
	return c.isBlocked(canonicalBlockedUpdate(updateType))
}

func (c *Channel) isBlocked(key string) bool {
	if key == "" {
		return false
	}
	c.mu.Lock()
	_, blocked := c.blockedUpdates[key]
	c.mu.Unlock()
	return blocked
}

func canonicalBlockedUpdate(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "none":
		return ""
	case "text", "agent_message_chunk":
		return "text"
	case "thought", "agent_thought_chunk":
		return "thought"
	case "tool", "tool_call", "tool_call_update", "tool_call_cancelled":
		return "tool_call"
	case "plan":
		return "plan"
	case "config_option_update":
		return "config_option_update"
	case "available_commands_update":
		return "available_commands_update"
	case "session_info_update":
		return "session_info_update"
	case "current_mode_update":
		return "current_mode_update"
	case "done", "prompt_result":
		return "prompt_result"
	case "error", "system":
		return "system"
	default:
		return strings.ToLower(strings.TrimSpace(raw))
	}
}
