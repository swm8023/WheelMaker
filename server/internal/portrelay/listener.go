package portrelay

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
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
		http.Redirect(w, r, relayLoginLocation(requestNextPath(r), false), http.StatusSeeOther)
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
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	slot := c.currentSlot()
	if !slot.Enabled {
		http.Error(w, "relay disabled", http.StatusServiceUnavailable)
		return
	}
	if r.Method == http.MethodGet {
		writeLoginPage(w, loginPageOptions{
			InvalidCode: r.URL.Query().Get("error") == "1",
			Next:        safeRelayNext(r.URL.Query().Get("next")),
		})
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	next := safeRelayNext(r.Form.Get("next"))
	if r.Form.Get("code") != slot.AccessCode {
		http.Redirect(w, r, relayLoginLocation(next, true), http.StatusSeeOther)
		return
	}
	c.setAuthCookie(w, slot)
	http.Redirect(w, r, next, http.StatusSeeOther)
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
	conn, err := relayUpgrader.Upgrade(w, r, copyWebSocketResponseHeaders(headers.Headers))
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

type loginPageOptions struct {
	InvalidCode bool
	Next        string
}

func writeLoginPage(w http.ResponseWriter, options loginPageOptions) {
	next := safeRelayNext(options.Next)
	errorClass := ""
	if options.InvalidCode {
		errorClass = " show"
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(fmt.Sprintf(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>WheelMaker Port Relay</title>
  <style>
    :root { color-scheme: light dark; }
    * { box-sizing: border-box; }
    html, body { width: 100%%; min-height: 100%%; margin: 0; }
    body {
      display: grid;
      place-items: center;
      padding: 24px;
      background: #101316;
      color: #f2f5f8;
      font: 14px/1.45 -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
    }
    main {
      width: min(100%%, 360px);
      border: 1px solid rgba(255, 255, 255, 0.14);
      border-radius: 8px;
      background: #171b20;
      padding: 22px;
      box-shadow: 0 18px 54px rgba(0, 0, 0, 0.34);
    }
    h1 { margin: 0 0 14px; font-size: 18px; line-height: 1.25; font-weight: 600; }
    form { display: grid; gap: 12px; }
    label { display: grid; gap: 6px; color: #aeb7c2; font-size: 12px; }
    input {
      width: 100%%;
      height: 42px;
      border: 1px solid rgba(255, 255, 255, 0.16);
      border-radius: 6px;
      background: #0f1216;
      color: #ffffff;
      padding: 0 12px;
      font: 600 18px/1.2 "SFMono-Regular", Consolas, monospace;
      letter-spacing: 0.18em;
      outline: none;
    }
    input:focus { border-color: #6aa6ff; box-shadow: 0 0 0 3px rgba(106, 166, 255, 0.18); }
    button {
      height: 40px;
      border: 0;
      border-radius: 6px;
      background: #2f81f7;
      color: #ffffff;
      font-weight: 600;
      cursor: pointer;
    }
    .error {
      display: none;
      margin: 0;
      color: #ff9b96;
      font-size: 12px;
    }
    .error.show { display: block; }
  </style>
</head>
<body>
  <main>
    <h1>WheelMaker Port Relay</h1>
    <form method="post" action="%s" autocomplete="off">
      <input type="hidden" name="next" value="%s">
      <label>
        <span>Access code</span>
        <input name="code" inputmode="numeric" autocomplete="one-time-code" maxlength="6" pattern="[0-9]{6}" autofocus>
      </label>
      <p class="error%s">Invalid access code</p>
      <button type="submit">Open</button>
    </form>
  </main>
</body>
</html>`, internalLoginPath, html.EscapeString(next), errorClass)))
}

func relayLoginLocation(next string, invalidCode bool) string {
	values := url.Values{}
	if invalidCode {
		values.Set("error", "1")
	}
	if safe := safeRelayNext(next); safe != "/" {
		values.Set("next", safe)
	}
	if encoded := values.Encode(); encoded != "" {
		return internalLoginPath + "?" + encoded
	}
	return internalLoginPath
}

func requestNextPath(r *http.Request) string {
	if r == nil || r.URL == nil {
		return "/"
	}
	next := r.URL.EscapedPath()
	if next == "" {
		next = "/"
	}
	if r.URL.RawQuery != "" {
		next += "?" + r.URL.RawQuery
	}
	return next
}

func safeRelayNext(value string) string {
	if value == "" {
		return "/"
	}
	u, err := url.Parse(value)
	if err != nil || u.IsAbs() || u.Host != "" || !strings.HasPrefix(u.Path, "/") {
		return "/"
	}
	if u.Path == internalLoginPath || strings.HasPrefix(u.Path, "/__wheelmaker/relay/") {
		return "/"
	}
	out := u.EscapedPath()
	if out == "" {
		out = "/"
	}
	if u.RawQuery != "" {
		out += "?" + u.RawQuery
	}
	return out
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
		if isHopByHopHeader(canonical) || isWebSocketDialerHeader(canonical) || isConditionalRequestHeader(canonical) || strings.EqualFold(canonical, "Cookie") {
			continue
		}
		out[canonical] = append([]string(nil), values...)
	}
	return out
}

func copyResponseHeaders(dst http.Header, headers map[string][]string) {
	for name, values := range headers {
		canonical := http.CanonicalHeaderKey(name)
		if isHopByHopHeader(canonical) || isBlockedFrameHeader(canonical) || strings.EqualFold(canonical, "Content-Length") {
			continue
		}
		values = sanitizeResponseHeaderValues(canonical, values)
		if len(values) == 0 {
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

func copyWebSocketResponseHeaders(headers map[string][]string) http.Header {
	dst := http.Header{}
	for name, values := range headers {
		canonical := http.CanonicalHeaderKey(name)
		if isHopByHopHeader(canonical) || isWebSocketDialerHeader(canonical) || strings.EqualFold(canonical, "Content-Length") {
			continue
		}
		for _, value := range values {
			if strings.EqualFold(canonical, "Set-Cookie") && strings.HasPrefix(strings.ToLower(value), strings.ToLower(relayCookieName)+"=") {
				continue
			}
			dst.Add(canonical, value)
		}
	}
	return dst
}

func sanitizeResponseHeaderValues(name string, values []string) []string {
	if !isContentSecurityPolicyHeader(name) {
		return values
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		next := removeContentSecurityPolicyDirective(value, "frame-ancestors")
		if next != "" {
			out = append(out, next)
		}
	}
	return out
}

func removeContentSecurityPolicyDirective(value string, directiveName string) string {
	parts := strings.Split(value, ";")
	kept := make([]string, 0, len(parts))
	for _, part := range parts {
		directive := strings.TrimSpace(part)
		if directive == "" {
			continue
		}
		name := directive
		if idx := strings.IndexAny(directive, " \t\r\n"); idx >= 0 {
			name = directive[:idx]
		}
		if strings.EqualFold(name, directiveName) {
			continue
		}
		kept = append(kept, directive)
	}
	return strings.Join(kept, "; ")
}

func isBlockedFrameHeader(name string) bool {
	return strings.EqualFold(name, "X-Frame-Options")
}

func isContentSecurityPolicyHeader(name string) bool {
	return strings.EqualFold(name, "Content-Security-Policy") || strings.EqualFold(name, "Content-Security-Policy-Report-Only")
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

func isConditionalRequestHeader(name string) bool {
	switch http.CanonicalHeaderKey(name) {
	case "If-Match", "If-Modified-Since", "If-None-Match", "If-Range", "If-Unmodified-Since":
		return true
	default:
		return false
	}
}
