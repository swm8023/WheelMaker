package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/swm8023/wheelmaker/internal/protocol"
	logger "github.com/swm8023/wheelmaker/internal/shared"
)

const (
	acpDebugPayloadMaxBytes = 64 * 1024
	acpSessionIDShortHead   = 6
	acpSessionIDShortTail   = 4
)

type acpRPCMessageType string

const (
	acpRPCMessageTypeRequest  acpRPCMessageType = "request"
	acpRPCMessageTypeNotify   acpRPCMessageType = "notify"
	acpRPCMessageTypeResponse acpRPCMessageType = "response"
	acpRPCMessageTypeRaw      acpRPCMessageType = "raw"
)

type acpLogEnvelope struct {
	msgType acpRPCMessageType
	method  string
	header  string
	payload []byte
}

type acpProcessLogSink interface {
	Frame(dir rune, raw []byte)
	StderrLine(line string)
	Errorf(format string, args ...any)
}

type defaultACPProcessLogSink struct {
	provider string
}

var defaultACPLogSink acpProcessLogSink = newACPProcessLogSink("-")

func newACPProcessLogSink(provider string) acpProcessLogSink {
	return defaultACPProcessLogSink{provider: normalizeACPProvider(provider)}
}

func (s defaultACPProcessLogSink) Frame(dir rune, raw []byte) {
	logger.Debug("%s", formatACPLogLine(dir, s.provider, raw))
}

func (s defaultACPProcessLogSink) StderrLine(line string) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}
	logger.Error("[acp] ![%s] %s", s.provider, string(redactAndTrimACPPayload([]byte(line))))
}

func (s defaultACPProcessLogSink) Errorf(format string, args ...any) {
	allArgs := make([]any, 0, len(args)+1)
	allArgs = append(allArgs, s.provider)
	allArgs = append(allArgs, args...)
	logger.Error("[acp] ![%s] "+format, allArgs...)
}

var acpSensitiveKeys = map[string]struct{}{
	"token":         {},
	"authorization": {},
	"cookie":        {},
	"secret":        {},
	"api_key":       {},
	"access_token":  {},
	"refresh_token": {},
	"password":      {},
}

func formatACPLogLine(dir rune, provider string, raw []byte) string {
	envelope := buildACPLogEnvelope(raw)
	payload := "-"
	if len(envelope.payload) > 0 {
		payload = string(redactAndTrimACPPayload(envelope.payload))
	}
	return fmt.Sprintf("[acp] %c[%s] %s %s", dir, normalizeACPProvider(provider), envelope.header, payload)
}

func normalizeACPProvider(provider string) string {
	name := strings.ToLower(strings.TrimSpace(provider))
	if name == "" {
		return "-"
	}
	return name
}

func buildACPLogEnvelope(raw []byte) acpLogEnvelope {
	rawNoRPC := stripJSONRPCField(raw)

	var msg protocol.ACPRPCRawMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		return acpLogEnvelope{
			msgType: acpRPCMessageTypeRaw,
			header:  "[Raw]",
			payload: rawNoRPC,
		}
	}

	method := strings.TrimSpace(msg.Method)
	switch {
	case msg.ID != nil && method != "":
		return acpLogEnvelope{
			msgType: acpRPCMessageTypeRequest,
			method:  method,
			header:  fmt.Sprintf("[Req %d %s]", *msg.ID, method),
			payload: filterACPBody(acpRPCMessageTypeRequest, method, selectRequestPayload(msg.Params, rawNoRPC)),
		}
	case method != "":
		return acpLogEnvelope{
			msgType: acpRPCMessageTypeNotify,
			method:  method,
			header:  fmt.Sprintf("[Notify %s]", method),
			payload: filterACPBody(acpRPCMessageTypeNotify, method, selectRequestPayload(msg.Params, rawNoRPC)),
		}
	case msg.ID != nil:
		return acpLogEnvelope{
			msgType: acpRPCMessageTypeResponse,
			header:  fmt.Sprintf("[Resp %d]", *msg.ID),
			payload: filterACPBody(acpRPCMessageTypeResponse, "", selectResponsePayload(msg.Result, msg.Error, rawNoRPC)),
		}
	default:
		return acpLogEnvelope{
			msgType: acpRPCMessageTypeRaw,
			header:  "[Raw]",
			payload: rawNoRPC,
		}
	}
}

