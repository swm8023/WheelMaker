package hub

import (
	"context"
	"strings"
	"testing"

	shared "github.com/swm8023/wheelmaker/internal/shared"
)

func TestBuildClient_FeishuEnablesIMWithoutVersion(t *testing.T) {
	h := New(&shared.AppConfig{}, t.TempDir()+"/state.json")
	c, err := h.buildClient(context.Background(), shared.ProjectConfig{
		Name: "p",
		Path: ".",
		IM:   shared.IMConfig{Type: "feishu"},
	})
	if err != nil {
		t.Fatalf("buildClient: %v", err)
	}
	if !c.HasIMRouter() {
		t.Fatal("expected IM router for feishu config")
	}
}

func TestBuildClient_AppEnablesIMStub(t *testing.T) {
	h := New(&shared.AppConfig{}, t.TempDir()+"/state.json")
	c, err := h.buildClient(context.Background(), shared.ProjectConfig{
		Name: "p",
		Path: ".",
		IM:   shared.IMConfig{Type: "app"},
	})
	if err != nil {
		t.Fatalf("buildClient: %v", err)
	}
	if !c.HasIMRouter() {
		t.Fatal("expected IM router for app config")
	}
}

func TestBuildClient_RejectsRemovedConsoleType(t *testing.T) {
	h := New(&shared.AppConfig{}, t.TempDir()+"/state.json")
	_, err := h.buildClient(context.Background(), shared.ProjectConfig{
		Name: "p",
		Path: ".",
		IM:   shared.IMConfig{Type: "console"},
	})
	if err == nil || !strings.Contains(err.Error(), "unsupported im.type") {
		t.Fatalf("err=%v, want unsupported im.type", err)
	}
}
