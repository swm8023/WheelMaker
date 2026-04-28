# Session View UpdateIndex Removal Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove `updateIndex` from the session-view model, keep realtime session message delivery turn-scoped, and make `session.read` return prompt snapshots starting from a client checkpoint.

**Architecture:** Keep the recorder and SQLite store turn-based so same-turn text and tool updates still merge into one durable turn row keyed by `(sessionId, promptIndex, turnIndex)`. Change the outward protocol in two directions: realtime `registry.session.message` publishes only the latest turn body, while `session.read` becomes a reconnect/resync API that returns full prompt snapshots from the client's current `(promptIndex, turnIndex)` checkpoint. App/web adapts by overwriting turn identity on realtime events and by replacing whole prompt state on `session.read`.

**Tech Stack:** Go, SQLite, TypeScript, React, Jest

---

### Task 1: Lock the New Server Contract With Failing Tests

**Files:**
- Modify: `server/internal/hub/client/client_test.go`
- Modify: `server/internal/hub/client/client.go`
- Modify: `server/internal/hub/client/session_recorder.go`
- Test: `server/internal/hub/client/client_test.go`

- [ ] **Step 1: Write the failing server tests**

Add tests in `server/internal/hub/client/client_test.go` that lock the new protocol:

```go
func TestSessionViewPublishesLatestTurnWithoutUpdateIndex(t *testing.T) {
	c := newSessionViewTestClient(t)
	ctx := context.Background()

	if err := c.RecordEvent(ctx, sessionViewCreatedEvent("sess-1", "Task")); err != nil {
		t.Fatalf("RecordEvent created: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewPromptEvent("sess-1", "run", nil)); err != nil {
		t.Fatalf("RecordEvent prompt: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewUpdateEvent("sess-1", acp.SessionUpdate{
		SessionUpdate: acp.SessionUpdateAgentMessageChunk,
		Content:       mustJSON(acp.ContentBlock{Type: acp.ContentBlockTypeText, Text: "hello"}),
	})); err != nil {
		t.Fatalf("RecordEvent update: %v", err)
	}

	last := lastPublishedEvent(t, c, "registry.session.message")
	if _, ok := last["updateIndex"]; ok {
		t.Fatalf("published payload unexpectedly contains updateIndex: %#v", last)
	}
	if got := last["turnIndex"].(int64); got != 2 {
		t.Fatalf("turnIndex = %d, want 2", got)
	}
	if got := strings.TrimSpace(last["content"].(string)); got == "" {
		t.Fatalf("content is empty")
	}
}

func TestSessionReadReturnsCheckpointPromptSnapshotAndLaterPrompts(t *testing.T) {
	c := newSessionViewTestClient(t)
	ctx := context.Background()

	seedPromptWithTurns(t, c, ctx, "sess-1", "first", []acp.SessionUpdate{
		{SessionUpdate: acp.SessionUpdateAgentMessageChunk, Content: mustJSON(acp.ContentBlock{Type: acp.ContentBlockTypeText, Text: "one"})},
	})
	seedPromptWithTurns(t, c, ctx, "sess-1", "second", []acp.SessionUpdate{
		{SessionUpdate: acp.SessionUpdateAgentMessageChunk, Content: mustJSON(acp.ContentBlock{Type: acp.ContentBlockTypeText, Text: "two"})},
	})

	payload := mustJSON(map[string]any{"sessionId": "sess-1", "promptIndex": 1, "turnIndex": 2})
	resp, err := c.HandleSessionRequest(ctx, "session.read", "proj1", payload)
	if err != nil {
		t.Fatalf("HandleSessionRequest(session.read): %v", err)
	}
	body := resp.(map[string]any)
	prompts := body["prompts"].([]sessionPromptSnapshot)
	if len(prompts) != 2 {
		t.Fatalf("prompts len = %d, want 2", len(prompts))
	}
	if prompts[0].PromptIndex != 1 {
		t.Fatalf("prompts[0].PromptIndex = %d, want 1", prompts[0].PromptIndex)
	}
	if prompts[0].TurnIndex != int64(len(prompts[0].Content)) {
		t.Fatalf("prompts[0].TurnIndex = %d, want %d", prompts[0].TurnIndex, len(prompts[0].Content))
	}
	if prompts[1].PromptIndex != 2 {
		t.Fatalf("prompts[1].PromptIndex = %d, want 2", prompts[1].PromptIndex)
	}
}

func TestSessionReadWithoutCheckpointReturnsAllPromptSnapshots(t *testing.T) {
	c := newSessionViewTestClient(t)
	ctx := context.Background()

	seedPromptWithTurns(t, c, ctx, "sess-1", "first", nil)
	seedPromptWithTurns(t, c, ctx, "sess-1", "second", nil)

	resp, err := c.HandleSessionRequest(ctx, "session.read", "proj1", mustJSON(map[string]any{"sessionId": "sess-1"}))
	if err != nil {
		t.Fatalf("HandleSessionRequest(session.read): %v", err)
	}
	body := resp.(map[string]any)
	prompts := body["prompts"].([]sessionPromptSnapshot)
	if len(prompts) != 2 {
		t.Fatalf("prompts len = %d, want 2", len(prompts))
	}
}
```

