package agent

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/swm8023/wheelmaker/internal/protocol"
)

var codexappMaxImageBytes = 20 * 1024 * 1024

var codexappArtifactRootPathFunc = func() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".wheelmaker"), nil
}

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
	Models []appServerModel `json:"data"`
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
	SessionID string             `json:"sessionId,omitempty"`
	CWD       string             `json:"cwd,omitempty"`
	Name      string             `json:"name,omitempty"`
	Preview   string             `json:"preview,omitempty"`
	UpdatedAt appServerTimestamp `json:"updatedAt,omitempty"`
	Turns     []appServerTurn    `json:"turns,omitempty"`
}

func (t appServerThread) displayTitle() string {
	return firstNonEmptyString(t.Name, t.Preview)
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

type appServerThreadReadParams struct {
	ThreadID     string `json:"threadId"`
	IncludeTurns bool   `json:"includeTurns"`
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
	Type                string   `json:"type"`
	WritableRoots       []string `json:"writableRoots,omitempty"`
	NetworkAccess       *bool    `json:"networkAccess,omitempty"`
	ExcludeTmpdirEnvVar *bool    `json:"excludeTmpdirEnvVar,omitempty"`
	ExcludeSlashTmp     *bool    `json:"excludeSlashTmp,omitempty"`
}

type appServerUserInput struct {
	Type         string `json:"type"`
	Text         string `json:"text,omitempty"`
	TextElements []any  `json:"text_elements"`
	URL          string `json:"url,omitempty"`
	Path         string `json:"path,omitempty"`
	Name         string `json:"name,omitempty"`
}

func (i appServerUserInput) MarshalJSON() ([]byte, error) {
	switch i.Type {
	case "text":
		elements := i.TextElements
		if elements == nil {
			elements = []any{}
		}
		return json.Marshal(struct {
			Type         string `json:"type"`
			Text         string `json:"text"`
			TextElements []any  `json:"text_elements"`
		}{Type: i.Type, Text: i.Text, TextElements: elements})
	case "image":
		return json.Marshal(struct {
			Type string `json:"type"`
			URL  string `json:"url"`
		}{Type: i.Type, URL: i.URL})
	case "localImage":
		return json.Marshal(struct {
			Type string `json:"type"`
			Path string `json:"path"`
		}{Type: i.Type, Path: i.Path})
	case "skill", "mention":
		return json.Marshal(struct {
			Type string `json:"type"`
			Name string `json:"name"`
			Path string `json:"path"`
		}{Type: i.Type, Name: i.Name, Path: i.Path})
	default:
		type alias appServerUserInput
		return json.Marshal(alias(i))
	}
}

type appServerTurnStartResponse struct {
	Turn appServerTurn `json:"turn"`
}

type appServerTurn struct {
	ID        string                `json:"id"`
	Items     []appServerThreadItem `json:"items,omitempty"`
	ItemsView string                `json:"itemsView,omitempty"`
	Status    string                `json:"status,omitempty"`
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
	Phase            string                `json:"phase,omitempty"`
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
	Path    string                   `json:"path"`
	Kind    appServerPatchChangeKind `json:"kind,omitempty"`
	Diff    string                   `json:"diff,omitempty"`
	OldText *string                  `json:"oldText,omitempty"`
	NewText string                   `json:"newText,omitempty"`
}

type appServerPatchChangeKind struct {
	Type     string  `json:"type"`
	MovePath *string `json:"move_path,omitempty"`
}

type appServerTurnEventParams struct {
	ThreadID string        `json:"threadId"`
	Turn     appServerTurn `json:"turn"`
}

func (p appServerTurnEventParams) turnID() string {
	return p.Turn.ID
}

type appServerTurnCompletedParams struct {
	ThreadID string        `json:"threadId"`
	Turn     appServerTurn `json:"turn"`
}

func (p appServerTurnCompletedParams) turnID() string {
	return p.Turn.ID
}

func (p appServerTurnCompletedParams) status() string {
	return p.Turn.Status
}

type appServerThreadNameUpdatedParams struct {
	ThreadID   string `json:"threadId"`
	ThreadName string `json:"threadName,omitempty"`
}

func (p appServerThreadNameUpdatedParams) displayName() string {
	return p.ThreadName
}

type appServerApprovalRequestParams struct {
	ThreadID    string          `json:"threadId"`
	TurnID      string          `json:"turnId,omitempty"`
	ItemID      string          `json:"itemId"`
	Command     string          `json:"command,omitempty"`
	CWD         string          `json:"cwd,omitempty"`
	Reason      string          `json:"reason,omitempty"`
	Path        string          `json:"path,omitempty"`
	GrantRoot   string          `json:"grantRoot,omitempty"`
	Permissions json.RawMessage `json:"permissions,omitempty"`
}

type appServerApprovalDecision struct {
	Decision string `json:"decision"`
}

type appServerPermissionsApprovalResponse struct {
	Permissions json.RawMessage `json:"permissions"`
	Scope       string          `json:"scope"`
}

type appServerMcpElicitationResponse struct {
	Action  string `json:"action"`
	Content any    `json:"content"`
	Meta    any    `json:"_meta"`
}

type appServerTurnPlanUpdatedParams struct {
	ThreadID string              `json:"threadId"`
	TurnID   string              `json:"turnId"`
	Plan     []appServerPlanStep `json:"plan"`
}

type appServerPlanStep struct {
	Step   string `json:"step"`
	Status string `json:"status"`
}

type appServerFileChangePatchUpdatedParams struct {
	ThreadID string                `json:"threadId"`
	TurnID   string                `json:"turnId"`
	ItemID   string                `json:"itemId"`
	Changes  []appServerFileChange `json:"changes"`
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
			CurrentValue: codexappNormalizeApprovalPreset(s.approvalPreset),
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
		normalized := codexappNormalizeApprovalPreset(value)
		if _, ok := codexappApprovalProfile(normalized); !ok {
			return fmt.Errorf("invalid approval preset %q", value)
		}
		s.approvalPreset = normalized
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
		SandboxPolicy:  profile.sandboxPolicy(cwd),
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

func (p codexappApprovalPreset) sandboxPolicy(cwd string) appServerSandbox {
	switch p.turnSandboxType {
	case "dangerFullAccess":
		return appServerSandbox{Type: "dangerFullAccess"}
	case "readOnly":
		network := p.networkAccess
		return appServerSandbox{Type: "readOnly", NetworkAccess: &network}
	default:
		network := p.networkAccess
		excludeTmpdir := false
		excludeSlashTmp := false
		return appServerSandbox{
			Type:                "workspaceWrite",
			WritableRoots:       p.writableRoots(cwd),
			NetworkAccess:       &network,
			ExcludeTmpdirEnvVar: &excludeTmpdir,
			ExcludeSlashTmp:     &excludeSlashTmp,
		}
	}
}

func codexappApprovalProfile(preset string) (codexappApprovalPreset, bool) {
	switch codexappNormalizeApprovalPreset(preset) {
	case "read_only":
		return codexappApprovalPreset{approvalPolicy: "on-request", threadSandbox: "read-only", turnSandboxType: "readOnly"}, true
	case "auto":
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

func codexappNormalizeApprovalPreset(preset string) string {
	if preset == "ask" {
		return "auto"
	}
	return preset
}

func codexappPromptToInput(blocks []protocol.ContentBlock) ([]appServerUserInput, error) {
	return codexappPromptToInputWithArtifacts("", "", blocks)
}

func codexappPromptToInputWithArtifacts(projectName string, sessionID string, blocks []protocol.ContentBlock) ([]appServerUserInput, error) {
	if len(blocks) == 0 {
		return nil, errors.New("codexapp prompt is empty")
	}
	out := make([]appServerUserInput, 0, len(blocks))
	for _, block := range blocks {
		switch block.Type {
		case protocol.ContentBlockTypeText:
			if block.Text != "" {
				out = append(out, appServerUserInput{Type: "text", Text: block.Text, TextElements: []any{}})
			}
		case protocol.ContentBlockTypeResourceLink:
			input, err := codexappResourceLinkToInput(block)
			if err != nil {
				return nil, err
			}
			out = append(out, input)
		case protocol.ContentBlockTypeImage:
			input, err := codexappImageToInput(projectName, sessionID, block)
			if err != nil {
				return nil, err
			}
			out = append(out, input)
		default:
			return nil, fmt.Errorf("codexapp phase 1 does not support prompt content type %q", block.Type)
		}
	}
	if len(out) == 0 {
		return nil, errors.New("codexapp prompt contains no text")
	}
	return out, nil
}

func codexappImageToInput(projectName string, sessionID string, block protocol.ContentBlock) (appServerUserInput, error) {
	if strings.TrimSpace(block.Data) != "" {
		path, err := codexappWriteImageArtifact(projectName, sessionID, block)
		if err != nil {
			return appServerUserInput{}, err
		}
		return appServerUserInput{Type: "localImage", Path: path}, nil
	}
	uriText := strings.TrimSpace(block.URI)
	if uriText == "" {
		return appServerUserInput{}, errors.New("codexapp image requires data or uri")
	}
	parsed, err := url.Parse(uriText)
	if err != nil || parsed.Scheme == "" {
		return appServerUserInput{}, fmt.Errorf("codexapp image uri is invalid: %q", block.URI)
	}
	switch strings.ToLower(parsed.Scheme) {
	case "file":
		path := codexappFileURIPath(parsed)
		if !filepath.IsAbs(path) {
			return appServerUserInput{}, fmt.Errorf("codexapp image file uri must resolve to an absolute path: %q", block.URI)
		}
		return appServerUserInput{Type: "localImage", Path: path}, nil
	case "http", "https":
		return appServerUserInput{Type: "image", URL: uriText}, nil
	default:
		return appServerUserInput{}, fmt.Errorf("codexapp image uri scheme %q is not supported", parsed.Scheme)
	}
}

func codexappWriteImageArtifact(projectName string, sessionID string, block protocol.ContentBlock) (string, error) {
	encoded := strings.TrimSpace(block.Data)
	if encoded == "" {
		return "", errors.New("codexapp image data is empty")
	}
	if base64.StdEncoding.DecodedLen(len(encoded)) > codexappMaxImageBytes+2 {
		return "", fmt.Errorf("codexapp image exceeds %d bytes", codexappMaxImageBytes)
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("codexapp image data is not valid base64: %w", err)
	}
	if len(data) == 0 {
		return "", errors.New("codexapp image data is empty")
	}
	if len(data) > codexappMaxImageBytes {
		return "", fmt.Errorf("codexapp image exceeds %d bytes", codexappMaxImageBytes)
	}
	mimeType := strings.ToLower(strings.TrimSpace(block.MimeType))
	if mimeType == "" {
		mimeType = strings.ToLower(http.DetectContentType(data))
	}
	ext, ok := codexappImageExtension(mimeType)
	if !ok {
		return "", fmt.Errorf("codexapp image mime type %q is not supported", mimeType)
	}
	dir, err := codexappImageArtifactDir(projectName, sessionID)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, fmt.Sprintf("sha256-%x%s", sum, ext))
	if _, err := os.Stat(path); err == nil {
		return path, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", err
	}
	return path, nil
}

// CleanupSessionArtifacts removes artifacts owned by an agent session.
func CleanupSessionArtifacts(projectName, agentType, sessionID string) error {
	if !strings.EqualFold(strings.TrimSpace(agentType), string(protocol.ACPProviderCodexApp)) {
		return nil
	}
	return cleanupCodexappSessionArtifacts(projectName, sessionID)
}

func cleanupCodexappSessionArtifacts(projectName string, sessionID string) error {
	dir, err := codexappImageArtifactDir(projectName, sessionID)
	if err != nil {
		return err
	}
	return os.RemoveAll(dir)
}

func codexappImageArtifactDir(projectName string, sessionID string) (string, error) {
	projectSegment, err := codexappSafePathSegment(projectName)
	if err != nil {
		return "", fmt.Errorf("codexapp image project name: %w", err)
	}
	sessionSegment, err := codexappSafePathSegment(sessionID)
	if err != nil {
		return "", fmt.Errorf("codexapp image session id: %w", err)
	}
	root, err := codexappArtifactRootPathFunc()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, projectSegment, "images", sessionSegment), nil
}

func codexappImageExtension(mimeType string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(mimeType)) {
	case "image/png":
		return ".png", true
	case "image/jpeg", "image/jpg":
		return ".jpg", true
	case "image/webp":
		return ".webp", true
	case "image/gif":
		return ".gif", true
	default:
		return "", false
	}
}

func codexappSafePathSegment(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", errors.New("value is empty")
	}
	var b strings.Builder
	changed := false
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			b.WriteRune(r)
			continue
		}
		changed = true
		b.WriteByte('_')
	}
	segment := b.String()
	if segment == "" || segment == "." || segment == ".." {
		sum := sha256.Sum256([]byte(value))
		return fmt.Sprintf("artifact-%x", sum[:6]), nil
	}
	if changed {
		sum := sha256.Sum256([]byte(value))
		segment = fmt.Sprintf("%s-%x", segment, sum[:6])
	}
	if strings.Contains(segment, "..") {
		segment = strings.ReplaceAll(segment, "..", "__")
	}
	return segment, nil
}

