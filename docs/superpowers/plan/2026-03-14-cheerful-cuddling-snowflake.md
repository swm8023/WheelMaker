# WheelMaker Iteration 2 â€” Multi-Project Hub

## Context

After the 2026-03-14 architecture redesign (hubâ†’client rename, adapter/agent layering), the first working version was reviewed and produced 6 improvement items:

1. config.json with project concept (name, im config, client config)
2. Hub managing multiple projects simultaneously
3. Console IM type for local testing
4. Lazy agent loading + 30-min idle timeout
5. `/` prefix is not always a command (code starting with `/` should fall through)
6. Console debug flag to print all ACP JSON exchanges

The current implementation has a hardcoded `codex` adapter in `main.go`, a single `client.Client`, and a flat `state.json`. This iteration replaces the entire CLI entrypoint with a project/hub model.

---

## Approach

Use **Hub as a new top-level package** (`internal/hub/`). Each project gets its own `client.Client` instance with its own IM adapter and agent. Hub orchestrates lifecycle and concurrency. `client.Client` receives targeted changes (lazy load, idle timeout, command dispatch fix). `acp.Conn` gets an optional debug logger.

---

## Package Changes

### New files

| File | Purpose |
|------|---------|
| `internal/hub/hub.go` | Hub struct: reads config, manages project clients |
| `internal/hub/config.go` | Config/ProjectConfig/IMConfig/ClientConf types + LoadConfig() |
| `internal/im/console/console.go` | Console IM adapter (stdin reader) |

### Modified files

| File | Change |
|------|--------|
| `internal/client/client.go` | Lazy agent init, idle timer, command dispatch fix, accept debug writer |
| `internal/client/state.go` | State keyed by project name: `map[string]*ProjectState` |
| `internal/client/store.go` | Migration: old flat state â†’ `projects["default"]` |
| `internal/agent/provider/connect.go` | Optional debug `io.Writer` for all JSON in/out |
| `cmd/wheelmaker/main.go` | Rewrite: load config â†’ create Hub â†’ Start â†’ Run |

### Unchanged

- `internal/agent/` (agent.go, session.go, prompt.go, callbacks.go, etc.)
- `internal/agent/provider/` (adapter interface + codex adapter)
- `internal/im/im.go` (IM interface)
- `internal/tools/`

---

## config.json

Location: `~/.wheelmaker/config.json`

```json
{
  "projects": [
    {
      "name": "local-dev",
      "im": {
        "type": "console",
        "debug": true
      },
      "client": {
        "adapter": "codex",
        "path": "/path/to/working/directory"
      }
    },
    {
      "name": "feishu-prod",
      "im": {
        "type": "feishu",
        "appID": "cli_xxx",
        "appSecret": "yyy"
      },
      "client": {
        "adapter": "codex",
        "path": "/path/to/other/project"
      }
    }
  ],
  "feishu": {
    "verificationToken": "zzz"
  }
}
```

**Rules:**
- `im.type`: `"console"` or `"feishu"`
- `im.debug`: console only; enables ACP JSON debug logging to stderr
- `client.path`: working directory for the agent session (cwd)
- `client.adapter`: adapter name (must be registered)
- At most **one** console IM project per Hub (validated at startup)

---

## 1. internal/hub/config.go

```go
package hub

type Config struct {
    Projects []ProjectConfig `json:"projects"`
    Feishu   FeishuConfig    `json:"feishu,omitempty"`
}

type ProjectConfig struct {
    Name   string     `json:"name"`
    IM     IMConfig   `json:"im"`
    Client ClientConf `json:"client"`
}

type IMConfig struct {
    Type      string `json:"type"`
    AppID     string `json:"appID,omitempty"`
    AppSecret string `json:"appSecret,omitempty"`
    Debug     bool   `json:"debug,omitempty"`
}

type ClientConf struct {
    Adapter string `json:"adapter"`
    Path    string `json:"path"`
}

type FeishuConfig struct {
    VerificationToken string `json:"verificationToken,omitempty"`
}

func LoadConfig(path string) (*Config, error)  // os.ReadFile + json.Unmarshal; returns clear error if file missing
```

---

## 2. internal/hub/hub.go

