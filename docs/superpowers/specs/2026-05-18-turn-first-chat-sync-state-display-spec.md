# Turn-First Chat Sync, State, and Display Spec

## Goal

Make chat synchronization, streaming updates, session status, durable cache, and chat window rendering deterministic while keeping the first implementation simple: active sessions load full raw turns into memory, React renders only a bounded visible window, and IndexedDB stores a raw finished-prefix blob.

This is a review spec. It intentionally does not claim the current implementation satisfies every requirement.

## Scope

In scope:

- Raw turn wire format for `session.message` and `session.read`.
- Server `session.read` continuity and hot gap projection.
- At most one unfinished tail turn per session.
- Active Turn Runtime Set with default capacity 5.
- Full in-memory raw turn stores for active sessions.
- Single Finished Cursor per session.
- Durable raw turn cache as a whole-session blob.
- Serialized Read Repair.
- Session list state from server Session Summary.
- Full lightweight Display Index and bounded `react-virtuoso` rendering.
- `session/gap` rendering and prompt copy behavior.

Out of scope:

- Backwards compatibility with old wire payloads.
- IndexedDB compatibility migration from old `messagesJson`.
- `session.read` pagination.
- IndexedDB chunking or per-turn entries.
- WMT2 content compaction or large artifact externalization.
- Multi-viewer read cursors.

## Wire Requirements

### WIRE-001 Raw Turn Shape

All source-store, read, realtime, and durable-cache turns MUST use:

```ts
type RegistrySessionTurn = {
  turnIndex: number;
  content: string;
  finished: boolean;
};
```

### WIRE-002 No Session ID Inside Turn

`RegistrySessionTurn` MUST NOT contain `sessionId`. Session identity belongs to the enclosing payload or local session key.

### WIRE-003 Realtime Payload

`session.message` payload MUST be:

```json
{
  "sessionId": "sess-1",
  "turn": {
    "turnIndex": 12,
    "content": "{\"method\":\"agent_message_chunk\",\"param\":{\"text\":\"hello\"}}",
    "finished": false
  }
}
```

The app MUST NOT support the old realtime payload shape after this migration.

### WIRE-004 Read Response

`session.read` response MUST be:

```json
{
  "sessionId": "sess-1",
  "latestTurnIndex": 132,
  "session": {
    "sessionId": "sess-1",
    "title": "Build sync",
    "preview": "Build sync",
    "updatedAt": "2026-05-18T12:00:00Z",
    "messageCount": 132,
    "running": true,
    "latestTurnIndex": 132,
    "lastDoneTurnIndex": 120,
    "lastDoneSuccess": true,
    "lastReadTurnIndex": 120
  },
  "turns": [
    {
      "turnIndex": 129,
      "content": "{\"method\":\"agent_message_chunk\",\"param\":{\"text\":\"...\"}}",
      "finished": true
    }
  ]
}
```

The app MUST reject a read response whose top-level `sessionId` differs from the requested session.

### WIRE-005 No Realtime Summary

`session.message` MUST NOT include Session Summary. Session Summary is provided by `session.updated`, `session.list`, `session.read`, and `session.markRead`.

### WIRE-006 Raw Content Preservation

The app MUST store incoming `turn.content` as the original string. It MUST NOT parse-normalize and stringify content when writing source stores or IndexedDB.

## Server Requirements

### SRV-001 Turn Index

The server MUST expose session-local `turnIndex` values starting at `1`. `0` MUST NOT be emitted as a real turn.

### SRV-002 Continuous Write

The WMT2 hot write path MUST reject skipped turn indexes. A write beginning at `N` is valid only when the previous persisted latest turn is `N - 1`.

### SRV-003 Non-Empty Content

The WMT2 hot write path MUST reject empty turn content.

### SRV-004 Read Continuity

`session.read(afterTurnIndex=K)` MUST return turns in strictly increasing `turnIndex` order and MUST NOT skip a turn within the returned range.

