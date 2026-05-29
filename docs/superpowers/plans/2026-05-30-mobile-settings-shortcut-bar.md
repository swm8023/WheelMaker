# Mobile Settings Shortcut Bar Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a Settings-only mobile bottom shortcut bar, remove duplicate shortcut rows from Settings `More`, move maintenance rows into `Debug`, and simplify the mobile Chat header shortcuts.

**Architecture:** Keep the change inside the existing React source-structure pattern in `app/web/src/main.tsx` and `app/web/src/styles.css`. Add no new routing or data model; all shortcuts reuse existing `settingsDetailView` detail pages and existing refresh/detail action behavior.

**Follow-up:** The bar uses visible labels below icons, a top active indicator, and replaces mobile Settings detail history entries when users switch between shortcuts so back exits predictably.

**Tech Stack:** React, TypeScript, CSS, Jest source-structure tests, npm scripts in `app/`.

---

## File Structure

- Modify `app/__tests__/web-agent-package-update-settings.test.ts`: update source-structure tests for mobile Settings bar, Chat toolbar cleanup, and Settings IA migration.
- Modify `app/__tests__/web-skill-management-settings.test.ts`: stop asserting `Skills` lives in `More`; assert it is in the mobile Settings shortcut bar.
- Modify `app/__tests__/web-port-relay-settings.test.ts`: stop asserting `Port Relay` lives in `More`; assert it is in the mobile Settings shortcut bar.
- Modify `app/__tests__/web-registry-debug-settings.test.ts`: assert `Debug` follows `Code Display` and includes `Database` plus `Clear Local Cache`.
- Modify `app/__tests__/web-chat-ui.test.ts`: update mobile Settings and Chat toolbar source-structure expectations.
- Modify `app/web/src/main.tsx`: render the mobile Settings shortcut bar, remove the old `More` section, move `Database` and `Clear Local Cache` rows to `Debug`, and remove mobile Chat header `Update`/`Port Relay` buttons.
- Modify `app/web/src/styles.css`: add mobile Settings shortcut bar styles and content bottom padding; restore mobile Chat Settings button to the shared `drawer-settings-icon-btn` visual treatment.

---

### Task 1: Write Failing Source-Structure Tests

**Files:**
- Modify: `app/__tests__/web-agent-package-update-settings.test.ts`
- Modify: `app/__tests__/web-skill-management-settings.test.ts`
- Modify: `app/__tests__/web-port-relay-settings.test.ts`
- Modify: `app/__tests__/web-registry-debug-settings.test.ts`
- Modify: `app/__tests__/web-chat-ui.test.ts`

- [ ] **Step 1: Update agent package settings tests**

Replace the first test in `app/__tests__/web-agent-package-update-settings.test.ts` with:

```ts
test('moves shortcut details out of More and keeps Chat focused on chat options', () => {
  const projectRoot = path.join(__dirname, '..');
  const mainTsx = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'main.tsx'), 'utf8');

  expect(mainTsx).toContain("type SettingsDetailView = 'update' | 'skills' | 'tokenStats' | 'ccSwitch' | 'database' | 'portRelay' | 'debugLogs' | null;");
  expect(mainTsx).toContain("settingsDetailView === 'update'");
  expect(mainTsx).toContain('renderUpdateSettingsDetail(options)');
  expect(mainTsx).not.toContain("renderSettingsSection('More'");
  expect(mainTsx).not.toContain("renderSettingsSection('Storage'");

  const chatStart = mainTsx.indexOf("renderSettingsSection('Chat'");
  const codeDisplayStart = mainTsx.indexOf("renderSettingsSection('Code Display'", chatStart);
  expect(chatStart).toBeGreaterThanOrEqual(0);
  expect(codeDisplayStart).toBeGreaterThan(chatStart);
  const chatSection = mainTsx.slice(chatStart, codeDisplayStart);
  expect(chatSection).not.toContain('Use Latest Prompt Title');
  expect(chatSection).toContain('Hide Tool Calls');
  expect(chatSection).not.toContain('Token Stats');
  expect(chatSection).not.toContain('CC Switch');

  const debugStart = mainTsx.indexOf("renderSettingsSection('Debug'");
  expect(debugStart).toBeGreaterThan(codeDisplayStart);
  const debugSection = mainTsx.slice(debugStart);
  expect(debugSection).not.toContain("setSettingsDetailView('update')");
  expect(debugSection).not.toContain("setSettingsDetailView('skills')");
  expect(debugSection).not.toContain("setSettingsDetailView('tokenStats')");
  expect(debugSection).not.toContain("setSettingsDetailView('ccSwitch')");
  expect(debugSection).not.toContain("setSettingsDetailView('portRelay')");
  expect(debugSection.indexOf("setSettingsDetailView('database')")).toBeGreaterThanOrEqual(0);
  expect(debugSection.indexOf("setSettingsDetailView('database')")).toBeLessThan(debugSection.indexOf('requestClearLocalCache'));
  expect(debugSection.indexOf('requestClearLocalCache')).toBeLessThan(debugSection.indexOf('handleRegistryDebugLogout'));
});
```

