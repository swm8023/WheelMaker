package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/swm8023/wheelmaker/internal/shared"
)

const guardianInterval = 30 * time.Second
const workerStopFileEnv = "WHEELMAKER_WORKER_STOP_FILE"

var (
	listWorkerProcessesForDaemon = listWorkerProcesses
	killProcessForDaemon         = killProcess
	workerStopPollInterval       = 200 * time.Millisecond
	workerGracefulStopTimeout    = 10 * time.Second
)

type daemonProcess struct {
	PID int
}

type workerSpec struct {
	name       string
	markerFlag string
	args       []string
	keepPID    int
	stopFile   string
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
			if strings.TrimSpace(spec.stopFile) == "" {
				spec.stopFile = newWorkerStopFile(spec.markerFlag)
			}
			pid, recErr := reconcileWorkers(exePath, exeName, spec.markerFlag, spec.args, spec.keepPID, spec.stopFile)
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

func reconcileWorkers(exePath, exeName, markerFlag string, workerArgs []string, preferredPID int, stopFile string) (int, error) {
	workers, err := listWorkerProcessesForDaemon(exeName, markerFlag)
	if err != nil {
		if preferredPID > 0 {
			hubScopedLogger.Warn("daemon list %s workers failed keep_previous_pid=%d err=%v", markerFlag, preferredPID, err)
			return preferredPID, nil
		}
		pid, startErr := startWorker(exePath, workerArgs, stopFile)
		if startErr != nil {
			return 0, fmt.Errorf("list workers failed: %w; start fallback failed: %v", err, startErr)
		}
		hubScopedLogger.Warn("daemon list %s workers failed started_fallback_pid=%d err=%v", markerFlag, pid, err)
		return pid, nil
	}
	if len(workers) == 0 {
		pid, startErr := startWorker(exePath, workerArgs, stopFile)
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
		if killErr := killProcessForDaemon(proc.PID); killErr != nil {
			hubScopedLogger.Warn("daemon failed to stop extra %s worker pid=%d err=%v", markerFlag, proc.PID, killErr)
			continue
		}
		hubScopedLogger.Warn("daemon stopped extra %s worker pid=%d", markerFlag, proc.PID)
	}
	return keepPID, nil
}

func shutdownWorkers(exeName string, specs []*workerSpec) {
	for _, spec := range specs {
		workers, err := listWorkerProcessesForDaemon(exeName, spec.markerFlag)
		if err != nil {
			hubScopedLogger.Warn("daemon shutdown list %s workers failed err=%v", spec.markerFlag, err)
			continue
		}
		for _, proc := range workers {
			if strings.TrimSpace(spec.stopFile) != "" {
				if err := requestWorkerStop(spec.stopFile); err != nil {
					hubScopedLogger.Warn("daemon shutdown request %s worker pid=%d graceful stop failed err=%v", spec.markerFlag, proc.PID, err)
				} else if waitForWorkerExit(exeName, spec.markerFlag, proc.PID, workerGracefulStopTimeout) {
					hubScopedLogger.Info("daemon shutdown gracefully stopped %s worker pid=%d", spec.markerFlag, proc.PID)
					continue
				} else {
					hubScopedLogger.Warn("daemon shutdown %s worker pid=%d graceful stop timed out", spec.markerFlag, proc.PID)
				}
			}
			if killErr := killProcessForDaemon(proc.PID); killErr != nil {
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

func startWorker(exePath string, workerArgs []string, stopFile string) (int, error) {
	cmd := exec.Command(exePath, workerArgs...)
	stopFile = strings.TrimSpace(stopFile)
	if stopFile != "" {
		_ = os.Remove(stopFile)
		if err := os.MkdirAll(filepath.Dir(stopFile), 0o755); err != nil {
			return 0, fmt.Errorf("create worker stop dir: %w", err)
		}
		cmd.Env = append(os.Environ(), workerStopFileEnv+"="+stopFile)
	}
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

func newWorkerStopFile(markerFlag string) string {
	name := strings.Trim(strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r
		case r >= 'A' && r <= 'Z':
			return r
		case r >= '0' && r <= '9':
			return r
		default:
			return '-'
		}
	}, markerFlag), "-")
	if name == "" {
		name = "worker"
	}
	return filepath.Join(os.TempDir(), "wheelmaker", fmt.Sprintf("%s-%d-%d.stop", name, os.Getpid(), time.Now().UnixNano()))
}

func requestWorkerStop(stopFile string) error {
	stopFile = strings.TrimSpace(stopFile)
	if stopFile == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(stopFile), 0o755); err != nil {
		return err
	}
	return os.WriteFile(stopFile, []byte("stop\n"), 0o644)
}

func waitForWorkerExit(exeName, markerFlag string, pid int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		workers, err := listWorkerProcessesForDaemon(exeName, markerFlag)
		if err != nil {
			return false
		}
		if !hasDaemonProcess(workers, pid) {
			return true
		}
		if !time.Now().Before(deadline) {
			return false
		}
		time.Sleep(workerStopPollInterval)
	}
}

func hasDaemonProcess(workers []daemonProcess, pid int) bool {
	for _, worker := range workers {
		if worker.PID == pid {
			return true
		}
	}
	return false
}

func workerContextWithStopFile(parent context.Context, stopFile string, pollInterval time.Duration) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)
	stopFile = strings.TrimSpace(stopFile)
	if stopFile == "" {
		return ctx, cancel
	}
	if pollInterval <= 0 {
		pollInterval = workerStopPollInterval
	}
	go func() {
		ticker := time.NewTicker(pollInterval)
		defer ticker.Stop()
		for {
			if workerStopFileExists(stopFile) {
				cancel()
				return
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return ctx, cancel
}

func workerStopFileExists(stopFile string) bool {
	if _, err := os.Stat(stopFile); err == nil {
		return true
	}
	return false
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
