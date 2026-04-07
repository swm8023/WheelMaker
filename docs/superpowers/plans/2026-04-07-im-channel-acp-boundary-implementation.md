# IM Channel ACP Boundary Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Refactor the current `server/internal/im` runtime to use ACP-shaped outbound channel APIs, command/prompt/permission inbound callbacks, and a slimmer Feishu implementation.

**Architecture:** Replace the current `OutboundEvent` and decision wrapper model with explicit IM channel methods for ACP `session/update`, ACP prompt result, ACP permission requests, and WheelMaker system notifications. Keep router focused on routing and binding, move Feishu rendering details into `render.go`, move Feishu transport into one `transport.go`, and update client/session code to stop synthesizing fake `done` and `error` updates.

**Tech Stack:** Go 1.26, ACP JSON-RPC protocol types, IM router/channel abstractions, Feishu transport and card rendering, Go test

---

## File Structure

### Files To Modify

- `server/internal/protocol/acp_const.go` — consolidate ACP method names, update types, content block types, statuses, stop reasons, and shared config IDs
- `server/internal/protocol/acp.go` — reuse centralized ACP constants in comments and helpers where needed
- `server/internal/protocol/update.go` — remove internal fake `done`/`error` update types from IM-facing flow
- `server/internal/im/router.go` — replace legacy send/decision entrypoints with typed prompt/command/permission-response and typed outbound publish methods
- `server/internal/im/router_test.go` — update router tests for the new channel contract
- `server/internal/im/history.go` — update stored event shape to reflect typed outbound methods
- `server/internal/im/history_test.go` — align history expectations with the new event model
- `server/internal/im/app/app.go` — implement the new channel interface as a stub
- `server/internal/im/feishu/feishu_test.go` — migrate Feishu tests to the new contract
- `server/internal/im/feishu/types_local.go` — keep or adjust local render structs used by cards
- `server/internal/hub/client/im_bridge.go` — replace `HandleIMInbound` and legacy send/decision calls with new router API usage
- `server/internal/hub/client/session.go` — forward ACP session updates, prompt result, and system notices without converting prompt completion into fake `UpdateDone` / `UpdateError`
- `server/internal/hub/client/permission.go` — route permission requests through the new IM router method using JSON-RPC request ID plus `toolCallId`
- `server/internal/hub/client/client_internal_test.go` — update fake routers, injected instances, and IM-facing assertions
- `server/internal/hub/client/client_test.go` — remove assumptions about synthesized `UpdateDone`/`UpdateError`

### Files To Create

- `server/internal/im/channel.go` — renamed canonical IM channel contract and shared types
- `server/internal/im/feishu/channel.go` — Feishu event-handling layer
- `server/internal/im/feishu/render.go` — Feishu rendering helpers for text, cards, summaries, prompt results, and system notifications
- `server/internal/im/feishu/transport.go` — renamed Feishu transport implementation file

### Files To Delete

- `server/internal/im/protocol.go`
- `server/internal/im/feishu/feishu.go`
- `server/internal/im/feishu/transport_impl.go`

## Task 1: Lock In Shared ACP Constants And IM Channel Contract

**Files:**
- Modify: `server/internal/protocol/acp_const.go`
- Modify: `server/internal/protocol/update.go`
- Create: `server/internal/im/channel.go`
- Delete: `server/internal/im/protocol.go`
- Test: `server/internal/im/router_test.go`

- [ ] **Step 1: Write failing router tests for the new typed channel contract**

