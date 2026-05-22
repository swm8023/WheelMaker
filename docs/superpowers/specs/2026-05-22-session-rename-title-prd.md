# Session 标题重命名 PRD

Status: ready-for-agent

## 问题陈述

WheelMaker 当前从 prompt title facts 推导 chat session 标题。用户无法在 Web UI 中修正、缩短或重置 session 标题。现有 `Use Latest Prompt Title` 选项也让标题行为不够可预测，因为一个全局设置会改变所有 session label 的显示方式。

用户需要一个简单的 session 级 rename action。手动重命名过的 session 必须保持该标题，直到用户主动清除；即使后续 prompt 或 agent-side title update 到达，也不能覆盖它。

## 方案

为每个 session 增加前端 rename action。该 action 打开一个单行 dialog，并预填当前显示标题。保存非空值会存储手动标题。保存空值或全空白值会清除手动标题，并恢复自动 first-prompt title。

后端暴露新的 `session.rename` 请求，并把手动标题状态存入现有 session title facts JSON。不需要 SQLite schema migration。显示标题规则变为确定性顺序：

1. 有 `manual` 时使用 `manual`。
2. 否则使用 `first`。
3. 否则在需要时使用 `last` 或 legacy raw title。

移除全局 `Use Latest Prompt Title` 设置。未重命名 session 默认使用 first prompt title。

## 用户故事

1. 作为 WheelMaker web 用户，我想从 session list 重命名 session，这样以后识别它时不必依赖首次生成的 prompt 标题。
2. 作为 WheelMaker web 用户，我希望 rename action 放在现有 session action 附近，这样不需要去 settings 或另一个页面寻找。
3. 作为 WheelMaker web 用户，我希望有一个聚焦的 rename dialog，这样不用离开 chat 就能编辑标题。
4. 作为 WheelMaker web 用户，我希望 rename dialog 显示当前标题，这样可以小幅修改而不是重新输入。
5. 作为 WheelMaker web 用户，我希望有 Save 和 Cancel action，这样可以明确提交或放弃标题编辑。
6. 作为 WheelMaker web 用户，我希望保存空标题能清除手动 rename，这样可以让 session 回到自动命名。
7. 作为 WheelMaker web 用户，我希望全空白输入按空输入处理，这样误输入空格不会创建不可见标题。
8. 作为 WheelMaker web 用户，我希望重命名后的 session 在新 prompt 后仍保留手动标题，这样后续 prompt 标题不会破坏我的组织方式。
9. 作为 WheelMaker web 用户，我希望重命名后的 session 在 agent-side title event 后仍保留手动标题，这样 provider 更新不会覆盖我的选择。
10. 作为 WheelMaker web 用户，我希望未重命名 session 使用 first prompt title，这样旧 session 标题稳定。
11. 作为 WheelMaker web 用户，我希望移除 latest-prompt title 设置，这样 session 标题行为一致。
12. 作为 WheelMaker web 用户，我希望能重命名运行中的 session，这样 agent 仍在工作时也能给它标注。
13. 作为 WheelMaker web 用户，我希望 rename 更新 session 行但不移动该行，这样列表不会意外跳动。
14. 作为 WheelMaker web 用户，我希望 rename 在 desktop 和 mobile layout 都可用，这样 session action strip 出现的地方都有该功能。
15. 作为 WheelMaker web 用户，我希望长标题有边界限制，这样误粘贴不会破坏 UI。
16. 作为 WheelMaker web 用户，我希望粘贴的多行文本归一化成单行标题，这样 session label 仍适合列表展示。
17. 作为 WheelMaker web 用户，我希望 rename 失败时保留旧标题可见，这样 UI 不会暗示未保存成功的变化。
18. 作为 WheelMaker web 用户，我希望其他 client 的标题变更通过正常 session update 呈现，这样多个打开的 client 能收敛。
19. 作为 WheelMaker 开发者，我希望一个后端 title-facts 模块拥有标题解析和优先级规则，这样自动标题和手动标题行为可测试。
20. 作为 WheelMaker 开发者，我希望一个前端 title resolver 镜像后端显示语义，这样 list label 和 selected-session label 不会分叉。
21. 作为 WheelMaker 开发者，我希望该功能不改数据库 schema，这样现有 hub 可以升级且没有启动迁移风险。
22. 作为 WheelMaker 开发者，我希望 `session.rename` 是 registry-routed session request，这样 local 和 remote hub 使用同一份契约。

