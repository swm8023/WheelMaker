package registry

import (
	"fmt"
	"strings"

	logger "github.com/swm8023/wheelmaker/internal/shared"
)

type scopedLogger struct {
	project string
}

func registryLogger(project string) scopedLogger {
	return scopedLogger{project: project}
}

func (l scopedLogger) tag() string {
	project := strings.TrimSpace(l.project)
	if project == "" {
		return "[Registry]"
	}
	return fmt.Sprintf("[Registry:%s]", project)
}

func (l scopedLogger) format(format string) string {
	format = strings.TrimSpace(format)
	if format == "" {
		return l.tag()
	}
	return l.tag() + " " + format
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
