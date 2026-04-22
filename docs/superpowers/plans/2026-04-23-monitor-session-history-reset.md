# Monitor Session History Reset Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a monitor action and dashboard button that clears `session_prompts` and `session_turns`, while simplifying session-view payloads by removing no-op and redundant fields.

**Architecture:** Extend the existing `monitor.action` path with a new `clear-session-history` action, clear persisted prompt/turn rows in one transaction, and reset recorder prompt state through a small runtime hook. In parallel, tighten the session-view wire model by removing `session.markRead`, renaming the message DTO, and dropping redundant `turnId`, `status`, and `projectName` fields from session-facing payloads. Update the app web repository to derive message IDs from indexes so the protocol cleanup stays behaviorally stable.

**Tech Stack:** Go, SQLite, embedded monitor dashboard JavaScript, TypeScript, Jest

---

### Task 1: Lock Down Server-Side Protocol Cleanup With Failing Tests

**Files:**
- Modify: `server/internal/hub/client/client_test.go`
- Modify: `server/internal/hub/client/session_recorder.go`
- Test: `server/internal/hub/client/client_test.go`

- [ ] **Step 1: Write the failing tests**

Add/replace tests in `server/internal/hub/client/client_test.go` that assert:

```go
func TestSessionReadOmitsTurnIDAndSummaryExtras(t *testing.T) {
	c := newSessionViewTestClient(t)
	ctx := context.Background()

	if err := c.RecordEvent(ctx, sessionViewCreatedEvent("sess-1", "Task")); err != nil {
		t.Fatalf("RecordEvent session created: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewPromptEvent("sess-1", "run", nil)); err != nil {
		t.Fatalf("RecordEvent prompt: %v", err)
	}

	payload, err := json.Marshal(map[string]any{"sessionId": "sess-1"})
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	resp, err := c.HandleSessionRequest(ctx, "session.read", "proj1", payload)
	if err != nil {
		t.Fatalf("HandleSessionRequest: %v", err)
	}
	body := resp.(map[string]any)
	summary := body["session"].(sessionViewSummary)
	if summary.Status != "" {
		t.Fatalf("summary.Status = %q, want empty", summary.Status)
	}
	if summary.ProjectName != "" {
		t.Fatalf("summary.ProjectName = %q, want empty", summary.ProjectName)
	}
	messages := body["messages"].([]sessionViewMessage)
	if len(messages) != 1 {
		t.Fatalf("messages len = %d, want 1", len(messages))
	}
}

func TestHandleSessionRequestMarkReadIsUnsupported(t *testing.T) {
	c := newSessionViewTestClient(t)
	_, err := c.HandleSessionRequest(context.Background(), "session.markRead", "proj1", []byte(`{"sessionId":"sess-1"}`))
	if err == nil {
		t.Fatalf("expected session.markRead to be unsupported")
	}
}
```

- [ ] **Step 2: Run the focused server tests to verify they fail**

Run: `go test ./internal/hub/client -run "TestSessionReadOmitsTurnIDAndSummaryExtras|TestHandleSessionRequestMarkReadIsUnsupported|TestSessionRecorderMarkReadReturnsSummaryWithoutUnreadState"`

Expected: FAIL because `session.markRead` still exists, `sessionReadMessage` is still the read DTO type, and summary/message payloads still expose removed fields.

- [ ] **Step 3: Write the minimal server implementation**

Update `server/internal/hub/client/session_recorder.go` and `server/internal/hub/client/client.go` to:

```go
type sessionViewSummary struct {
	SessionID string `json:"sessionId"`
	Title     string `json:"title"`
	UpdatedAt string `json:"updatedAt"`
	Agent     string `json:"agent,omitempty"`
}

type sessionViewMessage struct {
	SessionID   string `json:"sessionId"`
	PromptIndex int64  `json:"promptIndex"`
	TurnIndex   int64  `json:"turnIndex"`
	UpdateIndex int64  `json:"updateIndex"`
	Content     string `json:"content"`
}

func toSessionViewMessage(turn SessionTurnRecord) sessionViewMessage {
	content := strings.TrimSpace(turn.UpdateJSON)
	if content == "" {
		content = "{}"
	}
	return sessionViewMessage{
		SessionID:   strings.TrimSpace(turn.SessionID),
		PromptIndex: turn.PromptIndex,
		TurnIndex:   turn.TurnIndex,
		UpdateIndex: turn.UpdateIndex,
		Content:     content,
	}
}
```

And in `HandleSessionRequest` remove the `session.markRead` branch entirely so unsupported methods fall through to the existing error path.

- [ ] **Step 4: Run the focused server tests to verify they pass**

Run: `go test ./internal/hub/client -run "TestSessionReadOmitsTurnIDAndSummaryExtras|TestHandleSessionRequestMarkReadIsUnsupported"`

Expected: PASS

### Task 2: Add a Failing Monitor Reset Test Before Wiring the New Action

**Files:**
- Modify: `server/cmd/wheelmaker-monitor/monitor_test.go`
- Modify: `server/internal/hub/hub_monitor.go`
- Test: `server/cmd/wheelmaker-monitor/monitor_test.go`

