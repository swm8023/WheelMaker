# AgentV2 Conn Consolidation Design

Updated: 2026-04-04
Status: Draft

## 0. Background

The current server runtime still carries historical layering:

- `Session -> AgentInstance -> AgentConn -> acp.Forwarder -> acp.Conn`
- legacy registration (`RegisterAgent`) is wrapped into V2 factory flow.

The target in this design is an aggressive V2 consolidation:

1. Remove `acp.Forwarder` completely.
2. Keep `Session` visibility limited to `AgentInstance`.
3. Move protocol definitions to a hub-peer package (`internal/protocol`) as the single source of truth.
4. Merge old `internal/hub/agent/*` runtime provider responsibilities into `internal/hub/agentv2/*`.

## 1. Goals

1. Simplify runtime layering while preserving Session boundary:
   - `Session -> agentv2.Instance -> agentv2.Conn`
2. Consolidate all connection/transport behavior in `agentv2.Conn`.
3. Keep business and decision logic (permissions, agent-specific behavior) in `agentv2.Instance`.
4. Keep ACP protocol structs/tags unchanged, moved into one protocol file.
5. Provide safe migration with compatibility aliases before final deletion.

## 2. Non-Goals

1. Redesign ACP payload shapes.
2. Change IM routing model or Session lifecycle state machine.
3. Introduce per-agent duplicated transport implementations.
4. Move business permission decisions down into connection layer.

## 3. Final Decisions

| Decision | Final Choice |
|---|---|
| Refactor intensity | High (breaking changes allowed) |
| Forwarder fate | Delete `acp.Forwarder` entirely |
| Session boundary | Keep `Session` seeing `Instance` only |
| Protocol placement | `server/internal/protocol/acp.go` single file |
| Protocol migration style | Compatibility-first (old `acp` aliases to `protocol`) |
| Conn role | Connection-only raw transport (`Send/Notify/OnRequest`) |
| Agent-specific logic | Instance layer |
| Old `agent` package | Merge responsibilities into `agentv2` |
| provider placement | Flat files under `agentv2/` (no subdirectory) |

## 4. Target Runtime Topology

```text
Session
  -> agentv2.Instance (business + agent specialization)
      -> agentv2.Conn (transport + callback routing)
          -> subprocess stdio JSON-RPC
```

No runtime path may include `acp.Forwarder` after migration completion.

## 5. Package Layout (Target)

```text
server/internal/
  protocol/
    acp.go                     # ACP protocol types only (unchanged shape)

  hub/agentv2/
    provider.go                # provider contract
    provider_codex.go          # codex process launch + command resolution
    provider_claude.go         # claude process launch + command resolution
    provider_copilot.go        # copilot process launch + command resolution
    conn.go                    # conn lifecycle + raw Send/Notify/OnRequest
    callbacks.go               # raw callback bridge interfaces (conn->instance)
    instance.go                # base + specialized instance implementations
    factory.go                 # builds Instance + Conn by policy
```

## 6. Responsibilities by Layer

### 6.1 `protocol/acp.go`

- Defines ACP constants, params, results, update payloads.
- Contains no runtime behavior.
- Struct names, fields, and JSON tags remain unchanged.

### 6.2 `agentv2.Conn`

- Owns subprocess connection lifecycle.
- Owns JSON-RPC request/notification send path.
- Exposes only raw transport primitives (`Send`, `Notify`, `OnRequest`).
- Forwards inbound raw request/notification payloads to instance callback bridge.
- Supports owned/shared mode via `acpSessionId -> Instance` mapping.
- Does not contain permission decision or business policy.

### 6.3 `agentv2.Instance`

- Is the only object exposed to Session.
- Holds business semantics and agent-specific behavior.
- Owns typed ACP methods (`Initialize`, `SessionNew`, `SessionLoad`, `SessionPrompt`, `SessionCancel`, `SessionSetConfigOption`).
- Owns raw inbound message decode and method dispatch behavior.
- Implements permission-related decisions and other policy hooks.
- Delegates transport operations to `agentv2.Conn`.

### 6.4 Providers in `agentv2`

- Absorb old `hub/agent/*` responsibilities:
  - binary resolution
  - fallback command construction
  - environment preparation
  - launch argument composition

## 7. Core Interfaces (Design Skeleton)

```go
// internal/hub/agentv2/instance.go
package agentv2

type Instance interface {
    Name() string
    Initialize(ctx context.Context, p protocol.InitializeParams) (protocol.InitializeResult, error)
    SessionNew(ctx context.Context, p protocol.SessionNewParams) (protocol.SessionNewResult, error)
    SessionLoad(ctx context.Context, p protocol.SessionLoadParams) (protocol.SessionLoadResult, error)
    SessionList(ctx context.Context, p protocol.SessionListParams) (protocol.SessionListResult, error)
    SessionPrompt(ctx context.Context, p protocol.SessionPromptParams) (protocol.SessionPromptResult, error)
    SessionCancel(sessionID string) error
    SessionSetConfigOption(ctx context.Context, p protocol.SessionSetConfigOptionParams) ([]protocol.ConfigOption, error)
    Close() error
}
```

```go
// internal/hub/agentv2/conn.go
package agentv2

type RequestHandler func(ctx context.Context, method string, params json.RawMessage, noResponse bool) (any, error)

type Conn interface {
    Send(ctx context.Context, method string, params any, result any) error
    Notify(method string, params any) error
    OnRequest(h RequestHandler)
    RegisterInstance(acpSessionID string, inst Instance)
    UnregisterInstance(acpSessionID string)
    UnregisterAllForInstance(inst Instance)
    Close() error
}
```

```go
// internal/hub/agentv2/provider.go
package agentv2

type Provider interface {
    Name() string
    LaunchSpec() (exe string, args []string, env []string, err error)
}
```

