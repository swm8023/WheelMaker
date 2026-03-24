package client

import (
	"context"
	"strings"
	"testing"

	acp "github.com/swm8023/wheelmaker/internal/acp"
)

func TestResolveHelpModel_IncludesDebugStatusAction(t *testing.T) {
	c := New(&noopStore{}, nil, "test", "/tmp")
	c.RegisterAgent("codex", nil)
	c.session.ready = true

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

func TestResolveHelpModel_RootHasConfigEntriesAndAgentSubmenu(t *testing.T) {
	c := New(&noopStore{}, nil, "test", "/tmp")
	c.RegisterAgent("codex", nil)
	c.RegisterAgent("claude", nil)
	c.session.ready = true
	c.sessionMeta.ConfigOptions = []acp.ConfigOption{
		{
			ID:           "mode",
			CurrentValue: "plan",
			Options: []acp.ConfigOptionValue{
				{Name: "Plan", Value: "plan"},
				{Name: "Run", Value: "run"},
			},
		},
		{
			ID:           "theme",
			CurrentValue: "dark",
			Options: []acp.ConfigOptionValue{
				{Name: "Dark", Value: "dark"},
				{Name: "Light", Value: "light"},
			},
		},
	}

	model, err := c.resolveHelpModel(context.Background(), "chat-1")
	if err != nil {
		t.Fatalf("resolveHelpModel error: %v", err)
	}

	hasAgentSwitch := false
	hasModeAtRoot := false
	hasThemeAtRoot := false
	for _, opt := range model.Options {
		switch {
		case opt.Label == "Agent Switch" && strings.TrimSpace(opt.MenuID) != "":
			hasAgentSwitch = true
		case strings.HasPrefix(opt.Label, "Config: mode"):
			hasModeAtRoot = true
		case strings.HasPrefix(opt.Label, "Config: theme"):
			hasThemeAtRoot = true
		}
	}
	if !hasAgentSwitch {
		t.Fatalf("help root menu missing agent switch entry: %+v", model.Options)
	}
	if !hasModeAtRoot || !hasThemeAtRoot {
		t.Fatalf("help root menu missing config entries: %+v", model.Options)
	}
}

func TestResolveConfigArg_ValidatesOptionValue(t *testing.T) {
	st := &SessionState{
		ConfigOptions: []acp.ConfigOption{
			{
				ID: "theme",
				Options: []acp.ConfigOptionValue{
					{Name: "Dark", Value: "dark"},
					{Name: "Light", Value: "light"},
				},
			},
		},
	}

	id, value, err := resolveConfigArg("theme Dark", st)
	if err != nil {
		t.Fatalf("resolveConfigArg returned error: %v", err)
	}
	if id != "theme" || value != "dark" {
		t.Fatalf("resolveConfigArg = (%q,%q), want (%q,%q)", id, value, "theme", "dark")
	}

	if _, _, err := resolveConfigArg("theme blue", st); err == nil {
		t.Fatalf("expected unknown config value error")
	}
}

type noopStore struct{}

func (s *noopStore) Load() (*ProjectState, error) { return defaultProjectState(), nil }
func (s *noopStore) Save(_ *ProjectState) error   { return nil }
