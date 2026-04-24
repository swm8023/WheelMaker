package im

import (
	"context"
	"testing"

	acp "github.com/swm8023/wheelmaker/internal/protocol"
)

type captureInboundClient struct {
	prompts  []capturedPrompt
	commands []capturedCommand
}

type capturedPrompt struct {
	source ChatRef
	params acp.SessionPromptParams
}

type capturedCommand struct {
	source ChatRef
	cmd    Command
}

func (c *captureInboundClient) HandleIMPrompt(_ context.Context, source ChatRef, params acp.SessionPromptParams) error {
	c.prompts = append(c.prompts, capturedPrompt{source: source, params: params})
	return nil
}

func (c *captureInboundClient) HandleIMCommand(_ context.Context, source ChatRef, cmd Command) error {
	c.commands = append(c.commands, capturedCommand{source: source, cmd: cmd})
	return nil
}

type captureChannel struct {
	id string

	runCalled bool

	onPrompt  func(context.Context, ChatRef, acp.SessionPromptParams) error
	onCommand func(context.Context, ChatRef, Command) error

	updates       []capturedSessionUpdate
	promptResults []capturedPromptResult
	systems       []capturedSystem
}

type capturedSessionUpdate struct {
	target SendTarget
	params acp.SessionUpdateParams
}

type capturedPromptResult struct {
	target SendTarget
	result acp.SessionPromptResult
}

type capturedSystem struct {
	target  SendTarget
	payload SystemPayload
}

func (c *captureChannel) ID() string { return c.id }

func (c *captureChannel) OnPrompt(fn func(context.Context, ChatRef, acp.SessionPromptParams) error) {
	c.onPrompt = fn
}

func (c *captureChannel) OnCommand(fn func(context.Context, ChatRef, Command) error) {
	c.onCommand = fn
}

func (c *captureChannel) PublishSessionUpdate(_ context.Context, target SendTarget, params acp.SessionUpdateParams) error {
	c.updates = append(c.updates, capturedSessionUpdate{target: target, params: params})
	return nil
}

func (c *captureChannel) PublishPromptResult(_ context.Context, target SendTarget, result acp.SessionPromptResult) error {
	c.promptResults = append(c.promptResults, capturedPromptResult{target: target, result: result})
	return nil
}

func (c *captureChannel) SystemNotify(_ context.Context, target SendTarget, payload SystemPayload) error {
	c.systems = append(c.systems, capturedSystem{target: target, payload: payload})
	return nil
}

func (c *captureChannel) Run(context.Context) error {
	c.runCalled = true
	return nil
}

func TestHandlePrompt_UnboundChatReachesClientWithoutSession(t *testing.T) {
	ctx := context.Background()
	client := &captureInboundClient{}
	router := NewRouter(client, nil)
	ch := &captureChannel{id: "feishu"}
	if err := router.RegisterChannel(ch); err != nil {
		t.Fatalf("RegisterChannel: %v", err)
	}

	params := acp.SessionPromptParams{
		Prompt: []acp.ContentBlock{{Type: acp.ContentBlockTypeText, Text: "hello"}},
	}
	if err := ch.onPrompt(ctx, ChatRef{ChatID: "chat-a"}, params); err != nil {
		t.Fatalf("onPrompt: %v", err)
	}

	if len(client.prompts) != 1 {
		t.Fatalf("prompts=%d, want 1", len(client.prompts))
	}
	got := client.prompts[0]
	if got.source.ChannelID != "feishu" || got.source.ChatID != "chat-a" {
		t.Fatalf("source=%+v", got.source)
	}
	if text := promptText(got.params); text != "hello" {
		t.Fatalf("prompt text=%q, want hello", text)
	}
}

