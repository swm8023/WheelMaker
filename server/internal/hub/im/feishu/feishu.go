package feishu

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-lark/lark"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher/callback"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"

	"github.com/swm8023/wheelmaker/internal/hub/im"
)

// Config configures the Feishu IM adapter.
type Config struct {
	AppID             string
	AppSecret         string
	VerificationToken string
	EncryptKey        string
	Debug             bool
	YOLO              bool
}

// Channel implements im.Channel using Feishu WS (inbound) + go-lark (outbound).
type Channel struct {
	cfg Config

	mu      sync.RWMutex
	handler im.MessageHandler
	action  func(im.CardActionEvent)
	bot     *lark.Bot

	debugMu        sync.Mutex
	debugStreams   map[string]*debugStream
	textMu         sync.Mutex
	textStreams    map[string]*textStream
	thoughtStreams map[string]*textStream
	systemStreams  map[string]*textStream
	toolMu         sync.Mutex
	toolRenderMu   sync.Mutex
	toolCards      map[string]map[string]*toolCardState // chatID -> toolCallID -> state
	toolCompact    map[string]*compactToolStream        // chatID -> compact stream state

	seenMu        sync.Mutex
	seenMessageID map[string]time.Time
	ackMu         sync.Mutex
	pendingAck    map[string]pendingAckReaction // chatID -> latest inbound reaction to clear on first reply
	lastMu        sync.Mutex
	lastOutbound  map[string]string // chatID -> last outbound message ID
}

type debugStream struct {
	messageID string
	lines     []string
	flushing  bool
}

type textStream struct {
	messageID string
	content   strings.Builder
}

type toolCardState struct {
	messageID string
	update    im.ToolCallUpdate
	perm      *toolPermissionState
}

type toolPermissionState struct {
	decisionID    string
	options       []im.DecisionOption
	active        bool
	selectedID    string
	selectedLabel string
}

type compactToolStream struct {
	messageID string
	order     []string
	entries   map[string]compactToolEntry
}

type compactToolEntry struct {
	ToolCallID string
	Title      string
	Command    string
	Output     string
	Status     string
}

type pendingAckReaction struct {
	messageID  string
	reactionID string
}

