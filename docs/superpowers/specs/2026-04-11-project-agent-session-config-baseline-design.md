# Project-Agent Session Config Baseline Design

**Date:** 2026-04-11
**Scope:** `server/internal/hub/client/` session config persistence and restore behavior
**Approach:** Add one project-level per-agent baseline cache in `projects.agent_state_json`, and route all new-session and restore behavior through a single precedence model

---

## Goals

1. Make new conversations inherit the last saved config for the same `project + agent`.
2. Keep old conversations stable by restoring their own saved session config first.
3. Stop `cancel`, reconnect, and restore fallback paths from resetting `mode`, `model`, or `thought_level`.
4. Cache `available commands` and `config options` for each `project + agent` in one unified place.
5. Replace scattered restore patches with one explicit precedence and replay policy.

---

## Non-Goals

1. Do not make `available commands` writable from the client.
2. Do not replay arbitrary config options back to the agent.
3. Do not normalize agent baseline state into a new SQL table.
4. Do not add cross-project shared config inheritance.
5. Do not change ACP protocol semantics for `session/load` or `session/new`.

---

## Current Problems

Current behavior is split across multiple partial mechanisms:

1. Session-local config is stored inside `sessions.agents_json`.
2. A targeted fallback patch re-applies only `mode/model` when `session/load` fails and the code falls back to `session/new`.
3. New conversations do not have one canonical inheritance source.
4. `thought_level` is not part of the current compact snapshot model.
5. `available commands` exist only as session-local cache, so a fresh session has no persistent per-agent command snapshot.

This causes unstable behavior:

1. `/new` can lose the previous preferred `mode/model/effort`.
2. `cancel` followed by a path that creates a fresh ACP session can drift back to agent defaults.
3. Different restore paths apply different rules.
4. Session-local truth and project-level recent preferences are not separated cleanly.

---

## Design Summary

Persist two layers of state:

1. **Session cache**
   Stored in `sessions.agents_json`. This remains the source of truth for one specific saved conversation.
2. **Project-agent baseline**
   Stored in `projects.agent_state_json`. This is the source of truth for what a new conversation should inherit for the same `project + agent`.

These layers solve different problems:

1. Session cache preserves old conversations.
2. Project-agent baseline defines inheritance for new conversations and fallback rebuilds.

---

## Project Table Change

Extend `projects` with one JSON column:

- `agent_state_json TEXT NOT NULL DEFAULT '{}'`

The JSON shape is:

```json
{
  "codex": {
    "configOptions": [],
    "availableCommands": [],
    "updatedAt": "2026-04-11T12:00:00Z"
  },
  "claude": {
    "configOptions": [],
    "availableCommands": [],
    "updatedAt": "2026-04-11T12:05:00Z"
  }
}
```

Rules:

1. The top-level key is agent name.
2. `configOptions` stores the latest cached config list for that `project + agent`.
3. `availableCommands` stores the latest cached command list for that `project + agent`.
4. `updatedAt` is informational and used only for debugging/inspection.

There is no new table for this feature.

---

## Persisted Types

Introduce one store-level DTO for project-agent cache:

```go
type ProjectAgentState struct {
    ConfigOptions     []acp.ConfigOption     `json:"configOptions,omitempty"`
    AvailableCommands []acp.AvailableCommand `json:"availableCommands,omitempty"`
    UpdatedAt         string                 `json:"updatedAt,omitempty"`
}
```

`ProjectConfig` should gain:

```go
type ProjectConfig struct {
    YOLO       bool
    AgentState map[string]ProjectAgentState
}
```

This keeps project-level baseline state inside the existing `projects` row.

---

## Replay White List

Only the following config families are inheritable and replayable:

1. `mode`
2. `model`
3. `thought_level`

Matching rule:

1. Prefer exact config `ID`.
2. Fall back to config `Category`.

Everything outside this white list is cacheable but not replayable.

This keeps the client from force-writing arbitrary session-only or future agent-defined config options back onto restored sessions.

---

## Precedence Model

### Old Conversation: `session/load` succeeds

Priority order for config values:

1. Session-local saved config, for replayable white-list options only
2. Agent-returned config from successful `session/load`
3. Project-agent baseline, only for missing replayable white-list options

Operationally:

1. If the agent returns config options on `session/load`, treat them as the current loaded-session base.
2. Compare replayable white-list options against the saved session-local cache.
3. If the saved session-local replayable values differ, reapply the saved session-local values onto the loaded ACP session.
4. For non-white-list options, keep the agent-returned values.

Result:

1. Old conversations keep their remembered `mode/model/thought_level`.
2. Other agent-owned config is not forcibly overwritten.

### Old Conversation: `session/load` succeeds but returns empty or partial config

Priority order:

1. Session-local saved config for replayable white-list options
2. Agent-returned config for anything it did provide
3. Project-agent baseline only for missing replayable white-list options

This is a fill-missing policy, not a full overwrite policy.

