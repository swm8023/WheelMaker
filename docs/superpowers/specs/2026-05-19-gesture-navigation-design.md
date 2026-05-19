# Gesture Navigation Design

Date: 2026-05-19

## Goal

Add an optional mobile navigation mode that coexists with the current floating mobile controls. The current mode keeps the visible Chat, File, Git, and drawer buttons. The new mode keeps only the drawer button visible, shows the current tab as a small badge, and uses a long-press gesture on the drawer button to select Chat, File, or Git.

The goal is to reduce mobile visual clutter without changing desktop navigation or the drawer's normal short-tap behavior.

## Scope

This feature applies only when the responsive layout is in narrow/mobile mode. The setting is still available and persisted globally, like other Appearance settings, because desktop windows can be resized into narrow mode.

In scope:

- Add an Appearance setting named `Gesture Navigation`.
- Persist the setting in `workspacePersistence.global`.
- Keep the existing mobile floating control mode when the setting is off.
- Render the new single-button gesture navigation mode when the setting is on and the layout is narrow.
- Reuse the existing tab selection semantics through the current floating navigation handler.
- Reuse the existing floating control slot positioning and persistence.

Out of scope:

- Changing desktop activity bar behavior.
- Replacing the drawer content or drawer open/close semantics.
- Adding text labels to the gesture buttons.
- Moving the setting into a separate mobile-only settings page.
- Reworking the broader responsive shell architecture.

## Existing System

The current mobile navigation is rendered in `main.tsx` as `floatingControlStack` under `!isWide`.

It contains:

- A `floating-nav-group` with three buttons: Chat, File, Git.
- A `drawer-toggle-bubble` button that opens or closes the drawer.
- Drag handling attached to the stack and buttons.
- A persisted `floatingControlSlot` in `workspacePersistence.global`.

The desktop layout uses `desktop-activity-bar`, which is independent and should remain unchanged.

## Setting

Add a boolean global preference:

- Field: `gestureNavigation`
- Label: `Gesture Navigation`
- Default: `false`
- Section: `Appearance`
- Persistence: `workspacePersistence.global`, same IndexedDB/local workspace persistence path as `themeMode`, `hideToolCalls`, and other UI preferences.

Behavior:

- When `false`, render the current mobile floating controls unchanged.
- When `true` and `!isWide`, render the gesture navigation control.
- When `true` and `isWide`, keep the normal desktop activity bar.

## Visual Design

### Resting State

Only the drawer button is visible.

The drawer button keeps its current semantic role:

- Short tap toggles the drawer.
- `aria-expanded` continues to reflect drawer state.
- Icon remains `codicon-menu`.

A small current-tab badge appears at the drawer button's lower-right corner:

- Size: approximately 16px.
- Shape: circular.
- Background: accent blue.
- Icon color: white.
- Chat icon: `codicon-comment-discussion`.
- File icon: `codicon-files`.
- Git icon: `codicon-source-control`.

The badge represents the current main tab, not settings state. If settings is open, the badge still reflects the current Chat/File/Git tab.

### Expanded State

Long-pressing the drawer button opens a four-button radial layout:

- Center: drawer, `codicon-menu`.
- Up: Chat, `codicon-comment-discussion`.
- Left: File, `codicon-files`.
- Down: Git, `codicon-source-control`.

All four buttons are the same circular size.

The badge is hidden while expanded.

The center drawer button does not get an extra active background just because the drawer is open. Direction buttons use background states:

- Current tab has a background.
- Current gesture candidate has a background.
- If the current candidate is also the current tab, keep one stable highlighted background.

If the floating button is near a screen edge, the expanded group should remain within the viewport. The semantic directions stay fixed: Chat is up, File is left, Git is down.

## Interaction Model

### Short Tap

Short tap on the drawer button keeps the existing drawer behavior:

- Closed drawer becomes open.
- Open drawer becomes closed.
- No tab switch occurs.

### Long Press

Long press duration: `350ms`.

The gesture starts only from the drawer button in Gesture Navigation mode.

Movement thresholds before long-press activation:

- Less than `12px`: still eligible for long press.
- Between `12px` and `28px`: neutral zone. Do not trigger long press and do not immediately enter drag mode.
- Greater than `28px` before `350ms`: enter the existing floating control drag mode.

Once expanded, pointer movement no longer drags the floating control. It only selects a navigation candidate.

### Candidate Selection

Candidate selection should combine hit areas and direction fallback:

- If the pointer enters a target button hit area, that target becomes the candidate.
- If the pointer does not enter a button but movement from the press origin exceeds `42px`, choose the nearest primary direction.
- Directions are fixed:
  - Up: Chat
  - Left: File
  - Down: Git

On pointer release:

- If a candidate exists, call the same tab-selection path used by existing floating nav buttons.
- If no candidate exists, cancel with no action.

Selecting a different tab should close the drawer, matching the existing `handleFloatingNavSelect` behavior. Selecting the current tab should also follow existing behavior.

### Mouse And Accessibility

The feature is touch-first, but narrow desktop windows can still use it.

Support these paths:

- Long-press then drag/release on a direction.
- Long-press then click a visible direction button.
- Escape cancels expanded state.
- Clicking outside cancels expanded state.

Buttons must have `aria-label` and `title` values. The center drawer button keeps `aria-expanded`.

## Drag Positioning

Gesture Navigation keeps the existing floating control slot model.

When Gesture Navigation is on:

- Resting control uses the existing `floatingControlSlot`.
- Dragging the control position remains possible only when movement exceeds `28px` before long-press activation.
- Once long-press expansion starts, the gesture cannot become a position drag.

When Gesture Navigation is off:

- Existing floating control drag behavior remains unchanged.

## State Model

The implementation should keep two concepts separate:

- Persisted preference: `gestureNavigation`.
- Transient gesture state: expanded/candidate/drag start data.

The transient state should not be persisted.

Recommended transient states:

- `idle`
- `pressing`
- `expanded`
- `dragging`
- `cooldown`

The existing floating drag state can still back the old mode and the large-move drag path. The expanded gesture selection state should be explicit so it does not overload the existing drag-active meaning.

## Implementation Shape

Preferred implementation: extend the existing mobile floating controls in `main.tsx` with a mode branch.

Reason:

- Current tab, drawer, drag, and persistence handlers already live in `main.tsx`.
- A full component extraction would require many props and create a larger refactor.
- The first implementation should keep behavior changes local to the existing mobile floating control area.

Expected files:

- `app/web/src/main.tsx`
- `app/web/src/styles.css`
- `app/web/src/services/workspacePersistence.ts`
- Existing or new tests under `app/__tests__`

Potential later refactor:

- Extract `MobileFloatingNavigation` once both old and gesture modes are stable.

## Test Plan

Add source-level tests for:

- `Gesture Navigation` appears in the Appearance section.
- The setting persists through `workspacePersistence.global`.
- Default value is `false`.
- Desktop activity bar remains independent of the setting.
- Narrow/mobile `floatingControlStack` has a branch for old controls and gesture controls.
- Gesture mode renders only the drawer resting button plus current-tab badge.
- Expanded gesture mode includes Chat/File/Git directional buttons with correct icons and labels.
- Existing old-mode floating nav button markup remains available when the setting is off.
- Existing drawer short-tap path remains wired to drawer toggle behavior.

Add focused logic tests if the gesture selection is extracted into a helper:

- Movement under `12px` remains long-press eligible.
- Movement over `28px` before activation starts drag mode.
- No candidate on release cancels.
- Up selects Chat, left selects File, down selects Git.

Manual verification after implementation:

- Mobile/narrow: old mode works unchanged when setting is off.
- Mobile/narrow: resting gesture mode shows one drawer button and the current-tab badge.
- Mobile/narrow: short tap toggles drawer.
- Mobile/narrow: long press opens radial controls.
- Mobile/narrow: release without candidate cancels.
- Mobile/narrow: drag to each direction selects the correct tab.
- Mobile/narrow: large early movement drags the floating control.
- Wide/desktop: activity bar remains unchanged.

## Open Decisions

None. The interaction and persistence behavior are decided.
