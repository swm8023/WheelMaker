# Registry Debug Panel Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a settings-controlled Registry debug capture mode and desktop floating inspection panel.

**Architecture:** Capture stays at `RegistryClient`, the only unified WebSocket boundary. A small debug helper module owns record normalization, session ID extraction, correlation, and filtering. React state owns in-memory records and renders a floating `RegistryDebugPanel` with a virtualized list and pretty JSON detail pane.

**Tech Stack:** TypeScript, React 19, `react-virtuoso`, Jest source/unit tests, existing `workspacePersistence` global settings.

---

### Task 1: Debug Record Helpers

**Files:**
- Create: `app/web/src/debug/registryDebug.ts`
- Test: `app/__tests__/web-registry-debug-records.test.ts`

- [ ] **Step 1: Write failing helper tests**

Create tests that import `createRegistryDebugStore`, `extractRegistryDebugSessionIds`, `filterRegistryDebugRecords`, and `formatRegistryDebugTime`.

Cover:
- extracts session IDs from `payload.sessionId`, `payload.session.sessionId`, `payload.turn.sessionId`, and `payload.sessions[]`
- records multi-session entries
- filters single-session records while hiding multi-session records by default
- includes multi-session records when requested
- correlates response metadata and duration by `requestId`
- clears records and correlation state
- skips `connect.init`

Run: `cd app && npm test -- --runTestsByPath __tests__/web-registry-debug-records.test.ts`
Expected: FAIL because the module does not exist.

- [ ] **Step 2: Implement helper module**

Implement:
- exported record/event types
- `extractRegistryDebugSessionIds(value: unknown): string[]`
- `formatRegistryDebugTime(timestamp: number): string`
- `createRegistryDebugStore()`
- `filterRegistryDebugRecords(records, selectedSessionId, includeMultiSessionRecords)`
- store methods: `setEnabled`, `recordOutbound`, `recordInboundEnvelope`, `recordInboundParseError`, `recordLifecycle`, `clear`, `getRecords`, `subscribe`

Rules:
- When disabled, record methods return without extracting or normalizing.
- `recordOutbound` ignores `connect.init`.
- Store keeps raw JSON string for outbound/inbound messages.
- Response/error records inherit request metadata and compute duration from outbound request.

- [ ] **Step 3: Run helper tests**

Run: `cd app && npm test -- --runTestsByPath __tests__/web-registry-debug-records.test.ts`
Expected: PASS.

### Task 2: RegistryClient Debug Hooks

**Files:**
- Modify: `app/web/src/services/registryClient.ts`
- Test: `app/__tests__/web-registry-client-debug.test.ts`
- Update if useful: `app/__tests__/web-registry-client-close-policy.test.ts`

- [ ] **Step 1: Write failing client source tests**

Create source-level tests that assert:
- `RegistryClient` imports debug event types from `../debug/registryDebug`
- constructor accepts an optional debug sink or observer
- outbound request recording happens before `ws?.send(raw)`
- `connectInit` passes through `request` but `connect.init` is skipped by debug store, not duplicated in business code
- inbound parse failures call the debug sink before returning
- event/response/error/lifecycle paths call the debug sink
- existing close policy remains unchanged

Run: `cd app && npm test -- --runTestsByPath __tests__/web-registry-client-debug.test.ts __tests__/web-registry-client-close-policy.test.ts`
Expected: FAIL on missing debug hooks.

- [ ] **Step 2: Add debug sink support to RegistryClient**

Implement a lightweight constructor option:

```ts
export type RegistryDebugSink = (event: RegistryDebugCaptureEvent) => void;

export class RegistryClient {
  constructor(
    private readonly timeoutMs = 8000,
    private readonly debugSink?: RegistryDebugSink,
  ) {}
}
```

Emit capture events for:
- outbound request with envelope and raw JSON string
- inbound response/error/event with parsed envelope and raw string
- parse error with raw string and message
- lifecycle start/open/close/error

Keep current request behavior unchanged.

- [ ] **Step 3: Run client tests**

Run: `cd app && npm test -- --runTestsByPath __tests__/web-registry-client-debug.test.ts __tests__/web-registry-client-close-policy.test.ts`
Expected: PASS.

### Task 3: Wire Debug Store Through Repository And Workspace Service

**Files:**
- Modify: `app/web/src/services/registryRepository.ts`
- Modify: `app/web/src/services/registryWorkspaceService.ts`
- Test: `app/__tests__/web-registry-debug-service-wiring.test.ts`

- [ ] **Step 1: Write failing service wiring tests**

Assert source contains:
- `createRegistryRepository(debugSink?: RegistryDebugSink)`
- `new RegistryClient(8000, debugSink)` or equivalent preserving timeout
- `RegistryWorkspaceService` constructor accepts a debug sink
- repository creation passes the sink

Run: `cd app && npm test -- --runTestsByPath __tests__/web-registry-debug-service-wiring.test.ts`
Expected: FAIL.

- [ ] **Step 2: Implement service wiring**

