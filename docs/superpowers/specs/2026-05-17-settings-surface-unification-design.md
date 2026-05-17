# Settings Surface Unification Design

## Goal

Unify the desktop and mobile Settings experience around one content model while keeping their shell placement different.

Desktop continues to show Settings in the sidebar. Mobile and other narrow viewports continue to show Settings as a fullscreen overlay. Both shells should render the same settings sections, row ordering, detail routes, and confirmation behavior.

## Context

The current web app already has a shared `renderSettingsContent` entry point, but the Settings content is still a flat list with mixed row types. Mobile wraps that list in a fullscreen screen, while desktop renders it in the sidebar. Existing detail pages include `Token Stats` and `CC Switch`, and `Database` currently expands inline in the main Settings list.

The responsive shell ADR treats desktop and mobile as different shells backed by shared workspace UI state. This design follows that model: shell chrome can differ, but Settings information architecture should not.

## Confirmed Scope

- Runtime target: `app/web/`.
- Keep desktop Settings in the sidebar.
- Keep mobile Settings as a fullscreen overlay.
- Use one Settings content model for desktop and mobile.
- Replace the flat Settings list with ordered sections.
- Keep all setting rows compact, without secondary description text.
- Move `Database` from inline expansion to a Settings detail page.
- Keep `CC Switch` behavior and naming as-is.
- Keep `Token Stats` as a Settings detail page.
- Reuse the existing archive confirmation dialog style for clearing local cache.
- Update tests in the existing source-structure/style assertion style.

## Non-Goals

- Do not redesign the desktop sidebar width model.
- Do not introduce a separate Settings route/page outside the current shells.
- Do not add category landing pages, tabs, or segmented controls.
- Do not add descriptions under every settings row.
- Do not change `CC Switch` into an interactive switching workflow.
- Do not handle or redesign session row action long-press behavior in this slice.
- Do not migrate unrelated shell state.

## Information Architecture

Settings renders as one scrollable list with section headers in this order:

1. Appearance
2. Chat
3. Code Display
4. Storage

### Appearance

- `Dark Mode`

### Chat

- `Hide Tool Calls`
- `CC Switch`
- `Token Stats`

`Hide Tool Calls` is a switch row. `CC Switch` and `Token Stats` are detail rows.

### Code Display

- `Code Theme`
- `Code Font`
- `Font Size`
- `Line Height`
- `Tab Size`

All Code Display controls remain directly visible as select rows. The existing Shiki theme behavior, code font options, validation, and persistence semantics remain unchanged.

### Storage

- `Database`
- `Clear Local Cache`

`Database` is a detail row. `Clear Local Cache` is a danger row that opens a styled confirmation dialog before clearing cache.

## Surface Model

Desktop and mobile use different shells but the same Settings content:

- desktop: Settings content renders inside the existing sidebar scroll area
- mobile: Settings content renders inside the existing fullscreen Settings overlay

Both shells use the same section definitions, row order, detail routes, and detail renderers. CSS may adjust density and padding per shell, but content structure should not fork.

Viewport width remains the layout decision boundary. This design does not add device-orientation-specific logic.

## Row Design

Use a compact grouped-list style:

- section headers are lightweight labels
- setting rows are flat rows with separators
- rows do not include secondary description text
- switches and selects stay aligned within their row
- detail rows use a chevron affordance
- danger rows use a danger treatment but remain inside the Storage section

The desktop version should keep the current sidebar width behavior. Select rows should fit within the available row width using the existing sidebar constraints rather than expanding the shell.

## Detail Pages

Settings detail pages use a shared structure:

- back button
- title
- content area with consistent list/card density
- shared loading, error, and empty-state styling where applicable

The detail routes are:

- `ccSwitch`
- `tokenStats`
- `database`

`CC Switch` remains an information page in this slice. It should no longer reuse `token-stats-*` classes for its general layout. Shared Settings detail or metadata-list classes should carry common styling.

`Token Stats` keeps its existing data loading behavior and refresh action, but uses the shared detail shell.

`Database` moves out of the main Settings list. Opening the Database detail should load and show the local database dump using the existing export/loading/error behavior. Returning from the detail page goes back to the Settings main list.

## Shared Confirm Dialog

The current archive flow uses a custom modal confirmation style. Clearing local cache should reuse that visual system instead of `window.confirm`.

The implementation should introduce a shared confirm dialog renderer and style class set, then keep the existing archive confirmation behavior on that shared shell. Clear Local Cache should use the same shared shell with cache-specific copy, icon, and danger primary action styling.

Clear Local Cache confirmation behavior:

- open a styled confirmation dialog from the Storage danger row
- cancel closes the dialog without side effects
- confirm calls `workspaceStore.clearLocalCachePreservingToken()`
- after successful confirmation, reload the page as today
- the copy should clearly state that token and server address are preserved

## State Behavior

`settingsOpen` is already shared shell state. `settingsDetailView` should behave as the shared Settings route:

- switching between desktop and mobile layout preserves the current detail view
- clicking a detail back button returns to the Settings main list
- closing Settings does not need to clear the detail route
- reopening Settings returns to the last detail unless the user explicitly navigated back

This keeps Settings navigation stable across viewport changes.

## Testing Strategy

Follow the existing test style, which primarily asserts source structure and CSS behavior.

Update or add tests that lock:

- section order is `Appearance`, `Chat`, `Code Display`, `Storage`
- Chat row order is `Hide Tool Calls`, `CC Switch`, `Token Stats`
- Code Display keeps all five direct select controls
- Storage includes `Database` and `Clear Local Cache`
- `Database` is a detail route, not an inline main-list expansion
- `settingsDetailView` supports `database`
- `CC Switch`, `Token Stats`, and `Database` use a shared detail shell
- `CC Switch` does not use `token-stats-*` classes for its general page layout
- Clear Local Cache no longer uses `window.confirm`
- Clear Local Cache uses the shared confirm dialog styling derived from the archive confirm dialog
- desktop and mobile Settings use the same content renderer

Existing tests for Shiki settings, hide tool calls, and clear local cache should be updated to match the new structure without weakening their behavioral assertions.

## Acceptance Criteria

- Desktop and mobile Settings show the same sections and row order.
- Desktop Settings remains in the sidebar.
- Mobile Settings remains a fullscreen overlay.
- Settings rows are compact and have no secondary descriptions.
- `Database` opens as a detail page.
- `CC Switch` and `Token Stats` remain detail pages.
- All detail pages share the same back/title structure.
- Clear Local Cache uses the styled confirmation dialog, not `window.confirm`.
- Clear Local Cache still preserves token and address and reloads after confirmation.
- Existing code display settings remain directly configurable.
- Layout changes across the desktop/mobile breakpoint preserve the current Settings detail route.
