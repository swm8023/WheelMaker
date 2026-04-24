package feishu

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/swm8023/wheelmaker/internal/im"
	acp "github.com/swm8023/wheelmaker/internal/protocol"
)

// buildPlanCard renders agent plan entries as a structured card.
func buildPlanCard(update acp.SessionUpdate) RawCard {
	if len(update.Entries) == 0 {
		return nil
	}
	lines := make([]string, 0, len(update.Entries))
	for _, entry := range update.Entries {
		content := strings.TrimSpace(entry.Content)
		if content == "" {
			continue
		}
		emoji := planStatusEmoji(entry.Status)
		lines = append(lines, fmt.Sprintf("%s %s", emoji, previewLine(content, 80)))
	}
	if len(lines) == 0 {
		return nil
	}
	md := strings.Join(lines, "\n")
	return RawCard{
		"config": map[string]any{"update_multi": true},
		"header": map[string]any{
			"template": "indigo",
			"title": map[string]any{
				"tag":     "plain_text",
				"content": "📋 Plan",
			},
		},
		"elements": []map[string]any{
			{"tag": "markdown", "content": md},
		},
	}
}

func planStatusEmoji(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "completed", "done":
		return "✅"
	case "in_progress", "in-progress", "running":
		return "⏳"
	case "failed", "error":
		return "❌"
	case "skipped":
		return "⏭️"
	default:
		return "⬜"
	}
}

// buildConfigCard renders a config update as a small system card.
func buildConfigCard(update acp.SessionUpdate) RawCard {
	snap := acp.SessionConfigSnapshotFromOptions(update.ConfigOptions)
	parts := make([]string, 0, 4)
	if strings.TrimSpace(snap.Mode) != "" {
		parts = append(parts, "**mode** = "+strings.TrimSpace(snap.Mode))
	}
	if strings.TrimSpace(snap.Model) != "" {
		parts = append(parts, "**model** = "+strings.TrimSpace(snap.Model))
	}
	if len(parts) == 0 {
		return nil
	}
	return RawCard{
		"config": map[string]any{"update_multi": true},
		"header": map[string]any{
			"template": "grey",
			"title": map[string]any{
				"tag":     "plain_text",
				"content": "⚙️ Config Updated",
			},
		},
		"elements": []map[string]any{
			{"tag": "markdown", "content": strings.Join(parts, "\n")},
		},
	}
}

func renderUsageStreamText(update acp.SessionUpdate) string {
	size, used, ok := usageMetricsFromUpdate(update)
	if !ok {
		return ""
	}
	percent := (float64(used) / float64(size)) * 100
	return fmt.Sprintf("Context %.0f%%", percent)
}

func usageMetricsFromUpdate(update acp.SessionUpdate) (size int64, used int64, ok bool) {
	if update.Size == nil || update.Used == nil {
		return 0, 0, false
	}
	size = *update.Size
	used = *update.Used
	if size <= 0 || used < 0 {
		return 0, 0, false
	}
	return size, used, true
}

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
	return block.Text
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

func renderDefaultStr(v, d string) string {
	if strings.TrimSpace(v) == "" {
		return d
	}
	return strings.TrimSpace(v)
}

func resolveHelpMenu(model im.HelpModel, menuID string) (title, body string, options []im.HelpOption, parent string) {
	rootID := strings.TrimSpace(model.RootMenu)
	if rootID == "" {
		rootID = "root"
	}
	if strings.TrimSpace(menuID) == "" || menuID == rootID {
		return renderDefaultStr(model.Title, "Help"), strings.TrimSpace(model.Body), model.Options, ""
	}
	if menu, ok := model.Menus[menuID]; ok {
		return renderDefaultStr(menu.Title, renderDefaultStr(model.Title, "Help")), strings.TrimSpace(menu.Body), menu.Options, strings.TrimSpace(menu.Parent)
	}
	return renderDefaultStr(model.Title, "Help"), strings.TrimSpace(model.Body), model.Options, ""
}

func buildHelpCard(chatID string, model im.HelpModel, menuID string, page int) RawCard {
	const pageSize = 8

	title, body, options, parent := resolveHelpMenu(model, menuID)

	if page < 0 {
		page = 0
	}
	total := len(options)
	maxPage := 0
	if total > 0 {
		maxPage = (total - 1) / pageSize
		if page > maxPage {
			page = maxPage
		}
	}
	start := page * pageSize
	end := start + pageSize
	if end > total {
		end = total
	}

	actions := make([]map[string]any, 0, end-start)
	for _, opt := range options[start:end] {
		if strings.TrimSpace(opt.MenuID) != "" {
			actions = append(actions, map[string]any{
				"tag":  "button",
				"text": map[string]any{"tag": "plain_text", "content": opt.Label},
				"type": "default",
				"value": map[string]any{
					"kind":    "help_menu",
					"chat_id": chatID,
					"menu_id": opt.MenuID,
				},
			})
			continue
		}
		actions = append(actions, map[string]any{
			"tag":  "button",
			"text": map[string]any{"tag": "plain_text", "content": opt.Label},
			"type": "default",
			"value": map[string]any{
				"kind":    "help_option",
				"chat_id": chatID,
				"menu_id": menuID,
				"command": opt.Command,
				"value":   opt.Value,
			},
		})
	}

	elements := []map[string]any{
		{"tag": "markdown", "content": strings.TrimSpace(body)},
	}
	if len(actions) > 0 {
		elements = append(elements, map[string]any{"tag": "action", "actions": actions})
	}

	if maxPage > 0 {
		nav := make([]map[string]any, 0, 2)
		if page > 0 {
			nav = append(nav, map[string]any{
				"tag":  "button",
				"text": map[string]any{"tag": "plain_text", "content": "Prev"},
				"type": "default",
				"value": map[string]any{
					"kind":    "help_page",
					"chat_id": chatID,
					"menu_id": menuID,
					"page":    strconv.Itoa(page - 1),
				},
			})
		}
		if page < maxPage {
			nav = append(nav, map[string]any{
				"tag":  "button",
				"text": map[string]any{"tag": "plain_text", "content": "Next"},
				"type": "default",
				"value": map[string]any{
					"kind":    "help_page",
					"chat_id": chatID,
					"menu_id": menuID,
					"page":    strconv.Itoa(page + 1),
				},
			})
		}
		if len(nav) > 0 {
			elements = append(elements, map[string]any{"tag": "action", "actions": nav})
		}
	}

	if strings.TrimSpace(parent) != "" {
		elements = append(elements, map[string]any{
			"tag": "action",
			"actions": []map[string]any{
				{
					"tag":  "button",
					"text": map[string]any{"tag": "plain_text", "content": "Back"},
					"type": "primary",
					"value": map[string]any{
						"kind":    "help_menu",
						"chat_id": chatID,
						"menu_id": parent,
					},
				},
			},
		})
	}

	headerTitle := renderDefaultStr(title, "Help")
	if maxPage > 0 {
		headerTitle = fmt.Sprintf("%s (%d/%d)", headerTitle, page+1, maxPage+1)
	}

	return RawCard{
		"config": map[string]any{"update_multi": true},
		"header": map[string]any{
			"title": map[string]any{
				"tag":     "plain_text",
				"content": headerTitle,
			},
			"template": "green",
		},
		"elements": elements,
	}
}
