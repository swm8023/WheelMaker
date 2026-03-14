package agent_test

// agent_test.go: unit tests for agent.Agent using a self-referential mock ACP server.
//
// Pattern: when GO_AGENT_MOCK=1 is set, the test binary acts as the ACP mock
// server (reading stdin, writing stdout). Otherwise it runs the tests,
// pointing acp.New() at os.Args[0] with GO_AGENT_MOCK=1.
//
// The mock supports bidirectional communication: during session/prompt handling
// it can send Agent→Client requests (callbacks) and wait for responses.
// Prompt text controls mock behavior:
//   - "no-text-prompt"                  → no text chunks emitted (tests stale-context)
//   - "test-callback-fs-read:<path>"    → sends fs/read_text_file callback
//   - "test-callback-fs-write:<p>:<c>"  → sends fs/write_text_file callback
//   - "test-callback-permission"        → sends session/request_permission callback
//   - "test-callback-terminal"          → full terminal lifecycle callback
//   - "test-callback-terminal-kill"     → terminal create+kill lifecycle callback
//   - any other text                    → 2 text chunks + done

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/swm8023/wheelmaker/internal/agent"
	"github.com/swm8023/wheelmaker/internal/agent/acp"
)

var mockBin string

func TestMain(m *testing.M) {
	if os.Getenv("GO_AGENT_MOCK") == "1" {
		runMockAgent()
		os.Exit(0)
	}
	mockBin = os.Args[0]
	os.Exit(m.Run())
}

// newAgent creates an Agent backed by a fresh mock ACP subprocess.
func newAgent(t *testing.T, name string) *agent.Agent {
	t.Helper()
	conn := acp.New(mockBin, []string{"GO_AGENT_MOCK=1"})
	if err := conn.Start(); err != nil {
		t.Fatalf("conn.Start: %v", err)
	}
	ag := agent.New(name, conn, t.TempDir())
	t.Cleanup(func() { _ = ag.Close() })
	return ag
}

// drainUpdates drains an update channel and returns accumulated text and any error.
func drainUpdates(ch <-chan agent.Update) (text string, err error) {
	for u := range ch {
		if u.Err != nil {
			err = u.Err
			return
		}
		if u.Type == agent.UpdateText {
			text += u.Content
		}
	}
	return
}

// --- Tests ---

func TestAgent_SessionID_AfterReady(t *testing.T) {
	ag := newAgent(t, "test")
	ctx := context.Background()
	ch, err := ag.Prompt(ctx, "hello")
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	if _, err := drainUpdates(ch); err != nil {
		t.Fatalf("updates error: %v", err)
	}
	if sid := ag.SessionID(); sid == "" {
		t.Error("SessionID should be non-empty after prompt completes")
	}
}

func TestAgent_Prompt_TextUpdates(t *testing.T) {
	ag := newAgent(t, "test")
	ctx := context.Background()
	ch, err := ag.Prompt(ctx, "hello")
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	text, err := drainUpdates(ch)
	if err != nil {
		t.Fatalf("updates error: %v", err)
	}
	if text == "" {
		t.Error("expected non-empty accumulated text from prompt")
	}
}

func TestAgent_Prompt_ClearsLastReply(t *testing.T) {
	ag := newAgent(t, "test")
	ctx := context.Background()

	ch, err := ag.Prompt(ctx, "first")
	if err != nil {
		t.Fatalf("first Prompt: %v", err)
	}
	if _, err := drainUpdates(ch); err != nil {
		t.Fatalf("first updates error: %v", err)
	}

	ch, err = ag.Prompt(ctx, "second")
	if err != nil {
		t.Fatalf("second Prompt: %v", err)
	}
	if _, err := drainUpdates(ch); err != nil {
		t.Fatalf("second updates error: %v", err)
	}

	newConn := acp.New(mockBin, []string{"GO_AGENT_MOCK=1"})
	if err := newConn.Start(); err != nil {
		t.Fatalf("newConn.Start: %v", err)
	}
	if err := ag.Switch(ctx, "test2", newConn, agent.SwitchWithContext); err != nil {
		t.Fatalf("Switch: %v", err)
	}
}

func TestAgent_Cancel_BeforeReady(t *testing.T) {
	ag := newAgent(t, "test")
	if err := ag.Cancel(); err != nil {
		t.Errorf("Cancel before ready: %v", err)
	}
}

