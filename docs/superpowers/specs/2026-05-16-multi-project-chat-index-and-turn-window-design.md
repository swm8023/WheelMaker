# Multi-Project Chat Index and Turn Window Design

Date: 2026-05-16
Status: Draft

## Goal

Refactor the web Chat experience so Chat is a true multi-project session surface. Selecting, reading, sending, rendering, and updating a chat session must be scoped by the selected session's project, not by the File/Git workspace project.

The change also addresses visible delay when switching sessions by separating full session history from the smaller turn window React renders.

## Problems

The current implementation still carries a current-project chat model:

- The UI globally selects only one chat session, but stores only `sessionId`, so project context is recovered indirectly through `switchProject`.
- Some chat service methods use `selectedProjectId`, even though repository/protocol methods already accept `projectId`.
- Opening mobile Chat can fan out over all projects, which makes opening the drawer equivalent to an online refresh.
- Full message history can enter React state and render as Markdown in one pass, which makes long session switching visibly slow.
- The UI still has prompt-group-shaped rendering even though the protocol truth is session turns.

## Non-Goals

- Do not add a registry-level all-session aggregation endpoint.
- Do not add server-side recent/range session read in this phase.
- Do not rename the existing File/Git `projectId` state across the whole app.
- Do not add Chat search in this phase.
- Do not retain parallel current-project chat and multi-project chat paths.

## Chat Session Key

Chat state uses an explicit project-scoped key:

```ts
type ChatSessionKey = {
  projectId: string;
  sessionId: string;
};
```

Rules:

- `sessionId` alone is not a Chat business key.
- UI and business functions should pass `ChatSessionKey` objects where possible.
- Maps and refs encode the key through a small helper, for example `encodeChatSessionKey(key)`.
- Message store, cursor state, draft state, running/completed flags, and visible turn windows all use the encoded key.
- Chat has one global selected session: `selectedChatKey: ChatSessionKey | null`.

This keeps `projectId` available for every protocol call without reverse lookup through the session index.

## Selected Session Persistence

Persist the Chat selection as one global `ChatSessionKey`, not as one selected session per project.

Rules:

- Restore `selectedChatKey` on startup/reconnect before online refresh completes.
- If the selected project exists but its session list is not refreshed yet, keep the selected key and allow `readProjectSession` to try loading it.
- If the selected project disappears after refresh, clear `selectedChatKey`.
- If the selected session disappears from a refreshed project list, select the most recently updated session or enter an empty Chat state.
- During migration, if no global key exists, seed it from the current workspace project's stored selected chat session when available.

## Chat Index

Introduce one ChatIndex state model for the project/session list:

```ts
type ChatIndexState = {
  projects: RegistryProject[];
  sessionsByProjectId: Record<string, RegistryChatSession[]>;
  selected: ChatSessionKey | null;
  refresh: {
    fullRefreshInFlight: boolean;
    projectRefreshInFlight: Record<string, boolean>;
    projectErrors: Record<string, string>;
  };
};
```

ChatIndex stores list-level data only. It does not own full conversation turns, file state, or Git state.

The old `chatSessions` current-project list path should be removed from Chat business logic. Desktop and mobile session navigation both read from `ChatIndex.sessionsByProjectId`.

Project grouping remains in the UI because project identity is still the routing context. The UI should not emphasize a workspace-current project. Project order is pinned first, then recently active projects, then name. Sessions sort by `updatedAt desc`.

Session navigation should not use a manual Show More button. Each project initially renders a bounded number of recent sessions. When the user scrolls near the end of that project's visible session list, the visible count expands locally. This expansion does not trigger network requests; it only reveals more sessions already present in ChatIndex.

## Refresh Model

ChatIndex is an eventually consistent cache.

Refresh triggers:

- Reconnect success performs one full index refresh.
- User Refresh performs one full index refresh.
- Opening Chat or the drawer only restores local cache; it does not automatically fan out online.
- A session event for an unknown project ignores the payload and triggers index refresh.
- A `session.message` for a known project but unknown session ignores list patching and triggers that project's `session.list`.
- A `session.updated` for a known project can insert or patch the session summary.

Refresh scheduling:

- Only one full refresh may run at a time.
- Only one `session.list(projectId)` may run per project at a time.
- Repeated refresh requests are coalesced.
- If a refresh is in flight and another signal arrives, mark the target dirty and run at most one follow-up refresh after the current one finishes.
- Unknown-session message refreshes are debounced.
- Manual Refresh can request an immediate refresh, but it still does not create duplicate in-flight calls.

Failure behavior:

- `project.list` failure is a global refresh failure; keep the old index.
- A single `session.list(projectId)` failure records a project-level error and keeps that project's old sessions.
- Refresh never clears the visible list and never blocks selecting or sending in a session.

## Event Handling

`session.updated` and `session.message` must use envelope-level `projectId`.

Rules:

- Missing `projectId` is a protocol anomaly and does not enter normal patch logic.
- Do not fall back to the File/Git workspace `projectId`.
- Known project and known session: patch list/message state by `{projectId, sessionId}`.
- Known project and unknown session:
  - `session.updated`: insert/patch the summary.
  - `session.message`: do not create a low-quality list row; trigger `session.list(projectId)`.
- Unknown project: ignore the event payload and trigger an index refresh.

## Project-Scoped Session Operations

The repository already exposes project-scoped methods. The workspace service and Chat hooks should expose project-scoped variants for all Chat operations:

- `listProjectSessions(projectId)`
- `readProjectSession(projectId, sessionId, afterTurnIndex)`
- `sendProjectSessionMessage(projectId, payload)`
- `markProjectSessionRead(projectId, sessionId, lastReadTurnIndex)`
- `setProjectSessionConfig(projectId, payload)`
- `createProjectSession(projectId, agentType, title?)`
- `deleteProjectSession(projectId, sessionId)`
- `reloadProjectSession(projectId, sessionId)`

Selecting a Chat session must not call `switchProject`. File/Git can continue to use the existing workspace project state.

## Conversation State

Separate complete history from rendered history:

- `fullMessageStoreRef[chatKey]`: complete known turns for a session.
- `visibleTurnWindowByKey[chatKey]`: rendered turn range.
- React state for the conversation contains only the selected session's visible turns.
- IndexedDB/cache continues to store complete messages in this phase.

Opening a session:

1. Immediately set selected `{projectId, sessionId}`.
2. Close the drawer and remove the previous session DOM.
3. Show a lightweight skeleton or cached first window for the target session.
4. On the next animation frame, load the recent turn window, defaulting to the latest 200 turns.
5. Start `readProjectSession(projectId, sessionId, afterTurnIndex)` in the background to reconcile new turns.

Cold cache behavior:

- If there is no local history, first phase may still call `session.read(0)` and receive the full history.
- The full result is saved to the full store/cache, but React renders only the recent turn window.

Cache writes:

- Complete messages remain durable.
- Streaming writes should be debounced to avoid serializing full history for every turn update.
- `prompt_done`, session switch, and page unload should flush pending writes.

## Turn Window

The loading and rendering unit is raw `turnIndex`.

Defaults:

- Initial rendered window: latest 200 turns.
- Top expansion: extend by 200 earlier turns.

Rules:

- `hideToolCalls` affects rendering only, not window boundaries.
- If hidden turns make the visible content sparse, the window may expand further, but the boundary is still turn-based.
- Scrolling near the top automatically expands the local window if earlier turns exist in the full store.
- Do not show a manual Load Earlier button.
- Preserve the scroll anchor when earlier turns are inserted.
- Expanding the window reads only from local full store; it does not trigger network backfill.

New turn scrolling:

- If the user is near the bottom, follow new messages to the bottom.
- If the user is reading history, do not jump.
- When not near the bottom, show a bottom-right scroll-to-bottom icon button above the composer.
- Hide the button when the user is near the bottom.
- Switching sessions resets the target session view to the bottom.

## Turn Renderer

Conversation rendering accepts `visibleTurns: RegistryChatMessage[]` and renders each turn according to `method`.

Expected render behavior:

- `prompt_request`: user message turn.
- `agent_message_chunk`: assistant message turn.
- `agent_thought_chunk`: collapsible thought turn.
- Tool methods: tool turn, subject to `hideToolCalls`.
- Plan methods: plan turn.
- `system`: system turn.
- `prompt_done`: separator turn using the current separator appearance: divider, model name, duration, and copy button.

