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
