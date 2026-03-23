package debuglog

import (
	"fmt"
	"io"
	"strings"
)

// Logger defines unified debug log output.
type Logger interface {
	Log(direction, domain, content string)
}

type writerLogger struct {
	w io.Writer
}

// New creates a logger backed by io.Writer.
func New(w io.Writer) Logger {
	if w == nil {
		return nil
	}
	return &writerLogger{w: w}
}

func (l *writerLogger) Log(direction, domain, content string) {
	if l == nil || l.w == nil {
		return
	}
	direction = strings.TrimSpace(direction)
	domain = strings.TrimSpace(domain)
	content = strings.TrimSpace(content)
	if direction == "" || domain == "" || content == "" {
		return
	}
	_, _ = fmt.Fprintf(l.w, "%s[%s] %s\n", direction, domain, content)
}

