package feishu

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

type transport interface {
	OnMessage(MessageHandler)
	OnCardAction(func(CardActionEvent))
	Send(chatID, text string, kind TextKind) error
	SendCard(chatID, messageID string, card Card) error
	SendReaction(messageID, emoji string) error
	MarkDone(chatID string) error
	Run(ctx context.Context) error
}

type pendingDecision struct {
	id     string
	chatID string
	req    im.DecisionRequest
	ch     chan im.DecisionResult
}

type Channel struct {
	inner  transport
	mu     sync.Mutex
	msg    func(context.Context, string, string) error
	byID   map[string]pendingDecision
	byChat map[string]pendingDecision
	closed map[string]time.Time
	nextID atomic.Int64
}

func New(cfg Config) *Channel {
	return newWithTransport(newTransport(cfg))
}

func newWithTransport(inner transport) *Channel {
	c := &Channel{
		inner:  inner,
		byID:   map[string]pendingDecision{},
		byChat: map[string]pendingDecision{},
		closed: map[string]time.Time{},
	}
	inner.OnMessage(c.handleMessage)
	inner.OnCardAction(c.handleCardAction)
	return c
}

func (c *Channel) ID() string { return "feishu" }

func (c *Channel) OnMessage(handler func(context.Context, string, string) error) {
	c.mu.Lock()
	c.msg = handler
	c.mu.Unlock()
}

func (c *Channel) Send(ctx context.Context, chatID string, event im.OutboundEvent) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	switch event.Kind {
	case im.OutboundACP:
		return c.sendACP(chatID, event.Payload)
	case im.OutboundSystem:
		return c.inner.Send(chatID, payloadText(event.Payload), TextSystem)
	default:
		return c.inner.Send(chatID, payloadText(event.Payload), TextNormal)
	}
}

func (c *Channel) RequestDecision(ctx context.Context, chatID string, req im.DecisionRequest) (im.DecisionResult, error) {
	chatID = strings.TrimSpace(chatID)
	if chatID == "" {
		return im.DecisionResult{Outcome: "invalid"}, fmt.Errorf("im feishu: decision chat is empty")
	}
	timeout := 2 * time.Minute
	if v := strings.TrimSpace(req.Hint["timeoutSec"]); v != "" {
		if sec, err := strconv.Atoi(v); err == nil && sec > 0 {
			timeout = time.Duration(sec) * time.Second
		}
	}
	decisionID := fmt.Sprintf("im-dec-%d", c.nextID.Add(1))
	ch := make(chan im.DecisionResult, 1)
	pd := pendingDecision{id: decisionID, chatID: chatID, req: req, ch: ch}
	c.mu.Lock()
	c.byID[decisionID] = pd
	c.byChat[chatID] = pd
	c.mu.Unlock()

	meta := map[string]string{"kind": "decision", "decision_id": decisionID, "chat_id": chatID}
	for k, v := range req.Meta {
		if strings.TrimSpace(k) != "" {
			meta[k] = strings.TrimSpace(v)
		}
	}
	card := OptionsCard{
		Title:   req.Title,
		Body:    req.Body,
		Options: req.Options,
		Meta:    meta,
	}
	if err := c.inner.SendCard(chatID, "", card); err != nil {
		c.clearDecision(chatID, decisionID, ch)
		return im.DecisionResult{Outcome: "invalid"}, err
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case res := <-ch:
		return res, nil
	case <-ctx.Done():
		c.clearDecision(chatID, decisionID, ch)
		return im.DecisionResult{Outcome: "cancelled", Source: "default_policy"}, nil
	case <-timer.C:
		c.clearDecision(chatID, decisionID, ch)
		return im.DecisionResult{Outcome: "timeout", Source: "default_policy"}, nil
	}
}

func (c *Channel) Run(ctx context.Context) error {
	return c.inner.Run(ctx)
}

func (c *Channel) sendACP(chatID string, payload any) error {
	p, ok := payload.(im.ACPPayload)
	if !ok {
		return c.inner.Send(chatID, payloadText(payload), TextNormal)
	}
	switch p.UpdateType {
	case "text":
		return c.inner.Send(chatID, p.Text, TextNormal)
	case "thought":
		return c.inner.Send(chatID, p.Text, TextThought)
	case "done":
		return c.inner.MarkDone(chatID)
	case "error":
		text := strings.TrimSpace(p.Text)
		if text == "" {
			text = "Agent request failed."
		}
		return c.inner.Send(chatID, text, TextNormal)
	case "tool_call", "tool_call_update":
		if upd, ok := parseToolCallUpdate(p.Raw); ok {
			return c.inner.SendCard(chatID, "", ToolCallCard{Update: upd})
		}
	case "plan":
		if msg := renderRawSummary("Plan update", p.Raw); msg != "" {
			return c.inner.Send(chatID, msg, TextNormal)
		}
	case "config_option_update":
		if msg := renderRawSummary("Config updated", p.Raw); msg != "" {
			return c.inner.Send(chatID, msg, TextNormal)
		}
	}
	return nil
}

