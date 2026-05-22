# Codexapp One-Time Migration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace runtime `codexapp` compatibility with an idempotent startup data migration to `codex`.

**Architecture:** Add a focused SQLite data migration in `server/internal/hub/client/sqlite_store.go`, guarded by `PRAGMA user_version` so it runs once per hub database. Remove public `codexapp` compatibility from server protocol/provider/store and web registry/UI normalization after tests prove old data is migrated before normal store reads.

**Tech Stack:** Go SQLite store using `modernc.org/sqlite`, React/TypeScript registry repository, Jest, Go tests.

---

### Task 1: Server SQLite Migration

**Files:**
- Modify: `server/internal/hub/client/sqlite_store.go`
- Test: `server/internal/hub/client/client_test.go`

- [ ] **Step 1: Write failing migration tests**

Add tests in `client_test.go` covering:

```go
func TestSQLiteStoreMigratesCodexAppIdentityOnce(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "client.sqlite3")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore seed: %v", err)
	}
	sqliteStore := store.(*sqliteStore)
	if _, err := sqliteStore.db.ExecContext(ctx, `
		INSERT INTO projects (project_name, default_agent_type, updated_at)
		VALUES ('proj1', 'codexapp', '2026-05-20T00:00:00Z');
		INSERT INTO sessions (id, project_name, status, agent_type, agent_json, session_sync_json, title, created_at, updated_at)
		VALUES
			('sess-old', 'proj1', 2, 'codexapp', '{"agentInfo":{"name":"codexapp","title":"Codex App Server"}}', '{}', 'old', '2026-05-20T00:00:00Z', '2026-05-20T00:00:00Z'),
			('sess-bad-json', 'proj1', 2, 'codexapp', '{bad json', '{}', 'bad', '2026-05-20T00:00:00Z', '2026-05-20T00:00:00Z');
		INSERT INTO agent_preferences (project_name, agent_type, preference_json)
		VALUES
			('proj1', 'codex', '{"configOptions":[{"id":"model","currentValue":"gpt-5"}]}'),
			('proj1', 'codexapp', '{"configOptions":[{"id":"model","currentValue":"legacy"}]}'),
			('proj2', 'codexapp', '{"configOptions":[{"id":"model","currentValue":"only-legacy"}]}');
	`); err != nil {
		t.Fatalf("seed codexapp rows: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close seed store: %v", err)
	}

	store, err = NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore migrated: %v", err)
	}
	defer store.Close()
	sqliteStore = store.(*sqliteStore)

	var version int
	if err := sqliteStore.db.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatalf("query user_version: %v", err)
	}
	if version < sqliteMigrationVersionCodexAppIdentity {
		t.Fatalf("user_version=%d, want at least %d", version, sqliteMigrationVersionCodexAppIdentity)
	}

	for _, query := range []string{
		`SELECT COUNT(*) FROM projects WHERE lower(trim(default_agent_type)) = 'codexapp'`,
		`SELECT COUNT(*) FROM sessions WHERE lower(trim(agent_type)) = 'codexapp'`,
		`SELECT COUNT(*) FROM agent_preferences WHERE lower(trim(agent_type)) = 'codexapp'`,
	} {
		var count int
		if err := sqliteStore.db.QueryRowContext(ctx, query).Scan(&count); err != nil {
			t.Fatalf("query count %q: %v", query, err)
		}
		if count != 0 {
			t.Fatalf("query %q count=%d, want 0", query, count)
		}
	}

	var agentInfoName string
	if err := sqliteStore.db.QueryRowContext(ctx, `SELECT json_extract(agent_json, '$.agentInfo.name') FROM sessions WHERE id = 'sess-old'`).Scan(&agentInfoName); err != nil {
		t.Fatalf("query migrated agent_json: %v", err)
	}
	if agentInfoName != "codex" {
		t.Fatalf("agentInfo.name=%q, want codex", agentInfoName)
	}

	pref, err := store.LoadAgentPreference(ctx, "proj1", "codex")
	if err != nil {
		t.Fatalf("LoadAgentPreference proj1 codex: %v", err)
	}
	if pref == nil || !strings.Contains(pref.PreferenceJSON, "gpt-5") {
		t.Fatalf("pref=%+v, want existing codex preference", pref)
	}
	pref, err = store.LoadAgentPreference(ctx, "proj2", "codex")
	if err != nil {
		t.Fatalf("LoadAgentPreference proj2 codex: %v", err)
	}
	if pref == nil || !strings.Contains(pref.PreferenceJSON, "only-legacy") {
		t.Fatalf("pref=%+v, want renamed legacy preference", pref)
	}
}

func TestSQLiteStoreSkipsCodexAppMigrationWhenUserVersionIsCurrent(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "client.sqlite3")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore seed: %v", err)
	}
	sqliteStore := store.(*sqliteStore)
	if _, err := sqliteStore.db.ExecContext(ctx, fmt.Sprintf(`PRAGMA user_version = %d`, sqliteMigrationVersionCodexAppIdentity)); err != nil {
		t.Fatalf("set user_version: %v", err)
	}
	if _, err := sqliteStore.db.ExecContext(ctx, `
		INSERT INTO sessions (id, project_name, status, agent_type, agent_json, session_sync_json, title, created_at, updated_at)
		VALUES ('sess-sentinel', 'proj1', 2, 'codexapp', '{}', '{}', 'sentinel', '2026-05-20T00:00:00Z', '2026-05-20T00:00:00Z');
	`); err != nil {
		t.Fatalf("seed sentinel row: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close seed store: %v", err)
	}

	store, err = NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore reopened: %v", err)
	}
	defer store.Close()
	sqliteStore = store.(*sqliteStore)

	var raw string
	if err := sqliteStore.db.QueryRowContext(ctx, `SELECT agent_type FROM sessions WHERE id = 'sess-sentinel'`).Scan(&raw); err != nil {
		t.Fatalf("query sentinel: %v", err)
	}
	if raw != "codexapp" {
		t.Fatalf("agent_type=%q, want skipped sentinel codexapp", raw)
	}
}
```

