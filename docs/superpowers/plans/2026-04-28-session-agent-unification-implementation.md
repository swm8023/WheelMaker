# Session-Agent Unification Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rebuild server persistence, runtime session flow, IM help flow, monitor surfaces, and app/web session creation so one persisted session always matches one ACP conversation, agent defaults live in `agent_preferences`, and `/use` plus old schema compatibility are fully removed.

**Architecture:** Start with a hard-cut SQLite schema and record-model rewrite so the store only knows `route_bindings`, `sessions`, `agent_preferences`, `session_prompts`, and `session_turns`. Then collapse `Session` into a single-agent runtime object whose `ID` is the ACP session ID, route `/new` and `session.new` through explicit agent choice, and finally update session summaries, monitor rendering, registry project metadata, and app/web create-session UX to consume `agentType` plus project `agents`.

**Tech Stack:** Go, SQLite (`modernc.org/sqlite`), ACP protocol types, Feishu help cards, registry protocol DTOs, TypeScript/React app web UI, Jest

---

## File Map

| File | Action | Responsibility after change |
|------|--------|-----------------------------|
| `server/internal/hub/client/sqlite_store.go` | Modify | Hard-cut schema, store DTOs, schema validation, `agent_preferences` load/save |
| `server/internal/hub/client/session.go` | Modify | Single-agent runtime session, `ID == ACP session_id`, `agent_json` serialization |
| `server/internal/hub/client/client.go` | Modify | `CreateSession(agentType, title)`, IM `/new` flow, registry `session.new` request handling |
| `server/internal/hub/client/commands.go` | Modify | Remove `/use`, build `New Conversation` help menu, persist preference updates |
| `server/internal/hub/client/session_recorder.go` | Modify | Session summaries emit `agentType` and use single-agent metadata |
| `server/internal/hub/client/client_test.go` | Modify | Store, runtime, IM, and registry request tests for the hard cut |
| `server/internal/im/channel.go` | Modify | Remove `/use` from command allow-list |
| `server/internal/im/feishu/channel.go` | Modify | `/new` help-card interaction opens agent picker and keeps root refresh behavior |
| `server/internal/im/feishu/feishu_test.go` | Modify | Help card tests for new menu behavior |
| `server/internal/protocol/registry.go` | Modify | Add project `agents` field and session summary `agentType` consumers |
| `server/internal/hub/hub.go` | Modify | Publish available agent names in `ProjectInfo` |
| `server/internal/hub/hub_test.go` | Modify | Schema expectation tests updated for no `projects` table |
| `server/cmd/wheelmaker-monitor/monitor.go` | Modify | Monitor DTOs read `agent_json`, `agent_type`, and unified session ID |
| `server/cmd/wheelmaker-monitor/dashboard.go` | Modify | Show `agent_json`, remove `ACP Session`, drop `agents_json` modal labels |
| `server/cmd/wheelmaker-monitor/dashboard_test.go` | Modify | Dashboard string assertions for `agent_json` and unified session identity |
| `server/CLAUDE.md` | Modify | Remove `/use` from documented slash commands |
| `app/web/src/types/registry.ts` | Modify | Session summaries expose `agentType`, projects expose `agents` |
| `app/web/src/services/registryRepository.ts` | Modify | `createSession(projectId, agentType, title?)` and updated DTO normalization |
| `app/web/src/services/registryWorkspaceService.ts` | Modify | `createSession(agentType, title?)` pass-through |
| `app/web/src/main.tsx` | Modify | Agent picker before session creation, pending draft send flow, `agentType` rendering |
| `app/__tests__/web-chat-ui.test.ts` | Modify | String-based assertions for `agentType`, `agents`, and agent-required `session.new` |

---

### Task 1: Hard-Cut the SQLite Store Schema and Records

**Files:**
- Modify: `server/internal/hub/client/client_test.go`
- Modify: `server/internal/hub/client/sqlite_store.go`
- Modify: `server/internal/hub/hub_test.go`

- [ ] **Step 1: Add failing store tests for the new schema and hard-cut validation**

In `server/internal/hub/client/client_test.go`, add:

```go
func TestSQLiteStore_SessionAndAgentPreferenceRoundTrip_UsesAgentColumns(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "client.sqlite3"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	rec := &SessionRecord{
		ID:           "sess-1",
		ProjectName:  "proj1",
		Status:       SessionSuspended,
		AgentType:    "claude",
		AgentJSON:    `{"title":"Persisted","commands":[{"name":"/status"}]}`,
		Title:        "Persisted",
		CreatedAt:    time.Unix(10, 0).UTC(),
		LastActiveAt: time.Unix(20, 0).UTC(),
	}
	if err := store.SaveSession(ctx, rec); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}
	if err := store.SaveAgentPreference(ctx, AgentPreferenceRecord{
		ProjectName:    "proj1",
		AgentType:      "claude",
		PreferenceJSON: `{"configOptions":[{"id":"mode","currentValue":"code"}]}`,
	}); err != nil {
		t.Fatalf("SaveAgentPreference: %v", err)
	}

	loaded, err := store.LoadSession(ctx, "proj1", "sess-1")
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	if loaded == nil || loaded.AgentType != "claude" {
		t.Fatalf("LoadSession agentType = %+v, want claude", loaded)
	}
	if strings.Contains(loaded.AgentJSON, "acpSessionId") {
		t.Fatalf("AgentJSON should not contain acpSessionId: %s", loaded.AgentJSON)
	}

	pref, err := store.LoadAgentPreference(ctx, "proj1", "claude")
	if err != nil {
		t.Fatalf("LoadAgentPreference: %v", err)
	}
	if pref == nil || !strings.Contains(pref.PreferenceJSON, `"mode"`) {
		t.Fatalf("LoadAgentPreference = %+v, want mode config", pref)
	}

	entries, err := store.ListSessions(ctx, "proj1")
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(entries) != 1 || entries[0].AgentType != "claude" {
		t.Fatalf("ListSessions = %+v, want one claude session", entries)
	}
}

func TestCheckStoreSchema_RejectsLegacyProjectsAndSessionColumns(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "client.sqlite3")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	if _, err := db.Exec(`
		CREATE TABLE projects (
			project_name TEXT PRIMARY KEY,
			agent_state_json TEXT NOT NULL DEFAULT '{}',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);
		CREATE TABLE route_bindings (
			project_name TEXT NOT NULL,
			route_key TEXT NOT NULL,
			session_id TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			PRIMARY KEY (project_name, route_key)
		);
		CREATE TABLE sessions (
			id TEXT PRIMARY KEY,
			project_name TEXT NOT NULL,
			status INTEGER NOT NULL,
			acp_session_id TEXT NOT NULL DEFAULT '',
			agents_json TEXT NOT NULL DEFAULT '{}',
			title TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			last_active_at TEXT NOT NULL
		);
	`); err != nil {
		t.Fatalf("seed legacy schema: %v", err)
	}

	err = CheckStoreSchema(dbPath)
	if !IsStoreSchemaMismatch(err) {
		t.Fatalf("CheckStoreSchema() err = %v, want StoreSchemaMismatchError", err)
	}
}
```

In `server/internal/hub/hub_test.go`, update any raw schema seed that still creates `projects.agent_state_json` or `sessions.acp_session_id` / `sessions.agents_json` so the test continues to describe the current store shape after the implementation lands.

- [ ] **Step 2: Run the focused server tests and verify they fail**

Run:

```bash
cd D:/Code/WheelMaker/server
go test ./internal/hub/client ./internal/hub -run "TestSQLiteStore_SessionAndAgentPreferenceRoundTrip_UsesAgentColumns|TestCheckStoreSchema_RejectsLegacyProjectsAndSessionColumns" -count=1
```

Expected:
- `FAIL`
- missing `AgentType`, `AgentJSON`, `AgentPreferenceRecord`, `LoadAgentPreference`, or old schema validation still accepting the legacy tables

- [ ] **Step 3: Implement the new store schema, DTOs, and validation only**

In `server/internal/hub/client/sqlite_store.go`, replace the schema and record types with:

```go
const sqliteSchema = `
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
	agent_type TEXT NOT NULL,
	agent_json TEXT NOT NULL DEFAULT '{}',
	title TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL,
	last_active_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS agent_preferences (
	project_name TEXT NOT NULL,
	agent_type TEXT NOT NULL,
	preference_json TEXT NOT NULL DEFAULT '{}',
	PRIMARY KEY (project_name, agent_type)
);
CREATE TABLE IF NOT EXISTS session_prompts (
	session_id TEXT NOT NULL,
	prompt_index INTEGER NOT NULL,
	title TEXT NOT NULL DEFAULT '',
	stop_reason TEXT NOT NULL DEFAULT '',
	updated_at TEXT NOT NULL DEFAULT '',
	PRIMARY KEY (session_id, prompt_index)
);
CREATE TABLE IF NOT EXISTS session_turns (
	session_id TEXT NOT NULL,
	prompt_index INTEGER NOT NULL,
	turn_index INTEGER NOT NULL,
	update_json TEXT NOT NULL DEFAULT '{}',
	PRIMARY KEY (session_id, prompt_index, turn_index)
);
CREATE INDEX IF NOT EXISTS idx_route_bindings_project ON route_bindings(project_name);
CREATE INDEX IF NOT EXISTS idx_sessions_project_last_active ON sessions(project_name, last_active_at DESC);
CREATE INDEX IF NOT EXISTS idx_session_prompts_session_prompt ON session_prompts(session_id, prompt_index);
CREATE INDEX IF NOT EXISTS idx_session_turns_session_prompt_turn ON session_turns(session_id, prompt_index, turn_index);
`

type SessionRecord struct {
	ID           string
	ProjectName  string
	Status       SessionStatus
	AgentType    string
	AgentJSON    string
	Title        string
	Agent        string
	CreatedAt    time.Time
	LastActiveAt time.Time
	InMemory     bool
}

type AgentPreferenceRecord struct {
	ProjectName    string
	AgentType      string
	PreferenceJSON string
}

type Store interface {
	LoadRouteBindings(ctx context.Context, projectName string) (map[string]string, error)
	SaveRouteBinding(ctx context.Context, projectName, routeKey, sessionID string) error
	DeleteRouteBinding(ctx context.Context, projectName, routeKey string) error

	LoadSession(ctx context.Context, projectName, sessionID string) (*SessionRecord, error)
	SaveSession(ctx context.Context, rec *SessionRecord) error
	ListSessions(ctx context.Context, projectName string) ([]SessionRecord, error)
	DeleteSession(ctx context.Context, projectName, sessionID string) error

	LoadAgentPreference(ctx context.Context, projectName, agentType string) (*AgentPreferenceRecord, error)
	SaveAgentPreference(ctx context.Context, rec AgentPreferenceRecord) error

	UpsertSessionPrompt(ctx context.Context, rec SessionPromptRecord) error
	LoadSessionPrompt(ctx context.Context, projectName, sessionID string, promptIndex int64) (*SessionPromptRecord, error)
	ListSessionPrompts(ctx context.Context, projectName, sessionID string) ([]SessionPromptRecord, error)
	ListSessionPromptsAfterIndex(ctx context.Context, projectName, sessionID string, afterPromptIndex int64) ([]SessionPromptRecord, error)
	UpsertSessionTurn(ctx context.Context, rec SessionTurnRecord) error
	LoadSessionTurn(ctx context.Context, projectName, sessionID string, promptIndex, turnIndex int64) (*SessionTurnRecord, error)
	ListSessionTurns(ctx context.Context, projectName, sessionID string, promptIndex int64) ([]SessionTurnRecord, error)
	Close() error
}
```

