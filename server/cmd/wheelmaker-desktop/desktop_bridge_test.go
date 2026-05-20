package main

import (
	"strings"
	"testing"
)

func TestDesktopRuntimeInitScriptExposesWindowBridge(t *testing.T) {
	script := desktopRuntimeInitScript()

	for _, want := range []string{
		"window.WheelMakerDesktop",
		"enabled: true",
		desktopStartDragBinding,
		desktopMinimizeBinding,
		desktopToggleMaximizeBinding,
		desktopCloseBinding,
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("desktop runtime init script missing %q: %s", want, script)
		}
	}
}
