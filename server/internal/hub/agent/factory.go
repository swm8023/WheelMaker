package agent

import (
	"context"
	"fmt"
	"io"
)

// Factory creates runtime instances and declares connection sharing policy.
type Factory interface {
	Name() string
	SupportsSharedConn() bool
	CreateInstance(ctx context.Context, callbacks Callbacks, debugLog io.Writer) (Instance, error)
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
