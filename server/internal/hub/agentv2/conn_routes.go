package agentv2

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/swm8023/wheelmaker/internal/protocol"
)

const defaultOrphanTTL = 5 * time.Second

type activeBinding struct {
	instanceKey string
	epoch       uint64
}

type pendingBinding struct {
	token              string
	targetACPSessionID string
	instanceKey        string
	epoch              uint64
}

type orphanUpdate struct {
	update     protocol.SessionUpdateParams
	receivedAt time.Time
}

type routeState struct {
	mu       sync.RWMutex
	tokenSeq atomic.Uint64

	active  map[string]activeBinding
	pending map[string]pendingBinding
	orphan  map[string][]orphanUpdate

	orphanTTL time.Duration
	clock     func() time.Time
}

func newRouteState() *routeState {
	return &routeState{
		active:    make(map[string]activeBinding),
		pending:   make(map[string]pendingBinding),
		orphan:    make(map[string][]orphanUpdate),
		orphanTTL: defaultOrphanTTL,
		clock:     time.Now,
	}
}

func (r *routeState) beginLoad(targetACPSessionID, instanceKey string, epoch uint64) string {
	token := fmt.Sprintf("load-%d", r.tokenSeq.Add(1))
	r.mu.Lock()
	r.pending[token] = pendingBinding{
		token:              token,
		targetACPSessionID: targetACPSessionID,
		instanceKey:        instanceKey,
		epoch:              epoch,
	}
	r.mu.Unlock()
	return token
}

func (r *routeState) commitLoad(token string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	pending, ok := r.pending[token]
	if !ok {
		return false
	}
	delete(r.pending, token)

	if current, exists := r.active[pending.targetACPSessionID]; exists && current.epoch > pending.epoch {
		return false
	}

	r.active[pending.targetACPSessionID] = activeBinding{
		instanceKey: pending.instanceKey,
		epoch:       pending.epoch,
	}
	return true
}

func (r *routeState) rollbackLoad(token string) {
	r.mu.Lock()
	delete(r.pending, token)
	r.mu.Unlock()
}

func (r *routeState) lookupActive(acpSessionID string) *activeBinding {
	r.mu.RLock()
	binding, ok := r.active[acpSessionID]
	r.mu.RUnlock()
	if !ok {
		return nil
	}
	copy := binding
	return &copy
}

func (r *routeState) lookupActiveForEpoch(acpSessionID string, epoch uint64) *activeBinding {
	r.mu.RLock()
	binding, ok := r.active[acpSessionID]
	r.mu.RUnlock()
	if !ok || binding.epoch != epoch {
		return nil
	}
	copy := binding
	return &copy
}

func (r *routeState) bufferOrphan(acpSessionID string, update protocol.SessionUpdateParams, now time.Time) {
	r.mu.Lock()
	r.orphan[acpSessionID] = append(r.orphan[acpSessionID], orphanUpdate{update: update, receivedAt: now})
	r.pruneOrphansLocked(now)
	r.mu.Unlock()
}

func (r *routeState) replayOrphans(acpSessionID string) []protocol.SessionUpdateParams {
	now := r.now()
	if r.clock == nil {
		now = time.Now()
	}

	r.mu.Lock()
	r.pruneOrphansLocked(now)
	buffered := r.orphan[acpSessionID]
	delete(r.orphan, acpSessionID)
	r.mu.Unlock()

	out := make([]protocol.SessionUpdateParams, 0, len(buffered))
	for _, item := range buffered {
		out = append(out, item.update)
	}
	return out
}

func (r *routeState) pruneOrphans(now time.Time) {
	r.mu.Lock()
	r.pruneOrphansLocked(now)
	r.mu.Unlock()
}

func (r *routeState) pruneOrphansLocked(now time.Time) {
	if r.orphanTTL <= 0 {
		for key := range r.orphan {
			delete(r.orphan, key)
		}
		return
	}

	for acpSessionID, updates := range r.orphan {
		kept := updates[:0]
		for _, item := range updates {
			if now.Sub(item.receivedAt) <= r.orphanTTL {
				kept = append(kept, item)
			}
		}
		if len(kept) == 0 {
			delete(r.orphan, acpSessionID)
			continue
		}
		r.orphan[acpSessionID] = kept
	}
}

func (r *routeState) now() time.Time {
	if r.clock == nil {
		return time.Now()
	}
	return r.clock()
}
