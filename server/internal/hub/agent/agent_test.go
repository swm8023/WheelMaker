package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/swm8023/wheelmaker/internal/protocol"
	logger "github.com/swm8023/wheelmaker/internal/shared"
)

func TestFormatACPLogLine_MinimalShape(t *testing.T) {
	payload := []byte(`{"jsonrpc":"2.0","id":3,"method":"session/prompt","params":{"sessionId":"sess-1","token":"abc","prompt":"hello"}}`)
	line := formatACPLogLine('>', "codex", payload)
	if !strings.HasPrefix(line, "[acp] >[codex] [Req 3 session/prompt] ") {
		t.Fatalf("line=%q", line)
	}
	if strings.Contains(line, "jsonrpc") {
		t.Fatalf("line contains verbose metadata: %q", line)
	}
	if strings.Contains(line, `"token":"abc"`) {
		t.Fatalf("line should redact sensitive fields: %q", line)
	}
}

func TestRedactACPPayload_JSONKeys(t *testing.T) {
	raw := []byte(`{"authorization":"Bearer X","nested":{"token":"abc","password":"p"}}`)
	redacted := redactACPPayload(raw)
	s := string(redacted)
	if strings.Contains(s, "Bearer X") || strings.Contains(s, "abc") || strings.Contains(s, "\"p\"") {
		t.Fatalf("redaction failed: %s", s)
	}
	if !strings.Contains(s, "***") {
		t.Fatalf("expected masked marker: %s", s)
	}
	var obj map[string]any
	if err := json.Unmarshal(redacted, &obj); err != nil {
		t.Fatalf("redacted json invalid: %v", err)
	}
}

func TestRedactACPPayload_Truncate64KB(t *testing.T) {
	base := strings.Repeat("x", acpDebugPayloadMaxBytes+1024)
	raw := []byte(`{"method":"session/prompt","params":{"sessionId":"s","content":"` + base + `"}}`)
	out := redactAndTrimACPPayload(raw)
	if len(out) > acpDebugPayloadMaxBytes {
		t.Fatalf("len=%d, want <=%d", len(out), acpDebugPayloadMaxBytes)
	}
}

func TestLogOutboundACPDebugLine(t *testing.T) {
	debugLog := filepath.Join(t.TempDir(), "hub.debug.log")
	if err := logger.Setup(logger.LoggerConfig{Level: logger.LevelDebug, DebugLogFile: debugLog}); err != nil {
		t.Fatalf("setup logger: %v", err)
	}

	raw := []byte(`{"method":"session/prompt","params":{"sessionId":"sess-1","token":"abc"}}`)
	newACPProcessLogSink("codex").Frame('>', raw)
	logger.Close()

	data, err := os.ReadFile(debugLog)
	if err != nil {
		t.Fatalf("read debug log: %v", err)
	}

	got := string(data)
	if got == "" || !strings.Contains(got, "[acp] >[codex] [Notify session/prompt]") {
		t.Fatalf("unexpected outbound log: %q", got)
	}
	if strings.Contains(got, "abc") {
		t.Fatalf("outbound log should redact payload: %q", got)
	}
}

func TestLogACPStderrLineAsError(t *testing.T) {
	var buf bytes.Buffer
	if err := logger.Setup(logger.LoggerConfig{Level: logger.LevelWarn}); err != nil {
		t.Fatalf("setup logger: %v", err)
	}
	defer logger.Close()
	logger.SetOutput(&buf)
	defer logger.SetOutput(os.Stderr)

	newACPProcessLogSink("codex").StderrLine("panic: worker crashed")
	got := buf.String()
	if !strings.Contains(got, "[acp] ![codex] panic: worker crashed") {
		t.Fatalf("unexpected stderr log: %q", got)
	}
}

func TestFormatACPLogLine_ResponseShape(t *testing.T) {
	payload := []byte(`{"jsonrpc":"2.0","id":7,"result":{"ok":true}}`)
	line := formatACPLogLine('<', "claude", payload)
	if !strings.Contains(line, "[acp] <[claude] [Resp 7]") {
		t.Fatalf("line=%q", line)
	}
	if strings.Contains(line, "jsonrpc") {
		t.Fatalf("line contains verbose metadata: %q", line)
	}
}

func TestFormatACPLogLine_NotifySessionUpdateFilter(t *testing.T) {
	payload := []byte(`{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"session-1234567890","update":{"sessionUpdate":"tool_call_update","status":"completed","title":"Edit file"}}}`)
	line := formatACPLogLine('<', "copilot", payload)
	if !strings.Contains(line, "[acp] <[copilot] [Notify session/update]") {
		t.Fatalf("line=%q", line)
	}
	if !strings.Contains(line, "sessio...7890, tool_call_update {") {
		t.Fatalf("filtered body missing: %q", line)
	}
	if strings.Contains(line, `"sessionId":"session-1234567890"`) {
		t.Fatalf("session/update filter should replace raw params: %q", line)
	}
}

func TestCodexACPProvider_UsesNpxByDefault(t *testing.T) {
	p := NewCodexProvider()
	p.resolveBinary = func(name string, configuredPath string) (string, error) {
		t.Fatalf("resolveBinary should not be called: name=%q configuredPath=%q", name, configuredPath)
		return "", nil
	}
	p.lookPath = func(bin string) (string, error) {
		if bin != "npx" {
			t.Fatalf("lookPath bin=%q, want npx", bin)
		}
		return "/usr/bin/npx", nil
	}

	exe, args, env, err := p.Launch()
	if err != nil {
		t.Fatalf("launch: %v", err)
	}
	if exe != "/usr/bin/npx" {
		t.Fatalf("exe=%q", exe)
	}
	if len(args) == 0 || args[0] != "--yes" {
		t.Fatalf("args=%v", args)
	}
	if len(env) != 0 {
		t.Fatalf("env=%v, want empty", env)
	}
}

func TestClaudeACPProvider_UsesNpxByDefault(t *testing.T) {
	p := NewClaudeProvider()
	p.resolveBinary = func(name string, configuredPath string) (string, error) {
		t.Fatalf("resolveBinary should not be called: name=%q configuredPath=%q", name, configuredPath)
		return "", nil
	}
	p.lookPath = func(bin string) (string, error) {
		if bin != "npx" {
			t.Fatalf("lookPath bin=%q, want npx", bin)
		}
		return "/usr/bin/npx", nil
	}

	exe, args, _, err := p.Launch()
	if err != nil {
		t.Fatalf("launch: %v", err)
	}
	if exe != "/usr/bin/npx" {
		t.Fatalf("exe=%q", exe)
	}
	if len(args) != 2 || args[1] != "@agentclientprotocol/claude-agent-acp" {
		t.Fatalf("args=%v", args)
	}
}

func TestCopilotACPProvider_LaunchArgs(t *testing.T) {
	p := NewCopilotProvider()
	p.resolveBinary = func(name string, configuredPath string) (string, error) {
		if name != "copilot" {
			t.Fatalf("resolveBinary name=%q, want copilot", name)
		}
		if configuredPath != "" {
			t.Fatalf("resolveBinary configuredPath=%q, want empty", configuredPath)
		}
		return "/usr/bin/copilot", nil
	}

	exe, args, env, err := p.Launch()
	if err != nil {
		t.Fatalf("launch: %v", err)
	}
	if exe != "/usr/bin/copilot" {
		t.Fatalf("exe=%q", exe)
	}
	if !reflect.DeepEqual(args, []string{"--acp", "--stdio"}) {
		t.Fatalf("args=%v", args)
	}
	if len(env) != 0 {
		t.Fatalf("env=%v, want empty", env)
	}
}

