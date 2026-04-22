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

type sessionPromptState struct {
	promptIndex   int64
	nextTurnIndex int64
}

type SessionRecorder struct {
	projectName  string
	store        Store
	listSessions func(context.Context) ([]SessionListEntry, error)

	mu          sync.Mutex
	publish     func(method string, payload any) error
	unreadCount map[string]int

	writeMu     sync.Mutex
	promptState map[string]sessionPromptState
}

func newSessionRecorder(projectName string, store Store, listSessions func(context.Context) ([]SessionListEntry, error)) *SessionRecorder {
	return &SessionRecorder{
		projectName:  projectName,
		store:        store,
		listSessions: listSessions,
		unreadCount:  map[string]int{},
		promptState:  map[string]sessionPromptState{},
	}
}

func (r *SessionRecorder) Close() {
	if r == nil {
		return
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

	r.writeMu.Lock()
	defer r.writeMu.Unlock()

	switch method {
	case acp.MethodSessionNew:
		params := sessionViewSessionNewParams{}
		_ = decodeSessionViewEventParams(event.Content, &params)
		if strings.TrimSpace(params.SessionID) != "" {
			event.SessionID = strings.TrimSpace(params.SessionID)
		}
		return r.upsertSessionProjection(ctx, event.SessionID, strings.TrimSpace(params.Title), event.UpdatedAt, false)
	case acp.MethodSessionPrompt:
		var promptResult sessionViewPromptResult
		if decodeSessionViewEventResult(event.Content, &promptResult) {
			return r.handlePromptFinishedLocked(ctx, event, strings.TrimSpace(promptResult.StopReason))
		}
		var params acp.SessionPromptParams
		if !decodeSessionViewEventParams(event.Content, &params) {
			return nil
		}
		if strings.TrimSpace(params.SessionID) != "" {
			event.SessionID = strings.TrimSpace(params.SessionID)
		}
		return r.handlePromptStartedLocked(ctx, event, params)
	case acp.MethodSessionUpdate:
		return r.appendACPEventTurnLocked(ctx, event)
	case acp.MethodRequestPermission:
		var params acp.PermissionRequestParams
		if decodeSessionViewEventParams(event.Content, &params) {
			if strings.TrimSpace(params.SessionID) != "" {
				event.SessionID = strings.TrimSpace(params.SessionID)
			}
		}
		return r.appendACPEventTurnLocked(ctx, event)
	case sessionViewMethodSystem:
		return nil
	default:
		return nil
	}
}

func (r *SessionRecorder) handlePromptStartedLocked(ctx context.Context, event SessionViewEvent, params acp.SessionPromptParams) error {
	state, err := r.nextPromptStateLocked(ctx, event.SessionID)
	if err != nil {
		return err
	}
	promptTitle := strings.TrimSpace(PromptPreview(params.Prompt))
	if err := r.store.UpsertSessionPrompt(ctx, SessionPromptRecord{
		SessionID:   event.SessionID,
		ProjectName: r.projectName,
		PromptIndex: state.promptIndex,
		UpdateIndex: 0,
		Title:       promptTitle,
		UpdatedAt:   event.UpdatedAt,
	}); err != nil {
		return err
	}

	turn := SessionTurnRecord{
		TurnID:      formatPromptTurnSeq(state.promptIndex, 1),
		SessionID:   event.SessionID,
		ProjectName: r.projectName,
		PromptIndex: state.promptIndex,
		TurnIndex:   1,
		UpdateIndex: 1,
		UpdateJSON:  normalizeJSONDoc(event.Content, `{"method":"`+acp.MethodSessionPrompt+`"}`),
		ExtraJSON:   "{}",
	}
	if err := r.store.UpsertSessionTurn(ctx, turn); err != nil {
		return err
	}

	state.nextTurnIndex = 2
	r.promptState[event.SessionID] = state
	if err := r.upsertSessionProjection(ctx, event.SessionID, promptTitle, event.UpdatedAt, true); err != nil {
		return err
	}
	r.incrementSessionUnread(event.SessionID)
	r.publishSessionTurn(event.SessionID, turn, event.UpdatedAt)
	return nil
}

func (r *SessionRecorder) appendACPEventTurnLocked(ctx context.Context, event SessionViewEvent) error {
	state, err := r.ensurePromptStateLocked(ctx, event.SessionID, event.UpdatedAt)
	if err != nil {
		return err
	}
	if state.nextTurnIndex <= 0 {
		state.nextTurnIndex = 1
	}
	turn := SessionTurnRecord{
		TurnID:      formatPromptTurnSeq(state.promptIndex, state.nextTurnIndex),
		SessionID:   event.SessionID,
		ProjectName: r.projectName,
		PromptIndex: state.promptIndex,
		TurnIndex:   state.nextTurnIndex,
		UpdateIndex: 1,
		UpdateJSON:  normalizeJSONDoc(event.Content, `{"method":"`+acp.MethodSessionUpdate+`"}`),
		ExtraJSON:   "{}",
	}
	if err := r.store.UpsertSessionTurn(ctx, turn); err != nil {
		return err
	}
	state.nextTurnIndex++
	r.promptState[event.SessionID] = state
	r.publishSessionTurn(event.SessionID, turn, event.UpdatedAt)
	return nil
}

func (r *SessionRecorder) handlePromptFinishedLocked(ctx context.Context, event SessionViewEvent, stopReason string) error {
	state, err := r.ensurePromptStateLocked(ctx, event.SessionID, event.UpdatedAt)
	if err != nil {
		return err
	}
	finalUpdateIndex := state.nextTurnIndex - 1
	if finalUpdateIndex < 0 {
		finalUpdateIndex = 0
	}
	if err := r.store.UpsertSessionPrompt(ctx, SessionPromptRecord{
		SessionID:   event.SessionID,
		ProjectName: r.projectName,
		PromptIndex: state.promptIndex,
		UpdateIndex: finalUpdateIndex,
		StopReason:  strings.TrimSpace(stopReason),
		UpdatedAt:   event.UpdatedAt,
	}); err != nil {
		return err
	}
	delete(r.promptState, event.SessionID)
	return nil
}

func (r *SessionRecorder) nextPromptStateLocked(ctx context.Context, sessionID string) (sessionPromptState, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return sessionPromptState{}, fmt.Errorf("session id is required")
	}
	if current, ok := r.promptState[sessionID]; ok && current.promptIndex > 0 {
		return sessionPromptState{promptIndex: current.promptIndex + 1, nextTurnIndex: 1}, nil
	}
	prompts, err := r.store.ListSessionPrompts(ctx, r.projectName, sessionID)
	if err != nil {
		return sessionPromptState{}, err
	}
	promptIndex := int64(1)
	if len(prompts) > 0 && prompts[len(prompts)-1].PromptIndex > 0 {
		promptIndex = prompts[len(prompts)-1].PromptIndex + 1
	}
	return sessionPromptState{promptIndex: promptIndex, nextTurnIndex: 1}, nil
}

