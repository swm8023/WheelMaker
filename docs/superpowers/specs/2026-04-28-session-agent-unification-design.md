# Session-Agent Unification Design

Date: 2026-04-28
Status: Draft for review
Scope: `server/internal/hub/client/`, `server/internal/im/`, `server/cmd/wheelmaker-monitor/`, `app/web/`
Approach: Collapse business session and ACP session into one persisted session model, move project-agent defaults into a dedicated table, and delete multi-agent-in-one-session behavior.

## Goal

Remove the split between WheelMaker business sessions and ACP sessions so one persisted session row always represents one concrete agent conversation.

This change should:

- make `sessions.id` the single session identifier exposed everywhere and persist the ACP `session_id` directly in that column
- remove `acp_session_id`, `agents_json`, and all session-internal agent switching behavior
- make `/help` open a `New Conversation` flow instead of `Switch Agent`
- create a new conversation only after the user picks an agent
- store reusable per-agent defaults in one `project + agent_type` preference table
- delete old schema migration logic instead of carrying compatibility code forward

## Non-Goals

- Do not preserve backward compatibility for old session rows or old SQLite schemas.
- Do not keep `/use` as a deprecated command or alias.
- Do not keep `projects.agent_state_json` as a second source of truth.
- Do not persist a draft session before an agent has been chosen.
- Do not add cross-project shared preferences.

## Product Decisions Confirmed

1. This is a hard cut. Old session data does not need to survive.
2. `sessions.id` is the ACP `session_id` returned by `session/new`.
3. The per-session JSON column remains named `agent_json`.
4. Project-level reusable defaults move to `agent_preferences`.
5. `agent_preferences` contains no `created_at` or `updated_at` columns.
6. `/help` root replaces `Switch Agent` with `New Conversation`.
7. `/new` is a two-step flow: choose agent first, then create the session.
8. `/use` is removed instead of being kept as a compatibility stub.
9. Old migration code is removed; store startup only initializes a fresh schema or rejects a mismatched one.

## Current Problems

The current client/session model still reflects the older multi-agent design:

1. `sessions` stores both a business `id` and a separate `acp_session_id`.
2. Session state is bucketed by agent inside `agents_json`.
3. `Session` runtime logic supports `switchAgent`, saved ACP session IDs per agent, and config replay across agent boundaries.
4. `/help` still treats agent switching as a first-class action.
5. Project-level defaults are hidden inside `projects.agent_state_json`, which overlaps with session-local state.
6. SQLite startup still contains schema migration code for legacy layouts.

This creates unnecessary complexity:

- one user-visible conversation can carry multiple latent ACP sessions
- runtime logic has to reason about `activeAgent`, `savedSID`, and per-agent caches inside one session
- UI/help surfaces expose agent switching even though the intended product model is now one session per agent conversation
- persistence contains multiple overlapping representations of the same state
- startup keeps carrying forward schema-compatibility branches that are no longer needed

## Design Summary

### 1. One Session Row Equals One Agent Conversation

After this redesign, a persisted session row represents exactly one concrete conversation with exactly one agent.

Canonical session identity becomes:

- `project_name`
- `sessions.id`

Where `sessions.id` is the ACP `session_id` returned by the underlying agent.

Each session row also stores:

- `agent_type`
- `agent_json`
- `title`
- `status`
- `created_at`
- `last_active_at`

There is no separate `acp_session_id` column and no map of agent states inside one session.

### 2. Agent Defaults Move to a Dedicated Table

Reusable defaults for future conversations are stored separately from runtime session state.

The new table is:

- `agent_preferences(project_name, agent_type, preference_json)`

Semantics:

- one row per `project + agent`
- stores defaults that a newly created session should inherit for that agent
- does not store session-local runtime identity or prompt history

This replaces the current use of `projects.agent_state_json` as the baseline cache.

### 3. Agent Choice Happens Before Session Creation

`/new` no longer creates a blank session immediately.

Instead:

