# Chat Paste Images Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add desktop `Ctrl+V` clipboard image paste to the chat composer and expand the existing single-image attachment flow into a multi-image queue with per-image removal.

**Architecture:** Keep the change localized to the existing web chat composer in `app/web/src/main.tsx` by replacing the singleton attachment state with an attachment array and routing both file-picker and clipboard images through one shared ingestion helper. Preserve the existing server contract by continuing to send `blocks` payloads, but emit one image block per queued attachment and update the preview UI to render per-image tiles.

**Tech Stack:** React, TypeScript, existing Jest source-inspection tests, CSS

---

## File map

- Modify: `D:\Code\WheelMaker\app\web\src\main.tsx` — convert attachment state/drafts/send path to array semantics; add file multi-select and clipboard paste ingestion
- Modify: `D:\Code\WheelMaker\app\web\src\styles.css` — adapt the composer preview area from one attachment card to a compact multi-image layout
- Modify: `D:\Code\WheelMaker\app\__tests__\web-chat-ui.test.ts` — lock multi-image attachment and paste support expectations

## Task 1: Lock multi-image composer expectations

**Files:**
- Modify: `D:\Code\WheelMaker\app\__tests__\web-chat-ui.test.ts`
- Test: `D:\Code\WheelMaker\app\__tests__\web-chat-ui.test.ts`

- [ ] **Step 1: Write the failing test**

```ts
expect(mainTsx).toContain('const [chatAttachments, setChatAttachments] = useState<ChatAttachment[]>([]);');
expect(mainTsx).toContain('attachments: ChatAttachment[];');
expect(mainTsx).toContain('multiple');
expect(mainTsx).toContain('onPaste={event => {');
expect(mainTsx).toContain("item.type.toLowerCase().startsWith('image/')");
expect(mainTsx).toContain('blocks.push(...chatAttachments.map(attachment => ({');
expect(mainTsx).toContain('const removeChatAttachment = (attachmentId: string) => {');
expect(mainTsx).toContain('chatAttachments.map(attachment => (');
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd app && npm test -- --runInBand __tests__/web-chat-ui.test.ts`
Expected: FAIL because the code still contains `chatAttachment`, lacks `multiple`, and has no paste handler or array send path.

- [ ] **Step 3: Commit the red test**

```bash
git add app/__tests__/web-chat-ui.test.ts
git commit -m "test: lock multi-image chat composer expectations"
```

## Task 2: Convert composer attachment state and drafts to arrays

**Files:**
- Modify: `D:\Code\WheelMaker\app\web\src\main.tsx`
- Test: `D:\Code\WheelMaker\app\__tests__\web-chat-ui.test.ts`

- [ ] **Step 1: Replace singleton attachment types/state with array types**

```ts
type ChatAttachment = {
  id: string;
  name: string;
  mimeType: string;
  data: string;
};

type ChatComposerDraft = {
  text: string;
  attachments: ChatAttachment[];
};

const EMPTY_CHAT_COMPOSER_DRAFT: ChatComposerDraft = { text: '', attachments: [] };
const [chatAttachments, setChatAttachments] = useState<ChatAttachment[]>([]);
const chatAttachmentsRef = useRef<ChatAttachment[]>([]);
```

- [ ] **Step 2: Update draft save/restore/reset helpers to use the array**

```ts
const saveChatComposerDraft = useCallback(
  (draftKey: string, text: string, attachments: ChatAttachment[]) => {
    const hasContent = text.length > 0 || attachments.length > 0;
    // store attachments instead of attachment
  },
  [],
);

const resetChatComposer = () => {
  setChatComposerText('');
  setChatAttachments([]);
  saveChatComposerDraft(currentChatDraftKeyRef.current, '', []);
};
```

- [ ] **Step 3: Add append/remove helpers with stable ids**

```ts
const appendChatAttachments = useCallback(
  (nextAttachments: ChatAttachment[]) => {
    if (nextAttachments.length === 0) return;
    setChatAttachments(prev => {
      const merged = [...prev, ...nextAttachments];
      saveChatComposerDraft(currentChatDraftKeyRef.current, chatComposerTextRef.current, merged);
      return merged;
    });
  },
  [saveChatComposerDraft],
);

const removeChatAttachment = (attachmentId: string) => {
  setChatAttachments(prev => {
    const filtered = prev.filter(item => item.id !== attachmentId);
    saveChatComposerDraft(currentChatDraftKeyRef.current, chatComposerTextRef.current, filtered);
    return filtered;
  });
};
```

- [ ] **Step 4: Run the focused test to verify the state refactor passes the new assertions**

Run: `cd app && npm test -- --runInBand __tests__/web-chat-ui.test.ts`
Expected: PASS for the newly added array-state assertions, or fail only on the still-unimplemented ingestion/preview details from later tasks.

- [ ] **Step 5: Commit the state refactor**

```bash
git add app/web/src/main.tsx app/__tests__/web-chat-ui.test.ts
git commit -m "feat: queue chat image attachments"
```

## Task 3: Unify file-picker and clipboard image ingestion

**Files:**
- Modify: `D:\Code\WheelMaker\app\web\src\main.tsx`
- Test: `D:\Code\WheelMaker\app\__tests__\web-chat-ui.test.ts`

- [ ] **Step 1: Add a shared helper that converts files/clipboard items into attachment objects**

