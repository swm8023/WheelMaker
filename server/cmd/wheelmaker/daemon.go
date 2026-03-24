package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"slices"
	"syscall"
	"time"

	"github.com/swm8023/wheelmaker/internal/logger"
)

const guardianInterval = 30 * time.Second

type daemonProcess struct {
	PID int
}

func runGuardian(workerArgs []string) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable path: %w", err)
	}
	exeName := filepath.Base(exePath)
	args := sanitizeWorkerArgs(workerArgs)
	args = append(args, daemonWorkerArg)

	var keepPID int
	reconcile := func() {
		pid, recErr := reconcileWorkers(exePath, exeName, args, keepPID)
		if recErr != nil {
			fmt.Fprintf(os.Stderr, "wheelmaker guardian: %v\n", recErr)
			return
		}
		keepPID = pid
	}

	reconcile()
	ticker := time.NewTicker(guardianInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			reconcile()
		}
	}
}

func sanitizeWorkerArgs(args []string) []string {
	out := make([]string, 0, len(args))
	for _, arg := range args {
		switch arg {
		case "-d", "--daemon-worker":
			continue
		default:
			out = append(out, arg)
		}
	}
	return out
}

func reconcileWorkers(exePath, exeName string, workerArgs []string, preferredPID int) (int, error) {
	workers, err := listWorkerProcesses(exeName)
	if err != nil {
		return 0, err
	}
	if len(workers) == 0 {
		pid, startErr := startWorker(exePath, workerArgs)
		if startErr != nil {
			return 0, startErr
		}
		logger.Info("[daemon] started worker pid=%d", pid)
		return pid, nil
	}

	keepPID := chooseKeepPID(workers, preferredPID)
	for _, proc := range workers {
		if proc.PID == keepPID {
			continue
		}
		if killErr := killProcess(proc.PID); killErr != nil {
			logger.Warn("[daemon] failed to stop extra worker pid=%d: %v", proc.PID, killErr)
			continue
		}
		logger.Warn("[daemon] stopped extra worker pid=%d", proc.PID)
	}
	return keepPID, nil
}

func chooseKeepPID(workers []daemonProcess, preferredPID int) int {
	if preferredPID > 0 {
		for _, p := range workers {
			if p.PID == preferredPID {
				return preferredPID
			}
		}
	}
	slices.SortFunc(workers, func(a, b daemonProcess) int {
		if a.PID < b.PID {
			return -1
		}
		if a.PID > b.PID {
			return 1
		}
		return 0
	})
	return workers[0].PID
}

func startWorker(exePath string, workerArgs []string) (int, error) {
	cmd := exec.Command(exePath, workerArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("start worker: %w", err)
	}
	return cmd.Process.Pid, nil
}

func killProcess(pid int) error {
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return p.Kill()
}
