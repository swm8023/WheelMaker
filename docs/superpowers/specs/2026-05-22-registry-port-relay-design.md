# Registry 端口中转设计规格

Date: 2026-05-22
Status: ready-for-agent

## 问题陈述

WheelMaker 需要让 App 或浏览器访问某个 Hub 机器上的第三方本地 Web 页面。该页面由第三方服务提供，通常运行在 Hub 本机或 Hub 可访问的固定地址上，并且页面本身只是静态壳，主要能力通过 WebSocket 提供。

直接在 App 中打开 `127.0.0.1:<port>` 不成立，因为 App/WebView 中的 `127.0.0.1` 指向手机或当前浏览器机器，而不是目标 Hub。要求支持 Machine B 这类只主动连接 Registry、没有公网入口的 Hub，因此不能要求外部客户端直接访问 Hub 的本机端口。

目标是让 Registry 充当一个运行态中转站：App 在设置页面打开一个全局单例端口映射，指定 Registry 监听端口、目标 Hub、固定目标 host `127.0.0.1`、目标端口和 6 位访问码。之后 App 或浏览器访问 Registry 上的这个端口，就能通过 Hub 主动建立的数据 tunnel 访问目标第三方页面。

## 目标

让 App 或浏览器通过 Registry 上的一个用户配置端口访问目标 Hub 上的第三方本地 Web 服务。第三方页面使用根路径 `/`、普通 HTTP 静态资源和 WebSocket，不要求 path-prefix 改造。

该功能必须支持目标 Hub 在 NAT 或内网之后的场景：外部客户端只访问 Registry，Hub 通过现有标准 Registry 连接接收控制命令，并主动建立到 Registry relay 端口的数据 tunnel。

## 用户故事

1. 作为 WheelMaker 用户，我想在 App 设置页面打开端口中转，这样可以访问目标 Hub 上的第三方本地 Web 页面。
2. 作为 WheelMaker 用户，我想选择目标 Hub，这样可以访问没有公网入口但已经连上 Registry 的机器。
3. 作为 WheelMaker 用户，我想设置 Registry 监听端口，这样可以用固定端口在 App 或浏览器里打开页面。
4. 作为 WheelMaker 用户，我想设置目标 host 和目标 port，这样 relay 能映射到 Hub 可访问的固定服务地址。
5. 作为 WheelMaker 用户，我想一次只启用一个 mapping，这样当前 relay 端口的目标明确，不会出现多页面互相抢路径。
6. 作为 WheelMaker 用户，我想在设置页生成一个 6 位数字访问码，这样可以把访问地址临时分享给自己当前设备，而不需要改配置文件。
7. 作为 WheelMaker 用户，我希望每次开启、切换目标或重新生成访问码后旧访问码立即失效，这样旧链接不会长期可用。
8. 作为 WheelMaker 用户，我希望设置页显示 `Disabled`、`Opening`、`Up`、`Error` 状态，这样能判断 tunnel 是否真正建立成功。
9. 作为 WheelMaker 用户，我希望 App 内 iframe 和浏览器都访问同一个 relay 地址，这样不用为 App 和 Web 分别设计入口。
10. 作为 WheelMaker 用户，我希望第三方页面可以使用根路径 `/`、绝对静态资源路径和自己的 WebSocket 路径，这样不需要改造第三方静态壳。
11. 作为 WheelMaker 用户，我希望 HTTP 页面和 WebSocket 服务使用同一个 Registry relay 端口，这样第三方页面按普通浏览器访问模型运行。
12. 作为 WheelMaker 用户，我希望第三方 WebSocket 的文本帧、二进制帧都能转发，这样不要求第三方协议必须是 JSON。
13. 作为 WheelMaker 用户，我希望 Hub 侧请求带浏览器风格 `User-Agent`，这样依赖 User-Agent 判断的第三方本地服务可以正常响应。
14. 作为 WheelMaker 用户，我希望关闭 relay 时当前访问立即断开，这样本地服务不会继续暴露。
15. 作为 WheelMaker 用户，我希望切换 Hub 或目标端口时旧 tunnel 被关闭，新 tunnel 成功后状态更新，这样不会把请求转到旧目标。
16. 作为 WheelMaker 开发者，我希望端口中转逻辑集中在一个 server 包中，这样自定义 frame、stream 复用和认证逻辑有清晰边界。
17. 作为 WheelMaker 开发者，我希望现有 Registry JSON envelope 继续只做控制面，这样不会把不透明 HTTP/WS 数据塞进业务协议。
18. 作为 WheelMaker 开发者，我希望 relay 数据面使用自定义二进制 frame，这样可以高效传输 HTTP body 和 WebSocket binary frame。
19. 作为 WheelMaker 开发者，我希望一条 active tunnel 内复用多个 stream，这样页面加载多个静态资源和 WebSocket 时不会串行阻塞。
20. 作为 WheelMaker 开发者，我希望 App 只通过受控 control method 修改全局 relay slot，这样普通数据端口访问者不能修改映射。

