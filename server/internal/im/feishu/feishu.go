package feishu

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/go-lark/lark"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher/callback"
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

	debugMu      sync.Mutex
	debugStreams map[string]*debugStream

	seenMu        sync.Mutex
	seenMessageID map[string]time.Time
}

type debugStream struct {
	messageID string
	lines     []string
	flushing  bool
}

// New creates a Feishu IM adapter.
func New(cfg Config) *IM {
	return &IM{
		cfg:          cfg,
		debugStreams: map[string]*debugStream{},
		seenMessageID: map[string]time.Time{},
	}
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

// SendOptions renders decision options. Feishu presents them as interactive buttons.
func (f *IM) SendOptions(chatID, title, body string, options []im.DecisionOption, meta map[string]string) error {
	elements := make([]map[string]any, 0, 2)
	if strings.TrimSpace(body) != "" {
		elements = append(elements, map[string]any{
			"tag":     "markdown",
			"content": strings.TrimSpace(body),
		})
	}
	if len(options) > 0 {
		actions := make([]map[string]any, 0, len(options))
		for _, opt := range options {
			value := map[string]any{
				"kind":      "decision",
				"chat_id":   chatID,
				"option_id": opt.ID,
				"value":     opt.Value,
			}
			for k, v := range meta {
				if strings.TrimSpace(k) == "" {
					continue
				}
				value[k] = v
			}
			actions = append(actions, map[string]any{
				"tag":   "button",
				"text":  map[string]any{"tag": "plain_text", "content": opt.Label},
				"type":  "default",
				"value": value,
			})
		}
		elements = append(elements, map[string]any{
			"tag":     "action",
			"actions": actions,
		})
	}
	card := im.Card{
		"config": map[string]any{"update_multi": true},
		"header": map[string]any{
			"title": map[string]any{
				"tag":     "plain_text",
				"content": firstNonEmpty(title, "Decision required"),
			},
		},
		"elements": elements,
	}
	return f.SendCard(chatID, card)
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

// SendDebug appends debug text to a per-chat stream card and flushes every 2 seconds.
func (f *IM) SendDebug(chatID, text string) error {
	chatID = strings.TrimSpace(chatID)
	line := strings.TrimSpace(text)
	if chatID == "" || line == "" {
		return nil
	}
	f.debugMu.Lock()
	ds := f.debugStreams[chatID]
	if ds == nil {
		ds = &debugStream{}
		f.debugStreams[chatID] = ds
	}
	ds.lines = append(ds.lines, line)
	if len(ds.lines) > 200 {
		ds.lines = ds.lines[len(ds.lines)-200:]
	}
	if !ds.flushing {
		ds.flushing = true
		time.AfterFunc(2*time.Second, func() { f.flushDebug(chatID) })
	}
	f.debugMu.Unlock()
	return nil
}

func (f *IM) resetDebugStream(chatID string) {
	chatID = strings.TrimSpace(chatID)
	if chatID == "" {
		return
	}
	f.debugMu.Lock()
	delete(f.debugStreams, chatID)
	f.debugMu.Unlock()
}

func (f *IM) flushDebug(chatID string) {
	f.debugMu.Lock()
	ds := f.debugStreams[chatID]
	if ds == nil {
		f.debugMu.Unlock()
		return
	}
	lines := append([]string(nil), ds.lines...)
	messageID := ds.messageID
	ds.flushing = false
	f.debugMu.Unlock()

	if len(lines) == 0 {
		return
	}
	card := buildDebugCard(lines)
	raw, err := json.Marshal(card)
	if err != nil {
		return
	}

	bot, err := f.ensureBot()
	if err != nil {
		return
	}
	buf := lark.NewMsgBuffer(lark.MsgInteractive).Card(string(raw))

	if strings.TrimSpace(messageID) == "" {
		msg := buf.BindChatID(chatID).Build()
		resp, postErr := bot.PostMessage(msg)
		if postErr != nil {
			return
		}
		if resp != nil {
			f.debugMu.Lock()
			if cur := f.debugStreams[chatID]; cur != nil {
				cur.messageID = strings.TrimSpace(resp.Data.MessageID)
			}
			f.debugMu.Unlock()
		}
		return
	}
	msg := buf.Build()
	_, _ = bot.UpdateMessage(messageID, msg)
}

func buildDebugCard(lines []string) im.Card {
	if len(lines) > 120 {
		lines = lines[len(lines)-120:]
	}
	var b strings.Builder
	b.WriteString("```text\n")
	for _, line := range lines {
		b.WriteString(line)
		b.WriteString("\n")
	}
	b.WriteString("```")
	return im.Card{
		"config": map[string]any{"update_multi": true},
		"header": map[string]any{
			"title": map[string]any{
				"tag":     "plain_text",
				"content": "Debug Stream",
			},
		},
		"elements": []map[string]any{
			{"tag": "markdown", "content": b.String()},
		},
	}
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
		OnP2CardActionTrigger(f.handleCardAction)

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
	if !f.shouldHandleMessage(*msg.MessageId) {
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
		// Start a new debug stream card for each new user message in the chat.
		f.resetDebugStream(*msg.ChatId)
		h(im.Message{
			ChatID:    *msg.ChatId,
			MessageID: *msg.MessageId,
			UserID:    userID,
			Text:      text,
		})
	}
	return nil
}

func (f *IM) shouldHandleMessage(messageID string) bool {
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return true
	}
	const dedupTTL = 30 * time.Minute
	const maxTracked = 4096
	now := time.Now()
	cutoff := now.Add(-dedupTTL)

	f.seenMu.Lock()
	defer f.seenMu.Unlock()

	for id, ts := range f.seenMessageID {
		if ts.Before(cutoff) {
			delete(f.seenMessageID, id)
		}
	}
	if _, exists := f.seenMessageID[messageID]; exists {
		return false
	}
	if len(f.seenMessageID) >= maxTracked {
		// Keep memory bounded by dropping stale entries first, then oldest-ish fallback.
		for id, ts := range f.seenMessageID {
			if ts.Before(now.Add(-5 * time.Minute)) {
				delete(f.seenMessageID, id)
			}
		}
		if len(f.seenMessageID) >= maxTracked {
			for id := range f.seenMessageID {
				delete(f.seenMessageID, id)
				break
			}
		}
	}
	f.seenMessageID[messageID] = now
	return true
}

func (f *IM) handleCardAction(_ context.Context, event *callback.CardActionTriggerEvent) (*callback.CardActionTriggerResponse, error) {
	if event == nil || event.Event == nil || event.Event.Action == nil {
		return &callback.CardActionTriggerResponse{}, nil
	}
	payload := event.Event
	value := map[string]string{}
	for k, v := range payload.Action.Value {
		value[k] = fmt.Sprint(v)
	}

	f.mu.RLock()
	h := f.action
	f.mu.RUnlock()
	if h != nil {
		evt := im.CardActionEvent{
			ChatID:    firstNonEmpty(value["chat_id"], payload.Context.OpenChatID),
			MessageID: payload.Context.OpenMessageID,
			UserID:    payload.Operator.OpenID,
			Tag:       payload.Action.Tag,
			Option:    payload.Action.Option,
			Value:     value,
		}
		// ACK callback quickly; do not block Feishu callback thread on command execution.
		go h(evt)
	}
	return &callback.CardActionTriggerResponse{}, nil
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
var _ im.DebugSender = (*IM)(nil)
var _ im.CardActionSubscriber = (*IM)(nil)
var _ im.OptionSender = (*IM)(nil)
