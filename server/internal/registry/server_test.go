package registry

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
)

type testEnvelope struct {
	RequestID int64          `json:"requestId,omitempty"`
	Type      string         `json:"type"`
	Method    string         `json:"method,omitempty"`
	ProjectID string         `json:"projectId,omitempty"`
	Payload   map[string]any `json:"payload,omitempty"`
}

func TestConnectInit(t *testing.T) {
	s := New(Config{})
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)

	ws := dialWS(t, ts.URL+"/ws")
	defer ws.Close()

	mustWriteJSON(t, ws, testEnvelope{
		RequestID: 1,
		Type:      "request",
		Method:    "connect.init",
		Payload: map[string]any{
			"clientName":      "wm-web",
			"clientVersion":   "0.1.0",
			"protocolVersion": "2.2",
			"role":            "client",
			"token":           "",
		},
	})

	resp := mustReadEnvelope(t, ws)
	if resp.Type != "response" || resp.Method != "connect.init" {
		t.Fatalf("unexpected response: %#v", resp)
	}
	if resp.RequestID != 1 {
		t.Fatalf("requestId=%d, want 1", resp.RequestID)
	}
	if resp.Payload["serverInfo"] == nil {
		t.Fatalf("missing serverInfo: %#v", resp.Payload)
	}
}

func TestRegistryReportProjectsThenListProjects(t *testing.T) {
	s := New(Config{})
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)

	hub := dialWS(t, ts.URL+"/ws")
	defer hub.Close()

	mustWriteJSON(t, hub, testEnvelope{
		RequestID: 1,
		Type:      "request",
		Method:    "connect.init",
		Payload: map[string]any{
			"clientName":      "wm-hub",
			"clientVersion":   "0.1.0",
			"protocolVersion": "2.2",
			"role":            "hub",
			"hubId":           "hub-a",
			"token":           "",
		},
	})
	initResp := mustReadEnvelope(t, hub)
	principal, _ := initResp.Payload["principal"].(map[string]any)
	connectionEpoch, _ := principal["connectionEpoch"].(float64)

	mustWriteJSON(t, hub, testEnvelope{
		RequestID: 2,
		Type:      "request",
		Method:    "registry.reportProjects",
		Payload: map[string]any{
			"hubId":           "hub-a",
			"connectionEpoch": int64(connectionEpoch),
			"projects": []map[string]any{
				{"name": "server", "path": "D:/Code/WheelMaker/server", "online": true, "agent": "codex", "imType": "console", "projectRev": "", "git": map[string]any{}},
				{"name": "app", "path": "D:/Code/WheelMaker/app", "online": true, "agent": "claude", "imType": "feishu", "projectRev": "", "git": map[string]any{}},
			},
		},
	})

	reportResp := mustReadEnvelope(t, hub)
	if reportResp.Type != "response" || reportResp.Method != "registry.reportProjects" {
		t.Fatalf("unexpected report response: %#v", reportResp)
	}

	client := dialWS(t, ts.URL+"/ws")
	defer client.Close()
	mustWriteJSON(t, client, testEnvelope{
		RequestID: 1,
		Type:      "request",
		Method:    "connect.init",
		Payload: map[string]any{
			"clientName":      "wm-web",
			"clientVersion":   "0.1.0",
			"protocolVersion": "2.2",
			"role":            "client",
			"token":           "",
		},
	})
	_ = mustReadEnvelope(t, client)

	mustWriteJSON(t, client, testEnvelope{
		RequestID: 2,
		Type:      "request",
		Method:    "project.list",
		Payload:   map[string]any{},
	})
	listResp := mustReadEnvelope(t, client)
	if listResp.Type != "response" || listResp.Method != "project.list" {
		t.Fatalf("unexpected project.list response: %#v", listResp)
	}
	projects, ok := listResp.Payload["projects"].([]any)
	if !ok || len(projects) != 2 {
		t.Fatalf("projects=%v, want 2 items", listResp.Payload["projects"])
	}
	first, _ := projects[0].(map[string]any)
	if _, ok := first["projectId"].(string); !ok {
		t.Fatalf("projectId missing: %v", first)
	}
}

