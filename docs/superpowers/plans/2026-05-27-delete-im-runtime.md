# Delete IM Runtime Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Delete the Feishu/app IM Module cleanly so App conversations use only `session.*` and `SessionRecorder -> registry.session.*` events.

**Architecture:** `projectId + sessionId` becomes the only App Conversation identity. `Session` emits view events to `SessionRecorder`; the recorder publishes `registry.session.message` and `registry.session.updated`; Registry broadcasts them to App as `session.message` and `session.updated`. IM concepts (`IMRouter`, `ChatRef`, `routeMap`, `chat.send`, app IM channel, Feishu adapter, typed control commands) are removed from production code and active docs.

**Tech Stack:** Go 1.26.1 server, SQLite store, Registry WebSocket protocol, React/TypeScript App, Jest, `go test`.

---

## Clean Deletion Rule

Do not leave a no-op IM Interface. The only allowed temporary leftovers are inert migration data:

- `route_bindings` schema/table may remain in SQLite for one migration window, but production code must not read, write, query, or expose it.
- Legacy `projects[].feishu` JSON may be accepted long enough to avoid breaking old config files, but it must not affect validation, runtime startup, protocol payloads, UI, docs, or tests.
- Active docs must describe App/Conversation Registry only. Historical planning/spec files under `docs/superpowers/` can remain as history unless the user requests an archival purge.

## File Map

| Path | Responsibility after this refactor |
| --- | --- |
| `server/internal/hub/client/client.go` | Session request handling only; no IM routing, no `ChatRef`, no `routeMap`, no typed slash command interception. |
| `server/internal/hub/client/session.go` | Session lifecycle and ACP prompt handling; event output goes only through `SessionViewSink`. |
| `server/internal/hub/client/session_recorder.go` | Sole session message/event publisher. |
| `server/internal/hub/client/sqlite_store.go` | Session/project/agent preference persistence; `route_bindings` schema can remain inert, Store interface removes route methods. |
| `server/internal/hub/client/commands.go` | Delete. Typed control commands are removed. |
| `server/internal/im/` | Delete the entire package after all imports are removed. |
| `server/internal/hub/hub.go` | Build App-only project clients; no IM router/channel setup, no Feishu startup behavior. |
| `server/internal/hub/reporter.go` | Registry forwarding for `session.*`; remove `ChatHandler` and `chat.send`. |
| `server/internal/registry/server.go` | Client allowlist keeps `session.*`; remove `chat.send`. |
| `server/internal/shared/config.go` | Keep only app-only project config; parse legacy `feishu` as ignored compatibility input if needed. |
| `server/internal/protocol/registry.go` | Remove `imType` from Project payloads. |
| `server/internal/protocol/global.go` | Remove legacy `SessionOpenPayload.IMType`. |
| `app/web/src/types/registry.ts` | Remove `imType` from `RegistryProject`. |
| `server/cmd/wheelmaker-monitor/*` | Remove IM type fields and badges. |
| `README.md`, `server/CLAUDE.md`, `docs/registry-protocol.md`, `docs/session-management-and-sync.zh-CN.md` | Remove active IM/Feishu/route binding docs. |
| `docs/feishu-bot.md` | Delete. |

---

### Task 1: Make `session.send` Direct And Treat Slash Text As Prompt

**Files:**
- Modify: `server/internal/hub/client/client.go`
- Modify: `server/internal/hub/client/client_test.go`
- Test: `server/internal/hub/client/client_test.go`

- [ ] **Step 1: Replace the slash-command regression test**

Replace `TestHandleSessionRequest_SessionSendSlashSkills` with a test that proves `/skills` is normal prompt text:

```go
func TestHandleSessionRequestSessionSendSlashTextIsPrompt(t *testing.T) {
	mock := &mockSession{agentName: "codex", sessionID: "sess-send-slash"}
	c := newTestClient(t, mock)
	published := captureSessionMessageEvents(t, c)

	payload := json.RawMessage(`{"sessionId":"sess-send-slash","text":"/skills"}`)
	resp, err := c.HandleSessionRequest(context.Background(), "session.send", "proj1", payload)
	if err != nil {
		t.Fatalf("HandleSessionRequest(session.send): %v", err)
	}
	body, ok := resp.(map[string]any)
	if !ok || body["ok"] != true {
		t.Fatalf("response = %#v, want ok=true", resp)
	}

	first := (*published)[0].payload
	turn := publishedTurnMap(t, first)
	content, _ := turn["content"].(string)
	if !strings.Contains(content, `"/skills"`) {
		t.Fatalf("first turn content = %q, want prompt text /skills", content)
	}
	if strings.Contains(content, `"method":"system"`) {
		t.Fatalf("slash text was handled as a system command: %q", content)
	}
}
```

- [ ] **Step 2: Run the focused test and verify it fails**

Run:

```powershell
cd server
go test ./internal/hub/client -run TestHandleSessionRequestSessionSendSlashTextIsPrompt -count=1 -v
```

Expected: FAIL because `session.send` still parses `/skills` and emits a command reply instead of a prompt request.

- [ ] **Step 3: Change `PromptToSession` to use session identity only**

In `server/internal/hub/client/client.go`, replace:

```go
func (c *Client) PromptToSession(ctx context.Context, sessionID string, source im.ChatRef, blocks []acp.ContentBlock) error {
	sess, err := c.SessionByID(ctx, sessionID)
	if err != nil {
		return err
	}
	source = normalizeChatRef(source)
	if hasChatRef(source) {
		sess.setIMSource(source)
		if err := c.bindIM(ctx, source, sess.acpSessionID); err != nil {
			return err
		}
		if err := c.store.SaveRouteBinding(ctx, c.projectName, imRouteKey(source), sess.acpSessionID); err != nil {
			return fmt.Errorf("save route binding: %w", err)
		}
	}
	sess.handlePromptBlocks(blocks)
	return nil
}
```

with:

```go
func (c *Client) PromptToSession(ctx context.Context, sessionID string, blocks []acp.ContentBlock) error {
	sess, err := c.SessionByID(ctx, sessionID)
	if err != nil {
		return err
	}
	sess.handlePromptBlocks(blocks)
	return nil
}
```

Delete `normalizeChatRef` and `hasChatRef` from `client.go`.

- [ ] **Step 4: Remove command parsing from `session.send`**

In the `case "session.send"` branch, replace the command parsing block and old call:

```go
if text, ok := singleTextIMPrompt(blocks); ok {
	if cmd, args, parsed := parseCommand(text); parsed {
		sess, err := c.SessionByID(ctx, req.SessionID)
		if err != nil {
			return nil, err
		}
		sessionID := strings.TrimSpace(req.SessionID)
		sess.setIMSource(im.ChatRef{ChannelID: "app", ChatID: sessionID})
		c.handleCommand(sess, "app:"+sessionID, cmd, args)
		return map[string]any{"ok": true, "sessionId": strings.TrimSpace(req.SessionID)}, nil
	}
}
if err := c.PromptToSession(ctx, req.SessionID, im.ChatRef{ChannelID: "app", ChatID: strings.TrimSpace(req.SessionID)}, blocks); err != nil {
	return nil, err
}
```

with:

```go
sessionID := strings.TrimSpace(req.SessionID)
if err := c.PromptToSession(ctx, sessionID, blocks); err != nil {
	return nil, err
}
```

Keep the existing attachment validation and `markSessionAttachmentsSent` call.

- [ ] **Step 5: Remove App IM source writes from `session.cancel`**

In the `case "session.cancel"` branch, remove:

```go
sess.setIMSource(im.ChatRef{ChannelID: "app", ChatID: sessionID})
```

The branch should call `SessionByID`, then `sess.cancelPrompt()`, then return `{"ok": true, "sessionId": sessionID}`.

- [ ] **Step 6: Update tests that call `PromptToSession`**

Replace every test call shaped like:

```go
c.PromptToSession(context.Background(), "sess-id", im.ChatRef{ChannelID: "app", ChatID: "sess-id"}, blocks)
```

with:

```go
c.PromptToSession(context.Background(), "sess-id", blocks)
```

The known occurrences are near `server/internal/hub/client/client_test.go:7586` and `server/internal/hub/client/client_test.go:7632`.

- [ ] **Step 7: Run focused tests**

Run:

```powershell
cd server
go test ./internal/hub/client -run "TestHandleSessionRequestSessionSendSlashTextIsPrompt|TestHandleSessionRequestSessionCancel|TestPromptToSession" -count=1 -v
```

Expected: PASS for non-IM session send/cancel tests; any compile failures should be IM references left in `client.go` or tests.

- [ ] **Step 8: Commit**

```powershell
git add server/internal/hub/client/client.go server/internal/hub/client/client_test.go
git commit -m "refactor(session): send app prompts without im routing"
```

---

### Task 2: Make `SessionRecorder` The Only Session Message Outlet

**Files:**
- Modify: `server/internal/hub/client/session.go`
- Modify: `server/internal/hub/client/client.go`
- Modify: `server/internal/hub/client/client_test.go`
- Test: `server/internal/hub/client/client_test.go`

- [ ] **Step 1: Write recorder-only tests**

Replace `TestReplyWithTitleRecordsLegacySystemEvent` with:

```go
func TestReplyWithTitleRecordsSystemEventThroughViewSink(t *testing.T) {
	sink := &recordingSessionViewSink{}
	s := mustNewSession(t, "sess-1", "/tmp", "claude")
	s.viewSink = sink

	s.replyWithTitle("Switched", "session: sess-1")

	if len(sink.events) != 1 {
		t.Fatalf("session view events len = %d, want 1", len(sink.events))
	}
	event := sink.events[0]
	if event.Type != SessionViewEventTypeSystem {
		t.Fatalf("event.Type = %q, want %q", event.Type, SessionViewEventTypeSystem)
	}
	if event.SessionID != "sess-1" {
		t.Fatalf("event.SessionID = %q, want %q", event.SessionID, "sess-1")
	}
	if strings.TrimSpace(event.Content) != "Switched\nsession: sess-1" {
		t.Fatalf("event.Content = %q, want %q", event.Content, "Switched\nsession: sess-1")
	}
	if event.SourceChannel != "" || event.SourceChatID != "" {
		t.Fatalf("event source = (%q, %q), want empty source", event.SourceChannel, event.SourceChatID)
	}
}
```

Replace `TestReportTimeoutError_RecordsSystemEvent` with:

```go
func TestReportTimeoutErrorRecordsSystemEventThroughViewSink(t *testing.T) {
	sink := &recordingSessionViewSink{}
	s := mustNewSession(t, "sess-1", "/tmp", "claude")
	s.viewSink = sink

	s.reportTimeoutError("stream", "silence")

	if len(sink.events) != 1 {
		t.Fatalf("session view events len = %d, want 1", len(sink.events))
	}
	event := sink.events[0]
	if event.Type != SessionViewEventTypeSystem {
		t.Fatalf("event.Type = %q, want %q", event.Type, SessionViewEventTypeSystem)
	}
	if !strings.Contains(event.Content, "category=timeout stage=stream") {
		t.Fatalf("event.Content = %q, want timeout payload", event.Content)
	}
	if event.SourceChannel != "" || event.SourceChatID != "" {
		t.Fatalf("event source = (%q, %q), want empty source", event.SourceChannel, event.SourceChatID)
	}
}
```

- [ ] **Step 2: Run the focused tests and verify they fail**

Run:

```powershell
cd server
go test ./internal/hub/client -run "TestReplyWithTitleRecordsSystemEventThroughViewSink|TestReportTimeoutErrorRecordsSystemEventThroughViewSink" -count=1 -v
```

Expected: FAIL or compile error because `Session` still carries IM fields and the old tests still expect router notifications.

- [ ] **Step 3: Remove IM fields and methods from `Session`**

In `server/internal/hub/client/session.go`, remove the `internal/im` import and remove these fields from `Session`:

```go
imRouter    IMRouter
imSource    *im.ChatRef
```

Delete these methods:

```go
func (s *Session) setIMSource(source im.ChatRef) { ... }
func (s *Session) imContext() (IMRouter, im.ChatRef, bool) { ... }
func sessionViewEventToIMTurnMessage(event SessionViewEvent) (acp.IMTurnMessage, bool) { ... }
func sessionUpdateToIMTurnMessage(update acp.SessionUpdate) (acp.IMTurnMessage, bool) { ... }
func makeIMTurnMessage(method string, payload any) (acp.IMTurnMessage, bool) { ... }
```

- [ ] **Step 4: Replace `recordSessionViewEvent`**

Replace the whole function with:

```go
func (s *Session) recordSessionViewEvent(event SessionViewEvent) bool {
	if strings.TrimSpace(event.SessionID) == "" {
		event.SessionID = s.acpSessionID
	}
	if event.UpdatedAt.IsZero() {
		event.UpdatedAt = time.Now().UTC()
	}
	s.mu.Lock()
	s.lastActiveAt = maxTime(s.lastActiveAt, event.UpdatedAt)
	s.mu.Unlock()
	if s.viewSink != nil {
		_ = s.viewSink.RecordEvent(context.Background(), event)
		return true
	}
	return false
}
```

- [ ] **Step 5: Simplify prompt streaming fallback**

In `handlePromptBlocks`, remove:

```go
imRouter, imSource, hasIMEmitter := s.imContext()
```

and replace all checks of `hasIMEmitter` with `false` behavior. The update branch becomes:

```go
if ev.update != nil {
	params := *ev.update
	s.recordSessionViewEvent(SessionViewEvent{
		Type:      SessionViewEventTypeACP,
		SessionID: s.acpSessionID,
		Content: acp.BuildACPContentJSON(acp.MethodSessionUpdate, map[string]any{
			"params": params,
		}),
	})
	if params.Update.SessionUpdate == acp.SessionUpdateAgentMessageChunk {
		text := extractTextChunk(params.Update.Content)
		if strings.TrimSpace(text) != "" {
			buf.WriteString(text)
		}
	}
	if params.Update.SessionUpdate == acp.SessionUpdateConfigOptionUpdate {
		raw, _ := json.Marshal(params.Update)
		s.reply(formatConfigOptionUpdateMessage(raw))
		s.persistSessionBestEffort()
	}
}
```

The result branch becomes:

