# Multi-Project Chat Index And Turn Window Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make Chat selection, session operations, cache, and rendering project-scoped by the selected chat session while keeping long histories out of React render state.

**Architecture:** Add small pure Chat modules first, then migrate the web shell to use `ChatSessionKey` and a project/session index instead of the workspace-current project. Full session history stays in refs and IndexedDB; React renders only a turn window for the selected key. Registry calls are made with the selected chat key's `projectId`.

**Tech Stack:** React 19, TypeScript, Jest, IndexedDB-backed workspace persistence, existing registry protocol.

---

## File Structure

- Create `app/web/src/chat/chatSessionKey.ts`: composite key type, encode/decode/equality helpers.
- Create `app/web/src/chat/chatTurnWindow.ts`: latest-window slicing, top expansion, continuity checks.
- Create `app/web/src/chat/chatCopyRange.ts`: prompt-done copy range calculation over full turns.
- Create `app/web/src/chat/chatIndexState.ts`: project/session index merge, sort, event classification, refresh coalescing reducer.
- Modify `app/web/src/services/registryWorkspaceService.ts`: add missing project-scoped chat methods.
- Modify `app/web/src/services/workspacePersistence.ts`: persist one global selected chat key.
- Modify `app/web/src/services/workspaceStore.ts`: expose global selected chat key and keep per-project selected id only as migration fallback.
- Modify `app/web/src/main.tsx`: use encoded chat keys for store/cursor/draft/flags; stop switching File/Git project for Chat selection; render turn windows; use project-scoped read/send/config; coalesce refresh.
- Modify `app/web/src/styles.css`: scroll-to-bottom button and automatic nav expansion sentinel styling.
- Add or replace Jest tests under `app/__tests__/` for the pure modules and source-level integration invariants.

---

### Task 1: Composite Chat Session Key

**Files:**
- Create: `app/web/src/chat/chatSessionKey.ts`
- Test: `app/__tests__/web-chat-session-key.test.ts`

- [ ] **Step 1: Write failing tests**

Test these behaviors:
- `encodeChatSessionKey({projectId:'p1', sessionId:'s1'})` is stable and not equal to session id alone.
- `decodeChatSessionKey(encoded)` round-trips.
- empty project or session returns an empty encoded key and decodes to `null`.
- `sameChatSessionKey` compares both fields.
- `chatSessionKeyFromParts` trims boundary inputs.

Run: `cd app && npm test -- --runInBand __tests__/web-chat-session-key.test.ts`
Expected: FAIL because the module does not exist.

- [ ] **Step 2: Implement minimal module**

Use an internal delimiter and encode each key part with `encodeURIComponent`. Export:

```ts
export type ChatSessionKey = { projectId: string; sessionId: string };
export const EMPTY_CHAT_SESSION_KEY = '';
export function chatSessionKeyFromParts(projectId: string, sessionId: string): ChatSessionKey | null;
export function encodeChatSessionKey(key: ChatSessionKey | null | undefined): string;
export function decodeChatSessionKey(encoded: string): ChatSessionKey | null;
export function sameChatSessionKey(left: ChatSessionKey | null | undefined, right: ChatSessionKey | null | undefined): boolean;
```

- [ ] **Step 3: Verify and commit**

Run: `cd app && npm test -- --runInBand __tests__/web-chat-session-key.test.ts`
Expected: PASS.

Commit:

```bash
git add app/__tests__/web-chat-session-key.test.ts app/web/src/chat/chatSessionKey.ts
git commit -m "feat: add chat session key helpers"
```

### Task 2: Turn Window And Copy Range Pure Modules

**Files:**
- Create: `app/web/src/chat/chatTurnWindow.ts`
- Create: `app/web/src/chat/chatCopyRange.ts`
- Test: `app/__tests__/web-chat-turn-window.test.ts`
- Test: `app/__tests__/web-chat-copy-range.test.ts`

- [ ] **Step 1: Write failing turn window tests**

Test latest 200 turns, expansion by 200 earlier turns, visible slicing, sparse hidden rendering leaving boundaries turn-based, and gap detection on non-contiguous `turnIndex` values.

