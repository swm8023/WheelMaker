# Registry <-> App Chat Protocol (Current)

> The current implementation uses a session-global `turnIndex` and binary session turn files. `promptIndex` remains in some historical notes only and is no longer part of the realtime or read protocol. See [session-turn-binary-history-design.zh-CN.md](./session-turn-binary-history-design.zh-CN.md) for the latest persistence design.

## 1. Scope

This document describes the **current** app chat protocol between web app client (`role=client`) and registry/hub runtime.
It is based on live code paths in:

- `server/internal/registry/server.go`
- `server/internal/hub/reporter.go`
- `server/internal/hub/client/session_recorder.go`
- `server/internal/hub/client/session.go`
- `server/internal/hub/client/client.go`
- `app/web/src/services/registryClient.ts`
- `app/web/src/services/registryRepository.ts`
- `app/web/src/types/registry.ts`
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

- `requestId` is required for `request`/`response`/`error` messages, typed as positive integer (`>= 1`), monotonically increasing per connection without reuse.
- `event` messages have no `requestId`.
- App chat listens to `event.method = session.updated | session.message`.
- `projectId` format is `hubId:projectName`, required for business methods.

## 3. Handshake (`connect.init`)

App connects with `role=client` and protocol version `2.2`.

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

- Registry validates `protocolVersion` exactly, must be `"2.2"`.
- If registry token is configured, `payload.token` must be present and match.
- Auth failure returns `UNAUTHORIZED`, followed by immediate disconnect.
- Success response returns `{ok, principal, serverInfo, features, hashAlgorithms}`.

## 4. App Chat Request APIs

App chat read/write paths use the `session.*` method family. All methods are forwarded by registry to the target hub based on `projectId`, with hub responses returned as-is.

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

Response payload (`RegistrySessionSummary`):

```json
{
  "sessions": [
    {
      "sessionId": "sess-1",
      "title": "Fix sync bug",
      "preview": "latest message preview...",
      "updatedAt": "2026-04-30T10:00:00Z",
      "messageCount": 42,
      "unreadCount": 3,
      "agentType": "codex",
      "configOptions": [
        {
          "id": "model",
          "name": "Model",
          "currentValue": "claude-sonnet-4-6",
          "options": [{"value": "claude-sonnet-4-6", "name": "Sonnet 4.6"}]
        }
      ]
    }
  ]
}
```

Field description:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `sessionId` | `string` | yes | Session unique identifier |
| `title` | `string` | yes | Session title (defaults to sessionId) |
| `preview` | `string` | yes | Latest message preview text |
| `updatedAt` | `string` | yes | Last active time (RFC3339) |
| `messageCount` | `number` | yes | Total message count (prompt count) |
| `unreadCount` | `number` | no | Unread message count |
| `agentType` | `string` | no | Agent type |
| `configOptions` | `ConfigOption[]` | no | Session config options list |

### 4.2 `session.read`

Purpose: pull incremental prompts/turns from checkpoint `(promptIndex, turnIndex)`.

Request payload:

```json
{
  "sessionId": "sess-1",
  "promptIndex": 12,
  "turnIndex": 3
}
```

- `promptIndex` and `turnIndex` are optional. Omit or pass `0` for full message retrieval on first read.
- Passing `promptIndex > 0` or `turnIndex > 0` performs incremental read.
- Passing `promptIndex=N, turnIndex=0` reads every turn in prompt N, used for prompt-local refresh.

Response payload (server wire format):

```json
{
  "session": {
    "sessionId": "sess-1",
    "title": "Fix sync bug",
    "updatedAt": "2026-04-30T10:00:00Z",
    "agentType": "codex",
    "configOptions": [...]
  },
  "messages": [
    {
      "sessionId": "sess-1",
      "promptIndex": 12,
      "turnIndex": 4,
      "finished": true,
      "content": "{\"method\":\"agent_message_chunk\",\"param\":{\"text\":\"...\"}}"
    }
  ]
}
```

