# Chat Top Search Header Design

## Goal

Improve the chat session navigation header so search is promoted to the top chrome instead of living as the first row of the session list. The change covers both wide desktop and mobile drawer layouts, while keeping the existing session search protocol, result model, and turn navigation behavior.

## Current Context

The Web UI already has:

- `renderChatHubSummary()` in `app/web/src/main.tsx`, currently rendered in the wide `sidebar-title-row` and the mobile chat drawer header.
- `renderSessionSearchControls()` and `renderSessionSearchResults()`, currently rendered inside the session navigation area.
- Session search state that starts, polls, cancels, and merges results across `sortedProjectItems`.

This design moves the search entry point and active-search status into the chat header. It does not change server search actions or result semantics.

## Layout

Wide desktop default header:

```text
CHAT  [3 Hubs v]                         [search]
```

Mobile default header:

```text
[settings] [update] [port] [refresh]     [search] [3 Hubs v]
```

The two default layouts intentionally differ because mobile keeps its existing tool cluster and places search immediately to the left of the Hub button.

Search-expanded state is shared by desktop and mobile:

```text
[search input                                    ] [confirm] [close]
[active-search status line, when applicable]
```

When the search box is expanded, the header temporarily hides the wide title, Hub button, and mobile tool buttons. Closing search restores the default header.

## Search Interaction

Clicking the search icon expands and focuses the search input. It does not start a search.

Submitting with Enter or the confirm button starts search. If another search is active, the old search is cancelled and replaced by the new one.

The close button cancels any active search and exits search mode. This is the explicit search-exit action; switching to another page does not cancel search.

When the input expands, the existing query text is not selected. Focus is placed in the input so the user can continue editing.

If a search is active or complete and the user edits the input without submitting, the existing results and status continue to represent the last submitted query. Submitting again starts a new search for the edited input.

## Status Line

The status line lives directly under the expanded search input.

It appears only when there is an active or completed search. It does not appear merely because the input is expanded.

Searching:

```text
Searching 2/8 projects · 3 results
```

Completed:

```text
3 results
```

Errors are appended in both states:

```text
Searching 2/8 projects · 3 results · 1 error
3 results · 1 error
```

The project denominator is `sortedProjectItems.length`, matching the session navigation scope already shown in the list. The completed count is derived from the existing merged search sections.

Detailed per-project errors remain in the result list area, as they do today.

## Hub Summary

The Hub button changes from a hidden `Hubs` label plus numeric badge to a complete text label:

```text
0 Hubs
1 Hub
3 Hubs
```

The dropdown remains informational only. It lists hub id plus `Local` or `Remote` read status. It does not filter projects, sessions, or search scope.

The dropdown may be wider than the button. It should have a practical minimum width and stay within the viewport; long hub ids remain single-line with ellipsis.

## Components And State

Implementation should keep the existing state in `main.tsx` and avoid changing the registry service contract.

Expected shape:

- Add a chat-header search renderer that can be used by both wide and mobile headers.
- Keep `renderSessionSearchResults()` in the session list area.
- Split the current status text into a reusable status-line renderer or helper.
- Adjust `renderChatHubSummary()` to emit the full Hub label.
- Update wide and mobile header CSS so default and expanded layouts are explicit.

The existing `sessionSearchOpen`, `sessionSearchInput`, `activeSessionSearchId`, `sessionSearchDoneByProjectId`, `searchResultsByProjectId`, and error state should remain the source of truth.

## Error Handling

Search start/query/cancel failures keep current behavior: project-level errors are collected into `sessionSearchErrorsByProjectId`, and detailed messages render in the result area.

The header status line only shows an aggregate error count. It should not grow vertically with long messages.

If cancelling fails during close, the UI should still exit search mode using the existing cancel path behavior, consistent with current search exit semantics.

## Testing

Add or update focused Web tests that verify:

- Wide chat header contains `CHAT`, the full Hub label, and a right-aligned search entry.
- Mobile chat header keeps tool buttons, places search to the left of the Hub button, and uses the same expanded search markup.
- Expanded search hides the default header content on both wide and mobile.
- Status text distinguishes searching and completed states, including aggregate errors.
- Hub label pluralization is `0 Hubs`, `1 Hub`, and `N Hubs`.

Run the existing Web verification commands after implementation:

```bash
npm test -- --runInBand
npm run tsc:web
npm run build:web
```
