//go:build windows

package main

import (
	"context"
	"net/http"

	"github.com/swm8023/wheelmaker/internal/shared/winsvc"
)

func runAsWindowsServiceIfNeeded(addr string, handler http.Handler) (bool, error) {
	return winsvc.RunIfWindowsService(
		monitorServiceName,
		func(ctx context.Context) error {
			return runHTTPServer(ctx, addr, handler)
		},
		func(err error) bool { return err == http.ErrServerClosed },
	)
}
