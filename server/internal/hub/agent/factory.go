package agent

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/swm8023/wheelmaker/internal/protocol"
)

// InstanceCreator creates one runtime instance.
type InstanceCreator func(ctx context.Context) (Instance, error)

// ACPFactory is a built-in provider preset factory keyed by ACP provider enum.
type ACPFactory struct {
	providers map[protocol.ACPProvider]ACPProvider
}

var (
	defaultACPFactoryOnce sync.Once
	defaultACPFactory     *ACPFactory
)

// DefaultACPFactory returns the singleton built-in ACP factory.
func DefaultACPFactory() *ACPFactory {
	defaultACPFactoryOnce.Do(func() {
		defaultACPFactory = &ACPFactory{
			providers: map[protocol.ACPProvider]ACPProvider{
				protocol.ACPProviderCodex:   NewCodexProvider(),
				protocol.ACPProviderClaude:  NewClaudeProvider(),
				protocol.ACPProviderCopilot: NewCopilotProvider(),
			},
		}
	})
	return defaultACPFactory
}

// Creator returns an instance creator by provider enum.
func (f *ACPFactory) Creator(provider protocol.ACPProvider) InstanceCreator {
	if f == nil {
		return nil
	}
	if _, ok := f.providers[provider]; !ok {
		return nil
	}
	return func(ctx context.Context) (Instance, error) {
		return f.CreateInstance(ctx, provider)
	}
}

// Names returns built-in provider names in stable order.
func (f *ACPFactory) Names() []string {
	if f == nil {
		return nil
	}
	names := make([]string, 0, len(f.providers))
	for provider := range f.providers {
		names = append(names, string(provider))
	}
	sort.Strings(names)
	return names
}

// CreateInstance creates an instance by provider enum using preset config.
func (f *ACPFactory) CreateInstance(ctx context.Context, provider protocol.ACPProvider) (Instance, error) {
	if f == nil {
		return nil, fmt.Errorf("factory is nil")
	}
	preset := f.providers[provider]
	if preset == nil {
		return nil, fmt.Errorf("unknown provider: %q", provider)
	}
	conn, err := newOwnedProviderConn(preset)
	if err != nil {
		return nil, fmt.Errorf("connect %q: %w", preset.Name(), err)
	}
	return NewInstance(preset.Name(), conn), nil
}

func newOwnedProviderConn(provider ACPProvider) (Conn, error) {
	exe, args, env, err := provider.Launch()
	if err != nil {
		return nil, err
	}
	raw := NewACPProcess(exe, env, args...)
	if err := raw.Start(); err != nil {
		return nil, err
	}
	return NewOwnedConn(raw), nil
}
