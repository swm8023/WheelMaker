package client

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/swm8023/wheelmaker/internal/im"
	acp "github.com/swm8023/wheelmaker/internal/protocol"
)

type sessionViewSummary struct {
	SessionID    string `json:"sessionId"`
	Title        string `json:"title"`
	Preview      string `json:"preview"`
	UpdatedAt    string `json:"updatedAt"`
	MessageCount int    `json:"messageCount"`
	UnreadCount  int    `json:"unreadCount"`
	Agent        string `json:"agent,omitempty"`
	Status       string `json:"status,omitempty"`
}

type sessionViewMessage struct {
	MessageID string                 `json:"messageId"`
	SessionID string                 `json:"sessionId"`
	SyncIndex int64                  `json:"syncIndex,omitempty"`
	Role      string                 `json:"role"`
	Kind      string                 `json:"kind"`
	Text      string                 `json:"text"`
	Blocks    []acp.ContentBlock     `json:"blocks,omitempty"`
	Options   []acp.PermissionOption `json:"options,omitempty"`
	Status    string                 `json:"status"`
	CreatedAt string                 `json:"createdAt"`
	UpdatedAt string                 `json:"updatedAt"`
	RequestID int64                  `json:"requestId,omitempty"`
}

func (c *Client) SetSessionEventPublisher(publish func(method string, payload any) error) {
	c.mu.Lock()
	c.sessionEventPublish = publish
	c.mu.Unlock()
}

