package hub

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/swm8023/wheelmaker/internal/hub/client"
	rp "github.com/swm8023/wheelmaker/internal/protocol"
)

type TokenCommand struct{}

type tokenCommandPayload struct {
	Action string `json:"action"`
	HubID  string `json:"hubId"`
}

type tokenCommandError struct {
	Code    string
	Message string
}

func (e *tokenCommandError) Error() string {
	if e == nil {
		return ""
	}
	if e.Code == "" {
		return e.Message
	}
	return e.Code + ": " + e.Message
}

var scanHubTokenStats = client.ScanTokenStats

func NewTokenCommand() *TokenCommand {
	return &TokenCommand{}
}

func (c *TokenCommand) Handle(ctx context.Context, raw json.RawMessage) (any, *tokenCommandError) {
	var payload tokenCommandPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, &tokenCommandError{Code: rp.CodeInvalidArgument, Message: "invalid cmd.token payload"}
	}
	payload.Action = strings.TrimSpace(payload.Action)
	payload.HubID = strings.TrimSpace(payload.HubID)
	if payload.HubID == "" {
		return nil, &tokenCommandError{Code: rp.CodeInvalidArgument, Message: "hubId is required"}
	}

	switch payload.Action {
	case "scan":
		result, err := scanHubTokenStats(ctx)
		if err != nil {
			return map[string]any{
				"ok":        false,
				"hubId":     payload.HubID,
				"updatedAt": time.Now().UTC().Format(time.RFC3339),
				"providers": []map[string]any{},
				"error":     err.Error(),
			}, nil
		}
		body := tokenCommandResponseMap(result)
		body["hubId"] = payload.HubID
		return body, nil
	default:
		return nil, &tokenCommandError{Code: rp.CodeInvalidArgument, Message: "unsupported cmd.token action"}
	}
}

func tokenCommandResponseMap(result any) map[string]any {
	body, ok := result.(map[string]any)
	if ok {
		if _, exists := body["providers"]; !exists {
			body["providers"] = []map[string]any{}
		}
		return body
	}
	raw, err := json.Marshal(result)
	if err != nil {
		return map[string]any{"ok": false, "providers": []map[string]any{}, "error": err.Error()}
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return map[string]any{"ok": false, "providers": []map[string]any{}, "error": err.Error()}
	}
	if _, exists := body["providers"]; !exists {
		body["providers"] = []map[string]any{}
	}
	return body
}
