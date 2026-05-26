# Session Search Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build project-fanned session search across normal chat sessions, covering title and visible prompt content with progressive result display.

**Architecture:** The registry keeps routing `session.*` requests by `projectId`. Each hub `Client` owns async `session.search` tasks keyed by a client-generated `searchId`; the Web UI fans out one search id to all known projects, polls full per-project results, and renders a separate search list without disturbing normal session list updates.

**Tech Stack:** Go server (`server/internal/hub/client`, `server/internal/registry`), React/TypeScript app (`app/web/src`), Jest frontend tests, Go tests.

---

## File Structure

- Modify `server/internal/registry/server.go`: allow forwarding `session.search`.
- Create `server/internal/hub/client/session_search.go`: request/response types, search manager, task lifecycle, title matching, prompt matching, TTL cleanup.
- Modify `server/internal/hub/client/session_turn_files.go`: add file-once newest-to-oldest WMT2 scanning helpers.
- Modify `server/internal/hub/client/client.go`: initialize/close search manager and route `session.search`.
- Modify `server/internal/hub/client/client_test.go`: add protocol and task behavior tests.
- Modify `server/internal/hub/client/session_turn_files_test.go`: add file-once reverse scan tests.
- Modify `app/web/src/types/registry.ts`: add search request/result/response types.
- Modify `app/web/src/services/registryRepository.ts`: add `startSessionSearch`, `querySessionSearch`, and `cancelSessionSearch`.
- Create `app/web/src/chat/sessionSearchState.ts`: pure helpers for result normalization, merging, polling delay, title highlighting, and display rows.
- Modify `app/web/src/chat/chatDisplayIndex.ts`: add helper to resolve nearest visible item for a target `turnIndex`.
- Modify `app/web/src/chat/ChatVirtuosoTurnList.tsx`: expose `scrollToTurnIndex`.
- Modify `app/web/src/main.tsx`: add in-memory session search state, fan-out start/query/cancel, session-list search controls/results, and prompt-result turn jump/highlight.
- Modify `app/web/src/styles.css`: add compact session-search control, result marker, highlight, and empty/error states.
- Add or extend Jest tests under `app/__tests__/`: repository normalization, search state helpers, display index jump helper, Virtuoso imperative scroll, and source-level UI wiring.

---

### Task 1: Server RED Tests

- [ ] **Step 1: Add failing protocol tests**

Add tests in `server/internal/hub/client/client_test.go` that call:

```go
resp, err := c.HandleSessionRequest(ctx, "session.search", "proj1", json.RawMessage(`{"action":"start","searchId":"search-1","query":"deploy"}`))
```

Expected assertions:

```go
body := resp.(map[string]any)
if body["searchId"] != "search-1" || body["done"] != false {
	t.Fatalf("start response = %#v", body)
}
```

- [ ] **Step 2: Add failing result behavior tests**

Cover:

```go
// title hit returns source=title and no turnIndex
// prompt hit returns source=prompt and the newest matching turnIndex
// title hit skips prompt scanning
// cancel is idempotent
// invalid/expired query returns a clear error
```

- [ ] **Step 3: Add failing WMT2 scan tests**

In `server/internal/hub/client/session_turn_files_test.go`, write turns across two files and assert the new reverse scanner returns the newest matching `turnIndex` without using `ReadTurns`.

- [ ] **Step 4: Verify RED**

Run:

```powershell
go test ./internal/hub/client -run "SessionSearch|FileSessionTurnStore.*Search" -count=1
```

Expected: FAIL because `session.search` and scan helpers do not exist yet.

---

### Task 2: Server GREEN Implementation

- [ ] **Step 1: Add search manager**

Create `session_search.go` with:

```go
type sessionSearchRequest struct {
	Action   string `json:"action"`
	SearchID string `json:"searchId"`
	Query    string `json:"query,omitempty"`
}

type sessionSearchResult struct {
	ProjectID string `json:"projectId"`
	SessionID string `json:"sessionId"`
	Source    string `json:"source"`
	TurnIndex int64  `json:"turnIndex,omitempty"`
}
```

- [ ] **Step 2: Implement task lifecycle**

Implement `start`, `query`, `cancel`, 10-minute idle cleanup, and max 8 tasks per project.

