package client

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/swm8023/wheelmaker/internal/im"
	acp "github.com/swm8023/wheelmaker/internal/protocol"
)

type SessionViewEventType string

const (
	SessionViewEventSessionCreated      SessionViewEventType = "session_created"
	SessionViewEventUserMessageAccepted SessionViewEventType = "user_message_accepted"
	SessionViewEventAssistantChunk      SessionViewEventType = "assistant_chunk"
	SessionViewEventThoughtChunk        SessionViewEventType = "thought_chunk"
	SessionViewEventToolUpdated         SessionViewEventType = "tool_updated"
	SessionViewEventPermissionRequested SessionViewEventType = "permission_requested"
	SessionViewEventPermissionResolved  SessionViewEventType = "permission_resolved"
	SessionViewEventPromptFinished      SessionViewEventType = "prompt_finished"
	SessionViewEventSystemMessage       SessionViewEventType = "system_message"
)

type SessionViewEvent struct {
	Type          SessionViewEventType
	SessionID     string
	Title         string
	Role          string
	Kind          string
	Text          string
	Blocks        []acp.ContentBlock
	Options       []acp.PermissionOption
	Status        string
	AggregateKey  string
	RequestID     int64
	SourceChannel string
	SourceChatID  string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type SessionViewSink interface {
	RecordEvent(ctx context.Context, event SessionViewEvent) error
}

type sessionViewSummary struct {
	SessionID    string `json:"sessionId"`
	Title        string `json:"title"`
	Preview      string `json:"preview"`
	UpdatedAt    string `json:"updatedAt"`
	MessageCount int    `json:"messageCount"`
	UnreadCount  int    `json:"unreadCount"`
	Agent        string `json:"agent,omitempty"`
	Status       string `json:"status,omitempty"`
	ProjectName  string `json:"projectName,omitempty"`
}

type sessionViewMessage struct {
	MessageID string                 `json:"messageId"`
	SessionID string                 `json:"sessionId"`
	Index     int64                  `json:"index,omitempty"`
	SyncIndex int64                  `json:"syncIndex,omitempty"`
	SubIndex  int64                  `json:"subIndex,omitempty"`
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

type sessionProjectionAggregateState struct {
	message SessionMessageRecord
}

type sessionProjectionAggregator struct {
	mu     sync.Mutex
	nextID int64
	active map[string]*sessionProjectionAggregateState
}

func newSessionProjectionAggregator() *sessionProjectionAggregator {
	return &sessionProjectionAggregator{active: map[string]*sessionProjectionAggregateState{}}
}

func sessionProjectionAggregateKey(sessionID, kind string) string {
	return strings.TrimSpace(sessionID) + ":" + strings.TrimSpace(kind)
}

func (a *sessionProjectionAggregator) appendChunk(projectName string, event SessionViewEvent) SessionMessageRecord {
	a.mu.Lock()
	defer a.mu.Unlock()

	key := strings.TrimSpace(event.AggregateKey)
	if key == "" {
		key = sessionProjectionAggregateKey(event.SessionID, event.Kind)
	}
	state := a.active[key]
	now := event.UpdatedAt
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if state == nil {
		a.nextID += 1
		messageID := fmt.Sprintf("msg-agg-%d", a.nextID)
		state = &sessionProjectionAggregateState{message: SessionMessageRecord{
			MessageID:     messageID,
			ProjectName:   projectName,
			SessionID:     strings.TrimSpace(event.SessionID),
			Role:          strings.TrimSpace(event.Role),
			Kind:          strings.TrimSpace(event.Kind),
			Status:        firstNonEmpty(strings.TrimSpace(event.Status), "streaming"),
			AggregateKey:  key,
			SourceChannel: strings.TrimSpace(event.SourceChannel),
			SourceChatID:  strings.TrimSpace(event.SourceChatID),
			RequestID:     event.RequestID,
			CreatedAt:     now,
			UpdatedAt:     now,
		}}
		a.active[key] = state
	}
	state.message.Body += event.Text
	state.message.UpdatedAt = now
	state.message.Status = firstNonEmpty(strings.TrimSpace(event.Status), state.message.Status)
	if state.message.Role == "" {
		state.message.Role = strings.TrimSpace(event.Role)
	}
	if state.message.Kind == "" {
		state.message.Kind = strings.TrimSpace(event.Kind)
	}
	return state.message
}

func (a *sessionProjectionAggregator) flushSession(sessionID string) []SessionMessageRecord {
	a.mu.Lock()
	defer a.mu.Unlock()
	var out []SessionMessageRecord
	for key, state := range a.active {
		if state == nil || strings.TrimSpace(state.message.SessionID) != strings.TrimSpace(sessionID) {
			continue
		}
		state.message.Status = "done"
		state.message.UpdatedAt = time.Now().UTC()
		out = append(out, state.message)
		delete(a.active, key)
	}
	return out
}

type SessionRecorder struct {
	projectName  string
	store        Store
	listSessions func(context.Context) ([]SessionListEntry, error)

	mu              sync.Mutex
	publish         func(method string, payload any) error
	unreadCount     map[string]int
	chunkAggregator *sessionProjectionAggregator
}

func newSessionRecorder(projectName string, store Store, listSessions func(context.Context) ([]SessionListEntry, error)) *SessionRecorder {
	return &SessionRecorder{
		projectName:      projectName,
		store:            store,
		listSessions:     listSessions,
		unreadCount:      map[string]int{},
		chunkAggregator:  newSessionProjectionAggregator(),
	}
}

func (r *SessionRecorder) SetEventPublisher(publish func(method string, payload any) error) {
	r.mu.Lock()
	r.publish = publish
	r.mu.Unlock()
}

func (r *SessionRecorder) eventPublisher() func(method string, payload any) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.publish
}

func (r *SessionRecorder) RecordEvent(ctx context.Context, event SessionViewEvent) error {
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
		return r.upsertSessionProjection(ctx, event.SessionID, strings.TrimSpace(event.Title), "", event.UpdatedAt, false)
	case SessionViewEventUserMessageAccepted:
		if err := r.flushSessionProjection(ctx, event.SessionID); err != nil {
			return err
		}
		message := SessionMessageRecord{
			MessageID:    fmt.Sprintf("msg-user-%d", event.CreatedAt.UnixNano()),
			ProjectName:  r.projectName,
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
		if err := r.store.AppendSessionMessage(ctx, message); err != nil {
			return err
		}
		stored, err := r.loadStoredSessionMessage(ctx, event.SessionID, message.MessageID)
		if err != nil {
			return err
		}
		if err := r.upsertSessionProjection(ctx, event.SessionID, strings.TrimSpace(event.Title), stored.Body, stored.UpdatedAt, true); err != nil {
			return err
		}
		r.publishSessionMessage(stored)
		return nil
	case SessionViewEventAssistantChunk, SessionViewEventThoughtChunk:
		message := r.chunkAggregator.appendChunk(r.projectName, event)
		existed, err := r.store.HasSessionMessage(ctx, r.projectName, event.SessionID, message.MessageID)
		if err != nil {
			return err
		}
		if err := r.store.UpsertSessionMessage(ctx, message); err != nil {
			return err
		}
		stored, err := r.loadStoredSessionMessage(ctx, event.SessionID, message.MessageID)
		if err != nil {
			return err
		}
		if err := r.upsertSessionProjection(ctx, event.SessionID, strings.TrimSpace(event.Title), stored.Body, stored.UpdatedAt, !existed); err != nil {
			return err
		}
		if !existed {
			r.incrementSessionUnread(event.SessionID)
		}
		r.publishSessionMessage(stored)
		return nil
	case SessionViewEventPromptFinished:
		return r.flushSessionProjection(ctx, event.SessionID)
	case SessionViewEventPermissionRequested:
		message := SessionMessageRecord{
			MessageID:    fmt.Sprintf("msg-permission-%s-%d", event.SessionID, event.RequestID),
			ProjectName:  r.projectName,
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
		existed, err := r.store.HasSessionMessage(ctx, r.projectName, event.SessionID, message.MessageID)
		if err != nil {
			return err
		}
		if err := r.store.UpsertSessionMessage(ctx, message); err != nil {
			return err
		}
		stored, err := r.loadStoredSessionMessage(ctx, event.SessionID, message.MessageID)
		if err != nil {
			return err
		}
		if err := r.upsertSessionProjection(ctx, event.SessionID, strings.TrimSpace(event.Title), stored.Body, stored.UpdatedAt, !existed); err != nil {
			return err
		}
		if !existed {
			r.incrementSessionUnread(event.SessionID)
		}
		r.publishSessionMessage(stored)
		return nil
	case SessionViewEventPermissionResolved:
		messages, err := r.store.ListSessionMessages(ctx, r.projectName, event.SessionID)
		if err != nil {
			return err
		}
		for _, message := range messages {
			if message.RequestID != event.RequestID {
				continue
			}
			message.Status = firstNonEmpty(event.Status, "done")
			message.UpdatedAt = event.UpdatedAt
			if err := r.store.UpsertSessionMessage(ctx, message); err != nil {
				return err
			}
			stored, err := r.loadStoredSessionMessage(ctx, event.SessionID, message.MessageID)
			if err != nil {
				return err
			}
			r.publishSessionMessage(stored)
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
			ProjectName:  r.projectName,
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
		existed, err := r.store.HasSessionMessage(ctx, r.projectName, event.SessionID, message.MessageID)
		if err != nil {
			return err
		}
		if err := r.store.UpsertSessionMessage(ctx, message); err != nil {
			return err
		}
		stored, err := r.loadStoredSessionMessage(ctx, event.SessionID, message.MessageID)
		if err != nil {
			return err
		}
		if err := r.upsertSessionProjection(ctx, event.SessionID, strings.TrimSpace(event.Title), stored.Body, stored.UpdatedAt, !existed); err != nil {
			return err
		}
		if !existed {
			r.incrementSessionUnread(event.SessionID)
		}
		r.publishSessionMessage(stored)
		return nil
	default:
		return nil
	}
}

func (r *SessionRecorder) ListSessionViews(ctx context.Context) ([]sessionViewSummary, error) {
	entries, err := r.listSessions(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]sessionViewSummary, 0, len(entries))
	for _, entry := range entries {
		out = append(out, r.sessionViewSummaryFromEntry(entry))
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].UpdatedAt > out[j].UpdatedAt
	})
	return out, nil
}

