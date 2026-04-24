package app

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/swm8023/wheelmaker/internal/im"
	acp "github.com/swm8023/wheelmaker/internal/protocol"
)

type Channel struct {
	mu sync.Mutex

	onPrompt  func(context.Context, im.ChatRef, acp.SessionPromptParams) error
	onCommand func(context.Context, im.ChatRef, im.Command) error

	projectID string
	publish   func(string, any) error
	seq       atomic.Int64
	sessions  map[string]*chatSession
}

type chatSession struct {
	ChatID            string
	SessionID         string
	Title             string
	Preview           string
	UpdatedAt         string
	MessageCount      int
	Messages          []map[string]any
	activeAssistantID string
	activeThoughtID   string
}

type chatSendPayload struct {
	ChatID string             `json:"chatId"`
	Text   string             `json:"text,omitempty"`
	Blocks []acp.ContentBlock `json:"blocks,omitempty"`
}

type chatSessionReadPayload struct {
	ChatID string `json:"chatId"`
}

func New() *Channel {
	return &Channel{
		sessions: make(map[string]*chatSession),
	}
}

func (c *Channel) ID() string { return "app" }

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

func (c *Channel) SetEventPublisher(projectID string, publish func(string, any) error) {
	c.mu.Lock()
	c.projectID = strings.TrimSpace(projectID)
	c.publish = publish
	c.mu.Unlock()
}

func (c *Channel) HandleChatRequest(ctx context.Context, method string, projectID string, payload json.RawMessage) (any, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return nil, fmt.Errorf("projectId is required")
	}
	c.mu.Lock()
	if c.projectID == "" {
		c.projectID = projectID
	}
	c.mu.Unlock()

	switch strings.TrimSpace(method) {
	case "chat.send":
		var req chatSendPayload
		if err := decodePayload(payload, &req); err != nil {
			return nil, fmt.Errorf("invalid chat.send payload: %w", err)
		}
		chatID := strings.TrimSpace(req.ChatID)
		if chatID == "" {
			return nil, fmt.Errorf("chatId is required")
		}
		blocks := normalizePrompt(req.Text, req.Blocks)
		if len(blocks) == 0 {
			return nil, fmt.Errorf("chat prompt is empty")
		}

		source := im.ChatRef{ChannelID: c.ID(), ChatID: chatID}
		if text, ok := singleTextPrompt(blocks); ok {
			if cmd, parsed := im.ParseCommand(text); parsed {
				c.mu.Lock()
				handler := c.onCommand
				c.mu.Unlock()
				if handler != nil {
					if err := handler(ctx, source, cmd); err != nil {
						return nil, err
					}
				}
				return map[string]any{"ok": true, "chatId": chatID, "command": cmd.Name}, nil
			}
		}

		c.appendNewMessage(chatID, map[string]any{
			"role":   "user",
			"kind":   blocksKind(blocks),
			"text":   previewText(blocks),
			"status": "done",
			"blocks": blocks,
		})

		c.mu.Lock()
		handler := c.onPrompt
		c.mu.Unlock()
		if handler != nil {
			if err := handler(ctx, source, acp.SessionPromptParams{Prompt: blocks}); err != nil {
				return nil, err
			}
		}
		return map[string]any{"ok": true, "chatId": chatID}, nil
	default:
		return nil, fmt.Errorf("unsupported chat method: %s", method)
	}
}

func (c *Channel) PublishSessionUpdate(ctx context.Context, target im.SendTarget, params acp.SessionUpdateParams) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	chatID := resolveChatID(target)
	if chatID == "" {
		return fmt.Errorf("im app: chat is empty")
	}

	switch params.Update.SessionUpdate {
	case acp.SessionUpdateAgentMessageChunk:
		text := extractTextContent(params.Update.Content)
		if strings.TrimSpace(text) == "" {
			return nil
		}
		c.appendStreamingMessage(chatID, target.SessionID, "assistant", "text", text, &c.getSession(chatID).activeAssistantID)
	case acp.SessionUpdateAgentThoughtChunk:
		text := extractTextContent(params.Update.Content)
		if strings.TrimSpace(text) == "" {
			return nil
		}
		c.appendStreamingMessage(chatID, target.SessionID, "assistant", "thought", text, &c.getSession(chatID).activeThoughtID)
	case acp.SessionUpdateToolCall, acp.SessionUpdateToolCallUpdate:
		return c.appendNewMessage(chatID, map[string]any{
			"sessionId": target.SessionID,
			"role":      "system",
			"kind":      "tool",
			"text":      renderToolStatus(params.Update),
			"status":    "done",
		})
	}
	return nil
}

