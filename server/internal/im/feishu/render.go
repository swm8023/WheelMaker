package feishu

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/swm8023/wheelmaker/internal/im"
	acp "github.com/swm8023/wheelmaker/internal/protocol"
)

func extractTextContent(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var block acp.ContentBlock
	if err := json.Unmarshal(raw, &block); err != nil {
		return ""
	}
	if block.Type != "" && block.Type != acp.ContentBlockTypeText {
		return ""
	}
	return strings.TrimSpace(block.Text)
}

func parseToolCallUpdate(update acp.SessionUpdate) (ToolCallUpdate, bool) {
	raw, err := json.Marshal(update)
	if err != nil {
		return ToolCallUpdate{}, false
	}
	var upd ToolCallUpdate
	if err := json.Unmarshal(raw, &upd); err != nil {
		return ToolCallUpdate{}, false
	}
	upd.ToolCallID = strings.TrimSpace(upd.ToolCallID)
	if upd.ToolCallID == "" {
		return ToolCallUpdate{}, false
	}
	return upd, true
}

func renderUpdateSummary(prefix string, update acp.SessionUpdate) string {
	switch update.SessionUpdate {
	case acp.SessionUpdatePlan:
		if len(update.Entries) > 0 {
			parts := make([]string, 0, len(update.Entries))
			for _, entry := range update.Entries {
				content := strings.TrimSpace(entry.Content)
				if content == "" {
					continue
				}
				parts = append(parts, previewLine(content, 40))
				if len(parts) >= 3 {
					break
				}
			}
			if len(parts) > 0 {
				return prefix + ": " + strings.Join(parts, " | ")
			}
		}
	case acp.SessionUpdateConfigOptionUpdate:
		snap := acp.SessionConfigSnapshotFromOptions(update.ConfigOptions)
		parts := make([]string, 0, 2)
		if strings.TrimSpace(snap.Mode) != "" {
			parts = append(parts, "mode="+strings.TrimSpace(snap.Mode))
		}
		if strings.TrimSpace(snap.Model) != "" {
			parts = append(parts, "model="+strings.TrimSpace(snap.Model))
		}
		if len(parts) > 0 {
			return prefix + ": " + strings.Join(parts, ", ")
		}
	case acp.SessionUpdateAvailableCommandsUpdate:
		names := make([]string, 0, len(update.AvailableCommands))
		for _, cmd := range update.AvailableCommands {
			name := strings.TrimSpace(cmd.Name)
			if name == "" {
				continue
			}
			names = append(names, name)
			if len(names) >= 5 {
				break
			}
		}
		if len(names) > 0 {
			return prefix + ": " + strings.Join(names, " ")
		}
	case acp.SessionUpdateSessionInfoUpdate:
		if title := strings.TrimSpace(update.Title); title != "" {
			return prefix + ": " + title
		}
	case acp.SessionUpdateCurrentModeUpdate:
		if mode := strings.TrimSpace(update.ModeID); mode != "" {
			return prefix + ": mode=" + mode
		}
	}

	raw, err := json.Marshal(update)
	if err != nil {
		return prefix
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return prefix
	}
	if title, _ := payload["title"].(string); strings.TrimSpace(title) != "" {
		return fmt.Sprintf("%s: %s", prefix, strings.TrimSpace(title))
	}
	return prefix
}

func renderPromptResultText(result acp.SessionPromptResult) string {
	stopReason := strings.TrimSpace(result.StopReason)
	switch stopReason {
	case "", acp.StopReasonEndTurn:
		return ""
	case acp.StopReasonCancelled:
		return "Prompt cancelled."
	case acp.StopReasonMaxTokens:
		return "Prompt stopped: max tokens reached."
	case acp.StopReasonMaxTurnRequests:
		return "Prompt stopped: max turn requests reached."
	case acp.StopReasonRefusal:
		return "Prompt stopped: request refused."
	default:
		return "Prompt stopped: " + stopReason
	}
}

func renderSystemText(payload im.SystemPayload) string {
	title := strings.TrimSpace(payload.Title)
	body := strings.TrimSpace(payload.Body)
	switch {
	case title != "" && body != "":
		return title + "\n" + body
	case body != "":
		return body
	default:
		return title
	}
}

func buildPermissionCard(chatID string, requestID int64, params acp.PermissionRequestParams) OptionsCard {
	meta := map[string]string{
		"kind":       "permission",
		"chat_id":    chatID,
		"request_id": strconv.FormatInt(requestID, 10),
	}
	if toolCallID := strings.TrimSpace(params.ToolCall.ToolCallID); toolCallID != "" {
		meta["tool_call_id"] = toolCallID
	}
	if title := strings.TrimSpace(params.ToolCall.Title); title != "" {
		meta["tool_title"] = title
	}
	if kind := strings.TrimSpace(params.ToolCall.Kind); kind != "" {
		meta["tool_kind"] = kind
	}
	return OptionsCard{
		Title:   renderPermissionTitle(params),
		Body:    renderPermissionBody(params),
		Options: append([]acp.PermissionOption(nil), params.Options...),
		Meta:    meta,
	}
}

func renderPermissionTitle(params acp.PermissionRequestParams) string {
	title := strings.TrimSpace(params.ToolCall.Title)
	if title == "" {
		return "Permission required"
	}
	return "Permission required: " + title
}

func renderPermissionBody(params acp.PermissionRequestParams) string {
	lines := []string{}
	if kind := strings.TrimSpace(params.ToolCall.Kind); kind != "" {
		lines = append(lines, "Tool kind: "+kind)
	}
	if id := strings.TrimSpace(params.ToolCall.ToolCallID); id != "" {
		lines = append(lines, "Tool call: "+id)
	}
	if len(lines) == 0 {
		return "Choose how to respond to this tool request."
	}
	return strings.Join(lines, "\n")
}

func parsePermissionReply(input string, opts []acp.PermissionOption) acp.PermissionResponse {
	input = strings.TrimSpace(input)
	if input == "" {
		return acp.PermissionResponse{Outcome: acp.PermissionResult{Outcome: "invalid"}}
	}
	if strings.EqualFold(input, "cancel") {
		return acp.PermissionResponse{Outcome: acp.PermissionResult{Outcome: "cancelled"}}
	}
	if idx, err := strconv.Atoi(input); err == nil && idx >= 1 && idx <= len(opts) {
		return acp.PermissionResponse{Outcome: acp.PermissionResult{
			Outcome:  "selected",
			OptionID: strings.TrimSpace(opts[idx-1].OptionID),
		}}
	}
	for _, opt := range opts {
		if strings.EqualFold(input, opt.OptionID) || strings.EqualFold(input, opt.Name) {
			return acp.PermissionResponse{Outcome: acp.PermissionResult{
				Outcome:  "selected",
				OptionID: strings.TrimSpace(opt.OptionID),
			}}
		}
	}
	return acp.PermissionResponse{Outcome: acp.PermissionResult{Outcome: "invalid"}}
}
