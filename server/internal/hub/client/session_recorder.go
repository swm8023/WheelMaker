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
	if key := sessionTurnToolCallIDKey(turn.ExtraJSON); key != "" {
		s.toolTurnByToolCallID[key] = turn.TurnIndex
	}
	if requestID := sessionTurnPermissionRequestIDKey(turn.ExtraJSON); requestID > 0 {
		s.permissionTurnByRequestID[requestID] = turn.TurnIndex
	}
}

type sessionTurnMergeKind string

const (
	sessionTurnMergeNone       sessionTurnMergeKind = ""
	sessionTurnMergeTool       sessionTurnMergeKind = "tool"
	sessionTurnMergeText       sessionTurnMergeKind = "text"
	sessionTurnMergePermission sessionTurnMergeKind = "permission"
)

type sessionTurnMergePlan struct {
	kind                sessionTurnMergeKind
	toolCallID          string
	requestID           int64
	hasPermissionResult bool
}

type sessionTurnKey struct {
	ToolCallID          string
	PermissionRequestID int64
}

type sessionViewTurnMessage struct {
	IMMessage acp.IMMessage
	MergeKey  sessionTurnKey
}

func isToolSessionUpdateType(updateMethod string) bool {
	switch strings.TrimSpace(updateMethod) {
	case acp.SessionUpdateToolCall, acp.SessionUpdateToolCallUpdate:
		return true
	default:
		return false
	}
}

