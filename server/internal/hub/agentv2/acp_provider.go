package agentv2

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
)

// Provider resolves launch details for one ACP agent type.
type Provider interface {
	Name() string
	LaunchSpec() (exe string, args []string, env []string, err error)
}

// ACPProvider is kept as a compatibility alias for Provider.
type ACPProvider = Provider

// ProviderConfig configures an ACP provider instance.
type ProviderConfig struct {
	ExePath string
	Env     map[string]string
}

// ACPProviderConfig is kept as a compatibility alias for ProviderConfig.
type ACPProviderConfig = ProviderConfig

// ProviderPreset declares launch behavior for one provider kind.
type ProviderPreset struct {
	Name                   string
	BinaryName             string
	Args                   []string
	NPMPackage             string
	ResolveBinaryOnEmpty   bool
	UseExePathDirect       bool
	MissingPathErrTemplate string
}

var (
	CodexProviderPreset = ProviderPreset{
		Name:       "codex",
		BinaryName: "codex-acp",
		NPMPackage: "@zed-industries/codex-acp",
	}
	ClaudeProviderPreset = ProviderPreset{
		Name:                 "claude",
		BinaryName:           "claude-agent-acp",
		NPMPackage:           "@agentclientprotocol/claude-agent-acp",
		ResolveBinaryOnEmpty: true,
	}
	CopilotProviderPreset = ProviderPreset{
		Name:                   "copilot",
		BinaryName:             "copilot",
		Args:                   []string{"--acp", "--stdio"},
		UseExePathDirect:       true,
		MissingPathErrTemplate: "copilot: binary not found in PATH (install GitHub Copilot CLI): %v",
	}
)

// provider is the unified implementation for all ACP providers.
type provider struct {
	preset ProviderPreset
	cfg    ProviderConfig

	resolveBinary func(name, configuredPath string) (string, error)
	lookPath      func(file string) (string, error)
}

// NewProvider creates a provider from preset + config.
func NewProvider(preset ProviderPreset, cfg ProviderConfig) *provider {
	return &provider{
		preset:        preset,
		cfg:           cfg,
		resolveBinary: ResolveACPBinary,
		lookPath:      exec.LookPath,
	}
}

// NewACPProvider is kept as a compatibility wrapper for NewProvider.
func NewACPProvider(preset ProviderPreset, cfg ProviderConfig) *provider {
	return NewProvider(preset, cfg)
}

func NewCodexProvider(cfg ProviderConfig) *provider {
	return NewProvider(CodexProviderPreset, cfg)
}

func NewClaudeProvider(cfg ProviderConfig) *provider {
	return NewProvider(ClaudeProviderPreset, cfg)
}

func NewCopilotProvider(cfg ProviderConfig) *provider {
	return NewProvider(CopilotProviderPreset, cfg)
}

func (p *provider) Name() string { return p.preset.Name }

func (p *provider) LaunchSpec() (string, []string, []string, error) {
	env := buildEnv(p.cfg.Env)
	args := cloneArgs(p.preset.Args)

	if p.cfg.ExePath != "" {
		if p.preset.UseExePathDirect {
			return p.cfg.ExePath, args, env, nil
		}
		exePath, err := p.resolveBinary(p.preset.BinaryName, p.cfg.ExePath)
		if err != nil {
			return "", nil, nil, fmt.Errorf("%s: resolve binary: %w", p.preset.Name, err)
		}
		return exePath, args, env, nil
	}

	if p.preset.ResolveBinaryOnEmpty {
		exePath, err := p.resolveBinary(p.preset.BinaryName, "")
		if err == nil {
			return exePath, args, env, nil
		}
		if p.preset.NPMPackage != "" {
			npxPath, npxErr := p.resolveNpx()
			if npxErr != nil {
				return "", nil, nil, fmt.Errorf("%s: resolve binary: %w; and npx not found: %w", p.preset.Name, err, npxErr)
			}
			return npxPath, []string{"--yes", p.preset.NPMPackage}, env, nil
		}
		return "", nil, nil, fmt.Errorf("%s: resolve binary: %w", p.preset.Name, err)
	}

	if p.preset.NPMPackage != "" {
		npxPath, err := p.resolveNpx()
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
	return exePath, args, env, nil
}

func (p *provider) resolveNpx() (string, error) {
	return p.lookPath("npx")
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

func cloneArgs(args []string) []string {
	if len(args) == 0 {
		return nil
	}
	return append([]string(nil), args...)
}

// ResolveACPBinary locates an ACP executable in this order:
//  1. configuredPath if non-empty and exists
//  2. PATH
//  3. bin/{GOOS}_{GOARCH}/ next to the running executable
//  4. bin/{GOOS}_{GOARCH}/ relative to current working directory
func ResolveACPBinary(name, configuredPath string) (string, error) {
	if configuredPath != "" {
		if exists(configuredPath) {
			abs, err := filepath.Abs(configuredPath)
			if err != nil {
				return "", fmt.Errorf("agentv2: abs path %q: %w", configuredPath, err)
			}
			return abs, nil
		}
	}

	path, err := exec.LookPath(name)
	if err == nil {
		return path, nil
	}

	if exeDir, dirErr := executableDir(); dirErr == nil {
		for _, binName := range binaryNames(name) {
			candidate := filepath.Join(exeDir, "bin", platformDir(), binName)
			if exists(candidate) {
				return candidate, nil
			}
		}
	}

	for _, binName := range binaryNames(name) {
		candidate := filepath.Join("bin", platformDir(), binName)
		if exists(candidate) {
			abs, absErr := filepath.Abs(candidate)
			if absErr == nil {
				return abs, nil
			}
		}
	}

	return "", fmt.Errorf(
		"agentv2: %q not found (tried configured path, PATH, and bin/%s/); install with npm: npm install -g @zed-industries/%s",
		name, platformDir(), name,
	)
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func platformDir() string {
	return runtime.GOOS + "_" + runtime.GOARCH
}

func binaryNames(name string) []string {
	if runtime.GOOS == "windows" {
		return []string{name + ".exe", name + ".cmd"}
	}
	return []string{name}
}

func executableDir() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Dir(exe), nil
}
