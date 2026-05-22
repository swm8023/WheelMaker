# Registry 端口中转 PRD

Status: ready-for-agent

## 问题陈述

WheelMaker 需要让 App 或浏览器访问某个 Hub 机器上的第三方本地 Web 页面。该页面由第三方服务提供，通常运行在 Hub 本机或 Hub 可访问的固定地址上，并且页面本身只是静态壳，主要能力通过 WebSocket 提供。

直接在 App 中打开 `127.0.0.1:<port>` 不成立，因为 App/WebView 中的 `127.0.0.1` 指向手机或当前浏览器机器，而不是目标 Hub。要求支持 Machine B 这类只主动连接 Registry、没有公网入口的 Hub，因此不能要求外部客户端直接访问 Hub 的本机端口。

目标是让 Registry 充当一个运行态中转站：App 在设置页面打开一个全局单例端口映射，指定 Registry 监听端口、目标 Hub、目标 host/port 和 6 位访问码。之后 App 或浏览器访问 Registry 上的这个端口，就能通过 Hub 主动建立的数据 tunnel 访问目标第三方页面。

## 方案

新增 Registry 端口中转能力。Registry 通过现有标准 `/ws` 连接作为控制面，通过用户配置的 relay 端口作为数据面。一次只允许一个全局 active mapping 生效。

运行态流程：

1. Hub 继续通过现有 Registry 标准端口连接：
   `Hub -> registry:<registryPort>/ws`。
2. App 设置页更新 Registry 全局 relay slot：
   `enabled`、`listenPort`、`hubId`、`targetHost`、`targetPort`、`accessCode`。
3. Registry 通过现有控制面通知目标 Hub：
   `relay.open`，携带 relay 数据端口地址、目标地址、一次性 nonce 和 tunnel 参数。
4. Hub 主动连接 Registry relay 数据端口的内部路径：
   `ws://registry-host:<listenPort>/__wheelmaker/relay/hub`。
5. Tunnel 建立成功后 Registry 状态变为 `Up`，App 设置页显示可访问地址。
6. App 或浏览器访问：
   `http://registry-host:<listenPort>/` 与 `ws://registry-host:<listenPort>/...`。
7. Registry 在 relay 端口上同时处理普通 HTTP 和 WebSocket upgrade，并通过 active tunnel 转发到目标 Hub。
8. Hub 连接目标第三方服务：
   `targetHost:targetPort`，并在发给第三方服务的 HTTP/WS 请求中使用浏览器风格 `User-Agent`。

该能力应尽量集中在一个 Go 包内，例如 `server/internal/portrelay`。Registry、Hub reporter 只保留薄 wiring：控制面 method 分发、生命周期挂接和状态查询。不要把 tunnel frame 编解码、listener 热切换、访问码认证、stream 复用散落到 `registry/server.go` 或 `hub/reporter.go` 主文件里。

## 用户故事

1. 作为 WheelMaker 用户，我想在 App 设置页面打开端口中转，这样可以访问目标 Hub 上的第三方本地 Web 页面。
2. 作为 WheelMaker 用户，我想选择目标 Hub，这样可以访问没有公网入口但已经连上 Registry 的机器。
3. 作为 WheelMaker 用户，我想设置 Registry 监听端口，这样可以用固定端口在 App 或浏览器里打开页面。
4. 作为 WheelMaker 用户，我想设置目标 host 和目标 port，这样 relay 能映射到 Hub 可访问的固定服务地址。
5. 作为 WheelMaker 用户，我想一次只启用一个 mapping，这样当前 relay 端口的目标明确，不会出现多页面互相抢路径。
6. 作为 WheelMaker 用户，我想在设置页生成一个 6 位数字访问码，这样可以把访问地址临时分享给自己当前设备，而不需要改配置文件。
7. 作为 WheelMaker 用户，我希望每次开启、切换目标或重新生成访问码后旧访问码立即失效，这样旧链接不会长期可用。
8. 作为 WheelMaker 用户，我希望设置页显示 `Disabled`、`Opening`、`Up`、`Error` 状态，这样能判断 tunnel 是否真正建立成功。
9. 作为 WheelMaker 用户，我希望点击 Open 后 App 内和浏览器都访问同一个 relay 地址，这样不用为 App 和 Web 分别设计入口。
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