func isTextSessionUpdateType(updateMethod string) bool {
	switch strings.TrimSpace(updateMethod) {
	case acp.SessionUpdateAgentMessageChunk, acp.SessionUpdateAgentThoughtChunk, acp.SessionUpdateUserMessageChunk:
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
		case isTextSessionUpdateType(updateMethod):
			plan.kind = sessionTurnMergeText
			lastTurnIndex := state.nextTurnIndex - 1
			if lastTurnIndex > 0 {
				if message, ok := state.turns[lastTurnIndex]; ok && strings.TrimSpace(sessionTurnMethodKey(message.UpdateJSON)) == updateMethod {
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
	case acp.IMMethodPermission:
		if turn.MergeKey.PermissionRequestID <= 0 {
			return 0, plan
		}
		plan.kind = sessionTurnMergePermission
		plan.requestID = turn.MergeKey.PermissionRequestID
		plan.hasPermissionResult = len(turn.IMMessage.Result) > 0 && strings.TrimSpace(string(turn.IMMessage.Result)) != ""
		if turnIndex := state.permissionTurnByRequestID[plan.requestID]; turnIndex > 0 {
			return turnIndex, plan
		}
	}
	return 0, plan
}
func mergeTurnRecord(existing SessionTurnRecord, incomingMessage acp.IMMessage, plan sessionTurnMergePlan) (SessionTurnRecord, error) {
	merged := existing
	merged.UpdateIndex = maxInt64(existing.UpdateIndex, 0) + 1
	merged.UpdateJSON = normalizeJSONDoc(existing.UpdateJSON, `{}`)
	merged.ExtraJSON = normalizeJSONDoc(existing.ExtraJSON, `{}`)

	existingMessage, err := decodeIMMessage(merged.UpdateJSON)
	if err != nil {
		return SessionTurnRecord{}, err
	}

	var mergedMessage acp.IMMessage
	switch plan.kind {
	case sessionTurnMergeTool:
		mergedMessage, err = mergeToolResultMessage(existingMessage, incomingMessage)
	case sessionTurnMergePermission:
		mergedMessage, err = mergePermissionMessage(existingMessage, incomingMessage)
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
	if len(existing.Result) > 0 {
		if err := json.Unmarshal(existing.Result, &base); err != nil {
			return acp.IMMessage{}, err
		}
	}
	inc := acp.IMTextResult{}
	if len(incoming.Result) > 0 {
		if err := json.Unmarshal(incoming.Result, &inc); err != nil {
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
	incoming.Result = cloneJSONRaw(raw)
	if len(incoming.Request) == 0 {
		incoming.Request = cloneJSONRaw(existing.Request)
	}
	return incoming, nil
}

func mergeToolResultMessage(existing, incoming acp.IMMessage) (acp.IMMessage, error) {
	base := acp.IMToolResult{}
	if len(existing.Result) > 0 {
		if err := json.Unmarshal(existing.Result, &base); err != nil {
			return acp.IMMessage{}, err
		}
	}
	inc := acp.IMToolResult{}
	if len(incoming.Result) > 0 {
		if err := json.Unmarshal(incoming.Result, &inc); err != nil {
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
	incoming.Result = cloneJSONRaw(raw)
	if len(incoming.Request) == 0 {
		incoming.Request = cloneJSONRaw(existing.Request)
	}
	return incoming, nil
}

func mergePermissionMessage(existing, incoming acp.IMMessage) (acp.IMMessage, error) {
	if len(incoming.Request) == 0 {
		incoming.Request = cloneJSONRaw(existing.Request)
	}
	if len(incoming.Result) == 0 {
		incoming.Result = cloneJSONRaw(existing.Result)
		return incoming, nil
	}

	inc := acp.IMPermissionResult{}
	if err := json.Unmarshal(incoming.Result, &inc); err != nil {
		return acp.IMMessage{}, err
	}
	if strings.TrimSpace(inc.ToolCallID) == "" && len(incoming.Request) > 0 {
		request := acp.IMPermissionRequest{}
		if err := json.Unmarshal(incoming.Request, &request); err == nil {
			inc.ToolCallID = strings.TrimSpace(request.ToolCallID)
		}
	}

	if len(existing.Result) > 0 {
		base := acp.IMPermissionResult{}
		if err := json.Unmarshal(existing.Result, &base); err != nil {
			return acp.IMMessage{}, err
		}
		if strings.TrimSpace(inc.ToolCallID) == "" {
			inc.ToolCallID = strings.TrimSpace(base.ToolCallID)
		}
		if strings.TrimSpace(inc.Selected) == "" {
			inc.Selected = strings.TrimSpace(base.Selected)
		}
	}

	resultRaw, err := json.Marshal(inc)
	if err != nil {
		return acp.IMMessage{}, err
	}
	incoming.Result = cloneJSONRaw(resultRaw)
	return incoming, nil
}

func buildTurnMessageFromACPDoc(doc sessionViewACPContentDoc) (sessionViewTurnMessage, bool, error) {
	switch strings.TrimSpace(doc.Method) {
	case acp.MethodSessionUpdate:
		var params acp.SessionUpdateParams
		if err := decodeSessionViewEventParams(doc, &params); err != nil {
			if errors.Is(err, errSessionEventPayloadEmpty) {
				return sessionViewTurnMessage{}, false, nil
			}
			return sessionViewTurnMessage{}, false, err
		}
		return buildTurnMessageFromSessionUpdate(params.Update)
	case acp.MethodRequestPermission:
		return buildTurnMessageFromPermission(doc)
	case acp.IMMethodSystem:
		return buildTurnMessageFromSystemDoc(doc)
	default:
		return sessionViewTurnMessage{}, false, nil
	}
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
		message.Result = cloneJSONRaw(resultRaw)
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
		message.Result = cloneJSONRaw(resultRaw)
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
		message.Result = cloneJSONRaw(resultRaw)
		return sessionViewTurnMessage{IMMessage: message}, true, nil
	default:
		return sessionViewTurnMessage{}, false, nil
	}
}

func buildTurnMessageFromPermission(doc sessionViewACPContentDoc) (sessionViewTurnMessage, bool, error) {
	if doc.ID <= 0 {
		return sessionViewTurnMessage{}, false, nil
	}
	converted := sessionViewTurnMessage{
		IMMessage: acp.IMMessage{Method: acp.IMMethodPermission},
		MergeKey:  sessionTurnKey{PermissionRequestID: doc.ID},
	}

	if len(doc.Params) > 0 && strings.TrimSpace(string(doc.Params)) != "" {
		params := acp.PermissionRequestParams{}
		if err := decodeSessionViewEventParams(doc, &params); err != nil {
			return sessionViewTurnMessage{}, false, err
		}
		converted.MergeKey.ToolCallID = strings.TrimSpace(params.ToolCall.ToolCallID)
		options := make([]acp.IMRequestOption, 0, len(params.Options))
		for _, option := range params.Options {
			options = append(options, acp.IMRequestOption{OptionID: strings.TrimSpace(option.OptionID), Name: strings.TrimSpace(option.Name)})
		}
		requestRaw, err := json.Marshal(acp.IMPermissionRequest{
			ToolCallID: strings.TrimSpace(params.ToolCall.ToolCallID),
			Options:    options,
		})
		if err != nil {
			return sessionViewTurnMessage{}, false, err
		}
		converted.IMMessage.Request = cloneJSONRaw(requestRaw)
	}

	if len(doc.Result) > 0 && strings.TrimSpace(string(doc.Result)) != "" {
		response := acp.PermissionResponse{}
		if err := decodeSessionViewEventResult(doc, &response); err != nil {
			return sessionViewTurnMessage{}, false, err
		}
		selected := strings.TrimSpace(response.Outcome.OptionID)
		if selected == "" {
			selected = strings.TrimSpace(response.Outcome.Outcome)
		}
		resultRaw, err := json.Marshal(acp.IMPermissionResult{
			ToolCallID: strings.TrimSpace(converted.MergeKey.ToolCallID),
			Selected:   selected,
		})
		if err != nil {
			return sessionViewTurnMessage{}, false, err
		}
		converted.IMMessage.Result = cloneJSONRaw(resultRaw)
	}

	if len(converted.IMMessage.Request) == 0 && len(converted.IMMessage.Result) == 0 {
		return sessionViewTurnMessage{}, false, nil
	}
	return converted, true, nil
}

func buildTurnMessageFromSystemDoc(doc sessionViewACPContentDoc) (sessionViewTurnMessage, bool, error) {
	if len(doc.Result) == 0 || strings.TrimSpace(string(doc.Result)) == "" {
		return sessionViewTurnMessage{}, false, nil
	}
	text := strings.TrimSpace(extractUpdateText(doc.Result))
	if text == "" {
		text = strings.TrimSpace(stringifyRawJSON(doc.Result))
	}
	return buildTurnMessageFromSystemText(text)
}

func buildTurnMessageFromSystemText(text string) (sessionViewTurnMessage, bool, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return sessionViewTurnMessage{}, false, nil
	}
	resultRaw, err := json.Marshal(acp.IMTextResult{Text: text})
	if err != nil {
		return sessionViewTurnMessage{}, false, err
	}
	message := acp.IMMessage{Method: acp.IMMethodSystem, Result: cloneJSONRaw(resultRaw)}
	return sessionViewTurnMessage{IMMessage: message}, true, nil
}
func marshalConvertedMessage(message sessionViewTurnMessage) (string, string, error) {
	metaRaw, err := json.Marshal(sessionTurnMetaFromMergeKey(message.MergeKey))
	if err != nil {
		return "", "", err
	}
	return marshalIMMessageWithMeta(message.IMMessage, string(metaRaw))
}

func marshalIMMessage(message acp.IMMessage) (string, string, error) {
	return marshalIMMessageWithMeta(message, `{}`)
}

func marshalIMMessageWithMeta(message acp.IMMessage, metaRaw string) (string, string, error) {
	raw, err := json.Marshal(message)
	if err != nil {
		return "", "", err
	}
	return normalizeJSONDoc(string(raw), `{}`), normalizeJSONDoc(metaRaw, `{}`), nil
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

func extractUpdateText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text
	}
	block := acp.ContentBlock{}
	if err := json.Unmarshal(raw, &block); err == nil {
		return block.Text
	}
	obj := map[string]any{}
	if err := json.Unmarshal(raw, &obj); err == nil {
		if textValue, ok := obj["text"].(string); ok {
			return textValue
		}
	}
	return ""
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

type sessionTurnMeta struct {
	ToolCallID          string `json:"toolCallId,omitempty"`
	PermissionRequestID int64  `json:"permissionRequestId,omitempty"`
}

func sessionTurnMetaFromMergeKey(key sessionTurnKey) sessionTurnMeta {
	return sessionTurnMeta{
		ToolCallID:          strings.TrimSpace(key.ToolCallID),
		PermissionRequestID: key.PermissionRequestID,
	}
}

func sessionTurnToolCallIDKey(raw string) string {
	meta := sessionTurnMeta{}
	if err := json.Unmarshal([]byte(normalizeJSONDoc(raw, `{}`)), &meta); err != nil {
		return ""
	}
	return strings.TrimSpace(meta.ToolCallID)
}

func sessionTurnPermissionRequestIDKey(raw string) int64 {
	meta := sessionTurnMeta{}
	if err := json.Unmarshal([]byte(normalizeJSONDoc(raw, `{}`)), &meta); err != nil {
		return 0
	}
	if meta.PermissionRequestID <= 0 {
		return 0
	}
	return meta.PermissionRequestID
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

type sessionViewParsedEvent struct {
	event            SessionViewEvent
	skip             bool
	method           string
	sessionTitle     string
	hasPromptResult  bool
	promptStopReason string
	promptParams     acp.SessionPromptParams
	hasTurnMessage   bool
	turnMessage      sessionViewTurnMessage
}

func parseSessionViewEvent(event SessionViewEvent) (sessionViewParsedEvent, error) {
	event.SessionID = strings.TrimSpace(event.SessionID)
	if event.SessionID == "" {
		return sessionViewParsedEvent{skip: true}, nil
	}
	if event.UpdatedAt.IsZero() {
		event.UpdatedAt = time.Now().UTC()
	}
	doc := sessionViewACPContentDoc{}
	if strings.EqualFold(strings.TrimSpace(string(event.Type)), string(SessionViewEventTypeACP)) {
		parsedDoc, err := decodeSessionViewACPContentDoc(event.Content)
		if err != nil {
			return sessionViewParsedEvent{}, fmt.Errorf("decode acp event content: %w", err)
		}
		doc = parsedDoc
	}

	parsed := sessionViewParsedEvent{
		event:  event,
		method: sessionViewMethodFromEvent(event, doc),
	}
	switch parsed.method {
	case acp.MethodSessionNew:
		params := sessionViewSessionNewParams{}
		if err := decodeSessionViewEventParams(doc, &params); err != nil && !errors.Is(err, errSessionEventPayloadEmpty) {
			return sessionViewParsedEvent{}, fmt.Errorf("decode session.new params: %w", err)
		}
		parsed.sessionTitle = strings.TrimSpace(params.Title)
	case acp.MethodSessionPrompt:
		var promptResult sessionViewPromptResult
		if err := decodeSessionViewEventResult(doc, &promptResult); err == nil {
			parsed.hasPromptResult = true
			parsed.promptStopReason = strings.TrimSpace(promptResult.StopReason)
			return parsed, nil
		} else if !errors.Is(err, errSessionEventPayloadEmpty) {
			return sessionViewParsedEvent{}, fmt.Errorf("decode session.prompt result: %w", err)
		}
		var params acp.SessionPromptParams
		if err := decodeSessionViewEventParams(doc, &params); err != nil {
			return sessionViewParsedEvent{}, fmt.Errorf("decode session.prompt params: %w", err)
		}
		parsed.promptParams = params
	case acp.MethodSessionUpdate:
		var params acp.SessionUpdateParams
		if err := decodeSessionViewEventParams(doc, &params); err != nil {
			return sessionViewParsedEvent{}, fmt.Errorf("decode session.update params: %w", err)
		}
		turn, ok, err := buildTurnMessageFromACPDoc(doc)
		if err != nil {
			return sessionViewParsedEvent{}, fmt.Errorf("build session.update message: %w", err)
		}
		parsed.turnMessage = turn
		parsed.hasTurnMessage = ok
	case acp.MethodRequestPermission:
		var params acp.PermissionRequestParams
		if err := decodeSessionViewEventParams(doc, &params); err == nil {
		} else if !errors.Is(err, errSessionEventPayloadEmpty) {
			return sessionViewParsedEvent{}, fmt.Errorf("decode request_permission params: %w", err)
		}
		turn, ok, err := buildTurnMessageFromACPDoc(doc)
		if err != nil {
			return sessionViewParsedEvent{}, fmt.Errorf("build request_permission message: %w", err)
		}
		parsed.turnMessage = turn
		parsed.hasTurnMessage = ok
	case sessionViewMethodSystem:
		if strings.EqualFold(strings.TrimSpace(string(event.Type)), string(SessionViewEventTypeSystem)) {
			turn, ok, err := buildTurnMessageFromSystemText(event.Content)
			if err != nil {
				return sessionViewParsedEvent{}, fmt.Errorf("build system message from text: %w", err)
			}
			parsed.turnMessage = turn
			parsed.hasTurnMessage = ok
		} else {
			turn, ok, err := buildTurnMessageFromACPDoc(doc)
			if err != nil {
				return sessionViewParsedEvent{}, fmt.Errorf("build system message: %w", err)
			}
			parsed.turnMessage = turn
			parsed.hasTurnMessage = ok
		}
	}
	return parsed, nil
}

func (r *SessionRecorder) RecordEvent(ctx context.Context, event SessionViewEvent) error {
	parsed, err := parseSessionViewEvent(event)
	if err != nil {
		return err
	}
	if parsed.skip {
		return nil
	}

	r.writeMu.Lock()
	defer r.writeMu.Unlock()

	switch parsed.method {
	case acp.MethodSessionNew:
		return r.upsertSessionProjection(ctx, parsed.event.SessionID, parsed.sessionTitle, parsed.event.UpdatedAt, false)
	case acp.MethodSessionPrompt:
		if parsed.hasPromptResult {
			return r.handlePromptFinishedLocked(ctx, parsed.event, parsed.promptStopReason)
		}
		return r.handlePromptStartedLocked(ctx, parsed.event, parsed.promptParams)
	case acp.MethodSessionUpdate, acp.MethodRequestPermission, sessionViewMethodSystem:
		return r.appendACPEventMessageLocked(ctx, parsed.event, parsed.turnMessage, parsed.hasTurnMessage)
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

	requestRaw, err := json.Marshal(acp.IMPromptRequest{ContentBlocks: cloneSessionContentBlocks(params.Prompt)})
	if err != nil {
		return err
	}
	messageRaw, _, err := marshalIMMessage(acp.IMMessage{Method: acp.IMMethodPrompt, Request: cloneJSONRaw(requestRaw)})
	if err != nil {
		return err
	}
	indexedRaw, err := withIMTurnIndex(messageRaw, state.promptIndex, 1)
	if err != nil {
		return err
	}

	message := buildSessionTurnRecord(event.SessionID, state.promptIndex, 1, indexedRaw, `{}`)
	if err := r.appendSessionTurnLocked(ctx, message); err != nil {
		return err
	}

	state.assignTurn(message)
	r.promptState[event.SessionID] = state
	if err := r.upsertSessionProjection(ctx, event.SessionID, promptTitle, event.UpdatedAt, true); err != nil {
		return err
	}
	return nil
}

func (r *SessionRecorder) appendACPEventMessageLocked(ctx context.Context, event SessionViewEvent, turnMessage sessionViewTurnMessage, hasTurnMessage bool) error {
	state, ok, err := r.currentPromptStateLocked(ctx, event.SessionID)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	state.ensureMaps()

	turnIndex, plan := getMergedTurnFromTurnMessage(state, turnMessage)
	if !hasTurnMessage {
		return nil
	}
	converted := turnMessage

	if turnIndex > 0 {
		existingTurn, ok := state.turns[turnIndex]
		if !ok {
			return nil
		}
		indexedMessage := converted.IMMessage
		indexedMessage.Index = formatPromptTurnSeq(state.promptIndex, turnIndex)
		indexedIncomingRaw, _, err := marshalIMMessage(indexedMessage)
		if err != nil {
			return err
		}
		mergedTurn, err := mergeTurnRecord(existingTurn, indexedMessage, plan)
		if err != nil {
			return err
		}
		if err := r.appendSessionTurnLocked(ctx, mergedTurn, indexedIncomingRaw); err != nil {
			return err
		}
		state.assignTurn(mergedTurn)
		r.promptState[event.SessionID] = state
		return nil
	}

	if plan.kind == sessionTurnMergePermission && plan.hasPermissionResult {
		return nil
	}

	indexed := converted
	indexed.IMMessage.Index = formatPromptTurnSeq(state.promptIndex, state.nextTurnIndex)
	indexedIncomingRaw, incomingMetaRaw, err := marshalConvertedMessage(indexed)
	if err != nil {
		return err
	}
	message := buildSessionTurnRecord(event.SessionID, state.promptIndex, state.nextTurnIndex, indexedIncomingRaw, incomingMetaRaw)
	if err := r.appendSessionTurnLocked(ctx, message, indexedIncomingRaw); err != nil {
		return err
	}
	state.assignTurn(message)
	r.promptState[event.SessionID] = state
	return nil
}

func buildSessionTurnRecord(sessionID string, promptIndex, turnIndex int64, rawContent, metaRaw string) SessionTurnRecord {
	if turnIndex <= 0 {
		turnIndex = 1
	}
	return SessionTurnRecord{
		SessionID:   strings.TrimSpace(sessionID),
		PromptIndex: promptIndex,
		TurnIndex:   turnIndex,
		UpdateIndex: 1,
		UpdateJSON:  normalizeJSONDoc(rawContent, `{}`),
		ExtraJSON:   normalizeJSONDoc(metaRaw, `{}`),
	}
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
		if err := r.store.UpsertSessionPrompt(ctx, SessionPromptRecord{SessionID: sessionID, PromptIndex: 1, UpdatedAt: updatedAt}); err != nil {
			return sessionPromptState{}, err
		}
		r.promptState[sessionID] = state
		return state, nil
	}
	latest := prompts[len(prompts)-1]
	messages, err := r.store.ListSessionTurns(ctx, r.projectName, sessionID, latest.PromptIndex)
	if err != nil {
		return sessionPromptState{}, err
	}
	state := newSessionPromptState(latest.PromptIndex, 1)
	for i := range messages {
		state.assignTurn(messages[i])
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
	messages, err := r.store.ListSessionTurns(ctx, r.projectName, sessionID, latest.PromptIndex)
	if err != nil {
		return sessionPromptState{}, false, err
	}
	state := newSessionPromptState(latest.PromptIndex, 1)
	for i := range messages {
		state.assignTurn(messages[i])
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

		"content": content,
	})
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
		messages, err := r.store.ListSessionTurns(ctx, r.projectName, sessionID, prompt.PromptIndex)
		if err != nil {
			return sessionViewSummary{}, nil, 0, 0, err
		}
		for _, turn := range messages {
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
