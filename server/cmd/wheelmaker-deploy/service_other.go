//go:build !windows && !darwin && !linux

package main

import (
	"context"
	"errors"
)

type serviceManager struct{}

func newServiceManager(deployConfig, commandRunner) serviceManager { return serviceManager{} }

func (serviceManager) CheckDeployPrerequisites(context.Context) error {
	return errors.New("services are unsupported on this platform")
}

func (serviceManager) Configure(context.Context) error {
	return errors.New("services are unsupported on this platform")
}

func (serviceManager) Start(context.Context, bool) error {
	return errors.New("services are unsupported on this platform")
}

func (serviceManager) Stop(context.Context, bool) error {
	return errors.New("services are unsupported on this platform")
}

func (serviceManager) Restart(context.Context, bool) error {
	return errors.New("services are unsupported on this platform")
}

func (serviceManager) Status(context.Context) error {
	return errors.New("services are unsupported on this platform")
}
