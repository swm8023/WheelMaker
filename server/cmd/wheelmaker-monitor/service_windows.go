//go:build windows

package main

import (
	"context"
	"fmt"
	"net/http"

	"golang.org/x/sys/windows/svc"
)

type monitorService struct {
	addr    string
	handler http.Handler
}

func runAsWindowsServiceIfNeeded(addr string, handler http.Handler) (bool, error) {
	isService, err := svc.IsWindowsService()
	if err != nil {
		return false, fmt.Errorf("detect windows service mode: %w", err)
	}
	if !isService {
		return false, nil
	}
	s := &monitorService{addr: addr, handler: handler}
	if err := svc.Run(monitorServiceName, s); err != nil {
		return true, fmt.Errorf("run windows service %s: %w", monitorServiceName, err)
	}
	return true, nil
}

func (s *monitorService) Execute(_ []string, req <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	const accepts = svc.AcceptStop | svc.AcceptShutdown
	status <- svc.Status{State: svc.StartPending}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- runHTTPServer(ctx, s.addr, s.handler)
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
				if err := <-done; err != nil && err != http.ErrServerClosed {
					serviceCode = 1
				}
				status <- svc.Status{State: svc.Stopped}
				return false, serviceCode
			}
		case err := <-done:
			if err != nil && err != http.ErrServerClosed {
				serviceCode = 1
			}
			status <- svc.Status{State: svc.Stopped}
			return false, serviceCode
		}
	}
}
