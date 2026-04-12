package sessionview

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/swm8023/wheelmaker/internal/hub/client"
	"github.com/swm8023/wheelmaker/internal/im"
	acp "github.com/swm8023/wheelmaker/internal/protocol"
)

type Runtime interface {
	CreateSession(ctx context.Context, title string) (*client.Session, error)
	SendToSession(ctx context.Context, sessionID string, source im.ChatRef, blocks []acp.ContentBlock) error
	ListSessions(ctx context.Context) ([]client.SessionListEntry, error)
}

type PublishFunc func(method string, payload any) error

type SessionSummary struct {
	SessionID    string `json:"sessionId"`
	Title        string `json:"title"`
	Preview      string `json:"preview"`
	UpdatedAt    string `json:"updatedAt"`
	MessageCount int    `json:"messageCount"`
	UnreadCount  int    `json:"unreadCount"`
	Agent        string `json:"agent,omitempty"`
	Status       string `json:"status,omitempty"`
}

type SessionMessage struct {
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

type Service struct {
	projectName string
	store       client.Store
	runtime     Runtime
	publish     PublishFunc
	aggregator  *aggregator

	mu          sync.Mutex
	unreadCount map[string]int
}

func New(projectName string, store client.Store, runtime Runtime) *Service {
	return &Service{
		projectName: strings.TrimSpace(projectName),
		store:       store,
		runtime:     runtime,
		aggregator:  newAggregator(),
		unreadCount: map[string]int{},
	}
}

func (s *Service) SetEventPublisher(publish PublishFunc) {
	s.publish = publish
}

func (s *Service) RecordEvent(ctx context.Context, event client.SessionViewEvent) error {
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
	case client.SessionViewEventSessionCreated:
		return s.upsertSessionProjection(ctx, event.SessionID, strings.TrimSpace(event.Title), "", event.UpdatedAt, false)
	case client.SessionViewEventUserMessageAccepted:
		if err := s.flushSession(ctx, event.SessionID); err != nil {
			return err
		}
		message := client.SessionMessageRecord{
			MessageID:    fmt.Sprintf("msg-user-%d", event.CreatedAt.UnixNano()),
			ProjectName:  s.projectName,
			SessionID:    event.SessionID,
			Role:         firstNonEmpty(event.Role, "user"),
			Kind:         firstNonEmpty(event.Kind, "text"),
			Body:         strings.TrimSpace(event.Text),
			Blocks:       cloneContentBlocks(event.Blocks),
			Status:       firstNonEmpty(event.Status, "done"),
			CreatedAt:    event.CreatedAt,
			UpdatedAt:    event.UpdatedAt,
			RequestID:    event.RequestID,
			AggregateKey: firstNonEmpty(event.AggregateKey, fmt.Sprintf("user:%s:%d", event.SessionID, event.CreatedAt.UnixNano())),
		}
		if err := s.store.AppendSessionMessage(ctx, message); err != nil {
			return err
		}
		storedMessage, err := s.loadStoredMessage(ctx, event.SessionID, message.MessageID)
		if err != nil {
			return err
		}
		message = storedMessage
		if err := s.upsertSessionProjection(ctx, event.SessionID, strings.TrimSpace(event.Title), message.Body, message.UpdatedAt, true); err != nil {
			return err
		}
		s.publishSessionMessage(message)
		return nil
	case client.SessionViewEventAssistantChunk, client.SessionViewEventThoughtChunk:
		message := s.aggregator.appendChunk(s.projectName, event)
		existed, err := s.store.HasSessionMessage(ctx, s.projectName, event.SessionID, message.MessageID)
		if err != nil {
			return err
		}
		if err := s.store.UpsertSessionMessage(ctx, message); err != nil {
			return err
		}
		storedMessage, err := s.loadStoredMessage(ctx, event.SessionID, message.MessageID)
		if err != nil {
			return err
		}
		message = storedMessage
		if err := s.upsertSessionProjection(ctx, event.SessionID, strings.TrimSpace(event.Title), message.Body, message.UpdatedAt, !existed); err != nil {
			return err
		}
		if !existed {
			s.incrementUnread(event.SessionID)
		}
		s.publishSessionMessage(message)
		return nil
	case client.SessionViewEventPromptFinished:
		return s.flushSession(ctx, event.SessionID)
	case client.SessionViewEventPermissionRequested:
		message := client.SessionMessageRecord{
			MessageID:    fmt.Sprintf("msg-permission-%s-%d", event.SessionID, event.RequestID),
			ProjectName:  s.projectName,
			SessionID:    event.SessionID,
			Role:         firstNonEmpty(event.Role, "system"),
			Kind:         firstNonEmpty(event.Kind, "permission"),
			Body:         strings.TrimSpace(event.Text),
			Options:      clonePermissionOptions(event.Options),
			Status:       firstNonEmpty(event.Status, "needs_action"),
			CreatedAt:    event.CreatedAt,
			UpdatedAt:    event.UpdatedAt,
			RequestID:    event.RequestID,
			AggregateKey: fmt.Sprintf("permission:%s:%d", event.SessionID, event.RequestID),
		}
		existed, err := s.store.HasSessionMessage(ctx, s.projectName, event.SessionID, message.MessageID)
		if err != nil {
			return err
		}
		if err := s.store.UpsertSessionMessage(ctx, message); err != nil {
			return err
		}
		storedMessage, err := s.loadStoredMessage(ctx, event.SessionID, message.MessageID)
		if err != nil {
			return err
		}
		message = storedMessage
		if err := s.upsertSessionProjection(ctx, event.SessionID, strings.TrimSpace(event.Title), message.Body, message.UpdatedAt, !existed); err != nil {
			return err
		}
		if !existed {
			s.incrementUnread(event.SessionID)
		}
		s.publishSessionMessage(message)
		return nil
	case client.SessionViewEventPermissionResolved:
		messages, err := s.store.ListSessionMessages(ctx, s.projectName, event.SessionID)
		if err != nil {
			return err
		}
		for _, message := range messages {
			if message.RequestID != event.RequestID {
				continue
			}
			message.Status = firstNonEmpty(event.Status, "done")
			message.UpdatedAt = event.UpdatedAt
			if err := s.store.UpsertSessionMessage(ctx, message); err != nil {
				return err
			}
			storedMessage, err := s.loadStoredMessage(ctx, event.SessionID, message.MessageID)
			if err != nil {
				return err
			}
			message = storedMessage
			s.publishSessionMessage(message)
			break
		}
		return nil
	case client.SessionViewEventSystemMessage, client.SessionViewEventToolUpdated:
		messageID := fmt.Sprintf("msg-system-%d", event.CreatedAt.UnixNano())
		if strings.TrimSpace(event.AggregateKey) != "" {
			messageID = sanitizeAggregateMessageID(event.Kind, event.AggregateKey)
		}
		message := client.SessionMessageRecord{
			MessageID:    messageID,
			ProjectName:  s.projectName,
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
		existed, err := s.store.HasSessionMessage(ctx, s.projectName, event.SessionID, message.MessageID)
		if err != nil {
			return err
		}
		if err := s.store.UpsertSessionMessage(ctx, message); err != nil {
			return err
		}
		storedMessage, err := s.loadStoredMessage(ctx, event.SessionID, message.MessageID)
		if err != nil {
			return err
		}
		message = storedMessage
		if err := s.upsertSessionProjection(ctx, event.SessionID, strings.TrimSpace(event.Title), message.Body, message.UpdatedAt, !existed); err != nil {
			return err
		}
		if !existed {
			s.incrementUnread(event.SessionID)
		}
		s.publishSessionMessage(message)
		return nil
	default:
		return nil
	}
}

func (s *Service) HandleSessionRequest(ctx context.Context, method string, _ string, payload json.RawMessage) (any, error) {
	switch strings.TrimSpace(method) {
	case "session.list":
		sessions, err := s.ListSessions(ctx)
		if err != nil {
			return nil, err
		}
		return map[string]any{"sessions": sessions}, nil
	case "session.read":
		var req struct {
			SessionID  string `json:"sessionId"`
			AfterIndex int64  `json:"afterIndex,omitempty"`
		}
		if err := decodePayload(payload, &req); err != nil {
			return nil, fmt.Errorf("invalid session.read payload: %w", err)
		}
		summary, messages, lastIndex, err := s.ReadSession(ctx, req.SessionID, req.AfterIndex)
		if err != nil {
			return nil, err
		}
		return map[string]any{"session": summary, "messages": messages, "lastIndex": lastIndex}, nil
	case "session.new":
		var req struct {
			Title string `json:"title,omitempty"`
		}
		if err := decodePayload(payload, &req); err != nil {
			return nil, fmt.Errorf("invalid session.new payload: %w", err)
		}
		sess, err := s.runtime.CreateSession(ctx, req.Title)
		if err != nil {
			return nil, err
		}
		if err := s.RecordEvent(ctx, client.SessionViewEvent{Type: client.SessionViewEventSessionCreated, SessionID: sess.ID, Title: firstNonEmpty(req.Title, sess.ID)}); err != nil {
			return nil, err
		}
		summary, _, _, err := s.ReadSession(ctx, sess.ID, 0)
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
		if err := decodePayload(payload, &req); err != nil {
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
		if err := s.runtime.SendToSession(ctx, req.SessionID, im.ChatRef{ChannelID: "app", ChatID: strings.TrimSpace(req.SessionID)}, blocks); err != nil {
			return nil, err
		}
		return map[string]any{"ok": true, "sessionId": strings.TrimSpace(req.SessionID)}, nil
	case "session.markRead":
		var req struct {
			SessionID string `json:"sessionId"`
		}
		if err := decodePayload(payload, &req); err != nil {
			return nil, fmt.Errorf("invalid session.markRead payload: %w", err)
		}
		s.mu.Lock()
		s.unreadCount[strings.TrimSpace(req.SessionID)] = 0
		s.mu.Unlock()
		if summary, ok := s.currentSummary(ctx, req.SessionID); ok {
			s.publishUpdated(summary)
		}
		return map[string]any{"ok": true}, nil
	default:
		return nil, fmt.Errorf("unsupported session method: %s", method)
	}
}

func (s *Service) ListSessions(ctx context.Context) ([]SessionSummary, error) {
	storeEntries, err := s.store.ListSessions(ctx, s.projectName)
	if err != nil {
		return nil, err
	}
	entries := storeEntries
	if s.runtime != nil {
		runtimeEntries, err := s.runtime.ListSessions(ctx)
		if err != nil {
			return nil, err
		}
		entries = runtimeEntries
		storeByID := make(map[string]client.SessionListEntry, len(storeEntries))
		for _, entry := range storeEntries {
			storeByID[entry.ID] = entry
		}
		for i := range entries {
			stored, ok := storeByID[entries[i].ID]
			if !ok {
				continue
			}
			if entries[i].Agent == "" {
				entries[i].Agent = stored.Agent
			}
			if entries[i].Title == "" {
				entries[i].Title = stored.Title
			}
			entries[i].Preview = stored.Preview
			entries[i].MessageCount = stored.MessageCount
			entries[i].LastMessageAt = stored.LastMessageAt
		}
	}
	out := make([]SessionSummary, 0, len(entries))
	for _, entry := range entries {
		out = append(out, s.summaryFromEntry(entry))
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].UpdatedAt > out[j].UpdatedAt
	})
	return out, nil
}