func (r *SessionRecorder) ReadSessionView(ctx context.Context, sessionID string, afterIndex, afterSubIndex int64) (sessionViewSummary, []sessionViewMessage, int64, int64, error) {
	rec, err := r.store.LoadSession(ctx, r.projectName, strings.TrimSpace(sessionID))
	if err != nil {
		return sessionViewSummary{}, nil, 0, 0, err
	}
	if rec == nil {
		return sessionViewSummary{}, nil, 0, 0, fmt.Errorf("session not found: %s", sessionID)
	}
	var messages []SessionMessageRecord
	if afterIndex > 0 || afterSubIndex > 0 {
		messages, err = r.store.ListSessionMessagesAfterCursor(ctx, r.projectName, strings.TrimSpace(sessionID), afterIndex, afterSubIndex)
	} else {
		messages, err = r.store.ListSessionMessages(ctx, r.projectName, strings.TrimSpace(sessionID))
	}
	if err != nil {
		return sessionViewSummary{}, nil, 0, 0, err
	}
	out := make([]sessionViewMessage, 0, len(messages))
	for _, message := range messages {
		out = append(out, toSessionViewMessage(message))
	}
	return r.sessionViewSummaryFromRecord(*rec), out, rec.LastSyncIndex, rec.LastSyncSubIndex, nil
}