- [ ] **Step 2: Run the focused server tests to verify they fail**

Run: `go test ./internal/hub/client -run "TestSessionViewPublishesLatestTurnWithoutUpdateIndex|TestSessionReadReturnsCheckpointPromptSnapshotAndLaterPrompts|TestSessionReadWithoutCheckpointReturnsAllPromptSnapshots" -count=1`

Expected: FAIL because `registry.session.message` still publishes `updateIndex`, `session.read` still uses pagination fields, and the read response still returns turn-shaped `messages` rather than prompt snapshots.

- [ ] **Step 3: Write the minimal server protocol scaffolding**

Update `server/internal/hub/client/session_recorder.go` and `server/internal/hub/client/client.go` to introduce prompt-snapshot DTOs and request parsing:

```go
type sessionPromptSnapshot struct {
	SessionID   string   `json:"sessionId"`
	PromptIndex int64    `json:"promptIndex"`
	TurnIndex   int64    `json:"turnIndex"`
	Content     []string `json:"content"`
}

func (r *SessionRecorder) ReadSessionPrompts(ctx context.Context, sessionID string, checkpointPromptIndex, checkpointTurnIndex int64) (sessionViewSummary, []sessionPromptSnapshot, error) {
	panic("implement in Task 3")
}

case "session.read":
	var req struct {
		SessionID   string `json:"sessionId"`
		PromptIndex int64  `json:"promptIndex,omitempty"`
		TurnIndex   int64  `json:"turnIndex,omitempty"`
	}
	...
	summary, prompts, err := c.sessionRecorder.ReadSessionPrompts(ctx, req.SessionID, req.PromptIndex, req.TurnIndex)
	if err != nil {
		return nil, err
	}
	return map[string]any{"session": summary, "prompts": prompts}, nil
```

Also remove `updateIndex` from the realtime publish payload shape in `addMessageTurn`.

- [ ] **Step 4: Run the focused server tests to verify the contract now passes or fails later for the next missing layer**

Run: `go test ./internal/hub/client -run "TestSessionViewPublishesLatestTurnWithoutUpdateIndex|TestSessionReadReturnsCheckpointPromptSnapshotAndLaterPrompts|TestSessionReadWithoutCheckpointReturnsAllPromptSnapshots" -count=1`

Expected: either PASS for the wire contract or fail deeper in store/recorder logic because `updateIndex` still exists in persistence and snapshot building is not yet implemented.

- [ ] **Step 5: Commit**

```bash
git add server/internal/hub/client/client.go server/internal/hub/client/client_test.go server/internal/hub/client/session_recorder.go
git commit -m "refactor: lock session view prompt snapshot contract"
```

### Task 2: Remove `updateIndex` From Turn State and SQLite Storage

**Files:**
- Modify: `server/internal/hub/client/sqlite_store.go`
- Modify: `server/internal/hub/client/session_recorder.go`
- Modify: `server/internal/hub/client/client_test.go`
- Test: `server/internal/hub/client/client_test.go`

- [ ] **Step 1: Write the failing persistence tests**

Add tests in `server/internal/hub/client/client_test.go` that assert one durable turn row remains after same-turn updates:

