# IM2 Client And Feishu Integration Design

Date: 2026-04-07
Status: Draft for review

## Goal

Add an opt-in IM2 runtime path for `client.Client` and Feishu, controlled by `im.version: 2`.

IM2 should become a real upstream/downstream path:

- Client can receive IM2 inbound events and bind chats to sessions.
- Client can send normal replies, ACP stream updates, and permission decisions through IM2.
- Feishu IM2 implements its own rendering and decision handling inside `server/internal/im2/feishu`.
- IM1 remains the default runtime and is not removed in this phase.

## Non-Goals

- Do not make IM2 the default runtime.
- Do not remove IM1 packages.
- Do not keep IM2 Feishu dependent on `hub/im.ImAdapter`.
- Do not move Feishu-specific ACP parsing or card action handling into `client`.
- Do not implement the App channel network protocol.
- Do not add persistent IM2 history storage in this phase.

## Configuration

`shared.IMConfig` gains a runtime version:

```go
type IMConfig struct {
	Type      string `json:"type"`
	Version   int    `json:"version,omitempty"`
	AppID     string `json:"appID,omitempty"`
	AppSecret string `json:"appSecret,omitempty"`
}
```

Semantics:

- Missing, `0`, or `1`: use IM1.
- `2`: use IM2.

Example:

```json
{
  "im": {
    "type": "feishu",
    "version": 2,
    "appID": "...",
    "appSecret": "..."
  }
}
```

Unsupported combinations fail at startup with a clear error.

## Architecture

```text
Hub
  if im.version != 2:
    hub/im.ImAdapter -> client.Client  (existing IM1 path)

  if im.version == 2:
    im2/feishu.Channel -> im2.Router -> client.Client

Client
  IM1 outbound: Session -> im.ImAdapter
  IM2 outbound: Session -> im2.Router -> im2.Channel

im2/feishu
  Owns Feishu-specific rendering:
    message/system/acp/decision payloads
    text and thought stream buffering
    tool call cards
    plan/config update rendering
    permission card action and text fallback
```

IM2 does not reuse `hub/im.ImAdapter`. IM1 will be retired later, so IM2 should own the new implementation even if some rendering logic is initially copied from IM1.

## IM2 Protocol Additions

The existing `OutboundEvent` stays as the router send envelope:

```go
type OutboundEvent struct {
	Kind    OutboundKind
	Payload any
}
```

Add concrete payloads:

```go
type TextPayload struct {
	Text string
}

type ACPPayload struct {
	SessionID  string
	UpdateType string
	Text       string
	Raw        []byte
}
```

Add decision request/response types:

```go
type DecisionKind string

const (
	DecisionPermission DecisionKind = "permission"
	DecisionConfirm    DecisionKind = "confirm"
	DecisionSingle     DecisionKind = "single"
	DecisionInput      DecisionKind = "input"
)

type DecisionOption struct {
	ID    string
	Label string
	Value string
}

type DecisionRequest struct {
	SessionID string
	Kind      DecisionKind
	Title     string
	Body      string
	Options   []DecisionOption
	Meta      map[string]string
	Hint      map[string]string
}

type DecisionResult struct {
	Outcome  string
	OptionID string
	Value    string
	ActorID  string
	Source   string
}
```

Decision is a request/response operation, not a fire-and-forget send:

```go
func (r *Router) RequestDecision(ctx context.Context, target SendTarget, req DecisionRequest) (DecisionResult, error)
```

`RequestDecision` routes to the source chat. It should not fan out to watchers, because permission decisions must be made by the active requester.

The `Channel` interface grows a decision method:

```go
type Channel interface {
	ID() string
	OnMessage(func(ctx context.Context, chatID string, text string) error)
	Send(ctx context.Context, chatID string, event OutboundEvent) error
	RequestDecision(ctx context.Context, chatID string, req DecisionRequest) (DecisionResult, error)
	Run(ctx context.Context) error
}
```

The App stub implements this by returning a clear not-implemented error.

## Client Integration

Client gains an IM2 router field:

```go
type IM2Router interface {
	Bind(ctx context.Context, chat im2.ChatRef, sessionID string, opts im2.BindOptions) error
	Send(ctx context.Context, target im2.SendTarget, event im2.OutboundEvent) error
	RequestDecision(ctx context.Context, target im2.SendTarget, req im2.DecisionRequest) (im2.DecisionResult, error)
}
```

Client also tracks per-message source chat while handling an IM2 event. The source must not be derived from a global active chat, because App can later handle multiple concurrent chats.

Suggested session fields:

```go
type Session struct {
	im2Router IM2Router
	im2Source *im2.ChatRef
}
```

`Session` remains platform-agnostic. It knows only `im2.ChatRef` and router interfaces, not Feishu.

### Inbound Flow

```text
Feishu -> im2/feishu.Channel -> im2.Router.HandleInbound -> client.HandleIM2Inbound
```

`HandleIM2Inbound(ctx, event)` behavior:

1. Ignore empty text.
2. Build `source := im2.ChatRef{ChannelID: event.ChannelID, ChatID: event.ChatID}`.
3. If `event.SessionID` is non-empty, resolve or restore that session.
4. If `event.SessionID` is empty, let client command/session logic decide:
   - `/list`: run without creating a new session and reply directly to the source chat.
   - `/new`: create a session and bind source chat to it.
   - `/load <index>`: load selected session and bind source chat to it.
   - prompt: create or restore the chosen session and bind source chat to it.
