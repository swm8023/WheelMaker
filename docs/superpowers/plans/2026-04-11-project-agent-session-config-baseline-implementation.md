# Project-Agent Session Config Baseline Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make new sessions inherit the last saved `mode/model/thought_level` for the same `project + agent`, while keeping old sessions stable through session-local restore and caching recent `available commands` / `config options` in `projects.agent_state_json`.

**Architecture:** Extend the existing SQLite `projects` row with one `agent_state_json` column, teach the store to load/save project-agent baseline state, and route `/new`, `session/load`, and `session/load -> session/new` fallback through one replay white-list helper. Session-local state remains in `sessions.agents_json`; project-agent baseline becomes the inheritance source for fresh sessions and missing replayable values.

**Tech Stack:** Go, SQLite (`modernc.org/sqlite`), ACP config option types, `server/internal/hub/client`, `server/internal/protocol`

---

## File Map

| File | Action | Responsibility after change |
|------|--------|-----------------------------|
| `server/internal/hub/client/sqlite_store.go` | Modify | Add `agent_state_json` schema column and project-agent baseline load/save support |
| `server/internal/hub/client/session.go` | Modify | Replay white-list helpers, unified load/new resolution, session + project-agent cache updates |
| `server/internal/hub/client/client.go` | Modify | `/new` path uses project-agent baseline after `session/new`; keep `cancel` behavior config-neutral |
| `server/internal/hub/client/commands.go` | Modify | Config command success path updates project-agent baseline alongside session cache |
| `server/internal/protocol/acp.go` | Modify | Add replayable snapshot extraction for `mode/model/thought_level` |
| `server/internal/hub/client/client_runtime_test.go` | Modify | Runtime tests for baseline inheritance, load conflict resolution, and cancel stability |
| `server/internal/hub/client/store_test.go` | Create or Modify | Store tests for `projects.agent_state_json` round-trip |

---

### Task 1: Add Failing Tests for Project-Agent Baseline and Replay White List

**Files:**
- Modify: `server/internal/hub/client/client_runtime_test.go`
- Create: `server/internal/hub/client/store_test.go`

- [ ] **Step 1: Add a failing store round-trip test for `projects.agent_state_json`**

Create `server/internal/hub/client/store_test.go` with:

```go
package client

import (
    "context"
    "path/filepath"
    "testing"

    acp "github.com/swm8023/wheelmaker/internal/protocol"
)

func TestStoreProjectAgentStateRoundTrip(t *testing.T) {
    store, err := NewStore(filepath.Join(t.TempDir(), "client.sqlite3"))
    if err != nil {
        t.Fatalf("NewStore: %v", err)
    }
    defer store.Close()

    cfg := ProjectConfig{
        YOLO: true,
        AgentState: map[string]ProjectAgentState{
            "codex": {
                ConfigOptions: []acp.ConfigOption{
                    {ID: acp.ConfigOptionIDMode, Category: acp.ConfigOptionCategoryMode, CurrentValue: "code"},
                    {ID: acp.ConfigOptionIDModel, Category: acp.ConfigOptionCategoryModel, CurrentValue: "gpt-5"},
                    {ID: acp.ConfigOptionIDThoughtLevel, Category: acp.ConfigOptionCategoryThoughtLv, CurrentValue: "high"},
                },
                AvailableCommands: []acp.AvailableCommand{{Name: "/status"}},
                UpdatedAt:         "2026-04-11T00:00:00Z",
            },
        },
    }
    if err := store.SaveProject(context.Background(), "proj1", cfg); err != nil {
        t.Fatalf("SaveProject: %v", err)
    }

    loaded, err := store.LoadProject(context.Background(), "proj1")
    if err != nil {
        t.Fatalf("LoadProject: %v", err)
    }
    if !loaded.YOLO {
        t.Fatal("YOLO = false, want true")
    }
    codex := loaded.AgentState["codex"]
    if got := len(codex.ConfigOptions); got != 3 {
        t.Fatalf("config options = %d, want 3", got)
    }
    if got := len(codex.AvailableCommands); got != 1 {
        t.Fatalf("commands = %d, want 1", got)
    }
}
```

