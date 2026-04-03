# Architecture 3.0 Phase 1: Session Type Extraction

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extract an explicit `Session` type from `Client`, moving session/prompt/terminal/callback state into it while keeping single-session behavior and all existing tests unchanged.

**Architecture:** Session becomes the owner of ACP session state, prompt lifecycle, terminal management, and callback handling. Client retains route mapping, agent registry, IM bridge, and delegates all per-session work to Session. In this phase Client still holds exactly one Session — multi-session support comes in Phase 3.

**Tech Stack:** Go 1.22+, existing `acp`, `agent`, `im` packages unchanged.

---

## File Structure

| File | Action | Responsibility |
|------|--------|---------------|
| `internal/hub/client/session_type.go` | Create | `Session` struct, `SessionStatus`, `SessionAgentState`, `SessionCallbacks` interface, constructor |
| `internal/hub/client/session.go` | Modify | Move `ensureReady`, `ensureReadyAndNotify`, `promptStream`, `cancelPrompt`, `sessionConfigSnapshot`, `invalidateSessionForRetry`, `persistMeta` to be `Session` methods |
| `internal/hub/client/callbacks.go` | Modify | `Session` implements `acp.ClientCallbacks` instead of `Client` |
| `internal/hub/client/commands.go` | Modify | Session-level commands (`/use`, `/cancel`, `/status`, `/mode`, `/model`, `/config`) delegate to Session; Client-level commands (`/new`, `/load`, `/list`) stay on Client |
| `internal/hub/client/lifecycle.go` | Modify | `ensureForwarder` stays on Client (it creates the conn); `switchAgent` moves to Session |
| `internal/hub/client/client.go` | Modify | Remove `session`, `prompt`, `sessionMeta`, `initMeta`, `terminals`, `permRouter`, `initCond` fields; add `sessions map[string]*Session` + `activeSession *Session`; `HandleMessage` delegates to Session |
| `internal/hub/client/terminal.go` | No change | `terminalManager` stays as-is; ownership moves from Client to Session |
| `internal/hub/client/permission.go` | Modify | `permissionRouter` takes Session instead of Client |
| `internal/hub/client/state.go` | No change | Types stay the same |
| `internal/hub/client/store.go` | No change | |
| `internal/hub/client/client_internal_test.go` | Modify | Update `InjectForwarder`, `InjectState` to work with Session |
| `internal/hub/client/client_test.go` | Modify | Minimal updates to test helpers |

---

### Task 1: Define Session types

**Files:**
- Create: `internal/hub/client/session_type.go`

- [ ] **Step 1: Create `session_type.go` with type definitions**

```go
package client

import (
	"sync"
	"time"

	acp "github.com/swm8023/wheelmaker/internal/hub/acp"
)

// SessionStatus defines the lifecycle state of a Session.
type SessionStatus int

const (
	// SessionActive means the session is accepting messages.
	SessionActive SessionStatus = iota
	// SessionSuspended means the session is idle but still in memory.
	SessionSuspended
	// SessionPersisted means the session has been saved to disk and released from memory.
	SessionPersisted
)

// SessionAgentState holds per-agent metadata within one Session.
// Preserved across agent switches so that switching back restores previous state.
type SessionAgentState struct {
	ACPSessionID  string                 `json:"acpSessionId,omitempty"`
	ConfigOptions []acp.ConfigOption     `json:"configOptions,omitempty"`
	Commands      []acp.AvailableCommand `json:"commands,omitempty"`
	Title         string                 `json:"title,omitempty"`
	UpdatedAt     string                 `json:"updatedAt,omitempty"`
}

// Session is the business session object that owns ACP session state,
// prompt lifecycle, terminal management, and callback handling.
// In Phase 1 a Client holds exactly one Session.
type Session struct {
	ID     string
	Status SessionStatus

	// conn bundles the active agent subprocess and forwarder.
	// Owned by Session after Phase 1; created/injected by Client.
	conn *agentConn

	// Per-agent state indexed by agent name.
	// Captures acpSessionId, configOptions, etc. for each agent used in this session.
	agents map[string]*SessionAgentState

	// Runtime ACP session state (moved from Client.session / Client.sessionMeta / Client.initMeta).
	acpSessionID string
	ready        bool
	initializing bool
	lastReply    string
	replayH      func(acp.SessionUpdateParams)
	initMeta     clientInitMeta
	sessionMeta  clientSessionMeta

	prompt   promptState
	initCond *sync.Cond

	terminals  *terminalManager
	permRouter *permissionRouter

	// Back-references to Client-owned resources needed by Session methods.
	cwd              string
	yolo             bool
	debugLog         func() interface{} // returns io.Writer; avoids import cycle
	registry         *agentRegistry
	store            Store
	state            *ProjectState
	imBridge         imBridgeAccessor
	imBlockedUpdates func(acp.UpdateType) bool

	createdAt    time.Time
	lastActiveAt time.Time

	mu       sync.Mutex
	promptMu sync.Mutex
}

// imBridgeAccessor abstracts the IM operations Session needs from Client.
type imBridgeAccessor interface {
	Reply(text string)
	Emit(ctx context.Context, u interface{}) error
	CanHandleDecision() bool
	RequestDecision(ctx context.Context, req interface{}) (interface{}, error)
	ActiveChatID() string
}
```

