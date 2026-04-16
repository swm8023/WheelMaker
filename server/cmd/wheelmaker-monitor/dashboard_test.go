package main

import (
	"strings"
	"testing"
)

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

func TestDashboardHTML_HasAgentsJSONModalUI(t *testing.T) {
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

func TestDashboardHTML_HasAgentsJSONModalScriptHooks(t *testing.T) {
	if !strings.Contains(dashboardHTML, "openAgentsJSONModal") {
		t.Fatalf("dashboard should define openAgentsJSONModal")
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

func TestDashboardHTML_HasUpdatePublishAction(t *testing.T) {
	if !strings.Contains(dashboardHTML, "doAction('update-publish')") {
		t.Fatalf("dashboard should provide update-publish action button")
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