func TestRegistryReportProjectsRejectsStaleConnectionEpoch(t *testing.T) {
	s := New(Config{})
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)

	hubOld := dialWS(t, ts.URL+"/ws")
	defer hubOld.Close()
	mustWriteJSON(t, hubOld, testEnvelope{
		RequestID: 1,
		Type:      "request",
		Method:    "connect.init",
		Payload: map[string]any{
			"clientName":      "wm-hub-old",
			"clientVersion":   "0.1.0",
			"protocolVersion": "2.2",
			"role":            "hub",
			"hubId":           "hub-a",
		},
	})
	oldInit := mustReadEnvelope(t, hubOld)
	oldPrincipal, _ := oldInit.Payload["principal"].(map[string]any)
	oldEpoch, _ := oldPrincipal["connectionEpoch"].(float64)

	hubNew := dialWS(t, ts.URL+"/ws")
	defer hubNew.Close()
	mustWriteJSON(t, hubNew, testEnvelope{
		RequestID: 1,
		Type:      "request",
		Method:    "connect.init",
		Payload: map[string]any{
			"clientName":      "wm-hub-new",
			"clientVersion":   "0.1.0",
			"protocolVersion": "2.2",
			"role":            "hub",
			"hubId":           "hub-a",
		},
	})
	newInit := mustReadEnvelope(t, hubNew)
	newPrincipal, _ := newInit.Payload["principal"].(map[string]any)
	newEpoch, _ := newPrincipal["connectionEpoch"].(float64)

	mustWriteJSON(t, hubNew, testEnvelope{
		RequestID: 2,
		Type:      "request",
		Method:    "registry.reportProjects",
		Payload: map[string]any{
			"hubId":           "hub-a",
			"connectionEpoch": int64(newEpoch),
			"projects": []map[string]any{
				{"name": "server", "path": "D:/Code/WheelMaker/server", "online": true, "agent": "codex", "imType": "console", "projectRev": "p2", "git": map[string]any{"gitRev": "g2", "worktreeRev": "w2"}},
			},
		},
	})
	_ = mustReadEnvelope(t, hubNew)

	mustWriteJSON(t, hubOld, testEnvelope{
		RequestID: 2,
		Type:      "request",
		Method:    "registry.reportProjects",
		Payload: map[string]any{
			"hubId":           "hub-a",
			"connectionEpoch": int64(oldEpoch),
			"projects": []map[string]any{
				{"name": "server", "path": "D:/Code/WheelMaker/server", "online": true, "agent": "codex", "imType": "console", "projectRev": "p1", "git": map[string]any{"gitRev": "g1", "worktreeRev": "w1"}},
			},
		},
	})
	stale := mustReadEnvelope(t, hubOld)
	if stale.Type != "error" {
		t.Fatalf("stale response type=%q, want error", stale.Type)
	}
	if stale.Payload["code"] != "CONFLICT" {
		t.Fatalf("stale error code=%v, want CONFLICT", stale.Payload["code"])
	}
}

func TestConnectInitAuthRequired(t *testing.T) {
	s := New(Config{Token: "secret"})
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)

	ws := dialWS(t, ts.URL+"/ws")
	defer ws.Close()

	mustWriteJSON(t, ws, testEnvelope{
		RequestID: 1,
		Type:      "request",
		Method:    "connect.init",
		Payload: map[string]any{
			"clientName":      "wm-web",
			"clientVersion":   "0.1.0",
			"protocolVersion": "2.2",
			"role":            "client",
			"token":           "wrong",
		},
	})
	unauthorized := mustReadEnvelope(t, ws)
	if unauthorized.Type != "error" {
		t.Fatalf("unexpected response: %#v", unauthorized)
	}
	payload := unauthorized.Payload
	if payload["code"] != "UNAUTHORIZED" {
		t.Fatalf("error.code=%v, want UNAUTHORIZED", payload["code"])
	}
}

