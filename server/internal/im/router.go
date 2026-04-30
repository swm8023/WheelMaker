package im

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	acp "github.com/swm8023/wheelmaker/internal/protocol"
)

type binding struct {
	sessionID string
	watch     bool
}

type Router struct {
	mu       sync.RWMutex
	client   InboundHandler
	history  SessionHistoryStore
	channels map[string]Channel
	bindings map[ChatRef]binding
}

func NewRouter(client InboundHandler, history SessionHistoryStore) *Router {
	if history == nil {
		history = NoopHistoryStore{}
	}
	return &Router{
		client:   client,
		history:  history,
		channels: map[string]Channel{},
		bindings: map[ChatRef]binding{},
	}
}

func (r *Router) RegisterChannel(ch Channel) error {
	if ch == nil {
		return fmt.Errorf("im: channel is nil")
	}
	id := normalize(ch.ID())
	if id == "" {
		return fmt.Errorf("im: channel id is empty")
	}
	r.mu.Lock()
	r.channels[id] = ch
	r.mu.Unlock()
	ch.OnPrompt(func(ctx context.Context, source ChatRef, params acp.SessionPromptParams) error {
		source.ChannelID = id
		return r.HandlePrompt(ctx, source, params)
	})
	ch.OnCommand(func(ctx context.Context, source ChatRef, cmd Command) error {
		source.ChannelID = id
		return r.HandleCommand(ctx, source, cmd)
	})
	return nil
}

func (r *Router) Bind(_ context.Context, chat ChatRef, sessionID string, opts BindOptions) error {
	chat = normalizeChat(chat)
	sessionID = strings.TrimSpace(sessionID)
	if chat.ChannelID == "" || chat.ChatID == "" || sessionID == "" {
		return fmt.Errorf("im: invalid binding")
	}
	r.mu.Lock()
	r.bindings[chat] = binding{sessionID: sessionID, watch: opts.Watch}
	r.mu.Unlock()
	return nil
}

func (r *Router) Unbind(_ context.Context, chat ChatRef) error {
	chat = normalizeChat(chat)
	if chat.ChannelID == "" || chat.ChatID == "" {
		return fmt.Errorf("im: invalid chat")
	}
	r.mu.Lock()
	delete(r.bindings, chat)
	r.mu.Unlock()
	return nil
}

func (r *Router) HandlePrompt(ctx context.Context, source ChatRef, params acp.SessionPromptParams) error {
	source = normalizeChat(source)
	if source.ChannelID == "" || source.ChatID == "" {
		return fmt.Errorf("im: prompt source is invalid")
	}
	sessionID := r.lookupSessionID(source)
	_ = r.history.Append(ctx, HistoryEvent{
		SessionID: sessionID,
		Direction: HistoryInbound,
		Source:    &source,
		Kind:      HistoryKindPrompt,
		Payload:   params,
		Text:      promptText(params),
	})
	if r.client == nil {
		return nil
	}
	return r.client.HandleIMPrompt(ctx, source, params)
}

func (r *Router) HandleCommand(ctx context.Context, source ChatRef, cmd Command) error {
	source = normalizeChat(source)
	if source.ChannelID == "" || source.ChatID == "" {
		return fmt.Errorf("im: command source is invalid")
	}
	sessionID := r.lookupSessionID(source)
	_ = r.history.Append(ctx, HistoryEvent{
		SessionID: sessionID,
		Direction: HistoryInbound,
		Source:    &source,
		Kind:      HistoryKindCommand,
		Payload:   cmd,
		Text:      cmd.Raw,
	})
	if r.client == nil {
		return nil
	}
	return r.client.HandleIMCommand(ctx, source, cmd)
}

