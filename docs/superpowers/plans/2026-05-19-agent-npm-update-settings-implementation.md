# Agent NPM Update Settings Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the approved Settings `Update` page and controlled hub-level npm package management path.

**Architecture:** Registry exposes exactly one new hub-level method, `cmd.npm`, and forwards it by `hubId` without owning task state. Hub owns a small in-memory npm command handler with hard-coded package policy and one running task per hub. The Web app scans online hubs, renders package state in Settings, and uses the shared confirmation dialog for install/update/uninstall.

**Tech Stack:** Go registry/hub server, PowerShell/Bash deployment scripts, React/TypeScript Web app, Jest source-structure tests, Go unit tests.

---

### Task 1: Registry Hub-Level Forwarding

**Files:**
- Modify: `server/internal/registry/server.go`
- Modify: `server/internal/registry/server_test.go`

- [ ] **Step 1: Write failing registry tests**

Add tests that connect a hub and client, call `cmd.npm` with a payload containing `hubId`, assert the hub receives the forwarded request without `projectId`, and assert unknown `cmd.shell` remains forbidden.

Run: `go test ./internal/registry -run "CmdNPM|CmdPrefix" -count=1`
Expected before implementation: FAIL because `cmd.npm` is not allowlisted or forwarded.

- [ ] **Step 2: Implement registry forwarding**

Add `cmd.npm` to client allowlist only. In the main request switch and batch executor, route `cmd.npm` to a new hub-forward helper that decodes `hubId`, validates online hub state, forwards the original payload to that hub, and uses a 60 second timeout only for `action:"scan"`.

- [ ] **Step 3: Verify registry tests**

Run: `go test ./internal/registry -run "CmdNPM|CmdPrefix" -count=1`
Expected: PASS.

### Task 2: Hub NPM Command Handler

**Files:**
- Create: `server/internal/hub/npm_command.go`
- Create: `server/internal/hub/npm_command_test.go`
- Modify: `server/internal/hub/reporter.go`
- Modify: `server/internal/hub/reporter_test.go`

- [ ] **Step 1: Write failing hub command tests**

Cover scan from fake `npm list`, latest version lookup, deprecated package visibility, install acceptance, deprecated uninstall acceptance, unsupported package rejection, runtime uninstall rejection, deprecated install rejection, one-task conflict, query with no history, and failed task error summary truncation.

Run: `go test ./internal/hub -run "NPMCommand|CmdNPM" -count=1`
Expected before implementation: FAIL because the handler does not exist.

- [ ] **Step 2: Implement the handler**

Implement hard-coded runtime packages `@zed-industries/codex-acp`, `@agentclientprotocol/claude-agent-acp`, `@anthropic-ai/claude-code`, `@openai/codex`, `@github/copilot`, and `opencode-ai`, plus deprecated `@zed-industries/claude-agent-acp`. Use `npm list -g --depth=0 --json`, concurrent `npm view <package> version` for runtime packages, `npm install -g <package>@latest`, and `npm uninstall -g <package>`.

- [ ] **Step 3: Wire reporter dispatch**

Add `cmd.npm` handling in `Reporter.handleRegistryRequest`. Return typed registry error codes for invalid payload, unsupported package, and per-hub task conflict.

- [ ] **Step 4: Verify hub tests**

Run: `go test ./internal/hub -run "NPMCommand|CmdNPM" -count=1`
Expected: PASS.

### Task 3: Provider And Script Alignment

**Files:**
- Modify: `server/internal/hub/agent/acp_provider.go`
- Modify: `server/internal/hub/agent/agent_test.go`
- Modify: `scripts/refresh_server.sh`
- Modify: `scripts/test_refresh_server_sh.ps1`

- [ ] **Step 1: Write failing provider and script tests**

Update provider tests to expect `codex-acp` and `claude-agent-acp` global binaries, not `npx`. Update script source checks to assert macOS/Linux refresh requires `npm`, installs missing Codex/Claude ACP packages, removes deprecated Claude ACP, and no longer requires `npx`.

Run: `go test ./internal/hub/agent -run "CodexACPProvider|ClaudeACPProvider" -count=1`
Run: `powershell -NoProfile -ExecutionPolicy Bypass -File scripts/test_refresh_server_sh.ps1`
Expected before implementation: FAIL.

