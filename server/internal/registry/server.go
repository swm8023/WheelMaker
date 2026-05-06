package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	rp "github.com/swm8023/wheelmaker/internal/protocol"
)

const (
	defaultProtocolVersion = rp.DefaultProtocolVersion
	defaultServerVersion   = "0.1.0"
	defaultRequestTimeout  = 10 * time.Second
	clientIdleTimeout      = 5 * time.Minute
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
	pending   map[int64]chan envelope
}

func newPeerConn(ws websocketWriter) *peerConn {
	return &peerConn{
		ws:      ws,
		pending: make(map[int64]chan envelope),
	}
}

func (p *peerConn) write(v any) error {
	p.writeMu.Lock()
	defer p.writeMu.Unlock()
	return p.ws.WriteJSON(v)
}

func (p *peerConn) registerPending(id int64) chan envelope {
	ch := make(chan envelope, 1)
	p.pendingMu.Lock()
	p.pending[id] = ch
	p.pendingMu.Unlock()
	return ch
}

func (p *peerConn) resolvePending(id int64, msg envelope) bool {
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

// Server accepts client/hub connections and routes client requests to hub responders.
type Server struct {
	cfg Config

	mu           sync.RWMutex
	hubs         map[string]rp.HubSnapshot
	projectToHub map[string]string
	hubPeers     map[string]*peerConn
	clientPeers  map[string]*connectionState

	nextConnID    atomic.Int64
	nextForwardID atomic.Int64
	nextConnEpoch atomic.Int64
}

type connectionState struct {
	id              string
	role            string
	hubID           string
	scopeHubID      string
	initialized     bool
	connectionEpoch int64
	peer            *peerConn
	seenRequestIDs  map[int64]struct{}
	lastProjectSeq  map[string]int64
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
		clientPeers:  make(map[string]*connectionState),
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
	registryLogger("").Info("listening on %s", s.cfg.Addr)
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
		id:             fmt.Sprintf("conn-%d", s.nextConnID.Add(1)),
		peer:           newPeerConn(ws),
		seenRequestIDs: map[int64]struct{}{},
		lastProjectSeq: map[string]int64{},
	}
	registryLogger("").Info("ws connected id=%s remote=%s", state.id, r.RemoteAddr)
	defer registryLogger("").Info("ws disconnected id=%s role=%s hub=%s remote=%s", state.id, state.role, state.hubID, r.RemoteAddr)
	defer s.unregisterHub(state.peer, state)
	defer s.unregisterClient(state)
	defer state.peer.dropAllPending()

	var idleTimer *time.Timer
	resetIdleTimer := func() {
		if !state.initialized || (state.role != "client" && state.role != "monitor") {
			return
		}
		if idleTimer == nil {
			idleTimer = time.AfterFunc(clientIdleTimeout, func() {
				_ = state.peer.write(envelope{
					Type:   "event",
					Method: "connection.closing",
					Payload: rp.MustRaw(map[string]any{
						"reason": "idle_timeout",
					}),
				})
				_ = ws.Close()
			})
			return
		}
		idleTimer.Reset(clientIdleTimeout)
	}
	defer func() {
		if idleTimer != nil {
			idleTimer.Stop()
		}
	}()

	for {
		in, invalidRequestID, err := readEnvelope(ws)
		if err != nil {
			return
		}
		if invalidRequestID {
			_ = s.writeError(state.peer, 0, in.Method, codeInvalidArgument, "requestId must be integer", nil)
			continue
		}
		resetIdleTimer()
		if in.Type == "response" || in.Type == "error" {
			if state.peer.resolvePending(in.RequestID, in) {
				continue
			}
		}
		if in.Type != "request" {
			_ = s.writeError(state.peer, in.RequestID, in.Method, codeInvalidArgument, "type must be request", nil)
			continue
		}
		if in.RequestID < 1 {
			_ = s.writeError(state.peer, in.RequestID, in.Method, codeInvalidArgument, "requestId must be >= 1", nil)
			continue
		}
		if _, exists := state.seenRequestIDs[in.RequestID]; exists {
			_ = s.writeError(state.peer, in.RequestID, in.Method, codeConflict, "duplicate requestId", nil)
			continue
		}
		state.seenRequestIDs[in.RequestID] = struct{}{}

		if !state.initialized {
			if in.Method != "connect.init" {
				_ = s.writeError(state.peer, in.RequestID, in.Method, codeUnauthorized, "connect.init required", nil)
				continue
			}
			if !s.handleConnectInit(state.peer, state, in) {
				return
			}
			resetIdleTimer()
			continue
		}

		if !methodAllowed(state.role, in.Method) {
			_ = s.writeError(state.peer, in.RequestID, in.Method, codeForbidden, "method not allowed for role", map[string]any{"role": state.role})
			continue
		}

		switch in.Method {
		case "registry.reportProjects":
			s.handleHubReportProjects(state.peer, state, in)
		case "registry.updateProject":
			s.handleHubUpdateProject(state.peer, state, in)
		case "registry.session.updated":
			s.handleHubSessionEvent(state.peer, state, in, "session.updated")
		case "registry.session.message":
			s.handleHubSessionEvent(state.peer, state, in, "session.message")
		case "project.list":
			s.handleProjectList(state.peer, state, in)
		case "project.syncCheck":
			s.handleProjectSyncCheck(state.peer, state, in)
		case "monitor.listHub":
			s.handleMonitorListHub(state.peer, state, in)
		case "batch":
			s.handleBatch(state.peer, state, in)
		case "hub.ping":
			_ = s.writeResponse(state.peer, in.RequestID, in.Method, "", map[string]any{"ok": true})
		case "monitor.status", "monitor.log", "monitor.db", "monitor.action":
			s.handleMonitorForwardRequest(state.peer, state, in)
		case "chat.send",
			"session.list", "session.read", "session.new", "session.resume.list", "session.resume.import", "session.reload", "session.send", "session.markRead", "session.setConfig", "session.delete", "session.token.providers", "session.token.deepseek.stats", "session.token.scan",
			"fs.list", "fs.info", "fs.read", "fs.search", "fs.grep",
			"git.refs", "git.log", "git.commit.files", "git.commit.fileDiff",
			"git.diff", "git.diff.fileDiff", "git.status", "git.workingTree.fileDiff":
			s.handleForwardRequest(state.peer, state, in)
		default:
			_ = s.writeError(state.peer, in.RequestID, in.Method, codeInvalidArgument, "unsupported method", map[string]any{"method": in.Method})
		}
	}
}

