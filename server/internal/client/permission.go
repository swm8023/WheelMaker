package client

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/swm8023/wheelmaker/internal/acp"
	"github.com/swm8023/wheelmaker/internal/im"
)

type permissionRouter struct {
	client *Client

	mu         sync.Mutex
	lastChatID string
}

func newPermissionRouter(c *Client) *permissionRouter {
	return &permissionRouter{client: c}
}

func (r *permissionRouter) setLastChatID(chatID string) {
	r.mu.Lock()
	r.lastChatID = chatID
	r.mu.Unlock()
}

func (r *permissionRouter) clearLastChatID(chatID string) {
	r.mu.Lock()
	if r.lastChatID == chatID {
		r.lastChatID = ""
	}
	r.mu.Unlock()
}

func (r *permissionRouter) decide(ctx context.Context, params acp.PermissionRequestParams, mode string) (acp.PermissionResult, error) {
	r.client.mu.Lock()
	bridge := r.client.imBridge
	r.client.mu.Unlock()
	if bridge == nil || !bridge.CanHandleDecision() {
		return acp.PermissionResult{Outcome: "cancelled"}, nil
	}

	r.mu.Lock()
	chatID := r.lastChatID
	r.mu.Unlock()
	if strings.TrimSpace(chatID) == "" {
		chatID = r.client.projectName
	}

	title := strings.TrimSpace(params.ToolCall.Title)
	if title == "" {
		title = "Permission request"
	}

	req := im.DecisionRequest{
		Kind:   im.DecisionPermission,
		ChatID: chatID,
		Title:  title,
		Body:   fmt.Sprintf("mode=%s toolCall=%s", renderUnknown(mode), params.ToolCall.ToolCallID),
		Meta: map[string]string{
			"tool_call_id": params.ToolCall.ToolCallID,
			"tool_title":   params.ToolCall.Title,
			"tool_kind":    params.ToolCall.Kind,
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

	res, err := bridge.RequestDecision(ctx, req)
	if err != nil {
		return acp.PermissionResult{Outcome: "cancelled"}, nil
	}
	if res.Outcome == "selected" && strings.TrimSpace(res.OptionID) != "" {
		return acp.PermissionResult{Outcome: "selected", OptionID: res.OptionID}, nil
	}
	return acp.PermissionResult{Outcome: "cancelled"}, nil
}
