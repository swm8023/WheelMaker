package agent

import (
	"fmt"
	"os/exec"
)

// codexAppServerProvider launches the native Codex app-server ACP bridge.
type codexAppServerProvider struct {
	lookPath func(file string) (string, error)
}

func NewCodexAppServerProvider() *codexAppServerProvider {
	return &codexAppServerProvider{
		lookPath: exec.LookPath,
	}
}

func (p *codexAppServerProvider) Name() string {
	return "codex-app"
}

func (p *codexAppServerProvider) Launch() (string, []string, []string, error) {
	lookPath := p.lookPath
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	exe, err := lookPath("codex")
	if err != nil {
		return "", nil, nil, fmt.Errorf("codex-app: codex not found: %w", err)
	}
	return exe, []string{"app-server", "--listen", "stdio://"}, nil, nil
}