func TestOpenCodeACPProvider_LaunchArgs(t *testing.T) {
	p := NewOpenCodeProvider()
	p.resolveBinary = func(name string, configuredPath string) (string, error) {
		if name != "opencode" {
			t.Fatalf("resolveBinary name=%q, want opencode", name)
		}
		if configuredPath != "" {
			t.Fatalf("resolveBinary configuredPath=%q, want empty", configuredPath)
		}
		return "/usr/bin/opencode", nil
	}

	exe, args, env, err := p.Launch()
	if err != nil {
		t.Fatalf("launch: %v", err)
	}
	if exe != "/usr/bin/opencode" {
		t.Fatalf("exe=%q", exe)
	}
	if !reflect.DeepEqual(args, []string{"acp"}) {
		t.Fatalf("args=%v", args)
	}
	if len(env) != 0 {
		t.Fatalf("env=%v, want empty", env)
	}
}

func TestCodeBuddyACPProvider_LaunchArgs(t *testing.T) {
	p := NewCodeBuddyProvider()
	p.resolveBinary = func(name string, configuredPath string) (string, error) {
		if name != "codebuddy" {
			t.Fatalf("resolveBinary name=%q, want codebuddy", name)
		}
		if configuredPath != "" {
			t.Fatalf("resolveBinary configuredPath=%q, want empty", configuredPath)
		}
		return "/usr/bin/codebuddy", nil
	}

	exe, args, env, err := p.Launch()
	if err != nil {
		t.Fatalf("launch: %v", err)
	}
	if exe != "/usr/bin/codebuddy" {
		t.Fatalf("exe=%q", exe)
	}
	if !reflect.DeepEqual(args, []string{"--acp"}) {
		t.Fatalf("args=%v", args)
	}
	if len(env) != 0 {
		t.Fatalf("env=%v, want empty", env)
	}
}

func TestParseACPProviderCodexApp(t *testing.T) {
	provider, ok := protocol.ParseACPProvider("codexapp")
	if !ok {
		t.Fatal("ParseACPProvider returned ok=false")
	}
	if provider != protocol.ACPProviderCodexApp {
		t.Fatalf("provider=%q, want %q", provider, protocol.ACPProviderCodexApp)
	}
	if _, ok := protocol.ParseACPProvider("codex-app"); ok {
		t.Fatal("ParseACPProvider accepted legacy codex-app alias")
	}

	for _, name := range protocol.ACPProviderNames() {
		if name == string(protocol.ACPProviderCodexApp) {
			return
		}
	}
	t.Fatalf("ACPProviderNames missing %q: %v", protocol.ACPProviderCodexApp, protocol.ACPProviderNames())
}

func TestCodexAppProviderLaunchUsesAppServerStdio(t *testing.T) {
	p := NewCodexAppProvider()
	p.lookPath = func(bin string) (string, error) {
		if bin != "codex" {
			t.Fatalf("lookPath bin=%q, want codex", bin)
		}
		return "/usr/bin/codex", nil
	}

	exe, args, env, err := p.Launch()
	if err != nil {
		t.Fatalf("launch: %v", err)
	}
	if exe != "/usr/bin/codex" {
		t.Fatalf("exe=%q", exe)
	}
	if !reflect.DeepEqual(args, []string{"app-server", "--listen", "stdio://"}) {
		t.Fatalf("args=%v", args)
	}
	if len(env) != 0 {
		t.Fatalf("env=%v, want empty", env)
	}
}

func TestCodexAppRuntimeRequestMatchesNumberAndStringResponseIDs(t *testing.T) {
	tr := newFakeCodexappTransport()
	rt := newCodexappRuntimeWithTransport(tr)
	t.Cleanup(func() { _ = rt.close() })

	var got appServerModelListResponse
	errCh := make(chan error, 1)
	go func() {
		errCh <- rt.request(context.Background(), "model/list", nil, &got)
	}()

	first := tr.nextSent(t)
	if _, ok := first["jsonrpc"]; ok {
		t.Fatalf("app-server request must not include jsonrpc: %#v", first)
	}
	if first["method"] != "model/list" {
		t.Fatalf("method=%v, want model/list", first["method"])
	}
	if params, ok := first["params"].(map[string]any); !ok || len(params) != 0 {
		t.Fatalf("model/list params=%#v, want empty object", first["params"])
	}
	id, ok := first["id"]
	if !ok {
		t.Fatalf("request missing id: %#v", first)
	}

	if err := tr.emit(map[string]any{
		"id": id,
		"result": map[string]any{
			"models": []map[string]any{{
				"id":                        "gpt-5",
				"name":                      "GPT-5",
				"supportedReasoningEfforts": []string{"low", "high"},
				"defaultReasoningEffort":    "high",
			}},
		},
	}); err != nil {
		t.Fatalf("emit response: %v", err)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("request: %v", err)
	}
	if len(got.Models) != 1 || got.Models[0].ID != "gpt-5" {
		t.Fatalf("models=%#v", got.Models)
	}

	var got2 appServerModelListResponse
	errCh = make(chan error, 1)
	go func() {
		errCh <- rt.request(context.Background(), "model/list", nil, &got2)
	}()
	second := tr.nextSent(t)
	stringID := "req-string"
	if err := tr.emit(map[string]any{
		"id":     stringID,
		"result": map[string]any{"models": []map[string]any{{"id": "ignored"}}},
	}); err != nil {
		t.Fatalf("emit unrelated string response: %v", err)
	}
	select {
	case err := <-errCh:
		t.Fatalf("request completed for unrelated string response: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	if err := tr.emit(map[string]any{
		"id":     second["id"],
		"result": map[string]any{"models": []map[string]any{{"id": "gpt-5-mini"}}},
	}); err != nil {
		t.Fatalf("emit matching response: %v", err)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("request 2: %v", err)
	}
	if len(got2.Models) != 1 || got2.Models[0].ID != "gpt-5-mini" {
		t.Fatalf("models2=%#v", got2.Models)
	}
}