Replace the desktop/mobile shortcut test in the same file with assertions for the mobile Settings bar:

```ts
test('adds desktop shortcuts and a mobile Settings-only shortcut bar', () => {
  const projectRoot = path.join(__dirname, '..');
  const mainTsx = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'main.tsx'), 'utf8');
  const stylesCss = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'styles.css'), 'utf8');

  const activityBarStart = mainTsx.indexOf('const desktopActivityBar = isWide ? (');
  const activityBarEnd = mainTsx.indexOf('const floatingControlStack = !isWide ? (', activityBarStart);
  expect(activityBarStart).toBeGreaterThanOrEqual(0);
  expect(activityBarEnd).toBeGreaterThan(activityBarStart);
  const activityBar = mainTsx.slice(activityBarStart, activityBarEnd);

  expect(activityBar).toContain('codicon-cloud-download');
  expect(activityBar).toContain('codicon-graph-line');
  expect(activityBar).toContain('codicon-radio-tower');
  expect(activityBar).toContain("openSettingsDetail('update')");
  expect(activityBar).toContain("openSettingsDetail('tokenStats')");
  expect(activityBar).toContain('handleDesktopPortRelaySelect');

  const floatingStart = mainTsx.indexOf('const floatingControlStack = !isWide ? (');
  const mobileSettingsStart = mainTsx.indexOf('const mobileSettingsScreen = !isWide && sidebarSettingsOpen ? (', floatingStart);
  const mobileOnly = mainTsx.slice(floatingStart, mobileSettingsStart);
  expect(mobileOnly).not.toContain("openSettingsDetail('update')");

  const mobileBarStart = mainTsx.indexOf('const mobileSettingsShortcutBar = !isWide && sidebarSettingsOpen ? (');
  const mobileBarEnd = mainTsx.indexOf('const mobileSettingsScreen = !isWide && sidebarSettingsOpen ? (', mobileBarStart);
  expect(mobileBarStart).toBeGreaterThanOrEqual(0);
  expect(mobileBarEnd).toBeGreaterThan(mobileBarStart);
  const mobileBar = mainTsx.slice(mobileBarStart, mobileBarEnd);
  expect(mobileBar.indexOf('title="Settings"')).toBeLessThan(mobileBar.indexOf('title="Update"'));
  expect(mobileBar.indexOf('title="Update"')).toBeLessThan(mobileBar.indexOf('title="Skills"'));
  expect(mobileBar.indexOf('title="Skills"')).toBeLessThan(mobileBar.indexOf('title="Port Relay"'));
  expect(mobileBar.indexOf('title="Port Relay"')).toBeLessThan(mobileBar.indexOf('title="Token Stats"'));
  expect(mobileBar.indexOf('title="Token Stats"')).toBeLessThan(mobileBar.indexOf('title="CC Switch"'));
  expect(mobileBar).toContain("openSettingsDetail('update')");
  expect(mobileBar).toContain("openSettingsDetail('skills')");
  expect(mobileBar).toContain("openSettingsDetail('portRelay')");
  expect(mobileBar).toContain("openSettingsDetail('tokenStats')");
  expect(mobileBar).toContain("openSettingsDetail('ccSwitch')");
  expect(mobileBar).toContain('setSettingsDetailView(null);');

  const mobileToolbarStart = mainTsx.indexOf('<div className="mobile-chat-toolbar"');
  const mobileToolbarEnd = mainTsx.indexOf('{renderChatHubSummary()}', mobileToolbarStart);
  const mobileToolbar = mainTsx.slice(mobileToolbarStart, mobileToolbarEnd);
  expect(mobileToolbar).toContain('title="Open settings"');
  expect(mobileToolbar).not.toContain('title="Update"');
  expect(mobileToolbar).not.toContain('title="Port Relay"');
  expect(mobileToolbar).not.toContain("openSettingsDetail('update')");
  expect(mobileToolbar).not.toContain("openSettingsDetail('portRelay')");

  expect(stylesCss).toContain('.mobile-settings-shortcut-bar {');
  expect(stylesCss).toContain('.mobile-settings-shortcut-button {');
  expect(stylesCss).toContain('.mobile-settings-shortcut-button.active::before');
  expect(stylesCss).toContain('padding: 0 0 env(safe-area-inset-bottom, 0px);');
});
```

