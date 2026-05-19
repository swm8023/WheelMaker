# Turn-First Chat Sync State Display Refactor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Refactor chat sync so server/app use raw turns, in-memory runtime sessions sync correctly, session status comes from server summary, and React renders bounded virtualized chat rows over full selected-session turns.

**Architecture:** Server exposes raw `RegistrySessionTurn` in both realtime and read paths, with top-level `sessionId` and read summary. App stores raw turns in source stores and IndexedDB, keeps unbounded in-memory runtime stores for sessions that receive messages, hydrate, read, or become selected, performs serialized read repair from Finished Cursor, builds a lightweight Display Index for the selected session, and decodes raw turns only for mounted virtualized rendering/copy/status.

**Tech Stack:** Go server, WMT2 file store, TypeScript React app/web, `react-virtuoso`, IndexedDB workspace cache, Jest, Go tests.

---

## Review Gate

Do not implement until these documents are approved:

- `CONTEXT.md`
- `docs/chat-turn-sync-state-display-design.zh-CN.md`
- `docs/superpowers/specs/2026-05-18-turn-first-chat-sync-state-display-spec.md`
- `docs/session-management-and-sync.zh-CN.md`

## Target File Map

Server:

- Modify `server/internal/hub/client/session_turn_files.go`: read-time `session/gap` projection.
- Modify `server/internal/hub/client/session_recorder.go`: raw turn wire shape, at-most-one unfinished tail invariant, prompt_done publish order, read response summary.
- Modify `server/internal/hub/client/client.go`: `session.read` response shape.
- Modify server tests in `server/internal/hub/client/*_test.go`.

App wire/types:

- Modify `app/web/src/types/registry.ts`: add `RegistrySessionTurn`, update event/read response types.
- Modify `app/web/src/services/registryRepository.ts`: normalize raw turn wire without decoding content.
- Create `app/web/src/chat/chatWire.ts`: validate raw turn envelopes, reject old flat payloads, and keep wire parsing outside `main.tsx`.
- Modify `app/web/src/main.tsx`: delegate event payload validation to `chatWire.ts`.

App persistence:

- Modify `app/web/src/services/workspacePersistence.ts`: replace `messagesJson` with `turnsJson`, bump DB/cache schema, clear all incompatible local persistent cache except token/auth credentials, recreate tables.
- Modify `app/web/src/services/workspaceStore.ts`: store raw turns, retain cursorJson, sanitize raw durable prefix.

App sync:

- Create `app/web/src/chat/chatTurnStores.ts`: raw Finished Store / Live Turn Buffer / cursor / read repair helpers.
- Maintain in-memory runtime stores in `main.tsx` or a focused future `chatRuntimeStore.ts`: no capacity limit, no session-count eviction, reconnect read trigger from current store keys.
- Create `app/web/src/chat/chatReadRepair.ts`: serialized per-session read repair orchestration.
- Create `app/web/src/chat/chatDurablePersist.ts`: dirty tracking, 5s debounce, flush boundaries.
- Create `app/__tests__/web-chat-turn-stores.test.ts`.
- Modify `app/web/src/chatSync.ts`: keep compatibility exports if needed, delegate to raw turn helpers or remove obsolete decoded-store semantics.
- Modify `app/web/src/main.tsx`: orchestrate selected session and event flow only; do not directly own runtime store internals, read repair, or durable persist.

App status/display:

- Modify `app/web/src/chatSessionState.ts`: list state only from summary.
- Delete/retire `app/web/src/chat/chatTurnWindow.ts`: replace manual raw turn range rendering with Display Index plus `ChatVirtuosoTurnList`.
- Modify `app/web/src/chat/chatCopyRange.ts`: decode full selected source range, reject `session/gap`.
- Modify render branches in `app/web/src/main.tsx`: render `ChatVirtuosoTurnList` and delegate mounted-item decoding/rendering to chat display modules.
- Modify related Jest tests.

Docs:

- Modify `docs/session-management-and-sync.zh-CN.md` after implementation.

## Task 1: Server Raw Turn Wire and Read Response

**Files:**

- Modify: `server/internal/hub/client/session_turn_files.go`
- Modify: `server/internal/hub/client/session_recorder.go`
- Modify: `server/internal/hub/client/client.go`
- Modify/Test: `server/internal/hub/client/client_test.go`
- Modify/Test: `server/internal/hub/client/session_turn_files_test.go`

