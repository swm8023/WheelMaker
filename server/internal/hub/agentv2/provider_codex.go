package agentv2

import (
	"fmt"
	"os/exec"

	"github.com/swm8023/wheelmaker/internal/hub/tools"
)

const codexProviderName = "codex"

// CodexProviderConfig configures the Codex provider.
type CodexProviderConfig struct {
	ExePath string
	Env     map[string]string
}

// CodexProvider resolves launch spec for codex-acp.
type CodexProvider struct {
	cfg CodexProviderConfig

	resolveBinary func(name, configPath string) (string, error)
	lookPath      func(file string) (string, error)
}

func NewCodexProvider(cfg CodexProviderConfig) *CodexProvider {
	return &CodexProvider{
		cfg:           cfg,
		resolveBinary: tools.ResolveBinary,
		lookPath:      exec.LookPath,
	}
}

func (p *CodexProvider) Name() string { return codexProviderName }

func (p *CodexProvider) LaunchSpec() (string, []string, []string, error) {
	env := buildEnv(p.cfg.Env)
	if p.cfg.ExePath != "" {
		exePath, err := p.resolveBinary("codex-acp", p.cfg.ExePath)
		if err != nil {
			return "", nil, nil, fmt.Errorf("codex: resolve binary: %w", err)
		}
		return exePath, nil, env, nil
	}

	npxPath, err := p.lookPath("npx")
	if err != nil {
		return "", nil, nil, fmt.Errorf("codex: npx not found: %w", err)
	}
	return npxPath, []string{"--yes", "@zed-industries/codex-acp"}, env, nil
}
