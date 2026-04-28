package client

import (
	"bytes"
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

type SessionViewEvent struct {
	Type      SessionViewEventType
	SessionID string
	Content   string

	SourceChannel string
	SourceChatID  string
	UpdatedAt     time.Time
}

type SessionViewSink interface {
	RecordEvent(ctx context.Context, event SessionViewEvent) error
}

type sessionViewSummary struct {
	SessionID string `json:"sessionId"`
	Title     string `json:"title"`
	UpdatedAt string `json:"updatedAt"`
	AgentType string `json:"agentType,omitempty"`
}

type sessionViewMessage struct {
	SessionID   string `json:"sessionId"`
	PromptIndex int64  `json:"promptIndex"`
	TurnIndex   int64  `json:"turnIndex"`
	Content     string `json:"content"`
}

type sessionPromptSnapshot struct {
	SessionID   string   `json:"sessionId"`
	PromptIndex int64    `json:"promptIndex"`
	TurnIndex   int64    `json:"turnIndex"`
	Content     []string `json:"content"`
}

type sessionMessagePage struct {
	afterPromptIndex int64
	afterTurnIndex   int64
	lastPromptIndex  int64
	lastTurnIndex    int64
	messages         []sessionViewMessage
}

type sessionTurnMessage struct {
	SessionID   string
	method      string
	payload     any
	PromptIndex int64
	TurnIndex   int64
}

type sessionPromptState struct {
	promptIndex   int64
	nextTurnIndex int64

	turns          map[int64]sessionTurnMessage
	turnIndexByKey map[string]int64
}

type sessionViewSessionNewParams struct {
	SessionID string `json:"sessionId,omitempty"`
	AgentType string `json:"agentType,omitempty"`
	Title     string `json:"title,omitempty"`
}

type parsedSessionViewEvent struct {
	raw       SessionViewEvent
	bMessage  bool
	method    string
	payload   any
	acpMethod string
	turnKey   string
}

type sessionTurnMergeKind string

const (
	sessionTurnMergeNone sessionTurnMergeKind = ""
	sessionTurnMergeTool sessionTurnMergeKind = "tool"
	sessionTurnMergeText sessionTurnMergeKind = "text"
)

type SessionRecorder struct {
	projectName  string
	store        Store
	listSessions func(context.Context) ([]SessionRecord, error)

	mu      sync.Mutex
	publish func(method string, payload any) error

	writeMu     sync.Mutex
	promptState map[string]sessionPromptState
}

func newSessionRecorder(projectName string, store Store, listSessions func(context.Context) ([]SessionRecord, error)) *SessionRecorder {
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

func (r *SessionRecorder) RecordEvent(ctx context.Context, event SessionViewEvent) error {
	if event.SessionID == "" {
		return nil
	}

	parsed, err := parseSessionViewEvent(event)
	if err != nil {
		return err
	}

	r.writeMu.Lock()
	defer r.writeMu.Unlock()

	if parsed.bMessage {
		return r.handleIMMessage(ctx, parsed)
	}

	switch parsed.acpMethod {
	case acp.MethodSessionNew:
		title := ""
		agentType := ""
		jsonDecodeAt(json.RawMessage(parsed.raw.Content), "params.title", &title)
		jsonDecodeAt(json.RawMessage(parsed.raw.Content), "params.agentType", &agentType)
		title = strings.TrimSpace(title)
		return r.upsertSessionProjection(ctx, parsed.raw.SessionID, strings.TrimSpace(agentType), title, parsed.raw.UpdatedAt, false)
	default:
		return nil
	}
}

func (r *SessionRecorder) handleIMMessage(ctx context.Context, event parsedSessionViewEvent) error {
	method := event.method
	if method == "" {
		return nil
	}

	switch method {
	case acp.IMMethodPromptRequest:
		if event.payload != nil {
			return r.handlePromptStartedLocked(ctx, event)
		}
		return nil
	case acp.IMMethodPromptDone:
		if event.payload != nil {
			return r.handlePromptFinishedLocked(ctx, event)
		}
		return nil

	case acp.IMMethodSystem, acp.IMMethodAgentThought, acp.IMMethodAgentMessage, acp.SessionUpdateUserMessageChunk, acp.IMMethodAgentPlan, acp.IMMethodToolCall:
		return r.handleUpdateMessageLocked(ctx, event)
	default:
		return nil
	}
}

func (e *parsedSessionViewEvent) setJSONMessage(method string, payload any, turnKey string) {
	e.bMessage = true
	e.method = strings.TrimSpace(method)
	e.payload = payload
	e.turnKey = strings.TrimSpace(turnKey)
}

func (e parsedSessionViewEvent) imMessage() acp.IMMessage {
	message := acp.IMMessage{
		Method: e.method,
	}
	if e.payload != nil {
		message.Param = mustJSONRaw(e.payload)
	}
	return message
}

func (r *SessionRecorder) ListSessionViews(ctx context.Context) ([]sessionViewSummary, error) {
	entries, err := r.listSessions(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]sessionViewSummary, 0, len(entries))
	for _, entry := range entries {
		out = append(out, r.sessionViewSummaryFromRecord(entry))
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
	page := newSessionMessagePage(afterPromptIndex, afterTurnIndex)
	for _, prompt := range prompts {
		if err := r.appendPromptMessages(ctx, sessionID, prompt.PromptIndex, &page); err != nil {
			return sessionViewSummary{}, nil, 0, 0, err
		}
	}

	return r.sessionViewSummaryFromRecord(*rec), page.messages, page.lastPromptIndex, page.lastTurnIndex, nil
}

func (r *SessionRecorder) ReadSessionPrompts(ctx context.Context, sessionID string, checkpointPromptIndex, _ int64) (sessionViewSummary, []sessionPromptSnapshot, error) {
	sessionID = strings.TrimSpace(sessionID)
	rec, err := r.store.LoadSession(ctx, r.projectName, sessionID)
	if err != nil {
		return sessionViewSummary{}, nil, err
	}
	if rec == nil {
		return sessionViewSummary{}, nil, fmt.Errorf("session not found: %s", sessionID)
	}
	prompts, err := r.store.ListSessionPrompts(ctx, r.projectName, sessionID)
	if err != nil {
		return sessionViewSummary{}, nil, err
	}
	prompts = promptsFromCheckpoint(prompts, checkpointPromptIndex)
	out := make([]sessionPromptSnapshot, 0, len(prompts))
	for _, prompt := range prompts {
		snapshot, err := r.buildPromptSnapshot(ctx, sessionID, prompt.PromptIndex)
		if err != nil {
			return sessionViewSummary{}, nil, err
		}
		out = append(out, snapshot)
	}
	return r.sessionViewSummaryFromRecord(*rec), out, nil
}

func (r *SessionRecorder) handlePromptStartedLocked(ctx context.Context, event parsedSessionViewEvent) error {
	rawEvent := event.raw
	request, ok := event.payload.(acp.IMPromptRequest)
	if !ok {
		return fmt.Errorf("decode prompt request: unexpected payload type %T", event.payload)
	}

	state, err := r.nextPromptStateLocked(ctx, rawEvent.SessionID)
	if err != nil {
		return err
	}
	promptTitle := strings.TrimSpace(promptTitleFromBlocks(request.ContentBlocks))
	if err := r.store.UpsertSessionPrompt(ctx, SessionPromptRecord{
		SessionID:   rawEvent.SessionID,
		PromptIndex: state.promptIndex,
		Title:       promptTitle,
		UpdatedAt:   rawEvent.UpdatedAt,
	}); err != nil {
		return err
	}
	if err := r.addMessageTurn(ctx, &state, event); err != nil {
		return err
	}
	r.promptState[rawEvent.SessionID] = state
	if err := r.upsertSessionProjection(ctx, rawEvent.SessionID, "", promptTitle, rawEvent.UpdatedAt, true); err != nil {
		return err
	}
	return nil
}

func (r *SessionRecorder) handleUpdateMessageLocked(ctx context.Context, event parsedSessionViewEvent) error {
	state, err := r.currentPromptStateLocked(ctx, event.raw.SessionID)
	if err != nil {
		return err
	}
	if state == nil {
		return nil
	}
	if err := r.addMessageTurn(ctx, state, event); err != nil {
		return err
	}
	r.promptState[event.raw.SessionID] = *state
	return nil
}

func (r *SessionRecorder) addMessageTurn(ctx context.Context, state *sessionPromptState, event parsedSessionViewEvent) error {
	if state == nil {
		return fmt.Errorf("prompt state is required")
	}
	state.ensureMaps()

	turn := sessionTurnMessage{
		SessionID:   strings.TrimSpace(event.raw.SessionID),
		method:      event.method,
		payload:     event.payload,
		PromptIndex: state.promptIndex,
	}

	mergeKind, mergedTurnIndex := getTurnKindAndIndex(*state, event)
	if mergedTurnIndex > 0 {
		existingTurn, ok := state.turns[mergedTurnIndex]

		if ok {
			turn.TurnIndex = mergedTurnIndex

			merged, err := mergeTurnMessage(existingTurn, turn, mergeKind, mergedTurnIndex)
			if err != nil {
				return err
			}
			turn = merged
		}
	}

	if turn.TurnIndex <= 0 {
		turn.TurnIndex = state.nextTurnIndex
	}

	updateJSON, err := buildIMContentJSON(turn.method, turn.payload)
	if err != nil {
		return err
	}
	record := SessionTurnRecord{
		SessionID:   strings.TrimSpace(turn.SessionID),
		PromptIndex: turn.PromptIndex,
		TurnIndex:   maxInt64(turn.TurnIndex, 1),
		UpdateJSON:  updateJSON,
	}
	if err := r.store.UpsertSessionTurn(ctx, record); err != nil {
		return err
	}
	publish := r.eventPublisher()
	if publish != nil {
		_ = publish("registry.session.message", map[string]any{
			"sessionId":   strings.TrimSpace(turn.SessionID),
			"promptIndex": turn.PromptIndex,
			"turnIndex":   turn.TurnIndex,
			"content":     updateJSON,
		})
	}
	state.updateTurn(turn, event.turnKey)
	return nil
}

func (r *SessionRecorder) handlePromptFinishedLocked(ctx context.Context, parsedEvent parsedSessionViewEvent) error {
	event := parsedEvent.raw
	result, ok := parsedEvent.payload.(acp.IMPromptResult)
	if !ok {
		return fmt.Errorf("decode prompt result: unexpected payload type %T", parsedEvent.payload)
	}
	stopReason := strings.TrimSpace(result.StopReason)

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

func (r *SessionRecorder) appendPromptMessages(ctx context.Context, sessionID string, promptIndex int64, page *sessionMessagePage) error {
	messages, err := r.store.ListSessionTurns(ctx, r.projectName, sessionID, promptIndex)
	if err != nil {
		return err
	}
	for _, turn := range messages {
		page.append(turn)
	}
	return nil
}

func (r *SessionRecorder) upsertSessionProjection(ctx context.Context, sessionID, agentType, title string, updatedAt time.Time, titleIfEmptyOnly bool) error {
	rec, err := r.store.LoadSession(ctx, r.projectName, sessionID)
	if err != nil {
		return err
	}
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}
	if rec == nil {
		agentType = strings.TrimSpace(agentType)
		if agentType == "" {
			return fmt.Errorf("session agent type is required")
		}
		rec = &SessionRecord{ID: sessionID, ProjectName: r.projectName, Status: SessionActive, AgentType: agentType, CreatedAt: updatedAt, LastActiveAt: updatedAt}
	} else if strings.TrimSpace(rec.AgentType) == "" && strings.TrimSpace(agentType) != "" {
		rec.AgentType = strings.TrimSpace(agentType)
	}
	if strings.TrimSpace(rec.AgentType) == "" {
		return fmt.Errorf("session agent type is required")
	}
	title = strings.TrimSpace(title)
	if title != "" {
		if !titleIfEmptyOnly || strings.TrimSpace(rec.Title) == "" {
			rec.Title = title
		}
	}
	rec.LastActiveAt = updatedAt
	if err := r.store.SaveSession(ctx, rec); err != nil {
		return err
	}
	r.publishSessionUpdated(r.sessionViewSummaryFromRecord(*rec))
	return nil
}

func (r *SessionRecorder) sessionViewSummaryFromRecord(rec SessionRecord) sessionViewSummary {
	return buildSessionViewSummary(
		rec.ID,
		rec.Title,
		rec.LastActiveAt,
		rec.AgentType,
	)
}

func (r *SessionRecorder) publishSessionUpdated(summary sessionViewSummary) {
	publish := r.eventPublisher()
	if publish == nil {
		return
	}
	_ = publish("registry.session.updated", map[string]any{"session": summary})
}

func buildSessionViewSummary(sessionID, title string, lastActiveAt time.Time, agentType string) sessionViewSummary {
	return sessionViewSummary{
		SessionID: strings.TrimSpace(sessionID),
		Title:     firstNonEmpty(strings.TrimSpace(title), strings.TrimSpace(sessionID)),
		UpdatedAt: lastActiveAt.UTC().Format(time.RFC3339),
		AgentType: strings.TrimSpace(agentType),
	}
}

func promptsFromCheckpoint(prompts []SessionPromptRecord, checkpointPromptIndex int64) []SessionPromptRecord {
	if len(prompts) == 0 || checkpointPromptIndex <= 0 {
		return prompts
	}
	for i, prompt := range prompts {
		if prompt.PromptIndex >= checkpointPromptIndex {
			return prompts[i:]
		}
	}
	return prompts
}

func (r *SessionRecorder) buildPromptSnapshot(ctx context.Context, sessionID string, promptIndex int64) (sessionPromptSnapshot, error) {
	turns, err := r.store.ListSessionTurns(ctx, r.projectName, sessionID, promptIndex)
	if err != nil {
		return sessionPromptSnapshot{}, err
	}
	content := make([]string, 0, len(turns))
	lastTurnIndex := int64(0)
	for _, turn := range turns {
		content = append(content, normalizeJSONDoc(turn.UpdateJSON, `{}`))
		if turn.TurnIndex > lastTurnIndex {
			lastTurnIndex = turn.TurnIndex
		}
	}
	return sessionPromptSnapshot{
		SessionID:   strings.TrimSpace(sessionID),
		PromptIndex: promptIndex,
		TurnIndex:   lastTurnIndex,
		Content:     content,
	}, nil
}

func newSessionMessagePage(afterPromptIndex, afterTurnIndex int64) sessionMessagePage {
	return sessionMessagePage{
		afterPromptIndex: afterPromptIndex,
		afterTurnIndex:   afterTurnIndex,
		messages:         make([]sessionViewMessage, 0),
	}
}

func (p *sessionMessagePage) append(turn SessionTurnRecord) {
	p.advance(turn)
	if !p.includes(turn) {
		return
	}
	p.messages = append(p.messages, toSessionViewMessage(turn))
}

func (p *sessionMessagePage) advance(turn SessionTurnRecord) {
	if turn.PromptIndex > p.lastPromptIndex || (turn.PromptIndex == p.lastPromptIndex && turn.TurnIndex > p.lastTurnIndex) {
		p.lastPromptIndex = turn.PromptIndex
		p.lastTurnIndex = turn.TurnIndex
	}
}

func (p sessionMessagePage) includes(turn SessionTurnRecord) bool {
	if turn.PromptIndex < p.afterPromptIndex {
		return false
	}
	if turn.PromptIndex == p.afterPromptIndex && turn.TurnIndex <= p.afterTurnIndex {
		return false
	}
	return true
}

func newSessionPromptState(promptIndex, nextTurnIndex int64) sessionPromptState {
	if nextTurnIndex <= 0 {
		nextTurnIndex = 1
	}
	return sessionPromptState{
		promptIndex:    promptIndex,
		nextTurnIndex:  nextTurnIndex,
		turns:          map[int64]sessionTurnMessage{},
		turnIndexByKey: map[string]int64{},
	}
}

func (s *sessionPromptState) ensureMaps() {
	if s.turns == nil {
		s.turns = map[int64]sessionTurnMessage{}
	}
	if s.turnIndexByKey == nil {
		s.turnIndexByKey = map[string]int64{}
	}
	if s.nextTurnIndex <= 0 {
		s.nextTurnIndex = 1
	}
}

func (s *sessionPromptState) updateTurn(turn sessionTurnMessage, turnKey string) {
	s.ensureMaps()
	turn.SessionID = strings.TrimSpace(turn.SessionID)
	turn.method = strings.TrimSpace(turn.method)
	s.turns[turn.TurnIndex] = turn
	if turn.TurnIndex >= s.nextTurnIndex {
		s.nextTurnIndex = turn.TurnIndex + 1
	}
	if turnKey = strings.TrimSpace(turnKey); turnKey != "" {
		s.turnIndexByKey[turnKey] = turn.TurnIndex
	}
}

func (s *sessionPromptState) assignTurn(turn sessionTurnMessage, turnKey string) {
	s.updateTurn(turn, turnKey)
}

func normalizeSessionID(sessionID string) (string, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return "", fmt.Errorf("session id is required")
	}
	return sessionID, nil
}

func (r *SessionRecorder) nextPromptStateLocked(ctx context.Context, sessionID string) (sessionPromptState, error) {
	state, err := r.currentPromptStateLocked(ctx, sessionID)
	if err != nil {
		return sessionPromptState{}, err
	}
	if state == nil {
		return newSessionPromptState(1, 1), nil
	}
	return newSessionPromptState(state.promptIndex+1, 1), nil
}

func (r *SessionRecorder) ensurePromptStateLocked(ctx context.Context, sessionID string, updatedAt time.Time) (sessionPromptState, error) {
	sessionID, err := normalizeSessionID(sessionID)
	if err != nil {
		return sessionPromptState{}, err
	}
	state, err := r.currentPromptStateLocked(ctx, sessionID)
	if err != nil {
		return sessionPromptState{}, err
	}
	if state != nil {
		return *state, nil
	}
	created := newSessionPromptState(1, 1)
	if err := r.store.UpsertSessionPrompt(ctx, SessionPromptRecord{SessionID: sessionID, PromptIndex: 1, UpdatedAt: updatedAt}); err != nil {
		return sessionPromptState{}, err
	}
	r.promptState[sessionID] = created
	return created, nil
}

func (r *SessionRecorder) currentPromptStateLocked(ctx context.Context, sessionID string) (*sessionPromptState, error) {
	sessionID, err := normalizeSessionID(sessionID)
	if err != nil {
		return nil, err
	}
	if state, err := r.cachedPromptStateLocked(ctx, sessionID); state != nil || err != nil {
		return state, err
	}
	return r.loadLatestPromptStateLocked(ctx, sessionID)
}

func (r *SessionRecorder) cachedPromptStateLocked(ctx context.Context, sessionID string) (*sessionPromptState, error) {
	state, ok := r.promptState[sessionID]
	if !ok || state.promptIndex <= 0 {
		return nil, nil
	}
	prompt, err := r.store.LoadSessionPrompt(ctx, r.projectName, sessionID, state.promptIndex)
	if err != nil {
		return nil, err
	}
	if prompt == nil {
		delete(r.promptState, sessionID)
		return nil, nil
	}
	state.ensureMaps()
	return &state, nil
}

func (r *SessionRecorder) loadLatestPromptStateLocked(ctx context.Context, sessionID string) (*sessionPromptState, error) {
	prompts, err := r.store.ListSessionPrompts(ctx, r.projectName, sessionID)
	if err != nil {
		return nil, err
	}
	if len(prompts) == 0 {
		return nil, nil
	}
	latest := prompts[len(prompts)-1]
	state, err := r.restorePromptStateLocked(ctx, sessionID, latest.PromptIndex)
	if err != nil {
		return nil, err
	}
	r.promptState[sessionID] = state
	return &state, nil
}

func (r *SessionRecorder) restorePromptStateLocked(ctx context.Context, sessionID string, promptIndex int64) (sessionPromptState, error) {
	messages, err := r.store.ListSessionTurns(ctx, r.projectName, sessionID, promptIndex)
	if err != nil {
		return sessionPromptState{}, err
	}
	state := newSessionPromptState(promptIndex, 1)
	for i := range messages {
		turn, err := decodeSessionTurnMessage(messages[i])
		if err != nil {
			return sessionPromptState{}, err
		}
		state.assignTurn(turn, "")
	}
	return state, nil
}

func jsonGet(raw json.RawMessage, key string) (json.RawMessage, bool) {
	raw = json.RawMessage(bytes.TrimSpace(raw))
	key = strings.TrimSpace(key)
	if len(raw) == 0 || key == "" {
		return nil, false
	}
	obj := map[string]json.RawMessage{}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, false
	}
	value, ok := obj[key]
	if !ok {
		return nil, false
	}
	value = json.RawMessage(bytes.TrimSpace(value))
	if len(value) == 0 {
		return nil, false
	}
	return value, true
}