## 非目标

- 不支持多个 relay mapping 同时启用。
- 不支持每个用户、每个设备或每个 project 独立 mapping。
- 不支持任意 URL proxy 或每请求动态选择目标。
- 不在 Hub 配置文件中配置 relay mapping。
- 不要求额外域名、子域名或 path-prefix 兼容。
- 不复用 Hub 本机浏览器的 cookie、localStorage、Service Worker 或缓存。
- 不把 HTTP body 或 WebSocket binary frame base64 塞进 Registry JSON envelope。
- 不把 relay 数据端口变成通用 Registry API。

## 术语

- **Control Plane**：现有 Registry `/ws` JSON envelope 连接，负责认证、公开 `relay.enable` / `relay.disable` / `relay.status` / `relay.regenerateAccessCode`，并在 Registry 内部向目标 Hub 下发 `relay.open` / `relay.close`。
- **Data Plane**：用户配置的 Registry relay 端口，同一端口处理 HTTP 和 WebSocket upgrade。
- **Relay Slot**：Registry 内存中的全局单例 mapping，包含 listen port、目标 Hub、目标地址、访问码和状态。
- **Active Tunnel**：目标 Hub 主动连接到 relay 数据端口内部路径后形成的 WebSocket 数据通道。
- **Stream**：Active Tunnel 内通过 `streamId` 区分的一条 HTTP request/response 或一条 WebSocket 连接。
- **External Client**：访问 relay 数据端口的浏览器或 App WebView。
- **Target Service**：Hub 侧 `127.0.0.1:targetPort` 上的第三方本地 Web 服务。

## 架构

```text
App Settings
  -> Registry /ws
     relay.enable(listenPort, hubId, targetHost="127.0.0.1", targetPort, accessCode)

Registry control plane
  -> target Hub existing /ws connection
     relay.open(relayURL, targetHost="127.0.0.1", targetPort, nonce)

Hub
  -> Registry relay data port
     ws://registry-host:<listenPort>/__wheelmaker/relay/hub?nonce=...

Browser / App WebView
  -> http://registry-host:<listenPort>/
  -> ws://registry-host:<listenPort>/ws

Registry relay listener
  -> active tunnel stream frames
  -> Hub tunnel client
  -> http://127.0.0.1:targetPort/
  -> ws://127.0.0.1:targetPort/ws
```

Registry 只允许一个 active relay slot。启用、关闭、切换目标都通过 control plane 完成。Data plane 的外部访问者不能修改 relay slot。

## 包边界

核心实现集中在一个 Go 包：

