package client

import (
	"context"
	"fmt"
	"io"
)

// AgentFactoryV2 creates AgentInstance objects and declares connection sharing policy.
type AgentFactoryV2 interface {
	Name() string
	SupportsSharedConn() bool
	CreateInstance(ctx context.Context, callbacks SessionCallbacks, debugLog io.Writer) (*AgentInstance, error)
}

// legacyAgentFactory wraps the old AgentFactory function into an AgentFactoryV2.
// Each call to CreateInstance creates a new connection (owned mode).
type legacyAgentFactory struct {
	name string
	fn   AgentFactory
}

func (f *legacyAgentFactory) Name() string              { return f.name }
func (f *legacyAgentFactory) SupportsSharedConn() bool   { return false }

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