- [ ] **Step 3: Implement prompt text matching**

Parse each turn as `acp.IMTurnMessage`, extract visible text for `prompt_request`, `user_message_chunk`, `agent_message_chunk`, `agent_thought_chunk`, `agent_plan`, `tool_call`, `tool_call_update`, `tool_result`, and `prompt_done`, and avoid matching method names or JSON keys.

- [ ] **Step 4: Wire into client and registry**

Add `session.search` to `HandleSessionRequest` and to the registry forward list.

- [ ] **Step 5: Verify GREEN**

Run:

```powershell
go test ./internal/hub/client -run "SessionSearch|FileSessionTurnStore.*Search" -count=1
go test ./internal/registry -run Forward -count=1
```

Expected: PASS.

---

### Task 3: Frontend RED Tests

- [ ] **Step 1: Add repository tests**

Add a Jest test that stubs `client.request` and verifies:

```ts
await repository.startSessionSearch('proj1', 'search-1', 'Deploy')
await repository.querySessionSearch('proj1', 'search-1')
await repository.cancelSessionSearch('proj1', 'search-1')
```

Expected methods and payloads use `session.search` with actions `start`, `query`, and `cancel`.

- [ ] **Step 2: Add pure state tests**

Create tests for `sessionSearchState.ts`:

```ts
expect(resolveSessionSearchPollDelay({changed: true, unchangedPolls: 0})).toBe(300);
expect(resolveSessionSearchPollDelay({changed: false, unchangedPolls: 3})).toBe(800);
```

Also test merging full per-project results and title highlight segmentation.

- [ ] **Step 3: Add turn jump tests**

Extend `web-chat-display-index.test.ts` and `web-chat-virtuoso-mount.test.tsx` to verify exact and nearest-visible turn resolution plus imperative `scrollToTurnIndex`.

- [ ] **Step 4: Verify RED**

Run:

```powershell
npm test -- --runTestsByPath __tests__/web-session-search-service.test.ts __tests__/web-session-search-state.test.ts __tests__/web-chat-display-index.test.ts __tests__/web-chat-virtuoso-mount.test.tsx
```

Expected: FAIL because the new methods/helpers do not exist yet.

---

### Task 4: Frontend GREEN Implementation

- [ ] **Step 1: Add registry types and repository methods**

Add typed search responses in `types/registry.ts`, normalize results in `registryRepository.ts`, and keep transport timeouts short for start/query/cancel.

- [ ] **Step 2: Add search state helpers**

Implement `sessionSearchState.ts` with full-result merging, query highlighting segments, project display filtering, and adaptive poll delay.

- [ ] **Step 3: Add turn jump support**

Add `resolveChatDisplayScrollIndex(displayIndex, turnIndex)` and call it from `ChatVirtuosoTurnList.scrollToTurnIndex`.

- [ ] **Step 4: Add UI state and fan-out**

In `main.tsx`, add in-memory state for search input, expanded controls, active search id, per-project status, results, errors, and polling timers. Starting a new search cancels the previous UI search id across all known projects before starting the next one.

- [ ] **Step 5: Render search mode**

Render search controls in the session list area on desktop and mobile. In search mode, hide ordinary sessions and show only result sessions, keeping normal `projectSessionsByProjectId` updates in the background.

- [ ] **Step 6: Add prompt-result navigation**

Clicking a prompt result opens the session, scrolls to `turnIndex`, and highlights that turn for about 2 seconds.

- [ ] **Step 7: Verify GREEN**

Run the same Jest command from Task 3 and then:

```powershell
npm run tsc:web
```

Expected: PASS.

---

### Task 5: Full Verification And Completion

- [ ] **Step 1: Run server verification**

```powershell
go test ./...
```

- [ ] **Step 2: Run app verification**

```powershell
npm test -- --runInBand
npm run tsc:web
```

- [ ] **Step 3: Build web assets**

```powershell
npm run build:web
```

- [ ] **Step 4: Inspect git diff**

```powershell
git status --short
git diff --stat
```

- [ ] **Step 5: Commit and push**

Per the root completion gate:

```powershell
git add -A
git commit -m "feat: add session search"
git push origin feat/session-search
```