- [ ] **Step 1: Add server tests for new wire shape**

Add tests asserting realtime publish payload:

```json
{
  "sessionId": "sess-1",
  "turn": {
    "turnIndex": 1,
    "content": "{\"method\":\"prompt_request\",\"param\":{...}}",
    "finished": true
  }
}
```

and asserting `turn` does not contain `sessionId`.

- [ ] **Step 2: Add read response tests**

Assert `session.read` returns:

```json
{
  "sessionId": "sess-1",
  "latestTurnIndex": 3,
  "session": {"sessionId":"sess-1","latestTurnIndex":3},
  "turns": [
    {"turnIndex":1,"content":"...","finished":true}
  ]
}
```

and no `sessionId` inside `turns[]`.

- [ ] **Step 3: Add hot gap test**

Corrupt a persisted slot and assert `session.read` returns a raw `session/gap` turn at that index without mutating WMT2.

- [ ] **Step 4: Implement raw turn structs**

Use a raw turn DTO:

```go
type sessionTurnWire struct {
    TurnIndex int64  `json:"turnIndex"`
    Content   string `json:"content"`
    Finished  bool   `json:"finished"`
}
```

Use this inside realtime `turn` and read `turns`.

- [ ] **Step 5: Implement read response summary**

Change `session.read` to return top-level `sessionId`, `latestTurnIndex`, `session`, and `turns`.

- [ ] **Step 6: Implement hot gap projection**

Synthesize:

```json
{"method":"session/gap","param":{"reason":"missing_turn","turnIndex":N}}
```

for missing durable slots in read projection only.

- [ ] **Step 7: Run focused server tests**

Run:

```powershell
cd server
go test ./internal/hub/client -run "Session.*Read|Session.*Publish|Turn.*Gap" -count=1
```

Expected: PASS.

## Task 2: Server Unfinished Tail Invariant

**Files:**

- Modify: `server/internal/hub/client/session_recorder.go`
- Modify/Test: `server/internal/hub/client/client_test.go`

- [ ] **Step 1: Add tests for seal-before-next-turn**

Assert that when a second text turn starts, the previous text turn is published again with the same `turnIndex` and `finished:true` before the larger turn.

- [ ] **Step 2: Add read test for at most one unfinished tail**

Create active live state and assert `session.read` returns finished prefix plus at most one unfinished last turn.

- [ ] **Step 3: Enforce method semantics**

Ensure only `agent_message_chunk` and `agent_thought_chunk` can be `finished:false`. `tool_call`, `agent_plan`, `prompt_request`, `prompt_done` remain `finished:true`.

- [ ] **Step 4: Adjust prompt_done publish order**

Ensure prompt finish persists turns, publishes sealed text, publishes `prompt_done`, then publishes `session.updated`.

- [ ] **Step 5: Run focused tests**

Run:

```powershell
cd server
go test ./internal/hub/client -run "PromptDone|Finished|Streaming|Publish" -count=1
```

Expected: PASS.

## Task 3: App Raw Types and Wire Parsing

**Files:**

- Modify: `app/web/src/types/registry.ts`
- Modify: `app/web/src/services/registryRepository.ts`
- Create: `app/web/src/chat/chatWire.ts`
- Modify/Test: app web wire/parser tests if existing, otherwise add focused tests in `app/__tests__/web-chat-sync-reconcile.test.ts`

- [ ] **Step 1: Add raw turn types**

Add:

```ts
export interface RegistrySessionTurn {
  turnIndex: number;
  content: string;
  finished: boolean;
}
```

Update read response and message event payload types to use `turn` / `turns`.

- [ ] **Step 2: Normalize raw turns without parsing content**

`chatWire.ts` should expose `normalizeSessionWireTurn`, `normalizeSessionMessagePayload`, and `normalizeSessionReadPayload`. `normalizeSessionWireTurn` should validate:

- `turnIndex > 0`
- `content` non-empty string
- `finished === true` or false default

It must not JSON.parse `content`.

- [ ] **Step 3: Reject old wire shape**

After migration, old payloads with top-level `turnIndex/content/finished` but no `turn` should be ignored or rejected.

- [ ] **Step 4: Validate read sessionId**

If `session.read` response `sessionId` differs from request, throw or ignore response.

- [ ] **Step 5: Run parser tests**

