# Mobile Chat Project Session Drawer Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the mobile Chat drawer's single-project session list with a cross-project project/session index while keeping File/Git project-scoped.

**Architecture:** Reuse the existing wide project/session rail data and project-scoped actions in `app/web/src/main.tsx`, but render mobile Chat with a dedicated drawer header and inline action area. Promote project collapse state from desktop-only to shared UI state, move Token Stats to Settings as a detail page, and leave Agent Info unreachable.

**Tech Stack:** React 19, TypeScript, CSS, Jest structural tests, existing `WorkspaceStore` and registry workspace service.

---

### Task 1: Lock the Mobile Chat Drawer Contract

**Files:**
- Modify: `app/__tests__/web-chat-ui.test.ts`
- Modify: `app/__tests__/web-responsive-ui-state.test.ts`

- [ ] **Step 1: Add failing structural tests**

Add assertions that require:

```ts
expect(mainTsx).toContain('const renderMobileChatSessionSheet = () => {');
expect(mainTsx).toContain('className="mobile-chat-drawer-header"');
expect(mainTsx).toContain('<span className="mobile-chat-drawer-title">Chats</span>');
expect(mainTsx).toContain('className="mobile-project-session-nav"');
expect(mainTsx).toContain('className="mobile-project-action-panel"');
expect(mainTsx).toContain('const refreshMobileChatProjectSessions = async () => {');
expect(mainTsx).toContain('service.listProjectSessions(projectItem.projectId)');
expect(mainTsx).toContain('setDrawerOpen(false);');
expect(mainTsx).not.toContain('title="Token stats"');
expect(mainTsx).not.toContain('title="Agent info"');
expect(mainTsx).not.toContain('className="chat-session-swipe-row"');
expect(stylesCss).toContain('.mobile-project-session-nav {');
expect(stylesCss).toContain('.mobile-project-action-panel {');
expect(stylesCss).toContain('--mobile-floating-control-lane: 72px;');
expect(stylesCss).toContain('right: var(--mobile-floating-control-lane);');
```

Update responsive state tests to expect `collapsedProjectIds` in shared state and the `shared/setCollapsedProjectIds` action.

- [ ] **Step 2: Run tests to verify red**

Run: `cd app && npm test -- __tests__/web-chat-ui.test.ts __tests__/web-responsive-ui-state.test.ts --runInBand`

Expected: FAIL on missing mobile Chat drawer symbols and shared collapse state.

### Task 2: Promote Project Collapse State

**Files:**
- Modify: `app/web/src/services/workspacePersistence.ts`
- Modify: `app/web/src/services/workspaceUiState.ts`
- Modify: `app/web/src/main.tsx`

- [ ] **Step 1: Rename the live UI state**

In `workspaceUiState.ts`, move `collapsedProjectIds` under `shared`, accept both `collapsedProjectIds` and the legacy `desktopCollapsedProjectIds` input, and replace `desktop/setCollapsedProjectIds` with `shared/setCollapsedProjectIds`.

- [ ] **Step 2: Keep persistence backward compatible**

In `workspacePersistence.ts`, add `collapsedProjectIds` to `PersistedGlobalState` while still reading/writing the legacy `desktopCollapsedProjectIds` key. Use `collapsedProjectIds` in app code and keep legacy writes as compatibility until a later cleanup.

- [ ] **Step 3: Wire main to shared collapse**

In `main.tsx`, initialize `collapsedProjectIds` from global state, use `workspaceUiState.shared.collapsedProjectIds`, and dispatch `shared/setCollapsedProjectIds`.

- [ ] **Step 4: Run responsive state tests**

Run: `cd app && npm test -- __tests__/web-responsive-ui-state.test.ts --runInBand`

Expected: PASS.

### Task 3: Add Mobile Cross-Project Chat Drawer

**Files:**
- Modify: `app/web/src/main.tsx`
- Modify: `app/web/src/styles.css`
- Modify: `app/__tests__/web-chat-ui.test.ts`

- [ ] **Step 1: Add mobile action state and refresh helpers**

Add mobile inline action state, per-project session list error state, and `refreshMobileChatProjectSessions`. The helper refreshes projects and calls only `listProjectSessions(projectId)` for session indexes.

- [ ] **Step 2: Reuse project-scoped entry flows**

