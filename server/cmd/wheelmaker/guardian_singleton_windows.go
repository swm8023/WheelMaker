//go:build windows

package main

import (
	"errors"
	"fmt"
	"syscall"

	"golang.org/x/sys/windows"
)

const guardianMutexName = "Global\\WheelMakerGuardianSingleton"
const waitTimeoutStatus uint32 = 0x00000102

var errGuardianAlreadyRunning = errors.New("wheelmaker guardian is already running")

func acquireGuardianSingleton() (func(), error) {
	namePtr, err := syscall.UTF16PtrFromString(guardianMutexName)
	if err != nil {
		return nil, fmt.Errorf("guardian mutex name: %w", err)
	}
	handle, err := windows.CreateMutex(nil, false, namePtr)
	if err != nil {
		return nil, fmt.Errorf("create guardian mutex: %w", err)
	}

	waitStatus, waitErr := windows.WaitForSingleObject(handle, 0)
	if waitErr != nil {
		_ = windows.CloseHandle(handle)
		return nil, fmt.Errorf("wait guardian mutex: %w", waitErr)
	}
	if waitStatus == waitTimeoutStatus {
		_ = windows.CloseHandle(handle)
		return nil, errGuardianAlreadyRunning
	}
	if waitStatus != windows.WAIT_OBJECT_0 && waitStatus != windows.WAIT_ABANDONED {
		_ = windows.CloseHandle(handle)
		return nil, fmt.Errorf("unexpected guardian mutex wait status: %d", waitStatus)
	}

	release := func() {
		_ = windows.ReleaseMutex(handle)
		_ = windows.CloseHandle(handle)
	}
	return release, nil
}
