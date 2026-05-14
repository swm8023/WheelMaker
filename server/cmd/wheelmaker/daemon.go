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

	"github.com/swm8023/wheelmaker/internal/shared"
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

	registryCfg, err := loadGuardianRegistryConfig()
	if err != nil {
		return err
	}
	specs := guardianWorkerSpecs(baseArgs, registryCfg)

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

func loadGuardianRegistryConfig() (shared.RegistryConfig, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return shared.RegistryConfig{}, fmt.Errorf("home dir: %w", err)
	}
	cfgPath := filepath.Join(home, ".wheelmaker", "config.json")
	cfg, err := shared.LoadConfig(cfgPath)
	if err != nil {
		return shared.RegistryConfig{}, fmt.Errorf("cannot load config.json at %s: %w", cfgPath, err)
	}
	return cfg.Registry, nil
}

func guardianWorkerSpecs(baseArgs []string, registryCfg shared.RegistryConfig) []*workerSpec {
	specs := []*workerSpec{
		{name: "hub", markerFlag: hubWorkerArg, args: append(append([]string{}, baseArgs...), hubWorkerArg)},
	}
	if registryCfg.Listen {
		specs = append(specs, &workerSpec{name: "registry", markerFlag: registryWorkerArg, args: append(append([]string{}, baseArgs...), registryWorkerArg)})
	}
	return specs
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
			hubScopedLogger.Warn("daemon list %s workers failed keep_previous_pid=%d err=%v", markerFlag, preferredPID, err)
			return preferredPID, nil
		}
		pid, startErr := startWorker(exePath, workerArgs)
		if startErr != nil {
			return 0, fmt.Errorf("list workers failed: %w; start fallback failed: %v", err, startErr)
		}
		hubScopedLogger.Warn("daemon list %s workers failed started_fallback_pid=%d err=%v", markerFlag, pid, err)
		return pid, nil
	}
	if len(workers) == 0 {
		pid, startErr := startWorker(exePath, workerArgs)
		if startErr != nil {
			return 0, startErr
		}
		hubScopedLogger.Info("daemon started %s worker pid=%d", markerFlag, pid)
		return pid, nil
	}

	keepPID := chooseKeepPID(workers, preferredPID)
	for _, proc := range workers {
		if proc.PID == keepPID {
			continue
		}
		if killErr := killProcess(proc.PID); killErr != nil {
			hubScopedLogger.Warn("daemon failed to stop extra %s worker pid=%d err=%v", markerFlag, proc.PID, killErr)
			continue
		}
		hubScopedLogger.Warn("daemon stopped extra %s worker pid=%d", markerFlag, proc.PID)
	}
	return keepPID, nil
}

func shutdownWorkers(exeName string, specs []*workerSpec) {
	for _, spec := range specs {
		workers, err := listWorkerProcesses(exeName, spec.markerFlag)
		if err != nil {
			hubScopedLogger.Warn("daemon shutdown list %s workers failed err=%v", spec.markerFlag, err)
			continue
		}
		for _, proc := range workers {
			if killErr := killProcess(proc.PID); killErr != nil {
				hubScopedLogger.Warn("daemon shutdown stop %s worker pid=%d failed err=%v", spec.markerFlag, proc.PID, killErr)
				continue
			}
			hubScopedLogger.Info("daemon shutdown stopped %s worker pid=%d", spec.markerFlag, proc.PID)
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