func (c *Channel) handleMessage(m Message) {
	if c.resolveDecisionText(m) {
		return
	}
	c.mu.Lock()
	handler := c.msg
	c.mu.Unlock()
	if handler != nil {
		_ = handler(context.Background(), m.ChatID, m.Text)
	}
}

func (c *Channel) handleCardAction(evt CardActionEvent) {
	if strings.TrimSpace(evt.Value["kind"]) != "decision" {
		return
	}
	decisionID := strings.TrimSpace(evt.Value["decision_id"])
	if decisionID == "" {
		return
	}
	c.mu.Lock()
	pd, ok := c.byID[decisionID]
	if !ok {
		if c.wasClosedLocked(decisionID) {
			c.mu.Unlock()
			return
		}
		c.mu.Unlock()
		return
	}
	delete(c.byID, decisionID)
	delete(c.byChat, pd.chatID)
	c.markClosedLocked(decisionID)
	c.mu.Unlock()

	res := im.DecisionResult{
		Outcome:  "selected",
		OptionID: firstNonEmpty(evt.Value["option_id"], evt.Value["option"], evt.Option),
		Value:    firstNonEmpty(evt.Value["value"], evt.Value["option"], evt.Option),
		ActorID:  evt.UserID,
		Source:   "card_action",
	}
	select {
	case pd.ch <- res:
	default:
	}
}

func (c *Channel) resolveDecisionText(m Message) bool {
	chatID := strings.TrimSpace(m.ChatID)
	c.mu.Lock()
	pd, ok := c.byChat[chatID]
	if !ok {
		c.mu.Unlock()
		return false
	}
	delete(c.byChat, chatID)
	delete(c.byID, pd.id)
	c.markClosedLocked(pd.id)
	c.mu.Unlock()

	res := parseDecisionReply(strings.TrimSpace(m.Text), pd.req.Options)
	select {
	case pd.ch <- res:
	default:
	}
	return true
}

func (c *Channel) clearDecision(chatID, decisionID string, ch chan im.DecisionResult) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if pd, ok := c.byChat[chatID]; ok && pd.ch == ch {
		delete(c.byChat, chatID)
	}
	if pd, ok := c.byID[decisionID]; ok && pd.ch == ch {
		delete(c.byID, decisionID)
	}
	c.markClosedLocked(decisionID)
}

func (c *Channel) markClosedLocked(decisionID string) {
	c.closed[decisionID] = time.Now()
}

func (c *Channel) wasClosedLocked(decisionID string) bool {
	t, ok := c.closed[decisionID]
	return ok && time.Since(t) < 5*time.Minute
}

func parseDecisionReply(input string, opts []im.DecisionOption) im.DecisionResult {
	if input == "" {
		return im.DecisionResult{Outcome: "invalid", Source: "text_reply"}
	}
	if strings.EqualFold(input, "cancel") {
		return im.DecisionResult{Outcome: "cancelled", Source: "text_reply"}
	}
	if idx, err := strconv.Atoi(input); err == nil && idx >= 1 && idx <= len(opts) {
		o := opts[idx-1]
		return im.DecisionResult{Outcome: "selected", OptionID: o.ID, Value: o.Value, Source: "text_reply"}
	}
	for _, o := range opts {
		if strings.EqualFold(input, o.ID) || strings.EqualFold(input, o.Label) {
			return im.DecisionResult{Outcome: "selected", OptionID: o.ID, Value: o.Value, Source: "text_reply"}
		}
	}
	return im.DecisionResult{Outcome: "invalid", Source: "text_reply"}
}

func parseToolCallUpdate(raw []byte) (ToolCallUpdate, bool) {
	if len(raw) == 0 {
		return ToolCallUpdate{}, false
	}
	var upd ToolCallUpdate
	if err := json.Unmarshal(raw, &upd); err != nil {
		return ToolCallUpdate{}, false
	}
	upd.ToolCallID = strings.TrimSpace(upd.ToolCallID)
	if upd.ToolCallID == "" {
		return ToolCallUpdate{}, false
	}
	return upd, true
}

func payloadText(payload any) string {
	switch p := payload.(type) {
	case im.TextPayload:
		return p.Text
	case string:
		return p
	default:
		return fmt.Sprint(payload)
	}
}

func renderRawSummary(prefix string, raw []byte) string {
	if len(raw) == 0 {
		return prefix
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return prefix
	}
	if title, _ := m["title"].(string); strings.TrimSpace(title) != "" {
		return fmt.Sprintf("%s: %s", prefix, strings.TrimSpace(title))
	}
	return prefix
}
