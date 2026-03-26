package registry

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
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

func TestHello(t *testing.T) {
	s := New(Config{})
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)

	ws := dialWS(t, ts.URL+"/ws")
	defer ws.Close()

	mustWriteJSON(t, ws, testEnvelope{
		Version:   "1.0",
		RequestID: "r1",
		Type:      "request",
		Method:    "hello",
		Payload: map[string]any{
			"clientName":      "hub",
			"clientVersion":   "0.1.0",
			"protocolVersion": "1.0",
		},
	})

	resp := mustReadEnvelope(t, ws)
	if resp.Type != "response" || resp.Method != "hello" {
		t.Fatalf("unexpected response: %#v", resp)
	}
	if resp.RequestID != "r1" {
		t.Fatalf("requestId=%q, want r1", resp.RequestID)
	}
	if resp.Payload["protocolVersion"] != "1.0" {
		t.Fatalf("protocolVersion=%v, want 1.0", resp.Payload["protocolVersion"])
	}
}

func TestRegistryReportProjectsThenListProjects(t *testing.T) {
	s := New(Config{})
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)

	ws := dialWS(t, ts.URL+"/ws")
	defer ws.Close()

	mustWriteJSON(t, ws, testEnvelope{
		Version:   "1.0",
		RequestID: "r1",
		Type:      "request",
		Method:    "registry.reportProjects",
		Payload: map[string]any{
			"hubId": "hub-a",
			"projects": []map[string]any{
				{"id": "p1", "name": "server", "path": "D:/Code/WheelMaker/server"},
				{"id": "p2", "name": "app", "path": "D:/Code/WheelMaker/app"},
			},
		},
	})

	reportResp := mustReadEnvelope(t, ws)
	if reportResp.Type != "response" || reportResp.Method != "registry.reportProjects" {
		t.Fatalf("unexpected report response: %#v", reportResp)
	}

	mustWriteJSON(t, ws, testEnvelope{
		Version:   "1.0",
		RequestID: "r2",
		Type:      "request",
		Method:    "project.list",
		Payload:   map[string]any{},
	})
	listResp := mustReadEnvelope(t, ws)
	if listResp.Type != "response" || listResp.Method != "project.list" {
		t.Fatalf("unexpected project.list response: %#v", listResp)
	}
	projects, ok := listResp.Payload["projects"].([]any)
	if !ok || len(projects) != 2 {
		t.Fatalf("projects=%v, want 2 items", listResp.Payload["projects"])
	}

	mustWriteJSON(t, ws, testEnvelope{
		Version:   "1.0",
		RequestID: "r3",
		Type:      "request",
		Method:    "project.listFull",
		Payload: map[string]any{
			"includeStats": true,
		},
	})
	fullResp := mustReadEnvelope(t, ws)
	if fullResp.Type != "response" || fullResp.Method != "project.listFull" {
		t.Fatalf("unexpected project.listFull response: %#v", fullResp)
	}
	fullProjects, ok := fullResp.Payload["projects"].([]any)
	if !ok || len(fullProjects) != 2 {
		t.Fatalf("projects=%v, want 2 items", fullResp.Payload["projects"])
	}
	first, ok := fullProjects[0].(map[string]any)
	if !ok {
		t.Fatalf("unexpected first project type: %T", fullProjects[0])
	}
	if _, ok := first["capabilities"].(map[string]any); !ok {
		t.Fatalf("project.capabilities missing: %v", first)
	}
}

func TestAuthRequired(t *testing.T) {
	s := New(Config{Token: "secret"})
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)

	ws := dialWS(t, ts.URL+"/ws")
	defer ws.Close()

	mustWriteJSON(t, ws, testEnvelope{
		Version:   "1.0",
		RequestID: "r1",
		Type:      "request",
		Method:    "registry.reportProjects",
		Payload: map[string]any{
			"hubId":    "hub-a",
			"projects": []map[string]any{},
		},
	})
	unauthorized := mustReadEnvelope(t, ws)
	if unauthorized.Type != "error" {
		t.Fatalf("unexpected response: %#v", unauthorized)
	}
	if unauthorized.Error["code"] != "UNAUTHORIZED" {
		t.Fatalf("error.code=%v, want UNAUTHORIZED", unauthorized.Error["code"])
	}

	mustWriteJSON(t, ws, testEnvelope{
		Version:   "1.0",
		RequestID: "r2",
		Type:      "request",
		Method:    "auth",
		Payload: map[string]any{
			"token": "secret",
		},
	})
	authResp := mustReadEnvelope(t, ws)
	if authResp.Type != "response" || authResp.Method != "auth" {
		t.Fatalf("unexpected auth response: %#v", authResp)
	}

	mustWriteJSON(t, ws, testEnvelope{
		Version:   "1.0",
		RequestID: "r3",
		Type:      "request",
		Method:    "registry.reportProjects",
		Payload: map[string]any{
			"hubId": "hub-a",
			"projects": []map[string]any{
				{"id": "p1", "name": "server", "path": "D:/Code/WheelMaker/server"},
			},
		},
	})
	reportResp := mustReadEnvelope(t, ws)
	if reportResp.Type != "response" || reportResp.Method != "registry.reportProjects" {
		t.Fatalf("unexpected report response after auth: %#v", reportResp)
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