Run:

```powershell
cd app
npm test -- --runInBand __tests__/web-chat-sync-reconcile.test.ts
```

Expected: PASS after test updates.

## Task 4: IndexedDB Raw Turns Schema

**Files:**

- Modify: `app/web/src/services/workspacePersistence.ts`
- Modify: `app/web/src/services/workspaceStore.ts`
- Modify/Test: persistence/store tests if present; otherwise add unit coverage around store helpers.

- [ ] **Step 1: Rename content field**

Change chat session content from `messagesJson` decoded messages to `turnsJson` raw turns.

- [ ] **Step 2: Bump schema/cache version**

Do not migrate old rows. If old schema/version/shape is detected, preserve token/auth credentials, delete all other local persistent workspace/app cache, and recreate IndexedDB tables from the current schema.

Implementation rule:

```ts
// Pseudocode boundary, not final API.
const authSnapshot = await readAuthTokenSnapshot();
await deleteWorkspacePersistentDatabases();
await openWorkspaceDatabaseWithCurrentSchema();
await restoreAuthTokenSnapshot(authSnapshot);
```

- [ ] **Step 3: Retain cursorJson**

Keep `wm_chat_session_index.cursorJson.turnIndex`.

- [ ] **Step 4: Sanitize durable prefix**

On hydrate, compute actual continuous finished raw prefix and use:

```ts
Math.min(cursorJson.turnIndex, actualPrefix)
```

Discard durable turns beyond repaired cursor and persist corrected state.

- [ ] **Step 5: Store only raw finished prefix**

`rememberChatSessionContent` must write only `1..Finished Cursor` raw turns.

- [ ] **Step 6: Run typecheck**

Run:

```powershell
cd app
npm run tsc:web
```

Expected: PASS.

## Task 5: Raw Turn Store Helpers

**Files:**

- Create: `app/web/src/chat/chatTurnStores.ts`
- Create: `app/__tests__/web-chat-turn-stores.test.ts`

- [ ] **Step 1: Add tests**

Cover:

- cursor ignores holes (`1,2,4` => `2`)
- live does not advance cursor
- same-index false updates false
- same-index true absorbs live
- realtime `12 false` with cursor `10` triggers read
- realtime `11 false` with cursor `10` does not trigger read
- live `11 false` plus incoming `12 true` triggers read after `10`
- read response replaces covered range
- invalid read response with middle unfinished is rejected

- [ ] **Step 2: Implement raw store state**

```ts
export type ChatTurnStoreState = {
  finished: RegistrySessionTurn[];
  live: RegistrySessionTurn[];
  cursor: {turnIndex: number};
};
```

- [ ] **Step 3: Implement helpers**

Required helpers:

```ts
createEmptyChatTurnStore()
hydrateFinishedStore(turns)
getFinishedCursor(turns)
getDurableTurnPrefix(turns, cursor)
mergeRealtimeTurn(state, turn)
applySessionReadResult(state, afterTurnIndex, turns, latestTurnIndex)
buildMergedRawTurns(state)
```

- [ ] **Step 4: Run tests**

Run:

```powershell
cd app
npm test -- --runInBand __tests__/web-chat-turn-stores.test.ts
```

Expected: PASS.

## Task 6: In-Memory Runtime Stores

**Files:**

- Modify: `app/web/src/main.tsx`
- Optional future extraction: `app/web/src/chat/chatRuntimeStore.ts`
- Create: `app/__tests__/web-chat-runtime-memory-store.test.ts`

- [ ] **Step 1: Replace legacy refs with raw runtime stores**

Remove remaining use of these legacy refs from `main.tsx`:

```ts
chatSyncIndexRef
chatSyncSubIndexRef
```

Keep equivalent raw runtime state in in-memory stores, with a future extraction boundary if needed:

```ts
chatFinishedTurnStoreRef
chatLiveTurnBufferRef
chatFinishedCursorRef
chatReadInFlightRef
chatRepairPendingRef
```

`main.tsx` may temporarily own the refs, but the model must not include a bounded runtime set.

- [ ] **Step 2: Implement unbounded runtime membership**

Sessions enter the in-memory runtime store when they receive `session.message`, hydrate from cache, read from the server, or become selected. New and resumed sessions create runtime state and become selected.

- [ ] **Step 3: Remove session-count eviction**

