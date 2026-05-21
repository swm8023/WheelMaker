# Codex Agent Identity Rename Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rename external Codex agent identities so `codex` means Codex app-server and `codexacp` means the legacy codex-acp path, with automatic store migration and protocol version gating.

**Architecture:** Treat the rename as a protocol and persistence boundary change. Keep the Codex app-server bridge internals named `codexapp*`, but expose it through provider id `codex`; keep the legacy ACP provider available as `codexacp` for managed historical sessions only. Upgrade registry protocol to `2.3` so mixed old/new hub connections are rejected instead of guessed by Web.

**Tech Stack:** Go server (`internal/protocol`, `internal/hub`, `internal/hub/agent`, `internal/hub/client`, `internal/registry`), React/TypeScript Web UI (`app/web/src`), Jest, Go tests, SQLite store migration.

---

### Task 1: Protocol Version Gate

**Files:**
- Modify: `server/internal/protocol/registry.go`
- Modify: `server/internal/registry/server_test.go`
- Modify: `server/internal/hub/reporter_test.go`
- Modify: `app/web/src/services/registryRepository.ts`
- Test: existing registry/reporter/web service tests

- [ ] **Step 1: Write failing tests**

Add or update tests so registry/local-read expect `2.3` and reject `2.2`, and Web source contains `protocolVersion: '2.3'`.

- [ ] **Step 2: Run focused tests and verify RED**

Run:

```powershell
cd server; go test ./internal/registry ./internal/hub -run "ConnectInit|LocalRead|Protocol" -count=1
cd app; npm test -- --runTestsByPath __tests__/web-local-hub-read-service.test.ts __tests__/web-registry-client-debug.test.ts --runInBand
```

Expected: failures showing current `2.2` constants or payloads.

- [ ] **Step 3: Implement protocol `2.3`**

Update `DefaultProtocolVersion` and Web connect init payloads to `2.3`. Keep exact-match rejection semantics.

- [ ] **Step 4: Run focused tests and verify GREEN**

Run the same commands and confirm they pass.

### Task 2: Provider Identity Rename

**Files:**
- Modify: `server/internal/protocol/acp_const.go`
- Modify: `server/internal/hub/agent/acp_provider.go`
- Modify: `server/internal/hub/agent/codexapp_agent.go`
- Modify: `server/internal/hub/agent/codexapp_convert.go`
- Modify: `server/internal/hub/agent/factory.go`
- Modify: `server/internal/hub/agent/agent_test.go`
- Test: `server/internal/hub/agent/agent_test.go`

- [ ] **Step 1: Write failing provider tests**

Add tests that `ParseACPProvider("codex")` resolves to app-server provider, `ParseACPProvider("codexacp")` resolves to legacy ACP provider, and `ParseACPProvider("codexapp")` is rejected.

- [ ] **Step 2: Run focused tests and verify RED**

Run:

```powershell
cd server; go test ./internal/hub/agent ./internal/protocol -run "Codex|ParseACPProvider|DefaultACPFactory" -count=1
```

Expected: failures because `codexapp` is still accepted and `codexacp` is unknown.

- [ ] **Step 3: Implement provider identity rename**

Expose Codex app-server as `codex`, expose legacy codex-acp as `codexacp`, and keep `codexacp` out of preferred new-session fallback.

- [ ] **Step 4: Run focused tests and verify GREEN**

Run the same command and confirm it passes.

### Task 3: Store Migration and Session Entry Guards

**Files:**
- Modify: `server/internal/hub/client/sqlite_store.go`
- Modify: `server/internal/hub/client/client.go`
- Modify: `server/internal/hub/client/session_recovery.go`
- Modify: `server/internal/hub/client/client_test.go`
- Test: existing client tests

- [ ] **Step 1: Write failing migration and guard tests**

Add tests that opening a store migrates `codexapp -> codex` and `codex -> codexacp` across sessions, project defaults, and agent preferences. Add tests that `ClientNewSession`, `session.new`, `/new`, `session.resume.list`, and `session.resume.import` reject `codexacp`, while existing `codexacp` sessions still load through `SessionByID`.

- [ ] **Step 2: Run focused tests and verify RED**

Run:

```powershell
cd server; go test ./internal/hub/client -run "Codex|Migration|NewSession|Resume" -count=1
```

Expected: failures because migration and guards do not exist.

- [ ] **Step 3: Implement migration and guards**

Run store migration during `NewStore`, normalize stored old identities, add shared guard helpers for creation/import, and leave existing managed session load paths intact.

- [ ] **Step 4: Run focused tests and verify GREEN**

Run the same command and confirm it passes.

### Task 4: Web UI and NPM Metadata

**Files:**
- Modify: `app/web/src/main.tsx`
- Modify: `server/internal/hub/npm_command.go`
- Modify: `server/internal/hub/npm_command_test.go`
- Modify: `app/__tests__/web-chat-ui.test.ts`
- Modify: `app/__tests__/web-agent-package-update-settings.test.ts`
- Test: focused Jest and Go NPM tests

- [ ] **Step 1: Write failing UI/NPM tests**

Update tests to require `codexacp` filtered from New/Resume menus, `codex`/`codexacp` tag variants, and NPM agentTypes `@openai/codex -> codex`, `@zed-industries/codex-acp -> codexacp`.

- [ ] **Step 2: Run focused tests and verify RED**

Run:

```powershell
cd app; npm test -- --runTestsByPath __tests__/web-chat-ui.test.ts __tests__/web-agent-package-update-settings.test.ts --runInBand
cd server; go test ./internal/hub -run "NPM" -count=1
```

Expected: failures because old UI and package metadata still reference `codexapp`.

- [ ] **Step 3: Implement Web and NPM updates**

Filter `codexacp` from agent pickers, update tag variant indexes, update package policies, and adjust tests.

- [ ] **Step 4: Run focused tests and verify GREEN**

Run the same commands and confirm they pass.

### Task 5: Docs, Full Verification, and Commit

**Files:**
- Modify: `docs/registry-protocol.md`
- Modify: current implementation files from previous tasks
- Include: existing deleted `.agents/skills/vercel-react-native-skills/**` and `.claude/skills/vercel-react-native-skills/**` paths because the user requested committing them

- [ ] **Step 1: Update protocol docs**

Document Registry Protocol 2.3 and the Codex identity breaking change.

- [ ] **Step 2: Run full verification**

Run:

```powershell
cd server; go test ./...
cd app; npm test -- --runInBand
cd app; npm run tsc:web
```

Expected: all pass.

- [ ] **Step 3: Commit, push, and publish according to repo gate**

Run:

```powershell
git add -A
git commit -m "feat: rename codex agent identities"
git push origin main
cd app; npm run build:web:release
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/signal_update_now.ps1 -DelaySeconds 30 -SkipWebPublish
```

Expected: commit and push succeed, web release build succeeds, server update signal is accepted.
