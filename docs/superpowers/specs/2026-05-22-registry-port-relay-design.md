# Registry 端口中转设计规格

Date: 2026-05-22
Status: Draft

父级 PRD：`docs/superpowers/specs/2026-05-22-registry-port-relay-prd.md`

## 目标

让 App 或浏览器通过 Registry 上的一个用户配置端口访问目标 Hub 上的第三方本地 Web 服务。第三方页面使用根路径 `/`、普通 HTTP 静态资源和 WebSocket，不要求 path-prefix 改造。

该功能必须支持目标 Hub 在 NAT 或内网之后的场景：外部客户端只访问 Registry，Hub 通过现有标准 Registry 连接接收控制命令，并主动建立到 Registry relay 端口的数据 tunnel。

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

- **Control Plane**：现有 Registry `/ws` JSON envelope 连接，负责认证、设置 relay slot、下发 `relay.open` / `relay.close`。
- **Data Plane**：用户配置的 Registry relay 端口，同一端口处理 HTTP 和 WebSocket upgrade。
- **Relay Slot**：Registry 内存中的全局单例 mapping，包含 listen port、目标 Hub、目标地址、访问码和状态。
- **Active Tunnel**：目标 Hub 主动连接到 relay 数据端口内部路径后形成的 WebSocket 数据通道。
- **Stream**：Active Tunnel 内通过 `streamId` 区分的一条 HTTP request/response 或一条 WebSocket 连接。
- **External Client**：访问 relay 数据端口的浏览器或 App WebView。
- **Target Service**：Hub 侧 `targetHost:targetPort` 上的第三方本地 Web 服务。

## 架构

```text
App Settings
  -> Registry /ws
     cmd.portRelay(action=enable, listenPort, hubId, targetHost, targetPort, accessCode)

Registry control plane
  -> target Hub existing /ws connection
     relay.open(relayURL, targetHost, targetPort, nonce)

Hub
  -> Registry relay data port
     ws://registry-host:<listenPort>/__wheelmaker/relay/hub?nonce=...

Browser / App WebView
  -> http://registry-host:<listenPort>/
  -> ws://registry-host:<listenPort>/ws

Registry relay listener
  -> active tunnel stream frames
  -> Hub tunnel client
  -> http://targetHost:targetPort/
  -> ws://targetHost:targetPort/ws
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
  - client allowlist 增加 `cmd.portRelay`。
  - 将 `cmd.portRelay` 交给 `portrelay.Controller`。
  - Controller 需要向 hub peer 发送 `relay.open` / `relay.close` control request。
- `server/internal/hub/reporter.go`
  - hub request switch 增加 `relay.open` / `relay.close` / `relay.status`。
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

`accessCode` 只保存在内存中，不返回给未授权数据端口请求。Control plane status 可以返回掩码或仅返回 generation。App 设置页本地负责显示刚生成的 6 位码；刷新后如果无法恢复明文访问码，应要求用户重新生成。

## Control Plane

### `cmd.portRelay`

Client 角色可调用。Registry 按自身状态处理，不按 `projectId` 路由。

请求：

```json
{
  "action": "status|enable|disable|regenerate",
  "listenPort": 28810,
  "hubId": "hub-a",
  "targetHost": "127.0.0.1",
  "targetPort": 12345,
  "accessCode": "483921"
}
```

字段规则：

- `status` 不需要其他字段。
- `disable` 只需要 `action`。
- `enable` 需要 `listenPort`、`hubId`、`targetHost`、`targetPort`、`accessCode`。
- `regenerate` 需要新的 `accessCode`，并保留当前 mapping；如果当前未启用，返回 `INVALID_ARGUMENT`。
- `accessCode` 必须是 6 位数字。
- `targetHost` 默认为 `127.0.0.1`，但请求必须显式传入最终值，便于审计。
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

### `relay.open`

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
2. 用 `nonce` 主动连接 `relayURL`。
3. 进入 tunnel read loop。
4. 返回 `ok=true` 只表示已开始连接流程；Registry 以数据 tunnel 握手为准更新 `Up`。

### `relay.close`

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

未认证访问普通路径时返回一个最小 HTML PIN 页面。登录提交：

```text
POST /__wheelmaker/relay/login
code=483921
```

成功：

- 设置 `wm_port_relay=<signed-session>`。
- Cookie 属性：`HttpOnly`、`SameSite=Lax`、`Path=/`。
- 如果请求是 HTTPS 反代后的 HTTPS，部署层可通过 `X-Forwarded-Proto` 帮助设置 `Secure`；V1 可以在明文本地模式不设置 `Secure`。
- Redirect 到 `/`。

失败：

- 返回 401。
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
http://targetHost:targetPort/assets/app.js?v=1
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
ws://targetHost:targetPort/ws
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

Settings 增加 `Port Relay` detail 页面。V1 不需要独立主导航。

页面状态：

- 从 `cmd.portRelay status` 初始化。
- 生成 6 位码在前端完成，保存时随 `enable` / `regenerate` 发送给 Registry。
- 如果页面刷新后无法知道旧明文访问码，显示 `Regenerate required`，要求用户重新生成。
- `Open` 使用当前 registry address 推导 relay URL：
  - Registry address 是 `wss://host/ws` 或 `https://host` 时，Open 使用 `https://host:<listenPort>/`。
  - Registry address 是 `ws://host/ws` 或 `http://host` 时，Open 使用 `http://host:<listenPort>/`。
  - 如果原地址包含端口，替换为 `listenPort`。

