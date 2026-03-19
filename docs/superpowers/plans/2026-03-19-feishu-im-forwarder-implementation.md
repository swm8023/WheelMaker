# Feishu IM Forwarder Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** ?? Feishu IM ??,??? IM Forwarder,???/????? client ??? IM ?? 

**Architecture:** `client` ?????? `IMUpdate` ??? `Decision`;`im/forwarder` ?????????????????;`im/feishu` ??????? API ???`/help` ????? client ????????????????? 

**Tech Stack:** Go, larksuite/oapi-sdk-go/v3 (WS), go-lark (message APIs), ACP typed updates

---

## Chunk 1: IM ??? Forwarder ??

### Task 1: ?? IM ????(? client ??)

**Files:**
- Modify: `internal/im/im.go`
- Create: `internal/im/types.go`
- Test: `internal/im/types_test.go`

- [ ] **Step 1: ??????(??? + ???)**

```go
func TestIMUpdate_JSONRoundtrip(t *testing.T) {
    in := IMUpdate{ChatID: "c1", UpdateType: "tool_call", Text: "x"}
    b, _ := json.Marshal(in)
    var out IMUpdate
    _ = json.Unmarshal(b, &out)
    require.Equal(t, in.ChatID, out.ChatID)
    require.Equal(t, in.UpdateType, out.UpdateType)
}
```

- [ ] **Step 2: ????????**

Run: `go test ./internal/im -run TestIMUpdate_JSONRoundtrip -v`  
Expected: FAIL(?????)

- [ ] **Step 3: ?????????**

```go
type Gateway interface {
    OnMessage(MessageHandler)
    Emit(ctx context.Context, u IMUpdate) error
    RequestDecision(ctx context.Context, req DecisionRequest) (DecisionResult, error)
    Run(ctx context.Context) error
    Close(ctx context.Context) error
}
```

- [ ] **Step 4: ????????**

Run: `go test ./internal/im -run TestIMUpdate_JSONRoundtrip -v`  
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/im/im.go internal/im/types.go internal/im/types_test.go
git commit -m "feat(im): define gateway update and decision types"
```

### Task 2: ?? Forwarder ???????

**Files:**
- Create: `internal/im/forwarder/forwarder.go`
- Create: `internal/im/forwarder/state.go`
- Create: `internal/im/forwarder/adapter.go`
- Test: `internal/im/forwarder/forwarder_test.go`

- [ ] **Step 1: ?????(Emit ?? + OnMessage ??)**
- [ ] **Step 2: ??????**

Run: `go test ./internal/im/forwarder -v`  
Expected: FAIL(forwarder ???)

- [ ] **Step 3: ???? forwarder ???? adapter ??**

```go
type Adapter interface {
    Name() string
    OnMessage(func(im.Message))
    SendText(ctx context.Context, target SendTarget, text string) (MessageRef, error)
    SendCard(ctx context.Context, target SendTarget, card im.Card) (MessageRef, error)
    UpdateCard(ctx context.Context, messageID string, card im.Card) error
    OnCardAction(func(CardActionEvent))
    Run(ctx context.Context) error
    Close(ctx context.Context) error
}
```

- [ ] **Step 4: ???????**

Run: `go test ./internal/im/forwarder -v`  
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/im/forwarder
git commit -m "feat(im): add forwarder skeleton and adapter contract"
```

---

## Chunk 2: Decision ????????

### Task 3: RequestDecision ??????

**Files:**
- Modify: `internal/im/forwarder/forwarder.go`
- Create: `internal/im/forwarder/decision.go`
- Test: `internal/im/forwarder/decision_test.go`

- [ ] **Step 1: ?????(selected/timeout/cancelled/duplicate)**
- [ ] **Step 2: ??????**

Run: `go test ./internal/im/forwarder -run Decision -v`  
Expected: FAIL

- [ ] **Step 3: ?? pending map + timeout + resolve ??**

```go
type pendingDecision struct {
    req DecisionRequest
    ch  chan DecisionResult
    exp time.Time
}
```

- [ ] **Step 4: ???????**

Run: `go test ./internal/im/forwarder -run Decision -v`  
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/im/forwarder/decision.go internal/im/forwarder/decision_test.go internal/im/forwarder/forwarder.go
git commit -m "feat(im): implement multi-kind decision state machine"
```

### Task 4: `/help` ?????????

**Files:**
- Modify: `internal/im/forwarder/forwarder.go`
- Create: `internal/im/forwarder/help.go`
- Test: `internal/im/forwarder/help_test.go`

- [ ] **Step 1: ?????(/help ????????? /mode)**
- [ ] **Step 2: ??????**

Run: `go test ./internal/im/forwarder -run Help -v`  
Expected: FAIL

- [ ] **Step 3: ?? HelpResolver ? command injection**

```go
type HelpResolver func(ctx context.Context, chatID string) (HelpModel, error)
```

- [ ] **Step 4: ???????**

Run: `go test ./internal/im/forwarder -run Help -v`  
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/im/forwarder/help.go internal/im/forwarder/help_test.go internal/im/forwarder/forwarder.go
git commit -m "feat(im): add realtime help resolution and command mediation"
```

