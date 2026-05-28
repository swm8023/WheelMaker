# Chat Composer 长按语音实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**目标：** 更新 Chat 输入区复合动作按钮，让长按语音在“无内容”和“有文字/附件”两种状态下都可用，并支持松手结束、上移取消。

**架构：** Registry speech 协议不变。`VoiceInputButton` 负责 pointer 手势状态机，并向 `main.tsx` 报告 `locked` 或 `hold` 启动；`main.tsx` 负责 speech start/finish/cancel、textarea 编辑拦截和现有语音 session 写入。`VoiceInputSession` 继续负责只替换本次语音插入片段。

**技术栈：** React 19、TypeScript、Jest/react-test-renderer、Web Audio API、现有 Registry speech client、CSS。

---

## 文件结构

- 修改 `app/web/src/features/speech/VoiceInputButton.tsx`
  - 恢复 mode-aware 长按行为。
  - 有内容短按仍然只发送。
  - 有内容和无内容状态下，长按都进入 `hold`。
  - `hold` 松手结束，上移后松手取消。
  - iOS 权限弹窗在 microphone start settle 前打断手势时，降级为 `locked`，避免刚授权就立即结束。
- 修改 `app/__tests__/web-voice-input-button.test.tsx`
  - 把 locked-only 长按断言改成 hold-mode 断言。
  - 覆盖松手 finish、上移 cancel、pending-start 降级 locked。
- 修改 `app/web/src/main.tsx`
  - 把 `onCancel`、`onModeChange`、`onCancelIntentChange` 接回 `VoiceInputButton`。
  - 恢复 `voiceCancelIntent`，传给 `VoiceRecordingBar`。
  - 保留 `chatComposerHasSendableContent`、`onSend`、无 prewarm、textarea edit guard。
- 修改 `app/web/src/features/speech/VoiceRecordingBar.tsx`
  - 恢复 cancel intent 文案。
- 修改 `app/__tests__/web-voice-recording-bar.test.tsx`
  - 恢复 cancel intent 测试。
- 修改 `app/web/src/styles.css`
  - 恢复 button/bar 的 cancel intent 视觉反馈。
  - 保留 send-with-voice badge 和 locked/hold recording class。
- 修改 `app/__tests__/web-chat-ui.test.ts`
  - 断言 main wiring 包含 cancel intent 和 `onCancel={cancelVoiceInputByGesture}`。
- 修改 `app/__tests__/web-voice-input-local-buffer.test.ts`
  - 断言 `startVoiceInput` 默认仍是 `locked`，但按钮长按可调用 `onStart('hold')`。

## Task 1: Button Hold 手势状态机

**Files:**
- Modify: `app/__tests__/web-voice-input-button.test.tsx`
- Modify: `app/web/src/features/speech/VoiceInputButton.tsx`

- [x] **Step 1: 写失败的按钮测试**

更新测试，覆盖这些核心断言：

```tsx
expect(onStart).toHaveBeenCalledWith('hold');
expect(onFinish).toHaveBeenCalledTimes(1);
expect(onCancel).not.toHaveBeenCalled();
```

必须覆盖的场景：

- 有可发送内容时短按调用 `onSend`，不调用 `onStart`。
- 有可发送内容时长按调用 `onStart('hold')`；start settle 后 pointer up 调用 `onFinish`。
- 有可发送内容时长按并上移，调用 `onCancelIntentChange(true)`；start settle 后 pointer up 调用 `onCancel`。
- 无内容短按调用 `onStart('locked')`；pointer up 不结束；录音态再次 pointer down 调用 `onFinish`。
- 无内容长按通过 `onModeChange('hold')` 进入 hold；pointer up 调用 `onFinish`。
- async hold start 在 `onStart` settle 前收到 pointer up，不调用 `onFinish`；start settle 后调用 `onModeChange('locked')`。
- hold start settle 后收到 pointer cancel，且没有 cancel intent 时调用 `onFinish`。

- [x] **Step 2: 运行测试确认 red**

Run:

```bash
cd app
npm test -- --runTestsByPath __tests__/web-voice-input-button.test.tsx
```

预期：FAIL。当前实现仍然把长按当 locked 录音，并忽略 release/cancel。

- [x] **Step 3: 实现按钮状态机**

使用这个 props 形状：

