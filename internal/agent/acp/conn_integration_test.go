//go:build integration

// Integration tests for acp.Conn against the real codex-acp binary.
// Run with: go test -tags integration ./internal/agent/acp/... -v -timeout 60s
// Requires: OPENAI_API_KEY set and codex-acp binary available.
package acp_test

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/swm8023/wheelmaker/internal/agent/acp"
	"github.com/swm8023/wheelmaker/internal/tools"
)

func requireCodexAcp(t *testing.T) string {
	t.Helper()
	path, err := tools.ResolveBinary("codex-acp", "")
	if err != nil {
		t.Skipf("codex-acp not found: %v", err)
	}
	if os.Getenv("OPENAI_API_KEY") == "" {
		t.Skip("OPENAI_API_KEY not set")
	}
	return path
}

func TestIntegration_Initialize(t *testing.T) {
	exePath := requireCodexAcp(t)
	c := acp.New(exePath, nil)
	if err := c.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	var result acp.InitializeResult
	if err := c.Send(ctx, "initialize", acp.InitializeParams{
		ProtocolVersion: "0.1",
		ClientCapabilities: acp.ClientCapabilities{
			FS: &acp.FSCapabilities{
				ReadTextFile:  true,
				WriteTextFile: true,
			},
			Terminal: true,
		},
		ClientInfo: &acp.AgentInfo{Name: "wheelmaker-test", Version: "0.1"},
	}, &result); err != nil {
		t.Fatalf("initialize: %v", err)
	}

	t.Logf("Agent: %+v", result.AgentInfo)
	t.Logf("Protocol: %s", result.ProtocolVersion)
	t.Logf("Capabilities: %+v", result.AgentCapabilities)

	if result.ProtocolVersion == "" {
		t.Error("expected non-empty protocolVersion")
	}
}

func TestIntegration_SessionNew(t *testing.T) {
	exePath := requireCodexAcp(t)
	c := acp.New(exePath, nil)
	if err := c.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Handshake
	if err := c.Send(ctx, "initialize", acp.InitializeParams{
		ProtocolVersion: "0.1",
	}, nil); err != nil {
		t.Fatalf("initialize: %v", err)
	}

	// Create session
	wd, _ := os.Getwd()
	var sessResult acp.SessionNewResult
	if err := c.Send(ctx, "session/new", acp.SessionNewParams{
		CWD:        wd,
		MCPServers: []acp.MCPServer{},
	}, &sessResult); err != nil {
		t.Fatalf("session/new: %v", err)
	}

	if sessResult.SessionID == "" {
		t.Error("expected non-empty sessionId")
	}
	t.Logf("SessionID: %s", sessResult.SessionID)
}

func TestIntegration_Prompt(t *testing.T) {
	exePath := requireCodexAcp(t)
	c := acp.New(exePath, nil)
	if err := c.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Handshake + session
	if err := c.Send(ctx, "initialize", acp.InitializeParams{ProtocolVersion: "0.1"}, nil); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	wd, _ := os.Getwd()
	var sessResult acp.SessionNewResult
	if err := c.Send(ctx, "session/new", acp.SessionNewParams{
		CWD:        wd,
		MCPServers: []acp.MCPServer{},
	}, &sessResult); err != nil {
		t.Fatalf("session/new: %v", err)
	}

	// Collect updates
	var updates []acp.SessionUpdateParams
	cancelSub := c.Subscribe(func(n acp.Notification) {
		if n.Method != "session/update" {
			return
		}
		var p acp.SessionUpdateParams
		if err := json.Unmarshal(n.Params, &p); err != nil {
			return
		}
		if p.SessionID == sessResult.SessionID {
			updates = append(updates, p)
			t.Logf("update: %s", p.Update.SessionUpdate)
		}
	})
	defer cancelSub()

	// Simple prompt that doesn't require tool use
	var promptResult acp.SessionPromptResult
	if err := c.Send(ctx, "session/prompt", acp.SessionPromptParams{
		SessionID: sessResult.SessionID,
		Prompt:    "Reply with exactly: PONG",
	}, &promptResult); err != nil {
		t.Fatalf("session/prompt: %v", err)
	}

	t.Logf("StopReason: %s", promptResult.StopReason)
	t.Logf("Received %d updates", len(updates))

	if promptResult.StopReason == "" {
		t.Error("expected non-empty stopReason")
	}
	if len(updates) == 0 {
		t.Error("expected at least one session/update notification")
	}
}

func TestIntegration_Cancel(t *testing.T) {
	exePath := requireCodexAcp(t)
	c := acp.New(exePath, nil)
	if err := c.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := c.Send(ctx, "initialize", acp.InitializeParams{ProtocolVersion: "0.1"}, nil); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	wd, _ := os.Getwd()
	var sessResult acp.SessionNewResult
	if err := c.Send(ctx, "session/new", acp.SessionNewParams{
		CWD:        wd,
		MCPServers: []acp.MCPServer{},
	}, &sessResult); err != nil {
		t.Fatalf("session/new: %v", err)
	}

	// Start a prompt in the background, then cancel it.
	promptDone := make(chan error, 1)
	go func() {
		promptDone <- c.Send(ctx, "session/prompt", acp.SessionPromptParams{
			SessionID: sessResult.SessionID,
			Prompt:    "Count from 1 to 1000 slowly with explanations for each number.",
		}, nil)
	}()

	// Give the agent a moment to start, then cancel.
	time.Sleep(500 * time.Millisecond)
	if err := c.Notify("session/cancel", acp.SessionCancelParams{
		SessionID: sessResult.SessionID,
	}); err != nil {
		t.Fatalf("notify session/cancel: %v", err)
	}

	select {
	case err := <-promptDone:
		t.Logf("prompt completed after cancel: err=%v", err)
		// Either nil (cancelled cleanly) or error is acceptable.
	case <-time.After(15 * time.Second):
		t.Error("prompt did not complete after cancel within 15s")
	}
}
