# Session View UpdateIndex Removal Design

Date: 2026-04-28
Status: Draft for review
Scope: `server/internal/hub/client/`, `server/cmd/wheelmaker-monitor/`, `app/web/`
Approach: Remove `updateIndex` from the session-view model, keep realtime delivery turn-scoped, and make `session.read` a checkpoint-based prompt resync API.

## Goal

Remove `updateIndex` as a protocol, storage, and client-state concept while preserving the current ability to merge streaming text and tool updates into stable turn state.

This change should:

- Remove `updateIndex` from session-view server types, SQLite persistence, realtime events, monitor rendering, and app/web types.
- Keep `registry.session.message` as a lightweight realtime notification for the latest turn only.
- Make `session.read` an active resync API driven by a client checkpoint `(promptIndex, turnIndex)`.
- Return prompt snapshots from `session.read` so reconnect logic can repair missed same-turn overwrites.
- Preserve the current IMMessage payload contract where each turn body is still one serialized IMMessage JSON string.

## Non-Goals

- Do not redesign the ACP IMMessage inner shape beyond the already-confirmed `method + param` model.
- Do not turn durable storage into prompt-level blobs.
- Do not preserve backward compatibility for old `updateIndex`-based server or app/web payloads.
- Do not make realtime notifications carry full prompt snapshots on every turn update.
- Do not introduce a new unread-count persistence model in this change.

## Product Decisions Confirmed

1. This is a hard cut. `updateIndex` is removed rather than deprecated.
2. Realtime `registry.session.message` should notify only the latest turn, not a rebuilt full prompt snapshot.
3. `session.read` is an active pull path used for reconnect and repair, not a paginated history feed.
4. The client sends its current checkpoint `(promptIndex, turnIndex)` to `session.read`.
5. `session.read` should return prompt snapshots beginning from the checkpoint prompt, not only newly appended turns.
6. The first prompt returned by `session.read` is the latest full snapshot of the checkpoint prompt.
7. Later prompts, if any, are returned as full snapshots as well.
8. Each prompt snapshot contains `content []string`, where every item is one complete IMMessage JSON string for one turn.

## Current Problems

The current model mixes two different ideas under the same wire shape:

1. Realtime delivery uses `updateIndex` as a version-like dimension for same-turn updates.
2. Durable storage persists the same dimension in `session_turns`.
3. App/web derives message identity from `sessionId:promptIndex:turnIndex:updateIndex`.
4. Reconnect logic depends on replaying update rows instead of fetching the latest stable prompt state.
5. Monitor renders `updateIndex` as a sub-index even though the real business identity is the turn, not the update row.

This creates unnecessary complexity:

- the same tool turn can look like multiple durable messages
- same-turn overwrites require version-aware client logic
- reconnect semantics depend on replaying update history instead of reloading the current state
- storage shape reflects transport detail rather than business state

## Design Summary

### 1. Remove `updateIndex` From the Session-View Identity Model

The canonical session-view identity becomes:

- `sessionId`
- `promptIndex`
- `turnIndex`

`updateIndex` is removed from:

- `sessionTurnMessage`
- `sessionViewMessage`
- `SessionTurnRecord`
- `registry.session.message` payloads
- app/web registry types and message identity logic
- monitor session-history parsing and display

The business object is a turn. Same-turn updates overwrite that turn.

### 2. Keep Realtime Delivery Turn-Scoped

`registry.session.message` continues to be emitted per latest turn update.

Revised payload shape:

```json
{
  "sessionId": "sess-1",
  "promptIndex": 3,
  "turnIndex": 2,
  "content": "{\"method\":\"agent_message\",\"param\":{...}}"
}
```

Semantics:

- if `turnIndex` is new for the prompt, the client appends a new turn
- if `(sessionId, promptIndex, turnIndex)` already exists locally, the client overwrites that turn with the new `content`

The server does not rebuild and publish the full prompt snapshot on every update.

### 3. Make `session.read` a Checkpoint-Based Prompt Resync API

`session.read` becomes a repair path for reconnects and missed updates.

Request shape:

```json
{
  "sessionId": "sess-1",
  "promptIndex": 3,
  "turnIndex": 2
}
```

