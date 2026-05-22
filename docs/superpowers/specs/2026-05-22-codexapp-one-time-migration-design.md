# Codexapp One-Time Migration Design

## Goal

Replace the current `codexapp` compatibility strategy with a one-time startup migration. After migration, persisted agent identity must be `codex`; runtime code should no longer special-case `codexapp` as a valid public agent identity.

## Scope

The migration runs when a hub opens its SQLite store. It must not change the database schema. It only rewrites existing data values:

- `projects.default_agent_type`: `codexapp` to `codex`
- `sessions.agent_type`: `codexapp` to `codex`
- `agent_preferences.agent_type`: `codexapp` to `codex`
- `sessions.agent_json`: only `agentInfo.name` from `codexapp` to `codex`, when the JSON object can be parsed safely

The migration is local to each hub. Non-local hubs migrate when their installed hub version starts.

## Migration Semantics

The startup migration is idempotent. It can run more than once without changing already migrated data.

For `agent_preferences`, `(project_name, agent_type)` is the primary key. If both `codex` and `codexapp` preferences exist for the same project, the migration keeps the existing `codex` row and deletes the old `codexapp` row. If only `codexapp` exists, it is renamed to `codex`.

Malformed `sessions.agent_json` must not block hub startup. The migration should leave malformed JSON untouched and continue migrating the direct columns.

## Compatibility Removal

After migration support is in place, remove runtime compatibility branches that treat `codexapp` as an accepted identity:

- protocol provider parsing should not accept `codexapp`
- provider preset lookup should not accept `codexapp`
- server store reads should not normalize `codexapp` after migration
- server preference lookup should not fallback from `codex` to `codexapp`
- web registry/session normalization should not map `codexapp` to `codex`
- UI tag mapping should not special-case `codexapp`

The only remaining `codexapp` names should be internal bridge implementation names where they describe the Codex app-server adapter code, not public persisted agent identity.

## Testing

Server tests should prove:

- opening a store with old `codexapp` direct columns rewrites them to `codex`
- preference conflicts keep `codex` and remove `codexapp`
- malformed `agent_json` does not prevent startup
- after migration, parsing or creating sessions with `codexapp` is rejected instead of silently normalized

Web tests should prove:

- registry project/session payloads are no longer rewritten from `codexapp` to `codex`
- new/resume calls send exactly the selected agent identity without hidden `codexapp` mapping

## Rollout Notes

This intentionally shifts responsibility to hub upgrade order. A web client connected to a not-yet-upgraded hub may still see old `codexapp` data until that hub updates and starts. That is acceptable for this design because the goal is to remove compatibility code rather than keep long-lived dual semantics.
