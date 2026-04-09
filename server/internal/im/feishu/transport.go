package feishu

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/go-lark/lark"
	larkoapi "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher/callback"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"

	acp "github.com/swm8023/wheelmaker/internal/protocol"
	logger "github.com/swm8023/wheelmaker/internal/shared"
)

// streamThrottle is the minimum interval between card pushes for the same
// streaming card. Chunks arriving within this window are accumulated and
// pushed together to reduce Feishu API call frequency.
const streamThrottle = 3 * time.Second

// Config configures the Feishu IM adapter.
type Config struct {
	AppID             string
	AppSecret         string
	VerificationToken string
	EncryptKey        string
	YOLO              bool
	BlockedUpdates    []string
}

// Channel implements Channel using Feishu WS (inbound) + go-lark (outbound).
type transportChannel struct {
	cfg Config

	mu      sync.RWMutex
	handler MessageHandler
	action  func(CardActionEvent)
	bot     *lark.Bot
	api     *larkoapi.Client

	imageFetcher func(context.Context, string, string) (acp.ContentBlock, error)

	textMu         sync.Mutex
	unifiedStreams map[string]*unifiedStream
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
	wsReadyOnce   sync.Once
}

type segmentKind uint8

const (
	segText segmentKind = iota
	segThought
	segDivider
)

// unifiedStreamMaxRunes is the content threshold after which the next
// incoming segment starts a new card.
const unifiedStreamMaxRunes = 10000

type streamSegment struct {
	kind    segmentKind
	content string
}

type unifiedStream struct {
	messageID   string
	segments    []streamSegment
	totalRunes  int // running rune count across all segments
	pushedRunes int // totalRunes at last push (dirty check)
	lastPush    time.Time
	timer       *time.Timer
}

type toolCardState struct {
	messageID string
	update    ToolCallUpdate
	perm      *toolPermissionState
}

