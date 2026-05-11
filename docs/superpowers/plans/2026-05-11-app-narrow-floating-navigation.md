# App Narrow Floating Navigation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the web narrow-screen full-width header and horizontal tabs with a fixed Header Bubble plus a draggable, slot-snapped Floating Control Stack while preserving project switching, refresh, drawer access, and chat keyboard avoidance.

**Architecture:** Keep the wide shell unchanged and branch the shell structure only for `windowWidth < 900`. Persist the floating stack resting slot inside the existing workspace global state, compute the live y-position from safe regions at render time, and layer transient drag and keyboard offsets on top of that persisted slot. Reuse the current project menu, refresh flow, drawer flow, and chat keyboard inset instead of creating parallel behavior.

**Tech Stack:** React, TypeScript, existing workspace persistence layer, CSS, Jest source-inspection tests

---

## File map

- Modify: `D:\Code\WheelMaker\app\web\src\main.tsx` — add narrow-shell render split, floating stack state, long-press drag, slot snapping, keyboard avoidance, and content inset
- Modify: `D:\Code\WheelMaker\app\web\src\styles.css` — add Header Bubble and Floating Control Stack styles plus narrow-only shell layout overrides
- Modify: `D:\Code\WheelMaker\app\web\src\services\workspacePersistence.ts` — extend persisted global state with a floating slot identifier
- Modify: `D:\Code\WheelMaker\app\web\src\services\workspaceStore.ts` — preserve and patch the new persisted global slot
- Modify: `D:\Code\WheelMaker\app\__tests__\web-chat-ui.test.ts` — lock the narrow-shell contract with source assertions
- Update: `C:\Users\suweimin\.copilot\session-state\f6f64226-95bf-4d5d-97e9-52df274f641c\plan.md` — note active implementation progress and ship status

## Execution outline

### Task 1: Lock the narrow-shell contract in tests

**Files:**
- Modify: `D:\Code\WheelMaker\app\__tests__\web-chat-ui.test.ts`

- [ ] **Step 1: Write the failing test assertions**

Add assertions that describe the new narrow-shell contract:

```ts
expect(mainTsx).toContain("const FLOATING_CONTROL_SLOT_ORDER = ['upper', 'upper-middle', 'center', 'lower-middle'] as const;");
expect(mainTsx).toContain('className="header-bubble"');
expect(mainTsx).toContain('className="floating-control-stack"');
expect(mainTsx).toContain('className="floating-nav-group"');
expect(mainTsx).toContain('className="drawer-toggle-bubble"');
expect(mainTsx).toContain('const [floatingControlSlot, setFloatingControlSlot] = useState<PersistedFloatingControlSlot>(');
expect(mainTsx).toContain('const [floatingDragState, setFloatingDragState] = useState');
expect(mainTsx).toContain('const [floatingKeyboardOffset, setFloatingKeyboardOffset] = useState(0);');
expect(mainTsx).toContain('style={narrowContentInsetStyle}');
expect(stylesCss).toContain('.header-bubble {');
expect(stylesCss).toContain('.floating-control-stack {');
expect(stylesCss).toContain('.floating-nav-group {');
expect(stylesCss).toContain('.floating-nav-indicator {');
expect(stylesCss).toContain('.drawer-toggle-bubble {');
```

- [ ] **Step 2: Run the focused test to verify it fails**

Run:

```powershell
cd D:\Code\WheelMaker\app
npm test -- web-chat-ui.test.ts
```

Expected: FAIL because the new narrow-shell state, classes, and persistence hooks do not exist yet.

- [ ] **Step 3: Keep the failing assertions only**

Do not add production code in this task. The codebase should now have a red test that names the new shell contract.

- [ ] **Step 4: Re-run to confirm the failure is still the expected one**

Run:

```powershell
cd D:\Code\WheelMaker\app
npm test -- web-chat-ui.test.ts
```

Expected: FAIL on missing `header-bubble`, `floating-control-stack`, or `PersistedFloatingControlSlot`.

- [ ] **Step 5: Commit the red test**

```bash
git add app/__tests__/web-chat-ui.test.ts
git commit -m "test: lock narrow floating navigation contract"
```

### Task 2: Persist the floating stack slot

