package hub

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	rp "github.com/swm8023/wheelmaker/internal/protocol"
	"github.com/swm8023/wheelmaker/internal/registry"
)

type testEnvelope struct {
	RequestID int64          `json:"requestId,omitempty"`
	Type      string         `json:"type"`
	Method    string         `json:"method,omitempty"`
	ProjectID string         `json:"projectId,omitempty"`
	Payload   map[string]any `json:"payload,omitempty"`
}

type stubChatHandler struct {
	lastMethod string
	lastBody   string
}

func (s *stubChatHandler) HandleChatRequest(_ context.Context, method string, _ string, payload json.RawMessage) (any, error) {
	s.lastMethod = method
	s.lastBody = string(payload)
	return map[string]any{"ok": true}, nil
}

type stubSessionHandler struct {
	lastMethod string
	lastBody   string
}

func (s *stubSessionHandler) HandleSessionRequest(_ context.Context, method string, _ string, payload json.RawMessage) (any, error) {
	s.lastMethod = method
	s.lastBody = string(payload)
	return map[string]any{"ok": true, "sessionId": "sess-1"}, nil
}

func TestReporterRun_RegistersAndServesFSRequests(t *testing.T) {
	ts := newRegistryServer(t, registry.New(registry.Config{}).Handler())

	root := t.TempDir()
	initGitRepo(t, root)
	if err := os.WriteFile(filepath.Join(root, "hello.txt"), []byte("hello registry"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	runGitCmd(t, root, "add", ".")
	runGitCmd(t, root, "commit", "-m", "init")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := NewReporter(ReporterConfig{
		Server:            ts,
		HubID:             "hub-test",
		ReconnectInterval: 50 * time.Millisecond,
	}, []ProjectInfo{{Name: "proj1", Path: root, Online: true}})

	done := make(chan error, 1)
	go func() { done <- r.Run(ctx) }()
	defer func() {
		cancel()
		select {
		case <-done:
		case <-time.After(500 * time.Millisecond):
			t.Fatal("reporter did not stop")
		}
	}()

	waitForProjectOnline(t, ts, rp.ProjectID("hub-test", "proj1"), "")

	app := dialWS(t, "http://"+ts+"/ws")
	defer app.Close()
	connectClient(t, app, "")

	mustWriteJSON(t, app, testEnvelope{
		RequestID: 2,
		Type:      "request",
		Method:    "fs.list",
		ProjectID: rp.ProjectID("hub-test", "proj1"),
		Payload:   map[string]any{"path": ".", "limit": 50},
	})
	listResp := mustReadEnvelope(t, app)
	if listResp.Type != "response" || listResp.Method != "fs.list" {
		t.Fatalf("unexpected list response: %#v", listResp)
	}

	mustWriteJSON(t, app, testEnvelope{
		RequestID: 3,
		Type:      "request",
		Method:    "fs.read",
		ProjectID: rp.ProjectID("hub-test", "proj1"),
		Payload:   map[string]any{"path": "hello.txt", "offset": 0, "limit": 1024},
	})
	readResp := mustReadEnvelope(t, app)
	if readResp.Type != "response" || readResp.Method != "fs.read" {
		t.Fatalf("unexpected read response: %#v", readResp)
	}
	if readResp.Payload["content"] != "hello registry" {
		t.Fatalf("content=%v, want hello registry", readResp.Payload["content"])
	}
}

func TestReporterRespondsToSessionRequests(t *testing.T) {
	upgrader := websocket.Upgrader{}
	reqSeen := make(chan testEnvelope, 1)
	respSeen := make(chan testEnvelope, 1)
	errSeen := make(chan error, 1)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			errSeen <- err
			return
		}
		defer ws.Close()

		initReq := mustReadEnvelope(t, ws)
		if initReq.Method != "connect.init" {
			errSeen <- fmt.Errorf("init method=%q", initReq.Method)
			return
		}
		mustWriteJSON(t, ws, testEnvelope{
			RequestID: initReq.RequestID,
			Type:      "response",
			Method:    "connect.init",
			Payload: map[string]any{
				"ok": true,
				"principal": map[string]any{
					"role":            "hub",
					"hubId":           "hub-session",
					"connectionEpoch": 1,
				},
				"serverInfo": map[string]any{
					"serverVersion":   "test",
					"protocolVersion": rp.DefaultProtocolVersion,
				},
				"features":       map[string]any{},
				"hashAlgorithms": []string{"sha256"},
			},
		})

		reportReq := mustReadEnvelope(t, ws)
		if reportReq.Method != "registry.reportProjects" {
			errSeen <- fmt.Errorf("report method=%q", reportReq.Method)
			return
		}
		mustWriteJSON(t, ws, testEnvelope{
			RequestID: reportReq.RequestID,
			Type:      "response",
			Method:    "registry.reportProjects",
			Payload: map[string]any{
				"ok": true,
			},
		})

		request := testEnvelope{
			RequestID: 100,
			Type:      "request",
			Method:    "session.send",
			ProjectID: "hub-session:proj1",
			Payload: map[string]any{
				"sessionId": "sess-1",
				"text":      "hello session",
			},
		}
		mustWriteJSON(t, ws, request)
		reqSeen <- request

		_ = ws.SetReadDeadline(time.Now().Add(1500 * time.Millisecond))
		respSeen <- mustReadEnvelope(t, ws)
	}))
	t.Cleanup(ts.Close)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reporter := NewReporter(ReporterConfig{
		Server:            strings.TrimPrefix(ts.URL, "http://"),
		HubID:             "hub-session",
		ReconnectInterval: 50 * time.Millisecond,
	}, []ProjectInfo{{Name: "proj1", Path: t.TempDir(), Online: true}})
	handler := &stubSessionHandler{}
	reporter.RegisterSessionHandler(rp.ProjectID("hub-session", "proj1"), handler)

	done := make(chan error, 1)
	go func() { done <- reporter.Run(ctx) }()
	defer func() {
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("reporter did not stop")
		}
	}()

	select {
	case err := <-errSeen:
		t.Fatalf("fake registry error: %v", err)
	case <-reqSeen:
	case <-time.After(2 * time.Second):
		t.Fatal("did not receive session request")
	}

	select {
	case err := <-errSeen:
		t.Fatalf("fake registry error: %v", err)
	case resp := <-respSeen:
		if resp.Type != "response" || resp.Method != "session.send" {
			t.Fatalf("unexpected session.send response: %#v", resp)
		}
		if handler.lastMethod != "session.send" || !strings.Contains(handler.lastBody, "\"sessionId\":\"sess-1\"") {
			t.Fatalf("handler saw method=%q body=%q", handler.lastMethod, handler.lastBody)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("did not receive session.send response from reporter")
	}

}

