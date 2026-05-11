package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/swm8023/wheelmaker/internal/protocol"
)

// codexAppProvider launches the native Codex app-server ACP bridge.
type codexAppProvider struct {
	lookPath func(file string) (string, error)
}

func NewCodexAppProvider() *codexAppProvider {
	return &codexAppProvider{
		lookPath: exec.LookPath,
	}
}

func (p *codexAppProvider) Name() string {
	return "codexapp"
}

func (p *codexAppProvider) Launch() (string, []string, []string, error) {
	lookPath := p.lookPath
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	exe, err := lookPath("codex")
	if err != nil {
		return "", nil, nil, fmt.Errorf("codexapp: codex not found: %w", err)
	}
	return exe, []string{"app-server", "--listen", "stdio://"}, nil, nil
}

func codexappInstanceCreator(provider *codexAppProvider) InstanceCreator {
	if provider == nil {
		provider = NewCodexAppProvider()
	}
	return func(_ context.Context, cwd string) (Instance, error) {
		conn, err := newOwnedCodexappConn(provider, cwd)
		if err != nil {
			return nil, err
		}
		return NewInstance(provider.Name(), conn), nil
	}
}

type codexappTransport interface {
	SendMessage(v any) error
	OnMessage(h func(json.RawMessage))
	Done() <-chan struct{}
	Close() error
	Alive() bool
}

func newOwnedCodexappConn(provider *codexAppProvider, cwd string) (*codexappConn, error) {
	exe, args, env, err := provider.Launch()
	if err != nil {
		return nil, err
	}
	raw := NewACPProcess(provider.Name(), exe, env, args...)
	raw.SetDir(cwd)
	if err := raw.Start(); err != nil {
		return nil, err
	}
	return newCodexappConnWithRuntime(newCodexappRuntimeWithTransport(raw), cwd), nil
}

type codexappRuntime struct {
	transport codexappTransport

	mu       sync.Mutex
	nextID   int64
	pending  map[string]chan codexappRPCResponse
	conns    map[string]*codexappConn
	closed   bool
	closeErr error
}

func newCodexappRuntimeWithTransport(transport codexappTransport) *codexappRuntime {
	rt := &codexappRuntime{
		transport: transport,
		pending:   map[string]chan codexappRPCResponse{},
		conns:     map[string]*codexappConn{},
	}
	if transport != nil {
		transport.OnMessage(rt.handleMessage)
	}
	return rt
}

func (r *codexappRuntime) request(ctx context.Context, method string, params any, out any) error {
	if r == nil || r.transport == nil {
		return errors.New("codexapp runtime is not ready")
	}
	id := atomic.AddInt64(&r.nextID, 1)
	idRaw, _ := json.Marshal(id)
	key := string(idRaw)
	ch := make(chan codexappRPCResponse, 1)

	r.mu.Lock()
	if r.closed {
		err := r.closeErr
		r.mu.Unlock()
		if err == nil {
			err = errors.New("codexapp runtime closed")
		}
		return err
	}
	r.pending[key] = ch
	r.mu.Unlock()

	req := codexappRPCRequest{ID: id, Method: method, Params: params}
	if err := r.transport.SendMessage(req); err != nil {
		r.removePending(key)
		return err
	}

	select {
	case resp := <-ch:
		if resp.Error != nil {
			return fmt.Errorf("codexapp %s: %s", method, resp.Error.Message)
		}
		if out == nil {
			return nil
		}
		if len(resp.Result) == 0 {
			resp.Result = json.RawMessage(`null`)
		}
		if raw, ok := out.(*json.RawMessage); ok {
			*raw = append((*raw)[:0], resp.Result...)
			return nil
		}
		if err := json.Unmarshal(resp.Result, out); err != nil {
			return fmt.Errorf("codexapp %s: decode result: %w", method, err)
		}
		return nil
	case <-ctx.Done():
		r.removePending(key)
		return ctx.Err()
	case <-r.transport.Done():
		r.removePending(key)
		return errors.New("codexapp runtime stopped")
	}
}