```go
package hub

type Hub struct {
    cfg     *Config
    store   client.Store
    clients []*client.Client
}

func New(cfg *Config, store client.Store) *Hub

// Start validates config, creates one client.Client per project,
// registers adapters, calls client.Start() for each.
// Returns error if >1 console IM project, or any client.Start() fails fatally.
func (h *Hub) Start(ctx context.Context) error

// Run starts each client.Run(ctx) in a goroutine via errgroup.
// Blocks until ctx is done or all clients exit.
func (h *Hub) Run(ctx context.Context) error

// Close calls client.Close() for all clients.
func (h *Hub) Close() error
```

**Hub.Start() implementation notes:**
- Validates: at most one `im.type == "console"`
- Per project:
  - Creates `im.Adapter` from config (console or feishu)
  - Creates `client.Client(store, imAdapter, projectName)`
  - Calls `c.RegisterProvider("codex", codex.NewAdapter(...))`
  - Calls `c.Start(ctx)` (now only loads state, no eager connect)

---

## 3. internal/im/console/console.go

```go
package console

// ConsoleIM implements im.provider.
// Reads stdin line by line, dispatches each line as an im.Message.
// Prints responses with a project-name prefix.
type ConsoleIM struct {
    projectName string
    debug       bool
    handler     im.MessageHandler
}

func New(projectName string, debug bool) *ConsoleIM

func (c *ConsoleIM) OnMessage(handler im.MessageHandler)
func (c *ConsoleIM) SendText(chatID, text string) error   // prints to stdout with prefix
func (c *ConsoleIM) SendCard(chatID string, card im.Card) error  // prints JSON to stdout
func (c *ConsoleIM) SendReaction(messageID, emoji string) error  // no-op or print
func (c *ConsoleIM) Run(ctx context.Context) error        // stdin read loop

// Run() reads lines from os.Stdin. Each non-empty line becomes:
//   im.Message{ChatID: projectName, MessageID: uuid, Text: line}
// Prompt: prints "[projectName] > " before reading.
// Exits when ctx is done or stdin is closed.
```

**Debug logging**: `ConsoleIM.Debug() bool` accessor. Hub passes this flag to `client.Client` which passes to `acp.Conn` as a debug writer.

---

## 4. internal/agent/provider/connect.go â€” debug logging

```go
// Add to Conn struct:
debugLog io.Writer  // nil = no logging; set to os.Stderr for debug mode

// Add constructor option:
func (c *Conn) SetDebugLogger(w io.Writer)

// In read loop and Send():
// if c.debugLog != nil { fmt.Fprintf(c.debugLog, "â†’ %s\n", rawJSON) }
// if c.debugLog != nil { fmt.Fprintf(c.debugLog, "â† %s\n", rawJSON) }
```

The debug writer is injected by `client.Client` after `provider.Connect()` returns the `*acp.Conn`, using the debug flag from the console IM config.

---

## 5. internal/client/client.go â€” changes

### 5a. Constructor signature change

```go
// Add projectName and cwd parameters
// projectName: used as state key in FileState.Projects (immutable after creation)
// cwd: working directory for agent sessions (from config client.path)
func New(store Store, im im.Adapter, projectName string, cwd string) *Client
```

**Test impact**: `client_test.go` and `export_test.go` must be updated:
- All `client.New(store, nil)` â†’ `client.New(store, nil, "test", "/tmp")`
- `export_test.go` helper functions (InjectSession, InjectState, InjectIMAdapter) updated to match new struct fields
- No semantic change to test logic; purely mechanical signature updates

### 5b. Lazy agent initialization

**Remove** the eager `provider.Connect()` call from `Start()`.
`Start()` now only calls `loadState()`.

**Also remove** the old stdin loop from `Run()` â€” this moves entirely to `internal/im/console/console.go`. `Run()` with a non-nil `imRun` calls `imRun.Run(ctx)` (unchanged); with nil IM, it now returns an error ("no IM adapter configured, add a console project to config.json").

Add `ensureAgent(ctx context.Context) error` (private):
```
ensureAgent:
  // called under mu lock from handlePrompt / switchAdapter
  if c.session != nil { return nil }  // already active
  fac = c.providerFacs[c.state.ActiveAdapter]
  if fac == nil { return errors.New("no adapter registered") }
  conn, err = fac(c.state.Adapters[c.state.ActiveAdapter]).Connect(ctx)
  if err { return err }
  if c.debugLog != nil { conn.SetDebugLogger(c.debugLog) }  // inject debug logger
  c.ag = agent.NewWithSessionID(c.state.ActiveAdapter, conn, c.cwd,
             c.state.SessionIDs[c.state.ActiveAdapter])
  c.session = c.ag
  c.resetIdleTimer()
  return nil
```

