package agentv2

import (
	"fmt"
	"os/exec"

	"github.com/swm8023/wheelmaker/internal/hub/tools"
)

const claudeProviderName = "claude"

// ClaudeProviderConfig configures the Claude provider.
type ClaudeProviderConfig struct {
	ExePath string
	Env     map[string]string
}

// ClaudeProvider resolves launch spec for claude-agent-acp.
type ClaudeProvider struct {
	cfg ClaudeProviderConfig

	resolveBinary func(name, configPath string) (string, error)
	lookPath      func(file string) (string, error)
}

func NewClaudeProvider(cfg ClaudeProviderConfig) *ClaudeProvider {
	return &ClaudeProvider{
		cfg:           cfg,
		resolveBinary: tools.ResolveBinary,
		lookPath:      exec.LookPath,
	}
}

func (p *ClaudeProvider) Name() string { return claudeProviderName }

func (p *ClaudeProvider) LaunchSpec() (string, []string, []string, error) {
	env := buildEnv(p.cfg.Env)
	exePath, err := p.resolveBinary("claude-agent-acp", p.cfg.ExePath)
	if err == nil {
		return exePath, nil, env, nil
	}
	if p.cfg.ExePath != "" {
		return "", nil, nil, fmt.Errorf("claude: resolve binary: %w", err)
	}

	npxPath, npxErr := p.lookPath("npx")
	if npxErr != nil {
		return "", nil, nil, fmt.Errorf("claude: resolve binary: %w; and npx not found: %w", err, npxErr)
	}
	return npxPath, []string{"--yes", "@agentclientprotocol/claude-agent-acp"}, env, nil
}