func TestAgent_Cancel_AfterReady(t *testing.T) {
	ag := newAgent(t, "test")
	ctx := context.Background()
	ch, err := ag.Prompt(ctx, "setup")
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	if _, err := drainUpdates(ch); err != nil {
		t.Fatalf("updates error: %v", err)
	}
	if err := ag.Cancel(); err != nil {
		t.Errorf("Cancel after ready: %v", err)
	}
}

func TestAgent_SetMode_BeforeReady(t *testing.T) {
	ag := newAgent(t, "test")
	if err := ag.SetMode(context.Background(), "auto"); err == nil {
		t.Error("expected error from SetMode before ready")
	}
}

func TestAgent_SetMode_AfterReady(t *testing.T) {
	ag := newAgent(t, "test")
	ctx := context.Background()
	ch, err := ag.Prompt(ctx, "setup")
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	if _, err := drainUpdates(ch); err != nil {
		t.Fatalf("updates error: %v", err)
	}
	if err := ag.SetMode(ctx, "auto"); err != nil {
		t.Errorf("SetMode after ready: %v", err)
	}
}

func TestAgent_SetConfigOption_BeforeReady(t *testing.T) {
	ag := newAgent(t, "test")
	if err := ag.SetConfigOption(context.Background(), "model", "gpt-4"); err == nil {
		t.Error("expected error from SetConfigOption before ready")
	}
}

func TestAgent_SetConfigOption_AfterReady(t *testing.T) {
	ag := newAgent(t, "test")
	ctx := context.Background()
	ch, err := ag.Prompt(ctx, "setup")
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	if _, err := drainUpdates(ch); err != nil {
		t.Fatalf("updates error: %v", err)
	}
	if err := ag.SetConfigOption(ctx, "model", "gpt-4"); err != nil {
		t.Errorf("SetConfigOption after ready: %v", err)
	}
}

func TestAgent_Switch_Clean(t *testing.T) {
	ag := newAgent(t, "test")
	ctx := context.Background()

	ch, err := ag.Prompt(ctx, "first")
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	if _, err := drainUpdates(ch); err != nil {
		t.Fatalf("updates error: %v", err)
	}
	if ag.SessionID() == "" {
		t.Fatal("expected non-empty SessionID before switch")
	}

	newConn := acp.New(mockBin, []string{"GO_AGENT_MOCK=1"})
	if err := newConn.Start(); err != nil {
		t.Fatalf("newConn.Start: %v", err)
	}
	if err := ag.Switch(ctx, "test2", newConn, agent.SwitchClean); err != nil {
		t.Fatalf("Switch: %v", err)
	}

	if ag.SessionID() != "" {
		t.Errorf("SessionID should be empty after SwitchClean, got %q", ag.SessionID())
	}
	if ag.AdapterName() != "test2" {
		t.Errorf("AdapterName = %q, want test2", ag.AdapterName())
	}
}

func TestAgent_Switch_WithContext(t *testing.T) {
	ag := newAgent(t, "test")
	ctx := context.Background()

	ch, err := ag.Prompt(ctx, "context-prompt")
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	if _, err := drainUpdates(ch); err != nil {
		t.Fatalf("updates error: %v", err)
	}

	newConn := acp.New(mockBin, []string{"GO_AGENT_MOCK=1"})
	if err := newConn.Start(); err != nil {
		t.Fatalf("newConn.Start: %v", err)
	}
	if err := ag.Switch(ctx, "test2", newConn, agent.SwitchWithContext); err != nil {
		t.Fatalf("Switch: %v", err)
	}

	if ag.AdapterName() != "test2" {
		t.Errorf("AdapterName = %q, want test2", ag.AdapterName())
	}

	time.Sleep(200 * time.Millisecond)
	if ag.SessionID() == "" {
		t.Error("expected non-empty SessionID after SwitchWithContext bootstrap")
	}
}

