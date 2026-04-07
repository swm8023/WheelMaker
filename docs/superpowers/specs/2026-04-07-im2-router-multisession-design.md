# IM2 Router Multi-Session Design

Date: 2026-04-07
Status: Draft for review

## Goal

Design a fresh IM2 architecture that is isolated from the current IM 1.0 runtime until the initial IM2 version is complete.

Each `client.Client` owns one `im2.Router`. The router is the only IM entry and exit point for that client. It accepts normalized inbound text events from IM channels, hands them to the client with optional existing binding metadata, and sends normalized outbound events back through the registered channels.

The first implementation should include the router core, Feishu channel implementation, an App channel stub, and tests. It should not replace the current `server/internal/hub/im` + `client.HandleMessage(im.Message)` production path.

## Non-Goals

- Do not change the default Hub startup path to use IM2.
- Do not modify ACP protocol behavior.
- Do not move session lifecycle decisions into the router.
- Do not implement the real App network protocol in the first version.
- Do not add persistent session history storage in the first version.

## Architecture

IM2 uses one multi-session abstraction for all IM channels:

```text
Client
  -> im2.Router
       -> channel registry: feishu / app
       -> bindings: channelID + chatID -> sessionID + watch
       -> history store: append/list normalized events
       -> outbound fanout: direct chat, reply source + watchers, or session broadcast
```

Every channel is addressed by `channelID` and `chatID`.

- `channelID` identifies the IM channel, such as `feishu` or `app`.
- `chatID` identifies the conversation inside that channel.
- Feishu currently has one effective chat ID for the project, so rebinding that chat ID naturally switches the active session.
- App can later expose many chat IDs, allowing multiple App conversations to bind to the same or different sessions.

There is no channel mode declaration. The number of chat IDs a channel exposes naturally determines how many concurrent bindings it can support.

## Component Contracts

### Inbound Event

```go
type InboundEvent struct {
	ChannelID string
	ChatID    string
	Text      string
	SessionID string
}
```

`SessionID` is filled by the router when an existing binding is found. If no binding exists, it is empty and the client decides what to do.

### Chat Reference

```go
type ChatRef struct {
	ChannelID string
	ChatID    string
}
```

### Binding

```go
type BindOptions struct {
	Watch bool
}
```

The binding model is:

```text
channelID + chatID -> sessionID + watch
```

`watch=true` means this chat receives outputs from the same session even when the output is not a direct reply to this chat. `watch=false` means it receives only direct replies and explicit broadcasts.

### Outbound Event

```go
type OutboundKind string

const (
	OutboundMessage OutboundKind = "message"
	OutboundACP     OutboundKind = "acp"
	OutboundSystem  OutboundKind = "system"
)

type OutboundEvent struct {
	Kind    OutboundKind
	Payload any
}
```

The router treats `Payload` as opaque. Each channel renders `message`, `acp`, and `system` payloads using its own platform-specific UI rules.

### Send Target

Outbound sending is a single router method with target fields that select the behavior:

```go
type SendTarget struct {
	ChannelID string
	ChatID    string
	SessionID string
	Source    *ChatRef
}

func (r *Router) Send(ctx context.Context, target SendTarget, event OutboundEvent) error
```

Semantics:

- `SessionID == ""`: send directly to `ChannelID + ChatID`. This is valid before the chat is bound to a session.
- `SessionID != "" && Source != nil`: send as a normal reply to `Source`, and fan out to other chats bound to the same session with `watch=true`.
- `SessionID != "" && Source == nil`: send as a session broadcast to all chats bound to the session, regardless of `watch`.

### Channel

```go
type Channel interface {
	ID() string
	OnMessage(func(ctx context.Context, chatID string, text string) error)
	Send(ctx context.Context, chatID string, event OutboundEvent) error
	Run(ctx context.Context) error
}
```

Channels only translate between platform-specific events and IM2 normalized events.

- Inbound: channel extracts `chatID` and user text.
- Outbound: channel renders `OutboundEvent` by `Kind` and `Payload`.
- Channel code does not own session lifecycle or cross-channel fanout.

## Data Flow

### Inbound

```text
Feishu/App -> Channel.OnMessage -> Router.HandleInbound -> Client
```

Router responsibilities:

1. Normalize `channelID + chatID + text`.
2. Look up existing binding.
3. Pass an `InboundEvent` to the client.
4. If a binding exists, include `SessionID`.
5. If no binding exists, leave `SessionID` empty.

Router must not create, load, or select sessions.

Client responsibilities when `SessionID` is empty:

- If the message is a command such as `/list` that does not need a session, execute it without creating a session.
- If the chat was seen before and has persisted history, load the last session and call `Bind`.
- If the message is a prompt and no session exists, create a new session and call `Bind`.
- Before binding, use direct `Send` with `SessionID == ""` for replies to that chat.

### Bind

Client calls `Bind` after it has selected or created a session:

