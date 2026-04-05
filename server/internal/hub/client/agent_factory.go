package client

import (
	"context"
	"fmt"
	"io"

	"github.com/swm8023/wheelmaker/internal/hub/agentv2"
)

// SessionCallbacks is the callback contract that Session provides to agentv2 runtime.
type SessionCallbacks interface {
	agentv2.Callbacks
}

// AgentFactoryV2 creates runtime instances and declares connection sharing policy.
type AgentFactoryV2 interface {
	Name() string
	SupportsSharedConn() bool
	CreateInstance(ctx context.Context, callbacks SessionCallbacks, debugLog io.Writer) (agentv2.Instance, error)
}

// acpProviderAgentFactory creates agent instances from agentv2 ACP providers.
type acpProviderAgentFactory struct {
	provider agentv2.ACPProvider
}

func (f *acpProviderAgentFactory) Name() string { return f.provider.Name() }

func (f *acpProviderAgentFactory) SupportsSharedConn() bool { return false }

func (f *acpProviderAgentFactory) CreateInstance(_ context.Context, cb SessionCallbacks, _ io.Writer) (agentv2.Instance, error) {
	conn, err := newOwnedProviderConn(f.provider)
	if err != nil {
		return nil, fmt.Errorf("connect %q: %w", f.provider.Name(), err)
	}
	return agentv2.NewInstance(f.provider.Name(), conn, cb), nil
}

// NewACPProviderFactory adapts an agentv2.ACPProvider to client.AgentFactoryV2.
func NewACPProviderFactory(provider agentv2.ACPProvider) AgentFactoryV2 {
	return &acpProviderAgentFactory{provider: provider}
}

func newOwnedProviderConn(provider agentv2.ACPProvider) (agentv2.Conn, error) {
	exe, args, env, err := provider.Launch()
	if err != nil {
		return nil, err
	}
	raw := agentv2.NewACPProcess(exe, env, args...)
	if err := raw.Start(); err != nil {
		return nil, err
	}
	return agentv2.NewOwnedConn(raw), nil
}