func (r *SessionRecorder) MarkSessionRead(ctx context.Context, sessionID string) (sessionViewSummary, bool) {
	r.resetSessionUnread(strings.TrimSpace(sessionID))
	return r.currentSessionViewSummary(ctx, sessionID)
}

func (r *SessionRecorder) flushSessionProjection(ctx context.Context, sessionID string) error {
	flushed := r.chunkAggregator.flushSession(sessionID)
	for _, message := range flushed {
		existed, err := r.store.HasSessionMessage(ctx, r.projectName, sessionID, message.MessageID)
		if err != nil {
			return err
		}
		if err := r.store.UpsertSessionMessage(ctx, message); err != nil {
			return err
		}
		stored, err := r.loadStoredSessionMessage(ctx, sessionID, message.MessageID)
		if err != nil {
			return err
		}
		if err := r.upsertSessionProjection(ctx, sessionID, "", stored.Body, stored.UpdatedAt, !existed); err != nil {
			return err
		}
		if !existed {
			r.incrementSessionUnread(sessionID)
		}
		r.publishSessionMessage(stored)
	}
	return nil
}

func (r *SessionRecorder) loadStoredSessionMessage(ctx context.Context, sessionID, messageID string) (SessionMessageRecord, error) {
	message, err := r.store.LoadSessionMessage(ctx, r.projectName, strings.TrimSpace(sessionID), strings.TrimSpace(messageID))
	if err != nil {
		return SessionMessageRecord{}, err
	}
	if message != nil {
		return *message, nil
	}
	return SessionMessageRecord{}, fmt.Errorf("session message not found: %s", messageID)
}

