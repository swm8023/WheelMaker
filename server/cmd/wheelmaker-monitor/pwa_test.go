package main

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
)

func TestManifestScopeForMonitorPrefix(t *testing.T) {
	req := httptest.NewRequest("GET", "/monitor/manifest.webmanifest", nil)
	rr := httptest.NewRecorder()

	handleManifest().ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("status=%d, want 200", rr.Code)
	}

	var payload map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("manifest json parse error: %v", err)
	}
	if payload["start_url"] != "/monitor/" {
		t.Fatalf("start_url=%v, want /monitor/", payload["start_url"])
	}
	if payload["scope"] != "/monitor/" {
		t.Fatalf("scope=%v, want /monitor/", payload["scope"])
	}
}

func TestManifestScopeForRootPrefix(t *testing.T) {
	req := httptest.NewRequest("GET", "/manifest.webmanifest", nil)
	rr := httptest.NewRecorder()

	handleManifest().ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("status=%d, want 200", rr.Code)
	}

	var payload map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("manifest json parse error: %v", err)
	}
	if payload["start_url"] != "/" {
		t.Fatalf("start_url=%v, want /", payload["start_url"])
	}
	if payload["scope"] != "/" {
		t.Fatalf("scope=%v, want /", payload["scope"])
	}
}

func TestServiceWorkerServed(t *testing.T) {
	req := httptest.NewRequest("GET", "/service-worker.js", nil)
	rr := httptest.NewRecorder()

	handleServiceWorker().ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("status=%d, want 200", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); ct == "" {
		t.Fatalf("service worker content type should be set")
	}
	if rr.Body.Len() == 0 {
		t.Fatalf("service worker body should not be empty")
	}
}
