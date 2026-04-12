package client

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	acp "github.com/swm8023/wheelmaker/internal/protocol"
	_ "modernc.org/sqlite"
)

func TestStoreProjectAgentStateRoundTrip(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "client.sqlite3"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	cfg := ProjectConfig{
		YOLO: true,
		AgentState: map[string]ProjectAgentState{
			"codex": {
				ConfigOptions: []acp.ConfigOption{
					{ID: acp.ConfigOptionIDMode, Category: acp.ConfigOptionCategoryMode, CurrentValue: "code"},
					{ID: acp.ConfigOptionIDModel, Category: acp.ConfigOptionCategoryModel, CurrentValue: "gpt-5"},
					{ID: acp.ConfigOptionIDThoughtLevel, Category: acp.ConfigOptionCategoryThoughtLv, CurrentValue: "high"},
				},
				AvailableCommands: []acp.AvailableCommand{{Name: "/status"}},
				UpdatedAt:         "2026-04-11T00:00:00Z",
			},
		},
	}
	if err := store.SaveProject(context.Background(), "proj1", cfg); err != nil {
		t.Fatalf("SaveProject: %v", err)
	}

	loaded, err := store.LoadProject(context.Background(), "proj1")
	if err != nil {
		t.Fatalf("LoadProject: %v", err)
	}
	if !loaded.YOLO {
		t.Fatal("YOLO = false, want true")
	}
	codex := loaded.AgentState["codex"]
	if got := len(codex.ConfigOptions); got != 3 {
		t.Fatalf("config options = %d, want 3", got)
	}
	if got := len(codex.AvailableCommands); got != 1 {
		t.Fatalf("commands = %d, want 1", got)
	}
}

func TestStoreMigratesLegacyProjectsTable(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "client.sqlite3")

	legacyDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open legacy db: %v", err)
	}
	if _, err := legacyDB.Exec(`
		CREATE TABLE projects (
			project_name TEXT PRIMARY KEY,
			yolo INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);
	`); err != nil {
		_ = legacyDB.Close()
		t.Fatalf("create legacy projects table: %v", err)
	}
	if _, err := legacyDB.Exec(`
		INSERT INTO projects (project_name, yolo, created_at, updated_at)
		VALUES ('proj1', 1, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')
	`); err != nil {
		_ = legacyDB.Close()
		t.Fatalf("insert legacy project row: %v", err)
	}
	if err := legacyDB.Close(); err != nil {
		t.Fatalf("close legacy db: %v", err)
	}

	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	loaded, err := store.LoadProject(context.Background(), "proj1")
	if err != nil {
		t.Fatalf("LoadProject after migration: %v", err)
	}
	if !loaded.YOLO {
		t.Fatal("YOLO = false, want true")
	}
	if got := len(loaded.AgentState); got != 0 {
		t.Fatalf("agent state size = %d, want 0", got)
	}

	next := ProjectConfig{
		YOLO: true,
		AgentState: map[string]ProjectAgentState{
			"codex": {
				AvailableCommands: []acp.AvailableCommand{{Name: "/help"}},
			},
		},
	}
	if err := store.SaveProject(context.Background(), "proj1", next); err != nil {
		t.Fatalf("SaveProject after migration: %v", err)
	}
}