- [ ] **Step 2: Verify it compiles**

Run: `cd server && go build ./internal/hub/client/...`
Expected: SUCCESS (new types, no usage yet)

- [ ] **Step 3: Commit**

```bash
git add server/internal/hub/client/session_type.go
git commit -m "refactor(client): define Session, SessionStatus, SessionAgentState types"
```

---

### Task 2: Add activeSession to Client and wire initial Session

**Files:**
- Modify: `internal/hub/client/session_type.go`
- Modify: `internal/hub/client/client.go`

- [ ] **Step 1: Add Session constructor and reply method to `session_type.go`**

```go
// newSession creates a Session with sensible defaults.
func newSession(id string, cwd string) *Session {
	s := &Session{
		ID:        id,
		Status:    SessionActive,
		agents:    make(map[string]*SessionAgentState),
		cwd:       cwd,
		createdAt: time.Now(),
		prompt: promptState{
			activeTCs: make(map[string]struct{}),
		},
	}
	s.initCond = sync.NewCond(&s.mu)
	s.terminals = newTerminalManager()
	return s
}

// reply sends a text response via the IM bridge.
func (s *Session) reply(text string) {
	if s.imBridge != nil {
		s.imBridge.Reply(text)
		return
	}
	fmt.Println(text)
}
```

- [ ] **Step 2: Add `activeSession` field to Client struct**

Add to Client struct in `client.go`:

```go
	// activeSession is the Session currently handling messages.
	// In Phase 1 there is always exactly one Session.
	activeSession *Session
```

- [ ] **Step 3: Create initial Session in `Client.New()`**

After existing initialization, add:

```go
	sess := newSession("default", cwd)
	c.activeSession = sess
```

The Session is created but not yet used for delegation — that comes in subsequent tasks.

- [ ] **Step 4: Verify it compiles and tests pass**

Run: `cd server && go test ./internal/hub/client/... -v -count=1`
Expected: All tests pass (activeSession exists but is not used by any logic yet)

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "refactor(client): add activeSession field and Session constructor"
```

---

### Task 3: Session implements acp.ClientCallbacks

Move callback methods from Client to Session. Client's callback methods become thin delegates.

**Files:**
- Modify: `internal/hub/client/callbacks.go`
- Modify: `internal/hub/client/session_type.go` (compile-time check)

- [ ] **Step 1: Add Session callback methods to `callbacks.go`**

Keep the existing Client methods but have them delegate to the active Session. Add Session methods that contain the actual logic (moved from Client):

```go
// compile-time check: Session implements acp.ClientCallbacks.
var _ acp.ClientCallbacks = (*Session)(nil)