- [ ] **Step 2: Update Skills, Port Relay, Debug, and Chat tests**

Change `app/__tests__/web-skill-management-settings.test.ts` first test to assert Skills is in the mobile shortcut bar:

```ts
test('adds Skills as a settings detail and mobile shortcut bar entry', () => {
  expect(mainTsx).toContain("type SettingsDetailView = 'update' | 'skills' | 'tokenStats' | 'ccSwitch' | 'database' | 'portRelay' | 'debugLogs' | null;");
  expect(mainTsx).toContain("settingsDetailView === 'skills'");
  expect(mainTsx).toContain('renderSkillsSettingsDetail(options)');
  expect(mainTsx).not.toContain("renderSettingsSection('More'");

  const mobileBarStart = mainTsx.indexOf('const mobileSettingsShortcutBar = !isWide && sidebarSettingsOpen ? (');
  const mobileBarEnd = mainTsx.indexOf('const mobileSettingsScreen = !isWide && sidebarSettingsOpen ? (', mobileBarStart);
  const mobileBar = mainTsx.slice(mobileBarStart, mobileBarEnd);
  expect(mobileBar.indexOf('title="Update"')).toBeLessThan(mobileBar.indexOf('title="Skills"'));
  expect(mobileBar.indexOf('title="Skills"')).toBeLessThan(mobileBar.indexOf('title="Port Relay"'));
  expect(mobileBar).toContain("openSettingsDetail('skills')");
});
```

Change `app/__tests__/web-port-relay-settings.test.ts` first test to:

```ts
test('adds Port Relay as a settings detail and mobile shortcut bar entry', () => {
  expect(mainTsx).toContain("type SettingsDetailView = 'update' | 'skills' | 'tokenStats' | 'ccSwitch' | 'database' | 'portRelay' | 'debugLogs' | null;");
  expect(mainTsx).toContain("settingsDetailView === 'portRelay'");
  expect(mainTsx).toContain('renderPortRelaySettingsDetail(options)');
  expect(mainTsx).toContain("setSettingsDetailView('portRelay')");
  expect(mainTsx).not.toContain("renderSettingsSection('More'");

  const mobileBarStart = mainTsx.indexOf('const mobileSettingsShortcutBar = !isWide && sidebarSettingsOpen ? (');
  const mobileBarEnd = mainTsx.indexOf('const mobileSettingsScreen = !isWide && sidebarSettingsOpen ? (', mobileBarStart);
  const mobileBar = mainTsx.slice(mobileBarStart, mobileBarEnd);
  expect(mobileBar.indexOf('title="Skills"')).toBeLessThan(mobileBar.indexOf('title="Port Relay"'));
  expect(mobileBar.indexOf('title="Port Relay"')).toBeLessThan(mobileBar.indexOf('title="Token Stats"'));
  expect(mobileBar).toContain("openSettingsDetail('portRelay')");
});
```

Change the bottom-order test in `app/__tests__/web-registry-debug-settings.test.ts` to:

```ts
test('places debug maintenance settings at the bottom after code display', () => {
  const mainTsx = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'main.tsx'), 'utf8');

  const codeDisplaySectionIndex = mainTsx.indexOf("renderSettingsSection('Code Display'");
  const debugSectionIndex = mainTsx.indexOf("renderSettingsSection('Debug'");
  expect(debugSectionIndex).toBeGreaterThan(codeDisplaySectionIndex);
  expect(mainTsx).not.toContain("renderSettingsSection('More'");

  const debugSection = mainTsx.slice(debugSectionIndex);
  expect(debugSection.indexOf('Open Debug Panel')).toBeLessThan(debugSection.indexOf('Logs'));
  expect(debugSection.indexOf('Logs')).toBeLessThan(debugSection.indexOf('Database'));
  expect(debugSection.indexOf('Database')).toBeLessThan(debugSection.indexOf('Clear Local Cache'));
  expect(debugSection.indexOf('Clear Local Cache')).toBeLessThan(debugSection.indexOf('Logout'));
});
```

Update `app/__tests__/web-chat-ui.test.ts` expectations so they contain `renderSettingsSection('Debug'` instead of `renderSettingsSection('More'`, assert `codeDisplaySettingsIndex < debugSettingsIndex`, and assert the mobile Chat toolbar block does not contain `title="Update"` or `title="Port Relay"`.

