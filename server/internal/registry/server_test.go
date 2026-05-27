package registry

import (
	"github.com/gorilla/websocket"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
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
			"protocolVersion": "2.3",
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

func TestConnectInitRejectsLegacyProtocolVersion22(t *testing.T) {
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
	message, _ := resp.Payload["message"].(string)
	if resp.Type != "error" || resp.Payload["code"] != "INVALID_ARGUMENT" || !strings.Contains(message, "unsupported protocolVersion") {
		t.Fatalf("response=%#v, want unsupported protocolVersion error", resp)
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
			"protocolVersion": "2.3",
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
				{"name": "server", "path": "D:/Code/WheelMaker/server", "online": true, "agent": "codex", "agents": []string{"codex", "claude", "copilot"}, "projectRev": "", "git": map[string]any{}},
				{"name": "app", "path": "D:/Code/WheelMaker/app", "online": true, "agent": "claude", "agents": []string{"claude", "codex"}, "projectRev": "", "git": map[string]any{}},
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
			"protocolVersion": "2.3",
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
	hubs, ok := listResp.Payload["hubs"].([]any)
	if !ok || len(hubs) != 1 {
		t.Fatalf("hubs=%v, want 1 item", listResp.Payload["hubs"])
	}
	firstHub, _ := hubs[0].(map[string]any)
	if firstHub["hubId"] != "hub-a" {
		t.Fatalf("hub=%v, want hub-a", firstHub)
	}
	if _, ok := firstHub["online"]; ok {
		t.Fatalf("hub should not expose online state: %v", firstHub)
	}
	first, _ := projects[0].(map[string]any)
	if _, ok := first["projectId"].(string); !ok {
		t.Fatalf("projectId missing: %v", first)
	}

	projectsByName := map[string]map[string]any{}
	for _, item := range projects {
		proj, _ := item.(map[string]any)
		name, _ := proj["name"].(string)
		projectsByName[name] = proj
	}
	serverProject := projectsByName["server"]
	if serverProject == nil {
		t.Fatalf("server project missing: %v", projects)
	}
	serverAgents, _ := serverProject["agents"].([]any)
	if !reflect.DeepEqual(serverAgents, []any{"codex", "claude", "copilot"}) {
		t.Fatalf("server agents=%v, want [codex claude copilot]", serverAgents)
	}
	appProject := projectsByName["app"]
	if appProject == nil {
		t.Fatalf("app project missing: %v", projects)
	}
	appAgents, _ := appProject["agents"].([]any)
	if !reflect.DeepEqual(appAgents, []any{"claude", "codex"}) {
		t.Fatalf("app agents=%v, want [claude codex]", appAgents)
	}
}

func TestProjectListIncludesLocalReadCandidate(t *testing.T) {
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
			"protocolVersion": "2.3",
			"role":            "hub",
			"hubId":           "hub-local-read",
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
			"hubId":           "hub-local-read",
			"connectionEpoch": int64(connectionEpoch),
			"localRead": map[string]any{
				"endpointId":       "local-hub-1",
				"url":              "ws://127.0.0.1:53123/ws",
				"proofPublicKey":   "base64-public-key",
				"proofFingerprint": "sha256:fingerprint",
			},
			"projects": []map[string]any{
				{"name": "server", "path": "D:/Code/WheelMaker/server", "online": true, "agent": "codex", "projectRev": "", "git": map[string]any{}},
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
			"protocolVersion": "2.3",
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
	hubs, ok := listResp.Payload["hubs"].([]any)
	if !ok || len(hubs) != 1 {
		t.Fatalf("hubs=%v, want one hub", listResp.Payload["hubs"])
	}
	firstHub, _ := hubs[0].(map[string]any)
	localRead, _ := firstHub["localRead"].(map[string]any)
	if localRead["endpointId"] != "local-hub-1" {
		t.Fatalf("localRead=%#v, want candidate endpointId", localRead)
	}
	if localRead["url"] != "ws://127.0.0.1:53123/ws" {
		t.Fatalf("localRead url=%v", localRead["url"])
	}

	projects, ok := listResp.Payload["projects"].([]any)
	if !ok || len(projects) != 1 {
		t.Fatalf("projects=%v, want one project", listResp.Payload["projects"])
	}
	project, _ := projects[0].(map[string]any)
	if _, exists := project["localRead"]; exists {
		t.Fatalf("project should not expose localRead: %#v", project)
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
			"protocolVersion": "2.3",
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
			"protocolVersion": "2.3",
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
				{"name": "server", "path": "D:/Code/WheelMaker/server", "online": true, "agent": "codex", "projectRev": "p2", "git": map[string]any{"gitRev": "g2", "worktreeRev": "w2"}},
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
				{"name": "server", "path": "D:/Code/WheelMaker/server", "online": true, "agent": "codex", "projectRev": "p1", "git": map[string]any{"gitRev": "g1", "worktreeRev": "w1"}},
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
			"protocolVersion": "2.3",
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
			"protocolVersion": "2.3",
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
			"protocolVersion": "2.3",
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
				{"name": "server", "path": "D:/Code/WheelMaker/server", "online": true, "agent": "codex", "projectRev": "p1", "git": map[string]any{"gitRev": "g1", "worktreeRev": "w1"}},
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
			"protocolVersion": "2.3",
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
			"protocolVersion": "2.3",
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
				{"name": "server", "path": "D:/Code/WheelMaker/server", "online": true, "agent": "codex", "projectRev": "p1", "git": map[string]any{"gitRev": "g1", "worktreeRev": "w1", "headSha": "h1", "dirty": false}},
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
			"protocolVersion": "2.3",
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
			"protocolVersion": "2.3",
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
				{"name": "server", "path": "D:/Code/WheelMaker/server", "online": true, "agent": "codex", "projectRev": "p1", "git": map[string]any{"gitRev": "g1", "worktreeRev": "w1"}},
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
			"protocolVersion": "2.3",
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

	mustWriteJSON(t, client, testEnvelope{
		RequestID: 4,
		Type:      "request",
		Method:    "session.rename",
		ProjectID: "hub-a:server",
		Payload: map[string]any{
			"sessionId": "sess-1",
			"title":     "Renamed session",
		},
	})

	forwardedRename := mustReadEnvelope(t, hub)
	if forwardedRename.Method != "session.rename" {
		t.Fatalf("forwarded.method=%q, want session.rename", forwardedRename.Method)
	}
	if forwardedRename.ProjectID != "hub-a:server" {
		t.Fatalf("forwarded.projectId=%q, want hub-a:server", forwardedRename.ProjectID)
	}
	forwardRenamePayload := forwardedRename.Payload
	if forwardRenamePayload["sessionId"] != "sess-1" || forwardRenamePayload["title"] != "Renamed session" {
		t.Fatalf("forwarded rename payload=%v", forwardRenamePayload)
	}

	mustWriteJSON(t, hub, testEnvelope{
		RequestID: forwardedRename.RequestID,
		Type:      "response",
		Method:    "session.rename",
		ProjectID: forwardedRename.ProjectID,
		Payload: map[string]any{
			"ok": true,
		},
	})
	renameResp := mustReadEnvelope(t, client)
	if renameResp.Type != "response" || renameResp.Method != "session.rename" {
		t.Fatalf("unexpected session.rename response: %#v", renameResp)
	}

	mustWriteJSON(t, client, testEnvelope{
		RequestID: 5,
		Type:      "request",
		Method:    "session.archive",
		ProjectID: "hub-a:server",
		Payload: map[string]any{
			"sessionId": "sess-1",
		},
	})

	forwardedArchive := mustReadEnvelope(t, hub)
	if forwardedArchive.Method != "session.archive" {
		t.Fatalf("forwarded.method=%q, want session.archive", forwardedArchive.Method)
	}
	if forwardedArchive.ProjectID != "hub-a:server" {
		t.Fatalf("forwarded.projectId=%q, want hub-a:server", forwardedArchive.ProjectID)
	}
	forwardArchivePayload := forwardedArchive.Payload
	if forwardArchivePayload["sessionId"] != "sess-1" {
		t.Fatalf("forwarded archive payload=%v", forwardArchivePayload)
	}

	mustWriteJSON(t, hub, testEnvelope{
		RequestID: forwardedArchive.RequestID,
		Type:      "response",
		Method:    "session.archive",
		ProjectID: forwardedArchive.ProjectID,
		Payload: map[string]any{
			"ok": true,
		},
	})
	archiveResp := mustReadEnvelope(t, client)
	if archiveResp.Type != "response" || archiveResp.Method != "session.archive" {
		t.Fatalf("unexpected session.archive response: %#v", archiveResp)
	}

	mustWriteJSON(t, client, testEnvelope{
		RequestID: 6,
		Type:      "request",
		Method:    "session.delete",
		ProjectID: "hub-a:server",
		Payload: map[string]any{
			"sessionId": "sess-1",
		},
	})
	forwardedDelete := mustReadEnvelope(t, hub)
	if forwardedDelete.Method != "session.delete" {
		t.Fatalf("forwarded.method=%q, want session.delete", forwardedDelete.Method)
	}
	if forwardedDelete.ProjectID != "hub-a:server" {
		t.Fatalf("forwarded.projectId=%q, want hub-a:server", forwardedDelete.ProjectID)
	}
	forwardDeletePayload := forwardedDelete.Payload
	if forwardDeletePayload["sessionId"] != "sess-1" {
		t.Fatalf("forwarded delete payload=%v", forwardDeletePayload)
	}

	mustWriteJSON(t, hub, testEnvelope{
		RequestID: forwardedDelete.RequestID,
		Type:      "response",
		Method:    "session.delete",
		ProjectID: forwardedDelete.ProjectID,
		Payload: map[string]any{
			"ok":        true,
			"sessionId": "sess-1",
		},
	})
	deleteResp := mustReadEnvelope(t, client)
	if deleteResp.Type != "response" || deleteResp.Method != "session.delete" {
		t.Fatalf("unexpected session.delete response: %#v", deleteResp)
	}

	attachmentRequests := []struct {
		method  string
		payload map[string]any
	}{
		{method: "session.attachment.start", payload: map[string]any{"sessionId": "sess-1", "name": "a.txt", "mimeType": "text/plain", "size": 1}},
		{method: "session.attachment.chunk", payload: map[string]any{"sessionId": "sess-1", "uploadId": "upload-1", "offset": 0, "data": "YQ=="}},
		{method: "session.attachment.finish", payload: map[string]any{"sessionId": "sess-1", "uploadId": "upload-1", "sha256": "ca978112ca1bbdcafac231b39a23dc4da786eff8147c4e72b9807785afee48bb"}},
		{method: "session.attachment.cancel", payload: map[string]any{"sessionId": "sess-1", "uploadId": "upload-2"}},
		{method: "session.attachment.delete", payload: map[string]any{"sessionId": "sess-1", "attachmentId": "sha256-ca978112ca1bbdcafac231b39a23dc4da786eff8147c4e72b9807785afee48bb"}},
	}
	for i, request := range attachmentRequests {
		mustWriteJSON(t, client, testEnvelope{
			RequestID: int64(7 + i),
			Type:      "request",
			Method:    request.method,
			ProjectID: "hub-a:server",
			Payload:   request.payload,
		})
		forwardedAttachment := mustReadEnvelope(t, hub)
		if forwardedAttachment.Method != request.method {
			t.Fatalf("forwarded.method=%q, want %s", forwardedAttachment.Method, request.method)
		}
		if forwardedAttachment.ProjectID != "hub-a:server" {
			t.Fatalf("forwarded.projectId=%q, want hub-a:server", forwardedAttachment.ProjectID)
		}
		if forwardedAttachment.Payload["sessionId"] != "sess-1" {
			t.Fatalf("forwarded attachment payload=%v", forwardedAttachment.Payload)
		}
		mustWriteJSON(t, hub, testEnvelope{
			RequestID: forwardedAttachment.RequestID,
			Type:      "response",
			Method:    request.method,
			ProjectID: forwardedAttachment.ProjectID,
			Payload: map[string]any{
				"ok": true,
			},
		})
		attachmentResp := mustReadEnvelope(t, client)
		if attachmentResp.Type != "response" || attachmentResp.Method != request.method {
			t.Fatalf("unexpected %s response: %#v", request.method, attachmentResp)
		}
	}
}

func TestChatSendIsUnsupportedAfterIMRemoval(t *testing.T) {
	s := New(Config{Token: "tok"})
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	method := "chat" + ".send"

	client := dialWS(t, ts.URL+"/ws")
	defer client.Close()
	mustWriteJSON(t, client, testEnvelope{
		RequestID: 1,
		Type:      "request",
		Method:    "connect.init",
		Payload: map[string]any{
			"clientName":      "wm-web",
			"clientVersion":   "0.1.0",
			"protocolVersion": "2.3",
			"role":            "client",
			"token":           "tok",
		},
	})
	_ = mustReadEnvelope(t, client)

	mustWriteJSON(t, client, testEnvelope{
		RequestID: 2,
		Type:      "request",
		Method:    method,
		ProjectID: "hub-a:proj1",
		Payload: map[string]any{
			"chatId": "chat-1",
			"text":   "hello",
		},
	})
	resp := mustReadEnvelope(t, client)
	if resp.Type != "error" {
		t.Fatalf("response type = %q, want error", resp.Type)
	}
	if resp.Payload["code"] != codeInvalidArgument {
		t.Fatalf("code = %q, want %q", resp.Payload["code"], codeInvalidArgument)
	}
}

func TestBatchChatSendIsUnsupportedAfterIMRemoval(t *testing.T) {
	s := New(Config{})
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	method := removedChatSendMethod

	hub := dialWS(t, ts.URL+"/ws")
	defer hub.Close()
	mustWriteJSON(t, hub, testEnvelope{
		RequestID: 1,
		Type:      "request",
		Method:    "connect.init",
		Payload: map[string]any{
			"clientName":      "wm-hub",
			"clientVersion":   "0.1.0",
			"protocolVersion": "2.3",
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
				{"name": "server", "path": "D:/Code/WheelMaker/server", "online": true, "agent": "codex", "projectRev": "p1", "git": map[string]any{"gitRev": "g1", "worktreeRev": "w1"}},
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
			"protocolVersion": "2.3",
			"role":            "client",
		},
	})
	_ = mustReadEnvelope(t, client)

	forwardedCh := make(chan testEnvelope, 1)
	forwardWriteErrCh := make(chan error, 1)
	go func() {
		_ = hub.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		var forwarded testEnvelope
		if err := hub.ReadJSON(&forwarded); err != nil {
			return
		}
		forwardedCh <- forwarded
		forwardWriteErrCh <- hub.WriteJSON(testEnvelope{
			RequestID: forwarded.RequestID,
			Type:      "response",
			Method:    forwarded.Method,
			ProjectID: forwarded.ProjectID,
			Payload: map[string]any{
				"ok": true,
			},
		})
	}()

	mustWriteJSON(t, client, testEnvelope{
		RequestID: 2,
		Type:      "request",
		Method:    "batch",
		Payload: map[string]any{
			"requests": []map[string]any{
				{
					"method":    method,
					"projectId": "hub-a:server",
					"payload": map[string]any{
						"chatId": "chat-1",
						"text":   "hello",
					},
				},
			},
		},
	})

	select {
	case forwarded := <-forwardedCh:
		if err := <-forwardWriteErrCh; err != nil {
			t.Fatalf("write forwarded response: %v", err)
		}
		t.Fatalf("batch subrequest was forwarded to hub: %#v", forwarded)
	case <-time.After(200 * time.Millisecond):
	}

	resp := mustReadEnvelope(t, client)
	if resp.Type != "response" || resp.Method != "batch" {
		t.Fatalf("batch response=%#v, want response", resp)
	}
	responses, _ := resp.Payload["responses"].([]any)
	if len(responses) != 1 {
		t.Fatalf("responses=%#v, want one response", resp.Payload["responses"])
	}
	item, _ := responses[0].(map[string]any)
	if item["type"] != "error" {
		t.Fatalf("batch item=%#v, want error", item)
	}
	payload, _ := item["payload"].(map[string]any)
	if payload["code"] != codeInvalidArgument {
		t.Fatalf("batch item code=%v, want %s", payload["code"], codeInvalidArgument)
	}
}

func TestMonitorBatchRejectsClientForwardMethods(t *testing.T) {
	tests := []struct {
		name    string
		method  string
		payload map[string]any
	}{
		{
			name:    "session list",
			method:  "session.list",
			payload: map[string]any{},
		},
		{
			name:   "fs read",
			method: "fs.read",
			payload: map[string]any{
				"path": "README.md",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if methodAllowed("monitor", tt.method) {
				t.Fatalf("monitor direct methodAllowed(%q)=true, want false", tt.method)
			}

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
					"protocolVersion": "2.3",
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
						{"name": "server", "path": "D:/Code/WheelMaker/server", "online": true, "agent": "codex", "projectRev": "p1", "git": map[string]any{"gitRev": "g1", "worktreeRev": "w1"}},
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
					"protocolVersion": "2.3",
					"role":            "monitor",
				},
			})
			_ = mustReadEnvelope(t, monitor)

			forwardedCh := make(chan testEnvelope, 1)
			forwardWriteErrCh := make(chan error, 1)
			go func() {
				_ = hub.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
				var forwarded testEnvelope
				if err := hub.ReadJSON(&forwarded); err != nil {
					return
				}
				forwardedCh <- forwarded
				forwardWriteErrCh <- hub.WriteJSON(testEnvelope{
					RequestID: forwarded.RequestID,
					Type:      "response",
					Method:    forwarded.Method,
					ProjectID: forwarded.ProjectID,
					Payload: map[string]any{
						"ok": true,
					},
				})
			}()

			mustWriteJSON(t, monitor, testEnvelope{
				RequestID: 2,
				Type:      "request",
				Method:    "batch",
				Payload: map[string]any{
					"requests": []map[string]any{
						{
							"method":    tt.method,
							"projectId": "hub-a:server",
							"payload":   tt.payload,
						},
					},
				},
			})

			select {
			case forwarded := <-forwardedCh:
				if err := <-forwardWriteErrCh; err != nil {
					t.Fatalf("write forwarded response: %v", err)
				}
				t.Fatalf("monitor batch subrequest was forwarded to hub: %#v", forwarded)
			case <-time.After(200 * time.Millisecond):
			}

			resp := mustReadEnvelope(t, monitor)
			if resp.Type != "response" || resp.Method != "batch" {
				t.Fatalf("batch response=%#v, want response", resp)
			}
			responses, _ := resp.Payload["responses"].([]any)
			if len(responses) != 1 {
				t.Fatalf("responses=%#v, want one response", resp.Payload["responses"])
			}
			item, _ := responses[0].(map[string]any)
			if item["type"] != "error" {
				t.Fatalf("batch item=%#v, want error", item)
			}
			payload, _ := item["payload"].(map[string]any)
			if payload["code"] != codeForbidden {
				t.Fatalf("batch item code=%v, want %s", payload["code"], codeForbidden)
			}
		})
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
			"protocolVersion": "2.3",
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
			"protocolVersion": "2.3",
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
				{"name": "server", "path": "D:/Code/WheelMaker/server", "online": true, "agent": "codex", "projectRev": "", "git": map[string]any{}},
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
			"protocolVersion": "2.3",
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

func TestCmdNPMForwardsByHubIDWithoutProjectID(t *testing.T) {
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
			"protocolVersion": "2.3",
			"role":            "hub",
			"hubId":           "hub-npm",
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
			"hubId":           "hub-npm",
			"connectionEpoch": int64(connectionEpoch),
			"projects":        []map[string]any{},
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
			"protocolVersion": "2.3",
			"role":            "client",
		},
	})
	_ = mustReadEnvelope(t, client)

	mustWriteJSON(t, client, testEnvelope{
		RequestID: 2,
		Type:      "request",
		Method:    "cmd.npm",
		Payload: map[string]any{
			"action": "scan",
			"hubId":  "hub-npm",
		},
	})
	_ = hub.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	forwarded := mustReadEnvelope(t, hub)
	if forwarded.Type != "request" || forwarded.Method != "cmd.npm" {
		t.Fatalf("forwarded=%#v, want cmd.npm request", forwarded)
	}
	if forwarded.ProjectID != "" {
		t.Fatalf("forwarded projectId=%q, want empty", forwarded.ProjectID)
	}
	if forwarded.Payload["hubId"] != "hub-npm" || forwarded.Payload["action"] != "scan" {
		t.Fatalf("forwarded payload=%#v", forwarded.Payload)
	}

	mustWriteJSON(t, hub, testEnvelope{
		RequestID: forwarded.RequestID,
		Type:      "response",
		Method:    "cmd.npm",
		Payload: map[string]any{
			"ok": true,
		},
	})
	resp := mustReadEnvelope(t, client)
	if resp.Type != "response" || resp.Method != "cmd.npm" {
		t.Fatalf("client response=%#v, want cmd.npm response", resp)
	}
	if resp.ProjectID != "" {
		t.Fatalf("client response projectId=%q, want empty", resp.ProjectID)
	}
}

