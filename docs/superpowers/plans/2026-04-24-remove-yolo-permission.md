# Remove YOLO And Permission Flow Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove the `yolo` mode switch and the cross-layer permission flow so permission requests are always resolved internally and never reach IM, recorder, or web UI.

**Architecture:** Collapse permission handling into synchronous session-local selection, delete IM permission protocol surfaces, and remove `yolo` from runtime config and SQLite schema. Keep ACP permission request/response types intact only at the agent callback boundary.

**Tech Stack:** Go server, SQLite via `modernc.org/sqlite`, TypeScript web client, Jest tests.

---

### Task 1: Freeze Design Artifacts

**Files:**
- Create: `docs/superpowers/specs/2026-04-24-remove-yolo-permission-design.md`
- Create: `docs/superpowers/plans/2026-04-24-remove-yolo-permission.md`

- [ ] **Step 1: Confirm spec file exists with final decisions**

Run: `Test-Path d:\Code\WheelMaker\docs\superpowers\specs\2026-04-24-remove-yolo-permission-design.md`
Expected: `True`

- [ ] **Step 2: Confirm plan file exists**

Run: `Test-Path d:\Code\WheelMaker\docs\superpowers\plans\2026-04-24-remove-yolo-permission.md`
Expected: `True`

### Task 2: Remove Server Runtime Permission Flow

**Files:**
- Modify: `server/internal/hub/client/permission.go`
- Modify: `server/internal/hub/client/session.go`
- Modify: `server/internal/hub/client/client.go`
- Test: `server/internal/hub/client/client_test.go`

- [ ] **Step 1: Write failing tests for immediate internal permission resolution**

Add tests that assert:

```go
func TestSessionRequestPermissionAutoAllowsWithoutIMRoundTrip(t *testing.T) {}

func TestSessionRequestPermissionCancelsWhenNoAllowOptionExists(t *testing.T) {}
```

- [ ] **Step 2: Run only the new client permission tests and verify they fail**

Run: `cd d:\Code\WheelMaker\server; go test ./internal/hub/client -run "TestSessionRequestPermissionAutoAllowsWithoutIMRoundTrip|TestSessionRequestPermissionCancelsWhenNoAllowOptionExists"`
Expected: FAIL because current code still depends on IM / yolo state

- [ ] **Step 3: Implement minimal runtime removal**

Change code so that:

```go
func (s *Session) decidePermission(ctx context.Context, requestID int64, params acp.PermissionRequestParams, _ string) (acp.PermissionResult, error) {
    _ = ctx
    _ = requestID
    if optionID := chooseAutoAllowOption(params.Options); optionID != "" {
        return acp.PermissionResult{Outcome: "selected", OptionID: optionID}, nil
    }
    return acp.PermissionResult{Outcome: "cancelled"}, nil
}
```

Delete now-unused pending slot, pending map, IM resolve path, and `yolo` fields from client/session.

- [ ] **Step 4: Re-run the focused client permission tests**

Run: `cd d:\Code\WheelMaker\server; go test ./internal/hub/client -run "TestSessionRequestPermissionAutoAllowsWithoutIMRoundTrip|TestSessionRequestPermissionCancelsWhenNoAllowOptionExists"`
Expected: PASS

### Task 3: Remove Recorder Permission Persistence

**Files:**
- Modify: `server/internal/hub/client/session_recorder.go`
- Test: `server/internal/hub/client/client_test.go`

- [ ] **Step 1: Write failing tests showing permission events are ignored**

Add tests that assert new `request_permission` events do not produce persisted turns or published messages.

- [ ] **Step 2: Run only recorder permission tests and verify they fail**

Run: `cd d:\Code\WheelMaker\server; go test ./internal/hub/client -run "TestSessionViewPermission"`
Expected: FAIL because current recorder still stores permission turns

- [ ] **Step 3: Remove permission parse, merge, and payload support**

Delete `request_permission` handling from recorder entry, converted message builders, merge plans, and extra metadata.

- [ ] **Step 4: Re-run the focused recorder tests**

Run: `cd d:\Code\WheelMaker\server; go test ./internal/hub/client -run "TestSessionViewPermission|TestSessionViewDropsOrphanPermissionResult"`
Expected: PASS with updated expectations or removed legacy tests replaced by new ignore-path coverage

### Task 4: Remove IM And Registry Permission APIs

**Files:**
- Modify: `server/internal/protocol/im_protocol.go`
- Modify: `server/internal/im/router.go`
- Modify: `server/internal/im/app/app.go`
- Modify: `server/internal/im/feishu/*`
- Modify: `server/internal/registry/server.go`
- Modify: `server/internal/hub/reporter.go`
- Test: `server/internal/im/router_test.go`
- Test: `server/internal/im/app/app_test.go`
- Test: `server/internal/registry/server_test.go`

