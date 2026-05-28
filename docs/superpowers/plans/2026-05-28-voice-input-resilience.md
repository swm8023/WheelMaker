# Voice Input Resilience Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让浏览器语音输入在 iOS 上能启动，在 Android/移动网络断线时不丢已识别内容，并避免服务端语音流状态卡死或浪费时长。

**Architecture:** 前端把一次语音输入拆成“用户语音会话”和“Registry speech stream”两层：用户会话保存 base text、committed transcript 和当前 stream live transcript；Registry stream 可以因断线重建。服务端在 `finish` 时立刻释放 active 状态，同时短暂保留 closing route，只转发 final transcript/error，避免下一次 start 被旧 stream 阻塞。

**Tech Stack:** React 19 / TypeScript / Web Audio API / Browser WebSocket / Go Registry / Volcengine streaming ASR / Jest / Go test。

---

## 文件结构

- 修改 `app/web/src/features/speech/audioCapture.ts`
  - 支持麦克风 track `ended` 和 AudioContext 异常上报。
  - 继续输出 16kHz 16-bit mono PCM。
- 修改 `app/web/src/features/speech/VoiceInputButton.tsx`
  - `pointerdown` 立即调用预热入口。
  - 260ms 长按确认后进入正式语音输入。
  - 短按释放时调用丢弃预热入口。
- 修改 `app/web/src/features/speech/useVoiceInputController.ts`
  - 把单一 voice text 改成 `committedTranscript + liveTranscript`。
  - 同 stream 内允许 live transcript 变短/修正；跨 stream 固化旧 live transcript。
- 修改 `app/web/src/main.tsx`
  - 接入预热麦克风。
  - 断线时不取消整段语音，而是固化 live transcript、清空旧 stream、继续 5 秒本地 buffer。
  - 重连后重新 `speech.start` 并发送后续音频。
  - `speech.error` 默认保留已识别内容；retryable 且仍在录音时重开 stream。
  - `finish` 后进入 finalizing，最多 3 秒等待 final transcript，期间不允许下一次语音输入。
  - 只记录 voice warning/error 诊断事件。
- 修改 `server/internal/registry/speech_service.go`
  - `finish` 立刻从 `connActive` 释放 active stream。
  - 增加 closing route，最多保留 3 秒。
  - closing route 只转发 final transcript 和 error，不转发 interim。
- 修改测试：
  - `app/__tests__/web-voice-input-button.test.tsx`
  - `app/__tests__/web-voice-input-controller.test.ts`
  - `app/__tests__/web-voice-input-local-buffer.test.ts`
  - `server/internal/registry/speech_test.go`

## 状态机

### 前端用户语音会话

- `idle`: 没有语音输入。
- `prewarming`: pointerdown 后立刻启动麦克风，但长按未成立；PCM 暂不写入输入框，不发 Registry。
- `buffering`: 长按成立，麦克风已启动，Registry stream 未 ready；PCM 进入 5 秒本地 buffer。
- `recording`: stream ready；PCM 串行发送 `speech.chunk`。
- `reconnect_buffering`: 录音中 Registry WebSocket 断开；保留已识别内容，继续缓存最多 5 秒，重连后开新 stream。
- `finalizing`: 用户松手 finish；麦克风停止，drain queue，发 `speech.finish`，最多 3 秒等待 final transcript。
- `done`: 收到 final 或 final 等待超时，保留当前输入框内容并清理状态。
- `cancelled`: 用户上滑取消，恢复 base text，尽量 cancel 当前 stream。

### 服务端 speech stream

- `active`: 收 chunk 并转给火山。
- `closing`: 已收到 finish，立即释放 `connActive`，最多 3 秒保留 event route。
- `closed`: final/error/closing timeout/cancel/disconnect 后完全删除。

## Corner Cases

- iOS 短按触发权限弹窗：允许；短按不正式录入，不改输入框。
- iOS 长按：麦克风启动必须在 pointerdown 用户手势链路中完成。
- Android WebSocket 断开：不回滚输入框，不停止麦克风；固化当前 live transcript，清空旧 stream，继续 buffer。
- 断线超过 5 秒：停止麦克风，保留已识别内容，提示连接中断。
- 同一个 stream 的 transcript 变短：允许覆盖 live transcript。
- 新 stream 的 transcript：只能更新新的 live transcript，不能覆盖 committed transcript。
- 用户上滑取消：回滚整段语音输入，包括 committed transcript。
- `speech.error` retryable：如果仍在录音，按 stream 断开处理并尝试重连；否则保留已识别内容并结束。
- `speech.error` non-retryable：停止录音，保留已识别内容，显示错误。
- `finish` 后没有 final：3 秒后保留当前文本并清理，不阻塞下一次语音。
- 服务端 closing route 收到 interim：忽略。

