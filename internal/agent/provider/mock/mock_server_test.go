package mock_test

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/swm8023/wheelmaker/internal/agent/provider/acp"
	mockprovider "github.com/swm8023/wheelmaker/internal/agent/provider/mock"
)

func newInMemoryMockConn(t *testing.T) *acp.Conn {
	t.Helper()
	a := mockprovider.NewAdapter()
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
	if err := c.Send(ctx, "initialize", acp.InitializeParams{ProtocolVersion: 1}, &initResult); err != nil {
		t.Fatalf("initialize: %v", err)
	}

	var newResult acp.SessionNewResult
	if err := c.Send(ctx, "session/new", acp.SessionNewParams{CWD: t.TempDir()}, &newResult); err != nil {
		t.Fatalf("session/new: %v", err)
	}
	if len(newResult.ConfigOptions) == 0 {
		t.Fatalf("expected configOptions in session/new")
	}

	var mu sync.Mutex
	seen := map[string]bool{}
	cancel := c.Subscribe(func(n acp.Notification) {
		if n.Method != "session/update" {
			return
		}
		var p acp.SessionUpdateParams
		if json.Unmarshal(n.Params, &p) != nil {
			return
		}
		mu.Lock()
		seen[p.Update.SessionUpdate] = true
		mu.Unlock()
	})
	defer cancel()

	var promptResult acp.SessionPromptResult
	if err := c.Send(ctx, "session/prompt", acp.SessionPromptParams{
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
	if err := c.Send(ctx, "initialize", acp.InitializeParams{ProtocolVersion: 1}, &_init); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	var newResult acp.SessionNewResult
	if err := c.Send(ctx, "session/new", acp.SessionNewParams{CWD: t.TempDir()}, &newResult); err != nil {
		t.Fatalf("session/new: %v", err)
	}

	var gotConfigUpdate bool
	cancel := c.Subscribe(func(n acp.Notification) {
		if n.Method != "session/update" {
			return
		}
		var p acp.SessionUpdateParams
		if json.Unmarshal(n.Params, &p) != nil {
			return
		}
		if p.Update.SessionUpdate == "config_option_update" && len(p.Update.ConfigOptions) > 0 {
			gotConfigUpdate = true
		}
	})
	defer cancel()

	var promptResult acp.SessionPromptResult
	if err := c.Send(ctx, "session/prompt", acp.SessionPromptParams{
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
	if err := c.Send(ctx, "initialize", acp.InitializeParams{ProtocolVersion: 1}, &_init); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	var newResult acp.SessionNewResult
	if err := c.Send(ctx, "session/new", acp.SessionNewParams{CWD: t.TempDir()}, &newResult); err != nil {
		t.Fatalf("session/new: %v", err)
	}

	c.OnRequest(func(_ context.Context, method string, _ json.RawMessage) (any, error) {
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
		if err := c.Send(ctx, "session/prompt", acp.SessionPromptParams{
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

func TestInMemoryMock_ErrorInjection(t *testing.T) {
	c := newInMemoryMockConn(t)
	ctx := context.Background()
	var _init acp.InitializeResult
	if err := c.Send(ctx, "initialize", acp.InitializeParams{ProtocolVersion: 1}, &_init); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	var newResult acp.SessionNewResult
	if err := c.Send(ctx, "session/new", acp.SessionNewParams{CWD: t.TempDir()}, &newResult); err != nil {
		t.Fatalf("session/new: %v", err)
	}

	for _, tc := range []struct {
		input string
		code  string
	}{{input: "10", code: "-32602"}, {input: "11", code: "-32601"}, {input: "12", code: "-32603"}} {
		var result acp.SessionPromptResult
		err := c.Send(ctx, "session/prompt", acp.SessionPromptParams{SessionID: newResult.SessionID, Prompt: []acp.ContentBlock{{Type: "text", Text: tc.input}}}, &result)
		if err == nil {
			t.Fatalf("prompt %s: expected rpc error", tc.input)
		}
		if !strings.Contains(err.Error(), tc.code) {
			t.Fatalf("prompt %s: expected rpc code %s in err=%v", tc.input, tc.code, err)
		}
	}
}
