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

func TestDashboardHTML_IncludesLogKindFilter(t *testing.T) {
	if !strings.Contains(dashboardHTML, `id="log-kind"`) {
		t.Fatalf("dashboard should include log-kind filter")
	}
	if !strings.Contains(dashboardHTML, `<option value="tool">Tool</option>`) {
		t.Fatalf("dashboard should include tool filter option")
	}
	if !strings.Contains(dashboardHTML, `<option value="thought">Thought</option>`) {
		t.Fatalf("dashboard should include thought filter option")
	}
}
