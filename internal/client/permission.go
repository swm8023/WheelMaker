package client

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/swm8023/wheelmaker/internal/acp"
	"github.com/swm8023/wheelmaker/internal/backend"
)

type pendingPermission struct {
	mode   string
	params acp.PermissionRequestParams
	respCh chan acp.PermissionResult
}

type permissionRouter struct {
	client *Client

	mu         sync.Mutex
	byChatID   map[string]*pendingPermission
	lastChatID string
}

func newPermissionRouter(c *Client) *permissionRouter {
	return &permissionRouter{
		client:   c,
		byChatID: make(map[string]*pendingPermission),
	}
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

func (r *permissionRouter) resolveIncomingReply(chatID, text string) bool {
	r.mu.Lock()
	pp := r.byChatID[chatID]
	r.mu.Unlock()
	if pp == nil {
		return false
	}

	result, ok := parsePermissionReply(strings.TrimSpace(text), pp.params.Options)
	if !ok {
		r.client.reply(chatID, "Permission reply not recognized. Reply with: allow | reject | cancel | <optionId> | <index>")
		return true
	}
	select {
	case pp.respCh <- result:
		r.client.reply(chatID, fmt.Sprintf("Permission choice received: %s", permissionResultLabel(result)))
	default:
	}
	return true
}

func (r *permissionRouter) decide(ctx context.Context, params acp.PermissionRequestParams, mode string, fallback backend.Backend) (acp.PermissionResult, error) {
	normalized := strings.ToLower(strings.TrimSpace(mode))
	switch normalized {
	case "ask", "manual", "user":
		return r.waitForUser(ctx, params, mode)
	default:
		return fallback.HandlePermission(ctx, params, mode)
	}
}

func (r *permissionRouter) waitForUser(ctx context.Context, params acp.PermissionRequestParams, mode string) (acp.PermissionResult, error) {
	r.mu.Lock()
	chatID := r.lastChatID
	if strings.TrimSpace(chatID) == "" {
		chatID = r.client.projectName
	}
	pp := &pendingPermission{
		mode:   mode,
		params: params,
		respCh: make(chan acp.PermissionResult, 1),
	}
	r.byChatID[chatID] = pp
	r.mu.Unlock()

	defer func() {
		r.mu.Lock()
		if cur := r.byChatID[chatID]; cur == pp {
			delete(r.byChatID, chatID)
		}
		r.mu.Unlock()
	}()

	r.client.reply(chatID, formatPermissionPrompt(mode, params))

	select {
	case result := <-pp.respCh:
		return result, nil
	case <-ctx.Done():
		return acp.PermissionResult{Outcome: "cancelled"}, nil
	case <-time.After(2 * time.Minute):
		r.client.reply(chatID, "Permission request timed out.")
		return acp.PermissionResult{Outcome: "cancelled"}, nil
	}
}

func formatPermissionPrompt(mode string, params acp.PermissionRequestParams) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Permission request (mode=%s, toolCall=%s)\n", renderUnknown(mode), params.ToolCall.ToolCallID))
	b.WriteString("Reply with: allow | reject | cancel | <optionId> | <index>\n")
	for i, opt := range params.Options {
		b.WriteString(fmt.Sprintf("%d. %s (%s) id=%s\n", i+1, renderUnknown(opt.Name), renderUnknown(opt.Kind), opt.OptionID))
	}
	return strings.TrimSpace(b.String())
}

func parsePermissionReply(input string, options []acp.PermissionOption) (acp.PermissionResult, bool) {
	v := strings.ToLower(strings.TrimSpace(input))
	if v == "" {
		return acp.PermissionResult{}, false
	}
	if v == "cancel" {
		return acp.PermissionResult{Outcome: "cancelled"}, true
	}
	if v == "allow" || v == "approve" || v == "yes" || v == "y" {
		if id := findPermissionOption(options, func(o acp.PermissionOption) bool { return strings.HasPrefix(o.Kind, "allow") }); id != "" {
			return acp.PermissionResult{Outcome: "selected", OptionID: id}, true
		}
		return acp.PermissionResult{Outcome: "cancelled"}, true
	}
	if v == "reject" || v == "deny" || v == "no" || v == "n" {
		if id := findPermissionOption(options, func(o acp.PermissionOption) bool {
			return strings.HasPrefix(o.Kind, "reject") || strings.Contains(o.Kind, "deny")
		}); id != "" {
			return acp.PermissionResult{Outcome: "selected", OptionID: id}, true
		}
		return acp.PermissionResult{Outcome: "cancelled"}, true
	}

	if i, err := strconv.Atoi(v); err == nil && i >= 1 && i <= len(options) {
		return acp.PermissionResult{Outcome: "selected", OptionID: options[i-1].OptionID}, true
	}
	if strings.HasPrefix(v, "#") {
		if i, err := strconv.Atoi(strings.TrimPrefix(v, "#")); err == nil && i >= 1 && i <= len(options) {
			return acp.PermissionResult{Outcome: "selected", OptionID: options[i-1].OptionID}, true
		}
	}

	for _, o := range options {
		if strings.EqualFold(v, o.OptionID) || strings.EqualFold(v, o.Name) || strings.EqualFold(v, o.Kind) {
			return acp.PermissionResult{Outcome: "selected", OptionID: o.OptionID}, true
		}
	}
	return acp.PermissionResult{}, false
}

func findPermissionOption(options []acp.PermissionOption, match func(acp.PermissionOption) bool) string {
	for _, o := range options {
		if match(o) {
			return o.OptionID
		}
	}
	return ""
}

func permissionResultLabel(r acp.PermissionResult) string {
	if r.Outcome != "selected" {
		return r.Outcome
	}
	if strings.TrimSpace(r.OptionID) == "" {
		return "selected"
	}
	return "selected:" + r.OptionID
}

type interactiveBackend struct {
	base   backend.Backend
	router *permissionRouter
}

func (b *interactiveBackend) Name() string { return b.base.Name() }

func (b *interactiveBackend) Connect(ctx context.Context) (*acp.Conn, error) {
	return b.base.Connect(ctx)
}

func (b *interactiveBackend) Close() error { return b.base.Close() }

func (b *interactiveBackend) HandlePermission(ctx context.Context, params acp.PermissionRequestParams, mode string) (acp.PermissionResult, error) {
	return b.router.decide(ctx, params, mode, b.base)
}

func (b *interactiveBackend) NormalizeParams(method string, params json.RawMessage) json.RawMessage {
	return b.base.NormalizeParams(method, params)
}

var _ backend.Backend = (*interactiveBackend)(nil)
