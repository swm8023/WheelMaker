package client

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/swm8023/wheelmaker/internal/im"
	acp "github.com/swm8023/wheelmaker/internal/protocol"
)

type permissionRouter struct {
	session *Session

	mu         sync.Mutex
	decisionCh chan struct{}
}

func newPermissionRouter(s *Session) *permissionRouter {
	r := &permissionRouter{
		session:    s,
		decisionCh: make(chan struct{}, 1),
	}
	r.decisionCh <- struct{}{}
	return r
}

func (r *permissionRouter) decide(ctx context.Context, params acp.PermissionRequestParams, mode string) (acp.PermissionResult, error) {
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

	title := strings.TrimSpace(params.ToolCall.Title)
	if title == "" {
		title = "Permission request"
	}

	req := im.DecisionRequest{
		SessionID: params.SessionID,
		Kind:      im.DecisionPermission,
		Title:     title,
		Body:      fmt.Sprintf("mode=%s toolCall=%s", renderUnknown(mode), params.ToolCall.ToolCallID),
		Meta: map[string]string{
			"tool_call_id": params.ToolCall.ToolCallID,
			"tool_title":   params.ToolCall.Title,
			"tool_kind":    params.ToolCall.Kind,
		},
		Hint: map[string]string{
			// Permission decisions may require explicit human confirmation delay.
			// Keep them valid longer to avoid stale-button failures.
			"timeoutSec": "1800",
		},
	}
	req.Options = make([]im.DecisionOption, 0, len(params.Options))
	for _, o := range params.Options {
		req.Options = append(req.Options, im.DecisionOption{
			ID:    o.OptionID,
			Label: o.Name,
			Value: o.Kind,
		})
	}

	res, err := router.RequestDecision(ctx, im.SendTarget{SessionID: r.session.ID, Source: &source}, req)
	if err != nil {
		return acp.PermissionResult{Outcome: "cancelled"}, nil
	}
	if res.Outcome == "selected" && strings.TrimSpace(res.OptionID) != "" {
		return acp.PermissionResult{Outcome: "selected", OptionID: res.OptionID}, nil
	}
	return acp.PermissionResult{Outcome: "cancelled"}, nil
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
