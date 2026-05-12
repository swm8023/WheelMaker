# Session Finished Cursor File History Design

Date: 2026-05-13
Status: Approved direction for implementation planning
Scope: `server/internal/hub/client/`, `app/web/`, `docs/`
Approach: Move prompt history out of `session_prompts.turns_json` into per-prompt files, replace `done` with a universal turn-level `finished` field, and make reconnect use the latest cached finished turn cursor.

## Goal

Persist session prompt history as files without making reconnect or normal chat reads scan file contents unnecessarily.

This change should:

- Keep the `sessions` SQLite table as the hot session index.
- Migrate existing `session_prompts` rows into prompt snapshot files at startup.
- Stop using `session_prompts` as runtime prompt history storage after migration.
- Add a small `session_sync_json` projection on `sessions`.
- Replace the message envelope field `done` with `finished`.
- Store `prompt_done` as a real turn in the prompt history.
- Make client cache and reconnect logic advance only on `finished: true` turns.

## Non-Goals

- Do not file-store `sessions`, `route_bindings`, `projects`, or `agent_preferences`.
- Do not persist streaming text/thought partial turns in client IndexedDB.
- Do not guarantee recovery of intermediate `tool_call` or `plan` display states after disconnect.
- Do not write prompt snapshot files on every turn update.
- Do not introduce append-only JSONL history in this phase.
- Do not drop `session_prompts` immediately after migration.

## Confirmed Product Decisions

1. Migration is startup-wide, not lazy per `session.read`.
2. `sessions` stays in SQLite and gains one JSON projection field.
3. `session_prompts` is the migration source, then becomes legacy data.
4. Prompt history is stored as one session directory with one JSON file per prompt.
5. Runtime writes a prompt file once, when `prompt_done` completes the prompt.
6. `prompt_done` is a persisted turn with its own `turnIndex`.
7. Every turn carries `finished`.
8. `finished` means "cacheable and usable as a retransmission cursor".
9. `tool_call` and `plan` are `finished: true` even though later same-turn updates may replace their UI state.
10. The product accepts that reconnect may not restore intermediate tool/plan states before prompt completion.
11. `session.read(P, T)` is a delta after the client's finished cursor, not a full prompt snapshot response.
12. The app does not locally persist optimistic `prompt_request` turns; the server-emitted `prompt_request finished=true` turn is authoritative.
13. The server synthesizes a terminal `prompt_done finished=true` turn before moving past an incomplete non-empty prompt.

## Current Problems

The current recorder stores completed prompt turns in `session_prompts.turns_json`, where the value is a JSON array of JSON strings. This has three architectural costs:

1. Prompt history payloads live in SQLite even though SQLite is most useful here as an index.
2. `prompt_done` is published as a realtime event but is not part of the durable turn list.
3. Client reconnect needs special rules around `done`, text seal events, and `prompt_done` watermarks.

These rules make the current Session History Module shallow. Callers must know too much about terminal turn semantics, especially:

- `prompt_done` does not advance the read cursor
- `done` applies only to text/thought turns
- `prompt_done.turnIndex - 1` is used as an expected max turn watermark
- missed terminal text seal events require prompt-local repair

The new design deepens the module by making the persisted stream self-describing: all turns, including `prompt_done`, have an ordered `turnIndex` and a `finished` flag.

## Data Model

### SQLite `sessions`

Keep the existing fields:

```sql
id
project_name
status
agent_type
agent_json
title
created_at
last_active_at
```

Add one field:

```sql
session_sync_json TEXT NOT NULL DEFAULT '{}'
```

The value is intentionally small:

```json
{
  "promptIndex": 12,
  "turnIndex": 5,
  "finished": true
}
```

Field meanings:

- `promptIndex`: latest server-side prompt index for the session.
- `turnIndex`: latest server-side turn index in that prompt.
- `finished`: whether that latest turn is cacheable and can be used as a finished cursor.

This projection is used by `session.list`, reconnect checks, and diagnostics. It is not the source of message body content.

