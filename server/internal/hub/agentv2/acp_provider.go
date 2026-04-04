package agentv2

import (
	"fmt"
	"os/exec"
	"sort"

	"github.com/swm8023/wheelmaker/internal/hub/tools"
)

// ACPProvider resolves launch details for one ACP agent type.
type ACPProvider interface {
	Name() string
	LaunchSpec() (exe string, args []string, env []string, err error)
}

// Provider is kept as a compatibility alias for ACPProvider.
type Provider = ACPProvider

// ACPProviderConfig configures an ACP provider instance.
type ACPProviderConfig struct {
	ExePath string
	Env     map[string]string
}

// Backward-compatible aliases for existing config names.
type CodexProviderConfig = ACPProviderConfig
type ClaudeProviderConfig = ACPProviderConfig
type CopilotProviderConfig = ACPProviderConfig

// ACPProviderPreset declares launch behavior for one provider kind.
type ACPProviderPreset struct {
	Name                   string
	BinaryName             string
	Args                   []string
	NPMPackage             string
	ResolveBinaryOnEmpty   bool
	UseExePathDirect       bool
	MissingPathErrTemplate string
}

var (
	CodexACPProviderPreset = ACPProviderPreset{
		Name:       "codex",
		BinaryName: "codex-acp",
		NPMPackage: "@zed-industries/codex-acp",
	}
	ClaudeACPProviderPreset = ACPProviderPreset{
		Name:                 "claude",
		BinaryName:           "claude-agent-acp",
		NPMPackage:           "@agentclientprotocol/claude-agent-acp",
		ResolveBinaryOnEmpty: true,
	}
	CopilotACPProviderPreset = ACPProviderPreset{
		Name:                   "copilot",
		BinaryName:             "copilot",
		Args:                   []string{"--acp", "--stdio"},
		UseExePathDirect:       true,
		MissingPathErrTemplate: "copilot: binary not found in PATH (install GitHub Copilot CLI): %v",
	}
)

// acpProvider is the unified implementation for all ACP providers.
type acpProvider struct {
	preset ACPProviderPreset
	cfg    ACPProviderConfig

	resolveBinary func(name, configPath string) (string, error)
	lookPath      func(file string) (string, error)
}

// NewACPProvider creates a provider from preset + config.
func NewACPProvider(preset ACPProviderPreset, cfg ACPProviderConfig) *acpProvider {
	return &acpProvider{
		preset:        preset,
		cfg:           cfg,
		resolveBinary: tools.ResolveBinary,
		lookPath:      exec.LookPath,
	}
}

// Backward-compatible constructors.
func NewCodexProvider(cfg CodexProviderConfig) *acpProvider {
	return NewACPProvider(CodexACPProviderPreset, ACPProviderConfig(cfg))
}

func NewClaudeProvider(cfg ClaudeProviderConfig) *acpProvider {
	return NewACPProvider(ClaudeACPProviderPreset, ACPProviderConfig(cfg))
}

func NewCopilotProvider(cfg CopilotProviderConfig) *acpProvider {
	return NewACPProvider(CopilotACPProviderPreset, ACPProviderConfig(cfg))
}

func (p *acpProvider) Name() string { return p.preset.Name }

func (p *acpProvider) LaunchSpec() (string, []string, []string, error) {
	env := buildEnv(p.cfg.Env)

	if p.cfg.ExePath != "" {
		if p.preset.UseExePathDirect {
			return p.cfg.ExePath, append([]string(nil), p.preset.Args...), env, nil
		}
		exePath, err := p.resolveBinary(p.preset.BinaryName, p.cfg.ExePath)
		if err != nil {
			return "", nil, nil, fmt.Errorf("%s: resolve binary: %w", p.preset.Name, err)
		}
		return exePath, append([]string(nil), p.preset.Args...), env, nil
	}

	if p.preset.ResolveBinaryOnEmpty {
		exePath, err := p.resolveBinary(p.preset.BinaryName, "")
		if err == nil {
			return exePath, append([]string(nil), p.preset.Args...), env, nil
		}
		if p.preset.NPMPackage != "" {
			npxPath, npxErr := p.lookPath("npx")
			if npxErr != nil {
				return "", nil, nil, fmt.Errorf("%s: resolve binary: %w; and npx not found: %w", p.preset.Name, err, npxErr)
			}
			return npxPath, []string{"--yes", p.preset.NPMPackage}, env, nil
		}
		return "", nil, nil, fmt.Errorf("%s: resolve binary: %w", p.preset.Name, err)
	}

	if p.preset.NPMPackage != "" {
		npxPath, err := p.lookPath("npx")
		if err != nil {
			return "", nil, nil, fmt.Errorf("%s: npx not found: %w", p.preset.Name, err)
		}
		return npxPath, []string{"--yes", p.preset.NPMPackage}, env, nil
	}

	exePath, err := p.lookPath(p.preset.BinaryName)
	if err != nil {
		if p.preset.MissingPathErrTemplate != "" {
			return "", nil, nil, fmt.Errorf(p.preset.MissingPathErrTemplate, err)
		}
		return "", nil, nil, fmt.Errorf("%s: binary not found in PATH: %w", p.preset.Name, err)
	}
	return exePath, append([]string(nil), p.preset.Args...), env, nil
}

func buildEnv(m map[string]string) []string {
	if len(m) == 0 {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	env := make([]string, 0, len(keys))
	for _, k := range keys {
		env = append(env, k+"="+m[k])
	}
	return env
}
