package agentv2

import "errors"

// ConnPool opens per-instance Conn handles and owns their lifecycle policy.
type ConnPool interface {
	Open() (Conn, error)
	Close() error
}

// ProcessConnRouter is a compatibility shim around the new ConnPool model.
// New code should prefer NewOwnedConnPool/NewSharedConnPool directly.
type ProcessConnRouter struct {
	pool ConnPool
}

// NewProcessConnRouter creates a compatibility router backed by owned/shared pools.
func NewProcessConnRouter(connect func() (Conn, error), share bool) *ProcessConnRouter {
	if share {
		return &ProcessConnRouter{pool: NewSharedConnPool(connect)}
	}
	return &ProcessConnRouter{pool: NewOwnedConnPool(connect)}
}

func (r *ProcessConnRouter) Open() (Conn, error) {
	if r == nil || r.pool == nil {
		return nil, errors.New("agentv2 conn router: nil pool")
	}
	return r.pool.Open()
}

func (r *ProcessConnRouter) Close() error {
	if r == nil || r.pool == nil {
		return nil
	}
	return r.pool.Close()
}
