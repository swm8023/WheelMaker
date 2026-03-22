package forwarder

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/swm8023/wheelmaker/internal/im"
)

// Adapter is the low-level IM execution layer.
// Forwarder owns strategy and delegates transport/platform operations to Adapter.
type Adapter interface {
	im.Channel
}

// Forwarder is the strategy layer between client and concrete IM adapter.
// Current MVP behavior is transparent pass-through; strategy logic is added iteratively.
type Forwarder struct {
	adapter Adapter
	handler im.MessageHandler

	mu           sync.Mutex
	textBuf      map[string]*strings.Builder // chatID -> buffered text chunks
	decisions    map[string]pendingDecision  // chatID -> pending (text fallback)
	decisionByID map[string]pendingDecision  // decisionID -> pending (card action)
	helpResolver func(ctx context.Context, chatID string) (im.HelpModel, error)
	nextID       atomic.Int64
}

// New creates a pass-through forwarder over adapter.
func New(adapter Adapter) *Forwarder {
	f := &Forwarder{
		adapter:      adapter,
		textBuf:      map[string]*strings.Builder{},
		decisions:    map[string]pendingDecision{},
		decisionByID: map[string]pendingDecision{},
	}
	if sub, ok := adapter.(im.CardActionSubscriber); ok {
		sub.OnCardAction(f.handleCardAction)
	}
	return f
}

