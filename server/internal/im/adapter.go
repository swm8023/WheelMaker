package im

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ImAdapter is the strategy layer between client and concrete IM adapter.
// Current MVP behavior is transparent pass-through; strategy logic is added iteratively.
type ImAdapter struct {
	adapter Channel
	handler MessageHandler

	mu           sync.Mutex
	activeChatID string                      // most recent chat that sent a message
	textBuf      map[string]*strings.Builder // chatID -> buffered text chunks
	textFlush    map[string]*time.Timer      // chatID -> delayed text flush timer
	toolCalls    map[string]map[string]string
	decisions    map[string]pendingDecision // chatID -> pending (text fallback)
	decisionByID map[string]pendingDecision // decisionID -> pending (card action)
	closedDecide map[string]time.Time       // decisionID -> recently closed timestamp
	helpResolver func(ctx context.Context, chatID string) (HelpModel, error)
	nextID       atomic.Int64
	debugWriter  io.Writer
}

// New creates a pass-through bridge over adapter.
func New(adapter Channel) *ImAdapter {
	f := &ImAdapter{
		adapter:      adapter,
		textBuf:      map[string]*strings.Builder{},
		textFlush:    map[string]*time.Timer{},
		toolCalls:    map[string]map[string]string{},
		decisions:    map[string]pendingDecision{},
		decisionByID: map[string]pendingDecision{},
		closedDecide: map[string]time.Time{},
	}
	adapter.OnCardAction(f.handleCardAction)
	return f
}

// NewBridge creates a pass-through bridge over adapter.
func NewBridge(adapter Channel) *ImAdapter {
	return New(adapter)
}

// OnMessage registers user-message handler and bridges adapter inbound events.
func (f *ImAdapter) OnMessage(handler MessageHandler) {
	f.handler = handler
	f.adapter.OnMessage(func(m Message) {
		f.logIncomingMessage(m)
		if strings.TrimSpace(m.Text) == "/help" && f.tryHandleHelp(m) {
			return
		}
		if resolved := f.resolveDecision(m); resolved {
			return
		}
		if f.handler != nil {
			f.handler(m)
		}
	})
}

func (f *ImAdapter) SendText(chatID, text string) error {
	err := f.adapter.Send(chatID, text, TextNormal)
	f.logOutgoingSend(chatID, text, TextNormal, err)
	return err
}

func (f *ImAdapter) SendCard(chatID string, card Card) error {
	err := f.adapter.SendCard(chatID, "", card)
	f.logOutgoingCard(chatID, "", card, err)
	return err
}

func (f *ImAdapter) SendReaction(messageID, emoji string) error {
	err := f.adapter.SendReaction(messageID, emoji)
	f.logOutgoingReaction(messageID, emoji, err)
	return err
}

func (f *ImAdapter) SendDebug(chatID, text string) error {
	err := f.adapter.Send(chatID, text, TextDebug)
	f.logOutgoingSend(chatID, text, TextDebug, err)
	return err
}

func (f *ImAdapter) SendSystem(chatID, text string) error {
	err := f.adapter.Send(chatID, text, TextSystem)
	f.logOutgoingSend(chatID, text, TextSystem, err)
	return err
}

func (f *ImAdapter) Run(ctx context.Context) error {
	return f.adapter.Run(ctx)
}

// SetActiveChatID records the most recent chat to be addressed.
// Client calls this on every incoming message so that Reply* and Emit
// can route outbound messages without the caller specifying a chatID.
func (f *ImAdapter) SetActiveChatID(id string) {
	id = strings.TrimSpace(id)
	f.mu.Lock()
	f.activeChatID = id
	f.mu.Unlock()
}

// ActiveChatID returns the most recently set active chat ID.
func (f *ImAdapter) ActiveChatID() string {
	f.mu.Lock()
	id := f.activeChatID
	f.mu.Unlock()
	return id
}

// ReplySystem sends a system-level text to the active chat.
func (f *ImAdapter) ReplySystem(text string) error {
	chatID := f.ActiveChatID()
	if chatID == "" {
		return nil
	}
	return f.SendSystem(chatID, text)
}

// ReplyText sends a plain text message to the active chat.
func (f *ImAdapter) ReplyText(text string) error {
	chatID := f.ActiveChatID()
	if chatID == "" {
		return nil
	}
	return f.SendText(chatID, text)
}

