# Codex Agent Identity Rename Design

日期：2026-05-21
状态：已确认，待实现计划

## 背景

WheelMaker 目前有两条 Codex 运行路径：

- `codex`：旧 `codex-acp` 适配器路径。
- `codexapp`：新的 `codex app-server --listen stdio://` 路径。

这个命名已经和期望的用户语义相反。用户期望 `codex` 代表新的 Codex app-server，旧 ACP 适配器明确叫 `codexacp`。因为 agent type 已经写入 SQLite、registry payload、session tag、New/Resume 菜单和 NPM Update 页面，这次是协议和持久化语义变更，不应只做 UI 文案替换。

## 目标

- 外部 agent identity 统一为：
  - `codex` = Codex app-server。
  - `codexacp` = 旧 `codex-acp` 适配器。
- 自动迁移每台 hub 本地 WheelMaker SQLite 数据，避免新旧 `codex` 语义混用。
- New/Resume 入口不再支持 `codexacp`。
- 已管理的历史 `codexacp` session 仍可显示、加载、reload 和继续发送消息。
- Registry protocol 升级到 `2.3`，拒绝旧 `2.2` hub/client/local-read 混连，避免 Web 端推断旧语义。

## 非目标

- 不迁移真实 Codex CLI 的 `~/.codex` 数据目录。
- 不移除 `codex-acp` provider 后端能力；它仍用于历史 session。
- 不大规模重命名 Codex app-server bridge 的内部 helper 和文件，例如 `codexappConn`、`codexappRuntime`、`codexapp_agent.go`。这些名字描述实现细节，不是外部 provider identity。
- 不做 Web 端旧版本兼容映射。旧 hub 应通过协议版本拒绝连接。

## 协议设计

Registry protocol 从 `2.2` 升级到 `2.3`。

- `DefaultProtocolVersion = "2.3"`。
- Hub reporter、Web client、local-read connect init 都发送 `2.3`。
- Registry 和 local-read 继续使用 exact match；收到 `2.2` 时返回 unsupported protocolVersion。
- 文档更新为 Registry Protocol 2.3，并记录 agent identity rename 是破坏性协议变更。

结果是未升级远端 hub 会离线或不可见，直到该机器更新 WheelMaker。这样可以避免 Web 把旧 `codex` 猜成新 `codex` 或 `codexacp`。

## Agent Identity 设计

后端 provider 常量和注册语义改为：

- `ACPProviderCodex = "codex"`：注册到当前 Codex app-server agent 实现。
- `ACPProviderCodexACP = "codexacp"`：注册到旧 `codex-acp` 通用 ACP provider。

旧 ACP provider 的 preset 保留 `BinaryName: "codex-acp"`，但 `Name` 改为 `codexacp`。Codex app-server provider 的外部 `Name()` 和 `agentInfo.name` 改为 `codex`。内部 app-server bridge helper 可保留 `codexapp*` 命名。

默认 provider 优先级中 `codex` 应位于 `codexacp` 之前。`codexacp` 可以被 factory 创建，但不参与用户新建和外部恢复入口。

## 迁移设计

SQLite store 打开时执行幂等数据迁移。迁移必须在正常读写 session/default/preference 之前完成。

迁移规则：

- `codexapp -> codex`
- `codex -> codexacp`

覆盖表和字段：

- `sessions.agent_type`
- `projects.default_agent_type`
- `agent_preferences.agent_type`

迁移需要避免冲突。`agent_preferences` 以 `(project_name, agent_type)` 为主键；如果同一 project 同时存在旧 `codex` 和旧 `codexapp` preference，迁移后应分别落到 `codexacp` 和 `codex`，不会冲突。如果目标 key 已存在，保留目标记录并删除或忽略被迁移的旧重复记录，保证 store 可打开。

## 创建和恢复入口

`codexacp` 不再允许通过以下入口创建或导入：

- Web New Session 菜单。
- Web Resume Session 菜单。
- `session.new` API。
- `session.resume.list` API。
- `session.resume.import` API。
- IM `/new codexacp`。

已存在的 `codexacp` session 仍在 session list 中展示，并可通过普通 session 选择、`session.reload`、IM `/load` 或消息发送触发后端 load。

## Web UI 设计

Web 端只消费新语义，不兼容旧 `2.2` agent 命名。

- Session tag 直接显示后端新 agent type。
- New/Resume agent picker 过滤 `codexacp`。
- `AGENT_TAG_VARIANT_INDEX` 更新为新名字，保留 `codex` 的主要样式，并给 `codexacp` 独立样式。
- Update 页面 package tag 改为：
  - `@openai/codex` -> `codex`
  - `@zed-industries/codex-acp` -> `codexacp`

## NPM 和部署脚本

NPM package policy 更新 agentTypes：

- `@openai/codex` 标记为 `codex`。
- `@zed-industries/codex-acp` 标记为 `codexacp`。

部署脚本仍可检查和安装 `@zed-industries/codex-acp`，因为旧历史 session 仍可能依赖它。是否默认安装 `@openai/codex` 维持当前策略：通过 Update 页面管理，不由部署脚本强制安装。

## 错误处理

- 请求 `session.new` 或 `/new codexacp` 返回明确错误：`codexacp is not supported for new sessions`。
- 请求 `session.resume.list/import` 的 `agentType=codexacp` 返回明确错误：`codexacp is not supported for resume import`。
- 已存在 `codexacp` session load 失败时沿用现有 provider/load 错误，不伪装成 `codex`。
- 旧协议连接失败沿用 unsupported protocolVersion，并在 details 中包含 received/supported。

## 测试策略

Server:

- `protocol.ParseACPProvider` 接受 `codex` 和 `codexacp`，不接受 `codexapp`。
- Factory 注册 `codex` 到 Codex app-server provider，注册 `codexacp` 到 `codex-acp` provider。
- Store migration 覆盖 sessions/projects/agent_preferences。
- `session.new` 和 IM `/new` 禁止 `codexacp`。
- `session.resume.list/import` 禁止 `codexacp`。
- 已存在 `codexacp` session 可通过 load/reload 路径解析 provider。
- Registry/local-read 拒绝 `2.2`，接受 `2.3`。
- NPM package response agentTypes 更新。

Web:

- `connect.init` 使用 `2.3`。
- New/Resume agent picker 过滤 `codexacp`。
- Session tag variant 使用 `codex`/`codexacp`。
- Update 页面显示 `@openai/codex -> codex` 和 `@zed-industries/codex-acp -> codexacp`。

## Rollout

升级新版 WheelMaker 后：

1. Registry/server 使用 protocol `2.3`，旧 hub 被拒绝。
2. 每台 hub 启动时自动迁移自己的 SQLite store。
3. 已升级 hub 上报新 agent identity。
4. Web 不做旧名映射，只显示新 identity。

远端机器未升级前不可用；升级后自动恢复并完成本地迁移。