## 产品行为

### 设置入口

在 Settings 中新增 `Port Relay` 页面。V1 入口可以先放在现有设置列表中，不需要独立主导航。

页面字段：

- `Enable relay`：开关。
- `Listen port`：Registry relay 数据端口。
- `Hub`：从在线 Hub 列表选择。
- `Target host`：默认 `127.0.0.1`，也允许显式固定 IP 或 host。
- `Target port`：目标服务端口。
- `Access code`：6 位数字，可随机生成和手动重新生成。
- `Status`：`Disabled`、`Opening`、`Up`、`Error`。
- `Open`：根据当前 Registry 地址和 `listenPort` 生成访问地址。

设置不写入 Hub 配置文件。它是 Registry 运行态全局状态。页面刷新后可以通过 Registry 查询当前 relay slot 状态恢复 UI。

### 全局单例

Registry 同一时间只有一个 active relay slot。新的启用请求会替换旧 mapping。替换规则：

1. 校验控制请求来源已经通过 Registry token 鉴权。
2. 校验 `listenPort`、`hubId`、`targetHost`、`targetPort` 和访问码。
3. 如果 listen port 变化，先尝试启动新 listener。
4. 新 listener 和新 Hub tunnel 建立成功后，关闭旧 listener 或旧 tunnel。
5. 如果新 listener 启动失败，不破坏旧的 active slot；状态返回 `Error`。
6. 如果新 tunnel 建立失败，relay slot 进入 `Error`，旧 tunnel 必须关闭，避免请求继续落到旧目标。

关闭 relay 时，Registry 关闭数据 listener，通知旧 Hub `relay.close`，断开 active tunnel 和所有 stream。

### 访问码

访问码是 6 位数字，由 App 设置页随机生成。访问码不是配置文件字段，不随 Hub 发布配置持久化。每次开启 relay、切换 mapping 或手动 Regenerate 时，旧访问码立即失效。

数据端口访问规则：

1. 未认证访问 `http://registry-host:<listenPort>/` 时，Registry 返回极简登录页。
2. 用户输入 6 位访问码后，Registry 设置 relay 端口作用域的 HttpOnly cookie。
3. 后续 HTTP 请求和 WebSocket upgrade 自动携带 cookie。
4. 访问码错误需要限速：同来源连续错误 5 次后冷却 60 秒。
5. 关闭 relay 或访问码重生成时，既有 cookie 失效。

访问码可以满足临时访问和误扫保护，但不等价于强安全凭据。公网场景应继续依赖部署层 TLS；明文 HTTP/WS 只适合本机或可信内网。

### 数据访问

Relay 数据端口同时支持：

- 普通 HTTP request/response。
- WebSocket upgrade。
- 多个并发 HTTP 请求。
- 一个或多个第三方 WebSocket 连接。
- WebSocket text frame。
- WebSocket binary frame。

所有非内部路径都转发给 active mapping 的目标服务。内部路径使用保留前缀：

- `/__wheelmaker/relay/login`
- `/__wheelmaker/relay/logout`
- `/__wheelmaker/relay/hub`
- `/__wheelmaker/relay/status`

第三方页面自己的 `/ws`、`/api`、`/assets`、`/favicon.ico` 等路径不能和内部路径冲突。

### 目标地址约束

Relay slot 只允许转发到当前设置中的固定 `targetHost:targetPort`。外部请求不能通过 query、header、path 或 WebSocket payload 改写目标地址。

