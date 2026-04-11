# App Project Session Design

**Date:** 2026-04-12
**Scope:** `server/internal/hub/`, `server/internal/im/`, `server/internal/registry/`, `app/web/`
**Approach:** Introduce a project-level session view and aggregated message history layer keyed by `sessionId`, keep IM channels ACP-based, and move app UI from app-private chat state to project-wide session state.

---

## Goals

1. Show the full project session list in app, keyed by `sessionId` instead of app-private `chatId`.
2. Allow app and Feishu to coexist for the same project and the same session runtime.
3. Add server-side session message history so reopening app does not lose unread messages or recent conversation context.
4. Store history as an app-visible aggregated message view, not as raw ACP event logs.
5. Keep app and Feishu on the current ACP and IM flow instead of replacing them with a separate messaging architecture.
6. Preserve router watcher fanout semantics so app can observe all project sessions by default.

---

## Non-Goals

1. No raw event-sourcing or full ACP frame persistence in this pass.
2. No per-user or per-device durable unread model in this pass.
3. No requirement to reconstruct every historical Feishu message exactly as sent.
4. No redesign of ACP transport, agent lifecycle, or permission protocol.
5. No requirement that history store every streaming chunk as its own durable row.

---

## Current Problems

The current implementation is not a project session system. It is an app-private chat overlay.

1. App session state is keyed by `chatId` and owned by `server/internal/im/app/app.go`, so the app can only see chats known to the app channel.
2. `server/internal/hub/hub.go` currently registers either Feishu or app for a project, which blocks the required coexistence model.
3. `server/internal/im/router.go` supports multi-channel binding and watcher fanout, but the app flow does not consume it as a project-wide session view.
4. `server/internal/hub/client/sqlite_store.go` persists session metadata only. It does not persist readable message history.
5. The current app web UI in `app/web/src/main.tsx` treats session selection, message reading, and message sending as `chat.*` operations tied to app-private state.
6. `im.NewMemoryHistoryStore()` is runtime-only. Reopening the app cannot rely on it for durable unread messages or history replay.

---

## Product Decisions Confirmed

1. The canonical session key is `sessionId`.
2. The app must show the complete session list for the project, not only app-originated chats.
3. Creating a new session should happen explicitly through a create action, returning `sessionId` immediately.
4. The app should not persist a separate `chatId` model. If the app needs a routing identity, it should use `sessionId` directly.
5. Opening an existing session in app does not require exact historical reconstruction of every Feishu fragment.
6. The app should observe all project sessions by default, consistent with watcher semantics already supported by the router.
7. Unread counts are secondary, but unread messages themselves must not disappear when reopening the app.
8. Server-side history should store an app-visible aggregated message view.
9. Fine-grained outbound IM updates can remain detailed for Feishu and app realtime delivery even if durable history stores a single aggregated row.

---

## Target Architecture

The new model separates runtime execution, routing, and durable read models.

### Runtime Layer

`client.Session` remains the runtime owner for:

1. agent instance lifecycle
2. ACP session lifecycle
3. prompt execution
4. permission flow
5. session status and agent metadata

This layer remains the business runtime and does not become the durable history source by itself.

### Routing Layer

`im.Router` remains intentionally narrow:

1. bind and unbind chats to `sessionId`
2. watcher fanout for outbound updates
3. source-target routing for permission requests and replies
4. channel delivery orchestration

The router must not own:

1. project session list truth
2. durable history aggregation
3. app-private chat session state
4. protocol-specific session read models

### Session View Layer

Introduce a project-level `SessionViewService` inside the server domain. This layer owns:

1. project session list projection
2. durable aggregated message history keyed by `sessionId`
3. session preview and last activity projection
4. unread calculation inputs
5. registry-facing read APIs for app clients

This is the primary truth source for app session browsing.

### Channel Layer

`im/app` and `im/feishu` become channel adapters. They remain ACP and IM aware, but they no longer own their own durable session model.

Channel responsibilities are:

1. convert inbound channel input into session-level semantic input
2. receive outbound realtime updates from router
3. provide channel metadata needed by session view projection

They do not own project-level session list persistence.

---

## Persistence Model

### Existing Session Metadata

The current `SessionRecord` concept already exists in `server/internal/hub/client/sqlite_store.go` and should be extended rather than replaced.

The durable session metadata model should support:

1. `sessionId`
2. `projectName`
3. `status`
4. `agent`
5. `title`
6. `lastMessagePreview`
7. `lastMessageAt`
8. `messageCount`
9. `createdAt`
10. `lastActiveAt`

