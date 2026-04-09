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

const (
	logTimeLayout = "2006/01/02 15:04:05"
	logDayLayout  = "2006-01-02"
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
	switch strings.ToLower(strings.TrimSpace(s)) {
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

	out := l.targetWriterLocked(lvl)
	if out == nil {
		return
	}
	_, _ = io.WriteString(out, formatLogLine(lvl, format, args...))
}

func (l *loggerInst) setup(cfg LoggerConfig) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.resetWritersLocked()
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
			l.debugOut = newDebugDailyRotator(dbgPath, 1, time.Now)
		}
	}

	return nil
}

func (l *loggerInst) close() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.resetWritersLocked()
	l.out = os.Stderr
}

func (l *loggerInst) resetWritersLocked() {
	if c, ok := l.debugOut.(io.Closer); ok {
		_ = c.Close()
	}
	l.debugOut = nil
	if l.opFile != nil {
		_ = l.opFile.Close()
		l.opFile = nil
	}
}

func (l *loggerInst) targetWriterLocked(lvl Level) io.Writer {
	if lvl == LevelDebug {
		if l.level > LevelDebug {
			return nil
		}
		return l.debugOut
	}
	if lvl < l.level {
		return nil
	}
	return l.out
}

func formatLogLine(lvl Level, format string, args ...any) string {
	ts := time.Now().Format(logTimeLayout)
	msg := fmt.Sprintf(format, args...)
	var b strings.Builder
	b.Grow(len(ts) + 1 + len(levelTag[lvl]) + 1 + len(msg) + 1)
	b.WriteString(ts)
	b.WriteByte(' ')
	b.WriteString(levelTag[lvl])
	b.WriteByte(' ')
	b.WriteString(msg)
	b.WriteByte('\n')
	return b.String()
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
		keepDays = 1
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
	day := r.now().Format(logDayLayout)
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

	cutoffDay := r.now().AddDate(0, 0, -r.keepDays).Format(logDayLayout)
	prefix := base + "."
	for _, path := range matches {
		name := filepath.Base(path)
		if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, ext) {
			continue
		}
		day := strings.TrimSuffix(strings.TrimPrefix(name, prefix), ext)
		if _, err := time.Parse(logDayLayout, day); err != nil {
			continue
		}
		if day < cutoffDay {
			_ = os.Remove(path)
		}
	}
	return nil
}
