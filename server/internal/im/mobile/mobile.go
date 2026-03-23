package mobile

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/gorilla/websocket"
	"github.com/swm8023/wheelmaker/internal/im"
	"github.com/swm8023/wheelmaker/internal/logger"
)

// Config configures the mobile WebSocket IM adapter.
type Config struct {
	// Addr is the TCP listen address, e.g. ":9527". Default: ":9527".
	Addr string
	// Token is a shared secret required from connecting clients.
	// Empty string disables auth (useful for local dev).
	Token string
}

// mobileConn holds a single connected WebSocket client.
type mobileConn struct {
	chatID string
	ws     *websocket.Conn
	mu     sync.Mutex
	authed bool
}

// IM implements im.Channel, im.OptionSender, im.DebugSender, and im.CardActionSubscriber
// using an HTTP/WebSocket server. Each connected mobile client is assigned a unique chatID.
type IM struct {
	cfg               Config
	handler           im.MessageHandler
	cardActionHandler func(im.CardActionEvent)

	mu     sync.RWMutex
	conns  map[string]*mobileConn
	nextID atomic.Int64
}

var wsUpgrader = websocket.Upgrader{
	CheckOrigin: func(_ *http.Request) bool { return true }, // mobile clients may not send Origin
}

// New creates a mobile WebSocket IM adapter.
func New(cfg Config) *IM {
	if cfg.Addr == "" {
		cfg.Addr = ":9527"
	}
	return &IM{
		cfg:   cfg,
		conns: make(map[string]*mobileConn),
	}
}

// --- im.Channel ---

func (m *IM) OnMessage(handler im.MessageHandler) { m.handler = handler }

func (m *IM) SendText(chatID, text string) error {
	return m.send(chatID, outboundMsg{Type: "text", ChatID: chatID, Text: text})
}

func (m *IM) SendCard(chatID string, card im.Card) error {
	return m.send(chatID, outboundMsg{Type: "card", ChatID: chatID, Card: card})
}

// SendReaction is a no-op; mobile clients don't have message reaction UX.
func (m *IM) SendReaction(_, _ string) error { return nil }

// Run starts the WebSocket HTTP server and blocks until ctx is cancelled.
func (m *IM) Run(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", m.handleWS)
	srv := &http.Server{Addr: m.cfg.Addr, Handler: mux}
	logger.Debug("[mobile] WebSocket server listening on %s", m.cfg.Addr)

	errCh := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		return srv.Shutdown(context.Background())
	case err := <-errCh:
		return fmt.Errorf("mobile ws server: %w", err)
	}
}

// --- im.DebugSender ---

func (m *IM) SendDebug(chatID, text string) error {
	return m.send(chatID, outboundMsg{Type: "debug", ChatID: chatID, Text: text})
}

// --- im.OptionSender ---

// SendOptions sends a structured option prompt (decision request) to the client.
// meta["decision_id"] is forwarded so the client can include it in its reply.
func (m *IM) SendOptions(chatID, title, body string, options []im.DecisionOption, meta map[string]string) error {
	opts := make([]outboundOption, len(options))
	for i, o := range options {
		opts[i] = outboundOption{ID: o.ID, Label: o.Label}
	}
	decisionID := ""
	if meta != nil {
		decisionID = meta["decision_id"]
	}
	return m.send(chatID, outboundMsg{
		Type:       "options",
		ChatID:     chatID,
		Title:      title,
		Body:       body,
		Options:    opts,
		DecisionID: decisionID,
	})
}

// --- im.CardActionSubscriber ---

func (m *IM) OnCardAction(handler func(im.CardActionEvent)) {
	m.mu.Lock()
	m.cardActionHandler = handler
	m.mu.Unlock()
}

// --- WebSocket internals ---

func (m *IM) handleWS(w http.ResponseWriter, r *http.Request) {
	ws, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.Warn("[mobile] upgrade: %v", err)
		return
	}

	n := m.nextID.Add(1)
	chatID := fmt.Sprintf("mobile-%d", n)
	c := &mobileConn{chatID: chatID, ws: ws, authed: m.cfg.Token == ""}

	m.mu.Lock()
	m.conns[chatID] = c
	m.mu.Unlock()

	defer func() {
		ws.Close()
		m.mu.Lock()
		delete(m.conns, chatID)
		m.mu.Unlock()
		logger.Debug("[mobile] disconnected: %s", chatID)
	}()

	logger.Debug("[mobile] connected: %s from %s", chatID, r.RemoteAddr)

	// Tell client whether auth is needed.
	if m.cfg.Token != "" {
		_ = m.send(chatID, outboundMsg{Type: "auth_required", ChatID: chatID})
	} else {
		_ = m.send(chatID, outboundMsg{Type: "ready", ChatID: chatID})
	}

	for {
		_, data, err := ws.ReadMessage()
		if err != nil {
			break
		}
		var msg inboundMsg
		if json.Unmarshal(data, &msg) != nil {
			continue
		}
		switch msg.Type {
		case "auth":
			ok := m.cfg.Token == "" || msg.Token == m.cfg.Token
			c.mu.Lock()
			c.authed = ok
			c.mu.Unlock()
			if ok {
				_ = m.send(chatID, outboundMsg{Type: "ready", ChatID: chatID})
			} else {
				_ = m.send(chatID, outboundMsg{Type: "error", ChatID: chatID, Message: "invalid token"})
			}

		case "message":
			c.mu.Lock()
			authed := c.authed
			c.mu.Unlock()
			if !authed {
				_ = m.send(chatID, outboundMsg{Type: "error", ChatID: chatID, Message: "not authenticated"})
				continue
			}
			if text := strings.TrimSpace(msg.Text); text != "" && m.handler != nil {
				m.handler(im.Message{ChatID: chatID, Text: text})
			}

		case "option":
			// User selected an option in a decision prompt.
			// Convert to a CardActionEvent so Bridge can resolve the pending decision.
			c.mu.Lock()
			authed := c.authed
			c.mu.Unlock()
			if !authed {
				continue
			}
			m.mu.RLock()
			h := m.cardActionHandler
			m.mu.RUnlock()
			if h != nil && msg.DecisionID != "" && msg.OptionID != "" {
				h(im.CardActionEvent{
					ChatID: chatID,
					UserID: chatID,
					Value: map[string]string{
						"kind":        "decision",
						"decision_id": msg.DecisionID,
						"option_id":   msg.OptionID,
						"chat_id":     chatID,
					},
				})
			}

		case "ping":
			_ = m.send(chatID, outboundMsg{Type: "pong", ChatID: chatID})
		}
	}
}

// send marshals and writes msg to the given chatID's WebSocket connection.
// Silently drops if the client is no longer connected.
func (m *IM) send(chatID string, msg outboundMsg) error {
	m.mu.RLock()
	c, ok := m.conns[chatID]
	m.mu.RUnlock()
	if !ok {
		return nil // silently drop; client disconnected
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ws.WriteMessage(websocket.TextMessage, data)
}