```text
server/internal/portrelay/
  types.go        # 公共状态、payload、错误、常量
  frame.go        # tunnel frame codec
  controller.go   # registry 侧 slot 状态机和 control method handler
  listener.go     # registry relay HTTP server/listener 生命周期
  auth.go         # 6 位访问码、cookie、限速、内部路径
  mux.go          # registry 侧 HTTP/WS -> stream 复用
  hub_client.go   # hub 侧 relay.open/close 和目标服务转发
```

薄集成点：

- `server/internal/registry/server.go`
  - client allowlist 增加 `relay.enable`、`relay.disable`、`relay.status`、`relay.regenerateAccessCode`。
  - 将公开 `relay.*` 请求交给 `portrelay.Controller`。
  - Controller 负责启动/切换 listener、维护 slot，并向 hub peer 发送内部 `relay.open` / `relay.close` control request。
- `server/internal/hub/reporter.go`
  - hub request switch 增加内部 `relay.open` / `relay.close`。
  - 具体 tunnel client 逻辑委托 `portrelay.HubClient`。
- `server/internal/protocol/port_relay.go`
  - 放 control plane payload struct 和 method 常量。
- App service/UI 文件只负责 typed request 和设置页展示。

不要把 frame codec、stream multiplexing、access code 认证、listener 热切换直接写进 Registry 或 Hub reporter 主文件。

## Registry Slot 状态

状态枚举：

- `Disabled`：relay 未启用，无 listener，无 active tunnel。
- `Opening`：配置已接受，listener 已启动或正在启动，正在等待目标 Hub tunnel。
- `Up`：listener 和 Hub active tunnel 都可用。
- `Error`：relay 配置存在，但 listener 或 tunnel 建立失败。

Slot 数据：

```json
{
  "enabled": true,
  "status": "Opening|Up|Error|Disabled",
  "listenPort": 28810,
  "hubId": "hub-a",
  "targetHost": "127.0.0.1",
  "targetPort": 12345,
  "accessCodeGeneration": 7,
  "relayUrl": "http://registry-host:28810/",
  "tunnelConnectedAt": "2026-05-22T10:00:00Z",
  "error": ""
}
```

`accessCode` 只保存在内存中，不返回给未授权数据端口请求。Control plane status 可以返回掩码或仅返回 generation。App 设置页本地负责显示刚生成的 6 位码；如果当前 slot 已启用但本机没有与 `accessCodeGeneration` 匹配的明文访问码，应显示 unknown 并要求用户显式重新生成，不能从 status 自动生成一个新的本地码。

## Control Plane

### 公开 Client 方法

Client 角色可调用以下公开方法。Registry 按自身状态处理，不按 `projectId` 路由。

#### `relay.enable`

启用或替换全局 relay slot。Registry 收到后不是简单透传：它必须先校验请求、启动或切换 listener、生成 `relayId` 和 `nonce`、记录 slot 为 `Opening`，再通过现有 Hub control connection 向目标 Hub 发送内部 `relay.open`。`relay.enable` 的成功条件以 Hub 数据 tunnel 是否连回 Registry 为准，而不是以 Hub 收到 `relay.open` 为准。

请求：

```json
{
  "listenPort": 28810,
  "hubId": "hub-a",
  "targetHost": "127.0.0.1",
  "targetPort": 12345,
  "accessCode": "483921"
}
```

字段规则：

- `accessCode` 必须是 6 位数字。
- `targetHost` 必须严格等于 `127.0.0.1`。Registry 和 Hub 都必须拒绝 `localhost`、`0.0.0.0`、`::1`、`127.0.0.2`、`127.1.2.3` 等其他写法。
- `targetPort` 和 `listenPort` 必须在 `1..65535`。
- `listenPort` 不允许等于当前 Registry 标准端口。

响应：

```json
{
  "ok": true,
  "status": "Opening",
  "enabled": true,
  "listenPort": 28810,
  "hubId": "hub-a",
  "targetHost": "127.0.0.1",
  "targetPort": 12345,
  "relayUrl": "http://registry-host:28810/",
  "accessCodeGeneration": 7,
  "error": ""
}
```