### SRV-005 No Read Pagination In This Iteration

The current implementation SHOULD return the full known range after `K`. If pagination is added later, the page MUST be a continuous prefix.

### SRV-006 At Most One Unfinished Tail

For a session, the server MUST expose at most one `finished:false` turn. If present, it MUST be the latest returned/published turn.

### SRV-007 Unfinished Method Limit

`finished:false` MUST be used only for text streaming methods: `agent_message_chunk` and `agent_thought_chunk`. `prompt_request`, `prompt_done`, `tool_call`, and `agent_plan` MUST use `finished:true`; tool running state belongs in `param.status`.

### SRV-008 Seal Before Larger Turn

Before publishing a larger `turnIndex`, the server MUST publish any previous open text turn at the same `turnIndex` with `finished:true`.

### SRV-009 Empty Read

`session.read` MAY return an empty `turns` array only when `latestTurnIndex <= afterTurnIndex`.

### SRV-010 Hot Gap Projection

If `session.read` detects a missing durable slot for a turn that should exist, it MUST synthesize a finished hot gap raw turn:

```json
{
  "turnIndex": 42,
  "content": "{\"method\":\"session/gap\",\"param\":{\"reason\":\"missing_turn\",\"turnIndex\":42}}",
  "finished": true
}
```

### SRV-011 No Gap Backfill Write

The server MUST NOT write synthesized hot gap turns back to WMT2 during `session.read`.

### SRV-012 Archive Gap Separation

Hot read gap turns MUST use method `session/gap`. Archive-specific placeholders MUST remain `session/archive_gap`.

### SRV-013 prompt_done Publish Order

When a prompt finishes, the server MUST persist turns, publish any sealed text turn, publish `prompt_done`, and then publish `session.updated`.

## Client Store Requirements

### CLT-001 Raw Source Stores

The app MUST keep source stores as raw `RegistrySessionTurn`, not decoded message objects.

### CLT-002 Store Split

Each active session MUST maintain:

- Finished Store for full in-memory `finished:true` raw turns.
- Live Turn Buffer for unfinished raw turns.
- Finished Cursor.
- Read in-flight flag.
- Repair pending flag.

### CLT-003 Active Runtime Set

The app MUST maintain an Active Turn Runtime Set with default capacity `5`. The capacity SHOULD be configurable later.

### CLT-004 Selected Membership

The selected session MUST always be in the Active Turn Runtime Set and MUST NOT be evicted.

### CLT-005 New and Resumed Sessions

New sessions and imported/resumed sessions MUST enter the Active Turn Runtime Set and become selected.

### CLT-006 Active Set Consumption

Sessions in the Active Turn Runtime Set MUST fully consume `session.message`, update source stores, maintain Finished Cursor, run Read Repair, and write Durable Turn Cache.

### CLT-007 Outside Active Set

Sessions outside the Active Turn Runtime Set MUST NOT parse `turn.content`, update source stores, write Durable Turn Cache, or trigger Read Repair from `session.message`. Unknown outside-set session messages MAY trigger project session list refresh.

### CLT-008 Eviction

On capacity pressure, eviction MUST prefer non-selected, non-running, least-recently-used sessions. If all non-selected sessions are running, the least-recently-used non-selected session MAY be evicted.

### CLT-009 Flush Before Eviction

Dirty session cache MUST be flushed before eviction. Flush failure MUST NOT block eviction.

### CLT-010 Single Cursor

Each chat session MUST have exactly one Finished Cursor. Legacy sync-index/sub-index cursor layers MUST be removed from the active model.

### CLT-011 Cursor Calculation

The Finished Cursor MUST be the largest contiguous turn index starting at `1` for which Finished Store contains a `finished:true` turn.

### CLT-012 Sparse Finished Store

Finished Store MAY contain finished turns beyond a hole. Such turns MUST NOT advance Finished Cursor until earlier turns are present.