Thread the optional debug sink:
- `createRegistryRepository(debugSink?: RegistryDebugSink)`
- `new RegistryClient(undefined, debugSink)` if preserving default timeout through default parameter is cleaner
- `new RegistryWorkspaceService(debugSink?)`
- `connect()` passes the sink into repository creation

- [ ] **Step 3: Run service wiring tests**

Run: `cd app && npm test -- --runTestsByPath __tests__/web-registry-debug-service-wiring.test.ts`
Expected: PASS.

### Task 4: Persist Debug Setting And App State

**Files:**
- Modify: `app/web/src/services/workspacePersistence.ts`
- Modify: `app/web/src/main.tsx`
- Test: `app/__tests__/web-registry-debug-settings.test.ts`

- [ ] **Step 1: Write failing persistence/settings tests**

Assert:
- `PersistedGlobalState` has `registryDebug: boolean`
- `GLOBAL_KEYS.registryDebug` exists
- default is `false`
- sanitizer restores boolean only
- `patchGlobalState` persists it
- `main.tsx` initializes `registryDebug` from `persistedGlobal`
- Settings includes `Debug`, a checkbox bound to `registryDebug`, and an `Open` button

Run: `cd app && npm test -- --runTestsByPath __tests__/web-registry-debug-settings.test.ts`
Expected: FAIL.

- [ ] **Step 2: Implement setting persistence and React state**

Add:
- `registryDebug` to global persistence
- `const [registryDebug, setRegistryDebug] = useState(...)`
- panel open state, include multi-session state, selected debug session state
- effects to enable/disable debug store and clear records when disabled

Patch global persistence alongside existing settings.

- [ ] **Step 3: Run settings tests**

Run: `cd app && npm test -- --runTestsByPath __tests__/web-registry-debug-settings.test.ts`
Expected: PASS.

### Task 5: Floating Debug Panel UI

**Files:**
- Create: `app/web/src/debug/RegistryDebugPanel.tsx`
- Modify: `app/web/src/main.tsx`
- Modify: `app/web/src/styles.css`
- Test: `app/__tests__/web-registry-debug-panel-ui.test.ts`

- [ ] **Step 1: Write failing UI source tests**

Assert:
- `RegistryDebugPanel.tsx` imports `Virtuoso` from `react-virtuoso`
- panel has left list and right detail class names
- panel includes `Include multi-session records`, `Jump to latest`, `Clear`, `Copy`, and close controls
- detail pane uses `JSON.stringify(selectedEnvelopeOrLifecycle, null, 2)`
- `main.tsx` renders `<RegistryDebugPanel` only when `isWide && registryDebug && registryDebugPanelOpen`
- no payload summary class/text appears in the row renderer
- styles define `.registry-debug-panel`, `.registry-debug-list-pane`, and `.registry-debug-detail-pane`

Run: `cd app && npm test -- --runTestsByPath __tests__/web-registry-debug-panel-ui.test.ts`
Expected: FAIL.

- [ ] **Step 2: Implement panel component**

Implement a dense desktop tool UI:
- header drag handle
- Clear and close buttons
- session selector from discovered session IDs
- include multi-session checkbox
- virtualized rows
- follow latest behavior and `Jump to latest`
- right detail pretty JSON
- Copy button
- resize handle

Keep component props explicit and avoid coupling it to workspace app internals.

- [ ] **Step 3: Render panel and connect records**

In `main.tsx`:
- import `RegistryDebugPanel`
- create debug store once with `useMemo` or `useRef`
- pass store sink to `RegistryWorkspaceService`
- subscribe to store updates
- derive discovered session IDs
- render panel only on desktop while Debug is on and panel is open
- add settings row switch and Open button

- [ ] **Step 4: Add styles**

Add CSS for:
- floating fixed panel
- compact header
- two-pane grid
- virtual list rows and selected state
- metadata columns
- JSON detail pane
- resize handle

Use existing codicon styling and restrained app colors.

- [ ] **Step 5: Run UI tests**

Run: `cd app && npm test -- --runTestsByPath __tests__/web-registry-debug-panel-ui.test.ts`
Expected: PASS.

### Task 6: Full Verification And Completion Gate

**Files:**
- All touched files

- [ ] **Step 1: Run targeted tests**

Run:
`cd app && npm test -- --runTestsByPath __tests__/web-registry-debug-records.test.ts __tests__/web-registry-client-debug.test.ts __tests__/web-registry-debug-service-wiring.test.ts __tests__/web-registry-debug-settings.test.ts __tests__/web-registry-debug-panel-ui.test.ts __tests__/web-registry-client-close-policy.test.ts`

Expected: PASS.

- [ ] **Step 2: Run type check**

Run: `cd app && npm run tsc:web`
Expected: PASS.

- [ ] **Step 3: Run web build**

Run: `cd app && npm run build:web`
Expected: PASS.

- [ ] **Step 4: Commit, push, release build**

Follow repository completion gate exactly:

```powershell
git add -A
git commit -m "feat: add registry debug panel"
git push origin main
cd app && npm run build:web:release
```

Expected: commit and push succeed; release build publishes web assets.
