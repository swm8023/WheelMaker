# IM Protocol 2.0

Updated: 2026-04-07
Status: Draft (implemented in `server/internal/im2`)

## 1. Goal

IM Protocol 2.0 defines a client-centric IM routing model where:

- one `Client` owns one `IMRouter`
- one `clientSessionId` can be connected to multiple IM active chats
- outbound messages can be broadcast or targeted
- ACP payload is transparent to IM layer

## 2. Core Terms

- `clientSessionId`: persisted session identity in client domain.
- `IMActiveChatID`: normalized chat endpoint id, format `<imType>:<chatID>`.
- `IMRouter`: single ingress/egress routing entry for one client.
- `IMActiveChat`: lightweight chat endpoint runtime record.

## 3. Routing Model

### 3.1 Inbound (IM -> Client)

Inbound events are normalized by `IMRouter` into one callback:

- `prompt`
- `permission_reply`
- `command`

Flow:

1. router resolves/builds `IMActiveChatID`
2. router looks up `IMActiveChatID -> clientSessionId`
3. if not found, router creates a new `clientSessionId` and binds it to chat
4. router dispatches unified inbound event to client

### 3.2 Outbound (Client -> IM)

Client publishes unified events to router via `Publish(event)`:

- `message`
- `acp_update`
- `command_reply`

Routing rule:

- if `targetActiveChatID` is set: targeted send
- if `targetActiveChatID` is empty: broadcast to all online active chats in the same `clientSessionId`

## 4. Protocol Transparency

IMRouter does not interpret ACP details.

- `acp_update` payload is forwarded as opaque data
- IM integrations decide rendering/UI behavior

## 5. State Model

IM 2.0 state persists chat-related bindings only in SQLite with project isolation (`project_name`).

Table:

`im_active_chats`

- `project_name`
- `active_chat_id`
- `im_type`
- `chat_id`
- `client_session_id`
- `online`
- `last_seen_at`
- `updated_at`

Primary key: `(project_name, active_chat_id)`

Notes:

- `clientSessionId` lifecycle is owned by client domain.
- IM 2.0 only persists chat binding/runtime projection.

## 6. Persistence Policy

Write policy is hybrid:

- critical changes: sync write immediately
  - new chat binding
  - binding switch operations
- non-critical changes: debounced async flush
  - online/offline
  - heartbeat/last seen updates

## 7. Current Scope

Initial IM 2.0 scope:

- package location: `server/internal/im2`
- first IM integration target: `feishu`
- router and state are implemented and unit-tested
- existing `internal/hub/im` flow is still active (migration is incremental)
