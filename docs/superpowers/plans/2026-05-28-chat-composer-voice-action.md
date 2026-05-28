# Chat 输入区语音复合按钮实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**目标：** 实现已确认的 Chat composer 复合动作按钮：无内容点击进入锁定录音，有内容短按发送、有内容长按进入锁定录音，录音中再次点击停止；发送路径不触发麦克风权限；录音期间保持键盘状态并只修改本次语音片段。

**架构：** Registry speech 协议不变。`VoiceInputButton` 负责 pointer 手势和视觉状态；`main.tsx` 负责发送、语音生命周期、textarea 手动编辑拦截和现有语音 session 写入。继续沿用现有本地 buffer、串行发送队列、断线重连和 final transcript 流程。

**技术栈：** React 19, TypeScript, Jest/react-test-renderer, Web Audio API, existing Registry speech client, CSS.

---

## 文件结构

- 修改 `app/web/src/features/speech/VoiceInputButton.tsx`
  - 新增 `hasSendableContent`、`recordingMode`、`onSend`。
  - 有内容短按发送；有内容长按启动 `onStart('locked')`。
  - 无内容点击启动 `onStart('locked')`。
  - 录音态点击 `onFinish`。
  - 移除上滑取消和松手结束行为。
- 修改 `app/__tests__/web-voice-input-button.test.tsx`
  - 覆盖短按发送、长按启动、点击录音、再次点击停止、pointer cancel 不结束、权限弹窗导致 pointer 释放仍保持录音。
- 修改 `app/web/src/main.tsx`
  - 派生 `chatComposerHasSendableContent`。
  - 接入 `VoiceInputInteractionMode` state/ref。
  - 移除 pointerdown 麦克风预热。
  - `startVoiceInput` 默认 `locked`。
  - textarea DOM 只在 sending 时 readonly，录音/等待 final 时通过事件拦截阻止手动编辑。
- 修改 `app/web/src/styles.css`
  - 增加发送按钮上的麦克风 badge。
  - 增加录音按钮动画状态。
- 修改 `app/__tests__/web-voice-input-local-buffer.test.ts`
  - 固化 main wiring、默认 locked mode、无 prewarm props。
- 修改 `app/__tests__/web-chat-ui.test.ts`
  - 更新 composer、textarea guard 和 CSS 断言。

## Task 1: Button State Machine

**Files:**
- Modify: `app/__tests__/web-voice-input-button.test.tsx`
- Modify: `app/web/src/features/speech/VoiceInputButton.tsx`

- [x] **Step 1: 写失败测试**

覆盖：

- 有可发送内容时短按调用 `onSend`，不调用 `onStart`。
- 有可发送内容时长按调用 `onStart('locked')`，松手不 `finish`。
- 无内容点击调用 `onStart('locked')`，松手不 `finish`。
- 录音态再次 pointer down 调用 `onFinish`。
- async start 未完成时 pointer up/cancel 不触发 finish/cancel。
- pointer move 不产生取消意图。

- [x] **Step 2: 验证 red**

已运行：

```bash
cd app
npm test -- --runTestsByPath __tests__/web-voice-input-button.test.tsx
```

结果：按预期失败，旧实现仍会 `hold`、松手结束或上滑取消。

- [x] **Step 3: 实现按钮行为**

实现：

- `VoiceInputInteractionMode = 'locked' | 'hold'` 保留类型兼容，当前交互只从按钮启动 `locked`。
- `sendWithVoice` 短按发送。
- 长按阈值启动 locked voice。
- pointer up/cancel 不结束已启动语音。
- 录音态点击结束。

- [x] **Step 4: 验证 green**

已通过 focused button tests。

## Task 2: Main Wiring and Keyboard-Safe Textarea Guard

**Files:**
- Modify: `app/__tests__/web-voice-input-local-buffer.test.ts`
- Modify: `app/__tests__/web-chat-ui.test.ts`
- Modify: `app/web/src/main.tsx`

- [x] **Step 1: 写失败 wiring 测试**

断言：

```ts
const chatComposerHasSendableContent = chatComposerText.trim().length > 0 || chatAttachments.length > 0;
const [voiceInteractionMode, setVoiceInteractionMode] = useState<VoiceInputInteractionMode | null>(null);
const voiceInteractionModeRef = useRef<VoiceInputInteractionMode | null>(null);
const startVoiceInput = async (interactionMode: VoiceInputInteractionMode = 'locked') => {
recordingMode={voiceInteractionMode}
hasSendableContent={chatComposerHasSendableContent}
onSend={() => sendChatMessage().catch(() => undefined)}
readOnly={chatSending}
if (voiceRecordingRef.current) {
```

并断言旧 prewarm props 不存在。

- [x] **Step 2: 验证 red**

已运行：

```bash
cd app
npm test -- --runTestsByPath __tests__/web-voice-input-local-buffer.test.ts __tests__/web-chat-ui.test.ts
```

结果：按预期失败，`main.tsx` 尚未接入新 props。

- [x] **Step 3: 实现 main wiring**

实现：

- 新增 `voiceInteractionMode` state/ref。
- `resetVoiceRecordingUi` 清理 mode。
- `startVoiceInput` 接受 mode，默认 `locked`。
- 移除 microphone prewarm ref/function/JSX props。
- `VoiceInputButton` 接入 sendable、recordingMode、onSend。
- textarea 在录音和等待 final 时阻止 change/paste/keyDown，但保持 `readOnly={chatSending}`。

- [x] **Step 4: 验证 green**

已通过 focused wiring tests。

## Task 3: Visual State

**Files:**
- Modify: `app/web/src/styles.css`
- Modify: `app/__tests__/web-chat-ui.test.ts`

- [x] **Step 1: 写失败样式断言**

断言 CSS 包含：

```ts
.voice-input-button.send-with-voice
.voice-input-badge
.voice-input-button.locked-recording
.voice-input-button.hold-recording
```

- [x] **Step 2: 验证 red**

旧 CSS 缺少 badge 和录音 mode class。

- [x] **Step 3: 实现样式**

实现：

- 发送按钮右下角 mic badge。
- 录音按钮保持 36px 尺寸并添加 pulse 动画。
- reduced motion 下关闭动画。

- [x] **Step 4: 验证 green**

已通过 focused UI tests。

## Task 4: Focused and Full Verification

**Files:**
- All modified files.

- [x] **Step 1: Focused verification**

已通过：

```bash
cd app
npm test -- --runTestsByPath __tests__/web-voice-input-button.test.tsx __tests__/web-voice-input-local-buffer.test.ts __tests__/web-chat-ui.test.ts
```

- [x] **Step 2: Full verification**

已通过：

```bash
cd app
npm test
npm run tsc:web
npm run build:web
cd ../server
go test ./...
cd ..
git diff --check
```

## 自审

- 新交互已以“再次点击停止”为准，不再保留上滑取消和松手结束。
- 发送路径不会调用麦克风。
- iOS 权限弹窗导致 pointer 释放时不会结束录音。
- textarea 录音期间没有使用 readonly 锁键盘。
- 语音插入仍沿用 segment-only session 模型。
