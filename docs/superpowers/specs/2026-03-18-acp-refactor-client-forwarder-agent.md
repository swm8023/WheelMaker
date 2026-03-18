# ACP Refactor: client → forwarder → agent

**Date**: 2026-03-18
**Status**: Approved

---

## Problem Statement

After Codex's partial migration, the `acp` package still contains stateful session-management
logic (`acp.Agent`, `acp/session.go`, `acp/terminal.go`, `acp/prompt.go`, `acp/callbacks.go`)
that does not belong in a transport layer. Additionally, many filenames, function names,
comments, and documentation were not correctly updated.

Two concrete problems:

1. **Wrong layer boundaries**: `acp.Agent` owns session lifecycle, terminal subprocesses,
   and callback dispatch. Per the ACP protocol, these are *client* responsibilities.
   The `acp` package should be a pure protocol transport.

2. **Naming inconsistency**: Files and symbols left over from the prior `backend` refactor
   (comments saying "backend", `agent_test.go` testing a deleted struct, etc.).

---

## Target Architecture

```
Hub → client.Client ──────────────────────────► acp.Forwarder → acp.Conn → CLI subprocess
           │                                           ▲
           │ implements acp.ClientCallbacks            │ prefilter(NormalizeParams)
           └───────────────────────────────────────────┘
                      agent.Agent (factory + protocol hooks)
```

Layered responsibilities:

| Layer | Package | Owns |
|-------|---------|------|
| Transport | `acp` | Conn, Forwarder (typed outbound methods + SetCallbacks), protocol types, ClientCallbacks interface |
| Factory + hooks | `internal/agent/` | Subprocess factory (Connect), NormalizeParams, HandlePermission |
| Orchestration | `client` | Session state, lifecycle, terminal management, implements ClientCallbacks |

---

## Design

### 1. `acp/handler.go` (new file)

Defines the client-capabilities interface that `client.Client` must implement.
Maps to ACP protocol's client-side request handlers (fs, terminal, permission).

```go
// ClientCallbacks is the interface client.Client must implement.
// Forwarder dispatches inbound agent→client requests and notifications to these methods.
type ClientCallbacks interface {
    // SessionUpdate is called for each incoming session/update notification.
    // No response is sent (notifications are fire-and-forget from the protocol side).
    // The client routes the update to the active prompt channel by session ID.
    SessionUpdate(params SessionUpdateParams)

    // SessionRequestPermission responds to session/request_permission requests.
    SessionRequestPermission(ctx context.Context, params PermissionRequestParams) (PermissionResult, error)

    FSRead(params FSReadTextFileParams) (FSReadTextFileResult, error)
    FSWrite(params FSWriteTextFileParams) error
    TerminalCreate(params TerminalCreateParams) (TerminalCreateResult, error)
    TerminalOutput(params TerminalOutputParams) (TerminalOutputResult, error)
    TerminalWaitForExit(params TerminalWaitForExitParams) (TerminalWaitForExitResult, error)
    TerminalKill(params TerminalKillParams) error
    TerminalRelease(params TerminalReleaseParams) error
}
```

### 2. `acp/forwarder.go` changes

**New method: `SetCallbacks`**

Registers a `ClientCallbacks` implementation. Internally calls `f.conn.OnRequest` to
wire up JSON dispatch for all agent→client request methods, and `f.conn.Subscribe` to
dispatch `session/update` notifications. JSON unmarshal/marshal is handled inside
Forwarder; client methods receive and return typed structs only.

```go
func (f *Forwarder) SetCallbacks(h ClientCallbacks)
```

**New typed outbound methods (Client → Agent requests)**

Client calls these instead of raw `forwarder.Send(ctx, "method", raw, &raw)`.
Each method represents a JSON-RPC request; the return value is the response.
`SessionPrompt` covers both new user turns and continuation replies.

```go
// Initialization
func (f *Forwarder) Initialize(ctx context.Context, params InitializeParams) (InitializeResult, error)

// Session lifecycle
func (f *Forwarder) SessionNew(ctx context.Context, params SessionNewParams) (SessionNewResult, error)
func (f *Forwarder) SessionLoad(ctx context.Context, params SessionLoadParams) (SessionLoadResult, error)
func (f *Forwarder) SessionList(ctx context.Context, params SessionListParams) (SessionListResult, error)

// Session prompt turn: client sends user message (new turn or reply); blocks until agent
// returns a stop reason. Streaming session/update notifications arrive via SessionUpdate callback.
func (f *Forwarder) SessionPrompt(ctx context.Context, params SessionPromptParams) (SessionPromptResult, error)

// Session control
func (f *Forwarder) SessionCancel(sessionID string) error
func (f *Forwarder) SessionSetConfigOption(ctx context.Context, params SessionSetConfigOptionParams) ([]ConfigOption, error)
```

