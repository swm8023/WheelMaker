package portrelay

import (
	"net/http"
	"testing"

	rp "github.com/swm8023/wheelmaker/internal/protocol"
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

func TestHubClientOpenAllowsOnlyExactLoopbackTargetHost(t *testing.T) {
	client := NewHubClient()
	for _, targetHost := range []string{"localhost", "0.0.0.0", "::1", "127.0.0.2", "127.1.2.3"} {
		t.Run(targetHost, func(t *testing.T) {
			err := client.Open(rp.RelayOpenPayload{
				RelayID:    "relay_test",
				RelayURL:   "ws://127.0.0.1:9/__wheelmaker/relay/hub",
				Nonce:      "nonce",
				TargetHost: targetHost,
				TargetPort: 80,
			})
			if err == nil {
				t.Fatalf("Open targetHost=%q succeeded, want error", targetHost)
			}
		})
	}
}
