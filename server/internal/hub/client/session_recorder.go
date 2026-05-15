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
	SessionID         string             `json:"sessionId"`
	Title             string             `json:"title"`
	UpdatedAt         string             `json:"updatedAt"`
	AgentType         string             `json:"agentType,omitempty"`
	LatestTurnIndex   int64              `json:"latestTurnIndex"`
	Running           bool               `json:"running"`
	LastDoneTurnIndex int64              `json:"lastDoneTurnIndex"`
	LastDoneSuccess   bool               `json:"lastDoneSuccess"`
	LastReadTurnIndex int64              `json:"lastReadTurnIndex"`
	ConfigOptions     []acp.ConfigOption `json:"configOptions,omitempty"`
}

type sessionSyncProjection struct {
	LatestPersistedTurnIndex int64 `json:"latestPersistedTurnIndex"`
	LastDoneTurnIndex        int64 `json:"lastDoneTurnIndex,omitempty"`
	LastDoneSuccess          *bool `json:"lastDoneSuccess,omitempty"`
	LastReadTurnIndex        int64 `json:"lastReadTurnIndex,omitempty"`
}

type sessionTurnMessage struct {
	sessionID string
	method    string
	payload   any
	turnIndex int64
	finished  bool
}

type sessionPromptState struct {
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
	turnStore    *fileSessionTurnStore
	listSessions func(context.Context) ([]SessionRecord, error)

	mu      sync.Mutex
	publish func(method string, payload any) error

	writeMu       sync.Mutex
	promptState   map[string]*sessionPromptState
	nextTurnIndex map[string]int64
	finishedTurns map[string][]sessionViewTurn

	modelLookup func(sessionID string) string
}

