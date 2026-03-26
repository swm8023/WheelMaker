package hub

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	shared "github.com/swm8023/wheelmaker/internal/shared"
)

const (
	defaultProtocolVersion = shared.DefaultProtocolVersion
	codeInvalidArgument    = shared.CodeInvalidArgument
	codeNotFound           = shared.CodeNotFound
	codeInternal           = shared.CodeInternal
)

type ProjectInfo = shared.ProjectInfo
type envelope = shared.Envelope
type protocolError = shared.ProtocolError
type errorEnvelope = shared.ErrorEnvelope

// ReporterConfig controls hub->registry connection behavior.
type ReporterConfig struct {
	Server            string
	Port              int
	Token             string
	HubID             string
	ReconnectInterval time.Duration
}

// Reporter keeps a long-lived hub connection and serves local project queries.
type Reporter struct {
	cfg          ReporterConfig
	projects     []ProjectInfo
	projectsByID map[string]ProjectInfo
}

// NewReporter creates a Reporter.
func NewReporter(cfg ReporterConfig, projects []ProjectInfo) *Reporter {
	if cfg.Port == 0 {
		cfg.Port = 9630
	}
	if cfg.ReconnectInterval <= 0 {
		cfg.ReconnectInterval = 2 * time.Second
	}
	if strings.TrimSpace(cfg.HubID) == "" {
		cfg.HubID = "wheelmaker-hub"
	}
	cp := make([]ProjectInfo, len(projects))
	copy(cp, projects)
	byID := make(map[string]ProjectInfo, len(cp))
	for _, p := range cp {
		id := strings.TrimSpace(p.ID)
		if id == "" {
			id = strings.TrimSpace(p.Name)
		}
		if id != "" {
			byID[id] = p
		}
	}
	return &Reporter{cfg: cfg, projects: cp, projectsByID: byID}
}

// Run holds a persistent connection; reconnects on failure until ctx cancelled.
func (r *Reporter) Run(ctx context.Context) error {
	for {
		if err := r.runSession(ctx); err != nil && ctx.Err() == nil {
			shared.Warn("registry reporter: session ended: %v", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(r.cfg.ReconnectInterval):
		}
	}
}

func (r *Reporter) runSession(ctx context.Context) error {
	wsURL, err := buildWSURL(r.cfg.Server, r.cfg.Port)
	if err != nil {
		return err
	}
	shared.Info("hub registry: connecting to %s", wsURL)
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		return fmt.Errorf("dial registry %s: %w", wsURL, err)
	}
	defer conn.Close()
	shared.Info("hub registry: connected to %s", wsURL)
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

	for {
		var in envelope
		if err := conn.ReadJSON(&in); err != nil {
			return err
		}
		if in.Type != "request" {
			continue
		}
		switch in.Method {
		case "fs.list":
			r.replyFSList(conn, in)
		case "fs.read":
			r.replyFSRead(conn, in)
		case "git.branches":
			r.replyGitBranches(conn, in)
		case "git.log":
			r.replyGitLog(conn, in)
		case "git.commit.files":
			r.replyGitCommitFiles(conn, in)
		case "git.commit.fileDiff":
			r.replyGitCommitFileDiff(conn, in)
		default:
			_ = conn.WriteJSON(errorEnvelope{
				Version:   defaultProtocolVersion,
				RequestID: in.RequestID,
				Type:      "error",
				Error: protocolError{
					Code:    codeInvalidArgument,
					Message: "unsupported method on hub",
					Details: map[string]any{"method": in.Method},
				},
			})
		}
	}
}

func (r *Reporter) handshake(conn *websocket.Conn) error {
	if err := conn.WriteJSON(envelope{
		Version: defaultProtocolVersion,
		Type:    "request",
		Method:  "hello",
		Payload: shared.MustRaw(map[string]any{
			"clientName":      "wheelmaker-hub",
			"clientVersion":   "0.1.0",
			"protocolVersion": defaultProtocolVersion,
		}),
	}); err != nil {
		return err
	}
	if _, err := readAck(conn); err != nil {
		return fmt.Errorf("hello failed: %w", err)
	}
	shared.Info("hub registry: hello ok")

	if r.cfg.Token != "" {
		if err := conn.WriteJSON(envelope{
			Version: defaultProtocolVersion,
			Type:    "request",
			Method:  "auth",
			Payload: shared.MustRaw(map[string]any{"token": r.cfg.Token}),
		}); err != nil {
			return err
		}
		if _, err := readAck(conn); err != nil {
			return fmt.Errorf("auth failed: %w", err)
		}
		shared.Info("hub registry: auth ok")
	}

	if err := conn.WriteJSON(envelope{
		Version: defaultProtocolVersion,
		Type:    "request",
		Method:  "registry.reportProjects",
		Payload: shared.MustRaw(map[string]any{
			"hubId":    r.cfg.HubID,
			"projects": r.projects,
		}),
	}); err != nil {
		return err
	}
	if _, err := readAck(conn); err != nil {
		return fmt.Errorf("registry.reportProjects failed: %w", err)
	}
	shared.Info("hub registry: reportProjects ok hubId=%s projects=%d", r.cfg.HubID, len(r.projects))
	return nil
}

