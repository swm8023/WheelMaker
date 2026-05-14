# Wide Screen Project Session Sidebar Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the wide-screen project/session sidebar that replaces the project dropdown and keeps narrow layouts unchanged.

**Architecture:** Persist wide-only project collapse state in the existing responsive UI reducer and global workspace state. Add project-scoped session service methods, then render a wide sidebar backed by cached and refreshed sessions per project. Keep current chat/file/git main panes intact.

**Tech Stack:** React 19, TypeScript, Jest, CSS, existing registry websocket repository and workspace persistence.

---

### Task 1: Persist Wide Project Collapse State

**Files:**
- Modify: `app/web/src/services/workspacePersistence.ts`
- Modify: `app/web/src/services/workspaceUiState.ts`
- Modify: `app/web/src/main.tsx`
- Test: `app/__tests__/web-responsive-ui-state.test.ts`

- [x] Add a failing test that initializes `desktopCollapsedProjectIds`, toggles layout mode, and expects the desktop list to survive.
- [x] Add `desktopCollapsedProjectIds: string[]` to persisted global state with sanitization and global key persistence.
- [x] Add `desktop.collapsedProjectIds` and `desktop/setCollapsedProjectIds` to `workspaceUiState`.
- [x] Initialize the reducer from persisted global state in `main.tsx` and include the value in `rememberGlobalState`.
- [x] Run `npm test -- __tests__/web-responsive-ui-state.test.ts --runTestsByPath`.

### Task 2: Add Project-Scoped Session Service Methods

**Files:**
- Modify: `app/web/src/services/registryWorkspaceService.ts`
- Test: `app/__tests__/web-chat-project-scope.test.ts`

- [x] Add a failing test for method signatures and repository delegation names.
- [x] Add service methods for project-scoped session list, create, resume list, resume import, and reload.
- [x] Keep existing current-project chat methods unchanged.
- [x] Run `npm test -- __tests__/web-chat-project-scope.test.ts --runTestsByPath`.

### Task 3: Render Wide Project Session Navigation

**Files:**
- Modify: `app/web/src/main.tsx`
- Modify: `app/web/src/styles.css`
- Test: `app/__tests__/web-chat-ui.test.ts`

- [x] Add failing structural tests for the wide project session nav, header project picker removal, project action buttons, and CSS classes.
- [x] Add runtime state for `projectSessionsByProjectId`, per-project visible counts, and the anchored project action popover.
- [x] Hydrate each project's cached sessions when wide projects are available.
- [x] Refresh each project's sessions from the registry with project-scoped service methods while wide.
- [x] Render `renderWideProjectSessionNav()` for wide sidebar content; keep `renderSidebarMain()` for narrow.
- [x] Remove the wide header project picker while leaving narrow drawer project controls intact.
- [x] Add CSS for the dense project/session rail, hub pill, subdued action icons, session rows, active project, active session, show-more row, and anchored popover.
- [x] Run `npm test -- __tests__/web-chat-ui.test.ts --runTestsByPath`.

### Task 4: Wire Wide Session Actions

**Files:**
- Modify: `app/web/src/main.tsx`
- Test: `app/__tests__/web-chat-ui.test.ts`

- [x] Add failing tests for global project switch on session click and the new/resume action menu flow names.
- [x] Implement selecting a wide session by persisting the target session id, switching project when needed, setting the chat tab, and loading/selecting the session.
- [x] Implement project action popover outside-click dismissal.
- [x] Implement wide new-session agent selection using project-scoped creation and then global project switch.
- [x] Implement wide resume agent selection, in-place resumable session list, import, reload, cache update, global switch, and selected session load.
- [x] Run `npm test -- __tests__/web-chat-ui.test.ts --runTestsByPath`.

### Task 5: Verify

**Files:**
- No new files.

- [x] Run `npm test -- __tests__/web-responsive-ui-state.test.ts __tests__/web-chat-project-scope.test.ts __tests__/web-chat-ui.test.ts --runTestsByPath`.
- [x] Run `npm run tsc:web`.
- [x] Run `npm run build:web`.
- [x] Run a headless wide-screen smoke check against the built web app to confirm `.wide-project-session-nav` renders and `.project-wrap` is absent from the wide header.
- [ ] Commit and push following the repository completion gate, then run `npm run build:web:release`.
