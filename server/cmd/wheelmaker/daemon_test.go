package main

import "testing"

func TestSanitizeWorkerArgs(t *testing.T) {
	in := []string{"-d", "--daemon-worker", "--foo", "bar"}
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