- [ ] **Step 1: Write the failing monitor action test**

Add a test in `server/cmd/wheelmaker-monitor/monitor_test.go`:

```go
func TestExecuteActionClearSessionHistoryDeletesPromptAndTurnTables(t *testing.T) {
	base := t.TempDir()
	store, err := clientpkg.NewStore(filepath.Join(base, "db", "client.sqlite3"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	if err := store.SaveSession(ctx, &clientpkg.SessionRecord{ID: "sess-1", ProjectName: "proj1", Status: clientpkg.SessionActive, CreatedAt: time.Now().UTC(), LastActiveAt: time.Now().UTC()}); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}
	if err := store.UpsertSessionPrompt(ctx, clientpkg.SessionPromptRecord{SessionID: "sess-1", PromptIndex: 1, Title: "hello", UpdatedAt: time.Now().UTC()}); err != nil {
		t.Fatalf("UpsertSessionPrompt: %v", err)
	}
	if err := store.UpsertSessionTurn(ctx, clientpkg.SessionTurnRecord{SessionID: "sess-1", PromptIndex: 1, TurnIndex: 1, UpdateIndex: 1, UpdateJSON: "{}", ExtraJSON: "{}"}); err != nil {
		t.Fatalf("UpsertSessionTurn: %v", err)
	}

	mon := NewMonitor(base)
	if err := mon.ExecuteAction("clear-session-history"); err != nil {
		t.Fatalf("ExecuteAction(clear-session-history): %v", err)
	}

	prompts, err := store.ListSessionPrompts(ctx, "proj1", "sess-1")
	if err != nil {
		t.Fatalf("ListSessionPrompts: %v", err)
	}
	if len(prompts) != 0 {
		t.Fatalf("prompts len = %d, want 0", len(prompts))
	}
	turns, err := store.ListSessionTurns(ctx, "proj1", "sess-1", 1)
	if err != nil {
		t.Fatalf("ListSessionTurns: %v", err)
	}
	if len(turns) != 0 {
		t.Fatalf("turns len = %d, want 0", len(turns))
	}
}
```

- [ ] **Step 2: Run the focused monitor test to verify it fails**

Run: `go test ./cmd/wheelmaker-monitor -run TestExecuteActionClearSessionHistoryDeletesPromptAndTurnTables`

Expected: FAIL with `unsupported action: clear-session-history`

- [ ] **Step 3: Implement the minimal monitor action hook**

Update `server/internal/hub/hub_monitor.go` to add the action case and a reset hook shape such as:

```go
type MonitorCore struct {
	BaseDir                  string
	ResetSessionPromptState  func()
}

func (c *MonitorCore) ExecuteAction(action string) error {
	switch strings.TrimSpace(strings.ToLower(action)) {
	case "clear-session-history":
		return c.clearSessionHistory()
	}
	...
}
```

Implement `clearSessionHistory()` to open SQLite, run a transaction deleting `session_turns` then `session_prompts`, commit, and call `ResetSessionPromptState()` when present.

- [ ] **Step 4: Run the focused monitor test to verify it passes**

Run: `go test ./cmd/wheelmaker-monitor -run TestExecuteActionClearSessionHistoryDeletesPromptAndTurnTables`

Expected: PASS

### Task 3: Wire Runtime Recorder Reset and Dashboard Trigger

**Files:**
- Modify: `server/internal/hub/client/session_recorder.go`
- Modify: `server/cmd/wheelmaker-monitor/transport.go`
- Modify: `server/cmd/wheelmaker-monitor/dashboard.go`
- Test: `server/internal/hub/client/client_test.go`

- [ ] **Step 1: Write failing tests for recorder reset and realtime payload cleanup**

Add tests in `server/internal/hub/client/client_test.go` that assert:

```go
func TestSessionRecorderResetPromptStateRestartsIndexes(t *testing.T) {
	c := newSessionViewTestClient(t)
	ctx := context.Background()

	if err := c.RecordEvent(ctx, sessionViewCreatedEvent("sess-1", "Task")); err != nil {
		t.Fatalf("RecordEvent session created: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewPromptEvent("sess-1", "first", nil)); err != nil {
		t.Fatalf("RecordEvent prompt: %v", err)
	}
	c.sessionRecorder.ResetPromptState()
	if err := c.store.DeleteAllSessionTurnsAndPrompts(ctx); err != nil {
		t.Fatalf("DeleteAllSessionTurnsAndPrompts: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewPromptEvent("sess-1", "second", nil)); err != nil {
		t.Fatalf("RecordEvent prompt after reset: %v", err)
	}
	_, messages, _, _, err := c.sessionRecorder.ReadSessionMessages(ctx, "sess-1", 0, 0)
	if err != nil {
		t.Fatalf("ReadSessionMessages: %v", err)
	}
	if messages[0].PromptIndex != 1 {
		t.Fatalf("messages[0].PromptIndex = %d, want 1", messages[0].PromptIndex)
	}
}
```

And a realtime publish assertion that the published map no longer contains `turnId`.