**Files:**
- Modify: `D:\Code\WheelMaker\app\web\src\services\workspacePersistence.ts`
- Modify: `D:\Code\WheelMaker\app\web\src\services\workspaceStore.ts`
- Modify: `D:\Code\WheelMaker\app\web\src\main.tsx`
- Test: `D:\Code\WheelMaker\app\__tests__\web-chat-ui.test.ts`

- [ ] **Step 1: Extend the persistence types with a slot identifier**

Add the new persisted type and field:

```ts
export type PersistedFloatingControlSlot =
  | 'upper'
  | 'upper-middle'
  | 'center'
  | 'lower-middle';

export type PersistedGlobalState = {
  address: string;
  token: string;
  deepseekApiKey: string;
  themeMode: PersistedThemeMode;
  codeTheme: CodeThemeId;
  codeFont: CodeFontId;
  codeFontSize: number;
  codeLineHeight: number;
  codeTabSize: number;
  wrapLines: boolean;
  showLineNumbers: boolean;
  tab: PersistedTab;
  selectedProjectId: string;
  floatingControlSlot: PersistedFloatingControlSlot;
};
```

- [ ] **Step 2: Add the default and sanitizer support**

Update the default and sanitizer logic:

```ts
function defaultGlobalState(): PersistedGlobalState {
  return {
    address: '',
    token: '',
    deepseekApiKey: '',
    themeMode: 'dark',
    codeTheme: DEFAULT_CODE_THEME,
    codeFont: DEFAULT_CODE_FONT,
    codeFontSize: DEFAULT_CODE_FONT_SIZE,
    codeLineHeight: DEFAULT_CODE_LINE_HEIGHT,
    codeTabSize: DEFAULT_CODE_TAB_SIZE,
    wrapLines: false,
    showLineNumbers: true,
    tab: 'file',
    selectedProjectId: '',
    floatingControlSlot: 'upper-middle',
  };
}
```

```ts
const floatingControlSlot =
  input.floatingControlSlot === 'upper' ||
  input.floatingControlSlot === 'center' ||
  input.floatingControlSlot === 'lower-middle'
    ? input.floatingControlSlot
    : 'upper-middle';
```

- [ ] **Step 3: Persist the new global key**

Add the key and patch behavior:

```ts
const GLOBAL_KEYS = {
  deepseekApiKey: 'deepseekApiKey',
  themeMode: 'themeMode',
  codeTheme: 'codeTheme',
  codeFont: 'codeFont',
  codeFontSize: 'codeFontSize',
  codeLineHeight: 'codeLineHeight',
  codeTabSize: 'codeTabSize',
  wrapLines: 'wrapLines',
  showLineNumbers: 'showLineNumbers',
  tab: 'tab',
  selectedProjectId: 'selectedProjectId',
  floatingControlSlot: 'floatingControlSlot',
} as const;
```

Also include it in every global read/write path that already handles `tab` and `selectedProjectId`.

- [ ] **Step 4: Thread the slot through `WorkspaceStore`**

No new store abstraction is needed; the existing global-state API already works. Keep the patch path simple:

```ts
workspaceStore.rememberGlobalState({
  address,
  token,
  themeMode,
  codeTheme,
  codeFont,
  codeFontSize,
  codeLineHeight,
  codeTabSize,
  wrapLines,
  showLineNumbers,
  tab,
  selectedProjectId: projectId,
  floatingControlSlot,
});
```

- [ ] **Step 5: Run the focused test and typecheck**

Run:

```powershell
cd D:\Code\WheelMaker\app
npm test -- web-chat-ui.test.ts
npm run tsc:web
```

Expected: the test still fails on missing UI structure, but typecheck stays green and the persistence assertions now pass.

- [ ] **Step 6: Commit the persistence layer**

```bash
git add app/web/src/services/workspacePersistence.ts app/web/src/services/workspaceStore.ts app/web/src/main.tsx app/__tests__/web-chat-ui.test.ts
git commit -m "feat: persist floating navigation slot"
```

### Task 3: Replace the narrow header with the new narrow shell

**Files:**
- Modify: `D:\Code\WheelMaker\app\web\src\main.tsx`
- Modify: `D:\Code\WheelMaker\app\web\src\styles.css`
- Test: `D:\Code\WheelMaker\app\__tests__\web-chat-ui.test.ts`

- [ ] **Step 1: Add the narrow-shell constants and slot helpers**

Add the slot order and helper functions near the other top-level constants:

```ts
const FLOATING_CONTROL_SLOT_ORDER = ['upper', 'upper-middle', 'center', 'lower-middle'] as const;

function floatingControlSlotRatio(slot: PersistedFloatingControlSlot): number {
  switch (slot) {
    case 'upper':
      return 0.26;
    case 'center':
      return 0.5;
    case 'lower-middle':
      return 0.68;
    default:
      return 0.4;
  }
}
```

- [ ] **Step 2: Add the narrow-shell UI state**

Introduce the state that the test expects:

```ts
const [floatingControlSlot, setFloatingControlSlot] =
  useState<PersistedFloatingControlSlot>(initialGlobalState.floatingControlSlot ?? 'upper-middle');
const [floatingDragState, setFloatingDragState] = useState<{
  active: boolean;
  pressing: boolean;
  pointerId: number;
  originY: number;
  startTop: number;
  currentTop: number;
  cooldownUntil: number;
} | null>(null);
const [floatingKeyboardOffset, setFloatingKeyboardOffset] = useState(0);
```

- [ ] **Step 3: Branch the shell render for narrow screens**

Replace the narrow-screen dependency on the old full-width header with a render split. Keep the wide header untouched.

Use a structure like:

```tsx
const narrowHeaderBubble = !isWide ? (
  <div className="header-bubble-layer">
    <div className="header-bubble">
      <div className="project-wrap" onPointerDown={event => event.stopPropagation()}>
        <button className="project-btn header-bubble-project" onClick={() => setProjectMenuOpen(value => !value)}>
          <span className="project-arrow codicon codicon-chevron-down" />
          <span className="project-name" title={currentProjectName}>{currentProjectName}</span>
          {loadingProject || refreshingProject || reconnecting ? <span className="muted">...</span> : null}
        </button>
        {projectMenuOpen ? <div className="project-menu">...</div> : null}
      </div>
      <button
        className={`header-btn refresh-btn header-bubble-refresh${hasPendingProjectUpdates && !refreshingProject && !reconnecting ? ' has-update-badge' : ''}`}
        onClick={() => refreshProject().catch(() => undefined)}
        title={reconnecting ? 'Reconnecting...' : 'Refresh project'}
        disabled={refreshingProject || reconnecting}
      >
        {refreshingProject ? '...' : reconnecting ? <span className="codicon codicon-loading codicon-modifier-spin" /> : <span className="codicon codicon-refresh" />}
      </button>
    </div>
  </div>
) : null;
```

- [ ] **Step 4: Add the Floating Control Stack JSX**

Add a narrow-only floating stack alongside the body:

```tsx
const floatingControlStack = !isWide ? (
  <div className="floating-control-stack-layer">
    <div className="floating-control-stack">
      <div className="floating-nav-group" aria-label="Primary navigation">
        <div className="floating-nav-indicator" />
        <button type="button" className={`floating-nav-button ${tab === 'chat' ? 'active' : ''}`} onClick={() => setTab('chat')} title="Chat" aria-label="Chat">
          <span className="codicon codicon-comment-discussion" />
        </button>
        <button type="button" className={`floating-nav-button ${tab === 'file' ? 'active' : ''}`} onClick={() => setTab('file')} title="File" aria-label="File">
          <span className="codicon codicon-files" />
        </button>
        <button type="button" className={`floating-nav-button ${tab === 'git' ? 'active' : ''}`} onClick={() => setTab('git')} title="Git" aria-label="Git">
          <span className="codicon codicon-source-control" />
        </button>
      </div>
      <button type="button" className="drawer-toggle-bubble" onClick={() => setDrawerOpen(value => !value)} title="Toggle drawer" aria-label="Toggle drawer">
        <span className="codicon codicon-menu" />
      </button>
    </div>
  </div>
) : null;
```

- [ ] **Step 5: Remove the old narrow-header dependency from layout**

Do not keep the old narrow `.header` in layout. The render should look like:

```tsx
return (
  <div className={`workspace theme-${themeMode}${!isWide ? ' narrow-shell' : ''}`}>
    <style>{setiFontCss}</style>
    {isWide ? <header className="header">...</header> : null}
    {narrowHeaderBubble}
    {floatingControlStack}
    <div className="body">
      {isWide && !sidebarCollapsed ? <aside className="workspace-left">{renderSidebar()}</aside> : null}
      <main className="workspace-right" style={narrowContentInsetStyle}>{renderMain()}</main>
    </div>
    {!isWide ? <div className={`drawer-overlay ${drawerOpen ? 'show' : ''}`}>...</div> : null}
  </div>
);
```

