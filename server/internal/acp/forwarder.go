package acp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
)

// MessageDirection indicates message flow across ACP forwarder.
type MessageDirection string

const (
	// DirectionToAgent means client -> agent.
	DirectionToAgent MessageDirection = "to_agent"
	// DirectionToClient means agent -> client.
	DirectionToClient MessageDirection = "to_client"
)

// MessageKind identifies ACP message category.
type MessageKind string

const (
	// KindRequest is a JSON-RPC request.
	KindRequest MessageKind = "request"
	// KindNotification is a JSON-RPC notification.
	KindNotification MessageKind = "notification"
)

// ForwardMessage is passed through prefilters before crossing layers.
type ForwardMessage struct {
	Direction MessageDirection
	Kind      MessageKind
	Method    string
	Params    json.RawMessage
}

// Prefilter can mutate, allow, or block ACP messages.
// Returning allow=false drops the message.
type Prefilter func(ctx context.Context, msg ForwardMessage) (filtered ForwardMessage, allow bool, err error)

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

// Forwarder wraps Conn and applies a bidirectional prefilter.
type Forwarder struct {
	conn      *Conn
	prefilter Prefilter
}

// NewForwarder creates a forwarder over a connection.
func NewForwarder(conn *Conn, prefilter Prefilter) *Forwarder {
	return &Forwarder{conn: conn, prefilter: prefilter}
}

func (f *Forwarder) filter(ctx context.Context, msg ForwardMessage) (ForwardMessage, bool, error) {
	if f.prefilter == nil {
		return msg, true, nil
	}
	return f.prefilter(ctx, msg)
}

// SendAgent forwards a request to agent after filtering.
func (f *Forwarder) SendAgent(ctx context.Context, method string, params any, result any) error {
	raw, err := marshalParams(params)
	if err != nil {
		return err
	}
	msg, allow, err := f.filter(ctx, ForwardMessage{
		Direction: DirectionToAgent,
		Kind:      KindRequest,
		Method:    method,
		Params:    raw,
	})
	if err != nil {
		return err
	}
	if !allow {
		return fmt.Errorf("acp forwarder: request blocked: %s", method)
	}
	return f.conn.SendAgent(ctx, msg.Method, msg.Params, result)
}

// NotifyAgent forwards a notification to the agent after filtering.
func (f *Forwarder) NotifyAgent(method string, params any) error {
	raw, err := marshalParams(params)
	if err != nil {
		return err
	}
	msg, allow, err := f.filter(context.Background(), ForwardMessage{
		Direction: DirectionToAgent,
		Kind:      KindNotification,
		Method:    method,
		Params:    raw,
	})
	if err != nil {
		return err
	}
	if !allow {
		return nil
	}
	return f.conn.NotifyAgent(msg.Method, msg.Params)
}

// Close closes the underlying Conn.
func (f *Forwarder) Close() error { return f.conn.Close() }

// SetDebugLogger forwards raw ACP JSON logs from the underlying connection.
func (f *Forwarder) SetDebugLogger(w io.Writer) {
	f.conn.SetDebugLogger(w)
}

// SetCallbacks registers h as the handler for all agent->client requests and
// session/update notifications. It wires a single conn.OnRequest handler that
// routes both inbound requests (noResponse=false) and notifications
// (noResponse=true) to the appropriate ClientCallbacks method.
//
// SetCallbacks must be called before the first prompt; it is not safe to call
// concurrently with active requests.
func (f *Forwarder) SetCallbacks(h ClientCallbacks) {
	f.conn.OnRequest(func(ctx context.Context, method string, params json.RawMessage, noResponse bool) (any, error) {
		return dispatchAgentToClientMessage(ctx, method, params, noResponse, h)
	})
}

