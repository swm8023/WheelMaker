# Voice Input State Machine Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Refactor voice input into explicit client/server state machines, stabilize browser PCM capture, and align Volcengine ASR request fields with the agreed product behavior.

**Architecture:** Add small speech feature helpers for constants, capture state, and transcoding while keeping `main.tsx` as the wiring layer. Keep Registry speech code isolated in existing speech files, but enforce a single connection-level speech slot and forward final-empty responses.

**Tech Stack:** React 19, TypeScript, Jest, Web Audio API, Go Registry, Gorilla WebSocket, Go test.

---

## File Structure

- Create `app/web/src/features/speech/voiceInputConstants.ts`
  - Defines `VOICE_SHORT_TIMEOUT_MS`, `VOICE_LONG_TIMEOUT_MS`, `VOICE_AUDIO_CHUNK_MS`, `VOICE_AUDIO_CHUNK_BYTES`, and status/state type helpers.
- Modify `app/web/src/features/speech/audioCapture.ts`
  - Adds a stateful PCM transcoder and `onReady`.
  - Keeps ScriptProcessor and 200ms chunks.
- Modify `app/web/src/features/speech/useVoiceInputController.ts`
  - Removes heuristic transcript merge and treats provider text as full current result.
- Modify `app/web/src/features/speech/VoiceRecordingBar.tsx`
  - Adds `permission`, `starting`, and `recognizing` display states.
- Modify `app/web/src/main.tsx`
  - Uses the shared constants.
  - Represents voice status explicitly.
  - Starts Registry speech only after microphone capture is ready.
  - Uses 15s for buffer/final/retry ceilings.
- Modify `server/internal/registry/speech_protocol.go`
  - Defines shared server speech timeout constants.
- Modify `server/internal/registry/speech_service.go`
  - Enforces single speech slot per connection across active and closing streams.
  - Allows cancel of closing streams.
  - Uses 15s closing route.
- Modify `server/internal/registry/speech_volcengine.go`
  - Sends DDC on, utterances off.
  - Emits transcript events for final-empty frames.
- Modify focused tests in `app/__tests__/` and `server/internal/registry/*_test.go`.

## Tasks

### Task 1: Documentation

**Files:**
- Create: `docs/superpowers/specs/2026-05-29-voice-input-state-machine-design.md`
- Create: `docs/superpowers/plans/2026-05-29-voice-input-state-machine.md`

- [x] **Step 1: Write the design spec**

The spec records client states, server slot states, Volcengine fields, time constants, transcript semantics, and audio capture rules.

- [x] **Step 2: Write this implementation plan**

The plan maps the spec to concrete files and tests.

### Task 2: Frontend Tests

**Files:**
- Modify: `app/__tests__/web-speech-audio.test.ts`
- Modify: `app/__tests__/web-voice-input-controller.test.ts`
- Modify: `app/__tests__/web-voice-recording-bar.test.tsx`
- Modify: `app/__tests__/web-voice-input-local-buffer.test.ts`

- [ ] **Step 1: Add failing expectations**

Add expectations for:

```ts
expect(VOICE_AUDIO_CHUNK_MS).toBe(200);
expect(VOICE_LONG_TIMEOUT_MS).toBe(15000);
expect(session.applyTranscript('短')).toBe('prefix 短suffix');
expect(renderText('recognizing')).toContain('Recognizing...');
expect(mainTsx).toContain('VOICE_LONG_TIMEOUT_MS');
expect(mainTsx).toContain("setVoiceRecordingStatus('recognizing')");
```

- [ ] **Step 2: Run focused tests and verify red**

Run:

```powershell
cd app
npm test -- --runTestsByPath __tests__/web-speech-audio.test.ts __tests__/web-voice-input-controller.test.ts __tests__/web-voice-recording-bar.test.tsx __tests__/web-voice-input-local-buffer.test.ts
```

Expected: FAIL because constants, recognizing state, and full replacement semantics are not implemented yet.

### Task 3: Server Tests

**Files:**
- Modify: `server/internal/registry/speech_volcengine_test.go`
- Modify: `server/internal/registry/speech_test.go`

- [ ] **Step 1: Add failing expectations**

Add expectations that:

```go
request["enable_ddc"] == true
request["show_utterances"] == false
```

Add tests that final-empty provider frames emit `speech.transcript` with `final=true`, new start cancels old closing streams, and cancel accepts closing streams.

