package mock

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/swm8023/wheelmaker/internal/acp"
)

type rpcMsg struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int64          `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *acp.RPCError   `json:"error,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type mockSessionState struct {
	mode    string
	model   string
	thought string
}

type inMemoryMockServer struct {
	enc    *json.Encoder
	encMu  sync.Mutex
	nextID atomic.Int64

	sessMu    sync.Mutex
	sessState map[string]*mockSessionState
	sessSeq   atomic.Int64

	pendMu  sync.Mutex
	pending map[int64]chan rpcMsg

	cancelMu      sync.Mutex
	promptCancels map[string]context.CancelFunc
}

func runInMemoryMockServer(r io.Reader, w io.Writer) {
	s := &inMemoryMockServer{
		enc:           json.NewEncoder(w),
		sessState:     make(map[string]*mockSessionState),
		pending:       make(map[int64]chan rpcMsg),
		promptCancels: make(map[string]context.CancelFunc),
	}
	s.nextID.Store(10000)

	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 1<<20), 1<<20)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var raw rpcMsg
		if err := json.Unmarshal(line, &raw); err != nil {
			continue
		}
		if raw.ID != nil && raw.Method == "" {
			s.routeResponse(*raw.ID, raw)
			continue
		}
		if raw.ID == nil {
			if raw.Method != "" {
				s.handleNotification(raw.Method, raw.Params)
			}
			continue
		}
		s.handleRequest(*raw.ID, raw.Method, raw.Params)
	}
}

// handleNotification handles incoming Client→Agent notifications (no id, no response).
// Currently only session/cancel is handled; all others are silently ignored per spec §16.2.
func (s *inMemoryMockServer) handleNotification(method string, params json.RawMessage) {
	if method != "session/cancel" {
		return
	}
	var p struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(params, &p); err != nil || p.SessionID == "" {
		return
	}
	s.cancelMu.Lock()
	if cancel, ok := s.promptCancels[p.SessionID]; ok {
		cancel()
	}
	s.cancelMu.Unlock()
}

func (s *inMemoryMockServer) handleRequest(id int64, method string, params json.RawMessage) {
	switch method {
	case "initialize":
		s.respond(id, map[string]any{
			"protocolVersion": 1,
			"agentCapabilities": map[string]any{
				"loadSession":         true,
				"sessionCapabilities": map[string]any{"list": map[string]any{}},
			},
			"agentInfo":   map[string]any{"name": "wheelmaker-mock", "title": "WheelMaker Mock Agent", "version": "0.1.0"},
			"authMethods": []any{},
		})
	case "session/new":
		n := s.sessSeq.Add(1)
		sid := fmt.Sprintf("mock-session-%d", n)
		s.sessMu.Lock()
		s.sessState[sid] = &mockSessionState{mode: "ask", model: "gpt-4.1", thought: "medium"}
		state := s.sessState[sid]
		s.sessMu.Unlock()
		s.respond(id, map[string]any{
			"sessionId":     sid,
			"modes":         map[string]any{"currentModeId": "ask", "availableModes": []map[string]any{{"id": "ask", "name": "Ask"}, {"id": "code", "name": "Code"}}},
			"models":        map[string]any{"currentModelId": "gpt-4.1", "availableModels": []map[string]any{{"modelId": "gpt-4.1", "name": "GPT-4.1"}, {"modelId": "gpt-4.1-mini", "name": "GPT-4.1 Mini"}}},
			"configOptions": s.buildConfigOptions(state),
		})
	case "session/load":
		var p acp.SessionLoadParams
		_ = json.Unmarshal(params, &p)
		s.sessMu.Lock()
		if _, ok := s.sessState[p.SessionID]; !ok {
			s.sessState[p.SessionID] = &mockSessionState{mode: "ask", model: "gpt-4.1", thought: "medium"}
		}
		s.sessMu.Unlock()
		s.respond(id, map[string]any{})
	case "session/set_config_option":
		var p acp.SessionSetConfigOptionParams
		_ = json.Unmarshal(params, &p)
		s.applyConfigOption(p.SessionID, p.ConfigID, p.Value)
		s.respond(id, s.buildConfigOptions(s.getState(p.SessionID)))
	case "session/set_mode":
		var p struct {
			SessionID string `json:"sessionId"`
			ModeID    string `json:"modeId"`
		}
		_ = json.Unmarshal(params, &p)
		if p.SessionID != "" && p.ModeID != "" {
			// Keep session/set_mode 1:1 with the "mode" config option.
			s.applyConfigOption(p.SessionID, "mode", p.ModeID)
			s.sendUpdate(p.SessionID, map[string]any{
				"sessionUpdate": "config_option_update",
				"configOptions": s.buildConfigOptions(s.getState(p.SessionID)),
			})
		}
		s.respond(id, map[string]any{})
	case "session/prompt":
		go s.handlePrompt(id, params)
	default:
		s.respond(id, map[string]any{})
	}
}

