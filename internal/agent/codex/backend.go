// Package codex implements an agent.Agent and acp.AgentPlugin for the Codex CLI via codex-acp.
package codex

import (
	"context"
	"fmt"

	acp "github.com/swm8023/wheelmaker/internal/acp"
	"github.com/swm8023/wheelmaker/internal/agent"
	"github.com/swm8023/wheelmaker/internal/tools"
)

const agentName = "codex"

// Config holds configuration for the Backend.
type Config struct {
	// ExePath is the path to the codex-acp binary.
	// If empty, tools.ResolveBinary("codex-acp", "") is used.
	ExePath string

	// Env is extra environment variables for the subprocess (e.g. OPENAI_API_KEY).
	Env map[string]string
}

// Backend is a stateless connection factory and acp.AgentPlugin for the Codex CLI.
// Each call to Connect() spawns a new codex-acp subprocess.
type Backend struct {
	acp.DefaultPlugin
	cfg Config
}

// New creates a Backend with the given config.
func New(cfg Config) *Backend {
	return &Backend{cfg: cfg}
}

// Name returns the agent identifier.
func (p *Backend) Name() string { return agentName }

// Connect starts a new codex-acp subprocess and returns an initialized *agent.Conn.
// Conn.Start() is called internally; the caller must NOT call Start() again.
func (p *Backend) Connect(_ context.Context) (*agent.Conn, error) {
	exePath, err := tools.ResolveBinary("codex-acp", p.cfg.ExePath)
	if err != nil {
		return nil, fmt.Errorf("codex: resolve binary: %w", err)
	}

	env := buildEnv(p.cfg.Env)
	conn := agent.New(exePath, env)
	if err := conn.Start(); err != nil {
		return nil, fmt.Errorf("codex: start process: %w", err)
	}
	return conn, nil
}

// Close is a no-op since Connect() transfers subprocess ownership to Conn.
func (p *Backend) Close() error { return nil }

// ValidateConfigOptions enforces Codex-specific config requirements.
func (p *Backend) ValidateConfigOptions(opts []acp.ConfigOption) error {
	for _, opt := range opts {
		if (opt.ID == "mode" || opt.Category == "mode") && opt.CurrentValue == "" {
			return fmt.Errorf("codex: mode currentValue is empty")
		}
		if (opt.ID == "model" || opt.Category == "model") && opt.CurrentValue == "" {
			return fmt.Errorf("codex: model currentValue is empty")
		}
	}
	return nil
}

// buildEnv converts a map of environment variables to "KEY=VALUE" strings.
func buildEnv(m map[string]string) []string {
	env := make([]string, 0, len(m))
	for k, v := range m {
		env = append(env, k+"="+v)
	}
	return env
}
