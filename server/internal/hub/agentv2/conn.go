package agentv2

import (
	"context"
	"encoding/json"
)

// ACPRequestHandler handles inbound ACP requests that require a JSON-RPC response.
type ACPRequestHandler func(ctx context.Context, method string, params json.RawMessage) (any, error)

// ACPResponseHandler handles inbound ACP notifications that do not require a response.
type ACPResponseHandler func(ctx context.Context, method string, params json.RawMessage)

// Conn is a transport-only ACP connection contract.
//
// Conn must only provide raw request/response transport operations and
// callback registration. Protocol-specific business dispatch belongs
// to Instance callbacks.
type Conn interface {
	Send(ctx context.Context, method string, params any, result any) error
	Notify(method string, params any) error
	OnACPRequest(h ACPRequestHandler)
	OnACPResponse(h ACPResponseHandler)
	Close() error
}