func (r *SessionRecorder) ensurePromptStateLocked(ctx context.Context, sessionID string, updatedAt time.Time) (sessionPromptState, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return sessionPromptState{}, fmt.Errorf("session id is required")
	}
	if state, ok := r.promptState[sessionID]; ok && state.promptIndex > 0 {
		if state.nextTurnIndex <= 0 {
			state.nextTurnIndex = 1
		}
		return state, nil
	}
	prompts, err := r.store.ListSessionPrompts(ctx, r.projectName, sessionID)
	if err != nil {
		return sessionPromptState{}, err
	}
	if len(prompts) == 0 {
		state := sessionPromptState{promptIndex: 1, nextTurnIndex: 1}
		if err := r.store.UpsertSessionPrompt(ctx, SessionPromptRecord{
			SessionID:   sessionID,
			ProjectName: r.projectName,
			PromptIndex: 1,
			UpdateIndex: 0,
			UpdatedAt:   updatedAt,
		}); err != nil {
			return sessionPromptState{}, err
		}
		r.promptState[sessionID] = state
		return state, nil
	}
	latest := prompts[len(prompts)-1]
	turns, err := r.store.ListSessionTurns(ctx, r.projectName, sessionID, latest.PromptIndex)
	if err != nil {
		return sessionPromptState{}, err
	}
	nextTurn := int64(len(turns) + 1)
	state := sessionPromptState{promptIndex: latest.PromptIndex, nextTurnIndex: nextTurn}
	r.promptState[sessionID] = state
	return state, nil
}