```go
func TestSessionRecorderOverwritesMergedTurnWithoutUpdateIndex(t *testing.T) {
	c := newSessionViewTestClient(t)
	ctx := context.Background()

	seedPromptWithTurns(t, c, ctx, "sess-1", "run", []acp.SessionUpdate{
		{SessionUpdate: acp.SessionUpdateAgentMessageChunk, Content: mustJSON(acp.ContentBlock{Type: acp.ContentBlockTypeText, Text: "hello"})},
		{SessionUpdate: acp.SessionUpdateAgentMessageChunk, Content: mustJSON(acp.ContentBlock{Type: acp.ContentBlockTypeText, Text: " world"})},
	})

	turns, err := c.store.ListSessionTurns(ctx, "proj1", "sess-1", 1)
	if err != nil {
		t.Fatalf("ListSessionTurns: %v", err)
	}
	if len(turns) != 2 {
		t.Fatalf("turns len = %d, want 2", len(turns))
	}
	if _, ok := any(turns[1]).(interface{ UpdateIndex() int64 }); ok {
		t.Fatalf("turn record still exposes updateIndex")
	}
	msg, err := decodeSessionTurnMessage(turns[1])
	if err != nil {
		t.Fatalf("decodeSessionTurnMessage: %v", err)
	}
	result := msg.payload.(acp.IMTextResult)
	if result.Text != "hello world" {
		t.Fatalf("merged text = %q, want hello world", result.Text)
	}
}
```

- [ ] **Step 2: Run the focused persistence tests to verify they fail**

Run: `go test ./internal/hub/client -run "TestSessionRecorderOverwritesMergedTurnWithoutUpdateIndex|TestMergeTurnMessageMergesTypedTextPayload" -count=1`

Expected: FAIL because `SessionTurnRecord` and `sessionTurnMessage` still carry `UpdateIndex`, and SQLite still reads/writes `update_index`.

- [ ] **Step 3: Write the minimal store and recorder implementation**

Update `server/internal/hub/client/sqlite_store.go`:

```go
type SessionTurnRecord struct {
	SessionID   string
	PromptIndex int64
	TurnIndex   int64
	UpdateJSON  string
}

var expectedStoreSchemaColumns = map[string][]string{
	...
	"session_turns": {"session_id", "prompt_index", "turn_index", "update_json"},
}

func (s *sqliteStore) UpsertSessionTurn(ctx context.Context, rec SessionTurnRecord) error {
	...
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO session_turns (session_id, prompt_index, turn_index, update_json)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(session_id, prompt_index, turn_index) DO UPDATE SET
			update_json = excluded.update_json
	`, rec.SessionID, rec.PromptIndex, rec.TurnIndex, rec.UpdateJSON)
	...
}
```

Add a schema migration helper in the same file:

```go
func migrateSessionTurnsDropUpdateIndex(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE session_turns_next (
			session_id TEXT NOT NULL,
			prompt_index INTEGER NOT NULL,
			turn_index INTEGER NOT NULL,
			update_json TEXT NOT NULL DEFAULT '{}',
			PRIMARY KEY (session_id, prompt_index, turn_index)
		);
		INSERT INTO session_turns_next (session_id, prompt_index, turn_index, update_json)
		SELECT t.session_id, t.prompt_index, t.turn_index, t.update_json
		FROM session_turns t
		JOIN (
			SELECT session_id, prompt_index, turn_index, MAX(update_index) AS max_update_index
			FROM session_turns
			GROUP BY session_id, prompt_index, turn_index
		) latest
		ON latest.session_id = t.session_id
		AND latest.prompt_index = t.prompt_index
		AND latest.turn_index = t.turn_index
		AND latest.max_update_index = t.update_index;
		DROP TABLE session_turns;
		ALTER TABLE session_turns_next RENAME TO session_turns;
	`)
	return err
}
```

Update `server/internal/hub/client/session_recorder.go`:

```go
type sessionTurnMessage struct {
	SessionID   string
	method      string
	payload     any
	PromptIndex int64
	TurnIndex   int64
}

func mergeTurnMessage(existing, incoming sessionTurnMessage, mergeKind sessionTurnMergeKind, turnIndex int64) (sessionTurnMessage, error) {
	merged := sessionTurnMessage{
		SessionID:   strings.TrimSpace(firstNonEmpty(existing.SessionID, incoming.SessionID)),
		method:      strings.TrimSpace(firstNonEmpty(incoming.method, existing.method)),
		payload:     incoming.payload,
		PromptIndex: existing.PromptIndex,
		TurnIndex:   maxInt64(turnIndex, existing.TurnIndex),
	}
	...
	return merged, nil
}
```

- [ ] **Step 4: Run the focused persistence tests to verify they pass**

Run: `go test ./internal/hub/client -run "TestSessionRecorderOverwritesMergedTurnWithoutUpdateIndex|TestMergeTurnMessageMergesTypedTextPayload" -count=1`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add server/internal/hub/client/sqlite_store.go server/internal/hub/client/session_recorder.go server/internal/hub/client/client_test.go
git commit -m "refactor: remove updateindex from session turns"
```

### Task 3: Implement Checkpoint-Based Prompt Snapshot Reads

**Files:**
- Modify: `server/internal/hub/client/session_recorder.go`
- Modify: `server/internal/hub/client/client.go`
- Modify: `server/internal/hub/client/client_test.go`
- Test: `server/internal/hub/client/client_test.go`

- [ ] **Step 1: Write the failing snapshot-builder tests**

Add focused tests in `server/internal/hub/client/client_test.go`:

```go
func TestSessionReadBuildsPromptSnapshotsWithOrderedTurnContent(t *testing.T) {
	c := newSessionViewTestClient(t)
	ctx := context.Background()

	seedPromptWithTurns(t, c, ctx, "sess-1", "run", []acp.SessionUpdate{
		{SessionUpdate: acp.SessionUpdateAgentMessageChunk, Content: mustJSON(acp.ContentBlock{Type: acp.ContentBlockTypeText, Text: "hello"})},
		{SessionUpdate: acp.SessionUpdateToolCall, Title: "ls", ToolCallID: "tool-1", Status: "running", RawOutput: mustJSON("listing")},
	})

	summary, prompts, err := c.sessionRecorder.ReadSessionPrompts(ctx, "sess-1", 1, 2)
	if err != nil {
		t.Fatalf("ReadSessionPrompts: %v", err)
	}
	if summary.SessionID != "sess-1" {
		t.Fatalf("summary.SessionID = %q, want sess-1", summary.SessionID)
	}
	if len(prompts) != 1 {
		t.Fatalf("prompts len = %d, want 1", len(prompts))
	}
	if prompts[0].TurnIndex != 3 {
		t.Fatalf("turnIndex = %d, want 3", prompts[0].TurnIndex)
	}
	if len(prompts[0].Content) != 3 {
		t.Fatalf("content len = %d, want 3", len(prompts[0].Content))
	}
}

func TestSessionReadReturnsAllPromptsWhenCheckpointIsZero(t *testing.T) {
	c := newSessionViewTestClient(t)
	ctx := context.Background()
	seedPromptWithTurns(t, c, ctx, "sess-1", "one", nil)
	seedPromptWithTurns(t, c, ctx, "sess-1", "two", nil)

	_, prompts, err := c.sessionRecorder.ReadSessionPrompts(ctx, "sess-1", 0, 0)
	if err != nil {
		t.Fatalf("ReadSessionPrompts: %v", err)
	}
	if len(prompts) != 2 {
		t.Fatalf("prompts len = %d, want 2", len(prompts))
	}
}
```

- [ ] **Step 2: Run the focused snapshot tests to verify they fail**

Run: `go test ./internal/hub/client -run "TestSessionReadBuildsPromptSnapshotsWithOrderedTurnContent|TestSessionReadReturnsAllPromptsWhenCheckpointIsZero" -count=1`

Expected: FAIL because `ReadSessionPrompts` is not implemented and the recorder still exposes turn rows instead of prompt snapshots.

- [ ] **Step 3: Write the minimal snapshot-read implementation**

In `server/internal/hub/client/session_recorder.go`, add:

```go
func (r *SessionRecorder) ReadSessionPrompts(ctx context.Context, sessionID string, checkpointPromptIndex, _ int64) (sessionViewSummary, []sessionPromptSnapshot, error) {
	sessionID = strings.TrimSpace(sessionID)
	rec, err := r.store.LoadSession(ctx, r.projectName, sessionID)
	if err != nil {
		return sessionViewSummary{}, nil, err
	}
	if rec == nil {
		return sessionViewSummary{}, nil, fmt.Errorf("session not found: %s", sessionID)
	}
	prompts, err := r.store.ListSessionPrompts(ctx, r.projectName, sessionID)
	if err != nil {
		return sessionViewSummary{}, nil, err
	}
	prompts = promptsFromCheckpoint(prompts, checkpointPromptIndex)
	out := make([]sessionPromptSnapshot, 0, len(prompts))
	for _, prompt := range prompts {
		snapshot, err := r.buildPromptSnapshot(ctx, sessionID, prompt.PromptIndex)
		if err != nil {
			return sessionViewSummary{}, nil, err
		}
		out = append(out, snapshot)
	}
	return r.sessionViewSummaryFromRecord(*rec), out, nil
}

func (r *SessionRecorder) buildPromptSnapshot(ctx context.Context, sessionID string, promptIndex int64) (sessionPromptSnapshot, error) {
	turns, err := r.store.ListSessionTurns(ctx, r.projectName, sessionID, promptIndex)
	if err != nil {
		return sessionPromptSnapshot{}, err
	}
	content := make([]string, 0, len(turns))
	for _, turn := range turns {
		content = append(content, normalizeJSONDoc(turn.UpdateJSON, `{}`))
	}
	return sessionPromptSnapshot{
		SessionID:   sessionID,
		PromptIndex: promptIndex,
		TurnIndex:   int64(len(content)),
		Content:     content,
	}, nil
}
```

Also update `HandleSessionRequest` to return `session` + `prompts` only.

- [ ] **Step 4: Run the focused snapshot tests to verify they pass**

Run: `go test ./internal/hub/client -run "TestSessionReadBuildsPromptSnapshotsWithOrderedTurnContent|TestSessionReadReturnsAllPromptsWhenCheckpointIsZero|TestSessionReadReturnsCheckpointPromptSnapshotAndLaterPrompts" -count=1`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add server/internal/hub/client/client.go server/internal/hub/client/session_recorder.go server/internal/hub/client/client_test.go
git commit -m "feat: add checkpoint prompt snapshot session reads"
```

### Task 4: Adapt App/Web Realtime and Reconnect Logic

**Files:**
- Modify: `app/web/src/types/registry.ts`
- Modify: `app/web/src/services/registryRepository.ts`
- Modify: `app/web/src/main.tsx`
- Modify: `app/__tests__/web-chat-ui.test.ts`
- Test: `app/__tests__/web-chat-ui.test.ts`

- [ ] **Step 1: Write the failing app/web tests**

Update `app/__tests__/web-chat-ui.test.ts` to lock the new wire model:

```ts
expect(registryTypes).not.toContain('updateIndex: number;');
expect(registryTypes).toContain('content: string[];');
expect(repositoryTs).toContain("payload: promptIndex > 0 || turnIndex > 0 ? {sessionId, promptIndex, turnIndex} : {sessionId}");
expect(repositoryTs).not.toContain('afterIndex');
expect(repositoryTs).not.toContain('afterSubIndex');
expect(mainTsx).toContain('`${sessionId}:${promptIndex}:${turnIndex}`');
expect(mainTsx).not.toContain('updateIndex');
```

- [ ] **Step 2: Run the focused app/web test to verify it fails**

Run: `npm test -- --runTestsByPath __tests__/web-chat-ui.test.ts --watch=false`

Expected: FAIL because app/web types and request building still depend on `updateIndex`, `afterIndex`, and `afterSubIndex`.

- [ ] **Step 3: Write the minimal app/web implementation**

Update `app/web/src/types/registry.ts`:

```ts
export interface RegistrySessionPromptSnapshot {
  sessionId: string;
  promptIndex: number;
  turnIndex: number;
  content: string[];
}

export interface RegistrySessionReadResponse {
  session: RegistrySessionSummary;
  prompts: RegistrySessionPromptSnapshot[];
  messages: RegistrySessionMessage[];
}

export interface RegistrySessionMessageEventPayload {
  session?: RegistrySessionSummary;
  message?: RegistrySessionMessage;
  sessionId?: string;
  promptIndex?: number;
  turnIndex?: number;
  content?: string;
}
```

Update `app/web/src/services/registryRepository.ts` and `app/web/src/main.tsx`:

```ts
const messageId = `${sessionId}:${promptIndex}:${turnIndex}`;

payload: promptIndex > 0 || turnIndex > 0 ? { sessionId, promptIndex, turnIndex } : { sessionId },

syncSubIndex: turnIndex > 0 ? turnIndex : undefined,
```

In `readSessionByMethod`, map prompt snapshots into local turn/message state by expanding `content[]` and decoding each entry as one turn.

- [ ] **Step 4: Run the focused app/web test and typecheck to verify they pass**

Run: `npm test -- --runTestsByPath __tests__/web-chat-ui.test.ts --watch=false`

Run: `npm run tsc:web`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add app/web/src/types/registry.ts app/web/src/services/registryRepository.ts app/web/src/main.tsx app/__tests__/web-chat-ui.test.ts
git commit -m "refactor: update app session resync protocol"
```

### Task 5: Remove `updateIndex` From Monitor and Final Server Surfaces

**Files:**
- Modify: `server/cmd/wheelmaker-monitor/monitor.go`
- Modify: `server/cmd/wheelmaker-monitor/monitor_test.go`
- Modify: `server/internal/hub/client/client_test.go`
- Test: `server/cmd/wheelmaker-monitor/monitor_test.go`

- [ ] **Step 1: Write the failing monitor test**

Add or update a monitor test:

```go
func TestParseMonitorSessionTurnDoesNotUseUpdateIndexAsSubIndex(t *testing.T) {
	method, role, kind, body, status, requestID, index, subIndex, source, ts := parseMonitorSessionTurn(`{"method":"agent_message","param":{"text":"hello"}}`, "2026-04-28T09:00:00Z", 7)
	if method == "" || role == "" || kind == "" {
		t.Fatalf("unexpected empty parse result: method=%q role=%q kind=%q", method, role, kind)
	}
	if subIndex != 0 {
		t.Fatalf("subIndex = %d, want 0", subIndex)
	}
	_ = body
	_ = status
	_ = requestID
	_ = index
	_ = source
	_ = ts
}
```

- [ ] **Step 2: Run the focused monitor test to verify it fails**

Run: `go test ./cmd/wheelmaker-monitor -run TestParseMonitorSessionTurnDoesNotUseUpdateIndexAsSubIndex -count=1`

Expected: FAIL because `parseMonitorSessionTurn` still accepts and derives sub-index from `updateIndex`.

- [ ] **Step 3: Write the minimal monitor implementation**

Update `server/cmd/wheelmaker-monitor/monitor.go`:

```go
func parseMonitorSessionTurn(updateJSON, promptUpdatedAt string, fallbackIndex int64) (method, role, kind, body, status string, requestID, index, subIndex int64, source, ts string) {
	method = rp.MethodSessionUpdate
	index = fallbackIndex
	subIndex = 0
	ts = strings.TrimSpace(promptUpdatedAt)
	...
}
```

Remove `turnUpdateIndex` from SQL scans and sort behavior.

- [ ] **Step 4: Run the focused monitor test and a focused server regression set**

Run: `go test ./cmd/wheelmaker-monitor -run TestParseMonitorSessionTurnDoesNotUseUpdateIndexAsSubIndex -count=1`

Run: `go test ./internal/hub/client -run "TestSessionViewPublishesLatestTurnWithoutUpdateIndex|TestSessionReadBuildsPromptSnapshotsWithOrderedTurnContent|TestSessionRecorderOverwritesMergedTurnWithoutUpdateIndex" -count=1`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add server/cmd/wheelmaker-monitor/monitor.go server/cmd/wheelmaker-monitor/monitor_test.go server/internal/hub/client/client_test.go
git commit -m "refactor: remove updateindex from monitor session history"
```

### Task 6: Full Verification and Release Tail

**Files:**
- Modify: `server/internal/hub/client/session_recorder.go`
- Modify: `server/internal/hub/client/sqlite_store.go`
- Modify: `server/internal/hub/client/client.go`
- Modify: `app/web/src/services/registryRepository.ts`
- Modify: `app/web/src/main.tsx`
- Modify: `server/cmd/wheelmaker-monitor/monitor.go`

- [ ] **Step 1: Run focused Go package tests**

Run: `go test ./internal/hub/client -count=1`

Expected: PASS

- [ ] **Step 2: Run focused monitor tests**

Run: `go test ./cmd/wheelmaker-monitor -count=1`

Expected: PASS

- [ ] **Step 3: Run focused app/web verification**

Run: `cd ..\app && npm test -- --watch=false --runInBand`

Run: `cd ..\app && npm run tsc:web`

Expected: PASS

- [ ] **Step 4: Run full server verification**

Run: `go test ./...`

Expected: PASS

- [ ] **Step 5: Run required completion gate**

Run:

```bash
git add -A
git commit -m "refactor: remove updateindex from session view"
git push origin main
powershell -NoProfile -ExecutionPolicy Bypass -File ..\scripts\signal_update_now.ps1 -DelaySeconds 30
cd ..\app && npm run build:web:release
```

Expected: commit succeeds, push succeeds, server update signal launches, and the app web release build completes successfully.

## Self-Review

- Spec coverage: server protocol cleanup, SQLite migration, checkpoint-based prompt resync, app/web adaptation, and monitor cleanup all have dedicated tasks.
- Placeholder scan: no `TBD`, `TODO`, or deferred implementation markers remain.
- Type consistency: the plan uses `sessionPromptSnapshot`, `promptIndex`, `turnIndex`, and `content []string` consistently across server and app/web tasks.