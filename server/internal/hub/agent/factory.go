package agent

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/swm8023/wheelmaker/internal/protocol"
)

// InstanceCreator creates one runtime instance.
type InstanceCreator func(ctx context.Context) (Instance, error)

// ACPFactory resolves one provider enum to one instance creator.
// A creator map contains both default built-ins and optional overrides.
type ACPFactory struct {
	mu       sync.RWMutex
	creators map[protocol.ACPProvider]InstanceCreator
}

var (
	defaultACPFactoryOnce sync.Once
	defaultACPFactory     *ACPFactory
)

// DefaultACPFactory returns the singleton ACP factory.
func DefaultACPFactory() *ACPFactory {
	defaultACPFactoryOnce.Do(func() {
		defaultACPFactory = newACPFactoryWithDefaults()
	})
	return defaultACPFactory
}

func newACPFactoryWithDefaults() *ACPFactory {
	f := &ACPFactory{creators: map[protocol.ACPProvider]InstanceCreator{}}
	candidates := []struct {
		provider protocol.ACPProvider
		build    func() ACPProvider
	}{
		{provider: protocol.ACPProviderCodex, build: func() ACPProvider { return NewCodexProvider() }},
		{provider: protocol.ACPProviderClaude, build: func() ACPProvider { return NewClaudeProvider() }},
		{provider: protocol.ACPProviderCopilot, build: func() ACPProvider { return NewCopilotProvider() }},
		{provider: protocol.ACPProviderCodeflicker, build: func() ACPProvider { return NewCodeflickerProvider() }},
		{provider: protocol.ACPProviderOpenCode, build: func() ACPProvider { return NewOpenCodeProvider() }},
		{provider: protocol.ACPProviderCodeBuddy, build: func() ACPProvider { return NewCodeBuddyProvider() }},
	}
	for _, candidate := range candidates {
		prov := candidate.build()
		if !isProviderAvailable(prov) {
			continue
		}
		f.Register(candidate.provider, providerInstanceCreator(prov))
	}
	if len(f.Names()) == 0 {
		agentLogger().Warn("no available ACP providers detected")
	}
	return f
}

// Clone returns a shallow copy of the creator registry.
func (f *ACPFactory) Clone() *ACPFactory {
	if f == nil {
		return nil
	}
	f.mu.RLock()
	defer f.mu.RUnlock()
	cp := &ACPFactory{creators: make(map[protocol.ACPProvider]InstanceCreator, len(f.creators))}
	for provider, creator := range f.creators {
		cp.creators[provider] = creator
	}
	return cp
}

// Register binds a provider to a creator. Existing binding is replaced.
func (f *ACPFactory) Register(provider protocol.ACPProvider, creator InstanceCreator) {
	if f == nil || creator == nil {
		return
	}
	f.mu.Lock()
	if f.creators == nil {
		f.creators = map[protocol.ACPProvider]InstanceCreator{}
	}
	f.creators[provider] = creator
	f.mu.Unlock()
	agentLogger().Info("registered provider=%s", provider)
}

// Creator returns a creator by provider enum.
func (f *ACPFactory) Creator(provider protocol.ACPProvider) InstanceCreator {
	if f == nil {
		return nil
	}
	f.mu.RLock()
	creator := f.creators[provider]
	f.mu.RUnlock()
	return creator
}

// CreatorByName resolves provider name then returns its creator.
func (f *ACPFactory) CreatorByName(name string) InstanceCreator {
	provider, ok := protocol.ParseACPProvider(strings.ToLower(strings.TrimSpace(name)))
	if !ok {
		return nil
	}
	return f.Creator(provider)
}

// Names returns provider names in stable order.
func (f *ACPFactory) Names() []string {
	if f == nil {
		return nil
	}
	f.mu.RLock()
	names := make([]string, 0, len(f.creators))
	for provider := range f.creators {
		names = append(names, string(provider))
	}
	f.mu.RUnlock()
	sort.Strings(names)
	return names
}

// PreferredName returns the preferred available provider name.
func (f *ACPFactory) PreferredName() string {
	if f == nil {
		return ""
	}
	ordered := []protocol.ACPProvider{
		protocol.ACPProviderCodex,
		protocol.ACPProviderClaude,
		protocol.ACPProviderCopilot,
		protocol.ACPProviderCodeflicker,
		protocol.ACPProviderOpenCode,
		protocol.ACPProviderCodeBuddy,
	}
	f.mu.RLock()
	defer f.mu.RUnlock()
	for _, provider := range ordered {
		if f.creators[provider] != nil {
			return string(provider)
		}
	}
	return ""
}

// CreateInstance creates one runtime instance by provider enum.
func (f *ACPFactory) CreateInstance(ctx context.Context, provider protocol.ACPProvider) (Instance, error) {
	creator := f.Creator(provider)
	if creator == nil {
		return nil, fmt.Errorf("unknown provider: %q", provider)
	}
	return creator(ctx)
}

func providerInstanceCreator(provider ACPProvider) InstanceCreator {
	return func(_ context.Context) (Instance, error) {
		conn, err := newOwnedProviderConn(provider)
		if err != nil {
			return nil, fmt.Errorf("connect %q: %w", provider.Name(), err)
		}
		return NewInstance(provider.Name(), conn), nil
	}
}

func newOwnedProviderConn(provider ACPProvider) (Conn, error) {
	exe, args, env, err := provider.Launch()
	if err != nil {
		return nil, err
	}
	raw := NewACPProcess(provider.Name(), exe, env, args...)
	if err := raw.Start(); err != nil {
		return nil, err
	}
	return NewOwnedConn(raw), nil
}

func isProviderAvailable(provider ACPProvider) bool {
	if provider == nil {
		return false
	}
	_, _, _, err := provider.Launch()
	if err != nil {
		agentLogger().Warn("skip provider=%s reason=%v", provider.Name(), err)
		return false
	}
	return true
}
