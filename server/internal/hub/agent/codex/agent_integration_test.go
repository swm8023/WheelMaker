//go:build integration

// Integration test for agent/codex: verifies that codex.Agent.Connect()
// spawns a real codex-acp subprocess and returns a working *acp.Conn.
//
// Run with: go test -tags integration ./internal/agent/codex/... -v -timeout 60s
// Requires: codex-acp installed (for example via npm -g) and available in PATH,
//
//	and OPENAI_API_KEY set in the environment.
package codex_test

import (
	"context"
	"os"
	"testing"
	"time"

	acp "github.com/swm8023/wheelmaker/internal/hub/acp"
	"github.com/swm8023/wheelmaker/internal/hub/agent/codex"
	"github.com/swm8023/wheelmaker/internal/hub/tools"
)

// requireCodexBinary skips the test if the codex-acp binary is not available.
func requireCodexBinary(t *testing.T) {
	t.Helper()
	if _, err := tools.ResolveBinary("codex-acp", ""); err != nil {
		t.Skipf("codex-acp binary not found: %v", err)
	}
	if os.Getenv("OPENAI_API_KEY") == "" {
		t.Skip("OPENAI_API_KEY not set")
	}
}

// TestBackend_Connect verifies that Connect() spawns a subprocess and
// returns a Conn that can successfully complete the ACP initialize handshake.
func TestBackend_Connect(t *testing.T) {
	requireCodexBinary(t)

	a := codex.New(codex.Config{})

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	conn, err := a.Connect(ctx)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer conn.Close()

	// Verify the returned Conn works: the subprocess must be running.
	var result acp.InitializeResult
	if err := conn.SendAgent(ctx, "initialize", acp.InitializeParams{
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

	if result.ProtocolVersion == "" {
		t.Error("expected non-empty protocolVersion")
	}
	t.Logf("connected to codex-acp: agentInfo=%+v protocol=%s", result.AgentInfo, result.ProtocolVersion)
}

// TestBackend_ConnectMultiple verifies that Connect() is truly stateless:
// calling it twice produces two independent connections.
func TestBackend_ConnectMultiple(t *testing.T) {
	requireCodexBinary(t)

	a := codex.New(codex.Config{})

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

	// Both conns must be independently operable.
	for i, conn := range []*acp.Conn{conn1, conn2} {
		var result acp.InitializeResult
		if err := conn.SendAgent(ctx, "initialize", acp.InitializeParams{ProtocolVersion: "0.1"}, &result); err != nil {
			t.Errorf("conn %d initialize: %v", i+1, err)
		}
	}
}

// TestBackend_Close verifies that Close() is a no-op.
func TestBackend_Close(t *testing.T) {
	a := codex.New(codex.Config{})
	if err := a.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
	// Second Close should also be a no-op.
	if err := a.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
}
