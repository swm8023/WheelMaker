# Chat Attachment Upload Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add session-scoped chunked chat attachments for arbitrary files while keeping `session.send.blocks` as the send protocol.

**Architecture:** Registry clients upload files through `session.attachment.*` RPCs to the hub. The hub writes session-scoped attachment artifacts and sidecars, returns ACP-shaped content blocks, and validates those blocks again during `session.send`. The Web composer replaces base64 image attachments with upload-state attachments that store returned blocks.

**Tech Stack:** Go hub/registry server, React/TypeScript Web UI, Jest source-inspection tests, Go unit tests.

---

## File Map

- Modify: `server/internal/registry/server.go` - allow forwarding `session.attachment.*` methods.
- Modify: `server/internal/hub/reporter.go` - allow hub-side handling for `session.attachment.*` methods when proxied through registry.
- Modify: `server/internal/hub/client/client.go` - route upload RPCs, validate attachment blocks in `session.send`, and hold an upload manager.
- Create: `server/internal/hub/client/session_attachments.go` - upload manager, sidecar storage, file URI validation, content block generation.
- Modify: `server/internal/hub/client/client_test.go` - server-side upload and send validation tests.
- Modify: `server/internal/hub/agent/codexapp_convert.go` - rename Codex image artifact directory from `images` to unified `attachments`.
- Modify: `server/internal/hub/agent/agent_test.go` - update artifact path expectation.
- Modify: `app/web/src/types/registry.ts` - extend chat content blocks with `uri`, `name`, and `size`.
- Modify: `app/web/src/services/registryRepository.ts` - add attachment upload RPC client methods and widen `sendSessionMessage` block typing.
- Modify: `app/web/src/services/registryWorkspaceService.ts` - expose attachment upload methods to `main.tsx`.
- Modify: `app/web/src/main.tsx` - replace base64 attachment reads with upload queue, drag/drop/paste support, and returned blocks in `session.send`.
- Modify: `app/web/src/styles.css` - add progress/failed/completed attachment tile states and composer drag-over state.
- Modify: `app/__tests__/web-chat-ui.test.ts` - lock new upload queue behavior and ensure new UI no longer emits `image.data`.
- Create or modify focused app tests if existing source-inspection tests become too broad.

## Task 1: Lock Server Upload Protocol Behavior

**Files:**
- Modify: `server/internal/hub/client/client_test.go`
- Test: `server/internal/hub/client/client_test.go`

- [ ] **Step 1: Write failing tests**

Add tests near existing `session.send` tests:

```go
func TestSessionAttachmentUploadCompletesResourceLinkBlock(t *testing.T) {
	ctx := context.Background()
	c := NewClient(testConfigWithCodexAgent(t))
	sess := newTestSessionWithID(t, c, "proj1", "sess-attach", "codex")
	c.sessions["proj1:sess-attach"] = sess

	startResp, err := c.HandleSessionRequest(ctx, "session.attachment.start", "proj1", json.RawMessage(`{"sessionId":"sess-attach","name":"report.pdf","mimeType":"application/pdf","size":11}`))
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	uploadID := responseString(t, startResp, "uploadId")
	chunkPayload := fmt.Sprintf(`{"sessionId":"sess-attach","uploadId":%q,"offset":0,"data":"aGVsbG8gd29ybGQ="}`, uploadID)
	if _, err := c.HandleSessionRequest(ctx, "session.attachment.chunk", "proj1", json.RawMessage(chunkPayload)); err != nil {
		t.Fatalf("chunk: %v", err)
	}
	finishPayload := fmt.Sprintf(`{"sessionId":"sess-attach","uploadId":%q,"sha256":"b94d27b9934d3e08a52e52d7da7dabfadeb0380f71545ee8b7c5d8227cb04c8b"}`, uploadID)
	finishResp, err := c.HandleSessionRequest(ctx, "session.attachment.finish", "proj1", json.RawMessage(finishPayload))
	if err != nil {
		t.Fatalf("finish: %v", err)
	}
	block := responseMap(t, finishResp, "block")
	if block["type"] != "resource_link" || block["name"] != "report.pdf" || block["mimeType"] != "application/pdf" {
		t.Fatalf("block=%#v, want resource_link report.pdf", block)
	}
	if uri, _ := block["uri"].(string); !strings.HasPrefix(uri, "file://") || !strings.Contains(uri, "/attachments/sha256-") {
		t.Fatalf("uri=%q, want file attachment uri", uri)
	}
}
```

Add companion tests for image block generation, offset mismatch, SHA mismatch, idle expiry, cancel, delete, and `session.send` path containment.

- [ ] **Step 2: Run focused tests to verify RED**

Run:

```bash
cd server && go test ./internal/hub/client -run "SessionAttachment|AttachmentBlock" -count=1
```

Expected: FAIL because `session.attachment.*` methods are unsupported.

