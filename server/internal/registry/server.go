package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

const (
	defaultProtocolVersion = "1.0"
	defaultServerVersion   = "0.1.0"
)

// Config configures the project registry server.
type Config struct {
	Addr            string
	Token           string
	ProtocolVersion string
	ServerVersion   string
}

// ProjectInfo is one project reported by a hub.
type ProjectInfo struct {
	ID     string `json:"id,omitempty"`
	Name   string `json:"name"`
	Path   string `json:"path,omitempty"`
	Agent  string `json:"agent,omitempty"`
	IMType string `json:"imType,omitempty"`
}

// HubSnapshot describes a connected hub and its project list.
type HubSnapshot struct {
	HubID     string        `json:"hubId"`
	Projects  []ProjectInfo `json:"projects"`
	UpdatedAt string        `json:"updatedAt"`
}

// Server accepts hub connections and stores reported project metadata.
type Server struct {
	cfg Config

	mu   sync.RWMutex
	hubs map[string]HubSnapshot

	nextID atomic.Int64
}

type connectionState struct {
	id     string
	authed bool
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(_ *http.Request) bool { return true },
}

// New creates a registry server instance.
func New(cfg Config) *Server {
	if cfg.Addr == "" {
		cfg.Addr = ":9630"
	}
	if cfg.ProtocolVersion == "" {
		cfg.ProtocolVersion = defaultProtocolVersion
	}
	if cfg.ServerVersion == "" {
		cfg.ServerVersion = defaultServerVersion
	}
	return &Server{
		cfg:  cfg,
		hubs: make(map[string]HubSnapshot),
	}
}

// Handler returns the HTTP handler for this server.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", s.handleWS)
	return mux
}

// Run starts HTTP server and blocks until context cancellation.
func (s *Server) Run(ctx context.Context) error {
	srv := &http.Server{
		Addr:    s.cfg.Addr,
		Handler: s.Handler(),
	}
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
		return fmt.Errorf("registry server: %w", err)
	}
}

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer ws.Close()

	state := &connectionState{
		id:     fmt.Sprintf("hub-conn-%d", s.nextID.Add(1)),
		authed: s.cfg.Token == "",
	}
	defer s.dropHub(state.id)

	for {
		var in envelope
		if err := ws.ReadJSON(&in); err != nil {
			return
		}
		if in.Type != "request" {
			_ = s.writeError(ws, in.RequestID, codeInvalidArgument, "type must be request", nil)
			continue
		}

		switch in.Method {
		case "hello":
			s.handleHello(ws, in)
		case "auth":
			s.handleAuth(ws, state, in)
		case "registry.reportProjects":
			if !state.authed {
				_ = s.writeError(ws, in.RequestID, codeUnauthorized, "not authenticated", nil)
				continue
			}
			s.handleHubReportProjects(ws, state, in)
		case "registry.listProjects":
			if !state.authed {
				_ = s.writeError(ws, in.RequestID, codeUnauthorized, "not authenticated", nil)
				continue
			}
			s.handleRegistryListProjects(ws, in)
		default:
			_ = s.writeError(ws, in.RequestID, codeInvalidArgument, "unsupported method", map[string]any{"method": in.Method})
		}
	}
}

func (s *Server) handleHello(ws *websocket.Conn, in envelope) {
	payload := map[string]any{
		"serverVersion":   s.cfg.ServerVersion,
		"protocolVersion": s.cfg.ProtocolVersion,
		"features": map[string]bool{
			"fs":                   false,
			"git":                  false,
			"push":                 false,
			"registryReport":       true,
			"registryListProjects": true,
		},
	}
	_ = s.writeResponse(ws, in.RequestID, in.Method, payload)
}

func (s *Server) handleAuth(ws *websocket.Conn, state *connectionState, in envelope) {
	var payload authPayload
	if err := json.Unmarshal(in.Payload, &payload); err != nil {
		_ = s.writeError(ws, in.RequestID, codeInvalidArgument, "invalid auth payload", nil)
		return
	}
	if s.cfg.Token != "" && payload.Token != s.cfg.Token {
		state.authed = false
		_ = s.writeError(ws, in.RequestID, codeUnauthorized, "invalid token", nil)
		return
	}
	state.authed = true
	_ = s.writeResponse(ws, in.RequestID, in.Method, map[string]any{"ok": true})
}

func (s *Server) handleHubReportProjects(ws *websocket.Conn, state *connectionState, in envelope) {
	var payload hubReportProjectsPayload
	if err := json.Unmarshal(in.Payload, &payload); err != nil {
		_ = s.writeError(ws, in.RequestID, codeInvalidArgument, "invalid registry.reportProjects payload", nil)
		return
	}
	if payload.HubID == "" {
		payload.HubID = state.id
	}
	sort.Slice(payload.Projects, func(i, j int) bool {
		return payload.Projects[i].Name < payload.Projects[j].Name
	})

	s.mu.Lock()
	s.hubs[state.id] = HubSnapshot{
		HubID:     payload.HubID,
		Projects:  payload.Projects,
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	s.mu.Unlock()

	_ = s.writeResponse(ws, in.RequestID, in.Method, map[string]any{
		"hubId":        payload.HubID,
		"projectCount": len(payload.Projects),
	})
}

func (s *Server) handleRegistryListProjects(ws *websocket.Conn, in envelope) {
	s.mu.RLock()
	hubs := make([]HubSnapshot, 0, len(s.hubs))
	for _, h := range s.hubs {
		hubs = append(hubs, h)
	}
	s.mu.RUnlock()
	sort.Slice(hubs, func(i, j int) bool {
		return hubs[i].HubID < hubs[j].HubID
	})
	_ = s.writeResponse(ws, in.RequestID, in.Method, map[string]any{"hubs": hubs})
}

func (s *Server) dropHub(connID string) {
	s.mu.Lock()
	delete(s.hubs, connID)
	s.mu.Unlock()
}

func (s *Server) writeResponse(ws *websocket.Conn, requestID, method string, payload any) error {
	return ws.WriteJSON(envelope{
		Version:   s.cfg.ProtocolVersion,
		RequestID: requestID,
		Type:      "response",
		Method:    method,
		Payload:   mustRaw(payload),
	})
}

func (s *Server) writeError(ws *websocket.Conn, requestID, code, message string, details map[string]any) error {
	return ws.WriteJSON(errorEnvelope{
		Version:   s.cfg.ProtocolVersion,
		RequestID: requestID,
		Type:      "error",
		Error: protocolError{
			Code:    code,
			Message: message,
			Details: details,
		},
	})
}

func mustRaw(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}
