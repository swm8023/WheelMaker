package portrelay

import (
	"net/http"
	"testing"
)

func TestApplyTargetHeadersDropsExternalBrowserContextHeaders(t *testing.T) {
	dst := http.Header{}
	applyTargetHeaders(dst, map[string][]string{
		"Origin":  {"https://vimernas.myqnapcloud.com:28801"},
		"Referer": {"https://vimernas.myqnapcloud.com:28801/"},
		"Accept":  {"text/html"},
	})

	if got := dst.Get("Origin"); got != "" {
		t.Fatalf("applyTargetHeaders forwarded Origin=%q", got)
	}
	if got := dst.Get("Referer"); got != "" {
		t.Fatalf("applyTargetHeaders forwarded Referer=%q", got)
	}
	if got := dst.Get("Accept"); got != "text/html" {
		t.Fatalf("applyTargetHeaders Accept=%q, want text/html", got)
	}
}

func TestTargetOriginForWebSocketURLUsesHTTPOrigin(t *testing.T) {
	if got := targetOriginForURL("ws://127.0.0.1:5173/?token=abc"); got != "http://127.0.0.1:5173" {
		t.Fatalf("targetOriginForURL()=%q", got)
	}
}
