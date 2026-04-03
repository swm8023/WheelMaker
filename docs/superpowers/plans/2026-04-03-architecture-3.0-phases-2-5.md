# Architecture 3.0 Phases 2–5 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build on Phase 1's Session extraction to add AgentInstance/AgentConn abstraction (Phase 2), multi-session routing (Phase 3), SQLite session persistence (Phase 4), and final cleanup (Phase 5).

**Architecture:** Session uses AgentInstance (not raw agentConn). AgentConn handles shared/owned connection modes. Client routes IM messages via routeKey → Session mapping. Sessions persist to SQLite for suspension/recovery.

**Tech Stack:** Go 1.22+, existing `acp`, `agent`, `im` packages. New: `modernc.org/sqlite` for CGo-free SQLite.

---

## File Structure

| File | Phase | Action | Responsibility |
|------|-------|--------|---------------|
| `internal/hub/client/agent_conn.go` | 2 | Create | `AgentConn`, `ConnMode`, shared dispatch, reconnect |
| `internal/hub/client/agent_instance.go` | 2 | Create | `AgentInstance`, ACP method wrappers, `SessionCallbacks` interface |
| `internal/hub/client/agent_factory.go` | 2 | Create | `AgentFactory` interface, `legacyAgentFactory` adapter |
| `internal/hub/client/session_type.go` | 2 | Modify | Replace `conn *agentConn` with `instance *AgentInstance` |
| `internal/hub/client/session.go` | 2 | Modify | Use `s.instance` instead of `s.conn.forwarder` |
| `internal/hub/client/lifecycle.go` | 2 | Modify | `switchAgent` uses AgentInstance |
| `internal/hub/client/callbacks.go` | 2 | Modify | Session callback routing through AgentInstance |
| `internal/hub/client/commands.go` | 2 | Modify | Replace `s.conn` references with `s.instance` |
| `internal/hub/client/client.go` | 2,3 | Modify | AgentFactory usage; routeMap + multi-session routing |
| `internal/hub/client/client_internal_test.go` | 2,3 | Modify | Test helpers for AgentInstance |
| `internal/hub/im/im.go` | 3 | Modify | Add `RouteKey()` to Message |
| `internal/hub/im/feishu/*.go` | 3 | Modify | IM adapters set RouteKey |
| `internal/hub/im/console/*.go` | 3 | Modify | IM adapters set RouteKey |
| `internal/hub/client/session_store.go` | 4 | Create | `SessionStore` interface, `SessionSnapshot` |
| `internal/hub/client/sqlite_store.go` | 4 | Create | SQLite implementation |
| `internal/hub/client/sqlite_store_test.go` | 4 | Create | SQLite store tests |
| `internal/hub/client/state.go` | 4 | Modify | Add session summary entry type |
| `docs/architecture-3.0.md` | 5 | Modify | Final updates |
| `server/CLAUDE.md` | 5 | Modify | Updated architecture |

---

## Phase 2: Extract AgentInstance + AgentConn

### Task 2.1: Define AgentConn type

**Files:** Create `internal/hub/client/agent_conn.go`

- [ ] **Step 1: Create `agent_conn.go` with AgentConn struct**

AgentConn wraps agent.Agent + acp.Forwarder. Two modes: ConnOwned (1:1) and ConnShared (N:1).

```go
package client

type ConnMode int
const (
    ConnOwned  ConnMode = iota // one AgentInstance per connection
    ConnShared                 // multiple AgentInstances share one connection
)

type AgentConn struct {
    agent     agent.Agent
    forwarder *acp.Forwarder
    mode      ConnMode
    debugLog  io.Writer

    // shared mode: routes callbacks by acpSessionId
    mu        sync.RWMutex
    instances map[string]*AgentInstance // acpSessionId -> AgentInstance
}
```

AgentConn implements `acp.ClientCallbacks` for shared mode dispatch.

- [ ] **Step 2: Verify it compiles**

Run: `cd server && go build ./internal/hub/client/...`

---

### Task 2.2: Define AgentInstance type

**Files:** Create `internal/hub/client/agent_instance.go`

- [ ] **Step 1: Create `agent_instance.go` with AgentInstance struct and SessionCallbacks interface**

```go
type SessionCallbacks interface {
    acp.ClientCallbacks
}

type AgentInstance struct {
    name      string
    agentConn *AgentConn
    callbacks SessionCallbacks // owner Session
    initMeta  clientInitMeta
}
```