错误：

- payload 非法：`INVALID_ARGUMENT`
- Hub 不存在：`NOT_FOUND`
- Hub 离线：`UNAVAILABLE`
- listener 启动失败：协议响应成功但 `status="Error"`，`error` 填摘要；如果旧 slot 仍保留，响应显示旧 active slot。
- control request 转发超时：`TIMEOUT`

`relay.enable` 可以等待一个短超时窗口，例如 5-10 秒：

- Hub tunnel 在窗口内连回并通过 nonce 校验：返回 `status="Up"`。
- Hub 已接受 `relay.open` 但 tunnel 未在窗口内连回：返回 `status="Opening"`，App 继续调用 `relay.status`。
- Hub 离线、listener 启动失败或 tunnel 握手失败：返回标准错误或 `status="Error"`。

#### `relay.disable`

关闭当前全局 relay slot。Registry 通知旧目标 Hub 执行内部 `relay.close`，关闭 active tunnel、stream 和 listener。

请求 payload 可以为空对象：

```json
{}
```

响应返回当前 slot 状态，通常为：

```json
{
  "ok": true,
  "status": "Disabled",
  "enabled": false
}
```

#### `relay.status`

查询当前全局 relay slot，不改变状态。

请求 payload 可以为空对象：

```json
{}
```

响应返回当前 slot 快照；不返回访问码明文。

#### `relay.regenerateAccessCode`

更新当前 slot 的 6 位访问码并使旧 cookie 失效，不重建 listener，也不要求 Hub 重连 tunnel。如果当前未启用，返回 `INVALID_ARGUMENT`。

请求：

```json
{
  "accessCode": "483921"
}
```

响应返回当前 slot 快照和新的 `accessCodeGeneration`。

### 内部 Hub 方法

这些方法只由 Registry 发送给 Hub，不作为 App/Web client 的公开 API。

#### `relay.open`

Registry 下发到目标 Hub。

```json
{
  "relayId": "relay_01...",
  "relayURL": "ws://registry-host:28810/__wheelmaker/relay/hub",
  "nonce": "base64url...",
  "targetHost": "127.0.0.1",
  "targetPort": 12345,
  "userAgent": "Mozilla/5.0 ...",
  "openedAt": "2026-05-22T10:00:00Z"
}
```

Hub 收到后：

1. 关闭旧 Hub relay tunnel。
2. 校验 `targetHost` 严格等于 `127.0.0.1`，否则拒绝打开 tunnel。
3. 用 `nonce` 主动连接 `relayURL`。
4. 进入 tunnel read loop。
5. 返回 `ok=true` 只表示已开始连接流程；Registry 以数据 tunnel 握手为准更新 `Up`。

#### `relay.close`

Registry 下发到旧目标 Hub。

```json
{
  "relayId": "relay_01...",
  "reason": "disabled|replaced|shutdown"
}
```

Hub 必须关闭对应 tunnel 和所有目标服务连接。`relayId` 不匹配时可幂等返回 `ok=true`。

## Data Plane Listener

Registry relay listener 使用 `net.Listen` + `http.Server`。同一个 listener 处理：

- 内部 Hub tunnel：`/__wheelmaker/relay/hub`
- 访问码登录：`/__wheelmaker/relay/login`
- 访问码登出：`/__wheelmaker/relay/logout`
- 只读状态：`/__wheelmaker/relay/status`
- 外部 HTTP proxy：所有其他非 Upgrade 请求
- 外部 WebSocket proxy：所有其他 Upgrade 请求

内部路径优先匹配，不能转发给第三方服务。非内部路径必须要求数据端口 cookie 认证通过。

### 登录页

未认证访问普通路径时 303 跳转到访问码登录页，并将原同站相对路径写入 `next`。登录页只显示 WheelMaker Port Relay 标题、6 位访问码输入、提交按钮和错误状态，不显示 Hub、target host 或 target port。

