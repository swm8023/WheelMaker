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

type sessionPromptState struct {
	promptIndex   int64
	nextTurnIndex int64

	turns                map[int64]SessionTurnRecord
	toolTurnByToolCallID map[string]int64
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
	kind       sessionTurnMergeKind
	toolCallID string
}

type sessionTurnKey struct {
	ToolCallID string
}

type sessionViewTurnMessage struct {
	IMMessage acp.IMMessage
	MergeKey  sessionTurnKey
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
	parsed, err := parseSessionViewEvent(event)
	if err != nil {
		return err
	}
	if parsed.raw.SessionID == "" {
		return nil
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
	method := event.messageMethod()
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
		return r.appendACPEventMessageLocked(ctx, event.raw, event.turnMessage(), true)
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

func (e parsedSessionViewEvent) messageMethod() string {
	return strings.TrimSpace(e.method)
}

func (e parsedSessionViewEvent) imMessage() acp.IMMessage {
	message := acp.IMMessage{
		Method:  e.messageMethod(),
		Session: strings.TrimSpace(e.raw.SessionID),
	}
	if e.payload != nil {
		message.Param = mustJSONRaw(e.payload)
	}
	return message
}

func (e parsedSessionViewEvent) turnMessage() sessionViewTurnMessage {
	return sessionViewTurnMessage{
		IMMessage: e.imMessage(),
		MergeKey:  sessionTurnKey{ToolCallID: strings.TrimSpace(e.turnKey)},
	}
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
	message, err := buildPromptTurnRecord(rawEvent.SessionID, state.promptIndex, request)
	if err != nil {
		return err
	}
	if err := r.appendSessionTurnLocked(ctx, message); err != nil {
		return err
	}

	state.assignTurn(message, sessionTurnKey{})
	r.promptState[rawEvent.SessionID] = state
	if err := r.upsertSessionProjection(ctx, rawEvent.SessionID, promptTitle, rawEvent.UpdatedAt, true); err != nil {
		return err
	}
	return nil
}

func (r *SessionRecorder) appendACPEventMessageLocked(ctx context.Context, event SessionViewEvent, turnMessage sessionViewTurnMessage, hasTurnMessage bool) error {
	if !hasTurnMessage {
		return nil
	}
	state, ok, err := r.currentPromptStateLocked(ctx, event.SessionID)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	state, err = r.recordTurnMessageLocked(ctx, event.SessionID, state, turnMessage)
	if err != nil {
		return err
	}
	r.promptState[event.SessionID] = state
	return nil
}

func (r *SessionRecorder) recordTurnMessageLocked(ctx context.Context, sessionID string, state sessionPromptState, turnMessage sessionViewTurnMessage) (sessionPromptState, error) {
	state.ensureMaps()
	turnIndex, plan := getMergedTurnFromTurnMessage(state, turnMessage)
	if turnIndex > 0 {
		return r.mergeTurnMessageLocked(ctx, state, turnIndex, plan, turnMessage)
	}
	return r.appendTurnMessageLocked(ctx, sessionID, state, turnMessage)
}

func (r *SessionRecorder) mergeTurnMessageLocked(ctx context.Context, state sessionPromptState, turnIndex int64, plan sessionTurnMergePlan, turnMessage sessionViewTurnMessage) (sessionPromptState, error) {
	existingTurn, ok := state.turns[turnIndex]
	if !ok {
		return state, nil
	}
	indexedMessage := turnMessage.IMMessage
	indexedMessage.Index = formatPromptTurnSeq(state.promptIndex, turnIndex)
	indexedIncomingRaw, err := marshalIMMessage(indexedMessage)
	if err != nil {
		return state, err
	}
	mergedTurn, err := mergeTurnRecord(existingTurn, indexedMessage, plan)
	if err != nil {
		return state, err
	}
	if err := r.appendSessionTurnLocked(ctx, mergedTurn, indexedIncomingRaw); err != nil {
		return state, err
	}
	state.assignTurn(mergedTurn, sessionTurnKey{ToolCallID: strings.TrimSpace(plan.toolCallID)})
	return state, nil
}

func (r *SessionRecorder) appendTurnMessageLocked(ctx context.Context, sessionID string, state sessionPromptState, turnMessage sessionViewTurnMessage) (sessionPromptState, error) {
	indexed := turnMessage
	indexed.IMMessage.Index = formatPromptTurnSeq(state.promptIndex, state.nextTurnIndex)
	indexedIncomingRaw, err := marshalIMMessage(indexed.IMMessage)
	if err != nil {
		return state, err
	}
	message := buildSessionTurnRecord(sessionID, state.promptIndex, state.nextTurnIndex, indexedIncomingRaw)
	if err := r.appendSessionTurnLocked(ctx, message, indexedIncomingRaw); err != nil {
		return state, err
	}
	state.assignTurn(message, indexed.MergeKey)
	return state, nil
}

func (r *SessionRecorder) appendSessionTurnLocked(ctx context.Context, message SessionTurnRecord, publishContent ...string) error {
	if err := r.store.UpsertSessionTurn(ctx, message); err != nil {
		return err
	}
	content := message.UpdateJSON
	if len(publishContent) > 0 && strings.TrimSpace(publishContent[0]) != "" {
		content = publishContent[0]
	}
	r.publishSessionTurn(message.SessionID, message, content)
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

func (r *SessionRecorder) publishSessionTurn(sessionID string, turn SessionTurnRecord, rawContent string) {
	publish := r.eventPublisher()
	if publish == nil {
		return
	}
	sessionID = strings.TrimSpace(sessionID)
	content := strings.TrimSpace(rawContent)
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
		promptIndex:          promptIndex,
		nextTurnIndex:        nextTurnIndex,
		turns:                map[int64]SessionTurnRecord{},
		toolTurnByToolCallID: map[string]int64{},
	}
}

func (s *sessionPromptState) ensureMaps() {
	if s.turns == nil {
		s.turns = map[int64]SessionTurnRecord{}
	}
	if s.toolTurnByToolCallID == nil {
		s.toolTurnByToolCallID = map[string]int64{}
	}
	if s.nextTurnIndex <= 0 {
		s.nextTurnIndex = 1
	}
}

func (s *sessionPromptState) assignTurn(turn SessionTurnRecord, key sessionTurnKey) {
	s.ensureMaps()
	turn.UpdateJSON = normalizeJSONDoc(turn.UpdateJSON, `{}`)
	s.turns[turn.TurnIndex] = turn
	if turn.TurnIndex >= s.nextTurnIndex {
		s.nextTurnIndex = turn.TurnIndex + 1
	}
	if toolCallID := strings.TrimSpace(key.ToolCallID); toolCallID != "" {
		s.toolTurnByToolCallID[toolCallID] = turn.TurnIndex
	}
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
		state.assignTurn(messages[i], sessionTurnKey{})
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
			update := params.Update
			updateMethod := strings.TrimSpace(update.SessionUpdate)
			switch updateMethod {
			case acp.SessionUpdateAgentMessageChunk, acp.SessionUpdateAgentThoughtChunk, acp.SessionUpdateUserMessageChunk:
				parsed.setJSONMessage(updateMethod, acp.IMTextResult{Text: extractUpdateText(update.Content)}, "")
			case acp.SessionUpdateToolCall, acp.SessionUpdateToolCallUpdate:
				output := extractUpdateText(update.Content)
				if strings.TrimSpace(output) == "" {
					output = stringifyRawJSON(update.RawOutput)
				}
				parsed.setJSONMessage(acp.IMMethodToolCall, acp.IMToolResult{
					Cmd:    strings.TrimSpace(update.Title),
					Kind:   strings.TrimSpace(update.Kind),
					Status: strings.TrimSpace(update.Status),
					Output: output,
				}, update.ToolCallID)
			case acp.SessionUpdatePlan:
				entries := make([]acp.IMPlanResult, 0, len(update.Entries))
				for _, entry := range update.Entries {
					entries = append(entries, acp.IMPlanResult{Content: strings.TrimSpace(entry.Content), Status: strings.TrimSpace(entry.Status)})
				}
				parsed.setJSONMessage(acp.IMMethodAgentPlan, entries, "")
			default:
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

func getMergedTurnFromTurnMessage(state sessionPromptState, turn sessionViewTurnMessage) (int64, sessionTurnMergePlan) {
	plan := sessionTurnMergePlan{kind: sessionTurnMergeNone}
	method := strings.TrimSpace(turn.IMMessage.Method)
	switch method {
	case acp.IMMethodToolCall:
		if strings.TrimSpace(turn.MergeKey.ToolCallID) == "" {
			return 0, plan
		}
		plan.kind = sessionTurnMergeTool
		plan.toolCallID = strings.TrimSpace(turn.MergeKey.ToolCallID)
		if turnIndex := state.toolTurnByToolCallID[plan.toolCallID]; turnIndex > 0 {
			return turnIndex, plan
		}
	case acp.SessionUpdateAgentMessageChunk, acp.SessionUpdateAgentThoughtChunk, acp.SessionUpdateUserMessageChunk:
		plan.kind = sessionTurnMergeText
		lastTurnIndex := state.nextTurnIndex - 1
		if lastTurnIndex > 0 {
			if message, ok := state.turns[lastTurnIndex]; ok && strings.TrimSpace(sessionTurnMethodKey(message.UpdateJSON)) == method {
				return lastTurnIndex, plan
			}
		}
	}
	return 0, plan
}

func mergeTurnRecord(existing SessionTurnRecord, incomingMessage acp.IMMessage, plan sessionTurnMergePlan) (SessionTurnRecord, error) {
	merged := existing
	merged.UpdateIndex = maxInt64(existing.UpdateIndex, 0) + 1
	merged.UpdateJSON = normalizeJSONDoc(existing.UpdateJSON, `{}`)

	existingMessage, err := decodeIMMessage(merged.UpdateJSON)
	if err != nil {
		return SessionTurnRecord{}, err
	}

	var mergedMessage acp.IMMessage
	switch plan.kind {
	case sessionTurnMergeTool:
		mergedMessage, err = mergeToolResultMessage(existingMessage, incomingMessage)
	case sessionTurnMergeText:
		mergedMessage, err = mergeTextResultMessage(existingMessage, incomingMessage)
	default:
		mergedMessage = incomingMessage
	}
	if err != nil {
		return SessionTurnRecord{}, err
	}
	if strings.TrimSpace(mergedMessage.Index) == "" {
		mergedMessage.Index = strings.TrimSpace(existingMessage.Index)
	}
	raw, err := json.Marshal(mergedMessage)
	if err != nil {
		return SessionTurnRecord{}, err
	}
	merged.UpdateJSON = normalizeJSONDoc(string(raw), merged.UpdateJSON)
	return merged, nil
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

func buildTurnMessageFromSessionUpdate(update acp.SessionUpdate) (sessionViewTurnMessage, bool, error) {
	method := strings.TrimSpace(update.SessionUpdate)
	switch method {
	case acp.SessionUpdateAgentMessageChunk, acp.SessionUpdateAgentThoughtChunk, acp.SessionUpdateUserMessageChunk:
		message := acp.IMMessage{Method: method}
		resultRaw, err := json.Marshal(acp.IMTextResult{Text: extractUpdateText(update.Content)})
		if err != nil {
			return sessionViewTurnMessage{}, false, err
		}
		message.Param = cloneJSONRaw(resultRaw)
		return sessionViewTurnMessage{IMMessage: message}, true, nil
	case acp.SessionUpdateToolCall, acp.SessionUpdateToolCallUpdate:
		output := extractUpdateText(update.Content)
		if strings.TrimSpace(output) == "" {
			output = stringifyRawJSON(update.RawOutput)
		}
		message := acp.IMMessage{Method: acp.IMMethodToolCall}
		resultRaw, err := json.Marshal(acp.IMToolResult{
			Cmd:    strings.TrimSpace(update.Title),
			Kind:   strings.TrimSpace(update.Kind),
			Status: strings.TrimSpace(update.Status),
			Output: output,
		})
		if err != nil {
			return sessionViewTurnMessage{}, false, err
		}
		message.Param = cloneJSONRaw(resultRaw)
		return sessionViewTurnMessage{
			IMMessage: message,
			MergeKey:  sessionTurnKey{ToolCallID: strings.TrimSpace(update.ToolCallID)},
		}, true, nil
	case acp.SessionUpdatePlan:
		entries := make([]acp.IMPlanResult, 0, len(update.Entries))
		for _, entry := range update.Entries {
			entries = append(entries, acp.IMPlanResult{Content: strings.TrimSpace(entry.Content), Status: strings.TrimSpace(entry.Status)})
		}
		message := acp.IMMessage{Method: acp.IMMethodAgentPlan}
		resultRaw, err := json.Marshal(entries)
		if err != nil {
			return sessionViewTurnMessage{}, false, err
		}
		message.Param = cloneJSONRaw(resultRaw)
		return sessionViewTurnMessage{IMMessage: message}, true, nil
	default:
		return sessionViewTurnMessage{}, false, nil
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

func sessionTurnMethodKey(raw string) string {
	message, err := decodeIMMessage(raw)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(message.Method)
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

func buildSessionTurnRecord(sessionID string, promptIndex, turnIndex int64, rawContent string) SessionTurnRecord {
	if turnIndex <= 0 {
		turnIndex = 1
	}
	return SessionTurnRecord{
		SessionID:   strings.TrimSpace(sessionID),
		PromptIndex: promptIndex,
		TurnIndex:   turnIndex,
		UpdateIndex: 1,
		UpdateJSON:  normalizeJSONDoc(rawContent, `{}`),
	}
}

func buildPromptTurnRecord(sessionID string, promptIndex int64, request acp.IMPromptRequest) (SessionTurnRecord, error) {
	requestRaw, err := json.Marshal(acp.IMPromptRequest{ContentBlocks: cloneSessionContentBlocks(request.ContentBlocks)})
	if err != nil {
		return SessionTurnRecord{}, err
	}
	messageRaw, err := marshalIMMessage(acp.IMMessage{Method: acp.IMMethodPromptRequest, Session: strings.TrimSpace(sessionID), Param: cloneJSONRaw(requestRaw)})
	if err != nil {
		return SessionTurnRecord{}, err
	}
	indexedRaw, err := withIMTurnIndex(messageRaw, promptIndex, 1)
	if err != nil {
		return SessionTurnRecord{}, err
	}
	return buildSessionTurnRecord(sessionID, promptIndex, 1, indexedRaw), nil
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
