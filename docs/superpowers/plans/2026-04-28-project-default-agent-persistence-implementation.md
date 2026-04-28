# Project Default Agent Persistence Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Persist per-project default agent and make explicit new-session actions update it, while implicit auto-create only reads it with safe fallback.

**Architecture:** Re-introduce a minimal `projects` table in client SQLite store for `default_agent_type`. Extend store interface with load/save APIs, then update client session creation paths so `/new <agent>` and `session.new(agentType)` persist default, while route auto-create reads default then falls back to runtime preferred provider without rewriting the stored value.

**Tech Stack:** Go, SQLite (`modernc.org/sqlite`), existing hub client store/session orchestration tests.

---

## File Structure

- Modify: `server/internal/hub/client/sqlite_store.go`
- Modify: `server/internal/hub/client/client.go`
- Modify: `server/internal/hub/client/client_test.go`
- Modify: `server/cmd/wheelmaker-monitor/monitor.go`
- Modify: `server/cmd/wheelmaker-monitor/monitor_test.go`

Responsibility boundaries:

1. `sqlite_store.go`: schema + persistence API for project default agent.
2. `client.go`: runtime decision and best-effort persistence on explicit new-session paths.
3. `client_test.go`: behavior and schema coverage.
4. `monitor.go`/`monitor_test.go`: DB table visibility alignment with new schema.

---

### Task 1: Re-introduce `projects` table and store APIs

**Files:**
- Modify: `server/internal/hub/client/sqlite_store.go`
- Test: `server/internal/hub/client/client_test.go`

- [ ] **Step 1: Add failing store tests for project default-agent persistence**

```go
func TestStoreProjectDefaultAgentRoundTrip(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "client.sqlite3"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	if err := store.SaveProjectDefaultAgent(context.Background(), "proj1", "claude"); err != nil {
		t.Fatalf("SaveProjectDefaultAgent: %v", err)
	}
	got, err := store.LoadProjectDefaultAgent(context.Background(), "proj1")
	if err != nil {
		t.Fatalf("LoadProjectDefaultAgent: %v", err)
	}
	if got != "claude" {
		t.Fatalf("default agent = %q, want claude", got)
	}
}

func TestStoreProjectDefaultAgentMissingReturnsEmpty(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "client.sqlite3"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	got, err := store.LoadProjectDefaultAgent(context.Background(), "proj-missing")
	if err != nil {
		t.Fatalf("LoadProjectDefaultAgent: %v", err)
	}
	if got != "" {
		t.Fatalf("default agent = %q, want empty", got)
	}
}
```

- [ ] **Step 2: Run focused tests to verify failure**

Run: `cd server; go test ./internal/hub/client -run "ProjectDefaultAgent|StoreProjectDefaultAgent" -count=1`
Expected: FAIL with compile errors (missing `SaveProjectDefaultAgent` / `LoadProjectDefaultAgent`).

- [ ] **Step 3: Implement schema + interface + methods in store**

```go
const sqliteSchema = `
CREATE TABLE IF NOT EXISTS projects (
	project_name TEXT PRIMARY KEY,
	default_agent_type TEXT NOT NULL DEFAULT '',
	updated_at TEXT NOT NULL
);
...`

var expectedStoreSchemaColumns = map[string][]string{
	"projects":          {"project_name", "default_agent_type", "updated_at"},
	"route_bindings":    {"project_name", "route_key", "session_id", "created_at", "updated_at"},
	"sessions":          {"id", "project_name", "status", "agent_type", "agent_json", "title", "created_at", "last_active_at"},
	"agent_preferences": {"project_name", "agent_type", "preference_json"},
	"session_prompts":   {"session_id", "prompt_index", "title", "stop_reason", "updated_at", "turns_json", "turn_index"},
}

type Store interface {
	...
	LoadProjectDefaultAgent(ctx context.Context, projectName string) (string, error)
	SaveProjectDefaultAgent(ctx context.Context, projectName, agentType string) error
	...
}

func (s *sqliteStore) LoadProjectDefaultAgent(ctx context.Context, projectName string) (string, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT default_agent_type
		FROM projects
		WHERE project_name = ?
	`, strings.TrimSpace(projectName))
	var agentType string
	if err := row.Scan(&agentType); err != nil {
		if err == sql.ErrNoRows {
			return "", nil
		}
		return "", fmt.Errorf("load project default agent: %w", err)
	}
	return strings.TrimSpace(agentType), nil
}