登录提交：

```text
POST /__wheelmaker/relay/login
code=483921
next=/console?tab=relay
```

成功：

- 设置 `wm_port_relay=<signed-session>`。
- Cookie 属性：`HttpOnly`、`SameSite=Lax`、`Path=/`。
- 如果请求是 HTTPS 反代后的 HTTPS，部署层可通过 `X-Forwarded-Proto` 帮助设置 `Secure`；V1 可以在明文本地模式不设置 `Secure`。
- 303 Redirect 到安全的同站相对 `next`；外部 URL 或内部 `__wheelmaker/relay/*` 路径都回退到 `/`。

失败：

- 303 Redirect 到 `/__wheelmaker/relay/login?error=1`，保留安全的同站相对 `next`。
- 同来源连续 5 次失败后冷却 60 秒。

Cookie session 必须绑定：

- relay id
- access code generation
- issued at
- expiry

关闭 relay、切换 mapping 或 regenerate 后，旧 cookie 因 generation 不匹配失效。

## Tunnel Frame 协议

Data tunnel 是 Hub 主动连接 Registry 的 WebSocket。WebSocket message 使用 binary frame。禁止把 HTTP/WS body base64 包进 JSON envelope。

### Frame 布局

```text
0               1               2               3
+---------------+---------------+---------------+---------------+
| magic 'W'     | version 1     | type          | flags         |
+---------------+---------------+---------------+---------------+
| streamId uint32 big-endian                                    |
+---------------+---------------+---------------+---------------+
| metaLen uint32 big-endian                                     |
+---------------+---------------+---------------+---------------+
| payloadLen uint32 big-endian                                  |
+---------------+---------------+---------------+---------------+
| meta JSON bytes ...                                            |
+----------------------------------------------------------------+
| payload raw bytes ...                                          |
+----------------------------------------------------------------+
```

Frame type：

- `1 open`
- `2 headers`
- `3 data`
- `4 close`
- `5 error`
- `6 ping`
- `7 pong`

Flags：

- `0x01`：payload 是 WebSocket text frame。
- `0x02`：payload 是 WebSocket binary frame。
- `0x04`：stream half-close。
- `0x08`：metadata only。

大小限制：

- `metaLen` 最大 64 KiB。
- 单个 `payloadLen` 默认最大 1 MiB。
- 超过限制返回 stream error 并关闭 stream。

### HTTP Stream

Registry 收到外部 HTTP 请求后发送 `open`：

```json
{
  "kind": "http",
  "method": "GET",
  "path": "/assets/app.js",
  "rawQuery": "v=1",
  "headers": {
    "Accept": ["*/*"]
  }
}
```

Hub 连接目标服务：

```text
http://127.0.0.1:targetPort/assets/app.js?v=1
```

Hub 返回响应头：

```json
{
  "kind": "http",
  "status": 200,
  "headers": {
    "Content-Type": ["application/javascript"]
  }
}
```

HTTP body 用 `data` frame 流式返回。结束时发送 `close`。

### WebSocket Stream

Registry 收到外部 WebSocket upgrade 后发送 `open`：

```json
{
  "kind": "websocket",
  "method": "GET",
  "path": "/ws",
  "rawQuery": "",
  "headers": {
    "Sec-WebSocket-Protocol": ["..."]
  }
}
```

Hub 对目标服务发起 WebSocket dial：

```text
ws://127.0.0.1:targetPort/ws
```

目标 WS 握手成功后，Hub 发送 `headers`：

```json
{
  "kind": "websocket",
  "status": 101,
  "headers": {
    "Sec-WebSocket-Protocol": ["..."]
  }
}
```

之后：

- 外部 text message -> Registry `data(flags=text)` -> Hub target WS text message。
- 外部 binary message -> Registry `data(flags=binary)` -> Hub target WS binary message。
- 任一侧 close -> `close` frame -> 另一侧 close。

