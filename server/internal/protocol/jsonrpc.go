package protocol

import (
	"encoding/json"
	"fmt"
)

// JSONRPCVersion is the JSON-RPC protocol version used by WheelMaker transport.
const JSONRPCVersion = "2.0"

// JSON-RPC 2.0 standard error codes.
const (
	JSONRPCCodeParseError     = -32700
	JSONRPCCodeInvalidRequest = -32600
	JSONRPCCodeMethodNotFound = -32601
	JSONRPCCodeInvalidParams  = -32602
	JSONRPCCodeInternalError  = -32603
	JSONRPCCodeAuthRequired   = -32000
	JSONRPCCodeNotFound       = -32002
)

// MaxScannerBuf is the scanner buffer size for newline-delimited JSON-RPC.
const MaxScannerBuf = 1 << 20 // 1 MiB

// Request is a JSON-RPC 2.0 request message.
type Request struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int64  `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

// Notification is a JSON-RPC 2.0 notification message.
type Notification struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

// Response is a JSON-RPC 2.0 response message.
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

// RPCError is a JSON-RPC 2.0 error object.
type RPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *RPCError) Error() string {
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
type RawMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int64          `json:"id"`
	Method  string          `json:"method"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
}