Server filter rules:

- Skip prompts with `promptIndex < request.promptIndex`.
- Within the same prompt, skip turns with `turnIndex <= request.turnIndex`.
- `session.read(P, T)` returns the delta after the client's finished cursor, not a full prompt snapshot.
- `prompt_done` is a normal turn and advances the finished cursor.

> **Frontend processing**: Messages from `session.read` are decoded by `decodeSessionMessageFromEventPayload`, then grouped by `promptIndex` into a `prompts` array. The final return is `{session, prompts, messages}` for UI consumption.

### 4.3 `session.new`

Purpose: create a new session, typically called before first send when no session is selected.

Request payload:

```json
{
  "agentType": "codex",
  "title": "optional title"
}
```

- `agentType`: required, non-empty string.
- `title`: optional, session title.

Response payload:

```json
{
  "ok": true,
  "session": {
    "sessionId": "sess-1",
    "title": "optional title",
    "updatedAt": "2026-04-30T10:00:00Z",
    "agentType": "codex",
    "configOptions": [
      {
        "id": "model",
        "name": "Model",
        "description": "Select AI model",
        "category": "Model",
        "type": "enum",
        "currentValue": "claude-sonnet-4-6",
        "options": [
          {"value": "claude-sonnet-4-6", "name": "Sonnet 4.6"}
        ]
      }
    ]
  }
}
```

> **Implementation detail**: Server-side `CreateSession` performs these steps:
> 1. Agent `Initialize` + `SessionNew` → get initial configOptions
> 2. Apply persisted agent preference overrides
> 3. Wire up session state, persist to storage
> 4. Publish `registry.session.updated` event
>
> Frontend `createChatSession` immediately sets `selectedChatId`, initializes message store and sync cursors upon completion.

### 4.4 `session.send`

Purpose: send text/image blocks into the selected session.

Request payload:

```json
{
  "sessionId": "sess-1",
  "text": "hello",
  "blocks": [
    {"type": "text", "text": "hello"},
    {"type": "image", "mimeType": "image/png", "data": "<base64>"}
  ]
}
```

- `sessionId`: required.
- `text`: optional, plain text string (semantically equivalent to a text block in `blocks`).
- `blocks`: optional, `RegistryChatContentBlock[]`, each item `{type, text?, mimeType?, data?}`.
- `type` values: `"text"` | `"image"`.

Response payload:

```json
{
  "ok": true,
  "sessionId": "sess-1"
}
```

> **Relation to `chat.send`**: `chat.send` exists in registry/hub routing but follows the IM app channel path (`server/internal/im/app/app.go`) for slash command processing (`/new`, `/list`, `/model`, etc.). App chat UI uses the `session.send` path, not `chat.send`.

### 4.5 `session.setConfig`

Purpose: set a session config option (e.g. model switch).

Request payload:

```json
{
  "sessionId": "sess-1",
  "configId": "model",
  "value": "claude-opus-4-6"
}
```

- `sessionId`: required.
- `configId`: required, non-empty string, config item ID (e.g. `"model"`, `"mode"`, `"thought_level"`).
- `value`: required, non-empty string, target value.

Response payload:

```json
{
  "ok": true,
  "sessionId": "sess-1",
  "configOptions": [
    {
      "id": "model",
      "name": "Model",
      "description": "Select AI model",
      "category": "Model",
      "type": "enum",
      "currentValue": "claude-opus-4-6",
      "options": [
        {"value": "claude-sonnet-4-6", "name": "Sonnet 4.6"},
        {"value": "claude-opus-4-6", "name": "Opus 4.6"}
      ]
    }
  ]
}
```

