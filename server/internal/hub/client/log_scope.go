package client

import (
	"fmt"
	"strings"

	logger "github.com/swm8023/wheelmaker/internal/shared"
)

type scopedLogger struct {
	project string
}

func hubLogger(project string) scopedLogger {
	return scopedLogger{project: project}
}

func (l scopedLogger) tag() string {
	return hubTag(l.project)
}

func (l scopedLogger) format(message string) string {
	message = strings.TrimSpace(message)
	if message == "" {
		return l.tag()
	}
	return l.tag() + " " + message
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

func hubTag(project string) string {
	project = strings.TrimSpace(project)
	if project == "" {
		return "[Hub]"
	}
	return fmt.Sprintf("[Hub:%s]", project)
}