func readEnvelope(ws *websocket.Conn) (envelope, bool, error) {
	type rawEnvelope struct {
		RequestID json.RawMessage `json:"requestId,omitempty"`
		Type      string          `json:"type"`
		Method    string          `json:"method,omitempty"`
		ProjectID string          `json:"projectId,omitempty"`
		Payload   json.RawMessage `json:"payload,omitempty"`
	}
	var raw rawEnvelope
	if err := ws.ReadJSON(&raw); err != nil {
		return envelope{}, false, err
	}

	out := envelope{
		Type:      raw.Type,
		Method:    raw.Method,
		ProjectID: raw.ProjectID,
		Payload:   raw.Payload,
	}
	if len(raw.RequestID) == 0 || strings.TrimSpace(string(raw.RequestID)) == "null" {
		return out, false, nil
	}
	var id int64
	if err := json.Unmarshal(raw.RequestID, &id); err != nil {
		return out, true, nil
	}
	out.RequestID = id
	return out, false, nil
}

func methodAllowed(role string, method string) bool {
	switch role {
	case "hub":
		return method == "registry.reportProjects" || method == "registry.updateProject" || method == "registry.session.updated" || method == "registry.session.message" || method == "hub.ping"
	case "client":
		return method == "project.list" || method == "project.syncCheck" || method == "batch" ||
			method == "chat.send" || strings.HasPrefix(method, "session.") ||
			strings.HasPrefix(method, "fs.") || strings.HasPrefix(method, "git.")
	case "monitor":
		return method == "project.list" || method == "monitor.listHub" || method == "batch" || strings.HasPrefix(method, "monitor.")
	default:
		return false
	}
}
func (s *Server) handleConnectInit(peer *peerConn, state *connectionState, in envelope) bool {
	var payload connectInitPayload
	if err := decodePayload(in.Payload, &payload); err != nil {
		_ = s.writeError(peer, in.RequestID, in.Method, codeInvalidArgument, "invalid connect.init payload", nil)
		return true
	}
	role := strings.TrimSpace(payload.Role)
	if role != "hub" && role != "client" && role != "monitor" {
		_ = s.writeError(peer, in.RequestID, in.Method, codeInvalidArgument, "role must be hub, client, or monitor", nil)
		return true
	}
	if role == "hub" && strings.TrimSpace(payload.HubID) == "" {
		_ = s.writeError(peer, in.RequestID, in.Method, codeInvalidArgument, "hubId is required for hub role", nil)
		return true
	}
	if s.cfg.Token != "" && strings.TrimSpace(payload.Token) != s.cfg.Token {
		_ = s.writeError(peer, in.RequestID, in.Method, codeUnauthorized, "invalid token", nil)
		return false
	}
	if strings.TrimSpace(payload.ProtocolVersion) != s.cfg.ProtocolVersion {
		_ = s.writeError(peer, in.RequestID, in.Method, codeInvalidArgument, "unsupported protocolVersion", map[string]any{"protocolVersion": payload.ProtocolVersion, "supported": s.cfg.ProtocolVersion})
		return true
	}

	state.initialized = true
	state.role = role
	state.hubID = strings.TrimSpace(payload.HubID)
	state.scopeHubID = strings.TrimSpace(payload.HubID)
	state.connectionEpoch = s.nextConnEpoch.Add(1)
	if state.role == "client" || state.role == "monitor" {
		s.mu.Lock()
		s.clientPeers[state.id] = state
		s.mu.Unlock()
	}

	resp := connectInitResponsePayload{
		OK: true,
		Principal: rp.ConnectPrincipal{
			Role:            role,
			HubID:           strings.TrimSpace(payload.HubID),
			ConnectionEpoch: state.connectionEpoch,
		},
		ServerInfo: rp.ConnectServerInfo{
			ServerVersion:   s.cfg.ServerVersion,
			ProtocolVersion: s.cfg.ProtocolVersion,
		},
		Features: rp.ConnectFeatures{
			HubReportProjects:       true,
			PushHint:                false,
			PingPong:                true,
			SupportsHashNegotiation: true,
			SupportsBatch:           true,
		},
		HashAlgorithms: []string{"sha256"},
	}
	_ = s.writeResponse(peer, in.RequestID, in.Method, "", resp)
	return true
}

