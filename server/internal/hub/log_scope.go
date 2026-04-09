package hub

import (
	"fmt"
	"strings"

	logger "github.com/swm8023/wheelmaker/internal/shared"
)

type scopedLogger struct {
	source  string
	project string
}

func hubLogger(project string) scopedLogger {
	return scopedLogger{source: "Hub", project: project}
}

func registryLogger(project string) scopedLogger {
	return scopedLogger{source: "Registry", project: project}
}

func (l scopedLogger) scopedTag() string {
	return scopedTag(l.source, l.project)
}

func (l scopedLogger) scopedFormat(format string) string {
	format = strings.TrimSpace(format)
	if format == "" {
		return l.scopedTag()
	}
	return l.scopedTag() + " " + format
}

func (l scopedLogger) Debug(format string, args ...any) {
	logger.Debug(l.scopedFormat(format), args...)
}

func (l scopedLogger) Info(format string, args ...any) {
	logger.Info(l.scopedFormat(format), args...)
}

func (l scopedLogger) Warn(format string, args ...any) {
	logger.Warn(l.scopedFormat(format), args...)
}

func (l scopedLogger) Error(format string, args ...any) {
	logger.Error(l.scopedFormat(format), args...)
}

func scopedTag(source, project string) string {
	source = strings.TrimSpace(source)
	if source == "" {
		source = "Hub"
	}
	project = strings.TrimSpace(project)
	if project == "" {
		return fmt.Sprintf("[%s]", source)
	}
	return fmt.Sprintf("[%s:%s]", source, project)
}
