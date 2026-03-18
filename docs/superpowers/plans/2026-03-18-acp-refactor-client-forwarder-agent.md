# ACP Refactor: client → forwarder → agent — Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove `acp.Agent` and all stateful session logic from the `acp` package; move it to `client`; add typed methods to `acp.Forwarder`; define `acp.ClientCallbacks` interface that `client.Client` implements.

**Architecture:** `acp` becomes a pure transport layer (Conn + Forwarder + protocol types). `client.Client` owns all session state, terminal management, and ACP callback implementations. `agent.Agent` is a stateless subprocess factory with per-agent protocol hooks (NormalizeParams, HandlePermission).

**Tech Stack:** Go 1.21+; `go test ./...`; `git add -p && git commit && git push` after every task.

**Spec:** `docs/superpowers/specs/2026-03-18-acp-refactor-client-forwarder-agent.md`

---

## File Map

| Action | File | Responsibility |
|--------|------|----------------|
| New | `internal/acp/handler.go` | `ClientCallbacks` interface definition |
| Modify | `internal/acp/forwarder.go` | `SetCallbacks` + typed outbound methods |
| New | `internal/acp/protocol.go` additions | `SessionListParams`, `SessionListResult`, `SessionInfo` types |
| Delete | `internal/acp/agent.go` | `acp.Agent` struct removed |
| Delete | `internal/acp/session.go` | `ensureReady` removed |
| Delete | `internal/acp/prompt.go` | Prompt streaming removed |
| Delete | `internal/acp/callbacks.go` | Callback dispatch removed |
| Delete | `internal/acp/terminal.go` | TerminalManager removed |
| Delete | `internal/acp/agent_test.go` | Tests for deleted struct |
| New | `internal/client/terminal.go` | TerminalManager (moved from acp) |
| New | `internal/client/session.go` | ensureReady, promptStream, cancelPrompt, persistMeta, SwitchMode |
| New | `internal/client/callbacks.go` | Client implements acp.ClientCallbacks |
| Modify | `internal/client/client.go` | Remove acp.Agent fields; add forwarder/session fields; wire new API |
| Modify | `internal/client/permission.go` | Delete interactiveAgent, prefilterCap |
| Modify | `internal/client/export_test.go` | InjectSession → InjectForwarder |
| Modify | `internal/client/client_test.go` | Remove mockSession; use mock agent + InjectForwarder |
| Modify | `internal/client/store.go` | No changes expected; verify |
| Modify | `internal/agent/agent.go` | Cleanup comments only (interface already correct) |
| Rename | `internal/agent/claude/backend_test.go` → `agent_test.go` | Naming |
| Rename | `internal/agent/claude/backend_integration_test.go` → `agent_integration_test.go` | Naming |
| Rename | `internal/agent/codex/backend_test.go` → `agent_test.go` | Naming |
| Rename | `internal/agent/codex/backend_integration_test.go` → `agent_integration_test.go` | Naming |
| Rename | `internal/agent/mock/backend_test.go` → `agent_test.go` | Naming |
| Modify | `CLAUDE.md` | Update architecture diagram and layer table |

---

## Chunk 1: Transport layer — `acp` package

### Task 1: Add missing protocol types + create `acp/handler.go`

**Files:**
- Modify: `internal/acp/protocol.go`
- Create: `internal/acp/handler.go`

- [ ] **Step 1: Add SessionList types to `protocol.go`**

Append after the last type in `internal/acp/protocol.go`:

```go
// SessionListParams requests a paginated list of sessions.
type SessionListParams struct {
	CWD    string `json:"cwd,omitempty"`
	Cursor string `json:"cursor,omitempty"`
}

// SessionInfo is a single entry in a session list.
type SessionInfo struct {
	SessionID string `json:"sessionId"`
	CWD       string `json:"cwd"`
	Title     string `json:"title,omitempty"`
	UpdatedAt string `json:"updatedAt,omitempty"`
}

// SessionListResult is the response to session/list.
type SessionListResult struct {
	Sessions   []SessionInfo `json:"sessions"`
	NextCursor string        `json:"nextCursor,omitempty"`
}
```

- [ ] **Step 2: Create `internal/acp/handler.go`**

```go
package acp

import "context"

// ClientCallbacks is the interface client.Client must implement.
// Forwarder dispatches inbound agent→client requests and notifications
// to these methods after JSON unmarshal — client code never sees raw JSON.
type ClientCallbacks interface {
	// SessionUpdate is called for each incoming session/update notification.
	// Notifications require no response. The client routes updates to the
	// active prompt channel by matching the session ID.
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

- [ ] **Step 3: Verify compile**

```bash
cd /d/Code/WheelMaker && go build ./internal/acp/...
```

Expected: no output (success).

- [ ] **Step 4: Commit**

```bash
cd /d/Code/WheelMaker
git add internal/acp/handler.go internal/acp/protocol.go
git commit -m "feat(acp): add ClientCallbacks interface and SessionList protocol types"
git push
```

---

### Task 2: Add typed outbound methods + `SetCallbacks` to `acp/forwarder.go`

**Files:**
- Modify: `internal/acp/forwarder.go`

The current `forwarder.go` has bidirectional prefilter and raw `Send`/`Notify`/`OnRequest`/`Subscribe`. We add typed wrappers on top.

- [ ] **Step 1: Add `SetCallbacks` to `forwarder.go`**

Append after the `marshalParams` helper at the bottom of `internal/acp/forwarder.go`:

```go
// SetCallbacks registers h as the handler for all agent→client requests and
// session/update notifications. It wires up both conn.OnRequest (for requests)
// and conn.Subscribe (for notifications) internally so the client never deals
// with raw JSON dispatch.
//
// SetCallbacks must be called before the first prompt; it is not safe to call
// concurrently with active requests.
func (f *Forwarder) SetCallbacks(h ClientCallbacks) {
	// Wire inbound request dispatch.
	f.conn.OnRequest(func(ctx context.Context, method string, params json.RawMessage) (any, error) {
		return dispatchClientRequest(ctx, method, params, h)
	})
	// Wire session/update notification dispatch.
	f.conn.Subscribe(func(n Notification) {
		if n.Method != "session/update" {
			return
		}
		var p SessionUpdateParams
		if err := json.Unmarshal(n.Params, &p); err != nil {
			return
		}
		h.SessionUpdate(p)
	})
}

