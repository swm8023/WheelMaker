# Registry Debug Panel Iteration Design

Date: 2026-05-19

## Goal

Refine the existing desktop Registry Debug panel so it is easier to inspect long captures across multiple protocol areas and sessions.

This iteration keeps the existing Settings entry, unified `RegistryClient` capture boundary, in-memory retention model, floating desktop panel, virtualized record list, and pretty JSON detail view.

## Decisions

- Keep the debug panel as a floating panel. Do not add a side panel to the main shell or reserve main layout space.
- Add two-level filtering:
  - First level: scope filter derived from Registry method family, such as `session.*`, `fs.*`, `git.*`, and `project.*`.
  - Second level: session filter applied after scope filtering.
- Display friendly session labels when possible:
  - Prefer `ProjectName / SessionTitle`.
  - Fall back to `ProjectName / sessionId`, then `projectId / sessionId`, then raw `sessionId`.
- Make the left record list pane width resizable.
- Allow the right JSON detail pane to collapse.
- Do not auto-scroll the list to new records.
- Keep manual `Jump to latest`.
- Do not replace pretty JSON with a tree viewer.

## Approach

Use the existing panel and debug store as the base.

Rejected alternatives:

- Rebuilding the panel into a main-shell sidebar: rejected because the user explicitly requested no main UI change.
- Making scope and session a single combined filter: rejected because the requested mental model is two-level filtering, with protocol scope first and session second.
- Auto-expanding the right pane when a row is clicked while collapsed: rejected because collapsed mode should preserve list-focused inspection until the user explicitly reopens detail.

## Scope Filtering

Each `RegistryDebugRecord` gets a `scope` string.

For Registry envelopes with a method:

- `session.read`, `session.send`, and related methods map to `session.*`.
- `fs.list`, `fs.read`, and related methods map to `fs.*`.
- `git.status`, `git.diff`, and related methods map to `git.*`.
- `project.list`, `project.syncCheck`, and related methods map to `project.*`.
- Other dotted methods map to `<prefix>.*`.
- Methods without a dot map to the method name.

For non-envelope records:

- WebSocket lifecycle records map to `lifecycle`.
- Parse errors map to `parse_error`.
- Records without a known method map to `unknown`.

The scope selector includes `All` plus scopes discovered from current records. Filtering order is:

1. Apply selected scope.
2. Build session options from records in that scope.
3. Apply selected session.
4. Apply the existing multi-session include rule.

When the selected scope changes and the selected session is not available in that scope, reset the session filter to `All`.

## Session Labels

The panel receives a session label map from `main.tsx`.

`main.tsx` already has:

- `projects`
- `projectSessionsByProjectId`

Use those to build labels keyed by `sessionId`.

Label resolution:

1. If a session summary is known and its project is known: `ProjectName / SessionTitle`.
2. If the project is known but the title is unavailable: `ProjectName / sessionId`.
3. If only the record project ID is known: `projectId / sessionId`.
4. Otherwise: `sessionId`.

The record list can continue to show raw IDs in compact columns if needed, but the session selector should use friendly labels.

## Panel Layout

The debug panel body remains two panes:

- Left: virtualized record list.
- Right: selected record pretty JSON.

Add a draggable splitter between the list pane and the detail pane. The splitter changes list pane width inside the current panel frame. The width is local UI state and is not persisted.

Add a collapse/expand control for the right pane:

- Expanded: show list pane, splitter, and detail pane.
- Collapsed: hide detail pane and splitter; list pane fills the panel body.
- Clicking a record while collapsed updates selected record state but does not reopen the right pane.
- The user reopens detail with the explicit expand control.

Panel drag and outer resize behavior stay unchanged.

## Scrolling And Selection

Remove automatic latest-record following.

Behavior:

- New records append to the virtualized list without forcing scroll position.
- Keep the currently selected record when it remains in the filtered list.
- If the selected record disappears because of filtering or clearing, select the newest record in the filtered list, or clear selection if empty.
- Keep `Jump to latest` visible when records exist, independent of at-bottom tracking if that is simpler and clearer.

This means long debug sessions stay inspectable while new traffic arrives.

## Testing

Update tests for debug helpers:

- Records get the expected `scope`.
- Scope filtering works independently of session filtering.
- Session options are derived after scope filtering.
- Existing multi-session filtering behavior still applies.

Update UI/source tests:

- Panel exposes a scope selector before the session selector.
- Session selector renders friendly labels from a label map.
- List pane has a resizable splitter.
- Detail pane has collapse and expand controls.
- `followOutput` is removed from the `Virtuoso` list.
- `Jump to latest` remains available.

Manual verification:

- Enable Debug, perform session, file, git, and project actions.
- Confirm the scope selector groups traffic by method family.
- Select `session.*`, then a friendly session label, and confirm only that session's records remain.
- Drag the list/detail splitter and confirm list width changes without moving the whole panel.
- Collapse detail, click records, and confirm detail stays collapsed until explicitly expanded.
- Confirm new records do not auto-scroll the list.

## Out Of Scope

- Persisting pane widths or collapse state across page refresh.
- Adding search.
- Adding export.
- Adding a tree JSON viewer.
- Changing Registry protocol or server behavior.
