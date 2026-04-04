package agentv2

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
)

// ACPProvider resolves launch details for one ACP agent type.
type ACPProvider interface {
	Name() string
	Launch() (exe string, args []string, env []string, err error)
}

// ACPProviderConfig configures an ACP provider instance.
type ACPProviderConfig struct {
	ExePath string
	Env     map[string]string
}

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

	resolveBinary func(name, configuredPath string) (string, error)
	lookPath      func(file string) (string, error)
}

// NewACPProvider creates a provider from preset + config.
func NewACPProvider(preset ACPProviderPreset, cfg ACPProviderConfig) *acpProvider {
	return &acpProvider{
		preset:        preset,
		cfg:           cfg,
		resolveBinary: ResolveACPBinary,
		lookPath:      exec.LookPath,
	}
}

func NewCodexProvider(cfg ACPProviderConfig) *acpProvider {
	return NewACPProvider(CodexACPProviderPreset, cfg)
}

func NewClaudeProvider(cfg ACPProviderConfig) *acpProvider {
	return NewACPProvider(ClaudeACPProviderPreset, cfg)
}

func NewCopilotProvider(cfg ACPProviderConfig) *acpProvider {
	return NewACPProvider(CopilotACPProviderPreset, cfg)
}

func (p *acpProvider) Name() string { return p.preset.Name }

func (p *acpProvider) Launch() (string, []string, []string, error) {
	env := buildEnv(p.cfg.Env)
	defaultArgs := cloneArgs(p.preset.Args)

	if p.cfg.ExePath != "" {
		return p.launchFromConfigured(defaultArgs, env)
	}

	if p.preset.ResolveBinaryOnEmpty {
		exePath, err := p.resolveBinary(p.preset.BinaryName, "")
		if err == nil {
			return exePath, defaultArgs, env, nil
		}
		if p.preset.NPMPackage == "" {
			return "", nil, nil, fmt.Errorf("%s: resolve binary: %w", p.preset.Name, err)
		}
		npxPath, npxArgs, npxErr := p.launchWithNpx()
		if npxErr != nil {
			return "", nil, nil, fmt.Errorf("%s: resolve binary: %w; and npx not found: %w", p.preset.Name, err, npxErr)
		}
		return npxPath, npxArgs, env, nil
	}

	if p.preset.NPMPackage != "" {
		npxPath, npxArgs, err := p.launchWithNpx()
		if err != nil {
			return "", nil, nil, fmt.Errorf("%s: npx not found: %w", p.preset.Name, err)
		}
		return npxPath, npxArgs, env, nil
	}

	exePath, err := p.lookPath(p.preset.BinaryName)
	if err != nil {
		if p.preset.MissingPathErrTemplate != "" {
			return "", nil, nil, fmt.Errorf(p.preset.MissingPathErrTemplate, err)
		}
		return "", nil, nil, fmt.Errorf("%s: binary not found in PATH: %w", p.preset.Name, err)
	}
	return exePath, defaultArgs, env, nil
}

func (p *acpProvider) launchFromConfigured(defaultArgs, env []string) (string, []string, []string, error) {
	if p.preset.UseExePathDirect {
		return p.cfg.ExePath, defaultArgs, env, nil
	}
	exePath, err := p.resolveBinary(p.preset.BinaryName, p.cfg.ExePath)
	if err != nil {
		return "", nil, nil, fmt.Errorf("%s: resolve binary: %w", p.preset.Name, err)
	}
	return exePath, defaultArgs, env, nil
}

func (p *acpProvider) launchWithNpx() (string, []string, error) {
	npxPath, err := p.resolveNpx()
	if err != nil {
		return "", nil, err
	}
	return npxPath, []string{"--yes", p.preset.NPMPackage}, nil
}

func (p *acpProvider) resolveNpx() (string, error) {
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