- [ ] **Step 6: Add the core CSS for the new shell**

Add the new classes:

```css
.header-bubble-layer {
  position: fixed;
  top: calc(env(safe-area-inset-top, 0px) + 10px);
  left: 10px;
  z-index: 48;
  pointer-events: none;
}

.header-bubble {
  pointer-events: auto;
  display: inline-flex;
  align-items: center;
  gap: 8px;
  min-height: 40px;
  max-width: min(72vw, 360px);
  padding: 4px 6px 4px 10px;
  border: 1px solid color-mix(in srgb, var(--border) 88%, transparent);
  border-radius: 999px;
  background: color-mix(in srgb, var(--panel-2) 94%, transparent);
  box-shadow: 0 12px 28px rgba(0, 0, 0, 0.24);
  backdrop-filter: blur(12px);
}

.floating-control-stack-layer {
  position: fixed;
  top: 0;
  right: 0;
  bottom: 0;
  width: 88px;
  pointer-events: none;
  z-index: 44;
}
```

- [ ] **Step 7: Run the focused test to turn the shell contract green**

Run:

```powershell
cd D:\Code\WheelMaker\app
npm test -- web-chat-ui.test.ts
```

Expected: PASS once the test-visible structure and state are present.

- [ ] **Step 8: Commit the narrow-shell structure**

```bash
git add app/web/src/main.tsx app/web/src/styles.css app/__tests__/web-chat-ui.test.ts
git commit -m "feat: add narrow floating shell"
```

### Task 4: Add drag, snapping, safe bounds, and keyboard avoidance

**Files:**
- Modify: `D:\Code\WheelMaker\app\web\src\main.tsx`
- Modify: `D:\Code\WheelMaker\app\web\src\styles.css`
- Test: `D:\Code\WheelMaker\app\__tests__\web-chat-ui.test.ts`

- [ ] **Step 1: Compute safe slot positions**

Add helpers that derive the live resting position from safe bounds:

```ts
function clampFloatingTop(top: number, minTop: number, maxTop: number): number {
  return Math.min(maxTop, Math.max(minTop, top));
}

function nearestFloatingSlot(
  top: number,
  slotTops: Array<{slot: PersistedFloatingControlSlot; top: number}>,
): PersistedFloatingControlSlot {
  return slotTops.reduce((best, entry) =>
    Math.abs(entry.top - top) < Math.abs(best.top - top) ? entry : best,
  ).slot;
}
```

- [ ] **Step 2: Compute header inset and keyboard offset**

Use memoized values for the narrow shell:

```ts
const narrowHeaderInset = useMemo(() => {
  if (isWide) return 0;
  return 64;
}, [isWide]);

const narrowContentInsetStyle = !isWide
  ? ({paddingTop: `${narrowHeaderInset}px`} as const)
  : undefined;
```

And for the stack:

```ts
useEffect(() => {
  if (isWide || tab !== 'chat') {
    setFloatingKeyboardOffset(0);
    return;
  }
  const viewport = window.visualViewport;
  if (!viewport) return;
  const updateOffset = () => {
    const bottomGap = Math.max(0, Math.round(window.innerHeight - (viewport.height + viewport.offsetTop)));
    setFloatingKeyboardOffset(bottomGap >= 72 ? bottomGap : 0);
  };
  updateOffset();
  viewport.addEventListener('resize', updateOffset);
  viewport.addEventListener('scroll', updateOffset);
  return () => {
    viewport.removeEventListener('resize', updateOffset);
    viewport.removeEventListener('scroll', updateOffset);
  };
}, [isWide, tab]);
```

- [ ] **Step 3: Implement long-press drag and cooldown**

Add a narrow-only pointer handler that owns the whole stack:

```ts
const floatingLongPressTimerRef = useRef<number | null>(null);

const beginFloatingPress = (event: React.PointerEvent<HTMLDivElement>) => {
  if (isWide) return;
  const pointerId = event.pointerId;
  const originY = event.clientY;
  setFloatingDragState({
    active: false,
    pressing: true,
    pointerId,
    originY,
    startTop: floatingControlTop,
    currentTop: floatingControlTop,
    cooldownUntil: 0,
  });
  floatingLongPressTimerRef.current = window.setTimeout(() => {
    setFloatingDragState(prev => (prev ? {...prev, active: true, pressing: false} : prev));
  }, 350);
};
```