`handlePrompt()` calls `ensureAgent()` before proceeding (inside mu lock).

**Debug logger field**: add `debugLog io.Writer` to `Client` struct. Hub sets it via `c.SetDebugLogger(os.Stderr)` when the project's console IM has `debug: true`. This is stored in the Client and applied at `ensureAgent()` time (and on every subsequent `switchAdapter` connect).

`switchAdapter` also calls `resetIdleTimer()` after successful connect (since it creates a new conn directly, not via `ensureAgent`).

### 5c. Idle timeout (30 minutes)

```go
// Add to Client struct:
idleTimer *time.Timer

func (c *Client) resetIdleTimer() {
    // Must be called under mu lock OR from ensureAgent (already under lock)
    if c.idleTimer != nil { c.idleTimer.Stop() }
    c.idleTimer = time.AfterFunc(30*time.Minute, c.idleClose)
}

func (c *Client) idleClose() {
    // Fired from timer goroutine â€” must acquire promptMu first to prevent
    // closing agent mid-prompt, then mu for state/session cleanup.
    c.promptMu.Lock()
    defer c.promptMu.Unlock()
    c.mu.Lock()
    defer c.mu.Unlock()
    if c.session == nil { return }
    if c.state.SessionIDs == nil { c.state.SessionIDs = map[string]string{} }
    c.state.SessionIDs[c.state.ActiveAdapter] = c.ag.SessionID()
    c.store.Save(c.state)
    c.ag.Close()
    c.session = nil
    c.ag = nil
}
```

`resetIdleTimer()` is called:
- In `ensureAgent()` after connect (under mu lock â€” safe, AfterFunc goroutine not yet running)
- In `handlePrompt()` before/after prompt send (resets the 30-min countdown)
- In `switchAdapter()` after successful connect

### 5d. Command dispatch fix

```go
// Replace HasPrefix check with explicit command matching:
func parseCommand(text string) (cmd, args string, ok bool) {
    parts := strings.Fields(text)
    if len(parts) == 0 { return }
    switch parts[0] {
    case "/use", "/cancel", "/status":
        return parts[0], strings.Join(parts[1:], " "), true
    }
    return  // not a known command â†’ ok=false â†’ treat as message
}
```

`HandleMessage` calls `parseCommand`; if `!ok`, goes directly to `handlePrompt`.

---

## 6. internal/client/state.go â€” multi-project state

```go
// New shape (top-level state file):
type FileState struct {
    Projects map[string]*ProjectState `json:"projects"`
}

type ProjectState struct {
    ActiveAdapter string            `json:"activeAdapter"`
    SessionIDs    map[string]string `json:"session_ids"`
}
```

`client.Client` only reads/writes its own `projectName` entry in `FileState.Projects`.

**Exported type `State`** remains as a type alias or is renamed to `ProjectState` â€” the exported test helper `InjectState` must be updated to accept `*ProjectState`.

### Migration (store.go)

```
Load() â€” detection logic:
  raw = readFile() as map[string]json.RawMessage
  if "projects" key present â†’ new format, unmarshal to FileState
  else if "activeAdapter" or "active_agent" key present â†’ old flat state, migrate:
      ps = ProjectState{}
      ps.ActiveAdapter = raw["activeAdapter"] or raw["active_agent"]
      ps.SessionIDs    = raw["session_ids"] or raw["acp_session_ids"]
      // Note: AdapterConfig (exePath, env) is no longer stored in state â€”
      // adapter binary path is auto-resolved; per-project config is in config.json
      FileState.Projects["default"] = &ps

  return FileState (client reads its own projectName entry, creates if missing)
```

Save() always writes the new FileState format (never writes old keys).

---

## 7. cmd/wheelmaker/main.go â€” rewrite

```go
func run() error {
    home, _ := os.UserHomeDir()
    cfgPath   := filepath.Join(home, ".wheelmaker", "config.json")
    statePath := filepath.Join(home, ".wheelmaker", "state.json")

    cfg, err := hub.LoadConfig(cfgPath)
    if err != nil {
        return fmt.Errorf("cannot load config.json at %s: %w\n\nCreate one based on config.example.json in the project root.", cfgPath, err)
    }

    store := client.NewJSONStore(statePath)
    h := hub.New(cfg, store)

    ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
    defer stop()

    if err := h.Start(ctx); err != nil { return err }
    defer h.Close()     // called after Run() returns; closes all project clients
    return h.Run(ctx)
}
```

