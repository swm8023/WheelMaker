# Session Recovery / Resume / Reload 整理设计

日期：2026-05-05
状态：已确认，可直接实现

## 背景

当前会话恢复相关能力已经具备基本可用性，但代码和职责分布较为混乱：

- 服务端逻辑分散在 `client.go`、`claude_sessions.go`、`disk_sessions.go`、`session.go` 等多个文件
- `resume.list`、`resume.import`、`reload` 三条链路使用了相近但不一致的数据模型和过滤规则
- Claude / Codex / Copilot 的差异逻辑与公共流程交织，导致排查问题时需要跨多个文件追踪
- `reload` 清理了持久化 prompt，但没有明确重置 recorder 的对应内存 prompt state，容易造成 prompt/turn 索引错位
- 前端 resume UI 目前是侧边栏中的内嵌卡片，取消按钮弱，流程终点只做 import，不会直接得到已回灌的历史

之前围绕 resume 和 reload 的多次往复提交，本质上是在现有结构上持续补洞，而不是先把恢复链路的边界整理干净。

## 目标

1. 将所有服务端 session recovery 相关逻辑收口到单一 `session_recovery.go` 文件。
2. 保持 SQLite 会话记录仍然是 WheelMaker 内部唯一会话模型，外部原生会话只作为导入与重载的数据来源。
3. 提取 resume / import / reload 的公共流程，差异部分通过 interface 抽象。
4. 修复导致反复提交的恢复链路 bug，尤其是 reload 后 recorder 状态不一致的问题。
5. 改进前端 resume UI，使其更像一个明确的弹出流程，而不是列表中的临时卡片。
6. 将前端“Resume”语义调整为“导入并立即 reload”，用户无需再手动执行第二步。

## 非目标

1. 不修改 SQLite 的整体存储模型，不引入“外部原生会话为一等内部模型”的新架构。
2. 不重做 chat 列表、会话详情页或整体页面布局。
3. 不新增额外的 agent recovery 子文件，例如 `session_recovery_codex.go`。
4. 不重构与 session recovery 无关的 ACP、IM、registry 基础设施。

## 设计总览

服务端新增单一恢复协调层：`server/internal/hub/client/session_recovery.go`。

该文件负责三类能力：

1. 统一入口：
   - `ListResumableSessions`
   - `ImportResumableSession`
   - `ReloadSession`
2. 统一公共流程：
   - managed session 过滤
   - CWD 匹配与规范化
   - session 元数据排序、映射、响应组装
   - import 后 SQLite session record 创建
   - reload 时 replay 捕获与 recorder 回灌
3. agent 差异适配：
   - Claude
   - Codex
   - Copilot

所有 agent 差异实现都保留在同一个 `session_recovery.go` 文件内，通过私有 `source` 结构体和 interface 区分，不再继续散落到独立文件。

## 服务端设计

### 1. 统一 recovery 模型

引入中性命名的数据结构，替换继续复用 `ClaudeSessionInfo` 的做法。

建议字段：

- `SessionID string`
- `AgentType string`
- `Title string`
- `Preview string`
- `UpdatedAt string`
- `MessageCount int`
- `CWD string`

命名原则：

- 不再使用 `ClaudeSessionInfo` 这种对多 agent 不准确的命名
- 返回前端时仍映射为现有 resumable session DTO，避免前后端协议无谓扩散

### 2. 单一 recovery source 接口

在 `session_recovery.go` 内定义私有接口，例如：

```go
type recoverySource interface {
    AgentType() string
    List(projectCWD string, managedIDs map[string]bool) ([]recoverySession, error)
    Find(projectCWD, sessionID string, managedIDs map[string]bool) (*recoverySession, error)
}
```

设计原则：

- `source` 只负责发现外部 session，以及从对应磁盘格式中提取元数据
- `source` 不负责保存 SQLite，不负责 replay，不负责返回前端协议
- `source` 的实现全部留在同一个 `session_recovery.go` 文件中

### 3. recovery 协调层职责

`session_recovery.go` 作为公共协调层，统一持有并编排：

- source 选择
- managed session 加载
- session 排序
- import 后持久化
- reload 流程
- replay importer

`client.go` 中 `session.resume.list`、`session.resume.import`、`session.reload` 的 request handler 仅保留：

1. payload decode
2. 参数校验
3. 调用 recovery 协调层
4. 返回结果

这样 `client.go` 不再承载恢复链路的业务编排。

### 4. 文件收口原则

本次实现后，以下 recovery 相关逻辑应从原位置移出或折叠：

