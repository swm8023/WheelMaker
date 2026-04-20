# Session Recorder ACP-First Refactor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rebuild session recorder to use ACP protocol payloads as the single source of truth, removing redundant DTOs and legacy compatibility logic.

**Architecture:** Replace the current multi-layer projection (`SessionViewEvent` -> recorder DTO -> JSON envelope -> re-hydrated DTO) with an ACP-first event pipeline. Session runtime writes ACP-native events into a simplified store, while `session.read` performs only thin projection needed by UI ordering and pagination. IM publishing path remains unchanged and isolated from recorder persistence.

**Tech Stack:** Go (`server/internal/hub/client`), SQLite (`modernc.org/sqlite`), ACP protocol structs (`server/internal/protocol`), Web registry client (`app/web/src/services` + `app/web/src/types`).

---

## Scope and Non-Goals

**In scope**
- Remove redundant `role/kind/text/status` persistence duplication when ACP payload already exists.
- Remove legacy JSON assembly/hydration loops in recorder/store.
- Keep `session.read` API functional with minimized projection.
- Preserve IM runtime behavior and routing semantics.

**Out of scope**
- Reworking IM channel rendering contracts (`internal/im/*`).
- Re-designing registry transport envelope.
- Adding new user-facing features.

## File Structure and Responsibilities

### Server (core refactor)
- Modify: `server/internal/hub/client/session_recorder.go`
  - Replace `SessionViewEvent`-centric path with ACP-native recorder ingestion.
  - Remove legacy view DTO conversion logic not required by API contract.
- Modify: `server/internal/hub/client/sqlite_store.go`
  - Introduce simplified session event persistence model.
  - Remove `buildSessionTurnContentJSON`/`hydrateSessionTurnLegacyFields` compatibility path.
- Modify: `server/internal/hub/client/session.go`
  - Emit ACP-native recorder events directly from stream loop.
- Modify: `server/internal/hub/client/permission.go`
  - Emit ACP-native permission events directly.
- Modify: `server/internal/hub/client/client_test.go`
  - Update/add characterization + regression tests.

### Protocol and API boundary
- Reuse: `server/internal/protocol/acp.go`
- Reuse: `server/internal/protocol/acp_const.go`
- No protocol wire-format change expected.

### Web client
- Modify: `app/web/src/services/registryRepository.ts`
  - Consume slimmed `session.read` payload without fallback legacy adapters.
- Modify: `app/web/src/types/registry.ts`
  - Narrow types to ACP-first payload model.
- Modify: `app/web/src/main.tsx` (only if required by type changes)

## Task Plan

### Task 1: Lock Baseline Behavior with Characterization Tests

**Files:**
- Modify: `server/internal/hub/client/client_test.go`
- Test: `server/internal/hub/client/client_test.go`

- [ ] **Step 1: Add tests for assistant/thought/tool semantics derived from ACP update types**

```go
// verify agent_message_chunk => assistant/text
// verify agent_thought_chunk => assistant/thought
// verify tool_call(_update) => system/tool
```

- [ ] **Step 2: Add tests for merged persistence semantics**

```go
// stream chunks in one turn, ensure first append + subsequent upsert keep stable sync index
// assert update_index increments while turn identity remains stable
```

- [ ] **Step 3: Add tests for IM isolation**

```go
// recorder persistence failure should not mutate IM routing contract
// PublishSessionUpdate / PublishPromptResult / PublishPermissionRequest behavior remains unchanged
```

- [ ] **Step 4: Run focused tests**

Run: `cd server && go test ./internal/hub/client -run SessionRecorder -count=1`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add server/internal/hub/client/client_test.go
git commit -m "test: lock session recorder baseline semantics"
```

### Task 2: Introduce ACP-Native Recorder Event Model

**Files:**
- Modify: `server/internal/hub/client/session_recorder.go`
- Modify: `server/internal/hub/client/session.go`
- Modify: `server/internal/hub/client/permission.go`
- Test: `server/internal/hub/client/client_test.go`

- [ ] **Step 1: Define minimal recorder input model (ACP payload + metadata only)**

```go
type SessionEvent struct {
  SessionID string
  Method string // session.prompt | session.update | session.permission
  Params json.RawMessage // ACP-native payload
  RequestID int64
  CreatedAt time.Time
  SourceChannel string
  SourceChatID string
}
```

- [ ] **Step 2: Replace `SessionViewEvent` emissions in session runtime**

```go
// session.go stream loop emits ACP session/update directly
// permission.go emits ACP permission request/resolution directly
```

- [ ] **Step 3: Keep external API methods stable (`session.list/read/new/send/markRead`)**

```go
// no wire break for registry request methods
```

- [ ] **Step 4: Run tests**

Run: `cd server && go test ./internal/hub/client -count=1`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add server/internal/hub/client/session_recorder.go server/internal/hub/client/session.go server/internal/hub/client/permission.go server/internal/hub/client/client_test.go
git commit -m "refactor: switch session recorder ingestion to ACP-native events"
```

