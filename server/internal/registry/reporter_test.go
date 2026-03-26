package registry

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestReporterRun_RegistersAndServesFSRequests(t *testing.T) {
	s := New(Config{})
	ts := newTestServer(t, s)

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "hello.txt"), []byte("hello registry"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := NewReporter(ReporterConfig{
		Server:            ts,
		HubID:             "hub-test",
		ReconnectInterval: 50 * time.Millisecond,
	}, []ProjectInfo{
		{ID: "p1", Name: "proj1", Path: root},
	})

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
		Payload: map[string]any{
			"path":  ".",
			"limit": 50,
		},
	})
	listResp := mustReadEnvelope(t, app)
	if listResp.Type != "response" || listResp.Method != "fs.list" {
		t.Fatalf("unexpected list response: %#v", listResp)
	}

	entries, ok := listResp.Payload["entries"].([]any)
	if !ok || len(entries) == 0 {
		t.Fatalf("entries=%v, want non-empty", listResp.Payload["entries"])
	}

	mustWriteJSON(t, app, testEnvelope{
		Version:   "1.0",
		RequestID: "req-read",
		Type:      "request",
		Method:    "fs.read",
		ProjectID: "p1",
		Payload: map[string]any{
			"path":   "hello.txt",
			"offset": 0,
			"limit":  1024,
		},
	})
	readResp := mustReadEnvelope(t, app)
	if readResp.Type != "response" || readResp.Method != "fs.read" {
		t.Fatalf("unexpected read response: %#v", readResp)
	}
	if readResp.Payload["content"] != "hello registry" {
		t.Fatalf("content=%v, want hello registry", readResp.Payload["content"])
	}
}

func TestReporterRunReturnsOnContextCancel(t *testing.T) {
	s := New(Config{})
	ts := newTestServer(t, s)

	ctx, cancel := context.WithCancel(context.Background())
	r := NewReporter(ReporterConfig{
		Server:            ts,
		HubID:             "hub-cancel",
		ReconnectInterval: 30 * time.Millisecond,
	}, []ProjectInfo{{ID: "p1", Name: "server", Path: t.TempDir()}})

	done := make(chan error, 1)
	go func() {
		done <- r.Run(ctx)
	}()
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

func TestBuildWSURLDefaults(t *testing.T) {
	got, err := buildWSURL("", 9630)
	if err != nil {
		t.Fatalf("buildWSURL() err = %v", err)
	}
	want := "ws://127.0.0.1:9630/ws"
	if got != want {
		t.Fatalf("buildWSURL()=%q, want %q", got, want)
	}
}

func TestBuildWSURLHostAndPort(t *testing.T) {
	got, err := buildWSURL("10.0.0.8", 9001)
	if err != nil {
		t.Fatalf("buildWSURL() err = %v", err)
	}
	if got != "ws://10.0.0.8:9001/ws" {
		t.Fatalf("buildWSURL()=%q", got)
	}
}

func TestBuildWSURLAbsoluteURL(t *testing.T) {
	got, err := buildWSURL("http://127.0.0.1:9630", 0)
	if err != nil {
		t.Fatalf("buildWSURL() err = %v", err)
	}
	if got != "ws://127.0.0.1:9630/ws" {
		t.Fatalf("buildWSURL()=%q", got)
	}
}

func TestReporterAuth(t *testing.T) {
	s := New(Config{Token: "token-1"})
	ts := newTestServer(t, s)

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
	defer func() {
		cancel()
		<-done
	}()

	waitForProjectOnline(t, ts, "p1", "token-1")
}

func newTestServer(t *testing.T, s *Server) string {
	t.Helper()
	ln, err := startHTTPServer(s.Handler())
	if err != nil {
		t.Fatalf("start http server: %v", err)
	}
	t.Cleanup(func() { _ = ln.close() })
	return ln.addr
}

func waitForProjectOnline(t *testing.T, addr, projectID, token string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		ws := dialWS(t, "http://"+addr+"/ws")
		if strings.TrimSpace(token) != "" {
			mustWriteJSON(t, ws, testEnvelope{
				Version:   "1.0",
				RequestID: "wait-auth",
				Type:      "request",
				Method:    "auth",
				Payload:   map[string]any{"token": token},
			})
			_ = mustReadEnvelope(t, ws)
		}
		mustWriteJSON(t, ws, testEnvelope{
			Version:   "1.0",
			RequestID: "wait-list",
			Type:      "request",
			Method:    "registry.listProjects",
			Payload:   map[string]any{},
		})
		resp := mustReadEnvelope(t, ws)
		_ = ws.Close()
		hubs, ok := resp.Payload["hubs"].([]any)
		if ok {
			for _, raw := range hubs {
				hub, ok := raw.(map[string]any)
				if !ok {
					continue
				}
				projects, ok := hub["projects"].([]any)
				if !ok {
					continue
				}
				for _, pRaw := range projects {
					p, ok := pRaw.(map[string]any)
					if !ok {
						continue
					}
					id, _ := p["id"].(string)
					if id == projectID {
						return
					}
				}
			}
		}
		time.Sleep(40 * time.Millisecond)
	}
	t.Fatalf("project %q not online before timeout", projectID)
}
