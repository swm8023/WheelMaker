# Chat Hub Summary Dropdown Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a compact Hub count and dropdown beside the Chat page title on mobile and wide layouts.

**Architecture:** Keep the Hub summary local to the existing `App` shell because the relevant state (`registryHubs`) and Chat title rendering already live in `app/web/src/main.tsx`. Add a small render helper and close behavior alongside existing chat menu state, then style it in `app/web/src/styles.css` so title text truncates before the Hub control.

**Tech Stack:** React 19, TypeScript, Jest source-structure tests, CSS.

---

## File Structure

- Modify `app/__tests__/web-chat-ui.test.ts`: add source-structure regression tests for the Hub title summary and dropdown behavior.
- Modify `app/web/src/main.tsx`: add `chatHubMenuOpen` state, `chatHubMenuRef`, outside/Escape close effects, `renderChatHubSummary`, and use it in the Chat title branch for both mobile and wide layouts.
- Modify `app/web/src/styles.css`: add compact title row, Hub summary button, count badge, popover, row, and empty-state styles.

## Task 1: Lock Chat Title Hub Summary Behavior

**Files:**
- Test: `app/__tests__/web-chat-ui.test.ts`
- Modify later: `app/web/src/main.tsx`
- Modify later: `app/web/src/styles.css`

- [x] **Step 1: Write the failing source-structure test**

Add this test inside `describe('web chat integration', () => { ... })` in `app/__tests__/web-chat-ui.test.ts`:

```typescript
  test('chat title shows hub count summary with dropdown details', () => {
    const projectRoot = path.join(__dirname, '..');
    const mainTsx = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'main.tsx'), 'utf8');
    const stylesCss = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'styles.css'), 'utf8');

    expect(mainTsx).toContain('const [chatHubMenuOpen, setChatHubMenuOpen] = useState(false);');
    expect(mainTsx).toContain('const chatHubMenuRef = useRef<HTMLDivElement | null>(null);');
    expect(mainTsx).toContain('const renderChatHubSummary = useCallback(() => {');
    expect(mainTsx).toContain('const hubCount = registryHubs.length;');
    expect(mainTsx).toContain('aria-label="Show connected hubs"');
    expect(mainTsx).toContain('aria-expanded={chatHubMenuOpen}');
    expect(mainTsx).toContain('<span className="chat-hub-summary-label">Hubs</span>');
    expect(mainTsx).toContain('<span className="chat-hub-summary-count">{hubCount}</span>');
    expect(mainTsx).toContain('{registryHubs.length > 0 ? (');
    expect(mainTsx).toContain('registryHubs.map(hub => (');
    expect(mainTsx).toContain('<span className="chat-hub-row-name">{hub.hubId}</span>');
    expect(mainTsx).toContain('<div className="chat-hub-empty">No hubs</div>');
    expect(mainTsx).toContain('<div className="chat-title-row">');
    expect(mainTsx).toContain('renderChatHubSummary()');
    expect(mainTsx).toMatch(
      /<div className="chat-title-row">[\s\S]*?CHAT - \{selectedChatDisplayTitle \|\| 'New Session'\}[\s\S]*?renderChatHubSummary\(\)/,
    );
    expect(mainTsx).toMatch(
      /<div className="chat-title-row">[\s\S]*?renderBreadcrumbTitle\(chatBreadcrumbProjectName, chatBreadcrumbLabel\)[\s\S]*?renderChatHubSummary\(\)/,
    );
    expect(stylesCss).toContain('.chat-title-row {');
    expect(stylesCss).toContain('.chat-title-row > .title-text {');
    expect(stylesCss).toContain('.chat-hub-summary {');
    expect(stylesCss).toContain('.chat-hub-summary-button {');
    expect(stylesCss).toContain('.chat-hub-summary-count {');
    expect(stylesCss).toContain('.chat-hub-popover {');
    expect(stylesCss).toContain('.chat-hub-row-name {');
    expect(stylesCss).toContain('.chat-hub-empty {');
  });
```

- [x] **Step 2: Run the test to verify RED**

Run:

```powershell
cd app
npm test -- --runInBand web-chat-ui.test.ts
```

Expected: FAIL because `chatHubMenuOpen`, `renderChatHubSummary`, and the new CSS classes do not exist yet.

## Task 2: Implement Chat Title Hub Summary

**Files:**
- Modify: `app/web/src/main.tsx`
- Modify: `app/web/src/styles.css`

- [x] **Step 1: Add state and close behavior in `main.tsx`**

Add near the existing chat UI menu state:

```typescript
  const [chatHubMenuOpen, setChatHubMenuOpen] = useState(false);
  const chatHubMenuRef = useRef<HTMLDivElement | null>(null);
```

Add effects near other menu close effects:

```typescript
  useEffect(() => {
    if (!chatHubMenuOpen) {
      return;
    }
    const handlePointerDown = (event: PointerEvent) => {
      const target = event.target;
      if (target instanceof Node && chatHubMenuRef.current?.contains(target)) {
        return;
      }
      setChatHubMenuOpen(false);
    };
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        setChatHubMenuOpen(false);
      }
    };
    document.addEventListener('pointerdown', handlePointerDown);
    document.addEventListener('keydown', handleKeyDown);
    return () => {
      document.removeEventListener('pointerdown', handlePointerDown);
      document.removeEventListener('keydown', handleKeyDown);
    };
  }, [chatHubMenuOpen]);

  useEffect(() => {
    if (tab !== 'chat' || sidebarSettingsOpen) {
      setChatHubMenuOpen(false);
    }
  }, [sidebarSettingsOpen, tab]);
```

- [x] **Step 2: Add `renderChatHubSummary` in `main.tsx`**

