package main

import (
	"context"
	"encoding/json"
	clientpkg "github.com/swm8023/wheelmaker/internal/hub/client"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

func TestSessionAPIListsSessionsAndMessages(t *testing.T) {
	base := t.TempDir()
	store, err := clientpkg.NewStore(filepath.Join(base, "db", "client.sqlite3"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	if err := store.SaveSession(ctx, &clientpkg.SessionRecord{
		ID:            "sess-1",
		ProjectName:   "proj1",
		Status:        clientpkg.SessionActive,
		Title:         "Task",
		LastMessageAt: time.Date(2026, 4, 12, 10, 1, 0, 0, time.UTC),
		CreatedAt:     time.Date(2026, 4, 12, 10, 0, 0, 0, time.UTC),
		LastActiveAt:  time.Date(2026, 4, 12, 10, 1, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}
	if err := store.AppendSessionMessage(ctx, clientpkg.SessionMessageRecord{
		MessageID:   "msg-1",
		ProjectName: "proj1",
		SessionID:   "sess-1",
		Role:        "user",
		Kind:        "text",
		Body:        "hello",
		CreatedAt:   time.Date(2026, 4, 12, 10, 1, 0, 0, time.UTC),
		UpdatedAt:   time.Date(2026, 4, 12, 10, 1, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("AppendSessionMessage: %v", err)
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