func codexappResourceLinkToInput(block protocol.ContentBlock) (appServerUserInput, error) {
	uriText := strings.TrimSpace(block.URI)
	if uriText == "" {
		return appServerUserInput{}, errors.New("codexapp resource_link requires uri")
	}
	parsed, err := url.Parse(uriText)
	if err == nil && strings.EqualFold(parsed.Scheme, "file") {
		path := codexappFileURIPath(parsed)
		if strings.TrimSpace(path) == "" {
			return appServerUserInput{}, fmt.Errorf("codexapp resource_link file uri has no path: %q", block.URI)
		}
		return appServerUserInput{Type: "mention", Name: firstNonEmptyString(block.Name, block.Title, path), Path: path}, nil
	}
	return appServerUserInput{Type: "text", Text: codexappResourceLinkText(block), TextElements: []any{}}, nil
}

func codexappFileURIPath(parsed *url.URL) string {
	if parsed == nil {
		return ""
	}
	path := parsed.Path
	if parsed.Host != "" {
		path = "//" + parsed.Host + path
	}
	if len(path) >= 3 && path[0] == '/' && path[2] == ':' {
		path = path[1:]
	}
	return path
}

func codexappResourceLinkText(block protocol.ContentBlock) string {
	parts := []string{"Resource link:"}
	if value := firstNonEmptyString(block.Title, block.Name); value != "" {
		parts = append(parts, value)
	}
	if block.URI != "" {
		parts = append(parts, block.URI)
	}
	if block.MimeType != "" {
		parts = append(parts, "mime="+block.MimeType)
	}
	if block.Description != "" {
		parts = append(parts, block.Description)
	}
	return strings.Join(parts, " ")
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
