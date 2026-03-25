package acp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
)

// RequestHandler is called for each inbound message from the agent.
// noResponse is true for notifications (no JSON-RPC id); return values are ignored in that case.
// For requests (noResponse=false), the result is JSON-encoded and sent as the response.
type RequestHandler func(ctx context.Context, method string, params json.RawMessage, noResponse bool) (any, error)

// InMemoryServer runs an ACP-compatible JSON-RPC server in-process.
// It reads request lines from r and writes response/notification lines to w.
type InMemoryServer func(r io.Reader, w io.Writer)

// Conn manages a single ACP-compatible subprocess and communicates
// with it over stdin/stdout using JSON-RPC 2.0.
//
// The ACP protocol is bidirectional:
//   - Conn→Agent requests: SendAgent() — we initiate, agent responds.
//   - Agent→Conn requests: OnRequest() handler — agent initiates, we respond.
//   - Notifications (either direction, no response): Subscribe() / NotifyAgent().
type Conn struct {
	exePath string
	exeArgs []string
	env     []string // additional environment variables

	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser

	enc *json.Encoder
	mu  sync.Mutex // protects enc and pending

	nextID  atomic.Int64
	pending map[int64]chan Response

	reqMu      sync.RWMutex
	reqHandler RequestHandler

	debugMu  sync.RWMutex
	debugLog io.Writer // nil = no debug logging; set via SetDebugLogger

	// connCtx is cancelled when Close() is called, providing a cancellation
	// signal for in-flight Agent→Conn request handlers.
	connCtx    context.Context
	connCancel context.CancelFunc

	done chan struct{}

	inMemoryServer InMemoryServer

	// transport is set when the connection is backed by an io.ReadWriteCloser
	// (e.g. a TCP net.Conn) instead of a subprocess's stdio.
	transport io.ReadWriteCloser
}

func (c *Conn) debugWriter() io.Writer {
	c.debugMu.RLock()
	dw := c.debugLog
	c.debugMu.RUnlock()
	return dw
}

func (c *Conn) writeDebugJSON(prefix string, payload any) {
	dw := c.debugWriter()
	if dw == nil {
		return
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return
	}
	writeDebugLine(dw, prefix, raw)
}

func (c *Conn) writeDebugRaw(prefix string, raw []byte) {
	dw := c.debugWriter()
	if dw == nil || len(raw) == 0 {
		return
	}
	writeDebugLine(dw, prefix, trimDebugPayload(raw))
}

// trimDebugPayload returns a shorter representation of ACP messages for debug logs.
// session/update messages are reformatted to a single-line summary; all other messages
// are passed through unchanged.
func trimDebugPayload(raw []byte) []byte {
	// Only reformat inbound session/update notifications to avoid log spam.
	// Use a minimal parse: check method field cheaply before full unmarshal.
	var msg struct {
		Method string          `json:"method"`
		ID     *int64          `json:"id"`
		Params json.RawMessage `json:"params,omitempty"`
	}
	if err := json.Unmarshal(raw, &msg); err != nil {
		return raw
	}
	if msg.ID != nil || msg.Method != "session/update" {
		return raw // requests/responses pass through unchanged
	}

	var params struct {
		SessionID string `json:"sessionId"`
		Update    struct {
			SessionUpdate string          `json:"sessionUpdate"`
			ToolCallID    string          `json:"toolCallId,omitempty"`
			Title         string          `json:"title,omitempty"`
			Status        string          `json:"status,omitempty"`
			Content       json.RawMessage `json:"content,omitempty"`
			RawOutput     json.RawMessage `json:"rawOutput,omitempty"`
		} `json:"update"`
	}
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return raw
	}

	u := params.Update
	var summary string
	switch u.SessionUpdate {
	case "agent_message_chunk":
		text := extractTextFromContent(u.Content)
		summary = fmt.Sprintf("agent_message_chunk: %s", previewStr(text, 80))
	case "user_message_chunk":
		text := extractTextFromContent(u.Content)
		summary = fmt.Sprintf("user_message_chunk: %s", previewStr(text, 80))
	case "tool_call":
		summary = fmt.Sprintf("tool_call id=%s title=%s status=%s", u.ToolCallID, previewStr(u.Title, 60), u.Status)
	case "tool_call_update":
		out := previewStr(strings.TrimSpace(rawToString(u.RawOutput)), 100)
		summary = fmt.Sprintf("tool_call_update id=%s status=%s output=%s", u.ToolCallID, u.Status, out)
	default:
		// For less-common updates, keep a compact form.
		compact, err := json.Marshal(params.Update)
		if err != nil {
			return raw
		}
		summary = string(compact)
		if len(summary) > 200 {
			summary = summary[:200] + "…"
		}
	}

	sidShort := params.SessionID
	if len(sidShort) > 8 {
		sidShort = sidShort[:8]
	}
	out := fmt.Sprintf(`{"method":"session/update","sid":"%s","update":%q}`, sidShort, summary)
	return []byte(out)
}

