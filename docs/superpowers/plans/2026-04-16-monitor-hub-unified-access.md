# Monitor Hub Unified Access Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a unified monitor control plane that works with and without registry, introduces `role=monitor`, supports `monitor.*` methods by `hubId`, and keeps the existing dashboard layout with a new titlebar hub selector.

**Architecture:** Introduce a shared monitor capability layer (`internal/monitorcore`) and route all monitor operations through a transport abstraction. When registry is enabled, requests flow through `monitor-backend -> registry -> hub reporter`; when absent, monitor backend talks to local hub reporter directly with the same envelope contracts. Registry gets a dedicated `monitor` role with scoped method whitelist and ACL-ready routing by `hubId`.

**Tech Stack:** Go 1.x, Gorilla WebSocket, existing WheelMaker registry protocol envelope model, existing monitor dashboard HTML/JS.

---

### Task 1: Extend Protocol Model for `role=monitor` and `monitor.*` Payloads

**Files:**
- Modify: `server/internal/protocol/registry.go`
- Test: `server/internal/registry/server_test.go`

- [ ] **Step 1: Write the failing protocol/handshake test for monitor role**

```go
func TestServerConnectInit_AllowsMonitorRole(t *testing.T) {
    // Arrange: start registry test server and connect ws client
    // Act: send connect.init with role=monitor
    // Assert: response type=response and principal.role == "monitor"
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd server; go test ./internal/registry -run TestServerConnectInit_AllowsMonitorRole -v`
Expected: FAIL with invalid role validation (`role must be hub or client`).

- [ ] **Step 3: Add protocol fields/types for monitor methods**

```go
type MonitorHubRefPayload struct {
    HubID string `json:"hubId"`
}

type MonitorActionPayload struct {
    HubID  string `json:"hubId"`
    Action string `json:"action"`
}

type MonitorLogPayload struct {
    HubID string `json:"hubId"`
    File  string `json:"file,omitempty"`
    Level string `json:"level,omitempty"`
    Tail  int    `json:"tail,omitempty"`
}
```

- [ ] **Step 4: Update registry handshake role validation to include `monitor`**

```go
if role != "hub" && role != "client" && role != "monitor" {
    _ = s.writeError(peer, in.RequestID, in.Method, codeInvalidArgument, "role must be hub, client, or monitor", nil)
    return true
}
```

- [ ] **Step 5: Run focused tests to verify pass**

Run: `cd server; go test ./internal/registry -run TestServerConnectInit_AllowsMonitorRole -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add server/internal/protocol/registry.go server/internal/registry/server_test.go
git commit -m "feat: add monitor role and monitor protocol payload types"
```

### Task 2: Introduce Shared Monitor Capability Layer (`monitorcore`)

**Files:**
- Create: `server/internal/monitorcore/core.go`
- Create: `server/internal/monitorcore/core_test.go`
- Modify: `server/cmd/wheelmaker-monitor/monitor.go`
- Test: `server/cmd/wheelmaker-monitor/monitor_test.go`

- [ ] **Step 1: Write failing tests for core status/log/db/action APIs**

```go
func TestCoreGetStatus(t *testing.T) {}
func TestCoreGetLogs_NormalizesFileAndTail(t *testing.T) {}
func TestCoreGetDBTables_NoDBReturnsErrorResult(t *testing.T) {}
func TestCoreAction_UnsupportedAction(t *testing.T) {}
```

- [ ] **Step 2: Run tests to verify failures**

Run: `cd server; go test ./internal/monitorcore -v`
Expected: FAIL because package/file not implemented.

- [ ] **Step 3: Implement core service with monitor-compatible result structs**

```go
type Core struct {
    BaseDir string
}

func New(baseDir string) *Core { return &Core{BaseDir: baseDir} }
func (c *Core) GetServiceStatus() (*ServiceStatus, error) { /* moved from monitor.go */ }
func (c *Core) GetLogs(file, level string, tail int) (*LogResult, error) { /* moved */ }
func (c *Core) GetDBTables() *DBTablesResult { /* moved */ }
func (c *Core) ExecuteAction(action string) error { /* start/stop/restart/update-publish/restart-monitor */ }
```

- [ ] **Step 4: Refactor monitor command layer to delegate to monitorcore**

```go
type Monitor struct {
    baseDir string
    core    *monitorcore.Core
}

func NewMonitor(baseDir string) *Monitor {
    return &Monitor{baseDir: baseDir, core: monitorcore.New(baseDir)}
}
```

- [ ] **Step 5: Run tests for monitorcore and monitor cmd package**

Run: `cd server; go test ./internal/monitorcore ./cmd/wheelmaker-monitor -run TestCore -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add server/internal/monitorcore/core.go server/internal/monitorcore/core_test.go server/cmd/wheelmaker-monitor/monitor.go server/cmd/wheelmaker-monitor/monitor_test.go
git commit -m "refactor: extract shared monitorcore for status log db action"
```

### Task 3: Add Registry `monitor` Whitelist and `monitor.*` Hub-Scoped Routing

