// Package codex implements a agent.Agent for the Codex CLI via codex-acp.
package codex

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/swm8023/wheelmaker/internal/acp"
	"github.com/swm8023/wheelmaker/internal/tools"
)

const agentName = "codex"

// Config holds configuration for the agent.
type Config struct {
	// ExePath is the path to the codex-acp binary.
	// If empty, tools.ResolveBinary("codex-acp", "") is used.
	ExePath string

	// Env is extra environment variables for the subprocess (e.g. OPENAI_API_KEY).
	Env map[string]string
}

// Agent is a stateless connection factory for the Codex CLI.
// Each call to Connect() spawns a new codex-acp subprocess.
type Agent struct {
	cfg Config
}

// New creates an Agent with the given config.
func New(cfg Config) *Agent {
	return &Agent{cfg: cfg}
}

// Name returns the agent identifier.
func (p *Agent) Name() string { return agentName }

// Connect starts a new codex-acp subprocess and returns an initialized *acp.Conn.
// Conn.Start() is called internally; the caller must NOT call Start() again.
func (p *Agent) Connect(_ context.Context) (*acp.Conn, error) {
	exePath, err := tools.ResolveBinary("codex-acp", p.cfg.ExePath)
	if err != nil {
		return nil, fmt.Errorf("codex: resolve binary: %w", err)
	}

	env := buildEnv(p.cfg.Env)
	conn := acp.NewConn(exePath, env)
	if err := conn.Start(); err != nil {
		return nil, fmt.Errorf("codex: start process: %w", err)
	}
	return conn, nil
}

// Close is a no-op since Connect() transfers subprocess ownership to Conn.
func (p *Agent) Close() error { return nil }

// NormalizeParams passes notifications through unchanged.
func (p *Agent) NormalizeParams(_ string, params json.RawMessage) json.RawMessage { return params }

// buildEnv converts a map of environment variables to "KEY=VALUE" strings.
func buildEnv(m map[string]string) []string {
	env := make([]string, 0, len(m))
	for k, v := range m {
		env = append(env, k+"="+v)
	}
	return env
}