// New creates a Feishu IM adapter.
func New(cfg Config) *Channel {
	return &Channel{
		cfg:            cfg,
		debugStreams:   map[string]*debugStream{},
		textStreams:    map[string]*textStream{},
		thoughtStreams: map[string]*textStream{},
		systemStreams:  map[string]*textStream{},
		seenMessageID:  map[string]time.Time{},
		toolCards:      map[string]map[string]*toolCardState{},
		toolCompact:    map[string]*compactToolStream{},
		pendingAck:     map[string]pendingAckReaction{},
		lastOutbound:   map[string]string{},
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

// Send dispatches a text message by kind to the appropriate stream.
func (f *Channel) Send(chatID, text string, kind im.TextKind) error {
	switch kind {
	case im.TextThought:
		return f.sendThought(chatID, text)
	case im.TextDebug:
		return f.sendDebug(chatID, text)
	case im.TextSystem:
		return f.sendSystem(chatID, text)
	default:
		return f.sendText(chatID, text)
	}
}

func (f *Channel) sendText(chatID, text string) error {
	chatID = strings.TrimSpace(chatID)
	if chatID == "" || text == "" {
		return nil
	}
	// Non-tool text breaks the contiguous tool segment in YOLO mode.
	f.resetCompactToolStream(chatID)
	f.textMu.Lock()
	defer f.textMu.Unlock()

	ts := f.textStreams[chatID]
	if ts == nil {
		ts = &textStream{}
		f.textStreams[chatID] = ts
	}
	ts.content.WriteString(text)
	content := ts.content.String()
	if content == "" {
		return nil
	}

	card := buildTextStreamCard(content, false)
	raw, err := json.Marshal(card)
	if err != nil {
		return fmt.Errorf("feishu: marshal text stream card: %w", err)
	}
	bot, err := f.ensureBot()
	if err != nil {
		return err
	}
	buf := lark.NewMsgBuffer(lark.MsgInteractive).Card(string(raw))
	messageID := strings.TrimSpace(ts.messageID)
	if messageID == "" {
		resp, postErr := bot.PostMessage(buf.BindChatID(chatID).Build())
		if postErr != nil {
			return postErr
		}
		if resp != nil {
			mid := strings.TrimSpace(resp.Data.MessageID)
			if mid != "" {
				ts.messageID = mid
				f.setLastOutbound(chatID, mid)
			}
		}
		f.clearReceiveAck(chatID)
		return nil
	}
	if _, err := bot.UpdateMessage(messageID, buf.Build()); err == nil {
		f.setLastOutbound(chatID, messageID)
		f.clearReceiveAck(chatID)
		return nil
	}
	resp, postErr := bot.PostMessage(buf.BindChatID(chatID).Build())
	if postErr != nil {
		return postErr
	}
	if resp != nil {
		if mid := strings.TrimSpace(resp.Data.MessageID); mid != "" {
			ts.messageID = mid
			f.setLastOutbound(chatID, mid)
		}
	}
	f.clearReceiveAck(chatID)
	return nil
}

func (f *Channel) sendThought(chatID, text string) error {
	chatID = strings.TrimSpace(chatID)
	if chatID == "" || text == "" {
		return nil
	}
	f.resetCompactToolStream(chatID)
	f.textMu.Lock()
	defer f.textMu.Unlock()

	ts := f.thoughtStreams[chatID]
	if ts == nil {
		ts = &textStream{}
		f.thoughtStreams[chatID] = ts
	}
	ts.content.WriteString(text)
	content := ts.content.String()
	if content == "" {
		return nil
	}

	card := buildThoughtStreamCard(content, false)
	raw, err := json.Marshal(card)
	if err != nil {
		return fmt.Errorf("feishu: marshal thought stream card: %w", err)
	}
	bot, err := f.ensureBot()
	if err != nil {
		return err
	}
	buf := lark.NewMsgBuffer(lark.MsgInteractive).Card(string(raw))
	messageID := strings.TrimSpace(ts.messageID)
	if messageID == "" {
		resp, postErr := bot.PostMessage(buf.BindChatID(chatID).Build())
		if postErr != nil {
			return postErr
		}
		if resp != nil {
			if mid := strings.TrimSpace(resp.Data.MessageID); mid != "" {
				ts.messageID = mid
				f.setLastOutbound(chatID, mid)
			}
		}
		f.clearReceiveAck(chatID)
		return nil
	}
	if _, err := bot.UpdateMessage(messageID, buf.Build()); err == nil {
		f.setLastOutbound(chatID, messageID)
		f.clearReceiveAck(chatID)
		return nil
	}
	resp, postErr := bot.PostMessage(buf.BindChatID(chatID).Build())
	if postErr != nil {
		return postErr
	}
	if resp != nil {
		if mid := strings.TrimSpace(resp.Data.MessageID); mid != "" {
			ts.messageID = mid
			f.setLastOutbound(chatID, mid)
		}
	}
	f.clearReceiveAck(chatID)
	return nil
}

func (f *Channel) sendSystem(chatID, text string) error {
	chatID = strings.TrimSpace(chatID)
	text = strings.TrimSpace(text)
	if chatID == "" || text == "" {
		return nil
	}
	// System text also breaks contiguous tool segment and is rendered as
	// independent cards (no streaming merge).
	f.resetCompactToolStream(chatID)
	card := buildSystemStreamCard(text)
	raw, err := json.Marshal(card)
	if err != nil {
		return fmt.Errorf("feishu: marshal system stream card: %w", err)
	}
	bot, err := f.ensureBot()
	if err != nil {
		return err
	}
	buf := lark.NewMsgBuffer(lark.MsgInteractive).Card(string(raw))
	resp, postErr := bot.PostMessage(buf.BindChatID(chatID).Build())
	if postErr != nil {
		return postErr
	}
	if resp != nil {
		if mid := strings.TrimSpace(resp.Data.MessageID); mid != "" {
			f.setLastOutbound(chatID, mid)
		}
	}
	f.clearReceiveAck(chatID)
	f.resetSystemStream(chatID)
	return nil
}

func (f *Channel) resetTextStream(chatID string) {
	chatID = strings.TrimSpace(chatID)
	if chatID == "" {
		return
	}
	f.textMu.Lock()
	delete(f.textStreams, chatID)
	f.textMu.Unlock()
}

func (f *Channel) resetThoughtStream(chatID string) {
	chatID = strings.TrimSpace(chatID)
	if chatID == "" {
		return
	}
	f.textMu.Lock()
	delete(f.thoughtStreams, chatID)
	f.textMu.Unlock()
}

func (f *Channel) resetSystemStream(chatID string) {
	chatID = strings.TrimSpace(chatID)
	if chatID == "" {
		return
	}
	f.textMu.Lock()
	delete(f.systemStreams, chatID)
	f.textMu.Unlock()
}

// SendCard dispatches a card payload to the appropriate renderer.
func (f *Channel) SendCard(chatID, messageID string, card im.Card) error {
	switch c := card.(type) {
	case im.RawCard:
		return f.sendRawCard(chatID, messageID, c)
	case im.OptionsCard:
		return f.sendOptions(chatID, c)
	case im.ToolCallCard:
		return f.sendToolCall(chatID, c)
	default:
		return fmt.Errorf("feishu: unsupported card type %T", card)
	}
}

// sendRawCard posts a new card or updates an existing one in place.
func (f *Channel) sendRawCard(chatID, messageID string, card im.RawCard) error {
	bot, err := f.ensureBot()
	if err != nil {
		return err
	}
	raw, err := json.Marshal(card)
	if err != nil {
		return fmt.Errorf("feishu: marshal card: %w", err)
	}
	buf := lark.NewMsgBuffer(lark.MsgInteractive).Card(string(raw))
	if messageID = strings.TrimSpace(messageID); messageID != "" {
		_, err = bot.UpdateMessage(messageID, buf.Build())
		if err == nil {
			f.setLastOutbound(chatID, messageID)
			f.clearReceiveAck(chatID)
		}
		return err
	}
	resp, err := bot.PostMessage(buf.BindChatID(chatID).Build())
	if err == nil {
		if resp != nil {
			f.setLastOutbound(chatID, strings.TrimSpace(resp.Data.MessageID))
		}
		f.clearReceiveAck(chatID)
	}
	return err
}

// sendOptions renders decision options as interactive buttons.
func (f *Channel) sendOptions(chatID string, oc im.OptionsCard) error {
	title, body, options, meta := oc.Title, oc.Body, oc.Options, oc.Meta
	toolCallID := strings.TrimSpace(meta["tool_call_id"])
	decisionID := strings.TrimSpace(meta["decision_id"])
	if toolCallID != "" && decisionID != "" {
		err := f.attachPermissionOptionsToToolCard(chatID, toolCallID, title, options, meta)
		if err == nil {
			f.toolMu.Lock()
			if cards := f.toolCards[chatID]; cards != nil && cards[toolCallID] != nil {
			}
			f.toolMu.Unlock()
			f.clearReceiveAck(chatID)
		}
		return err
	}

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
				"kind":         "decision",
				"chat_id":      chatID,
				"option_id":    opt.ID,
				"option_label": opt.Label,
				"value":        opt.Value,
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
	rawCard := im.RawCard{
		"config": map[string]any{"update_multi": true},
		"header": map[string]any{
			"title": map[string]any{
				"tag":     "plain_text",
				"content": firstNonEmpty(title, "Decision required"),
			},
		},
		"elements": elements,
	}
	return f.sendRawCard(chatID, "", rawCard)
}

func (f *Channel) attachPermissionOptionsToToolCard(chatID, toolCallID, title string, options []im.DecisionOption, meta map[string]string) error {
	chatID = strings.TrimSpace(chatID)
	toolCallID = strings.TrimSpace(toolCallID)
	if chatID == "" || toolCallID == "" {
		return nil
	}
	decisionID := strings.TrimSpace(meta["decision_id"])
	toolTitle := strings.TrimSpace(meta["tool_title"])
	toolKind := strings.TrimSpace(meta["tool_kind"])
	if toolTitle == "" {
		toolTitle = strings.TrimSpace(title)
	}
	if toolTitle == "" {
		toolTitle = "Tool Call"
	}

	f.textMu.Lock()
	delete(f.textStreams, chatID)
	f.textMu.Unlock()

	f.toolMu.Lock()
	chatCards := f.toolCards[chatID]
	if chatCards == nil {
		chatCards = map[string]*toolCardState{}
		f.toolCards[chatID] = chatCards
	}
	st := chatCards[toolCallID]
	if st == nil {
		st = &toolCardState{
			update: im.ToolCallUpdate{
				ToolCallID: toolCallID,
				Title:      toolTitle,
				Kind:       toolKind,
				Status:     "pending",
			},
		}
		chatCards[toolCallID] = st
	}
	if strings.TrimSpace(st.update.Title) == "" {
		st.update.Title = toolTitle
	}
	if strings.TrimSpace(st.update.Kind) == "" {
		st.update.Kind = toolKind
	}
	st.perm = &toolPermissionState{
		decisionID: decisionID,
		options:    append([]im.DecisionOption(nil), options...),
		active:     true,
	}
	stCopy := *st
	if st.perm != nil {
		permCopy := *st.perm
		stCopy.perm = &permCopy
	}
	f.toolMu.Unlock()

	return f.upsertToolCard(chatID, toolCallID, &stCopy, true)
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

// MarkDone adds DONE reaction to the last outbound message in this chat.
func (f *Channel) MarkDone(chatID string) error {
	chatID = strings.TrimSpace(chatID)
	if chatID == "" {
		return nil
	}
	messageID := f.getLastOutbound(chatID)
	if messageID == "" {
		return nil
	}
	return f.SendReaction(messageID, "DONE")
}

func (f *Channel) addReceiveAck(chatID, messageID string) {
	chatID = strings.TrimSpace(chatID)
	messageID = strings.TrimSpace(messageID)
	if chatID == "" || messageID == "" {
		return
	}
	bot, err := f.ensureBot()
	if err != nil {
		return
	}
	resp, err := bot.AddReaction(messageID, lark.EmojiTypeGet)
	if err != nil || resp == nil {
		return
	}
	reactionID := strings.TrimSpace(resp.Data.ReactionID)
	if reactionID == "" {
		return
	}
	f.ackMu.Lock()
	f.pendingAck[chatID] = pendingAckReaction{messageID: messageID, reactionID: reactionID}
	f.ackMu.Unlock()
}

func (f *Channel) popPendingAck(chatID string) (pendingAckReaction, bool) {
	chatID = strings.TrimSpace(chatID)
	if chatID == "" {
		return pendingAckReaction{}, false
	}
	f.ackMu.Lock()
	defer f.ackMu.Unlock()
	ack, ok := f.pendingAck[chatID]
	if !ok {
		return pendingAckReaction{}, false
	}
	delete(f.pendingAck, chatID)
	return ack, true
}

func (f *Channel) clearReceiveAck(chatID string) (pendingAckReaction, bool) {
	ack, ok := f.popPendingAck(chatID)
	if !ok {
		return pendingAckReaction{}, false
	}
	// Keep GET reaction as a durable "processed" marker; only consume pending state.
	return ack, true
}

func (f *Channel) setLastOutbound(chatID, messageID string) {
	chatID = strings.TrimSpace(chatID)
	messageID = strings.TrimSpace(messageID)
	if chatID == "" || messageID == "" {
		return
	}
	f.lastMu.Lock()
	f.lastOutbound[chatID] = messageID
	f.lastMu.Unlock()
}

func (f *Channel) getLastOutbound(chatID string) string {
	chatID = strings.TrimSpace(chatID)
	if chatID == "" {
		return ""
	}
	f.lastMu.Lock()
	defer f.lastMu.Unlock()
	return strings.TrimSpace(f.lastOutbound[chatID])
}

func (f *Channel) sendDebug(chatID, text string) error {
	chatID = strings.TrimSpace(chatID)
	line := sanitizeDebugStreamLine(text)
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

func sanitizeDebugStreamLine(text string) string {
	line := strings.TrimSpace(strings.ReplaceAll(text, "\r\n", "\n"))
	if line == "" {
		return ""
	}
	if strings.HasPrefix(line, "[debug][") {
		if idx := strings.Index(line, "] "); idx >= 0 && idx+2 < len(line) {
			line = strings.TrimSpace(line[idx+2:])
		}
	}
	return line
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

func buildDebugCard(lines []string) im.RawCard {
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
	return im.RawCard{
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

func buildTextStreamCard(content string, streaming bool) im.RawCard {
	_ = streaming
	elements := []map[string]any{
		{"tag": "markdown", "content": normalizeStreamMarkdown(content)},
	}
	return im.RawCard{
		"config":   map[string]any{"update_multi": true},
		"elements": elements,
	}
}

func buildThoughtStreamCard(content string, streaming bool) im.RawCard {
	_ = streaming
	return im.RawCard{
		"config": map[string]any{"update_multi": true},
		"header": map[string]any{
			"title": map[string]any{
				"tag":     "plain_text",
				"content": "🧠 Thinking",
			},
		},
		"elements": []map[string]any{
			{"tag": "markdown", "content": normalizeStreamMarkdown(content)},
		},
	}
}

func buildSystemStreamCard(content string) im.RawCard {
	return im.RawCard{
		"config": map[string]any{"update_multi": true},
		"header": map[string]any{
			"title": map[string]any{
				"tag":     "plain_text",
				"content": "📣 System Message",
			},
		},
		"elements": []map[string]any{
			{"tag": "markdown", "content": normalizeStreamMarkdown(content)},
		},
	}
}

func normalizeStreamMarkdown(content string) string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	lines := strings.Split(content, "\n")
	out := make([]string, 0, len(lines)+4)
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if i > 0 && trimmed != "" && startsMarkdownSection(trimmed) {
			prev := ""
			if len(out) > 0 {
				prev = strings.TrimSpace(out[len(out)-1])
			}
			if prev != "" {
				out = append(out, "")
			}
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

func startsMarkdownSection(line string) bool {
	if strings.HasPrefix(line, "#") || strings.HasPrefix(line, ">") {
		return true
	}
	if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") {
		return true
	}
	if len(line) >= 3 && line[0] >= '0' && line[0] <= '9' {
		for i := 1; i < len(line); i++ {
			if line[i] == '.' || line[i] == ')' {
				return i+1 < len(line) && line[i+1] == ' '
			}
			if line[i] < '0' || line[i] > '9' {
				return false
			}
		}
	}
	return false
}

// SendToolCall renders one streaming card per toolCallId and updates it in place.
func (f *Channel) sendToolCall(chatID string, tc im.ToolCallCard) error {
	update := tc.Update
	chatID = strings.TrimSpace(chatID)
	toolCallID := strings.TrimSpace(update.ToolCallID)
	if chatID == "" || toolCallID == "" {
		return nil
	}
	if f.cfg.YOLO {
		err := f.sendToolCallCompact(chatID, update)
		if err == nil {
			f.clearReceiveAck(chatID)
		}
		return err
	}

	// Tool updates interrupt the current assistant text stream card.
	f.textMu.Lock()
	delete(f.textStreams, chatID)
	delete(f.thoughtStreams, chatID)
	f.textMu.Unlock()

	f.toolMu.Lock()
	chatCards := f.toolCards[chatID]
	if chatCards == nil {
		chatCards = map[string]*toolCardState{}
		f.toolCards[chatID] = chatCards
	}
	st := chatCards[toolCallID]
	if st == nil {
		st = &toolCardState{}
		chatCards[toolCallID] = st
	}
	if strings.TrimSpace(update.Status) == "" && strings.TrimSpace(st.update.Status) != "" {
		update.Status = st.update.Status
	}
	if strings.TrimSpace(update.Status) == "" {
		update.Status = "pending"
	}
	if strings.TrimSpace(update.Title) == "" {
		update.Title = st.update.Title
	}
	if strings.TrimSpace(update.Kind) == "" {
		update.Kind = st.update.Kind
	}
	st.update = update
	if isToolCallTerminalStatus(update.Status) && st.perm != nil {
		st.perm.active = false
	}
	stCopy := *st
	if st.perm != nil {
		permCopy := *st.perm
		stCopy.perm = &permCopy
	}
	f.toolMu.Unlock()

	err := f.upsertToolCard(chatID, toolCallID, &stCopy, false)
	if err == nil {
		f.clearReceiveAck(chatID)
	}
	return err
}

func (f *Channel) upsertToolCard(chatID, toolCallID string, st *toolCardState, _ bool) error {
	if st == nil {
		return nil
	}
	f.toolRenderMu.Lock()
	defer f.toolRenderMu.Unlock()

	f.toolMu.Lock()
	chatCards := f.toolCards[chatID]
	if chatCards == nil {
		chatCards = map[string]*toolCardState{}
		f.toolCards[chatID] = chatCards
	}
	cur := chatCards[toolCallID]
	if cur == nil {
		cur = &toolCardState{}
		chatCards[toolCallID] = cur
	}
	cur.update = st.update
	cur.perm = st.perm
	messageID := strings.TrimSpace(cur.messageID)
	update := cur.update
	perm := cur.perm
	f.toolMu.Unlock()

	card := buildToolCallCard(chatID, update, perm, false)
	raw, err := json.Marshal(card)
	if err != nil {
		return fmt.Errorf("feishu: marshal tool card: %w", err)
	}
	bot, err := f.ensureBot()
	if err != nil {
		return err
	}
	buf := lark.NewMsgBuffer(lark.MsgInteractive).Card(string(raw))
	if messageID == "" {
		resp, postErr := bot.PostMessage(buf.BindChatID(chatID).Build())
		if postErr != nil {
			return postErr
		}
		if resp != nil {
			messageID = strings.TrimSpace(resp.Data.MessageID)
		}
	} else if _, err := bot.UpdateMessage(messageID, buf.Build()); err != nil {
		// Do not fallback to posting a new card on update failure: that creates
		// duplicate cards for a single tool call. Surface the error for logging/retry.
		return err
	}
	if messageID != "" {
		f.setLastOutbound(chatID, messageID)
		f.toolMu.Lock()
		if cards := f.toolCards[chatID]; cards != nil {
			if curState := cards[toolCallID]; curState != nil {
				curState.messageID = messageID
			}
		}
		f.toolMu.Unlock()
	}
	return nil
}

func (f *Channel) resetCompactToolStream(chatID string) {
	if !f.cfg.YOLO {
		return
	}
	chatID = strings.TrimSpace(chatID)
	if chatID == "" {
		return
	}
	f.toolMu.Lock()
	delete(f.toolCompact, chatID)
	f.toolMu.Unlock()
}

func (f *Channel) sendToolCallCompact(chatID string, update im.ToolCallUpdate) error {
	toolCallID := strings.TrimSpace(update.ToolCallID)
	if toolCallID == "" {
		return nil
	}
	if strings.TrimSpace(update.Status) == "" {
		update.Status = "pending"
	}
	if strings.TrimSpace(update.Title) == "" {
		update.Title = "tool_call"
	}

	// Tool updates interrupt assistant text card.
	f.textMu.Lock()
	delete(f.textStreams, chatID)
	f.textMu.Unlock()

	f.toolMu.Lock()
	stream := f.toolCompact[chatID]
	if stream == nil {
		stream = &compactToolStream{
			entries: map[string]compactToolEntry{},
		}
		f.toolCompact[chatID] = stream
	}
	if _, ok := stream.entries[toolCallID]; !ok {
		stream.order = append(stream.order, toolCallID)
	}
	prev := stream.entries[toolCallID]
	command := strings.TrimSpace(toolCallCommandSummary(update))
	if command == "" {
		command = strings.TrimSpace(prev.Command)
	}
	output := strings.TrimSpace(toolCallOutputText(update))
	if output == "" {
		output = strings.TrimSpace(prev.Output)
	}
	stream.entries[toolCallID] = compactToolEntry{
		ToolCallID: toolCallID,
		Title:      strings.TrimSpace(update.Title),
		Command:    command,
		Output:     output,
		Status:     strings.TrimSpace(update.Status),
	}
	const maxLines = 30
	if len(stream.order) > maxLines {
		drop := stream.order[:len(stream.order)-maxLines]
		for _, id := range drop {
			delete(stream.entries, id)
		}
		stream.order = stream.order[len(stream.order)-maxLines:]
	}
	messageID := strings.TrimSpace(stream.messageID)
	lines := compactToolLines(stream)
	transcript := compactToolTranscript(stream)
	f.toolMu.Unlock()

	card := buildCompactToolCard(lines, transcript, false)
	raw, err := json.Marshal(card)
	if err != nil {
		return fmt.Errorf("feishu: marshal compact tool card: %w", err)
	}
	bot, err := f.ensureBot()
	if err != nil {
		return err
	}
	buf := lark.NewMsgBuffer(lark.MsgInteractive).Card(string(raw))
	if messageID == "" {
		resp, postErr := bot.PostMessage(buf.BindChatID(chatID).Build())
		if postErr != nil {
			return postErr
		}
		if resp != nil {
			messageID = strings.TrimSpace(resp.Data.MessageID)
		}
	} else if _, err := bot.UpdateMessage(messageID, buf.Build()); err != nil {
		return err
	}

	if messageID != "" {
		f.setLastOutbound(chatID, messageID)
		f.toolMu.Lock()
		if st := f.toolCompact[chatID]; st != nil {
			st.messageID = messageID
		}
		f.toolMu.Unlock()
	}
	return nil
}

func compactToolLines(stream *compactToolStream) []string {
	if stream == nil || len(stream.order) == 0 {
		return nil
	}
	lines := make([]string, 0, len(stream.order))
	for _, id := range stream.order {
		e, ok := stream.entries[id]
		if !ok {
			continue
		}
		cmd := strings.TrimSpace(e.Command)
		if cmd == "" {
			cmd = strings.TrimSpace(e.Title)
		}
		if cmd == "" {
			cmd = e.ToolCallID
		}
		lines = append(lines, fmt.Sprintf("%s %s", compactStatusEmoji(e.Status), previewLine(cmd, 90)))
	}
	return lines
}

func compactStatusEmoji(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "completed":
		return "✅"
	case "failed":
		return "❌"
	case "cancelled":
		return "⛔"
	case "in_progress":
		return "⏳"
	default:
		return "🟡"
	}
}

func buildCompactToolCard(lines []string, transcript string, streaming bool) im.RawCard {
	_ = streaming
	title := compactToolIconTitle(lines)
	if strings.TrimSpace(transcript) == "" {
		transcript = "<no output>"
	}
	content := "```text\n" + transcript + "\n```"
	elements := []map[string]any{
		{"tag": "markdown", "content": content},
	}
	return im.RawCard{
		"config": map[string]any{"update_multi": true},
		"header": map[string]any{
			"template": "blue",
			"title": map[string]any{
				"tag":     "plain_text",
				"content": title,
			},
		},
		"elements": elements,
	}
}

func compactToolIconTitle(lines []string) string {
	const maxIcons = 8
	icons := make([]string, 0, maxIcons)
	allowed := map[string]struct{}{
		compactStatusEmoji("completed"):   {},
		compactStatusEmoji("failed"):      {},
		compactStatusEmoji("cancelled"):   {},
		compactStatusEmoji("in_progress"): {},
		compactStatusEmoji("pending"):     {},
	}
	for _, line := range lines {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) == 0 {
			continue
		}
		icon := fields[0]
		if _, ok := allowed[icon]; !ok {
			continue
		}
		icons = append(icons, icon)
		if len(icons) >= maxIcons {
			break
		}
	}
	if len(icons) == 0 {
		return "Tools"
	}
	return "Tools " + strings.Join(icons, " ")
}

func compactToolTranscript(stream *compactToolStream) string {
	if stream == nil || len(stream.order) == 0 {
		return ""
	}
	blocks := make([]string, 0, len(stream.order))
	for _, id := range stream.order {
		e, ok := stream.entries[id]
		if !ok {
			continue
		}
		cmd := strings.TrimSpace(e.Command)
		if cmd == "" {
			cmd = strings.TrimSpace(e.Title)
		}
		if cmd == "" {
			cmd = e.ToolCallID
		}
		status := strings.TrimSpace(e.Status)
		if status == "" {
			status = "pending"
		}
		out := strings.TrimSpace(e.Output)
		if out == "" {
			out = "<no output>"
		}
		block := fmt.Sprintf("$ %s\n[%s]\n%s", cmd, status, previewBlock(out, 600))
		blocks = append(blocks, block)
	}
	return previewBlock(strings.Join(blocks, "\n\n"), 3200)
}

func isToolCallTerminalStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "completed", "failed", "cancelled":
		return true
	default:
		return false
	}
}

func buildToolCallCard(chatID string, update im.ToolCallUpdate, perm *toolPermissionState, streaming bool) im.RawCard {
	_ = streaming
	status := strings.ToLower(strings.TrimSpace(update.Status))
	if status == "" {
		status = "pending"
	}
	title := strings.TrimSpace(update.Title)
	if title == "" {
		title = "tool_call"
	}
	_, template := toolStatusStyle(status)
	permEmoji := toolPermissionEmoji(perm)

	content := toolCallDetailBlock(update)
	elements := []map[string]any{
		{"tag": "markdown", "content": "```text\n" + content + "\n```"},
	}
	if perm != nil {
		if perm.active && len(perm.options) > 0 {
			actions := make([]map[string]any, 0, len(perm.options))
			for _, opt := range perm.options {
				actions = append(actions, map[string]any{
					"tag":  "button",
					"text": map[string]any{"tag": "plain_text", "content": opt.Label},
					"type": "default",
					"value": map[string]any{
						"kind":         "decision",
						"chat_id":      chatID,
						"decision_id":  perm.decisionID,
						"option_id":    opt.ID,
						"value":        opt.Value,
						"option_label": opt.Label,
						"tool_call_id": update.ToolCallID,
					},
				})
			}
			elements = append(elements, map[string]any{"tag": "action", "actions": actions})
		}
	}
	return im.RawCard{
		"config": map[string]any{"update_multi": true},
		"header": map[string]any{
			"template": template,
			"title": map[string]any{
				"tag":     "plain_text",
				"content": fmt.Sprintf("🛠️ %s %s", permEmoji, previewLine(title, 22)),
			},
		},
		"elements": elements,
	}
}

func toolCallCommandSummary(update im.ToolCallUpdate) string {
	out := toolCallCommandLine(update)
	if out == "" {
		if strings.TrimSpace(update.Kind) != "" {
			out = update.Kind
		} else {
			out = update.ToolCallID
		}
	}
	return previewLine(out, 160)
}

func toolStatusStyle(status string) (emoji string, template string) {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "completed":
		return "\U00002705", "green"
	case "failed":
		return "\U0000274C", "red"
	case "cancelled":
		return "\U000026D4", "grey"
	case "in_progress":
		return "\U000023F3", "blue"
	default:
		return "\U0001F7E1", "orange"
	}
}
func toolPermissionEmoji(perm *toolPermissionState) string {
	if perm == nil {
		return "\U000026AA"
	}
	if perm.active {
		return "\U0001F7E1"
	}
	v := strings.ToLower(strings.TrimSpace(perm.selectedID + " " + perm.selectedLabel))
	if strings.Contains(v, "allow") || strings.Contains(v, "approve") || strings.Contains(v, "yes") {
		return "\U0001F7E2"
	}
	if strings.Contains(v, "reject") || strings.Contains(v, "abort") || strings.Contains(v, "no") || strings.Contains(v, "cancel") {
		return "\U0001F534"
	}
	return "\U000026AA"
}
func toolCallDetailBlock(update im.ToolCallUpdate) string {
	cmd := strings.TrimSpace(toolCallCommandLine(update))
	if cmd == "" {
		cmd = "<unknown command>"
	}
	out := strings.TrimSpace(toolCallOutputText(update))
	if out == "" {
		out = "<no output>"
	}
	block := fmt.Sprintf("$ %s\n\n%s", cmd, out)
	return previewBlock(block, 3200)
}

func toolCallCommandLine(update im.ToolCallUpdate) string {
	if cmd := commandFromRawInput(update.RawInput); cmd != "" {
		return cmd
	}
	if cmd := commandFromRawOutput(update.RawOutput); cmd != "" {
		return cmd
	}
	if text := decodeRawText(update.RawInput); text != "" {
		return text
	}
	for _, c := range update.ToolCallContent {
		if text := decodeRawText(c.Content); text != "" {
			return text
		}
	}
	return ""
}
func commandFromRawInput(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var payload struct {
		Command []string `json:"command"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return ""
	}
	if len(payload.Command) == 0 {
		return ""
	}
	return strings.TrimSpace(strings.Join(payload.Command, " "))
}

func commandFromRawOutput(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var payload struct {
		Command []string `json:"command"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return ""
	}
	if len(payload.Command) == 0 {
		return ""
	}
	return strings.TrimSpace(strings.Join(payload.Command, " "))
}
func toolCallOutputText(update im.ToolCallUpdate) string {
	if text := outputFromRawOutput(update.RawOutput); text != "" {
		return text
	}
	for _, c := range update.ToolCallContent {
		if text := decodeRawText(c.Content); text != "" {
			return text
		}
		if strings.TrimSpace(c.NewText) != "" {
			return strings.TrimSpace(c.NewText)
		}
	}
	return ""
}

func outputFromRawOutput(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var payload struct {
		Output string `json:"output"`
		Stdout string `json:"stdout"`
		Stderr string `json:"stderr"`
		Error  string `json:"error"`
	}
	if err := json.Unmarshal(raw, &payload); err == nil {
		lines := []string{}
		if strings.TrimSpace(payload.Output) != "" {
			lines = append(lines, strings.TrimSpace(payload.Output))
		}
		if strings.TrimSpace(payload.Stdout) != "" {
			lines = append(lines, strings.TrimSpace(payload.Stdout))
		}
		if strings.TrimSpace(payload.Stderr) != "" {
			lines = append(lines, "stderr: "+strings.TrimSpace(payload.Stderr))
		}
		if strings.TrimSpace(payload.Error) != "" {
			lines = append(lines, "error: "+strings.TrimSpace(payload.Error))
		}
		if len(lines) > 0 {
			return strings.Join(lines, "\n")
		}
	}
	return decodeRawText(raw)
}

func previewLine(s string, maxRunes int) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\r", ""))
	if s == "" {
		return ""
	}
	if idx := strings.Index(s, "\n"); idx >= 0 {
		s = strings.TrimSpace(s[:idx])
	}
	r := []rune(s)
	if len(r) <= maxRunes {
		return s
	}
	return string(r[:maxRunes]) + "..."
}

func previewBlock(s string, maxRunes int) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\r", ""))
	if s == "" {
		return ""
	}
	r := []rune(s)
	if len(r) <= maxRunes {
		return s
	}
	return string(r[:maxRunes]) + "..."
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
	if isFeishuMessageStale(msg.CreateTime) {
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
		f.resetTextStream(*msg.ChatId)
		f.resetThoughtStream(*msg.ChatId)
		f.resetSystemStream(*msg.ChatId)
		f.addReceiveAck(*msg.ChatId, *msg.MessageId)
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
	const dedupTTL = 2 * time.Hour
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

func isFeishuMessageStale(createTime *string) bool {
	if createTime == nil {
		return false
	}
	raw := strings.TrimSpace(*createTime)
	if raw == "" {
		return false
	}
	ms, err := parseEpochMillis(raw)
	if err != nil {
		return false
	}
	msgAt := time.UnixMilli(ms)
	if msgAt.IsZero() {
		return false
	}
	// Drop very old retries so a historic user message cannot trigger a fresh prompt.
	// Keep this window conservative to avoid discarding near-real-time delivery jitter.
	const maxMessageAge = 15 * time.Minute
	return time.Since(msgAt) > maxMessageAge
}

func parseEpochMillis(raw string) (int64, error) {
	// Feishu create_time is usually epoch millis as string.
	return strconv.ParseInt(raw, 10, 64)
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
	if strings.TrimSpace(value["kind"]) == "decision" {
		chatID := firstNonEmpty(value["chat_id"], payload.Context.OpenChatID)
		toolCallID := strings.TrimSpace(value["tool_call_id"])
		decisionID := strings.TrimSpace(value["decision_id"])
		selectedID := strings.TrimSpace(value["option_id"])
		selectedLabel := strings.TrimSpace(value["option_label"])
		if chatID != "" && toolCallID != "" && decisionID != "" {
			f.toolMu.Lock()
			if chatCards := f.toolCards[chatID]; chatCards != nil {
				if st := chatCards[toolCallID]; st != nil && st.perm != nil && st.perm.decisionID == decisionID {
					st.perm.active = false
					st.perm.selectedID = selectedID
					if selectedLabel == "" {
						selectedLabel = selectedID
					}
					st.perm.selectedLabel = selectedLabel
					stCopy := *st
					permCopy := *st.perm
					stCopy.perm = &permCopy
					f.toolMu.Unlock()
					_ = f.upsertToolCard(chatID, toolCallID, &stCopy, false)
					goto forward
				}
			}
			f.toolMu.Unlock()
		}
	}

forward:
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
