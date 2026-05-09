# Chat Composer Clipboard Image Paste Design

Date: 2026-05-09
Status: Approved for implementation

## Background

The desktop web chat composer already supports attaching a single image through the hidden file input and sending it as an `image` content block. That path is working, but two UX gaps remain:

1. Desktop users cannot paste clipboard images with `Ctrl+V` directly into the chat composer.
2. The composer only supports one pending image at a time, so repeated file picks or clipboard pastes overwrite the previous attachment instead of building a queue.

The user wants desktop clipboard paste to behave the same as the current attachment flow: pasted images should appear in the preview area first and be sent only when the user explicitly presses send. They also want multiple pending images, with per-image removal.

## Goals

1. Support `Ctrl+V` clipboard image paste in the desktop chat composer.
2. Expand the existing single-image attachment flow into a multi-image attachment queue.
3. Keep file picker and clipboard paste on the same attachment pipeline and preview UI.
4. Preserve the current send protocol by reusing existing `blocks` payload semantics.
5. Support removing individual pending images before send.

## Non-goals

1. No server protocol changes.
2. No mobile-specific clipboard UX work.
3. No auto-send on paste.
4. No bulk “clear all attachments” action in this iteration.
5. No redesign of the wider chat layout beyond what is needed for multi-image preview.

## Current State

The current web implementation in `app/web/src/main.tsx` stores one pending attachment:

- `type ChatAttachment = { name, mimeType, data }`
- `type ChatComposerDraft = { text, attachment }`
- `chatAttachment` / `chatAttachmentRef`
- `updateChatComposerAttachment(nextAttachment)`
- `handleChatFileChange()` reads the first selected file and replaces the existing attachment
- `sendChatMessage()` appends at most one `image` block
- the composer renders a single preview card with one remove button

This means the file picker, draft storage, preview rendering, and send path are all currently shaped around a single attachment.

## Chosen Approach

Extend the existing attachment flow into an attachment queue and route clipboard paste through the same queue builder.

This is preferred over adding a paste-only path or wrapping the existing single-image state with a second multi-image layer because:

1. file picker and clipboard paste stay behaviorally identical
2. draft persistence remains coherent
3. preview and send logic only need one attachment model
4. future changes stay localized to one clear path

## UX Design

### 1. Attachment semantics

The chat composer owns a single ordered attachment queue:

- file selection appends one or more images to the queue
- clipboard paste appends one or more images to the queue
- sending transmits the current text plus all queued images together
- successful send clears the queue

Existing behavior remains unchanged for text-only input.

### 2. Paste behavior

Desktop paste support is only added on the chat composer textarea.

Rules:

1. If the clipboard contains one or more `image/*` items, intercept paste and append those images to the attachment queue.
2. If the clipboard contains no image items, do not intercept paste; preserve normal text paste behavior.
3. If the clipboard contains both text and image items, treat it as an image paste event and do not auto-insert the text payload.

This keeps image paste stable and avoids mixed browser-default text insertion behavior during image handling.

### 3. Preview behavior

The single preview card becomes a multi-image preview strip/grid:

- one preview tile per pending image
- thumbnail preview for each tile
- attachment name shown per tile
- each tile gets its own remove button

No global “clear all” action is added in this iteration.

### 4. New-session flow

The existing “start composing before session exists” path remains intact.

If the user pastes images before a chat session is selected:

- the images remain in the pending queue
- pressing send still goes through `beginNewChatFlow(...)`
- once the user picks an agent, the new session is created and the queued blocks are sent using the same payload structure as a normal existing session send

## Frontend Design

### 1. Data model changes

Replace the single attachment model with an array-backed queue.

Recommended shapes:

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
```

Notes:

- `id` is needed for stable React rendering and per-image removal.
- Draft persistence must store the attachment array rather than a nullable singleton.
- `EMPTY_CHAT_COMPOSER_DRAFT` should carry `attachments: []`.

### 2. Shared attachment ingestion

Introduce a single helper path that converts incoming files or clipboard items into `ChatAttachment[]` and appends them to the queue.

Responsibilities:

1. accept incoming image sources from file picker or clipboard
2. read each image into a data URL / base64 payload
3. normalize name and mime type
4. append in source order
5. update draft state

Clipboard images without a filename should receive a generated default name such as `pasted-image-1.png`.

### 3. File picker behavior

The hidden file input should change from single-file semantics to multi-file semantics:

- add `multiple`
- iterate all selected image files
- append them instead of replacing the current queue

The input element should still be reset after send or when the queue becomes empty so the same file can be selected again later.

### 4. Clipboard behavior

The textarea gains an `onPaste` handler that:

1. inspects `event.clipboardData.items`
2. filters for `image/*`
3. if no image items exist, returns without interfering
4. otherwise calls `preventDefault()`, converts the images, and appends them

This behavior should only exist in the desktop web chat composer and should not affect unrelated inputs.

### 5. Send path

`sendChatMessage()` should build `blocks` as:

1. text block first if non-empty
2. one `image` block per attachment, in queue order

The composer title/preview fallback for new or just-created sessions should use:

- trimmed text when present
- otherwise the first attachment name
- otherwise the session id fallback

### 6. Removal behavior

Attachment removal should be per-image:

- each preview tile removes only its own attachment id
- queue order of remaining items is preserved
- draft storage updates immediately after removal

## Error Handling

Attachment ingestion should be best-effort:

1. If one pasted/selected image fails to read, show the existing error surface.
2. Successfully read images from the same batch should still remain queued.
3. A read failure must not clear already queued attachments.

This avoids throwing away good images because one item in the batch was bad.

## Testing Strategy

Reuse the existing source-inspection style tests already used for chat UI regressions.

Minimum coverage:

1. `chatAttachments` array state exists instead of single `chatAttachment`
2. `ChatComposerDraft` stores `attachments`
3. file input includes `multiple`
4. textarea includes an `onPaste` handler
5. send path maps all attachments into blocks
6. per-image removal function exists
7. draft reset clears the attachment array

Validation after implementation:

- `cd app && npm test -- --runInBand`
- `cd app && npm run tsc:web`

## Implementation Scope

Primary files:

- `app/web/src/main.tsx`
- `app/web/src/styles.css`
- `app/__tests__/web-chat-ui.test.ts`

No server file changes are expected because the existing `session.send` blocks payload already supports multiple image blocks.

## Risks and Mitigations

### 1. Draft/state drift

Risk: composer text and attachment queue can fall out of sync across refs, state, and restored drafts.

Mitigation:

- keep one queue model only
- update draft persistence from the same attachment helpers
- update refs immediately from state effects, matching the current composer pattern

### 2. Duplicate logic between file picker and paste

Risk: clipboard and file attachments diverge over time.

Mitigation:

- both entry points must share one ingestion helper

### 3. Layout crowding with many images

Risk: multiple previews can visually overwhelm the composer.

Mitigation:

- use a compact preview strip/grid
- keep per-tile metadata minimal
- avoid introducing extra controls beyond per-image remove
