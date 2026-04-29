# Session Creation Immutability Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ensure newly created sessions are only instantiated after ACP returns a non-empty session ID, and keep `acpSessionID` immutable after `Session` construction while preserving restore behavior.

**Architecture:** Split new-session creation into an ACP in-flight creation phase owned by `Client.CreateSession`, followed by validated `Session` construction. Keep restore paths constructor-based, and narrow `Session.ensureReady` so it only prepares an existing ACP session instead of allocating one.

**Tech Stack:** Go, ACP agent runtime, SQLite session store, Go unit tests.

---

### Task 1: Lock the creation invariant with tests

**Files:**
- Modify: `server/internal/hub/client/client_test.go`
- Test: `server/internal/hub/client/client_test.go`

- [ ] **Step 1: Write the failing tests**

```go
func TestCreateSessionWithAgent_FailsWhenACPReturnsEmptySessionID(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "client.sqlite3"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	inst := &testInjectedInstance{
		name: "claude",
		initResult: acp.InitializeResult{
			ProtocolVersion:   "0.1",
			AgentCapabilities: acp.AgentCapabilities{},
		},
		newResult: &acp.SessionNewResult{SessionID: ""},
	}

	c := New(store, "proj1", "/tmp")
	c.registry = agent.DefaultACPFactory().Clone()
	c.registry.Register("claude", func(context.Context, string) (agent.Instance, error) { return inst, nil })

	sess, err := c.CreateSession(context.Background(), "claude", "hello")
	if err == nil {
		t.Fatalf("CreateSession error = nil, want error")
	}
	if sess != nil {
		t.Fatalf("CreateSession session = %#v, want nil", sess)
	}

	loaded, err := store.LoadSession(context.Background(), "proj1", "")
	if err == nil && loaded != nil {
		t.Fatalf("LoadSession(empty) = %+v, want nil", loaded)
	}
}

func TestNewSession_RequiresNonEmptyACPID(t *testing.T) {
	sess, err := newSession("   ", "/tmp")
	if err == nil {
		t.Fatalf("newSession error = nil, want error")
	}
	if sess != nil {
		t.Fatalf("newSession session = %#v, want nil", sess)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/hub/client/... -run "TestCreateSessionWithAgent_FailsWhenACPReturnsEmptySessionID|TestNewSession_RequiresNonEmptyACPID"`
Expected: FAIL because `CreateSession` still allows delayed ID assignment and `newSession` does not reject empty IDs.

- [ ] **Step 3: Commit**

```bash
git add server/internal/hub/client/client_test.go
git commit -m "test: cover immutable session creation invariants"
```

### Task 2: Refactor construction to require ACP session ID upfront

**Files:**
- Modify: `server/internal/hub/client/session.go`
- Modify: `server/internal/hub/client/client.go`
- Test: `server/internal/hub/client/client_test.go`

- [ ] **Step 1: Add the in-flight creation result and validating constructor**

```go
type createdSessionState struct {
	sessionID string
	agentType string
	state     SessionAgentState
	instance  agent.Instance
	createdAt time.Time
}

func newSession(id string, cwd string) (*Session, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, fmt.Errorf("session id is required")
	}
	s := &Session{
		acpSessionID:   id,
		Status:         SessionActive,
		cwd:            cwd,
		createdAt:      time.Now(),
		prompt:         promptState{},
		timeoutLimiter: newTimeoutNotifyLimiter(timeoutNotifyCooldown),
	}
	s.initCond = sync.NewCond(&s.mu)
	return s, nil
}
```

- [ ] **Step 2: Move ACP `session/new` out of `ensureReady` into `CreateSession` path**

```go
func (c *Client) createSessionState(ctx context.Context, agentType, title string) (*createdSessionState, error) {
	// create instance, initialize, call SessionNew, validate non-empty sessionID,
	// build SessionAgentState from ACP results, and return the transport object.
}
```

- [ ] **Step 3: Construct `Session` only from complete state**

```go
func (c *Client) CreateSession(ctx context.Context, agentType, title string) (*Session, error) {
	created, err := c.createSessionState(ctx, agentType, title)
	if err != nil {
		return nil, err
	}
	sess, err := newSession(created.sessionID, c.cwd)
	if err != nil {
		_ = created.instance.Close()
		return nil, err
	}
	// wire state, instance, metadata, persist, then register in c.sessions
	return sess, nil
}
```

- [ ] **Step 4: Run focused tests to verify they pass**

Run: `go test ./internal/hub/client/... -run "TestCreateSessionWithAgent_UsesACPResultAsUnifiedSessionID|TestCreateSessionWithAgent_FailsWhenACPReturnsEmptySessionID|TestNewSession_RequiresNonEmptyACPID"`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add server/internal/hub/client/session.go server/internal/hub/client/client.go server/internal/hub/client/client_test.go
git commit -m "refactor: require acp session id before session construction"
```

### Task 3: Preserve restore and route behavior

**Files:**
- Modify: `server/internal/hub/client/session.go`
- Modify: `server/internal/hub/client/client_test.go`
- Test: `server/internal/hub/client/client_test.go`

- [ ] **Step 1: Update restore path to use the validating constructor**

```go
func sessionFromRecord(rec *SessionRecord, cwd string) (*Session, error) {
	s, err := newSession(rec.ID, cwd)
	if err != nil {
		return nil, err
	}
	// restore remaining persisted fields without mutating acpSessionID again
	return s, nil
}
```

- [ ] **Step 2: Adjust or add restore coverage if needed**

```go
func TestSessionFromRecord_UsesPersistedACPUnifiedSessionID(t *testing.T) {
	rec := &SessionRecord{ID: "sess-restored", AgentType: "claude"}
	sess, err := sessionFromRecord(rec, "/tmp")
	if err != nil {
		t.Fatalf("sessionFromRecord: %v", err)
	}
	if sess.acpSessionID != "sess-restored" {
		t.Fatalf("session ID = %q, want sess-restored", sess.acpSessionID)
	}
}
```

- [ ] **Step 3: Run focused restore and routing tests**

Run: `go test ./internal/hub/client/... -run "TestSessionFromRecord_UsesPersistedACPUnifiedSessionID|TestClientNewSession_ReappliesProjectAgentBaseline"`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add server/internal/hub/client/session.go server/internal/hub/client/client_test.go
git commit -m "test: preserve restore path under immutable session ids"
```

### Task 4: Full package verification and completion gate

**Files:**
- Modify: `docs/superpowers/plans/2026-04-29-session-creation-immutability.md`

- [ ] **Step 1: Run full client package tests**

Run: `go test ./internal/hub/client/...`
Expected: PASS

- [ ] **Step 2: Run server build**

Run: `go build ./...`
Expected: PASS

- [ ] **Step 3: Update plan checkboxes if needed and commit implementation**

```bash
git add -A
git commit -m "refactor: make session creation id-first and immutable"
```

- [ ] **Step 4: Push and trigger server update**

```bash
git push origin <current-branch>
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/signal_update_now.ps1 -DelaySeconds 30
```