# 移除 Latest Prompt Title 模式

Status: ready-for-agent
Type: AFK
Category: enhancement

## 父级

`docs/issues/session-rename-title/PRD.md`

## 要构建什么

移除全局 `Use Latest Prompt Title` 模式，让未重命名 session 只有一条可预测的显示规则。Workspace 不应再暴露、持久化或基于该设置分支。没有手动标题时，session 显示应优先使用 first prompt title。

这个竖切独立于主要 rename 写路径，因为它移除旧显示模式，并简化所有 session 的标题解析。

## 验收标准

- [ ] Settings UI 不再显示 `Use Latest Prompt Title`。
- [ ] Workspace state 不再把该设置写入 durable persistence。
- [ ] 已存在的持久化值被忽略，不需要客户端迁移。
- [ ] 前端标题解析不再接收 latest-prompt mode flag。
- [ ] 未重命名 session 在 first prompt title 可用时显示 first prompt title。
- [ ] 如果 first prompt title 不可用，显示回退到 latest prompt 或 legacy raw title。
- [ ] 过去期待 latest-prompt 设置的测试被删除或更新。
- [ ] 测试覆盖未重命名 session 默认显示 first-prompt。

## 阻塞关系

无，可以立即开始。

## 覆盖用户故事

10, 11

## 评论
