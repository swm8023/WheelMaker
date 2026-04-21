package hub

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	rp "github.com/swm8023/wheelmaker/internal/protocol"
)

const (
	defaultProtocolVersion = rp.DefaultProtocolVersion
	codeInvalidArgument    = rp.CodeInvalidArgument
	codeNotFound           = rp.CodeNotFound
	codeInternal           = rp.CodeInternal
)

type ProjectInfo = rp.ProjectInfo
type envelope = rp.Envelope
type errorPayload = rp.ErrorPayload
type monitorActionPayload = rp.MonitorActionPayload
type monitorLogPayload = rp.MonitorLogPayload

type ChatHandler interface {
	HandleChatRequest(ctx context.Context, method string, projectID string, payload json.RawMessage) (any, error)
}

type SessionHandler interface {
	HandleSessionRequest(ctx context.Context, method string, projectID string, payload json.RawMessage) (any, error)
}

// ReporterConfig controls hub->registry connection behavior.
type ReporterConfig struct {
	Server            string
	Port              int
	Token             string
	HubID             string
	ReconnectInterval time.Duration
	PingInterval      time.Duration
	PongTimeout       time.Duration
	MonitorBaseDir    string
}

// Reporter keeps a long-lived hub connection and serves local project queries.
type Reporter struct {
	cfg          ReporterConfig
	debugLog     io.Writer
	writeMu      sync.Mutex
	mu           sync.RWMutex
	projects     []ProjectInfo
	projectsByID map[string]ProjectInfo
	chatByID     map[string]ChatHandler
	sessionByID  map[string]SessionHandler
	conn         *websocket.Conn
	pending      map[int64]chan envelope
	requestSeq   atomic.Int64
	updateSeq    atomic.Int64

	connectionEpoch int64
	monitorCore     *MonitorCore
}

// NewReporter creates a Reporter.
func NewReporter(cfg ReporterConfig, projects []ProjectInfo) *Reporter {
	if cfg.Port == 0 {
		cfg.Port = 9630
	}
	if cfg.ReconnectInterval <= 0 {
		cfg.ReconnectInterval = 2 * time.Second
	}
	if cfg.PingInterval <= 0 {
		cfg.PingInterval = 15 * time.Second
	}
	if cfg.PongTimeout <= 0 {
		cfg.PongTimeout = 45 * time.Second
	}
	if strings.TrimSpace(cfg.HubID) == "" {
		cfg.HubID = "wheelmaker-hub"
	}
	cp := make([]ProjectInfo, len(projects))
	copy(cp, projects)
	byID := make(map[string]ProjectInfo, len(cp))
	for _, p := range cp {
		name := strings.TrimSpace(p.Name)
		if name == "" {
			continue
		}
		publicID := rp.ProjectID(cfg.HubID, name)
		byID[publicID] = p
		byID[name] = p
	}
	monitorBase := strings.TrimSpace(cfg.MonitorBaseDir)
	if monitorBase == "" {
		if home, err := os.UserHomeDir(); err == nil {
			monitorBase = filepath.Join(home, ".wheelmaker")
		}
	}
	r := &Reporter{
		cfg:          cfg,
		projects:     cp,
		projectsByID: byID,
		chatByID:     make(map[string]ChatHandler),
		sessionByID:  make(map[string]SessionHandler),
		pending:      make(map[int64]chan envelope),
		monitorCore:  NewMonitorCore(monitorBase),
	}
	r.requestSeq.Store(2)
	return r
}

// Run holds a persistent connection; reconnects on failure until ctx cancelled.
func (r *Reporter) Run(ctx context.Context) error {
	for {
		if err := r.runSession(ctx); err != nil && ctx.Err() == nil {
			registryLogger("").Warn("reporter session ended: %v", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(r.cfg.ReconnectInterval):
		}
	}
}

// SetDebugLogger sets an optional writer for debug logging of registry envelopes.
func (r *Reporter) SetDebugLogger(w io.Writer) {
	r.debugLog = w
}

func (r *Reporter) RegisterChatHandler(projectID string, handler ChatHandler) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if handler == nil {
		delete(r.chatByID, projectID)
		return
	}
	r.chatByID[projectID] = handler
}

func (r *Reporter) RegisterSessionHandler(projectID string, handler SessionHandler) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if handler == nil {
		delete(r.sessionByID, projectID)
		return
	}
	r.sessionByID[projectID] = handler
}

func (r *Reporter) UpdateProject(project ProjectInfo) error {
	project.Name = strings.TrimSpace(project.Name)
	if project.Name == "" {
		return fmt.Errorf("project name is required")
	}

	now := time.Now().UTC().Format(time.RFC3339)
	seq := r.updateSeq.Add(1)
	changedDomains := []string{"project"}

	r.mu.Lock()
	previous := r.projectsByID[project.Name]
	if previous.Name != "" {
		changedDomains = diffProjectDomains(previous, project)
		if len(changedDomains) == 0 {
			changedDomains = []string{"project"}
		}
	}
	r.setProjectLocked(project)
	conn := r.conn
	connectionEpoch := r.connectionEpoch
	r.mu.Unlock()

	if conn == nil || connectionEpoch == 0 {
		return nil
	}

	requestID := r.requestSeq.Add(1)
	waitCh := make(chan envelope, 1)
	r.mu.Lock()
	r.pending[requestID] = waitCh
	r.mu.Unlock()

	if err := r.writeJSON(conn, "->", envelope{
		RequestID: requestID,
		Type:      "request",
		Method:    "registry.updateProject",
		Payload: rp.MustRaw(map[string]any{
			"hubId":           r.cfg.HubID,
			"connectionEpoch": connectionEpoch,
			"seq":             seq,
			"project":         project,
			"changedDomains":  changedDomains,
			"updatedAt":       now,
		}),
	}); err != nil {
		r.mu.Lock()
		delete(r.pending, requestID)
		r.mu.Unlock()
		return err
	}

	select {
	case resp, ok := <-waitCh:
		if !ok {
			return fmt.Errorf("registry connection closed")
		}
		if resp.Type == "error" {
			var payload errorPayload
			if err := decodePayload(resp.Payload, &payload); err == nil {
				return fmt.Errorf("%s: %s", payload.Code, payload.Message)
			}
			return fmt.Errorf("registry update failed")
		}
		return nil
	case <-time.After(10 * time.Second):
		r.mu.Lock()
		delete(r.pending, requestID)
		r.mu.Unlock()
		return fmt.Errorf("registry update timeout")
	}
}

