package acp

import (
	"encoding/json"

	p "github.com/swm8023/wheelmaker/internal/protocol"
)

// NormalizeNotificationParams applies protocol compatibility transforms for
// inbound agent->client notifications. Unknown payloads are passed through.
func NormalizeNotificationParams(method string, params json.RawMessage) json.RawMessage {
	if method != MethodSessionUpdate || len(params) == 0 {
		return params
	}

	var envelope map[string]any
	if err := json.Unmarshal(params, &envelope); err != nil {
		return params
	}
	update, ok := envelope["update"].(map[string]any)
	if !ok {
		return params
	}
	sessionUpdate, _ := update["sessionUpdate"].(string)
	if sessionUpdate != p.SessionUpdateCurrentModeUpdate {
		return params
	}
	modeID, _ := update["modeId"].(string)
	if modeID == "" {
		return params
	}

	update["sessionUpdate"] = p.SessionUpdateConfigOptionUpdate
	if _, exists := update["configOptions"]; !exists {
		update["configOptions"] = []any{
			map[string]any{
				"id":           p.ConfigOptionIDMode,
				"category":     p.ConfigOptionCategoryMode,
				"currentValue": modeID,
			},
		}
	}
	delete(update, "modeId")

	b, err := json.Marshal(envelope)
	if err != nil {
		return params
	}
	return b
}