- [ ] **Step 2: Run focused tests and verify red**

Run:

```powershell
cd server
go test ./internal/registry -run "TestVolcengine|TestSpeech"
```

Expected: FAIL because current code still requests utterances, does not emit final-empty transcript, and does not cancel closing streams on new start/cancel.

### Task 4: Frontend Implementation

**Files:**
- Create: `app/web/src/features/speech/voiceInputConstants.ts`
- Modify: `app/web/src/features/speech/audioCapture.ts`
- Modify: `app/web/src/features/speech/useVoiceInputController.ts`
- Modify: `app/web/src/features/speech/VoiceRecordingBar.tsx`
- Modify: `app/web/src/main.tsx`

- [ ] **Step 1: Add constants and status types**

Define shared voice timing and chunk constants. Import them from `main.tsx`, `audioCapture.ts`, and buffer setup.

- [ ] **Step 2: Add stateful transcoder**

Implement a small transcoder that keeps pending bytes and resampler phase across callbacks. `stop({flush:true})` must emit the tail chunk.

- [ ] **Step 3: Add `onReady`**

Call `onReady` once from the first audio process callback before emitting chunks. `main.tsx` uses this to move from `mic_starting` to `local_buffering` or `streaming`.

- [ ] **Step 4: Apply full transcript semantics**

`applyTranscript(text)` sets live transcript to `text`. It no longer guesses or appends shorter segment-only text.

- [ ] **Step 5: Add UI recognizing states**

Voice bar supports `permission`, `starting`, `buffering`, `recording`, `finishing`, `recognizing`.

- [ ] **Step 6: Update main voice flow**

Use 15s final/buffer/start/finish waits. Start remote stream after capture is ready. Set `recognizing` after `speech.finish` and keep the button inert until final or timeout.

- [ ] **Step 7: Run focused frontend tests and verify green**

Run the same focused Jest command from Task 2. Expected: PASS.

### Task 5: Server Implementation

**Files:**
- Modify: `server/internal/registry/speech_protocol.go`
- Modify: `server/internal/registry/speech_service.go`
- Modify: `server/internal/registry/speech_volcengine.go`

- [ ] **Step 1: Add server timeout constants**

Use `15 * time.Second` for start, finish, idle, and closing route.

- [ ] **Step 2: Enforce connection slot cleanup**

On `speech.start`, detach and cancel both active and closing streams for the connection before installing the new stream.

- [ ] **Step 3: Support closing cancel**

`speech.cancel` first tries active, then closing. Closing cancel responds OK and cancels provider.

- [ ] **Step 4: Emit final-empty transcript**

In the Volcengine read loop, call `events.Transcript(parsed.Text, parsed.Final)` when `parsed.Final` is true, even if `parsed.Text` is empty.

- [ ] **Step 5: Update Volcengine request fields**

Set `enable_ddc=true` and `show_utterances=false`.

- [ ] **Step 6: Run focused server tests and verify green**

Run:

```powershell
cd server
go test ./internal/registry
```

Expected: PASS.

### Task 6: Full Verification and Commit

**Files:**
- All modified files.

- [ ] **Step 1: Run frontend verification**

Run:

```powershell
cd app
npm test -- --runTestsByPath __tests__/web-speech-audio.test.ts __tests__/web-voice-input-controller.test.ts __tests__/web-voice-recording-bar.test.tsx __tests__/web-voice-input-local-buffer.test.ts __tests__/web-speech-client.test.ts
npm run tsc:web
npm run build:web
```

Expected: all exit 0.

- [ ] **Step 2: Run server verification**

Run:

```powershell
cd server
go test ./internal/registry
```

Expected: exit 0.

- [ ] **Step 3: Run repository checks**

Run:

```powershell
git diff --check
git status --short
```

Expected: no whitespace errors; status only shows intentional files.

- [ ] **Step 4: Commit and push**

Run the repository completion gate:

```powershell
git add -A
git commit -m "refactor: stabilize voice input state machine"
git push origin main
```

Expected: commit and push succeed.

## Self Review

- Spec coverage: client states, server slot, time constants, Volcengine fields, transcript final handling, capture/transcoding, permission behavior, and waiting UI are mapped to tasks.
- Placeholder scan: no TBD/TODO placeholders remain.
- Type consistency: frontend status values and server state language match the spec.