func (r *Reporter) runSession(ctx context.Context) error {
	wsURL, err := buildWSURL(r.cfg.Server, r.cfg.Port)
	if err != nil {
		return err
	}
	registryLogger("").Info("connecting to %s", wsURL)
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		return fmt.Errorf("dial registry %s: %w", wsURL, err)
	}
	defer conn.Close()
	r.mu.Lock()
	r.conn = conn
	r.mu.Unlock()
	defer r.clearConn(conn)
	registryLogger("").Info("connected to %s", wsURL)
	stop := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-stop:
		}
	}()
	defer close(stop)

	if err := r.handshake(conn); err != nil {
		return err
	}

	keepaliveDone := make(chan struct{})
	defer close(keepaliveDone)
	go r.runKeepalive(ctx, conn, keepaliveDone)

	for {
		var in envelope
		if err := r.readJSON(conn, "<-", &in); err != nil {
			return err
		}
		if in.Type == "response" || in.Type == "error" {
			if r.resolvePending(in.RequestID, in) {
				continue
			}
		}
		if in.Type != "request" {
			continue
		}
		switch in.Method {
		case "chat.send", "chat.permission.respond":
			r.replyChat(conn, in)
		case "session.list", "session.read", "session.new", "session.send", "session.markRead":
			r.replySession(conn, in)
		case "monitor.status":
			r.replyMonitorStatus(conn, in)
		case "monitor.log":
			r.replyMonitorLog(conn, in)
		case "monitor.db":
			r.replyMonitorDB(conn, in)
		case "monitor.action":
			r.replyMonitorAction(conn, in)
		case "fs.list":
			r.replyFSList(conn, in)
		case "fs.info":
			r.replyFSInfo(conn, in)
		case "fs.read":
			r.replyFSRead(conn, in)
		case "fs.search":
			r.replyFSSearch(conn, in)
		case "fs.grep":
			r.replyFSGrep(conn, in)
		case "git.refs", "git.branches":
			r.replyGitRefs(conn, in)
		case "git.log":
			r.replyGitLog(conn, in)
		case "git.commit.files":
			r.replyGitCommitFiles(conn, in)
		case "git.commit.fileDiff":
			r.replyGitCommitFileDiff(conn, in)
		case "git.diff":
			r.replyGitDiff(conn, in)
		case "git.diff.fileDiff":
			r.replyGitDiffFileDiff(conn, in)
		case "git.status":
			r.replyGitStatus(conn, in)
		case "git.workingTree.fileDiff":
			r.replyGitWorkingTreeFileDiff(conn, in)
		default:
			_ = r.writeJSON(conn, "->", envelope{
				RequestID: in.RequestID,
				Type:      "error",
				Method:    in.Method,
				Payload: rp.MustRaw(errorPayload{
					Code:    codeInvalidArgument,
					Message: "unsupported method on hub",
					Details: map[string]any{"method": in.Method},
				}),
			})
		}
	}
}

func (r *Reporter) runKeepalive(ctx context.Context, conn *websocket.Conn, done <-chan struct{}) {
	if r.cfg.PingInterval <= 0 {
		return
	}
	ticker := time.NewTicker(r.cfg.PingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-done:
			return
		case <-ticker.C:
			if err := r.sendHubPing(conn); err != nil {
				registryLogger("").Warn("ping failed: %v", err)
				_ = conn.Close()
				return
			}
		}
	}
}

func (r *Reporter) sendHubPing(conn *websocket.Conn) error {
	requestID := r.requestSeq.Add(1)
	waitCh := make(chan envelope, 1)
	r.mu.Lock()
	r.pending[requestID] = waitCh
	r.mu.Unlock()

	if err := r.writeJSON(conn, "->", envelope{
		RequestID: requestID,
		Type:      "request",
		Method:    "hub.ping",
		Payload:   rp.MustRaw(map[string]any{"ts": time.Now().UTC().Unix()}),
	}); err != nil {
		r.mu.Lock()
		delete(r.pending, requestID)
		r.mu.Unlock()
		return err
	}

	select {
	case resp, ok := <-waitCh:
		if !ok {
			return fmt.Errorf("registry connection closed")
		}
		if resp.Type == "error" {
			var payload errorPayload
			if err := decodePayload(resp.Payload, &payload); err == nil {
				return fmt.Errorf("%s: %s", payload.Code, payload.Message)
			}
			return fmt.Errorf("hub.ping failed")
		}
		return nil
	case <-time.After(r.cfg.PongTimeout):
		r.mu.Lock()
		delete(r.pending, requestID)
		r.mu.Unlock()
		return fmt.Errorf("hub.ping timeout")
	}
}

func (r *Reporter) handshake(conn *websocket.Conn) error {
	if err := r.writeJSON(conn, "->", envelope{
		RequestID: 1,
		Type:      "request",
		Method:    "connect.init",
		Payload: rp.MustRaw(map[string]any{
			"clientName":      "wheelmaker-hub",
			"clientVersion":   "0.1.0",
			"protocolVersion": defaultProtocolVersion,
			"role":            "hub",
			"hubId":           r.cfg.HubID,
			"token":           r.cfg.Token,
		}),
	}); err != nil {
		return err
	}
	resp, err := r.readAck(conn)
	if err != nil {
		return fmt.Errorf("connect.init failed: %w", err)
	}
	var initResp rp.ConnectInitResponsePayload
	if err := decodePayload(resp.Payload, &initResp); err != nil {
		return fmt.Errorf("decode connect.init response: %w", err)
	}
	r.mu.Lock()
	r.connectionEpoch = initResp.Principal.ConnectionEpoch
	projects := append([]ProjectInfo(nil), r.projects...)
	r.mu.Unlock()
	registryLogger("").Info("connect.init ok epoch=%d", initResp.Principal.ConnectionEpoch)

	if err := r.writeJSON(conn, "->", envelope{
		RequestID: 2,
		Type:      "request",
		Method:    "registry.reportProjects",
		Payload: rp.MustRaw(map[string]any{
			"hubId":           r.cfg.HubID,
			"connectionEpoch": initResp.Principal.ConnectionEpoch,
			"projects":        projects,
		}),
	}); err != nil {
		return err
	}
	if _, err := r.readAck(conn); err != nil {
		return fmt.Errorf("registry.reportProjects failed: %w", err)
	}
	registryLogger("").Info("reportProjects ok hubId=%s projects=%d", r.cfg.HubID, len(r.projects))
	return nil
}

func (r *Reporter) PublishProjectEvent(projectID string, method string, payload any) error {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return fmt.Errorf("projectId is required")
	}
	method = strings.TrimSpace(method)
	if method == "" {
		return fmt.Errorf("method is required")
	}

	r.mu.RLock()
	conn := r.conn
	r.mu.RUnlock()
	if conn == nil {
		return nil
	}

	requestID := r.requestSeq.Add(1)
	waitCh := make(chan envelope, 1)
	r.mu.Lock()
	r.pending[requestID] = waitCh
	r.mu.Unlock()

	if err := r.writeJSON(conn, "->", envelope{
		RequestID: requestID,
		Type:      "request",
		Method:    method,
		ProjectID: projectID,
		Payload:   rp.MustRaw(payload),
	}); err != nil {
		r.mu.Lock()
		delete(r.pending, requestID)
		r.mu.Unlock()
		return err
	}

	select {
	case resp, ok := <-waitCh:
		if !ok {
			return fmt.Errorf("registry connection closed")
		}
		if resp.Type == "error" {
			var payload errorPayload
			if err := decodePayload(resp.Payload, &payload); err == nil {
				return fmt.Errorf("%s: %s", payload.Code, payload.Message)
			}
			return fmt.Errorf("registry event publish failed")
		}
		return nil
	case <-time.After(10 * time.Second):
		r.mu.Lock()
		delete(r.pending, requestID)
		r.mu.Unlock()
		return fmt.Errorf("registry event publish timeout")
	}
}

