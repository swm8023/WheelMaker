package agentv2

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

	"github.com/swm8023/wheelmaker/internal/protocol"
)

// RequestHandler handles inbound ACP requests/notifications.
type RequestHandler func(ctx context.Context, method string, params json.RawMessage, noResponse bool) (any, error)

// Conn is a transport-only ACP connection contract.
//
// Conn must only provide raw request/response transport operations and
// request callback registration. Protocol-specific business dispatch belongs
// to Instance callbacks.
type Conn interface {
	Send(ctx context.Context, method string, params any, result any) error
	Notify(method string, params any) error
	OnRequest(h RequestHandler)
	Close() error
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
	pending map[int64]chan protocol.ACPRPCResponse

	reqMu      sync.RWMutex
	reqHandler RequestHandler

	connCtx    context.Context
	connCancel context.CancelFunc
	done       chan struct{}
}

var _ Conn = (*ProcessConn)(nil)

// NewProcessConn creates a subprocess-backed connection.
func NewProcessConn(exePath string, env []string, args ...string) *ProcessConn {
	ctx, cancel := context.WithCancel(context.Background())
	return &ProcessConn{
		exePath:    exePath,
		exeArgs:    append([]string(nil), args...),
		env:        env,
		pending:    make(map[int64]chan protocol.ACPRPCResponse),
		connCtx:    ctx,
		connCancel: cancel,
		done:       make(chan struct{}),
	}
}

// Start starts the subprocess transport.
func (c *ProcessConn) Start() error {
	return c.startProcess()
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
	ch := make(chan protocol.ACPRPCResponse, 1)
	c.setPending(id, ch)

	req := protocol.ACPRPCRequest{
		JSONRPC: protocol.ACPRPCVersion,
		ID:      id,
		Method:  method,
		Params:  params,
	}
	if err := c.encodeLocked(req); err != nil {
		c.removePending(id)
		return fmt.Errorf("agentv2 conn: encode request: %w", err)
	}

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
	n := protocol.ACPRPCNotification{
		JSONRPC: protocol.ACPRPCVersion,
		Method:  method,
		Params:  params,
	}
	if err := c.encodeLocked(n); err != nil {
		return fmt.Errorf("agentv2 conn: encode notification: %w", err)
	}
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

func (c *ProcessConn) setPending(id int64, ch chan protocol.ACPRPCResponse) {
	c.mu.Lock()
	c.pending[id] = ch
	c.mu.Unlock()
}

func (c *ProcessConn) removePending(id int64) {
	c.mu.Lock()
	delete(c.pending, id)
	c.mu.Unlock()
}

func (c *ProcessConn) popPending(id int64) (chan protocol.ACPRPCResponse, bool) {
	c.mu.Lock()
	ch, ok := c.pending[id]
	if ok {
		delete(c.pending, id)
	}
	c.mu.Unlock()
	return ch, ok
}

func (c *ProcessConn) failAllPending(err *protocol.ACPRPCError) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for id, ch := range c.pending {
		ch <- protocol.ACPRPCResponse{ID: id, Error: err}
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
	scanner.Buffer(make([]byte, protocol.ACPRPCMaxScannerBuf), protocol.ACPRPCMaxScannerBuf)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var raw protocol.ACPRPCRawMessage
		if err := json.Unmarshal(line, &raw); err != nil {
			continue
		}

		switch {
		case raw.ID != nil && raw.Method != "":
			go c.handleIncomingRequest(*raw.ID, raw.Method, raw.Params)
		case raw.ID != nil:
			resp := protocol.ACPRPCResponse{
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

	c.failAllPending(&protocol.ACPRPCError{Code: -1, Message: "agent process exited"})
}

func (c *ProcessConn) handleIncomingRequest(id int64, method string, params json.RawMessage) {
	c.reqMu.RLock()
	handler := c.reqHandler
	c.reqMu.RUnlock()

	type rpcResp struct {
		JSONRPC string                `json:"jsonrpc"`
		ID      int64                 `json:"id"`
		Result  any                   `json:"result,omitempty"`
		Error   *protocol.ACPRPCError `json:"error,omitempty"`
	}

	resp := rpcResp{JSONRPC: protocol.ACPRPCVersion, ID: id}
	if handler == nil {
		resp.Error = &protocol.ACPRPCError{Code: protocol.ACPRPCCodeMethodNotFound, Message: fmt.Sprintf("method not found: %s", method)}
	} else {
		result, err := handler(c.connCtx, method, params, false)
		if err != nil {
			resp.Error = &protocol.ACPRPCError{Code: protocol.ACPRPCCodeInternalError, Message: err.Error()}
		} else if result == nil {
			resp.Result = json.RawMessage("null")
		} else {
			resp.Result = result
		}
	}

	_ = c.encodeLocked(resp)
}