## 实现决策

- 公共请求 method 是 `session.rename`。
- 请求 payload 包含 `sessionId` 和 `title`。
- 成功响应返回 `ok: true`、`sessionId` 和更新后的 session summary。
- Registry method whitelist 接受 `session.rename`。
- 后端通过给现有 session title facts object 扩展 `manual` 字段来存储手动标题状态。
- 后端不为该功能新增、删除或迁移 SQLite column。
- 后端 title facts helper 成为权威 deep module，负责解析、更新 automatic facts、设置 manual facts、清除 manual facts、解析 display title。
- 自动 title event 继续更新 automatic facts，但必须保留已存在的手动标题。
- 手动标题拥有最高显示优先级。
- 空值或全空白 rename 输入会清除手动标题。
- 标题归一化发生在输入边界：trim 外层空白、把换行替换为空格、把不安全的多行内容折叠成一行，并强制 200 字符上限。
- 运行中的 session 允许 Rename。
- Rename 不得更新 session 活动排序、last-active timestamp、turn cursor、read cursor 或 archived state。
- Rename 只属于 WheelMaker 显示元数据，不传播到 Codex、Claude、ACP 或 provider-native thread title state。
- 前端标题解析移除全局 latest-prompt toggle，并始终使用确定性标题优先级。
- 从 UI state、persistence write 和 settings rendering 中移除 `Use Latest Prompt Title` 设置。
- 已存在的持久化 `Use Latest Prompt Title` 值可以忽略；不需要客户端迁移。
- session action strip 增加 Rename action；即使运行中 session 的 Reload 和 Archive 被禁用，Rename 仍可用。
- rename dialog 是紧凑的单行 modal，尽量复用现有 confirmation/dialog 风格。
- 前端 service layer 暴露 rename method，通过现有 registry repository 路径调用 `session.rename`。
- UI 在保存后应用服务端返回的 session summary，并接受后续 `registry.session.updated` event 作为收敛来源。

## 测试决策

- 测试应断言外部可见行为，而不是内部 helper 调用顺序。
- 后端请求测试应覆盖带非空手动标题的 `session.rename`。
- 后端请求测试应覆盖空值或全空白 rename 清除手动标题。
- 后端请求测试应覆盖手动 rename 后，自动 prompt 或 agent title update 不覆盖手动标题。
- 后端请求测试应覆盖运行中 session 的 rename。
- 后端请求测试应覆盖 rename 不改变 list ordering 或 last-active timestamp。
- 后端请求测试应覆盖输入归一化和 200 字符限制。
- Registry 测试应覆盖 `session.rename` 通过与其他 session request 相同的路径转发。
- 前端 title resolver 测试应覆盖 `manual > first > last > legacy raw`。
- 前端 repository 和 workspace service 测试应覆盖 `session.rename` request method 和 payload。
- 前端 UI 测试应覆盖 session action strip 中存在 Rename action。
- 前端 UI 测试应覆盖从 settings 和 persistence 期望中移除 `Use Latest Prompt Title`。
- 现有 session archive、reload、config 和 sync 测试是 request routing 与 session summary 断言的先例。

## 非目标

- 重命名 provider-native Codex、Claude、ACP 或 app-server thread title。
- 为手动标题添加数据库 column 或 schema migration。
- 支持 per-client title preference。
- 支持标题历史、undo 或 audit log。
- 支持批量 rename 或 project-level rename rule。
- 改变 session sorting、filtering、archive behavior、reload behavior 或 read cursor behavior。
- 用另一个设置名添加 latest-prompt-title display mode。
- 在现有 session list workflow 之外构建独立标题编辑页面。

## 补充说明

关键产品规则是：手动标题状态是用户拥有的 WheelMaker metadata。自动标题数据仍可记录用于 fallback display，但在用户明确清除手动标题之前，绝不能优先于 `manual`。

本仓库未配置外部 issue tracker。该 PRD 作为本地 `ready-for-agent` 方案存放在现有 Superpowers specs 目录下。