func (r *codexappRuntime) notify(method string, params any) error {
	if r == nil || r.transport == nil {
		return errors.New("codexapp runtime is not ready")
	}
	return r.transport.SendMessage(codexappRPCNotification{Method: method, Params: params})
}

func (r *codexappRuntime) register(threadID string, conn *codexappConn) {
	threadID = strings.TrimSpace(threadID)
	if r == nil || threadID == "" || conn == nil {
		return
	}
	r.mu.Lock()
	r.conns[threadID] = conn
	r.mu.Unlock()
}

func (r *codexappRuntime) unregister(threadID string, conn *codexappConn) {
	threadID = strings.TrimSpace(threadID)
	if r == nil || threadID == "" {
		return
	}
	r.mu.Lock()
	if r.conns[threadID] == conn {
		delete(r.conns, threadID)
	}
	r.mu.Unlock()
}

func (r *codexappRuntime) close() error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	r.closed = true
	if r.closeErr == nil {
		r.closeErr = errors.New("codexapp runtime closed")
	}
	for key, ch := range r.pending {
		delete(r.pending, key)
		ch <- codexappRPCResponse{Error: &codexappRPCError{Code: -32000, Message: r.closeErr.Error()}}
	}
	conns := make([]*codexappConn, 0, len(r.conns))
	for _, conn := range r.conns {
		conns = append(conns, conn)
	}
	r.mu.Unlock()
	for _, conn := range conns {
		conn.failActivePrompt(r.closeErr)
	}
	if r.transport == nil {
		return nil
	}
	return r.transport.Close()
}

func (r *codexappRuntime) alive() bool {
	return r != nil && r.transport != nil && r.transport.Alive()
}

func (r *codexappRuntime) removePending(key string) {
	r.mu.Lock()
	delete(r.pending, key)
	r.mu.Unlock()
}

func (r *codexappRuntime) handleMessage(raw json.RawMessage) {
	var msg codexappRPCEnvelope
	if err := json.Unmarshal(raw, &msg); err != nil {
		return
	}
	if len(msg.ID) > 0 && msg.Method == "" {
		r.resolveResponse(msg)
		return
	}
	if msg.Method == "" {
		return
	}
	if len(msg.ID) > 0 {
		go r.handleServerRequest(msg)
		return
	}
	go r.handleNotification(msg)
}

func (r *codexappRuntime) resolveResponse(msg codexappRPCEnvelope) {
	key := string(msg.ID)
	r.mu.Lock()
	ch := r.pending[key]
	delete(r.pending, key)
	r.mu.Unlock()
	if ch == nil {
		return
	}
	ch <- codexappRPCResponse{Result: msg.Result, Error: msg.Error}
}

func (r *codexappRuntime) handleNotification(msg codexappRPCEnvelope) {
	threadID := codexappThreadIDFromParams(msg.Params)
	if threadID == "" {
		return
	}
	conn := r.connForThread(threadID)
	if conn == nil {
		return
	}
	conn.handleAppServerNotification(msg.Method, msg.Params)
}

func (r *codexappRuntime) handleServerRequest(msg codexappRPCEnvelope) {
	threadID := codexappThreadIDFromParams(msg.Params)
	conn := r.connForThread(threadID)
	if conn == nil {
		_ = r.transport.SendMessage(codexappRPCServerResponse{
			ID:    msg.ID,
			Error: &codexappRPCError{Code: -32601, Message: "method not found: " + msg.Method},
		})
		return
	}
	result, err := conn.handleAppServerRequest(context.Background(), msg.Method, msg.Params)
	if err != nil {
		code := -32000
		var methodNotFound codexappMethodNotFoundError
		if errors.As(err, &methodNotFound) {
			code = -32601
		}
		_ = r.transport.SendMessage(codexappRPCServerResponse{
			ID:    msg.ID,
			Error: &codexappRPCError{Code: code, Message: err.Error()},
		})
		return
	}
	_ = r.transport.SendMessage(codexappRPCServerResponse{ID: msg.ID, Result: result})
}