**Hub.Run() error handling**: use `errgroup` but with project-level error isolation â€” each project goroutine logs errors and continues; only a ctx cancellation (Ctrl-C) or all-projects-failed terminates Hub.Run(). Individual project errors are logged to stderr with project name prefix.

**Hub.Close()**: called by `defer` in main after `Run()` returns (from context cancellation). Iterates all clients and calls `c.Close()`.

Add `config.example.json` to project root for reference.

## 8. Feishu config threading

The top-level `feishu` block in config.json contains shared settings (e.g., `verificationToken`) needed by the Feishu adapter alongside per-project credentials (`appID`, `appSecret`).

Hub.Start() creates the Feishu IM adapter for each feishu-type project by combining:
- Per-project: `project.IM.AppID`, `project.IM.AppSecret`
- Shared: `cfg.Feishu.VerificationToken` (passed directly to `feishu.New(...)`)

This is handled inside `Hub.Start()` which has access to the full `*Config`.

## 9. Console MessageID

No UUID library required. Use:
```go
MessageID: fmt.Sprintf("console-%d", time.Now().UnixNano())
```

---

## Data Flow After Change

```
main.go
  â†’ hub.LoadConfig(~/.wheelmaker/config.json)
  â†’ hub.New(cfg, store)
  â†’ hub.Start(ctx)
      â†’ per project: create IM adapter (console/feishu)
      â†’ per project: client.New(store, imAdapter, projectName)
      â†’ per project: c.RegisterProvider("codex", codex.NewAdapter(...))
      â†’ per project: c.Start(ctx)   â† only loads state, no connect
  â†’ hub.Run(ctx)
      â†’ errgroup: per project: c.Run(ctx)
          â†’ console IM: reads stdin, dispatches messages
          â†’ feishu IM: webhook/polling loop
          â†’ c.HandleMessage(msg)
              â†’ ensureAgent() if no active session  â† lazy
              â†’ resetIdleTimer()
              â†’ handleCommand() or handlePrompt()
```

---

## State File Example

```json
{
  "projects": {
    "local-dev": {
      "activeAdapter": "codex",
      "session_ids": { "codex": "sess_abc123" }
    },
    "feishu-prod": {
      "activeAdapter": "codex",
      "session_ids": { "codex": "sess_def456" }
    }
  }
}
```

---

## Verification

```bash
# 1. Build
go build ./cmd/wheelmaker/

# 2. No config â†’ clear error message
wheelmaker
# Expected: "cannot load config.json at ~/.wheelmaker/config.json: ..."

# 3. Console IM project
cat > ~/.wheelmaker/config.json << 'EOF'
{
  "projects": [
    {
      "name": "test",
      "im": { "type": "console", "debug": true },
      "client": { "adapter": "codex", "path": "/tmp" }
    }
  ]
}
EOF
wheelmaker
# Expected: "[test] > " prompt appears
# Type: hello world   â†’ agent responds
# Type: /status       â†’ shows adapter + session ID
# Type: /cancel       â†’ cancels in-flight prompt
# Type: /foo bar      â†’ treated as message (not command error)
# Type: // some code  â†’ treated as message

# 4. Idle timeout (can shorten to 5s for test)
# Send a message, wait 30m â†’ agent closes; send another â†’ session/load restores

# 5. Debug mode
# With debug: true â†’ stderr shows all ACP JSON {"jsonrpc":"2.0",...}

# 6. Multi-project (feishu + console together)
# Start with 2 projects â†’ both start, console reads stdin, feishu awaits webhook

# 7. Run existing tests
go test ./internal/client/...
go test ./internal/agent/...
go test ./internal/hub/...
```

---

## Implementation Order

1. `internal/hub/config.go` â€” Config types + LoadConfig()
2. `internal/agent/provider/connect.go` â€” add SetDebugLogger()
3. `internal/im/console/console.go` â€” Console IM adapter
4. `internal/client/state.go` + `store.go` â€” multi-project state + migration
5. `internal/client/client.go` â€” lazy init + idle timer + command fix
6. `internal/hub/hub.go` â€” Hub orchestrator
7. `cmd/wheelmaker/main.go` â€” rewrite entrypoint
8. `config.example.json` â€” example config in project root