Then remove the legacy migration path entirely and make schema validation strict:

```go
var expectedStoreSchemaColumns = map[string][]string{
	"route_bindings":    {"project_name", "route_key", "session_id", "created_at", "updated_at"},
	"sessions":          {"id", "project_name", "status", "agent_type", "agent_json", "title", "created_at", "last_active_at"},
	"agent_preferences": {"project_name", "agent_type", "preference_json"},
	"session_prompts":   {"session_id", "prompt_index", "title", "stop_reason", "updated_at"},
	"session_turns":     {"session_id", "prompt_index", "turn_index", "update_json"},
}

func NewStore(dbPath string) (Store, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("mkdir db dir: %w", err)
	}
	db, err := sql.Open("sqlite", dbPath+"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("open sqlite %q: %w", dbPath, err)
	}
	if _, err := db.Exec(sqliteSchema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("init schema: %w", err)
	}
	if err := CheckStoreSchema(dbPath); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &sqliteStore{db: db}, nil
}
```

Add the new preference methods and update session queries so they use `agent_type` and `agent_json` only:

```go
func (s *sqliteStore) LoadAgentPreference(ctx context.Context, projectName, agentType string) (*AgentPreferenceRecord, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT project_name, agent_type, preference_json
		FROM agent_preferences
		WHERE project_name = ? AND agent_type = ?
	`, strings.TrimSpace(projectName), strings.TrimSpace(agentType))

	var rec AgentPreferenceRecord
	if err := row.Scan(&rec.ProjectName, &rec.AgentType, &rec.PreferenceJSON); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("load agent preference: %w", err)
	}
	return &rec, nil
}