func (c *Client) RecordEvent(ctx context.Context, event SessionViewEvent) error {
	event.SessionID = strings.TrimSpace(event.SessionID)
	if event.SessionID == "" {
		return nil
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}
	if event.UpdatedAt.IsZero() {
		event.UpdatedAt = event.CreatedAt
	}

	switch event.Type {
	case SessionViewEventSessionCreated:
		return c.upsertSessionProjection(ctx, event.SessionID, strings.TrimSpace(event.Title), "", event.UpdatedAt, false)
	case SessionViewEventUserMessageAccepted:
		if err := c.flushSessionProjection(ctx, event.SessionID); err != nil {
			return err
		}
		message := SessionMessageRecord{
			MessageID:    fmt.Sprintf("msg-user-%d", event.CreatedAt.UnixNano()),
			ProjectName:  c.projectName,
			SessionID:    event.SessionID,
			Role:         firstNonEmpty(event.Role, "user"),
			Kind:         firstNonEmpty(event.Kind, "text"),
			Body:         strings.TrimSpace(event.Text),
			Blocks:       cloneSessionContentBlocks(event.Blocks),
			Status:       firstNonEmpty(event.Status, "done"),
			CreatedAt:    event.CreatedAt,
			UpdatedAt:    event.UpdatedAt,
			RequestID:    event.RequestID,
			AggregateKey: firstNonEmpty(event.AggregateKey, fmt.Sprintf("user:%s:%d", event.SessionID, event.CreatedAt.UnixNano())),
		}
		if err := c.store.AppendSessionMessage(ctx, message); err != nil {
			return err
		}
		stored, err := c.loadStoredSessionMessage(ctx, event.SessionID, message.MessageID)
		if err != nil {
			return err
		}
		if err := c.upsertSessionProjection(ctx, event.SessionID, strings.TrimSpace(event.Title), stored.Body, stored.UpdatedAt, true); err != nil {
			return err
		}
		c.publishSessionMessage(stored)
		return nil
	case SessionViewEventAssistantChunk, SessionViewEventThoughtChunk:
		message := c.sessionAggregator.appendChunk(c.projectName, event)
		existed, err := c.store.HasSessionMessage(ctx, c.projectName, event.SessionID, message.MessageID)
		if err != nil {
			return err
		}
		if err := c.store.UpsertSessionMessage(ctx, message); err != nil {
			return err
		}
		stored, err := c.loadStoredSessionMessage(ctx, event.SessionID, message.MessageID)
		if err != nil {
			return err
		}
		if err := c.upsertSessionProjection(ctx, event.SessionID, strings.TrimSpace(event.Title), stored.Body, stored.UpdatedAt, !existed); err != nil {
			return err
		}
		if !existed {
			c.incrementSessionUnread(event.SessionID)
		}
		c.publishSessionMessage(stored)
		return nil
	case SessionViewEventPromptFinished:
		return c.flushSessionProjection(ctx, event.SessionID)
	case SessionViewEventPermissionRequested:
		message := SessionMessageRecord{
			MessageID:    fmt.Sprintf("msg-permission-%s-%d", event.SessionID, event.RequestID),
			ProjectName:  c.projectName,
			SessionID:    event.SessionID,
			Role:         firstNonEmpty(event.Role, "system"),
			Kind:         firstNonEmpty(event.Kind, "permission"),
			Body:         strings.TrimSpace(event.Text),
			Options:      cloneSessionPermissionOptions(event.Options),
			Status:       firstNonEmpty(event.Status, "needs_action"),
			CreatedAt:    event.CreatedAt,
			UpdatedAt:    event.UpdatedAt,
			RequestID:    event.RequestID,
			AggregateKey: fmt.Sprintf("permission:%s:%d", event.SessionID, event.RequestID),
		}
		existed, err := c.store.HasSessionMessage(ctx, c.projectName, event.SessionID, message.MessageID)
		if err != nil {
			return err
		}
		if err := c.store.UpsertSessionMessage(ctx, message); err != nil {
			return err
		}
		stored, err := c.loadStoredSessionMessage(ctx, event.SessionID, message.MessageID)
		if err != nil {
			return err
		}
		if err := c.upsertSessionProjection(ctx, event.SessionID, strings.TrimSpace(event.Title), stored.Body, stored.UpdatedAt, !existed); err != nil {
			return err
		}
		if !existed {
			c.incrementSessionUnread(event.SessionID)
		}
		c.publishSessionMessage(stored)
		return nil
	case SessionViewEventPermissionResolved:
		messages, err := c.store.ListSessionMessages(ctx, c.projectName, event.SessionID)
		if err != nil {
			return err
		}
		for _, message := range messages {
			if message.RequestID != event.RequestID {
				continue
			}
			message.Status = firstNonEmpty(event.Status, "done")
			message.UpdatedAt = event.UpdatedAt
			if err := c.store.UpsertSessionMessage(ctx, message); err != nil {
				return err
			}
			stored, err := c.loadStoredSessionMessage(ctx, event.SessionID, message.MessageID)
			if err != nil {
				return err
			}
			c.publishSessionMessage(stored)
			break
		}
		return nil
	case SessionViewEventSystemMessage, SessionViewEventToolUpdated:
		messageID := fmt.Sprintf("msg-system-%d", event.CreatedAt.UnixNano())
		if strings.TrimSpace(event.AggregateKey) != "" {
			messageID = sanitizeSessionAggregateMessageID(event.Kind, event.AggregateKey)
		}
		message := SessionMessageRecord{
			MessageID:    messageID,
			ProjectName:  c.projectName,
			SessionID:    event.SessionID,
			Role:         firstNonEmpty(event.Role, "system"),
			Kind:         firstNonEmpty(event.Kind, "system"),
			Body:         strings.TrimSpace(event.Text),
			Status:       firstNonEmpty(event.Status, "done"),
			CreatedAt:    event.CreatedAt,
			UpdatedAt:    event.UpdatedAt,
			RequestID:    event.RequestID,
			AggregateKey: firstNonEmpty(event.AggregateKey, fmt.Sprintf("system:%s:%d", event.SessionID, event.CreatedAt.UnixNano())),
		}
		existed, err := c.store.HasSessionMessage(ctx, c.projectName, event.SessionID, message.MessageID)
		if err != nil {
			return err
		}
		if err := c.store.UpsertSessionMessage(ctx, message); err != nil {
			return err
		}
		stored, err := c.loadStoredSessionMessage(ctx, event.SessionID, message.MessageID)
		if err != nil {
			return err
		}
		if err := c.upsertSessionProjection(ctx, event.SessionID, strings.TrimSpace(event.Title), stored.Body, stored.UpdatedAt, !existed); err != nil {
			return err
		}
		if !existed {
			c.incrementSessionUnread(event.SessionID)
		}
		c.publishSessionMessage(stored)
		return nil
	default:
		return nil
	}
}

