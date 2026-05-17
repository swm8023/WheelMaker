package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const testMonitorToken = "test-monitor-token"

func writeMonitorTestConfig(t *testing.T, baseDir string, token string) {
	t.Helper()
	data := `{"projects":[],"registry":{"token":` + strconv.Quote(token) + `,"hubId":"local"},"monitor":{"port":9631}}`
	if err := os.WriteFile(filepath.Join(baseDir, "config.json"), []byte(data), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

func newAuthedMonitorMux(t *testing.T) (*http.ServeMux, string) {
	t.Helper()
	baseDir := t.TempDir()
	writeMonitorTestConfig(t, baseDir, testMonitorToken)
	mon := NewMonitor(baseDir)
	mux := http.NewServeMux()
	registerRoutes(mux, mon)
	return mux, baseDir
}

func TestLoadMonitorRuntimeConfigRequiresRegistryToken(t *testing.T) {
	baseDir := t.TempDir()
	writeMonitorTestConfig(t, baseDir, "")

	_, err := loadMonitorRuntimeConfig(baseDir)
	if err == nil || !strings.Contains(err.Error(), "registry.token is required for monitor authentication") {
		t.Fatalf("err=%v, want registry token required error", err)
	}
}

func TestRoutesUnauthenticatedDashboardShowsLogin(t *testing.T) {
	mux, _ := newAuthedMonitorMux(t)

	req := httptest.NewRequest(http.MethodGet, "/monitor/", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d, want=%d, body=%s", rr.Code, http.StatusOK, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "Connect to WheelMaker Monitor") {
		t.Fatalf("dashboard should show monitor login page, body=%s", rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), `id="hub-select"`) {
		t.Fatalf("unauthenticated dashboard should not render monitor shell")
	}
}

func TestRoutesUnauthenticatedAPIReturnsUnauthorized(t *testing.T) {
	mux, _ := newAuthedMonitorMux(t)

	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d, want=%d, body=%s", rr.Code, http.StatusUnauthorized, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "unauthorized") {
		t.Fatalf("body=%q, want unauthorized error", rr.Body.String())
	}
}

func TestRoutesBearerTokenAllowsAPI(t *testing.T) {
	mux, _ := newAuthedMonitorMux(t)

	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	req.Header.Set("Authorization", "Bearer "+testMonitorToken)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d, want=%d, body=%s", rr.Code, http.StatusOK, rr.Body.String())
	}
}

func TestMonitorLoginCookieRequiresCSRFForPostActions(t *testing.T) {
	mux, baseDir := newAuthedMonitorMux(t)
	cookies := loginMonitor(t, mux, testMonitorToken, "")
	csrf := cookieValue(cookies, "wm_monitor_csrf")
	if csrf == "" {
		t.Fatalf("login should set csrf cookie: %#v", cookies)
	}

	noCSRFReq := httptest.NewRequest(http.MethodPost, "/api/action/update-publish", nil)
	for _, cookie := range cookies {
		noCSRFReq.AddCookie(cookie)
	}
	noCSRFRR := httptest.NewRecorder()
	mux.ServeHTTP(noCSRFRR, noCSRFReq)
	if noCSRFRR.Code != http.StatusForbidden {
		t.Fatalf("status=%d, want=%d, body=%s", noCSRFRR.Code, http.StatusForbidden, noCSRFRR.Body.String())
	}

	req := httptest.NewRequest(http.MethodPost, "/api/action/update-publish", nil)
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}
	req.Header.Set("X-WheelMaker-Monitor-CSRF", csrf)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d, want=%d, body=%s", rr.Code, http.StatusOK, rr.Body.String())
	}
	if _, err := os.Stat(filepath.Join(baseDir, "update-now.signal")); err != nil {
		t.Fatalf("expected update signal file after csrf-authenticated action: %v", err)
	}
}

func TestBearerTokenPostActionSkipsCSRF(t *testing.T) {
	mux, baseDir := newAuthedMonitorMux(t)

	req := httptest.NewRequest(http.MethodPost, "/api/action/update-publish", nil)
	req.Header.Set("Authorization", "Bearer "+testMonitorToken)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d, want=%d, body=%s", rr.Code, http.StatusOK, rr.Body.String())
	}
	if _, err := os.Stat(filepath.Join(baseDir, "update-now.signal")); err != nil {
		t.Fatalf("expected update signal file after bearer-authenticated action: %v", err)
	}
}

func TestMonitorLoginUsesSecureCookiesBehindHTTPSProxy(t *testing.T) {
	mux, _ := newAuthedMonitorMux(t)
	cookies := loginMonitor(t, mux, testMonitorToken, "https")

	sessionCookie := findCookie(cookies, "wm_monitor_session")
	if sessionCookie == nil {
		t.Fatalf("session cookie missing: %#v", cookies)
	}
	if !sessionCookie.HttpOnly {
		t.Fatalf("session cookie should be HttpOnly")
	}
	if !sessionCookie.Secure {
		t.Fatalf("session cookie should be Secure behind HTTPS proxy")
	}
}

func loginMonitor(t *testing.T, mux *http.ServeMux, token string, forwardedProto string) []*http.Cookie {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"token":`+strconv.Quote(token)+`}`))
	req.Header.Set("Content-Type", "application/json")
	if forwardedProto != "" {
		req.Header.Set("X-Forwarded-Proto", forwardedProto)
	}
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("login status=%d, want=%d, body=%s", rr.Code, http.StatusOK, rr.Body.String())
	}
	return rr.Result().Cookies()
}

func findCookie(cookies []*http.Cookie, name string) *http.Cookie {
	for _, cookie := range cookies {
		if cookie.Name == name {
			return cookie
		}
	}
	return nil
}

func cookieValue(cookies []*http.Cookie, name string) string {
	if cookie := findCookie(cookies, name); cookie != nil {
		return cookie.Value
	}
	return ""
}

func authorizeMonitorRequest(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+testMonitorToken)
}