func TestCmdTokenForwardsByHubIDWithoutProjectID(t *testing.T) {
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
			"protocolVersion": "2.3",
			"role":            "hub",
			"hubId":           "hub-token",
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
			"hubId":           "hub-token",
			"connectionEpoch": int64(connectionEpoch),
			"projects":        []map[string]any{},
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
			"protocolVersion": "2.3",
			"role":            "client",
		},
	})
	_ = mustReadEnvelope(t, client)

	mustWriteJSON(t, client, testEnvelope{
		RequestID: 2,
		Type:      "request",
		Method:    "cmd.token",
		Payload: map[string]any{
			"action": "scan",
			"hubId":  "hub-token",
		},
	})
	_ = hub.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	forwarded := mustReadEnvelope(t, hub)
	if forwarded.Type != "request" || forwarded.Method != "cmd.token" {
		t.Fatalf("forwarded=%#v, want cmd.token request", forwarded)
	}
	if forwarded.ProjectID != "" {
		t.Fatalf("forwarded projectId=%q, want empty", forwarded.ProjectID)
	}
	if forwarded.Payload["hubId"] != "hub-token" || forwarded.Payload["action"] != "scan" {
		t.Fatalf("forwarded payload=%#v", forwarded.Payload)
	}

	mustWriteJSON(t, hub, testEnvelope{
		RequestID: forwarded.RequestID,
		Type:      "response",
		Method:    "cmd.token",
		Payload: map[string]any{
			"ok":        true,
			"updatedAt": "2026-05-19T10:00:00Z",
			"providers": []map[string]any{},
		},
	})
	resp := mustReadEnvelope(t, client)
	if resp.Type != "response" || resp.Method != "cmd.token" {
		t.Fatalf("client response=%#v, want cmd.token response", resp)
	}
	if resp.ProjectID != "" {
		t.Fatalf("client response projectId=%q, want empty", resp.ProjectID)
	}
}

