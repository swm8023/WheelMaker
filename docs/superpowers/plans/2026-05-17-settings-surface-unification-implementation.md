# Settings Surface Unification Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement the approved Settings surface unification for `app/web/`.

**Architecture:** Keep the existing desktop sidebar and mobile fullscreen shells, but render one shared sectioned Settings content model from `app/web/src/main.tsx`. Add `database` as a Settings detail route, unify detail page markup/classes, and replace the local-cache `window.confirm` with a shared confirm dialog shell used by both Archive and Clear Local Cache.

**Tech Stack:** React 19, TypeScript, CSS, Jest source-structure tests, webpack web build.

---

## File Map

- Modify: `app/web/src/main.tsx`
  - Add `database` to `SettingsDetailView`.
  - Add a shared confirm target type for archive and clear-cache confirmations.
  - Render sectioned Settings rows in one shared content renderer.
  - Render `CC Switch`, `Token Stats`, and `Database` through one detail shell.
  - Move Database dump UI from inline expansion to the `database` detail route.
  - Route Clear Local Cache through the shared confirm dialog.
- Modify: `app/web/src/styles.css`
  - Add compact Settings section/row/detail styles.
  - Add shared metadata-list classes for Token Stats and CC Switch.
  - Generalize archive confirm classes to shared app-confirm classes while keeping archive behavior.
- Modify: `app/__tests__/web-clear-local-cache-settings.test.ts`
  - Lock styled confirm dialog behavior and preserved cache identity.
- Modify: `app/__tests__/web-chat-ui.test.ts`
  - Lock Settings section order, detail route support, shared renderer, and shared confirm shell.
- Modify: `app/__tests__/web-shiki-theme-settings.test.ts`
  - Keep Code Display direct controls assertions aligned with sectioned Settings.
- Modify: `app/__tests__/web-hide-tool-calls-settings.test.ts`
  - Keep Hide Tool Calls assertion aligned with the Chat section.

## Task 1: Add Failing Settings Structure Tests

**Files:**
- Modify: `app/__tests__/web-chat-ui.test.ts`
- Modify: `app/__tests__/web-clear-local-cache-settings.test.ts`
- Modify: `app/__tests__/web-shiki-theme-settings.test.ts`
- Modify: `app/__tests__/web-hide-tool-calls-settings.test.ts`

- [ ] **Step 1: Add source assertions for the new Settings information architecture**

In `app/__tests__/web-chat-ui.test.ts`, update the existing mobile settings coverage to assert:

```ts
expect(mainTsx).toContain("type SettingsDetailView = 'tokenStats' | 'ccSwitch' | 'database' | null;");
expect(mainTsx).toContain('const renderSettingsSection = (title: string, rows: React.ReactNode) => (');
expect(mainTsx).toContain('renderSettingsSection(\'Appearance\'');
expect(mainTsx).toContain('renderSettingsSection(\'Chat\'');
expect(mainTsx).toContain('renderSettingsSection(\'Code Display\'');
expect(mainTsx).toContain('renderSettingsSection(\'Storage\'');
expect(mainTsx.indexOf("renderSettingsSection('Appearance'")).toBeLessThan(mainTsx.indexOf("renderSettingsSection('Chat'")));
expect(mainTsx.indexOf("renderSettingsSection('Chat'")).toBeLessThan(mainTsx.indexOf("renderSettingsSection('Code Display'")));
expect(mainTsx.indexOf("renderSettingsSection('Code Display'")).toBeLessThan(mainTsx.indexOf("renderSettingsSection('Storage'")));
expect(mainTsx).toContain("setSettingsDetailView('database');");
expect(mainTsx).toContain("settingsDetailView === 'database'");
expect(mainTsx).toContain('renderDatabaseSettingsDetail()');
expect(mainTsx).toContain('className="settings-section-title"');
expect(mainTsx).toContain('className="settings-row settings-detail-row"');
expect(stylesCss).toContain('.settings-section-title {');
expect(stylesCss).toContain('.settings-row {');
expect(stylesCss).toContain('.settings-danger-row {');
expect(stylesCss).toContain('.settings-metadata-list {');
expect(stylesCss).toContain('.settings-database-dump {');
```

- [ ] **Step 2: Add source assertions for shared confirm dialog behavior**

In `app/__tests__/web-clear-local-cache-settings.test.ts`, replace the old `window.confirm` assertions with:

```ts
expect(mainTsx).toContain('Clear Local Cache');
expect(mainTsx).not.toContain('Clear Local Cache (Keep Token)');
expect(mainTsx).not.toContain('window.confirm(');
expect(mainTsx).toContain("type ConfirmTarget =");
expect(mainTsx).toContain("kind: 'clearCache'");
expect(mainTsx).toContain("setConfirmTarget({kind: 'clearCache'});");
expect(mainTsx).toContain('Clear local cache?');
expect(mainTsx).toContain('Token and server address will be preserved.');
expect(mainTsx).toContain('workspaceStore.clearLocalCachePreservingToken();');
expect(mainTsx).toContain('window.location.reload();');
expect(mainTsx).toContain('const appConfirmDialog = confirmTarget ? (');
expect(mainTsx).toContain('className="app-confirm-backdrop"');
expect(mainTsx).toContain('className="app-confirm-btn primary danger"');
```

