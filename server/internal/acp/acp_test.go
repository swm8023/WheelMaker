package acp_test

// acp_test.go — unit tests for acp.Conn and ACP normalization helpers.
//
// Self-referential mock pattern: when GO_ACP_MOCK=1 is set, this test binary
// acts as the ACP mock server (reads stdin, writes stdout). Otherwise it runs
// the test suite, pointing acp.NewConn() at os.Args[0] with GO_ACP_MOCK=1.
//
// TestMain handles both modes, so the binary doubles as mock agent and test runner.

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	acp "github.com/swm8023/wheelmaker/internal/acp"
)

// mockAgentBin is set in TestMain to the current test binary path.
var mockAgentBin string

func TestMain(m *testing.M) {
	if os.Getenv("GO_ACP_MOCK") == "1" {
		runConnMockAgent()
		os.Exit(0)
	}
	mockAgentBin = os.Args[0]
	os.Exit(m.Run())
}

// newMockConn creates a Conn pointed at the mock agent subprocess.
func newMockConn(t *testing.T) *acp.Conn {
	t.Helper()
	c := acp.NewConn(mockAgentBin, []string{"GO_ACP_MOCK=1"})
	if err := c.Start(); err != nil {
		t.Fatalf("conn.Start: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

// ── Conn tests ────────────────────────────────────────────────────────────────

// TestSend_Initialize verifies the basic request/response cycle.
func TestSend_Initialize(t *testing.T) {
	c := newMockConn(t)
	var result acp.InitializeResult
	err := c.SendAgent(context.Background(), "initialize", acp.InitializeParams{
		ProtocolVersion: 1,
		ClientInfo:      &acp.AgentInfo{Name: "test", Version: "0"},
	}, &result)
	if err != nil {
		t.Fatalf("Send initialize: %v", err)
	}
	if result.ProtocolVersion != "0.1" {
		t.Errorf("protocolVersion = %q, want 0.1", result.ProtocolVersion)
	}
	if !result.AgentCapabilities.LoadSession {
		t.Error("expected agentCapabilities.loadSession = true")
	}
	if result.AgentInfo == nil || result.AgentInfo.Name != "mock-agent" {
		t.Errorf("agentInfo.name = %v, want mock-agent", result.AgentInfo)
	}
}

// TestSend_SessionNew verifies session/new returns a sessionId.
func TestSend_SessionNew(t *testing.T) {
	c := newMockConn(t)
	var result acp.SessionNewResult
	err := c.SendAgent(context.Background(), "session/new", acp.SessionNewParams{
		CWD:        "/tmp",
		MCPServers: []acp.MCPServer{},
	}, &result)
	if err != nil {
		t.Fatalf("Send session/new: %v", err)
	}
	if result.SessionID == "" {
		t.Error("expected non-empty sessionId")
	}
}

// TestSend_RPCError verifies JSON-RPC error responses are returned as errors.
func TestSend_RPCError(t *testing.T) {
	c := newMockConn(t)
	err := c.SendAgent(context.Background(), "error_test", nil, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	t.Logf("got expected error: %v", err)
}

// TestSend_ContextCancel verifies that Send returns ctx.Err() when cancelled.
func TestSend_ContextCancel(t *testing.T) {
	c := newMockConn(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately
	err := c.SendAgent(ctx, "slow_response", nil, nil)
	if err == nil {
		t.Fatal("expected error from cancelled context, got nil")
	}
	t.Logf("got expected error: %v", err)
}

// TestSend_ContextTimeout verifies that Send returns error on timeout.
func TestSend_ContextTimeout(t *testing.T) {
	c := newMockConn(t)
	// slow_response waits 200ms in mock; set a 50ms timeout
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err := c.SendAgent(ctx, "slow_response", nil, nil)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	t.Logf("got expected error: %v", err)
}

// TestSend_Concurrent verifies multiple simultaneous requests are routed correctly.
func TestSend_Concurrent(t *testing.T) {
	c := newMockConn(t)
	const n = 20
	var wg sync.WaitGroup
	var errCount atomic.Int32
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			var result acp.SessionNewResult
			if err := c.SendAgent(context.Background(), "session/new", acp.SessionNewParams{
				CWD: fmt.Sprintf("/tmp/session-%d", i),
			}, &result); err != nil {
				t.Errorf("goroutine %d: %v", i, err)
				errCount.Add(1)
			} else if result.SessionID == "" {
				t.Errorf("goroutine %d: empty sessionId", i)
				errCount.Add(1)
			}
		}(i)
	}
	wg.Wait()
	if errCount.Load() > 0 {
		t.Fatalf("%d concurrent sends failed", errCount.Load())
	}
}

// TestOnRequest_Notification verifies that the OnRequest handler receives incoming notifications.
func TestOnRequest_Notification(t *testing.T) {
	c := newMockConn(t)

	var initResult acp.InitializeResult
	if err := c.SendAgent(context.Background(), "initialize", acp.InitializeParams{ProtocolVersion: 1}, &initResult); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	var sessResult acp.SessionNewResult
	if err := c.SendAgent(context.Background(), "session/new", acp.SessionNewParams{CWD: "."}, &sessResult); err != nil {
		t.Fatalf("session/new: %v", err)
	}

	var mu sync.Mutex
	var notifications []acp.SessionUpdateParams
	c.OnRequest(func(_ context.Context, method string, params json.RawMessage, noResponse bool) (any, error) {
		if noResponse && method == "session/update" {
			var p acp.SessionUpdateParams
			if err := json.Unmarshal(params, &p); err != nil {
				return nil, nil
			}
			mu.Lock()
			notifications = append(notifications, p)
			mu.Unlock()
		}
		if !noResponse {
			return nil, fmt.Errorf("unsupported method: %s", method)
		}
		return nil, nil
	})

	var promptResult acp.SessionPromptResult
	if err := c.SendAgent(context.Background(), "session/prompt", acp.SessionPromptParams{
		SessionID: sessResult.SessionID,
		Prompt:    []acp.ContentBlock{{Type: "text", Text: "hello"}},
	}, &promptResult); err != nil {
		t.Fatalf("session/prompt: %v", err)
	}
	if promptResult.StopReason != "end_turn" {
		t.Errorf("stopReason = %q, want end_turn", promptResult.StopReason)
	}

	mu.Lock()
	count := len(notifications)
	mu.Unlock()
	if count != 3 {
		t.Errorf("received %d notifications, want 3", count)
	}
	for _, n := range notifications {
		if n.Update.SessionUpdate != "agent_message_chunk" {
			t.Errorf("unexpected update type: %s", n.Update.SessionUpdate)
		}
		var cb acp.ContentBlock
		if err := json.Unmarshal(n.Update.Content, &cb); err != nil || cb.Type != "text" {
			t.Errorf("unexpected content: %s", n.Update.Content)
		}
	}
}

// TestOnRequest_NilHandler verifies that notifications are silently dropped and
// Send still succeeds when no OnRequest handler is registered.
func TestOnRequest_NilHandler(t *testing.T) {
	c := newMockConn(t)

	var sessResult acp.SessionNewResult
	_ = c.SendAgent(context.Background(), "session/new", acp.SessionNewParams{CWD: "."}, &sessResult)
	err := c.SendAgent(context.Background(), "session/prompt", acp.SessionPromptParams{
		SessionID: sessResult.SessionID,
		Prompt:    []acp.ContentBlock{{Type: "text", Text: "test"}},
	}, nil)
	if err != nil {
		t.Fatalf("session/prompt with nil handler: %v", err)
	}
}

// TestNotify verifies fire-and-forget notifications don't block or error.
func TestNotify(t *testing.T) {
	c := newMockConn(t)
	err := c.NotifyAgent("session/cancel", acp.SessionCancelParams{SessionID: "some-id"})
	if err != nil {
		t.Fatalf("Notify: %v", err)
	}
}

func TestNotify_DebugLogger(t *testing.T) {
	var dbg bytes.Buffer
	c := acp.NewInMemoryConn(func(r io.Reader, _ io.Writer) {
		scanner := bufio.NewScanner(r)
		if scanner.Scan() {
			return
		}
	})
	c.SetDebugLogger(&dbg)
	if err := c.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer c.Close()

	if err := c.NotifyAgent("session/cancel", acp.SessionCancelParams{SessionID: "sess-1"}); err != nil {
		t.Fatalf("Notify: %v", err)
	}
	got := dbg.String()
	if !strings.Contains(got, "session/cancel") {
		t.Fatalf("debug log = %q, want session/cancel", got)
	}
}

// TestClose_Idempotent verifies Close can be called multiple times safely.
func TestClose_Idempotent(t *testing.T) {
	c := acp.NewConn(mockAgentBin, []string{"GO_ACP_MOCK=1"})
	if err := c.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

// TestSend_AfterClose verifies Send returns error after the conn is closed.
func TestSend_AfterClose(t *testing.T) {
	c := acp.NewConn(mockAgentBin, []string{"GO_ACP_MOCK=1"})
	if err := c.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	err := c.SendAgent(context.Background(), "initialize", nil, nil)
	if err == nil {
		t.Fatal("expected error sending after close, got nil")
	}
	t.Logf("got expected error: %v", err)
}

// TestSend_ProcessExit verifies pending Sends receive an error when the process exits.
func TestSend_ProcessExit(t *testing.T) {
	c := newMockConn(t)
	errCh := make(chan error, 1)
	go func() {
		errCh <- c.SendAgent(context.Background(), "exit_now", nil, nil)
	}()
	select {
	case err := <-errCh:
		if err == nil {
			t.Error("expected error after process exit, got nil")
		}
		t.Logf("got expected error: %v", err)
	case <-time.After(3 * time.Second):
		t.Error("Send did not return after process exit")
	}
}

// TestIncomingRequest_Handler verifies that Agent→Client requests are routed
// to the registered RequestHandler and that the response is sent back.
func TestIncomingRequest_Handler(t *testing.T) {
	c := newMockConn(t)

	var receivedMethod string
	var receivedParams json.RawMessage
	c.OnRequest(func(_ context.Context, method string, params json.RawMessage, noResponse bool) (any, error) {
		if noResponse {
			return nil, nil
		}
		receivedMethod = method
		receivedParams = params
		return map[string]string{"content": "file content"}, nil
	})

	var result struct {
		ReceivedContent string `json:"receivedContent"`
	}
	if err := c.SendAgent(context.Background(), "trigger_incoming_request", nil, &result); err != nil {
		t.Fatalf("Send trigger_incoming_request: %v", err)
	}

	if receivedMethod != "fs/read_text_file" {
		t.Errorf("handler got method %q, want fs/read_text_file", receivedMethod)
	}
	if receivedParams == nil {
		t.Error("handler received nil params")
	}
	if result.ReceivedContent != "file content" {
		t.Errorf("round-trip content = %q, want 'file content'", result.ReceivedContent)
	}
}

// TestIncomingRequest_NoHandler verifies that without a handler, the conn
// sends a -32601 method-not-found error back to the agent.
func TestIncomingRequest_NoHandler(t *testing.T) {
	c := newMockConn(t)
	err := c.SendAgent(context.Background(), "trigger_incoming_request_no_handler", nil, nil)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
}

// TestSend_NilResult verifies that Send with nil result doesn't panic.
func TestSend_NilResult(t *testing.T) {
	c := newMockConn(t)
	err := c.SendAgent(context.Background(), "session/new", acp.SessionNewParams{CWD: "."}, nil)
	if err != nil {
		t.Fatalf("Send with nil result: %v", err)
	}
}

// TestNotificationHandling verifies OnRequest receives session/update notifications.
func TestNotificationHandling(t *testing.T) {
	c := newMockConn(t)

	var count atomic.Int32
	c.OnRequest(func(_ context.Context, method string, _ json.RawMessage, noResponse bool) (any, error) {
		if noResponse && method == "session/update" {
			count.Add(1)
		}
		return nil, nil
	})

	var sessResult acp.SessionNewResult
	_ = c.SendAgent(context.Background(), "session/new", acp.SessionNewParams{CWD: "."}, &sessResult)
	_ = c.SendAgent(context.Background(), "session/prompt", acp.SessionPromptParams{
		SessionID: sessResult.SessionID,
		Prompt:    []acp.ContentBlock{{Type: "text", Text: "multi"}},
	}, nil)

	time.Sleep(30 * time.Millisecond)
	if n := count.Load(); n != 3 {
		t.Errorf("notification count = %d, want 3", n)
	}
}

// ── Normalize tests ───────────────────────────────────────────────────────────

func TestNormalizeNotificationParams_CurrentModeUpdate(t *testing.T) {
	in := json.RawMessage(`{
		"sessionId":"sess-1",
		"update":{"sessionUpdate":"current_mode_update","modeId":"code"}
	}`)

	out := acp.NormalizeNotificationParams(acp.MethodSessionUpdate, in)

	var p acp.SessionUpdateParams
	if err := json.Unmarshal(out, &p); err != nil {
		t.Fatalf("unmarshal normalized payload: %v", err)
	}
	if p.Update.SessionUpdate != "config_option_update" {
		t.Fatalf("sessionUpdate=%q, want config_option_update", p.Update.SessionUpdate)
	}
	if len(p.Update.ConfigOptions) != 1 {
		t.Fatalf("configOptions len=%d, want 1", len(p.Update.ConfigOptions))
	}
	if p.Update.ConfigOptions[0].ID != "mode" {
		t.Fatalf("configOptions[0].id=%q, want mode", p.Update.ConfigOptions[0].ID)
	}
	if p.Update.ConfigOptions[0].CurrentValue != "code" {
		t.Fatalf("configOptions[0].currentValue=%q, want code", p.Update.ConfigOptions[0].CurrentValue)
	}
}

func TestNormalizeNotificationParams_PassThrough(t *testing.T) {
	in := json.RawMessage(`{"sessionId":"sess-1","update":{"sessionUpdate":"agent_message_chunk"}}`)
	out := acp.NormalizeNotificationParams(acp.MethodSessionUpdate, in)
	if string(out) != string(in) {
		t.Fatalf("unexpected payload rewrite: got %s want %s", out, in)
	}
}

func TestNormalizeNotificationParams_NonUpdateMethod(t *testing.T) {
	in := json.RawMessage(`{"foo":"bar"}`)
	out := acp.NormalizeNotificationParams("session/cancel", in)
	if string(out) != string(in) {
		t.Fatalf("non-update method should pass through: got %s", out)
	}
}

func TestNormalizeNotificationParams_EmptyParams(t *testing.T) {
	out := acp.NormalizeNotificationParams(acp.MethodSessionUpdate, nil)
	if out != nil {
		t.Fatalf("nil params should return nil, got %s", out)
	}
	out = acp.NormalizeNotificationParams(acp.MethodSessionUpdate, json.RawMessage{})
	if len(out) != 0 {
		t.Fatalf("empty params should return empty, got %s", out)
	}
}

func TestNormalizeNotificationParams_MalformedJSON(t *testing.T) {
	in := json.RawMessage(`{not valid json`)
	out := acp.NormalizeNotificationParams(acp.MethodSessionUpdate, in)
	if string(out) != string(in) {
		t.Fatalf("malformed JSON should pass through: got %s", out)
	}
}

func TestNormalizeNotificationParams_MissingModeID(t *testing.T) {
	in := json.RawMessage(`{
		"sessionId":"sess-1",
		"update":{"sessionUpdate":"current_mode_update","modeId":""}
	}`)
	out := acp.NormalizeNotificationParams(acp.MethodSessionUpdate, in)
	if string(out) != string(in) {
		t.Fatalf("empty modeId should pass through: got %s", out)
	}
}

func TestNormalizeNotificationParams_ExistingConfigOptions(t *testing.T) {
	in := json.RawMessage(`{
		"sessionId":"sess-1",
		"update":{
			"sessionUpdate":"current_mode_update",
			"modeId":"code",
			"configOptions":[{"id":"custom","currentValue":"x"}]
		}
	}`)
	out := acp.NormalizeNotificationParams(acp.MethodSessionUpdate, in)

	var p acp.SessionUpdateParams
	if err := json.Unmarshal(out, &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if p.Update.SessionUpdate != "config_option_update" {
		t.Fatalf("sessionUpdate=%q, want config_option_update", p.Update.SessionUpdate)
	}
	if len(p.Update.ConfigOptions) != 1 || p.Update.ConfigOptions[0].ID != "custom" {
		t.Fatalf("existing configOptions should be preserved, got %+v", p.Update.ConfigOptions)
	}
}

func TestNormalizeNotificationParams_NoUpdateField(t *testing.T) {
	in := json.RawMessage(`{"sessionId":"sess-1","other":"value"}`)
	out := acp.NormalizeNotificationParams(acp.MethodSessionUpdate, in)
	if string(out) != string(in) {
		t.Fatalf("no update field should pass through: got %s", out)
	}
}

// ── Mock agent (runs when GO_ACP_MOCK=1) ─────────────────────────────────────

func runConnMockAgent() {
	enc := json.NewEncoder(os.Stdout)
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 1<<20), 1<<20)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var raw struct {
			JSONRPC string          `json:"jsonrpc"`
			ID      *int64          `json:"id"`
			Method  string          `json:"method"`
			Params  json.RawMessage `json:"params,omitempty"`
		}
		if err := json.Unmarshal(line, &raw); err != nil {
			continue
		}

		if raw.ID == nil {
			continue // notifications — ignore
		}
		id := *raw.ID

		switch raw.Method {
		case "initialize":
			mockRespond(enc, id, map[string]any{
				"protocolVersion": "0.1",
				"agentCapabilities": map[string]any{
					"loadSession": true,
				},
				"agentInfo": map[string]any{
					"name":    "mock-agent",
					"version": "0.1",
				},
			})

		case "session/new", "session/load":
			mockRespond(enc, id, map[string]any{
				"sessionId": "mock-session-abc123",
			})

		case "session/prompt":
			var params struct {
				SessionID string          `json:"sessionId"`
				Prompt    json.RawMessage `json:"prompt"`
			}
			_ = json.Unmarshal(raw.Params, &params)

			for i := range 3 {
				mockNotify(enc, "session/update", map[string]any{
					"sessionId": params.SessionID,
					"update": map[string]any{
						"sessionUpdate": "agent_message_chunk",
						"content": map[string]any{
							"type": "text",
							"text": fmt.Sprintf("chunk%d ", i),
						},
					},
				})
			}
			mockRespond(enc, id, map[string]any{
				"stopReason": "end_turn",
			})

		case "error_test":
			mockError(enc, id, -32600, "intentional test error")

		case "slow_response":
			time.Sleep(200 * time.Millisecond)
			mockRespond(enc, id, map[string]any{"ok": true})

		case "exit_now":
			return

		case "trigger_incoming_request":
			mockIncomingRequest(enc, 9999, "fs/read_text_file", map[string]any{
				"sessionId": "test-session",
				"path":      "/mock/path/file.txt",
			})
			if !scanner.Scan() {
				return
			}
			var clientResp struct {
				ID     int64 `json:"id"`
				Result struct {
					Content string `json:"content"`
				} `json:"result"`
				Error *struct {
					Code    int    `json:"code"`
					Message string `json:"message"`
				} `json:"error"`
			}
			_ = json.Unmarshal(scanner.Bytes(), &clientResp)
			content := clientResp.Result.Content
			if clientResp.Error != nil {
				content = fmt.Sprintf("error: %s", clientResp.Error.Message)
			}
			mockRespond(enc, id, map[string]any{
				"receivedContent": content,
			})

		case "trigger_incoming_request_no_handler":
			mockIncomingRequest(enc, 8888, "fs/read_text_file", map[string]any{
				"sessionId": "test",
				"path":      "/mock/file.txt",
			})
			if !scanner.Scan() {
				return
			}
			mockRespond(enc, id, map[string]any{})

		default:
			mockRespond(enc, id, map[string]any{})
		}
	}
}

func mockRespond(enc *json.Encoder, id int64, result any) {
	_ = enc.Encode(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"result":  result,
	})
}

func mockError(enc *json.Encoder, id int64, code int, message string) {
	_ = enc.Encode(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"error": map[string]any{
			"code":    code,
			"message": message,
		},
	})
}

func mockIncomingRequest(enc *json.Encoder, id int64, method string, params any) {
	_ = enc.Encode(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
		"params":  params,
	})
}

func mockNotify(enc *json.Encoder, method string, params any) {
	_ = enc.Encode(map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
	})
}
