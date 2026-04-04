# AgentV2 Conn Consolidation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the legacy `hub/acp + hub/agent` runtime stack with `hub/agentv2` while keeping Session boundary stable and making `Conn` transport-only (`Send/Notify/OnRequest`) and `Instance` ACP-typed.

**Architecture:** `Session` depends only on `agentv2.Instance`. `agentv2.Instance` owns ACP typed flows and agent-specific behavior. `agentv2.Conn` owns subprocess lifecycle, JSON-RPC transport, callback ingress routing, and ACP-session route maps (`activeMap`, `pendingMap`, `orphanBuffer`). ACP protocol types become single-source in `server/internal/protocol/acp.go`, with temporary aliases in `server/internal/hub/acp` during migration.

**Tech Stack:** Go 1.22+, existing WheelMaker server packages (`client`, `im`, `tools`), `modernc.org/sqlite` unchanged.

---

## Scope Check

This plan covers one subsystem: server runtime refactor for AgentV2 consolidation. IM adapters, registry protocol, and app/mobile code are out of scope.

## File Structure

| File | Action | Responsibility |
|---|---|---|
| `server/internal/protocol/acp.go` | Create | ACP protocol type single source (`clientSession` vs `acpSession` naming clarified in comments) |
| `server/internal/hub/acp/protocol_alias.go` | Create | Type aliases from legacy `hub/acp` to `internal/protocol` during transition |
| `server/internal/hub/agentv2/conn.go` | Create | Transport-only conn (`Send/Notify/OnRequest`), subprocess lifecycle, raw callback ingress |
| `server/internal/hub/agentv2/conn_routes.go` | Create | `activeMap/pendingMap/orphanBuffer` route state, epoch guards, replay TTL |
| `server/internal/hub/agentv2/callbacks.go` | Create | Raw callback bridge contracts between conn and instance |
| `server/internal/hub/agentv2/instance.go` | Create | ACP typed methods + decode/dispatch + bootstrap state (`connReady/initialized/acpSessionReady`) |
| `server/internal/hub/agentv2/factory.go` | Create | Build instance/conn and choose owned/shared policy |
| `server/internal/hub/agentv2/provider.go` | Create | Provider contract |
| `server/internal/hub/agentv2/provider_codex.go` | Create | Codex launch resolution migrated from old `hub/agent/codex` |
| `server/internal/hub/agentv2/provider_claude.go` | Create | Claude launch resolution migrated from old `hub/agent/claude` |
| `server/internal/hub/agentv2/provider_copilot.go` | Create | Copilot launch resolution migrated from old `hub/agent/copilot` |
| `server/internal/hub/agentv2/conn_test.go` | Create | Transport tests (`Send/Notify/OnRequest`) |
| `server/internal/hub/agentv2/conn_routes_test.go` | Create | route map tests (`load` pending->active/rollback, orphan replay, epoch stale drop) |
| `server/internal/hub/agentv2/instance_test.go` | Create | bootstrap and typed ACP behavior tests |
| `server/internal/hub/client/session_type.go` | Modify | Session instance type switched to `agentv2.Instance` |
| `server/internal/hub/client/session.go` | Modify | typed ACP calls routed through `agentv2.Instance`; bootstrap semantics for `/new` and `/load` |
| `server/internal/hub/client/lifecycle.go` | Modify | switch/create instance via `agentv2.Factory` |
| `server/internal/hub/client/agent_factory.go` | Modify | replace legacy factory internals with `agentv2` provider/factory wrappers |
| `server/internal/hub/client/client.go` | Modify | registration path supports agentv2 providers and keeps old API compatibility |
| `server/internal/hub/hub.go` | Modify | register codex/claude/copilot through `agentv2` |
| `server/internal/hub/client/client_test.go` | Modify | `/new` and `/load` behavior when ACP session not ready |
| `server/internal/hub/client/multi_session_test.go` | Modify | shared conn dispatch against `agentv2` mappings |
| `server/internal/hub/acp/forwarder.go` | Delete | removed after parity |
| `server/internal/hub/agent/agent.go` | Delete | migrated into agentv2 |
| `server/internal/hub/agent/codex/agent.go` | Delete | migrated into `provider_codex.go` |
| `server/internal/hub/agent/claude/agent.go` | Delete | migrated into `provider_claude.go` |
| `server/internal/hub/agent/copilot/agent.go` | Delete | migrated into `provider_copilot.go` |

---

### Task 1: Freeze ACP protocol source in `internal/protocol`

**Files:**
- Create: `server/internal/protocol/acp.go`
- Create: `server/internal/hub/acp/protocol_alias.go`
- Test: `server/internal/protocol/acp_test.go`

