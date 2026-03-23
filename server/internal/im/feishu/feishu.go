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

// Channel implements im.Channel using Feishu WS (inbound) + go-lark (outbound).
type Channel struct {
	cfg Config

	mu      sync.RWMutex
	handler im.MessageHandler
	action  func(im.CardActionEvent)
	bot     *lark.Bot

	debugMu      sync.Mutex
	debugStreams map[string]*debugStream
	toolMu       sync.Mutex
	toolCards    map[string]map[string]string // chatID -> toolCallID -> messageID

	seenMu        sync.Mutex
	seenMessageID map[string]time.Time
}

type debugStream struct {
	messageID string
	lines     []string
	flushing  bool
}

// New creates a Feishu IM adapter.
func New(cfg Config) *Channel {
	return &Channel{
		cfg:           cfg,
		debugStreams:  map[string]*debugStream{},
		seenMessageID: map[string]time.Time{},
		toolCards:     map[string]map[string]string{},
	}
}

// OnMessage registers the inbound message handler.
func (f *Channel) OnMessage(handler im.MessageHandler) {
	f.mu.Lock()
	f.handler = handler
	f.mu.Unlock()
}

// OnCardAction registers card interaction callback.
func (f *Channel) OnCardAction(handler func(im.CardActionEvent)) {
	f.mu.Lock()
	f.action = handler
	f.mu.Unlock()
}

// Abilities reports optional Feishu channel features.
func (f *Channel) Abilities() im.Ability {
	return im.AbilitySendDebug | im.AbilitySendOptions | im.AbilityCardActions | im.AbilitySendToolCards
}

// SendText posts a plain text message to a Feishu chat.
func (f *Channel) SendText(chatID, text string) error {
	bot, err := f.ensureBot()
	if err != nil {
		return err
	}
	parts := splitTextForFeishu(text, 3000)
	if len(parts) == 0 {
		return nil
	}
	for _, part := range parts {
		msg := lark.NewMsgBuffer(lark.MsgText).
			BindChatID(chatID).
			Text(part).
			Build()
		if _, err = bot.PostMessage(msg); err != nil {
			return err
		}
	}
	return nil
}