func (c *Channel) PublishPromptResult(ctx context.Context, target im.SendTarget, result acp.SessionPromptResult) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	chatID := resolveChatID(target)
	if chatID == "" {
		return fmt.Errorf("im app: chat is empty")
	}
	return c.finishStreamingMessages(chatID, target.SessionID, result.StopReason)
}

func (c *Channel) SystemNotify(ctx context.Context, target im.SendTarget, payload im.SystemPayload) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	chatID := resolveChatID(target)
	if chatID == "" {
		return fmt.Errorf("im app: chat is empty")
	}
	body := strings.TrimSpace(payload.Body)
	if body == "" {
		body = strings.TrimSpace(payload.Title)
	}
	if body == "" {
		return nil
	}
	return c.appendNewMessage(chatID, map[string]any{
		"sessionId": target.SessionID,
		"role":      "system",
		"kind":      strings.TrimSpace(payload.Kind),
		"text":      body,
		"status":    "done",
	})
}

func (c *Channel) Run(context.Context) error {
	return nil
}

func (c *Channel) snapshotSessions() []map[string]any {
	c.mu.Lock()
	defer c.mu.Unlock()

	items := make([]map[string]any, 0, len(c.sessions))
	for _, session := range c.sessions {
		items = append(items, map[string]any{
			"chatId":       session.ChatID,
			"sessionId":    session.SessionID,
			"title":        session.Title,
			"preview":      session.Preview,
			"updatedAt":    session.UpdatedAt,
			"messageCount": session.MessageCount,
		})
	}
	sort.Slice(items, func(i, j int) bool {
		left, _ := items[i]["updatedAt"].(string)
		right, _ := items[j]["updatedAt"].(string)
		return left > right
	})
	return items
}

func (c *Channel) snapshotSession(chatID string) (map[string]any, []map[string]any, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	session := c.sessions[chatID]
	if session == nil {
		return nil, nil, fmt.Errorf("chat session not found: %s", chatID)
	}
	meta := map[string]any{
		"chatId":       session.ChatID,
		"sessionId":    session.SessionID,
		"title":        session.Title,
		"preview":      session.Preview,
		"updatedAt":    session.UpdatedAt,
		"messageCount": session.MessageCount,
	}
	messages := make([]map[string]any, 0, len(session.Messages))
	for _, message := range session.Messages {
		messages = append(messages, cloneMap(message))
	}
	return meta, messages, nil
}

func (c *Channel) appendStreamingMessage(chatID, sessionID, role, kind, text string, activeID *string) error {
	c.mu.Lock()
	session := c.ensureSessionLocked(chatID)
	session.SessionID = firstNonEmpty(strings.TrimSpace(sessionID), session.SessionID)
	now := time.Now().UTC().Format(time.RFC3339)
	var message map[string]any
	if strings.TrimSpace(*activeID) == "" {
		message = map[string]any{
			"messageId": c.nextMessageID(),
			"chatId":    chatID,
			"sessionId": session.SessionID,
			"role":      role,
			"kind":      kind,
			"text":      text,
			"status":    "streaming",
			"createdAt": now,
			"updatedAt": now,
		}
		session.Messages = append(session.Messages, message)
		session.MessageCount++
		*activeID = messageString(message, "messageId")
	} else {
		message = findMessage(session.Messages, *activeID)
		if message == nil {
			message = map[string]any{
				"messageId": *activeID,
				"chatId":    chatID,
				"sessionId": session.SessionID,
				"role":      role,
				"kind":      kind,
				"text":      "",
				"status":    "streaming",
				"createdAt": now,
				"updatedAt": now,
			}
			session.Messages = append(session.Messages, message)
			session.MessageCount++
		}
		message["text"] = messageString(message, "text") + text
		message["updatedAt"] = now
	}
	session.Preview = messageString(message, "text")
	session.UpdatedAt = now
	projectID, publish := c.publisherLocked()
	payload := map[string]any{
		"session": sessionSummary(session),
		"message": cloneMap(message),
	}
	c.mu.Unlock()
	return publishEvent(projectID, publish, payload)
}

