# Session Finished Cursor File History Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move session prompt history from SQLite `session_prompts.turns_json` into prompt files, add a finished-turn reconnect cursor, and make app/web cache only finished turns.

**Architecture:** Keep SQLite as the hot session index and add `sessions.session_sync_json` for reconnect projection. Add a file-backed Session History Store Adapter for prompt bodies, write one prompt snapshot file at `prompt_done`, and migrate existing prompt rows at startup. Change the app chat protocol from `done` to universal `finished`, with `prompt_done` stored as a real turn. `session.read(P, T)` returns the delta after the client's finished cursor, realtime gap detection asks for that same cursor, the server synthesizes terminal `prompt_done` before moving past an incomplete prompt, and the client treats server-emitted `prompt_request` as authoritative.

**Tech Stack:** Go, SQLite, JSON files, TypeScript, React, Jest

---

### Task 1: Add the Finished Cursor Contract on Server

**Files:**
- Modify: `server/internal/hub/client/session_recorder.go`
- Modify: `server/internal/hub/client/client_test.go`
- Test: `server/internal/hub/client/client_test.go`

- [ ] **Step 1: Write the failing server contract tests**

Add these tests to `server/internal/hub/client/client_test.go`:

```go
func TestSessionMessagePublishesFinishedFieldInsteadOfDone(t *testing.T) {
	c := newSessionViewTestClient(t)
	ctx := context.Background()

	if err := c.RecordEvent(ctx, sessionViewCreatedEvent("sess-1", "Finished Field")); err != nil {
		t.Fatalf("RecordEvent created: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewPromptEvent("sess-1", "hello", nil)); err != nil {
		t.Fatalf("RecordEvent prompt: %v", err)
	}

	last := lastPublishedEvent(t, c, "registry.session.message")
	if _, ok := last["done"]; ok {
		t.Fatalf("payload contains legacy done field: %#v", last)
	}
	if got := last["finished"]; got != true {
		t.Fatalf("finished = %v, want true", got)
	}
}

func TestPromptDoneIsPublishedAsFinishedRealTurn(t *testing.T) {
	c := newSessionViewTestClient(t)
	ctx := context.Background()

	if err := c.RecordEvent(ctx, sessionViewCreatedEvent("sess-1", "Prompt Done Turn")); err != nil {
		t.Fatalf("RecordEvent created: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewPromptEvent("sess-1", "hello", nil)); err != nil {
		t.Fatalf("RecordEvent prompt: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewUpdateEvent("sess-1", acp.SessionUpdate{
		SessionUpdate: acp.SessionUpdateAgentMessageChunk,
		Content:       mustJSON(acp.ContentBlock{Type: acp.ContentBlockTypeText, Text: "answer"}),
	})); err != nil {
		t.Fatalf("RecordEvent answer: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewPromptDoneEvent("sess-1", "end_turn")); err != nil {
		t.Fatalf("RecordEvent done: %v", err)
	}

	last := lastPublishedEvent(t, c, "registry.session.message")
	content := last["content"].(string)
	var msg acp.IMTurnMessage
	if err := json.Unmarshal([]byte(content), &msg); err != nil {
		t.Fatalf("unmarshal content: %v", err)
	}
	if msg.Method != acp.IMMethodPromptDone {
		t.Fatalf("method = %q, want %q", msg.Method, acp.IMMethodPromptDone)
	}
	if got := last["finished"]; got != true {
		t.Fatalf("finished = %v, want true", got)
	}
	if got := last["turnIndex"].(int64); got != 4 {
		t.Fatalf("turnIndex = %d, want 4", got)
	}
}
```

- [ ] **Step 2: Run the focused tests to verify they fail**

Run: `cd server && go test ./internal/hub/client -run "TestSessionMessagePublishesFinishedFieldInsteadOfDone|TestPromptDoneIsPublishedAsFinishedRealTurn" -count=1`

Expected: FAIL because the server still publishes `done`, and `prompt_done` is terminal notification state rather than a stored real turn.

- [ ] **Step 3: Implement the minimal server envelope change**

In `server/internal/hub/client/session_recorder.go`, rename envelope state:

```go
type sessionViewMessage struct {
	SessionID   string `json:"sessionId"`
	PromptIndex int64  `json:"promptIndex"`
	TurnIndex   int64  `json:"turnIndex"`
	Content     string `json:"content"`
	Finished    bool   `json:"finished"`
}

type sessionTurnMessage struct {
	sessionID   string
	method      string
	payload     any
	promptIndex int64
	turnIndex   int64
	finished    bool
}

type sessionTurnReadMessage struct {
	content  string
	finished bool
}
```

