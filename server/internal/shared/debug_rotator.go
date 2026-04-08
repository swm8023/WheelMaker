package shared

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

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