## Task 1: 前端 transcript 分段模型

**Files:**
- Modify: `app/web/src/features/speech/useVoiceInputController.ts`
- Test: `app/__tests__/web-voice-input-controller.test.ts`

- [ ] **Step 1: Write the failing test**

```ts
test('commits current stream transcript before applying a new stream transcript', () => {
  const session = createVoiceInputSession('prefix suffix', 7, 7);
  expect(session.applyTranscript('你好')).toBe('prefix 你好suffix');
  session.commitLiveTranscript();
  expect(session.applyTranscript('世界')).toBe('prefix 你好世界suffix');
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `npm test -- --runTestsByPath __tests__/web-voice-input-controller.test.ts`
Expected: FAIL because `commitLiveTranscript` is not implemented.

- [ ] **Step 3: Write minimal implementation**

Add `commitLiveTranscript`, `currentTranscriptText`, and make `applyTranscript` render `committed + live`.

- [ ] **Step 4: Run test to verify it passes**

Run: `npm test -- --runTestsByPath __tests__/web-voice-input-controller.test.ts`
Expected: PASS.

## Task 2: iOS pointerdown microphone prewarm

**Files:**
- Modify: `app/web/src/features/speech/VoiceInputButton.tsx`
- Test: `app/__tests__/web-voice-input-button.test.tsx`

- [ ] **Step 1: Write the failing test**

```tsx
test('prewarms voice input immediately on pointer down before long press settles', () => {
  const onPrewarmStart = jest.fn();
  renderVoiceButton({onPrewarmStart});
  pointerDownVoiceButton();
  expect(onPrewarmStart).toHaveBeenCalledTimes(1);
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `npm test -- --runTestsByPath __tests__/web-voice-input-button.test.tsx`
Expected: FAIL because the prop does not exist.

- [ ] **Step 3: Write minimal implementation**

Add `onPrewarmStart` and `onPrewarmCancel` props. Call `onPrewarmStart` synchronously in pointerdown before the long-press timer. Call `onPrewarmCancel` when pointer ends before start is requested.

- [ ] **Step 4: Run test to verify it passes**

Run: `npm test -- --runTestsByPath __tests__/web-voice-input-button.test.tsx`
Expected: PASS.

## Task 3: 前端断线续录和 5 秒 buffer

**Files:**
- Modify: `app/web/src/main.tsx`
- Test: `app/__tests__/web-voice-input-local-buffer.test.ts`

- [ ] **Step 1: Write the failing wiring test**

Assert `main.tsx` contains the agreed events and lifecycle hooks:

```ts
expect(mainTsx).toContain('handleVoiceRegistryClosedDuringInput');
expect(mainTsx).toContain("logVoiceInputDiagnostic('warn', 'registry_closed_during_voice'");
expect(mainTsx).toContain("logVoiceInputDiagnostic('warn', 'voice_reconnect_buffering'");
expect(mainTsx).toContain('commitLiveTranscript()');
expect(mainTsx).toContain('clearVoiceStreamStateForReconnect');
```

- [ ] **Step 2: Run test to verify it fails**

Run: `npm test -- --runTestsByPath __tests__/web-voice-input-local-buffer.test.ts`
Expected: FAIL because the reconnect lifecycle is not implemented.

- [ ] **Step 3: Write minimal implementation**

Refactor close/error handling:

- On close with active voice, call `handleVoiceRegistryClosedDuringInput`.
- Commit current live transcript.
- Stop only old queue/stream id, keep microphone and user session.
- Continue buffering up to 5 seconds.
- Silent reconnect opens a new `speech.start`.
- Buffer overflow stops microphone and keeps input text.

- [ ] **Step 4: Run test to verify it passes**

Run: `npm test -- --runTestsByPath __tests__/web-voice-input-local-buffer.test.ts`
Expected: PASS.

## Task 4: speech.error 保留已识别内容

**Files:**
- Modify: `app/web/src/main.tsx`
- Test: `app/__tests__/web-voice-input-local-buffer.test.ts`

- [ ] **Step 1: Write the failing wiring test**

```ts
expect(mainTsx).toContain("logVoiceInputDiagnostic('error', 'speech_error_event'");
expect(mainTsx).toContain('handleVoiceSpeechErrorEvent');
expect(mainTsx).toContain('finishVoiceInputPreservingTranscript');
```

- [ ] **Step 2: Run test to verify it fails**

Run: `npm test -- --runTestsByPath __tests__/web-voice-input-local-buffer.test.ts`
Expected: FAIL because current error handling calls `session.cancel()`.

- [ ] **Step 3: Write minimal implementation**

Replace default speech error handler with preserving behavior. Retryable errors while recording reuse reconnect flow; non-retryable errors preserve current text and clear voice state.

- [ ] **Step 4: Run test to verify it passes**

Run: `npm test -- --runTestsByPath __tests__/web-voice-input-local-buffer.test.ts`
Expected: PASS.

## Task 5: finish finalizing 3 秒等待

**Files:**
- Modify: `app/web/src/main.tsx`
- Test: `app/__tests__/web-voice-input-local-buffer.test.ts`

- [ ] **Step 1: Write the failing wiring test**

```ts
expect(mainTsx).toContain('VOICE_INPUT_FINAL_WAIT_MS = 3000');
expect(mainTsx).toContain('voiceAwaitingFinalRef');
expect(mainTsx).toContain('scheduleVoiceFinalTimeout');
expect(mainTsx).toContain('completeVoiceInputFinalizing');
```

- [ ] **Step 2: Run test to verify it fails**

Run: `npm test -- --runTestsByPath __tests__/web-voice-input-local-buffer.test.ts`
Expected: FAIL because finish currently does not wait for final.

- [ ] **Step 3: Write minimal implementation**

After `speech.finish`, set finalizing state and wait up to 3 seconds. Final transcript or timeout calls one cleanup function. `startVoiceInput` rejects new starts while finalizing.

- [ ] **Step 4: Run test to verify it passes**

Run: `npm test -- --runTestsByPath __tests__/web-voice-input-local-buffer.test.ts`
Expected: PASS.

## Task 6: 服务端 finish closing route

**Files:**
- Modify: `server/internal/registry/speech_service.go`
- Test: `server/internal/registry/speech_test.go`

- [ ] **Step 1: Write the failing tests**

Add tests:

- `TestSpeechFinishKeepsClosingRouteForFinalTranscript`
- `TestSpeechFinishClosingRouteIgnoresInterimTranscript`
- `TestSpeechFinishClosingRouteExpires`

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/registry -run "TestSpeechFinish(KeepsClosingRouteForFinalTranscript|ClosingRouteIgnoresInterimTranscript|ClosingRouteExpires)"`
Expected: FAIL because final transcript after finish is currently dropped.

- [ ] **Step 3: Write minimal implementation**

Add closing stream map with timeout. `handleFinish` detaches active and registers closing route before calling provider `Finish`. `speechStreamEvents.Transcript` routes final transcript through active or closing stream; interim only through active.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/registry -run "TestSpeechFinish(KeepsClosingRouteForFinalTranscript|ClosingRouteIgnoresInterimTranscript|ClosingRouteExpires)"`
Expected: PASS.

## Task 7: 验证

**Files:**
- All modified files.

- [ ] **Step 1: Run focused frontend tests**

Run:

```bash
cd app
npm test -- --runTestsByPath __tests__/web-voice-input-controller.test.ts __tests__/web-voice-input-button.test.tsx __tests__/web-voice-input-local-buffer.test.ts
```

Expected: PASS.

- [ ] **Step 2: Run focused server tests**

Run:

```bash
cd server
go test ./internal/registry
```

Expected: PASS.

- [ ] **Step 3: Run full verification**

Run:

```bash
cd server && go test ./...
cd app && npm test
cd app && npm run tsc:web
cd app && npm run build:web
git diff --check
```

Expected: all exit 0.

## Self Review

- Spec coverage: iOS 预热、Android 断线续录、5 秒 buffer、保留已识别内容、上滑回滚、finish 3 秒 final、服务端 closing route 都有任务覆盖。
- Placeholder scan: 无 TBD/TODO/implement later。
- Type consistency: 前端新增方法集中在 `VoiceInputSession` 和 `VoiceInputButton` props；服务端新增 closing route 只属于 speech service。
