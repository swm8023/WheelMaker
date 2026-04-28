package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	clientpkg "github.com/swm8023/wheelmaker/internal/hub/client"
)

func TestResolveLogFilePath_PrefersLogDir(t *testing.T) {
	base := t.TempDir()
	m := NewMonitor(base)

	newPath := filepath.Join(base, "log", "hub.log")
	if err := os.MkdirAll(filepath.Dir(newPath), 0o755); err != nil {
		t.Fatalf("mkdir log dir: %v", err)
	}
	if err := os.WriteFile(newPath, []byte("new"), 0o644); err != nil {
		t.Fatalf("write new log: %v", err)
	}
	oldPath := filepath.Join(base, "hub.log")
	if err := os.WriteFile(oldPath, []byte("old"), 0o644); err != nil {
		t.Fatalf("write old log: %v", err)
	}

	got := m.resolveLogFilePath("hub")
	if got != newPath {
		t.Fatalf("resolveLogFilePath(hub)=%q, want %q", got, newPath)
	}
}

func TestResolveLogFilePath_FallbackOldRoot(t *testing.T) {
	base := t.TempDir()
	m := NewMonitor(base)

	oldPath := filepath.Join(base, "registry.log")
	if err := os.WriteFile(oldPath, []byte("old"), 0o644); err != nil {
		t.Fatalf("write old log: %v", err)
	}

	got := m.resolveLogFilePath("registry")
	if got != oldPath {
		t.Fatalf("resolveLogFilePath(registry)=%q, want %q", got, oldPath)
	}
}

func TestGetLogs_DebugOmitsTimeLevelAndDedupsSessionID(t *testing.T) {
	base := t.TempDir()
	m := NewMonitor(base)

	logDir := filepath.Join(base, "log")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatalf("mkdir log dir: %v", err)
	}

	sid := "019d6db0-3e60-7cf3-85c6-d2bf7e2a6f8a"
	line := "2026/04/09 06:44:32 DEBUG [acp] < {" + sid + " session/update} {\"sessionId\":\"" + sid + "\",\"update\":{\"sessionUpdate\":\"agent_message_chunk\"}}"
	if err := os.WriteFile(filepath.Join(logDir, "hub.debug.log"), []byte(line+"\n"), 0o644); err != nil {
		t.Fatalf("write debug log: %v", err)
	}

	res, err := m.GetLogs("debug", "", 100)
	if err != nil {
		t.Fatalf("GetLogs(debug): %v", err)
	}
	if len(res.Entries) != 1 {
		t.Fatalf("entries=%d, want 1", len(res.Entries))
	}
	entry := res.Entries[0]
	if entry.Time != "" {
		t.Fatalf("debug time should be hidden, got %q", entry.Time)
	}
	if entry.Level != "" {
		t.Fatalf("debug level should be hidden, got %q", entry.Level)
	}
	if strings.Contains(entry.Message, "\"sessionId\":\""+sid+"\"") {
		t.Fatalf("duplicate sessionId should be removed from debug json payload: %q", entry.Message)
	}
	if !strings.Contains(entry.Message, "{019d6db0..6f8a session/update}") {
		t.Fatalf("session id should be shortened in debug prefix: %q", entry.Message)
	}
}