- [ ] **Step 2: Add a failing runtime test for `/new` inheriting project-agent baseline**

In `server/internal/hub/client/client_runtime_test.go`, add:

```go
func TestClientNewSession_ReappliesProjectAgentBaseline(t *testing.T) {
    store, err := NewStore(filepath.Join(t.TempDir(), "client.sqlite3"))
    if err != nil {
        t.Fatalf("NewStore: %v", err)
    }
    defer store.Close()

    if err := store.SaveProject(context.Background(), "proj1", ProjectConfig{
        AgentState: map[string]ProjectAgentState{
            "claude": {
                ConfigOptions: []acp.ConfigOption{
                    {ID: acp.ConfigOptionIDMode, Category: acp.ConfigOptionCategoryMode, CurrentValue: "code"},
                    {ID: acp.ConfigOptionIDModel, Category: acp.ConfigOptionCategoryModel, CurrentValue: "gpt-5"},
                    {ID: acp.ConfigOptionIDThoughtLevel, Category: acp.ConfigOptionCategoryThoughtLv, CurrentValue: "high"},
                },
            },
        },
    }); err != nil {
        t.Fatalf("SaveProject: %v", err)
    }

    c := New(store, "proj1", "/tmp")
    inst := &testInjectedInstance{
        name: "claude",
        initResult: acp.InitializeResult{ProtocolVersion: "0.1", AgentCapabilities: acp.AgentCapabilities{}},
        newResult: &acp.SessionNewResult{SessionID: "acp-new", ConfigOptions: []acp.ConfigOption{
            {ID: acp.ConfigOptionIDMode, Category: acp.ConfigOptionCategoryMode, CurrentValue: "ask"},
            {ID: acp.ConfigOptionIDModel, Category: acp.ConfigOptionCategoryModel, CurrentValue: "gpt-4o-mini"},
            {ID: acp.ConfigOptionIDThoughtLevel, Category: acp.ConfigOptionCategoryThoughtLv, CurrentValue: "low"},
        }},
    }
    c.registry = agent.DefaultACPFactory().Clone()
    c.registry.Register(acp.ACPProviderClaude, func(context.Context) (agent.Instance, error) { return inst, nil })

    sess := c.newWiredSession("sess-1")
    if err := sess.ensureInstance(context.Background()); err != nil {
        t.Fatalf("ensureInstance: %v", err)
    }
    if err := sess.ensureReady(context.Background()); err != nil {
        t.Fatalf("ensureReady: %v", err)
    }

    if got := len(inst.setCalls); got != 3 {
        t.Fatalf("set calls = %d, want 3", got)
    }
}
```

- [ ] **Step 3: Add a failing runtime test for `session/load` preferring session-local replayable values only**

Add to `server/internal/hub/client/client_runtime_test.go`:

```go
func TestEnsureReady_SessionLoadSuccess_ReplaysOnlyReplayableSessionValues(t *testing.T) {
    s := newSession("restore-load-success", "/tmp")
    s.projectName = "proj1"
    s.activeAgent = "claude"
    s.acpSessionID = "acp-old"
    s.agents = map[string]*SessionAgentState{
        "claude": {
            ACPSessionID: "acp-old",
            ConfigOptions: []acp.ConfigOption{
                {ID: acp.ConfigOptionIDMode, Category: acp.ConfigOptionCategoryMode, CurrentValue: "code"},
                {ID: acp.ConfigOptionIDModel, Category: acp.ConfigOptionCategoryModel, CurrentValue: "gpt-5"},
                {ID: acp.ConfigOptionIDThoughtLevel, Category: acp.ConfigOptionCategoryThoughtLv, CurrentValue: "high"},
                {ID: "custom_toggle", CurrentValue: "persisted-custom"},
            },
        },
    }

    inst := &testInjectedInstance{
        name: "claude",
        sessionID: "acp-old",
        initResult: acp.InitializeResult{ProtocolVersion: "0.1", AgentCapabilities: acp.AgentCapabilities{LoadSession: true}},
        loadResult: acp.SessionLoadResult{ConfigOptions: []acp.ConfigOption{
            {ID: acp.ConfigOptionIDMode, Category: acp.ConfigOptionCategoryMode, CurrentValue: "ask"},
            {ID: acp.ConfigOptionIDModel, Category: acp.ConfigOptionCategoryModel, CurrentValue: "gpt-4o-mini"},
            {ID: acp.ConfigOptionIDThoughtLevel, Category: acp.ConfigOptionCategoryThoughtLv, CurrentValue: "low"},
            {ID: "custom_toggle", CurrentValue: "agent-custom"},
        }},
    }
    inst.setConfigFn = func(_ context.Context, p acp.SessionSetConfigOptionParams) ([]acp.ConfigOption, error) {
        switch p.ConfigID {
        case acp.ConfigOptionIDMode:
            return []acp.ConfigOption{
                {ID: acp.ConfigOptionIDMode, Category: acp.ConfigOptionCategoryMode, CurrentValue: p.Value},
                {ID: acp.ConfigOptionIDModel, Category: acp.ConfigOptionCategoryModel, CurrentValue: "gpt-4o-mini"},
                {ID: acp.ConfigOptionIDThoughtLevel, Category: acp.ConfigOptionCategoryThoughtLv, CurrentValue: "low"},
                {ID: "custom_toggle", CurrentValue: "agent-custom"},
            }, nil
        case acp.ConfigOptionIDModel:
            return []acp.ConfigOption{
                {ID: acp.ConfigOptionIDMode, Category: acp.ConfigOptionCategoryMode, CurrentValue: "code"},
                {ID: acp.ConfigOptionIDModel, Category: acp.ConfigOptionCategoryModel, CurrentValue: p.Value},
                {ID: acp.ConfigOptionIDThoughtLevel, Category: acp.ConfigOptionCategoryThoughtLv, CurrentValue: "low"},
                {ID: "custom_toggle", CurrentValue: "agent-custom"},
            }, nil
        case acp.ConfigOptionIDThoughtLevel:
            return []acp.ConfigOption{
                {ID: acp.ConfigOptionIDMode, Category: acp.ConfigOptionCategoryMode, CurrentValue: "code"},
                {ID: acp.ConfigOptionIDModel, Category: acp.ConfigOptionCategoryModel, CurrentValue: "gpt-5"},
                {ID: acp.ConfigOptionIDThoughtLevel, Category: acp.ConfigOptionCategoryThoughtLv, CurrentValue: p.Value},
                {ID: "custom_toggle", CurrentValue: "agent-custom"},
            }, nil
        default:
            return nil, errors.New("unexpected config id")
        }
    }

    s.registry = agent.DefaultACPFactory().Clone()
    s.registry.Register(acp.ACPProviderClaude, func(context.Context) (agent.Instance, error) { return inst, nil })

    if err := s.ensureInstance(context.Background()); err != nil {
        t.Fatalf("ensureInstance: %v", err)
    }
    if err := s.ensureReady(context.Background()); err != nil {
        t.Fatalf("ensureReady: %v", err)
    }

    state, _ := s.currentAgentStateSnapshot()
    if findCurrentValue(state.ConfigOptions, "custom_toggle") != "agent-custom" {
        t.Fatalf("custom_toggle should stay agent-owned")
    }
    if got := len(inst.setCalls); got != 3 {
        t.Fatalf("set calls = %d, want 3", got)
    }
}
```

- [ ] **Step 4: Run the targeted tests to confirm they fail**

Run:

```bash
cd D:/Code/WheelMaker/server
go test ./internal/hub/client -run "TestStoreProjectAgentStateRoundTrip|TestClientNewSession_ReappliesProjectAgentBaseline|TestEnsureReady_SessionLoadSuccess_ReplaysOnlyReplayableSessionValues" -count=1
```

Expected:
- `FAIL`
- missing `ProjectAgentState`, `ProjectConfig.AgentState`, or replay helper behavior

- [ ] **Step 5: Commit the failing tests**

```bash
git add server/internal/hub/client/store_test.go server/internal/hub/client/client_runtime_test.go
git commit -m "test(client): add failing project-agent baseline tests"
```

---

### Task 2: Extend the Store with `projects.agent_state_json`

**Files:**
- Modify: `server/internal/hub/client/sqlite_store.go`
- Modify: `server/internal/hub/client/store_test.go`

- [ ] **Step 1: Add the project-agent cache types**

In `server/internal/hub/client/sqlite_store.go`, extend the project config types:

```go
type ProjectAgentState struct {
    ConfigOptions     []acp.ConfigOption     `json:"configOptions,omitempty"`
    AvailableCommands []acp.AvailableCommand `json:"availableCommands,omitempty"`
    UpdatedAt         string                 `json:"updatedAt,omitempty"`
}

type ProjectConfig struct {
    YOLO       bool
    AgentState map[string]ProjectAgentState
}
```

Add imports:

```go
import (
    "encoding/json"

    acp "github.com/swm8023/wheelmaker/internal/protocol"
)
```

- [ ] **Step 2: Add the schema column and load/save support**

Update the schema:

```go
CREATE TABLE IF NOT EXISTS projects (
    project_name TEXT PRIMARY KEY,
    yolo INTEGER NOT NULL DEFAULT 0,
    agent_state_json TEXT NOT NULL DEFAULT '{}',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
```

Update project load/save queries:

```go
func (s *sqliteStore) LoadProject(ctx context.Context, projectName string) (*ProjectConfig, error) {
    row := s.db.QueryRowContext(ctx, `SELECT yolo, agent_state_json FROM projects WHERE project_name = ?`, strings.TrimSpace(projectName))

    var yolo int
    var agentStateJSON string
    if err := row.Scan(&yolo, &agentStateJSON); err != nil {
        if err == sql.ErrNoRows {
            return &ProjectConfig{AgentState: map[string]ProjectAgentState{}}, nil
        }
        return nil, fmt.Errorf("load project: %w", err)
    }

    cfg := &ProjectConfig{YOLO: yolo != 0, AgentState: map[string]ProjectAgentState{}}
    if strings.TrimSpace(agentStateJSON) != "" {
        if err := json.Unmarshal([]byte(agentStateJSON), &cfg.AgentState); err != nil {
            return nil, fmt.Errorf("unmarshal agent_state_json: %w", err)
        }
    }
    return cfg, nil
}

func (s *sqliteStore) SaveProject(ctx context.Context, projectName string, cfg ProjectConfig) error {
    if cfg.AgentState == nil {
        cfg.AgentState = map[string]ProjectAgentState{}
    }
    raw, err := json.Marshal(cfg.AgentState)
    if err != nil {
        return fmt.Errorf("marshal agent_state_json: %w", err)
    }
    // existing UPSERT gains agent_state_json=excluded.agent_state_json
}
```

- [ ] **Step 3: Run the store test and verify it passes**

Run:

```bash
go test ./internal/hub/client -run TestStoreProjectAgentStateRoundTrip -count=1
```

Expected:
- `PASS`

- [ ] **Step 4: Commit**

```bash
git add server/internal/hub/client/sqlite_store.go server/internal/hub/client/store_test.go
git commit -m "feat(client): persist project-agent baseline state in projects table"
```

---

### Task 3: Add Replayable Snapshot Helpers for `mode/model/thought_level`