Public ACP methods delegate to `agentConn.forwarder`:
- `Initialize`, `SessionNew`, `SessionLoad`, `SessionList`, `SessionPrompt`, `SessionCancel`, `SessionSetConfigOption`, `Close`

For owned mode: `SetCallbacks(callbacks)` directly on forwarder.
For shared mode: register in `agentConn.instances[acpSessionId]`.

- [ ] **Step 2: Verify it compiles**

---

### Task 2.3: Define AgentFactory interface

**Files:** Create `internal/hub/client/agent_factory.go`

- [ ] **Step 1: Create `agent_factory.go` with interface + legacy adapter**

```go
type AgentFactoryV2 interface {
    Name() string
    SupportsSharedConn() bool
    CreateInstance(ctx context.Context, callbacks SessionCallbacks, debugLog io.Writer) (*AgentInstance, error)
}
```

`legacyAgentFactory` wraps existing `AgentFactory` (function) into `AgentFactoryV2`:

```go
type legacyAgentFactory struct {
    name string
    fn   AgentFactory
}

func (f *legacyAgentFactory) Name() string { return f.name }
func (f *legacyAgentFactory) SupportsSharedConn() bool { return false }
func (f *legacyAgentFactory) CreateInstance(ctx context.Context, cb SessionCallbacks, dw io.Writer) (*AgentInstance, error) {
    a := f.fn("", nil)
    conn, err := a.Connect(ctx)
    // ... wrap in AgentConn + AgentInstance
}
```

- [ ] **Step 2: Verify it compiles**

---

### Task 2.4: Integrate AgentInstance into Session

**Files:** Modify `session_type.go`, `session.go`, `lifecycle.go`, `callbacks.go`, `commands.go`

- [ ] **Step 1: Replace `conn *agentConn` with `instance *AgentInstance` in Session struct**

In `session_type.go`, change:
```go
conn *agentConn  →  instance *AgentInstance
```

- [ ] **Step 2: Update `ensureForwarder` → `ensureInstance`**

Session.ensureForwarder becomes Session.ensureInstance:
- Uses `Session.registry` to find `AgentFactoryV2`
- Calls `factory.CreateInstance(ctx, s, debugLog)` (Session is the callbacks)
- Sets `s.instance = inst`

- [ ] **Step 3: Update session.go — replace `s.conn.forwarder.*` with `s.instance.*`**

All ACP calls go through `s.instance.Initialize()`, `s.instance.SessionNew()`, etc.

- [ ] **Step 4: Update lifecycle.go — switchAgent uses AgentInstance**

`switchAgent` creates new AgentInstance via factory, replaces `s.instance`.

- [ ] **Step 5: Update commands.go — replace `s.conn` with `s.instance`**

`listSessions`, `createNewSession`, `loadSessionByIndex`, `handleConfigCommand`, `resolveHelpModel` use `s.instance`.

- [ ] **Step 6: Update callbacks.go — SetCallbacks wiring through AgentInstance**

AgentInstance sets callbacks on its AgentConn's forwarder (owned mode).

- [ ] **Step 7: Remove old `agentConn` type and related code**

Old `agentConn` struct in `client.go` is removed. Tests use AgentInstance.

- [ ] **Step 8: Update test helpers (InjectForwarder → InjectInstance)**

`InjectForwarder` now creates AgentInstance wrapping the forwarder.

- [ ] **Step 9: Verify all tests pass, commit**

Run: `cd server && go test ./internal/hub/client/... -v -count=1`

```bash
git add -A && git commit -m "refactor(client): extract AgentInstance + AgentConn from agentConn"
```

---

### Task 2.5: Update agentRegistry to hold AgentFactoryV2

**Files:** Modify `client.go`

- [ ] **Step 1: Change registry to store AgentFactoryV2 instead of AgentFactory**

`RegisterAgent(name, factory)` wraps in `legacyAgentFactory` automatically.

The old `AgentFactory` type remains as a public alias for backward compat.

- [ ] **Step 2: Verify all tests pass, commit**

```bash
git add -A && git commit -m "refactor(client): agentRegistry stores AgentFactoryV2, legacy compat"
```

---

## Phase 3: Multi-Session Routing

### Task 3.1: Add RouteKey to im.Message

**Files:** Modify `internal/hub/im/im.go`

- [ ] **Step 1: Add RouteKey field to Message struct**