Add near `renderBreadcrumbTitle`:

```typescript
  const renderChatHubSummary = useCallback(() => {
    const hubCount = registryHubs.length;
    return (
      <div ref={chatHubMenuRef} className="chat-hub-summary">
        <button
          type="button"
          className="chat-hub-summary-button"
          aria-label="Show connected hubs"
          aria-haspopup="menu"
          aria-expanded={chatHubMenuOpen}
          onClick={() => setChatHubMenuOpen(open => !open)}
        >
          <span className="chat-hub-summary-label">Hubs</span>
          <span className="chat-hub-summary-count">{hubCount}</span>
          <span className="codicon codicon-chevron-down" aria-hidden="true" />
        </button>
        {chatHubMenuOpen ? (
          <div className="chat-hub-popover" role="menu">
            {registryHubs.length > 0 ? (
              registryHubs.map(hub => (
                <div key={hub.hubId} className="chat-hub-row" role="menuitem">
                  <span className="chat-hub-row-name">{hub.hubId}</span>
                </div>
              ))
            ) : (
              <div className="chat-hub-empty">No hubs</div>
            )}
          </div>
        ) : null}
      </div>
    );
  }, [chatHubMenuOpen, registryHubs]);
```

- [x] **Step 3: Use the helper in both Chat title paths**

Replace the Chat `block-title` content with:

```tsx
          <div className="block-title">
            {isWide ? (
              <div className="chat-title-row">
                <span className="title-text">
                  CHAT - {selectedChatDisplayTitle || 'New Session'}
                </span>
                {renderChatHubSummary()}
              </div>
            ) : (
              <div className="chat-title-row">
                {renderBreadcrumbTitle(chatBreadcrumbProjectName, chatBreadcrumbLabel)}
                {renderChatHubSummary()}
              </div>
            )}
          </div>
```

- [x] **Step 4: Add compact CSS**

Add after `.block-title` / `.title-text` styles in `app/web/src/styles.css`:

```css
.chat-title-row {
  position: relative;
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
  min-width: 0;
}

.chat-title-row > .title-text {
  flex: 1 1 auto;
}

.chat-title-row .breadcrumb-title {
  flex: 1 1 auto;
}

.chat-hub-summary {
  position: relative;
  flex: 0 0 auto;
  display: inline-flex;
  align-items: center;
}

.chat-hub-summary-button {
  height: 22px;
  display: inline-flex;
  align-items: center;
  gap: 5px;
  border: 1px solid color-mix(in srgb, var(--accent) 34%, transparent);
  border-radius: 7px;
  background: color-mix(in srgb, var(--accent) 12%, var(--panel));
  color: var(--text);
  padding: 0 7px;
  font-size: 11px;
  font-weight: 700;
  cursor: pointer;
}

.chat-hub-summary-count {
  min-width: 16px;
  height: 16px;
  border-radius: 999px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  background: color-mix(in srgb, var(--accent) 24%, transparent);
  color: color-mix(in srgb, var(--accent) 82%, var(--text));
  padding: 0 4px;
  font-size: 10px;
  line-height: 1;
}

.chat-hub-popover {
  position: absolute;
  top: calc(100% + 6px);
  right: 0;
  z-index: 50;
  min-width: 180px;
  max-width: min(280px, calc(100vw - 24px));
  max-height: 240px;
  overflow: auto;
  border: 1px solid var(--border);
  border-radius: 8px;
  background: var(--panel);
  box-shadow: 0 16px 34px rgba(0, 0, 0, 0.28);
  padding: 6px;
}

.chat-hub-row,
.chat-hub-empty {
  min-height: 28px;
  display: flex;
  align-items: center;
  padding: 0 8px;
  border-radius: 6px;
  font-size: 12px;
}

.chat-hub-row-name {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.chat-hub-empty {
  color: var(--muted);
}
```

- [x] **Step 5: Run the focused test to verify GREEN**

Run:

```powershell
cd app
npm test -- --runInBand web-chat-ui.test.ts
```

Expected: PASS.

## Task 3: Verify, Commit, Push, and Publish

**Files:**
- Verify all touched files and the app build.

- [x] **Step 1: Run full app tests**

Run:

```powershell
cd app
npm test -- --runInBand
```

Expected: PASS.

- [x] **Step 2: Run TypeScript check**

Run:

```powershell
cd app
npm run tsc:web
```

Expected: PASS.

- [ ] **Step 3: Run release build**

Run:

```powershell
cd app
npm run build:web:release
```

Expected: PASS and export web assets to the configured release directory.

- [ ] **Step 4: Commit only intended changes**

Because `CONTEXT.md` has unrelated pre-existing edits, inspect status and keep it out of the commit:

```powershell
git status --short
git add app/__tests__/web-chat-ui.test.ts app/web/src/main.tsx app/web/src/styles.css docs/superpowers/plans/2026-05-20-chat-hub-summary-dropdown.md
git commit -m "feat: add chat hub summary dropdown"
```

Expected: commit succeeds and does not include `CONTEXT.md`.

- [ ] **Step 5: Push current branch**

Run:

```powershell
git branch --show-current
git push origin main
```

Expected: push succeeds.

## Self-Review

- Spec coverage: the plan covers mobile and wide title placement, count display from `registryHubs.length`, dropdown rows from `hub.hubId`, empty state, outside/Escape/settings/tab close, CSS truncation, tests, TypeScript, release build, commit, and push.
- Placeholder scan: no `TBD`, `TODO`, `placeholder`, or vague implementation steps remain.
- Type consistency: `RegistryHub` exposes `hubId`, `registryHubs` is the existing state array, `chatHubMenuOpen` is a boolean state value, and `chatHubMenuRef` points at the popover anchor wrapper.
