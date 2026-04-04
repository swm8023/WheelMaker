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
	pool     agentv2.ConnPool
}

func (f *acpProviderAgentFactory) Name() string { return f.provider.Name() }

func (f *acpProviderAgentFactory) SupportsSharedConn() bool { return false }

func (f *acpProviderAgentFactory) CreateInstance(_ context.Context, cb SessionCallbacks, _ io.Writer) (agentv2.Instance, error) {
	if f.pool == nil {
		f.pool = newACPProviderConnPool(f.provider)
	}
	conn, err := f.pool.Open()
	if err != nil {
		return nil, fmt.Errorf("connect %q: %w", f.provider.Name(), err)
	}
	return agentv2.NewInstance(f.provider.Name(), conn, cb), nil
}

// NewACPProviderFactory adapts an agentv2.ACPProvider to client.AgentFactoryV2.
func NewACPProviderFactory(provider agentv2.ACPProvider) AgentFactoryV2 {
	return &acpProviderAgentFactory{
		provider: provider,
		pool:     newACPProviderConnPool(provider),
	}
}

// NewProviderFactory is kept as a compatibility wrapper.
func NewProviderFactory(provider agentv2.ACPProvider) AgentFactoryV2 {
	return NewACPProviderFactory(provider)
}

func newACPProviderConnPool(provider agentv2.ACPProvider) agentv2.ConnPool {
	connect := func() (agentv2.Conn, error) {
		exe, args, env, err := provider.LaunchSpec()
		if err != nil {
			return nil, err
		}
		raw := agentv2.NewACPProcess(exe, env, args...)
		if err := raw.Start(); err != nil {
			return nil, err
		}
		return raw, nil
	}
	// Providers default to owned-conn mode for strict isolation.
	return agentv2.NewOwnedConnPool(connect)
}

// legacyAgentFactory wraps the old AgentFactory function into an AgentFactoryV2.
// Each call to CreateInstance creates a new connection (owned mode).
type legacyAgentFactory struct {
	name string
	fn   AgentFactory
}

func (f *legacyAgentFactory) Name() string             { return f.name }
func (f *legacyAgentFactory) SupportsSharedConn() bool { return false }

func (f *legacyAgentFactory) CreateInstance(ctx context.Context, cb SessionCallbacks, debugLog io.Writer) (agentv2.Instance, error) {
	a := f.fn("", nil)
	conn, err := a.Connect(ctx)
	if err != nil {
		return nil, fmt.Errorf("connect %q: %w", f.name, err)
	}
	if debugLog != nil {
		conn.SetDebugLogger(debugLog)
	}
	return agentv2.NewInstance(f.name, wrapACPConn(conn), cb), nil
}

// sharedAgentFactory keeps API compatibility; runtime currently uses one instance per conn.
type sharedAgentFactory struct {
	name string
	fn   AgentFactory
}

func (f *sharedAgentFactory) Name() string             { return f.name }
func (f *sharedAgentFactory) SupportsSharedConn() bool { return true }

func (f *sharedAgentFactory) CreateInstance(ctx context.Context, cb SessionCallbacks, debugLog io.Writer) (agentv2.Instance, error) {
	legacy := &legacyAgentFactory{name: f.name, fn: f.fn}
	return legacy.CreateInstance(ctx, cb, debugLog)
}

// CloseConn is kept for compatibility with previous shared factory shape.
func (f *sharedAgentFactory) CloseConn() error { return nil }

// agentRegistryV2 maps agent names to their AgentFactoryV2 implementations.
type agentRegistryV2 struct {
	facs map[string]AgentFactoryV2
}

func newAgentRegistryV2() *agentRegistryV2 {
	return &agentRegistryV2{facs: make(map[string]AgentFactoryV2)}
}

func (r *agentRegistryV2) register(name string, f AgentFactoryV2) {
	r.facs[name] = f
}

func (r *agentRegistryV2) get(name string) AgentFactoryV2 {
	return r.facs[name]
}

func (r *agentRegistryV2) names() []string {
	ns := make([]string, 0, len(r.facs))
	for n := range r.facs {
		ns = append(ns, n)
	}
	return ns
}

// wrapLegacyFactory converts an old-style AgentFactory func into AgentFactoryV2.
func wrapLegacyFactory(name string, fn AgentFactory) AgentFactoryV2 {
	return &legacyAgentFactory{name: name, fn: fn}
}

// wrapSharedFactory converts an old-style AgentFactory func into a shared AgentFactoryV2.
func wrapSharedFactory(name string, fn AgentFactory) AgentFactoryV2 {
	return &sharedAgentFactory{name: name, fn: fn}
}