## Header 规则

不得跨端透传 hop-by-hop headers：

- `Connection`
- `Upgrade`
- `Keep-Alive`
- `Proxy-Authenticate`
- `Proxy-Authorization`
- `TE`
- `Trailer`
- `Transfer-Encoding`

Registry 自己的 relay auth cookie 不能被第三方服务覆盖。至少过滤同名 `Set-Cookie: wm_port_relay=...`。

Hub 侧请求目标服务时设置固定浏览器风格 `User-Agent`。V1 不强制伪装 `Origin`、`Referer`、浏览器 cookie jar 或其他浏览器环境。

## 热切换

启用或切换流程：

1. Validate request。
2. 如果新 listen port 和当前不同，先创建新 listener，但不立即关闭旧 listener。
3. 更新 slot 为 `Opening`，生成新 `relayId` 和 `nonce`。
4. 向目标 Hub 发送 `relay.open`。
5. 等待 Hub 连接新 listener 的内部 tunnel path。
6. Tunnel nonce 匹配后状态变为 `Up`。
7. 关闭旧 tunnel；如果旧 listener 不再使用，关闭旧 listener。

失败规则：

- 新 listener 创建失败：旧 slot 保持不变，响应返回旧 slot 和错误摘要。
- Hub 不在线：不启动新 slot，返回 `UNAVAILABLE`。
- Hub 未在超时时间内连接 tunnel：新 slot 进入 `Error`，旧 tunnel 必须关闭，避免错转。
- Active tunnel 运行中断开：slot 进入 `Error`，外部 stream 全部关闭。

关闭流程：

1. Slot 状态设为 `Disabled`。
2. 通知 Hub `relay.close`。
3. 关闭 active tunnel 和 stream。
4. 关闭 listener。
5. 清理访问码 session generation。

## App UI

Settings 增加 `Port Relay` detail 页面，并配合桌面左侧工具栏、移动端悬浮按钮打开内嵌 iframe。桌面端 `Port Relay` 是常用工作区切换项，放在左侧 primary activity bar 的 `Git` 下方，而不是和 `Update` / `Skills` / `Settings` 放在 secondary 设置区。

页面状态：

