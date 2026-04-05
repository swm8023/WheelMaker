package client

import (
	"context"
	"fmt"
	"io"

	"github.com/swm8023/wheelmaker/internal/hub/agent"
)

// SessionCallbacks is the callback contract that Session provides to agent runtime.
type SessionCallbacks interface {
	agent.Callbacks
}

// AgentFactory creates runtime instances and declares connection sharing policy.
type AgentFactory interface {
	Name() string
	SupportsSharedConn() bool
	CreateInstance(ctx context.Context, callbacks SessionCallbacks, debugLog io.Writer) (agent.Instance, error)
}

// acpProviderAgentFactory creates agent instances from agent ACP providers.
type acpProviderAgentFactory struct {
	provider agent.ACPProvider
}

func (f *acpProviderAgentFactory) Name() string { return f.provider.Name() }

func (f *acpProviderAgentFactory) SupportsSharedConn() bool { return false }

func (f *acpProviderAgentFactory) CreateInstance(_ context.Context, cb SessionCallbacks, _ io.Writer) (agent.Instance, error) {
	conn, err := newOwnedProviderConn(f.provider)
	if err != nil {
		return nil, fmt.Errorf("connect %q: %w", f.provider.Name(), err)
	}
	return agent.NewInstance(f.provider.Name(), conn, cb), nil
}

// NewACPProviderFactory adapts an agent.ACPProvider to client.AgentFactory.
func NewACPProviderFactory(provider agent.ACPProvider) AgentFactory {
	return &acpProviderAgentFactory{provider: provider}
}

func newOwnedProviderConn(provider agent.ACPProvider) (agent.Conn, error) {
	exe, args, env, err := provider.Launch()
	if err != nil {
		return nil, err
	}
	raw := agent.NewACPProcess(exe, env, args...)
	if err := raw.Start(); err != nil {
		return nil, err
	}
	return agent.NewOwnedConn(raw), nil
}
