# ADR: Responsive Shells Use One Workspace UI State

Date: 2026-05-14
Status: Accepted

## Context

WheelMaker Workspace now has materially different desktop and mobile shell structures. Desktop uses a wide header plus fixed sidebar. Mobile uses a drawer, Floating Control Stack, and fullscreen surfaces such as settings. A narrow PC browser window must follow the mobile shell rules, so the shell decision must be based on viewport width rather than physical device type.

The existing implementation accumulated `isWide` checks and local UI `useState` calls in `main.tsx`. That made layout behavior easy to add incrementally, but it increased the risk that a future change would update only one shell or reset state unexpectedly during a wide/narrow transition.

## Decision

Use two Responsive Shell structures, backed by one Workspace UI State.

The Layout Mode is resolved from viewport width:

```ts
windowWidth >= 900 ? 'desktop' : 'mobile'
```

The Workspace UI State is organized into four namespaces:

- `shared`: UI state that must survive across both shells, such as selected tab and settings route/open state.
- `desktop`: desktop-only shell state, such as sidebar collapse.
- `mobile`: mobile-only shell state, such as drawer state, Floating Control Stack slot, and mobile overflow menus.
- `transient`: gesture, keyboard, and measurement state that must not be restored across Layout Mode transitions.

Business data remains outside Workspace UI State. Chat sessions, selected files, selected diffs, project data, persistence caches, and server data continue to live in their existing workspace data flow.

## Consequences

- Desktop and mobile shells can diverge structurally without duplicating business UI state.
- PC narrow windows use mobile shell behavior consistently.
- Layout Mode transitions become explicit reducer events instead of scattered cleanup effects.
- Long-lived layout preferences can persist, while transient drag and keyboard state is cleared.
- Future shell extraction should pass shared content into `DesktopShell` and `MobileShell`, not duplicate Chat/File/Git content.

## Implementation Notes

The first implementation slice introduces:

- `responsiveLayout.ts` for the 900px Layout Mode rule.
- `workspaceUiState.ts` for root UI state creation and reducer transitions.
- A compatibility bridge in `main.tsx` that keeps existing variable names while sourcing the first migrated UI fields from the reducer.
- `shell/ResponsiveShell.tsx` for the desktop/mobile Responsive Shell split while keeping Chat/File/Git content shared.

The next slices should continue moving UI-only state into Workspace UI State and then extract focused shell surfaces such as Settings and drawer-specific overlays.
