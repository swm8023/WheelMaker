package main

import (
	"errors"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

type recordingLauncher struct {
	url  string
	opts desktopWindowOptions
	err  error
}

func (r *recordingLauncher) Launch(url string, opts desktopWindowOptions) error {
	r.url = url
	r.opts = opts
	return r.err
}

func TestRunDesktopAppLaunchesStableLoopbackStorageOrigin(t *testing.T) {
	launcher := &recordingLauncher{}
	err := runDesktopApp(fstest.MapFS{
		"index.html": {Data: []byte("<html>desktop</html>")},
	}, launcher)

	if err != nil {
		t.Fatalf("runDesktopApp: %v", err)
	}
	if launcher.url != "http://127.0.0.1:9632/" {
		t.Fatalf("url=%q should use stable desktop storage origin", launcher.url)
	}
}

func TestRunDesktopAppLaunchesWithCustomTitleBarAndIcon(t *testing.T) {
	launcher := &recordingLauncher{}
	err := runDesktopApp(fstest.MapFS{
		"index.html": {Data: []byte("<html>desktop</html>")},
	}, launcher)

	if err != nil {
		t.Fatalf("runDesktopApp: %v", err)
	}
	if launcher.opts.Title != "WheelMaker - Embedded" {
		t.Fatalf("title=%q, want WheelMaker - Embedded", launcher.opts.Title)
	}
	if !launcher.opts.CustomTitleBar {
		t.Fatal("expected custom title bar to be enabled")
	}
	if launcher.opts.IconID != desktopResourceIconID {
		t.Fatalf("IconID=%d, want %d", launcher.opts.IconID, desktopResourceIconID)
	}
	if launcher.opts.ThemeColor != desktopTitleBarThemeColor {
		t.Fatalf("ThemeColor=%q, want %q", launcher.opts.ThemeColor, desktopTitleBarThemeColor)
	}
}

func TestRunDesktopAppReturnsActionableWebViewError(t *testing.T) {
	launcher := &recordingLauncher{err: errWebView2Unavailable}
	err := runDesktopApp(fstest.MapFS{
		"index.html": {Data: []byte("<html>desktop</html>")},
	}, launcher)

	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "Microsoft Edge WebView2 Runtime") {
		t.Fatalf("error=%q should mention WebView2 runtime", err.Error())
	}
}

func TestRunDesktopAppReportsMissingIndex(t *testing.T) {
	launcher := &recordingLauncher{}
	err := runDesktopApp(fstest.MapFS{}, launcher)

	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("error=%v should wrap fs.ErrNotExist", err)
	}
}

func TestDesktopRuntimeInitScriptExposesWindowBridge(t *testing.T) {
	script := desktopRuntimeInitScript()

	for _, want := range []string{
		"window.WheelMakerDesktop",
		"enabled: true",
		desktopStartDragBinding,
		desktopMinimizeBinding,
		desktopToggleMaximizeBinding,
		desktopCloseBinding,
		desktopGetWebSourceBinding,
		desktopSetWebSourceBinding,
		desktopSetRemoteWebBinding,
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("desktop runtime init script missing %q: %s", want, script)
		}
	}
}

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

