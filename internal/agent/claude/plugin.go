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

func (claudePlugin) ValidateConfigOptions(opts []acp.ConfigOption) error {
	for _, opt := range opts {
		if (opt.ID == "mode" || opt.Category == "mode") && opt.CurrentValue == "" {
			return fmt.Errorf("claude: mode currentValue is empty")
		}
		if (opt.ID == "model" || opt.Category == "model") && opt.CurrentValue == "" {
			return fmt.Errorf("claude: model currentValue is empty")
		}
	}
	return nil
}
