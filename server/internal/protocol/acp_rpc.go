package protocol

import (
	"encoding/json"
	"fmt"
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
const ACPRPCMaxScannerBuf = 1 << 20 // 1 MiB

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
