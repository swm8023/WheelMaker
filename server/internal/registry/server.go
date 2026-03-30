package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	rp "github.com/swm8023/wheelmaker/internal/shared"
)

const (
	defaultProtocolVersion = rp.DefaultProtocolVersion
	defaultServerVersion   = "0.1.0"
	defaultRequestTimeout  = 10 * time.Second
)

// Config configures the project registry server.
type Config struct {
	Addr            string
	Token           string
	ProtocolVersion string
	ServerVersion   string
}

type peerConn struct {
	ws websocketWriter

	writeMu sync.Mutex

	pendingMu sync.Mutex
	pending   map[string]chan envelope
}

func newPeerConn(ws websocketWriter) *peerConn {
	return &peerConn{
		ws:      ws,
		pending: make(map[string]chan envelope),
	}
}

func (p *peerConn) write(v any) error {
	p.writeMu.Lock()
	defer p.writeMu.Unlock()
	return p.ws.WriteJSON(v)
}

func (p *peerConn) registerPending(id string) chan envelope {
	ch := make(chan envelope, 1)
	p.pendingMu.Lock()
	p.pending[id] = ch
	p.pendingMu.Unlock()
	return ch
}

func (p *peerConn) resolvePending(id string, msg envelope) bool {
	p.pendingMu.Lock()
	ch, ok := p.pending[id]
	if ok {
		delete(p.pending, id)
	}
	p.pendingMu.Unlock()
	if ok {
		ch <- msg
		close(ch)
	}
	return ok
}

func (p *peerConn) dropAllPending() {
	p.pendingMu.Lock()
	for id, ch := range p.pending {
		delete(p.pending, id)
		close(ch)
	}
	p.pendingMu.Unlock()
}

type websocketWriter interface {
	WriteJSON(v any) error
}

type websocketReader interface {
	ReadJSON(v any) error
}

// Server accepts app/hub connections and routes app requests to hub responders.
type Server struct {
	cfg Config

	mu           sync.RWMutex
	hubs         map[string]rp.HubSnapshot
	projectToHub map[string]string
	hubPeers     map[string]*peerConn

	nextID atomic.Int64
}

type connectionState struct {
	id     string
	authed bool
	hubID  string
	peer   *peerConn
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
		cfg:          cfg,
		hubs:         make(map[string]rp.HubSnapshot),
		projectToHub: make(map[string]string),
		hubPeers:     make(map[string]*peerConn),
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
	rp.Info("registry: listening on %s", s.cfg.Addr)
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
		id:     fmt.Sprintf("conn-%d", s.nextID.Add(1)),
		authed: s.cfg.Token == "",
		peer:   newPeerConn(ws),
	}
	rp.Info("registry: ws connected id=%s remote=%s authed=%t", state.id, r.RemoteAddr, state.authed)
	defer rp.Info("registry: ws disconnected id=%s hub=%s remote=%s", state.id, state.hubID, r.RemoteAddr)
	defer s.unregisterHub(state.peer, state)
	defer state.peer.dropAllPending()

	for {
		var in envelope
		if err := ws.ReadJSON(&in); err != nil {
			return
		}
		if in.Type == "response" || in.Type == "error" {
			if state.peer.resolvePending(in.RequestID, in) {
				continue
			}
		}
		if in.Type != "request" {
			_ = s.writeError(state.peer, in.RequestID, codeInvalidArgument, "type must be request", nil)
			continue
		}

		switch in.Method {
		case "hello":
			s.handleHello(state.peer, in)
		case "auth":
			s.handleAuth(state.peer, state, in)
		case "registry.reportProjects":
			if !state.authed {
				_ = s.writeError(state.peer, in.RequestID, codeUnauthorized, "not authenticated", nil)
				continue
			}
			s.handleHubReportProjects(state.peer, state, in)
		case "registry.listProjects":
			if !state.authed {
				_ = s.writeError(state.peer, in.RequestID, codeUnauthorized, "not authenticated", nil)
				continue
			}
			s.handleRegistryListProjects(state.peer, in)
		case "project.list":
			if !state.authed {
				_ = s.writeError(state.peer, in.RequestID, codeUnauthorized, "not authenticated", nil)
				continue
			}
			s.handleProjectList(state.peer, in)
		case "project.listFull":
			if !state.authed {
				_ = s.writeError(state.peer, in.RequestID, codeUnauthorized, "not authenticated", nil)
				continue
			}
			s.handleProjectListFull(state.peer, in)
		case "fs.list", "fs.read", "git.branches", "git.log", "git.commit.files", "git.commit.fileDiff":
			if !state.authed {
				_ = s.writeError(state.peer, in.RequestID, codeUnauthorized, "not authenticated", nil)
				continue
			}
			s.handleForwardRequest(state.peer, in)
		default:
			_ = s.writeError(state.peer, in.RequestID, codeInvalidArgument, "unsupported method", map[string]any{"method": in.Method})
		}
	}
}

