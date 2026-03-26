package hub

import (
	"context"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/swm8023/wheelmaker/internal/registry"
)

type testEnvelope struct {
	Version   string         `json:"version"`
	RequestID string         `json:"requestId,omitempty"`
	Type      string         `json:"type"`
	Method    string         `json:"method,omitempty"`
	ProjectID string         `json:"projectId,omitempty"`
	Payload   map[string]any `json:"payload,omitempty"`
	Error     map[string]any `json:"error,omitempty"`
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
	}, []ProjectInfo{{ID: "p1", Name: "proj1", Path: root}})

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

	waitForProjectOnline(t, ts, "p1", "")

	app := dialWS(t, "http://"+ts+"/ws")
	defer app.Close()

	mustWriteJSON(t, app, testEnvelope{
		Version:   "1.0",
		RequestID: "req-list",
		Type:      "request",
		Method:    "fs.list",
		ProjectID: "p1",
		Payload:   map[string]any{"path": ".", "limit": 50},
	})
	listResp := mustReadEnvelope(t, app)
	if listResp.Type != "response" || listResp.Method != "fs.list" {
		t.Fatalf("unexpected list response: %#v", listResp)
	}

	mustWriteJSON(t, app, testEnvelope{
		Version:   "1.0",
		RequestID: "req-read",
		Type:      "request",
		Method:    "fs.read",
		ProjectID: "p1",
		Payload:   map[string]any{"path": "hello.txt", "offset": 0, "limit": 1024},
	})
	readResp := mustReadEnvelope(t, app)
	if readResp.Type != "response" || readResp.Method != "fs.read" {
		t.Fatalf("unexpected read response: %#v", readResp)
	}
	if readResp.Payload["content"] != "hello registry" {
		t.Fatalf("content=%v, want hello registry", readResp.Payload["content"])
	}

	mustWriteJSON(t, app, testEnvelope{
		Version:   "1.0",
		RequestID: "req-git-branches",
		Type:      "request",
		Method:    "git.branches",
		ProjectID: "p1",
		Payload:   map[string]any{},
	})
	branchesResp := mustReadEnvelope(t, app)
	if branchesResp.Type != "response" || branchesResp.Method != "git.branches" {
		t.Fatalf("unexpected git.branches response: %#v", branchesResp)
	}
	branches, ok := branchesResp.Payload["branches"].([]any)
	if !ok || len(branches) < 1 {
		t.Fatalf("branches=%v, want at least 1 branch", branchesResp.Payload["branches"])
	}
}

func TestReporterRunReturnsOnContextCancel(t *testing.T) {
	ts := newRegistryServer(t, registry.New(registry.Config{}).Handler())
	ctx, cancel := context.WithCancel(context.Background())
	r := NewReporter(ReporterConfig{
		Server:            ts,
		HubID:             "hub-cancel",
		ReconnectInterval: 30 * time.Millisecond,
	}, []ProjectInfo{{ID: "p1", Name: "server", Path: t.TempDir()}})

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
	}, []ProjectInfo{{ID: "p1", Name: "server", Path: t.TempDir()}})
	done := make(chan error, 1)
	go func() { done <- r.Run(ctx) }()
	defer func() { cancel(); <-done }()

	waitForProjectOnline(t, ts, "p1", "token-1")
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
		Version:   "1.0",
		RequestID: "req-1",
		Type:      "request",
		Method:    "registry.reportProjects",
		Payload:   []byte(`{"hubId":"hub-1","note":"hello\nworld"}`),
	})
	r.writeDebugEnvelope("<-", envelope{
		Version:   "1.0",
		RequestID: "req-1",
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

func waitForProjectOnline(t *testing.T, addr, projectID, token string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		ws := dialWS(t, "http://"+addr+"/ws")
		if strings.TrimSpace(token) != "" {
			mustWriteJSON(t, ws, testEnvelope{
				Version: "1.0", RequestID: "wait-auth", Type: "request", Method: "auth",
				Payload: map[string]any{"token": token},
			})
			_ = mustReadEnvelope(t, ws)
		}
		mustWriteJSON(t, ws, testEnvelope{
			Version: "1.0", RequestID: "wait-list", Type: "request", Method: "project.list", Payload: map[string]any{},
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