If the client has no checkpoint, it sends `0, 0` or omits both indexes.

Response shape:

```json
{
  "session": {
    "sessionId": "sess-1",
    "title": "Run command",
    "updatedAt": "2026-04-28T09:00:00Z",
    "agent": "codex"
  },
  "prompts": [
    {
      "sessionId": "sess-1",
      "promptIndex": 3,
      "turnIndex": 2,
      "content": [
        "{\"method\":\"prompt_request\",\"param\":{...}}",
        "{\"method\":\"tool_call\",\"param\":{...}}"
      ]
    },
    {
      "sessionId": "sess-1",
      "promptIndex": 4,
      "turnIndex": 1,
      "content": [
        "{\"method\":\"prompt_request\",\"param\":{...}}"
      ]
    }
  ]
}
```

Semantics:

- if the client sends no checkpoint, return full snapshots for all prompts in the session
- start from the checkpoint prompt, not from the checkpoint turn
- the first returned prompt is always the latest full snapshot of that prompt
- later prompts are also returned as full snapshots
- if the checkpoint prompt no longer exists, return from the earliest available prompt after reset or migration
- if the checkpoint prompt exists and there are no later prompts, return that single prompt snapshot
- an empty `prompts` array is valid only when the session currently has no prompts

This ensures reconnect repair can recover same-turn overwrites that realtime delivery may have missed.

### 4. Keep Internal State Turn-Based

The durable and in-memory model remains turn-based rather than prompt-blob-based.

`SessionRecorder` still keeps:

- `turns map[int64]sessionTurnMessage`
- `turnIndexByKey map[string]int64`

Same-turn merge behavior remains:

- text chunks merge into one text turn
- tool updates merge into one tool turn

But the merged result overwrites the existing turn record keyed by `(sessionId, promptIndex, turnIndex)`.

Prompt snapshots are built only when serving `session.read`, not during every realtime publish.

## Detailed Design

## 1. Realtime `registry.session.message`

### Server Behavior

`SessionRecorder.addMessageTurn` should:

1. determine whether the incoming event creates a new turn or overwrites an existing one
2. merge payloads when the event belongs to an existing text or tool turn
3. persist the latest turn body to the single durable row for that turn
4. publish the latest turn body only

The published `content` remains the serialized IMMessage JSON string for that turn.

### Client Behavior

App/web should treat the event as turn-scoped:

1. locate the prompt by `sessionId + promptIndex`
2. if `turnIndex` exceeds current prompt length, append a new turn
3. if `turnIndex` already exists, overwrite that turn

The client should not derive a new identity from overwrite events.

## 2. `session.read` Request and Response Model

`session.read` no longer behaves like paginated turn history.

### Request

Checkpoint fields:

- `sessionId` (required)
- `promptIndex` (optional, default `0`)
- `turnIndex` (optional, default `0`)

The request does not include pagination markers such as `afterIndex`, `afterSubIndex`, `lastPromptUpdateIndex`, or `lastSubIndex`.

### Response

Use prompt snapshots instead of turn rows.

Recommended DTO:

```json
{
  "sessionId": "sess-1",
  "promptIndex": 3,
  "turnIndex": 2,
  "content": ["...", "..."]
}
```

Field meaning:

- `promptIndex`: prompt identity
- `turnIndex`: current total number of turns in the prompt
- `content`: prompt turn array ordered by ascending turn index

The server should prefer a dedicated `prompts` field in the response rather than reusing `messages`, because the shape is no longer message-like.

### Response Semantics

Given checkpoint `(P, T)`:

1. If prompt `P` exists, return the latest full snapshot of prompt `P`.
2. Return full snapshots for all later prompts.
3. Ignore `T` for slicing within the prompt; it only identifies the client's current position.

Rationale:

- same-turn overwrites can change the content of turn `T` without creating a later turn index
- returning only turns after `T` would miss those overwrites

## 3. Recorder and Merge Logic

### In-Memory Types

Remove `UpdateIndex` from `sessionTurnMessage` and `sessionViewMessage`.

`sessionViewMessage` should be replaced or renamed to a prompt-snapshot DTO if used by `session.read`.

### Merge Path

