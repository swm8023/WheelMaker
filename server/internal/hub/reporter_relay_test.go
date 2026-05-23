package hub

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/swm8023/wheelmaker/internal/registry"
)

func TestReporterPortRelayHTTPAndWebSocketSmoke(t *testing.T) {
	var seenUserAgent string
	targetUpgrader := websocket.Upgrader{
		CheckOrigin:  func(_ *http.Request) bool { return true },
		Subprotocols: []string{"vite-hmr"},
	}
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenUserAgent = r.UserAgent()
		if r.URL.Path == "/ws" {
			conn, err := targetUpgrader.Upgrade(w, r, nil)
			if err != nil {
				t.Fatalf("target ws upgrade: %v", err)
			}
			defer conn.Close()
			for {
				messageType, payload, err := conn.ReadMessage()
				if err != nil {
					return
				}
				if err := conn.WriteMessage(messageType, append([]byte("echo:"), payload...)); err != nil {
					return
				}
			}
		}
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("hello through relay"))
	}))
	t.Cleanup(target.Close)
	targetURL, err := url.Parse(target.URL)
	if err != nil {
		t.Fatalf("parse target url: %v", err)
	}
	targetHost, targetPort := splitHostPortForTest(t, targetURL.Host)

	reg := registry.New(registry.Config{})
	registryAddr := newRegistryServer(t, reg.Handler())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reporter := NewReporter(ReporterConfig{
		Server:            registryAddr,
		HubID:             "hub-relay-smoke",
		ReconnectInterval: 50 * time.Millisecond,
	}, nil)
	done := make(chan error, 1)
	go func() { done <- reporter.Run(ctx) }()
	defer func() {
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("reporter did not stop")
		}
	}()

	waitForRelayHubOnline(t, registryAddr, "hub-relay-smoke")
	relayPort := reservePortForHubTest(t)
	client := dialWS(t, "http://"+registryAddr+"/ws")
	defer client.Close()
	connectClient(t, client, "")
	mustWriteJSON(t, client, testEnvelope{
		RequestID: 2,
		Type:      "request",
		Method:    "relay.enable",
		Payload: map[string]any{
			"listenPort": relayPort,
			"hubId":      "hub-relay-smoke",
			"targetHost": targetHost,
			"targetPort": targetPort,
			"accessCode": "483921",
		},
	})
	enableResp := mustReadEnvelope(t, client)
	if enableResp.Type != "response" || enableResp.Payload["status"] != "Up" {
		t.Fatalf("relay.enable response=%#v, want Up", enableResp)
	}

	relayBase := "http://127.0.0.1:" + strconv.Itoa(relayPort)
	jarClient := &http.Client{CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	unauthResp, err := jarClient.Get(relayBase + "/")
	if err != nil {
		t.Fatalf("unauth relay get: %v", err)
	}
	if unauthResp.StatusCode != http.StatusSeeOther {
		t.Fatalf("unauth status=%d, want 303", unauthResp.StatusCode)
	}
	if got := unauthResp.Header.Get("Location"); got != "/__wheelmaker/relay/login" {
		t.Fatalf("unauth Location=%q, want login page", got)
	}
	_ = unauthResp.Body.Close()

	loginResp, err := jarClient.Post(relayBase+"/__wheelmaker/relay/login", "application/x-www-form-urlencoded", strings.NewReader("code=483921"))
	if err != nil {
		t.Fatalf("login relay: %v", err)
	}
	if loginResp.StatusCode != http.StatusSeeOther {
		t.Fatalf("login status=%d, want 303", loginResp.StatusCode)
	}
	cookies := loginResp.Cookies()
	_ = loginResp.Body.Close()

	req, err := http.NewRequest(http.MethodGet, relayBase+"/", nil)
	if err != nil {
		t.Fatalf("new relay request: %v", err)
	}
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}
	authedResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("authed relay get: %v", err)
	}
	body, _ := io.ReadAll(authedResp.Body)
	_ = authedResp.Body.Close()
	if authedResp.StatusCode != http.StatusOK || string(body) != "hello through relay" {
		t.Fatalf("authed status=%d body=%q", authedResp.StatusCode, string(body))
	}
	if !strings.Contains(seenUserAgent, "Mozilla/5.0") {
		t.Fatalf("target user-agent=%q, want browser-like agent", seenUserAgent)
	}

	wsHeader := http.Header{}
	for _, cookie := range cookies {
		wsHeader.Add("Cookie", cookie.String())
	}
	wsConn, _, err := websocket.DefaultDialer.Dial("ws://127.0.0.1:"+strconv.Itoa(relayPort)+"/ws", wsHeader)
	if err != nil {
		t.Fatalf("dial relay ws: %v", err)
	}
	defer wsConn.Close()
	if err := wsConn.WriteMessage(websocket.TextMessage, []byte("text")); err != nil {
		t.Fatalf("write relay ws text: %v", err)
	}
	messageType, payload, err := wsConn.ReadMessage()
	if err != nil {
		t.Fatalf("read relay ws text: %v", err)
	}
	if messageType != websocket.TextMessage || string(payload) != "echo:text" {
		t.Fatalf("ws text type=%d payload=%q", messageType, string(payload))
	}
	if err := wsConn.WriteMessage(websocket.BinaryMessage, []byte{1, 2, 3}); err != nil {
		t.Fatalf("write relay ws binary: %v", err)
	}
	messageType, payload, err = wsConn.ReadMessage()
	if err != nil {
		t.Fatalf("read relay ws binary: %v", err)
	}
	if messageType != websocket.BinaryMessage || string(payload) != "echo:\x01\x02\x03" {
		t.Fatalf("ws binary type=%d payload=%v", messageType, payload)
	}

	subprotocolDialer := websocket.Dialer{Subprotocols: []string{"vite-hmr"}}
	subprotocolConn, _, err := subprotocolDialer.Dial("ws://127.0.0.1:"+strconv.Itoa(relayPort)+"/ws", wsHeader)
	if err != nil {
		t.Fatalf("dial relay ws subprotocol: %v", err)
	}
	defer subprotocolConn.Close()
	if got := subprotocolConn.Subprotocol(); got != "vite-hmr" {
		t.Fatalf("relay ws subprotocol=%q, want vite-hmr", got)
	}
}

func waitForRelayHubOnline(t *testing.T, addr string, hubID string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		ws := dialWS(t, "http://"+addr+"/ws")
		connectClient(t, ws, "")
		mustWriteJSON(t, ws, testEnvelope{RequestID: 2, Type: "request", Method: "project.list", Payload: map[string]any{}})
		resp := mustReadEnvelope(t, ws)
		_ = ws.Close()
		hubs, _ := resp.Payload["hubs"].([]any)
		for _, raw := range hubs {
			hub, _ := raw.(map[string]any)
			if hub["hubId"] == hubID {
				return
			}
		}
		time.Sleep(40 * time.Millisecond)
	}
	t.Fatalf("hub %q not online before timeout", hubID)
}

func splitHostPortForTest(t *testing.T, hostport string) (string, int) {
	t.Helper()
	host, portText, err := net.SplitHostPort(hostport)
	if err != nil {
		t.Fatalf("split host port: %v", err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatalf("parse port: %v", err)
	}
	return host, port
}

func reservePortForHubTest(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve relay port: %v", err)
	}
	defer ln.Close()
	addr, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("reserved addr=%v is not tcp", ln.Addr())
	}
	return addr.Port
}
