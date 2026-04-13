package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	clientpkg "github.com/swm8023/wheelmaker/internal/hub/client"
)

func TestGetDBTablesIncludesSessionMessages(t *testing.T) {
	base := t.TempDir()
	store, err := clientpkg.NewStore(filepath.Join(base, "db", "client.sqlite3"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	if err := store.SaveSession(ctx, &clientpkg.SessionRecord{
		ID:          "sess-1",
		ProjectName: "proj1",
		Status:      clientpkg.SessionActive,
		CreatedAt:   time.Date(2026, 4, 12, 10, 0, 0, 0, time.UTC),
		LastActiveAt: time.Date(2026, 4, 12, 10, 0, 0, 0, time.UTC),
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
	res := mon.GetDBTables()
	if res.Error != "" {
		t.Fatalf("GetDBTables error: %s", res.Error)
	}
	found := false
	for _, table := range res.Tables {
		if table.Name == "session_messages" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("session_messages table missing: %#v", res.Tables)
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
		ID:                 "sess-1",
		ProjectName:        "proj1",
		Status:             clientpkg.SessionActive,
		Title:              "Task",
		LastMessagePreview: "hello",
		LastMessageAt:      time.Date(2026, 4, 12, 10, 1, 0, 0, time.UTC),
		MessageCount:       1,
		CreatedAt:          time.Date(2026, 4, 12, 10, 0, 0, 0, time.UTC),
		LastActiveAt:       time.Date(2026, 4, 12, 10, 1, 0, 0, time.UTC),
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