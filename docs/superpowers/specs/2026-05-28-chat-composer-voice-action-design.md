# Chat 输入区语音操作交互迭代设计

日期：2026-05-28

## 背景

当前 Chat 输入区在开启 Voice Input 后，需要把发送和语音合并成一个更符合移动端输入习惯的复合按钮：

- 没有可发送内容时，主动作是语音。
- 有文字或附件时，主动作是发送。
- 有可发送内容时仍能通过长按启动语音。

本文是 `docs/superpowers/specs/2026-05-28-registry-speech-input-design.md` 的 Chat 输入区交互迭代。若语音按钮、发送按钮、录音手势和输入框键盘行为与旧文档冲突，以本文为准。Registry speech 协议和服务端 ASR 接入不在本次迭代范围内。

## 目标

- 让右侧操作按钮根据“是否有可发送内容”在语音和发送之间切换。
- 支持短按锁定录音、再次点击停止。
- 支持有无可发送内容时都可以长按按住说话，松手结束，上移取消。
- 避免非语音路径触发 iOS/Safari 麦克风权限弹窗。
- 语音只修改本次语音插入片段，不覆盖或重写整个输入框。
- 语音开始和结束都尽量保持输入法/键盘展开状态不变。

## 非目标

- 不改变 Registry speech 协议。
- 不改变火山引擎 ASR 接入方式。
- 不新增空闲麦克风 warm stream。
- 不在本次迭代中重做 Chat 输入区整体布局。

## 核心规则

### 可发送内容

按钮状态按“可发送内容”判断，而不是只按输入框文字判断：

- `chatComposerText.trim()` 非空：有可发送内容。
- 有附件：有可发送内容。
- 两者都没有：无可发送内容。

附件-only 场景也显示发送按钮，避免用户已添加附件却找不到发送入口。

### 按钮形态

复合按钮有四种主要视觉/行为形态：

- `voice`：无可发送内容时显示纯麦克风按钮。
- `sendWithVoice`：有可发送内容时显示发送按钮，并在右下角叠加小麦克风 badge，表示长按可语音。
- `recordingLocked`：短按启动后的锁定录音态，按钮持续录音动画，再次点击停止。
- `recordingHold`：长按启动后的按住说话态，松手结束；上移超过取消阈值后，松手取消本次语音输入。

### 触发行为

无可发送内容时：

- 点击：启动锁定录音。
- 松手不会结束录音。
- 再次点击录音按钮：停止录音。
- 长按达到 `260ms` 阈值：启动按住说话。
- 按住说话松手：停止录音并保留识别文本。
- 按住说话上移超过取消阈值后松手：取消本次语音输入，并恢复语音开始前的输入框内容。

有可发送内容时：

- `260ms` 内松开：发送消息。
- 达到 `260ms` 长按阈值：启动按住说话。
- 长按启动语音后，松手结束语音，不发送原文本。
- 长按启动语音后，上移超过取消阈值再松手：取消本次语音输入，原有文字和附件保持不变。
- 短按发送路径绝不调用 `getUserMedia`，避免 iOS/Safari 在发送时弹麦克风权限。

录音中：

- 锁定录音：再次点击按钮停止录音。
- 按住说话：松手停止录音并保留识别文本。
- 按住说话：上移超过取消阈值后松手取消本次语音输入。
- 停止录音复用现有 `speech.finish -> 最多等待 3 秒 final transcript -> cleanup` 流程。

### iOS/Safari 权限弹窗

麦克风权限弹窗不是应用绘制的 UI，而是浏览器在 `navigator.mediaDevices.getUserMedia({audio: ...})` 时决定是否弹出的系统权限请求。应用只能控制何时调用 `getUserMedia`。

本次设计的原则：

- 只有明确语音路径会调用 `getUserMedia`。
- 有可发送内容的短按发送路径不能调用 `getUserMedia`。
- 录音结束立即停止 tracks 和 AudioContext，不保留空闲麦克风流。
- 如果 iOS 权限弹窗打断长按手势，授权成功后降级为锁定录音，不因为用户点击系统弹窗时松手而立即停止。
- 如果用户拒绝授权，退出语音状态，不修改输入框，并显示错误。

Safari 普通标签页支持对站点设置 Microphone 为 Ask/Deny/Allow。若用户设置为 Ask 或 PWA/WKWebView 权限无法稳定持久化，系统可能仍会重复询问；这属于浏览器权限策略，不由应用弹窗实现。

## 文本插入模型

语音开始时锁定一次插入上下文：

- `baseText`
- `insertStart`
- `insertEnd`

插入位置规则：

- 如果 textarea 有焦点并能读取 selection，使用当前 selection。
- 如果 textarea 没有焦点或 selection 不可用，插入到输入框末尾。
- 语音过程中用户移动光标，不改变本次语音片段的位置。

ASR transcript 只替换本次语音片段：

```text
nextText = baseText[0:insertStart] + committedTranscript + liveTranscript + baseText[insertEnd:]
```