```go
func TestRouterPublishPromptResult_RoutesToBoundSource(t *testing.T) {
    ctx := context.Background()
    client := &captureInboundClient{}
    router := NewRouter(client, nil)
    ch := &captureChannel{id: "feishu"}
    if err := router.RegisterChannel(ch); err != nil {
        t.Fatalf("RegisterChannel: %v", err)
    }
    source := ChatRef{ChannelID: "feishu", ChatID: "chat-a"}
    if err := router.Bind(ctx, source, "s1", BindOptions{}); err != nil {
        t.Fatalf("Bind: %v", err)
    }

    err := router.PublishPromptResult(ctx, SendTarget{SessionID: "s1", Source: &source}, acp.SessionPromptResult{StopReason: acp.StopReasonEndTurn})
    if err != nil {
        t.Fatalf("PublishPromptResult: %v", err)
    }
    if len(ch.promptResults) != 1 || ch.promptResults[0].chatID != "chat-a" {
        t.Fatalf("promptResults=%+v", ch.promptResults)
    }
}

func TestRouterPublishPermissionRequest_RoutesOnlyToSourceChat(t *testing.T) {
    ctx := context.Background()
    router := NewRouter(nil, nil)
    ch := &captureChannel{id: "feishu"}
    _ = router.RegisterChannel(ch)
    source := ChatRef{ChannelID: "feishu", ChatID: "chat-a"}

    err := router.PublishPermissionRequest(ctx, SendTarget{SessionID: "s1", Source: &source}, 42, acp.PermissionRequestParams{
        SessionID: "acp-1",
        ToolCall:  acp.ToolCallRef{ToolCallID: "call-1", Title: "Write file"},
    })
    if err != nil {
        t.Fatalf("PublishPermissionRequest: %v", err)
    }
    if len(ch.permissions) != 1 || ch.permissions[0].requestID != 42 {
        t.Fatalf("permissions=%+v", ch.permissions)
    }
}
```

- [ ] **Step 2: Run the IM package tests to verify failure**

Run: `cd server && go test ./internal/im/... -run "PublishPromptResult|PublishPermissionRequest" -count=1`

Expected: FAIL because the current channel contract still exposes `OnMessage`, `Send`, and `RequestDecision`.

- [ ] **Step 3: Implement the new shared channel contract and ACP constants**

```go
type Channel interface {
    ID() string

    OnPrompt(func(ctx context.Context, source ChatRef, params acp.SessionPromptParams) error)
    OnCommand(func(ctx context.Context, source ChatRef, cmd Command) error)
    OnPermissionResponse(func(ctx context.Context, source ChatRef, requestID int64, result acp.PermissionResponse) error)

    PublishSessionUpdate(ctx context.Context, target SendTarget, params acp.SessionUpdateParams) error
    PublishPromptResult(ctx context.Context, target SendTarget, result acp.SessionPromptResult) error
    PublishPermissionRequest(ctx context.Context, target SendTarget, requestID int64, params acp.PermissionRequestParams) error
    SystemNotify(ctx context.Context, target SendTarget, payload SystemPayload) error

    Run(ctx context.Context) error
}

const (
    StopReasonEndTurn        = "end_turn"
    StopReasonMaxTokens      = "max_tokens"
    StopReasonMaxTurnRequest = "max_turn_requests"
    StopReasonRefusal        = "refusal"
    StopReasonCancelled      = "cancelled"

    ToolCallStatusPending    = "pending"
    ToolCallStatusInProgress = "in_progress"
)
```

- [ ] **Step 4: Re-run the IM package tests**

Run: `cd server && go test ./internal/im/... -count=1`

Expected: PASS for the updated router and shared-type tests.

- [ ] **Step 5: Commit**

```bash
git add server/internal/protocol/acp_const.go server/internal/protocol/update.go server/internal/im/channel.go server/internal/im/router.go server/internal/im/router_test.go server/internal/im/history.go server/internal/im/history_test.go
git commit -m "refactor: redefine im channel contract around acp"
```

## Task 2: Refactor Client Session Flow To Emit ACP-Native Outbound Events

**Files:**
- Modify: `server/internal/hub/client/im_bridge.go`
- Modify: `server/internal/hub/client/session.go`
- Modify: `server/internal/hub/client/permission.go`
- Test: `server/internal/hub/client/client_internal_test.go`
- Test: `server/internal/hub/client/client_test.go`

- [ ] **Step 1: Write failing client tests for prompt-result and system-notify behavior**

