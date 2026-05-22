# Registry 端口中转

Status: ready-for-agent

父级 PRD：`docs/superpowers/specs/2026-05-22-registry-port-relay-prd.md`

技术设计规格：`docs/superpowers/specs/2026-05-22-registry-port-relay-design.md`

这组本地 issue 将 Registry 端口中转功能拆成可交给 AFK agent 执行的竖切任务。目标是在 App 设置页启用一个 Registry 全局单例 relay slot，指定 listen port、目标 Hub、目标 host/port 和 6 位访问码；Registry 使用现有 `/ws` 作为控制面，通知目标 Hub 主动连接 relay 数据端口；App 或浏览器访问该 relay 端口时，HTTP 与 WebSocket 都通过自定义 tunnel 转发到目标 Hub 上的第三方本地服务。

核心约束：

- 同一时间只有一个 active mapping。
- 设置来自 App UI，不写入 Hub 配置文件。
- 数据端口需要 6 位访问码登录，登录后用 HttpOnly cookie 访问 HTTP 和 WS。
- Relay 必须支持 Machine B/NAT 场景，由 Hub 主动建立数据 tunnel。
- HTTP 与 WS 使用同一个 relay 端口。
- 第三方页面不需要 path-prefix 改造，普通根路径 `/` 与 `/ws` 应可工作。
- Tunnel 使用自定义二进制 frame，并在一条 active tunnel 内用 `streamId` 复用多个 HTTP/WS stream。
- 实现尽量集中在 `server/internal/portrelay` 一个 Go 包内，Registry 与 Hub reporter 只做薄 wiring。
