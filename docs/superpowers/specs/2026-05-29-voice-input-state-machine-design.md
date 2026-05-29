# 语音输入状态机与音频采集重构设计

日期：2026-05-29

## 背景

当前语音输入已经接入火山引擎豆包流式语音识别 2.0，但客户端和服务端的语音生命周期仍由多组布尔值和 ref 隐式组合，导致一些时序问题难以判断：iOS 权限弹窗会打断手势，Android 上可能短时间断流，finish 后 final 结果可能迟到，服务端 active/closing stream 也可能和客户端状态不同步。

这次重构目标是把客户端、服务端、音频采集和超时策略都收敛成明确模型，减少开始丢字、尾部丢字和状态错位。

## 目标

- 客户端使用明确语音状态机，而不是散落的隐式状态。
- 服务端维持单 connection 单 speech slot 模型，不允许同一连接同时存在多个语音 stream。
- 权限弹窗、待连接、本地缓存、finish、recognizing、cancel/error 都有明确转移规则。
- 音频采集层负责稳定产出 `16kHz / 16-bit / mono` PCM，分包窗口固定为 `200ms`。
- 火山引擎请求字段按当前产品决策固定，不再使用没有消费的返回字段。
- 时间参数收敛为 `1s` 和 `15s` 两档，手势阈值和音频分包窗口单独命名。

## 非目标

- 不改 Nginx。
- 不新增二进制语音 WebSocket。
- 不改为浏览器直连火山引擎。
- 不引入 AudioWorklet。继续使用现有 Web Audio / ScriptProcessor 路线，先修状态与采集时序。
- 不开放 `result_type=single`。当前只支持 `full`。

## 火山引擎请求字段

固定使用豆包流式语音识别 2.0 后付费资源：

- Endpoint：`wss://openspeech.bytedance.com/api/v3/sauc/bigmodel_async`
- Resource ID：`volc.seedasr.sauc.duration`
- Model name：`bigmodel`

Full client request 固定为：

```json
{
  "user": {
    "uid": "wheelmaker"
  },
  "audio": {
    "format": "pcm",
    "codec": "raw",
    "rate": 16000,
    "bits": 16,
    "channel": 1
  },
  "request": {
    "model_name": "bigmodel",
    "enable_itn": true,
    "enable_punc": true,
    "enable_ddc": true,
    "show_utterances": false,
    "result_type": "full",
    "enable_nonstream": true
  }
}
```

字段决策：

- `result_type=full`：客户端把 `result.text` 视为本次语音片段的完整当前文本，直接替换该片段。
- `show_utterances=false`：当前 UI 不消费分句、`definite`、时间轴或服务端音量字段，因此不请求分句。
- `enable_ddc=true`：Chat 输入更需要顺滑文本，而不是严格逐字原文。
- `enable_nonstream=true`：保留二遍识别，提高最终文本质量。
- 不使用豆包返回音量做录音动画；前端动画使用本地 PCM level，避免识别链路延迟。

## 时间常量

只保留两档语音超时：

```text
VOICE_SHORT_TIMEOUT_MS = 1000
VOICE_LONG_TIMEOUT_MS = 15000
```

映射：

- `1s`：重连 retry、状态短等待、轮询间隔。
- `15s`：本地待连接缓存上限、speech start/finish 请求超时、await final、服务端 provider start/finish、服务端 idle、服务端 closing route。

非超时常量：

```text
VOICE_LONG_PRESS_MS = 260
VOICE_AUDIO_CHUNK_MS = 200
```

`VOICE_AUDIO_CHUNK_MS=200` 对应 `16000 * 2 * 1 * 200 / 1000 = 6400` bytes。

## 客户端状态机

客户端主状态：

```text
idle
-> permission_pending
-> mic_starting
-> local_buffering
-> remote_starting
-> streaming
-> finishing
-> recognizing
-> completed
```

异常终态：

```text
cancelled
error_preserve_text
error_restore_text
```

状态含义：

- `permission_pending`：正在触发浏览器麦克风授权。此时不启动服务端 stream，避免授权弹窗期间占用豆包计费。
- `mic_starting`：授权已通过，正在启动 AudioContext 和采样链路。
- `local_buffering`：麦克风已经产出 PCM，但 Registry 或豆包 stream 尚未 ready，本地缓存音频。
- `remote_starting`：正在创建 Registry speech stream。
- `streaming`：本地 PCM chunk 通过串行队列发送给 Registry。
- `finishing`：停止麦克风、flush 尾包、drain 队列、发送 `speech.finish`。
- `recognizing`：等待豆包 final。UI 显示识别整理中，语音按钮无反应，不允许发送当前输入框。
- `completed`：收到 final 或 final 等待超时，提交当前文本并回到 `idle`。