```go
func TestHandleIMPrompt_EmitsPromptResultInsteadOfSyntheticDoneUpdate(t *testing.T) {
    c := New(&noopStore{}, nil, "test", "/tmp")
    fake := &fakeIMRouter{}
    c.SetIMRouter(fake)
    c.InjectAgentFactory(acp.ACPProviderClaude, func(context.Context) (agent.Instance, error) {
        return &testInjectedInstance{
            name:      "claude",
            sessionID: "acp-1",
            promptFn: func(context.Context, string) (<-chan acp.Update, error) {
                ch := make(chan acp.Update, 1)
                ch <- acp.Update{Type: acp.UpdateText, Content: "hello back"}
                close(ch)
                return ch, nil
            },
        }, nil
    })

    err := c.HandleIMPrompt(context.Background(), im.ChatRef{ChannelID: "feishu", ChatID: "chat-a"}, acp.SessionPromptParams{
        Prompt: []acp.ContentBlock{{Type: acp.ContentBlockTypeText, Text: "hello"}},
    })
    if err != nil {
        t.Fatalf("HandleIMPrompt: %v", err)
    }
    if len(fake.promptResults) != 1 || fake.promptResults[0].result.StopReason != acp.StopReasonEndTurn {
        t.Fatalf("promptResults=%+v", fake.promptResults)
    }
}

func TestPromptError_UsesSystemNotify(t *testing.T) {
    c := New(&noopStore{}, nil, "test", "/tmp")
    fake := &fakeIMRouter{}
    c.SetIMRouter(fake)
    c.InjectForwarder("claude", "acp-1", func(context.Context, string) (<-chan acp.Update, error) {
        return nil, errors.New("boom")
    }, nil)

    sess := c.activeSession
    sess.setIMSource(im.ChatRef{ChannelID: "feishu", ChatID: "chat-a"})
    sess.handlePrompt("hello")

    if len(fake.systems) == 0 {
        t.Fatal("expected system notification")
    }
}
```

- [ ] **Step 2: Run the targeted client tests to verify they fail**

Run: `cd server && go test ./internal/hub/client -run "PromptResultInsteadOfSyntheticDoneUpdate|PromptError_UsesSystemNotify" -count=1`

Expected: FAIL because the current flow still emits `im.OutboundACP` with synthetic `done` and `error` update types.

- [ ] **Step 3: Update client/session/permission code to use the new IM router contract**

```go
type IMRouter interface {
    Bind(ctx context.Context, chat im.ChatRef, sessionID string, opts im.BindOptions) error
    PublishSessionUpdate(ctx context.Context, target im.SendTarget, params acp.SessionUpdateParams) error
    PublishPromptResult(ctx context.Context, target im.SendTarget, result acp.SessionPromptResult) error
    PublishPermissionRequest(ctx context.Context, target im.SendTarget, requestID int64, params acp.PermissionRequestParams) error
    SystemNotify(ctx context.Context, target im.SendTarget, payload im.SystemPayload) error
    Run(ctx context.Context) error
}

func (s *Session) emitPromptResult(ctx context.Context, source im.ChatRef, result acp.SessionPromptResult) {
    if s.imRouter == nil {
        return
    }
    _ = s.imRouter.PublishPromptResult(ctx, im.SendTarget{SessionID: s.ID, Source: &source}, result)
}
```

- [ ] **Step 4: Re-run the full client package tests**

Run: `cd server && go test ./internal/hub/client/... -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add server/internal/hub/client/im_bridge.go server/internal/hub/client/session.go server/internal/hub/client/permission.go server/internal/hub/client/client_internal_test.go server/internal/hub/client/client_test.go
git commit -m "refactor: emit acp-native im events from client"
```

## Task 3: Slim Down Feishu Into Channel, Render, And Transport Layers

**Files:**
- Create: `server/internal/im/feishu/channel.go`
- Create: `server/internal/im/feishu/render.go`
- Create: `server/internal/im/feishu/transport.go`
- Delete: `server/internal/im/feishu/feishu.go`
- Delete: `server/internal/im/feishu/transport_impl.go`
- Modify: `server/internal/im/feishu/feishu_test.go`
- Modify: `server/internal/im/app/app.go`

- [ ] **Step 1: Write failing Feishu tests for the new ACP-shaped outbound methods**

