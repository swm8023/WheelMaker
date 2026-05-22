package registry

import (
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestRelayStatusIsAllowedForClientOnly(t *testing.T) {
	if !methodAllowed("client", "relay.status") {
		t.Fatal("client should be allowed to call relay.status")
	}
	if methodAllowed("hub", "relay.status") {
		t.Fatal("hub should not be allowed to call public relay.status")
	}
	if methodAllowed("monitor", "relay.enable") {
		t.Fatal("monitor should not be allowed to mutate relay slot")
	}
}

func TestRelayStatusReturnsDisabledSnapshot(t *testing.T) {
	s := New(Config{})
	ts := httptestNewRegistryServer(t, s.Handler())

	client := dialWS(t, "http://"+ts+"/ws")
	defer client.Close()
	connectRegistryClient(t, client)

	mustWriteJSON(t, client, testEnvelope{
		RequestID: 2,
		Type:      "request",
		Method:    "relay.status",
		Payload:   map[string]any{},
	})
	resp := mustReadEnvelope(t, client)
	if resp.Type != "response" || resp.Method != "relay.status" {
		t.Fatalf("relay.status response=%#v", resp)
	}
	if resp.Payload["status"] != "Disabled" || resp.Payload["enabled"] != false {
		t.Fatalf("relay.status payload=%#v, want disabled snapshot", resp.Payload)
	}
}

func TestRelayEnableForwardsInternalOpenToHub(t *testing.T) {
	s := New(Config{})
	ts := httptestNewRegistryServer(t, s.Handler())

	hub := dialReportedHub(t, "http://"+ts+"/ws", "hub-relay")
	defer hub.Close()

	client := dialWS(t, "http://"+ts+"/ws")
	defer client.Close()
	connectRegistryClient(t, client)

	listenPort := reserveTCPPort(t)
	mustWriteJSON(t, client, testEnvelope{
		RequestID: 2,
		Type:      "request",
		Method:    "relay.enable",
		Payload: map[string]any{
			"listenPort": listenPort,
			"hubId":      "hub-relay",
			"targetHost": "127.0.0.1",
			"targetPort": 43210,
			"accessCode": "483921",
		},
	})

	_ = hub.SetReadDeadline(time.Now().Add(2 * time.Second))
	forwarded := mustReadEnvelope(t, hub)
	if forwarded.Type != "request" || forwarded.Method != "relay.open" {
		t.Fatalf("forwarded=%#v, want relay.open request", forwarded)
	}
	if forwarded.Payload["targetHost"] != "127.0.0.1" || forwarded.Payload["targetPort"] != float64(43210) {
		t.Fatalf("forwarded payload=%#v", forwarded.Payload)
	}
	if forwarded.Payload["nonce"] == "" || forwarded.Payload["relayURL"] == "" {
		t.Fatalf("forwarded payload missing tunnel fields: %#v", forwarded.Payload)
	}
	mustWriteJSON(t, hub, testEnvelope{
		RequestID: forwarded.RequestID,
		Type:      "response",
		Method:    "relay.open",
		Payload: map[string]any{
			"ok": true,
		},
	})

	resp := mustReadEnvelope(t, client)
	if resp.Type != "response" || resp.Method != "relay.enable" {
		t.Fatalf("relay.enable response=%#v", resp)
	}
	if resp.Payload["enabled"] != true || resp.Payload["status"] != "Opening" {
		t.Fatalf("relay.enable payload=%#v, want opening snapshot", resp.Payload)
	}
}

func connectRegistryClient(t *testing.T, ws *websocket.Conn) {
	t.Helper()
	mustWriteJSON(t, ws, testEnvelope{
		RequestID: 1,
		Type:      "request",
		Method:    "connect.init",
		Payload: map[string]any{
			"clientName":      "wm-web",
			"clientVersion":   "0.1.0",
			"protocolVersion": "2.3",
			"role":            "client",
		},
	})
	_ = mustReadEnvelope(t, ws)
}

func httptestNewRegistryServer(t *testing.T, handler http.Handler) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen registry: %v", err)
	}
	srv := &http.Server{Handler: handler}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() {
		_ = srv.Close()
		_ = ln.Close()
	})
	return ln.Addr().String()
}

func reserveTCPPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve tcp port: %v", err)
	}
	defer ln.Close()
	addr, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("reserved addr=%v is not tcp", ln.Addr())
	}
	return addr.Port
}
