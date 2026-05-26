package main

import (
	"context"
	"encoding/json"
	"errors"
	clientpkg "github.com/swm8023/wheelmaker/internal/hub/client"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestResolveLogFilePath_PrefersLogDir(t *testing.T) {
	base := t.TempDir()
	m := NewMonitor(base)

	newPath := filepath.Join(base, "log", "hub.log")
	if err := os.MkdirAll(filepath.Dir(newPath), 0o755); err != nil {
		t.Fatalf("mkdir log dir: %v", err)
	}
	if err := os.WriteFile(newPath, []byte("new"), 0o644); err != nil {
		t.Fatalf("write new log: %v", err)
	}
	oldPath := filepath.Join(base, "hub.log")
	if err := os.WriteFile(oldPath, []byte("old"), 0o644); err != nil {
		t.Fatalf("write old log: %v", err)
	}

	got := m.resolveLogFilePath("hub")
	if got != newPath {
		t.Fatalf("resolveLogFilePath(hub)=%q, want %q", got, newPath)
	}
}

func TestLaunchAgentLabels(t *testing.T) {
	all := allLaunchAgentLabels()
	want := []string{launchAgentHubLabel, launchAgentMonitorLabel, launchAgentUpdaterLabel}
	if strings.Join(all, ",") != strings.Join(want, ",") {
		t.Fatalf("labels=%#v want %#v", all, want)
	}
	managed := managedLaunchAgentLabels()
	if strings.Join(managed, ",") != strings.Join([]string{launchAgentHubLabel, launchAgentUpdaterLabel}, ",") {
		t.Fatalf("managed labels=%#v", managed)
	}
}

func TestLaunchAgentPlistPath(t *testing.T) {
	home := filepath.Join("Users", "me")
	got := launchAgentPlistPath(home, launchAgentHubLabel)
	want := filepath.Join(home, "Library", "LaunchAgents", launchAgentHubLabel+".plist")
	if got != want {
		t.Fatalf("plist path=%q want %q", got, want)
	}
}

func TestParseLaunchAgentServiceInfo(t *testing.T) {
	running := parseLaunchAgentServiceInfo(launchAgentHubLabel, true, []byte("state = running\npid = 123\n"))
	if !running.Installed || running.Status != "Running" || running.StartType != "LaunchAgent" {
		t.Fatalf("running info=%#v", running)
	}
	stopped := parseLaunchAgentServiceInfo(launchAgentHubLabel, true, []byte("state = waiting\n"))
	if stopped.Status != "Stopped" {
		t.Fatalf("stopped info=%#v", stopped)
	}
	missing := parseLaunchAgentServiceInfo(launchAgentHubLabel, false, nil)
	if missing.Installed || missing.Status != "NotInstalled" {
		t.Fatalf("missing info=%#v", missing)
	}
}

func TestSystemdUserServiceNames(t *testing.T) {
	all := allSystemdUserServiceNames()
	want := []string{systemdUserHubService, systemdUserMonitorService, systemdUserUpdaterService}
	if strings.Join(all, ",") != strings.Join(want, ",") {
		t.Fatalf("systemd services=%#v want %#v", all, want)
	}
	managed := managedSystemdUserServiceNames()
	if strings.Join(managed, ",") != strings.Join([]string{systemdUserHubService, systemdUserUpdaterService}, ",") {
		t.Fatalf("managed systemd services=%#v", managed)
	}
}

func TestSystemdUserUnitPath(t *testing.T) {
	home := filepath.Join("home", "me")
	got := systemdUserUnitPath(home, systemdUserHubService)
	want := filepath.Join(home, ".config", "systemd", "user", systemdUserHubService)
	if got != want {
		t.Fatalf("unit path=%q want %q", got, want)
	}
}

func TestParseSystemdUserServiceInfo(t *testing.T) {
	running := parseSystemdUserServiceInfo(systemdUserHubService, true, []byte("LoadState=loaded\nActiveState=active\nUnitFileState=enabled\n"))
	if !running.Installed || running.Status != "Running" || running.StartType != "systemd --user" {
		t.Fatalf("running info=%#v", running)
	}
	stopped := parseSystemdUserServiceInfo(systemdUserHubService, true, []byte("LoadState=loaded\nActiveState=inactive\nUnitFileState=enabled\n"))
	if stopped.Status != "Stopped" {
		t.Fatalf("stopped info=%#v", stopped)
	}
	missing := parseSystemdUserServiceInfo(systemdUserHubService, false, []byte("LoadState=not-found\nActiveState=inactive\n"))
	if missing.Installed || missing.Status != "NotInstalled" {
		t.Fatalf("missing info=%#v", missing)
	}
}

func TestParseUnixWheelmakerProcessesExcludesUpdaterAndShell(t *testing.T) {
	out := []byte(`123 Mon May 18 20:01:02 2026 /Users/me/.wheelmaker/bin/wheelmaker -d
124 Mon May 18 20:01:03 2026 /Users/me/.wheelmaker/bin/wheelmaker --hub-worker
125 Mon May 18 20:01:04 2026 /Users/me/.wheelmaker/bin/wheelmaker-updater --repo /repo
126 Mon May 18 20:01:05 2026 bash -lc wheelmaker --hub-worker
127 Mon May 18 20:01:06 2026 /Users/me/.wheelmaker/bin/wheelmaker-monitor
128 Mon May 18 20:01:07 2026 grep wheelmaker
129 Mon May 18 20:01:08 2026 /bin/bash /usr/local/bin/wheelmaker-wrapper --hub-worker
`)

	procs := parseUnixWheelmakerProcessesFromPS(out)
	if len(procs) != 2 {
		t.Fatalf("processes=%#v, want guardian and hub worker only", procs)
	}
	if procs[0].PID != 123 || procs[0].Role != "guardian" {
		t.Fatalf("first process=%#v, want guardian pid 123", procs[0])
	}
	if procs[1].PID != 124 || procs[1].Role != "hub-worker" {
		t.Fatalf("second process=%#v, want hub-worker pid 124", procs[1])
	}
}

func TestResolveLogFilePath_FallbackOldRoot(t *testing.T) {
	base := t.TempDir()
	m := NewMonitor(base)

	oldPath := filepath.Join(base, "registry.log")
	if err := os.WriteFile(oldPath, []byte("old"), 0o644); err != nil {
		t.Fatalf("write old log: %v", err)
	}

	got := m.resolveLogFilePath("registry")
	if got != oldPath {
		t.Fatalf("resolveLogFilePath(registry)=%q, want %q", got, oldPath)
	}
}

func TestGetLogs_DebugOmitsTimeLevelAndDedupsSessionID(t *testing.T) {
	base := t.TempDir()
	m := NewMonitor(base)

	logDir := filepath.Join(base, "log")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatalf("mkdir log dir: %v", err)
	}

	sid := "019d6db0-3e60-7cf3-85c6-d2bf7e2a6f8a"
	line := "2026/04/09 06:44:32 DEBUG [acp] < {" + sid + " session/update} {\"sessionId\":\"" + sid + "\",\"update\":{\"sessionUpdate\":\"agent_message_chunk\"}}"
	if err := os.WriteFile(filepath.Join(logDir, "hub.debug.log"), []byte(line+"\n"), 0o644); err != nil {
		t.Fatalf("write debug log: %v", err)
	}

	res, err := m.GetLogs("debug", "", 100)
	if err != nil {
		t.Fatalf("GetLogs(debug): %v", err)
	}
	if len(res.Entries) != 1 {
		t.Fatalf("entries=%d, want 1", len(res.Entries))
	}
	entry := res.Entries[0]
	if entry.Time != "" {
		t.Fatalf("debug time should be hidden, got %q", entry.Time)
	}
	if entry.Level != "" {
		t.Fatalf("debug level should be hidden, got %q", entry.Level)
	}
	if strings.Contains(entry.Message, "\"sessionId\":\""+sid+"\"") {
		t.Fatalf("duplicate sessionId should be removed from debug json payload: %q", entry.Message)
	}
	if !strings.Contains(entry.Message, "{019d6db0..6f8a session/update}") {
		t.Fatalf("session id should be shortened in debug prefix: %q", entry.Message)
	}
}