`mergeTurnMessage` should keep the existing text/tool merge behavior, but the returned value no longer increments any update counter.

After merge:

- `TurnIndex` stays stable
- payload becomes the latest stable turn payload
- persistence overwrites the existing turn row

### Snapshot Builder

Add one server-side builder used only by `session.read`:

1. load ordered turns for a prompt
2. serialize each turn to IMMessage JSON string
3. append to `content []string`
4. set `turnIndex = len(content)`

This builder should not be used by realtime publish.

## 4. SQLite Persistence

### SessionTurnRecord

Remove `UpdateIndex` from `SessionTurnRecord` and all `session_turns` queries.

The durable row shape becomes:

- `session_id`
- `prompt_index`
- `turn_index`
- `update_json`

### Unique Key

The unique identity becomes:

- `project_name`
- `session_id`
- `prompt_index`
- `turn_index`

### Migration

Because existing data may contain multiple rows that differ only by `updateIndex`, migration should preserve the latest row per turn.

Recommended SQLite migration flow:

1. create a new `session_turns_next` table without `update_index`
2. insert one row per `(project_name, session_id, prompt_index, turn_index)` by selecting the row with the greatest previous `update_index`
3. drop old `session_turns`
4. rename `session_turns_next` to `session_turns`
5. recreate indexes for the new key shape

This is still a hard cut because the old field is removed after migration.

## 5. App/Web Changes

### Realtime State

App/web should stop building message identity from:

```text
sessionId:promptIndex:turnIndex:updateIndex
```

Instead, turn identity becomes:

```text
sessionId:promptIndex:turnIndex
```

If prompt-level identity is needed for resync bookkeeping, use:

```text
sessionId:promptIndex
```

### Reconnect Flow

The client stores its current checkpoint:

- current `sessionId`
- current `promptIndex`
- current `turnIndex`

On reconnect:

1. resume websocket or registry subscription
2. call `session.read` with the checkpoint
3. overwrite the checkpoint prompt with the returned prompt snapshot
4. append or replace any later returned prompts
5. resume applying realtime turn notifications

### Rendering

Prompt snapshots are not the rendered UI primitive.

The client should locally expand `content[]` into turn objects for display. That expansion is view logic, not wire identity.

## 6. Monitor Changes

Monitor should stop treating session history as update rows with sub-indexes.

Required changes:

- remove `updateIndex` columns from session-view parsing logic
- remove `subIndex = updateIndex - 1` derivation
- treat the durable session-turn table as the latest turn state table

If monitor wants prompt-level inspection, it can locally group ordered turns by `(sessionId, promptIndex)` and build `content[]` arrays for display.

## 7. Error Handling

### Realtime Path

If a realtime turn payload cannot be serialized, fail the write and publish path for that event. Do not publish a partially built event.

### Read Path

If one stored turn in a prompt cannot be decoded while building a prompt snapshot:

- return the error to the caller for now
- do not silently drop a turn and return a truncated snapshot

Rationale:

- reconnect repair must be deterministic
- silently dropping one turn can corrupt prompt reconstruction more severely than failing the resync

This is stricter than monitor UI behavior because `session.read` is a correctness path, not only a debugging surface.

## 8. Testing

Add or update tests for these behaviors:

1. Same-turn tool and text updates overwrite one durable turn row and no longer produce multiple `updateIndex` variants.
2. `registry.session.message` publishes only the latest turn payload and no longer includes `updateIndex`.
3. `session.read` with checkpoint `(P, T)` returns the latest full snapshot of prompt `P` plus later prompts.
4. `session.read` does not paginate and no longer returns last-index markers.
5. App/web identity logic overwrites `(sessionId, promptIndex, turnIndex)` instead of treating same-turn updates as distinct messages.
6. Migration preserves the latest historical value for each old turn key when collapsing old rows.

## Open Implementation Notes

1. Prefer renaming the read DTO away from `sessionViewMessage` if it now represents prompt snapshots rather than single-turn messages.
2. Keep the IMMessage inner serialization helper shared between realtime publish and read snapshot building.
3. Do not rebuild full snapshots on the write path unless a future product requirement explicitly needs prompt-level realtime fanout.