func (s *inMemoryMockServer) handlePrompt(id int64, params json.RawMessage) {
	var p acp.SessionPromptParams
	_ = json.Unmarshal(params, &p)

	// Register a cancellable context so session/cancel notifications can interrupt this prompt.
	ctx, cancel := context.WithCancel(context.Background())
	s.cancelMu.Lock()
	s.promptCancels[p.SessionID] = cancel
	s.cancelMu.Unlock()
	defer func() {
		cancel()
		s.cancelMu.Lock()
		delete(s.promptCancels, p.SessionID)
		s.cancelMu.Unlock()
	}()

	if len(p.Prompt) == 0 {
		s.respond(id, map[string]any{"stopReason": "end_turn"})
		return
	}
	text := strings.TrimSpace(p.Prompt[0].Text)
	if s.handleGlobalCommand(p.SessionID, text) {
		s.respond(id, map[string]any{"stopReason": "end_turn"})
		return
	}
	switch text {
	case "1":
		s.sendUpdate(p.SessionID, map[string]any{"sessionUpdate": "agent_message_chunk", "content": map[string]any{"type": "text", "text": "mock text chunk"}})
		s.sendUpdate(p.SessionID, map[string]any{"sessionUpdate": "agent_thought_chunk", "content": map[string]any{"type": "text", "text": "mock thought chunk"}})
		s.sendUpdate(p.SessionID, map[string]any{"sessionUpdate": "plan", "entries": []map[string]any{{"content": "step1", "priority": "high", "status": "pending"}, {"content": "step2", "priority": "medium", "status": "in_progress"}}})
		s.sendUpdate(p.SessionID, map[string]any{"sessionUpdate": "session_info_update", "title": "Mock Session", "updatedAt": time.Now().UTC().Format(time.RFC3339)})
		s.sendUpdate(p.SessionID, map[string]any{"sessionUpdate": "available_commands_update", "availableCommands": []map[string]any{{"name": "plan", "description": "Create a plan"}, {"name": "test", "description": "Run tests"}}})
		s.respond(id, map[string]any{"stopReason": "end_turn"})
	case "2":
		readRaw, readErr := s.callbackRequest("fs/read_text_file", map[string]any{"sessionId": p.SessionID, "path": "/mock/file.txt"})
		readContent := "fs-read-error"
		if readErr == nil {
			var rr acp.FSReadTextFileResult
			_ = json.Unmarshal(readRaw.Result, &rr)
			if rr.Content != "" {
				readContent = rr.Content
			}
		}
		_, _ = s.callbackRequest("fs/write_text_file", map[string]any{"sessionId": p.SessionID, "path": "/mock/written.txt", "content": readContent})
		s.sendUpdate(p.SessionID, map[string]any{"sessionUpdate": "agent_message_chunk", "content": map[string]any{"type": "text", "text": "fs:" + readContent}})
		s.respond(id, map[string]any{"stopReason": "end_turn"})
	case "3":
		createRaw, err := s.callbackRequest("terminal/create", map[string]any{"sessionId": p.SessionID, "command": "echo", "args": []string{"terminal"}})
		if err != nil {
			s.respondError(id, -32603, "terminal/create failed")
			return
		}
		var cr acp.TerminalCreateResult
		_ = json.Unmarshal(createRaw.Result, &cr)
		_, _ = s.callbackRequest("terminal/output", map[string]any{"sessionId": p.SessionID, "terminalId": cr.TerminalID})
		_, _ = s.callbackRequest("terminal/wait_for_exit", map[string]any{"sessionId": p.SessionID, "terminalId": cr.TerminalID})
		_, _ = s.callbackRequest("terminal/release", map[string]any{"sessionId": p.SessionID, "terminalId": cr.TerminalID})
		s.sendUpdate(p.SessionID, map[string]any{"sessionUpdate": "agent_message_chunk", "content": map[string]any{"type": "text", "text": "terminal:ok"}})
		s.respond(id, map[string]any{"stopReason": "end_turn"})
	case "4":
		// tool_call: create with pending status (§9.1)
		s.sendUpdate(p.SessionID, map[string]any{"sessionUpdate": "tool_call", "toolCallId": "call_001", "title": "Permission check", "kind": "execute", "status": "pending"})
		// tool_call_update: transition to in_progress (§9.2)
		s.sendUpdate(p.SessionID, map[string]any{"sessionUpdate": "tool_call_update", "toolCallId": "call_001", "status": "in_progress"})
		permRaw, err := s.callbackRequest("session/request_permission", map[string]any{"sessionId": p.SessionID, "toolCall": map[string]any{"toolCallId": "call_001"}, "options": []map[string]any{{"optionId": "allow_once", "name": "Allow once", "kind": "allow_once"}, {"optionId": "reject_once", "name": "Reject once", "kind": "reject_once"}}})
		cancelled := false
		status := "completed"
		msg := "permission:allowed"
		if err != nil {
			status = "failed"
			msg = "permission:error"
		} else {
			var pr acp.PermissionResponse
			_ = json.Unmarshal(permRaw.Result, &pr)
			if pr.Outcome.Outcome == "cancelled" {
				cancelled = true
				status = "failed"
				msg = "permission:cancelled"
			} else if strings.HasPrefix(pr.Outcome.OptionID, "reject") {
				status = "failed"
				msg = "permission:rejected"
			}
		}
		// tool_call_update: report final status (§9.2)
		s.sendUpdate(p.SessionID, map[string]any{"sessionUpdate": "tool_call_update", "toolCallId": "call_001", "status": status})
		s.sendUpdate(p.SessionID, map[string]any{"sessionUpdate": "agent_message_chunk", "content": map[string]any{"type": "text", "text": msg}})
		// Per spec §7.4: Agent MUST respond with stopReason "cancelled" when prompt was cancelled.
		if cancelled {
			s.respond(id, map[string]any{"stopReason": "cancelled"})
		} else {
			s.respond(id, map[string]any{"stopReason": "end_turn"})
		}
	case "10":
		s.respondError(id, -32602, "invalid params")
	case "11":
		s.respondError(id, -32601, "method not found")
	case "12":
		s.respondError(id, -32603, "internal error")
	case "13":
		s.respond(id, map[string]any{"stopReason": "cancelled"})
	case "14":
		// Slow response: supports cancellation via session/cancel (§7.4).
		select {
		case <-time.After(2 * time.Second):
			s.respond(id, map[string]any{"stopReason": "end_turn"})
		case <-ctx.Done():
			s.respond(id, map[string]any{"stopReason": "cancelled"})
		}
	default:
		s.sendUpdate(p.SessionID, map[string]any{"sessionUpdate": "agent_message_chunk", "content": map[string]any{"type": "text", "text": "echo:" + text}})
		s.respond(id, map[string]any{"stopReason": "end_turn"})
	}
}