func TestGetDBTablesDoesNotIncludeLegacyPromptTable(t *testing.T) {
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
		AgentJSON:    `{}`,
		CreatedAt:    time.Date(2026, 4, 12, 10, 0, 0, 0, time.UTC),
		LastActiveAt: time.Date(2026, 4, 12, 10, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}
	mon := NewMonitor(base)
	res := mon.GetDBTables()
	if res.Error != "" {
		t.Fatalf("GetDBTables error: %s", res.Error)
	}
	foundPrompts := false
	for _, table := range res.Tables {
		if table.Name == "session_prompts" {
			foundPrompts = true
		}
	}
	if foundPrompts {
		t.Fatalf("session_prompts table unexpectedly present: %#v", res.Tables)
	}
}

func TestGetDBTablesMatchesCurrentStoreSchema(t *testing.T) {
	base := t.TempDir()
	store, err := clientpkg.NewStore(filepath.Join(base, "db", "client.sqlite3"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	mon := NewMonitor(base)
	res := mon.GetDBTables()
	if res.Error != "" {
		t.Fatalf("GetDBTables error: %s", res.Error)
	}

	foundAgentPreferences := false
	foundProjects := false
	for _, table := range res.Tables {
		if table.Name == "agent_preferences" {
			foundAgentPreferences = true
		}
		if table.Name == "projects" {
			foundProjects = true
		}
	}
	if !foundAgentPreferences {
		t.Fatalf("agent_preferences table missing: %#v", res.Tables)
	}
	if !foundProjects {
		t.Fatalf("projects table missing: %#v", res.Tables)
	}
}

func TestParseMonitorSessionTurnDoesNotUseUpdateIndexAsSubIndex(t *testing.T) {
	method, role, kind, body, status, requestID, index, subIndex, source, ts := parseMonitorSessionTurn(
		`{"method":"agent_message_chunk","param":{"text":"hello"}}`,
		"2026-04-28T09:00:00Z",
		7,
	)
	if method != "agent_message_chunk" {
		t.Fatalf("method = %q, want %q", method, "agent_message_chunk")
	}
	if role != "assistant" {
		t.Fatalf("role = %q, want %q", role, "assistant")
	}
	if kind != "agent_message_chunk" {
		t.Fatalf("kind = %q, want %q", kind, "agent_message_chunk")
	}
	if body != "hello" {
		t.Fatalf("body = %q, want %q", body, "hello")
	}
	if status != "done" {
		t.Fatalf("status = %q, want %q", status, "done")
	}
	if requestID != 0 {
		t.Fatalf("requestID = %d, want 0", requestID)
	}
	if index != 7 {
		t.Fatalf("index = %d, want 7", index)
	}
	if subIndex != 0 {
		t.Fatalf("subIndex = %d, want 0", subIndex)
	}
	if source != "" {
		t.Fatalf("source = %q, want empty", source)
	}
	if ts != "2026-04-28T09:00:00Z" {
		t.Fatalf("ts = %q, want %q", ts, "2026-04-28T09:00:00Z")
	}
}

func TestExecuteActionClearSessionHistoryResetsSessionSync(t *testing.T) {
	base := t.TempDir()
	store, err := clientpkg.NewStore(filepath.Join(base, "db", "client.sqlite3"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	now := time.Date(2026, 4, 12, 10, 0, 0, 0, time.UTC)
	if err := store.SaveSession(ctx, &clientpkg.SessionRecord{
		ID:           "sess-1",
		ProjectName:  "proj1",
		Status:       clientpkg.SessionActive,
		AgentType:    "claude",
		AgentJSON:    `{}`,
		CreatedAt:    now,
		LastActiveAt: now,
	}); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}
	mon := NewMonitor(base)
	sessionRoot := filepath.Join(base, "db", "session")
	oldSessionRoot := filepath.Join(base, "session")
	if err := os.MkdirAll(filepath.Join(sessionRoot, "proj1", "sess-1", "turns"), 0o755); err != nil {
		t.Fatalf("mkdir session root: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sessionRoot, "proj1", "sess-1", "turns", "t000000.bin"), []byte("data"), 0o644); err != nil {
		t.Fatalf("write session turn file: %v", err)
	}

	if err := mon.ExecuteActionByHub(context.Background(), "", "clear-session-history"); err != nil {
		t.Fatalf("ExecuteActionByHub(clear-session-history): %v", err)
	}
	if _, err := os.Stat(filepath.Join(sessionRoot, "proj1", "sess-1", "turns", "t000000.bin")); err == nil {
		t.Fatalf("clear-session-history should remove db/session turn files")
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat db/session turn file: %v", err)
	}
	if info, err := os.Stat(sessionRoot); err != nil || !info.IsDir() {
		t.Fatalf("db/session root should be recreated, info=%v err=%v", info, err)
	}
	if _, err := os.Stat(oldSessionRoot); err == nil {
		t.Fatalf("clear-session-history should not create old session root")
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat old session root: %v", err)
	}

	rec, err := store.LoadSession(ctx, "proj1", "sess-1")
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	if rec == nil {
		t.Fatalf("LoadSession returned nil, want existing session record")
	}
	if rec.SessionSyncJSON != `{"latestPersistedTurnIndex":0}` {
		t.Fatalf("SessionSyncJSON = %q, want latestPersistedTurnIndex=0", rec.SessionSyncJSON)
	}
}

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

func TestDashboardHTML_RemovesStateSummaryCard(t *testing.T) {
	if strings.Contains(dashboardHTML, "State Summary") {
		t.Fatalf("dashboard should not contain State Summary card")
	}
	if strings.Contains(dashboardHTML, `id="state-summary"`) {
		t.Fatalf("dashboard should not contain state-summary container")
	}
}

func TestDashboardHTML_HasPWASetup(t *testing.T) {
	if !strings.Contains(dashboardHTML, `id="wm-manifest"`) {
		t.Fatalf("dashboard should include manifest link placeholder")
	}
	if !strings.Contains(dashboardHTML, "serviceWorker.register") {
		t.Fatalf("dashboard should register service worker")
	}
	if !strings.Contains(dashboardHTML, "appBasePath") {
		t.Fatalf("dashboard should compute app base path for /monitor scope")
	}
}

func TestRenderDashboardHTML_PWALinksForMonitor(t *testing.T) {
	html := renderDashboardHTML("/monitor")
	if !strings.Contains(html, `href="/monitor/manifest.webmanifest"`) {
		t.Fatalf("dashboard should render monitor manifest href")
	}
	if !strings.Contains(html, `href="/monitor/icons/icon.svg"`) {
		t.Fatalf("dashboard should render monitor icon href")
	}
	if strings.Contains(html, "__WM_MANIFEST__") || strings.Contains(html, "__WM_ICON__") {
		t.Fatalf("dashboard placeholders should be replaced")
	}
}

func TestRenderDashboardHTML_PWALinksForRoot(t *testing.T) {
	html := renderDashboardHTML("/")
	if !strings.Contains(html, `href="/manifest.webmanifest"`) {
		t.Fatalf("dashboard should render root manifest href")
	}
	if !strings.Contains(html, `href="/icons/icon.svg"`) {
		t.Fatalf("dashboard should render root icon href")
	}
}

func TestDashboardHTML_HasAgentJSONModalUI(t *testing.T) {
	if !strings.Contains(dashboardHTML, `id="json-modal"`) {
		t.Fatalf("dashboard should include json modal container")
	}
	if !strings.Contains(dashboardHTML, `id="json-modal-body"`) {
		t.Fatalf("dashboard should include json modal body")
	}
	if !strings.Contains(dashboardHTML, "View JSON") {
		t.Fatalf("dashboard should include View JSON action text")
	}
}

func TestDashboardHTML_HasAgentJSONModalScriptHooks(t *testing.T) {
	if strings.Contains(dashboardHTML, "openAgentsJSONModal") {
		t.Fatalf("dashboard should not define openAgentsJSONModal")
	}
	if !strings.Contains(dashboardHTML, "renderAgentJSONContent") {
		t.Fatalf("dashboard should define renderAgentJSONContent")
	}
	if !strings.Contains(dashboardHTML, "closeJSONModal") {
		t.Fatalf("dashboard should define closeJSONModal")
	}
	if !strings.Contains(dashboardHTML, "json-cell-btn") {
		t.Fatalf("dashboard should include json-cell-btn class hook")
	}
}
func TestDashboardHTML_HasGenericJSONCellViewHook(t *testing.T) {
	if !strings.Contains(dashboardHTML, "colName.endsWith('_json')") {
		t.Fatalf("dashboard should apply View JSON button to generic *_json columns")
	}
	if !strings.Contains(dashboardHTML, "openJSONModal(") {
		t.Fatalf("dashboard should define generic openJSONModal hook")
	}
}

func TestDashboardHTML_ShowsSessionSyncJSONAsViewJSON(t *testing.T) {
	if !strings.Contains(dashboardHTML, "displayDBColumnName(t.name, col)") {
		t.Fatalf("dashboard should render database column display labels")
	}
	if !strings.Contains(dashboardHTML, "table === 'sessions' && col === 'session_sync_json'") {
		t.Fatalf("dashboard should special-case sessions.session_sync_json")
	}
	if !strings.Contains(dashboardHTML, "isSessionSyncJSON ?") {
		t.Fatalf("dashboard should render session_sync_json cells as a View JSON action")
	}
}

func TestDashboardHTML_UsesAgentJSONAndUnifiedSessionIdentity(t *testing.T) {
	if strings.Contains(dashboardHTML, "ACP Session") {
		t.Fatalf("dashboard should not render ACP Session once session.id is unified")
	}
	if strings.Contains(dashboardHTML, "agents_json") {
		t.Fatalf("dashboard should not reference agents_json")
	}
	if !strings.Contains(dashboardHTML, "agent_json") {
		t.Fatalf("dashboard should expose agent_json column hooks")
	}
}

func TestDashboardHTML_HasUpdatePublishAction(t *testing.T) {
	if !strings.Contains(dashboardHTML, "doAction('update-publish')") {
		t.Fatalf("dashboard should provide update-publish action button")
	}
}

func TestDashboardHTML_HasMonitorLogoutAction(t *testing.T) {
	if !strings.Contains(dashboardHTML, "logoutMonitor()") {
		t.Fatalf("dashboard should provide monitor logout action")
	}
	if !strings.Contains(dashboardHTML, "Logout") {
		t.Fatalf("dashboard should render logout button text")
	}
}

func TestDashboardHTML_SendsMonitorCSRFHeaderForActions(t *testing.T) {
	if !strings.Contains(dashboardHTML, "X-WheelMaker-Monitor-CSRF") {
		t.Fatalf("dashboard should send monitor csrf header for post actions")
	}
	if !strings.Contains(dashboardHTML, "wm_monitor_csrf") {
		t.Fatalf("dashboard should read monitor csrf cookie")
	}
}

func TestDashboardHTML_HasHubSelectorUnderTopbar(t *testing.T) {
	if !strings.Contains(dashboardHTML, `id="hub-select"`) {
		t.Fatalf("dashboard should include hub selector")
	}
	if !strings.Contains(dashboardHTML, "onHubChanged()") {
		t.Fatalf("dashboard should wire hub selector change handler")
	}
}

func TestDashboardHTML_LoadsHubListAndHubScopedAPIs(t *testing.T) {
	if !strings.Contains(dashboardHTML, "api('hubs')") {
		t.Fatalf("dashboard should load hub list from api/hubs")
	}
	if !strings.Contains(dashboardHTML, "hubId=") {
		t.Fatalf("dashboard should attach selected hubId to hub-scoped API calls")
	}
}

func TestDashboardHTML_DefinesHubScopedHelpers(t *testing.T) {
	if !strings.Contains(dashboardHTML, "function hubPath(") {
		t.Fatalf("dashboard should define hubPath helper")
	}
	if !strings.Contains(dashboardHTML, "function apiHub(") {
		t.Fatalf("dashboard should define apiHub helper")
	}
}

func TestDashboardHTML_ShowsActionHintFromAPI(t *testing.T) {
	if !strings.Contains(dashboardHTML, "Hint:") {
		t.Fatalf("dashboard should surface action hint from backend response")
	}
	if !strings.Contains(dashboardHTML, "data.hint") {
		t.Fatalf("dashboard should read data.hint from action response")
	}
}

func TestDashboardHTML_ShowsProcessStartedAt(t *testing.T) {
	if !strings.Contains(dashboardHTML, "p.startedAt") {
		t.Fatalf("dashboard should read process startedAt field")
	}
	if !strings.Contains(dashboardHTML, "Started ") {
		t.Fatalf("dashboard should render Started label for process time")
	}
}

func TestDashboardHTML_UsesSingleProcessRolePIDTag(t *testing.T) {
	if !strings.Contains(dashboardHTML, "esc(roleLabel) + '#' + esc(String(p.pid))") {
		t.Fatalf("dashboard should combine role and pid in one tag")
	}
}

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

func TestActionUpdatePublishWritesFullUpdateSignal(t *testing.T) {
	baseDir := t.TempDir()
	writeMonitorTestConfig(t, baseDir, testMonitorToken)
	mon := NewMonitor(baseDir)

	mux := http.NewServeMux()
	registerRoutes(mux, mon)

	req := httptest.NewRequest(http.MethodPost, "/api/action/update-publish", nil)
	authorizeMonitorRequest(req)
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
	writeMonitorTestConfig(t, baseDir, testMonitorToken)
	store, err := clientpkg.NewStore(filepath.Join(baseDir, "db", "client.sqlite3"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	mon := NewMonitor(baseDir)

	mux := http.NewServeMux()
	registerRoutes(mux, mon)

	req := httptest.NewRequest(http.MethodPost, "/api/action/clear-session-history", nil)
	authorizeMonitorRequest(req)
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
	writeMonitorTestConfig(t, base, testMonitorToken)
	store, err := clientpkg.NewStore(filepath.Join(base, "db", "client.sqlite3"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	if _, err := clientpkg.WriteSessionTurnFiles(ctx, filepath.Join(base, "db", "session"), "proj1", "sess-1", 1, []string{
		`{"method":"session/prompt","params":{"prompt":[{"type":"text","text":"hello"}]}}`,
	}); err != nil {
		t.Fatalf("WriteSessionTurnFiles: %v", err)
	}
	if err := store.SaveSession(ctx, &clientpkg.SessionRecord{
		ID:              "sess-1",
		ProjectName:     "proj1",
		Status:          clientpkg.SessionActive,
		AgentType:       "claude",
		AgentJSON:       `{"title":"Task"}`,
		Title:           "Task",
		SessionSyncJSON: `{"latestPersistedTurnIndex":1}`,
		CreatedAt:       time.Date(2026, 4, 12, 10, 0, 0, 0, time.UTC),
		LastActiveAt:    time.Date(2026, 4, 12, 10, 1, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}

	mon := NewMonitor(base)
	mux := http.NewServeMux()
	registerRoutes(mux, mon)

	listReq := httptest.NewRequest(http.MethodGet, "/api/sessions", nil)
	authorizeMonitorRequest(listReq)
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
	authorizeMonitorRequest(msgReq)
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
	base := t.TempDir()
	writeMonitorTestConfig(t, base, testMonitorToken)
	mon := NewMonitor(base)
	stub := &stubHubTransport{}
	mon.transport = stub
	mon.defaultHubID = "hub-a"

	mux := http.NewServeMux()
	registerRoutes(mux, mon)

	req := httptest.NewRequest(http.MethodGet, "/api/status?hubId=hub-2", nil)
	authorizeMonitorRequest(req)
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
	base := t.TempDir()
	writeMonitorTestConfig(t, base, testMonitorToken)
	mon := NewMonitor(base)
	stub := &stubHubTransport{}
	mon.transport = stub
	mon.defaultHubID = "hub-a"

	mux := http.NewServeMux()
	registerRoutes(mux, mon)

	req := httptest.NewRequest(http.MethodPost, "/api/action/restart?hubId=hub-9", nil)
	authorizeMonitorRequest(req)
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
	writeMonitorTestConfig(t, baseDir, testMonitorToken)
	mon := NewMonitor(baseDir)
	stub := &stubHubTransport{actionErr: errors.New("registry down")}
	mon.transport = stub
	mon.defaultHubID = "local-hub"

	mux := http.NewServeMux()
	registerRoutes(mux, mon)

	req := httptest.NewRequest(http.MethodPost, "/api/action/update-publish?hubId=local-hub", nil)
	authorizeMonitorRequest(req)
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
	base := t.TempDir()
	writeMonitorTestConfig(t, base, testMonitorToken)
	mon := NewMonitor(base)
	stub := &stubHubTransport{}
	mon.transport = stub
	mon.defaultHubID = "local-hub"

	mux := http.NewServeMux()
	registerRoutes(mux, mon)

	req := httptest.NewRequest(http.MethodPost, "/api/action/start?hubId=remote-hub", nil)
	authorizeMonitorRequest(req)
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
