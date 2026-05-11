# App Narrow Floating Navigation Design

Date: 2026-05-11
Status: Approved for implementation

## Background

The current web narrow-screen layout still uses the same top header row and horizontal tab bar shape as the wide layout, only compressed through media-query adjustments. That keeps the implementation simple, but the narrow-screen experience is now constrained by a full-width top bar that competes with content and does not match the user's preferred one-handed navigation model.

The user wants a larger narrow-screen redesign for the web shell. On narrow screens, the top bar should be replaced by a compact floating title surface, while primary navigation and drawer access should move into a right-side floating control layer that is draggable as one unit. The redesign must preserve the current project switch behavior, current refresh behavior, and current chat keyboard avoidance logic, while making the main content area feel substantially more full-screen.

## Goals

1. Replace the narrow-screen full-width top header with a fixed top-left **Header Bubble**.
2. Replace the narrow-screen horizontal tab row with a right-side **Floating Control Stack** made of:
   - a vertical **Primary Navigation Bubble Group**
   - a single **Drawer Toggle Bubble**
3. Make the **Floating Control Stack** draggable as one vertical unit on the right edge.
4. Persist the user's resting position locally across sessions.
5. Snap the floating stack to a small set of predefined vertical resting slots instead of leaving it at arbitrary positions.
6. Preserve current project switching and refresh behavior inside the new **Header Bubble**.
7. Preserve the narrow-screen “initial top spacing, then overlap while scrolling” effect for content under the header.
8. Preserve current chat keyboard avoidance while adding temporary avoidance for the floating stack when the keyboard is visible.
9. Keep the redesign scoped to the web narrow-screen shell only.

## Non-goals

1. No Flutter native shell changes.
2. No wide-screen shell redesign.
3. No bottom status bar work in this iteration.
4. No redesign of existing file-side floating tools in this iteration.
5. No new first-load entrance animation or decorative looping animation.
6. No server or protocol changes.

## Current State

Current narrow-screen web layout behavior is still derived from the wide shell:

- `app/web/src/main.tsx` renders a single `<header className="header">` for both wide and narrow screens.
- The project switch control (`project-wrap`) and refresh button live in that header.
- The Chat / File / Git tabs remain in the header as a horizontal `.tabs` row.
- `app/web/src/styles.css` narrows the layout under `@media (max-width: 900px)` by shrinking tab buttons and hiding tab labels, but the top row still occupies full width.
- Chat already has a narrow-screen keyboard inset mechanism driven by `window.visualViewport`, stored as `chatKeyboardInset`, and applied as bottom padding to `.chat-main`.

This means the current narrow-screen shell is a compressed version of the desktop shell rather than a dedicated mobile-first shell.

## Chosen Approach

Introduce a distinct narrow-screen shell composed of a fixed **Header Bubble**, a draggable **Floating Control Stack**, and content-specific top inset rules, while leaving the wide shell intact.

This is preferred over incremental header shrinking because:

1. the old full-width header would continue to consume scarce vertical space
2. the current horizontal tab row does not support the desired one-handed right-side interaction model
3. the drag/snap/persist behavior is easier to reason about as a dedicated narrow-shell layer
4. the new design can stay localized to web narrow-screen conditions without disturbing the wide layout

## UX Design

### 1. Narrow-screen shell structure

When `windowWidth < 900`, the web shell switches from the current full-width header layout to a narrow-shell structure with three key surfaces:

1. **Header Bubble**
2. **Floating Control Stack**
3. main content area with an initial top inset

The current narrow-screen full-width `header + tabs` row exits the layout completely. This is not a visual hide; the narrow shell uses different structure and spacing rules.

Wide screens continue to use the existing header and tab row with no product change.

### 2. Header Bubble

The **Header Bubble** is the top-left anchored floating title surface for narrow screens.

Rules:

1. It is fixed in the top-left corner and is not draggable.
2. It remains visible at all times.
3. It stays visually above the **Floating Control Stack**.
4. It keeps the current project-switch behavior.
5. It keeps the current refresh action.
6. Its dropdown / project menu behavior remains unchanged from the current implementation.

Content model:

- left/main area: project switch affordance and project name
- right area: refresh button

Width behavior:

1. The bubble width expands naturally until a viewport-based maximum.
2. The project-name area is the compressible region.
3. The refresh button never compresses below its stable hit target.
4. The project-name area truncates in one line once the width cap is reached.

Visual model:

- a floating horizontal pill, not a residual flat top bar
- VSCode-like dark panel language
- slightly thicker than a chip/tag
- same visual family as the right-side floating controls

### 3. Initial content inset and scroll overlap

On narrow screens, the main content area must reserve top space for the **Header Bubble** only at the initial resting position.

Behavior:

1. When the page is first shown, top content should not begin underneath the **Header Bubble**.
2. As the user scrolls upward, content is allowed to pass underneath and be visually covered by the **Header Bubble**.
3. The bubble itself does not hide, collapse, or animate away during scroll.

This preserves the desired “safe first row, then intentional overlap” effect without a stateful hide/reveal header.

### 4. Floating Control Stack

The **Floating Control Stack** is the right-side, vertically arranged floating control layer for narrow screens.

Composition:

1. **Primary Navigation Bubble Group**
2. **Drawer Toggle Bubble**

Structure:

- the group and the drawer bubble are visually matched
- the navigation group is a single vertical pill containing three tab icons
- the drawer control is a separate single bubble beneath it
- both belong to one draggable stack

The stack is:

- fixed to the right edge
- vertically draggable only
- persisted locally
- snapped to predefined vertical slots on release

### 5. Primary Navigation Bubble Group

The **Primary Navigation Bubble Group** is the narrow-screen primary navigation surface.

Rules:

1. It contains only icons for Chat / File / Git.
2. No visible text labels are shown inside the narrow-screen control.
3. `title` and `aria-label` continue to provide the full semantic label for each tab.
4. The active state uses a fixed outer track plus an internal highlighted selection block that moves between the three positions.
5. Icons shift to a higher-contrast active treatment when selected.

This should feel like one cohesive navigation capsule, not three unrelated stacked buttons.

### 6. Drawer Toggle Bubble

The **Drawer Toggle Bubble** keeps the same visual family as the navigation group while remaining a single standalone bubble below it.

Rules:

1. It opens and closes the existing narrow-screen drawer behavior.
2. It is not merged into the 3-tab pill.
3. It shares the same color system, shadow system, and active/pressed treatment as the navigation group.

### 7. Floating stack resting position

Default resting position:

- right side
- vertically around the upper-middle zone
- centered roughly around 40% of the available safe viewport height

The stack does not support horizontal dragging. It always remains aligned to the right edge.

### 8. Dragging model

The **Floating Control Stack** is the only draggable floating UI in this redesign.

The **Header Bubble** is explicitly not draggable.

Drag behavior:

1. A short tap on a tab switches tabs immediately.
2. A short tap on the drawer bubble toggles the drawer immediately.
3. A long press of approximately 350ms on the stack enters drag mode.
4. Before the long press is recognized, pointer movement beyond a small threshold cancels the long press and falls back to normal click behavior.
5. Once drag mode is active, that gesture no longer triggers tab switching or drawer toggle behavior.
6. After drop, a short click-cooldown window prevents accidental activation on release.

Feedback:

1. pre-long-press press state: slight scale-down
2. drag activation: scale-up, stronger shadow, stronger highlight
3. drag move: direct follow on the vertical axis
4. release: animated snap to the resting slot

### 9. Edge Snap

The stack already stays attached to the right edge, so **Edge Snap** is defined as vertical slot snapping, not horizontal snapping.

The first implementation should use four vertical resting slots:

1. upper
2. upper-middle
3. center
4. lower-middle

The default slot is **upper-middle**.

Drop behavior:

1. Drag release resolves to the nearest valid slot.
2. The slot index is what gets persisted, not an arbitrary pixel y-position.
3. If the stored slot becomes invalid due to viewport changes, the stack falls back to the nearest safe slot or the default slot.

### 10. Keyboard behavior

Current chat narrow-screen keyboard avoidance should remain in place.

Additional rule:

1. When the chat keyboard is visible, the **Floating Control Stack** may temporarily shift upward to stay inside the visible safe region.
2. This temporary keyboard-driven offset must not overwrite the persisted resting slot.
3. When the keyboard closes, the stack returns to its remembered resting slot.

