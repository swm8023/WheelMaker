# IM Channel ACP Boundary Design

Date: 2026-04-07
Status: Draft for review

## Goal

Redefine the `server/internal/im` channel boundary around ACP-native semantics instead of the current mixed `in/out` wrapper model.

The new design should:

- Keep ACP protocol shapes visible at the IM boundary for outbound agent events.
- Keep IM inbound behavior simple by using prompt, command, and permission-response callbacks.
- Allow each IM implementation to choose its own presentation strategy.
- Keep `feishu/channel.go` compact and focused on event handling logic.
- Consolidate cross-module ACP constants into one canonical constants file.

## Non-Goals

- Do not change ACP wire behavior.
- Do not push `fs/*` or `terminal/*` requests through IM.
- Do not introduce a generic dispatch API with stringly-typed method routing.
- Do not over-split Feishu into many small renderer or transport files.
- Do not invent internal fake ACP notifications for prompt completion or errors.

## Design Summary

The IM boundary is split into four categories:

1. ACP outbound notifications to IM:
   - `session/update`
   - `session/request_permission`
   - `session/prompt` final result
2. WheelMaker system outbound notifications to IM:
   - local errors
   - reconnecting / reconnected notices
   - local command results
   - other non-ACP runtime notices
3. IM inbound user actions to Client:
   - prompt
   - command
   - permission response
4. ACP runtime-only callbacks:
   - `fs/*`
   - `terminal/*`

`session/update` stays a real ACP notification.

`session/request_permission` stays a real ACP request-response flow.

`session/prompt` result is emitted to IM as its own outbound callback and must not be rewritten into `session/update`.

Prompt errors are not ACP notifications. They are emitted through a system notification path and rendered by the IM implementation in its own style.

## Channel Contract

Rename `server/internal/im/protocol.go` to `server/internal/im/channel.go`.

The channel interface becomes ACP-aware on the outbound side and IM-native on the inbound side:

```go
type Channel interface {
	ID() string

	OnPrompt(func(ctx context.Context, source ChatRef, params acp.SessionPromptParams) error)
	OnCommand(func(ctx context.Context, source ChatRef, cmd Command) error)
	OnPermissionResponse(func(ctx context.Context, source ChatRef, requestID int64, result acp.PermissionResponse) error)

	PublishSessionUpdate(ctx context.Context, target SendTarget, params acp.SessionUpdateParams) error
	PublishPromptResult(ctx context.Context, target SendTarget, result acp.SessionPromptResult) error
	PublishPermissionRequest(ctx context.Context, target SendTarget, requestID int64, params acp.PermissionRequestParams) error
	SystemNotify(ctx context.Context, target SendTarget, payload SystemPayload) error

	Run(ctx context.Context) error
}
```

## Shared IM Types

Recommended shared types in `server/internal/im/channel.go`:

```go
type ChatRef struct {
	ChannelID string
	ChatID    string
}

type BindOptions struct {
	Watch bool
}

type SendTarget struct {
	ChannelID string
	ChatID    string
	SessionID string
	Source    *ChatRef
}

type Command struct {
	Name string
	Args string
	Raw  string
}

type SystemPayload struct {
	Kind  string
	Title string
	Body  string
	Meta  map[string]string
}
```

`Command` represents WheelMaker command intent, not ACP methods.

Examples:

- `/cancel`
- `/mode code`
- `/model gpt-5.4`
- `/config mode=code`
- `/list`
- `/load 2`
- `/new`

The IM layer parses command syntax once and passes structured command intent upward.

## Router Responsibilities

`server/internal/im/router.go` remains the IM routing and binding layer only.

Router responsibilities:

- Register channels.
- Maintain `channelID + chatID -> sessionID + watch`.
- Route channel inbound callbacks to the client-facing IM handler.
- Route outbound calls from client/session to the correct channel.
- Append normalized events to session history.

Router must not understand ACP prompt lifecycle beyond carrying typed payloads to the proper channel.

The router fanout semantics remain:

- direct send by `ChannelID + ChatID`
- reply send by `SessionID + Source`
- watcher fanout by `watch=true`
- session broadcast by `SessionID`

## Client and Session Responsibilities

### Client / Session Inbound

The client receives three inbound event shapes from IM:

1. prompt
2. command
3. permission response

Prompt handling:

- Convert inbound prompt to `acp.SessionPromptParams`
- Execute `session/prompt`
- Forward all outbound ACP events back through the router

Command handling:

- Map command intent to WheelMaker or ACP actions
- `/cancel` becomes `session/cancel`
- `/mode`, `/model`, `/config` become `session/set_config_option`
- `/new`, `/load`, `/list` remain client-level commands

Permission response handling:

- Resolve a pending permission request by JSON-RPC request ID
- Respond to the ACP request using that same request ID

### Client / Session Outbound

The client or session forwards:

- `session/update` to `PublishSessionUpdate`
- `session/prompt` final result to `PublishPromptResult`
- `session/request_permission` to `PublishPermissionRequest`
- local non-ACP notices to `SystemNotify`

The client must not collapse prompt completion into `session/update`.

The client must not collapse prompt transport/runtime errors into fake ACP updates.

## Permission Request Correlation

Permission handling uses two different identifiers for two different jobs:

- JSON-RPC `requestID`: protocol response correlation
- `toolCallId`: UI association and display correlation

`requestID` is the only valid key for sending the ACP response.

`toolCallId` remains important because IM may attach the permission UI to an existing tool-call card or command card.

