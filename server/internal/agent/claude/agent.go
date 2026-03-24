// Package claude implements a agent.Agent for Claude Code CLI via claude-agent-acp.
package claude

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/swm8023/wheelmaker/internal/acp"
	"github.com/swm8023/wheelmaker/internal/tools"
)

const agentName = "claude"

// Config holds configuration for the agent.
type Config struct {
	// ExePath is the path to the claude-agent-acp binary.
	// If empty, npx @zed-industries/claude-agent-acp is used.
	ExePath string

	// Env is extra environment variables for the subprocess (e.g. ANTHROPIC_API_KEY).
	Env map[string]string
}

// Agent is a stateless connection factory for Claude Code CLI.
// Each call to Connect() spawns a new claude-agent-acp subprocess.
type Agent struct {
	cfg Config
}

// New creates an Agent with the given config.
func New(cfg Config) *Agent {
	return &Agent{cfg: cfg}
}

// Name returns the agent identifier.
func (p *Agent) Name() string { return agentName }

// Connect starts a new claude-agent-acp subprocess and returns an initialized *acp.Conn.
// Conn.Start() is called internally; the caller must NOT call Start() again.
func (p *Agent) Connect(_ context.Context) (*acp.Conn, error) {
	exePath, args, err := resolveCommand(p.cfg.ExePath)
	if err != nil {
		return nil, err
	}
	env := buildEnv(p.cfg.Env)
	conn := acp.NewConn(exePath, env, args...)
	if err := conn.Start(); err != nil {
		return nil, fmt.Errorf("claude: start process: %w", err)
	}
	return conn, nil
}

// Close is a no-op since Connect() transfers subprocess ownership to Conn.
func (p *Agent) Close() error { return nil }

// buildEnv converts a map of environment variables to "KEY=VALUE" strings.
func buildEnv(m map[string]string) []string {
	env := make([]string, 0, len(m))
	for k, v := range m {
		env = append(env, k+"="+v)
	}
	return env
}

func resolveCommand(configPath string) (string, []string, error) {
	if configPath != "" {
		exePath, err := tools.ResolveBinary("claude-agent-acp", configPath)
		if err == nil {
			return exePath, nil, nil
		}
		return "", nil, fmt.Errorf("claude: resolve binary: %w", err)
	}
	npxPath, npxErr := exec.LookPath("npx")
	if npxErr != nil {
		return "", nil, fmt.Errorf("claude: npx not found: %w", npxErr)
	}
	return npxPath, []string{"--yes", "@zed-industries/claude-agent-acp"}, nil
}
