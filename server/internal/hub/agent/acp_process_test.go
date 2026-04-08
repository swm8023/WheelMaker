package agent

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	shared "github.com/swm8023/wheelmaker/internal/shared"
)

func TestLogOutboundACPDebugLine(t *testing.T) {
	debugLog := filepath.Join(t.TempDir(), "hub.debug.log")
	if err := shared.Setup(shared.LoggerConfig{Level: shared.LevelDebug, DebugLogFile: debugLog}); err != nil {
		t.Fatalf("setup logger: %v", err)
	}

	raw := []byte(`{"method":"session/prompt","params":{"sessionId":"sess-1","token":"abc"}}`)
	logACPDebugRaw('>', raw)
	shared.Close()

	data, err := os.ReadFile(debugLog)
	if err != nil {
		t.Fatalf("read debug log: %v", err)
	}

	got := string(data)
	if got == "" || !strings.Contains(got, "[acp] > {sess-1 session/prompt}") {
		t.Fatalf("unexpected outbound log: %q", got)
	}
	if strings.Contains(got, "abc") {
		t.Fatalf("outbound log should redact payload: %q", got)
	}
}

func TestLogACPStderrLineAsError(t *testing.T) {
	var buf bytes.Buffer
	if err := shared.Setup(shared.LoggerConfig{Level: shared.LevelWarn}); err != nil {
		t.Fatalf("setup logger: %v", err)
	}
	defer shared.Close()
	shared.SetOutput(&buf)
	defer shared.SetOutput(os.Stderr)

	logACPStderrLine("panic: worker crashed")
	got := buf.String()
	if !strings.Contains(got, "[acp] ! {- -} panic: worker crashed") {
		t.Fatalf("unexpected stderr log: %q", got)
	}
}