type toolPermissionState struct {
	requestID     string
	options       []PermissionOption
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
func newTransport(cfg Config) *transportChannel {
	t := &transportChannel{
		cfg:            cfg,
		unifiedStreams: map[string]*unifiedStream{},
		seenMessageID:  map[string]time.Time{},
		toolCards:      map[string]map[string]*toolCardState{},
		toolCompact:    map[string]*compactToolStream{},
		pendingAck:     map[string]pendingAckReaction{},
		lastOutbound:   map[string]string{},
	}
	t.imageFetcher = t.fetchImageBlock
	return t
}

// OnMessage registers the inbound message handler.
func (f *transportChannel) OnMessage(handler MessageHandler) {
	f.mu.Lock()
	f.handler = handler
	f.mu.Unlock()
}

// OnCardAction registers card interaction callback.
func (f *transportChannel) OnCardAction(handler func(CardActionEvent)) {
	f.mu.Lock()
	f.action = handler
	f.mu.Unlock()
}

// Send dispatches a text message by kind to the appropriate stream.
func (f *transportChannel) Send(chatID, text string, kind TextKind) error {
	switch kind {
	case TextThought:
		return f.sendThought(chatID, text)
	case TextSystem:
		return f.sendSystem(chatID, text)
	case TextDivider:
		f.insertDivider(chatID)
		return nil
	default:
		return f.sendText(chatID, text)
	}
}

func (f *transportChannel) sendText(chatID, text string) error {
	chatID = strings.TrimSpace(chatID)
	if chatID == "" || text == "" {
		return nil
	}
	// Non-tool text breaks the contiguous tool segment in YOLO mode.
	f.resetCompactToolStream(chatID)
	f.textMu.Lock()
	defer f.textMu.Unlock()

	us := f.unifiedStreams[chatID]
	if us == nil {
		us = &unifiedStream{}
		f.unifiedStreams[chatID] = us
	}

	// Auto-split: if content exceeds threshold, finalize and start new card
	if us.totalRunes > unifiedStreamMaxRunes && len(us.segments) > 0 {
		if us.timer != nil {
			us.timer.Stop()
			us.timer = nil
		}
		_ = f.pushUnifiedCardLocked(chatID, us, true)
		us = &unifiedStream{}
		f.unifiedStreams[chatID] = us
	}

	// Append to last text segment, or create new one
	n := len(us.segments)
	if n > 0 && us.segments[n-1].kind == segText {
		us.segments[n-1].content += text
	} else {
		seg := streamSegment{kind: segText, content: text}
		us.segments = append(us.segments, seg)
	}
	us.totalRunes += utf8.RuneCountInString(text)

	f.armUnifiedTimer(chatID, us)
	return nil
}

func (f *transportChannel) sendThought(chatID, text string) error {
	chatID = strings.TrimSpace(chatID)
	if chatID == "" || text == "" {
		return nil
	}
	f.resetCompactToolStream(chatID)
	f.textMu.Lock()
	defer f.textMu.Unlock()

	us := f.unifiedStreams[chatID]
	if us == nil {
		us = &unifiedStream{}
		f.unifiedStreams[chatID] = us
	}

	if us.totalRunes > unifiedStreamMaxRunes && len(us.segments) > 0 {
		if us.timer != nil {
			us.timer.Stop()
			us.timer = nil
		}
		_ = f.pushUnifiedCardLocked(chatID, us, true)
		us = &unifiedStream{}
		f.unifiedStreams[chatID] = us
	}

	n := len(us.segments)
	if n > 0 && us.segments[n-1].kind == segThought {
		us.segments[n-1].content += text
	} else {
		seg := streamSegment{kind: segThought, content: text}
		us.segments = append(us.segments, seg)
	}
	us.totalRunes += utf8.RuneCountInString(text)

	f.armUnifiedTimer(chatID, us)
	return nil
}

func (f *transportChannel) insertDivider(chatID string) {
	chatID = strings.TrimSpace(chatID)
	if chatID == "" {
		return
	}
	f.textMu.Lock()
	defer f.textMu.Unlock()
	us := f.unifiedStreams[chatID]
	if us == nil || len(us.segments) == 0 {
		return
	}
	// Don't add consecutive dividers
	if us.segments[len(us.segments)-1].kind == segDivider {
		return
	}
	us.segments = append(us.segments, streamSegment{kind: segDivider})
}

func (f *transportChannel) armUnifiedTimer(chatID string, us *unifiedStream) {
	if us.timer != nil {
		return
	}
	us.timer = time.AfterFunc(streamThrottle, func() {
		f.textMu.Lock()
		defer f.textMu.Unlock()
		us.timer = nil
		if us.totalRunes > us.pushedRunes {
			if err := f.pushUnifiedCardLocked(chatID, us, false); err != nil {
				logger.Warn("feishu: deferred unified flush: %v", err)
			}
		}
	})
}

// pushUnifiedCardLocked builds and posts/updates the unified card.
// Must be called with f.textMu held.
func (f *transportChannel) pushUnifiedCardLocked(chatID string, us *unifiedStream, done bool) error {
	if len(us.segments) == 0 {
		return nil
	}
	card := buildUnifiedStreamCard(us.segments, done)
	if card == nil {
		return nil
	}
	raw, err := json.Marshal(card)
	if err != nil {
		return fmt.Errorf("feishu: marshal unified card: %w", err)
	}
	bot, err := f.ensureBot()
	if err != nil {
		return err
	}
	buf := lark.NewMsgBuffer(lark.MsgInteractive).Card(string(raw))
	messageID := strings.TrimSpace(us.messageID)
	if messageID == "" {
		resp, postErr := bot.PostMessage(buf.BindChatID(chatID).Build())
		if postErr != nil {
			return postErr
		}
		if resp != nil {
			if mid := strings.TrimSpace(resp.Data.MessageID); mid != "" {
				us.messageID = mid
				f.setLastOutbound(chatID, mid)
			}
		}
		f.clearReceiveAck(chatID)
	} else {
		if _, err := bot.UpdateMessage(messageID, buf.Build()); err == nil {
			f.setLastOutbound(chatID, messageID)
			f.clearReceiveAck(chatID)
		} else {
			resp, postErr := bot.PostMessage(buf.BindChatID(chatID).Build())
			if postErr != nil {
				return postErr
			}
			if resp != nil {
				if mid := strings.TrimSpace(resp.Data.MessageID); mid != "" {
					us.messageID = mid
					f.setLastOutbound(chatID, mid)
				}
			}
			f.clearReceiveAck(chatID)
		}
	}
	us.pushedRunes = us.totalRunes
	us.lastPush = time.Now()
	return nil
}

// flushPendingStreams pushes any buffered unified stream content that has not
// yet been pushed due to throttling. Call before MarkDone or stream reset.
func (f *transportChannel) flushPendingStreams(chatID string) {
	f.textMu.Lock()
	defer f.textMu.Unlock()
	us := f.unifiedStreams[chatID]
	if us == nil {
		return
	}
	if us.timer != nil {
		us.timer.Stop()
		us.timer = nil
	}
	_ = f.pushUnifiedCardLocked(chatID, us, true)
}

func (f *transportChannel) sendSystem(chatID, text string) error {
	chatID = strings.TrimSpace(chatID)
	text = strings.TrimSpace(text)
	if chatID == "" || text == "" {
		return nil
	}
	// System text also breaks contiguous tool segment and is rendered as
	// independent cards (no streaming merge).
	f.resetCompactToolStream(chatID)
	// Finalize unified stream so system message is visually separate.
	f.finalizeUnifiedStream(chatID)
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
	return nil
}

func (f *transportChannel) finalizeUnifiedStream(chatID string) {
	f.textMu.Lock()
	defer f.textMu.Unlock()
	us := f.unifiedStreams[chatID]
	if us == nil {
		return
	}
	if us.timer != nil {
		us.timer.Stop()
		us.timer = nil
	}
	_ = f.pushUnifiedCardLocked(chatID, us, true)
	delete(f.unifiedStreams, chatID)
}

func (f *transportChannel) resetUnifiedStream(chatID string) {
	chatID = strings.TrimSpace(chatID)
	if chatID == "" {
		return
	}
	f.textMu.Lock()
	if us := f.unifiedStreams[chatID]; us != nil && us.timer != nil {
		us.timer.Stop()
		us.timer = nil
	}
	delete(f.unifiedStreams, chatID)
	f.textMu.Unlock()
}

// SendCard dispatches a card payload to the appropriate renderer.
func (f *transportChannel) SendCard(chatID, messageID string, card Card) (string, error) {
	switch c := card.(type) {
	case RawCard:
		return f.sendRawCard(chatID, messageID, c)
	case OptionsCard:
		return f.sendOptions(chatID, c)
	case ToolCallCard:
		return f.sendToolCall(chatID, c)
	default:
		return "", fmt.Errorf("feishu: unsupported card type %T", card)
	}
}

// sendRawCard posts a new card or updates an existing one in place.
func (f *transportChannel) sendRawCard(chatID, messageID string, card RawCard) (string, error) {
	bot, err := f.ensureBot()
	if err != nil {
		return "", err
	}
	raw, err := json.Marshal(card)
	if err != nil {
		return "", fmt.Errorf("feishu: marshal card: %w", err)
	}
	buf := lark.NewMsgBuffer(lark.MsgInteractive).Card(string(raw))
	if messageID = strings.TrimSpace(messageID); messageID != "" {
		_, err = bot.UpdateMessage(messageID, buf.Build())
		if err == nil {
			f.setLastOutbound(chatID, messageID)
			f.clearReceiveAck(chatID)
		}
		return messageID, err
	}
	resp, err := bot.PostMessage(buf.BindChatID(chatID).Build())
	if err == nil {
		if resp != nil {
			messageID = strings.TrimSpace(resp.Data.MessageID)
			f.setLastOutbound(chatID, messageID)
		}
		f.clearReceiveAck(chatID)
	}
	return messageID, err
}

// sendOptions renders decision options as interactive buttons.
func (f *transportChannel) sendOptions(chatID string, oc OptionsCard) (string, error) {
	title, body, options, meta := oc.Title, oc.Body, oc.Options, oc.Meta
	toolCallID := strings.TrimSpace(meta["tool_call_id"])
	requestID := strings.TrimSpace(meta["request_id"])
	if toolCallID != "" && requestID != "" {
		err := f.attachPermissionOptionsToToolCard(chatID, toolCallID, title, options, meta)
		if err == nil {
			f.toolMu.Lock()
			if cards := f.toolCards[chatID]; cards != nil && cards[toolCallID] != nil {
			}
			f.toolMu.Unlock()
			f.clearReceiveAck(chatID)
		}
		return "", err
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
			label := strings.TrimSpace(opt.Name)
			if label == "" {
				label = strings.TrimSpace(opt.OptionID)
			}
			value := map[string]any{
				"kind":        "permission",
				"chat_id":     chatID,
				"option_id":   opt.OptionID,
				"option_name": label,
				"option_kind": opt.Kind,
			}
			for k, v := range meta {
				if strings.TrimSpace(k) == "" {
					continue
				}
				value[k] = v
			}
			actions = append(actions, map[string]any{
				"tag":   "button",
				"text":  map[string]any{"tag": "plain_text", "content": label},
				"type":  "default",
				"value": value,
			})
		}
		elements = append(elements, map[string]any{
			"tag":     "action",
			"actions": actions,
		})
	}
	rawCard := RawCard{
		"config": map[string]any{"update_multi": true},
		"header": map[string]any{
			"title": map[string]any{
				"tag":     "plain_text",
				"content": firstNonEmpty(title, "Permission required"),
			},
		},
		"elements": elements,
	}
	return f.sendRawCard(chatID, "", rawCard)
}