Replace publish payloads:

```go
payload := map[string]any{
	"sessionId":   turn.sessionID,
	"promptIndex": turn.promptIndex,
	"turnIndex":   turn.turnIndex,
	"content":     updateJSON,
	"finished":    turn.finished,
}
```

Add prompt done as a normal turn before publishing:

```go
doneTurn := sessionTurnMessage{
	sessionID:   event.SessionID,
	method:      acp.IMMethodPromptDone,
	payload:     acp.IMPromptResult{StopReason: stopReason},
	promptIndex: state.promptIndex,
	turnIndex:   state.nextTurnIndex,
	finished:    true,
}
state.updateTurn(doneTurn, "")
```

- [ ] **Step 4: Run the focused tests to verify they pass**

Run: `cd server && go test ./internal/hub/client -run "TestSessionMessagePublishesFinishedFieldInsteadOfDone|TestPromptDoneIsPublishedAsFinishedRealTurn" -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add server/internal/hub/client/session_recorder.go server/internal/hub/client/client_test.go
git commit -m "refactor: add finished turn envelope"
```

### Task 2: Add Session Sync Projection to SQLite

**Files:**
- Modify: `server/internal/hub/client/sqlite_store.go`
- Modify: `server/internal/hub/client/session_recorder.go`
- Modify: `server/internal/hub/client/client_test.go`
- Test: `server/internal/hub/client/client_test.go`

- [ ] **Step 1: Write failing session sync JSON tests**

Add this test:

```go
func TestStoreSessionSyncJSONRoundTrip(t *testing.T) {
	store := newTempSQLiteStore(t)
	ctx := context.Background()

	rec := &SessionRecord{
		ID:              "sess-1",
		ProjectName:     "proj1",
		Status:          SessionActive,
		AgentType:       "codex",
		CreatedAt:       mustRFC3339Time(t, "2026-05-13T10:00:00Z"),
		LastActiveAt:    mustRFC3339Time(t, "2026-05-13T10:01:00Z"),
		SessionSyncJSON: `{"promptIndex":2,"turnIndex":4,"finished":true}`,
	}
	if err := store.SaveSession(ctx, rec); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}

	loaded, err := store.LoadSession(ctx, "proj1", "sess-1")
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	if loaded == nil {
		t.Fatal("LoadSession returned nil")
	}
	if loaded.SessionSyncJSON != rec.SessionSyncJSON {
		t.Fatalf("SessionSyncJSON = %q, want %q", loaded.SessionSyncJSON, rec.SessionSyncJSON)
	}
}
```

- [ ] **Step 2: Run the focused test to verify it fails**

Run: `cd server && go test ./internal/hub/client -run TestStoreSessionSyncJSONRoundTrip -count=1`

Expected: FAIL because `SessionRecord` and the SQLite schema do not expose `session_sync_json`.

- [ ] **Step 3: Implement the sync projection field**

In `server/internal/hub/client/sqlite_store.go`, add:

```go
CREATE TABLE IF NOT EXISTS sessions (
	id TEXT PRIMARY KEY,
	project_name TEXT NOT NULL,
	status INTEGER NOT NULL,
	agent_type TEXT NOT NULL,
	agent_json TEXT NOT NULL DEFAULT '{}',
	title TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL,
	last_active_at TEXT NOT NULL,
	session_sync_json TEXT NOT NULL DEFAULT '{}'
);
```

Add migration:

```go
_, _ = db.Exec(`ALTER TABLE sessions ADD COLUMN session_sync_json TEXT NOT NULL DEFAULT '{}'`)
```

Extend `SessionRecord`:

```go
type SessionRecord struct {
	ID              string
	ProjectName     string
	Status          SessionStatus
	AgentType       string
	AgentJSON       string
	Title           string
	Agent           string
	CreatedAt       time.Time
	LastActiveAt    time.Time
	SessionSyncJSON string
	InMemory        bool
}
```

Update `SaveSession`, `LoadSession`, and `ListSessions` selects/inserts/scans to read and write `SessionSyncJSON`.

- [ ] **Step 4: Run the focused test to verify it passes**

Run: `cd server && go test ./internal/hub/client -run TestStoreSessionSyncJSONRoundTrip -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add server/internal/hub/client/sqlite_store.go server/internal/hub/client/client_test.go
git commit -m "feat: persist session sync projection"
```

### Task 3: Add File-Backed Prompt History Store