func TestReporterRunReturnsOnContextCancel(t *testing.T) {
	ts := newRegistryServer(t, registry.New(registry.Config{}).Handler())
	ctx, cancel := context.WithCancel(context.Background())
	r := NewReporter(ReporterConfig{
		Server:            ts,
		HubID:             "hub-cancel",
		ReconnectInterval: 30 * time.Millisecond,
	}, []ProjectInfo{{Name: "server", Path: t.TempDir(), Online: true}})

	done := make(chan error, 1)
	go func() { done <- r.Run(ctx) }()
	time.Sleep(60 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run() err = %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Run() did not return after cancel")
	}
}

func TestReporterAuth(t *testing.T) {
	ts := newRegistryServer(t, registry.New(registry.Config{Token: "token-1"}).Handler())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := NewReporter(ReporterConfig{
		Server:            ts,
		Token:             "token-1",
		HubID:             "hub-auth",
		ReconnectInterval: 30 * time.Millisecond,
	}, []ProjectInfo{{Name: "server", Path: t.TempDir(), Online: true}})
	done := make(chan error, 1)
	go func() { done <- r.Run(ctx) }()
	defer func() { cancel(); <-done }()

	waitForProjectOnline(t, ts, rp.ProjectID("hub-auth", "server"), "token-1")
}

