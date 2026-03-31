package main

import (
	"context"
	"errors"
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

func TestRunUpdateRound_UpToDate(t *testing.T) {
	cfg := UpdaterConfig{RepoDir: `D:/Code/WheelMaker`, InstallDir: `C:/Users/test/.wheelmaker/bin`}
	f := &fakeRunner{results: map[string]fakeResult{
		"git rev-parse --abbrev-ref HEAD": {out: "main"},
		"git rev-parse HEAD":              {out: "abcdef01"},
		"git rev-parse origin/main":       {out: "abcdef01"},
	}}

	if err := runUpdateRound(context.Background(), cfg, f); err != nil {
		t.Fatalf("runUpdateRound: %v", err)
	}

	var got []string
	for _, c := range f.calls {
		got = append(got, c.name+" "+strings.Join(c.args, " "))
	}
	want := []string{
		"git fetch origin",
		"git rev-parse --abbrev-ref HEAD",
		"git rev-parse HEAD",
		"git rev-parse origin/main",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("commands mismatch\n got: %#v\nwant: %#v", got, want)
	}
}

func TestRunUpdateRound_WithUpdatesRunsDeploy(t *testing.T) {
	cfg := UpdaterConfig{RepoDir: `D:/Code/WheelMaker`, InstallDir: `C:/Users/test/.wheelmaker/bin`}
	f := &fakeRunner{results: map[string]fakeResult{
		"git rev-parse --abbrev-ref HEAD": {out: "main"},
		"git rev-parse HEAD":              {out: "oldsha"},
		"git rev-parse origin/main":       {out: "newsha"},
	}}

	err := runUpdateRound(context.Background(), cfg, f)
	if err == nil {
		if len(f.calls) < 6 {
			t.Fatalf("expected deploy command, calls=%d", len(f.calls))
		}
		return
	}

	if !strings.Contains(err.Error(), "refresh script missing") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunUpdateRound_PullFailure(t *testing.T) {
	cfg := UpdaterConfig{RepoDir: `D:/Code/WheelMaker`, InstallDir: `C:/Users/test/.wheelmaker/bin`}
	f := &fakeRunner{results: map[string]fakeResult{
		"git rev-parse --abbrev-ref HEAD": {out: "main"},
		"git rev-parse HEAD":              {out: "oldsha"},
		"git rev-parse origin/main":       {out: "newsha"},
		"git pull --ff-only origin main":  {err: errors.New("pull failed")},
	}}

	err := runUpdateRound(context.Background(), cfg, f)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "pull failed") {
		t.Fatalf("unexpected error: %v", err)
	}
}
