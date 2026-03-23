// Package logger provides a unified, levelled operational logger for WheelMaker.
//
// Usage:
//
//	logger.Setup(logger.Config{Level: logger.LevelWarn, LogFile: "/path/to/wheelmaker.log"})
//	defer logger.Close()
//
//	logger.Info("[mobile] listening on %s", addr)
//	logger.Warn("client: idle timeout: %v", err)
//	logger.Error("hub: project run error: %v", err)
//
// Protocol-trace output (ACP JSON, IM bridge) uses DebugWriter():
//
//	w := logger.DebugWriter() // nil when level > Debug
//	if w != nil { fmt.Fprintf(w, "->[acp] %s\n", raw) }
//
// When Level == LevelDebug and LogFile is set, protocol trace is written to
// <logdir>/wheelmaker.debug.log (truncated on each startup) instead of the main log.
package logger

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Level represents minimum log severity.
type Level int

const (
	// LevelDebug emits all messages including verbose protocol trace.
	LevelDebug Level = iota
	// LevelInfo emits informational, warning, and error messages.
	LevelInfo
	// LevelWarn emits warnings and errors (default).
	LevelWarn
	// LevelError emits errors only.
	LevelError
)

// ParseLevel converts a string to Level. Unknown values default to LevelWarn.
//
//	"debug"          → LevelDebug
//	"info"           → LevelInfo
//	"warn"/"warning" → LevelWarn
//	"error"          → LevelError
func ParseLevel(s string) Level {
	switch s {
	case "debug":
		return LevelDebug
	case "info":
		return LevelInfo
	case "error":
		return LevelError
	default:
		return LevelWarn
	}
}

// Config holds logger setup parameters.
type Config struct {
	// Level is the minimum severity to emit. Default LevelWarn.
	Level Level

	// LogFile, if non-empty, appends operational logs to this file in addition
	// to stderr. When Level == LevelDebug, protocol-trace output is written to
	// <dir>/wheelmaker.debug.log (truncated each startup) in the same directory.
	LogFile string
}

// global is the process-wide logger instance.
var global = &inst{
	level: LevelWarn,
	out:   os.Stderr,
}

// Setup configures the global logger. Should be called once before any logging.
// Calling Setup again closes the previously opened files.
func Setup(cfg Config) error { return global.setup(cfg) }

// Close releases resources held by the global logger (flushes and closes open files).
func Close() { global.close() }

// DebugWriter returns a writer for protocol-level trace output (ACP JSON, IM bridge).
// Returns nil when Level > LevelDebug, effectively disabling trace logging.
func DebugWriter() io.Writer {
	global.mu.Lock()
	defer global.mu.Unlock()
	return global.debugOut
}

// SetOutput overrides the operational output writer.
// Intended for use in tests to capture log output.
// Callers are responsible for restoring (typically via defer).
func SetOutput(w io.Writer) {
	global.mu.Lock()
	defer global.mu.Unlock()
	global.out = w
}

// Debug emits a message at debug level.
func Debug(format string, args ...any) { global.emit(LevelDebug, format, args...) }

// Info emits a message at info level.
func Info(format string, args ...any) { global.emit(LevelInfo, format, args...) }

// Warn emits a message at warn level.
func Warn(format string, args ...any) { global.emit(LevelWarn, format, args...) }

// Error emits a message at error level.
func Error(format string, args ...any) { global.emit(LevelError, format, args...) }

// ---- internal ----

type inst struct {
	mu        sync.Mutex
	level     Level
	out       io.Writer // operational destination (stderr, or MultiWriter(stderr, file))
	debugOut  io.Writer // protocol-trace writer; nil when debug disabled
	opFile    *os.File
	debugFile *os.File
}

var levelTag = [4]string{"DEBUG", "INFO ", "WARN ", "ERROR"}

func (l *inst) emit(lvl Level, format string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if lvl < l.level {
		return
	}
	ts := time.Now().Format("2006/01/02 15:04:05")
	msg := fmt.Sprintf(format, args...)
	line := fmt.Sprintf("%s %s %s\n", ts, levelTag[lvl], msg)
	_, _ = io.WriteString(l.out, line)
}

func (l *inst) setup(cfg Config) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.level = cfg.Level

	// Operational log file — appends across restarts.
	if cfg.LogFile != "" {
		if err := os.MkdirAll(filepath.Dir(cfg.LogFile), 0o755); err != nil {
			return fmt.Errorf("logger: mkdir %q: %w", filepath.Dir(cfg.LogFile), err)
		}
		f, err := os.OpenFile(cfg.LogFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return fmt.Errorf("logger: open log file %q: %w", cfg.LogFile, err)
		}
		if l.opFile != nil {
			_ = l.opFile.Close()
		}
		l.opFile = f
		l.out = io.MultiWriter(os.Stderr, f)
	}

	// Protocol-trace debug writer (only active when level == Debug).
	if cfg.Level <= LevelDebug {
		if cfg.LogFile != "" {
			// Write trace to wheelmaker.debug.log in the same directory as the main log.
			dbgPath := filepath.Join(filepath.Dir(cfg.LogFile), "wheelmaker.debug.log")
			// Truncated each startup so one run's trace doesn't bleed into the next.
			f, err := os.OpenFile(dbgPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
			if err != nil {
				return fmt.Errorf("logger: open debug log %q: %w", dbgPath, err)
			}
			if l.debugFile != nil {
				_ = l.debugFile.Close()
			}
			l.debugFile = f
			l.debugOut = f
		} else {
			// No log file configured — trace goes to the same output as operational logs.
			l.debugOut = l.out
		}
	} else {
		// Level > Debug: disable trace writer.
		l.debugOut = nil
	}

	return nil
}

func (l *inst) close() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.debugFile != nil {
		_ = l.debugFile.Close()
		l.debugFile = nil
		l.debugOut = nil
	}
	if l.opFile != nil {
		_ = l.opFile.Close()
		l.opFile = nil
		l.out = os.Stderr
	}
}