**Files:**
- Create: `server/internal/hub/client/session_history_files.go`
- Modify: `server/internal/hub/client/client_test.go`
- Test: `server/internal/hub/client/client_test.go`

- [ ] **Step 1: Write failing prompt file round-trip tests**

Add this test:

```go
func TestFileSessionHistoryWritesAndReadsPromptSnapshot(t *testing.T) {
	root := t.TempDir()
	store := newFileSessionHistoryStore(root)
	ctx := context.Background()

	prompt := sessionHistoryPrompt{
		SessionID:   "sess-1",
		PromptIndex: 1,
		Title:       "hello",
		ModelName:   "gpt-5",
		StopReason:  "end_turn",
		UpdatedAt:   mustRFC3339Time(t, "2026-05-13T10:01:00Z"),
		Turns: []sessionHistoryTurn{
			{TurnIndex: 1, Method: acp.IMMethodPromptRequest, Finished: true, Content: `{"method":"prompt_request"}`},
			{TurnIndex: 2, Method: acp.IMMethodPromptDone, Finished: true, Content: `{"method":"prompt_done","param":{"stopReason":"end_turn"}}`},
		},
	}
	if err := store.WritePrompt(ctx, "proj1", prompt); err != nil {
		t.Fatalf("WritePrompt: %v", err)
	}

	loaded, err := store.ReadPrompt(ctx, "proj1", "sess-1", 1)
	if err != nil {
		t.Fatalf("ReadPrompt: %v", err)
	}
	if loaded.PromptIndex != 1 || len(loaded.Turns) != 2 {
		t.Fatalf("loaded prompt = %#v", loaded)
	}
	if loaded.Turns[1].Method != acp.IMMethodPromptDone {
		t.Fatalf("last method = %q, want prompt_done", loaded.Turns[1].Method)
	}
}
```

- [ ] **Step 2: Run the focused test to verify it fails**

Run: `cd server && go test ./internal/hub/client -run TestFileSessionHistoryWritesAndReadsPromptSnapshot -count=1`

Expected: FAIL because the file history store does not exist.

- [ ] **Step 3: Implement the minimal file history store**

Create `server/internal/hub/client/session_history_files.go`:

```go
package client

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type sessionHistoryTurn struct {
	TurnIndex int64  `json:"turnIndex"`
	Method    string `json:"method"`
	Finished  bool   `json:"finished"`
	Content   string `json:"content"`
}

type sessionHistoryPrompt struct {
	SchemaVersion int64                `json:"schemaVersion"`
	SessionID     string               `json:"sessionId"`
	PromptIndex   int64                `json:"promptIndex"`
	Title         string               `json:"title,omitempty"`
	ModelName     string               `json:"modelName,omitempty"`
	StartedAt     time.Time            `json:"startedAt,omitempty"`
	UpdatedAt     time.Time            `json:"updatedAt,omitempty"`
	StopReason    string               `json:"stopReason,omitempty"`
	TurnIndex     int64                `json:"turnIndex"`
	Turns         []sessionHistoryTurn `json:"turns"`
}

type fileSessionHistoryStore struct {
	root string
}

func newFileSessionHistoryStore(root string) *fileSessionHistoryStore {
	return &fileSessionHistoryStore{root: root}
}

func (s *fileSessionHistoryStore) WritePrompt(ctx context.Context, projectName string, prompt sessionHistoryPrompt) error {
	_ = ctx
	prompt.SchemaVersion = 1
	prompt.TurnIndex = int64(len(prompt.Turns))
	dir := filepath.Join(s.root, safeHistoryPathPart(projectName), safeHistoryPathPart(prompt.SessionID), "prompts")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir prompt history: %w", err)
	}
	path := filepath.Join(dir, fmt.Sprintf("p%06d.json", prompt.PromptIndex))
	tmp := path + ".tmp"
	raw, err := json.MarshalIndent(prompt, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal prompt history: %w", err)
	}
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return fmt.Errorf("write prompt history temp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("replace prompt history: %w", err)
	}
	return nil
}

func (s *fileSessionHistoryStore) ReadPrompt(ctx context.Context, projectName, sessionID string, promptIndex int64) (sessionHistoryPrompt, error) {
	_ = ctx
	path := filepath.Join(s.root, safeHistoryPathPart(projectName), safeHistoryPathPart(sessionID), "prompts", fmt.Sprintf("p%06d.json", promptIndex))
	raw, err := os.ReadFile(path)
	if err != nil {
		return sessionHistoryPrompt{}, fmt.Errorf("read prompt history: %w", err)
	}
	var prompt sessionHistoryPrompt
	if err := json.Unmarshal(raw, &prompt); err != nil {
		return sessionHistoryPrompt{}, fmt.Errorf("decode prompt history: %w", err)
	}
	return prompt, nil
}

func safeHistoryPathPart(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "_"
	}
	replacer := strings.NewReplacer("\\", "_", "/", "_", ":", "_", "*", "_", "?", "_", "\"", "_", "<", "_", ">", "_", "|", "_")
	return replacer.Replace(value)
}
```

