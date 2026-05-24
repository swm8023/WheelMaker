package portrelay

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	rp "github.com/swm8023/wheelmaker/internal/protocol"
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
	headers.Set("Accept-Encoding", "gzip, br")
	headers.Set("Accept", "text/html")

	filtered := filterRequestHeaders(headers)

	for _, name := range []string{"If-None-Match", "If-Modified-Since", "If-Match", "If-Unmodified-Since", "If-Range", "Accept-Encoding"} {
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

func TestHTMLViewportInjectorAddsMobileViewportMeta(t *testing.T) {
	injector := newHTMLViewportInjector(map[string][]string{
		"Content-Type": {"text/html; charset=utf-8"},
	})

	out := injector.Transform([]byte("<!doctype html><html><head><title>x</title></head><body>ok</body></html>"), true)
	got := string(out)

	if !strings.Contains(got, `<meta name="viewport" content="width=device-width, initial-scale=1, viewport-fit=cover">`) {
		t.Fatalf("Transform() did not inject viewport meta: %s", got)
	}
	if strings.Index(got, `<meta name="viewport"`) < strings.Index(got, "<title>") {
		return
	}
	t.Fatalf("Transform() injected viewport after existing head content: %s", got)
}

func TestHTMLViewportInjectorKeepsExistingViewportMeta(t *testing.T) {
	injector := newHTMLViewportInjector(map[string][]string{
		"Content-Type": {"text/html"},
	})
	input := `<html><head><meta name="viewport" content="initial-scale=1"><title>x</title></head></html>`

	out := string(injector.Transform([]byte(input), true))

	if out != input {
		t.Fatalf("Transform() changed page with existing viewport:\n%s", out)
	}
}

func TestHTMLViewportInjectorSkipsNonHTML(t *testing.T) {
	injector := newHTMLViewportInjector(map[string][]string{
		"Content-Type": {"application/javascript"},
	})
	input := `const html = "<head></head>";`

	out := string(injector.Transform([]byte(input), true))

	if out != input {
		t.Fatalf("Transform() changed non-html payload: %s", out)
	}
}

func TestHTMLViewportInjectorSkipsEncodedHTML(t *testing.T) {
	injector := newHTMLViewportInjector(map[string][]string{
		"Content-Type":     {"text/html"},
		"Content-Encoding": {"gzip"},
	})
	input := `<html><head></head><body>ok</body></html>`

	out := string(injector.Transform([]byte(input), true))

	if out != input {
		t.Fatalf("Transform() changed encoded html payload: %s", out)
	}
}

func TestHTMLViewportInjectorWaitsForHeadBeforeInjecting(t *testing.T) {
	injector := newHTMLViewportInjector(map[string][]string{
		"Content-Type": {"text/html"},
	})

	if out := injector.Transform([]byte("<html><head>"), false); out != nil {
		t.Fatalf("first Transform()=%q, want buffered", string(out))
	}
	out := string(injector.Transform([]byte("<title>x</title></head><body>ok</body></html>"), false))

	if !strings.Contains(out, relayViewportMeta) {
		t.Fatalf("second Transform() missing viewport meta: %s", out)
	}
	if strings.Index(out, relayViewportMeta) > strings.Index(out, "<title>") {
		t.Fatalf("viewport meta should be injected before title: %s", out)
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

func TestRelayEnableAllowsOnlyExactLoopbackTargetHost(t *testing.T) {
	c := NewController(ControllerConfig{
		RegistryAddr: "127.0.0.1:9630",
		ForwardHubRequest: func(context.Context, string, string, any) ControlResult {
			t.Fatal("ForwardHubRequest must not be called for invalid targetHost")
			return ControlResult{}
		},
	})
	for _, targetHost := range []string{"localhost", "0.0.0.0", "::1", "127.0.0.2", "127.1.2.3"} {
		t.Run(targetHost, func(t *testing.T) {
			_, errPayload := c.Enable(context.Background(), rp.RelayEnablePayload{
				ListenPort: reserveRelayTestPort(t),
				HubID:      "hub-local",
				TargetHost: targetHost,
				TargetPort: 80,
				AccessCode: "123456",
			}, "127.0.0.1:9630", false)
			if errPayload == nil || errPayload.Code != rp.CodeInvalidArgument {
				t.Fatalf("Enable targetHost=%q err=%#v, want invalid_argument", targetHost, errPayload)
			}
			if !strings.Contains(errPayload.Message, "127.0.0.1") {
				t.Fatalf("Enable targetHost=%q message=%q, want explicit loopback constraint", targetHost, errPayload.Message)
			}
		})
	}
}

func TestRelayLoginFlowUsesSafeRelativeNextAndHidesMappingInfo(t *testing.T) {
	c := NewController(ControllerConfig{})
	c.mu.Lock()
	c.slot = relaySlot{
		Enabled:              true,
		Status:               rp.RelayStatusOpening,
		HubID:                "private-hub",
		TargetHost:           "127.0.0.1",
		TargetPort:           5173,
		AccessCode:           "123456",
		AccessCodeGeneration: 1,
	}
	c.mu.Unlock()

	pageReq := httptest.NewRequest(http.MethodGet, internalLoginPath+"?error=1&next=%2Fconsole%3Ftab%3Drelay", nil)
	pageResp := httptest.NewRecorder()
	c.handleLogin(pageResp, pageReq)
	if pageResp.Code != http.StatusOK {
		t.Fatalf("login page status=%d, want 200", pageResp.Code)
	}
	body := pageResp.Body.String()
	for _, want := range []string{"WheelMaker Port Relay", "Invalid access code", `maxlength="6"`, `name="next" value="/console?tab=relay"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("login page missing %q:\n%s", want, body)
		}
	}
	for _, leaked := range []string{"private-hub", "5173", "127.0.0.1"} {
		if strings.Contains(body, leaked) {
			t.Fatalf("login page leaked mapping value %q:\n%s", leaked, body)
		}
	}

	badForm := url.Values{"code": {"000000"}, "next": {"/console?tab=relay"}}
	badReq := httptest.NewRequest(http.MethodPost, internalLoginPath, strings.NewReader(badForm.Encode()))
	badReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	badResp := httptest.NewRecorder()
	c.handleLogin(badResp, badReq)
	if badResp.Code != http.StatusSeeOther {
		t.Fatalf("bad login status=%d, want 303", badResp.Code)
	}
	if got := badResp.Header().Get("Location"); got != internalLoginPath+"?error=1&next=%2Fconsole%3Ftab%3Drelay" {
		t.Fatalf("bad login Location=%q", got)
	}

	goodForm := url.Values{"code": {"123456"}, "next": {"https://evil.example/steal"}}
	goodReq := httptest.NewRequest(http.MethodPost, internalLoginPath, strings.NewReader(goodForm.Encode()))
	goodReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	goodResp := httptest.NewRecorder()
	c.handleLogin(goodResp, goodReq)
	if goodResp.Code != http.StatusSeeOther {
		t.Fatalf("good login status=%d, want 303", goodResp.Code)
	}
	if got := goodResp.Header().Get("Location"); got != "/" {
		t.Fatalf("good login unsafe next Location=%q, want /", got)
	}
	if got := goodResp.Header().Get("Set-Cookie"); !strings.Contains(got, relayCookieName+"=") {
		t.Fatalf("good login missing relay auth cookie: %q", got)
	}
}

func TestUnauthenticatedRelayRequestRedirectsToLoginWithNext(t *testing.T) {
	c := NewController(ControllerConfig{})
	c.mu.Lock()
	c.slot = relaySlot{
		Enabled:              true,
		Status:               rp.RelayStatusOpening,
		AccessCode:           "123456",
		AccessCodeGeneration: 1,
	}
	c.mu.Unlock()

	req := httptest.NewRequest(http.MethodGet, "/console?tab=relay", nil)
	resp := httptest.NewRecorder()
	c.handleDataPlane(resp, req)

	if resp.Code != http.StatusSeeOther {
		t.Fatalf("unauthenticated status=%d, want 303", resp.Code)
	}
	if got := resp.Header().Get("Location"); got != internalLoginPath+"?next=%2Fconsole%3Ftab%3Drelay" {
		t.Fatalf("unauthenticated Location=%q", got)
	}
}

func reserveRelayTestPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve relay test port: %v", err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}
