# Project Default Agent Persistence Design

**Date:** 2026-04-28  
**Scope:** `server/internal/hub/client` project-level default agent persistence  
**Approach:** Re-introduce a dedicated `projects` table to store `default_agent_type`, and update it only after explicit successful session creation (`/new <agent>` and `session.new(agentType)`).

---

## Goals

1. Persist one default agent per project.
2. Make explicit new-session actions become the source of truth for that project default.
3. Use the saved project default when auto-creating sessions for new IM routes.
4. Keep fallback behavior safe when the saved default is temporarily unavailable.

---

## Non-Goals

1. Do not add global cross-project defaults.
2. Do not change `session.new` API to make `agentType` optional.
3. Do not update default agent during implicit/automatic session creation fallback.
4. Do not alter existing agent preference payload semantics in `agent_preferences`.

---

## Current Problem

After `projects` table removal, there is no dedicated persisted project-level field for "default agent". Current behavior uses runtime `PreferredName()` ordering, not user intent remembered from recent explicit new-session choice.

---

## Design Summary

1. Add a `projects` table back into SQLite schema with:
   - `project_name TEXT PRIMARY KEY`
   - `default_agent_type TEXT NOT NULL DEFAULT ''`
   - `updated_at TEXT NOT NULL`
2. Extend store interface with project default agent load/save methods.
3. Update explicit new-session success paths to persist the selected agent as project default:
   - IM command `/new <agent>`
   - API `session.new` with `agentType`
4. Auto route-first session creation reads saved default first; if unavailable, fallback to runtime preferred provider without rewriting saved default.

---

## Data Model

### New Table

```sql
CREATE TABLE IF NOT EXISTS projects (
    project_name TEXT PRIMARY KEY,
    default_agent_type TEXT NOT NULL DEFAULT '',
    updated_at TEXT NOT NULL
);
```

### Store Contract Additions

1. `LoadProjectDefaultAgent(ctx, projectName) (string, error)`
2. `SaveProjectDefaultAgent(ctx, projectName, agentType string) error`

Behavior:

1. Load returns empty string when row absent.
2. Save upserts by `project_name`.
3. Save validates non-empty trimmed `projectName` and `agentType`.

---

## Runtime Behavior

### Explicit New Session (writes default)

The following successful operations persist project default:

1. `ClientNewSession(routeKey, agentType)` success path
2. `HandleSessionRequest("session.new", payload{agentType})` success path

Write timing:

1. Save only after session is created and ready.
2. If save fails, log warning and keep request success (best-effort persistence, matching existing session persistence style).

### Implicit Session Creation (reads default, no write)

When route has no bound session and client auto-creates:

1. Try stored project default first.
2. If missing or unavailable, fallback to `PreferredName()`.
3. Do not overwrite stored default during this fallback path.

---

## Agent Availability Rules

`default_agent_type` is treated as preference, not a strict hard requirement:

1. If saved default has a registered creator, use it.
2. If not available, use fallback preferred provider.
3. Preserve stored value unchanged so it can recover automatically when that provider returns.

---

## Error Handling

1. Invalid input for save (`projectName`/`agentType` empty) returns validation error.
2. DB read/write failures return wrapped errors in store layer.
3. Callers treat default-agent persistence as non-fatal in session creation flows:
   - warn and continue for save failure
   - fallback and continue for load/unavailable cases

---

## Test Plan

1. `sqlite_store`:
   - save then load default agent round-trip
   - absent project row returns empty default
   - upsert updates existing default
2. `client`:
   - `/new <agent>` path updates stored project default
   - `session.new` path updates stored project default
   - auto-create route uses stored default when available
   - auto-create route falls back when stored default unavailable
   - fallback path does not rewrite stored default
3. Schema verification:
   - include `projects` in `expectedStoreSchemaColumns`
   - update monitor schema expectations if table list assertions are present

---

## Implementation Notes

1. Keep `agent_preferences` unchanged; it still stores per-project-per-agent preference payload.
2. Re-introduced `projects` table is intentionally minimal and single-purpose.
3. Future project-level settings can extend this table without overloading `agent_preferences`.

---

## Out of Scope

1. Backfilling historical default from old sessions.
2. Adding UI endpoints to manually set/reset project default.
3. Schema migration tooling beyond current startup schema checks.