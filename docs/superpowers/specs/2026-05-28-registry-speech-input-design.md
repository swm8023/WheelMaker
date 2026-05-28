# Registry 语音输入设计

日期：2026-05-28

## 背景

WheelMaker 需要在 Chat 中支持流式语音输入，识别服务使用火山引擎豆包流式语音识别 2.0。用户在 Workspace Web UI 中填写自己的火山引擎 API Key。第一版实现应避免调整部署配置，例如不新增 Nginx 路由，也不依赖 Hub。

当前浏览器到 Registry 的连接使用已有 `/ws` JSON envelope。这个路径现在按 JSON 读取消息，不适合直接接收原始二进制音频帧。火山引擎流式 ASR WebSocket 还要求在握手阶段设置 `X-Api-Key`、`X-Api-Resource-Id`、`X-Api-Request-Id`、`X-Api-Sequence` 等自定义 header，而浏览器 WebSocket API 不能设置这些 header。因此，火山引擎连接必须由 Registry 代理。

## 目标

- 在 Chat 中增加语音输入模式：用户按住麦克风按钮时，浏览器把麦克风音频流式发送给 Registry。
- ASR 返回内容实时插入当前 Chat 输入框。
- 用户可以在 Chat 设置中填写自己的火山引擎 API Key。
- 第一版使用豆包流式语音识别 2.0 后付费模式。
- 实现必须拆成边界清晰的文件和模块，避免把大段逻辑塞进现有 Registry 或 UI 大文件。
- 第一版不改 Nginx 和部署配置。

## 非目标

- 不做浏览器直连火山引擎。
- 不放到 Hub、市场、Registry 包发布流程，也不依赖 Desktop bridge。
- 第一版不新增专用的二进制 Registry 语音 WebSocket。
- 在确实新增第二个语音供应商之前，不抽象多供应商语音框架。
- Registry 不持久化 API Key 或录音音频。

## 产品决策

### 设置

在 Chat 设置中新增语音输入配置：

- `Voice Input` 开关。
- `Volcengine API Key` 密码输入框。
- `Speech Model` 下拉选择，目前只有一个选项：
  - Label：`Doubao Streaming ASR 2.0`
  - Resource ID：`volc.seedasr.sauc.duration`

API Key 按本功能已确认的方案存储在浏览器本地持久化设置里。调试数据库导出必须排除或脱敏这个字段，任何诊断展示也必须脱敏。

### 输入框交互

当 `Voice Input` 关闭时，输入框右侧操作按钮保持当前发送按钮。

当 `Voice Input` 开启时，桌面端和移动端的右侧操作按钮都变成麦克风按钮，即使当前已有文本或附件也保持为麦克风。发送仍通过键盘或输入法发送行为完成。这样可以保持桌面端和移动端一致。

麦克风按钮行为：

- 长按开始录音。
- 松手结束录音。
- 上滑取消本次语音输入的全部内容。
- `Esc`、输入框失焦、Registry 断开、或致命流式错误都会取消录音并恢复原输入。

录音期间，输入框底部整条区域应切换成更强的语音录入状态，而不是只改变麦克风图标。这个状态需要展示计时器、音量或声波动画，以及“松手完成 / 上滑取消”一类的提示。上滑超过取消阈值后，松手前就切换到取消状态。

### 识别文本插入

录音开始时捕获：

- `baseText`
- `insertStart`
- `insertEnd`

每次收到 `speech.transcript` 事件时，只替换当前录音对应的那一段文本：

```text
nextText = baseText[0:insertStart] + voiceText + baseText[insertEnd:]
```

ASR 返回内容应被视为当前录音的完整文本，而不是 append-only delta。这样可以避免流式 ASR 修正中间结果时产生重复文本。

最终成功时，保留最终识别文本。取消或流式失败时，恢复 `baseText`。

## 架构

### 数据流

1. 用户在 Chat 设置中开启 `Voice Input` 并填写火山引擎 API Key。
2. 用户在 Chat 中长按麦克风按钮。
3. 浏览器通过 `getUserMedia` 获取麦克风音频。
4. 浏览器把音频转换成 `16kHz`、单声道、signed `int16` PCM。
5. 浏览器把 Base64 PCM chunk 通过已有 `/ws` JSON envelope 发送给 Registry。
6. Registry 使用用户提供的 API Key 作为握手 header，打开火山引擎流式 ASR WebSocket。
7. Registry 把浏览器 JSON chunk 转换成火山引擎要求的二进制/gzip 协议。
8. Registry 把 partial 和 final transcript 事件转发回同一个浏览器连接。
9. 浏览器把返回文本实时替换到当前输入框中的语音片段位置。

