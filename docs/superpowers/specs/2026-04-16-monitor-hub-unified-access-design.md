# Monitor-Hub Unified Access Design

Date: 2026-04-16
Status: Draft for review (rev-2)

## Goal

Provide one monitor-facing protocol for operating and observing hubs, regardless of whether registry is enabled.

The monitor surface should:

- Use one method family: `monitor.*`
- Route by `hubId` only (no `projectId` dependency for `monitor.*`)
- Support the minimal operation/view set:
  - `monitor.listHub`
  - `monitor.status`
  - `monitor.log`
  - `monitor.db`
  - `monitor.action`
- Keep monitor page ability to show projects under selected hub via `project.list`
- Keep compatibility with future multi-user token-based access control.

## Non-Goals

- Do not introduce project-scoped monitor methods in V1.
- Do not add separate monitor-only protocol transport formats.
- Do not require browser clients to connect to registry directly.
- Do not redesign dashboard layout structure in V1.

## Context and Constraints

- Deployment can have at most one registry, or no registry.
- If registry exists, monitor traffic should go through registry and then hub reporter.
- If registry does not exist, monitor should connect directly to local hub reporter.
- Local and remote hub access must share one logical API surface.
- Future requirement: token-based hub visibility per user.

## Design Summary

### 1. Unified Method Surface

All monitor capabilities are exposed as `monitor.*`:

- `monitor.listHub` (hub discovery)
- `monitor.status` (basic runtime status)
- `monitor.log` (hub log query)
- `monitor.db` (hub DB table query)
- `monitor.action` (hub operations)

These methods are hub-scoped and use `payload.hubId`.

For project list display in monitor UI, monitor role can still call existing `project.list` (scoped by selected `hubId` connection context).

### 2. Unified Routing with Two Transports

Monitor backend introduces a transport abstraction:

- `RegistryTransport`:
  - `monitor-backend -> registry(ws) -> hub reporter`
- `DirectHubTransport`:
  - `monitor-backend -> local hub reporter(ws)` when registry is absent

Selection rule:

- Registry configured and enabled: use `RegistryTransport`
- Otherwise: use `DirectHubTransport`

Upper layer method contracts remain identical in both paths.

### 3. Hub Reporter as Capability Owner

`monitor.*` business handling is owned by hub reporter-side handlers.

Monitor process and hub reporter should share core monitor access logic through a shared package (for example `internal/monitorcore`) to avoid duplicated behavior and output drift.

### 4. Role and Identity Model

V1 introduces registry role `monitor`.

- `hub`: hub reporter connection role
- `client`: generic client role (existing semantics)
- `monitor`: monitor backend role (new)

Monitor backend connects as `role=monitor`.

Method whitelist proposal:

- `hub`: `registry.reportProjects`, `registry.updateProject`, `registry.chat.message`, `registry.session.*`, `hub.ping`
- `client`: existing methods (`project.*`, `chat.*`, `session.*`, `fs.*`, `git.*`, `batch`)
- `monitor`: `monitor.*`, `project.list`, `batch` (optional), `hub.ping` (optional)

Rationale:

- Clear separation of responsibilities.
- Monitor cannot call generic client methods such as `fs.*`/`git.*` unless explicitly allowed in future.
- Monitor keeps required `project.list` capability for current page display.

## Protocol Details

## 1) `monitor.listHub`

Purpose: list hubs visible to current token principal.

Request:

```json
{
  "requestId": 10,
  "type": "request",
  "method": "monitor.listHub",
  "payload": {}
}
```

Response:

```json
{
  "requestId": 10,
  "type": "response",
  "method": "monitor.listHub",
  "payload": {
    "hubs": [
      {
        "hubId": "local",
        "online": true,
        "scope": "local",
        "capabilities": {
          "monitorStatus": true,
          "monitorLog": true,
          "monitorDB": true,
          "monitorAction": true
        }
      }
    ]
  }
}
```

Handling location:

- Registry path: resolved locally by registry.
- Direct path (no registry): synthesized from local hub connection state.

## 2) `monitor.status`

Purpose: basic runtime state for target hub.

Request:

```json
{
  "requestId": 11,
  "type": "request",
  "method": "monitor.status",
  "payload": { "hubId": "local" }
}
```

Response payload aligns with existing basic monitor status shape (service/process focused).

## 3) `monitor.log`

Purpose: query logs from target hub runtime.

