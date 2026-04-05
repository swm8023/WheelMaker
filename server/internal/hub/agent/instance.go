package agent

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
}

// Instance is the only ACP-typed runtime interface exposed to Session.
type Instance interface {
	Name() string
	HandleACPRequest(ctx context.Context, method string, params json.RawMessage) (any, error)
	HandleACPResponse(ctx context.Context, method string, params json.RawMessage)
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
	runtime   *toolRuntime

	mu              sync.RWMutex
	connReady       bool
	initialized     bool
	acpSessionReady bool
	acpSessionID    string
	initResult      protocol.InitializeResult
}

var _ Instance = (*instance)(nil)

// NewInstance creates an agent instance and wires conn inbound routing.
func NewInstance(name string, conn Conn, callbacks Callbacks) Instance {
	inst := &instance{
		name:      strings.TrimSpace(name),
		conn:      conn,
		callbacks: callbacks,
		runtime:   newToolRuntime(),
		connReady: conn != nil,
	}
	if conn != nil {
		conn.OnACPRequest(inst.HandleACPRequest)
		conn.OnACPResponse(inst.HandleACPResponse)
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

func (i *instance) HandleACPResponse(_ context.Context, method string, params json.RawMessage) {
	if method == protocol.MethodSessionUpdate && i.callbacks != nil {
		var p protocol.SessionUpdateParams
		if err := json.Unmarshal(params, &p); err != nil {
			return
		}
		i.callbacks.SessionUpdate(p)
	}
}

func (i *instance) HandleACPRequest(ctx context.Context, method string, params json.RawMessage) (any, error) {
	switch method {
	case protocol.MethodRequestPermission:
		return i.onPermissionRequest(ctx, method, params)
	case protocol.MethodFSRead:
		var p protocol.FSReadTextFileParams
		if err := decodeACPParams(method, params, &p); err != nil {
			return nil, err
		}
		return i.runtime.FSRead(p)
	case protocol.MethodFSWrite:
		var p protocol.FSWriteTextFileParams
		if err := decodeACPParams(method, params, &p); err != nil {
			return nil, err
		}
		return nil, i.runtime.FSWrite(p)
	case protocol.MethodTerminalCreate:
		var p protocol.TerminalCreateParams
		if err := decodeACPParams(method, params, &p); err != nil {
			return nil, err
		}
		return i.runtime.TerminalCreate(p)
	case protocol.MethodTerminalOutput:
		var p protocol.TerminalOutputParams
		if err := decodeACPParams(method, params, &p); err != nil {
			return nil, err
		}
		return i.runtime.TerminalOutput(p)
	case protocol.MethodTerminalWaitExit:
		var p protocol.TerminalWaitForExitParams
		if err := decodeACPParams(method, params, &p); err != nil {
			return nil, err
		}
		return i.runtime.TerminalWaitForExit(p)
	case protocol.MethodTerminalKill:
		var p protocol.TerminalKillParams
		if err := decodeACPParams(method, params, &p); err != nil {
			return nil, err
		}
		return nil, i.runtime.TerminalKill(p)
	case protocol.MethodTerminalRelease:
		var p protocol.TerminalReleaseParams
		if err := decodeACPParams(method, params, &p); err != nil {
			return nil, err
		}
		return nil, i.runtime.TerminalRelease(p)
	default:
		return nil, fmt.Errorf("unsupported method: %s", method)
	}
}

func (i *instance) Close() error {
	if i.runtime != nil {
		i.runtime.Close()
	}
	if i.conn == nil {
		return nil
	}
	return i.conn.Close()
}

func (i *instance) ensureConn() error {
	if i.conn == nil {
		return errors.New("agent instance: conn is nil")
	}
	return nil
}

func (i *instance) onPermissionRequest(ctx context.Context, method string, params json.RawMessage) (any, error) {
	if i.callbacks == nil {
		return protocol.PermissionResponse{Outcome: protocol.PermissionResult{Outcome: "cancelled"}}, nil
	}
	var p protocol.PermissionRequestParams
	if err := decodeACPParams(method, params, &p); err != nil {
		return nil, err
	}
	result, err := i.callbacks.SessionRequestPermission(ctx, p)
	if err != nil {
		return nil, err
	}
	return protocol.PermissionResponse{Outcome: result}, nil
}

func decodeACPParams(method string, params json.RawMessage, out any) error {
	if err := json.Unmarshal(params, out); err != nil {
		return fmt.Errorf("%s: unmarshal: %w", method, err)
	}
	return nil
}
