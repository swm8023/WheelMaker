# Gesture Navigation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add the optional narrow/mobile Gesture Navigation mode described in `docs/superpowers/specs/2026-05-19-gesture-navigation-design.md`.

**Architecture:** Persist a new global boolean preference, add a small pure gesture helper for thresholds and candidate selection, then branch the existing mobile `floatingControlStack` rendering between old controls and the new drawer-centered gesture control. Keep desktop activity bar and drawer semantics unchanged.

**Tech Stack:** React 19, TypeScript, CSS, Jest source-level tests, existing IndexedDB workspace persistence.

---

### Task 1: Persist The Appearance Setting

**Files:**
- Modify: `app/web/src/services/workspacePersistence.ts`
- Test: `app/__tests__/web-gesture-navigation.test.ts`

- [x] **Step 1: Write the failing persistence test**

Create `app/__tests__/web-gesture-navigation.test.ts` with expectations that `workspacePersistence.ts` contains `gestureNavigation: boolean`, `gestureNavigation: 'gestureNavigation'`, default `false`, normalization from boolean input, and global save rows for both `this.state.global` and `next`.

- [x] **Step 2: Run test to verify it fails**

Run: `npm test -- --runTestsByPath __tests__/web-gesture-navigation.test.ts`
Expected: FAIL because `gestureNavigation` does not exist.

- [x] **Step 3: Implement persistence**

Add `gestureNavigation` to `PersistedGlobalState`, `GLOBAL_KEYS`, `defaultGlobalState`, `normalizeGlobalState`, `persistGlobalState`, and `saveGlobalState`.

- [x] **Step 4: Run test to verify it passes**

Run: `npm test -- --runTestsByPath __tests__/web-gesture-navigation.test.ts`
Expected: PASS.

### Task 2: Add Gesture Decision Helpers

**Files:**
- Create: `app/web/src/services/gestureNavigation.ts`
- Test: `app/__tests__/web-gesture-navigation.test.ts`

- [x] **Step 1: Extend the failing test**

Add tests for:
- distance under `12px` remains long-press eligible.
- distance over `28px` before activation enters drag mode.
- elapsed `350ms` with small movement enters expanded mode.
- up resolves to `chat`, left resolves to `file`, down resolves to `git`.
- rightward movement resolves to `null`.

- [x] **Step 2: Run test to verify it fails**

Run: `npm test -- --runTestsByPath __tests__/web-gesture-navigation.test.ts`
Expected: FAIL because helper exports do not exist.

- [x] **Step 3: Implement helper**

Create `gestureNavigation.ts` with constants `GESTURE_LONG_PRESS_MS = 350`, `GESTURE_LONG_PRESS_CANCEL_PX = 12`, `GESTURE_DRAG_START_PX = 28`, `GESTURE_SELECTION_PX = 42`; export `resolveGesturePressIntent` and `resolveGestureDirectionCandidate`.

- [x] **Step 4: Run test to verify it passes**

Run: `npm test -- --runTestsByPath __tests__/web-gesture-navigation.test.ts`
Expected: PASS.

### Task 3: Wire Setting And Mobile Rendering

**Files:**
- Modify: `app/web/src/main.tsx`
- Test: `app/__tests__/web-gesture-navigation.test.ts`

- [x] **Step 1: Extend the failing source test**

Assert `main.tsx`:
- initializes `gestureNavigation` from `persistedGlobal`.
- renders `<span>Gesture Navigation</span>` in Appearance.
- persists with `workspaceStore.rememberGlobalState({ gestureNavigation })`.
- branches `floatingControlStack` by `gestureNavigation`.
- keeps the old `floating-nav-group`.
- includes `gesture-nav-control`, `gesture-nav-badge`, `gesture-nav-option`, and Chat/File/Git directional labels/icons.

- [x] **Step 2: Run test to verify it fails**

Run: `npm test -- --runTestsByPath __tests__/web-gesture-navigation.test.ts`
Expected: FAIL because main rendering is not wired.

- [x] **Step 3: Implement state and render branch**

In `main.tsx`, add `gestureNavigation` state, persist it with other global settings, add the Appearance row, add transient gesture state, import helper functions, and render the new mobile control branch when `gestureNavigation` is true.

- [x] **Step 4: Run test to verify it passes**

Run: `npm test -- --runTestsByPath __tests__/web-gesture-navigation.test.ts`
Expected: PASS.

### Task 4: Add Styles

**Files:**
- Modify: `app/web/src/styles.css`
- Test: `app/__tests__/web-gesture-navigation.test.ts`

- [x] **Step 1: Extend the failing style test**

Assert `styles.css` defines `.gesture-nav-control`, `.gesture-nav-button`, `.gesture-nav-badge`, `.gesture-nav-option`, and active/candidate states.

- [x] **Step 2: Run test to verify it fails**

Run: `npm test -- --runTestsByPath __tests__/web-gesture-navigation.test.ts`
Expected: FAIL because styles do not exist.

- [x] **Step 3: Implement styles**

Add circular button styles matching the existing floating control visual language, badge positioning, expanded option positions, and highlighted background states.

- [x] **Step 4: Run test to verify it passes**

Run: `npm test -- --runTestsByPath __tests__/web-gesture-navigation.test.ts`
Expected: PASS.

### Task 5: Verify And Ship

**Files:**
- Verify all changed files.

- [x] **Step 1: Run focused test**

Run: `npm test -- --runTestsByPath __tests__/web-gesture-navigation.test.ts`
Expected: PASS.

- [x] **Step 2: Run TypeScript**

Run: `npm run tsc:web`
Expected: PASS.

- [x] **Step 3: Run full Jest suite**

Run: `npm test -- --no-cache`
Expected: PASS.

- [x] **Step 4: Check diff**

Run: `git diff --check`
Expected: no whitespace errors.

- [x] **Step 5: Commit, push, and build release**

Follow repository gate:

```powershell
git add -A
git commit -m "feat: add mobile gesture navigation"
git push origin main
cd app
npm run build:web:release
```
