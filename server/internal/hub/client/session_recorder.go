package client

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	acp "github.com/swm8023/wheelmaker/internal/protocol"
)

type SessionViewEventType string

const (
	SessionViewEventTypeSystem SessionViewEventType = "system"
	SessionViewEventTypeACP    SessionViewEventType = "acp"
)

const sessionViewMethodSystem = "system"

type SessionViewEvent struct {
	Type      SessionViewEventType
	SessionID string
	Content   string

	SourceChannel string
	SourceChatID  string
	UpdatedAt     time.Time
}

type sessionViewSessionNewParams struct {
	SessionID string `json:"sessionId,omitempty"`
	Title     string `json:"title,omitempty"`
}

type sessionViewPromptResult struct {
	StopReason string `json:"stopReason,omitempty"`
}

type sessionViewACPContentDoc struct {
	ID     int64           `json:"id,omitempty"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
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
type sessionViewTurn struct {
	TurnID      string                 `json:"turnId"`
	PromptIndex int64                  `json:"promptIndex"`
	TurnIndex   int64                  `json:"turnIndex"`
	UpdateIndex int64                  `json:"updateIndex"`
	Role        string                 `json:"role,omitempty"`
	Kind        string                 `json:"kind,omitempty"`
	Text        string                 `json:"text,omitempty"`
	Status      string                 `json:"status,omitempty"`
	RequestID   int64                  `json:"requestId,omitempty"`
	ToolCallID  string                 `json:"toolCallId,omitempty"`
	Blocks      []acp.ContentBlock     `json:"blocks,omitempty"`
	Options     []acp.PermissionOption `json:"options,omitempty"`
	Update      json.RawMessage        `json:"update,omitempty"`
	Extra       json.RawMessage        `json:"extra,omitempty"`
}

type sessionViewPrompt struct {
	MessageID   string            `json:"messageId"`
	PromptID    string            `json:"promptId"`
	SessionID   string            `json:"sessionId"`
	PromptIndex int64             `json:"promptIndex"`
	UpdateIndex int64             `json:"updateIndex"`
	Title       string            `json:"title"`
	StopReason  string            `json:"stopReason,omitempty"`
	Status      string            `json:"status"`
	UpdatedAt   string            `json:"updatedAt"`
	Turns       []sessionViewTurn `json:"turns"`
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
	message         SessionTurnMessageRecord
	turnKey         string
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
	updates  map[string]map[string]*bufferedSessionUpdate
	stopCh   chan struct{}
	doneCh   chan struct{}
}

func newSessionRecorder(projectName string, store Store, listSessions func(context.Context) ([]SessionListEntry, error)) *SessionRecorder {
	r := &SessionRecorder{
		projectName:  projectName,
		store:        store,
		listSessions: listSessions,
		unreadCount:  map[string]int{},
		updates:      map[string]map[string]*bufferedSessionUpdate{},
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

func decodeSessionViewACPContentDoc(content string) (sessionViewACPContentDoc, bool) {
	raw := strings.TrimSpace(content)
	if raw == "" {
		return sessionViewACPContentDoc{}, false
	}
	var doc sessionViewACPContentDoc
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		return sessionViewACPContentDoc{}, false
	}
	doc.Method = strings.TrimSpace(doc.Method)
	return doc, doc.Method != ""
}

func decodeSessionViewEventMethod(content string) (string, bool) {
	doc, ok := decodeSessionViewACPContentDoc(content)
	if !ok {
		return "", false
	}
	return strings.TrimSpace(doc.Method), true
}

func decodeSessionViewEventParams(content string, out any) bool {
	if out == nil {
		return false
	}
	doc, ok := decodeSessionViewACPContentDoc(content)
	if !ok || len(doc.Params) == 0 || strings.TrimSpace(string(doc.Params)) == "" {
		return false
	}
	return json.Unmarshal(doc.Params, out) == nil
}

func decodeSessionViewEventResult(content string, out any) bool {
	if out == nil {
		return false
	}
	doc, ok := decodeSessionViewACPContentDoc(content)
	if !ok || len(doc.Result) == 0 || strings.TrimSpace(string(doc.Result)) == "" {
		return false
	}
	return json.Unmarshal(doc.Result, out) == nil
}

func decodeSessionViewEventID(content string) int64 {
	doc, ok := decodeSessionViewACPContentDoc(content)
	if !ok {
		return 0
	}
	return doc.ID
}

func decodeSessionViewEventUpdate(content string) (acp.SessionUpdate, bool) {
	method, ok := decodeSessionViewEventMethod(content)
	if !ok || method != acp.MethodSessionUpdate {
		return acp.SessionUpdate{}, false
	}
	var params acp.SessionUpdateParams
	if !decodeSessionViewEventParams(content, &params) {
		return acp.SessionUpdate{}, false
	}
	update := params.Update
	if strings.TrimSpace(update.SessionUpdate) == "" {
		return acp.SessionUpdate{}, false
	}
	return update, true
}

func sessionViewMethodFromEvent(event SessionViewEvent) string {
	if method, ok := decodeSessionViewEventMethod(event.Content); ok {
		return method
	}
	if strings.EqualFold(strings.TrimSpace(string(event.Type)), string(SessionViewEventTypeSystem)) {
		return sessionViewMethodSystem
	}
	return ""
}

func (r *SessionRecorder) RecordEvent(ctx context.Context, event SessionViewEvent) error {
	event.SessionID = strings.TrimSpace(event.SessionID)
	if event.SessionID == "" {
		return nil
	}
	if event.UpdatedAt.IsZero() {
		event.UpdatedAt = time.Now().UTC()
	}
	method := sessionViewMethodFromEvent(event)

	switch method {
	case acp.MethodSessionNew:
		params := sessionViewSessionNewParams{}
		_ = decodeSessionViewEventParams(event.Content, &params)
		if strings.TrimSpace(params.SessionID) != "" {
			event.SessionID = strings.TrimSpace(params.SessionID)
		}
		return r.upsertSessionProjection(ctx, event.SessionID, strings.TrimSpace(params.Title), event.UpdatedAt)
	case acp.MethodSessionPrompt:
		var promptResult sessionViewPromptResult
		if decodeSessionViewEventResult(event.Content, &promptResult) {
			if err := r.flushBufferedSessionUpdate(ctx, event.SessionID); err != nil {
				return err
			}
			r.clearBufferedSessionUpdates(event.SessionID)
			if err := r.persistPromptStopReason(ctx, event.SessionID, promptResult.StopReason, event.UpdatedAt); err != nil {
				return err
			}
			return nil
		}
		var params acp.SessionPromptParams
		if !decodeSessionViewEventParams(event.Content, &params) {
			return nil
		}
		if strings.TrimSpace(params.SessionID) != "" {
			event.SessionID = strings.TrimSpace(params.SessionID)
		}
		if err := r.flushBufferedSessionUpdate(ctx, event.SessionID); err != nil {
			return err
		}
		r.clearBufferedSessionUpdates(event.SessionID)
		message := SessionTurnMessageRecord{
			ProjectName: r.projectName,
			SessionID:   event.SessionID,
			Method:      acp.MethodSessionPrompt,
			Role:        "user",
			Kind:        "text",
			Body:        PromptPreview(params.Prompt),
			Blocks:      cloneSessionContentBlocks(params.Prompt),
			Status:      "done",
			CreatedAt:   event.UpdatedAt,
			UpdatedAt:   event.UpdatedAt,
			Time:        event.UpdatedAt,

			Source: normalizeRecorderEventSource(event),
		}
		message.ContentJSON = normalizeJSONDoc(event.Content, `{"method":"`+acp.MethodSessionPrompt+`"}`)
		if err := r.store.AppendSessionTurnMessage(ctx, message); err != nil {
			return err
		}
		stored, err := r.loadLatestStoredSessionMessage(ctx, event.SessionID)
		if err != nil {
			return err
		}
		if err := r.upsertSessionProjection(ctx, event.SessionID, "", stored.UpdatedAt); err != nil {
			return err
		}
		r.publishSessionMessage(stored)
		return nil
	case acp.MethodSessionUpdate:
		return r.recordBufferedSessionUpdate(ctx, event)
	case acp.MethodRequestPermission:
		var permissionResult acp.PermissionResponse
		if decodeSessionViewEventResult(event.Content, &permissionResult) {
			requestID := decodeSessionViewEventID(event.Content)
			if requestID == 0 {
				return nil
			}
			if err := r.flushBufferedSessionUpdate(ctx, event.SessionID); err != nil {
				return err
			}
			messages, err := r.store.ListSessionTurnMessages(ctx, r.projectName, event.SessionID)
			if err != nil {
				return err
			}
			status := firstNonEmpty(strings.TrimSpace(permissionResult.Outcome.Outcome), "done")
			for _, message := range messages {
				if message.RequestID != requestID {
					continue
				}
				message.Status = status
				message.UpdatedAt = event.UpdatedAt
				message.Time = event.UpdatedAt
				message.ContentJSON = mergeSessionPermissionResultContentJSON(message.ContentJSON, event.Content)
				if err := r.store.UpsertSessionTurnMessage(ctx, message); err != nil {
					return err
				}
				stored, err := r.loadStoredSessionMessageByIndex(ctx, event.SessionID, message.SyncIndex)
				if err != nil {
					return err
				}
				r.publishSessionMessage(stored)
				break
			}
			return nil
		}
		var params acp.PermissionRequestParams
		if !decodeSessionViewEventParams(event.Content, &params) {
			return nil
		}
		if strings.TrimSpace(params.SessionID) != "" {
			event.SessionID = strings.TrimSpace(params.SessionID)
		}
		requestID := decodeSessionViewEventID(event.Content)
		if err := r.flushBufferedSessionUpdate(ctx, event.SessionID); err != nil {
			return err
		}
		message := SessionTurnMessageRecord{
			ProjectName: r.projectName,
			SessionID:   event.SessionID,
			Method:      acp.MethodRequestPermission,
			Role:        "system",
			Kind:        "permission",
			Body:        strings.TrimSpace(params.ToolCall.Title),
			Options:     cloneSessionPermissionOptions(params.Options),
			Status:      "needs_action",
			CreatedAt:   event.UpdatedAt,
			UpdatedAt:   event.UpdatedAt,
			Time:        event.UpdatedAt,
			RequestID:   requestID,

			Source: normalizeRecorderEventSource(event),
		}
		message.ContentJSON = normalizeJSONDoc(event.Content, `{"method":"`+acp.MethodRequestPermission+`"}`)
		existing, err := r.findPermissionMessageByRequestID(ctx, event.SessionID, requestID)
		if err != nil {
			return err
		}
		existed := existing != nil
		if existed {
			message.SyncIndex = existing.SyncIndex
			message.SyncSubIndex = existing.SyncSubIndex
			if !existing.CreatedAt.IsZero() {
				message.CreatedAt = existing.CreatedAt
			}
			if err := r.store.UpsertSessionTurnMessage(ctx, message); err != nil {
				return err
			}
		} else {
			if err := r.store.AppendSessionTurnMessage(ctx, message); err != nil {
				return err
			}
		}
		stored, err := r.loadLatestStoredSessionMessage(ctx, event.SessionID)
		if err != nil {
			return err
		}
		if err := r.upsertSessionProjection(ctx, event.SessionID, "", stored.UpdatedAt); err != nil {
			return err
		}
		if !existed {
			r.incrementSessionUnread(event.SessionID)
		}
		r.publishSessionMessage(stored)
		return nil
	case sessionViewMethodSystem:
		return nil
	default:
		return nil
	}
}

func (r *SessionRecorder) persistPromptStopReason(ctx context.Context, sessionID, stopReason string, updatedAt time.Time) error {
	sessionID = strings.TrimSpace(sessionID)
	stopReason = strings.TrimSpace(stopReason)
	if sessionID == "" || stopReason == "" {
		return nil
	}
	prompts, err := r.store.ListSessionPrompts(ctx, r.projectName, sessionID)
	if err != nil {
		return err
	}
	if len(prompts) == 0 {
		return nil
	}
	latest := prompts[len(prompts)-1]
	if latest.PromptIndex <= 0 {
		return nil
	}
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}
	updateIndex := latest.UpdateIndex + 1
	if updateIndex <= 0 {
		updateIndex = 1
	}
	return r.store.UpsertSessionPrompt(ctx, SessionPromptRecord{
		SessionID:   sessionID,
		ProjectName: r.projectName,
		PromptIndex: latest.PromptIndex,
		UpdateIndex: updateIndex,
		StopReason:  stopReason,
		UpdatedAt:   updatedAt,
	})
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

type recorderTurnKeyStrategy string

const (
	recorderTurnKeyBySessionUpdate recorderTurnKeyStrategy = "session_update"
	recorderTurnKeyByToolCallID    recorderTurnKeyStrategy = "tool_call_id"
)

var recorderSessionUpdateTurnKeyStrategy = map[string]recorderTurnKeyStrategy{
	acp.SessionUpdateToolCall:       recorderTurnKeyByToolCallID,
	acp.SessionUpdateToolCallUpdate: recorderTurnKeyByToolCallID,
}

func recorderTurnKeyStrategyForSessionUpdate(updateName string) recorderTurnKeyStrategy {
	if strategy, ok := recorderSessionUpdateTurnKeyStrategy[strings.TrimSpace(updateName)]; ok {
		return strategy
	}
	return recorderTurnKeyBySessionUpdate
}

func recorderCanonicalSessionUpdate(updateName string, strategy recorderTurnKeyStrategy) string {
	updateName = strings.TrimSpace(updateName)
	if strategy == recorderTurnKeyByToolCallID {
		return acp.SessionUpdateToolCall
	}
	return updateName
}

func recorderUpdateTurnKey(event SessionViewEvent) string {
	if update, ok := sessionUpdateFromEvent(event); ok {
		updateName := strings.TrimSpace(update.SessionUpdate)
		if updateName == "" {
			updateName = acp.MethodSessionUpdate
		}
		strategy := recorderTurnKeyStrategyForSessionUpdate(updateName)
		canonical := recorderCanonicalSessionUpdate(updateName, strategy)
		switch strategy {
		case recorderTurnKeyByToolCallID:
			toolCallID := strings.TrimSpace(update.ToolCallID)
			if toolCallID == "" {
				toolCallID = "unknown"
			}
			return canonical + ":" + toolCallID
		default:
			return canonical
		}
	}
	return sessionViewMethodFromEvent(event)
}

func sessionUpdateFromEvent(event SessionViewEvent) (acp.SessionUpdate, bool) {
	if !strings.EqualFold(strings.TrimSpace(string(event.Type)), string(SessionViewEventTypeACP)) {
		return acp.SessionUpdate{}, false
	}
	return decodeSessionViewEventUpdate(event.Content)
}

func mergeSessionUpdateContent(base, incoming acp.SessionUpdate) acp.SessionUpdate {
	merged := incoming
	if strings.TrimSpace(base.SessionUpdate) == "" || !strings.EqualFold(strings.TrimSpace(base.SessionUpdate), strings.TrimSpace(incoming.SessionUpdate)) {
		return merged
	}
	if base.ToolCallID != "" && merged.ToolCallID == "" {
		merged.ToolCallID = base.ToolCallID
	}
	switch strings.TrimSpace(incoming.SessionUpdate) {
	case acp.SessionUpdateAgentMessageChunk, acp.SessionUpdateUserMessageChunk, acp.SessionUpdateAgentThoughtChunk:
		text := extractTextChunk(base.Content) + extractTextChunk(incoming.Content)
		if strings.TrimSpace(text) != "" {
			raw, err := json.Marshal(acp.ContentBlock{Type: acp.ContentBlockTypeText, Text: text})
			if err == nil {
				merged.Content = raw
			}
		}
	}
	return merged
}

func sessionUpdateText(update acp.SessionUpdate) string {
	raw := extractTextChunk(update.Content)
	if strings.TrimSpace(raw) != "" {
		return raw
	}
	return ""
}

func sessionUpdateBody(update acp.SessionUpdate) string {
	switch strings.TrimSpace(update.SessionUpdate) {
	case acp.SessionUpdateToolCall, acp.SessionUpdateToolCallUpdate:
		return renderSessionToolStatus(update)
	default:
		return sessionUpdateText(update)
	}
}

func sessionUpdateRole(update acp.SessionUpdate) string {
	switch strings.TrimSpace(update.SessionUpdate) {
	case acp.SessionUpdateUserMessageChunk:
		return "user"
	case acp.SessionUpdateAgentMessageChunk, acp.SessionUpdateAgentThoughtChunk:
		return "assistant"
	case acp.SessionUpdateToolCall, acp.SessionUpdateToolCallUpdate:
		return "system"
	default:
		return ""
	}
}

func sessionUpdateKind(update acp.SessionUpdate) string {
	return strings.TrimSpace(update.SessionUpdate)
}

func sessionUpdateContentJSONHasParamsUpdate(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	var doc struct {
		Params struct {
			Update acp.SessionUpdate `json:"update"`
		} `json:"params"`
	}
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		return false
	}
	return strings.TrimSpace(doc.Params.Update.SessionUpdate) != ""
}
func buildACPMethodContentJSON(method string, body map[string]any) string {
	method = strings.TrimSpace(method)
	if method == "" {
		return "{}"
	}
	doc := map[string]any{"method": method}
	for k, v := range body {
		doc[k] = v
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		return fmt.Sprintf(`{"method":%q}`, method)
	}
	return string(raw)
}

func buildACPMethodParamsContent(method string, params any) string {
	doc := map[string]any{
		"params": params,
	}
	return buildACPMethodContentJSON(method, doc)
}

func buildACPMethodResultContent(method string, result any) string {
	doc := map[string]any{
		"result": result,
	}
	return buildACPMethodContentJSON(method, doc)
}

func buildACPMethodRequestContent(id int64, method string, params any) string {
	doc := map[string]any{
		"id":     id,
		"params": params,
	}
	return buildACPMethodContentJSON(method, doc)
}

func buildACPMethodResponseContent(id int64, method string, result any) string {
	doc := map[string]any{
		"id":     id,
		"result": result,
	}
	return buildACPMethodContentJSON(method, doc)
}

func mergeSessionPermissionResultContentJSON(requestContentJSON, responseContentJSON string) string {
	requestContentJSON = normalizeJSONDoc(requestContentJSON, `{"method":"`+acp.MethodRequestPermission+`"}`)
	responseContentJSON = normalizeJSONDoc(responseContentJSON, `{"method":"`+acp.MethodRequestPermission+`"}`)
	var reqDoc map[string]any
	var respDoc map[string]any
	if err := json.Unmarshal([]byte(requestContentJSON), &reqDoc); err != nil {
		return responseContentJSON
	}
	if err := json.Unmarshal([]byte(responseContentJSON), &respDoc); err != nil {
		return requestContentJSON
	}
	if result, ok := respDoc["result"]; ok {
		reqDoc["result"] = result
	}
	if id, ok := respDoc["id"]; ok {
		reqDoc["id"] = id
	}
	raw, err := json.Marshal(reqDoc)
	if err != nil {
		return responseContentJSON
	}
	return string(raw)
}

func newBufferedSessionUpdateMessage(projectName string, event SessionViewEvent) SessionTurnMessageRecord {
	message := SessionTurnMessageRecord{
		ProjectName: projectName,
		SessionID:   strings.TrimSpace(event.SessionID),
		Method:      acp.MethodSessionUpdate,
		Role:        "assistant",
		Kind:        "text",
		Body:        "",
		Status:      "streaming",

		RequestID:     0,
		Source:        normalizeRecorderEventSource(event),
		SourceChannel: strings.TrimSpace(event.SourceChannel),
		SourceChatID:  strings.TrimSpace(event.SourceChatID),
		CreatedAt:     event.UpdatedAt,
		UpdatedAt:     event.UpdatedAt,
		Time:          event.UpdatedAt,
	}
	if update, ok := sessionUpdateFromEvent(event); ok {
		message.Role = firstNonEmpty(sessionUpdateRole(update), strings.TrimSpace(message.Role), "assistant")
		message.Kind = firstNonEmpty(sessionUpdateKind(update), strings.TrimSpace(message.Kind), "text")
		message.Body = firstNonEmpty(sessionUpdateBody(update), strings.TrimSpace(message.Body))
		message.Status = firstNonEmpty(strings.TrimSpace(update.Status), strings.TrimSpace(message.Status), "streaming")
		message.ContentJSON = buildACPMethodParamsContent(acp.MethodSessionUpdate, acp.SessionUpdateParams{
			SessionID: strings.TrimSpace(event.SessionID),
			Update:    update,
		})
	}
	return message
}

func mergeBufferedSessionUpdateMessage(msg *SessionTurnMessageRecord, event SessionViewEvent) {
	if msg == nil {
		return
	}
	if update, ok := sessionUpdateFromEvent(event); ok {
		rawText := sessionUpdateBody(update)
		switch strings.TrimSpace(update.SessionUpdate) {
		case acp.SessionUpdateAgentMessageChunk, acp.SessionUpdateUserMessageChunk, acp.SessionUpdateAgentThoughtChunk:
			msg.Body += rawText
		default:
			msg.Body = firstNonEmpty(strings.TrimSpace(rawText), msg.Body)
		}
		msg.Role = firstNonEmpty(sessionUpdateRole(update), strings.TrimSpace(msg.Role), "assistant")
		msg.Kind = firstNonEmpty(sessionUpdateKind(update), strings.TrimSpace(msg.Kind), "text")
		msg.Status = firstNonEmpty(strings.TrimSpace(update.Status), msg.Status)
		if strings.TrimSpace(msg.ContentJSON) != "" {
			var existing struct {
				Params struct {
					Update acp.SessionUpdate `json:"update"`
				} `json:"params"`
			}
			if err := json.Unmarshal([]byte(msg.ContentJSON), &existing); err == nil {
				update = mergeSessionUpdateContent(existing.Params.Update, update)
			}
		}
		msg.ContentJSON = buildACPMethodParamsContent(acp.MethodSessionUpdate, acp.SessionUpdateParams{
			SessionID: strings.TrimSpace(msg.SessionID),
			Update:    update,
		})
	}
	if strings.TrimSpace(event.SourceChannel) != "" || strings.TrimSpace(event.SourceChatID) != "" {
		msg.Source = normalizeRecorderEventSource(event)
		msg.SourceChannel = strings.TrimSpace(event.SourceChannel)
		msg.SourceChatID = strings.TrimSpace(event.SourceChatID)
	}
	if msg.CreatedAt.IsZero() || event.UpdatedAt.Before(msg.CreatedAt) {
		msg.CreatedAt = event.UpdatedAt
	}
	msg.UpdatedAt = event.UpdatedAt
	msg.Time = event.UpdatedAt
}

func firstNonZeroInt64(v int64, fallback int64) int64 {
	if v != 0 {
		return v
	}
	return fallback
}

func isBufferedUpdateTerminalStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "done", "needs_action", "completed", "failed", "cancelled", "canceled":
		return true
	default:
		return false
	}
}

func shouldFlushBufferedSessionUpdateImmediately(event SessionViewEvent) bool {
	if update, ok := sessionUpdateFromEvent(event); ok {
		if isBufferedUpdateTerminalStatus(update.Status) {
			return true
		}
	}
	return false
}

// recordBufferedSessionUpdate batches ACP update chunks but flushes terminal statuses immediately.
func (r *SessionRecorder) recordBufferedSessionUpdate(ctx context.Context, event SessionViewEvent) error {
	r.updateMu.Lock()
	defer r.updateMu.Unlock()

	sessionID := strings.TrimSpace(event.SessionID)
	turnKey := recorderUpdateTurnKey(event)
	if turnKey == "" {
		turnKey = sessionViewMethodFromEvent(event)
	}
	sessionUpdates := r.updates[sessionID]
	if sessionUpdates == nil {
		sessionUpdates = map[string]*bufferedSessionUpdate{}
		r.updates[sessionID] = sessionUpdates
	}
	state := sessionUpdates[turnKey]
	if state == nil {
		msg := newBufferedSessionUpdateMessage(r.projectName, event)
		state = &bufferedSessionUpdate{message: msg, turnKey: turnKey, dirty: true}
		sessionUpdates[turnKey] = state
		if err := r.persistBufferedSessionUpdateLocked(ctx, sessionID, turnKey, state, true); err != nil {
			return err
		}
		return nil
	}
	mergeBufferedSessionUpdateMessage(&state.message, event)
	state.dirty = true
	if shouldFlushBufferedSessionUpdateImmediately(event) {
		if err := r.persistBufferedSessionUpdateLocked(ctx, sessionID, turnKey, state, false); err != nil {
			return err
		}
		return nil
	}
	if state.lastPersistedAt.IsZero() || event.UpdatedAt.Sub(state.lastPersistedAt) >= sessionUpdateFlushInterval {
		if err := r.persistBufferedSessionUpdateLocked(ctx, sessionID, turnKey, state, false); err != nil {
			return err
		}
	}
	return nil
}

func (r *SessionRecorder) flushBufferedSessionUpdate(ctx context.Context, sessionID string) error {
	r.updateMu.Lock()
	defer r.updateMu.Unlock()
	return r.flushBufferedSessionUpdateLocked(ctx, strings.TrimSpace(sessionID), false)
}

func (r *SessionRecorder) flushBufferedSessionUpdateLocked(ctx context.Context, sessionID string, force bool) error {
	sessionID = strings.TrimSpace(sessionID)
	sessionUpdates := r.updates[sessionID]
	if len(sessionUpdates) == 0 {
		return nil
	}
	keys := make([]string, 0, len(sessionUpdates))
	for turnKey := range sessionUpdates {
		keys = append(keys, turnKey)
	}
	sort.Strings(keys)
	for _, turnKey := range keys {
		state := sessionUpdates[turnKey]
		if state == nil {
			continue
		}
		if !state.persisted {
			if err := r.persistBufferedSessionUpdateLocked(ctx, sessionID, turnKey, state, true); err != nil {
				return err
			}
			continue
		}
		if !state.dirty && !force {
			continue
		}
		if err := r.persistBufferedSessionUpdateLocked(ctx, sessionID, turnKey, state, false); err != nil {
			return err
		}
	}
	return nil
}

func (r *SessionRecorder) persistBufferedSessionUpdateLocked(ctx context.Context, sessionID, turnKey string, state *bufferedSessionUpdate, appendMode bool) error {
	if state == nil {
		return nil
	}
	msg := state.message
	if appendMode {
		if err := r.store.AppendSessionTurnMessage(ctx, msg); err != nil {
			return err
		}
	} else {
		if !sessionUpdateContentJSONHasParamsUpdate(msg.ContentJSON) {
			msg.ContentJSON = ""
		}
		msg.MetaJSON = "{}"
		if err := r.store.UpsertSessionTurnMessage(ctx, msg); err != nil {
			return err
		}
	}
	stored := SessionTurnMessageRecord{}
	if msg.SyncIndex > 0 {
		storedByIndex, err := r.loadStoredSessionMessageByIndex(ctx, sessionID, msg.SyncIndex)
		if err != nil {
			return err
		}
		stored = storedByIndex
	} else {
		latest, err := r.loadLatestStoredSessionMessage(ctx, sessionID)
		if err != nil {
			return err
		}
		stored = latest
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
	sessionUpdates := r.updates[sessionID]
	if sessionUpdates == nil {
		sessionUpdates = map[string]*bufferedSessionUpdate{}
		r.updates[sessionID] = sessionUpdates
	}
	sessionUpdates[turnKey] = state
	return nil
}

func (r *SessionRecorder) clearBufferedSessionUpdates(sessionID string) {
	r.updateMu.Lock()
	defer r.updateMu.Unlock()
	r.clearBufferedSessionUpdatesLocked(sessionID)
}

func (r *SessionRecorder) clearBufferedSessionUpdatesLocked(sessionID string) {
	delete(r.updates, strings.TrimSpace(sessionID))
}

func (r *SessionRecorder) flushDueBufferedUpdates(ctx context.Context, now time.Time) {
	r.updateMu.Lock()
	defer r.updateMu.Unlock()
	for sessionID, sessionUpdates := range r.updates {
		for turnKey, state := range sessionUpdates {
			if state == nil || !state.persisted || !state.dirty {
				continue
			}
			if now.Sub(state.lastPersistedAt) < sessionUpdateFlushInterval {
				continue
			}
			_ = r.persistBufferedSessionUpdateLocked(ctx, sessionID, turnKey, state, false)
		}
	}
}

func (r *SessionRecorder) flushAllBufferedUpdates(ctx context.Context) {
	r.updateMu.Lock()
	defer r.updateMu.Unlock()
	for sessionID, sessionUpdates := range r.updates {
		for turnKey, state := range sessionUpdates {
			if state == nil {
				continue
			}
			if state.persisted && !state.dirty {
				continue
			}
			_ = r.persistBufferedSessionUpdateLocked(ctx, sessionID, turnKey, state, !state.persisted)
		}
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
	var messages []SessionTurnMessageRecord
	if afterIndex > 0 || afterSubIndex > 0 {
		messages, err = r.store.ListSessionTurnMessagesAfterCursor(ctx, r.projectName, strings.TrimSpace(sessionID), afterIndex, afterSubIndex)
	} else {
		messages, err = r.store.ListSessionTurnMessages(ctx, r.projectName, strings.TrimSpace(sessionID))
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

func (r *SessionRecorder) ReadSessionPrompts(ctx context.Context, sessionID string, afterPromptIndex, afterPromptUpdateIndex int64) (sessionViewSummary, []sessionViewPrompt, int64, int64, error) {
	sessionID = strings.TrimSpace(sessionID)
	rec, err := r.store.LoadSession(ctx, r.projectName, sessionID)
	if err != nil {
		return sessionViewSummary{}, nil, 0, 0, err
	}
	if rec == nil {
		return sessionViewSummary{}, nil, 0, 0, fmt.Errorf("session not found: %s", sessionID)
	}
	prompts, err := r.store.ListSessionPrompts(ctx, r.projectName, sessionID)
	if err != nil {
		return sessionViewSummary{}, nil, 0, 0, err
	}
	out := make([]sessionViewPrompt, 0, len(prompts))
	var lastPromptIndex int64
	var lastPromptUpdateIndex int64
	for _, prompt := range prompts {
		if prompt.PromptIndex > lastPromptIndex {
			lastPromptIndex = prompt.PromptIndex
			lastPromptUpdateIndex = prompt.UpdateIndex
		}
		if prompt.PromptIndex < afterPromptIndex {
			continue
		}
		if prompt.PromptIndex == afterPromptIndex && prompt.UpdateIndex <= afterPromptUpdateIndex {
			continue
		}
		turns, err := r.store.ListSessionTurns(ctx, r.projectName, sessionID, prompt.PromptIndex)
		if err != nil {
			return sessionViewSummary{}, nil, 0, 0, err
		}
		out = append(out, toSessionViewPrompt(prompt, turns))
	}
	return r.sessionViewSummaryFromRecord(*rec), out, lastPromptIndex, lastPromptUpdateIndex, nil
}
func (r *SessionRecorder) MarkSessionRead(ctx context.Context, sessionID string) (sessionViewSummary, bool) {
	r.resetSessionUnread(strings.TrimSpace(sessionID))
	return r.currentSessionViewSummary(ctx, sessionID)
}

func (r *SessionRecorder) loadLatestStoredSessionMessage(ctx context.Context, sessionID string) (SessionTurnMessageRecord, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return SessionTurnMessageRecord{}, fmt.Errorf("session id is required")
	}
	rec, err := r.store.LoadSession(ctx, r.projectName, sessionID)
	if err != nil {
		return SessionTurnMessageRecord{}, err
	}
	if rec == nil || rec.LastSyncIndex <= 0 {
		return SessionTurnMessageRecord{}, fmt.Errorf("session message not found for session %s", sessionID)
	}
	return r.loadStoredSessionMessageByIndex(ctx, sessionID, rec.LastSyncIndex)
}

func (r *SessionRecorder) loadStoredSessionMessageByIndex(ctx context.Context, sessionID string, index int64) (SessionTurnMessageRecord, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return SessionTurnMessageRecord{}, fmt.Errorf("session id is required")
	}
	if index <= 0 {
		return SessionTurnMessageRecord{}, fmt.Errorf("invalid session index: %d", index)
	}
	messages, err := r.store.ListSessionTurnMessagesAfterCursor(ctx, r.projectName, sessionID, index-1, -1)
	if err != nil {
		return SessionTurnMessageRecord{}, err
	}
	for _, message := range messages {
		if message.SyncIndex == index {
			return message, nil
		}
	}
	return SessionTurnMessageRecord{}, fmt.Errorf("session message not found: %s@%d", sessionID, index)
}

func (r *SessionRecorder) findPermissionMessageByRequestID(ctx context.Context, sessionID string, requestID int64) (*SessionTurnMessageRecord, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" || requestID == 0 {
		return nil, nil
	}
	messages, err := r.store.ListSessionTurnMessages(ctx, r.projectName, sessionID)
	if err != nil {
		return nil, err
	}
	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		if msg.RequestID != requestID {
			continue
		}
		method := strings.TrimSpace(msg.Method)
		if method != acp.MethodRequestPermission && method != "session.permission" && !(method == "" && strings.EqualFold(strings.TrimSpace(msg.Kind), "permission")) {
			continue
		}
		copyMsg := msg
		return &copyMsg, nil
	}
	return nil, nil
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

func (r *SessionRecorder) publishSessionMessage(message SessionTurnMessageRecord) {
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

func toSessionViewPrompt(prompt SessionPromptRecord, turns []SessionTurnRecord) sessionViewPrompt {
	promptID := strings.TrimSpace(prompt.PromptID)
	if promptID == "" {
		promptID = formatPromptSeq(prompt.PromptIndex)
	}
	updatedAt := prompt.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}
	out := sessionViewPrompt{
		MessageID:   promptID,
		PromptID:    promptID,
		SessionID:   strings.TrimSpace(prompt.SessionID),
		PromptIndex: prompt.PromptIndex,
		UpdateIndex: prompt.UpdateIndex,
		Title:       strings.TrimSpace(prompt.Title),
		StopReason:  strings.TrimSpace(prompt.StopReason),
		Status:      promptStatusFromStopReason(prompt.StopReason),
		UpdatedAt:   updatedAt.UTC().Format(time.RFC3339),
		Turns:       make([]sessionViewTurn, 0, len(turns)),
	}
	for _, turn := range turns {
		out.Turns = append(out.Turns, toSessionViewTurn(turn))
	}
	return out
}

func toSessionViewTurn(turn SessionTurnRecord) sessionViewTurn {
	updateJSON := normalizeJSONDoc(turn.UpdateJSON, `{}`)
	extraJSON := normalizeJSONDoc(turn.ExtraJSON, `{}`)
	turnID := strings.TrimSpace(turn.TurnID)
	if turnID == "" {
		turnID = formatPromptTurnSeq(turn.PromptIndex, turn.TurnIndex)
	}
	out := sessionViewTurn{
		TurnID:      turnID,
		PromptIndex: turn.PromptIndex,
		TurnIndex:   turn.TurnIndex,
		UpdateIndex: turn.UpdateIndex,
		Update:      json.RawMessage(updateJSON),
		Extra:       json.RawMessage(extraJSON),
	}
	var updateDoc struct {
		Method string `json:"method"`
		ID     int64  `json:"id"`
		Params struct {
			SessionID string                 `json:"sessionId"`
			Prompt    []acp.ContentBlock     `json:"prompt"`
			Update    acp.SessionUpdate      `json:"update"`
			ToolCall  acp.ToolCallRef        `json:"toolCall"`
			Options   []acp.PermissionOption `json:"options"`
		} `json:"params"`
		Result struct {
			StopReason string               `json:"stopReason"`
			Outcome    acp.PermissionResult `json:"outcome"`
		} `json:"result"`
		Payload struct {
			Role       string                 `json:"role"`
			Kind       string                 `json:"kind"`
			Text       string                 `json:"text"`
			Status     string                 `json:"status"`
			RequestID  int64                  `json:"requestId"`
			ToolCallID string                 `json:"toolCallId"`
			Blocks     []acp.ContentBlock     `json:"blocks"`
			Options    []acp.PermissionOption `json:"options"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(updateJSON), &updateDoc); err == nil {
		out.RequestID = updateDoc.ID
		out.Role = strings.TrimSpace(updateDoc.Payload.Role)
		out.Kind = firstNonEmpty(strings.TrimSpace(updateDoc.Payload.Kind), strings.TrimSpace(updateDoc.Method))
		out.Text = updateDoc.Payload.Text
		out.Status = strings.TrimSpace(updateDoc.Payload.Status)
		out.RequestID = firstNonZeroInt64(out.RequestID, updateDoc.Payload.RequestID)
		out.ToolCallID = strings.TrimSpace(updateDoc.Payload.ToolCallID)
		out.Blocks = cloneSessionContentBlocks(updateDoc.Payload.Blocks)
		out.Options = cloneSessionPermissionOptions(updateDoc.Payload.Options)

		if strings.TrimSpace(updateDoc.Params.Update.SessionUpdate) != "" {
			out.Kind = firstNonEmpty(out.Kind, strings.TrimSpace(updateDoc.Params.Update.SessionUpdate))
			out.Text = firstNonEmpty(out.Text, sessionUpdateText(updateDoc.Params.Update))
			out.Status = firstNonEmpty(out.Status, strings.TrimSpace(updateDoc.Params.Update.Status))
			out.ToolCallID = firstNonEmpty(out.ToolCallID, strings.TrimSpace(updateDoc.Params.Update.ToolCallID))
			switch strings.TrimSpace(updateDoc.Params.Update.SessionUpdate) {
			case acp.SessionUpdateUserMessageChunk:
				out.Role = firstNonEmpty(out.Role, "user")
			case acp.SessionUpdateAgentMessageChunk, acp.SessionUpdateAgentThoughtChunk:
				out.Role = firstNonEmpty(out.Role, "assistant")
			case acp.SessionUpdateToolCall, acp.SessionUpdateToolCallUpdate:
				out.Role = firstNonEmpty(out.Role, "system")
			default:
				out.Role = firstNonEmpty(out.Role, "assistant")
			}
		}
		switch strings.TrimSpace(updateDoc.Method) {
		case acp.MethodSessionPrompt:
			if len(updateDoc.Params.Prompt) > 0 {
				out.Role = firstNonEmpty(out.Role, "user")
				out.Kind = firstNonEmpty(out.Kind, acp.MethodSessionPrompt)
				out.Blocks = cloneSessionContentBlocks(updateDoc.Params.Prompt)
				out.Text = firstNonEmpty(out.Text, PromptPreview(updateDoc.Params.Prompt))
				out.Status = firstNonEmpty(out.Status, "done")
			}
			if strings.TrimSpace(updateDoc.Result.StopReason) != "" {
				out.Status = firstNonEmpty(out.Status, "done")
				out.Text = firstNonEmpty(out.Text, strings.TrimSpace(updateDoc.Result.StopReason))
			}
		case acp.MethodRequestPermission:
			out.Role = firstNonEmpty(out.Role, "system")
			out.Kind = firstNonEmpty(out.Kind, "permission")
			out.Text = firstNonEmpty(out.Text, strings.TrimSpace(updateDoc.Params.ToolCall.Title))
			out.ToolCallID = firstNonEmpty(out.ToolCallID, strings.TrimSpace(updateDoc.Params.ToolCall.ToolCallID))
			if len(out.Options) == 0 {
				out.Options = cloneSessionPermissionOptions(updateDoc.Params.Options)
			}
			if strings.TrimSpace(updateDoc.Result.Outcome.Outcome) != "" {
				out.Status = strings.TrimSpace(updateDoc.Result.Outcome.Outcome)
			} else {
				out.Status = firstNonEmpty(out.Status, "needs_action")
			}
		}
	}
	var extraDoc struct {
		RequestID int64 `json:"requestId"`
	}
	if err := json.Unmarshal([]byte(extraJSON), &extraDoc); err == nil {
		if out.RequestID == 0 {
			out.RequestID = extraDoc.RequestID
		}
	}
	return out
}

func promptStatusFromStopReason(stopReason string) string {
	if strings.TrimSpace(stopReason) == "" {
		return "running"
	}
	return "done"
}
func toSessionViewMessage(message SessionTurnMessageRecord) sessionViewMessage {
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
