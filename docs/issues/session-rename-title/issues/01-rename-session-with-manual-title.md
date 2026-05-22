# 使用手动标题重命名 Session

Status: ready-for-agent
Type: AFK

## 父级

`docs/issues/session-rename-title/PRD.md`

## 要构建什么

添加非空手动 session 标题的主端到端重命名路径。Workspace 用户应能打开 session action，输入新标题并保存，然后看到 Session Summary 使用手动标题更新。请求必须通过 Conversation Registry 使用 `session.rename` 路由到目标 Hub。

手动标题状态属于 WheelMaker 显示元数据。它必须存入现有 title facts 模型，不改变 SQLite schema；显示优先级必须高于自动 prompt 标题或 agent 标题 facts；后续自动标题更新不能覆盖它。

## 验收标准

- [ ] 带有 `sessionId` 和非空 `title` 的 `session.rename` 请求，可以通过其他 Session method 使用的同一条路由 session 请求链路成功执行。
- [ ] Registry 接受并转发 `session.rename`。
- [ ] 成功响应包含 `ok: true`、`sessionId` 和更新后的 Session Summary。
- [ ] Hub 在重命名成功后发布常规 session-updated 事件。
- [ ] 手动标题状态持久化在现有 title facts 存储中，不改变 SQLite schema。
- [ ] 手动标题显示优先于 first-prompt、latest-prompt、legacy raw 和 agent-side title facts。
- [ ] 后续自动 prompt 标题或 agent 标题更新保留手动标题。
- [ ] desktop 和 mobile 的 session action strip 都暴露 Rename action。
- [ ] 运行中的 session 也可以 Rename。
- [ ] 从重命名 dialog 保存后，使用返回的 Session Summary 或后续 session-updated 事件更新可见 session 行和选中 session 标题。
- [ ] Rename 失败时保留当前可见标题不变，并通过现有 UI 错误处理展示错误。
- [ ] 测试覆盖端到端非空 rename 路径、手动标题优先级、Registry 转发和运行中 session 允许 rename。

## 阻塞关系

无，可以立即开始。

## 覆盖用户故事

1, 2, 3, 4, 5, 8, 9, 12, 14, 17, 18, 19, 20, 21, 22

## 评论
