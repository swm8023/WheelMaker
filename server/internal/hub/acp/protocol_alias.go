package acp

import p "github.com/swm8023/wheelmaker/internal/protocol"

const (
	MethodInitialize        = p.MethodInitialize
	MethodSessionNew        = p.MethodSessionNew
	MethodSessionPrompt     = p.MethodSessionPrompt
	MethodSessionCancel     = p.MethodSessionCancel
	MethodSessionLoad       = p.MethodSessionLoad
	MethodSessionList       = p.MethodSessionList
	MethodSetConfigOption   = p.MethodSetConfigOption
	MethodRequestPermission = p.MethodRequestPermission
	MethodFSRead            = p.MethodFSRead
	MethodFSWrite           = p.MethodFSWrite
	MethodTerminalCreate    = p.MethodTerminalCreate
	MethodTerminalOutput    = p.MethodTerminalOutput
	MethodTerminalWaitExit  = p.MethodTerminalWaitExit
	MethodTerminalKill      = p.MethodTerminalKill
	MethodTerminalRelease   = p.MethodTerminalRelease
	MethodSessionUpdate     = p.MethodSessionUpdate
)

type (
	InitializeParams             = p.InitializeParams
	InitializeResult             = p.InitializeResult
	ClientCapabilities           = p.ClientCapabilities
	FSCapabilities               = p.FSCapabilities
	AgentCapabilities            = p.AgentCapabilities
	PromptCapabilities           = p.PromptCapabilities
	MCPCapabilities              = p.MCPCapabilities
	SessionCapabilities          = p.SessionCapabilities
	SessionListCapability        = p.SessionListCapability
	AgentInfo                    = p.AgentInfo
	AuthMethodVar                = p.AuthMethodVar
	AuthMethod                   = p.AuthMethod
	ConfigOptionValue            = p.ConfigOptionValue
	ConfigOption                 = p.ConfigOption
	AvailableCommand             = p.AvailableCommand
	SessionNewParams             = p.SessionNewParams
	SessionNewResult             = p.SessionNewResult
	SessionLoadParams            = p.SessionLoadParams
	EnvVariable                  = p.EnvVariable
	HttpHeader                   = p.HttpHeader
	MCPServer                    = p.MCPServer
	SessionPromptParams          = p.SessionPromptParams
	SessionPromptResult          = p.SessionPromptResult
	SessionCancelParams          = p.SessionCancelParams
	SessionUpdateParams          = p.SessionUpdateParams
	SessionUpdate                = p.SessionUpdate
	PlanEntry                    = p.PlanEntry
	ToolCallLocation             = p.ToolCallLocation
	ToolCallContent              = p.ToolCallContent
	EmbeddedResource             = p.EmbeddedResource
	ContentBlock                 = p.ContentBlock
	ToolCallRef                  = p.ToolCallRef
	PermissionRequestParams      = p.PermissionRequestParams
	PermissionOption             = p.PermissionOption
	PermissionResult             = p.PermissionResult
	PermissionResponse           = p.PermissionResponse
	SessionLoadResult            = p.SessionLoadResult
	SessionSetConfigOptionParams = p.SessionSetConfigOptionParams
	FSReadTextFileParams         = p.FSReadTextFileParams
	FSReadTextFileResult         = p.FSReadTextFileResult
	FSWriteTextFileParams        = p.FSWriteTextFileParams
	TerminalCreateParams         = p.TerminalCreateParams
	TerminalCreateResult         = p.TerminalCreateResult
	TerminalOutputParams         = p.TerminalOutputParams
	TerminalExitStatus           = p.TerminalExitStatus
	TerminalOutputResult         = p.TerminalOutputResult
	TerminalWaitForExitParams    = p.TerminalWaitForExitParams
	TerminalWaitForExitResult    = p.TerminalWaitForExitResult
	TerminalKillParams           = p.TerminalKillParams
	TerminalReleaseParams        = p.TerminalReleaseParams
	SessionListParams            = p.SessionListParams
	SessionInfo                  = p.SessionInfo
	SessionListResult            = p.SessionListResult
	SessionConfigSnapshot        = p.SessionConfigSnapshot
)

var SessionConfigSnapshotFromOptions = p.SessionConfigSnapshotFromOptions
