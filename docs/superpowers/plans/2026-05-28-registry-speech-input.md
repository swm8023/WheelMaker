# Registry Speech Input Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 Chat 中实现用户自带火山引擎 API Key 的豆包流式语音输入，录音内容经 Registry 代理给火山引擎，识别文本实时写回当前输入框。

**Architecture:** 第一版沿用现有 Registry `/ws` JSON envelope，浏览器以 Base64 PCM chunk 发送语音，Registry 转换为火山引擎二进制/gzip WebSocket 协议。前端语音设置、录音状态机、音频采集、Registry speech client、录音按钮和录音状态条都放到独立 `features/speech/` 模块；现有 `main.tsx` 只做状态接线和组件挂载。

**Tech Stack:** Go 1.26, gorilla/websocket, React 19, TypeScript, CSS, Jest, Web Audio API.

---

## File Structure

- Create: `server/internal/registry/speech_protocol.go`
  - Registry-local speech 方法名、payload struct、脱敏 helper、Volcengine 常量。
- Create: `server/internal/registry/speech_service.go`
  - 每个 Registry client connection 的 speech stream 生命周期、Base64 解码、finish/cancel、事件回写。
- Create: `server/internal/registry/speech_volcengine.go`
  - 火山引擎 WebSocket 握手、二进制/gzip frame 编解码、response text 解析。
- Modify: `server/internal/registry/server.go`
  - 初始化 speech service；在主 dispatch 中接入 `speech.*`；disconnect 时清理 stream。
- Modify: `server/internal/protocol/registry_methods.go`
  - 添加 `speech.start/chunk/finish/cancel` 客户端方法描述。
- Create: `server/internal/registry/speech_test.go`
  - Registry-local dispatch、脱敏、chunk validation、fake provider lifecycle 测试。
- Create: `server/internal/registry/speech_volcengine_test.go`
  - 火山 frame 编解码和 transcript extraction 测试。
- Create: `app/web/src/features/speech/speechSettings.ts`
  - 前端 speech settings 类型、默认值、模型选项、脱敏导出。
- Create: `app/web/src/features/speech/registrySpeechClient.ts`
  - 封装 `speech.start/chunk/finish/cancel` request 和 `speech.transcript/error` event subscription。
- Create: `app/web/src/features/speech/audioCapture.ts`
  - `getUserMedia`、Float32 到 16kHz mono int16 PCM、Base64 chunk 工具。
- Create: `app/web/src/features/speech/useVoiceInputController.ts`
  - 长按、上滑取消、release finish、transcript replacement state machine。
- Create: `app/web/src/features/speech/VoiceInputButton.tsx`
  - composer 右侧 mic button，pointer capture。
- Create: `app/web/src/features/speech/VoiceRecordingBar.tsx`
  - 录音时替换 composer bottom strip 的计时/波形/取消状态 UI。
- Modify: `app/web/src/services/workspacePersistence.ts`
  - 持久化 `speechSettings`，database dump 中脱敏 API Key。
- Modify: `app/web/src/debug/registryDebug.ts`
  - outbound/inbound debug 中脱敏 `speech.start.apiKey`，摘要化 `speech.chunk.pcm`。
- Modify: `app/web/src/types/registry.ts`
  - 添加 speech payload/event 类型。
- Modify: `app/web/src/services/registryClient.ts`
  - 在 websocket boundary debug 前使用 speech redaction。
- Modify: `app/web/src/main.tsx`
  - Chat 设置挂载、speech controller 接线、send button/mic button 切换、录音条挂载、textarea recording readonly。
- Modify: `app/web/src/styles.css`
  - mic button、recording bar、音量动画、取消状态样式。
- Create: `app/__tests__/web-speech-settings.test.ts`
  - speech settings 持久化、模型选项、dump 脱敏源结构测试。
- Create: `app/__tests__/web-speech-client.test.ts`
  - Registry speech client 和 debug redaction 测试。
- Create: `app/__tests__/web-speech-audio.test.ts`
  - PCM/Base64 工具测试。
