# Mobile Settings System Back Design

Date: 2026-05-20

## Goal

Make mobile Settings feel like a native layered page: the system back gesture or Android back action should move from a Settings detail page back to the Settings list, then from the Settings list back to the drawer.

The mobile visual hierarchy should also stop showing a top `Settings` title plus a second detail title below it. Detail pages should use one mobile title bar whose title is the active detail name.

## Context

The app is a React web UI loaded by the native shell. iOS already enables WebView back-forward gestures, and Android users expect the hardware/system back action to follow the current in-app layer. The current Settings state is internal React state:

- `sidebarSettingsOpen` controls whether Settings is open.
- `settingsDetailView` controls detail pages such as `Update`, `Skills`, `Token Stats`, `CC Switch`, and `Database`.

Because the Settings layers are not represented in `window.history`, the platform back gesture has no app-level Settings layer to pop.

## Scope

Runtime target: `app/web/`.

In scope:

- Mobile/narrow Settings history integration.
- System back from detail to Settings list.
- System back from Settings list to drawer.
- Mobile Settings title bar restructuring.
- Keep detail actions, including refresh buttons, in the mobile title bar.
- Keep desktop Settings detail presentation effectively unchanged.
- Add focused tests in the existing source-structure/service style.

Out of scope:

- Custom edge-swipe gesture recognition.
- Changing the existing optional `Gesture Navigation` floating-control feature.
- Reworking desktop Settings layout.
- Changing detail page data loading, refresh, or error behavior.
- Introducing a full router.

## Navigation Model

Mobile Settings should mirror its layer stack into browser history.

When narrow layout is active:

1. Opening Settings from the drawer creates a history layer for the Settings list.
2. Opening a Settings detail creates a second history layer for that detail.
3. A browser `popstate` while on a Settings detail clears `settingsDetailView`.
4. A browser `popstate` while on the Settings list closes Settings.

The implementation should avoid custom pointer or touch listeners for this behavior. Native platform gestures should drive the normal browser history path.

Desktop Settings should not push these mobile Settings history layers. Desktop activity-bar shortcuts and sidebar detail behavior should keep their current behavior.

## History State Shape

Use a small app-owned state marker instead of encoding Settings state in the URL.

Required properties:

- app marker to identify WheelMaker Settings history entries
- layer: `settings`
- detail: current `SettingsDetailView` or `null`

The handler must ignore unrelated browser history entries.

The implementation should guard against duplicate pushes when React re-renders with the same Settings layer.

## Mobile Title Bar

Mobile Settings should render one title bar:

- Settings list: title is `Settings`.
- Detail page: title is the detail label, such as `Update`, `Skills`, or `Token Stats`.
- Left button:
  - Settings list: returns to drawer.
  - Detail page: returns to Settings list.
- Right area:
  - Detail page actions render here.
  - Existing refresh/export actions remain available in the title bar.
  - Settings list may keep an empty spacer for alignment.

The detail body should not render a second title header on mobile. The current shared detail shell may remain for desktop, but mobile should avoid the stacked `Settings` plus detail-heading effect.

## Desktop Behavior

Desktop should keep the current Settings detail layout and visual weight unless a small shared refactor is needed to pass title/action data to the mobile shell.

Desktop detail pages still have:

- back button
- detail title
- right-side actions where present
- scrollable content body

## Testing Strategy

Use existing Jest source-structure tests and add pure helper tests where practical.

Tests should lock:

- mobile Settings includes a history-backed popstate path rather than pointer/touch swipe handling
- mobile Settings title changes to the active detail title
- detail actions are passed to the mobile title bar
- detail body can render without the duplicate detail header on mobile
- desktop detail shell still includes the existing `.settings-detail-header`
- system back from detail clears `settingsDetailView`
- system back from Settings list closes Settings

## Acceptance Criteria

- On mobile, system back from a Settings detail returns to the Settings list.
- On mobile, system back from the Settings list closes Settings and returns to the drawer.
- No custom left-edge swipe recognizer is introduced.
- Mobile detail pages show a single title bar.
- Existing detail refresh/export actions remain in the title bar.
- Desktop Settings detail pages keep their current layout.
- Existing Settings detail loading, refresh, and content behavior remains unchanged.
