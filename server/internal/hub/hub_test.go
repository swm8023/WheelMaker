package hub

import (
	"bytes"
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	clientpkg "github.com/swm8023/wheelmaker/internal/hub/client"
	acp "github.com/swm8023/wheelmaker/internal/protocol"
	logger "github.com/swm8023/wheelmaker/internal/shared"
	_ "modernc.org/sqlite"
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

func TestBuildClientStartsWithSessionTurnStore(t *testing.T) {
	baseDir := t.TempDir()
	dbPath := filepath.Join(baseDir, "db", "client.sqlite3")
	store, err := clientpkg.NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	ctx := context.Background()
	if err := store.SaveSession(ctx, &clientpkg.SessionRecord{
		ID:           "sess-1",
		ProjectName:  "proj1",
		Status:       clientpkg.SessionActive,
		AgentType:    "codex",
		CreatedAt:    time.Date(2026, 5, 13, 10, 0, 0, 0, time.UTC),
		LastActiveAt: time.Date(2026, 5, 13, 10, 1, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}
	if err := store.UpsertSessionPrompt(ctx, clientpkg.SessionPromptRecord{
		SessionID:   "sess-1",
		PromptIndex: 1,
		StopReason:  acp.StopReasonEndTurn,
		UpdatedAt:   time.Date(2026, 5, 13, 10, 1, 0, 0, time.UTC),
		TurnsJSON: clientpkg.EncodeStoredTurns([]string{
			`{"method":"prompt_request","param":{"contentBlocks":[{"type":"text","text":"hello"}]}}`,
		}),
		TurnIndex: 1,
	}); err != nil {
		t.Fatalf("UpsertSessionPrompt: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close store: %v", err)
	}

	h := New(&logger.AppConfig{}, dbPath)
	c, err := h.buildClient(ctx, logger.ProjectConfig{Name: "proj1", Path: "."})
	if err != nil {
		t.Fatalf("buildClient: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	if _, err := os.Stat(filepath.Join(baseDir, "session")); err != nil && !os.IsNotExist(err) {
		t.Fatalf("stat session root: %v", err)
	}
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
	if !strings.Contains(buf.String(), "[Hub:p] build client failed") {
		t.Fatalf("missing startup error log: %s", buf.String())
	}
}

func TestStartRejectsSchemaMismatchWithDeleteHint(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "db", "client.sqlite3")

	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatalf("mkdir db dir: %v", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if _, err := db.Exec(`
		CREATE TABLE projects (
			project_name TEXT PRIMARY KEY,
			yolo INTEGER NOT NULL DEFAULT 0,
			agent_state_json TEXT NOT NULL DEFAULT '{}',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)
	`); err != nil {
		_ = db.Close()
		t.Fatalf("create legacy projects table: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}

	h := New(&logger.AppConfig{}, dbPath)
	err = h.Start(context.Background())
	if err == nil {
		t.Fatal("Start() error = nil, want schema mismatch")
	}
	if !strings.Contains(err.Error(), "delete local db directory") {
		t.Fatalf("Start() err = %v, want delete local db directory hint", err)
	}
}