- Create: `app/__tests__/web-voice-input-controller.test.ts`
  - transcript replacement、cancel restore、gesture threshold state 测试。
- Modify: `app/__tests__/web-chat-ui.test.ts`
  - Chat settings 与 composer voice UI source-structure 测试。

---

### Task 1: Plan and Guardrail Commit

**Files:**
- Create: `docs/superpowers/plans/2026-05-28-registry-speech-input.md`

- [ ] **Step 1: Save this implementation plan**

Use `apply_patch` to create this file.

- [ ] **Step 2: Review for unfinished markers**

Run:

```powershell
rg -n "TO(DO)|TB(D)|\?\?" docs\superpowers\plans\2026-05-28-registry-speech-input.md
```

Expected: no matches.

### Task 2: Registry Speech Protocol and Redaction

**Files:**
- Modify: `server/internal/protocol/registry_methods.go`
- Create: `server/internal/registry/speech_protocol.go`
- Create: `server/internal/registry/speech_test.go`

- [ ] **Step 1: Write failing protocol tests**

Add tests that assert:

```go
if !methodAllowed("client", "speech.start") { t.Fatal("client should call speech.start") }
if methodAllowed("hub", "speech.start") { t.Fatal("hub must not call speech.start") }
redacted := redactSpeechPayload("speech.start", speechStartPayload{APIKey: "secret"})
```

Expected redacted payload has `apiKey: "[redacted]"`.

- [ ] **Step 2: Run failing Registry tests**

Run:

```powershell
go test ./internal/registry -run Speech
```

Expected: FAIL because speech methods and redaction helpers do not exist.

- [ ] **Step 3: Add method descriptors and protocol structs**

Add `speech.start`, `speech.chunk`, `speech.finish`, `speech.cancel` constants to protocol descriptors with client role only, then implement `speech_protocol.go` with `speechStartPayload`, `speechChunkPayload`, `speechTranscriptPayload`, `speechErrorPayload`, `redactSpeechPayload`, and `isSpeechRequestMethod`.

- [ ] **Step 4: Run Registry protocol tests**

Run:

```powershell
go test ./internal/registry -run Speech
```

Expected: PASS.

### Task 3: Registry Stream Lifecycle With Fake Provider

**Files:**
- Modify: `server/internal/registry/server.go`
- Create/modify: `server/internal/registry/speech_service.go`
- Modify: `server/internal/registry/speech_test.go`

- [ ] **Step 1: Write failing lifecycle tests**

Test over real `/ws`:

```go
client sends speech.start with apiKey -> receives response streamId
client sends speech.chunk with base64 pcm -> fake provider receives bytes
fake provider emits transcript -> client receives speech.transcript event
client sends speech.finish -> fake provider closes and response ok
```

Also test bad Base64 returns `INVALID_ARGUMENT`, and disconnect cancels the fake stream.

- [ ] **Step 2: Run failing lifecycle tests**

Run:

```powershell
go test ./internal/registry -run Speech
```

Expected: FAIL because dispatch and service do not exist.

- [ ] **Step 3: Implement speech service**

Implement:

```go
type speechProvider interface {
  Start(ctx context.Context, req speechProviderStartRequest, events speechEventSink) (speechProviderStream, error)
}
type speechProviderStream interface {
  WriteAudio(ctx context.Context, pcm []byte) error
  Finish(ctx context.Context) error
  Cancel()
}
```

`speechService` owns active streams by `connectionID + streamID`, validates ownership, decodes Base64 chunks, writes transcript/error events through `peer.write`, and cleans streams on finish/cancel/disconnect.

- [ ] **Step 4: Wire Registry dispatch**

In `server.go`, initialize `s.speech = newSpeechService(defaultVolcengineSpeechProvider())`; before generic forward dispatch, handle `isSpeechRequestMethod(in.Method)` with `s.speech.handleRequest(state, in)`. In disconnect cleanup, call `s.speech.cancelConnection(state.id)`.

- [ ] **Step 5: Run lifecycle tests**

Run:

```powershell
go test ./internal/registry -run Speech
```

