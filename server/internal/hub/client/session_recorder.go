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

	turns                     map[int64]SessionTurnRecord
	toolTurnByToolCallID      map[string]int64
	permissionTurnByRequestID map[int64]int64
}

func newSessionPromptState(promptIndex, nextTurnIndex int64) sessionPromptState {
	if nextTurnIndex <= 0 {
		nextTurnIndex = 1
	}
	return sessionPromptState{
		promptIndex:               promptIndex,
		nextTurnIndex:             nextTurnIndex,
		turns:                     map[int64]SessionTurnRecord{},
		toolTurnByToolCallID:      map[string]int64{},
		permissionTurnByRequestID: map[int64]int64{},
	}
}

func (s *sessionPromptState) ensureMaps() {
	if s.turns == nil {
		s.turns = map[int64]SessionTurnRecord{}
	}
	if s.toolTurnByToolCallID == nil {
		s.toolTurnByToolCallID = map[string]int64{}
	}
	if s.permissionTurnByRequestID == nil {
		s.permissionTurnByRequestID = map[int64]int64{}
	}
	if s.nextTurnIndex <= 0 {
		s.nextTurnIndex = 1
	}
}

func (s *sessionPromptState) assignTurn(turn SessionTurnRecord) {
	s.ensureMaps()
	turn.UpdateJSON = normalizeJSONDoc(turn.UpdateJSON, `{}`)
	turn.ExtraJSON = normalizeJSONDoc(turn.ExtraJSON, `{}`)
	s.turns[turn.TurnIndex] = turn
	if turn.TurnIndex >= s.nextTurnIndex {
		s.nextTurnIndex = turn.TurnIndex + 1
	}
	if key := sessionTurnToolCallIDKey(turn.UpdateJSON); key != "" {
		s.toolTurnByToolCallID[key] = turn.TurnIndex
	}
	if requestID := sessionTurnPermissionRequestIDKey(turn.UpdateJSON); requestID > 0 {
		s.permissionTurnByRequestID[requestID] = turn.TurnIndex
	}
}

type sessionTurnMergeKind string

const (
	sessionTurnMergeNone       sessionTurnMergeKind = ""
	sessionTurnMergeTool       sessionTurnMergeKind = "tool"
	sessionTurnMergePermission sessionTurnMergeKind = "permission"
)

type sessionTurnMergePlan struct {
	kind                sessionTurnMergeKind
	toolCallID          string
	requestID           int64
	hasPermissionResult bool
}

func isToolSessionUpdateType(updateMethod string) bool {
	switch strings.TrimSpace(updateMethod) {
	case acp.SessionUpdateToolCall, acp.SessionUpdateToolCallUpdate:
		return true
	default:
		return false
	}
}

func getMergedTurn(state sessionPromptState, doc sessionViewACPContentDoc) (int64, sessionTurnMergePlan, error) {
	plan := sessionTurnMergePlan{kind: sessionTurnMergeNone}
	switch strings.TrimSpace(doc.Method) {
	case acp.MethodSessionUpdate:
		var params acp.SessionUpdateParams
		if err := decodeSessionViewEventParams(doc, &params); err != nil {
			if errors.Is(err, errSessionEventPayloadEmpty) {
				return 0, plan, nil
			}
			return 0, plan, err
		}
		updateMethod := strings.TrimSpace(params.Update.SessionUpdate)
		switch {
		case isToolSessionUpdateType(updateMethod) && strings.TrimSpace(params.Update.ToolCallID) != "":
			plan.kind = sessionTurnMergeTool
			plan.toolCallID = strings.TrimSpace(params.Update.ToolCallID)
			if turnIndex := state.toolTurnByToolCallID[plan.toolCallID]; turnIndex > 0 {
				return turnIndex, plan, nil
			}
		case updateMethod != "":
			lastTurnIndex := state.nextTurnIndex - 1
			if lastTurnIndex > 0 {
				if turn, ok := state.turns[lastTurnIndex]; ok && strings.TrimSpace(sessionTurnUpdateType(turn.UpdateJSON)) == updateMethod {
					return lastTurnIndex, plan, nil
				}
			}
		}
	case acp.MethodRequestPermission:
		if doc.ID <= 0 {
			return 0, plan, nil
		}
		plan.kind = sessionTurnMergePermission
		plan.requestID = doc.ID
		plan.hasPermissionResult = len(doc.Result) > 0 && strings.TrimSpace(string(doc.Result)) != ""
		if turnIndex := state.permissionTurnByRequestID[plan.requestID]; turnIndex > 0 {
			return turnIndex, plan, nil
		}
	}
	return 0, plan, nil
}

