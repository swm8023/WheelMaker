# 飞书 Bot 协议与 SDK 详细参考

> 整理日期：2026-03-17
> 官方文档：https://open.feishu.cn/document/client-docs/bot-v3/bot-overview
> 社区 SDK (go-lark)：https://github.com/go-lark/lark
> 官方 Go SDK：https://github.com/larksuite/oapi-sdk-go

---

## 1. Bot 类型

| 类型 | 说明 | 适用场景 |
|------|------|----------|
| **自定义应用 Bot** | 在飞书开放平台创建应用后关联的机器人 | WheelMaker 使用此类型 |
| Webhook 通知 Bot | 只能发消息，无法接收 | 单向通知 |

---

## 2. 接收消息的两种模式

### 2.1 Webhook 事件订阅（传统）

- 飞书服务器主动 POST 事件到开发者配置的 URL
- **需要公网可访问地址**（或 ngrok/内网穿透）
- 需要对请求进行签名校验
- 适合生产环境部署在有固定 IP 的服务器上

### 2.2 WebSocket 长连接（推荐，WheelMaker 采用）

- **开发者主动连接**飞书 WebSocket 网关
- **无需公网 IP**，适合本地开发机
- 飞书通过此 channel 推送事件
- 使用官方 SDK `github.com/larksuite/oapi-sdk-go/v3` 的 `ws` 包实现
- **注意**：社区 SDK `go-lark` 不支持 WebSocket，仅支持 HTTP webhook 模式

官方文档：https://open.feishu.cn/document/uAjLw4CM/ukTMukTMukTM/event-subscription-guide/long-connection-mode

---

## 3. 应用创建步骤

