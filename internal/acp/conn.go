package acp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os/exec"
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
//   - Conn→Agent requests: Send() — we initiate, agent responds.
//   - Agent→Conn requests: OnRequest() handler — agent initiates, we respond.
//   - Notifications (either direction, no response): Subscribe() / Notify().
type Conn struct {
	exePath string
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
}

// NewConn creates a new Conn for the given binary.
// env is a list of "KEY=VALUE" strings appended to the process environment.
func NewConn(exePath string, env []string) *Conn {
	ctx, cancel := context.WithCancel(context.Background())
	return &Conn{
		exePath:    exePath,
		env:        env,
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

// Start launches the agent subprocess and begins the read loop.
// stderr of the subprocess is forwarded to the application log via log.Writer().
func (c *Conn) Start() error {
	if c.inMemoryServer != nil {
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

	cmd := exec.Command(c.exePath)
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

// Send sends a JSON-RPC request and waits for the matching response.
// result must be a pointer; on success it is populated by json.Unmarshal.
func (c *Conn) Send(ctx context.Context, method string, params any, result any) error {
	id := c.nextID.Add(1)

	ch := make(chan Response, 1)
	c.mu.Lock()
	c.pending[id] = ch
	c.mu.Unlock()

	req := Request{
		JSONRPC: jsonrpcVersion,
		ID:      id,
		Method:  method,
		Params:  params,
	}

	c.mu.Lock()
	err := c.enc.Encode(req)
	c.mu.Unlock()
	if err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return fmt.Errorf("acp: encode request: %w", err)
	}

	c.debugMu.RLock()
	dw := c.debugLog
	c.debugMu.RUnlock()
	if dw != nil {
		if raw, e := json.Marshal(req); e == nil {
			fmt.Fprintf(dw, "→ %s\n", raw)
		}
	}

	select {
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
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

// Notify sends a JSON-RPC notification (no id, no response expected).
func (c *Conn) Notify(method string, params any) error {
	n := struct {
		JSONRPC string `json:"jsonrpc"`
		Method  string `json:"method"`
		Params  any    `json:"params,omitempty"`
	}{
		JSONRPC: jsonrpcVersion,
		Method:  method,
		Params:  params,
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.enc.Encode(n); err != nil {
		return fmt.Errorf("acp: encode notification: %w", err)
	}
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
	scanner.Buffer(make([]byte, 1<<20), 1<<20)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		c.debugMu.RLock()
		dw := c.debugLog
		c.debugMu.RUnlock()
		if dw != nil {
			fmt.Fprintf(dw, "← %s\n", line)
		}

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
			c.mu.Lock()
			ch, ok := c.pending[resp.ID]
			if ok {
				delete(c.pending, resp.ID)
			}
			c.mu.Unlock()
			if ok {
				ch <- resp
			}

		case raw.Method != "":
			// Notification (no id, no response expected). Deliver synchronously so that
			// all notifications preceding a response are processed before conn.Send returns.
			c.reqMu.RLock()
			h := c.reqHandler
			c.reqMu.RUnlock()
			if h != nil {
				h(c.connCtx, raw.Method, raw.Params, true) // noResponse=true; return values ignored
			}
		}
	}

	// stdout closed — unblock all pending requests.
	c.mu.Lock()
	for id, ch := range c.pending {
		ch <- Response{ID: id, Error: &RPCError{Code: -1, Message: "agent process exited"}}
		delete(c.pending, id)
	}
	c.mu.Unlock()
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
		resp.Error = &RPCError{Code: -32601, Message: fmt.Sprintf("method not found: %s", method)}
	} else {
		result, err := handler(c.connCtx, method, params, false)
		if err != nil {
			resp.Error = &RPCError{Code: -32603, Message: err.Error()}
		} else if result == nil {
			// Per JSON-RPC 2.0: result member MUST be present on success.
			// Use explicit null rather than omitting the field.
			resp.Result = json.RawMessage("null")
		} else {
			resp.Result = result
		}
	}

	c.mu.Lock()
	_ = c.enc.Encode(resp)
	c.mu.Unlock()
}