Expected: PASS.

### Task 4: Volcengine Frame Codec and Provider

**Files:**
- Create: `server/internal/registry/speech_volcengine.go`
- Create: `server/internal/registry/speech_volcengine_test.go`

- [ ] **Step 1: Write failing codec tests**

Test that:

```go
buildVolcengineFullClientRequest(...)
```

emits a header with version 1, header size 1, full client request type, JSON serialization, gzip compression, big-endian payload size; audio frame uses audio-only type and gzip compression; final audio frame sets the final flag; response parser extracts `result.text`.

- [ ] **Step 2: Run failing codec tests**

Run:

```powershell
go test ./internal/registry -run Volcengine
```

Expected: FAIL because codec functions do not exist.

- [ ] **Step 3: Implement Volcengine provider**

Implement `volcengineSpeechProvider` using `websocket.DefaultDialer.Dial` with headers:

```go
X-Api-Key: <user key>
X-Api-Resource-Id: volc.seedasr.sauc.duration
X-Api-Request-Id: <generated id>
X-Api-Sequence: -1
```

Send full client request first, then audio-only frames, then final audio-only frame on finish. A read goroutine parses server full responses and emits `speech.transcript`; error frames become `speech.error`.

- [ ] **Step 4: Run Volcengine tests**

Run:

```powershell
go test ./internal/registry -run Volcengine
```

Expected: PASS.

### Task 5: Web Speech Settings, Persistence, and Debug Redaction

**Files:**
- Create: `app/web/src/features/speech/speechSettings.ts`
- Modify: `app/web/src/services/workspacePersistence.ts`
- Modify: `app/web/src/debug/registryDebug.ts`
- Modify: `app/web/src/services/registryClient.ts`
- Create: `app/__tests__/web-speech-settings.test.ts`
- Create: `app/__tests__/web-speech-client.test.ts`

- [ ] **Step 1: Write failing Jest tests**

Tests assert:

```ts
DEFAULT_SPEECH_SETTINGS.enabled === false
SPEECH_MODEL_OPTIONS[0].resourceId === 'volc.seedasr.sauc.duration'
workspacePersistence contains speechSettings and redacts volcengineApiKey in dumpDatabase()
debug record for speech.start does not contain raw apiKey
debug record for speech.chunk does not contain raw pcm
```

- [ ] **Step 2: Run failing Jest tests**

Run:

```powershell
npm test -- --runTestsByPath __tests__/web-speech-settings.test.ts __tests__/web-speech-client.test.ts --runInBand
```

Expected: FAIL because speech settings and redaction do not exist.

- [ ] **Step 3: Implement settings and redaction**

Add `speechSettings` to persisted global state and sanitize it through `normalizeSpeechSettings`. Persist it as one JSON global key. In database dump, replace `volcengineApiKey` with `[redacted]`. Add `redactRegistryDebugEnvelope` so debug captures redact API key and summarize PCM before records are stored.

- [ ] **Step 4: Run Jest tests**

Run:

```powershell
npm test -- --runTestsByPath __tests__/web-speech-settings.test.ts __tests__/web-speech-client.test.ts --runInBand
```

Expected: PASS.

### Task 6: Web Audio and Voice Controller Modules

**Files:**
- Create: `app/web/src/features/speech/audioCapture.ts`
- Create: `app/web/src/features/speech/registrySpeechClient.ts`
- Create: `app/web/src/features/speech/useVoiceInputController.ts`
- Create: `app/__tests__/web-speech-audio.test.ts`
- Create: `app/__tests__/web-voice-input-controller.test.ts`

- [ ] **Step 1: Write failing module tests**

Tests assert:

```ts
floatTo16BitPCM(new Float32Array([-1, 0, 1])) // clamps to int16 little-endian
base64FromBytes(new Uint8Array([1, 2, 3])) === 'AQID'
replaceVoiceSegment('hi world', 3, 8, 'there') === 'hi there'
cancel restores baseText
swipe up beyond threshold changes gesture state to cancel
```

