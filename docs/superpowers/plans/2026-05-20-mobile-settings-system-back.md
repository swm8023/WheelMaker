# Mobile Settings System Back Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make mobile Settings participate in browser/WebView history so native back returns detail -> Settings list -> drawer, while mobile detail pages show a single title bar with existing actions.

**Architecture:** Add a small `mobileSettingsHistory` service with pure helpers for history state and pop decisions. Wire `app/web/src/main.tsx` to push mobile-only Settings history entries, handle `popstate`, and render mobile detail content without the duplicate inner detail header. Keep desktop detail rendering on the existing `.settings-detail-header` shell.

**Tech Stack:** React 19, TypeScript, Jest source-structure tests, CSS.

---

### Task 1: History Helper Tests

**Files:**
- Create: `app/web/src/services/mobileSettingsHistory.ts`
- Create: `app/__tests__/web-mobile-settings-system-back.test.ts`

- [ ] **Step 1: Write failing tests**

Add tests for app-owned history state recognition and pop actions:

```ts
import {
  createMobileSettingsHistoryState,
  isMobileSettingsHistoryState,
  mobileSettingsHistoryKey,
  resolveMobileSettingsPopAction,
} from '../web/src/services/mobileSettingsHistory';

describe('mobile settings system back', () => {
  test('marks and keys mobile settings history states', () => {
    const root = createMobileSettingsHistoryState(null);
    const update = createMobileSettingsHistoryState('update');

    expect(isMobileSettingsHistoryState(root)).toBe(true);
    expect(isMobileSettingsHistoryState(update)).toBe(true);
    expect(isMobileSettingsHistoryState({})).toBe(false);
    expect(mobileSettingsHistoryKey(null)).toBe('mobile-settings:root');
    expect(mobileSettingsHistoryKey('update')).toBe('mobile-settings:update');
  });

  test('resolves native back actions for settings layers', () => {
    expect(resolveMobileSettingsPopAction({
      nextState: createMobileSettingsHistoryState(null),
      settingsOpen: true,
      settingsDetailView: 'skills',
    })).toBe('back-to-list');

    expect(resolveMobileSettingsPopAction({
      nextState: null,
      settingsOpen: true,
      settingsDetailView: null,
    })).toBe('close-settings');

    expect(resolveMobileSettingsPopAction({
      nextState: createMobileSettingsHistoryState(null),
      settingsOpen: true,
      settingsDetailView: null,
    })).toBe('none');

    expect(resolveMobileSettingsPopAction({
      nextState: null,
      settingsOpen: false,
      settingsDetailView: null,
    })).toBe('none');
  });
});
```

- [ ] **Step 2: Run tests to verify RED**

Run: `cd app && npm test -- --runTestsByPath __tests__/web-mobile-settings-system-back.test.ts`

Expected: FAIL because `mobileSettingsHistory.ts` does not exist.

- [ ] **Step 3: Implement helper**

Create `app/web/src/services/mobileSettingsHistory.ts` with a marker, state factory, type guard, key helper, and pop action resolver.

- [ ] **Step 4: Run tests to verify GREEN**

Run: `cd app && npm test -- --runTestsByPath __tests__/web-mobile-settings-system-back.test.ts`

Expected: PASS.

### Task 2: Main UI Source Tests

**Files:**
- Modify: `app/__tests__/web-mobile-settings-system-back.test.ts`
- Modify: `app/__tests__/web-chat-ui.test.ts`

- [ ] **Step 1: Add source assertions**

Extend the new test file to assert:

```ts
import fs from 'fs';
import path from 'path';

function readMain(): string {
  return fs.readFileSync(path.join(__dirname, '..', 'web', 'src', 'main.tsx'), 'utf8');
}

test('wires mobile settings to history and mobile title actions', () => {
  const main = readMain();

  expect(main).toContain("} from './services/mobileSettingsHistory';");
  expect(main).toContain('window.history.pushState(createMobileSettingsHistoryState(settingsDetailView');
  expect(main).toContain("window.addEventListener('popstate', handleMobileSettingsPopState)");
  expect(main).toContain('resolveMobileSettingsPopAction({');
  expect(main).toContain('const mobileSettingsTitle = settingsDetailView');
  expect(main).toContain('const mobileSettingsActions = settingsDetailView');
  expect(main).toContain('renderSettingsContent(false, { hideDetailHeader: true })');
  expect(main).toContain('handleMobileSettingsBackButton');
  expect(main).toContain('renderSettingsDetailActions(settingsDetailView)');
  expect(main).not.toContain('mobileSettingsSwipe');
});
```

Update existing `web-chat-ui.test.ts` expectations from a literal `<div className="mobile-settings-title">Settings</div>` to the dynamic mobile title variable.

- [ ] **Step 2: Run tests to verify RED**

Run: `cd app && npm test -- --runTestsByPath __tests__/web-mobile-settings-system-back.test.ts __tests__/web-chat-ui.test.ts`

Expected: FAIL because main UI has not been wired.

### Task 3: Main UI Implementation

**Files:**
- Modify: `app/web/src/main.tsx`
- Modify: `app/web/src/styles.css`

- [ ] **Step 1: Wire history helpers**

Import the helper service in `main.tsx`. Add refs/effects to:

- push Settings root/detail history entries only when `!isWide && sidebarSettingsOpen`
- avoid duplicate pushes using `mobileSettingsHistoryKey`
- handle `popstate` using `resolveMobileSettingsPopAction`

- [ ] **Step 2: Add mobile-aware Settings back handlers**

Add callbacks:

- detail back on mobile calls `window.history.back()` when a mobile Settings history entry is active
- list back on mobile calls `window.history.back()` when a mobile Settings history entry is active
- fallback behavior keeps direct React state updates

- [ ] **Step 3: Split detail title/actions from detail body**

Add:

- `settingsDetailTitle(detail)`
- `renderSettingsDetailActions(detail)`
- `SettingsDetailShellOptions` with `hideDetailHeader`

Update detail renderers to accept options and keep desktop header unchanged. Mobile calls `renderSettingsContent(false, { hideDetailHeader: true })`.

- [ ] **Step 4: Update mobile title bar**

Compute:

- `mobileSettingsTitle`
- `mobileSettingsActions`

Render those in `.mobile-settings-nav`. The right title-bar area must hold refresh/export buttons for detail pages.

- [ ] **Step 5: Keep CSS scoped**

Add only small mobile title action styles if needed. Do not redesign desktop `.settings-detail-header`.

- [ ] **Step 6: Run targeted tests**

Run: `cd app && npm test -- --runTestsByPath __tests__/web-mobile-settings-system-back.test.ts __tests__/web-chat-ui.test.ts __tests__/web-agent-package-update-settings.test.ts __tests__/web-skill-management-settings.test.ts`

Expected: PASS.

### Task 4: Verification

**Files:**
- No new files unless tests expose a necessary fix.

- [ ] **Step 1: Type-check web code**

Run: `cd app && npm run tsc:web`

Expected: PASS.

- [ ] **Step 2: Run focused Settings tests**

Run: `cd app && npm test -- --runTestsByPath __tests__/web-mobile-settings-system-back.test.ts __tests__/web-chat-ui.test.ts __tests__/web-agent-package-update-settings.test.ts __tests__/web-skill-management-settings.test.ts __tests__/web-gesture-navigation.test.ts`

Expected: PASS.

- [ ] **Step 3: Inspect diff**

Run: `git diff -- app/web/src/main.tsx app/web/src/styles.css app/web/src/services/mobileSettingsHistory.ts app/__tests__/web-mobile-settings-system-back.test.ts app/__tests__/web-chat-ui.test.ts`

Expected: Diff only covers mobile Settings history/title behavior and targeted tests.