### Task 3: Simplify Store Schema and Remove Redundant Fields

**Files:**
- Modify: `server/internal/hub/client/sqlite_store.go`
- Test: `server/internal/hub/client/client_test.go`

- [ ] **Step 1: Introduce single event record persistence path**

```go
// remove dual representation: ContentJSON+expanded fields
// persist one canonical ACP payload + minimal index metadata
```

- [ ] **Step 2: Remove legacy build/hydrate compatibility helpers**

```go
// delete buildSessionTurnContentJSON
// delete hydrateSessionTurnLegacyFields
// delete legacy fallback conversions
```

- [ ] **Step 3: Keep pagination cursors stable**

```go
// preserve last_sync_index/last_sync_subindex semantics for incremental read
```

- [ ] **Step 4: Run tests**

Run: `cd server && go test ./internal/hub/client -count=1`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add server/internal/hub/client/sqlite_store.go server/internal/hub/client/client_test.go
git commit -m "refactor: simplify session recorder store to ACP-first schema"
```

### Task 4: Thin `session.read` Projection Only

**Files:**
- Modify: `server/internal/hub/client/session_recorder.go`
- Modify: `server/internal/hub/client/client_test.go`

- [ ] **Step 1: Reduce read-side conversion to ordering + pagination + minimal UI fields**

```go
// do not reconstruct business semantics through legacy DTO re-hydration
// derive display role/kind lazily from ACP update type when needed
```

- [ ] **Step 2: Remove dead legacy read path**

```go
// remove or deprecate ReadSessionView/messages fallback path not used by main session.read contract
```

- [ ] **Step 3: Run tests**

Run: `cd server && go test ./internal/hub/client -count=1`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add server/internal/hub/client/session_recorder.go server/internal/hub/client/client_test.go
git commit -m "refactor: thin session.read projection and remove legacy read path"
```

### Task 5: Frontend Contract Cleanup (No Legacy Adapter)

**Files:**
- Modify: `app/web/src/services/registryRepository.ts`
- Modify: `app/web/src/types/registry.ts`
- Modify: `app/web/src/main.tsx` (if compile requires)
- Test: `app` TypeScript build

- [ ] **Step 1: Remove fallback parsing that assumes legacy mixed message/prompt payload**

```ts
// consume prompt/turn payload from ACP-first backend shape
// keep strict guards for requestId/options/toolCallId
```

- [ ] **Step 2: Align TS types with slim backend payload**

```ts
// remove no-longer-used legacy optional fields
```

- [ ] **Step 3: Run web typecheck/build**

Run: `cd app && npm run tsc:web`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add app/web/src/services/registryRepository.ts app/web/src/types/registry.ts app/web/src/main.tsx
git commit -m "refactor(web): align session payload parsing with ACP-first recorder"
```

### Task 6: End-to-End Verification and Cleanup

**Files:**
- Modify: `server/internal/hub/client/client_test.go` (final touchups if needed)
- Optional docs: `docs/session-persistence-sqlite.md`

- [ ] **Step 1: Full server tests**

Run: `cd server && go test ./... -count=1`
Expected: PASS

- [ ] **Step 2: Full web release build**

Run: `cd app && npm run build:web:release`
Expected: PASS (warnings acceptable, no build failure)

- [ ] **Step 3: IM smoke validation**

Run:
- `chat -> prompt` verify streaming still arrives
- permission request -> response roundtrip
- prompt finish event

Expected: Behavior unchanged from baseline.

- [ ] **Step 4: Commit + push**

```bash
git add -A
git commit -m "refactor: session recorder ACP-first simplification without legacy baggage"
git push origin main
```

## Risks and Mitigations

- **Risk:** Refactor blocks IM updates due to synchronous recorder path.
  - **Mitigation:** Keep IM publish call path untouched; ensure recorder writes are non-blocking or bounded.
- **Risk:** Cursor/pagination regression.
  - **Mitigation:** Preserve `last_sync_index/last_sync_subindex` invariants and add incremental-read tests.
- **Risk:** Frontend parsing mismatch.
  - **Mitigation:** Land server + web contract updates in same change set and run `npm run tsc:web`.

## Acceptance Criteria

- No duplicate persistence of ACP payload + expanded semantic fields.
- No legacy build/hydrate compatibility helpers in recorder/store path.
- `session.read` returns stable prompt/turn ordering and pagination.
- IM session updates, prompt results, and permission flows remain behaviorally unchanged.
- Server tests pass, web build passes, release assets build successfully.
