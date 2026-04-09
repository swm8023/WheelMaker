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
	SendCard(chatID, messageID string, card Card) (string, error)
	SendReaction(messageID, emoji string) error
	SetUsage(chatID, usage string)
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

	mu                  sync.Mutex
	blockedUpdates      map[string]struct{}
	onPrompt            func(context.Context, im.ChatRef, acp.SessionPromptParams) error
	onCommand           func(context.Context, im.ChatRef, im.Command) error
	onPermissionResult  func(context.Context, im.ChatRef, int64, acp.PermissionResponse) error
	pendingByRequestID  map[int64]PendingPermission
	pendingByChat       map[string]PendingPermission
	helpCards           map[string]string
	pendingPromptByChat map[string][]acp.ContentBlock
	closed              map[int64]time.Time
}

func New(cfg Config) *Channel {
	return newWithTransportConfig(newTransport(cfg), cfg)
}

func newWithTransport(inner transport) *Channel {
	return newWithTransportConfig(inner, Config{})
}

func newWithTransportConfig(inner transport, cfg Config) *Channel {
	c := &Channel{
		inner:               inner,
		blockedUpdates:      buildBlockedUpdates(cfg.BlockedUpdates),
		pendingByRequestID:  map[int64]PendingPermission{},
		pendingByChat:       map[string]PendingPermission{},
		helpCards:           map[string]string{},
		pendingPromptByChat: map[string][]acp.ContentBlock{},
		closed:              map[int64]time.Time{},
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
	if payload.Kind == "help_card" && payload.HelpCard != nil {
		card := buildHelpCard(chatID, payload.HelpCard.Model, payload.HelpCard.MenuID, payload.HelpCard.Page)
		messageID := c.helpCardMessageID(chatID)
		sentID, err := c.inner.SendCard(chatID, messageID, card)
		if err != nil && messageID != "" {
			sentID, err = c.inner.SendCard(chatID, "", card)
		}
		if err != nil {
			return err
		}
		c.rememberHelpCard(chatID, sentID)
		return nil
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
		// Stream-breaking updates still insert a divider even when blocked,
		// so adjacent text chunks don't silently concatenate.
		if isStreamBreakingUpdate(params.Update.SessionUpdate) {
			return c.inner.Send(chatID, "", TextDivider)
		}
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
			_, err := c.inner.SendCard(chatID, "", ToolCallCard{Update: upd})
			return err
		}
	case acp.SessionUpdatePlan:
		if card := buildPlanCard(params.Update); card != nil {
			_, err := c.inner.SendCard(chatID, "", card)
			return err
		}
	case acp.SessionUpdateConfigOptionUpdate:
		if card := buildConfigCard(params.Update); card != nil {
			_, err := c.inner.SendCard(chatID, "", card)
			return err
		}
	case acp.SessionUpdateUsageUpdate:
		if text := renderUsageStreamText(params.Update); text != "" {
			c.inner.SetUsage(chatID, text)
		}
		return nil
	case acp.SessionUpdateAvailableCommandsUpdate:
		// Silenced: command list updates are noisy and rarely useful to the user.
	case acp.SessionUpdateUserMessageChunk:
		// User message reflection: skip (the user already sees their own message).
	case acp.SessionUpdateSessionInfoUpdate, acp.SessionUpdateCurrentModeUpdate:
		if msg := renderUpdateSummary("Session updated", params.Update); msg != "" {
			return c.inner.Send(chatID, msg, TextSystem)
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
	_, err := c.inner.SendCard(chatID, "", buildPermissionCard(chatID, requestID, params))
	return err
}

func (c *Channel) handleMessage(m Message) {
	if c.resolvePermissionText(m) {
		return
	}
	source := im.ChatRef{ChannelID: c.ID(), ChatID: strings.TrimSpace(m.ChatID)}
	if source.ChatID == "" {
		return
	}
	blocks := normalizePromptBlocks(m)
	if len(blocks) == 0 {
		return
	}
	if text, ok := singleTextPrompt(blocks); ok {
		if cmd, parsed := im.ParseCommand(text); parsed {
			c.mu.Lock()
			handler := c.onCommand
			c.mu.Unlock()
			if handler != nil {
				_ = handler(context.Background(), source, cmd)
			}
			return
		}
	}
	if !hasTextPromptBlock(blocks) {
		c.cachePendingPromptBlocks(source.ChatID, blocks)
		return
	}
	prefix := c.takePendingPromptBlocks(source.ChatID)
	if len(prefix) > 0 {
		merged := make([]acp.ContentBlock, 0, len(prefix)+len(blocks))
		merged = append(merged, prefix...)
		merged = append(merged, blocks...)
		blocks = merged
	}

	c.mu.Lock()
	handler := c.onPrompt
	c.mu.Unlock()
	if handler != nil {
		_ = handler(context.Background(), source, acp.SessionPromptParams{Prompt: blocks})
	}
}

func normalizePromptBlocks(m Message) []acp.ContentBlock {
	if len(m.Prompt) == 0 {
		text := strings.TrimSpace(m.Text)
		if text == "" {
			return nil
		}
		return []acp.ContentBlock{{Type: acp.ContentBlockTypeText, Text: text}}
	}
	blocks := make([]acp.ContentBlock, 0, len(m.Prompt))
	for _, block := range m.Prompt {
		kind := strings.TrimSpace(block.Type)
		if kind == "" {
			continue
		}
		if kind == acp.ContentBlockTypeText {
			text := strings.TrimSpace(block.Text)
			if text == "" {
				continue
			}
			block.Text = text
		}
		blocks = append(blocks, block)
	}
	if len(blocks) == 0 {
		text := strings.TrimSpace(m.Text)
		if text == "" {
			return nil
		}
		return []acp.ContentBlock{{Type: acp.ContentBlockTypeText, Text: text}}
	}
	return blocks
}

func singleTextPrompt(blocks []acp.ContentBlock) (string, bool) {
	if len(blocks) != 1 || blocks[0].Type != acp.ContentBlockTypeText {
		return "", false
	}
	text := strings.TrimSpace(blocks[0].Text)
	if text == "" {
		return "", false
	}
	return text, true
}

func hasTextPromptBlock(blocks []acp.ContentBlock) bool {
	for _, block := range blocks {
		if block.Type == acp.ContentBlockTypeText && strings.TrimSpace(block.Text) != "" {
			return true
		}
	}
	return false
}

func (c *Channel) cachePendingPromptBlocks(chatID string, blocks []acp.ContentBlock) {
	chatID = strings.TrimSpace(chatID)
	if chatID == "" || len(blocks) == 0 {
		return
	}
	copied := make([]acp.ContentBlock, 0, len(blocks))
	for _, block := range blocks {
		if block.Type != acp.ContentBlockTypeText {
			copied = append(copied, block)
		}
	}
	if len(copied) == 0 {
		return
	}
	c.mu.Lock()
	c.pendingPromptByChat[chatID] = append(c.pendingPromptByChat[chatID], copied...)
	c.mu.Unlock()
}

func (c *Channel) takePendingPromptBlocks(chatID string) []acp.ContentBlock {
	chatID = strings.TrimSpace(chatID)
	if chatID == "" {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	blocks := c.pendingPromptByChat[chatID]
	if len(blocks) == 0 {
		return nil
	}
	copied := append([]acp.ContentBlock(nil), blocks...)
	delete(c.pendingPromptByChat, chatID)
	return copied
}

func (c *Channel) handleCardAction(evt CardActionEvent) {
	kind := strings.TrimSpace(evt.Value["kind"])
	switch kind {
	case "permission":
		c.handlePermissionAction(evt)
	case "help_menu":
		c.handleHelpMenuAction(evt)
	case "help_page":
		c.handleHelpPageAction(evt)
	case "help_option":
		c.handleHelpOptionAction(evt)
	}
}

func (c *Channel) handlePermissionAction(evt CardActionEvent) {
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

func (c *Channel) handleHelpMenuAction(evt CardActionEvent) {
	chatID := strings.TrimSpace(evt.Value["chat_id"])
	if chatID == "" {
		chatID = evt.ChatID
	}
	menuID := strings.TrimSpace(evt.Value["menu_id"])
	if chatID == "" {
		return
	}
	c.rememberHelpCard(chatID, evt.MessageID)
	source := im.ChatRef{ChannelID: c.ID(), ChatID: chatID}
	args := menuID
	c.mu.Lock()
	handler := c.onCommand
	c.mu.Unlock()
	if handler != nil {
		_ = handler(context.Background(), source, im.Command{Name: "/help", Args: args, Raw: "/help " + args})
	}
}

func (c *Channel) handleHelpPageAction(evt CardActionEvent) {
	chatID := strings.TrimSpace(evt.Value["chat_id"])
	if chatID == "" {
		chatID = evt.ChatID
	}
	menuID := strings.TrimSpace(evt.Value["menu_id"])
	pageStr := strings.TrimSpace(evt.Value["page"])
	if chatID == "" {
		return
	}
	c.rememberHelpCard(chatID, evt.MessageID)
	source := im.ChatRef{ChannelID: c.ID(), ChatID: chatID}
	args := menuID + " " + pageStr
	c.mu.Lock()
	handler := c.onCommand
	c.mu.Unlock()
	if handler != nil {
		_ = handler(context.Background(), source, im.Command{Name: "/help", Args: args, Raw: "/help " + args})
	}
}

func (c *Channel) handleHelpOptionAction(evt CardActionEvent) {
	chatID := strings.TrimSpace(evt.Value["chat_id"])
	if chatID == "" {
		chatID = evt.ChatID
	}
	cmd := strings.TrimSpace(evt.Value["command"])
	val := strings.TrimSpace(evt.Value["value"])
	if cmd == "" || chatID == "" {
		return
	}
	c.rememberHelpCard(chatID, evt.MessageID)
	source := im.ChatRef{ChannelID: c.ID(), ChatID: chatID}
	text := cmd
	if val != "" {
		text = cmd + " " + val
	}

	// Execute the action command, then re-open help at root
	c.mu.Lock()
	handler := c.onCommand
	c.mu.Unlock()
	if handler == nil {
		return
	}
	if parsed, ok := im.ParseCommand(text); ok {
		_ = handler(context.Background(), source, parsed)
	}

	// Re-open help menu at root to show updated state
	_ = handler(context.Background(), source, im.Command{Name: "/help", Args: "", Raw: "/help"})
}

func (c *Channel) resolvePermissionText(m Message) bool {
	text := strings.TrimSpace(m.Text)
	if text == "" {
		return false
	}
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
		_ = handler(context.Background(), im.ChatRef{ChannelID: c.ID(), ChatID: chatID}, pending.RequestID, parsePermissionReply(text, pending.Options))
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

func (c *Channel) helpCardMessageID(chatID string) string {
	chatID = strings.TrimSpace(chatID)
	if chatID == "" {
		return ""
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return strings.TrimSpace(c.helpCards[chatID])
}

func (c *Channel) rememberHelpCard(chatID, messageID string) {
	chatID = strings.TrimSpace(chatID)
	messageID = strings.TrimSpace(messageID)
	if chatID == "" || messageID == "" {
		return
	}
	c.mu.Lock()
	c.helpCards[chatID] = messageID
	c.mu.Unlock()
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
	case "usage", "usage_update":
		return "usage_update"
	case "done", "prompt_result":
		return "prompt_result"
	case "error", "system":
		return "system"
	default:
		return strings.ToLower(strings.TrimSpace(raw))
	}
}

func isStreamBreakingUpdate(updateType string) bool {
	switch updateType {
	case acp.SessionUpdateToolCall, acp.SessionUpdateToolCallUpdate, acp.SessionUpdatePlan:
		return true
	}
	return false
}
