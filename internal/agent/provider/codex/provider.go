// Package codex implements an provider.Provider for the Codex CLI via codex-acp.
package codex

import (
	"context"
	"fmt"

	acp "github.com/swm8023/wheelmaker/internal/agent/provider"
	"github.com/swm8023/wheelmaker/internal/tools"
)

const providerName = "codex"

// Config holds configuration for the CodexProvider.
type Config struct {
	// ExePath is the path to the codex-acp binary.
	// If empty, tools.ResolveBinary("codex-acp", "") is used.
	ExePath string

	// Env is extra environment variables for the subprocess (e.g. OPENAI_API_KEY).
	Env map[string]string
}

// CodexProvider is a stateless connection factory for the Codex CLI via codex-acp.
// Each Call to Connect() spawns a new subprocess.
type CodexProvider struct {
	cfg Config
}

// NewProvider creates a CodexProvider with the given config.
func NewProvider(cfg Config) *CodexProvider {
	return &CodexProvider{cfg: cfg}
}

// Name returns the provider's identifier.
func (a *CodexProvider) Name() string { return providerName }

// Connect starts a new codex-acp subprocess and returns an initialized *acp.Conn.
// Conn.Start() is called internally; the caller must NOT call Start() again.
func (a *CodexProvider) Connect(_ context.Context) (*acp.Conn, error) {
	exePath, err := tools.ResolveBinary("codex-acp", a.cfg.ExePath)
	if err != nil {
		return nil, fmt.Errorf("codex: resolve binary: %w", err)
	}

	env := buildEnv(a.cfg.Env)
	conn := acp.New(exePath, env)
	if err := conn.Start(); err != nil {
		return nil, fmt.Errorf("codex: start process: %w", err)
	}
	return conn, nil
}

// Close is a no-op for CodexProvider since Connect() transfers subprocess ownership to Conn.
func (a *CodexProvider) Close() error { return nil }

// buildEnv converts a map of environment variables to "KEY=VALUE" strings.
func buildEnv(m map[string]string) []string {
	env := make([]string, 0, len(m))
	for k, v := range m {
		env = append(env, k+"="+v)
	}
	return env
}



