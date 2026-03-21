# ACP 兼容与迁移策略（WheelMaker）

更新时间：2026-03-21

## 目标

- `internal/acp/protocol.go` 只承载协议字段定义，不混入非协议运行时字段。
- 旧协议兼容逻辑统一放在 ACP 传输层归一化（normalize）中，不分散到每个 agent。

## 当前原则

1. 协议结构体不挂运行时派生字段  
`SessionUpdateParams` 仅包含协议字段：
- `sessionId`
- `update`

2. 运行时派生数据独立计算  
`ParseSessionUpdateParams` 返回 `SessionUpdateDerived` 值对象，不回写协议结构体。

3. 旧协议 -> 新协议统一入口  
只在 `NormalizeNotificationParams` 中处理 agent->client 通知兼容转换。

## 已实施兼容转换

### 1) Session Modes 旧通知

输入（旧）：
- `sessionUpdate = "current_mode_update"`
- `modeId = "..."`

归一化后（新）：
- `sessionUpdate = "config_option_update"`
- `configOptions = [{ id: "mode", category: "mode", currentValue: "<modeId>" }]`

说明：`modeId` 仅保留为 legacy 解析字段，不作为标准路径使用。

## 废弃/legacy 字段约定

- `SessionUpdate.ModeID`：legacy，仅用于兼容输入解析。
- `UpdateModeChange`：legacy 事件类型，标准路径应使用 `UpdateConfigOption`。

## 后续建议

1. 若未来出现更多历史包袱字段，继续只在 normalize 层接入。
2. 避免在 `agent.Agent` 接口增加协议兼容钩子，保持 agent 层职责为“连接工厂”。