func TestCodexAppRuntimeRoutesNotificationsByThread(t *testing.T) {
	tr := newFakeCodexappTransport()
	rt := newCodexappRuntimeWithTransport(tr)
	t.Cleanup(func() { _ = rt.close() })

	connA := newCodexappConnWithRuntime(rt, t.TempDir())
	connB := newCodexappConnWithRuntime(rt, t.TempDir())
	chA := make(chan protocol.SessionUpdateParams, 1)
	chB := make(chan protocol.SessionUpdateParams, 1)
	connA.OnACPResponse(captureSessionUpdate(t, chA))
	connB.OnACPResponse(captureSessionUpdate(t, chB))
	rt.register("thread-a", connA)
	rt.register("thread-b", connB)

	if err := tr.emit(map[string]any{
		"method": "item/agentMessage/delta",
		"params": map[string]any{"threadId": "thread-b", "turnId": "turn-1", "delta": "hello"},
	}); err != nil {
		t.Fatalf("emit notification: %v", err)
	}

	select {
	case got := <-chB:
		if got.SessionID != "thread-b" || got.Update.SessionUpdate != protocol.SessionUpdateAgentMessageChunk {
			t.Fatalf("unexpected routed update: %#v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("thread-b did not receive routed update")
	}
	select {
	case got := <-chA:
		t.Fatalf("thread-a received cross-thread update: %#v", got)
	case <-time.After(20 * time.Millisecond):
	}
}

func TestCodexAppRuntimeDispatchesNotificationsAsynchronously(t *testing.T) {
	tr := newFakeCodexappTransport()
	rt := newCodexappRuntimeWithTransport(tr)
	t.Cleanup(func() { _ = rt.close() })

	unblock := make(chan struct{})
	defer close(unblock)

	conn := newCodexappConnWithRuntime(rt, t.TempDir())
	conn.OnACPResponse(func(context.Context, string, json.RawMessage) {
		<-unblock
	})
	rt.register("thread-1", conn)

	emitDone := make(chan error, 1)
	go func() {
		emitDone <- tr.emit(map[string]any{
			"method": "item/agentMessage/delta",
			"params": map[string]any{"threadId": "thread-1", "turnId": "turn-1", "delta": "hello"},
		})
	}()

	select {
	case err := <-emitDone:
		if err != nil {
			t.Fatalf("emit notification: %v", err)
		}
	case <-time.After(20 * time.Millisecond):
		t.Fatal("notification dispatch blocked the transport read-loop")
	}
}

func TestCodexAppRuntimeRoutesServerRequestAndRoundTripsStringID(t *testing.T) {
	tr := newFakeCodexappTransport()
	rt := newCodexappRuntimeWithTransport(tr)
	t.Cleanup(func() { _ = rt.close() })

	conn := newCodexappConnWithRuntime(rt, t.TempDir())
	conn.OnACPRequest(func(_ context.Context, _ int64, method string, params json.RawMessage) (any, error) {
		if method != protocol.MethodRequestPermission {
			t.Fatalf("method=%q, want request permission", method)
		}
		var p protocol.PermissionRequestParams
		if err := json.Unmarshal(params, &p); err != nil {
			t.Fatalf("unmarshal permission params: %v", err)
		}
		if p.SessionID != "thread-1" || p.ToolCall.ToolCallID != "item-1" {
			t.Fatalf("permission params=%#v", p)
		}
		return protocol.PermissionResponse{
			Outcome: protocol.PermissionResult{Outcome: "allow_always", OptionID: "allow_always"},
		}, nil
	})
	rt.register("thread-1", conn)

	if err := tr.emit(map[string]any{
		"id":     "approval-req-1",
		"method": "item/commandExecution/requestApproval",
		"params": map[string]any{
			"threadId": "thread-1",
			"turnId":   "turn-1",
			"itemId":   "item-1",
			"command":  "go test ./...",
		},
	}); err != nil {
		t.Fatalf("emit request: %v", err)
	}

	resp := tr.nextSent(t)
	if resp["id"] != "approval-req-1" {
		t.Fatalf("response id=%#v, want original string id", resp["id"])
	}
	result := resp["result"].(map[string]any)
	if result["decision"] != "acceptForSession" {
		t.Fatalf("decision=%#v", result["decision"])
	}
}

func TestCodexAppRuntimeUnsupportedKnownThreadServerRequestReturnsMethodNotFound(t *testing.T) {
	tr := newFakeCodexappTransport()
	rt := newCodexappRuntimeWithTransport(tr)
	t.Cleanup(func() { _ = rt.close() })

	conn := newCodexappConnWithRuntime(rt, t.TempDir())
	rt.register("thread-1", conn)

	if err := tr.emit(map[string]any{
		"id":     "unknown-req-1",
		"method": "session/unsupported",
		"params": map[string]any{"threadId": "thread-1"},
	}); err != nil {
		t.Fatalf("emit request: %v", err)
	}

	resp := tr.nextSent(t)
	if resp["id"] != "unknown-req-1" {
		t.Fatalf("response id=%#v, want original string id", resp["id"])
	}
	errObj, ok := resp["error"].(map[string]any)
	if !ok {
		t.Fatalf("response error=%#v, want object", resp["error"])
	}
	if code := int(errObj["code"].(float64)); code != -32601 {
		t.Fatalf("error code=%d, want -32601", code)
	}
}

func TestCodexAppPromptIgnoresStaleTurnCompletedForDifferentTurnID(t *testing.T) {
	tr := newFakeCodexappTransport()
	rt := newCodexappRuntimeWithTransport(tr)
	t.Cleanup(func() { _ = rt.close() })
	tr.onSend = func(msg map[string]any) {
		if msg["method"] == "turn/start" {
			_ = tr.emit(map[string]any{
				"id": msg["id"],
				"result": map[string]any{
					"turn": map[string]any{"id": "turn-current"},
				},
			})
		}
	}

	conn := newCodexappConnWithRuntime(rt, t.TempDir())
	conn.BindSessionID("thread-1")

	var promptRes protocol.SessionPromptResult
	errCh := make(chan error, 1)
	go func() {
		errCh <- conn.Send(context.Background(), protocol.MethodSessionPrompt, protocol.SessionPromptParams{
			SessionID: "thread-1",
			Prompt:    []protocol.ContentBlock{{Type: protocol.ContentBlockTypeText, Text: "ping"}},
		}, &promptRes)
	}()

	waitForActiveTurn(t, conn, "turn-current")
	if err := tr.emit(map[string]any{
		"method": "turn/completed",
		"params": map[string]any{"threadId": "thread-1", "turnId": "turn-stale", "status": "completed"},
	}); err != nil {
		t.Fatalf("emit stale completion: %v", err)
	}
	select {
	case err := <-errCh:
		t.Fatalf("prompt completed for stale turn: err=%v result=%#v", err, promptRes)
	case <-time.After(20 * time.Millisecond):
	}

	if err := tr.emit(map[string]any{
		"method": "turn/completed",
		"params": map[string]any{"threadId": "thread-1", "turnId": "turn-current", "status": "completed"},
	}); err != nil {
		t.Fatalf("emit current completion: %v", err)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("prompt: %v", err)
	}
	if promptRes.StopReason != protocol.StopReasonEndTurn {
		t.Fatalf("stopReason=%q, want end_turn", promptRes.StopReason)
	}
}

func TestCodexAppPromptCompletesWhenTurnCompletedArrivesBeforeTurnIDStored(t *testing.T) {
	tr := newFakeCodexappTransport()
	rt := newCodexappRuntimeWithTransport(tr)
	t.Cleanup(func() { _ = rt.close() })
	releaseResponse := make(chan struct{})
	tr.onSend = func(msg map[string]any) {
		if msg["method"] == "turn/start" {
			_ = tr.emit(map[string]any{
				"id": msg["id"],
				"result": map[string]any{
					"turn": map[string]any{"id": "turn-fast"},
				},
			})
			_ = tr.emit(map[string]any{
				"method": "turn/completed",
				"params": map[string]any{"threadId": "thread-1", "turnId": "turn-fast", "status": "completed"},
			})
			<-releaseResponse
		}
	}

	conn := newCodexappConnWithRuntime(rt, t.TempDir())
	conn.BindSessionID("thread-1")

	var promptRes protocol.SessionPromptResult
	errCh := make(chan error, 1)
	go func() {
		errCh <- conn.Send(context.Background(), protocol.MethodSessionPrompt, protocol.SessionPromptParams{
			SessionID: "thread-1",
			Prompt:    []protocol.ContentBlock{{Type: protocol.ContentBlockTypeText, Text: "ping"}},
		}, &promptRes)
	}()

	close(releaseResponse)
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("prompt: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("prompt did not complete after early turn/completed")
	}
	if promptRes.StopReason != protocol.StopReasonEndTurn {
		t.Fatalf("stopReason=%q, want end_turn", promptRes.StopReason)
	}
}

func TestCodexAppPromptFiltersStaleStreamingDeltas(t *testing.T) {
	tr := newFakeCodexappTransport()
	rt := newCodexappRuntimeWithTransport(tr)
	t.Cleanup(func() { _ = rt.close() })
	tr.onSend = func(msg map[string]any) {
		if msg["method"] == "turn/start" {
			_ = tr.emit(map[string]any{
				"id": msg["id"],
				"result": map[string]any{
					"turn": map[string]any{"id": "turn-current"},
				},
			})
		}
	}

	conn := newCodexappConnWithRuntime(rt, t.TempDir())
	conn.BindSessionID("thread-1")
	updates := make(chan protocol.SessionUpdateParams, 4)
	conn.OnACPResponse(captureSessionUpdate(t, updates))

	errCh := make(chan error, 1)
	go func() {
		var promptRes protocol.SessionPromptResult
		errCh <- conn.Send(context.Background(), protocol.MethodSessionPrompt, protocol.SessionPromptParams{
			SessionID: "thread-1",
			Prompt:    []protocol.ContentBlock{{Type: protocol.ContentBlockTypeText, Text: "ping"}},
		}, &promptRes)
	}()

	waitForActiveTurn(t, conn, "turn-current")
	if err := tr.emit(map[string]any{
		"method": "item/agentMessage/delta",
		"params": map[string]any{"threadId": "thread-1", "turnId": "turn-stale", "delta": "stale"},
	}); err != nil {
		t.Fatalf("emit stale delta: %v", err)
	}
	if err := tr.emit(map[string]any{
		"method": "item/agentMessage/delta",
		"params": map[string]any{"threadId": "thread-1", "turnId": "turn-current", "delta": "current"},
	}); err != nil {
		t.Fatalf("emit current delta: %v", err)
	}
	deadline := time.After(time.Second)
	var sawCurrent bool
	for !sawCurrent {
		select {
		case update := <-updates:
			if update.Update.SessionUpdate == protocol.SessionUpdateUserMessageChunk {
				continue
			}
			var content protocol.ContentBlock
			if err := json.Unmarshal(update.Update.Content, &content); err != nil {
				t.Fatalf("unmarshal content: %v", err)
			}
			if content.Text == "stale" {
				t.Fatal("stale delta was emitted")
			}
			if content.Text == "current" {
				sawCurrent = true
			}
		case <-deadline:
			t.Fatal("current delta was not emitted")
		}
	}
	if err := tr.emit(map[string]any{
		"method": "turn/completed",
		"params": map[string]any{"threadId": "thread-1", "turnId": "turn-current", "status": "completed"},
	}); err != nil {
		t.Fatalf("emit completion: %v", err)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("prompt: %v", err)
	}
	drain := time.After(50 * time.Millisecond)
	for {
		select {
		case update := <-updates:
			if update.Update.SessionUpdate == protocol.SessionUpdateUserMessageChunk {
				continue
			}
			var content protocol.ContentBlock
			if err := json.Unmarshal(update.Update.Content, &content); err != nil {
				t.Fatalf("unmarshal content: %v", err)
			}
			if content.Text == "stale" {
				t.Fatal("stale delta was emitted")
			}
		case <-drain:
			return
		}
	}
}

func TestCodexAppCancelClearsActivePromptState(t *testing.T) {
	tr := newFakeCodexappTransport()
	rt := newCodexappRuntimeWithTransport(tr)
	t.Cleanup(func() { _ = rt.close() })

	var mu sync.Mutex
	nextTurn := 0
	tr.onSend = func(msg map[string]any) {
		switch msg["method"] {
		case "turn/start":
			mu.Lock()
			nextTurn++
			turnID := "turn-1"
			if nextTurn == 2 {
				turnID = "turn-2"
			}
			mu.Unlock()
			_ = tr.emit(map[string]any{
				"id": msg["id"],
				"result": map[string]any{
					"turn": map[string]any{"id": turnID},
				},
			})
		case "turn/interrupt":
			_ = tr.emit(map[string]any{"id": msg["id"], "result": map[string]any{}})
		}
	}

	conn := newCodexappConnWithRuntime(rt, t.TempDir())
	conn.BindSessionID("thread-1")

	var firstRes protocol.SessionPromptResult
	firstErr := make(chan error, 1)
	go func() {
		firstErr <- conn.Send(context.Background(), protocol.MethodSessionPrompt, protocol.SessionPromptParams{
			SessionID: "thread-1",
			Prompt:    []protocol.ContentBlock{{Type: protocol.ContentBlockTypeText, Text: "first"}},
		}, &firstRes)
	}()

	waitForActiveTurn(t, conn, "turn-1")
	if err := conn.Notify(protocol.MethodSessionCancel, protocol.SessionCancelParams{SessionID: "thread-1"}); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if err := <-firstErr; err != nil {
		t.Fatalf("first prompt: %v", err)
	}
	if firstRes.StopReason != protocol.StopReasonCancelled {
		t.Fatalf("first stopReason=%q, want cancelled", firstRes.StopReason)
	}

	var secondRes protocol.SessionPromptResult
	secondErr := make(chan error, 1)
	go func() {
		secondErr <- conn.Send(context.Background(), protocol.MethodSessionPrompt, protocol.SessionPromptParams{
			SessionID: "thread-1",
			Prompt:    []protocol.ContentBlock{{Type: protocol.ContentBlockTypeText, Text: "second"}},
		}, &secondRes)
	}()

	waitForActiveTurn(t, conn, "turn-2")
	if err := tr.emit(map[string]any{
		"method": "turn/completed",
		"params": map[string]any{"threadId": "thread-1", "turnId": "turn-2", "status": "completed"},
	}); err != nil {
		t.Fatalf("emit second completion: %v", err)
	}
	if err := <-secondErr; err != nil {
		t.Fatalf("second prompt should start after cancel: %v", err)
	}
	if secondRes.StopReason != protocol.StopReasonEndTurn {
		t.Fatalf("second stopReason=%q, want end_turn", secondRes.StopReason)
	}
}

func TestCodexAppCancelInterruptsAfterPromptContextCancelledFirst(t *testing.T) {
	tr := newFakeCodexappTransport()
	rt := newCodexappRuntimeWithTransport(tr)
	t.Cleanup(func() { _ = rt.close() })
	interrupts := make(chan map[string]any, 1)
	tr.onSend = func(msg map[string]any) {
		switch msg["method"] {
		case "turn/start":
			_ = tr.emit(map[string]any{
				"id": msg["id"],
				"result": map[string]any{
					"turn": map[string]any{"id": "turn-1"},
				},
			})
		case "turn/interrupt":
			interrupts <- msg
			_ = tr.emit(map[string]any{"id": msg["id"], "result": map[string]any{}})
		}
	}

	conn := newCodexappConnWithRuntime(rt, t.TempDir())
	conn.BindSessionID("thread-1")
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		var promptRes protocol.SessionPromptResult
		errCh <- conn.Send(ctx, protocol.MethodSessionPrompt, protocol.SessionPromptParams{
			SessionID: "thread-1",
			Prompt:    []protocol.ContentBlock{{Type: protocol.ContentBlockTypeText, Text: "first"}},
		}, &promptRes)
	}()
	waitForActiveTurn(t, conn, "turn-1")

	cancel()
	if err := <-errCh; err == nil {
		t.Fatal("prompt should return context cancellation")
	}
	if err := conn.Notify(protocol.MethodSessionCancel, protocol.SessionCancelParams{SessionID: "thread-1"}); err != nil {
		t.Fatalf("cancel notify: %v", err)
	}
	select {
	case msg := <-interrupts:
		params := msg["params"].(map[string]any)
		if params["turnId"] != "turn-1" {
			t.Fatalf("interrupt params=%#v", params)
		}
	case <-time.After(time.Second):
		t.Fatal("turn/interrupt was not sent")
	}
}

func TestCodexAppRuntimeCloseCompletesActivePrompt(t *testing.T) {
	tr := newFakeCodexappTransport()
	rt := newCodexappRuntimeWithTransport(tr)
	tr.onSend = func(msg map[string]any) {
		if msg["method"] == "turn/start" {
			_ = tr.emit(map[string]any{
				"id": msg["id"],
				"result": map[string]any{
					"turn": map[string]any{"id": "turn-1"},
				},
			})
		}
	}

	conn := newCodexappConnWithRuntime(rt, t.TempDir())
	conn.BindSessionID("thread-1")
	errCh := make(chan error, 1)
	go func() {
		var promptRes protocol.SessionPromptResult
		errCh <- conn.Send(context.Background(), protocol.MethodSessionPrompt, protocol.SessionPromptParams{
			SessionID: "thread-1",
			Prompt:    []protocol.ContentBlock{{Type: protocol.ContentBlockTypeText, Text: "ping"}},
		}, &promptRes)
	}()
	waitForActiveTurn(t, conn, "turn-1")

	if err := rt.close(); err != nil {
		t.Fatalf("close runtime: %v", err)
	}
	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("prompt should fail when runtime closes")
		}
	case <-time.After(time.Second):
		t.Fatal("prompt did not unblock after runtime close")
	}
}

