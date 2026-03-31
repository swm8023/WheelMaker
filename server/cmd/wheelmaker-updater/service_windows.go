//go:build windows

package main

import (
	"context"
	"fmt"

	"golang.org/x/sys/windows/svc"
)

type updaterService struct {
	cfg UpdaterConfig
}

func runAsWindowsServiceIfNeeded(cfg UpdaterConfig) (bool, error) {
	isService, err := svc.IsWindowsService()
	if err != nil {
		return false, fmt.Errorf("detect windows service mode: %w", err)
	}
	if !isService {
		return false, nil
	}
	handler := &updaterService{cfg: cfg}
	if err := svc.Run(updaterServiceName, handler); err != nil {
		return true, fmt.Errorf("run windows service %s: %w", updaterServiceName, err)
	}
	return true, nil
}

func (s *updaterService) Execute(_ []string, req <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	const accepts = svc.AcceptStop | svc.AcceptShutdown
	status <- svc.Status{State: svc.StartPending}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- runUpdaterService(ctx, s.cfg)
	}()

	status <- svc.Status{State: svc.Running, Accepts: accepts}
	var serviceCode uint32

	for {
		select {
		case change := <-req:
			switch change.Cmd {
			case svc.Interrogate:
				status <- change.CurrentStatus
			case svc.Stop, svc.Shutdown:
				status <- svc.Status{State: svc.StopPending}
				cancel()
				if err := <-done; err != nil {
					serviceCode = 1
				}
				status <- svc.Status{State: svc.Stopped}
				return false, serviceCode
			}
		case err := <-done:
			if err != nil {
				serviceCode = 1
			}
			status <- svc.Status{State: svc.Stopped}
			return false, serviceCode
		}
	}
}

func runUpdaterService(ctx context.Context, cfg UpdaterConfig) error {
	return RunUpdater(ctx, cfg)
}
