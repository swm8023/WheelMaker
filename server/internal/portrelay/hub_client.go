package portrelay

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
	rp "github.com/swm8023/wheelmaker/internal/protocol"
)

type HubClient struct {
	mu      sync.Mutex
	relayID string
	cancel  context.CancelFunc
}

func NewHubClient() *HubClient {
	return &HubClient{}
}

func (c *HubClient) Open(payload rp.RelayOpenPayload) error {
	payload.TargetHost = strings.TrimSpace(payload.TargetHost)
	if strings.TrimSpace(payload.RelayID) == "" || strings.TrimSpace(payload.RelayURL) == "" || strings.TrimSpace(payload.Nonce) == "" {
		return fmt.Errorf("invalid relay.open payload")
	}
	if payload.TargetHost != relayTargetHost {
		return fmt.Errorf("targetHost must be 127.0.0.1")
	}
	ctx, cancel := context.WithCancel(context.Background())
	c.mu.Lock()
	if c.cancel != nil {
		c.cancel()
	}
	c.relayID = payload.RelayID
	c.cancel = cancel
	c.mu.Unlock()
	go c.run(ctx, payload)
	return nil
}

func (c *HubClient) Close(relayID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cancel == nil {
		return
	}
	if strings.TrimSpace(relayID) != "" && relayID != c.relayID {
		return
	}
	c.cancel()
	c.cancel = nil
	c.relayID = ""
}

func (c *HubClient) run(ctx context.Context, payload rp.RelayOpenPayload) {
	tunnelURL, err := addTunnelQuery(payload.RelayURL, payload.RelayID, payload.Nonce)
	if err != nil {
		relayLogger().Warn("invalid relay tunnel url: %v", err)
		return
	}
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, tunnelURL, nil)
	if err != nil {
		relayLogger().Warn("dial relay tunnel failed: %v", err)
		return
	}
	defer conn.Close()
	tunnel := newHubTunnel(conn, payload)
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-done:
		}
	}()
	defer close(done)
	tunnel.readLoop(ctx)
}

func addTunnelQuery(rawURL string, relayID string, nonce string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("relayId", relayID)
	q.Set("nonce", nonce)
	u.RawQuery = q.Encode()
	return u.String(), nil
}

type hubTunnel struct {
	conn    *websocket.Conn
	payload rp.RelayOpenPayload
	writeMu sync.Mutex

	mu      sync.Mutex
	streams map[uint32]*hubStream
}

type hubStream struct {
	id       uint32
	kind     string
	ws       *websocket.Conn
	closeMux sync.Once
}

func newHubTunnel(conn *websocket.Conn, payload rp.RelayOpenPayload) *hubTunnel {
	return &hubTunnel{
		conn:    conn,
		payload: payload,
		streams: make(map[uint32]*hubStream),
	}
}

func (t *hubTunnel) readLoop(ctx context.Context) {
	for {
		frame, err := readFrame(t.conn)
		if err != nil {
			return
		}
		switch frame.Type {
		case FrameOpen:
			t.handleOpen(ctx, frame)
		case FrameData:
			t.handleData(frame)
		case FrameClose, FrameError:
			t.closeStream(frame.StreamID)
		}
	}
}

func (t *hubTunnel) handleOpen(ctx context.Context, frame Frame) {
	var meta requestMeta
	if err := json.Unmarshal(frame.Meta, &meta); err != nil {
		_ = t.writeError(frame.StreamID, "invalid stream metadata")
		return
	}
	switch meta.Kind {
	case "http":
		go t.handleHTTP(ctx, frame.StreamID, meta)
	case "websocket":
		go t.handleWebSocket(ctx, frame.StreamID, meta)
	default:
		_ = t.writeError(frame.StreamID, "unsupported stream kind")
	}
}

func (t *hubTunnel) handleHTTP(ctx context.Context, streamID uint32, meta requestMeta) {
	targetURL := buildTargetURL("http", t.payload.TargetHost, t.payload.TargetPort, meta.Path, meta.RawQuery)
	req, err := http.NewRequestWithContext(ctx, meta.Method, targetURL, nil)
	if err != nil {
		_ = t.writeError(streamID, "invalid target request")
		return
	}
	applyTargetHeaders(req.Header, meta.Headers)
	req.Header.Set("User-Agent", firstNonEmpty(t.payload.UserAgent, BrowserUserAgent))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		_ = t.writeError(streamID, "target request failed")
		return
	}
	defer resp.Body.Close()
	if err := t.writeHeaders(streamID, responseMeta{Kind: "http", Status: resp.StatusCode, Headers: filterResponseHeaders(resp.Header)}); err != nil {
		return
	}
	buf := make([]byte, 32*1024)
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if err := t.write(Frame{Type: FrameData, StreamID: streamID, Payload: append([]byte(nil), buf[:n]...)}); err != nil {
				return
			}
		}
		if readErr != nil {
			if readErr != io.EOF {
				_ = t.writeError(streamID, "target response read failed")
				return
			}
			_ = t.write(Frame{Type: FrameClose, StreamID: streamID})
			return
		}
	}
}

