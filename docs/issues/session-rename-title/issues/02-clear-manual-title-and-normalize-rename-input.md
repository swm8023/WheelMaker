# 清除手动标题并归一化重命名输入

Status: ready-for-agent
Type: AFK

## 父级

`docs/issues/session-rename-title/PRD.md`

## 要构建什么

扩展 rename 路径，让用户可以清除手动标题，并保证标题输入适合在 session list 中显示。保存空字符串或全空白标题时，应移除手动标题状态并恢复自动显示行为。非空输入应在前端和后端输入边界做归一化。

结果必须保持标题语义确定：清除手动标题后，显示优先级回到 first prompt title；只有 first prompt title 不可用时，才回退到 latest 或 legacy 标题。

## 验收标准

- [ ] 保存空标题会清除手动标题状态。
- [ ] 保存全空白输入会清除手动标题状态。
- [ ] 清除手动标题后，如果 first prompt title 可用，显示标题回退到 first prompt title。
- [ ] 清除后如果 first prompt title 不可用，显示标题回退到 latest prompt 或 legacy raw title。
- [ ] Rename 输入会 trim 外层空白。
- [ ] 粘贴的多行输入会被归一化为单行标题。
- [ ] 后端校验执行与前端一致的边界语义。
- [ ] 标题长度上限为 200 字符。
- [ ] 超过限制的标题不能创建超过 200 字符的已存储标题或已显示标题。
- [ ] 测试覆盖空标题清除、全空白清除、fallback 显示、换行归一化、trim 行为和 200 字符限制。

## 阻塞关系

- `docs/issues/session-rename-title/issues/01-rename-session-with-manual-title.md`

## 覆盖用户故事

6, 7, 15, 16

## 评论
