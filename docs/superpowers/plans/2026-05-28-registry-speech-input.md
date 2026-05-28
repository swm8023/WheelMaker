# Registry 语音输入实现计划

> **给执行代理：** 实现本计划时必须使用 `subagent-driven-development`（优先）或 `executing-plans`，按任务逐项执行并更新状态。

**目标：** 在 Chat 中实现用户自带火山引擎 API Key 的豆包流式语音输入。浏览器录音后把音频流发送给 Registry，Registry 代理接入火山引擎豆包流式语音识别 2.0，并把识别文本实时写回当前输入框。

**架构：** 第一版沿用现有 Registry `/ws` JSON envelope。浏览器以 Base64 PCM chunk 发送语音；Registry 负责转换为火山引擎二进制 gzip WebSocket 协议。前端语音设置、录音状态机、音频采集、Registry speech client、录音按钮和录音状态条都放到独立 `features/speech/` 模块；现有 `main.tsx` 只做状态接线和组件挂载。

**技术栈：** Go 1.26、gorilla/websocket、React 19、TypeScript、CSS、Jest、Web Audio API。

---

## 文件结构

- 新增 `server/internal/registry/speech_protocol.go`
  - Registry 本地 speech 方法名、payload struct、脱敏 helper、Volcengine 常量。
- 新增 `server/internal/registry/speech_service.go`
  - 每个 Registry client connection 的 speech stream 生命周期、Base64 解码、finish/cancel、事件回写。
- 新增 `server/internal/registry/speech_volcengine.go`
  - 火山引擎 WebSocket 握手、二进制/gzip frame 编解码、response text 解析。
- 修改 `server/internal/registry/server.go`
  - 初始化 speech service；在主 dispatch 中接入 `speech.*`；disconnect 时清理 stream。
- 修改 `server/internal/protocol/registry_methods.go`
  - 添加 `speech.start/chunk/finish/cancel` 客户端方法描述。
- 新增 `server/internal/registry/speech_test.go`
  - Registry-local dispatch、脱敏、chunk validation、fake provider lifecycle 测试。
- 新增 `server/internal/registry/speech_volcengine_test.go`
  - 火山 frame 编解码和 transcript extraction 测试。
- 新增 `app/web/src/features/speech/speechSettings.ts`
  - 前端 speech settings 类型、默认值、模型选项、脱敏导出。
- 新增 `app/web/src/features/speech/registrySpeechClient.ts`
  - 封装 `speech.start/chunk/finish/cancel` request 和 `speech.transcript/error` event subscription。
- 新增 `app/web/src/features/speech/audioCapture.ts`
  - `getUserMedia`、Float32 到 16kHz mono int16 PCM、Base64 chunk 工具。
- 新增 `app/web/src/features/speech/useVoiceInputController.ts`
  - 长按、上滑取消、release finish、transcript replacement state machine。
- 新增 `app/web/src/features/speech/VoiceInputButton.tsx`
  - composer 右侧 mic button，pointer capture。
- 新增 `app/web/src/features/speech/VoiceRecordingBar.tsx`
  - 录音时替换 composer bottom strip 的计时/波形/取消状态 UI。
- 修改 `app/web/src/services/workspacePersistence.ts`
  - 持久化 `speechSettings`，database dump 中脱敏 API Key。
- 修改 `app/web/src/debug/registryDebug.ts`
  - outbound/inbound debug 中脱敏 `speech.start.apiKey`，摘要化 `speech.chunk.pcm`。
- 修改 `app/web/src/types/registry.ts`
  - 添加 speech payload/event 类型。
- 修改 `app/web/src/services/registryRepository.ts`
  - 添加 speech request 方法。
- 修改 `app/web/src/services/registryWorkspaceService.ts`
  - 添加 speech request 的 Workspace service 代理方法。
- 修改 `app/web/src/main.tsx`
  - Chat 设置挂载、speech controller 接线、send button/mic button 切换、录音条挂载、textarea recording readonly。
- 修改 `app/web/src/styles.css`
  - mic button、recording bar、音量动画、取消状态样式。
- 新增 `app/__tests__/web-speech-settings.test.ts`
  - speech settings 持久化、模型选项、dump 脱敏源结构测试。
- 新增 `app/__tests__/web-speech-client.test.ts`
  - Registry speech client 和 debug redaction 测试。
- 新增 `app/__tests__/web-speech-audio.test.ts`
  - PCM/Base64 工具测试。
- 新增 `app/__tests__/web-voice-input-controller.test.ts`
  - transcript replacement、cancel restore、gesture threshold state 测试。
- 修改 `app/__tests__/web-chat-ui.test.ts`
  - Chat settings 与 composer voice UI source-structure 测试。

---

## 任务 1：保存计划与保护性检查

- [x] 保存本实现计划到 `docs/superpowers/plans/2026-05-28-registry-speech-input.md`。
- [x] 执行未完成标记扫描，预期无匹配。

## 任务 2：Registry speech 协议与脱敏

- [x] 先写失败测试，覆盖 client-only 方法权限和 `speech.start` API Key 脱敏。
- [x] 运行失败测试：