func (s *Service) ReadSession(ctx context.Context, sessionID string, afterIndex int64) (SessionSummary, []SessionMessage, int64, error) {
	rec, err := s.store.LoadSession(ctx, s.projectName, strings.TrimSpace(sessionID))
	if err != nil {
		return SessionSummary{}, nil, 0, err
	}
	if rec == nil {
		return SessionSummary{}, nil, 0, fmt.Errorf("session not found: %s", sessionID)
	}
	var messages []client.SessionMessageRecord
	if afterIndex > 0 {
		messages, err = s.store.ListSessionMessagesAfterIndex(ctx, s.projectName, strings.TrimSpace(sessionID), afterIndex)
	} else {
		messages, err = s.store.ListSessionMessages(ctx, s.projectName, strings.TrimSpace(sessionID))
	}
	if err != nil {
		return SessionSummary{}, nil, 0, err
	}
	out := make([]SessionMessage, 0, len(messages))
	for _, message := range messages {
		out = append(out, toSessionMessage(message))
	}
	return s.summaryFromRecord(*rec), out, rec.LastSyncIndex, nil
}

func (s *Service) flushSession(ctx context.Context, sessionID string) error {
	flushed := s.aggregator.flushSession(sessionID)
	for _, message := range flushed {
		existed, err := s.store.HasSessionMessage(ctx, s.projectName, sessionID, message.MessageID)
		if err != nil {
			return err
		}
		if err := s.store.UpsertSessionMessage(ctx, message); err != nil {
			return err
		}
		storedMessage, err := s.loadStoredMessage(ctx, sessionID, message.MessageID)
		if err != nil {
			return err
		}
		message = storedMessage
		if err := s.upsertSessionProjection(ctx, sessionID, "", message.Body, message.UpdatedAt, !existed); err != nil {
			return err
		}
		if !existed {
			s.incrementUnread(sessionID)
		}
		s.publishSessionMessage(message)
	}
	return nil
}

