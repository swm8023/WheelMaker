# IM2 Formalization And IM1 Removal Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove IM1 completely, make IM2 the only supported IM runtime, preserve Feishu production behavior, and keep App as an official IM2 stub.

**Architecture:** Hub becomes IM2-only and always wires `im2.Router` plus one registered `im2.Channel`. Client removes all `hub/im` dependencies and uses only `IM2Router` for inbound messages, outbound replies, ACP updates, and decision requests. Feishu is reimplemented as a self-contained IM2 package, with existing IM1 behavior migrated into focused IM2 files instead of adapter wrappers.

**Tech Stack:** Go 1.26, IM2 router/channel contracts, Feishu HTTP/webhook APIs, ACP client callbacks, Go test

---

## File Structure

### Files To Modify

- `server/internal/shared/config.go` — remove `IM.Version`, add strict validation for removed IM1 fields/types
- `server/internal/shared/config_test.go` — update config parsing/validation expectations
- `server/internal/hub/hub.go` — remove IM1 branch and wire IM2-only startup
- `server/internal/hub/hub_test.go` — update startup tests to IM2-only behavior
- `server/internal/hub/client/client.go` — remove IM1 runtime entrypoints and IM1 state
- `server/internal/hub/client/session.go` — route all replies/permissions/updates through IM2 only
- `server/internal/hub/client/commands.go` — keep commands but ensure they operate only through IM2-driven session flow
- `server/internal/hub/client/permission.go` — remove IM1 decision fallback
- `server/internal/hub/client/im2_bridge.go` — keep as the sole IM integration path and simplify if needed
- `server/internal/hub/client/client_internal_test.go` — replace IM1-oriented helpers/tests with IM2-oriented ones
- `server/internal/hub/client/client_test.go` — update any `hub/im` assumptions
- `server/internal/im2/protocol.go` — keep canonical contract, adjust comments if needed
- `server/internal/im2/router.go` — preserve routing behavior and update tests if startup/usage assumptions change
- `server/internal/im2/router_test.go` — expand coverage for IM2-only runtime expectations
- `server/internal/im2/app/app.go` — keep official stub semantics explicit
- `server/internal/im2/app/app_test.go` — verify official stub behavior
- `server/internal/im2/feishu/feishu_test.go` — migrate tests off `hub/im` payload types
- `server/CLAUDE.md` — reflect IM2-only architecture

### Files To Create

- `server/internal/im2/feishu/channel.go` — top-level IM2 channel implementation and wiring
- `server/internal/im2/feishu/client.go` — Feishu outbound API client and helpers
- `server/internal/im2/feishu/webhook.go` — inbound webhook message/card-action handling
- `server/internal/im2/feishu/render.go` — `im2.OutboundEvent`/`im2.ACPPayload` rendering to Feishu messages/cards
- `server/internal/im2/feishu/decision.go` — pending decision lifecycle and resolution helpers
- `server/internal/im2/feishu/types.go` — local Feishu payload/card/update structs needed by IM2

### Files To Delete

- `server/internal/hub/im/adapter.go`
- `server/internal/hub/im/im.go`
- `server/internal/hub/im/types.go`
- `server/internal/hub/im/console/console.go`
- `server/internal/hub/im/feishu/feishu.go`
- `server/internal/hub/im/feishu/feishu_test.go`
- `server/internal/hub/im/mobile/protocol.go`
- `server/internal/hub/im/mobile/mobile.go`
- `server/internal/hub/im/forwarder_test.go`
- `server/internal/im2/feishu/feishu.go` (replace with split files above)

### Task 1: Hard-Cut Config And Hub Startup

**Files:**
- Modify: `server/internal/shared/config.go`
- Modify: `server/internal/shared/config_test.go`
- Modify: `server/internal/hub/hub.go`
- Modify: `server/internal/hub/hub_test.go`

- [ ] **Step 1: Write the failing config and hub tests**

```go
func TestLoadConfig_RejectsRemovedIMVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	data := []byte(`{"projects":[{"name":"p","path":".","im":{"type":"feishu","version":2}}]}`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := LoadConfig(path)
	if err == nil || !strings.Contains(err.Error(), "im.version has been removed") {
		t.Fatalf("err=%v, want removed im.version error", err)
	}
}

func TestBuildClient_RejectsRemovedConsoleType(t *testing.T) {
	h := New(&shared.AppConfig{Projects: []shared.ProjectConfig{{
		Name: "p",
		Path: ".",
		IM:   shared.IMConfig{Type: "console"},
	}}}, filepath.Join(t.TempDir(), "state.json"))
	err := h.Start(context.Background())
	if err == nil || !strings.Contains(err.Error(), "unsupported im.type") {
		t.Fatalf("err=%v, want unsupported im.type error", err)
	}
}

func TestBuildClient_AppTypeBuildsIM2Stub(t *testing.T) {
	h := New(&shared.AppConfig{Projects: []shared.ProjectConfig{{
		Name: "p",
		Path: ".",
		IM:   shared.IMConfig{Type: "app"},
	}}}, filepath.Join(t.TempDir(), "state.json"))
	if err := h.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
}
```