> **Implementation detail**:
> 1. Server-side `SetConfigOption` holds `promptMu` lock to serialize with prompt processing.
> 2. Lazy agent instance initialization (`ensureInstance` + `ensureReady`).
> 3. Dispatches to agent via ACP method `session/set_config_option`.
> 4. Merges returned configOptions into session state, persists agent preferences.
> 5. Frontend merges returned `configOptions` into local `chatSessions` state via `applyChatSessionConfigOptions`.

### 4.6 ConfigOption Type Definition

```typescript
interface RegistrySessionConfigOptionValue {
  value: string;
  name?: string;
  description?: string;
}

interface RegistrySessionConfigOption {
  id: string;
  name?: string;
  description?: string;
  category?: string;
  type?: string;
  currentValue?: string;
  options?: RegistrySessionConfigOptionValue[];
}
```

- `type` example values: `"enum"`, `"string"`, `"boolean"`.
- When `options` is non-empty, frontend renders a dropdown selector; otherwise renders a text input.
- Known config IDs: `"model"`, `"mode"`, `"thought_level"`.

### 4.7 `session.markRead`

Registry forwards `session.markRead`, but the current hub session handler **does not implement** this method. Do not depend on it in app chat flow. Calling it will return an unsupported error.

## 5. Realtime Events

Hub publishes to registry via:

- `registry.session.updated`
- `registry.session.message`

Registry fans out to app clients scoped by project:

- `event.method = session.updated`
- `event.method = session.message`

### 5.1 `session.updated`

