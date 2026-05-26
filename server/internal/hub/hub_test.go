package hub

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
	clientpkg "github.com/swm8023/wheelmaker/internal/hub/client"
	"github.com/swm8023/wheelmaker/internal/hub/tools"
	rp "github.com/swm8023/wheelmaker/internal/protocol"
	"github.com/swm8023/wheelmaker/internal/registry"
	logger "github.com/swm8023/wheelmaker/internal/shared"
	"io"
	_ "modernc.org/sqlite"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestBuildClient_FeishuEnablesIMWithoutVersion(t *testing.T) {
	h := New(&logger.AppConfig{}, t.TempDir()+"/db/client.sqlite3")
	c, err := h.buildClient(context.Background(), logger.ProjectConfig{
		Name:   "p",
		Path:   ".",
		Feishu: &logger.FeishuConfig{AppID: "cli_xxx", AppSecret: "yyy"},
	})
	if err != nil {
		t.Fatalf("buildClient: %v", err)
	}
	if !c.HasIMRouter() {
		t.Fatal("expected IM router for feishu config")
	}
	t.Cleanup(func() { _ = c.Close() })
}

func TestBuildClient_AppEnablesIMStub(t *testing.T) {
	h := New(&logger.AppConfig{}, t.TempDir()+"/db/client.sqlite3")
	c, err := h.buildClient(context.Background(), logger.ProjectConfig{
		Name: "p",
		Path: ".",
	})
	if err != nil {
		t.Fatalf("buildClient: %v", err)
	}
	if !c.HasIMRouter() {
		t.Fatal("expected IM router for app config")
	}
	t.Cleanup(func() { _ = c.Close() })
}