// OnMessage registers user-message handler and bridges adapter inbound events.
func (f *Forwarder) OnMessage(handler im.MessageHandler) {
	f.handler = handler
	f.adapter.OnMessage(func(m im.Message) {
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

func (f *Forwarder) SendText(chatID, text string) error {
	return f.adapter.SendText(chatID, text)
}

func (f *Forwarder) SendCard(chatID string, card im.Card) error {
	return f.adapter.SendCard(chatID, card)
}

func (f *Forwarder) SendReaction(messageID, emoji string) error {
	return f.adapter.SendReaction(messageID, emoji)
}

func (f *Forwarder) SendDebug(chatID, text string) error {
	if sender, ok := f.adapter.(im.DebugSender); ok {
		return sender.SendDebug(chatID, text)
	}
	return f.adapter.SendText(chatID, text)
}

func (f *Forwarder) Run(ctx context.Context) error {
	return f.adapter.Run(ctx)
}

// SetHelpResolver injects realtime help payload provider from client.
func (f *Forwarder) SetHelpResolver(resolver func(ctx context.Context, chatID string) (im.HelpModel, error)) {
	f.mu.Lock()
	f.helpResolver = resolver
	f.mu.Unlock()
}

// Emit renders semantic updates. Current policy: buffer text chunks and flush on done.
func (f *Forwarder) Emit(_ context.Context, u im.IMUpdate) error {
	chatID := strings.TrimSpace(u.ChatID)
	if chatID == "" {
		return nil
	}
	switch u.UpdateType {
	case "text":
		f.mu.Lock()
		buf := f.textBuf[chatID]
		if buf == nil {
			buf = &strings.Builder{}
			f.textBuf[chatID] = buf
		}
		buf.WriteString(u.Text)
		f.mu.Unlock()
	case "done":
		f.mu.Lock()
		buf := f.textBuf[chatID]
		delete(f.textBuf, chatID)
		f.mu.Unlock()
		if buf != nil && strings.TrimSpace(buf.String()) != "" {
			return f.adapter.SendText(chatID, buf.String())
		}
	case "error":
		msg := strings.TrimSpace(u.Text)
		if msg == "" {
			msg = "Agent request failed."
		}
		return f.adapter.SendText(chatID, msg)
	case "thought":
		if strings.TrimSpace(u.Text) != "" {
			return f.adapter.SendText(chatID, "🤔 "+strings.TrimSpace(u.Text))
		}
	case "tool_call":
		if msg := renderToolCallUpdate(u.Raw); msg != "" {
			return f.adapter.SendText(chatID, msg)
		}
	case "plan":
		if msg := renderPlanUpdate(u.Raw); msg != "" {
			return f.adapter.SendText(chatID, msg)
		}
	case "config_option_update":
		if msg := renderConfigOptionUpdate(u.Raw); msg != "" {
			return f.adapter.SendText(chatID, msg)
		}
	}
	return nil
}

type pendingDecision struct {
	id     string
	chatID string
	req    DecisionRequestWithDeadline
	ch     chan im.DecisionResult
}

type DecisionRequestWithDeadline struct {
	im.DecisionRequest
	deadline time.Time
}

// RequestDecision sends a textual decision prompt and waits for reply.
// Card-interaction support will be added on top of the same state machine.
func (f *Forwarder) RequestDecision(ctx context.Context, req im.DecisionRequest) (im.DecisionResult, error) {
	chatID := strings.TrimSpace(req.ChatID)
	if chatID == "" {
		return im.DecisionResult{Outcome: "invalid"}, fmt.Errorf("decision: empty chat id")
	}
	timeout := 2 * time.Minute
	if v := strings.TrimSpace(req.Hint["timeoutSec"]); v != "" {
		if sec, err := strconv.Atoi(v); err == nil && sec > 0 {
			timeout = time.Duration(sec) * time.Second
		}
	}
	decisionID := fmt.Sprintf("dec-%d", f.nextID.Add(1))
	ch := make(chan im.DecisionResult, 1)
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

	meta := map[string]string{
		"decision_id": pd.id,
		"chat_id":     pd.chatID,
	}
	if sender, ok := f.adapter.(im.OptionSender); ok {
		if err := sender.SendOptions(pd.chatID, req.Title, req.Body, req.Options, meta); err != nil {
			_ = f.adapter.SendText(chatID, renderDecisionPrompt(req))
		}
	} else {
		_ = f.adapter.SendText(chatID, renderDecisionPrompt(req))
	}

	select {
	case r := <-ch:
		return r, nil
	case <-ctx.Done():
		f.clearDecision(chatID, decisionID, ch)
		return im.DecisionResult{Outcome: "cancelled", Source: "default_policy"}, nil
	case <-time.After(timeout):
		f.clearDecision(chatID, decisionID, ch)
		return im.DecisionResult{Outcome: "timeout", Source: "default_policy"}, nil
	}
}

func (f *Forwarder) clearDecision(chatID, decisionID string, ch chan im.DecisionResult) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if pd, ok := f.decisions[chatID]; ok && pd.ch == ch {
		delete(f.decisions, chatID)
	}
	if pd, ok := f.decisionByID[decisionID]; ok && pd.ch == ch {
		delete(f.decisionByID, decisionID)
	}
}

func renderDecisionPrompt(req im.DecisionRequest) string {
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

func (f *Forwarder) tryHandleHelp(m im.Message) bool {
	f.mu.Lock()
	resolver := f.helpResolver
	f.mu.Unlock()
	if resolver == nil {
		return false
	}
	model, err := resolver(context.Background(), m.ChatID)
	if err != nil {
		_ = f.adapter.SendText(m.ChatID, fmt.Sprintf("help load error: %v", err))
		return true
	}
	if err := f.sendHelpPage(m.ChatID, model, 0); err != nil {
		_ = f.adapter.SendText(m.ChatID, strings.TrimSpace(model.Body))
	}
	return true
}

func renderDefault(v, d string) string {
	if strings.TrimSpace(v) == "" {
		return d
	}
	return strings.TrimSpace(v)
}

func (f *Forwarder) handleCardAction(evt im.CardActionEvent) {
	kind := strings.TrimSpace(evt.Value["kind"])
	switch kind {
	case "decision":
		decisionID := strings.TrimSpace(evt.Value["decision_id"])
		if decisionID == "" {
			return
		}
		f.mu.Lock()
		pd, ok := f.decisionByID[decisionID]
		if ok {
			delete(f.decisionByID, decisionID)
			delete(f.decisions, pd.chatID)
		}
		f.mu.Unlock()
		if !ok {
			return
		}
		res := im.DecisionResult{
			Outcome:  "selected",
			OptionID: strings.TrimSpace(evt.Value["option_id"]),
			Value:    strings.TrimSpace(evt.Value["value"]),
			ActorID:  evt.UserID,
			Source:   "card_action",
		}
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
		go f.handler(im.Message{
			ChatID: chatID,
			UserID: evt.UserID,
			Text:   text,
		})
	case "help_page":
		chatID := strings.TrimSpace(evt.Value["chat_id"])
		if chatID == "" {
			chatID = evt.ChatID
		}
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
			_ = f.adapter.SendText(chatID, fmt.Sprintf("help load error: %v", err))
			return
		}
		_ = f.sendHelpPage(chatID, model, page)
	}
}

func (f *Forwarder) sendHelpPage(chatID string, model im.HelpModel, page int) error {
	if len(model.Options) == 0 {
		return f.adapter.SendText(chatID, strings.TrimSpace(model.Body))
	}
	card := buildHelpCard(chatID, model, page)
	return f.adapter.SendCard(chatID, card)
}

func buildHelpCard(chatID string, model im.HelpModel, page int) im.Card {
	const pageSize = 8
	if page < 0 {
		page = 0
	}
	total := len(model.Options)
	maxPage := (total - 1) / pageSize
	if page > maxPage {
		page = maxPage
	}
	start := page * pageSize
	end := start + pageSize
	if end > total {
		end = total
	}

	actions := make([]map[string]any, 0, end-start)
	for _, opt := range model.Options[start:end] {
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
				"command": opt.Command,
				"value":   opt.Value,
			},
		})
	}

	elements := []map[string]any{
		{"tag": "markdown", "content": strings.TrimSpace(model.Body)},
		{"tag": "action", "actions": actions},
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
					"page":    strconv.Itoa(page + 1),
				},
			})
		}
		if len(nav) > 0 {
			elements = append(elements, map[string]any{"tag": "action", "actions": nav})
		}
	}

	return im.Card{
		"config": map[string]any{"update_multi": true},
		"header": map[string]any{
			"title": map[string]any{
				"tag":     "plain_text",
				"content": fmt.Sprintf("%s (%d/%d)", renderDefault(model.Title, "Help"), page+1, maxPage+1),
			},
		},
		"elements": elements,
	}
}