- `claude_sessions.go` 中的恢复扫描逻辑
- `disk_sessions.go` 中的 Codex / Copilot 扫描逻辑
- `client.go` 中的 resume.list / resume.import / session.reload 主流程
- `client.go` 中的 `feedReplayToRecorder`

目标状态是：

- `session_recovery.go` 成为会话恢复相关逻辑的唯一主入口
- 原文件只保留与 recovery 无关的通用 session/client 能力

### 5. reload 流程下沉

`reload` 不再由 `client.go` 零散拼流程，而改为 recovery 层统一执行如下步骤：

1. 删除该 session 的持久化 prompts
2. 删除 recorder 的该 session 内存 prompt state
3. 解析并获取目标 session
4. suspend 当前 session
5. 触发 replay capture
6. 将 replay updates 送入统一 replay importer
7. 返回成功结果

将 recorder 状态清理与 replay 回灌纳入同一层，可避免只清理一半状态。

## 服务端逻辑与 bug 修复

### 1. `resume.list` 统一化

当前问题：

- agent 分流逻辑写在 `client.go`
- Claude / disk scan 的 fallback 规则混在一起
- 公共排序和结果映射没有集中

本次改为：

1. recovery 协调层先读取 managed IDs
2. 按 `agentType` 精确选择 source
3. source 返回统一 `recoverySession`
4. recovery 层完成排序与响应映射

规则：

- `codex` 只走 Codex source
- `copilot` 只走 Copilot source
- `claude` 只走 Claude source
- 不再允许 Codex / Copilot 意外落回 Claude 扫描逻辑

这样可以彻底消除“列表查找时 agent 边界不干净”的问题。

### 2. `resume.import` 统一化

当前问题：

- import 的查找逻辑与 list 使用不同规则
- import 里又拼了一套“disk first + Claude fallback”
- managed 冲突检查和 list 的过滤不完全同源

本次改为：

1. recovery 层选择 source
2. 调用统一 `Find(projectCWD, sessionID, managedIDs)`
3. source 只负责精确查找 session
4. recovery 层负责判断：
   - session 是否存在
   - session 是否已托管
   - 是否应保存为 SQLite session record
5. recovery 层统一返回 session summary

这样可以修复：

- “列表里看得到但导不进”
- “已经托管还可重复导”
- “不同 agent import 行为不一致”

### 3. `reload` 清理逻辑修复

这是本次必须修的核心问题。

当前 `session.reload` 已经删除了数据库中的 session prompts，但没有明确清理 recorder 中该 session 的内存 prompt state。结果是：

- DB 中 prompt 序号被清空
- 内存中 prompt/turn 续写位置还在旧值
- replay 回灌后容易从错误的 promptIndex / turnIndex 继续

这类问题非常容易表现为：

- 某次 reload 看似成功
- 后续再 reload 或继续聊天时顺序错乱
- 修一处又在别处复现

修复方案：

- 在 reload 入口显式调用 recorder 的单-session prompt state 清理能力
- 该清理必须与 `DeleteSessionPrompts` 同属于 recovery 的统一流程
- 只有在“DB 清理 + 内存清理”都完成后，才进入 replay capture

### 4. replay importer 收口

当前 `feedReplayToRecorder` 在 `client.go` 中，语义上已经属于 recovery 流程的一部分，但位置与职责不匹配。

本次改为：

- 将 replay importer 迁入 `session_recovery.go`
- 作为 `ReloadSession` 的内部步骤，不再作为 `client.go` 的通用 helper

它的职责明确为两件事：

1. 根据 replay 的 `session/update` 流识别 prompt 边界
2. 将 replay updates 转为 recorder 能消费的统一事件流

过滤规则也收口到同一处，统一决定：

- 哪些 update 用于 prompt 起止判断
- 哪些 update 只更新 config / commands / session info，不应写入消息历史
- 哪些 update 参与最终消息历史回灌

### 5. 差异与公共逻辑边界

本次明确三层职责：

1. **source**
   - 发现外部 session
   - 从各自的磁盘格式中提取 metadata
2. **recovery**
   - 将外部 session 转为 WheelMaker 的内部 session
   - 负责 list / import / reload 的业务编排
3. **recorder**
   - 将 replay 后的事件流转成内部消息历史

这样每一层的修改范围才清晰，不会再出现扫描逻辑、恢复逻辑、消息回灌逻辑互相越界。

## 前端设计

### 1. Resume UI 由内嵌卡片改为明确弹出层

当前 resume picker 是 chat 侧边栏中的一张嵌入式卡片，视觉上更像列表中的一个临时块，而不是一个明确的操作流程。

