# 飞书 Bot 协议摘要

> 整理日期：2026-03-14
> 官方文档：https://open.feishu.cn/document/client-docs/bot-v3/bot-overview
> Go SDK：https://github.com/go-lark/lark

## 1. Bot 类型

| 类型 | 说明 | 适用场景 |
|------|------|----------|
| **自定义应用 Bot** | 在飞书开放平台创建应用后关联的机器人 | WheelMaker 使用此类型 |
| Webhook 通知 Bot | 只能发消息，无法接收 | 单向通知 |

## 2. 接收消息的两种模式

### 2.1 Webhook 事件订阅（传统）

- 飞书服务器主动 POST 事件到开发者配置的 URL
- **需要公网可访问地址**（或 ngrok/内网穿透）
- 需要开发者服务器进行签名校验

### 2.2 WebSocket 长连接（推荐，WheelMaker 采用）

- **开发者主动连接**飞书 WebSocket 网关
- **无需公网 IP**，适合本地开发机
- 飞书通过此 channel 推送事件
- 使用 go-lark SDK 开箱即用

官方文档：https://open.feishu.cn/document/uAjLw4CM/ukTMukTMukTM/event-subscription-guide/long-connection-mode

## 3. 应用创建步骤

1. 进入 [飞书开放平台](https://open.feishu.cn/)，创建企业自建应用
2. 在「机器人」功能中启用 Bot
3. 获取 `App ID` 和 `App Secret`
4. 配置事件订阅：选择「使用长连接接收事件」
5. 订阅事件：`im.message.receive_v1`（接收消息）
6. 配置权限：
   - `im:message` — 获取消息内容
   - `im:message:send_as_bot` — 发送消息

## 4. 消息格式

### 4.1 接收消息事件（im.message.receive_v1）

```json
{
  "schema": "2.0",
  "header": {
    "event_id": "...",
    "event_type": "im.message.receive_v1",
    "app_id": "cli_xxx"
  },
  "event": {
    "sender": {
      "sender_id": { "open_id": "ou_xxx", "user_id": "xxx" },
      "sender_type": "user"
    },
    "message": {
      "message_id": "om_xxx",
      "chat_id": "oc_xxx",
      "chat_type": "p2p",
      "message_type": "text",
      "content": "{\"text\":\"Hello Bot\"}"
    }
  }
}
```

### 4.2 发送消息

**发送文本**：
```
POST https://open.feishu.cn/open-apis/im/v1/messages?receive_id_type=chat_id

{
  "receive_id": "oc_xxx",
  "msg_type": "text",
  "content": "{\"text\":\"Hello!\"}"
}
```

**发送富文本卡片**：
```
{
  "msg_type": "interactive",
  "content": "{...飞书交互卡片 JSON...}"
}
```

## 5. Go SDK 使用（go-lark）

### 安装

```bash
go get github.com/go-lark/lark/v2
```

### 初始化 Bot

```go
import "github.com/go-lark/lark/v2"

bot := lark.NewChatBot("<App ID>", "<App Secret>")
// 国际版（Lark）使用：
// bot.SetDomain(lark.DomainLark)
```

### WebSocket 长连接接收消息

```go
// 使用 WebSocket 长连接模式
// go-lark v2 支持通过 middleware 注册事件处理
// 具体 WebSocket 模式参考：https://github.com/go-lark/lark/tree/main/examples
```

### 发送消息

```go
// 发送文本
bot.PostText("Hello World", lark.WithChatID("oc_xxx"))

// 发送图片/卡片
bot.PostNotification(
    lark.NewMsgBuffer(lark.MsgText).Text("Hello").Build(),
)
```

### 添加消息 Reaction

```go
// POST /open-apis/im/v1/messages/{message_id}/reactions
// emoji_type: "THUMBSUP", "DONE" 等
```

## 6. 消息类型

| message_type | 说明 |
|-------------|------|
| `text` | 纯文本 |
| `post` | 富文本 |
| `image` | 图片 |
| `interactive` | 交互卡片 |
| `file` | 文件 |
| `audio` | 语音 |

## 7. WheelMaker 中的使用方式

```
飞书用户发消息 → WebSocket 长连接 → go-lark 事件回调
→ im/feishu.Adapter.OnMessage handler
→ im.Message{ChatID, MessageID, UserID, Text}
→ Hub.HandleMessage()
```

回复时：
```
Hub → im/feishu.Adapter.SendText(chatID, text)
→ 飞书 API POST /im/v1/messages
→ 飞书用户收到消息
```

## 8. 注意事项

- **消息去重**：飞书事件可能重复投递，需通过 `event_id` 去重
- **At Bot**：群聊中 Bot 只处理 @ 了自己的消息（或 P2P 消息）
- **消息长度**：单条文本消息上限约 30000 字符，超出需分段发送
- **频率限制**：发消息 API 有频率限制，注意控制流速
- **Token 刷新**：`tenant_access_token` 有效期 2 小时，SDK 会自动刷新