func selectRequestPayload(params json.RawMessage, fallback []byte) []byte {
	trimmed := bytes.TrimSpace(params)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return fallback
	}
	return stripJSONRPCField(trimmed)
}

func selectResponsePayload(result json.RawMessage, rpcErr *protocol.ACPRPCError, fallback []byte) []byte {
	trimmed := bytes.TrimSpace(result)
	if len(trimmed) != 0 && !bytes.Equal(trimmed, []byte("null")) {
		return stripJSONRPCField(trimmed)
	}
	if rpcErr != nil {
		if b, err := json.Marshal(rpcErr); err == nil {
			return stripJSONRPCField(b)
		}
	}
	return fallback
}

func filterACPBody(msgType acpRPCMessageType, method string, body []byte) []byte {
	body = stripJSONRPCField(body)
	if msgType == acpRPCMessageTypeNotify && method == protocol.MethodSessionUpdate {
		if filtered, ok := filterNotifySessionUpdateBody(body); ok {
			return filtered
		}
	}
	return body
}

func filterNotifySessionUpdateBody(body []byte) ([]byte, bool) {
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, false
	}

	update, ok := m["update"].(map[string]any)
	if !ok {
		return nil, false
	}
	updateKind, _ := update["sessionUpdate"].(string)
	updateKind = strings.TrimSpace(updateKind)
	if updateKind == "" {
		return nil, false
	}

	updateBody := make(map[string]any, len(update))
	for k, v := range update {
		if k == "sessionUpdate" {
			continue
		}
		updateBody[k] = v
	}
	updateBytes, err := json.Marshal(updateBody)
	if err != nil {
		return nil, false
	}

	sessionID, _ := m["sessionId"].(string)
	sessionID = shortenSessionID(sessionID)
	return []byte(fmt.Sprintf("%s, %s %s", sessionID, updateKind, string(updateBytes))), true
}

func shortenSessionID(sessionID string) string {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return "-"
	}
	if len(sessionID) <= acpSessionIDShortHead+acpSessionIDShortTail+3 {
		return sessionID
	}
	return sessionID[:acpSessionIDShortHead] + "..." + sessionID[len(sessionID)-acpSessionIDShortTail:]
}

func stripJSONRPCField(raw []byte) []byte {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return raw
	}

	var m map[string]any
	if err := json.Unmarshal(trimmed, &m); err != nil {
		return raw
	}
	delete(m, "jsonrpc")

	b, err := json.Marshal(m)
	if err != nil {
		return raw
	}
	return b
}

func redactAndTrimACPPayload(raw []byte) []byte {
	redacted := redactACPPayload(raw)
	if len(redacted) <= acpDebugPayloadMaxBytes {
		return redacted
	}
	return redacted[:acpDebugPayloadMaxBytes]
}

func redactACPPayload(raw []byte) []byte {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return []byte(redactPlainText(string(raw)))
	}
	sanitizeJSONValue(v)

	buf := bytes.NewBuffer(nil)
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return []byte(redactPlainText(string(raw)))
	}
	return bytes.TrimSpace(buf.Bytes())
}

func sanitizeJSONValue(v any) {
	switch x := v.(type) {
	case map[string]any:
		for k, vv := range x {
			if isSensitiveKey(k) {
				x[k] = "***"
				continue
			}
			sanitizeJSONValue(vv)
		}
	case []any:
		for i := range x {
			sanitizeJSONValue(x[i])
		}
	}
}

func isSensitiveKey(k string) bool {
	_, ok := acpSensitiveKeys[strings.ToLower(strings.TrimSpace(k))]
	return ok
}

func redactPlainText(s string) string {
	repls := []string{"token", "authorization", "cookie", "secret", "api_key", "password"}
	out := s
	for _, key := range repls {
		out = strings.ReplaceAll(out, key+":", key+":***")
		out = strings.ReplaceAll(out, key+"=", key+"=***")
	}
	return out
}
