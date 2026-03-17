package acp

import (
	"context"
	"encoding/json"
)

// AgentPlugin is the per-agent customization hook for acp.Agent.
// Embed DefaultPlugin and override only the methods needed.
//
// Concurrency note: ValidateConfigOptions is called while configOptsMu is held.
// Implementations must NOT acquire acp.Agent's internal locks (mu, configOptsMu).
type AgentPlugin interface {
	// ValidateConfigOptions validates a config_option_update list.
	// Only validate fields that are present — partial updates are allowed.
	// Return non-nil to reject; the update is dropped (or connection fails on session/new).
	ValidateConfigOptions(opts []ConfigOption) error

	// HandlePermission responds to session/request_permission callbacks.
	// Signature matches the former PermissionHandler.RequestPermission.
	HandlePermission(ctx context.Context, params PermissionRequestParams) (PermissionResult, error)

	// NormalizeParams is called before acp processes each incoming session/update
	// notification. Translate legacy protocol fields to modern format here.
	// Return params unchanged for pass-through (default behaviour).
	NormalizeParams(method string, params json.RawMessage) json.RawMessage
}

// DefaultPlugin is the zero-value AgentPlugin. All methods are no-ops or auto-allow.
// Embed this in agent-specific plugins; add new extension points here without
// requiring all implementations to update.
type DefaultPlugin struct{}

// ValidateConfigOptions accepts all options (no validation).
func (DefaultPlugin) ValidateConfigOptions(_ []ConfigOption) error { return nil }

// HandlePermission auto-selects allow_once (matching former AutoAllowHandler behaviour).
func (DefaultPlugin) HandlePermission(_ context.Context, params PermissionRequestParams) (PermissionResult, error) {
	optionID := ""
	for _, opt := range params.Options {
		if opt.Kind == "allow_once" {
			optionID = opt.OptionID
			break
		}
	}
	if optionID == "" && len(params.Options) > 0 {
		optionID = params.Options[0].OptionID
	}
	return PermissionResult{Outcome: "selected", OptionID: optionID}, nil
}

// NormalizeParams passes notifications through unchanged.
func (DefaultPlugin) NormalizeParams(_ string, params json.RawMessage) json.RawMessage { return params }

// pluginOrDefault returns p if non-nil, otherwise DefaultPlugin{}.
func pluginOrDefault(p AgentPlugin) AgentPlugin {
	if p == nil {
		return DefaultPlugin{}
	}
	return p
}

// Compile-time interface check.
var _ AgentPlugin = DefaultPlugin{}
