//go:build integration

// Integration test for provider/claude: verifies that ClaudeAdapter.Connect()
// spawns a real claude-agent-acp subprocess and returns a working *acp.Conn.
//
// Run with: go test -tags integration ./internal/provider/claude/... -v -timeout 60s
// Requires: claude-agent-acp binary at bin/windows_amd64/claude-agent-acp.exe or in PATH,
//
//	and ANTHROPIC_API_KEY set in the environment.
package claude_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/swm8023/wheelmaker/internal/agent/acp"
	"github.com/swm8023/wheelmaker/internal/provider/claude"
	"github.com/swm8023/wheelmaker/internal/tools"
)

// requireClaudeBinary skips the test if the claude-agent-acp binary is not available.
func requireClaudeBinary(t *testing.T) {
	t.Helper()
	if _, err := tools.ResolveBinary("claude-agent-acp", ""); err != nil {
		t.Skipf("claude-agent-acp binary not found: %v", err)
	}
	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		t.Skip("ANTHROPIC_API_KEY not set")
	}
}

// TestClaudeAdapter_Connect verifies that Connect() spawns a subprocess and
// returns a Conn that can successfully complete the ACP initialize handshake.
func TestClaudeAdapter_Connect(t *testing.T) {
	requireClaudeBinary(t)

	a := claude.NewAdapter(claude.Config{})

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	conn, err := a.Connect(ctx)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer conn.Close()

	var result acp.InitializeResult
	if err := conn.Send(ctx, "initialize", acp.InitializeParams{
		ProtocolVersion: 1,
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

	if result.ProtocolVersion == "" {
		t.Error("expected non-empty protocolVersion")
	}
	t.Logf("connected to claude-agent-acp: agentInfo=%+v protocol=%s", result.AgentInfo, result.ProtocolVersion)
}

// TestClaudeAdapter_ConnectMultiple verifies that Connect() is stateless:
// calling it twice produces two independent connections.
func TestClaudeAdapter_ConnectMultiple(t *testing.T) {
	requireClaudeBinary(t)

	a := claude.NewAdapter(claude.Config{})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	conn1, err := a.Connect(ctx)
	if err != nil {
		t.Fatalf("Connect #1: %v", err)
	}
	defer conn1.Close()

	conn2, err := a.Connect(ctx)
	if err != nil {
		t.Fatalf("Connect #2: %v", err)
	}
	defer conn2.Close()

	for i, conn := range []*acp.Conn{conn1, conn2} {
		var result acp.InitializeResult
		if err := conn.Send(ctx, "initialize", acp.InitializeParams{ProtocolVersion: 1}, &result); err != nil {
			t.Errorf("conn %d initialize: %v", i+1, err)
		}
	}
}

// TestClaudeAdapter_Close verifies that Close() on the adapter is a no-op.
func TestClaudeAdapter_Close(t *testing.T) {
	a := claude.NewAdapter(claude.Config{})
	if err := a.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
	if err := a.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
}