func jsonDecodeAt(raw json.RawMessage, path string, out any) bool {
	if out == nil {
		return false
	}
	path = strings.TrimSpace(path)
	if path == "" {
		raw = json.RawMessage(bytes.TrimSpace(raw))
		if len(raw) == 0 {
			return false
		}
		return json.Unmarshal(raw, out) == nil
	}
	key, child, hasChild := strings.Cut(path, ".")
	if hasChild && strings.Contains(child, ".") {
		return false
	}
	value, ok := jsonGet(raw, key)
	if !ok {
		return false
	}
	if hasChild {
		value, ok = jsonGet(value, child)
		if !ok {
			return false
		}
	}
	if err := json.Unmarshal(value, out); err != nil {
		return false
	}
	return true
}

func extractUpdateText(raw json.RawMessage) string {
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text
	}
	block := struct {
		Text string `json:"text,omitempty"`
	}{}
	if err := json.Unmarshal(raw, &block); err == nil {
		return block.Text
	}
	return ""
}

func parseSessionViewEvent(event SessionViewEvent) (parsedSessionViewEvent, error) {
	parsed := parsedSessionViewEvent{
		raw: event,
	}
	parsed.raw.SessionID = strings.TrimSpace(parsed.raw.SessionID)
	parsed.raw.Content = strings.TrimSpace(parsed.raw.Content)
	if parsed.raw.SessionID == "" {
		return parsed, nil
	}
	if parsed.raw.UpdatedAt.IsZero() {
		parsed.raw.UpdatedAt = time.Now().UTC()
	}

	eventType := strings.TrimSpace(string(parsed.raw.Type))
	if strings.EqualFold(eventType, string(SessionViewEventTypeACP)) {
		contentRaw := json.RawMessage(parsed.raw.Content)
		jsonDecodeAt(contentRaw, "method", &parsed.acpMethod)
		parsed.acpMethod = strings.TrimSpace(parsed.acpMethod)
		if parsed.acpMethod == "" {
			return parsedSessionViewEvent{}, fmt.Errorf("decode acp event content: %w", fmt.Errorf("session event method is required"))
		}
		switch parsed.acpMethod {
		case acp.MethodSessionNew:
			return parsed, nil
		case acp.MethodSessionPrompt:
			promptResult := acp.SessionPromptResult{}
			ok := jsonDecodeAt(contentRaw, "result", &promptResult)
			if ok {
				parsed.setJSONMessage(acp.IMMethodPromptDone, acp.IMPromptResult{StopReason: strings.TrimSpace(promptResult.StopReason)}, "")
				return parsed, nil
			}
			params := acp.SessionPromptParams{}
			jsonDecodeAt(contentRaw, "params", &params)
			parsed.setJSONMessage(acp.IMMethodPromptRequest, acp.IMPromptRequest{ContentBlocks: cloneJSON(params.Prompt)}, "")
		case acp.MethodSessionUpdate:
			params := acp.SessionUpdateParams{}
			jsonDecodeAt(contentRaw, "params", &params)
			turn, turnKey, ok, err := buildTurnMessageFromSessionUpdate(params.Update)
			if err != nil {
				return parsedSessionViewEvent{}, err
			}
			if ok {
				parsed.setJSONMessage(turn.method, turn.payload, turnKey)
			}
		default:
		}
	} else if strings.EqualFold(eventType, string(SessionViewEventTypeSystem)) {
		text := strings.TrimSpace(parsed.raw.Content)
		if text == "" {
			return parsed, nil
		}
		parsed.setJSONMessage(acp.IMMethodSystem, acp.IMTextResult{Text: text}, "")
	}

	return parsed, nil
}

