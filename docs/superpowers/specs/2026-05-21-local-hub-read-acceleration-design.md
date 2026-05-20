# Local Hub Read Acceleration Design

Date: 2026-05-21
Status: Draft

## Goal

Avoid sending large File and Git payloads through the external network when the Workspace client and the target Hub are on the same machine.

Chat and Session traffic must still use the remote Conversation Registry so outside clients receive the same session events. Local Hub Read Acceleration is only for File/Git workspace reads.

## Problems

- File and Git requests can move large file bodies, grep results, and diffs through the remote Registry path even when the Hub is on the same machine as the client.
- A fixed local port is unreliable because another local service may already own it.
- A localhost port accepting WebSocket is not proof that it is WheelMaker. Sending the Registry token before endpoint proof could leak the token to an unrelated local service.
- A second local Registry would duplicate routing responsibilities and blur the Chat/Session boundary.

## Non-Goals

- Do not move Chat, Session, command, monitor, or realtime session event traffic to the local endpoint.
- Do not expose local reads over LAN, public hostnames, or remote file URLs.
- Do not add a separate REST or raw file-serving API.
- Do not add per-project local read endpoint configuration.
- Do not make local read acceleration a required capability for Hub startup or Workspace operation.
- Do not solve duplicate `hubId` identity conflicts in this slice.
- Do not add a new realtime local-read changed event.

## Terms

- **Conversation Registry**: the Registry connection that owns Chat and Session traffic.
- **Local Hub Read Endpoint**: a same-machine Hub-owned WebSocket endpoint for File/Git reads only.
- **Local Hub Read Candidate**: Hub-level localhost connection metadata reported through `project.list.hubs[]`.
- **Local Read Endpoint Proof**: the token-free proof step that confirms the localhost endpoint matches the reported candidate.
- **Local Read Project Match**: exact `projectId` equality between the Conversation Registry project and the local endpoint project list.
- **Local Read State**: UI state shown as `Local` or `Remote`.

## Architecture

The Workspace uses two independent connections when local reads are available:

- The existing Registry connection remains the Conversation Registry.
- A new optional local read connection is opened to the Hub-reported localhost candidate.

Hub startup creates the Local Hub Read Endpoint before reporting its Hub snapshot when possible. The endpoint binds an available loopback port at runtime, not a fixed port. If it fails to start, Hub startup continues and the Hub reports no local read candidate.

The Hub reports a Hub-level local read candidate through `project.list.hubs[]`. Candidate changes flow through normal Hub/project snapshot refresh. The Registry does not push a new local-read event and does not advertise local candidates from `connect.init`.

Suggested hub item shape:

```json
{
  "hubId": "hub-a",
  "localRead": {
    "enabled": true,
    "host": "127.0.0.1",
    "port": 49152,
    "path": "/ws",
    "protocolVersion": "2.2",
    "endpointId": "lr_01...",
    "proofPublicKey": "base64..."
  }
}
```

`localRead` is a localhost hint only. It must not contain tokens or advertise LAN/public addresses.

## Protocol

The Local Hub Read Endpoint reuses the Registry envelope shape over WebSocket. It supports a strict method subset:

- pre-auth only: `local_read.proof`
- auth: `connect.init` with role `local_read`
- project discovery: `project.list`
- File/Git freshness: `project.syncCheck`
- File: `fs.list`, `fs.info`, `fs.read`, `fs.search`, `fs.grep`
- Git: `git.refs`, `git.log`, `git.commit.files`, `git.commit.fileDiff`, `git.diff`, `git.diff.fileDiff`, `git.status`, `git.workingTree.fileDiff`

Rejected methods include:

- `chat.*`
- `session.*`
- `cmd.*`
- `monitor.*`
- `batch`

`project.list` on the local endpoint returns all projects owned by that Hub, using canonical `projectId = hubId + ":" + projectName`. The App uses that list for exact Local Read Project Match.

## Endpoint Proof

The Hub generates an ephemeral proof identity for each Hub process. It is not persisted across Hub restarts.

Flow:

1. App connects to the candidate localhost WebSocket.
2. Before sending any token, App sends `local_read.proof` with a fresh nonce and expected `endpointId`.
3. Hub signs the nonce with the private proof key for the current local read endpoint.
4. App verifies the signature using `proofPublicKey` from the candidate.
5. Only after proof succeeds, App sends `connect.init` with role `local_read` and the current Conversation Registry token.

If proof fails, the App must not send a token and must fall back to Remote reads.

## Security

First-version local read security is:

- loopback-only binding
- endpoint proof before token authentication
- shared Registry token authentication
- strict local-read method whitelist

TLS is not required for the first version. If a browser blocks `ws://127.0.0.1` from an HTTPS Workspace page, that is a local read fallback condition, not a visible workspace error.

Local read logs and debug records must not include tokens, proof private key material, file content, diff bodies, or grep result text.

## Client Routing

Local Hub Read Acceleration is default-on.

When the App receives a project snapshot:

