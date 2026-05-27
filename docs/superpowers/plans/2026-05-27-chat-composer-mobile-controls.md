# Chat Composer Mobile Controls Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Update the chat composer controls and mobile floating navigation so the requested UI behaviors are implemented and covered by focused tests.

**Architecture:** Extend the existing `main.tsx` composer and floating-control code instead of extracting new components. Add the mobile floating side as persisted workspace UI state beside the existing vertical floating slot, then mirror CSS placement based on a `data-side` attribute.

**Tech Stack:** React 19, TypeScript, CSS, Jest source-structure tests.

---

## File Structure

- Modify: `app/__tests__/web-chat-ui.test.ts`
  - Source-structure tests for composer layout, image input, and mobile Enter behavior.
- Modify: `app/__tests__/web-responsive-ui-state.test.ts`
  - Reducer tests for persisted floating side defaulting, sanitizing, and layout reset behavior.
- Modify: `app/__tests__/web-port-relay-settings.test.ts`
  - Source-structure tests for floating side persistence and mirrored relay target menu CSS.
- Modify: `app/web/src/services/workspaceUiState.ts`
  - Add `left | right` side state and reducer action.
- Modify: `app/web/src/services/workspacePersistence.ts`
  - Persist floating side in global workspace state.
- Modify: `app/web/src/main.tsx`
  - Reorder composer controls, add image input, route image button to existing attachment enqueue path, send on mobile Enter, and snap floating control side on drag release.
- Modify: `app/web/src/styles.css`
  - Bottom-align the send/cancel column, style the new control placement, and mirror floating controls/menus for left side.

---

### Task 1: Composer Structure Tests

**Files:**
- Modify: `app/__tests__/web-chat-ui.test.ts`

- [x] **Step 1: Write the failing composer layout test**

Add this test near the existing chat composer source-structure assertions:

```ts
test('places skills beside the composer input and stacks send above cancel on the right', () => {
  const projectRoot = path.join(__dirname, '..');
  const mainTsx = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'main.tsx'), 'utf8');
  const stylesCss = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'styles.css'), 'utf8');

  const inputRowStart = mainTsx.indexOf('className="chat-composer-input-row"');
  const inputRowEnd = mainTsx.indexOf('{chatFileMentionMenuOpen ?', inputRowStart);
  expect(inputRowStart).toBeGreaterThanOrEqual(0);
  expect(inputRowEnd).toBeGreaterThan(inputRowStart);
  const inputRow = mainTsx.slice(inputRowStart, inputRowEnd);

  expect(inputRow).toContain('className="chat-composer-skill-trigger chat-slash-button"');
  expect(inputRow).toContain('onClick={openChatPromptMenu}');
  expect(inputRow.indexOf('chat-composer-skill-trigger')).toBeLessThan(inputRow.indexOf('className="chat-composer-input-shell"'));
  expect(inputRow).toContain('className="chat-composer-action-column"');
  expect(inputRow).toContain('className="chat-send-button"');
  expect(inputRow).toContain('className={`chat-composer-stop-trigger${selectedChatPromptRunning ? \\' active\\' : \\'\\'}`}');
  expect(inputRow.indexOf('chat-send-button')).toBeLessThan(inputRow.indexOf('chat-composer-stop-trigger'));

  const toolsStart = mainTsx.indexOf('className="chat-composer-tools"');
  const toolsEnd = mainTsx.indexOf('className="chat-config-options-wrap"', toolsStart);
  const toolsBlock = mainTsx.slice(toolsStart, toolsEnd);
  expect(toolsBlock).not.toContain('chat-slash-button');

  expect(stylesCss).toMatch(/\\.chat-composer-input-row \\{[\\s\\S]*align-items: flex-end;[\\s\\S]*\\}/);
  expect(stylesCss).toContain('.chat-composer-action-column');
});
```

- [x] **Step 2: Run the failing test**

Run:

```powershell
npm test -- --runTestsByPath __tests__/web-chat-ui.test.ts --runInBand
```

Expected: FAIL because `chat-composer-skill-trigger` and `chat-composer-action-column` do not exist yet, and the toolbar still contains `chat-slash-button`.

- [x] **Step 3: Implement composer layout**

In `app/web/src/main.tsx`:

- Move the existing Skills button markup from `chat-composer-toolbar` into `chat-composer-input-row` before `chat-composer-input-shell`.
- Wrap Send and Cancel in `<div className="chat-composer-action-column">`.
- Move the existing cancel button into that column under the Send button.
- Remove the old bottom-toolbar Skills button.