func (s *Server) handleHubReportProjects(peer *peerConn, state *connectionState, in envelope) {
	var payload hubReportProjectsPayload
	if err := decodePayload(in.Payload, &payload); err != nil {
		_ = s.writeError(peer, in.RequestID, in.Method, codeInvalidArgument, "invalid registry.reportProjects payload", nil)
		return
	}
	if strings.TrimSpace(payload.HubID) == "" {
		payload.HubID = state.hubID
	}
	if payload.HubID != state.hubID {
		_ = s.writeError(peer, in.RequestID, in.Method, codeForbidden, "hubId mismatch", nil)
		return
	}
	if payload.ConnectionEpoch != state.connectionEpoch {
		_ = s.writeError(peer, in.RequestID, in.Method, codeConflict, "connectionEpoch mismatch", nil)
		return
	}

	s.mu.RLock()
	currentHubSnapshot, hasCurrentHubSnapshot := s.hubs[payload.HubID]
	s.mu.RUnlock()
	if hasCurrentHubSnapshot && payload.ConnectionEpoch < currentHubSnapshot.ConnectionEpoch {
		_ = s.writeError(peer, in.RequestID, in.Method, codeConflict, "stale connectionEpoch", map[string]any{
			"hubId":           payload.HubID,
			"connectionEpoch": payload.ConnectionEpoch,
			"currentEpoch":    currentHubSnapshot.ConnectionEpoch,
		})
		return
	}

	sort.Slice(payload.Projects, func(i, j int) bool {
		return payload.Projects[i].Name < payload.Projects[j].Name
	})
	state.lastProjectSeq = map[string]int64{}

	s.mu.Lock()
	previous := s.hubs[payload.HubID]
	s.hubPeers[payload.HubID] = peer
	for projectID, hubID := range s.projectToHub {
		if hubID == payload.HubID {
			delete(s.projectToHub, projectID)
		}
	}
	for _, p := range payload.Projects {
		projectID := rp.ProjectID(payload.HubID, p.Name)
		if strings.TrimSpace(projectID) != "" {
			s.projectToHub[projectID] = payload.HubID
		}
	}
	s.hubs[payload.HubID] = rp.HubSnapshot{
		HubID:           payload.HubID,
		ConnectionEpoch: payload.ConnectionEpoch,
		Projects:        payload.Projects,
		UpdatedAt:       time.Now().UTC().Format(time.RFC3339),
	}
	s.mu.Unlock()
	s.emitProjectSnapshotEvents(payload.HubID, previous.Projects, payload.Projects)

	_ = s.writeResponse(peer, in.RequestID, in.Method, "", map[string]any{
		"ok":           true,
		"hubId":        payload.HubID,
		"projectCount": len(payload.Projects),
	})
}