`SessionSetConfigOption` handles the dual-format response (array or wrapped object) internally.

**Agent → Client** interactions are handled via `ClientCallbacks` (Section 1):
- `SessionUpdate` — notification, no response required
- `SessionRequestPermission` — request, client must return `PermissionResult`
- `FSRead`, `FSWrite`, `Terminal*` — requests, client returns typed results

**Unchanged**: `NewForwarder(conn, prefilter)`, `Send`, `Notify`, `OnRequest`, `Subscribe` stay as-is.
`conn_test.go` and `conn_integration_test.go` are untouched.

### 3. `acp` files deleted

| File | Reason |
|------|--------|
| `acp/agent.go` | `acp.Agent` struct deleted; session logic moves to client |
| `acp/session.go` | `ensureReady` moves to `client/session.go` |
| `acp/prompt.go` | Prompt streaming moves to `client/session.go` |
| `acp/callbacks.go` | Callback dispatch moves to `client/callbacks.go` |
| `acp/terminal.go` | `TerminalManager` moves to `client/terminal.go` |
| `acp/agent_test.go` | Tests for deleted struct |

**Types deleted from `acp`**: `acp.Session` interface, `acp.InitMeta`, `acp.SessionMeta`,
`acp.SwitchMode`, `acp.SwitchClean`, `acp.SwitchWithContext`, `backendHooks`, `noopHooks`.

**Types that move to `client`**: `SwitchMode`, `SwitchClean`, `SwitchWithContext`
(defined in `client/session.go`; used only by `client.switchAgent`).

`acp.SessionConfigSnapshot` and `sessionConfigSnapshotFromOptions` stay in `acp/session_config.go`.

### 4. `client` package changes

#### New `Client` struct fields (in `client.go`)

```go
// replaces ag *acp.Agent and session acp.Session
currentAgent agent.Agent       // factory kept for switchAgent; nil until ensureForwarder
forwarder     *acp.Forwarder   // active transport; nil until first connection
terminals     *TerminalManager // created once in New(), never replaced

// session state (moved from acp.Agent)
sessionID        string
ready            bool
initializing     bool
initCond         *sync.Cond    // guards single-flight ensureReady; associated with mu
initMeta         clientInitMeta   // local type: ProtocolVersion, AgentCapabilities, etc.
sessionMeta      clientSessionMeta // local type: ConfigOptions, AvailableCommands, Title, UpdatedAt
lastReply        string
loadHistory      []acp.Update
activeToolCalls  map[string]struct{}
promptCtx        context.Context
promptCancel     context.CancelFunc
promptUpdatesCh  chan<- acp.Update
```

`terminals` is allocated in `New()` and lives for the lifetime of the `Client`.
On `idleClose` and agent switch, `terminals.KillAll()` is called to clean up.

`clientInitMeta` and `clientSessionMeta` are unexported struct types defined in
`client/session.go`, mirroring the fields of the former `acp.InitMeta` and `acp.SessionMeta`.

#### `client/session.go` (new file)

Absorbs `acp.Agent`'s session logic as `*Client` methods:

- `ensureReady(ctx) error` — single-flight initialize + session/new or session/load handshake.
  Uses `c.initCond` for the same "one goroutine initializes, others wait" pattern from `acp/session.go`.
- `promptStream(ctx, text) (<-chan acp.Update, error)` — calls `c.forwarder.SessionPrompt(...)`,
  subscribes to `session/update` notifications, streams `acp.Update` values. Logic mirrors
  `acp/prompt.go`'s goroutine pattern including `promptCtx`, `activeToolCalls`, `lastReply`.
- `cancelPrompt() error` — emits tool_call_cancelled updates then calls `c.forwarder.SessionCancel`.
- Local `SwitchMode`, `SwitchClean`, `SwitchWithContext` constants.
- `persistMeta()` — replaces `persistAgentMeta(ag *acp.Agent)`. Reads `c.sessionID`,
  `c.initMeta`, `c.sessionMeta` directly from the Client, updates `c.state`, returns bool.
