package codex

import (
	"fmt"

	acp "github.com/swm8023/wheelmaker/internal/acp"
)

// codexPlugin is the AgentPlugin for Codex. It embeds DefaultPlugin and
// overrides ValidateConfigOptions to enforce Codex-specific requirements.
type codexPlugin struct {
	acp.DefaultPlugin
}

func (codexPlugin) ValidateConfigOptions(options []acp.ConfigOption) error {
	hasMode := false
	hasModel := false
	for _, opt := range options {
		if opt.ID == "mode" || opt.Category == "mode" {
			hasMode = true
			if opt.CurrentValue == "" {
				return fmt.Errorf("codex mode currentValue is empty")
			}
		}
		if opt.ID == "model" || opt.Category == "model" {
			hasModel = true
			if opt.CurrentValue == "" {
				return fmt.Errorf("codex model currentValue is empty")
			}
		}
	}
	if !hasMode {
		return fmt.Errorf("codex missing mode config option")
	}
	if !hasModel {
		return fmt.Errorf("codex missing model config option")
	}
	return nil
}