## 权限弹窗与手势规则

- 如果启动语音过程中出现系统权限弹窗，用户松手不视为取消。
- 授权成功后统一降级为 locked 录音模式：点击开始，再点击停止。
- 权限拒绝进入 `error_restore_text`，恢复录音前输入框。
- 没有文字时短按语音按钮启动 locked 录音；有文字时短按发送，长按启动 hold 录音。
- hold 录音松手 finish，上滑取消。
- `recognizing` 期间按钮无反应，UI 保持语音处理中。

## 待连接与本地缓存

开始语音时如果 Registry 尚未连接：

```text
permission_pending/mic_starting
-> local_buffering
-> 每 1s 尝试连接
-> 最多 15s
```

15 秒内连上：

```text
remote_starting
-> streaming
-> flush 本地 buffer
```

15 秒仍未连上：

```text
error_preserve_text
```

如果录音中 Registry 断开：

- 固化当前 live transcript。
- 清理旧 stream 和旧发送队列。
- 麦克风继续采集，本地最多缓存 15 秒。
- 重连后开新 stream 并发送后续 PCM。
- 超过 15 秒仍失败，保留已经识别文本并结束。

## 音频采集与转码器

继续使用 Web Audio：

```text
getUserMedia
-> AudioContext / ScriptProcessor
-> Float32 mono samples
-> stateful resample to 16000Hz
-> Int16 little-endian PCM
-> 200ms chunk
-> Base64 JSON chunk
-> Registry
```

采集器需要明确暴露：

- `onReady`：第一帧音频 callback 已经到达，表示本地采样真实开始。
- `onLevel`：本地 PCM 音量，用于录音动画。
- `onChunk`：输出 200ms PCM chunk。
- `stop({flush:true})`：停止时发送不足 200ms 的尾包。

重采样器必须是有状态的。它保留跨 buffer 的 fractional position，避免 44100Hz 到 16000Hz 这类非整数比例在每个 WebAudio buffer 边界重置。

## Transcript 更新模型

当前固定 `result_type=full`。客户端不做 full/delta 猜测，不做“短文本保留旧文本”的启发式 merge。

规则：

- `final=false`：用服务端返回的 `text` 替换本次语音输入片段。
- `final=true` 且 `text` 非空：用最终 `text` 替换本次语音输入片段并完成。
- `final=true` 且 `text` 为空：不改当前输入框，只结束 `recognizing`。
- `final` 超时：保留当前输入框文本并完成，记录 warning。

## 服务端状态机

每个 Registry connection 只有一个 speech slot：

```text
empty
-> starting
-> active
-> closing
-> empty
```

规则：

- 新 `speech.start` 到达时，先关闭同 connection 的 active 和 closing stream，再创建新 stream。
- `speech.chunk` 只接受当前 active stream。
- `speech.finish` 只接受当前 active stream，成功进入 `closing`。
- `speech.cancel` 可取消 active 或 closing stream。
- `closing` 最多保留 15 秒，用来转发 final 或 final empty。
- provider transcript/error 必须匹配当前 slot 的 stream id；旧 stream 的迟到事件丢弃。
- final empty 也要转发为 `speech.transcript { text: "", final: true }`，让客户端退出 `recognizing`。

## 错误与取消语义

统一为三类：

- 用户主动取消：`cancelled` / `error_restore_text`，恢复 base text。
- 系统错误、断线、provider error：`error_preserve_text`，保留已识别文本。
- 会话切换/上下文不匹配：`error_restore_text` 或取消且不写入新会话，避免串会话。

## 测试策略

前端：

- 音频转码器测试：200ms chunk、stop flush、有状态重采样、onReady。
- 输入控制器测试：`result_type=full` 下短文本直接替换，不做启发式 append。
- 状态/源码 wiring 测试：统一时间常量、recognizing 状态、权限 pending 和本地缓存。
- 录音条测试：显示 Preparing、Connecting、Recording、Finishing、Recognizing。

服务端：

- 火山 full client request 字段测试：DDC 开启、utterances 关闭。
- final empty frame 解析和 read loop 转发测试。
- 新 start 清理旧 active 和 closing stream。
- cancel 支持 closing stream。
- closing route 15 秒超时。

## 验收标准

- iOS 首次授权后不会因为松手直接取消录音。
- UI 只有在本地音频真实开始后进入录音态。
- 开头语音不再因为权限/AudioContext 启动延迟而丢失。
- finish 后进入 `recognizing`，按钮无反应，直到 final 或 15 秒超时。
- 服务端同 connection 不会同时保留多个 speech stream。
- 火山请求不再请求未使用的 utterances，DDC 开启。
- 所有新增测试、Registry 测试、Web 类型检查和构建通过。
