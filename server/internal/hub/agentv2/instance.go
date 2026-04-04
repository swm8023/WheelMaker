package agentv2

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

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

// Instance is the only ACP-typed runtime interface exposed to Session.
type Instance interface {
	Name() string
	HandleInbound(ctx context.Context, method string, params json.RawMessage, noResponse bool) (any, error)
	Initialize(ctx context.Context, p protocol.InitializeParams) (protocol.InitializeResult, error)
	SessionNew(ctx context.Context, p protocol.SessionNewParams) (protocol.SessionNewResult, error)
	SessionLoad(ctx context.Context, p protocol.SessionLoadParams) (protocol.SessionLoadResult, error)
	SessionList(ctx context.Context, p protocol.SessionListParams) (protocol.SessionListResult, error)
	SessionPrompt(ctx context.Context, p protocol.SessionPromptParams) (protocol.SessionPromptResult, error)
	SessionCancel(acpSessionID string) error
	SessionSetConfigOption(ctx context.Context, p protocol.SessionSetConfigOptionParams) ([]protocol.ConfigOption, error)
	Close() error
}

type instance struct {
	name      string
	conn      Conn
	callbacks Callbacks

	mu              sync.RWMutex
	connReady       bool
	initialized     bool
	acpSessionReady bool
	acpSessionID    string
	initResult      protocol.InitializeResult
}

var _ Instance = (*instance)(nil)

// NewInstance creates an agentv2 instance and wires conn inbound routing.
func NewInstance(name string, conn Conn, callbacks Callbacks) Instance {
	inst := &instance{
		name:      strings.TrimSpace(name),
		conn:      conn,
		callbacks: callbacks,
		connReady: conn != nil,
	}
	if conn != nil {
		conn.OnRequest(inst.HandleInbound)
	}
	return inst
}

func (i *instance) Name() string { return i.name }

func (i *instance) Initialize(ctx context.Context, p protocol.InitializeParams) (protocol.InitializeResult, error) {
	if err := i.ensureConn(); err != nil {
		return protocol.InitializeResult{}, err
	}

	var out protocol.InitializeResult
	if err := i.conn.Send(ctx, protocol.MethodInitialize, p, &out); err != nil {
		return protocol.InitializeResult{}, err
	}

	i.mu.Lock()
	i.connReady = true
	i.initialized = true
	i.initResult = out
	i.mu.Unlock()
	return out, nil
}

func (i *instance) SessionNew(ctx context.Context, p protocol.SessionNewParams) (protocol.SessionNewResult, error) {
	if err := i.ensureConn(); err != nil {
		return protocol.SessionNewResult{}, err
	}

	var out protocol.SessionNewResult
	if err := i.conn.Send(ctx, protocol.MethodSessionNew, p, &out); err != nil {
		return protocol.SessionNewResult{}, err
	}

	if sid := strings.TrimSpace(out.SessionID); sid != "" {
		if binder, ok := i.conn.(sessionBinder); ok {
			binder.BindSessionID(sid)
		}
		i.mu.Lock()
		i.acpSessionID = sid
		i.acpSessionReady = true
		i.mu.Unlock()
	}
	return out, nil
}

func (i *instance) SessionLoad(ctx context.Context, p protocol.SessionLoadParams) (protocol.SessionLoadResult, error) {
	if err := i.ensureConn(); err != nil {
		return protocol.SessionLoadResult{}, err
	}
	if strings.TrimSpace(p.SessionID) == "" {
		return protocol.SessionLoadResult{}, errors.New("acp session id is required")
	}

	var out protocol.SessionLoadResult
	if err := i.conn.Send(ctx, protocol.MethodSessionLoad, p, &out); err != nil {
		return protocol.SessionLoadResult{}, err
	}

	if binder, ok := i.conn.(sessionBinder); ok {
		binder.BindSessionID(p.SessionID)
	}
	i.mu.Lock()
	i.acpSessionID = p.SessionID
	i.acpSessionReady = true
	i.mu.Unlock()
	return out, nil
}

func (i *instance) SessionList(ctx context.Context, p protocol.SessionListParams) (protocol.SessionListResult, error) {
	if err := i.ensureConn(); err != nil {
		return protocol.SessionListResult{}, err
	}
	var out protocol.SessionListResult
	if err := i.conn.Send(ctx, protocol.MethodSessionList, p, &out); err != nil {
		return protocol.SessionListResult{}, err
	}
	return out, nil
}

