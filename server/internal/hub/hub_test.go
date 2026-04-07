package hub

import (
	"context"
	"strings"
	"testing"

	shared "github.com/swm8023/wheelmaker/internal/shared"
)

func TestBuildClient_IM1DefaultDoesNotEnableIM2(t *testing.T) {
	h := New(&shared.AppConfig{}, t.TempDir()+"/state.json")
	c, err := h.buildClient(context.Background(), shared.ProjectConfig{
		Name: "p",
		Path: ".",
		IM:   shared.IMConfig{Type: "console"},
	})
	if err != nil {
		t.Fatalf("buildClient: %v", err)
	}
	if c.HasIM2Router() {
		t.Fatal("IM2 router enabled for default IM1 config")
	}
}

func TestBuildClient_IM2FeishuEnablesIM2(t *testing.T) {
	h := New(&shared.AppConfig{}, t.TempDir()+"/state.json")
	c, err := h.buildClient(context.Background(), shared.ProjectConfig{
		Name: "p",
		Path: ".",
		IM:   shared.IMConfig{Type: "feishu", Version: 2},
	})
	if err != nil {
		t.Fatalf("buildClient: %v", err)
	}
	if !c.HasIM2Router() {
		t.Fatal("expected IM2 router for im.version=2")
	}
}

func TestBuildClient_IM2UnsupportedType(t *testing.T) {
	h := New(&shared.AppConfig{}, t.TempDir()+"/state.json")
	_, err := h.buildClient(context.Background(), shared.ProjectConfig{
		Name: "p",
		Path: ".",
		IM:   shared.IMConfig{Type: "console", Version: 2},
	})
	if err == nil || !strings.Contains(err.Error(), "unsupported im2 type") {
		t.Fatalf("err=%v, want unsupported im2 type", err)
	}
}
