package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

const acpDebugPayloadMaxBytes = 64 * 1024

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

func formatACPLogLine(dir rune, raw []byte) string {
	sessionID, method := extractACPLogSessionMethod(raw)
	payload := string(redactAndTrimACPPayload(extractACPLogPayload(raw)))
	return fmt.Sprintf("[acp] %c {%s %s} %s", dir, sessionID, method, payload)
}

func extractACPLogSessionMethod(raw []byte) (string, string) {
	sessionID := "-"
	method := "-"

	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return sessionID, method
	}

	if v, ok := m["method"].(string); ok {
		v = strings.TrimSpace(v)
		if v != "" {
			method = v
		}
	}

	if params, ok := m["params"].(map[string]any); ok {
		if sid, ok := params["sessionId"].(string); ok {
			sid = strings.TrimSpace(sid)
			if sid != "" {
				sessionID = sid
			}
		}
	}
	if sessionID == "-" {
		if sid, ok := m["sessionId"].(string); ok {
			sid = strings.TrimSpace(sid)
			if sid != "" {
				sessionID = sid
			}
		}
	}

	return sessionID, method
}

func extractACPLogPayload(raw []byte) []byte {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return raw
	}

	if params, ok := m["params"]; ok {
		if b, err := json.Marshal(params); err == nil {
			return b
		}
	}
	if result, ok := m["result"]; ok {
		if b, err := json.Marshal(result); err == nil {
			return b
		}
	}
	if errObj, ok := m["error"]; ok {
		if b, err := json.Marshal(errObj); err == nil {
			return b
		}
	}
	return raw
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

