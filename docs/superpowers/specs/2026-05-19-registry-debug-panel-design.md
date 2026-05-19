# Registry Debug Panel Design

Date: 2026-05-19

## Goal

Add a desktop-only debug capture mode for the web app that records raw Registry WebSocket I/O through one unified client boundary. When Debug is enabled in Settings, the app captures Registry requests, responses, errors, events, and WebSocket lifecycle records, then shows them in a floating inspection panel.

The goal is to debug Registry traffic and session synchronization without scattering debug hooks across feature-specific service methods and without changing the normal workspace layout.

## Scope

In scope:

- Add a Settings `Debug` control with a capture switch and an `Open` button.
- Persist the Debug capture switch.
- Capture all Registry WebSocket I/O through `RegistryClient`.
- Capture WebSocket lifecycle records.
- Do not capture `connect.init`.
- Store records in memory only while Debug is enabled.
- Clear records when Debug is disabled.
- Provide a desktop floating panel that can be closed, dragged, and resized.
- Render a virtualized record list on the left and selected record JSON on the right.
- Filter records by session ID, with session IDs discovered from captured records.
- Keep a separate `Include multi-session records` filter switch, defaulting to off.
- Provide a `Clear` action that clears current records while capture remains enabled.
- Follow latest records only while the list is already at the bottom.

Out of scope:

- Persisting debug records across page refresh.
- Persisting floating panel position or size.
- Searching records.
- Exporting records.
- Masking or redacting normal Registry payloads.
- Adding a tree JSON viewer.
- Changing mobile layout beyond exposing the setting state.
- Changing Registry protocol or server behavior.

## Existing System

The web app already has a central Registry client path:

- `app/web/src/services/registryClient.ts`
  - `RegistryClient.request()` constructs and sends request envelopes.
  - `RegistryClient.bind().onmessage` receives and parses WebSocket messages.
  - `RegistryClient.onEvent()` broadcasts event envelopes.
  - `RegistryClient.onClose()` broadcasts socket close.
- `app/web/src/services/registryRepository.ts` calls `RegistryClient.request()` for project, file, git, session, token, npm, and update operations.
- `app/web/src/services/registryWorkspaceService.ts` owns repository lifecycle and forwards events to the app.

The debug hook should live at `RegistryClient`, because this is the only layer that sees both outbound envelopes and inbound raw socket data before business normalization.

## Capture Model

Add a debug observer mechanism to `RegistryClient`.

When Debug is disabled:

- `RegistryClient` does not build debug records.
- It does not stringify extra data beyond existing send behavior.
- It does not extract session IDs.
- It only performs the minimum listener-enabled check.

When Debug is enabled:

- Outbound request envelopes are recorded before `ws.send`.
- Outbound records keep the same raw JSON string that is passed to `ws.send`.
- Inbound WebSocket strings are recorded after receive and successful JSON parse.
- Inbound parse failures are recorded as debug parse-error records.
- Response and error envelopes are recorded before resolving or rejecting pending requests.
- Event envelopes are recorded before broadcasting to event listeners.
- WebSocket lifecycle records are recorded for connect start, open, close, and error.
- `connect.init` requests are not recorded.

Records are stored only in React memory for the current page lifecycle. Refreshing the page drops previous records. Disabling Debug clears records and request correlation state.

## Record Shape

The UI should keep a normalized debug record shape separate from Registry wire types.

Recommended fields:

- `id`: monotonically increasing number for UI ordering.
- `timestamp`: local epoch milliseconds.
- `timeText`: local `HH:mm:ss.SSS`.
- `direction`: `out`, `in`, or `lifecycle`.
- `phase`: `request`, `response`, `error`, `event`, `parse_error`, or lifecycle phase.
- `method`: Registry method when available.
- `requestId`: Registry request ID when available.
- `projectId`: Registry project ID when available.
- `sessionIds`: session IDs extracted from this record or inherited by request ID.
- `multiSession`: true when the record maps to multiple session IDs.
- `durationMs`: response or error duration when correlated to an outbound request.
- `raw`: raw JSON string for outbound and inbound Registry messages when available.
- `envelope`: parsed Registry envelope when available.
- `lifecycle`: lifecycle payload for non-envelope records.

Raw text is retained in the record because it represents the wire text. The right detail pane displays only:

- Pretty JSON of `envelope` for Registry records.
- Pretty JSON of `lifecycle` for lifecycle records.
- Pretty JSON containing parse error details and raw text for parse-error records.

The list uses normalized metadata, but it does not display payload summaries.

## Request Correlation

While Debug is enabled, keep an in-memory request map keyed by `requestId`.

For each outbound request, store:

- method
- project ID
- extracted session IDs
- timestamp

For inbound response and error envelopes:

- Inherit method, project ID, and session IDs from the original request when the inbound envelope does not include them.
- Compute `durationMs` from the stored request timestamp.
- Keep the request map entry until response/error arrives, or until Debug is cleared/disabled.

This makes single-session filtering show complete request/response/error pairs even when the response payload omits `sessionId`.

## Session ID Extraction

Session IDs are discovered only from captured records.

Extract session IDs from common Registry shapes, including:

- top-level `sessionId`
- `payload.sessionId`
- `payload.session.sessionId`
- `payload.turn.sessionId`
- arrays such as `payload.sessions[].sessionId`
- request payloads such as `session.send`, `session.read`, `session.cancel`, `session.archive`, `session.setConfig`, and related session methods

The extractor should be generic enough to walk plain objects and arrays, but bounded to avoid excessive work on huge payloads. It should run only when Debug is enabled.

Session selector behavior:

- Always include `All`.
- Add session IDs as they are discovered.
- Single-session mode shows records explicitly mapped to that session.
- Single-session mode hides multi-session records by default.
- `Include multi-session records` allows records containing the selected session plus other session IDs.

## Settings Behavior

Add a `Debug` setting in Settings.

The row contains:

- A checkbox or switch for Debug capture.
- An `Open` button enabled when Debug is on.

Behavior:

- Turning Debug on starts capture and opens the floating panel.
- Turning Debug off stops capture, clears all records, clears request correlation, and hides the floating panel.
- Closing the floating panel hides only the panel. Capture continues.
- `Open` shows the panel again without changing capture.
- `Clear` in the panel clears records and request correlation but leaves Debug capture on.

The Debug setting is persisted as a global preference. Debug records, panel position, and panel size are not persisted.

## Floating Panel

The panel appears only on desktop layout.

Behavior:

- It floats above the workspace and does not reserve layout space.
- It does not modify the main sidebar, activity bar, chat, file, or git layout.
- It has a header with title, record count, Clear, and close controls.
- It can be dragged by the header.
- It can be resized.
- Position and size are clamped to the viewport during interaction.
- Initial position and size use a fixed desktop default each page lifecycle.

Mobile behavior:

- The Debug setting can remain visible.
- The floating panel is not rendered in narrow/mobile layout.
- Capture may remain enabled if the user toggles Debug, but there is no mobile inspection surface in this design.

## Panel Layout

The panel has two panes:

- Left pane: virtualized record list.
- Right pane: selected record detail.

Left list:

- Uses `react-virtuoso`, matching the existing app dependency.
- Shows one row per record.
- Shows time, direction, phase/type, method, request ID, duration, project ID, and session IDs.
- Does not show payload summaries.
- Supports `All` and discovered-session filtering.
- Supports `Include multi-session records`, default off.
- Automatically follows new records only while the user is already at the bottom.
- If the user scrolls up, new records do not force-scroll the list.
- When not following the latest, show a `Jump to latest` control.

Right detail:

- Shows pretty JSON for the selected record's envelope or lifecycle payload.
- Does not use a tree viewer.
- Does not truncate large strings.
- Provides a Copy button for the displayed JSON.
- Shows an empty state when no record is selected.

## Performance

The design accepts unbounded in-memory capture while Debug is enabled, but keeps rendering bounded:

- Capture arrays may grow until Debug is disabled or cleared.
- List rendering must be virtualized.
- Expensive metadata extraction and JSON formatting run only while Debug is enabled.
- Pretty JSON for the right pane should be computed only for the selected record.
- Filtering should operate on normalized metadata, not repeatedly parse raw JSON.

This is a local diagnostic tool. If future use requires very long captures, export or bounded retention can be designed separately.

## Security And Privacy

Normal Registry payloads are displayed without automatic redaction.

Exception:

- `connect.init` is not captured at all, so its token is not stored or displayed.

Debug records are memory-only and are cleared when Debug is disabled. They are not written to IndexedDB, localStorage, or server storage.

## Implementation Shape

Expected files:

- `app/web/src/services/registryClient.ts`
- `app/web/src/services/registryRepository.ts` if constructor wiring needs observer injection
- `app/web/src/services/registryWorkspaceService.ts` if the app needs to pass debug observer state through the service boundary
- `app/web/src/main.tsx`
- `app/web/src/styles.css`
- `app/web/src/services/workspacePersistence.ts`
- New focused web debug modules if keeping `main.tsx` smaller is practical
- Existing or new tests under `app/__tests__`

Preferred structure:

- Keep low-level capture hooks in `RegistryClient`.
- Keep debug record storage and UI state in the React app layer.
- Use a small pure helper module for session ID extraction, record normalization, and filtering.
- Use a dedicated `RegistryDebugPanel` component if the implementation would otherwise make `main.tsx` noticeably larger.

## Test Plan

Add source or unit tests for capture behavior:

- `RegistryClient` records outbound requests when Debug is enabled.
- `RegistryClient` does not record `connect.init`.
- `RegistryClient` records inbound responses, errors, events, and parse failures.
- `RegistryClient` records lifecycle start/open/close/error records.
- Debug disabled avoids record construction and extraction.
- Response/error records inherit method, project ID, session IDs, and duration through `requestId`.

Add tests for record helpers:

- Session IDs are extracted from top-level, payload, nested session, turn, and sessions-array shapes.
- Multi-session records are marked.
- Single-session filtering hides multi-session records by default.
- `Include multi-session records` includes records that contain the selected session among other sessions.

Add UI/source tests for settings and panel behavior:

- Settings includes `Debug` switch and `Open` button.
- Debug preference persists globally.
- Turning Debug off clears records.
- Closing the panel does not disable capture.
- Clear clears records while keeping Debug enabled.
- Desktop renders the floating panel when Debug is enabled and open.
- Mobile/narrow layout does not render the floating panel.
- The record list uses `react-virtuoso`.
- The detail pane renders pretty JSON for the selected envelope.
- The list has a `Jump to latest` path when auto-follow is paused.

Manual verification after implementation:

- Enable Debug, perform file/git/session actions, and confirm records appear.
- Confirm `connect.init` is absent.
- Select a session ID and confirm request/response pairs remain visible through inherited `requestId` metadata.
- Confirm multi-session records are hidden by default in single-session view and appear when the include switch is enabled.
- Close and reopen the panel from Settings without losing records.
- Clear records while capture continues.
- Disable Debug and confirm records are cleared and capture stops.

## Open Decisions

None. The capture boundary, retention behavior, filtering behavior, panel behavior, and first-version exclusions are decided.