func (r *codexappRuntime) connForThread(threadID string) *codexappConn {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.conns[threadID]
}

type codexappConn struct {
	runtime *codexappRuntime
	cwd     string

	mu            sync.Mutex
	reqHandler    ACPRequestHandler
	respHandler   ACPResponseHandler
	initialized   bool
	initializeRes protocol.InitializeResult
	threadID      string
	config        codexappConfigState
	activeTurnID  string
	lastTurnID    string
	promptDone    chan codexappPromptResult

	pendingPromptStops   map[string]string
	pendingPromptUpdates map[string][]protocol.SessionUpdateParams
}

type codexappPromptResult struct {
	stopReason string
	err        error
}

func newCodexappConnWithRuntime(runtime *codexappRuntime, cwd string) *codexappConn {
	return &codexappConn{
		runtime: runtime,
		cwd:     cwd,
		config:  newCodexappConfigState(),
	}
}

var _ Conn = (*codexappConn)(nil)

func (c *codexappConn) Send(ctx context.Context, method string, params any, result any) error {
	switch method {
	case protocol.MethodInitialize:
		return c.sendInitialize(ctx, result)
	case protocol.MethodSessionNew:
		var p protocol.SessionNewParams
		if err := remarshal(params, &p); err != nil {
			return err
		}
		return c.sendSessionNew(ctx, p, result)
	case protocol.MethodSessionLoad:
		var p protocol.SessionLoadParams
		if err := remarshal(params, &p); err != nil {
			return err
		}
		return c.sendSessionLoad(ctx, p, result)
	case protocol.MethodSessionList:
		var p protocol.SessionListParams
		if err := remarshal(params, &p); err != nil {
			return err
		}
		return c.sendSessionList(ctx, p, result)
	case protocol.MethodSessionPrompt:
		var p protocol.SessionPromptParams
		if err := remarshal(params, &p); err != nil {
			return err
		}
		return c.sendSessionPrompt(ctx, p, result)
	case protocol.MethodSetConfigOption:
		var p protocol.SessionSetConfigOptionParams
		if err := remarshal(params, &p); err != nil {
			return err
		}
		return c.sendSetConfigOption(result, p)
	default:
		return fmt.Errorf("codexapp: unsupported ACP method %s", method)
	}
}

func (c *codexappConn) Notify(method string, params any) error {
	switch method {
	case protocol.MethodSessionCancel:
		var p protocol.SessionCancelParams
		if err := remarshal(params, &p); err != nil {
			return err
		}
		return c.cancel(p.SessionID)
	default:
		return nil
	}
}

func (c *codexappConn) OnACPRequest(h ACPRequestHandler) {
	c.mu.Lock()
	c.reqHandler = h
	c.mu.Unlock()
}

func (c *codexappConn) OnACPResponse(h ACPResponseHandler) {
	c.mu.Lock()
	c.respHandler = h
	c.mu.Unlock()
}

func (c *codexappConn) Close() error {
	c.mu.Lock()
	threadID := c.threadID
	c.mu.Unlock()
	if c.runtime != nil {
		c.runtime.unregister(threadID, c)
		return c.runtime.close()
	}
	return nil
}

func (c *codexappConn) Alive() bool {
	return c != nil && c.runtime != nil && c.runtime.alive()
}

func (c *codexappConn) BindSessionID(sessionID string) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}
	c.mu.Lock()
	old := c.threadID
	c.threadID = sessionID
	c.mu.Unlock()
	if c.runtime != nil {
		if old != "" && old != sessionID {
			c.runtime.unregister(old, c)
		}
		c.runtime.register(sessionID, c)
	}
}

