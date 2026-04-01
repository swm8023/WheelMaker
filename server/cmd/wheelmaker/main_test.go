package main

import (
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