`targetHost` 默认推荐 `127.0.0.1`。如果用户明确设置固定 IP 或 host，Hub 只访问这个固定地址。V1 不提供任意 URL 输入、端口扫描或每请求动态目标。

### User-Agent

Hub 侧连接第三方服务时使用浏览器风格 `User-Agent`。V1 只伪装 `User-Agent`，不强制改写 `Origin`、`Referer` 或其他浏览器环境信息。

默认 User-Agent 使用固定现代桌面浏览器 UA。后续如有需要，可以在设置页增加高级字段，但 V1 不要求暴露。

## 技术决策

- 控制面继续使用现有 Registry `/ws` JSON envelope。
- App 到 Registry 的控制方法使用显式 allowlist，例如 `cmd.portRelay`。
- Registry 到 Hub 的控制请求使用 `relay.open`、`relay.close`、`relay.status`，通过现有 Hub control connection 下发。
- Relay 数据面不复用现有 Registry JSON envelope。
- Relay 数据面使用自定义二进制 frame。
- Frame 必须包含 `streamId`，以便一条 active tunnel 内复用多个 HTTP/WS stream。
- HTTP metadata 与 WebSocket metadata 作为 frame metadata 传输。
- HTTP body 和 WebSocket binary payload 必须按 raw bytes 传输，不做 JSON base64 包装。
- Registry relay listener 使用 Go `http.Server`，同一端口处理 HTTP 和 WebSocket upgrade。
- Hub 收到 `relay.open` 后主动连接 relay 数据端口内部路径，携带一次性 nonce 完成 tunnel 握手。
- Registry 只有在 nonce 匹配且 slot 状态为 `Opening` 时接受 Hub tunnel。
- Active tunnel 断开后，Registry 状态变为 `Error` 或 `Opening`，并关闭所有外部 stream。
- Registry 控制面状态查询返回当前 relay slot、状态、错误消息、访问地址和 tunnel 连接时间。
- Relay 端口的访问码 cookie 不得被第三方响应覆盖。Registry 至少要过滤与自身 auth cookie 同名的 `Set-Cookie`。
- 第三方服务自己的其他 cookie 可以按 relay 端口 origin 使用；它们不会复用 Hub 本机浏览器的 cookie jar。
- Hop-by-hop headers 不能原样跨端转发，包括 `Connection`、`Upgrade`、`Keep-Alive`、`Proxy-Authenticate`、`Proxy-Authorization`、`TE`、`Trailer`、`Transfer-Encoding`。
- 实现尽量集中在 `server/internal/portrelay` 一个包内。该包同时拥有 registry 侧 controller/listener、hub 侧 tunnel client、frame codec、auth、stream multiplexing 和测试 helper。
- `server/internal/registry/server.go` 只负责把 `cmd.portRelay` 路由到 portrelay controller。
- `server/internal/hub/reporter.go` 只负责把 `relay.open/close/status` 路由到 portrelay hub client。
- Web 前端通过现有 `RegistryRepository` 和 `RegistryWorkspaceService` 增加 port relay 方法。

## 建议文件边界

新增 server 包集中承载核心逻辑：

- `server/internal/portrelay/types.go`
- `server/internal/portrelay/frame.go`
- `server/internal/portrelay/controller.go`
- `server/internal/portrelay/listener.go`
- `server/internal/portrelay/auth.go`
- `server/internal/portrelay/mux.go`
- `server/internal/portrelay/hub_client.go`

现有文件只做薄集成：

- `server/internal/protocol/port_relay.go`
- `server/internal/registry/server.go`
- `server/internal/hub/reporter.go`
- `app/web/src/types/registry.ts`
- `app/web/src/services/registryRepository.ts`
- `app/web/src/services/registryWorkspaceService.ts`
- `app/web/src/portRelayView.tsx`
- `app/web/src/main.tsx`
- `app/web/src/styles.css`

测试按职责放置：