The input row should include this structure:

```tsx
<div className="chat-composer-input-row">
  <button
    type="button"
    ref={chatPromptButtonRef}
    className="chat-composer-skill-trigger chat-slash-button"
    onPointerDown={event => event.preventDefault()}
    onClick={openChatPromptMenu}
    title="Skills"
    aria-label="Open skills"
    aria-haspopup="listbox"
    aria-expanded={chatPromptMenuOpen}
  >
    <span className="chat-slash-symbol">/</span>
  </button>
  <div className="chat-composer-input-shell">
    <textarea
      ref={chatComposerTextareaRef}
      rows={1}
      className="chat-composer-input"
      value={chatComposerText}
      readOnly={chatSending}
      onChange={event => updateChatComposerText(event.target.value)}
      placeholder="Send a message..."
    />
  </div>
  <div className="chat-composer-action-column">
    <button type="button" className="chat-send-button" aria-label="Send message">
      <span className="codicon codicon-send" />
    </button>
    <button type="button" className={`chat-composer-stop-trigger${selectedChatPromptRunning ? ' active' : ''}`} aria-label="Cancel prompt">
      <span className={`codicon ${selectedChatPromptCancelling ? 'codicon-loading codicon-modifier-spin' : 'codicon-debug-stop'}`} aria-hidden="true" />
    </button>
  </div>
</div>
```

In `app/web/src/styles.css`:

```css
.chat-composer-input-row {
  display: flex;
  align-items: flex-end;
  gap: 5px;
  min-height: 32px;
  min-width: 0;
}

.chat-composer-skill-trigger {
  flex: 0 0 auto;
  width: 32px;
  height: 32px;
  border: none;
  border-radius: 8px;
  background: transparent;
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  justify-content: center;
}

.chat-composer-action-column {
  flex: 0 0 auto;
  width: 32px;
  display: flex;
  flex-direction: column;
  align-items: stretch;
  justify-content: flex-end;
  gap: 4px;
}
```

- [x] **Step 4: Run the composer layout test again**

Run:

```powershell
npm test -- --runTestsByPath __tests__/web-chat-ui.test.ts --runInBand
```

Expected: PASS for the new composer layout test. If older tests fail because they expected the stop button on the left or the toolbar Skills button, update those assertions to the new design without weakening the new test.

---

### Task 2: Image Attachment Button Tests and Implementation

**Files:**
- Modify: `app/__tests__/web-chat-ui.test.ts`
- Modify: `app/web/src/main.tsx`
- Modify: `app/web/src/styles.css`

- [x] **Step 1: Write the failing image attachment test**

Add this test near the existing attachment assertions:

```ts
test('adds a dedicated multi-image attachment picker that reuses chat attachment enqueueing', () => {
  const projectRoot = path.join(__dirname, '..');
  const mainTsx = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'main.tsx'), 'utf8');
  const stylesCss = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'styles.css'), 'utf8');

  expect(mainTsx).toContain('const chatImageInputRef = useRef<HTMLInputElement | null>(null);');
  expect(mainTsx).toContain('const handleChatImageChange = (');
  expect(mainTsx).toContain('accept="image/*"');
  expect(mainTsx).toContain('ref={chatImageInputRef}');
  expect(mainTsx).toContain('chatImageInputRef.current?.click();');
  expect(mainTsx).toContain('className="chat-tool-button chat-image-attach-button"');
  expect(mainTsx).toContain('aria-label="Attach image"');

  const imageHandlerStart = mainTsx.indexOf('const handleChatImageChange = (');
  const imageHandlerEnd = mainTsx.indexOf('const connect = async', imageHandlerStart);
  const imageHandler = mainTsx.slice(imageHandlerStart, imageHandlerEnd);
  expect(imageHandler).toContain('const files = chatFilesFromFileList(event.target.files);');
  expect(imageHandler).toContain('enqueueChatAttachmentFiles(files, attachmentDraftKey, attachmentDraftGeneration);');
  expect(imageHandler).toContain("event.target.value = '';");

  const toolsStart = mainTsx.indexOf('className="chat-composer-tools"');
  const toolsEnd = mainTsx.indexOf('className="chat-config-options-wrap"', toolsStart);
  const toolsBlock = mainTsx.slice(toolsStart, toolsEnd);
  expect(toolsBlock.indexOf('chat-attach-button')).toBeLessThan(toolsBlock.indexOf('chat-image-attach-button'));

  expect(stylesCss).toContain('.chat-image-attach-button');
});
```