5. Set the target session's current IM2 source for the duration of command/prompt handling.

This design keeps the earlier rule: router never creates, loads, or selects sessions.

### Binding

Client calls:

```go
router.Bind(ctx, source, sessionID, im2.BindOptions{Watch: false})
```

after it creates or loads a session for a chat.

Watcher management can be added by a later command. This phase only needs the data path to preserve existing watch fanout behavior in the router.

### Outbound Reply

`Session.reply(text)` behavior:

- If IM2 source/router exists: call `router.Send` with `SessionID + Source` and `OutboundSystem`.
- Else if IM1 bridge exists: current `imBridge.SendSystem` behavior.
- Else print to stdout.

For non-system text emitted by ACP, `Session.handlePrompt` sends `OutboundACP` with `ACPPayload`.

### ACP Stream Updates

When prompt streaming receives an `acp.Update`, client sends:

```go
router.Send(ctx, im2.SendTarget{
	SessionID: sid,
	Source:    source,
}, im2.OutboundEvent{
	Kind: im2.OutboundACP,
	Payload: im2.ACPPayload{
		SessionID:  sid,
		UpdateType: string(u.Type),
		Text:       u.Content,
		Raw:        u.Raw,
	},
})
```

The client does not parse tool call raw, plan raw, or config raw. It only forwards the raw data and update type.

### Permission Decision

`SessionRequestPermission` remains owned by Session, but the decision provider changes by runtime:

- IM1: existing permission router and `imBridge.RequestDecision`.
- IM2: call `router.RequestDecision` with the current source chat and an IM2 `DecisionRequest`.

Client maps ACP permission options into IM2 decision options. It maps IM2 decision results back into `acp.PermissionResult`.

If no IM2 source is available, return a safe cancelled result.

## Hub Integration

`Hub.buildClient` branches on `pc.IM.Version`:

```text
if pc.IM.Version == 2:
  build IM2 channel
  build IM2 router
  create client with nil IM1 bridge
  client.SetIM2Router(router)
  client.Start(ctx)
else:
  existing IM1 build path
```

`Client.Run(ctx)` runs the IM2 router/channel path when IM2 is configured.

The router can either expose `Run(ctx)` to run registered channels, or the client can hold a small `im2Runner` interface. Prefer `Router.Run(ctx)` so Hub only needs to start the client.

Supported in this phase:

- `im.version=2,type=feishu`: supported.
- `im.version=2,type=app`: builds App stub if useful for tests; runtime returns not implemented.
- `im.version=2,type=console`: unsupported until a console IM2 channel exists.

## Feishu IM2 Implementation

`server/internal/im2/feishu` should own Feishu-specific behavior.

It may reference old IM1 files as a migration source, but it should not compose `hub/im.ImAdapter`.

Required behavior in this phase:

- Inbound Feishu text maps to `chatID + text`.
- Inbound card actions map to pending decisions.
- `OutboundMessage` renders plain agent text.
- `OutboundSystem` renders system text.
- `OutboundACP` handles:
  - text chunks
  - thought chunks
  - tool call updates
  - plan updates
  - config option updates
  - done
  - error
- `RequestDecision` renders a permission/options card and waits for:
  - Feishu card action
  - text fallback reply in the same chat
  - context cancellation
  - timeout
- Duplicate card actions are ignored.

Text and thought streaming, tool call card state, compact YOLO rendering, and mark-done behavior should follow the old IM1 Feishu behavior as closely as possible.

## Error Handling

- IM2 startup with unsupported `im.version=2,type=<x>` returns a clear error.
- Router direct send without channel/chat returns a parameter error.
- Router decision request requires `SessionID` and `Source`.
- Missing channel returns a clear error.
- Permission request with no current source returns `cancelled` instead of blocking.
- Feishu decision timeout returns `DecisionResult{Outcome: "timeout", Source: "default_policy"}`.
- Feishu card action for an unknown or closed decision is ignored.
- Feishu outbound rendering errors are returned to client; client may reply with a system error if possible.

## Testing Scope

Client tests:

- `SetIM2Router` makes `Client.Run` use IM2.
- `HandleIM2Inbound` with no binding and `/list` does not create a new session and replies by direct chat send.
- `HandleIM2Inbound` with no binding and prompt creates/binds a session before prompting.
- `HandleIM2Inbound` with existing `SessionID` routes to that session.
- `/new` and `/load` in IM2 bind the source chat to the new/loaded session.
- prompt ACP updates send `OutboundACP` with raw payload to the router.
- permission request in IM2 calls `RequestDecision` and maps the result back to ACP.

Router tests:

- `RequestDecision` routes only to the source chat.
- `RequestDecision` errors when source or channel is missing.

Hub tests:

- `im.version=1` or omitted still builds IM1.
- `im.version=2,type=feishu` builds IM2 and does not create an IM1 bridge.
- unsupported IM2 type returns a clear error.

Feishu tests:

- Channel no longer wraps `hub/im.ImAdapter`.
- inbound text reaches the registered handler.
- ACP text/thought/tool/plan/config/done/error payloads route through IM2 Feishu rendering.
- permission decision card action resolves once.
- duplicate card action is ignored.
- text fallback decision reply resolves pending decision.

## Migration Notes

This phase intentionally duplicates or ports Feishu rendering logic into IM2. After IM2 is stable, IM1 can be deleted or reduced to a compatibility shim.

The client should remain IM-version agnostic at the business level: it owns sessions and prompts; IM2 owns chat fanout and platform rendering.
