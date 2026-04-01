package main

import (
	"path/filepath"
	"testing"
)

func TestResolveStateDir_FromBinInstallDir(t *testing.T) {
	home := `C:\Users\swm`
	installDir := `C:\Users\swm\.wheelmaker\bin`
	got := resolveStateDir(home, installDir)
	want := filepath.Clean(`C:\Users\swm\.wheelmaker`)
	if got != want {
		t.Fatalf("resolveStateDir=%q want=%q", got, want)
	}
}

func TestResolveStateDir_FromCustomInstallDir(t *testing.T) {
	home := `C:\Users\swm`
	installDir := `D:\WheelMaker\bin\prod`
	got := resolveStateDir(home, installDir)
	want := filepath.Clean(`D:\WheelMaker\bin\prod`)
	if got != want {
		t.Fatalf("resolveStateDir=%q want=%q", got, want)
	}
}

func TestResolveStateDir_FallbackHome(t *testing.T) {
	home := `C:\Users\swm`
	got := resolveStateDir(home, "")
	want := filepath.Clean(`C:\Users\swm\.wheelmaker`)
	if got != want {
		t.Fatalf("resolveStateDir=%q want=%q", got, want)
	}
}

func TestUpdaterLogFilePath(t *testing.T) {
	stateDir := filepath.Clean(`C:\Users\swm\.wheelmaker`)
	got := updaterLogFilePath(stateDir)
	want := filepath.Join(stateDir, "log", "updater.log")
	if got != want {
		t.Fatalf("updaterLogFilePath=%q want=%q", got, want)
	}
}