func TestReporterUpdateProjectRefreshesRegistrySnapshot(t *testing.T) {
	ts := newRegistryServer(t, registry.New(registry.Config{}).Handler())

	root := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := NewReporter(ReporterConfig{
		Server:            ts,
		HubID:             "hub-update",
		ReconnectInterval: 50 * time.Millisecond,
	}, []ProjectInfo{{Name: "proj1", Path: root, Online: true, ProjectRev: "p1", Git: rp.ProjectGitState{GitRev: "g1", WorktreeRev: "w1"}}})

	done := make(chan error, 1)
	go func() { done <- r.Run(ctx) }()
	defer func() {
		cancel()
		select {
		case <-done:
		case <-time.After(500 * time.Millisecond):
			t.Fatal("reporter did not stop")
		}
	}()

	waitForProjectOnline(t, ts, rp.ProjectID("hub-update", "proj1"), "")

	if err := r.UpdateProject(ProjectInfo{
		Name:       "proj1",
		Path:       root,
		Online:     true,
		ProjectRev: "p2",
		Git: rp.ProjectGitState{
			GitRev:      "g2",
			WorktreeRev: "w2",
			Dirty:       true,
		},
	}); err != nil {
		t.Fatalf("UpdateProject() err = %v", err)
	}

	app := dialWS(t, "http://"+ts+"/ws")
	defer app.Close()
	connectClient(t, app, "")
	mustWriteJSON(t, app, testEnvelope{
		RequestID: 2,
		Type:      "request",
		Method:    "project.list",
		Payload:   map[string]any{},
	})
	resp := mustReadEnvelope(t, app)
	projects, ok := resp.Payload["projects"].([]any)
	if !ok || len(projects) != 1 {
		t.Fatalf("projects=%v, want 1 item", resp.Payload["projects"])
	}
	project, _ := projects[0].(map[string]any)
	if project["projectRev"] != "p2" {
		t.Fatalf("projectRev=%v, want p2", project["projectRev"])
	}
	gitState, _ := project["git"].(map[string]any)
	if gitState["gitRev"] != "g2" || gitState["dirty"] != true {
		t.Fatalf("git=%v, want updated rev/dirty", gitState)
	}
}