func getTurnKindAndIndex(state sessionPromptState, event parsedSessionViewEvent) (sessionTurnMergeKind, int64) {
	method := event.method
	switch method {
	case acp.IMMethodAgentMessage, acp.IMMethodAgentThought, acp.IMMethodAgentPlan:
		lastTurnIndex := state.nextTurnIndex - 1
		if lastTurnIndex > 0 {
			if existing, ok := state.turns[lastTurnIndex]; ok && existing.method == method {
				return sessionTurnMergeText, lastTurnIndex
			}
		}
	case acp.IMMethodToolCall:
		if event.turnKey == "" {
			return sessionTurnMergeNone, 0
		}
		if turnIndex := state.turnIndexByKey[event.turnKey]; turnIndex > 0 {
			return sessionTurnMergeTool, turnIndex
		}
	}
	return sessionTurnMergeNone, 0
}

func mergeTurnMessage(existing, incoming sessionTurnMessage, mergeKind sessionTurnMergeKind, turnIndex int64) (sessionTurnMessage, error) {
	merged := sessionTurnMessage{
		SessionID:   strings.TrimSpace(firstNonEmpty(strings.TrimSpace(existing.SessionID), strings.TrimSpace(incoming.SessionID))),
		method:      strings.TrimSpace(firstNonEmpty(strings.TrimSpace(incoming.method), strings.TrimSpace(existing.method))),
		payload:     incoming.payload,
		PromptIndex: existing.PromptIndex,
		TurnIndex:   maxInt64(turnIndex, existing.TurnIndex),
	}
	var err error
	switch mergeKind {
	case sessionTurnMergeTool:
		merged.payload, err = mergeToolPayload(existing.payload, incoming.payload)
	case sessionTurnMergeText:
		merged.payload, err = mergeTextPayload(existing.payload, incoming.payload)
	}
	if err != nil {
		return sessionTurnMessage{}, err
	}
	return merged, nil
}