func mergeTurnRecord(existing SessionTurnRecord, incomingRaw string, plan sessionTurnMergePlan) (SessionTurnRecord, error) {
	merged := existing
	merged.UpdateIndex = maxInt64(existing.UpdateIndex, 0) + 1
	merged.UpdateJSON = normalizeJSONDoc(existing.UpdateJSON, `{}`)
	merged.ExtraJSON = normalizeJSONDoc(existing.ExtraJSON, `{}`)

	var err error
	switch plan.kind {
	case sessionTurnMergeTool:
		merged.UpdateJSON, err = mergeSessionUpdateToolJSON(existing.UpdateJSON, incomingRaw)
		if err != nil {
			return SessionTurnRecord{}, err
		}
	case sessionTurnMergePermission:
		merged.UpdateJSON, merged.ExtraJSON, err = mergeSessionPermissionJSON(existing.UpdateJSON, existing.ExtraJSON, incomingRaw)
		if err != nil {
			return SessionTurnRecord{}, err
		}
	default:
		merged.UpdateJSON, err = mergeSessionUpdateDefaultJSON(existing.UpdateJSON, incomingRaw)
		if err != nil {
			return SessionTurnRecord{}, err
		}
	}
	return merged, nil
}

func mergeSessionUpdateToolJSON(existingRaw, incomingRaw string) (string, error) {
	return mergeSessionUpdateDoc(existingRaw, incomingRaw, func(base, incoming acp.SessionUpdate) acp.SessionUpdate {
		return mergeSessionUpdateFields(base, incoming)
	})
}

func mergeSessionUpdateDefaultJSON(existingRaw, incomingRaw string) (string, error) {
	return mergeSessionUpdateDoc(existingRaw, incomingRaw, func(base, incoming acp.SessionUpdate) acp.SessionUpdate {
		merged := mergeSessionUpdateFields(base, incoming)
		merged.Content = mergeSessionUpdateTextContent(base.Content, incoming.Content)
		return merged
	})
}

func mergeSessionUpdateDoc(existingRaw, incomingRaw string, mergeUpdate func(base, incoming acp.SessionUpdate) acp.SessionUpdate) (string, error) {
	type sessionUpdateEnvelope struct {
		Method string `json:"method"`
		Params struct {
			SessionID string            `json:"sessionId,omitempty"`
			Update    acp.SessionUpdate `json:"update"`
		} `json:"params"`
	}

	existingRaw = normalizeJSONDoc(existingRaw, `{}`)
	incomingRaw = normalizeJSONDoc(incomingRaw, existingRaw)

	var existingDoc sessionUpdateEnvelope
	if err := json.Unmarshal([]byte(existingRaw), &existingDoc); err != nil {
		return incomingRaw, nil
	}
	var incomingDoc sessionUpdateEnvelope
	if err := json.Unmarshal([]byte(incomingRaw), &incomingDoc); err != nil {
		return existingRaw, nil
	}
	if strings.TrimSpace(existingDoc.Method) != acp.MethodSessionUpdate || strings.TrimSpace(incomingDoc.Method) != acp.MethodSessionUpdate {
		return incomingRaw, nil
	}
	existingDoc.Params.Update = mergeUpdate(existingDoc.Params.Update, incomingDoc.Params.Update)
	if strings.TrimSpace(incomingDoc.Params.SessionID) != "" {
		existingDoc.Params.SessionID = strings.TrimSpace(incomingDoc.Params.SessionID)
	}
	raw, err := json.Marshal(existingDoc)
	if err != nil {
		return "", err
	}
	return normalizeJSONDoc(string(raw), incomingRaw), nil
}