func TestCodexAppModelRefreshResetsMissingSelectedModel(t *testing.T) {
	state := newCodexappConfigState()
	state.setModels([]appServerModel{{ID: "gpt-5"}})
	if err := state.set(protocol.ConfigOptionIDModel, "gpt-5"); err != nil {
		t.Fatalf("set model: %v", err)
	}
	state.setModels([]appServerModel{{ID: "gpt-5-mini", DefaultReasoningEffort: "low", SupportedReasoningEfforts: []string{"low"}}})
	if got := currentConfigValue(state.options(), protocol.ConfigOptionIDModel); got != "gpt-5-mini" {
		t.Fatalf("model=%q, want gpt-5-mini", got)
	}
	if got := currentConfigValue(state.options(), protocol.ConfigOptionIDReasoningEffort); got != "low" {
		t.Fatalf("reasoning=%q, want low", got)
	}
}

func TestCodexAppModelListDecodesAppServerDataShape(t *testing.T) {
	var resp appServerModelListResponse
	if err := json.Unmarshal([]byte(`{
		"data": [{
			"id": "gpt-5.5",
			"displayName": "GPT-5.5",
			"supportedReasoningEfforts": [
				{"reasoningEffort": "low", "description": "Fast"},
				{"reasoningEffort": "high", "description": "Deep"}
			],
			"defaultReasoningEffort": "high",
			"isDefault": true
		}]
	}`), &resp); err != nil {
		t.Fatalf("unmarshal model list: %v", err)
	}
	if len(resp.Models) != 1 {
		t.Fatalf("models=%#v, want one model", resp.Models)
	}
	model := resp.Models[0]
	if model.ID != "gpt-5.5" || model.Name != "GPT-5.5" {
		t.Fatalf("model=%#v, want id and display name", model)
	}
	if !reflect.DeepEqual(model.SupportedReasoningEfforts, []string{"low", "high"}) {
		t.Fatalf("efforts=%#v", model.SupportedReasoningEfforts)
	}
	if model.DefaultReasoningEffort != "high" {
		t.Fatalf("default reasoning=%q", model.DefaultReasoningEffort)
	}
}