func TestReporterFSHashNegotiationAndGitStatus(t *testing.T) {
	ts := newRegistryServer(t, registry.New(registry.Config{}).Handler())

	root := t.TempDir()
	initGitRepo(t, root)
	if err := os.WriteFile(filepath.Join(root, "hello.txt"), []byte("alpha\nbeta\n"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	runGitCmd(t, root, "add", ".")
	runGitCmd(t, root, "commit", "-m", "init")
	if err := os.WriteFile(filepath.Join(root, "hello.txt"), []byte("alpha\nbeta\ngamma\n"), 0o644); err != nil {
		t.Fatalf("update fixture: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := NewReporter(ReporterConfig{
		Server:            ts,
		HubID:             "hub-hash",
		ReconnectInterval: 50 * time.Millisecond,
	}, []ProjectInfo{{Name: "proj1", Path: root, Online: true}})

	done := make(chan error, 1)
	go func() { done <- r.Run(ctx) }()
	defer func() {
		cancel()
		select {
		case <-done:
		case <-time.After(500 * time.Millisecond):
			t.Fatal("reporter did not stop")
		}
	}()

	waitForProjectOnline(t, ts, rp.ProjectID("hub-hash", "proj1"), "")

	app := dialWS(t, "http://"+ts+"/ws")
	defer app.Close()
	connectClient(t, app, "")

	mustWriteJSON(t, app, testEnvelope{
		RequestID: 2,
		Type:      "request",
		Method:    "fs.list",
		ProjectID: rp.ProjectID("hub-hash", "proj1"),
		Payload:   map[string]any{"path": "."},
	})
	listResp := mustReadEnvelope(t, app)
	listHash, _ := listResp.Payload["hash"].(string)
	if listHash == "" || listResp.Payload["notModified"] != false {
		t.Fatalf("unexpected fs.list payload: %#v", listResp.Payload)
	}

	mustWriteJSON(t, app, testEnvelope{
		RequestID: 3,
		Type:      "request",
		Method:    "fs.list",
		ProjectID: rp.ProjectID("hub-hash", "proj1"),
		Payload:   map[string]any{"path": ".", "knownHash": listHash},
	})
	listCached := mustReadEnvelope(t, app)
	if listCached.Payload["notModified"] != true {
		t.Fatalf("expected notModified fs.list response: %#v", listCached.Payload)
	}

	mustWriteJSON(t, app, testEnvelope{
		RequestID: 4,
		Type:      "request",
		Method:    "fs.read",
		ProjectID: rp.ProjectID("hub-hash", "proj1"),
		Payload:   map[string]any{"path": "hello.txt", "offset": 1, "count": 20},
	})
	readResp := mustReadEnvelope(t, app)
	readHash, _ := readResp.Payload["hash"].(string)
	if readHash == "" || readResp.Payload["notModified"] != false {
		t.Fatalf("unexpected fs.read payload: %#v", readResp.Payload)
	}

	mustWriteJSON(t, app, testEnvelope{
		RequestID: 5,
		Type:      "request",
		Method:    "fs.read",
		ProjectID: rp.ProjectID("hub-hash", "proj1"),
		Payload:   map[string]any{"path": "hello.txt", "knownHash": readHash, "offset": 1, "count": 20},
	})
	readCached := mustReadEnvelope(t, app)
	if readCached.Payload["notModified"] != true {
		t.Fatalf("expected notModified fs.read response: %#v", readCached.Payload)
	}

	mustWriteJSON(t, app, testEnvelope{
		RequestID: 6,
		Type:      "request",
		Method:    "git.status",
		ProjectID: rp.ProjectID("hub-hash", "proj1"),
		Payload:   map[string]any{},
	})
	statusResp := mustReadEnvelope(t, app)
	if statusResp.Payload["dirty"] != true {
		t.Fatalf("expected dirty git.status payload: %#v", statusResp.Payload)
	}
	unstaged, _ := statusResp.Payload["unstaged"].([]any)
	if len(unstaged) == 0 {
		t.Fatalf("expected unstaged entries: %#v", statusResp.Payload)
	}

	mustWriteJSON(t, app, testEnvelope{
		RequestID: 7,
		Type:      "request",
		Method:    "git.workingTree.fileDiff",
		ProjectID: rp.ProjectID("hub-hash", "proj1"),
		Payload:   map[string]any{"path": "hello.txt", "scope": "unstaged", "contextLines": 2},
	})
	diffResp := mustReadEnvelope(t, app)
	diffText, _ := diffResp.Payload["diff"].(string)
	if !strings.Contains(diffText, "+gamma") {
		t.Fatalf("unexpected working tree diff: %q", diffText)
	}
}

func TestReporterRespondsToChatSendRequests(t *testing.T) {
	upgrader := websocket.Upgrader{}
	reqSeen := make(chan testEnvelope, 1)
	respSeen := make(chan testEnvelope, 1)
	errSeen := make(chan error, 1)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			errSeen <- err
			return
		}
		defer ws.Close()

		initReq := mustReadEnvelope(t, ws)
		if initReq.Method != "connect.init" {
			errSeen <- fmt.Errorf("init method=%q", initReq.Method)
			return
		}
		mustWriteJSON(t, ws, testEnvelope{
			RequestID: initReq.RequestID,
			Type:      "response",
			Method:    "connect.init",
			Payload: map[string]any{
				"ok": true,
				"principal": map[string]any{
					"role":            "hub",
					"hubId":           "hub-chat",
					"connectionEpoch": 1,
				},
				"serverInfo": map[string]any{
					"serverVersion":   "test",
					"protocolVersion": rp.DefaultProtocolVersion,
				},
				"features":       map[string]any{},
				"hashAlgorithms": []string{"sha256"},
			},
		})

		reportReq := mustReadEnvelope(t, ws)
		if reportReq.Method != "registry.reportProjects" {
			errSeen <- fmt.Errorf("report method=%q", reportReq.Method)
			return
		}
		mustWriteJSON(t, ws, testEnvelope{
			RequestID: reportReq.RequestID,
			Type:      "response",
			Method:    "registry.reportProjects",
			Payload: map[string]any{
				"ok": true,
			},
		})

		request := testEnvelope{
			RequestID: 100,
			Type:      "request",
			Method:    "chat.send",
			ProjectID: "hub-chat:proj1",
			Payload: map[string]any{
				"chatId": "chat-1",
				"text":   "hello",
			},
		}
		mustWriteJSON(t, ws, request)
		reqSeen <- request

		_ = ws.SetReadDeadline(time.Now().Add(1500 * time.Millisecond))
		respSeen <- mustReadEnvelope(t, ws)
	}))
	t.Cleanup(ts.Close)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reporter := NewReporter(ReporterConfig{
		Server:            strings.TrimPrefix(ts.URL, "http://"),
		HubID:             "hub-chat",
		ReconnectInterval: 50 * time.Millisecond,
	}, []ProjectInfo{{Name: "proj1", Path: t.TempDir(), Online: true}})
	handler := &stubChatHandler{}
	reporter.RegisterChatHandler(rp.ProjectID("hub-chat", "proj1"), handler)

	done := make(chan error, 1)
	go func() { done <- reporter.Run(ctx) }()
	defer func() {
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("reporter did not stop")
		}
	}()

	select {
	case err := <-errSeen:
		t.Fatalf("fake registry error: %v", err)
	case <-reqSeen:
	case <-time.After(2 * time.Second):
		t.Fatal("did not receive chat request")
	}

	select {
	case err := <-errSeen:
		t.Fatalf("fake registry error: %v", err)
	case resp := <-respSeen:
		if resp.Type != "response" || resp.Method != "chat.send" {
			t.Fatalf("unexpected chat.send response: %#v", resp)
		}
		if handler.lastMethod != "chat.send" || !strings.Contains(handler.lastBody, "\"chatId\":\"chat-1\"") {
			t.Fatalf("handler saw method=%q body=%q", handler.lastMethod, handler.lastBody)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("did not receive chat.send response from reporter")
	}
}