func TestInvalidRequestIDReturnsErrorAndKeepsConnection(t *testing.T) {
	s := New(Config{})
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)

	ws := dialWS(t, ts.URL+"/ws")
	defer ws.Close()

	mustWriteJSON(t, ws, testEnvelope{
		RequestID: 1,
		Type:      "request",
		Method:    "connect.init",
		Payload: map[string]any{
			"clientName":      "wm-web",
			"clientVersion":   "0.1.0",
			"protocolVersion": "2.2",
			"role":            "client",
			"token":           "",
		},
	})
	_ = mustReadEnvelope(t, ws)

	mustWriteJSON(t, ws, map[string]any{
		"requestId": "bad-id",
		"type":      "request",
		"method":    "project.list",
		"payload":   map[string]any{},
	})
	invalid := mustReadEnvelope(t, ws)
	if invalid.Type != "error" {
		t.Fatalf("unexpected invalid requestId response: %#v", invalid)
	}
	if invalid.Payload["code"] != "INVALID_ARGUMENT" {
		t.Fatalf("error.code=%v, want INVALID_ARGUMENT", invalid.Payload["code"])
	}

	mustWriteJSON(t, ws, testEnvelope{
		RequestID: 2,
		Type:      "request",
		Method:    "project.list",
		Payload:   map[string]any{},
	})
	listResp := mustReadEnvelope(t, ws)
	if listResp.Type != "response" || listResp.Method != "project.list" {
		t.Fatalf("unexpected project.list response after invalid requestId: %#v", listResp)
	}
}

func TestBatchForwardsProjectRequests(t *testing.T) {
	s := New(Config{})
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)

	hub := dialWS(t, ts.URL+"/ws")
	defer hub.Close()
	mustWriteJSON(t, hub, testEnvelope{
		RequestID: 1,
		Type:      "request",
		Method:    "connect.init",
		Payload: map[string]any{
			"clientName":      "wm-hub",
			"clientVersion":   "0.1.0",
			"protocolVersion": "2.2",
			"role":            "hub",
			"hubId":           "hub-a",
		},
	})
	initResp := mustReadEnvelope(t, hub)
	principal, _ := initResp.Payload["principal"].(map[string]any)
	connectionEpoch, _ := principal["connectionEpoch"].(float64)
	mustWriteJSON(t, hub, testEnvelope{
		RequestID: 2,
		Type:      "request",
		Method:    "registry.reportProjects",
		Payload: map[string]any{
			"hubId":           "hub-a",
			"connectionEpoch": int64(connectionEpoch),
			"projects": []map[string]any{
				{"name": "server", "path": "D:/Code/WheelMaker/server", "online": true, "agent": "codex", "imType": "console", "projectRev": "p1", "git": map[string]any{"gitRev": "g1", "worktreeRev": "w1"}},
			},
		},
	})
	_ = mustReadEnvelope(t, hub)

	client := dialWS(t, ts.URL+"/ws")
	defer client.Close()
	mustWriteJSON(t, client, testEnvelope{
		RequestID: 1,
		Type:      "request",
		Method:    "connect.init",
		Payload: map[string]any{
			"clientName":      "wm-web",
			"clientVersion":   "0.1.0",
			"protocolVersion": "2.2",
			"role":            "client",
		},
	})
	_ = mustReadEnvelope(t, client)

	mustWriteJSON(t, client, testEnvelope{
		RequestID: 2,
		Type:      "request",
		Method:    "batch",
		Payload: map[string]any{
			"requests": []map[string]any{
				{
					"method":    "project.syncCheck",
					"projectId": "hub-a:server",
					"payload": map[string]any{
						"knownProjectRev":  "old-project",
						"knownGitRev":      "old-git",
						"knownWorktreeRev": "old-worktree",
					},
				},
				{
					"method":    "fs.list",
					"projectId": "hub-a:server",
					"payload": map[string]any{
						"path": ".",
					},
				},
			},
		},
	})

	forwarded := mustReadEnvelope(t, hub)
	if forwarded.Method != "fs.list" {
		t.Fatalf("forwarded.method=%q, want fs.list", forwarded.Method)
	}
	mustWriteJSON(t, hub, testEnvelope{
		RequestID: forwarded.RequestID,
		Type:      "response",
		Method:    "fs.list",
		ProjectID: forwarded.ProjectID,
		Payload: map[string]any{
			"path": ".",
			"entries": []map[string]any{
				{"name": "go.mod", "path": "go.mod", "type": "file"},
			},
		},
	})

	batchResp := mustReadEnvelope(t, client)
	if batchResp.Type != "response" || batchResp.Method != "batch" {
		t.Fatalf("unexpected batch response: %#v", batchResp)
	}
	responses, ok := batchResp.Payload["responses"].([]any)
	if !ok || len(responses) != 2 {
		t.Fatalf("responses=%v, want 2 entries", batchResp.Payload["responses"])
	}

	first, _ := responses[0].(map[string]any)
	if first["method"] != "project.syncCheck" || first["type"] != "response" {
		t.Fatalf("unexpected syncCheck item: %v", first)
	}
	firstPayload, _ := first["payload"].(map[string]any)
	stale, _ := firstPayload["staleDomains"].([]any)
	if len(stale) != 3 {
		t.Fatalf("staleDomains=%v, want 3 entries", stale)
	}

	second, _ := responses[1].(map[string]any)
	if second["method"] != "fs.list" || second["type"] != "response" {
		t.Fatalf("unexpected fs.list item: %v", second)
	}
	secondPayload, _ := second["payload"].(map[string]any)
	entries, _ := secondPayload["entries"].([]any)
	if len(entries) != 1 {
		t.Fatalf("entries=%v, want 1 item", entries)
	}
}