// dispatchClientRequest maps an agent→client JSON-RPC method to the typed
// ClientCallbacks method. Returns the result or an error; Forwarder's
// OnRequest handler serialises the return value to JSON automatically.
func dispatchClientRequest(ctx context.Context, method string, params json.RawMessage, h ClientCallbacks) (any, error) {
	switch method {
	case "session/request_permission":
		var p PermissionRequestParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, fmt.Errorf("permission: unmarshal: %w", err)
		}
		result, err := h.SessionRequestPermission(ctx, p)
		if err != nil {
			return nil, err
		}
		// B2 fix: wrap in PermissionResponse so result JSON is {"outcome":{...}}.
		return PermissionResponse{Outcome: result}, nil

	case "fs/read_text_file":
		var p FSReadTextFileParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, fmt.Errorf("fs/read: unmarshal: %w", err)
		}
		return h.FSRead(p)

	case "fs/write_text_file":
		var p FSWriteTextFileParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, fmt.Errorf("fs/write: unmarshal: %w", err)
		}
		return nil, h.FSWrite(p)

	case "terminal/create":
		var p TerminalCreateParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, fmt.Errorf("terminal/create: unmarshal: %w", err)
		}
		return h.TerminalCreate(p)

	case "terminal/output":
		var p TerminalOutputParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, fmt.Errorf("terminal/output: unmarshal: %w", err)
		}
		return h.TerminalOutput(p)

	case "terminal/wait_for_exit":
		var p TerminalWaitForExitParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, fmt.Errorf("terminal/wait_for_exit: unmarshal: %w", err)
		}
		return h.TerminalWaitForExit(p)

	case "terminal/kill":
		var p TerminalKillParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, fmt.Errorf("terminal/kill: unmarshal: %w", err)
		}
		return nil, h.TerminalKill(p)

	case "terminal/release":
		var p TerminalReleaseParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, fmt.Errorf("terminal/release: unmarshal: %w", err)
		}
		return nil, h.TerminalRelease(p)

	default:
		return nil, fmt.Errorf("unsupported method: %s", method)
	}
}
```

- [ ] **Step 2: Add typed outbound methods to `forwarder.go`**

Append after `SetCallbacks`:

```go
// Initialize sends the ACP initialize handshake (client→agent).
func (f *Forwarder) Initialize(ctx context.Context, params InitializeParams) (InitializeResult, error) {
	var result InitializeResult
	if err := f.conn.Send(ctx, "initialize", params, &result); err != nil {
		return InitializeResult{}, err
	}
	return result, nil
}

// SessionNew creates a new ACP session (client→agent).
func (f *Forwarder) SessionNew(ctx context.Context, params SessionNewParams) (SessionNewResult, error) {
	var result SessionNewResult
	if err := f.Send(ctx, "session/new", params, &result); err != nil {
		return SessionNewResult{}, err
	}
	return result, nil
}

// SessionLoad resumes an existing ACP session (client→agent).
func (f *Forwarder) SessionLoad(ctx context.Context, params SessionLoadParams) (SessionLoadResult, error) {
	var result SessionLoadResult
	if err := f.Send(ctx, "session/load", params, &result); err != nil {
		return SessionLoadResult{}, err
	}
	return result, nil
}

// SessionList returns a paginated list of available sessions (client→agent).
// Only call this after verifying AgentCapabilities.SessionCapabilities.List != nil.
func (f *Forwarder) SessionList(ctx context.Context, params SessionListParams) (SessionListResult, error) {
	var result SessionListResult
	if err := f.Send(ctx, "session/list", params, &result); err != nil {
		return SessionListResult{}, err
	}
	return result, nil
}

// SessionPrompt sends a user message (new turn or reply) to the agent and
// blocks until the agent returns a stop reason. Streaming session/update
// notifications are delivered concurrently via the SessionUpdate callback.
func (f *Forwarder) SessionPrompt(ctx context.Context, params SessionPromptParams) (SessionPromptResult, error) {
	var result SessionPromptResult
	if err := f.Send(ctx, "session/prompt", params, &result); err != nil {
		return SessionPromptResult{}, err
	}
	return result, nil
}

// SessionCancel sends session/cancel to abort an in-progress prompt.
func (f *Forwarder) SessionCancel(sessionID string) error {
	return f.Notify("session/cancel", SessionCancelParams{SessionID: sessionID})
}

// SessionSetConfigOption sets a named config option on the active session and
// returns the updated config option list. Handles both response formats:
// []ConfigOption and {"configOptions":[...]}.
func (f *Forwarder) SessionSetConfigOption(ctx context.Context, params SessionSetConfigOptionParams) ([]ConfigOption, error) {
	var raw json.RawMessage
	if err := f.Send(ctx, "session/set_config_option", params, &raw); err != nil {
		return nil, err
	}
	var opts []ConfigOption
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &opts); err != nil {
			var wrapped struct {
				ConfigOptions []ConfigOption `json:"configOptions"`
			}
			if json.Unmarshal(raw, &wrapped) == nil {
				opts = wrapped.ConfigOptions
			}
		}
	}
	return opts, nil
}
```

- [ ] **Step 3: Verify compile**

```bash
cd /d/Code/WheelMaker && go build ./internal/acp/...
```

Expected: no output.

- [ ] **Step 4: Run existing acp tests**

```bash
cd /d/Code/WheelMaker && go test ./internal/acp/... -count=1
```

Expected: all tests pass (conn tests are untouched).

- [ ] **Step 5: Commit**

```bash
cd /d/Code/WheelMaker
git add internal/acp/forwarder.go
git commit -m "feat(acp): add SetCallbacks and typed outbound methods to Forwarder"
git push
```

---

## Chunk 2: `client` new files

### Task 3: Create `client/terminal.go`

Move `TerminalManager` from `acp/terminal.go` to `client/terminal.go` verbatim.

**Files:**
- Create: `internal/client/terminal.go`

- [ ] **Step 1: Create the file**

Copy the entire content of `internal/acp/terminal.go` and change the package declaration to `package client`. The `acp` import for protocol types (`TerminalCreateParams`, etc.) stays.

```go
package client

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/swm8023/wheelmaker/internal/acp"
)
```

Then copy all types and methods from `acp/terminal.go` unchanged (replacing `package acp` header with the above). The only change is the package line and the import of `acp` for param types.

- [ ] **Step 2: Verify the file compiles in isolation**

```bash
cd /d/Code/WheelMaker && go build ./internal/client/...
```

Expected: may fail with "duplicate" errors because `acp/terminal.go` still exists — that's OK at this step.

- [ ] **Step 3: Commit (do not delete acp/terminal.go yet)**

```bash
cd /d/Code/WheelMaker
git add internal/client/terminal.go
git commit -m "feat(client): add TerminalManager (copy from acp before deletion)"
git push
```

---

### Task 4: Create `client/callbacks.go`

Implement `acp.ClientCallbacks` on `*Client` with all callback methods.

**Files:**
- Create: `internal/client/callbacks.go`

- [ ] **Step 1: Create the file**

```go
package client

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/swm8023/wheelmaker/internal/acp"
)

// compile-time check: Client implements acp.ClientCallbacks.
var _ acp.ClientCallbacks = (*Client)(nil)

// SessionUpdate receives session/update notifications from the Forwarder.
// Routes the update to the active promptUpdatesCh if the session ID matches.
func (c *Client) SessionUpdate(params acp.SessionUpdateParams) {
	c.mu.Lock()
	sessID := c.sessionID
	ch := c.promptUpdatesCh
	c.mu.Unlock()

	if params.SessionID != sessID || ch == nil {
		return
	}

	// NormalizeParams is applied by the Forwarder's prefilter before this
	// method is called, so params are already in standard ACP format.
	u := sessionUpdateToUpdate(params.Update)

	// Track session metadata updates.
	switch params.Update.SessionUpdate {
	case "available_commands_update":
		if len(params.Update.AvailableCommands) > 0 {
			c.mu.Lock()
			c.sessionMeta.AvailableCommands = params.Update.AvailableCommands
			c.mu.Unlock()
		}
	case "config_option_update":
		if len(params.Update.ConfigOptions) > 0 {
			c.mu.Lock()
			c.sessionMeta.ConfigOptions = params.Update.ConfigOptions
			c.mu.Unlock()
		}
	case "session_info_update":
		c.mu.Lock()
		if params.Update.Title != "" {
			c.sessionMeta.Title = params.Update.Title
		}
		if params.Update.UpdatedAt != "" {
			c.sessionMeta.UpdatedAt = params.Update.UpdatedAt
		}
		c.mu.Unlock()
	}

	// Track active tool calls for cancelPrompt.
	if id := params.Update.ToolCallID; id != "" {
		switch params.Update.SessionUpdate {
		case "tool_call":
			if s := params.Update.Status; s == "completed" || s == "failed" {
				c.mu.Lock()
				delete(c.activeToolCalls, id)
				c.mu.Unlock()
			} else {
				c.mu.Lock()
				c.activeToolCalls[id] = struct{}{}
				c.mu.Unlock()
			}
		case "tool_call_update":
			if s := params.Update.Status; s == "completed" || s == "failed" {
				c.mu.Lock()
				delete(c.activeToolCalls, id)
				c.mu.Unlock()
			}
		}
	}

	// Send update to the active prompt channel. Use recover() to handle the
	// case where the channel is already closed (ctx cancelled race).
	func() {
		defer func() { recover() }() //nolint:errcheck
		select {
		case ch <- u:
		default:
		}
	}()
}

