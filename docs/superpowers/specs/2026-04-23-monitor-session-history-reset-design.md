# Monitor Session History Reset Design

Date: 2026-04-23
Status: Draft for review

## Goal

Provide one monitor-triggered action that clears persisted session view history, while simplifying the session-view wire shape to match the actual identity model used by the client.

This change should:

- Add a monitor button that clears all rows in `session_prompts` and `session_turns`
- Reset in-memory prompt sequencing state after the clear so new writes restart cleanly
- Remove the no-op `session.markRead` path
- Rename `sessionReadMessage` to `sessionViewMessage` for consistent session-view naming
- Remove redundant `status` and `projectName` from `sessionViewSummary`
- Remove redundant `turnId` from session-view read results and realtime message events
- Keep app/web session rendering working by deriving message identity from session and index fields

## Non-Goals

- Do not delete rows from `sessions`, `projects`, or `route_bindings`
- Do not redesign the overall monitor page layout
- Do not change monitor DB table browsing beyond adding the reset control and refresh behavior
- Do not redesign the persisted `session_turns` schema in this change
- Do not introduce unread-count persistence or a new read-state model

## Context and Constraints

- The current monitor surface already uses `monitor.action`, so the lowest-risk extension point is a new action value rather than a new RPC.
- `SessionRecorder` keeps in-memory `promptState` per session. Clearing only SQLite tables without clearing that memory risks writing future turns using stale prompt indexes.
- `session.markRead` currently does not update any stored or in-memory read state. It only republishes the current summary, and `UnreadCount` remains `0`.
- `turnId` is currently a formatted alias of prompt/turn identity on the server side. The app/web code already has a fallback path that can derive message identity from `sessionId`, `promptIndex`, `turnIndex`, and `updateIndex`.
- The app/web client currently still reads `turnId`, so removing it requires coordinated server and app updates.
- `sessionViewSummary.projectName` is not consumed by the app/web session summary flow.
- `sessionViewSummary.status` is currently normalized by app/web but has no visible product behavior in the session summary flow.

## Design Summary

### 1. Add a Global Session History Reset Action

Extend `monitor.action` with a new action value:

- `clear-session-history`

Behavior:

- Open the client SQLite database used by monitor DB inspection
- Execute one transaction that deletes all rows from:
  - `session_turns`
  - `session_prompts`
- Leave `sessions` untouched so session shells still exist in monitor and app views
- Return success to the caller using the existing `monitor.action` response shape

This action is global to the selected hub database, not scoped to a single session or project.

### 2. Reset Recorder Runtime State After Clear

The hub-side session recorder should expose a narrow reset method that clears its in-memory `promptState` map.

The monitor action path should call that reset after the database clear succeeds.

Required behavior after reset:

- A new incoming prompt for an existing session recreates prompt state starting from prompt index `1` when no prompt rows exist
- A new turn append after reset does not rely on stale in-memory prompt indexes

This prevents the mismatch where tables are empty but runtime state still assumes a later prompt sequence.

### 3. Remove the No-Op Read-State API

Remove the `session.markRead` request handling path and the `SessionRecorder.MarkSessionRead` method.

Rationale:

- The method does not mark anything as read
- It does not mutate unread counters or persistence
- Its continued presence implies semantics the system does not implement

This change also removes the corresponding app/web repository method and any direct callers.

### 4. Unify Session View Message Naming

Rename the internal read DTO:

- `sessionReadMessage` -> `sessionViewMessage`

This is an internal naming cleanup to match the existing `sessionViewSummary` and session-view terminology.

The rename should cover:

- Type definition
- Helper constructors/converters
- `ReadSessionMessages` return type
- Affected tests

### 5. Remove Redundant `turnId` from Session View Payloads

Remove `turnId` from:

- `session.read` response message items
- `registry.session.message` realtime event payloads

The canonical message identity becomes the tuple:

- `sessionId`
- `promptIndex`
- `turnIndex`
- `updateIndex`

App/web should derive `messageId` as:

```text
${sessionId}:${promptIndex}:${turnIndex}:${updateIndex}
```

This keeps the externally visible message identity stable without carrying a second server-generated alias.

### 6. Remove Redundant Summary Fields

Remove these fields from `sessionViewSummary` and all session summary payloads built from it:

- `status`
- `projectName`