**Files:**
- Modify: `server/internal/registry/server.go`
- Modify: `server/internal/registry/protocol.go`
- Test: `server/internal/registry/server_test.go`

- [ ] **Step 1: Write failing tests for monitor whitelist and hubId routing**

```go
func TestMethodAllowed_MonitorRole(t *testing.T) {}
func TestMonitorListHub_WithMonitorRole(t *testing.T) {}
func TestMonitorStatus_RequiresHubID(t *testing.T) {}
func TestMonitorStatus_ForwardsByHubID(t *testing.T) {}
func TestMonitorRole_ProjectListAllowed(t *testing.T) {}
```

- [ ] **Step 2: Run tests to verify failures**

Run: `cd server; go test ./internal/registry -run Monitor -v`
Expected: FAIL due to missing whitelist/routing.

- [ ] **Step 3: Add monitor role whitelist**

```go
case "monitor":
    return method == "project.list" || method == "monitor.listHub" ||
        strings.HasPrefix(method, "monitor.") || method == "batch"
```

- [ ] **Step 4: Implement registry-local `monitor.listHub`**

```go
func (s *Server) handleMonitorListHub(peer *peerConn, state *connectionState, in envelope) {
    // Build hubs[] from hub snapshots + peer online state + capability defaults
}
```

- [ ] **Step 5: Implement `monitor.*` forwarding by `payload.hubId` (not `projectId`)**

```go
func (s *Server) executeHubScopedMonitorRequest(state *connectionState, in envelope) envelope {
    // parse payload.hubId
    // lookup hub peer
    // forward request with forwardID
    // return response/error envelope
}
```

- [ ] **Step 6: Run registry tests**

Run: `cd server; go test ./internal/registry -v`
Expected: PASS with monitor-role and monitor-routing coverage.

- [ ] **Step 7: Commit**

```bash
git add server/internal/registry/server.go server/internal/registry/protocol.go server/internal/registry/server_test.go
git commit -m "feat: add monitor role whitelist and hub-scoped monitor routing"
```

### Task 4: Add Hub Reporter Handlers for `monitor.*`

**Files:**
- Modify: `server/internal/hub/reporter.go`
- Modify: `server/internal/hub/reporter_test.go`

- [ ] **Step 1: Write failing reporter tests for monitor methods**

```go
func TestReporter_MonitorStatus(t *testing.T) {}
func TestReporter_MonitorLog(t *testing.T) {}
func TestReporter_MonitorDB(t *testing.T) {}
func TestReporter_MonitorAction(t *testing.T) {}
func TestReporter_MonitorUnsupportedAction(t *testing.T) {}
```

- [ ] **Step 2: Run reporter tests and confirm failures**

Run: `cd server; go test ./internal/hub -run Reporter_Monitor -v`
Expected: FAIL (`unsupported method on hub`).

- [ ] **Step 3: Add `monitor.*` switch branches and payload decode**

```go
case "monitor.status":
    r.replyMonitorStatus(conn, in)
case "monitor.log":
    r.replyMonitorLog(conn, in)
case "monitor.db":
    r.replyMonitorDB(conn, in)
case "monitor.action":
    r.replyMonitorAction(conn, in)
```

- [ ] **Step 4: Implement handlers delegating to monitorcore**

```go
func (r *Reporter) replyMonitorStatus(conn *websocket.Conn, req envelope) {
    // core.GetServiceStatus()
}
```

- [ ] **Step 5: Run hub package tests**

Run: `cd server; go test ./internal/hub -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add server/internal/hub/reporter.go server/internal/hub/reporter_test.go
 git commit -m "feat: support monitor methods in hub reporter"
```

### Task 5: Add Monitor Backend Transport Selection (Registry vs Direct Local Hub)

**Files:**
- Create: `server/cmd/wheelmaker-monitor/transport.go`
- Modify: `server/cmd/wheelmaker-monitor/monitor.go`
- Modify: `server/cmd/wheelmaker-monitor/routes.go`
- Test: `server/cmd/wheelmaker-monitor/monitor_test.go`
- Test: `server/cmd/wheelmaker-monitor/routes_test.go`

- [ ] **Step 1: Write failing tests for transport selection and hub-scoped API calls**

```go
func TestMonitorTransport_UsesRegistryWhenConfigured(t *testing.T) {}
func TestMonitorTransport_UsesDirectWhenRegistryDisabled(t *testing.T) {}
func TestRoutes_StatusByHubID(t *testing.T) {}
func TestRoutes_ActionByHubID(t *testing.T) {}
```

- [ ] **Step 2: Run monitor cmd tests and confirm failures**

Run: `cd server; go test ./cmd/wheelmaker-monitor -run Transport -v`
Expected: FAIL (missing transport abstraction).

- [ ] **Step 3: Implement `HubTransport` interface and two implementations**