// SessionRequestPermission responds to session/request_permission agent requests.
// Substitutes promptCtx so that Cancel() unblocks pending permission dialogs.
func (c *Client) SessionRequestPermission(ctx context.Context, params acp.PermissionRequestParams) (acp.PermissionResult, error) {
	c.mu.Lock()
	pCtx := c.promptCtx
	snap := acp.SessionConfigSnapshotFromOptions(c.sessionMeta.ConfigOptions)
	ag := c.currentAgent
	c.mu.Unlock()
	if pCtx != nil {
		ctx = pCtx
	}
	return c.permRouter.decide(ctx, params, snap.Mode, ag)
}

// FSRead responds to fs/read_text_file agent requests.
func (c *Client) FSRead(params acp.FSReadTextFileParams) (acp.FSReadTextFileResult, error) {
	data, err := os.ReadFile(params.Path)
	if err != nil {
		return acp.FSReadTextFileResult{}, fmt.Errorf("fs/read: %w", err)
	}
	content := string(data)
	if params.Line != nil || params.Limit != nil {
		lines := strings.Split(content, "\n")
		start := 0
		if params.Line != nil {
			start = *params.Line - 1
			if start < 0 {
				start = 0
			}
			if start > len(lines) {
				start = len(lines)
			}
		}
		end := len(lines)
		if params.Limit != nil {
			end = start + *params.Limit
			if end > len(lines) {
				end = len(lines)
			}
		}
		content = strings.Join(lines[start:end], "\n")
	}
	return acp.FSReadTextFileResult{Content: content}, nil
}

// FSWrite responds to fs/write_text_file agent requests.
func (c *Client) FSWrite(params acp.FSWriteTextFileParams) error {
	if err := os.MkdirAll(filepath.Dir(params.Path), 0o755); err != nil {
		return fmt.Errorf("fs/write: mkdir: %w", err)
	}
	if err := os.WriteFile(params.Path, []byte(params.Content), 0o644); err != nil {
		return fmt.Errorf("fs/write: %w", err)
	}
	return nil
}

// TerminalCreate responds to terminal/create agent requests.
func (c *Client) TerminalCreate(params acp.TerminalCreateParams) (acp.TerminalCreateResult, error) {
	return c.terminals.Create(params)
}

// TerminalOutput responds to terminal/output agent requests.
func (c *Client) TerminalOutput(params acp.TerminalOutputParams) (acp.TerminalOutputResult, error) {
	return c.terminals.Output(params.TerminalID)
}

// TerminalWaitForExit responds to terminal/wait_for_exit agent requests.
func (c *Client) TerminalWaitForExit(params acp.TerminalWaitForExitParams) (acp.TerminalWaitForExitResult, error) {
	return c.terminals.WaitForExit(params.TerminalID)
}

// TerminalKill responds to terminal/kill agent requests.
func (c *Client) TerminalKill(params acp.TerminalKillParams) error {
	return c.terminals.Kill(params.TerminalID)
}

// TerminalRelease responds to terminal/release agent requests.
func (c *Client) TerminalRelease(params acp.TerminalReleaseParams) error {
	return c.terminals.Release(params.TerminalID)
}
```

Note: `sessionUpdateToUpdate` is defined in `client/session.go` (Task 5). The compile-time check `var _ acp.ClientCallbacks = (*Client)(nil)` will fail until `session.go` is created and `Client` has all required fields. That's expected at this step.

- [ ] **Step 2: Verify the file compiles (errors expected about missing fields)**

```bash
cd /d/Code/WheelMaker && go build ./internal/client/... 2>&1 | head -20
```

Expected: errors about missing fields (`promptUpdatesCh`, `sessionMeta`, etc.) — that is correct, they come in Task 6.

- [ ] **Step 3: Commit**

```bash
cd /d/Code/WheelMaker
git add internal/client/callbacks.go
git commit -m "feat(client): add ClientCallbacks implementation (compile errors expected until session.go)"
git push
```

---

### Task 5: Create `client/session.go`

Session lifecycle, prompt streaming, state persistence — the bulk of logic moved from `acp/agent.go`, `acp/session.go`, `acp/prompt.go`.

**Files:**
- Create: `internal/client/session.go`

- [ ] **Step 1: Create `internal/client/session.go`**

```go
package client

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/swm8023/wheelmaker/internal/acp"
)

// SwitchMode controls how an agent switch affects session context.
type SwitchMode int

const (
	// SwitchClean discards the current session; new conn is lazily initialized on next prompt.
	SwitchClean SwitchMode = iota
	// SwitchWithContext passes the last reply as bootstrap context to the new session.
	SwitchWithContext
)

// clientInitMeta holds agent-level metadata from the initialize handshake.
type clientInitMeta struct {
	ProtocolVersion    string
	AgentCapabilities  acp.AgentCapabilities
	AgentInfo          *acp.AgentInfo
	AuthMethods        []acp.AuthMethod
	ClientProtocolVersion int
	ClientCapabilities acp.ClientCapabilities
	ClientInfo         *acp.AgentInfo
}

// clientSessionMeta holds session-level metadata updated by session/update notifications.
type clientSessionMeta struct {
	ConfigOptions     []acp.ConfigOption
	AvailableCommands []acp.AvailableCommand
	Title             string
	UpdatedAt         string
}