func newSessionRecorder(projectName string, store Store, listSessions func(context.Context) ([]SessionRecord, error)) *SessionRecorder {
	return &SessionRecorder{
		projectName:   projectName,
		store:         store,
		listSessions:  listSessions,
		promptState:   map[string]*sessionPromptState{},
		nextTurnIndex: map[string]int64{},
		finishedTurns: map[string][]sessionViewTurn{},
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
	r.nextTurnIndex = map[string]int64{}
	r.finishedTurns = map[string][]sessionViewTurn{}
	r.writeMu.Unlock()
}

func (r *SessionRecorder) ResetPromptState() {
	if r == nil {
		return
	}
	r.writeMu.Lock()
	r.promptState = map[string]*sessionPromptState{}
	r.nextTurnIndex = map[string]int64{}
	r.finishedTurns = map[string][]sessionViewTurn{}
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
	delete(r.nextTurnIndex, sessionID)
	delete(r.finishedTurns, sessionID)
	r.writeMu.Unlock()
}

func (r *SessionRecorder) ResetSessionTurns(ctx context.Context, sessionID string) error {
	if r == nil {
		return nil
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil
	}
	r.RemovePromptState(sessionID)
	if r.turnStore == nil {
		return nil
	}
	return r.turnStore.DeleteTurns(ctx, r.projectName, sessionID)
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

func (r *SessionRecorder) ReadSessionSummary(ctx context.Context, sessionID string) (sessionViewSummary, error) {
	sessionID = strings.TrimSpace(sessionID)
	rec, err := r.store.LoadSession(ctx, r.projectName, sessionID)
	if err != nil {
		return sessionViewSummary{}, err
	}
	if rec == nil {
		return sessionViewSummary{}, fmt.Errorf("session not found: %s", sessionID)
	}
	return r.sessionViewSummaryFromRecord(*rec), nil
}

func (r *SessionRecorder) ReadSessionTurns(ctx context.Context, sessionID string, afterTurnIndex int64) (int64, []sessionViewTurn, error) {
	sessionID = strings.TrimSpace(sessionID)
	rec, err := r.store.LoadSession(ctx, r.projectName, sessionID)
	if err != nil {
		return 0, nil, err
	}
	if rec == nil {
		return 0, nil, fmt.Errorf("session not found: %s", sessionID)
	}
	if afterTurnIndex < 0 {
		afterTurnIndex = 0
	}
	persistedLatest := sessionSyncLatestPersistedTurnIndex(rec.SessionSyncJSON)
	turns := []sessionViewTurn{}
	if persistedLatest > afterTurnIndex {
		if r.turnStore != nil {
			readTurns, err := r.turnStore.ReadTurns(ctx, r.projectName, sessionID, afterTurnIndex, persistedLatest)
			if err != nil {
				return 0, nil, err
			}
			turns = append(turns, readTurns...)
		} else {
			r.writeMu.Lock()
			for _, turn := range r.finishedTurns[sessionID] {
				if turn.TurnIndex > afterTurnIndex && turn.TurnIndex <= persistedLatest {
					turns = append(turns, turn)
				}
			}
			r.writeMu.Unlock()
		}
	}
	latestTurnIndex := persistedLatest
	r.writeMu.Lock()
	if state := r.promptState[sessionID]; state != nil {
		for _, turn := range sortedSessionTurns(state.turns) {
			if turn.turnIndex > latestTurnIndex {
				latestTurnIndex = turn.turnIndex
			}
			if turn.turnIndex <= afterTurnIndex || turn.turnIndex <= persistedLatest {
				continue
			}
			turns = append(turns, sessionViewTurn{
				SessionID: sessionID,
				TurnIndex: turn.turnIndex,
				Content:   buildIMContentJSON(turn.method, turn.payload),
				Finished:  turn.finished,
			})
		}
	}
	r.writeMu.Unlock()
	sort.Slice(turns, func(i, j int) bool {
		return turns[i].TurnIndex < turns[j].TurnIndex
	})
	return latestTurnIndex, turns, nil
}

func (r *SessionRecorder) MarkSessionRead(ctx context.Context, sessionID string, lastReadTurnIndex int64) (sessionViewSummary, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return sessionViewSummary{}, fmt.Errorf("sessionId is required")
	}
	if lastReadTurnIndex < 0 {
		lastReadTurnIndex = 0
	}
	rec, err := r.store.LoadSession(ctx, r.projectName, sessionID)
	if err != nil {
		return sessionViewSummary{}, err
	}
	if rec == nil {
		return sessionViewSummary{}, fmt.Errorf("session not found: %s", sessionID)
	}
	projection := sessionSyncProjectionFromJSON(rec.SessionSyncJSON)
	if projection.LastDoneTurnIndex > 0 && lastReadTurnIndex > projection.LastDoneTurnIndex {
		lastReadTurnIndex = projection.LastDoneTurnIndex
	}
	if lastReadTurnIndex > projection.LastReadTurnIndex {
		projection.LastReadTurnIndex = lastReadTurnIndex
		rec.SessionSyncJSON = sessionSyncProjectionJSON(projection)
		if err := r.store.SaveSession(ctx, rec); err != nil {
			return sessionViewSummary{}, err
		}
		summary := r.sessionViewSummaryFromRecord(*rec)
		r.publishSessionUpdated(summary)
		return summary, nil
	}
	return r.sessionViewSummaryFromRecord(*rec), nil
}

func (r *SessionRecorder) handlePromptStartedLocked(ctx context.Context, event parsedSessionViewEvent) error {
	rawEvent := event.raw
	request, ok := event.payload.(acp.IMPromptRequest)
	if !ok {
		return fmt.Errorf("decode prompt request: unexpected payload type %T", event.payload)
	}

	state, err := r.nextPromptStateLocked(ctx, rawEvent.SessionID, rawEvent.UpdatedAt)
	if err != nil {
		return err
	}
	promptTitle := strings.TrimSpace(promptTitleFromBlocks(request.ContentBlocks))
	modelName := ""
	if r.modelLookup != nil {
		modelName = strings.TrimSpace(r.modelLookup(rawEvent.SessionID))
	}
	request.ModelName = modelName
	request.CreatedAt = rawEvent.UpdatedAt.UTC().Format(time.RFC3339Nano)
	event.payload = request
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
		sessionID: event.raw.SessionID,
		method:    event.method,
		payload:   event.payload,
		finished:  !isSessionTextTurnMethod(event.method),
	}

	mergedTurnIndex := int64(0)
	switch event.method {
	case acp.IMMethodToolCall:
		if event.turnKey != "" {
			mergedTurnIndex = state.turnIndexByKey[event.turnKey]
		}
	case acp.IMMethodAgentMessage, acp.IMMethodAgentThought:
		if len(state.turns) > 0 {
			if existing := state.turns[len(state.turns)-1]; existing.method == event.method {
				mergedTurnIndex = existing.turnIndex
			}
		}
	}
	if mergedTurnIndex > 0 {
		for _, existingTurn := range state.turns {
			if existingTurn.turnIndex == mergedTurnIndex {
				turn.turnIndex = mergedTurnIndex
				turn = mergeTurnMessage(existingTurn, turn, mergedTurnIndex)
				break
			}
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

	state, err := r.ensurePromptStateLocked(ctx, event.SessionID)
	if err != nil {
		return err
	}
	return r.finishPromptStateLocked(ctx, event.SessionID, state, stopReason, event.UpdatedAt, true)
}

func (r *SessionRecorder) finishPromptStateLocked(ctx context.Context, sessionID string, state *sessionPromptState, stopReason string, updatedAt time.Time, publishDone bool) error {
	if state == nil {
		return nil
	}
	stopReason = strings.TrimSpace(stopReason)
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}
	needsPromptDone := !sessionPromptStateTerminal(state)
	r.publishOpenTextTurnDone(state)

	var doneTurn sessionTurnMessage
	if needsPromptDone {
		doneTurn = sessionTurnMessage{
			sessionID: sessionID,
			method:    acp.IMMethodPromptDone,
			payload:   acp.IMPromptResult{StopReason: stopReason, CompletedAt: updatedAt.UTC().Format(time.RFC3339Nano)},
			turnIndex: state.nextTurnIndex,
			finished:  true,
		}
		state.updateTurn(doneTurn, "")
	}

	if err := r.persistSessionStateTurnsLocked(ctx, sessionID, state, updatedAt); err != nil {
		return err
	}
	if err := r.upsertSessionProjection(ctx, sessionID, "", "", updatedAt, true); err != nil {
		return err
	}
	if publishDone && needsPromptDone {
		r.publishSessionTurn(doneTurn, buildIMContentJSON(doneTurn.method, doneTurn.payload))
	}
	r.nextTurnIndex[sessionID] = state.nextTurnIndex
	delete(r.promptState, sessionID)
	return nil
}

func (r *SessionRecorder) latestPromptFinishedWithoutLiveStateLocked(ctx context.Context, sessionID string) (bool, error) {
	if state, ok := r.promptState[sessionID]; ok && state != nil {
		return false, nil
	}
	rec, err := r.store.LoadSession(ctx, r.projectName, sessionID)
	if err != nil {
		return false, err
	}
	if rec == nil {
		return false, nil
	}
	return sessionSyncLatestPersistedTurnIndex(rec.SessionSyncJSON) > 0, nil
}

func (r *SessionRecorder) publishSessionTurn(turn sessionTurnMessage, updateJSON string) {
	publish := r.eventPublisher()
	if publish == nil {
		return
	}
	_ = publish("registry.session.message", map[string]any{
		"sessionId": turn.sessionID,
		"turnIndex": turn.turnIndex,
		"content":   updateJSON,
		"finished":  turn.finished,
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
	r.publishSessionUpdated(r.sessionViewSummaryFromRecordLocked(*rec))
	return nil
}

func (r *SessionRecorder) sessionViewSummaryFromRecord(rec SessionRecord) sessionViewSummary {
	r.writeMu.Lock()
	defer r.writeMu.Unlock()
	return r.sessionViewSummaryFromRecordLocked(rec)
}

func (r *SessionRecorder) sessionViewSummaryFromRecordLocked(rec SessionRecord) sessionViewSummary {
	projection := sessionSyncProjectionFromJSON(rec.SessionSyncJSON)
	latestTurnIndex := projection.LatestPersistedTurnIndex
	running := false
	if state := r.promptState[rec.ID]; state != nil {
		for _, turn := range state.turns {
			if turn.turnIndex > latestTurnIndex {
				latestTurnIndex = turn.turnIndex
			}
		}
		running = !sessionPromptStateTerminal(state)
	}
	for _, turn := range r.finishedTurns[rec.ID] {
		if turn.TurnIndex > latestTurnIndex {
			latestTurnIndex = turn.TurnIndex
		}
	}
	return buildSessionViewSummary(
		rec.ID,
		rec.Title,
		rec.LastActiveAt,
		rec.AgentType,
		latestTurnIndex,
		running,
		projection.LastDoneTurnIndex,
		projection.LastDoneSuccess != nil && *projection.LastDoneSuccess,
		projection.LastReadTurnIndex,
	)
}

func (r *SessionRecorder) publishSessionUpdated(summary sessionViewSummary) {
	publish := r.eventPublisher()
	if publish == nil {
		return
	}
	_ = publish("registry.session.updated", map[string]any{"session": summary})
}

func buildSessionViewSummary(
	sessionID, title string,
	lastActiveAt time.Time,
	agentType string,
	latestTurnIndex int64,
	running bool,
	lastDoneTurnIndex int64,
	lastDoneSuccess bool,
	lastReadTurnIndex int64,
) sessionViewSummary {
	return sessionViewSummary{
		SessionID:         strings.TrimSpace(sessionID),
		Title:             strings.TrimSpace(title),
		UpdatedAt:         lastActiveAt.UTC().Format(time.RFC3339),
		AgentType:         strings.TrimSpace(agentType),
		LatestTurnIndex:   maxInt64(0, latestTurnIndex),
		Running:           running,
		LastDoneTurnIndex: maxInt64(0, lastDoneTurnIndex),
		LastDoneSuccess:   lastDoneSuccess,
		LastReadTurnIndex: maxInt64(0, lastReadTurnIndex),
	}
}

func (r *SessionRecorder) persistSessionStateTurnsLocked(ctx context.Context, sessionID string, state *sessionPromptState, updatedAt time.Time) error {
	if state == nil || len(state.turns) == 0 {
		return nil
	}
	rec, err := r.store.LoadSession(ctx, r.projectName, sessionID)
	if err != nil {
		return err
	}
	if rec == nil {
		return fmt.Errorf("session not found: %s", sessionID)
	}
	persistedLatest := sessionSyncLatestPersistedTurnIndex(rec.SessionSyncJSON)
	ordered := sortedSessionTurns(state.turns)
	contents := make([]string, 0, len(ordered))
	startTurnIndex := int64(0)
	latestTurnIndex := persistedLatest
	lastDoneTurnIndex := int64(0)
	lastDoneSuccess := false
	for _, turn := range ordered {
		if turn.turnIndex <= persistedLatest {
			continue
		}
		if startTurnIndex == 0 {
			startTurnIndex = turn.turnIndex
		}
		contents = append(contents, buildIMContentJSON(turn.method, turn.payload))
		latestTurnIndex = turn.turnIndex
		if turn.method == acp.IMMethodPromptDone {
			lastDoneTurnIndex = turn.turnIndex
			lastDoneSuccess = sessionDoneTurnSuccess(turn)
		}
	}
	if len(contents) == 0 {
		return nil
	}
	if startTurnIndex != persistedLatest+1 {
		return fmt.Errorf("session %s turn persistence gap: start=%d latest=%d", sessionID, startTurnIndex, persistedLatest)
	}
	if r.turnStore != nil {
		latest, err := r.turnStore.WriteTurns(ctx, r.projectName, sessionID, startTurnIndex, contents)
		if err != nil {
			return err
		}
		latestTurnIndex = latest
	} else {
		for i, content := range contents {
			r.finishedTurns[sessionID] = append(r.finishedTurns[sessionID], sessionViewTurn{
				SessionID: sessionID,
				TurnIndex: startTurnIndex + int64(i),
				Content:   normalizeJSONDoc(content, "{}"),
				Finished:  true,
			})
		}
	}
	projection := sessionSyncProjectionFromJSON(rec.SessionSyncJSON)
	projection.LatestPersistedTurnIndex = latestTurnIndex
	if lastDoneTurnIndex > 0 {
		projection.LastDoneTurnIndex = lastDoneTurnIndex
		projection.LastDoneSuccess = boolPtr(lastDoneSuccess)
	}
	rec.SessionSyncJSON = sessionSyncProjectionJSON(projection)
	rec.LastActiveAt = updatedAt
	if err := r.store.SaveSession(ctx, rec); err != nil {
		return err
	}
	return nil
}

func sessionSyncLatestPersistedTurnIndex(raw string) int64 {
	return sessionSyncProjectionFromJSON(raw).LatestPersistedTurnIndex
}

func sessionSyncJSON(latestPersistedTurnIndex int64) string {
	if latestPersistedTurnIndex < 0 {
		latestPersistedTurnIndex = 0
	}
	raw, err := json.Marshal(sessionSyncProjection{LatestPersistedTurnIndex: latestPersistedTurnIndex})
	if err != nil {
		return "{}"
	}
	return string(raw)
}

func sessionSyncProjectionFromJSON(raw string) sessionSyncProjection {
	var sync sessionSyncProjection
	if err := json.Unmarshal([]byte(firstNonEmpty(strings.TrimSpace(raw), "{}")), &sync); err != nil {
		return sessionSyncProjection{}
	}
	return normalizeSessionSyncProjection(sync)
}

func normalizeSessionSyncProjection(sync sessionSyncProjection) sessionSyncProjection {
	if sync.LatestPersistedTurnIndex < 0 {
		sync.LatestPersistedTurnIndex = 0
	}
	if sync.LastDoneTurnIndex < 0 {
		sync.LastDoneTurnIndex = 0
	}
	if sync.LastReadTurnIndex < 0 {
		sync.LastReadTurnIndex = 0
	}
	return sync
}

func sessionSyncProjectionJSON(sync sessionSyncProjection) string {
	sync = normalizeSessionSyncProjection(sync)
	raw, err := json.Marshal(sync)
	if err != nil {
		return "{}"
	}
	return string(raw)
}

func sessionDoneTurnSuccess(turn sessionTurnMessage) bool {
	result, ok := turn.payload.(acp.IMPromptResult)
	if !ok {
		return true
	}
	return strings.TrimSpace(result.StopReason) != acp.StopReasonFailed
}

func boolPtr(value bool) *bool {
	return &value
}

func sortedSessionTurns(turns []sessionTurnMessage) []sessionTurnMessage {
	out := append([]sessionTurnMessage(nil), turns...)
	sort.Slice(out, func(i, j int) bool {
		return out[i].turnIndex < out[j].turnIndex
	})
	return out
}

func newSessionPromptState(nextTurnIndex int64) sessionPromptState {
	if nextTurnIndex <= 0 {
		nextTurnIndex = 1
	}
	return sessionPromptState{
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
	replaced := false
	for i := range s.turns {
		if s.turns[i].turnIndex == turn.turnIndex {
			s.turns[i] = turn
			replaced = true
			break
		}
	}
	if !replaced {
		s.turns = append(s.turns, turn)
	}
	s.nextTurnIndex = maxInt64(s.nextTurnIndex, turn.turnIndex+1)
	if turnKey != "" {
		s.turnIndexByKey[turnKey] = turn.turnIndex
	}
}

func (r *SessionRecorder) nextPromptStateLocked(ctx context.Context, sessionID string, updatedAt time.Time) (*sessionPromptState, error) {
	state, err := r.currentPromptStateLocked(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if state == nil {
		nextTurnIndex, err := r.nextSessionTurnIndexLocked(ctx, sessionID)
		if err != nil {
			return nil, err
		}
		created := newSessionPromptState(nextTurnIndex)
		return &created, nil
	}
	if len(state.turns) == 0 {
		nextTurnIndex, err := r.nextSessionTurnIndexLocked(ctx, sessionID)
		if err != nil {
			return nil, err
		}
		created := newSessionPromptState(nextTurnIndex)
		return &created, nil
	}
	if len(state.turns) > 0 && !sessionPromptStateTerminal(state) {
		if err := r.finishPromptStateLocked(ctx, sessionID, state, "interrupted", updatedAt, true); err != nil {
			return nil, err
		}
	}
	nextTurnIndex, err := r.nextSessionTurnIndexLocked(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	next := newSessionPromptState(maxInt64(state.nextTurnIndex, nextTurnIndex))
	return &next, nil
}

func sessionPromptStateTerminal(state *sessionPromptState) bool {
	if state == nil || len(state.turns) == 0 {
		return false
	}
	return strings.TrimSpace(state.turns[len(state.turns)-1].method) == acp.IMMethodPromptDone
}

func (r *SessionRecorder) ensurePromptStateLocked(ctx context.Context, sessionID string) (*sessionPromptState, error) {
	state, err := r.currentPromptStateLocked(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if state != nil {
		return state, nil
	}
	nextTurnIndex, err := r.nextSessionTurnIndexLocked(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	created := newSessionPromptState(nextTurnIndex)
	r.promptState[sessionID] = &created
	return &created, nil
}

func (r *SessionRecorder) nextSessionTurnIndexLocked(ctx context.Context, sessionID string) (int64, error) {
	next := r.nextTurnIndex[sessionID]
	if next <= 0 {
		rec, err := r.store.LoadSession(ctx, r.projectName, sessionID)
		if err != nil {
			return 0, err
		}
		if rec != nil {
			next = sessionSyncLatestPersistedTurnIndex(rec.SessionSyncJSON) + 1
		}
	}
	if next <= 0 {
		next = 1
	}
	return next, nil
}

func (r *SessionRecorder) currentPromptStateLocked(ctx context.Context, sessionID string) (*sessionPromptState, error) {
	return r.cachedPromptStateLocked(ctx, sessionID)
}

func (r *SessionRecorder) cachedPromptStateLocked(ctx context.Context, sessionID string) (*sessionPromptState, error) {
	state, ok := r.promptState[sessionID]
	if !ok || state == nil {
		return nil, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	state.ensureMaps()
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