func TestBuildWSURLDefaults(t *testing.T) {
	got, err := buildWSURL("", 9630)
	if err != nil || got != "ws://127.0.0.1:9630/ws" {
		t.Fatalf("buildWSURL() = %q, err=%v", got, err)
	}
}

func TestBuildWSURLAbsoluteURL(t *testing.T) {
	got, err := buildWSURL("http://127.0.0.1:9630", 0)
	if err != nil || got != "ws://127.0.0.1:9630/ws" {
		t.Fatalf("buildWSURL() = %q, err=%v", got, err)
	}
}

func TestReporterDebugEnvelope_OneLineWithDirection(t *testing.T) {
	var sb strings.Builder
	r := NewReporter(ReporterConfig{}, nil)
	r.SetDebugLogger(&sb)

	r.writeDebugEnvelope("->", envelope{
		RequestID: 1,
		Type:      "request",
		Method:    "registry.reportProjects",
		Payload:   []byte(`{"hubId":"hub-1","note":"hello\nworld"}`),
	})
	r.writeDebugEnvelope("<-", envelope{
		RequestID: 1,
		Type:      "response",
		Method:    "registry.reportProjects",
		Payload:   []byte(`{"ok":true}`),
	})

	lines := strings.Split(strings.TrimSpace(sb.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("line count=%d, want 2; logs=%q", len(lines), sb.String())
	}
	if !strings.HasPrefix(lines[0], "->[registry] ") {
		t.Fatalf("first line prefix=%q", lines[0])
	}
	if !strings.HasPrefix(lines[1], "<-[registry] ") {
		t.Fatalf("second line prefix=%q", lines[1])
	}
}

func TestReporterRespondsToMonitorStatusRequests(t *testing.T) {
	ts := newRegistryServer(t, registry.New(registry.Config{}).Handler())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := NewReporter(ReporterConfig{Server: ts, HubID: "hub-monitor", ReconnectInterval: 50 * time.Millisecond, MonitorBaseDir: t.TempDir()}, []ProjectInfo{{Name: "proj1", Path: t.TempDir(), Online: true}})
	done := make(chan error, 1)
	go func() { done <- r.Run(ctx) }()
	defer func() {
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("reporter did not stop")
		}
	}()

	waitForProjectOnline(t, ts, rp.ProjectID("hub-monitor", "proj1"), "")

	monitor := dialWS(t, "http://"+ts+"/ws")
	defer monitor.Close()
	mustWriteJSON(t, monitor, testEnvelope{RequestID: 1, Type: "request", Method: "connect.init", Payload: map[string]any{"clientName": "wm-monitor", "clientVersion": "0.1.0", "protocolVersion": "2.2", "role": "monitor"}})
	_ = mustReadEnvelope(t, monitor)

	mustWriteJSON(t, monitor, testEnvelope{RequestID: 2, Type: "request", Method: "monitor.status", Payload: map[string]any{"hubId": "hub-monitor"}})
	resp := mustReadEnvelope(t, monitor)
	if resp.Type != "response" || resp.Method != "monitor.status" {
		t.Fatalf("unexpected monitor.status response: %#v", resp)
	}
	if _, ok := resp.Payload["running"]; !ok {
		t.Fatalf("missing running field: %#v", resp.Payload)
	}
}

