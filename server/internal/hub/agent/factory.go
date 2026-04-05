package agent

import (
	"context"
	"fmt"
	"io"
	"sort"
	"sync"

	"github.com/swm8023/wheelmaker/internal/protocol"
)

// Factory creates runtime instances and declares connection sharing policy.
type Factory interface {
	Name() string
	SupportsSharedConn() bool
	CreateInstance(ctx context.Context, callbacks Callbacks, debugLog io.Writer) (Instance, error)
}

// ACPFactory is a built-in provider preset factory keyed by ACP provider enum.
type ACPFactory struct {
	mu      sync.RWMutex
	presets map[protocol.ACPProvider]Factory
}

var (
	defaultACPFactoryOnce sync.Once
	defaultACPFactory     *ACPFactory
)

// DefaultACPFactory returns the singleton built-in ACP factory.
func DefaultACPFactory() *ACPFactory {
	defaultACPFactoryOnce.Do(func() {
		defaultACPFactory = &ACPFactory{
			presets: map[protocol.ACPProvider]Factory{
				protocol.ACPProviderCodex:   NewACPProviderFactory(NewCodexProvider(ACPProviderConfig{})),
				protocol.ACPProviderClaude:  NewACPProviderFactory(NewClaudeProvider(ACPProviderConfig{})),
				protocol.ACPProviderCopilot: NewACPProviderFactory(NewCopilotProvider(ACPProviderConfig{})),
			},
		}
	})
	return defaultACPFactory
}

// Get returns a preset factory by provider enum.
func (f *ACPFactory) Get(provider protocol.ACPProvider) Factory {
	if f == nil {
		return nil
	}
	f.mu.RLock()
	preset := f.presets[provider]
	f.mu.RUnlock()
	return preset
}

// Names returns built-in provider names in stable order.
func (f *ACPFactory) Names() []string {
	if f == nil {
		return nil
	}
	f.mu.RLock()
	names := make([]string, 0, len(f.presets))
	for provider := range f.presets {
		names = append(names, string(provider))
	}
	f.mu.RUnlock()
	sort.Strings(names)
	return names
}

// CreateInstance creates an instance by provider enum using preset config.
func (f *ACPFactory) CreateInstance(ctx context.Context, provider protocol.ACPProvider, callbacks Callbacks, debugLog io.Writer) (Instance, error) {
	preset := f.Get(provider)
	if preset == nil {
		return nil, fmt.Errorf("unknown provider: %q", provider)
	}
	return preset.CreateInstance(ctx, callbacks, debugLog)
}

// acpProviderFactory creates agent instances from ACP providers.
type acpProviderFactory struct {
	provider ACPProvider
}

func (f *acpProviderFactory) Name() string { return f.provider.Name() }

func (f *acpProviderFactory) SupportsSharedConn() bool { return false }

func (f *acpProviderFactory) CreateInstance(_ context.Context, cb Callbacks, _ io.Writer) (Instance, error) {
	conn, err := newOwnedProviderConn(f.provider)
	if err != nil {
		return nil, fmt.Errorf("connect %q: %w", f.provider.Name(), err)
	}
	return NewInstance(f.provider.Name(), conn, cb), nil
}

// NewACPProviderFactory adapts an ACPProvider to Factory.
func NewACPProviderFactory(provider ACPProvider) Factory {
	return &acpProviderFactory{provider: provider}
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
