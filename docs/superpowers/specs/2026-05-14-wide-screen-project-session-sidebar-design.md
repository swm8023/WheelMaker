# Wide Screen Project Session Sidebar Design

## Goal

Replace the wide-screen project dropdown and tab-specific sidebar with a persistent project/session navigation rail. The behavior is tied to the existing wide layout breakpoint, not to desktop hardware. Narrow layouts keep their current drawer and sidebar behavior.

## Scope

This first iteration changes only the wide-screen left navigation and the project/session actions that live inside it. It does not redesign the chat, file, or git main panes, and it does not add a breadcrumb or project label to the header.

## Decisions

- The wide sidebar lists all registry projects in one scrollable rail.
- Each project row can expand or collapse by clicking its title area.
- Project collapsed state is persisted as wide-screen UI state and survives wide/narrow transitions.
- New projects default to expanded.
- The active project can be collapsed; the project row keeps an active visual indicator while the current session remains in the main pane.
- Each expanded project shows the five most recent sessions by default.
- Projects with more than five sessions show a `Show more` control that reveals more sessions for that project in the current runtime.
- Clicking a session under any project globally switches the active project and selects that session.
- Project rows display a hub tag next to the project name.
- Project actions are two icon buttons at the right edge: new session and resume session.
- Action buttons are visible but subdued by default, then become prominent on hover or when the project is active.
- The new-session popover first lists available agents.
- The resume popover first lists available agents, then switches in place to resumable sessions for the selected agent.
- Selecting a resumable session imports it, switches to that project, selects the imported session, and loads its history.
- The wide header no longer renders the project picker. Breadcrumb/main-pane context is intentionally left for a later layout iteration.

## Data Model

The shared app state keeps using one responsive UI state object. A new `desktop.collapsedProjectIds` list stores the wide-screen project collapse state. Persistence writes the list to global workspace state so it is restored after reload and preserved while the app is in a narrow layout.

The current project session list remains the source of truth for the active project. The wide sidebar adds a `projectSessionsByProjectId` runtime map so non-active projects can display cached sessions immediately and refresh from the registry without switching the selected project.

## Service Boundary

`RegistryWorkspaceService` gains project-scoped chat helpers that delegate to existing repository methods:

- list sessions for a specific project
- create a session in a specific project
- list resumable sessions for a specific project and agent
- import a resumed session into a specific project
- reload a session in a specific project

Existing current-project methods remain for the narrow and current chat flows.

## Interaction Details

Clicking a session:

1. Persist the selected session for that session's project.
2. Switch the global active project if needed.
3. Switch to the chat tab.
4. Select and load the session.

Creating a session from a project row:

1. Open an anchored menu listing agents.
2. Create the session in the target project.
3. Cache the returned session.
4. Switch globally to the target project and select the new session.

Resuming a session from a project row:

1. Open an anchored menu listing agents.
2. Load resumable sessions for the selected agent and target project.
3. Replace the agent menu with a session list in the same popover.
4. Import the selected session.
5. Switch globally to the target project and select the imported session.

## Visual Direction

The rail should be dense, calm, and work-focused. It should follow the reference image structurally but with clearer hierarchy: folder/chevron icon, strong project title, colored hub pill, subdued action icons, compact session rows, right-aligned relative time, and a stronger active row. Cards inside cards and marketing-style layout are out of scope.

## Testing

Tests cover:

- persisted wide collapsed project ids in `workspaceUiState`
- project-scoped chat methods in `RegistryWorkspaceService`
- wide sidebar structural expectations in `main.tsx`
- CSS hooks for project groups, hub tags, subdued project actions, anchored popover, and compact session rows
- type checking and production web build
