# Client Session Persistence Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove the `state` layer from `server/internal/hub/client`, replace it with one SQLite-backed store, and persist only project config, route bindings, and session snapshots under `~/.wheelmaker/db/client.sqlite3`.

**Architecture:** Introduce a single `store.go` that owns schema, records, and serialization; refactor `Client` and `Session` into pure runtime models; load project config plus route bindings at startup and restore sessions lazily on route hit. Empty route keys become invalid input, and no fallback `"default"` route remains.

**Tech Stack:** Go, SQLite (`modernc.org/sqlite`), JSON columns for `agents_json`, ACP protocol types, `internal/hub/client`, `internal/hub`, `cmd/wheelmaker`

---

## File Map

| File | Action | Responsibility after change |
|------|--------|-----------------------------|
| `server/internal/hub/client/store.go` | **Create** | Store interface, SQLite schema, records, serialization, SQLite implementation |
| `server/internal/hub/client/store_test.go` | **Create** | Store-focused schema/load/save tests |
| `server/internal/hub/client/client.go` | Modify | Runtime-only `Client`, startup light restore, route binding restore, lazy session restore |
| `server/internal/hub/client/session.go` | Modify | Runtime-only `Session`, `toRecord`, `sessionFromRecord`, no `ProjectState` sync |
| `server/internal/hub/client/commands.go` | Modify | `/new`, `/load`, config persistence through the new store |
| `server/internal/hub/client/client_test.go` | Modify | Behavior tests for startup restore, route rebinding, empty route rejection |
| `server/internal/hub/client/client_internal_test.go` | Modify | Test helpers updated for new store boundary |
| `server/internal/hub/hub.go` | Modify | Construct the unified store at `~/.wheelmaker/db/client.sqlite3` |
| `server/cmd/wheelmaker/main.go` | Modify | Pass DB path instead of `state.json` path |
| `server/internal/hub/client/state.go` | Delete | Removed state DTO layer |
| `server/internal/hub/client/state_store.go` | Delete | Removed JSON/project state store layer |
| `server/internal/hub/client/sqlite_store.go` | Delete | Folded into `store.go` |
| `server/internal/hub/client/session_meta.go` | Delete | Folded into per-agent persisted state |

---

### Task 1: Write Failing Store and Startup Tests

**Files:**
- Create: `server/internal/hub/client/store_test.go`
- Modify: `server/internal/hub/client/client_test.go`

- [ ] **Step 1: Add a failing store round-trip test for project config + route bindings + session records**

Create `server/internal/hub/client/store_test.go` with these two tests first:

```go
package client_test

import (
    "context"
    "path/filepath"
    "testing"

    "github.com/swm8023/wheelmaker/internal/hub/client"
)

func TestSQLiteStore_ProjectRouteAndSessionRoundTrip(t *testing.T) {
    dir := t.TempDir()
    dbPath := filepath.Join(dir, "db", "client.sqlite3")

    store, err := client.NewStore(dbPath)
    if err != nil {
        t.Fatalf("NewStore() error = %v", err)
    }
    defer store.Close()

    ctx := context.Background()
    if err := store.SaveProject(ctx, "proj-a", client.ProjectConfig{YOLO: true}); err != nil {
        t.Fatalf("SaveProject() error = %v", err)
    }
    if err := store.SaveRouteBinding(ctx, "proj-a", "im:feishu:chat-1", "sess-1"); err != nil {
        t.Fatalf("SaveRouteBinding() error = %v", err)
    }
    if err := store.SaveSession(ctx, &client.SessionRecord{
        ID:          "sess-1",
        ProjectName: "proj-a",
        Status:      client.SessionSuspended,
        AgentsJSON:  `{"claude":{"acpSessionId":"acp-1"}}`,
    }); err != nil {
        t.Fatalf("SaveSession() error = %v", err)
    }

    cfg, err := store.LoadProject(ctx, "proj-a")
    if err != nil || !cfg.YOLO {
        t.Fatalf("LoadProject() = %+v, %v; want YOLO=true", cfg, err)
    }

    bindings, err := store.LoadRouteBindings(ctx, "proj-a")
    if err != nil {
        t.Fatalf("LoadRouteBindings() error = %v", err)
    }
    if got := bindings["im:feishu:chat-1"]; got != "sess-1" {
        t.Fatalf("binding = %q, want sess-1", got)
    }

    rec, err := store.LoadSession(ctx, "proj-a", "sess-1")
    if err != nil || rec == nil || rec.ID != "sess-1" {
        t.Fatalf("LoadSession() = %+v, %v; want sess-1", rec, err)
    }
}

func TestSQLiteStore_RejectsEmptyRouteKey(t *testing.T) {
    dir := t.TempDir()
    dbPath := filepath.Join(dir, "db", "client.sqlite3")

    store, err := client.NewStore(dbPath)
    if err != nil {
        t.Fatalf("NewStore() error = %v", err)
    }
    defer store.Close()

    err = store.SaveRouteBinding(context.Background(), "proj-a", "", "sess-1")
    if err == nil {
        t.Fatal("SaveRouteBinding() should reject empty route keys")
    }
}
```

