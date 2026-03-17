package acp

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// recordingConfigValidator tracks concurrent ValidateConfigOptions calls.
// It embeds DefaultPlugin so it satisfies AgentPlugin without implementing
// HandlePermission or NormalizeParams.
type recordingConfigValidator struct {
	DefaultPlugin
	active int32
	max    int32
}

func (v *recordingConfigValidator) ValidateConfigOptions(_ []ConfigOption) error {
	n := atomic.AddInt32(&v.active, 1)
	for {
		m := atomic.LoadInt32(&v.max)
		if n <= m || atomic.CompareAndSwapInt32(&v.max, m, n) {
			break
		}
	}
	time.Sleep(20 * time.Millisecond)
	atomic.AddInt32(&v.active, -1)
	return nil
}

func TestSetConfigOptions_ValidatorCallsSerialized(t *testing.T) {
	v := &recordingConfigValidator{}
	ag := New("test", nil, ".", v) // v is passed as the plugin at construction

	opts := []ConfigOption{{ID: "model", Category: "model", CurrentValue: "gpt-4.1"}}
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)
	for i := 0; i < 2; i++ {
		go func() {
			defer wg.Done()
			<-start
			ag.setConfigOptions(opts)
		}()
	}
	close(start)
	wg.Wait()

	if got := atomic.LoadInt32(&v.max); got > 1 {
		t.Fatalf("validator calls overlapped (max=%d), want serialized updates", got)
	}
}