// ReplyDebug sends a debug message to the active chat.
func (f *ImAdapter) ReplyDebug(text string) error {
	chatID := f.ActiveChatID()
	if chatID == "" {
		return nil
	}
	return f.SendDebug(chatID, text)
}

// SetHelpResolver injects realtime help payload provider from client.
func (f *ImAdapter) SetHelpResolver(resolver func(ctx context.Context, chatID string) (HelpModel, error)) {
	f.mu.Lock()
	f.helpResolver = resolver
	f.mu.Unlock()
}

// SetDebugLogger sets optional IM-level debug logging writer.
func (f *ImAdapter) SetDebugLogger(w io.Writer) {
	f.mu.Lock()
	f.debugWriter = w
	f.mu.Unlock()
}

// Emit renders semantic updates with incremental text flushing and tool-call streaming.
// If u.ChatID is empty, the current activeChatID is used.
func (f *ImAdapter) Emit(_ context.Context, u IMUpdate) error {
	chatID := strings.TrimSpace(u.ChatID)
	if chatID == "" {
		chatID = f.ActiveChatID()
	}
	if chatID == "" {
		return nil
	}
	switch u.UpdateType {
	case IMUpdateText:
		f.enqueueTextChunk(chatID, u.Text)
	case IMUpdateDone:
		if err := f.flushTextNow(chatID); err != nil {
			return err
		}
		f.mu.Lock()
		delete(f.toolCalls, chatID)
		f.mu.Unlock()
		return f.adapter.MarkDone(chatID)
	case IMUpdateError:
		if err := f.flushTextNow(chatID); err != nil {
			return err
		}
		msg := strings.TrimSpace(u.Text)
		if msg == "" {
			msg = "Agent request failed."
		}
		return f.SendText(chatID, msg)
	case IMUpdateThought:
		if err := f.flushTextNow(chatID); err != nil {
			return err
		}
		if strings.TrimSpace(u.Text) != "" {
			return f.SendText(chatID, strings.TrimSpace(u.Text))
		}
	case IMUpdateToolCall:
		if err := f.flushTextNow(chatID); err != nil {
			return err
		}
		return f.emitToolCall(chatID, u.Raw)
	case IMUpdatePlan:
		if err := f.flushTextNow(chatID); err != nil {
			return err
		}
		if msg := renderPlanUpdate(u.Raw); msg != "" {
			return f.SendText(chatID, msg)
		}
	case IMUpdateConfigOption:
		if err := f.flushTextNow(chatID); err != nil {
			return err
		}
		if msg := renderConfigOptionUpdate(u.Raw); msg != "" {
			return f.SendText(chatID, msg)
		}
	}
	return nil
}

const textFlushDelay = 300 * time.Millisecond

func (f *ImAdapter) enqueueTextChunk(chatID, chunk string) {
	if chunk == "" {
		return
	}
	f.mu.Lock()
	buf := f.textBuf[chatID]
	if buf == nil {
		buf = &strings.Builder{}
		f.textBuf[chatID] = buf
	}
	buf.WriteString(chunk)
	timer := f.textFlush[chatID]
	if timer == nil {
		f.textFlush[chatID] = time.AfterFunc(textFlushDelay, func() {
			_ = f.flushTextNow(chatID)
		})
	} else {
		timer.Reset(textFlushDelay)
	}
	flushNow := strings.Contains(chunk, "\n") || buf.Len() >= 320
	f.mu.Unlock()

	if flushNow {
		_ = f.flushTextNow(chatID)
	}
}

func (f *ImAdapter) flushTextNow(chatID string) error {
	f.mu.Lock()
	buf := f.textBuf[chatID]
	delete(f.textBuf, chatID)
	if timer := f.textFlush[chatID]; timer != nil {
		timer.Stop()
		delete(f.textFlush, chatID)
	}
	f.mu.Unlock()

	if buf == nil || buf.Len() == 0 {
		return nil
	}
	return f.SendText(chatID, buf.String())
}

func (f *ImAdapter) emitToolCall(chatID string, raw []byte) error {
	upd, signature, ok := parseToolCallUpdate(raw)
	if !ok {
		return nil
	}
	if !f.shouldEmitToolCall(chatID, upd.ToolCallID, signature) {
		return nil
	}
	err := f.adapter.SendCard(chatID, "", ToolCallCard{Update: upd})
	f.logOutgoingToolCall(chatID, upd, err)
	return err
}

