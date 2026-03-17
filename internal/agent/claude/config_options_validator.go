package claude

import (
	"fmt"

	acp "github.com/swm8023/wheelmaker/internal/acp"
)

// claudePlugin is the AgentPlugin for Claude. It embeds DefaultPlugin and
// overrides ValidateConfigOptions to enforce Claude-specific requirements.
type claudePlugin struct {
	acp.DefaultPlugin
}

func (claudePlugin) ValidateConfigOptions(options []acp.ConfigOption) error {
	hasMode := false
	hasModel := false
	for _, opt := range options {
		if opt.ID == "mode" || opt.Category == "mode" {
			hasMode = true
			if opt.CurrentValue == "" {
				return fmt.Errorf("claude mode currentValue is empty")
			}
		}
		if opt.ID == "model" || opt.Category == "model" {
			hasModel = true
			if opt.CurrentValue == "" {
				return fmt.Errorf("claude model currentValue is empty")
			}
		}
	}
	if !hasMode {
		return fmt.Errorf("claude missing mode config option")
	}
	if !hasModel {
		return fmt.Errorf("claude missing model config option")
	}
	return nil
}
