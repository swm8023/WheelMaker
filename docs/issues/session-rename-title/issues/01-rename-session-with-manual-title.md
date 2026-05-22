# Rename Session With Manual Title

Status: ready-for-agent
Type: AFK

## Parent

`docs/issues/session-rename-title/PRD.md`

## What to build

Add the primary end-to-end rename path for a non-empty manual session title. A Workspace user should be able to open a session action, enter a new title, save it, and see the Session Summary update with the manual title. The request must travel through the Conversation Registry to the target Hub using `session.rename`.

Manual title state belongs to WheelMaker display metadata. It must be stored in the existing title facts model without changing the SQLite schema, must have higher display priority than automatic prompt or agent title facts, and must survive later automatic title updates.

## Acceptance criteria

- [ ] A `session.rename` request with `sessionId` and a non-empty `title` succeeds through the same routed session request path used by other Session methods.
- [ ] The registry accepts and forwards `session.rename`.
- [ ] The success response includes `ok: true`, the `sessionId`, and the updated Session Summary.
- [ ] The Hub publishes the normal session-updated event after a successful rename.
- [ ] Manual title state is persisted inside the existing title facts storage without a SQLite schema change.
- [ ] Manual title display wins over first-prompt, latest-prompt, legacy raw, and agent-side title facts.
- [ ] Later automatic prompt or agent title updates preserve the manual title.
- [ ] The session action strip exposes a Rename action on desktop and mobile.
- [ ] Rename is enabled for running sessions.
- [ ] Saving from the rename dialog updates the visible session row and selected-session title using the returned Session Summary or later session-updated event.
- [ ] Rename failures leave the existing visible title unchanged and surface an error through existing UI error handling.
- [ ] Tests cover the end-to-end non-empty rename path, manual-title priority, registry forwarding, and running-session allowance.

## Blocked by

None - can start immediately

## User stories covered

1, 2, 3, 4, 5, 8, 9, 12, 14, 17, 18, 19, 20, 21, 22

## Comments
