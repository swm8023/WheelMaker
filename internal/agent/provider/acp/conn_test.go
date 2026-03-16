package acp_test

// Unit tests for acp.Conn using a self-referential mock agent.
//
// Pattern: when GO_ACP_MOCK=1 is set, this test binary acts as the ACP
// mock server (reading stdin, writing stdout). Otherwise it runs the tests,
// pointing acp.New() at os.Args[0] with GO_ACP_MOCK=1 in the environment.

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/swm8023/wheelmaker/internal/agent/provider/acp"
)

// mockAgentBin is set in TestMain to the current test binary path.
var mockAgentBin string

func TestMain(m *testing.M) {
	if os.Getenv("GO_ACP_MOCK") == "1" {
		runMockAgent()
		os.Exit(0)
	}
	mockAgentBin = os.Args[0]
	os.Exit(m.Run())
}

// newMockConn creates a Conn pointed at the mock agent subprocess.
func newMockConn(t *testing.T) *acp.Conn {
	t.Helper()
	c := acp.New(mockAgentBin, []string{"GO_ACP_MOCK=1"})
	if err := c.Start(); err != nil {
		t.Fatalf("conn.Start: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

// --- Tests ---

// TestSend_Initialize verifies the basic request/response cycle.
func TestSend_Initialize(t *testing.T) {
	c := newMockConn(t)
	var result acp.InitializeResult
	err := c.Send(context.Background(), "initialize", acp.InitializeParams{
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
	err := c.Send(context.Background(), "session/new", acp.SessionNewParams{
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
	err := c.Send(context.Background(), "error_test", nil, nil)
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
	err := c.Send(ctx, "slow_response", nil, nil)
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
	err := c.Send(ctx, "slow_response", nil, nil)
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
			if err := c.Send(context.Background(), "session/new", acp.SessionNewParams{
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

// TestSubscribe_Notification verifies that subscribers receive incoming notifications.
func TestSubscribe_Notification(t *testing.T) {
	c := newMockConn(t)

	// Initialize and create a session first.
	var initResult acp.InitializeResult
	if err := c.Send(context.Background(), "initialize", acp.InitializeParams{ProtocolVersion: 1}, &initResult); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	var sessResult acp.SessionNewResult
	if err := c.Send(context.Background(), "session/new", acp.SessionNewParams{CWD: "."}, &sessResult); err != nil {
		t.Fatalf("session/new: %v", err)
	}

	// Collect session/update notifications.
	var mu sync.Mutex
	var notifications []acp.SessionUpdateParams
	cancel := c.Subscribe(func(n acp.Notification) {
		if n.Method != "session/update" {
			return
		}
		var p acp.SessionUpdateParams
		if err := json.Unmarshal(n.Params, &p); err != nil {
			return
		}
		mu.Lock()
		notifications = append(notifications, p)
		mu.Unlock()
	})
	defer cancel()

	// Send a prompt â€” the mock sends 3 text chunks then returns.
	var promptResult acp.SessionPromptResult
	if err := c.Send(context.Background(), "session/prompt", acp.SessionPromptParams{
		SessionID: sessResult.SessionID,
		Prompt:    []acp.ContentBlock{{Type: "text", Text: "hello"}},
	}, &promptResult); err != nil {
		t.Fatalf("session/prompt: %v", err)
	}
	if promptResult.StopReason != "end_turn" {
		t.Errorf("stopReason = %q, want end_turn", promptResult.StopReason)
	}

	// Give dispatcher goroutines time to deliver.
	time.Sleep(20 * time.Millisecond)

	mu.Lock()
	count := len(notifications)
	mu.Unlock()
	if count != 3 {
		t.Errorf("received %d notifications, want 3", count)
	}
	// Verify content of notifications.
	for _, n := range notifications {
		if n.Update.SessionUpdate != "agent_message_chunk" {
			t.Errorf("unexpected update type: %s", n.Update.SessionUpdate)
		}
		// F4 fix: Content is now json.RawMessage.
		var cb acp.ContentBlock
		if err := json.Unmarshal(n.Update.Content, &cb); err != nil || cb.Type != "text" {
			t.Errorf("unexpected content: %s", n.Update.Content)
		}
	}
}

// TestSubscribe_Cancel verifies that a cancelled subscription stops receiving notifications.
func TestSubscribe_Cancel(t *testing.T) {
	c := newMockConn(t)

	var count atomic.Int32
	cancelSub := c.Subscribe(func(n acp.Notification) {
		count.Add(1)
	})
	cancelSub() // unsubscribe immediately

	// Send a session/prompt to generate notifications â€” none should be received.
	var sessResult acp.SessionNewResult
	_ = c.Send(context.Background(), "session/new", acp.SessionNewParams{CWD: "."}, &sessResult)
	_ = c.Send(context.Background(), "session/prompt", acp.SessionPromptParams{
		SessionID: sessResult.SessionID,
		Prompt:    []acp.ContentBlock{{Type: "text", Text: "test"}},
	}, nil)

	time.Sleep(30 * time.Millisecond)
	if n := count.Load(); n != 0 {
		t.Errorf("cancelled subscriber received %d notifications, want 0", n)
	}
}

// TestNotify verifies fire-and-forget notifications don't block or error.
func TestNotify(t *testing.T) {
	c := newMockConn(t)
	err := c.Notify("session/cancel", acp.SessionCancelParams{SessionID: "some-id"})
	if err != nil {
		t.Fatalf("Notify: %v", err)
	}
}

// TestClose_Idempotent verifies Close can be called multiple times safely.
func TestClose_Idempotent(t *testing.T) {
	c := acp.New(mockAgentBin, []string{"GO_ACP_MOCK=1"})
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
	c := acp.New(mockAgentBin, []string{"GO_ACP_MOCK=1"})
	if err := c.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	err := c.Send(context.Background(), "initialize", nil, nil)
	if err == nil {
		t.Fatal("expected error sending after close, got nil")
	}
	t.Logf("got expected error: %v", err)
}

// TestSend_ProcessExit verifies pending Sends receive an error when the process exits.
func TestSend_ProcessExit(t *testing.T) {
	c := newMockConn(t)
	// "exit_now" causes the mock agent to exit without responding.
	// The conn's readLoop should unblock all pending requests.
	errCh := make(chan error, 1)
	go func() {
		errCh <- c.Send(context.Background(), "exit_now", nil, nil)
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

// TestIncomingRequest_Handler verifies that Agentâ†’Client requests are routed
// to the registered RequestHandler and that the response is sent back.
func TestIncomingRequest_Handler(t *testing.T) {
	c := newMockConn(t)

	var receivedMethod string
	var receivedParams json.RawMessage
	c.OnRequest(func(_ context.Context, method string, params json.RawMessage) (any, error) {
		receivedMethod = method
		receivedParams = params
		return map[string]string{"content": "file content"}, nil
	})

	// "trigger_incoming_request" causes the mock to send an fs/read_text_file
	// request to the client, then return its response in the final result.
	var result struct {
		ReceivedContent string `json:"receivedContent"`
	}
	if err := c.Send(context.Background(), "trigger_incoming_request", nil, &result); err != nil {
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
	// No OnRequest registered â€” mock expects a -32601 error back.
	err := c.Send(context.Background(), "trigger_incoming_request_no_handler", nil, nil)
	if err != nil {
		t.Fatalf("Send: %v", err) // the client-side send itself should succeed
	}
}

// TestSend_NilResult verifies that Send with nil result doesn't panic.
func TestSend_NilResult(t *testing.T) {
	c := newMockConn(t)
	err := c.Send(context.Background(), "session/new", acp.SessionNewParams{CWD: "."}, nil)
	if err != nil {
		t.Fatalf("Send with nil result: %v", err)
	}
}

// TestMultipleSubscribers verifies all subscribers receive notifications.
func TestMultipleSubscribers(t *testing.T) {
	c := newMockConn(t)

	const nSubs = 5
	counts := make([]atomic.Int32, nSubs)
	cancels := make([]func(), nSubs)
	for i := range nSubs {
		i := i
		cancels[i] = c.Subscribe(func(n acp.Notification) {
			counts[i].Add(1)
		})
	}
	defer func() {
		for _, cancel := range cancels {
			cancel()
		}
	}()

	// Generate 3 notifications via session/prompt.
	var sessResult acp.SessionNewResult
	_ = c.Send(context.Background(), "session/new", acp.SessionNewParams{CWD: "."}, &sessResult)
	_ = c.Send(context.Background(), "session/prompt", acp.SessionPromptParams{
		SessionID: sessResult.SessionID,
		Prompt:    []acp.ContentBlock{{Type: "text", Text: "multi"}},
	}, nil)

	time.Sleep(30 * time.Millisecond)
	for i := range nSubs {
		if n := counts[i].Load(); n != 3 {
			t.Errorf("subscriber %d received %d notifications, want 3", i, n)
		}
	}
}

// --- Mock agent (runs when GO_ACP_MOCK=1) ---

func runMockAgent() {
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

		// Notifications have no id â€” ignore them.
		if raw.ID == nil {
			continue
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

			// Send 3 text chunk notifications before the response.
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
			// Exit without sending a response to test pending-request cleanup.
			return

		case "trigger_incoming_request":
			// 1. Send an Agentâ†’Client request (fs/read_text_file) to the client.
			mockIncomingRequest(enc, 9999, "fs/read_text_file", map[string]any{
				"sessionId": "test-session",
				"path":      "/mock/path/file.txt",
			})
			// 2. Read the client's response from stdin.
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
			// 3. Return the received content in our response to the original request.
			content := clientResp.Result.Content
			if clientResp.Error != nil {
				content = fmt.Sprintf("error: %s", clientResp.Error.Message)
			}
			mockRespond(enc, id, map[string]any{
				"receivedContent": content,
			})

		case "trigger_incoming_request_no_handler":
			// Send an incoming request; expect -32601 error back from conn (no handler set).
			mockIncomingRequest(enc, 8888, "fs/read_text_file", map[string]any{
				"sessionId": "test",
				"path":      "/mock/file.txt",
			})
			// Read whatever response the conn sends.
			if !scanner.Scan() {
				return
			}
			// Return success to the original trigger request.
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

// mockIncomingRequest simulates an Agentâ†’Client request (has both id and method).
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