### CLT-013 Live Non-Durable

Live Turn Buffer turns MUST NOT be written to Durable Turn Cache and MUST NOT advance Finished Cursor.

### CLT-014 Finished Wins Same Index

When a turn index exists in both Finished Store and Live Turn Buffer, the finished turn MUST be authoritative and the matching live turn MUST be removed.

### CLT-015 Same-Index Updates

For realtime updates, `finished:false` MAY replace an earlier same-index `finished:false`; `finished:true` MUST replace same-index live. A realtime `finished:false` MUST NOT replace an existing `finished:true`.

### CLT-016 Decoded View Model

Decoded chat message objects MAY be created only for rendering, copy, prompt status, preview, or selected prompt_done handling. Sync, cursor, and cache logic MUST NOT depend on decoded `method` or `param`.

## Durable Cache Requirements

### DB-001 Raw Turns Blob

IndexedDB chat content MUST store raw turns in `turnsJson`, not decoded messages in `messagesJson`.

### DB-002 Cursor Metadata

IndexedDB chat session index MUST retain `cursorJson.turnIndex` as the durable finished prefix cursor.

### DB-003 No Compatibility Migration

The app MUST NOT migrate old `messagesJson` decoded-message cache rows. If the DB schema version or chat content shape is old/incompatible, the app MUST clear all local persistent cache except token/auth credentials and recreate IndexedDB tables from the current schema.

### DB-004 Durable Prefix

Durable Turn Cache MUST store only turns `1..Finished Cursor`.

### DB-005 Cursor Reconcile

On hydrate, the app MUST compute the actual continuous finished prefix from `turnsJson` and use `min(cursorJson.turnIndex, actualPrefix)` as the repaired cursor. It MUST discard durable turns beyond the repaired cursor and persist the corrected state.

### DB-006 Debounced Persist

Finished Store / Finished Cursor changes MUST update memory immediately and schedule IndexedDB persist with a 5 second debounce.

### DB-007 Required Flush Boundaries

The app MUST flush dirty durable cache before LRU eviction, page hide/unload, reload/archive/delete, and selected `prompt_done`.

### DB-008 Flush Failure

IndexedDB flush failure MUST NOT block UI or LRU eviction. The next activation MUST recover by `session.read(after=oldCursor)`.

## Read Repair Requirements

### RR-001 Read Cursor

Read Repair MUST use the current Finished Cursor as `afterTurnIndex`.

### RR-002 Activation Read

When a session first enters the Active Turn Runtime Set, the app MUST hydrate durable cache and then unconditionally call `session.read(after=Finished Cursor)`.

### RR-003 No Repeat Activation Read

Selecting a session already in the Active Turn Runtime Set MUST NOT unconditionally call read again. It uses the in-memory source stores and any existing repair state.

### RR-004 Realtime Gap Trigger

For active set sessions, if a realtime turn arrives with `turnIndex > Finished Cursor + 1`, the app MUST trigger Read Repair, regardless of `finished`.

### RR-005 Contiguous Live No Read

If `Finished Cursor = K` and the app receives `turnIndex = K + 1` with `finished:false`, it MUST NOT trigger read solely for that turn.

### RR-006 Live Does Not Bridge Gap

Live Turn Buffer MUST NOT advance the read cursor or bridge a gap. If `Finished Cursor = 10`, live `11 false` exists, and `12 true` arrives, the app MUST trigger read after 10.

### RR-007 Reconnect

After reconnect, the app MUST call `session.read(after=Finished Cursor)` for active set sessions. It MUST NOT read turns for sessions outside the active set.

### RR-008 Single In-Flight Repair

Each session MUST have at most one Read Repair in flight.

### RR-009 Pending Flag

If another repair trigger occurs while a Read Repair is in flight, the app MUST set a Repair Pending Flag. After read completes, it MUST re-check current store state rather than queueing one request per trigger.