本次改为：

- 仍然限制在 chat 区域内，不做复杂全屏 modal
- 但样式调整为更明显的弹出层/浮层感
- 让用户直观感知自己处于“恢复会话流程”中

### 2. 关闭按钮放到右上角

当前底部的纯文本 `Cancel` 交互存在感弱。

本次改为：

- 在弹出层头部右上角放关闭按钮
- 使用现有 icon button / secondary button 风格
- 需要有明显边框、hover、点击反馈
- 底部不再保留额外的 `Cancel` 文本按钮

### 3. 保留两段式恢复流程

流程仍然分两步：

1. 选择 agent
2. 展示该 agent 的可恢复 session 列表

但第二步需要补一个“返回上一步/切换 agent”的入口，避免用户只能关闭后重来。

### 4. Resume 语义调整为“导入并立即 reload”

当前问题：

- Resume 只完成 import
- 导入后用户还需要再执行一次 reload，才会真正看到已回灌的历史

本次改为：

1. 用户点击某个 resumable session
2. 前端调用 `session.resume.import`
3. import 成功后立即调用 `session.reload`
4. reload 成功后：
   - 选中该 session
   - 清空旧缓存
   - 关闭 resume 弹层
   - 直接加载并展示回灌后的消息

对用户而言，`Resume` 直接等于“恢复到可阅读状态”。

### 5. 已托管 session 的 Reload 语义保持不变

chat 列表中 swipe 露出的 `Reload` 动作继续保留，语义定义为：

- 对已托管 session 重新从外部原生会话回灌历史

调整后的产品语义为：

- **Resume**：导入并立即 reload
- **Reload**：对已托管 session 再次回灌

这样两者不再看起来功能重复但行为不一致。

### 6. 错误处理

前端错误语义需要明确：

1. import 失败：
   - 保持在 resume 弹层
   - 直接显示错误
2. import 成功但 reload 失败：
   - 不能假装 resume 已成功完成
   - 明确提示“导入成功，但加载历史失败”之类的错误
   - 允许用户稍后再对该已托管 session 执行 reload

这能避免“用户看见 session 进入列表，就误以为恢复已经完整完成”。

## 协议与兼容性

本次尽量保持现有前后端 RPC 方法名不变：

- `session.resume.list`
- `session.resume.import`
- `session.reload`

变化主要体现在：

- 服务端内部结构重组
- 前端对 `resume.import` 的调用后追加自动 `reload`

如无必要，不新增额外 RPC。

## 测试计划

### 服务端

补充或调整现有 `client_test.go` 中的测试，重点覆盖：

1. `session.resume.list` 对不同 agent 精确分流，不发生错误 fallback
2. `session.resume.import` 与 `session.resume.list` 使用一致的过滤/查找规则
3. 已托管 session 不会重复导入
4. `session.reload` 会同时清理：
   - 持久化 prompt
   - recorder 内存 prompt state
5. reload 后 replay 回灌的 prompt / turn 索引从正确位置重新开始
6. replay importer 正确忽略 config / command / session-info 这类非消息更新

### 前端

补充或调整现有 web 测试，重点覆盖：

1. resume 弹层右上角关闭按钮存在且可关闭
2. 选择 agent 后可展示 resumable session 列表
3. 点击 resumable session 后会顺序执行：
   - import
   - reload
4. reload 成功后会直接加载消息
5. import 成功但 reload 失败时错误会正确展示
6. 原有 swipe reload 行为不受影响

## 风险

1. `session_recovery.go` 会变大，但这是本次有意的收口结果，目的是统一恢复链路边界，而不是继续横向分散。
2. 迁移逻辑时如果遗漏旧 helper 的隐式行为，可能引入短期回归，因此测试必须围绕 list/import/reload 全链路补齐。
3. 前端将 Resume 改成 import+reload 后，恢复动作时长可能比现在更长，需要明确 loading 态，避免重复点击。

## 建议实施顺序

1. 在服务端引入统一 recovery 模型与 `recoverySource` 接口
2. 将 Claude / Codex / Copilot 扫描逻辑迁入 `session_recovery.go`
3. 将 `session.resume.list`、`session.resume.import` 主流程迁入 recovery 协调层
4. 将 `session.reload` 与 replay importer 迁入 recovery 协调层
5. 补上 recorder state 清理，修复 reload 索引错位
6. 调整前端 resume 弹层结构与右上角关闭按钮
7. 将前端 Resume 改为 import 后自动 reload
8. 补齐服务端与前端测试