func (r *Reporter) replyFSList(conn *websocket.Conn, req envelope) {
	type fsListPayload struct {
		Path      string `json:"path,omitempty"`
		KnownHash string `json:"knownHash,omitempty"`
	}
	var payload fsListPayload
	if err := decodePayload(req.Payload, &payload); err != nil {
		_ = r.writeError(conn, req.RequestID, codeInvalidArgument, "invalid fs.list payload")
		return
	}
	root, err := r.projectRoot(req.ProjectID)
	if err != nil {
		_ = r.writeError(conn, req.RequestID, codeNotFound, err.Error())
		return
	}
	target, rel, err := safeJoin(root, payload.Path)
	if err != nil {
		_ = r.writeError(conn, req.RequestID, codeInvalidArgument, err.Error())
		return
	}
	entries, err := os.ReadDir(target)
	if err != nil {
		_ = r.writeError(conn, req.RequestID, codeInternal, err.Error())
		return
	}
	sort.Slice(entries, func(i, j int) bool {
		return strings.ToLower(entries[i].Name()) < strings.ToLower(entries[j].Name())
	})
	hash := hashDirectoryEntries(target, entries)
	if payload.KnownHash != "" && payload.KnownHash == hash {
		_ = r.writeJSON(conn, "->", envelope{
			RequestID: req.RequestID,
			Type:      "response",
			Method:    req.Method,
			ProjectID: req.ProjectID,
			Payload: rp.MustRaw(map[string]any{
				"path":        rel,
				"hash":        hash,
				"notModified": true,
			}),
		})
		return
	}
	outEntries := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		kind := resolveDirEntryKind(target, e)
		outEntries = append(outEntries, map[string]any{
			"name": e.Name(),
			"kind": kind,
		})
	}
	_ = r.writeJSON(conn, "->", envelope{
		RequestID: req.RequestID,
		Type:      "response",
		Method:    req.Method,
		ProjectID: req.ProjectID,
		Payload: rp.MustRaw(map[string]any{
			"path":        rel,
			"hash":        hash,
			"notModified": false,
			"entries":     outEntries,
		}),
	})
}

func (r *Reporter) replyMonitorStatus(conn *websocket.Conn, req envelope) {
	status, err := r.monitorCore.GetServiceStatus()
	if err != nil {
		_ = r.writeError(conn, req.RequestID, codeInternal, err.Error())
		return
	}
	_ = r.writeJSON(conn, "->", envelope{RequestID: req.RequestID, Type: "response", Method: req.Method, Payload: rp.MustRaw(status)})
}

func (r *Reporter) replyMonitorLog(conn *websocket.Conn, req envelope) {
	var payload monitorLogPayload
	if err := decodePayload(req.Payload, &payload); err != nil {
		_ = r.writeError(conn, req.RequestID, codeInvalidArgument, "invalid monitor.log payload")
		return
	}
	result, err := r.monitorCore.GetLogs(payload.File, payload.Level, payload.Tail)
	if err != nil {
		_ = r.writeError(conn, req.RequestID, codeInternal, err.Error())
		return
	}
	_ = r.writeJSON(conn, "->", envelope{RequestID: req.RequestID, Type: "response", Method: req.Method, Payload: rp.MustRaw(result)})
}

func (r *Reporter) replyMonitorDB(conn *websocket.Conn, req envelope) {
	result := r.monitorCore.GetDBTables()
	_ = r.writeJSON(conn, "->", envelope{RequestID: req.RequestID, Type: "response", Method: req.Method, Payload: rp.MustRaw(result)})
}

func (r *Reporter) replyMonitorAction(conn *websocket.Conn, req envelope) {
	var payload monitorActionPayload
	if err := decodePayload(req.Payload, &payload); err != nil {
		_ = r.writeError(conn, req.RequestID, codeInvalidArgument, "invalid monitor.action payload")
		return
	}
	action := strings.TrimSpace(payload.Action)
	if action == "" {
		_ = r.writeError(conn, req.RequestID, codeInvalidArgument, "action is required")
		return
	}
	if err := r.monitorCore.ExecuteAction(action); err != nil {
		_ = r.writeError(conn, req.RequestID, codeInternal, err.Error())
		return
	}
	_ = r.writeJSON(conn, "->", envelope{RequestID: req.RequestID, Type: "response", Method: req.Method, Payload: rp.MustRaw(map[string]any{"ok": true, "action": action})})
}

func (r *Reporter) replyChat(conn *websocket.Conn, req envelope) {
	projectID := strings.TrimSpace(req.ProjectID)
	if projectID == "" {
		_ = r.writeError(conn, req.RequestID, codeInvalidArgument, "projectId is required")
		return
	}

	r.mu.RLock()
	handler := r.chatByID[projectID]
	r.mu.RUnlock()
	if handler == nil {
		_ = r.writeError(conn, req.RequestID, codeNotFound, "chat unavailable for project")
		return
	}

	payload, err := handler.HandleChatRequest(context.Background(), req.Method, projectID, req.Payload)
	if err != nil {
		_ = r.writeError(conn, req.RequestID, codeInternal, err.Error())
		return
	}

	_ = r.writeJSON(conn, "->", envelope{
		RequestID: req.RequestID,
		Type:      "response",
		Method:    req.Method,
		ProjectID: projectID,
		Payload:   rp.MustRaw(payload),
	})
}

func (r *Reporter) replySession(conn *websocket.Conn, req envelope) {
	projectID := strings.TrimSpace(req.ProjectID)
	if projectID == "" {
		_ = r.writeError(conn, req.RequestID, codeInvalidArgument, "projectId is required")
		return
	}

	r.mu.RLock()
	handler := r.sessionByID[projectID]
	r.mu.RUnlock()
	if handler == nil {
		_ = r.writeError(conn, req.RequestID, codeNotFound, "session unavailable for project")
		return
	}

	payload, err := handler.HandleSessionRequest(context.Background(), req.Method, projectID, req.Payload)
	if err != nil {
		_ = r.writeError(conn, req.RequestID, codeInternal, err.Error())
		return
	}

	_ = r.writeJSON(conn, "->", envelope{
		RequestID: req.RequestID,
		Type:      "response",
		Method:    req.Method,
		ProjectID: projectID,
		Payload:   rp.MustRaw(payload),
	})
}