```go
if ev.result != nil {
	s.recordSessionViewEvent(SessionViewEvent{
		Type:      SessionViewEventTypeACP,
		SessionID: s.acpSessionID,
		Content: acp.BuildACPContentJSON(acp.MethodSessionPrompt, map[string]any{
			"result": *ev.result,
		}),
	})
	streamDone = true
}
```

At the end, replace:

```go
if !hasIMEmitter && buf.Len() > 0 {
	s.reply(buf.String())
}
```

with:

```go
if buf.Len() > 0 {
	s.reply(buf.String())
}
```

- [ ] **Step 6: Remove `IMRouter` wiring from Client**

In `server/internal/hub/client/client.go`, remove the `IMRouter` and `IMSessionMessageRouter` interface definitions, the `imRouter` field, and `SetIMRouter`/`HasIMRouter`. In `wireSession`, remove:

```go
sess.imRouter = c.imRouter
```

In `Client.Run`, replace the IM router delegation with:

```go
func (c *Client) Run(ctx context.Context) error {
	<-ctx.Done()
	return nil
}
```

- [ ] **Step 7: Delete IM-specific tests**

Remove these tests and helper types from `server/internal/hub/client/client_test.go`:

```text
fakeIMRouter
TestCaptureRouter
failingPermissionIMRouter
captureReplies
TestHandleIMInbound_ListDirectDoesNotBind
TestHandleIMInbound_NewWithoutAgentOpensHelpCardAndDoesNotBind
TestHandleIMInbound_UnboundPromptBindsAndEmitsACP
TestHandleIMInbound_ViewSinkFailureDoesNotBlockIMUpdates
TestSessionRequestPermissionAutoAllowsWithoutIMRoundTrip
```

Keep `recordingSessionViewSink` because recorder-only tests still use it.

- [ ] **Step 8: Run focused tests**

Run:

```powershell
cd server
go test ./internal/hub/client -run "TestReplyWithTitleRecordsSystemEventThroughViewSink|TestReportTimeoutErrorRecordsSystemEventThroughViewSink|TestHandleSessionRequestSessionSendSlashTextIsPrompt" -count=1 -v
```

Expected: PASS.

- [ ] **Step 9: Commit**

```powershell
git add server/internal/hub/client/session.go server/internal/hub/client/client.go server/internal/hub/client/client_test.go
git commit -m "refactor(session): publish session events through recorder only"
```

---

### Task 3: Remove Typed Command And Route-Key Modules

**Files:**
- Delete: `server/internal/hub/client/commands.go`
- Modify: `server/internal/hub/client/client.go`
- Modify: `server/internal/hub/client/client_test.go`
- Test: `server/internal/hub/client/client_test.go`

- [ ] **Step 1: Delete command and help-card tests**

Remove tests that call or assert these deleted functions:

```text
handleCommand
handleListCommand
handleNewCommand
handleLoadCommand
resolveConfigArg
resolveModeArg
resolveModelArg
resolveHelpModel
helpModelForRoute
resolveDetachedHelpModel
parseCommand
```

Known test names include:

```text
TestResolveHelpModelRefreshesSessionMenuFromRuntimeList
TestResolveHelpModel_RootStartsWithNewConversationMenu
TestResolveHelpModel_UsesRawConfigOptionName
TestHandleSessionRequest_SessionSendSlashSkills
TestResolveSession_RejectsEmptyRouteKey
```

The replacement coverage is:

- `session.new` tests continue to cover creating sessions.
- `session.setConfig` and `Session.SetConfigOption` tests continue to cover config changes.
- `session.cancel` tests continue to cover cancellation.
- `session.list`, `session.read`, and sidebar UI cover session discovery.

- [ ] **Step 2: Delete `commands.go`**

Delete `server/internal/hub/client/commands.go`.

- [ ] **Step 3: Delete IM inbound and route-key functions from `client.go`**

Remove these functions from `server/internal/hub/client/client.go`:

```text
HandleIMPrompt
HandleIMCommand
HandleIMInbound
bindIM
sendIMDirect
loadSessionForIM
imRouteKey
singleTextIMPrompt
resolveOrCreateIMSession
parseHelpArgs
sendHelpCard
resolveSession
helpModelForRoute
resolveDetachedHelpModel
ClientNewSession
clientNewSessionWithOptions
ClientLoadSession
parseCommand
normalizeRouteKey
```

After deletion, `client.go` must not import `github.com/swm8023/wheelmaker/internal/im`, `strconv`, or `errors` for removed IM-only code. Keep imports only if non-IM code still uses them.

- [ ] **Step 4: Replace route-based test helpers**

In `server/internal/hub/client/client_test.go`, replace helpers that create sessions by route with helpers that use session IDs. Replace:

```go
func (c *Client) RouteSessionIDForTest(routeKey string) string { ... }
func (c *Client) ResolveSessionForTest(routeKey string) (*Session, error) { ... }
```

