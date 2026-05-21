package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestDesktopAssetHandlerServesRootIndex(t *testing.T) {
	handler := newDesktopAssetHandler(fstest.MapFS{
		"index.html": {Data: []byte("<html>WheelMaker</html>")},
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "WheelMaker") {
		t.Fatalf("body=%q should include index content", rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); !strings.Contains(got, "text/html") {
		t.Fatalf("Content-Type=%q should be text/html", got)
	}
}

func TestDesktopAssetHandlerServesStaticAsset(t *testing.T) {
	handler := newDesktopAssetHandler(fstest.MapFS{
		"index.html": {Data: []byte("<html></html>")},
		"bundle.js":  {Data: []byte("console.log('wm')")},
	})

	req := httptest.NewRequest(http.MethodGet, "/bundle.js", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Body.String(); got != "console.log('wm')" {
		t.Fatalf("body=%q", got)
	}
}

func TestDesktopAssetHandlerFallsBackToIndexForWorkspaceRoute(t *testing.T) {
	handler := newDesktopAssetHandler(fstest.MapFS{
		"index.html": {Data: []byte("<html>shell</html>")},
	})

	req := httptest.NewRequest(http.MethodGet, "/settings/skills", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Body.String(); !strings.Contains(got, "shell") {
		t.Fatalf("body=%q should be index fallback", got)
	}
}

func TestDesktopAssetHandlerDoesNotFallbackForMissingFileAsset(t *testing.T) {
	handler := newDesktopAssetHandler(fstest.MapFS{
		"index.html": {Data: []byte("<html>shell</html>")},
	})

	req := httptest.NewRequest(http.MethodGet, "/missing.js", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status=%d want %d", rec.Code, http.StatusNotFound)
	}
}

func TestStartDesktopAssetServerUsesLoopback(t *testing.T) {
	srv, err := startDesktopAssetServer(fstest.MapFS{
		"index.html": {Data: []byte("<html>loopback</html>")},
	})
	if err != nil {
		t.Fatalf("startDesktopAssetServer: %v", err)
	}
	defer srv.Close()

	resp, err := http.Get(srv.URL())
	if err != nil {
		t.Fatalf("GET server root: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d want %d", resp.StatusCode, http.StatusOK)
	}
	if !strings.Contains(string(body), "loopback") {
		t.Fatalf("body=%q should include embedded index", string(body))
	}
	if srv.URL() != "http://127.0.0.1:9632/" {
		t.Fatalf("url=%q should use stable desktop storage origin", srv.URL())
	}
}
