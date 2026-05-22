# Session Rename Title PRD

Status: ready-for-agent

## Problem Statement

WheelMaker currently derives chat session titles from prompt title facts. Users cannot correct, shorten, or reset a session title from the web UI. The current `Use Latest Prompt Title` option also makes title behavior less predictable because one global setting can change how every session label is displayed.

Users need a simple session-level rename action. A manually renamed session must keep that title until the user clears it, even if later prompts or agent-side title updates arrive.

## Solution

Add a frontend rename action for each session. The action opens a single-line dialog prefilled with the current displayed title. Saving a non-empty value stores a manual title. Saving an empty or whitespace-only value clears the manual title and restores the automatic first-prompt title.

The backend exposes a new `session.rename` request and stores manual title state inside the existing session title facts JSON. No SQLite schema migration is required. The display-title rule becomes deterministic:

1. Use `manual` when present.
2. Otherwise use `first`.
3. Otherwise use `last` or legacy raw title when needed.

The global `Use Latest Prompt Title` setting is removed. Unrenamed sessions use the first prompt title by default.

## User Stories

1. As a WheelMaker web user, I want to rename a session from the session list, so that I can recognize it later without relying on the first generated prompt title.
2. As a WheelMaker web user, I want the rename action near existing session actions, so that I do not need to search through settings or another screen.
3. As a WheelMaker web user, I want a focused rename dialog, so that I can edit the title without navigating away from the chat.
4. As a WheelMaker web user, I want the rename dialog to show the current displayed title, so that I can make a small edit instead of retyping it.
5. As a WheelMaker web user, I want Save and Cancel actions, so that I can either commit or abandon title edits explicitly.
6. As a WheelMaker web user, I want an empty saved title to clear my manual rename, so that I can return a session to automatic naming.
7. As a WheelMaker web user, I want whitespace-only input to behave like empty input, so that accidental spaces do not create invisible titles.
8. As a WheelMaker web user, I want renamed sessions to keep their manual title after new prompts, so that later prompt titles do not undo my organization.
9. As a WheelMaker web user, I want renamed sessions to keep their manual title after agent-side title events, so that provider updates do not override my choice.
10. As a WheelMaker web user, I want unrenamed sessions to use the first prompt title, so that old sessions have stable names.
11. As a WheelMaker web user, I want the latest-prompt title setting removed, so that session title behavior is consistent.
12. As a WheelMaker web user, I want to rename a running session, so that I can label it while the agent is still working.
13. As a WheelMaker web user, I want renaming to update the session row without moving the row, so that the list does not jump unexpectedly.
14. As a WheelMaker web user, I want renaming to work on desktop and mobile layouts, so that the feature is available wherever the session action strip appears.
15. As a WheelMaker web user, I want long titles to be bounded, so that accidental paste input cannot break the UI.
16. As a WheelMaker web user, I want pasted multiline text normalized into a single-line title, so that session labels remain list-friendly.
17. As a WheelMaker web user, I want rename failures to leave the old title visible, so that the UI does not imply a change that was not saved.
18. As a WheelMaker web user, I want title changes from another client to appear through normal session updates, so that multiple open clients converge.
19. As a WheelMaker developer, I want one backend title-facts module to own title parsing and priority rules, so that automatic and manual title behavior stays testable.
20. As a WheelMaker developer, I want one frontend title resolver to mirror backend display semantics, so that list labels and selected-session labels do not diverge.
21. As a WheelMaker developer, I want no database schema change for this feature, so that existing hubs can upgrade without startup migration risk.
22. As a WheelMaker developer, I want `session.rename` to be a registry-routed session request, so that local and remote hub behavior uses the same contract.

## Implementation Decisions

- The public request method is `session.rename`.
- The request payload contains `sessionId` and `title`.
- The success response returns `ok: true`, the `sessionId`, and the updated session summary.
- The registry method whitelist accepts `session.rename`.
- The backend stores manual title state by extending the existing session title facts object with `manual`.
- The backend does not add, remove, or migrate SQLite columns for this feature.
- The backend title facts helper becomes the authoritative deep module for parsing, updating automatic facts, setting manual facts, clearing manual facts, and resolving display title.
- Automatic title events continue to update automatic facts, but they must preserve any existing manual title.
- Manual title has the highest display priority.
- Empty or whitespace-only rename input clears the manual title.
- Title normalization happens at input boundaries: trim outer whitespace, replace newlines with spaces, collapse unsafe multiline content into one line, and enforce a 200-character maximum.
- Rename is allowed while a session is running.
- Rename must not update session activity ordering, last-active timestamps, turn cursors, read cursors, or archived state.
- Rename is WheelMaker display metadata only. It does not propagate to Codex, Claude, ACP, or provider-native thread title state.
- Frontend title resolution removes the global latest-prompt toggle and always uses the deterministic title priority.
- The `Use Latest Prompt Title` setting is removed from UI state, persistence writes, and settings rendering.
- Existing persisted `Use Latest Prompt Title` values can be ignored; no client-side migration is needed.
- The session action strip gains a Rename action that is enabled even when Reload and Archive are disabled for running sessions.
- The rename dialog is a compact single-line modal using the existing confirmation/dialog style where practical.
- The frontend service layer exposes a rename method that calls `session.rename` through the existing registry repository path.
- The UI applies the server-returned session summary after save and also accepts later `registry.session.updated` events as the source of convergence.

## Testing Decisions

- Tests should assert externally visible behavior, not internal helper call order.
- Backend request tests should cover `session.rename` with a non-empty manual title.
- Backend request tests should cover empty or whitespace-only rename clearing the manual title.
- Backend request tests should cover automatic prompt or agent title updates after manual rename without overriding manual title.
- Backend request tests should cover rename while a session is running.
- Backend request tests should cover rename not changing list ordering or last-active timestamps.
- Backend request tests should cover input normalization and the 200-character limit.
- Registry tests should cover forwarding `session.rename` through the same route as other session requests.
- Frontend title resolver tests should cover `manual > first > last > legacy raw`.
- Frontend repository and workspace service tests should cover the `session.rename` request method and payload.
- Frontend UI tests should cover the Rename action presence in the session action strip.
- Frontend UI tests should cover the removal of `Use Latest Prompt Title` from settings and persistence expectations.
- Existing session archive, reload, config, and sync tests are prior art for request routing and session summary assertions.

## Out of Scope

- Renaming provider-native Codex, Claude, ACP, or app-server thread titles.
- Adding a database column or schema migration for manual titles.
- Supporting per-client title preferences.
- Supporting title history, undo, or audit logs.
- Supporting bulk rename or project-level rename rules.
- Changing session sorting, filtering, archive behavior, reload behavior, or read cursor behavior.
- Adding a latest-prompt-title display mode under a different setting name.
- Building a separate title editing screen outside the existing session list workflow.

## Further Notes

The key product rule is that manual title state is user-owned WheelMaker metadata. Automatic title data can still be recorded for fallback display, but it must never win over `manual` until the user explicitly clears the manual title.

No external issue tracker is configured in this repository. This PRD is stored as a local `ready-for-agent` proposal under the existing Superpowers specs directory.