func (s *inMemoryMockServer) handleGlobalCommand(sessionID, text string) bool {
	parts := strings.Fields(text)
	if len(parts) != 2 {
		return false
	}
	switch parts[0] {
	case "/model":
		s.applyConfigOption(sessionID, "model", parts[1])
	case "/mode":
		s.applyConfigOption(sessionID, "mode", parts[1])
	case "/thought":
		s.applyConfigOption(sessionID, "thought_level", parts[1])
	default:
		return false
	}
	s.sendUpdate(sessionID, map[string]any{"sessionUpdate": "config_option_update", "configOptions": s.buildConfigOptions(s.getState(sessionID))})
	return true
}

func (s *inMemoryMockServer) applyConfigOption(sessionID, configID, value string) {
	s.sessMu.Lock()
	defer s.sessMu.Unlock()
	state, ok := s.sessState[sessionID]
	if !ok {
		state = &mockSessionState{mode: "ask", model: "gpt-4.1", thought: "medium"}
		s.sessState[sessionID] = state
	}
	switch configID {
	case "model":
		state.model = value
	case "mode":
		state.mode = value
	case "thought_level":
		state.thought = value
	}
}

func (s *inMemoryMockServer) getState(sessionID string) *mockSessionState {
	s.sessMu.Lock()
	defer s.sessMu.Unlock()
	state, ok := s.sessState[sessionID]
	if !ok {
		state = &mockSessionState{mode: "ask", model: "gpt-4.1", thought: "medium"}
		s.sessState[sessionID] = state
	}
	return &mockSessionState{mode: state.mode, model: state.model, thought: state.thought}
}