```go
func (r *Router) Bind(ctx context.Context, chat ChatRef, sessionID string, opts BindOptions) error
func (r *Router) Unbind(ctx context.Context, chat ChatRef) error
```

Binding the same `channelID + chatID` again replaces the previous session binding. This naturally supports Feishu switching from one session to another.

### Normal Reply

```text
Client/Session -> Router.Send(SessionID + Source) -> source chat + watch chats
```

The `Source` must come from the inbound message context. It must not come from a global active chat value, because future App conversations can be concurrent.

Example:

```text
A -> session1, watch=false
B -> session1, watch=true
```

If A sends a prompt, A receives the direct reply. B also receives the output because B watches the session. If B sends a prompt, B receives the direct reply; A receives it only if A also has `watch=true`.

### Direct Chat Send

```text
Client -> Router.Send(ChannelID + ChatID, SessionID="") -> one chat
```

This is used for pre-bind responses, command responses, replay responses, or explicit private sends.

### Session Broadcast

```text
Client -> Router.Send(SessionID, Source=nil) -> all bound chats for that session
```

This is an explicit broadcast. It ignores `watch`. Normal agent replies should not use this path unless the client intentionally wants all bound IM chats to receive the output.

## Session History

IM2 keeps a router-level history abstraction for normalized IM events:

```go
type SessionHistoryStore interface {
	Append(ctx context.Context, event HistoryEvent) error
	List(ctx context.Context, sessionID string, query HistoryQuery) ([]HistoryEvent, error)
}
```

Suggested event fields:

```go
type HistoryEvent struct {
	SessionID string
	Direction string
	Source *ChatRef
	Targets []ChatRef
	Kind OutboundKind
	Payload any
	Text string
	CreatedAt time.Time
}
```

The first implementation should provide an in-memory or no-op store and tests proving that router paths call the store. Persistent storage can be added later, likely with SQLite or the existing session persistence layer.

Future IM-side replay can be implemented by having a channel send a replay request to the router, then rendering history events back through that same channel.

## Feishu Channel

The first IM2 version includes a Feishu channel implementation.

- `ID()` returns `feishu`.
- Inbound Feishu messages become `chatID + text` callbacks.
- Outbound `message`, `acp`, and `system` events are rendered inside the Feishu channel.
- Existing Feishu rendering ideas can be reused, but the IM2 channel contract should stay independent from `server/internal/hub/im`.
- Since current Feishu usage exposes one effective chat ID, it naturally behaves as one active session at a time. Switching sessions is just rebinding that chat ID.

## App Channel Stub

The first IM2 version includes an App channel stub.

- `ID()` returns `app`.
- It implements the `Channel` interface.
- `Run` and `Send` may return a clear not-implemented error or no-op, depending on test needs.
- Tests should prove the router can bind multiple App chat IDs to the same or different sessions.
- Real WebSocket/mobile protocol work is deferred.

## Error Handling

- `HandleInbound` does not fail just because a chat has no binding.
- Direct send requires `ChannelID + ChatID`.
- Reply send requires `SessionID` and `Source`.
- Session broadcast requires `SessionID`.
- Missing channel returns a clear error.
- When replying to `Source` plus watchers, source delivery is attempted first.
- Watcher delivery failure should not prevent source delivery. The router may return an aggregated error after all attempted sends.
- `Bind` validates non-empty `ChannelID`, `ChatID`, and `SessionID`.

## Testing Scope

Router tests:

- Inbound unbound chat reaches client with `SessionID == ""`.
- Client-driven `Bind` causes later inbound events from the same chat to include `SessionID`.
- Direct send with `SessionID == ""` sends only to the target chat.
- Reply send reaches source and other `watch=true` chats bound to the same session.
- Reply send does not reach `watch=false` non-source chats.
- Session broadcast reaches all chats bound to the session.
- Rebinding the same chat replaces the old session binding.
- History store is called on inbound and outbound paths.

Feishu tests:

- Feishu channel satisfies the IM2 `Channel` contract.
- Inbound Feishu text is normalized to `chatID + text`.
- Outbound `message`, `acp`, and `system` events are routed through Feishu rendering methods.

App stub tests:

- App channel satisfies the IM2 `Channel` contract.
- Router can bind multiple App chat IDs to multiple sessions.
- Router can bind multiple App chat IDs to the same session and apply `watch` fanout.

Isolation tests:

- Default Hub startup path remains IM 1.0.
- IM2 packages do not require changes to `server/internal/hub/im`.

## Implementation Notes

Recommended package layout:

```text
server/internal/im2/
  protocol.go
  router.go
  history.go
  router_test.go
  history_test.go
server/internal/im2/feishu/
  feishu.go
  feishu_test.go
server/internal/im2/app/
  app.go
  app_test.go
```

The first implementation should avoid wiring IM2 into `hub.buildClient`. Once the IM2 core and Feishu channel are tested, a later plan can add an opt-in config path while keeping IM 1.0 as the default.