func (r *Reporter) replyFSInfo(conn *websocket.Conn, req envelope) {
	type fsInfoPayload struct {
		Path string `json:"path"`
	}
	var payload fsInfoPayload
	if err := decodePayload(req.Payload, &payload); err != nil {
		_ = r.writeError(conn, req.RequestID, codeInvalidArgument, "invalid fs.info payload")
		return
	}
	root, err := r.projectRoot(req.ProjectID)
	if err != nil {
		_ = r.writeError(conn, req.RequestID, codeNotFound, err.Error())
		return
	}
	target, rel, err := safeJoin(root, payload.Path)
	if err != nil {
		_ = r.writeError(conn, req.RequestID, codeInvalidArgument, err.Error())
		return
	}
	info, err := os.Stat(target)
	if err != nil {
		_ = r.writeError(conn, req.RequestID, codeNotFound, err.Error())
		return
	}
	if info.IsDir() {
		entries, err := os.ReadDir(target)
		if err != nil {
			_ = r.writeError(conn, req.RequestID, codeInternal, err.Error())
			return
		}
		_ = r.writeJSON(conn, "->", envelope{
			RequestID: req.RequestID,
			Type:      "response",
			Method:    req.Method,
			ProjectID: req.ProjectID,
			Payload: rp.MustRaw(map[string]any{
				"path":       rel,
				"kind":       "dir",
				"entryCount": len(entries),
				"hash":       hashDirectoryEntries(target, entries),
			}),
		})
		return
	}

	data, err := os.ReadFile(target)
	if err != nil {
		_ = r.writeError(conn, req.RequestID, codeInternal, err.Error())
		return
	}
	isBinary, mimeType := detectBinaryAndMime(data)
	resp := map[string]any{
		"path":     rel,
		"kind":     "file",
		"size":     info.Size(),
		"isBinary": isBinary,
		"mimeType": mimeType,
		"hash":     hashBytes(data),
	}
	if !isBinary {
		resp["totalLines"] = countLines(data)
	}
	_ = r.writeJSON(conn, "->", envelope{
		RequestID: req.RequestID,
		Type:      "response",
		Method:    req.Method,
		ProjectID: req.ProjectID,
		Payload:   rp.MustRaw(resp),
	})
}

func (r *Reporter) replyFSRead(conn *websocket.Conn, req envelope) {
	type fsReadPayload struct {
		Path      string `json:"path"`
		KnownHash string `json:"knownHash,omitempty"`
	}
	var payload fsReadPayload
	if err := decodePayload(req.Payload, &payload); err != nil {
		_ = r.writeError(conn, req.RequestID, codeInvalidArgument, "invalid fs.read payload")
		return
	}
	root, err := r.projectRoot(req.ProjectID)
	if err != nil {
		_ = r.writeError(conn, req.RequestID, codeNotFound, err.Error())
		return
	}
	target, rel, err := safeJoin(root, payload.Path)
	if err != nil {
		_ = r.writeError(conn, req.RequestID, codeInvalidArgument, err.Error())
		return
	}
	data, err := os.ReadFile(target)
	if err != nil {
		_ = r.writeError(conn, req.RequestID, codeInternal, err.Error())
		return
	}
	hash := hashBytes(data)
	if payload.KnownHash != "" && payload.KnownHash == hash {
		_ = r.writeJSON(conn, "->", envelope{
			RequestID: req.RequestID,
			Type:      "response",
			Method:    req.Method,
			ProjectID: req.ProjectID,
			Payload: rp.MustRaw(map[string]any{
				"path":        rel,
				"hash":        hash,
				"notModified": true,
			}),
		})
		return
	}
	isBinary, mimeType := detectBinaryAndMime(data)
	if isBinary {
		r.replyFSReadBinary(conn, req, rel, data, hash, mimeType)
		return
	}
	r.replyFSReadText(conn, req, rel, data, hash, mimeType)
}

func (r *Reporter) replyFSReadText(conn *websocket.Conn, req envelope, rel string, data []byte, hash string, mimeType string) {
	lines := splitFileLines(data)
	total := len(lines)
	content := strings.Join(lines, "\n")
	_ = r.writeJSON(conn, "->", envelope{
		RequestID: req.RequestID,
		Type:      "response",
		Method:    req.Method,
		ProjectID: req.ProjectID,
		Payload: rp.MustRaw(map[string]any{
			"path":        rel,
			"hash":        hash,
			"notModified": false,
			"isBinary":    false,
			"mimeType":    mimeType,
			"encoding":    "utf-8",
			"content":     content,
			"size":        len(data),
			"total":       total,
			"returned":    total,
		}),
	})
}

func (r *Reporter) replyFSReadBinary(conn *websocket.Conn, req envelope, rel string, data []byte, hash string, mimeType string) {
	_ = r.writeJSON(conn, "->", envelope{
		RequestID: req.RequestID,
		Type:      "response",
		Method:    req.Method,
		ProjectID: req.ProjectID,
		Payload: rp.MustRaw(map[string]any{
			"path":        rel,
			"hash":        hash,
			"notModified": false,
			"isBinary":    true,
			"mimeType":    mimeType,
			"encoding":    "base64",
			"content":     base64.StdEncoding.EncodeToString(data),
			"size":        len(data),
			"total":       len(data),
			"returned":    len(data),
		}),
	})
}
func (r *Reporter) replyFSSearch(conn *websocket.Conn, req envelope) {
	type payload struct {
		Query         string `json:"query"`
		Root          string `json:"root,omitempty"`
		CaseSensitive bool   `json:"caseSensitive,omitempty"`
		Limit         int    `json:"limit,omitempty"`
	}
	var p payload
	if err := decodePayload(req.Payload, &p); err != nil || strings.TrimSpace(p.Query) == "" {
		_ = r.writeError(conn, req.RequestID, codeInvalidArgument, "invalid fs.search payload")
		return
	}
	root, err := r.projectRoot(req.ProjectID)
	if err != nil {
		_ = r.writeError(conn, req.RequestID, codeNotFound, err.Error())
		return
	}
	searchRoot, relRoot, err := safeJoin(root, p.Root)
	if err != nil {
		_ = r.writeError(conn, req.RequestID, codeInvalidArgument, err.Error())
		return
	}
	limit := p.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	needle := normalizeNeedle(p.Query, p.CaseSensitive)
	results := make([]map[string]any, 0, limit)
	_ = filepath.WalkDir(searchRoot, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil || d == nil || path == searchRoot || len(results) >= limit {
			return walkErr
		}
		score := fuzzyScore(normalizeNeedle(d.Name(), p.CaseSensitive), needle)
		if score <= 0 {
			return nil
		}
		relPath, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		kind := "file"
		if d.IsDir() {
			kind = "dir"
		}
		results = append(results, map[string]any{
			"path":  filepath.ToSlash(relPath),
			"name":  d.Name(),
			"kind":  kind,
			"score": score,
		})
		return nil
	})
	sort.Slice(results, func(i, j int) bool {
		left := results[i]["score"].(float64)
		right := results[j]["score"].(float64)
		if left == right {
			return results[i]["path"].(string) < results[j]["path"].(string)
		}
		return left > right
	})
	_ = r.writeJSON(conn, "->", envelope{
		RequestID: req.RequestID,
		Type:      "response",
		Method:    req.Method,
		ProjectID: req.ProjectID,
		Payload: rp.MustRaw(map[string]any{
			"root":       relRoot,
			"results":    results,
			"nextCursor": "",
		}),
	})
}

