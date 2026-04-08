package shared

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
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
)

// ParseLevel converts a string to Level. Unknown values default to LevelWarn.
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

// LoggerConfig holds logger setup parameters.
type LoggerConfig struct {
	Level        Level
	LogFile      string
	DebugLogFile string
}

var global = &loggerInst{
	level: LevelWarn,
	out:   os.Stderr,
}

func Setup(cfg LoggerConfig) error { return global.setup(cfg) }
func Close()                       { global.close() }

func DebugWriter() io.Writer {
	global.mu.Lock()
	defer global.mu.Unlock()
	return global.debugOut
}

func SetOutput(w io.Writer) {
	global.mu.Lock()
	defer global.mu.Unlock()
	global.out = w
}

func Debug(format string, args ...any) { global.emit(LevelDebug, format, args...) }
func Info(format string, args ...any)  { global.emit(LevelInfo, format, args...) }
func Warn(format string, args ...any)  { global.emit(LevelWarn, format, args...) }
func Error(format string, args ...any) { global.emit(LevelError, format, args...) }

type loggerInst struct {
	mu        sync.Mutex
	level     Level
	out       io.Writer
	debugOut  io.Writer
	opFile    *os.File
}

var levelTag = [4]string{"DEBUG", "INFO ", "WARN ", "ERROR"}

func (l *loggerInst) emit(lvl Level, format string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()

	ts := time.Now().Format("2006/01/02 15:04:05")
	msg := fmt.Sprintf(format, args...)
	line := fmt.Sprintf("%s %s %s\n", ts, levelTag[lvl], msg)

	if lvl == LevelDebug {
		if l.level <= LevelDebug && l.debugOut != nil {
			_, _ = io.WriteString(l.debugOut, line)
		}
		return
	}
	if lvl < l.level {
		return
	}
	_, _ = io.WriteString(l.out, line)
}

func (l *loggerInst) setup(cfg LoggerConfig) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.level = cfg.Level
	l.out = os.Stderr

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

	if cfg.Level <= LevelDebug {
		dbgPath := cfg.DebugLogFile
		if dbgPath == "" && cfg.LogFile != "" {
			dbgPath = filepath.Join(filepath.Dir(cfg.LogFile), "wheelmaker.debug.log")
		}
		if dbgPath != "" {
			if err := os.MkdirAll(filepath.Dir(dbgPath), 0o755); err != nil {
				return fmt.Errorf("logger: mkdir %q: %w", filepath.Dir(dbgPath), err)
			}
			if c, ok := l.debugOut.(io.Closer); ok {
				_ = c.Close()
			}
			l.debugOut = newDebugDailyRotator(dbgPath, 7, time.Now)
		} else {
			if c, ok := l.debugOut.(io.Closer); ok {
				_ = c.Close()
			}
			l.debugOut = nil
		}
	} else {
		if c, ok := l.debugOut.(io.Closer); ok {
			_ = c.Close()
		}
		l.debugOut = nil
	}

	return nil
}

func (l *loggerInst) close() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if c, ok := l.debugOut.(io.Closer); ok {
		_ = c.Close()
	}
	l.debugOut = nil
	if l.opFile != nil {
		_ = l.opFile.Close()
		l.opFile = nil
		l.out = os.Stderr
	}
}