Ensure the file imports `time`.

- [ ] **Step 4: Run the focused test to verify it passes**

Run: `cd server && go test ./internal/hub/client -run TestFileSessionHistoryWritesAndReadsPromptSnapshot -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add server/internal/hub/client/session_history_files.go server/internal/hub/client/client_test.go
git commit -m "feat: add file session history store"
```

### Task 4: Add Startup Migration From `session_prompts`

**Files:**
- Modify: `server/internal/hub/client/session_history_files.go`
- Modify: `server/internal/hub/client/sqlite_store.go`
- Modify: `server/internal/hub/client/client_test.go`
- Test: `server/internal/hub/client/client_test.go`

- [ ] **Step 1: Write failing migration tests**

Add this test:

```go
func TestMigrateSessionPromptsToFilesAppendsPromptDoneTurn(t *testing.T) {
	store := newTempSQLiteStore(t)
	ctx := context.Background()
	root := t.TempDir()
	files := newFileSessionHistoryStore(root)

	if err := store.SaveSession(ctx, &SessionRecord{
		ID:           "sess-1",
		ProjectName:  "proj1",
		Status:       SessionActive,
		AgentType:    "codex",
		CreatedAt:    mustRFC3339Time(t, "2026-05-13T10:00:00Z"),
		LastActiveAt: mustRFC3339Time(t, "2026-05-13T10:01:00Z"),
	}); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}

	turnsJSON := EncodeStoredTurns([]string{
		`{"method":"prompt_request","param":{"contentBlocks":[{"type":"text","text":"hello"}]}}`,
		`{"method":"agent_message_chunk","param":{"text":"answer"}}`,
	})
	if err := store.UpsertSessionPrompt(ctx, SessionPromptRecord{
		SessionID:   "sess-1",
		PromptIndex: 1,
		StopReason:  "end_turn",
		UpdatedAt:   mustRFC3339Time(t, "2026-05-13T10:01:00Z"),
		TurnsJSON:   turnsJSON,
		TurnIndex:   2,
	}); err != nil {
		t.Fatalf("UpsertSessionPrompt: %v", err)
	}

	if err := migrateSessionPromptsToFiles(ctx, store, files, "proj1"); err != nil {
		t.Fatalf("migrateSessionPromptsToFiles: %v", err)
	}

	prompt, err := files.ReadPrompt(ctx, "proj1", "sess-1", 1)
	if err != nil {
		t.Fatalf("ReadPrompt: %v", err)
	}
	if len(prompt.Turns) != 3 {
		t.Fatalf("turns len = %d, want 3", len(prompt.Turns))
	}
	if prompt.Turns[2].Method != acp.IMMethodPromptDone {
		t.Fatalf("last method = %q, want prompt_done", prompt.Turns[2].Method)
	}
}
```

- [ ] **Step 2: Run the focused migration test to verify it fails**

Run: `cd server && go test ./internal/hub/client -run TestMigrateSessionPromptsToFilesAppendsPromptDoneTurn -count=1`

Expected: FAIL because migration does not exist.

- [ ] **Step 3: Implement migration helpers**

Add a migration helper in `session_history_files.go`:

```go
func migrateSessionPromptsToFiles(ctx context.Context, store Store, files *fileSessionHistoryStore, projectName string) error {
	sessions, err := store.ListSessions(ctx, projectName)
	if err != nil {
		return err
	}
	for _, session := range sessions {
		prompts, err := store.ListSessionPrompts(ctx, projectName, session.ID)
		if err != nil {
			return err
		}
		for _, prompt := range prompts {
			turns, err := DecodeStoredTurns(prompt.TurnsJSON)
			if err != nil {
				return err
			}
			historyTurns := make([]sessionHistoryTurn, 0, len(turns)+1)
			for i, content := range turns {
				method := sessionHistoryMethodFromContent(content)
				historyTurns = append(historyTurns, sessionHistoryTurn{
					TurnIndex: int64(i + 1),
					Method:    method,
					Finished:  true,
					Content:   normalizeJSONDoc(content, `{}`),
				})
			}
			if strings.TrimSpace(prompt.StopReason) != "" {
				content := buildIMContentJSON(acp.IMMethodPromptDone, acp.IMPromptResult{StopReason: strings.TrimSpace(prompt.StopReason)})
				historyTurns = append(historyTurns, sessionHistoryTurn{
					TurnIndex: int64(len(historyTurns) + 1),
					Method:    acp.IMMethodPromptDone,
					Finished:  true,
					Content:   content,
				})
			}
			if err := files.WritePrompt(ctx, projectName, sessionHistoryPrompt{
				SessionID:   session.ID,
				PromptIndex: prompt.PromptIndex,
				Title:       prompt.Title,
				ModelName:   prompt.ModelName,
				StartedAt:   prompt.StartedAt,
				UpdatedAt:   prompt.UpdatedAt,
				StopReason:  prompt.StopReason,
				Turns:       historyTurns,
			}); err != nil {
				return err
			}
		}
	}
	return nil
}
```

Add `sessionHistoryMethodFromContent` using `json.Unmarshal` into `acp.IMTurnMessage`.

- [ ] **Step 4: Run the focused migration test to verify it passes**

Run: `cd server && go test ./internal/hub/client -run TestMigrateSessionPromptsToFilesAppendsPromptDoneTurn -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add server/internal/hub/client/session_history_files.go server/internal/hub/client/sqlite_store.go server/internal/hub/client/client_test.go
git commit -m "feat: migrate session prompts to files"
```

### Task 5: Switch Session Reads and Prompt Finish Writes to File History

**Files:**
- Modify: `server/internal/hub/client/session_recorder.go`
- Modify: `server/internal/hub/client/client.go`
- Modify: `server/internal/hub/client/client_test.go`
- Test: `server/internal/hub/client/client_test.go`

- [ ] **Step 1: Write failing read/write integration tests**

Add tests:

```go
func TestPromptFinishedWritesPromptFileBeforePublishingPromptDone(t *testing.T) {
	c := newSessionViewTestClient(t)
	ctx := context.Background()
	c.sessionRecorder.historyStore = newFileSessionHistoryStore(t.TempDir())

	if err := c.RecordEvent(ctx, sessionViewCreatedEvent("sess-1", "File Write Before Done")); err != nil {
		t.Fatalf("RecordEvent created: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewPromptEvent("sess-1", "hello", nil)); err != nil {
		t.Fatalf("RecordEvent prompt: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewPromptDoneEvent("sess-1", "end_turn")); err != nil {
		t.Fatalf("RecordEvent done: %v", err)
	}

	prompt, err := c.sessionRecorder.historyStore.ReadPrompt(ctx, "proj1", "sess-1", 1)
	if err != nil {
		t.Fatalf("ReadPrompt: %v", err)
	}
	if prompt.Turns[len(prompt.Turns)-1].Method != acp.IMMethodPromptDone {
		t.Fatalf("last stored method = %q, want prompt_done", prompt.Turns[len(prompt.Turns)-1].Method)
	}
}

func TestSessionReadUsesFinishedCursorAndPromptFiles(t *testing.T) {
	c := newSessionViewTestClient(t)
	ctx := context.Background()
	c.sessionRecorder.historyStore = newFileSessionHistoryStore(t.TempDir())

	seedPromptWithTurns(t, c, ctx, "sess-1", "hello", []acp.SessionUpdate{
		{SessionUpdate: acp.SessionUpdateAgentMessageChunk, Content: mustJSON(acp.ContentBlock{Type: acp.ContentBlockTypeText, Text: "answer"})},
	})

	_, _, messages, err := c.sessionRecorder.ReadSessionPrompts(ctx, "sess-1", 1, 1)
	if err != nil {
		t.Fatalf("ReadSessionPrompts: %v", err)
	}
	if len(messages) == 0 {
		t.Fatalf("messages len = 0, want turns after cursor")
	}
	for _, message := range messages {
		if message.PromptIndex == 1 && message.TurnIndex <= 1 {
			t.Fatalf("returned message before cursor: %#v", message)
		}
	}
}

func TestStartingNextPromptSynthesizesInterruptedPromptDone(t *testing.T) {
	c := newSessionViewTestClient(t)
	ctx := context.Background()
	c.sessionRecorder.historyStore = newFileSessionHistoryStore(t.TempDir())

	if err := c.RecordEvent(ctx, sessionViewCreatedEvent("sess-1", "Interrupted Prompt")); err != nil {
		t.Fatalf("RecordEvent created: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewPromptEvent("sess-1", "first", nil)); err != nil {
		t.Fatalf("RecordEvent first prompt: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewPromptEvent("sess-1", "second", nil)); err != nil {
		t.Fatalf("RecordEvent second prompt: %v", err)
	}

	_, _, messages, err := c.sessionRecorder.ReadSessionPrompts(ctx, "sess-1", 1, 1)
	if err != nil {
		t.Fatalf("ReadSessionPrompts: %v", err)
	}
	if !hasPromptDoneWithStopReason(messages, 1, "interrupted") {
		t.Fatalf("messages missing interrupted prompt_done: %#v", messages)
	}
}
```