func (s *Server) handleHubUpdateProject(peer *peerConn, state *connectionState, in envelope) {
	var payload hubUpdateProjectPayload
	if err := decodePayload(in.Payload, &payload); err != nil {
		_ = s.writeError(peer, in.RequestID, in.Method, codeInvalidArgument, "invalid registry.updateProject payload", nil)
		return
	}
	if strings.TrimSpace(payload.HubID) == "" {
		payload.HubID = state.hubID
	}
	if payload.HubID != state.hubID {
		_ = s.writeError(peer, in.RequestID, in.Method, codeForbidden, "hubId mismatch", nil)
		return
	}
	if payload.ConnectionEpoch != state.connectionEpoch {
		_ = s.writeError(peer, in.RequestID, in.Method, codeConflict, "connectionEpoch mismatch", nil)
		return
	}
	if payload.Seq < 1 {
		_ = s.writeError(peer, in.RequestID, in.Method, codeInvalidArgument, "seq must be >= 1", nil)
		return
	}
	projectName := strings.TrimSpace(payload.Project.Name)
	if projectName == "" {
		_ = s.writeError(peer, in.RequestID, in.Method, codeInvalidArgument, "project.name is required", nil)
		return
	}
	if strings.TrimSpace(payload.UpdatedAt) == "" {
		_ = s.writeError(peer, in.RequestID, in.Method, codeInvalidArgument, "updatedAt is required", nil)
		return
	}
	if _, err := time.Parse(time.RFC3339, payload.UpdatedAt); err != nil {
		_ = s.writeError(peer, in.RequestID, in.Method, codeInvalidArgument, "updatedAt must be RFC3339", nil)
		return
	}
	lastSeq := state.lastProjectSeq[projectName]
	if payload.Seq <= lastSeq {
		_ = s.writeError(peer, in.RequestID, in.Method, codeConflict, "stale project update seq", map[string]any{
			"projectName": projectName,
			"lastSeq":     lastSeq,
			"seq":         payload.Seq,
		})
		return
	}

	s.mu.Lock()
	hub := s.hubs[payload.HubID]
	if hub.HubID == "" {
		s.mu.Unlock()
		_ = s.writeError(peer, in.RequestID, in.Method, codeConflict, "hub snapshot not initialized", nil)
		return
	}
	if hub.ConnectionEpoch != state.connectionEpoch {
		s.mu.Unlock()
		_ = s.writeError(peer, in.RequestID, in.Method, codeConflict, "connectionEpoch mismatch", nil)
		return
	}

	var previous *rp.ProjectInfo
	replaced := false
	for index := range hub.Projects {
		if strings.TrimSpace(hub.Projects[index].Name) != projectName {
			continue
		}
		prev := hub.Projects[index]
		previous = &prev
		hub.Projects[index] = payload.Project
		replaced = true
		break
	}
	if !replaced {
		hub.Projects = append(hub.Projects, payload.Project)
	}
	sort.Slice(hub.Projects, func(i, j int) bool {
		return hub.Projects[i].Name < hub.Projects[j].Name
	})
	hub.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	s.hubs[payload.HubID] = hub
	state.lastProjectSeq[projectName] = payload.Seq
	projectID := rp.ProjectID(payload.HubID, payload.Project.Name)
	if strings.TrimSpace(projectID) != "" {
		s.projectToHub[projectID] = payload.HubID
	}
	s.mu.Unlock()

	s.emitProjectUpdateEvents(payload.HubID, previous, payload.Project)
	_ = s.writeResponse(peer, in.RequestID, in.Method, projectID, map[string]any{
		"ok":        true,
		"projectId": projectID,
	})
}