func mergeSessionUpdateTextContent(baseRaw, incomingRaw json.RawMessage) json.RawMessage {
	if len(incomingRaw) == 0 {
		return cloneJSONRaw(baseRaw)
	}
	if len(baseRaw) == 0 {
		return cloneJSONRaw(incomingRaw)
	}

	var base map[string]any
	if err := json.Unmarshal(baseRaw, &base); err != nil {
		return cloneJSONRaw(incomingRaw)
	}
	var incoming map[string]any
	if err := json.Unmarshal(incomingRaw, &incoming); err != nil {
		return cloneJSONRaw(incomingRaw)
	}

	baseText, baseOK := base["text"].(string)
	incomingText, incomingOK := incoming["text"].(string)
	if !baseOK || !incomingOK {
		return cloneJSONRaw(incomingRaw)
	}

	incoming["text"] = baseText + incomingText
	raw, err := json.Marshal(incoming)
	if err != nil {
		return cloneJSONRaw(incomingRaw)
	}
	return cloneJSONRaw(raw)
}

func mergeSessionPermissionJSON(existingUpdateJSON, existingExtraJSON, incomingRaw string) (string, string, error) {
	incomingRaw = normalizeJSONDoc(incomingRaw, `{}`)
	incomingDoc, err := decodeSessionViewACPContentDoc(incomingRaw)
	if err != nil {
		return normalizeJSONDoc(existingUpdateJSON, incomingRaw), normalizeJSONDoc(existingExtraJSON, `{}`), nil
	}
	if strings.TrimSpace(incomingDoc.Method) != acp.MethodRequestPermission {
		return normalizeJSONDoc(incomingRaw, normalizeJSONDoc(existingUpdateJSON, `{}`)), normalizeJSONDoc(existingExtraJSON, `{}`), nil
	}

	updateJSON := normalizeJSONDoc(existingUpdateJSON, `{}`)
	extraJSON := normalizeJSONDoc(existingExtraJSON, `{}`)

	hasParams := len(incomingDoc.Params) > 0 && strings.TrimSpace(string(incomingDoc.Params)) != ""
	hasResult := len(incomingDoc.Result) > 0 && strings.TrimSpace(string(incomingDoc.Result)) != ""

	if hasParams || strings.TrimSpace(updateJSON) == "" || strings.TrimSpace(updateJSON) == "{}" {
		updateJSON = normalizeJSONDoc(incomingRaw, updateJSON)
	}
	if !hasResult {
		return updateJSON, extraJSON, nil
	}

	extraDoc := map[string]any{}
	_ = json.Unmarshal([]byte(extraJSON), &extraDoc)
	permissionResult := map[string]any{
		"id":     incomingDoc.ID,
		"method": strings.TrimSpace(incomingDoc.Method),
	}
	var result any
	if err := json.Unmarshal(incomingDoc.Result, &result); err == nil {
		permissionResult["result"] = result
	}
	extraDoc["acpUserResult"] = permissionResult

	raw, err := json.Marshal(extraDoc)
	if err != nil {
		return updateJSON, extraJSON, nil
	}
	return updateJSON, normalizeJSONDoc(string(raw), `{}`), nil
}

func mergeSessionUpdateFields(base, incoming acp.SessionUpdate) acp.SessionUpdate {
	merged := base
	if strings.TrimSpace(incoming.SessionUpdate) != "" {
		merged.SessionUpdate = strings.TrimSpace(incoming.SessionUpdate)
	}
	if len(incoming.Content) > 0 {
		merged.Content = cloneJSONRaw(incoming.Content)
	}
	if len(incoming.AvailableCommands) > 0 {
		merged.AvailableCommands = append([]acp.AvailableCommand(nil), incoming.AvailableCommands...)
	}
	if strings.TrimSpace(incoming.ToolCallID) != "" {
		merged.ToolCallID = strings.TrimSpace(incoming.ToolCallID)
	}
	if strings.TrimSpace(incoming.Title) != "" {
		merged.Title = strings.TrimSpace(incoming.Title)
	}
	if strings.TrimSpace(incoming.Kind) != "" {
		merged.Kind = strings.TrimSpace(incoming.Kind)
	}
	if strings.TrimSpace(incoming.Status) != "" {
		merged.Status = strings.TrimSpace(incoming.Status)
	}
	if len(incoming.Entries) > 0 {
		merged.Entries = append([]acp.PlanEntry(nil), incoming.Entries...)
	}
	if len(incoming.Locations) > 0 {
		merged.Locations = append([]acp.ToolCallLocation(nil), incoming.Locations...)
	}
	if len(incoming.RawInput) > 0 {
		merged.RawInput = cloneJSONRaw(incoming.RawInput)
	}
	if len(incoming.RawOutput) > 0 {
		merged.RawOutput = cloneJSONRaw(incoming.RawOutput)
	}
	if len(incoming.ToolCallContent) > 0 {
		merged.ToolCallContent = append([]acp.ToolCallContent(nil), incoming.ToolCallContent...)
	}
	if strings.TrimSpace(incoming.ModeID) != "" {
		merged.ModeID = strings.TrimSpace(incoming.ModeID)
	}
	if len(incoming.ConfigOptions) > 0 {
		merged.ConfigOptions = append([]acp.ConfigOption(nil), incoming.ConfigOptions...)
	}
	if incoming.Size != nil {
		merged.Size = cloneInt64Ptr(incoming.Size)
	}
	if incoming.Used != nil {
		merged.Used = cloneInt64Ptr(incoming.Used)
	}
	if strings.TrimSpace(incoming.UpdatedAt) != "" {
		merged.UpdatedAt = strings.TrimSpace(incoming.UpdatedAt)
	}
	return merged
}