### RR-010 Read Range Authority

The read result for `[afterTurnIndex + 1, responseLastTurnIndex]` MUST be authoritative. The app MUST remove old finished/live turns in that range before applying response turns.

### RR-011 Response Shape Validation

The app MUST reject or ignore a read response that is non-continuous, has more than one `finished:false`, or has `finished:false` before the last returned turn.

### RR-012 Empty Response

If `session.read` returns no turns while `latestTurnIndex > requested Finished Cursor`, the app MUST NOT advance Finished Cursor and SHOULD keep the session eligible for later repair.

### RR-013 Stale Local Cursor

If `latestTurnIndex < requested Finished Cursor`, the app MUST treat local state as stale, clear that session cache, and perform a full read from `0`.

## Session Status Requirements

### STAT-001 Summary Source of Truth

Session list status MUST be derived only from server Session Summary.

### STAT-002 Summary Sources

The app MAY update session list state from `session.list`, `session.updated`, `session.read.session`, and `session.markRead.session`.

### STAT-003 No Message List Patch

`session.message` MUST NOT update session title, preview, running, done, success, read cursor, unread count, or completed flags.

### STAT-004 Running

If `session.running === true`, the list MUST display running state.

### STAT-005 Completed Unviewed

If `running !== true` and `lastDoneTurnIndex > lastReadTurnIndex` and `lastDoneSuccess !== false`, the list MUST display completed-unviewed state.

### STAT-006 Failed Unviewed

If `running !== true` and `lastDoneTurnIndex > lastReadTurnIndex` and `lastDoneSuccess === false`, the list MUST display failed-unviewed state.

### STAT-007 Composer Local State

The client MAY keep composer-local pending/running/cancelling state, but that state MUST only affect the composer/current-session controls.

### STAT-008 Selected markRead

When the selected visible session decodes a raw turn as `prompt_done`, the client SHOULD call `session.markRead(lastReadTurnIndex=turn.turnIndex)`.

### STAT-009 Background No markRead

When a non-selected active session receives `prompt_done`, the client MUST NOT call `session.markRead`. Sessions outside the active set do not parse message content and therefore cannot mark read from realtime messages.

## Display Requirements

### UI-001 Derived Display View

The displayed chat turn list MUST be derived from source stores through Display Index and virtualizer state. It MUST NOT be a source store or cache.

### UI-002 Full Source, Bounded React

Active sessions MAY keep full raw turns in memory, but mounted React chat row components and DOM MUST contain only virtualizer visible + overscan items.

### UI-003 Merge Sources

Display Index MUST be derived by merging Finished Store and Live Turn Buffer by raw Turn Index, with finished turns winning conflicts.

### UI-004 Raw Turn Coordinates In Display Items

The app MUST NOT use a manual raw turn range as the primary scroll/render state. Raw `turnIndex` MUST remain metadata on each Display Item and continue to define copy ranges, gap checks, and cursor semantics.

### UI-005 No Scroll Read

Scrolling up or down MUST NOT trigger `session.read`; active sessions already have full source turns in memory.

### UI-006 Tail Initial Position

When a selected session opens, the app MUST build the full lightweight Display Index and initialize the virtualizer at the latest Display Item with end alignment.

### UI-007 Upward Scroll

When the user scrolls upward, the virtualizer MUST compute visible + overscan items from the full Display Index. The app MUST NOT move a manual raw turn range or trigger server read.

### UI-008 Downward Scroll

When the user scrolls downward, the virtualizer MUST compute visible + overscan items from the full Display Index. The app MUST NOT move a manual raw turn range or trigger server read.

### UI-009 Tail Lock

When the virtualized list reaches the latest Display Item and the scroll container is within the bottom threshold, the app MUST enter tail-lock mode.

### UI-010 New Turns While Following

In tail-lock mode, new selected-session turns MUST update Display Index and scroll to the latest Display Item.

