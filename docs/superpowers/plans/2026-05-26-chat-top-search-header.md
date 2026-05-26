# Chat Top Search Header Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move session search into the chat header on desktop and mobile, with full Hub labels and an inline search status row.

**Architecture:** Reuse the existing `main.tsx` session-search state and service calls. Move the search controls from the session list into chat header renderers, add a small status-line helper, and update CSS for default and expanded header layouts.

**Tech Stack:** React, TypeScript, Jest source-structure tests, CSS.

---

## File Map

- Modify `app/__tests__/web-session-search-ui.test.ts`: source-structure coverage for the new header placement, status text, and Hub label pluralization.
- Modify `app/web/src/main.tsx`: chat header search renderers, Hub label text, status text helper, desktop/mobile header placement.
- Modify `app/web/src/styles.css`: desktop/mobile header layout, expanded search row, status line, Hub button label/dropdown sizing.

## Task 1: Header Search Source Tests

**Files:**
- Modify: `app/__tests__/web-session-search-ui.test.ts`

- [ ] **Step 1: Write the failing test**

Add assertions for `renderChatHeaderSearchControls`, `renderSessionSearchStatusLine`, `chat-header-search-status`, desktop `sidebar-title-row search-open`, mobile search-before-Hub ordering, and full Hub label text.

- [ ] **Step 2: Run test to verify it fails**

Run: `npm test -- --runInBand web-session-search-ui.test.ts`

Expected: FAIL because the new renderers/classes are not implemented.

## Task 2: Header Search Markup

**Files:**
- Modify: `app/web/src/main.tsx`

- [ ] **Step 1: Implement minimal markup**

Create `renderChatHeaderSearchControls()` and `renderSessionSearchStatusLine()`. Use existing `sessionSearchOpen`, `activeSessionSearchId`, `startSessionSearch()`, and `exitSessionSearch()` state. Remove `renderSessionSearchControls()` from the wide and mobile session list body. Render header search in:

- wide `sidebar-title-row`
- mobile `mobile-chat-drawer-header`

- [ ] **Step 2: Run focused test**

Run: `npm test -- --runInBand web-session-search-ui.test.ts`

Expected: PASS for source-structure tests.

## Task 3: Hub Label And Header CSS

**Files:**
- Modify: `app/web/src/main.tsx`
- Modify: `app/web/src/styles.css`

- [ ] **Step 1: Implement full Hub label**

Change Hub summary button text to `${hubCount} Hub(s)` and remove the separate numeric badge from the button.

- [ ] **Step 2: Implement layout styles**

Add explicit classes for:

- `.sidebar-title-row.search-open`
- `.chat-header-search-control`
- `.chat-header-search-control.open`
- `.chat-header-search-status`
- mobile header search-expanded layout

Keep dropdown wider than the button and bounded by viewport.

- [ ] **Step 3: Run focused test**

Run: `npm test -- --runInBand web-session-search-ui.test.ts`

Expected: PASS.

## Task 4: Verification

**Files:**
- Verify only.

- [ ] **Step 1: Run Web tests**

Run: `npm test -- --runInBand`

Expected: PASS.

- [ ] **Step 2: Run TypeScript check**

Run: `npm run tsc:web`

Expected: PASS.

- [ ] **Step 3: Run Web build**

Run: `npm run build:web`

Expected: PASS.
