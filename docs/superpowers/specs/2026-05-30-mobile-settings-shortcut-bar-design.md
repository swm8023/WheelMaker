# Mobile Settings Shortcut Bar Design

Date: 2026-05-30
Status: Approved

## Goal

Make mobile Settings expose the same high-frequency settings shortcuts that desktop already exposes in the left activity bar.

The mobile shortcut surface should live only inside the mobile Settings fullscreen screen. It should not become a global mobile navigation bar and should not compete with Chat input, floating navigation, or the Port Relay fullscreen layer.

## Context

Desktop has a `desktop-activity-bar` with workspace navigation and settings shortcuts. The relevant settings shortcuts are:

- `Update`
- `Skills`
- `Token Stats`
- `Settings`

Desktop also exposes `Port Relay` as a primary activity-bar item below `Git`. Mobile currently has no equivalent Settings-local shortcut bar. Those entries live in the Settings `More` section as rows, which makes them feel nested instead of same-level.

Mobile Chat currently also has compact toolbar shortcuts for `Update` and `Port Relay` beside the Settings button. After adding the Settings-local bottom bar, those Chat-header shortcuts become redundant.

## Confirmed Design

Add a Settings-only bottom activity bar on mobile, matching the desktop activity bar visual language:

- flat icon buttons
- visible labels below icons
- same muted icon tone
- active state using a top accent indicator and active foreground
- background aligned with the activity bar surface
- safe-area padding at the bottom

The bar appears whenever `!isWide && sidebarSettingsOpen`.

The bar remains visible on:

- the Settings root list
- Settings detail pages

The bar does not appear outside the mobile Settings screen.

## Mobile Bar Entries

The mobile Settings bottom bar entries are:

1. `Settings`
2. `Update`
3. `Skills`
4. `Port Relay`
5. `Token Stats`
6. `CC Switch`

Behavior:

- `Update`, `Skills`, `Token Stats`, `CC Switch`, and `Port Relay` open their existing settings detail pages.
- `Settings` clears `settingsDetailView` and returns to the Settings root list.
- Active state follows the current `settingsDetailView`.
- The `Settings` button is active when Settings is open and no shortcut detail is active.

`Database` is intentionally not in the bottom bar. It is a lower-frequency diagnostics/storage tool, not a primary shortcut.

## Settings Information Architecture

Remove the old shortcut rows from the `More` section:

- `Update`
- `Skills`
- `Token Stats`
- `CC Switch`
- `Port Relay`

Move these rows into the existing `Debug` section:

- `Database`
- `Clear Local Cache`

If `More` has no remaining rows after the shortcut migration, remove the `More` section.

The existing `Debug` section should then contain diagnostics and maintenance actions in this order:

1. `Debug`
2. `Disable File Cache`
3. `Open Debug Panel`
4. `Logs`
5. `Database`
6. `Clear Local Cache`
7. `Logout`

## Layout Details

The mobile Settings screen should reserve enough scroll padding so content is not hidden behind the bottom activity bar.

The bottom bar should use stable dimensions:

- fixed icon-button hit targets similar to desktop
- horizontal layout
- short labels below each icon
- bottom safe-area padding

The detail page top title bar and right-side detail actions remain unchanged. The bottom activity bar is an additional navigation surface, not a replacement for the back button.

## Mobile Chat Toolbar Cleanup

The mobile Chat drawer header should return to a single Settings shortcut on the left side.

Remove these Chat-header shortcuts:

- `Update`
- `Port Relay`

Keep:

- the existing Settings button
- search controls and hub summary

The Settings button styling should return to the same baseline treatment used by other mobile header buttons. It should no longer look like a mini shortcuts group that carries Settings, Update, and Port Relay together.

## Out Of Scope

- Do not add a global mobile bottom navigation bar.
- Do not change desktop activity bar behavior.
- Do not change Settings detail data loading, refresh actions, or error handling.
- Do not change the mobile floating controls.
- Do not add labels to the shortcut bar in this slice.

## Testing Strategy

Use existing Jest source-structure tests.

Tests should lock:

- mobile Settings renders a bottom shortcut bar inside `mobileSettingsScreen`
- the mobile bar order is `Settings`, `Update`, `Skills`, `Port Relay`, `Token Stats`, `CC Switch`
- the mobile bar opens existing detail pages through the same state paths as desktop
- the mobile bar stays separate from `floatingControlStack`
- the mobile Chat toolbar only keeps the Settings shortcut and no longer includes `Update` or `Port Relay`
- `More` no longer contains the migrated shortcut rows
- `Database` and `Clear Local Cache` appear in the `Debug` section
- CSS includes bottom safe-area padding and content bottom padding for the new bar

## Acceptance Criteria

- On mobile Settings root, users can open `Update`, `Skills`, `Port Relay`, `Token Stats`, and `CC Switch` from the bottom bar.
- On mobile Settings detail pages, the bottom bar remains visible and can switch details or return to Settings root.
- The bar is not visible in mobile Chat, File, Git, or Port Relay fullscreen iframe surfaces.
- The mobile Chat drawer header left side only keeps the Settings shortcut.
- `More` no longer duplicates the shortcut entries.
- `Database` and `Clear Local Cache` are available in the `Debug` section.
- Desktop behavior remains unchanged.
