package main

import (
	"github.com/swm8023/wheelmaker/internal/shared"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestWheelmakerLogDir(t *testing.T) {
	home := filepath.Clean(`C:\Users\swm`)
	got := wheelmakerLogDir(home)
	want := filepath.Join(home, ".wheelmaker", "log")
	if got != want {
		t.Fatalf("wheelmakerLogDir()=%q, want %q", got, want)
	}
}

func TestSanitizeWorkerArgs(t *testing.T) {
	in := []string{"-d", "--daemon-worker", "--hub-worker", "--registry-worker", "--foo", "bar"}
	got := sanitizeWorkerArgs(in)
	if len(got) != 2 || got[0] != "--foo" || got[1] != "bar" {
		t.Fatalf("sanitizeWorkerArgs()=%v", got)
	}
}

func TestGuardianWorkerSpecsSkipRegistryWorkerWhenRegistryListenDisabled(t *testing.T) {
	cfg := shared.RegistryConfig{
		Listen: false,
		Server: "wss://registry.example/ws",
		Port:   28800,
	}
	specs := guardianWorkerSpecs([]string{"--foo", "bar"}, cfg)

	if len(specs) != 1 {
		t.Fatalf("guardianWorkerSpecs() len=%d want 1 specs=%v", len(specs), specs)
	}
	if specs[0].markerFlag != hubWorkerArg {
		t.Fatalf("guardianWorkerSpecs()[0].markerFlag=%q want %q", specs[0].markerFlag, hubWorkerArg)
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

func TestParseWorkerProcessesFromPSAcceptsPathComm(t *testing.T) {
	out := []byte(`123 /Users/me/.wheelmaker/bin/wheelmaker /Users/me/.wheelmaker/bin/wheelmaker --hub-worker
124 /Users/me/.wheelmaker/bin/wheelmaker /Users/me/.wheelmaker/bin/wheelmaker --registry-worker
125 /Users/me/.wheelmaker/bin/wheelmaker-updater /Users/me/.wheelmaker/bin/wheelmaker-updater --repo /repo
126 bash bash -lc wheelmaker --hub-worker
`)

	workers, err := parseWorkerProcessesFromPS(out, "wheelmaker", "--hub-worker")
	if err != nil {
		t.Fatalf("parseWorkerProcessesFromPS() error = %v", err)
	}
	if len(workers) != 1 || workers[0].PID != 123 {
		t.Fatalf("workers=%#v, want only pid 123", workers)
	}
}

func TestParseWorkerProcessesFromPSAcceptsTruncatedDarwinComm(t *testing.T) {
	out := []byte(`123 /Users/me/.whe /Users/me/.wheelmaker/bin/wheelmaker --hub-worker
124 /Users/me/.whe /Users/me/.wheelmaker/bin/wheelmaker --registry-worker
125 /Users/me/.whe /Users/me/.wheelmaker/bin/wheelmaker-updater --hub-worker
126 bash bash -lc wheelmaker --hub-worker
`)

	workers, err := parseWorkerProcessesFromPS(out, "wheelmaker", "--hub-worker")
	if err != nil {
		t.Fatalf("parseWorkerProcessesFromPS() error = %v", err)
	}
	if len(workers) != 1 || workers[0].PID != 123 {
		t.Fatalf("workers=%#v, want only pid 123", workers)
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

func TestRedirectProcessStdioToDevNull(t *testing.T) {
	oldStdout := os.Stdout
	oldStderr := os.Stderr

	restore, err := redirectProcessStdioToDevNull()
	if err != nil {
		t.Fatalf("redirectProcessStdioToDevNull() error = %v", err)
	}
	if os.Stdout == oldStdout {
		t.Fatalf("stdout file was not replaced")
	}
	if os.Stderr == oldStderr {
		t.Fatalf("stderr file was not replaced")
	}
	if os.Stdout.Name() != os.DevNull {
		t.Fatalf("stdout sink = %q, want %q", os.Stdout.Name(), os.DevNull)
	}
	if os.Stderr.Name() != os.DevNull {
		t.Fatalf("stderr sink = %q, want %q", os.Stderr.Name(), os.DevNull)
	}

	restore()
	if os.Stdout != oldStdout {
		t.Fatalf("stdout not restored")
	}
	if os.Stderr != oldStderr {
		t.Fatalf("stderr not restored")
	}
}
