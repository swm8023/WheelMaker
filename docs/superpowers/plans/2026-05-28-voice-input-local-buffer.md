# Voice Input Local Buffer Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let voice input start recording even when Registry is temporarily disconnected, cache a few seconds of local PCM, and flush it in order once Registry reconnects.

**Architecture:** Move transient audio buffering and ordered chunk sending into focused speech feature modules. `main.tsx` remains the UI wiring layer: it opens the microphone, starts/retries the Registry speech stream, reacts to user finish/cancel, and updates the composer from transcript events.

**Tech Stack:** React 19, TypeScript, Jest, existing Registry JSON WebSocket speech methods.

---

## File Structure

- Create `app/web/src/features/speech/voiceInputBuffer.ts`
  - Owns the in-memory PCM ring limit, byte/duration accounting, overflow detection, and drain/clear operations.
- Create `app/web/src/features/speech/voiceInputSendQueue.ts`
  - Owns fully serial `speech.chunk` sending, `seq` allocation, Base64 encoding at send time, queue stats, drain, and cancellation.
- Modify `app/web/src/features/speech/VoiceRecordingBar.tsx`
  - Adds `status: 'buffering' | 'recording' | 'finishing'` display text while preserving cancel intent behavior.
- Modify `app/web/src/main.tsx`
  - Reorders voice startup: open microphone first, buffer while disconnected, connect/start stream, flush queue, finish after flush.
  - Removes the hard `connected` gate for voice start when a reconnect context exists.
  - Adds pending-finish timeout and context checks.
- Add tests:
  - `app/__tests__/web-voice-input-buffer.test.ts`
  - `app/__tests__/web-voice-input-send-queue.test.ts`
  - Extend existing UI/source tests where they are already string-based.

## Constants

- `VOICE_INPUT_BUFFER_MAX_MS = 5000`
- `VOICE_INPUT_FINISH_WAIT_MS = 3000`
- PCM format remains 16000 Hz, 16-bit, mono.
- `bytesPerSecond = 16000 * 2 * 1 = 32000`

## Tasks

### Task 1: PCM Buffer

**Files:**
- Create: `app/web/src/features/speech/voiceInputBuffer.ts`
- Test: `app/__tests__/web-voice-input-buffer.test.ts`

- [ ] **Step 1: Write failing tests**

Tests cover:
- appending PCM chunks records byte count, duration, and chunk count;
- draining returns chunks in capture order and clears the buffer;
- appending past the 5 second limit reports overflow and does not silently drop the beginning.

- [ ] **Step 2: Run test and verify red**

Run:

```powershell
npm test -- --runTestsByPath __tests__/web-voice-input-buffer.test.ts
```

Expected: FAIL because the module does not exist.

- [ ] **Step 3: Implement minimal buffer**

Create a small class/factory with:
- `append(bytes: Uint8Array): {ok: boolean; overflow: boolean; stats: VoiceInputBufferStats}`
- `drain(): Uint8Array[]`
- `clear(): void`
- `stats(): VoiceInputBufferStats`

- [ ] **Step 4: Verify green**

Run the same test command. Expected: PASS.

### Task 2: Serial Send Queue

**Files:**
- Create: `app/web/src/features/speech/voiceInputSendQueue.ts`
- Test: `app/__tests__/web-voice-input-send-queue.test.ts`

- [ ] **Step 1: Write failing tests**

Tests cover:
- chunks are sent strictly serially in enqueue order;
- `seq` is allocated at send time starting from 1;
- bytes are converted to Base64 only at send time;
- `drain()` waits for queued sends;
- a send failure rejects drain and prevents later chunks from being treated as successful.

- [ ] **Step 2: Run test and verify red**

Run:

```powershell
npm test -- --runTestsByPath __tests__/web-voice-input-send-queue.test.ts
```

Expected: FAIL because the module does not exist.

- [ ] **Step 3: Implement minimal queue**

The queue receives:
- `streamId`
- `sendChunk({streamId, seq, pcm})`

It stores PCM bytes, then calls existing `base64FromBytes(bytes)` immediately before `sendChunk`.

- [ ] **Step 4: Verify green**

Run the same test command. Expected: PASS.

### Task 3: Recording Bar States

**Files:**
- Modify: `app/web/src/features/speech/VoiceRecordingBar.tsx`
- Test: existing UI/source tests as needed.

- [ ] **Step 1: Write failing expectation**

Assert `VoiceRecordingBar` accepts and renders:
- `Connecting...`
- `Recording`
- `Finishing...`

- [ ] **Step 2: Implement status prop**

Add `status?: 'buffering' | 'recording' | 'finishing'`, default to `recording`.

- [ ] **Step 3: Verify**

Run:

```powershell
npm test -- --runTestsByPath __tests__/web-chat-ui.test.ts
```

Expected: PASS after updating source expectations if needed.

### Task 4: Main Voice Flow

**Files:**
- Modify: `app/web/src/main.tsx`

- [ ] **Step 1: Add state/refs**

Add refs for:
- buffer
- send queue
- voice status
- operation generation/cancelled state
- pending finish timer
- runtime key captured at voice start

- [ ] **Step 2: Change start behavior**

New start behavior:
- validate voice enabled and API Key first;
- if not connected but no `address + projectId`, fail before opening mic;
- create voice session from current composer text/selection;
- set UI to buffering and open microphone immediately;
- enqueue PCM into buffer/queue;
- if connected or reconnect succeeds, call `speech.start`;
- on start success, create serial send queue and flush buffered chunks.

- [ ] **Step 3: Change finish behavior**

New finish behavior:
- stop microphone immediately;
- if no stream yet, wait up to 3 seconds for reconnect/start/flush;
- once stream exists and queue drains, call `speech.finish`;
- if timeout occurs, cancel and restore base text.

- [ ] **Step 4: Change cancel/error behavior**

Rules:
- user gesture cancel wins immediately;
- buffer overflow cancels immediately;
- chunk failure cancels immediately;
- API/Volcengine start failures cancel immediately;
- disconnected/timeout start failures retry only inside the allowed wait window.

- [ ] **Step 5: Transcript/context guard**

Keep existing stream ID filtering, and additionally ignore/cancel if the captured chat runtime key no longer matches the current chat runtime key.

### Task 5: Verification

Run:

```powershell
npm test -- --runTestsByPath __tests__/web-voice-input-buffer.test.ts __tests__/web-voice-input-send-queue.test.ts __tests__/web-voice-input-controller.test.ts __tests__/web-speech-client.test.ts
npm test
npm run tsc:web
npm run build:web
git diff --check
```

Expected:
- all Jest suites pass;
- TypeScript passes;
- Webpack production build succeeds;
- no whitespace errors.

## Self-Review

- The plan covers disconnected start, local-only memory buffer, 5 second overflow cancellation, 3 second pending finish, serial sending, transcript filtering, cancel priority, warning/error-only logs, and UI states.
- There are no persistent audio writes and no Base64 audio logging.
- Server protocol is unchanged.