// SendCard posts an interactive card to a Feishu chat.
func (f *Channel) SendCard(chatID string, card im.Card) error {
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
func (f *Channel) SendOptions(chatID, title, body string, options []im.DecisionOption, meta map[string]string) error {
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
func (f *Channel) SendReaction(messageID, emoji string) error {
	bot, err := f.ensureBot()
	if err != nil {
		return err
	}
	_, err = bot.AddReaction(messageID, lark.EmojiType(emoji))
	return err
}

// SendDebug appends debug text to a per-chat stream card and flushes every 2 seconds.
func (f *Channel) SendDebug(chatID, text string) error {
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

func (f *Channel) resetDebugStream(chatID string) {
	chatID = strings.TrimSpace(chatID)
	if chatID == "" {
		return
	}
	f.debugMu.Lock()
	delete(f.debugStreams, chatID)
	f.debugMu.Unlock()
}

func (f *Channel) flushDebug(chatID string) {
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

// SendToolCall renders one streaming card per toolCallId and updates it in place.
func (f *Channel) SendToolCall(chatID string, update im.ToolCallUpdate) error {
	chatID = strings.TrimSpace(chatID)
	toolCallID := strings.TrimSpace(update.ToolCallID)
	if chatID == "" || toolCallID == "" {
		return nil
	}

	card := buildToolCallCard(update)
	raw, err := json.Marshal(card)
	if err != nil {
		return fmt.Errorf("feishu: marshal tool card: %w", err)
	}
	bot, err := f.ensureBot()
	if err != nil {
		return err
	}
	buf := lark.NewMsgBuffer(lark.MsgInteractive).Card(string(raw))

	f.toolMu.Lock()
	chatCards := f.toolCards[chatID]
	if chatCards == nil {
		chatCards = map[string]string{}
		f.toolCards[chatID] = chatCards
	}
	messageID := strings.TrimSpace(chatCards[toolCallID])
	f.toolMu.Unlock()

	if messageID == "" {
		msg := buf.BindChatID(chatID).Build()
		resp, err := bot.PostMessage(msg)
		if err != nil {
			return err
		}
		if resp != nil {
			mid := strings.TrimSpace(resp.Data.MessageID)
			if mid != "" {
				f.toolMu.Lock()
				if cc := f.toolCards[chatID]; cc != nil {
					cc[toolCallID] = mid
				}
				f.toolMu.Unlock()
			}
		}
		return nil
	}
	_, err = bot.UpdateMessage(messageID, buf.Build())
	return err
}

func buildToolCallCard(update im.ToolCallUpdate) im.Card {
	status := strings.ToLower(strings.TrimSpace(update.Status))
	if status == "" {
		status = "pending"
	}
	title := strings.TrimSpace(update.Title)
	if title == "" {
		title = "Tool Call"
	}
	icon := "🔧"
	statusText := status
	switch status {
	case "pending":
		icon = "🟡"
		statusText = "pending"
	case "in_progress":
		icon = "⏳"
		statusText = "running"
	case "completed":
		icon = "✅"
		statusText = "completed"
	case "failed":
		icon = "❌"
		statusText = "failed"
	case "cancelled":
		icon = "⛔"
		statusText = "cancelled"
	}

	elements := []map[string]any{
		{
			"tag": "markdown",
			"content": fmt.Sprintf("**Status:** %s %s\n**Tool ID:** `%s`",
				icon, statusText, strings.TrimSpace(update.ToolCallID)),
		},
	}
	if strings.TrimSpace(update.Kind) != "" {
		elements = append(elements, map[string]any{
			"tag":     "markdown",
			"content": fmt.Sprintf("**Kind:** `%s`", strings.TrimSpace(update.Kind)),
		})
	}
	if status == "pending" {
		elements = append(elements, map[string]any{
			"tag":     "markdown",
			"content": "⚠️ Waiting for confirmation or permission.",
		})
	}
	if out := toolCallOutputSummary(update); out != "" {
		elements = append(elements, map[string]any{
			"tag":     "markdown",
			"content": "```text\n" + out + "\n```",
		})
	}

	return im.Card{
		"config": map[string]any{"update_multi": true},
		"header": map[string]any{
			"title": map[string]any{
				"tag":     "plain_text",
				"content": fmt.Sprintf("%s %s", icon, title),
			},
		},
		"elements": elements,
	}
}

func toolCallOutputSummary(update im.ToolCallUpdate) string {
	parts := []string{}
	if text := decodeRawText(update.RawOutput); text != "" {
		parts = append(parts, text)
	}
	if text := decodeRawText(update.RawInput); text != "" {
		parts = append(parts, text)
	}
	for _, c := range update.ToolCallContent {
		if text := decodeRawText(c.Content); text != "" {
			parts = append(parts, text)
		}
		if strings.TrimSpace(c.NewText) != "" {
			parts = append(parts, strings.TrimSpace(c.NewText))
		}
	}
	out := strings.TrimSpace(strings.Join(parts, "\n"))
	if len(out) > 1600 {
		out = out[:1600] + "..."
	}
	return out
}

func decodeRawText(raw json.RawMessage) string {
	raw = json.RawMessage(strings.TrimSpace(string(raw)))
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return strings.TrimSpace(s)
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return strings.TrimSpace(string(raw))
	}
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// Run starts Feishu WS event loop and blocks until ctx is done.
func (f *Channel) Run(ctx context.Context) error {
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

func (f *Channel) ensureBot() (*lark.Bot, error) {
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

func (f *Channel) handleP2MessageReceive(_ context.Context, event *larkim.P2MessageReceiveV1) error {
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

func (f *Channel) shouldHandleMessage(messageID string) bool {
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

func (f *Channel) handleCardAction(_ context.Context, event *callback.CardActionTriggerEvent) (*callback.CardActionTriggerResponse, error) {
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

func splitTextForFeishu(text string, maxRunes int) []string {
	if text == "" {
		return nil
	}
	if maxRunes <= 0 {
		maxRunes = 3000
	}
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return []string{text}
	}
	parts := make([]string, 0, (len(runes)/maxRunes)+1)
	for start := 0; start < len(runes); start += maxRunes {
		end := start + maxRunes
		if end > len(runes) {
			end = len(runes)
		}
		parts = append(parts, string(runes[start:end]))
	}
	return parts
}

var _ im.Channel = (*Channel)(nil)
var _ im.DebugSender = (*Channel)(nil)
var _ im.CardActionSubscriber = (*Channel)(nil)
var _ im.OptionSender = (*Channel)(nil)
var _ im.ToolCallSender = (*Channel)(nil)
