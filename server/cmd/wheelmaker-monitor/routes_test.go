package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	clientpkg "github.com/swm8023/wheelmaker/internal/hub/client"
)

func TestActionUpdatePublishWritesFullUpdateSignal(t *testing.T) {
	baseDir := t.TempDir()
	mon := NewMonitor(baseDir)

	mux := http.NewServeMux()
	registerRoutes(mux, mon)

	req := httptest.NewRequest(http.MethodPost, "/api/action/update-publish", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d, want=%d, body=%s", rr.Code, http.StatusOK, rr.Body.String())
	}

	signalPath := filepath.Join(baseDir, "update-now.signal")
	raw, err := os.ReadFile(signalPath)
	if err != nil {
		t.Fatalf("read signal file: %v", err)
	}
	if !strings.Contains(strings.ToLower(string(raw)), "full-update") {
		t.Fatalf("signal content should include full-update marker, got: %q", string(raw))
	}
}

func TestActionClearSessionHistoryRoute(t *testing.T) {
	baseDir := t.TempDir()
	store, err := clientpkg.NewStore(filepath.Join(baseDir, "db", "client.sqlite3"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	mon := NewMonitor(baseDir)

	mux := http.NewServeMux()
	registerRoutes(mux, mon)

	req := httptest.NewRequest(http.MethodPost, "/api/action/clear-session-history", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d, want=%d, body=%s", rr.Code, http.StatusOK, rr.Body.String())
	}

	var body map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if body["action"] != "clear-session-history" {
		t.Fatalf("action=%q, want clear-session-history", body["action"])
	}
}

func TestSessionAPIListsSessionsAndMessages(t *testing.T) {
	base := t.TempDir()
	store, err := clientpkg.NewStore(filepath.Join(base, "db", "client.sqlite3"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	if err := store.SaveSession(ctx, &clientpkg.SessionRecord{
		ID:           "sess-1",
		ProjectName:  "proj1",
		Status:       clientpkg.SessionActive,
		AgentType:    "claude",
		AgentJSON:    `{"title":"Task"}`,
		Title:        "Task",
		CreatedAt:    time.Date(2026, 4, 12, 10, 0, 0, 0, time.UTC),
		LastActiveAt: time.Date(2026, 4, 12, 10, 1, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}
	if err := store.UpsertSessionPrompt(ctx, clientpkg.SessionPromptRecord{
		SessionID:   "sess-1",
		PromptIndex: 1,
		UpdatedAt:   time.Date(2026, 4, 12, 10, 1, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("UpsertSessionPrompt: %v", err)
	}
	turnsJSON := clientpkg.EncodeStoredTurns([]clientpkg.SessionTurnRecord{
		{
			SessionID:   "sess-1",
			PromptIndex: 1,
			TurnIndex:   1,
			UpdateJSON:  `{"method":"session/prompt","params":{"prompt":[{"type":"text","text":"hello"}]}}`,
		},
	})
	if err := store.UpsertSessionPrompt(ctx, clientpkg.SessionPromptRecord{
		SessionID:   "sess-1",
		PromptIndex: 1,
		UpdatedAt:   time.Date(2026, 4, 12, 10, 1, 0, 0, time.UTC),
		TurnsJSON:   turnsJSON,
		TurnIndex:   1,
		StopReason:  "done",
	}); err != nil {
		t.Fatalf("UpsertSessionPrompt with turns: %v", err)
	}

	mon := NewMonitor(base)
	mux := http.NewServeMux()
	registerRoutes(mux, mon)

	listReq := httptest.NewRequest(http.MethodGet, "/api/sessions", nil)
	listRR := httptest.NewRecorder()
	mux.ServeHTTP(listRR, listReq)
	if listRR.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", listRR.Code, listRR.Body.String())
	}
	var listBody struct {
		Sessions []map[string]any `json:"sessions"`
	}
	if err := json.Unmarshal(listRR.Body.Bytes(), &listBody); err != nil {
		t.Fatalf("unmarshal list body: %v", err)
	}
	if len(listBody.Sessions) != 1 {
		t.Fatalf("sessions len = %d, want 1", len(listBody.Sessions))
	}

	msgReq := httptest.NewRequest(http.MethodGet, "/api/sessions/sess-1/messages?limit=10", nil)
	msgRR := httptest.NewRecorder()
	mux.ServeHTTP(msgRR, msgReq)
	if msgRR.Code != http.StatusOK {
		t.Fatalf("messages status=%d body=%s", msgRR.Code, msgRR.Body.String())
	}
	var msgBody struct {
		Messages []map[string]any `json:"messages"`
	}
	if err := json.Unmarshal(msgRR.Body.Bytes(), &msgBody); err != nil {
		t.Fatalf("unmarshal message body: %v", err)
	}
	if len(msgBody.Messages) != 1 {
		t.Fatalf("messages len = %d, want 1", len(msgBody.Messages))
	}
	if got := msgBody.Messages[0]["body"]; got != "hello" {
		t.Fatalf("messages[0].body = %v, want hello", got)
	}
}

func TestSessionMessagesRequiresSessionID(t *testing.T) {
	handler := handleSessionMessages(NewMonitor(t.TempDir()))
	req := httptest.NewRequest(http.MethodGet, "/api/sessions/%20/messages", nil)
	req.SetPathValue("sessionID", " ")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want=%d, body=%s", rr.Code, http.StatusBadRequest, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "sessionID is required") {
		t.Fatalf("body=%q, want sessionID error", rr.Body.String())
	}
}

type stubHubTransport struct {
	lastStatusHub string
	lastActionHub string
	lastAction    string
	actionErr     error
	actionCalls   int
}

func (s *stubHubTransport) ListHub(context.Context) ([]HubInfo, error) {
	return []HubInfo{{HubID: "hub-a", Online: true}}, nil
}

func (s *stubHubTransport) MonitorStatus(_ context.Context, hubID string) (*ServiceStatus, error) {
	s.lastStatusHub = hubID
	return &ServiceStatus{Running: true, Timestamp: "2026-04-16T00:00:00Z"}, nil
}

func (s *stubHubTransport) MonitorLog(context.Context, MonitorLogRequest) (*LogResult, error) {
	return &LogResult{}, nil
}

func (s *stubHubTransport) MonitorDB(context.Context, string) (*DBTablesResult, error) {
	return &DBTablesResult{}, nil
}

func (s *stubHubTransport) MonitorAction(_ context.Context, hubID string, action string) error {
	s.lastActionHub = hubID
	s.lastAction = action
	s.actionCalls++
	return s.actionErr
}

func (s *stubHubTransport) ProjectList(context.Context, string) ([]RegistryProject, error) {
	return nil, nil
}

func TestRoutes_StatusByHubID(t *testing.T) {
	mon := NewMonitor(t.TempDir())
	stub := &stubHubTransport{}
	mon.transport = stub
	mon.defaultHubID = "hub-a"

	mux := http.NewServeMux()
	registerRoutes(mux, mon)

	req := httptest.NewRequest(http.MethodGet, "/api/status?hubId=hub-2", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if stub.lastStatusHub != "hub-2" {
		t.Fatalf("status hub=%q want hub-2", stub.lastStatusHub)
	}
}

func TestRoutes_ActionByHubID(t *testing.T) {
	mon := NewMonitor(t.TempDir())
	stub := &stubHubTransport{}
	mon.transport = stub
	mon.defaultHubID = "hub-a"

	mux := http.NewServeMux()
	registerRoutes(mux, mon)

	req := httptest.NewRequest(http.MethodPost, "/api/action/restart?hubId=hub-9", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if stub.lastActionHub != "hub-9" {
		t.Fatalf("action hub=%q want hub-9", stub.lastActionHub)
	}
	if stub.lastAction != "restart" {
		t.Fatalf("action=%q want restart", stub.lastAction)
	}
}

func TestRoutes_LocalActionBypassesTransport(t *testing.T) {
	baseDir := t.TempDir()
	mon := NewMonitor(baseDir)
	stub := &stubHubTransport{actionErr: errors.New("registry down")}
	mon.transport = stub
	mon.defaultHubID = "local-hub"

	mux := http.NewServeMux()
	registerRoutes(mux, mon)

	req := httptest.NewRequest(http.MethodPost, "/api/action/update-publish?hubId=local-hub", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if stub.actionCalls != 0 {
		t.Fatalf("transport should not be called for local action, got calls=%d", stub.actionCalls)
	}
	signalPath := filepath.Join(baseDir, "update-now.signal")
	if _, err := os.Stat(signalPath); err != nil {
		t.Fatalf("expected local signal file, err=%v", err)
	}
}

func TestRoutes_RemoteStartReturnsStructuredPolicyError(t *testing.T) {
	mon := NewMonitor(t.TempDir())
	stub := &stubHubTransport{}
	mon.transport = stub
	mon.defaultHubID = "local-hub"

	mux := http.NewServeMux()
	registerRoutes(mux, mon)

	req := httptest.NewRequest(http.MethodPost, "/api/action/start?hubId=remote-hub", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusConflict {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var body map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if body["code"] != "REMOTE_START_UNSUPPORTED" {
		t.Fatalf("code=%q", body["code"])
	}
	if strings.TrimSpace(body["hint"]) == "" {
		t.Fatalf("hint should not be empty")
	}
	if stub.actionCalls != 0 {
		t.Fatalf("transport should not be called for remote start policy rejection")
	}
}