func (c *codexappConn) sendInitialize(ctx context.Context, result any) error {
	c.mu.Lock()
	if c.initialized {
		cached := c.initializeRes
		c.mu.Unlock()
		return assignResult(result, cached)
	}
	c.mu.Unlock()

	var ignored json.RawMessage
	if err := c.runtime.request(ctx, "initialize", appServerInitializeParams{
		ClientInfo: appServerClientInfo{Name: "wheelmaker", Title: "WheelMaker"},
	}, &ignored); err != nil {
		return err
	}
	if err := c.runtime.notify("initialized", nil); err != nil {
		return err
	}
	out := protocol.InitializeResult{
		ProtocolVersion: json.Number("1"),
		AgentInfo:       &protocol.AgentInfo{Name: "codexapp", Title: "Codex App Server"},
		AgentCapabilities: protocol.AgentCapabilities{
			LoadSession: true,
			PromptCapabilities: &protocol.PromptCapabilities{
				Image:           false,
				Audio:           false,
				EmbeddedContext: false,
			},
			SessionCapabilities: &protocol.SessionCapabilities{List: &protocol.SessionListCapability{}},
		},
	}
	c.mu.Lock()
	c.initialized = true
	c.initializeRes = out
	c.mu.Unlock()
	return assignResult(result, out)
}

func (c *codexappConn) sendSessionNew(ctx context.Context, p protocol.SessionNewParams, result any) error {
	if len(p.MCPServers) > 0 {
		return errors.New("codexapp phase 1 does not support MCP servers")
	}
	if err := c.refreshModels(ctx); err != nil {
		return err
	}
	req := c.config.threadStartParams(firstNonEmptyString(p.CWD, c.cwd))
	var resp appServerThreadStartResponse
	if err := c.runtime.request(ctx, "thread/start", req, &resp); err != nil {
		return err
	}
	threadID := strings.TrimSpace(resp.Thread.ID)
	if threadID == "" {
		return errors.New("codexapp thread/start returned empty thread id")
	}
	c.BindSessionID(threadID)
	return assignResult(result, protocol.SessionNewResult{
		SessionID:     threadID,
		ConfigOptions: c.config.options(),
	})
}

func (c *codexappConn) sendSessionLoad(ctx context.Context, p protocol.SessionLoadParams, result any) error {
	if len(p.MCPServers) > 0 {
		return errors.New("codexapp phase 1 does not support MCP servers")
	}
	threadID := strings.TrimSpace(p.SessionID)
	if threadID == "" {
		return errors.New("codexapp session/load requires sessionId")
	}
	if err := c.refreshModels(ctx); err != nil {
		return err
	}
	c.BindSessionID(threadID)
	req := c.config.threadResumeParams(threadID, firstNonEmptyString(p.CWD, c.cwd))
	var ignored json.RawMessage
	if err := c.runtime.request(ctx, "thread/resume", req, &ignored); err != nil {
		return err
	}
	return assignResult(result, protocol.SessionLoadResult{ConfigOptions: c.config.options()})
}

func (c *codexappConn) sendSessionList(ctx context.Context, p protocol.SessionListParams, result any) error {
	var resp appServerThreadListResponse
	if err := c.runtime.request(ctx, "thread/list", appServerThreadListParams{CWD: p.CWD, Cursor: p.Cursor}, &resp); err != nil {
		return err
	}
	out := protocol.SessionListResult{NextCursor: resp.NextCursor}
	for _, thread := range resp.Threads {
		out.Sessions = append(out.Sessions, protocol.SessionInfo{
			SessionID: thread.ID,
			CWD:       thread.CWD,
			Title:     thread.Title,
			UpdatedAt: thread.UpdatedAt,
		})
	}
	return assignResult(result, out)
}

