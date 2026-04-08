package hub

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	shared "github.com/swm8023/wheelmaker/internal/shared"
)

func TestBuildClient_FeishuEnablesIMWithoutVersion(t *testing.T) {
	h := New(&shared.AppConfig{}, t.TempDir()+"/db/client.sqlite3")
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
	t.Cleanup(func() { _ = c.Close() })
}

func TestBuildClient_AppEnablesIMStub(t *testing.T) {
	h := New(&shared.AppConfig{}, t.TempDir()+"/db/client.sqlite3")
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
	t.Cleanup(func() { _ = c.Close() })
}

func TestBuildClient_RejectsRemovedConsoleType(t *testing.T) {
	h := New(&shared.AppConfig{}, t.TempDir()+"/db/client.sqlite3")
	_, err := h.buildClient(context.Background(), shared.ProjectConfig{
		Name: "p",
		Path: ".",
		IM:   shared.IMConfig{Type: "console"},
	})
	if err == nil || !strings.Contains(err.Error(), "unsupported im.type") {
		t.Fatalf("err=%v, want unsupported im.type", err)
	}
}

func TestBuildClient_UnsupportedTypeLogsError(t *testing.T) {
	var buf bytes.Buffer
	if err := shared.Setup(shared.LoggerConfig{Level: shared.LevelInfo}); err != nil {
		t.Fatalf("setup logger: %v", err)
	}
	defer shared.Close()
	shared.SetOutput(&buf)
	defer shared.SetOutput(os.Stderr)

	h := New(&shared.AppConfig{}, t.TempDir()+"/db/client.sqlite3")
	_, _ = h.buildClient(context.Background(), shared.ProjectConfig{
		Name: "p",
		Path: ".",
		IM:   shared.IMConfig{Type: "console"},
	})
	if !strings.Contains(buf.String(), "hub: build client failed") {
		t.Fatalf("missing startup error log: %s", buf.String())
	}
}