func (i *instance) SessionPrompt(ctx context.Context, p protocol.SessionPromptParams) (protocol.SessionPromptResult, error) {
	if err := i.ensureConn(); err != nil {
		return protocol.SessionPromptResult{}, err
	}

	if strings.TrimSpace(p.SessionID) == "" {
		i.mu.RLock()
		sid := i.acpSessionID
		ready := i.acpSessionReady
		i.mu.RUnlock()
		if !ready || strings.TrimSpace(sid) == "" {
			return protocol.SessionPromptResult{}, errors.New("acp session is not ready")
		}
		p.SessionID = sid
	}

	var out protocol.SessionPromptResult
	if err := i.conn.Send(ctx, protocol.MethodSessionPrompt, p, &out); err != nil {
		return protocol.SessionPromptResult{}, err
	}
	return out, nil
}

func (i *instance) SessionCancel(acpSessionID string) error {
	if err := i.ensureConn(); err != nil {
		return err
	}
	sid := strings.TrimSpace(acpSessionID)
	if sid == "" {
		i.mu.RLock()
		sid = strings.TrimSpace(i.acpSessionID)
		i.mu.RUnlock()
	}
	if sid == "" {
		return errors.New("acp session id is required")
	}
	return i.conn.Notify(protocol.MethodSessionCancel, protocol.SessionCancelParams{SessionID: sid})
}

func (i *instance) SessionSetConfigOption(ctx context.Context, p protocol.SessionSetConfigOptionParams) ([]protocol.ConfigOption, error) {
	if err := i.ensureConn(); err != nil {
		return nil, err
	}

	var raw json.RawMessage
	if err := i.conn.Send(ctx, protocol.MethodSetConfigOption, p, &raw); err != nil {
		return nil, err
	}
	var opts []protocol.ConfigOption
	if len(raw) == 0 {
		return opts, nil
	}
	if err := json.Unmarshal(raw, &opts); err == nil {
		return opts, nil
	}
	var wrapped struct {
		ConfigOptions []protocol.ConfigOption `json:"configOptions"`
	}
	if err := json.Unmarshal(raw, &wrapped); err == nil {
		return wrapped.ConfigOptions, nil
	}
	return opts, nil
}

func (i *instance) HandleInbound(ctx context.Context, method string, params json.RawMessage, noResponse bool) (any, error) {
	if noResponse {
		if method == protocol.MethodSessionUpdate && i.callbacks != nil {
			var p protocol.SessionUpdateParams
			if err := json.Unmarshal(params, &p); err != nil {
				return nil, nil
			}
			i.callbacks.SessionUpdate(p)
		}
		return nil, nil
	}

	if i.callbacks == nil {
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
		result, err := i.callbacks.SessionRequestPermission(ctx, p)
		if err != nil {
			return nil, err
		}
		return protocol.PermissionResponse{Outcome: result}, nil
	case protocol.MethodFSRead:
		return callbackCall(method, params, i.callbacks.FSRead)
	case protocol.MethodFSWrite:
		return callbackCallVoid(method, params, i.callbacks.FSWrite)
	case protocol.MethodTerminalCreate:
		return callbackCall(method, params, i.callbacks.TerminalCreate)
	case protocol.MethodTerminalOutput:
		return callbackCall(method, params, i.callbacks.TerminalOutput)
	case protocol.MethodTerminalWaitExit:
		return callbackCall(method, params, i.callbacks.TerminalWaitForExit)
	case protocol.MethodTerminalKill:
		return callbackCallVoid(method, params, i.callbacks.TerminalKill)
	case protocol.MethodTerminalRelease:
		return callbackCallVoid(method, params, i.callbacks.TerminalRelease)
	default:
		return nil, fmt.Errorf("unsupported method: %s", method)
	}
}

func (i *instance) Close() error {
	if i.conn == nil {
		return nil
	}
	return i.conn.Close()
}

func (i *instance) ensureConn() error {
	if i.conn == nil {
		return errors.New("agentv2 instance: conn is nil")
	}
	return nil
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