Run: `cd app && npm test -- --runInBand __tests__/web-chat-turn-window.test.ts`
Expected: FAIL because the module does not exist.

- [ ] **Step 2: Implement `chatTurnWindow.ts`**

Export constants `DEFAULT_TURN_WINDOW_SIZE = 200` and `TURN_WINDOW_EXPAND_SIZE = 200`, plus:

```ts
export type ChatTurnWindow = { startTurnIndex: number; endTurnIndex: number };
export function createLatestTurnWindow(turns: RegistryChatMessage[], size?: number): ChatTurnWindow;
export function expandTurnWindowEarlier(turns: RegistryChatMessage[], window: ChatTurnWindow, size?: number): ChatTurnWindow;
export function sliceTurnsForWindow(turns: RegistryChatMessage[], window: ChatTurnWindow): RegistryChatMessage[];
export function hasContinuousTurnRange(turns: RegistryChatMessage[], startTurnIndex: number, endTurnIndex: number): boolean;
```

- [ ] **Step 3: Write failing copy range tests**

Test copy from `prompt_done` walks backward to the preceding `prompt_request`, uses full store, includes only agent message turns, excludes tool/thought/plan/user, disables on missing request, and disables on gaps.

Run: `cd app && npm test -- --runInBand __tests__/web-chat-copy-range.test.ts`
Expected: FAIL because the module does not exist.

- [ ] **Step 4: Implement `chatCopyRange.ts`**

Export:

```ts
export type ChatCopyRangeResult =
  | { ok: true; markdown: string; startTurnIndex: number; endTurnIndex: number }
  | { ok: false; reason: 'missing_done' | 'missing_request' | 'gap' | 'empty_agent_response' };
export function buildPromptDoneCopyRange(turns: RegistryChatMessage[], doneTurnIndex: number): ChatCopyRangeResult;
```

Use `buildPromptAgentMarkdown` for markdown assembly and only pass `{kind:'message', text}` entries.

- [ ] **Step 5: Verify and commit**

Run: `cd app && npm test -- --runInBand __tests__/web-chat-turn-window.test.ts __tests__/web-chat-copy-range.test.ts`
Expected: PASS.

Commit:

```bash
git add app/__tests__/web-chat-turn-window.test.ts app/__tests__/web-chat-copy-range.test.ts app/web/src/chat/chatTurnWindow.ts app/web/src/chat/chatCopyRange.ts
git commit -m "feat: add chat turn window helpers"
```

### Task 3: Chat Index State

**Files:**
- Create: `app/web/src/chat/chatIndexState.ts`
- Test: `app/__tests__/web-chat-index-state.test.ts`

- [ ] **Step 1: Write failing tests**

Test:
- project order is pinned first, then newest session activity, then project name.
- `session.updated` patches or inserts a summary for a known project.
- unknown project event returns `refreshAll`.
- known project unknown-session `session.message` returns `refreshProject`.
- refresh scheduling coalesces full and project refreshes with dirty follow-up flags.

Run: `cd app && npm test -- --runInBand __tests__/web-chat-index-state.test.ts`
Expected: FAIL because the module does not exist.

- [ ] **Step 2: Implement reducer and helpers**

Export:

```ts
export type ChatIndexRefreshState = {
  fullRefreshInFlight: boolean;
  fullRefreshDirty: boolean;
  projectRefreshInFlight: Record<string, boolean>;
  projectRefreshDirty: Record<string, boolean>;
  projectErrors: Record<string, string>;
};
export type ChatIndexState = {
  projects: RegistryProject[];
  sessionsByProjectId: Record<string, RegistryChatSession[]>;
  selected: ChatSessionKey | null;
  refresh: ChatIndexRefreshState;
};
export function createChatIndexState(): ChatIndexState;
export function sortChatIndexProjects(projects: RegistryProject[], sessionsByProjectId: Record<string, RegistryChatSession[]>, pinnedProjectIds: string[]): RegistryProject[];
export function mergeChatIndexSession(state: ChatIndexState, projectId: string, session: Partial<RegistryChatSession> & {sessionId: string}): ChatIndexState;
export function classifyChatSessionEvent(state: ChatIndexState, event: Pick<RegistryEnvelope, 'method' | 'projectId' | 'payload'>): { kind: 'patch' | 'refreshProject' | 'refreshAll' | 'ignore'; projectId?: string; session?: RegistryChatSession };
export function requestChatIndexFullRefresh(state: ChatIndexState): ChatIndexState;
export function finishChatIndexFullRefresh(state: ChatIndexState, error?: string): ChatIndexState;
export function requestChatIndexProjectRefresh(state: ChatIndexState, projectId: string): ChatIndexState;
export function finishChatIndexProjectRefresh(state: ChatIndexState, projectId: string, error?: string): ChatIndexState;
```