func mergeTextPayload(existing, incoming any) (any, error) {
	base, ok := existing.(acp.IMTextResult)
	if !ok {
		return nil, fmt.Errorf("merge text payload: unexpected existing type %T", existing)
	}
	inc, ok := incoming.(acp.IMTextResult)
	if !ok {
		return nil, fmt.Errorf("merge text payload: unexpected incoming type %T", incoming)
	}
	inc.Text = base.Text + inc.Text
	if strings.TrimSpace(inc.Text) == "" {
		inc.Text = base.Text
	}
	return inc, nil
}

func mergeToolPayload(existing, incoming any) (any, error) {
	base, ok := existing.(acp.IMToolResult)
	if !ok {
		return nil, fmt.Errorf("merge tool payload: unexpected existing type %T", existing)
	}
	inc, ok := incoming.(acp.IMToolResult)
	if !ok {
		return nil, fmt.Errorf("merge tool payload: unexpected incoming type %T", incoming)
	}
	if strings.TrimSpace(inc.Cmd) == "" {
		inc.Cmd = strings.TrimSpace(base.Cmd)
	}
	if strings.TrimSpace(inc.Kind) == "" {
		inc.Kind = strings.TrimSpace(base.Kind)
	}
	if strings.TrimSpace(inc.Status) == "" {
		inc.Status = strings.TrimSpace(base.Status)
	}
	if strings.TrimSpace(inc.Output) == "" {
		inc.Output = base.Output
	} else if strings.TrimSpace(base.Output) != "" {
		inc.Output = base.Output + inc.Output
	}
	return inc, nil
}