```ts
export type VoiceInputInteractionMode = 'locked' | 'hold';

export type VoiceInputButtonProps = {
  disabled?: boolean;
  readOnly?: boolean;
  hasSendableContent?: boolean;
  recording: boolean;
  recordingMode?: VoiceInputInteractionMode | null;
  onSend?: () => void | Promise<void>;
  onStart: (mode: VoiceInputInteractionMode) => void | Promise<void>;
  onFinish: () => void | Promise<void>;
  onCancel: () => void | Promise<void>;
  onModeChange?: (mode: VoiceInputInteractionMode) => void;
  onCancelIntentChange?: (cancelIntent: boolean) => void;
  onLog?: (entry: VoiceInputDiagnosticEntry) => void;
};
```

实现规则：

- `recording && recordingMode === 'locked'` 时，pointer down 调用 `onFinish`。
- `hasSendableContent` 且 pointer up 发生在 `VOICE_LONG_PRESS_MS` 前时，调用 `onSend`。
- `hasSendableContent` 且长按阈值触发时，调用 `onStart('hold')`。
- 无内容 pointer down 立即调用 `onStart('locked')`。
- 无内容长按阈值在 release 前触发时，调用 `onModeChange('hold')`，后续 release/cancel 按 hold 处理。
- active hold 且无 cancel intent 时，pointer up 或 pointer cancel 调用 `onFinish`。
- active hold 且有 cancel intent 时，pointer up 或 pointer cancel 调用 `onCancel`。
- hold start 仍 pending 时收到 pointer up/cancel，记录 pending action 为 locked fallback；start settle 后调用 `onModeChange('locked')`，不调用 finish/cancel。

- [x] **Step 4: 运行按钮测试确认 green**

Run:

```bash
cd app
npm test -- --runTestsByPath __tests__/web-voice-input-button.test.tsx
```

预期：PASS。

## Task 2: App Wiring 和录音条取消反馈

**Files:**
- Modify: `app/__tests__/web-chat-ui.test.ts`
- Modify: `app/__tests__/web-voice-input-local-buffer.test.ts`
- Modify: `app/__tests__/web-voice-recording-bar.test.tsx`
- Modify: `app/web/src/main.tsx`
- Modify: `app/web/src/features/speech/VoiceRecordingBar.tsx`

- [x] **Step 1: 写失败的 wiring 测试**

断言 `main.tsx` 包含：

```tsx
const [voiceCancelIntent, setVoiceCancelIntent] = useState(false);
onCancel={cancelVoiceInputByGesture}
onModeChange={setVoiceInputInteractionMode}
onCancelIntentChange={setVoiceCancelIntent}
cancelIntent={voiceCancelIntent}
```

断言录音条可以显示取消文案：

```tsx
expect(JSON.stringify(renderer!.toJSON())).toContain('Release to cancel');
```

同时保留这些既有安全约束：

```tsx
expect(mainTsx).toContain('readOnly={chatSending}');
expect(mainTsx).toContain('onSend={() => sendChatMessage().catch(() => undefined)}');
expect(mainTsx).not.toContain('onPrewarmStart={prewarmVoiceCapture}');
expect(mainTsx).not.toContain('onPrewarmCancel={cancelVoicePrewarmCapture}');
```

- [x] **Step 2: 运行 wiring 测试确认 red**

Run:

```bash
cd app
npm test -- --runTestsByPath __tests__/web-chat-ui.test.ts __tests__/web-voice-input-local-buffer.test.ts __tests__/web-voice-recording-bar.test.tsx
```

预期：FAIL。当前 `main.tsx` 已移除 cancel intent plumbing，录音条也不再渲染取消文案。

- [x] **Step 3: 实现 app wiring**

恢复 cancel intent state：

```ts
const [voiceCancelIntent, setVoiceCancelIntent] = useState(false);
```

恢复 mode update helper：

```ts
const setVoiceInputInteractionMode = (mode: VoiceInputInteractionMode) => {
  voiceInteractionModeRef.current = mode;
  setVoiceInteractionMode(mode);
};
```

恢复 gesture cancel handler：

```ts
const cancelVoiceInputByGesture = () => {
  void cancelVoiceInput('gesture');
};
```

传给按钮：

```tsx
<VoiceInputButton
  recording={voiceRecording}
  recordingMode={voiceInteractionMode}
  hasSendableContent={chatComposerHasSendableContent}
  disabled={chatAttachmentUploadPending}
  readOnly={chatSending}
  onSend={() => sendChatMessage().catch(() => undefined)}
  onStart={startVoiceInput}
  onFinish={finishVoiceInput}
  onCancel={cancelVoiceInputByGesture}
  onModeChange={setVoiceInputInteractionMode}
  onCancelIntentChange={setVoiceCancelIntent}
  onLog={logVoiceInputButtonEvent}
/>
```

