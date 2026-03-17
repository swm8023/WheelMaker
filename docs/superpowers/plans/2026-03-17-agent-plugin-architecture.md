# Agent Plugin Architecture Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace setter-based injection (`SetConfigOptionsValidator`, `SetPermissionHandler`) with a single `AgentPlugin` interface, fix the partial config-option update validator bug, and save config changes to disk immediately instead of waiting for prompt completion.

**Architecture:** `acp/plugin.go` defines the `AgentPlugin` interface and `DefaultPlugin` (zero-value implementation via embedding); each agent package overrides only the methods it needs; `acp.Agent` receives a plugin at construction time (`New`/`NewWithSessionID`) and on switch (`Switch`); `client.Client` calls `backend.Plugin()` instead of the old duck-typed `configOptionsValidatorProvider` interface; `handlePrompt` calls `saveAgentState` immediately on `UpdateConfigOption`.

**Tech Stack:** Go 1.21+; `go test ./...`; `git commit + push` after every task.

**Spec:** `docs/superpowers/specs/2026-03-17-agent-plugin-architecture-design.md`

---

## File Map

| Action | File | What changes |
|--------|------|--------------|
| Create | `internal/acp/plugin.go` | `AgentPlugin` interface, `DefaultPlugin`, `pluginOrDefault` |
| Modify | `internal/acp/agent.go` | replace `validator`/`permission` fields with `plugin`; update `New`/`NewWithSessionID`/`Switch` signatures; delete setter methods |
| Modify | `internal/acp/session.go` | `validateConfigOptions` → `a.plugin.ValidateConfigOptions`; add `NormalizeParams` call |
| Modify | `internal/acp/prompt.go` | add `NormalizeParams` call in subscribe callback |
| Modify | `internal/acp/callbacks.go` | `a.permission.RequestPermission` → `a.plugin.HandlePermission` |
| Delete | `internal/acp/config_options_validator.go` | interface + noop moved to plugin.go |
| Delete | `internal/acp/permission.go` | `PermissionHandler` + `AutoAllowHandler` moved to `DefaultPlugin` |
| Modify | `internal/acp/agent_test.go` | `acp.New(name, conn, dir)` → `acp.New(name, conn, dir, nil)` |
| Modify | `internal/acp/agent_config_options_test.go` | migrate `recordingConfigValidator` to `AgentPlugin`; remove `SetConfigOptionsValidator` |
| Modify | `internal/agent/factory.go` | add `Plugin() acp.AgentPlugin` to `Agent` interface |
| Create | `internal/agent/claude/plugin.go` | `claudePlugin` (partial-update-safe validator) |
| Modify | `internal/agent/claude/claude_agent.go` | add `Plugin()` method; remove `ConfigOptionsValidator()` |
| Delete | `internal/agent/claude/config_options_validator.go` | replaced by claude/plugin.go |
| Create | `internal/agent/codex/plugin.go` | `codexPlugin` (same pattern as claude) |
| Modify | `internal/agent/codex/codex_agent.go` | add `Plugin()` method; remove `ConfigOptionsValidator()` |
| Delete | `internal/agent/codex/config_options_validator.go` | replaced by codex/plugin.go |
| Modify | `internal/agent/mock/mock_agent.go` | add `Plugin()` method |
| Modify | `internal/client/client.go` | remove `configOptionsValidatorProvider`; pass `backend.Plugin()` to `acp.New`/`Switch`; call `saveAgentState` on `UpdateConfigOption` |
| Modify | `internal/client/client_test.go` | add `Plugin()` to three local agent stubs |

---

## Chunk 1: Define `AgentPlugin` interface

### Task 1: Create `internal/acp/plugin.go`

**Files:**
- Create: `internal/acp/plugin.go`

- [ ] **Step 1: Create the file**