func (s *Server) handleHubSessionEvent(peer *peerConn, state *connectionState, in envelope, eventMethod string) {
	projectID := strings.TrimSpace(in.ProjectID)
	if projectID == "" {
		_ = s.writeError(peer, in.RequestID, in.Method, codeInvalidArgument, "projectId is required", nil)
		return
	}
	if state.hubID != "" && !strings.HasPrefix(projectID, state.hubID+":") {
		_ = s.writeError(peer, in.RequestID, in.Method, codeForbidden, "project out of hub scope", map[string]any{"projectId": projectID})
		return
	}

	s.mu.RLock()
	hubID := s.projectToHub[projectID]
	s.mu.RUnlock()
	if hubID == "" {
		_ = s.writeError(peer, in.RequestID, in.Method, codeNotFound, "project not found", map[string]any{"projectId": projectID})
		return
	}
	if state.hubID != "" && hubID != state.hubID {
		_ = s.writeError(peer, in.RequestID, in.Method, codeForbidden, "project not owned by connected hub", map[string]any{"projectId": projectID})
		return
	}

	s.broadcastProjectEvent(hubID, projectID, eventMethod, json.RawMessage(in.Payload))
	_ = s.writeResponse(peer, in.RequestID, in.Method, projectID, map[string]any{"ok": true})
}

func (s *Server) handleProjectList(peer *peerConn, state *connectionState, in envelope) {
	items := s.snapshotProjects(state.scopeHubID)
	_ = s.writeResponse(peer, in.RequestID, in.Method, "", map[string]any{
		"projects": items,
	})
}

func (s *Server) handleMonitorListHub(peer *peerConn, state *connectionState, in envelope) {
	hubs := s.snapshotHubs()
	_ = s.writeResponse(peer, in.RequestID, in.Method, "", map[string]any{"hubs": hubs})
}

func (s *Server) snapshotHubs() []map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()

	items := make([]map[string]any, 0, len(s.hubs))
	for hubID := range s.hubs {
		_, online := s.hubPeers[hubID]
		items = append(items, map[string]any{
			"hubId":  hubID,
			"online": online,
		})
	}
	sort.Slice(items, func(i, j int) bool {
		li, _ := items[i]["hubId"].(string)
		lj, _ := items[j]["hubId"].(string)
		return li < lj
	})
	return items
}

func (s *Server) handleMonitorForwardRequest(clientPeer *peerConn, state *connectionState, in envelope) {
	resp := s.executeMonitorRequest(state, in)
	resp.RequestID = in.RequestID
	_ = clientPeer.write(resp)
}

func (s *Server) executeMonitorRequest(_ *connectionState, in envelope) envelope {
	var payload monitorHubRefPayload
	if err := decodePayload(in.Payload, &payload); err != nil {
		return s.errorEnvelope(in.Method, codeInvalidArgument, "invalid monitor payload", nil)
	}
	hubID := strings.TrimSpace(payload.HubID)
	if hubID == "" {
		return s.errorEnvelope(in.Method, codeInvalidArgument, "hubId is required", nil)
	}

	s.mu.RLock()
	hubPeer := s.hubPeers[hubID]
	s.mu.RUnlock()
	if hubPeer == nil {
		return s.errorEnvelope(in.Method, codeUnavailable, "hub offline", map[string]any{"hubId": hubID})
	}

	forwardID := s.nextForwardID.Add(1)
	waitCh := hubPeer.registerPending(forwardID)
	err := hubPeer.write(envelope{
		RequestID: forwardID,
		Type:      "request",
		Method:    in.Method,
		Payload:   in.Payload,
	})
	if err != nil {
		hubPeer.resolvePending(forwardID, envelope{})
		return s.errorEnvelope(in.Method, codeInternal, "forward request write failed", nil)
	}

	select {
	case resp, ok := <-waitCh:
		if !ok {
			return s.errorEnvelope(in.Method, codeInternal, "hub disconnected", nil)
		}
		return resp
	case <-time.After(defaultRequestTimeout):
		hubPeer.resolvePending(forwardID, envelope{})
		return s.errorEnvelope(in.Method, codeTimeout, "hub response timeout", nil)
	}
}