func TestCmdUpdateForwardsByHubIDWithoutProjectID(t *testing.T) {
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
			"clientName":      "wheelmaker-hub",
			"clientVersion":   "0.1.0",
			"protocolVersion": "2.3",
			"role":            "hub",
			"hubId":           "hub-update",
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
			"hubId":           "hub-update",
			"connectionEpoch": int64(connectionEpoch),
			"projects":        []map[string]any{},
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
			"protocolVersion": "2.3",
			"role":            "client",
		},
	})
	_ = mustReadEnvelope(t, client)

	mustWriteJSON(t, client, testEnvelope{
		RequestID: 2,
		Type:      "request",
		Method:    "cmd.update",
		Payload: map[string]any{
			"action": "query",
			"hubId":  "hub-update",
		},
	})
	_ = hub.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	forwarded := mustReadEnvelope(t, hub)
	if forwarded.Type != "request" || forwarded.Method != "cmd.update" {
		t.Fatalf("forwarded=%#v, want cmd.update request", forwarded)
	}
	if forwarded.ProjectID != "" {
		t.Fatalf("forwarded projectId=%q, want empty", forwarded.ProjectID)
	}
	if forwarded.Payload["hubId"] != "hub-update" || forwarded.Payload["action"] != "query" {
		t.Fatalf("forwarded payload=%#v", forwarded.Payload)
	}

	mustWriteJSON(t, hub, testEnvelope{
		RequestID: forwarded.RequestID,
		Type:      "response",
		Method:    "cmd.update",
		Payload: map[string]any{
			"ok":     true,
			"status": "up_to_date",
		},
	})
	resp := mustReadEnvelope(t, client)
	if resp.Type != "response" || resp.Method != "cmd.update" {
		t.Fatalf("client response=%#v, want cmd.update response", resp)
	}
	if resp.ProjectID != "" {
		t.Fatalf("client response projectId=%q, want empty", resp.ProjectID)
	}
}

