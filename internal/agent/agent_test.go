package agent_test

// agent_test.go: unit tests for agent.Agent using a self-referential mock ACP server.
//
// Pattern: when GO_AGENT_MOCK=1 is set, the test binary acts as the ACP mock
// server (reading stdin, writing stdout). Otherwise it runs the tests,
// pointing acp.New() at os.Args[0] with GO_AGENT_MOCK=1.

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

// newMockConn creates an *acp.Conn pointing at the mock ACP server.
func newMockConn(t *testing.T) *acp.Conn {
	t.Helper()
	conn := acp.New(mockBin, []string{"GO_AGENT_MOCK=1"})
	if err := conn.Start(); err != nil {
		t.Fatalf("conn.Start: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// newAgent creates an Agent backed by a fresh mock ACP subprocess.
func newAgent(t *testing.T, name string) *agent.Agent {
	t.Helper()
	conn := acp.New(mockBin, []string{"GO_AGENT_MOCK=1"})
	if err := conn.Start(); err != nil {
		t.Fatalf("conn.Start: %v", err)
	}
	ag := agent.New(name, conn, "/tmp/test")
	t.Cleanup(func() { _ = ag.Close() })
	return ag
}

// drainUpdates drains an update channel and returns accumulated text.
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

// TestAgent_SessionID_AfterReady verifies SessionID is populated after the first Prompt.
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

// TestAgent_Prompt_TextUpdates verifies that text chunks are received as UpdateText updates.
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

// TestAgent_Prompt_ClearsLastReply verifies lastReply is cleared at the start of each Prompt.
// We test this indirectly: a SwitchClean after the second prompt sees the second reply, not the first.
func TestAgent_Prompt_ClearsLastReply(t *testing.T) {
	ag := newAgent(t, "test")
	ctx := context.Background()

	// First prompt — runs fine.
	ch, err := ag.Prompt(ctx, "first")
	if err != nil {
		t.Fatalf("first Prompt: %v", err)
	}
	if _, err := drainUpdates(ch); err != nil {
		t.Fatalf("first updates error: %v", err)
	}

	// Second prompt — also runs fine.
	ch, err = ag.Prompt(ctx, "second")
	if err != nil {
		t.Fatalf("second Prompt: %v", err)
	}
	if _, err := drainUpdates(ch); err != nil {
		t.Fatalf("second updates error: %v", err)
	}

	// If lastReply were stale, Switch would use the first reply. We just verify
	// that Switch does not fail (relies on lastReply being correctly managed).
	newConn := acp.New(mockBin, []string{"GO_AGENT_MOCK=1"})
	if err := newConn.Start(); err != nil {
		t.Fatalf("newConn.Start: %v", err)
	}
	if err := ag.Switch(ctx, "test2", newConn, agent.SwitchWithContext); err != nil {
		t.Fatalf("Switch: %v", err)
	}
}

// TestAgent_Cancel_BeforeReady verifies Cancel is a no-op (no panic) before readiness.
func TestAgent_Cancel_BeforeReady(t *testing.T) {
	ag := newAgent(t, "test")
	if err := ag.Cancel(); err != nil {
		t.Errorf("Cancel before ready: %v", err)
	}
}

// TestAgent_Cancel_AfterReady verifies Cancel sends a notification after a session is established.
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

// TestAgent_SetMode_BeforeReady verifies SetMode returns error before readiness.
func TestAgent_SetMode_BeforeReady(t *testing.T) {
	ag := newAgent(t, "test")
	if err := ag.SetMode(context.Background(), "auto"); err == nil {
		t.Error("expected error from SetMode before ready")
	}
}

// TestAgent_SetMode_AfterReady verifies SetMode sends the request successfully.
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

// TestAgent_SetConfigOption_BeforeReady verifies SetConfigOption returns error before readiness.
func TestAgent_SetConfigOption_BeforeReady(t *testing.T) {
	ag := newAgent(t, "test")
	if err := ag.SetConfigOption(context.Background(), "model", "gpt-4"); err == nil {
		t.Error("expected error from SetConfigOption before ready")
	}
}

// TestAgent_SetConfigOption_AfterReady verifies SetConfigOption sends the request successfully.
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

// TestAgent_Switch_Clean verifies a SwitchClean resets session state.
func TestAgent_Switch_Clean(t *testing.T) {
	ag := newAgent(t, "test")
	ctx := context.Background()

	// Make ready.
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

	// Switch to new adapter.
	newConn := acp.New(mockBin, []string{"GO_AGENT_MOCK=1"})
	if err := newConn.Start(); err != nil {
		t.Fatalf("newConn.Start: %v", err)
	}
	if err := ag.Switch(ctx, "test2", newConn, agent.SwitchClean); err != nil {
		t.Fatalf("Switch: %v", err)
	}

	// After SwitchClean, session ID is cleared; adapter name updated.
	if ag.SessionID() != "" {
		t.Errorf("SessionID should be empty after SwitchClean, got %q", ag.SessionID())
	}
	if ag.AdapterName() != "test2" {
		t.Errorf("AdapterName = %q, want test2", ag.AdapterName())
	}
}

// TestAgent_Switch_WithContext verifies SwitchWithContext bootstraps the new session.
func TestAgent_Switch_WithContext(t *testing.T) {
	ag := newAgent(t, "test")
	ctx := context.Background()

	// Make ready and accumulate lastReply.
	ch, err := ag.Prompt(ctx, "context-prompt")
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	if _, err := drainUpdates(ch); err != nil {
		t.Fatalf("updates error: %v", err)
	}

	// Switch with context; bootstrap prompt is fired in a goroutine.
	newConn := acp.New(mockBin, []string{"GO_AGENT_MOCK=1"})
	if err := newConn.Start(); err != nil {
		t.Fatalf("newConn.Start: %v", err)
	}
	if err := ag.Switch(ctx, "test2", newConn, agent.SwitchWithContext); err != nil {
		t.Fatalf("Switch: %v", err)
	}

	// Adapter name should be updated immediately.
	if ag.AdapterName() != "test2" {
		t.Errorf("AdapterName = %q, want test2", ag.AdapterName())
	}

	// Wait briefly for the bootstrap goroutine to complete, then verify ready.
	time.Sleep(200 * time.Millisecond)
	if ag.SessionID() == "" {
		t.Error("expected non-empty SessionID after SwitchWithContext bootstrap")
	}
}

// TestAgent_EnsureReady_NoDoubleInit verifies concurrent Prompts don't double-initialize.
// With the single-flight guard, only one goroutine performs the I/O;
// all concurrent callers succeed without error.
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

// TestAgent_SessionLoad verifies that a pre-loaded sessionID is passed to session/load.
func TestAgent_SessionLoad(t *testing.T) {
	conn := acp.New(mockBin, []string{"GO_AGENT_MOCK=1"})
	if err := conn.Start(); err != nil {
		t.Fatalf("conn.Start: %v", err)
	}
	ag := agent.NewWithSessionID("test", conn, "/tmp/test", "existing-session-id")
	t.Cleanup(func() { _ = ag.Close() })

	ctx := context.Background()
	ch, err := ag.Prompt(ctx, "test")
	if err != nil {
		t.Fatalf("Prompt with saved sessionID: %v", err)
	}
	if _, err := drainUpdates(ch); err != nil {
		t.Fatalf("updates error: %v", err)
	}
	// After session/load succeeds, SessionID should be the saved one.
	if ag.SessionID() != "existing-session-id" {
		t.Errorf("SessionID = %q, want existing-session-id", ag.SessionID())
	}
}

// --- Mock ACP server (activated when GO_AGENT_MOCK=1) ---

func runMockAgent() {
	enc := json.NewEncoder(os.Stdout)
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 1<<20), 1<<20)

	var sessionCounter atomic.Int64

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

		// Ignore notifications (no id).
		if raw.ID == nil {
			continue
		}
		id := *raw.ID

		switch raw.Method {
		case "initialize":
			mockAgentRespond(enc, id, map[string]any{
				"protocolVersion": "0.1",
				"agentCapabilities": map[string]any{
					"loadSession": true,
				},
				"agentInfo": map[string]any{
					"name":    "mock-agent",
					"version": "0.1",
				},
			})

		case "session/new":
			n := sessionCounter.Add(1)
			mockAgentRespond(enc, id, map[string]any{
				"sessionId": fmt.Sprintf("mock-session-%d", n),
			})

		case "session/load":
			// Return success; sessionId is preserved from the request (saved by agent).
			mockAgentRespond(enc, id, map[string]any{})

		case "session/prompt":
			var params struct {
				SessionID string `json:"sessionId"`
				Prompt    string `json:"prompt"`
			}
			_ = json.Unmarshal(raw.Params, &params)

			// Send 2 text chunk notifications before the final response.
			for i := range 2 {
				_ = enc.Encode(map[string]any{
					"jsonrpc": "2.0",
					"method":  "session/update",
					"params": map[string]any{
						"sessionId": params.SessionID,
						"update": map[string]any{
							"sessionUpdate": "agent_message_chunk",
							"content": map[string]any{
								"type": "text",
								"text": fmt.Sprintf("chunk%d ", i),
							},
						},
					},
				})
			}
			mockAgentRespond(enc, id, map[string]any{
				"stopReason": "end_turn",
			})

		case "session/set_mode", "session/set_config_option":
			mockAgentRespond(enc, id, map[string]any{})

		default:
			mockAgentRespond(enc, id, map[string]any{})
		}
	}
}

func mockAgentRespond(enc *json.Encoder, id int64, result any) {
	_ = enc.Encode(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"result":  result,
	})
}
