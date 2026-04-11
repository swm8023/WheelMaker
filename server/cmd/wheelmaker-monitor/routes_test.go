package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
