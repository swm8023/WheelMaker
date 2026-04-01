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