---

## Chunk 3: Feishu Adapter ??

### Task 5: Feishu adapter ??(WS)????????

**Files:**
- Create: `internal/im/feishu/adapter.go`
- Create: `internal/im/feishu/ws.go`
- Create: `internal/im/feishu/convert.go`
- Test: `internal/im/feishu/convert_test.go`

- [ ] **Step 1: ?????(???????@bot ????)**
- [ ] **Step 2: ??????**

Run: `go test ./internal/im/feishu -run Convert -v`  
Expected: FAIL

- [ ] **Step 3: ??????? Message ??**
- [ ] **Step 4: ???????**

Run: `go test ./internal/im/feishu -run Convert -v`  
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/im/feishu/adapter.go internal/im/feishu/ws.go internal/im/feishu/convert.go internal/im/feishu/convert_test.go
git commit -m "feat(feishu): add websocket inbound event conversion"
```

### Task 6: Feishu adapter ??(??/??/??/??/??)

**Files:**
- Modify: `internal/im/feishu/adapter.go`
- Create: `internal/im/feishu/send.go`
- Create: `internal/im/feishu/action.go`
- Test: `internal/im/feishu/send_test.go`
- Test: `internal/im/feishu/action_test.go`

- [ ] **Step 1: ?????(SendText?SendCard?UpdateCard?Reply)**
- [ ] **Step 2: ??????**

Run: `go test ./internal/im/feishu -run "Send|Action" -v`  
Expected: FAIL

- [ ] **Step 3: ??????? go-lark**
- [ ] **Step 4: ?? card action ????? forwarder**
- [ ] **Step 5: ???????**

Run: `go test ./internal/im/feishu -run "Send|Action" -v`  
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/im/feishu
git commit -m "feat(feishu): implement outbound messaging and card actions"
```

---

## Chunk 4: Client/Hub ????????

### Task 7: Client ?? Gateway ? IMUpdate ??

**Files:**
- Modify: `internal/client/client.go`
- Modify: `internal/client/permission.go`
- Modify: `internal/client/callbacks.go`
- Test: `internal/client/client_test.go`

- [ ] **Step 1: ?????(UpdateType ???Decision ??)**
- [ ] **Step 2: ??????**

Run: `go test ./internal/client -run "UpdateType|Decision|Help" -v`  
Expected: FAIL

- [ ] **Step 3: ?? client -> gateway ????**
- [ ] **Step 4: ???????**

Run: `go test ./internal/client -run "UpdateType|Decision|Help" -v`  
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/client/client.go internal/client/permission.go internal/client/callbacks.go internal/client/client_test.go
git commit -m "feat(client): integrate im gateway update and decision flow"
```

### Task 8: Hub ????? Feishu

**Files:**
- Modify: `internal/hub/config.go`
- Modify: `internal/hub/hub.go`
- Modify: `config.example.json`
- Test: `internal/hub/hub_test.go`

- [ ] **Step 1: ?????(im.type=feishu ????)**
- [ ] **Step 2: ??????**

Run: `go test ./internal/hub -v`  
Expected: FAIL

- [ ] **Step 3: ?? feishu adapter ???????**
- [ ] **Step 4: ???????**

Run: `go test ./internal/hub -v`  
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/hub/config.go internal/hub/hub.go config.example.json internal/hub/hub_test.go
git commit -m "feat(hub): wire feishu im gateway"
```

### Task 9: ?????????

**Files:**
- Modify: `README.md`
- Modify: `CLAUDE.md`
- Modify: `docs/feishu-bot.md` (?????????)

- [ ] **Step 1: ??????**

Run: `go test ./...`  
Expected: PASS

- [ ] **Step 2: ??????**

Run: `go vet ./...`  
Expected: ?????

- [ ] **Step 3: ?????????**

Run: `go test ./...`  
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add README.md CLAUDE.md docs/feishu-bot.md
git commit -m "docs: document feishu gateway and forwarder behavior"
```

---

## ????

- `@superpowers:test-driven-development`
- `@superpowers:verification-before-completion`
- `@superpowers:subagent-driven-development`

## ??????

?? Chunk ????? plan reviewer ????;??? chunk ?? >5 ????,??????????