UI 不需要嵌入 iframe。用户点击 Open 后在 App/WebView 或浏览器打开 relay 端口页面。

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

- 未认证：返回 PIN 登录页或 401。
- relay disabled：503。
- tunnel not up：503。
- stream open 超时：504。
- target 连接失败：502。
- frame 协议错误：关闭 tunnel，slot 进入 `Error`。

错误页保持纯文本或最小 HTML，不暴露 token、nonce、access code 或内部 stack。

## 可观测性

Registry log：

- relay enable/disable/regenerate/status
- listener start/stop 和端口
- selected hub 和 target host/port
- tunnel connected/disconnected
- active stream count
- auth failures 计数和冷却

Hub log：

- relay.open/close 收到
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

- client role allowlist 允许 `cmd.portRelay`。
- hub/monitor role 不能修改 relay slot。
- unknown hub 返回 `NOT_FOUND`。
- offline hub 返回 `UNAVAILABLE`。
- `enable` 下发 `relay.open` 给目标 Hub。
- `disable` 下发 `relay.close` 给旧 Hub。
- `status` 返回当前 slot。

Hub reporter 测试：

- `relay.open` 创建 Hub tunnel client。
- `relay.close` 关闭匹配 relay id。
- `relay.close` relay id 不匹配时幂等。
- target request 带固定 User-Agent。

App 测试：

- repository/service 发送 `cmd.portRelay` status/enable/disable/regenerate。
- Settings detail 包含 Port Relay 页面。
- 6 位访问码生成。
- 状态 `Disabled/Opening/Up/Error` 展示。
- Open URL 从 registry address 和 listen port 推导。
- 页面刷新后无法恢复明文访问码时要求 regenerate。

## 实施顺序

1. 建立 `server/internal/portrelay` 包和 frame codec 测试。
2. 实现 registry 侧 controller/listener/auth 的纯包测试。
3. 接入 Registry `cmd.portRelay` allowlist 和转发到 Hub control request。
4. 实现 Hub `relay.open/close` wiring 和 hub tunnel client。
5. 做 HTTP request 转发 smoke。
6. 做 WebSocket text/binary 转发 smoke。
7. 增加 App service 方法。
8. 增加 Settings Port Relay 页面。
9. 更新协议文档中 client command 列表和版本历史。

## Open Decisions

None. 单例 slot、App 设置来源、6 位访问码、Hub 主动 tunnel、自定义二进制 frame、同端口 HTTP/WS、只伪装 User-Agent、核心逻辑集中在 `server/internal/portrelay` 均已确定。
