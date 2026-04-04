package agentv2

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
)

// ProcessConnRouter provides routed Conn handles for instances.
//
// Router can work in own-conn mode (one raw conn per route) or shared mode
// (multiple routes multiplexed over one raw conn).
type ProcessConnRouter struct {
	mu      sync.RWMutex
	connect func() (Conn, error)
	share   bool

	closed atomic.Bool
	seq    atomic.Uint64

	routes         map[string]*routerConn
	sessionToRoute map[string]string

	sharedConn         Conn
	sharedRefCount     int
	sharedDefaultRoute string
}

// NewProcessConnRouter creates a new router.
// connect must return a ready-to-use connection (already started).
func NewProcessConnRouter(connect func() (Conn, error), share bool) *ProcessConnRouter {
	return &ProcessConnRouter{
		connect:        connect,
		share:          share,
		routes:         make(map[string]*routerConn),
		sessionToRoute: make(map[string]string),
	}
}

// Open allocates a routed Conn handle for one instance.
func (r *ProcessConnRouter) Open() (Conn, error) {
	if r == nil {
		return nil, errors.New("agentv2 conn router: nil router")
	}
	if r.connect == nil {
		return nil, errors.New("agentv2 conn router: connect func is nil")
	}
	if r.closed.Load() {
		return nil, errors.New("agentv2 conn router: router is closed")
	}

	routeKey := fmt.Sprintf("route-%d", r.seq.Add(1))

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed.Load() {
		return nil, errors.New("agentv2 conn router: router is closed")
	}

	var raw Conn
	if r.share {
		if r.sharedConn == nil {
			conn, err := r.connect()
			if err != nil {
				return nil, err
			}
			r.sharedConn = conn
			r.sharedConn.OnRequest(r.dispatchSharedInbound)
		}
		raw = r.sharedConn
		r.sharedRefCount++
	} else {
		conn, err := r.connect()
		if err != nil {
			return nil, err
		}
		raw = conn
	}

	rc := &routerConn{
		router:   r,
		routeKey: routeKey,
		raw:      raw,
	}

	r.routes[routeKey] = rc
	if r.share {
		if r.sharedDefaultRoute == "" {
			r.sharedDefaultRoute = routeKey
		}
	} else {
		raw.OnRequest(rc.dispatchOwnedInbound)
	}

	return rc, nil
}

// Close closes router and all active routed/raw connections.
func (r *ProcessConnRouter) Close() error {
	if r == nil {
		return nil
	}
	if !r.closed.CompareAndSwap(false, true) {
		return nil
	}

	var toClose []Conn

	r.mu.Lock()
	if r.share {
		if r.sharedConn != nil {
			toClose = append(toClose, r.sharedConn)
		}
	} else {
		for _, rc := range r.routes {
			if rc != nil && rc.raw != nil {
				toClose = append(toClose, rc.raw)
			}
		}
	}
	for _, rc := range r.routes {
		if rc != nil {
			rc.mu.Lock()
			rc.closed = true
			rc.mu.Unlock()
		}
	}
	r.routes = map[string]*routerConn{}
	r.sessionToRoute = map[string]string{}
	r.sharedConn = nil
	r.sharedRefCount = 0
	r.sharedDefaultRoute = ""
	r.mu.Unlock()

	for _, c := range toClose {
		_ = c.Close()
	}
	return nil
}

func (r *ProcessConnRouter) dispatchSharedInbound(ctx context.Context, method string, params json.RawMessage, noResponse bool) (any, error) {
	target := r.resolveRoute("", params)
	if target == nil {
		if noResponse {
			return nil, nil
		}
		return nil, fmt.Errorf("agentv2 conn router: no route for inbound method %q", method)
	}
	return target.invokeInbound(ctx, method, params, noResponse)
}

func (r *ProcessConnRouter) resolveRoute(defaultRouteKey string, params json.RawMessage) *routerConn {
	sid := parseInboundSessionID(params)

	r.mu.RLock()
	defer r.mu.RUnlock()

	routeKey := defaultRouteKey
	if sid != "" {
		if mapped, ok := r.sessionToRoute[sid]; ok {
			routeKey = mapped
		}
	}
	if routeKey == "" && r.share {
		routeKey = r.sharedDefaultRoute
	}
	if routeKey == "" {
		return nil
	}
	return r.routes[routeKey]
}