- [ ] **Step 2: Run targeted tests to verify they fail first**

Run: `cd server && go test ./internal/shared ./internal/hub -run "IMVersion|ConsoleType|AppType" -count=1`

Expected: FAIL because `IM.Version` is still accepted, `console` is still wired, and `app` is not yet supported in Hub.

- [ ] **Step 3: Implement IM2-only config validation and Hub startup**

```go
type IMConfig struct {
	Type      string `json:"type"`
	AppID     string `json:"appID,omitempty"`
	AppSecret string `json:"appSecret,omitempty"`
}

func LoadConfig(path string) (*AppConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}
	if bytes.Contains(data, []byte(`"version"`)) {
		return nil, fmt.Errorf("parse config %s: im.version has been removed; IM2 is the only supported runtime", path)
	}
	var cfg AppConfig
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	for _, p := range cfg.Projects {
		switch strings.ToLower(strings.TrimSpace(p.IM.Type)) {
		case "feishu", "app":
		default:
			return nil, fmt.Errorf("parse config %s: unsupported im.type %q (supported: feishu, app)", path, p.IM.Type)
		}
	}
	return &cfg, nil
}

func (h *Hub) buildClient(ctx context.Context, pc shared.ProjectConfig) (*client.Client, error) {
	return h.buildIM2Client(ctx, pc, resolveProjectCWD(pc.Path))
}

func (h *Hub) buildIM2Client(ctx context.Context, pc shared.ProjectConfig, cwd string) (*client.Client, error) {
	store := client.NewProjectJSONStore(h.statePath, pc.Name)
	c := client.New(store, pc.Name, cwd)
	router := im2.NewRouter(c, im2.NewMemoryHistoryStore())
	switch pc.IM.Type {
	case "feishu":
		_ = router.RegisterChannel(im2feishu.New(im2feishu.Config{AppID: pc.IM.AppID, AppSecret: pc.IM.AppSecret, Debug: pc.Debug, YOLO: pc.YOLO}))
	case "app":
		_ = router.RegisterChannel(im2app.New())
	default:
		return nil, fmt.Errorf("unsupported im.type %q (supported: feishu, app)", pc.IM.Type)
	}
	c.SetIM2Router(router)
	return c, c.Start(ctx)
}
```

- [ ] **Step 4: Re-run targeted tests**

Run: `cd server && go test ./internal/shared ./internal/hub -run "IMVersion|ConsoleType|AppType" -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add server/internal/shared/config.go server/internal/shared/config_test.go server/internal/hub/hub.go server/internal/hub/hub_test.go
git commit -m "refactor: hard-cut config and hub to IM2 only"
```

### Task 2: Remove IM1 From Client Runtime

**Files:**
- Modify: `server/internal/hub/client/client.go`
- Modify: `server/internal/hub/client/session.go`
- Modify: `server/internal/hub/client/commands.go`
- Modify: `server/internal/hub/client/permission.go`
- Modify: `server/internal/hub/client/im2_bridge.go`
- Test: `server/internal/hub/client/client_internal_test.go`
- Test: `server/internal/hub/client/client_test.go`

- [ ] **Step 1: Write failing IM2-only client tests**

```go
func TestSessionReply_UsesIM2Only(t *testing.T) {
	fake := &fakeIM2Router{}
	c := New(&noopStore{}, "proj", "/tmp")
	c.SetIM2Router(fake)
	sess := c.activeSession
	sess.setIM2Source(im2.ChatRef{ChannelID: "feishu", ChatID: "chat-a"})
	sess.reply("hello")
	if len(fake.sent) != 1 || fake.sent[0].event.Kind != im2.OutboundSystem {
		t.Fatalf("sent=%+v, want one IM2 system send", fake.sent)
	}
}

func TestSessionRequestPermission_NoIM2SourceCancels(t *testing.T) {
	s := newSession("s1", "/tmp")
	res, err := s.SessionRequestPermission(context.Background(), acp.PermissionRequestParams{})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if res.Outcome != "cancelled" {
		t.Fatalf("outcome=%q, want cancelled", res.Outcome)
	}
}

func TestClientHasNoIM1EntryPoint(t *testing.T) {
	var _ im2.InboundHandler = (*Client)(nil)
}
```

