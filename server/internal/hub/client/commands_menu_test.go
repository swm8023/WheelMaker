package client

import (
	"context"
	"strings"
	"testing"

	acp "github.com/swm8023/wheelmaker/internal/protocol"
)

func TestResolveHelpModelRefreshesSessionMenuFromRuntimeList(t *testing.T) {
	s := newSession("sess-local", ".")
	inst := &testInjectedInstance{
		name:      string(acp.ACPProviderClaude),
		sessionID: "sess-current",
		alive:     true,
		listResult: acp.SessionListResult{
			Sessions: []acp.SessionInfo{
				{SessionID: "sess-older", Title: "Older Session"},
				{SessionID: "sess-current", Title: "Current Session"},
			},
		},
	}

	s.mu.Lock()
	s.instance = inst
	s.activeAgent = inst.name
	s.acpSessionID = "sess-current"
	s.ready = true
	state := s.agentStateLocked(inst.name)
	state.AgentCapabilities = acp.AgentCapabilities{
		LoadSession: true,
		SessionCapabilities: &acp.SessionCapabilities{
			List: &acp.SessionListCapability{},
		},
	}
	s.mu.Unlock()

	model, err := s.resolveHelpModel(context.Background(), "")
	if err != nil {
		t.Fatalf("resolveHelpModel() err = %v", err)
	}

	sessionMenu, ok := model.Menus["menu:sessions"]
	if !ok {
		t.Fatalf("session menu not found")
	}
	if len(sessionMenu.Options) != 2 {
		t.Fatalf("session menu options len = %d, want 2", len(sessionMenu.Options))
	}
	if sessionMenu.Options[0].Command != "/load" || sessionMenu.Options[0].Value != "1" {
		t.Fatalf("session menu option[0] = %#v, want /load 1", sessionMenu.Options[0])
	}
	if sessionMenu.Options[1].Command != "/load" || sessionMenu.Options[1].Value != "2" {
		t.Fatalf("session menu option[1] = %#v, want /load 2", sessionMenu.Options[1])
	}
	if strings.Contains(sessionMenu.Body, "No cached sessions") {
		t.Fatalf("session menu body should not show cached-session fallback: %q", sessionMenu.Body)
	}
}