```go
package acp

import (
	"context"
	"encoding/json"
)

// AgentPlugin is the per-agent customization hook for acp.Agent.
// Embed DefaultPlugin and override only the methods needed.
//
// Concurrency note: ValidateConfigOptions is called while configOptsMu is held.
// Implementations must NOT acquire acp.Agent's internal locks (mu, configOptsMu).
type AgentPlugin interface {
	// ValidateConfigOptions validates a config_option_update list.
	// Only validate fields that are present — partial updates are allowed.
	// Return non-nil to reject; the update is dropped (or connection fails on session/new).
	ValidateConfigOptions(opts []ConfigOption) error

	// HandlePermission responds to session/request_permission callbacks.
	// Signature matches the former PermissionHandler.RequestPermission.
	HandlePermission(ctx context.Context, params PermissionRequestParams) (PermissionResult, error)

	// NormalizeParams is called before acp processes each incoming session/update
	// notification. Translate legacy protocol fields to modern format here.
	// Return params unchanged for pass-through (default behaviour).
	NormalizeParams(method string, params json.RawMessage) json.RawMessage
}

// DefaultPlugin is the zero-value AgentPlugin. All methods are no-ops or auto-allow.
// Embed this in agent-specific plugins; add new extension points here without
// requiring all implementations to update.
type DefaultPlugin struct{}

// ValidateConfigOptions accepts all options (no validation).
func (DefaultPlugin) ValidateConfigOptions(_ []ConfigOption) error { return nil }

// HandlePermission auto-selects allow_once (matching former AutoAllowHandler behaviour).
func (DefaultPlugin) HandlePermission(_ context.Context, params PermissionRequestParams) (PermissionResult, error) {
	optionID := ""
	for _, opt := range params.Options {
		if opt.Kind == "allow_once" {
			optionID = opt.OptionID
			break
		}
	}
	if optionID == "" && len(params.Options) > 0 {
		optionID = params.Options[0].OptionID
	}
	return PermissionResult{Outcome: "selected", OptionID: optionID}, nil
}

// NormalizeParams passes notifications through unchanged.
func (DefaultPlugin) NormalizeParams(_ string, params json.RawMessage) json.RawMessage { return params }

// pluginOrDefault returns p if non-nil, otherwise DefaultPlugin{}.
func pluginOrDefault(p AgentPlugin) AgentPlugin {
	if p == nil {
		return DefaultPlugin{}
	}
	return p
}

// Compile-time interface check.
var _ AgentPlugin = DefaultPlugin{}
```

- [ ] **Step 2: Verify it compiles**

```bash
cd d:/Code/WheelMaker && go build ./internal/acp/...
```

Expected: no output (success). If `PermissionRequestParams` or `PermissionResult` are not found, they are in `internal/acp/protocol.go` — no action needed, they are already in scope within `package acp`.

- [ ] **Step 3: Commit and push**

```bash
git add internal/acp/plugin.go
git commit -m "feat(acp): add AgentPlugin interface and DefaultPlugin"
git push
```

---

## Chunk 2: Migrate `acp.Agent` core

### Task 2: Update test files to use new `acp.Agent` API (they will fail to compile until Task 3)

**Files:**
- Modify: `internal/acp/agent_config_options_test.go`
- Modify: `internal/acp/agent_test.go`

- [ ] **Step 1: Rewrite `internal/acp/agent_config_options_test.go`**

Replace the entire file with:

```go
package acp

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// recordingConfigValidator tracks concurrent ValidateConfigOptions calls.
// It embeds DefaultPlugin so it satisfies AgentPlugin without implementing
// HandlePermission or NormalizeParams.
type recordingConfigValidator struct {
	DefaultPlugin
	active int32
	max    int32
}

func (v *recordingConfigValidator) ValidateConfigOptions(_ []ConfigOption) error {
	n := atomic.AddInt32(&v.active, 1)
	for {
		m := atomic.LoadInt32(&v.max)
		if n <= m || atomic.CompareAndSwapInt32(&v.max, m, n) {
			break
		}
	}
	time.Sleep(20 * time.Millisecond)
	atomic.AddInt32(&v.active, -1)
	return nil
}

func TestSetConfigOptions_ValidatorCallsSerialized(t *testing.T) {
	v := &recordingConfigValidator{}
	ag := New("test", nil, ".", v) // v is passed as the plugin at construction

	opts := []ConfigOption{{ID: "model", Category: "model", CurrentValue: "gpt-4.1"}}
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)
	for i := 0; i < 2; i++ {
		go func() {
			defer wg.Done()
			<-start
			ag.setConfigOptions(opts)
		}()
	}
	close(start)
	wg.Wait()

	if got := atomic.LoadInt32(&v.max); got > 1 {
		t.Fatalf("validator calls overlapped (max=%d), want serialized updates", got)
	}
}
```