func extractTextFromContent(content json.RawMessage) string {
	if len(content) == 0 {
		return ""
	}
	var cb struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(content, &cb); err == nil && cb.Type == "text" {
		return cb.Text
	}
	return string(content)
}

func rawToString(r json.RawMessage) string {
	if len(r) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(r, &s); err == nil {
		return s
	}
	return string(r)
}

func previewStr(s string, max int) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}

func sanitizeDebugJSON(raw []byte) string {
	s := strings.TrimSpace(string(raw))
	// Remove "jsonrpc":"2.0" field (always the first field in our messages)
	s = strings.Replace(s, `"jsonrpc":"2.0",`, "", 1)
	// Shorten full UUIDs (8-4-4-4-12 lowercase hex) to their first 8 characters
	return shortenUUIDs(s)
}

// shortenUUIDs replaces every lowercase UUID in s with just its first 8 hex chars.
// UUID format: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx (36 bytes, all lowercase hex)
func shortenUUIDs(s string) string {
	const uuidLen = 36
	if len(s) < uuidLen {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	i := 0
	for i <= len(s)-uuidLen {
		if looksLikeUUID(s[i : i+uuidLen]) {
			b.WriteString(s[i : i+8])
			i += uuidLen
		} else {
			b.WriteByte(s[i])
			i++
		}
	}
	b.WriteString(s[i:])
	return b.String()
}

func looksLikeUUID(s string) bool {
	if s[8] != '-' || s[13] != '-' || s[18] != '-' || s[23] != '-' {
		return false
	}
	return isHexSegment(s[:8]) && isHexSegment(s[9:13]) &&
		isHexSegment(s[14:18]) && isHexSegment(s[19:23]) && isHexSegment(s[24:])
}

func isHexSegment(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}

func writeDebugLine(w io.Writer, prefix string, raw []byte) {
	if p := strings.TrimSpace(prefix); p != "" {
		_, _ = fmt.Fprintf(w, "%s[acp] %s\n", p, sanitizeDebugJSON(raw))
	}
}

func (c *Conn) setPending(id int64, ch chan Response) {
	c.mu.Lock()
	c.pending[id] = ch
	c.mu.Unlock()
}

func (c *Conn) removePending(id int64) {
	c.mu.Lock()
	delete(c.pending, id)
	c.mu.Unlock()
}

func (c *Conn) popPending(id int64) (chan Response, bool) {
	c.mu.Lock()
	ch, ok := c.pending[id]
	if ok {
		delete(c.pending, id)
	}
	c.mu.Unlock()
	return ch, ok
}

func (c *Conn) encodeLocked(v any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.enc.Encode(v)
}

func (c *Conn) failAllPending(err *RPCError) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for id, ch := range c.pending {
		ch <- Response{ID: id, Error: err}
		delete(c.pending, id)
	}
}

// NewConn creates a new Conn for the given binary.
// env is a list of "KEY=VALUE" strings appended to the process environment.
func NewConn(exePath string, env []string, args ...string) *Conn {
	ctx, cancel := context.WithCancel(context.Background())
	return &Conn{
		exePath:    exePath,
		exeArgs:    append([]string(nil), args...),
		env:        env,
		pending:    make(map[int64]chan Response),
		connCtx:    ctx,
		connCancel: cancel,
		done:       make(chan struct{}),
	}
}

// NewConnWithTransport creates a Conn backed by an io.ReadWriteCloser transport
// (e.g. a TCP net.Conn) instead of a subprocess's stdio.
// If cmd is non-nil, its process is killed when Close() is called.
// Start() must be called before using the conn.
func NewConnWithTransport(cmd *exec.Cmd, transport io.ReadWriteCloser) *Conn {
	ctx, cancel := context.WithCancel(context.Background())
	return &Conn{
		cmd:        cmd,
		transport:  transport,
		pending:    make(map[int64]chan Response),
		connCtx:    ctx,
		connCancel: cancel,
		done:       make(chan struct{}),
	}
}

