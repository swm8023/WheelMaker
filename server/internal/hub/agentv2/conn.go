package agentv2

import (
	"context"
	"encoding/json"
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
