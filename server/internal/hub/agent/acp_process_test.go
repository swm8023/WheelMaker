package agent

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	logger "github.com/swm8023/wheelmaker/internal/shared"
)

func TestLogOutboundACPDebugLine(t *testing.T) {
	debugLog := filepath.Join(t.TempDir(), "hub.debug.log")
	if err := logger.Setup(logger.LoggerConfig{Level: logger.LevelDebug, DebugLogFile: debugLog}); err != nil {
		t.Fatalf("setup logger: %v", err)
	}

	raw := []byte(`{"method":"session/prompt","params":{"sessionId":"sess-1","token":"abc"}}`)
	defaultACPLogSink.Frame('>', raw)
	logger.Close()

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
	if err := logger.Setup(logger.LoggerConfig{Level: logger.LevelWarn}); err != nil {
		t.Fatalf("setup logger: %v", err)
	}
	defer logger.Close()
	logger.SetOutput(&buf)
	defer logger.SetOutput(os.Stderr)

	defaultACPLogSink.StderrLine("panic: worker crashed")
	got := buf.String()
	if !strings.Contains(got, "[acp] ! {- -} panic: worker crashed") {
		t.Fatalf("unexpected stderr log: %q", got)
	}
}