- [ ] **Step 2: Run targeted client tests to verify failure**

Run: `cd server && go test ./internal/hub/client -run "IM2Only|NoIM1EntryPoint|Permission_NoIM2Source" -count=1`

Expected: FAIL because `Client`/`Session` still depend on `hub/im` types and IM1 fallbacks.

- [ ] **Step 3: Remove IM1 fields and route all client behavior through IM2**

```go
type Client struct {
	projectName string
	cwd         string
	yolo        bool

	registry    *agent.ACPFactory
	persistence ClientStateStore
	state       *ProjectState
	im2Router   IM2Router

	mu       sync.Mutex
	sessions map[string]*Session
	routeMap map[string]string
}

type Session struct {
	ID        string
	Status    SessionStatus
	instance  agent.Instance
	im2Router IM2Router
	im2Source *im2.ChatRef
}

func (s *Session) reply(text string) {
	router, source, ok := s.im2Context()
	if !ok {
		return
	}
	_ = router.Send(context.Background(), im2.SendTarget{SessionID: s.ID, Source: &source}, im2.OutboundEvent{
		Kind:    im2.OutboundSystem,
		Payload: im2.TextPayload{Text: text},
	})
}

func (r *permissionRouter) decide(ctx context.Context, params acp.PermissionRequestParams, mode string) (acp.PermissionResult, error) {
	if !r.acquireDecisionSlot(ctx) {
		return acp.PermissionResult{Outcome: "cancelled"}, nil
	}
	defer r.releaseDecisionSlot()
	if router, source, ok := r.session.im2Context(); ok {
		res, err := router.RequestDecision(ctx, im2.SendTarget{SessionID: r.session.ID, Source: &source}, im2.DecisionRequest{Kind: im2.DecisionPermission})
		if err == nil && res.Outcome == "selected" {
			return acp.PermissionResult{Outcome: "selected", OptionID: res.OptionID}, nil
		}
	}
	return acp.PermissionResult{Outcome: "cancelled"}, nil
}
```

- [ ] **Step 4: Re-run client tests and the full client package**

Run: `cd server && go test ./internal/hub/client/... -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add server/internal/hub/client/client.go server/internal/hub/client/session.go server/internal/hub/client/commands.go server/internal/hub/client/permission.go server/internal/hub/client/im2_bridge.go server/internal/hub/client/client_internal_test.go server/internal/hub/client/client_test.go
git commit -m "refactor: remove IM1 from client runtime"
```

### Task 3: Rebuild Feishu As A Self-Contained IM2 Channel

**Files:**
- Delete: `server/internal/im2/feishu/feishu.go`
- Create: `server/internal/im2/feishu/channel.go`
- Create: `server/internal/im2/feishu/client.go`
- Create: `server/internal/im2/feishu/webhook.go`
- Create: `server/internal/im2/feishu/render.go`
- Create: `server/internal/im2/feishu/decision.go`
- Create: `server/internal/im2/feishu/types.go`
- Modify: `server/internal/im2/feishu/feishu_test.go`

- [ ] **Step 1: Write failing Feishu parity tests against the IM2 contract**

```go
func TestChannel_SendACPThought(t *testing.T) {
	f := newFakeFeishuClient()
	ch := newWithClient(f)
	err := ch.Send(context.Background(), "chat-a", im2.OutboundEvent{
		Kind: im2.OutboundACP,
		Payload: im2.ACPPayload{UpdateType: "thought", Text: "thinking"},
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if got := f.lastTextKind; got != textKindThought {
		t.Fatalf("kind=%v, want thought", got)
	}
}

func TestChannel_RequestDecision_ResolvesCardAction(t *testing.T) {
	f := newFakeFeishuClient()
	ch := newWithClient(f)
	done := make(chan im2.DecisionResult, 1)
	go func() {
		res, _ := ch.RequestDecision(context.Background(), "chat-a", im2.DecisionRequest{Title: "Approve", Options: []im2.DecisionOption{{ID: "allow", Label: "Allow", Value: "allow_once"}}})
		done <- res
	}()
	f.fireCardAction(fakeCardAction{DecisionID: f.lastDecisionID, OptionID: "allow"})
	res := <-done
	if res.Outcome != "selected" || res.OptionID != "allow" {
		t.Fatalf("res=%+v, want selected allow", res)
	}
}
```

- [ ] **Step 2: Run Feishu tests to verify failure**

Run: `cd server && go test ./internal/im2/feishu -count=1`

Expected: FAIL because the current package still depends on `hub/im` and the split files do not exist yet.

- [ ] **Step 3: Move Feishu transport, rendering, and decision state into IM2-owned files**

