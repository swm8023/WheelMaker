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

func TestRunUpdateRound_RunsDeployCLI(t *testing.T) {
	old := runtimeGOOS
	runtimeGOOS = "windows"
	defer func() { runtimeGOOS = old }()

	repoDir := t.TempDir()
	installDir := filepath.Join(t.TempDir(), "bin")
	deployPath := createDeployCLI(t, installDir, "windows")

	cfg := UpdaterConfig{RepoDir: repoDir, InstallDir: installDir, DailyTime: "03:00"}
	f := &fakeRunner{results: map[string]fakeResult{}}

	if err := runUpdateRound(context.Background(), cfg, f, false); err != nil {
		t.Fatalf("runUpdateRound: %v", err)
	}

	var got []string
	for _, c := range f.calls {
		got = append(got, c.dir+"|"+c.name+" "+strings.Join(c.args, " "))
	}
	want := []string{
		repoDir + "|" + deployPath + " bootstrap-update --repo " + repoDir + " --bin " + installDir + " --time 03:00",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("commands mismatch\n got: %#v\nwant: %#v", got, want)
	}
}

func TestRunUpdateRound_DarwinRunsDeployCLI(t *testing.T) {
	old := runtimeGOOS
	runtimeGOOS = "darwin"
	defer func() { runtimeGOOS = old }()

	repoDir := t.TempDir()
	installDir := filepath.Join(t.TempDir(), "bin")
	deployPath := createDeployCLI(t, installDir, "darwin")

	cfg := UpdaterConfig{RepoDir: repoDir, InstallDir: installDir, DailyTime: "04:00"}
	f := &fakeRunner{results: map[string]fakeResult{}}

	if err := runUpdateRound(context.Background(), cfg, f, false); err != nil {
		t.Fatalf("runUpdateRound: %v", err)
	}

	if len(f.calls) != 1 {
		t.Fatalf("expected one darwin deploy call, got %d", len(f.calls))
	}
	call := f.calls[0]
	if call.name != deployPath {
		t.Fatalf("command=%q want %q", call.name, deployPath)
	}
	args := strings.Join(call.args, " ")
	if !strings.Contains(args, "bootstrap-update") || !strings.Contains(args, "--bin "+installDir) || !strings.Contains(args, "--time 04:00") {
		t.Fatalf("unexpected args: %s", args)
	}
	if strings.Contains(args, "--no-web") {
		t.Fatalf("full update should publish web through deploy cli: %s", args)
	}
}

func TestRunUpdateRound_DarwinManualSkipWebSignalDisablesWeb(t *testing.T) {
	old := runtimeGOOS
	runtimeGOOS = "darwin"
	defer func() { runtimeGOOS = old }()

	repoDir := t.TempDir()
	installDir := filepath.Join(t.TempDir(), "bin")
	createDeployCLI(t, installDir, "darwin")

	cfg := UpdaterConfig{RepoDir: repoDir, InstallDir: installDir, DailyTime: "03:00"}
	f := &fakeRunner{results: map[string]fakeResult{}}

	if err := runUpdateRoundWithOptions(context.Background(), cfg, f, updateRoundOptions{skipWebPublish: true}); err != nil {
		t.Fatalf("runUpdateRound: %v", err)
	}

	args := strings.Join(f.calls[0].args, " ")
	if !strings.Contains(args, "--no-web") {
		t.Fatalf("skip-web-publish signal should pass --no-web, got: %s", args)
	}
}

func TestRunUpdateRound_LinuxRunsDeployCLI(t *testing.T) {
	old := runtimeGOOS
	runtimeGOOS = "linux"
	defer func() { runtimeGOOS = old }()

	repoDir := t.TempDir()
	installDir := filepath.Join(t.TempDir(), "bin")
	deployPath := createDeployCLI(t, installDir, "linux")

	cfg := UpdaterConfig{RepoDir: repoDir, InstallDir: installDir, DailyTime: "03:00"}
	f := &fakeRunner{results: map[string]fakeResult{}}

	if err := runUpdateRound(context.Background(), cfg, f, false); err != nil {
		t.Fatalf("runUpdateRound: %v", err)
	}

	if len(f.calls) != 1 {
		t.Fatalf("expected one linux deploy call, got %d", len(f.calls))
	}
	call := f.calls[0]
	if call.name != deployPath {
		t.Fatalf("command=%q want %q", call.name, deployPath)
	}
	args := strings.Join(call.args, " ")
	if !strings.Contains(args, "bootstrap-update") || !strings.Contains(args, "--bin "+installDir) {
		t.Fatalf("unexpected args: %s", args)
	}
	if strings.Contains(args, "--no-web") {
		t.Fatalf("full update should publish web through deploy cli: %s", args)
	}
}

