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

// Channel implements im.Channel using an HTTP/WebSocket server.
// Each connected mobile client is assigned a unique chatID.
type Channel struct {
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
func New(cfg Config) *Channel {
	if cfg.Addr == "" {
		cfg.Addr = ":9527"
	}
	return &Channel{
		cfg:   cfg,
		conns: make(map[string]*mobileConn),
	}
}

// --- im.Channel ---

func (m *Channel) OnMessage(handler im.MessageHandler) { m.handler = handler }

// Send dispatches a text message to the connected client.
func (m *Channel) SendText(chatID, text string) error {
	return m.send(chatID, outboundMsg{Type: "text", ChatID: chatID, Text: text})
}

func (m *Channel) Send(chatID, text string, kind im.TextKind) error {
	switch kind {
	case im.TextDebug:
		return m.send(chatID, outboundMsg{Type: "debug", ChatID: chatID, Text: text})
	case im.TextSystem:
		return m.send(chatID, outboundMsg{Type: "text", ChatID: chatID, Text: text})
	default:
		return m.send(chatID, outboundMsg{Type: "text", ChatID: chatID, Text: text})
	}
}

// SendCard sends or renders a card payload to the client.
func (m *Channel) SendCard(chatID, _ string, card im.Card) error {
	switch c := card.(type) {
	case im.OptionsCard:
		opts := make([]outboundOption, len(c.Options))
		for i, o := range c.Options {
			opts[i] = outboundOption{ID: o.ID, Label: o.Label}
		}
		decisionID := ""
		if c.Meta != nil {
			decisionID = c.Meta["decision_id"]
		}
		return m.send(chatID, outboundMsg{
			Type:       "options",
			ChatID:     chatID,
			Title:      c.Title,
			Body:       c.Body,
			Options:    opts,
			DecisionID: decisionID,
		})
	case im.ToolCallCard:
		if msg := im.RenderToolCallMessage(c.Update); msg != "" {
			return m.send(chatID, outboundMsg{Type: "text", ChatID: chatID, Text: msg})
		}
		return nil
	default:
		return m.send(chatID, outboundMsg{Type: "card", ChatID: chatID, Card: card})
	}
}

// SendReaction is a no-op; mobile clients don't have message reaction UX.
func (m *Channel) SendReaction(_, _ string) error { return nil }

// Run starts the WebSocket HTTP server and blocks until ctx is cancelled.
func (m *Channel) Run(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", m.handleWS)
	srv := &http.Server{Addr: m.cfg.Addr, Handler: mux}
	logger.Info("[mobile] WebSocket server listening on %s", m.cfg.Addr)

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

// MarkDone is a no-op for mobile; the client drives its own "done" state.
func (m *Channel) MarkDone(_ string) error { return nil }

func (m *Channel) OnCardAction(handler func(im.CardActionEvent)) {
	m.mu.Lock()
	m.cardActionHandler = handler
	m.mu.Unlock()
}

// --- WebSocket internals ---

func (m *Channel) handleWS(w http.ResponseWriter, r *http.Request) {
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
		logger.Info("[mobile] disconnected: %s", chatID)
	}()

	logger.Info("[mobile] connected: %s from %s", chatID, r.RemoteAddr)

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
func (m *Channel) send(chatID string, msg outboundMsg) error {
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

var _ im.Channel = (*Channel)(nil)