func TestBuildClientStartsWithSessionTurnStore(t *testing.T) {
	baseDir := t.TempDir()
	dbPath := filepath.Join(baseDir, "db", "client.sqlite3")
	store, err := clientpkg.NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	ctx := context.Background()
	if err := store.SaveSession(ctx, &clientpkg.SessionRecord{
		ID:              "sess-1",
		ProjectName:     "proj1",
		Status:          clientpkg.SessionActive,
		AgentType:       "codex",
		SessionSyncJSON: `{"latestPersistedTurnIndex":1}`,
		CreatedAt:       time.Date(2026, 5, 13, 10, 0, 0, 0, time.UTC),
		LastActiveAt:    time.Date(2026, 5, 13, 10, 1, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}
	if _, err := clientpkg.WriteSessionTurnFiles(ctx, filepath.Join(baseDir, "db", "session"), "proj1", "sess-1", 1, []string{
		`{"method":"agent_message_chunk","param":{"text":"from-db-session"}}`,
	}); err != nil {
		t.Fatalf("WriteSessionTurnFiles: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close store: %v", err)
	}

	h := New(&logger.AppConfig{}, dbPath)
	c, err := h.buildClient(ctx, logger.ProjectConfig{Name: "proj1", Path: "."})
	if err != nil {
		t.Fatalf("buildClient: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	if _, err := os.Stat(filepath.Join(baseDir, "session")); err != nil && !os.IsNotExist(err) {
		t.Fatalf("stat session root: %v", err)
	}
	payload, err := json.Marshal(map[string]any{"sessionId": "sess-1"})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	resp, err := c.HandleSessionRequest(ctx, "session.read", "proj1", payload)
	if err != nil {
		t.Fatalf("session.read: %v", err)
	}
	raw, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	var body struct {
		Turns []struct {
			Content string `json:"content"`
		} `json:"turns"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(body.Turns) != 1 || !strings.Contains(body.Turns[0].Content, "from-db-session") {
		t.Fatalf("turns=%+v, want db/session turn", body.Turns)
	}
}

func TestBuildClient_RejectsInvalidFeishuConfig(t *testing.T) {
	h := New(&logger.AppConfig{}, t.TempDir()+"/db/client.sqlite3")
	_, err := h.buildClient(context.Background(), logger.ProjectConfig{
		Name:   "p",
		Path:   ".",
		Feishu: &logger.FeishuConfig{AppID: "cli_xxx"},
	})
	if err == nil || !strings.Contains(err.Error(), "invalid feishu config") {
		t.Fatalf("err=%v, want invalid feishu config", err)
	}
}

func TestBuildClient_InvalidFeishuLogsError(t *testing.T) {
	var buf bytes.Buffer
	if err := logger.Setup(logger.LoggerConfig{Level: logger.LevelInfo}); err != nil {
		t.Fatalf("setup logger: %v", err)
	}
	defer logger.Close()
	logger.SetOutput(&buf)
	defer logger.SetOutput(os.Stderr)

	h := New(&logger.AppConfig{}, t.TempDir()+"/db/client.sqlite3")
	_, _ = h.buildClient(context.Background(), logger.ProjectConfig{
		Name:   "p",
		Path:   ".",
		Feishu: &logger.FeishuConfig{AppID: "cli_xxx"},
	})
	if !strings.Contains(buf.String(), "[Hub:p] build client failed") {
		t.Fatalf("missing startup error log: %s", buf.String())
	}
}

func TestStartRejectsSchemaMismatchWithDeleteHint(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "db", "client.sqlite3")

	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatalf("mkdir db dir: %v", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if _, err := db.Exec(`
		CREATE TABLE projects (
			project_name TEXT PRIMARY KEY,
			yolo INTEGER NOT NULL DEFAULT 0,
			agent_state_json TEXT NOT NULL DEFAULT '{}',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)
	`); err != nil {
		_ = db.Close()
		t.Fatalf("create legacy projects table: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}

	h := New(&logger.AppConfig{}, dbPath)
	err = h.Start(context.Background())
	if err == nil {
		t.Fatal("Start() error = nil, want schema mismatch")
	}
	if !strings.Contains(err.Error(), "delete local db directory") {
		t.Fatalf("Start() err = %v, want delete local db directory hint", err)
	}
}

func TestReporterPortRelayHTTPAndWebSocketSmoke(t *testing.T) {
	var seenUserAgent string
	targetUpgrader := websocket.Upgrader{
		CheckOrigin:  func(_ *http.Request) bool { return true },
		Subprotocols: []string{"vite-hmr"},
	}
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenUserAgent = r.UserAgent()
		if r.URL.Path == "/ws" {
			conn, err := targetUpgrader.Upgrade(w, r, nil)
			if err != nil {
				t.Fatalf("target ws upgrade: %v", err)
			}
			defer conn.Close()
			for {
				messageType, payload, err := conn.ReadMessage()
				if err != nil {
					return
				}
				if err := conn.WriteMessage(messageType, append([]byte("echo:"), payload...)); err != nil {
					return
				}
			}
		}
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("hello through relay"))
	}))
	t.Cleanup(target.Close)
	targetURL, err := url.Parse(target.URL)
	if err != nil {
		t.Fatalf("parse target url: %v", err)
	}
	targetHost, targetPort := splitHostPortForTest(t, targetURL.Host)

	reg := registry.New(registry.Config{})
	registryAddr := newRegistryServer(t, reg.Handler())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reporter := NewReporter(ReporterConfig{
		Server:            registryAddr,
		HubID:             "hub-relay-smoke",
		ReconnectInterval: 50 * time.Millisecond,
	}, nil)
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

	waitForRelayHubOnline(t, registryAddr, "hub-relay-smoke")
	relayPort := reservePortForHubTest(t)
	client := dialWS(t, "http://"+registryAddr+"/ws")
	defer client.Close()
	connectClient(t, client, "")
	mustWriteJSON(t, client, testEnvelope{
		RequestID: 2,
		Type:      "request",
		Method:    "relay.enable",
		Payload: map[string]any{
			"listenPort": relayPort,
			"hubId":      "hub-relay-smoke",
			"targetHost": targetHost,
			"targetPort": targetPort,
			"accessCode": "483921",
		},
	})
	enableResp := mustReadEnvelope(t, client)
	if enableResp.Type != "response" || enableResp.Payload["status"] != "Up" {
		t.Fatalf("relay.enable response=%#v, want Up", enableResp)
	}

	relayBase := "http://127.0.0.1:" + strconv.Itoa(relayPort)
	jarClient := &http.Client{CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	unauthResp, err := jarClient.Get(relayBase + "/")
	if err != nil {
		t.Fatalf("unauth relay get: %v", err)
	}
	if unauthResp.StatusCode != http.StatusSeeOther {
		t.Fatalf("unauth status=%d, want 303", unauthResp.StatusCode)
	}
	if got := unauthResp.Header.Get("Location"); got != "/__wheelmaker/relay/login" {
		t.Fatalf("unauth Location=%q, want login page", got)
	}
	_ = unauthResp.Body.Close()

	loginResp, err := jarClient.Post(relayBase+"/__wheelmaker/relay/login", "application/x-www-form-urlencoded", strings.NewReader("code=483921"))
	if err != nil {
		t.Fatalf("login relay: %v", err)
	}
	if loginResp.StatusCode != http.StatusSeeOther {
		t.Fatalf("login status=%d, want 303", loginResp.StatusCode)
	}
	cookies := loginResp.Cookies()
	_ = loginResp.Body.Close()

	req, err := http.NewRequest(http.MethodGet, relayBase+"/", nil)
	if err != nil {
		t.Fatalf("new relay request: %v", err)
	}
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}
	authedResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("authed relay get: %v", err)
	}
	body, _ := io.ReadAll(authedResp.Body)
	_ = authedResp.Body.Close()
	if authedResp.StatusCode != http.StatusOK || string(body) != "hello through relay" {
		t.Fatalf("authed status=%d body=%q", authedResp.StatusCode, string(body))
	}
	if !strings.Contains(seenUserAgent, "Mozilla/5.0") {
		t.Fatalf("target user-agent=%q, want browser-like agent", seenUserAgent)
	}

	wsHeader := http.Header{}
	for _, cookie := range cookies {
		wsHeader.Add("Cookie", cookie.String())
	}
	wsConn, _, err := websocket.DefaultDialer.Dial("ws://127.0.0.1:"+strconv.Itoa(relayPort)+"/ws", wsHeader)
	if err != nil {
		t.Fatalf("dial relay ws: %v", err)
	}
	defer wsConn.Close()
	if err := wsConn.WriteMessage(websocket.TextMessage, []byte("text")); err != nil {
		t.Fatalf("write relay ws text: %v", err)
	}
	messageType, payload, err := wsConn.ReadMessage()
	if err != nil {
		t.Fatalf("read relay ws text: %v", err)
	}
	if messageType != websocket.TextMessage || string(payload) != "echo:text" {
		t.Fatalf("ws text type=%d payload=%q", messageType, string(payload))
	}
	if err := wsConn.WriteMessage(websocket.BinaryMessage, []byte{1, 2, 3}); err != nil {
		t.Fatalf("write relay ws binary: %v", err)
	}
	messageType, payload, err = wsConn.ReadMessage()
	if err != nil {
		t.Fatalf("read relay ws binary: %v", err)
	}
	if messageType != websocket.BinaryMessage || string(payload) != "echo:\x01\x02\x03" {
		t.Fatalf("ws binary type=%d payload=%v", messageType, payload)
	}

	subprotocolDialer := websocket.Dialer{Subprotocols: []string{"vite-hmr"}}
	subprotocolConn, _, err := subprotocolDialer.Dial("ws://127.0.0.1:"+strconv.Itoa(relayPort)+"/ws", wsHeader)
	if err != nil {
		t.Fatalf("dial relay ws subprotocol: %v", err)
	}
	defer subprotocolConn.Close()
	if got := subprotocolConn.Subprotocol(); got != "vite-hmr" {
		t.Fatalf("relay ws subprotocol=%q, want vite-hmr", got)
	}
}

func waitForRelayHubOnline(t *testing.T, addr string, hubID string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		ws := dialWS(t, "http://"+addr+"/ws")
		connectClient(t, ws, "")
		mustWriteJSON(t, ws, testEnvelope{RequestID: 2, Type: "request", Method: "project.list", Payload: map[string]any{}})
		resp := mustReadEnvelope(t, ws)
		_ = ws.Close()
		hubs, _ := resp.Payload["hubs"].([]any)
		for _, raw := range hubs {
			hub, _ := raw.(map[string]any)
			if hub["hubId"] == hubID {
				return
			}
		}
		time.Sleep(40 * time.Millisecond)
	}
	t.Fatalf("hub %q not online before timeout", hubID)
}

func splitHostPortForTest(t *testing.T, hostport string) (string, int) {
	t.Helper()
	host, portText, err := net.SplitHostPort(hostport)
	if err != nil {
		t.Fatalf("split host port: %v", err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatalf("parse port: %v", err)
	}
	return host, port
}

func reservePortForHubTest(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve relay port: %v", err)
	}
	defer ln.Close()
	addr, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("reserved addr=%v is not tcp", ln.Addr())
	}
	return addr.Port
}

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

type stubToolCommandHandler struct {
	mu       sync.Mutex
	method   string
	payload  string
	projects []ProjectInfo
	response any
	err      *tools.CommandError
}

func (s *stubToolCommandHandler) Handle(_ context.Context, method string, payload json.RawMessage) (any, *tools.CommandError) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.method = method
	s.payload = string(payload)
	if s.response == nil {
		s.response = map[string]any{"ok": true}
	}
	return s.response, s.err
}

func (s *stubToolCommandHandler) SetProjects(projects []ProjectInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.projects = append([]ProjectInfo(nil), projects...)
}

func (s *stubToolCommandHandler) snapshot() (string, string, []ProjectInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.method, s.payload, append([]ProjectInfo(nil), s.projects...)
}

func TestReporterRun_RegistersAndServesFSRequests(t *testing.T) {
	ts := newRegistryServer(t, registry.New(registry.Config{}).Handler())

	root := t.TempDir()
	initGitRepo(t, root)
	if err := os.WriteFile(filepath.Join(root, "hello.txt"), []byte("hello registry"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	linkedTarget := filepath.Join(root, "linked-target")
	if err := os.MkdirAll(linkedTarget, 0o755); err != nil {
		t.Fatalf("mkdir linked target: %v", err)
	}
	if err := os.WriteFile(filepath.Join(linkedTarget, "nested.txt"), []byte("nested"), 0o644); err != nil {
		t.Fatalf("write linked target fixture: %v", err)
	}
	createDirLink(t, linkedTarget, filepath.Join(root, "linked-dir"))
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
	assertListEntryKind(t, listResp.Payload, "linked-dir", "dir")

	mustWriteJSON(t, app, testEnvelope{
		RequestID: 3,
		Type:      "request",
		Method:    "fs.read",
		ProjectID: rp.ProjectID("hub-test", "proj1"),
		Payload:   map[string]any{"path": "hello.txt"},
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

func TestReporterRespondsToSessionAttachmentRequests(t *testing.T) {
	addr := newRegistryServer(t, registry.New(registry.Config{}).Handler())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	reporter := NewReporter(ReporterConfig{
		Server:            addr,
		HubID:             "hub-attachment",
		ReconnectInterval: 50 * time.Millisecond,
	}, []ProjectInfo{{Name: "proj1", Path: t.TempDir(), Online: true}})
	handler := &stubSessionHandler{}
	reporter.RegisterSessionHandler(rp.ProjectID("hub-attachment", "proj1"), handler)

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

	waitForProjectOnline(t, addr, "hub-attachment:proj1", "")
	app := dialWS(t, "http://"+addr+"/ws")
	defer app.Close()
	connectClient(t, app, "")

	mustWriteJSON(t, app, testEnvelope{
		RequestID: 2,
		Type:      "request",
		Method:    "session.attachment.start",
		ProjectID: "hub-attachment:proj1",
		Payload: map[string]any{
			"sessionId": "sess-1",
			"name":      "a.txt",
			"mimeType":  "text/plain",
			"size":      1,
		},
	})
	resp := mustReadEnvelope(t, app)
	if resp.Type != "response" || resp.Method != "session.attachment.start" {
		t.Fatalf("unexpected session.attachment.start response: %#v", resp)
	}
	if handler.lastMethod != "session.attachment.start" || !strings.Contains(handler.lastBody, `"name":"a.txt"`) {
		t.Fatalf("handler saw method=%q body=%q, want session.attachment.start payload", handler.lastMethod, handler.lastBody)
	}
}

func TestReporterRespondsToCmdNPMRequests(t *testing.T) {
	upgrader := websocket.Upgrader{}
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
					"hubId":           "hub-cmd-npm",
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

		mustWriteJSON(t, ws, testEnvelope{
			RequestID: 100,
			Type:      "request",
			Method:    "cmd.npm",
			Payload: map[string]any{
				"action": "scan",
				"hubId":  "hub-cmd-npm",
			},
		})
		_ = ws.SetReadDeadline(time.Now().Add(1500 * time.Millisecond))
		respSeen <- mustReadEnvelope(t, ws)
	}))

	t.Cleanup(ts.Close)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	toolHandler := &stubToolCommandHandler{response: map[string]any{"ok": true}}
	reporter := NewReporter(ReporterConfig{
		Server:            strings.TrimPrefix(ts.URL, "http://"),
		HubID:             "hub-cmd-npm",
		ReconnectInterval: 50 * time.Millisecond,
	}, nil)
	reporter.toolHandler = toolHandler

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
	case resp := <-respSeen:
		if resp.Type != "response" || resp.Method != "cmd.npm" {
			t.Fatalf("unexpected cmd.npm response: %#v", resp)
		}
		if resp.Payload["ok"] != true {
			t.Fatalf("payload=%#v, want ok=true", resp.Payload)
		}
		method, payload, _ := toolHandler.snapshot()
		if method != "cmd.npm" || !strings.Contains(payload, `"hubId":"hub-cmd-npm"`) {
			t.Fatalf("tool call method=%q payload=%q", method, payload)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("did not receive cmd.npm response from reporter")
	}
}

func TestReporterRespondsToCmdUpdateRequests(t *testing.T) {
	upgrader := websocket.Upgrader{}
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
					"hubId":           "hub-cmd-update",
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

		mustWriteJSON(t, ws, testEnvelope{
			RequestID: 100,
			Type:      "request",
			Method:    "cmd.update",
			Payload: map[string]any{
				"action": "query",
				"hubId":  "hub-cmd-update",
			},
		})
		_ = ws.SetReadDeadline(time.Now().Add(1500 * time.Millisecond))
		respSeen <- mustReadEnvelope(t, ws)
	}))

	t.Cleanup(ts.Close)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	toolHandler := &stubToolCommandHandler{response: map[string]any{"ok": true, "status": "not_published"}}
	reporter := NewReporter(ReporterConfig{
		Server:            strings.TrimPrefix(ts.URL, "http://"),
		HubID:             "hub-cmd-update",
		ReconnectInterval: 50 * time.Millisecond,
		MonitorBaseDir:    t.TempDir(),
	}, nil)
	reporter.toolHandler = toolHandler

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
	case resp := <-respSeen:
		if resp.Type != "response" || resp.Method != "cmd.update" {
			t.Fatalf("unexpected cmd.update response: %#v", resp)
		}
		if resp.Payload["status"] != "not_published" {
			t.Fatalf("payload=%#v, want status=not_published", resp.Payload)
		}
		method, payload, _ := toolHandler.snapshot()
		if method != "cmd.update" || !strings.Contains(payload, `"hubId":"hub-cmd-update"`) {
			t.Fatalf("tool call method=%q payload=%q", method, payload)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("did not receive cmd.update response from reporter")
	}
}

func TestReporterRespondsToCmdSkillsRequests(t *testing.T) {
	upgrader := websocket.Upgrader{}
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
					"hubId":           "hub-cmd-skills",
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

		mustWriteJSON(t, ws, testEnvelope{
			RequestID: 100,
			Type:      "request",
			Method:    "cmd.skills",
			Payload: map[string]any{
				"action": "scan",
				"hubId":  "hub-cmd-skills",
			},
		})
		_ = ws.SetReadDeadline(time.Now().Add(1500 * time.Millisecond))
		respSeen <- mustReadEnvelope(t, ws)
	}))

	t.Cleanup(ts.Close)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	toolHandler := &stubToolCommandHandler{response: map[string]any{"ok": true}}
	reporter := NewReporter(ReporterConfig{
		Server:            strings.TrimPrefix(ts.URL, "http://"),
		HubID:             "hub-cmd-skills",
		ReconnectInterval: 50 * time.Millisecond,
	}, []ProjectInfo{{Name: "WheelMaker", Path: t.TempDir(), Online: true}})
	reporter.toolHandler = toolHandler

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
	case resp := <-respSeen:
		if resp.Type != "response" || resp.Method != "cmd.skills" {
			t.Fatalf("unexpected cmd.skills response: %#v", resp)
		}
		if resp.Payload["ok"] != true {
			t.Fatalf("payload=%#v, want ok=true", resp.Payload)
		}
		method, payload, projects := toolHandler.snapshot()
		if method != "cmd.skills" || !strings.Contains(payload, `"hubId":"hub-cmd-skills"`) {
			t.Fatalf("tool call method=%q payload=%q", method, payload)
		}
		if len(projects) != 1 || projects[0].Name != "WheelMaker" {
			t.Fatalf("tool projects=%#v", projects)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("did not receive cmd.skills response from reporter")
	}
}

func TestReporterRespondsToSessionSetConfigRequests(t *testing.T) {
	reqSeen := make(chan testEnvelope, 1)
	respSeen := make(chan testEnvelope, 1)
	errSeen := make(chan error, 1)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
		ws, err := upgrader.Upgrade(w, req, nil)
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
			RequestID: 101,
			Type:      "request",
			Method:    "session.setConfig",
			ProjectID: "hub-session:proj1",
			Payload: map[string]any{
				"sessionId": "sess-1",
				"configId":  "model",
				"value":     "gpt-5",
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
		t.Fatal("did not receive session.setConfig request")
	}

	select {
	case err := <-errSeen:
		t.Fatalf("fake registry error: %v", err)
	case resp := <-respSeen:
		if resp.Type != "response" || resp.Method != "session.setConfig" {
			t.Fatalf("unexpected session.setConfig response: %#v", resp)
		}
		if handler.lastMethod != "session.setConfig" || !strings.Contains(handler.lastBody, "\"configId\":\"model\"") {
			t.Fatalf("handler saw method=%q body=%q", handler.lastMethod, handler.lastBody)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("did not receive session.setConfig response from reporter")
	}

}

func TestReporterForwardsSessionDeleteRequests(t *testing.T) {
	reqSeen := make(chan testEnvelope, 1)
	respSeen := make(chan testEnvelope, 1)
	errSeen := make(chan error, 1)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
		ws, err := upgrader.Upgrade(w, req, nil)
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
					"hubId":           "hub-session-del",
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
			RequestID: 102,
			Type:      "request",
			Method:    "session.delete",
			ProjectID: "hub-session-del:proj1",
			Payload: map[string]any{
				"sessionId": "sess-1",
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
		HubID:             "hub-session-del",
		ReconnectInterval: 50 * time.Millisecond,
	}, []ProjectInfo{{Name: "proj1", Path: t.TempDir(), Online: true}})
	handler := &stubSessionHandler{}
	reporter.RegisterSessionHandler(rp.ProjectID("hub-session-del", "proj1"), handler)

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
		t.Fatal("did not receive session.delete request")
	}

	select {
	case err := <-errSeen:
		t.Fatalf("fake registry error: %v", err)
	case resp := <-respSeen:
		if resp.Type != "response" || resp.Method != "session.delete" {
			t.Fatalf("unexpected session.delete response: %#v", resp)
		}
		if resp.Payload["ok"] != true || resp.Payload["sessionId"] != "sess-1" {
			t.Fatalf("session.delete response payload=%#v, want ok sessionId", resp.Payload)
		}
		if handler.lastMethod != "session.delete" || !strings.Contains(handler.lastBody, `"sessionId":"sess-1"`) {
			t.Fatalf("handler saw method=%q body=%q, want session.delete payload", handler.lastMethod, handler.lastBody)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("did not receive session.delete response from reporter")
	}
}

func TestReporterForwardsSessionRenameRequests(t *testing.T) {
	reqSeen := make(chan testEnvelope, 1)
	respSeen := make(chan testEnvelope, 1)
	errSeen := make(chan error, 1)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
		ws, err := upgrader.Upgrade(w, req, nil)
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
					"hubId":           "hub-session-rename",
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
			RequestID: 103,
			Type:      "request",
			Method:    "session.rename",
			ProjectID: "hub-session-rename:proj1",
			Payload: map[string]any{
				"sessionId": "sess-1",
				"title":     "Manual title",
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
		HubID:             "hub-session-rename",
		ReconnectInterval: 50 * time.Millisecond,
	}, []ProjectInfo{{Name: "proj1", Path: t.TempDir(), Online: true}})
	handler := &stubSessionHandler{}
	reporter.RegisterSessionHandler(rp.ProjectID("hub-session-rename", "proj1"), handler)

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
		t.Fatal("did not receive session.rename request")
	}

	select {
	case err := <-errSeen:
		t.Fatalf("fake registry error: %v", err)
	case resp := <-respSeen:
		if resp.Type != "response" || resp.Method != "session.rename" {
			t.Fatalf("unexpected session.rename response: %#v", resp)
		}
		if resp.Payload["ok"] != true || resp.Payload["sessionId"] != "sess-1" {
			t.Fatalf("session.rename response payload=%#v, want ok sessionId", resp.Payload)
		}
		if handler.lastMethod != "session.rename" || !strings.Contains(handler.lastBody, `"title":"Manual title"`) {
			t.Fatalf("handler saw method=%q body=%q, want session.rename payload", handler.lastMethod, handler.lastBody)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("did not receive session.rename response from reporter")
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
		Payload:   map[string]any{"path": "hello.txt"},
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
		Payload:   map[string]any{"path": "hello.txt", "knownHash": readHash},
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
	if got, want := statusResp.Payload["worktreeRev"], collectGitState(root).WorktreeRev; got != want {
		t.Fatalf("git.status worktreeRev=%v, want reported normalized rev %s", got, want)
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

func TestLocalReadEndpointRequiresToken(t *testing.T) {
	r := NewReporter(ReporterConfig{HubID: "hub-local-read-empty-token"}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := r.StartLocalReadEndpoint(ctx); err != nil {
		t.Fatalf("StartLocalReadEndpoint() err = %v", err)
	}
	if candidate := r.LocalReadCandidate(); candidate != nil {
		t.Fatalf("LocalReadCandidate()=%#v, want nil without shared token", candidate)
	}
}

func TestLocalReadEndpointProofReadAndRejectsSession(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "hello.txt"), []byte("hello local read"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	r := NewReporter(ReporterConfig{
		HubID: "hub-local-read",
		Token: "local-token",
	}, []ProjectInfo{{Name: "proj1", Path: root, Online: true, ProjectRev: "p1", Git: rp.ProjectGitState{GitRev: "g1", WorktreeRev: "w1"}}})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := r.StartLocalReadEndpoint(ctx); err != nil {
		t.Fatalf("StartLocalReadEndpoint() err = %v", err)
	}
	candidate := r.LocalReadCandidate()
	if candidate == nil {
		t.Fatal("LocalReadCandidate() nil, want endpoint metadata")
	}
	if !strings.HasPrefix(candidate.URL, "ws://127.0.0.1:") {
		t.Fatalf("candidate URL=%q, want loopback websocket", candidate.URL)
	}

	ws, _, err := websocket.DefaultDialer.Dial(candidate.URL, nil)
	if err != nil {
		t.Fatalf("dial local read: %v", err)
	}
	defer ws.Close()

	mustWriteJSON(t, ws, testEnvelope{
		RequestID: 1,
		Type:      "request",
		Method:    "local_read.proof",
		Payload: map[string]any{
			"endpointId": candidate.EndpointID,
			"nonce":      "nonce-1",
		},
	})
	proofResp := mustReadEnvelope(t, ws)
	if proofResp.Type != "response" || proofResp.Method != "local_read.proof" {
		t.Fatalf("proof response=%#v", proofResp)
	}
	signatureText, _ := proofResp.Payload["signature"].(string)
	signature, err := base64.StdEncoding.DecodeString(signatureText)
	if err != nil {
		t.Fatalf("decode signature: %v", err)
	}
	publicKey, err := base64.StdEncoding.DecodeString(candidate.ProofPublicKey)
	if err != nil {
		t.Fatalf("decode public key: %v", err)
	}
	if !ed25519.Verify(ed25519.PublicKey(publicKey), []byte(candidate.EndpointID+"\nnonce-1"), signature) {
		t.Fatal("proof signature did not verify")
	}

	mustWriteJSON(t, ws, testEnvelope{
		RequestID: 2,
		Type:      "request",
		Method:    "connect.init",
		Payload: map[string]any{
			"clientName":      "wheelmaker-web",
			"clientVersion":   "0.1.0",
			"protocolVersion": rp.DefaultProtocolVersion,
			"role":            "local_read",
			"hubId":           "hub-local-read",
			"token":           "local-token",
		},
	})
	initResp := mustReadEnvelope(t, ws)
	if initResp.Type != "response" || initResp.Method != "connect.init" {
		t.Fatalf("connect.init response=%#v", initResp)
	}

	mustWriteJSON(t, ws, testEnvelope{
		RequestID: 3,
		Type:      "request",
		Method:    "fs.read",
		ProjectID: rp.ProjectID("hub-local-read", "proj1"),
		Payload:   map[string]any{"path": "hello.txt"},
	})
	readResp := mustReadEnvelope(t, ws)
	if readResp.Type != "response" || readResp.Method != "fs.read" {
		t.Fatalf("fs.read response=%#v", readResp)
	}
	if readResp.Payload["content"] != "hello local read" {
		t.Fatalf("content=%v, want hello local read", readResp.Payload["content"])
	}

	mustWriteJSON(t, ws, testEnvelope{
		RequestID: 4,
		Type:      "request",
		Method:    "session.list",
		ProjectID: rp.ProjectID("hub-local-read", "proj1"),
		Payload:   map[string]any{},
	})
	sessionResp := mustReadEnvelope(t, ws)
	if sessionResp.Type != "error" || sessionResp.Method != "session.list" {
		t.Fatalf("session.list response=%#v, want error", sessionResp)
	}
	if sessionResp.Payload["code"] != rp.CodeForbidden {
		t.Fatalf("session.list code=%v, want FORBIDDEN", sessionResp.Payload["code"])
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
	mustWriteJSON(t, monitor, testEnvelope{RequestID: 1, Type: "request", Method: "connect.init", Payload: map[string]any{"clientName": "wm-monitor", "clientVersion": "0.1.0", "protocolVersion": "2.3", "role": "monitor"}})
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
			"protocolVersion": "2.3",
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

func createDirLink(t *testing.T, target string, link string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		cmd := exec.Command("cmd", "/c", "mklink", "/J", link, target)
		if _, err := cmd.CombinedOutput(); err == nil {
			return
		}
		t.Skipf("unable to create junction for test")
		return
	}
	if err := os.Symlink(target, link); err == nil {
		return
	}
	t.Skipf("unable to create directory link for test")
}

func assertListEntryKind(t *testing.T, payload map[string]any, name string, wantKind string) {
	t.Helper()
	entries, ok := payload["entries"].([]any)
	if !ok {
		t.Fatalf("entries missing in fs.list payload: %#v", payload)
	}
	for _, item := range entries {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		entryName, _ := entry["name"].(string)
		if entryName != name {
			continue
		}
		kind, _ := entry["kind"].(string)
		if kind != wantKind {
			t.Fatalf("entry %q kind=%q want %q (payload=%#v)", name, kind, wantKind, payload)
		}
		return
	}
	t.Fatalf("entry %q not found in fs.list payload: %#v", name, payload)
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