// SessionUpdate on Session receives session/update notifications from the Forwarder.
func (s *Session) SessionUpdate(params acp.SessionUpdateParams) {
	s.mu.Lock()
	sessID := s.acpSessionID
	ch := s.prompt.updatesCh
	promptCtx := s.prompt.ctx
	replayH := s.replayH
	s.mu.Unlock()

	if replayH != nil {
		replayH(params)
	}

	if params.SessionID != sessID {
		return
	}

	derived := acp.ParseSessionUpdateParams(params)

	if len(derived.AvailableCommands) > 0 || len(derived.ConfigOptions) > 0 || derived.Title != "" || derived.UpdatedAt != "" {
		s.mu.Lock()
		if len(derived.AvailableCommands) > 0 {
			s.sessionMeta.AvailableCommands = derived.AvailableCommands
		}
		if len(derived.ConfigOptions) > 0 {
			s.sessionMeta.ConfigOptions = derived.ConfigOptions
		}
		if derived.Title != "" {
			s.sessionMeta.Title = derived.Title
		}
		if derived.UpdatedAt != "" {
			s.sessionMeta.UpdatedAt = derived.UpdatedAt
		}
		s.mu.Unlock()
	}

	if derived.TrackAddToolCall != "" || derived.TrackDoneToolCall != "" {
		s.mu.Lock()
		if derived.TrackAddToolCall != "" {
			s.prompt.activeTCs[derived.TrackAddToolCall] = struct{}{}
		}
		if derived.TrackDoneToolCall != "" {
			delete(s.prompt.activeTCs, derived.TrackDoneToolCall)
		}
		s.mu.Unlock()
	}

	if ch == nil {
		return
	}
	if promptCtx == nil {
		ch <- derived.Update
		return
	}
	select {
	case ch <- derived.Update:
	case <-promptCtx.Done():
	}
}
```

Similarly move `SessionRequestPermission`, `FSRead`, `FSWrite`, `TerminalCreate`, `TerminalOutput`, `TerminalWaitForExit`, `TerminalKill`, `TerminalRelease` to Session, with Client methods delegating to `c.activeSession`.

- [ ] **Step 2: Update Client callbacks to delegate**

```go
func (c *Client) SessionUpdate(params acp.SessionUpdateParams) {
	c.mu.Lock()
	sess := c.activeSession
	c.mu.Unlock()
	if sess != nil {
		sess.SessionUpdate(params)
	}
}
// ... same pattern for all other callbacks
```

- [ ] **Step 3: Verify it compiles and tests pass**

Run: `cd server && go test ./internal/hub/client/... -v -count=1`
Expected: All tests pass (Client still implements `acp.ClientCallbacks` and delegates to Session)

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "refactor(client): Session implements acp.ClientCallbacks, Client delegates"
```

---

### Task 4: Move session lifecycle to Session

Move `ensureReady`, `ensureReadyAndNotify`, `promptStream`, `cancelPrompt`, `sessionConfigSnapshot`, `invalidateSessionForRetry`, `resetSessionFields`, `persistMeta`, `saveSessionState` from Client to Session.

**Files:**
- Modify: `internal/hub/client/session.go`
- Modify: `internal/hub/client/client.go`

- [ ] **Step 1: Convert methods in `session.go` from `(c *Client)` to `(s *Session)`**

Key changes:
- `c.mu` → `s.mu`
- `c.session.id` → `s.acpSessionID`
- `c.session.ready` → `s.ready`
- `c.session.initializing` → `s.initializing`
- `c.conn.forwarder` → `s.conn.forwarder`
- `c.conn.name` → `s.conn.name`
- `c.initMeta` → `s.initMeta`
- `c.sessionMeta` → `s.sessionMeta`
- `c.prompt` → `s.prompt`
- `c.initCond` → `s.initCond`
- `c.cwd` → `s.cwd`
- `c.state` → `s.state` (back-reference set by Client)

- [ ] **Step 2: Add delegating methods on Client**

```go
func (c *Client) ensureReady(ctx context.Context) error {
	return c.activeSession.ensureReady(ctx)
}
func (c *Client) ensureReadyAndNotify(ctx context.Context) error {
	return c.activeSession.ensureReadyAndNotify(ctx)
}
// etc.
```

- [ ] **Step 3: Verify tests pass**

Run: `cd server && go test ./internal/hub/client/... -v -count=1`
Expected: All tests pass

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "refactor(client): move session lifecycle methods to Session"
```

---

### Task 5: Move handlePrompt to Session

Move the prompt execution loop from `Client.handlePrompt` to `Session.handlePrompt`.

**Files:**
- Modify: `internal/hub/client/client.go`

- [ ] **Step 1: Create `Session.handlePrompt` with existing logic**

Move the full `handlePrompt` body from `Client` to `Session`, changing:
- `c.promptMu` → `s.promptMu`
- `c.ensureForwarder(ctx)` → Client must ensure forwarder before calling Session
- `c.reply(...)` → `s.reply(...)`
- `c.imBridge` → `s.imBridge`
- `c.resetDeadConnection(err)` → stays on Client (connection management)
- `c.forceReconnect()` → stays on Client

For cross-layer calls (resetDeadConnection, forceReconnect, ensureForwarder), Session holds a `clientOps` interface:

```go
// clientOps provides Client-level operations that Session needs.
type clientOps interface {
	ensureForwarder(ctx context.Context) error
	resetDeadConnection(err error) bool
	forceReconnect()
}
```

- [ ] **Step 2: Client.handlePrompt delegates**

```go
func (c *Client) handlePrompt(msg im.Message, text string) {
	c.mu.Lock()
	sess := c.activeSession
	c.mu.Unlock()
	sess.handlePrompt(msg, text)
}
```

- [ ] **Step 3: Verify tests pass**

Run: `cd server && go test ./internal/hub/client/... -v -count=1`
Expected: All tests pass

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "refactor(client): move handlePrompt to Session"
```