- [ ] **Step 2: Update `internal/acp/agent_test.go` — three types of call sites**

**2a. `acp.New` call (line 55):**
```go
// Before:
ag := acp.New(name, conn, t.TempDir())
// After:
ag := acp.New(name, conn, t.TempDir(), nil)
```

**2b. `acp.NewWithSessionID` calls (lines 199, 237, 365) — add `nil` as 5th argument:**
```go
// Before:
ag := acp.NewWithSessionID("test", conn, t.TempDir(), "seeded-session-id")
// After:
ag := acp.NewWithSessionID("test", conn, t.TempDir(), "seeded-session-id", nil)
```
Apply the same change at all three occurrences.

**2c. `ag.Switch` calls (lines 163, 286, 314, 538, 585) — add `nil` as 6th argument:**
```go
// Before:
ag.Switch(ctx, "test2", newConn, acp.SwitchWithContext, "")
// After:
ag.Switch(ctx, "test2", newConn, acp.SwitchWithContext, "", nil)
```
Apply the same change at all five occurrences (the 4th argument varies per call site, but always append `nil` at the end).

- [ ] **Step 3: Confirm compile fails (expected)**

```bash
cd d:/Code/WheelMaker && go build ./internal/acp/...
```

Expected: errors like `too many arguments in call to New` and `undefined: SetConfigOptionsValidator`. This is correct — the implementation change comes in Task 3.

---

### Task 3: Migrate `acp/agent.go` + update session/prompt/callbacks + delete obsolete files

**Files:**
- Modify: `internal/acp/agent.go`
- Modify: `internal/acp/session.go`
- Modify: `internal/acp/prompt.go`
- Modify: `internal/acp/callbacks.go`
- Delete: `internal/acp/config_options_validator.go`
- Delete: `internal/acp/permission.go`

- [ ] **Step 1: Update the `Agent` struct fields in `agent.go`**

In the `Agent` struct, replace:
```go
permission PermissionHandler // injectable; defaults to AutoAllowHandler
validator  ConfigOptionsValidator
```
with:
```go
plugin AgentPlugin
```

Also remove the `configOptsMu sync.Mutex` comment line that mentions "serializes config option updates from different sources" — keep the field but update the comment to: `configOptsMu sync.Mutex // serializes setConfigOptions calls`.

- [ ] **Step 2: Update `New` in `agent.go`**

```go
// New creates an Agent using an already-started *agent.Conn.
// plugin customizes per-agent behaviour; nil uses DefaultPlugin.
func New(name string, conn *agent.Conn, cwd string, plugin AgentPlugin) *Agent {
	ag := &Agent{
		name:            name,
		conn:            conn,
		cwd:             cwd,
		mcpServers:      []MCPServer{},
		plugin:          pluginOrDefault(plugin),
		terminals:       newTerminalManager(),
		activeToolCalls: make(map[string]struct{}),
	}
	ag.initCond = sync.NewCond(&ag.mu)
	return ag
}
```

- [ ] **Step 3: Update `NewWithSessionID` in `agent.go`**

```go
// NewWithSessionID creates an Agent with a pre-existing session ID to attempt session/load.
func NewWithSessionID(name string, conn *agent.Conn, cwd string, sessionID string, plugin AgentPlugin) *Agent {
	ag := New(name, conn, cwd, plugin)
	ag.sessionID = sessionID
	return ag
}
```

- [ ] **Step 4: Update `Switch` signature and body in `agent.go`**

Change the signature to add `plugin AgentPlugin` as the last parameter:

```go
func (a *Agent) Switch(ctx context.Context, name string, newConn *agent.Conn, mode SwitchMode, savedSessionID string, plugin AgentPlugin) error {
```

Inside the `a.mu.Lock()` block where other fields are reset (around the current line that sets `a.conn = newConn`, `a.name = name`, etc.), add:

```go
a.plugin = pluginOrDefault(plugin)
```

- [ ] **Step 5: Delete `SetConfigOptionsValidator`, `SetPermissionHandler`, `validateConfigOptions` methods from `agent.go`**

Remove these three methods entirely:
- `SetPermissionHandler` (around line 138)
- `SetConfigOptionsValidator` (around line 144)
- `validateConfigOptions` (around line 453)

- [ ] **Step 6: Update `setConfigOptions` in `agent.go`**

```go
func (a *Agent) setConfigOptions(opts []ConfigOption) {
	a.configOptsMu.Lock()
	defer a.configOptsMu.Unlock()
	if err := a.plugin.ValidateConfigOptions(opts); err != nil {
		log.Printf("agent: ignore invalid configOptions update: %v", err)
		return
	}
	a.mu.Lock()
	a.sessionMeta.ConfigOptions = opts
	a.mu.Unlock()
}
```

- [ ] **Step 7: Update `session.go` — session/load replay subscriber**

In the `conn.Subscribe` closure inside `ensureReady` (the session/load block), find the line:

```go
var p SessionUpdateParams
if err := json.Unmarshal(n.Params, &p); err != nil || p.SessionID != savedSessionID {
```

Change to:

```go
normalized := a.plugin.NormalizeParams(n.Method, n.Params)
var p SessionUpdateParams
if err := json.Unmarshal(normalized, &p); err != nil || p.SessionID != savedSessionID {
```

Also change the `validateConfigOptions` call at line 114:

```go
// Before:
if err := a.validateConfigOptions(p.Update.ConfigOptions); err == nil {
// After:
if err := a.plugin.ValidateConfigOptions(p.Update.ConfigOptions); err == nil {
```

And change the session/new validation at line 164:

```go
// Before:
if err := a.validateConfigOptions(newResult.ConfigOptions); err != nil {
// After:
if err := a.plugin.ValidateConfigOptions(newResult.ConfigOptions); err != nil {
```

- [ ] **Step 8: Update `prompt.go` — Prompt subscribe callback**

In the `conn.Subscribe` closure, find:

```go
var p SessionUpdateParams
if err := json.Unmarshal(n.Params, &p); err != nil {
    return
}
```

Change to:

```go
normalized := a.plugin.NormalizeParams(n.Method, n.Params)
var p SessionUpdateParams
if err := json.Unmarshal(normalized, &p); err != nil {
    return
}
```

- [ ] **Step 9: Update `callbacks.go` — permission callback**

In `callbackPermission`, change:

```go
// Before:
a.mu.Lock()
h := a.permission
pCtx := a.promptCtx
a.mu.Unlock()
...
result, err := h.RequestPermission(pCtx, p)
// After:
a.mu.Lock()
h := a.plugin
pCtx := a.promptCtx
a.mu.Unlock()
...
result, err := h.HandlePermission(pCtx, p)
```

- [ ] **Step 10: Delete obsolete files**

```bash
cd d:/Code/WheelMaker
rm internal/acp/config_options_validator.go
rm internal/acp/permission.go
```

- [ ] **Step 11: Run tests**

```bash
cd d:/Code/WheelMaker && go test ./internal/acp/...
```

Expected: all tests PASS. If any test references `AutoAllowHandler` or `noopConfigOptionsValidator`, it needs the same fix: replace with `DefaultPlugin{}` or `nil`.

- [ ] **Step 12: Commit and push**

```bash
git add internal/acp/
git commit -m "feat(acp): migrate Agent to AgentPlugin; remove SetConfigOptionsValidator/SetPermissionHandler"
git push
```

---

## Chunk 3: `agent.Agent` interface + per-agent implementations

### Task 4: Add `Plugin()` to `agent.Agent` interface

**Files:**
- Modify: `internal/agent/factory.go`

- [ ] **Step 1: Add `Plugin()` to the interface**