func (f *ImAdapter) shouldEmitToolCall(chatID, toolCallID, signature string) bool {
	if strings.TrimSpace(toolCallID) == "" {
		return true
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	chatCalls := f.toolCalls[chatID]
	if chatCalls == nil {
		chatCalls = map[string]string{}
		f.toolCalls[chatID] = chatCalls
	}
	if prev, ok := chatCalls[toolCallID]; ok && prev == signature {
		return false
	}
	chatCalls[toolCallID] = signature
	return true
}

type pendingDecision struct {
	id     string
	chatID string
	req    DecisionRequestWithDeadline
	ch     chan DecisionResult
}

type DecisionRequestWithDeadline struct {
	DecisionRequest
	deadline time.Time
}

// RequestDecision sends a textual decision prompt and waits for reply.
// Card-interaction support will be added on top of the same state machine.
func (f *ImAdapter) RequestDecision(ctx context.Context, req DecisionRequest) (DecisionResult, error) {
	chatID := strings.TrimSpace(req.ChatID)
	if chatID == "" {
		chatID = f.ActiveChatID()
	}
	if chatID == "" {
		return DecisionResult{Outcome: "invalid"}, fmt.Errorf("decision: no active chat id")
	}
	timeout := 2 * time.Minute
	if v := strings.TrimSpace(req.Hint["timeoutSec"]); v != "" {
		if sec, err := strconv.Atoi(v); err == nil && sec > 0 {
			timeout = time.Duration(sec) * time.Second
		}
	}
	decisionID := fmt.Sprintf("dec-%d", f.nextID.Add(1))
	ch := make(chan DecisionResult, 1)
	pd := pendingDecision{
		id:     decisionID,
		chatID: chatID,
		req: DecisionRequestWithDeadline{
			DecisionRequest: req,
			deadline:        time.Now().Add(timeout),
		},
		ch: ch,
	}
	f.mu.Lock()
	f.decisions[chatID] = pd
	f.decisionByID[decisionID] = pd
	f.mu.Unlock()
	f.writeDebugLine(fmt.Sprintf("event=decision_open id=%q chat=%q kind=%q timeout_sec=%d options=%d",
		decisionID, chatID, string(req.Kind), int(timeout/time.Second), len(req.Options)))

	meta := map[string]string{
		"decision_id": pd.id,
		"chat_id":     pd.chatID,
	}
	for k, v := range req.Meta {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		meta[k] = strings.TrimSpace(v)
	}
	_ = f.adapter.SendCard(pd.chatID, "", OptionsCard{Title: req.Title, Body: req.Body, Options: req.Options, Meta: meta})
	f.logOutgoingOptions(chatID, req, nil)

	select {
	case r := <-ch:
		f.writeDebugLine(fmt.Sprintf("event=decision_resolved id=%q chat=%q outcome=%q source=%q option_id=%q",
			decisionID, chatID, r.Outcome, r.Source, r.OptionID))
		return r, nil
	case <-ctx.Done():
		f.clearDecision(chatID, decisionID, ch)
		f.writeDebugLine(fmt.Sprintf("event=decision_expire id=%q chat=%q reason=%q", decisionID, chatID, "context_done"))
		return DecisionResult{Outcome: "cancelled", Source: "default_policy"}, nil
	case <-time.After(timeout):
		f.clearDecision(chatID, decisionID, ch)
		f.writeDebugLine(fmt.Sprintf("event=decision_expire id=%q chat=%q reason=%q", decisionID, chatID, "timeout"))
		return DecisionResult{Outcome: "timeout", Source: "default_policy"}, nil
	}
}

func (f *ImAdapter) clearDecision(chatID, decisionID string, ch chan DecisionResult) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if pd, ok := f.decisions[chatID]; ok && pd.ch == ch {
		delete(f.decisions, chatID)
	}
	if pd, ok := f.decisionByID[decisionID]; ok && pd.ch == ch {
		delete(f.decisionByID, decisionID)
	}
	f.markDecisionClosedLocked(decisionID)
}

func renderDecisionPrompt(req DecisionRequest) string {
	var b strings.Builder
	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = "Decision required"
	}
	b.WriteString(title)
	if body := strings.TrimSpace(req.Body); body != "" {
		b.WriteString("\n")
		b.WriteString(body)
	}
	if len(req.Options) > 0 {
		b.WriteString("\nReply with: <index> | <option id> | cancel")
		for i, opt := range req.Options {
			b.WriteString(fmt.Sprintf("\n%d. %s (id=%s)", i+1, opt.Label, opt.ID))
		}
	}
	return b.String()
}