func (c *Client) HandleSessionRequest(ctx context.Context, method string, _ string, payload json.RawMessage) (any, error) {
	switch strings.TrimSpace(method) {
	case "session.list":
		sessions, err := c.listSessionViews(ctx)
		if err != nil {
			return nil, err
		}
		return map[string]any{"sessions": sessions}, nil
	case "session.read":
		var req struct {
			SessionID  string `json:"sessionId"`
			AfterIndex int64  `json:"afterIndex,omitempty"`
		}
		if err := decodeSessionRequestPayload(payload, &req); err != nil {
			return nil, fmt.Errorf("invalid session.read payload: %w", err)
		}
		summary, messages, lastIndex, err := c.readSessionView(ctx, req.SessionID, req.AfterIndex)
		if err != nil {
			return nil, err
		}
		return map[string]any{"session": summary, "messages": messages, "lastIndex": lastIndex}, nil
	case "session.new":
		var req struct {
			Title string `json:"title,omitempty"`
		}
		if err := decodeSessionRequestPayload(payload, &req); err != nil {
			return nil, fmt.Errorf("invalid session.new payload: %w", err)
		}
		sess, err := c.CreateSession(ctx, req.Title)
		if err != nil {
			return nil, err
		}
		if err := c.RecordEvent(ctx, SessionViewEvent{Type: SessionViewEventSessionCreated, SessionID: sess.ID, Title: firstNonEmpty(req.Title, sess.ID)}); err != nil {
			return nil, err
		}
		summary, _, _, err := c.readSessionView(ctx, sess.ID, 0)
		if err != nil {
			return nil, err
		}
		return map[string]any{"ok": true, "session": summary}, nil
	case "session.send":
		var req struct {
			SessionID string             `json:"sessionId"`
			Text      string             `json:"text,omitempty"`
			Blocks    []acp.ContentBlock `json:"blocks,omitempty"`
		}
		if err := decodeSessionRequestPayload(payload, &req); err != nil {
			return nil, fmt.Errorf("invalid session.send payload: %w", err)
		}
		blocks := req.Blocks
		if len(blocks) == 0 && strings.TrimSpace(req.Text) != "" {
			blocks = []acp.ContentBlock{{Type: acp.ContentBlockTypeText, Text: req.Text}}
		}
		if strings.TrimSpace(req.SessionID) == "" {
			return nil, fmt.Errorf("sessionId is required")
		}
		if len(blocks) == 0 {
			return nil, fmt.Errorf("session prompt is empty")
		}
		if err := c.SendToSession(ctx, req.SessionID, im.ChatRef{ChannelID: "app", ChatID: strings.TrimSpace(req.SessionID)}, blocks); err != nil {
			return nil, err
		}
		return map[string]any{"ok": true, "sessionId": strings.TrimSpace(req.SessionID)}, nil
	case "session.markRead":
		var req struct {
			SessionID string `json:"sessionId"`
		}
		if err := decodeSessionRequestPayload(payload, &req); err != nil {
			return nil, fmt.Errorf("invalid session.markRead payload: %w", err)
		}
		c.resetSessionUnread(strings.TrimSpace(req.SessionID))
		if summary, ok := c.currentSessionViewSummary(ctx, req.SessionID); ok {
			c.publishSessionUpdated(summary)
		}
		return map[string]any{"ok": true}, nil
	default:
		return nil, fmt.Errorf("unsupported session method: %s", method)
	}
}

func (c *Client) listSessionViews(ctx context.Context) ([]sessionViewSummary, error) {
	entries, err := c.ListSessions(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]sessionViewSummary, 0, len(entries))
	for _, entry := range entries {
		out = append(out, c.sessionViewSummaryFromEntry(entry))
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].UpdatedAt > out[j].UpdatedAt
	})
	return out, nil
}