func TestCodexAppInstanceBasicChatAndConfigOptions(t *testing.T) {
	tr := newFakeCodexappTransport()
	rt := newCodexappRuntimeWithTransport(tr)
	t.Cleanup(func() { _ = rt.close() })
	tr.onSend = func(msg map[string]any) {
		method, _ := msg["method"].(string)
		id := msg["id"]
		switch method {
		case "initialize":
			params := msg["params"].(map[string]any)
			clientInfo := params["clientInfo"].(map[string]any)
			if version, _ := clientInfo["version"].(string); version == "" {
				t.Errorf("initialize clientInfo missing version: %#v", clientInfo)
			}
			_ = tr.emit(map[string]any{"id": id, "result": map[string]any{}})
		case "initialized":
			if params, ok := msg["params"].(map[string]any); !ok || len(params) != 0 {
				t.Errorf("initialized params=%#v, want empty object", msg["params"])
			}
		case "model/list":
			if params, ok := msg["params"].(map[string]any); !ok || len(params) != 0 {
				t.Errorf("model/list params=%#v, want empty object", msg["params"])
			}
			_ = tr.emit(map[string]any{"id": id, "result": map[string]any{
				"models": []map[string]any{{
					"id":                        "gpt-5",
					"name":                      "GPT-5",
					"supportedReasoningEfforts": []string{"low", "medium", "high"},
					"defaultReasoningEffort":    "medium",
				}},
			}})
		case "thread/start":
			params := msg["params"].(map[string]any)
			if params["approvalPolicy"] != "on-request" || params["sandbox"] != "workspace-write" {
				t.Errorf("thread/start params=%#v", params)
			}
			_ = tr.emit(map[string]any{"id": id, "result": map[string]any{
				"thread": map[string]any{"id": "thread-1", "title": "Thread 1", "updatedAt": float64(1778536492)},
			}})
		case "turn/start":
			params := msg["params"].(map[string]any)
			if params["threadId"] != "thread-1" || params["model"] != "gpt-5" || params["effort"] != "high" {
				t.Errorf("turn/start params=%#v", params)
			}
			if _, ok := params["sandbox"]; ok {
				t.Errorf("turn/start must use sandboxPolicy, not sandbox: %#v", params)
			}
			_ = tr.emit(map[string]any{"id": id, "result": map[string]any{
				"turn": map[string]any{"id": "turn-1"},
			}})
			_ = tr.emit(map[string]any{
				"method": "item/agentMessage/delta",
				"params": map[string]any{"threadId": "thread-1", "turnId": "turn-1", "delta": "pong"},
			})
			_ = tr.emit(map[string]any{
				"method": "turn/completed",
				"params": map[string]any{"threadId": "thread-1", "turnId": "turn-1", "status": "completed"},
			})
		default:
			t.Errorf("unexpected app-server method %q", method)
		}
	}

	conn := newCodexappConnWithRuntime(rt, t.TempDir())
	inst := NewInstance("codexapp", conn)
	updates := make(chan protocol.SessionUpdateParams, 4)
	inst.SetCallbacks(&fakeCodexappCallbacks{updates: updates})

	initRes, err := inst.Initialize(context.Background(), protocol.InitializeParams{})
	if err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if initRes.AgentInfo == nil || initRes.AgentInfo.Name != "codexapp" {
		t.Fatalf("agent info=%#v", initRes.AgentInfo)
	}
	if initRes.AgentCapabilities.PromptCapabilities == nil || initRes.AgentCapabilities.PromptCapabilities.Image {
		t.Fatalf("prompt capabilities=%#v", initRes.AgentCapabilities.PromptCapabilities)
	}

	newRes, err := inst.SessionNew(context.Background(), protocol.SessionNewParams{CWD: t.TempDir()})
	if err != nil {
		t.Fatalf("SessionNew: %v", err)
	}
	if newRes.SessionID != "thread-1" {
		t.Fatalf("sessionId=%q", newRes.SessionID)
	}
	if currentConfigValue(newRes.ConfigOptions, protocol.ConfigOptionIDApprovalPreset) != "ask" {
		t.Fatalf("config options missing ask approval preset: %#v", newRes.ConfigOptions)
	}
	opts, err := inst.SessionSetConfigOption(context.Background(), protocol.SessionSetConfigOptionParams{
		SessionID: "thread-1",
		ConfigID:  protocol.ConfigOptionIDReasoningEffort,
		Value:     "high",
	})
	if err != nil {
		t.Fatalf("SessionSetConfigOption: %v", err)
	}
	if currentConfigValue(opts, protocol.ConfigOptionIDReasoningEffort) != "high" {
		t.Fatalf("reasoning option not updated: %#v", opts)
	}

	promptRes, err := inst.SessionPrompt(context.Background(), protocol.SessionPromptParams{
		SessionID: "thread-1",
		Prompt:    []protocol.ContentBlock{{Type: protocol.ContentBlockTypeText, Text: "ping"}},
	})
	if err != nil {
		t.Fatalf("SessionPrompt: %v", err)
	}
	if promptRes.StopReason != protocol.StopReasonEndTurn {
		t.Fatalf("stopReason=%q", promptRes.StopReason)
	}
	deadline := time.After(time.Second)
	for {
		select {
		case update := <-updates:
			if update.SessionID == "thread-1" && update.Update.SessionUpdate == protocol.SessionUpdateAgentMessageChunk {
				return
			}
		case <-deadline:
			t.Fatal("missing agent message update")
		}
	}
}