func (c *codexappConn) sendSessionPrompt(ctx context.Context, p protocol.SessionPromptParams, result any) error {
	input, err := codexappPromptToInput(p.Prompt)
	if err != nil {
		return err
	}
	threadID := strings.TrimSpace(p.SessionID)
	if threadID == "" {
		c.mu.Lock()
		threadID = c.threadID
		c.mu.Unlock()
	}
	if threadID == "" {
		return errors.New("codexapp session/prompt requires sessionId")
	}
	done := make(chan codexappPromptResult, 1)
	c.mu.Lock()
	if c.promptDone != nil {
		c.mu.Unlock()
		return errors.New("codexapp session already has an active turn")
	}
	c.promptDone = done
	c.activeTurnID = ""
	c.pendingPromptStops = nil
	c.pendingPromptUpdates = nil
	c.mu.Unlock()

	c.emitSessionUpdate(protocol.SessionUpdateParams{
		SessionID: threadID,
		Update: protocol.SessionUpdate{
			SessionUpdate: protocol.SessionUpdateUserMessageChunk,
			Content:       mustRaw(protocol.ContentBlock{Type: protocol.ContentBlockTypeText, Text: codexappInputText(input)}),
		},
	})

	var resp appServerTurnStartResponse
	if err := c.runtime.request(ctx, "turn/start", c.config.turnStartParams(threadID, firstNonEmptyString(c.cwd), input), &resp); err != nil {
		c.clearPromptDone(done)
		return err
	}
	if resp.Turn.ID != "" {
		c.setActiveTurnID(resp.Turn.ID)
	}

	select {
	case promptResult := <-done:
		if promptResult.err != nil {
			return promptResult.err
		}
		return assignResult(result, protocol.SessionPromptResult{StopReason: promptResult.stopReason})
	case <-ctx.Done():
		c.clearPromptDone(done)
		return ctx.Err()
	}
}

func (c *codexappConn) sendSetConfigOption(result any, p protocol.SessionSetConfigOptionParams) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.config.set(p.ConfigID, p.Value); err != nil {
		return err
	}
	return assignResult(result, c.config.options())
}

func (c *codexappConn) refreshModels(ctx context.Context) error {
	var resp appServerModelListResponse
	if err := c.runtime.request(ctx, "model/list", nil, &resp); err != nil {
		return err
	}
	c.mu.Lock()
	c.config.setModels(resp.Models)
	c.mu.Unlock()
	return nil
}

func (c *codexappConn) cancel(sessionID string) error {
	c.mu.Lock()
	threadID := firstNonEmptyString(strings.TrimSpace(sessionID), c.threadID)
	turnID := firstNonEmptyString(c.activeTurnID, c.lastTurnID)
	done := c.promptDone
	c.promptDone = nil
	c.activeTurnID = ""
	c.pendingPromptStops = nil
	c.pendingPromptUpdates = nil
	c.mu.Unlock()
	if threadID == "" {
		if done != nil {
			select {
			case done <- codexappPromptResult{stopReason: protocol.StopReasonCancelled}:
			default:
			}
		}
		return nil
	}
	if turnID != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		var ignored json.RawMessage
		_ = c.runtime.request(ctx, "turn/interrupt", appServerTurnInterruptParams{ThreadID: threadID, TurnID: turnID}, &ignored)
	}
	if done != nil {
		select {
		case done <- codexappPromptResult{stopReason: protocol.StopReasonCancelled}:
		default:
		}
	}
	return nil
}

type codexappMethodNotFoundError struct {
	method string
}

func (e codexappMethodNotFoundError) Error() string {
	return "method not found: " + e.method
}