- [ ] **Step 2: Add a failing client startup test that loads route bindings without eagerly restoring sessions**

In `server/internal/hub/client/client_test.go`, add:

```go
func TestClientStart_LoadsRouteBindingsWithoutRestoringSessions(t *testing.T) {
    dir := t.TempDir()
    dbPath := filepath.Join(dir, "db", "client.sqlite3")

    store, err := client.NewStore(dbPath)
    if err != nil {
        t.Fatalf("NewStore() error = %v", err)
    }
    defer store.Close()

    ctx := context.Background()
    if err := store.SaveProject(ctx, "proj-a", client.ProjectConfig{YOLO: false}); err != nil {
        t.Fatalf("SaveProject() error = %v", err)
    }
    if err := store.SaveRouteBinding(ctx, "proj-a", "im:feishu:chat-1", "sess-1"); err != nil {
        t.Fatalf("SaveRouteBinding() error = %v", err)
    }
    if err := store.SaveSession(ctx, &client.SessionRecord{
        ID:          "sess-1",
        ProjectName: "proj-a",
        Status:      client.SessionPersisted,
        AgentsJSON:  `{"claude":{"acpSessionId":"acp-1"}}`,
    }); err != nil {
        t.Fatalf("SaveSession() error = %v", err)
    }

    c := client.New(store, "claude", "proj-a", dir)
    if err := c.Start(ctx); err != nil {
        t.Fatalf("Start() error = %v", err)
    }

    if got := c.RouteSessionIDForTest("im:feishu:chat-1"); got != "sess-1" {
        t.Fatalf("route binding = %q, want sess-1", got)
    }
    if c.HasSessionInMemoryForTest("sess-1") {
        t.Fatal("persisted session should not be eagerly restored during Start()")
    }
}
```

- [ ] **Step 3: Run the new tests and verify they fail for the expected reason**

Run:

```bash
cd E:/_Code/WheelMaker/server
go test ./internal/hub/client -run "TestSQLiteStore_|TestClientStart_LoadsRouteBindingsWithoutRestoringSessions" -count=1
```

Expected:
- `FAIL`
- missing `NewStore`, `ProjectConfig`, `SessionRecord`, or test helper symbols

- [ ] **Step 4: Commit the failing tests**

```bash
git add server/internal/hub/client/store_test.go server/internal/hub/client/client_test.go
git commit -m "test(client): add failing unified persistence store tests"
```

---

### Task 2: Implement the Unified SQLite Store

**Files:**
- Create: `server/internal/hub/client/store.go`
- Modify: `server/internal/hub/client/store_test.go`

- [ ] **Step 1: Create the new store types and public interface**

Create `server/internal/hub/client/store.go` and add the top-level types:

```go
package client

import (
    "context"
    "database/sql"
    "encoding/json"
    "fmt"
    "os"
    "path/filepath"
    "strings"
    "time"

    _ "modernc.org/sqlite"
)

type ProjectConfig struct {
    YOLO bool
}

type SessionRecord struct {
    ID           string
    ProjectName  string
    Status       SessionStatus
    LastReply    string
    ACPSessionID string
    AgentsJSON   string
    CreatedAt    time.Time
    LastActiveAt time.Time
}

type SessionListEntry struct {
    ID           string
    CreatedAt    time.Time
    LastActiveAt time.Time
    Status       SessionStatus
}

type Store interface {
    LoadProject(ctx context.Context, projectName string) (*ProjectConfig, error)
    SaveProject(ctx context.Context, projectName string, cfg ProjectConfig) error

    LoadRouteBindings(ctx context.Context, projectName string) (map[string]string, error)
    SaveRouteBinding(ctx context.Context, projectName, routeKey, sessionID string) error
    DeleteRouteBinding(ctx context.Context, projectName, routeKey string) error

    LoadSession(ctx context.Context, projectName, sessionID string) (*SessionRecord, error)
    SaveSession(ctx context.Context, rec *SessionRecord) error
    ListSessions(ctx context.Context, projectName string) ([]SessionListEntry, error)
    DeleteSession(ctx context.Context, projectName, sessionID string) error

    Close() error
}
```

- [ ] **Step 2: Add the SQLite schema and constructor**

In the same file, add the constructor and schema:

```go
const sqliteSchema = `
CREATE TABLE IF NOT EXISTS projects (
    project_name TEXT PRIMARY KEY,
    yolo INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS route_bindings (
    project_name TEXT NOT NULL,
    route_key TEXT NOT NULL,
    session_id TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    PRIMARY KEY (project_name, route_key)
);
CREATE TABLE IF NOT EXISTS sessions (
    id TEXT PRIMARY KEY,
    project_name TEXT NOT NULL,
    status INTEGER NOT NULL,
    last_reply TEXT NOT NULL DEFAULT '',
    acp_session_id TEXT NOT NULL DEFAULT '',
    agents_json TEXT NOT NULL DEFAULT '{}',
    created_at TEXT NOT NULL,
    last_active_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_sessions_project_last_active ON sessions(project_name, last_active_at DESC);
CREATE INDEX IF NOT EXISTS idx_route_bindings_project ON route_bindings(project_name);
`

type sqliteStore struct {
    db *sql.DB
}

func NewStore(dbPath string) (Store, error) {
    if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
        return nil, fmt.Errorf("mkdir db dir: %w", err)
    }
    db, err := sql.Open("sqlite", dbPath+"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)")
    if err != nil {
        return nil, fmt.Errorf("open sqlite %q: %w", dbPath, err)
    }
    if _, err := db.Exec(sqliteSchema); err != nil {
        _ = db.Close()
        return nil, fmt.Errorf("init schema: %w", err)
    }
    return &sqliteStore{db: db}, nil
}
```

- [ ] **Step 3: Implement project, route, and session CRUD with route-key validation**

Use this validation in `SaveRouteBinding`:

```go
func validateRouteKey(routeKey string) error {
    if strings.TrimSpace(routeKey) == "" {
        return fmt.Errorf("route key is required")
    }
    return nil
}
```

Persist timestamps with `time.RFC3339Nano`. For missing projects, `LoadProject` should return `&ProjectConfig{YOLO:false}, nil`.

For `SaveSession`, the upsert should look like:

```go
_, err := s.db.ExecContext(ctx, `
    INSERT INTO sessions (
        id, project_name, status, last_reply, acp_session_id, agents_json, created_at, last_active_at
    ) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
    ON CONFLICT(id) DO UPDATE SET
        project_name=excluded.project_name,
        status=excluded.status,
        last_reply=excluded.last_reply,
        acp_session_id=excluded.acp_session_id,
        agents_json=excluded.agents_json,
        last_active_at=excluded.last_active_at
`, rec.ID, rec.ProjectName, int(rec.Status), rec.LastReply, rec.ACPSessionID, rec.AgentsJSON,
    rec.CreatedAt.Format(time.RFC3339Nano), rec.LastActiveAt.Format(time.RFC3339Nano),
)
```

- [ ] **Step 4: Run the targeted tests and verify they pass**

Run:

```bash
go test ./internal/hub/client -run "TestSQLiteStore_|TestClientStart_LoadsRouteBindingsWithoutRestoringSessions" -count=1
```

Expected:
- `PASS` for the store tests
- `FAIL` still acceptable for the startup test if `Client.Start()` has not been refactored yet, but only because runtime wiring is incomplete

- [ ] **Step 5: Commit**

```bash
git add server/internal/hub/client/store.go server/internal/hub/client/store_test.go
git commit -m "feat(client): add unified SQLite persistence store"
```

---

### Task 3: Refactor Session Persistence Shape

**Files:**
- Modify: `server/internal/hub/client/session.go`

- [ ] **Step 1: Fold persisted meta into per-agent session state**

Replace the old split between `SessionAgentState`, `clientSessionMeta`, and `clientInitMeta` with one persisted agent state:

```go
type SessionAgentState struct {
    ACPSessionID      string                 `json:"acpSessionId,omitempty"`
    ConfigOptions     []acp.ConfigOption     `json:"configOptions,omitempty"`
    Commands          []acp.AvailableCommand `json:"commands,omitempty"`
    Title             string                 `json:"title,omitempty"`
    UpdatedAt         string                 `json:"updatedAt,omitempty"`
    ProtocolVersion   string                 `json:"protocolVersion,omitempty"`
    AgentCapabilities acp.AgentCapabilities  `json:"agentCapabilities,omitempty"`
    AgentInfo         *acp.AgentInfo         `json:"agentInfo,omitempty"`
    AuthMethods       []acp.AuthMethod       `json:"authMethods,omitempty"`
}
```

Remove `initMeta`, `sessionMeta`, and any `ProjectState` fields from `Session`.

- [ ] **Step 2: Add direct record conversion helpers**

Add these helpers to `session.go`:

```go
func (s *Session) toRecord(projectName string) (*SessionRecord, error) {
    s.mu.Lock()
    defer s.mu.Unlock()

    agentsJSON, err := json.Marshal(s.agents)
    if err != nil {
        return nil, fmt.Errorf("marshal agents: %w", err)
    }

    return &SessionRecord{
        ID:           s.ID,
        ProjectName:  projectName,
        Status:       s.Status,
        LastReply:    s.lastReply,
        ACPSessionID: s.acpSessionID,
        AgentsJSON:   string(agentsJSON),
        CreatedAt:    s.createdAt,
        LastActiveAt: s.lastActiveAt,
    }, nil
}

func sessionFromRecord(rec *SessionRecord, cwd string) (*Session, error) {
    s := newSession(rec.ID, cwd)
    s.Status = rec.Status
    s.lastReply = rec.LastReply
    s.acpSessionID = rec.ACPSessionID
    s.createdAt = rec.CreatedAt
    s.lastActiveAt = rec.LastActiveAt
    if strings.TrimSpace(rec.AgentsJSON) != "" {
        if err := json.Unmarshal([]byte(rec.AgentsJSON), &s.agents); err != nil {
            return nil, fmt.Errorf("unmarshal agents_json: %w", err)
        }
    }
    return s, nil
}
```

- [ ] **Step 3: Replace old snapshot/meta sync calls with direct store saves**

Change suspend/close paths from:

```go
snap := s.Snapshot(projectName)
return s.persistence.SaveSession(ctx, snap)
```

to:

```go
rec, err := s.toRecord(projectName)
if err != nil {
    return err
}
return s.store.SaveSession(ctx, rec)
```

Also remove:
- `syncRuntimeToProjectState`
- `syncAndPersistProjectState`
- `persistProjectState`
- `RestoreFromSnapshot`

- [ ] **Step 4: Run the package tests and verify the expected next failures**

Run:

```bash
go test ./internal/hub/client -count=1
```

Expected:
- failures now concentrated in `Client`/constructor code still referencing `ProjectState`, `ClientStateStore`, or `RestoreFromSnapshot`

- [ ] **Step 5: Commit**

```bash
git add server/internal/hub/client/session.go
git commit -m "refactor(client): make session persistence record-based"
```

---

### Task 4: Refactor Client Startup, Route Binding, and Lazy Restore

**Files:**
- Modify: `server/internal/hub/client/client.go`
- Modify: `server/internal/hub/client/commands.go`
- Modify: `server/internal/hub/client/client_test.go`
- Modify: `server/internal/hub/client/client_internal_test.go`

- [ ] **Step 1: Change `Client` to hold the unified store and drop `state`**

Update the struct and constructor:

```go
type Client struct {
    projectName     string
    cwd             string
    yolo            bool
    configuredAgent string

    registry *agent.ACPFactory
    store    Store
    imRouter IMRouter

    mu sync.Mutex

    sessions map[string]*Session
    routeMap map[string]string

    suspendTimeout time.Duration
    stopPersistCh  chan struct{}
}

func New(store Store, preferredAgent any, projectName string, cwd string) *Client {
    c := &Client{
        projectName:     projectName,
        cwd:             cwd,
        configuredAgent: normalizePreferredAgent(preferredAgent),
        registry:        agent.DefaultACPFactory(),
        store:           store,
        sessions:        map[string]*Session{},
        routeMap:        map[string]string{},
        suspendTimeout:  5 * time.Minute,
        stopPersistCh:   make(chan struct{}),
    }
    return c
}
```

- [ ] **Step 2: Implement light startup restore**

Replace the old `LoadProjectState()` path in `Start()` with:

```go
func (c *Client) Start(ctx context.Context) error {
    cfg, err := c.store.LoadProject(ctx, c.projectName)
    if err != nil {
        return fmt.Errorf("client: load project config: %w", err)
    }

    bindings, err := c.store.LoadRouteBindings(ctx, c.projectName)
    if err != nil {
        return fmt.Errorf("client: load route bindings: %w", err)
    }

    c.mu.Lock()
    c.yolo = cfg.YOLO
    c.routeMap = bindings
    c.mu.Unlock()

    go c.persistLoop()
    return nil
}
```

- [ ] **Step 3: Make route resolution reject empty keys and restore sessions lazily**

Use this helper:

```go
func normalizeRouteKey(routeKey string) (string, error) {
    routeKey = strings.TrimSpace(routeKey)
    if routeKey == "" {
        return "", fmt.Errorf("route key is required")
    }
    return routeKey, nil
}
```

Then in `resolveSession`:
- validate routeKey
- if bound session is in memory, reuse
- if bound session is not in memory, call `c.store.LoadSession(...)`
- if binding exists but row missing, return an explicit error
- if unbound, create session and immediately persist session + route binding

- [ ] **Step 4: Update `/new` and `/load` to persist route rebinding immediately**

In `ClientNewSession` and `ClientLoadSession`, after binding a route, add:

```go
if err := c.store.SaveRouteBinding(context.Background(), c.projectName, routeKey, sess.ID); err != nil {
    return nil, fmt.Errorf("save route binding: %w", err)
}
```

For `/new`, persist the new session row immediately after creation.

- [ ] **Step 5: Add test helpers and make the startup test pass**

In `client_internal_test.go`, add helpers like:

```go
func (c *Client) RouteSessionIDForTest(routeKey string) string {
    c.mu.Lock()
    defer c.mu.Unlock()
    return c.routeMap[routeKey]
}

func (c *Client) HasSessionInMemoryForTest(sessionID string) bool {
    c.mu.Lock()
    defer c.mu.Unlock()
    _, ok := c.sessions[sessionID]
    return ok
}
```

In `client_test.go`, add:

```go
func TestClientResolveSession_RejectsEmptyRouteKey(t *testing.T) {
    dir := t.TempDir()
    store, err := client.NewStore(filepath.Join(dir, "db", "client.sqlite3"))
    if err != nil {
        t.Fatalf("NewStore() error = %v", err)
    }
    defer store.Close()

    c := client.New(store, "claude", "proj-a", dir)
    if _, err := c.ResolveSessionForTest(""); err == nil {
        t.Fatal("ResolveSessionForTest(\"\") should fail")
    }
}
```

- [ ] **Step 6: Run tests and verify they pass**

Run:

```bash
go test ./internal/hub/client -count=1
```

Expected:
- `PASS`

- [ ] **Step 7: Commit**

```bash
git add server/internal/hub/client/client.go server/internal/hub/client/commands.go server/internal/hub/client/client_test.go server/internal/hub/client/client_internal_test.go
git commit -m "refactor(client): restore route bindings and sessions through unified store"
```

---

### Task 5: Wire the Store into Hub Startup and Remove Legacy Files

**Files:**
- Modify: `server/internal/hub/hub.go`
- Modify: `server/cmd/wheelmaker/main.go`
- Delete: `server/internal/hub/client/state.go`
- Delete: `server/internal/hub/client/state_store.go`
- Delete: `server/internal/hub/client/sqlite_store.go`
- Delete: `server/internal/hub/client/session_meta.go`

- [ ] **Step 1: Change startup path from `state.json` to `db/client.sqlite3`**

In `server/cmd/wheelmaker/main.go`, replace:

```go
statePath := filepath.Join(home, ".wheelmaker", "state.json")
...
h := hub.New(cfg, statePath)
```

with:

```go
dbPath := filepath.Join(home, ".wheelmaker", "db", "client.sqlite3")
...
h := hub.New(cfg, dbPath)
```

- [ ] **Step 2: Update `hub.New` and `buildIMClient` to create the new store**

In `server/internal/hub/hub.go`, rename `statePath` to `dbPath` and change:

```go
store := client.NewProjectJSONStore(h.statePath, pc.Name)
c := client.New(store, pc.Client.Agent, pc.Name, cwd)
```

to:

```go
store, err := client.NewStore(h.dbPath)
if err != nil {
    return nil, fmt.Errorf("new store: %w", err)
}
c := client.New(store, pc.Client.Agent, pc.Name, cwd)
```

- [ ] **Step 3: Delete the legacy state-layer files**

Delete:

```text
server/internal/hub/client/state.go
server/internal/hub/client/state_store.go
server/internal/hub/client/sqlite_store.go
server/internal/hub/client/session_meta.go
```

The new `store.go` must be the only persistence file left in `server/internal/hub/client/`.

- [ ] **Step 4: Run source search to verify the old layer is gone**

Run:

```bash
Get-ChildItem server -Recurse -File -Include *.go | Select-String -Pattern 'ProjectState|ClientStateStore|NewProjectJSONStore|state.json|RestoreFromSnapshot'
```

Expected:
- zero hits in source files, or only hits inside tests/comments being actively updated in this task

- [ ] **Step 5: Commit**

```bash
git add server/cmd/wheelmaker/main.go server/internal/hub/hub.go server/internal/hub/client
git commit -m "refactor(client): remove legacy state layer and wire unified store"
```

---

### Task 6: Full Verification

**Files:**
- Modify as needed from previous tasks only

- [ ] **Step 1: Run targeted client tests**

Run:

```bash
cd E:/_Code/WheelMaker/server
go test ./internal/hub/client -count=1
```

Expected:
- `PASS`

- [ ] **Step 2: Run the full server test suite**

Run:

```bash
go test ./... 
```

Expected:
- all packages `ok`

- [ ] **Step 3: Build the daemon binary**

Run:

```bash
go build ./cmd/wheelmaker/
```

Expected:
- exit code `0`

- [ ] **Step 4: Check git diff and working tree**

Run:

```bash
git status --short
git diff --stat
```

Expected:
- only intended persistence-refactor files changed

- [ ] **Step 5: Final commit sequence**

```bash
git add -A
git commit -m "Refactor client session persistence into unified store"
git push origin <branch>
```

- [ ] **Step 6: If files under `server/` changed, trigger update**

```bash
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/signal_update_now.ps1 -DelaySeconds 30
```

Expected:
- updater trigger accepted