func (c *Channel) appendNewMessage(chatID string, fields map[string]any) error {
	c.mu.Lock()
	session := c.ensureSessionLocked(chatID)
	now := time.Now().UTC().Format(time.RFC3339)
	message := map[string]any{
		"messageId": c.nextMessageID(),
		"chatId":    chatID,
		"sessionId": firstNonEmpty(messageString(fields, "sessionId"), session.SessionID),
		"role":      firstNonEmpty(messageString(fields, "role"), "system"),
		"kind":      firstNonEmpty(messageString(fields, "kind"), "text"),
		"text":      messageString(fields, "text"),
		"status":    firstNonEmpty(messageString(fields, "status"), "done"),
		"createdAt": now,
		"updatedAt": now,
	}
	for key, value := range fields {
		if _, exists := message[key]; exists {
			continue
		}
		message[key] = value
	}
	session.SessionID = messageString(message, "sessionId")
	session.Messages = append(session.Messages, message)
	session.MessageCount++
	session.Preview = messageString(message, "text")
	session.UpdatedAt = now
	if strings.TrimSpace(session.Title) == "" {
		session.Title = defaultTitle(chatID, session.Preview)
	}
	projectID, publish := c.publisherLocked()
	payload := map[string]any{
		"session": sessionSummary(session),
		"message": cloneMap(message),
	}
	c.mu.Unlock()
	return publishEvent(projectID, publish, payload)
}

func (c *Channel) finishStreamingMessages(chatID, sessionID, stopReason string) error {
	c.mu.Lock()
	session := c.ensureSessionLocked(chatID)
	session.SessionID = firstNonEmpty(strings.TrimSpace(sessionID), session.SessionID)
	now := time.Now().UTC().Format(time.RFC3339)
	toPublish := make([]map[string]any, 0, 2)
	for _, activeID := range []string{session.activeAssistantID, session.activeThoughtID} {
		if strings.TrimSpace(activeID) == "" {
			continue
		}
		message := findMessage(session.Messages, activeID)
		if message == nil {
			continue
		}
		message["status"] = "done"
		message["updatedAt"] = now
		toPublish = append(toPublish, cloneMap(message))
	}
	session.activeAssistantID = ""
	session.activeThoughtID = ""
	if strings.TrimSpace(stopReason) != "" && stopReason != acp.StopReasonEndTurn {
		message := map[string]any{
			"messageId": c.nextMessageID(),
			"chatId":    chatID,
			"sessionId": session.SessionID,
			"role":      "system",
			"kind":      "prompt_result",
			"text":      stopReason,
			"status":    "done",
			"createdAt": now,
			"updatedAt": now,
		}
		session.Messages = append(session.Messages, message)
		session.MessageCount++
		session.Preview = stopReason
		toPublish = append(toPublish, cloneMap(message))
	}
	session.UpdatedAt = now
	projectID, publish := c.publisherLocked()
	summary := sessionSummary(session)
	c.mu.Unlock()

	for _, message := range toPublish {
		if err := publishEvent(projectID, publish, map[string]any{"session": summary, "message": message}); err != nil {
			return err
		}
	}
	return nil
}

func (c *Channel) ensureSessionLocked(chatID string) *chatSession {
	if c.sessions == nil {
		c.sessions = make(map[string]*chatSession)
	}
	session := c.sessions[chatID]
	if session == nil {
		session = &chatSession{
			ChatID: chatID,
			Title:  defaultTitle(chatID, ""),
		}
		c.sessions[chatID] = session
	}
	return session
}