- `saveSessionState()` — calls `persistMeta()` then `c.store.Save(c.state)` if changed.
  Replaces all `saveAgentState(ag)` call sites in `client.go`.
- `ensureReadyAndNotify(ctx, chatID) error` — same as current, but calls `c.ensureReady(ctx)`.

#### `client/terminal.go` (new file)

`TerminalManager` moved verbatim from `acp/terminal.go`.
Retains `acp` package imports for protocol types (`TerminalCreateParams`, etc.)
— the struct parameter types are defined in `acp/protocol.go` and remain there.

#### `client/callbacks.go` (new file)

Implements `acp.ClientCallbacks` on `*Client`. All methods use typed params from `acp` package.

**SessionRequestPermission**:
```go
func (c *Client) SessionRequestPermission(ctx context.Context, params acp.PermissionRequestParams) (acp.PermissionResult, error) {
    // Use per-prompt context so Cancel() unblocks pending permission requests.
    // If promptCtx is set (i.e. a prompt is active), substitute it for the
    // connection-level ctx passed by Forwarder's OnRequest handler.
    c.mu.Lock()
    pCtx := c.promptCtx
    snap := acp.SessionConfigSnapshotFromOptions(c.sessionMeta.ConfigOptions)
    c.mu.Unlock()
    if pCtx != nil {
        ctx = pCtx
    }
    return c.permRouter.decide(ctx, params, snap.Mode, c.currentAgent)
}
```

`permRouter.decide` already handles "ask" mode (interactive) and falls back to
`c.currentAgent.HandlePermission` for other modes. The `c.currentAgent` reference
replaces the former `fallback agent.Agent` parameter pattern in `interactiveAgent`.

**FSRead / FSWrite**: same logic as `acp/callbacks.go` `callbackFSRead`/`callbackFSWrite`.

**Terminal***: delegates to `c.terminals`.

#### `client/permission.go` changes

Delete `interactiveAgent` struct and `prefilterCap` interface — no longer needed.
`permissionRouter` stays unchanged, but `decide`'s `fallback` parameter type changes from
`agent.Agent` to be looked up from `c.currentAgent` (see HandlePermission above).
Specifically: `decide(ctx, params, mode string, fallback agent.Agent)` signature stays;
callers pass `c.currentAgent`.

#### `client/export_test.go` changes

`InjectSession(sess acp.Session)` is replaced by `InjectForwarder(f *acp.Forwarder)` which
sets `c.forwarder` and marks the session ready with a preset `sessionID`. Tests that
previously relied on `InjectSession` with a mock `acp.Session` will use a mock forwarder
or the existing mock agent infrastructure.

#### `client.go` changes

- Remove fields: `ag *acp.Agent`, `session acp.Session`
- Add fields: see new struct fields above
- `New()`: allocate `c.terminals = newTerminalManager()`, allocate `c.activeToolCalls`
- `ensureAgent` → renamed `ensureForwarder`:
  ```
  if c.forwarder != nil { return nil }
  baseAgent := fac("", nil)
  conn, err := baseAgent.Connect(ctx)
  c.forwarder = acp.NewForwarder(conn, makePrefilter(baseAgent))
  c.forwarder.SetCallbacks(c)
  c.currentAgent = baseAgent
  ```
- `switchAgent`: cancel current prompt → drain → kill terminals → close old conn →
  create new conn → create new forwarder → set callbacks → reset session state fields
  (sessionID="", ready=false, initMeta={}, sessionMeta={}, lastReply="", loadHistory=nil).
  For `SwitchWithContext`: after reset, call `promptStream` with the saved `lastReply`
  as the bootstrap message (same logic as former `acp.Agent.Switch`).
- `handlePrompt`: calls `c.promptStream(ctx, text)` instead of `sess.Prompt(ctx, text)`.
  `saveAgentState(ag)` → `c.saveSessionState()`.
- `/cancel` command: calls `c.cancelPrompt()`.
- `/status` command: reads `c.sessionID`, `c.currentAgent.Name()` directly.
- `idleClose`: calls `c.terminals.KillAll()`, `c.saveSessionState()`, resets forwarder/state.
- All `ensureReadyAndNotify(ctx, chatID, ag)` calls → `c.ensureReadyAndNotify(ctx, chatID)`.

