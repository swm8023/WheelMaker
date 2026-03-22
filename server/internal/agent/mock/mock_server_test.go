package mock_test

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	acp "github.com/swm8023/wheelmaker/internal/acp"
	agentmock "github.com/swm8023/wheelmaker/internal/agent/mock"
)

func newInMemoryMockConn(t *testing.T) *acp.Conn {
	t.Helper()
	a := agentmock.New()
	c, err := a.Connect(context.Background())
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func TestInMemoryMock_PromptCase1_TextAndMetaUpdates(t *testing.T) {
	c := newInMemoryMockConn(t)
	ctx := context.Background()

	var initResult acp.InitializeResult
	if err := c.SendAgent(ctx, "initialize", acp.InitializeParams{ProtocolVersion: 1}, &initResult); err != nil {
		t.Fatalf("initialize: %v", err)
	}

	var newResult acp.SessionNewResult
	if err := c.SendAgent(ctx, "session/new", acp.SessionNewParams{CWD: t.TempDir()}, &newResult); err != nil {
		t.Fatalf("session/new: %v", err)
	}
	if len(newResult.ConfigOptions) == 0 {
		t.Fatalf("expected configOptions in session/new")
	}

	var mu sync.Mutex
	seen := map[string]bool{}
	c.OnRequest(func(_ context.Context, method string, params json.RawMessage, noResponse bool) (any, error) {
		if !noResponse || method != "session/update" {
			return nil, nil
		}
		var p acp.SessionUpdateParams
		if json.Unmarshal(params, &p) != nil {
			return nil, nil
		}
		mu.Lock()
		seen[p.Update.SessionUpdate] = true
		mu.Unlock()
		return nil, nil
	})

	var promptResult acp.SessionPromptResult
	if err := c.SendAgent(ctx, "session/prompt", acp.SessionPromptParams{
		SessionID: newResult.SessionID,
		Prompt:    []acp.ContentBlock{{Type: "text", Text: "1"}},
	}, &promptResult); err != nil {
		t.Fatalf("session/prompt: %v", err)
	}
	if promptResult.StopReason != "end_turn" {
		t.Fatalf("stopReason=%q, want end_turn", promptResult.StopReason)
	}

	time.Sleep(30 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	for _, k := range []string{"agent_message_chunk", "agent_thought_chunk", "plan", "session_info_update", "available_commands_update"} {
		if !seen[k] {
			t.Fatalf("missing update type: %s", k)
		}
	}
}

func TestInMemoryMock_GlobalConfigCommand(t *testing.T) {
	c := newInMemoryMockConn(t)
	ctx := context.Background()
	var _init acp.InitializeResult
	if err := c.SendAgent(ctx, "initialize", acp.InitializeParams{ProtocolVersion: 1}, &_init); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	var newResult acp.SessionNewResult
	if err := c.SendAgent(ctx, "session/new", acp.SessionNewParams{CWD: t.TempDir()}, &newResult); err != nil {
		t.Fatalf("session/new: %v", err)
	}

	var gotConfigUpdate bool
	c.OnRequest(func(_ context.Context, method string, params json.RawMessage, noResponse bool) (any, error) {
		if !noResponse || method != "session/update" {
			return nil, nil
		}
		var p acp.SessionUpdateParams
		if json.Unmarshal(params, &p) != nil {
			return nil, nil
		}
		if p.Update.SessionUpdate == "config_option_update" && len(p.Update.ConfigOptions) > 0 {
			gotConfigUpdate = true
		}
		return nil, nil
	})

	var promptResult acp.SessionPromptResult
	if err := c.SendAgent(ctx, "session/prompt", acp.SessionPromptParams{
		SessionID: newResult.SessionID,
		Prompt:    []acp.ContentBlock{{Type: "text", Text: "/model gpt-4.1-mini"}},
	}, &promptResult); err != nil {
		t.Fatalf("session/prompt: %v", err)
	}
	if !gotConfigUpdate {
		t.Fatalf("expected config_option_update from /model command")
	}
}

func TestInMemoryMock_CallbackCases(t *testing.T) {
	c := newInMemoryMockConn(t)
	ctx := context.Background()
	var _init acp.InitializeResult
	if err := c.SendAgent(ctx, "initialize", acp.InitializeParams{ProtocolVersion: 1}, &_init); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	var newResult acp.SessionNewResult
	if err := c.SendAgent(ctx, "session/new", acp.SessionNewParams{CWD: t.TempDir()}, &newResult); err != nil {
		t.Fatalf("session/new: %v", err)
	}

	c.OnRequest(func(_ context.Context, method string, _ json.RawMessage, noResponse bool) (any, error) {
		if noResponse {
			return nil, nil
		}
		switch method {
		case "fs/read_text_file":
			return acp.FSReadTextFileResult{Content: "fs-ok"}, nil
		case "fs/write_text_file":
			return map[string]any{}, nil
		case "terminal/create":
			return acp.TerminalCreateResult{TerminalID: "term-1"}, nil
		case "terminal/output":
			return acp.TerminalOutputResult{Output: "terminal-ok", Truncated: false}, nil
		case "terminal/wait_for_exit":
			code := 0
			return acp.TerminalWaitForExitResult{ExitCode: &code}, nil
		case "terminal/release":
			return map[string]any{}, nil
		case "session/request_permission":
			return acp.PermissionResponse{Outcome: acp.PermissionResult{Outcome: "selected", OptionID: "allow_once"}}, nil
		default:
			return nil, nil
		}
	})

	for _, prompt := range []string{"2", "3", "4"} {
		var result acp.SessionPromptResult
		if err := c.SendAgent(ctx, "session/prompt", acp.SessionPromptParams{
			SessionID: newResult.SessionID,
			Prompt:    []acp.ContentBlock{{Type: "text", Text: prompt}},
		}, &result); err != nil {
			t.Fatalf("prompt %s: %v", prompt, err)
		}
		if result.StopReason == "" {
			t.Fatalf("prompt %s returned empty stopReason", prompt)
		}
	}
}

func TestInMemoryMock_PermissionRequestsUserChoice(t *testing.T) {
	c := newInMemoryMockConn(t)
	ctx := context.Background()

	var initResult acp.InitializeResult
	if err := c.SendAgent(ctx, "initialize", acp.InitializeParams{ProtocolVersion: 1}, &initResult); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	var newResult acp.SessionNewResult
	if err := c.SendAgent(ctx, "session/new", acp.SessionNewParams{CWD: t.TempDir()}, &newResult); err != nil {
		t.Fatalf("session/new: %v", err)
	}

	var (
		mu                sync.Mutex
		permissionAsked   bool
		allowOptionSeen   bool
		rejectOptionSeen  bool
		finalToolStatus   string
		finalMessageChunk string
	)

	c.OnRequest(func(_ context.Context, method string, params json.RawMessage, noResponse bool) (any, error) {
		if noResponse {
			if method != "session/update" {
				return nil, nil
			}
			var p acp.SessionUpdateParams
			if json.Unmarshal(params, &p) != nil {
				return nil, nil
			}
			if p.Update.SessionUpdate == "tool_call" || p.Update.SessionUpdate == "tool_call_update" {
				mu.Lock()
				finalToolStatus = p.Update.Status
				mu.Unlock()
			}
			if p.Update.SessionUpdate == "agent_message_chunk" && p.Update.Content != nil {
				var cb acp.ContentBlock
				if json.Unmarshal(p.Update.Content, &cb) == nil {
					mu.Lock()
					finalMessageChunk = cb.Text
					mu.Unlock()
				}
			}
			return nil, nil
		}

		if method != "session/request_permission" {
			return nil, nil
		}

		var p acp.PermissionRequestParams
		if err := json.Unmarshal(params, &p); err != nil {
			t.Fatalf("unmarshal permission params: %v", err)
		}

		mu.Lock()
		permissionAsked = true
		for _, opt := range p.Options {
			if opt.OptionID == "allow_once" {
				allowOptionSeen = true
			}
			if opt.OptionID == "reject_once" {
				rejectOptionSeen = true
			}
		}
		mu.Unlock()

		return acp.PermissionResponse{
			Outcome: acp.PermissionResult{Outcome: "selected", OptionID: "reject_once"},
		}, nil
	})

	var promptResult acp.SessionPromptResult
	if err := c.SendAgent(ctx, "session/prompt", acp.SessionPromptParams{
		SessionID: newResult.SessionID,
		Prompt:    []acp.ContentBlock{{Type: "text", Text: "4"}},
	}, &promptResult); err != nil {
		t.Fatalf("session/prompt: %v", err)
	}
	if promptResult.StopReason != "end_turn" {
		t.Fatalf("stopReason=%q, want end_turn", promptResult.StopReason)
	}

	time.Sleep(30 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if !permissionAsked {
		t.Fatalf("expected session/request_permission to be called")
	}
	if !allowOptionSeen || !rejectOptionSeen {
		t.Fatalf("expected permission options to include allow_once and reject_once")
	}
	if finalToolStatus != "failed" {
		t.Fatalf("tool_call final status=%q, want failed", finalToolStatus)
	}
	if finalMessageChunk != "permission:rejected" {
		t.Fatalf("final permission message=%q, want permission:rejected", finalMessageChunk)
	}
}

func TestInMemoryMock_ErrorInjection(t *testing.T) {
	c := newInMemoryMockConn(t)
	ctx := context.Background()
	var _init acp.InitializeResult
	if err := c.SendAgent(ctx, "initialize", acp.InitializeParams{ProtocolVersion: 1}, &_init); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	var newResult acp.SessionNewResult
	if err := c.SendAgent(ctx, "session/new", acp.SessionNewParams{CWD: t.TempDir()}, &newResult); err != nil {
		t.Fatalf("session/new: %v", err)
	}

	for _, tc := range []struct {
		input string
		code  string
	}{{input: "10", code: "-32602"}, {input: "11", code: "-32601"}, {input: "12", code: "-32603"}} {
		var result acp.SessionPromptResult
		err := c.SendAgent(ctx, "session/prompt", acp.SessionPromptParams{SessionID: newResult.SessionID, Prompt: []acp.ContentBlock{{Type: "text", Text: tc.input}}}, &result)
		if err == nil {
			t.Fatalf("prompt %s: expected rpc error", tc.input)
		}
		if !strings.Contains(err.Error(), tc.code) {
			t.Fatalf("prompt %s: expected rpc code %s in err=%v", tc.input, tc.code, err)
		}
	}
}

// TestInMemoryMock_SessionList verifies that session/list returns sessions created via session/new
// with the correct sessionId and cwd fields (§4.4).
func TestInMemoryMock_SessionList(t *testing.T) {
	c := newInMemoryMockConn(t)
	ctx := context.Background()

	var initResult acp.InitializeResult
	if err := c.SendAgent(ctx, "initialize", acp.InitializeParams{ProtocolVersion: 1}, &initResult); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	// Capability must be declared.
	if initResult.AgentCapabilities.SessionCapabilities == nil || initResult.AgentCapabilities.SessionCapabilities.List == nil {
		t.Fatal("expected agentCapabilities.sessionCapabilities.list to be present")
	}

	cwd1, cwd2 := t.TempDir(), t.TempDir()
	var s1, s2 acp.SessionNewResult
	if err := c.SendAgent(ctx, "session/new", acp.SessionNewParams{CWD: cwd1}, &s1); err != nil {
		t.Fatalf("session/new 1: %v", err)
	}
	if err := c.SendAgent(ctx, "session/new", acp.SessionNewParams{CWD: cwd2}, &s2); err != nil {
		t.Fatalf("session/new 2: %v", err)
	}

	var listResult acp.SessionListResult
	if err := c.SendAgent(ctx, "session/list", acp.SessionListParams{}, &listResult); err != nil {
		t.Fatalf("session/list: %v", err)
	}
	if len(listResult.Sessions) < 2 {
		t.Fatalf("expected at least 2 sessions, got %d", len(listResult.Sessions))
	}
	seen := map[string]bool{}
	for _, s := range listResult.Sessions {
		if s.SessionID == "" {
			t.Fatal("session entry has empty sessionId")
		}
		if s.CWD == "" {
			t.Fatalf("session %q has empty cwd (required per §4.4)", s.SessionID)
		}
		seen[s.SessionID] = true
	}
	if !seen[s1.SessionID] || !seen[s2.SessionID] {
		t.Fatalf("expected sessions %q and %q in list", s1.SessionID, s2.SessionID)
	}
}

// TestInMemoryMock_SessionList_CWDFilter verifies that the optional CWD filter
// in session/list returns only matching sessions (§4.4).
func TestInMemoryMock_SessionList_CWDFilter(t *testing.T) {
	c := newInMemoryMockConn(t)
	ctx := context.Background()

	if err := c.SendAgent(ctx, "initialize", acp.InitializeParams{ProtocolVersion: 1}, &acp.InitializeResult{}); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	cwd1 := t.TempDir()
	var s1 acp.SessionNewResult
	if err := c.SendAgent(ctx, "session/new", acp.SessionNewParams{CWD: cwd1}, &s1); err != nil {
		t.Fatalf("session/new 1: %v", err)
	}
	if err := c.SendAgent(ctx, "session/new", acp.SessionNewParams{CWD: t.TempDir()}, &acp.SessionNewResult{}); err != nil {
		t.Fatalf("session/new 2: %v", err)
	}

	var filtered acp.SessionListResult
	if err := c.SendAgent(ctx, "session/list", acp.SessionListParams{CWD: cwd1}, &filtered); err != nil {
		t.Fatalf("session/list with cwd filter: %v", err)
	}
	if len(filtered.Sessions) != 1 {
		t.Fatalf("expected 1 session for cwd filter, got %d", len(filtered.Sessions))
	}
	if filtered.Sessions[0].SessionID != s1.SessionID {
		t.Fatalf("filtered session ID=%q, want %q", filtered.Sessions[0].SessionID, s1.SessionID)
	}
}

// TestInMemoryMock_SessionLoad_HistoryReplay verifies that session/load sends session/update
// notifications (history replay) before the null response (§4.3).
func TestInMemoryMock_SessionLoad_HistoryReplay(t *testing.T) {
	c := newInMemoryMockConn(t)
	ctx := context.Background()

	if err := c.SendAgent(ctx, "initialize", acp.InitializeParams{ProtocolVersion: 1}, &acp.InitializeResult{}); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	var newResult acp.SessionNewResult
	if err := c.SendAgent(ctx, "session/new", acp.SessionNewParams{CWD: t.TempDir()}, &newResult); err != nil {
		t.Fatalf("session/new: %v", err)
	}

	// Collect session/update notifications during session/load.
	// Per §4.3, notifications are delivered synchronously before SendAgent returns.
	var historyTypes []string
	c.OnRequest(func(_ context.Context, method string, params json.RawMessage, noResponse bool) (any, error) {
		if !noResponse || method != "session/update" {
			return nil, nil
		}
		var p acp.SessionUpdateParams
		if json.Unmarshal(params, &p) == nil {
			historyTypes = append(historyTypes, p.Update.SessionUpdate)
		}
		return nil, nil
	})

	var loadResult acp.SessionLoadResult
	if err := c.SendAgent(ctx, "session/load", acp.SessionLoadParams{
		SessionID:  newResult.SessionID,
		CWD:        t.TempDir(),
		MCPServers: nil,
	}, &loadResult); err != nil {
		t.Fatalf("session/load: %v", err)
	}

	// Must have received at least one history update before the null response.
	if len(historyTypes) == 0 {
		t.Fatal("expected history replay session/update notifications from session/load")
	}
	// config_option_update should always be present in replay (§4.3).
	found := false
	for _, u := range historyTypes {
		if u == "config_option_update" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected config_option_update in history replay, got: %v", historyTypes)
	}
}

// TestInMemoryMock_SessionLoad_HistoryReplayWithTitle verifies that session_info_update
// is included in history replay when the session previously received a title (§4.3, §4.4).
func TestInMemoryMock_SessionLoad_HistoryReplayWithTitle(t *testing.T) {
	c := newInMemoryMockConn(t)
	ctx := context.Background()

	if err := c.SendAgent(ctx, "initialize", acp.InitializeParams{ProtocolVersion: 1}, &acp.InitializeResult{}); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	var newResult acp.SessionNewResult
	if err := c.SendAgent(ctx, "session/new", acp.SessionNewParams{CWD: t.TempDir()}, &newResult); err != nil {
		t.Fatalf("session/new: %v", err)
	}
	// Prompt "1" sends a session_info_update and persists the title.
	if err := c.SendAgent(ctx, "session/prompt", acp.SessionPromptParams{
		SessionID: newResult.SessionID,
		Prompt:    []acp.ContentBlock{{Type: "text", Text: "1"}},
	}, &acp.SessionPromptResult{}); err != nil {
		t.Fatalf("session/prompt 1: %v", err)
	}

	var historyTypes []string
	c.OnRequest(func(_ context.Context, method string, params json.RawMessage, noResponse bool) (any, error) {
		if !noResponse || method != "session/update" {
			return nil, nil
		}
		var p acp.SessionUpdateParams
		if json.Unmarshal(params, &p) == nil {
			historyTypes = append(historyTypes, p.Update.SessionUpdate)
		}
		return nil, nil
	})

	if err := c.SendAgent(ctx, "session/load", acp.SessionLoadParams{
		SessionID: newResult.SessionID,
		CWD:       t.TempDir(),
	}, &acp.SessionLoadResult{}); err != nil {
		t.Fatalf("session/load: %v", err)
	}

	found := false
	for _, u := range historyTypes {
		if u == "session_info_update" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected session_info_update in history replay when title is set, got: %v", historyTypes)
	}
}

// TestInMemoryMock_ToolCallDiffContent verifies the full tool call lifecycle
// (pending→in_progress→completed) with diff-type content (§7.1-7.3).
func TestInMemoryMock_ToolCallDiffContent(t *testing.T) {
	c := newInMemoryMockConn(t)
	ctx := context.Background()

	if err := c.SendAgent(ctx, "initialize", acp.InitializeParams{ProtocolVersion: 1}, &acp.InitializeResult{}); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	var newResult acp.SessionNewResult
	if err := c.SendAgent(ctx, "session/new", acp.SessionNewParams{CWD: t.TempDir()}, &newResult); err != nil {
		t.Fatalf("session/new: %v", err)
	}

	var mu sync.Mutex
	var toolCallStatuses []string
	var diffSeen bool

	c.OnRequest(func(_ context.Context, method string, params json.RawMessage, noResponse bool) (any, error) {
		if !noResponse || method != "session/update" {
			return nil, nil
		}
		var p acp.SessionUpdateParams
		if json.Unmarshal(params, &p) != nil {
			return nil, nil
		}
		upd := p.Update
		if upd.SessionUpdate != "tool_call" && upd.SessionUpdate != "tool_call_update" {
			return nil, nil
		}
		mu.Lock()
		toolCallStatuses = append(toolCallStatuses, upd.Status)
		// Protocol uses "content" key for ToolCallContent array (§7.2).
		if upd.Status == "completed" && upd.Content != nil {
			var items []map[string]any
			if json.Unmarshal(upd.Content, &items) == nil {
				for _, item := range items {
					if item["type"] == "diff" {
						diffSeen = true
					}
				}
			}
		}
		mu.Unlock()
		return nil, nil
	})

	var result acp.SessionPromptResult
	if err := c.SendAgent(ctx, "session/prompt", acp.SessionPromptParams{
		SessionID: newResult.SessionID,
		Prompt:    []acp.ContentBlock{{Type: "text", Text: "5"}},
	}, &result); err != nil {
		t.Fatalf("prompt 5: %v", err)
	}
	if result.StopReason != "end_turn" {
		t.Fatalf("stopReason=%q, want end_turn", result.StopReason)
	}

	time.Sleep(30 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()

	// Expect lifecycle: pending (tool_call) → in_progress (update) → completed (update).
	if len(toolCallStatuses) < 3 {
		t.Fatalf("expected 3 tool call status events, got %d: %v", len(toolCallStatuses), toolCallStatuses)
	}
	if toolCallStatuses[0] != "pending" {
		t.Fatalf("first status=%q, want pending", toolCallStatuses[0])
	}
	if toolCallStatuses[len(toolCallStatuses)-1] != "completed" {
		t.Fatalf("final status=%q, want completed", toolCallStatuses[len(toolCallStatuses)-1])
	}
	if !diffSeen {
		t.Fatal("expected diff-type ToolCallContent in completed tool_call_update (§7.3)")
	}
}

// TestInMemoryMock_ToolCallTerminalContent verifies the tool call flow that embeds
// a terminal reference so the client can render live output (§7.3, §9).
func TestInMemoryMock_ToolCallTerminalContent(t *testing.T) {
	c := newInMemoryMockConn(t)
	ctx := context.Background()

	if err := c.SendAgent(ctx, "initialize", acp.InitializeParams{ProtocolVersion: 1}, &acp.InitializeResult{}); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	var newResult acp.SessionNewResult
	if err := c.SendAgent(ctx, "session/new", acp.SessionNewParams{CWD: t.TempDir()}, &newResult); err != nil {
		t.Fatalf("session/new: %v", err)
	}

	var mu sync.Mutex
	var terminalIDSeen string
	var toolCallFinalStatus string

	c.OnRequest(func(_ context.Context, method string, params json.RawMessage, noResponse bool) (any, error) {
		if noResponse && method == "session/update" {
			var p acp.SessionUpdateParams
			if json.Unmarshal(params, &p) == nil {
				upd := p.Update
				if upd.SessionUpdate == "tool_call" || upd.SessionUpdate == "tool_call_update" {
					mu.Lock()
					toolCallFinalStatus = upd.Status
					// Look for terminal content in the "content" array field.
					if upd.Content != nil {
						var items []map[string]any
						if json.Unmarshal(upd.Content, &items) == nil {
							for _, item := range items {
								if item["type"] == "terminal" {
									if tid, ok := item["terminalId"].(string); ok {
										terminalIDSeen = tid
									}
								}
							}
						}
					}
					mu.Unlock()
				}
			}
			return nil, nil
		}
		switch method {
		case "terminal/create":
			return acp.TerminalCreateResult{TerminalID: "term-test-6"}, nil
		case "terminal/wait_for_exit":
			code := 0
			return acp.TerminalWaitForExitResult{ExitCode: &code}, nil
		case "terminal/release":
			return nil, nil
		}
		return nil, nil
	})

	var result acp.SessionPromptResult
	if err := c.SendAgent(ctx, "session/prompt", acp.SessionPromptParams{
		SessionID: newResult.SessionID,
		Prompt:    []acp.ContentBlock{{Type: "text", Text: "6"}},
	}, &result); err != nil {
		t.Fatalf("prompt 6: %v", err)
	}
	if result.StopReason != "end_turn" {
		t.Fatalf("stopReason=%q, want end_turn", result.StopReason)
	}

	time.Sleep(30 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()

	if terminalIDSeen == "" {
		t.Fatal("expected terminal-type content in tool_call_update (§7.3)")
	}
	if toolCallFinalStatus != "completed" {
		t.Fatalf("final tool call status=%q, want completed", toolCallFinalStatus)
	}
}

// TestInMemoryMock_TerminalKill verifies the kill flow:
// create→in_progress→kill→output→release (§9 timeout pattern).
func TestInMemoryMock_TerminalKill(t *testing.T) {
	c := newInMemoryMockConn(t)
	ctx := context.Background()

	if err := c.SendAgent(ctx, "initialize", acp.InitializeParams{ProtocolVersion: 1}, &acp.InitializeResult{}); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	var newResult acp.SessionNewResult
	if err := c.SendAgent(ctx, "session/new", acp.SessionNewParams{CWD: t.TempDir()}, &newResult); err != nil {
		t.Fatalf("session/new: %v", err)
	}

	var mu sync.Mutex
	var killCalled bool
	var toolCallFinalStatus string

	c.OnRequest(func(_ context.Context, method string, params json.RawMessage, noResponse bool) (any, error) {
		if noResponse && method == "session/update" {
			var p acp.SessionUpdateParams
			if json.Unmarshal(params, &p) == nil && (p.Update.SessionUpdate == "tool_call" || p.Update.SessionUpdate == "tool_call_update") {
				mu.Lock()
				toolCallFinalStatus = p.Update.Status
				mu.Unlock()
			}
			return nil, nil
		}
		switch method {
		case "terminal/create":
			return acp.TerminalCreateResult{TerminalID: "term-kill"}, nil
		case "terminal/kill":
			mu.Lock()
			killCalled = true
			mu.Unlock()
			return nil, nil
		case "terminal/output":
			return acp.TerminalOutputResult{Output: "", Truncated: false}, nil
		case "terminal/release":
			return nil, nil
		}
		return nil, nil
	})

	var result acp.SessionPromptResult
	if err := c.SendAgent(ctx, "session/prompt", acp.SessionPromptParams{
		SessionID: newResult.SessionID,
		Prompt:    []acp.ContentBlock{{Type: "text", Text: "7"}},
	}, &result); err != nil {
		t.Fatalf("prompt 7: %v", err)
	}
	if result.StopReason != "end_turn" {
		t.Fatalf("stopReason=%q, want end_turn", result.StopReason)
	}

	time.Sleep(30 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()

	if !killCalled {
		t.Fatal("expected terminal/kill to be called in kill flow (§9)")
	}
	if toolCallFinalStatus != "completed" {
		t.Fatalf("final tool call status=%q, want completed", toolCallFinalStatus)
	}
}

// TestInMemoryMock_MaxTokensStopReason verifies that the mock can signal
// max_tokens as the stop reason (§5.4).
func TestInMemoryMock_MaxTokensStopReason(t *testing.T) {
	c := newInMemoryMockConn(t)
	ctx := context.Background()

	if err := c.SendAgent(ctx, "initialize", acp.InitializeParams{ProtocolVersion: 1}, &acp.InitializeResult{}); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	var newResult acp.SessionNewResult
	if err := c.SendAgent(ctx, "session/new", acp.SessionNewParams{CWD: t.TempDir()}, &newResult); err != nil {
		t.Fatalf("session/new: %v", err)
	}

	var result acp.SessionPromptResult
	if err := c.SendAgent(ctx, "session/prompt", acp.SessionPromptParams{
		SessionID: newResult.SessionID,
		Prompt:    []acp.ContentBlock{{Type: "text", Text: "8"}},
	}, &result); err != nil {
		t.Fatalf("prompt 8: %v", err)
	}
	if result.StopReason != "max_tokens" {
		t.Fatalf("stopReason=%q, want max_tokens", result.StopReason)
	}
}

// TestInMemoryMock_ResourceLinkContent verifies that the agent can reply
// with a resource_link content block (§6, §5.3).
func TestInMemoryMock_ResourceLinkContent(t *testing.T) {
	c := newInMemoryMockConn(t)
	ctx := context.Background()

	if err := c.SendAgent(ctx, "initialize", acp.InitializeParams{ProtocolVersion: 1}, &acp.InitializeResult{}); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	var newResult acp.SessionNewResult
	if err := c.SendAgent(ctx, "session/new", acp.SessionNewParams{CWD: t.TempDir()}, &newResult); err != nil {
		t.Fatalf("session/new: %v", err)
	}

	var mu sync.Mutex
	var resourceLinkSeen bool

	c.OnRequest(func(_ context.Context, method string, params json.RawMessage, noResponse bool) (any, error) {
		if !noResponse || method != "session/update" {
			return nil, nil
		}
		var p acp.SessionUpdateParams
		if json.Unmarshal(params, &p) != nil {
			return nil, nil
		}
		if p.Update.SessionUpdate == "agent_message_chunk" && p.Update.Content != nil {
			var cb acp.ContentBlock
			if json.Unmarshal(p.Update.Content, &cb) == nil && cb.Type == "resource_link" {
				mu.Lock()
				resourceLinkSeen = true
				mu.Unlock()
			}
		}
		return nil, nil
	})

	var result acp.SessionPromptResult
	if err := c.SendAgent(ctx, "session/prompt", acp.SessionPromptParams{
		SessionID: newResult.SessionID,
		Prompt:    []acp.ContentBlock{{Type: "text", Text: "9"}},
	}, &result); err != nil {
		t.Fatalf("prompt 9: %v", err)
	}
	if result.StopReason != "end_turn" {
		t.Fatalf("stopReason=%q, want end_turn", result.StopReason)
	}

	time.Sleep(30 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()

	if !resourceLinkSeen {
		t.Fatal("expected resource_link type in agent_message_chunk (§6)")
	}
}

// TestInMemoryMock_SetConfigOption verifies that session/set_config_option
// returns the complete updated config option list (§10.2).
func TestInMemoryMock_SetConfigOption(t *testing.T) {
	c := newInMemoryMockConn(t)
	ctx := context.Background()

	if err := c.SendAgent(ctx, "initialize", acp.InitializeParams{ProtocolVersion: 1}, &acp.InitializeResult{}); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	var newResult acp.SessionNewResult
	if err := c.SendAgent(ctx, "session/new", acp.SessionNewParams{CWD: t.TempDir()}, &newResult); err != nil {
		t.Fatalf("session/new: %v", err)
	}

	// Change model; agent must respond with complete list including updated currentValue.
	var raw json.RawMessage
	if err := c.SendAgent(ctx, "session/set_config_option", acp.SessionSetConfigOptionParams{
		SessionID: newResult.SessionID,
		ConfigID:  "model",
		Value:     "gpt-4.1-mini",
	}, &raw); err != nil {
		t.Fatalf("session/set_config_option: %v", err)
	}

	// Response may be array or {"configOptions":[...]} — both are valid (§10.2).
	var opts []acp.ConfigOption
	if err := json.Unmarshal(raw, &opts); err != nil {
		var wrapped struct {
			ConfigOptions []acp.ConfigOption `json:"configOptions"`
		}
		if err2 := json.Unmarshal(raw, &wrapped); err2 != nil {
			t.Fatalf("set_config_option response is neither array nor {configOptions:[...]}: %s", raw)
		}
		opts = wrapped.ConfigOptions
	}
	if len(opts) == 0 {
		t.Fatal("expected non-empty config options in set_config_option response (§10.2)")
	}
	var modelOpt *acp.ConfigOption
	for i := range opts {
		if opts[i].ID == "model" {
			modelOpt = &opts[i]
		}
	}
	if modelOpt == nil {
		t.Fatal("model configOption missing from response")
	}
	if modelOpt.CurrentValue != "gpt-4.1-mini" {
		t.Fatalf("model currentValue=%q, want gpt-4.1-mini", modelOpt.CurrentValue)
	}
}