func buildTurnMessageFromSessionUpdate(update acp.SessionUpdate) (sessionTurnMessage, string, bool, error) {
	method := strings.TrimSpace(update.SessionUpdate)
	switch method {
	case acp.SessionUpdateAgentMessageChunk, acp.SessionUpdateAgentThoughtChunk, acp.SessionUpdateUserMessageChunk:
		return sessionTurnMessage{method: method, payload: acp.IMTextResult{Text: extractUpdateText(update.Content)}}, "", true, nil
	case acp.SessionUpdateToolCall, acp.SessionUpdateToolCallUpdate:
		output := extractUpdateText(update.Content)
		if strings.TrimSpace(output) == "" {
			output = stringifyRawJSON(update.RawOutput)
		}
		return sessionTurnMessage{method: acp.IMMethodToolCall, payload: acp.IMToolResult{
			Cmd:    strings.TrimSpace(update.Title),
			Kind:   strings.TrimSpace(update.Kind),
			Status: strings.TrimSpace(update.Status),
			Output: output,
		}}, strings.TrimSpace(update.ToolCallID), true, nil
	case acp.SessionUpdatePlan:
		entries := make([]acp.IMPlanResult, 0, len(update.Entries))
		for _, entry := range update.Entries {
			entries = append(entries, acp.IMPlanResult{Content: strings.TrimSpace(entry.Content), Status: strings.TrimSpace(entry.Status)})
		}
		return sessionTurnMessage{method: acp.IMMethodAgentPlan, payload: entries}, "", true, nil
	default:
		return sessionTurnMessage{}, "", false, nil
	}
}

