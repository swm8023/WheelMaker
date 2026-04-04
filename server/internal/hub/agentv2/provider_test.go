package agentv2

import (
	"errors"
	"reflect"
	"testing"
)

func TestCodexProvider_UsesNpxFallback(t *testing.T) {
	p := NewCodexProvider(CodexProviderConfig{})
	p.resolveBinary = func(_ string, _ string) (string, error) {
		return "", errors.New("not found")
	}
	p.lookPath = func(bin string) (string, error) {
		if bin != "npx" {
			t.Fatalf("lookPath bin=%q, want npx", bin)
		}
		return "/usr/bin/npx", nil
	}

	exe, args, env, err := p.LaunchSpec()
	if err != nil {
		t.Fatalf("launch spec: %v", err)
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

func TestClaudeProvider_UsesNpxFallback(t *testing.T) {
	p := NewClaudeProvider(ClaudeProviderConfig{})
	p.resolveBinary = func(_ string, _ string) (string, error) {
		return "", errors.New("not found")
	}
	p.lookPath = func(bin string) (string, error) {
		if bin != "npx" {
			t.Fatalf("lookPath bin=%q, want npx", bin)
		}
		return "/usr/bin/npx", nil
	}

	exe, args, _, err := p.LaunchSpec()
	if err != nil {
		t.Fatalf("launch spec: %v", err)
	}
	if exe != "/usr/bin/npx" {
		t.Fatalf("exe=%q", exe)
	}
	if len(args) != 2 || args[1] != "@agentclientprotocol/claude-agent-acp" {
		t.Fatalf("args=%v", args)
	}
}

func TestCopilotProvider_LaunchArgsAndEnv(t *testing.T) {
	p := NewCopilotProvider(CopilotProviderConfig{Env: map[string]string{"B": "2", "A": "1"}})
	p.lookPath = func(bin string) (string, error) {
		if bin != "copilot" {
			t.Fatalf("lookPath bin=%q, want copilot", bin)
		}
		return "/usr/bin/copilot", nil
	}

	exe, args, env, err := p.LaunchSpec()
	if err != nil {
		t.Fatalf("launch spec: %v", err)
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
