package main

import (
	"os"
	"os/exec"
	"testing"
)

func TestSanitizeWorkerArgs(t *testing.T) {
	in := []string{"-d", "--daemon-worker", "--hub-worker", "--registry-worker", "--foo", "bar"}
	got := sanitizeWorkerArgs(in)
	if len(got) != 2 || got[0] != "--foo" || got[1] != "bar" {
		t.Fatalf("sanitizeWorkerArgs()=%v", got)
	}
}

func TestChooseKeepPID(t *testing.T) {
	workers := []daemonProcess{{PID: 42}, {PID: 17}, {PID: 29}}
	if got := chooseKeepPID(workers, 29); got != 29 {
		t.Fatalf("chooseKeepPID preferred mismatch: got=%d want=29", got)
	}
	if got := chooseKeepPID(workers, 999); got != 17 {
		t.Fatalf("chooseKeepPID fallback mismatch: got=%d want=17", got)
	}
}

func TestConfigureWorkerCommandIOToDevNull(t *testing.T) {
	cmd := exec.Command("wheelmaker.exe", "--hub-worker")
	restore, err := configureWorkerCommandIO(cmd)
	if err != nil {
		t.Fatalf("configureWorkerCommandIO() error = %v", err)
	}
	defer restore()

	stdoutFile, ok := cmd.Stdout.(*os.File)
	if !ok {
		t.Fatalf("stdout sink type = %T, want *os.File", cmd.Stdout)
	}
	stderrFile, ok := cmd.Stderr.(*os.File)
	if !ok {
		t.Fatalf("stderr sink type = %T, want *os.File", cmd.Stderr)
	}
	if stdoutFile.Name() != os.DevNull {
		t.Fatalf("stdout sink = %q, want %q", stdoutFile.Name(), os.DevNull)
	}
	if stderrFile.Name() != os.DevNull {
		t.Fatalf("stderr sink = %q, want %q", stderrFile.Name(), os.DevNull)
	}
}
