package agentv2

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/swm8023/wheelmaker/internal/hub/acp"
)

// RequestHandler handles inbound ACP requests/notifications.
type RequestHandler func(ctx context.Context, method string, params json.RawMessage, noResponse bool) (any, error)

// Conn is a transport-only ACP connection contract.
type Conn interface {
	Send(ctx context.Context, method string, params any, result any) error
	Notify(method string, params any) error
	OnRequest(h RequestHandler)
	Close() error
}

type transportConn struct {
	raw *acp.Conn
}

var _ Conn = (*transportConn)(nil)

// New wraps an already-started acp.Conn as an agentv2 transport conn.
func New(raw *acp.Conn) Conn {
	return &transportConn{raw: raw}
}

// NewProcessConn starts a subprocess-backed ACP transport connection.
func NewProcessConn(exePath string, env []string, args ...string) (Conn, error) {
	raw := acp.NewConn(exePath, env, args...)
	if err := raw.Start(); err != nil {
		return nil, err
	}
	return &transportConn{raw: raw}, nil
}

// NewInMemoryConn starts an in-memory ACP transport connection for tests.
func NewInMemoryConn(server acp.InMemoryServer) (Conn, error) {
	raw := acp.NewInMemoryConn(server)
	if err := raw.Start(); err != nil {
		return nil, err
	}
	return &transportConn{raw: raw}, nil
}

func (c *transportConn) Send(ctx context.Context, method string, params any, result any) error {
	if c == nil || c.raw == nil {
		return errors.New("agentv2 conn: nil transport")
	}
	return c.raw.SendAgent(ctx, method, params, result)
}

func (c *transportConn) Notify(method string, params any) error {
	if c == nil || c.raw == nil {
		return errors.New("agentv2 conn: nil transport")
	}
	return c.raw.NotifyAgent(method, params)
}

func (c *transportConn) OnRequest(h RequestHandler) {
	if c == nil || c.raw == nil {
		return
	}
	if h == nil {
		c.raw.OnRequest(nil)
		return
	}
	c.raw.OnRequest(func(ctx context.Context, method string, params json.RawMessage, noResponse bool) (any, error) {
		return h(ctx, method, params, noResponse)
	})
}

func (c *transportConn) Close() error {
	if c == nil || c.raw == nil {
		return nil
	}
	return c.raw.Close()
}