```go
type apiClient interface {
	SendText(chatID, text string, kind textKind) error
	SendCard(chatID string, card cardPayload) error
	MarkDone(chatID string) error
	Run(ctx context.Context) error
	OnMessage(func(inboundMessage))
	OnCardAction(func(cardAction))
}

type Channel struct {
	client apiClient
	mu     sync.Mutex
	msg    func(context.Context, string, string) error
	byID   map[string]pendingDecision
	byChat map[string]pendingDecision
	closed map[string]time.Time
	nextID atomic.Int64
}

func (c *Channel) Send(ctx context.Context, chatID string, event im2.OutboundEvent) error {
	switch event.Kind {
	case im2.OutboundACP:
		return c.renderACP(chatID, event.Payload)
	case im2.OutboundSystem:
		return c.client.SendText(chatID, payloadText(event.Payload), textKindSystem)
	default:
		return c.client.SendText(chatID, payloadText(event.Payload), textKindNormal)
	}
}

func (c *Channel) RequestDecision(ctx context.Context, chatID string, req im2.DecisionRequest) (im2.DecisionResult, error) {
	decisionID := fmt.Sprintf("im2-dec-%d", c.nextID.Add(1))
	card := buildDecisionCard(req, decisionID, chatID)
	if err := c.client.SendCard(chatID, card); err != nil {
		return im2.DecisionResult{Outcome: "invalid"}, err
	}
	return c.waitDecision(ctx, chatID, decisionID, req)
}
```

- [ ] **Step 4: Re-run Feishu tests**

Run: `cd server && go test ./internal/im2/feishu -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add server/internal/im2/feishu server/internal/im2/feishu/feishu_test.go
git commit -m "refactor: make Feishu a self-contained IM2 channel"
```

### Task 4: Delete IM1 Package And Update Remaining References

**Files:**
- Delete: `server/internal/hub/im/adapter.go`
- Delete: `server/internal/hub/im/im.go`
- Delete: `server/internal/hub/im/types.go`
- Delete: `server/internal/hub/im/console/console.go`
- Delete: `server/internal/hub/im/feishu/feishu.go`
- Delete: `server/internal/hub/im/feishu/feishu_test.go`
- Delete: `server/internal/hub/im/mobile/protocol.go`
- Delete: `server/internal/hub/im/mobile/mobile.go`
- Delete: `server/internal/hub/im/forwarder_test.go`
- Modify: `server/CLAUDE.md`

- [ ] **Step 1: Write a repository guardrail test**

```go
func TestRepositoryHasNoHubIMImports(t *testing.T) {
	cmd := exec.Command("go", "list", "-f", `{{.ImportPath}} {{join .Imports \" \"}}`, "./...")
	cmd.Dir = filepath.Join("..", "..", "..")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list: %v\n%s", err, out)
	}
	if strings.Contains(string(out), "internal/hub/im") {
		t.Fatalf("found forbidden hub/im import:\n%s", out)
	}
}
```

- [ ] **Step 2: Run the guardrail test to verify it fails**

Run: `cd server && go test ./... -run "RepositoryHasNoHubIMImports" -count=1`

Expected: FAIL because IM1 files and imports still exist.

- [ ] **Step 3: Delete IM1 packages and update docs/imports**

```text
Delete server/internal/hub/im/** entirely.
Update server/CLAUDE.md package map and architecture text to refer only to internal/im2.
Replace any remaining imports of github.com/swm8023/wheelmaker/internal/hub/im with IM2-native paths or remove them.
```

- [ ] **Step 4: Run the guardrail and the full server test suite**

Run: `cd server && go test ./... -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add server/CLAUDE.md server/internal/hub server/internal/im2 server/internal/shared
git rm -r server/internal/hub/im
git commit -m "refactor: remove IM1 packages and finalize IM2 runtime"
```

### Task 5: Final Verification And Release Steps

**Files:**
- Modify: any remaining touched files from Tasks 1-4

- [ ] **Step 1: Run targeted verification commands**

Run: `cd server && go test ./internal/shared ./internal/hub ./internal/im2/... -count=1`

Expected: PASS.

- [ ] **Step 2: Run full repository verification**

Run: `cd server && go test ./... -count=1`

Expected: PASS.

- [ ] **Step 3: Review final diff**

Run: `cd .. && git status --short && git diff --stat HEAD~3..HEAD`

Expected: only IM2 formalization, IM1 removal, Feishu migration, and doc updates are present.

- [ ] **Step 4: Commit any final cleanup**

```bash
git add -A
git commit -m "chore: finalize IM2 formalization cleanup"
```

- [ ] **Step 5: Push and trigger deploy hooks**

```bash
git push origin main
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/signal_update_now.ps1 -DelaySeconds 30
```