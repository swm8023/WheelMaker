# Client Session Persistence Consolidation Design

**Date:** 2026-04-08
**Scope:** `server/internal/hub/client/` persistence cleanup
**Approach:** Replace `state` with a single SQLite-backed store and move all persistence code into one store file

---

## Goals

1. Remove the `ProjectState/state.json` layer entirely from `client` and `session`.
2. Use one SQLite store as the only persistence boundary for project config, route bindings, and session snapshots.
3. Keep `Client` and `Session` as runtime business objects, not persisted-state mirrors.
4. Persist `routeKey -> sessionID` bindings so IM routes continue after restart.
5. Treat empty or implicit default routes as invalid input instead of silently falling back to `"default"`.

---

## Non-Goals

1. No migration from old `state.json`.
2. No event-sourcing or audit log redesign.
3. No schema split for per-agent rows in this pass.
4. No persistence of runtime-only selections such as `active_agent`.

---

## Current Problems

The current persistence model is split across three overlapping layers:

1. `state.go` / `state_store.go`
   A JSON `ProjectState` model stored in `~/.wheelmaker/state.json`.
2. `sqlite_store.go`
   A separate SQLite `SessionStore` used only for session snapshots.
3. `ClientStateStore`
   A wrapper that hides both layers but preserves both sets of concepts.

This causes several issues:

1. `Client` and `Session` hold runtime state plus persisted DTOs at the same time.
2. Persistence writes are indirect: runtime data is first copied into `ProjectState`, then saved.
3. Project-level and session-level data use different storage backends and different lifecycles.
4. Route restoration depends on in-memory fallbacks such as `"default"`, which should not be legal persisted state.

---

## Target Architecture

The new model has one persistence layer:

1. `store.go`
   Contains the SQLite schema, the persistence records, the store interface, the SQLite implementation, and the serialization helpers.
2. `client.go`
   Loads project config and route bindings at startup, restores sessions on demand, and updates bindings when routes change.
3. `session.go`
   Owns runtime lifecycle and agent interactions. It only converts runtime state to and from store records.

There is no `state` layer after this refactor.

---

## Disk Layout

SQLite storage moves under the dedicated database directory:

- `~/.wheelmaker/db/client.sqlite3`

Rules:

1. The database file must not live directly under `~/.wheelmaker/`.
2. The store initialization path creates `~/.wheelmaker/db/` when missing.
3. All client/session persistence tables live in this single database file.

---

## Table Design

### `projects`

Purpose: store project-level persisted configuration only.

Columns:

- `project_name TEXT PRIMARY KEY`
- `yolo INTEGER NOT NULL DEFAULT 0`
- `created_at TEXT NOT NULL`
- `updated_at TEXT NOT NULL`

Rules:

1. Do not store `active_agent`.
2. Do not store route information.
3. Do not store derived session state here.

### `route_bindings`

Purpose: restore `project + route_key -> session_id`.

Columns:

- `project_name TEXT NOT NULL`
- `route_key TEXT NOT NULL`
- `session_id TEXT NOT NULL`
- `created_at TEXT NOT NULL`
- `updated_at TEXT NOT NULL`
- `PRIMARY KEY (project_name, route_key)`

Rules:

1. `route_key` must be non-empty.
2. Empty `route_key` is a hard error, not a fallback case.
3. This table stores only bindings, not session summaries.

### `sessions`

Purpose: persist full session snapshots in one row per session.

Columns:

- `id TEXT PRIMARY KEY`
- `project_name TEXT NOT NULL`
- `status INTEGER NOT NULL`
- `last_reply TEXT NOT NULL DEFAULT ''`
- `acp_session_id TEXT NOT NULL DEFAULT ''`
- `agents_json TEXT NOT NULL DEFAULT '{}'`
- `created_at TEXT NOT NULL`
- `last_active_at TEXT NOT NULL`

Rules:

1. There is no separate `session_agents` table.
2. There are no separate `session_meta` or `init_meta` columns.
3. Agent-specific persisted data lives inside `agents_json`.

---

## Agent JSON Shape

`agents_json` stores the per-agent map needed to resume or inspect a session:

```json
{
  "claude": {
    "acpSessionId": "sess_xxx",
    "configOptions": [],
    "commands": [],
    "title": "",
    "updatedAt": "",
    "protocolVersion": "",
    "agentCapabilities": {},
    "agentInfo": {},
    "authMethods": []
  }
}
```

This replaces the old split between `SessionAgentState`, `clientSessionMeta`, and `clientInitMeta` as persisted concepts.

Runtime code may still use a focused in-memory struct, but the persisted shape is one agent record per agent name.

---

## Runtime Model Changes

### `Client`

`Client` remains a runtime coordinator and should keep only:

- `projectName`
- `cwd`
- `yolo`
- `configuredAgent`
- `registry`
- `store`
- `sessions`
- `routeMap`
- `imRouter`
- suspend/eviction timer fields

Remove:

- `stateStore ClientStateStore`
- `state *ProjectState`

### `Session`

`Session` remains a runtime object and should keep only runtime business state:

- `ID`
- `Status`
- `instance`
- `agents`
- `acpSessionID`
- `ready`
- `initializing`
- `lastReply`
- `createdAt`
- `lastActiveAt`
- `cwd`
- `yolo`
- `registry`
- `store`
- IM context and prompt lifecycle fields

Remove persisted-state coupling:

- `state *ProjectState`
- persisted `state` sync helpers

The current persisted information from `initMeta` and `sessionMeta` is folded into per-agent persisted state.

---

## Store Boundary

`store.go` becomes the single persistence boundary.

It contains:

1. SQLite schema definitions
2. Persistence records:
   - `projectRecord`
   - `routeBindingRecord`
   - `sessionRecord`
3. Public store interface
4. SQLite implementation
5. Serialization and deserialization helpers

Recommended store interface:

```go
type Store interface {
    LoadProject(ctx context.Context, projectName string) (*ProjectConfig, error)
    SaveProject(ctx context.Context, projectName string, cfg ProjectConfig) error

    LoadRouteBindings(ctx context.Context, projectName string) (map[string]string, error)
    SaveRouteBinding(ctx context.Context, projectName, routeKey, sessionID string) error
    DeleteRouteBinding(ctx context.Context, projectName, routeKey string) error

    LoadSession(ctx context.Context, projectName, sessionID string) (*SessionRecord, error)
    SaveSession(ctx context.Context, rec *SessionRecord) error
    ListSessions(ctx context.Context, projectName string) ([]SessionListEntry, error)
    DeleteSession(ctx context.Context, projectName, sessionID string) error

    Close() error
}
```

Key rule:

`Client` and `Session` must not know about SQL or schema details.

---

## Startup and Restore Flow

`Client.Start()` performs a light restore only:

1. Open `~/.wheelmaker/db/client.sqlite3`
2. Ensure schema exists
3. Load the project record for the current project
4. Load all route bindings for the current project
5. Populate in-memory `routeMap`
6. Do not eagerly restore all sessions into memory

If the database cannot be opened, startup fails with an error. There is no silent fallback to in-memory mode.

### Route resolution

When a valid `routeKey` arrives:

1. If there is no binding:
   - create a new session
   - persist the session row
   - persist the route binding
2. If the binding exists and the session is already in memory:
   - reuse it
3. If the binding exists but the session is not in memory:
   - load it from `sessions`
   - restore it into memory
   - reuse it
4. If the binding exists but the session row is missing:
   - return a data-integrity error
   - do not silently create a replacement session

---

## Save Timing

### Project-level saves

Persist `projects` immediately when project config changes, such as:

1. `SetYOLO`

### Route-binding saves

Persist `route_bindings` immediately when:

1. a new route receives its first session
2. `/new` creates a replacement session for a route
3. `/load` rebinds a route to another session

### Session saves

Persist `sessions` when:

1. a new session is created
2. `ensureReady` completes `session/new`
3. `ensureReady` completes `session/load`
4. session update notifications change persisted agent metadata
5. agent switch saves outgoing state
6. agent switch completes incoming state
7. `Suspend` runs
8. `Close` runs

The system no longer writes a project-level runtime state snapshot during these operations.

---

## Serialization Boundary

The new serialization path is direct:

1. `Session.toRecord(projectName)` produces a `sessionRecord`
2. `sessionFromRecord(record, cwd)` restores a runtime `Session`

There is no intermediate `ProjectState` sync step.

This removes methods such as:

- `syncRuntimeToProjectState`
- `syncAndPersistProjectState`
- `persistProjectState`

The replacement rule is:

runtime object -> record -> store

not:

runtime object -> state DTO -> store

---

## Route Legality Rules

The old `"default"` route fallback is removed.

Rules:

1. Empty `routeKey` is invalid input.
2. `normalizeRouteKey` must stop manufacturing `"default"`.
3. No binding may be persisted for an empty route.
4. No restore logic may depend on a default route.

If a caller cannot provide a route key, the caller must be fixed; persistence must not paper over it.

---

## Error Handling Rules

Persistence failures are not best-effort for binding-critical paths.

Rules:

1. If route binding save fails, the operation fails.
2. If session save fails during a persistence-triggering path, keep the in-memory session and return/log the error.
3. If a route binding points to a missing session row, return an explicit corruption error.
4. Do not silently recreate sessions to repair bad persisted bindings.

This keeps route and session state coherent and debuggable.

---

## Testing Requirements

This design requires coverage for:

1. startup loads project config and route bindings from SQLite
2. startup does not eagerly restore sessions
3. route hit restores a persisted session on demand
4. `/new` replaces the route binding and saves the new session
5. `/load` rebinds routes correctly
6. empty route keys are rejected
7. missing session rows behind a valid binding return an explicit error
8. `SetYOLO` persists through the `projects` table only
9. close/suspend paths persist sessions without any `state.json`

---

## File Layout After Refactor

Expected client package layout:

| File | Responsibility |
|------|----------------|
| `client.go` | runtime coordinator, startup restore, route resolution, eviction |
| `session.go` | runtime session lifecycle, agent callbacks, snapshot conversion |
| `store.go` | SQLite schema, records, serialization, store methods |
| `commands.go` | command behavior using runtime state + store-backed listing/load |
| `im_bridge.go` | IM command/prompt bridge |
| `permission.go` | permission routing |

Files removed or merged:

- `state.go`
- `state_store.go`
- `sqlite_store.go`
- `session_meta.go`

---

## Out of Scope

1. Migrating old `state.json` contents into SQLite
2. Splitting `agents_json` into normalized SQL tables
3. Adding audit/event history
4. Persisting active in-memory agent/process handles
5. Introducing a fallback default route