Cancel on early move threshold and suppress click after drag release by writing `cooldownUntil = Date.now() + 120`.

- [ ] **Step 4: Snap to the nearest slot on release and persist it**

On pointer release:

```ts
const nextSlot = nearestFloatingSlot(currentTop, slotTops);
setFloatingControlSlot(nextSlot);
setFloatingDragState(prev => prev ? {...prev, active: false, pressing: false, cooldownUntil: Date.now() + 120} : prev);
workspaceStore.rememberGlobalState({floatingControlSlot: nextSlot});
```

Remember: persist the slot id only. The keyboard offset must never be written.

- [ ] **Step 5: Finish the interaction styles**

Add styles for idle, pressed, dragging, and active states:

```css
.floating-control-stack {
  position: absolute;
  right: 10px;
  display: flex;
  flex-direction: column;
  gap: 10px;
  pointer-events: auto;
  transition: transform 180ms ease, opacity 180ms ease, box-shadow 180ms ease;
}

.floating-control-stack.drag-ready {
  transform: scale(0.98);
}

.floating-control-stack.dragging {
  transform: scale(1.04);
}

.floating-nav-group,
.drawer-toggle-bubble {
  border: 1px solid color-mix(in srgb, var(--border) 88%, transparent);
  background: color-mix(in srgb, var(--panel-2) 94%, transparent);
  box-shadow: 0 12px 28px rgba(0, 0, 0, 0.22);
}
```

- [ ] **Step 6: Run focused and full validation**

Run:

```powershell
cd D:\Code\WheelMaker\app
npm test -- web-chat-ui.test.ts
npm test
npm run tsc:web
```

Expected: PASS, PASS, PASS.

- [ ] **Step 7: Commit the interaction behavior**

```bash
git add app/web/src/main.tsx app/web/src/styles.css app/__tests__/web-chat-ui.test.ts app/web/src/services/workspacePersistence.ts app/web/src/services/workspaceStore.ts
git commit -m "feat: add floating navigation drag behavior"
```

### Task 5: Final verification and ship

**Files:**
- Modify: `C:\Users\suweimin\.copilot\session-state\f6f64226-95bf-4d5d-97e9-52df274f641c\plan.md`
- Ship: Git + web release

- [ ] **Step 1: Update session plan progress**

Record the implementation milestone in the session plan file so the next worker sees the active narrow-shell work and current ship state.

- [ ] **Step 2: Run final validation**

Run:

```powershell
cd D:\Code\WheelMaker\app
npm test
npm run tsc:web
```

Expected: all 25 app suites pass and TypeScript remains clean.

- [ ] **Step 3: Stage all changes**

```bash
git add -A
git status --short
```

Expected: only the intended plan, test, app, and persistence files are staged.

- [ ] **Step 4: Commit**

```bash
git commit -m "feat: redesign narrow web navigation"
```

- [ ] **Step 5: Push**

```bash
git push origin main
```

- [ ] **Step 6: Publish the web release**

```powershell
cd D:\Code\WheelMaker\app
npm run build:web:release
```

Expected: web assets exported to `C:\Users\suweimin\.wheelmaker\web`.

## Self-review

### Spec coverage

- Header Bubble: covered by Tasks 1, 3, and 4
- Floating Control Stack structure: covered by Tasks 1 and 3
- Slot persistence: covered by Task 2 and Task 4
- Long-press drag + cooldown: covered by Task 4
- Keyboard avoidance: covered by Task 4
- Initial top inset + overlap model: covered by Task 4
- Web-only scope: respected throughout; no Flutter task exists
- No bottom status bar work: no task includes it

### Placeholder scan

- No `TBD` / `TODO`
- All tasks use exact file paths
- Every code-changing step includes concrete code snippets
- Every verification step includes exact commands

### Type consistency

- Persisted slot type is consistently named `PersistedFloatingControlSlot`
- State variable is consistently named `floatingControlSlot`
- Drag unit is consistently named `Floating Control Stack`
- Snap behavior consistently uses slot ids instead of raw persisted pixels
