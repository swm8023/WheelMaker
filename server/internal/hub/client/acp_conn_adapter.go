package client

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/swm8023/wheelmaker/internal/hub/acp"
	"github.com/swm8023/wheelmaker/internal/hub/agentv2"
)

type acpTransportConn struct {
	raw *acp.Conn
}

var _ agentv2.Conn = (*acpTransportConn)(nil)

func wrapACPConn(raw *acp.Conn) agentv2.Conn {
	return &acpTransportConn{raw: raw}
}

func (c *acpTransportConn) Send(ctx context.Context, method string, params any, result any) error {
	if c == nil || c.raw == nil {
		return errors.New("client acp transport: nil conn")
	}
	return c.raw.SendAgent(ctx, method, params, result)
}

func (c *acpTransportConn) Notify(method string, params any) error {
	if c == nil || c.raw == nil {
		return errors.New("client acp transport: nil conn")
	}
	return c.raw.NotifyAgent(method, params)
}

func (c *acpTransportConn) OnRequest(h agentv2.RequestHandler) {
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

func (c *acpTransportConn) Close() error {
	if c == nil || c.raw == nil {
		return nil
	}
	return c.raw.Close()
}
