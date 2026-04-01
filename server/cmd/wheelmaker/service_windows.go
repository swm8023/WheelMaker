//go:build windows

package main

import (
	"context"

	"github.com/swm8023/wheelmaker/internal/shared/winsvc"
)

const wheelmakerWindowsServiceName = "WheelMaker"

func runAsWindowsServiceIfNeeded(workerArgs []string) (bool, error) {
	sanitizedArgs := sanitizeWorkerArgs(workerArgs)
	return winsvc.RunIfWindowsService(
		wheelmakerWindowsServiceName,
		func(ctx context.Context) error {
			return runGuardianWithContext(ctx, sanitizedArgs)
		},
		nil,
	)
}
