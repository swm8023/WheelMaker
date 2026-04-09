package agent

import (
	"strings"

	logger "github.com/swm8023/wheelmaker/internal/shared"
)

type scopedLogger struct {
	source string
}

func agentLogger() scopedLogger {
	return scopedLogger{source: "Agent"}
}

func (l scopedLogger) format(format string) string {
	format = strings.TrimSpace(format)
	tag := "[Agent]"
	if strings.TrimSpace(l.source) != "" {
		tag = "[" + strings.TrimSpace(l.source) + "]"
	}
	if format == "" {
		return tag
	}
	return tag + " " + format
}

func (l scopedLogger) Debug(format string, args ...any) {
	logger.Debug(l.format(format), args...)
}

func (l scopedLogger) Info(format string, args ...any) {
	logger.Info(l.format(format), args...)
}

func (l scopedLogger) Warn(format string, args ...any) {
	logger.Warn(l.format(format), args...)
}

func (l scopedLogger) Error(format string, args ...any) {
	logger.Error(l.format(format), args...)
}
