// package provider implements the Agent Client Protocol JSON-RPC 2.0 stdio transport.
// This package is an internal detail of the agent layer and is not imported directly
// by the client or adapter layers; they interact through the agent.Session interface.
package provider

import (
	"encoding/json"
	"fmt"
)

const jsonrpcVersion = "2.0"

// Request is a JSON-RPC 2.0 request message.
type Request struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int64  `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

// Response is a JSON-RPC 2.0 response message.
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

// Notification is a JSON-RPC 2.0 notification (no id, no response expected).
type Notification struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// RPCError is a JSON-RPC 2.0 error object.
type RPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *RPCError) Error() string {
	if len(e.Data) > 0 {
		// Try to extract a nested "message" field from data for a friendlier description.
		var d struct {
			Message string `json:"message"`
		}
		if json.Unmarshal(e.Data, &d) == nil && d.Message != "" {
			return d.Message
		}
		return fmt.Sprintf("%s: %s", e.Message, e.Data)
	}
	return e.Message
}

// rawMessage is used internally to detect whether an incoming line is a
// Response (has "id") or a Notification (no "id").
type rawMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int64          `json:"id"`
	Method  string          `json:"method"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// --- ACP-specific param/result types ---

// InitializeParams are sent by the client during the initialize handshake.
type InitializeParams struct {
	ProtocolVersion    int                `json:"protocolVersion"` // B3: integer per spec
	ClientCapabilities ClientCapabilities `json:"clientCapabilities"`
	ClientInfo         *AgentInfo         `json:"clientInfo,omitempty"`
}

// InitializeResult is returned by the agent during the initialize handshake.
type InitializeResult struct {
	ProtocolVersion   json.Number       `json:"protocolVersion"` // may be string or number depending on agent version
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

// Mode is an agent operating mode (e.g. "read-only", "auto", "full-access").
type Mode struct {
	ID          string `json:"id"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
}

// ModeState describes the current and available modes for a session.
type ModeState struct {
	CurrentModeID  string `json:"currentModeId,omitempty"`
	AvailableModes []Mode `json:"availableModes,omitempty"`
}

// Model is an available AI model for a session.
type Model struct {
	ModelID     string `json:"modelId"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
}

// ModelState describes the current and available models for a session.
type ModelState struct {
	CurrentModelID  string  `json:"currentModelId,omitempty"`
	AvailableModels []Model `json:"availableModels,omitempty"`
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
	Modes         *ModeState     `json:"modes,omitempty"`
	Models        *ModelState    `json:"models,omitempty"`
	ConfigOptions []ConfigOption `json:"configOptions,omitempty"`
}

// SessionLoadParams resumes an existing ACP session.
type SessionLoadParams struct {
	SessionID  string      `json:"sessionId"`
	CWD        string      `json:"cwd"`
	MCPServers []MCPServer `json:"mcpServers"`
}

// EnvVariable is a name/value environment variable pair.
// The protocol uses [{"name":"K","value":"V"}] format, not {"K":"V"}.
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
	Type    string        `json:"type,omitempty"` // "stdio" | "http" | "sse"
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
	StopReason string `json:"stopReason"` // "end_turn" | "cancelled" | "max_tokens" | ...
}

// SessionCancelParams cancels an in-progress prompt (sent as a notification).
type SessionCancelParams struct {
	SessionID string `json:"sessionId"`
}

// SessionUpdateParams is the payload of a session/update notification.
type SessionUpdateParams struct {
	SessionID string        `json:"sessionId"`
	Update    SessionUpdate `json:"update"`
}