func (r *Router) PublishSessionUpdate(ctx context.Context, target SendTarget, params acp.SessionUpdateParams) error {
	recipients, err := r.recipients(target)
	if err != nil {
		return err
	}
	var firstErr error
	for _, chat := range recipients {
		ch, err := r.channel(chat.ChannelID)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		deliver := deliveryTarget(target, chat)
		if err := ch.PublishSessionUpdate(ctx, deliver, params); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	_ = r.history.Append(ctx, HistoryEvent{
		SessionID: strings.TrimSpace(target.SessionID),
		Direction: HistoryOutbound,
		Source:    target.Source,
		Targets:   recipients,
		Kind:      HistoryKindSessionUpdate,
		Payload:   params,
	})
	return firstErr
}

func (r *Router) PublishPromptResult(ctx context.Context, target SendTarget, result acp.SessionPromptResult) error {
	recipients, err := r.recipients(target)
	if err != nil {
		return err
	}
	var firstErr error
	for _, chat := range recipients {
		ch, err := r.channel(chat.ChannelID)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		deliver := deliveryTarget(target, chat)
		if err := ch.PublishPromptResult(ctx, deliver, result); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	_ = r.history.Append(ctx, HistoryEvent{
		SessionID: strings.TrimSpace(target.SessionID),
		Direction: HistoryOutbound,
		Source:    target.Source,
		Targets:   recipients,
		Kind:      HistoryKindPromptResult,
		Payload:   result,
	})
	return firstErr
}

func (r *Router) PublishSessionMessage(ctx context.Context, target SendTarget, message acp.IMTurnMessage) error {
	recipients, err := r.recipients(target)
	if err != nil {
		return err
	}
	var firstErr error
	for _, chat := range recipients {
		ch, err := r.channel(chat.ChannelID)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		deliver := deliveryTarget(target, chat)
		if messageChannel, ok := ch.(SessionMessageChannel); ok {
			if err := messageChannel.PublishSessionMessage(ctx, deliver, message); err != nil && firstErr == nil {
				firstErr = err
			}
			continue
		}
		if err := publishSessionMessageLegacy(ctx, ch, deliver, message); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	_ = r.history.Append(ctx, HistoryEvent{
		SessionID: strings.TrimSpace(target.SessionID),
		Direction: HistoryOutbound,
		Source:    target.Source,
		Targets:   recipients,
		Kind:      historyKindForSessionMessage(message.Method),
		Payload:   message,
		Text:      historyTextForSessionMessage(message),
	})
	return firstErr
}
func (r *Router) SystemNotify(ctx context.Context, target SendTarget, payload SystemPayload) error {
	recipients, err := r.recipients(target)
	if err != nil {
		return err
	}
	var firstErr error
	for _, chat := range recipients {
		ch, err := r.channel(chat.ChannelID)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		deliver := deliveryTarget(target, chat)
		if err := ch.SystemNotify(ctx, deliver, payload); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	_ = r.history.Append(ctx, HistoryEvent{
		SessionID: strings.TrimSpace(target.SessionID),
		Direction: HistoryOutbound,
		Source:    target.Source,
		Targets:   recipients,
		Kind:      HistoryKindSystem,
		Payload:   payload,
		Text:      strings.TrimSpace(payload.Body),
	})
	return firstErr
}

func (r *Router) Run(ctx context.Context) error {
	r.mu.RLock()
	channels := make([]Channel, 0, len(r.channels))
	for _, ch := range r.channels {
		channels = append(channels, ch)
	}
	r.mu.RUnlock()
	if len(channels) == 0 {
		return fmt.Errorf("im: no channels registered")
	}

	var wg sync.WaitGroup
	errCh := make(chan error, len(channels))
	for _, ch := range channels {
		ch := ch
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := ch.Run(ctx); err != nil && !errors.Is(err, context.Canceled) && ctx.Err() == nil {
				errCh <- err
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			return err
		}
	}
	return nil
}

func publishSessionMessageLegacy(ctx context.Context, ch Channel, target SendTarget, message acp.IMTurnMessage) error {
	method := acp.NormalizeIMMethod(message.Method)
	switch method {
	case acp.IMMethodSystem:
		textPayload := acp.IMTextResult{}
		if !decodeIMMessagePayload(message.Param, &textPayload) {
			return nil
		}
		text := strings.TrimSpace(textPayload.Text)
		if text == "" {
			return nil
		}
		return ch.SystemNotify(ctx, target, SystemPayload{Kind: "message", Body: text})
	case acp.IMMethodPromptDone:
		promptResult := acp.IMPromptResult{}
		if !decodeIMMessagePayload(message.Param, &promptResult) {
			return nil
		}
		return ch.PublishPromptResult(ctx, target, acp.SessionPromptResult{StopReason: strings.TrimSpace(promptResult.StopReason)})
	case acp.IMMethodAgentMessage, acp.IMMethodAgentThought, acp.SessionUpdateUserMessageChunk:
		textPayload := acp.IMTextResult{}
		if !decodeIMMessagePayload(message.Param, &textPayload) {
			return nil
		}
		rawContent, err := json.Marshal(acp.ContentBlock{Type: acp.ContentBlockTypeText, Text: textPayload.Text})
		if err != nil {
			return err
		}
		return ch.PublishSessionUpdate(ctx, target, acp.SessionUpdateParams{
			SessionID: target.SessionID,
			Update: acp.SessionUpdate{
				SessionUpdate: method,
				Content:       rawContent,
			},
		})
	case acp.IMMethodToolCall:
		toolPayload := acp.IMToolResult{}
		if !decodeIMMessagePayload(message.Param, &toolPayload) {
			return nil
		}
		return ch.PublishSessionUpdate(ctx, target, acp.SessionUpdateParams{
			SessionID: target.SessionID,
			Update: acp.SessionUpdate{
				SessionUpdate: acp.SessionUpdateToolCall,
				Title:         strings.TrimSpace(toolPayload.Cmd),
				Kind:          strings.TrimSpace(toolPayload.Kind),
				Status:        strings.TrimSpace(toolPayload.Status),
			},
		})
	case acp.IMMethodAgentPlan:
		var planPayload []acp.IMPlanResult
		if !decodeIMMessagePayload(message.Param, &planPayload) {
			return nil
		}
		entries := make([]acp.PlanEntry, 0, len(planPayload))
		for _, entry := range planPayload {
			entries = append(entries, acp.PlanEntry{Content: strings.TrimSpace(entry.Content), Status: strings.TrimSpace(entry.Status)})
		}
		return ch.PublishSessionUpdate(ctx, target, acp.SessionUpdateParams{
			SessionID: target.SessionID,
			Update:    acp.SessionUpdate{SessionUpdate: acp.SessionUpdatePlan, Entries: entries},
		})
	default:
		return nil
	}
}

func decodeIMMessagePayload(raw json.RawMessage, out any) bool {
	raw = json.RawMessage(strings.TrimSpace(string(raw)))
	if len(raw) == 0 || string(raw) == "null" {
		return false
	}
	return json.Unmarshal(raw, out) == nil
}

func historyKindForSessionMessage(method string) string {
	switch acp.NormalizeIMMethod(method) {
	case acp.IMMethodSystem:
		return HistoryKindSystem
	case acp.IMMethodPromptDone:
		return HistoryKindPromptResult
	default:
		return HistoryKindSessionUpdate
	}
}

func historyTextForSessionMessage(message acp.IMTurnMessage) string {
	if acp.NormalizeIMMethod(message.Method) != acp.IMMethodSystem {
		return ""
	}
	textPayload := acp.IMTextResult{}
	if !decodeIMMessagePayload(message.Param, &textPayload) {
		return ""
	}
	return strings.TrimSpace(textPayload.Text)
}
func (r *Router) recipients(target SendTarget) ([]ChatRef, error) {
	sessionID := strings.TrimSpace(target.SessionID)
	if sessionID == "" {
		chat := normalizeChat(ChatRef{ChannelID: target.ChannelID, ChatID: target.ChatID})
		if chat.ChannelID == "" || chat.ChatID == "" {
			return nil, fmt.Errorf("im: direct send target is invalid")
		}
		return []ChatRef{chat}, nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if target.Source != nil {
		source := normalizeChat(*target.Source)
		if source.ChannelID == "" || source.ChatID == "" {
			return nil, fmt.Errorf("im: reply source is invalid")
		}
		out := []ChatRef{source}
		for chat, b := range r.bindings {
			if b.sessionID == sessionID && b.watch && chat != source {
				out = append(out, chat)
			}
		}
		return out, nil
	}
	var out []ChatRef
	for chat, b := range r.bindings {
		if b.sessionID == sessionID {
			out = append(out, chat)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("im: no chats bound to session %q", sessionID)
	}
	return out, nil
}

func (r *Router) channel(channelID string) (Channel, error) {
	id := normalize(channelID)
	r.mu.RLock()
	ch := r.channels[id]
	r.mu.RUnlock()
	if ch == nil {
		return nil, fmt.Errorf("im: channel %q is not registered", id)
	}
	return ch, nil
}

func normalizeChat(chat ChatRef) ChatRef {
	return ChatRef{ChannelID: normalize(chat.ChannelID), ChatID: strings.TrimSpace(chat.ChatID)}
}

func (r *Router) lookupSessionID(chat ChatRef) string {
	r.mu.RLock()
	b := r.bindings[chat]
	r.mu.RUnlock()
	return b.sessionID
}

func promptText(params acp.SessionPromptParams) string {
	for _, block := range params.Prompt {
		if block.Type == acp.ContentBlockTypeText {
			return block.Text
		}
	}
	return ""
}

func deliveryTarget(target SendTarget, chat ChatRef) SendTarget {
	out := target
	out.ChannelID = chat.ChannelID
	out.ChatID = chat.ChatID
	return out
}

func normalize(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}