func (s *Server) handleProjectSyncCheck(peer *peerConn, state *connectionState, in envelope) {
	resp := s.projectSyncCheckEnvelope(state, in)
	resp.RequestID = in.RequestID
	_ = peer.write(resp)
}

func (s *Server) handleBatch(peer *peerConn, state *connectionState, in envelope) {
	type batchItem struct {
		Method    string          `json:"method"`
		ProjectID string          `json:"projectId,omitempty"`
		Payload   json.RawMessage `json:"payload,omitempty"`
	}
	type batchPayload struct {
		Requests []batchItem `json:"requests"`
	}

	var payload batchPayload
	if err := decodePayload(in.Payload, &payload); err != nil {
		_ = s.writeError(peer, in.RequestID, in.Method, codeInvalidArgument, "invalid batch payload", nil)
		return
	}

	responses := make([]map[string]any, 0, len(payload.Requests))
	for index, item := range payload.Requests {
		if strings.TrimSpace(item.Method) == "" || item.Method == "batch" || item.Method == "connect.init" {
			responses = append(responses, map[string]any{
				"index":  index,
				"type":   "error",
				"method": item.Method,
				"payload": errorPayload{
					Code:    codeInvalidArgument,
					Message: "unsupported batch subrequest",
				},
			})
			continue
		}

		subResp := s.executeBatchRequest(state, envelope{
			Type:      "request",
			Method:    item.Method,
			ProjectID: item.ProjectID,
			Payload:   item.Payload,
		})
		responses = append(responses, map[string]any{
			"index":     index,
			"type":      subResp.Type,
			"method":    subResp.Method,
			"projectId": subResp.ProjectID,
			"payload":   json.RawMessage(subResp.Payload),
		})
	}

	_ = s.writeResponse(peer, in.RequestID, in.Method, "", map[string]any{
		"responses": responses,
	})
}

func (s *Server) executeBatchRequest(state *connectionState, in envelope) envelope {
	switch in.Method {
	case "project.list":
		return envelope{
			Type:   "response",
			Method: in.Method,
			Payload: rp.MustRaw(map[string]any{
				"projects": s.snapshotProjects(state.scopeHubID),
			}),
		}
	case "project.syncCheck":
		return s.projectSyncCheckEnvelope(state, in)
	case "hub.ping":
		return envelope{
			Type:   "response",
			Method: in.Method,
			Payload: rp.MustRaw(map[string]any{
				"ok": true,
			}),
		}
	default:
		return s.executeClientRequest(state, in)
	}
}

func (s *Server) handleForwardRequest(clientPeer *peerConn, state *connectionState, in envelope) {
	resp := s.executeClientRequest(state, in)
	resp.RequestID = in.RequestID
	_ = clientPeer.write(resp)
}

