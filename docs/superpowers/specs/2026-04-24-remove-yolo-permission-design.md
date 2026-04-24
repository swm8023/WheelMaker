# Remove YOLO And Permission Flow Design

日期：2026-04-24

## 背景

当前实现里，`yolo` 既是项目配置开关，也是 permission 自动放行与 IM 展示分叉的来源。

这导致同一个能力被拆散在多层：

- `server/internal/hub/client/permission.go` 里决定是自动放行还是进入 IM 往返
- `server/internal/im/*` 里承载 permission 请求和响应
- `server/internal/hub/client/session_recorder.go` 里把 permission 持久化为 session turn
- `app/web/src/*` 里暴露 permission 消息、按钮和响应 API
- `server/internal/shared/config.go` 和 SQLite `projects.yolo` 里长期保留了一个已经不再需要的模式开关

目标是去掉这个模式分叉：系统默认总是自动放行，不再把 permission 作为一个跨层协议能力来维护。

## 已确认决策

### 1. `yolo` 完整移除

- 删除项目配置里的 `yolo`
- 删除运行时 `Client` / `Session` / Feishu transport 上的 `yolo` 状态
- 删除 SQLite `projects.yolo` 列
- 不做旧数据库兼容迁移
- 旧库继续由 schema gate 拒绝启动，用户手动删库重建

### 2. permission 不再进入 client / IM / UI 决策链

- agent 发起 `session/request_permission` 后，session 内部直接同步返回结果
- 不再发 IM permission request
- 不再接收 IM permission response
- 不再暴露 `chat.permission.respond`
- 不再在 web/app UI 渲染 permission 按钮

### 3. permission 新流程完全无痕

- 不写 session recorder turn
- 不发 `session.message`
- 不在 pull/read 结果里保留一条自动通过记录

### 4. 旧 permission 历史不做兼容

- 删除 permission 专属协议和展示逻辑
- 旧数据如果仍存在，只按通用未知内容回退处理
- 不提供专门迁移或转换

## 目标状态

### 服务端

`SessionRequestPermission` 收到 ACP permission 请求后，直接选出一个允许型 option 并返回 `PermissionResponse`。

允许型 option 选择规则需要标准化：

1. 优先 `kind=allow_always` 或等价 always 语义
2. 其次 `kind=allow_once`、`kind=allow`、`kind=once`
3. 再其次匹配 `optionId` / `name` 明显表示 allow 的选项
4. 如果不存在可识别的允许项，则返回 `cancelled`

返回结果仍然使用现有 ACP `PermissionResult` 形状，避免破坏 agent 请求的同步返回契约。

### 存储

SQLite schema 更新为最终形态：

- `projects` 表仅保留 `project_name`、`agent_state_json`、`created_at`、`updated_at`
- `expectedStoreSchemaColumns` 同步更新
- `CheckStoreSchema` 继续用于阻止旧库启动

这次改动明确要求用户手动删除本地数据库目录后重启，不引入任何自动迁移。

### Session Recorder

permission 不再被视为 session history 的一部分：

- 删除 `request_permission` parse 分支
- 删除 permission merge key / merge plan / merge function
- 删除 permission turn payload 结构
- 删除相关测试基线

保留的 recorder 行为只覆盖 prompt、session.update、system 等仍然存在的事件。

### IM 与 Registry

permission 从 IM 边界移除：

- IM router 不再声明/调用 `PublishPermissionRequest`
- app channel 不再维护 pending permission request
- registry server / reporter 不再转发 `chat.permission.respond`
- Feishu transport 不再渲染 permission option card

Feishu 里原先挂在 `YOLO` 下的紧凑 tool stream 保留为默认行为，不再受配置开关影响。

### Web App

web 侧删除整条 permission 路径：

- 删除 `permission` kind 的专属解析和动作
- 删除 `respondToSessionPermission` repository / service API
- 删除 permission action UI 和样式
- 删除 “New permission request” 等专属提示

结果是 session/chat UI 只保留 message、thought、tool、prompt_result 等仍然有效的消息类别。

## 影响文件

服务端主要涉及：

- `server/internal/hub/client/permission.go`
- `server/internal/hub/client/session.go`
- `server/internal/hub/client/client.go`
- `server/internal/hub/client/session_recorder.go`
- `server/internal/hub/client/sqlite_store.go`
- `server/internal/hub/hub.go`
- `server/internal/shared/config.go`
- `server/internal/im/router.go`
- `server/internal/im/app/app.go`
- `server/internal/im/feishu/*`
- `server/internal/registry/server.go`
- `server/internal/hub/reporter.go`
- `server/internal/protocol/im_protocol.go`

前端主要涉及：

- `app/web/src/types/registry.ts`
- `app/web/src/services/registryRepository.ts`
- `app/web/src/services/registryWorkspaceService.ts`
- `app/web/src/main.tsx`
- `app/web/src/styles.css`
- `app/__tests__/web-chat-ui.test.ts`

## 非目标

- 不修改 ACP 自身的 `session/request_permission` 方法名和基础类型定义
- 不为旧 session history 提供 permission 专属兼容层
- 不引入数据库自动迁移
- 不重构与本次删除无关的 session/update 或 tool 流程

## 风险与约束

### 1. 旧库无法直接启动

这是本次改动的预期行为，不作为缺陷处理。需要在文档和最终说明里明确告知用户删库重建。

### 2. Feishu 默认 tool 渲染不能回退

移除 `yolo` 开关后，当前紧凑 tool stream 成为唯一行为。需要用现有 Feishu 测试覆盖确认其仍可工作。

### 3. permission option `kind` 目前并不统一

现有测试数据同时出现 `allow_always`、`allow_once`、`allow`、`once`。自动放行选择器必须显式兼容这些已存在形状，否则会把本应允许的请求误判为 `cancelled`。

## 验证策略

至少覆盖以下回归：

1. session permission 请求会立即返回允许结果，且不写 session turn
2. schema check 会拒绝仍包含 `projects.yolo` 的旧库
3. registry / reporter 不再接受或转发 `chat.permission.respond`
4. app/web 不再暴露 permission 响应 API 和 UI
5. Feishu 默认 tool stream 在移除 `yolo` 后仍通过现有关键测试