func TestCmdSkillsForwardsByHubIDWithoutProjectID(t *testing.T) {
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
			"clientName":      "wheelmaker-hub",
			"clientVersion":   "0.1.0",
			"protocolVersion": "2.3",
			"role":            "hub",
			"hubId":           "hub-skills",
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
			"hubId":           "hub-skills",
			"connectionEpoch": int64(connectionEpoch),
			"projects":        []map[string]any{},
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
			"protocolVersion": "2.3",
			"role":            "client",
		},
	})
	_ = mustReadEnvelope(t, client)

	mustWriteJSON(t, client, testEnvelope{
		RequestID: 2,
		Type:      "request",
		Method:    "cmd.skills",
		Payload: map[string]any{
			"action": "scan",
			"hubId":  "hub-skills",
		},
	})
	_ = hub.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	forwarded := mustReadEnvelope(t, hub)
	if forwarded.Type != "request" || forwarded.Method != "cmd.skills" {
		t.Fatalf("forwarded=%#v, want cmd.skills request", forwarded)
	}
	if forwarded.ProjectID != "" {
		t.Fatalf("forwarded projectId=%q, want empty", forwarded.ProjectID)
	}
	if forwarded.Payload["hubId"] != "hub-skills" || forwarded.Payload["action"] != "scan" {
		t.Fatalf("forwarded payload=%#v", forwarded.Payload)
	}

	mustWriteJSON(t, hub, testEnvelope{
		RequestID: forwarded.RequestID,
		Type:      "response",
		Method:    "cmd.skills",
		Payload: map[string]any{
			"ok":    true,
			"hubId": "hub-skills",
		},
	})
	resp := mustReadEnvelope(t, client)
	if resp.Type != "response" || resp.Method != "cmd.skills" {
		t.Fatalf("client response=%#v, want cmd.skills response", resp)
	}
	if resp.ProjectID != "" {
		t.Fatalf("client response projectId=%q, want empty", resp.ProjectID)
	}
}