func TestBind_CausesLaterPromptHistoryToCarrySessionID(t *testing.T) {
	ctx := context.Background()
	client := &captureInboundClient{}
	store := NewMemoryHistoryStore()
	router := NewRouter(client, store)
	ch := &captureChannel{id: "app"}
	_ = router.RegisterChannel(ch)
	if err := router.Bind(ctx, ChatRef{ChannelID: "app", ChatID: "chat-a"}, "session-1", BindOptions{}); err != nil {
		t.Fatalf("Bind: %v", err)
	}

	if err := ch.onPrompt(ctx, ChatRef{ChatID: "chat-a"}, acp.SessionPromptParams{
		Prompt: []acp.ContentBlock{{Type: acp.ContentBlockTypeText, Text: "hello"}},
	}); err != nil {
		t.Fatalf("onPrompt: %v", err)
	}

	got, err := store.List(ctx, "session-1", HistoryQuery{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 || got[0].Kind != HistoryKindPrompt {
		t.Fatalf("history=%+v", got)
	}
}

func TestHandleCommand_PreservesRawText(t *testing.T) {
	ctx := context.Background()
	client := &captureInboundClient{}
	router := NewRouter(client, nil)
	ch := &captureChannel{id: "feishu"}
	_ = router.RegisterChannel(ch)

	cmd := Command{Name: "/status", Raw: "/status"}
	if err := ch.onCommand(ctx, ChatRef{ChatID: "chat-a"}, cmd); err != nil {
		t.Fatalf("onCommand: %v", err)
	}
	if got := client.commands[0].cmd.Raw; got != "/status" {
		t.Fatalf("Raw=%q, want /status", got)
	}
}

func TestPublishSessionUpdate_ReplyFansOutToWatchChatsOnly(t *testing.T) {
	ctx := context.Background()
	router := NewRouter(nil, nil)
	ch := &captureChannel{id: "app"}
	_ = router.RegisterChannel(ch)
	_ = router.Bind(ctx, ChatRef{ChannelID: "app", ChatID: "a"}, "s1", BindOptions{})
	_ = router.Bind(ctx, ChatRef{ChannelID: "app", ChatID: "b"}, "s1", BindOptions{Watch: true})
	_ = router.Bind(ctx, ChatRef{ChannelID: "app", ChatID: "c"}, "s1", BindOptions{})
	source := ChatRef{ChannelID: "app", ChatID: "a"}

	err := router.PublishSessionUpdate(ctx, SendTarget{SessionID: "s1", Source: &source}, acp.SessionUpdateParams{
		SessionID: "acp-1",
		Update:    acp.SessionUpdate{SessionUpdate: acp.SessionUpdatePlan},
	})
	if err != nil {
		t.Fatalf("PublishSessionUpdate: %v", err)
	}

	if len(ch.updates) != 2 {
		t.Fatalf("updates count=%d, want 2: %+v", len(ch.updates), ch.updates)
	}
	if ch.updates[0].target.ChatID != "a" || ch.updates[1].target.ChatID != "b" {
		t.Fatalf("updates=%+v", ch.updates)
	}
}

func TestPublishPromptResult_SessionBroadcastSendsAllBoundChats(t *testing.T) {
	ctx := context.Background()
	router := NewRouter(nil, nil)
	ch := &captureChannel{id: "app"}
	_ = router.RegisterChannel(ch)
	_ = router.Bind(ctx, ChatRef{ChannelID: "app", ChatID: "a"}, "s1", BindOptions{})
	_ = router.Bind(ctx, ChatRef{ChannelID: "app", ChatID: "b"}, "s1", BindOptions{Watch: true})

	err := router.PublishPromptResult(ctx, SendTarget{SessionID: "s1"}, acp.SessionPromptResult{StopReason: acp.StopReasonEndTurn})
	if err != nil {
		t.Fatalf("PublishPromptResult: %v", err)
	}
	if len(ch.promptResults) != 2 {
		t.Fatalf("promptResults count=%d, want 2: %+v", len(ch.promptResults), ch.promptResults)
	}
}

func TestSystemNotify_DirectChatSendsOnlyTarget(t *testing.T) {
	ctx := context.Background()
	router := NewRouter(nil, nil)
	ch := &captureChannel{id: "feishu"}
	_ = router.RegisterChannel(ch)

	err := router.SystemNotify(ctx, SendTarget{ChannelID: "feishu", ChatID: "chat-a"}, SystemPayload{Kind: "status", Body: "choose a session"})
	if err != nil {
		t.Fatalf("SystemNotify: %v", err)
	}
	if len(ch.systems) != 1 || ch.systems[0].target.ChatID != "chat-a" {
		t.Fatalf("systems=%+v", ch.systems)
	}
}

func TestRun_StartsRegisteredChannels(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	router := NewRouter(nil, nil)
	ch := &captureChannel{id: "feishu"}
	_ = router.RegisterChannel(ch)

	if err := router.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !ch.runCalled {
		t.Fatal("channel Run was not called")
	}
}