```go
type HubTransport interface {
    ListHub(ctx context.Context) ([]HubInfo, error)
    MonitorStatus(ctx context.Context, hubID string) (*ServiceStatus, error)
    MonitorLog(ctx context.Context, req MonitorLogRequest) (*LogResult, error)
    MonitorDB(ctx context.Context, hubID string) (*DBTablesResult, error)
    MonitorAction(ctx context.Context, hubID, action string) error
    ProjectList(ctx context.Context, hubID string) ([]RegistryProject, error)
}
```

- [ ] **Step 4: Update monitor API handlers to require/read selected `hubId`**

```go
// GET /api/status?hubId=...
// GET /api/logs?hubId=...
// GET /api/db?hubId=...
// POST /api/action/{action} with hubId
```

- [ ] **Step 5: Run monitor route tests**

Run: `cd server; go test ./cmd/wheelmaker-monitor -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add server/cmd/wheelmaker-monitor/transport.go server/cmd/wheelmaker-monitor/monitor.go server/cmd/wheelmaker-monitor/routes.go server/cmd/wheelmaker-monitor/monitor_test.go server/cmd/wheelmaker-monitor/routes_test.go
git commit -m "feat: add monitor transport abstraction for registry and direct hub"
```

### Task 6: UI Update - Add Titlebar Hub Dropdown Without Layout Rewrite

**Files:**
- Modify: `server/cmd/wheelmaker-monitor/dashboard.go`
- Modify: `server/cmd/wheelmaker-monitor/dashboard_test.go`

- [ ] **Step 1: Write failing dashboard tests for hub selector rendering and request wiring**

```go
func TestDashboard_ContainsHubSelectorUnderTopbar(t *testing.T) {}
func TestDashboard_LoadsDataBySelectedHub(t *testing.T) {}
```

- [ ] **Step 2: Run dashboard tests and confirm failures**

Run: `cd server; go test ./cmd/wheelmaker-monitor -run Dashboard -v`
Expected: FAIL due to missing selector/JS hooks.

- [ ] **Step 3: Add titlebar-adjacent hub dropdown and preserve existing layout grid**

```html
<div class="topbar-hubsel">
  <label for="hub-select">Hub</label>
  <select id="hub-select" onchange="onHubChanged()"></select>
</div>
```

- [ ] **Step 4: Update JS to load `monitor.listHub`, keep selection, reload status/log/db/project list**

```javascript
let selectedHubId = '';
async function loadHubList() { /* fetch api/hubs */ }
function onHubChanged() { /* set selectedHubId + refresh() */ }
```

- [ ] **Step 5: Run monitor cmd tests**

Run: `cd server; go test ./cmd/wheelmaker-monitor -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add server/cmd/wheelmaker-monitor/dashboard.go server/cmd/wheelmaker-monitor/dashboard_test.go
git commit -m "feat: add hub selector under titlebar without layout redesign"
```

### Task 7: End-to-End Verification and Protocol Doc Sync

**Files:**
- Modify: `docs/registry-protocol.md`
- Modify: `docs/superpowers/specs/2026-04-16-monitor-hub-unified-access-design.md` (if implementation drift)
- Test: existing server test suites

- [ ] **Step 1: Write doc assertion tests/checklist in PR notes (manual verification script)**

```text
- connect.init supports role=monitor
- monitor whitelist includes monitor.* and project.list
- monitor.* uses payload.hubId
- registry=0 path uses direct hub transport
```

- [ ] **Step 2: Update protocol doc sections for role and method whitelist**

```md
- role: hub | client | monitor
- monitor role whitelist and monitor.* semantics
```

- [ ] **Step 3: Run full server tests**

Run: `cd server; go test ./...`
Expected: PASS.

- [ ] **Step 4: Run monitor smoke manually (registry on/off)**

Run:
- `cd server; go run ./cmd/wheelmaker/`
- `cd server; go run ./cmd/wheelmaker-monitor/`
Expected:
- Hub dropdown shows hubs
- Status/log/db/actions follow selected hub
- Project list remains visible under selected hub

- [ ] **Step 5: Commit**

```bash
git add docs/registry-protocol.md docs/superpowers/specs/2026-04-16-monitor-hub-unified-access-design.md
git commit -m "docs: update protocol and spec for monitor role and hub-scoped monitor methods"
```

## Spec Coverage Check

- Added monitor role model and whitelist isolation: covered by Tasks 1 and 3.
- Unified monitor method set (`monitor.listHub/status/log/db/action`): covered by Tasks 3 and 4.
- Registry-on vs registry-off transport unification: covered by Task 5.
- Keep project list visible in monitor page: covered by Tasks 5 and 6 (`project.list` allowed for monitor role).
- UI change limited to titlebar hub dropdown: covered by Task 6.
- Future ACL-ready hub scoping by token principal: covered by Task 3 routing/validation and Task 7 docs.

## Placeholder Scan

- No TODO/TBD placeholders remain.
- Every task has concrete files, commands, and expected outcomes.
- Method names and payload fields are consistent (`monitor.*`, `hubId`, `project.list`).

## Type Consistency Check

- Role values consistently referenced as `hub | client | monitor`.
- Hub-scoped monitor requests consistently use `payload.hubId`.
- Dashboard still uses `project.list` for selected hub project display.
