package provider

import (
	"encoding/json"

	acp "github.com/swm8023/wheelmaker/internal/agent/acp"
)

const jsonrpcVersion = "2.0"

// rawMessage is used internally by Conn to route JSON-RPC frames.
type rawMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int64          `json:"id"`
	Method  string          `json:"method"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Protocol type aliases from internal/agent/acp.
type Request = acp.Request
type Response = acp.Response
type Notification = acp.Notification
type RPCError = acp.RPCError

type InitializeParams = acp.InitializeParams
type InitializeResult = acp.InitializeResult
type ClientCapabilities = acp.ClientCapabilities
type FSCapabilities = acp.FSCapabilities
type AgentCapabilities = acp.AgentCapabilities
type PromptCapabilities = acp.PromptCapabilities
type MCPCapabilities = acp.MCPCapabilities
type SessionCapabilities = acp.SessionCapabilities
type SessionListCapability = acp.SessionListCapability
type AgentInfo = acp.AgentInfo
type AuthMethodVar = acp.AuthMethodVar
type AuthMethod = acp.AuthMethod
type Mode = acp.Mode
type ModeState = acp.ModeState
type Model = acp.Model
type ModelState = acp.ModelState
type ConfigOptionValue = acp.ConfigOptionValue
type ConfigOption = acp.ConfigOption
type AvailableCommand = acp.AvailableCommand
type SessionNewParams = acp.SessionNewParams
type SessionNewResult = acp.SessionNewResult
type SessionLoadParams = acp.SessionLoadParams
type EnvVariable = acp.EnvVariable
type HttpHeader = acp.HttpHeader
type MCPServer = acp.MCPServer
type SessionPromptParams = acp.SessionPromptParams
type SessionPromptResult = acp.SessionPromptResult
type SessionCancelParams = acp.SessionCancelParams
type SessionUpdateParams = acp.SessionUpdateParams
type SessionUpdate = acp.SessionUpdate
type PlanEntry = acp.PlanEntry
type ContentBlock = acp.ContentBlock
type ToolCallRef = acp.ToolCallRef
type PermissionRequestParams = acp.PermissionRequestParams
type PermissionOption = acp.PermissionOption
type PermissionResult = acp.PermissionResult
type PermissionResponse = acp.PermissionResponse
type SessionLoadResult = acp.SessionLoadResult
type SessionSetConfigOptionParams = acp.SessionSetConfigOptionParams

type FSReadTextFileParams = acp.FSReadTextFileParams
type FSReadTextFileResult = acp.FSReadTextFileResult
type FSWriteTextFileParams = acp.FSWriteTextFileParams
type TerminalCreateParams = acp.TerminalCreateParams
type TerminalCreateResult = acp.TerminalCreateResult
type TerminalOutputParams = acp.TerminalOutputParams
type TerminalExitStatus = acp.TerminalExitStatus
type TerminalOutputResult = acp.TerminalOutputResult
type TerminalWaitForExitParams = acp.TerminalWaitForExitParams
type TerminalWaitForExitResult = acp.TerminalWaitForExitResult
type TerminalKillParams = acp.TerminalKillParams
type TerminalReleaseParams = acp.TerminalReleaseParams