```go
type Message struct {
    ChatID    string
    MessageID string
    UserID    string
    Text      string
    RouteKey  string // IM-provided routing key; defaults to ChatID if empty
}
```

The field defaults to ChatID if empty (handled in Client.HandleMessage).

- [ ] **Step 2: Set RouteKey in IM adapters**

Feishu: `RouteKey = chatId`
Console: `RouteKey = "console"`
Mobile: `RouteKey = chatID` (or userID as appropriate)

- [ ] **Step 3: Verify all tests pass, commit**

```bash
git add -A && git commit -m "feat(im): add RouteKey field to Message"
```

---

### Task 3.2: Add routeMap and multi-session routing to Client

**Files:** Modify `client.go`

- [ ] **Step 1: Add routeMap to Client struct**

```go
type Client struct {
    // ...existing...
    routeMap map[string]string   // routeKey -> Session.ID
}
```

- [ ] **Step 2: Implement route decision in HandleMessage**

```go
func (c *Client) HandleMessage(msg im.Message) {
    routeKey := msg.RouteKey
    if routeKey == "" { routeKey = msg.ChatID }

    c.mu.Lock()
    sessID := c.routeMap[routeKey]
    sess := c.sessions[sessID]
    if sess == nil {
        // create new Session for this route
        sess = c.createSessionForRoute(routeKey)
    }
    c.mu.Unlock()

    // Set active chat for reply routing
    if c.imBridge != nil {
        c.imBridge.SetActiveChatID(msg.ChatID)
    }

    text := strings.TrimSpace(msg.Text)
    if text == "" { return }

    if cmd, args, ok := parseCommand(text); ok {
        c.handleCommand(sess, msg, cmd, args)
        return
    }
    sess.handlePrompt(msg, text)
}
```

- [ ] **Step 3: Implement createSessionForRoute**

Creates a new Session, wires back-references, adds to sessions map and routeMap.

- [ ] **Step 4: Migrate /new and /load to create/switch sessions on the current route**

`/new`: suspend current session on this route → create new → bind to route.
`/load`: suspend current → restore target → bind to route.

- [ ] **Step 5: Update /list to show all sessions**

Show in-memory sessions with their status and current agent.

- [ ] **Step 6: Verify all tests pass, commit**

```bash
git add -A && git commit -m "feat(client): multi-session routing via routeMap + routeKey"
```

---

### Task 3.3: Session isolation — independent promptMu

**Files:** Verify existing design

- [ ] **Step 1: Verify Session.promptMu is per-Session (already the case from Phase 1)**

Each Session has its own `promptMu`. Multiple Sessions can prompt concurrently since they use their own `promptMu`.

- [ ] **Step 2: Write integration test for concurrent sessions**

Two different routeKeys send messages simultaneously; both get responses without blocking each other.

- [ ] **Step 3: Verify tests pass, commit**

```bash
git add -A && git commit -m "test(client): verify concurrent session independence"
```

---

## Phase 4: Session Persistence (SQLite)

### Task 4.1: Define SessionStore interface

**Files:** Create `internal/hub/client/session_store.go`

- [ ] **Step 1: Create SessionStore interface and snapshot types**

```go
type SessionStore interface {
    Save(ctx context.Context, s *SessionSnapshot) error
    Load(ctx context.Context, sessionID string) (*SessionSnapshot, error)
    List(ctx context.Context) ([]SessionSummaryEntry, error)
    Delete(ctx context.Context, sessionID string) error
    Close() error
}

type SessionSnapshot struct {
    ID           string
    ProjectName  string
    Status       SessionStatus
    ActiveAgent  string
    LastReply    string
    CreatedAt    time.Time
    LastActiveAt time.Time
    Agents       map[string]*SessionAgentState
}

type SessionSummaryEntry struct {
    ID           string
    ActiveAgent  string
    Title        string
    CreatedAt    time.Time
    LastActiveAt time.Time
}
```

- [ ] **Step 2: Verify it compiles, commit**

```bash
git add -A && git commit -m "feat(client): define SessionStore interface and snapshot types"
```

---

### Task 4.2: Implement SQLiteSessionStore

**Files:** Create `internal/hub/client/sqlite_store.go`, `sqlite_store_test.go`

- [ ] **Step 1: Add `modernc.org/sqlite` to go.mod**

```bash
cd server && go get modernc.org/sqlite
```

- [ ] **Step 2: Implement SQLiteSessionStore with schema**