func TestStoreBackfillsSyncIndicesForLegacySessionMessages(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "client.sqlite3")

	legacyDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open legacy db: %v", err)
	}
	if _, err := legacyDB.Exec(`
		CREATE TABLE sessions (
			id TEXT PRIMARY KEY,
			project_name TEXT NOT NULL,
			status INTEGER NOT NULL,
			last_reply TEXT NOT NULL DEFAULT '',
			acp_session_id TEXT NOT NULL DEFAULT '',
			agents_json TEXT NOT NULL DEFAULT '{}',
			title TEXT NOT NULL DEFAULT '',
			last_message_preview TEXT NOT NULL DEFAULT '',
			last_message_at TEXT NOT NULL DEFAULT '',
			message_count INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL,
			last_active_at TEXT NOT NULL
		);
		CREATE TABLE session_messages (
			message_id TEXT PRIMARY KEY,
			project_name TEXT NOT NULL,
			session_id TEXT NOT NULL,
			role TEXT NOT NULL,
			kind TEXT NOT NULL,
			body TEXT NOT NULL DEFAULT '',
			blocks_json TEXT NOT NULL DEFAULT '[]',
			options_json TEXT NOT NULL DEFAULT '[]',
			status TEXT NOT NULL DEFAULT 'done',
			source_channel TEXT NOT NULL DEFAULT '',
			source_chat_id TEXT NOT NULL DEFAULT '',
			request_id INTEGER NOT NULL DEFAULT 0,
			aggregate_key TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);
	`); err != nil {
		_ = legacyDB.Close()
		t.Fatalf("create legacy tables: %v", err)
	}
	if _, err := legacyDB.Exec(`
		INSERT INTO sessions (id, project_name, status, title, last_message_preview, message_count, created_at, last_active_at)
		VALUES ('sess-1', 'proj1', 1, 'Legacy', 'second', 2, '2026-04-12T10:00:00Z', '2026-04-12T10:02:00Z');
		INSERT INTO session_messages (message_id, project_name, session_id, role, kind, body, created_at, updated_at)
		VALUES
			('msg-1', 'proj1', 'sess-1', 'user', 'text', 'first', '2026-04-12T10:01:00Z', '2026-04-12T10:01:00Z'),
			('msg-2', 'proj1', 'sess-1', 'assistant', 'text', 'second', '2026-04-12T10:02:00Z', '2026-04-12T10:02:00Z');
	`); err != nil {
		_ = legacyDB.Close()
		t.Fatalf("insert legacy rows: %v", err)
	}
	if err := legacyDB.Close(); err != nil {
		t.Fatalf("close legacy db: %v", err)
	}

	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	rec, err := store.LoadSession(context.Background(), "proj1", "sess-1")
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	if rec == nil || rec.LastSyncIndex != 2 {
		t.Fatalf("LoadSession().LastSyncIndex = %#v, want 2", rec)
	}
	messages, err := store.ListSessionMessagesAfterIndex(context.Background(), "proj1", "sess-1", 0)
	if err != nil {
		t.Fatalf("ListSessionMessagesAfterIndex: %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("ListSessionMessagesAfterIndex() len = %d, want 2", len(messages))
	}
	if messages[0].SyncIndex != 1 || messages[1].SyncIndex != 2 {
		t.Fatalf("sync indexes = [%d, %d], want [1, 2]", messages[0].SyncIndex, messages[1].SyncIndex)
	}
}

func TestStoreSessionProjectionRoundTrip(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "client.sqlite3"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	rec := &SessionRecord{
		ID:                 "sess-1",
		ProjectName:        "proj1",
		Status:             SessionActive,
		LastReply:          "legacy",
		CreatedAt:          time.Date(2026, 4, 12, 10, 0, 0, 0, time.UTC),
		LastActiveAt:       time.Date(2026, 4, 12, 10, 5, 0, 0, time.UTC),
		Title:              "Fix app sessions",
		LastMessagePreview: "hello world",
		LastMessageAt:      time.Date(2026, 4, 12, 10, 5, 0, 0, time.UTC),
		MessageCount:       3,
	}

	if err := store.SaveSession(context.Background(), rec); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}

	loaded, err := store.LoadSession(context.Background(), "proj1", "sess-1")
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	if loaded == nil {
		t.Fatal("LoadSession returned nil record")
	}
	if loaded.Title != "Fix app sessions" {
		t.Fatalf("LoadSession().Title = %q, want %q", loaded.Title, "Fix app sessions")
	}
	if loaded.LastMessagePreview != "hello world" {
		t.Fatalf("LoadSession().LastMessagePreview = %q, want %q", loaded.LastMessagePreview, "hello world")
	}
	if loaded.MessageCount != 3 {
		t.Fatalf("LoadSession().MessageCount = %d, want 3", loaded.MessageCount)
	}

	entries, err := store.ListSessions(context.Background(), "proj1")
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("ListSessions() len = %d, want 1", len(entries))
	}
	if entries[0].Title != "Fix app sessions" {
		t.Fatalf("ListSessions()[0].Title = %q, want %q", entries[0].Title, "Fix app sessions")
	}
	if entries[0].Preview != "hello world" {
		t.Fatalf("ListSessions()[0].Preview = %q, want %q", entries[0].Preview, "hello world")
	}
	if entries[0].MessageCount != 3 {
		t.Fatalf("ListSessions()[0].MessageCount = %d, want 3", entries[0].MessageCount)
	}
}