- [ ] **Step 2: Run focused tests to verify they fail**

Run: `cd server && go test ./internal/hub/client -run "TestPromptFinishedWritesPromptFileBeforePublishingPromptDone|TestSessionReadUsesFinishedCursorAndPromptFiles|TestStartingNextPromptSynthesizesInterruptedPromptDone" -count=1`

Expected: FAIL because recorder still persists prompt content to SQLite `session_prompts.turns_json`.

- [ ] **Step 3: Implement file-backed prompt finish and read**

Update `SessionRecorder`:

```go
type SessionRecorder struct {
	projectName  string
	store        Store
	historyStore *fileSessionHistoryStore
	...
}
```

Convert state to file prompt on finish:

```go
func promptHistoryFromState(sessionID string, state *sessionPromptState, stopReason string, updatedAt time.Time) sessionHistoryPrompt {
	turns := make([]sessionHistoryTurn, 0, len(state.turns))
	for _, turn := range state.turns {
		turns = append(turns, sessionHistoryTurn{
			TurnIndex: turn.turnIndex,
			Method:    turn.method,
			Finished:  turn.finished,
			Content:   buildIMContentJSON(turn.method, turn.payload),
		})
	}
	return sessionHistoryPrompt{
		SessionID:   sessionID,
		PromptIndex: state.promptIndex,
		UpdatedAt:   updatedAt,
		StopReason:  stopReason,
		Turns:       turns,
	}
}
```

Use `historyStore.WritePrompt` before publishing the `prompt_done` turn. Before creating a new prompt state, call an `ensurePromptTerminalLocked` style helper that seals the previous non-empty prompt with `prompt_done` and `stopReason: "interrupted"` if it is not already terminal. Update `ReadSessionPrompts` so finished prompts call `historyStore.ReadPrompt`, active prompts still read from memory, and `(P, T)` returns only `turnIndex > T` for prompt `P` plus all later prompts.

- [ ] **Step 4: Run focused tests to verify they pass**

Run: `cd server && go test ./internal/hub/client -run "TestPromptFinishedWritesPromptFileBeforePublishingPromptDone|TestSessionReadUsesFinishedCursorAndPromptFiles|TestStartingNextPromptSynthesizesInterruptedPromptDone" -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add server/internal/hub/client/session_recorder.go server/internal/hub/client/client.go server/internal/hub/client/client_test.go
git commit -m "feat: read session history from prompt files"
```

### Task 6: Update App/Web Finished Cache and Reconnect Logic

**Files:**
- Modify: `app/web/src/types/registry.ts`
- Modify: `app/web/src/services/registryRepository.ts`
- Modify: `app/web/src/services/workspacePersistence.ts`
- Modify: `app/web/src/services/workspaceStore.ts`
- Modify: `app/web/src/chatSync.ts`
- Modify: `app/web/src/main.tsx`
- Modify: `app/__tests__/web-chat-sync-reconcile.test.ts`
- Modify: `app/__tests__/web-reconnect-fallback.test.ts`
- Test: `app/__tests__/web-chat-sync-reconcile.test.ts`
- Test: `app/__tests__/web-reconnect-fallback.test.ts`

- [ ] **Step 1: Write failing app cache tests**

Add expectations to `app/__tests__/web-chat-sync-reconcile.test.ts`:

```ts
it('advances cursor only for finished turns', () => {
  const messages = [
    {sessionId: 's1', promptIndex: 1, turnIndex: 1, method: 'prompt_request', param: {}, finished: true},
    {sessionId: 's1', promptIndex: 1, turnIndex: 2, method: 'agent_message_chunk', param: {text: 'partial'}, finished: false},
  ];

  expect(getLatestSessionReadCursor(messages)).toEqual({promptIndex: 1, turnIndex: 1});
});

it('treats prompt_done as a normal finished cursor turn', () => {
  const messages = [
    {sessionId: 's1', promptIndex: 1, turnIndex: 1, method: 'prompt_request', param: {}, finished: true},
    {sessionId: 's1', promptIndex: 1, turnIndex: 2, method: 'prompt_done', param: {stopReason: 'end_turn'}, finished: true},
  ];

  expect(getLatestSessionReadCursor(messages)).toEqual({promptIndex: 1, turnIndex: 2});
});

it('requests a read when the next prompt arrives before the previous prompt is terminal', () => {
  const local = {
    cursor: {promptIndex: 1, turnIndex: 3},
    terminalPrompts: new Set<number>(),
  };
  const incoming = {sessionId: 's1', promptIndex: 2, turnIndex: 1, method: 'prompt_request', param: {}, finished: true};

  expect(shouldRequestSessionReadForIncomingTurn(local, incoming)).toEqual({promptIndex: 1, turnIndex: 3});
});

it('requests a read when a prompt or turn gap is detected', () => {
  const local = {
    cursor: {promptIndex: 1, turnIndex: 3},
    terminalPrompts: new Set<number>([1]),
  };

  expect(shouldRequestSessionReadForIncomingTurn(local, {sessionId: 's1', promptIndex: 3, turnIndex: 1, method: 'prompt_request', param: {}, finished: true})).toEqual({promptIndex: 1, turnIndex: 3});
  expect(shouldRequestSessionReadForIncomingTurn(local, {sessionId: 's1', promptIndex: 2, turnIndex: 2, method: 'agent_message_chunk', param: {}, finished: true})).toEqual({promptIndex: 1, turnIndex: 3});
  expect(shouldRequestSessionReadForIncomingTurn(local, {sessionId: 's1', promptIndex: 1, turnIndex: 5, method: 'agent_message_chunk', param: {}, finished: true})).toEqual({promptIndex: 1, turnIndex: 3});
});
```

- [ ] **Step 2: Run focused app tests to verify they fail**

Run: `cd app && npm test -- --runTestsByPath __tests__/web-chat-sync-reconcile.test.ts --watch=false --runInBand`

Expected: FAIL because app code still uses `done`, skips `prompt_done` for cursor advancement, or has no realtime gap detector.

- [ ] **Step 3: Implement finished-aware app cache rules**

Update `app/web/src/types/registry.ts`:

```ts
export interface RegistrySessionMessage {
  sessionId: string;
  promptIndex: number;
  turnIndex: number;
  method: string;
  param: Record<string, unknown>;
  finished: boolean;
}
```

Update decode in `registryRepository.ts`:

```ts
const finished = input.finished === true;
return { sessionId, promptIndex, turnIndex, method, param, finished };
```

Update `chatSync.ts`:

```ts
export function getLatestSessionReadCursor(messages: RegistryChatMessage[]): {
  promptIndex: number;
  turnIndex: number;
} {
  return messages.reduce(
    (latest, message) => {
      if (message.finished !== true) return latest;
      const promptIndex = message.promptIndex ?? 0;
      const turnIndex = message.turnIndex ?? 0;
      if (promptIndex > latest.promptIndex || (promptIndex === latest.promptIndex && turnIndex > latest.turnIndex)) {
        return {promptIndex, turnIndex};
      }
      return latest;
    },
    {promptIndex: 0, turnIndex: 0},
  );
}
```

Add a small gap detector in `chatSync.ts` and use it before merging realtime turns:

```ts
export function shouldRequestSessionReadForIncomingTurn(
  local: {cursor: SessionReadCursor; terminalPrompts: ReadonlySet<number>},
  incoming: RegistrySessionMessage,
): SessionReadCursor | null {
  const {promptIndex: p, turnIndex: t} = local.cursor;
  const pi = incoming.promptIndex ?? 0;
  const ti = incoming.turnIndex ?? 0;
  if (pi > p + 1) return local.cursor;
  if (pi === p + 1 && !local.terminalPrompts.has(p)) return local.cursor;
  if (pi === p + 1 && ti !== 1) return local.cursor;
  if (pi === p && ti > t + 1) return local.cursor;
  return null;
}
```

When applying a `prompt_done` turn, mark that prompt terminal. The send path must not create a local durable `prompt_request`; the server-emitted `prompt_request finished=true` turn is the cacheable source of truth.

Update persistence calls so only `finished: true` messages are written to IndexedDB:

```ts
const cacheableMessages = messages.filter(message => message.finished === true);
workspaceStore.rememberChatSessionContent(activeProjectId, sessionId, cacheableMessages, prompts);
```

- [ ] **Step 4: Run focused app tests and typecheck**