1. the user enters `New Conversation`
2. the IM surface shows an agent selection menu
3. the user picks an agent
4. the server loads `agent_preferences` for that `project + agent`
5. the server starts the chosen agent and calls `session/new`
6. the returned ACP `session_id` becomes `sessions.id`
7. the route binding is switched to the new session only after creation succeeds

There is never a persisted session without an agent.

### 4. Old Paths Are Deleted Rather Than Redirected

This redesign intentionally removes obsolete behavior:

- no `switchAgent`
- no `SwitchMode`
- no per-agent session map inside a single session
- no `/use`
- no multi-agent help submenu
- no legacy schema migration helpers

The goal is to leave one clear model in code rather than preserve the old shape behind wrappers.

## Target Persistence Model

## Tables

### `route_bindings`

Purpose: restore `project + route_key -> session_id`.

Columns:

- `project_name TEXT NOT NULL`
- `route_key TEXT NOT NULL`
- `session_id TEXT NOT NULL`
- `created_at TEXT NOT NULL`
- `updated_at TEXT NOT NULL`
- `PRIMARY KEY (project_name, route_key)`

This table remains unchanged in purpose.

### `sessions`

Purpose: store one persisted conversation per row.

Columns:

- `id TEXT PRIMARY KEY`
- `project_name TEXT NOT NULL`
- `status INTEGER NOT NULL`
- `agent_type TEXT NOT NULL`
- `agent_json TEXT NOT NULL DEFAULT '{}'`
- `title TEXT NOT NULL DEFAULT ''`
- `created_at TEXT NOT NULL`
- `last_active_at TEXT NOT NULL`

Rules:

1. `id` is the ACP `session_id`.
2. `agent_type` is required for every row.
3. `agent_json` stores only the current session's agent-owned state.
4. There is no `acp_session_id` column.
5. There is no `agents_json` column.

### `agent_preferences`

Purpose: store reusable defaults for future conversations per project and per agent.

Columns:

- `project_name TEXT NOT NULL`
- `agent_type TEXT NOT NULL`
- `preference_json TEXT NOT NULL DEFAULT '{}'`
- `PRIMARY KEY (project_name, agent_type)`

Rules:

1. This table has no timestamp fields.
2. This table does not store prompt history or session identity.
3. `preference_json` stores the latest inheritable defaults for that agent in that project.

### Removed Tables and Columns

Delete these persistence concepts:

- `projects`
- `sessions.acp_session_id`
- `sessions.agents_json`

If the existing database still contains the old schema, startup rejects it instead of migrating it.

## JSON Shapes

### `sessions.agent_json`

`agent_json` stores the state needed to continue or inspect one concrete session. The exact struct can remain implementation-owned, but it should contain only one agent's state.

Recommended shape:

```json
{
  "configOptions": [],
  "commands": [],
  "title": "",
  "updatedAt": "",
  "protocolVersion": "",
  "agentCapabilities": {},
  "agentInfo": {},
  "authMethods": []
}
```

Notably absent:

- no nested agent-name map
- no second ACP session ID field

### `agent_preferences.preference_json`

`preference_json` stores only values that should apply to future sessions for the same `project + agent`.

Recommended shape:

```json
{
  "configOptions": [],
  "commands": []
}
```

This keeps the preference table narrow and avoids duplicating session-specific metadata.

## Store Boundary Changes

The SQLite store becomes simpler and more explicit.

Recommended interface shape:

```go
type Store interface {
    LoadRouteBindings(ctx context.Context, projectName string) (map[string]string, error)
    SaveRouteBinding(ctx context.Context, projectName, routeKey, sessionID string) error
    DeleteRouteBinding(ctx context.Context, projectName, routeKey string) error

    LoadSession(ctx context.Context, projectName, sessionID string) (*SessionRecord, error)
    SaveSession(ctx context.Context, rec *SessionRecord) error
    ListSessions(ctx context.Context, projectName string) ([]SessionRecord, error)
    DeleteSession(ctx context.Context, projectName, sessionID string) error

    LoadAgentPreference(ctx context.Context, projectName, agentType string) (*AgentPreferenceRecord, error)
    SaveAgentPreference(ctx context.Context, rec AgentPreferenceRecord) error

    UpsertSessionPrompt(ctx context.Context, rec SessionPromptRecord) error
    LoadSessionPrompt(ctx context.Context, projectName, sessionID string, promptIndex int64) (*SessionPromptRecord, error)
    ListSessionPrompts(ctx context.Context, projectName, sessionID string) ([]SessionPromptRecord, error)
    ListSessionPromptsAfterIndex(ctx context.Context, projectName, sessionID string, afterPromptIndex int64) ([]SessionPromptRecord, error)
    UpsertSessionTurn(ctx context.Context, rec SessionTurnRecord) error
    LoadSessionTurn(ctx context.Context, projectName, sessionID string, promptIndex, turnIndex int64) (*SessionTurnRecord, error)
    ListSessionTurns(ctx context.Context, projectName, sessionID string, promptIndex int64) ([]SessionTurnRecord, error)

    Close() error
}
```

Delete from the store boundary:

- `LoadProject`
- `SaveProject`
- `ProjectConfig`
- `ProjectAgentState`

Add one new record type:

```go
type AgentPreferenceRecord struct {
    ProjectName    string
    AgentType      string
    PreferenceJSON string
}
```

## Schema Initialization and Validation

Startup behavior is a hard cut:

1. if the database is empty, initialize the new schema
2. if the database already contains user tables, verify the exact expected table and column sets
3. if the schema does not match, return `StoreSchemaMismatchError`

Delete all migration logic, including targeted legacy rewrites.

That means:

- remove `migrateLegacyStoreSchema`
- remove column-shape migrations for `update_index`
- remove compatibility branches for old `sessions` layouts

The code should either run on the new schema or fail loudly.

## Runtime Model Changes

## `Client`

`Client` remains the route coordinator, but it no longer owns any project-level agent baseline cache.

Responsibilities:

- load route bindings on startup
- resolve sessions by `session.id`
- create a new session only after an agent has been selected
- rebind routes when `/new` or `/load` succeeds

Delete from `Client` behavior:

- implicit support for switching one existing session between multiple agents
- code that assumes a session row can represent more than one agent conversation

For routes with no existing bound session and a direct freeform prompt, keep the current project-configured default agent behavior unless the IM surface explicitly entered the `/new` flow.

## `Session`

`Session` becomes a single-agent runtime object.

Required runtime fields:

- `ID`
- `Status`
- `agentType`
- `instance`
- `ready`
- `initializing`
- `lastReply`
- `createdAt`
- `lastActiveAt`
- prompt lifecycle state

Delete from `Session`:

- `activeAgent`
- `agents map[...]`
- per-agent saved ACP session IDs
- `switchAgent`
- `SwitchMode`
- any helper that reads or writes session-local state through agent-name keys

`ensureReady` simplifies:

1. initialize the current agent instance
2. if the session already exists, load state from `agent_json`
3. restore or continue the same ACP session identified by `Session.ID`
4. apply current-session config and command state

There is no cross-agent restore path.

## Agent Preference Read and Write Rules

`agent_preferences` is the source of truth for new-session defaults.

Read path:

1. `/new <agent>` loads the preference row for that `project + agent`
2. `session/new` is executed
3. replayable defaults from `preference_json` are applied to the new ACP session

Write path:

Update both the current session row and the preference row when the user changes values that should become future defaults:

- `/mode`
- `/model`
- `/config`
- agent-published config updates
- agent-published command updates

Result:

- `sessions.agent_json` stays session-local
- `agent_preferences.preference_json` tracks the latest reusable defaults for future conversations of the same agent

## Command and Help Flow

## `/help`

Root help menu changes:

- first item becomes `New Conversation`
- `Switch Agent` is removed entirely

The new root flow should be:

1. `New Conversation`
2. session list
3. status
4. config entries for the current session

If the current session has no available config options yet, config menus may remain hidden or empty.