The current persisted fields such as `acp_session_id`, `agents_json`, and existing timestamps remain valid inputs to this model.

### New Message History Store

Add a durable `SessionMessageRecord` store keyed by `messageId` and `sessionId`.

Recommended fields:

1. `messageId`
2. `sessionId`
3. `role`
4. `kind`
5. `body`
6. `status`
7. `createdAt`
8. `updatedAt`
9. `sourceChannel`
10. `sourceChatId`
11. `requestId`
12. `aggregateKey`

This store is not a raw ACP event log. It is the app-visible historical message view.

### Watcher State

Watcher state is primarily runtime-only in this pass.

It may exist in memory as:

1. which channels are watching which `sessionId`
2. which session the current app client has opened
3. last seen message markers for temporary unread calculations

This state does not need to be fully durable in the first version.

---

## History Aggregation Model

Durable history and realtime channel delivery are intentionally different products.

1. Realtime delivery may remain fine-grained.
2. Durable history stores aggregated stable messages.

### Aggregation Rules

Recommended aggregated history kinds are:

1. `user`
2. `assistant`
3. `thought`
4. `tool`
5. `permission`
6. `system`
7. `prompt_result`

Examples:

1. A long assistant streaming reply may emit many chunks to Feishu or app, but durable history stores one assistant message row.
2. A tool call may emit multiple progress updates to realtime channels, but durable history stores one tool row with the latest stable summary.
3. A permission request is stored once as pending and later updated to a resolved final state.

### Flush Strategy

Use an in-memory `HistoryAggregator` and delay durable writes until a stable boundary is reached.

Recommended flush boundaries:

1. message kind changes
2. prompt finishes
3. tool call completes or changes semantic phase
4. permission request is resolved
5. session is suspended, persisted, or closed
6. a short idle timeout expires after streaming updates

### Immediate Writes

These can be written immediately:

1. user messages
2. stable system messages
3. explicit session creation events if represented as system history

### Failure Tradeoff

If the process crashes during an in-memory aggregation window, the latest partial streaming text may be lost. This is acceptable for the first pass.

Do not introduce raw per-chunk durability just to avoid this edge case.

---

## History Aggregator Placement

The `HistoryAggregator` should be an internal component of `SessionViewService`.

It must not live in `im.Router` because that would overload routing with:

1. semantic message understanding
2. history flush policy
3. preview and message count projection
4. persistence orchestration

It should also not be embedded directly into `client.Session` as the durable read model owner, because `Session` should remain focused on runtime business execution.

### Recommended Event Flow

`Session`, runtime IM adapters, and channel-facing logic should emit semantic session events such as:

1. `SessionCreated`
2. `UserMessageAccepted`
3. `AssistantMessageChunk`
4. `AssistantThoughtChunk`
5. `ToolCallUpdated`
6. `PermissionRequested`
7. `PermissionResolved`
8. `PromptFinished`
9. `SystemMessagePublished`
10. `SessionMetadataChanged`

`SessionViewService` consumes these events, updates in-memory aggregation state, flushes durable messages, and updates session list projections.

---

## IM Coexistence Model

App and Feishu must be able to coexist for the same project.

### Required Change

`server/internal/hub/hub.go` must stop treating Feishu and app as mutually exclusive project IM choices.

Instead:

1. register Feishu when configured
2. register app when app registry features are enabled
3. allow both to bind or watch the same `sessionId`

### Delivery Semantics

1. app can act as a watcher for all project sessions by default
2. Feishu remains a source-specific conversational channel
3. permission requests still route only to the source channel that initiated the interaction
4. watcher fanout semantics in `im.Router` remain valid and should be reused rather than replaced

---

## Registry and App Protocol Design

The current `chat.session.*` vocabulary should no longer be the primary protocol model for app session browsing.

### New Session-Centric Methods

Add registry client methods keyed by `sessionId`.

#### `session.list`

Purpose: return the full project session list for app sidebar rendering.

Response item fields should include:

1. `sessionId`
2. `title`
3. `preview`
4. `updatedAt`
5. `messageCount`
6. `unreadCount`
7. `agent`
8. `status`

#### `session.read`

Purpose: return session metadata and aggregated durable history for one `sessionId`.

#### `session.new`

Purpose: create a new empty session immediately and return `sessionId`.

This matches the confirmed product behavior that new session creation should happen before the first message send.

#### `session.send`

Purpose: send a message into an existing `sessionId`.

It replaces `chat.send` as the app-facing session-centric write API.

#### `session.markRead`

Purpose: advance the app-visible read marker for one `sessionId`.