func (s *Server) executeClientRequest(state *connectionState, in envelope) envelope {
	projectID := strings.TrimSpace(in.ProjectID)
	if projectID == "" {
		return s.errorEnvelope(in.Method, codeInvalidArgument, "projectId is required", nil)
	}
	if state.scopeHubID != "" && !strings.HasPrefix(projectID, state.scopeHubID+":") {
		return s.errorEnvelope(in.Method, codeForbidden, "project out of client scope", map[string]any{"projectId": projectID})
	}

	s.mu.RLock()
	hubID := s.projectToHub[projectID]
	hubPeer := s.hubPeers[hubID]
	s.mu.RUnlock()
	if hubID == "" {
		return s.errorEnvelope(in.Method, codeNotFound, "project not found", map[string]any{"projectId": projectID})
	}
	if hubPeer == nil {
		return s.errorEnvelope(in.Method, codeUnavailable, "hub offline", map[string]any{"projectId": projectID})
	}

	forwardID := s.nextForwardID.Add(1)
	waitCh := hubPeer.registerPending(forwardID)

	err := hubPeer.write(envelope{
		RequestID: forwardID,
		Type:      "request",
		Method:    in.Method,
		ProjectID: projectID,
		Payload:   in.Payload,
	})
	if err != nil {
		hubPeer.resolvePending(forwardID, envelope{})
		return s.errorEnvelope(in.Method, codeInternal, "forward request write failed", nil)
	}

	select {
	case resp, ok := <-waitCh:
		if !ok {
			return s.errorEnvelope(in.Method, codeInternal, "hub disconnected", nil)
		}
		resp.ProjectID = projectID
		return resp
	case <-time.After(defaultRequestTimeout):
		hubPeer.resolvePending(forwardID, envelope{})
		return s.errorEnvelope(in.Method, codeTimeout, "hub response timeout", nil)
	}
}

func (s *Server) lookupProject(projectID string) (rp.ProjectListItem, bool) {
	for _, item := range s.snapshotProjects("") {
		if item.ProjectID == projectID {
			return item, true
		}
	}
	return rp.ProjectListItem{}, false
}

func (s *Server) snapshotProjects(scopeHubID string) []rp.ProjectListItem {
	s.mu.RLock()
	defer s.mu.RUnlock()

	items := make([]rp.ProjectListItem, 0, len(s.projectToHub))
	for hubID, hub := range s.hubs {
		if scopeHubID != "" && hubID != scopeHubID {
			continue
		}
		for _, p := range hub.Projects {
			agents := append([]string(nil), p.Agents...)
			items = append(items, rp.ProjectListItem{
				ProjectID:  rp.ProjectID(hubID, p.Name),
				Name:       strings.TrimSpace(p.Name),
				Path:       strings.TrimSpace(p.Path),
				Online:     p.Online,
				Agent:      p.Agent,
				Agents:     agents,
				IMType:     p.IMType,
				ProjectRev: p.ProjectRev,
				Git:        p.Git,
			})
		}
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].ProjectID < items[j].ProjectID
	})
	return items
}

func (s *Server) projectSyncCheckEnvelope(state *connectionState, in envelope) envelope {
	var payload syncCheckPayload
	if err := decodePayload(in.Payload, &payload); err != nil {
		return s.errorEnvelope(in.Method, codeInvalidArgument, "invalid project.syncCheck payload", nil)
	}

	projectID := strings.TrimSpace(in.ProjectID)
	if projectID == "" {
		return s.errorEnvelope(in.Method, codeInvalidArgument, "projectId is required", nil)
	}
	if state.scopeHubID != "" && !strings.HasPrefix(projectID, state.scopeHubID+":") {
		return s.errorEnvelope(in.Method, codeForbidden, "project out of client scope", map[string]any{"projectId": projectID})
	}

	project, ok := s.lookupProject(projectID)
	if !ok {
		return s.errorEnvelope(in.Method, codeNotFound, "project not found", map[string]any{"projectId": projectID})
	}

	stale := make([]string, 0, 3)
	if payload.KnownProjectRev != project.ProjectRev {
		stale = append(stale, "project")
	}
	if payload.KnownGitRev != project.Git.GitRev {
		stale = append(stale, "git")
	}
	if payload.KnownWorktreeRev != project.Git.WorktreeRev {
		stale = append(stale, "worktree")
	}

	return envelope{
		Type:      "response",
		Method:    in.Method,
		ProjectID: projectID,
		Payload: rp.MustRaw(syncCheckResponsePayload{
			ProjectRev:   project.ProjectRev,
			GitRev:       project.Git.GitRev,
			WorktreeRev:  project.Git.WorktreeRev,
			StaleDomains: stale,
		}),
	}
}

