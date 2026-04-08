# Server Observability And Self-Recovery (Observation-First) Design

## Context
WheelMaker server already has basic reconnect/retry behavior in `Session.handlePrompt`, but lacks consistent lifecycle logging, long-wait observability, and a unified ACP debug logging standard.

This design focuses on two goals:
1. Strong multi-level operational reporting from startup to runtime failures.
2. Better self-recovery *observability* first (no new auto-recovery actions in this phase).

## Confirmed Scope
- Runtime target: `server/` only.
- Startup coverage: process-level + per-project client wiring details.
- Prompt runtime: high-frequency path logs exceptions only.
- ACP debug stream: full ACP payload logging only when `log.level=debug`.
- IM reporting: only `ERROR` events.

## Non-Goals
- No new auto restart/retry policy beyond current behavior.
- No protocol redesign.
- No monitor/updater/registry-wide refactor in this phase.

## Logging Policy
### 1) Startup and lifecycle logs
Record key and detailed startup nodes:
- process argument parsing start/finish
- config load success/failure
- hub start/run enter/exit
- per-project: store open, client build, channel register, client start

Levels:
- success path: `INFO`
- failure path: `ERROR`

### 2) Runtime exception logs
Include both core and runtime exceptions:
- ACP process start failure / unexpected exit
- `ensureReady` failure
- prompt or stream failure
- session cancel failure
- persist failure
- route binding failure
- permission timeout/invalid response
- IM emit failure

### 3) Long-wait observability
Observe both:
- first-token wait
- in-stream silence

Unified thresholds:
- `60s` -> `WARN`
- `180s` -> `ERROR` + IM error notification

Rate limit:
- only repeated timeout notifications are rate-limited (`60s` window, same session + same timeout class).
- crash/exit class is not rate-limited.

## ACP Debug Log Design
### Format (minimal, content-first)
- outbound: `[acp] > {session method} payload`
- inbound: `[acp] < {session method} payload`
- stderr: `[acp] ! {session method} payload`

Constraints:
- No verbose metadata fields (for example JSON-RPC version tags).
- Keep output focused on content.

### Level routing
- `>` and `<` lines: `shared.Debug(...)` only (active when `log.level=debug`).
- `!` lines: `shared.Error(...)` always.
- All logs must go through shared logger interface (`shared.Debug/Info/Warn/Error`).
- No writer plumbing through business layers.

### Safety
- Mandatory payload redaction for sensitive keys/values (token/authorization/cookie/secret/api keys/password variants).
- Max payload length: `64KB`; overflow is truncated before log output.

### Retention
- Debug logs rotate daily.
- Keep 7 days.

## IM ERROR Notification Shape
Use standard compact operational fields:
- category
- stage (`startup|ready|prompt|stream|persist|im`)
- agent
- sessionID
- action advice

Only `ERROR` sends IM notification.

## Architecture and Touch Points
- `cmd/wheelmaker/main.go`: startup-stage logs.
- `internal/hub/hub.go`: per-project boot/run lifecycle logs.
- `internal/hub/client/session.go` + `client.go`: wait-observe timers, timeout warning/error, timeout IM report/rate-limit.
- `internal/hub/agent/acp_process.go`: unified ACP debug/stderr logging format and redaction/truncation hooks.
- `internal/shared/logger.go`: daily debug rotation + 7-day retention, still exposed via shared log APIs only.

## Testing Strategy
- unit tests for timeout thresholds and duplicate-timeout rate-limit behavior.
- unit tests for ACP log line shaping (`> < !`), redaction, and 64KB truncation.
- unit tests for startup failure/success log points in key flow methods.
- regression tests for existing prompt path behavior to ensure observability changes do not alter current control flow.

## Risks and Mitigations
- Risk: debug payload logs may grow rapidly.
  - Mitigation: debug-level gating + daily rotation + retention + truncation.
- Risk: too many timeout notifications in noisy sessions.
  - Mitigation: timeout-class rate-limit.
- Risk: accidental log API bypass.
  - Mitigation: implement logging through shared logger calls only and add tests for formatter helpers.

## Acceptance Criteria
- Startup and runtime error events are observable at clear severity levels.
- Wait-long scenarios produce `WARN`/`ERROR` exactly at configured thresholds.
- IM receives only `ERROR` class notifications with standard operational fields.
- ACP debug lines follow minimal format and redaction/truncation policy.
- Debug logging remains behind `log.level=debug`.
