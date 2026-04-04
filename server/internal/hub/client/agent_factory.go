package client

import (
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/swm8023/wheelmaker/internal/hub/acp"
	"github.com/swm8023/wheelmaker/internal/hub/agentv2"
)

// AgentFactoryV2 creates AgentInstance objects and declares connection sharing policy.
type AgentFactoryV2 interface {
	Name() string
	SupportsSharedConn() bool
	CreateInstance(ctx context.Context, callbacks SessionCallbacks, debugLog io.Writer) (*AgentInstance, error)
}

// providerAgentFactory creates agent instances from agentv2 providers.
type providerAgentFactory struct {
	provider agentv2.Provider
}

func (f *providerAgentFactory) Name() string { return f.provider.Name() }

func (f *providerAgentFactory) SupportsSharedConn() bool { return false }

func (f *providerAgentFactory) CreateInstance(_ context.Context, cb SessionCallbacks, debugLog io.Writer) (*AgentInstance, error) {
	exe, args, env, err := f.provider.LaunchSpec()
	if err != nil {
		return nil, err
	}
	raw := acp.NewConn(exe, env, args...)
	if debugLog != nil {
		raw.SetDebugLogger(debugLog)
	}
	if err := raw.Start(); err != nil {
		return nil, fmt.Errorf("connect %q: %w", f.provider.Name(), err)
	}
	runtimeConn := agentv2.New(raw)
	runtimeInst := agentv2.NewInstance(f.provider.Name(), runtimeConn, cb)
	return &AgentInstance{name: f.provider.Name(), runtime: runtimeInst, callbacks: cb}, nil
}

// NewProviderFactory adapts an agentv2.Provider to client.AgentFactoryV2.
func NewProviderFactory(provider agentv2.Provider) AgentFactoryV2 {
	return &providerAgentFactory{provider: provider}
}

// legacyAgentFactory wraps the old AgentFactory function into an AgentFactoryV2.
// Each call to CreateInstance creates a new connection (owned mode).
type legacyAgentFactory struct {
	name string
	fn   AgentFactory
}

func (f *legacyAgentFactory) Name() string             { return f.name }
func (f *legacyAgentFactory) SupportsSharedConn() bool { return false }

func (f *legacyAgentFactory) CreateInstance(ctx context.Context, cb SessionCallbacks, debugLog io.Writer) (*AgentInstance, error) {
	a := f.fn("", nil)
	conn, err := a.Connect(ctx)
	if err != nil {
		return nil, fmt.Errorf("connect %q: %w", f.name, err)
	}

	ac := newOwnedAgentConn(a, conn, debugLog)

	inst := &AgentInstance{
		name:      f.name,
		agentConn: ac,
		callbacks: cb,
	}

	// Owned mode: route all callbacks directly to the instance's Session.
	ac.SetCallbacks(cb)

	return inst, nil
}

// sharedAgentFactory wraps an AgentFactory and manages a shared AgentConn
// that is reused across multiple AgentInstance objects.
type sharedAgentFactory struct {
	name string
	fn   AgentFactory

	mu   sync.Mutex
	conn *AgentConn // lazily created, shared across all instances
}

func (f *sharedAgentFactory) Name() string             { return f.name }
func (f *sharedAgentFactory) SupportsSharedConn() bool { return true }

func (f *sharedAgentFactory) CreateInstance(ctx context.Context, cb SessionCallbacks, debugLog io.Writer) (*AgentInstance, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	// Lazily create the shared connection on first use.
	if f.conn == nil {
		a := f.fn("", nil)
		rawConn, err := a.Connect(ctx)
		if err != nil {
			return nil, fmt.Errorf("connect shared %q: %w", f.name, err)
		}
		f.conn = newSharedAgentConn(a, rawConn, debugLog)
	}

	inst := &AgentInstance{
		name:      f.name,
		agentConn: f.conn,
		callbacks: cb,
		shared:    true,
	}

	return inst, nil
}

// CloseConn closes the shared connection. Should be called during shutdown.
func (f *sharedAgentFactory) CloseConn() error {
	f.mu.Lock()
	c := f.conn
	f.conn = nil
	f.mu.Unlock()
	if c != nil {
		return c.Close()
	}
	return nil
}

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