### UI-011 New Turns While Reading Earlier

Outside tail-lock mode, new selected-session turns MUST update source stores but MUST NOT force the window to jump to bottom. The app SHOULD show a scroll-to-bottom affordance.

### UI-012 Hidden Tool Stability

Hiding tool/thought turns MUST NOT affect raw turn cursor/copy semantics, and hidden turns MUST NOT enter the visible-height Display Index.

### UI-013 Full Display Index

The selected session Display Index MUST cover the full selected active session source turns after hide/show filters. It MUST remain lightweight enough to keep in memory.

### UI-014 DOM Bound

DOM node count MUST be bounded by virtualizer visible + overscan items, not by the number of turns in the selected session.

### UI-015 Gap Rendering

`session/gap` MUST render as a lightweight unrecoverable message placeholder, not as assistant/user/tool content.

### UI-016 Copy Raw Range

`prompt_done` copy range MUST be found by raw Turn Index boundaries from `prompt_done` back to nearest preceding `prompt_request` using full active source store, not only the current window.

### UI-017 Copy Type Filter

Within a valid copy range, the client MUST copy only copyable decoded turn types such as `agent_message_chunk`.

### UI-018 Copy Gap Disable

If a prompt copy range contains `session/gap`, copy MUST be disabled.

### UI-019 Virtuoso Virtualizer

The displayed chat list MUST use `react-virtuoso` through a local wrapper component named `ChatVirtuosoTurnList` or equivalent local module. Browser scrollbar behavior MUST NOT be derived only from the currently mounted DOM nodes.

### UI-019A Main Orchestration Boundary

`main.tsx` MUST NOT directly own wire parsing, active runtime set internals, read repair state machine, durable persist debounce, Display Index construction, or Virtuoso list mechanics. These responsibilities MUST live in focused chat/persistence modules with `main.tsx` acting as the selected-session and event-flow orchestrator.

### UI-020 Display Index

The app MUST build a lightweight Display Index from source raw turns before rendering. Each display item MUST include at least `key`, `turnIndex`, `kind`, `finished`, `contentRevision`, and an estimated height. Display Items MAY include `affordance` for option replies or confirmation replies.

### UI-021 Display Index Is Not Message Storage

Display Index entries MUST NOT store full decoded messages, markdown render output, or copied message text. Source stores remain raw-turn stores.

### UI-022 Shallow Parse Budget

The app MAY shallow-parse all active selected source turns to classify display items, but MUST NOT fully decode markdown, code blocks, rich component props, or copy text outside visible + overscan items.

### UI-023 Hidden Turns Scroll Height

Hidden tool/thought turns MUST NOT contribute visible scrollbar height. They SHOULD be omitted from the Display Index rather than represented as positive-height items.

### UI-024 Gap Turns Scroll Height

`session/gap` turns MUST enter the Display Index and contribute a small visible placeholder height.

### UI-025 Lazy Height Measurement

Unmounted items MUST use estimated height. Exact height MUST be measured only for mounted visible + overscan items by `react-virtuoso` internal measurement state. The app MUST NOT maintain a second height cache keyed by session or Display Item.

### UI-026 No Full Pre-Render For Scrollbar

The app MUST NOT pre-render, mount, or fully decode all turns to compute an exact scrollbar height.

### UI-027 Height Correction Anchor

When measured height replaces an estimate outside tail-lock mode, the app MUST preserve the active viewport anchor so visible content does not jump.

### UI-028 Viewport Anchor Invariant

Outside tail-lock mode, window expansion, window trimming, display index updates, height corrections, and streaming content growth MUST preserve the current anchor item's viewport offset.

### UI-029 Tail Lock Invariant

In tail-lock mode, new selected-session turns and streaming growth MUST keep the viewport scrolled to the latest item.

### UI-030 Special Rendering Classification

