package agentv2

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

const (
	jsonrpcVersion     = "2.0"
	maxScannerBuf      = 1 << 20
	codeMethodNotFound = -32601
	codeInternalError  = -32603
)

// InMemoryServer runs a JSON-RPC compatible server in-process.
type InMemoryServer func(r io.Reader, w io.Writer)

type processRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int64  `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type processResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *rpcError) Error() string {
	if e == nil {
		return ""
	}
	if len(e.Data) > 0 {
		var d struct {
			Message string `json:"message"`
		}
		if json.Unmarshal(e.Data, &d) == nil && d.Message != "" {
			return d.Message
		}
		return fmt.Sprintf("%s: %s", e.Message, e.Data)
	}
	return e.Message
}

type rawMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int64          `json:"id"`
	Method  string          `json:"method"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// ProcessConn is a transport-only JSON-RPC connection over subprocess stdio.
type ProcessConn struct {
	exePath string
	exeArgs []string
	env     []string

	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser

	enc *json.Encoder
	mu  sync.Mutex

	nextID  atomic.Int64
	pending map[int64]chan processResponse

	reqMu      sync.RWMutex
	reqHandler RequestHandler

	debugMu  sync.RWMutex
	debugLog io.Writer

	connCtx    context.Context
	connCancel context.CancelFunc
	done       chan struct{}

	inMemoryServer InMemoryServer
}

var _ Conn = (*ProcessConn)(nil)

// NewProcessConn creates a subprocess-backed connection.
func NewProcessConn(exePath string, env []string, args ...string) *ProcessConn {
	ctx, cancel := context.WithCancel(context.Background())
	return &ProcessConn{
		exePath:    exePath,
		exeArgs:    append([]string(nil), args...),
		env:        env,
		pending:    make(map[int64]chan processResponse),
		connCtx:    ctx,
		connCancel: cancel,
		done:       make(chan struct{}),
	}
}

// NewInMemoryProcessConn creates a connection backed by an in-memory server.
func NewInMemoryProcessConn(server InMemoryServer) *ProcessConn {
	ctx, cancel := context.WithCancel(context.Background())
	return &ProcessConn{
		pending:        make(map[int64]chan processResponse),
		connCtx:        ctx,
		connCancel:     cancel,
		done:           make(chan struct{}),
		inMemoryServer: server,
	}
}

// SetDebugLogger sets a writer for transport debug logs.
func (c *ProcessConn) SetDebugLogger(w io.Writer) {
	c.debugMu.Lock()
	c.debugLog = w
	c.debugMu.Unlock()
}

// Start starts the underlying transport.
func (c *ProcessConn) Start() error {
	if c.inMemoryServer != nil {
		return c.startInMemory()
	}
	return c.startProcess()
}

func (c *ProcessConn) startInMemory() error {
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

func (c *ProcessConn) startProcess() error {
	cmd := exec.Command(c.exePath, c.exeArgs...)
	cmd.Env = append(cmd.Environ(), c.env...)
	cmd.Stderr = log.Writer()

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("agentv2 conn: stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("agentv2 conn: stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("agentv2 conn: start process: %w", err)
	}

	c.cmd = cmd
	c.stdin = stdin
	c.stdout = stdout
	c.enc = json.NewEncoder(stdin)
	go c.readLoop(stdout)
	return nil
}

func (c *ProcessConn) Send(ctx context.Context, method string, params any, result any) error {
	id := c.nextID.Add(1)
	ch := make(chan processResponse, 1)
	c.setPending(id, ch)

	req := processRequest{
		JSONRPC: jsonrpcVersion,
		ID:      id,
		Method:  method,
		Params:  params,
	}
	if err := c.encodeLocked(req); err != nil {
		c.removePending(id)
		return fmt.Errorf("agentv2 conn: encode request: %w", err)
	}
	c.writeDebugJSON("->", req)

	select {
	case <-ctx.Done():
		c.removePending(id)
		return ctx.Err()
	case resp := <-ch:
		if resp.Error != nil {
			return fmt.Errorf("agentv2 rpc error %d: %s", resp.Error.Code, resp.Error.Error())
		}
		if result != nil && len(resp.Result) > 0 {
			if err := json.Unmarshal(resp.Result, result); err != nil {
				return fmt.Errorf("agentv2 conn: unmarshal result: %w", err)
			}
		}
		return nil
	case <-c.done:
		return fmt.Errorf("agentv2 conn: connection closed")
	}
}

func (c *ProcessConn) Notify(method string, params any) error {
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
		return fmt.Errorf("agentv2 conn: encode notification: %w", err)
	}
	c.writeDebugJSON("->", n)
	return nil
}

