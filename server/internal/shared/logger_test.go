package shared

import (
	"bytes"
	"path/filepath"
	"testing"
)

type panicStringer struct{}

func (panicStringer) String() string {
	panic("String() should not be called for filtered logs")
}

func TestLogger_FilteredLogsDoNotFormatArguments(t *testing.T) {
	var out bytes.Buffer
	if err := Setup(LoggerConfig{Level: LevelWarn}); err != nil {
		t.Fatalf("setup logger: %v", err)
	}
	defer Close()
	SetOutput(&out)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("filtered log unexpectedly formatted arguments: %v", r)
		}
	}()

	Debug("%v", panicStringer{})
	Info("%v", panicStringer{})
}

func TestDeriveDebugLogPath(t *testing.T) {
	base := filepath.Join("tmp", "hub.log")
	got := deriveDebugLogPath(base)
	want := filepath.Join("tmp", "hub.debug.log")
	if got != want {
		t.Fatalf("deriveDebugLogPath(%q)=%q, want %q", base, got, want)
	}
}