func (r *Reporter) replyFSGrep(conn *websocket.Conn, req envelope) {
	type payload struct {
		Pattern       string `json:"pattern"`
		Root          string `json:"root,omitempty"`
		IsRegex       bool   `json:"isRegex,omitempty"`
		CaseSensitive bool   `json:"caseSensitive,omitempty"`
		IncludeGlob   string `json:"includeGlob,omitempty"`
		ContextLines  int    `json:"contextLines,omitempty"`
		Limit         int    `json:"limit,omitempty"`
	}
	var p payload
	if err := decodePayload(req.Payload, &p); err != nil || strings.TrimSpace(p.Pattern) == "" {
		_ = r.writeError(conn, req.RequestID, codeInvalidArgument, "invalid fs.grep payload")
		return
	}
	root, err := r.projectRoot(req.ProjectID)
	if err != nil {
		_ = r.writeError(conn, req.RequestID, codeNotFound, err.Error())
		return
	}
	searchRoot, relRoot, err := safeJoin(root, p.Root)
	if err != nil {
		_ = r.writeError(conn, req.RequestID, codeInvalidArgument, err.Error())
		return
	}
	matcher, err := buildGrepMatcher(p.Pattern, p.IsRegex, p.CaseSensitive)
	if err != nil {
		_ = r.writeError(conn, req.RequestID, codeInvalidArgument, err.Error())
		return
	}
	contextLines := p.ContextLines
	if contextLines < 0 || contextLines > 10 {
		contextLines = 2
	}
	limit := p.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	results := make([]map[string]any, 0, limit)
	totalMatches := 0
	_ = filepath.WalkDir(searchRoot, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil || d == nil || d.IsDir() || len(results) >= limit {
			return walkErr
		}
		relPath, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		relPath = filepath.ToSlash(relPath)
		if p.IncludeGlob != "" {
			matched, err := filepath.Match(p.IncludeGlob, filepath.Base(relPath))
			if err != nil || !matched {
				return nil
			}
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		isBinary, _ := detectBinaryAndMime(data)
		if isBinary {
			return nil
		}
		lines := splitFileLines(data)
		matches := make([]map[string]any, 0)
		for index, line := range lines {
			if !matcher(line) {
				continue
			}
			totalMatches++
			beforeStart := maxInt(0, index-contextLines)
			afterEnd := minInt(len(lines), index+1+contextLines)
			matches = append(matches, map[string]any{
				"line":          index + 1,
				"content":       line,
				"contextBefore": lines[beforeStart:index],
				"contextAfter":  lines[index+1 : afterEnd],
			})
			if len(matches) >= limit {
				break
			}
		}
		if len(matches) == 0 {
			return nil
		}
		results = append(results, map[string]any{
			"path":    relPath,
			"matches": matches,
		})
		return nil
	})
	_ = r.writeJSON(conn, "->", envelope{
		RequestID: req.RequestID,
		Type:      "response",
		Method:    req.Method,
		ProjectID: req.ProjectID,
		Payload: rp.MustRaw(map[string]any{
			"root":         relRoot,
			"results":      results,
			"totalMatches": totalMatches,
			"nextCursor":   "",
		}),
	})
}

func (r *Reporter) replyGitRefs(conn *websocket.Conn, req envelope) {
	root, err := r.projectRoot(req.ProjectID)
	if err != nil {
		_ = r.writeError(conn, req.RequestID, codeNotFound, err.Error())
		return
	}
	current, _ := runGit(root, "rev-parse", "--abbrev-ref", "HEAD")
	branchesRaw, err := runGit(root, "branch", "--format=%(refname:short)")
	if err != nil {
		_ = r.writeError(conn, req.RequestID, codeInternal, err.Error())
		return
	}
	branches := make([]string, 0)
	for _, line := range strings.Split(strings.TrimSpace(branchesRaw), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			branches = append(branches, line)
		}
	}
	remoteRaw, err := runGit(root, "for-each-ref", "--format=%(refname:short)", "refs/remotes")
	if err != nil {
		_ = r.writeError(conn, req.RequestID, codeInternal, err.Error())
		return
	}
	remoteBranches := make([]string, 0)
	for _, line := range strings.Split(strings.TrimSpace(remoteRaw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasSuffix(line, "/HEAD") {
			continue
		}
		remoteBranches = append(remoteBranches, line)
	}
	tagsRaw, err := runGit(root, "tag", "--list")
	if err != nil {
		_ = r.writeError(conn, req.RequestID, codeInternal, err.Error())
		return
	}
	tags := make([]map[string]any, 0)
	for _, line := range strings.Split(strings.TrimSpace(tagsRaw), "\n") {
		tag := strings.TrimSpace(line)
		if tag == "" {
			continue
		}
		sha, err := runGit(root, "rev-list", "-n", "1", tag)
		if err != nil {
			continue
		}
		tags = append(tags, map[string]any{
			"name": tag,
			"sha":  strings.TrimSpace(sha),
		})
	}
	_ = r.writeJSON(conn, "->", envelope{
		RequestID: req.RequestID,
		Type:      "response",
		Method:    req.Method,
		ProjectID: req.ProjectID,
		Payload: rp.MustRaw(map[string]any{
			"current":        strings.TrimSpace(current),
			"branches":       branches,
			"remoteBranches": remoteBranches,
			"tags":           tags,
		}),
	})
}
func (r *Reporter) replyGitLog(conn *websocket.Conn, req envelope) {
	type gitLogPayload struct {
		Ref    string   `json:"ref,omitempty"`
		Refs   []string `json:"refs,omitempty"`
		Cursor string   `json:"cursor,omitempty"`
		Limit  int      `json:"limit,omitempty"`
	}
	var payload gitLogPayload
	if err := decodePayload(req.Payload, &payload); err != nil {
		_ = r.writeError(conn, req.RequestID, codeInvalidArgument, "invalid git.log payload")
		return
	}
	root, err := r.projectRoot(req.ProjectID)
	if err != nil {
		_ = r.writeError(conn, req.RequestID, codeNotFound, err.Error())
		return
	}

	requestedRefs := make([]string, 0, len(payload.Refs)+1)
	if ref := strings.TrimSpace(payload.Ref); ref != "" {
		requestedRefs = append(requestedRefs, ref)
	}
	for _, candidate := range payload.Refs {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		requestedRefs = append(requestedRefs, candidate)
	}
	if len(requestedRefs) == 0 {
		requestedRefs = append(requestedRefs, "HEAD")
	}
	refs := make([]string, 0, len(requestedRefs))
	seenRefs := make(map[string]struct{}, len(requestedRefs))
	for _, ref := range requestedRefs {
		if _, ok := seenRefs[ref]; ok {
			continue
		}
		seenRefs[ref] = struct{}{}
		refs = append(refs, ref)
	}

	limit := payload.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	offset := 0
	if payload.Cursor != "" {
		if v, err := strconv.Atoi(payload.Cursor); err == nil && v >= 0 {
			offset = v
		}
	}

	args := []string{"log"}
	args = append(args, refs...)
	args = append(args, "--date=iso-strict", "--pretty=format:%H%x1f%an%x1f%ae%x1f%aI%x1f%s")
	raw, err := runGit(root, args...)
	if err != nil {
		_ = r.writeError(conn, req.RequestID, codeInternal, err.Error())
		return
	}
	lines := make([]string, 0)
	for _, line := range strings.Split(strings.TrimSpace(raw), "\n") {
		if strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	if offset > len(lines) {
		offset = len(lines)
	}
	end := offset + limit
	if end > len(lines) {
		end = len(lines)
	}
	commits := make([]map[string]any, 0, end-offset)
	for _, line := range lines[offset:end] {
		parts := strings.Split(line, "\x1f")
		if len(parts) < 5 {
			continue
		}
		commits = append(commits, map[string]any{
			"sha":    parts[0],
			"author": parts[1],
			"email":  parts[2],
			"time":   parts[3],
			"title":  parts[4],
		})
	}
	nextCursor := ""
	if end < len(lines) {
		nextCursor = strconv.Itoa(end)
	}
	_ = r.writeJSON(conn, "->", envelope{
		RequestID: req.RequestID,
		Type:      "response",
		Method:    req.Method,
		ProjectID: req.ProjectID,
		Payload: rp.MustRaw(map[string]any{
			"ref":        refs[0],
			"refs":       refs,
			"commits":    commits,
			"nextCursor": nextCursor,
		}),
	})
}
func (r *Reporter) replyGitCommitFiles(conn *websocket.Conn, req envelope) {
	type payload struct {
		SHA string `json:"sha"`
	}
	var p payload
	if err := decodePayload(req.Payload, &p); err != nil || strings.TrimSpace(p.SHA) == "" {
		_ = r.writeError(conn, req.RequestID, codeInvalidArgument, "invalid git.commit.files payload")
		return
	}
	root, err := r.projectRoot(req.ProjectID)
	if err != nil {
		_ = r.writeError(conn, req.RequestID, codeNotFound, err.Error())
		return
	}
	numstatRaw, err := runGit(root, "show", "--numstat", "--format=", p.SHA)
	if err != nil {
		_ = r.writeError(conn, req.RequestID, codeInternal, err.Error())
		return
	}
	statusRaw, _ := runGit(root, "diff-tree", "--no-commit-id", "--name-status", "-r", p.SHA)
	statusMap := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(statusRaw), "\n") {
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) == 2 {
			statusMap[strings.TrimSpace(parts[1])] = strings.TrimSpace(parts[0])
		}
	}
	files := make([]map[string]any, 0)
	for _, line := range strings.Split(strings.TrimSpace(numstatRaw), "\n") {
		parts := strings.Split(line, "\t")
		if len(parts) < 3 {
			continue
		}
		path := strings.TrimSpace(parts[2])
		additions, _ := strconv.Atoi(strings.ReplaceAll(parts[0], "-", "0"))
		deletions, _ := strconv.Atoi(strings.ReplaceAll(parts[1], "-", "0"))
		status := statusMap[path]
		if status == "" {
			status = "M"
		}
		files = append(files, map[string]any{
			"path":      path,
			"status":    status,
			"additions": additions,
			"deletions": deletions,
		})
	}
	_ = r.writeJSON(conn, "->", envelope{
		RequestID: req.RequestID,
		Type:      "response",
		Method:    req.Method,
		ProjectID: req.ProjectID,
		Payload: rp.MustRaw(map[string]any{
			"sha":   p.SHA,
			"files": files,
		}),
	})
}