- [ ] **Step 3: Run tests and verify RED**

Run:

```powershell
cd app
npm test -- --runTestsByPath __tests__/web-agent-package-update-settings.test.ts __tests__/web-skill-management-settings.test.ts __tests__/web-port-relay-settings.test.ts __tests__/web-registry-debug-settings.test.ts __tests__/web-chat-ui.test.ts --runInBand
```

Expected: FAIL because production code still renders `More`, lacks `mobileSettingsShortcutBar`, and mobile Chat still contains `Update` / `Port Relay` toolbar buttons.

---

### Task 2: Implement Mobile Settings Shortcut Bar And IA Migration

**Files:**
- Modify: `app/web/src/main.tsx`
- Modify: `app/web/src/styles.css`

- [ ] **Step 1: Add a mobile Settings shortcut bar renderer**

In `app/web/src/main.tsx`, immediately before `const mobileSettingsTitle = ...`, add:

```tsx
  const mobileSettingsShortcutBar = !isWide && sidebarSettingsOpen ? (
    <nav className="mobile-settings-shortcut-bar" aria-label="Settings shortcuts">
      <button
        type="button"
        className={`mobile-settings-shortcut-button${settingsDetailView === null ? ' active' : ''}`}
        onClick={() => {
          setSettingsDetailView(null);
        }}
        title="Settings"
        aria-label="Settings"
      >
        <span className="codicon codicon-settings-gear" />
      </button>
      <button
        type="button"
        className={`mobile-settings-shortcut-button${settingsDetailView === 'update' ? ' active' : ''}`}
        onClick={() => openSettingsDetail('update')}
        title="Update"
        aria-label="Update"
      >
        <span className="codicon codicon-cloud-download" />
      </button>
      <button
        type="button"
        className={`mobile-settings-shortcut-button${settingsDetailView === 'skills' ? ' active' : ''}`}
        onClick={() => openSettingsDetail('skills')}
        title="Skills"
        aria-label="Skills"
      >
        <span className="codicon codicon-extensions" />
      </button>
      <button
        type="button"
        className={`mobile-settings-shortcut-button${settingsDetailView === 'portRelay' ? ' active' : ''}`}
        onClick={() => openSettingsDetail('portRelay')}
        title="Port Relay"
        aria-label="Port Relay"
      >
        <span className="codicon codicon-radio-tower" />
      </button>
      <button
        type="button"
        className={`mobile-settings-shortcut-button${settingsDetailView === 'tokenStats' ? ' active' : ''}`}
        onClick={() => openSettingsDetail('tokenStats')}
        title="Token Stats"
        aria-label="Token Stats"
      >
        <span className="codicon codicon-graph-line" />
      </button>
      <button
        type="button"
        className={`mobile-settings-shortcut-button${settingsDetailView === 'ccSwitch' ? ' active' : ''}`}
        onClick={() => openSettingsDetail('ccSwitch')}
        title="CC Switch"
        aria-label="CC Switch"
      >
        <span className="codicon codicon-arrow-swap" />
      </button>
    </nav>
  ) : null;
```

Render `{mobileSettingsShortcutBar}` after `.mobile-settings-scroll` inside `mobileSettingsScreen`.

- [ ] **Step 2: Remove `More` and move maintenance rows into `Debug`**

Delete the `renderSettingsSection('More', ...)` block from `renderSettingsContent`.

In the existing `renderSettingsSection('Debug', ...)` block, add the `Database` row after `Logs` and before `Clear Local Cache`, then add `Clear Local Cache` before `Logout`:

```tsx
        <button
          type="button"
          className="settings-row settings-detail-row"
          onClick={() => {
            setSettingsDetailView('database');
            openDatabasePanel();
          }}
        >
          <span>
            <span className="codicon codicon-database settings-row-icon" aria-hidden="true" />
            Database
          </span>
          <span className="codicon codicon-chevron-right" />
        </button>
        <button
          type="button"
          className="settings-row settings-danger-row"
          onClick={requestClearLocalCache}
        >
          <span>
            <span className="codicon codicon-trash settings-row-icon" aria-hidden="true" />
            Clear Local Cache
          </span>
          <span className="codicon codicon-chevron-right" />
        </button>
```

- [ ] **Step 3: Simplify the mobile Chat toolbar**

In `renderMobileChatSessionSheet`, keep only the Settings shortcut inside `<div className="mobile-chat-toolbar" aria-label="Chat tools">`.

