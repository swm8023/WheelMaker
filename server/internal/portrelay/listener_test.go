package portrelay

import (
	"net"
	"net/http"
	"testing"
)

func TestRelayListenerBindsLoopbackOnly(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve relay listener port: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()

	listener, err := newRelayListener(port, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	if err != nil {
		t.Fatalf("newRelayListener() err=%v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	addr, ok := listener.ln.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("listener addr=%T, want *net.TCPAddr", listener.ln.Addr())
	}
	if !addr.IP.Equal(net.ParseIP("127.0.0.1")) {
		t.Fatalf("listener IP=%s, want 127.0.0.1", addr.IP.String())
	}
}

func TestFilterRequestHeadersDropsConditionalCacheHeaders(t *testing.T) {
	headers := http.Header{}
	headers.Set("If-None-Match", `"empty-index"`)
	headers.Set("If-Modified-Since", "Sat, 23 May 2026 00:00:00 GMT")
	headers.Set("If-Match", `"current"`)
	headers.Set("If-Unmodified-Since", "Sat, 23 May 2026 00:00:00 GMT")
	headers.Set("If-Range", `"range"`)
	headers.Set("Accept", "text/html")

	filtered := filterRequestHeaders(headers)

	for _, name := range []string{"If-None-Match", "If-Modified-Since", "If-Match", "If-Unmodified-Since", "If-Range"} {
		if _, ok := filtered[name]; ok {
			t.Fatalf("filterRequestHeaders forwarded %s: %#v", name, filtered)
		}
	}
	if got := filtered["Accept"]; len(got) != 1 || got[0] != "text/html" {
		t.Fatalf("filterRequestHeaders dropped Accept: %#v", filtered)
	}
}

func TestCopyResponseHeadersDropsContentLength(t *testing.T) {
	src := map[string][]string{
		"Content-Length": {"1234"},
		"Content-Type":   {"application/javascript"},
	}
	dst := http.Header{}

	copyResponseHeaders(dst, src)

	if got := dst.Get("Content-Length"); got != "" {
		t.Fatalf("copyResponseHeaders forwarded Content-Length=%q", got)
	}
	if got := dst.Get("Content-Type"); got != "application/javascript" {
		t.Fatalf("copyResponseHeaders Content-Type=%q, want application/javascript", got)
	}
}

func TestCopyResponseHeadersAllowsRelayEmbedding(t *testing.T) {
	src := map[string][]string{
		"Content-Security-Policy":             {"default-src 'self'; frame-ancestors 'none'; connect-src ws: wss:"},
		"Content-Security-Policy-Report-Only": {"frame-ancestors https://example.com; script-src 'self'"},
		"X-Frame-Options":                     {"DENY"},
		"Content-Type":                        {"text/html"},
	}
	dst := http.Header{}

	copyResponseHeaders(dst, src)

	if got := dst.Get("X-Frame-Options"); got != "" {
		t.Fatalf("X-Frame-Options=%q, want empty", got)
	}
	if got := dst.Get("Content-Security-Policy"); got != "default-src 'self'; connect-src ws: wss:" {
		t.Fatalf("Content-Security-Policy=%q, want frame-ancestors removed", got)
	}
	if got := dst.Get("Content-Security-Policy-Report-Only"); got != "script-src 'self'" {
		t.Fatalf("Content-Security-Policy-Report-Only=%q, want frame-ancestors removed", got)
	}
	if got := dst.Get("Content-Type"); got != "text/html" {
		t.Fatalf("Content-Type=%q, want text/html", got)
	}
}

func TestCopyWebSocketResponseHeadersKeepsSubprotocolAndDropsExtensions(t *testing.T) {
	src := map[string][]string{
		"Sec-Websocket-Protocol":   {"vite-hmr"},
		"Sec-Websocket-Accept":     {"target-accept"},
		"Sec-Websocket-Extensions": {"permessage-deflate"},
		"Content-Length":           {"12"},
	}

	dst := copyWebSocketResponseHeaders(src)

	if got := dst.Get("Sec-Websocket-Protocol"); got != "vite-hmr" {
		t.Fatalf("Sec-Websocket-Protocol=%q, want vite-hmr", got)
	}
	if got := dst.Get("Sec-Websocket-Extensions"); got != "" {
		t.Fatalf("Sec-Websocket-Extensions=%q, want empty", got)
	}
	if got := dst.Get("Sec-Websocket-Accept"); got != "" {
		t.Fatalf("Sec-Websocket-Accept=%q, want empty", got)
	}
	if got := dst.Get("Content-Length"); got != "" {
		t.Fatalf("Content-Length=%q, want empty", got)
	}
}