- 从 `relay.status` 初始化。
- 打开页面时如果 relay 未启用且本地没有 6 位访问码，前端可以预生成；如果 relay 已启用但本机不知道当前 `accessCodeGeneration` 对应的明文，访问码显示为 unknown。
- 在访问码 unknown 时，Copy 和切换 target 都不能使用本地随机码继续操作；用户必须点击 Generate/Reset，App 调用 `relay.regenerateAccessCode` 后才恢复可复制和可切换状态。
- App 在本地 global workspace state 中保存多个 target preset。每条 preset 只包含 `{hubId, targetPort}`，不保存 `listenPort`、访问码或 enabled 状态。
- Registry 仍然只维护一个 active relay slot；App 切换 preset 时继续调用现有 `relay.enable` 替换当前 slot，不新增协议方法。
- 顶部统一配置 `Listen Port`、访问码、`Enable` / `Disable` 和运行状态。`Listen Port` 改动不自动重启 relay，用户再次点击 `Enable` 时才应用。
- target 列表在顶部配置下方展示。每行左侧是单选 checkbox，中间显示 Hub 和端口，右侧 `X` 删除；最后一行始终是新增行，没有删除按钮，端口默认 `80`。
- 已 enabled 时切换 checkbox 立即调用 `relay.enable` 切换到新 target；未 enabled 时只更新本地 selected target。
- 删除当前 selected/active target 时，如果 relay 正在 enabled，App 自动调用 `relay.disable`。
- `relay.status` 返回的 active target 如果不在本地 preset 列表中，App 自动补入并选中它，以保持 UI 和 Registry 实际状态一致。
- 设置页不显示可编辑 target host。启用时固定发送 `targetHost="127.0.0.1"`。
- `Target` 只读显示为 `<hubId> -> 127.0.0.1:<targetPort>`。
- 设置页不提供 `Open` 按钮。桌面端复用左侧 `Port Relay` 按钮打开/关闭右侧 iframe；移动端在 relay Up 后显示悬浮按钮打开/关闭全屏 iframe。
- 桌面端点击 `Enable` 后，App 记录一次 auto-open 意图：如果响应已经是 `Up`，右侧立即切到 iframe；如果响应仍是 `Opening`，App 静默轮询 `relay.status`，等状态变成 `Up` 后自动打开。移动端仍只显示悬浮按钮，不自动弹全屏 iframe。
- Chat 消息中的 `http://localhost:<port>` 和 `http://127.0.0.1:<port>` 链接作为本地服务入口处理。用户点击后，App 使用当前选中项目的 `hubId` 自动新增/选中 `{hubId, targetPort}` preset，启用或切换 relay，并把右侧/移动端 iframe 打开到原链接的 path、query、hash。普通外部链接和 `https://localhost` 不走 relay。
- 如果当前 relay 已启用但本机不知道当前 `accessCodeGeneration` 对应的明文访问码，Chat 本地链接不能静默覆盖 code；App 打开 Port Relay 设置页并要求用户先 Generate/Reset。
- iframe URL 使用当前 registry address 推导 relay URL：
  - Registry address 是 `wss://host/ws` 或 `https://host` 时，iframe 使用 `https://host:<listenPort>/`。
  - Registry address 是 `ws://host/ws` 或 `http://host` 时，iframe 使用 `http://host:<listenPort>/`。
  - 如果原地址包含端口，替换为 `listenPort`。
  - 如果 `relay.status` / `relay.enable` 返回的 `relayUrl` 是 `127.0.0.1` / `localhost` / `::1`，但当前 registry address 是非 loopback 地址，App 使用当前 registry address 重新推导 URL。这主要兜底代理未透传 `Host` / `X-Forwarded-*` 时 Registry 返回 loopback URL 的情况；在 Windows EXE embedded 本地连接 registry 时仍保留 loopback URL。

## 错误处理

Control plane 错误使用 Registry 标准错误结构：

- `INVALID_ARGUMENT`
- `UNAUTHORIZED`
- `FORBIDDEN`
- `NOT_FOUND`
- `UNAVAILABLE`
- `TIMEOUT`
- `INTERNAL`

Data plane 错误：

- 未认证：303 跳转到 PIN 登录页。
- relay disabled：503。
- tunnel not up：503。
- stream open 超时：504。
- target 连接失败：502。
- frame 协议错误：关闭 tunnel，slot 进入 `Error`。

错误页保持纯文本或最小 HTML，不暴露 token、nonce、access code 或内部 stack。

Data plane 兼容性：

- Registry 不改写第三方 HTML 内容，不注入 viewport meta 或样式。
- 如果目标页面缺少移动端 viewport 或自身 CSS 设置固定最小宽度，页面内部可能横向滚动；这属于目标页面适配问题。

## 可观测性

Registry log：

- `relay.enable` / `relay.disable` / `relay.regenerateAccessCode` / `relay.status`
- listener start/stop 和端口
- selected hub、固定 target host 和 target port
- tunnel connected/disconnected
- active stream count
- auth failures 计数和冷却

Hub log：

- `relay.open` / `relay.close` 收到
- tunnel dial 成功/失败
- target HTTP/WS 连接成功/失败
- stream open/close/error

禁止记录：

- access code 明文
- cookie
- nonce
- HTTP body
- WebSocket payload
- 第三方页面内容

## 测试策略

核心包测试：

- frame codec 编解码、非法 magic/version/length。
- stream multiplexing 同时处理多个 HTTP request。
- WebSocket text/binary frame 双向转发。
- access code 成功、失败、限速、generation 失效。
- listener 热切换成功和失败路径。
- tunnel nonce 校验。
- tunnel 断开后关闭所有 stream。
- hop-by-hop header 过滤。
- browser User-Agent 设置。