- [ ] **Step 2: Run RED**

Run: `go test ./internal/hub/client -run "CodexAppIdentity|ParseACPProviderCodexAliases" -count=1`

Expected: fails because migration constants/functions do not exist or rows remain `codexapp`.

- [ ] **Step 3: Implement migration**

In `sqlite_store.go`:

- Add `const sqliteMigrationVersionCodexAppIdentity = 1`.
- Call `runSQLiteDataMigrations(db)` after schema validation and before returning `sqliteStore`.
- `runSQLiteDataMigrations` checks `PRAGMA user_version`; if current version is at least the migration version, return immediately.
- The migration runs direct-column SQL updates, handles `agent_preferences` conflicts, then updates parseable `sessions.agent_json` rows where `agentInfo.name` is `codexapp`.
- Set `PRAGMA user_version = 1` only after migration work completes.

- [ ] **Step 4: Run GREEN**

Run: `go test ./internal/hub/client -run "CodexAppIdentity" -count=1`

Expected: tests pass.

### Task 2: Remove Server Runtime Compatibility

**Files:**
- Modify: `server/internal/protocol/acp_const.go`
- Modify: `server/internal/hub/agent/skills.go`
- Modify: `server/internal/hub/client/sqlite_store.go`
- Test: `server/internal/hub/agent/agent_test.go`
- Test: `server/internal/hub/client/client_test.go`

- [ ] **Step 1: Update failing expectations**

Change existing tests so `codexapp` is rejected after migration:

```go
func TestParseACPProviderCodexAliases(t *testing.T) {
	provider, ok := protocol.ParseACPProvider("codex")
	if !ok {
		t.Fatal("ParseACPProvider(codex) returned ok=false")
	}
	if provider != protocol.ACPProviderCodex {
		t.Fatalf("provider=%q, want %q", provider, protocol.ACPProviderCodex)
	}
	if _, ok := protocol.ParseACPProvider("codexapp"); ok {
		t.Fatal("ParseACPProvider accepted removed codexapp alias")
	}
	if _, ok := protocol.ParseACPProvider("codex-app"); ok {
		t.Fatal("ParseACPProvider accepted legacy codex-app alias")
	}
}
```

