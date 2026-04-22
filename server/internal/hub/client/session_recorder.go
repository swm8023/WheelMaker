package client

import (
	"context"
	"encoding/json"
	"errors"
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

var errSessionEventPayloadEmpty = errors.New("session event payload is empty")

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
	SessionID string `json:"sessionId"`
	Title     string `json:"title"`
	UpdatedAt string `json:"updatedAt"`
	Agent     string `json:"agent,omitempty"`
}

type sessionViewMessage struct {
	SessionID   string `json:"sessionId"`
	PromptIndex int64  `json:"promptIndex"`
	TurnIndex   int64  `json:"turnIndex"`
	UpdateIndex int64  `json:"updateIndex"`
	Content     string `json:"content"`
}

type sessionPromptState struct {
	promptIndex   int64
	nextTurnIndex int64
}

type SessionRecorder struct {
	projectName  string
	store        Store
	listSessions func(context.Context) ([]SessionListEntry, error)

	mu      sync.Mutex
	publish func(method string, payload any) error

	writeMu     sync.Mutex
	promptState map[string]sessionPromptState
}

func newSessionRecorder(projectName string, store Store, listSessions func(context.Context) ([]SessionListEntry, error)) *SessionRecorder {
	return &SessionRecorder{
		projectName:  projectName,
		store:        store,
		listSessions: listSessions,
		promptState:  map[string]sessionPromptState{},
	}
}

func (r *SessionRecorder) Close() {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.publish = nil
	r.mu.Unlock()
	r.writeMu.Lock()
	r.promptState = map[string]sessionPromptState{}
	r.writeMu.Unlock()
}

func (r *SessionRecorder) ResetPromptState() {
	if r == nil {
		return
	}
	r.writeMu.Lock()
	r.promptState = map[string]sessionPromptState{}
	r.writeMu.Unlock()
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

func decodeSessionViewACPContentDoc(content string) (sessionViewACPContentDoc, error) {
	raw := strings.TrimSpace(content)
	if raw == "" {
		return sessionViewACPContentDoc{}, errSessionEventPayloadEmpty
	}
	var doc sessionViewACPContentDoc
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		return sessionViewACPContentDoc{}, err
	}
	doc.Method = strings.TrimSpace(doc.Method)
	if doc.Method == "" {
		return sessionViewACPContentDoc{}, fmt.Errorf("session event method is required")
	}
	return doc, nil
}

func decodeSessionViewEventParams(doc sessionViewACPContentDoc, out any) error {
	if out == nil {
		return fmt.Errorf("params decode target is nil")
	}
	if len(doc.Params) == 0 || strings.TrimSpace(string(doc.Params)) == "" {
		return errSessionEventPayloadEmpty
	}
	if err := json.Unmarshal(doc.Params, out); err != nil {
		return err
	}
	return nil
}

func decodeSessionViewEventResult(doc sessionViewACPContentDoc, out any) error {
	if out == nil {
		return fmt.Errorf("result decode target is nil")
	}
	if len(doc.Result) == 0 || strings.TrimSpace(string(doc.Result)) == "" {
		return errSessionEventPayloadEmpty
	}
	if err := json.Unmarshal(doc.Result, out); err != nil {
		return err
	}
	return nil
}

