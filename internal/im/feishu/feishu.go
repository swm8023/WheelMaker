package feishu

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/go-lark/lark"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	larkevent "github.com/larksuite/oapi-sdk-go/v3/event"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"

	"github.com/swm8023/wheelmaker/internal/im"
)

// Config configures the Feishu IM adapter.
type Config struct {
	AppID             string
	AppSecret         string
	VerificationToken string
	EncryptKey        string
	Debug             bool
}

// IM implements im.Channel using Feishu WS (inbound) + go-lark (outbound).
type IM struct {
	cfg Config

	mu      sync.RWMutex
	handler im.MessageHandler
	action  func(im.CardActionEvent)
	bot     *lark.Bot
}

// New creates a Feishu IM adapter.
func New(cfg Config) *IM {
	return &IM{cfg: cfg}
}

// OnMessage registers the inbound message handler.
func (f *IM) OnMessage(handler im.MessageHandler) {
	f.mu.Lock()
	f.handler = handler
	f.mu.Unlock()
}

// OnCardAction registers card interaction callback.
func (f *IM) OnCardAction(handler func(im.CardActionEvent)) {
	f.mu.Lock()
	f.action = handler
	f.mu.Unlock()
}

// SendText posts a plain text message to a Feishu chat.
func (f *IM) SendText(chatID, text string) error {
	bot, err := f.ensureBot()
	if err != nil {
		return err
	}
	msg := lark.NewMsgBuffer(lark.MsgText).
		BindChatID(chatID).
		Text(text).
		Build()
	_, err = bot.PostMessage(msg)
	return err
}

// SendCard posts an interactive card to a Feishu chat.
func (f *IM) SendCard(chatID string, card im.Card) error {
	bot, err := f.ensureBot()
	if err != nil {
		return err
	}
	raw, err := json.Marshal(card)
	if err != nil {
		return fmt.Errorf("feishu: marshal card: %w", err)
	}
	msg := lark.NewMsgBuffer(lark.MsgInteractive).
		BindChatID(chatID).
		Card(string(raw)).
		Build()
	_, err = bot.PostMessage(msg)
	return err
}

// SendReaction adds an emoji reaction.
func (f *IM) SendReaction(messageID, emoji string) error {
	bot, err := f.ensureBot()
	if err != nil {
		return err
	}
	_, err = bot.AddReaction(messageID, lark.EmojiType(emoji))
	return err
}

// Run starts Feishu WS event loop and blocks until ctx is done.
func (f *IM) Run(ctx context.Context) error {
	bot, err := f.ensureBot()
	if err != nil {
		return err
	}
	bot.StartHeartbeat()
	defer bot.StopHeartbeat()

	eventHandler := dispatcher.
		NewEventDispatcher(f.cfg.VerificationToken, f.cfg.EncryptKey).
		OnP2MessageReceiveV1(f.handleP2MessageReceive).
		OnCustomizedEvent("card.action.trigger", f.handleCardAction)

	logLevel := larkcore.LogLevelInfo
	if f.cfg.Debug {
		logLevel = larkcore.LogLevelDebug
	}

	wsClient := larkws.NewClient(
		f.cfg.AppID,
		f.cfg.AppSecret,
		larkws.WithEventHandler(eventHandler),
		larkws.WithLogLevel(logLevel),
	)

	if err := wsClient.Start(ctx); err != nil && ctx.Err() == nil {
		return fmt.Errorf("feishu ws start: %w", err)
	}
	return nil
}

func (f *IM) ensureBot() (*lark.Bot, error) {
	f.mu.RLock()
	if f.bot != nil {
		b := f.bot
		f.mu.RUnlock()
		return b, nil
	}
	f.mu.RUnlock()

	if strings.TrimSpace(f.cfg.AppID) == "" || strings.TrimSpace(f.cfg.AppSecret) == "" {
		return nil, fmt.Errorf("feishu: app id/secret required")
	}

	bot := lark.NewChatBot(f.cfg.AppID, f.cfg.AppSecret)
	f.mu.Lock()
	f.bot = bot
	f.mu.Unlock()
	return bot, nil
}

func (f *IM) handleP2MessageReceive(_ context.Context, event *larkim.P2MessageReceiveV1) error {
	if event == nil || event.Event == nil || event.Event.Message == nil {
		return nil
	}
	msg := event.Event.Message
	if msg.ChatId == nil || msg.MessageId == nil {
		return nil
	}
	text := parseMessageText(msg.MessageType, msg.Content)
	if strings.TrimSpace(text) == "" {
		return nil
	}

	userID := ""
	if event.Event.Sender != nil &&
		event.Event.Sender.SenderId != nil &&
		event.Event.Sender.SenderId.OpenId != nil {
		userID = *event.Event.Sender.SenderId.OpenId
	}

	f.mu.RLock()
	h := f.handler
	f.mu.RUnlock()
	if h != nil {
		h(im.Message{
			ChatID:    *msg.ChatId,
			MessageID: *msg.MessageId,
			UserID:    userID,
			Text:      text,
		})
	}
	return nil
}

func (f *IM) handleCardAction(_ context.Context, event *larkevent.EventReq) error {
	if event == nil || len(event.Body) == 0 {
		return nil
	}
	var payload struct {
		Event struct {
			Operator struct {
				OperatorID struct {
					OpenID string `json:"open_id"`
				} `json:"operator_id"`
			} `json:"operator"`
			Context struct {
				OpenMessageID string `json:"open_message_id"`
				OpenChatID    string `json:"open_chat_id"`
			} `json:"context"`
			Action struct {
				Tag    string                 `json:"tag"`
				Option string                 `json:"option"`
				Value  map[string]interface{} `json:"value"`
			} `json:"action"`
		} `json:"event"`
	}
	if err := json.Unmarshal(event.Body, &payload); err != nil {
		return nil
	}
	value := map[string]string{}
	for k, v := range payload.Event.Action.Value {
		value[k] = fmt.Sprint(v)
	}

	f.mu.RLock()
	h := f.action
	f.mu.RUnlock()
	if h != nil {
		h(im.CardActionEvent{
			ChatID:    firstNonEmpty(value["chat_id"], payload.Event.Context.OpenChatID),
			MessageID: payload.Event.Context.OpenMessageID,
			UserID:    payload.Event.Operator.OperatorID.OpenID,
			Tag:       payload.Event.Action.Tag,
			Option:    payload.Event.Action.Option,
			Value:     value,
		})
	}
	return nil
}

func parseMessageText(msgType *string, content *string) string {
	if msgType == nil || content == nil {
		return ""
	}
	// Only text is supported in MVP.
	if *msgType != "text" {
		return ""
	}
	var payload struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(*content), &payload); err != nil {
		return ""
	}
	return strings.TrimSpace(payload.Text)
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

var _ im.Channel = (*IM)(nil)
var _ im.CardActionSubscriber = (*IM)(nil)
