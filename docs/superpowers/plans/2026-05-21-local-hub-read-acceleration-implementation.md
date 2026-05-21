# Local Hub Read Acceleration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add default-on Local Hub Read acceleration so File/Git/project read operations use a same-machine Hub endpoint when available, while Chat/Session continue through the Conversation Registry.

**Architecture:** The Hub starts a loopback-only local read WebSocket on an available port, proves identity with an ephemeral Ed25519 challenge response, then authenticates with the same registry token under role `local_read`. The Registry includes Hub-level local read candidates in `project.list.hubs[]`; the App manages optional local read repositories per Hub and routes only read-safe project operations through them.

**Tech Stack:** Go WebSocket server with Gorilla, Ed25519 proof from the Go standard library, React/TypeScript App services, existing Jest/Go tests, existing CSS system.

---

## File Structure

- Modify `server/internal/protocol/registry.go`: add `LocalReadCandidate` and extend `HubListItem`.
- Modify `server/internal/registry/server.go`: persist candidate data from Hub reports and expose it through `project.list.hubs[]`.
- Modify `server/internal/hub/reporter.go`: start/stop local read endpoint, generate proof identity, include candidate in `registry.reportProjects` and `registry.updateProject`, and reuse existing File/Git/project read handlers.
- Modify `server/internal/hub/hub.go`: pass the registry token to the reporter so local read endpoint can decide whether to start.
- Add focused tests in `server/internal/registry/server_test.go` and `server/internal/hub/reporter_test.go`.
- Modify `app/web/src/types/registry.ts`: add local read candidate and status types.
- Modify `app/web/src/services/registryClient.ts`: add proof request support by reusing `request`.
- Modify `app/web/src/services/registryRepository.ts`: normalize hub candidate metadata and expose the underlying read methods unchanged.
- Add `app/web/src/services/localHubReadManager.ts`: own proof, auth, project match, per-Hub repository lifecycle, and Remote/Local status.
- Modify `app/web/src/services/registryWorkspaceService.ts`: route project read methods through `LocalHubReadManager`, while session/chat/cmd methods continue using the remote repository.
- Modify `app/web/src/services/workspacePersistence.ts`: persist `localHubReadEnabled`, default `true`.
- Modify `app/web/src/main.tsx` and `app/web/src/styles.css`: add Settings switch and Remote/Local tags next to Hub rows.
- Add focused App tests in `app/__tests__/web-local-hub-read-*.test.ts`.

## Task 1: Protocol And Registry Candidate Snapshot

**Files:**
- Modify: `server/internal/protocol/registry.go`
- Modify: `server/internal/registry/server.go`
- Test: `server/internal/registry/server_test.go`

- [x] **Step 1: Write the failing registry test**

Add a test that connects a Hub, sends `registry.reportProjects` with:

```json
"localRead": {
  "endpointId": "local-hub-1",
  "url": "ws://127.0.0.1:53123/ws",
  "proofPublicKey": "base64-public-key",
  "proofFingerprint": "sha256:fingerprint"
}
```

Then connect a client and assert `project.list` returns `hubs[0].localRead.endpointId === "local-hub-1"` and does not place local read metadata on projects.

Run: `cd server && go test ./internal/registry -run TestProjectListIncludesLocalReadCandidate -count=1`

Expected: FAIL because `HubReportProjectsPayload` ignores `localRead` and `HubListItem` only has `hubId`.

- [x] **Step 2: Implement protocol fields**

Add:

```go
type LocalReadCandidate struct {
    EndpointID       string `json:"endpointId"`
    URL              string `json:"url"`
    ProofPublicKey   string `json:"proofPublicKey"`
    ProofFingerprint string `json:"proofFingerprint"`
}
```

Then add `LocalRead *LocalReadCandidate` to `HubSnapshot`, `HubReportProjectsPayload`, `HubUpdateProjectPayload`, and `HubListItem`.

- [x] **Step 3: Persist and expose candidates**

In registry report/update handlers, copy valid candidate values into `HubSnapshot.LocalRead`; in `snapshotProjectListHubs`, emit `rp.HubListItem{HubID: hubID, LocalRead: hub.LocalRead}`.

- [x] **Step 4: Run registry test**

Run: `cd server && go test ./internal/registry -run TestProjectListIncludesLocalReadCandidate -count=1`

Expected: PASS.

## Task 2: Hub Local Read Endpoint

**Files:**
- Modify: `server/internal/hub/reporter.go`
- Modify: `server/internal/hub/hub.go`
- Test: `server/internal/hub/reporter_test.go`

- [x] **Step 1: Write failing proof and routing tests**

Add tests for:

- `LocalReadCandidate()` returns empty when reporter token is empty.
- With a token, local read endpoint binds `127.0.0.1:0`, exposes `local_read.proof`, and signs nonce + endpoint ID.
- `connect.init role=local_read` accepts the shared token.
- `fs.read` succeeds for `hub:project`.
- `session.list` returns `FORBIDDEN`.

Run: `cd server && go test ./internal/hub -run LocalRead -count=1`

Expected: FAIL because there is no local read endpoint API.

- [x] **Step 2: Add endpoint lifecycle**

Add reporter fields for local read listener/server, proof private/public key, endpoint ID, candidate, and a `StartLocalReadEndpoint(ctx)` method that binds `tcp 127.0.0.1:0` only when `cfg.Token` is non-empty.

- [x] **Step 3: Add proof and auth handling**

