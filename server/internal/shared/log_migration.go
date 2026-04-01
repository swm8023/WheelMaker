package shared

import (
	"fmt"
	"os"
	"path/filepath"
)

// MigrateLegacyLogFile moves an old log file into the new target path when safe.
// It only migrates when legacy exists and target does not exist yet.
func MigrateLegacyLogFile(legacyPath, targetPath string) error {
	legacy := filepath.Clean(legacyPath)
	target := filepath.Clean(targetPath)
	if legacy == "" || target == "" || legacy == target {
		return nil
	}
	info, err := os.Stat(legacy)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("log migration stat legacy %q: %w", legacy, err)
	}
	if info.IsDir() {
		return nil
	}
	if _, err := os.Stat(target); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("log migration stat target %q: %w", target, err)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("log migration mkdir %q: %w", filepath.Dir(target), err)
	}
	if err := os.Rename(legacy, target); err != nil {
		return fmt.Errorf("log migration move %q -> %q: %w", legacy, target, err)
	}
	return nil
}