1. 进入 [飞书开放平台](https://open.feishu.cn/)，创建企业自建应用
2. 在「机器人」功能中启用 Bot
3. 获取 `App ID` 和 `App Secret`
4. 配置事件订阅：选择「使用长连接接收事件」
5. 订阅事件：`im.message.receive_v1`（接收消息）
6. 配置权限：
   - `im:message` — 获取消息内容
   - `im:message:send_as_bot` — 发送消息

---

## 4. SDK 分工

WheelMaker 同时使用两个 SDK：

| SDK | 用途 | 模块路径 |
|-----|------|----------|
| `go-lark` | **发送消息**、消息构建、用户/群组 API | `github.com/go-lark/lark` |
| `larksuite/oapi-sdk-go/v3` | **接收事件**（WebSocket 长连接） | `github.com/larksuite/oapi-sdk-go/v3` |

安装：
```bash
go get github.com/go-lark/lark
go get github.com/larksuite/oapi-sdk-go/v3
```

---

## 5. WebSocket 长连接（官方 SDK）

### 5.1 完整示例

```go
package main

import (
    "context"
    "fmt"
    "os"

    larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
    "github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
    larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
    larkws "github.com/larksuite/oapi-sdk-go/v3/ws"
)

func main() {
    eventHandler := dispatcher.NewEventDispatcher("", "").
        OnP2MessageReceiveV1(func(ctx context.Context, event *larkim.P2MessageReceiveV1) error {
            fmt.Printf("message: %s\n", larkcore.Prettify(event))
            return nil
        })

    cli := larkws.NewClient(os.Getenv("APP_ID"), os.Getenv("APP_SECRET"),
        larkws.WithEventHandler(eventHandler),
        larkws.WithLogLevel(larkcore.LogLevelInfo),
    )

    // Start blocks; run in goroutine for daemon use
    if err := cli.Start(context.Background()); err != nil {
        panic(err)
    }
}
```

### 5.2 Client 构造与选项

```go
cli := larkws.NewClient(appID, appSecret string, opts ...ClientOption) *Client

// Available options:
larkws.WithEventHandler(handler *dispatcher.EventDispatcher)
larkws.WithLogLevel(larkcore.LogLevel)   // LogLevelDebug / Info / Warn / Error
larkws.WithLogger(larkcore.Logger)
larkws.WithAutoReconnect(bool)           // default: true, unlimited retries
larkws.WithDomain(domain string)         // default: Feishu; use larkcore.FeishuBaseUrl / LarkBaseUrl

// Start (blocking):
err := cli.Start(ctx context.Context) error
```

### 5.3 EventDispatcher — 事件注册

```go
d := dispatcher.NewEventDispatcher(verifyToken, encryptKey string)

// Common handlers:
d.OnP2MessageReceiveV1(func(ctx context.Context, event *larkim.P2MessageReceiveV1) error { ... })
d.OnP2MessageReadV1(func(ctx context.Context, event *larkim.P2MessageReadV1) error { ... })
d.OnP2ChatMemberBotAddedV1(func(...) error { ... })
d.OnP2ChatMemberBotDeletedV1(func(...) error { ... })
d.OnP2ChatMemberUserAddedV1(func(...) error { ... })
d.OnP2ChatMemberUserDeletedV1(func(...) error { ... })
d.OnP2ChatUpdatedV1(func(...) error { ... })
d.OnP2ChatDisbandedV1(func(...) error { ... })

// Custom / unknown event type:
d.OnCustomizedEvent(eventType string, func(ctx context.Context, event *larkevent.EventReq) error { ... })
```

### 5.4 P2MessageReceiveV1 消息结构

```go
type P2MessageReceiveV1 struct {
    Sender  *P2MessageReceiveV1Sender
    Message *P2MessageReceiveV1Message
}

type P2MessageReceiveV1Sender struct {
    SenderId   *UserId    // OpenId, UserId, UnionId
    SenderType *string    // "user"
    TenantKey  *string
}

type P2MessageReceiveV1Message struct {
    MessageId   *string
    RootId      *string
    ParentId    *string
    CreateTime  *string
    UpdateTime  *string
    ChatId      *string
    ChatType    *string    // "p2p" | "group"
    ThreadId    *string
    MessageType *string    // "text" | "post" | "image" | "interactive" | ...
    Content     *string    // JSON string; use json.Unmarshal to parse
    UserAgent   *string
    Mentions    []*P2MessageMention
}

type P2MessageMention struct {
    Key       *string
    Id        *UserId
    Name      *string
    TenantKey *string
}
```

解析文本消息 Content：
```go
var textContent struct {
    Text string `json:"text"`
}
json.Unmarshal([]byte(*event.Message.Content), &textContent)
```

### 5.5 连接机制

- 客户端 POST 到 `/callback/ws/endpoint` 获取签名 WebSocket URL
- 通过 `gorilla/websocket` 建立连接
- 后台 `pingLoop` 按服务端协商间隔发送心跳
- 自动重连：默认开启，无限次，间隔 2 分钟，首次重连随机 0~30s 抖动
- 大消息（multi-part frames）在 5 秒 TTL 缓存中重组后再分发
- 不可重试错误：403 Forbidden、514 AuthFailed（超出连接数限制）

---

## 6. 发送消息（go-lark）

### 6.1 Bot 初始化

```go
import "github.com/go-lark/lark"

bot := lark.NewChatBot("<App ID>", "<App Secret>")
bot.StartHeartbeat()   // 启动 TenantAccessToken 自动续期（在 (Expire-20)s 时刷新）
defer bot.StopHeartbeat()

// 国际版（Lark）：
bot.SetDomain(lark.DomainLark)  // DomainFeishu = "https://open.feishu.cn"
                                 // DomainLark   = "https://open.larksuite.com"
```

### 6.2 MsgBuffer — 链式消息构建器

```go
buf := lark.NewMsgBuffer(msgType string)

// 绑定接收方（必须调用一个）：
buf.BindChatID(chatID string) *MsgBuffer     // 群/P2P chat_id
buf.BindOpenID(openID string) *MsgBuffer
buf.BindUserID(userID string) *MsgBuffer
buf.BindUnionID(unionID string) *MsgBuffer
buf.BindEmail(email string) *MsgBuffer
buf.BindReply(rootID string) *MsgBuffer      // 设置回复的父消息 ID

// 设置内容（与 MsgType 对应）：
buf.Text(text string) *MsgBuffer
buf.Post(postContent *PostContent) *MsgBuffer
buf.Card(cardContent string) *MsgBuffer      // JSON 字符串
buf.Template(tc *TemplateContent) *MsgBuffer
buf.Image(imageKey string) *MsgBuffer
buf.File(fileKey string) *MsgBuffer
buf.Audio(fileKey string) *MsgBuffer
buf.Media(fileKey, imageKey string) *MsgBuffer
buf.Sticker(fileKey string) *MsgBuffer
buf.ShareChat(chatID string) *MsgBuffer
buf.ShareUser(userID string) *MsgBuffer

// 其他选项：
buf.ReplyInThread(bool) *MsgBuffer
buf.WithSign(secret string, ts int64) *MsgBuffer  // webhook 签名
buf.WithUUID(uuid string) *MsgBuffer              // 幂等 key

// 完成：
msg := buf.Build() // returns OutcomingMessage
err := buf.Error() // 检查类型不匹配错误
buf.Clear()
```

消息类型常量：
```go
const (
    MsgText        = "text"
    MsgPost        = "post"         // 富文本
    MsgInteractive = "interactive"  // 交互卡片
    MsgImage       = "image"
    MsgShareCard   = "share_chat"
    MsgShareUser   = "share_user"
    MsgAudio       = "audio"
    MsgMedia       = "media"
    MsgFile        = "file"
    MsgSticker     = "sticker"
)
```

### 6.3 发送 API

```go
// 发送新消息
bot.PostMessage(om OutcomingMessage) (*PostMessageResponse, error)

// 回复消息
bot.ReplyMessage(om OutcomingMessage) (*PostMessageResponse, error)

// 更新消息（仅 interactive 类型）
bot.UpdateMessage(messageID string, om OutcomingMessage) (*UpdateMessageResponse, error)

// 撤回消息
bot.RecallMessage(messageID string) (*RecallMessageResponse, error)

// 转发消息
bot.ForwardMessage(messageID string, receiveID *OptionalUserID) (*ForwardMessageResponse, error)

// 获取消息
bot.GetMessage(messageID string) (*GetMessageResponse, error)

// 已读回执
bot.MessageReadReceipt(messageID string) (*MessageReceiptResponse, error)

// Pin/Unpin
bot.PinMessage(messageID string) (*PinMessageResponse, error)
bot.UnpinMessage(messageID string) (*UnpinMessageResponse, error)

// Reaction
bot.AddReaction(messageID string, emojiType EmojiType) (*ReactionResponse, error)
bot.DeleteReaction(messageID, reactionID string) (*ReactionResponse, error)

// Ephemeral（仅群聊，只有指定用户可见）
bot.PostEphemeralMessage(om OutcomingMessage) (*PostEphemeralMessageResponse, error)
bot.DeleteEphemeralMessage(messageID string) (*DeleteEphemeralMessageResponse, error)

// 加急（催办）
bot.BuzzMessage(buzzType, messageID string, userIDList ...string) (*BuzzMessageResponse, error)
// buzzType: lark.BuzzTypeInApp | BuzzTypeSMS | BuzzTypePhone
```

### 6.4 快捷发送方法

```go
bot.PostText(text string, userID *OptionalUserID)
bot.PostRichText(postContent *PostContent, userID *OptionalUserID)
bot.PostTextMention(text, atUserID string, userID *OptionalUserID)
bot.PostTextMentionAll(text string, userID *OptionalUserID)
bot.PostTextMentionAndReply(text, atUserID string, userID *OptionalUserID, replyID string)
bot.PostImage(imageKey string, userID *OptionalUserID)
bot.PostShareChat(chatID string, userID *OptionalUserID)
bot.PostShareUser(openID string, userID *OptionalUserID)
```

`OptionalUserID` 构造：
```go
lark.WithChatID(chatID string)   // oc_xxx
lark.WithOpenID(openID string)   // ou_xxx
lark.WithUserID(userID string)
lark.WithUnionID(unionID string)
lark.WithEmail(email string)
```

### 6.5 典型用法

发送文本到群聊：
```go
msg := lark.NewMsgBuffer(lark.MsgText).
    BindChatID("oc_xxxx").
    Text("Hello World").
    Build()
resp, err := bot.PostMessage(msg)
```

回复消息：
```go
msg := lark.NewMsgBuffer(lark.MsgText).
    BindChatID("oc_xxxx").
    BindReply(rootMessageID).
    Text("Reply content").
    Build()
resp, err := bot.ReplyMessage(msg)
```

---

## 7. MsgTextBuilder — 富文本构建（带 @ 功能）

```go
tb := lark.NewTextBuilder()
tb.Text(text ...interface{}) *MsgTextBuilder      // 追加文本
tb.Textln(text ...interface{}) *MsgTextBuilder    // 追加文本 + 换行
tb.Textf(format string, args ...interface{}) *MsgTextBuilder
tb.Mention(userID string) *MsgTextBuilder         // @指定用户
tb.MentionAll() *MsgTextBuilder                   // @所有人
tb.Render() string                                // 输出最终字符串
tb.Len() int
tb.Clear()

// 与 MsgBuffer 配合：
text := lark.NewTextBuilder().
    Text("Hello ").
    Mention("ou_xxxx").
    Text(" done!").
    Render()
msg := lark.NewMsgBuffer(lark.MsgText).BindChatID("oc_xxxx").Text(text).Build()
```

---

## 8. MsgPostBuilder — 富文本（post 类型）

```go
pb := lark.NewPostBuilder()
pb.Title(title string) *MsgPostBuilder
pb.WithLocale(locale string) *MsgPostBuilder   // 默认 "zh_cn"
pb.TextTag(text string, lines int, unescape bool) *MsgPostBuilder
pb.LinkTag(text, href string) *MsgPostBuilder
pb.AtTag(text, userID string) *MsgPostBuilder
pb.ImageTag(imageKey string, width, height int) *MsgPostBuilder
pb.Render() *PostContent

// Locale 常量：
lark.LocaleZhCN = "zh_cn"
lark.LocaleZhHK = "zh_hk"
lark.LocaleZhTW = "zh_tw"
lark.LocaleEnUS = "en_us"
lark.LocaleJaJP = "ja_jp"
```

---

## 9. CardBuilder — 交互卡片

```go
cb := lark.NewCardBuilder()

// 顶层卡片
cb.Card(elements ...card.Element) *card.Block

// 内容块
cb.Text(s string) *card.TextBlock
cb.Markdown(s string) *card.MarkdownBlock
cb.Img(key string) *card.ImgBlock
cb.Div(fields ...*card.FieldBlock) *card.DivBlock
cb.Field(text *card.TextBlock) *card.FieldBlock
cb.Hr() *card.HrBlock
cb.Note() *card.NoteBlock

// 交互元素
cb.Button(text *card.TextBlock) *card.ButtonBlock
cb.Action(actions ...card.Element) *card.ActionBlock
cb.SelectMenu(options ...*card.OptionBlock) *card.SelectMenuBlock
cb.Overflow(options ...*card.OptionBlock) *card.OverflowBlock
cb.Option(value string) *card.OptionBlock
cb.Confirm(title, text string) *card.ConfirmBlock
cb.DatePicker() *card.DatePickerBlock
cb.TimePicker() *card.TimePickerBlock
cb.DatetimePicker() *card.DatetimePickerBlock

// 布局
cb.Column(elements ...card.Element) *card.ColumnBlock
cb.ColumnSet(columns ...*card.ColumnBlock) *card.ColumnSetBlock
cb.URL() *card.URLBlock

// i18n 支持
cb.I18N.Card(blocks ...*i18n.LocalizedBlock)
cb.I18N.WithLocale(locale string, elements ...card.Element)
```

卡片 JSON 序列化后通过 `MsgBuffer.Card(jsonStr)` 传入：
```go
cardJSON, _ := json.Marshal(cb.Card(...))
msg := lark.NewMsgBuffer(lark.MsgInteractive).
    BindChatID("oc_xxxx").
    Card(string(cardJSON)).
    Build()
```

### 卡片模板

```go
tb := lark.NewTemplateBuilder()
content := tb.BindTemplate(
    templateID string,
    versionName string,
    variables map[string]interface{},  // 模板变量，无则传 nil
) *TemplateContent

msg := lark.NewMsgBuffer(lark.MsgInteractive).
    BindChatID("oc_xxxx").
    Template(content).
    Build()
```

---

## 10. 交互卡片回调事件

飞书用户点击卡片按钮后，服务端会收到回调（通过 Webhook POST 或 WebSocket）：

```go
type EventCardCallback struct {
    AppID     string
    TenantKey string
    Token     string
    OpenID    string
    UserID    string
    MessageID string
    ChatID    string
    Action    EventCardAction
}

type EventCardAction struct {
    Tag      string          // "button" | "overflow" | "select_static" | "select_person" | "datepicker"
    Option   string          // Overflow / SelectMenu 的选中值
    Timezone string          // DatePicker 使用
    Value    json.RawMessage // 元素自定义 value，需自行 json.Unmarshal
}
```

---

## 11. 文件与图片上传

```go
bot.UploadImage(path string) (*UploadImageResponse, error)
bot.UploadImageObject(img image.Image) (*UploadImageResponse, error)
bot.UploadFile(req UploadFileRequest) (*UploadFileResponse, error)

type UploadFileRequest struct {
    FileType string      // "opus" | "mp4" | "pdf" | "doc" | "xls" | "ppt" | "stream"
    FileName string
    Duration int         // 视频文件时长（毫秒）
    Path     string      // 本地文件路径（与 Reader 二选一）
    Reader   io.Reader
}

// 返回值：
// UploadImageResponse.Data.ImageKey — 用于发送图片消息的 image_key
// UploadFileResponse.Data.FileKey   — 用于发送文件消息的 file_key
```

---

## 12. 群组管理 API

```go
bot.GetChat(chatID string) (*GetChatResponse, error)
bot.ListChat(sortType, pageToken string, pageSize int) (*ListChatResponse, error)
bot.SearchChat(query, pageToken string, pageSize int) (*ListChatResponse, error)
bot.CreateChat(req CreateChatRequest) (*CreateChatResponse, error)
bot.UpdateChat(chatID string, req UpdateChatRequest) (*UpdateChatResponse, error)
bot.DeleteChat(chatID string) (*DeleteChatResponse, error)
bot.JoinChat(chatID string) (*JoinChatResponse, error)
bot.IsInChat(chatID string) (*IsInChatResponse, error)
bot.GetChatMembers(chatID, pageToken string, pageSize int) (*GetChatMembersResponse, error)
bot.AddChatMember(chatID string, idList []string) (*AddChatMemberResponse, error)
bot.RemoveChatMember(chatID string, idList []string) (*RemoveChatMemberResponse, error)
bot.SetTopNotice(chatID, actionType, messageID string) (*SetTopNoticeResponse, error)
bot.DeleteTopNotice(chatID string) (*DeleteChatResponse, error)
```

---

## 13. 用户信息 API

```go
bot.GetUserInfo(userID *OptionalUserID) (*GetUserInfoResponse, error)
bot.BatchGetUserInfo(userIDType string, userIDs ...string) (*BatchGetUserInfoResponse, error)
// 最多 50 个用户/次

// UserInfo 字段：
// OpenID, UserID, ChatID, Email, Mobile, Name, Gender,
// Avatar (Avatar72/240/640/Origin), EmployeeNo, JobTitle,
// DepartmentIDs, Status (IsFrozen/IsResigned/IsActivated)
```

---

## 14. Bot 信息

```go
bot.GetBotInfo() (*GetBotInfoResponse, error)
// Bot 字段：ActivateStatus, AppName, AvatarURL, IPWhiteList, OpenID
```

---

## 15. 加密与签名

```go
// AES-CBC 解密（加密事件 payload）
key := lark.EncryptKey(encryptionKey string) []byte   // SHA256 → 32 字节
plain, err := lark.Decrypt(key []byte, data string) ([]byte, error)

// HMAC-SHA256 签名（Webhook 验证）
sig, err := lark.GenSign(secret string, timestamp int64) (string, error)
// 算法：HMAC-SHA256(timestamp + "\n" + secret)
```

---

## 16. 错误处理

```go
// 所有响应共用 BaseResponse
type BaseResponse struct {
    Code  int       // 0 = 成功，非 0 = 飞书 API 错误码
    Msg   string
    Error BaseError
}

// 客户端 Sentinel 错误
var (
    ErrBotTypeError
    ErrParamUserID
    ErrParamMessageID
    ErrParamExceedInputLimit
    ErrMessageTypeNotSuppored
    ErrEncryptionNotEnabled
    ErrCustomHTTPClientNotSet
    ErrMessageNotBuild
    ErrUnsupportedUIDType
    ErrInvalidReceiveID
    ErrEventTypeNotMatch
    ErrMessageType
)

// 使用方式：
if errors.Is(err, lark.ErrBotTypeError) { ... }
```

---

## 17. WheelMaker 中的数据流

```
飞书用户发消息
  → 飞书 WebSocket 网关
  → larkws.Client (larksuite/oapi-sdk-go/v3)
  → dispatcher.OnP2MessageReceiveV1 callback
  → im/feishu.channel.onMessage handler
  → im.Message{ChatID, MessageID, UserID, Text}
  → client.Client.HandleMessage()
```

回复时：
```
client.Client → im/feishu.channel.SendText(chatID, text)
  → lark.NewMsgBuffer(MsgText).BindChatID(chatID).Text(text).Build()
  → bot.PostMessage(msg)
  → 飞书 POST /open-apis/im/v1/messages
  → 飞书用户收到消息
```

---

## 18. 注意事项

- **消息去重**：飞书事件可能重复投递，需通过 `EventV2Header.EventID` 或官方 SDK 的 `MessageId` 去重
- **At Bot**：群聊中 Bot 只处理 @ 了自己的消息（或 P2P 消息）；可检查 `Mentions` 字段
- **消息长度**：单条文本消息上限约 30000 字符，超出需分段发送
- **频率限制**：发消息 API 有频率限制，注意控制流速
- **Token 刷新**：`tenant_access_token` 有效期 2 小时，`bot.StartHeartbeat()` 自动续期
- **WebSocket 重连**：官方 SDK 默认自动重连，错误码 403/514 不重连（应立即报警）
- **连接数限制**：单应用 WebSocket 连接数有上限，超出会收到 514 ExceedConnLimit