Do not evict runtime stores because of a fixed session-count capacity. Stores remain until page lifecycle end or explicit clear/reload.

- [ ] **Step 4: Persist dirty prefixes**

Dirty finished prefixes still persist through the 5 second Durable Cache debounce and required lifecycle flushes.

- [ ] **Step 5: First runtime entry flow**

When a session first enters the in-memory runtime store:

1. hydrate raw durable turns and cursor
2. set full Finished Store in memory
3. clear Live Turn Buffer
4. if selected, rebuild Display Index and mount Virtualized Chat View immediately
5. unconditionally call `session.read(after=Finished Cursor)`

- [ ] **Step 6: Already in-memory selection flow**

Selecting an already in-memory session must not unconditionally read. Rebuild selected Virtualized Chat View from memory.

## Task 7: Realtime and Read Repair Integration

**Files:**

- Create: `app/web/src/chat/chatReadRepair.ts`
- Modify: `app/web/src/main.tsx`
- Modify focused chat sync tests.

- [ ] **Step 1: Message consumption behavior**

For `session.message` in a known project:

- write the raw turn into the in-memory runtime store
- update the Finished Cursor from contiguous finished turns
- schedule durable persist when the finished prefix changes
- trigger read repair on gaps
- refresh project session list if the session is unknown

- [ ] **Step 2: Selected-session behavior**

For selected-session `session.message`:

- merge raw turn
- update cursor
- schedule 5s durable persist for finished prefix
- trigger read repair on gap
- selected session rebuilds Virtualized Chat View

- [ ] **Step 3: Serialized read repair**

Only one read in flight per session. Pending flag is dirty boolean, not a queue.

- [ ] **Step 4: Read application**

Apply read response as authoritative over `[after+1, responseLast]`; split finished/live; reject invalid unfinished placement.

- [ ] **Step 5: Reconnect**

After reconnect, read sessions already present in the in-memory runtime stores plus the selected session after their Finished Cursor. Do not read every session list row.

## Task 8: 5 Second Durable Persist

**Files:**

- Create: `app/web/src/chat/chatDurablePersist.ts`
- Modify: `app/web/src/main.tsx`
- Modify: `app/web/src/services/workspaceStore.ts` if helper API needs raw-turn signature.

- [ ] **Step 1: Dirty session tracking**

Mark a session dirty when its finished prefix changes.

- [ ] **Step 2: Debounce persist**

Schedule IndexedDB persist 5 seconds after dirty mark. Reschedule on further dirty marks.

- [ ] **Step 3: Required flush**

Flush dirty sessions on page hidden/beforeunload, reload/archive/delete, and selected `prompt_done`.

- [ ] **Step 4: Failure handling**

Do not block UI on flush failure.

## Task 9: Session Summary Source of Truth

**Files:**

- Modify: `app/web/src/chatSessionState.ts`
- Modify: `app/__tests__/web-chat-session-state.test.ts`
- Modify: `app/web/src/main.tsx`

- [ ] **Step 1: Remove local list flags**

Remove local running/completed list override behavior.

- [ ] **Step 2: Message does not patch list metadata**

Ensure `session.message` does not update title, preview, running, done, read, or unread state.

- [ ] **Step 3: Merge summary sources**

Allow list state updates from `session.list`, `session.updated`, `session.read.session`, and `session.markRead.session`.

- [ ] **Step 4: markRead only selected prompt_done**

Decode selected raw turn; if method is `prompt_done`, call `session.markRead(turn.turnIndex)`. Non-selected in-memory runtime sessions do not mark read.

- [ ] **Step 5: Run tests**

Run:

```powershell
cd app
npm test -- --runInBand __tests__/web-chat-session-state.test.ts
```

Expected: PASS.

## Task 10: Display Index and Virtualized Turn Rendering

**Files:**

- Delete or retire: `app/web/src/chat/chatTurnWindow.ts` as the primary render window.
- Create: `app/web/src/chat/chatDisplayIndex.ts`
- Create: `app/web/src/chat/ChatVirtuosoTurnList.tsx`
- Modify: `app/package.json`
- Modify: `app/package-lock.json`
- Delete or rewrite: `app/__tests__/web-chat-turn-window.test.ts`
- Create: `app/__tests__/web-chat-display-index.test.ts`
- Create: `app/__tests__/web-chat-virtuoso-scroll.test.ts`
- Modify: `app/web/src/main.tsx`

