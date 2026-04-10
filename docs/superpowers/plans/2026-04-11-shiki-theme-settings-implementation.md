# Shiki Theme Settings Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Expose the full bundled Shiki theme inventory in the web settings panel while keeping existing global code font and code layout settings intact.

**Architecture:** Replace the hand-maintained theme shortlist in `shikiRenderer.ts` with Shiki bundled theme metadata, derive grouped dark/light options from that metadata, and keep persistence validation centralized in the existing `isCodeThemeId` flow. Update the settings UI in `main.tsx` to render grouped theme options and leave the global persistence wiring unchanged.

**Tech Stack:** React, TypeScript, Shiki 4, Jest, existing workspace persistence services

---

## File Map

| File | Action | Responsibility after change |
|------|--------|-----------------------------|
| `app/__tests__/web-shiki-theme-settings.test.ts` | Create | TDD coverage for bundled theme inventory wiring and settings UI exposure |
| `app/web/src/services/shikiRenderer.ts` | Modify | Bundled theme ids, generated theme options, grouped dark/light theme metadata, theme validation |
| `app/web/src/main.tsx` | Modify | Grouped `Code Theme` select rendering in sidebar settings |
| `app/web/src/services/workspacePersistence.ts` | Verify only | Continue sanitizing `codeTheme` through centralized validation |

---

### Task 1: Lock Behavior With Failing Tests

**Files:**
- Create: `app/__tests__/web-shiki-theme-settings.test.ts`

- [ ] **Step 1: Write the failing source-level tests**

Add tests that assert:
- `shikiRenderer.ts` imports `bundledThemesInfo` and `BundledTheme` from `shiki`
- `CodeThemeId` is based on `BundledTheme` plus `auto-plus`
- grouped theme option structures for dark/light themes exist
- `main.tsx` renders grouped theme `<optgroup>` sections
- `workspacePersistence.ts` still sanitizes `codeTheme` through `isCodeThemeId`

- [ ] **Step 2: Run the targeted Jest test to verify it fails**

Run: `npm test -- --runInBand web-shiki-theme-settings.test.ts`
Expected: FAIL because the current implementation still uses a narrow hard-coded theme list and a flat select.

### Task 2: Implement Bundled Theme Inventory And UI Wiring

**Files:**
- Modify: `app/web/src/services/shikiRenderer.ts`
- Modify: `app/web/src/main.tsx`

- [ ] **Step 1: Replace the hard-coded theme shortlist with bundled theme metadata**

Use `bundledThemesInfo` to derive:
- `CodeThemeId = 'auto-plus' | BundledTheme`
- `CODE_THEME_OPTIONS`
- grouped dark/light option collections for UI rendering
- `isCodeThemeId()` validation based on the generated option set

- [ ] **Step 2: Update the settings UI to render `Auto` plus dark/light optgroups**

Keep the existing sidebar settings structure and all current code display settings. Only change `Code Theme` rendering from a flat map to grouped option rendering.

- [ ] **Step 3: Run the targeted Jest test to verify it passes**

Run: `npm test -- --runInBand web-shiki-theme-settings.test.ts`
Expected: PASS

- [ ] **Step 4: Run the existing regression test for web code layout**

Run: `npm test -- --runInBand web-code-layout.test.ts`
Expected: PASS

### Task 3: Verify And Finish

**Files:**
- Modify: `docs/superpowers/plans/2026-04-11-shiki-theme-settings-implementation.md`

- [ ] **Step 1: Run TypeScript verification for web code**

Run: `npm run tsc:web`
Expected: PASS

- [ ] **Step 2: Build release assets for app completion gate**

Run: `cd app && npm run build:web:release`
Expected: PASS

- [ ] **Step 3: Commit and push**

Run:
- `git add -A`
- `git commit -m "feat(web): expand shiki theme settings"`
- `git push origin main`