```powershell
go test ./internal/registry -run Speech
```

- [x] 添加 `speech.start`、`speech.chunk`、`speech.finish`、`speech.cancel` 协议描述，仅允许 client 调用。
- [x] 添加 `speech_protocol.go`，包含 payload struct、方法判断和脱敏 helper。
- [x] 重跑 Registry speech 测试并通过。

## 任务 3：Registry stream 生命周期与 fake provider

- [x] 先写失败生命周期测试，覆盖真实 `/ws` 上的 start/chunk/transcript/finish。
- [x] 覆盖 bad Base64 返回 `INVALID_ARGUMENT`。
- [x] 覆盖 websocket disconnect 时取消 stream。
- [x] 实现 `speechProvider`、`speechProviderStream`、`speechEventSink` 接口和 `speechService`。
- [x] 在 `server.go` 初始化 speech service、分发 `speech.*`、disconnect 时清理。
- [x] 重跑 Registry speech 测试并通过。

## 任务 4：火山引擎 frame codec 与 provider

- [x] 先写失败 codec 测试，覆盖 full client request header、gzip JSON payload、audio frame、final audio frame、response parser。
- [x] 运行失败测试：

```powershell
go test ./internal/registry -run Volcengine
```

- [x] 实现 `volcengineSpeechProvider`，使用以下握手头：

```text
X-Api-Key: <user key>
X-Api-Resource-Id: volc.seedasr.sauc.duration
X-Api-Request-Id: <generated id>
X-Api-Sequence: -1
```

- [x] 发送 full client request、audio-only frames、finish final frame。
- [x] read loop 解析 `speech.transcript` 和 `speech.error`。
- [x] 重跑 Volcengine 测试并通过。

## 任务 5：Web speech 设置、持久化与 debug 脱敏

- [x] 先写失败 Jest 测试，覆盖默认设置、模型 resource ID、持久化、database dump 脱敏。
- [x] 覆盖 debug record 中 `speech.start` 不含原始 API Key、`speech.chunk` 不含原始 PCM。
- [x] 实现 `speechSettings.ts`、`workspacePersistence.ts`、`registryDebug.ts`。
- [x] 重跑聚焦 Jest 测试并通过：

```powershell
npm test -- --runTestsByPath __tests__/web-speech-settings.test.ts __tests__/web-speech-client.test.ts --runInBand
```

## 任务 6：Web 音频采集与语音控制器模块

- [x] 先写失败模块测试，覆盖 `floatTo16BitPCM`、`base64FromBytes`、`replaceVoiceSegment`、cancel restore、上滑取消阈值。
- [x] 实现纯工具和 Registry speech client。
- [x] 实现浏览器麦克风采集：`getUserMedia`、Web Audio、重采样到 16kHz mono int16 PCM、Base64 chunk。
- [x] 重跑模块测试并通过：

```powershell
npm test -- --runTestsByPath __tests__/web-speech-audio.test.ts __tests__/web-voice-input-controller.test.ts --runInBand
```

## 任务 7：Chat composer UI 接线

- [x] 先写失败 UI source 测试，覆盖 Chat 设置、语音按钮、录音条、textarea readonly、CSS class。
- [x] 实现 `VoiceInputButton` 和 `VoiceRecordingBar`。
- [x] 在 Chat 设置中添加 Voice Input、Volcengine API Key、Speech Model。
- [x] 开启语音输入后，用 mic button 替换发送按钮。
- [x] 录音期间用 `VoiceRecordingBar` 替换底部 toolbar，并把 transcript 实时写回输入框。
- [x] 松手 finish，上滑 cancel 并恢复录音前文本。
- [x] 重跑 UI 测试并通过：

```powershell
npm test -- --runTestsByPath __tests__/web-chat-ui.test.ts --runInBand
```

## 任务 8：完整验证

- [x] 运行 Go 聚焦测试：

```powershell
go test ./internal/protocol ./internal/registry
```

- [x] 运行前端聚焦测试：

```powershell
npm test -- --runTestsByPath __tests__/web-speech-settings.test.ts __tests__/web-speech-client.test.ts __tests__/web-speech-audio.test.ts __tests__/web-voice-input-controller.test.ts __tests__/web-chat-ui.test.ts --runInBand
```

- [x] 运行 TypeScript 检查：

```powershell
npm run tsc:web
```

- [x] 运行完整 Go 测试：

```powershell
go test ./...
```

- [x] 构建 Web：

```powershell
npm run build:web
```

---

## 自检

- 需求覆盖：用户自带 API Key、Registry 代理、Base64 PCM、火山引擎二进制协议、实时 transcript 写入、上滑取消、录音条 UI、debug 脱敏、代码拆分和测试均已覆盖。
- 方法名一致：`speech.start`、`speech.chunk`、`speech.finish`、`speech.cancel`；事件名 `speech.transcript`、`speech.error`。
- 模型资源 ID：`volc.seedasr.sauc.duration`。
- 第一版音频传输：仍走 Registry JSON envelope + Base64；后续如需要降低带宽开销，可单独升级为二进制 websocket frame。
