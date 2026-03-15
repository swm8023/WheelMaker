package agent

import (
	"context"

	"github.com/swm8023/wheelmaker/internal/agent/acp"
)

// PermissionHandler decides how to respond to the agent's permission requests.
// MVP: AutoAllowHandler always selects allow_once.
// Phase 2: IMPermissionHandler routes to IM for user confirmation.
type PermissionHandler interface {
	RequestPermission(ctx context.Context, params acp.PermissionRequestParams) (acp.PermissionResult, error)
}

// AutoAllowHandler is a stateless PermissionHandler that auto-selects allow_once.
type AutoAllowHandler struct{}

// RequestPermission selects the allow_once option, or the first available option.
func (h *AutoAllowHandler) RequestPermission(_ context.Context, params acp.PermissionRequestParams) (acp.PermissionResult, error) {
	optionID := ""
	for _, opt := range params.Options {
		// B1 fix: field is OptionID (json:"optionId"), was ID (json:"id").
		if opt.Kind == "allow_once" {
			optionID = opt.OptionID
			break
		}
	}
	// Fall back to first option if allow_once is not present.
	if optionID == "" && len(params.Options) > 0 {
		optionID = params.Options[0].OptionID
	}
	return acp.PermissionResult{
		Outcome:  "selected",
		OptionID: optionID,
	}, nil
}