## 8. Migration Plan (Compatibility-First)

### Phase 1: Protocol freeze

1. Add `internal/protocol/acp.go` with unchanged ACP protocol structs/tags.
2. Add tests ensuring JSON marshal/unmarshal parity.

### Phase 2: Alias bridge

1. In `internal/hub/acp`, add alias files (`type X = protocol.X`).
2. Keep existing imports compiling while shifting source-of-truth to `protocol`.

### Phase 3: Introduce `agentv2` core

1. Add `agentv2` interfaces (`Provider`, `Conn`, `Instance`, `Factory`).
2. Add adapters so current Client/Session can compile without behavior change.

### Phase 4: Move forwarder responsibilities into `agentv2`

1. Move raw transport behavior from old forwarder/conn stack to `agentv2.Conn` (`Send`, `Notify`, `OnRequest`).
2. Move typed ACP method wrappers to `agentv2.Instance`.
3. Keep callback entry in `agentv2.Conn`, but method decode/typed dispatch in `agentv2.Instance`.
4. Keep behavior parity for stream, cancel, and callback routing.

### Phase 5: Move old agent package responsibilities

1. Migrate launch logic from `hub/agent/codex|claude|copilot` into `agentv2` flat provider files.
2. Replace registrations in hub/client to use `agentv2` providers/factory.

### Phase 6: Client boundary switch

1. Change Session field type from old instance type to `agentv2.Instance`.
2. Remove direct dependencies on old `hub/acp` protocol types in Session/Client.

### Phase 7: Delete legacy layers

1. Delete `internal/hub/acp/forwarder.go`.
2. Delete old `internal/hub/agent/*` package.
3. Remove now-unused bridges/aliases after import migration is complete.

## 9. Validation Matrix

For each phase, require:

1. `go test ./internal/hub/...`
2. callback routing tests (session/update and request handlers)
3. streaming prompt tests (done/error/cancel)
4. shared connection dispatch tests (`acpSessionId -> instance`)
5. agent launch tests (codex/claude/copilot launch resolution)

No phase advances with failing parity in transport behavior.

## 10. Risks and Mitigations

1. Risk: callback regressions during forwarder removal.
   - Mitigation: move dispatch with parity tests first, then delete.

2. Risk: mixed protocol source during transition.
   - Mitigation: strict single source (`protocol/acp.go`) + temporary aliases only.

3. Risk: hidden business logic accidentally moved into Conn.
   - Mitigation: enforce review rule: Conn cannot make permission/policy decisions.

4. Risk: migration blast radius in Client package.
   - Mitigation: phase-gated adapters before final type switch.

## 11. Exit Criteria

Migration is complete only when all are true:

1. No production import depends on `internal/hub/acp/forwarder.go`.
2. `internal/hub/agent/*` no longer exists.
3. Session depends on `agentv2.Instance` only.
4. ACP protocol source-of-truth is `internal/protocol/acp.go`.
5. Full hub test suite passes.

## 12. Bootstrap and SessionID Routing Guarantees

### 12.1 Three-Stage Bootstrap State

Session and Instance bootstrap must be modeled as three independent states:

1. `connReady`: subprocess + RPC channel are alive (`Conn` exists and can `Send/Notify`).
2. `initialized`: ACP `initialize` handshake completed and capabilities are available.
3. `acpSessionReady`: ACP session ID is bound and active (from `session/new` or `session/load`).

This separation is mandatory so `/new` and `/load` work correctly before prompt traffic starts.

### 12.2 `/new` and `/load` Before ACP Session Exists

Behavior contract:

1. `/new`
   - Ensure `connReady`.
   - Ensure `initialized` (run initialize if needed).
   - Call `SessionNew`.
   - Bind returned `sessionID` and set `acpSessionReady=true`.

2. `/load <sessionID>`
   - Ensure `connReady`.
   - Ensure `initialized` (run initialize if needed).
   - Pre-register target `sessionID` as pending route (see 12.3).
   - Call `SessionLoad`.
   - On success: promote to active route and set `acpSessionReady=true`.
   - On failure: rollback pending registration.

3. Prompt when not `acpSessionReady`
   - Follow explicit bootstrap policy: attempt saved session via `load`, then fallback `new` (or return explicit error if fallback disabled).

### 12.3 Three-Layer SessionID Mapping in `agentv2.Conn`

To guarantee correct callback routing during `new/load` races, Conn maintains three maps:

1. `activeMap`
   - `sessionID -> binding(instance, epoch)`
   - Stable, committed routes for normal callback dispatch.

2. `pendingMap`
   - `requestID -> pendingBinding(instance, targetSessionID, opType, epoch)`
   - Temporary route during in-flight `session/load` or `session/new`.
   - `load` success promotes to `activeMap`; failure removes entry.

3. `orphanBuffer`
   - `sessionID -> []update` with TTL.
   - Buffers early/late updates that arrive before route commitment.
   - Replayed after route becomes active; dropped on TTL expiry with warning log.

### 12.4 Correctness Rules

1. `session/load` must register pending route before request send.
2. Pending route must rollback on load error.
3. Dispatch must validate `(sessionID, epoch)` to avoid stale-instance delivery.
4. Unknown-session updates must not be silently dropped; they must be buffered or explicitly logged and rejected by policy.
5. For `session/new`, if protocol may emit updates before response, orphan buffering is required.

### 12.5 Test Requirements for This Contract

At minimum, add and keep passing tests for:

1. `load` success: pending -> active promotion.
2. `load` failure: pending rollback and no stale active route.
3. concurrent switch/load: stale epoch callback is rejected.
4. unknown-session update before commit: buffered then replayed (or policy-drop with deterministic log).
5. `new` response + callback ordering edge cases.