### 为什么第一版用 Base64

当前 Registry `/ws` 路径预期接收 JSON，现有 Nginx 也已经代理这个路径。Base64 PCM 放在 JSON 里会增加体积，但 200ms 的 `16kHz` 单声道 `int16` PCM 原始数据约 6.4KB，Base64 后约 8.5KB。按每秒 5 个 chunk 计算，作为 Chat 语音输入 MVP 是可以接受的。

这个方案让第一版可以避免：

- 新增 Nginx 路由。
- 新增第二个 Registry WebSocket endpoint。
- 改造现有 `/ws` reader 以兼容二进制帧。

如果后续发现延迟、CPU 或带宽不可接受，再新增专用二进制语音 endpoint，例如 `/speech/volcengine`，并单独配置代理。

## Registry 协议

语音使用现有浏览器 Registry 连接上的 Registry-local 消息。这些消息不路由到 Hub、project session 或 tool execution。

Client 到 Registry：

- `speech.start`
- `speech.chunk`
- `speech.finish`
- `speech.cancel`

Registry 到 Client：

- `speech.transcript`
- `speech.error`

建议 payload：

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

`speech.chunk` 应快速 ack，并把音频放入内部队列，避免 Registry read loop 被上游火山引擎网络写入阻塞。

## 火山引擎接入

第一版火山引擎实现固定接入豆包流式语音识别 2.0 后付费：

- Endpoint：`wss://openspeech.bytedance.com/api/v3/sauc/bigmodel_async`
- Resource ID：`volc.seedasr.sauc.duration`
- Model name：`bigmodel`
- Language：`zh-CN`
- Audio：`pcm`、`raw`、`16000`、`16-bit`、`mono`
- 开启 ITN 和标点。
- 在有助于最终结果判断时，请求 utterance-level 输出。

Registry 负责封装所有火山引擎协议细节：

- WebSocket 握手 header。
- `X-Api-Key`、`X-Api-Resource-Id`、`X-Api-Request-Id`、`X-Api-Sequence: -1` 的处理。
- Request ID 生成。
- 火山引擎二进制帧编码和解码。
- gzip JSON payload 处理。
- 把上游 partial/final 结果映射成 `speech.transcript`。
- 把上游认证、额度、格式、网络错误映射成 `speech.error`。

Registry 可以记录火山引擎 request ID 和 `X-Tt-Logid` 用于排查问题，但绝不能记录 API Key 或 Base64 音频内容。

## 代码组织

这个功能必须保持隔离。对现有大文件的改动应尽量只做 handler 注册、dispatch 接入、组件挂载等最小接触点。

### Registry

优先在 `server/internal/registry/` 下新增聚焦文件：

- `speech_protocol.go`：speech 方法名、request/response/event struct、脱敏 helper。
- `speech_handler.go`：Registry-local 的 `speech.*` JSON 消息分发。
- `speech_service.go`：stream 生命周期、队列、取消、超时、connection ownership。
- `speech_volcengine.go`：火山引擎 client、握手 header、二进制/gzip 协议、transcript 解析。
- `speech_test.go` 或聚焦的 `speech_*_test.go`：协议、生命周期、脱敏测试。

现有 Registry 文件只负责把 speech dispatch 接入当前 WebSocket envelope 流程。不要把火山引擎协议代码放进现有 Registry 主 server 文件。

### Web UI

优先在类似 `app/web/src/features/speech/` 的 feature 目录下新增聚焦模块：

- `speechSettings.ts`：持久化设置 shape、默认值、mask/export redaction helper。
- `registrySpeechClient.ts`：`speech.start/chunk/finish/cancel` 和事件订阅 wrapper。
- `audioCapture.ts`：麦克风权限、采集生命周期、PCM 转换、chunk sizing。
- `audioWorklet` 文件或等价的小 worker 模块：保证稳定流式采集。
- `useVoiceInputController.ts`：面向 composer 的状态机，处理按压、录音、取消、完成、错误。
- `VoiceInputButton.tsx` 和 `VoiceRecordingBar.tsx`：可视控件和录音状态条。
- 移动端手势逻辑放在 voice controller 或小的 `useVoicePressGesture.ts` helper 中，不嵌进现有 composer 主体。

现有 Chat 文件只挂载新组件并传入当前 composer state handler。实现时应避免把音频采集、Base64 编码、pointer gesture、ASR 协议细节继续塞进 `main.tsx`。