Run: `cd app && npm test -- --runTestsByPath __tests__/web-chat-sync-reconcile.test.ts __tests__/web-reconnect-fallback.test.ts --watch=false --runInBand`

Run: `cd app && npm run tsc:web`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add app/web/src/types/registry.ts app/web/src/services/registryRepository.ts app/web/src/services/workspacePersistence.ts app/web/src/services/workspaceStore.ts app/web/src/chatSync.ts app/web/src/main.tsx app/__tests__/web-chat-sync-reconcile.test.ts app/__tests__/web-reconnect-fallback.test.ts
git commit -m "refactor: cache app chat by finished cursor"
```

### Task 7: Update Protocol Documentation

**Files:**
- Modify: `docs/app-chat-recorder-sync-protocol.zh-CN.md`
- Modify: `docs/app-chat-recorder-sync-protocol.md`
- Modify: `docs/session-persistence-sqlite.md`

- [ ] **Step 1: Update app chat protocol docs**

In both app chat protocol documents, replace the old `done` contract with:

```typescript
interface RegistrySessionMessage {
  sessionId: string;
  promptIndex: number;
  turnIndex: number;
  method: string;
  param: Record<string, unknown>;
  finished: boolean;
}
```

Document these rules:

```text
prompt_done is a normal turn and advances the finished cursor.
finished=true means the turn is cacheable and can be used as the retransmission cursor.
finished=false turns are display-only streaming state and are not saved to client IndexedDB.
session.read(P, T) returns the delta after the client's finished cursor, not a full prompt snapshot.
The app does not persist optimistic prompt_request turns; server prompt_request is authoritative.
The server synthesizes prompt_done with interrupted/server_restart/reload_recovered stop reasons when needed.
```

- [ ] **Step 2: Update persistence docs**

In `docs/session-persistence-sqlite.md`, document:

```sql
sessions.session_sync_json TEXT NOT NULL DEFAULT '{}'
```

Add the prompt file layout:

```text
session-history/<project-key>/<session-id>/manifest.json
session-history/<project-key>/<session-id>/prompts/p000001.json
```

- [ ] **Step 3: Verify docs contain the new protocol terms**

Run: `rg -n "finished|session_sync_json|prompt_done is a normal turn|session-history|server prompt_request is authoritative|session.read\\(P, T\\)" docs/app-chat-recorder-sync-protocol.zh-CN.md docs/app-chat-recorder-sync-protocol.md docs/session-persistence-sqlite.md`

Expected: output includes all four terms.

- [ ] **Step 4: Commit**

```bash
git add docs/app-chat-recorder-sync-protocol.zh-CN.md docs/app-chat-recorder-sync-protocol.md docs/session-persistence-sqlite.md
git commit -m "docs: record finished cursor session protocol"
```

### Task 8: Full Verification and Release Tail

**Files:**
- Modify: `server/internal/hub/client/session_recorder.go`
- Modify: `server/internal/hub/client/sqlite_store.go`
- Modify: `server/internal/hub/client/session_history_files.go`
- Modify: `app/web/src/chatSync.ts`
- Modify: `app/web/src/main.tsx`
- Modify: `docs/app-chat-recorder-sync-protocol.zh-CN.md`

- [ ] **Step 1: Run full focused server verification**

Run: `cd server && go test ./internal/hub/client -count=1`

Expected: PASS.

- [ ] **Step 2: Run full server verification**

Run: `cd server && go test ./...`

Expected: PASS.

- [ ] **Step 3: Run app verification**

Run: `cd app && npm test -- --watch=false --runInBand`

Run: `cd app && npm run tsc:web`

Expected: PASS.

- [ ] **Step 4: Run release build after app changes**

Run: `cd app && npm run build:web:release`

Expected: PASS with only known bundle-size warnings.

- [ ] **Step 5: Run repository completion gate**

Run:

```bash
git add -A
git commit -m "feat: store session history as finished prompt files"
git push origin main
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/signal_update_now.ps1 -DelaySeconds 30
cd app && npm run build:web:release
```

Expected: commit succeeds, push succeeds, the server update signal command exits successfully, and the app web release build completes.

## Self-Review

- Spec coverage: the plan covers finished field migration, prompt_done as a real turn, session sync projection, prompt file storage, startup migration, app cache rules, reconnect behavior, and protocol docs.
- Placeholder scan: no deferred implementation markers remain.
- Type consistency: the plan uses `finished`, `session_sync_json`, `sessionHistoryPrompt`, `sessionHistoryTurn`, and finished cursor terminology consistently.