Rationale:

- Session summaries are already project-scoped by the current request path, so `projectName` is redundant in this DTO
- Session summary `status` is not used to drive the current app/web session list behavior
- Keeping fewer derived fields reduces protocol drift and cleanup burden when session-view payloads evolve

The summary payload should keep only fields that the session list actually uses, such as session identity, title, updated timestamp, and agent when present.

## Detailed Behavior

## 1) Monitor UI

Add one control in the DB area:

- Label: `Clear Session History`

Interaction:

- User clicks the button
- UI asks for confirmation because the action is destructive for session history
- UI calls the existing `monitor.action` flow with `action = clear-session-history`
- On success, UI reloads DB tables so `session_prompts` and `session_turns` show as empty
- On failure, UI displays the action error in the existing monitor error surface

The button should live near the DB view because the action directly affects the tables shown there.

## 2) Monitor Backend / Transport

No new transport method is added.

Existing path remains:

- dashboard -> monitor backend -> `MonitorAction` -> direct or registry transport -> hub-side `monitor.action`

Only the allowed action set expands to include `clear-session-history`.

## 3) Hub Monitor Core

`MonitorCore.ExecuteAction` should gain a new case for `clear-session-history`.

Execution steps:

1. Open the client DB
2. Begin transaction
3. `DELETE FROM session_turns`
4. `DELETE FROM session_prompts`
5. Commit transaction
6. Trigger recorder prompt-state reset through the owning hub runtime hook

If any SQL step fails, rollback and return the error.

## 4) Session View Wire Model

Revised message item shape for `session.read` and realtime event fallback payload:

```json
{
  "sessionId": "sess-1",
  "promptIndex": 1,
  "turnIndex": 2,
  "updateIndex": 1,
  "content": "{\"method\":\"session.update\"}"
}
```

Removed field:

```json
{
  "turnId": "1.2"
}
```

No replacement server field is needed because identity is derived client-side.

Revised summary item shape:

```json
{
  "sessionId": "sess-1",
  "title": "Build monitor action",
  "updatedAt": "2026-04-23T10:00:00Z",
  "agent": "codex"
}
```

Removed summary fields:

```json
{
  "status": "active",
  "projectName": "proj-1"
}
```

## Testing Plan

Server:

- Add or extend monitor action tests to verify `clear-session-history` deletes rows from both tables and leaves `sessions` intact
- Add or extend session recorder/client tests to verify prompt-state reset causes new writes to restart from clean indexes after a clear
- Update session request tests to verify `session.read` no longer returns `turnId`
- Update session list/read tests to verify summaries no longer return `status` or `projectName`
- Update realtime publish tests to verify `registry.session.message` no longer emits `turnId`
- Remove or update tests that exercise `session.markRead`

App/web:

- Update repository normalization tests so wire messages without `turnId` still produce stable `messageId`
- Update session summary normalization tests to stop expecting `status` in summary payloads
- Remove tests that depend on `markSessionRead` when no longer needed
- Verify session list / session detail flows still render after `turnId` removal

Manual verification:

- Open monitor DB page
- Confirm prompt/turn rows exist
- Click `Clear Session History`
- Confirm both tables are empty and `sessions` rows remain
- Send a new prompt from app/web or IM
- Confirm new prompt/turn rows start from clean indexes and render normally

## Risks and Mitigations

- Risk: clearing DB tables without resetting runtime state causes index drift
  - Mitigation: make recorder reset part of the same action flow after successful DB commit
- Risk: app/web still assumes `turnId` exists
  - Mitigation: update all message normalization paths in app/web in the same change
- Risk: app/web still expects summary `status`
  - Mitigation: remove summary-field normalization and update any affected tests in the same change
- Risk: removing `session.markRead` breaks callers not updated in this repo
  - Mitigation: remove in-repo callers together and keep the protocol change scoped to this repo version

## Open Questions Resolved

- Scope of clear action: global to the selected hub database
- Tables to clear: only `session_prompts` and `session_turns`
- `sessions` retention: keep existing rows
- `MarkSessionRead`: remove because it is a no-op
- `sessionViewSummary.status`: remove because it does not drive current session-list behavior
- `sessionViewSummary.projectName`: remove because the summary is already request-scoped and the field is unused
- `turnId`: remove from session-view wire payloads and rely on index tuple identity