- [ ] **Step 3: Verify and commit**

Run: `cd app && npm test -- --runInBand __tests__/web-chat-index-state.test.ts`
Expected: PASS.

Commit:

```bash
git add app/__tests__/web-chat-index-state.test.ts app/web/src/chat/chatIndexState.ts
git commit -m "feat: add chat index state helpers"
```

### Task 4: Project-Scoped Service And Global Selected Chat Persistence

**Files:**
- Modify: `app/web/src/services/registryWorkspaceService.ts`
- Modify: `app/web/src/services/workspacePersistence.ts`
- Modify: `app/web/src/services/workspaceStore.ts`
- Test: `app/__tests__/web-chat-project-service.test.ts`
- Test: `app/__tests__/web-chat-selection-persistence.test.ts`

- [ ] **Step 1: Write failing service tests**

Test that `readProjectSession`, `sendProjectSessionMessage`, and `setProjectSessionConfig` delegate to repository methods with the passed `projectId`, not `selectedProjectId`.

Run: `cd app && npm test -- --runInBand __tests__/web-chat-project-service.test.ts`
Expected: FAIL because the project-scoped methods are missing.

- [ ] **Step 2: Add missing service methods**

Add:

```ts
async readProjectSession(projectId: string, sessionId: string, afterTurnIndex = 0): Promise<RegistrySessionReadResponse>;
async sendProjectSessionMessage(projectId: string, payload: {sessionId: string; text?: string; blocks?: unknown[]}): Promise<{ok: boolean; sessionId: string}>;
async setProjectSessionConfig(projectId: string, payload: {sessionId: string; configId: string; value: string}): Promise<{ok: boolean; sessionId: string; configOptions: RegistrySessionConfigOption[]}>;
```

- [ ] **Step 3: Write failing persistence tests**

Test that the store can remember and restore one global `ChatSessionKey`, and that migration falls back to a project state's `selectedChatSessionId` when no global key exists.

Run: `cd app && npm test -- --runInBand __tests__/web-chat-selection-persistence.test.ts`
Expected: FAIL because persistence has no global selected chat key.

- [ ] **Step 4: Add persistence/store APIs**

Add `selectedChatProjectId` and `selectedChatSessionId` to global persisted state. Add `WorkspaceStore.getSelectedChatSessionKey()`, `rememberSelectedChatSessionKey(key)`, and `migrateSelectedChatSessionKey(projectId)`.

- [ ] **Step 5: Verify and commit**

Run: `cd app && npm test -- --runInBand __tests__/web-chat-project-service.test.ts __tests__/web-chat-selection-persistence.test.ts`
Expected: PASS.

Commit:

```bash
git add app/__tests__/web-chat-project-service.test.ts app/__tests__/web-chat-selection-persistence.test.ts app/web/src/services/registryWorkspaceService.ts app/web/src/services/workspacePersistence.ts app/web/src/services/workspaceStore.ts
git commit -m "feat: persist global chat selection"
```

### Task 5: Migrate Main Chat State To Composite Keys

**Files:**
- Modify: `app/web/src/main.tsx`
- Test: `app/__tests__/web-chat-main-composite-key.test.ts`

- [ ] **Step 1: Write failing integration invariant tests**

Test source invariants that:
- selected Chat state is `ChatSessionKey | null`.
- `selectProjectChatSession` does not call `switchProject`.
- `loadChatSession` takes a `ChatSessionKey` and calls `service.readProjectSession(key.projectId, key.sessionId, ...)`.
- `sendChatMessage` calls `service.sendProjectSessionMessage(selectedChatKey.projectId, ...)`.
- runtime maps use `encodeChatSessionKey`.