- `server/internal/portrelay/*_test.go` 覆盖核心包。
- `server/internal/registry/server_test.go` 覆盖控制面 allowlist、状态查询和 Hub 转发。
- `server/internal/hub/reporter_test.go` 覆盖 Hub 收到 relay control request 后创建/关闭 tunnel。
- `app/__tests__/web-port-relay-service.test.ts` 覆盖 repository/service payload。
- `app/__tests__/web-port-relay-settings.test.ts` 覆盖设置页入口、字段和状态展示。

## 测试决策

- Frame codec 测试覆盖 metadata frame、raw data frame、close/error frame 和非法 frame。
- Stream 复用测试覆盖同一 tunnel 内并发 `GET /`、`GET /assets/app.js` 和 `WS /ws`。
- Registry listener 测试覆盖 HTTP request 转发。
- Registry listener 测试覆盖 WebSocket text frame 转发。
- Registry listener 测试覆盖 WebSocket binary frame 转发。
- Auth 测试覆盖未登录返回 PIN 页面。
- Auth 测试覆盖正确访问码设置 cookie。
- Auth 测试覆盖错误访问码限速。
- Auth 测试覆盖 regenerate 后旧 cookie 失效。
- Control method 测试覆盖 client 角色允许 `cmd.portRelay`，monitor/hub 角色不允许修改 slot。
- Control method 测试覆盖 unknown hub 返回 `NOT_FOUND`，offline hub 返回 `UNAVAILABLE`。
- Hot switch 测试覆盖新 listener 启动失败时旧 slot 不被破坏。
- Hot switch 测试覆盖切换 Hub 时旧 tunnel 被关闭。
- Hub client 测试覆盖收到 `relay.open` 后拨数据端口内部路径。
- Hub client 测试覆盖 hub 侧目标请求使用浏览器风格 `User-Agent`。
- Header 测试覆盖 hop-by-hop headers 不跨端透传。
- UI 测试覆盖 Settings 中存在 Port Relay 入口。
- UI 测试覆盖状态 `Disabled/Opening/Up/Error`。
- UI 测试覆盖随机生成 6 位访问码。
- UI 测试覆盖 Open 地址由当前 registry host 和 listen port 派生。

## 非目标

- 不支持多个 relay mapping 同时生效。
- 不支持每个用户或每个设备单独 mapping。
- 不支持从数据端口请求中动态选择目标 host/port。
- 不支持任意 URL proxy。
- 不支持端口扫描。
- 不要求第三方页面支持 path-prefix。
- 不要求额外域名或子域名。
- 不复用 Hub 本机浏览器的 cookie、localStorage、Service Worker 或缓存。
- 不把第三方页面运行在 WheelMaker 主 Web UI 同源 path 下。
- 不在 V1 提供外部强认证系统；6 位访问码只作为 relay 访问门禁。
- 不在 V1 提供多租户 ACL、审计报表或长期分享链接。
- 不改造第三方页面的 JavaScript、WebSocket URL 或静态资源路径。
- 不把 HTTP/WS body base64 包进现有 Registry JSON envelope。

## 验收标准

1. 用户可以在 Settings 中打开 Port Relay 页面。
2. 用户可以选择在线 Hub、设置 listen port、target host、target port，并生成 6 位访问码。
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
13. 切换 Hub 或 target port 后，旧 tunnel 不再接收新请求。
14. Machine B 只主动连接 Registry 时，仍能作为 relay 目标 Hub 被访问。

## 补充说明

该功能的核心产品规则是：Registry 暂时成为一个用户可控的单例中转站，目标明确、生命周期明确、状态可见。为了兼容第三方页面，数据访问使用独立 relay 端口和根路径代理；为了兼容 NAT 后 Hub，数据 tunnel 必须由 Hub 主动连接 Registry。

本仓库未配置外部 issue tracker。该 PRD 作为本地 `ready-for-agent` 方案存放在现有 Superpowers specs 目录下。