func (c *ProcessConn) OnRequest(h RequestHandler) {
	c.reqMu.Lock()
	c.reqHandler = h
	c.reqMu.Unlock()
}

func (c *ProcessConn) Close() error {
	select {
	case <-c.done:
		return nil
	default:
	}
	close(c.done)
	c.connCancel()
	if c.stdin != nil {
		_ = c.stdin.Close()
	}
	if c.cmd != nil {
		if c.cmd.Process != nil {
			_ = c.cmd.Process.Kill()
		}
		_ = c.cmd.Wait()
	}
	return nil
}

func (c *ProcessConn) setPending(id int64, ch chan processResponse) {
	c.mu.Lock()
	c.pending[id] = ch
	c.mu.Unlock()
}

func (c *ProcessConn) removePending(id int64) {
	c.mu.Lock()
	delete(c.pending, id)
	c.mu.Unlock()
}

func (c *ProcessConn) popPending(id int64) (chan processResponse, bool) {
	c.mu.Lock()
	ch, ok := c.pending[id]
	if ok {
		delete(c.pending, id)
	}
	c.mu.Unlock()
	return ch, ok
}

func (c *ProcessConn) failAllPending(err *rpcError) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for id, ch := range c.pending {
		ch <- processResponse{ID: id, Error: err}
		delete(c.pending, id)
	}
}

func (c *ProcessConn) encodeLocked(v any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.enc == nil {
		return fmt.Errorf("encoder is not ready")
	}
	return c.enc.Encode(v)
}

func (c *ProcessConn) readLoop(r io.Reader) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, maxScannerBuf), maxScannerBuf)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		c.writeDebugRaw("<-", line)

		var raw rawMessage
		if err := json.Unmarshal(line, &raw); err != nil {
			continue
		}

		switch {
		case raw.ID != nil && raw.Method != "":
			go c.handleIncomingRequest(*raw.ID, raw.Method, raw.Params)
		case raw.ID != nil:
			resp := processResponse{
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
			c.reqMu.RLock()
			h := c.reqHandler
			c.reqMu.RUnlock()
			if h != nil {
				_, _ = h(c.connCtx, raw.Method, raw.Params, true)
			}
		}
	}

	c.failAllPending(&rpcError{Code: -1, Message: "agent process exited"})
}

func (c *ProcessConn) handleIncomingRequest(id int64, method string, params json.RawMessage) {
	c.reqMu.RLock()
	handler := c.reqHandler
	c.reqMu.RUnlock()

	type rpcResp struct {
		JSONRPC string    `json:"jsonrpc"`
		ID      int64     `json:"id"`
		Result  any       `json:"result,omitempty"`
		Error   *rpcError `json:"error,omitempty"`
	}

	resp := rpcResp{JSONRPC: jsonrpcVersion, ID: id}
	if handler == nil {
		resp.Error = &rpcError{Code: codeMethodNotFound, Message: fmt.Sprintf("method not found: %s", method)}
	} else {
		result, err := handler(c.connCtx, method, params, false)
		if err != nil {
			resp.Error = &rpcError{Code: codeInternalError, Message: err.Error()}
		} else if result == nil {
			resp.Result = json.RawMessage("null")
		} else {
			resp.Result = result
		}
	}

	_ = c.encodeLocked(resp)
}

func (c *ProcessConn) debugWriter() io.Writer {
	c.debugMu.RLock()
	dw := c.debugLog
	c.debugMu.RUnlock()
	return dw
}

func (c *ProcessConn) writeDebugJSON(prefix string, payload any) {
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

func (c *ProcessConn) writeDebugRaw(prefix string, raw []byte) {
	dw := c.debugWriter()
	if dw == nil || len(raw) == 0 {
		return
	}
	writeDebugLine(dw, prefix, raw)
}

func writeDebugLine(w io.Writer, prefix string, raw []byte) {
	if p := strings.TrimSpace(prefix); p != "" {
		_, _ = fmt.Fprintf(w, "%s[agentv2] %s\n", p, strings.TrimSpace(string(raw)))
	}
}
