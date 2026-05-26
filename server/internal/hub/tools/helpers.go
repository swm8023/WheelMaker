package tools

import "os"

func fileExists(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

func maxInt64(v int64, fallback int64) int64 {
	if v > fallback {
		return v
	}
	return fallback
}