桌面端和移动端应共享同一套 voice-input 状态机。移动端专属展示和 pointer 阈值细节应留在 speech feature 模块内部，不要散落在现有 composer 条件判断里。

## 前端音频细节

使用 `navigator.mediaDevices.getUserMedia({ audio: true })` 获取浏览器麦克风音频。

推荐采集路径：

- `AudioContext`
- `AudioWorklet`，用于稳定音频处理
- 重采样到 `16000Hz`
- 下混到单声道
- 把 Float32 sample 转成 signed little-endian `int16` PCM
- 每约 200ms 产出一个 chunk

浏览器到 Registry 的 JSON chunk 使用原始 PCM bytes 的 Base64。Registry 收到后再解码成 bytes，并写入火山引擎连接。

录音约束：

- MVP 单次录音最长 60 秒。
- 约 300ms 以下视为误触短录音。
- MVP 每个浏览器连接只允许一个 active stream。
- 如果 backpressure 或 Registry 错误导致流不健康，前端立即取消本地录音。

## 错误处理

用户可见错误要短且可操作：

- 缺少 API Key：提示用户填写 Chat speech API key。
- 麦克风权限被拒绝：提示用户允许麦克风访问。
- Registry 断开：取消录音并恢复原输入。
- 火山引擎认证或额度失败：展示供应商错误类别，并让用户能回到设置检查配置。
- 音频不支持：提示当前浏览器无法采集兼容的麦克风音频。
- 等待最终 transcript 超时：如果 Registry 已正常完成，则保留最新 partial；否则恢复 `baseText`。

服务端在 finish、cancel、上游错误、下游断开、超时场景都必须清理 stream。

## 可观测性与安全

- 在 client debug log、server log、database/debug export 路径中脱敏 `speech.start.apiKey`。
- 对 `speech.chunk.pcm` 进行省略或摘要化，只记录 `streamId`、`seq`、byte count、timing 这类元数据。
- Registry 绝不持久化 API Key 或音频。
- API Key 生命周期仅限当前 active Registry stream。
- 不把 transcript event 写入无关 debug stream，除非现有 Chat message flow 已经记录 composer state。

## 测试

Registry 测试：

- Speech dispatch 能识别 `speech.*` 为 Registry-local 消息。
- `speech.start` 在所有诊断视图中都会脱敏 API Key。
- `speech.chunk` 校验 `streamId`、sequence 顺序、Base64 解码错误。
- Stream 生命周期覆盖 finish、cancel、disconnect、上游错误、timeout cleanup。
- 火山引擎 client 通过 fake WebSocket transport 测试 frame 编码/解码和 transcript 映射。

Web UI 测试：

- Chat 设置展示 Voice Input、masked API key field、唯一的 Doubao Streaming ASR 2.0 模型选项。
- Database/debug dump 排除或脱敏 speech API key。
- 开启 Voice Input 后，桌面端和移动端都用麦克风按钮替换发送按钮。
- 长按开始录音，松手完成，上滑取消。
- 录音状态条替换 composer 底部区域，并在超过上滑阈值后切换到取消状态。
- Transcript event 替换 active voice segment，而不是追加导致重复文本。
- Cancel/error 恢复原 composer input。
- PCM chunking 产出 `16kHz` 单声道 `int16` 数据和 Base64 chunk。

手工验证：

- 浏览器只在开始录音时请求麦克风权限。
- 录音过程中 partial transcript 实时出现在输入框中。
- 松手后 final transcript 保留。
- 上滑取消会移除本次录音插入的全部文本。
- App debug export 中看不到原始 API Key。

## 实施阶段

1. 新增 Chat speech settings 的持久化和脱敏。
2. 在默认关闭的开关后面新增浏览器音频采集和 voice-input 状态机。
3. 新增 Registry-local `speech.*` 协议处理，并先用 fake provider 打通本地 UI 流程。
4. 在隔离的 Registry 文件中实现火山引擎 provider。
5. 把实时 transcript 插入接入 Chat composer。
6. 补充桌面端和移动端的聚焦测试及手工验证。

## 风险

- 浏览器音频重采样质量和 AudioWorklet 支持需要真机验证。
- 火山引擎流式响应可能会修正 partial text；当前 replacement model 能处理这种情况，但 transcript parsing 需要确认上游哪个字段最权威。
- Base64 over JSON 对 MVP 可接受，但如果录音很长或并发用户很多，可能会带来额外成本。
- 当前 Web UI 的 composer 代码可能比较集中；实现时只抽取本功能需要的语音相关部分，避免借机做大范围 composer 重写。
