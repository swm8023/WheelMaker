# App Project Session Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build project-wide app session browsing and messaging keyed by `sessionId`, backed by durable aggregated server-side history, while keeping app and Feishu on the existing ACP and IM flow.

**Architecture:** Extend the current client SQLite store with session-list projection fields and a durable message history table, add a focused session view service with an in-memory history aggregator, expose new `session.*` registry APIs and push events, then migrate the app web client from `chatId` state to `sessionId` state. Keep `im.Router` as the delivery layer and make app plus Feishu coexist instead of replacing ACP or inventing a second messaging stack.

**Tech Stack:** Go, SQLite (`modernc.org/sqlite`), Jest, TypeScript, React, registry WebSocket protocol.

**Spec:** `docs/superpowers/specs/2026-04-12-app-session-design.md`

---

## File Map

| Action | File | Responsibility |
|--------|------|----------------|
| Modify | `server/internal/hub/client/sqlite_store.go` | Add session projection fields and durable session message/history persistence APIs |
| Modify | `server/internal/hub/client/store_test.go` | Add schema and round-trip tests for session list projection and message history |
| Create | `server/internal/hub/sessionview/service.go` | Project-level session view service and public read APIs |
| Create | `server/internal/hub/sessionview/aggregator.go` | In-memory aggregation of runtime events into durable history rows |
| Create | `server/internal/hub/sessionview/service_test.go` | Session view and aggregation behavior tests |
| Modify | `server/internal/hub/client/session.go` | Emit semantic session events during prompt, permission, and system flows |
| Modify | `server/internal/hub/client/permission.go` | Forward permission lifecycle events to session view service |
| Modify | `server/internal/im/app/app.go` | Remove app-private durable session ownership; treat app as a channel adapter and compatibility shim |
| Modify | `server/internal/hub/hub.go` | Register app and Feishu together and wire session view service into project startup |
| Modify | `server/internal/hub/reporter.go` | Route `session.*` requests to the session view service |
| Modify | `server/internal/registry/server.go` | Forward `session.*` methods and broadcast `session.updated` / `session.message` events |
| Modify | `server/internal/registry/server_test.go` | Add registry tests for `session.*` forwarding and event broadcasting |
| Modify | `app/web/src/types/registry.ts` | Replace chat-centric app types with session-centric registry types |
| Modify | `app/web/src/services/registryRepository.ts` | Add `session.*` request methods and event types |
| Modify | `app/web/src/services/registryWorkspaceService.ts` | Expose session-centric app service methods |
| Modify | `app/web/src/main.tsx` | Migrate sidebar, message pane, and realtime state from `chatId` to `sessionId` |
| Modify | `app/__tests__/web-chat-ui.test.ts` | Replace chat protocol assertions with session protocol assertions |
| Create | `app/__tests__/web-session-ui.test.ts` | Guard the new session-centric UI and event model |

---

## Task 1: Extend SQLite Persistence for Session Projection and History

**Files:**
- Modify: `server/internal/hub/client/sqlite_store.go`
- Modify: `server/internal/hub/client/store_test.go`

- [ ] **Step 1: Write failing store tests for session projection fields and durable history**

Add tests that assert:

```go
func TestStoreSessionProjectionRoundTrip(t *testing.T) {
    store, _ := NewStore(filepath.Join(t.TempDir(), "client.sqlite3"))
    defer store.Close()

    rec := &SessionRecord{
        ID:                "sess-1",
        ProjectName:       "proj1",
        Status:            SessionActive,
        LastReply:         "legacy",
        CreatedAt:         time.Date(2026, 4, 12, 10, 0, 0, 0, time.UTC),
        LastActiveAt:      time.Date(2026, 4, 12, 10, 5, 0, 0, time.UTC),
        Title:             "Fix app sessions",
        LastMessagePreview:"hello world",
        LastMessageAt:     time.Date(2026, 4, 12, 10, 5, 0, 0, time.UTC),
        MessageCount:      3,
    }

    require.NoError(t, store.SaveSession(context.Background(), rec))
    entries, err := store.ListSessions(context.Background(), "proj1")
    require.NoError(t, err)
    require.Len(t, entries, 1)
    require.Equal(t, "Fix app sessions", entries[0].Title)
    require.Equal(t, "hello world", entries[0].Preview)
    require.Equal(t, 3, entries[0].MessageCount)
}

func TestStoreSessionMessageHistoryRoundTrip(t *testing.T) {
    store, _ := NewStore(filepath.Join(t.TempDir(), "client.sqlite3"))
    defer store.Close()

    msg := SessionMessageRecord{
        MessageID:    "msg-1",
        SessionID:    "sess-1",
        ProjectName:  "proj1",
        Role:         "assistant",
        Kind:         "text",
        Body:         "aggregated reply",
        Status:       "done",
        AggregateKey: "assistant:sess-1:turn-1",
    }

    require.NoError(t, store.AppendSessionMessage(context.Background(), msg))
    messages, err := store.ListSessionMessages(context.Background(), "proj1", "sess-1")
    require.NoError(t, err)
    require.Len(t, messages, 1)
    require.Equal(t, "aggregated reply", messages[0].Body)
}
```