func (s *sqliteStore) SaveProjectDefaultAgent(ctx context.Context, projectName, agentType string) error {
	projectName = strings.TrimSpace(projectName)
	agentType = strings.TrimSpace(agentType)
	if projectName == "" || agentType == "" {
		return fmt.Errorf("project name and agent type are required")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO projects (project_name, default_agent_type, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(project_name) DO UPDATE SET
			default_agent_type=excluded.default_agent_type,
			updated_at=excluded.updated_at
	`, projectName, agentType, now)
	if err != nil {
		return fmt.Errorf("save project default agent: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Update legacy schema test expectation**

Replace assertion in `TestCheckStoreSchemaRejectsLegacyProjectsTable` to require projects-column mismatch instead of `unexpected table "projects"`:

```go
if !strings.Contains(err.Error(), `table "projects" columns mismatch`) &&
	!strings.Contains(err.Error(), `table "sessions" columns mismatch`) {
	t.Fatalf("CheckStoreSchema() err = %v, want projects/session schema mismatch", err)
}
```

- [ ] **Step 5: Re-run focused tests**

Run: `cd server; go test ./internal/hub/client -run "ProjectDefaultAgent|CheckStoreSchemaRejectsLegacyProjectsTable" -count=1`
Expected: PASS.

- [ ] **Step 6: Commit Task 1**

```bash
git add server/internal/hub/client/sqlite_store.go server/internal/hub/client/client_test.go
git commit -m "feat(client): persist project default agent in sqlite store"
```

---

### Task 2: Apply runtime default-agent read/write behavior

**Files:**
- Modify: `server/internal/hub/client/client.go`
- Test: `server/internal/hub/client/client_test.go`

- [ ] **Step 1: Add failing behavior tests**

```go
func TestClientNewSessionPersistsProjectDefaultAgent(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "client.sqlite3"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	c := New(store, "proj1", "/tmp")
	inst := &testInjectedInstance{name: "claude", initResult: acp.InitializeResult{ProtocolVersion: "0.1"}, newResult: &acp.SessionNewResult{SessionID: "sess-new"}}
	c.registry = agent.DefaultACPFactory().Clone()
	c.registry.Register(acp.ACPProviderClaude, func(context.Context, string) (agent.Instance, error) { return inst, nil })

	if _, err := c.ClientNewSession("route-1", "claude"); err != nil {
		t.Fatalf("ClientNewSession: %v", err)
	}
	got, err := store.LoadProjectDefaultAgent(context.Background(), "proj1")
	if err != nil {
		t.Fatalf("LoadProjectDefaultAgent: %v", err)
	}
	if got != "claude" {
		t.Fatalf("default agent = %q, want claude", got)
	}
}

func TestResolveOrCreateIMSessionUsesStoredDefaultAndFallbackDoesNotRewrite(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "client.sqlite3"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()
	if err := store.SaveProjectDefaultAgent(context.Background(), "proj1", "claude"); err != nil {
		t.Fatalf("SaveProjectDefaultAgent: %v", err)
	}

	c := New(store, "proj1", "/tmp")
	c.registry = agent.DefaultACPFactory().Clone()
	codexInst := &testInjectedInstance{name: "codex", initResult: acp.InitializeResult{ProtocolVersion: "0.1"}, newResult: &acp.SessionNewResult{SessionID: "sess-codex"}}
	c.registry.Register(acp.ACPProviderCodex, func(context.Context, string) (agent.Instance, error) { return codexInst, nil })

	sess := c.resolveOrCreateIMSession(context.Background(), im.ChatRef{ChannelID: "feishu", ChatID: "chat-a"}, "feishu:chat-a")
	if sess == nil {
		t.Fatal("resolveOrCreateIMSession = nil")
	}
	if sess.agentType != "codex" {
		t.Fatalf("agentType = %q, want codex fallback", sess.agentType)
	}
	still, err := store.LoadProjectDefaultAgent(context.Background(), "proj1")
	if err != nil {
		t.Fatalf("LoadProjectDefaultAgent: %v", err)
	}
	if still != "claude" {
		t.Fatalf("stored default rewritten to %q, want claude", still)
	}
}
```

- [ ] **Step 2: Run focused tests to verify failure**

Run: `cd server; go test ./internal/hub/client -run "PersistsProjectDefaultAgent|UsesStoredDefaultAndFallback" -count=1`
Expected: FAIL because behavior is not implemented.

- [ ] **Step 3: Implement client runtime behavior**

```go
func (c *Client) resolveOrCreateIMSession(ctx context.Context, source im.ChatRef, routeKey string) *Session {
	...
	agentType := c.preferredAgentForAutoCreate()
	if agentType == "" {
		_ = c.sendIMDirect(ctx, source, "No available agent.")
		return nil
	}
	sess, err := c.clientNewSessionWithOptions(routeKey, agentType, false)
	...
}

func (c *Client) preferredAgentForAutoCreate() string {
	if c.store != nil {
		agentType, err := c.store.LoadProjectDefaultAgent(context.Background(), c.projectName)
		if err != nil {
			hubLogger(c.projectName).Warn("load project default agent failed err=%v", err)
		} else if agentType != "" && c.registry != nil && c.registry.CreatorByName(agentType) != nil {
			return agentType
		}
	}
	return c.preferredAvailableAgent()
}

func (c *Client) ClientNewSession(routeKey, agentType string) (*Session, error) {
	return c.clientNewSessionWithOptions(routeKey, agentType, true)
}

func (c *Client) clientNewSessionWithOptions(routeKey, agentType string, persistDefault bool) (*Session, error) {
	...
	sess, err := c.CreateSession(context.Background(), agentType, "")
	if err != nil {
		return nil, err
	}
	if persistDefault {
		if err := c.store.SaveProjectDefaultAgent(context.Background(), c.projectName, agentType); err != nil {
			hubLogger(c.projectName).Warn("save project default agent failed agent=%s err=%v", agentType, err)
		}
	}
	...
}
```

In `HandleSessionRequest("session.new")` add best-effort persistence after successful `CreateSession`:

```go
if err := c.store.SaveProjectDefaultAgent(ctx, c.projectName, req.AgentType); err != nil {
	hubLogger(c.projectName).Warn("save project default agent failed agent=%s err=%v", req.AgentType, err)
}
```

- [ ] **Step 4: Re-run focused tests**

Run: `cd server; go test ./internal/hub/client -run "PersistsProjectDefaultAgent|UsesStoredDefaultAndFallback|SessionNewRequiresAgentType" -count=1`
Expected: PASS.

- [ ] **Step 5: Commit Task 2**

```bash
git add server/internal/hub/client/client.go server/internal/hub/client/client_test.go
git commit -m "feat(client): persist and apply project default agent"
```

---

### Task 3: Align monitor DB table list with schema

**Files:**
- Modify: `server/cmd/wheelmaker-monitor/monitor.go`
- Test: `server/cmd/wheelmaker-monitor/monitor_test.go`

- [ ] **Step 1: Add failing monitor test expectation update**

Change `TestGetDBTablesMatchesCurrentStoreSchema` to require `projects` table:

```go
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
```

- [ ] **Step 2: Run focused test to verify failure**

Run: `cd server; go test ./cmd/wheelmaker-monitor -run TestGetDBTablesMatchesCurrentStoreSchema -count=1`
Expected: FAIL because monitor table whitelist omits `projects`.

- [ ] **Step 3: Update monitor table whitelist**

```go
tableNames := []string{"projects", "route_bindings", "sessions", "agent_preferences", "session_prompts", "session_turns"}
```

- [ ] **Step 4: Re-run focused test**

Run: `cd server; go test ./cmd/wheelmaker-monitor -run TestGetDBTablesMatchesCurrentStoreSchema -count=1`
Expected: PASS.

- [ ] **Step 5: Commit Task 3**

```bash
git add server/cmd/wheelmaker-monitor/monitor.go server/cmd/wheelmaker-monitor/monitor_test.go
git commit -m "fix(monitor): include projects table in db schema view"
```

---

### Task 4: Full verification and completion gate

**Files:**
- Modify: none (verification + integration)

- [ ] **Step 1: Run client and monitor test suites touching changed behavior**

Run:

```bash
cd server
go test ./internal/hub/client -count=1
go test ./cmd/wheelmaker-monitor -count=1
```

Expected: PASS.

- [ ] **Step 2: Run full server tests for regression check**

Run: `cd server; go test ./... -count=1`
Expected: PASS.

- [ ] **Step 3: Final commit hygiene**

```bash
git status --short
```

Expected: only intended files are modified.

- [ ] **Step 4: Completion gate commands from repo root**

```bash
git add -A
git commit -m "feat(client): persist project default agent per project"
git push origin <branch>
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/signal_update_now.ps1 -DelaySeconds 30
```

Expected: all commands succeed.

---

## Spec Coverage Self-Review

1. Re-introduced `projects` table: covered in Task 1.
2. Explicit new-session updates default (`/new`, `session.new`): covered in Task 2.
3. Implicit auto-create reads default with fallback and does not rewrite: covered in Task 2.
4. Schema/test alignment (monitor + schema guard): covered in Task 1 and Task 3.
5. Validation and regression testing: covered in Task 4.

No placeholder markers (`TBD`/`TODO`) remain.