func (r *SessionRecorder) publishSessionTurn(sessionID string, turn SessionTurnRecord, updatedAt time.Time) {
	publish := r.eventPublisher()
	if publish == nil {
		return
	}
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}
	ctx := context.Background()
	summary, ok := r.currentSessionViewSummary(ctx, sessionID)
	if !ok {
		summary = sessionViewSummary{SessionID: sessionID, Title: sessionID, UpdatedAt: updatedAt.UTC().Format(time.RFC3339), ProjectName: r.projectName}
	}
	decoded := toSessionViewTurn(turn)
	messageID := strings.TrimSpace(turn.TurnID)
	if messageID == "" {
		messageID = formatPromptTurnSeq(turn.PromptIndex, turn.TurnIndex)
	}
	_ = publish("registry.session.message", map[string]any{"session": summary, "message": sessionViewMessage{
		MessageID: messageID,
		SessionID: sessionID,
		Index:     turn.PromptIndex,
		SubIndex:  turn.TurnIndex,
		Role:      decoded.Role,
		Kind:      decoded.Kind,
		Text:      decoded.Text,
		Blocks:    cloneSessionContentBlocks(decoded.Blocks),
		Options:   cloneSessionPermissionOptions(decoded.Options),
		Status:    decoded.Status,
		CreatedAt: updatedAt.UTC().Format(time.RFC3339),
		UpdatedAt: updatedAt.UTC().Format(time.RFC3339),
		RequestID: decoded.RequestID,
	}})
}

func sessionUpdateText(update acp.SessionUpdate) string {
	raw := extractTextChunk(update.Content)
	if strings.TrimSpace(raw) != "" {
		return raw
	}
	return ""
}

func firstNonZeroInt64(v int64, fallback int64) int64 {
	if v != 0 {
		return v
	}
	return fallback
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
	var lastIndex int64
	var lastSubIndex int64
	for _, message := range messages {
		out = append(out, toSessionViewMessage(message))
		if message.SyncIndex > lastIndex || (message.SyncIndex == lastIndex && message.SyncSubIndex > lastSubIndex) {
			lastIndex = message.SyncIndex
			lastSubIndex = message.SyncSubIndex
		}
	}
	return r.sessionViewSummaryFromRecord(*rec), out, lastIndex, lastSubIndex, nil
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

func (r *SessionRecorder) upsertSessionProjection(ctx context.Context, sessionID, title string, updatedAt time.Time, titleIfEmptyOnly ...bool) error {
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
	onlyIfEmpty := false
	if len(titleIfEmptyOnly) > 0 {
		onlyIfEmpty = titleIfEmptyOnly[0]
	}
	title = strings.TrimSpace(title)
	if title != "" {
		if !onlyIfEmpty || strings.TrimSpace(rec.Title) == "" {
			rec.Title = title
		}
	}
	rec.LastActiveAt = updatedAt
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
	updatedAt := entry.LastActiveAt
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
	updatedAt := rec.LastActiveAt
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
	extraJSON := "{}"
	if message.RequestID != 0 {
		extraJSON = fmt.Sprintf(`{"requestId":%d}`, message.RequestID)
	}
	decoded := toSessionViewTurn(SessionTurnRecord{
		TurnID:      strings.TrimSpace(message.MessageID),
		PromptIndex: 0,
		TurnIndex:   0,
		UpdateIndex: message.SyncSubIndex + 1,
		UpdateJSON:  message.ContentJSON,
		ExtraJSON:   extraJSON,
	})
	text := firstNonEmpty(decoded.Text, message.Body)
	blocks := cloneSessionContentBlocks(decoded.Blocks)
	if len(blocks) == 0 {
		blocks = cloneSessionContentBlocks(message.Blocks)
	}
	options := cloneSessionPermissionOptions(decoded.Options)
	if len(options) == 0 {
		options = cloneSessionPermissionOptions(message.Options)
	}
	requestID := firstNonZeroInt64(decoded.RequestID, message.RequestID)

	return sessionViewMessage{
		MessageID: message.MessageID,
		SessionID: message.SessionID,
		Index:     message.SyncIndex,
		SubIndex:  message.SyncSubIndex,
		Role:      decoded.Role,
		Kind:      decoded.Kind,
		Text:      text,
		Blocks:    blocks,
		Options:   options,
		Status:    decoded.Status,
		CreatedAt: message.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt: message.UpdatedAt.UTC().Format(time.RFC3339),
		RequestID: requestID,
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
