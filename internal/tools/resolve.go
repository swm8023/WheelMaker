// Package tools provides utilities for locating third-party tool binaries.
package tools

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// ResolveBinary locates a tool binary using the following priority:
//  1. configPath, if non-empty and the file exists
//  2. bin/{GOOS}_{GOARCH}/{name}[.exe] relative to the executable's directory
//  3. PATH lookup
//
// Returns an error if the binary cannot be found by any method.
func ResolveBinary(name, configPath string) (string, error) {
	// 1. Explicit config path.
	if configPath != "" {
		if _, err := os.Stat(configPath); err == nil {
			abs, err := filepath.Abs(configPath)
			if err != nil {
				return "", fmt.Errorf("tools: abs path %q: %w", configPath, err)
			}
			return abs, nil
		}
	}

	// 2. bin/{GOOS}_{GOARCH}/ relative to the executable.
	exeDir, err := executableDir()
	if err == nil {
		candidate := filepath.Join(exeDir, "bin", platformDir(), binaryName(name))
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}

	// Fallback: also check relative to the working directory (useful during `go run`).
	candidate := filepath.Join("bin", platformDir(), binaryName(name))
	if _, err := os.Stat(candidate); err == nil {
		abs, err := filepath.Abs(candidate)
		if err == nil {
			return abs, nil
		}
	}

	// 3. PATH.
	path, err := exec.LookPath(name)
	if err == nil {
		return path, nil
	}

	return "", fmt.Errorf(
		"tools: %q not found (tried config path, bin/%s/, and PATH); "+
			"run scripts/install-tools.sh to download it",
		name, platformDir(),
	)
}

// platformDir returns the platform-specific subdirectory name, e.g. "windows_amd64".
func platformDir() string {
	return runtime.GOOS + "_" + runtime.GOARCH
}

// binaryName appends ".exe" on Windows.
func binaryName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

// executableDir returns the directory containing the running executable.
func executableDir() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Dir(exe), nil
}
