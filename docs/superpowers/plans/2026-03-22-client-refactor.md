# internal/client Refactor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Split `client.go` (1017 lines) into three focused files and apply 7 targeted detail fixes, with zero behavioral changes.

**Architecture:** Two-commit approach — Step 1 is a pure mechanical move (verifiable by inspection), Step 2 applies all detail fixes. Both steps must keep `go test -race ./internal/client/...` green throughout.

**Tech Stack:** Go, `sync.Mutex`, `sync.Cond`, JSON-RPC 2.0 (ACP protocol), `internal/acp`, `internal/im`, `internal/agent`

---

## File Map

| File | Action | After refactor |
|------|--------|----------------|
| `internal/client/client.go` | Modify (remove moved funcs, add constants) | Client struct, New/Start/Run/Close, HandleMessage, parseCommand, handlePrompt, reply/replyDebug, renderUnknown, `defaultAgentName`, `acpClientProtocolVersion`, `acpClientInfo` |
| `internal/client/commands.go` | **Create** | handleCommand, handleConfigCommand, resolveModeArg, resolveModelArg, resolveConfigSelectArg, formatConfigOptionUpdateMessage, listSessions, createNewSession, loadSessionByIndex, persistSessionSummaries, resolveHelpModel, firstNonEmpty |
| `internal/client/lifecycle.go` | **Create** | ensureForwarder, switchAgent, registeredAgentNames |
| `internal/client/session.go` | Modify (add comment + resetSessionFields, promote constants) | All existing functions unchanged except: `persistMeta` gets invariant comment, `resetSessionFields` extracted, `ensureReady` uses package-level constants |
| All other files | Unchanged | `callbacks.go`, `state.go`, `store.go`, `permission.go`, `terminal.go`, `debug.go` |

---

## Task 1: Structural Split — Create `commands.go`

**Files:**
- Modify: `internal/client/client.go`
- Create: `internal/client/commands.go`

- [ ] **Step 1: Create `commands.go` with the package declaration and move these functions verbatim**

Move the following from `client.go` into `internal/client/commands.go`. Copy them exactly — no logic changes:
- `handleCommand`
- `handleConfigCommand`
- `resolveModeArg`
- `resolveModelArg`
- `resolveConfigSelectArg`
- `formatConfigOptionUpdateMessage`
- `listSessions`
- `createNewSession`
- `loadSessionByIndex`
- `persistSessionSummaries`
- `resolveHelpModel`
- `firstNonEmpty`