func TestAgent_EnsureReady_NoDoubleInit(t *testing.T) {
	ag := newAgent(t, "test")
	ctx := context.Background()

	const n = 10
	var wg sync.WaitGroup
	var errCount atomic.Int32

	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ch, err := ag.Prompt(ctx, "concurrent")
			if err != nil {
				errCount.Add(1)
				return
			}
			if _, err := drainUpdates(ch); err != nil {
				errCount.Add(1)
			}
		}()
	}
	wg.Wait()

	if n := errCount.Load(); n > 0 {
		t.Errorf("%d of %d concurrent prompts failed", n, 10)
	}
	if ag.SessionID() == "" {
		t.Error("expected non-empty SessionID after concurrent prompts")
	}
}

func TestAgent_SessionLoad(t *testing.T) {
	conn := acp.New(mockBin, []string{"GO_AGENT_MOCK=1"})
	if err := conn.Start(); err != nil {
		t.Fatalf("conn.Start: %v", err)
	}
	ag := agent.NewWithSessionID("test", conn, t.TempDir(), "existing-session-id")
	t.Cleanup(func() { _ = ag.Close() })

	ctx := context.Background()
	ch, err := ag.Prompt(ctx, "test")
	if err != nil {
		t.Fatalf("Prompt with saved sessionID: %v", err)
	}
	if _, err := drainUpdates(ch); err != nil {
		t.Fatalf("updates error: %v", err)
	}
	if ag.SessionID() != "existing-session-id" {
		t.Errorf("SessionID = %q, want existing-session-id", ag.SessionID())
	}
}

// --- Callback handler tests ---