Registry 集成测试：

- client role allowlist 允许 `relay.enable`、`relay.disable`、`relay.status`、`relay.regenerateAccessCode`。
- hub/monitor role 不能修改 relay slot。
- unknown hub 返回 `NOT_FOUND`。
- offline hub 返回 `UNAVAILABLE`。
- `relay.enable` 下发内部 `relay.open` 给目标 Hub，并以 tunnel 连回结果更新响应状态。
- `relay.disable` 下发内部 `relay.close` 给旧 Hub。
- `relay.status` 返回当前 slot。

Hub reporter 测试：

- `relay.open` 创建 Hub tunnel client。
- `relay.close` 关闭匹配 relay id。
- `relay.close` relay id 不匹配时幂等。
- target request 带固定 User-Agent。

App 测试：

- repository/service 发送 `relay.status`、`relay.enable`、`relay.disable`、`relay.regenerateAccessCode`。
- Settings detail 包含 Port Relay 页面，target preset 列表默认新增端口为 80，不显示 Target Host 输入。
- App 本地持久化多个 `{hubId, targetPort}` preset 和 selected target。
- 已启用时切换 target checkbox 会调用 `relay.enable` 替换当前 active slot。
- 6 位访问码生成。
- 状态 `Disabled/Opening/Up/Error` 展示。
- iframe URL 从 registry address 和 listen port 推导。
- 设置页不显示 `Open` 按钮。

## 验收标准

1. 用户可以在 Settings 中打开 Port Relay 页面。
2. 用户可以设置 listen port、生成 6 位访问码，并在 target 列表中添加多个 Hub local port preset。
3. 启用 relay 后，Registry 通过现有控制连接通知目标 Hub 建立数据 tunnel。
4. Tunnel 建立成功后，设置页显示 `Up` 和可访问地址。
5. 浏览器访问 relay 端口时，未认证用户看到访问码登录页。
6. 输入正确访问码后，浏览器能加载 Hub 目标端口上的 `GET /` 页面。
7. 页面发起 `GET /assets/...` 等资源请求时，能通过同一 active tunnel 成功加载。
8. 页面发起 `ws://host:<listenPort>/ws` 或同源 WebSocket 时，能连接到 Hub 目标服务的对应 WS path。
9. WebSocket text frame 能双向转发。
10. WebSocket binary frame 能双向转发。
11. Hub 侧目标服务能看到浏览器风格 `User-Agent`。
12. 关闭 relay 后，relay 端口不可继续访问目标服务，既有 stream 被关闭。
13. 切换 target checkbox 后，旧 tunnel 不再接收新请求，同一个 relay 端口只指向一个 active target。
14. Machine B 只主动连接 Registry 时，仍能作为 relay 目标 Hub 被访问。
15. Registry 和 Hub 都拒绝任何非 `127.0.0.1` 的 target host。

## 实施顺序

1. 建立 `server/internal/portrelay` 包和 frame codec 测试。
2. 实现 registry 侧 controller/listener/auth 的纯包测试。
3. 接入 Registry 公开 `relay.*` allowlist，并从 `relay.enable` / `relay.disable` 驱动 Hub 内部 control request。
4. 实现 Hub `relay.open/close` wiring 和 hub tunnel client。
5. 做 HTTP request 转发 smoke。
6. 做 WebSocket text/binary 转发 smoke。
7. 增加 App service 方法。
8. 增加 Settings Port Relay 页面。
9. 更新协议文档中 client command 列表和版本历史。

## Open Decisions

None. 单例 slot、App 设置来源、6 位访问码、Hub 主动 tunnel、自定义二进制 frame、同端口 HTTP/WS、只伪装 User-Agent、核心逻辑集中在 `server/internal/portrelay` 均已确定。