- [x] **Step 2: Run the failing image test**

Run:

```powershell
npm test -- --runTestsByPath __tests__/web-chat-ui.test.ts --runInBand
```

Expected: FAIL because `chatImageInputRef`, `handleChatImageChange`, and `.chat-image-attach-button` are missing.

- [x] **Step 3: Implement image attachment picker**

In `app/web/src/main.tsx`:

```tsx
const chatImageInputRef = useRef<HTMLInputElement | null>(null);
```

Add:

```tsx
const handleChatImageChange = (
  event: React.ChangeEvent<HTMLInputElement>,
) => {
  if (chatSending) {
    event.target.value = '';
    return;
  }
  const files = chatFilesFromFileList(event.target.files);
  if (files.length === 0) {
    return;
  }
  const attachmentDraftKey = currentChatDraftKeyRef.current;
  const attachmentDraftGeneration = getChatDraftGeneration(attachmentDraftKey);
  enqueueChatAttachmentFiles(files, attachmentDraftKey, attachmentDraftGeneration);
  event.target.value = '';
};
```

Add a hidden input beside the file input:

```tsx
<input
  ref={chatImageInputRef}
  type="file"
  multiple
  accept="image/*"
  style={{ display: 'none' }}
  onChange={handleChatImageChange}
/>
```

Add the toolbar button immediately after the file button:

```tsx
<button
  type="button"
  className="chat-tool-button chat-image-attach-button"
  onClick={() => {
    setChatPromptMenuOpen(false);
    setChatFileMentionMenuOpen(false);
    setChatConfigMenuOptionId('');
    setChatConfigOverflowOpen(false);
    chatImageInputRef.current?.click();
  }}
  disabled={chatSending}
  title="Attach image"
  aria-label="Attach image"
>
  <span className="codicon codicon-file-media" />
</button>
```

In `app/web/src/styles.css`:

```css
.chat-image-attach-button {
  color: color-mix(in srgb, #7aa2ff 74%, var(--text));
}
```

- [x] **Step 4: Run the image attachment test again**

Run:

```powershell
npm test -- --runTestsByPath __tests__/web-chat-ui.test.ts --runInBand
```

Expected: PASS for the image attachment test.

---

### Task 3: Mobile Enter Send Tests and Implementation

**Files:**
- Modify: `app/__tests__/web-chat-ui.test.ts`
- Modify: `app/web/src/main.tsx`

- [x] **Step 1: Write the failing mobile Enter test**

Add this test near the composer tests:

```ts
test('uses mobile Enter as send while keeping modified Enter and IME composition from sending', () => {
  const projectRoot = path.join(__dirname, '..');
  const mainTsx = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'main.tsx'), 'utf8');

  expect(mainTsx).toContain('enterKeyHint={isWide ? undefined : \\'send\\'}');
  expect(mainTsx).toContain('const shouldSendChatOnEnter = event.key === \\'Enter\\' && !event.shiftKey && !event.altKey && !event.nativeEvent.isComposing;');
  expect(mainTsx).toContain('if (!isWide || isWindowsPlatform) {');
  expect(mainTsx).toContain('if (!shouldSendChatOnEnter) {');
  expect(mainTsx).toContain('event.preventDefault();');
  expect(mainTsx).toContain('sendChatMessage().catch(() => undefined);');
});
```

- [x] **Step 2: Run the failing mobile Enter test**

Run:

```powershell
npm test -- --runTestsByPath __tests__/web-chat-ui.test.ts --runInBand
```

Expected: FAIL because the textarea lacks `enterKeyHint` and current Enter handling sends only on Windows.

- [x] **Step 3: Implement mobile Enter behavior**

In the textarea props in `app/web/src/main.tsx`, add:

```tsx
enterKeyHint={isWide ? undefined : 'send'}
```

Replace the platform-specific Enter block with:

```tsx
const shouldSendChatOnEnter = event.key === 'Enter' && !event.shiftKey && !event.altKey && !event.nativeEvent.isComposing;
if (!shouldSendChatOnEnter) {
  return;
}
if (!isWide || isWindowsPlatform) {
  event.preventDefault();
  if (chatSending || chatAttachmentUploadPending) {
    return;
  }
  sendChatMessage().catch(() => undefined);
}
```

Keep the slash-menu Enter selection branch before this block so selecting a slash item still takes precedence.

- [x] **Step 4: Run the mobile Enter test again**

Run:

```powershell
npm test -- --runTestsByPath __tests__/web-chat-ui.test.ts --runInBand
```

Expected: PASS for the mobile Enter behavior test.

---

### Task 4: Mobile Floating Side State, Persistence, Drag Snap, and CSS

**Files:**
- Modify: `app/__tests__/web-responsive-ui-state.test.ts`
- Modify: `app/__tests__/web-port-relay-settings.test.ts`
- Modify: `app/web/src/services/workspaceUiState.ts`
- Modify: `app/web/src/services/workspacePersistence.ts`
- Modify: `app/web/src/main.tsx`
- Modify: `app/web/src/styles.css`

- [x] **Step 1: Write failing reducer and persistence tests**

In `app/__tests__/web-responsive-ui-state.test.ts`, extend the existing state test:

```ts
floatingControlSide: 'left',
```

Then add assertions:

```ts
expect(state.mobile).toMatchObject({
  drawerOpen: true,
  floatingControlSlot: 'center',
  floatingControlSide: 'left',
  chatConfigOverflowOpen: true,
});
expect(createWorkspaceUiState({ floatingControlSide: 'invalid' }).mobile.floatingControlSide).toBe('right');
```

After the `layout/modeChanged` action, add:

```ts
expect(state.mobile.floatingControlSide).toBe('left');
```

Add a reducer action check:

```ts
state = workspaceUiReducer(state, {
  type: 'mobile/setFloatingControlSide',
  next: 'right',
});
expect(state.mobile.floatingControlSide).toBe('right');
```

In the persistence test in the same file, add:

```ts
expect(persistenceTs).toContain("export type PersistedFloatingControlSide = 'left' | 'right';");
expect(persistenceTs).toContain('floatingControlSide: PersistedFloatingControlSide;');
expect(persistenceTs).toContain("floatingControlSide: 'floatingControlSide',");
expect(persistenceTs).toContain("floatingControlSide: 'right',");
expect(persistenceTs).toContain('floatingControlSide: sanitizeFloatingControlSide(input.floatingControlSide, base.floatingControlSide),');
expect(persistenceTs).toContain('{k: GLOBAL_KEYS.floatingControlSide, v: serialize(next.floatingControlSide), updatedAt: now}');
```

- [x] **Step 2: Write failing floating UI source tests**

In `app/__tests__/web-port-relay-settings.test.ts`, extend the mobile relay frame/floating test with:

```ts
expect(mainTsx).toContain('const PORT_RELAY_FLOATING_SIDE_STORAGE_KEY = \\'wheelmaker:portRelayFloatingSide\\';');
expect(mainTsx).toContain('readPortRelayFloatingSide()');
expect(mainTsx).toContain('window.localStorage.setItem(PORT_RELAY_FLOATING_SIDE_STORAGE_KEY, nextSide);');
expect(mainTsx).toContain('const floatingControlSide = workspaceUiState.mobile.floatingControlSide;');
expect(mainTsx).toContain('data-side={floatingControlSide}');
expect(mainTsx).toContain('originX: event.clientX,');
expect(mainTsx).toContain('startSide: floatingControlSide,');
expect(mainTsx).toContain('currentX: event.clientX,');
expect(mainTsx).toContain('const nextSide = current.currentX < windowWidth / 2 ? \\'left\\' : \\'right\\';');
expect(mainTsx).toContain("dispatchWorkspaceUi({ type: 'mobile/setFloatingControlSide', next });");
expect(stylesCss).toContain(".floating-control-stack[data-side='left']");
expect(stylesCss).toContain(".floating-control-stack[data-side='right']");
expect(stylesCss).toContain(".floating-control-stack[data-side='left'] .port-relay-target-switch-menu");
```

- [x] **Step 3: Run failing floating tests**

Run:

```powershell
npm test -- --runTestsByPath __tests__/web-responsive-ui-state.test.ts __tests__/web-port-relay-settings.test.ts --runInBand
```

Expected: FAIL because `floatingControlSide` state, persistence, drag X tracking, and mirrored CSS do not exist.

- [x] **Step 4: Implement UI state and persistence**

In `app/web/src/services/workspacePersistence.ts`, add:

```ts
export type PersistedFloatingControlSide = 'left' | 'right';
```

Add `floatingControlSide` to the global state type, global keys, defaults, sanitization, hydrate, and remember paths. The sanitizer should accept only `left` and `right`, falling back to the base value or `right`.

In `app/web/src/services/workspaceUiState.ts`:

```ts
import type {
  PersistedFloatingControlSide,
  PersistedFloatingControlSlot,
  PersistedTab,
} from './workspacePersistence';
```

Add `floatingControlSide` to `WorkspaceUiState.mobile`, `WorkspaceUiStateInput`, and `WorkspaceUiAction`:

```ts
| {
    type: 'mobile/setFloatingControlSide';
    next: WorkspaceUiStateValue<PersistedFloatingControlSide>;
  }
```

Add:

```ts
function sanitizeFloatingControlSide(value: unknown): PersistedFloatingControlSide {
  return value === 'left' || value === 'right' ? value : 'right';
}
```

Handle the reducer action by sanitizing the next side.

- [x] **Step 5: Implement floating drag side snap**

In `app/web/src/main.tsx`:

- Add `PORT_RELAY_FLOATING_SIDE_STORAGE_KEY`.
- Add `readPortRelayFloatingSide()`.
- Initialize `floatingControlSide` from workspace UI state, using global persistence and local storage fallback.
- Add `setFloatingControlSide`.
- Extend `WorkspaceFloatingDragState` with `originX`, `currentX`, and `startSide`.
- In `beginFloatingPress`, capture `event.clientX` and current side.
- In `handleFloatingPointerMove`, update `currentX`.
- In `finishFloatingDrag`, compute:

```ts
const nextSide = current.currentX < windowWidth / 2 ? 'left' : 'right';
setFloatingControlSide(nextSide);
workspaceStore.rememberGlobalState({ floatingControlSlot: nextSlot, floatingControlSide: nextSide });
window.localStorage.setItem(PORT_RELAY_FLOATING_SIDE_STORAGE_KEY, nextSide);
```

- Add `data-side={floatingControlSide}` to `.floating-control-stack`.

- [x] **Step 6: Implement mirrored CSS**

In `app/web/src/styles.css`:

```css
.floating-control-stack[data-side='right'] {
  right: calc(env(safe-area-inset-right, 0px) + 6px);
  left: auto;
}

.floating-control-stack[data-side='left'] {
  left: calc(env(safe-area-inset-left, 0px) + 6px);
  right: auto;
}

.floating-control-stack[data-side='left'] .port-relay-target-switch-menu {
  left: 58px;
  right: auto;
}
```

Keep `.floating-control-stack-layer` full width so the floating stack can be positioned on either side. The drawer width continues to reserve `--mobile-floating-control-lane`; no settings toggle is added.

- [x] **Step 7: Run floating tests again**

Run:

```powershell
npm test -- --runTestsByPath __tests__/web-responsive-ui-state.test.ts __tests__/web-port-relay-settings.test.ts --runInBand
```

Expected: PASS for floating side state, persistence, and source-structure assertions.

---

### Task 5: Full Verification and Commit

**Files:**
- All modified files from prior tasks.

- [x] **Step 1: Run focused tests**

Run:

```powershell
npm test -- --runTestsByPath __tests__/web-chat-ui.test.ts __tests__/web-responsive-ui-state.test.ts __tests__/web-port-relay-settings.test.ts --runInBand
```

Expected: PASS.

- [x] **Step 2: Run TypeScript check**

Run:

```powershell
npm run tsc:web
```

Expected: exit code 0.

- [x] **Step 3: Run web build**

Run:

```powershell
npm run build:web
```

Expected: exit code 0 and production assets emitted to `~/.wheelmaker/web`.

- [x] **Step 4: Inspect git diff**

Run:

```powershell
git diff -- app/__tests__/web-chat-ui.test.ts app/__tests__/web-responsive-ui-state.test.ts app/__tests__/web-port-relay-settings.test.ts app/web/src/services/workspaceUiState.ts app/web/src/services/workspacePersistence.ts app/web/src/main.tsx app/web/src/styles.css
```

Expected: only scoped composer, attachment, mobile Enter, floating side, and related test changes.

- [x] **Step 5: Commit implementation**

Run:

```powershell
git add app/__tests__/web-chat-ui.test.ts app/__tests__/web-responsive-ui-state.test.ts app/__tests__/web-port-relay-settings.test.ts app/web/src/services/workspaceUiState.ts app/web/src/services/workspacePersistence.ts app/web/src/main.tsx app/web/src/styles.css docs/superpowers/plans/2026-05-27-chat-composer-mobile-controls.md
git commit -m "feat: update chat composer mobile controls"
```

Expected: commit succeeds on `feature/chat-composer-mobile-controls`.
