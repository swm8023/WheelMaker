# App Project-Scoped Workspace State Design

## Goal

Unify Chat / Files / Diff under project-scoped data and UI state, so switching tabs or switching wide/narrow layout always uses the same source of truth.

## Scope

- Introduce project-level state model for chat/files/diff + workspace UI state.
- Keep independent state per project and restore exactly when switching back.
- Trigger refresh hooks on project switch, with parallel refresh for chat/files/diff.
- Keep drawer content style matched to current tab (chat list / file tree / diff split list).

## Architecture

- Add `ProjectWorkspaceStore` as a lightweight `ChangeNotifier` single source of truth.
- Use `projectId` as top-level key: `Map<String, ProjectWorkspaceState>`.
- `WorkspaceDebugScreen` becomes a view/controller shell that reads/writes store state.
- Data loading is abstracted by `ProjectDataSource`; current implementation is mock-first with reserved async refresh interface.

## Components

- `ProjectWorkspaceStore`
  - active project id
  - project state map
  - tab/sidebar/ui actions
  - refresh orchestration
- `ProjectWorkspaceState`
  - chat pane: session list + selected index
  - file pane: file root + expanded set + selected file path
  - diff pane: commit list + selected commit/file
  - ui pane: selected tab + sidebar collapsed
- `ProjectDataSource`
  - `fetchChatSessions(projectId)`
  - `fetchFileTree(projectId)`
  - `fetchDiffCommits(projectId)`

## Data Flow

- App enter:
  - initialize store with default project state from data source.
- Tab switch:
  - update active project's `ui.selectedTab` only.
- Project switch:
  - switch pointer to target project's cached state immediately.
  - trigger `refreshProject(projectId)` and refresh chat/files/diff in parallel.
  - merge refreshed data while preserving still-valid selections.
- Wide/narrow switch:
  - layout only; no state migration and no source swap.

## Error Handling

- Per-pane refresh isolation: one pane failure does not block others.
- Keep old cached data if refresh fails.
- If selected item disappears after refresh, fallback to first item or null.

## Testing

- Store behavior tests:
  - project switching does not leak state
  - tab/sidebar state remains consistent across layouts
  - per-project file expanded/selected state is isolated
  - refresh updates data and preserves valid selections
- Widget smoke validation:
  - project dropdown switch restores prior state
  - drawer content always matches current selected tab
