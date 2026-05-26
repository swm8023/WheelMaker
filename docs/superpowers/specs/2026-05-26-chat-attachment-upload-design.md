# Chat Attachment Upload Design

Date: 2026-05-26
Status: Approved for specification

## Background

The chat composer already supports image attachments by reading browser `File` objects into base64 and sending them directly through `session.send.blocks` as ACP `image` content blocks. That behavior works for small images, but it creates two problems for the next file-upload feature:

1. Images and files would follow different ingestion paths.
2. Large files would be forced through whole-file base64 payloads, increasing memory pressure and WebSocket payload size.

The new design keeps `session.send.blocks` as the send protocol, but moves all new attachments through a WheelMaker-owned chunk upload pipeline before they become ACP content blocks.

## Protocol Sources

- ACP content blocks define `image`, `resource`, and `resource_link`; `text` and `resource_link` are baseline prompt content, while `image` depends on `promptCapabilities.image`.
- Codex app-server `UserInput` in local `codex-cli 0.133.0` contains `text`, `image`, `localImage`, `skill`, and `mention`; it does not expose a generic file/blob input.
- WheelMaker registry remains the envelope and routing protocol. Agent-facing business content remains ACP.

## Goals

1. Support uploading arbitrary files from the chat composer.
2. Route images and non-image files through one attachment ingestion and upload pipeline.
3. Preserve `session.send.blocks` as the message send protocol.
4. Send uploaded images as ACP `image` blocks with `file://` URIs.
5. Send uploaded non-image files as ACP `resource_link` blocks with `file://` URIs.
6. Avoid new database tables for attachment metadata.
7. Keep upload progress, cancellation, failure, and retry visible in the composer.

## Non-goals

1. No ACP protocol extension such as a custom `file_upload` block.
2. No Codex app-server protocol extension.
3. No binary WebSocket or HTTP multipart upload in the first version.
4. No upload before a chat session exists.
5. No resumable upload after browser refresh.
6. No persistent recovery of unsent composer attachments after refresh.
7. No full migration of Feishu inbound images away from `image.data` in this iteration.

## Chosen Approach

Add a WheelMaker registry attachment upload API and keep chat sending block-based.

The attachment pipeline is WheelMaker internal:

```text
Browser File
  -> registry chunk upload
  -> session-scoped attachment artifact
  -> upload finish returns ACP content block
  -> session.send.blocks sends that block
  -> agent bridge maps ACP to provider input
```

This keeps the three protocol layers separate:

1. WheelMaker attachment upload stores and validates files.
2. ACP content blocks describe what the agent receives.
3. Codex app-server receives `localImage` or `mention` after bridge conversion.

## User Experience

### Attachment Entry Points

Attachments require an existing selected chat session. If no session is selected, the attachment controls should be disabled or show an error prompting the user to select or create a session.

Supported entry points:

1. Attach button with multi-file selection.
2. Drag and drop onto the composer area.
3. `Ctrl+V` opportunity paste when `ClipboardEvent.clipboardData.items` exposes file/image items.

If a paste event contains both text and files/images, the composer should accept the attachment items and not insert the text payload.

### Composer State

The user may continue editing composer text while attachments upload. The send button is disabled until every queued attachment is completed or removed.

Attachment tile states:

| State | UI behavior |
|---|---|
| queued/uploading | show progress and cancel action |
| failed | show failure state and retry/remove actions |
| completed unsent | show delete action |
| sent | cleared from composer after successful send |

Image previews use `URL.createObjectURL(file)` from the current page's `File` object. The browser UI must not rely on server `file://` URIs for preview because remote browsers and normal web security cannot load arbitrary server-local paths.

## Upload Protocol

All methods are registry methods routed to the hub owning the project.

### `session.attachment.start`

Request:

```json
{
  "sessionId": "sess-1",
  "name": "report.pdf",
  "mimeType": "application/pdf",
  "size": 123456
}
```

Response:

```json
{
  "ok": true,
  "sessionId": "sess-1",
  "uploadId": "upl_...",
  "attachmentId": "att_...",
  "chunkSize": 1048576,
  "received": 0
}
```

Rules:

1. `sessionId` is required and must refer to an existing active session in the project.
2. `size` must be greater than 0 and no larger than 50 MiB.
3. `name` is kept for UI/protocol display but must not be used as the final filesystem path.
4. The server creates an in-memory upload record and a `.part` file under the session attachment directory.

### `session.attachment.chunk`

Request:

```json
{
  "sessionId": "sess-1",
  "uploadId": "upl_...",
  "offset": 0,
  "data": "<base64 chunk>"
}
```

Response:

```json
{
  "ok": true,
  "sessionId": "sess-1",
  "uploadId": "upl_...",
  "received": 1048576
}
```

Rules:

1. Chunk size is 1 MiB of raw bytes except for the final chunk.
2. `data` is base64 for that chunk only, not the whole file.
3. Chunks must be sent sequentially.
4. `offset` must equal the server upload record's current received byte count.
5. A failed network request may retry the same chunk.
6. Offset mismatch, expired upload, or missing upload record requires retrying the whole file.
7. Each valid chunk refreshes `lastActivityAt`.

### `session.attachment.finish`

Request:

```json
{
  "sessionId": "sess-1",
  "uploadId": "upl_...",
  "sha256": "<hex sha256 of raw file bytes>"
}
```

Response for an image:

```json
{
  "ok": true,
  "attachment": {
    "attachmentId": "att_...",
    "name": "diagram.png",
    "mimeType": "image/png",
    "size": 123456,
    "sha256": "<hex sha256>",
    "kind": "image",
    "status": "completed"
  },
  "block": {
    "type": "image",
    "uri": "file:///.../attachments/sha256-....png",
    "mimeType": "image/png"
  }
}
```

Response for a non-image file:

```json
{
  "ok": true,
  "attachment": {
    "attachmentId": "att_...",
    "name": "report.pdf",
    "mimeType": "application/pdf",
    "size": 123456,
    "sha256": "<hex sha256>",
    "kind": "file",
    "status": "completed"
  },
  "block": {
    "type": "resource_link",
    "uri": "file:///.../attachments/sha256-....pdf",
    "name": "report.pdf",
    "mimeType": "application/pdf",
    "size": 123456
  }
}
```

Rules:

1. The server verifies received byte count and whole-file SHA-256.
2. The server atomically renames the `.part` file to `sha256-<hash><ext>`.
3. The server writes sidecar metadata JSON next to the final file.
4. `attachment.kind` is `image` when the final MIME type starts with `image/`; otherwise it is `file`.
5. `block.uri` is a real `file://` URI to the server artifact.

### `session.attachment.cancel`

Request:

```json
{
  "sessionId": "sess-1",
  "uploadId": "upl_..."
}
```

Response:

```json
{ "ok": true }
```

Rules:

1. Cancelling an upload removes its in-memory upload record and partial `.part` file.
2. The composer shows cancel while uploading, not delete.

### `session.attachment.delete`

Request:

```json
{
  "sessionId": "sess-1",
  "attachmentId": "att_..."
}
```

Response:

```json
{ "ok": true }
```

Rules:

1. Delete applies to completed but unsent attachments removed from the composer.
2. Delete removes the artifact file and sidecar if the sidecar is not marked sent.
3. Delete failure should not block the UI; cleanup is eventually handled by session cleanup.

## Upload Lifecycle

Uploads are not persisted in a database. In-progress upload records are in memory.

If an upload is not finished and receives no `chunk`, `finish`, or `cancel` message for 3 minutes, the server treats it as failed and removes the partial file.

State transitions:

```text
starting/uploading
  -> chunk received: update received bytes and lastActivityAt
  -> finish success: completed attachment
  -> cancel: cancelled and partial file removed
  -> 3 minute idle timeout: failed/expired and partial file removed
```

The frontend supports retry after failure only while the current page still has the original `File` object. Refreshing the page loses that object and cannot resume the upload.

## Storage

Attachment artifacts live under the existing session resource tree:

```text
~/.wheelmaker/db/session/<project>/<session>/attachments/
```

Final file naming:

```text
sha256-<hex><ext>
```

Rules:

1. Disk filenames use content hash, not user-provided names.
2. UI and ACP `name` preserve the original file name.
3. Extension is chosen from the original file's final safe extension first, then MIME type fallback, then empty extension.
4. Files are first written as `.part`; `finish` moves them to the final hash name.
5. Sidecar JSON is required for completed attachments.

Sidecar shape:

```json
{
  "attachmentId": "att_...",
  "name": "report.pdf",
  "mimeType": "application/pdf",
  "size": 123456,
  "sha256": "<hex sha256>",
  "kind": "file",
  "createdAt": "2026-05-26T12:00:00Z",
  "sent": false
}
```

No new SQLite table is introduced. Normal prompt/session recording remains the database-backed history source.

## Send Protocol

`session.send` remains block-based:

```json
{
  "sessionId": "sess-1",
  "text": "Look at these files",
  "blocks": [
    { "type": "text", "text": "Look at these files" },
    { "type": "image", "uri": "file:///.../attachments/sha256-....png", "mimeType": "image/png" },
    {
      "type": "resource_link",
      "uri": "file:///.../attachments/sha256-....pdf",
      "name": "report.pdf",
      "mimeType": "application/pdf",
      "size": 123456
    }
  ]
}
```

The frontend should send the `block` returned by `session.attachment.finish`; it should not construct `file://` URIs itself.

Server validation in `session.send`:

1. `image.uri` and `resource_link.uri` with `file://` must resolve under the current session's `attachments/` directory.
2. The artifact file must exist.
3. A matching sidecar must exist.
4. Sidecar metadata must match the content block enough to prevent path or name spoofing.
5. After successful prompt submission, sidecar `sent` may be set to `true` so later delete calls cannot remove sent history assets.

Legacy `image.data` is kept compatible for existing callers, but the new Web attachment UI must not generate it.

## ACP and Codex Mapping

The server sends ACP blocks to the agent layer. The Codex bridge maps those to app-server `UserInput`:

```text
ACP image(uri=file://...)
  -> Codex app-server localImage { path }

ACP resource_link(uri=file://...)
  -> Codex app-server mention { name, path }
```

Images keep visual input semantics. Non-image files keep resource-reference semantics.

## Frontend Design

The composer attachment model should keep upload state separate from send blocks:

```ts
type ChatAttachment = {
  localId: string;
  uploadId?: string;
  attachmentId?: string;
  name: string;
  mimeType: string;
  size: number;
  kind: 'image' | 'file';
  status: 'queued' | 'uploading' | 'failed' | 'completed';
  progress: number;
  objectUrl?: string;
  block?: RegistryChatContentBlock;
  file?: File;
  error?: string;
};
```

Rules:

1. Upload files serially.
2. Upload chunks sequentially within each file.
3. Keep text editable while uploading.
4. Disable send while any attachment is queued, uploading, or failed.
5. Send uses completed attachment `block` values plus an optional text block.
6. Remove completed unsent attachments via `session.attachment.delete`.
7. Cancel uploading attachments via `session.attachment.cancel`.
8. Revoke image `objectUrl` values when attachments are removed or composer state is reset.

## Server Design

Implementation should stay near the existing hub session request handling:

1. Add registry method names to the registry allowlist/forwarding path.
2. Add hub handlers for `session.attachment.*`.
3. Add an in-memory upload manager keyed by session and upload ID.
4. Reuse the existing session artifact cleanup path, but rename the Codex image artifact directory from `images` to the unified `attachments` directory.
5. Add validation helpers for safe path segments, file URI parsing, attachment root containment, sidecar read/write, and MIME/extension normalization.
6. Extend `session.send` block validation for completed attachment artifacts while keeping legacy `image.data` compatibility.

The upload manager must not trust frontend MIME or filename values for filesystem safety. It may use them for display and fallback classification after independent checks.

## Testing Strategy

Server tests:

1. `session.attachment.start` rejects missing session and files over 50 MiB.
2. Chunk upload writes sequential bytes and rejects offset mismatch.
3. Finish validates SHA-256 and atomically creates final artifact plus sidecar.
4. Finish returns `image` block for image MIME and `resource_link` block for other files.
5. Idle upload cleanup removes `.part` after 3 minutes without activity.
6. Cancel removes partial upload.
7. Delete removes completed unsent artifact and sidecar.
8. `session.send` accepts uploaded attachment `file://` blocks and rejects paths outside the session attachment root.
9. Legacy `image.data` remains accepted.

Frontend tests:

1. File picker, drag/drop, and paste route into the same attachment queue.
2. Upload progress disables send but leaves text editing enabled.
3. Successful finish stores returned `block` and attachment view.
4. Failed upload can retry with the current `File` object.
5. Uploading attachment cancel calls `session.attachment.cancel`.
6. Completed unsent attachment delete calls `session.attachment.delete`.
7. Send emits text block plus returned attachment blocks in queue order.
8. New UI no longer emits `image.data`.

Validation commands after implementation:

```bash
cd server && go test ./...
cd app && npm test -- --runInBand
cd app && npm run tsc:web
```

## Risks and Mitigations

### Registry Payload Size

Risk: 1 MiB chunks become about 1.33 MiB JSON payloads after base64.

Mitigation: keep chunks sequential and bounded; avoid whole-file base64; allow chunk size to be reduced later if remote registry deployments show pressure.

### Stale Partial Files

Risk: browser close or network interruption leaves `.part` files.

Mitigation: upload manager expires unfinished uploads after 3 minutes of inactivity and removes partial files.

### Path Spoofing

Risk: a client sends a forged `file://` URI in `session.send.blocks`.

Mitigation: server validates containment under the current session attachment root and requires matching sidecar metadata.

### Lost Unsent Attachments on Refresh

Risk: refresh loses in-memory `File` objects and object URL previews.

Mitigation: first version intentionally does not restore unsent attachments; users can reselect files. Sent artifacts remain available through session history.

### Protocol Confusion

Risk: WheelMaker attachment metadata, ACP content blocks, and Codex app-server inputs get conflated.

Mitigation: keep upload APIs WheelMaker-owned, keep `session.send.blocks` ACP-shaped, and keep provider-specific conversion in the Codex bridge.

## Open Decisions

No open decisions remain for the first implementation plan.