func TestRegistryUpdateProjectBroadcastsEvents(t *testing.T) {
	s := New(Config{})
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)

	hub := dialWS(t, ts.URL+"/ws")
	defer hub.Close()
	mustWriteJSON(t, hub, testEnvelope{
		RequestID: 1,
		Type:      "request",
		Method:    "connect.init",
		Payload: map[string]any{
			"clientName":      "wm-hub",
			"clientVersion":   "0.1.0",
			"protocolVersion": "2.2",
			"role":            "hub",
			"hubId":           "hub-a",
		},
	})
	initResp := mustReadEnvelope(t, hub)
	principal, _ := initResp.Payload["principal"].(map[string]any)
	connectionEpoch, _ := principal["connectionEpoch"].(float64)
	mustWriteJSON(t, hub, testEnvelope{
		RequestID: 2,
		Type:      "request",
		Method:    "registry.reportProjects",
		Payload: map[string]any{
			"hubId":           "hub-a",
			"connectionEpoch": int64(connectionEpoch),
			"projects": []map[string]any{
				{"name": "server", "path": "D:/Code/WheelMaker/server", "online": true, "agent": "codex", "imType": "console", "projectRev": "p1", "git": map[string]any{"gitRev": "g1", "worktreeRev": "w1", "headSha": "h1", "dirty": false}},
			},
		},
	})
	_ = mustReadEnvelope(t, hub)

	client := dialWS(t, ts.URL+"/ws")
	defer client.Close()
	mustWriteJSON(t, client, testEnvelope{
		RequestID: 1,
		Type:      "request",
		Method:    "connect.init",
		Payload: map[string]any{
			"clientName":      "wm-web",
			"clientVersion":   "0.1.0",
			"protocolVersion": "2.2",
			"role":            "client",
		},
	})
	_ = mustReadEnvelope(t, client)

	mustWriteJSON(t, hub, testEnvelope{
		RequestID: 3,
		Type:      "request",
		Method:    "registry.updateProject",
		Payload: map[string]any{
			"hubId":           "hub-a",
			"connectionEpoch": int64(connectionEpoch),
			"seq":             1,
			"project": map[string]any{
				"name":       "server",
				"path":       "D:/Code/WheelMaker/server",
				"online":     true,
				"agent":      "codex",
				"imType":     "console",
				"projectRev": "p2",
				"git": map[string]any{
					"gitRev":      "g2",
					"worktreeRev": "w2",
					"headSha":     "h2",
					"dirty":       true,
				},
			},
			"updatedAt": "2026-03-31T10:01:23Z",
		},
	})
	updateResp := mustReadEnvelope(t, hub)
	if updateResp.Type != "response" || updateResp.Method != "registry.updateProject" {
		t.Fatalf("unexpected update response: %#v", updateResp)
	}

	if err := hub.Close(); err != nil {
		t.Fatalf("close hub: %v", err)
	}
	offline := mustReadEnvelope(t, client)
	if offline.Type != "event" || offline.Method != "project.offline" {
		t.Fatalf("unexpected offline event: %#v", offline)
	}
}

