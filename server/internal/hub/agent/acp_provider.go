package agent

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// ACPProvider resolves launch details for one ACP agent type.
type ACPProvider interface {
	Name() string
	Launch() (exe string, args []string, env []string, err error)
}

// ACPProviderPreset declares launch behavior for one provider kind.
type ACPProviderPreset struct {
	Name                   string
	BinaryName             string
	Args                   []string
	NPMPackage             string
	MissingPathErrTemplate string
}

var (
	CodexACPProviderPreset = ACPProviderPreset{
		Name:       "codex",
		BinaryName: "codex-acp",
		NPMPackage: "@zed-industries/codex-acp",
	}
	ClaudeACPProviderPreset = ACPProviderPreset{
		Name:       "claude",
		BinaryName: "claude-agent-acp",
		NPMPackage: "@agentclientprotocol/claude-agent-acp",
	}
	CopilotACPProviderPreset = ACPProviderPreset{
		Name:                   "copilot",
		BinaryName:             "copilot",
		Args:                   []string{"--acp", "--stdio"},
		MissingPathErrTemplate: "copilot: binary not found in PATH (install GitHub Copilot CLI): %v",
	}
)

// acpProvider is the unified implementation for all ACP providers.
type acpProvider struct {
	preset ACPProviderPreset

	resolveBinary func(name, configuredPath string) (string, error)
	lookPath      func(file string) (string, error)
}

// NewACPProvider creates a provider from preset.
func NewACPProvider(preset ACPProviderPreset) *acpProvider {
	return &acpProvider{
		preset:        preset,
		resolveBinary: ResolveACPBinary,
		lookPath:      exec.LookPath,
	}
}

func NewCodexProvider() *acpProvider {
	return NewACPProvider(CodexACPProviderPreset)
}

func NewClaudeProvider() *acpProvider {
	return NewACPProvider(ClaudeACPProviderPreset)
}

func NewCopilotProvider() *acpProvider {
	return NewACPProvider(CopilotACPProviderPreset)
}

func (p *acpProvider) Name() string { return p.preset.Name }

func (p *acpProvider) Launch() (string, []string, []string, error) {
	defaultArgs := cloneArgs(p.preset.Args)

	if p.preset.NPMPackage != "" {
		npxPath, err := p.lookPath("npx")
		if err != nil {
			return "", nil, nil, fmt.Errorf("%s: npx not found: %w", p.preset.Name, err)
		}
		return npxPath, []string{"--yes", p.preset.NPMPackage}, nil, nil
	}

	exePath, err := p.resolveBinary(p.preset.BinaryName, "")
	if err != nil {
		if p.preset.MissingPathErrTemplate != "" {
			return "", nil, nil, fmt.Errorf(p.preset.MissingPathErrTemplate, err)
		}
		return "", nil, nil, fmt.Errorf("%s: resolve binary: %w", p.preset.Name, err)
	}
	return exePath, defaultArgs, nil, nil
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
				return "", fmt.Errorf("agent: abs path %q: %w", configuredPath, err)
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
		"agent: %q not found (tried configured path, PATH, and bin/%s/); install with npm: npm install -g @zed-industries/%s",
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
