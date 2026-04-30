# App Chat Recorder Sync Protocol

## 1. Scope

This document defines app chat sync behavior based on the current `SessionRecorder` output.
App does not support legacy chat/session payload formats.
Only the new recorder payload is accepted.

## 2. Session APIs

### 2.1 `session.list`

Used to list session summaries when entering chat page.

Response payload:

```json
{
  "sessions": [
    {
      "sessionId": "sess-1",
      "title": "Fix sync bug",
      "updatedAt": "2026-04-30T10:00:00Z",
      "agentType": "codex"
    }
  ]
}
```

### 2.2 `session.read`

Used to pull updates for one selected session with checkpoint `(promptIndex, turnIndex)`.

Request payload:

```json
{
  "sessionId": "sess-1",
  "promptIndex": 12,
  "turnIndex": 3
}
```

Response payload:

```json
{
  "session": {
    "sessionId": "sess-1",
    "title": "Fix sync bug",
    "updatedAt": "2026-04-30T10:00:00Z",
    "agentType": "codex"
  },
  "messages": [
    {
      "sessionId": "sess-1",
      "promptIndex": 12,
      "turnIndex": 4,
      "content": "{\"method\":\"agent_message_chunk\",\"param\":{\"text\":\"...\"}}"
    }
  ]
}
```

Server filter semantics:

- skip all records with `promptIndex < request.promptIndex`
- for the same prompt, skip all records with `turnIndex <= request.turnIndex`
- each record identity is `(sessionId, promptIndex, turnIndex)`

## 3. Realtime Event

Event name: `session.message` (registry event from `registry.session.message`).

Event payload:

```json
{
  "sessionId": "sess-1",
  "promptIndex": 12,
  "turnIndex": 4,
  "content": "{\"method\":\"agent_message_chunk\",\"param\":{\"text\":\"...\"}}"
}
```

App must parse `content` as recorder IM turn message:

```json
{
  "method": "agent_message_chunk",
  "param": { "...": "..." }
}
```

## 4. Local Sync Model

### 4.1 Identity and overwrite rule

- local message key: `sessionId:promptIndex:turnIndex`
- if same key arrives again, overwrite local message with latest content
- this handles same-turn multi-update merge behavior

### 4.2 Cursor

Per session cursor:

- `cursor.promptIndex`
- `cursor.turnIndex`

Cursor always tracks max seen `(promptIndex, turnIndex)` for that session.

### 4.3 Store

Maintain in-memory per-session message store:

- `sessionMessages[sessionId] = ordered messages`

Ordered by:

1. `promptIndex` ascending
2. `turnIndex` ascending

## 5. Pull Triggers

App may actively pull only in these cases:

1. User enters chat page: call `session.list` (list only, no automatic message read).
2. User clicks a session: call `session.read` with that session cursor.
3. Reconnect succeeds and user is currently on chat page with a selected session: call `session.read` for that selected session with cursor.

App should not proactively pull sessions/messages on project switch.

## 6. Render Mapping (from `content.method`)

Recommended mapping:

- `prompt_request` -> `role=user`, `kind=message`
- `user_message_chunk` -> `role=user`, `kind=message`
- `agent_message_chunk` -> `role=assistant`, `kind=message`
- `agent_thought_chunk` -> `role=assistant`, `kind=thought`
- `agent_plan` -> `role=assistant`, `kind=thought`
- `tool_call` -> `role=system`, `kind=tool`
- `prompt_done` -> `role=system`, `kind=prompt_result`
- `system` -> `role=system`, `kind=message`

If JSON parse fails, treat raw `content` as plain text assistant message.

