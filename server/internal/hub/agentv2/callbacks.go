package agentv2

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/swm8023/wheelmaker/internal/protocol"
)

// Callbacks defines business callback handlers owned by instance users.
type Callbacks interface {
	SessionUpdate(params protocol.SessionUpdateParams)
	SessionRequestPermission(ctx context.Context, params protocol.PermissionRequestParams) (protocol.PermissionResult, error)
	FSRead(params protocol.FSReadTextFileParams) (protocol.FSReadTextFileResult, error)
	FSWrite(params protocol.FSWriteTextFileParams) error
	TerminalCreate(params protocol.TerminalCreateParams) (protocol.TerminalCreateResult, error)
	TerminalOutput(params protocol.TerminalOutputParams) (protocol.TerminalOutputResult, error)
	TerminalWaitForExit(params protocol.TerminalWaitForExitParams) (protocol.TerminalWaitForExitResult, error)
	TerminalKill(params protocol.TerminalKillParams) error
	TerminalRelease(params protocol.TerminalReleaseParams) error
}

// RawCallbackBridge accepts inbound raw ACP requests/notifications.
type RawCallbackBridge interface {
	HandleInbound(ctx context.Context, method string, params json.RawMessage, noResponse bool) (any, error)
}

func dispatchInbound(ctx context.Context, method string, params json.RawMessage, noResponse bool, h Callbacks) (any, error) {
	if noResponse {
		if method == protocol.MethodSessionUpdate && h != nil {
			var p protocol.SessionUpdateParams
			if err := json.Unmarshal(params, &p); err != nil {
				return nil, nil
			}
			h.SessionUpdate(p)
		}
		return nil, nil
	}

	if h == nil {
		if method == protocol.MethodRequestPermission {
			return protocol.PermissionResponse{Outcome: protocol.PermissionResult{Outcome: "cancelled"}}, nil
		}
		return nil, fmt.Errorf("no callback handler for method: %s", method)
	}

	switch method {
	case protocol.MethodRequestPermission:
		var p protocol.PermissionRequestParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, fmt.Errorf("%s: unmarshal: %w", method, err)
		}
		result, err := h.SessionRequestPermission(ctx, p)
		if err != nil {
			return nil, err
		}
		return protocol.PermissionResponse{Outcome: result}, nil
	case protocol.MethodFSRead:
		return callbackCall(method, params, h.FSRead)
	case protocol.MethodFSWrite:
		return callbackCallVoid(method, params, h.FSWrite)
	case protocol.MethodTerminalCreate:
		return callbackCall(method, params, h.TerminalCreate)
	case protocol.MethodTerminalOutput:
		return callbackCall(method, params, h.TerminalOutput)
	case protocol.MethodTerminalWaitExit:
		return callbackCall(method, params, h.TerminalWaitForExit)
	case protocol.MethodTerminalKill:
		return callbackCallVoid(method, params, h.TerminalKill)
	case protocol.MethodTerminalRelease:
		return callbackCallVoid(method, params, h.TerminalRelease)
	default:
		return nil, fmt.Errorf("unsupported method: %s", method)
	}
}

func callbackCall[P any, R any](method string, params json.RawMessage, fn func(P) (R, error)) (any, error) {
	var p P
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("%s: unmarshal: %w", method, err)
	}
	return fn(p)
}

func callbackCallVoid[P any](method string, params json.RawMessage, fn func(P) error) (any, error) {
	var p P
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("%s: unmarshal: %w", method, err)
	}
	return nil, fn(p)
}