func (f *ImAdapter) tryHandleHelp(m Message) bool {
	f.mu.Lock()
	resolver := f.helpResolver
	f.mu.Unlock()
	if resolver == nil {
		return false
	}
	model, err := resolver(context.Background(), m.ChatID)
	if err != nil {
		_ = f.SendText(m.ChatID, fmt.Sprintf("help load error: %v", err))
		return true
	}
	if err := f.sendHelpPage(m.ChatID, "", model, model.RootMenu, 0); err != nil {
		_ = f.SendText(m.ChatID, strings.TrimSpace(model.Body))
	}
	return true
}

func renderDefault(v, d string) string {
	if strings.TrimSpace(v) == "" {
		return d
	}
	return strings.TrimSpace(v)
}

func (f *ImAdapter) handleCardAction(evt CardActionEvent) {
	kind := strings.TrimSpace(evt.Value["kind"])
	switch kind {
	case "decision":
		decisionID := strings.TrimSpace(evt.Value["decision_id"])
		if decisionID == "" {
			f.writeDebugLine("event=card_action_drop reason=\"missing_decision_id\"")
			return
		}
		f.mu.Lock()
		pd, ok := f.decisionByID[decisionID]
		if ok {
			delete(f.decisionByID, decisionID)
			delete(f.decisions, pd.chatID)
			f.markDecisionClosedLocked(decisionID)
		}
		f.mu.Unlock()
		if !ok {
			if f.wasDecisionClosedRecently(decisionID) {
				f.writeDebugLine(fmt.Sprintf("event=card_action_ignore reason=%q decision_id=%q", "decision_already_closed", decisionID))
				return
			}
			f.writeDebugLine(fmt.Sprintf("event=card_action_drop reason=%q decision_id=%q", "decision_not_found", decisionID))
			return
		}
		res := DecisionResult{
			Outcome:  "selected",
			OptionID: strings.TrimSpace(evt.Value["option_id"]),
			Value:    strings.TrimSpace(evt.Value["value"]),
			ActorID:  evt.UserID,
			Source:   "card_action",
		}
		f.writeDebugLine(fmt.Sprintf("event=card_action_accept decision_id=%q option_id=%q actor=%q",
			decisionID, res.OptionID, evt.UserID))
		select {
		case pd.ch <- res:
		default:
		}
	case "help_option":
		cmd := strings.TrimSpace(evt.Value["command"])
		val := strings.TrimSpace(evt.Value["value"])
		chatID := strings.TrimSpace(evt.Value["chat_id"])
		if chatID == "" {
			chatID = evt.ChatID
		}
		if cmd == "" || f.handler == nil || chatID == "" {
			return
		}
		text := cmd
		if val != "" {
			text = cmd + " " + val
		}
		f.handler(Message{
			ChatID: chatID,
			UserID: evt.UserID,
			Text:   text,
		})
		f.mu.Lock()
		resolver := f.helpResolver
		f.mu.Unlock()
		if resolver == nil {
			return
		}
		model, err := resolver(context.Background(), chatID)
		if err != nil {
			_ = f.SendText(chatID, fmt.Sprintf("help load error: %v", err))
			return
		}
		_ = f.sendHelpPage(chatID, evt.MessageID, model, model.RootMenu, 0)
	case "help_menu":
		chatID := strings.TrimSpace(evt.Value["chat_id"])
		if chatID == "" {
			chatID = evt.ChatID
		}
		menuID := strings.TrimSpace(evt.Value["menu_id"])
		if chatID == "" {
			return
		}
		f.mu.Lock()
		resolver := f.helpResolver
		f.mu.Unlock()
		if resolver == nil {
			return
		}
		model, err := resolver(context.Background(), chatID)
		if err != nil {
			_ = f.SendText(chatID, fmt.Sprintf("help load error: %v", err))
			return
		}
		_ = f.sendHelpPage(chatID, evt.MessageID, model, menuID, 0)
	case "help_page":
		chatID := strings.TrimSpace(evt.Value["chat_id"])
		if chatID == "" {
			chatID = evt.ChatID
		}
		menuID := strings.TrimSpace(evt.Value["menu_id"])
		pageStr := strings.TrimSpace(evt.Value["page"])
		page := 0
		if v, err := strconv.Atoi(pageStr); err == nil && v >= 0 {
			page = v
		}
		f.mu.Lock()
		resolver := f.helpResolver
		f.mu.Unlock()
		if resolver == nil {
			return
		}
		model, err := resolver(context.Background(), chatID)
		if err != nil {
			_ = f.SendText(chatID, fmt.Sprintf("help load error: %v", err))
			return
		}
		_ = f.sendHelpPage(chatID, evt.MessageID, model, menuID, page)
	}
}

