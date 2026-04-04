package agentv2

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/swm8023/wheelmaker/internal/protocol"
)

// sessionBinder allows Conn implementations to bind ACP session IDs for inbound routing.
type sessionBinder interface {
	BindSessionID(acpSessionID string)
}

type ownedTransport interface {
	SendMessage(v any) error
	OnMessage(h func(json.RawMessage))
	Done() <-chan struct{}
	Close() error
}

// ownedConn maps one instance to one underlying transport and owns JSON-RPC request/response matching.
type ownedConn struct {
	transport ownedTransport

	nextID atomic.Int64

	mu      sync.RWMutex
	handler RequestHandler
	closed  bool

	pendingMu sync.Mutex
	pending   map[int64]chan protocol.ACPRPCResponse

	connCtx    context.Context
	connCancel context.CancelFunc
}

var _ Conn = (*ownedConn)(nil)

// NewOwnedConn creates a per-instance owned conn from a raw ACP process transport.
func NewOwnedConn(transport ownedTransport) Conn {
	ctx, cancel := context.WithCancel(context.Background())
	c := &ownedConn{
		transport:  transport,
		pending:    make(map[int64]chan protocol.ACPRPCResponse),
		connCtx:    ctx,
		connCancel: cancel,
	}
	if transport != nil {
		transport.OnMessage(c.handleRawMessage)
	}
	return c
}

func (c *ownedConn) Send(ctx context.Context, method string, params any, result any) error {
	transport, err := c.ensureOpen()
	if err != nil {
		return err
	}

	id := c.nextID.Add(1)
	ch := make(chan protocol.ACPRPCResponse, 1)
	c.setPending(id, ch)

	req := protocol.ACPRPCRequest{
		JSONRPC: protocol.ACPRPCVersion,
		ID:      id,
		Method:  method,
		Params:  params,
	}
	if err := transport.SendMessage(req); err != nil {
		c.removePending(id)
		return fmt.Errorf("agentv2 owned conn: send request: %w", err)
	}

	select {
	case <-ctx.Done():
		c.removePending(id)
		return ctx.Err()
	case <-c.connCtx.Done():
		c.removePending(id)
		return errors.New("agentv2 owned conn: conn is closed")
	case <-transport.Done():
		c.removePending(id)
		return errors.New("agentv2 owned conn: process exited")
	case resp := <-ch:
		if resp.Error != nil {
			return resp.Error
		}
		if result != nil && len(resp.Result) > 0 {
			if err := json.Unmarshal(resp.Result, result); err != nil {
				return fmt.Errorf("agentv2 owned conn: unmarshal result: %w", err)
			}
		}
		return nil
	}
}

func (c *ownedConn) Notify(method string, params any) error {
	transport, err := c.ensureOpen()
	if err != nil {
		return err
	}

	n := protocol.ACPRPCNotification{
		JSONRPC: protocol.ACPRPCVersion,
		Method:  method,
		Params:  params,
	}
	if err := transport.SendMessage(n); err != nil {
		return fmt.Errorf("agentv2 owned conn: send notification: %w", err)
	}
	return nil
}

func (c *ownedConn) OnRequest(h RequestHandler) {
	c.mu.Lock()
	c.handler = h
	c.mu.Unlock()
}

func (c *ownedConn) Close() error {
	if c == nil {
		return nil
	}

	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	c.handler = nil
	transport := c.transport
	c.transport = nil
	c.mu.Unlock()

	c.connCancel()
	c.failAllPending(&protocol.ACPRPCError{Code: -1, Message: "owned conn closed"})

	if transport != nil {
		return transport.Close()
	}
	return nil
}

func (c *ownedConn) ensureOpen() (ownedTransport, error) {
	c.mu.RLock()
	closed := c.closed
	transport := c.transport
	c.mu.RUnlock()

	if closed {
		return nil, errors.New("agentv2 owned conn: conn is closed")
	}
	if transport == nil {
		return nil, errors.New("agentv2 owned conn: transport is nil")
	}
	return transport, nil
}

func (c *ownedConn) setPending(id int64, ch chan protocol.ACPRPCResponse) {
	c.pendingMu.Lock()
	c.pending[id] = ch
	c.pendingMu.Unlock()
}

func (c *ownedConn) removePending(id int64) {
	c.pendingMu.Lock()
	delete(c.pending, id)
	c.pendingMu.Unlock()
}

func (c *ownedConn) popPending(id int64) (chan protocol.ACPRPCResponse, bool) {
	c.pendingMu.Lock()
	ch, ok := c.pending[id]
	if ok {
		delete(c.pending, id)
	}
	c.pendingMu.Unlock()
	return ch, ok
}

func (c *ownedConn) failAllPending(err *protocol.ACPRPCError) {
	c.pendingMu.Lock()
	defer c.pendingMu.Unlock()
	for id, ch := range c.pending {
		ch <- protocol.ACPRPCResponse{ID: id, Error: err}
		delete(c.pending, id)
	}
}

func (c *ownedConn) handleRawMessage(raw json.RawMessage) {
	if len(raw) == 0 {
		return
	}

	var msg protocol.ACPRPCRawMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		return
	}

	switch {
	case msg.ID != nil && msg.Method != "":
		go c.handleIncomingRequest(msg.JSONRPC, *msg.ID, msg.Method, msg.Params)
	case msg.ID != nil:
		resp := protocol.ACPRPCResponse{
			JSONRPC: msg.JSONRPC,
			ID:      *msg.ID,
			Result:  msg.Result,
			Error:   msg.Error,
		}
		if ch, ok := c.popPending(resp.ID); ok {
			ch <- resp
		}
	case msg.Method != "":
		c.mu.RLock()
		h := c.handler
		closed := c.closed
		c.mu.RUnlock()
		if closed || h == nil {
			return
		}
		_, _ = h(c.connCtx, msg.Method, msg.Params, true)
	}
}

func (c *ownedConn) handleIncomingRequest(jsonrpc string, id int64, method string, params json.RawMessage) {
	c.mu.RLock()
	h := c.handler
	closed := c.closed
	transport := c.transport
	c.mu.RUnlock()
	if closed || transport == nil {
		return
	}

	if jsonrpc == "" {
		jsonrpc = protocol.ACPRPCVersion
	}
	resp := protocol.ACPRPCResponse{JSONRPC: jsonrpc, ID: id}

	if h == nil {
		resp.Error = &protocol.ACPRPCError{Code: protocol.ACPRPCCodeMethodNotFound, Message: fmt.Sprintf("method not found: %s", method)}
	} else {
		result, err := h(c.connCtx, method, params, false)
		if err != nil {
			resp.Error = &protocol.ACPRPCError{Code: protocol.ACPRPCCodeInternalError, Message: err.Error()}
		} else if result == nil {
			resp.Result = json.RawMessage("null")
		} else {
			resultJSON, marshalErr := json.Marshal(result)
			if marshalErr != nil {
				resp.Error = &protocol.ACPRPCError{Code: protocol.ACPRPCCodeInternalError, Message: marshalErr.Error()}
			} else {
				resp.Result = resultJSON
			}
		}
	}

	_ = transport.SendMessage(resp)
}
