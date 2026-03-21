package client

import (
	"context"
	"testing"
)

func TestResolveHelpModel_IncludesDebugStatusAction(t *testing.T) {
	c := New(&noopStore{}, nil, "test", "/tmp")
	c.RegisterAgent("codex", nil)
	c.ready = true

	model, err := c.resolveHelpModel(context.Background(), "chat-1")
	if err != nil {
		t.Fatalf("resolveHelpModel error: %v", err)
	}

	hasDebugStatus := false
	for _, opt := range model.Options {
		if opt.Label == "Project Debug Status" && opt.Command == "/debug" && opt.Value == "" {
			hasDebugStatus = true
		}
	}
	if !hasDebugStatus {
		t.Fatalf("help options missing debug status action: %+v", model.Options)
	}
}

type noopStore struct{}

func (s *noopStore) Load() (*ProjectState, error) { return defaultProjectState(), nil }
func (s *noopStore) Save(_ *ProjectState) error   { return nil }