---

### Task 6: Move switchAgent to Session

Move agent switching logic from `Client.switchAgent` to `Session.switchAgent`.

**Files:**
- Modify: `internal/hub/client/lifecycle.go`

- [ ] **Step 1: Create `Session.switchAgent`**

Key changes:
- Session calls `s.clientOps.ensureForwarder()` equivalent — but actually for Phase 1, Session owns the conn, so the connect logic can be inlined or called via `clientOps`
- Save/restore `SessionAgentState` in `s.agents` map during switch
- Terminal kill moves to Session (it owns terminals)
- State persistence uses `s.store` / `s.state`

- [ ] **Step 2: Client.switchAgent delegates**

```go
func (c *Client) switchAgent(ctx context.Context, name string, mode SwitchMode) error {
	c.mu.Lock()
	sess := c.activeSession
	c.mu.Unlock()
	return sess.switchAgent(ctx, name, mode)
}
```

- [ ] **Step 3: Verify tests pass**

Run: `cd server && go test ./internal/hub/client/... -v -count=1`
Expected: All tests pass

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "refactor(client): move switchAgent to Session"
```

---

### Task 7: Move session-level commands to Session

Move `/use`, `/cancel`, `/status`, `/mode`, `/model`, `/config` handlers from Client to Session. Keep `/new`, `/load`, `/list` on Client.

**Files:**
- Modify: `internal/hub/client/commands.go`

- [ ] **Step 1: Split handleCommand**

Client.handleCommand handles `/new`, `/load`, `/list` directly. For the rest, it delegates to `Session.handleCommand`:

```go
func (c *Client) handleCommand(msg im.Message, cmd, args string) {
	switch cmd {
	case "/new", "/load", "/list":
		// Client-level commands stay here (existing code)
		// ...
	default:
		c.mu.Lock()
		sess := c.activeSession
		c.mu.Unlock()
		if sess == nil {
			c.reply("No active session.")
			return
		}
		sess.handleCommand(msg, cmd, args)
	}
}
```

Session.handleCommand handles `/use`, `/cancel`, `/status`, `/mode`, `/model`, `/config`.

- [ ] **Step 2: Move `handleConfigCommand` and resolve helpers to Session**

The resolve functions (`resolveModeArg`, `resolveModelArg`, `resolveConfigArg`, `resolveConfigSelectArg`) are pure functions — they stay as package-level functions. `handleConfigCommand` becomes a Session method.

- [ ] **Step 3: Verify tests pass**

Run: `cd server && go test ./internal/hub/client/... -v -count=1`
Expected: All tests pass

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "refactor(client): split commands between Client and Session"
```

---

### Task 8: Wire Client to Session — remove old fields

Replace Client's direct session/prompt/terminal fields. `activeSession` already exists from Task 2; this task removes the now-unused old fields and adds the `sessions` map.

**Files:**
- Modify: `internal/hub/client/client.go`

- [ ] **Step 1: Remove deprecated fields from Client struct**

Remove from Client (all moved into Session by Tasks 3-7):
- `session sessionState`
- `prompt promptState`
- `initMeta clientInitMeta`
- `sessionMeta clientSessionMeta`
- `terminals *terminalManager`
- `permRouter *permissionRouter`
- `initCond *sync.Cond`
- `conn *agentConn`

Add to Client:
- `sessions map[string]*Session` (for future multi-session; Phase 1 always has one entry)

- [ ] **Step 2: Update `Client.New()` — consolidate Session wiring**

`activeSession` is already created in Task 2. Update `New()` to:
- Initialize `sessions` map
- Wire Session back-references (store, registry, state, imBridge, debugLog, yolo, imBlockedUpdates)
- Register Session in `c.sessions`