func TestHubCommandForwardRequestsToDifferentHubsDoNotBlockBehindSlowHub(t *testing.T) {
	s := New(Config{})
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)

	hubA := dialReportedHub(t, ts.URL+"/ws", "hub-a")
	defer hubA.Close()
	hubB := dialReportedHub(t, ts.URL+"/ws", "hub-b")
	defer hubB.Close()

	client := dialWS(t, ts.URL+"/ws")
	defer client.Close()
	mustWriteJSON(t, client, testEnvelope{
		RequestID: 1,
		Type:      "request",
		Method:    "connect.init",
		Payload: map[string]any{
			"clientName":      "wm-web",
			"clientVersion":   "0.1.0",
			"protocolVersion": "2.3",
			"role":            "client",
		},
	})
	_ = mustReadEnvelope(t, client)

	mustWriteJSON(t, client, testEnvelope{
		RequestID: 2,
		Type:      "request",
		Method:    "cmd.skills",
		Payload: map[string]any{
			"action": "update",
			"hubId":  "hub-a",
			"scope":  "hub",
		},
	})
	_ = hubA.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	forwardedA := mustReadEnvelope(t, hubA)
	if forwardedA.Type != "request" || forwardedA.Method != "cmd.skills" {
		t.Fatalf("forwardedA=%#v, want cmd.skills request", forwardedA)
	}

	mustWriteJSON(t, client, testEnvelope{
		RequestID: 3,
		Type:      "request",
		Method:    "cmd.update",
		Payload: map[string]any{
			"action": "query",
			"hubId":  "hub-b",
		},
	})
	_ = hubB.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	forwardedB := mustReadEnvelope(t, hubB)
	if forwardedB.Type != "request" || forwardedB.Method != "cmd.update" {
		t.Fatalf("forwardedB=%#v, want cmd.update request", forwardedB)
	}

	mustWriteJSON(t, hubB, testEnvelope{
		RequestID: forwardedB.RequestID,
		Type:      "response",
		Method:    "cmd.update",
		Payload: map[string]any{
			"ok":     true,
			"hubId":  "hub-b",
			"status": "up_to_date",
		},
	})
	mustWriteJSON(t, hubA, testEnvelope{
		RequestID: forwardedA.RequestID,
		Type:      "response",
		Method:    "cmd.skills",
		Payload: map[string]any{
			"ok":    true,
			"hubId": "hub-a",
		},
	})
	responses := map[int64]testEnvelope{}
	for len(responses) < 2 {
		resp := mustReadEnvelope(t, client)
		responses[resp.RequestID] = resp
	}
	if responses[2].Type != "response" || responses[2].Method != "cmd.skills" {
		t.Fatalf("client response 2=%#v, want cmd.skills response", responses[2])
	}
	if responses[3].Type != "response" || responses[3].Method != "cmd.update" {
		t.Fatalf("client response 3=%#v, want cmd.update response", responses[3])
	}
}