Request:

```json
{
  "requestId": 12,
  "type": "request",
  "method": "monitor.log",
  "payload": {
    "hubId": "local",
    "file": "hub",
    "level": "warn",
    "tail": 200
  }
}
```

Response payload aligns with current log query shape (`file`, `entries`, `total`).

## 4) `monitor.db`

Purpose: query monitor DB tables from target hub runtime.

Request:

```json
{
  "requestId": 13,
  "type": "request",
  "method": "monitor.db",
  "payload": { "hubId": "local" }
}
```

Response payload aligns with current DB table view shape.

## 5) `monitor.action`

Purpose: execute one hub operation.

Request:

```json
{
  "requestId": 14,
  "type": "request",
  "method": "monitor.action",
  "payload": {
    "hubId": "local",
    "action": "restart"
  }
}
```

Allowed action values:

- `start`
- `stop`
- `restart`
- `update-publish`
- `restart-monitor`

Response:

```json
{
  "requestId": 14,
  "type": "response",
  "method": "monitor.action",
  "payload": {
    "ok": true,
    "action": "restart"
  }
}
```

## Validation and Error Rules

- `payload.hubId` is required for all `monitor.status|log|db|action`.
- `monitor.listHub` does not require `hubId`.
- Hub not found: `NOT_FOUND`
- Hub exists but principal cannot access: `FORBIDDEN`
- Hub offline/unreachable: `UNAVAILABLE`
- Invalid method payload: `INVALID_ARGUMENT`
- Forward timeout: `TIMEOUT`

Error payload format remains the existing standard.

## Security and ACL Model (Future-Compatible Baseline)

- Registry resolves principal from `connect.init.token`.
- `monitor.listHub` returns only hubs visible to principal.
- For hub-scoped methods, registry enforces `principal -> hubId` authorization before forwarding.
- `project.list` for monitor role must return only projects under authorized hub scope.
- Direct local mode (no registry) is treated as single-principal local trusted mode in V1.

## Monitor Page Design Update (V1)

Keep existing dashboard layout. Do not redesign page structure.

UI changes:

- Add a `Hub` dropdown selector directly under the title bar.
- Current panels (`status`, `logs`, `db`, actions, project list) keep the same layout and style.
- All panel data reloads use selected `hubId`.
- Project list display for selected hub uses `project.list`.

Behavior rules:

- Default selected hub is first online hub in `monitor.listHub`; fallback to first entry.
- If selected hub becomes offline, keep selection but show offline state and disable actions except refresh.

## Architecture and File Impact

Suggested components:

- `server/internal/monitorcore/`
  - shared status/log/db/action logic
- `server/internal/protocol/registry.go`
  - extend `ConnectInitPayload.Role` doc/validation values for `monitor`
  - add monitor request/response payload structs (optional but recommended)
- `server/internal/registry/server.go`
  - allow `monitor` role in `connect.init`
  - add monitor whitelist and `monitor.*` routing
  - add local handling for `monitor.listHub`
  - keep `project.list` allowed for monitor role
- `server/internal/hub/reporter.go`
  - add `monitor.*` request handlers
  - call shared `monitorcore`
- `server/cmd/wheelmaker-monitor/`
  - add hub selector support in frontend
  - route handlers call unified transport by selected hub

## Compatibility Plan

- Existing monitor local APIs continue to work during migration.
- New `monitor.*` path is introduced behind monitor backend internal transport layer.
- UI migration is incremental:
  1. wire `monitor.listHub` + titlebar dropdown
  2. switch status/log/db panels to hub-scoped calls
  3. switch action buttons to `monitor.action`
  4. keep project list using `project.list` with monitor role scope

## Open Decisions

- Whether `monitor.action` should include optional idempotency key in V1.
- Whether `monitor` role should allow `batch` in V1.
- Exact transport pooling strategy for per-user long-lived registry connections.

## Testing Scope

- Registry:
  - `role=monitor` handshake tests
  - monitor whitelist/routing tests
  - ACL checks (`FORBIDDEN`, `NOT_FOUND`, `UNAVAILABLE`)
  - `project.list` under monitor scope tests
- Hub reporter:
  - each `monitor.*` handler success/failure tests
- Monitor backend:
  - transport selection tests (registry-on vs registry-off)
  - hub dropdown selection and reload behavior tests
- End-to-end smoke:
  - registry enabled path
  - direct local path