func sessionTurnToolCallIDKey(raw string) string {
	type sessionUpdateEnvelope struct {
		Method string `json:"method"`
		Params struct {
			Update acp.SessionUpdate `json:"update"`
		} `json:"params"`
	}
	var doc sessionUpdateEnvelope
	if err := json.Unmarshal([]byte(normalizeJSONDoc(raw, `{}`)), &doc); err != nil {
		return ""
	}
	if strings.TrimSpace(doc.Method) != acp.MethodSessionUpdate {
		return ""
	}
	if !isToolSessionUpdateType(doc.Params.Update.SessionUpdate) {
		return ""
	}
	return strings.TrimSpace(doc.Params.Update.ToolCallID)
}

func sessionTurnPermissionRequestIDKey(raw string) int64 {
	var doc sessionViewACPContentDoc
	if err := json.Unmarshal([]byte(normalizeJSONDoc(raw, `{}`)), &doc); err != nil {
		return 0
	}
	if strings.TrimSpace(doc.Method) != acp.MethodRequestPermission {
		return 0
	}
	if doc.ID <= 0 {
		return 0
	}
	return doc.ID
}

func sessionTurnUpdateType(raw string) string {
	type sessionUpdateEnvelope struct {
		Method string `json:"method"`
		Params struct {
			Update acp.SessionUpdate `json:"update"`
		} `json:"params"`
	}
	var doc sessionUpdateEnvelope
	if err := json.Unmarshal([]byte(normalizeJSONDoc(raw, `{}`)), &doc); err != nil {
		return ""
	}
	if strings.TrimSpace(doc.Method) != acp.MethodSessionUpdate {
		return ""
	}
	return strings.TrimSpace(doc.Params.Update.SessionUpdate)
}

func cloneJSONRaw(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	out := make([]byte, len(raw))
	copy(out, raw)
	return json.RawMessage(out)
}

func cloneInt64Ptr(v *int64) *int64 {
	if v == nil {
		return nil
	}
	cp := *v
	return &cp
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
		return r.handlePromptStartedLocked(ctx, event, params)
	case acp.MethodSessionUpdate:
		var params acp.SessionUpdateParams
		if err := decodeSessionViewEventParams(doc, &params); err != nil {
			return fmt.Errorf("decode session.update params: %w", err)
		}
		return r.appendACPEventTurnLocked(ctx, event, doc)
	case acp.MethodRequestPermission:
		var params acp.PermissionRequestParams
		if err := decodeSessionViewEventParams(doc, &params); err == nil {
		} else if !errors.Is(err, errSessionEventPayloadEmpty) {
			return fmt.Errorf("decode request_permission params: %w", err)
		}
		return r.appendACPEventTurnLocked(ctx, event, doc)
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
	promptTitle := strings.TrimSpace(promptTitleFromBlocks(params.Prompt))
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

	state.assignTurn(turn)
	r.promptState[event.SessionID] = state
	if err := r.upsertSessionProjection(ctx, event.SessionID, promptTitle, event.UpdatedAt, true); err != nil {
		return err
	}
	return nil
}