**Files:**
- Modify: `server/internal/protocol/acp.go`
- Modify: `server/internal/hub/client/session.go`

- [ ] **Step 1: Extend the compact snapshot type**

In `server/internal/protocol/acp.go`, change:

```go
type SessionConfigSnapshot struct {
    Mode  string
    Model string
}
```

to:

```go
type SessionConfigSnapshot struct {
    Mode         string
    Model        string
    ThoughtLevel string
}
```

Update extraction:

```go
func SessionConfigSnapshotFromOptions(opts []ConfigOption) SessionConfigSnapshot {
    snap := SessionConfigSnapshot{}
    for _, opt := range opts {
        if snap.Mode == "" && (opt.ID == ConfigOptionIDMode || opt.Category == ConfigOptionCategoryMode) {
            snap.Mode = resolveOptionDisplayValue(opt)
        }
        if snap.Model == "" && (opt.ID == ConfigOptionIDModel || opt.Category == ConfigOptionCategoryModel) {
            snap.Model = resolveOptionDisplayValue(opt)
        }
        if snap.ThoughtLevel == "" && (opt.ID == ConfigOptionIDThoughtLevel || opt.Category == ConfigOptionCategoryThoughtLv) {
            snap.ThoughtLevel = resolveOptionDisplayValue(opt)
        }
    }
    return snap
}
```

- [ ] **Step 2: Add replay white-list helpers in `session.go`**

Add these helpers to `server/internal/hub/client/session.go`:

```go
type replayableConfigTarget struct {
    label    string
    id       string
    category string
    value    string
}

func replayableTargetsFromSnapshot(snap acp.SessionConfigSnapshot) []replayableConfigTarget {
    return []replayableConfigTarget{
        {label: "mode", id: acp.ConfigOptionIDMode, category: acp.ConfigOptionCategoryMode, value: strings.TrimSpace(snap.Mode)},
        {label: "model", id: acp.ConfigOptionIDModel, category: acp.ConfigOptionCategoryModel, value: strings.TrimSpace(snap.Model)},
        {label: "thought_level", id: acp.ConfigOptionIDThoughtLevel, category: acp.ConfigOptionCategoryThoughtLv, value: strings.TrimSpace(snap.ThoughtLevel)},
    }
}

func currentReplayableValue(label string, snap acp.SessionConfigSnapshot) string {
    switch label {
    case "mode":
        return strings.TrimSpace(snap.Mode)
    case "model":
        return strings.TrimSpace(snap.Model)
    case "thought_level":
        return strings.TrimSpace(snap.ThoughtLevel)
    default:
        return ""
    }
}
```

- [ ] **Step 3: Replace the `mode/model`-only reapply helper with a replayable white-list helper**

Replace `reapplyPersistedModeModel(...)` with:

```go
func applyReplayableConfigBaseline(ctx context.Context, projectName string, inst agent.Instance, sessionID string, current []acp.ConfigOption, desired acp.SessionConfigSnapshot) []acp.ConfigOption {
    options := append([]acp.ConfigOption(nil), current...)
    for _, target := range replayableTargetsFromSnapshot(desired) {
        if target.value == "" {
            continue
        }
        currentSnap := acp.SessionConfigSnapshotFromOptions(options)
        if strings.EqualFold(currentReplayableValue(target.label, currentSnap), target.value) {
            continue
        }
        configID := findConfigOptionID(options, target.id, target.category)
        if configID == "" {
            hubLogger(projectName).Warn("skip reapply %s: config option not found", target.label)
            continue
        }
        updated, err := inst.SessionSetConfigOption(ctx, acp.SessionSetConfigOptionParams{SessionID: sessionID, ConfigID: configID, Value: target.value})
        if err != nil {
            hubLogger(projectName).Warn("reapply %s failed session=%s value=%s err=%v", target.label, sessionID, target.value, err)
            continue
        }
        if len(updated) > 0 {
            options = append([]acp.ConfigOption(nil), updated...)
        }
    }
    return options
}
```