## `/new`

Two supported entry forms:

1. `/new`
   opens agent selection
2. `/new <agent-type>`
   immediately creates a new session with the chosen agent

Behavior:

1. `/new` does not persist anything by itself
2. agent choice is held in a short-lived IM interaction state, not in SQLite
3. once an agent is chosen, the server creates the session and then updates the route binding
4. if creation fails, the old route binding remains unchanged and no session row is written

## `/load`

`/load` continues to load an existing session by index.

Semantics change:

- loading a session also loads its fixed `agent_type`
- there is no post-load agent switch step

## `/use`

Delete `/use` parsing, help exposure, tests, and UI references.

This includes:

- command dispatch in `client.go`
- help menu option generation
- command parsing allow-lists in IM channels
- tests that expect `/use` to exist

## IM Interaction Changes

## Feishu Help Cards

The existing help card action model should be reused, but the first menu becomes new-conversation driven.

Required behavior:

1. selecting `New Conversation` opens an agent picker submenu
2. selecting an agent triggers `/new <agent>`
3. help reopens at the root after success so the user sees the new current session context

There is no help path that switches the current session to another agent.

## App/Web Help and Session Surfaces

Any app/web surface that assumes:

- a separate `acpSessionId`
- a session-local agent map
- an agent switch action for the current session

must be updated or deleted.

Required UI state after this redesign:

- one session row has one `sessionId`
- one session row has one `agentType`
- new conversation flow offers agent choice before creation

## Monitor and Observability Changes

The monitor must stop rendering a separate ACP session field.

Rules:

1. session displays should use `session.id` as the only session identifier
2. if agent information is shown, render `agent_type`
3. remove any debug view that implies a second ACP session identity layered under the business session

## Deletion Plan

Delete the following logic rather than wrapping it:

1. per-session multi-agent state maps
2. `switchAgent` and `SwitchMode`
3. `/use` command handling and tests
4. project baseline helpers backed by `projects.agent_state_json`
5. store migration helpers for legacy schemas
6. monitor and app fields named `acpSessionId`
7. help menus and labels that mention agent switching

The remaining code should express only the post-cut model.

## Testing Strategy

## Server Tests

Add or rewrite focused tests for:

1. store schema validation rejects the old `projects` or old `sessions` column layout
2. new schema includes `sessions.agent_type`, `sessions.agent_json`, and `agent_preferences.preference_json`
3. `/help` root menu exposes `New Conversation` and does not expose `Switch Agent`
4. `/new` opens agent selection without creating a session row
5. `/new <agent>` creates a session only after the agent is known and persists `session.id == ACP session_id`
6. route binding changes only after successful session creation
7. `/load` restores a session with its fixed `agent_type`
8. config and command updates write both `sessions.agent_json` and `agent_preferences.preference_json`
9. `/use` is no longer recognized as a valid command

## App/Web Tests

Update or add tests so they assert:

1. session list/state uses one `sessionId` field only
2. UI does not depend on `acpSessionId`
3. help-driven new conversation flow requires agent choice before creation
4. switch-agent actions and labels no longer exist

## Monitor Tests

Update monitor assertions so they do not reference a separate ACP session field and instead assert the unified session identity.

## Risks and Mitigations

### Risk: Hidden code paths still assume multi-agent sessions

Mitigation:

- delete the old types and helpers instead of leaving adapters
- update command allow-lists and tests in every IM channel

### Risk: Old local databases fail after deploy

Mitigation:

- this is expected product behavior for the hard cut
- startup should fail with a precise schema mismatch error rather than partial runtime corruption

### Risk: New-session defaults drift from current-session state

Mitigation:

- update session-local and preference-local JSON in the same command and update handlers

## Rollout Notes

This design intentionally trades compatibility for simplicity.

Operational expectation:

1. developers clear or recreate local client databases when adopting the new build
2. no runtime migration path is provided
3. failures due to old schema are fixed by rebuilding the local store, not by patching code to accept old layouts
