# Chat Composer Tool Rebalance Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rebalance the chat composer controls by removing composer quick replies, moving Skills to the input-row left button, adding an `@` file mention placeholder action, and removing config pill chevrons.

**Architecture:** Keep the implementation inside the existing `app/web/src/main.tsx` composer code and `app/web/src/styles.css` styles. Preserve message-level option/confirmation replies; only composer-level action state and markup changes.

**Tech Stack:** React, TypeScript, Jest source-structure tests, existing Codicon/CSS chat UI.

---

### Task 1: Source Assertions

**Files:**
- Modify: `app/__tests__/web-chat-ui.test.ts`

- [x] **Step 1: Write failing assertions**

Update the existing `chat composer is a unified command frame with compact custom config pills` test to require:

```ts
expect(mainTsx).not.toContain('CHAT_QUICK_REPLY_OPTIONS');
expect(mainTsx).not.toContain('chatQuickReplyMenuOpen');
expect(mainTsx).not.toContain('openChatQuickReplyMenu');
expect(mainTsx).not.toContain('handleChatQuickReplySelect');
expect(mainTsx).not.toContain('className="chat-quick-reply-menu"');
expect(mainTsx).not.toContain('className="chat-quick-reply-item"');
expect(mainTsx).toContain('className="chat-composer-skill-trigger"');
expect(mainTsx).toContain('title="Skills"');
expect(mainTsx).toContain('aria-label="Open skills"');
expect(mainTsx).toContain('codicon-terminal');
expect(mainTsx).toContain('const [chatFileMentionMenuOpen, setChatFileMentionMenuOpen] = useState(false);');
expect(mainTsx).toContain('const openChatFileMentionMenu = useCallback(() => {');
expect(mainTsx).toContain('className="chat-tool-button chat-mention-button"');
expect(mainTsx).toContain('className="chat-file-mention-menu"');
expect(mainTsx).toContain('File mentions coming soon');
expect(mainTsx).not.toContain('<span className="codicon codicon-chevron-down" aria-hidden="true" />');
expect(mainTsx).toContain('className="chat-config-value-menu"');
```

Update CSS assertions to require new classes and removal of quick reply classes:

```ts
expect(stylesCss).toContain('.chat-composer-skill-trigger {');
expect(stylesCss).toContain('.chat-mention-button {');
expect(stylesCss).toContain('.chat-file-mention-menu {');
expect(stylesCss).toContain('.chat-file-mention-empty {');
expect(stylesCss).not.toContain('.chat-composer-quick-trigger {');
expect(stylesCss).not.toContain('.chat-quick-reply-menu {');
expect(stylesCss).not.toContain('.chat-quick-reply-item {');
```

- [x] **Step 2: Run UI test and verify RED**

Run: `cd app && npm test -- --runInBand __tests__/web-chat-ui.test.ts`

Expected: FAIL because the old quick reply controls still exist and the new Skills/mention controls do not.

### Task 2: Composer Control Wiring

**Files:**
- Modify: `app/web/src/main.tsx`
- Modify: `app/web/src/styles.css`
- Test: `app/__tests__/web-chat-ui.test.ts`

- [x] **Step 1: Remove composer quick reply state and handlers**

Remove `CHAT_QUICK_REPLY_OPTIONS`, quick reply refs, `chatQuickReplyMenuOpen`, `openChatQuickReplyMenu`, `handleChatQuickReplySelect`, the quick reply outside-click effect, and the quick reply menu JSX.

- [x] **Step 2: Move Skills to the input row**

Replace the left quick reply button with:

```tsx
<button
  ref={chatPromptButtonRef}
  type="button"
  className="chat-composer-skill-trigger"
  onPointerDown={event => event.preventDefault()}
  onClick={openChatPromptMenu}
  title="Skills"
  aria-label="Open skills"
  aria-haspopup="listbox"
  aria-expanded={chatPromptMenuOpen}
>
  <span className="codicon codicon-terminal" />
</button>
```

- [x] **Step 3: Add file mention placeholder state and menu**

Add:

```ts
const [chatFileMentionMenuOpen, setChatFileMentionMenuOpen] = useState(false);
```

Add `openChatFileMentionMenu` that closes Skills/config menus and toggles the file mention menu without changing composer text.

Render:

```tsx
{chatFileMentionMenuOpen ? (
  <div className="chat-file-mention-menu" role="menu" aria-label="File mentions">
    <div className="chat-file-mention-empty">File mentions coming soon</div>
  </div>
) : null}
```

- [x] **Step 4: Replace lower Skills with @**

Replace the lower `.chat-skill-button` with:

```tsx
<button
  type="button"
  className="chat-tool-button chat-mention-button"
  onClick={openChatFileMentionMenu}
  title="Mention files"
  aria-label="Mention files"
  aria-haspopup="menu"
  aria-expanded={chatFileMentionMenuOpen}
>
  <span className="chat-mention-symbol">@</span>
</button>
```

Keep image and stop buttons after it.

- [x] **Step 5: Remove config pill chevron**

Delete only the config pill chevron span from `renderChatConfigPill`. Keep the overflow chevron.

- [x] **Step 6: Update menu-closing paths**

When Skills opens, close file mentions. When file mentions open, close Skills/config/overflow. When image attach opens or slash command is applied, close file mentions.

- [x] **Step 7: Update CSS**

Rename quick trigger styles to `.chat-composer-skill-trigger`, remove quick reply menu/item styles, add `.chat-mention-button`, `.chat-mention-symbol`, `.chat-file-mention-menu`, and `.chat-file-mention-empty`.

- [x] **Step 8: Run UI test and verify GREEN**

Run: `cd app && npm test -- --runInBand __tests__/web-chat-ui.test.ts`

Expected: PASS.

### Task 3: Verification And Completion

**Files:**
- Verify: `app/__tests__/web-chat-ui.test.ts`
- Verify: `app/web/src/main.tsx`
- Verify: `app/web/src/styles.css`
- Verify: `docs/superpowers/plans/2026-05-18-chat-composer-tool-rebalance.md`

- [x] **Step 1: Run targeted UI test**

Run: `cd app && npm test -- --runInBand __tests__/web-chat-ui.test.ts`

Expected: PASS.

- [x] **Step 2: Run TypeScript check**

Run: `cd app && npm run tsc:web`

Expected: PASS.

- [x] **Step 3: Run full app tests**

Run: `cd app && npm test -- --runInBand`

Expected: PASS.

- [x] **Step 4: Run diff whitespace check**

Run: `git diff --check`

Expected: no whitespace errors.

- [x] **Step 5: Execute completion gate**

Run from repo root:

```powershell
git add -A
git commit -m "feat(web): rebalance chat composer tools"
git push origin main
cd app
npm run build:web:release
```

Expected: commit and push succeed; release exports to `~/.wheelmaker/web`.

## Self-Review

- Spec coverage: quick reply removal, Skills relocation, `@` placeholder, config chevron removal, message reply preservation, and verification are covered.
- Placeholder scan: no unspecified implementation placeholders remain.
- Type consistency: `chatFileMentionMenuOpen`, `openChatFileMentionMenu`, `chat-composer-skill-trigger`, and `chat-mention-button` are used consistently.