#### `client.go` NormalizeParams prefilter helper

```go
func makePrefilter(ag agent.Agent) acp.Prefilter {
    return func(ctx context.Context, msg acp.ForwardMessage) (acp.ForwardMessage, bool, error) {
        if msg.Direction == acp.DirectionToClient && msg.Kind == acp.KindNotification {
            msg.Params = ag.NormalizeParams(msg.Method, msg.Params)
        }
        return msg, true, nil
    }
}
```

The former `backendHooks.Prefilter` method (from `interactiveAgent`) is intentionally
removed. Neither Claude nor Codex requires a bidirectional prefilter beyond `NormalizeParams`.

### 5. `internal/agent` package changes

**`internal/agent/agent.go`**

```go
type Agent interface {
    Name() string
    Connect(ctx context.Context) (*acp.Conn, error)
    Close() error
    // HandlePermission decides the default permission outcome for non-interactive modes.
    HandlePermission(ctx context.Context, params acp.PermissionRequestParams, mode string) (acp.PermissionResult, error)
    // NormalizeParams translates agent-specific protocol fields to standard ACP.
    // Called via Forwarder prefilter on every incoming notification.
    NormalizeParams(method string, params json.RawMessage) json.RawMessage
}
```

`internal/agent/claude/agent.go`, `internal/agent/codex/agent.go`, `internal/agent/mock/agent.go`:
already satisfy this interface; no changes needed to method signatures.

### 6. Documentation and naming cleanup

- `CLAUDE.md` architecture section updated: layer table and diagram
- `acp` package doc comment updated (remove "Session interface, Agent runtime")
- `internal/agent/` package doc comment updated (replace "backend" references)
- `internal/agent/mock/backend_test.go` → review and rename/delete stale file
- Test files in `internal/agent/claude/` (`backend_test.go`, `backend_integration_test.go`)
  → rename to `agent_test.go`, `agent_integration_test.go`
- Test files in `internal/agent/codex/` → same rename pattern

---

## File Change Summary

| Action | File |
|--------|------|
| New | `acp/handler.go` |
| Modify | `acp/forwarder.go` (typed methods + SetCallbacks) |
| Delete | `acp/agent.go` |
| Delete | `acp/session.go` |
| Delete | `acp/prompt.go` |
| Delete | `acp/callbacks.go` |
| Delete | `acp/terminal.go` |
| Delete | `acp/agent_test.go` |
| Unchanged | `acp/conn.go`, `acp/conn_test.go`, `acp/conn_integration_test.go`, `acp/jsonrpc.go`, `acp/protocol.go`, `acp/update.go`, `acp/session_config.go` |
| New | `client/session.go` |
| New | `client/terminal.go` |
| New | `client/callbacks.go` |
| Modify | `client/client.go` |
| Modify | `client/permission.go` (delete interactiveAgent + prefilterCap) |
| Modify | `client/export_test.go` (InjectSession → InjectForwarder) |
| Modify | `client/client_test.go` (update to new API) |
| Modify | `internal/agent/agent.go` (cleanup only, interface unchanged) |
| Rename | `internal/agent/claude/backend_test.go` → `agent_test.go` |
| Rename | `internal/agent/claude/backend_integration_test.go` → `agent_integration_test.go` |
| Rename | `internal/agent/codex/backend_integration_test.go` → `agent_integration_test.go` |
| Rename | `internal/agent/codex/backend_test.go` → `agent_test.go` |
| Rename | `internal/agent/mock/backend_test.go` → `agent_test.go` (or delete if redundant) |
| Modify | `CLAUDE.md` |

---

## Acceptance Criteria

1. `go test ./...` passes with zero compile errors
2. `acp` package has no stateful session management; no `acp.Agent` struct
3. `acp.Forwarder` has typed outbound methods (`Initialize`, `SessionNew`, etc.) and `SetCallbacks`
4. `acp.ClientCallbacks` interface defined and implemented by `client.Client`
5. `client.Client` owns session state, terminal management, and all ACP callback implementations
6. `interactiveAgent` and `prefilterCap` removed from `client/permission.go`
7. `promptCtx` is substituted into `SessionRequestPermission` so `Cancel()` unblocks pending permission
8. No "backend" references remain in comments or identifiers
9. `CLAUDE.md` architecture matches actual code
10. Integration test: console IM project can initialize, prompt, and cancel a session