```go
// Agent is a stateless connection factory for an ACP-compatible CLI backend.
type Agent interface {
	// Name returns the identifier for this agent (e.g. "claude").
	Name() string

	// Connect starts a new subprocess and returns an initialized *Conn.
	Connect(ctx context.Context) (*Conn, error)

	// Close cleans up any resources held by the agent.
	Close() error

	// Plugin returns the per-agent customization plugin for acp.Agent.
	Plugin() acp.AgentPlugin
}
```

Add the import `acp "github.com/swm8023/wheelmaker/internal/acp"` to `factory.go`.

- [ ] **Step 2: Verify compile fails on implementations (expected)**

```bash
cd d:/Code/WheelMaker && go build ./internal/agent/...
```

Expected: errors like `*ClaudeAgent does not implement agent.Agent (missing Plugin method)`. This is correct — the implementations come next.

---

### Task 5: claude plugin implementation

**Files:**
- Create: `internal/agent/claude/plugin.go`
- Modify: `internal/agent/claude/claude_agent.go`
- Delete: `internal/agent/claude/config_options_validator.go`

- [ ] **Step 1: Create `internal/agent/claude/plugin.go`**

```go
package claude

import (
	"fmt"

	acp "github.com/swm8023/wheelmaker/internal/acp"
)

// claudePlugin implements acp.AgentPlugin for Claude Code CLI.
// It embeds DefaultPlugin so HandlePermission and NormalizeParams use defaults.
type claudePlugin struct{ acp.DefaultPlugin }

// ValidateConfigOptions validates only the options present in the update.
// Partial updates (mode-only or model-only) are explicitly allowed.
// An empty CurrentValue is rejected because Claude requires a non-empty value
// for any mode/model option that is present.
func (claudePlugin) ValidateConfigOptions(opts []acp.ConfigOption) error {
	for _, opt := range opts {
		if (opt.ID == "mode" || opt.Category == "mode") && opt.CurrentValue == "" {
			return fmt.Errorf("claude: mode currentValue is empty")
		}
		if (opt.ID == "model" || opt.Category == "model") && opt.CurrentValue == "" {
			return fmt.Errorf("claude: model currentValue is empty")
		}
	}
	return nil
}

// Compile-time check.
var _ acp.AgentPlugin = claudePlugin{}
```

- [ ] **Step 2: Add `Plugin()` to `claude_agent.go` and remove `ConfigOptionsValidator()`**

In `internal/agent/claude/claude_agent.go`, replace:

```go
// ConfigOptionsValidator returns the expected config option validator for Claude.
func (a *ClaudeAgent) ConfigOptionsValidator() acp.ConfigOptionsValidator {
	return claudeConfigOptionsValidator{}
}
```

with:

```go
// Plugin returns the Claude-specific AgentPlugin.
func (a *ClaudeAgent) Plugin() acp.AgentPlugin { return claudePlugin{} }
```

- [ ] **Step 3: Delete obsolete file**

```bash
cd d:/Code/WheelMaker && rm internal/agent/claude/config_options_validator.go
```

- [ ] **Step 4: Run claude package tests**

```bash
cd d:/Code/WheelMaker && go test ./internal/agent/claude/...
```

Expected: PASS.

---

### Task 6: codex plugin implementation

**Files:**
- Create: `internal/agent/codex/plugin.go`
- Modify: `internal/agent/codex/codex_agent.go`
- Delete: `internal/agent/codex/config_options_validator.go`

- [ ] **Step 1: Create `internal/agent/codex/plugin.go`**

```go
package codex

import (
	"fmt"

	acp "github.com/swm8023/wheelmaker/internal/acp"
)

// codexPlugin implements acp.AgentPlugin for the Codex CLI.
type codexPlugin struct{ acp.DefaultPlugin }

// ValidateConfigOptions validates only the options present in the update.
// Partial updates are explicitly allowed.
func (codexPlugin) ValidateConfigOptions(opts []acp.ConfigOption) error {
	for _, opt := range opts {
		if (opt.ID == "mode" || opt.Category == "mode") && opt.CurrentValue == "" {
			return fmt.Errorf("codex: mode currentValue is empty")
		}
		if (opt.ID == "model" || opt.Category == "model") && opt.CurrentValue == "" {
			return fmt.Errorf("codex: model currentValue is empty")
		}
	}
	return nil
}

// Compile-time check.
var _ acp.AgentPlugin = codexPlugin{}
```