func (c *Client) readSessionView(ctx context.Context, sessionID string, afterIndex int64) (sessionViewSummary, []sessionViewMessage, int64, error) {
	rec, err := c.store.LoadSession(ctx, c.projectName, strings.TrimSpace(sessionID))
	if err != nil {
		return sessionViewSummary{}, nil, 0, err
	}
	if rec == nil {
		return sessionViewSummary{}, nil, 0, fmt.Errorf("session not found: %s", sessionID)
	}
	var messages []SessionMessageRecord
	if afterIndex > 0 {
		messages, err = c.store.ListSessionMessagesAfterIndex(ctx, c.projectName, strings.TrimSpace(sessionID), afterIndex)
	} else {
		messages, err = c.store.ListSessionMessages(ctx, c.projectName, strings.TrimSpace(sessionID))
	}
	if err != nil {
		return sessionViewSummary{}, nil, 0, err
	}
	out := make([]sessionViewMessage, 0, len(messages))
	for _, message := range messages {
		out = append(out, toSessionViewMessage(message))
	}
	return c.sessionViewSummaryFromRecord(*rec), out, rec.LastSyncIndex, nil
}

func (c *Client) flushSessionProjection(ctx context.Context, sessionID string) error {
	flushed := c.sessionAggregator.flushSession(sessionID)
	for _, message := range flushed {
		existed, err := c.store.HasSessionMessage(ctx, c.projectName, sessionID, message.MessageID)
		if err != nil {
			return err
		}
		if err := c.store.UpsertSessionMessage(ctx, message); err != nil {
			return err
		}
		stored, err := c.loadStoredSessionMessage(ctx, sessionID, message.MessageID)
		if err != nil {
			return err
		}
		if err := c.upsertSessionProjection(ctx, sessionID, "", stored.Body, stored.UpdatedAt, !existed); err != nil {
			return err
		}
		if !existed {
			c.incrementSessionUnread(sessionID)
		}
		c.publishSessionMessage(stored)
	}
	return nil
}

func (c *Client) loadStoredSessionMessage(ctx context.Context, sessionID, messageID string) (SessionMessageRecord, error) {
	message, err := c.store.LoadSessionMessage(ctx, c.projectName, strings.TrimSpace(sessionID), strings.TrimSpace(messageID))
	if err != nil {
		return SessionMessageRecord{}, err
	}
	if message != nil {
		return *message, nil
	}
	return SessionMessageRecord{}, fmt.Errorf("session message not found: %s", messageID)
}

func (c *Client) upsertSessionProjection(ctx context.Context, sessionID, title, preview string, updatedAt time.Time, incrementMessageCount bool) error {
	rec, err := c.store.LoadSession(ctx, c.projectName, sessionID)
	if err != nil {
		return err
	}
	if rec == nil {
		rec = &SessionRecord{ID: sessionID, ProjectName: c.projectName, Status: SessionActive, CreatedAt: updatedAt, LastActiveAt: updatedAt}
	}
	if strings.TrimSpace(title) != "" {
		rec.Title = strings.TrimSpace(title)
	}
	if strings.TrimSpace(preview) != "" {
		rec.LastMessagePreview = strings.TrimSpace(preview)
		rec.LastMessageAt = updatedAt
	}
	rec.LastActiveAt = updatedAt
	if incrementMessageCount {
		rec.MessageCount += 1
	}
	if err := c.store.SaveSession(ctx, rec); err != nil {
		return err
	}
	if summary, ok := c.currentSessionViewSummary(ctx, sessionID); ok {
		c.publishSessionUpdated(summary)
	}
	return nil
}

func (c *Client) currentSessionViewSummary(ctx context.Context, sessionID string) (sessionViewSummary, bool) {
	rec, err := c.store.LoadSession(ctx, c.projectName, strings.TrimSpace(sessionID))
	if err != nil || rec == nil {
		return sessionViewSummary{}, false
	}
	return c.sessionViewSummaryFromRecord(*rec), true
}

func (c *Client) sessionViewSummaryFromEntry(entry SessionListEntry) sessionViewSummary {
	updatedAt := entry.LastMessageAt
	if updatedAt.IsZero() {
		updatedAt = entry.LastActiveAt
	}
	return sessionViewSummary{
		SessionID:    entry.ID,
		Title:        firstNonEmpty(entry.Title, entry.ID),
		Preview:      entry.Preview,
		UpdatedAt:    updatedAt.UTC().Format(time.RFC3339),
		MessageCount: entry.MessageCount,
		UnreadCount:  c.sessionUnread(entry.ID),
		Agent:        entry.Agent,
		Status:       sessionStatusLabel(entry.Status),
	}
}

