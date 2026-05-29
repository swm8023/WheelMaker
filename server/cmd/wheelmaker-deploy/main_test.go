package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func parseDeployArgsForTest(t *testing.T, args []string) deployConfig {
	t.Helper()
	cfg, err := parseArgs(args)
	if err != nil {
		t.Fatalf("parseArgs(%v): %v", args, err)
	}
	return cfg
}

func runDeployCLIForTest(t *testing.T, args []string) error {
	t.Helper()
	return run(context.Background(), args)
}

func TestParseDeployDefaults(t *testing.T) {
	cfg := parseDeployArgsForTest(t, []string{"deploy", "--repo", "C:/repo", "--bin", "C:/bin", "--time", "04:30"})
	if cfg.Mode != modeDeploy || cfg.RepoRoot != "C:/repo" || cfg.InstallDir != "C:/bin" || cfg.UpdaterTime != "04:30" {
		t.Fatalf("cfg=%+v", cfg)
	}
	if cfg.NoPull || cfg.NoNPM || cfg.NoBuild || cfg.NoInstall || cfg.NoRestart || cfg.NoConfig || cfg.NoWeb || cfg.NoUpdater {
		t.Fatalf("deploy defaults disabled work: %+v", cfg)
	}
}

func TestParseUpdateDefaults(t *testing.T) {
	cfg := parseDeployArgsForTest(t, []string{"update"})
	if cfg.Mode != modeUpdate {
		t.Fatalf("mode=%v", cfg.Mode)
	}
	if !cfg.NoPull || !cfg.NoConfig || !cfg.NoUpdater {
		t.Fatalf("update should imply no pull/config/updater: %+v", cfg)
	}
}

func TestReservedCommandsReturnNotImplemented(t *testing.T) {
	for _, args := range [][]string{{"upgrade-updater"}, {"service", "uninstall"}} {
		err := runDeployCLIForTest(t, args)
		if err == nil || !strings.Contains(err.Error(), "not implemented") {
			t.Fatalf("%v err=%v, want not implemented", args, err)
		}
	}
}

type testRunner struct {
	events *[]string
}

func (r testRunner) Run(_ context.Context, dir string, name string, args ...string) (string, error) {
	line := name + " " + strings.Join(args, " ")
	switch {
	case name == "git" && strings.Join(args, " ") == "branch --show-current":
		return "main", nil
	case name == "git" && strings.Join(args, " ") == "rev-parse HEAD":
		return "abc123", nil
	case name == "git" && len(args) >= 1 && args[0] == "pull":
		*r.events = append(*r.events, "git "+strings.Join(args, " "))
	case name == "npm":
		*r.events = append(*r.events, "npm "+strings.Join(args, " "))
	case name == "go" && len(args) >= 4 && args[0] == "build":
		*r.events = append(*r.events, "go build "+buildLabelFromOutput(args[2]))
	case name == "exec":
		*r.events = append(*r.events, "exec "+strings.Join(args, " "))
	default:
		*r.events = append(*r.events, dir+"|"+line)
	}
	return "", nil
}

type testServices struct {
	events *[]string
}

func (s testServices) CheckDeployPrerequisites(context.Context) error { return nil }
func (s testServices) Configure(context.Context) error {
	*s.events = append(*s.events, "service configure")
	return nil
}
func (s testServices) Start(_ context.Context, includeUpdater bool) error {
	if includeUpdater {
		*s.events = append(*s.events, "service start all")
	} else {
		*s.events = append(*s.events, "service start hub-monitor")
	}
	return nil
}
func (s testServices) Stop(_ context.Context, includeUpdater bool) error {
	if includeUpdater {
		*s.events = append(*s.events, "service stop all")
	} else {
		*s.events = append(*s.events, "service stop hub-monitor")
	}
	return nil
}
func (s testServices) Restart(context.Context, bool) error { return nil }
func (s testServices) Status(context.Context) error        { return nil }

type deployHarness struct {
	home   string
	cfg    deployConfig
	deps   deployDeps
	events *[]string
}

func newDeployHarness(t *testing.T) *deployHarness {
	t.Helper()
	root := t.TempDir()
	home := filepath.Join(root, "home")
	repo := filepath.Join(root, "repo")
	if err := os.MkdirAll(filepath.Join(repo, "server", "cmd"), 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "app"), 0o755); err != nil {
		t.Fatalf("mkdir app: %v", err)
	}
	events := []string{}
	cfg := deployConfig{
		Mode:        modeDeploy,
		RepoRoot:    repo,
		HomeDir:     home,
		InstallDir:  filepath.Join(home, ".wheelmaker", "bin"),
		UpdaterTime: "03:00",
	}
	return &deployHarness{
		home:   home,
		cfg:    cfg,
		events: &events,
		deps: deployDeps{
			Runner:   testRunner{events: &events},
			Services: testServices{events: &events},
			Now:      func() time.Time { return time.Date(2026, 5, 30, 1, 2, 3, 0, time.UTC) },
			Record:   func(event string) { events = append(events, event) },
		},
	}
}

func TestDeployPipelineOrder(t *testing.T) {
	h := newDeployHarness(t)
	h.cfg.Mode = modeDeploy
	if err := runDeployWithDeps(context.Background(), h.cfg, h.deps); err != nil {
		t.Fatalf("runDeployWithDeps: %v", err)
	}
	want := []string{
		"git pull --ff-only origin main",
		"npm ci --include=dev",
		"go build wheelmaker",
		"go build wheelmaker-monitor",
		"go build wheelmaker-updater",
		"go build wheelmaker-deploy",
		"npm run build:web:release",
		"service stop hub-monitor",
		"install wheelmaker",
		"install wheelmaker-monitor",
		"install wheelmaker-updater",
		"install wheelmaker-deploy",
		"write config",
		"write wrappers",
		"service configure",
		"write release",
		"service start all",
	}
	if diff := cmpStringSlices(*h.events, want); diff != "" {
		t.Fatal(diff)
	}
}

func TestUpdatePipelineSkipsUpdaterAndConfig(t *testing.T) {
	h := newDeployHarness(t)
	h.cfg.Mode = modeUpdate
	h.cfg.NoPull = true
	h.cfg.NoConfig = true
	h.cfg.NoUpdater = true
	if err := runUpdateWithDeps(context.Background(), h.cfg, h.deps); err != nil {
		t.Fatalf("runUpdateWithDeps: %v", err)
	}
	assertEventsDoNotContain(t, *h.events, "wheelmaker-updater")
	assertEventsDoNotContain(t, *h.events, "service configure")
}

func buildLabelFromOutput(out string) string {
	base := filepath.Base(out)
	base = strings.TrimSuffix(base, ".exe")
	if strings.Contains(base, "wheelmaker-deploy-next") {
		return "wheelmaker-deploy-next"
	}
	return base
}

func cmpStringSlices(got []string, want []string) string {
	if len(got) != len(want) {
		return "events length mismatch\n got: " + strings.Join(got, "\n  ") + "\nwant: " + strings.Join(want, "\n  ")
	}
	for i := range got {
		if got[i] != want[i] {
			return "event mismatch at index " + string(rune('0'+i)) + "\n got: " + strings.Join(got, "\n  ") + "\nwant: " + strings.Join(want, "\n  ")
		}
	}
	return ""
}

func assertEventsDoNotContain(t *testing.T, events []string, needle string) {
	t.Helper()
	for _, event := range events {
		if strings.Contains(event, needle) {
			t.Fatalf("events should not contain %q: %#v", needle, events)
		}
	}
}