If a `prompt_done` indicates failure, render an additional lightweight failure status message.

The UI no longer builds a prompt-group data model. Any separator metadata is derived while rendering the turn stream.

## Copy Behavior

The copy button on a `prompt_done` separator copies the complete agent response for that completed prompt range.

Rules:

- Find the matching range by walking backward from the `prompt_done` to the preceding `prompt_request`.
- The range must be present and continuous in the full message store.
- Copy only agent message turns.
- Never include tool or thought turns.
- Copy is based on the full store, not the visible turn window.
- If the full store lacks the request or contains a gap, disable the copy button.
- Disabled copy does not trigger automatic network backfill.

## Drafts and Attachments

Composer drafts belong to the selected Chat session key.

Rules:

- Draft text and attachments are stored by encoded `{projectId, sessionId}`.
- Switching sessions preserves the old session's draft and restores the target session's draft.
- Sending clears only the selected session draft.
- Async attachment reads are scoped to the draft key that started the read.

## Component and Module Split

Add focused Chat modules under `app/web/src/chat/`:

```text
app/web/src/chat/
  chatSessionKey.ts
  chatIndexState.ts
  chatTurnWindow.ts
  chatCopyRange.ts
  useChatIndex.ts
  useChatSession.ts
  ChatProjectSessionNav.tsx
  ChatConversationView.tsx
  ChatComposer.tsx
```

Responsibilities:

- `chatSessionKey.ts`: key type, encode/decode, equality helpers.
- `chatIndexState.ts`: index reducer, session merge, event patch results, refresh status.
- `chatTurnWindow.ts`: recent window, expansion, visible slicing, continuity helpers.
- `chatCopyRange.ts`: copy range selection and disabled reason.
- `useChatIndex.ts`: cache restore, full/project refresh scheduling, event-driven index updates.
- `useChatSession.ts`: selected key, read/send/markRead/config, full store, visible window, scroll/follow state.
- `ChatProjectSessionNav.tsx`: project/session navigation for desktop and mobile variants.
- `ChatConversationView.tsx`: turn renderer, separator, top expansion trigger, scroll-to-bottom button.
- `ChatComposer.tsx`: draft, attachments, and send trigger.

Desktop and mobile shells should reuse the same data hooks. They may choose different layout variants, but they must not duplicate Chat business logic.

## Tests

Move coverage toward pure-function and behavior tests.

Required tests:

- `chatSessionKey`: composite key encode/decode/equality.
- `chatIndexState`: `session.updated` patch/insert, unknown project signal, unknown-session message refresh signal, refresh in-flight coalescing.
- `chatTurnWindow`: latest 200 turns, top expansion, visible slicing, continuity/gap detection.
- `chatCopyRange`: full copy range, missing request disables, gap disables, tool/thought excluded.
- `chatSync`: reconcile by composite chat key where relevant.
- UI behavior:
  - Chat selection does not call `switchProject`.
  - Read/send use project-scoped service calls.
  - Conversation renders turns directly and no longer depends on prompt-group state.
  - Scroll-to-bottom button appears only when not near the bottom.

Automatic top expansion should have either a small DOM test around scroll anchoring or a manual verification checklist if DOM testing becomes too brittle.

## Rollout

Implement in slices:

1. Add pure key/index/window/copy modules and tests.
2. Add project-scoped workspace service methods.
3. Introduce `useChatIndex` and migrate navigation to ChatIndex.
4. Introduce `useChatSession` with full store and visible turn window.
5. Replace prompt-group conversation rendering with turn rendering.
6. Remove old current-project chat state paths.
7. Update mobile/desktop navigation to automatic visible-count expansion and non-blocking refresh indicators.

Each slice should keep the app runnable and should not introduce a second long-lived Chat data path.

## Open Risks

- Cold-cache long sessions still require full `session.read(0)` in this phase.
- Full-history IndexedDB serialization may remain expensive until chunked cache storage is introduced.
- Scroll anchor preservation for automatic top expansion needs careful browser testing.
- Removing prompt-group rendering may affect copy/separator edge cases around incomplete local history.
