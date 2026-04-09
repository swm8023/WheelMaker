package hub

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	logger "github.com/swm8023/wheelmaker/internal/shared"
)

func TestBuildClient_FeishuEnablesIMWithoutVersion(t *testing.T) {
	h := New(&logger.AppConfig{}, t.TempDir()+"/db/client.sqlite3")
	c, err := h.buildClient(context.Background(), logger.ProjectConfig{
		Name:   "p",
		Path:   ".",
		Feishu: &logger.FeishuConfig{AppID: "cli_xxx", AppSecret: "yyy"},
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
	h := New(&logger.AppConfig{}, t.TempDir()+"/db/client.sqlite3")
	c, err := h.buildClient(context.Background(), logger.ProjectConfig{
		Name: "p",
		Path: ".",
	})
	if err != nil {
		t.Fatalf("buildClient: %v", err)
	}
	if !c.HasIMRouter() {
		t.Fatal("expected IM router for app config")
	}
	t.Cleanup(func() { _ = c.Close() })
}

func TestBuildClient_RejectsInvalidFeishuConfig(t *testing.T) {
	h := New(&logger.AppConfig{}, t.TempDir()+"/db/client.sqlite3")
	_, err := h.buildClient(context.Background(), logger.ProjectConfig{
		Name:   "p",
		Path:   ".",
		Feishu: &logger.FeishuConfig{AppID: "cli_xxx"},
	})
	if err == nil || !strings.Contains(err.Error(), "invalid feishu config") {
		t.Fatalf("err=%v, want invalid feishu config", err)
	}
}

func TestBuildClient_InvalidFeishuLogsError(t *testing.T) {
	var buf bytes.Buffer
	if err := logger.Setup(logger.LoggerConfig{Level: logger.LevelInfo}); err != nil {
		t.Fatalf("setup logger: %v", err)
	}
	defer logger.Close()
	logger.SetOutput(&buf)
	defer logger.SetOutput(os.Stderr)

	h := New(&logger.AppConfig{}, t.TempDir()+"/db/client.sqlite3")
	_, _ = h.buildClient(context.Background(), logger.ProjectConfig{
		Name:   "p",
		Path:   ".",
		Feishu: &logger.FeishuConfig{AppID: "cli_xxx"},
	})
	if !strings.Contains(buf.String(), "hub: build client failed") {
		t.Fatalf("missing startup error log: %s", buf.String())
	}
}
