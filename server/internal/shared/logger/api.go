// Package logger provides unified leveled logging and separated debug trace sinks.
package logger

import (
	"io"
	"os"
)

// global is the process-wide logger instance.
var global = &inst{
	level: LevelWarn,
	out:   os.Stderr,
}

// Setup configures the global logger.
func Setup(cfg Config) error { return global.setup(cfg) }

// Close releases open file resources.
func Close() { global.close() }

// DebugWriter returns protocol trace writer, nil unless level is debug.
func DebugWriter() io.Writer {
	global.mu.Lock()
	defer global.mu.Unlock()
	return global.debugOut
}

// SetOutput overrides operational output writer. Primarily for tests.
func SetOutput(w io.Writer) {
	global.mu.Lock()
	defer global.mu.Unlock()
	global.out = w
}

// Debug emits a debug-level message to debug sink only.
func Debug(format string, args ...any) { global.emit(LevelDebug, format, args...) }

// Info emits info-level message.
func Info(format string, args ...any) { global.emit(LevelInfo, format, args...) }

// Warn emits warning-level message.
func Warn(format string, args ...any) { global.emit(LevelWarn, format, args...) }

// Error emits error-level message.
func Error(format string, args ...any) { global.emit(LevelError, format, args...) }
