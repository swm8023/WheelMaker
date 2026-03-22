package mock_test

import (
	"context"
	"testing"

	acp "github.com/swm8023/wheelmaker/internal/acp"
	agentmock "github.com/swm8023/wheelmaker/internal/agent/mock"
)

func TestMockBackend_Name(t *testing.T) {
	a := agentmock.New()
	if got := a.Name(); got != "mock" {
		t.Fatalf("Name() = %q, want %q", got, "mock")
	}
}

func TestMockBackend_Connect(t *testing.T) {
	a := agentmock.New()
	conn, err := a.Connect(context.Background())
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	var initResult acp.InitializeResult
	if err := conn.SendAgent(context.Background(), "initialize", acp.InitializeParams{
		ProtocolVersion: 1,
	}, &initResult); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	if initResult.ProtocolVersion.String() == "" {
		t.Fatalf("empty protocolVersion")
	}
}

func TestMockBackend_Close(t *testing.T) {
	a := agentmock.New()
	if err := a.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := a.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