func TestGetDBTablesIncludesPromptTurnTables(t *testing.T) {
	base := t.TempDir()
	store, err := clientpkg.NewStore(filepath.Join(base, "db", "client.sqlite3"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	if err := store.SaveSession(ctx, &clientpkg.SessionRecord{
		ID:           "sess-1",
		ProjectName:  "proj1",
		Status:       clientpkg.SessionActive,
		AgentType:    "claude",
		AgentJSON:    `{}`,
		CreatedAt:    time.Date(2026, 4, 12, 10, 0, 0, 0, time.UTC),
		LastActiveAt: time.Date(2026, 4, 12, 10, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}
	if err := store.UpsertSessionPrompt(ctx, clientpkg.SessionPromptRecord{
		SessionID:   "sess-1",
		PromptIndex: 1,
		UpdatedAt:   time.Date(2026, 4, 12, 10, 1, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("UpsertSessionPrompt: %v", err)
	}
	mon := NewMonitor(base)
	res := mon.GetDBTables()
	if res.Error != "" {
		t.Fatalf("GetDBTables error: %s", res.Error)
	}
	foundPrompts := false
	for _, table := range res.Tables {
		if table.Name == "session_prompts" {
			foundPrompts = true
		}
	}
	if !foundPrompts {
		t.Fatalf("session_prompts table missing: %#v", res.Tables)
	}
}

func TestGetDBTablesMatchesCurrentStoreSchema(t *testing.T) {
	base := t.TempDir()
	store, err := clientpkg.NewStore(filepath.Join(base, "db", "client.sqlite3"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	mon := NewMonitor(base)
	res := mon.GetDBTables()
	if res.Error != "" {
		t.Fatalf("GetDBTables error: %s", res.Error)
	}

	foundAgentPreferences := false
	foundProjects := false
	for _, table := range res.Tables {
		if table.Name == "agent_preferences" {
			foundAgentPreferences = true
		}
		if table.Name == "projects" {
			foundProjects = true
		}
	}
	if !foundAgentPreferences {
		t.Fatalf("agent_preferences table missing: %#v", res.Tables)
	}
	if !foundProjects {
		t.Fatalf("projects table missing: %#v", res.Tables)
	}
}

func TestParseMonitorSessionTurnDoesNotUseUpdateIndexAsSubIndex(t *testing.T) {
	method, role, kind, body, status, requestID, index, subIndex, source, ts := parseMonitorSessionTurn(
		`{"method":"agent_message_chunk","param":{"text":"hello"}}`,
		"2026-04-28T09:00:00Z",
		7,
	)
	if method != "agent_message_chunk" {
		t.Fatalf("method = %q, want %q", method, "agent_message_chunk")
	}
	if role != "assistant" {
		t.Fatalf("role = %q, want %q", role, "assistant")
	}
	if kind != "agent_message_chunk" {
		t.Fatalf("kind = %q, want %q", kind, "agent_message_chunk")
	}
	if body != "hello" {
		t.Fatalf("body = %q, want %q", body, "hello")
	}
	if status != "done" {
		t.Fatalf("status = %q, want %q", status, "done")
	}
	if requestID != 0 {
		t.Fatalf("requestID = %d, want 0", requestID)
	}
	if index != 7 {
		t.Fatalf("index = %d, want 7", index)
	}
	if subIndex != 0 {
		t.Fatalf("subIndex = %d, want 0", subIndex)
	}
	if source != "" {
		t.Fatalf("source = %q, want empty", source)
	}
	if ts != "2026-04-28T09:00:00Z" {
		t.Fatalf("ts = %q, want %q", ts, "2026-04-28T09:00:00Z")
	}
}

func TestExecuteActionClearSessionHistoryDeletesPromptAndTurnTables(t *testing.T) {
	base := t.TempDir()
	store, err := clientpkg.NewStore(filepath.Join(base, "db", "client.sqlite3"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	now := time.Date(2026, 4, 12, 10, 0, 0, 0, time.UTC)
	if err := store.SaveSession(ctx, &clientpkg.SessionRecord{
		ID:           "sess-1",
		ProjectName:  "proj1",
		Status:       clientpkg.SessionActive,
		AgentType:    "claude",
		AgentJSON:    `{}`,
		CreatedAt:    now,
		LastActiveAt: now,
	}); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}
	if err := store.UpsertSessionPrompt(ctx, clientpkg.SessionPromptRecord{
		SessionID:   "sess-1",
		PromptIndex: 1,
		Title:       "hello",
		UpdatedAt:   now,
	}); err != nil {
		t.Fatalf("UpsertSessionPrompt: %v", err)
	}
	mon := NewMonitor(base)
	if err := mon.ExecuteActionByHub(context.Background(), "", "clear-session-history"); err != nil {
		t.Fatalf("ExecuteActionByHub(clear-session-history): %v", err)
	}

	prompts, err := store.ListSessionPrompts(ctx, "proj1", "sess-1")
	if err != nil {
		t.Fatalf("ListSessionPrompts: %v", err)
	}
	if len(prompts) != 0 {
		t.Fatalf("prompts len = %d, want 0", len(prompts))
	}
	rec, err := store.LoadSession(ctx, "proj1", "sess-1")
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	if rec == nil {
		t.Fatalf("LoadSession returned nil, want existing session record")
	}
}