- [ ] **Step 2: Run failing module tests**

Run:

```powershell
npm test -- --runTestsByPath __tests__/web-speech-audio.test.ts __tests__/web-voice-input-controller.test.ts --runInBand
```

Expected: FAIL because modules do not exist.

- [ ] **Step 3: Implement modules**

Implement reusable pure helpers first, then the hook:

```ts
export function replaceVoiceSegment(baseText: string, insertStart: number, insertEnd: number, voiceText: string): string
export function resolveVoiceGestureState(startY: number, currentY: number, threshold = 52): 'recording' | 'cancel'
export function floatTo16BitPCM(input: Float32Array): ArrayBuffer
```

`registrySpeechClient` wraps `RegistryClient.request`, subscribes to speech events, and exposes typed callbacks.

- [ ] **Step 4: Run module tests**

Run:

```powershell
npm test -- --runTestsByPath __tests__/web-speech-audio.test.ts __tests__/web-voice-input-controller.test.ts --runInBand
```

Expected: PASS.

### Task 7: Composer UI Wiring

**Files:**
- Create: `app/web/src/features/speech/VoiceInputButton.tsx`
- Create: `app/web/src/features/speech/VoiceRecordingBar.tsx`
- Modify: `app/web/src/main.tsx`
- Modify: `app/web/src/styles.css`
- Modify: `app/__tests__/web-chat-ui.test.ts`

- [ ] **Step 1: Write failing UI source tests**

Tests assert:

```ts
main.tsx imports VoiceInputButton and VoiceRecordingBar
Chat settings include Voice Input, Volcengine API Key, Doubao Streaming ASR 2.0
send button is conditionally replaced by VoiceInputButton when speechSettings.enabled
textarea readOnly includes voice recording state
toolbar area conditionally renders VoiceRecordingBar while recording
styles contain .voice-input-button and .voice-recording-bar
```

- [ ] **Step 2: Run failing UI tests**

Run:

```powershell
npm test -- --runTestsByPath __tests__/web-chat-ui.test.ts --runInBand
```

Expected: FAIL because UI is not wired.

- [ ] **Step 3: Implement UI wiring**

Add Chat settings rows, initialize speech settings from persistence, persist changes through workspace store, create speech client from the active Registry repository/client path, and replace the send button with `VoiceInputButton` when enabled. Render `VoiceRecordingBar` instead of composer toolbar while recording.

- [ ] **Step 4: Run UI tests**

Run:

```powershell
npm test -- --runTestsByPath __tests__/web-chat-ui.test.ts --runInBand
```

Expected: PASS.

### Task 8: Full Verification

**Files:** all touched files.

- [ ] **Step 1: Run focused Go tests**

Run:

```powershell
go test ./internal/protocol ./internal/registry
```

Expected: PASS.

- [ ] **Step 2: Run focused web tests**

Run:

```powershell
npm test -- --runTestsByPath __tests__/web-speech-settings.test.ts __tests__/web-speech-client.test.ts __tests__/web-speech-audio.test.ts __tests__/web-voice-input-controller.test.ts __tests__/web-chat-ui.test.ts --runInBand
```

Expected: PASS.

- [ ] **Step 3: Run TypeScript check**

Run:

```powershell
npm run tsc:web
```

Expected: PASS.

- [ ] **Step 4: Run full Go tests**

Run:

```powershell
go test ./...
```

Expected: PASS.

- [ ] **Step 5: Build web**

Run:

```powershell
npm run build:web
```

Expected: PASS and output under `~/.wheelmaker/web`.

---

## Self-Review

- Spec coverage: settings, Base64 `/ws`, Registry-local protocol, Volcengine proxy, transcript replacement, cancel gesture, debug redaction, code split, tests, and manual verification all map to tasks.
- Unfinished-marker scan: no standard unfinished markers or incomplete sections should remain.
- Type consistency: speech method names are `speech.start`, `speech.chunk`, `speech.finish`, `speech.cancel`; event names are `speech.transcript`, `speech.error`; model resource ID is `volc.seedasr.sauc.duration`.