This keeps the stack reachable during text input without polluting the user's preferred position.

### 11. Visual emphasis model

The right-side stack should not be fully flat or fully hidden while idle.

Idle-state rules:

1. slightly reduced visual emphasis
2. softer shadow / lower contrast than active interactions
3. no auto-hide
4. no auto-collapse to the edge

Interaction emphasis rules:

1. touch/press raises emphasis
2. tab change briefly emphasizes the active state
3. dragging strongly emphasizes the whole stack

### 12. Motion scope

This iteration includes only state-transition motion, not decorative entrance motion.

Included motion:

1. active tab highlight movement
2. press-to-drag elevation feedback
3. snap-to-slot release animation
4. drawer-toggle state transition
5. idle-to-active emphasis transitions

Excluded motion:

1. first-load fly-in animation
2. perpetual breathing / pulsing
3. autonomous floating behavior

## Implementation Design

### 1. Structural strategy

Keep the redesign localized to the current web shell inside:

- `app/web/src/main.tsx`
- `app/web/src/styles.css`
- existing web source-inspection tests

Recommended structure:

1. branch the shell render by narrow vs wide mode at the header/navigation layer
2. keep existing wide header structure intact
3. introduce narrow-only JSX for:
   - `Header Bubble`
   - `Floating Control Stack`
4. keep the existing content renderers for Chat / File / Git as much as possible

This should be a narrow-shell replacement, not a mutation of the current narrow header styles alone.

### 2. Narrow-shell state

Expected new narrow-screen state includes:

1. persisted floating-stack resting slot
2. transient drag state
3. transient drag activation / cooldown state
4. transient keyboard avoidance offset for the stack

The persisted model should prefer a semantic slot id or slot index over raw pixel positions.

### 3. Persistence strategy

Persist the **Floating Control Stack** resting slot in existing local web state storage.

Requirements:

1. restore on reload
2. restore on reconnect / hydration where appropriate
3. validate against current viewport and safe bounds
4. reset to default when the stored value is invalid or no longer safe

### 4. Safe regions

The narrow-shell safe region must account for:

1. top-left **Header Bubble**
2. viewport top/bottom safe areas
3. chat keyboard visible area

The **Floating Control Stack** must never rest inside the **Header Bubble** area.

### 5. Content inset strategy

The narrow-screen content container needs an initial top inset that matches the **Header Bubble** footprint plus spacing.

This inset is:

1. present at initial render / top position
2. part of the layout, not a dynamic scroll-follow transform
3. independent from chat keyboard inset behavior

## Testing Strategy

Continue using the existing web source-inspection tests already used in this codebase for layout/UI regressions.

Minimum coverage:

1. narrow-screen shell no longer relies on the old full-width header/tabs layout path
2. **Header Bubble** exists and preserves project-switch plus refresh structure
3. **Floating Control Stack** exists as a narrow-only structure
4. the navigation control uses a vertical pill structure with icon-only tabs
5. the drawer control is a separate single bubble
6. drag-related state and persistence hooks exist
7. stack persistence uses slot-based resting data
8. keyboard-driven temporary offset logic exists without writing back the persisted slot
9. header content inset logic exists for narrow screens

Validation after implementation:

- `cd app && npm test`
- `cd app && npm run tsc:web`

## Implementation Scope

Primary files:

- `app/web/src/main.tsx`
- `app/web/src/styles.css`
- `app/__tests__/web-chat-ui.test.ts`
- other narrow-shell web tests if needed

## Risks and Controls

### Risk 1: narrow and wide shells become tangled

Control:

- keep a clear render split at the shell level
- avoid mixing the new floating-shell states into the wide header path

### Risk 2: drag logic breaks click behavior

Control:

- explicitly separate tap, long-press activation, drag-active, and release-cooldown states
- add regression assertions for drag-related state presence

### Risk 3: persisted position becomes unstable across viewport changes

Control:

- persist slot identity instead of raw pixels
- recalculate actual y from safe bounds on each layout pass
- fall back to default safe slot when needed

### Risk 4: chat keyboard and floating controls fight each other

Control:

- treat keyboard offset as transient
- do not write transient offset into persisted state
- compose keyboard avoidance after resting-slot resolution
