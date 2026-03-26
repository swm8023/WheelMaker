//go:build !windows

package main

import (
	"bufio"
	"bytes"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

func listWorkerProcesses(exeName, markerFlag string) ([]daemonProcess, error) {
	// On Unix-like systems, scan the process table and match both executable
	// name and daemon worker marker so we only supervise daemon-managed workers.
	cmd := exec.Command("ps", "-eo", "pid=,comm=,args=")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("list workers: %w", err)
	}
	base := strings.TrimSpace(filepath.Base(exeName))
	markerFlag = strings.TrimSpace(markerFlag)
	if markerFlag == "" {
		markerFlag = daemonWorkerArg
	}
	var procs []daemonProcess
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		pid, convErr := strconv.Atoi(fields[0])
		if convErr != nil || pid <= 0 {
			continue
		}
		comm := strings.TrimSpace(fields[1])
		args := strings.Join(fields[2:], " ")
		if base != "" && comm != base {
			continue
		}
		if !strings.Contains(args, markerFlag) {
			continue
		}
		procs = append(procs, daemonProcess{PID: pid})
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return procs, nil
}