- [ ] **Step 2: Run the focused server tests to verify they fail**

Run: `go test ./internal/hub/client -run "TestSessionRecorderResetPromptStateRestartsIndexes|TestSessionViewPublishMessageOmitsTurnID"`

Expected: FAIL because no reset method/hook exists and realtime events still publish `turnId`.

- [ ] **Step 3: Implement runtime reset and dashboard button**

Update `server/internal/hub/client/session_recorder.go` to add:

```go
func (r *SessionRecorder) ResetPromptState() {
	if r == nil {
		return
	}
	r.writeMu.Lock()
	r.promptState = map[string]sessionPromptState{}
	r.writeMu.Unlock()
}
```

Update the monitor transport/bootstrap so `MonitorCore.ResetSessionPromptState` is populated from the active hub client runtime.

Update `server/cmd/wheelmaker-monitor/dashboard.go` to add a DB-area button and JS handler:

```javascript
async function clearSessionHistory() {
  if (!confirm('Clear all session prompts and turns for this hub?')) return;
  await apiHub('action', {
    method: 'POST',
    body: JSON.stringify({ action: 'clear-session-history' }),
  });
  await loadDB();
}
```

- [ ] **Step 4: Run the focused server tests to verify they pass**

Run: `go test ./internal/hub/client -run "TestSessionRecorderResetPromptStateRestartsIndexes|TestSessionViewPublishMessageOmitsTurnID"`

Expected: PASS

### Task 4: Update App Web Protocol Consumers With Failing Tests First

**Files:**
- Modify: `app/web/src/types/registry.ts`
- Modify: `app/web/src/services/registryRepository.ts`
- Modify: `app/web/src/services/registryWorkspaceService.ts`
- Modify: `app/web/src/main.tsx`
- Modify: `app/__tests__/web-chat-ui.test.ts`
- Test: `app/__tests__/web-chat-ui.test.ts`

- [ ] **Step 1: Write the failing app tests**

Extend `app/__tests__/web-chat-ui.test.ts` with assertions that:

```ts
expect(repositoryTs).not.toContain("method: 'session.markRead'");
expect(repositoryTs).not.toContain('turnId = typeof input.turnId');
expect(registryTypes).not.toContain('turnId: string;');
expect(registryTypes).not.toContain('status?: string;');
expect(mainTsx).toContain('`${sessionId}:${promptIndex}:${turnIndex}:${updateIndex}`');
```

- [ ] **Step 2: Run the focused app test to verify it fails**

Run: `cd app && npm test -- --runInBand __tests__/web-chat-ui.test.ts`

Expected: FAIL because the repository and types still expose `markSessionRead`, `turnId`, and summary `status`.

- [ ] **Step 3: Implement the minimal app changes**

Update app files to:

```ts
export interface RegistrySessionSummary {
  sessionId: string;
  title: string;
  preview: string;
  updatedAt: string;
  messageCount: number;
  unreadCount?: number;
  agent?: string;
}

export interface RegistrySessionTurn {
  promptIndex: number;
  turnIndex: number;
  updateIndex: number;
  ...
}
```

Remove `markSessionRead()` from `registryRepository.ts` and any forwarding wrapper in `registryWorkspaceService.ts`.

In both session message normalization paths, compute:

```ts
const messageId = `${sessionId}:${promptIndex}:${turnIndex}:${updateIndex}`;
```

without reading `turnId`.

- [ ] **Step 4: Run the focused app test to verify it passes**

Run: `cd app && npm test -- --runInBand __tests__/web-chat-ui.test.ts`

Expected: PASS

### Task 5: Run Focused Regression Verification Before Final Integration Steps

**Files:**
- Modify: `server/internal/hub/client/client_test.go`
- Modify: `server/cmd/wheelmaker-monitor/monitor_test.go`
- Modify: `app/__tests__/web-chat-ui.test.ts`

- [ ] **Step 1: Run the focused server regression suite**

Run: `go test ./cmd/wheelmaker-monitor ./internal/hub/client -run "TestExecuteActionClearSessionHistoryDeletesPromptAndTurnTables|TestSessionReadOmitsTurnIDAndSummaryExtras|TestHandleSessionRequestMarkReadIsUnsupported|TestSessionRecorderResetPromptStateRestartsIndexes|TestSessionViewPublishMessageOmitsTurnID"`

Expected: PASS

- [ ] **Step 2: Run the focused app regression suite**

Run: `cd app && npm test -- --runInBand __tests__/web-chat-ui.test.ts`

Expected: PASS

- [ ] **Step 3: Run broader touched-slice validation**

Run: `go test ./cmd/wheelmaker-monitor ./internal/hub/client`

Expected: PASS

- [ ] **Step 4: Commit the integrated change**

Run:

```bash
git add server/cmd/wheelmaker-monitor server/internal/hub/client app/web/src app/__tests__/web-chat-ui.test.ts docs/superpowers/specs/2026-04-23-monitor-session-history-reset-design.md docs/superpowers/plans/2026-04-23-monitor-session-history-reset.md
git commit -m "feat: reset session history from monitor"
```