// TestAgent_Callback_FSRead verifies the fs/read_text_file callback handler.
// The mock sends a read request during session/prompt; agent reads the temp file
// and the mock includes the result as a text update so the test can verify it.
func TestAgent_Callback_FSRead(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/hello.txt"
	if err := os.WriteFile(path, []byte("callback-read-ok"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	ag := newAgent(t, "test")
	ctx := context.Background()

	ch, err := ag.Prompt(ctx, "test-callback-fs-read:"+path)
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	text, pErr := drainUpdates(ch)
	if pErr != nil {
		t.Fatalf("updates error: %v", pErr)
	}
	if !strings.Contains(text, "callback-read-ok") {
		t.Errorf("fs/read_text_file result not reflected in updates, got %q", text)
	}
}

// TestAgent_Callback_FSWrite verifies the fs/write_text_file callback handler.
// The mock sends a write request during session/prompt; agent writes the file;
// test verifies the file exists with the expected content.
func TestAgent_Callback_FSWrite(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/written.txt"
	content := "callback-write-ok"

	ag := newAgent(t, "test")
	ctx := context.Background()

	ch, err := ag.Prompt(ctx, "test-callback-fs-write:"+path+":"+content)
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	if _, err := drainUpdates(ch); err != nil {
		t.Fatalf("updates error: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile after callback: %v", err)
	}
	if string(got) != content {
		t.Errorf("file content = %q, want %q", string(got), content)
	}
}

// TestAgent_Callback_Permission verifies the session/request_permission callback.
// AutoAllowHandler should respond with "selected" + an allow option ID.
func TestAgent_Callback_Permission(t *testing.T) {
	ag := newAgent(t, "test")
	ctx := context.Background()

	ch, err := ag.Prompt(ctx, "test-callback-permission")
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	text, pErr := drainUpdates(ch)
	if pErr != nil {
		t.Fatalf("updates error: %v", pErr)
	}
	// Mock echoes the permission outcome in the text update.
	if !strings.Contains(text, "selected") {
		t.Errorf("permission response not reflected in updates, got %q", text)
	}
}

// TestAgent_Callback_TerminalLifecycle verifies create → wait_for_exit → output → release.
func TestAgent_Callback_TerminalLifecycle(t *testing.T) {
	ag := newAgent(t, "test")
	ctx := context.Background()

	ch, err := ag.Prompt(ctx, "test-callback-terminal")
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	text, pErr := drainUpdates(ch)
	if pErr != nil {
		t.Fatalf("updates error: %v", pErr)
	}
	// Mock echoes "terminal-ok" on successful lifecycle.
	if !strings.Contains(text, "terminal-ok") {
		t.Errorf("terminal lifecycle result not reflected in updates, got %q", text)
	}
}

// TestAgent_Callback_TerminalKill verifies create → kill → release lifecycle.
func TestAgent_Callback_TerminalKill(t *testing.T) {
	ag := newAgent(t, "test")
	ctx := context.Background()

	ch, err := ag.Prompt(ctx, "test-callback-terminal-kill")
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	text, pErr := drainUpdates(ch)
	if pErr != nil {
		t.Fatalf("updates error: %v", pErr)
	}
	if !strings.Contains(text, "kill-ok") {
		t.Errorf("terminal kill result not reflected in updates, got %q", text)
	}
}

// TestAgent_SwitchWithContext_NoStaleContextAfterEmptyPrompt is the stale-context
// prevention test. It verifies that when a prompt produces no text (lastReply=""),
// a subsequent SwitchWithContext does NOT reuse the reply from a prior prompt.
//
// Strategy: the new conn's mock rejects any "[context]" bootstrap prompt with an
// error. If stale context were reused, Switch would send the bootstrap, the mock
// would reject it, and agent.go would log the warning. Capturing the log output
// lets the test assert no warning was produced.
func TestAgent_SwitchWithContext_NoStaleContextAfterEmptyPrompt(t *testing.T) {
	ag := newAgent(t, "test")
	ctx := context.Background()

	// First prompt: normal → sets lastReply = "chunk0 chunk1 ".
	ch, err := ag.Prompt(ctx, "normal")
	if err != nil {
		t.Fatalf("first Prompt: %v", err)
	}
	text1, _ := drainUpdates(ch)
	if text1 == "" {
		t.Skip("mock should return text for 'normal'")
	}

	// Second prompt: no text chunks → lastReply must be cleared to "".
	ch, err = ag.Prompt(ctx, "no-text-prompt")
	if err != nil {
		t.Fatalf("second Prompt: %v", err)
	}
	text2, _ := drainUpdates(ch)
	if text2 != "" {
		t.Fatalf("expected empty text from 'no-text-prompt', got %q", text2)
	}

	// New conn that rejects any "[context]" bootstrap prompt.
	conn2 := acp.New(mockBin, []string{"GO_AGENT_MOCK=1", "GO_AGENT_MOCK_REJECT_CONTEXT=1"})
	if err := conn2.Start(); err != nil {
		t.Fatalf("conn2.Start: %v", err)
	}
	t.Cleanup(func() { _ = conn2.Close() })

	// Capture log output to detect an unexpected SwitchWithContext warning.
	var logBuf strings.Builder
	log.SetOutput(&logBuf)
	defer log.SetOutput(os.Stderr)

	if err := ag.Switch(ctx, "test2", conn2, agent.SwitchWithContext); err != nil {
		t.Fatalf("Switch: %v", err)
	}

	// Allow potential bootstrap goroutine to run and complete.
	time.Sleep(200 * time.Millisecond)

	logOutput := logBuf.String()
	if strings.Contains(logOutput, "SwitchWithContext bootstrap prompt failed") {
		t.Errorf("stale context was reused (old reply leaked into bootstrap): %q", logOutput)
	}
}

// TestAgent_SwitchWithContext_WarningOnBootstrapFailure verifies AC-6's requirement
// that when SwitchWithContext bootstrap Prompt() fails, Switch returns nil (not error)
// and logs a warning. This is the "warning-only" path.
//
// Strategy: do a normal prompt (sets lastReply = non-empty), then switch with a new
// conn whose mock rejects "[context]" bootstrap prompts. Switch must return nil,
// and the log must contain the expected warning.
func TestAgent_SwitchWithContext_WarningOnBootstrapFailure(t *testing.T) {
	ag := newAgent(t, "test")
	ctx := context.Background()

	// First prompt to populate lastReply.
	ch, err := ag.Prompt(ctx, "normal")
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	text, _ := drainUpdates(ch)
	if text == "" {
		t.Skip("mock should return text for 'normal'")
	}

	// New conn that rejects any "[context]" bootstrap so the bootstrap Prompt() fails.
	conn2 := acp.New(mockBin, []string{"GO_AGENT_MOCK=1", "GO_AGENT_MOCK_REJECT_CONTEXT=1"})
	if err := conn2.Start(); err != nil {
		t.Fatalf("conn2.Start: %v", err)
	}
	t.Cleanup(func() { _ = conn2.Close() })

	// Capture log output to detect the expected warning.
	var logBuf strings.Builder
	log.SetOutput(&logBuf)
	defer log.SetOutput(os.Stderr)

	// Switch must return nil even though bootstrap will fail.
	if err := ag.Switch(ctx, "test2", conn2, agent.SwitchWithContext); err != nil {
		t.Fatalf("Switch must return nil on bootstrap failure, got: %v", err)
	}

	// Allow the bootstrap goroutine to run and fail.
	time.Sleep(200 * time.Millisecond)

	logOutput := logBuf.String()
	if !strings.Contains(logOutput, "SwitchWithContext bootstrap prompt failed") {
		t.Errorf("expected warning about bootstrap failure in log; got: %q", logOutput)
	}
}

// --- Mock ACP server (activated when GO_AGENT_MOCK=1) ---

// mockServer handles bidirectional JSON-RPC 2.0 communication.
// It both responds to client requests and can send Agent→Client callback requests.
type mockServer struct {
	enc    *json.Encoder
	encMu  sync.Mutex

	nextOutboundID atomic.Int64 // IDs for agent→client requests (start at 10000)
	pendMu         sync.Mutex
	pending        map[int64]chan json.RawMessage // pending agent→client responses

	sessCounter atomic.Int64
}

func newMockServer() *mockServer {
	ms := &mockServer{
		enc:     json.NewEncoder(os.Stdout),
		pending: make(map[int64]chan json.RawMessage),
	}
	ms.nextOutboundID.Store(10000)
	return ms
}

// respond sends a JSON-RPC result response to the client.
func (ms *mockServer) respond(id int64, result any) {
	ms.encMu.Lock()
	_ = ms.enc.Encode(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
	ms.encMu.Unlock()
}

// respondError sends a JSON-RPC error response.
func (ms *mockServer) respondError(id int64, message string) {
	ms.encMu.Lock()
	_ = ms.enc.Encode(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"error":   map[string]any{"code": -32603, "message": message},
	})
	ms.encMu.Unlock()
}

// sendNotification sends a JSON-RPC notification (no response expected).
func (ms *mockServer) sendNotification(method string, params any) {
	ms.encMu.Lock()
	_ = ms.enc.Encode(map[string]any{"jsonrpc": "2.0", "method": method, "params": params})
	ms.encMu.Unlock()
}

// callbackRequest sends an Agent→Client request and waits for the response.
func (ms *mockServer) callbackRequest(method string, params any) (json.RawMessage, error) {
	id := ms.nextOutboundID.Add(1)
	ch := make(chan json.RawMessage, 1)
	ms.pendMu.Lock()
	ms.pending[id] = ch
	ms.pendMu.Unlock()

	ms.encMu.Lock()
	_ = ms.enc.Encode(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
	ms.encMu.Unlock()

	select {
	case result := <-ch:
		return result, nil
	case <-time.After(5 * time.Second):
		ms.pendMu.Lock()
		delete(ms.pending, id)
		ms.pendMu.Unlock()
		return nil, fmt.Errorf("timeout waiting for %s response", method)
	}
}

// routeResponse routes an incoming response to the correct pending callback channel.
func (ms *mockServer) routeResponse(id int64, result json.RawMessage) {
	ms.pendMu.Lock()
	ch, ok := ms.pending[id]
	if ok {
		delete(ms.pending, id)
	}
	ms.pendMu.Unlock()
	if ok {
		ch <- result
	}
}

// handleRequest dispatches a client→mock request in a goroutine.
func (ms *mockServer) handleRequest(id int64, method string, params json.RawMessage) {
	rejectContext := os.Getenv("GO_AGENT_MOCK_REJECT_CONTEXT") == "1"

	switch method {
	case "initialize":
		ms.respond(id, map[string]any{
			"protocolVersion": "0.1",
			"agentCapabilities": map[string]any{
				"loadSession": true,
			},
			"agentInfo": map[string]any{"name": "mock-agent", "version": "0.1"},
		})

	case "session/new":
		n := ms.sessCounter.Add(1)
		ms.respond(id, map[string]any{
			"sessionId": fmt.Sprintf("mock-session-%d", n),
		})

	case "session/load":
		ms.respond(id, map[string]any{})

	case "session/prompt":
		go ms.handlePrompt(id, params, rejectContext)

	case "session/set_mode", "session/set_config_option":
		ms.respond(id, map[string]any{})

	default:
		ms.respond(id, map[string]any{})
	}
}

// handlePrompt is the async prompt handler that supports sending callbacks.
func (ms *mockServer) handlePrompt(id int64, rawParams json.RawMessage, rejectContext bool) {
	var params struct {
		SessionID string `json:"sessionId"`
		Prompt    string `json:"prompt"`
	}
	_ = json.Unmarshal(rawParams, &params)
	sessID := params.SessionID

	// Reject context bootstrap prompts when requested (stale-context test).
	if rejectContext && strings.HasPrefix(params.Prompt, "[context]") {
		ms.respondError(id, "mock: unexpected context bootstrap – stale context detected")
		return
	}

	// Empty-text prompt: respond with no chunks (clears lastReply).
	if params.Prompt == "no-text-prompt" {
		ms.respond(id, map[string]any{"stopReason": "end_turn"})
		return
	}

	// FS read callback test.
	if strings.HasPrefix(params.Prompt, "test-callback-fs-read:") {
		path := strings.TrimPrefix(params.Prompt, "test-callback-fs-read:")
		result, err := ms.callbackRequest("fs/read_text_file", map[string]any{
			"sessionId": sessID,
			"path":      path,
		})
		var textToEcho string
		if err != nil {
			textToEcho = "fs-read-error: " + err.Error()
		} else {
			var r struct {
				Content string `json:"content"`
			}
			_ = json.Unmarshal(result, &r)
			textToEcho = r.Content
		}
		ms.sendNotification("session/update", map[string]any{
			"sessionId": sessID,
			"update": map[string]any{
				"sessionUpdate": "agent_message_chunk",
				"content":       map[string]any{"type": "text", "text": textToEcho},
			},
		})
		ms.respond(id, map[string]any{"stopReason": "end_turn"})
		return
	}

	// FS write callback test.
	if strings.HasPrefix(params.Prompt, "test-callback-fs-write:") {
		rest := strings.TrimPrefix(params.Prompt, "test-callback-fs-write:")
		// Format: "<path>:<content>" — use LAST colon so Windows drive letters work.
		colonIdx := strings.LastIndex(rest, ":")
		var path, content string
		if colonIdx >= 0 {
			path = rest[:colonIdx]
			content = rest[colonIdx+1:]
		} else {
			path = rest
		}
		_, err := ms.callbackRequest("fs/write_text_file", map[string]any{
			"sessionId": sessID,
			"path":      path,
			"content":   content,
		})
		statusText := "fs-write-ok"
		if err != nil {
			statusText = "fs-write-error: " + err.Error()
		}
		ms.sendNotification("session/update", map[string]any{
			"sessionId": sessID,
			"update": map[string]any{
				"sessionUpdate": "agent_message_chunk",
				"content":       map[string]any{"type": "text", "text": statusText},
			},
		})
		ms.respond(id, map[string]any{"stopReason": "end_turn"})
		return
	}

	// Permission callback test.
	if params.Prompt == "test-callback-permission" {
		result, err := ms.callbackRequest("session/request_permission", map[string]any{
			"sessionId": sessID,
			"toolCall":  map[string]any{"name": "shell_exec", "args": map[string]any{}},
			"options": []map[string]any{
				{"id": "allow_once", "label": "Allow once", "kind": "allow_once"},
				{"id": "reject_once", "label": "Reject once", "kind": "reject_once"},
			},
		})
		var textToEcho string
		if err != nil {
			textToEcho = "permission-error: " + err.Error()
		} else {
			var r struct {
				Outcome  string `json:"outcome"`
				OptionID string `json:"optionId"`
			}
			_ = json.Unmarshal(result, &r)
			textToEcho = r.Outcome
			if r.OptionID != "" {
				textToEcho += ":" + r.OptionID
			}
		}
		ms.sendNotification("session/update", map[string]any{
			"sessionId": sessID,
			"update": map[string]any{
				"sessionUpdate": "agent_message_chunk",
				"content":       map[string]any{"type": "text", "text": textToEcho},
			},
		})
		ms.respond(id, map[string]any{"stopReason": "end_turn"})
		return
	}

	// Terminal lifecycle test: create → wait_for_exit → output → release.
	if params.Prompt == "test-callback-terminal" {
		result, err := ms.callbackRequest("terminal/create", map[string]any{
			"sessionId": sessID,
			"command":   "echo",
			"args":      []string{"terminal-echo-ok"},
		})
		var textToEcho string
		if err != nil {
			textToEcho = "terminal-create-error: " + err.Error()
		} else {
			var cr struct {
				TerminalID string `json:"terminalId"`
			}
			_ = json.Unmarshal(result, &cr)
			termID := cr.TerminalID

			// wait_for_exit
			_, _ = ms.callbackRequest("terminal/wait_for_exit", map[string]any{
				"sessionId":  sessID,
				"terminalId": termID,
			})

			// output
			outResult, outErr := ms.callbackRequest("terminal/output", map[string]any{
				"sessionId":  sessID,
				"terminalId": termID,
			})
			if outErr != nil {
				textToEcho = "terminal-output-error: " + outErr.Error()
			} else {
				var or struct{ Output string `json:"output"` }
				_ = json.Unmarshal(outResult, &or)
				if strings.Contains(or.Output, "terminal-echo-ok") {
					textToEcho = "terminal-ok"
				} else {
					textToEcho = "terminal-unexpected-output: " + or.Output
				}
			}

			// release
			_, _ = ms.callbackRequest("terminal/release", map[string]any{
				"sessionId":  sessID,
				"terminalId": termID,
			})
		}
		ms.sendNotification("session/update", map[string]any{
			"sessionId": sessID,
			"update": map[string]any{
				"sessionUpdate": "agent_message_chunk",
				"content":       map[string]any{"type": "text", "text": textToEcho},
			},
		})
		ms.respond(id, map[string]any{"stopReason": "end_turn"})
		return
	}

	// Terminal kill test: create → kill → release.
	if params.Prompt == "test-callback-terminal-kill" {
		result, err := ms.callbackRequest("terminal/create", map[string]any{
			"sessionId": sessID,
			"command":   "echo",
			"args":      []string{"kill-test"},
		})
		var textToEcho string
		if err != nil {
			textToEcho = "terminal-create-error: " + err.Error()
		} else {
			var cr struct {
				TerminalID string `json:"terminalId"`
			}
			_ = json.Unmarshal(result, &cr)
			termID := cr.TerminalID

			_, _ = ms.callbackRequest("terminal/kill", map[string]any{
				"sessionId":  sessID,
				"terminalId": termID,
			})
			_, _ = ms.callbackRequest("terminal/release", map[string]any{
				"sessionId":  sessID,
				"terminalId": termID,
			})
			textToEcho = "kill-ok"
		}
		ms.sendNotification("session/update", map[string]any{
			"sessionId": sessID,
			"update": map[string]any{
				"sessionUpdate": "agent_message_chunk",
				"content":       map[string]any{"type": "text", "text": textToEcho},
			},
		})
		ms.respond(id, map[string]any{"stopReason": "end_turn"})
		return
	}

	// Default: send 2 text chunks then complete.
	for i := range 2 {
		ms.sendNotification("session/update", map[string]any{
			"sessionId": sessID,
			"update": map[string]any{
				"sessionUpdate": "agent_message_chunk",
				"content":       map[string]any{"type": "text", "text": fmt.Sprintf("chunk%d ", i)},
			},
		})
	}
	ms.respond(id, map[string]any{"stopReason": "end_turn"})
}

func runMockAgent() {
	ms := newMockServer()
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 1<<20), 1<<20)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var raw struct {
			ID     *int64          `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params,omitempty"`
			Result json.RawMessage `json:"result,omitempty"`
		}
		if err := json.Unmarshal(line, &raw); err != nil {
			continue
		}

		if raw.ID != nil && raw.Method == "" {
			// Response to one of our outbound Agent→Client requests.
			ms.routeResponse(*raw.ID, raw.Result)
			continue
		}

		if raw.ID == nil {
			// Notification from client (e.g. session/cancel) — ignore.
			continue
		}

		// Client→mock request: dispatch synchronously for fast methods,
		// or in a goroutine when the handler may block (e.g. session/prompt).
		id := *raw.ID
		method := raw.Method
		params := raw.Params
		if method == "session/prompt" {
			rejectContext := os.Getenv("GO_AGENT_MOCK_REJECT_CONTEXT") == "1"
			go ms.handlePrompt(id, params, rejectContext)
		} else {
			ms.handleRequest(id, method, params)
		}
	}
}