- [ ] **Step 1: Add Virtuoso dependency**

Add `react-virtuoso` to `app/package.json` dependencies and update `app/package-lock.json`. Do not add a second virtualizer dependency.

Run:

```powershell
cd app
npm install react-virtuoso
```

Expected: package and lockfile updated without changing unrelated dependencies.

- [ ] **Step 2: Add lightweight DisplayItem model**

Create `chatDisplayIndex.ts` with a display-only item shape:

```ts
export type ChatDisplayItemKind =
  | "text"
  | "tool"
  | "thought"
  | "plan"
  | "prompt_request"
  | "prompt_done"
  | "gap"
  | "system";

export type ChatDisplayItem = {
  key: string;
  turnIndex: number;
  kind: ChatDisplayItemKind;
  affordance?: "option_replies" | "confirmation_reply";
  finished: boolean;
  contentRevision: string;
  estimatedHeight: number;
};
```

Build Display Index from source raw turns by shallow parsing only the content envelope needed to classify `method`, `param.type`, status, hidden/tool flags, and gap/copy behavior. Do not store full decoded messages, markdown output, or copy text in Display Index.

- [ ] **Step 3: Implement render filters in Display Index**

Hidden tool/thought turns should not enter Display Index and therefore should not contribute visible scrollbar height. `session/gap` turns must enter Display Index as `kind: "gap"` with compact placeholder height.

Protocol rendering such as prompt request/done separators, plans, tool states, and gaps must be classified in Display Index. Assistant text affordances such as A/B/C choices and Chinese confirmation replies stay on the owning `kind: "text"` item through `affordance`, not as separate virtualized rows.

- [ ] **Step 4: Provide lightweight height estimates**

Do not create a second app-owned height cache. The Display Index provides `estimatedHeight`; `react-virtuoso` owns mounted item measurement internally. Exact measurement is allowed only for mounted visible + overscan items. The implementation must not pre-render, mount, or fully decode all turns to compute exact scrollbar height.

- [ ] **Step 5: Create ChatVirtuosoTurnList wrapper**

Create `ChatVirtuosoTurnList.tsx` and keep Virtuoso API usage inside this wrapper.

Use `Virtuoso`:

```tsx
<Virtuoso
  customScrollParent={scrollParent}
  data={displayIndex.items}
  computeItemKey={(index, item) => item.key}
  defaultItemHeight={defaultItemHeight}
  heightEstimates={heightEstimates}
  initialTopMostItemIndex={{index: 'LAST', align: 'end'}}
  followOutput={isAtBottom => (isAtBottom && shouldAutoscroll() ? 'auto' : false)}
/>
```

The wrapper owns Virtuoso range calculation, mounted item measurement, tail-lock detection, and scroll-to-bottom mechanics. `main.tsx` should not call Virtuoso APIs directly.

- [ ] **Step 6: Render only mounted virtual items**

The virtualizer data source is the full lightweight Display Index. Mounted item renderer decodes full turn content only for visible + overscan items.

- [ ] **Step 7: Implement initial tail position**

When selected session opens, build the full Display Index and initialize the list at the last item with end alignment. Do not create a manual raw turn window.

- [ ] **Step 8: Implement scroll up/down without read**

Scrolling changes virtualizer range only. It does not move a raw turn window and does not trigger server read. DOM remains bounded by visible + overscan.

- [ ] **Step 9: Implement tail lock**

When the virtualized list is within 48px of the bottom and includes the latest Display Item, enter tail lock. New selected-session turns and streaming height growth keep the viewport at bottom only in this mode.

When the user has scrolled up, new turns and streaming chunks update source stores and Display Index but preserve the current viewport anchor and show the scroll-to-bottom affordance.

- [ ] **Step 10: Preserve virtualizer remount state**

Move UI state that must survive virtualizer unmount/remount into session UI state keyed by `sessionId + item.key`, or derive it from raw turns / Session Summary. This includes expanded tool state, confirmation selection, input state, copy state, and choice state.

- [ ] **Step 11: Coalesce streaming height measurement**

For mounted streaming tail items, coalesce re-measurement with `requestAnimationFrame` or equivalent throttling so each chunk does not force a full layout pass.

- [ ] **Step 12: Run tests**

Run:

