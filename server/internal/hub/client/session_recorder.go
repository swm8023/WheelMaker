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
	Agent     string `json:"agent,omitempty"`
}

type sessionViewMessage struct {
	SessionID   string `json:"sessionId"`
	PromptIndex int64  `json:"promptIndex"`
	TurnIndex   int64  `json:"turnIndex"`
	UpdateIndex int64  `json:"updateIndex"`
	Content     string `json:"content"`
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
	UpdateIndex int64
}

type sessionPromptState struct {
	promptIndex   int64
	nextTurnIndex int64

	turns          map[int64]sessionTurnMessage
	turnIndexByKey map[string]int64
}

type sessionViewSessionNewParams struct {
	SessionID string `json:"sessionId,omitempty"`
	Title     string `json:"title,omitempty"`
}

type sessionViewPromptResult struct {
	StopReason string `json:"stopReason,omitempty"`
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

type sessionTurnMergePlan struct {
	kind    sessionTurnMergeKind
	turnKey string
}

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
		jsonDecodeAt(json.RawMessage(parsed.raw.Content), "params.title", &title)
		title = strings.TrimSpace(title)
		return r.upsertSessionProjection(ctx, parsed.raw.SessionID, title, parsed.raw.UpdatedAt, false)
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
		return r.appendACPEventMessageLocked(ctx, event)
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
		Method:  e.method,
		Session: strings.TrimSpace(e.raw.SessionID),
	}
	if e.payload != nil {
		message.Param = mustJSONRaw(e.payload)
	}
	return message
}