func (r *Reporter) replyGitCommitFileDiff(conn *websocket.Conn, req envelope) {
	type payload struct {
		SHA          string `json:"sha"`
		Path         string `json:"path"`
		ContextLines int    `json:"contextLines,omitempty"`
	}
	var p payload
	if err := decodePayload(req.Payload, &p); err != nil || strings.TrimSpace(p.SHA) == "" || strings.TrimSpace(p.Path) == "" {
		_ = r.writeError(conn, req.RequestID, codeInvalidArgument, "invalid git.commit.fileDiff payload")
		return
	}
	root, err := r.projectRoot(req.ProjectID)
	if err != nil {
		_ = r.writeError(conn, req.RequestID, codeNotFound, err.Error())
		return
	}
	contextLines := p.ContextLines
	if contextLines < 0 || contextLines > 20 {
		contextLines = 3
	}
	diff, err := runGit(root, "show", "--no-color", fmt.Sprintf("--unified=%d", contextLines), p.SHA, "--", p.Path)
	if err != nil {
		_ = r.writeError(conn, req.RequestID, codeInternal, err.Error())
		return
	}
	isBinary := strings.Contains(diff, "Binary files")
	_ = r.writeJSON(conn, "->", envelope{
		RequestID: req.RequestID,
		Type:      "response",
		Method:    req.Method,
		ProjectID: req.ProjectID,
		Payload: rp.MustRaw(map[string]any{
			"sha":       p.SHA,
			"path":      p.Path,
			"isBinary":  isBinary,
			"diff":      diff,
			"truncated": false,
		}),
	})
}

func (r *Reporter) replyGitDiff(conn *websocket.Conn, req envelope) {
	type payload struct {
		Base string `json:"base"`
		Head string `json:"head"`
	}
	var p payload
	if err := decodePayload(req.Payload, &p); err != nil || strings.TrimSpace(p.Base) == "" || strings.TrimSpace(p.Head) == "" {
		_ = r.writeError(conn, req.RequestID, codeInvalidArgument, "invalid git.diff payload")
		return
	}
	root, err := r.projectRoot(req.ProjectID)
	if err != nil {
		_ = r.writeError(conn, req.RequestID, codeNotFound, err.Error())
		return
	}
	files, err := gitDiffFiles(root, p.Base, p.Head)
	if err != nil {
		_ = r.writeError(conn, req.RequestID, codeInternal, err.Error())
		return
	}
	_ = r.writeJSON(conn, "->", envelope{
		RequestID: req.RequestID,
		Type:      "response",
		Method:    req.Method,
		ProjectID: req.ProjectID,
		Payload: rp.MustRaw(map[string]any{
			"base":       p.Base,
			"head":       p.Head,
			"files":      files,
			"nextCursor": "",
		}),
	})
}

func (r *Reporter) replyGitDiffFileDiff(conn *websocket.Conn, req envelope) {
	type payload struct {
		Base         string `json:"base"`
		Head         string `json:"head"`
		Path         string `json:"path"`
		ContextLines int    `json:"contextLines,omitempty"`
	}
	var p payload
	if err := decodePayload(req.Payload, &p); err != nil || strings.TrimSpace(p.Base) == "" || strings.TrimSpace(p.Head) == "" || strings.TrimSpace(p.Path) == "" {
		_ = r.writeError(conn, req.RequestID, codeInvalidArgument, "invalid git.diff.fileDiff payload")
		return
	}
	root, err := r.projectRoot(req.ProjectID)
	if err != nil {
		_ = r.writeError(conn, req.RequestID, codeNotFound, err.Error())
		return
	}
	contextLines := normalizeContextLines(p.ContextLines)
	diff, err := runGit(root, "diff", "--no-color", fmt.Sprintf("--unified=%d", contextLines), p.Base, p.Head, "--", p.Path)
	if err != nil {
		_ = r.writeError(conn, req.RequestID, codeInternal, err.Error())
		return
	}
	isBinary := strings.Contains(diff, "Binary files")
	_ = r.writeJSON(conn, "->", envelope{
		RequestID: req.RequestID,
		Type:      "response",
		Method:    req.Method,
		ProjectID: req.ProjectID,
		Payload: rp.MustRaw(map[string]any{
			"base":      p.Base,
			"head":      p.Head,
			"path":      p.Path,
			"isBinary":  isBinary,
			"diff":      diff,
			"truncated": false,
		}),
	})
}

func (r *Reporter) replyGitStatus(conn *websocket.Conn, req envelope) {
	root, err := r.projectRoot(req.ProjectID)
	if err != nil {
		_ = r.writeError(conn, req.RequestID, codeNotFound, err.Error())
		return
	}
	raw, err := runGit(root, "status", "--porcelain")
	if err != nil {
		_ = r.writeError(conn, req.RequestID, codeInternal, err.Error())
		return
	}
	staged, unstaged, untracked := parsePorcelainStatus(raw)
	_ = r.writeJSON(conn, "->", envelope{
		RequestID: req.RequestID,
		Type:      "response",
		Method:    req.Method,
		ProjectID: req.ProjectID,
		Payload: rp.MustRaw(map[string]any{
			"dirty":       len(staged)+len(unstaged)+len(untracked) > 0,
			"worktreeRev": hashBytes([]byte(raw)),
			"staged":      staged,
			"unstaged":    unstaged,
			"untracked":   untracked,
		}),
	})
}