func (f *transportChannel) attachPermissionOptionsToToolCard(chatID, toolCallID, title string, options []PermissionOption, meta map[string]string) error {
	chatID = strings.TrimSpace(chatID)
	toolCallID = strings.TrimSpace(toolCallID)
	if chatID == "" || toolCallID == "" {
		return nil
	}
	requestID := strings.TrimSpace(meta["request_id"])
	toolTitle := strings.TrimSpace(meta["tool_title"])
	toolKind := strings.TrimSpace(meta["tool_kind"])
	if toolTitle == "" {
		toolTitle = strings.TrimSpace(title)
	}
	if toolTitle == "" {
		toolTitle = "Tool Call"
	}

	f.textMu.Lock()
	if us := f.unifiedStreams[chatID]; us != nil && us.timer != nil {
		us.timer.Stop()
		us.timer = nil
	}
	delete(f.unifiedStreams, chatID)
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
			update: ToolCallUpdate{
				ToolCallID: toolCallID,
				Title:      toolTitle,
				Kind:       toolKind,
				Status:     acp.ToolCallStatusPending,
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
		requestID: requestID,
		options:   append([]PermissionOption(nil), options...),
		active:    true,
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
func (f *transportChannel) SendReaction(messageID, emoji string) error {
	bot, err := f.ensureBot()
	if err != nil {
		return err
	}
	_, err = bot.AddReaction(messageID, lark.EmojiType(emoji))
	return err
}

// MarkDone adds DONE reaction to the last outbound message in this chat.
func (f *transportChannel) MarkDone(chatID string) error {
	chatID = strings.TrimSpace(chatID)
	if chatID == "" {
		return nil
	}
	// Flush any throttled stream content before marking done.
	f.flushPendingStreams(chatID)
	messageID := f.getLastOutbound(chatID)
	if messageID == "" {
		return nil
	}
	return f.SendReaction(messageID, "DONE")
}

func (f *transportChannel) addReceiveAck(chatID, messageID string) {
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

func (f *transportChannel) popPendingAck(chatID string) (pendingAckReaction, bool) {
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

func (f *transportChannel) clearReceiveAck(chatID string) (pendingAckReaction, bool) {
	ack, ok := f.popPendingAck(chatID)
	if !ok {
		return pendingAckReaction{}, false
	}
	// Keep GET reaction as a durable "processed" marker; only consume pending state.
	return ack, true
}

func (f *transportChannel) setLastOutbound(chatID, messageID string) {
	chatID = strings.TrimSpace(chatID)
	messageID = strings.TrimSpace(messageID)
	if chatID == "" || messageID == "" {
		return
	}
	f.lastMu.Lock()
	f.lastOutbound[chatID] = messageID
	f.lastMu.Unlock()
}

func (f *transportChannel) getLastOutbound(chatID string) string {
	chatID = strings.TrimSpace(chatID)
	if chatID == "" {
		return ""
	}
	f.lastMu.Lock()
	defer f.lastMu.Unlock()
	return strings.TrimSpace(f.lastOutbound[chatID])
}

func buildUnifiedStreamCard(segments []streamSegment, done bool) RawCard {
	elements := make([]map[string]any, 0, len(segments)*2)
	for i, seg := range segments {
		switch seg.kind {
		case segText:
			content := seg.content
			if content != "" {
				elements = append(elements, map[string]any{
					"tag":     "markdown",
					"content": normalizeStreamMarkdown(content),
				})
			}
		case segThought:
			content := seg.content
			if content == "" {
				continue
			}
			isLastSeg := (i == len(segments)-1)
			thoughtDone := done || !isLastSeg
			if thoughtDone {
				elements = append(elements, buildThoughtCollapsible(content))
			} else {
				md := "**🧠 Thinking**\n\n" + normalizeStreamMarkdown(content)
				elements = append(elements, map[string]any{
					"tag":     "markdown",
					"content": md,
				})
			}
		case segDivider:
			// Only render a visible divider between segments of different
			// kinds (ABC pattern). When both sides are the same kind (ABA),
			// the separate segments already produce a paragraph break.
			prevKind := findAdjacentKind(segments, i, -1)
			nextKind := findAdjacentKind(segments, i, 1)
			if prevKind != nextKind && prevKind != segDivider && nextKind != segDivider {
				elements = append(elements, map[string]any{
					"tag":     "markdown",
					"content": "---",
				})
			}
		}
	}
	// Strip leading/trailing dividers
	for len(elements) > 0 {
		if isMarkdownDividerElement(elements[0]) {
			elements = elements[1:]
		} else {
			break
		}
	}
	for len(elements) > 0 {
		if isMarkdownDividerElement(elements[len(elements)-1]) {
			elements = elements[:len(elements)-1]
		} else {
			break
		}
	}
	if len(elements) == 0 {
		return nil
	}
	return RawCard{
		"schema": "2.0",
		"config": map[string]any{"update_multi": true},
		"body": map[string]any{
			"elements": elements,
		},
	}
}

// findAdjacentKind scans from position `from` in direction `dir` (-1 or +1)
// and returns the kind of the first non-divider segment found.
func findAdjacentKind(segments []streamSegment, from int, dir int) segmentKind {
	for j := from + dir; j >= 0 && j < len(segments); j += dir {
		if segments[j].kind != segDivider {
			return segments[j].kind
		}
	}
	return segDivider
}

// buildThoughtCollapsible renders a completed thought as a Feishu
// collapsible_panel element, collapsed by default.
func buildThoughtCollapsible(content string) map[string]any {
	summary := thoughtSummary(content)
	return map[string]any{
		"tag":              "collapsible_panel",
		"expanded":         false,
		"vertical_spacing": "8px",
		"padding":          "4px 8px",
		"header": map[string]any{
			"title": map[string]any{
				"tag":     "plain_text",
				"content": "🧠 " + summary,
			},
			"vertical_align": "center",
		},
		"border": map[string]any{
			"color":         "grey",
			"corner_radius": "8px",
		},
		"elements": []map[string]any{
			{"tag": "markdown", "content": normalizeStreamMarkdown(content)},
		},
	}
}

// thoughtSummary returns a short preview of the first non-empty line.
func thoughtSummary(content string) string {
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			return previewLine(trimmed, 60)
		}
	}
	return "Thought"
}

func isMarkdownDividerElement(element map[string]any) bool {
	if element == nil {
		return false
	}
	tag, _ := element["tag"].(string)
	if tag != "markdown" {
		return false
	}
	content, _ := element["content"].(string)
	return strings.TrimSpace(content) == "---"
}

func buildSystemStreamCard(content string) RawCard {
	return RawCard{
		"schema": "2.0",
		"config": map[string]any{"update_multi": true},
		"header": map[string]any{
			"title": map[string]any{
				"tag":     "plain_text",
				"content": "📣 System Message",
			},
		},
		"body": map[string]any{
			"elements": []map[string]any{
				{"tag": "markdown", "content": normalizeStreamMarkdown(content)},
			},
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
func (f *transportChannel) sendToolCall(chatID string, tc ToolCallCard) (string, error) {
	update := tc.Update
	chatID = strings.TrimSpace(chatID)
	toolCallID := strings.TrimSpace(update.ToolCallID)
	if chatID == "" || toolCallID == "" {
		return "", nil
	}
	if f.cfg.YOLO {
		err := f.sendToolCallCompact(chatID, update)
		if err == nil {
			f.clearReceiveAck(chatID)
		}
		return "", err
	}

	// Tool updates interrupt the current unified stream.
	f.finalizeUnifiedStream(chatID)

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
	return "", err
}

func (f *transportChannel) upsertToolCard(chatID, toolCallID string, st *toolCardState, _ bool) error {
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

func (f *transportChannel) resetCompactToolStream(chatID string) {
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

func (f *transportChannel) sendToolCallCompact(chatID string, update ToolCallUpdate) error {
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
	if us := f.unifiedStreams[chatID]; us != nil {
		if us.timer != nil {
			us.timer.Stop()
			us.timer = nil
		}
		_ = f.pushUnifiedCardLocked(chatID, us, true)
	}
	delete(f.unifiedStreams, chatID)
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

func buildCompactToolCard(lines []string, transcript string, streaming bool) RawCard {
	_ = streaming
	title := compactToolIconTitle(lines)
	if strings.TrimSpace(transcript) == "" {
		transcript = "<no output>"
	}
	content := "``	ext\n" + transcript + "\n```"
	elements := []map[string]any{
		{"tag": "markdown", "content": content},
	}
	return RawCard{
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

func buildToolCallCard(chatID string, update ToolCallUpdate, perm *toolPermissionState, streaming bool) RawCard {
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
		{"tag": "markdown", "content": "``	ext\n" + content + "\n```"},
	}
	if perm != nil {
		if perm.active && len(perm.options) > 0 {
			actions := make([]map[string]any, 0, len(perm.options))
			for _, opt := range perm.options {
				label := strings.TrimSpace(opt.Name)
				if label == "" {
					label = strings.TrimSpace(opt.OptionID)
				}
				actions = append(actions, map[string]any{
					"tag":  "button",
					"text": map[string]any{"tag": "plain_text", "content": label},
					"type": "default",
					"value": map[string]any{
						"kind":         "permission",
						"chat_id":      chatID,
						"request_id":   perm.requestID,
						"option_id":    opt.OptionID,
						"option_name":  label,
						"option_kind":  opt.Kind,
						"tool_call_id": update.ToolCallID,
					},
				})
			}
			elements = append(elements, map[string]any{"tag": "action", "actions": actions})
		}
	}
	return RawCard{
		"config": map[string]any{"update_multi": true},
		"header": map[string]any{
			"template": template,
			"title": map[string]any{
				"tag":     "plain_text",
				"content": fmt.Sprintf("%s %s %s", toolKindIcon(update.Kind), permEmoji, previewLine(title, 22)),
			},
		},
		"elements": elements,
	}
}

func toolCallCommandSummary(update ToolCallUpdate) string {
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

func toolKindIcon(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "bash", "shell", "terminal", "command":
		return "💻"
	case "read", "read_file", "view":
		return "📖"
	case "write", "write_file", "create", "create_file":
		return "✏️"
	case "edit", "edit_file", "replace", "str_replace_editor":
		return "📝"
	case "grep", "search", "ripgrep":
		return "🔍"
	case "glob", "find", "list_dir", "ls":
		return "📁"
	case "web", "browser", "fetch", "curl":
		return "🌐"
	default:
		return "🛠️"
	}
}

func toolCallDetailBlock(update ToolCallUpdate) string {
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

func toolCallCommandLine(update ToolCallUpdate) string {
	if cmd := commandFromRawInput(update.RawInput); cmd != "" {
		return cmd
	}
	if cmd := commandFromRawOutput(update.RawOutput); cmd != "" {
		return cmd
	}
	// Kind-aware: build a summary from structured input fields.
	if cmd := commandFromInputFields(update.Kind, update.RawInput); cmd != "" {
		return cmd
	}
	// ToolCallContent path fields.
	for _, c := range update.ToolCallContent {
		if p := strings.TrimSpace(c.Path); p != "" {
			kind := strings.TrimSpace(update.Kind)
			if kind != "" {
				return kind + " " + p
			}
			return p
		}
	}
	// Raw text (only if rawInput is a simple string, not an object).
	if text := decodeRawText(update.RawInput); text != "" && !isJSONObject(update.RawInput) {
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

// commandFromInputFields extracts a human-readable command line from
// structured rawInput fields based on the tool kind.
func commandFromInputFields(kind string, raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var fields map[string]any
	if json.Unmarshal(raw, &fields) != nil {
		return ""
	}
	kind = strings.ToLower(strings.TrimSpace(kind))
	path := firstStringField(fields, "path", "file_path", "file", "directory")
	pattern := firstStringField(fields, "pattern", "query", "regex")
	switch {
	case kind != "" && path != "":
		if pattern != "" {
			return kind + " '" + previewLine(pattern, 40) + "' " + path
		}
		return kind + " " + path
	case kind != "" && pattern != "":
		return kind + " '" + previewLine(pattern, 60) + "'"
	case path != "":
		return path
	case pattern != "":
		return pattern
	}
	return ""
}

func firstStringField(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k].(string); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func isJSONObject(raw json.RawMessage) bool {
	trimmed := strings.TrimSpace(string(raw))
	return len(trimmed) > 0 && trimmed[0] == '{'
}

func toolCallOutputText(update ToolCallUpdate) string {
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
		Output  string `json:"output"`
		Stdout  string `json:"stdout"`
		Stderr  string `json:"stderr"`
		Error   string `json:"error"`
		Content string `json:"content"`
		Result  string `json:"result"`
		Text    string `json:"text"`
		Diff    string `json:"diff"`
	}
	if err := json.Unmarshal(raw, &payload); err == nil {
		lines := []string{}
		if strings.TrimSpace(payload.Output) != "" {
			lines = append(lines, strings.TrimSpace(payload.Output))
		}
		if strings.TrimSpace(payload.Stdout) != "" {
			lines = append(lines, strings.TrimSpace(payload.Stdout))
		}
		// Fallback content fields when no primary output is present.
		if len(lines) == 0 {
			for _, v := range []string{payload.Content, payload.Result, payload.Text, payload.Diff} {
				if strings.TrimSpace(v) != "" {
					lines = append(lines, strings.TrimSpace(v))
					break
				}
			}
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
func (f *transportChannel) Run(ctx context.Context) error {
	bot, err := f.ensureBot()
	if err != nil {
		logger.Error("feishu ws: bot init failed: %v", err)
		return err
	}
	logger.Info("feishu ws: starting event loop app_id=%s", maskAppID(f.cfg.AppID))
	bot.StartHeartbeat()
	defer bot.StopHeartbeat()

	eventHandler := dispatcher.
		NewEventDispatcher(f.cfg.VerificationToken, f.cfg.EncryptKey).
		OnP2MessageReceiveV1(f.handleP2MessageReceive).
		OnP2CardActionTrigger(f.handleCardAction)

	wsClient := larkws.NewClient(
		f.cfg.AppID,
		f.cfg.AppSecret,
		larkws.WithEventHandler(eventHandler),
		larkws.WithLogLevel(larkcore.LogLevelInfo),
	)
	logger.Info("feishu ws: websocket client created, connecting")

	startErr := wsClient.Start(ctx)
	switch classifyWSRunExit(ctx.Err(), startErr) {
	case wsExitContextDone:
		logger.Warn("feishu ws: stopped by context: %v", ctx.Err())
		return nil
	case wsExitStartFailed:
		logger.Error("feishu ws: connection failed: %v", startErr)
		return finalizeWSRunError(ctx.Err(), startErr)
	default:
		logger.Warn("feishu ws: event loop exited without context cancellation")
		return nil
	}
}

func finalizeWSRunError(ctxErr, startErr error) error {
	if classifyWSRunExit(ctxErr, startErr) != wsExitStartFailed {
		return nil
	}
	return fmt.Errorf("feishu ws start: %w", startErr)
}

type wsRunExit string

const (
	wsExitContextDone wsRunExit = "context_done"
	wsExitStartFailed wsRunExit = "start_failed"
	wsExitUnexpected  wsRunExit = "unexpected"
)

func classifyWSRunExit(ctxErr error, startErr error) wsRunExit {
	if ctxErr != nil {
		return wsExitContextDone
	}
	if startErr != nil {
		return wsExitStartFailed
	}
	return wsExitUnexpected
}

func maskAppID(appID string) string {
	id := strings.TrimSpace(appID)
	if id == "" {
		return "unknown"
	}
	if len(id) <= 6 {
		return id
	}
	return id[:3] + "***" + id[len(id)-3:]
}

func (f *transportChannel) ensureBot() (*lark.Bot, error) {
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

func (f *transportChannel) ensureOpenAPIClient() (*larkoapi.Client, error) {
	f.mu.RLock()
	if f.api != nil {
		cli := f.api
		f.mu.RUnlock()
		return cli, nil
	}
	f.mu.RUnlock()

	if strings.TrimSpace(f.cfg.AppID) == "" || strings.TrimSpace(f.cfg.AppSecret) == "" {
		return nil, fmt.Errorf("feishu: app id/secret required")
	}

	cli := larkoapi.NewClient(
		f.cfg.AppID,
		f.cfg.AppSecret,
		larkoapi.WithLogLevel(larkcore.LogLevelWarn),
	)
	f.mu.Lock()
	if f.api == nil {
		f.api = cli
	}
	out := f.api
	f.mu.Unlock()
	return out, nil
}

func (f *transportChannel) parseMessagePromptBlocks(ctx context.Context, messageID string, msgType *string, content *string) []acp.ContentBlock {
	if msgType == nil || content == nil {
		return nil
	}
	switch strings.ToLower(strings.TrimSpace(*msgType)) {
	case "text":
		text := parseMessageText(msgType, content)
		if strings.TrimSpace(text) == "" {
			return nil
		}
		return []acp.ContentBlock{{Type: acp.ContentBlockTypeText, Text: text}}
	case "image":
		imageKey := parseMessageImageKey(content)
		if imageKey == "" {
			return nil
		}
		fetcher := f.imageFetcher
		if fetcher == nil {
			return nil
		}
		block, err := fetcher(ctx, messageID, imageKey)
		if err != nil {
			logger.Warn("feishu: fetch inbound image failed message=%s image=%s err=%v", messageID, imageKey, err)
			return nil
		}
		return []acp.ContentBlock{block}
	default:
		return nil
	}
}

func parseMessageImageKey(content *string) string {
	if content == nil {
		return ""
	}
	var payload struct {
		ImageKey string `json:"image_key"`
		FileKey  string `json:"file_key"`
	}
	if err := json.Unmarshal([]byte(*content), &payload); err != nil {
		return ""
	}
	if key := strings.TrimSpace(payload.ImageKey); key != "" {
		return key
	}
	return strings.TrimSpace(payload.FileKey)
}

func (f *transportChannel) fetchImageBlock(ctx context.Context, messageID, imageKey string) (acp.ContentBlock, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	data, mimeType, err := f.downloadMessageImage(ctx, messageID, imageKey)
	if err != nil {
		return acp.ContentBlock{}, err
	}
	if len(data) == 0 {
		return acp.ContentBlock{}, fmt.Errorf("feishu: image payload is empty")
	}
	mimeType = normalizeContentType(mimeType)
	if mimeType == "" {
		mimeType = normalizeContentType(http.DetectContentType(data))
	}
	if !strings.HasPrefix(mimeType, "image/") {
		mimeType = "image/png"
	}
	return acp.ContentBlock{
		Type:     acp.ContentBlockTypeImage,
		MimeType: mimeType,
		Data:     base64.StdEncoding.EncodeToString(data),
	}, nil
}

func (f *transportChannel) downloadMessageImage(ctx context.Context, messageID, imageKey string) ([]byte, string, error) {
	if data, mimeType, err := f.fetchImageViaMessageResource(ctx, messageID, imageKey); err == nil {
		return data, mimeType, nil
	}
	if data, mimeType, err := f.fetchImageViaImageAPI(ctx, imageKey); err == nil {
		return data, mimeType, nil
	}
	return nil, "", fmt.Errorf("feishu: unable to download image key=%s", strings.TrimSpace(imageKey))
}

func (f *transportChannel) fetchImageViaMessageResource(ctx context.Context, messageID, imageKey string) ([]byte, string, error) {
	if strings.TrimSpace(messageID) == "" {
		return nil, "", fmt.Errorf("message id is empty")
	}
	if strings.TrimSpace(imageKey) == "" {
		return nil, "", fmt.Errorf("image key is empty")
	}
	cli, err := f.ensureOpenAPIClient()
	if err != nil {
		return nil, "", err
	}
	req := larkim.NewGetMessageResourceReqBuilder().
		MessageId(strings.TrimSpace(messageID)).
		FileKey(strings.TrimSpace(imageKey)).
		Type("image").
		Build()
	resp, err := cli.Im.V1.MessageResource.Get(ctx, req)
	if err != nil {
		return nil, "", err
	}
	if resp == nil || resp.File == nil {
		return nil, "", fmt.Errorf("empty response file")
	}
	data, err := io.ReadAll(resp.File)
	if err != nil {
		return nil, "", err
	}
	mimeType := ""
	if resp.ApiResp != nil {
		mimeType = normalizeContentType(resp.ApiResp.Header.Get("Content-Type"))
	}
	return data, mimeType, nil
}

func (f *transportChannel) fetchImageViaImageAPI(ctx context.Context, imageKey string) ([]byte, string, error) {
	if strings.TrimSpace(imageKey) == "" {
		return nil, "", fmt.Errorf("image key is empty")
	}
	cli, err := f.ensureOpenAPIClient()
	if err != nil {
		return nil, "", err
	}
	req := larkim.NewGetImageReqBuilder().ImageKey(strings.TrimSpace(imageKey)).Build()
	resp, err := cli.Im.V1.Image.Get(ctx, req)
	if err != nil {
		return nil, "", err
	}
	if resp == nil || resp.File == nil {
		return nil, "", fmt.Errorf("empty response file")
	}
	data, err := io.ReadAll(resp.File)
	if err != nil {
		return nil, "", err
	}
	mimeType := ""
	if resp.ApiResp != nil {
		mimeType = normalizeContentType(resp.ApiResp.Header.Get("Content-Type"))
	}
	return data, mimeType, nil
}

func normalizeContentType(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if idx := strings.Index(raw, ";"); idx >= 0 {
		raw = raw[:idx]
	}
	return strings.ToLower(strings.TrimSpace(raw))
}
func (f *transportChannel) handleP2MessageReceive(ctx context.Context, event *larkim.P2MessageReceiveV1) error {
	if event == nil || event.Event == nil || event.Event.Message == nil {
		return nil
	}
	f.wsReadyOnce.Do(func() {
		logger.Info("feishu ws: connected (inbound event stream active)")
	})
	msg := event.Event.Message
	if msg.ChatId == nil || msg.MessageId == nil {
		return nil
	}
	chatID := strings.TrimSpace(*msg.ChatId)
	messageID := strings.TrimSpace(*msg.MessageId)
	if chatID == "" || messageID == "" {
		return nil
	}
	if isFeishuMessageStale(msg.CreateTime) {
		return nil
	}
	if !f.shouldHandleMessage(messageID) {
		return nil
	}
	promptBlocks := f.parseMessagePromptBlocks(ctx, messageID, msg.MessageType, msg.Content)
	if len(promptBlocks) == 0 {
		return nil
	}
	text := ""
	for _, block := range promptBlocks {
		if block.Type == acp.ContentBlockTypeText && strings.TrimSpace(block.Text) != "" {
			text = strings.TrimSpace(block.Text)
			break
		}
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
		f.resetUnifiedStream(chatID)
		f.addReceiveAck(chatID, messageID)
		h(Message{
			ChatID:    chatID,
			MessageID: messageID,
			UserID:    userID,
			Text:      text,
			Prompt:    promptBlocks,
			RouteKey:  chatID,
		})
	}
	return nil
}

func (f *transportChannel) shouldHandleMessage(messageID string) bool {
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

func (f *transportChannel) handleCardAction(_ context.Context, event *callback.CardActionTriggerEvent) (*callback.CardActionTriggerResponse, error) {
	if event == nil || event.Event == nil || event.Event.Action == nil {
		return &callback.CardActionTriggerResponse{}, nil
	}
	f.wsReadyOnce.Do(func() {
		logger.Info("feishu ws: connected (card action stream active)")
	})
	payload := event.Event
	value := map[string]string{}
	for k, v := range payload.Action.Value {
		value[k] = fmt.Sprint(v)
	}
	if strings.TrimSpace(value["kind"]) == "permission" {
		chatID := firstNonEmpty(value["chat_id"], payload.Context.OpenChatID)
		toolCallID := strings.TrimSpace(value["tool_call_id"])
		requestID := strings.TrimSpace(value["request_id"])
		selectedID := strings.TrimSpace(value["option_id"])
		selectedLabel := strings.TrimSpace(value["option_name"])
		if chatID != "" && toolCallID != "" && requestID != "" {
			f.toolMu.Lock()
			if chatCards := f.toolCards[chatID]; chatCards != nil {
				if st := chatCards[toolCallID]; st != nil && st.perm != nil && st.perm.requestID == requestID {
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
		evt := CardActionEvent{
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

var _ transport = (*transportChannel)(nil)
