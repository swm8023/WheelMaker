package agentv2

import (
	"reflect"
	"testing"
)

func TestCodexACPProvider_UsesNpxByDefault(t *testing.T) {
	p := NewCodexProvider(ACPProviderConfig{})
	p.resolveBinary = func(name string, configuredPath string) (string, error) {
		t.Fatalf("resolveBinary should not be called: name=%q configuredPath=%q", name, configuredPath)
		return "", nil
	}
	p.lookPath = func(bin string) (string, error) {
		if bin != "npx" {
			t.Fatalf("lookPath bin=%q, want npx", bin)
		}
		return "/usr/bin/npx", nil
	}

	exe, args, env, err := p.Launch()
	if err != nil {
		t.Fatalf("launch: %v", err)
	}
	if exe != "/usr/bin/npx" {
		t.Fatalf("exe=%q", exe)
	}
	if len(args) == 0 || args[0] != "--yes" {
		t.Fatalf("args=%v", args)
	}
	if len(env) != 0 {
		t.Fatalf("env=%v, want empty", env)
	}
}

func TestClaudeACPProvider_UsesNpxByDefault(t *testing.T) {
	p := NewClaudeProvider(ACPProviderConfig{})
	p.resolveBinary = func(name string, configuredPath string) (string, error) {
		t.Fatalf("resolveBinary should not be called: name=%q configuredPath=%q", name, configuredPath)
		return "", nil
	}
	p.lookPath = func(bin string) (string, error) {
		if bin != "npx" {
			t.Fatalf("lookPath bin=%q, want npx", bin)
		}
		return "/usr/bin/npx", nil
	}

	exe, args, _, err := p.Launch()
	if err != nil {
		t.Fatalf("launch: %v", err)
	}
	if exe != "/usr/bin/npx" {
		t.Fatalf("exe=%q", exe)
	}
	if len(args) != 2 || args[1] != "@agentclientprotocol/claude-agent-acp" {
		t.Fatalf("args=%v", args)
	}
}

func TestCopilotACPProvider_LaunchArgsAndEnv(t *testing.T) {
	p := NewCopilotProvider(ACPProviderConfig{Env: map[string]string{"B": "2", "A": "1"}})
	p.resolveBinary = func(name string, configuredPath string) (string, error) {
		if name != "copilot" {
			t.Fatalf("resolveBinary name=%q, want copilot", name)
		}
		if configuredPath != "" {
			t.Fatalf("resolveBinary configuredPath=%q, want empty", configuredPath)
		}
		return "/usr/bin/copilot", nil
	}

	exe, args, env, err := p.Launch()
	if err != nil {
		t.Fatalf("launch: %v", err)
	}
	if exe != "/usr/bin/copilot" {
		t.Fatalf("exe=%q", exe)
	}
	if !reflect.DeepEqual(args, []string{"--acp", "--stdio"}) {
		t.Fatalf("args=%v", args)
	}
	if !reflect.DeepEqual(env, []string{"A=1", "B=2"}) {
		t.Fatalf("env=%v", env)
	}
}