Rename `selectWideProjectSession` to a shared project-scoped selection helper or add a shared wrapper that closes the mobile drawer on mobile. Ensure create/resume success enters the resulting session through the same path.

- [ ] **Step 3: Render `renderMobileChatSessionSheet`**

Render a Chat-only drawer header with settings, `Chats`, and refresh. Under it, render all projects, hub tags, project New/Resume buttons, inline mobile action panel, sessions, agent tags, times, empty state, and show-more.

- [ ] **Step 4: Keep File/Git project headers**

Make `renderSidebar` choose the mobile Chat header/sheet only for `tab === 'chat'`; File/Git continue to use the existing settings + project pill + refresh header.

- [ ] **Step 5: Remove mobile Chat global actions**

Remove mobile Chat global New, Resume, Token Stats, Agent Info header actions and remove the old swipe row path from the mobile Chat drawer list.

- [ ] **Step 6: Run Chat UI tests**

Run: `cd app && npm test -- __tests__/web-chat-ui.test.ts --runInBand`

Expected: PASS.

### Task 4: Move Token Stats to Settings

**Files:**
- Modify: `app/web/src/main.tsx`
- Modify: `app/web/src/styles.css`
- Modify: `app/__tests__/web-chat-ui.test.ts`

- [ ] **Step 1: Add settings detail state**

Add `settingsDetailView: '' | 'tokenStats'` and render a Token Stats settings detail page from `renderSettingsContent`.

- [ ] **Step 2: Reuse existing Token Stats content**

Move the current Token Stats card/list body into the settings detail renderer. Opening the detail triggers `refreshTokenStats` through the existing effect.

- [ ] **Step 3: Keep Agent Info unreachable**

Remove reachable Agent Info buttons and settings entries. Internal helper code can remain if not worth untangling in this task.

- [ ] **Step 4: Run targeted tests**

Run: `cd app && npm test -- __tests__/web-chat-ui.test.ts --runInBand`

Expected: PASS.

### Task 5: Fix Mobile Drawer Width and Floating Lane

**Files:**
- Modify: `app/web/src/shell/ResponsiveShell.tsx`
- Modify: `app/web/src/styles.css`
- Modify: `app/__tests__/web-responsive-shell.test.ts`
- Modify: `app/__tests__/web-chat-ui.test.ts`

- [ ] **Step 1: Make drawer overlay leave the floating lane clickable**

Use CSS custom property `--mobile-floating-control-lane: 72px;` and set `.drawer-overlay { right: var(--mobile-floating-control-lane); }`. Keep `.floating-control-stack-layer` above or outside this clickable lane.

- [ ] **Step 2: Widen drawer consistently**

Set drawer width to `min(420px, calc(100vw - var(--mobile-floating-control-lane) - env(safe-area-inset-right, 0px)))` so Chat/File/Git match.

- [ ] **Step 3: Make active Chat floating button close drawer**

Adjust mobile floating nav click behavior so tapping active Chat with the drawer open toggles it closed; tapping File/Git switches tab and closes drawer.

- [ ] **Step 4: Run shell and Chat tests**

Run: `cd app && npm test -- __tests__/web-responsive-shell.test.ts __tests__/web-chat-ui.test.ts --runInBand`

Expected: PASS.

### Task 6: Final Verification

**Files:**
- Modify: all touched files

- [ ] **Step 1: Run typecheck**

Run: `cd app && npm run tsc:web`

Expected: PASS.

- [ ] **Step 2: Run full tests**

Run: `cd app && npm test -- --runInBand`

Expected: all suites pass.

- [ ] **Step 3: Run production web build**

Run: `cd app && npm run build:web`

Expected: build exits 0; existing bundle size warnings are acceptable.

- [ ] **Step 4: Run repository whitespace check**

Run: `git diff --check`

Expected: exit 0.

- [ ] **Step 5: Follow repository completion gate**

Run:

```powershell
git add -A
git commit -m "feat: add mobile project chat drawer"
git push origin main
cd app
npm run build:web:release
```

Expected: push succeeds and release exports to `C:\Users\suweimin\.wheelmaker\web`.

## Self-Review

- Spec coverage: all confirmed decisions are covered by Tasks 1-5.
- Placeholder scan: no placeholders remain.
- Type consistency: state names are `collapsedProjectIds`, `settingsDetailView`, and `mobileProjectActionMenu` throughout the plan.
