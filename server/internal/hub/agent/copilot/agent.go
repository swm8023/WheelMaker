// Package copilot implements agent.Agent for GitHub Copilot CLI via its native ACP server.
// It uses stdio mode: copilot --acp --stdio
package copilot

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/swm8023/wheelmaker/internal/hub/acp"
)

const agentName = "copilot"

// Config holds configuration for the Copilot CLI agent.
type Config struct {
	// ExePath is the path to the copilot binary. Empty = search PATH for "copilot".
	ExePath string

	// Env is extra environment variables passed to the subprocess.
	Env map[string]string
}

// Agent is a stateless connection factory for GitHub Copilot CLI.
// Each call to Connect() spawns a new copilot subprocess.
type Agent struct {
	cfg Config
}

// New creates an Agent with the given config.
func New(cfg Config) *Agent {
	return &Agent{cfg: cfg}
}

// Name returns the agent identifier.
func (a *Agent) Name() string { return agentName }

// Close is a no-op since Connect() transfers subprocess ownership to the Conn.
func (a *Agent) Close() error { return nil }

// Connect starts copilot with --acp --stdio and returns an initialized *acp.Conn.
// Conn.Start() is called internally.
func (a *Agent) Connect(_ context.Context) (*acp.Conn, error) {
	exePath, err := resolveExe(a.cfg.ExePath)
	if err != nil {
		return nil, err
	}
	env := buildEnv(a.cfg.Env)
	conn := acp.NewConn(exePath, env, "--acp", "--stdio")
	if err := conn.Start(); err != nil {
		return nil, fmt.Errorf("copilot: start process: %w", err)
	}
	return conn, nil
}

// resolveExe returns the path to the copilot binary.
// If configPath is non-empty it is used directly; otherwise PATH is searched.
func resolveExe(configPath string) (string, error) {
	if configPath != "" {
		return configPath, nil
	}
	exePath, err := exec.LookPath("copilot")
	if err != nil {
		return "", fmt.Errorf("copilot: binary not found in PATH (install GitHub Copilot CLI): %w", err)
	}
	return exePath, nil
}

// buildEnv converts a map of environment variables to "KEY=VALUE" strings.
func buildEnv(m map[string]string) []string {
	env := make([]string, 0, len(m))
	for k, v := range m {
		env = append(env, k+"="+v)
	}
	return env
}