```go
	c.sessions = make(map[string]*Session)
	sess := c.activeSession
	sess.store = store
	sess.registry = c.registry
	sess.state = c.state
	sess.imBridge = imProvider
	sess.debugLog = c.debugLog
	sess.yolo = c.yolo
	c.sessions[sess.ID] = sess
```

- [ ] **Step 3: Update `Close()`, `Start()` to act on `activeSession`**

`Close()` should call `c.activeSession.Close()` (kill terminals, cancel prompt, close conn).
`Start()` should call `c.activeSession.Start()` if needed, or remain on Client if it's purely route setup.

- [ ] **Step 4: Remove all remaining `c.session.*`, `c.prompt.*`, `c.initMeta`, `c.sessionMeta`, `c.terminals`, `c.permRouter` references**

The compiler will guide this — any remaining reference will fail to compile.

- [ ] **Step 5: Remove Client delegate methods that are no longer needed**

After wiring `HandleMessage` to call `c.activeSession.handlePrompt()`/`c.activeSession.handleCommand()` directly, the thin `c.handlePrompt`, `c.handleCommand` delegates can be removed if not part of the public API.

- [ ] **Step 6: Verify tests pass**

Run: `cd server && go test ./internal/hub/client/... -v -count=1`
Expected: All tests pass

- [ ] **Step 7: Commit**

```bash
git add -A
git commit -m "refactor(client): Client delegates to activeSession, remove deprecated fields"
```

---

### Task 9: Update test helpers

Update `InjectForwarder`, `InjectState`, and test code to work with the new Session-based architecture.

**Files:**
- Modify: `internal/hub/client/client_internal_test.go`
- Modify: `internal/hub/client/client_test.go`

- [ ] **Step 1: Update `InjectForwarder`**

```go
func (c *Client) InjectForwarder(f *acp.Forwarder, sessionID string) {
	c.mu.Lock()
	name := defaultAgentName
	if c.state != nil && strings.TrimSpace(c.state.ActiveAgent) != "" {
		name = c.state.ActiveAgent
	}
	sess := c.activeSession
	if sess == nil {
		sess = newSession("test", c.cwd)
		c.activeSession = sess
		c.sessions["test"] = sess
	}
	sess.conn = &agentConn{name: name, forwarder: f}
	sess.acpSessionID = sessionID
	sess.ready = true
	if c.state == nil {
		c.state = defaultProjectState()
	}
	sess.state = c.state
	sess.store = c.store
	sess.registry = c.registry
	c.mu.Unlock()
	f.SetCallbacks(sess) // Session is now the callback target
}
```

- [ ] **Step 2: Verify all tests pass**

Run: `cd server && go test ./internal/hub/client/... -v -count=1`
Expected: All existing tests pass without behavior changes

- [ ] **Step 3: Commit**

```bash
git add -A
git commit -m "refactor(client): update test helpers for Session-based architecture"
```

---

### Task 10: Full verification and cleanup

**Files:**
- All files in `internal/hub/client/`
- `internal/hub/hub.go` (if it references Client internals)

- [ ] **Step 1: Run full server test suite**

Run: `cd server && go test ./... -count=1`
Expected: All tests pass

- [ ] **Step 2: Build all binaries**

Run: `cd server && go build ./cmd/...`
Expected: All binaries compile

- [ ] **Step 3: Verify no remaining direct session/prompt field access on Client**

Run: `cd server && grep -rn 'c\.session\.' internal/hub/client/*.go | grep -v '_test.go' | grep -v 'c\.sessionStore'`
Expected: No output (all session state access goes through Session)

- [ ] **Step 4: Commit and tag**

```bash
git add -A
git commit -m "refactor(client): Phase 1 complete — Session type extracted

Session owns:
- ACP session state (acpSessionID, ready, initializing, lastReply)
- Prompt lifecycle (promptState, promptMu)
- Terminal management (terminalManager)
- Callback handling (implements acp.ClientCallbacks)
- Session-level commands (/use, /cancel, /status, /mode, /model, /config)
- Agent switching (switchAgent with SessionAgentState snapshot/restore)

Client retains:
- Agent registry and factory
- IM bridge and route mapping
- Client-level commands (/new, /load, /list)
- Connection creation (ensureForwarder)
- State persistence (store)

All existing tests pass. No behavioral changes."
```

```bash
git push origin main
```
