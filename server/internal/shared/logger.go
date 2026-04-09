package shared

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
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
	mu       sync.Mutex
	level    Level
	out      io.Writer
	debugOut io.Writer
	opFile   *os.File
}

var levelTag = [4]string{"DEBUG", "INFO ", "WARN ", "ERROR"}

func (l *loggerInst) emit(lvl Level, format string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if lvl == LevelDebug {
		if l.level > LevelDebug || l.debugOut == nil {
			return
		}
		ts := time.Now().Format("2006/01/02 15:04:05")
		msg := fmt.Sprintf(format, args...)
		line := fmt.Sprintf("%s %s %s\n", ts, levelTag[lvl], msg)
		_, _ = io.WriteString(l.debugOut, line)
		return
	}
	if lvl < l.level {
		return
	}
	ts := time.Now().Format("2006/01/02 15:04:05")
	msg := fmt.Sprintf(format, args...)
	line := fmt.Sprintf("%s %s %s\n", ts, levelTag[lvl], msg)
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
			dbgPath = deriveDebugLogPath(cfg.LogFile)
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

func deriveDebugLogPath(logPath string) string {
	path := strings.TrimSpace(logPath)
	if path == "" {
		return ""
	}
	ext := filepath.Ext(path)
	if ext == "" {
		return path + ".debug.log"
	}
	base := strings.TrimSuffix(path, ext)
	return base + ".debug" + ext
}

type debugDailyRotator struct {
	mu        sync.Mutex
	basePath  string
	keepDays  int
	now       func() time.Time
	file      *os.File
	dayString string
}

func newDebugDailyRotator(path string, keepDays int, now func() time.Time) *debugDailyRotator {
	if keepDays <= 0 {
		keepDays = 7
	}
	if now == nil {
		now = time.Now
	}
	return &debugDailyRotator{
		basePath: path,
		keepDays: keepDays,
		now:      now,
	}
}

func (r *debugDailyRotator) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.rotateIfNeededLocked(); err != nil {
		return 0, err
	}
	n, err := r.file.Write(p)
	if err != nil {
		return n, err
	}
	if syncErr := r.file.Sync(); syncErr != nil {
		return n, syncErr
	}
	return n, nil
}

func (r *debugDailyRotator) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.file == nil {
		return nil
	}
	err := r.file.Close()
	r.file = nil
	return err
}

func (r *debugDailyRotator) rotateIfNeededLocked() error {
	day := r.now().Format("2006-01-02")
	if r.file != nil && day == r.dayString {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(r.basePath), 0o755); err != nil {
		return err
	}

	if r.file != nil {
		_ = r.file.Close()
		archive := r.archivePath(r.dayString)
		_ = os.Remove(archive)
		if err := os.Rename(r.basePath, archive); err != nil && !os.IsNotExist(err) {
			return err
		}
	}

	f, err := os.OpenFile(r.basePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	r.file = f
	r.dayString = day

	return r.cleanupOldArchives()
}

func (r *debugDailyRotator) archivePath(day string) string {
	ext := filepath.Ext(r.basePath)
	base := strings.TrimSuffix(r.basePath, ext)
	return fmt.Sprintf("%s.%s%s", base, day, ext)
}

func (r *debugDailyRotator) cleanupOldArchives() error {
	if r.keepDays <= 0 {
		return nil
	}

	ext := filepath.Ext(r.basePath)
	base := strings.TrimSuffix(filepath.Base(r.basePath), ext)
	glob := filepath.Join(filepath.Dir(r.basePath), base+".*"+ext)

	matches, err := filepath.Glob(glob)
	if err != nil {
		return err
	}

	cutoff := r.now().AddDate(0, 0, -r.keepDays)
	prefix := base + "."
	for _, path := range matches {
		name := filepath.Base(path)
		if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, ext) {
			continue
		}
		day := strings.TrimSuffix(strings.TrimPrefix(name, prefix), ext)
		d, err := time.Parse("2006-01-02", day)
		if err != nil {
			continue
		}
		if d.Before(cutoff) {
			_ = os.Remove(path)
		}
	}
	return nil
}
