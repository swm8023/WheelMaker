# Registry Speech Input Design

Date: 2026-05-28

## Context

WheelMaker should support streaming speech input in Chat using Volcengine Doubao Streaming ASR 2.0. The user enters their own Volcengine API key in the Workspace Web UI. The first implementation should avoid deployment changes such as Nginx route updates and should not require Hub involvement.

The current Registry browser connection uses the existing `/ws` JSON envelope. That path currently reads JSON messages, so it is not ready for raw binary audio frames. Volcengine's streaming ASR WebSocket also requires custom handshake headers such as `X-Api-Key`, `X-Api-Resource-Id`, `X-Api-Request-Id`, and `X-Api-Sequence`, which browser WebSocket APIs cannot set directly. Therefore the Registry must proxy the Volcengine connection.

## Goals

- Add a Chat voice-input mode that streams microphone audio to Registry while the user holds the mic button.
- Stream ASR text back into the current Chat input in real time.
- Let each user provide their own Volcengine API key in Chat settings.
- Use Doubao Streaming ASR 2.0 postpaid mode for the first version.
- Keep the implementation split into focused files/modules instead of adding large blocks to existing Registry or UI files.
- Avoid Nginx and deployment changes in the first version.

## Non-Goals

- No direct browser-to-Volcengine connection.
- No Hub, marketplace, Registry package publishing, or Desktop bridge dependency.
- No dedicated binary Registry speech WebSocket in the first version.
- No multi-provider speech abstraction until another provider is actually added.
- No server-side persistence of API keys or recorded audio.

## Product Decisions

### Settings

Add Chat settings for speech input:

- `Voice Input` toggle.
- `Volcengine API Key` password field.
- `Speech Model` select with a single option for now:
  - Label: `Doubao Streaming ASR 2.0`
  - Resource ID: `volc.seedasr.sauc.duration`

The API key is stored in browser-local persistent settings, matching the user decision for this feature. It must be excluded from debug database dumps and redacted anywhere settings are displayed for diagnostics.

### Composer Interaction

When `Voice Input` is disabled, the right composer action remains the current send button.

When `Voice Input` is enabled, the right composer action becomes a mic button on both desktop and mobile, even if there is typed text or attachments. Sending remains available through keyboard and IME send behavior. This keeps desktop and mobile behavior consistent.

Mic behavior:

- Long press starts recording.
- Release finishes recording.
- Swipe up cancels the entire voice input for that recording.
- `Esc`, composer blur, Registry disconnect, or fatal stream errors cancel and restore the previous input.

During recording, the composer bottom strip is replaced by a stronger voice-recording state instead of only changing the icon. It should show a timer, audio-level animation, and release/cancel guidance. When the swipe-up threshold is crossed, it changes to a cancel state before release.

### Transcript Insertion

At recording start, capture:

- `baseText`
- `insertStart`
- `insertEnd`

Each `speech.transcript` event updates one voice segment in the composer:

```text
nextText = baseText[0:insertStart] + voiceText + baseText[insertEnd:]
```

The transcript should be treated as the current full text for the active recording, not as an append-only delta. This avoids duplicated text when the ASR stream revises partial results.

On successful final result, keep the final transcript in the input. On cancel or stream failure, restore `baseText`.

## Architecture

### Data Flow

1. User enables Voice Input and enters a Volcengine API key in Chat settings.
2. User long-presses the mic button in Chat.
3. Browser captures microphone audio with `getUserMedia`.
4. Browser converts audio to `16kHz`, mono, signed `int16` PCM.
5. Browser sends Base64 PCM chunks to Registry over the existing `/ws` JSON envelope.
6. Registry opens a Volcengine streaming ASR WebSocket using the user-provided API key as a handshake header.
7. Registry translates browser JSON chunks into the Volcengine binary/gzip protocol.
8. Registry forwards partial and final transcript events back to the same browser connection.
9. Browser replaces the active voice segment in the current input as transcripts arrive.

### Why Base64 First

The current Registry `/ws` path expects JSON and Nginx already proxies it. Base64 PCM inside JSON adds overhead, but a 200ms `16kHz` mono `int16` PCM chunk is about 6.4KB raw and roughly 8.5KB after Base64. At 5 chunks per second, the MVP traffic is acceptable for chat voice input.

This lets the first version avoid:

- Nginx route changes.
- A second Registry WebSocket endpoint.
- Binary-frame compatibility changes in the existing `/ws` reader.

If latency, CPU, or bandwidth become an issue, add a later binary speech endpoint such as `/speech/volcengine` and proxy it separately.

## Registry Protocol

Speech uses Registry-local messages on the existing browser Registry connection. These messages are not routed to Hub, project sessions, or tool execution.

Client to Registry:

- `speech.start`
- `speech.chunk`
- `speech.finish`
- `speech.cancel`

Registry to Client:

- `speech.transcript`
- `speech.error`

Suggested payloads:

```ts
type SpeechStartRequest = {
  provider: "volcengine";
  model: "doubao-streaming-asr-2.0";
  apiKey: string;
  audio: {
    format: "pcm";
    codec: "raw";
    rate: 16000;
    bits: 16;
    channel: 1;
  };
};

type SpeechStartResponse = {
  streamId: string;
};

type SpeechChunkRequest = {
  streamId: string;
  seq: number;
  pcm: string;
};

type SpeechFinishRequest = {
  streamId: string;
};

type SpeechCancelRequest = {
  streamId: string;
  reason: "user" | "gesture" | "error" | "disconnect";
};

type SpeechTranscriptEvent = {
  streamId: string;
  text: string;
  final: boolean;
};

type SpeechErrorEvent = {
  streamId?: string;
  code: string;
  message: string;
  retryable: boolean;
};
```

`speech.chunk` should acknowledge quickly and enqueue audio internally so the Registry read loop is not blocked by upstream Volcengine network writes.

## Volcengine Integration

The first Volcengine implementation targets Doubao Streaming ASR 2.0 postpaid:

- Endpoint: `wss://openspeech.bytedance.com/api/v3/sauc/bigmodel_async`
- Resource ID: `volc.seedasr.sauc.duration`
- Model name: `bigmodel`
- Language: `zh-CN`
- Audio: `pcm`, `raw`, `16000`, `16-bit`, `mono`
- Enable ITN and punctuation.
- Request utterance-level output when useful for finalization.

Registry owns the Volcengine protocol details:

- WebSocket handshake headers.
- `X-Api-Key`, `X-Api-Resource-Id`, `X-Api-Request-Id`, and `X-Api-Sequence: -1` handling.
- Request ID generation.
- Volcengine binary frame encoding and decoding.
- Gzip JSON payload handling.
- Mapping upstream partial/final results to `speech.transcript`.
- Mapping upstream auth, quota, format, and network errors to `speech.error`.

Registry should log Volcengine request IDs and `X-Tt-Logid` values for debugging when available, but must never log the API key or Base64 audio payload.

## Code Organization

Keep this feature isolated. The expected implementation should make only minimal touch-point edits in existing large files to register handlers or mount UI.

### Registry

Prefer new focused files under `server/internal/registry/`:

- `speech_protocol.go`: speech method names, request/response/event structs, redaction helpers.
- `speech_handler.go`: Registry-local dispatch for `speech.*` JSON messages.
- `speech_service.go`: stream lifecycle, queueing, cancellation, timeouts, per-connection ownership.
- `speech_volcengine.go`: Volcengine client, handshake headers, binary/gzip protocol, transcript parsing.
- `speech_test.go` or focused `speech_*_test.go` files for protocol, lifecycle, and redaction tests.

Existing Registry files should only wire speech dispatch into the current WebSocket envelope flow. Do not place Volcengine protocol code inside the existing main Registry server file.

### Web UI

Prefer new focused modules under a feature folder such as `app/web/src/features/speech/`:

- `speechSettings.ts`: persisted settings shape, defaults, masking/export redaction helpers.
- `registrySpeechClient.ts`: `speech.start/chunk/finish/cancel` and event subscription wrapper.
- `audioCapture.ts`: microphone permission, capture lifecycle, PCM conversion, chunk sizing.
- `audioWorklet` file or equivalent small worker module for stable streaming capture.
- `useVoiceInputController.ts`: composer-facing state machine for press, recording, cancel, finish, errors.
- `VoiceInputButton.tsx` and `VoiceRecordingBar.tsx`: visual controls and recording strip.
- Mobile gesture logic should be inside the voice controller or a small `useVoicePressGesture.ts` helper, not embedded into the existing composer body.