Run: `cd app && npm test -- --runInBand __tests__/web-chat-main-composite-key.test.ts`
Expected: FAIL against current `selectedChatId`/`switchProject` path.

- [ ] **Step 2: Migrate refs/state**

Replace session-only chat state with:

```ts
const selectedChatKeyRef = useRef<ChatSessionKey | null>(null);
const [selectedChatKey, setSelectedChatKey] = useState<ChatSessionKey | null>(null);
const selectedChatEncodedKey = useMemo(() => encodeChatSessionKey(selectedChatKey), [selectedChatKey]);
```

Keep `selectedChatId` only as a derived local string if needed by narrow existing JSX.

- [ ] **Step 3: Update runtime maps**

Use encoded chat keys for `chatSyncIndexRef`, `chatSyncSubIndexRef`, `chatMessageStoreRef`, running/completed flags, config update key, draft key, attachment read counts, and draft generation.

- [ ] **Step 4: Update selection and load**

`selectProjectChatSession(projectId, sessionId)` immediately sets the selected key, clears `chatMessages`, closes drawer when requested, sets `tab` to chat, persists global key, hydrates cached content by `{projectId, sessionId}`, schedules turn-window rendering on the next animation frame, and calls `loadChatSession(key, ...)`. It must not call `switchProject`.

- [ ] **Step 5: Update send/config/delete/reload/resume**

Use selected key's project for send and config. Delete/reload/creation/resume update map entries and cache by encoded key. When deleting the selected key, clear only if the encoded deleted key matches.

- [ ] **Step 6: Verify and commit**

Run:

```bash
cd app
npm test -- --runInBand __tests__/web-chat-main-composite-key.test.ts
npm run tsc:web
```

Expected: PASS.

Commit:

```bash
git add app/__tests__/web-chat-main-composite-key.test.ts app/web/src/main.tsx
git commit -m "feat: scope chat selection by project"
```

### Task 6: Chat Index Refresh And Event Handling

**Files:**
- Modify: `app/web/src/main.tsx`
- Test: `app/__tests__/web-chat-refresh-model.test.ts`

- [ ] **Step 1: Write failing integration invariant tests**

Test that:
- mobile drawer open no longer calls `refreshMobileChatProjectSessions`.
- manual refresh calls a full chat index refresh.
- reconnect performs one full chat index refresh.
- `session.updated` with unknown project schedules full refresh.
- `session.message` with known project unknown session schedules project refresh and ignores payload insertion.

Run: `cd app && npm test -- --runInBand __tests__/web-chat-refresh-model.test.ts`
Expected: FAIL against current drawer fan-out and current-project event handling.

- [ ] **Step 2: Add refresh scheduler refs**

Add full/project in-flight and dirty refs. Implement `refreshChatIndex`, `refreshChatProjectSessions`, `scheduleChatIndexRefresh`, and `scheduleChatProjectRefresh` around `service.listProjects()` and `service.listProjectSessions(projectId)`.

- [ ] **Step 3: Replace mobile refresh path**

Keep `refreshMobileChatProjectSessions` as a manual refresh wrapper or rename it to `refreshChatIndex`. Remove the drawer-open effect. Hydrate `projectSessionsByProjectId` from local cache when projects are available.

- [ ] **Step 4: Update event handling**

Use envelope `event.projectId` only for Chat events. Missing project id is ignored for normal patch logic. Unknown project schedules full refresh. Known project unknown-session `session.message` schedules project refresh. Known selected key messages merge into the encoded full store and update the visible window.

- [ ] **Step 5: Verify and commit**

Run:

```bash
cd app
npm test -- --runInBand __tests__/web-chat-refresh-model.test.ts
npm run tsc:web
```

Expected: PASS.

Commit:

```bash
git add app/__tests__/web-chat-refresh-model.test.ts app/web/src/main.tsx
git commit -m "feat: add chat index refresh scheduling"
```

