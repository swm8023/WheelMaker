# Issue Tracker：本地 Markdown

本仓库的 issue 和 PRD 以 markdown 文件形式存放在 `docs/issues/`。

## 约定

- 每个功能一个目录：`docs/issues/<feature-slug>/`
- 需要 PRD 副本或父级引用时，放在 `docs/issues/<feature-slug>/PRD.md`。
- 实现 issue 放在 `docs/issues/<feature-slug>/issues/` 下。
- issue 文件从 `01` 开始编号，命名为 `NN-<slug>.md`。
- triage 状态写在每个 issue 文件顶部附近的 `Status:` 行。
- 评论和对话历史追加到文件底部的 `## 评论` 标题下。

## 当技能要求“发布到 issue tracker”

在 `docs/issues/<feature-slug>/issues/` 下创建新的 markdown issue 文件；目录不存在时先创建目录。

## 当技能要求“读取相关 ticket”

读取被引用的 markdown 文件。用户通常会直接给出路径或 issue 编号。