func TestDesktopAssetHandlerDoesNotFallbackForRegistryWebSocketPath(t *testing.T) {
	handler := newDesktopAssetHandler(fstest.MapFS{
		"index.html": {Data: []byte("<html>shell</html>")},
	})

	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
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

type memoryDesktopWebSourceConfigStore struct {
	config desktopWebSourceConfig
	saved  desktopWebSourceConfig
}

func (s *memoryDesktopWebSourceConfigStore) Load() (desktopWebSourceConfig, error) {
	return s.config, nil
}

func (s *memoryDesktopWebSourceConfigStore) Save(config desktopWebSourceConfig) error {
	s.saved = config
	s.config = config
	return nil
}

func TestDesktopWebSourceCandidateReplacesRemoteForSecureRegistry(t *testing.T) {
	store := &memoryDesktopWebSourceConfigStore{config: desktopWebSourceConfig{
		WebSourcePreference:     desktopWebSourcePreferenceAuto,
		RemoteWebURL:            "https://old.example.com/",
		RemoteWebRegistryOrigin: "wss://old.example.com",
	}}
	runtime := newDesktopWebSourceRuntime(store, nil)
	runtime.SetActualSource(desktopWebSourceActualEmbedded)

	state, err := runtime.SetRemoteCandidate(desktopRemoteWebCandidate{
		RegistryAddress: "wss://new.example.com/ws",
		RemoteWebURL:    "https://new.example.com/",
	})
	if err != nil {
		t.Fatalf("SetRemoteCandidate: %v", err)
	}

	if store.saved.RemoteWebURL != "https://new.example.com/" {
		t.Fatalf("RemoteWebURL=%q", store.saved.RemoteWebURL)
	}
	if store.saved.RemoteWebRegistryOrigin != "wss://new.example.com" {
		t.Fatalf("RemoteWebRegistryOrigin=%q", store.saved.RemoteWebRegistryOrigin)
	}
	if state.RemoteHost != "new.example.com" {
		t.Fatalf("RemoteHost=%q", state.RemoteHost)
	}
	if state.ActualSource != desktopWebSourceActualEmbedded {
		t.Fatalf("ActualSource=%q should not change current window source", state.ActualSource)
	}
}

func TestDesktopAssetHandlerDoesNotUseNewRemoteCandidateUntilActualSourceChanges(t *testing.T) {
	remoteHits := 0
	remote := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		remoteHits++
		_, _ = io.WriteString(w, "console.log('remote')")
	}))
	defer remote.Close()

	runtime := newDesktopWebSourceRuntime(&memoryDesktopWebSourceConfigStore{config: desktopWebSourceConfig{
		WebSourcePreference: desktopWebSourcePreferenceAuto,
	}}, remote.Client())
	runtime.mu.Lock()
	runtime.config.RemoteWebURL = remote.URL + "/"
	runtime.actual = desktopWebSourceActualEmbedded
	runtime.actualRemoteURL = ""
	runtime.mu.Unlock()

	handler := newDesktopAssetHandlerWithWebSource(fstest.MapFS{
		"index.html": {Data: []byte("<html>embedded</html>")},
		"bundle.js":  {Data: []byte("console.log('embedded')")},
	}, runtime)

	req := httptest.NewRequest(http.MethodGet, "/bundle.js", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if got := rec.Body.String(); got != "console.log('embedded')" {
		t.Fatalf("body=%q", got)
	}
	if remoteHits != 0 {
		t.Fatalf("remoteHits=%d, want 0", remoteHits)
	}
}

func TestDesktopWebSourceCandidateClearsRemoteForLocalRegistry(t *testing.T) {
	store := &memoryDesktopWebSourceConfigStore{config: desktopWebSourceConfig{
		WebSourcePreference:     desktopWebSourcePreferenceAuto,
		RemoteWebURL:            "https://old.example.com/",
		RemoteWebRegistryOrigin: "wss://old.example.com",
	}}
	runtime := newDesktopWebSourceRuntime(store, nil)
	runtime.SetActualSource(desktopWebSourceActualEmbedded)

	state, err := runtime.SetRemoteCandidate(desktopRemoteWebCandidate{
		RegistryAddress: "ws://127.0.0.1:9630/ws",
		RemoteWebURL:    "",
	})
	if err != nil {
		t.Fatalf("SetRemoteCandidate: %v", err)
	}

	if store.saved.RemoteWebURL != "" {
		t.Fatalf("RemoteWebURL=%q, want cleared", store.saved.RemoteWebURL)
	}
	if store.saved.RemoteWebRegistryOrigin != "" {
		t.Fatalf("RemoteWebRegistryOrigin=%q, want cleared", store.saved.RemoteWebRegistryOrigin)
	}
	if state.ActualSource != desktopWebSourceActualEmbedded {
		t.Fatalf("ActualSource=%q", state.ActualSource)
	}
}

func TestDesktopWebSourcePreferencePersistsWithoutClearingRemoteURL(t *testing.T) {
	store := &memoryDesktopWebSourceConfigStore{config: desktopWebSourceConfig{
		WebSourcePreference:     desktopWebSourcePreferenceAuto,
		RemoteWebURL:            "https://remote.example.com/",
		RemoteWebRegistryOrigin: "wss://remote.example.com",
	}}
	runtime := newDesktopWebSourceRuntime(store, nil)

	state, err := runtime.SetPreference(desktopWebSourcePreferenceEmbedded)
	if err != nil {
		t.Fatalf("SetPreference: %v", err)
	}

	if store.saved.WebSourcePreference != desktopWebSourcePreferenceEmbedded {
		t.Fatalf("WebSourcePreference=%q", store.saved.WebSourcePreference)
	}
	if store.saved.RemoteWebURL != "https://remote.example.com/" {
		t.Fatalf("RemoteWebURL=%q should be retained", store.saved.RemoteWebURL)
	}
	if state.Preference != desktopWebSourcePreferenceEmbedded {
		t.Fatalf("Preference=%q", state.Preference)
	}
	if state.ActualSource != desktopWebSourceActualEmbedded {
		t.Fatalf("ActualSource=%q", state.ActualSource)
	}
}