func sessionViewMethodFromEvent(event SessionViewEvent, doc sessionViewACPContentDoc) string {
	if strings.TrimSpace(doc.Method) != "" {
		return strings.TrimSpace(doc.Method)
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
	doc := sessionViewACPContentDoc{}
	if strings.EqualFold(strings.TrimSpace(string(event.Type)), string(SessionViewEventTypeACP)) {
		parsedDoc, err := decodeSessionViewACPContentDoc(event.Content)
		if err != nil {
			return fmt.Errorf("decode acp event content: %w", err)
		}
		doc = parsedDoc
	}
	method := sessionViewMethodFromEvent(event, doc)

	r.writeMu.Lock()
	defer r.writeMu.Unlock()

	switch method {
	case acp.MethodSessionNew:
		params := sessionViewSessionNewParams{}
		if err := decodeSessionViewEventParams(doc, &params); err != nil && !errors.Is(err, errSessionEventPayloadEmpty) {
			return fmt.Errorf("decode session.new params: %w", err)
		}
		if strings.TrimSpace(params.SessionID) != "" {
			event.SessionID = strings.TrimSpace(params.SessionID)
		}
		return r.upsertSessionProjection(ctx, event.SessionID, strings.TrimSpace(params.Title), event.UpdatedAt, false)
	case acp.MethodSessionPrompt:
		var promptResult sessionViewPromptResult
		if err := decodeSessionViewEventResult(doc, &promptResult); err == nil {
			return r.handlePromptFinishedLocked(ctx, event, strings.TrimSpace(promptResult.StopReason))
		} else if !errors.Is(err, errSessionEventPayloadEmpty) {
			return fmt.Errorf("decode session.prompt result: %w", err)
		}
		var params acp.SessionPromptParams
		if err := decodeSessionViewEventParams(doc, &params); err != nil {
			return fmt.Errorf("decode session.prompt params: %w", err)
		}
		if strings.TrimSpace(params.SessionID) != "" {
			event.SessionID = strings.TrimSpace(params.SessionID)
		}
		return r.handlePromptStartedLocked(ctx, event, params)
	case acp.MethodSessionUpdate:
		return r.appendACPEventTurnLocked(ctx, event)
	case acp.MethodRequestPermission:
		var params acp.PermissionRequestParams
		if err := decodeSessionViewEventParams(doc, &params); err == nil {
			if strings.TrimSpace(params.SessionID) != "" {
				event.SessionID = strings.TrimSpace(params.SessionID)
			}
		} else if !errors.Is(err, errSessionEventPayloadEmpty) {
			return fmt.Errorf("decode request_permission params: %w", err)
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
		PromptIndex: state.promptIndex,
		Title:       promptTitle,
		UpdatedAt:   event.UpdatedAt,
	}); err != nil {
		return err
	}

	turn := buildSessionTurnRecord(event.SessionID, state.promptIndex, 1, event.Content, acp.MethodSessionPrompt)
	if err := r.appendSessionTurnLocked(ctx, turn); err != nil {
		return err
	}

	state.nextTurnIndex = 2
	r.promptState[event.SessionID] = state
	if err := r.upsertSessionProjection(ctx, event.SessionID, promptTitle, event.UpdatedAt, true); err != nil {
		return err
	}
	r.publishSessionTurn(event.SessionID, turn)
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
	turn := buildSessionTurnRecord(event.SessionID, state.promptIndex, state.nextTurnIndex, event.Content, acp.MethodSessionUpdate)
	if err := r.appendSessionTurnLocked(ctx, turn); err != nil {
		return err
	}
	state.nextTurnIndex++
	r.promptState[event.SessionID] = state
	return nil
}

func buildSessionTurnRecord(sessionID string, promptIndex, turnIndex int64, rawContent, fallbackMethod string) SessionTurnRecord {
	return SessionTurnRecord{
		TurnID:      formatPromptTurnSeq(promptIndex, turnIndex),
		SessionID:   strings.TrimSpace(sessionID),
		PromptIndex: promptIndex,
		TurnIndex:   turnIndex,
		UpdateIndex: 1,
		UpdateJSON:  normalizeJSONDoc(rawContent, `{"method":"`+strings.TrimSpace(fallbackMethod)+`"}`),
		ExtraJSON:   "{}",
	}
}

func (r *SessionRecorder) appendSessionTurnLocked(ctx context.Context, turn SessionTurnRecord) error {
	if err := r.store.UpsertSessionTurn(ctx, turn); err != nil {
		return err
	}
	r.publishSessionTurn(turn.SessionID, turn)
	return nil
}

func (r *SessionRecorder) handlePromptFinishedLocked(ctx context.Context, event SessionViewEvent, stopReason string) error {
	state, err := r.ensurePromptStateLocked(ctx, event.SessionID, event.UpdatedAt)
	if err != nil {
		return err
	}
	if err := r.store.UpsertSessionPrompt(ctx, SessionPromptRecord{
		SessionID:   event.SessionID,
		PromptIndex: state.promptIndex,
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
		prompt, err := r.store.LoadSessionPrompt(ctx, r.projectName, sessionID, current.promptIndex)
		if err != nil {
			return sessionPromptState{}, err
		}
		if prompt == nil {
			delete(r.promptState, sessionID)
			return sessionPromptState{promptIndex: 1, nextTurnIndex: 1}, nil
		}
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
		prompt, err := r.store.LoadSessionPrompt(ctx, r.projectName, sessionID, state.promptIndex)
		if err != nil {
			return sessionPromptState{}, err
		}
		if prompt == nil {
			delete(r.promptState, sessionID)
		} else {
			if state.nextTurnIndex <= 0 {
				state.nextTurnIndex = 1
			}
			return state, nil
		}
	}
	prompts, err := r.store.ListSessionPrompts(ctx, r.projectName, sessionID)
	if err != nil {
		return sessionPromptState{}, err
	}
	if len(prompts) == 0 {
		state := sessionPromptState{promptIndex: 1, nextTurnIndex: 1}
		if err := r.store.UpsertSessionPrompt(ctx, SessionPromptRecord{
			SessionID:   sessionID,
			PromptIndex: 1,
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

func (r *SessionRecorder) publishSessionTurn(sessionID string, turn SessionTurnRecord) {
	publish := r.eventPublisher()
	if publish == nil {
		return
	}
	sessionID = strings.TrimSpace(sessionID)
	content := strings.TrimSpace(turn.UpdateJSON)
	if content == "" {
		content = "{}"
	}
	_ = publish("registry.session.message", map[string]any{
		"sessionId":   sessionID,
		"promptIndex": turn.PromptIndex,
		"turnIndex":   turn.TurnIndex,
		"updateIndex": turn.UpdateIndex,
		"content":     content,
	})
}

func toSessionViewMessage(turn SessionTurnRecord) sessionViewMessage {
	content := strings.TrimSpace(turn.UpdateJSON)
	if content == "" {
		content = "{}"
	}
	return sessionViewMessage{
		SessionID:   strings.TrimSpace(turn.SessionID),
		PromptIndex: turn.PromptIndex,
		TurnIndex:   turn.TurnIndex,
		UpdateIndex: turn.UpdateIndex,
		Content:     content,
	}
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
	out := make([]sessionViewSummary, 0, len(entries))
	for _, entry := range entries {
		out = append(out, r.sessionViewSummaryFromEntry(entry))
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].UpdatedAt > out[j].UpdatedAt
	})
	return out, nil
}

func (r *SessionRecorder) ReadSessionMessages(ctx context.Context, sessionID string, afterPromptIndex, afterTurnIndex int64) (sessionViewSummary, []sessionViewMessage, int64, int64, error) {
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

	out := make([]sessionViewMessage, 0)
	var lastIndex int64
	var lastSubIndex int64
	for _, prompt := range prompts {
		turns, err := r.store.ListSessionTurns(ctx, r.projectName, sessionID, prompt.PromptIndex)
		if err != nil {
			return sessionViewSummary{}, nil, 0, 0, err
		}
		for _, turn := range turns {
			if turn.PromptIndex > lastIndex || (turn.PromptIndex == lastIndex && turn.TurnIndex > lastSubIndex) {
				lastIndex = turn.PromptIndex
				lastSubIndex = turn.TurnIndex
			}
			if turn.PromptIndex < afterPromptIndex {
				continue
			}
			if turn.PromptIndex == afterPromptIndex && turn.TurnIndex <= afterTurnIndex {
				continue
			}
			out = append(out, toSessionViewMessage(turn))
		}
	}

	return r.sessionViewSummaryFromRecord(*rec), out, lastIndex, lastSubIndex, nil
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
		return sessionViewSummary{}, false
	}
	return r.sessionViewSummaryFromRecord(*rec), true
}

func (r *SessionRecorder) sessionViewSummaryFromEntry(entry SessionListEntry) sessionViewSummary {
	return buildSessionViewSummary(
		entry.ID,
		entry.Title,
		entry.LastActiveAt,
		entry.Agent,
	)
}

func (r *SessionRecorder) sessionViewSummaryFromRecord(rec SessionRecord) sessionViewSummary {
	return buildSessionViewSummary(
		rec.ID,
		rec.Title,
		rec.LastActiveAt,
		"",
	)
}

func (r *SessionRecorder) publishSessionUpdated(summary sessionViewSummary) {
	publish := r.eventPublisher()
	if publish == nil {
		return
	}
	_ = publish("registry.session.updated", map[string]any{"session": summary})
}

func buildSessionViewSummary(sessionID, title string, lastActiveAt time.Time, agent string) sessionViewSummary {
	return sessionViewSummary{
		SessionID: strings.TrimSpace(sessionID),
		Title:     firstNonEmpty(strings.TrimSpace(title), strings.TrimSpace(sessionID)),
		UpdatedAt: lastActiveAt.UTC().Format(time.RFC3339),
		Agent:     strings.TrimSpace(agent),
	}
}
