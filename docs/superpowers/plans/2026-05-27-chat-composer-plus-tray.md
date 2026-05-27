# Chat Composer Plus Tray Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Refine the chat composer so the slash/send controls stay anchored as the textarea grows, bottom attachment tools collapse behind a plus tray, and security-like config options use a lock icon.

**Architecture:** Keep the existing `main.tsx` composer and CSS structure. Add a small local React state for the plus tray, reuse the current file mention/file upload/photo upload handlers, and shift layout responsibility to CSS so controls stay fixed at the one-line composer baseline while the textarea grows.

**Tech Stack:** React, TypeScript, CSS, Jest source-structure tests.

---

## File Structure

- Modify: `app/__tests__/web-chat-ui.test.ts`
  - Add source-structure tests for plus tray state, close behavior, layout anchoring, and lock config icon.
- Modify: `app/web/src/main.tsx`
  - Add plus tray state/refs, tray close handlers, plus tray markup, and lock icon mapping.
- Modify: `app/web/src/styles.css`
  - Shrink slash width, bottom-anchor slash/send, style the plus tray and fade overlay.

---

### Task 1: Plus Tray Source Tests

- [x] Add failing tests in `app/__tests__/web-chat-ui.test.ts` asserting:
  - `chatAttachmentTrayOpen` state and tray refs exist.
  - The bottom toolbar contains `chat-attachment-plus-button` instead of always-visible `chat-mention-button`, `chat-attach-button`, and `chat-image-attach-button`.
  - The tray renders `Code`, `File`, and `Photo` buttons with larger hit areas.
  - Input `onChange` closes the tray before updating text.
  - Pointer-down outside the tray and plus button closes the tray.

- [x] Run:

```powershell
npm test -- --runTestsByPath __tests__/web-chat-ui.test.ts --runInBand
```

Expected: FAIL before implementation.

### Task 2: Composer Layout and Tray Implementation

- [x] Add `chatAttachmentTrayOpen`, `chatAttachmentTrayRef`, and `chatAttachmentTrayButtonRef`.
- [x] Add handlers to toggle/close the tray and to trigger Code/File/Photo actions.
- [x] Replace always-visible toolbar attachment buttons with a plus button and conditional tray.
- [x] Change `chatConfigIconClass` so mode/permission/access uses `codicon-lock`.
- [x] Update textarea `onChange` to close the tray when the user types.

### Task 3: CSS Layout

- [x] Reduce slash rail width.
- [x] Restore bottom anchoring for slash/send so they stay in the one-line baseline position when textarea height grows.
- [x] Style the tray as a horizontal overlay with large labeled buttons and a right-side fade.
- [x] Align cancel with config options.

### Task 4: Verification

- [x] Run focused Jest for chat UI.
- [x] Run related responsive/relay tests.
- [x] Run full Jest, `npm run tsc:web`, and `npm run build:web`.
- [ ] Commit and push the branch.