```sql
CREATE TABLE IF NOT EXISTS sessions (
    id           TEXT PRIMARY KEY,
    project_name TEXT NOT NULL,
    status       INTEGER NOT NULL DEFAULT 0,
    active_agent TEXT NOT NULL DEFAULT '',
    last_reply   TEXT NOT NULL DEFAULT '',
    created_at   TEXT NOT NULL,
    last_active  TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS session_agents (
    session_id      TEXT NOT NULL,
    agent_name      TEXT NOT NULL,
    acp_session_id  TEXT NOT NULL DEFAULT '',
    config_options  TEXT NOT NULL DEFAULT '[]',
    commands        TEXT NOT NULL DEFAULT '[]',
    title           TEXT NOT NULL DEFAULT '',
    updated_at      TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (session_id, agent_name),
    FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_sessions_project ON sessions(project_name);
```

Methods: `Save`, `Load`, `List`, `Delete`, `Close`.

- [ ] **Step 3: Write tests for SQLiteSessionStore**

Test CRUD operations: save, load, list, delete. Verify FK cascade on delete.

- [ ] **Step 4: Verify tests pass, commit**

```bash
git add -A && git commit -m "feat(client): SQLiteSessionStore implementation with tests"
```

---

### Task 4.3: Wire SessionStore into Client

**Files:** Modify `client.go`

- [ ] **Step 1: Add SessionStore field to Client**

```go
type Client struct {
    // ...existing...
    sessionStore SessionStore
}
```

Update `New()` to accept SessionStore (optional; nil = in-memory only).

- [ ] **Step 2: Implement Session.Suspend() → snapshot to SQLite**

```go
func (s *Session) Suspend(ctx context.Context, store SessionStore) error
```

Cancels prompt, kills terminals, snapshots state, writes to SQLite, sets Status = Suspended.

- [ ] **Step 3: Implement Client.restoreSession() → restore from SQLite**

Creates new Session from SessionSnapshot, sets Status = Active, agent re-connect is lazy.

- [ ] **Step 4: Update /new to suspend current session**

`/new` on current route: suspend current → create new → bind to route.

- [ ] **Step 5: Update /load to restore from SQLite**

`/load <index>`: suspend current → load from SQLite (or memory) → bind to route.

- [ ] **Step 6: Update /list to merge in-memory + SQLite**

Show both active in-memory sessions and persisted sessions.

- [ ] **Step 7: Flush all sessions on Client.Close()**

```go
func (c *Client) Close() error {
    // Snapshot all active sessions to SQLite
    for _, sess := range c.sessions {
        if sess.Status == SessionActive {
            _ = sess.Suspend(ctx, c.sessionStore)
        }
    }
    // ...existing close logic...
}
```

- [ ] **Step 8: Verify all tests pass, commit**

```bash
git add -A && git commit -m "feat(client): wire SessionStore, suspend/restore/flush lifecycle"
```

---

## Phase 5: Cleanup and Documentation

### Task 5.1: Remove deprecated remnants

**Files:** All `internal/hub/client/*.go`

- [ ] **Step 1: Remove old agentConn type if still present**
- [ ] **Step 2: Clean up any unused fields/methods**
- [ ] **Step 3: Verify tests pass, commit**

```bash
git add -A && git commit -m "refactor(client): remove deprecated session/prompt remnants"
```

---

### Task 5.2: Update documentation

**Files:** `docs/architecture-3.0.md`, `server/CLAUDE.md`

- [ ] **Step 1: Update architecture-3.0.md with implementation status**

Mark Phases 1-4 as implemented. Add implementation details and notes.

- [ ] **Step 2: Update server/CLAUDE.md architecture description**

Update the Architecture section to reflect Session, AgentInstance, AgentConn, SessionStore.

- [ ] **Step 3: Commit and push**

```bash
git add -A && git commit -m "docs: update architecture-3.0 and CLAUDE.md for Phase 1-5 completion"
git push origin main
```

---

## Verification Checklist

After all phases complete:

1. `cd server && go build ./internal/hub/client/...` — clean build
2. `cd server && go test ./internal/hub/client/... -v -count=1` — all tests pass
3. `cd server && go test ./... -count=1` — full suite (client package must pass; existing hub/registry failures are pre-existing)
4. `grep -rn 'c\.session\.' internal/hub/client/*.go` — no direct session field access on Client
5. `grep -rn 'agentConn' internal/hub/client/*.go` — removed (replaced by AgentConn/AgentInstance)
6. Git history: each phase has clean commits with passing tests
