# Workspace Project Lightweight Sync Design

Date: 2026-05-18
Status: Draft

## Goal

Make File and Git clearly follow a workspace project while keeping Chat scoped by the selected chat session.

Selecting a Chat session should make File/Git target that session's project without paying the current heavy `switchProject()` cost. PC File and Git also need an explicit project selector because they currently have no obvious project choice affordance.

## Problems

- Chat now has project-scoped session selection, but File and Git still depend on the global workspace `projectId`.
- The current `switchProject()` path is too heavy for Chat session selection because it selects the project through the service, loads root files, hydrates File/Git state, and resets visible surfaces.
- PC File and Git do not expose a clear project selector, so users cannot see or change the File/Git workspace project from those tabs.
- Showing stale File/Git content after the workspace project changes is misleading.

## Non-Goals

- Do not make File and Git independently selectable projects.
- Do not make File/Git selection change the selected Chat session.
- Do not refactor every File/Git repository call to accept explicit `projectId` in this slice.
- Do not add a mobile-specific File/Git project selector in this slice.
- Do not remove the old heavy project-loading path if another internal flow still needs it.

## Terms

- **Chat selection**: the selected `ChatSessionKey`, `{projectId, sessionId}`.
- **Workspace project**: the global `projectId` used by File and Git.
- **Lightweight project sync**: update the workspace project and service target without loading root files or Git data unless the affected tab is currently visible.

## Architecture

Keep two project axes:

- Chat owns `selectedChatKey`.
- File/Git own the workspace project, backed by global `selectedProjectId`.

Add a lightweight service method:

```ts
selectProjectLightweight(projectId: string): WorkspaceSession
```

Rules:

- It requires an initialized repository session.
- It verifies the target `projectId` exists in `session.projects`.
- It only updates `session.selectedProjectId`.
- It does not call `repository.listFiles(projectId, '.')`.

Add a controller/main UI path that all user project selection entry points can share, for example `syncWorkspaceProject(projectId, reason)`.

The existing heavy `switchProject()` path stops being the default user-selection route. It may remain for connect or explicit refresh flows that require a fresh root file list.

## Interaction

PC File/Git gain a shared workspace selector above the existing section title:

```text
WORKSPACE  WheelMaker v
EXPLORER
```

```text
WORKSPACE  WheelMaker v
GRAPH      12 items
```

Rules:

- File and Git share the same workspace project.
- The selector appears in PC File/Git, not Chat.
- The selector list contains the current `projects` list only.
- The list reuses the existing pinned/project ordering.
- The selected item is the current workspace project.
- Selecting a project from File/Git does not change `selectedChatKey`.
- Selecting a Chat session syncs the workspace project to the session's `projectId`.
- Tab changes alone do not sync projects.
- If the workspace selector is open and Chat selection changes the workspace project, close the selector.

Last explicit action wins for the workspace project:

- Chat session click pushes File/Git to that session project.
- File/Git selector pushes File/Git to the selected workspace project.
- File/Git selector never pushes Chat to a different session.

## Loading Policy

Lightweight project sync always updates visible identity immediately and avoids stale content.

After a successful workspace project change:

- Persist the previous project snapshot.
- Update the service selected project.
- Persist global `selectedProjectId`.
- Apply the target project's cached File/Git state.
- If no cache exists, show empty/loading-ready File/Git state.
- Do not show the previous project's file tree, selected file, commits, selected diff, or diff text.

Network loading depends on the current tab:

- Current tab is File: load the root file tree for the selected workspace project.
- Current tab is Git: load Git data for the selected workspace project.
- Current tab is Chat: do not load File or Git data.

Chat session selection should not wait for File/Git network work. If the current tab is File/Git, the visible tab can load after the workspace project sync, but Chat session loading should not be blocked by File/Git refresh.

## Persistence

Keep persistence split:

- Workspace project persists through global `selectedProjectId`.
- Chat selection persists through global `selectedChatProjectId` and `selectedChatSessionId`.

File/Git selector updates `selectedProjectId` only.

Chat session selection updates `selectedChatProjectId` and `selectedChatSessionId`, then also syncs `selectedProjectId` when the chat project exists in the current project list.

Cold start restores both independently:

- File/Git starts from `selectedProjectId`.
- Chat starts from `selectedChatProjectId` / `selectedChatSessionId`.

They may differ until the next explicit Chat session selection or File/Git selector selection.

## Error Handling

Invalid target project:

- If a manually selected target is not in current `projects` or service session projects, reject the sync.
- Show `Project is no longer available`.
- Keep the previous workspace project, service target, and visible File/Git state.

Chat selection with an unknown project:

- Keep the Chat selection behavior.
- Skip workspace project sync.
- Keep the previous File/Git workspace project.

Visible tab loading failure:

- Keep the workspace project switched.
- Show the File or Git loading error in that tab.
- Do not roll back to the previous project.

## Testing

Add focused tests instead of relying only on structure checks.

Service tests:

- `selectProjectLightweight(projectId)` updates `session.selectedProjectId`.
- It does not call `listFiles`.
- It rejects unknown project IDs.

Controller/store tests:

- Lightweight sync persists `selectedProjectId`.
- Lightweight sync does not change `selectedChatProjectId` or `selectedChatSessionId`.
- Cached File/Git state can hydrate after a lightweight sync.

UI wiring tests:

- `selectProjectChatSession` uses lightweight workspace sync and does not call `switchProject()`.
- PC File/Git selector calls the lightweight workspace sync entry point.
- Existing project menu also calls the lightweight workspace sync entry point.
- Current File tab triggers root file load after sync.
- Current Git tab triggers Git load after sync.
- Current Chat tab does not trigger File/Git network loading after workspace sync.

Regression cases:

- Selecting a Chat session in project B makes File/Git selector show B.
- Selecting project C from File/Git leaves Chat showing its previous session.
- Selecting a Chat session in project A after that makes File/Git show A.
- Switching to an uncached project does not leave the previous project's file tree or diff visible.
- Reload restores File/Git and Chat selections independently.