func (c *codexappConn) handleAppServerNotification(method string, params json.RawMessage) {
	switch method {
	case "item/agentMessage/delta":
		var p appServerAgentMessageDeltaParams
		if json.Unmarshal(params, &p) == nil && p.ThreadID != "" && p.Delta != "" {
			c.emitTurnTextUpdate(p.ThreadID, p.TurnID, protocol.SessionUpdateAgentMessageChunk, p.Delta)
		}
	case "item/reasoning/textDelta", "item/reasoning/summaryTextDelta":
		var p appServerAgentMessageDeltaParams
		if json.Unmarshal(params, &p) == nil && p.ThreadID != "" && p.Delta != "" {
			c.emitTurnTextUpdate(p.ThreadID, p.TurnID, protocol.SessionUpdateAgentThoughtChunk, p.Delta)
		}
	case "turn/started":
		var p appServerTurnEventParams
		if json.Unmarshal(params, &p) == nil {
			c.setActiveTurnID(p.TurnID)
		}
	case "turn/completed":
		var p appServerTurnCompletedParams
		if json.Unmarshal(params, &p) == nil {
			c.completePrompt(p.TurnID, codexappStopReason(p.Status))
		}
	case "thread/name/updated":
		var p appServerThreadNameUpdatedParams
		if json.Unmarshal(params, &p) == nil && p.ThreadID != "" {
			c.emitSessionUpdate(protocol.SessionUpdateParams{
				SessionID: p.ThreadID,
				Update: protocol.SessionUpdate{
					SessionUpdate: protocol.SessionUpdateSessionInfoUpdate,
					Title:         p.Name,
				},
			})
		}
	}
}

func (c *codexappConn) handleAppServerRequest(ctx context.Context, method string, params json.RawMessage) (any, error) {
	switch method {
	case "item/commandExecution/requestApproval":
		return c.handleApprovalRequest(ctx, params, protocol.ToolKindExecute)
	case "item/fileChange/requestApproval":
		return c.handleApprovalRequest(ctx, params, protocol.ToolKindWrite)
	default:
		return nil, codexappMethodNotFoundError{method: method}
	}
}

func (c *codexappConn) handleApprovalRequest(ctx context.Context, params json.RawMessage, kind string) (any, error) {
	var p appServerApprovalRequestParams
	if err := json.Unmarshal(params, &p); err != nil {
		return appServerApprovalDecision{Decision: "cancel"}, nil
	}
	c.mu.Lock()
	h := c.reqHandler
	c.mu.Unlock()
	if h == nil {
		return appServerApprovalDecision{Decision: "cancel"}, nil
	}
	title := firstNonEmptyString(p.Command, p.Path, "Approval requested")
	resp, err := h(ctx, time.Now().UnixNano(), protocol.MethodRequestPermission, mustRaw(protocol.PermissionRequestParams{
		SessionID: p.ThreadID,
		ToolCall: protocol.ToolCallRef{
			ToolCallID: p.ItemID,
			Title:      title,
			Kind:       kind,
			Status:     protocol.ToolCallStatusPending,
		},
		Options: []protocol.PermissionOption{
			{OptionID: "allow_once", Name: "Allow once", Kind: "allow_once"},
			{OptionID: "allow_always", Name: "Allow for session", Kind: "allow_always"},
			{OptionID: "reject", Name: "Reject", Kind: "reject_once"},
		},
	}))
	if err != nil {
		return appServerApprovalDecision{Decision: "cancel"}, nil
	}
	var permission protocol.PermissionResponse
	if err := remarshal(resp, &permission); err != nil {
		return appServerApprovalDecision{Decision: "cancel"}, nil
	}
	return appServerApprovalDecision{Decision: codexappApprovalDecision(permission.Outcome)}, nil
}

func (c *codexappConn) emitTextUpdate(sessionID string, updateType string, text string) {
	c.emitSessionUpdate(protocol.SessionUpdateParams{
		SessionID: sessionID,
		Update: protocol.SessionUpdate{
			SessionUpdate: updateType,
			Content:       mustRaw(protocol.ContentBlock{Type: protocol.ContentBlockTypeText, Text: text}),
		},
	})
}

func (c *codexappConn) emitTurnTextUpdate(sessionID string, turnID string, updateType string, text string) {
	update := protocol.SessionUpdateParams{
		SessionID: sessionID,
		Update: protocol.SessionUpdate{
			SessionUpdate: updateType,
			Content:       mustRaw(protocol.ContentBlock{Type: protocol.ContentBlockTypeText, Text: text}),
		},
	}
	if c.deferOrDropTurnUpdate(turnID, update) {
		return
	}
	c.emitSessionUpdate(update)
}

