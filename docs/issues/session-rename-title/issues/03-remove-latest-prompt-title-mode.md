# Remove Latest Prompt Title Mode

Status: ready-for-agent
Type: AFK

## Parent

`docs/issues/session-rename-title/PRD.md`

## What to build

Remove the global `Use Latest Prompt Title` mode so unrenamed sessions have one predictable display rule. The Workspace should no longer expose, persist, or branch on this setting. Without a manual title, session display should prefer the first prompt title.

This slice is independent from the primary rename write path because it removes the old display mode and simplifies title resolution for all sessions.

## Acceptance criteria

- [ ] The settings UI no longer shows `Use Latest Prompt Title`.
- [ ] Workspace state no longer writes this setting to durable persistence.
- [ ] Existing persisted values are ignored without requiring a client-side migration.
- [ ] Frontend title resolution no longer accepts a latest-prompt mode flag.
- [ ] Unrenamed sessions display first prompt title when it is available.
- [ ] If first prompt title is unavailable, display falls back to latest prompt or legacy raw title.
- [ ] Tests that previously expected the latest-prompt setting are removed or updated.
- [ ] Tests cover first-prompt default display for unrenamed sessions.

## Blocked by

None - can start immediately

## User stories covered

10, 11

## Comments