func TestSessionForwardingAndSessionEventBroadcast(t *testing.T) {
	s := New(Config{})
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)

	hub := dialWS(t, ts.URL+"/ws")
	defer hub.Close()
	mustWriteJSON(t, hub, testEnvelope{
		RequestID: 1,
		Type:      "request",
		Method:    "connect.init",
		Payload: map[string]any{
			"clientName":      "wm-hub",
			"clientVersion":   "0.1.0",
			"protocolVersion": "2.2",
			"role":            "hub",
			"hubId":           "hub-a",
		},
	})
	initResp := mustReadEnvelope(t, hub)
	principal, _ := initResp.Payload["principal"].(map[string]any)
	connectionEpoch, _ := principal["connectionEpoch"].(float64)
	mustWriteJSON(t, hub, testEnvelope{
		RequestID: 2,
		Type:      "request",
		Method:    "registry.reportProjects",
		Payload: map[string]any{
			"hubId":           "hub-a",
			"connectionEpoch": int64(connectionEpoch),
			"projects": []map[string]any{
				{"name": "server", "path": "D:/Code/WheelMaker/server", "online": true, "agent": "codex", "imType": "app", "projectRev": "p1", "git": map[string]any{"gitRev": "g1", "worktreeRev": "w1"}},
			},
		},
	})
	_ = mustReadEnvelope(t, hub)

	client := dialWS(t, ts.URL+"/ws")
	defer client.Close()
	mustWriteJSON(t, client, testEnvelope{
		RequestID: 1,
		Type:      "request",
		Method:    "connect.init",
		Payload: map[string]any{
			"clientName":      "wm-web",
			"clientVersion":   "0.1.0",
			"protocolVersion": "2.2",
			"role":            "client",
		},
	})
	_ = mustReadEnvelope(t, client)

	mustWriteJSON(t, client, testEnvelope{
		RequestID: 2,
		Type:      "request",
		Method:    "session.send",
		ProjectID: "hub-a:server",
		Payload: map[string]any{
			"sessionId": "sess-1",
			"text":      "hello registry session",
		},
	})

	forwarded := mustReadEnvelope(t, hub)
	if forwarded.Method != "session.send" {
		t.Fatalf("forwarded.method=%q, want session.send", forwarded.Method)
	}
	if forwarded.ProjectID != "hub-a:server" {
		t.Fatalf("forwarded.projectId=%q, want hub-a:server", forwarded.ProjectID)
	}
	forwardPayload := forwarded.Payload
	if forwardPayload["sessionId"] != "sess-1" || forwardPayload["text"] != "hello registry session" {
		t.Fatalf("forwarded payload=%v", forwardPayload)
	}

	mustWriteJSON(t, hub, testEnvelope{
		RequestID: forwarded.RequestID,
		Type:      "response",
		Method:    "session.send",
		ProjectID: forwarded.ProjectID,
		Payload: map[string]any{
			"ok": true,
		},
	})
	sendResp := mustReadEnvelope(t, client)
	if sendResp.Type != "response" || sendResp.Method != "session.send" {
		t.Fatalf("unexpected session.send response: %#v", sendResp)
	}

	mustWriteJSON(t, client, testEnvelope{
		RequestID: 3,
		Type:      "request",
		Method:    "session.setConfig",
		ProjectID: "hub-a:server",
		Payload: map[string]any{
			"sessionId": "sess-1",
			"configId":  "model",
			"value":     "gpt-5",
		},
	})

	forwardedConfig := mustReadEnvelope(t, hub)
	if forwardedConfig.Method != "session.setConfig" {
		t.Fatalf("forwarded.method=%q, want session.setConfig", forwardedConfig.Method)
	}
	if forwardedConfig.ProjectID != "hub-a:server" {
		t.Fatalf("forwarded.projectId=%q, want hub-a:server", forwardedConfig.ProjectID)
	}
	forwardConfigPayload := forwardedConfig.Payload
	if forwardConfigPayload["sessionId"] != "sess-1" || forwardConfigPayload["configId"] != "model" || forwardConfigPayload["value"] != "gpt-5" {
		t.Fatalf("forwarded config payload=%v", forwardConfigPayload)
	}

	mustWriteJSON(t, hub, testEnvelope{
		RequestID: forwardedConfig.RequestID,
		Type:      "response",
		Method:    "session.setConfig",
		ProjectID: forwardedConfig.ProjectID,
		Payload: map[string]any{
			"ok": true,
		},
	})
	setConfigResp := mustReadEnvelope(t, client)
	if setConfigResp.Type != "response" || setConfigResp.Method != "session.setConfig" {
		t.Fatalf("unexpected session.setConfig response: %#v", setConfigResp)
	}

}