func (c *codexappConn) emitSessionUpdate(update protocol.SessionUpdateParams) {
	c.mu.Lock()
	h := c.respHandler
	c.mu.Unlock()
	if h == nil {
		return
	}
	h(context.Background(), protocol.MethodSessionUpdate, mustRaw(update))
}

func (c *codexappConn) setActiveTurnID(turnID string) {
	turnID = strings.TrimSpace(turnID)
	if turnID == "" {
		return
	}
	c.mu.Lock()
	if c.promptDone == nil {
		c.lastTurnID = turnID
		c.mu.Unlock()
		return
	}
	c.activeTurnID = turnID
	c.lastTurnID = turnID
	updates := append([]protocol.SessionUpdateParams(nil), c.pendingPromptUpdates[turnID]...)
	if c.pendingPromptUpdates != nil {
		delete(c.pendingPromptUpdates, turnID)
	}
	stopReason := ""
	if c.pendingPromptStops != nil {
		stopReason = c.pendingPromptStops[turnID]
		delete(c.pendingPromptStops, turnID)
	}
	c.mu.Unlock()

	for _, update := range updates {
		c.emitSessionUpdate(update)
	}
	if stopReason != "" {
		c.completePrompt(turnID, stopReason)
	}
}

func (c *codexappConn) deferOrDropTurnUpdate(turnID string, update protocol.SessionUpdateParams) bool {
	turnID = strings.TrimSpace(turnID)
	if turnID == "" {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.promptDone == nil {
		return c.lastTurnID != "" && c.lastTurnID != turnID
	}
	if c.activeTurnID == "" {
		if c.pendingPromptUpdates == nil {
			c.pendingPromptUpdates = map[string][]protocol.SessionUpdateParams{}
		}
		c.pendingPromptUpdates[turnID] = append(c.pendingPromptUpdates[turnID], update)
		return true
	}
	return c.activeTurnID != turnID
}

func (c *codexappConn) completePrompt(turnID string, stopReason string) {
	turnID = strings.TrimSpace(turnID)
	c.mu.Lock()
	done := c.promptDone
	if done == nil || turnID == "" {
		c.mu.Unlock()
		return
	}
	if c.activeTurnID == "" {
		if c.pendingPromptStops == nil {
			c.pendingPromptStops = map[string]string{}
		}
		c.pendingPromptStops[turnID] = stopReason
		c.mu.Unlock()
		return
	}
	if c.activeTurnID != turnID {
		c.mu.Unlock()
		return
	}
	c.promptDone = nil
	c.activeTurnID = ""
	c.pendingPromptStops = nil
	c.pendingPromptUpdates = nil
	c.mu.Unlock()
	select {
	case done <- codexappPromptResult{stopReason: stopReason}:
	default:
	}
}

func (c *codexappConn) failActivePrompt(err error) {
	if err == nil {
		err = errors.New("codexapp runtime stopped")
	}
	c.mu.Lock()
	done := c.promptDone
	c.promptDone = nil
	c.activeTurnID = ""
	c.pendingPromptStops = nil
	c.pendingPromptUpdates = nil
	c.mu.Unlock()
	if done != nil {
		select {
		case done <- codexappPromptResult{err: err}:
		default:
		}
	}
}

func (c *codexappConn) clearPromptDone(done chan codexappPromptResult) {
	c.mu.Lock()
	if c.promptDone == done {
		c.promptDone = nil
		c.pendingPromptStops = nil
		c.pendingPromptUpdates = nil
	}
	c.mu.Unlock()
}

func assignResult(result any, value any) error {
	if result == nil {
		return nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if out, ok := result.(*json.RawMessage); ok {
		*out = append((*out)[:0], raw...)
		return nil
	}
	return json.Unmarshal(raw, result)
}

func remarshal(in any, out any) error {
	raw, err := json.Marshal(in)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return err
	}
	return nil
}

func mustRaw(v any) json.RawMessage {
	raw, _ := json.Marshal(v)
	return raw
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
