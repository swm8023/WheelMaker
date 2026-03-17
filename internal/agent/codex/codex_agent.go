// Package codex implements an agent.Agent for the Codex CLI via codex-acp.
package codex

import (
	"context"
	"fmt"

	acp "github.com/swm8023/wheelmaker/internal/acp"
	"github.com/swm8023/wheelmaker/internal/agent"
	"github.com/swm8023/wheelmaker/internal/tools"
)

const agentName = "codex"

// Config holds configuration for the CodexAgent.
type Config struct {
	// ExePath is the path to the codex-acp binary.
	// If empty, tools.ResolveBinary("codex-acp", "") is used.
	ExePath string

	// Env is extra environment variables for the subprocess (e.g. OPENAI_API_KEY).
	Env map[string]string
}

// CodexAgent is a stateless connection factory for the Codex CLI via codex-acp.
// Each Call to Connect() spawns a new subprocess.
type CodexAgent struct {
	cfg Config
}

// NewAgent creates a CodexAgent with the given config.
func NewAgent(cfg Config) *CodexAgent {
	return &CodexAgent{cfg: cfg}
}

// Name returns the agent identifier.
func (a *CodexAgent) Name() string { return agentName }

// Connect starts a new codex-acp subprocess and returns an initialized *agent.Conn.
// Conn.Start() is called internally; the caller must NOT call Start() again.
func (a *CodexAgent) Connect(_ context.Context) (*agent.Conn, error) {
	exePath, err := tools.ResolveBinary("codex-acp", a.cfg.ExePath)
	if err != nil {
		return nil, fmt.Errorf("codex: resolve binary: %w", err)
	}

	env := buildEnv(a.cfg.Env)
	conn := agent.New(exePath, env)
	if err := conn.Start(); err != nil {
		return nil, fmt.Errorf("codex: start process: %w", err)
	}
	return conn, nil
}

// Close is a no-op for CodexAgent since Connect() transfers subprocess ownership to Conn.
func (a *CodexAgent) Close() error { return nil }

// AgentPlugin returns the per-agent plugin for Codex.
func (a *CodexAgent) AgentPlugin() acp.AgentPlugin {
	return codexPlugin{}
}

// buildEnv converts a map of environment variables to "KEY=VALUE" strings.
func buildEnv(m map[string]string) []string {
	env := make([]string, 0, len(m))
	for k, v := range m {
		env = append(env, k+"="+v)
	}
	return env
}