func (f *ImAdapter) markDecisionClosedLocked(decisionID string) {
	decisionID = strings.TrimSpace(decisionID)
	if decisionID == "" {
		return
	}
	now := time.Now()
	const keepTTL = 2 * time.Hour
	cutoff := now.Add(-keepTTL)
	for id, ts := range f.closedDecide {
		if ts.Before(cutoff) {
			delete(f.closedDecide, id)
		}
	}
	f.closedDecide[decisionID] = now
}

func (f *ImAdapter) wasDecisionClosedRecently(decisionID string) bool {
	decisionID = strings.TrimSpace(decisionID)
	if decisionID == "" {
		return false
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	ts, ok := f.closedDecide[decisionID]
	if !ok {
		return false
	}
	return time.Since(ts) <= 2*time.Hour
}

func (f *ImAdapter) sendHelpPage(chatID, messageID string, model HelpModel, menuID string, page int) error {
	card := buildHelpCard(chatID, model, menuID, page)
	mid := strings.TrimSpace(messageID)
	err := f.adapter.SendCard(chatID, mid, card)
	f.logOutgoingCard(chatID, mid, card, err)
	return err
}

func buildHelpCard(chatID string, model HelpModel, menuID string, page int) RawCard {
	const pageSize = 8
	title, body, options, parent := resolveHelpMenu(model, menuID)
	if page < 0 {
		page = 0
	}
	total := len(options)
	maxPage := 0
	if total > 0 {
		maxPage = (total - 1) / pageSize
		if page > maxPage {
			page = maxPage
		}
	}
	start := page * pageSize
	end := start + pageSize
	if end > total {
		end = total
	}

	actions := make([]map[string]any, 0, end-start)
	for _, opt := range options[start:end] {
		if strings.TrimSpace(opt.MenuID) != "" {
			actions = append(actions, map[string]any{
				"tag": "button",
				"text": map[string]any{
					"tag":     "plain_text",
					"content": opt.Label,
				},
				"type": "default",
				"value": map[string]any{
					"kind":    "help_menu",
					"chat_id": chatID,
					"menu_id": opt.MenuID,
				},
			})
			continue
		}
		actions = append(actions, map[string]any{
			"tag": "button",
			"text": map[string]any{
				"tag":     "plain_text",
				"content": opt.Label,
			},
			"type": "default",
			"value": map[string]any{
				"kind":    "help_option",
				"chat_id": chatID,
				"menu_id": menuID,
				"command": opt.Command,
				"value":   opt.Value,
			},
		})
	}

	elements := []map[string]any{
		{"tag": "markdown", "content": strings.TrimSpace(body)},
	}
	if len(actions) > 0 {
		elements = append(elements, map[string]any{"tag": "action", "actions": actions})
	}
	if maxPage > 0 {
		nav := make([]map[string]any, 0, 2)
		if page > 0 {
			nav = append(nav, map[string]any{
				"tag":  "button",
				"text": map[string]any{"tag": "plain_text", "content": "Prev"},
				"type": "default",
				"value": map[string]any{
					"kind":    "help_page",
					"chat_id": chatID,
					"menu_id": menuID,
					"page":    strconv.Itoa(page - 1),
				},
			})
		}
		if page < maxPage {
			nav = append(nav, map[string]any{
				"tag":  "button",
				"text": map[string]any{"tag": "plain_text", "content": "Next"},
				"type": "default",
				"value": map[string]any{
					"kind":    "help_page",
					"chat_id": chatID,
					"menu_id": menuID,
					"page":    strconv.Itoa(page + 1),
				},
			})
		}
		if len(nav) > 0 {
			elements = append(elements, map[string]any{"tag": "action", "actions": nav})
		}
	}
	if strings.TrimSpace(parent) != "" {
		elements = append(elements, map[string]any{
			"tag": "action",
			"actions": []map[string]any{
				{
					"tag":  "button",
					"text": map[string]any{"tag": "plain_text", "content": "Back"},
					"type": "primary",
					"value": map[string]any{
						"kind":    "help_menu",
						"chat_id": chatID,
						"menu_id": parent,
					},
				},
			},
		})
	}
	headerTitle := renderDefault(title, "Help")
	if maxPage > 0 {
		headerTitle = fmt.Sprintf("%s (%d/%d)", headerTitle, page+1, maxPage+1)
	}

	header := map[string]any{
		"title": map[string]any{
			"tag":     "plain_text",
			"content": headerTitle,
		},
		"template": "green",
	}
	return RawCard{
		"config":   map[string]any{"update_multi": true},
		"header":   header,
		"elements": elements,
	}
}

func resolveHelpMenu(model HelpModel, menuID string) (title, body string, options []HelpOption, parent string) {
	rootID := strings.TrimSpace(model.RootMenu)
	if rootID == "" {
		rootID = "root"
	}
	if strings.TrimSpace(menuID) == "" || menuID == rootID {
		return renderDefault(model.Title, "Help"), strings.TrimSpace(model.Body), model.Options, ""
	}
	if menu, ok := model.Menus[menuID]; ok {
		return renderDefault(menu.Title, renderDefault(model.Title, "Help")), strings.TrimSpace(menu.Body), menu.Options, strings.TrimSpace(menu.Parent)
	}
	return renderDefault(model.Title, "Help"), strings.TrimSpace(model.Body), model.Options, ""
}

func (f *ImAdapter) resolveDecision(m Message) bool {
	chatID := strings.TrimSpace(m.ChatID)
	if chatID == "" {
		return false
	}
	f.mu.Lock()
	pd, ok := f.decisions[chatID]
	if ok {
		delete(f.decisions, chatID)
		delete(f.decisionByID, pd.id)
	}
	f.mu.Unlock()
	if !ok {
		return false
	}
	result := parseDecisionReply(strings.TrimSpace(m.Text), pd.req.Options)
	select {
	case pd.ch <- result:
	default:
	}
	return true
}

func parseDecisionReply(input string, opts []DecisionOption) DecisionResult {
	v := strings.ToLower(strings.TrimSpace(input))
	if v == "" {
		return DecisionResult{Outcome: "invalid", Source: "text_reply"}
	}
	if v == "cancel" {
		return DecisionResult{Outcome: "cancelled", Source: "text_reply"}
	}
	if i := parseIndex(v); i >= 1 && i <= len(opts) {
		o := opts[i-1]
		return DecisionResult{Outcome: "selected", OptionID: o.ID, Value: o.Value, Source: "text_reply"}
	}
	for _, o := range opts {
		if strings.EqualFold(v, o.ID) || strings.EqualFold(v, o.Label) {
			return DecisionResult{Outcome: "selected", OptionID: o.ID, Value: o.Value, Source: "text_reply"}
		}
	}
	return DecisionResult{Outcome: "invalid", Source: "text_reply"}
}

func parseToolCallUpdate(raw []byte) (ToolCallUpdate, string, bool) {
	if len(raw) == 0 {
		return ToolCallUpdate{}, "", false
	}
	var u ToolCallUpdate
	if err := json.Unmarshal(raw, &u); err != nil {
		return ToolCallUpdate{}, "", false
	}
	u.ToolCallID = strings.TrimSpace(u.ToolCallID)
	if u.ToolCallID == "" {
		return ToolCallUpdate{}, "", false
	}
	u.Title = strings.TrimSpace(u.Title)
	u.Status = strings.TrimSpace(u.Status)
	u.Kind = strings.TrimSpace(u.Kind)

	normalizedOutput := normalizeToolCallOutput(u)
	// Some agents emit heartbeat-like tool_call_update packets with no status/title/output.
	// Ignore those to avoid spurious "pending" regressions and duplicate card churn.
	if u.SessionUpdate == "tool_call_update" && u.Status == "" && u.Title == "" && u.Kind == "" && normalizedOutput == "" {
		return ToolCallUpdate{}, "", false
	}

	signature := strings.Join([]string{
		u.SessionUpdate,
		u.ToolCallID,
		u.Title,
		u.Kind,
		u.Status,
		normalizedOutput,
	}, "|")
	return u, signature, true
}

func normalizeToolCallOutput(u ToolCallUpdate) string {
	// Prefer one latest output snapshot instead of concatenating every field;
	// concatenation can create repeated code blocks and truncated mixed fragments.
	switch {
	case decodeRawText(u.RawOutput) != "":
		return previewText(stripCodeFenceLines(decodeRawText(u.RawOutput)), 1200)
	case latestToolCallContentText(u) != "":
		return previewText(stripCodeFenceLines(latestToolCallContentText(u)), 1200)
	case decodeRawText(u.RawInput) != "":
		return previewText(stripCodeFenceLines(decodeRawText(u.RawInput)), 1200)
	default:
		return ""
	}
}

func latestToolCallContentText(u ToolCallUpdate) string {
	for i := len(u.ToolCallContent) - 1; i >= 0; i-- {
		c := u.ToolCallContent[i]
		if t := strings.TrimSpace(c.NewText); t != "" {
			return t
		}
		if t := decodeToolCallContentText(c.Content); t != "" {
			return t
		}
	}
	return ""
}

func decodeToolCallContentText(raw json.RawMessage) string {
	raw = json.RawMessage(strings.TrimSpace(string(raw)))
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var c struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &c); err == nil && strings.TrimSpace(c.Text) != "" {
		return strings.TrimSpace(c.Text)
	}
	return decodeRawText(raw)
}