- [ ] **Step 1: Write failing protocol parity test**

```go
package protocol

import (
	"encoding/json"
	"testing"
)

func TestSessionUpdateParams_JSONParity(t *testing.T) {
	in := SessionUpdateParams{SessionID: "s1"}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out SessionUpdateParams
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.SessionID != "s1" {
		t.Fatalf("session id = %q", out.SessionID)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./server/internal/protocol -run TestSessionUpdateParams_JSONParity -v`
Expected: FAIL with undefined ACP types.

- [ ] **Step 3: Add `acp.go` and legacy alias file**

```go
// server/internal/hub/acp/protocol_alias.go
package acp

import p "github.com/swm8023/wheelmaker/internal/protocol"

type SessionUpdateParams = p.SessionUpdateParams
type SessionNewParams = p.SessionNewParams
type SessionNewResult = p.SessionNewResult
```

```go
// server/internal/protocol/acp.go
package protocol

const (
	MethodInitialize = "initialize"
	MethodSessionNew = "session/new"
)

type SessionUpdateParams struct {
	SessionID string `json:"sessionId"`
}
```

- [ ] **Step 4: Run protocol tests**

Run: `go test ./server/internal/protocol ./server/internal/hub/acp -run Protocol -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add server/internal/protocol/acp.go server/internal/protocol/acp_test.go server/internal/hub/acp/protocol_alias.go
git commit -m "refactor(protocol): add ACP single-source types with legacy aliases"
```

### Task 2: Build transport-only `agentv2.Conn`

**Files:**
- Create: `server/internal/hub/agentv2/conn.go`
- Test: `server/internal/hub/agentv2/conn_test.go`

- [ ] **Step 1: Write failing transport contract tests**

```go
func TestConn_SendAndNotify(t *testing.T) {
	c := newFakeConn(t)
	var out map[string]any
	if err := c.Send(context.Background(), "initialize", map[string]any{"x": 1}, &out); err != nil {
		t.Fatalf("send: %v", err)
	}
	if err := c.Notify("session/cancel", map[string]any{"sessionId": "s1"}); err != nil {
		t.Fatalf("notify: %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./server/internal/hub/agentv2 -run TestConn_SendAndNotify -v`
Expected: FAIL (`agentv2` package missing).

- [ ] **Step 3: Implement transport-only conn interface**

```go
package agentv2

type RequestHandler func(ctx context.Context, method string, params json.RawMessage, noResponse bool) (any, error)

type Conn interface {
	Send(ctx context.Context, method string, params any, result any) error
	Notify(method string, params any) error
	OnRequest(h RequestHandler)
	Close() error
}
```

- [ ] **Step 4: Run `agentv2` tests**

Run: `go test ./server/internal/hub/agentv2 -run TestConn_SendAndNotify -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add server/internal/hub/agentv2/conn.go server/internal/hub/agentv2/conn_test.go
git commit -m "feat(agentv2): add transport-only conn contract"
```

### Task 3: Add ACP-session route guarantees (`active/pending/orphan`)

**Files:**
- Create: `server/internal/hub/agentv2/conn_routes.go`
- Test: `server/internal/hub/agentv2/conn_routes_test.go`

- [ ] **Step 1: Write failing route guarantee tests**

```go
func TestRoutes_LoadPendingPromotesToActive(t *testing.T) {
	r := newRouteState()
	tok := r.beginLoad("acp-1", "inst-A", 3)
	r.commitLoad(tok)
	if got := r.lookupActive("acp-1"); got == nil {
		t.Fatal("active route missing")
	}
}

func TestRoutes_LoadFailureRollsBack(t *testing.T) {
	r := newRouteState()
	tok := r.beginLoad("acp-1", "inst-A", 3)
	r.rollbackLoad(tok)
	if got := r.lookupActive("acp-1"); got != nil {
		t.Fatal("unexpected active route")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./server/internal/hub/agentv2 -run TestRoutes_ -v`
Expected: FAIL with undefined route state.

- [ ] **Step 3: Implement route maps and epoch guard**

```go
type activeBinding struct { instanceKey string; epoch uint64 }
type pendingBinding struct { token string; targetACPSessionID string; instanceKey string; epoch uint64 }

type routeState struct {
	active map[string]activeBinding
	pending map[string]pendingBinding
	orphan map[string][]protocol.Update
}
```

- [ ] **Step 4: Add orphan replay TTL behavior and tests**

```go
func (r *routeState) bufferOrphan(acpSessionID string, u protocol.Update, now time.Time) {
    r.orphan[acpSessionID] = append(r.orphan[acpSessionID], u)
    r.pruneOrphans(now)
}
func (r *routeState) replayOrphans(acpSessionID string) []protocol.Update {
    u := r.orphan[acpSessionID]
    delete(r.orphan, acpSessionID)
    return u
}
```