The recommended pending permission state includes:

```go
type PendingPermission struct {
	RequestID  int64
	SessionID  string
	ChatID     string
	ToolCallID string
	Options    []acp.PermissionOption
	CreatedAt  time.Time
}
```

Rules:

- Resolve responses by `RequestID`.
- Use `ToolCallID` for display grouping and card placement.
- Use `SessionID` for session routing and validation.
- Treat `RequestID` as opaque in IM code. It only needs to be preserved and returned.

## Feishu Design

Feishu is split into three files only:

```text
server/internal/im/feishu/
  channel.go
  render.go
  transport.go
```

### `feishu/channel.go`

This file should stay compact and focus on event handling logic.

Responsibilities:

- Receive ACP outbound events and choose the Feishu presentation path.
- Receive Feishu user messages and card actions, then emit prompt, command, or permission-response callbacks.
- Maintain minimal pending state for permission requests and tool-card correlation.
- Coordinate render and transport helpers.

This file should not build large card payloads inline.

This file should not duplicate text/card formatting logic.

### `feishu/render.go`

This file owns Feishu presentation behavior:

- render normal message text
- render thought text
- render tool call cards
- render permission cards
- render plan/config/session summaries
- render prompt result completion state
- render system notifications
- manage card update payload building
- manage reaction or done-mark rendering decisions

The goal is maximum reuse of rendering and payload assembly logic while keeping `channel.go` readable.

### `feishu/transport.go`

This file owns Feishu I/O only:

- websocket or event transport
- send text
- send card
- update card if supported
- send reactions
- mark done
- register inbound message and card action handlers

Transport must not decide ACP behavior.

Transport must not know command semantics.

## Feishu Event Mapping

Recommended mapping in `feishu/channel.go`:

- `agent_message_chunk` -> render normal text
- thought chunk -> render thought style text if enabled
- `tool_call` / `tool_call_update` -> render or update tool call card
- `plan` -> render compact summary or plan card
- `config_option_update` -> render summary text/card
- `available_commands_update` -> optional summary or ignore
- `session_info_update` -> optional summary or metadata refresh
- prompt result `end_turn` -> mark completion
- prompt result `cancelled` -> mark interrupted completion
- prompt result `max_tokens` / `refusal` / others -> render channel-defined completion or warning state
- system notify -> channel-specific system presentation

The exact Feishu UX stays implementation-specific. The design only fixes the boundary and responsibilities.

## ACP Constants Consolidation

Cross-module ACP constants should be consolidated into `server/internal/protocol/acp_const.go`.

This file becomes the canonical source for ACP protocol vocabulary reused across modules.

Move or define there:

- method names
  - `initialize`
  - `session/new`
  - `session/prompt`
  - `session/load`
  - `session/list`
  - `session/set_config_option`
  - `session/cancel`
  - `session/update`
  - `session/request_permission`
  - `fs/*`
  - `terminal/*`
- session update type names
- content block type names
- tool call status values
- tool call kind values
- stop reason values
- config option IDs and category values used across modules

Keep JSON-RPC container types and JSON-RPC error-code definitions in `acp_rpc.go`.

Do not turn `acp_const.go` into a dump for file-local private constants.

## File Naming

Recommended IM file naming after the refactor:

```text
server/internal/im/
  channel.go
  router.go
  history.go

server/internal/im/feishu/
  channel.go
  render.go
  transport.go
```

This naming is preferred over `protocol.go` because the file defines the IM channel contract, not a generic protocol layer.

## Error Handling

- ACP prompt result is always emitted through `PublishPromptResult`, not `session/update`.
- ACP prompt error is always emitted through `SystemNotify`.
- Unsupported or malformed user commands are handled through `SystemNotify` or a direct command result path.
- A permission response with an unknown `requestID` should be ignored or rejected as stale.
- Feishu stale button presses should not corrupt pending permission state.
- Permission UI may use `toolCallId` to attach to an existing card, but final ACP response must still use `requestID`.

## Testing Scope

Channel and router tests should verify:

- prompt inbound path
- command inbound path
- permission response inbound path
- `session/update` outbound routing
- prompt result outbound routing
- system notify outbound routing
- watcher fanout behavior
- direct-send behavior before binding

Feishu tests should verify:

- user message parsing into prompt vs command
- permission request storage by `requestID`
- permission UI association by `toolCallId`
- correct callback emission on button or text response
- rendering reuse through `render.go`
- minimal logic in `channel.go`

Protocol tests should verify:

- all cross-module ACP constants live in `acp_const.go`
- stop reasons and session update names are reused consistently

## Migration Notes

Migration should proceed in small steps:

1. Introduce the new IM channel contract in `im/channel.go`.
2. Update router to use the new typed callbacks and outbound methods.
3. Adapt client/session to send ACP-native outbound events and system notifications.
4. Refactor Feishu into `channel.go`, `render.go`, and `transport.go`.
5. Remove legacy `OutboundACP`, `OutboundEvent`, `DecisionRequest`, and related wrappers once all call sites are migrated.
6. Consolidate ACP constants into `acp_const.go` and replace scattered literal strings.

## Open Questions Resolved

- `session/prompt` result is sent to IM explicitly and is not rewritten into `session/update`.
- prompt error is sent through `SystemNotify`, not a fake ACP update.
- inbound cancel and config changes are folded into command handling.
- `requestID` and `toolCallId` both remain in the design, each with a different responsibility.
- Feishu is split into three files only: `channel.go`, `render.go`, and `transport.go`.