跨 Registry stream 重连时，当前 live transcript 会先固化到 committed transcript；新 stream 只能更新新的 live transcript，不能覆盖旧识别结果。用户上移取消按住说话时，恢复语音开始前的 `baseText`。

## 键盘和输入框状态

语音开始、停止都不主动调用 `focus()` 或 `blur()`：

- 原来输入法展开，尽量保持展开。
- 原来输入法未展开，不主动展开。
- 按钮 pointerdown 使用 `preventDefault()`，避免按钮抢焦点导致键盘收起。

录音期间不允许手动编辑输入框，但不能通过 `readOnly` 强制锁定，因为 iOS 可能因 textarea 变为 readonly 而关闭键盘。实现应采用事件拦截/状态判断：

- 录音期间阻止或忽略用户键入、粘贴、删除等手动文本修改。
- ASR 更新仍通过语音 session 写入输入框。
- 输入框焦点和键盘状态尽量保持原样。

如果用户切换项目、会话或当前 Chat 上下文不再匹配，沿用现有安全策略取消本次语音，避免写入错误输入框。

## 组件边界

### VoiceInputButton

当前 `VoiceInputButton` 负责复合按钮的手势和视觉状态，不直接了解 Registry speech 协议。

职责：

- 接收是否有可发送内容、录音状态、禁用状态。
- 在 `sendWithVoice` 模式下短按调用 `onSend`，长按调用 `onStart('hold')`。
- 在 `voice` 模式下点击调用 `onStart('locked')`。
- 在 `voice` 模式下长按达到阈值后调用 `onStart('hold')`。
- 锁定录音态再次点击调用 `onFinish`。
- 按住说话态松手调用 `onFinish`。
- 按住说话态上移超过取消阈值后松手调用 `onCancel`。

### Voice session controller

沿用当前 `VoiceInputSession`：

- 继续负责 `baseText + insertStart + insertEnd` 片段替换。
- 支持 committed/live transcript。
- 跨 stream 重连时提交 live transcript，避免后续 live 结果覆盖旧文本。

### App glue

`main.tsx` 负责把按钮事件连接到现有业务：

- `onSend` -> `sendChatMessage()`
- `onStart(mode)` -> `startVoiceInput(mode)`
- `onFinish` -> `finishVoiceInput()`
- `onCancel` -> `cancelVoiceInput('gesture')`

## 视觉反馈

有可发送内容时，发送按钮是主视觉，右下角麦克风 badge 是辅助提示。badge 不单独成为可点击目标，避免增加命中区域复杂度。

录音中：

- 按钮持续录音动画。
- 锁定录音再次点击按钮停止。
- 按住说话松手停止，上移后底部长条切换为取消反馈。
- 底部长条显示录音状态、连接状态、取消意图和结束等待状态。

首次授权弹窗打断长按手势后，授权成功使用锁定录音视觉。

## Corner Cases

- 有附件但无文字：显示 `sendWithVoice`，短按发送附件，长按语音。
- 有文字时长按进入语音：达到 260ms 后不再触发发送。
- 有文字时短按发送：不调用 `getUserMedia`。
- 无内容点击语音：可以触发麦克风权限；授权成功后进入锁定录音。
- 无内容长按语音：已授权时进入按住说话，松手结束，上移取消。
- 权限弹窗导致 pointer up/cancel：不立即结束录音，授权成功后降级为锁定录音。
- 权限拒绝：退出语音，不修改输入框。
- 录音中 Registry 断线：沿用本地 buffer 和重连策略。
- 录音中手动输入：阻止或忽略，保持语音片段的唯一写入权。
- 录音中切换会话/项目：取消本次语音，避免写错上下文。

## 测试策略

前端测试应覆盖：

- 无可发送内容时点击启动锁定录音，松手不结束，再次点击停止。
- 无可发送内容时长按启动按住说话，松手结束，上移取消。
- 有可发送内容时短按发送，不调用麦克风启动。
- 有可发送内容时长按启动按住说话，不触发发送，松手结束，上移取消。
- 权限弹窗/启动延迟导致 pointer 已释放时，授权成功后降级锁定录音而不是立即 finish。
- pointer cancel 在 hold start 未完成时降级锁定录音；在 hold start 已完成后，如果已经处于上移取消意图则取消，否则按松手结束处理，不发送。
- 录音期间 textarea 不使用 readonly 锁键盘，且手动 onChange 不覆盖语音文本。
- 附件-only 场景显示发送主按钮。
- 语音插入只替换本次语音 segment。

服务端测试无需因本次交互变化新增协议测试；继续依赖已有 speech stream、finish finalizing、断线恢复测试。

## 自审

- 无占位项。
- 本文明确覆盖旧语音输入设计中的按钮交互差异。
- iOS 权限弹窗责任边界明确：应用控制调用时机，弹窗由浏览器决定。
- 键盘保持与录音期间禁止编辑之间没有使用 readonly 的矛盾。
- 发送和语音在有内容复合按钮里的短按/长按判定有明确阈值，长按语音在有无内容时都可用。