// ensureReady performs the ACP handshake if the client is not yet connected:
//  1. Send "initialize" and store agent capabilities.
//  2. If caps.LoadSession and a sessionID is stored, attempt session/load.
//  3. Otherwise, create a new session via session/new.
//
// Single-flight: if concurrent callers race here, only one performs the I/O;
// others wait on c.initCond and return once ready is set.
func (c *Client) ensureReady(ctx context.Context) error {
	c.mu.Lock()
	for c.initializing {
		c.initCond.Wait()
	}
	if c.ready {
		c.mu.Unlock()
		return nil
	}
	c.initializing = true
	fwd := c.forwarder
	savedSID := c.sessionID
	cwd := c.cwd
	c.mu.Unlock()

	notifyDone := func() {
		c.mu.Lock()
		c.initializing = false
		c.mu.Unlock()
		c.initCond.Broadcast()
	}

	// Step 1: initialize handshake.
	clientCaps := acp.ClientCapabilities{
		FS: &acp.FSCapabilities{
			ReadTextFile:  true,
			WriteTextFile: true,
		},
		Terminal: true,
	}
	clientInfo := &acp.AgentInfo{Name: "wheelmaker", Version: "0.1"}
	const clientProtocolVersion = 1

	initResult, err := fwd.Initialize(ctx, acp.InitializeParams{
		ProtocolVersion:    clientProtocolVersion,
		ClientCapabilities: clientCaps,
		ClientInfo:         clientInfo,
	})
	if err != nil {
		notifyDone()
		return fmt.Errorf("ensureReady: initialize: %w", err)
	}

	newInitMeta := clientInitMeta{
		ProtocolVersion:       initResult.ProtocolVersion.String(),
		AgentCapabilities:     initResult.AgentCapabilities,
		AgentInfo:             initResult.AgentInfo,
		AuthMethods:           initResult.AuthMethods,
		ClientProtocolVersion: clientProtocolVersion,
		ClientCapabilities:    clientCaps,
		ClientInfo:            clientInfo,
	}

	// Step 2: attempt session/load if possible.
	if savedSID != "" && initResult.AgentCapabilities.LoadSession {
		// The Forwarder's SetCallbacks subscriber dispatches session/update
		// notifications to SessionUpdate(), which routes them to promptUpdatesCh.
		// During session/load we don't have a promptUpdatesCh, so we capture
		// replayed updates via a temporary channel before setting up the real one.
		replayCh := make(chan acp.SessionUpdateParams, 128)
		var replayOnce sync.Once

		// Temporarily override SessionUpdate to capture replay notifications.
		// We achieve this by subscribing directly via forwarder.Subscribe before load.
		var replayMu sync.Mutex
		var replay []acp.Update
		replayMeta := clientSessionMeta{}

		cancelReplaySub := fwd.Subscribe(func(n acp.Notification) {
			if n.Method != "session/update" {
				return
			}
			// Apply NormalizeParams (already done by prefilter in Forwarder).
			var p acp.SessionUpdateParams
			if err := json.Unmarshal(n.Params, &p); err != nil || p.SessionID != savedSID {
				return
			}
			u := sessionUpdateToUpdate(p.Update)
			replayMu.Lock()
			replay = append(replay, u)
			switch p.Update.SessionUpdate {
			case "available_commands_update":
				if len(p.Update.AvailableCommands) > 0 {
					replayMeta.AvailableCommands = p.Update.AvailableCommands
				}
			case "config_option_update":
				if len(p.Update.ConfigOptions) > 0 {
					replayMeta.ConfigOptions = p.Update.ConfigOptions
				}
			case "session_info_update":
				if p.Update.Title != "" {
					replayMeta.Title = p.Update.Title
				}
				if p.Update.UpdatedAt != "" {
					replayMeta.UpdatedAt = p.Update.UpdatedAt
				}
			}
			replayMu.Unlock()
			replayOnce.Do(func() { close(replayCh) }) // signal that at least one arrived
		})

		loadErr := func() error {
			_, err := fwd.SessionLoad(ctx, acp.SessionLoadParams{
				SessionID:  savedSID,
				CWD:        cwd,
				MCPServers: []acp.MCPServer{},
			})
			return err
		}()
		cancelReplaySub()

		if loadErr == nil {
			replayMu.Lock()
			replayUpdates := replay
			meta := replayMeta
			replayMu.Unlock()

			c.mu.Lock()
			c.initMeta = newInitMeta
			c.sessionMeta = meta
			c.loadHistory = replayUpdates
			c.ready = true
			c.initializing = false
			c.mu.Unlock()
			c.initCond.Broadcast()
			log.Printf("[client] connected: agent=%s session=%s (resumed, %d history updates)",
				c.currentAgent.Name(), savedSID, len(replayUpdates))
			return nil
		}
		// session/load failed — fall through to session/new.
	}

	// Step 3: create a new session.
	newResult, err := fwd.SessionNew(ctx, acp.SessionNewParams{
		CWD:        cwd,
		MCPServers: []acp.MCPServer{},
	})
	if err != nil {
		notifyDone()
		return fmt.Errorf("ensureReady: session/new: %w", err)
	}

	c.mu.Lock()
	c.initMeta = newInitMeta
	c.sessionID = newResult.SessionID
	c.sessionMeta = clientSessionMeta{
		ConfigOptions: newResult.ConfigOptions,
	}
	c.ready = true
	c.initializing = false
	c.mu.Unlock()
	c.initCond.Broadcast()

	modeID := ""
	for _, opt := range newResult.ConfigOptions {
		if opt.ID == "mode" || opt.Category == "mode" {
			modeID = opt.CurrentValue
			break
		}
	}
	log.Printf("[client] connected: agent=%s session=%s mode=%s",
		c.currentAgent.Name(), newResult.SessionID, modeID)
	return nil
}

// ensureReadyAndNotify calls ensureReady and sends a "Session ready" message
// to chatID when this call is the one that first transitions to ready.
func (c *Client) ensureReadyAndNotify(ctx context.Context, chatID string) error {
	c.mu.Lock()
	wasReady := c.ready
	c.mu.Unlock()

	if err := c.ensureReady(ctx); err != nil {
		return err
	}

	if !wasReady {
		snap := c.sessionConfigSnapshot()
		c.reply(chatID, fmt.Sprintf("Session ready: mode=%s model=%s",
			renderUnknown(snap.Mode), renderUnknown(snap.Model)))
		c.saveSessionState()
	}
	return nil
}

// sessionConfigSnapshot returns the current mode/model values.
func (c *Client) sessionConfigSnapshot() acp.SessionConfigSnapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	return acp.SessionConfigSnapshotFromOptions(c.sessionMeta.ConfigOptions)
}

// promptStream sends a prompt and returns a channel of streaming updates.
// The caller must drain the channel until a Done update is received.
func (c *Client) promptStream(ctx context.Context, text string) (<-chan acp.Update, error) {
	// Clear lastReply so SwitchWithContext never sees a stale value.
	c.mu.Lock()
	c.lastReply = ""
	c.mu.Unlock()

	if err := c.ensureReady(ctx); err != nil {
		return nil, err
	}

	c.mu.Lock()
	sessID := c.sessionID
	promptCtx, promptCancel := context.WithCancel(ctx)
	c.promptCtx = promptCtx
	c.promptCancel = promptCancel
	c.mu.Unlock()

	updates := make(chan acp.Update, 32)

	c.mu.Lock()
	c.promptUpdatesCh = updates
	c.mu.Unlock()

	// replyBuf accumulates text for SwitchWithContext.
	var replyMu sync.Mutex
	var replyBuf strings.Builder

	// The Forwarder's SetCallbacks subscriber already dispatches session/update
	// notifications to c.SessionUpdate(), which routes them to c.promptUpdatesCh.
	// No additional subscription is needed here.

	go func() {
		defer func() {
			c.mu.Lock()
			if c.promptCancel != nil {
				c.promptCtx = nil
				c.promptCancel = nil
			}
			c.activeToolCalls = make(map[string]struct{})
			c.promptUpdatesCh = nil
			c.mu.Unlock()
			promptCancel()
		}()

		// Override: we need to intercept updates for replyBuf accumulation
		// before they go to the channel. To do this cleanly, we wrap the channel.
		interceptCh := make(chan acp.Update, 32)
		c.mu.Lock()
		c.promptUpdatesCh = interceptCh
		c.mu.Unlock()

		result, err := c.forwarder.SessionPrompt(promptCtx, acp.SessionPromptParams{
			SessionID: sessID,
			Prompt:    []acp.ContentBlock{{Type: "text", Text: text}},
		})

		// Collect text for lastReply.
		replyMu.Lock()
		reply := replyBuf.String()
		replyMu.Unlock()
		c.mu.Lock()
		c.lastReply = reply
		c.mu.Unlock()

		// Drain interceptCh into updates, accumulating text as we go.
		draining := true
		for draining {
			select {
			case u, ok := <-interceptCh:
				if !ok {
					draining = false
					break
				}
				if u.Type == acp.UpdateText {
					replyMu.Lock()
					replyBuf.WriteString(u.Content)
					replyMu.Unlock()
				}
				select {
				case updates <- u:
				case <-ctx.Done():
					draining = false
				}
			default:
				draining = false
			}
		}

		var finalUpdate acp.Update
		if err != nil {
			finalUpdate = acp.Update{Type: acp.UpdateError, Err: err, Done: true}
		} else {
			finalUpdate = acp.Update{Type: acp.UpdateDone, Content: result.StopReason, Done: true}
		}
		select {
		case updates <- finalUpdate:
		case <-ctx.Done():
		}
		close(updates)
	}()

	return updates, nil
}

