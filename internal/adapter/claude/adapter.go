// Package claude implements an adapter.Adapter for Claude Code CLI via claude-agent-acp.
package claude

import (
	"context"
	"fmt"

	"github.com/swm8023/wheelmaker/internal/agent/acp"
	"github.com/swm8023/wheelmaker/internal/tools"
)

const adapterName = "claude"

// Config holds configuration for the ClaudeAdapter.
type Config struct {
	// ExePath is the path to the claude-agent-acp binary.
	// If empty, tools.ResolveBinary("claude-agent-acp", "") is used.
	ExePath string

	// Env is extra environment variables for the subprocess (e.g. ANTHROPIC_API_KEY).
	Env map[string]string
}

// ClaudeAdapter is a stateless connection factory for Claude Code CLI via claude-agent-acp.
// Each call to Connect() spawns a new subprocess.
type ClaudeAdapter struct {
	cfg Config
}

// NewAdapter creates a ClaudeAdapter with the given config.
func NewAdapter(cfg Config) *ClaudeAdapter {
	return &ClaudeAdapter{cfg: cfg}
}

// Name returns the adapter's identifier.
func (a *ClaudeAdapter) Name() string { return adapterName }

// Connect starts a new claude-agent-acp subprocess and returns an initialized *acp.Conn.
// Conn.Start() is called internally; the caller must NOT call Start() again.
func (a *ClaudeAdapter) Connect(_ context.Context) (*acp.Conn, error) {
	exePath, err := tools.ResolveBinary("claude-agent-acp", a.cfg.ExePath)
	if err != nil {
		return nil, fmt.Errorf("claude: resolve binary: %w", err)
	}

	env := buildEnv(a.cfg.Env)
	conn := acp.New(exePath, env)
	if err := conn.Start(); err != nil {
		return nil, fmt.Errorf("claude: start process: %w", err)
	}
	return conn, nil
}

// Close is a no-op for ClaudeAdapter since Connect() transfers subprocess ownership to Conn.
func (a *ClaudeAdapter) Close() error { return nil }

// buildEnv converts a map of environment variables to "KEY=VALUE" strings.
func buildEnv(m map[string]string) []string {
	env := make([]string, 0, len(m))
	for k, v := range m {
		env = append(env, k+"="+v)
	}
	return env
}