Use this Settings button markup:

```tsx
                <button
                  type="button"
                  className="drawer-settings-icon-btn"
                  onClick={() => {
                    setProjectMenuOpen(false);
                    setSettingsDetailView(null);
                    setSidebarSettingsOpen(true);
                  }}
                  title="Open settings"
                  aria-label="Open settings"
                >
                  <span className="codicon codicon-settings-gear" />
                </button>
```

Delete the `Update` and `Port Relay` buttons from that toolbar.
Delete the refresh button from that toolbar as well so the Chat header left tool group contains only Settings.

- [ ] **Step 4: Add CSS for the mobile Settings bar and padding**

In `app/web/src/styles.css`, change `.mobile-settings-scroll` padding to:

```css
  padding: 14px 0 calc(env(safe-area-inset-bottom, 0px) + 82px);
```

Add:

```css
.mobile-settings-shortcut-bar {
  flex: 0 0 auto;
  display: grid;
  grid-template-columns: repeat(6, minmax(0, 1fr));
  align-items: center;
  border-top: 1px solid var(--border);
  background: color-mix(in srgb, var(--panel) 62%, var(--panel-3));
  padding: 0 0 env(safe-area-inset-bottom, 0px);
}

.mobile-settings-shortcut-button {
  position: relative;
  height: 48px;
  min-width: 0;
  border: 0;
  border-radius: 0;
  background: transparent;
  color: color-mix(in srgb, var(--muted) 86%, var(--text));
  display: inline-flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  transition: background-color 100ms ease, color 100ms ease;
  -webkit-tap-highlight-color: transparent;
}

.mobile-settings-shortcut-button .codicon {
  font-size: 22px;
}

.mobile-settings-shortcut-button:hover {
  color: var(--text);
  background: var(--hover);
}

.mobile-settings-shortcut-button.active {
  color: var(--text);
}

.mobile-settings-shortcut-button.active::before {
  content: '';
  position: absolute;
  left: 0;
  right: 0;
  bottom: 0;
  height: 2px;
  border-radius: 2px 2px 0 0;
  background: var(--accent);
}
```

Remove the dedicated `.mobile-chat-toolbar-icon` rules if they are no longer used. Keep `.mobile-chat-toolbar` as the lightweight layout container.

- [ ] **Step 5: Run targeted tests and verify GREEN**

Run:

```powershell
cd app
npm test -- --runTestsByPath __tests__/web-agent-package-update-settings.test.ts __tests__/web-skill-management-settings.test.ts __tests__/web-port-relay-settings.test.ts __tests__/web-registry-debug-settings.test.ts __tests__/web-chat-ui.test.ts --runInBand
```

Expected: PASS.

---

### Task 3: Final Verification

**Files:**
- Verify all modified files.

- [ ] **Step 1: Run focused mobile/settings tests**

Run:

```powershell
cd app
npm test -- --runTestsByPath __tests__/web-agent-package-update-settings.test.ts __tests__/web-skill-management-settings.test.ts __tests__/web-port-relay-settings.test.ts __tests__/web-registry-debug-settings.test.ts __tests__/web-chat-ui.test.ts __tests__/web-mobile-settings-system-back.test.ts __tests__/web-clear-local-cache-settings.test.ts --runInBand
```

Expected: PASS.

- [ ] **Step 2: Run TypeScript check**

Run:

```powershell
cd app
npm run tsc:web
```

Expected: exit code 0.

- [ ] **Step 3: Run web build**

Run:

```powershell
cd app
npm run build:web
```

Expected: exit code 0.

- [ ] **Step 4: Inspect diff**

Run:

```powershell
git diff -- app/web/src/main.tsx app/web/src/styles.css app/__tests__/web-agent-package-update-settings.test.ts app/__tests__/web-skill-management-settings.test.ts app/__tests__/web-port-relay-settings.test.ts app/__tests__/web-registry-debug-settings.test.ts app/__tests__/web-chat-ui.test.ts docs/superpowers/specs/2026-05-30-mobile-settings-shortcut-bar-design.md docs/superpowers/plans/2026-05-30-mobile-settings-shortcut-bar.md
```

Expected: diff only includes the approved mobile Settings shortcut bar, IA migration, Chat toolbar cleanup, tests, and plan/spec docs.

- [ ] **Step 5: Commit and push**

Run from repo root:

```powershell
git add -A
git commit -m "feat: add mobile settings shortcut bar"
git push origin main
```

Expected: commit and push succeed.