func (t *hubTunnel) handleWebSocket(ctx context.Context, streamID uint32, meta requestMeta) {
	targetURL := buildTargetURL("ws", t.payload.TargetHost, t.payload.TargetPort, meta.Path, meta.RawQuery)
	headers := http.Header{}
	applyTargetHeaders(headers, meta.Headers)
	if origin := targetOriginForURL(targetURL); origin != "" {
		headers.Set("Origin", origin)
	}
	headers.Set("User-Agent", firstNonEmpty(t.payload.UserAgent, BrowserUserAgent))
	ws, resp, err := websocket.DefaultDialer.DialContext(ctx, targetURL, headers)
	if err != nil {
		_ = t.writeError(streamID, "target websocket failed")
		return
	}
	responseHeaders := http.Header{}
	if resp != nil {
		responseHeaders = resp.Header
	}
	if err := t.writeHeaders(streamID, responseMeta{Kind: "websocket", Status: http.StatusSwitchingProtocols, Headers: filterResponseHeaders(responseHeaders)}); err != nil {
		_ = ws.Close()
		return
	}
	stream := &hubStream{id: streamID, kind: "websocket", ws: ws}
	t.mu.Lock()
	t.streams[streamID] = stream
	t.mu.Unlock()
	go func() {
		defer t.closeStream(streamID)
		for {
			messageType, payload, err := ws.ReadMessage()
			if err != nil {
				return
			}
			flags := FlagWebSocketBinary
			if messageType == websocket.TextMessage {
				flags = FlagWebSocketText
			}
			if err := t.write(Frame{Type: FrameData, Flags: flags, StreamID: streamID, Payload: payload}); err != nil {
				return
			}
		}
	}()
}

func (t *hubTunnel) handleData(frame Frame) {
	t.mu.Lock()
	stream := t.streams[frame.StreamID]
	t.mu.Unlock()
	if stream == nil || stream.ws == nil {
		return
	}
	messageType := websocket.BinaryMessage
	if frame.Flags&FlagWebSocketText != 0 {
		messageType = websocket.TextMessage
	}
	_ = stream.ws.WriteMessage(messageType, frame.Payload)
}

func (t *hubTunnel) closeStream(streamID uint32) {
	t.mu.Lock()
	stream := t.streams[streamID]
	delete(t.streams, streamID)
	t.mu.Unlock()
	if stream != nil && stream.ws != nil {
		stream.closeMux.Do(func() {
			_ = stream.ws.Close()
		})
	}
}

func (t *hubTunnel) writeHeaders(streamID uint32, meta responseMeta) error {
	raw, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	return t.write(Frame{Type: FrameHeaders, StreamID: streamID, Meta: raw})
}

func (t *hubTunnel) writeError(streamID uint32, message string) error {
	raw, _ := json.Marshal(map[string]string{"message": message})
	return t.write(Frame{Type: FrameError, StreamID: streamID, Meta: raw})
}

func (t *hubTunnel) write(frame Frame) error {
	t.writeMu.Lock()
	defer t.writeMu.Unlock()
	return writeFrame(t.conn, frame)
}

func buildTargetURL(scheme string, host string, port int, path string, rawQuery string) string {
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	u := url.URL{
		Scheme:   scheme,
		Host:     fmt.Sprintf("%s:%d", host, port),
		Path:     path,
		RawQuery: rawQuery,
	}
	return u.String()
}

func applyTargetHeaders(dst http.Header, src map[string][]string) {
	for name, values := range src {
		canonical := http.CanonicalHeaderKey(name)
		if isHopByHopHeader(canonical) || isBrowserContextHeader(canonical) {
			continue
		}
		for _, value := range values {
			dst.Add(canonical, value)
		}
	}
}

func isBrowserContextHeader(name string) bool {
	switch http.CanonicalHeaderKey(name) {
	case "Origin", "Referer":
		return true
	default:
		return false
	}
}

func targetOriginForURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return ""
	}
	scheme := u.Scheme
	switch scheme {
	case "ws":
		scheme = "http"
	case "wss":
		scheme = "https"
	}
	if scheme == "" {
		return ""
	}
	return scheme + "://" + u.Host
}

func filterResponseHeaders(src http.Header) map[string][]string {
	out := map[string][]string{}
	for name, values := range src {
		canonical := http.CanonicalHeaderKey(name)
		if isHopByHopHeader(canonical) {
			continue
		}
		out[canonical] = append([]string(nil), values...)
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func relayLogger() relayLog {
	return relayLog{}
}

type relayLog struct{}

func (relayLog) Warn(format string, args ...any) {
	// Logging is intentionally quiet in tests; registry and hub callers emit outer lifecycle logs.
}