1. Read `hubs[].localRead` candidates.
2. If the App setting is enabled, attempt each candidate at most once for the current snapshot/candidate state.
3. Perform endpoint proof, then `connect.init`.
4. Call local `project.list`.
5. Store one local read connection per Hub when proof/auth succeeds.

File/Git requests choose transport per request:

- If local read is enabled, the Hub has an active local read connection, and local `project.list` contains the request `projectId`, use Local.
- Otherwise use Remote through the Conversation Registry.

File/Git `project.syncCheck` follows the same selected read path for that project. Chat/session list, session read, session send, session events, command, and monitor operations always use the Conversation Registry.

## Fallback

Local Read Fallback applies to local path availability and identity failures:

- local WebSocket blocked, unavailable, closed, or timed out
- endpoint proof failure
- local auth failure
- local project list missing the exact `projectId`

Fallback does not apply to normal File/Git business errors from a matched local endpoint. For example, a file `NOT_FOUND` after a successful Local Read Project Match should surface as the operation result instead of silently retrying Remote.

When a local read connection closes, the Hub's Local Read State becomes `Remote` until the next candidate/project refresh or explicit user refresh triggers another attempt.

## Settings And UI

Settings gets a default-on switch for Local Hub Read Acceleration. Turning it off closes active local read connections and makes affected Hub tags show `Remote`. Turning it on immediately tries the latest known candidates; it does not wait for the next Registry refresh.

The Chat Hub dropdown shows a compact tag beside each Hub:

- `Local`: this Hub has an active local read connection.
- `Remote`: File/Git reads for this Hub are using the Conversation Registry, including disabled settings, no candidate, proof failure, auth failure, browser blocking, or local connection close.

The tag is Hub-level status only. Each File/Git request still requires Local Read Project Match. Project-level mismatch does not change the Hub-level tag by itself.

Do not show local port numbers or failure detail in normal workspace UI. Debug output may show failure reason and endpoint metadata.

## Debug

Registry debug capture should include a simple transport label:

- `Local`
- `Remote`

`connect.init` remains excluded from capture. Proof records may be captured because they contain no token, but they must not include proof private key material.

Debug records should make these cases visible:

- proof failed
- auth failed
- browser or WebSocket connect failure
- candidate changed
- project mismatch
- request routed Local or Remote

## Server Implementation Shape

Avoid copying File/Git logic.

Refactor Hub read handling so both remote Registry forwarding and the Local Hub Read Endpoint call a shared Hub-owned read handler for:

- project list and sync check
- File methods
- Git methods

The Local Hub Read Endpoint adds only:

- loopback listener lifecycle
- endpoint proof identity and proof method
- `local_read` authentication
- local-read method whitelist
- local read audit metadata logging

The existing Reporter can keep owning remote Registry connection and Hub snapshot reporting, but candidate reporting should be sourced from the local read endpoint runtime state.

## Observability

Hub logs should include metadata only:

- local read endpoint start/stop and bound port
- candidate reported or cleared
- proof success/failure
- auth success/failure
- request method, project ID, and path/ref summary

Do not log file content, diff bodies, grep result text, token values, or proof private key material.

## Testing

Server tests:

- Local read endpoint binds loopback on an available port and exposes a candidate.
- Hub startup continues when local read endpoint cannot start.
- Candidate is absent when token is empty or endpoint is unavailable.
- `local_read.proof` succeeds for the matching candidate and fails for a wrong nonce/key/endpoint ID.
- `connect.init` is rejected before successful proof.
- `connect.init` accepts role `local_read` only after proof and valid token.
- Local read whitelist allows project, File, and Git methods.
- Local read whitelist rejects Chat, Session, command, monitor, and batch methods.
- Local `project.list` returns all local Hub projects with canonical project IDs.
- Multiple local clients can proof/auth independently.
- Logs omit content and secrets.

App service tests:

- Project snapshots read Hub-level local read candidates from `hubs[]`.
- Local read is attempted when enabled and a candidate appears.
- Turning the setting off closes local read connections and routes File/Git Remote.
- Turning the setting on immediately tries current candidates.
- Proof failure does not send `connect.init` or token.
- Proof/auth success creates one local read connection per Hub.
- Candidate changes replace the old local read connection.
- Conversation Registry close closes local read connections.
- File/Git requests route Local only when exact project ID match succeeds.
- File/Git business errors from a matched local endpoint do not retry Remote.
- Connection, proof, auth, and project-match failures fallback to Remote.
- File/Git syncCheck follows the selected read transport.
- Chat/Session requests always use Remote.

UI/debug tests:

- Chat Hub dropdown renders `Local` or `Remote` tags beside Hub identity.
- Disabled setting shows `Remote`.
- Hub tag remains Hub-level and does not change because one project mismatches.
- Debug records show `Local` or `Remote` transport labels.
- `connect.init` remains absent from debug capture.
- Proof failures and routing fallback reasons appear in debug metadata.

## Open Decisions

None. The local read scope, Hub-level candidate shape, proof-before-token security boundary, fallback behavior, UI state, and first-version exclusions are decided.
