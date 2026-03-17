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

func (codexPlugin) ValidateConfigOptions(opts []acp.ConfigOption) error {
	for _, opt := range opts {
		if (opt.ID == "mode" || opt.Category == "mode") && opt.CurrentValue == "" {
			return fmt.Errorf("codex: mode currentValue is empty")
		}
		if (opt.ID == "model" || opt.Category == "model") && opt.CurrentValue == "" {
			return fmt.Errorf("codex: model currentValue is empty")
		}
	}
	return nil
}
