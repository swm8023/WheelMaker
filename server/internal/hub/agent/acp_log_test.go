package agent

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestFormatACPLogLine_MinimalShape(t *testing.T) {
	payload := []byte(`{"jsonrpc":"2.0","id":3,"method":"session/prompt","params":{"sessionId":"sess-1","token":"abc","prompt":"hello"}}`)
	line := formatACPLogLine('>', payload)
	if !strings.HasPrefix(line, "[acp] > {sess-1 session/prompt} ") {
		t.Fatalf("line=%q", line)
	}
	if strings.Contains(line, "jsonrpc") {
		t.Fatalf("line contains verbose metadata: %q", line)
	}
}

func TestRedactACPPayload_JSONKeys(t *testing.T) {
	raw := []byte(`{"authorization":"Bearer X","nested":{"token":"abc","password":"p"}}`)
	redacted := redactACPPayload(raw)
	s := string(redacted)
	if strings.Contains(s, "Bearer X") || strings.Contains(s, "abc") || strings.Contains(s, "\"p\"") {
		t.Fatalf("redaction failed: %s", s)
	}
	if !strings.Contains(s, "***") {
		t.Fatalf("expected masked marker: %s", s)
	}
	var obj map[string]any
	if err := json.Unmarshal(redacted, &obj); err != nil {
		t.Fatalf("redacted json invalid: %v", err)
	}
}

func TestRedactACPPayload_Truncate64KB(t *testing.T) {
	base := strings.Repeat("x", acpDebugPayloadMaxBytes+1024)
	raw := []byte(`{"method":"session/prompt","params":{"sessionId":"s","content":"` + base + `"}}`)
	out := redactAndTrimACPPayload(raw)
	if len(out) > acpDebugPayloadMaxBytes {
		t.Fatalf("len=%d, want <=%d", len(out), acpDebugPayloadMaxBytes)
	}
}