func (s *sqliteStore) SaveAgentPreference(ctx context.Context, rec AgentPreferenceRecord) error {
	rec.ProjectName = strings.TrimSpace(rec.ProjectName)
	rec.AgentType = strings.TrimSpace(rec.AgentType)
	if rec.ProjectName == "" || rec.AgentType == "" {
		return fmt.Errorf("project name and agent type are required")
	}
	if strings.TrimSpace(rec.PreferenceJSON) == "" {
		rec.PreferenceJSON = "{}"
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO agent_preferences (project_name, agent_type, preference_json)
		VALUES (?, ?, ?)
		ON CONFLICT(project_name, agent_type) DO UPDATE SET
			preference_json=excluded.preference_json
	`, rec.ProjectName, rec.AgentType, rec.PreferenceJSON)
	if err != nil {
		return fmt.Errorf("save agent preference: %w", err)
	}
	return nil
}
```

Also update `ListSessions` so `entry.Agent = strings.TrimSpace(entry.AgentType)` and infer title from `agent_json`, not from `acp_session_id`.

- [ ] **Step 4: Re-run the focused server tests and verify they pass**

Run:

```bash
cd D:/Code/WheelMaker/server
go test ./internal/hub/client ./internal/hub -run "TestSQLiteStore_SessionAndAgentPreferenceRoundTrip_UsesAgentColumns|TestCheckStoreSchema_RejectsLegacyProjectsAndSessionColumns" -count=1
```

Expected:
- `PASS`
- store tests assert only `agent_type`, `agent_json`, and `agent_preferences`

- [ ] **Step 5: Commit the schema hard cut**

```bash
git add server/internal/hub/client/sqlite_store.go server/internal/hub/client/client_test.go server/internal/hub/hub_test.go
git commit -m "refactor(client): hard-cut session store schema"
```

---

### Task 2: Collapse Runtime Sessions to a Single Agent and Unified Session ID

**Files:**
- Modify: `server/internal/hub/client/client_test.go`
- Modify: `server/internal/hub/client/session.go`
- Modify: `server/internal/hub/client/client.go`

- [ ] **Step 1: Add failing runtime tests for single-agent session restore and create flow**

In `server/internal/hub/client/client_test.go`, add:

```go
func TestSessionFromRecord_RestoresSingleAgentState(t *testing.T) {
	rec := &SessionRecord{
		ID:          "sess-restored",
		ProjectName: "proj1",
		Status:      SessionPersisted,
		AgentType:   "claude",
		AgentJSON:   `{"title":"Persisted","commands":[{"name":"/status"}]}`,
		Title:       "Persisted",
	}

	sess, err := sessionFromRecord(rec, "/tmp")
	if err != nil {
		t.Fatalf("sessionFromRecord: %v", err)
	}
	if sess.ID != "sess-restored" {
		t.Fatalf("ID = %q, want sess-restored", sess.ID)
	}
	if got := sess.agentType; got != "claude" {
		t.Fatalf("agentType = %q, want claude", got)
	}
	if got := sess.agentState.Title; got != "Persisted" {
		t.Fatalf("agentState.Title = %q, want Persisted", got)
	}
}

func TestCreateSessionWithAgent_UsesACPResultAsUnifiedSessionID(t *testing.T) {
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
		newResult: &acp.SessionNewResult{SessionID: "sess-from-agent"},
	}

	c := New(store, "proj1", "/tmp")
	c.registry = agent.DefaultACPFactory().Clone()
	c.registry.Register("claude", func(context.Context, string) (agent.Instance, error) { return inst, nil })

	sess, err := c.CreateSession(context.Background(), "claude", "hello")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if sess.ID != "sess-from-agent" {
		t.Fatalf("session ID = %q, want sess-from-agent", sess.ID)
	}

	loaded, err := store.LoadSession(context.Background(), "proj1", "sess-from-agent")
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	if loaded == nil || loaded.AgentType != "claude" {
		t.Fatalf("LoadSession = %+v, want agentType claude", loaded)
	}
}
```

- [ ] **Step 2: Run the focused runtime tests and verify they fail**

Run:

```bash
cd D:/Code/WheelMaker/server
go test ./internal/hub/client -run "TestSessionFromRecord_RestoresSingleAgentState|TestCreateSessionWithAgent_UsesACPResultAsUnifiedSessionID" -count=1
```

Expected:
- `FAIL`
- `Session` still exposes `activeAgent`, `agents`, `acpSessionID`, or `CreateSession` still takes only `title`

- [ ] **Step 3: Replace the multi-agent runtime model with a single-agent session**

In `server/internal/hub/client/session.go`, collapse the persisted state to one `agentType` plus one `agentState`:

```go
type SessionAgentState struct {
	ConfigOptions     []acp.ConfigOption     `json:"configOptions,omitempty"`
	Commands          []acp.AvailableCommand `json:"commands,omitempty"`
	Title             string                 `json:"title,omitempty"`
	UpdatedAt         string                 `json:"updatedAt,omitempty"`
	ProtocolVersion   string                 `json:"protocolVersion,omitempty"`
	AgentCapabilities acp.AgentCapabilities  `json:"agentCapabilities,omitempty"`
	AgentInfo         *acp.AgentInfo         `json:"agentInfo,omitempty"`
	AuthMethods       []acp.AuthMethod       `json:"authMethods,omitempty"`
	Sessions          []SessionSummary       `json:"sessions,omitempty"`
}

type Session struct {
	ID        string
	Status    SessionStatus
	agentType string
	agentState SessionAgentState
	instance  agent.Instance
	ready     bool
	initializing bool
	lastReply string
	...
}

func newSession(id, agentType, cwd string) *Session {
	return &Session{
		ID:        strings.TrimSpace(id),
		Status:    SessionActive,
		agentType: strings.TrimSpace(agentType),
		cwd:       cwd,
		createdAt: time.Now(),
		prompt: promptState{activeTCs: make(map[string]struct{})},
		timeoutLimiter: newTimeoutNotifyLimiter(timeoutNotifyCooldown),
	}
}
```

Update session restore and persistence so `ID` and `agentType` are the only identity fields:

```go
func (s *Session) toRecord() (*SessionRecord, error) {
	stateJSON, err := json.Marshal(s.agentState)
	if err != nil {
		return nil, fmt.Errorf("marshal agent_json: %w", err)
	}
	return &SessionRecord{
		ID:           strings.TrimSpace(s.ID),
		ProjectName:  s.projectName,
		Status:       s.Status,
		AgentType:    strings.TrimSpace(s.agentType),
		AgentJSON:    string(stateJSON),
		Title:        firstNonEmpty(strings.TrimSpace(s.agentState.Title), strings.TrimSpace(s.ID)),
		CreatedAt:    s.createdAt,
		LastActiveAt: s.lastActiveAt,
	}, nil
}

func sessionFromRecord(rec *SessionRecord, cwd string) (*Session, error) {
	s := newSession(rec.ID, rec.AgentType, cwd)
	s.Status = rec.Status
	s.createdAt = rec.CreatedAt
	s.lastActiveAt = rec.LastActiveAt
	if strings.TrimSpace(rec.AgentJSON) != "" {
		if err := json.Unmarshal([]byte(rec.AgentJSON), &s.agentState); err != nil {
			return nil, fmt.Errorf("unmarshal agent_json: %w", err)
		}
	}
	return s, nil
}
```

Then rewrite `ensureReady` so it uses `Session.ID` as the ACP session ID and sets it only from `session/new` when the session is brand new:

```go
func (s *Session) ensureReady(ctx context.Context) error {
	...
	savedSID := strings.TrimSpace(s.ID)
	if savedSID != "" && initResult.AgentCapabilities.LoadSession {
		loadResult, loadErr := inst.SessionLoad(ctx, acp.SessionLoadParams{
			SessionID:  savedSID,
			CWD:        cwd,
			MCPServers: emptyMCPServers(),
		})
		if loadErr == nil {
			s.mu.Lock()
			s.agentState.ConfigOptions = append([]acp.ConfigOption(nil), loadResult.ConfigOptions...)
			s.ready = true
			s.mu.Unlock()
			return nil
		}
	}

	newResult, err := inst.SessionNew(ctx, acp.SessionNewParams{CWD: cwd, MCPServers: emptyMCPServers()})
	if err != nil {
		return fmt.Errorf("ensureReady: session/new: %w", err)
	}
	s.mu.Lock()
	s.ID = strings.TrimSpace(newResult.SessionID)
	s.agentState.ConfigOptions = append([]acp.ConfigOption(nil), newResult.ConfigOptions...)
	s.ready = true
	s.mu.Unlock()
	return nil
}
```

In `server/internal/hub/client/client.go`, make creation explicit and agent-driven:

```go
func (c *Client) CreateSession(ctx context.Context, agentType, title string) (*Session, error) {
	agentType = strings.TrimSpace(agentType)
	if agentType == "" {
		return nil, fmt.Errorf("agent type is required")
	}
	sess := c.newWiredSession("", agentType)
	if err := sess.ensureInstance(ctx); err != nil {
		return nil, err
	}
	if err := sess.ensureReady(ctx); err != nil {
		return nil, err
	}
	if strings.TrimSpace(title) != "" {
		sess.mu.Lock()
		sess.agentState.Title = strings.TrimSpace(title)
		sess.mu.Unlock()
	}
	if err := sess.persistSession(ctx); err != nil {
		return nil, fmt.Errorf("save session: %w", err)
	}
	c.mu.Lock()
	c.sessions[sess.ID] = sess
	c.mu.Unlock()
	return sess, nil
}
```

Also remove the no-op `LoadProject` call from `Client.Start`, and change `newWiredSession` so it accepts `(id, agentType string)`.

- [ ] **Step 4: Re-run the focused runtime tests and verify they pass**

Run:

```bash
cd D:/Code/WheelMaker/server
go test ./internal/hub/client -run "TestSessionFromRecord_RestoresSingleAgentState|TestCreateSessionWithAgent_UsesACPResultAsUnifiedSessionID" -count=1
```

Expected:
- `PASS`
- runtime uses `agentType`, `agentState`, and unified `Session.ID`

- [ ] **Step 5: Commit the runtime collapse**

```bash
git add server/internal/hub/client/session.go server/internal/hub/client/client.go server/internal/hub/client/client_test.go
git commit -m "refactor(client): collapse sessions to one agent"
```

---

### Task 3: Rework `/new`, Registry `session.new`, Help Menus, and Remove `/use`

**Files:**
- Modify: `server/internal/hub/client/client_test.go`
- Modify: `server/internal/hub/client/client.go`
- Modify: `server/internal/hub/client/commands.go`
- Modify: `server/internal/im/channel.go`
- Modify: `server/internal/im/feishu/channel.go`
- Modify: `server/internal/im/feishu/feishu_test.go`
- Modify: `server/CLAUDE.md`

- [ ] **Step 1: Add failing tests for `session.new`, `/new`, and the help root menu**

In `server/internal/hub/client/client_test.go`, add:

```go
func TestHandleSessionRequest_SessionNewRequiresAgentType(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "client.sqlite3"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	c := New(store, "proj1", "/tmp")
	_, err = c.HandleSessionRequest(context.Background(), "session.new", "proj1", json.RawMessage(`{"title":"hello"}`))
	if err == nil || !strings.Contains(err.Error(), "agentType is required") {
		t.Fatalf("HandleSessionRequest() err = %v, want agentType is required", err)
	}
}

func TestResolveHelpModel_RootStartsWithNewConversationMenu(t *testing.T) {
	s := newSession("sess-help", "claude", "/tmp")
	s.registry = agent.DefaultACPFactory().Clone()
	s.registry.Register("claude", func(context.Context, string) (agent.Instance, error) { return &testInjectedInstance{name: "claude"}, nil })
	s.registry.Register("codex", func(context.Context, string) (agent.Instance, error) { return &testInjectedInstance{name: "codex"}, nil })

	model, err := s.resolveHelpModel(context.Background(), "")
	if err != nil {
		t.Fatalf("resolveHelpModel: %v", err)
	}
	if len(model.Options) == 0 || model.Options[0].Label != "New Conversation" {
		t.Fatalf("root option[0] = %+v, want New Conversation", model.Options)
	}
	if _, ok := model.Menus["menu:new"]; !ok {
		t.Fatalf("menus = %+v, want menu:new", model.Menus)
	}
	for _, opt := range model.Options {
		if strings.Contains(opt.Label, "Switch Agent") || opt.Command == "/use" {
			t.Fatalf("unexpected switch-agent option: %+v", opt)
		}
	}
}
```

In `server/internal/im/feishu/feishu_test.go`, add:

```go
func TestHandleHelpOptionAction_NewConversationUsesNewCommand(t *testing.T) {
	ft := newFakeTransport()
	ch := NewChannel("app", ft, "app-id", "app-secret", "encrypt")
	var cmds []im.Command
	ch.SetCommandHandler(func(_ context.Context, _ im.ChatRef, cmd im.Command) error {
		cmds = append(cmds, cmd)
		return nil
	})

	ch.handleHelpOptionAction(CardActionEvent{
		ChatID:    "chat-1",
		MessageID: "om_1",
		Value: map[string]string{
			"kind":    "help_option",
			"command": "/new",
			"value":   "claude",
		},
	})

	if len(cmds) == 0 || cmds[0].Name != "/new" || cmds[0].Args != "claude" {
		t.Fatalf("commands = %+v, want /new claude", cmds)
	}
}
```

- [ ] **Step 2: Run the focused command/help tests and verify they fail**

Run:

```bash
cd D:/Code/WheelMaker/server
go test ./internal/hub/client ./internal/im/feishu -run "TestHandleSessionRequest_SessionNewRequiresAgentType|TestResolveHelpModel_RootStartsWithNewConversationMenu|TestHandleHelpOptionAction_NewConversationUsesNewCommand" -count=1
```

Expected:
- `FAIL`
- `session.new` still accepts missing `agentType`, help still renders agent switch, or `/use` still appears in commands/help

- [ ] **Step 3: Implement the new command and protocol flow, and delete `/use`**

In `server/internal/hub/client/client.go`, require `agentType` in registry `session.new` and make IM `/new` bifurcate on whether an agent argument was supplied:

```go
case "session.new":
	var req struct {
		AgentType string `json:"agentType"`
		Title     string `json:"title,omitempty"`
	}
	if err := decodeSessionRequestPayload(payload, &req); err != nil {
		return nil, fmt.Errorf("invalid session.new payload: %w", err)
	}
	if strings.TrimSpace(req.AgentType) == "" {
		return nil, fmt.Errorf("agentType is required")
	}
	sess, err := c.CreateSession(ctx, req.AgentType, req.Title)
	...
```

```go
if cmd == "/new" {
	if strings.TrimSpace(args) == "" {
		sess := c.resolveOrCreateIMSession(ctx, source, routeKey)
		if sess == nil {
			return nil
		}
		sess.setIMSource(source)
		model, err := sess.resolveHelpModel(ctx, source.ChatID)
		if err != nil {
			return c.sendIMDirect(ctx, source, fmt.Sprintf("New error: %v", err))
		}
		return c.sendHelpCard(ctx, source, model, "menu:new", 0)
	}
	sess, err := c.ClientNewSession(routeKey, strings.TrimSpace(args))
	...
}
```

In `server/internal/hub/client/commands.go`, delete the `/use` branch completely and change the help model to use a `menu:new` submenu:

```go
newMenuID := "menu:new"
model.Options = append(model.Options, HelpOption{
	Label:  "New Conversation",
	MenuID: newMenuID,
})
newMenu := HelpMenu{
	Title:  "New Conversation",
	Body:   "Choose an agent for the new conversation.",
	Parent: model.RootMenu,
}
for _, name := range s.registry.Names() {
	newMenu.Options = append(newMenu.Options, HelpOption{
		Label:   "Agent: " + name,
		Command: "/new",
		Value:   name,
	})
}
model.Menus[newMenuID] = newMenu
```

In `server/internal/hub/client/client.go`, change default route creation so freeform prompts on an unbound route still create a session with the preferred agent, but now through the explicit agent-typed creation helper:

```go
func (c *Client) resolveOrCreateIMSession(ctx context.Context, source im.ChatRef, routeKey string) *Session {
	...
	agentType := c.preferredAvailableAgent()
	if agentType == "" {
		_ = c.sendIMDirect(ctx, source, "No available agent.")
		return nil
	}
	sess, err := c.ClientNewSession(routeKey, agentType)
	...
}
```

Then remove `/use` from every allow-list and doc reference:

```go
// server/internal/im/channel.go
case "/cancel", "/status", "/mode", "/model", "/config", "/list", "/new", "/load", "/help":
	return true
```

And in `server/CLAUDE.md`, change:

```md
- Slash commands: `/cancel` `/status` `/mode` `/model` `/list` `/new` `/load` `/config`
```

- [ ] **Step 4: Re-run the focused command/help tests and verify they pass**

Run:

```bash
cd D:/Code/WheelMaker/server
go test ./internal/hub/client ./internal/im/feishu -run "TestHandleSessionRequest_SessionNewRequiresAgentType|TestResolveHelpModel_RootStartsWithNewConversationMenu|TestHandleHelpOptionAction_NewConversationUsesNewCommand" -count=1
```

Expected:
- `PASS`
- `session.new` requires `agentType`, `/new` without args opens the agent picker, and `/use` is gone

- [ ] **Step 5: Commit the command/help rewrite**

```bash
git add server/internal/hub/client/client.go server/internal/hub/client/commands.go server/internal/hub/client/client_test.go server/internal/im/channel.go server/internal/im/feishu/channel.go server/internal/im/feishu/feishu_test.go server/CLAUDE.md
git commit -m "refactor(client): route new conversations through agent selection"
```

---

### Task 4: Update Session Summaries, Monitor Rendering, Registry Project Metadata, and Web Create-Session UX

**Files:**
- Modify: `server/internal/hub/client/client_test.go`
- Modify: `server/internal/hub/client/session_recorder.go`
- Modify: `server/internal/protocol/registry.go`
- Modify: `server/internal/hub/hub.go`
- Modify: `server/cmd/wheelmaker-monitor/monitor.go`
- Modify: `server/cmd/wheelmaker-monitor/dashboard.go`
- Modify: `server/cmd/wheelmaker-monitor/dashboard_test.go`
- Modify: `app/web/src/types/registry.ts`
- Modify: `app/web/src/services/registryRepository.ts`
- Modify: `app/web/src/services/registryWorkspaceService.ts`
- Modify: `app/web/src/main.tsx`
- Modify: `app/__tests__/web-chat-ui.test.ts`

- [ ] **Step 1: Add failing tests for `agentType`, project `agents`, and the web/monitor create-session flow**

In `server/cmd/wheelmaker-monitor/dashboard_test.go`, add:

```go
func TestDashboardHTML_UsesAgentJSONAndUnifiedSessionIdentity(t *testing.T) {
	if strings.Contains(dashboardHTML, "ACP Session") {
		t.Fatalf("dashboard should not render ACP Session once session.id is unified")
	}
	if strings.Contains(dashboardHTML, "agents_json") {
		t.Fatalf("dashboard should not reference agents_json")
	}
	if !strings.Contains(dashboardHTML, "agent_json") {
		t.Fatalf("dashboard should expose agent_json column hooks")
	}
}
```

In `app/__tests__/web-chat-ui.test.ts`, extend the existing assertions with:

```ts
expect(registryTypes).toContain('agentType?: string;');
expect(registryTypes).toContain('agents?: string[];');
expect(repositoryTs).toContain('async createSession(projectId: string, agentType: string, title?: string)');
expect(repositoryTs).toContain('payload: title?.trim() ? {agentType, title: title.trim()} : {agentType}');
expect(workspaceServiceTs).toContain('async createSession(agentType: string, title?: string)');
expect(mainTsx).toContain('const [newChatAgentPickerOpen, setNewChatAgentPickerOpen] = useState(false);');
expect(mainTsx).toContain('const [pendingNewChatDraft, setPendingNewChatDraft] = useState<PendingNewChatDraft | null>(null);');
expect(mainTsx).toContain('await service.createSession(agentType, title);');
expect(mainTsx).toContain('project?.agents ?? []');
```

- [ ] **Step 2: Run the focused monitor and app/web tests and verify they fail**

Run:

```bash
cd D:/Code/WheelMaker/server
go test ./cmd/wheelmaker-monitor -run "TestDashboardHTML_UsesAgentJSONAndUnifiedSessionIdentity" -count=1

cd D:/Code/WheelMaker/app
npm test -- --runTestsByPath __tests__/web-chat-ui.test.ts --runInBand
```

Expected:
- Go test fails because dashboard still references `ACP Session` / `agents_json`
- Jest fails because types and `createSession` still use `agent` or omit agent selection state

- [ ] **Step 3: Implement the summary/type/UI changes with the smallest end-to-end slice**

In `server/internal/hub/client/session_recorder.go`, rename session summary output to `agentType`:

```go
type sessionViewSummary struct {
	SessionID string `json:"sessionId"`
	Title     string `json:"title"`
	UpdatedAt string `json:"updatedAt"`
	AgentType string `json:"agentType,omitempty"`
}

func buildSessionViewSummary(sessionID, title string, lastActiveAt time.Time, agentType string) sessionViewSummary {
	return sessionViewSummary{
		SessionID: sessionID,
		Title:     title,
		UpdatedAt: lastActiveAt.UTC().Format(time.RFC3339Nano),
		AgentType: strings.TrimSpace(agentType),
	}
}
```

In `server/internal/protocol/registry.go` and `server/internal/hub/hub.go`, expose available agent names to the web client:

```go
type ProjectInfo struct {
	Name       string          `json:"name"`
	Path       string          `json:"path"`
	Online     bool            `json:"online"`
	Agent      string          `json:"agent"`
	Agents     []string        `json:"agents,omitempty"`
	IMType     string          `json:"imType"`
	ProjectRev string          `json:"projectRev"`
	Git        ProjectGitState `json:"git"`
}

func (h *Hub) collectProjectInfo(cfgProject logger.ProjectConfig) ProjectInfo {
	...
	info.Agents = append([]string(nil), agent.DefaultACPFactory().Names()...)
	return info
}
```

In `server/cmd/wheelmaker-monitor/dashboard.go`, swap `agents_json` hooks to `agent_json` and remove the extra ACP-session panel:

```js
if (col.toLowerCase() === 'agent_json') {
  return '<button class="json-cell-btn" onclick="openJSONModal(' + key + ',\'agent_json\')">View JSON</button>';
}
...
const isAgentJSON = isSessionsTable && colName === 'agent_json';
...
// Delete the '<div class="json-k">ACP Session</div>' block entirely.
```

In `app/web/src/types/registry.ts`, `registryRepository.ts`, and `registryWorkspaceService.ts`, require `agentType` on create and surface `agents` from projects:

```ts
export interface RegistrySessionSummary {
  sessionId: string;
  title: string;
  preview: string;
  updatedAt: string;
  messageCount: number;
  unreadCount?: number;
  agentType?: string;
}

export interface RegistryProject {
  projectId: string;
  name: string;
  online: boolean;
  path: string;
  hubId?: string;
  agent?: string;
  agents?: string[];
  imType?: string;
  projectRev?: string;
  git?: RegistryProjectGitState;
}

async createSession(projectId: string, agentType: string, title?: string): Promise<{ok: boolean; session: RegistrySessionSummary}> {
  const resp = await this.client.request({
    method: 'session.new',
    projectId,
    payload: title?.trim() ? {agentType, title: title.trim()} : {agentType},
    timeoutMs: 15000,
  });
  ...
}

async createSession(agentType: string, title?: string): Promise<{ok: boolean; session: RegistrySessionSummary}> {
  if (!this.session || !this.repository) throw new Error('session is not ready');
  return this.repository.createSession(this.session.selectedProjectId, agentType, title);
}
```

In `app/web/src/main.tsx`, add a pending draft + agent picker so a brand-new send does not create a session until an agent is chosen:

```tsx
type PendingNewChatDraft = {
  title: string;
  text: string;
  blocks: RegistryChatContentBlock[];
};

const [newChatAgentPickerOpen, setNewChatAgentPickerOpen] = useState(false);
const [pendingNewChatDraft, setPendingNewChatDraft] = useState<PendingNewChatDraft | null>(null);

const createChatSession = async (agentType: string, title = '') => {
  const result = await service.createSession(agentType, title);
  if (!result.session.sessionId) {
    throw new Error('Session was created without a sessionId');
  }
  setChatSessions(prev => mergeChatSession(prev, result.session));
  setSelectedChatId(result.session.sessionId);
  setChatMessages([]);
  return result.session.sessionId;
};

const beginNewChatFlow = (draft: PendingNewChatDraft) => {
  setPendingNewChatDraft(draft);
  setNewChatAgentPickerOpen(true);
};
```

Then replace the implicit auto-create branch in `sendChatMessage`:

```tsx
if (!selectedChatId) {
  beginNewChatFlow({
    title: trimmedText || chatAttachment?.name || '',
    text: trimmedText,
    blocks,
  });
  setChatSending(false);
  return;
}
```

And render the picker using project agent names:

```tsx
const project = projects.find(item => item.projectId === activeProjectId);
const availableChatAgents = project?.agents ?? [];

{newChatAgentPickerOpen && pendingNewChatDraft ? (
  <div className="chat-agent-picker-card">
    <h3>Choose Agent</h3>
    {availableChatAgents.map(agentType => (
      <button
        key={agentType}
        type="button"
        onClick={async () => {
          const sessionId = await createChatSession(agentType, pendingNewChatDraft.title);
          setNewChatAgentPickerOpen(false);
          setPendingNewChatDraft(null);
          if (sessionId) {
            await service.sendSessionMessage({
              sessionId,
              text: pendingNewChatDraft.text,
              blocks: pendingNewChatDraft.blocks,
            });
          }
        }}
      >
        {agentType}
      </button>
    ))}
  </div>
) : null}
```

- [ ] **Step 4: Re-run the focused monitor and app/web tests and verify they pass**

Run:

```bash
cd D:/Code/WheelMaker/server
go test ./cmd/wheelmaker-monitor -run "TestDashboardHTML_UsesAgentJSONAndUnifiedSessionIdentity" -count=1

cd D:/Code/WheelMaker/app
npm test -- --runTestsByPath __tests__/web-chat-ui.test.ts --runInBand
```

Expected:
- monitor tests pass with `agent_json` and no `ACP Session`
- Jest passes with `agentType`, `agents`, and agent-required `createSession`

- [ ] **Step 5: Commit the surface updates**

```bash
git add server/internal/hub/client/session_recorder.go server/internal/protocol/registry.go server/internal/hub/hub.go server/cmd/wheelmaker-monitor/monitor.go server/cmd/wheelmaker-monitor/dashboard.go server/cmd/wheelmaker-monitor/dashboard_test.go app/web/src/types/registry.ts app/web/src/services/registryRepository.ts app/web/src/services/registryWorkspaceService.ts app/web/src/main.tsx app/__tests__/web-chat-ui.test.ts
git commit -m "feat(app): require agent choice for new sessions"
```

---

## Self-Review Checklist

### Spec Coverage

This plan covers all approved design requirements:

1. `sessions.id == ACP session_id` is implemented in Task 2.
2. `sessions.agent_json` and `sessions.agent_type` replace the old split model in Tasks 1 and 2.
3. `agent_preferences` replaces `projects.agent_state_json` in Task 1 and is used in Tasks 2 and 3.
4. `/use` removal is implemented in Task 3.
5. `/help` root switching to `New Conversation` is implemented in Task 3.
6. `session.new` requiring explicit `agentType` is implemented in Task 3.
7. monitor and web/session surfaces consuming `agentType` and `agents` are implemented in Task 4.

### Placeholder Scan

Checked for disallowed placeholders:

1. No `TODO`, `TBD`, or "similar to previous task" placeholders remain.
2. Every task includes concrete files, code snippets, commands, and expected outcomes.
3. No task asks for generic "add tests" without specific test code.

### Type Consistency

Verified planned names are consistent:

1. Store DTOs use `AgentType`, `AgentJSON`, and `AgentPreferenceRecord` throughout.
2. Session summary output uses `agentType` in Go JSON tags and TypeScript interfaces.
3. Session creation uses `CreateSession(ctx, agentType, title)` on the server and `createSession(agentType, title?)` in the web service.