- [ ] **Step 1: Write failing tests for removed permission transport endpoints**

Add or update tests so `chat.permission.respond` is rejected / absent and IM router no longer exposes permission request forwarding.

- [ ] **Step 2: Run focused IM and registry tests and verify they fail**

Run: `cd d:\Code\WheelMaker\server; go test ./internal/im ./internal/registry -run "Permission|chat.permission.respond"`
Expected: FAIL because the old permission API still exists

- [ ] **Step 3: Remove IM permission types and endpoints**

Delete permission-specific IM message types, channel methods, router handling, app pending request state, registry forwarding, and reporter allowlist entries.

- [ ] **Step 4: Re-run the focused IM and registry tests**

Run: `cd d:\Code\WheelMaker\server; go test ./internal/im ./internal/registry -run "Permission|chat.permission.respond"`
Expected: PASS with updated deletion-oriented coverage

### Task 5: Remove YOLO Config And SQLite Schema Support

**Files:**
- Modify: `server/internal/shared/config.go`
- Modify: `server/internal/hub/hub.go`
- Modify: `server/internal/hub/client/sqlite_store.go`
- Test: `server/internal/hub/client/client_test.go`
- Test: `server/internal/hub/hub_test.go`

- [ ] **Step 1: Write failing tests for final schema shape and startup rejection**

Add/update tests that assert `projects` no longer contains `yolo` and old schemas still trigger mismatch.

- [ ] **Step 2: Run schema-focused tests and verify they fail**

Run: `cd d:\Code\WheelMaker\server; go test ./internal/hub/client ./internal/hub -run "Schema|YOLO|LoadProject"`
Expected: FAIL because code and expected columns still include `yolo`

- [ ] **Step 3: Remove `yolo` from config, runtime wiring, and schema**

Update config structs, hub client construction, store schema, load/save logic, and mismatch expectations to the new column set without migration code.

- [ ] **Step 4: Re-run the schema-focused tests**

Run: `cd d:\Code\WheelMaker\server; go test ./internal/hub/client ./internal/hub -run "Schema|YOLO|LoadProject"`
Expected: PASS

### Task 6: Remove Web Permission UI And API

**Files:**
- Modify: `app/web/src/types/registry.ts`
- Modify: `app/web/src/services/registryRepository.ts`
- Modify: `app/web/src/services/registryWorkspaceService.ts`
- Modify: `app/web/src/main.tsx`
- Modify: `app/web/src/styles.css`
- Test: `app/__tests__/web-chat-ui.test.ts`

- [ ] **Step 1: Write failing tests for deleted permission UI path**

Update the web integration test to assert the repository no longer contains `chat.permission.respond`, the service no longer exposes `respondToSessionPermission`, and the UI no longer renders permission actions.

- [ ] **Step 2: Run the focused web test and verify it fails**

Run: `cd d:\Code\WheelMaker\app; npm test -- --runInBand __tests__/web-chat-ui.test.ts`
Expected: FAIL because current web code still contains permission support

- [ ] **Step 3: Remove web permission parsing and rendering**

Delete permission kind handling, repository call, service wrapper, action button rendering, and permission-only CSS.

- [ ] **Step 4: Re-run the focused web test**

Run: `cd d:\Code\WheelMaker\app; npm test -- --runInBand __tests__/web-chat-ui.test.ts`
Expected: PASS

### Task 7: Run End-To-End Focused Verification

**Files:**
- Modify: `docs/session-recorder-record-event.md`

- [ ] **Step 1: Update user-facing docs for the new behavior**

Document that permission no longer enters recorder history and `yolo` no longer exists.

- [ ] **Step 2: Run the touched server package tests**

Run: `cd d:\Code\WheelMaker\server; go test ./internal/hub/client ./internal/im ./internal/registry ./internal/hub`
Expected: PASS

- [ ] **Step 3: Run the touched web tests**

Run: `cd d:\Code\WheelMaker\app; npm test -- --runInBand __tests__/web-chat-ui.test.ts`
Expected: PASS

- [ ] **Step 4: Record the manual upgrade requirement**

Ensure final user communication includes: delete local DB directory before restarting this version.

### Task 8: Commit Implementation

**Files:**
- Modify: all changed files from previous tasks

- [ ] **Step 1: Review git diff for only intended files**

Run: `git -C d:\Code\WheelMaker diff --stat`
Expected: only server/app/docs files related to yolo and permission removal

- [ ] **Step 2: Commit the changes**

Run: `git -C d:\Code\WheelMaker add -A && git -C d:\Code\WheelMaker commit -m "refactor: remove yolo permission flow"`
Expected: commit succeeds

- [ ] **Step 3: Push the branch**

Run: `git -C d:\Code\WheelMaker push origin main`
Expected: push succeeds