func (f *Forwarder) resolveDecision(m im.Message) bool {
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

func parseDecisionReply(input string, opts []im.DecisionOption) im.DecisionResult {
	v := strings.ToLower(strings.TrimSpace(input))
	if v == "" {
		return im.DecisionResult{Outcome: "invalid", Source: "text_reply"}
	}
	if v == "cancel" {
		return im.DecisionResult{Outcome: "cancelled", Source: "text_reply"}
	}
	if i := parseIndex(v); i >= 1 && i <= len(opts) {
		o := opts[i-1]
		return im.DecisionResult{Outcome: "selected", OptionID: o.ID, Value: o.Value, Source: "text_reply"}
	}
	for _, o := range opts {
		if strings.EqualFold(v, o.ID) || strings.EqualFold(v, o.Label) {
			return im.DecisionResult{Outcome: "selected", OptionID: o.ID, Value: o.Value, Source: "text_reply"}
		}
	}
	return im.DecisionResult{Outcome: "invalid", Source: "text_reply"}
}

func renderToolCallUpdate(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	var u struct {
		SessionUpdate string `json:"sessionUpdate"`
		ToolCallID    string `json:"toolCallId"`
		Title         string `json:"title"`
		Status        string `json:"status"`
	}
	if err := json.Unmarshal(raw, &u); err != nil {
		return ""
	}
	title := strings.TrimSpace(u.Title)
	if title == "" {
		title = "tool"
	}
	status := strings.TrimSpace(u.Status)
	if status == "" {
		status = "pending"
	}
	if u.ToolCallID != "" {
		return fmt.Sprintf("🔧 %s [%s] (%s)", title, status, u.ToolCallID)
	}
	return fmt.Sprintf("🔧 %s [%s]", title, status)
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

var _ im.Channel = (*Forwarder)(nil)
var _ im.DebugSender = (*Forwarder)(nil)
var _ im.UpdateEmitter = (*Forwarder)(nil)
var _ im.DecisionRequester = (*Forwarder)(nil)
var _ im.HelpResolverSetter = (*Forwarder)(nil)
