# Chat Inline Confirmation Replies Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an inline check-frame quick reply for the latest assistant confirmation question when no A/B/C option block is present.

**Architecture:** Keep detection in the existing pure parser module `app/web/src/chat/chatOptionReplies.ts`. `main.tsx` stays responsible for deciding whether the latest assistant message is selectable and for sending the mapped reply through the existing `sendDirectChatText` path.

**Tech Stack:** React, TypeScript, Jest, existing Codicon/CSS chat UI.

---

### Task 1: Parser Support For Confirmation Questions

**Files:**
- Modify: `app/web/src/chat/chatOptionReplies.ts`
- Test: `app/__tests__/web-chat-option-replies.test.ts`

- [ ] **Step 1: Write failing parser tests**

Add tests for:

```ts
import {
  extractChatConfirmationReply,
  splitChatConfirmationReplyText,
} from '../web/src/chat/chatOptionReplies';

test('extracts trailing Chinese confirmation replies with mapped reply text', () => {
  expect(extractChatConfirmationReply('我的推荐是 B。你认这个边界吗？')).toEqual({
    sentence: '你认这个边界吗？',
    replyText: '确认',
  });
  expect(extractChatConfirmationReply('确认这个修正版？')).toEqual({
    sentence: '确认这个修正版？',
    replyText: '确认',
  });
  expect(extractChatConfirmationReply('你同意这个定义吗？')).toEqual({
    sentence: '你同意这个定义吗？',
    replyText: '同意',
  });
  expect(extractChatConfirmationReply('你接受这个例外吗？还是你要更强规则？')).toEqual({
    sentence: '你接受这个例外吗？',
    replyText: '接受',
  });
});

test('does not extract confirmation replies from option prompts, code fences, or English text', () => {
  expect(extractChatConfirmationReply(['A. 确认', 'B. 接受'].join('\n'))).toBeNull();
  expect(extractChatConfirmationReply(['```text', '确认这个修正版？', '```'].join('\n'))).toBeNull();
  expect(extractChatConfirmationReply('Does this look right?')).toBeNull();
});

test('splits the confirmation sentence while preserving surrounding markdown', () => {
  expect(splitChatConfirmationReplyText('前文 **说明**。你同意这个定义吗？')).toEqual([
    {type: 'markdown', text: '前文 **说明**。'},
    {type: 'confirmation', reply: {sentence: '你同意这个定义吗？', replyText: '同意'}},
  ]);
});
```

- [ ] **Step 2: Run parser test and verify RED**

Run: `cd app && npm test -- --runInBand __tests__/web-chat-option-replies.test.ts`

Expected: FAIL because `extractChatConfirmationReply` and `splitChatConfirmationReplyText` are not exported.

- [ ] **Step 3: Implement minimal parser helpers**

Add:

```ts
export type ChatConfirmationReply = {
  sentence: string;
  replyText: '确认' | '接受' | '同意';
};

export type ChatConfirmationReplyTextPart =
  | {type: 'markdown'; text: string}
  | {type: 'confirmation'; reply: ChatConfirmationReply};
```

Implement `extractChatConfirmationReply(text)` and `splitChatConfirmationReplyText(text)` by scanning non-code-fence lines near the end of the text. Return no confirmation when `extractChatOptionReplies(text)` is non-empty.

- [ ] **Step 4: Run parser test and verify GREEN**

Run: `cd app && npm test -- --runInBand __tests__/web-chat-option-replies.test.ts`

Expected: PASS.

### Task 2: Inline Confirmation Rendering

**Files:**
- Modify: `app/web/src/main.tsx`
- Modify: `app/web/src/styles.css`
- Test: `app/__tests__/web-chat-ui.test.ts`

- [ ] **Step 1: Write failing UI source assertions**

Assert that `main.tsx` imports and uses the new helpers:

```ts
expect(mainTsx).toContain('extractChatConfirmationReply');
expect(mainTsx).toContain('splitChatConfirmationReplyText(text)');
expect(mainTsx).toContain('const confirmationReplyParts = splitChatConfirmationReplyText(text);');
expect(mainTsx).toContain('optionReplies.length === 0');
expect(mainTsx).toContain('className="chat-confirmation-reply-line"');
expect(mainTsx).toContain('className="chat-confirmation-reply-check"');
expect(mainTsx).toContain('className="chat-confirmation-reply-text"');
expect(mainTsx).toContain('onClick={() => onSelectConfirmationReply?.(part.reply.replyText)}');
```

Assert CSS classes exist:

```ts
expect(stylesCss).toContain('.chat-confirmation-reply-line {');
expect(stylesCss).toContain('.chat-confirmation-reply-action {');
expect(stylesCss).toContain('.chat-confirmation-reply-check {');
expect(stylesCss).toContain('.chat-confirmation-reply-text {');
```

- [ ] **Step 2: Run UI test and verify RED**

Run: `cd app && npm test -- --runInBand __tests__/web-chat-ui.test.ts`

Expected: FAIL because confirmation helpers and classes are not wired.

- [ ] **Step 3: Implement UI wiring**

Update `ChatTurnViewProps` with:

```ts
confirmationReply?: ChatConfirmationReply | null;
onSelectConfirmationReply?: (replyText: string) => void;
```

In `ChatTurnView`, render `splitChatConfirmationReplyText(text)` only when `confirmationReply` is present and A/B/C option parts are absent. The confirmation part should render a button containing the check frame and sentence text.

In latest-message selection, compute confirmation data only when `extractChatOptionReplies(text).length === 0`, and pass it only to the latest eligible assistant message.

- [ ] **Step 4: Add CSS**

Add compact inline styles next to the existing option reply CSS. The action should look like an inline framed sentence with a small check frame, not a large separate button.

- [ ] **Step 5: Run UI test and verify GREEN**

Run: `cd app && npm test -- --runInBand __tests__/web-chat-ui.test.ts`

Expected: PASS.

### Task 3: Verification And Completion

**Files:**
- Verify: `app/web/src/chat/chatOptionReplies.ts`
- Verify: `app/web/src/main.tsx`
- Verify: `app/web/src/styles.css`
- Verify: `app/__tests__/web-chat-option-replies.test.ts`
- Verify: `app/__tests__/web-chat-ui.test.ts`

- [ ] **Step 1: Run targeted parser and UI tests**

Run: `cd app && npm test -- --runInBand __tests__/web-chat-option-replies.test.ts __tests__/web-chat-ui.test.ts`

Expected: PASS.

- [ ] **Step 2: Run TypeScript check**

Run: `cd app && npm run tsc:web`

Expected: PASS.

- [ ] **Step 3: Run app test suite**

Run: `cd app && npm test -- --runInBand`

Expected: PASS.

- [ ] **Step 4: Run diff whitespace check**

Run: `git diff --check`

Expected: no output.

- [ ] **Step 5: Execute repository completion gate**

Run from repository root:

```powershell
git add -A
git commit -m "feat(web): add inline confirmation quick replies"
git push origin main
cd app
npm run build:web:release
```

Expected: commit and push succeed; web release assets are exported to `~/.wheelmaker/web`.

## Self-Review

- Spec coverage: latest-only behavior, A/B/C priority, Chinese-only detection, reply mapping, clickable check frame plus sentence, composer preservation, and historical normal Markdown are covered by Tasks 1 and 2.
- Placeholder scan: no placeholders are present.
- Type consistency: `ChatConfirmationReply`, `extractChatConfirmationReply`, and `splitChatConfirmationReplyText` are used consistently across tests, parser, and UI.
