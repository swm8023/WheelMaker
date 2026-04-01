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