- [ ] **Step 2: Run store tests to verify they fail**

Run: `cd server && go test ./internal/hub/client -run "Store(SessionProjection|SessionMessageHistory)" -v`

Expected: FAIL because the new fields, schema columns, and history APIs do not exist yet.

- [ ] **Step 3: Add minimal schema and store support**

Implement:

```go
type SessionRecord struct {
    ID                 string
    ProjectName        string
    Status             SessionStatus
    LastReply          string
    ACPSessionID       string
    AgentsJSON         string
    CreatedAt          time.Time
    LastActiveAt       time.Time
    Title              string
    LastMessagePreview string
    LastMessageAt      time.Time
    MessageCount       int
}

type SessionListEntry struct {
    ID           string
    Agent        string
    Title        string
    Preview      string
    Status       SessionStatus
    MessageCount int
    CreatedAt    time.Time
    LastActiveAt time.Time
    LastMessageAt time.Time
}

type SessionMessageRecord struct {
    MessageID    string
    SessionID    string
    ProjectName  string
    Role         string
    Kind         string
    Body         string
    Status       string
    SourceChannel string
    SourceChatID string
    RequestID    int64
    AggregateKey string
    CreatedAt    time.Time
    UpdatedAt    time.Time
}
```

Add a `session_messages` table plus store methods:

```go
AppendSessionMessage(ctx context.Context, rec SessionMessageRecord) error
UpsertSessionMessage(ctx context.Context, rec SessionMessageRecord) error
ListSessionMessages(ctx context.Context, projectName, sessionID string) ([]SessionMessageRecord, error)
```

Also migrate `sessions` table with `title`, `last_message_preview`, `last_message_at`, and `message_count` columns.

- [ ] **Step 4: Run store tests to verify they pass**

Run: `cd server && go test ./internal/hub/client -run "Store(SessionProjection|SessionMessageHistory)" -v`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add server/internal/hub/client/sqlite_store.go server/internal/hub/client/store_test.go
git commit -m "feat(server): persist session projections and message history"
```

---

## Task 2: Add Session View Service and Aggregated History Projection

**Files:**
- Create: `server/internal/hub/sessionview/service.go`
- Create: `server/internal/hub/sessionview/aggregator.go`
- Create: `server/internal/hub/sessionview/service_test.go`
- Modify: `server/internal/hub/client/session.go`
- Modify: `server/internal/hub/client/permission.go`

- [ ] **Step 1: Write failing tests for aggregation and read APIs**

Create tests that assert:

```go
func TestSessionViewAggregatesAssistantChunksIntoSingleMessage(t *testing.T) {
    store, _ := client.NewStore(filepath.Join(t.TempDir(), "client.sqlite3"))
    svc := NewService("proj1", store)

    svc.RecordEvent(SessionEvent{Type: EventSessionCreated, SessionID: "sess-1", Title: "New Session"})
    svc.RecordEvent(SessionEvent{Type: EventAssistantChunk, SessionID: "sess-1", Kind: "assistant", Text: "hello"})
    svc.RecordEvent(SessionEvent{Type: EventAssistantChunk, SessionID: "sess-1", Kind: "assistant", Text: " world"})
    svc.RecordEvent(SessionEvent{Type: EventPromptFinished, SessionID: "sess-1"})

    messages, _ := svc.ReadSessionMessages(context.Background(), "sess-1")
    require.Len(t, messages, 1)
    require.Equal(t, "hello world", messages[0].Body)
}

