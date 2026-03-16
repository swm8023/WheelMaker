// Package claude implements an agent.Agent for Claude Code CLI via claude-agent-acp.
package claude

import (
	"context"
	"fmt"

	"github.com/swm8023/wheelmaker/internal/agent"
	"github.com/swm8023/wheelmaker/internal/tools"
)

const agentName = "claude"

// Config holds configuration for the ClaudeAgent.
type Config struct {
	// ExePath is the path to the claude-agent-acp binary.
	// If empty, tools.ResolveBinary("claude-agent-acp", "") is used.
	ExePath string

	// Env is extra environment variables for the subprocess (e.g. ANTHROPIC_API_KEY).
	Env map[string]string
}

// ClaudeAgent is a stateless connection factory for Claude Code CLI via claude-agent-acp.
// Each call to Connect() spawns a new subprocess.
type ClaudeAgent struct {
	cfg Config
}

// NewAgent creates a ClaudeAgent with the given config.
func NewAgent(cfg Config) *ClaudeAgent {
	return &ClaudeAgent{cfg: cfg}
}

// Name returns the agent identifier.
func (a *ClaudeAgent) Name() string { return agentName }

// Connect starts a new claude-agent-acp subprocess and returns an initialized *agent.Conn.
// Conn.Start() is called internally; the caller must NOT call Start() again.
func (a *ClaudeAgent) Connect(_ context.Context) (*agent.Conn, error) {
	exePath, err := tools.ResolveBinary("claude-agent-acp", a.cfg.ExePath)
	if err != nil {
		return nil, fmt.Errorf("claude: resolve binary: %w", err)
	}

	env := buildEnv(a.cfg.Env)
	conn := agent.New(exePath, env)
	if err := conn.Start(); err != nil {
		return nil, fmt.Errorf("claude: start process: %w", err)
	}
	return conn, nil
}

// Close is a no-op for ClaudeAgent since Connect() transfers subprocess ownership to Conn.
func (a *ClaudeAgent) Close() error { return nil }

// buildEnv converts a map of environment variables to "KEY=VALUE" strings.
func buildEnv(m map[string]string) []string {
	env := make([]string, 0, len(m))
	for k, v := range m {
		env = append(env, k+"="+v)
	}
	return env
}