// SessionUpdate is the body of a single streaming update from the agent.
// All fields beyond SessionUpdate are optional and depend on the update type.
type SessionUpdate struct {
	// SessionUpdate is the update type discriminator.
	SessionUpdate string `json:"sessionUpdate"`

	// agent_message_chunk / agent_thought_chunk: single ContentBlock as raw JSON.
	Content json.RawMessage `json:"content,omitempty"`

	// available_commands_update
	AvailableCommands []AvailableCommand `json:"availableCommands,omitempty"`

	// tool_call / tool_call_update
	ToolCallID string `json:"toolCallId,omitempty"`
	Title      string `json:"title,omitempty"` // also used by session_info_update
	Kind       string `json:"kind,omitempty"`
	Status     string `json:"status,omitempty"`

	// plan
	Entries []PlanEntry `json:"entries,omitempty"`

	// current_mode_update
	ModeID string `json:"modeId,omitempty"`

	// config_option_update
	ConfigOptions []ConfigOption `json:"configOptions,omitempty"`

	// session_info_update
	UpdatedAt string `json:"updatedAt,omitempty"`
}

// PlanEntry is a single step in an agent execution plan.
type PlanEntry struct {
	Content  string `json:"content"`
	Priority string `json:"priority"` // "high" | "medium" | "low"
	Status   string `json:"status"`   // "pending" | "in_progress" | "completed"
}

// ContentBlock is a piece of content within a session update or prompt.
type ContentBlock struct {
	Type     string `json:"type"` // "text" | "image" | "audio" | "resource_link" | "resource"
	Text     string `json:"text,omitempty"`
	MimeType string `json:"mimeType,omitempty"`
	Data     string `json:"data,omitempty"`
	URI      string `json:"uri,omitempty"`
	Name     string `json:"name,omitempty"`
}

// ToolCallRef is a reference to a tool call, used in permission requests.
type ToolCallRef struct {
	ToolCallID string `json:"toolCallId"`
}

// PermissionRequestParams is sent by the agent when it needs permission to use a tool.
type PermissionRequestParams struct {
	SessionID string             `json:"sessionId"`
	ToolCall  ToolCallRef        `json:"toolCall"`
	Options   []PermissionOption `json:"options"`
}

// PermissionOption is a choice the client can make for a permission request.
// B1 fix: field names are optionId/name per ACP spec (was id/label).
type PermissionOption struct {
	OptionID string `json:"optionId"`
	Name     string `json:"name"`
	Kind     string `json:"kind"` // "allow_once" | "allow_always" | "reject_once" | "reject_always"
}

// PermissionResult is the inner permission outcome.
type PermissionResult struct {
	Outcome  string `json:"outcome"`            // "selected" | "cancelled"
	OptionID string `json:"optionId,omitempty"` // set when Outcome == "selected"
}

// PermissionResponse is the JSON-RPC result for session/request_permission.
// B2 fix: protocol requires wrapping: {"outcome": {"outcome":"selected","optionId":"..."}}
type PermissionResponse struct {
	Outcome PermissionResult `json:"outcome"`
}

// SessionLoadResult is the response to session/load.
// Note: unlike session/new, session/load does NOT return sessionId in the response.
type SessionLoadResult struct {
}

// SessionSetConfigOptionParams sets a configuration option on an active session.
type SessionSetConfigOptionParams struct {
	SessionID string `json:"sessionId"`
	ConfigID  string `json:"configId"`
	Value     string `json:"value"`
}

// --- AgentÃ¢â€ â€™Client callback types ---

// FSReadTextFileParams is sent by the agent to request a file read.
type FSReadTextFileParams struct {
	SessionID string `json:"sessionId"`
	Path      string `json:"path"`
	Line      *int   `json:"line,omitempty"`  // 1-based start line (optional)
	Limit     *int   `json:"limit,omitempty"` // max lines to return (optional)
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
// The protocol uses {"exitCode": N, "signal": null}, not a plain integer.
type TerminalExitStatus struct {
	ExitCode *int    `json:"exitCode,omitempty"`
	Signal   *string `json:"signal,omitempty"`
}

// TerminalOutputResult returns the accumulated output and exit status if done.
type TerminalOutputResult struct {
	Output     string              `json:"output"`
	Truncated  bool                `json:"truncated"`
	ExitStatus *TerminalExitStatus `json:"exitStatus,omitempty"` // nil if still running
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