func TestStoreSessionMessageHistoryRoundTrip(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "client.sqlite3"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()
	if err := store.SaveSession(context.Background(), &SessionRecord{
		ID:           "sess-1",
		ProjectName:  "proj1",
		Status:       SessionActive,
		CreatedAt:    time.Date(2026, 4, 12, 10, 0, 0, 0, time.UTC),
		LastActiveAt: time.Date(2026, 4, 12, 10, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}

	msg := SessionMessageRecord{
		MessageID:    "msg-1",
		SessionID:    "sess-1",
		ProjectName:  "proj1",
		Role:         "assistant",
		Kind:         "text",
		Body:         "aggregated reply",
		Blocks:       []acp.ContentBlock{{Type: acp.ContentBlockTypeImage, MimeType: "image/png", Data: "abc123"}},
		Options:      []acp.PermissionOption{{OptionID: "allow", Name: "Allow", Kind: "allow_once"}},
		Status:       "done",
		AggregateKey: "assistant:sess-1:turn-1",
		CreatedAt:    time.Date(2026, 4, 12, 10, 6, 0, 0, time.UTC),
		UpdatedAt:    time.Date(2026, 4, 12, 10, 6, 0, 0, time.UTC),
	}

	if err := store.AppendSessionMessage(context.Background(), msg); err != nil {
		t.Fatalf("AppendSessionMessage: %v", err)
	}

	messages, err := store.ListSessionMessages(context.Background(), "proj1", "sess-1")
	if err != nil {
		t.Fatalf("ListSessionMessages: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("ListSessionMessages() len = %d, want 1", len(messages))
	}
	if messages[0].Body != "aggregated reply" {
		t.Fatalf("ListSessionMessages()[0].Body = %q, want %q", messages[0].Body, "aggregated reply")
	}
	if len(messages[0].Blocks) != 1 || messages[0].Blocks[0].Type != acp.ContentBlockTypeImage {
		t.Fatalf("ListSessionMessages()[0].Blocks = %#v, want image block", messages[0].Blocks)
	}
	if len(messages[0].Options) != 1 || messages[0].Options[0].OptionID != "allow" {
		t.Fatalf("ListSessionMessages()[0].Options = %#v, want allow option", messages[0].Options)
	}
}

func TestStoreSessionMessageSyncIndexRoundTrip(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "client.sqlite3"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	if err := store.SaveSession(ctx, &SessionRecord{
		ID:           "sess-1",
		ProjectName:  "proj1",
		Status:       SessionActive,
		CreatedAt:    time.Date(2026, 4, 12, 10, 0, 0, 0, time.UTC),
		LastActiveAt: time.Date(2026, 4, 12, 10, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}

	msg := SessionMessageRecord{
		MessageID:   "msg-1",
		SessionID:   "sess-1",
		ProjectName: "proj1",
		Role:        "system",
		Kind:        "permission",
		Body:        "Run tool?",
		Status:      "needs_action",
		CreatedAt:   time.Date(2026, 4, 12, 10, 1, 0, 0, time.UTC),
		UpdatedAt:   time.Date(2026, 4, 12, 10, 1, 0, 0, time.UTC),
		RequestID:   42,
	}

	if err := store.AppendSessionMessage(ctx, msg); err != nil {
		t.Fatalf("AppendSessionMessage: %v", err)
	}
	msg.Status = "done"
	msg.UpdatedAt = time.Date(2026, 4, 12, 10, 2, 0, 0, time.UTC)
	if err := store.UpsertSessionMessage(ctx, msg); err != nil {
		t.Fatalf("UpsertSessionMessage: %v", err)
	}

	messages, err := store.ListSessionMessagesAfterIndex(ctx, "proj1", "sess-1", 1)
	if err != nil {
		t.Fatalf("ListSessionMessagesAfterIndex: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("ListSessionMessagesAfterIndex() len = %d, want 1", len(messages))
	}
	if messages[0].Status != "done" {
		t.Fatalf("messages[0].Status = %q, want done", messages[0].Status)
	}
	if messages[0].SyncIndex != 2 {
		t.Fatalf("messages[0].SyncIndex = %d, want 2", messages[0].SyncIndex)
	}

	rec, err := store.LoadSession(ctx, "proj1", "sess-1")
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	if rec == nil || rec.LastSyncIndex != 2 {
		t.Fatalf("LoadSession().LastSyncIndex = %v, want 2", rec)
	}
}