Allow only `local_read.proof` before auth. For proof payload `{nonce, endpointId}`, sign `endpointId + "\n" + nonce` with the ephemeral private key and return `{endpointId, nonce, signature, proofPublicKey, proofFingerprint}`. Then allow `connect.init` role `local_read` with matching token and protocol version.

- [x] **Step 4: Reuse Hub read handlers**

Dispatch only `project.list`, `project.syncCheck`, File methods, and Git methods. Reject `chat.*`, `session.*`, `cmd.*`, `monitor.*`, and `batch` with `FORBIDDEN`.

- [x] **Step 5: Report candidate**

Start the local read endpoint before `registry.reportProjects`, include `localRead` in report and update payloads, and make endpoint failure non-fatal.

- [x] **Step 6: Run hub tests**

Run: `cd server && go test ./internal/hub -run LocalRead -count=1`

Expected: PASS.

## Task 3: App Local Read Manager And Routing

**Files:**
- Modify: `app/web/src/types/registry.ts`
- Modify: `app/web/src/services/registryClient.ts`
- Modify: `app/web/src/services/registryRepository.ts`
- Add: `app/web/src/services/localHubReadManager.ts`
- Modify: `app/web/src/services/registryWorkspaceService.ts`
- Test: `app/__tests__/web-local-hub-read-service.test.ts`

- [x] **Step 1: Write failing App service tests**

Test that:

- Hub candidate metadata is normalized from `project.list`.
- Local read manager does not send a token until proof verifies.
- File/Git/syncCheck calls for matched project IDs use the local repository.
- Session calls continue using the remote repository.
- Business errors from matched local File/Git calls are not retried remotely.

Run: `cd app && npm test -- --runInBand app/__tests__/web-local-hub-read-service.test.ts`

Expected: FAIL because the manager and routing do not exist.

- [x] **Step 2: Add local read types**

Add `RegistryLocalReadCandidate` and `RegistryHub.localRead?: RegistryLocalReadCandidate`.

- [x] **Step 3: Add manager**

Implement a manager that stores enabled flag, candidates by Hub, repositories by Hub, verified project IDs by Hub, and status `Local`/`Remote`. It verifies proof using WebCrypto Ed25519 when available; if WebCrypto cannot verify Ed25519, it fails closed to Remote.

- [x] **Step 4: Route read-safe calls**

In `RegistryWorkspaceService`, keep `this.repository` as the Conversation Registry and add `this.localHubReadManager`. Use local repository only for `selectProject`, `listDirectory`, `getFileInfo`, `readFile`, Git methods, and `syncCheck`. Keep session/chat/cmd methods remote.

- [x] **Step 5: Run App service tests**

Run: `cd app && npm test -- --runInBand app/__tests__/web-local-hub-read-service.test.ts`

Expected: PASS.

## Task 4: Settings And Remote/Local Tags

**Files:**
- Modify: `app/web/src/services/workspacePersistence.ts`
- Modify: `app/web/src/main.tsx`
- Modify: `app/web/src/styles.css`
- Test: `app/__tests__/web-local-hub-read-ui.test.ts`

- [x] **Step 1: Write failing UI tests**

Assert:

- `localHubReadEnabled` persists and defaults to `true`.
- Settings includes `Local Hub Read` switch.
- Chat Hub dropdown rows render a tag with text `Local` or `Remote`.
- Tag text is simple and no port is rendered.

Run: `cd app && npm test -- --runInBand app/__tests__/web-local-hub-read-ui.test.ts`

Expected: FAIL before UI wiring.

- [x] **Step 2: Add persistence and state**

Add `localHubReadEnabled` to persisted global state, wire it into `RegistryWorkspaceService.setLocalHubReadEnabled`, and save changes from Settings.

- [x] **Step 3: Render tags**

Render `Local` when a Hub has an active verified local read connection, otherwise `Remote`. Keep failure detail only in `title`/debug records, not normal text.

- [x] **Step 4: Run UI tests**

Run: `cd app && npm test -- --runInBand app/__tests__/web-local-hub-read-ui.test.ts`

Expected: PASS.

## Task 5: Full Verification And Completion Gate

**Files:**
- All changed files

- [x] **Step 1: Run focused server tests**

Run: `cd server && go test ./internal/registry ./internal/hub -count=1`

Expected: PASS.

- [x] **Step 2: Run focused App tests**

Run: `cd app && npm test -- --runInBand app/__tests__/web-local-hub-read-service.test.ts app/__tests__/web-local-hub-read-ui.test.ts app/__tests__/web-agent-package-update-view.test.ts app/__tests__/web-registry-debug-records.test.ts`

Expected: PASS.

- [x] **Step 3: Run full server tests**

Run: `cd server && go test ./...`

Expected: PASS.

- [ ] **Step 4: Run required repo completion sequence**

Run:

```powershell
git add -A
git commit -m "feat: add local hub read acceleration"
git push origin main
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/signal_update_now.ps1 -DelaySeconds 30
cd app
npm run build:web:release
```

Expected: all commands PASS. If any command fails, fix and rerun from the failed command.

## Self-Review

- Spec coverage: includes Hub-level candidate, dynamic loopback binding, proof-before-token, default-on App setting, Remote/Local tag, read-only routing, Chat/Session remote-only, and business-error no-retry.
- Placeholder scan: no `TBD`, `TODO`, or generic "implement later" steps.
- Type consistency: `localRead`, `endpointId`, `proofPublicKey`, and `proofFingerprint` are used consistently across Go and TypeScript.