func (s *Server) handleHello(peer *peerConn, in envelope) {
	rp.Info("registry: hello request requestId=%s", in.RequestID)
	payload := map[string]any{
		"serverVersion":   s.cfg.ServerVersion,
		"protocolVersion": s.cfg.ProtocolVersion,
		"features": map[string]bool{
			"fs":                   true,
			"git":                  true,
			"push":                 false,
			"registryReport":       true,
			"registryListProjects": true,
		},
	}
	_ = s.writeResponse(peer, in.RequestID, in.Method, "", payload)
}

func (s *Server) handleAuth(peer *peerConn, state *connectionState, in envelope) {
	var payload authPayload
	if err := json.Unmarshal(in.Payload, &payload); err != nil {
		_ = s.writeError(peer, in.RequestID, codeInvalidArgument, "invalid auth payload", nil)
		return
	}
	if s.cfg.Token != "" && payload.Token != s.cfg.Token {
		state.authed = false
		rp.Warn("registry: auth failed id=%s requestId=%s", state.id, in.RequestID)
		_ = s.writeError(peer, in.RequestID, codeUnauthorized, "invalid token", nil)
		return
	}
	state.authed = true
	rp.Info("registry: auth ok id=%s requestId=%s", state.id, in.RequestID)
	_ = s.writeResponse(peer, in.RequestID, in.Method, "", map[string]any{"ok": true})
}

func (s *Server) handleHubReportProjects(peer *peerConn, state *connectionState, in envelope) {
	var payload hubReportProjectsPayload
	if err := json.Unmarshal(in.Payload, &payload); err != nil {
		_ = s.writeError(peer, in.RequestID, codeInvalidArgument, "invalid registry.reportProjects payload", nil)
		return
	}
	if payload.HubID == "" {
		payload.HubID = state.id
	}
	sort.Slice(payload.Projects, func(i, j int) bool {
		return payload.Projects[i].Name < payload.Projects[j].Name
	})

	s.mu.Lock()
	state.hubID = payload.HubID
	s.hubPeers[payload.HubID] = peer
	for projectID, hubID := range s.projectToHub {
		if hubID == payload.HubID {
			delete(s.projectToHub, projectID)
		}
	}
	for _, p := range payload.Projects {
		id := strings.TrimSpace(p.ID)
		if id == "" {
			id = strings.TrimSpace(p.Name)
		}
		if id != "" {
			s.projectToHub[id] = payload.HubID
		}
	}
	s.hubs[payload.HubID] = rp.HubSnapshot{
		HubID:     payload.HubID,
		Projects:  payload.Projects,
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	s.mu.Unlock()
	rp.Info("registry: reportProjects hub=%s count=%d", payload.HubID, len(payload.Projects))
	for _, p := range payload.Projects {
		pid := strings.TrimSpace(p.ID)
		if pid == "" {
			pid = strings.TrimSpace(p.Name)
		}
		rp.Info("registry: project hub=%s id=%s name=%s path=%s agent=%s im=%s",
			payload.HubID, pid, p.Name, p.Path, p.Agent, p.IMType)
	}

	_ = s.writeResponse(peer, in.RequestID, in.Method, "", map[string]any{
		"hubId":        payload.HubID,
		"projectCount": len(payload.Projects),
	})
}

func (s *Server) handleRegistryListProjects(peer *peerConn, in envelope) {
	s.mu.RLock()
	hubs := make([]rp.HubSnapshot, 0, len(s.hubs))
	for _, h := range s.hubs {
		hubs = append(hubs, h)
	}
	s.mu.RUnlock()
	sort.Slice(hubs, func(i, j int) bool {
		return hubs[i].HubID < hubs[j].HubID
	})
	_ = s.writeResponse(peer, in.RequestID, in.Method, "", map[string]any{"hubs": hubs})
}

func (s *Server) handleProjectList(peer *peerConn, in envelope) {
	type projectListItem struct {
		ProjectID string `json:"projectId"`
		Name      string `json:"name"`
		Online    bool   `json:"online"`
	}
	items := make([]projectListItem, 0)
	for _, p := range s.snapshotProjects() {
		items = append(items, projectListItem{
			ProjectID: p.ID,
			Name:      p.Name,
			Online:    true,
		})
	}
	_ = s.writeResponse(peer, in.RequestID, in.Method, "", map[string]any{
		"projects": items,
	})
}

func (s *Server) handleProjectListFull(peer *peerConn, in envelope) {
	type listFullPayload struct {
		IncludeStats bool `json:"includeStats,omitempty"`
	}
	type listFullItem struct {
		ProjectID    string         `json:"projectId"`
		Name         string         `json:"name"`
		CWD          string         `json:"cwd,omitempty"`
		Online       bool           `json:"online"`
		Capabilities map[string]any `json:"capabilities"`
		Git          map[string]any `json:"git,omitempty"`
		Stats        map[string]any `json:"stats,omitempty"`
	}
	var payload listFullPayload
	if err := decodePayload(in.Payload, &payload); err != nil {
		_ = s.writeError(peer, in.RequestID, codeInvalidArgument, "invalid project.listFull payload", nil)
		return
	}

	items := make([]listFullItem, 0)
	for _, p := range s.snapshotProjects() {
		item := listFullItem{
			ProjectID: p.ID,
			Name:      p.Name,
			CWD:       p.Path,
			Online:    true,
			Capabilities: map[string]any{
				"fs":  true,
				"git": true,
			},
		}
		if branch := gitCurrentBranch(strings.TrimSpace(p.Path)); branch != "" {
			item.Git = map[string]any{"currentBranch": branch}
		}
		if payload.IncludeStats && strings.TrimSpace(p.UpdatedAt) != "" {
			item.Stats = map[string]any{"lastActiveAt": p.UpdatedAt}
		}
		items = append(items, item)
	}

	_ = s.writeResponse(peer, in.RequestID, in.Method, "", map[string]any{
		"projects": items,
	})
}

type projectSnapshot struct {
	ID        string
	Name      string
	Path      string
	UpdatedAt string
}

func (s *Server) snapshotProjects() []projectSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := make([]projectSnapshot, 0, len(s.projectToHub))
	for projectID, hubID := range s.projectToHub {
		hub := s.hubs[hubID]
		for _, p := range hub.Projects {
			id := strings.TrimSpace(p.ID)
			if id == "" {
				id = strings.TrimSpace(p.Name)
			}
			if id != projectID {
				continue
			}
			items = append(items, projectSnapshot{
				ID:        projectID,
				Name:      strings.TrimSpace(p.Name),
				Path:      strings.TrimSpace(p.Path),
				UpdatedAt: strings.TrimSpace(hub.UpdatedAt),
			})
			break
		}
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].ID < items[j].ID
	})
	return items
}

