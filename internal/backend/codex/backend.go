// Package codex implements a backend.Backend for the Codex CLI via codex-acp.
package codex

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/swm8023/wheelmaker/internal/acp"
	"github.com/swm8023/wheelmaker/internal/tools"
)

const backendName = "codex"

// Config holds configuration for the Backend.
type Config struct {
	// ExePath is the path to the codex-acp binary.
	// If empty, tools.ResolveBinary("codex-acp", "") is used.
	ExePath string

	// Env is extra environment variables for the subprocess (e.g. OPENAI_API_KEY).
	Env map[string]string
}

// Backend is a stateless connection factory for the Codex CLI.
// Each call to Connect() spawns a new codex-acp subprocess.
type Backend struct {
	cfg Config
}

// New creates a Backend with the given config.
func New(cfg Config) *Backend {
	return &Backend{cfg: cfg}
}

// Name returns the backend identifier.
func (p *Backend) Name() string { return backendName }

// Connect starts a new codex-acp subprocess and returns an initialized *acp.Conn.
// Conn.Start() is called internally; the caller must NOT call Start() again.
func (p *Backend) Connect(_ context.Context) (*acp.Conn, error) {
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
func (p *Backend) Close() error { return nil }

// HandlePermission resolves permission by current mode:
// - reject/deny/read -> reject_once
// - ask/manual/user  -> cancelled (explicit user decision required)
// - others           -> allow_once
func (p *Backend) HandlePermission(_ context.Context, params acp.PermissionRequestParams, mode string) (acp.PermissionResult, error) {
	normalizedMode := strings.ToLower(strings.TrimSpace(mode))
	preferredKind := "allow_once"
	switch normalizedMode {
	case "reject", "deny", "read":
		preferredKind = "reject_once"
	case "ask", "manual", "user":
		return acp.PermissionResult{Outcome: "cancelled"}, nil
	}

	optionID := ""
	for _, opt := range params.Options {
		if opt.Kind == preferredKind {
			optionID = opt.OptionID
			break
		}
	}
	if optionID == "" {
		return acp.PermissionResult{Outcome: "cancelled"}, nil
	}
	return acp.PermissionResult{Outcome: "selected", OptionID: optionID}, nil
}

// NormalizeParams passes notifications through unchanged.
func (p *Backend) NormalizeParams(_ string, params json.RawMessage) json.RawMessage { return params }

// buildEnv converts a map of environment variables to "KEY=VALUE" strings.
func buildEnv(m map[string]string) []string {
	env := make([]string, 0, len(m))
	for k, v := range m {
		env = append(env, k+"="+v)
	}
	return env
}
