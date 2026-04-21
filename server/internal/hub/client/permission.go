package client

import (
	"context"
	"strings"

	"github.com/swm8023/wheelmaker/internal/im"
	acp "github.com/swm8023/wheelmaker/internal/protocol"
)

func (s *Session) decidePermission(ctx context.Context, requestID int64, params acp.PermissionRequestParams, _ string) (acp.PermissionResult, error) {
	if !s.acquirePermissionDecisionSlot(ctx) {
		return acp.PermissionResult{Outcome: "cancelled"}, nil
	}
	defer s.releasePermissionDecisionSlot()

	s.mu.Lock()
	yolo := s.yolo
	s.mu.Unlock()
	if yolo {
		if optionID := chooseAutoAllowOption(params.Options); optionID != "" {
			return acp.PermissionResult{Outcome: "selected", OptionID: optionID}, nil
		}
		return acp.PermissionResult{Outcome: "cancelled"}, nil
	}

	bridge, source, ok := s.imContext()
	if !ok {
		return acp.PermissionResult{Outcome: "cancelled"}, nil
	}

	waitCh := make(chan acp.PermissionResult, 1)
	s.permissionMu.Lock()
	s.permissionPending[requestID] = waitCh
	s.permissionMu.Unlock()
	defer s.clearPermissionPending(requestID, waitCh)

	if err := bridge.PublishPermissionRequest(ctx, im.SendTarget{SessionID: s.ID, Source: &source}, requestID, params); err != nil {
		hubLogger(s.projectName).Error("permission publish failed session=%s request=%d err=%v", s.ID, requestID, err)
		return acp.PermissionResult{Outcome: "cancelled"}, nil
	}
	s.recordSessionViewEvent(SessionViewEvent{
		Method:    SessionViewMethodPermission,
		Text:      strings.TrimSpace(params.ToolCall.Title),
		Options:   cloneSessionPermissionOptions(params.Options),
		Status:    "needs_action",
		RequestID: requestID,
	})

	select {
	case <-ctx.Done():
		hubLogger(s.projectName).Warn("permission request timeout/cancelled session=%s request=%d", s.ID, requestID)
		s.recordSessionViewEvent(SessionViewEvent{Method: SessionViewMethodPermissionResolved, Status: "cancelled", RequestID: requestID})
		return acp.PermissionResult{Outcome: "cancelled"}, nil
	case result := <-waitCh:
		s.recordSessionViewEvent(SessionViewEvent{Method: SessionViewMethodPermissionResolved, Status: firstNonEmpty(strings.TrimSpace(result.Outcome), "done"), RequestID: requestID})
		if result.Outcome == "selected" && strings.TrimSpace(result.OptionID) != "" {
			return result, nil
		}
		return acp.PermissionResult{Outcome: "cancelled"}, nil
	}
}

func (s *Session) acquirePermissionDecisionSlot(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return false
	case <-s.permissionDecisionCh:
		return true
	}
}

func (s *Session) releasePermissionDecisionSlot() {
	select {
	case s.permissionDecisionCh <- struct{}{}:
	default:
	}
}

func (s *Session) resolvePermission(requestID int64, result acp.PermissionResponse) bool {
	s.permissionMu.Lock()
	ch, ok := s.permissionPending[requestID]
	if ok {
		delete(s.permissionPending, requestID)
	}
	s.permissionMu.Unlock()
	if !ok {
		return false
	}
	select {
	case ch <- result.Outcome:
	default:
	}
	return true
}

func (s *Session) clearPermissionPending(requestID int64, ch chan acp.PermissionResult) {
	s.permissionMu.Lock()
	defer s.permissionMu.Unlock()
	current, ok := s.permissionPending[requestID]
	if ok && current == ch {
		delete(s.permissionPending, requestID)
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