func buildIMContentJSON(method string, payload any) (string, error) {
	message := acp.IMMessage{Method: strings.TrimSpace(method)}
	if payload != nil {
		switch value := payload.(type) {
		case json.RawMessage:
			message.Param = cloneJSONRaw(value)
		default:
			message.Param = mustJSONRaw(value)
		}
	}
	raw, err := json.Marshal(message)
	if err != nil {
		return "", err
	}
	return normalizeJSONDoc(string(raw), `{}`), nil
}

func decodeIMMessage(raw string) (acp.IMMessage, error) {
	raw = normalizeJSONDoc(raw, `{}`)
	message := acp.IMMessage{}
	if err := json.Unmarshal([]byte(raw), &message); err != nil {
		return acp.IMMessage{}, err
	}
	message.Method = strings.TrimSpace(message.Method)
	return message, nil
}

func stringifyRawJSON(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text
	}
	val := any(nil)
	if err := json.Unmarshal(raw, &val); err == nil {
		out, marshalErr := json.Marshal(val)
		if marshalErr == nil {
			return string(out)
		}
	}
	return strings.TrimSpace(string(raw))
}

func cloneJSONRaw(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	out := make([]byte, len(raw))
	copy(out, raw)
	return json.RawMessage(out)
}

func mustJSONRaw(value any) json.RawMessage {
	raw, err := json.Marshal(value)
	if err != nil {
		panic(fmt.Errorf("marshal message payload: %w", err))
	}
	return cloneJSONRaw(raw)
}

