package main

import (
	"context"
	"strings"
	"testing"
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