// NewInMemoryConn creates a connection backed by an in-process ACP server.
func NewInMemoryConn(server InMemoryServer) *Conn {
	ctx, cancel := context.WithCancel(context.Background())
	return &Conn{
		pending:        make(map[int64]chan Response),
		connCtx:        ctx,
		connCancel:     cancel,
		done:           make(chan struct{}),
		inMemoryServer: server,
	}
}

// SetDebugLogger sets an optional writer for debug logging of all ACP JSON
// messages. When non-nil, outgoing messages are prefixed with "→ " and
// incoming messages with "← ". Set to nil to disable.
// Safe to call at any time; uses a separate RWMutex to avoid log contention.
func (c *Conn) SetDebugLogger(w io.Writer) {
	c.debugMu.Lock()
	c.debugLog = w
	c.debugMu.Unlock()
}

// OnRequest registers the handler for Agent→Conn requests.
// Replaces any previously set handler.
//
// The handler is responsible for implementing:
//   - session/request_permission
//   - fs/read_text_file, fs/write_text_file
//   - terminal/create, terminal/output, terminal/wait_for_exit, terminal/kill, terminal/release
func (c *Conn) OnRequest(h RequestHandler) {
	c.reqMu.Lock()
	c.reqHandler = h
	c.reqMu.Unlock()
}

// Start launches the agent subprocess (or activates the transport) and begins the read loop.
// stderr of the subprocess is forwarded to the application log via log.Writer().
func (c *Conn) Start() error {
	if c.transport != nil {
		return c.startTransport()
	}
	if c.inMemoryServer != nil {
		return c.startInMemory()
	}
	return c.startProcess()
}

func (c *Conn) startTransport() error {
	// io.ReadWriteCloser satisfies both io.WriteCloser and io.ReadCloser.
	c.stdin = c.transport
	c.stdout = c.transport
	c.enc = json.NewEncoder(c.transport)
	go c.readLoop(c.transport)
	return nil
}

func (c *Conn) startInMemory() error {
	clientToServerR, clientToServerW := io.Pipe()
	serverToClientR, serverToClientW := io.Pipe()

	c.stdin = clientToServerW
	c.stdout = serverToClientR
	c.enc = json.NewEncoder(c.stdin)

	go c.readLoop(c.stdout)
	go func() {
		defer serverToClientW.Close()
		c.inMemoryServer(clientToServerR, serverToClientW)
	}()
	return nil
}

func (c *Conn) startProcess() error {
	cmd := exec.Command(c.exePath, c.exeArgs...)
	cmd.Env = append(cmd.Environ(), c.env...)
	cmd.Stderr = log.Writer() // forward subprocess stderr to the application log

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("acp: stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("acp: stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("acp: start process: %w", err)
	}

	c.cmd = cmd
	c.stdin = stdin
	c.stdout = stdout
	c.enc = json.NewEncoder(stdin)

	go c.readLoop(stdout)
	return nil
}

// SendAgent sends a JSON-RPC request to the agent and waits for the matching response.
// result must be a pointer; on success it is populated by json.Unmarshal.
func (c *Conn) SendAgent(ctx context.Context, method string, params any, result any) error {
	id := c.nextID.Add(1)

	ch := make(chan Response, 1)
	c.setPending(id, ch)

	req := Request{
		JSONRPC: jsonrpcVersion,
		ID:      id,
		Method:  method,
		Params:  params,
	}

	if err := c.encodeLocked(req); err != nil {
		c.removePending(id)
		return fmt.Errorf("acp: encode request: %w", err)
	}

	c.writeDebugJSON("->", req)

	select {
	case <-ctx.Done():
		c.removePending(id)
		return ctx.Err()
	case resp := <-ch:
		if resp.Error != nil {
			return fmt.Errorf("acp rpc error %d: %s", resp.Error.Code, resp.Error.Error())
		}
		if result != nil && len(resp.Result) > 0 {
			if err := json.Unmarshal(resp.Result, result); err != nil {
				return fmt.Errorf("acp: unmarshal result: %w", err)
			}
		}
		return nil
	case <-c.done:
		return fmt.Errorf("acp: connection closed")
	}
}

