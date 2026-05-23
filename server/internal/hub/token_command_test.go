package hub

import (
	"context"
	"testing"

	rp "github.com/swm8023/wheelmaker/internal/protocol"
)

func TestTokenCommandScanUsesHubLevelScanner(t *testing.T) {
	previousScanner := scanHubTokenStats
	t.Cleanup(func() {
		scanHubTokenStats = previousScanner
	})
	var called bool
	scanHubTokenStats = func(ctx context.Context) (any, error) {
		called = true
		return map[string]any{
			"ok":        true,
			"updatedAt": "2026-05-19T10:00:00Z",
			"providers": []map[string]any{
				{"id": "codex", "name": "Codex", "accounts": []map[string]any{}},
			},
		}, nil
	}

	cmd := NewTokenCommand()
	payload, cmdErr := cmd.Handle(context.Background(), rp.MustRaw(map[string]any{
		"action": "scan",
		"hubId":  "hub-token",
	}))

	if cmdErr != nil {
		t.Fatalf("cmdErr=%v", cmdErr)
	}
	if !called {
		t.Fatal("token scanner was not called")
	}
	body, ok := payload.(map[string]any)
	if !ok {
		t.Fatalf("payload=%#v, want map", payload)
	}
	if body["hubId"] != "hub-token" || body["ok"] != true {
		t.Fatalf("payload=%#v", body)
	}
}