func (c *Channel) getSession(chatID string) *chatSession {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ensureSessionLocked(chatID)
}

func (c *Channel) publisherLocked() (string, func(string, any) error) {
	return c.projectID, c.publish
}

func (c *Channel) nextMessageID() string {
	return fmt.Sprintf("msg-%d", c.seq.Add(1))
}

func sessionSummary(session *chatSession) map[string]any {
	return map[string]any{
		"chatId":       session.ChatID,
		"sessionId":    session.SessionID,
		"title":        session.Title,
		"preview":      session.Preview,
		"updatedAt":    session.UpdatedAt,
		"messageCount": session.MessageCount,
	}
}

func publishEvent(projectID string, publish func(string, any) error, payload any) error {
	if publish == nil || strings.TrimSpace(projectID) == "" {
		return nil
	}
	return publish(projectID, payload)
}

func resolveChatID(target im.SendTarget) string {
	if chatID := strings.TrimSpace(target.ChatID); chatID != "" {
		return chatID
	}
	if target.Source != nil {
		return strings.TrimSpace(target.Source.ChatID)
	}
	return ""
}

func decodePayload(raw json.RawMessage, out any) error {
	if len(raw) == 0 || strings.TrimSpace(string(raw)) == "" {
		return nil
	}
	return json.Unmarshal(raw, out)
}

func normalizePrompt(text string, blocks []acp.ContentBlock) []acp.ContentBlock {
	if len(blocks) > 0 {
		return blocks
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	return []acp.ContentBlock{{Type: acp.ContentBlockTypeText, Text: text}}
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

func blocksKind(blocks []acp.ContentBlock) string {
	for _, block := range blocks {
		if block.Type == acp.ContentBlockTypeImage {
			return "image"
		}
	}
	return "text"
}

func flattenText(blocks []acp.ContentBlock) string {
	parts := make([]string, 0, len(blocks))
	for _, block := range blocks {
		if block.Type == acp.ContentBlockTypeText && strings.TrimSpace(block.Text) != "" {
			parts = append(parts, strings.TrimSpace(block.Text))
		}
	}
	return strings.Join(parts, "\n")
}

func previewText(blocks []acp.ContentBlock) string {
	text := flattenText(blocks)
	if strings.TrimSpace(text) != "" {
		return text
	}
	for _, block := range blocks {
		if block.Type == acp.ContentBlockTypeImage {
			return "Sent an image"
		}
	}
	return ""
}

func extractTextContent(raw json.RawMessage) string {
	var block acp.ContentBlock
	if err := json.Unmarshal(raw, &block); err != nil {
		return ""
	}
	if block.Type != acp.ContentBlockTypeText {
		return ""
	}
	return block.Text
}

func renderToolStatus(update acp.SessionUpdate) string {
	title := strings.TrimSpace(update.Title)
	if title == "" {
		title = strings.TrimSpace(update.Kind)
	}
	status := strings.TrimSpace(update.Status)
	if title == "" && status == "" {
		return "Tool activity"
	}
	if title == "" {
		return status
	}
	if status == "" {
		return title
	}
	return title + " - " + status
}

func defaultTitle(chatID, preview string) string {
	if strings.TrimSpace(preview) != "" {
		runes := []rune(strings.TrimSpace(preview))
		if len(runes) > 24 {
			return string(runes[:24]) + "..."
		}
		return string(runes)
	}
	if strings.TrimSpace(chatID) == "" {
		return "Chat"
	}
	return chatID
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func findMessage(messages []map[string]any, messageID string) map[string]any {
	for _, message := range messages {
		if messageString(message, "messageId") == messageID {
			return message
		}
	}
	return nil
}

func cloneMap(input map[string]any) map[string]any {
	out := make(map[string]any, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func messageString(input map[string]any, key string) string {
	value, _ := input[key].(string)
	return strings.TrimSpace(value)
}

func hasPermissionOption(options []acp.PermissionOption, optionID string) bool {
	for _, option := range options {
		if strings.TrimSpace(option.OptionID) == optionID {
			return true
		}
	}
	return false
}
