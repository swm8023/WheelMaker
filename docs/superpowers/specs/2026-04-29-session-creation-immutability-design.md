# Session Creation Immutability Design

## Context

The current create flow constructs a `Session` before ACP returns a session ID. The `Session` is instantiated with an empty `acpSessionID`, then `ensureReady` calls `session/new`, and only after that writes the returned ACP session ID back into the existing object.

This leaves a temporary invalid state where a `Session` exists without an ACP session ID. That weakens invariants, complicates reasoning about lifecycle, and creates a risk that a future call path could persist or publish a partially initialized session.

## Goals

1. A newly created `Session` must always have a non-empty `acpSessionID` at construction time.
2. `acpSessionID` must be immutable after `Session` construction.
3. Restored sessions remain valid: loading from persisted storage may still construct a `Session` from an existing persisted ACP session ID.
4. Existing session behavior after construction should remain unchanged as much as possible.

## Non-Goals

1. Redesigning session persistence format.
2. Changing client-level `/list` or `/load` semantics.
3. Reworking prompt streaming or callback interfaces unrelated to session creation.

## Chosen Design

### 1. Introduce an in-flight creation result

Add a temporary creation result object for the new-session path. This object is not a `Session`; it is a transport for ACP creation outputs.

Expected fields:

- `sessionID string`
- `agentType string`
- `state SessionAgentState`
- `instance agent.Instance`
- `createdAt time.Time`

This object is produced only after ACP `initialize` and ACP `session/new` both succeed.

### 2. Make `Session` constructible only with a real ACP session ID

Change `newSession` so that it requires a non-empty session ID and returns an error if the ID is blank.

Implications:

- New-session path: `newSession` is called only after ACP has returned a valid `sessionID`.
- Restore path: `sessionFromRecord` continues to call `newSession(rec.ID, cwd)` with the persisted ID.

### 3. Move ACP `session/new` out of `Session.ensureReady`

`ensureReady` currently does two jobs:

1. Bring an existing session to ready state.
2. Create a brand-new ACP session when no ID exists.

After the refactor, it should only do job 1.

New responsibilities:

- If a valid `acpSessionID` already exists and the agent supports `LoadSession`, try `session/load`.
- If load is not possible or fails, only handle recovery behavior for an already-created ACP session.
- Update runtime readiness flags and agent metadata.

It must no longer allocate a new ACP session ID.

### 4. New-session flow becomes two-phase

`Client.CreateSession` becomes:

1. Validate `agentType`.
2. Create/connect an `agent.Instance`.
3. Call ACP `initialize`.
4. Call ACP `session/new`.
5. Validate returned `sessionID` is non-empty.
6. Build the in-flight creation result.
7. Construct a `Session` from that complete result.
8. Persist and register the `Session` in memory.

This guarantees that once a `Session` instance exists, it already owns a valid ACP session ID.

### 5. Treat `acpSessionID` as immutable by convention and code path

The field may remain private for now, but all write sites after construction must be removed from the new-session path.

Accepted write points after refactor:

- `newSession(...)` constructor
- restore constructor path using persisted ID

Disallowed write points after refactor:

- `CreateSession` assigning empty ID first
- `ensureReady` assigning ACP `newResult.SessionID`
- any other delayed backfill of `acpSessionID`

## API and Code Shape Changes

### Session-side changes

- `newSession(id, cwd)` becomes validating constructor, likely `(*Session, error)`.
- Add a helper to construct a `Session` from the in-flight creation result.
- Keep `SessionAgentState` as the persisted/runtime metadata container.

### Client-side changes

- `CreateSession` owns new-session ACP handshake.
- `newWiredSession` must no longer be used to create an ID-less session.
- `ClientNewSession` and IM auto-create continue to delegate through `CreateSession`.

## Failure Handling

If ACP `session/new` returns an empty ID, creation fails immediately.

If instance creation or initialization fails before a `Session` object exists, close the temporary instance and return an error.

If persistence fails after constructing a valid `Session`, return the persistence error and do not register the session in `c.sessions`.

## Testing Plan

Add or update tests to verify:

1. `CreateSession` returns a session with non-empty `acpSessionID` sourced from ACP.
2. `CreateSession` fails when ACP `session/new` returns an empty `SessionID`.
3. No newly created `Session` can exist with an empty ID.
4. `sessionFromRecord` still restores sessions correctly using persisted IDs.
5. Existing route-binding behavior for `ClientNewSession` remains unchanged.

## Risks

1. Tests and helpers currently mutate `sess.acpSessionID` directly and will need focused updates.
2. The current implementation reuses `ensureReady` for creation; extracting creation logic may surface hidden coupling between initialization and readiness bookkeeping.

## Recommendation

Implement the change incrementally:

1. Add failing tests for empty-ID rejection and constructor invariants.
2. Introduce the in-flight creation result object.
3. Move ACP `session/new` logic from `ensureReady` into `CreateSession` path.
4. Remove remaining delayed writes to `acpSessionID` in new-session flow.