func (r *SessionRecorder) appendACPEventTurnLocked(ctx context.Context, event SessionViewEvent, doc sessionViewACPContentDoc) error {
	state, ok, err := r.currentPromptStateLocked(ctx, event.SessionID)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	state.ensureMaps()

	turnIndex, plan, err := getMergedTurn(state, doc)
	if err != nil {
		return err
	}
	if turnIndex > 0 {
		existingTurn, ok := state.turns[turnIndex]
		if !ok {
			return nil
		}
		mergedTurn, err := mergeTurnRecord(existingTurn, event.Content, plan)
		if err != nil {
			return err
		}
		if err := r.appendSessionTurnLocked(ctx, mergedTurn, event.Content); err != nil {
			return err
		}
		state.assignTurn(mergedTurn)
		r.promptState[event.SessionID] = state
		return nil
	}

	if plan.kind == sessionTurnMergePermission && plan.hasPermissionResult {
		return nil
	}

	method := strings.TrimSpace(doc.Method)
	if method == "" {
		return fmt.Errorf("session event method is required")
	}
	turn := buildSessionTurnRecord(event.SessionID, state.promptIndex, state.nextTurnIndex, event.Content, method)
	if err := r.appendSessionTurnLocked(ctx, turn, event.Content); err != nil {
		return err
	}
	state.assignTurn(turn)
	r.promptState[event.SessionID] = state
	return nil
}

func buildSessionTurnRecord(sessionID string, promptIndex, turnIndex int64, rawContent, method string) SessionTurnRecord {
	if turnIndex <= 0 {
		turnIndex = 1
	}
	return SessionTurnRecord{
		SessionID:   strings.TrimSpace(sessionID),
		PromptIndex: promptIndex,
		TurnIndex:   turnIndex,
		UpdateIndex: 1,
		UpdateJSON:  normalizeJSONDoc(rawContent, `{"method":"`+strings.TrimSpace(method)+`"}`),
		ExtraJSON:   "{}",
	}
}

func (r *SessionRecorder) appendSessionTurnLocked(ctx context.Context, turn SessionTurnRecord, publishContent ...string) error {
	if err := r.store.UpsertSessionTurn(ctx, turn); err != nil {
		return err
	}
	content := turn.UpdateJSON
	if len(publishContent) > 0 && strings.TrimSpace(publishContent[0]) != "" {
		content = publishContent[0]
	}
	r.publishSessionTurn(turn.SessionID, turn, content)
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
			return newSessionPromptState(1, 1), nil
		}
		return newSessionPromptState(current.promptIndex+1, 1), nil
	}
	prompts, err := r.store.ListSessionPrompts(ctx, r.projectName, sessionID)
	if err != nil {
		return sessionPromptState{}, err
	}
	promptIndex := int64(1)
	if len(prompts) > 0 && prompts[len(prompts)-1].PromptIndex > 0 {
		promptIndex = prompts[len(prompts)-1].PromptIndex + 1
	}
	return newSessionPromptState(promptIndex, 1), nil
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
			state.ensureMaps()
			return state, nil
		}
	}
	prompts, err := r.store.ListSessionPrompts(ctx, r.projectName, sessionID)
	if err != nil {
		return sessionPromptState{}, err
	}
	if len(prompts) == 0 {
		state := newSessionPromptState(1, 1)
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
	state := newSessionPromptState(latest.PromptIndex, 1)
	for i := range turns {
		state.assignTurn(turns[i])
	}
	r.promptState[sessionID] = state
	return state, nil
}

func (r *SessionRecorder) currentPromptStateLocked(ctx context.Context, sessionID string) (sessionPromptState, bool, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return sessionPromptState{}, false, fmt.Errorf("session id is required")
	}
	if state, ok := r.promptState[sessionID]; ok && state.promptIndex > 0 {
		prompt, err := r.store.LoadSessionPrompt(ctx, r.projectName, sessionID, state.promptIndex)
		if err != nil {
			return sessionPromptState{}, false, err
		}
		if prompt == nil {
			delete(r.promptState, sessionID)
		} else {
			state.ensureMaps()
			return state, true, nil
		}
	}
	prompts, err := r.store.ListSessionPrompts(ctx, r.projectName, sessionID)
	if err != nil {
		return sessionPromptState{}, false, err
	}
	if len(prompts) == 0 {
		return sessionPromptState{}, false, nil
	}
	latest := prompts[len(prompts)-1]
	turns, err := r.store.ListSessionTurns(ctx, r.projectName, sessionID, latest.PromptIndex)
	if err != nil {
		return sessionPromptState{}, false, err
	}
	state := newSessionPromptState(latest.PromptIndex, 1)
	for i := range turns {
		state.assignTurn(turns[i])
	}
	r.promptState[sessionID] = state
	return state, true, nil
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
