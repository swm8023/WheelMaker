package main

import (
	"context"

	"github.com/swm8023/wheelmaker/internal/shared/winsvc"
)

func runAsWindowsServiceIfNeeded(cfg UpdaterConfig) (bool, error) {
	return winsvc.RunIfWindowsService(
		updaterServiceName,
		func(ctx context.Context) error {
			return runUpdaterService(ctx, cfg)
		},
		nil,
	)
}

func runUpdaterService(ctx context.Context, cfg UpdaterConfig) error {
	return RunUpdater(ctx, cfg)
}