func (s *Server) emitProjectSnapshotEvents(hubID string, previous, current []rp.ProjectInfo) {
	prevByName := make(map[string]rp.ProjectInfo, len(previous))
	for _, item := range previous {
		name := strings.TrimSpace(item.Name)
		if name != "" {
			prevByName[name] = item
		}
	}
	for _, item := range current {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			continue
		}
		prev, ok := prevByName[name]
		if ok {
			s.emitProjectUpdateEvents(hubID, &prev, item)
		} else {
			s.emitProjectUpdateEvents(hubID, nil, item)
		}
		delete(prevByName, name)
	}
	for _, item := range prevByName {
		offline := item
		offline.Online = false
		s.emitProjectUpdateEvents(hubID, &item, offline)
	}
}

func (s *Server) emitProjectUpdateEvents(hubID string, previous *rp.ProjectInfo, current rp.ProjectInfo) {
	projectID := rp.ProjectID(hubID, current.Name)
	if strings.TrimSpace(projectID) == "" {
		return
	}
	if previous == nil {
		if current.Online {
			s.broadcastProjectEvent(hubID, projectID, "project.online", map[string]any{})
		}
		return
	}
	if !previous.Online && current.Online {
		s.broadcastProjectEvent(hubID, projectID, "project.online", map[string]any{})
	}
	if previous.Online && !current.Online {
		s.broadcastProjectEvent(hubID, projectID, "project.offline", map[string]any{})
	}
}

func (s *Server) broadcastProjectEvent(hubID, projectID, method string, payload any) {
	s.mu.RLock()
	peers := make([]*peerConn, 0, len(s.clientPeers))
	for _, client := range s.clientPeers {
		if client == nil || client.peer == nil {
			continue
		}
		if client.scopeHubID != "" && client.scopeHubID != hubID {
			continue
		}
		peers = append(peers, client.peer)
	}
	s.mu.RUnlock()

	msg := envelope{
		Type:      "event",
		Method:    method,
		ProjectID: projectID,
		Payload:   rp.MustRaw(payload),
	}
	for _, peer := range peers {
		_ = peer.write(msg)
	}
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

func (s *Server) unregisterHub(peer *peerConn, state *connectionState) {
	if strings.TrimSpace(state.hubID) == "" {
		return
	}
	s.mu.Lock()
	if s.hubPeers[state.hubID] != peer {
		s.mu.Unlock()
		return
	}
	projects := append([]rp.ProjectInfo(nil), s.hubs[state.hubID].Projects...)
	delete(s.hubPeers, state.hubID)
	delete(s.hubs, state.hubID)
	for projectID, hubID := range s.projectToHub {
		if hubID == state.hubID {
			delete(s.projectToHub, projectID)
		}
	}
	s.mu.Unlock()
	for _, item := range projects {
		if strings.TrimSpace(item.Name) == "" || !item.Online {
			continue
		}
		s.broadcastProjectEvent(state.hubID, rp.ProjectID(state.hubID, item.Name), "project.offline", map[string]any{})
	}
}

func (s *Server) unregisterClient(state *connectionState) {
	if state == nil || (state.role != "client" && state.role != "monitor") {
		return
	}
	s.mu.Lock()
	delete(s.clientPeers, state.id)
	s.mu.Unlock()
}

func (s *Server) writeResponse(peer *peerConn, requestID int64, method, projectID string, payload any) error {
	return peer.write(envelope{
		RequestID: requestID,
		Type:      "response",
		Method:    method,
		ProjectID: projectID,
		Payload:   rp.MustRaw(payload),
	})
}

func (s *Server) writeError(peer *peerConn, requestID int64, method, code, message string, details map[string]any) error {
	errEnv := s.errorEnvelope(method, code, message, details)
	errEnv.RequestID = requestID
	return peer.write(errEnv)
}

func (s *Server) errorEnvelope(method, code, message string, details map[string]any) envelope {
	return envelope{
		Type:   "error",
		Method: method,
		Payload: rp.MustRaw(errorPayload{
			Code:    code,
			Message: message,
			Details: details,
		}),
	}
}
