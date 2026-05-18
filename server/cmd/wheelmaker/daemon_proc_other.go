//go:build !windows

package main

import (
	"fmt"
	"os/exec"
)

func listWorkerProcesses(exeName, markerFlag string) ([]daemonProcess, error) {
	// On Unix-like systems, scan the process table and match both executable
	// name and daemon worker marker so we only supervise daemon-managed workers.
	cmd := exec.Command("ps", "-eo", "pid=,comm=,args=")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("list workers: %w", err)
	}
	return parseWorkerProcessesFromPS(out, exeName, markerFlag)
}
