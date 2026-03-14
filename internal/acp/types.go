// Package acp implements the Agent Client Protocol JSON-RPC 2.0 stdio transport.
// Reference: https://agentclientprotocol.com/protocol/transports
package acp

import "encoding/json"

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
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *RPCError) Error() string {
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
	ProtocolVersion    string             `json:"protocolVersion"`
	ClientCapabilities ClientCapabilities `json:"clientCapabilities"`
	ClientInfo         *AgentInfo         `json:"clientInfo,omitempty"`
}

// InitializeResult is returned by the agent during the initialize handshake.
type InitializeResult struct {
	ProtocolVersion    string             `json:"protocolVersion"`
	AgentCapabilities  AgentCapabilities  `json:"agentCapabilities"`
	AgentInfo          *AgentInfo         `json:"agentInfo,omitempty"`
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
	LoadSession bool `json:"loadSession,omitempty"`
}

// AgentInfo identifies the client or agent.
type AgentInfo struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

// SessionNewParams creates a new ACP session.
type SessionNewParams struct {
	CWD        string      `json:"cwd"`
	MCPServers []MCPServer `json:"mcpServers"`
}

// SessionNewResult is returned after a successful session/new.
type SessionNewResult struct {
	SessionID string `json:"sessionId"`
}

// SessionLoadParams resumes an existing ACP session.
type SessionLoadParams struct {
	SessionID  string      `json:"sessionId"`
	CWD        string      `json:"cwd"`
	MCPServers []MCPServer `json:"mcpServers"`
}

// MCPServer represents a Model Context Protocol server configuration.
type MCPServer struct {
	Type    string            `json:"type,omitempty"` // "stdio" | "http" | "sse"
	Name    string            `json:"name"`
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	URL     string            `json:"url,omitempty"`
}

// SessionPromptParams sends a prompt to the agent.
type SessionPromptParams struct {
	SessionID string `json:"sessionId"`
	Prompt    string `json:"prompt"`
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
type SessionUpdate struct {
	// SessionUpdate is the update type discriminator.
	SessionUpdate string `json:"sessionUpdate"`
	// Content is present for message/thought chunk updates.
	Content *ContentBlock `json:"content,omitempty"`
}

// ContentBlock is a piece of content within a session update.
type ContentBlock struct {
	Type string `json:"type"` // "text" | "image" | "audio" | "resource_link" | "resource"
	Text string `json:"text,omitempty"`
}

// PermissionRequestParams is sent by the agent when it needs permission to use a tool.
type PermissionRequestParams struct {
	SessionID string             `json:"sessionId"`
	ToolCall  ToolCall           `json:"toolCall"`
	Options   []PermissionOption `json:"options"`
}

// ToolCall describes a tool the agent wants to invoke.
type ToolCall struct {
	Name string          `json:"name"`
	Args json.RawMessage `json:"args,omitempty"`
}

// PermissionOption is a choice the client can make for a permission request.
type PermissionOption struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Kind  string `json:"kind"` // "allow_once" | "allow_always" | "reject_once" | "reject_always"
}

// PermissionResult is the client's response to a permission request.
type PermissionResult struct {
	Outcome  string `json:"outcome"`           // "selected" | "cancelled"
	OptionID string `json:"optionId,omitempty"` // set when Outcome == "selected"
}

// SessionLoadResult is the response to session/load.
// Note: unlike session/new, session/load does NOT return sessionId in the response.
type SessionLoadResult struct {
	// Modes and ConfigOptions are optional; WheelMaker ignores them in MVP.
}

// --- Agent→Client callback types (§5.2 of ACP protocol) ---

// FSReadTextFileParams is sent by the agent to request a file read.
type FSReadTextFileParams struct {
	SessionID string `json:"sessionId"`
	Path      string `json:"path"`
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
	SessionID       string            `json:"sessionId"`
	Command         string            `json:"command"`
	Args            []string          `json:"args,omitempty"`
	CWD             string            `json:"cwd,omitempty"`
	Env             map[string]string `json:"env,omitempty"`
	OutputByteLimit *int              `json:"outputByteLimit,omitempty"`
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

// TerminalOutputResult returns the accumulated output and exit status if done.
type TerminalOutputResult struct {
	Output     string `json:"output"`
	Truncated  bool   `json:"truncated"`
	ExitStatus *int   `json:"exitStatus,omitempty"` // nil if still running
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