func TestCmdPrefixIsNotAllowedByWildcard(t *testing.T) {
	s := New(Config{})
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)

	client := dialWS(t, ts.URL+"/ws")
	defer client.Close()
	mustWriteJSON(t, client, testEnvelope{
		RequestID: 1,
		Type:      "request",
		Method:    "connect.init",
		Payload: map[string]any{
			"clientName":      "wm-web",
			"clientVersion":   "0.1.0",
			"protocolVersion": "2.3",
			"role":            "client",
		},
	})
	_ = mustReadEnvelope(t, client)

	mustWriteJSON(t, client, testEnvelope{
		RequestID: 2,
		Type:      "request",
		Method:    "cmd.shell",
		Payload: map[string]any{
			"hubId": "hub-npm",
		},
	})
	resp := mustReadEnvelope(t, client)
	if resp.Type != "error" {
		t.Fatalf("resp=%#v, want error", resp)
	}
	if resp.Payload["code"] != codeForbidden {
		t.Fatalf("code=%v, want %s", resp.Payload["code"], codeForbidden)
	}

	if !methodAllowed("client", "cmd.skills") {
		t.Fatal("cmd.skills should be explicitly allowed")
	}
	if !methodAllowed("client", "cmd.token") {
		t.Fatal("cmd.token should be explicitly allowed")
	}
}