### Prompt Files

Prompt history is stored under the WheelMaker data directory:

```text
session-history/
  <project-key>/
    <session-id>/
      manifest.json
      prompts/
        p000001.json
        p000002.json
```

`manifest.json`:

```json
{
  "schemaVersion": 1,
  "sessionId": "sess-1",
  "projectName": "D:\\Code\\WheelMaker",
  "complete": true,
  "promptCount": 2,
  "migratedAt": "2026-05-13T10:00:00Z"
}
```

Prompt file:

```json
{
  "schemaVersion": 1,
  "sessionId": "sess-1",
  "promptIndex": 2,
  "title": "Fix sync bug",
  "modelName": "gpt-5",
  "startedAt": "2026-05-13T10:00:00Z",
  "updatedAt": "2026-05-13T10:01:00Z",
  "stopReason": "end_turn",
  "turnIndex": 4,
  "turns": [
    {
      "turnIndex": 1,
      "method": "prompt_request",
      "finished": true,
      "content": "{\"method\":\"prompt_request\",\"param\":{\"contentBlocks\":[{\"type\":\"text\",\"text\":\"hello\"}]}}"
    },
    {
      "turnIndex": 2,
      "method": "tool_call",
      "finished": true,
      "content": "{\"method\":\"tool_call\",\"param\":{\"cmd\":\"go test\",\"status\":\"running\"}}"
    },
    {
      "turnIndex": 3,
      "method": "agent_message_chunk",
      "finished": true,
      "content": "{\"method\":\"agent_message_chunk\",\"param\":{\"text\":\"Done\"}}"
    },
    {
      "turnIndex": 4,
      "method": "prompt_done",
      "finished": true,
      "content": "{\"method\":\"prompt_done\",\"param\":{\"stopReason\":\"end_turn\"}}"
    }
  ]
}
```

`content` remains the serialized `IMTurnMessage` JSON string so existing decode helpers can be reused. The surrounding prompt file provides storage metadata and the `finished` envelope.

## Turn Semantics

All session message events and `session.read` messages use this envelope:

```typescript
interface RegistrySessionMessage {
  sessionId: string;
  promptIndex: number;
  turnIndex: number;
  method: string;
  param: Record<string, unknown>;
  finished: boolean;
}
```

Server wire payloads still carry `content` as a JSON string:

```json
{
  "sessionId": "sess-1",
  "promptIndex": 12,
  "turnIndex": 5,
  "finished": true,
  "content": "{\"method\":\"prompt_done\",\"param\":{\"stopReason\":\"end_turn\"}}"
}
```

Turn rules:

| Method | `finished` rule |
| --- | --- |
| `prompt_request` | `true` |
| `user_message_chunk` | `true` |
| `tool_call` | `true` |
| `plan` | `true` |
| `system` | `true` |
| `agent_message_chunk` | `false` while streaming, `true` when sealed |
| `agent_thought_chunk` | `false` while streaming, `true` when sealed |
| `prompt_done` | `true` and stored as the last prompt turn |

For `tool_call` and `plan`, later same-turn updates may replace the in-memory UI state. The finished cursor does not promise recovery of those intermediate state changes after a disconnect. The final prompt snapshot catches the completed state when `prompt_done` is written.

## Server Flow

### Prompt Start

When `prompt_request` arrives:

1. Before advancing to a new prompt, ensure any previous non-empty prompt has a terminal `prompt_done finished=true` turn.
2. Create or advance `sessionPromptState`.
3. Add a `prompt_request` turn with `finished: true`.
4. Publish `registry.session.message`.
5. Update `sessions.session_sync_json` to the current prompt and turn.

The app does not create a local durable `prompt_request` for sent text. It waits for this server turn, and reconnect or `session.read` recovers it if the realtime event is missed.

No prompt file is written yet.

### Prompt Terminal Synthesis

Every non-empty prompt must eventually have a terminal turn. The terminal turn is always `prompt_done finished=true`; `stopReason` explains why it was emitted:

- `end_turn`: normal model completion.
- `cancelled`: user cancellation.
- `error`: server or agent error.
- `interrupted`: the server is starting the next prompt and the previous prompt was still incomplete.
- `server_restart`: startup recovery found an incomplete latest prompt.
- `reload_recovered`: reload/resume recovered an unclosed prompt.

If the server starts prompt 2, prompt 1 must already be terminal. A client that missed prompt 1's terminal turn can call `session.read(1, t)` and receive the synthesized `prompt_done`.

### Streaming Updates

When a text or thought chunk arrives:

1. Merge consecutive chunks into the current text/thought turn.
2. Publish the current turn with `finished: false`.
3. Update `session_sync_json` to the current prompt and turn with `finished: false`.

When a different turn starts or `prompt_done` arrives:

1. Republish the open text/thought turn with the complete text and `finished: true`.
2. Allow clients to cache that turn and advance their finished cursor.

### Tool and Plan Updates

Tool and plan turns are published with `finished: true`.

If the same tool call or plan turn is updated later, the server can republish the same `(promptIndex, turnIndex)` with newer content. Connected clients upsert it in memory. Clients that disconnect may miss intermediate replacements, which is accepted.

### Prompt Finish

When `prompt_done` arrives:

1. Seal any open text/thought turn by publishing `finished: true`.
2. Add `prompt_done` as the next real turn with `finished: true`.
3. Serialize the complete prompt state.
4. Write `prompts/pNNNNNN.json.tmp`.
5. Atomically rename the temp file to `prompts/pNNNNNN.json`.
6. Update `session_sync_json` to the `prompt_done` turn.
7. Publish the `prompt_done` turn.
8. Remove the in-memory prompt state.

Publishing `prompt_done` after the file write ensures a client that immediately asks for prompt repair can read the completed snapshot.

## Client Flow

The client keeps two versions of chat state:

1. In-memory UI state contains all received turns, including `finished: false`.
2. IndexedDB cache contains only `finished: true` turns.

The sync cursor is the latest cached finished turn:

```json
{
  "promptIndex": 12,
  "turnIndex": 5
}
```

Realtime handling:

1. Decode the incoming turn.
2. Upsert it into in-memory UI state by `sessionId:promptIndex:turnIndex`.
3. If `finished: true`, write it to cache and advance the finished cursor.
4. If `finished: false`, do not write it to cache and do not advance the cursor.

Realtime gap detection uses the local finished cursor `(P, T)` and the incoming turn `(Pi, Ti)`:

- `Pi > P + 1`: prompt gap, call `session.read(sessionId, P, T)`.
- `Pi == P + 1` and local prompt `P` is not terminal: prior prompt terminal was likely missed, call `session.read(sessionId, P, T)`.
- `Pi == P + 1` and `Ti != 1`: new prompt start was missed, call `session.read(sessionId, P, T)`.
- `Pi == P` and `Ti > T + 1`: turn gap inside the prompt, call `session.read(sessionId, P, T)`.
- `Pi == P + 1`, prompt `P` is terminal, and `Ti == 1`: accept without repair.

A prompt is terminal only after `prompt_done`.

Reconnect handling:

1. Call `session.list`.
2. Compare the server `sync` projection for the selected session to the local finished cursor.
3. If the server is ahead or the latest server turn is unfinished, call `session.read` with the local finished cursor.
4. Apply returned turns with the same realtime cache rules.

`session.read` no longer needs special `prompt_done` watermark logic because `prompt_done` is a normal turn.

## `session.read`

Request:

```json
{
  "sessionId": "sess-1",
  "promptIndex": 12,
  "turnIndex": 3
}
```

The cursor means: "the client has cached finished turns through this point".

The response is a delta after that cursor. It is not a full snapshot of prompt `P` unless the cursor itself asks for that range, such as `(0, 0)` or `(P, 0)`.

Response:

```json
{
  "session": {
    "sessionId": "sess-1",
    "title": "Fix sync bug",
    "updatedAt": "2026-05-13T10:01:00Z",
    "agentType": "codex",
    "sync": {
      "promptIndex": 12,
      "turnIndex": 5,
      "finished": true
    }
  },
  "prompts": [
    {
      "sessionId": "sess-1",
      "promptIndex": 12,
      "turnIndex": 5,
      "modelName": "gpt-5",
      "durationMs": 5000,
      "finished": true
    }
  ],
  "messages": [
    {
      "sessionId": "sess-1",
      "promptIndex": 12,
      "turnIndex": 4,
      "finished": true,
      "content": "{\"method\":\"agent_message_chunk\",\"param\":{\"text\":\"...\"}}"
    },
    {
      "sessionId": "sess-1",
      "promptIndex": 12,
      "turnIndex": 5,
      "finished": true,
      "content": "{\"method\":\"prompt_done\",\"param\":{\"stopReason\":\"end_turn\"}}"
    }
  ]
}
```

Read rules:

- `(0, 0)` returns all persisted prompts plus the active prompt tail when present.
- `(P, 0)` returns all turns for prompt `P` plus later prompts.
- `(P, T)` returns turns after `T` in prompt `P` plus all turns in later prompts.
- For finished prompts, read from prompt files.
- For the active prompt, read from in-memory `sessionPromptState`.
- Returned unfinished turns are valid UI updates but are not cacheable.

## Startup Migration

Migration runs once during server startup before the file-backed history adapter becomes active.

Flow:

1. Check global store metadata for `prompt_files_v1`.
2. If already migrated, skip.
3. Scan all `sessions` rows.
4. For each session, read its `session_prompts` rows.
5. Decode `turns_json`.
6. Convert each stored turn into a prompt file turn with `finished: true`.
7. Append a `prompt_done` turn when `stop_reason` is non-empty.
8. Write prompt files via temp file and atomic rename.
9. Write the session `manifest.json` last.
10. Update `sessions.session_sync_json`.
11. After every session succeeds, set the global metadata value to `prompt_files_v1`.

The migration is idempotent. A failed run leaves SQLite data untouched and does not set the global completion marker. The next startup can retry.

## Storage Seam

The implementation should introduce a real Session History Store seam:

- SQLite remains the Adapter for session index data.
- File prompt history becomes the Adapter for prompt bodies.
- `SessionRecorder` owns turn semantics and prompt lifecycle.
- The storage Adapter owns file layout, migration, and atomic writes.

This improves Locality: prompt file format, migration, and path rules are concentrated in the history store Module instead of spread across recorder and request handlers.

## Testing Strategy

Server tests should cover:

1. `session_sync_json` round trip on `sessions`.
2. Startup migration from existing `session_prompts.turns_json` to prompt files.
3. Prompt files include `prompt_done` as the final turn.
4. `session.read` returns turns after the finished cursor.
5. Realtime `session.message` uses `finished`, not `done`.
6. Streaming text/thought turns are not finished until sealed.
7. Tool and plan turns are finished immediately.
8. Prompt file write happens before publishing `prompt_done`.

App tests should cover:

1. `finished: false` turns update UI but do not enter cache.
2. `finished: true` turns enter cache and advance the cursor.
3. `prompt_done` advances the cursor as a normal turn.
4. Reconnect sends the latest cached finished cursor.
5. The old `done` field is no longer required by app chat.

## Compatibility

This is a hard protocol change inside app chat. During implementation, update both server and app/web in the same release.

Old `session_prompts` data remains available for migration and manual rollback during the first version. Runtime reads should not silently fall back to stale `session_prompts` rows after the global migration marker is set.

## Documentation Updates

After implementation, update:

- `docs/app-chat-recorder-sync-protocol.zh-CN.md`
- `docs/app-chat-recorder-sync-protocol.md`
- `docs/session-persistence-sqlite.md`

Those documents should describe the implemented protocol. This design remains the historical decision record for the migration.
