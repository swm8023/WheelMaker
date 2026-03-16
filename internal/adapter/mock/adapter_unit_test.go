package mock_test

import (
	"context"
	"testing"

	mockadapter "github.com/swm8023/wheelmaker/internal/adapter/mock"
	"github.com/swm8023/wheelmaker/internal/agent/acp"
)

func TestMockAdapter_Name(t *testing.T) {
	a := mockadapter.NewAdapter()
	if got := a.Name(); got != "mock" {
		t.Fatalf("Name() = %q, want %q", got, "mock")
	}
}

func TestMockAdapter_Connect(t *testing.T) {
	a := mockadapter.NewAdapter()
	conn, err := a.Connect(context.Background())
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	var initResult acp.InitializeResult
	if err := conn.Send(context.Background(), "initialize", acp.InitializeParams{
		ProtocolVersion: 1,
	}, &initResult); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	if initResult.ProtocolVersion.String() == "" {
		t.Fatalf("empty protocolVersion")
	}
}

func TestMockAdapter_Close(t *testing.T) {
	a := mockadapter.NewAdapter()
	if err := a.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := a.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}
