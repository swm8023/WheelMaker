//go:build windows

package winsvc

import (
	"context"
	"fmt"

	"golang.org/x/sys/windows/svc"
)

type Runner func(context.Context) error
type IgnoreError func(error) bool

type genericService struct {
	run    Runner
	ignore IgnoreError
}

func RunIfWindowsService(serviceName string, run Runner, ignore IgnoreError) (bool, error) {
	isService, err := svc.IsWindowsService()
	if err != nil {
		return false, fmt.Errorf("detect windows service mode: %w", err)
	}
	if !isService {
		return false, nil
	}
	handler := &genericService{run: run, ignore: ignore}
	if err := svc.Run(serviceName, handler); err != nil {
		return true, fmt.Errorf("run windows service %s: %w", serviceName, err)
	}
	return true, nil
}

func (s *genericService) Execute(_ []string, req <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	const accepts = svc.AcceptStop | svc.AcceptShutdown
	status <- svc.Status{State: svc.StartPending}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- s.run(ctx)
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
				if err := <-done; !s.isIgnored(err) {
					serviceCode = 1
				}
				status <- svc.Status{State: svc.Stopped}
				return false, serviceCode
			}
		case err := <-done:
			if !s.isIgnored(err) {
				serviceCode = 1
			}
			status <- svc.Status{State: svc.Stopped}
			return false, serviceCode
		}
	}
}

func (s *genericService) isIgnored(err error) bool {
	if err == nil {
		return true
	}
	if s.ignore == nil {
		return false
	}
	return s.ignore(err)
}