Add a client request test:

```go
func TestHandleSessionRequestRejectsCodexAppAfterMigration(t *testing.T) {
	c := newTestClient(t)
	_, err := c.HandleSessionRequest(context.Background(), "session.new", "proj1", json.RawMessage(`{"agentType":"codexapp"}`))
	if err == nil || !strings.Contains(err.Error(), `no agent registered for "codexapp"`) {
		t.Fatalf("session.new err=%v, want codexapp rejection", err)
	}
}
```

- [ ] **Step 2: Run RED**

Run: `go test ./internal/hub/agent ./internal/hub/client -run "CodexAliases|RejectsCodexApp|CodexAppIdentity" -count=1`

Expected: fails while `codexapp` alias/fallback remains accepted.

- [ ] **Step 3: Remove server compatibility**

Remove:

- `case "codexapp"` from `ParseACPProvider`
- `"codexapp"` case from `providerPresetByName`
- `normalizeAgentType` mapping from `codexapp` to `codex`
- fallback query from `LoadAgentPreference` that looks up `codexapp`

Keep internal `codexapp*` app-server bridge implementation names.

- [ ] **Step 4: Run GREEN**

Run: `go test ./internal/hub/agent ./internal/hub/client -run "CodexAliases|RejectsCodexApp|CodexAppIdentity" -count=1`

Expected: tests pass.

### Task 3: Remove Web Runtime Compatibility

**Files:**
- Modify: `app/web/src/services/registryRepository.ts`
- Modify: `app/web/src/main.tsx`
- Test: `app/__tests__/web-chat-ui.test.ts`
- Test: `app/__tests__/web-agent-package-update-settings.test.ts`

- [ ] **Step 1: Update failing web tests**

Change tests so they assert `codexapp` is no longer normalized:

```ts
test('does not rewrite removed codexapp agent names in web payloads', () => {
  expect(repositoryTs).not.toContain("return normalized.toLowerCase() === 'codexapp' ? 'codex' : normalized;");
  expect(mainTsx).not.toContain("return normalized.toLowerCase() === 'codexapp' ? 'codex' : normalized;");
});
```

- [ ] **Step 2: Run RED**

Run: `npm test -- --runTestsByPath __tests__/web-chat-ui.test.ts __tests__/web-agent-package-update-settings.test.ts --runInBand`

Expected: fails while web normalization still maps `codexapp` to `codex`.

- [ ] **Step 3: Remove web compatibility**

In `registryRepository.ts`, make `normalizeAgentType` only trim and reject empty non-string values.

In `main.tsx`, make `normalizeAgentTypeName` only trim. Keep using it to centralize whitespace handling, but remove `codexapp` mapping.

Update brittle source-structure tests to reflect the new behavior.

- [ ] **Step 4: Run GREEN**

Run: `npm test -- --runTestsByPath __tests__/web-chat-ui.test.ts __tests__/web-agent-package-update-settings.test.ts --runInBand`

Expected: tests pass.

### Task 4: Full Verification and Completion Gate

**Files:**
- No new code files.

- [ ] **Step 1: Search for public compatibility remnants**

Run:

```powershell
rg -n "codexapp|codex-app" app server docs --glob '!**/dist/**' --glob '!**/node_modules/**' --glob '!server/cmd/wheelmaker-desktop/webroot/**'
```

Expected: only internal `codexapp*` bridge implementation names, migration tests/specs/plans, and historical docs remain.

- [ ] **Step 2: Full tests**

Run:

```powershell
cd server; go test ./...
cd ../app; npm test -- --runInBand
npm run tsc:web
cd ..; git diff --check
```

Expected: all commands exit 0.

- [ ] **Step 3: Commit and publish**

Run from repo root:

```powershell
git add -A
git commit -m "feat: migrate codexapp identity once"
git push origin main
cd app; npm run build:web:release
cd ..; powershell -NoProfile -ExecutionPolicy Bypass -File scripts/signal_update_now.ps1 -DelaySeconds 30 -SkipWebPublish
git status --short
```

Expected: commit pushed, web release exported, update signal accepted, clean working tree.