// dispatchAgentToClientMessage routes an inbound agent→client message to the typed
// ClientCallbacks method. noResponse is true for notifications (session/update);
// in that case all return values are discarded by the caller.
func dispatchAgentToClientMessage(ctx context.Context, method string, params json.RawMessage, noResponse bool, h ClientCallbacks) (any, error) {
	if noResponse {
		if method == MethodSessionUpdate {
			params = NormalizeNotificationParams(method, params)
			var p SessionUpdateParams
			if err := json.Unmarshal(params, &p); err != nil {
				return nil, nil
			}
			h.SessionUpdate(p)
		}
		return nil, nil
	}
	switch method {
	case MethodRequestPermission:
		var p PermissionRequestParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, fmt.Errorf("%s: unmarshal: %w", method, err)
		}
		result, err := h.SessionRequestPermission(ctx, p)
		if err != nil {
			return nil, err
		}
		return PermissionResponse{Outcome: result}, nil

	case MethodFSRead:
		var p FSReadTextFileParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, fmt.Errorf("%s: unmarshal: %w", method, err)
		}
		return h.FSRead(p)

	case MethodFSWrite:
		var p FSWriteTextFileParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, fmt.Errorf("%s: unmarshal: %w", method, err)
		}
		return nil, h.FSWrite(p)

	case MethodTerminalCreate:
		var p TerminalCreateParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, fmt.Errorf("%s: unmarshal: %w", method, err)
		}
		return h.TerminalCreate(p)

	case MethodTerminalOutput:
		var p TerminalOutputParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, fmt.Errorf("%s: unmarshal: %w", method, err)
		}
		return h.TerminalOutput(p)

	case MethodTerminalWaitExit:
		var p TerminalWaitForExitParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, fmt.Errorf("%s: unmarshal: %w", method, err)
		}
		return h.TerminalWaitForExit(p)

	case MethodTerminalKill:
		var p TerminalKillParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, fmt.Errorf("%s: unmarshal: %w", method, err)
		}
		return nil, h.TerminalKill(p)

	case MethodTerminalRelease:
		var p TerminalReleaseParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, fmt.Errorf("%s: unmarshal: %w", method, err)
		}
		return nil, h.TerminalRelease(p)

	default:
		return nil, fmt.Errorf("unsupported method: %s", method)
	}
}

// Initialize sends the ACP initialize handshake (client->agent).
// Per §3, this must complete before any session/* method.
func (f *Forwarder) Initialize(ctx context.Context, params InitializeParams) (InitializeResult, error) {
	var result InitializeResult
	if err := f.conn.SendAgent(ctx, MethodInitialize, params, &result); err != nil {
		return InitializeResult{}, err
	}
	return result, nil
}

// SessionNew creates a new ACP session (client->agent).
func (f *Forwarder) SessionNew(ctx context.Context, params SessionNewParams) (SessionNewResult, error) {
	var result SessionNewResult
	if err := f.SendAgent(ctx, MethodSessionNew, params, &result); err != nil {
		return SessionNewResult{}, err
	}
	return result, nil
}

// SessionLoad resumes an existing ACP session (client->agent).
func (f *Forwarder) SessionLoad(ctx context.Context, params SessionLoadParams) (SessionLoadResult, error) {
	var result SessionLoadResult
	if err := f.SendAgent(ctx, MethodSessionLoad, params, &result); err != nil {
		return SessionLoadResult{}, err
	}
	return result, nil
}

// SessionList returns a paginated list of available sessions (client->agent).
func (f *Forwarder) SessionList(ctx context.Context, params SessionListParams) (SessionListResult, error) {
	var result SessionListResult
	if err := f.SendAgent(ctx, MethodSessionList, params, &result); err != nil {
		return SessionListResult{}, err
	}
	return result, nil
}

// SessionPrompt sends a user message (new turn or reply) to the agent and
// blocks until the agent returns a stop reason. Streaming session/update
// notifications are delivered concurrently via the SessionUpdate callback.
func (f *Forwarder) SessionPrompt(ctx context.Context, params SessionPromptParams) (SessionPromptResult, error) {
	var result SessionPromptResult
	if err := f.SendAgent(ctx, MethodSessionPrompt, params, &result); err != nil {
		return SessionPromptResult{}, err
	}
	return result, nil
}

// SessionCancel sends session/cancel notification to abort an in-progress prompt.
func (f *Forwarder) SessionCancel(sessionID string) error {
	return f.NotifyAgent(MethodSessionCancel, SessionCancelParams{SessionID: sessionID})
}

// SessionSetConfigOption sets a named config option on the active session and
// returns the updated config option list. Handles both response formats:
// []ConfigOption and {"configOptions":[...]}.
func (f *Forwarder) SessionSetConfigOption(ctx context.Context, params SessionSetConfigOptionParams) ([]ConfigOption, error) {
	var raw json.RawMessage
	if err := f.SendAgent(ctx, MethodSetConfigOption, params, &raw); err != nil {
		return nil, err
	}
	var opts []ConfigOption
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &opts); err != nil {
			var wrapped struct {
				ConfigOptions []ConfigOption `json:"configOptions"`
			}
			if json.Unmarshal(raw, &wrapped) == nil {
				opts = wrapped.ConfigOptions
			}
		}
	}
	return opts, nil
}

func marshalParams(params any) (json.RawMessage, error) {
	if params == nil {
		return nil, nil
	}
	if raw, ok := params.(json.RawMessage); ok {
		return raw, nil
	}
	b, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("acp forwarder: marshal params: %w", err)
	}
	return b, nil
}
