# Chat Composer and Mobile Floating Controls Design

Date: 2026-05-27

## Context

The Workspace Web UI chat composer needs several layout and mobile interaction refinements:

- Keep the send button fixed at the bottom as the textarea grows.
- Move the prompt cancel control from the left of the input to the lower right, under send.
- Move the Skills control into the old cancel-control position.
- Add an image attachment button beside the existing file attachment button.
- Allow the mobile floating 3-tab, drawer, and relay controls to live on either screen side.
- Make mobile keyboard Enter send by default instead of inserting a newline.

The existing implementation is concentrated in `app/web/src/main.tsx` and `app/web/src/styles.css`. Composer state, attachment upload, slash command selection, and mobile floating-control state already exist and should be extended rather than replaced.

## Decisions

### Chat Composer Layout

The composer input row will become a three-part layout:

1. A left Skills button using the current slash-command menu behavior.
2. The textarea in the center.
3. A right-side vertical action column with Send above Cancel.

The Send button remains visually anchored to the bottom of the input row. When the textarea grows, the button column aligns to the lower edge instead of moving upward with the top of the textarea.

The Cancel button moves under Send. It uses the existing `cancelSelectedChatPrompt` behavior and keeps a stable footprint so the composer does not reflow when a prompt starts or stops. It is disabled or visually muted when no prompt is running.

The old bottom-toolbar Skills button is removed to avoid duplicate slash-command entry points.

### Attachment Buttons

The existing file attachment button remains in the toolbar.

A new image attachment button is added to the right of the file button. It uses a separate hidden file input with:

- `type="file"`
- `multiple`
- `accept="image/*"`

Selected images reuse the existing attachment pipeline: `chatFilesFromFileList`, `enqueueChatAttachmentFiles`, previews, upload, send blocks, retry, and removal. No separate image attachment model is introduced.

### Mobile Enter Behavior

On mobile layout, the chat textarea uses `enterKeyHint="send"` and Enter sends the message by default. External-keyboard newline behavior remains available through modified Enter keys such as Shift+Enter or Alt+Enter. IME composition must continue to be respected so composing text does not send early.

Desktop behavior remains platform-specific as currently implemented unless needed to preserve the mobile behavior cleanly.

### Mobile Floating Control Side

The mobile floating control stack gains a persisted side value: `left` or `right`.

Default side is `right` to preserve current behavior. Long-press drag continues to support vertical repositioning and now also tracks horizontal movement. On release, the stack snaps to the nearest side based on pointer position and persists that side with the existing global UI state persistence path.

The CSS should position menus and popovers relative to the selected side:

- Right side: current placement behavior.
- Left side: mirror horizontal placement so target menus open inward and stay inside the viewport.

Drawer width must continue reserving the floating-control lane on the side where controls live so the drawer and floating controls do not overlap.

## Testing

Add or update focused tests around existing string/structure test style:

- Composer structure includes the left Skills button, right send/cancel column, and no duplicate bottom Skills button.
- Image attachment input uses `accept="image/*"` and `multiple`, and calls the existing attachment enqueue path.
- Mobile Enter sends by default while modified Enter and IME composition do not trigger send.
- Floating-control state accepts and persists side, defaults to right, and resets only transient drag data on layout changes.
- Floating-control CSS supports both left and right placement including mirrored relay target menu placement.

## Out of Scope

- Rebuilding the composer as a separate React component.
- Introducing a separate image upload data model.
- Adding a settings screen toggle for floating-control side.
- Implementing file mention behavior beyond the existing placeholder.