- [ ] **Step 2: Add `Plugin()` to `codex_agent.go` and remove `ConfigOptionsValidator()`**

In `internal/agent/codex/codex_agent.go`, replace:

```go
// ConfigOptionsValidator returns the expected config option validator for Codex.
func (a *CodexAgent) ConfigOptionsValidator() acp.ConfigOptionsValidator {
	return codexConfigOptionsValidator{}
}
```

with:

```go
// Plugin returns the Codex-specific AgentPlugin.
func (a *CodexAgent) Plugin() acp.AgentPlugin { return codexPlugin{} }
```

- [ ] **Step 3: Delete obsolete file**

```bash
cd d:/Code/WheelMaker && rm internal/agent/codex/config_options_validator.go
```

- [ ] **Step 4: Run codex package tests**

```bash
cd d:/Code/WheelMaker && go test ./internal/agent/codex/...
```

Expected: PASS.

---

### Task 7: mock agent `Plugin()` method

**Files:**
- Modify: `internal/agent/mock/mock_agent.go`

- [ ] **Step 1: Add import and `Plugin()` method to `mock_agent.go`**

Add to imports:
```go
acp "github.com/swm8023/wheelmaker/internal/acp"
```

Add method after `Close()`:
```go
// Plugin returns the default AgentPlugin (no customization needed for tests).
func (a *MockAgent) Plugin() acp.AgentPlugin { return acp.DefaultPlugin{} }
```

- [ ] **Step 2: Add compile-time check**

At the bottom of `mock_agent.go` (or alongside the existing type), add or verify:
```go
var _ agent.Agent = (*MockAgent)(nil)
```

- [ ] **Step 3: Run agent package tests**

```bash
cd d:/Code/WheelMaker && go test ./internal/agent/...
```

Expected: PASS.

- [ ] **Step 4: Commit and push**

```bash
git add internal/agent/
git commit -m "feat(agent): add Plugin() to factory interface; implement claude/codex/mock plugins"
git push
```

---

## Chunk 4: `client/client.go` fixes

### Task 8: Update `client.go` — remove old plumbing, fix persistence

**Files:**
- Modify: `internal/client/client.go`

- [ ] **Step 1: Delete `configOptionsValidatorProvider` interface**

Remove these lines from `client.go` (around line 26):

```go
type configOptionsValidatorProvider interface {
	ConfigOptionsValidator() acp.ConfigOptionsValidator
}
```

- [ ] **Step 2: Update `ensureAgent` — replace `SetConfigOptionsValidator` with `Plugin()`**

In `ensureAgent` (around line 487), replace:

```go
if vp, ok := backend.(configOptionsValidatorProvider); ok {
    ag.SetConfigOptionsValidator(vp.ConfigOptionsValidator())
}
```

with the plugin being passed directly to `acp.New`/`acp.NewWithSessionID`:

```go
plugin := backend.Plugin()
var ag *acp.Agent
if savedSID != "" {
    ag = acp.NewWithSessionID(name, conn, c.cwd, savedSID, plugin)
} else {
    ag = acp.New(name, conn, c.cwd, plugin)
}
```

(Remove the old `var ag *acp.Agent` declaration and the two `acp.New`/`acp.NewWithSessionID` calls that precede the validator block.)

- [ ] **Step 3: Update `switchAgent` — pass plugin to `Switch` and new-agent creation**

In `switchAgent`, the `ag.Switch(...)` call (around line 606):

```go
// Before:
if err := ag.Switch(ctx, name, newConn, mode, savedSID); err != nil {
    return fmt.Errorf("switch %q: %w", name, err)
}
if vp, ok := newBackend.(configOptionsValidatorProvider); ok {
    ag.SetConfigOptionsValidator(vp.ConfigOptionsValidator())
} else {
    ag.SetConfigOptionsValidator(nil)
}
// After:
if err := ag.Switch(ctx, name, newConn, mode, savedSID, newBackend.Plugin()); err != nil {
    return fmt.Errorf("switch %q: %w", name, err)
}
```

In the `else` branch (nil-ag path, around line 619):