func TestDesktopWebSourceRefreshActualSourceUsesEmbeddedWhenRemoteFails(t *testing.T) {
	remote := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "remote down", http.StatusInternalServerError)
	}))
	defer remote.Close()

	runtime := newDesktopWebSourceRuntime(&memoryDesktopWebSourceConfigStore{config: desktopWebSourceConfig{
		WebSourcePreference: desktopWebSourcePreferenceAuto,
	}}, remote.Client())
	runtime.mu.Lock()
	runtime.config.WebSourcePreference = desktopWebSourcePreferenceAuto
	runtime.config.RemoteWebURL = remote.URL + "/"
	runtime.actual = desktopWebSourceActualRemote
	runtime.actualRemoteURL = remote.URL + "/"
	runtime.mu.Unlock()

	state := runtime.RefreshActualSource()

	if state.ActualSource != desktopWebSourceActualEmbedded {
		t.Fatalf("ActualSource=%q", state.ActualSource)
	}
	if state.DisplayTitle != "WheelMaker - Embedded" {
		t.Fatalf("DisplayTitle=%q", state.DisplayTitle)
	}
}

func TestDesktopAssetHandlerPrefersRemoteAndFallsBackToEmbedded(t *testing.T) {
	remoteFails := false
	remote := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if remoteFails {
			http.Error(w, "remote down", http.StatusInternalServerError)
			return
		}
		if r.URL.Path == "/bundle.js" {
			w.Header().Set("Content-Type", "application/javascript")
			_, _ = io.WriteString(w, "console.log('remote')")
			return
		}
		http.NotFound(w, r)
	}))
	defer remote.Close()

	runtime := newDesktopWebSourceRuntime(&memoryDesktopWebSourceConfigStore{config: desktopWebSourceConfig{
		WebSourcePreference: desktopWebSourcePreferenceAuto,
		RemoteWebURL:        remote.URL + "/",
	}}, remote.Client())
	runtime.mu.Lock()
	runtime.config.WebSourcePreference = desktopWebSourcePreferenceAuto
	runtime.config.RemoteWebURL = remote.URL + "/"
	runtime.actual = desktopWebSourceActualRemote
	runtime.actualRemoteURL = remote.URL + "/"
	runtime.mu.Unlock()
	handler := newDesktopAssetHandlerWithWebSource(fstest.MapFS{
		"index.html": {Data: []byte("<html>embedded</html>")},
		"bundle.js":  {Data: []byte("console.log('embedded')")},
	}, runtime)

	req := httptest.NewRequest(http.MethodGet, "/bundle.js", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if got := rec.Body.String(); got != "console.log('remote')" {
		t.Fatalf("remote body=%q", got)
	}
	if runtime.State().ActualSource != desktopWebSourceActualRemote {
		t.Fatalf("ActualSource=%q", runtime.State().ActualSource)
	}

	remoteFails = true
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if got := rec.Body.String(); !strings.Contains(got, "embedded") {
		t.Fatalf("fallback body=%q", got)
	}
	if runtime.State().ActualSource != desktopWebSourceActualEmbedded {
		t.Fatalf("ActualSource=%q", runtime.State().ActualSource)
	}
}

func TestDesktopAssetHandlerFallsBackToRemoteIndexForWorkspaceRoute(t *testing.T) {
	remote := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/index.html" || r.URL.Path == "/" {
			_, _ = io.WriteString(w, "<html>remote shell</html>")
			return
		}
		http.NotFound(w, r)
	}))
	defer remote.Close()

	runtime := newDesktopWebSourceRuntime(&memoryDesktopWebSourceConfigStore{config: desktopWebSourceConfig{
		WebSourcePreference: desktopWebSourcePreferenceAuto,
	}}, remote.Client())
	runtime.mu.Lock()
	runtime.config.WebSourcePreference = desktopWebSourcePreferenceAuto
	runtime.config.RemoteWebURL = remote.URL + "/"
	runtime.actual = desktopWebSourceActualRemote
	runtime.actualRemoteURL = remote.URL + "/"
	runtime.mu.Unlock()
	handler := newDesktopAssetHandlerWithWebSource(fstest.MapFS{
		"index.html": {Data: []byte("<html>embedded shell</html>")},
	}, runtime)

	req := httptest.NewRequest(http.MethodGet, "/settings/update", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if got := rec.Body.String(); !strings.Contains(got, "remote shell") {
		t.Fatalf("body=%q should use remote shell fallback", got)
	}
}