Event envelope:

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
      "agentType": "codex",
      "configOptions": [...]
    }
  }
}
```

Server-side `publishSessionUpdated` sends a `sessionViewSummary` struct:

| Field | Source |
|-------|--------|
| `sessionId` | session ID |
| `title` | `firstNonEmpty(title, sessionId)` |
| `updatedAt` | `lastActiveAt.UTC().Format(time.RFC3339)` |
| `agentType` | agentType at creation |
| `configOptions` | from persisted config in `SessionRecord.AgentJSON` |

Client behavior:

- Merge/update session summary in sidebar via `mergeChatSession(prev, payload.session)`.
- Does **not** update message body here.

### 5.2 `session.message`

Event envelope:

```json
{
  "type": "event",
  "method": "session.message",
  "projectId": "local-hub:WheelMaker",
  "payload": {
    "sessionId": "sess-1",
    "promptIndex": 12,
    "turnIndex": 4,
    "finished": true,
    "content": "{\"method\":\"agent_message_chunk\",\"param\":{\"text\":\"...\"}}"
  }
}
```

`RegistryChatMessageEventPayload` (TypeScript type):

```typescript
interface RegistrySessionMessageEventPayload {
  sessionId: string;
  promptIndex: number;
  turnIndex: number;
  finished?: boolean;
  content: string;  // JSON string
}
```

> **`content` string** is generated by server-side `buildIMContentJSON`, formatted as `json.Marshal(IMTurnMessage{Method, Param})`.
> **`finished` is turn envelope state**: `finished=true` means the turn is sealed and cacheable. `content.param.text` is always the full text snapshot for the current turn.

Client message key:

- `messageId = sessionId:promptIndex:turnIndex`

Overwrite rule:

- Same key → overwrite with latest content (same-turn incremental merge).

Full client event processing flow:

1. Validate `sessionId` non-empty, `promptIndex > 0`, `turnIndex > 0`.
2. Generate `messageId`.
3. Update `chatSessions` sidebar (preview, time, title).
4. Dedup via `(promptIndex, turnIndex)` cursors; only `finished=true` turns advance the read cursor.
5. Upsert message into `chatMessageStoreRef`.
6. If `sessionId === chatSelectedIdRef.current` (currently viewed session), update `chatMessages` to trigger UI refresh.
7. If an incoming turn skips a prompt/turn, or starts the next prompt before the previous prompt has a local `prompt_done`, call `session.read` from the last finished cursor.

## 6. IM Message Content (`payload.content`)

### 6.1 Content Format

`session.message.payload.content` is a JSON string:

```json
{
  "method": "...",
  "param": {...}
}
```

`IMTurnMessage` struct (Go):

```go
type IMTurnMessage struct {
    Method string          `json:"method"`
    Param  json.RawMessage `json:"param,omitempty"`
}
```

### 6.2 Current Method List

All content methods emitted by the recorder path:

| Wire Method | Go Constant | ACP Source | Meaning |
|-------------|-------------|------------|---------|
| `prompt_request` | `IMMethodPromptRequest` | `session/prompt` (params) | User initiated a prompt |
| `prompt_done` | `IMMethodPromptDone` | `session/prompt` (result) | Prompt completed |
| `user_message_chunk` | `SessionUpdateUserMessageChunk` | `user_message_chunk` | User message streaming text |
| `agent_message_chunk` | `IMMethodAgentMessage` | `agent_message_chunk` | Agent message streaming text |
| `agent_thought_chunk` | `IMMethodAgentThought` | `agent_thought_chunk` | Agent thought streaming text |
| `tool_call` | `IMMethodToolCall` | `tool_call` / `tool_call_update` | Tool call and its updates |
| `plan` | `IMMethodAgentPlan` | `plan` | Agent plan |
| `system` | `IMMethodSystem` | System event | System message |

> **Notes**:
> - On the wire, `tool_call_update` is mapped to `tool_call` (same method, merged by `turnKey = ToolCallID`).
> - On the wire, `agent_plan` has method **`"plan"`** (not `"agent_plan"`).

### 6.3 Parameter Structures Per Method

#### `prompt_request` params

```json
{
  "contentBlocks": [
    {"type": "text", "text": "hello"},
    {"type": "image", "mimeType": "image/png", "data": "<base64>"}
  ]
}
```

`contentBlocks` is `RegistrySessionContentBlock[]`, each item `{type, text?, mimeType?, data?}`.

#### `prompt_done` params

```json
{
  "stopReason": "end_turn"
}
```

`stopReason` values: `"end_turn"` | `"max_tokens"` | etc.

#### Text chunk params (`user_message_chunk` / `agent_message_chunk` / `agent_thought_chunk`)

```json
{
  "text": "partial response..."
}
```

Or `param` can be a plain string `"partial response..."` (backward compatible).

#### `tool_call` params

```json
{
  "cmd": "bash",
  "kind": "execute_command",
  "status": "running",
  "output": "stdout content..."
}
```

| Field | Description |
|-------|-------------|
| `cmd` | Command executed |
| `kind` | Tool type identifier |
| `status` | Status: `"running"` / `"done"` / `"error"` / `"need_action"` |
| `output` | Tool output |

Same tool call merges incremental updates (status/output changes) via `turnKey = params.Update.ToolCallID`.

#### `plan` params (agent_plan)

```json
{
  "content": "plan description",
  "status": "streaming"
}
```

**Note**: `param` on the Go side is `[]IMPlanResult` array, but on the wire it is a **single object** (depends on serialization). Frontend uses `extractTextFromIMParam` for unified handling.

#### `system` params

```json
{
  "text": "system notification message"
}
```

### 6.4 Frontend Decode Mapping (`decodeSessionMessageFromEventPayload`)

| `method` (within content JSON) | `role` | `kind` | `status` | text source |
|---|---|---|---|---|
| `prompt_request` | `user` | `message` | `done` | `extractTextFromACPContent(param.contentBlocks)` |
| `prompt_done` | `system` | `prompt_result` | `done` | `param.stopReason` |
| `user_message_chunk` | `user` | `message` | `streaming` | `extractTextFromIMParam(param)` |
| `agent_message_chunk` | `assistant` | `message` | `streaming` | `extractTextFromIMParam(param)` |
| `agent_thought_chunk` | `assistant` | `thought` | `streaming` | `extractTextFromIMParam(param)` |
| `tool_call` | `system` | `tool` | derived from `param.status` | `extractTextFromIMParam(param)` |
| `agent_plan` | `assistant` | `thought` | `streaming` | `extractTextFromIMParam(param)` |
| `system` | `system` | `message` | `done` | `extractTextFromIMParam(param)` |
| Other unknown methods | `assistant` | `message` | `done` | `extractTextFromIMParam(param)` |
| JSON parse failure | `assistant` | `message` | `done` | raw `content` string |

### 6.5 `extractTextFromIMParam` Text Extraction Logic

Due to IM channel historical compatibility, `param` can be in multiple formats:

1. **String**: returned directly.
2. **Array**: filter items where `type === 'content'`, extract `content` field, join.
3. **Object**: extract by priority:
   - `text` (string) → returned directly
   - `output` (string) → returned directly
   - `cmd` (string) → returned directly
   - `contentBlocks` (array) → call `extractTextFromACPContent`

### 6.6 `normalizeChatStatus` Status Normalization

Frontend normalizes server-side status strings during decode:

| Wire value | Normalized to |
|------------|---------------|
| `"streaming"` / `"running"` / `"in_progress"` | `"streaming"` |
| `"done"` / `"completed"` | `"done"` |
| `"need_action"` / `"needs_action"` | `"needs_action"` |

## 7. App Sync Model

### 7.1 Per-Session Cursors

Frontend maintains two cursors per session (stored in `Ref`):

- `promptIndex`: current latest prompt sequence number.
- `turnIndex`: current latest turn sequence number within the prompt.

### 7.2 Update Logic

- **Realtime `session.message` events**: before upsert, detect prompt/turn gaps from the latest finished cursor. If no gap exists, upsert by `(sessionId, promptIndex, turnIndex)` and advance the cursor only when `finished=true`.
- **`session.read` active pull**: pull delta from checkpoint, replace messages after affected prompt boundary.
- **Cache rule**: `finished=false` turns are display-only streaming state and are not saved to client IndexedDB.
- **Prompt boundaries**: `prompt_done` is a normal finished turn and advances the cursor.
- **Authoritative request turn**: server `prompt_request` is authoritative; the app does not persist optimistic local `prompt_request` turns.

### 7.3 `RegistryChatMessage` Type

```typescript
// Matches backend IMTurnMessage wire format exactly.
// role/kind/status/text/blocks are computed via helper functions, not stored.
interface RegistrySessionMessage {
  sessionId: string;
  promptIndex: number;
  turnIndex: number;
  method: string;                     // IMTurnMessage.method
  param: Record<string, unknown>;     // IMTurnMessage.param
  finished: boolean;                  // sealed, cacheable turn envelope state
}
```

## 8. Frontend Pull Triggers

Current app chat triggers are:

1. **Enter chat tab** (`tab` changes to `chat`): call `session.list` only.
2. **User clicks/opens a session**: call `session.read` with current cursor.
3. **Reconnect succeeds and user is still on chat tab with selected session**:
   - Silent reconnect path: call `session.read` (incremental) for selected session.
4. **Full reconnect while on chat tab**:
   - Call `session.list`; message body is not auto-read until user selects/reads a session.

Current app does not proactively pull all session messages on project switch.

## 9. Frontend Write Flows

### Send Message Flow (`sendChatMessage`)

```
User input → click Send
  ├─ Sync: capture trimmedText / blocks
  ├─ Sync: resetChatComposer() clear input
  ├─ setChatSending(true)
  └─ Branch:
       ├─ !selectedChatId (no selected session)
       │   ├─ beginNewChatFlow({title, text, blocks})
       │   │   ├─ setPendingNewChatDraft(draft)
       │   │   └─ setNewChatAgentPickerOpen(true)  open agent picker
       │   └─ return (wait for user to pick agent)
       │
       └─ selectedChatId exists
           ├─ service.sendSessionMessage({sessionId, text, blocks})
           └─ update session list / selectedChatId after response