- [ ] **Step 3: Keep existing settings-specific tests aligned**

In `app/__tests__/web-shiki-theme-settings.test.ts`, add:

```ts
expect(mainTsx).toContain("renderSettingsSection('Code Display'");
```

In `app/__tests__/web-hide-tool-calls-settings.test.ts`, add:

```ts
expect(mainTsx).toContain("renderSettingsSection('Chat'");
```

- [ ] **Step 4: Run the targeted tests and verify they fail for missing implementation**

Run:

```bash
cd app
npm test -- --runInBand web-chat-ui.test.ts web-clear-local-cache-settings.test.ts web-shiki-theme-settings.test.ts web-hide-tool-calls-settings.test.ts
```

Expected: FAIL because `database` detail route, section renderer, shared confirm dialog, and new classes are not implemented yet.

## Task 2: Implement Sectioned Settings and Detail Routes

**Files:**
- Modify: `app/web/src/main.tsx`

- [ ] **Step 1: Extend Settings and confirm types/state**

Change the settings detail type to:

```ts
type SettingsDetailView = 'tokenStats' | 'ccSwitch' | 'database' | null;
```

Add a shared confirm target type:

```ts
type ConfirmTarget =
  | {
      kind: 'archive';
      projectId: string;
      sessionId: string;
      title: string;
    }
  | {kind: 'clearCache'};
```

Replace archive confirm state with:

```ts
const [confirmTarget, setConfirmTarget] = useState<ConfirmTarget | null>(null);
const [confirmError, setConfirmError] = useState('');
```

- [ ] **Step 2: Add reusable Settings section, row, and detail helpers inside `App`**

Add helper renderers near the current Settings renderers:

```tsx
const renderSettingsSection = (title: string, rows: React.ReactNode) => (
  <section className="settings-section" aria-label={title}>
    <div className="settings-section-title">{title}</div>
    <div className="settings-section-rows">{rows}</div>
  </section>
);
```

Keep switches/selects as direct JSX rows so existing state wiring remains unchanged. Use `className="settings-row sidebar-setting-row"` for switch/select rows, `className="settings-row settings-detail-row"` for detail buttons, and `className="settings-row settings-danger-row"` for Clear Local Cache.

- [ ] **Step 3: Add shared detail shell**

Add:

```tsx
const renderSettingsDetailShell = (
  title: string,
  content: React.ReactNode,
  actions?: React.ReactNode,
) => (
  <div className="settings-detail-page">
    <div className="settings-detail-header">
      <button
        type="button"
        className="mobile-settings-back settings-detail-back"
        onClick={() => setSettingsDetailView(null)}
        aria-label="Back to settings"
        title="Back"
      >
        <span className="codicon codicon-arrow-left" />
      </button>
      <div className="settings-detail-title">{title}</div>
      {actions ?? <span className="settings-detail-header-spacer" aria-hidden="true" />}
    </div>
    <div className="settings-detail-body">{content}</div>
  </div>
);
```

- [ ] **Step 4: Convert `Token Stats` and `CC Switch` details to the shared detail shell**

Use `renderSettingsDetailShell('Token Stats', ..., refreshButton)` for token stats.

For `CC Switch`, replace `token-stats-account-list`, `token-stats-account-item`, and `token-stats-card-line` general layout classes with `settings-metadata-list`, `settings-metadata-card`, and `settings-metadata-line`. Keep `token-stats-pill` only for token-specific pills in Token Stats.

- [ ] **Step 5: Move Database into a detail renderer**

Create `renderDatabaseSettingsDetail()` that returns:

```tsx
return renderSettingsDetailShell(
  'Database',
  <>
    {databaseLoading ? <div className="muted block">Loading database...</div> : null}
    {databaseError ? <div className="error">Database error: {databaseError}</div> : null}
    {!databaseLoading && !databaseError ? (
      <pre className="settings-database-dump">{databaseDumpText}</pre>
    ) : null}
  </>,
  <button
    type="button"
    className="git-section-btn"
    onClick={exportDatabaseDump}
    disabled={databaseLoading || !!databaseError || !databaseDumpText}
    title="Export current database dump"
  >
    Export
  </button>,
);
```

The `Database` row should call:

```ts
setSettingsDetailView('database');
openDatabasePanel();
```

- [ ] **Step 6: Render the sectioned main Settings list**

Make `renderSettingsContent` route details first:

```ts
if (settingsDetailView === 'ccSwitch') return renderCCSwitchSettingsDetail();
if (settingsDetailView === 'tokenStats') return renderTokenStatsSettingsDetail();
if (settingsDetailView === 'database') return renderDatabaseSettingsDetail();
```

