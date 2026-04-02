# WheelMaker Global Protocol (Draft v0.1)

## 1. Background

Current communication contracts are split across multiple places:

- `internal/shared/registry_proto.go` defines Registry and Hub/Client wire structures.
- `internal/hub/im/mobile/protocol.go` defines Mobile IM websocket messages.
- ACP JSON-RPC structs live in `internal/hub/acp/*`.

This fragmentation causes:

- Different envelope shapes and naming conventions.
- Duplicate error and routing semantics.
- Harder end-to-end evolution for `Agent <-> Client <-> IM <-> Registry`.

This document defines a unified direction named **Global Protocol** and introduces a centralized location for protocol definitions under `server/internal/protocol/`.

## 2. Goals

1. Unify wire-level envelope semantics across Agent, Client, IM, and Registry.
2. Keep compatibility with existing implementations during migration.
3. Centralize protocol constants, payload structs, and method namespaces in one directory.
4. Make routing, tracing, and error handling consistent.

## 3. Non-goals

- Replacing ACP transport immediately.
- Rewriting all existing handlers in one shot.
- Breaking current `registry`/`hub` behavior.

## 4. Unified Envelope

All cross-component messages should converge to a single envelope model:

```json
{
  "schema": "wm.global",
  "version": "0.1",
  "messageId": "7c6ef4c6-...",
  "correlationId": "5dbf54c5-...",
  "type": "request",
  "method": "message.user",
  "projectId": "local-hub:WheelMaker",
  "source": {
    "component": "client",
    "id": "wm-web",
    "sessionId": "sess-123"
  },
  "target": {
    "component": "agent",
    "id": "codex"
  },
  "ts": 1777777777,
  "payload": {}
}
```

### Field conventions

- `schema`: fixed namespace, currently `wm.global`.
- `version`: protocol version (independent from product version).
- `messageId`: unique ID for this message.
- `correlationId`: request/response chain ID.
- `type`: `request | response | event | error`.
- `method`: namespaced method string.
- `projectId`: required for project-scoped traffic.
- `source/target`: explicit endpoints for routing and observability.
- `ts`: unix timestamp in seconds.
- `payload`: method-specific data.

## 5. Method Namespace

Use a flat namespace with domain prefixes:

- `auth.*` - authentication and handshake.
- `session.*` - lifecycle control (`open`, `close`, `resume`).
- `message.*` - human/agent text flow and streaming.
- `decision.*` - options/confirmations and user selections.
- `project.*` - project metadata and sync status.
- `registry.*` - registry snapshot/update/report methods.
- `heartbeat.*` - ping/pong and keepalive.

Examples:

- `auth.init`
- `session.open`
- `message.user`
- `message.agent.delta`
- `decision.request`
- `decision.reply`
- `project.syncCheck`
- `registry.reportProjects`

## 6. Unified Error Model

Error envelope payload should use one shape everywhere:

```json
{
  "code": "INVALID_ARGUMENT",
  "message": "projectId is required",
  "details": {
    "field": "projectId"
  }
}
```

Shared error code set:

- `INVALID_ARGUMENT`
- `UNAUTHORIZED`
- `FORBIDDEN`
- `NOT_FOUND`
- `CONFLICT`
- `UNAVAILABLE`
- `RATE_LIMITED`
- `TIMEOUT`
- `INTERNAL`

## 7. Migration Plan

### Phase 0: Centralize definitions (current iteration)

- Add `server/internal/protocol/` as the single source of protocol definitions.
- Move Registry protocol types/constants to this package.
- Keep `internal/shared/registry_proto.go` as compatibility aliases.
- Add this `docs/global-protocol.md` specification.

### Phase 1: Envelope adoption in runtime

- Hub/Registry switch internal envelope aliases to `internal/protocol`.
- IM adapters map their transport payloads to global envelope semantics.

### Phase 2: Agent/IM convergence

- Define mapping between ACP JSON-RPC updates and `message.*` global events.
- Normalize option/decision flow under `decision.*`.

### Phase 3: Full protocol uniformity

- Reduce old per-module protocol structs.
- Keep transport-specific adapters only (WebSocket, JSON-RPC), with shared semantic payload types.

## 8. Directory Convention

`server/internal/protocol/` is the unified home for protocol definitions:

- `global.go` - cross-component envelope, endpoint, method names.
- `registry.go` - registry and project synchronization payloads.
- future: `agent.go`, `im.go`, `client.go` for domain-specific payload contracts.

`internal/shared/registry_proto.go` remains temporarily as a thin compatibility layer to avoid disruptive refactor in one release.