```go
func TestPublishPromptResult_EndTurnMarksDone(t *testing.T) {
    ft := &fakeTransport{}
    ch := newWithTransport(ft)

    err := ch.PublishPromptResult(context.Background(), im.SendTarget{ChannelID: "feishu", ChatID: "chat-a"}, acp.SessionPromptResult{StopReason: acp.StopReasonEndTurn})
    if err != nil {
        t.Fatalf("PublishPromptResult: %v", err)
    }
    if len(ft.done) != 1 || ft.done[0] != "chat-a" {
        t.Fatalf("done=%+v", ft.done)
    }
}

func TestPublishPermissionRequest_StoresRequestIDAndToolCallID(t *testing.T) {
    ft := &fakeTransport{}
    ch := newWithTransport(ft)

    err := ch.PublishPermissionRequest(context.Background(), im.SendTarget{ChannelID: "feishu", ChatID: "chat-a"}, 42, acp.PermissionRequestParams{
        SessionID: "acp-1",
        ToolCall:  acp.ToolCallRef{ToolCallID: "call-1", Title: "Write file"},
        Options:   []acp.PermissionOption{{OptionID: "allow", Name: "Allow", Kind: "allow_once"}},
    })
    if err != nil {
        t.Fatalf("PublishPermissionRequest: %v", err)
    }
    if got := ch.pendingByRequestID[42].ToolCallID; got != "call-1" {
        t.Fatalf("toolCallID=%q, want call-1", got)
    }
}
```

- [ ] **Step 2: Run Feishu tests to verify they fail**

Run: `cd server && go test ./internal/im/feishu -count=1`

Expected: FAIL because the current Feishu channel still implements `Send` and `RequestDecision`.

- [ ] **Step 3: Split Feishu into a small coordinator plus reusable render/transport helpers**

```go
type Channel struct {
    inner transport

    mu                 sync.Mutex
    onPrompt           func(context.Context, im.ChatRef, acp.SessionPromptParams) error
    onCommand          func(context.Context, im.ChatRef, im.Command) error
    onPermissionResult func(context.Context, im.ChatRef, int64, acp.PermissionResponse) error

    pendingByRequestID map[int64]PendingPermission
    pendingByChat      map[string]PendingPermission
}

func (c *Channel) PublishSessionUpdate(ctx context.Context, target im.SendTarget, params acp.SessionUpdateParams) error {
    return c.renderSessionUpdate(ctx, resolveChatID(target), params)
}

func (c *Channel) PublishPromptResult(ctx context.Context, target im.SendTarget, result acp.SessionPromptResult) error {
    return c.renderPromptResult(ctx, resolveChatID(target), result)
}

func (c *Channel) PublishPermissionRequest(ctx context.Context, target im.SendTarget, requestID int64, params acp.PermissionRequestParams) error {
    return c.renderPermissionRequest(ctx, resolveChatID(target), requestID, params)
}
```

- [ ] **Step 4: Re-run Feishu, app, and IM package tests**

Run: `cd server && go test ./internal/im/... -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add server/internal/im/app/app.go server/internal/im/feishu/channel.go server/internal/im/feishu/render.go server/internal/im/feishu/transport.go server/internal/im/feishu/feishu_test.go server/internal/im/feishu/types_local.go
git rm server/internal/im/feishu/feishu.go server/internal/im/feishu/transport_impl.go server/internal/im/protocol.go
git commit -m "refactor: split feishu channel rendering and transport"
```

## Task 4: Final Verification

**Files:**
- Modify: any remaining files touched by Tasks 1-3

- [ ] **Step 1: Run targeted IM and client verification**

Run: `cd server && go test ./internal/protocol ./internal/im/... ./internal/hub/client/... -count=1`

Expected: PASS.

- [ ] **Step 2: Run full server verification**

Run: `cd server && go test ./... -count=1`

Expected: PASS.

- [ ] **Step 3: Review final diff**

Run: `git status --short && git diff --stat`

Expected: only IM channel refactor, Feishu split, client routing changes, and test updates are present.

- [ ] **Step 4: Commit final cleanup if needed**

```bash
git add -A
git commit -m "chore: finalize im channel acp refactor"
```

- [ ] **Step 5: Push and trigger deploy hook**

```bash
git push origin main
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/signal_update_now.ps1 -DelaySeconds 30
```