Then render sections in the approved order:

```tsx
<div className="settings-list">
  {renderSettingsSection('Appearance', ...)}
  {renderSettingsSection('Chat', ...)}
  {renderSettingsSection('Code Display', ...)}
  {renderSettingsSection('Storage', ...)}
</div>
```

Use Chat row order `Hide Tool Calls`, `CC Switch`, `Token Stats`. Use Storage row order `Database`, `Clear Local Cache`.

- [ ] **Step 7: Run targeted tests and expect remaining confirm/CSS failures**

Run:

```bash
cd app
npm test -- --runInBand web-chat-ui.test.ts web-clear-local-cache-settings.test.ts web-shiki-theme-settings.test.ts web-hide-tool-calls-settings.test.ts
```

Expected: some tests may still fail until shared confirm and CSS are implemented.

## Task 3: Implement Shared Confirm Dialog and Styles

**Files:**
- Modify: `app/web/src/main.tsx`
- Modify: `app/web/src/styles.css`

- [ ] **Step 1: Replace archive-specific confirm rendering with shared confirm rendering**

Archive request should set:

```ts
setConfirmError('');
setConfirmTarget({
  kind: 'archive',
  projectId: targetProjectId,
  sessionId: normalizedSessionId,
  title: (session.title || '').trim(),
});
```

Clear Local Cache should be:

```ts
const requestClearLocalCache = () => {
  setConfirmError('');
  setConfirmTarget({kind: 'clearCache'});
};
```

Confirm handler should branch by `confirmTarget.kind`, preserving archive behavior and running cache clear/reload for `clearCache`.

- [ ] **Step 2: Render shared confirm dialog copy and variants**

Use `const appConfirmDialog = confirmTarget ? (...) : null;`.

Archive dialog content:

- title: `Archive session?`
- name: archive title or `Untitled session`
- copy: `Archived sessions leave the chat list.`
- icon: `codicon-archive`
- primary label: `Archive`
- primary class: `app-confirm-btn primary`

Clear cache dialog content:

- title: `Clear local cache?`
- name: `Token and server address will be preserved.`
- copy: `The app will reload after local cached workspace data is cleared.`
- icon: `codicon-trash`
- primary label: `Clear Cache`
- primary class: `app-confirm-btn primary danger`

- [ ] **Step 3: Update CSS to shared Settings and confirm classes**

Add or replace styles for:

```css
.settings-list { ... }
.settings-section { ... }
.settings-section-title { ... }
.settings-section-rows { ... }
.settings-row { ... }
.settings-row:hover { ... }
.settings-danger-row { ... }
.settings-danger-row:hover { ... }
.settings-detail-body { ... }
.settings-detail-header-spacer { ... }
.settings-metadata-list { ... }
.settings-metadata-card { ... }
.settings-metadata-line { ... }
.settings-metadata-line-primary { ... }
.settings-metadata-line-tags { ... }
.settings-metadata-title { ... }
.settings-metadata-error { ... }
.settings-database-dump { ... }
.app-confirm-backdrop { ... }
.app-confirm-dialog { ... }
.app-confirm-icon { ... }
.app-confirm-icon.danger { ... }
.app-confirm-content { ... }
.app-confirm-title { ... }
.app-confirm-name { ... }
.app-confirm-copy { ... }
.app-confirm-error { ... }
.app-confirm-actions { ... }
.app-confirm-btn { ... }
.app-confirm-btn.secondary { ... }
.app-confirm-btn.primary { ... }
.app-confirm-btn.primary.danger { ... }
```

Remove or stop using `.session-archive-confirm-*` for the dialog. Keep no duplicate unrelated confirm style.

- [ ] **Step 4: Run targeted tests and verify they pass**

Run:

```bash
cd app
npm test -- --runInBand web-chat-ui.test.ts web-clear-local-cache-settings.test.ts web-shiki-theme-settings.test.ts web-hide-tool-calls-settings.test.ts
```

Expected: PASS.

## Task 4: Full Verification and Completion Gate

**Files:**
- No source edits unless verification exposes a defect.

- [ ] **Step 1: Run TypeScript check**

Run:

```bash
cd app
npm run tsc:web
```

Expected: exit 0.

- [ ] **Step 2: Run focused tests**

Run:

```bash
cd app
npm test -- --runInBand web-chat-ui.test.ts web-clear-local-cache-settings.test.ts web-shiki-theme-settings.test.ts web-hide-tool-calls-settings.test.ts
```

Expected: exit 0.

- [ ] **Step 3: Commit and push**

Run:

```bash
git add -A
git commit -m "feat(web): unify settings surface"
git push origin main
```

Expected: commit and push succeed.

- [ ] **Step 4: Run web release build**

Run:

```bash
cd app
npm run build:web:release
```

Expected: exit 0 and web assets exported to `~/.wheelmaker/web`.