func (r *Reporter) replyGitWorkingTreeFileDiff(conn *websocket.Conn, req envelope) {
	type payload struct {
		Path         string `json:"path"`
		Scope        string `json:"scope"`
		ContextLines int    `json:"contextLines,omitempty"`
	}
	var p payload
	if err := decodePayload(req.Payload, &p); err != nil || strings.TrimSpace(p.Path) == "" {
		_ = r.writeError(conn, req.RequestID, codeInvalidArgument, "invalid git.workingTree.fileDiff payload")
		return
	}
	root, err := r.projectRoot(req.ProjectID)
	if err != nil {
		_ = r.writeError(conn, req.RequestID, codeNotFound, err.Error())
		return
	}
	contextLines := normalizeContextLines(p.ContextLines)
	scope := strings.TrimSpace(p.Scope)
	if scope == "" {
		scope = "unstaged"
	}
	diff, err := gitWorkingTreeDiff(root, p.Path, scope, contextLines)
	if err != nil {
		_ = r.writeError(conn, req.RequestID, codeInternal, err.Error())
		return
	}
	isBinary := strings.Contains(diff, "Binary files")
	_ = r.writeJSON(conn, "->", envelope{
		RequestID: req.RequestID,
		Type:      "response",
		Method:    req.Method,
		ProjectID: req.ProjectID,
		Payload: rp.MustRaw(map[string]any{
			"path":      p.Path,
			"scope":     scope,
			"isBinary":  isBinary,
			"diff":      diff,
			"truncated": false,
		}),
	})
}

func (r *Reporter) projectRoot(projectID string) (string, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return "", fmt.Errorf("projectId is required")
	}
	r.mu.RLock()
	p, ok := r.projectsByID[projectID]
	r.mu.RUnlock()
	if !ok {
		return "", fmt.Errorf("project %q not found", projectID)
	}
	root := strings.TrimSpace(p.Path)
	if root == "" {
		return "", fmt.Errorf("project %q has empty path", projectID)
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve project path: %w", err)
	}
	return abs, nil
}

func (r *Reporter) setProjectLocked(project ProjectInfo) {
	publicID := rp.ProjectID(r.cfg.HubID, project.Name)
	updated := false
	for index := range r.projects {
		if strings.TrimSpace(r.projects[index].Name) != project.Name {
			continue
		}
		r.projects[index] = project
		updated = true
		break
	}
	if !updated {
		r.projects = append(r.projects, project)
	}
	sort.Slice(r.projects, func(i, j int) bool {
		return strings.ToLower(r.projects[i].Name) < strings.ToLower(r.projects[j].Name)
	})
	if r.projectsByID == nil {
		r.projectsByID = make(map[string]ProjectInfo)
	}
	r.projectsByID[publicID] = project
	r.projectsByID[project.Name] = project
}

func (r *Reporter) clearConn(conn *websocket.Conn) {
	r.mu.Lock()
	if r.conn == conn {
		r.conn = nil
		r.connectionEpoch = 0
	}
	for id, ch := range r.pending {
		delete(r.pending, id)
		close(ch)
	}
	r.mu.Unlock()
}

func (r *Reporter) resolvePending(id int64, msg envelope) bool {
	r.mu.Lock()
	ch, ok := r.pending[id]
	if ok {
		delete(r.pending, id)
	}
	r.mu.Unlock()
	if ok {
		ch <- msg
		close(ch)
	}
	return ok
}

func diffProjectDomains(previous, current ProjectInfo) []string {
	domains := make([]string, 0, 3)
	projectChanged := previous.ProjectRev != current.ProjectRev || previous.Agent != current.Agent || previous.IMType != current.IMType || previous.Path != current.Path || previous.Online != current.Online
	gitChanged := previous.Git.GitRev != current.Git.GitRev || previous.Git.HeadSHA != current.Git.HeadSHA || previous.Git.Branch != current.Git.Branch
	worktreeChanged := previous.Git.WorktreeRev != current.Git.WorktreeRev || previous.Git.Dirty != current.Git.Dirty
	if projectChanged {
		domains = append(domains, "project")
	}
	if gitChanged {
		domains = append(domains, "git")
	}
	if worktreeChanged {
		domains = append(domains, "worktree")
	}
	return domains
}