func TestRunUpdateRound_LinuxManualSignalSkipsWebPublish(t *testing.T) {
	old := runtimeGOOS
	runtimeGOOS = "linux"
	defer func() { runtimeGOOS = old }()

	repoDir := t.TempDir()
	installDir := filepath.Join(t.TempDir(), "bin")
	createDeployCLI(t, installDir, "linux")

	cfg := UpdaterConfig{RepoDir: repoDir, InstallDir: installDir, DailyTime: "03:00"}
	f := &fakeRunner{results: map[string]fakeResult{}}

	if err := runUpdateRoundWithOptions(context.Background(), cfg, f, updateRoundOptions{skipWebPublish: true}); err != nil {
		t.Fatalf("runUpdateRound: %v", err)
	}

	args := strings.Join(f.calls[0].args, " ")
	if !strings.Contains(args, "--no-web") {
		t.Fatalf("manual signal should skip web publish, got: %s", args)
	}
}

func TestRequiredCommandsForOS(t *testing.T) {
	if got := requiredCommandsForOS("windows"); !reflect.DeepEqual(got, []string{}) {
		t.Fatalf("windows commands=%#v", got)
	}
	if got := requiredCommandsForOS("darwin"); !reflect.DeepEqual(got, []string{}) {
		t.Fatalf("darwin commands=%#v", got)
	}
	if got := requiredCommandsForOS("linux"); !reflect.DeepEqual(got, []string{}) {
		t.Fatalf("linux commands=%#v", got)
	}
}

func TestRunUpdateRound_DeployCLIMissing(t *testing.T) {
	cfg := UpdaterConfig{RepoDir: t.TempDir(), InstallDir: `C:/Users/test/.wheelmaker/bin`}
	f := &fakeRunner{results: map[string]fakeResult{}}
	err := runUpdateRound(context.Background(), cfg, f, false)
	if err == nil || !strings.Contains(err.Error(), "wheelmaker-deploy") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunUpdateRound_ManualSignalCanSkipWebPublish(t *testing.T) {
	repoDir := t.TempDir()
	installDir := filepath.Join(t.TempDir(), "bin")
	createDeployCLI(t, installDir, runtimeGOOS)

	cfg := UpdaterConfig{RepoDir: repoDir, InstallDir: installDir, DailyTime: "03:00"}
	f := &fakeRunner{results: map[string]fakeResult{}}

	err := runUpdateRoundWithOptions(context.Background(), cfg, f, updateRoundOptions{
		skipWebPublish: true,
	})
	if err != nil {
		t.Fatalf("runUpdateRoundWithOptions: %v", err)
	}
	if len(f.calls) != 1 {
		t.Fatalf("expected one command call, got %d", len(f.calls))
	}
	argsLine := strings.Join(f.calls[0].args, " ")
	if !strings.Contains(argsLine, "--no-web") {
		t.Fatalf("expected --no-web in args, got: %s", argsLine)
	}
}

func TestConsumeManualSignal_PlainSignalUsesFullUpdate(t *testing.T) {
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
	if reason != triggerReasonManualFullUpdate {
		t.Fatalf("reason=%q, want=%q", reason, triggerReasonManualFullUpdate)
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

func TestParseManualSignalReason_PlainSignalFallsBackToFullUpdate(t *testing.T) {
	if got := parseManualSignalReason("2026-04-11T03:00:00Z"); got != triggerReasonManualFullUpdate {
		t.Fatalf("reason=%q, want=%q", got, triggerReasonManualFullUpdate)
	}
}

func TestParseManualSignalReason_FullUpdateSignal(t *testing.T) {
	if got := parseManualSignalReason("full-update"); got != triggerReasonManualFullUpdate {
		t.Fatalf("reason=%q, want=%q", got, triggerReasonManualFullUpdate)
	}
}

func TestParseManualSignalReason_SkipWebPublishSignal(t *testing.T) {
	if got := parseManualSignalReason("skip-web-publish\n2026-05-21T00:00:00Z"); got != triggerReasonManualSignalSkipWebPublish {
		t.Fatalf("reason=%q, want=%q", got, triggerReasonManualSignalSkipWebPublish)
	}
}

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

func createDeployCLI(t *testing.T, installDir string, goos string) string {
	t.Helper()
	if err := os.MkdirAll(installDir, 0o755); err != nil {
		t.Fatalf("mkdir install dir: %v", err)
	}
	name := "wheelmaker-deploy"
	if goos == "windows" {
		name += ".exe"
	}
	path := filepath.Join(installDir, name)
	if err := os.WriteFile(path, []byte("deploy cli"), 0o755); err != nil {
		t.Fatalf("write deploy cli: %v", err)
	}
	return path
}