```go
// Before:
var newAg *acp.Agent
if savedSID != "" {
    newAg = acp.NewWithSessionID(name, newConn, c.cwd, savedSID)
} else {
    newAg = acp.New(name, newConn, c.cwd)
}
if vp, ok := newBackend.(configOptionsValidatorProvider); ok {
    newAg.SetConfigOptionsValidator(vp.ConfigOptionsValidator())
}
// After:
plugin := newBackend.Plugin()
var newAg *acp.Agent
if savedSID != "" {
    newAg = acp.NewWithSessionID(name, newConn, c.cwd, savedSID, plugin)
} else {
    newAg = acp.New(name, newConn, c.cwd, plugin)
}
```

- [ ] **Step 4: Fix persistence timing in `handlePrompt`**

In `handlePrompt`, find (around line 428):

```go
if u.Type == acp.UpdateConfigOption {
    c.reply(msg.ChatID, formatConfigOptionUpdateMessage(u.Raw))
}
```

Change to:

```go
if u.Type == acp.UpdateConfigOption {
    c.reply(msg.ChatID, formatConfigOptionUpdateMessage(u.Raw))
    c.saveAgentState(ag) // persist immediately; don't wait for prompt to finish
}
```

- [ ] **Step 5: Run client package tests**

```bash
cd d:/Code/WheelMaker && go test ./internal/client/...
```

Expected: compile errors on the three local agent stubs in `client_test.go` (missing `Plugin()` method). Fix in Task 9.

---

### Task 9: Fix `client_test.go` local agent stubs

**Files:**
- Modify: `internal/client/client_test.go`

- [ ] **Step 1: Add `Plugin()` to the three local stubs**

Find `minimalMockAgent`, `contextRejectMockAgent`, and `failConnectAgent` in `client_test.go` (around lines 605–646). Add a `Plugin()` method to each:

```go
func (a *minimalMockAgent)       Plugin() acp.AgentPlugin { return acp.DefaultPlugin{} }
func (a *contextRejectMockAgent) Plugin() acp.AgentPlugin { return acp.DefaultPlugin{} }
func (a *failConnectAgent)       Plugin() acp.AgentPlugin { return acp.DefaultPlugin{} }
```

`client_test.go` already imports `acp` (used elsewhere in the file). If not, add:

```go
acp "github.com/swm8023/wheelmaker/internal/acp"
```

- [ ] **Step 2: Check for any `Switch` calls in test files**

```bash
cd d:/Code/WheelMaker && grep -rn "\.Switch(" internal/
```

If any test calls `ag.Switch(...)` directly with 5 arguments, add the 6th `nil` argument (which resolves to `DefaultPlugin{}` via `pluginOrDefault`).

- [ ] **Step 3: Run all tests**

```bash
cd d:/Code/WheelMaker && go test ./...
```

Expected: all tests PASS. If any test still fails:
- Compile error about `acp.New` arity: find remaining 3-arg `acp.New` calls, add `nil` as 4th arg.
- Compile error about `acp.NewWithSessionID` arity: same pattern.
- Compile error about `Switch` arity: add `nil` as 6th arg.

- [ ] **Step 4: Commit and push**

```bash
git add internal/client/
git commit -m "feat(client): use AgentPlugin; save config options immediately on update"
git push
```

---

## Final Verification

- [ ] **Run full test suite**

```bash
cd d:/Code/WheelMaker && go test ./...
```

Expected: all tests PASS, zero compile errors.

- [ ] **Verify removed types/methods are gone**

```bash
cd d:/Code/WheelMaker
grep -rn "SetConfigOptionsValidator\|SetPermissionHandler\|configOptionsValidatorProvider\|AutoAllowHandler\|noopConfigOptionsValidator" internal/
```

Expected: no output. If any matches remain, remove them.

- [ ] **Verify plugin chain is wired**

```bash
cd d:/Code/WheelMaker
grep -rn "\.Plugin()" internal/
```

Expected: hits in `ensureAgent`, `switchAgent` (two places) in `client.go`, and the compile-time checks in `plugin.go`, `claude/plugin.go`, `codex/plugin.go`.