func TestCmdNPMBatchRequiresClientRole(t *testing.T) {
	s := New(Config{})
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)

	monitor := dialWS(t, ts.URL+"/ws")
	defer monitor.Close()
	mustWriteJSON(t, monitor, testEnvelope{
		RequestID: 1,
		Type:      "request",
		Method:    "connect.init",
		Payload: map[string]any{
			"clientName":      "wm-monitor",
			"clientVersion":   "0.1.0",
			"protocolVersion": "2.3",
			"role":            "monitor",
		},
	})
	_ = mustReadEnvelope(t, monitor)

	mustWriteJSON(t, monitor, testEnvelope{
		RequestID: 2,
		Type:      "request",
		Method:    "batch",
		Payload: map[string]any{
			"requests": []map[string]any{
				{
					"method":  "cmd.npm",
					"payload": map[string]any{"action": "scan", "hubId": "hub-a"},
				},
			},
		},
	})
	resp := mustReadEnvelope(t, monitor)
	responses, _ := resp.Payload["responses"].([]any)
	if len(responses) != 1 {
		t.Fatalf("responses=%#v, want one response", resp.Payload["responses"])
	}
	item, _ := responses[0].(map[string]any)
	if item["type"] != "error" {
		t.Fatalf("batch item=%#v, want error", item)
	}
	payload, _ := item["payload"].(map[string]any)
	if payload["code"] != codeForbidden {
		t.Fatalf("payload=%#v, want FORBIDDEN", payload)
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

func dialReportedHub(t *testing.T, rawURL string, hubID string) *websocket.Conn {
	t.Helper()
	hub := dialWS(t, rawURL)
	mustWriteJSON(t, hub, testEnvelope{
		RequestID: 1,
		Type:      "request",
		Method:    "connect.init",
		Payload: map[string]any{
			"clientName":      "wheelmaker-hub",
			"clientVersion":   "0.1.0",
			"protocolVersion": "2.3",
			"role":            "hub",
			"hubId":           hubID,
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
			"hubId":           hubID,
			"connectionEpoch": int64(connectionEpoch),
			"projects":        []map[string]any{},
		},
	})
	_ = mustReadEnvelope(t, hub)
	return hub
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

func TestRelayStatusIsAllowedForClientOnly(t *testing.T) {
	if !methodAllowed("client", "relay.status") {
		t.Fatal("client should be allowed to call relay.status")
	}
	if methodAllowed("hub", "relay.status") {
		t.Fatal("hub should not be allowed to call public relay.status")
	}
	if methodAllowed("monitor", "relay.enable") {
		t.Fatal("monitor should not be allowed to mutate relay slot")
	}
}

func TestRelayStatusReturnsDisabledSnapshot(t *testing.T) {
	s := New(Config{})
	ts := httptestNewRegistryServer(t, s.Handler())

	client := dialWS(t, "http://"+ts+"/ws")
	defer client.Close()
	connectRegistryClient(t, client)

	mustWriteJSON(t, client, testEnvelope{
		RequestID: 2,
		Type:      "request",
		Method:    "relay.status",
		Payload:   map[string]any{},
	})
	resp := mustReadEnvelope(t, client)
	if resp.Type != "response" || resp.Method != "relay.status" {
		t.Fatalf("relay.status response=%#v", resp)
	}
	if resp.Payload["status"] != "Disabled" || resp.Payload["enabled"] != false {
		t.Fatalf("relay.status payload=%#v, want disabled snapshot", resp.Payload)
	}
}

func TestRelayEnableForwardsInternalOpenToHub(t *testing.T) {
	s := New(Config{})
	ts := httptestNewRegistryServer(t, s.Handler())

	hub := dialReportedHub(t, "http://"+ts+"/ws", "hub-relay")
	defer hub.Close()

	client := dialWS(t, "http://"+ts+"/ws")
	defer client.Close()
	connectRegistryClient(t, client)

	listenPort := reserveTCPPort(t)
	mustWriteJSON(t, client, testEnvelope{
		RequestID: 2,
		Type:      "request",
		Method:    "relay.enable",
		Payload: map[string]any{
			"listenPort": listenPort,
			"hubId":      "hub-relay",
			"targetHost": "127.0.0.1",
			"targetPort": 43210,
			"accessCode": "483921",
		},
	})

	_ = hub.SetReadDeadline(time.Now().Add(2 * time.Second))
	forwarded := mustReadEnvelope(t, hub)
	if forwarded.Type != "request" || forwarded.Method != "relay.open" {
		t.Fatalf("forwarded=%#v, want relay.open request", forwarded)
	}
	if forwarded.Payload["targetHost"] != "127.0.0.1" || forwarded.Payload["targetPort"] != float64(43210) {
		t.Fatalf("forwarded payload=%#v", forwarded.Payload)
	}
	if forwarded.Payload["nonce"] == "" || forwarded.Payload["relayURL"] == "" {
		t.Fatalf("forwarded payload missing tunnel fields: %#v", forwarded.Payload)
	}
	mustWriteJSON(t, hub, testEnvelope{
		RequestID: forwarded.RequestID,
		Type:      "response",
		Method:    "relay.open",
		Payload: map[string]any{
			"ok": true,
		},
	})

	resp := mustReadEnvelope(t, client)
	if resp.Type != "response" || resp.Method != "relay.enable" {
		t.Fatalf("relay.enable response=%#v", resp)
	}
	if resp.Payload["enabled"] != true || resp.Payload["status"] != "Opening" {
		t.Fatalf("relay.enable payload=%#v, want opening snapshot", resp.Payload)
	}
}

func connectRegistryClient(t *testing.T, ws *websocket.Conn) {
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
		},
	})
	_ = mustReadEnvelope(t, ws)
}

func httptestNewRegistryServer(t *testing.T, handler http.Handler) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen registry: %v", err)
	}
	srv := &http.Server{Handler: handler}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() {
		_ = srv.Close()
		_ = ln.Close()
	})
	return ln.Addr().String()
}

func reserveTCPPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve tcp port: %v", err)
	}
	defer ln.Close()
	addr, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("reserved addr=%v is not tcp", ln.Addr())
	}
	return addr.Port
}