Existing Chat files should mount the new components and pass current composer state handlers. The implementation should avoid growing `main.tsx` with audio capture, Base64 encoding, pointer gesture, or ASR protocol details.

Desktop and mobile should share the same voice-input state machine. Mobile-only presentation and pointer threshold details should stay in the speech feature modules instead of being scattered through existing composer conditionals.

## Frontend Audio Details

Use browser microphone capture through `navigator.mediaDevices.getUserMedia({ audio: true })`.

Preferred capture path:

- `AudioContext`
- `AudioWorklet` for steady audio processing
- resample to `16000Hz`
- downmix to mono
- convert Float32 samples to signed little-endian `int16` PCM
- emit chunks around 200ms

The browser-to-Registry JSON chunk uses Base64 of the raw PCM bytes. The Registry converts that back to bytes before writing to Volcengine.

Recording constraints:

- Maximum recording duration: 60 seconds for MVP.
- Accidental short recording threshold: about 300ms.
- One active stream per browser connection for MVP.
- Cancel local recording immediately when backpressure or Registry errors make the stream unhealthy.

## Error Handling

User-visible errors should be short and actionable:

- Missing API key: ask the user to fill the Chat speech API key.
- Microphone permission denied: ask the user to allow microphone access.
- Registry disconnected: cancel and restore previous input.
- Volcengine auth or quota failure: keep settings openable and show the provider error category.
- Audio unsupported: report that this browser cannot capture compatible microphone audio.
- Timeout waiting for final transcript: keep the latest partial text only if Registry completed successfully; otherwise restore `baseText`.

Server-side cleanup must run on finish, cancel, upstream error, downstream disconnect, and timeout.

## Observability and Security

- Redact `speech.start.apiKey` in client debug logs, server logs, and database/debug export paths.
- Omit or summarize `speech.chunk.pcm`; record only metadata such as `streamId`, `seq`, byte count, and timing.
- Never persist API keys or audio in Registry.
- Keep API key lifetime scoped to the active Registry stream.
- Avoid writing transcript events into unrelated debug streams unless existing Chat message flow already records composer state.

## Testing

Registry tests:

- Speech dispatch recognizes `speech.*` as Registry-local messages.
- `speech.start` redacts API key in all diagnostic views.
- `speech.chunk` validates `streamId`, sequence ordering, and Base64 decoding errors.
- Stream lifecycle handles finish, cancel, disconnect, upstream error, and timeout cleanup.
- Volcengine client can be tested with a fake WebSocket transport for frame encoding/decoding and transcript mapping.

Web UI tests:

- Chat settings expose Voice Input, masked API key field, and the single Doubao Streaming ASR 2.0 model option.
- Database/debug dump excludes or masks the speech API key.
- Enabling Voice Input replaces the send button with the mic button on desktop and mobile.
- Long press starts recording, release finishes, swipe up cancels.
- Recording strip replaces the bottom composer strip and switches to cancel state after the swipe threshold.
- Transcript events replace the active voice segment instead of appending duplicate text.
- Cancel/error restores the original composer input.
- PCM chunking produces `16kHz` mono `int16` data and Base64 chunks.

Manual verification:

- Browser asks for mic permission only when starting recording.
- Partial transcripts appear in the input during recording.
- Final transcript remains after release.
- Swipe-up cancel removes all text inserted by the active recording.
- Network debug tools do not show raw API keys in app debug exports.

## Implementation Phases

1. Add settings persistence and redaction for Chat speech settings.
2. Add browser audio capture and voice-input state machine behind the disabled-by-default toggle.
3. Add Registry-local `speech.*` protocol handling with a fake provider for local UI flow testing.
4. Add Volcengine provider implementation in isolated Registry files.
5. Wire live transcript insertion into the Chat composer.
6. Add focused tests and manual verification for desktop and mobile.

## Open Risks

- Browser audio resampling quality and AudioWorklet support need real-device testing.
- Volcengine streaming responses may revise partial text; the replacement model handles this, but transcript parsing must confirm which upstream field is authoritative.
- Base64 over JSON is acceptable for MVP but may become costly if recordings are long or many users record concurrently.
- The current Web UI may have concentrated composer code; implementation should extract only the speech-related parts needed for this feature and avoid a broad composer rewrite.
