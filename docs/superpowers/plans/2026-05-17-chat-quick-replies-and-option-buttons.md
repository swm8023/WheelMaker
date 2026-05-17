# Chat Quick Replies And Option Buttons Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the chat composer voice affordance with quick skill/image actions and add click-to-reply option chips for the latest assistant choice prompt.

**Architecture:** Keep option detection in a small pure helper under `app/web/src/chat/` so false-positive rules are testable. Keep `main.tsx` responsible for UI state, rendering, and sending short direct replies through the existing session send path without mutating the current composer draft.

**Tech Stack:** React, TypeScript, Jest, existing Codicon/CSS chat UI.

---

### Task 1: Option Reply Parser

**Files:**
- Create: `app/web/src/chat/chatOptionReplies.ts`
- Test: `app/__tests__/web-chat-option-replies.test.ts`

- [ ] Add failing tests for `A.`/`B.` line-start option extraction, code-fence exclusion, and false-positive rejection for `方案 A` and inline summaries.
- [ ] Implement `extractChatOptionReplies(text)` returning contiguous labels from `A` onward with display text, only when at least two options are present.
- [ ] Run `cd app && npm test -- --runInBand __tests__/web-chat-option-replies.test.ts`.

### Task 2: Composer Toolbar And Quick Phrase Menu

**Files:**
- Modify: `app/web/src/main.tsx`
- Modify: `app/web/src/styles.css`
- Test: `app/__tests__/web-chat-ui.test.ts`

- [ ] Update structural tests so the top-left composer button opens quick phrases, the lower first tool button opens the existing skill menu, the second tool button attaches images, and voice UI/error text are absent.
- [ ] Add direct-send support that sends override text without clearing the existing composer text or attachments.
- [ ] Add quick phrase menu values `A`, `B`, `C`, `确认`, `接受`; clicking one sends that exact text.

### Task 3: Latest Assistant Option Chips

**Files:**
- Modify: `app/web/src/main.tsx`
- Modify: `app/web/src/styles.css`
- Test: `app/__tests__/web-chat-ui.test.ts`

- [ ] Use `extractChatOptionReplies` on assistant message text and render chips only for the latest eligible assistant turn in the selected session.
- [ ] Chips send only the label, such as `A`, through the same direct-send path and keep current draft/attachments intact.
- [ ] Style chips as compact assistant-adjacent actions consistent with the existing utilitarian chat surface.

### Verification

- [ ] Run targeted Jest tests for option parsing and chat UI.
- [ ] Run web TypeScript check.
- [ ] Run existing app test suite if targeted checks pass.
- [ ] Execute repository completion gate: `git add -A`, commit, push, then `cd app && npm run build:web:release`.