- [ ] **Step 2: Implement provider and script changes**

Remove default `npx --yes` behavior for Codex/Claude presets. Keep `codexapp` unchanged. Add `ensure_acp_dependencies` behavior to `refresh_server.sh` mirroring the Windows script.

- [ ] **Step 3: Verify provider and script tests**

Run both commands from Step 1.
Expected: PASS.

### Task 4: App Data Layer And Pure Helpers

**Files:**
- Modify: `app/web/src/types/registry.ts`
- Modify: `app/web/src/services/registryRepository.ts`
- Modify: `app/web/src/services/registryWorkspaceService.ts`
- Create: `app/web/src/agentPackageUpdateView.ts`
- Create: `app/__tests__/web-agent-package-update-service.test.ts`
- Create: `app/__tests__/web-agent-package-update-view.test.ts`

- [ ] **Step 1: Write failing app data tests**

Test that repository methods send `cmd.npm` scan/install/uninstall/query payloads with `hubId`, that scan uses `timeoutMs: 60000`, and that pure helpers derive sorted online hub IDs from `project.list`.

Run: `cd app && npm test -- --runTestsByPath __tests__/web-agent-package-update-service.test.ts __tests__/web-agent-package-update-view.test.ts --runInBand`
Expected before implementation: FAIL because files and methods do not exist.

- [ ] **Step 2: Implement types, repository methods, service methods, and helpers**

Add `RegistryNpmPackage`, `RegistryNpmHubSnapshot`, `RegistryNpmTask`, and response types. Implement `scanNpmPackages`, `installNpmPackage`, `uninstallNpmPackage`, and `queryNpmPackageTask`.

- [ ] **Step 3: Verify app data tests**

Run the command from Step 1.
Expected: PASS.

### Task 5: Settings Update UI

**Files:**
- Modify: `app/web/src/main.tsx`
- Modify: `app/web/src/styles.css`
- Modify: `app/__tests__/web-chat-ui.test.ts`
- Create: `app/__tests__/web-agent-package-update-settings.test.ts`

- [ ] **Step 1: Write failing Settings UI tests**

Test `SettingsDetailView` includes `update`, Settings sections use `More` instead of `Storage`, More row order is `Update`, `Token Stats`, `CC Switch`, `Database`, `Clear Local Cache`, desktop shortcuts use `codicon-cloud-download` and `codicon-pulse`, mobile gets no shortcut buttons, and the Update page contains `Agent Packages` plus confirmation flow hooks.

Run: `cd app && npm test -- --runTestsByPath __tests__/web-agent-package-update-settings.test.ts __tests__/web-chat-ui.test.ts --runInBand`
Expected before implementation: FAIL.

- [ ] **Step 2: Implement Settings UI**

Add Update page state, scanning, write action confirmation, query polling, all-hub rescan after completion, hub cards, package rows, status labels, and desktop shortcut active-state logic.

- [ ] **Step 3: Verify Settings UI tests**

Run the command from Step 1.
Expected: PASS.

### Task 6: Integration Verification And Completion

**Files:**
- All changed files.

- [ ] **Step 1: Run focused Go tests**

Run: `cd server && go test ./internal/registry ./internal/hub ./internal/hub/agent`
Expected: PASS.

- [ ] **Step 2: Run focused app tests and typecheck**

Run: `cd app && npm test -- --runTestsByPath __tests__/web-agent-package-update-service.test.ts __tests__/web-agent-package-update-view.test.ts __tests__/web-agent-package-update-settings.test.ts __tests__/web-chat-ui.test.ts --runInBand`
Run: `cd app && npm run tsc:web`
Expected: PASS.

- [ ] **Step 3: Run script source checks**

Run: `powershell -NoProfile -ExecutionPolicy Bypass -File scripts/test_refresh_server_sh.ps1`
Expected: PASS.

- [ ] **Step 4: Build release web assets**

Run: `cd app && npm run build:web:release`
Expected: PASS and publish web assets to `~/.wheelmaker/web`.

- [ ] **Step 5: Commit, push, and signal update**

Run: `git add -A`
Run: `git commit -m "feat: add agent npm update settings"`
Run: `git push origin main`
Run: `powershell -NoProfile -ExecutionPolicy Bypass -File scripts/signal_update_now.ps1 -DelaySeconds 30`
Expected: all commands complete successfully.