func (r *Reporter) replyFSList(conn *websocket.Conn, req envelope) {
	type fsListPayload struct {
		Path   string `json:"path,omitempty"`
		Cursor string `json:"cursor,omitempty"`
		Limit  int    `json:"limit,omitempty"`
	}
	var payload fsListPayload
	if err := decodePayload(req.Payload, &payload); err != nil {
		_ = writeError(conn, req.RequestID, codeInvalidArgument, "invalid fs.list payload")
		return
	}
	root, err := r.projectRoot(req.ProjectID)
	if err != nil {
		_ = writeError(conn, req.RequestID, codeNotFound, err.Error())
		return
	}
	target, rel, err := safeJoin(root, payload.Path)
	if err != nil {
		_ = writeError(conn, req.RequestID, codeInvalidArgument, err.Error())
		return
	}
	entries, err := os.ReadDir(target)
	if err != nil {
		_ = writeError(conn, req.RequestID, codeInternal, err.Error())
		return
	}
	sort.Slice(entries, func(i, j int) bool {
		return strings.ToLower(entries[i].Name()) < strings.ToLower(entries[j].Name())
	})

	limit := payload.Limit
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	if len(entries) > limit {
		entries = entries[:limit]
	}
	outEntries := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		info, _ := e.Info()
		kind := "file"
		size := int64(0)
		mtime := ""
		if info != nil {
			size = info.Size()
			mtime = info.ModTime().UTC().Format(time.RFC3339)
		}
		if e.IsDir() {
			kind = "dir"
			size = 0
		}
		childPath := e.Name()
		if rel != "." && rel != "" {
			childPath = filepath.ToSlash(filepath.Join(rel, e.Name()))
		}
		outEntries = append(outEntries, map[string]any{
			"name":  e.Name(),
			"path":  childPath,
			"kind":  kind,
			"size":  size,
			"mtime": mtime,
		})
	}
	_ = conn.WriteJSON(envelope{
		Version:   defaultProtocolVersion,
		RequestID: req.RequestID,
		Type:      "response",
		Method:    req.Method,
		ProjectID: req.ProjectID,
		Payload: shared.MustRaw(map[string]any{
			"path":       rel,
			"entries":    outEntries,
			"nextCursor": "",
		}),
	})
}

func (r *Reporter) replyFSRead(conn *websocket.Conn, req envelope) {
	type fsReadPayload struct {
		Path   string `json:"path"`
		Offset int64  `json:"offset,omitempty"`
		Limit  int64  `json:"limit,omitempty"`
	}
	var payload fsReadPayload
	if err := decodePayload(req.Payload, &payload); err != nil {
		_ = writeError(conn, req.RequestID, codeInvalidArgument, "invalid fs.read payload")
		return
	}
	root, err := r.projectRoot(req.ProjectID)
	if err != nil {
		_ = writeError(conn, req.RequestID, codeNotFound, err.Error())
		return
	}
	target, rel, err := safeJoin(root, payload.Path)
	if err != nil {
		_ = writeError(conn, req.RequestID, codeInvalidArgument, err.Error())
		return
	}
	f, err := os.Open(target)
	if err != nil {
		_ = writeError(conn, req.RequestID, codeInternal, err.Error())
		return
	}
	defer f.Close()

	if payload.Offset < 0 {
		payload.Offset = 0
	}
	if payload.Limit <= 0 || payload.Limit > 1<<20 {
		payload.Limit = 64 * 1024
	}
	if _, err := f.Seek(payload.Offset, io.SeekStart); err != nil {
		_ = writeError(conn, req.RequestID, codeInternal, err.Error())
		return
	}
	buf := make([]byte, payload.Limit)
	n, err := f.Read(buf)
	if err != nil && err != io.EOF {
		_ = writeError(conn, req.RequestID, codeInternal, err.Error())
		return
	}
	eof := err == io.EOF || int64(n) < payload.Limit
	nextOffset := payload.Offset + int64(n)

	_ = conn.WriteJSON(envelope{
		Version:   defaultProtocolVersion,
		RequestID: req.RequestID,
		Type:      "response",
		Method:    req.Method,
		ProjectID: req.ProjectID,
		Payload: shared.MustRaw(map[string]any{
			"path":       rel,
			"content":    string(buf[:n]),
			"encoding":   "utf-8",
			"eof":        eof,
			"nextOffset": nextOffset,
		}),
	})
}

