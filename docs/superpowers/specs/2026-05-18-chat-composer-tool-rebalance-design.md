# Chat Composer Tool Rebalance Design

## Goal

Make the chat composer more compact by removing the composer-level quick reply menu, moving Skills into the left input-row affordance, adding a placeholder `@` file mention action in the lower tool row, and removing the chevron icon from chat config pills while preserving their selection behavior.

## Context

The current web chat composer has three separate action areas:

- a left input-row quick reply button that opens `确认` / `接受`
- a right send button
- a lower tool row with Skills, image attach, stop, and config pills

The composer also has assistant-message option chips and inline confirmation replies. Those are message-level reply actions and should remain.

This design only changes the composer controls. It does not change message rendering, option chips, inline confirmation replies, or future file mention search behavior beyond adding an explicit placeholder entry point.

## Confirmed Scope

- Runtime target: `app/web/`.
- Delete the composer-level quick reply feature:
  - remove the left `A` quick reply trigger
  - remove the `确认` / `接受` menu
  - remove quick-reply-only state, refs, handlers, tests, and CSS
- Keep assistant message A/B/C option chips.
- Keep assistant message inline confirmation check replies.
- Move the existing Skills action into the left input-row position.
- The left Skills action keeps the existing Skills behavior:
  - opens the existing slash/skill list
  - keeps title `Skills`
  - keeps aria label `Open skills`
  - focuses the textarea after opening
- Use a terminal/command-style Codicon for the left Skills action instead of the current wand.
- Replace the lower tool-row Skills button with an `@` file mention placeholder button.
- Lower tool-row order becomes:
  1. `@`
  2. image attach
  3. stop
- The `@` button opens a full-width menu above the input frame, matching the existing skill/slash menu placement.
- The `@` button does not insert `@` into the composer text.
- The `@` menu shows only `File mentions coming soon`.
- Opening `@` closes other composer menus.
- Opening Skills closes the `@` menu and other composer menus.
- Remove the chevron icon from chat config pills.
- Keep config pill click selection behavior unchanged.

## Non-Goals

- Do not implement file search or file mention insertion.
- Do not change keyboard behavior for Skills.
- Do not remove assistant message option chips.
- Do not remove assistant message inline confirmation replies.
- Do not redesign project/session chevrons outside the chat composer config pills.
- Do not move the stop button away from the lower tool row.
- Do not change image attachment behavior.

## Composer Layout

The input row becomes:

1. left Skills button
2. textarea
3. send button

The lower tool row becomes:

1. `@` file mention placeholder
2. image attach
3. stop prompt
4. config pills and overflow controls

The left Skills button should reuse the compact footprint of the existing quick reply trigger so the textarea stays aligned and the composer remains dense.

## Menu Behavior

### Skills

The left Skills button opens the existing slash/skill menu. The menu remains a listbox above the composer. Opening it focuses the textarea, preserving the current flow where the user can continue filtering or use keyboard navigation.

### File Mentions Placeholder

The `@` button opens a full-width menu above the composer with a single disabled-looking placeholder row:

`File mentions coming soon`

The placeholder row is informational only. It is not selectable and does not mutate the composer draft. The menu closes when another composer menu opens, when clicking outside, or when the user takes an action that already closes composer menus.

## Config Pills

Config pills keep their existing click targets, menus, and selected-value behavior. Only the chevron icon inside each pill is removed to reduce visual clutter and horizontal width.

The config overflow button can keep its icon because it is a separate "more options" affordance, not an individual config pill chevron.

## Testing

Update existing web chat source/CSS tests:

- assert the quick reply options, quick reply menu, and quick reply handlers are removed
- assert the input-row left action is the Skills action
- assert the lower tool row contains an `@` placeholder action before image and stop
- assert the file mention placeholder menu is present with `File mentions coming soon`
- assert opening Skills and opening `@` close each other's menu state
- assert config pill chevron markup is absent while config value menus remain present
- keep existing assertions for assistant option chips and inline confirmation replies

Run targeted chat UI tests, TypeScript, full app Jest, diff check, and the web release build.
