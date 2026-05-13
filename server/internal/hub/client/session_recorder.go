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
	SessionID     string             `json:"sessionId"`
	Title         string             `json:"title"`
	UpdatedAt     string             `json:"updatedAt"`
	AgentType     string             `json:"agentType,omitempty"`
	ConfigOptions []acp.ConfigOption `json:"configOptions,omitempty"`
}

type sessionViewPromptSnapshot struct {
	SessionID   string `json:"sessionId"`
	PromptIndex int64  `json:"promptIndex"`
	TurnIndex   int64  `json:"turnIndex"`
	ModelName   string `json:"modelName"`
	DurationMs  int64  `json:"durationMs"`
	Finished    bool   `json:"finished"`
}

type sessionViewMessage struct {
	SessionID   string `json:"sessionId"`
	PromptIndex int64  `json:"promptIndex"`
	TurnIndex   int64  `json:"turnIndex"`
	Content     string `json:"content"`
	Finished    bool   `json:"finished"`
}

type sessionTurnMessage struct {
	sessionID   string
	method      string
	payload     any
	promptIndex int64
	turnIndex   int64
	finished    bool
}

type sessionTurnReadMessage struct {
	content  string
	finished bool
}

type sessionPromptState struct {
	promptIndex   int64
	nextTurnIndex int64

	turns          []sessionTurnMessage
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

type SessionRecorder struct {
	projectName  string
	store        Store
	listSessions func(context.Context) ([]SessionRecord, error)

	mu      sync.Mutex
	publish func(method string, payload any) error

	writeMu     sync.Mutex
	promptState map[string]*sessionPromptState

	modelLookup func(sessionID string) string
}

func newSessionRecorder(projectName string, store Store, listSessions func(context.Context) ([]SessionRecord, error)) *SessionRecorder {
	return &SessionRecorder{
		projectName:  projectName,
		store:        store,
		listSessions: listSessions,
		promptState:  map[string]*sessionPromptState{},
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
	r.promptState = map[string]*sessionPromptState{}
	r.writeMu.Unlock()
}

func (r *SessionRecorder) ResetPromptState() {
	if r == nil {
		return
	}
	r.writeMu.Lock()
	r.promptState = map[string]*sessionPromptState{}
	r.writeMu.Unlock()
}

func (r *SessionRecorder) RemovePromptState(sessionID string) {
	if r == nil {
		return
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}
	r.writeMu.Lock()
	delete(r.promptState, sessionID)
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
	e.method = method
	e.payload = payload
	e.turnKey = turnKey
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

func (r *SessionRecorder) ReadSessionPrompts(ctx context.Context, sessionID string, afterPromptIndex, afterTurnIndex int64) (sessionViewSummary, []sessionViewPromptSnapshot, []sessionViewMessage, error) {
	sessionID = strings.TrimSpace(sessionID)
	rec, err := r.store.LoadSession(ctx, r.projectName, sessionID)
	if err != nil {
		return sessionViewSummary{}, nil, nil, err
	}
	if rec == nil {
		return sessionViewSummary{}, nil, nil, fmt.Errorf("session not found: %s", sessionID)
	}
	prompts, err := r.store.ListSessionPrompts(ctx, r.projectName, sessionID)
	if err != nil {
		return sessionViewSummary{}, nil, nil, err
	}
	var snapshot []sessionViewPromptSnapshot
	var messages []sessionViewMessage
	for _, prompt := range prompts {
		turns, err := r.promptTurnsForRead(ctx, sessionID, prompt.PromptIndex)
		if err != nil {
			return sessionViewSummary{}, nil, nil, err
		}
		// Build prompt snapshot with model + duration.
		finished := sessionPromptRecordFinished(prompt)
		ps := sessionViewPromptSnapshot{
			SessionID:   sessionID,
			PromptIndex: prompt.PromptIndex,
			ModelName:   strings.TrimSpace(prompt.ModelName),
			TurnIndex:   prompt.TurnIndex,
			Finished:    finished,
		}
		if !prompt.StartedAt.IsZero() {
			endTime := prompt.UpdatedAt
			if !finished {
				endTime = time.Now().UTC()
			}
			ps.DurationMs = endTime.Sub(prompt.StartedAt).Milliseconds()
		}
		snapshot = append(snapshot, ps)
		for i, updateJSON := range turns {
			turnIndex := int64(i + 1)
			if prompt.PromptIndex < afterPromptIndex {
				continue
			}
			if prompt.PromptIndex == afterPromptIndex && turnIndex <= afterTurnIndex {
				continue
			}
			messages = append(messages, sessionViewMessage{
				SessionID:   sessionID,
				PromptIndex: prompt.PromptIndex,
				TurnIndex:   turnIndex,
				Content:     normalizeJSONDoc(updateJSON.content, "{}"),
				Finished:    updateJSON.finished,
			})
		}
	}
	return r.sessionViewSummaryFromRecord(*rec), snapshot, messages, nil
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
	modelName := ""
	if r.modelLookup != nil {
		modelName = strings.TrimSpace(r.modelLookup(rawEvent.SessionID))
	}
	if err := r.store.UpsertSessionPrompt(ctx, SessionPromptRecord{
		SessionID:   rawEvent.SessionID,
		PromptIndex: state.promptIndex,
		Title:       promptTitle,
		UpdatedAt:   rawEvent.UpdatedAt,
		ModelName:   modelName,
		StartedAt:   rawEvent.UpdatedAt,
	}); err != nil {
		return err
	}
	if err := r.addMessageTurn(state, event); err != nil {
		return err
	}
	r.promptState[rawEvent.SessionID] = state
	if err := r.upsertSessionProjection(ctx, rawEvent.SessionID, "", promptTitle, rawEvent.UpdatedAt, false); err != nil {
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
	return r.addMessageTurn(state, event)
}

func (r *SessionRecorder) addMessageTurn(state *sessionPromptState, event parsedSessionViewEvent) error {
	if state == nil {
		return fmt.Errorf("prompt state is required")
	}
	state.ensureMaps()

	turn := sessionTurnMessage{
		sessionID:   event.raw.SessionID,
		method:      event.method,
		payload:     event.payload,
		promptIndex: state.promptIndex,
		finished:    !isSessionTextTurnMethod(event.method),
	}

	mergedTurnIndex := int64(0)
	switch event.method {
	case acp.IMMethodToolCall:
		if event.turnKey != "" {
			mergedTurnIndex = state.turnIndexByKey[event.turnKey]
		}
	case acp.IMMethodAgentMessage, acp.IMMethodAgentThought:
		lastTurnIndex := int64(len(state.turns))
		if lastTurnIndex > 0 {
			if existing := state.turns[lastTurnIndex-1]; existing.method == event.method {
				mergedTurnIndex = lastTurnIndex
			}
		}
	}
	if mergedTurnIndex > 0 {
		if idx := int(mergedTurnIndex - 1); idx >= 0 && idx < len(state.turns) {
			existingTurn := state.turns[idx]
			turn.turnIndex = mergedTurnIndex
			turn = mergeTurnMessage(existingTurn, turn, mergedTurnIndex)
		}
	}

	if turn.turnIndex <= 0 {
		r.publishOpenTextTurnDone(state)
		turn.turnIndex = state.nextTurnIndex
	}

	updateJSON := buildIMContentJSON(turn.method, turn.payload)
	r.publishSessionTurn(turn, updateJSON)
	state.updateTurn(turn, event.turnKey)
	return nil
}

func (r *SessionRecorder) publishOpenTextTurnDone(state *sessionPromptState) {
	if state == nil || len(state.turns) == 0 {
		return
	}
	idx := len(state.turns) - 1
	turn := state.turns[idx]
	if turn.finished || !isSessionTextTurnMethod(turn.method) {
		return
	}
	turn.finished = true
	state.turns[idx] = turn
	r.publishSessionTurn(turn, buildIMContentJSON(turn.method, turn.payload))
}

func (r *SessionRecorder) handlePromptFinishedLocked(ctx context.Context, parsedEvent parsedSessionViewEvent) error {
	event := parsedEvent.raw
	result, ok := parsedEvent.payload.(acp.IMPromptResult)
	if !ok {
		return fmt.Errorf("decode prompt result: unexpected payload type %T", parsedEvent.payload)
	}
	stopReason := strings.TrimSpace(result.StopReason)

	alreadyFinished, err := r.latestPromptFinishedWithoutLiveStateLocked(ctx, event.SessionID)
	if err != nil {
		return err
	}
	if alreadyFinished {
		return nil
	}

	state, err := r.ensurePromptStateLocked(ctx, event.SessionID, event.UpdatedAt)
	if err != nil {
		return err
	}
	r.publishOpenTextTurnDone(state)
	doneTurn := sessionTurnMessage{
		sessionID:   event.SessionID,
		method:      acp.IMMethodPromptDone,
		payload:     acp.IMPromptResult{StopReason: stopReason},
		promptIndex: state.promptIndex,
		turnIndex:   state.nextTurnIndex,
		finished:    true,
	}
	state.updateTurn(doneTurn, "")
	turnsJSON, turnIndex := encodePromptStateTurns(state)
	if err := r.store.UpsertSessionPrompt(ctx, SessionPromptRecord{
		SessionID:   event.SessionID,
		PromptIndex: state.promptIndex,
		StopReason:  stopReason,
		UpdatedAt:   event.UpdatedAt,
		TurnsJSON:   turnsJSON,
		TurnIndex:   turnIndex,
	}); err != nil {
		return err
	}
	if err := r.upsertSessionProjection(ctx, event.SessionID, "", "", event.UpdatedAt, true); err != nil {
		return err
	}
	r.publishSessionTurn(doneTurn, buildIMContentJSON(doneTurn.method, doneTurn.payload))
	delete(r.promptState, event.SessionID)
	return nil
}

func (r *SessionRecorder) latestPromptFinishedWithoutLiveStateLocked(ctx context.Context, sessionID string) (bool, error) {
	if state, ok := r.promptState[sessionID]; ok && state != nil {
		return false, nil
	}
	prompts, err := r.store.ListSessionPrompts(ctx, r.projectName, sessionID)
	if err != nil {
		return false, err
	}
	if len(prompts) == 0 {
		return false, nil
	}
	return sessionPromptRecordFinished(prompts[len(prompts)-1]), nil
}

func (r *SessionRecorder) publishSessionTurn(turn sessionTurnMessage, updateJSON string) {
	publish := r.eventPublisher()
	if publish == nil {
		return
	}
	_ = publish("registry.session.message", map[string]any{
		"sessionId":   turn.sessionID,
		"promptIndex": turn.promptIndex,
		"turnIndex":   turn.turnIndex,
		"content":     updateJSON,
		"finished":    turn.finished,
	})
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

// encodePromptStateTurns serialises all in-memory turns into the turns_json format
// and returns the JSON string plus the maximum turn index seen.
func encodePromptStateTurns(state *sessionPromptState) (string, int64) {
	if state == nil || len(state.turns) == 0 {
		return "", 0
	}
	encodedTurns := make([]string, 0, len(state.turns))
	for _, turn := range state.turns {
		updateJSON := buildIMContentJSON(turn.method, turn.payload)
		encodedTurns = append(encodedTurns, updateJSON)
	}
	return EncodeStoredTurns(encodedTurns), int64(len(encodedTurns))
}

// promptTurnsForRead returns the persisted turns for a prompt.
// For finished prompts the turns come from session_prompts.turns_json.
// For the active (in-flight) prompt, turns are taken from in-memory state.
func (r *SessionRecorder) promptTurnsForRead(ctx context.Context, sessionID string, promptIndex int64) ([]sessionTurnReadMessage, error) {
	prompt, err := r.store.LoadSessionPrompt(ctx, r.projectName, sessionID, promptIndex)
	if err != nil {
		return nil, err
	}
	if prompt != nil && sessionPromptRecordFinished(*prompt) {
		// Finished prompt – read from persisted turns_json.
		turns, err := DecodeStoredTurns(prompt.TurnsJSON)
		if err != nil {
			return nil, err
		}
		return sessionTurnReadMessagesFromStored(turns, true), nil
	}
	// Active (in-flight) prompt – read from in-memory state.
	r.writeMu.Lock()
	state, ok := r.promptState[sessionID]
	if !ok || state == nil || state.promptIndex != promptIndex {
		r.writeMu.Unlock()
		// No live state available; return persisted turns_json.
		if prompt != nil {
			turns, err := DecodeStoredTurns(prompt.TurnsJSON)
			if err != nil {
				return nil, err
			}
			return sessionTurnReadMessagesFromStored(turns, false), nil
		}
		return nil, nil
	}
	// If in-memory state has no turns yet (e.g. restored after restart),
	// fall back to persisted turns_json so reads don't return empty results.
	if len(state.turns) == 0 {
		r.writeMu.Unlock()
		if prompt != nil {
			turns, err := DecodeStoredTurns(prompt.TurnsJSON)
			if err != nil {
				return nil, err
			}
			return sessionTurnReadMessagesFromStored(turns, false), nil
		}
		return nil, nil
	}
	// Snapshot in-memory turns (while holding the lock).
	updates := make([]sessionTurnReadMessage, 0, len(state.turns))
	for _, turn := range state.turns {
		updateJSON := buildIMContentJSON(turn.method, turn.payload)
		updates = append(updates, sessionTurnReadMessage{content: updateJSON, finished: turn.finished})
	}
	r.writeMu.Unlock()
	return updates, nil
}

func sessionTurnReadMessagesFromStored(turns []string, finished bool) []sessionTurnReadMessage {
	out := make([]sessionTurnReadMessage, 0, len(turns))
	for _, turn := range turns {
		out = append(out, sessionTurnReadMessage{
			content:  turn,
			finished: finished || !isSessionTextTurnContent(turn),
		})
	}
	return out
}

func sessionPromptRecordFinished(prompt SessionPromptRecord) bool {
	return strings.TrimSpace(prompt.StopReason) != "" ||
		(prompt.TurnIndex > 0 && strings.TrimSpace(prompt.TurnsJSON) != "")
}

func isSessionTextTurnContent(content string) bool {
	msg := acp.IMTurnMessage{}
	if err := json.Unmarshal([]byte(content), &msg); err != nil {
		return false
	}
	return isSessionTextTurnMethod(msg.Method)
}

func newSessionPromptState(promptIndex, nextTurnIndex int64) sessionPromptState {
	if nextTurnIndex <= 0 {
		nextTurnIndex = 1
	}
	return sessionPromptState{
		promptIndex:    promptIndex,
		nextTurnIndex:  nextTurnIndex,
		turns:          make([]sessionTurnMessage, 0),
		turnIndexByKey: map[string]int64{},
	}
}

func (s *sessionPromptState) ensureMaps() {
	if s.turns == nil {
		s.turns = make([]sessionTurnMessage, 0)
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
	if turn.turnIndex <= 0 {
		turn.turnIndex = s.nextTurnIndex
	}
	idx := int(turn.turnIndex - 1)
	if idx >= 0 && idx < len(s.turns) {
		s.turns[idx] = turn
	} else {
		turn.turnIndex = int64(len(s.turns) + 1)
		s.turns = append(s.turns, turn)
	}
	s.nextTurnIndex = int64(len(s.turns) + 1)
	if turnKey != "" {
		s.turnIndexByKey[turnKey] = turn.turnIndex
	}
}

func (r *SessionRecorder) nextPromptStateLocked(ctx context.Context, sessionID string) (*sessionPromptState, error) {
	state, err := r.currentPromptStateLocked(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if state == nil {
		created := newSessionPromptState(1, 1)
		return &created, nil
	}
	next := newSessionPromptState(state.promptIndex+1, 1)
	return &next, nil
}

func (r *SessionRecorder) ensurePromptStateLocked(ctx context.Context, sessionID string, updatedAt time.Time) (*sessionPromptState, error) {
	state, err := r.currentPromptStateLocked(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if state != nil {
		return state, nil
	}
	created := newSessionPromptState(1, 1)
	if err := r.store.UpsertSessionPrompt(ctx, SessionPromptRecord{
		SessionID:   sessionID,
		PromptIndex: 1,
		UpdatedAt:   updatedAt,
		StartedAt:   updatedAt,
	}); err != nil {
		return nil, err
	}
	r.promptState[sessionID] = &created
	return &created, nil
}

func (r *SessionRecorder) currentPromptStateLocked(ctx context.Context, sessionID string) (*sessionPromptState, error) {
	if state, err := r.cachedPromptStateLocked(ctx, sessionID); state != nil || err != nil {
		return state, err
	}
	return r.loadLatestPromptStateLocked(ctx, sessionID)
}

func (r *SessionRecorder) cachedPromptStateLocked(ctx context.Context, sessionID string) (*sessionPromptState, error) {
	state, ok := r.promptState[sessionID]
	if !ok || state == nil || state.promptIndex <= 0 {
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
	return state, nil
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
	nextTurnIndex := latest.TurnIndex + 1
	if nextTurnIndex <= 0 {
		nextTurnIndex = 1
	}
	created := newSessionPromptState(latest.PromptIndex, nextTurnIndex)
	state := &created
	r.promptState[sessionID] = state
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
			method := strings.TrimSpace(params.Update.SessionUpdate)
			if method == "" {
				return parsed, nil
			}
			switch method {
			case acp.SessionUpdateAgentMessageChunk, acp.SessionUpdateAgentThoughtChunk, acp.SessionUpdateUserMessageChunk:
				parsed.setJSONMessage(method, acp.IMTextResult{Text: extractUpdateText(params.Update.Content)}, "")
			case acp.SessionUpdateToolCall, acp.SessionUpdateToolCallUpdate:
				parsed.setJSONMessage(acp.IMMethodToolCall, acp.IMToolResult{
					Cmd:    strings.TrimSpace(params.Update.Title),
					Kind:   strings.TrimSpace(params.Update.Kind),
					Status: strings.TrimSpace(params.Update.Status),
				}, strings.TrimSpace(params.Update.ToolCallID))
			case acp.SessionUpdatePlan:
				entries := make([]acp.IMPlanResult, 0, len(params.Update.Entries))
				for _, entry := range params.Update.Entries {
					entries = append(entries, acp.IMPlanResult{Content: strings.TrimSpace(entry.Content), Status: strings.TrimSpace(entry.Status)})
				}
				parsed.setJSONMessage(acp.IMMethodAgentPlan, entries, "")
			default:
				return parsedSessionViewEvent{}, fmt.Errorf("unsupported session update type: %s", method)
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

func mergeTurnMessage(existing, incoming sessionTurnMessage, turnIndex int64) sessionTurnMessage {
	existing.sessionID = firstNonEmpty(existing.sessionID, incoming.sessionID)
	existing.method = firstNonEmpty(incoming.method, existing.method)
	existing.turnIndex = maxInt64(turnIndex, existing.turnIndex)
	if existing.promptIndex <= 0 {
		existing.promptIndex = incoming.promptIndex
	}
	switch incoming.method {
	case acp.IMMethodToolCall:
		base := existing.payload.(acp.IMToolResult)
		inc := incoming.payload.(acp.IMToolResult)
		if inc.Cmd == "" {
			inc.Cmd = base.Cmd
		}
		if inc.Kind == "" {
			inc.Kind = base.Kind
		}
		if inc.Status == "" {
			inc.Status = base.Status
		}
		existing.payload = inc
	case acp.IMMethodAgentMessage, acp.IMMethodAgentThought:
		base := existing.payload.(acp.IMTextResult)
		inc := incoming.payload.(acp.IMTextResult)
		inc.Text = base.Text + inc.Text
		if inc.Text == "" {
			inc.Text = base.Text
		}
		existing.payload = inc
	default:
		existing.payload = incoming.payload
	}
	return existing
}

func isSessionTextTurnMethod(method string) bool {
	switch strings.TrimSpace(method) {
	case acp.IMMethodAgentMessage, acp.IMMethodAgentThought:
		return true
	default:
		return false
	}
}

func buildIMContentJSON(method string, payload any) string {
	message := acp.IMTurnMessage{Method: method}
	if payload != nil {
		message.Param = mustJSONRaw(payload)
	}
	raw, _ := json.Marshal(message)
	return string(raw)
}

func mustJSONRaw(value any) json.RawMessage {
	raw, err := json.Marshal(value)
	if err != nil {
		panic(fmt.Errorf("marshal message payload: %w", err))
	}
	return json.RawMessage(raw)
}
