# Chat Hub Summary Dropdown Design

Date: 2026-05-20

## Goal

Show the number of connected Hubs beside the Chat drawer title on both mobile and wide layouts, with a small dropdown that lists the current Hub names.

This gives users quick visibility into how many Hubs are connected where they already manage Chat sessions, without adding more UI to the main conversation view.

## Context

The web app already receives Hub data from `project.list` and stores it in `registryHubs`. The current `RegistryHub` model only exposes `hubId`, so the first version should show Hub names only.

The relevant Chat drawer titles are rendered in the sidebar/drawer shell:

- wide layout: `.sidebar-title-row` showing `CHAT`
- mobile layout: `.mobile-chat-drawer-header` showing `Chats`

The new Hub control belongs beside these drawer Chat titles, not in the main content title bar, mobile floating navigation, or desktop activity bar.

## Scope

Runtime target: `app/web/`.

In scope:

- Add a compact Hub summary control beside the Chat drawer title.
- Show the current Hub count.
- Add a dropdown button with a chevron.
- On click, show a lightweight popover listing current Hub names.
- Use the same render path for mobile and wide layouts, with CSS density adjustments.
- Show `No hubs` when the Hub list is empty.
- Close the popover when clicking outside or pressing Escape.
- Keep current main Chat title, breadcrumb, and session title behavior unchanged.

Out of scope:

- Changing Registry protocol or adding Hub metadata fields.
- Showing health, version, endpoint, package, or skill details.
- Making the Hub list selectable.
- Moving project or session navigation.
- Changing Settings or Update details.

## Information Display

The collapsed control shows:

- label: `Hubs`
- count: number of entries in `registryHubs`
- chevron icon

The expanded popover shows:

- one row per Hub
- each row displays only `hub.hubId`
- empty state: `No hubs`

Hub names should use the same visual language as existing Hub tags where practical, but the collapsed control should stay compact enough for drawer title bars.

## Placement

The Hub summary renders inside the Chat drawer/sidebar title area.

Wide layout:

- keep the main content title `CHAT - ...` unchanged
- place the Hub control to the right of the sidebar `CHAT` title in `.sidebar-title-row`
- do not add an activity-bar item

Mobile layout:

- keep the main content breadcrumb title unchanged
- place the Hub control beside `Chats` in `.mobile-chat-drawer-header`
- allow the `Chats` label to truncate before the Hub control
- keep tap target size comfortable enough for touch

## Interaction

Clicking the Hub control toggles the popover.

The popover should close when:

- the user clicks outside it
- the user presses Escape
- the user switches away from the Chat tab
- the user enters Settings

The popover does not change project, Hub, or session state.

## Testing Strategy

Follow the existing Jest source-structure and CSS assertion style.

Tests should lock:

- `registryHubs.length` drives the Hub count.
- Chat drawer/sidebar title renders a Hub summary control in the Chat branch.
- The control has an explicit dropdown button and chevron.
- The popover lists `registryHubs.map(hub => hub.hubId)`.
- The empty state is `No hubs`.
- Outside click and Escape close the popover.
- Main content Chat title does not render the Hub summary control.
- Mobile and wide drawer title paths both use the same Hub summary renderer.
- CSS keeps the Hub control compact and allows drawer title text to truncate.

## Acceptance Criteria

- Mobile Chat drawer title shows a Hub count control beside `Chats`.
- Wide Chat sidebar title shows the same Hub count control beside `CHAT`.
- Main Chat content title does not show the Hub count control.
- Clicking the control opens a dropdown listing current Hub names.
- With no Hubs, the dropdown shows `No hubs`.
- The control does not affect project/session selection.
- Existing main Chat title, breadcrumb, and toolbar behavior remain intact.