func decodeTurnPayload(message acp.IMMessage) (any, error) {
	method := strings.TrimSpace(message.Method)
	decode := func(out any) (any, error) {
		if len(message.Param) == 0 {
			return out, nil
		}
		if err := json.Unmarshal(message.Param, out); err != nil {
			return nil, err
		}
		return out, nil
	}

	switch method {
	case acp.IMMethodPromptRequest:
		payload, err := decode(&acp.IMPromptRequest{})
		if err != nil {
			return nil, err
		}
		request := payload.(*acp.IMPromptRequest)
		request.ContentBlocks = cloneSessionContentBlocks(request.ContentBlocks)
		return *request, nil
	case acp.IMMethodPromptDone:
		payload, err := decode(&acp.IMPromptResult{})
		if err != nil {
			return nil, err
		}
		return *(payload.(*acp.IMPromptResult)), nil
	case acp.IMMethodAgentMessage, acp.IMMethodAgentThought, acp.SessionUpdateUserMessageChunk, acp.IMMethodSystem:
		payload, err := decode(&acp.IMTextResult{})
		if err != nil {
			return nil, err
		}
		return *(payload.(*acp.IMTextResult)), nil
	case acp.IMMethodToolCall:
		payload, err := decode(&acp.IMToolResult{})
		if err != nil {
			return nil, err
		}
		return *(payload.(*acp.IMToolResult)), nil
	case acp.IMMethodAgentPlan:
		payload := []acp.IMPlanResult{}
		if len(message.Param) == 0 {
			return payload, nil
		}
		if err := json.Unmarshal(message.Param, &payload); err != nil {
			return nil, err
		}
		return payload, nil
	default:
		return cloneJSONRaw(message.Param), nil
	}
}

func decodeSessionTurnMessage(record SessionTurnRecord) (sessionTurnMessage, error) {
	imMessage, err := decodeIMMessage(record.UpdateJSON)
	if err != nil {
		return sessionTurnMessage{}, err
	}
	payload, err := decodeTurnPayload(imMessage)
	if err != nil {
		return sessionTurnMessage{}, err
	}
	return sessionTurnMessage{
		SessionID:   strings.TrimSpace(record.SessionID),
		method:      strings.TrimSpace(imMessage.Method),
		payload:     payload,
		PromptIndex: record.PromptIndex,
		TurnIndex:   record.TurnIndex,
	}, nil
}

func toSessionViewMessage(message SessionTurnRecord) sessionViewMessage {
	content := strings.TrimSpace(message.UpdateJSON)
	if content == "" {
		content = "{}"
	}
	return sessionViewMessage{
		SessionID:   strings.TrimSpace(message.SessionID),
		PromptIndex: message.PromptIndex,
		TurnIndex:   message.TurnIndex,
		Content:     content,
	}
}
