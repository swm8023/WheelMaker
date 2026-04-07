package client

import (
	"context"
	"strings"
	"sync"

	"github.com/swm8023/wheelmaker/internal/im"
	acp "github.com/swm8023/wheelmaker/internal/protocol"
)

type permissionRouter struct {
	session *Session

	mu         sync.Mutex
	decisionCh chan struct{}
	pending    map[int64]chan acp.PermissionResult
}

func newPermissionRouter(s *Session) *permissionRouter {
	r := &permissionRouter{
		session:    s,
		decisionCh: make(chan struct{}, 1),
		pending:    map[int64]chan acp.PermissionResult{},
	}
	r.decisionCh <- struct{}{}
	return r
}

func (r *permissionRouter) decide(ctx context.Context, requestID int64, params acp.PermissionRequestParams, _ string) (acp.PermissionResult, error) {
	if !r.acquireDecisionSlot(ctx) {
		return acp.PermissionResult{Outcome: "cancelled"}, nil
	}
	defer r.releaseDecisionSlot()

	r.session.mu.Lock()
	yolo := r.session.yolo
	r.session.mu.Unlock()
	if yolo {
		if optionID := chooseAutoAllowOption(params.Options); optionID != "" {
			return acp.PermissionResult{Outcome: "selected", OptionID: optionID}, nil
		}
		return acp.PermissionResult{Outcome: "cancelled"}, nil
	}

	router, source, ok := r.session.imContext()
	if !ok {
		return acp.PermissionResult{Outcome: "cancelled"}, nil
	}

	waitCh := make(chan acp.PermissionResult, 1)
	r.mu.Lock()
	r.pending[requestID] = waitCh
	r.mu.Unlock()
	defer r.clearPending(requestID, waitCh)

	if err := router.PublishPermissionRequest(ctx, im.SendTarget{SessionID: r.session.ID, Source: &source}, requestID, params); err != nil {
		return acp.PermissionResult{Outcome: "cancelled"}, nil
	}

	select {
	case <-ctx.Done():
		return acp.PermissionResult{Outcome: "cancelled"}, nil
	case result := <-waitCh:
		if result.Outcome == "selected" && strings.TrimSpace(result.OptionID) != "" {
			return result, nil
		}
		return acp.PermissionResult{Outcome: "cancelled"}, nil
	}
}

func (r *permissionRouter) acquireDecisionSlot(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return false
	case <-r.decisionCh:
		return true
	}
}

func (r *permissionRouter) releaseDecisionSlot() {
	select {
	case r.decisionCh <- struct{}{}:
	default:
	}
}

func (r *permissionRouter) resolve(requestID int64, result acp.PermissionResponse) bool {
	r.mu.Lock()
	ch, ok := r.pending[requestID]
	if ok {
		delete(r.pending, requestID)
	}
	r.mu.Unlock()
	if !ok {
		return false
	}
	select {
	case ch <- result.Outcome:
	default:
	}
	return true
}

func (r *permissionRouter) clearPending(requestID int64, ch chan acp.PermissionResult) {
	r.mu.Lock()
	defer r.mu.Unlock()
	current, ok := r.pending[requestID]
	if ok && current == ch {
		delete(r.pending, requestID)
	}
}

func chooseAutoAllowOption(options []acp.PermissionOption) string {
	// Prefer allow_always so yolo mode permanently grants the permission.
	// Fall back to any allow_* option if allow_always is not offered.
	fallback := ""
	for _, o := range options {
		kind := strings.ToLower(strings.TrimSpace(o.Kind))
		id := strings.TrimSpace(o.OptionID)
		if id == "" {
			continue
		}
		if kind == "allow_always" {
			return id
		}
		if fallback == "" && strings.HasPrefix(kind, "allow") {
			fallback = id
		}
	}
	return fallback
}
