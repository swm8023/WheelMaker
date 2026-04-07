# IM2 Formalization And IM1 Removal Design

Updated: 2026-04-07
Status: Proposed

## 0. Background

WheelMaker currently carries two IM runtimes:

- `server/internal/hub/im/` (`im1`): the deprecated production IM layer with `console`, `feishu`, and adapter-style client integration.
- `server/internal/im2/` (`im2`): the newer normalized IM layer with router-based session binding, Feishu integration, and an App channel stub.

This dual-runtime structure now creates the wrong optimization pressure:

- Hub startup still branches between IM1 and IM2.
- Client logic still contains both IM1 and IM2 outbound paths.
- IM2 Feishu still depends on IM1 Feishu and IM1 card/message abstractions.
- The config model still exposes `im.version`, which makes the deprecated path look valid.

This design makes IM2 the only formal IM runtime, removes IM1 completely, and keeps Feishu behavior fully available after the migration.

This spec supersedes the opt-in migration assumptions in:

- `2026-04-07-im2-router-multisession-design.md`
- `2026-04-07-im2-client-feishu-integration-design.md`

## 1. Goals

1. Remove `server/internal/hub/im/` and all runtime references to IM1.
2. Make `server/internal/im2/` the only supported IM runtime and interface contract.
3. Keep Feishu functionally on par with the current production behavior:
   - inbound text messages
   - outbound normal/system text
   - ACP text/thought/error/done updates
   - tool call cards
   - permission/decision cards
   - card action handling
   - text-reply fallback for decisions
4. Simplify Hub and Client by removing all IM1/IM2 branching.
5. Keep `im2/app` as a formal channel implementation stub so the IM2 contract is channel-generic, even if App remains unimplemented.

## 2. Non-Goals

1. Do not preserve runtime compatibility with old IM1 config.
2. Do not keep `im.version` as a deprecated alias.
3. Do not keep `console` or `mobile` channels.
4. Do not implement the App channel beyond a formal stub in this phase.
5. Do not add new product features unrelated to the IM runtime migration.

## 3. Core Decisions

| Topic | Decision |
|------|----------|
| Canonical contract | `server/internal/im2/protocol.go` is the only formal IM channel contract |
| Runtime entry/exit | `im2.Router` is the only IM orchestration layer used by `client.Client` |
| Deprecated runtime | `server/internal/hub/im/` is deleted, not retained behind compatibility shims |
| Config policy | Hard cut: only IM2-style config is valid; old IM1 shape fails fast |
| Feishu ownership | `server/internal/im2/feishu/` owns Feishu transport, rendering, decisions, and card actions directly |
| App channel | Keep `im2/app` as an official but intentionally not-yet-implemented channel |
| Client outbound path | Session replies, ACP updates, and decision requests all route only through IM2 |

## 4. Target Architecture

### 4.1 Runtime Shape

```text
Hub
  -> build IM2-only client per project
  -> create im2.Router
  -> register one im2.Channel (feishu or app)
  -> client.Client uses only IM2Router

Session
  -> prompt / command / permission handling
  -> router.Send(...)
  -> router.RequestDecision(...)

im2/feishu
  -> Feishu webhook input
  -> Feishu API output
  -> decision lifecycle
  -> ACP update rendering
```

### 4.2 Package Boundaries

- `server/internal/im2/`
  - canonical protocol types
  - router
  - history abstraction
  - concrete channels
- `server/internal/hub/client/`
  - session lifecycle and business logic
  - depends only on `IM2Router`
- `server/internal/hub/hub.go`
  - startup and wiring
  - no IM1 branch
- `server/internal/hub/im/`
  - removed entirely

## 5. Protocol And Config Changes

### 5.1 IM2 Contract

`im2.Channel` remains the formal contract:

```go
type Channel interface {
	ID() string
	OnMessage(func(ctx context.Context, chatID string, text string) error)
	Send(ctx context.Context, chatID string, event OutboundEvent) error
	RequestDecision(ctx context.Context, chatID string, req DecisionRequest) (DecisionResult, error)
	Run(ctx context.Context) error
}
```

No IM1 mirror contract remains after this migration.

### 5.2 Config

`shared.IMConfig` is simplified to IM2-only fields:

- keep: `type`, `appID`, `appSecret`
- remove: `version`

Valid `im.type` values in this phase:

- `feishu`
- `app`

Invalid values fail startup with explicit errors:

- `console`
- `mobile`
- unknown custom values

Old configs that still include `im.version` are invalid. The runtime should report a clear message indicating that IM1 has been removed and IM2 is now the only supported runtime.

This rejection must be explicit. It is not sufficient to silently ignore the old field during JSON unmarshal. The config layer must either:

- use strict decoding for unknown fields, or
- run explicit post-load validation that rejects removed IM1 fields and removed IM1-only types

## 6. Client Changes

### 6.1 Remove IM1 State

`client.Client` and `client.Session` stop carrying IM1-specific state and helpers.

Remove:

- `imBridge *im.ImAdapter`
- `HandleMessage(im.Message)` as a production entrypoint
- IM1-specific reply fallbacks
- IM1 decision fallback path in `permissionRouter`

Keep:

- `im2Router IM2Router`
- `im2Source *im2.ChatRef`
- `HandleIM2Inbound(ctx, event)` as the only IM entrypoint

### 6.2 Unified Outbound Behavior

The following all go through IM2 only:

- `Session.reply(...)`
- ACP stream updates
- prompt completion / error / done notifications
- permission decisions
- command responses (`/new`, `/load`, `/list`, `/status`, `/use`, `/cancel`)

There is no fallback to active chat globals from IM1. All reply routing is source-based through `im2.ChatRef` and router send targets.

### 6.3 Permission Decisions

`SessionRequestPermission(...)` always maps ACP permission requests into IM2 `DecisionRequest` and calls `router.RequestDecision(...)`.

If no IM2 source is available for the current session, the safe behavior is:

- return cancelled
- do not attempt any deprecated IM1 fallback

## 7. Feishu Channel Design

`server/internal/im2/feishu/` becomes self-contained and stops importing:

- `server/internal/hub/im`
- `server/internal/hub/im/feishu`

### 7.1 Responsibilities

The IM2 Feishu package owns:

1. Feishu config and startup
2. webhook request validation and parsing
3. inbound text event conversion into IM2 handler calls
4. outbound message rendering for `OutboundEvent`
5. ACP payload rendering
6. decision card rendering
7. card action decoding and pending-decision resolution
8. text reply fallback for decisions
9. done state signaling

### 7.2 File Layout

Recommended structure:

```text
server/internal/im2/feishu/
  channel.go      // im2.Channel implementation and orchestration
  client.go       // Feishu HTTP/API operations
  webhook.go      // inbound webhook parsing and callbacks
  render.go       // OutboundEvent -> Feishu message/card rendering
  decision.go     // pending decision lifecycle
  types.go        // local Feishu payload structs
  *_test.go       // focused tests per area
```

### 7.3 Feature Parity Requirements

The Feishu IM2 channel must support all currently required production behavior:

- normal text replies
- system text replies
- thought rendering
- ACP done handling
- ACP error handling
- tool call card rendering
- plan/config summary rendering where currently expected
- decision card creation
- decision card action handling
- text reply decision fallback

No feature should depend on IM1 abstractions surviving in another package.

## 8. App Channel

`server/internal/im2/app/` remains in the tree as an official IM2 channel implementation stub.

Requirements for this phase:

- keep it compiling against the formal IM2 contract
- keep it constructible by Hub when `im.type == "app"`
- it may still return `ErrNotImplemented` for runtime operations

This preserves the generality of the IM2 model without forcing App delivery into the same migration scope.

For this phase, `app` is a valid formal channel type but not a production-complete runtime. Construction and wiring must succeed; actual runtime operations may return a clear `ErrNotImplemented` error.

## 9. Hub Changes

`server/internal/hub/hub.go` becomes IM2-only.

Required changes:

1. Remove `buildIM()` and all IM1 imports.
2. Remove `pc.IM.Version` branching.
3. Remove console-only validation and constraints.
4. Always construct client + IM2 router + one IM2 channel.
5. Support at least:
   - `feishu` -> `im2/feishu.New(...)`
   - `app` -> `im2/app.New()`
6. Reject all removed IM1-only types with clear errors.

## 10. Deletion Boundaries

Delete completely:

- `server/internal/hub/im/adapter.go`
- `server/internal/hub/im/im.go`
- `server/internal/hub/im/types.go`
- `server/internal/hub/im/console/`
- `server/internal/hub/im/feishu/`
- `server/internal/hub/im/mobile/`
- IM1-only tests under `server/internal/hub/im/`

Delete all imports of `server/internal/hub/im...` across the server codebase.

## 11. Testing Strategy

### 11.1 Config And Startup

- loading config without `im.version` succeeds
- configs still carrying removed IM1 semantics fail clearly
- `im.type=feishu` starts IM2 wiring successfully
- `im.type=app` starts with the App channel stub path
- removed types such as `console` fail with explicit errors

### 11.2 Client + Router

- inbound IM2 command handling works without IM1
- prompt replies route through IM2 only
- ACP permission decisions use IM2 only
- `/new`, `/load`, `/list` still work in IM2 mode
- session reply routing remains source-correct across multiple chats

### 11.3 Feishu

- inbound text reaches router handler
- outbound text/system/acp events render correctly
- tool call updates render as cards
- decision requests create pending entries and return selected results on card action
- text reply fallback resolves decisions
- timeout and cancellation behavior remains safe

### 11.4 Repository Guardrail

Add at least one test or build-time assertion proving there is no remaining runtime dependency on `server/internal/hub/im`.

## 12. Success Criteria

This migration is complete when all of the following are true:

1. `server/internal/hub/im/` does not exist.
2. No server package imports `server/internal/hub/im`.
3. `shared.IMConfig` no longer exposes `Version`.
4. `hub.Start()` and `hub.buildClient()` are IM2-only.
5. `client.Session` replies and permission decisions route only through IM2.
6. Feishu behavior remains functionally complete for the current production use cases.
7. The full server test suite passes after the deletion.

## 13. Risks And Mitigations

| Risk | Mitigation |
|------|------------|
| Feishu parity regresses during package move | Migrate behavior with focused tests per rendering and decision path before deleting old code |
| Client logic accidentally loses reply routing | Keep source-based IM2 integration tests around command, prompt, and permission flows |
| Config hard cut surprises runtime users | Emit clear startup errors that explain IM1 removal and accepted IM2 `im.type` values |
| Large-file migration becomes messy | Split IM2 Feishu by responsibility while moving logic, rather than copying all behavior into one file |