func TestSessionViewListIncludesProjectionFields(t *testing.T) {
    store, _ := client.NewStore(filepath.Join(t.TempDir(), "client.sqlite3"))
    svc := NewService("proj1", store)

    svc.RecordEvent(SessionEvent{Type: EventSessionCreated, SessionID: "sess-1", Title: "Task"})
    svc.RecordEvent(SessionEvent{Type: EventUserMessageAccepted, SessionID: "sess-1", Text: "hello"})

    sessions, _ := svc.ListSessions(context.Background())
    require.Len(t, sessions, 1)
    require.Equal(t, "Task", sessions[0].Title)
    require.Equal(t, "hello", sessions[0].Preview)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd server && go test ./internal/hub/sessionview -v`

Expected: FAIL because the package and service do not exist yet.

- [ ] **Step 3: Implement minimal session view service and event projection**

Add focused types:

```go
type SessionEventType string

const (
    EventSessionCreated      SessionEventType = "session_created"
    EventUserMessageAccepted SessionEventType = "user_message_accepted"
    EventAssistantChunk      SessionEventType = "assistant_chunk"
    EventThoughtChunk        SessionEventType = "thought_chunk"
    EventToolUpdated         SessionEventType = "tool_updated"
    EventPermissionRequested SessionEventType = "permission_requested"
    EventPermissionResolved  SessionEventType = "permission_resolved"
    EventPromptFinished      SessionEventType = "prompt_finished"
    EventSystemMessage       SessionEventType = "system_message"
)

type SessionEvent struct {
    Type       SessionEventType
    SessionID  string
    Title      string
    Role       string
    Kind       string
    Text       string
    RequestID  int64
    Status     string
}
```

Implement `RecordEvent`, `ListSessions`, `ReadSession`, and `ReadSessionMessages`. Keep aggregation rules small: user writes immediately, assistant and thought aggregate until `EventPromptFinished`, permission rows upsert by request ID, tool rows upsert by aggregate key.

- [ ] **Step 4: Wire runtime events from session and permission flows**

In `session.go` and `permission.go`, call the session view service when:

```go
svc.RecordEvent(SessionEvent{Type: EventUserMessageAccepted, SessionID: s.ID, Text: promptText})
svc.RecordEvent(SessionEvent{Type: EventAssistantChunk, SessionID: s.ID, Kind: "assistant", Text: chunk})
svc.RecordEvent(SessionEvent{Type: EventThoughtChunk, SessionID: s.ID, Kind: "thought", Text: chunk})
svc.RecordEvent(SessionEvent{Type: EventPromptFinished, SessionID: s.ID})
svc.RecordEvent(SessionEvent{Type: EventPermissionRequested, SessionID: s.ID, RequestID: requestID, Text: title, Status: "needs_action"})
svc.RecordEvent(SessionEvent{Type: EventPermissionResolved, SessionID: s.ID, RequestID: requestID, Status: "done"})
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd server && go test ./internal/hub/sessionview ./internal/hub/client -run "SessionView|Permission|Prompt" -v`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add server/internal/hub/sessionview server/internal/hub/client/session.go server/internal/hub/client/permission.go
git commit -m "feat(server): add session view service and history aggregation"
```

---

## Task 3: Add Session-Centric Registry APIs and Enable App + Feishu Coexistence

**Files:**
- Modify: `server/internal/hub/hub.go`
- Modify: `server/internal/hub/reporter.go`
- Modify: `server/internal/registry/server.go`
- Modify: `server/internal/registry/server_test.go`
- Modify: `server/internal/im/app/app.go`

- [ ] **Step 1: Write failing registry tests for `session.*` methods and events**

Add tests that assert:

```go
func TestRegistryForwardsSessionReadRequests(t *testing.T) {
    // connect hub and client
    // client sends session.read
    // hub receives session.read forward
    // hub replies with session metadata plus messages
    // client receives matching response
}

func TestRegistryBroadcastsSessionMessageEvents(t *testing.T) {
    // hub sends registry.session.message
    // client receives event session.message for same project
}
```

- [ ] **Step 2: Run registry tests to verify they fail**

Run: `cd server && go test ./internal/registry -run "Session(Read|Message)" -v`

Expected: FAIL because `session.*` forwarding and events do not exist.

- [ ] **Step 3: Implement new methods and event plumbing**

Update registry and reporter forwarding lists to include:

```go
"session.list", "session.read", "session.new", "session.send", "session.markRead"
```

Add broadcast support for:

```go
"registry.session.updated" -> event "session.updated"
"registry.session.message" -> event "session.message"
```

In `hub.go`, register both channels when configured:

```go
if pc.HasFeishu() {
    _ = router.RegisterChannel(imfeishu.New(...))
}
appChannel := imapp.New(viewService)
_ = router.RegisterChannel(appChannel)
```

Treat `im/app` as an adapter and compatibility shim, not as the durable owner of session state.

- [ ] **Step 4: Run registry and hub tests to verify they pass**

Run: `cd server && go test ./internal/registry ./internal/hub ./internal/im/app -run "Session|Chat|Coexist" -v`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add server/internal/hub/hub.go server/internal/hub/reporter.go server/internal/registry/server.go server/internal/registry/server_test.go server/internal/im/app/app.go
git commit -m "feat(server): expose session registry APIs and enable app plus feishu coexistence"
```

---

## Task 4: Migrate App Web from `chatId` State to `sessionId` State

**Files:**
- Modify: `app/web/src/types/registry.ts`
- Modify: `app/web/src/services/registryRepository.ts`
- Modify: `app/web/src/services/registryWorkspaceService.ts`
- Modify: `app/web/src/main.tsx`
- Modify: `app/__tests__/web-chat-ui.test.ts`
- Create: `app/__tests__/web-session-ui.test.ts`

- [ ] **Step 1: Write failing app tests for `session.*` protocol and UI state**

Create assertions like:

```ts
expect(registryTypes).toContain('export interface RegistrySessionSummary');
expect(repositoryTs).toContain("method: 'session.list'");
expect(repositoryTs).toContain("method: 'session.read'");
expect(repositoryTs).toContain("method: 'session.new'");
expect(repositoryTs).toContain("method: 'session.send'");
expect(workspaceServiceTs).toContain('async listSessions(');
expect(mainTsx).toContain('selectedSessionId');
expect(mainTsx).toContain('session.message');
expect(mainTsx).not.toContain('selectedChatId');
```

- [ ] **Step 2: Run app tests to verify they fail**

Run: `cd app && npm test -- --runTestsByPath __tests__/web-chat-ui.test.ts __tests__/web-session-ui.test.ts`

Expected: FAIL because session-centric protocol and UI state do not exist yet.

- [ ] **Step 3: Implement minimal session-centric app state**

Add or rename types and service methods:

```ts
export interface RegistrySessionSummary {
  sessionId: string;
  title: string;
  preview: string;
  updatedAt: string;
  messageCount: number;
  unreadCount: number;
  agent?: string;
  status?: string;
}

export interface RegistrySessionMessage {
  messageId: string;
  sessionId: string;
  role: 'user' | 'assistant' | 'system';
  kind: string;
  text: string;
  status: string;
  createdAt: string;
  updatedAt: string;
}
```

Replace repository and workspace service methods with:

```ts
listSessions()
readSession(sessionId: string)
createSession(title?: string)
sendSessionMessage(payload: {sessionId: string; text?: string; blocks?: unknown[]})
markSessionRead(payload: {sessionId: string; messageId?: string})
```

Update `main.tsx` state names and event handling so sidebar is driven by `session.updated`, the message pane by `session.message`, and new session creation calls `session.new` before the first send.

- [ ] **Step 4: Run app tests and type-check to verify they pass**

Run: `cd app && npm test -- --runTestsByPath __tests__/web-chat-ui.test.ts __tests__/web-session-ui.test.ts`

Run: `cd app && npm run tsc:web`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add app/web/src/types/registry.ts app/web/src/services/registryRepository.ts app/web/src/services/registryWorkspaceService.ts app/web/src/main.tsx app/__tests__/web-chat-ui.test.ts app/__tests__/web-session-ui.test.ts
git commit -m "feat(app): migrate web chat UI to project sessions"
```

---

## Task 5: Full Verification and Integration Review

**Files:**
- Modify: any files needed for final fixes from verification

- [ ] **Step 1: Run server verification**

Run: `cd server && go test ./...`

Expected: PASS.

- [ ] **Step 2: Run app verification**

Run: `cd app && npm test -- --runInBand`

Run: `cd app && npm run tsc:web`

Expected: PASS.

- [ ] **Step 3: Run code review**

Review against `docs/superpowers/specs/2026-04-12-app-session-design.md` and the changed files. Confirm:

1. app and Feishu can coexist
2. app uses `sessionId` rather than `chatId`
3. durable aggregated history exists on server
4. router remains a delivery layer, not a history owner

- [ ] **Step 4: Commit final fixes**

```bash
git add -A
git commit -m "fix: polish app project session integration"
```

- [ ] **Step 5: Push**

```bash
git push origin main
```