`commands.go` needs these imports (take from client.go's import block):
```go
package client

import (
    "context"
    "encoding/json"
    "errors"
    "fmt"
    "slices"
    "strconv"
    "strings"

    acp "github.com/swm8023/wheelmaker/internal/acp"
    "github.com/swm8023/wheelmaker/internal/im"
)
```

- [ ] **Step 2: Delete those same functions from `client.go`**

After moving, remove the 12 functions from `client.go`. The import block in `client.go` will shrink — remove `encoding/json`, `slices`, `strconv` if they are no longer used there. Keep `errors`, `fmt`, `io`, `log`, `strings`, `sync`, `time`, and both internal imports.

- [ ] **Step 3: Verify the build compiles**

```bash
cd d:/Code/WheelMaker && go build ./internal/client/...
```
Expected: no errors.

- [ ] **Step 4: Run tests**

```bash
go test -race ./internal/client/... -v 2>&1 | tail -20
```
Expected: all tests PASS, no race detected.

- [ ] **Step 5: Commit**

```bash
git add internal/client/commands.go internal/client/client.go
git commit -m "refactor(client): extract commands.go (mechanical move, no logic change)"
```

---

## Task 2: Structural Split — Create `lifecycle.go`

**Files:**
- Modify: `internal/client/client.go`
- Create: `internal/client/lifecycle.go`

- [ ] **Step 1: Create `lifecycle.go` with these functions moved verbatim from `client.go`**

Move:
- `ensureForwarder`
- `switchAgent`
- `registeredAgentNames`

```go
package client

import (
    "context"
    "fmt"
    "log"

    acp "github.com/swm8023/wheelmaker/internal/acp"
)
```

- [ ] **Step 2: Delete those three functions from `client.go`**

Remove `ensureForwarder`, `switchAgent`, `registeredAgentNames` from `client.go`. Trim the `client.go` import block: remove `log` if no longer used there (check — `log` is used in `session.go` not `client.go`).

- [ ] **Step 3: Verify build and tests**

```bash
go build ./internal/client/... && go test -race ./internal/client/... -v 2>&1 | tail -20
```
Expected: builds clean, all tests PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/client/lifecycle.go internal/client/client.go
git commit -m "refactor(client): extract lifecycle.go (mechanical move, no logic change)"
```

---

## Task 3: Fix 1 + Fix 3 — Package-level constants

**Files:**
- Modify: `internal/client/client.go`
- Modify: `internal/client/session.go`
- Modify: `internal/client/state.go`

Fix 1: `defaultAgentName` constant.
Fix 3: `acpClientProtocolVersion` and `acpClientInfo` promoted from local-to-ensureReady to package-level.

- [ ] **Step 1: Add constants to `client.go`**

In `client.go`, after the import block and before the `Client` struct, add:

```go
const defaultAgentName = "claude"

const acpClientProtocolVersion = 1

var acpClientInfo = &acp.AgentInfo{Name: "wheelmaker", Version: "0.1"}
```

- [ ] **Step 2: Replace all `"claude"` literals with `defaultAgentName`**

Two locations to update:

In `lifecycle.go`, `ensureForwarder`:
```go
// Before:
name := c.state.ActiveAgent
if name == "" {
    name = "claude"
}
// After:
name := c.state.ActiveAgent
if name == "" {
    name = defaultAgentName
}
```

In `state.go`, `defaultProjectState()`:
```go
// Before:
return &ProjectState{
    ActiveAgent: "claude",
    ...
}
// After:
return &ProjectState{
    ActiveAgent: defaultAgentName,
    ...
}
```

Also check `debug.go` `resolveCurrentAgentName()` — it has `return "claude"` as final fallback:
```go
// Before:
return "claude"
// After:
return defaultAgentName
```

- [ ] **Step 3: Replace local constants in `ensureReady` with package-level**

In `session.go`, `ensureReady`, find and remove:
```go
clientInfo := &acp.AgentInfo{Name: "wheelmaker", Version: "0.1"}
const clientProtocolVersion = 1
```

Replace all uses in the function:
- `clientProtocolVersion` → `acpClientProtocolVersion`
- `clientInfo` → `acpClientInfo`

The `InitializeParams` call becomes:
```go
initResult, err := fwd.Initialize(ctx, acp.InitializeParams{
    ProtocolVersion:    acpClientProtocolVersion,
    ClientCapabilities: clientCaps,
    ClientInfo:         acpClientInfo,
})
```

The `newInitMeta` block becomes:
```go
newInitMeta := clientInitMeta{
    ProtocolVersion:       initResult.ProtocolVersion.String(),
    AgentCapabilities:     initResult.AgentCapabilities,
    AgentInfo:             initResult.AgentInfo,
    AuthMethods:           initResult.AuthMethods,
    ClientProtocolVersion: acpClientProtocolVersion,
    ClientCapabilities:    clientCaps,
    ClientInfo:            acpClientInfo,
}
```

- [ ] **Step 4: Build and test**

```bash
go build ./internal/client/... && go test -race ./internal/client/... -v 2>&1 | tail -20
```
Expected: builds clean, all tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/client/client.go internal/client/session.go internal/client/state.go internal/client/debug.go
git commit -m "refactor(client): promote defaultAgentName and ACP client identity to package-level constants"
```

---

## Task 4: Fix 2 — `emptyMCPServers` helper

**Files:**
- Modify: `internal/client/session.go`
- Modify: `internal/client/commands.go`

- [ ] **Step 1: Add helper to `session.go`**

Add near the top of `session.go` (below imports, above `Session` interface):

```go
// emptyMCPServers returns an empty MCP server list for session/new and session/load calls.
// Replace this helper when MCP config support is added.
func emptyMCPServers() []acp.MCPServer {
    return []acp.MCPServer{}
}
```

- [ ] **Step 2: Replace all four `[]acp.MCPServer{}` literals**

There are 4 locations — all inside session params:

**In `session.go`, `ensureReady`, session/load path (line ~151):**
```go
// Before:
_, err := fwd.SessionLoad(ctx, acp.SessionLoadParams{
    SessionID:  savedSID,
    CWD:        cwd,
    MCPServers: []acp.MCPServer{},
})
// After:
_, err := fwd.SessionLoad(ctx, acp.SessionLoadParams{
    SessionID:  savedSID,
    CWD:        cwd,
    MCPServers: emptyMCPServers(),
})
```

**In `session.go`, `ensureReady`, session/new path (line ~183):**
```go
// Before:
newResult, err := fwd.SessionNew(ctx, acp.SessionNewParams{
    CWD:        cwd,
    MCPServers: []acp.MCPServer{},
})
// After:
newResult, err := fwd.SessionNew(ctx, acp.SessionNewParams{
    CWD:        cwd,
    MCPServers: emptyMCPServers(),
})
```

**In `commands.go`, `createNewSession`:**
```go
// Before:
res, err := fwd.SessionNew(ctx, acp.SessionNewParams{
    CWD:        cwd,
    MCPServers: []acp.MCPServer{},
})
// After:
res, err := fwd.SessionNew(ctx, acp.SessionNewParams{
    CWD:        cwd,
    MCPServers: emptyMCPServers(),
})
```

**In `commands.go`, `loadSessionByIndex`:**
```go
// Before:
_, err = fwd.SessionLoad(ctx, acp.SessionLoadParams{
    SessionID:  target,
    CWD:        cwd,
    MCPServers: []acp.MCPServer{},
})
// After:
_, err = fwd.SessionLoad(ctx, acp.SessionLoadParams{
    SessionID:  target,
    CWD:        cwd,
    MCPServers: emptyMCPServers(),
})
```

- [ ] **Step 3: Grep to confirm no literals remain**

```bash
grep -rn '\[\]acp\.MCPServer{}' internal/client/
```
Expected: no output (zero matches).

- [ ] **Step 4: Build and test**

```bash
go build ./internal/client/... && go test -race ./internal/client/... -v 2>&1 | tail -20
```

- [ ] **Step 5: Commit**

```bash
git add internal/client/session.go internal/client/commands.go
git commit -m "refactor(client): replace inline MCPServers literals with emptyMCPServers() helper"
```

---

## Task 5: Fix 4 — `persistMeta` invariant comment

**Files:**
- Modify: `internal/client/session.go`

- [ ] **Step 1: Add the invariant comment to `persistMeta`**

Replace the existing doc comment on `persistMeta`:
```go
// Before:
// persistMeta snapshots current session metadata into in-memory state.
// Returns true if anything changed. Must be called while NOT holding c.mu.

// After:
// persistMeta snapshots current session metadata into in-memory state and
// returns true if anything changed. Must be called while NOT holding c.mu.
//
// Concurrency safety: this function acquires c.mu twice (once to read, once
// to write). This looks like a TOCTOU window but is safe in practice because
// every caller is serialized by promptMu:
//   - saveSessionState is called only from handlePrompt, ensureReadyAndNotify,
//     switchAgent, createNewSession, and loadSessionByIndex — all under promptMu.
//   - Close is a known exception: it calls saveSessionState during shutdown
//     without promptMu, which is acceptable because Close is not concurrent
//     with prompt operations by contract.
//
// c.store.Save (file I/O) is intentionally kept outside c.mu to avoid
// stalling ACP callback goroutines during disk writes.
```

- [ ] **Step 2: Build and test**

```bash
go build ./internal/client/... && go test -race ./internal/client/... -v 2>&1 | tail -20
```

- [ ] **Step 3: Commit**

```bash
git add internal/client/session.go
git commit -m "docs(client): document persistMeta promptMu serialization invariant"
```

---

## Task 6: Fix 5 — Extract `resetSessionFields`

**Files:**
- Modify: `internal/client/session.go`
- Modify: `internal/client/commands.go`
- Modify: `internal/client/lifecycle.go`

- [ ] **Step 1: Add `resetSessionFields` to `session.go`**

Add this method near `persistMeta` / `saveSessionState`:

```go
// resetSessionFields resets the 6 session-level fields common to session/new,
// session/load, and agent-switch operations. Callers MUST hold c.mu.
func (c *Client) resetSessionFields(sid string, configOpts []acp.ConfigOption) {
    c.sessionID = sid
    c.ready = true
    c.lastReply = ""
    c.loadHistory = nil
    c.activeToolCalls = make(map[string]struct{})
    c.sessionMeta = clientSessionMeta{ConfigOptions: configOpts}
}
```

- [ ] **Step 2: Update `createNewSession` in `commands.go`**

Find the block in `createNewSession` after `fwd.SessionNew` succeeds:
```go
// Before:
c.mu.Lock()
c.sessionID = res.SessionID
c.ready = true
c.lastReply = ""
c.loadHistory = nil
c.activeToolCalls = make(map[string]struct{})
c.sessionMeta = clientSessionMeta{
    ConfigOptions: res.ConfigOptions,
}
c.mu.Unlock()

// After:
c.mu.Lock()
c.resetSessionFields(res.SessionID, res.ConfigOptions)
c.mu.Unlock()
```

- [ ] **Step 3: Update `loadSessionByIndex` in `commands.go`**

Find the block after `fwd.SessionLoad` succeeds:
```go
// Before:
c.mu.Lock()
c.sessionID = target
c.ready = true
c.lastReply = ""
c.loadHistory = nil
c.activeToolCalls = make(map[string]struct{})
c.sessionMeta = clientSessionMeta{}
c.mu.Unlock()

// After:
c.mu.Lock()
c.resetSessionFields(target, nil)
c.mu.Unlock()
```

- [ ] **Step 4: Update `switchAgent` in `lifecycle.go`**

`switchAgent` resets 10 fields plus calls `c.initCond.Broadcast()`. Only the 6 common fields are extracted; the agent-switch-specific fields (`initializing`, `promptUpdatesCh`, `initMeta`, `currentAgent`, `currentAgentName`, `forwarder`) stay in place.

Find the large reset block in `switchAgent` (inside `c.mu.Lock()` / `c.mu.Unlock()`):
```go
// Before:
c.mu.Lock()
c.terminals.KillAll()
c.forwarder = newFwd
c.currentAgent = baseAgent
c.currentAgentName = name
c.ready = false
c.initializing = false
c.sessionID = savedSID
c.lastReply = ""
c.loadHistory = nil
c.activeToolCalls = make(map[string]struct{})
c.promptUpdatesCh = nil
c.initMeta = clientInitMeta{}
c.sessionMeta = clientSessionMeta{}
c.mu.Unlock()

// After (resetSessionFields called with ready=true is wrong here — switchAgent
// sets ready=false to force re-initialization, so we call resetSessionFields
// then override ready):
c.mu.Lock()
c.terminals.KillAll()
c.forwarder = newFwd
c.currentAgent = baseAgent
c.currentAgentName = name
c.initializing = false
c.promptUpdatesCh = nil
c.initMeta = clientInitMeta{}
c.resetSessionFields(savedSID, nil) // sets ready=true — override next:
c.ready = false
c.mu.Unlock()
c.initCond.Broadcast() // wake any goroutine blocked in ensureReady's Wait loop
```

> **Note:** `resetSessionFields` sets `ready = true`, but `switchAgent` needs `ready = false` to force a new `ensureReady` handshake. Override `ready` immediately after the call within the same lock section. `c.initCond.Broadcast()` **must** be called after `c.mu.Unlock()` — never inside the lock.

- [ ] **Step 5: Build and test**

```bash
go build ./internal/client/... && go test -race ./internal/client/... -v 2>&1 | tail -20
```
Expected: all tests PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/client/session.go internal/client/commands.go internal/client/lifecycle.go
git commit -m "refactor(client): extract resetSessionFields to eliminate 6-field reset duplication"
```

---

## Task 7: Fix 6 — Remove dead double-unmarshal in `formatConfigOptionUpdateMessage`

**Files:**
- Modify: `internal/client/commands.go`

- [ ] **Step 1: Simplify `formatConfigOptionUpdateMessage`**

The current function tries unmarshaling into `acp.SessionUpdate` first, then `acp.SessionUpdateParams`. The second branch is unreachable because the `Raw` bytes always come from `json.Marshal(u)` in `sessionUpdateToUpdate` where `u` is `acp.SessionUpdate`.

Replace the body:
```go
func formatConfigOptionUpdateMessage(raw []byte) string {
    if len(raw) == 0 {
        return "Config options updated."
    }
    var u acp.SessionUpdate
    var opts []acp.ConfigOption
    if err := json.Unmarshal(raw, &u); err == nil {
        opts = u.ConfigOptions
    }
    if len(opts) == 0 {
        return "Config options updated."
    }
    mode := ""
    model := ""
    for _, opt := range opts {
        if mode == "" && (opt.ID == "mode" || strings.EqualFold(opt.Category, "mode")) {
            mode = strings.TrimSpace(opt.CurrentValue)
        }
        if model == "" && (opt.ID == "model" || strings.EqualFold(opt.Category, "model")) {
            model = strings.TrimSpace(opt.CurrentValue)
        }
    }
    if mode == "" && model == "" {
        return "Config options updated."
    }
    return fmt.Sprintf("Config options updated: mode=%s model=%s", renderUnknown(mode), renderUnknown(model))
}
```

- [ ] **Step 2: Verify `acp.SessionUpdateParams` import is still needed in `commands.go`**

After removing the second unmarshal, check if `acp.SessionUpdateParams` is referenced elsewhere in `commands.go`. If not, it's fine — Go will error if unused imports remain.

```bash
go build ./internal/client/...
```
Fix any "imported and not used" errors by removing unused imports.

- [ ] **Step 3: Test**

```bash
go test -race ./internal/client/... -v 2>&1 | tail -20
```

- [ ] **Step 4: Commit**

```bash
git add internal/client/commands.go
git commit -m "refactor(client): remove dead second-unmarshal branch in formatConfigOptionUpdateMessage"
```

---

## Task 8: Fix 7 — Remove redundant nil-check in `handleConfigCommand`

**Files:**
- Modify: `internal/client/commands.go`

- [ ] **Step 1: Rewrite `handleConfigCommand` with the corrected lock structure**

The original has two `c.mu` sections after `ensureForwarder`: one reads `agentName`/`sessionState`, the other reads `fwd`/`sid` and checks `if fwd == nil`. The fix removes the `fwd == nil` guard (unreachable after successful `ensureForwarder`) and reads `fwd`/`sid` **after** `ensureReadyAndNotify` — because `ensureReady` sets `c.sessionID`, so reading it before would yield a stale empty value on first connect.

Final corrected structure (two lock sections, down from the three-guard original; the dead nil-check is gone):

```go
func (c *Client) handleConfigCommand(
    ctx context.Context,
    chatID string,
    args string,
    usage string,
    label string,
    resolve func(input string, st *SessionState) (configID, value string, err error),
) {
    input := strings.TrimSpace(args)
    if input == "" {
        c.reply(chatID, usage)
        return
    }

    c.promptMu.Lock()
    defer c.promptMu.Unlock()

    if err := c.ensureForwarder(ctx); err != nil {
        c.reply(chatID, fmt.Sprintf("No active session: %v. Use /use <agent> to connect.", err))
        return
    }

    // Lock section 1: read agentName and sessionState for config resolution.
    c.mu.Lock()
    agentName := ""
    if c.currentAgent != nil {
        agentName = c.currentAgent.Name()
    }
    var sessionState *SessionState
    if c.state != nil && c.state.Agents != nil {
        if as := c.state.Agents[agentName]; as != nil {
            sessionState = as.Session
        }
    }
    c.mu.Unlock()

    if err := c.ensureReadyAndNotify(ctx, chatID); err != nil {
        c.reply(chatID, fmt.Sprintf("%s error: %v", label, err))
        return
    }

    configID, value, err := resolve(input, sessionState)
    if err != nil {
        c.reply(chatID, fmt.Sprintf("%s error: %v", label, err))
        return
    }

    // Lock section 2: read fwd and sid after ensureReady has set c.sessionID.
    c.mu.Lock()
    fwd := c.forwarder
    sid := c.sessionID
    c.mu.Unlock()

    if _, err := fwd.SessionSetConfigOption(ctx, acp.SessionSetConfigOptionParams{
        SessionID: sid,
        ConfigID:  configID,
        Value:     value,
    }); err != nil {
        c.reply(chatID, fmt.Sprintf("%s error: %v", label, err))
        return
    }

    c.saveSessionState()
    c.reply(chatID, fmt.Sprintf("%s set to: %s", label, value))
}
```

- [ ] **Step 2: Build and test**

```bash
go build ./internal/client/... && go test -race ./internal/client/... -v 2>&1 | tail -20
```

- [ ] **Step 3: Final acceptance check**

```bash
go vet ./...
grep -rn '"claude"' internal/client/
grep -rn '\[\]acp\.MCPServer{}' internal/client/
```
Expected:
- `go vet` exits 0
- `"claude"` only appears in `client.go` as the value of `defaultAgentName` (the constant definition itself), nowhere else
- `[]acp.MCPServer{}` has zero matches

- [ ] **Step 4: Commit**

```bash
git add internal/client/commands.go
git commit -m "refactor(client): remove redundant forwarder nil-check in handleConfigCommand"
```

---

## Final Verification

- [ ] **Run the full test suite with race detection**

```bash
go test -race ./... 2>&1 | grep -E 'FAIL|PASS|race'
```
Expected: all packages PASS, no DATA RACE lines.

- [ ] **Build the binary**

```bash
go build ./cmd/wheelmaker/
```
Expected: exits 0.

- [ ] **Push**

```bash
git push
```