func safeJoin(root, rel string) (string, string, error) {
	rel = strings.TrimSpace(rel)
	if rel == "" {
		rel = "."
	}
	clean := filepath.Clean(rel)
	if filepath.IsAbs(clean) || strings.HasPrefix(clean, "..") || strings.Contains(clean, `:\`) {
		return "", "", fmt.Errorf("path escapes project root")
	}
	target := filepath.Join(root, clean)
	rootAbs, _ := filepath.Abs(root)
	targetAbs, _ := filepath.Abs(target)
	rootPrefix := rootAbs
	if !strings.HasSuffix(rootPrefix, string(filepath.Separator)) {
		rootPrefix += string(filepath.Separator)
	}
	if targetAbs != rootAbs && !strings.HasPrefix(targetAbs, rootPrefix) {
		return "", "", fmt.Errorf("path escapes project root")
	}
	return targetAbs, filepath.ToSlash(clean), nil
}

func decodePayload(raw []byte, out any) error {
	if len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, out)
}

func hashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func resolveDirEntryKind(basePath string, entry os.DirEntry) string {
	if entry.IsDir() {
		return "dir"
	}
	if entry.Type()&os.ModeSymlink != 0 {
		resolvedInfo, err := os.Stat(filepath.Join(basePath, entry.Name()))
		if err == nil && resolvedInfo.IsDir() {
			return "dir"
		}
	}
	return "file"
}

func hashDirectoryEntries(basePath string, entries []os.DirEntry) string {
	lines := make([]string, 0, len(entries))
	for _, entry := range entries {
		kind := resolveDirEntryKind(basePath, entry)
		lines = append(lines, kind+"|"+entry.Name())
	}
	sort.Strings(lines)
	return hashBytes([]byte(strings.Join(lines, "\n")))
}

func detectBinaryAndMime(data []byte) (bool, string) {
	sample := data
	if len(sample) > 512 {
		sample = sample[:512]
	}
	mimeType := http.DetectContentType(sample)
	isBinary := bytes.IndexByte(sample, 0) >= 0
	if strings.HasPrefix(mimeType, "text/") || strings.Contains(mimeType, "json") || strings.Contains(mimeType, "xml") {
		isBinary = false
	}
	return isBinary, mimeType
}

func countLines(data []byte) int {
	if len(data) == 0 {
		return 0
	}
	return bytes.Count(data, []byte{'\n'}) + 1
}

func splitFileLines(data []byte) []string {
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	if text == "" {
		return []string{}
	}
	return strings.Split(text, "\n")
}

func normalizeNeedle(value string, caseSensitive bool) string {
	value = strings.TrimSpace(value)
	if caseSensitive {
		return value
	}
	return strings.ToLower(value)
}

func fuzzyScore(candidate, needle string) float64 {
	if needle == "" || candidate == "" {
		return 0
	}
	if strings.Contains(candidate, needle) {
		return float64(len(needle)) / float64(len(candidate))
	}
	idx := 0
	for _, r := range candidate {
		if idx < len(needle) && byte(r) == needle[idx] {
			idx++
		}
	}
	if idx == 0 {
		return 0
	}
	return float64(idx) / float64(len(candidate)+len(needle))
}

func buildGrepMatcher(pattern string, isRegex bool, caseSensitive bool) (func(string) bool, error) {
	if isRegex {
		expr := pattern
		if !caseSensitive {
			expr = "(?i)" + expr
		}
		re, err := regexp.Compile(expr)
		if err != nil {
			return nil, fmt.Errorf("invalid regex: %w", err)
		}
		return re.MatchString, nil
	}
	needle := normalizeNeedle(pattern, caseSensitive)
	return func(line string) bool {
		return strings.Contains(normalizeNeedle(line, caseSensitive), needle)
	}, nil
}

func normalizeContextLines(contextLines int) int {
	if contextLines < 0 || contextLines > 20 {
		return 3
	}
	return contextLines
}

func parsePorcelainStatus(raw string) ([]map[string]any, []map[string]any, []map[string]any) {
	staged := make([]map[string]any, 0)
	unstaged := make([]map[string]any, 0)
	untracked := make([]map[string]any, 0)
	for _, line := range strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" || len(line) < 3 {
			continue
		}
		x := string(line[0])
		y := string(line[1])
		path := strings.TrimSpace(line[3:])
		if x == "?" && y == "?" {
			untracked = append(untracked, map[string]any{"path": path, "status": "?"})
			continue
		}
		if x != " " {
			staged = append(staged, map[string]any{"path": path, "status": x})
		}
		if y != " " {
			unstaged = append(unstaged, map[string]any{"path": path, "status": y})
		}
	}
	return staged, unstaged, untracked
}

func gitDiffFiles(root, base, head string) ([]map[string]any, error) {
	numstatRaw, err := runGit(root, "diff", "--numstat", fmt.Sprintf("%s..%s", base, head))
	if err != nil {
		return nil, err
	}
	statusRaw, err := runGit(root, "diff", "--name-status", fmt.Sprintf("%s..%s", base, head))
	if err != nil {
		return nil, err
	}
	statusMap := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(statusRaw), "\n") {
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 {
			continue
		}
		statusMap[strings.TrimSpace(parts[1])] = strings.TrimSpace(parts[0])
	}
	files := make([]map[string]any, 0)
	for _, line := range strings.Split(strings.TrimSpace(numstatRaw), "\n") {
		parts := strings.Split(line, "\t")
		if len(parts) < 3 {
			continue
		}
		path := strings.TrimSpace(parts[2])
		additions, _ := strconv.Atoi(strings.ReplaceAll(parts[0], "-", "0"))
		deletions, _ := strconv.Atoi(strings.ReplaceAll(parts[1], "-", "0"))
		status := statusMap[path]
		if status == "" {
			status = "M"
		}
		files = append(files, map[string]any{
			"path":      path,
			"status":    status,
			"additions": additions,
			"deletions": deletions,
		})
	}
	return files, nil
}

func gitWorkingTreeDiff(root, path, scope string, contextLines int) (string, error) {
	switch scope {
	case "staged":
		return runGit(root, "diff", "--cached", "--no-color", fmt.Sprintf("--unified=%d", contextLines), "--", path)
	case "unstaged":
		return runGit(root, "diff", "--no-color", fmt.Sprintf("--unified=%d", contextLines), "--", path)
	case "untracked":
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
		if err != nil {
			return "", err
		}
		return buildUntrackedDiff(path, data), nil
	default:
		return "", fmt.Errorf("unsupported scope %q", scope)
	}
}

func buildUntrackedDiff(path string, data []byte) string {
	lines := splitFileLines(data)
	var b strings.Builder
	b.WriteString("diff --git a/")
	b.WriteString(path)
	b.WriteString(" b/")
	b.WriteString(path)
	b.WriteString("\nnew file mode 100644\n--- /dev/null\n+++ b/")
	b.WriteString(path)
	b.WriteString("\n@@ -0,0 +1,")
	b.WriteString(strconv.Itoa(len(lines)))
	b.WriteString(" @@\n")
	for _, line := range lines {
		b.WriteString("+")
		b.WriteString(line)
		b.WriteString("\n")
	}
	return b.String()
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (r *Reporter) readAck(conn *websocket.Conn) (envelope, error) {
	var resp envelope
	if err := r.readJSON(conn, "<-", &resp); err != nil {
		return envelope{}, err
	}
	if resp.Type == "error" {
		var payload errorPayload
		if err := decodePayload(resp.Payload, &payload); err == nil {
			return resp, fmt.Errorf("%s: %s", payload.Code, payload.Message)
		}
		return resp, fmt.Errorf("registry error")
	}
	return resp, nil
}

func (r *Reporter) writeError(conn *websocket.Conn, requestID int64, code, message string) error {
	return r.writeJSON(conn, "->", envelope{
		RequestID: requestID,
		Type:      "error",
		Payload: rp.MustRaw(errorPayload{
			Code:    code,
			Message: message,
		}),
	})
}

func (r *Reporter) readJSON(conn *websocket.Conn, direction string, out any) error {
	if err := conn.ReadJSON(out); err != nil {
		return err
	}
	r.writeDebugEnvelope(direction, out)
	return nil
}

func (r *Reporter) writeJSON(conn *websocket.Conn, direction string, v any) error {
	r.writeDebugEnvelope(direction, v)
	r.writeMu.Lock()
	defer r.writeMu.Unlock()
	return conn.WriteJSON(v)
}

func (r *Reporter) writeDebugEnvelope(direction string, v any) {
	if r.debugLog == nil {
		return
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return
	}
	if d := strings.TrimSpace(direction); d != "" {
		_, _ = fmt.Fprintf(r.debugLog, "%s[registry] %s\n", d, strings.TrimSpace(string(raw)))
	}
}

func buildWSURL(server string, port int) (string, error) {
	base := strings.TrimSpace(server)
	if base == "" {
		base = "127.0.0.1"
	}
	if strings.HasPrefix(base, "ws://") || strings.HasPrefix(base, "wss://") ||
		strings.HasPrefix(base, "http://") || strings.HasPrefix(base, "https://") {
		u, err := url.Parse(base)
		if err != nil {
			return "", fmt.Errorf("invalid registry server %q: %w", base, err)
		}
		switch u.Scheme {
		case "http":
			u.Scheme = "ws"
		case "https":
			u.Scheme = "wss"
		}
		if u.Path == "" || u.Path == "/" {
			u.Path = "/ws"
		}
		return u.String(), nil
	}
	host := base
	if strings.Contains(base, ":") {
		host = base
	} else {
		host = fmt.Sprintf("%s:%d", base, port)
	}
	return "ws://" + host + "/ws", nil
}

func runGit(root string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), strings.TrimSpace(string(out)))
	}
	return string(out), nil
}