func TestConnectInitMonitorRole(t *testing.T) {
	s := New(Config{})
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)

	ws := dialWS(t, ts.URL+"/ws")
	defer ws.Close()

	mustWriteJSON(t, ws, testEnvelope{
		RequestID: 1,
		Type:      "request",
		Method:    "connect.init",
		Payload: map[string]any{
			"clientName":      "wm-monitor",
			"clientVersion":   "0.1.0",
			"protocolVersion": "2.2",
			"role":            "monitor",
			"token":           "",
		},
	})
	resp := mustReadEnvelope(t, ws)
	if resp.Type != "response" || resp.Method != "connect.init" {
		t.Fatalf("unexpected response: %#v", resp)
	}
	principal, _ := resp.Payload["principal"].(map[string]any)
	if principal["role"] != "monitor" {
		t.Fatalf("principal.role=%v, want monitor", principal["role"])
	}
}

func TestMonitorListHubAndMonitorStatusForwarding(t *testing.T) {
	s := New(Config{})
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)

	hub := dialWS(t, ts.URL+"/ws")
	defer hub.Close()
	mustWriteJSON(t, hub, testEnvelope{
		RequestID: 1,
		Type:      "request",
		Method:    "connect.init",
		Payload: map[string]any{
			"clientName":      "wm-hub",
			"clientVersion":   "0.1.0",
			"protocolVersion": "2.2",
			"role":            "hub",
			"hubId":           "hub-a",
		},
	})
	initResp := mustReadEnvelope(t, hub)
	principal, _ := initResp.Payload["principal"].(map[string]any)
	connectionEpoch, _ := principal["connectionEpoch"].(float64)
	mustWriteJSON(t, hub, testEnvelope{
		RequestID: 2,
		Type:      "request",
		Method:    "registry.reportProjects",
		Payload: map[string]any{
			"hubId":           "hub-a",
			"connectionEpoch": int64(connectionEpoch),
			"projects": []map[string]any{
				{"name": "server", "path": "D:/Code/WheelMaker/server", "online": true, "agent": "codex", "imType": "console", "projectRev": "", "git": map[string]any{}},
			},
		},
	})
	_ = mustReadEnvelope(t, hub)

	monitor := dialWS(t, ts.URL+"/ws")
	defer monitor.Close()
	mustWriteJSON(t, monitor, testEnvelope{
		RequestID: 1,
		Type:      "request",
		Method:    "connect.init",
		Payload: map[string]any{
			"clientName":      "wm-monitor",
			"clientVersion":   "0.1.0",
			"protocolVersion": "2.2",
			"role":            "monitor",
		},
	})
	_ = mustReadEnvelope(t, monitor)

	mustWriteJSON(t, monitor, testEnvelope{RequestID: 2, Type: "request", Method: "monitor.listHub", Payload: map[string]any{}})
	listResp := mustReadEnvelope(t, monitor)
	if listResp.Type != "response" || listResp.Method != "monitor.listHub" {
		t.Fatalf("unexpected monitor.listHub response: %#v", listResp)
	}
	hubs, _ := listResp.Payload["hubs"].([]any)
	if len(hubs) != 1 {
		t.Fatalf("hubs=%v, want 1", listResp.Payload["hubs"])
	}

	mustWriteJSON(t, monitor, testEnvelope{RequestID: 3, Type: "request", Method: "monitor.status", Payload: map[string]any{"hubId": "hub-a"}})
	forwarded := mustReadEnvelope(t, hub)
	if forwarded.Method != "monitor.status" {
		t.Fatalf("forwarded.method=%q, want monitor.status", forwarded.Method)
	}
	mustWriteJSON(t, hub, testEnvelope{RequestID: forwarded.RequestID, Type: "response", Method: "monitor.status", Payload: map[string]any{"running": true}})
	statusResp := mustReadEnvelope(t, monitor)
	if statusResp.Type != "response" || statusResp.Method != "monitor.status" {
		t.Fatalf("unexpected monitor.status response: %#v", statusResp)
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
