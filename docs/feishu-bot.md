# é£žä¹¦ Bot åè®®æ‘˜è¦

> æ•´ç†æ—¥æœŸï¼š2026-03-14
> å®˜æ–¹æ–‡æ¡£ï¼šhttps://open.feishu.cn/document/client-docs/bot-v3/bot-overview
> Go SDKï¼šhttps://github.com/go-lark/lark

## 1. Bot ç±»åž‹

| ç±»åž‹ | è¯´æ˜Ž | é€‚ç”¨åœºæ™¯ |
|------|------|----------|
| **è‡ªå®šä¹‰åº”ç”¨ Bot** | åœ¨é£žä¹¦å¼€æ”¾å¹³å°åˆ›å»ºåº”ç”¨åŽå…³è”çš„æœºå™¨äºº | WheelMaker ä½¿ç”¨æ­¤ç±»åž‹ |
| Webhook é€šçŸ¥ Bot | åªèƒ½å‘æ¶ˆæ¯ï¼Œæ— æ³•æŽ¥æ”¶ | å•å‘é€šçŸ¥ |

## 2. æŽ¥æ”¶æ¶ˆæ¯çš„ä¸¤ç§æ¨¡å¼

### 2.1 Webhook äº‹ä»¶è®¢é˜…ï¼ˆä¼ ç»Ÿï¼‰

- é£žä¹¦æœåŠ¡å™¨ä¸»åŠ¨ POST äº‹ä»¶åˆ°å¼€å‘è€…é…ç½®çš„ URL
- **éœ€è¦å…¬ç½‘å¯è®¿é—®åœ°å€**ï¼ˆæˆ– ngrok/å†…ç½‘ç©¿é€ï¼‰
- éœ€è¦å¼€å‘è€…æœåŠ¡å™¨è¿›è¡Œç­¾åæ ¡éªŒ

### 2.2 WebSocket é•¿è¿žæŽ¥ï¼ˆæŽ¨èï¼ŒWheelMaker é‡‡ç”¨ï¼‰

- **å¼€å‘è€…ä¸»åŠ¨è¿žæŽ¥**é£žä¹¦ WebSocket ç½‘å…³
- **æ— éœ€å…¬ç½‘ IP**ï¼Œé€‚åˆæœ¬åœ°å¼€å‘æœº
- é£žä¹¦é€šè¿‡æ­¤ channel æŽ¨é€äº‹ä»¶
- ä½¿ç”¨ go-lark SDK å¼€ç®±å³ç”¨

å®˜æ–¹æ–‡æ¡£ï¼šhttps://open.feishu.cn/document/uAjLw4CM/ukTMukTMukTM/event-subscription-guide/long-connection-mode

## 3. åº”ç”¨åˆ›å»ºæ­¥éª¤