func gitCurrentBranch(repoPath string) string {
	if repoPath == "" {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 800*time.Millisecond)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "-C", repoPath, "rev-parse", "--abbrev-ref", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	branch := strings.TrimSpace(string(out))
	if branch == "" || branch == "HEAD" {
		return ""
	}
	return branch
}

func decodePayload(raw []byte, out any) error {
	if len(raw) == 0 {
		return nil
	}
	if strings.TrimSpace(string(raw)) == "" {
		return nil
	}
	return json.Unmarshal(raw, out)
}

func (s *Server) handleForwardRequest(appPeer *peerConn, in envelope) {
	projectID := strings.TrimSpace(in.ProjectID)
	if projectID == "" {
		_ = s.writeError(appPeer, in.RequestID, codeInvalidArgument, "projectId is required", nil)
		return
	}

	s.mu.RLock()
	hubID := s.projectToHub[projectID]
	hubPeer := s.hubPeers[hubID]
	s.mu.RUnlock()
	if hubID == "" || hubPeer == nil {
		_ = s.writeError(appPeer, in.RequestID, codeNotFound, "project not found or hub offline", map[string]any{"projectId": projectID})
		return
	}

	forwardID := fmt.Sprintf("fwd-%d", s.nextID.Add(1))
	waitCh := hubPeer.registerPending(forwardID)

	err := hubPeer.write(envelope{
		Version:   s.cfg.ProtocolVersion,
		RequestID: forwardID,
		Type:      "request",
		Method:    in.Method,
		ProjectID: projectID,
		Payload:   in.Payload,
	})
	if err != nil {
		hubPeer.resolvePending(forwardID, envelope{})
		_ = s.writeError(appPeer, in.RequestID, codeInternal, "forward request write failed", nil)
		return
	}

	select {
	case resp, ok := <-waitCh:
		if !ok {
			_ = s.writeError(appPeer, in.RequestID, codeInternal, "hub disconnected", nil)
			return
		}
		resp.RequestID = in.RequestID
		resp.ProjectID = projectID
		_ = appPeer.write(resp)
	case <-time.After(defaultRequestTimeout):
		hubPeer.resolvePending(forwardID, envelope{})
		_ = s.writeError(appPeer, in.RequestID, codeTimeout, "hub response timeout", nil)
	}
}

func (s *Server) unregisterHub(peer *peerConn, state *connectionState) {
	if strings.TrimSpace(state.hubID) == "" {
		return
	}
	s.mu.Lock()
	if s.hubPeers[state.hubID] != peer {
		s.mu.Unlock()
		return
	}
	delete(s.hubPeers, state.hubID)
	delete(s.hubs, state.hubID)
	for projectID, hubID := range s.projectToHub {
		if hubID == state.hubID {
			delete(s.projectToHub, projectID)
		}
	}
	s.mu.Unlock()
}

func (s *Server) writeResponse(peer *peerConn, requestID, method, projectID string, payload any) error {
	return peer.write(envelope{
		Version:   s.cfg.ProtocolVersion,
		RequestID: requestID,
		Type:      "response",
		Method:    method,
		ProjectID: projectID,
		Payload:   rp.MustRaw(payload),
	})
}

func (s *Server) writeError(peer *peerConn, requestID, code, message string, details map[string]any) error {
	return peer.write(errorEnvelope{
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