```ts
const readChatAttachmentFile = async (file: File, fallbackName: string): Promise<ChatAttachment> => {
  const dataUrl = await new Promise<string>((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(typeof reader.result === 'string' ? reader.result : '');
    reader.onerror = () => reject(reader.error ?? new Error('Failed to read image file'));
    reader.readAsDataURL(file);
  });
  const base64 = dataUrl.includes(',') ? dataUrl.slice(dataUrl.indexOf(',') + 1) : dataUrl;
  return {
    id: `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`,
    name: file.name || fallbackName,
    mimeType: file.type || 'image/png',
    data: base64,
  };
};
```

- [ ] **Step 2: Update file input handling to support multiple files and append**

```ts
const handleChatFileChange = async (event: React.ChangeEvent<HTMLInputElement>) => {
  const files = Array.from(event.target.files ?? []);
  if (files.length === 0) return;
  const attachments = await Promise.all(
    files.map((file, index) => readChatAttachmentFile(file, `selected-image-${index + 1}.png`)),
  );
  appendChatAttachments(attachments);
};
```

- [ ] **Step 3: Add textarea paste handling for clipboard images**

```ts
onPaste={event => {
  const items = Array.from(event.clipboardData?.items ?? []);
  const imageItems = items.filter(item => item.type.toLowerCase().startsWith('image/'));
  if (imageItems.length === 0) {
    return;
  }
  event.preventDefault();
  Promise.all(
    imageItems.map((item, index) => {
      const file = item.getAsFile();
      if (!file) {
        throw new Error('Clipboard image is unavailable');
      }
      return readChatAttachmentFile(file, `pasted-image-${index + 1}.png`);
    }),
  )
    .then(appendChatAttachments)
    .catch(err => setError(err instanceof Error ? err.message : String(err)));
}}
```

- [ ] **Step 4: Run the focused test to verify ingestion expectations**

Run: `cd app && npm test -- --runInBand __tests__/web-chat-ui.test.ts`
Expected: PASS for `multiple`, `onPaste`, image filtering, and append helper assertions.

- [ ] **Step 5: Commit the ingestion work**

```bash
git add app/web/src/main.tsx app/__tests__/web-chat-ui.test.ts
git commit -m "feat: support pasted chat images"
```

## Task 4: Update preview rendering, send path, and composer styling

**Files:**
- Modify: `D:\Code\WheelMaker\app\web\src\main.tsx`
- Modify: `D:\Code\WheelMaker\app\web\src\styles.css`
- Test: `D:\Code\WheelMaker\app\__tests__\web-chat-ui.test.ts`

- [ ] **Step 1: Build one image block per queued attachment in send/new-session flows**

```ts
const blocks: RegistryChatContentBlock[] = [];
if (trimmedText) {
  blocks.push({ type: 'text', text: trimmedText });
}
blocks.push(
  ...chatAttachments.map(attachment => ({
    type: 'image',
    mimeType: attachment.mimeType,
    data: attachment.data,
  })),
);
```

- [ ] **Step 2: Render a multi-image preview with per-tile remove buttons**

```tsx
{chatAttachments.length > 0 ? (
  <div className="chat-attachment-preview-list">
    {chatAttachments.map(attachment => (
      <div key={attachment.id} className="chat-attachment-preview">
        <img
          className="chat-attachment-thumb"
          src={`data:${attachment.mimeType || 'image/png'};base64,${attachment.data}`}
          alt={attachment.name || 'attachment preview'}
        />
        <div className="chat-attachment-name">{attachment.name}</div>
        <button
          type="button"
          className="chat-attachment-remove"
          onClick={() => removeChatAttachment(attachment.id)}
        />
      </div>
    ))}
  </div>
) : null}
```

- [ ] **Step 3: Add compact preview layout CSS**

```css
.chat-attachment-preview-list {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.chat-attachment-preview {
  position: relative;
  width: 88px;
}

.chat-attachment-thumb {
  width: 88px;
  height: 88px;
  object-fit: cover;
}
```

- [ ] **Step 4: Run focused tests, then the full app verification**

Run:
1. `cd app && npm test -- --runInBand __tests__/web-chat-ui.test.ts`
2. `cd app && npm test -- --runInBand`
3. `cd app && npm run tsc:web`

Expected:
1. focused chat UI test passes
2. full Jest suite passes
3. TypeScript check passes

- [ ] **Step 5: Commit the preview/send integration**

```bash
git add app/web/src/main.tsx app/web/src/styles.css app/__tests__/web-chat-ui.test.ts
git commit -m "feat: support multi-image chat previews"
```

## Task 5: Final review and ship

**Files:**
- Verify: `D:\Code\WheelMaker\docs\superpowers\specs\2026-05-09-chat-paste-images-design.md`
- Verify: `D:\Code\WheelMaker\docs\superpowers\plans\2026-05-09-chat-paste-images.md`
- Verify: touched app files

- [ ] **Step 1: Check spec coverage against implementation**

Confirm:
- clipboard images paste into the composer
- queue supports multiple pending images
- file picker and paste use one attachment path
- per-image removal works
- send path emits one image block per queued image

- [ ] **Step 2: Run fresh verification commands**

Run:
1. `cd app && npm test -- --runInBand`
2. `cd app && npm run tsc:web`

Expected:
1. all Jest suites pass
2. TypeScript check passes

- [ ] **Step 3: Ship through the repo completion gate**

Run:
1. `git add -A`
2. `git commit -m "feat: support pasted chat images"`
3. `git push origin main`
4. `cd app && npm run build:web:release`

Expected:
1. commit succeeds with intended files only
2. push to `main` succeeds
3. web release exports to `~/.wheelmaker/web`