func parseInboundSessionID(params json.RawMessage) string {
	if len(params) == 0 {
		return ""
	}
	var p struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return ""
	}
	return strings.TrimSpace(p.SessionID)
}

type routerConn struct {
	router   *ProcessConnRouter
	routeKey string
	raw      Conn

	mu      sync.RWMutex
	handler RequestHandler
	closed  bool
}

var _ Conn = (*routerConn)(nil)

type sessionBinder interface {
	BindSessionID(acpSessionID string)
}

func (c *routerConn) BindSessionID(acpSessionID string) {
	if c == nil || c.router == nil {
		return
	}
	sid := strings.TrimSpace(acpSessionID)
	if sid == "" {
		return
	}
	c.router.mu.Lock()
	if _, ok := c.router.routes[c.routeKey]; ok {
		c.router.sessionToRoute[sid] = c.routeKey
	}
	c.router.mu.Unlock()
}

func (c *routerConn) Send(ctx context.Context, method string, params any, result any) error {
	raw, err := c.rawConn()
	if err != nil {
		return err
	}
	return raw.Send(ctx, method, params, result)
}

func (c *routerConn) Notify(method string, params any) error {
	raw, err := c.rawConn()
	if err != nil {
		return err
	}
	return raw.Notify(method, params)
}

func (c *routerConn) OnRequest(h RequestHandler) {
	c.mu.Lock()
	c.handler = h
	c.mu.Unlock()
}

func (c *routerConn) Close() error {
	if c == nil || c.router == nil {
		return nil
	}
	return c.router.closeRoute(c.routeKey)
}

func (c *routerConn) rawConn() (Conn, error) {
	c.mu.RLock()
	closed := c.closed
	raw := c.raw
	c.mu.RUnlock()
	if closed {
		return nil, errors.New("agentv2 conn router: route is closed")
	}
	if raw == nil {
		return nil, errors.New("agentv2 conn router: raw conn is nil")
	}
	return raw, nil
}

func (c *routerConn) dispatchOwnedInbound(ctx context.Context, method string, params json.RawMessage, noResponse bool) (any, error) {
	target := c.router.resolveRoute(c.routeKey, params)
	if target == nil {
		if noResponse {
			return nil, nil
		}
		return nil, fmt.Errorf("agentv2 conn router: no route for inbound method %q", method)
	}
	return target.invokeInbound(ctx, method, params, noResponse)
}

func (c *routerConn) invokeInbound(ctx context.Context, method string, params json.RawMessage, noResponse bool) (any, error) {
	c.mu.RLock()
	closed := c.closed
	h := c.handler
	c.mu.RUnlock()

	if closed {
		if noResponse {
			return nil, nil
		}
		return nil, errors.New("agentv2 conn router: route is closed")
	}
	if h == nil {
		if noResponse {
			return nil, nil
		}
		return nil, fmt.Errorf("agentv2 conn router: no handler for route %s", c.routeKey)
	}
	return h(ctx, method, params, noResponse)
}

func (r *ProcessConnRouter) closeRoute(routeKey string) error {
	if r == nil {
		return nil
	}

	r.mu.Lock()
	rc, ok := r.routes[routeKey]
	if !ok {
		r.mu.Unlock()
		return nil
	}
	delete(r.routes, routeKey)
	for sid, mapped := range r.sessionToRoute {
		if mapped == routeKey {
			delete(r.sessionToRoute, sid)
		}
	}

	var toClose Conn
	if r.share {
		r.sharedRefCount--
		if r.sharedRefCount <= 0 {
			toClose = r.sharedConn
			r.sharedConn = nil
			r.sharedRefCount = 0
			r.sharedDefaultRoute = ""
		} else if r.sharedDefaultRoute == routeKey {
			r.sharedDefaultRoute = ""
			for k := range r.routes {
				r.sharedDefaultRoute = k
				break
			}
		}
	} else {
		toClose = rc.raw
	}

	rc.mu.Lock()
	rc.closed = true
	rc.handler = nil
	rc.mu.Unlock()
	r.mu.Unlock()

	if toClose != nil {
		return toClose.Close()
	}
	return nil
}
