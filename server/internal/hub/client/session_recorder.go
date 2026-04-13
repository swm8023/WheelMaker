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
	SessionID   string `json:"sessionId"`
	Title       string `json:"title"`
	UpdatedAt   string `json:"updatedAt"`
	UnreadCount int    `json:"unreadCount"`
	Agent       string `json:"agent,omitempty"`
	Status      string `json:"status,omitempty"`
	ProjectName string `json:"projectName,omitempty"`
}
type sessionViewMessage struct {
	MessageID string                 `json:"messageId"`
	SessionID string                 `json:"sessionId"`
	Index     int64                  `json:"index,omitempty"`
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

const sessionUpdateFlushInterval = 5 * time.Second

type bufferedSessionUpdate struct {
	message         SessionMessageRecord
	variant         string
	persisted       bool
	dirty           bool
	lastPersistedAt time.Time
}

type SessionRecorder struct {
	projectName  string
	store        Store
	listSessions func(context.Context) ([]SessionListEntry, error)

	mu          sync.Mutex
	publish     func(method string, payload any) error
	unreadCount map[string]int

	updateMu sync.Mutex
	updates  map[string]*bufferedSessionUpdate
	stopCh   chan struct{}
	doneCh   chan struct{}
}

func newSessionRecorder(projectName string, store Store, listSessions func(context.Context) ([]SessionListEntry, error)) *SessionRecorder {
	r := &SessionRecorder{
		projectName:  projectName,
		store:        store,
		listSessions: listSessions,
		unreadCount:  map[string]int{},
		updates:      map[string]*bufferedSessionUpdate{},
		stopCh:       make(chan struct{}),
		doneCh:       make(chan struct{}),
	}
	go r.runFlushLoop()
	return r
}

func (r *SessionRecorder) Close() {
	if r == nil {
		return
	}
	select {
	case <-r.stopCh:
		return
	default:
		close(r.stopCh)
	}
	<-r.doneCh
}

func (r *SessionRecorder) runFlushLoop() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	defer close(r.doneCh)
	for {
		select {
		case <-r.stopCh:
			r.flushAllBufferedUpdates(context.Background())
			return
		case <-ticker.C:
			r.flushDueBufferedUpdates(context.Background(), time.Now().UTC())
		}
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
		return r.upsertSessionProjection(ctx, event.SessionID, strings.TrimSpace(event.Title), event.UpdatedAt)
	case SessionViewEventUserMessageAccepted:
		if err := r.flushBufferedSessionUpdate(ctx, event.SessionID); err != nil {
			return err
		}
		message := SessionMessageRecord{
			MessageID:    fmt.Sprintf("msg-user-%d", event.CreatedAt.UnixNano()),
			ProjectName:  r.projectName,
			SessionID:    event.SessionID,
			Method:       "session.prompt",
			Role:         firstNonEmpty(event.Role, "user"),
			Kind:         firstNonEmpty(event.Kind, "text"),
			Body:         strings.TrimSpace(event.Text),
			Blocks:       cloneSessionContentBlocks(event.Blocks),
			Status:       firstNonEmpty(event.Status, "done"),
			CreatedAt:    event.CreatedAt,
			UpdatedAt:    event.UpdatedAt,
			EventTime:    event.UpdatedAt,
			RequestID:    event.RequestID,
			AggregateKey: firstNonEmpty(event.AggregateKey, fmt.Sprintf("user:%s:%d", event.SessionID, event.CreatedAt.UnixNano())),
			Source:       normalizeRecorderEventSource(event),
		}
		if err := r.store.AppendSessionMessage(ctx, message); err != nil {
			return err
		}
		stored, err := r.loadStoredSessionMessage(ctx, event.SessionID, message.MessageID)
		if err != nil {
			return err
		}
		if err := r.upsertSessionProjection(ctx, event.SessionID, strings.TrimSpace(event.Title), stored.UpdatedAt); err != nil {
			return err
		}
		r.publishSessionMessage(stored)
		return nil
	case SessionViewEventAssistantChunk, SessionViewEventThoughtChunk, SessionViewEventToolUpdated, SessionViewEventSystemMessage:
		return r.recordBufferedSessionUpdate(ctx, event)
	case SessionViewEventPromptFinished:
		return r.flushBufferedSessionUpdate(ctx, event.SessionID)
	case SessionViewEventPermissionRequested:
		if err := r.flushBufferedSessionUpdate(ctx, event.SessionID); err != nil {
			return err
		}
		message := SessionMessageRecord{
			MessageID:    fmt.Sprintf("msg-permission-%s-%d", event.SessionID, event.RequestID),
			ProjectName:  r.projectName,
			SessionID:    event.SessionID,
			Method:       "session.permission",
			Role:         firstNonEmpty(event.Role, "system"),
			Kind:         firstNonEmpty(event.Kind, "permission"),
			Body:         strings.TrimSpace(event.Text),
			Options:      cloneSessionPermissionOptions(event.Options),
			Status:       firstNonEmpty(event.Status, "needs_action"),
			CreatedAt:    event.CreatedAt,
			UpdatedAt:    event.UpdatedAt,
			EventTime:    event.UpdatedAt,
			RequestID:    event.RequestID,
			AggregateKey: fmt.Sprintf("permission:%s:%d", event.SessionID, event.RequestID),
			Source:       normalizeRecorderEventSource(event),
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
		if err := r.upsertSessionProjection(ctx, event.SessionID, strings.TrimSpace(event.Title), stored.UpdatedAt); err != nil {
			return err
		}
		if !existed {
			r.incrementSessionUnread(event.SessionID)
		}
		r.publishSessionMessage(stored)
		return nil
	case SessionViewEventPermissionResolved:
		if err := r.flushBufferedSessionUpdate(ctx, event.SessionID); err != nil {
			return err
		}
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
			message.EventTime = event.UpdatedAt
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
	default:
		return nil
	}
}

func normalizeRecorderEventSource(event SessionViewEvent) string {
	channel := strings.TrimSpace(event.SourceChannel)
	chatID := strings.TrimSpace(event.SourceChatID)
	if channel == "" && chatID == "" {
		return ""
	}
	if channel == "" {
		return chatID
	}
	if chatID == "" {
		return channel
	}
	return channel + ":" + chatID
}

func recorderUpdateVariant(event SessionViewEvent) string {
	variant := string(event.Type)
	if strings.TrimSpace(event.Kind) != "" {
		variant += ":" + strings.TrimSpace(event.Kind)
	}
	if strings.TrimSpace(event.AggregateKey) != "" {
		variant += ":" + strings.TrimSpace(event.AggregateKey)
	}
	return variant
}

func newBufferedSessionUpdateMessage(projectName string, event SessionViewEvent) SessionMessageRecord {
	return SessionMessageRecord{
		MessageID:     fmt.Sprintf("msg-update-%s-%d", strings.TrimSpace(event.SessionID), event.CreatedAt.UnixNano()),
		ProjectName:   projectName,
		SessionID:     strings.TrimSpace(event.SessionID),
		Method:        "session.update",
		Role:          firstNonEmpty(strings.TrimSpace(event.Role), "assistant"),
		Kind:          firstNonEmpty(strings.TrimSpace(event.Kind), "text"),
		Body:          strings.TrimSpace(event.Text),
		Status:        firstNonEmpty(strings.TrimSpace(event.Status), "streaming"),
		AggregateKey:  strings.TrimSpace(event.AggregateKey),
		RequestID:     event.RequestID,
		Source:        normalizeRecorderEventSource(event),
		SourceChannel: strings.TrimSpace(event.SourceChannel),
		SourceChatID:  strings.TrimSpace(event.SourceChatID),
		CreatedAt:     event.CreatedAt,
		UpdatedAt:     event.UpdatedAt,
		EventTime:     event.UpdatedAt,
	}
}

func mergeBufferedSessionUpdateMessage(msg *SessionMessageRecord, event SessionViewEvent) {
	if msg == nil {
		return
	}
	rawText := event.Text
	switch event.Type {
	case SessionViewEventAssistantChunk, SessionViewEventThoughtChunk:
		msg.Body += rawText
	default:
		msg.Body = strings.TrimSpace(rawText)
	}
	msg.Role = firstNonEmpty(strings.TrimSpace(msg.Role), strings.TrimSpace(event.Role))
	msg.Kind = firstNonEmpty(strings.TrimSpace(event.Kind), msg.Kind)
	msg.Status = firstNonEmpty(strings.TrimSpace(event.Status), msg.Status)
	msg.RequestID = firstNonZeroInt64(event.RequestID, msg.RequestID)
	if strings.TrimSpace(event.AggregateKey) != "" {
		msg.AggregateKey = strings.TrimSpace(event.AggregateKey)
	}
	if strings.TrimSpace(event.SourceChannel) != "" || strings.TrimSpace(event.SourceChatID) != "" {
		msg.Source = normalizeRecorderEventSource(event)
		msg.SourceChannel = strings.TrimSpace(event.SourceChannel)
		msg.SourceChatID = strings.TrimSpace(event.SourceChatID)
	}
	if event.CreatedAt.Before(msg.CreatedAt) {
		msg.CreatedAt = event.CreatedAt
	}
	msg.UpdatedAt = event.UpdatedAt
	msg.EventTime = event.UpdatedAt
}

func firstNonZeroInt64(v int64, fallback int64) int64 {
	if v != 0 {
		return v
	}
	return fallback
}

func (r *SessionRecorder) recordBufferedSessionUpdate(ctx context.Context, event SessionViewEvent) error {
	r.updateMu.Lock()
	defer r.updateMu.Unlock()

	sessionID := strings.TrimSpace(event.SessionID)
	variant := recorderUpdateVariant(event)
	state := r.updates[sessionID]
	if state != nil && state.variant != variant {
		if err := r.flushBufferedSessionUpdateLocked(ctx, sessionID, true); err != nil {
			return err
		}
		state = nil
	}
	if state == nil {
		msg := newBufferedSessionUpdateMessage(r.projectName, event)
		state = &bufferedSessionUpdate{message: msg, variant: variant, dirty: true}
		r.updates[sessionID] = state
		if err := r.persistBufferedSessionUpdateLocked(ctx, sessionID, state, true); err != nil {
			return err
		}
		return nil
	}
	mergeBufferedSessionUpdateMessage(&state.message, event)
	state.dirty = true
	if state.lastPersistedAt.IsZero() || event.UpdatedAt.Sub(state.lastPersistedAt) >= sessionUpdateFlushInterval {
		if err := r.persistBufferedSessionUpdateLocked(ctx, sessionID, state, false); err != nil {
			return err
		}
	}
	return nil
}

func (r *SessionRecorder) flushBufferedSessionUpdate(ctx context.Context, sessionID string) error {
	r.updateMu.Lock()
	defer r.updateMu.Unlock()
	return r.flushBufferedSessionUpdateLocked(ctx, strings.TrimSpace(sessionID), true)
}

func (r *SessionRecorder) flushBufferedSessionUpdateLocked(ctx context.Context, sessionID string, force bool) error {
	state := r.updates[strings.TrimSpace(sessionID)]
	if state == nil {
		return nil
	}
	if !state.persisted {
		return r.persistBufferedSessionUpdateLocked(ctx, sessionID, state, true)
	}
	if !state.dirty && !force {
		return nil
	}
	return r.persistBufferedSessionUpdateLocked(ctx, sessionID, state, false)
}

func (r *SessionRecorder) persistBufferedSessionUpdateLocked(ctx context.Context, sessionID string, state *bufferedSessionUpdate, appendMode bool) error {
	if state == nil {
		return nil
	}
	msg := state.message
	if appendMode {
		if err := r.store.AppendSessionMessage(ctx, msg); err != nil {
			return err
		}
	} else {
		if err := r.store.UpsertSessionMessage(ctx, msg); err != nil {
			return err
		}
	}
	stored, err := r.loadStoredSessionMessage(ctx, sessionID, msg.MessageID)
	if err != nil {
		return err
	}
	if err := r.upsertSessionProjection(ctx, sessionID, "", stored.UpdatedAt); err != nil {
		return err
	}
	if appendMode {
		r.incrementSessionUnread(sessionID)
	}
	r.publishSessionMessage(stored)
	state.message = stored
	state.persisted = true
	state.dirty = false
	state.lastPersistedAt = stored.UpdatedAt
	r.updates[sessionID] = state
	return nil
}

func (r *SessionRecorder) flushDueBufferedUpdates(ctx context.Context, now time.Time) {
	r.updateMu.Lock()
	defer r.updateMu.Unlock()
	for sessionID, state := range r.updates {
		if state == nil || !state.persisted || !state.dirty {
			continue
		}
		if now.Sub(state.lastPersistedAt) < sessionUpdateFlushInterval {
			continue
		}
		_ = r.persistBufferedSessionUpdateLocked(ctx, sessionID, state, false)
	}
}

func (r *SessionRecorder) flushAllBufferedUpdates(ctx context.Context) {
	r.updateMu.Lock()
	defer r.updateMu.Unlock()
	for sessionID, state := range r.updates {
		if state == nil {
			continue
		}
		if state.persisted && !state.dirty {
			continue
		}
		_ = r.persistBufferedSessionUpdateLocked(ctx, sessionID, state, !state.persisted)
	}
}
func (r *SessionRecorder) ListSessionViews(ctx context.Context) ([]sessionViewSummary, error) {
	entries, err := r.listSessions(ctx)
	if err != nil {
		return nil, err
	}
	r.pruneUnreadCounts(entries)
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

func (r *SessionRecorder) upsertSessionProjection(ctx context.Context, sessionID, title string, updatedAt time.Time) error {
	rec, err := r.store.LoadSession(ctx, r.projectName, sessionID)
	if err != nil {
		return err
	}
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}
	if rec == nil {
		rec = &SessionRecord{ID: sessionID, ProjectName: r.projectName, Status: SessionActive, CreatedAt: updatedAt, LastActiveAt: updatedAt}
	}
	if strings.TrimSpace(title) != "" {
		rec.Title = strings.TrimSpace(title)
	}
	rec.LastActiveAt = updatedAt
	if rec.LastMessageAt.IsZero() || updatedAt.After(rec.LastMessageAt) {
		rec.LastMessageAt = updatedAt
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
		r.resetSessionUnread(sessionID)
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
		SessionID:   entry.ID,
		Title:       firstNonEmpty(entry.Title, entry.ID),
		UpdatedAt:   updatedAt.UTC().Format(time.RFC3339),
		UnreadCount: r.sessionUnread(entry.ID),
		Agent:       entry.Agent,
		Status:      sessionStatusLabel(entry.Status),
		ProjectName: entry.ProjectName,
	}
}

func (r *SessionRecorder) sessionViewSummaryFromRecord(rec SessionRecord) sessionViewSummary {
	updatedAt := rec.LastMessageAt
	if updatedAt.IsZero() {
		updatedAt = rec.LastActiveAt
	}
	return sessionViewSummary{
		SessionID:   rec.ID,
		Title:       firstNonEmpty(rec.Title, rec.ID),
		UpdatedAt:   updatedAt.UTC().Format(time.RFC3339),
		UnreadCount: r.sessionUnread(rec.ID),
		Status:      sessionStatusLabel(rec.Status),
		ProjectName: rec.ProjectName,
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
	delete(r.unreadCount, sessionID)
	r.mu.Unlock()
}

func (r *SessionRecorder) pruneUnreadCounts(entries []SessionListEntry) {
	activeSessionIDs := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		sessionID := strings.TrimSpace(entry.ID)
		if sessionID != "" {
			activeSessionIDs[sessionID] = struct{}{}
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for sessionID, count := range r.unreadCount {
		if count <= 0 {
			delete(r.unreadCount, sessionID)
			continue
		}
		if _, ok := activeSessionIDs[sessionID]; !ok {
			delete(r.unreadCount, sessionID)
		}
	}
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
		summary = sessionViewSummary{SessionID: message.SessionID, Title: message.SessionID, UpdatedAt: message.UpdatedAt.UTC().Format(time.RFC3339), ProjectName: message.ProjectName}
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
