# Clear Manual Title And Normalize Rename Input

Status: ready-for-agent
Type: AFK

## Parent

`docs/issues/session-rename-title/PRD.md`

## What to build

Extend the rename path so users can clear a manual title and so title input is safe for session-list display. Saving an empty or whitespace-only title should remove manual title state and restore automatic display behavior. Non-empty input should be normalized at the frontend and backend input boundaries.

The result should keep title semantics deterministic: clearing manual title returns display priority to first prompt title, then latest or legacy fallback only when first prompt title is unavailable.

## Acceptance criteria

- [ ] Saving an empty title clears manual title state.
- [ ] Saving whitespace-only input clears manual title state.
- [ ] After clearing manual title, the displayed title falls back to first prompt title when available.
- [ ] If first prompt title is unavailable after clearing, display falls back to latest prompt or legacy raw title.
- [ ] Rename input trims outer whitespace.
- [ ] Multiline pasted input is normalized into a single-line title.
- [ ] Backend validation enforces the same boundary semantics as the frontend.
- [ ] Title length is capped at 200 characters.
- [ ] A title over the limit cannot create a stored or displayed title longer than 200 characters.
- [ ] Tests cover empty clear, whitespace clear, fallback display, newline normalization, trim behavior, and the 200-character limit.

## Blocked by

- `docs/issues/session-rename-title/issues/01-rename-session-with-manual-title.md`

## User stories covered

6, 7, 15, 16

## Comments