func (s *Service) loadStoredMessage(ctx context.Context, sessionID, messageID string) (client.SessionMessageRecord, error) {
	message, err := s.store.LoadSessionMessage(ctx, s.projectName, strings.TrimSpace(sessionID), strings.TrimSpace(messageID))
	if err != nil {
		return client.SessionMessageRecord{}, err
	}
	if message != nil {
		return *message, nil
	}
	return client.SessionMessageRecord{}, fmt.Errorf("session message not found: %s", messageID)
}

func (s *Service) upsertSessionProjection(ctx context.Context, sessionID, title, preview string, updatedAt time.Time, incrementMessageCount bool) error {
	rec, err := s.store.LoadSession(ctx, s.projectName, sessionID)
	if err != nil {
		return err
	}
	if rec == nil {
		rec = &client.SessionRecord{ID: sessionID, ProjectName: s.projectName, Status: client.SessionActive, CreatedAt: updatedAt, LastActiveAt: updatedAt}
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
	if err := s.store.SaveSession(ctx, rec); err != nil {
		return err
	}
	if summary, ok := s.currentSummary(ctx, sessionID); ok {
		s.publishUpdated(summary)
	}
	return nil
}

func (s *Service) currentSummary(ctx context.Context, sessionID string) (SessionSummary, bool) {
	rec, err := s.store.LoadSession(ctx, s.projectName, strings.TrimSpace(sessionID))
	if err != nil || rec == nil {
		return SessionSummary{}, false
	}
	return s.summaryFromRecord(*rec), true
}

func (s *Service) summaryFromEntry(entry client.SessionListEntry) SessionSummary {
	s.mu.Lock()
	unread := s.unreadCount[entry.ID]
	s.mu.Unlock()
	updatedAt := entry.LastMessageAt
	if updatedAt.IsZero() {
		updatedAt = entry.LastActiveAt
	}
	return SessionSummary{
		SessionID:    entry.ID,
		Title:        firstNonEmpty(entry.Title, entry.ID),
		Preview:      entry.Preview,
		UpdatedAt:    updatedAt.UTC().Format(time.RFC3339),
		MessageCount: entry.MessageCount,
		UnreadCount:  unread,
		Agent:        entry.Agent,
		Status:       formatStatus(entry.Status),
	}
}

func (s *Service) summaryFromRecord(rec client.SessionRecord) SessionSummary {
	s.mu.Lock()
	unread := s.unreadCount[rec.ID]
	s.mu.Unlock()
	updatedAt := rec.LastMessageAt
	if updatedAt.IsZero() {
		updatedAt = rec.LastActiveAt
	}
	return SessionSummary{
		SessionID:    rec.ID,
		Title:        firstNonEmpty(rec.Title, rec.ID),
		Preview:      rec.LastMessagePreview,
		UpdatedAt:    updatedAt.UTC().Format(time.RFC3339),
		MessageCount: rec.MessageCount,
		UnreadCount:  unread,
		Status:       formatStatus(rec.Status),
	}
}

func (s *Service) incrementUnread(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.unreadCount[strings.TrimSpace(sessionID)] += 1
}

func (s *Service) publishUpdated(summary SessionSummary) {
	if s.publish == nil {
		return
	}
	_ = s.publish("registry.session.updated", map[string]any{"session": summary})
}

func (s *Service) publishSessionMessage(message client.SessionMessageRecord) {
	if s.publish == nil {
		return
	}
	ctx := context.Background()
	summary, ok := s.currentSummary(ctx, message.SessionID)
	if !ok {
		summary = SessionSummary{SessionID: message.SessionID, Title: message.SessionID, Preview: message.Body, UpdatedAt: message.UpdatedAt.UTC().Format(time.RFC3339)}
	}
	_ = s.publish("registry.session.message", map[string]any{"session": summary, "message": toSessionMessage(message)})
}

func toSessionMessage(message client.SessionMessageRecord) SessionMessage {
	return SessionMessage{
		MessageID: message.MessageID,
		SessionID: message.SessionID,
		SyncIndex: message.SyncIndex,
		Role:      message.Role,
		Kind:      message.Kind,
		Text:      message.Body,
		Blocks:    cloneContentBlocks(message.Blocks),
		Options:   clonePermissionOptions(message.Options),
		Status:    message.Status,
		CreatedAt: message.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt: message.UpdatedAt.UTC().Format(time.RFC3339),
		RequestID: message.RequestID,
	}
}

func sanitizeAggregateMessageID(kind, aggregateKey string) string {
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

func cloneContentBlocks(blocks []acp.ContentBlock) []acp.ContentBlock {
	if len(blocks) == 0 {
		return nil
	}
	out := make([]acp.ContentBlock, len(blocks))
	copy(out, blocks)
	return out
}

func clonePermissionOptions(options []acp.PermissionOption) []acp.PermissionOption {
	if len(options) == 0 {
		return nil
	}
	out := make([]acp.PermissionOption, len(options))
	copy(out, options)
	return out
}

func decodePayload(raw json.RawMessage, out any) error {
	if len(raw) == 0 || strings.TrimSpace(string(raw)) == "" {
		return nil
	}
	return json.Unmarshal(raw, out)
}

func formatStatus(status client.SessionStatus) string {
	switch status {
	case client.SessionActive:
		return "active"
	case client.SessionSuspended:
		return "suspended"
	case client.SessionPersisted:
		return "persisted"
	default:
		return "unknown"
	}
}
