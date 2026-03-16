# Mock ACP Adapter Design

Date: 2026-03-16

## Goal
Provide a built-in mock ACP adapter for WheelMaker to test ACP flows without external binaries.

## Scope
- Add a new adapter name: `mock`.
- No standalone mock executable.
- Use in-process JSON-RPC transport to emulate ACP agent behavior.
- Scenario routing by prompt input:
  - `1`: text-oriented updates only
  - `2`: fs callbacks
  - `3`: terminal callbacks
  - `4`: permission + tool_call lifecycle
  - `10+`: error and edge cases
- Global command routing (independent from numeric scenarios):
  - `/model <id>`
  - `/mode <id>`
  - `/thought <id>`

## Architecture
1. Add an in-memory mode in `internal/agent/provider/connect.go`.
2. Implement mock ACP server loop in `internal/agent/provider/mock_server.go`.
3. Add `internal/agent/provider/mock/provider.go` using the in-memory conn constructor.
4. Register `mock` adapter in `internal/hub/hub.go`.

## Behavior Matrix
- `initialize`: return protocol v1, capabilities including `loadSession`.
- `session/new`: return sessionId + modes/models/configOptions.
- `session/load`: succeed and keep existing session context.
- `session/prompt`:
  - `1`: emit text/thought/plan/session_info/available_commands updates; stop `end_turn`.
  - `2`: issue `fs/read_text_file` and `fs/write_text_file` callbacks.
  - `3`: issue terminal create/output/wait/release callbacks.
  - `4`: emit tool_call status updates and request permission.
  - `10`: invalid params error (-32602)
  - `11`: method not found (-32601)
  - `12`: internal error (-32603)
  - `13`: stopReason=cancelled
  - `14`: delayed response (slow/timeout style)
- `/model`, `/mode`, `/thought`: update matching config option and emit full `config_option_update`.

## Testing Strategy
- TDD with focused tests first:
  - mock adapter connect/name/close
  - scenario updates for 1/2/3/4
  - config command routing
  - error injection for 10+
- Verify with `go test` on touched packages, then full `go test ./...`.