- [ ] **Step 4: Run the focused runtime tests and verify the next failure shifts to missing project-agent integration**

Run:

```bash
go test ./internal/hub/client -run "TestClientNewSession_ReappliesProjectAgentBaseline|TestEnsureReady_SessionLoadSuccess_ReplaysOnlyReplayableSessionValues" -count=1
```

Expected:
- `FAIL`
- failures now due to missing project baseline lookup or missing session-load merge behavior, not missing `thought_level` helper support

- [ ] **Step 5: Commit**

```bash
git add server/internal/protocol/acp.go server/internal/hub/client/session.go
git commit -m "feat(client): add replayable config white-list helpers"
```

---

### Task 4: Wire Project-Agent Baseline into `/new`, `session/load`, and Update Paths

**Files:**
- Modify: `server/internal/hub/client/session.go`
- Modify: `server/internal/hub/client/client.go`
- Modify: `server/internal/hub/client/commands.go`
- Modify: `server/internal/hub/client/client_runtime_test.go`

- [ ] **Step 1: Add project-agent baseline lookup and save helpers**

In `server/internal/hub/client/session.go`, add:

```go
func (s *Session) loadProjectAgentState(agentName string) ProjectAgentState {
    if s.store == nil || strings.TrimSpace(agentName) == "" {
        return ProjectAgentState{}
    }
    cfg, err := s.store.LoadProject(context.Background(), s.projectName)
    if err != nil || cfg == nil || cfg.AgentState == nil {
        return ProjectAgentState{}
    }
    return cfg.AgentState[strings.TrimSpace(agentName)]
}

func (s *Session) persistProjectAgentState(agentName string, configOptions []acp.ConfigOption, commands []acp.AvailableCommand) {
    if s.store == nil || strings.TrimSpace(agentName) == "" {
        return
    }
    cfg, err := s.store.LoadProject(context.Background(), s.projectName)
    if err != nil {
        hubLogger(s.projectName).Warn("load project baseline failed agent=%s err=%v", agentName, err)
        return
    }
    if cfg.AgentState == nil {
        cfg.AgentState = map[string]ProjectAgentState{}
    }
    next := cfg.AgentState[agentName]
    if configOptions != nil {
        next.ConfigOptions = append([]acp.ConfigOption(nil), configOptions...)
    }
    if commands != nil {
        next.AvailableCommands = append([]acp.AvailableCommand(nil), commands...)
    }
    next.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
    cfg.AgentState[agentName] = next
    if err := s.store.SaveProject(context.Background(), s.projectName, *cfg); err != nil {
        hubLogger(s.projectName).Warn("save project baseline failed agent=%s err=%v", agentName, err)
    }
}
```

- [ ] **Step 2: Apply project-agent baseline after fresh `session/new`**

In `ensureReady`, after successful `SessionNew`:

```go
baseline := s.loadProjectAgentState(agentName)
resolved := newResult.ConfigOptions
if len(baseline.ConfigOptions) > 0 {
    resolved = applyReplayableConfigBaseline(ctx, s.projectName, inst, newResult.SessionID, resolved, acp.SessionConfigSnapshotFromOptions(baseline.ConfigOptions))
}

s.mu.Lock()
state := s.agentStateLocked(agentName)
state.ACPSessionID = newResult.SessionID
state.ConfigOptions = append([]acp.ConfigOption(nil), resolved...)
// existing protocol/capability fields remain
s.mu.Unlock()
s.persistProjectAgentState(agentName, resolved, nil)
```

Update `ClientNewSession` / the `/new` path to rely on the same `ensureReady` logic instead of introducing a second custom replay branch.

- [ ] **Step 3: Apply session-local white-list overrides on successful `session/load`, then fill from project baseline only when missing**

In `ensureReady`, replace the current load-success block with:

```go
resolved := append([]acp.ConfigOption(nil), loadResult.ConfigOptions...)
savedSessionSnap := acp.SessionConfigSnapshotFromOptions(persistedConfigOptions)
if len(resolved) > 0 {
    resolved = applyReplayableConfigBaseline(ctx, s.projectName, inst, savedSID, resolved, savedSessionSnap)
} else if len(persistedConfigOptions) > 0 {
    resolved = append([]acp.ConfigOption(nil), persistedConfigOptions...)
}

baseline := s.loadProjectAgentState(agentName)
resolvedSnap := acp.SessionConfigSnapshotFromOptions(resolved)
missing := acp.SessionConfigSnapshot{}
if resolvedSnap.Mode == "" { missing.Mode = acp.SessionConfigSnapshotFromOptions(baseline.ConfigOptions).Mode }
if resolvedSnap.Model == "" { missing.Model = acp.SessionConfigSnapshotFromOptions(baseline.ConfigOptions).Model }
if resolvedSnap.ThoughtLevel == "" { missing.ThoughtLevel = acp.SessionConfigSnapshotFromOptions(baseline.ConfigOptions).ThoughtLevel }
if missing.Mode != "" || missing.Model != "" || missing.ThoughtLevel != "" {
    resolved = applyReplayableConfigBaseline(ctx, s.projectName, inst, savedSID, resolved, missing)
}
```

Then persist:

```go
state.ConfigOptions = append([]acp.ConfigOption(nil), resolved...)
s.persistProjectAgentState(agentName, resolved, state.Commands)
```

- [ ] **Step 4: Update config-command and session-update paths to refresh project baseline**

In `commands.go`, after successful `SessionSetConfigOption` and local session cache update, add:

```go
commands := []acp.AvailableCommand(nil)
s.mu.Lock()
if state := s.agentStateLocked(s.currentAgentNameLocked()); state != nil {
    commands = append(commands, state.Commands...)
    updatedOpts = append([]acp.ConfigOption(nil), state.ConfigOptions...)
}
agentName := s.currentAgentNameLocked()
s.mu.Unlock()
s.persistProjectAgentState(agentName, updatedOpts, commands)
```

In `SessionUpdate`, after handling `available_commands_update` and `config_option_update`, add:

```go
if changed {
    s.mu.Lock()
    agentName := s.currentAgentNameLocked()
    state := cloneSessionAgentState(s.agents[agentName])
    s.mu.Unlock()
    if state != nil {
        s.persistProjectAgentState(agentName, state.ConfigOptions, state.Commands)
    }
    s.persistSessionBestEffort()
}
```

- [ ] **Step 5: Add a test that `cancel` does not clear config state**

In `server/internal/hub/client/client_runtime_test.go`, add:

```go
func TestCancelPrompt_DoesNotClearSessionConfig(t *testing.T) {
    s := newSession("cancel-keep-config", "/tmp")
    s.ready = true
    s.acpSessionID = "acp-1"
    s.activeAgent = "claude"
    s.agents = map[string]*SessionAgentState{
        "claude": {
            ConfigOptions: []acp.ConfigOption{
                {ID: acp.ConfigOptionIDMode, Category: acp.ConfigOptionCategoryMode, CurrentValue: "code"},
                {ID: acp.ConfigOptionIDModel, Category: acp.ConfigOptionCategoryModel, CurrentValue: "gpt-5"},
                {ID: acp.ConfigOptionIDThoughtLevel, Category: acp.ConfigOptionCategoryThoughtLv, CurrentValue: "high"},
            },
        },
    }
    s.instance = &testInjectedInstance{name: "claude"}

    if err := s.cancelPrompt(); err != nil {
        t.Fatalf("cancelPrompt: %v", err)
    }

    snap := s.sessionConfigSnapshot()
    if snap.Mode != "code" || snap.Model != "gpt-5" || snap.ThoughtLevel != "high" {
        t.Fatalf("snapshot after cancel = %+v", snap)
    }
}
```