func TestCodexAppRejectsUnsupportedPhaseOneInputs(t *testing.T) {
	tr := newFakeCodexappTransport()
	rt := newCodexappRuntimeWithTransport(tr)
	t.Cleanup(func() { _ = rt.close() })
	conn := newCodexappConnWithRuntime(rt, t.TempDir())

	var newRes protocol.SessionNewResult
	if err := conn.Send(context.Background(), protocol.MethodSessionNew, protocol.SessionNewParams{
		CWD:        t.TempDir(),
		MCPServers: []protocol.MCPServer{{Name: "fs", Command: "mcp"}},
	}, &newRes); err == nil {
		t.Fatal("SessionNew accepted non-empty MCP servers")
	}

	var promptRes protocol.SessionPromptResult
	if err := conn.Send(context.Background(), protocol.MethodSessionPrompt, protocol.SessionPromptParams{
		SessionID: "thread-1",
		Prompt:    []protocol.ContentBlock{{Type: protocol.ContentBlockTypeImage, URI: "file:///tmp/a.png"}},
	}, &promptRes); err == nil {
		t.Fatal("SessionPrompt accepted image input in phase 1")
	}
}

func TestOwnedConn_SendMatchesResponse(t *testing.T) {
	tr := newFakeOwnedTransport()
	tr.onSend = func(v any) {
		req, ok := v.(protocol.ACPRPCRequest)
		if !ok {
			return
		}
		_ = tr.emit(protocol.ACPRPCResponse{
			JSONRPC: protocol.ACPRPCVersion,
			ID:      req.ID,
			Result:  json.RawMessage(`{"ok":true}`),
		})
	}

	conn := NewOwnedConn(tr)
	t.Cleanup(func() { _ = conn.Close() })

	var out struct {
		OK bool `json:"ok"`
	}
	if err := conn.Send(context.Background(), "test/method", map[string]any{"x": 1}, &out); err != nil {
		t.Fatalf("send: %v", err)
	}
	if !out.OK {
		t.Fatalf("result decode failed: %+v", out)
	}
}

