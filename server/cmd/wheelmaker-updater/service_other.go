//go:build !windows

package main

import "context"

func runAsWindowsServiceIfNeeded(_ UpdaterConfig) (bool, error) {
	return false, nil
}

func runUpdaterService(_ context.Context, cfg UpdaterConfig) error {
	return RunUpdater(context.Background(), cfg)
}
