package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/swm8023/wheelmaker/internal/protocol"
)

type codexappRPCRequest struct {
	ID     int64  `json:"id"`
	Method string `json:"method"`
	Params any    `json:"params"`
}

type codexappRPCNotification struct {
	Method string `json:"method"`
	Params any    `json:"params"`
}

type codexappRPCServerResponse struct {
	ID     json.RawMessage   `json:"id"`
	Result any               `json:"result,omitempty"`
	Error  *codexappRPCError `json:"error,omitempty"`
}

type codexappRPCEnvelope struct {
	ID     json.RawMessage   `json:"id,omitempty"`
	Method string            `json:"method,omitempty"`
	Params json.RawMessage   `json:"params,omitempty"`
	Result json.RawMessage   `json:"result,omitempty"`
	Error  *codexappRPCError `json:"error,omitempty"`
}

type codexappRPCResponse struct {
	Result json.RawMessage
	Error  *codexappRPCError
}

type codexappRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type appServerClientInfo struct {
	Name    string `json:"name"`
	Title   string `json:"title,omitempty"`
	Version string `json:"version"`
}

type appServerInitializeParams struct {
	ClientInfo appServerClientInfo `json:"clientInfo"`
}

type appServerModelListResponse struct {
	Models []appServerModel `json:"models"`
}