Run: `go test ./server/internal/hub/agentv2 -run TestRoutes_ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add server/internal/hub/agentv2/conn_routes.go server/internal/hub/agentv2/conn_routes_test.go
git commit -m "feat(agentv2): add ACP session route state with pending and orphan handling"
```

### Task 4: Implement typed ACP in `agentv2.Instance`

**Files:**
- Create: `server/internal/hub/agentv2/callbacks.go`
- Create: `server/internal/hub/agentv2/instance.go`
- Test: `server/internal/hub/agentv2/instance_test.go`

- [ ] **Step 1: Write failing instance bootstrap test**

```go
func TestInstance_NewAndLoadWithoutACPReady(t *testing.T) {
	inst := newTestInstance(t)
	if _, err := inst.SessionNew(context.Background(), protocol.SessionNewParams{CWD: "."}); err != nil {
		t.Fatalf("session new: %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./server/internal/hub/agentv2 -run TestInstance_NewAndLoadWithoutACPReady -v`
Expected: FAIL (`Instance` methods missing).

- [ ] **Step 3: Implement typed methods on instance using conn raw APIs**

```go
type Instance interface {
	Name() string
	Initialize(ctx context.Context, p protocol.InitializeParams) (protocol.InitializeResult, error)
	SessionNew(ctx context.Context, p protocol.SessionNewParams) (protocol.SessionNewResult, error)
	SessionLoad(ctx context.Context, p protocol.SessionLoadParams) (protocol.SessionLoadResult, error)
	SessionPrompt(ctx context.Context, p protocol.SessionPromptParams) (protocol.SessionPromptResult, error)
	SessionCancel(acpSessionID string) error
}
```

- [ ] **Step 4: Implement inbound decode/dispatch in instance callback bridge**

```go
type RawCallbackBridge interface {
	HandleInbound(ctx context.Context, method string, params json.RawMessage, noResponse bool) (any, error)
}
```

Run: `go test ./server/internal/hub/agentv2 -run TestInstance_ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add server/internal/hub/agentv2/callbacks.go server/internal/hub/agentv2/instance.go server/internal/hub/agentv2/instance_test.go
git commit -m "feat(agentv2): move typed ACP and inbound dispatch to instance layer"
```

### Task 5: Migrate old `hub/agent` launch logic into `agentv2`

**Files:**
- Create: `server/internal/hub/agentv2/provider.go`
- Create: `server/internal/hub/agentv2/provider_codex.go`
- Create: `server/internal/hub/agentv2/provider_claude.go`
- Create: `server/internal/hub/agentv2/provider_copilot.go`
- Test: `server/internal/hub/agentv2/provider_test.go`

- [ ] **Step 1: Write failing provider resolution tests**

```go
func TestCodexProvider_UsesNpxFallback(t *testing.T) {
	p := NewCodexProvider(CodexProviderConfig{})
	_, args, _, err := p.LaunchSpec()
	if err != nil { t.Fatalf("launch spec: %v", err) }
	if len(args) == 0 { t.Fatal("expected fallback args") }
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./server/internal/hub/agentv2 -run Test.*Provider -v`
Expected: FAIL.

- [ ] **Step 3: Implement provider files by migrating existing logic**

```go
// provider.go
package agentv2

type Provider interface {
	Name() string
	LaunchSpec() (exe string, args []string, env []string, err error)
}
```

```go
// provider_copilot.go
func (p *CopilotProvider) LaunchSpec() (string, []string, []string, error) {
	exe, err := resolveExe(p.cfg.ExePath)
	if err != nil { return "", nil, nil, err }
	return exe, []string{"--acp", "--stdio"}, buildEnv(p.cfg.Env), nil
}
```

- [ ] **Step 4: Run provider tests**

Run: `go test ./server/internal/hub/agentv2 -run Test.*Provider -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add server/internal/hub/agentv2/provider*.go server/internal/hub/agentv2/provider_test.go
git commit -m "refactor(agentv2): migrate codex claude copilot provider launch logic"
```

### Task 6: Integrate `agentv2` into Client and Hub

**Files:**
- Modify: `server/internal/hub/client/agent_factory.go`
- Modify: `server/internal/hub/client/session_type.go`
- Modify: `server/internal/hub/client/session.go`
- Modify: `server/internal/hub/client/lifecycle.go`
- Modify: `server/internal/hub/client/client.go`
- Modify: `server/internal/hub/hub.go`
- Test: `server/internal/hub/client/client_test.go`
- Test: `server/internal/hub/client/multi_session_test.go`

- [ ] **Step 1: Write failing integration test for `/load` before ACP session ready**