### Old Conversation: `session/load` fails and code falls back to `session/new`

Priority order:

1. Project-agent baseline for replayable white-list options
2. Agent defaults from `session/new`

Session-local cache is not replayed in this case because the old ACP session no longer exists and the path is now semantically a fresh session.

### New Conversation: explicit `/new` or first route creation

Priority order:

1. Project-agent baseline for replayable white-list options
2. Agent defaults from `session/new`

This gives "new conversation inherits previous settings" behavior.

---

## Available Commands Policy

`available commands` are never replayed to the agent.

Rules:

1. Agent-published command lists are authoritative.
2. Session cache stores command lists for that saved conversation.
3. Project-agent baseline stores the latest recent command list for that agent in the project.
4. A fresh session may expose cached commands in help/UI before the agent publishes a new list, but once the agent publishes commands, those replace the cache.

This avoids pretending that commands are a writable config surface.

---

## Cancel Behavior

`/cancel` changes prompt execution only. It does not mutate config state.

Rules:

1. `cancel` must not clear session config options.
2. `cancel` must not clear project-agent baseline.
3. If the same ACP session continues after cancel, config remains unchanged.
4. If a later reconnect path creates a brand-new ACP session, that path applies the normal new-session baseline rules.

This removes the current accidental reset behavior where "cancel then start again" may drift to defaults through indirect rebuild paths.

---

## Update Sources

Project-agent baseline should be updated from the same moments that session-local cache is updated.

### Config changes

Update both session cache and project-agent baseline when:

1. `/mode`
2. `/model`
3. `/config`
4. agent `config_option_update`
5. successful `session/load` resolution after any replayable white-list overrides have finished
6. successful `session/new` plus baseline replay result

### Command changes

Update both session cache and project-agent baseline when:

1. agent `available_commands_update`
2. successful `session/load` if the agent exposes commands during or after restore

---

## Session Flow Changes

### `ensureReady`

`ensureReady` should stop hardcoding one fallback helper for only `mode/model`.

Replace that with one shared flow:

1. initialize agent
2. attempt `session/load` if possible
3. if load succeeds:
   - merge or overwrite according to the precedence rules
   - replay session-local white-list config when needed
   - persist the resolved session cache
   - persist the updated project-agent baseline
4. if load fails:
   - create `session/new`
   - apply project-agent baseline white-list config
   - persist the resolved session cache
   - persist the updated project-agent baseline

### `/new`

After `session/new`:

1. apply project-agent baseline white-list config
2. store the resolved config into session cache
3. update project-agent baseline timestamp and resolved cache

### `/load`

After successful `session/load`:

1. accept agent config as the initial loaded state
2. reapply session-local replayable white-list values if they differ
3. keep agent-returned non-white-list values
4. refresh session cache and project-agent baseline from the resolved final state

---

## Helper Boundaries

Introduce explicit helpers instead of embedding restore policy inline:

1. `replayableConfigSnapshotFromOptions`
   Extract `mode/model/thought_level`.
2. `applyReplayableConfigBaseline`
   Reapply white-list values to a live ACP session.
3. `mergeLoadedConfigOptions`
   Resolve agent-returned load config with session-local cache and baseline.
4. `updateProjectAgentState`
   Persist project baseline cache after resolved changes.

The key rule is that `/new`, load-success, and load-fallback must all use the same helper family and the same white-list.

---

## Error Handling

Rules:

1. Failure to persist project-agent baseline is a warning, not a fatal session error.
2. Failure to replay a white-list config item logs a warning and continues with the rest.
3. A failed replay must not discard agent-provided config already obtained from `load` or `new`.
4. Session persistence and project baseline persistence remain best-effort in non-binding paths, matching current session save behavior.

This keeps the runtime usable even if baseline cache writes fail.

---

## Testing Requirements

Add or update tests for:

1. new session inherits project-agent `mode/model/thought_level`
2. load-success with agent config differing from session cache replays only white-list options from session cache
3. load-success keeps non-white-list agent config values
4. load-success fills missing replayable values from session cache, then from project-agent baseline
5. load-failure fallback to new session applies project-agent baseline instead of a mode/model-only special case
6. available commands from agent override cached commands on load
7. project-agent baseline updates after `/mode`, `/model`, `/config`, `config_option_update`, and `available_commands_update`
8. cancel does not clear session config cache or project-agent baseline

---

## Migration

Schema migration is additive:

1. add `agent_state_json TEXT NOT NULL DEFAULT '{}'` to `projects`
2. existing rows read as empty baseline state
3. no backfill is required

There is no migration from session-local cache into project baseline on upgrade. Baseline state will populate naturally as sessions connect and update.

---

## Out of Scope

1. sharing config baselines across projects
2. replaying arbitrary future config options without explicit review
3. persisting active in-memory process handles
4. changing the IM help-card protocol
5. introducing a project-independent "global last agent config"