func (r *Reporter) replyGitBranches(conn *websocket.Conn, req envelope) {
	root, err := r.projectRoot(req.ProjectID)
	if err != nil {
		_ = writeError(conn, req.RequestID, codeNotFound, err.Error())
		return
	}
	current, _ := runGit(root, "rev-parse", "--abbrev-ref", "HEAD")
	branchesRaw, err := runGit(root, "branch", "--format=%(refname:short)")
	if err != nil {
		_ = writeError(conn, req.RequestID, codeInternal, err.Error())
		return
	}
	branches := make([]string, 0)
	for _, line := range strings.Split(strings.TrimSpace(branchesRaw), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			branches = append(branches, line)
		}
	}
	_ = conn.WriteJSON(envelope{
		Version:   defaultProtocolVersion,
		RequestID: req.RequestID,
		Type:      "response",
		Method:    req.Method,
		ProjectID: req.ProjectID,
		Payload: shared.MustRaw(map[string]any{
			"current":  strings.TrimSpace(current),
			"branches": branches,
		}),
	})
}

func (r *Reporter) replyGitLog(conn *websocket.Conn, req envelope) {
	type gitLogPayload struct {
		Ref    string `json:"ref,omitempty"`
		Cursor string `json:"cursor,omitempty"`
		Limit  int    `json:"limit,omitempty"`
	}
	var payload gitLogPayload
	if err := decodePayload(req.Payload, &payload); err != nil {
		_ = writeError(conn, req.RequestID, codeInvalidArgument, "invalid git.log payload")
		return
	}
	root, err := r.projectRoot(req.ProjectID)
	if err != nil {
		_ = writeError(conn, req.RequestID, codeNotFound, err.Error())
		return
	}
	ref := strings.TrimSpace(payload.Ref)
	if ref == "" {
		ref = "HEAD"
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
	raw, err := runGit(root, "log", ref, "--date=iso-strict", "--pretty=format:%H%x1f%an%x1f%ae%x1f%aI%x1f%s")
	if err != nil {
		_ = writeError(conn, req.RequestID, codeInternal, err.Error())
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
	_ = conn.WriteJSON(envelope{
		Version:   defaultProtocolVersion,
		RequestID: req.RequestID,
		Type:      "response",
		Method:    req.Method,
		ProjectID: req.ProjectID,
		Payload: shared.MustRaw(map[string]any{
			"ref":        ref,
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
		_ = writeError(conn, req.RequestID, codeInvalidArgument, "invalid git.commit.files payload")
		return
	}
	root, err := r.projectRoot(req.ProjectID)
	if err != nil {
		_ = writeError(conn, req.RequestID, codeNotFound, err.Error())
		return
	}
	numstatRaw, err := runGit(root, "show", "--numstat", "--format=", p.SHA)
	if err != nil {
		_ = writeError(conn, req.RequestID, codeInternal, err.Error())
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
	_ = conn.WriteJSON(envelope{
		Version:   defaultProtocolVersion,
		RequestID: req.RequestID,
		Type:      "response",
		Method:    req.Method,
		ProjectID: req.ProjectID,
		Payload: shared.MustRaw(map[string]any{
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
		_ = writeError(conn, req.RequestID, codeInvalidArgument, "invalid git.commit.fileDiff payload")
		return
	}
	root, err := r.projectRoot(req.ProjectID)
	if err != nil {
		_ = writeError(conn, req.RequestID, codeNotFound, err.Error())
		return
	}
	contextLines := p.ContextLines
	if contextLines < 0 || contextLines > 20 {
		contextLines = 3
	}
	diff, err := runGit(root, "show", "--no-color", fmt.Sprintf("--unified=%d", contextLines), p.SHA, "--", p.Path)
	if err != nil {
		_ = writeError(conn, req.RequestID, codeInternal, err.Error())
		return
	}
	isBinary := strings.Contains(diff, "Binary files")
	_ = conn.WriteJSON(envelope{
		Version:   defaultProtocolVersion,
		RequestID: req.RequestID,
		Type:      "response",
		Method:    req.Method,
		ProjectID: req.ProjectID,
		Payload: shared.MustRaw(map[string]any{
			"sha":       p.SHA,
			"path":      p.Path,
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
	p, ok := r.projectsByID[projectID]
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

func readAck(conn *websocket.Conn) (envelope, error) {
	var resp envelope
	if err := conn.ReadJSON(&resp); err != nil {
		return envelope{}, err
	}
	if resp.Type == "error" && resp.Error != nil {
		return resp, fmt.Errorf("%s: %s", resp.Error.Code, resp.Error.Message)
	}
	return resp, nil
}

func writeError(conn *websocket.Conn, requestID, code, message string) error {
	return conn.WriteJSON(errorEnvelope{
		Version:   defaultProtocolVersion,
		RequestID: requestID,
		Type:      "error",
		Error: protocolError{
			Code:    code,
			Message: message,
		},
	})
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
