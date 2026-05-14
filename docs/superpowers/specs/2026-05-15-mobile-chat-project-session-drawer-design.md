# Mobile Chat Project Session Drawer Design

## Goal

Replace the mobile Chat drawer's current single-project session flow with a cross-project session index, matching the wide-screen project/session navigation model while preserving mobile drawer behavior.

## Scope

This change applies to the mobile Chat drawer only. Mobile File and Git keep their project-scoped behavior, but the drawer width and right-side floating control lane behavior are shared across all mobile drawer modes for consistency.

The first version does not keep the old mobile chat session swipe actions for Reload and Delete. Session row actions will be redesigned later with project scope.

## Decisions

1. The mobile Chat drawer shows all projects and their sessions directly. Users no longer choose a project before choosing a chat session.
2. Clicking a session automatically switches to that session's project, selects the session, closes the drawer, and reads only that session's detail.
3. The list phase reads only session lists through project-scoped list calls. It does not read session details or turns.
4. The drawer opens from cache immediately, then refreshes the project list and every project's session list in the background.
5. Refresh updates both projects and session lists. Project refresh failure is global; a single project's session-list failure keeps its old list and displays a light project-level error/retry affordance.
6. Project order, session sorting, show-more behavior, empty project behavior, hub tags, and session agent tags reuse the wide project/session rail behavior.
7. Project title click toggles collapse/expand. The collapse state is shared across wide and mobile layouts.
8. Project row `New` and `Resume` actions move into the project row. There are no global New/Resume buttons in the mobile Chat drawer.
9. Mobile project actions use inline expansion under the project row, not floating popovers. Only one mobile action expansion can be open at a time.
10. Collapsing a project closes any open action area for that project.
11. New session and Resume import both close the drawer and enter the resulting session after success. Failure leaves the drawer open and does not change the current main chat.
12. The mobile Chat drawer hides the project selector header. It uses a Chat-specific header with settings on the left, `Chats` in the middle, and refresh on the right.
13. Mobile File/Git drawer headers keep the settings button, project selector, and refresh button.
14. Token Stats moves out of Chat and into Settings as a settings detail page.
15. Agent Info is deprecated for now. It is removed from Chat and is not added to Settings.
16. Settings detail pages use a single state such as `settingsDetailView`.
17. Drawer width is shared across mobile Chat/File/Git and leaves the right-side floating tab lane clickable. The width should be close to `min(420px, calc(100vw - 72px - env(safe-area-inset-right, 0px)))`, adjusted to match the actual floating lane.
18. The drawer overlay does not cover the right-side floating control lane. Tapping outside the drawer but outside the lane closes the drawer; tapping the 3-tab control remains interactive.
19. With the drawer open, the 3-tab control remains clickable. Tapping File/Git switches tab and closes the drawer. Tapping the active Chat control toggles the drawer closed.
20. After selecting a cross-project chat session, File and Git keep using that newly active project. Chat selection is not a temporary project context.
21. If refreshed projects no longer include the current project, existing fallback project hydration applies, and selected chat state for the missing project is cleared.

## Architecture

Extract the shared project/session navigation behavior from the current wide rail into a reusable project-session navigation unit. Wide and mobile renderers should share project/session data, collapse state, project-scoped actions, sorting, show-more logic, tags, and selection behavior, while using different presentation for action menus.

The mobile renderer becomes a `MobileChatSessionSheet` used only when the drawer is open on the Chat tab. It renders the shared project/session model with touch-friendly spacing, inline action expansion, and a Chat-specific drawer header.

The wide renderer continues to use a sidebar rail and popover action menu. Its behavior should remain visually stable except for reading shared state from the renamed shared collapse state.

## Data Flow

1. Opening the mobile Chat drawer hydrates project session lists from `workspaceStore` for every known project.
2. It starts a background refresh:
   - refresh project list
   - for each project, call `listProjectSessions(projectId)`
   - store sorted session lists in `projectSessionsByProjectId`
   - replace cached session indexes without reading session details
3. Selecting a session calls the existing project-scoped selection path:
   - remember selected chat session for the target project
   - switch project if needed
   - set Chat tab
   - close mobile drawer
   - hydrate cached content for the selected session
   - load that session incrementally
4. Creating or resuming a project session uses project-scoped service methods and then enters the resulting session through the same selection path.

## State Changes

The existing `desktop.collapsedProjectIds` state should become shared state, because project collapse now applies to both wide and mobile project/session indexes.

Mobile-specific state should track:

- one open inline project action area
- the active action kind: `new` or `resume`
- the action phase: agent list or resumable session list
- per-project lightweight session-list refresh errors
- settings detail view, starting with `tokenStats`

The old Chat header panel states for Token Stats and Agent Info should stop being driven from Chat.

## UI Requirements

Mobile Chat drawer:

- no project selector
- settings button remains on the left
- title is `Chats`
- refresh button refreshes projects and all session lists
- projects render as collapsible rows with folder state and hub tag
- project row has New and Resume buttons
- session row shows title, optional agent tag, and compact relative time
- empty project displays the same lightweight empty state as wide
- no swipe Reload/Delete in the first version

Mobile File/Git drawer:

- same drawer width as Chat
- existing project selector header remains
- right-side floating tab control remains clickable while drawer is open

Settings:

- add Token Stats as a settings detail page
- remove Agent Info as a reachable feature for now

## Testing

Add tests that lock:

- mobile Chat drawer does not render the project selector
- mobile File/Git drawer still renders the project selector
- mobile Chat drawer uses all-project project/session navigation
- project collapse state is shared with wide navigation
- opening mobile Chat drawer hydrates cached lists and refreshes only list data
- selecting a cross-project session switches project and loads only the selected session detail
- project New/Resume actions are project-scoped and close the drawer on success
- global New/Resume, Token Stats, and Agent Info no longer appear in the Chat drawer header
- Token Stats is reachable from Settings as a detail page
- Agent Info is not reachable from Settings
- drawer width leaves the right-side floating control lane interactive
- session swipe Reload/Delete is absent from the first mobile cross-project list

## Deferred Todo

Redesign per-session actions for the shared project/session list. Reload and Delete should return only after the interaction model handles project scope explicitly and works consistently on wide and mobile.
