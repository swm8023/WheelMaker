package registry

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"
)

func TestReporterReportOnce(t *testing.T) {
	s := New(Config{})
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)

	r := NewReporter(ReporterConfig{
		Server: ts.URL,
		HubID:  "hub-test",
	}, []ProjectInfo{
		{ID: "p1", Name: "server", Path: "/repo/server", IMType: "console", Agent: "claude"},
	})
	if err := r.ReportOnce(context.Background()); err != nil {
		t.Fatalf("ReportOnce() err = %v", err)
	}

	ws := dialWS(t, ts.URL+"/ws")
	defer ws.Close()
	mustWriteJSON(t, ws, testEnvelope{
		Version:   "1.0",
		RequestID: "list1",
		Type:      "request",
		Method:    "registry.listProjects",
		Payload:   map[string]any{},
	})
	resp := mustReadEnvelope(t, ws)
	if resp.Type != "response" {
		t.Fatalf("response type = %q, want response", resp.Type)
	}
	hubs, ok := resp.Payload["hubs"].([]any)
	if !ok || len(hubs) == 0 {
		t.Fatalf("hubs=%v, want non-empty", resp.Payload["hubs"])
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

func TestReporterRunReturnsOnContextCancel(t *testing.T) {
	s := New(Config{})
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)

	ctx, cancel := context.WithCancel(context.Background())
	r := NewReporter(ReporterConfig{
		Server:   ts.URL,
		HubID:    "hub-cancel",
		Interval: 30 * time.Millisecond,
	}, []ProjectInfo{{ID: "p1", Name: "server"}})

	done := make(chan error, 1)
	go func() {
		done <- r.Run(ctx)
	}()
	time.Sleep(40 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run() err = %v", err)
		}
	case <-time.After(300 * time.Millisecond):
		t.Fatal("Run() did not return after cancel")
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
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)

	r := NewReporter(ReporterConfig{
		Server: ts.URL,
		Token:  "token-1",
		HubID:  "hub-auth",
	}, []ProjectInfo{{ID: "p1", Name: "server"}})
	if err := r.ReportOnce(context.Background()); err != nil {
		t.Fatalf("ReportOnce() err = %v", err)
	}

	ws := dialWS(t, ts.URL+"/ws")
	defer ws.Close()
	mustWriteJSON(t, ws, testEnvelope{
		Version:   "1.0",
		RequestID: "a1",
		Type:      "request",
		Method:    "auth",
		Payload:   map[string]any{"token": "token-1"},
	})
	_ = mustReadEnvelope(t, ws)
	mustWriteJSON(t, ws, testEnvelope{
		Version:   "1.0",
		RequestID: "a2",
		Type:      "request",
		Method:    "registry.listProjects",
		Payload:   map[string]any{},
	})
	resp := mustReadEnvelope(t, ws)
	hubs, ok := resp.Payload["hubs"].([]any)
	if !ok || len(hubs) == 0 {
		t.Fatalf("hubs=%v, want non-empty", resp.Payload["hubs"])
	}
}