func TestOwnedConn_IncomingRequestDispatchesAndReplies(t *testing.T) {
	tr := newFakeOwnedTransport()
	conn := NewOwnedConn(tr)
	t.Cleanup(func() { _ = conn.Close() })

	conn.OnACPRequest(func(_ context.Context, requestID int64, method string, _ json.RawMessage) (any, error) {
		if requestID != 42 {
			t.Fatalf("requestID=%d, want 42", requestID)
		}
		if method != "session/request_permission" {
			t.Fatalf("method=%q", method)
		}
		return map[string]any{"ok": true}, nil
	})

	if err := tr.emit(protocol.ACPRPCRequest{
		JSONRPC: protocol.ACPRPCVersion,
		ID:      42,
		Method:  "session/request_permission",
		Params:  map[string]any{"sessionId": "s1"},
	}); err != nil {
		t.Fatalf("emit request: %v", err)
	}

	select {
	case sent := <-tr.sent:
		raw, err := json.Marshal(sent)
		if err != nil {
			t.Fatalf("marshal sent response: %v", err)
		}
		var resp struct {
			ID     int64                 `json:"id"`
			Result map[string]any        `json:"result"`
			Error  *protocol.ACPRPCError `json:"error"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			t.Fatalf("unmarshal sent response: %v", err)
		}
		if resp.ID != 42 {
			t.Fatalf("response id=%d, want 42", resp.ID)
		}
		if resp.Error != nil {
			t.Fatalf("unexpected response error: %v", resp.Error)
		}
		if v, ok := resp.Result["ok"].(bool); !ok || !v {
			t.Fatalf("response result=%v", resp.Result)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for response")
	}
}

func TestOwnedConn_NotificationDispatchesResponseCallback(t *testing.T) {
	tr := newFakeOwnedTransport()
	conn := NewOwnedConn(tr)
	t.Cleanup(func() { _ = conn.Close() })

	notified := make(chan struct{}, 1)
	conn.OnACPResponse(func(_ context.Context, method string, _ json.RawMessage) {
		if method == protocol.MethodSessionUpdate {
			notified <- struct{}{}
		}
	})

	if err := tr.emit(protocol.ACPRPCNotification{
		JSONRPC: protocol.ACPRPCVersion,
		Method:  protocol.MethodSessionUpdate,
		Params:  map[string]any{"sessionId": "s1"},
	}); err != nil {
		t.Fatalf("emit notification: %v", err)
	}

	select {
	case <-notified:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for notification dispatch")
	}
}

func TestSharedConnPool_RoutesBySessionID(t *testing.T) {
	raw := &fakeRawConn{}
	shared := NewSharedConnPool(func() (Conn, error) {
		return raw, nil
	})

	r1, err := shared.Open()
	if err != nil {
		t.Fatalf("open route1: %v", err)
	}
	r2, err := shared.Open()
	if err != nil {
		t.Fatalf("open route2: %v", err)
	}
	t.Cleanup(func() {
		_ = r1.Close()
		_ = r2.Close()
		_ = shared.Close()
	})

	count1 := 0
	count2 := 0
	r1.OnACPResponse(func(_ context.Context, _ string, _ json.RawMessage) {
		count1++
	})
	r2.OnACPResponse(func(_ context.Context, _ string, _ json.RawMessage) {
		count2++
	})

	b1, ok := r1.(sessionBinder)
	if !ok {
		t.Fatal("route1 does not support session binder")
	}
	b2, ok := r2.(sessionBinder)
	if !ok {
		t.Fatal("route2 does not support session binder")
	}
	b1.BindSessionID("sid-1")
	b2.BindSessionID("sid-2")

	params, _ := json.Marshal(map[string]any{"sessionId": "sid-2"})
	raw.emitResponse(protocol.MethodSessionUpdate, params)
	if count1 != 0 || count2 != 1 {
		t.Fatalf("counts after sid-2 emit: c1=%d c2=%d", count1, count2)
	}

	unknown, _ := json.Marshal(map[string]any{"sessionId": "unknown"})
	raw.emitResponse(protocol.MethodSessionUpdate, unknown)
	if count1 != 1 || count2 != 1 {
		t.Fatalf("counts after unknown emit: c1=%d c2=%d", count1, count2)
	}
}

func TestRoutes_LoadPendingPromotesToActive(t *testing.T) {
	r := newRouteState()
	tok := r.beginLoad("acp-1", "inst-A", 3)
	if ok := r.commitLoad(tok); !ok {
		t.Fatal("commitLoad returned false")
	}
	got := r.lookupActive("acp-1")
	if got == nil {
		t.Fatal("active route missing")
	}
	if got.instanceKey != "inst-A" || got.epoch != 3 {
		t.Fatalf("active route = %+v", *got)
	}
}

func TestRoutes_LoadFailureRollsBack(t *testing.T) {
	r := newRouteState()
	tok := r.beginLoad("acp-1", "inst-A", 3)
	r.rollbackLoad(tok)
	if got := r.lookupActive("acp-1"); got != nil {
		t.Fatalf("unexpected active route: %+v", *got)
	}
}

func TestRoutes_EpochGuardRejectsStaleCommit(t *testing.T) {
	r := newRouteState()
	fresh := r.beginLoad("acp-1", "inst-new", 4)
	if ok := r.commitLoad(fresh); !ok {
		t.Fatal("fresh commit failed")
	}
	stale := r.beginLoad("acp-1", "inst-old", 2)
	if ok := r.commitLoad(stale); ok {
		t.Fatal("expected stale commit rejection")
	}
	got := r.lookupActive("acp-1")
	if got == nil || got.instanceKey != "inst-new" || got.epoch != 4 {
		t.Fatalf("active route changed by stale commit: %+v", got)
	}
	if got := r.lookupActiveForEpoch("acp-1", 2); got != nil {
		t.Fatalf("stale epoch lookup should fail: %+v", got)
	}
}

func TestRoutes_OrphanReplayAndTTL(t *testing.T) {
	r := newRouteState()
	r.orphanTTL = 1 * time.Second
	t0 := time.Unix(100, 0)

	r.bufferOrphan("acp-1", newUpdate("acp-1", "u1"), t0)
	r.bufferOrphan("acp-1", newUpdate("acp-1", "u2"), t0.Add(500*time.Millisecond))
	r.pruneOrphans(t0.Add(1500 * time.Millisecond))
	r.clock = func() time.Time { return t0.Add(1500 * time.Millisecond) }

	got := r.replayOrphans("acp-1")
	if len(got) != 1 {
		t.Fatalf("replay len=%d, want 1", len(got))
	}
	if got[0].Update.SessionUpdate != "u2" {
		t.Fatalf("replayed update=%q, want u2", got[0].Update.SessionUpdate)
	}
	if gotAgain := r.replayOrphans("acp-1"); len(gotAgain) != 0 {
		t.Fatalf("replay after drain len=%d, want 0", len(gotAgain))
	}
}

func TestInstance_NewAndLoadWithoutACPReady(t *testing.T) {
	fc := &fakeConn{}
	inst := NewInstance("codex", fc)

	newRes, err := inst.SessionNew(context.Background(), protocol.SessionNewParams{CWD: "."})
	if err != nil {
		t.Fatalf("session new: %v", err)
	}
	if newRes.SessionID == "" {
		t.Fatal("expected session id from session/new")
	}

	loadRes, err := inst.SessionLoad(context.Background(), protocol.SessionLoadParams{SessionID: "loaded-1", CWD: "."})
	if err != nil {
		t.Fatalf("session load: %v", err)
	}
	_ = loadRes

	impl := inst.(*instance)
	if !impl.acpSessionReady || impl.acpSessionID != "loaded-1" {
		t.Fatalf("acp session state not updated: ready=%v sid=%q", impl.acpSessionReady, impl.acpSessionID)
	}
}

func TestInstance_HandleInboundDispatch(t *testing.T) {
	fc := &fakeConn{}
	cb := &fakeCallbacks{}
	inst := NewInstance("codex", fc)
	inst.SetCallbacks(cb)
	if fc.resp == nil || fc.req == nil {
		t.Fatal("expected ACP request/response handler registration")
	}

	updateRaw, _ := json.Marshal(protocol.SessionUpdateParams{
		SessionID: "acp-1",
		Update:    protocol.SessionUpdate{SessionUpdate: "agent_message_chunk"},
	})
	fc.resp(context.Background(), protocol.MethodSessionUpdate, updateRaw)
	if cb.updateCount != 1 {
		t.Fatalf("updateCount=%d, want 1", cb.updateCount)
	}

	permRaw, _ := json.Marshal(protocol.PermissionRequestParams{
		SessionID: "acp-1",
		ToolCall:  protocol.ToolCallRef{ToolCallID: "tc-1"},
		Options:   []protocol.PermissionOption{{OptionID: "allow", Name: "Allow", Kind: "once"}},
	})
	resp, err := fc.req(context.Background(), 42, protocol.MethodRequestPermission, permRaw)
	if err != nil {
		t.Fatalf("permission dispatch: %v", err)
	}
	permResp, ok := resp.(protocol.PermissionResponse)
	if !ok {
		t.Fatalf("response type=%T, want protocol.PermissionResponse", resp)
	}
	if permResp.Outcome.Outcome != "allow_once" {
		t.Fatalf("permission outcome=%q", permResp.Outcome.Outcome)
	}
	if cb.permissionCount != 1 {
		t.Fatalf("permissionCount=%d, want 1", cb.permissionCount)
	}
	if cb.lastRequestID != 42 {
		t.Fatalf("lastRequestID=%d, want 42", cb.lastRequestID)
	}

	_ = inst
}

func newUpdate(acpSessionID, name string) protocol.SessionUpdateParams {
	return protocol.SessionUpdateParams{
		SessionID: acpSessionID,
		Update: protocol.SessionUpdate{
			SessionUpdate: name,
		},
	}
}

type fakeConn struct {
	req  ACPRequestHandler
	resp ACPResponseHandler
}

func (f *fakeConn) Send(_ context.Context, method string, _ any, result any) error {
	switch method {
	case protocol.MethodInitialize:
		if out, ok := result.(*protocol.InitializeResult); ok {
			out.ProtocolVersion = json.Number("1")
		}
	case protocol.MethodSessionNew:
		if out, ok := result.(*protocol.SessionNewResult); ok {
			out.SessionID = "new-1"
		}
	case protocol.MethodSessionLoad:
		if out, ok := result.(*protocol.SessionLoadResult); ok {
			out.ConfigOptions = []protocol.ConfigOption{{ID: "mode", CurrentValue: "code"}}
		}
	case protocol.MethodSessionPrompt:
		if out, ok := result.(*protocol.SessionPromptResult); ok {
			out.StopReason = "end_turn"
		}
	}
	return nil
}

func (f *fakeConn) Notify(_ string, _ any) error { return nil }

func (f *fakeConn) OnACPRequest(h ACPRequestHandler) { f.req = h }

func (f *fakeConn) OnACPResponse(h ACPResponseHandler) { f.resp = h }

func (f *fakeConn) Close() error { return nil }

type fakeCallbacks struct {
	updateCount     int
	permissionCount int
	lastRequestID   int64
}

func (f *fakeCallbacks) SessionUpdate(_ protocol.SessionUpdateParams) {
	f.updateCount++
}

func (f *fakeCallbacks) SessionRequestPermission(_ context.Context, requestID int64, _ protocol.PermissionRequestParams) (protocol.PermissionResult, error) {
	f.permissionCount++
	f.lastRequestID = requestID
	return protocol.PermissionResult{Outcome: "allow_once", OptionID: "allow"}, nil
}

type fakeCodexappCallbacks struct {
	updates chan protocol.SessionUpdateParams
}

func (f *fakeCodexappCallbacks) SessionUpdate(p protocol.SessionUpdateParams) {
	f.updates <- p
}

func (f *fakeCodexappCallbacks) SessionRequestPermission(context.Context, int64, protocol.PermissionRequestParams) (protocol.PermissionResult, error) {
	return protocol.PermissionResult{Outcome: "cancelled"}, nil
}

func captureSessionUpdate(t *testing.T, ch chan<- protocol.SessionUpdateParams) ACPResponseHandler {
	t.Helper()
	return func(_ context.Context, method string, params json.RawMessage) {
		if method != protocol.MethodSessionUpdate {
			t.Fatalf("method=%q, want session/update", method)
		}
		var update protocol.SessionUpdateParams
		if err := json.Unmarshal(params, &update); err != nil {
			t.Fatalf("unmarshal update: %v", err)
		}
		ch <- update
	}
}

func currentConfigValue(opts []protocol.ConfigOption, id string) string {
	for _, opt := range opts {
		if opt.ID == id {
			return opt.CurrentValue
		}
	}
	return ""
}

func waitForActiveTurn(t *testing.T, conn *codexappConn, want string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		conn.mu.Lock()
		got := conn.activeTurnID
		conn.mu.Unlock()
		if got == want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	conn.mu.Lock()
	got := conn.activeTurnID
	conn.mu.Unlock()
	t.Fatalf("activeTurnID=%q, want %q", got, want)
}

type fakeRawConn struct {
	req  ACPRequestHandler
	resp ACPResponseHandler
}

func (f *fakeRawConn) Send(_ context.Context, _ string, _ any, _ any) error { return nil }
func (f *fakeRawConn) Notify(_ string, _ any) error                         { return nil }
func (f *fakeRawConn) OnACPRequest(h ACPRequestHandler)                     { f.req = h }
func (f *fakeRawConn) OnACPResponse(h ACPResponseHandler)                   { f.resp = h }
func (f *fakeRawConn) Close() error                                         { return nil }

func (f *fakeRawConn) emitResponse(method string, params []byte) {
	if f.resp == nil {
		return
	}
	f.resp(context.Background(), method, params)
}

type fakeOwnedTransport struct {
	mu sync.RWMutex

	h      func(json.RawMessage)
	onSend func(v any)

	sent chan any
	done chan struct{}
}

func newFakeOwnedTransport() *fakeOwnedTransport {
	return &fakeOwnedTransport{
		sent: make(chan any, 16),
		done: make(chan struct{}),
	}
}

func (f *fakeOwnedTransport) SendMessage(v any) error {
	f.sent <- v
	f.mu.RLock()
	hook := f.onSend
	f.mu.RUnlock()
	if hook != nil {
		hook(v)
	}
	return nil
}

func (f *fakeOwnedTransport) OnMessage(h func(json.RawMessage)) {
	f.mu.Lock()
	f.h = h
	f.mu.Unlock()
}

func (f *fakeOwnedTransport) Done() <-chan struct{} { return f.done }

func (f *fakeOwnedTransport) Close() error {
	select {
	case <-f.done:
	default:
		close(f.done)
	}
	return nil
}

func (f *fakeOwnedTransport) emit(v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	f.mu.RLock()
	h := f.h
	f.mu.RUnlock()
	if h != nil {
		h(raw)
	}
	return nil
}

type fakeCodexappTransport struct {
	mu sync.RWMutex

	h      func(json.RawMessage)
	onSend func(map[string]any)

	sent chan map[string]any
	done chan struct{}
}

func newFakeCodexappTransport() *fakeCodexappTransport {
	return &fakeCodexappTransport{
		sent: make(chan map[string]any, 32),
		done: make(chan struct{}),
	}
}

func (f *fakeCodexappTransport) SendMessage(v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	var msg map[string]any
	if err := json.Unmarshal(raw, &msg); err != nil {
		return err
	}
	f.sent <- msg
	f.mu.RLock()
	hook := f.onSend
	f.mu.RUnlock()
	if hook != nil {
		hook(msg)
	}
	return nil
}

func (f *fakeCodexappTransport) OnMessage(h func(json.RawMessage)) {
	f.mu.Lock()
	f.h = h
	f.mu.Unlock()
}

func (f *fakeCodexappTransport) Done() <-chan struct{} { return f.done }

func (f *fakeCodexappTransport) Alive() bool {
	select {
	case <-f.done:
		return false
	default:
		return true
	}
}

func (f *fakeCodexappTransport) Close() error {
	select {
	case <-f.done:
	default:
		close(f.done)
	}
	return nil
}

func (f *fakeCodexappTransport) emit(v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	f.mu.RLock()
	h := f.h
	f.mu.RUnlock()
	if h != nil {
		h(raw)
	}
	return nil
}

func (f *fakeCodexappTransport) nextSent(t *testing.T) map[string]any {
	t.Helper()
	select {
	case msg := <-f.sent:
		return msg
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for sent app-server message")
		return nil
	}
}

func TestListSkillsForPreset_ProjectDirUsesRelativeDirectoryName(t *testing.T) {
	root := t.TempDir()
	skillDir := filepath.Join(root, ".agents", "skills", "frontend-design")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	skillFile := filepath.Join(skillDir, "SKILL.md")
	content := "---\nname: fancy-ui\ndescription: test\n---\ncontent"
	if err := os.WriteFile(skillFile, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	skills, err := listSkillsForPreset(context.Background(), ACPProviderPreset{
		SkillProjectDirs: []string{".agents/skills"},
	}, root)
	if err != nil {
		t.Fatalf("listSkillsForPreset: %v", err)
	}
	if len(skills) != 1 {
		t.Fatalf("skills len = %d, want 1", len(skills))
	}
	if skills[0].Name != "frontend-design" {
		t.Fatalf("skill name = %q, want %q", skills[0].Name, "frontend-design")
	}
	if !strings.HasSuffix(skills[0].Path, filepath.Join("frontend-design", "SKILL.md")) {
		t.Fatalf("skill path = %q", skills[0].Path)
	}
}

func TestListSkillsForPreset_NestedSkillNameUsesLeafDirectoryName(t *testing.T) {
	root := t.TempDir()
	skillDir := filepath.Join(root, ".agents", "skills", "A", "B")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	skillFile := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(skillFile, []byte("---\nname: ignored\n---\ncontent"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	skills, err := listSkillsForPreset(context.Background(), ACPProviderPreset{
		SkillProjectDirs: []string{".agents/skills"},
	}, root)
	if err != nil {
		t.Fatalf("listSkillsForPreset: %v", err)
	}
	if len(skills) != 1 {
		t.Fatalf("skills len = %d, want 1", len(skills))
	}
	if skills[0].Name != "B" {
		t.Fatalf("skill name = %q, want %q", skills[0].Name, "B")
	}
}
func TestInstanceListSkills_UnknownProvider(t *testing.T) {
	inst := NewInstance("unknown-agent", nil)
	_, err := inst.ListSkills(context.Background(), t.TempDir())
	if err == nil {
		t.Fatal("ListSkills should fail for unknown provider")
	}
}
func TestCodexPreset_IncludesAgentsUserSkillsDir(t *testing.T) {
	found := false
	for _, dir := range CodexACPProviderPreset.SkillUserDirs {
		if strings.EqualFold(strings.TrimSpace(dir), "~/.agents/skills") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("codex preset user dirs missing ~/.agents/skills: %v", CodexACPProviderPreset.SkillUserDirs)
	}
}