func (r *SessionRecorder) upsertSessionProjection(ctx context.Context, sessionID, title, preview string, updatedAt time.Time, incrementMessageCount bool) error {
	rec, err := r.store.LoadSession(ctx, r.projectName, sessionID)
	if err != nil {
		return err
	}
	if rec == nil {
		rec = &SessionRecord{ID: sessionID, ProjectName: r.projectName, Status: SessionActive, CreatedAt: updatedAt, LastActiveAt: updatedAt}
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
	if err := r.store.SaveSession(ctx, rec); err != nil {
		return err
	}
	if summary, ok := r.currentSessionViewSummary(ctx, sessionID); ok {
		r.publishSessionUpdated(summary)
	}
	return nil
}

func (r *SessionRecorder) currentSessionViewSummary(ctx context.Context, sessionID string) (sessionViewSummary, bool) {
	rec, err := r.store.LoadSession(ctx, r.projectName, strings.TrimSpace(sessionID))
	if err != nil || rec == nil {
		return sessionViewSummary{}, false
	}
	return r.sessionViewSummaryFromRecord(*rec), true
}

func (r *SessionRecorder) sessionViewSummaryFromEntry(entry SessionListEntry) sessionViewSummary {
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
		UnreadCount:  r.sessionUnread(entry.ID),
		Agent:        entry.Agent,
		Status:       sessionStatusLabel(entry.Status),
		ProjectName:  entry.ProjectName,
	}
}

func (r *SessionRecorder) sessionViewSummaryFromRecord(rec SessionRecord) sessionViewSummary {
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
		UnreadCount:  r.sessionUnread(rec.ID),
		Status:       sessionStatusLabel(rec.Status),
		ProjectName:  rec.ProjectName,
	}
}

func (r *SessionRecorder) incrementSessionUnread(sessionID string) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}
	r.mu.Lock()
	r.unreadCount[sessionID] += 1
	r.mu.Unlock()
}

func (r *SessionRecorder) resetSessionUnread(sessionID string) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}
	r.mu.Lock()
	r.unreadCount[sessionID] = 0
	r.mu.Unlock()
}

func (r *SessionRecorder) sessionUnread(sessionID string) int {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.unreadCount[sessionID]
}

