package client

import (
	"context"
	"testing"
)

func TestResolveHelpModel_IncludesDebugQuickActions(t *testing.T) {
	c := New(&noopStore{}, nil, "test", "/tmp")
	c.RegisterAgent("codex", nil)
	c.ready = true

	model, err := c.resolveHelpModel(context.Background(), "chat-1")
	if err != nil {
		t.Fatalf("resolveHelpModel error: %v", err)
	}

	hasDebugOn := false
	hasDebugOff := false
	for _, opt := range model.Options {
		if opt.Label == "Project Debug On" && opt.Command == "/debug" && opt.Value == "on" {
			hasDebugOn = true
		}
		if opt.Label == "Project Debug Off" && opt.Command == "/debug" && opt.Value == "off" {
			hasDebugOff = true
		}
	}
	if !hasDebugOn || !hasDebugOff {
		t.Fatalf("help options missing debug quick actions: %+v", model.Options)
	}
}

type noopStore struct{}

func (s *noopStore) Load() (*ProjectState, error) { return defaultProjectState(), nil }
func (s *noopStore) Save(_ *ProjectState) error   { return nil }