```

### Agent Picker Completion Flow (`completeNewChatFlow`)

```
User picks agent
  ├─ newChatFlowGuardRef mutex check (prevent concurrent)
  ├─ service.createSession(agentType, title)
  │   ├─ server creates session → returns {session, configOptions}
  │   ├─ setChatSessions merge new session
  │   └─ setSelectedChatId(sessionId)
  ├─ setNewChatAgentPickerOpen(false)
  ├─ setPendingNewChatDraft(null)
  └─ If draft has content:
       └─ service.sendSessionMessage({sessionId, text, blocks})
```

### Config Option Change Flow

```
User selects config option
  ├─ handleChatConfigOptionChange(option, value)
  ├─ setChatConfigUpdatingKey(sessionId:configId)
  ├─ Show "Applying config..." feedback
  ├─ service.setSessionConfig({sessionId, configId, value})
  ├─ Success: applyChatSessionConfigOptions(sessionId, result.configOptions)
  │   └─ Merge into chatSessions configOptions for the session
  ├─ Failure: Show "Config update failed: ..." error feedback
  └─ Clear updatingKey
```

## 10. Prompt/Turn Lifecycle

### 10.1 Timeline

```
prompt_request  ──────────────────────────────> prompt_done
  (promptIndex=N)                                  (stopReason)
     │                                                │
     ├─ user_message_chunk (turn 1)                   │
     ├─ agent_thought_chunk (turn 2)                  │
     ├─ tool_call (turn 3) ── status running ── done  │
     ├─ agent_plan (turn 4)                           │
     └─ agent_message_chunk (turn 5)                  │
