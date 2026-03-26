package acp

import "encoding/json"

// --- ACP method names ---

// Client → Agent methods.
const (
	MethodInitialize      = "initialize"
	MethodSessionNew      = "session/new"
	MethodSessionPrompt   = "session/prompt"
	MethodSessionCancel   = "session/cancel" // notification
	MethodSessionLoad     = "session/load"
	MethodSessionList     = "session/list"
	MethodSetConfigOption = "session/set_config_option"
)

// Agent → Client methods.
const (
	MethodRequestPermission = "session/request_permission"
	MethodFSRead            = "fs/read_text_file"
	MethodFSWrite           = "fs/write_text_file"
	MethodTerminalCreate    = "terminal/create"
	MethodTerminalOutput    = "terminal/output"
	MethodTerminalWaitExit  = "terminal/wait_for_exit"
	MethodTerminalKill      = "terminal/kill"
	MethodTerminalRelease   = "terminal/release"
	MethodSessionUpdate     = "session/update" // notification
)

// --- ACP-specific param/result types ---

// InitializeParams are sent by the client during the initialize handshake.
type InitializeParams struct {
	ProtocolVersion    int                `json:"protocolVersion"`
	ClientCapabilities ClientCapabilities `json:"clientCapabilities"`
	ClientInfo         *AgentInfo         `json:"clientInfo,omitempty"`
}

// InitializeResult is returned by the agent during the initialize handshake.
type InitializeResult struct {
	ProtocolVersion   json.Number       `json:"protocolVersion"`
	AgentCapabilities AgentCapabilities `json:"agentCapabilities"`
	AgentInfo         *AgentInfo        `json:"agentInfo,omitempty"`
	AuthMethods       []AuthMethod      `json:"authMethods,omitempty"`
}

// ClientCapabilities declares which client-side callbacks the client supports.
type ClientCapabilities struct {
	FS       *FSCapabilities `json:"fs,omitempty"`
	Terminal bool            `json:"terminal,omitempty"`
}

// FSCapabilities declares file system callback support.
type FSCapabilities struct {
	ReadTextFile  bool `json:"readTextFile,omitempty"`
	WriteTextFile bool `json:"writeTextFile,omitempty"`
}

// AgentCapabilities declares what the agent supports.
type AgentCapabilities struct {
	LoadSession         bool                 `json:"loadSession,omitempty"`
	PromptCapabilities  *PromptCapabilities  `json:"promptCapabilities,omitempty"`
	MCPCapabilities     *MCPCapabilities     `json:"mcp,omitempty"`
	SessionCapabilities *SessionCapabilities `json:"sessionCapabilities,omitempty"`
}

// PromptCapabilities declares which content block types the agent accepts in prompts.
type PromptCapabilities struct {
	Image           bool `json:"image,omitempty"`
	Audio           bool `json:"audio,omitempty"`
	EmbeddedContext bool `json:"embeddedContext,omitempty"`
}

// MCPCapabilities declares which MCP transport types the agent supports.
type MCPCapabilities struct {
	HTTP bool `json:"http,omitempty"`
	SSE  bool `json:"sse,omitempty"`
}

// SessionCapabilities declares optional session-level capabilities.
type SessionCapabilities struct {
	List *SessionListCapability `json:"list,omitempty"`
}

// SessionListCapability is an opaque marker indicating session/list is supported.
type SessionListCapability struct{}

// AgentInfo identifies the client or agent.
type AgentInfo struct {
	Name    string `json:"name"`
	Title   string `json:"title,omitempty"`
	Version string `json:"version,omitempty"`
}

// AuthMethodVar is an environment variable required by an auth method.
type AuthMethodVar struct {
	Name string `json:"name"`
}

// AuthMethod is an authentication option offered by the agent during initialize.
type AuthMethod struct {
	ID          string          `json:"id"`
	Type        string          `json:"type,omitempty"`
	Name        string          `json:"name,omitempty"`
	Description string          `json:"description,omitempty"`
	Vars        []AuthMethodVar `json:"vars,omitempty"`
}

