package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

type runCall struct {
	name string
	args []string
	dir  string
}

type fakeRunner struct {
	calls   []runCall
	results map[string]fakeResult
}

type fakeResult struct {
	out string
	err error
}

func (f *fakeRunner) CombinedOutput(_ context.Context, dir string, name string, args ...string) (string, error) {
	f.calls = append(f.calls, runCall{name: name, args: append([]string{}, args...), dir: dir})
	key := name + " " + strings.Join(args, " ")
	if r, ok := f.results[key]; ok {
		return r.out, r.err
	}
	return "", nil
}

func TestParseDailyTime(t *testing.T) {
	h, m, err := parseDailyTime("03:00")
	if err != nil {
		t.Fatalf("parseDailyTime: %v", err)
	}
	if h != 3 || m != 0 {
		t.Fatalf("unexpected hour/minute: %d:%d", h, m)
	}

	if _, _, err := parseDailyTime("3:00"); err == nil {
		t.Fatalf("expected invalid time error")
	}
}

func TestNextRunAfterNow(t *testing.T) {
	now := time.Date(2026, 4, 1, 4, 0, 0, 0, time.Local)
	next := nextRunTime(now, 3, 0)
	if !next.After(now) {
		t.Fatalf("next should be after now: next=%v now=%v", next, now)
	}
	if got, want := next.Day(), 2; got != want {
		t.Fatalf("next day=%d want=%d", got, want)
	}
}

func TestRunUpdateRound_RunsRefreshScript(t *testing.T) {
	repoDir := t.TempDir()
	scriptsDir := filepath.Join(repoDir, "scripts")
	if err := os.MkdirAll(scriptsDir, 0o755); err != nil {
		t.Fatalf("mkdir scripts: %v", err)
	}
	refreshPath := filepath.Join(scriptsDir, "refresh_server.ps1")
	if err := os.WriteFile(refreshPath, []byte(""), 0o644); err != nil {
		t.Fatalf("write refresh script: %v", err)
	}

	cfg := UpdaterConfig{RepoDir: repoDir, InstallDir: `C:/Users/test/.wheelmaker/bin`}
	f := &fakeRunner{results: map[string]fakeResult{}}

	if err := runUpdateRound(context.Background(), cfg, f, false); err != nil {
		t.Fatalf("runUpdateRound: %v", err)
	}

	var got []string
	for _, c := range f.calls {
		got = append(got, c.name+" "+strings.Join(c.args, " "))
	}
	want := []string{
		"powershell -NoProfile -ExecutionPolicy Bypass -File " + refreshPath + " -InstallDir C:/Users/test/.wheelmaker/bin -SkipUpdaterInstall -SkipServiceConfig",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("commands mismatch\n got: %#v\nwant: %#v", got, want)
	}
}

func TestRunUpdateRound_RefreshScriptMissing(t *testing.T) {
	cfg := UpdaterConfig{RepoDir: t.TempDir(), InstallDir: `C:/Users/test/.wheelmaker/bin`}
	f := &fakeRunner{results: map[string]fakeResult{}}
	err := runUpdateRound(context.Background(), cfg, f, false)
	if err == nil || !strings.Contains(err.Error(), "refresh script missing") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunUpdateRound_ManualSignalSkipsUpdate(t *testing.T) {
	repoDir := t.TempDir()
	scriptsDir := filepath.Join(repoDir, "scripts")
	if err := os.MkdirAll(scriptsDir, 0o755); err != nil {
		t.Fatalf("mkdir scripts: %v", err)
	}
	refreshPath := filepath.Join(scriptsDir, "refresh_server.ps1")
	if err := os.WriteFile(refreshPath, []byte(""), 0o644); err != nil {
		t.Fatalf("write refresh script: %v", err)
	}

	cfg := UpdaterConfig{RepoDir: repoDir, InstallDir: `C:/Users/test/.wheelmaker/bin`}
	f := &fakeRunner{results: map[string]fakeResult{}}

	if err := runUpdateRound(context.Background(), cfg, f, true); err != nil {
		t.Fatalf("runUpdateRound: %v", err)
	}
	if len(f.calls) != 1 {
		t.Fatalf("expected one command call, got %d", len(f.calls))
	}
	call := f.calls[0]
	argsLine := strings.Join(call.args, " ")
	if !strings.Contains(argsLine, "-SkipUpdate") {
		t.Fatalf("expected -SkipUpdate in args, got: %s", argsLine)
	}
}

func TestConsumeManualSignal(t *testing.T) {
	dir := t.TempDir()
	signalPath := filepath.Join(dir, "update-now.signal")
	if err := os.WriteFile(signalPath, []byte("trigger"), 0o644); err != nil {
		t.Fatalf("write signal: %v", err)
	}

	reason, ok, err := consumeManualSignal(signalPath)
	if err != nil {
		t.Fatalf("consumeManualSignal error: %v", err)
	}
	if !ok {
		t.Fatalf("expected signal consumed")
	}
	if reason != triggerReasonManualSignal {
		t.Fatalf("reason=%q, want=%q", reason, triggerReasonManualSignal)
	}
	if _, err := os.Stat(signalPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected signal file removed, stat err=%v", err)
	}
}

func TestConsumeManualSignal_MissingFile(t *testing.T) {
	reason, ok, err := consumeManualSignal(filepath.Join(t.TempDir(), "missing.signal"))
	if err != nil {
		t.Fatalf("consumeManualSignal error: %v", err)
	}
	if ok {
		t.Fatalf("expected no signal when file is missing")
	}
	if reason != "" {
		t.Fatalf("expected empty reason for missing signal, got: %q", reason)
	}
}

func TestConsumeManualSignal_FullUpdateMode(t *testing.T) {
	dir := t.TempDir()
	signalPath := filepath.Join(dir, "update-now.signal")
	if err := os.WriteFile(signalPath, []byte("full-update\n"), 0o644); err != nil {
		t.Fatalf("write signal: %v", err)
	}

	reason, ok, err := consumeManualSignal(signalPath)
	if err != nil {
		t.Fatalf("consumeManualSignal error: %v", err)
	}
	if !ok {
		t.Fatalf("expected signal consumed")
	}
	if reason != triggerReasonManualFullUpdate {
		t.Fatalf("reason=%q, want=%q", reason, triggerReasonManualFullUpdate)
	}
}

func TestParseManualSignalReason_DefaultManualSignal(t *testing.T) {
	if got := parseManualSignalReason("2026-04-11T03:00:00Z"); got != triggerReasonManualSignal {
		t.Fatalf("reason=%q, want=%q", got, triggerReasonManualSignal)
	}
}

func TestParseManualSignalReason_FullUpdateSignal(t *testing.T) {
	if got := parseManualSignalReason("full-update"); got != triggerReasonManualFullUpdate {
		t.Fatalf("reason=%q, want=%q", got, triggerReasonManualFullUpdate)
	}
}