- [ ] **Step 6: Run the focused runtime suite and verify it passes**

Run:

```bash
go test ./internal/hub/client -run "TestClientNewSession_ReappliesProjectAgentBaseline|TestEnsureReady_SessionLoadSuccess_ReplaysOnlyReplayableSessionValues|TestCancelPrompt_DoesNotClearSessionConfig" -count=1
```

Expected:
- `PASS`

- [ ] **Step 7: Commit**

```bash
git add server/internal/hub/client/session.go server/internal/hub/client/client.go server/internal/hub/client/commands.go server/internal/hub/client/client_runtime_test.go
git commit -m "feat(client): unify project-agent config baseline restore"
```

---

### Task 5: Verify Commands Cache and Full Client Runtime Behavior

**Files:**
- Modify: `server/internal/hub/client/client_runtime_test.go`

- [ ] **Step 1: Add a runtime test that agent commands override cached commands on load**

Add:

```go
func TestEnsureReady_SessionLoadSuccess_AgentCommandsOverrideCachedCommands(t *testing.T) {
    s := newSession("commands-load", "/tmp")
    s.projectName = "proj1"
    s.activeAgent = "claude"
    s.acpSessionID = "acp-1"
    s.agents = map[string]*SessionAgentState{
        "claude": {
            ACPSessionID: "acp-1",
            Commands:     []acp.AvailableCommand{{Name: "/cached"}},
        },
    }

    inst := &testInjectedInstance{
        name:      "claude",
        sessionID: "acp-1",
        initResult: acp.InitializeResult{ProtocolVersion: "0.1", AgentCapabilities: acp.AgentCapabilities{LoadSession: true}},
        loadResult: acp.SessionLoadResult{ConfigOptions: []acp.ConfigOption{{ID: acp.ConfigOptionIDMode, Category: acp.ConfigOptionCategoryMode, CurrentValue: "code"}}},
    }

    s.registry = agent.DefaultACPFactory().Clone()
    s.registry.Register(acp.ACPProviderClaude, func(context.Context) (agent.Instance, error) { return inst, nil })

    if err := s.ensureInstance(context.Background()); err != nil {
        t.Fatalf("ensureInstance: %v", err)
    }
    s.SessionUpdate(acp.SessionUpdateParams{SessionID: "acp-1", Update: acp.SessionUpdate{SessionUpdate: acp.SessionUpdateAvailableCommandsUpdate, AvailableCommands: []acp.AvailableCommand{{Name: "/agent"}}}})

    state, _ := s.currentAgentStateSnapshot()
    if got := state.Commands[0].Name; got != "/agent" {
        t.Fatalf("command = %q, want /agent", got)
    }
}
```

- [ ] **Step 2: Run the full client test package**

Run:

```bash
go test ./internal/hub/client -count=1
```

Expected:
- all tests in `internal/hub/client` pass

- [ ] **Step 3: Commit**

```bash
git add server/internal/hub/client/client_runtime_test.go
git commit -m "test(client): verify command cache and baseline restore behavior"
```

---

### Task 6: Full Verification and Delivery

**Files:**
- Modify previous files only as needed to fix verification failures

- [ ] **Step 1: Run the full server test suite**

Run:

```bash
cd D:/Code/WheelMaker/server
go test ./... -count=1
```

Expected:
- all packages `ok`

- [ ] **Step 2: Check the final diff**

Run:

```bash
git status --short
git diff --stat
```

Expected:
- only the planned session-config files changed

- [ ] **Step 3: Final commit sequence**

Run:

```bash
git add -A
git commit -m "feat(client): unify project-agent session config restore"
git push origin main
```

Expected:
- commit created
- push succeeds

- [ ] **Step 4: Trigger the server updater because files under `server/` changed**

Run:

```bash
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/signal_update_now.ps1 -DelaySeconds 30
```

Expected:
- updater trigger accepted
