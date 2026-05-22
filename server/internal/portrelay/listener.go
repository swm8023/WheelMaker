package portrelay

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	rp "github.com/swm8023/wheelmaker/internal/protocol"
)

var relayUpgrader = websocket.Upgrader{CheckOrigin: func(_ *http.Request) bool { return true }}

type relayListener struct {
	port int
	srv  *http.Server
	ln   net.Listener
}

func newRelayListener(port int, handler http.Handler) (*relayListener, error) {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("start relay listener on %s: %w", addr, err)
	}
	srv := &http.Server{Handler: handler}
	listener := &relayListener{port: port, srv: srv, ln: ln}
	go func() {
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			relayLogger().Warn("listener stopped port=%d err=%v", port, err)
		}
	}()
	return listener, nil
}

func (l *relayListener) Close() error {
	if l == nil || l.srv == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	return l.srv.Shutdown(ctx)
}

func (c *Controller) handleDataPlane(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case internalHubPath:
		c.handleHubTunnel(w, r)
		return
	case internalLoginPath:
		c.handleLogin(w, r)
		return
	case internalLogoutPath:
		c.handleLogout(w, r)
		return
	case internalStatusPath:
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(c.Status())
		return
	}

	slot := c.currentSlot()
	if !slot.Enabled {
		http.Error(w, "relay disabled", http.StatusServiceUnavailable)
		return
	}
	if !c.authenticated(r, slot) {
		writeLoginPage(w)
		return
	}
	tunnel, snapshot := c.activeTunnel()
	if tunnel == nil || snapshot.Status != rp.RelayStatusUp {
		http.Error(w, "relay tunnel is not up", http.StatusServiceUnavailable)
		return
	}
	if websocket.IsWebSocketUpgrade(r) {
		c.handleExternalWebSocket(w, r, tunnel)
		return
	}
	c.handleExternalHTTP(w, r, tunnel)
}

func (c *Controller) handleHubTunnel(w http.ResponseWriter, r *http.Request) {
	slot := c.currentSlot()
	relayID := r.URL.Query().Get("relayId")
	nonce := r.URL.Query().Get("nonce")
	if relayID == "" {
		relayID = slot.RelayID
	}
	if !slot.Enabled || slot.RelayID != relayID || slot.Nonce != nonce {
		http.Error(w, "invalid relay tunnel", http.StatusUnauthorized)
		return
	}
	conn, err := relayUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	if !c.acceptTunnel(relayID, nonce, conn) {
		_ = conn.Close()
	}
}

