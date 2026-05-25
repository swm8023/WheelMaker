# Session Search Design

## Goal

Add search across all normal chat sessions in the current workspace. Search covers session titles and prompt content. It is performance-conscious: starting a search hides non-matching sessions, then matching sessions appear as project-scoped search tasks progress.

## Scope

Search includes sessions returned by normal `session.list` for known projects. It does not include archived sessions, deleted sessions, or resumable sessions that have not been imported into the normal session list.

Search is exact text containment with case-insensitive matching. It does not include regex, fuzzy matching, pinyin matching, or tokenization.

## Architecture

Use project-scoped asynchronous `session.search` tasks. The Registry continues to route `session.*` calls by `projectId`; it does not become a global search aggregator.

The Web UI creates one UI search id for a user search and fans out project-scoped `session.search` requests to all known projects. The UI merges per-project results and renders them in the existing project/session navigation order.

This fits the existing `session.list` and `session.read` routing model, where each project `Client` owns only that project's sessions.

## Protocol

Add `session.search` to the existing `session.*` method family.

Request payload:

```ts
type SessionSearchRequest =
  | {
      action: 'start';
      searchId: string;
      query: string;
    }
  | {
      action: 'query';
      searchId: string;
    }
  | {
      action: 'cancel';
      searchId: string;
    };
```

`start` response:

```ts
type SessionSearchStartResponse = {
  searchId: string;
  done: false;
};
```

`query` response:

```ts
type SessionSearchQueryResponse = {
  searchId: string;
  done: boolean;
  results: SessionSearchResult[];
  errors: SessionSearchError[];
};

type SessionSearchResult = {
  projectId: string;
  sessionId: string;
  source: 'title' | 'prompt';
  turnIndex?: number;
};

type SessionSearchError = {
  projectId: string;
  sessionId?: string;
  message: string;
};
```

`cancel` response:

```ts
type SessionSearchCancelResponse = {
  searchId: string;
  done: true;
};
```

`query` always returns the current full result set for that project search task. It does not use a cursor and does not return incremental-only batches.

Result constraints:

- `source: 'title'` means the session title matched. It has no `turnIndex`.
- `source: 'prompt'` means prompt content matched. It must include `turnIndex`.
- There is no snippet, preview, or match count in the protocol.

## Server Behavior

Each project `Client` owns a search manager keyed by `searchId`.

`start`:

1. Validate `query`: trim it, require non-empty text, and reject values over 200 characters.
2. Take a snapshot of that project's normal sessions at start time.
3. Create a task with the snapshot, query, cancellation context, results buffer, errors buffer, `done` flag, and `lastTouched`.
4. Start a background goroutine and return immediately.

Search task execution:

1. Process sessions serially. Do not scan multiple sessions concurrently.
2. For each session, first match the session title.
3. If the title matches, append one result with `source: 'title'` and do not scan that session's prompt content.
4. If the title does not match, scan prompt turns from newest to oldest.
5. Search only text that the chat UI would display to the user. Do not search JSON key names, `method`, `sessionId`, protocol fields, or local command metadata hidden by the UI.
6. If prompt content matches, append one result with `source: 'prompt'` and the matching `turnIndex`, then stop scanning that session.
7. If reading a session or turn file fails, append a session-level error and continue with the remaining sessions.
8. Set `done: true` when the snapshot is fully scanned or the task is cancelled.

Search should use a dedicated session-turn scanner that reads each WMT2 turn file once and scans slots within the file. It should not use the existing `session.read(0)` path for full-session searching, because that path is optimized for correctness reads rather than search.

`query`:

1. Look up the task by `searchId`.
2. Refresh `lastTouched`.
3. Return the current full `results`, current `errors`, and `done`.
4. If the task does not exist or expired, return a clear error.

`cancel`:

1. Cancel and remove the task if it exists.
2. Treat missing tasks as successful cancellation.

Cleanup:

- `start` and `query` update `lastTouched`.
- Completed tasks keep their results for delayed `query` calls.
- Tasks idle for 10 minutes are cancelled and cleaned up.
- A project keeps at most 8 search tasks. When the cap is reached, prefer clearing completed tasks with the oldest `lastTouched`, then oldest idle tasks. If no task can be cleared, reject the new `start`.
- A new `start` does not globally cancel other `searchId` tasks, because different Web UI clients can be searching at the same time. The Web UI cancels its own previous search id before starting another search.

## Web UI Behavior

Search state is in memory only. It is not persisted across page reloads or app restart.

Search controls live inside the chat session list area, near the existing project or Hub information row. Desktop and mobile use the same interaction model. The default state is a search icon. Clicking it expands an input, a search/confirm button, and a close button.

Start search:

1. Pressing Enter or clicking the confirm button starts a search.
2. Empty trimmed input cancels any active search and restores the normal list.
3. A new search cancels the previous UI search id across all projects, creates a new search id, sends `start` to every known project, and enters search mode immediately.
4. Search mode hides ordinary sessions first and shows `Searching... 0 results` until results arrive.

Polling:

1. While the Chat page/list is visible and any project search is not done, poll `query`.
2. Poll every 300ms while results are changing.
3. After 3 consecutive polls with no visible result change, back off to 800ms.
4. Stop polling a project once its `done` is true.
5. When switching to File or Git, stop polling but do not cancel search tasks.
6. When returning to Chat, immediately `query` active project searches and resume polling if needed.

Rendering:

1. Maintain `searchResultsByProjectId` separately from the normal `projectSessionsByProjectId`.
2. Existing session index updates continue normally while search mode is active, but search mode rendering only uses search results until the user exits search.
3. Show only projects with matching sessions. If no results exist and all searches are done, show `No matching sessions`.
4. Keep project ordering and session ordering consistent with the normal session list. Do not sort by match strength.
5. For `source: 'title'`, highlight the query text in the displayed session title.
6. For `source: 'prompt'`, show a compact marker such as `Prompt · turn 42`.
7. Clicking a title result opens the session.
8. Clicking a prompt result opens the session, scrolls to the matched `turnIndex`, and temporarily highlights that turn for about 2 seconds.
9. Clicking a result keeps search mode active so the user can inspect multiple results. On mobile, selecting a result closes the drawer after opening the session.

Exit search:

1. The close/clear button cancels the active UI search id across all projects.
2. The UI exits search mode and restores the normal project/session list.
3. Cancel failures or already-expired project tasks do not block leaving search mode.

## Chat Turn Jump

`ChatVirtuosoTurnList` currently exposes bottom-scroll operations. Add an imperative operation for scrolling to a `turnIndex`.

The implementation should map `turnIndex` to the corresponding display item in `ChatDisplayIndex`. If that exact turn is hidden by existing display rules, scroll to the nearest following visible turn. If no following item exists, scroll to the nearest preceding visible turn.

The chat view stores a transient highlighted turn index. After a prompt search result opens a session and the target turn is visible, the UI highlights it briefly and then clears the highlight.

## Error Handling

- Project-level `start` or `query` failure affects only that project. Other project results remain usable.
- Session-level read failures produce task errors and do not block other sessions.
- Missing or expired `searchId` on `query` produces a clear project-level error. If every project search has expired, the UI stops searching and lets the user start a new search.
- Missing `searchId` on `cancel` is treated as successful cancellation.
- Search results are a snapshot of sessions captured at `start`. Sessions created, imported, renamed, archived, or deleted during a search do not dynamically change the task's session set. Exiting search reveals the normal session index, which continues to update in the background.

## Performance Notes

Measured on the current local dataset:

- 10 sessions with persisted turns.
- 35 WMT2 turn files.
- About 7,391 turns.
- About 13.69 MB of payload.
- File-once scanning of all turn files took about 29 ms.
- JSON string-value scanning took about 39 ms average.
- Simulating the existing per-turn file read path took about 667 ms.

Therefore the search implementation should scan WMT2 turn files directly and avoid using `session.read(0)` for full-session search.

Search also avoids unnecessary work by checking title before prompt and stopping prompt scanning after the first matched turn in a session.

## Tests

Server tests:

- `session.search` supports `start`, `query`, and `cancel`.
- `start` validates query emptiness and max length.
- `query` returns full results, not only incremental deltas.
- `cancel` is idempotent for missing tasks.
- Title matches return `source: 'title'` and skip prompt scanning.
- Prompt scanning runs newest-to-oldest and stops at the first matched turn.
- Prompt extraction searches visible text but not JSON keys, protocol fields, or hidden command metadata.
- Session read/turn corruption errors are returned in `errors` and do not stop other sessions.
- Completed tasks remain queryable until 10 minutes idle.
- Idle cleanup and per-project task cap are enforced.

Web tests:

- The search control expands from the session-list search icon.
- Enter and confirm button start search; empty input exits search.
- Starting a new search cancels the previous UI search id for every project.
- Project fan-out merges full per-project results.
- Search mode hides non-result projects and sessions.
- Title results highlight title text.
- Prompt results display `Prompt · turn N`.
- Switching away from Chat stops polling without cancelling; returning to Chat queries again.
- Clicking a title result opens the session and keeps search mode.
- Clicking a prompt result opens the session, closes the mobile drawer when applicable, scrolls to the turn, and briefly highlights it.
- Closing search cancels project tasks and restores the normal list.