func stripCodeFenceLines(s string) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\r", ""))
	if s == "" {
		return ""
	}
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			continue
		}
		out = append(out, line)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
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
	var anyVal any
	if err := json.Unmarshal(raw, &anyVal); err != nil {
		return strings.TrimSpace(string(raw))
	}
	b, err := json.Marshal(anyVal)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// RenderToolCallMessage renders a ToolCallUpdate as a plain-text message.
// Channel implementations that don't support rich tool-call cards can use this
// to produce a text fallback inside their SendToolCall method.
func RenderToolCallMessage(u ToolCallUpdate) string {
	return renderToolCallMessage(u)
}

func renderToolCallMessage(u ToolCallUpdate) string {
	icon := "??"
	switch strings.ToLower(strings.TrimSpace(u.Status)) {
	case "completed":
		icon = "?"
	case "failed":
		icon = "?"
	case "in_progress":
		icon = "?"
	case "pending":
		icon = "??"
	}
	title := strings.TrimSpace(u.Title)
	if title == "" {
		title = "tool"
	}
	msg := fmt.Sprintf("%s %s [%s] (%s)", icon, title, u.Status, u.ToolCallID)
	if strings.EqualFold(u.Status, "pending") {
		msg += "\nWaiting for confirmation."
	}
	out := normalizeToolCallOutput(u)
	if out != "" {
		msg += "\n" + out
	}
	return msg
}
func renderPlanUpdate(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	var u struct {
		Entries []struct {
			Content  string `json:"content"`
			Status   string `json:"status"`
			Priority string `json:"priority"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(raw, &u); err != nil || len(u.Entries) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("🗂 Plan update:")
	for i, e := range u.Entries {
		line := strings.TrimSpace(e.Content)
		if line == "" {
			continue
		}
		b.WriteString(fmt.Sprintf("\n%d. %s", i+1, line))
		if strings.TrimSpace(e.Status) != "" {
			b.WriteString(" [" + strings.TrimSpace(e.Status) + "]")
		}
	}
	return b.String()
}

func renderConfigOptionUpdate(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	var u struct {
		ConfigOptions []struct {
			ID           string `json:"id"`
			Category     string `json:"category"`
			CurrentValue string `json:"currentValue"`
		} `json:"configOptions"`
	}
	if err := json.Unmarshal(raw, &u); err != nil || len(u.ConfigOptions) == 0 {
		return ""
	}
	mode := ""
	model := ""
	for _, opt := range u.ConfigOptions {
		if mode == "" && (opt.ID == "mode" || strings.EqualFold(opt.Category, "mode")) {
			mode = strings.TrimSpace(opt.CurrentValue)
		}
		if model == "" && (opt.ID == "model" || strings.EqualFold(opt.Category, "model")) {
			model = strings.TrimSpace(opt.CurrentValue)
		}
	}
	if mode == "" && model == "" {
		return "⚙️ Config updated."
	}
	return fmt.Sprintf("⚙️ Config updated: mode=%s model=%s", renderDefault(mode, "unknown"), renderDefault(model, "unknown"))
}

func parseIndex(v string) int {
	n := 0
	for _, ch := range v {
		if ch < '0' || ch > '9' {
			return -1
		}
		n = n*10 + int(ch-'0')
	}
	return n
}

// CanHandleDecision always returns true because all Channel implementations
// are required to support SendOptions.
func (f *ImAdapter) CanHandleDecision() bool {
	return true
}

var _ UpdateEmitter = (*ImAdapter)(nil)
var _ DecisionRequester = (*ImAdapter)(nil)
var _ HelpResolverSetter = (*ImAdapter)(nil)

func (f *ImAdapter) writeDebugLine(line string) {
	f.mu.Lock()
	w := f.debugWriter
	f.mu.Unlock()
	if w == nil || strings.TrimSpace(line) == "" {
		return
	}
	_, _ = fmt.Fprintf(w, "->[im] %s\n", strings.TrimSpace(line))
}

func (f *ImAdapter) writeDebugInbound(line string) {
	f.mu.Lock()
	w := f.debugWriter
	f.mu.Unlock()
	if w == nil || strings.TrimSpace(line) == "" {
		return
	}
	_, _ = fmt.Fprintf(w, "<-[im] %s\n", strings.TrimSpace(line))
}

func previewText(s string, maxRunes int) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	r := []rune(s)
	if len(r) <= maxRunes {
		return s
	}
	return string(r[:maxRunes]) + "..."
}

func (f *ImAdapter) logIncomingMessage(m Message) {
	f.writeDebugInbound(fmt.Sprintf("event=message chat=%q msg=%q user=%q text=%q",
		m.ChatID, m.MessageID, m.UserID, previewText(m.Text, 300)))
}

func (f *ImAdapter) logOutgoingSend(chatID, text string, kind TextKind, err error) {
	kindStr := "send_text"
	switch kind {
	case TextDebug:
		if err != nil {
			f.writeDebugLine(fmt.Sprintf("event=send_debug status=error chat=%q err=%q text=%q",
				chatID, err.Error(), previewText(text, 300)))
		}
		return // suppress successful debug log to avoid duplicate noise
	case TextSystem:
		kindStr = "send_system"
	}
	if err != nil {
		f.writeDebugLine(fmt.Sprintf("event=%s status=error chat=%q err=%q text=%q",
			kindStr, chatID, err.Error(), previewText(text, 300)))
		return
	}
	f.writeDebugLine(fmt.Sprintf("event=%s status=ok chat=%q len=%d text=%q",
		kindStr, chatID, len([]rune(text)), previewText(text, 300)))
}

func (f *ImAdapter) logOutgoingCard(chatID, messageID string, card Card, err error) {
	raw, _ := json.Marshal(card)
	preview := previewText(string(raw), 400)
	event := "send_card"
	if messageID != "" {
		event = "update_card"
	}
	if err != nil {
		f.writeDebugLine(fmt.Sprintf("event=%s status=error chat=%q msg=%q err=%q card=%q",
			event, chatID, messageID, err.Error(), preview))
		return
	}
	f.writeDebugLine(fmt.Sprintf("event=%s status=ok chat=%q msg=%q card=%q", event, chatID, messageID, preview))
}

func (f *ImAdapter) logOutgoingReaction(messageID, emoji string, err error) {
	if err != nil {
		f.writeDebugLine(fmt.Sprintf("event=send_reaction status=error msg=%q emoji=%q err=%q",
			messageID, emoji, err.Error()))
		return
	}
	f.writeDebugLine(fmt.Sprintf("event=send_reaction status=ok msg=%q emoji=%q", messageID, emoji))
}

func (f *ImAdapter) logOutgoingOptions(chatID string, req DecisionRequest, err error) {
	if err != nil {
		f.writeDebugLine(fmt.Sprintf("event=send_options status=error chat=%q title=%q kind=%q options=%d err=%q",
			chatID, previewText(req.Title, 120), string(req.Kind), len(req.Options), err.Error()))
		return
	}
	f.writeDebugLine(fmt.Sprintf("event=send_options status=ok chat=%q title=%q kind=%q options=%d",
		chatID, previewText(req.Title, 120), string(req.Kind), len(req.Options)))
}

func (f *ImAdapter) logOutgoingToolCall(chatID string, upd ToolCallUpdate, err error) {
	if err != nil {
		f.writeDebugLine(fmt.Sprintf("event=send_tool_call status=error chat=%q id=%q title=%q tool_status=%q err=%q",
			chatID, upd.ToolCallID, previewText(upd.Title, 120), upd.Status, err.Error()))
		return
	}
	f.writeDebugLine(fmt.Sprintf("event=send_tool_call status=ok chat=%q id=%q title=%q tool_status=%q",
		chatID, upd.ToolCallID, previewText(upd.Title, 120), upd.Status))
}
