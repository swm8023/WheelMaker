package agentv2

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
)

// sessionBinder allows Conn implementations to bind ACP session IDs for inbound routing.
type sessionBinder interface {
	BindSessionID(acpSessionID string)
}

// OwnedConnPool creates one raw connection per opened Conn handle.
type OwnedConnPool struct {
	connect func() (Conn, error)
	closed  atomic.Bool
}

func NewOwnedConnPool(connect func() (Conn, error)) *OwnedConnPool {
	return &OwnedConnPool{connect: connect}
}

func (p *OwnedConnPool) Open() (Conn, error) {
	if p == nil {
		return nil, errors.New("agentv2 owned conn pool: nil pool")
	}
	if p.connect == nil {
		return nil, errors.New("agentv2 owned conn pool: connect func is nil")
	}
	if p.closed.Load() {
		return nil, errors.New("agentv2 owned conn pool: pool is closed")
	}

	raw, err := p.connect()
	if err != nil {
		return nil, err
	}
	return newOwnedConn(raw), nil
}

func (p *OwnedConnPool) Close() error {
	if p == nil {
		return nil
	}
	p.closed.Store(true)
	return nil
}

// ownedConn maps one instance to one underlying raw connection.
type ownedConn struct {
	raw Conn

	mu      sync.RWMutex
	handler RequestHandler
	closed  bool
}

var _ Conn = (*ownedConn)(nil)

func newOwnedConn(raw Conn) *ownedConn {
	c := &ownedConn{raw: raw}
	if raw != nil {
		raw.OnRequest(c.dispatchInbound)
	}
	return c
}

func (c *ownedConn) Send(ctx context.Context, method string, params any, result any) error {
	raw, err := c.rawConn()
	if err != nil {
		return err
	}
	return raw.Send(ctx, method, params, result)
}

func (c *ownedConn) Notify(method string, params any) error {
	raw, err := c.rawConn()
	if err != nil {
		return err
	}
	return raw.Notify(method, params)
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
	raw := c.raw
	c.raw = nil
	c.mu.Unlock()

	if raw != nil {
		return raw.Close()
	}
	return nil
}

func (c *ownedConn) rawConn() (Conn, error) {
	c.mu.RLock()
	closed := c.closed
	raw := c.raw
	c.mu.RUnlock()

	if closed {
		return nil, errors.New("agentv2 owned conn: conn is closed")
	}
	if raw == nil {
		return nil, errors.New("agentv2 owned conn: raw conn is nil")
	}
	return raw, nil
}

func (c *ownedConn) dispatchInbound(ctx context.Context, method string, params json.RawMessage, noResponse bool) (any, error) {
	c.mu.RLock()
	closed := c.closed
	h := c.handler
	c.mu.RUnlock()

	if closed {
		if noResponse {
			return nil, nil
		}
		return nil, errors.New("agentv2 owned conn: conn is closed")
	}
	if h == nil {
		if noResponse {
			return nil, nil
		}
		return nil, fmt.Errorf("agentv2 owned conn: no request handler for method %q", method)
	}
	return h(ctx, method, params, noResponse)
}