func (c *Client) sessionViewSummaryFromRecord(rec SessionRecord) sessionViewSummary {
	updatedAt := rec.LastMessageAt
	if updatedAt.IsZero() {
		updatedAt = rec.LastActiveAt
	}
	return sessionViewSummary{
		SessionID:    rec.ID,
		Title:        firstNonEmpty(rec.Title, rec.ID),
		Preview:      rec.LastMessagePreview,
		UpdatedAt:    updatedAt.UTC().Format(time.RFC3339),
		MessageCount: rec.MessageCount,
		UnreadCount:  c.sessionUnread(rec.ID),
		Status:       sessionStatusLabel(rec.Status),
	}
}

func (c *Client) incrementSessionUnread(sessionID string) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}
	c.mu.Lock()
	c.sessionUnreadCount[sessionID] += 1
	c.mu.Unlock()
}

func (c *Client) resetSessionUnread(sessionID string) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}
	c.mu.Lock()
	c.sessionUnreadCount[sessionID] = 0
	c.mu.Unlock()
}

func (c *Client) sessionUnread(sessionID string) int {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.sessionUnreadCount[sessionID]
}

func (c *Client) publishSessionUpdated(summary sessionViewSummary) {
	publish := c.sessionEventPublisher()
	if publish == nil {
		return
	}
	_ = publish("registry.session.updated", map[string]any{"session": summary})
}

func (c *Client) publishSessionMessage(message SessionMessageRecord) {
	publish := c.sessionEventPublisher()
	if publish == nil {
		return
	}
	ctx := context.Background()
	summary, ok := c.currentSessionViewSummary(ctx, message.SessionID)
	if !ok {
		summary = sessionViewSummary{SessionID: message.SessionID, Title: message.SessionID, Preview: message.Body, UpdatedAt: message.UpdatedAt.UTC().Format(time.RFC3339)}
	}
	_ = publish("registry.session.message", map[string]any{"session": summary, "message": toSessionViewMessage(message)})
}

func (c *Client) sessionEventPublisher() func(method string, payload any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.sessionEventPublish
}

func toSessionViewMessage(message SessionMessageRecord) sessionViewMessage {
	return sessionViewMessage{
		MessageID: message.MessageID,
		SessionID: message.SessionID,
		SyncIndex: message.SyncIndex,
		Role:      message.Role,
		Kind:      message.Kind,
		Text:      message.Body,
		Blocks:    cloneSessionContentBlocks(message.Blocks),
		Options:   cloneSessionPermissionOptions(message.Options),
		Status:    message.Status,
		CreatedAt: message.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt: message.UpdatedAt.UTC().Format(time.RFC3339),
		RequestID: message.RequestID,
	}
}

func sanitizeSessionAggregateMessageID(kind, aggregateKey string) string {
	replacer := strings.NewReplacer(":", "-", "/", "-", "\\", "-", " ", "-", ".", "-")
	cleanKey := strings.Trim(replacer.Replace(strings.TrimSpace(aggregateKey)), "-")
	if cleanKey == "" {
		cleanKey = "event"
	}
	cleanKind := strings.Trim(replacer.Replace(strings.TrimSpace(kind)), "-")
	if cleanKind == "" {
		cleanKind = "system"
	}
	return fmt.Sprintf("msg-%s-%s", cleanKind, cleanKey)
}

func decodeSessionRequestPayload(raw json.RawMessage, out any) error {
	if len(raw) == 0 || strings.TrimSpace(string(raw)) == "" {
		return nil
	}
	return json.Unmarshal(raw, out)
}

func sessionStatusLabel(status SessionStatus) string {
	switch status {
	case SessionActive:
		return "active"
	case SessionSuspended:
		return "suspended"
	case SessionPersisted:
		return "persisted"
	default:
		return "unknown"
	}
}