func (r *appServerModelListResponse) UnmarshalJSON(data []byte) error {
	var raw struct {
		Models []appServerModel `json:"models"`
		Data   []appServerModel `json:"data"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if len(raw.Models) > 0 {
		r.Models = raw.Models
		return nil
	}
	r.Models = raw.Data
	return nil
}

type appServerModel struct {
	ID                        string   `json:"id"`
	Name                      string   `json:"name,omitempty"`
	SupportedReasoningEfforts []string `json:"supportedReasoningEfforts,omitempty"`
	DefaultReasoningEffort    string   `json:"defaultReasoningEffort,omitempty"`
}

func (m *appServerModel) UnmarshalJSON(data []byte) error {
	var raw struct {
		ID                        string            `json:"id"`
		Name                      string            `json:"name"`
		DisplayName               string            `json:"displayName"`
		SupportedReasoningEfforts []json.RawMessage `json:"supportedReasoningEfforts"`
		DefaultReasoningEffort    string            `json:"defaultReasoningEffort"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	m.ID = raw.ID
	m.Name = firstNonEmptyString(raw.Name, raw.DisplayName)
	m.DefaultReasoningEffort = raw.DefaultReasoningEffort
	m.SupportedReasoningEfforts = m.SupportedReasoningEfforts[:0]
	for _, effort := range raw.SupportedReasoningEfforts {
		var value string
		if err := json.Unmarshal(effort, &value); err == nil && value != "" {
			m.SupportedReasoningEfforts = append(m.SupportedReasoningEfforts, value)
			continue
		}
		var object struct {
			ReasoningEffort string `json:"reasoningEffort"`
		}
		if err := json.Unmarshal(effort, &object); err == nil && object.ReasoningEffort != "" {
			m.SupportedReasoningEfforts = append(m.SupportedReasoningEfforts, object.ReasoningEffort)
		}
	}
	return nil
}

type appServerThreadStartParams struct {
	CWD            string `json:"cwd,omitempty"`
	Model          string `json:"model,omitempty"`
	ApprovalPolicy string `json:"approvalPolicy,omitempty"`
	Sandbox        string `json:"sandbox,omitempty"`
	ServiceName    string `json:"serviceName,omitempty"`
}

type appServerThreadResumeParams struct {
	ThreadID       string `json:"threadId"`
	CWD            string `json:"cwd,omitempty"`
	Model          string `json:"model,omitempty"`
	ApprovalPolicy string `json:"approvalPolicy,omitempty"`
	Sandbox        string `json:"sandbox,omitempty"`
}

type appServerThreadStartResponse struct {
	Thread appServerThread `json:"thread"`
}

type appServerThread struct {
	ID        string             `json:"id"`
	CWD       string             `json:"cwd,omitempty"`
	Title     string             `json:"title,omitempty"`
	Name      string             `json:"name,omitempty"`
	Preview   string             `json:"preview,omitempty"`
	UpdatedAt appServerTimestamp `json:"updatedAt,omitempty"`
}

func (t appServerThread) displayTitle() string {
	return firstNonEmptyString(t.Title, t.Name, t.Preview)
}

type appServerTimestamp string

func (s *appServerTimestamp) UnmarshalJSON(data []byte) error {
	var text string
	if err := json.Unmarshal(data, &text); err == nil {
		*s = appServerTimestamp(text)
		return nil
	}
	var number json.Number
	if err := json.Unmarshal(data, &number); err == nil {
		seconds, err := number.Int64()
		if err != nil {
			*s = appServerTimestamp(number.String())
			return nil
		}
		*s = appServerTimestamp(time.Unix(seconds, 0).UTC().Format(time.RFC3339))
		return nil
	}
	if string(data) == "null" {
		*s = ""
		return nil
	}
	return fmt.Errorf("unsupported string value %s", string(data))
}

type appServerThreadListParams struct {
	CWD    string `json:"cwd,omitempty"`
	Cursor string `json:"cursor,omitempty"`
}

type appServerThreadListResponse struct {
	Data       []appServerThread `json:"data"`
	NextCursor string            `json:"nextCursor,omitempty"`
}

type appServerTurnStartParams struct {
	ThreadID       string               `json:"threadId"`
	Input          []appServerUserInput `json:"input"`
	CWD            string               `json:"cwd,omitempty"`
	Model          string               `json:"model,omitempty"`
	Effort         string               `json:"effort,omitempty"`
	ApprovalPolicy string               `json:"approvalPolicy,omitempty"`
	SandboxPolicy  appServerSandbox     `json:"sandboxPolicy,omitempty"`
}

type appServerSandbox struct {
	Type          string   `json:"type"`
	WritableRoots []string `json:"writableRoots,omitempty"`
	NetworkAccess bool     `json:"networkAccess,omitempty"`
}

type appServerUserInput struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type appServerTurnStartResponse struct {
	Turn appServerTurn `json:"turn"`
}

type appServerTurn struct {
	ID     string `json:"id"`
	Status string `json:"status,omitempty"`
}

type appServerTurnInterruptParams struct {
	ThreadID string `json:"threadId"`
	TurnID   string `json:"turnId"`
}

type appServerAgentMessageDeltaParams struct {
	ThreadID string `json:"threadId"`
	TurnID   string `json:"turnId,omitempty"`
	ItemID   string `json:"itemId,omitempty"`
	Delta    string `json:"delta"`
}

type appServerItemEventParams struct {
	ThreadID string              `json:"threadId"`
	TurnID   string              `json:"turnId,omitempty"`
	Item     appServerThreadItem `json:"item"`
}

type appServerThreadItem struct {
	ID               string                `json:"id"`
	Type             string                `json:"type"`
	Text             string                `json:"text,omitempty"`
	Command          string                `json:"command,omitempty"`
	CWD              string                `json:"cwd,omitempty"`
	Status           string                `json:"status,omitempty"`
	AggregatedOutput string                `json:"aggregatedOutput,omitempty"`
	Server           string                `json:"server,omitempty"`
	Tool             string                `json:"tool,omitempty"`
	Query            string                `json:"query,omitempty"`
	Path             string                `json:"path,omitempty"`
	Summary          json.RawMessage       `json:"summary,omitempty"`
	Content          json.RawMessage       `json:"content,omitempty"`
	Arguments        json.RawMessage       `json:"arguments,omitempty"`
	Result           json.RawMessage       `json:"result,omitempty"`
	Error            json.RawMessage       `json:"error,omitempty"`
	Changes          []appServerFileChange `json:"changes,omitempty"`
}

type appServerFileChange struct {
	Path    string  `json:"path"`
	Kind    string  `json:"kind,omitempty"`
	Diff    string  `json:"diff,omitempty"`
	OldText *string `json:"oldText,omitempty"`
	NewText string  `json:"newText,omitempty"`
}

type appServerTurnEventParams struct {
	ThreadID string `json:"threadId"`
	TurnID   string `json:"turnId,omitempty"`
	Turn     appServerTurn
}

type appServerTurnCompletedParams struct {
	ThreadID string `json:"threadId"`
	TurnID   string `json:"turnId,omitempty"`
	Status   string `json:"status,omitempty"`
	Turn     appServerTurn
}

func (p *appServerTurnEventParams) UnmarshalJSON(data []byte) error {
	var raw struct {
		ThreadID string        `json:"threadId"`
		TurnID   string        `json:"turnId"`
		Turn     appServerTurn `json:"turn"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	p.ThreadID = raw.ThreadID
	p.Turn = raw.Turn
	p.TurnID = firstNonEmptyString(raw.TurnID, raw.Turn.ID)
	return nil
}

func (p *appServerTurnCompletedParams) UnmarshalJSON(data []byte) error {
	var raw struct {
		ThreadID string        `json:"threadId"`
		TurnID   string        `json:"turnId"`
		Status   string        `json:"status"`
		Turn     appServerTurn `json:"turn"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	p.ThreadID = raw.ThreadID
	p.Turn = raw.Turn
	p.TurnID = firstNonEmptyString(raw.TurnID, raw.Turn.ID)
	p.Status = firstNonEmptyString(raw.Status, raw.Turn.Status)
	return nil
}

type appServerThreadNameUpdatedParams struct {
	ThreadID   string `json:"threadId"`
	ThreadName string `json:"threadName,omitempty"`
	Name       string `json:"name,omitempty"`
}

func (p appServerThreadNameUpdatedParams) displayName() string {
	return firstNonEmptyString(p.ThreadName, p.Name)
}

type appServerApprovalRequestParams struct {
	ThreadID  string `json:"threadId"`
	TurnID    string `json:"turnId,omitempty"`
	ItemID    string `json:"itemId"`
	Command   string `json:"command,omitempty"`
	Path      string `json:"path,omitempty"`
	GrantRoot string `json:"grantRoot,omitempty"`
}

type appServerApprovalDecision struct {
	Decision string `json:"decision"`
}

type codexappConfigState struct {
	approvalPreset  string
	model           string
	reasoningEffort string
	models          []appServerModel
}

func newCodexappConfigState() codexappConfigState {
	return codexappConfigState{
		approvalPreset:  "auto",
		reasoningEffort: "medium",
	}
}

func (s *codexappConfigState) setModels(models []appServerModel) {
	s.models = append(s.models[:0], models...)
	if len(models) > 0 && (s.model == "" || !s.modelAllowed(s.model)) {
		s.model = models[0].ID
	}
	if !s.reasoningAllowed(s.reasoningEffort) {
		s.reasoningEffort = s.defaultReasoningEffort()
	}
}

func (s codexappConfigState) options() []protocol.ConfigOption {
	return []protocol.ConfigOption{
		{
			ID:           protocol.ConfigOptionIDApprovalPreset,
			Name:         "Approval Preset",
			Category:     protocol.ConfigOptionCategoryApprovalPreset,
			Type:         "select",
			CurrentValue: s.approvalPreset,
			Options: []protocol.ConfigOptionValue{
				{Value: "auto", Name: "Auto"},
				{Value: "read_only", Name: "Read-only"},
				{Value: "full", Name: "Full Access"},
			},
		},
		{
			ID:           protocol.ConfigOptionIDModel,
			Name:         "Model",
			Category:     protocol.ConfigOptionCategoryModel,
			Type:         "select",
			CurrentValue: s.model,
			Options:      s.modelOptions(),
		},
		{
			ID:           protocol.ConfigOptionIDReasoningEffort,
			Name:         "Reasoning Effort",
			Category:     protocol.ConfigOptionCategoryThoughtLv,
			Type:         "select",
			CurrentValue: s.reasoningEffort,
			Options:      s.reasoningOptions(),
		},
	}
}

func (s *codexappConfigState) set(id, value string) error {
	switch id {
	case protocol.ConfigOptionIDApprovalPreset:
		if _, ok := codexappApprovalProfile(value); !ok {
			return fmt.Errorf("invalid approval preset %q", value)
		}
		if value == "ask" {
			value = "auto"
		}
		s.approvalPreset = value
	case protocol.ConfigOptionIDModel:
		if !s.modelAllowed(value) {
			return fmt.Errorf("invalid model %q", value)
		}
		s.model = value
		if !s.reasoningAllowed(s.reasoningEffort) {
			s.reasoningEffort = s.defaultReasoningEffort()
		}
	case protocol.ConfigOptionIDReasoningEffort:
		if !s.reasoningAllowed(value) {
			return fmt.Errorf("invalid reasoning effort %q", value)
		}
		s.reasoningEffort = value
	default:
		return fmt.Errorf("unsupported config option %q", id)
	}
	return nil
}

func (s codexappConfigState) threadStartParams(cwd string) appServerThreadStartParams {
	profile, _ := codexappApprovalProfile(s.approvalPreset)
	return appServerThreadStartParams{
		CWD:            cwd,
		Model:          s.model,
		ApprovalPolicy: profile.approvalPolicy,
		Sandbox:        profile.threadSandbox,
		ServiceName:    "wheelmaker",
	}
}

func (s codexappConfigState) threadResumeParams(threadID, cwd string) appServerThreadResumeParams {
	profile, _ := codexappApprovalProfile(s.approvalPreset)
	return appServerThreadResumeParams{
		ThreadID:       threadID,
		CWD:            cwd,
		Model:          s.model,
		ApprovalPolicy: profile.approvalPolicy,
		Sandbox:        profile.threadSandbox,
	}
}

func (s codexappConfigState) turnStartParams(threadID, cwd string, input []appServerUserInput) appServerTurnStartParams {
	profile, _ := codexappApprovalProfile(s.approvalPreset)
	return appServerTurnStartParams{
		ThreadID:       threadID,
		Input:          input,
		CWD:            cwd,
		Model:          s.model,
		Effort:         s.reasoningEffort,
		ApprovalPolicy: profile.approvalPolicy,
		SandboxPolicy: appServerSandbox{
			Type:          profile.turnSandboxType,
			WritableRoots: profile.writableRoots(cwd),
			NetworkAccess: profile.networkAccess,
		},
	}
}

func (s codexappConfigState) modelOptions() []protocol.ConfigOptionValue {
	out := make([]protocol.ConfigOptionValue, 0, len(s.models))
	for _, model := range s.models {
		out = append(out, protocol.ConfigOptionValue{Value: model.ID, Name: firstNonEmptyString(model.Name, model.ID)})
	}
	return out
}

func (s codexappConfigState) reasoningOptions() []protocol.ConfigOptionValue {
	efforts := s.currentModel().SupportedReasoningEfforts
	if len(efforts) == 0 {
		efforts = []string{"low", "medium", "high"}
	}
	out := make([]protocol.ConfigOptionValue, 0, len(efforts))
	for _, effort := range efforts {
		out = append(out, protocol.ConfigOptionValue{Value: effort, Name: effort})
	}
	return out
}

func (s codexappConfigState) modelAllowed(value string) bool {
	if value == "" && len(s.models) == 0 {
		return true
	}
	for _, model := range s.models {
		if model.ID == value {
			return true
		}
	}
	return false
}

func (s codexappConfigState) reasoningAllowed(value string) bool {
	if value == "" {
		return true
	}
	for _, effort := range s.currentModel().SupportedReasoningEfforts {
		if effort == value {
			return true
		}
	}
	return len(s.currentModel().SupportedReasoningEfforts) == 0 &&
		(value == "low" || value == "medium" || value == "high")
}

func (s codexappConfigState) defaultReasoningEffort() string {
	if effort := s.currentModel().DefaultReasoningEffort; effort != "" {
		return effort
	}
	return "medium"
}

func (s codexappConfigState) currentModel() appServerModel {
	for _, model := range s.models {
		if model.ID == s.model {
			return model
		}
	}
	return appServerModel{}
}

type codexappApprovalPreset struct {
	approvalPolicy  string
	threadSandbox   string
	turnSandboxType string
	networkAccess   bool
}

func (p codexappApprovalPreset) writableRoots(cwd string) []string {
	if p.turnSandboxType != "workspaceWrite" || strings.TrimSpace(cwd) == "" {
		return nil
	}
	return []string{cwd}
}

func codexappApprovalProfile(preset string) (codexappApprovalPreset, bool) {
	switch preset {
	case "read_only":
		return codexappApprovalPreset{approvalPolicy: "on-request", threadSandbox: "read-only", turnSandboxType: "readOnly"}, true
	case "auto", "ask":
		return codexappApprovalPreset{approvalPolicy: "on-request", threadSandbox: "workspace-write", turnSandboxType: "workspaceWrite"}, true
	case "full":
		return codexappApprovalPreset{
			approvalPolicy:  "never",
			threadSandbox:   "danger-full-access",
			turnSandboxType: "dangerFullAccess",
			networkAccess:   true,
		}, true
	default:
		return codexappApprovalPreset{}, false
	}
}

func codexappPromptToInput(blocks []protocol.ContentBlock) ([]appServerUserInput, error) {
	if len(blocks) == 0 {
		return nil, errors.New("codexapp prompt is empty")
	}
	out := make([]appServerUserInput, 0, len(blocks))
	for _, block := range blocks {
		switch block.Type {
		case protocol.ContentBlockTypeText:
			if block.Text != "" {
				out = append(out, appServerUserInput{Type: "text", Text: block.Text})
			}
		default:
			return nil, fmt.Errorf("codexapp phase 1 does not support prompt content type %q", block.Type)
		}
	}
	if len(out) == 0 {
		return nil, errors.New("codexapp prompt contains no text")
	}
	return out, nil
}

func codexappInputText(input []appServerUserInput) string {
	var parts []string
	for _, item := range input {
		if item.Text != "" {
			parts = append(parts, item.Text)
		}
	}
	return strings.Join(parts, "\n")
}

func codexappThreadIDFromParams(raw json.RawMessage) string {
	var p struct {
		ThreadID string `json:"threadId"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return ""
	}
	return strings.TrimSpace(p.ThreadID)
}

func codexappStopReason(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "cancelled", "canceled", "interrupted":
		return protocol.StopReasonCancelled
	case "max_tokens":
		return protocol.StopReasonMaxTokens
	default:
		return protocol.StopReasonEndTurn
	}
}

func codexappApprovalDecision(outcome protocol.PermissionResult) string {
	value := firstNonEmptyString(outcome.OptionID, outcome.Outcome)
	switch value {
	case "allow_once":
		return "accept"
	case "allow_always":
		return "acceptForSession"
	case "reject", "reject_once", "reject_always":
		return "decline"
	default:
		return "cancel"
	}
}

func codexappParams(params any) any {
	if params == nil {
		return map[string]any{}
	}
	return params
}