func newRegistryServer(t *testing.T, h http.Handler) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := &http.Server{Handler: h}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() {
		_ = srv.Close()
		_ = ln.Close()
	})
	return ln.Addr().String()
}

func connectClient(t *testing.T, ws *websocket.Conn, token string) {
	t.Helper()
	mustWriteJSON(t, ws, testEnvelope{
		RequestID: 1,
		Type:      "request",
		Method:    "connect.init",
		Payload: map[string]any{
			"clientName":      "wm-web",
			"clientVersion":   "0.1.0",
			"protocolVersion": "2.2",
			"role":            "client",
			"token":           token,
		},
	})
	_ = mustReadEnvelope(t, ws)
}

func waitForProjectOnline(t *testing.T, addr, projectID, token string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		ws := dialWS(t, "http://"+addr+"/ws")
		connectClient(t, ws, token)
		mustWriteJSON(t, ws, testEnvelope{
			RequestID: 2, Type: "request", Method: "project.list", Payload: map[string]any{},
		})
		resp := mustReadEnvelope(t, ws)
		_ = ws.Close()
		projects, ok := resp.Payload["projects"].([]any)
		if ok {
			for _, pRaw := range projects {
				p, ok := pRaw.(map[string]any)
				if !ok {
					continue
				}
				if id, _ := p["projectId"].(string); id == projectID {
					return
				}
			}
		}
		time.Sleep(40 * time.Millisecond)
	}
	t.Fatalf("project %q not online before timeout", projectID)
}

func initGitRepo(t *testing.T, dir string) {
	t.Helper()
	runGitCmd(t, dir, "init")
	runGitCmd(t, dir, "config", "user.email", "test@example.com")
	runGitCmd(t, dir, "config", "user.name", "Test User")
}

func runGitCmd(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, string(out))
	}
}

func dialWS(t *testing.T, rawURL string) *websocket.Conn {
	t.Helper()
	wsURL := "ws" + strings.TrimPrefix(rawURL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial ws: %v", err)
	}
	return conn
}

func mustWriteJSON(t *testing.T, ws *websocket.Conn, v any) {
	t.Helper()
	if err := ws.WriteJSON(v); err != nil {
		t.Fatalf("write json: %v", err)
	}
}

func mustReadEnvelope(t *testing.T, ws *websocket.Conn) testEnvelope {
	t.Helper()
	var out testEnvelope
	if err := ws.ReadJSON(&out); err != nil {
		t.Fatalf("read json: %v", err)
	}
	return out
}

func TestMonitorCoreGetStatus(t *testing.T) {
	core := NewMonitorCore(t.TempDir())
	status, err := core.GetServiceStatus()
	if err != nil {
		t.Fatalf("GetServiceStatus: %v", err)
	}
	if status.Timestamp == "" {
		t.Fatalf("timestamp should not be empty")
	}
}

func TestMonitorCoreGetLogs_NormalizesFileAndTail(t *testing.T) {
	base := t.TempDir()
	core := NewMonitorCore(base)
	logPath := filepath.Join(base, "log", "hub.log")
	if err := writeMonitorFile(logPath, "line1\nline2\nline3\n"); err != nil {
		t.Fatalf("write log: %v", err)
	}
	res, err := core.GetLogs("hub", "", 2)
	if err != nil {
		t.Fatalf("GetLogs: %v", err)
	}
	if res.File != "hub" {
		t.Fatalf("file=%q want hub", res.File)
	}
	if res.Total != 2 {
		t.Fatalf("total=%d want 2", res.Total)
	}
	if len(res.Entries) != 2 || res.Entries[0].Message != "line2" || res.Entries[1].Message != "line3" {
		t.Fatalf("unexpected entries: %#v", res.Entries)
	}
}

func TestMonitorCoreGetDBTables_NoDBReturnsErrorResult(t *testing.T) {
	core := NewMonitorCore(t.TempDir())
	res := core.GetDBTables()
	if res.Error == "" {
		t.Fatalf("expected db error when client.sqlite3 missing")
	}
}

func TestMonitorCoreAction_UnsupportedAction(t *testing.T) {
	core := NewMonitorCore(t.TempDir())
	if err := core.ExecuteAction("unknown-action"); err == nil {
		t.Fatalf("expected unsupported action error")
	}
}

func writeMonitorFile(path string, content string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