```

### 10.2 Indexing

- `promptIndex`: increments per user prompt (starts from 1).
- `turnIndex`: monotonically increases within each prompt (starts from 1; `tool_call` and same-type `agent_thought_chunk` share a turn).
- `tool_call` merge rule: multiple updates with same `ToolCallID` share one `turnIndex` (via `turnIndexByKey` mapping).
- `agent_message_chunk` / `agent_thought_chunk` merge rule: consecutive same-type messages within a turn concatenate text.
- `agent_message_chunk` / `agent_thought_chunk` seal rule: when the next different turn or `prompt_done` arrives, server republishes the same `(promptIndex, turnIndex)` with the complete text and `finished: true` on the envelope.

### 10.3 Persistence

- On `prompt_request` arrival: create `sessionPromptState`, record `SessionPromptRecord` metadata (status=started).
- On `prompt_done` arrival: write a prompt snapshot under `session/<project-key>/<session-id>/prompts/p000001.json`, save final prompt metadata, then publish the `prompt_done` turn.
- Before starting the next prompt, if the previous prompt is still non-terminal, the server synthesizes `prompt_done` with stop reason `interrupted`.
- On startup: legacy `session_prompts.turns_json` rows are exported once into prompt files. Runtime reads finished prompt bodies from files and active prompt turns from memory.

## 11. Compatibility Notes

- This document is the app chat contract as implemented now.
- Legacy `chat.session.*` format is not part of this protocol flow.
- `session.markRead` route exists but is **not implemented**; do not depend on it.
- `chat.send` route exists but goes through IM app channel (slash command path), **not** used by app chat UI.
- Any protocol change must synchronize updates to:
  - Backend recorder/event payloads
  - Frontend decode and merge logic
  - This document

## 12. Related Documents

- [registry-protocol.md](./registry-protocol.md) — Full Registry Protocol 2.2 (handshake, routing, fs/git APIs, sync strategy)
- [session-recorder-record-event.md](./session-recorder-record-event.md) — Internal implementation analysis of `SessionRecorder.RecordEvent` (Go server-side)
- [global-protocol.md](./global-protocol.md) — Global Protocol envelope design (routing, tracing, ACP pass-through)
- [session-persistence-sqlite.md](./session-persistence-sqlite.md) — SQLite session persistence schema