// NotifyAgent sends a JSON-RPC notification to the agent (no id, no response expected).
func (c *Conn) NotifyAgent(method string, params any) error {
	n := struct {
		JSONRPC string `json:"jsonrpc"`
		Method  string `json:"method"`
		Params  any    `json:"params,omitempty"`
	}{
		JSONRPC: jsonrpcVersion,
		Method:  method,
		Params:  params,
	}
	if err := c.encodeLocked(n); err != nil {
		return fmt.Errorf("acp: encode notification: %w", err)
	}

	c.writeDebugJSON("->", n)
	return nil
}

// Close shuts down the connection: cancels in-flight callbacks, kills the subprocess,
// and waits for it to exit.
func (c *Conn) Close() error {
	select {
	case <-c.done:
		return nil // already closed
	default:
	}
	close(c.done)
	c.connCancel()
	_ = c.stdin.Close()
	if c.cmd != nil {
		// Kill the process explicitly so Close() never blocks waiting for a subprocess
		// that ignores stdin EOF (e.g. Node.js-based agents).
		if c.cmd.Process != nil {
			_ = c.cmd.Process.Kill()
		}
		_ = c.cmd.Wait()
	}
	return nil
}

// readLoop reads newline-delimited JSON from stdout and dispatches each message.
//
// ACP is bidirectional JSON-RPC 2.0. The three message types are:
//
//	Response:     id != nil, method == ""  → route to pending[id]
//	Request:      id != nil, method != ""  → call reqHandler, send response
//	Notification: id == nil,  method != ""  → dispatch to subscribers
func (c *Conn) readLoop(r io.Reader) {
	scanner := bufio.NewScanner(r)
	// Increase buffer for large messages (e.g. file contents in tool calls).
	scanner.Buffer(make([]byte, maxScannerBuf), maxScannerBuf)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		c.writeDebugRaw("<-", line)

		var raw rawMessage
		if err := json.Unmarshal(line, &raw); err != nil {
			continue // ignore malformed lines
		}

		switch {
		case raw.ID != nil && raw.Method != "":
			// Agent→Conn request: agent wants us to do something and expects a response.
			go c.handleIncomingRequest(*raw.ID, raw.Method, raw.Params)

		case raw.ID != nil:
			// Response to one of our Conn→Agent requests.
			resp := Response{
				JSONRPC: raw.JSONRPC,
				ID:      *raw.ID,
				Result:  raw.Result,
				Error:   raw.Error,
			}
			ch, ok := c.popPending(resp.ID)
			if ok {
				ch <- resp
			}

		case raw.Method != "":
			// Notification (no id, no response expected). Deliver synchronously so that
			// all notifications preceding a response are processed before conn.SendAgent returns.
			c.reqMu.RLock()
			h := c.reqHandler
			c.reqMu.RUnlock()
			if h != nil {
				h(c.connCtx, raw.Method, raw.Params, true) // noResponse=true; return values ignored
			}
		}
	}

	// stdout closed — unblock all pending requests.
	c.failAllPending(&RPCError{Code: -1, Message: "agent process exited"})
}

// handleIncomingRequest processes an Agent→Conn request and sends the response.
// Uses connCtx so that callbacks are cancelled when the connection is closed.
func (c *Conn) handleIncomingRequest(id int64, method string, params json.RawMessage) {
	c.reqMu.RLock()
	handler := c.reqHandler
	c.reqMu.RUnlock()

	type rpcResp struct {
		JSONRPC string    `json:"jsonrpc"`
		ID      int64     `json:"id"`
		Result  any       `json:"result,omitempty"`
		Error   *RPCError `json:"error,omitempty"`
	}

	resp := rpcResp{JSONRPC: jsonrpcVersion, ID: id}

	if handler == nil {
		resp.Error = &RPCError{Code: CodeMethodNotFound, Message: fmt.Sprintf("method not found: %s", method)}
	} else {
		result, err := handler(c.connCtx, method, params, false)
		if err != nil {
			resp.Error = &RPCError{Code: CodeInternalError, Message: err.Error()}
		} else if result == nil {
			// Per JSON-RPC 2.0: result member MUST be present on success.
			// Use explicit null rather than omitting the field.
			resp.Result = json.RawMessage("null")
		} else {
			resp.Result = result
		}
	}

	_ = c.encodeLocked(resp)
}