// cancelPrompt emits tool_call_cancelled updates then sends session/cancel.
func (c *Client) cancelPrompt() error {
	c.mu.Lock()
	sessID := c.sessionID
	ready := c.ready
	cancel := c.promptCancel
	ch := c.promptUpdatesCh
	var cancelIDs []string
	for id := range c.activeToolCalls {
		cancelIDs = append(cancelIDs, id)
	}
	c.mu.Unlock()

	if cancel != nil {
		cancel()
	}

	for _, id := range cancelIDs {
		u := acp.Update{Type: acp.UpdateToolCallCancelled, Content: id}
		func() {
			defer func() { recover() }() //nolint:errcheck
			if ch != nil {
				select {
				case ch <- u:
				default:
				}
			}
		}()
	}

	if sessID == "" || !ready {
		return nil
	}
	return c.forwarder.SessionCancel(sessID)
}

// persistMeta snapshots current session metadata into in-memory state.
// Returns true if anything changed. Must be called while NOT holding c.mu.
func (c *Client) persistMeta() bool {
	c.mu.Lock()
	agentName := ""
	if c.currentAgent != nil {
		agentName = c.currentAgent.Name()
	}
	sessionID := c.sessionID
	initMeta := c.initMeta
	sessMeta := c.sessionMeta
	c.mu.Unlock()

	if agentName == "" {
		return false
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.state == nil {
		return false
	}
	if c.state.Agents == nil {
		c.state.Agents = map[string]*AgentState{}
	}
	as := c.state.Agents[agentName]
	if as == nil {
		as = &AgentState{}
		c.state.Agents[agentName] = as
	}

	changed := false

	if sessionID != "" && as.LastSessionID != sessionID {
		as.LastSessionID = sessionID
		changed = true
	}

	if initMeta.ProtocolVersion != "" {
		as.ProtocolVersion = initMeta.ProtocolVersion
		as.AgentCapabilities = initMeta.AgentCapabilities
		as.AgentInfo = initMeta.AgentInfo
		as.AuthMethods = initMeta.AuthMethods
		if c.state.Connection == nil {
			c.state.Connection = &ConnectionConfig{}
		}
		c.state.Connection.ProtocolVersion = initMeta.ClientProtocolVersion
		c.state.Connection.ClientCapabilities = initMeta.ClientCapabilities
		c.state.Connection.ClientInfo = initMeta.ClientInfo
		changed = true
	}

	hasSessionData := len(sessMeta.AvailableCommands) > 0 || len(sessMeta.ConfigOptions) > 0 ||
		sessMeta.Title != "" || sessMeta.UpdatedAt != ""
	if hasSessionData {
		if as.Session == nil {
			as.Session = &SessionState{}
		}
		as.Session.ConfigOptions = sessMeta.ConfigOptions
		as.Session.AvailableCommands = sessMeta.AvailableCommands
		if sessMeta.Title != "" {
			as.Session.Title = sessMeta.Title
		}
		if sessMeta.UpdatedAt != "" {
			as.Session.UpdatedAt = sessMeta.UpdatedAt
		}
		changed = true
	}

	return changed
}

// saveSessionState calls persistMeta and writes to disk if changed.
func (c *Client) saveSessionState() {
	if !c.persistMeta() {
		return
	}
	c.mu.Lock()
	s := c.state
	c.mu.Unlock()
	if s != nil {
		_ = c.store.Save(s)
	}
}

// sessionUpdateToUpdate converts an ACP SessionUpdate notification into a client Update.
func sessionUpdateToUpdate(u acp.SessionUpdate) acp.Update {
	switch u.SessionUpdate {
	case "agent_message_chunk":
		text := ""
		if u.Content != nil {
			var cb acp.ContentBlock
			if err := json.Unmarshal(u.Content, &cb); err == nil && cb.Type == "text" {
				text = cb.Text
			}
		}
		return acp.Update{Type: acp.UpdateText, Content: text}

	case "agent_thought_chunk":
		text := ""
		if u.Content != nil {
			var cb acp.ContentBlock
			if err := json.Unmarshal(u.Content, &cb); err == nil {
				text = cb.Text
			}
		}
		return acp.Update{Type: acp.UpdateThought, Content: text}

	case "tool_call", "tool_call_update":
		raw, _ := json.Marshal(u)
		return acp.Update{Type: acp.UpdateToolCall, Raw: raw}

	case "plan":
		raw, _ := json.Marshal(u)
		return acp.Update{Type: acp.UpdatePlan, Raw: raw}

	case "config_option_update":
		raw, _ := json.Marshal(u)
		return acp.Update{Type: acp.UpdateConfigOption, Raw: raw}

	case "current_mode_update":
		raw, _ := json.Marshal(u)
		return acp.Update{Type: acp.UpdateModeChange, Raw: raw}

	default:
		raw, _ := json.Marshal(u)
		return acp.Update{Type: acp.UpdateType(u.SessionUpdate), Raw: raw}
	}
}
```

- [ ] **Step 2: Export `SessionConfigSnapshotFromOptions` in `acp/session_config.go`**

The current function is unexported (`sessionConfigSnapshotFromOptions`). Rename to exported:

In `internal/acp/session_config.go`, change:
```go
func sessionConfigSnapshotFromOptions(opts []ConfigOption) SessionConfigSnapshot {
```
to:
```go
func SessionConfigSnapshotFromOptions(opts []ConfigOption) SessionConfigSnapshot {
```

Then search for all callers of the old name and update them (currently only in `acp/agent.go` and `acp/callbacks.go`, which will be deleted; and in `client/callbacks.go` which uses the new name).

- [ ] **Step 3: Verify compile (will still fail — client.go fields not updated yet)**

```bash
cd /d/Code/WheelMaker && go build ./internal/client/... 2>&1 | head -30
```

Expected: errors about missing struct fields on Client. That's OK — fixed in Task 6.

- [ ] **Step 4: Commit**

```bash
cd /d/Code/WheelMaker
git add internal/client/session.go internal/acp/session_config.go
git commit -m "feat(client): add session.go with ensureReady/promptStream/persistMeta; export SessionConfigSnapshotFromOptions"
git push
```

---

## Chunk 3: Rewrite `client.go` and `permission.go`

### Task 6: Rewrite `client/client.go` — struct fields and wiring

This is the largest change. Work methodically through the file.

**Files:**
- Modify: `internal/client/client.go`

- [ ] **Step 1: Replace struct fields**

In the `Client` struct, remove:
```go
session   acp.Session
ag        *acp.Agent
```

Add:
```go
currentAgent agent.Agent       // active agent factory; nil until ensureForwarder
forwarder    *acp.Forwarder    // active transport; nil until first connection

// session state
sessionID       string
ready           bool
initializing    bool
initCond        *sync.Cond
initMeta        clientInitMeta
sessionMeta     clientSessionMeta
lastReply       string
loadHistory     []acp.Update
activeToolCalls map[string]struct{}
promptCtx       context.Context
promptCancel    context.CancelFunc
promptUpdatesCh chan<- acp.Update

terminals *TerminalManager
```

- [ ] **Step 2: Update `New()` to initialise new fields**

In `New()`, add after the existing struct initialisation:
```go
c.initCond = sync.NewCond(&c.mu)
c.activeToolCalls = make(map[string]struct{})
c.terminals = newTerminalManager()
```

- [ ] **Step 3: Update `Close()`**

Replace `ag := c.ag` + `saveAgentState(ag)` + `ag.Close()` with:
```go
c.mu.Lock()
fwd := c.forwarder
c.mu.Unlock()

if fwd != nil {
    c.saveSessionState()
    _ = fwd.conn.Close()  // close the underlying connection
}
```

Actually: `Forwarder` doesn't expose `conn` directly. Add a `Close()` method to `Forwarder` in `acp/forwarder.go`:
```go
// Close closes the underlying Conn.
func (f *Forwarder) Close() error { return f.conn.Close() }
```

Then `client.Close()` calls `fwd.Close()`.

- [ ] **Step 4: Rename `ensureAgent` → `ensureForwarder`**

Replace the entire `ensureAgent` method body:

```go
// ensureForwarder connects the active agent and sets up the Forwarder if not already running.
// Must be called while holding c.mu.
func (c *Client) ensureForwarder(ctx context.Context) error {
	if c.forwarder != nil {
		return nil
	}
	if c.state == nil {
		return errors.New("state not loaded")
	}
	name := c.state.ActiveAgent
	if name == "" {
		name = "claude"
	}
	fac := c.agentFacs[name]
	if fac == nil {
		return fmt.Errorf("no agent registered for %q", name)
	}
	baseAgent := fac("", nil)
	conn, err := baseAgent.Connect(ctx)
	if err != nil {
		return fmt.Errorf("connect %q: %w", name, err)
	}
	if c.debugLog != nil {
		conn.SetDebugLogger(c.debugLog)
	}
	fwd := acp.NewForwarder(conn, makePrefilter(baseAgent))
	fwd.SetCallbacks(c)
	c.forwarder = fwd
	c.currentAgent = baseAgent
	c.ready = false
	// Restore saved session ID if present.
	if c.state.Agents != nil {
		if as := c.state.Agents[name]; as != nil && as.LastSessionID != "" {
			c.sessionID = as.LastSessionID
		}
	}
	c.resetIdleTimer()
	return nil
}
```

- [ ] **Step 5: Add `makePrefilter` helper to `client.go`**

```go
// makePrefilter returns a Forwarder prefilter that applies ag.NormalizeParams
// to every incoming notification, ensuring the client receives standard ACP.
func makePrefilter(ag agent.Agent) acp.Prefilter {
	return func(ctx context.Context, msg acp.ForwardMessage) (acp.ForwardMessage, bool, error) {
		if msg.Direction == acp.DirectionToClient && msg.Kind == acp.KindNotification {
			msg.Params = ag.NormalizeParams(msg.Method, msg.Params)
		}
		return msg, true, nil
	}
}
```

- [ ] **Step 6: Update `handleCommand` — replace old session references**

Replace every `c.session` reference with direct field/method access:
- `sess.Cancel()` → `c.cancelPrompt()`
- `sess.AgentName()` → `c.currentAgent.Name()` (guarded: `if c.currentAgent == nil`)
- `sess.SessionID()` → `c.sessionID` (read under `c.mu`)
- `ag.SetConfigOption(...)` → `c.forwarder.SessionSetConfigOption(ctx, acp.SessionSetConfigOptionParams{SessionID: c.sessionID, ConfigID: configID, Value: value})`
- `c.ensureAgent(ctx)` → `c.ensureForwarder(ctx)`
- `c.ensureReadyAndNotify(ctx, chatID, ag)` → `c.ensureReadyAndNotify(ctx, chatID)`
- `c.saveAgentState(ag)` → `c.saveSessionState()`
- `ag.SessionConfigSnapshot()` → `c.sessionConfigSnapshot()` (returns `(acp.SessionConfigSnapshot, bool)` — update callers)

- [ ] **Step 7: Update `handlePrompt`**

Replace:
```go
sess := c.session
ag := c.ag
...
updates, err := sess.Prompt(ctx, text)
...
c.saveAgentState(ag)
```
With:
```go
updates, err := c.promptStream(ctx, text)
...
c.saveSessionState()
```

- [ ] **Step 8: Update `idleClose`**

Replace:
```go
ag := c.ag
c.saveAgentState(ag)
_ = ag.Close()
c.session = nil
c.ag = nil
```
With:
```go
c.saveSessionState()
c.terminals.KillAll()
fwd := c.forwarder
if fwd != nil {
    _ = fwd.Close()
}
c.forwarder = nil
c.currentAgent = nil
c.ready = false
c.sessionID = ""
c.initMeta = clientInitMeta{}
c.sessionMeta = clientSessionMeta{}
```

- [ ] **Step 9: Rewrite `switchAgent`**

This replaces the existing `switchAgent` method. The logic:

```
1. Signal cancel → promptMu.Lock() → drain currentPromptCh
2. Re-read c.forwarder and c.currentAgent under mu
3. Save outgoing agent state (persistMeta) before resetting
4. Read savedSID for new agent from state
5. Connect new agent → build new forwarder
6. Kill terminals → close old conn
7. Reset session state fields
8. For SwitchWithContext: capture lastReply before reset, run bootstrap prompt after
9. Update c.state.ActiveAgent → resetIdleTimer → store.Save
10. Reply "Switched to agent: X"
```

```go
func (c *Client) switchAgent(ctx context.Context, chatID, name string, mode SwitchMode) error {
	c.mu.Lock()
	fac := c.agentFacs[name]
	c.mu.Unlock()
	if fac == nil {
		return fmt.Errorf("unknown agent: %q (registered: %v)", name, c.registeredAgentNames())
	}

	// Step 1: cancel in-progress prompt, wait for handlePrompt to release promptMu.
	_ = c.cancelPrompt()
	c.promptMu.Lock()
	defer c.promptMu.Unlock()
	c.mu.Lock()
	promptCh := c.currentPromptCh
	c.mu.Unlock()
	if promptCh != nil {
		for range promptCh {
		}
		c.mu.Lock()
		c.currentPromptCh = nil
		c.mu.Unlock()
	}

	// Step 2: capture outgoing state.
	c.mu.Lock()
	oldFwd := c.forwarder
	savedLastReply := c.lastReply
	c.mu.Unlock()
	c.persistMeta() // save outgoing agent state before reset

	// Step 3: read saved session ID for incoming agent.
	c.mu.Lock()
	var savedSID string
	if c.state != nil && c.state.Agents != nil {
		if as := c.state.Agents[name]; as != nil {
			savedSID = as.LastSessionID
		}
	}
	dw := c.debugLog
	c.mu.Unlock()

	// Step 4: connect new agent.
	baseAgent := fac("", nil)
	newConn, err := baseAgent.Connect(ctx)
	if err != nil {
		return fmt.Errorf("connect %q: %w", name, err)
	}
	if dw != nil {
		newConn.SetDebugLogger(dw)
	}
	newFwd := acp.NewForwarder(newConn, makePrefilter(baseAgent))
	newFwd.SetCallbacks(c)

	// Step 5: replace forwarder atomically; kill terminals; close old conn.
	c.mu.Lock()
	c.terminals.KillAll()
	c.forwarder = newFwd
	c.currentAgent = baseAgent
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
	c.initCond.Broadcast()

	if oldFwd != nil {
		_ = oldFwd.Close()
	}

	// Step 6: SwitchWithContext — bootstrap new session with previous reply.
	if mode == SwitchWithContext && savedLastReply != "" {
		ch, err := c.promptStream(ctx, "[context] "+savedLastReply)
		if err != nil {
			log.Printf("client: SwitchWithContext bootstrap failed: %v", err)
		} else {
			for u := range ch {
				if u.Err != nil {
					log.Printf("client: SwitchWithContext bootstrap failed: %v", u.Err)
				}
			}
		}
		c.persistMeta()
	}

	// Step 7: update ActiveAgent, save, reset timer.
	c.mu.Lock()
	if c.state != nil {
		c.state.ActiveAgent = name
	}
	c.resetIdleTimer()
	s := c.state
	c.mu.Unlock()
	if s != nil {
		_ = c.store.Save(s)
	}

	c.reply(chatID, fmt.Sprintf("Switched to agent: %s", name))
	snap := c.sessionConfigSnapshot()
	if snap.Mode != "" || snap.Model != "" {
		c.reply(chatID, fmt.Sprintf("Session ready: mode=%s model=%s",
			renderUnknown(snap.Mode), renderUnknown(snap.Model)))
	}
	return nil
}
```

- [ ] **Step 10: Remove `persistAgentMeta`, `saveAgentState`, `ensureReadyAndNotify` (old signatures), `formatConfigOptionUpdateMessage` raw JSON parsing — replace with session.go equivalents**

Delete `persistAgentMeta(ag *acp.Agent)`, `saveAgentState(ag *acp.Agent)`.
The `formatConfigOptionUpdateMessage` function parses raw JSON from `acp.Update.Raw`. This still works — keep it but update the `handlePrompt` call site to pass `u.Raw`.

- [ ] **Step 11: Attempt compile**

```bash
cd /d/Code/WheelMaker && go build ./internal/client/... 2>&1 | head -40
```

Fix all remaining compile errors iteratively. Common issues:
- References to deleted `c.session` / `c.ag`
- Old import of `acp.SwitchMode` → use local `SwitchMode` from `client/session.go`
- References to `acp.Session` in imports
- `resolveModeArg` / `resolveModelArg` still use `*SessionState` — check they still compile

- [ ] **Step 12: Commit once it compiles**

```bash
cd /d/Code/WheelMaker
git add internal/client/client.go
git commit -m "refactor(client): replace acp.Agent with direct forwarder + session state"
git push
```

---

### Task 7: Update `client/permission.go`

**Files:**
- Modify: `internal/client/permission.go`

- [ ] **Step 1: Delete `interactiveAgent` and `prefilterCap`**

Remove from `internal/client/permission.go`:
- The `interactiveAgent` struct (lines ~186-218)
- The `prefilterCap` interface
- The `var _ agent.Agent = (*interactiveAgent)(nil)` compile-time check

Keep: `permissionRouter`, `pendingPermission`, `newPermissionRouter`, all helper functions.

- [ ] **Step 2: Update `permRouter.decide` if needed**

The method signature `decide(ctx, params, mode string, fallback agent.Agent)` stays unchanged — `client/callbacks.go` passes `c.currentAgent` as the fallback. Verify no changes needed.

- [ ] **Step 3: Verify compile**

```bash
cd /d/Code/WheelMaker && go build ./internal/client/...
```

Expected: no output.

- [ ] **Step 4: Commit**

```bash
cd /d/Code/WheelMaker
git add internal/client/permission.go
git commit -m "refactor(client): remove interactiveAgent and prefilterCap"
git push
```

---

### Task 8: Update `client/export_test.go`

**Files:**
- Modify: `internal/client/export_test.go`

- [ ] **Step 1: Replace `InjectSession` with `InjectForwarder`**

Replace the entire `export_test.go` content:

```go
package client

// export_test.go exposes internal helpers for package client_test.
// Compiled only during `go test`.

import (
	"github.com/swm8023/wheelmaker/internal/acp"
	"github.com/swm8023/wheelmaker/internal/im"
)

// InjectForwarder sets an active Forwarder and marks the session as ready
// with a preset session ID. Used by client_test to bypass Start() when
// testing with a mock agent.
func (c *Client) InjectForwarder(f *acp.Forwarder, sessionID string) {
	c.mu.Lock()
	c.forwarder = f
	c.sessionID = sessionID
	c.ready = true
	if c.state == nil {
		c.state = defaultProjectState()
	}
	c.mu.Unlock()
}

// InjectState replaces the persisted state.
func (c *Client) InjectState(st *ProjectState) {
	c.mu.Lock()
	c.state = st
	c.mu.Unlock()
}

// InjectIMChannel sets the IM channel and registers the HandleMessage callback.
func (c *Client) InjectIMChannel(p im.Channel) {
	c.imRun = p
}

// DefaultState returns a freshly initialised default state.
func DefaultState() *ProjectState {
	return defaultProjectState()
}
```

- [ ] **Step 2: Verify compile**

```bash
cd /d/Code/WheelMaker && go build ./internal/client/...
```

- [ ] **Step 3: Commit**

```bash
cd /d/Code/WheelMaker
git add internal/client/export_test.go
git commit -m "refactor(client): replace InjectSession with InjectForwarder in export_test"
git push
```

---

## Chunk 4: Delete `acp` dead code

### Task 9: Delete `acp` files and update tests

**Files:**
- Delete: `internal/acp/agent.go`, `acp/session.go`, `acp/prompt.go`, `acp/callbacks.go`, `acp/terminal.go`, `acp/agent_test.go`

- [ ] **Step 1: Delete the files**

```bash
cd /d/Code/WheelMaker
rm internal/acp/agent.go internal/acp/session.go internal/acp/prompt.go \
   internal/acp/callbacks.go internal/acp/terminal.go internal/acp/agent_test.go
```

- [ ] **Step 2: Verify acp compiles**

```bash
cd /d/Code/WheelMaker && go build ./internal/acp/...
```

Expected: no errors. If any symbol is referenced from deleted files, the error will point to the place that still imports it — fix those references (they should all be in `client` now).

- [ ] **Step 3: Verify full build**

```bash
cd /d/Code/WheelMaker && go build ./...
```

Fix any remaining references to deleted types (`acp.Agent`, `acp.Session`, `acp.InitMeta`, `acp.SessionMeta`, `acp.SwitchMode`, `acp.SwitchClean`, `acp.SwitchWithContext`, `backendHooks`).

- [ ] **Step 4: Update `hub.go` if it references deleted types**

Check `internal/hub/hub.go` for any reference to `acp.Agent` or `acp.Session`:

```bash
cd /d/Code/WheelMaker && grep -n "acp\.Agent\|acp\.Session\|acp\.Switch" internal/hub/hub.go
```

Update any found references.

- [ ] **Step 5: Commit**

```bash
cd /d/Code/WheelMaker
git add -A
git commit -m "refactor(acp): delete Agent/Session/prompt/callbacks/terminal — logic moved to client"
git push
```

---

## Chunk 5: Fix tests

### Task 10: Update `client/client_test.go`

The existing tests use `mockSession` (implements `acp.Session`) + `InjectSession`. These must be migrated to use the mock agent with `InjectForwarder` or a real in-memory ACP connection.

**Files:**
- Modify: `internal/client/client_test.go`

- [ ] **Step 1: Remove `mockSession` struct and all its methods**

Delete lines ~40-100 that define `mockSession` and its methods (`Prompt`, `Cancel`, `SetMode`, `SetConfigOption`, `AgentName`, `SessionID`).

- [ ] **Step 2: Replace `InjectSession` calls with `InjectForwarder`**

Find all `c.InjectSession(mock)` calls. For each test:
- If the test needs a controllable prompt response, use the in-memory mock agent from `backendmock`:
  ```go
  ag := backendmock.New()
  conn, _ := ag.Connect(context.Background())
  fwd := acp.NewForwarder(conn, func(ctx context.Context, msg acp.ForwardMessage) (acp.ForwardMessage, bool, error) {
      return msg, true, nil
  })
  fwd.SetCallbacks(c)
  c.InjectForwarder(fwd, "test-session-id")
  ```
- The mock server in `internal/agent/mock/mock_server.go` handles `initialize`, `session/new`, `session/prompt` and returns scripted responses.

- [ ] **Step 3: Update three local agent stubs (`minimalMockAgent`, `contextRejectMockAgent`, `failConnectAgent`)**

These already implement `agent.Agent` interface. Verify they still compile with the (unchanged) `agent.Agent` interface. No changes needed if the interface is unchanged.

- [ ] **Step 4: Run tests**

```bash
cd /d/Code/WheelMaker && go test ./internal/client/... -count=1 -v 2>&1 | head -60
```

Fix any failing tests. The mock server's session/prompt response format must match what `promptStream` expects.

- [ ] **Step 5: Commit**

```bash
cd /d/Code/WheelMaker
git add internal/client/client_test.go
git commit -m "test(client): migrate tests from mockSession/InjectSession to mock agent + InjectForwarder"
git push
```

---

## Chunk 6: Naming cleanup and docs

### Task 11: Rename test files in `agent` sub-packages

- [ ] **Step 1: Rename claude test files**

```bash
cd /d/Code/WheelMaker
git mv internal/agent/claude/backend_test.go internal/agent/claude/agent_test.go
git mv internal/agent/claude/backend_integration_test.go internal/agent/claude/agent_integration_test.go
```

- [ ] **Step 2: Rename codex test files**

```bash
git mv internal/agent/codex/backend_test.go internal/agent/codex/agent_test.go
git mv internal/agent/codex/backend_integration_test.go internal/agent/codex/agent_integration_test.go
```

- [ ] **Step 3: Rename mock test file**

```bash
git mv internal/agent/mock/backend_test.go internal/agent/mock/agent_test.go
```

- [ ] **Step 4: Update any `package` declarations inside renamed files if they reference "backend"**

```bash
cd /d/Code/WheelMaker
grep -rn "backend" internal/agent/ --include="*.go" | grep -v "_test.go:"
```

Fix any stale "backend" references in comments or identifiers.

- [ ] **Step 5: Compile and test**

```bash
cd /d/Code/WheelMaker && go build ./internal/agent/... && go test ./internal/agent/... -count=1
```

- [ ] **Step 6: Commit**

```bash
cd /d/Code/WheelMaker
git add -A
git commit -m "refactor(agent): rename backend_test.go → agent_test.go across claude/codex/mock"
git push
```

---

### Task 12: Update `acp` and `agent` package comments + `CLAUDE.md`

**Files:**
- Modify: `internal/acp/forwarder.go` (package comment)
- Modify: `internal/agent/agent.go` (package comment + any "backend" refs)
- Modify: `CLAUDE.md`

- [ ] **Step 1: Update `acp` package doc comment**

In `internal/acp/agent.go` is deleted. The package comment now lives in `internal/acp/conn.go` or a new `doc.go`. Check where the current package comment is:

```bash
cd /d/Code/WheelMaker && grep -n "^// Package acp" internal/acp/*.go
```

Update it to:
```go
// Package acp implements the ACP (Agent Client Protocol) transport layer.
// It provides Conn (JSON-RPC 2.0 over subprocess stdio), Forwarder (bidirectional
// message filter with typed outbound methods and ClientCallbacks dispatch),
// and all ACP protocol types.
```

- [ ] **Step 2: Update `internal/agent/agent.go` package comment**

```go
// Package agent defines the Agent interface for ACP-compatible CLI agents.
// An Agent is a stateless subprocess factory: Connect() starts a new binary
// and returns its acp.Conn. Per-agent protocol hooks (NormalizeParams,
// HandlePermission) are also provided through this interface.
```

Remove any "backend" references in comments.

- [ ] **Step 3: Update `CLAUDE.md` architecture section**

Replace the current architecture diagram and table with:

```markdown
## 架构层次

```
Hub (internal/hub/)          — 多 project 生命周期管理，读 config.json
  └─ client.Client           — 单 project 主控：命令路由、会话管理、idle 超时、状态持久化
       └─ acp.Forwarder      — ACP 协议封装：类型化出站方法、ClientCallbacks 分发、消息过滤
            └─ acp.Conn      — JSON-RPC 2.0 over stdio → CLI binary
```

## 包职责

| 包 | 职责 |
|----|------|
| `internal/hub/` | 读 `~/.wheelmaker/config.json`，为每个 project 创建 Client + IM |
| `internal/client/` | 主控：命令路由、会话生命周期（ensureReady/promptStream/switchAgent）、terminal 管理、实现 acp.ClientCallbacks |
| `internal/acp/` | 纯传输层：Conn（子进程 stdio）、Forwarder（类型化 ACP 方法 + ClientCallbacks 分发）、协议类型 |
| `internal/agent/claude/` | 启动 claude-agent-acp 子进程，返回 `*acp.Conn`；NormalizeParams/HandlePermission 钩子 |
| `internal/agent/codex/` | 启动 codex-acp 子进程，返回 `*acp.Conn`；同上 |
| `internal/im/console/` | Console IM：读 stdin，debug 模式打印所有 ACP JSON |
| `internal/im/feishu/` | 飞书 Bot IM channel |
| `internal/tools/` | 工具二进制路径解析（`bin/{GOOS}_{GOARCH}/`） |
```

- [ ] **Step 4: Commit**

```bash
cd /d/Code/WheelMaker
git add internal/acp/ internal/agent/agent.go CLAUDE.md
git commit -m "docs: update package comments and CLAUDE.md architecture for new client→forwarder→agent structure"
git push
```

---

## Final Verification

### Task 13: Full test run and acceptance criteria check

- [ ] **Step 1: Run all tests**

```bash
cd /d/Code/WheelMaker && go test ./... -count=1
```

Expected: all tests PASS.

- [ ] **Step 2: Verify no `acp.Agent` struct remains**

```bash
cd /d/Code/WheelMaker && grep -rn "acp\.Agent\b" internal/ --include="*.go"
```

Expected: no output.

- [ ] **Step 3: Verify no `backend` references remain**

```bash
cd /d/Code/WheelMaker && grep -rni "\bbackend\b" internal/ --include="*.go" | grep -v "_test.go" | grep -v "//.*backend"
```

Expected: no output (or only in unrelated test/comment strings).

- [ ] **Step 4: Verify ClientCallbacks is wired**

```bash
cd /d/Code/WheelMaker && grep -n "SetCallbacks\|ClientCallbacks" internal/ -r --include="*.go"
```

Expected: `SetCallbacks` called in `client.go ensureForwarder` and `switchAgent`; `ClientCallbacks` defined in `acp/handler.go`, implemented (`var _ acp.ClientCallbacks`) in `client/callbacks.go`.

- [ ] **Step 5: Smoke test with console IM**

If `~/.wheelmaker/config.json` is configured:
```bash
cd /d/Code/WheelMaker && go run ./cmd/wheelmaker/
```

Type a message, verify the agent responds. Type `/cancel`, verify cancellation works.

- [ ] **Step 6: Final commit**

```bash
cd /d/Code/WheelMaker
git add -A
git commit -m "chore: final cleanup — ACP refactor complete (client→forwarder→agent)"
git push
```