Protocol-level special UI such as prompt request/done separators, plans, tool states, and gaps MUST be selected by Display Item kind/subtype produced during shallow parse, not by ad hoc guessing inside mounted React components. Assistant text affordances such as A/B/C choices and Chinese confirmation replies MUST remain affordances on the owning text Display Item, not separate scrolling items.

### UI-031 Virtualizer Remount State

Any UI state that must survive virtualizer unmount/remount, including expanded tool state, confirmation selection, input state, copy state, and choice state, MUST be stored by `sessionId + item.key` or derived from raw turns / Session Summary.

### UI-032 Streaming Height Throttle

Streaming tail height measurement SHOULD be coalesced with `requestAnimationFrame` or a similar throttle so a chunk stream does not cause layout work for every chunk.

## Acceptance Tests

The implementation is accepted when these scenarios pass:

1. Server realtime event uses `{sessionId, turn}` and turns do not contain `sessionId`.
2. Server read response uses `{sessionId, latestTurnIndex, session, turns}` and turns do not contain `sessionId`.
3. App rejects old wire message shape after the migration.
4. Incompatible old IndexedDB/local persistent cache is cleared and tables are recreated, preserving only token/auth credentials.
5. Server read returns `session/gap` for a missing WMT2 durable slot and does not mutate WMT2.
6. Server read returns at most one unfinished tail, and it is last.
7. Client active set default capacity is 5, selected is never evicted, and evicted dirty sessions flush first.
8. Set outside `session.message` does not parse `turn.content`, write cache, or trigger read.
9. First activation hydrates durable cache and unconditionally reads after Finished Cursor.
10. Selecting an already-active session does not unconditionally read again.
11. Client cache `1,2,4` hydrates to Finished Cursor `2` and durable prefix `1,2`.
12. Realtime `turnIndex=12 finished:false` with cursor `10` triggers read after `10`.
13. Realtime `turnIndex=11 finished:false` with cursor `10` does not trigger read by itself.
14. Read result `11 finished:true, 12 finished:false` advances cursor to `11` and leaves `12` live.
15. Finished turn `12` arriving later deletes live turn `12`, advances cursor, and schedules durable persist.
16. Durable persist is debounced at 5 seconds and required flush boundaries write dirty cache.
17. `session.message` does not patch session list title/preview/running/done/read.
18. Realtime `prompt_done` for selected session calls markRead but does not locally set list completed.
19. Realtime `prompt_done` for background active session does not call markRead.
20. Session list visual state changes only after Session Summary merge.
21. Initial selected chat builds a full lightweight Display Index but mounts only the tail virtualized view.
22. Scrolling up/down is handled by `react-virtuoso` without server read or manual raw turn range movement.
23. Hiding tool calls does not affect raw turn cursor/copy semantics and does not contribute visible scroll height.
24. New turns follow only when the selected virtualized list is in tail-lock mode.
25. `session/gap` is shown as a placeholder and disables prompt copy.
26. The mounted chat DOM remains bounded while the virtualizer scrollbar represents the logical visible Display Index height.
27. The app does not decode/render all source turns to compute scroll height.
28. Hidden tool/thought turns do not contribute visible scrollbar height.
29. A gap turn appears as a visible placeholder and contributes placeholder height.
30. Scrolling up through long history keeps DOM bounded and does not visually jump while item heights are corrected.
31. Scrolling down through long history keeps DOM bounded and does not visually jump while item heights are corrected.
32. While at bottom, streaming chunks and new turns keep the viewport tail-locked.
33. While scrolled up, streaming chunks and new turns do not force the viewport to jump to bottom.
34. Confirmation/choice affordances and prompt/tool/gap render items still display correctly after virtualizer unmount/remount.
35. `main.tsx` delegates wire parsing, active runtime set, read repair, durable persist, Display Index, and virtualizer mechanics to focused modules.
36. Full app/web typecheck and focused tests pass.
37. Server Go tests pass.
