package agentv2

import (
	"fmt"
	"os/exec"
)

const copilotProviderName = "copilot"

// CopilotProviderConfig configures the Copilot provider.
type CopilotProviderConfig struct {
	ExePath string
	Env     map[string]string
}

// CopilotProvider resolves launch spec for GitHub Copilot CLI ACP mode.
type CopilotProvider struct {
	cfg CopilotProviderConfig

	lookPath func(file string) (string, error)
}

func NewCopilotProvider(cfg CopilotProviderConfig) *CopilotProvider {
	return &CopilotProvider{
		cfg:      cfg,
		lookPath: exec.LookPath,
	}
}

func (p *CopilotProvider) Name() string { return copilotProviderName }

func (p *CopilotProvider) LaunchSpec() (string, []string, []string, error) {
	exePath, err := p.resolveExe()
	if err != nil {
		return "", nil, nil, err
	}
	return exePath, []string{"--acp", "--stdio"}, buildEnv(p.cfg.Env), nil
}

func (p *CopilotProvider) resolveExe() (string, error) {
	if p.cfg.ExePath != "" {
		return p.cfg.ExePath, nil
	}
	exePath, err := p.lookPath("copilot")
	if err != nil {
		return "", fmt.Errorf("copilot: binary not found in PATH (install GitHub Copilot CLI): %w", err)
	}
	return exePath, nil
}
