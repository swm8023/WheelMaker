package acp

import "context"

// ClientCallbacks is the interface client.Client must implement.
// Forwarder dispatches inbound agent→client requests and notifications
// to these methods after JSON unmarshal — client code never sees raw JSON.
type ClientCallbacks interface {
	// SessionUpdate is called for each incoming session/update notification.
	// Notifications require no response. The client routes updates to the
	// active prompt channel by matching the session ID.
	SessionUpdate(params SessionUpdateParams)

	// SessionRequestPermission responds to session/request_permission requests.
	SessionRequestPermission(ctx context.Context, params PermissionRequestParams) (PermissionResult, error)

	FSRead(params FSReadTextFileParams) (FSReadTextFileResult, error)
	FSWrite(params FSWriteTextFileParams) error
	TerminalCreate(params TerminalCreateParams) (TerminalCreateResult, error)
	TerminalOutput(params TerminalOutputParams) (TerminalOutputResult, error)
	TerminalWaitForExit(params TerminalWaitForExitParams) (TerminalWaitForExitResult, error)
	TerminalKill(params TerminalKillParams) error
	TerminalRelease(params TerminalReleaseParams) error
}
