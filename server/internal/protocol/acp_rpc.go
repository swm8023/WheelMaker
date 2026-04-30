package protocol

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ACPRPCVersion is the JSON-RPC protocol version used by WheelMaker transport.
const ACPRPCVersion = "2.0"

// JSON-RPC 2.0 standard error codes.
const (
	ACPRPCCodeParseError     = -32700
	ACPRPCCodeInvalidRequest = -32600
	ACPRPCCodeMethodNotFound = -32601
	ACPRPCCodeInvalidParams  = -32602
	ACPRPCCodeInternalError  = -32603
	ACPRPCCodeAuthRequired   = -32000
	ACPRPCCodeNotFound       = -32002
)

// ACPRPCMaxScannerBuf is the scanner buffer size for newline-delimited JSON-RPC.
// Some providers can emit very large single-line JSON payloads (for example
// large tool outputs). Keep this comfortably above the default scanner limit.
const ACPRPCMaxScannerBuf = 8 << 20 // 8 MiB

// Request is a JSON-RPC 2.0 request message.
type ACPRPCRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int64  `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

// Notification is a JSON-RPC 2.0 notification message.
type ACPRPCNotification struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

// Response is a JSON-RPC 2.0 response message.
type ACPRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *ACPRPCError    `json:"error,omitempty"`
}

// RPCError is a JSON-RPC 2.0 error object.
type ACPRPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *ACPRPCError) Error() string {
	if e == nil {
		return ""
	}
	if len(e.Data) > 0 {
		var d struct {
			Message string `json:"message"`
		}
		if json.Unmarshal(e.Data, &d) == nil && d.Message != "" {
			return d.Message
		}
		return fmt.Sprintf("%s: %s", e.Message, e.Data)
	}
	return e.Message
}

// RawMessage is an internal parse shape for JSON-RPC routing.
type ACPRPCRawMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int64          `json:"id"`
	Method  string          `json:"method"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *ACPRPCError    `json:"error,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
}

func BuildACPContentJSON(method string, fields map[string]any) string {
	method = strings.TrimSpace(method)
	if method == "" {
		return "{}"
	}
	doc := map[string]any{"method": method}
	for key, value := range fields {
		doc[key] = value
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		return fmt.Sprintf(`{"method":%q}`, method)
	}
	return string(raw)
}