### Task 7: Visible Turn Window And Turn Renderer

**Files:**
- Modify: `app/web/src/main.tsx`
- Modify: `app/web/src/styles.css`
- Test: `app/__tests__/web-chat-turn-rendering.test.ts`

- [ ] **Step 1: Write failing rendering invariant tests**

Test source invariants that:
- `groupChatMessagesByPrompt` and `ChatPromptGroupView` are not used by render.
- visible messages are computed through `sliceTurnsForWindow`.
- `prompt_done` copy uses `buildPromptDoneCopyRange` and the full store.
- disabled copy does not backfill.
- scroll-to-bottom button exists and is hidden near bottom.

Run: `cd app && npm test -- --runInBand __tests__/web-chat-turn-rendering.test.ts`
Expected: FAIL against current prompt-group render path.

- [ ] **Step 2: Add visible window state**

Use `visibleTurnWindowByKeyRef` and set `chatMessages` to only `sliceTurnsForWindow(fullStore, window)` for the selected key. On session switch, set empty messages immediately, then use `requestAnimationFrame` to show the latest window. On top scroll proximity, call `expandTurnWindowEarlier` and preserve anchor.

- [ ] **Step 3: Replace prompt group renderer with turn renderer**

Render each visible turn by `method`. Keep current user, assistant Markdown, thought, tool, plan, and prompt separator visuals. `prompt_done` renders divider, model, duration, copy button, and a lightweight failure line when stop reason is `failed` or `lastDoneSuccess` indicates failure.

- [ ] **Step 4: Add bottom follow button**

Track `chatShowScrollToBottom`. Do not auto-jump on new turns unless near bottom. Place a codicon arrow button above the composer that calls forced scroll and hides near bottom.

- [ ] **Step 5: Verify and commit**

Run:

```bash
cd app
npm test -- --runInBand __tests__/web-chat-turn-rendering.test.ts __tests__/web-chat-turn-window.test.ts __tests__/web-chat-copy-range.test.ts
npm run tsc:web
```

Expected: PASS.

Commit:

```bash
git add app/__tests__/web-chat-turn-rendering.test.ts app/web/src/main.tsx app/web/src/styles.css
git commit -m "feat: render chat turn windows"
```

### Task 8: Automatic Session List Expansion And Cleanup

**Files:**
- Modify: `app/web/src/main.tsx`
- Modify: `app/web/src/styles.css`
- Modify: brittle source tests that no longer match the design.

- [ ] **Step 1: Write failing nav tests**

Test that Show More text is gone, a per-project sentinel exists, and the selected row condition compares encoded selected key or both project/session ids.

Run: `cd app && npm test -- --runInBand __tests__/web-chat-ui.test.ts __tests__/web-responsive-shell.test.ts`
Expected: current brittle tests fail or new tests fail until updated.

- [ ] **Step 2: Replace Show More with automatic expansion**

Add a small sentinel at the end of each rendered project session list. Use `IntersectionObserver` where available and a scroll fallback to increment the project's visible count. Do not call network refresh from this expansion.

- [ ] **Step 3: Remove obsolete current-project chat paths**

Remove current-project-only `chatSessions` business paths or make them derived from `projectSessionsByProjectId[selectedChatKey.projectId]`. Remove any selected-session comparison that requires `targetProjectId === projectId`.

- [ ] **Step 4: Verify and commit**

Run:

```bash
cd app
npm test -- --runInBand
npm run tsc:web
```

Expected: PASS except no known baseline failures remain.

Commit:

```bash
git add app/__tests__ app/web/src/main.tsx app/web/src/styles.css
git commit -m "feat: finish multi-project chat navigation"
```

### Task 9: Final Verification

**Files:**
- Any files changed by previous tasks.

- [ ] **Step 1: Full app verification**

Run:

```bash
cd app
npm test -- --runInBand
npm run tsc:web
```

Expected: PASS.

- [ ] **Step 2: Release gate**

From repo root run:

```bash
git add -A
git commit -m "feat: optimize multi-project chat switching"
git push origin feature/multi-project-chat-index
cd app && npm run build:web:release
```

Expected: commit and push succeed; release build succeeds.

