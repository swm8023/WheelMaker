package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/swm8023/wheelmaker/internal/hub/acp"
	"github.com/swm8023/wheelmaker/internal/hub/agentv2"
)

type acpTransportConn struct {
	raw *acp.Conn

	mu          sync.RWMutex
	reqHandler  agentv2.ACPRequestHandler
	respHandler agentv2.ACPResponseHandler
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

func (c *acpTransportConn) OnACPRequest(h agentv2.ACPRequestHandler) {
	if c == nil || c.raw == nil {
		return
	}
	c.mu.Lock()
	c.reqHandler = h
	c.mu.Unlock()
	c.bindRawHandler()
}

func (c *acpTransportConn) OnACPResponse(h agentv2.ACPResponseHandler) {
	if c == nil || c.raw == nil {
		return
	}
	c.mu.Lock()
	c.respHandler = h
	c.mu.Unlock()
	c.bindRawHandler()
}

func (c *acpTransportConn) bindRawHandler() {
	c.mu.RLock()
	req := c.reqHandler
	resp := c.respHandler
	c.mu.RUnlock()

	if req == nil && resp == nil {
		c.raw.OnRequest(nil)
		return
	}

	c.raw.OnRequest(func(ctx context.Context, method string, params json.RawMessage, noResponse bool) (any, error) {
		c.mu.RLock()
		currentReq := c.reqHandler
		currentResp := c.respHandler
		c.mu.RUnlock()

		if noResponse {
			if currentResp != nil {
				currentResp(ctx, method, params)
			}
			return nil, nil
		}

		if currentReq == nil {
			return nil, fmt.Errorf("no ACP request handler for method: %s", method)
		}
		return currentReq(ctx, method, params)
	})
}

func (c *acpTransportConn) Close() error {
	if c == nil || c.raw == nil {
		return nil
	}
	return c.raw.Close()
}