```go
func TestHandleLoadCommand_BeforeACPReady(t *testing.T) {
	c := newClientForTest(t)
	c.HandleMessage(im.Message{ChatID: "c1", Text: "/load acp-1"})
	msgs := c.imBridge.DebugMessages()
    if len(msgs) == 0 {
        t.Fatal("expected feedback message")
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./server/internal/hub/client -run TestHandleLoadCommand_BeforeACPReady -v`
Expected: FAIL.

- [ ] **Step 3: Switch Session instance type and factory wiring**

```go
// session_type.go
type Session struct {
	instance agentv2.Instance
}
```

```go
// agent_factory.go
func wrapLegacyFactory(name string, fn AgentFactory) AgentFactoryV2 {
	return agentv2.NewLegacyAdapter(name, fn)
}
```

- [ ] **Step 4: Update hub registration to use agentv2 providers**

```go
c.RegisterAgentV2("codex", agentv2.NewProviderFactory(agentv2.NewCodexProvider(agentv2.CodexProviderConfig{})))
c.RegisterAgentV2("claude", agentv2.NewProviderFactory(agentv2.NewClaudeProvider(agentv2.ClaudeProviderConfig{})))
c.RegisterAgentV2("copilot", agentv2.NewProviderFactory(agentv2.NewCopilotProvider(agentv2.CopilotProviderConfig{})))
```

Run: `go test ./server/internal/hub/client ./server/internal/hub -run TestHandleLoadCommand_BeforeACPReady -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add server/internal/hub/client/agent_factory.go server/internal/hub/client/session_type.go server/internal/hub/client/session.go server/internal/hub/client/lifecycle.go server/internal/hub/client/client.go server/internal/hub/hub.go server/internal/hub/client/client_test.go server/internal/hub/client/multi_session_test.go
git commit -m "refactor(client): switch runtime wiring to agentv2 instance and provider factory"
```

### Task 7: Remove legacy forwarder and old agent package

**Files:**
- Delete: `server/internal/hub/acp/forwarder.go`
- Delete: `server/internal/hub/agent/agent.go`
- Delete: `server/internal/hub/agent/codex/agent.go`
- Delete: `server/internal/hub/agent/claude/agent.go`
- Delete: `server/internal/hub/agent/copilot/agent.go`
- Modify: tests importing deleted packages

- [ ] **Step 1: Write failing anti-regression test ensuring no forwarder import path remains**
```go
func TestNoLegacyForwarderDependency(t *testing.T) {
	cmd := exec.Command("go", "list", "-deps", "./server/internal/hub/...")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list failed: %v, out=%s", err, string(out))
	}
	if strings.Contains(string(out), "internal/hub/acp") {
		t.Fatalf("legacy acp package still in deps: %s", string(out))
	}
}
```
- [ ] **Step 2: Run targeted build/test to capture breakages**

Run: `go test ./server/internal/hub/... -run TestNoLegacyForwarderDependency -v`
Expected: FAIL until imports and references are removed.

- [ ] **Step 3: Delete legacy files and fix imports**

```bash
git rm server/internal/hub/acp/forwarder.go server/internal/hub/agent/agent.go server/internal/hub/agent/codex/agent.go server/internal/hub/agent/claude/agent.go server/internal/hub/agent/copilot/agent.go
```

- [ ] **Step 4: Run full hub test suite**

Run: `go test ./server/internal/hub/... -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "refactor(server): remove legacy forwarder and old agent package"
```

### Task 8: Final validation and docs sync

**Files:**
- Modify: `server/CLAUDE.md`
- Modify: `docs/architecture-3.0.md`

- [ ] **Step 1: Update docs to reflect final topology**

```md
Session -> agentv2.Instance -> agentv2.Conn -> subprocess
```

- [ ] **Step 2: Run full server test suite**

Run: `go test ./server/...`
Expected: PASS.

- [ ] **Step 3: Run repository completion gate sequence**

Run:

```bash
git add -A
git commit -m "docs: finalize agentv2 runtime architecture references"
git push origin main
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/signal_update_now.ps1 -DelaySeconds 30
```

Expected: all commands succeed.

- [ ] **Step 4: Commit (if docs changed after validation)**

```bash
git add server/CLAUDE.md docs/architecture-3.0.md
git commit -m "docs: align architecture docs with agentv2 consolidation"
```

## Self-Review Notes

1. Spec coverage: includes protocol source move, conn/instance boundary, bootstrap states, 3-map routing guarantees, old agent merge, and legacy deletion.
2. Placeholder scan: no unresolved markers detected.
3. Type consistency: `acpSessionID` naming is consistent across conn/instance/route mapping tasks.