func (m sessionTurnMessage) imMessage() acp.IMMessage {
	message := acp.IMMessage{
		Method:  m.method,
		Session: strings.TrimSpace(m.SessionID),
	}
	if m.payload != nil {
		switch value := m.payload.(type) {
		case json.RawMessage:
			message.Param = cloneJSONRaw(value)
		default:
			message.Param = mustJSONRaw(value)
		}
	}
	if m.PromptIndex > 0 && m.TurnIndex > 0 {
		message.Index = formatPromptTurnSeq(m.PromptIndex, m.TurnIndex)
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
	if err := r.upsertSessionProjection(ctx, rawEvent.SessionID, promptTitle, rawEvent.UpdatedAt, true); err != nil {
		return err
	}
	return nil
}

func (r *SessionRecorder) appendACPEventMessageLocked(ctx context.Context, event parsedSessionViewEvent) error {
	state, ok, err := r.currentPromptStateLocked(ctx, event.raw.SessionID)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	if err := r.addMessageTurn(ctx, &state, event); err != nil {
		return err
	}
	r.promptState[event.raw.SessionID] = state
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
	publishContent := ""

	mergeKind, mergedTurnIndex := getTurnKindAndIndex(*state, event)
	if mergedTurnIndex > 0 {
		existingTurn, ok := state.turns[mergedTurnIndex]

		if ok {
			turn.SessionID = strings.TrimSpace(firstNonEmpty(strings.TrimSpace(turn.SessionID), strings.TrimSpace(existingTurn.SessionID)))
			turn.PromptIndex = state.promptIndex
			turn.TurnIndex = mergedTurnIndex
			turn.UpdateIndex = maxInt64(existingTurn.UpdateIndex, 0) + 1

			incomingRaw, err := json.Marshal(turn.imMessage())
			if err != nil {
				return err
			}
			publishContent = normalizeJSONDoc(string(incomingRaw), `{}`)

			merged, err := mergeTurnMessage(existingTurn, turn, mergeKind, mergedTurnIndex)
			if err != nil {
				return err
			}
			turn = merged
		}
	}

	turn.PromptIndex = state.promptIndex
	if turn.TurnIndex <= 0 {
		turn.TurnIndex = state.nextTurnIndex
	}
	if turn.UpdateIndex <= 0 {
		turn.UpdateIndex = 1
	}

	messageRaw, err := json.Marshal(turn.imMessage())
	if err != nil {
		return err
	}
	updateJSON := normalizeJSONDoc(string(messageRaw), `{}`)
	record := SessionTurnRecord{
		SessionID:   strings.TrimSpace(turn.SessionID),
		PromptIndex: turn.PromptIndex,
		TurnIndex:   maxInt64(turn.TurnIndex, 1),
		UpdateIndex: maxInt64(turn.UpdateIndex, 1),
		UpdateJSON:  updateJSON,
	}
	if err := r.store.UpsertSessionTurn(ctx, record); err != nil {
		return err
	}
	if strings.TrimSpace(publishContent) == "" {
		publishContent = updateJSON
	}
	publish := r.eventPublisher()
	if publish != nil {
		_ = publish("registry.session.message", map[string]any{
			"sessionId":   strings.TrimSpace(turn.SessionID),
			"promptIndex": turn.PromptIndex,
			"turnIndex":   turn.TurnIndex,
			"updateIndex": turn.UpdateIndex,
			"content":     publishContent,
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

func (r *SessionRecorder) upsertSessionProjection(ctx context.Context, sessionID, title string, updatedAt time.Time, titleIfEmptyOnly bool) error {
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
		rec.Agent,
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
	turn.UpdateIndex = maxInt64(turn.UpdateIndex, 1)
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
	state, ok, err := r.currentPromptStateLocked(ctx, sessionID)
	if err != nil {
		return sessionPromptState{}, err
	}
	if !ok {
		return newSessionPromptState(1, 1), nil
	}
	return newSessionPromptState(state.promptIndex+1, 1), nil
}

func (r *SessionRecorder) ensurePromptStateLocked(ctx context.Context, sessionID string, updatedAt time.Time) (sessionPromptState, error) {
	sessionID, err := normalizeSessionID(sessionID)
	if err != nil {
		return sessionPromptState{}, err
	}
	state, ok, err := r.currentPromptStateLocked(ctx, sessionID)
	if err != nil {
		return sessionPromptState{}, err
	}
	if ok {
		return state, nil
	}
	state = newSessionPromptState(1, 1)
	if err := r.store.UpsertSessionPrompt(ctx, SessionPromptRecord{SessionID: sessionID, PromptIndex: 1, UpdatedAt: updatedAt}); err != nil {
		return sessionPromptState{}, err
	}
	r.promptState[sessionID] = state
	return state, nil
}

func (r *SessionRecorder) currentPromptStateLocked(ctx context.Context, sessionID string) (sessionPromptState, bool, error) {
	sessionID, err := normalizeSessionID(sessionID)
	if err != nil {
		return sessionPromptState{}, false, err
	}
	if state, ok, err := r.cachedPromptStateLocked(ctx, sessionID); ok || err != nil {
		return state, ok, err
	}
	return r.loadLatestPromptStateLocked(ctx, sessionID)
}

func (r *SessionRecorder) cachedPromptStateLocked(ctx context.Context, sessionID string) (sessionPromptState, bool, error) {
	state, ok := r.promptState[sessionID]
	if !ok || state.promptIndex <= 0 {
		return sessionPromptState{}, false, nil
	}
	prompt, err := r.store.LoadSessionPrompt(ctx, r.projectName, sessionID, state.promptIndex)
	if err != nil {
		return sessionPromptState{}, false, err
	}
	if prompt == nil {
		delete(r.promptState, sessionID)
		return sessionPromptState{}, false, nil
	}
	state.ensureMaps()
	return state, true, nil
}

func (r *SessionRecorder) loadLatestPromptStateLocked(ctx context.Context, sessionID string) (sessionPromptState, bool, error) {
	prompts, err := r.store.ListSessionPrompts(ctx, r.projectName, sessionID)
	if err != nil {
		return sessionPromptState{}, false, err
	}
	if len(prompts) == 0 {
		return sessionPromptState{}, false, nil
	}
	latest := prompts[len(prompts)-1]
	state, err := r.restorePromptStateLocked(ctx, sessionID, latest.PromptIndex)
	if err != nil {
		return sessionPromptState{}, false, err
	}
	r.promptState[sessionID] = state
	return state, true, nil
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

func jsonGet(raw json.RawMessage, path string) (json.RawMessage, bool) {
	current := json.RawMessage(bytes.TrimSpace(raw))
	if len(current) == 0 {
		return nil, false
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return current, true
	}
	for _, key := range strings.Split(path, ".") {
		key = strings.TrimSpace(key)
		if key == "" {
			return nil, false
		}
		obj := map[string]json.RawMessage{}
		if err := json.Unmarshal(current, &obj); err != nil {
			return nil, false
		}
		next, ok := obj[key]
		if !ok {
			return nil, false
		}
		next = json.RawMessage(bytes.TrimSpace(next))
		if len(next) == 0 {
			return nil, false
		}
		current = next
	}
	return current, true
}

func jsonDecodeAt(raw json.RawMessage, path string, out any) bool {
	if out == nil {
		return false
	}
	value, ok := jsonGet(raw, path)
	if !ok {
		return false
	}
	if err := json.Unmarshal(value, out); err != nil {
		return false
	}
	return true
}

func extractUpdateText(raw json.RawMessage) string {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return ""
	}
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
			promptResult := sessionViewPromptResult{}
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
	existingMessage := existing.imMessage()
	incomingMessage := incoming.imMessage()

	var mergedMessage acp.IMMessage
	var err error
	switch mergeKind {
	case sessionTurnMergeTool:
		mergedMessage, err = mergeToolResultMessage(existingMessage, incomingMessage)
	case sessionTurnMergeText:
		mergedMessage, err = mergeTextResultMessage(existingMessage, incomingMessage)
	default:
		mergedMessage = incomingMessage
	}
	if err != nil {
		return sessionTurnMessage{}, err
	}
	payload, err := decodeTurnPayload(mergedMessage)
	if err != nil {
		return sessionTurnMessage{}, err
	}
	return sessionTurnMessage{
		SessionID:   strings.TrimSpace(firstNonEmpty(strings.TrimSpace(mergedMessage.Session), strings.TrimSpace(existing.SessionID), strings.TrimSpace(incoming.SessionID))),
		method:      strings.TrimSpace(mergedMessage.Method),
		payload:     payload,
		PromptIndex: existing.PromptIndex,
		TurnIndex:   maxInt64(turnIndex, existing.TurnIndex),
		UpdateIndex: maxInt64(existing.UpdateIndex, 0) + 1,
	}, nil
}

func mergeTextResultMessage(existing, incoming acp.IMMessage) (acp.IMMessage, error) {
	base := acp.IMTextResult{}
	if len(existing.Param) > 0 {
		if err := json.Unmarshal(existing.Param, &base); err != nil {
			return acp.IMMessage{}, err
		}
	}
	inc := acp.IMTextResult{}
	if len(incoming.Param) > 0 {
		if err := json.Unmarshal(incoming.Param, &inc); err != nil {
			return acp.IMMessage{}, err
		}
	}
	inc.Text = base.Text + inc.Text
	if strings.TrimSpace(inc.Text) == "" {
		inc.Text = base.Text
	}
	raw, err := json.Marshal(inc)
	if err != nil {
		return acp.IMMessage{}, err
	}
	incoming.Param = cloneJSONRaw(raw)
	if strings.TrimSpace(incoming.Session) == "" {
		incoming.Session = strings.TrimSpace(existing.Session)
	}
	return incoming, nil
}

func mergeToolResultMessage(existing, incoming acp.IMMessage) (acp.IMMessage, error) {
	base := acp.IMToolResult{}
	if len(existing.Param) > 0 {
		if err := json.Unmarshal(existing.Param, &base); err != nil {
			return acp.IMMessage{}, err
		}
	}
	inc := acp.IMToolResult{}
	if len(incoming.Param) > 0 {
		if err := json.Unmarshal(incoming.Param, &inc); err != nil {
			return acp.IMMessage{}, err
		}
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
	raw, err := json.Marshal(inc)
	if err != nil {
		return acp.IMMessage{}, err
	}
	incoming.Param = cloneJSONRaw(raw)
	if strings.TrimSpace(incoming.Session) == "" {
		incoming.Session = strings.TrimSpace(existing.Session)
	}
	return incoming, nil
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

func marshalIMMessage(message acp.IMMessage) (string, error) {
	raw, err := json.Marshal(message)
	if err != nil {
		return "", err
	}
	return normalizeJSONDoc(string(raw), `{}`), nil
}

func withIMTurnIndex(messageRaw string, promptIndex, turnIndex int64) (string, error) {
	message, err := decodeIMMessage(messageRaw)
	if err != nil {
		return "", err
	}
	message.Index = formatPromptTurnSeq(promptIndex, turnIndex)
	raw, err := json.Marshal(message)
	if err != nil {
		return "", err
	}
	return normalizeJSONDoc(string(raw), messageRaw), nil
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
		SessionID:   strings.TrimSpace(firstNonEmpty(strings.TrimSpace(imMessage.Session), strings.TrimSpace(record.SessionID))),
		method:      strings.TrimSpace(imMessage.Method),
		payload:     payload,
		PromptIndex: record.PromptIndex,
		TurnIndex:   record.TurnIndex,
		UpdateIndex: maxInt64(record.UpdateIndex, 1),
	}, nil
}

func buildPromptTurnMessage(sessionID string, request acp.IMPromptRequest) sessionTurnMessage {
	return sessionTurnMessage{
		SessionID: strings.TrimSpace(sessionID),
		method:    acp.IMMethodPromptRequest,
		payload: acp.IMPromptRequest{
			ContentBlocks: cloneSessionContentBlocks(request.ContentBlocks),
		},
	}
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
		UpdateIndex: message.UpdateIndex,
		Content:     content,
	}
}
