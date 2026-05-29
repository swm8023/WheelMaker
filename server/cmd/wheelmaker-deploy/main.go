package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
)

type runMode string

const (
	modeDeploy          runMode = "deploy"
	modeBootstrapUpdate runMode = "bootstrap-update"
	modeUpdate          runMode = "update"
	modeUpgradeUpdater  runMode = "upgrade-updater"
	modeService         runMode = "service"
	modeDoctor          runMode = "doctor"
)

type deployConfig struct {
	Mode          runMode
	ServiceAction string
	RepoRoot      string
	InstallDir    string
	UpdaterTime   string
	NoPull        bool
	NoNPM         bool
	NoBuild       bool
	NoInstall     bool
	NoRestart     bool
	NoConfig      bool
	NoWeb         bool
	NoUpdater     bool
}

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "wheelmaker-deploy: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	cfg, err := parseArgs(args)
	if err != nil {
		return err
	}
	switch cfg.Mode {
	case modeDeploy:
		return runDeploy(ctx, cfg)
	case modeBootstrapUpdate:
		return runBootstrapUpdate(ctx, cfg)
	case modeUpdate:
		return runUpdate(ctx, cfg)
	case modeUpgradeUpdater:
		return errors.New("upgrade-updater is not implemented in this transitional CLI")
	case modeService:
		return runService(ctx, cfg)
	case modeDoctor:
		return runDoctor(ctx, cfg)
	default:
		return fmt.Errorf("unsupported mode: %s", cfg.Mode)
	}
}

func parseArgs(args []string) (deployConfig, error) {
	if len(args) == 0 {
		return deployConfig{}, errors.New("command is required")
	}
	mode := runMode(args[0])
	cfg := deployConfig{
		Mode:        mode,
		UpdaterTime: "03:00",
	}
	fs := flag.NewFlagSet("wheelmaker-deploy "+string(mode), flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.StringVar(&cfg.RepoRoot, "repo", "", "WheelMaker repository root")
	fs.StringVar(&cfg.InstallDir, "bin", "", "WheelMaker install directory")
	fs.StringVar(&cfg.UpdaterTime, "time", cfg.UpdaterTime, "updater daily time in HH:mm")
	fs.BoolVar(&cfg.NoPull, "no-pull", false, "skip git pull")
	fs.BoolVar(&cfg.NoNPM, "no-npm", false, "skip app npm dependency sync")
	fs.BoolVar(&cfg.NoBuild, "no-build", false, "skip Go builds")
	fs.BoolVar(&cfg.NoInstall, "no-install", false, "skip binary install")
	fs.BoolVar(&cfg.NoRestart, "no-restart", false, "skip service restart")
	fs.BoolVar(&cfg.NoConfig, "no-config", false, "skip service configuration")
	fs.BoolVar(&cfg.NoWeb, "no-web", false, "skip Web publish")
	fs.BoolVar(&cfg.NoUpdater, "no-updater", false, "skip updater build/install/config")
	if err := fs.Parse(args[1:]); err != nil {
		return deployConfig{}, err
	}

	switch mode {
	case modeDeploy, modeBootstrapUpdate, modeUpdate, modeUpgradeUpdater, modeDoctor:
	case modeService:
		rest := fs.Args()
		if len(rest) == 0 {
			return deployConfig{}, errors.New("service action is required")
		}
		cfg.ServiceAction = rest[0]
	default:
		return deployConfig{}, fmt.Errorf("unsupported command: %s", mode)
	}

	if cfg.NoWeb {
		cfg.NoNPM = true
	}
	if mode == modeUpdate {
		cfg.NoPull = true
		cfg.NoConfig = true
		cfg.NoUpdater = true
	}
	if mode == modeBootstrapUpdate {
		cfg.NoNPM = true
		cfg.NoInstall = true
		cfg.NoRestart = true
		cfg.NoConfig = true
		cfg.NoWeb = true
		cfg.NoUpdater = true
	}
	return cfg, nil
}

func runDeploy(context.Context, deployConfig) error {
	return errors.New("deploy is not implemented")
}

func runBootstrapUpdate(context.Context, deployConfig) error {
	return errors.New("bootstrap-update is not implemented")
}

func runUpdate(context.Context, deployConfig) error {
	return errors.New("update is not implemented")
}

func runService(_ context.Context, cfg deployConfig) error {
	if cfg.ServiceAction == "uninstall" {
		return errors.New("service uninstall is not implemented in this transitional CLI")
	}
	return errors.New("service command is not implemented")
}

func runDoctor(context.Context, deployConfig) error {
	return errors.New("doctor is not implemented")
}