This can remain lightweight in the first pass and may be client-scoped rather than globally durable.

### Event Design

Push events should be split by responsibility:

1. `session.updated`
   Used to update list ordering, preview text, timestamps, status, and unread state.
2. `session.message`
   Used to update the currently open message pane and non-open-session unread accumulation.

### Compatibility

Short-term compatibility can keep `chat.*` methods as thin adapters that call the new session view layer.

However, app UI should migrate to `session.*` as the canonical model.

---

## App Web State Model

The app web UI should stop modeling app session state as `chatId` state.

### State Renames

1. `chatSessions` -> `sessions`
2. `selectedChatId` -> `selectedSessionId`
3. `chatMessages` -> `sessionMessages`
4. `sendChatMessage()` -> `sendSessionMessage()`
5. `loadChatSessions()` -> `loadSessions()`
6. `loadChatSession()` -> `loadSession()`

### App Behavior

1. left sidebar renders the project-wide session list from `session.list`
2. opening a session loads durable aggregated history from `session.read`
3. current session realtime updates append into the active message pane
4. other sessions update preview, ordering, and unread state through `session.updated` and `session.message`
5. creating a session calls `session.new` before the first message is sent

### Not Required

The app does not need a separate durable `chatId` identity.

If a routing target needs a stable app-side identity, use `sessionId` directly.

---

## Unread Model

Unread counts are secondary. Message durability is primary.

### First-Pass Rules

1. unread messages must remain available because durable history exists on the server
2. unread count can be a lightweight derived field rather than a globally durable shared counter
3. app reopen must not lose messages just because local state reset

### Acceptable First Version

1. store durable history on the server
2. compute unread relative to a lightweight read marker
3. keep unread semantics app-oriented instead of global project state

---

## Migration Plan

### Phase 1: Build Durable Session View and History

1. extend current session metadata persistence
2. add message history persistence tables
3. add `SessionViewService` and `HistoryAggregator`
4. project runtime events into durable session and history state

Result: the server has a real project session read model before app protocol migration.

### Phase 2: Add Session-Centric Registry APIs and App UI Migration

1. add `session.list`, `session.read`, `session.new`, `session.send`, and `session.markRead`
2. emit `session.updated` and `session.message`
3. migrate app web state from `chatId` to `sessionId`

Result: app becomes a project session browser and chat client.

### Phase 3: Remove App-Private Chat Ownership

1. remove durable session ownership from `im/app`
2. keep `im/app` as a channel adapter only
3. leave optional compatibility shims only if needed during rollout

Result: one project session model shared by app and Feishu.

---

## Risks

1. Aggregation boundaries may be defined too loosely and merge semantically distinct events.
2. History projection may drift from realtime delivery semantics if aggregation rules are underspecified.
3. Session list projection and message history updates may race unless updated through one projection path.
4. App and Feishu coexistence may regress if mutual-exclusion assumptions remain in project startup logic.
5. Permission and tool-call history may become misleading if final state updates are not flushed consistently.

---

## Testing Strategy

### Store and Aggregator Tests

Cover:

1. assistant chunk aggregation into one durable assistant message
2. thought chunk aggregation into one durable thought message
3. tool status update aggregation into one durable tool message
4. permission request creation and resolution state transitions
5. flush on kind change, prompt finish, and idle timeout

### Session View Service Tests

Cover:

1. session creation appears in `session.list`
2. preview and timestamps update when history flushes
3. message count stays aligned with durable history rows
4. unread accumulation for non-open sessions behaves correctly

### Router and Channel Integration Tests

Cover:

1. app and Feishu register together for one project
2. watcher fanout still reaches app observers
3. reply routing still targets the source channel correctly
4. permission requests still route only to the originating channel

### Registry and App Protocol Tests

Cover:

1. `session.list`
2. `session.read`
3. `session.new`
4. `session.send`
5. `session.markRead`
6. `session.updated`
7. `session.message`

---

## Implementation Constraints

1. Keep comments and identifiers in English.
2. Do not reintroduce app-private durable chat state as a shortcut.
3. Do not overload `im.Router` with persistence or history ownership.
4. Keep ACP transport and runtime session behavior intact unless a protocol gap requires a focused change.
5. Preserve current watcher semantics instead of inventing a second fanout model.

---

## Summary

The app session feature should be implemented as a project-level session read model keyed by `sessionId`, backed by durable aggregated history, with app and Feishu coexisting as IM channels on top of the same runtime sessions.

The essential refactor is not replacing ACP or IM. It is moving durable session truth out of app-private channel state and into a dedicated session view layer that serves registry and app clients.