// ConfigOptionValue is a selectable value for a config option.
type ConfigOptionValue struct {
	Value       string `json:"value"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
}

// ConfigOption is a configurable session parameter (e.g. mode, model, reasoning effort).
type ConfigOption struct {
	ID           string              `json:"id"`
	Name         string              `json:"name,omitempty"`
	Description  string              `json:"description,omitempty"`
	Category     string              `json:"category,omitempty"`
	Type         string              `json:"type,omitempty"`
	CurrentValue string              `json:"currentValue,omitempty"`
	Options      []ConfigOptionValue `json:"options,omitempty"`
}

// AvailableCommand is a slash command advertised by the agent via session/update.
type AvailableCommand struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Input       json.RawMessage `json:"input,omitempty"`
}

// SessionNewParams creates a new ACP session.
type SessionNewParams struct {
	CWD        string      `json:"cwd"`
	MCPServers []MCPServer `json:"mcpServers"`
}

// SessionNewResult is returned after a successful session/new.
type SessionNewResult struct {
	SessionID     string         `json:"sessionId"`
	ConfigOptions []ConfigOption `json:"configOptions,omitempty"`
}

// SessionLoadParams resumes an existing ACP session.
type SessionLoadParams struct {
	SessionID  string      `json:"sessionId"`
	CWD        string      `json:"cwd"`
	MCPServers []MCPServer `json:"mcpServers"`
}

// EnvVariable is a name/value environment variable pair.
type EnvVariable struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// HttpHeader is a name/value HTTP header pair (used by HTTP/SSE MCP transports).
type HttpHeader struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// MCPServer represents a Model Context Protocol server configuration.
type MCPServer struct {
	Type    string        `json:"type,omitempty"`
	Name    string        `json:"name"`
	Command string        `json:"command,omitempty"`
	Args    []string      `json:"args,omitempty"`
	Env     []EnvVariable `json:"env,omitempty"`
	URL     string        `json:"url,omitempty"`
	Headers []HttpHeader  `json:"headers,omitempty"`
}

// SessionPromptParams sends a prompt to the agent.
type SessionPromptParams struct {
	SessionID string         `json:"sessionId"`
	Prompt    []ContentBlock `json:"prompt"`
}

// SessionPromptResult is the final result after a prompt completes.
type SessionPromptResult struct {
	StopReason string `json:"stopReason"`
}

// SessionCancelParams cancels an in-progress prompt.
type SessionCancelParams struct {
	SessionID string `json:"sessionId"`
}

// SessionUpdateParams is the payload of a session/update notification.
type SessionUpdateParams struct {
	SessionID string        `json:"sessionId"`
	Update    SessionUpdate `json:"update"`
}

// SessionUpdate is the body of a single streaming update from the agent.
type SessionUpdate struct {
	SessionUpdate     string             `json:"sessionUpdate"`
	Content           json.RawMessage    `json:"content,omitempty"`
	AvailableCommands []AvailableCommand `json:"availableCommands,omitempty"`
	ToolCallID        string             `json:"toolCallId,omitempty"`
	Title             string             `json:"title,omitempty"`
	Kind              string             `json:"kind,omitempty"`
	Status            string             `json:"status,omitempty"`
	Entries           []PlanEntry        `json:"entries,omitempty"`
	Locations         []ToolCallLocation `json:"locations,omitempty"`
	RawInput          json.RawMessage    `json:"rawInput,omitempty"`
	RawOutput         json.RawMessage    `json:"rawOutput,omitempty"`
	ToolCallContent   []ToolCallContent  `json:"toolCallContent,omitempty"`
	// ModeID is legacy Session Modes payload ("current_mode_update").
	// Prefer ConfigOptions via "config_option_update"; this field is retained
	// only for backward-compatible input parsing.
	ModeID        string         `json:"modeId,omitempty"`
	ConfigOptions []ConfigOption `json:"configOptions,omitempty"`
	UpdatedAt     string         `json:"updatedAt,omitempty"`
}

// PlanEntry is a single step in an agent execution plan.
type PlanEntry struct {
	Content  string `json:"content"`
	Priority string `json:"priority"`
	Status   string `json:"status"`
}

// ToolCallLocation is a file location affected by a tool call.
type ToolCallLocation struct {
	Path string `json:"path"`
	Line *int   `json:"line,omitempty"`
}

// ToolCallContent is a content entry within a tool call (§7.3).
// Type is one of "content", "diff", "terminal".
type ToolCallContent struct {
	Type       string        `json:"type"`
	Content    *ContentBlock `json:"content,omitempty"`    // type="content"
	Path       string        `json:"path,omitempty"`       // type="diff"
	OldText    *string       `json:"oldText,omitempty"`    // type="diff" (null = new file)
	NewText    string        `json:"newText,omitempty"`    // type="diff"
	TerminalID string        `json:"terminalId,omitempty"` // type="terminal"
}

// EmbeddedResource is the nested payload for ContentBlock type "resource".
// Text resources carry Text; blob resources carry Blob (base64-encoded).
type EmbeddedResource struct {
	URI      string `json:"uri"`
	MimeType string `json:"mimeType,omitempty"`
	Text     string `json:"text,omitempty"`
	Blob     string `json:"blob,omitempty"`
}

// ContentBlock is a piece of content within a session update or prompt.
//
// Type-to-field mapping:
//   - "text"          → Text
//   - "image"         → Data (base64), MimeType
//   - "audio"         → Data (base64), MimeType
//   - "resource"      → Resource (embedded resource with inline text or blob)
//   - "resource_link" → URI, Name, MimeType, Size, Title, Description
type ContentBlock struct {
	Type        string            `json:"type"`
	Text        string            `json:"text,omitempty"`
	MimeType    string            `json:"mimeType,omitempty"`
	Data        string            `json:"data,omitempty"`
	URI         string            `json:"uri,omitempty"`
	Name        string            `json:"name,omitempty"`
	Title       string            `json:"title,omitempty"`
	Description string            `json:"description,omitempty"`
	Size        int               `json:"size,omitempty"`
	Resource    *EmbeddedResource `json:"resource,omitempty"`
	Annotations json.RawMessage   `json:"annotations,omitempty"`
}

// ToolCallRef is the tool call object sent with permission requests.
// Per ACP §5.5, includes identifying fields beyond the ID so the client
// can render a meaningful permission dialog.
type ToolCallRef struct {
	ToolCallID string `json:"toolCallId"`
	Title      string `json:"title,omitempty"`
	Status     string `json:"status,omitempty"`
	Kind       string `json:"kind,omitempty"`
}

// PermissionRequestParams is sent by the agent when it needs permission to use a tool.
type PermissionRequestParams struct {
	SessionID string             `json:"sessionId"`
	ToolCall  ToolCallRef        `json:"toolCall"`
	Options   []PermissionOption `json:"options"`
}

// PermissionOption is a choice the client can make for a permission request.
type PermissionOption struct {
	OptionID string `json:"optionId"`
	Name     string `json:"name"`
	Kind     string `json:"kind"`
}

// PermissionResult is the inner permission outcome.
type PermissionResult struct {
	Outcome  string `json:"outcome"`
	OptionID string `json:"optionId,omitempty"`
}

// PermissionResponse is the JSON-RPC result for session/request_permission.
type PermissionResponse struct {
	Outcome PermissionResult `json:"outcome"`
}

// SessionLoadResult is the response to session/load.
type SessionLoadResult struct {
	ConfigOptions []ConfigOption `json:"configOptions,omitempty"`
}

// SessionSetConfigOptionParams sets a configuration option on an active session.
type SessionSetConfigOptionParams struct {
	SessionID string `json:"sessionId"`
	ConfigID  string `json:"configId"`
	Value     string `json:"value"`
}

// FSReadTextFileParams is sent by the agent to request a file read.
type FSReadTextFileParams struct {
	SessionID string `json:"sessionId"`
	Path      string `json:"path"`
	Line      *int   `json:"line,omitempty"`
	Limit     *int   `json:"limit,omitempty"`
}

// FSReadTextFileResult is the client's response with the file content.
type FSReadTextFileResult struct {
	Content string `json:"content"`
}

// FSWriteTextFileParams is sent by the agent to request a file write.
type FSWriteTextFileParams struct {
	SessionID string `json:"sessionId"`
	Path      string `json:"path"`
	Content   string `json:"content"`
}

// TerminalCreateParams is sent by the agent to spawn a terminal process.
type TerminalCreateParams struct {
	SessionID       string        `json:"sessionId"`
	Command         string        `json:"command"`
	Args            []string      `json:"args,omitempty"`
	CWD             string        `json:"cwd,omitempty"`
	Env             []EnvVariable `json:"env,omitempty"`
	OutputByteLimit *int          `json:"outputByteLimit,omitempty"`
}

// TerminalCreateResult is the client's response with the new terminal ID.
type TerminalCreateResult struct {
	TerminalID string `json:"terminalId"`
}

// TerminalOutputParams requests buffered output from a terminal.
type TerminalOutputParams struct {
	SessionID  string `json:"sessionId"`
	TerminalID string `json:"terminalId"`
}

// TerminalExitStatus is the exit status object returned in TerminalOutputResult.
type TerminalExitStatus struct {
	ExitCode *int    `json:"exitCode,omitempty"`
	Signal   *string `json:"signal,omitempty"`
}

// TerminalOutputResult returns the accumulated output and exit status if done.
type TerminalOutputResult struct {
	Output     string              `json:"output"`
	Truncated  bool                `json:"truncated"`
	ExitStatus *TerminalExitStatus `json:"exitStatus,omitempty"`
}

// TerminalWaitForExitParams blocks until the terminal process exits.
type TerminalWaitForExitParams struct {
	SessionID  string `json:"sessionId"`
	TerminalID string `json:"terminalId"`
}

// TerminalWaitForExitResult contains the process exit information.
type TerminalWaitForExitResult struct {
	ExitCode *int    `json:"exitCode,omitempty"`
	Signal   *string `json:"signal,omitempty"`
}

// TerminalKillParams requests termination of a terminal process.
type TerminalKillParams struct {
	SessionID  string `json:"sessionId"`
	TerminalID string `json:"terminalId"`
}

// TerminalReleaseParams releases terminal resources.
type TerminalReleaseParams struct {
	SessionID  string `json:"sessionId"`
	TerminalID string `json:"terminalId"`
}

// SessionListParams requests a paginated list of sessions.
type SessionListParams struct {
	CWD    string `json:"cwd,omitempty"`
	Cursor string `json:"cursor,omitempty"`
}

// SessionInfo is a single entry in a session list.
type SessionInfo struct {
	SessionID string `json:"sessionId"`
	CWD       string `json:"cwd"`
	Title     string `json:"title,omitempty"`
	UpdatedAt string `json:"updatedAt,omitempty"`
}

// SessionListResult is the response to session/list.
type SessionListResult struct {
	Sessions   []SessionInfo `json:"sessions"`
	NextCursor string        `json:"nextCursor,omitempty"`
}

// --- Session config helpers ---

// SessionConfigSnapshot is a compact view of session-level mode/model values.
type SessionConfigSnapshot struct {
	Mode  string
	Model string
}

// SessionConfigSnapshotFromOptions extracts mode/model from a ConfigOption list.
func SessionConfigSnapshotFromOptions(opts []ConfigOption) SessionConfigSnapshot {
	snap := SessionConfigSnapshot{}
	for _, opt := range opts {
		if snap.Mode == "" && (opt.ID == "mode" || opt.Category == "mode") {
			snap.Mode = opt.CurrentValue
		}
		if snap.Model == "" && (opt.ID == "model" || opt.Category == "model") {
			snap.Model = opt.CurrentValue
		}
	}
	return snap
}