```powershell
cd app
npm test -- --runInBand __tests__/web-chat-display-index.test.ts __tests__/web-chat-turn-rendering.test.ts
```

Expected: PASS.

## Task 11: Gap Rendering and Copy

**Files:**

- Modify: `app/web/src/chat/chatCopyRange.ts`
- Modify: `app/__tests__/web-chat-copy-range.test.ts`
- Modify: `app/web/src/main.tsx`

- [ ] **Step 1: Decode copy range from full selected source store**

Copy uses full active raw source store, not only current Virtualized Chat View.

- [ ] **Step 2: Reject session/gap**

If decoded range contains `method === "session/gap"`, return copy gap failure.

- [ ] **Step 3: Render session/gap**

Render `session/gap` as a compact unrecoverable system placeholder.

- [ ] **Step 4: Run tests**

Run:

```powershell
cd app
npm test -- --runInBand __tests__/web-chat-copy-range.test.ts __tests__/web-chat-turn-rendering.test.ts
```

Expected: PASS.

## Task 12: Documentation Update

**Files:**

- Modify: `docs/session-management-and-sync.zh-CN.md`
- Modify: `CONTEXT.md` if terminology changes.

- [ ] **Step 1: Update normative protocol**

Document raw turn wire, unbounded in-memory runtime stores, no compatibility migration, 5s persist, and window strategy.

- [ ] **Step 2: Search stale terms**

Run:

```powershell
rg -n "messagesJson|chatSyncIndex|chatSyncSubIndex|latestKnownTurnIndex|local running|completed flags" docs/session-management-and-sync.zh-CN.md CONTEXT.md app/web/src
```

Expected: no stale active-model references after implementation.

## Task 13: Full Verification

**Files:**

- No source changes unless tests reveal a real issue.

- [ ] **Step 1: Server tests**

Run:

```powershell
cd server
go test ./...
```

Expected: PASS.

- [ ] **Step 2: App typecheck**

Run:

```powershell
cd app
npm run tsc:web
```

Expected: PASS.

- [ ] **Step 3: Focused app tests**

Run:

```powershell
cd app
npm test -- --runInBand __tests__/web-chat-turn-stores.test.ts __tests__/web-chat-sync-reconcile.test.ts __tests__/web-chat-session-state.test.ts __tests__/web-chat-display-index.test.ts __tests__/web-chat-turn-rendering.test.ts __tests__/web-chat-copy-range.test.ts
```

Expected: PASS.

- [ ] **Step 4: Full app tests**

Run:

```powershell
cd app
npm test -- --runInBand
```

Expected: PASS, or known pre-existing brittle UI assertion is reported with exact test name and reason.

## Completion Criteria

Implementation is complete only when:

- Server/app use raw turn wire with no `sessionId` inside turns.
- Old wire/cache formats are not treated as compatible.
- Incompatible local persistent cache clears everything except token/auth credentials and recreates IndexedDB tables.
- `react-virtuoso` is added to `app/package.json` and locked in `app/package-lock.json`; no second virtualizer dependency is present.
- `main.tsx` is reduced to orchestration; wire parsing, runtime store, read repair, durable persist, display index, and virtualizer mechanics live in dedicated modules.
- In-memory runtime stores are unbounded; there is no session-count capacity or eviction.
- Every known-project `session.message` writes the raw turn to the runtime store; unknown sessions also trigger project session list refresh.
- First runtime-store entry reads after Finished Cursor; already in-memory selection does not unconditionally read.
- Read repair is serialized and uses only Finished Cursor.
- Durable cache stores raw finished prefix in `turnsJson` and persists with 5s debounce plus required flushes.
- Session list state comes only from Session Summary.
- Selected React view is bounded by `react-virtuoso`, not by a manual raw turn range.
- Display Index is lightweight and does not store full decoded messages or markdown render output.
- Display Index covers the full selected session after hide/show filters.
- Scrollbar is driven by logical Display Index height, not mounted DOM node count.
- Exact item height is measured lazily for mounted visible + overscan items only.
- Scrolling up/down never triggers server read.
- Height correction and streaming growth preserve viewport anchor outside tail lock.
- New turns follow only in tail-lock mode.
- Hidden tool/thought items do not contribute visible scroll height.
- Confirmation/choice affordances and prompt/tool/gap render state survive virtualizer unmount/remount.
- Verification commands pass or remaining failure is explicitly identified as pre-existing.