传给录音条：

```tsx
<VoiceRecordingBar
  status={voiceRecordingStatus}
  cancelIntent={voiceCancelIntent}
  elapsedMs={voiceElapsedMs}
  level={voiceLevel}
/>
```

恢复 `VoiceRecordingBarProps`：

```ts
export type VoiceRecordingBarProps = {
  cancelIntent: boolean;
  elapsedMs: number;
  level?: number;
  status?: 'buffering' | 'recording' | 'finishing';
};
```

渲染：

```tsx
<div className={`voice-recording-bar${cancelIntent ? ' cancel-intent' : ''}`} role="status" aria-live="polite">
  ...
  <span className="voice-recording-text">
    {cancelIntent ? 'Release to cancel' : statusText}
  </span>
</div>
```

- [x] **Step 4: 运行 wiring 测试确认 green**

Run:

```bash
cd app
npm test -- --runTestsByPath __tests__/web-chat-ui.test.ts __tests__/web-voice-input-local-buffer.test.ts __tests__/web-voice-recording-bar.test.tsx
```

预期：PASS。

## Task 3: 取消态视觉反馈

**Files:**
- Modify: `app/web/src/styles.css`
- Modify: `app/__tests__/web-chat-ui.test.ts`

- [x] **Step 1: 写失败的样式断言**

新增断言：

```ts
expect(stylesCss).toContain('.voice-input-button.cancel-intent');
expect(stylesCss).toContain('.voice-recording-bar.cancel-intent');
expect(stylesCss).toContain('.voice-recording-bar.cancel-intent .voice-recording-dot');
```

- [x] **Step 2: 运行 UI 测试确认 red**

Run:

```bash
cd app
npm test -- --runTestsByPath __tests__/web-chat-ui.test.ts
```

预期：FAIL。当前 CSS 已移除 cancel-intent 样式。

- [x] **Step 3: 恢复取消样式**

添加：

```css
.voice-input-button.cancel-intent {
  border-color: color-mix(in srgb, #f85149 82%, var(--border));
  background: color-mix(in srgb, #f85149 14%, var(--panel));
  color: #ff8a82;
}

.voice-recording-bar.cancel-intent {
  border-color: color-mix(in srgb, #f85149 55%, var(--border));
  background: color-mix(in srgb, #f85149 12%, var(--panel));
}

.voice-recording-bar.cancel-intent .voice-recording-dot {
  background: #ff8a82;
  box-shadow: 0 0 0 4px color-mix(in srgb, #f85149 18%, transparent);
}
```

- [x] **Step 4: 运行 UI 测试确认 green**

Run:

```bash
cd app
npm test -- --runTestsByPath __tests__/web-chat-ui.test.ts
```

预期：PASS。

## Task 4: 文档和验证

**Files:**
- Modify: `docs/superpowers/specs/2026-05-28-chat-composer-voice-action-design.md`
- Modify: `docs/superpowers/plans/2026-05-28-chat-composer-voice-action.md`

- [x] **Step 1: 验证 spec 语言**

确认 spec 明确写到：

- 长按语音在有无可发送内容时都可用。
- hold 模式松手结束。
- hold 模式上移后松手取消。
- iOS 权限弹窗打断 pending hold 时降级 locked。
- 有内容短按发送仍然不会调用 `getUserMedia`。

- [x] **Step 2: Focused verification**

Run:

```bash
cd app
npm test -- --runTestsByPath __tests__/web-voice-input-button.test.tsx __tests__/web-voice-input-local-buffer.test.ts __tests__/web-chat-ui.test.ts __tests__/web-voice-recording-bar.test.tsx
npm run tsc:web
```

预期：PASS。

- [x] **Step 3: Full verification**

Run:

```bash
cd app
npm test
npm run build:web
cd ../server
go test ./...
cd ..
git diff --check
```

预期：所有命令 exit 0。Webpack 可能打印既有 bundle-size/Babel 提示，只要 exit code 为 0 即可。

## 自审

- Spec coverage：计划覆盖有内容短按发送、无内容短按 locked、有无内容长按 hold、松手 finish、上移 cancel、iOS pending-start 降级、无 prewarm、textarea guard、取消态视觉反馈。
- Placeholder scan：无占位项。
- Type consistency：`VoiceInputInteractionMode`、`recordingMode`、`onCancel`、`onModeChange`、`onCancelIntentChange` 在测试、组件 props 和 `main.tsx` 中命名一致。
