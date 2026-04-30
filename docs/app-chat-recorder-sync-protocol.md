# Registry <-> App Chat Protocol (Current)

## 1. Scope

This document describes the **current** app chat protocol between web app client (`role=client`) and registry/hub runtime.
It is based on live code paths in:

- `server/internal/registry/server.go`
- `server/internal/hub/reporter.go`
- `server/internal/hub/client/session_recorder.go`
- `app/web/src/services/registryClient.ts`
- `app/web/src/services/registryRepository.ts`
- `app/web/src/main.tsx`

This is the protocol used by app chat today. It does not rely on legacy chat payload formats.

## 2. Transport And Envelope

- Transport: WebSocket
- Envelope:

```json
{
  "requestId": 1,
  "type": "request|response|error|event",
  "method": "...",
  "projectId": "hubId:projectName",
  "payload": {}
}
```

Rules:

- `requestId` is required for `request/response/error` messages.
- `event` messages have no `requestId`.
- App chat listens to `event.method = session.updated | session.message`.

## 3. Handshake (`connect.init`)

App connects with `role=client` and protocol `2.2`.

Request:

```json
{
  "requestId": 1,
  "type": "request",
  "method": "connect.init",
  "payload": {
    "clientName": "wheelmaker-web",
    "clientVersion": "0.1.0",
    "protocolVersion": "2.2",
    "role": "client",
    "token": "<token-or-empty>"
  }
}
```

Notes:

- Registry validates `protocolVersion` exactly.
- If registry token is configured, `payload.token` must match.

## 4. App Chat Request APIs

App chat write/read path is `session.*`.

### 4.1 `session.list`

Purpose: load session sidebar when entering chat page.

Request:

```json
{
  "requestId": 10,
  "type": "request",
  "method": "session.list",
  "projectId": "local-hub:WheelMaker",
  "payload": {}
}
```

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

### 4.2 `session.read`

Purpose: pull prompt/turn delta from checkpoint `(promptIndex, turnIndex)`.

Request payload:

```json
{
  "sessionId": "sess-1",
  "promptIndex": 12,
  "turnIndex": 3
}
```

Response payload (wire shape from server):

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

Server filter rule:

- skip `promptIndex < request.promptIndex`
- for same prompt: skip `turnIndex <= request.turnIndex`

Identity on wire:

- `(sessionId, promptIndex, turnIndex)`

### 4.3 `session.new`

Purpose: create a session before first send when no session selected.

Request payload:

```json
{
  "agentType": "codex",
  "title": "optional"
}
```

Notes:

- `agentType` is required in current server path.

### 4.4 `session.send`

Purpose: send text/image blocks into selected session.

Request payload:

```json
{
  "sessionId": "sess-1",
  "text": "hello",
  "blocks": [
    {"type": "text", "text": "hello"},
    {"type": "image", "mimeType": "image/png", "data": "..."}
  ]
}
```

Response payload:

```json
{
  "ok": true,
  "sessionId": "sess-1"
}
```

### 4.5 `session.markRead`

Registry forwards `session.markRead`, but current hub session handler does not implement it.
Do not depend on it in app chat flow.

## 5. Realtime Events

Hub publishes to registry:

- `registry.session.updated`
- `registry.session.message`

Registry fans out to app clients as:

- `event.method = session.updated`
- `event.method = session.message`

### 5.1 `session.updated`

Envelope:

```json
{
  "type": "event",
  "method": "session.updated",
  "projectId": "local-hub:WheelMaker",
  "payload": {
    "session": {
      "sessionId": "sess-1",
      "title": "Fix sync bug",
      "updatedAt": "2026-04-30T10:00:00Z",
      "agentType": "codex"
    }
  }
}
```

Client behavior:

- merge/update session summary in sidebar
- no message body update here

### 5.2 `session.message`

Envelope:

```json
{
  "type": "event",
  "method": "session.message",
  "projectId": "local-hub:WheelMaker",
  "payload": {
    "sessionId": "sess-1",
    "promptIndex": 12,
    "turnIndex": 4,
    "content": "{\"method\":\"agent_message_chunk\",\"param\":{\"text\":\"...\"}}"
  }
}
```

Client message key:

- `messageId = sessionId:promptIndex:turnIndex`

Overwrite rule:

- same key => overwrite with latest content
- used for same-turn incremental merges (message/tool updates)

## 6. IM Message Content (`payload.content`)

`session.message.payload.content` is a JSON string:

```json
{
  "method": "...",
  "param": {...}
}
```

Current methods emitted by recorder path:

- `prompt_request`
- `prompt_done`
- `user_message_chunk`
- `agent_message_chunk`
- `agent_thought_chunk`
- `tool_call`
- `agent_plan`
- `system`

App decode mapping (current main.tsx):

- `prompt_request` -> `role=user`, `kind=message`
- `prompt_done` -> `role=system`, `kind=prompt_result`
- `user_message_chunk` -> `role=user`, `kind=message`, `status=streaming`
- `agent_message_chunk` -> `role=assistant`, `kind=message`, `status=streaming`
- `agent_thought_chunk` -> `role=assistant`, `kind=thought`, `status=streaming`
- `tool_call` -> `role=system`, `kind=tool`, `status` from `param.status`
- `agent_plan` -> `role=assistant`, `kind=thought`, `status=streaming`
- `system` -> `role=system`, `kind=message`

Fallback:

- if JSON parse fails, treat raw `content` as message text

## 7. App Sync Model

Per-session cursor:

- `syncIndex` (promptIndex)
- `syncSubIndex` (turnIndex)

Update logic:

- realtime `session.message`: upsert by `messageId`, then advance cursor if incoming is newer
- `session.read`: pull checkpoint delta and replace from affected prompt boundary

## 8. Pull Triggers In Current Frontend

Current app chat triggers are:

1. Enter chat tab (`tab` changes to `chat`): call `session.list` only.
2. User clicks/opens a session: call `session.read` with current cursor.
3. Reconnect succeeds and user is still on chat tab with selected session:
   - silent reconnect path: call `session.read` (incremental) for selected session.
4. Full reconnect while on chat tab:
   - call `session.list`; message body is not auto-read until session selection/read path runs.

Current app does not proactively pull all session messages on project switch.

## 9. Write Flow In Current Frontend

- If no selected session:
  - call `session.new(agentType)`
  - then call `session.send(sessionId, text/blocks)`
- If selected session exists:
  - call `session.send` directly

`chat.send` exists in registry/hub routing but app chat UI currently uses `session.send` path.

## 10. Compatibility Notes

- This document is the app chat contract as implemented now.
- Legacy `chat.session.*` style formats are not part of this flow.
- For any protocol change, update both:
  - backend recorder/event payload
  - frontend decode and merge logic
