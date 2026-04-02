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

	logger "github.com/swm8023/wheelmaker/internal/shared"
)

const guardianInterval = 30 * time.Second

type daemonProcess struct {
	PID int
}

type workerSpec struct {
	name       string
	markerFlag string
	args       []string
	keepPID    int
}

func runGuardian(workerArgs []string) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	return runGuardianWithContext(ctx, workerArgs)
}

func runGuardianWithContext(ctx context.Context, workerArgs []string) error {
	releaseSingleton, err := acquireGuardianSingleton()
	if err != nil {
		return err
	}
	defer releaseSingleton()

	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable path: %w", err)
	}
	exeName := filepath.Base(exePath)
	baseArgs := sanitizeWorkerArgs(workerArgs)

	specs := []*workerSpec{
		{name: "hub", markerFlag: hubWorkerArg, args: append(append([]string{}, baseArgs...), hubWorkerArg)},
		{name: "registry", markerFlag: registryWorkerArg, args: append(append([]string{}, baseArgs...), registryWorkerArg)},
	}

	reconcile := func() {
		for _, spec := range specs {
			pid, recErr := reconcileWorkers(exePath, exeName, spec.markerFlag, spec.args, spec.keepPID)
			if recErr != nil {
				fmt.Fprintf(os.Stderr, "wheelmaker guardian[%s]: %v\n", spec.name, recErr)
				continue
			}
			spec.keepPID = pid
		}
	}

	reconcile()
	ticker := time.NewTicker(guardianInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			shutdownWorkers(exeName, specs)
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
		case "-d", daemonWorkerArg, hubWorkerArg, registryWorkerArg:
			continue
		default:
			out = append(out, arg)
		}
	}
	return out
}

func reconcileWorkers(exePath, exeName, markerFlag string, workerArgs []string, preferredPID int) (int, error) {
	workers, err := listWorkerProcesses(exeName, markerFlag)
	if err != nil {
		if preferredPID > 0 {
			logger.Warn("[daemon] list %s workers failed, keep previous pid=%d: %v", markerFlag, preferredPID, err)
			return preferredPID, nil
		}
		pid, startErr := startWorker(exePath, workerArgs)
		if startErr != nil {
			return 0, fmt.Errorf("list workers failed: %w; start fallback failed: %v", err, startErr)
		}
		logger.Warn("[daemon] list %s workers failed; started fallback pid=%d: %v", markerFlag, pid, err)
		return pid, nil
	}
	if len(workers) == 0 {
		pid, startErr := startWorker(exePath, workerArgs)
		if startErr != nil {
			return 0, startErr
		}
		logger.Info("[daemon] started %s worker pid=%d", markerFlag, pid)
		return pid, nil
	}

	keepPID := chooseKeepPID(workers, preferredPID)
	for _, proc := range workers {
		if proc.PID == keepPID {
			continue
		}
		if killErr := killProcess(proc.PID); killErr != nil {
			logger.Warn("[daemon] failed to stop extra %s worker pid=%d: %v", markerFlag, proc.PID, killErr)
			continue
		}
		logger.Warn("[daemon] stopped extra %s worker pid=%d", markerFlag, proc.PID)
	}
	return keepPID, nil
}

func shutdownWorkers(exeName string, specs []*workerSpec) {
	for _, spec := range specs {
		workers, err := listWorkerProcesses(exeName, spec.markerFlag)
		if err != nil {
			logger.Warn("[daemon] shutdown list %s workers failed: %v", spec.markerFlag, err)
			continue
		}
		for _, proc := range workers {
			if killErr := killProcess(proc.PID); killErr != nil {
				logger.Warn("[daemon] shutdown stop %s worker pid=%d failed: %v", spec.markerFlag, proc.PID, killErr)
				continue
			}
			logger.Info("[daemon] shutdown stopped %s worker pid=%d", spec.markerFlag, proc.PID)
		}
	}
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
	restoreIO, err := configureWorkerCommandIO(cmd)
	if err != nil {
		return 0, err
	}
	if err := cmd.Start(); err != nil {
		restoreIO()
		return 0, fmt.Errorf("start worker: %w", err)
	}
	restoreIO()
	return cmd.Process.Pid, nil
}

func configureWorkerCommandIO(cmd *exec.Cmd) (func(), error) {
	stdoutSink, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		return nil, fmt.Errorf("open stdout sink: %w", err)
	}
	stderrSink, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		_ = stdoutSink.Close()
		return nil, fmt.Errorf("open stderr sink: %w", err)
	}
	cmd.Stdout = stdoutSink
	cmd.Stderr = stderrSink
	return func() {
		_ = stdoutSink.Close()
		_ = stderrSink.Close()
	}, nil
}

func redirectProcessStdioToDevNull() (func(), error) {
	oldStdout := os.Stdout
	oldStderr := os.Stderr

	stdoutSink, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		return nil, fmt.Errorf("open process stdout sink: %w", err)
	}
	stderrSink, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		_ = stdoutSink.Close()
		return nil, fmt.Errorf("open process stderr sink: %w", err)
	}

	os.Stdout = stdoutSink
	os.Stderr = stderrSink

	return func() {
		os.Stdout = oldStdout
		os.Stderr = oldStderr
		_ = stdoutSink.Close()
		_ = stderrSink.Close()
	}, nil
}

func killProcess(pid int) error {
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return p.Kill()
}