func (s *inMemoryMockServer) buildConfigOptions(state *mockSessionState) []map[string]any {
	return []map[string]any{
		{"id": "mode", "name": "Mode", "category": "mode", "type": "select", "currentValue": state.mode, "options": []map[string]any{{"value": "ask", "name": "Ask"}, {"value": "code", "name": "Code"}}},
		{"id": "model", "name": "Model", "category": "model", "type": "select", "currentValue": state.model, "options": []map[string]any{{"value": "gpt-4.1", "name": "GPT-4.1"}, {"value": "gpt-4.1-mini", "name": "GPT-4.1 Mini"}}},
		{"id": "thought_level", "name": "Thought Level", "category": "thought_level", "type": "select", "currentValue": state.thought, "options": []map[string]any{{"value": "low", "name": "Low"}, {"value": "medium", "name": "Medium"}, {"value": "high", "name": "High"}}},
	}
}

func (s *inMemoryMockServer) callbackRequest(method string, params any) (rpcMsg, error) {
	id := s.nextID.Add(1)
	respCh := make(chan rpcMsg, 1)
	s.pendMu.Lock()
	s.pending[id] = respCh
	s.pendMu.Unlock()

	s.encMu.Lock()
	_ = s.enc.Encode(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
	s.encMu.Unlock()

	select {
	case resp := <-respCh:
		if resp.Error != nil {
			return rpcMsg{}, fmt.Errorf("rpc error %d: %s", resp.Error.Code, resp.Error.Message)
		}
		return resp, nil
	case <-time.After(3 * time.Second):
		s.pendMu.Lock()
		delete(s.pending, id)
		s.pendMu.Unlock()
		return rpcMsg{}, fmt.Errorf("timeout waiting callback %s", method)
	}
}

func (s *inMemoryMockServer) routeResponse(id int64, resp rpcMsg) {
	s.pendMu.Lock()
	ch, ok := s.pending[id]
	if ok {
		delete(s.pending, id)
	}
	s.pendMu.Unlock()
	if ok {
		ch <- resp
	}
}

func (s *inMemoryMockServer) sendUpdate(sessionID string, update any) {
	s.encMu.Lock()
	_ = s.enc.Encode(map[string]any{"jsonrpc": "2.0", "method": "session/update", "params": map[string]any{"sessionId": sessionID, "update": update}})
	s.encMu.Unlock()
}

func (s *inMemoryMockServer) respond(id int64, result any) {
	s.encMu.Lock()
	_ = s.enc.Encode(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
	s.encMu.Unlock()
}

func (s *inMemoryMockServer) respondError(id int64, code int, message string) {
	s.encMu.Lock()
	_ = s.enc.Encode(map[string]any{"jsonrpc": "2.0", "id": id, "error": map[string]any{"code": code, "message": message}})
	s.encMu.Unlock()
}
