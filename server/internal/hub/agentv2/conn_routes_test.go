package agentv2

import (
	"testing"
	"time"

	"github.com/swm8023/wheelmaker/internal/protocol"
)

func TestRoutes_LoadPendingPromotesToActive(t *testing.T) {
	r := newRouteState()
	tok := r.beginLoad("acp-1", "inst-A", 3)
	if ok := r.commitLoad(tok); !ok {
		t.Fatal("commitLoad returned false")
	}
	got := r.lookupActive("acp-1")
	if got == nil {
		t.Fatal("active route missing")
	}
	if got.instanceKey != "inst-A" || got.epoch != 3 {
		t.Fatalf("active route = %+v", *got)
	}
}

func TestRoutes_LoadFailureRollsBack(t *testing.T) {
	r := newRouteState()
	tok := r.beginLoad("acp-1", "inst-A", 3)
	r.rollbackLoad(tok)
	if got := r.lookupActive("acp-1"); got != nil {
		t.Fatalf("unexpected active route: %+v", *got)
	}
}

func TestRoutes_EpochGuardRejectsStaleCommit(t *testing.T) {
	r := newRouteState()
	fresh := r.beginLoad("acp-1", "inst-new", 4)
	if ok := r.commitLoad(fresh); !ok {
		t.Fatal("fresh commit failed")
	}
	stale := r.beginLoad("acp-1", "inst-old", 2)
	if ok := r.commitLoad(stale); ok {
		t.Fatal("expected stale commit rejection")
	}
	got := r.lookupActive("acp-1")
	if got == nil || got.instanceKey != "inst-new" || got.epoch != 4 {
		t.Fatalf("active route changed by stale commit: %+v", got)
	}
	if got := r.lookupActiveForEpoch("acp-1", 2); got != nil {
		t.Fatalf("stale epoch lookup should fail: %+v", got)
	}
}

func TestRoutes_OrphanReplayAndTTL(t *testing.T) {
	r := newRouteState()
	r.orphanTTL = 1 * time.Second
	t0 := time.Unix(100, 0)

	r.bufferOrphan("acp-1", newUpdate("acp-1", "u1"), t0)
	r.bufferOrphan("acp-1", newUpdate("acp-1", "u2"), t0.Add(500*time.Millisecond))
	r.pruneOrphans(t0.Add(1500 * time.Millisecond))
	r.clock = func() time.Time { return t0.Add(1500 * time.Millisecond) }

	got := r.replayOrphans("acp-1")
	if len(got) != 1 {
		t.Fatalf("replay len=%d, want 1", len(got))
	}
	if got[0].Update.SessionUpdate != "u2" {
		t.Fatalf("replayed update=%q, want u2", got[0].Update.SessionUpdate)
	}
	if gotAgain := r.replayOrphans("acp-1"); len(gotAgain) != 0 {
		t.Fatalf("replay after drain len=%d, want 0", len(gotAgain))
	}
}

func newUpdate(acpSessionID, name string) protocol.SessionUpdateParams {
	return protocol.SessionUpdateParams{
		SessionID: acpSessionID,
		Update: protocol.SessionUpdate{
			SessionUpdate: name,
		},
	}
}