func (c *Controller) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeLoginPage(w)
		return
	}
	slot := c.currentSlot()
	if !slot.Enabled {
		http.Error(w, "relay disabled", http.StatusServiceUnavailable)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	if r.Form.Get("code") != slot.AccessCode {
		http.Error(w, "invalid access code", http.StatusUnauthorized)
		return
	}
	c.setAuthCookie(w, slot)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (c *Controller) handleLogout(w http.ResponseWriter, r *http.Request) {
	c.clearAuthCookie(w)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (c *Controller) handleExternalHTTP(w http.ResponseWriter, r *http.Request, tunnel *registryTunnel) {
	stream, err := tunnel.openStream(openMetaFromRequest("http", r))
	if err != nil {
		http.Error(w, "open relay stream failed", http.StatusBadGateway)
		return
	}
	defer stream.Close()
	if r.Body != nil {
		_, _ = io.Copy(io.Discard, r.Body)
		_ = r.Body.Close()
	}

	headersFrame, err := stream.waitHeaders(defaultStreamWait)
	if err != nil {
		http.Error(w, "target response timeout", http.StatusGatewayTimeout)
		return
	}
	var headers responseMeta
	if err := json.Unmarshal(headersFrame.Meta, &headers); err != nil {
		http.Error(w, "invalid target response", http.StatusBadGateway)
		return
	}
	copyResponseHeaders(w.Header(), headers.Headers)
	status := headers.Status
	if status == 0 {
		status = http.StatusOK
	}
	w.WriteHeader(status)
	for {
		frame, ok := stream.nextData()
		if !ok {
			return
		}
		if frame.Type == FrameData && len(frame.Payload) > 0 {
			if _, err := w.Write(frame.Payload); err != nil {
				return
			}
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
		}
		if frame.Type == FrameClose {
			return
		}
	}
}

func (c *Controller) handleExternalWebSocket(w http.ResponseWriter, r *http.Request, tunnel *registryTunnel) {
	stream, err := tunnel.openStream(openMetaFromRequest("websocket", r))
	if err != nil {
		http.Error(w, "open relay websocket failed", http.StatusBadGateway)
		return
	}
	headersFrame, err := stream.waitHeaders(defaultStreamWait)
	if err != nil {
		stream.Close()
		http.Error(w, "target websocket timeout", http.StatusGatewayTimeout)
		return
	}
	var headers responseMeta
	if err := json.Unmarshal(headersFrame.Meta, &headers); err != nil || headers.Status != http.StatusSwitchingProtocols {
		stream.Close()
		http.Error(w, "target websocket failed", http.StatusBadGateway)
		return
	}
	conn, err := relayUpgrader.Upgrade(w, r, nil)
	if err != nil {
		stream.Close()
		return
	}
	defer conn.Close()
	defer stream.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			messageType, payload, err := conn.ReadMessage()
			if err != nil {
				_ = stream.sendClose()
				return
			}
			flags := FlagWebSocketBinary
			if messageType == websocket.TextMessage {
				flags = FlagWebSocketText
			}
			if err := stream.sendData(flags, payload); err != nil {
				return
			}
		}
	}()

	for {
		select {
		case <-done:
			return
		default:
		}
		frame, ok := stream.nextData()
		if !ok {
			return
		}
		if frame.Type == FrameClose {
			return
		}
		if frame.Type != FrameData {
			continue
		}
		messageType := websocket.BinaryMessage
		if frame.Flags&FlagWebSocketText != 0 {
			messageType = websocket.TextMessage
		}
		if err := conn.WriteMessage(messageType, frame.Payload); err != nil {
			return
		}
	}
}

func (c *Controller) currentSlot() relaySlot {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.slot
}

func writeLoginPage(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte(`<!doctype html><html><body><form method="post" action="/__wheelmaker/relay/login"><input name="code" inputmode="numeric" autocomplete="one-time-code" /><button type="submit">Open</button></form></body></html>`))
}

type requestMeta struct {
	Kind     string              `json:"kind"`
	Method   string              `json:"method"`
	Path     string              `json:"path"`
	RawQuery string              `json:"rawQuery"`
	Headers  map[string][]string `json:"headers"`
}

type responseMeta struct {
	Kind    string              `json:"kind"`
	Status  int                 `json:"status"`
	Headers map[string][]string `json:"headers"`
}

func openMetaFromRequest(kind string, r *http.Request) requestMeta {
	return requestMeta{
		Kind:     kind,
		Method:   r.Method,
		Path:     r.URL.Path,
		RawQuery: r.URL.RawQuery,
		Headers:  filterRequestHeaders(r.Header),
	}
}

func filterRequestHeaders(headers http.Header) map[string][]string {
	out := map[string][]string{}
	for name, values := range headers {
		canonical := http.CanonicalHeaderKey(name)
		if isHopByHopHeader(canonical) || isWebSocketDialerHeader(canonical) || strings.EqualFold(canonical, "Cookie") {
			continue
		}
		out[canonical] = append([]string(nil), values...)
	}
	return out
}

func copyResponseHeaders(dst http.Header, headers map[string][]string) {
	for name, values := range headers {
		canonical := http.CanonicalHeaderKey(name)
		if isHopByHopHeader(canonical) {
			continue
		}
		for _, value := range values {
			if strings.EqualFold(canonical, "Set-Cookie") && strings.HasPrefix(strings.ToLower(value), strings.ToLower(relayCookieName)+"=") {
				continue
			}
			dst.Add(canonical, value)
		}
	}
}

func isHopByHopHeader(name string) bool {
	switch http.CanonicalHeaderKey(name) {
	case "Connection", "Upgrade", "Keep-Alive", "Proxy-Authenticate", "Proxy-Authorization", "Te", "Trailer", "Transfer-Encoding":
		return true
	default:
		return false
	}
}

func isWebSocketDialerHeader(name string) bool {
	switch http.CanonicalHeaderKey(name) {
	case "Sec-Websocket-Key", "Sec-Websocket-Version", "Sec-Websocket-Extensions", "Sec-Websocket-Accept":
		return true
	default:
		return false
	}
}