## Task 2: Implement Server Attachment Upload Manager

**Files:**
- Create: `server/internal/hub/client/session_attachments.go`
- Modify: `server/internal/hub/client/client.go`
- Modify: `server/internal/hub/client/client_test.go`

- [ ] **Step 1: Add upload manager and request/response types**

Implement:

```go
const (
	attachmentChunkSize = 1024 * 1024
	attachmentMaxBytes  = 50 * 1024 * 1024
	attachmentIdleTTL   = 3 * time.Minute
)
```

Create an `attachmentManager` with mutex-protected `uploads map[string]*attachmentUpload`, and helpers for `start`, `appendChunk`, `finish`, `cancel`, and `deleteAttachment`.

- [ ] **Step 2: Store files and sidecars under session attachments root**

Use:

```text
~/.wheelmaker/db/session/<project>/<session>/attachments/
```

Final file path is `sha256-<hex><ext>`. Use `.part` before finish and write a JSON sidecar with attachment metadata.

- [ ] **Step 3: Route methods from `HandleSessionRequest`**

Add cases:

```go
case "session.attachment.start":
	return c.handleSessionAttachmentStart(ctx, projectName, payload)
case "session.attachment.chunk":
	return c.handleSessionAttachmentChunk(ctx, projectName, payload)
case "session.attachment.finish":
	return c.handleSessionAttachmentFinish(ctx, projectName, payload)
case "session.attachment.cancel":
	return c.handleSessionAttachmentCancel(ctx, projectName, payload)
case "session.attachment.delete":
	return c.handleSessionAttachmentDelete(ctx, projectName, payload)
```

- [ ] **Step 4: Run focused server tests to verify GREEN**

Run:

```bash
cd server && go test ./internal/hub/client -run "SessionAttachment|AttachmentBlock" -count=1
```

Expected: PASS.

## Task 3: Validate Send Blocks and Registry Forwarding

**Files:**
- Modify: `server/internal/registry/server.go`
- Modify: `server/internal/hub/reporter.go`
- Modify: `server/internal/hub/client/client.go`
- Modify: `server/internal/registry/server_test.go`
- Modify: `server/internal/hub/reporter_test.go`
- Modify: `server/internal/hub/client/client_test.go`

- [ ] **Step 1: Add failing tests for forwarding**

Extend registry and reporter tests so `session.attachment.start`, `session.attachment.chunk`, `session.attachment.finish`, `session.attachment.cancel`, and `session.attachment.delete` are accepted and forwarded like `session.send`.

- [ ] **Step 2: Add failing tests for send validation**

Assert `session.send` accepts uploaded `file://` blocks and rejects:

```json
{"type":"resource_link","uri":"file:///C:/outside/report.pdf","name":"report.pdf"}
```

Expected: rejection with an attachment containment/sidecar error.

- [ ] **Step 3: Implement forwarding allowlists and validation**

Add `session.attachment.*` methods to registry and reporter allowlists. In `session.send`, validate `image.uri` and `resource_link.uri` blocks before calling `PromptToSession`. Keep legacy `image.data` accepted.

- [ ] **Step 4: Run focused tests**

Run:

```bash
cd server && go test ./internal/registry ./internal/hub -run "Attachment|session.attachment|SessionSend" -count=1
```

Expected: PASS.

## Task 4: Update Codex Attachment Artifact Directory

**Files:**
- Modify: `server/internal/hub/agent/codexapp_convert.go`
- Modify: `server/internal/hub/agent/agent_test.go`

- [ ] **Step 1: Write/update failing expectation**

Change the Codex image artifact test to expect `/attachments/sha256-` instead of `/images/sha256-`.

- [ ] **Step 2: Run focused test to verify RED**

Run:

```bash
cd server && go test ./internal/hub/agent -run "CodexAppSessionPromptSendsBase64ImageAsLocalImage|CleanupSessionArtifacts" -count=1
```

Expected: FAIL while code still writes to `images`.

- [ ] **Step 3: Rename artifact directory**

Change `codexappImageArtifactDir` to return `attachments` as the terminal path segment.

- [ ] **Step 4: Run focused test to verify GREEN**

Run:

```bash
cd server && go test ./internal/hub/agent -run "CodexAppSessionPromptSendsBase64ImageAsLocalImage|CleanupSessionArtifacts" -count=1
```

Expected: PASS.

## Task 5: Lock Web Upload Queue Behavior

**Files:**
- Modify: `app/__tests__/web-chat-ui.test.ts`
- Modify: `app/__tests__/web-chat-project-service.test.ts`
- Test: `app/__tests__/web-chat-ui.test.ts`

- [ ] **Step 1: Add failing source-inspection assertions**

Assert `main.tsx` contains:

```ts
status: 'queued' | 'uploading' | 'failed' | 'completed';
progress: number;
block?: RegistryChatContentBlock;
objectUrl?: string;
uploadChatAttachmentFile(file, fallbackName, selectedProjectId, sessionId)
session.attachment.start
session.attachment.chunk
session.attachment.finish
session.attachment.cancel
session.attachment.delete
blocks.push(...sourceAttachments.map(attachment => attachment.block).filter(
```

Assert it does not contain new UI generation of `data: attachment.data`.

- [ ] **Step 2: Run focused tests to verify RED**

Run:

```bash
cd app && npm test -- --runInBand __tests__/web-chat-ui.test.ts __tests__/web-chat-project-service.test.ts
```

Expected: FAIL because Web still reads attachments to base64.

## Task 6: Add Web Repository Upload API

**Files:**
- Modify: `app/web/src/types/registry.ts`
- Modify: `app/web/src/services/registryRepository.ts`
- Modify: `app/web/src/services/registryWorkspaceService.ts`
- Modify: `app/__tests__/web-chat-project-service.test.ts`

- [ ] **Step 1: Add types**

Extend `RegistrySessionContentBlock`:

```ts
export interface RegistrySessionContentBlock {
  type: 'text' | 'image' | 'resource_link';
  text?: string;
  mimeType?: string;
  data?: string;
  uri?: string;
  name?: string;
  size?: number;
}
```

Add attachment response types for start/chunk/finish/cancel/delete.

- [ ] **Step 2: Add repository methods**

Implement:

```ts
startSessionAttachment(projectId, payload)
uploadSessionAttachmentChunk(projectId, payload)
finishSessionAttachment(projectId, payload)
cancelSessionAttachment(projectId, payload)
deleteSessionAttachment(projectId, payload)
```

- [ ] **Step 3: Expose workspace service methods**

Delegate through `RegistryWorkspaceService` with explicit `projectId`.

- [ ] **Step 4: Run focused service tests**

Run:

```bash
cd app && npm test -- --runInBand __tests__/web-chat-project-service.test.ts
```

Expected: PASS.

## Task 7: Implement Web Attachment Upload Queue

**Files:**
- Modify: `app/web/src/main.tsx`
- Modify: `app/web/src/styles.css`
- Modify: `app/__tests__/web-chat-ui.test.ts`

- [ ] **Step 1: Replace base64 attachment model**

Use upload-state attachments with `file`, `objectUrl`, `progress`, `status`, `block`, `uploadId`, and `attachmentId`.

- [ ] **Step 2: Upload files serially**

For each selected/dropped/pasted file:

1. Require selected session.
2. Start upload.
3. Slice file into 1 MiB chunks.
4. Convert each chunk to base64.
5. Send chunk with offset.
6. Compute SHA-256 with Web Crypto.
7. Finish and store returned `attachment` and `block`.

- [ ] **Step 3: Add cancel/delete/retry**

Uploading attachments call `session.attachment.cancel`. Completed unsent attachments call `session.attachment.delete`. Failed attachments retry with the current in-memory `File`.

- [ ] **Step 4: Send returned blocks**

`sendChatMessage` builds:

```ts
const blocks: RegistryChatContentBlock[] = [];
if (trimmedText) blocks.push({type: 'text', text: trimmedText});
blocks.push(...sourceAttachments.map(attachment => attachment.block).filter(isRegistryChatContentBlock));
```

Block send while any attachment is not completed.

- [ ] **Step 5: Add drag/drop and paste file handling**

Composer drag/drop and paste both call the same file enqueue helper. Mixed paste with files and text accepts files only.

- [ ] **Step 6: Run focused UI tests**

Run:

```bash
cd app && npm test -- --runInBand __tests__/web-chat-ui.test.ts
```

Expected: PASS.

## Task 8: Full Verification

**Files:**
- Verify all touched files.

- [ ] **Step 1: Run server tests**

```bash
cd server && go test ./...
```

Expected: PASS.

- [ ] **Step 2: Run app tests**

```bash
cd app && npm test -- --runInBand
```

Expected: PASS.

- [ ] **Step 3: Run TypeScript check**

```bash
cd app && npm run tsc:web
```

Expected: PASS.

- [ ] **Step 4: Commit and push**

```bash
git add -A
git commit -m "feat: upload chat attachments"
git push origin main
```

Expected: commit and push succeed.

## Self-Review

Spec coverage:

- Chunk upload API: covered by Tasks 1-3.
- Session-scoped storage, sidecar, hash naming, `.part`, 50 MiB, 3-minute expiry: covered by Task 2.
- `session.send.blocks` preservation and URI validation: covered by Task 3.
- Codex image artifact path unification: covered by Task 4.
- Web attach button, drag/drop, paste, upload progress, cancel/delete/retry, disabled send: covered by Tasks 5-7.
- Legacy `image.data` compatibility: covered by Task 3.

Placeholder scan: no open placeholders remain.

Type consistency: upload response types, attachment view, and `RegistrySessionContentBlock` fields are introduced before use in Web tasks.
