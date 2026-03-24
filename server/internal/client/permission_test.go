package client

import (
	"context"
	"testing"
	"time"

	"github.com/swm8023/wheelmaker/internal/acp"
)

func TestPermissionRouterDecisionSlotSerializes(t *testing.T) {
	r := newPermissionRouter(&Client{})

	if !r.acquireDecisionSlot(context.Background()) {
		t.Fatal("first acquire failed")
	}

	acquiredSecond := make(chan struct{})
	releaseSecond := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		if !r.acquireDecisionSlot(context.Background()) {
			return
		}
		close(acquiredSecond)
		<-releaseSecond
		r.releaseDecisionSlot()
	}()

	select {
	case <-acquiredSecond:
		t.Fatal("second acquire should block until first release")
	case <-time.After(120 * time.Millisecond):
	}

	r.releaseDecisionSlot()

	select {
	case <-acquiredSecond:
	case <-time.After(2 * time.Second):
		t.Fatal("second acquire did not proceed after first release")
	}

	close(releaseSecond)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("second goroutine did not finish")
	}
}

func TestPermissionRouterDecisionSlotRespectsContextCancel(t *testing.T) {
	r := newPermissionRouter(&Client{})

	if !r.acquireDecisionSlot(context.Background()) {
		t.Fatal("first acquire failed")
	}
	defer r.releaseDecisionSlot()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	if r.acquireDecisionSlot(ctx) {
		t.Fatal("acquire should fail when context is cancelled")
	}
	if time.Since(start) < 80*time.Millisecond {
		t.Fatal("acquire returned too early; expected to wait for context cancellation")
	}
}

func TestChooseAutoAllowOption(t *testing.T) {
	opts := []acp.PermissionOption{
		{OptionID: "reject", Kind: "reject_once"},
		{OptionID: "allow", Kind: "allow_once"},
	}
	if got := chooseAutoAllowOption(opts); got != "allow" {
		t.Fatalf("chooseAutoAllowOption()=%q, want allow", got)
	}
}

func TestChooseAutoAllowOptionFallbackFirst(t *testing.T) {
	opts := []acp.PermissionOption{
		{OptionID: "abort", Kind: "reject_once"},
		{OptionID: "deny", Kind: "reject_always"},
	}
	if got := chooseAutoAllowOption(opts); got != "" {
		t.Fatalf("chooseAutoAllowOption()=%q, want empty", got)
	}
}