func (r *SessionRecorder) publishSessionUpdated(summary sessionViewSummary) {
	publish := r.eventPublisher()
	if publish == nil {
		return
	}
	_ = publish("registry.session.updated", map[string]any{"session": summary})
}

func (r *SessionRecorder) publishSessionMessage(message SessionMessageRecord) {
	publish := r.eventPublisher()
	if publish == nil {
		return
	}
	ctx := context.Background()
	summary, ok := r.currentSessionViewSummary(ctx, message.SessionID)
	if !ok {
		summary = sessionViewSummary{SessionID: message.SessionID, Title: message.SessionID, Preview: message.Body, UpdatedAt: message.UpdatedAt.UTC().Format(time.RFC3339), ProjectName: message.ProjectName}
	}
	_ = publish("registry.session.message", map[string]any{"session": summary, "message": toSessionViewMessage(message)})
}

func (c *Client) SetSessionViewSink(sink SessionViewSink) {
	if sink == nil {
		sink = c.sessionRecorder
	}
	c.mu.Lock()
	c.viewSink = sink
	for _, sess := range c.sessions {
		sess.viewSink = sink
	}
	c.mu.Unlock()
}

func (c *Client) SetSessionEventPublisher(publish func(method string, payload any) error) {
	c.sessionRecorder.SetEventPublisher(publish)
}

func (c *Client) RecordEvent(ctx context.Context, event SessionViewEvent) error {
	return c.sessionRecorder.RecordEvent(ctx, event)
}

func (c *Client) HandleSessionRequest(ctx context.Context, method string, _ string, payload json.RawMessage) (any, error) {
	switch strings.TrimSpace(method) {
	case "session.list":
		sessions, err := c.sessionRecorder.ListSessionViews(ctx)
		if err != nil {
			return nil, err
		}
		return map[string]any{"sessions": sessions}, nil
	case "session.read":
		var req struct {
			SessionID     string `json:"sessionId"`
			AfterIndex    int64  `json:"afterIndex,omitempty"`
			AfterSubIndex int64  `json:"afterSubIndex,omitempty"`
		}
		if err := decodeSessionRequestPayload(payload, &req); err != nil {
			return nil, fmt.Errorf("invalid session.read payload: %w", err)
		}
		summary, messages, lastIndex, lastSubIndex, err := c.sessionRecorder.ReadSessionView(ctx, req.SessionID, req.AfterIndex, req.AfterSubIndex)
		if err != nil {
			return nil, err
		}
		return map[string]any{"session": summary, "messages": messages, "lastIndex": lastIndex, "lastSubIndex": lastSubIndex}, nil
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
		summary, _, _, _, err := c.sessionRecorder.ReadSessionView(ctx, sess.ID, 0, 0)
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
		if summary, ok := c.sessionRecorder.MarkSessionRead(ctx, strings.TrimSpace(req.SessionID)); ok {
			c.sessionRecorder.publishSessionUpdated(summary)
		}
		return map[string]any{"ok": true}, nil
	default:
		return nil, fmt.Errorf("unsupported session method: %s", method)
	}
}

func (c *Client) listSessionViews(ctx context.Context) ([]sessionViewSummary, error) {
	return c.sessionRecorder.ListSessionViews(ctx)
}

func (c *Client) readSessionView(ctx context.Context, sessionID string, afterIndex int64) (sessionViewSummary, []sessionViewMessage, int64, error) {
	summary, messages, lastIndex, _, err := c.sessionRecorder.ReadSessionView(ctx, sessionID, afterIndex, 0)
	return summary, messages, lastIndex, err
}

func toSessionViewMessage(message SessionMessageRecord) sessionViewMessage {
	return sessionViewMessage{
		MessageID: message.MessageID,
		SessionID: message.SessionID,
		Index:     message.SyncIndex,
		SyncIndex: message.SyncIndex,
		SubIndex:  message.SyncSubIndex,
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