1. è¿›å…¥ [é£žä¹¦å¼€æ”¾å¹³å°](https://open.feishu.cn/)ï¼Œåˆ›å»ºä¼ä¸šè‡ªå»ºåº”ç”¨
2. åœ¨ã€Œæœºå™¨äººã€åŠŸèƒ½ä¸­å¯ç”¨ Bot
3. èŽ·å– `App ID` å’Œ `App Secret`
4. é…ç½®äº‹ä»¶è®¢é˜…ï¼šé€‰æ‹©ã€Œä½¿ç”¨é•¿è¿žæŽ¥æŽ¥æ”¶äº‹ä»¶ã€
5. è®¢é˜…äº‹ä»¶ï¼š`im.message.receive_v1`ï¼ˆæŽ¥æ”¶æ¶ˆæ¯ï¼‰
6. é…ç½®æƒé™ï¼š
   - `im:message` â€” èŽ·å–æ¶ˆæ¯å†…å®¹
   - `im:message:send_as_bot` â€” å‘é€æ¶ˆæ¯

## 4. æ¶ˆæ¯æ ¼å¼

### 4.1 æŽ¥æ”¶æ¶ˆæ¯äº‹ä»¶ï¼ˆim.message.receive_v1ï¼‰

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

### 4.2 å‘é€æ¶ˆæ¯

**å‘é€æ–‡æœ¬**ï¼š
```
POST https://open.feishu.cn/open-apis/im/v1/messages?receive_id_type=chat_id

{
  "receive_id": "oc_xxx",
  "msg_type": "text",
  "content": "{\"text\":\"Hello!\"}"
}
```

**å‘é€å¯Œæ–‡æœ¬å¡ç‰‡**ï¼š
```
{
  "msg_type": "interactive",
  "content": "{...é£žä¹¦äº¤äº’å¡ç‰‡ JSON...}"
}
```

## 5. Go SDK ä½¿ç”¨ï¼ˆgo-larkï¼‰

### å®‰è£…

```bash
go get github.com/go-lark/lark/v2
```

### åˆå§‹åŒ– Bot

```go
import "github.com/go-lark/lark/v2"

bot := lark.NewChatBot("<App ID>", "<App Secret>")
// å›½é™…ç‰ˆï¼ˆLarkï¼‰ä½¿ç”¨ï¼š
// bot.SetDomain(lark.DomainLark)
```

### WebSocket é•¿è¿žæŽ¥æŽ¥æ”¶æ¶ˆæ¯

```go
// ä½¿ç”¨ WebSocket é•¿è¿žæŽ¥æ¨¡å¼
// go-lark v2 æ”¯æŒé€šè¿‡ middleware æ³¨å†Œäº‹ä»¶å¤„ç†
// å…·ä½“ WebSocket æ¨¡å¼å‚è€ƒï¼šhttps://github.com/go-lark/lark/tree/main/examples
```

### å‘é€æ¶ˆæ¯

```go
// å‘é€æ–‡æœ¬
bot.PostText("Hello World", lark.WithChatID("oc_xxx"))

// å‘é€å›¾ç‰‡/å¡ç‰‡
bot.PostNotification(
    lark.NewMsgBuffer(lark.MsgText).Text("Hello").Build(),
)
```

### æ·»åŠ æ¶ˆæ¯ Reaction

```go
// POST /open-apis/im/v1/messages/{message_id}/reactions
// emoji_type: "THUMBSUP", "DONE" ç­‰
```

## 6. æ¶ˆæ¯ç±»åž‹

| message_type | è¯´æ˜Ž |
|-------------|------|
| `text` | çº¯æ–‡æœ¬ |
| `post` | å¯Œæ–‡æœ¬ |
| `image` | å›¾ç‰‡ |
| `interactive` | äº¤äº’å¡ç‰‡ |
| `file` | æ–‡ä»¶ |
| `audio` | è¯­éŸ³ |

## 7. WheelMaker ä¸­çš„ä½¿ç”¨æ–¹å¼

```
é£žä¹¦ç”¨æˆ·å‘æ¶ˆæ¯ â†’ WebSocket é•¿è¿žæŽ¥ â†’ go-lark äº‹ä»¶å›žè°ƒ
â†’ im/feishu.provider.OnMessage handler
â†’ im.Message{ChatID, MessageID, UserID, Text}
â†’ Hub.HandleMessage()
```

å›žå¤æ—¶ï¼š
```
Hub â†’ im/feishu.provider.SendText(chatID, text)
â†’ é£žä¹¦ API POST /im/v1/messages
â†’ é£žä¹¦ç”¨æˆ·æ”¶åˆ°æ¶ˆæ¯
```

## 8. æ³¨æ„äº‹é¡¹

- **æ¶ˆæ¯åŽ»é‡**ï¼šé£žä¹¦äº‹ä»¶å¯èƒ½é‡å¤æŠ•é€’ï¼Œéœ€é€šè¿‡ `event_id` åŽ»é‡
- **At Bot**ï¼šç¾¤èŠä¸­ Bot åªå¤„ç† @ äº†è‡ªå·±çš„æ¶ˆæ¯ï¼ˆæˆ– P2P æ¶ˆæ¯ï¼‰
- **æ¶ˆæ¯é•¿åº¦**ï¼šå•æ¡æ–‡æœ¬æ¶ˆæ¯ä¸Šé™çº¦ 30000 å­—ç¬¦ï¼Œè¶…å‡ºéœ€åˆ†æ®µå‘é€
- **é¢‘çŽ‡é™åˆ¶**ï¼šå‘æ¶ˆæ¯ API æœ‰é¢‘çŽ‡é™åˆ¶ï¼Œæ³¨æ„æŽ§åˆ¶æµé€Ÿ
- **Token åˆ·æ–°**ï¼š`tenant_access_token` æœ‰æ•ˆæœŸ 2 å°æ—¶ï¼ŒSDK ä¼šè‡ªåŠ¨åˆ·æ–°

