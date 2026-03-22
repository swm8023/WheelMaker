# internal/client Refactor Design

**Date:** 2026-03-22
**Scope:** `internal/client/` — structural split + 7 detail fixes
**Approach:** Two-step (structure first, then details)

---

## Goals

1. Split the 1017-line `client.go` into three focused files without changing behavior.
2. Eliminate hardcodes, duplicate logic, and a latent race condition found during review.
3. Maintain full ACP protocol compliance (all 7 client-side callbacks, initialize handshake flow).

---

## Step 1 — Structural Split (pure mechanical move)

No logic changes. Only move declarations between files. Commit separately for clean diff.

### File layout after split

| File | Responsibility |
|------|----------------|
| `client.go` | `Client` struct, `New`, `Start`, `Run`, `Close`, `HandleMessage`, `parseCommand`, `handlePrompt`, `reply`, `replyDebug`, `renderUnknown` |
| `commands.go` | `handleCommand`, `handleConfigCommand`, `resolveModeArg`, `resolveModelArg`, `resolveConfigSelectArg`, `formatConfigOptionUpdateMessage`, `listSessions`, `createNewSession`, `loadSessionByIndex`, `persistSessionSummaries`, `resolveHelpModel`, `firstNonEmpty` |
| `lifecycle.go` | `ensureForwarder`, `switchAgent`, `registeredAgentNames` |
| `session.go` | Unchanged — `Session` interface, `SwitchMode`, `clientInitMeta`, `clientSessionMeta`, `ensureReady`, `ensureReadyAndNotify`, `sessionConfigSnapshot`, `promptStream`, `cancelPrompt`, `persistMeta`, `saveSessionState` |

All other files (`callbacks.go`, `state.go`, `store.go`, `permission.go`, `terminal.go`, `debug.go`) are unchanged.

---

## Step 2 — Detail Fixes (7 issues)

### Fix 1: Hardcoded default agent name

**Problem:** `"claude"` appears literally in `ensureForwarder` (line 706) and `defaultProjectState()`.
**Fix:** Add package-level constant `defaultAgentName = "claude"` in `client.go`; replace all literal occurrences.

### Fix 2: Repeated `MCPServers: []acp.MCPServer{}`

**Problem:** `session/new` and `session/load` params both hardcode an empty MCP server list in three places (`ensureReady`, `createNewSession`, `loadSessionByIndex`).
**Fix:** Add helper `emptyMCPServers() []acp.MCPServer { return []acp.MCPServer{} }` or use a typed nil-safe approach. All three call sites use the helper. When MCP config support is added later, only the helper changes.

### Fix 3: Client identity constants scattered in `ensureReady`

**Problem:** `const clientProtocolVersion = 1` and `&acp.AgentInfo{Name: "wheelmaker", Version: "0.1"}` are defined inside `ensureReady`, making them invisible to other methods.
**Fix:** Promote to package-level constants/vars in `client.go`:
```go
const acpClientProtocolVersion = 1
var acpClientInfo = &acp.AgentInfo{Name: "wheelmaker", Version: "0.1"}
```

### Fix 4: Apparent race window in `persistMeta`

**Problem:** `persistMeta` currently acquires `c.mu` twice (read then write), which looks like a TOCTOU. However, every caller of `persistMeta` is serialized by `promptMu`: `saveSessionState` is only called from inside `promptMu`-held paths (`handlePrompt`, `switchAgent`, `createNewSession`, `loadSessionByIndex`), and `switchAgent` holds `promptMu` for its entire body.

**Fix:** No lock restructuring. Instead, add an explicit comment at the top of `persistMeta` documenting the `promptMu` serialization invariant so future readers understand why the two-acquisition pattern is safe. Do **not** collapse into a single lock: `c.store.Save` performs file I/O and must never run while holding `c.mu` (would stall ACP callback goroutines).

### Fix 5: Duplicate session state reset

**Problem:** The same 6-field reset block (`sessionID`, `ready`, `lastReply`, `loadHistory`, `activeToolCalls`, `sessionMeta`) is copy-pasted in `createNewSession`, `loadSessionByIndex`, and `switchAgent`.

**Fix:** Extract private method `c.resetSessionFields(sid string, configOpts []acp.ConfigOption)` that resets exactly these 6 fields. **Callers must hold `c.mu` before calling** (consistent with surrounding lock patterns at all three call sites).

`switchAgent` also resets additional agent-switch-specific fields (`initializing`, `promptUpdatesCh`, `initMeta`, `currentAgent`, `currentAgentName`, `forwarder`) and calls `c.initCond.Broadcast()` **after** releasing `c.mu` — these remain at the `switchAgent` call site and are not moved into the helper. The helper is responsible only for the 6 common fields listed above.

### Fix 6: Double-unmarshal in `formatConfigOptionUpdateMessage`

**Problem:** The function tries `json.Unmarshal` into `acp.SessionUpdate`, and if that "fails" (no ConfigOptions), retries into `acp.SessionUpdateParams`. But the Raw bytes for `UpdateConfigOption` are always marshaled from `acp.SessionUpdate` (in `sessionUpdateToUpdate`), so the second branch is dead code.
**Fix:** Remove the second unmarshal attempt. If `len(opts) == 0` after the first parse, return the fallback string directly.

### Fix 7: Redundant forwarder nil-check in `handleConfigCommand`

**Problem:** After `ensureForwarder(ctx)` returns `nil` (success), the function re-reads `c.forwarder` under lock and checks `if fwd == nil` — which can never be true at that point.
**Fix:** Remove the second `c.mu.Lock()` / `fwd == nil` guard block. Read `sessID` in the same lock section as `agentName` (earlier in the function), eliminating the extra critical section entirely.

---

## Acceptance Criteria

- `go build ./...` passes with no errors.
- `go test -race ./internal/client/...` passes (all existing tests green, no data races).
- `go vet ./...` reports no issues.
- Step 1 commit contains zero logic changes (verifiable by inspection).
- Step 2 commit addresses all 7 fixes:
  - No hardcoded agent name strings outside `defaultAgentName` constant.
  - No inline `[]acp.MCPServer{}` literals at session/new or session/load call sites.
  - `acpClientProtocolVersion` and `acpClientInfo` are package-level, not local to `ensureReady`.
  - `persistMeta` has a comment documenting the `promptMu` serialization invariant.
  - `resetSessionFields` (or equivalent) is the sole location for the 6-field common session reset; `switchAgent`'s agent-switch-specific resets remain at its call site.
  - `formatConfigOptionUpdateMessage` contains exactly one `json.Unmarshal` into `acp.SessionUpdate`.
  - `handleConfigCommand` acquires `c.mu` at most twice after `ensureForwarder` returns (once before `ensureReadyAndNotify` for `agentName`/`sessionState`, once after for `fwd`/`sid`); the unreachable `fwd == nil` guard is removed.

---

## Out of Scope

- Adding MCP server config loading (Fix 2 only creates the helper, not the config wiring).
- Changing the `session.go` file or ACP protocol types.
- Any new features or behavioral changes.
