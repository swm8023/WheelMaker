package client

import (
	"context"
	"strings"

	acp "github.com/swm8023/wheelmaker/internal/protocol"
)

func (s *Session) decidePermission(ctx context.Context, requestID int64, params acp.PermissionRequestParams, _ string) (acp.PermissionResult, error) {
	_ = ctx
	_ = requestID
	if optionID := chooseAutoAllowOption(params.Options); optionID != "" {
		return acp.PermissionResult{Outcome: "selected", OptionID: optionID}, nil
	}
	return acp.PermissionResult{Outcome: "cancelled"}, nil
}

func chooseAutoAllowOption(options []acp.PermissionOption) string {
	// Prefer persistent allow semantics, then one-shot allow semantics.
	fallback := ""
	for _, o := range options {
		kind := strings.ToLower(strings.TrimSpace(o.Kind))
		id := strings.TrimSpace(o.OptionID)
		name := strings.ToLower(strings.TrimSpace(o.Name))
		if id == "" {
			continue
		}
		if kind == "allow_always" || kind == "always" {
			return id
		}
		if fallback == "" && (kind == "allow_once" || kind == "allow" || kind == "once" || strings.HasPrefix(kind, "allow") || strings.EqualFold(id, "allow") || strings.Contains(name, "allow")) {
			fallback = id
		}
	}
	return fallback
}