with:

```go
func (c *Client) SessionForTest(sessionID string) (*Session, error) {
	return c.SessionByID(context.Background(), sessionID)
}
```

Update tests that used `c.resolveSession(testRouteKey)` to use `c.SessionForTest("expected-session-id")` or `c.SessionByID(context.Background(), "expected-session-id")`.

- [ ] **Step 5: Run route/command negative search**

Run:

```powershell
rg -n "handleCommand|HandleIM|ChatRef|IMRouter|routeMap|parseCommand|HelpCard|ClientNewSession|ClientLoadSession|resolveSession\\(" server\\internal\\hub\\client
```

Expected: no matches, except this plan file is outside the searched directory.

- [ ] **Step 6: Run client package tests**

Run:

```powershell
cd server
go test ./internal/hub/client -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit**

```powershell
git add server/internal/hub/client
git commit -m "refactor(client): remove im commands and route keys"
```

---

### Task 4: Remove Route Binding Store Interface Usage

**Files:**
- Modify: `server/internal/hub/client/sqlite_store.go`
- Modify: `server/internal/hub/client/session_archive.go`
- Modify: `server/internal/hub/client/client_test.go`
- Test: `server/internal/hub/client/client_test.go`

- [ ] **Step 1: Remove route methods from `Store`**

In `server/internal/hub/client/sqlite_store.go`, replace:

```go
type Store interface {
	LoadRouteBindings(ctx context.Context, projectName string) (map[string]string, error)
	SaveRouteBinding(ctx context.Context, projectName, routeKey, sessionID string) error
	DeleteRouteBinding(ctx context.Context, projectName, routeKey string) error
	LoadProjectDefaultAgent(ctx context.Context, projectName string) (string, error)
	SaveProjectDefaultAgent(ctx context.Context, projectName, agentType string) error
```

with:

```go
type Store interface {
	LoadProjectDefaultAgent(ctx context.Context, projectName string) (string, error)
	SaveProjectDefaultAgent(ctx context.Context, projectName, agentType string) error
```

- [ ] **Step 2: Delete route binding methods**

Delete these functions from `sqlite_store.go`:

```text
LoadRouteBindings
SaveRouteBinding
DeleteRouteBinding
validateRouteKey
```

Keep the `route_bindings` table in `sqliteSchema` for this task. It is inert migration data.

- [ ] **Step 3: Remove route deletes from session deletion/archive**

In `sqlite_store.go`, update `DeleteSession` to delete from `sessions` and related session data only. Remove:

```sql
DELETE FROM route_bindings
WHERE project_name = ? AND session_id = ?
```

In `session_archive.go` and archive tests, remove assertions that archive/delete clear route mappings. The session delete/archive behavior is now scoped to sessions and history files only.

- [ ] **Step 4: Update test stores**

In `client_test.go`, update `noopStore` to remove:

```go
func (s *noopStore) LoadRouteBindings(context.Context, string) (map[string]string, error) { ... }
func (s *noopStore) SaveRouteBinding(context.Context, string, string, string) error { ... }
func (s *noopStore) DeleteRouteBinding(context.Context, string, string) error { ... }
```

Delete these tests:

```text
TestStart_LoadsRouteBindingsWithoutRestoringSessions
TestSQLiteStore_ProjectRouteAndSessionRoundTrip
TestSQLiteStore_RejectsEmptyRouteKey
```

- [ ] **Step 5: Run route store negative search**

Run:

```powershell
rg -n "LoadRouteBindings|SaveRouteBinding|DeleteRouteBinding|validateRouteKey|routeMap|route binding" server\\internal\\hub\\client
```

Expected: no production code matches. Test descriptions mentioning old behavior should also be gone.

- [ ] **Step 6: Run client tests**

Run:

```powershell
cd server
go test ./internal/hub/client -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit**

```powershell
git add server/internal/hub/client
git commit -m "refactor(store): stop reading and writing route bindings"
```

---

### Task 5: Remove `chat.send`, App IM Adapter, And IM Router Wiring

**Files:**
- Delete: `server/internal/im/app/app.go`
- Delete: `server/internal/im/app/app_test.go`
- Modify: `server/internal/hub/hub.go`
- Modify: `server/internal/hub/reporter.go`
- Modify: `server/internal/registry/server.go`
- Modify: `server/internal/hub/hub_test.go`
- Modify: `server/internal/registry/server_test.go`
- Test: `server/internal/hub`, `server/internal/registry`

- [ ] **Step 1: Remove chat handler interfaces from Reporter**

In `server/internal/hub/reporter.go`, delete:

```go
type ChatHandler interface {
	HandleChatRequest(ctx context.Context, method string, projectID string, payload json.RawMessage) (any, error)
}
```

Remove the `chatByID` map field from `Reporter`, remove `RegisterChatHandler`, remove `replyChat`, and remove the `case "chat.send"` branch from `handleRegistryRequest`.

- [ ] **Step 2: Remove `chat.send` forwarding from Registry Server**

In `server/internal/registry/server.go`, remove `"chat.send"` from:

```go
case "chat.send",
	"session.list", ...
```

and from any method allowlist text. Client requests for `chat.send` should now return `INVALID_ARGUMENT` / unsupported method.

- [ ] **Step 3: Replace Registry test coverage**

In `server/internal/registry/server_test.go`, update tests that include `chat.send` in allowlists or examples. Add this test:

```go
func TestChatSendIsUnsupportedAfterIMRemoval(t *testing.T) {
	s := NewServer(Config{Token: "tok"})
	client, cleanup := newRegistryTestClient(t, s, "client", "")
	defer cleanup()

	resp := client.request(t, envelope{
		RequestID: 1,
		Type:      "request",
		Method:    "chat.send",
		ProjectID: "hub-a:proj1",
		Payload:   json.RawMessage(`{"chatId":"chat-1","text":"hello"}`),
	})
	if resp.Type != "error" {
		t.Fatalf("chat.send response type = %q, want error", resp.Type)
	}
	payload := errorPayloadFromEnvelopeForTest(t, resp)
	if payload.Code != codeInvalidArgument {
		t.Fatalf("chat.send code = %q, want %q", payload.Code, codeInvalidArgument)
	}
}
```

If the existing registry test helpers use different names, adapt only the helper names while keeping the assertion: `chat.send` must be unsupported.

- [ ] **Step 4: Remove Hub IM setup**

In `server/internal/hub/hub.go`, remove imports:

```go
im "github.com/swm8023/wheelmaker/internal/im"
imapp "github.com/swm8023/wheelmaker/internal/im/app"
imfeishu "github.com/swm8023/wheelmaker/internal/im/feishu"
```

Delete constants:

```go
const (
	feishuVerificationToken = ""
	feishuEncryptKey        = ""
)
```

Remove `appIM map[string]*imapp.Channel` from `Hub`. In `New`, remove initialization of `appIM`.

In `buildIMClient`, replace the router/channel registration block with:

```go
hubLogger(pc.Name).Info("starting client")
if err := c.Start(ctx); err != nil {
	hubLogger(pc.Name).Error("start client failed err=%v", err)
	_ = c.Close()
	return nil, fmt.Errorf("start: %w", err)
}
hubLogger(pc.Name).Info("client started")
return c, nil
```

In `setupRegistrySync`, remove:

```go
appChannel := h.appIM[project.Name]
...
if appChannel != nil {
	rep.RegisterChatHandler(projectID, appChannel)
}
```

- [ ] **Step 5: Delete app IM adapter package**

Delete:

```text
server/internal/im/app/app.go
server/internal/im/app/app_test.go
```

- [ ] **Step 6: Update Hub tests**

Remove tests that assert `c.HasIMRouter()` or registered app/Feishu channels. Keep tests that assert:

- project clients are built,
- session handlers are registered,
- `session.send` forwards through Reporter,
- registry session events broadcast.

Known failing tests after this step include early `server/internal/hub/hub_test.go` tests around `HasIMRouter`; rewrite them to assert `c != nil` and successful `session.list` or `session.new`.

- [ ] **Step 7: Run package tests**

Run:

```powershell
cd server
go test ./internal/hub ./internal/registry -count=1
```

Expected: PASS.

- [ ] **Step 8: Commit**

```powershell
git add server/internal/hub server/internal/registry server/internal/im/app
git commit -m "refactor(registry): remove chat send and app im adapter"
```

---

### Task 6: Remove Feishu Adapter And Remaining `internal/im` Package

**Files:**
- Delete: `server/internal/im/channel.go`
- Delete: `server/internal/im/history.go`
- Delete: `server/internal/im/router.go`
- Delete: `server/internal/im/router_test.go`
- Delete: `server/internal/im/feishu/*`
- Modify: `server/internal/shared/config.go`
- Modify: `server/internal/shared/shared_test.go`
- Test: `server/internal/shared`, full server compile

- [ ] **Step 1: Make legacy Feishu config parse-only**

In `server/internal/shared/config.go`, keep these structs only if strict JSON decoding needs them:

```go
type FeishuConfig struct {
	AppID     string `json:"app_id,omitempty"`
	AppSecret string `json:"app_secret,omitempty"`
}
```

Remove `ProjectConfig.IMType`, `ProjectConfig.HasFeishu`, and `validateFeishuConfig`.

In `LoadConfig`, remove:

```go
if err := validateFeishuConfig(path, cfg.Projects); err != nil {
	return nil, err
}
```

Update `validateRemovedLegacyFields` messages to app-only wording:

```go
return fmt.Errorf("parse config %s: projects[].im has been removed; configure App sessions through registry settings", path)
```

and:

```go
return fmt.Errorf("parse config %s: projects[].imFilter has been removed", path)
```

- [ ] **Step 2: Replace shared config tests**

Replace `TestLoadConfig_FeishuSupportsSnakeAndTypoSecretField` with:

```go
func TestLoadConfig_FeishuFieldIsAcceptedAsIgnoredLegacyConfig(t *testing.T) {
	path := writeTempConfig(t, `{
		"projects": [{
			"name": "proj",
			"path": "D:/repo",
			"feishu": {"app_id": "cli_xxx"}
		}]
	}`)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if len(cfg.Projects) != 1 {
		t.Fatalf("projects len = %d, want 1", len(cfg.Projects))
	}
	if cfg.Projects[0].Name != "proj" {
		t.Fatalf("project name = %q, want proj", cfg.Projects[0].Name)
	}
}
```

Add:

```go
func TestLoadConfig_FeishuDoesNotRequireCredentials(t *testing.T) {
	path := writeTempConfig(t, `{
		"projects": [{
			"name": "proj",
			"path": "D:/repo",
			"feishu": {}
		}]
	}`)
	if _, err := LoadConfig(path); err != nil {
		t.Fatalf("LoadConfig() error = %v, want feishu ignored", err)
	}
}
```

- [ ] **Step 3: Delete Feishu and IM packages**

Delete:

```text
server/internal/im/channel.go
server/internal/im/history.go
server/internal/im/router.go
server/internal/im/router_test.go
server/internal/im/feishu/
```

- [ ] **Step 4: Run IM negative search**

Run:

```powershell
rg -n "internal/im|IMRouter|ChatRef|SendTarget|InboundEvent|ParseCommand|HelpCard|imfeishu|imapp|NewRouter|SetIMRouter|HasIMRouter" server --glob '!**/dist/**'
```

Expected: no matches in production code or tests.

- [ ] **Step 5: Run tests**

Run:

```powershell
cd server
go test ./internal/shared ./internal/hub/client ./internal/hub ./internal/registry -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```powershell
git add server/internal/shared server/internal/im server/internal/hub server/internal/registry
git commit -m "refactor(im): delete feishu and im runtime packages"
```

---

### Task 7: Remove IM Metadata From Protocol, App Types, Monitor, And Active Docs

**Files:**
- Modify: `server/internal/protocol/registry.go`
- Modify: `server/internal/protocol/global.go`
- Modify: `server/internal/registry/server.go`
- Modify: `server/internal/registry/server_test.go`
- Modify: `server/internal/hub/hub.go`
- Modify: `server/internal/hub/reporter.go`
- Modify: `server/cmd/wheelmaker-monitor/transport.go`
- Modify: `server/cmd/wheelmaker-monitor/monitor.go`
- Modify: `server/cmd/wheelmaker-monitor/dashboard.go`
- Modify: `app/web/src/types/registry.ts`
- Modify: `README.md`
- Modify: `server/CLAUDE.md`
- Modify: `docs/registry-protocol.md`
- Modify: `docs/session-management-and-sync.zh-CN.md`
- Delete: `docs/feishu-bot.md`
- Test: Registry, monitor, App typecheck

- [ ] **Step 1: Remove `imType` from Go protocol types**

In `server/internal/protocol/registry.go`, remove:

```go
IMType string `json:"imType"`
```

from `ProjectInfo` and `ProjectListItem`.

In `server/internal/protocol/global.go`, remove:

```go
IMType string `json:"imType"`
```

from `SessionOpenPayload`.

- [ ] **Step 2: Remove `imType` population and diff checks**

In `server/internal/hub/hub.go`, remove `IMType: cfgProject.IMType(),` from `collectProjectInfo`.

In `server/internal/hub/reporter.go`, remove `IMType: project.IMType,` from `localReadProjectListPayload`, and change:

```go
projectChanged := previous.ProjectRev != current.ProjectRev || previous.Agent != current.Agent || previous.IMType != current.IMType || previous.Path != current.Path || previous.Online != current.Online
```

to:

```go
projectChanged := previous.ProjectRev != current.ProjectRev || previous.Agent != current.Agent || previous.Path != current.Path || previous.Online != current.Online
```

In `server/internal/registry/server.go`, remove `IMType: p.IMType,` from `ProjectListItem` construction.

- [ ] **Step 3: Remove monitor IM fields and badge**

In `server/cmd/wheelmaker-monitor/transport.go`, `monitor.go`, and related monitor structs, remove `IMType` fields and assignments.

In `server/cmd/wheelmaker-monitor/dashboard.go`, remove the badge expression:

```js
'<span class="badge badge-yellow">' + esc(p.imType || 'none') + '</span>' +
```

Do not replace it with another badge. The project row should show Hub/project identity, online status, agents, git state, and actions only.

- [ ] **Step 4: Remove App `imType` type field**

In `app/web/src/types/registry.ts`, remove:

```ts
imType?: string;
```

No App UI code currently reads `imType`; `npm run tsc:web` should confirm this.

- [ ] **Step 5: Update Registry tests**

In `server/internal/registry/server_test.go`, remove `imType` from all test project payloads and expected project list items. Example replacement:

```go
map[string]any{
	"name": "server",
	"path": "D:/Code/WheelMaker/server",
	"online": true,
	"agent": "codex",
	"agents": []string{"codex", "claude", "copilot"},
	"projectRev": "",
	"git": map[string]any{},
}
```

- [ ] **Step 6: Update active docs**

Make these active doc edits:

- `README.md`: remove Feishu setup/config examples and rewrite product path as `Workspace Web UI / App -> WheelMaker -> agents`.
- `server/CLAUDE.md`: remove `im.Router -- feishu | app`, slash command list, Feishu Bot link, and routeMap invariant. Replace architecture diagram with `registry.Reporter -> client.Client -> Session -> AgentInstance`.
- `docs/registry-protocol.md`: remove `chat.send`, `imType`, `imChannel`, and Feishu examples. Keep `session.*`, `registry.session.message`, and `registry.session.updated`.
- `docs/session-management-and-sync.zh-CN.md`: remove references to deleting `route_bindings`; session archive/delete docs should mention `sessions`, history files, artifacts, archives, and tombstones only.
- Delete `docs/feishu-bot.md`.

- [ ] **Step 7: Run active-doc negative search**

Run:

```powershell
rg -n "feishu|Feishu|飞书|chat\\.send|imType|imChannel|route_bindings|routeMap|IMRouter|ChatRef" README.md server\\CLAUDE.md docs\\registry-protocol.md docs\\session-management-and-sync.zh-CN.md app\\web\\src server --glob '!**/dist/**'
```

Expected: no matches except:

- `server/internal/shared/config.go` may still contain parse-only `FeishuConfig`.
- `server/internal/shared/shared_test.go` may contain tests proving legacy `feishu` config is ignored.

- [ ] **Step 8: Run tests and typecheck**

Run:

```powershell
cd server
go test ./... -count=1
```

Run:

```powershell
cd app
npm run tsc:web
```

Expected: both pass.

- [ ] **Step 9: Commit**

```powershell
git add README.md server app docs
git commit -m "docs(protocol): remove im metadata and feishu docs"
```

---

### Task 8: Final Cleanliness Verification

**Files:**
- Verify: repository-wide active code and docs

- [ ] **Step 1: Run full server tests**

Run:

```powershell
cd server
go test ./... -count=1
```

Expected: PASS.

- [ ] **Step 2: Run App typecheck**

Run:

```powershell
cd app
npm run tsc:web
```

Expected: PASS.

- [ ] **Step 3: Run clean deletion search**

Run:

```powershell
rg -n "internal/im|IMRouter|ChatRef|SendTarget|InboundEvent|ParseCommand|HelpCard|chat\\.send|routeMap|LoadRouteBindings|SaveRouteBinding|DeleteRouteBinding|imType|imChannel|feishu|Feishu|飞书" server app\\web\\src README.md docs\\registry-protocol.md docs\\session-management-and-sync.zh-CN.md server\\CLAUDE.md --glob '!**/dist/**'
```

Expected: no matches except parse-only legacy config acceptance in `server/internal/shared/config.go` and corresponding tests in `server/internal/shared/shared_test.go`.

- [ ] **Step 4: Check deleted paths**

Run:

```powershell
Test-Path server\\internal\\im
Test-Path docs\\feishu-bot.md
```

Expected output:

```text
False
False
```

- [ ] **Step 5: Run git status**

Run:

```powershell
git status --short
```

Expected: only intended changes from this refactor are present. Do not revert unrelated user-owned changes.

- [ ] **Step 6: Final commit and push**

If the implementation tasks were not committed individually, commit the complete result:

```powershell
git add -A
git commit -m "refactor: delete im runtime"
git push origin HEAD
```

Expected: push succeeds.

If individual task commits already exist, run:

```powershell
git status --short
git push origin HEAD
```

Expected: working tree is clean before push; push succeeds.

---

## Self-Review

- Spec coverage: The plan covers App-only `session.send`, recorder-only outbound messages, typed command deletion, `chat.send` deletion, app IM adapter deletion, Feishu adapter deletion, route binding runtime deletion, `imType` metadata deletion, active docs cleanup, and final negative searches.
- Red-flag scan: The plan has no unresolved markers, no open implementation slots, and no generic "add tests" instructions without concrete commands or assertions.
- Type consistency: The target chain is consistently `projectId + sessionId -> Client.SessionByID -> Session.handlePromptBlocks -> SessionRecorder -> registry.session.* -> App`.
