package client

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ClaudeSessionInfo represents a resumable Claude session discovered on disk.
type ClaudeSessionInfo struct {
	SessionID    string `json:"sessionId"`
	Title        string `json:"title"`
	Preview      string `json:"preview,omitempty"`
	UpdatedAt    string `json:"updatedAt"`
	MessageCount int    `json:"messageCount"`
	CWD          string `json:"cwd"`
}

type scanClaudeEvent struct {
	Type    string          `json:"type"`
	Message json.RawMessage `json:"message"`
	CWD     string          `json:"cwd"`
}

type scanClaudeMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"` // string or []contentBlock
}

type scanClaudeContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// scanUnmanagedClaudeSessions scans ~/.claude/projects/*/*.jsonl for sessions
// whose CWD matches the given projectCWD and are not already managed.
func scanUnmanagedClaudeSessions(projectCWD string, managedIDs map[string]bool) ([]ClaudeSessionInfo, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	claudeProjectsDir := filepath.Join(home, ".claude", "projects")

	entries, err := os.ReadDir(claudeProjectsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	normalizedCWD := normalizeCWD(projectCWD)
	var results []ClaudeSessionInfo

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		projectDir := filepath.Join(claudeProjectsDir, entry.Name())
		jsonlFiles, err := filepath.Glob(filepath.Join(projectDir, "*.jsonl"))
		if err != nil {
			continue
		}
		for _, jsonlPath := range jsonlFiles {
			info, err := readClaudeSessionInfo(jsonlPath, normalizedCWD, managedIDs)
			if err != nil || info == nil {
				continue
			}
			results = append(results, *info)
		}
	}

	return results, nil
}

// readClaudeSessionInfo extracts metadata from a single Claude jsonl session file.
func readClaudeSessionInfo(jsonlPath string, normalizedCWD string, managedIDs map[string]bool) (*ClaudeSessionInfo, error) {
	data, err := os.ReadFile(jsonlPath)
	if err != nil {
		return nil, err
	}

	sessionID := strings.TrimSuffix(filepath.Base(jsonlPath), ".jsonl")
	if managedIDs[sessionID] {
		return nil, nil
	}

	fileInfo, err := os.Stat(jsonlPath)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(data), "\n")
	messageCount := 0
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			messageCount++
		}
	}

	var firstCWD string
	var firstTitle string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var ev scanClaudeEvent
		if json.Unmarshal([]byte(line), &ev) != nil {
			continue
		}

		// Capture CWD from first event that has one
		if firstCWD == "" && ev.CWD != "" {
			firstCWD = ev.CWD
			if normalizeCWD(firstCWD) != normalizedCWD {
				return nil, nil // cwd mismatch, skip
			}
		}

		// Capture title from first user message (the prompt itself).
		if firstTitle == "" && ev.Type == "user" {
			var msg scanClaudeMessage
			if json.Unmarshal(ev.Message, &msg) == nil {
				if text, ok := extractAssistantText(msg.Content); ok && text != "" {
					firstTitle = strings.TrimSpace(text)
				}
			}
		}

		if firstCWD != "" && firstTitle != "" {
			break
		}
	}

	// If no CWD found in events, skip
	if firstCWD == "" {
		return nil, nil
	}

	if firstTitle == "" {
		firstTitle = sessionID
	}

	// Extract preview from last assistant message.
	var lastText string
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		var ev scanClaudeEvent
		if json.Unmarshal([]byte(line), &ev) != nil {
			continue
		}
		if ev.Type == "assistant" {
			var msg scanClaudeMessage
			if json.Unmarshal(ev.Message, &msg) == nil {
				if text, ok := extractAssistantText(msg.Content); ok && text != "" {
					lastText = text
					break
				}
			}
		}
	}

	return &ClaudeSessionInfo{
		SessionID:    sessionID,
		Title:        truncateString(firstTitle, 200),
		Preview:      truncateString(cleanClaudeReply(lastText), 200),
		UpdatedAt:    fileInfo.ModTime().UTC().Format(time.RFC3339),
		MessageCount: messageCount,
		CWD:          firstCWD,
	}, nil
}

// extractAssistantText parses the assistant content which may be a plain string
// or an array of content blocks (e.g., [{"type":"text","text":"..."}]).
func extractAssistantText(raw json.RawMessage) (string, bool) {
	// Try string first
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s, true
	}
	// Try []contentBlock
	var blocks []scanClaudeContentBlock
	if json.Unmarshal(raw, &blocks) != nil {
		return "", false
	}
	for _, b := range blocks {
		if b.Type == "text" && b.Text != "" {
			return b.Text, true
		}
	}
	return "", false
}

// cleanClaudeReply removes XML tags and whitespace from a Claude reply for use as a title.
func cleanClaudeReply(s string) string {
	// Strip common XML tags
	for _, tag := range []string{"thinking", "tool_calls", "tool_result", "antml:function_calls", "antml:function_results"} {
		for {
			open := "<" + tag + ">"
			close := "</" + tag + ">"
			i := strings.Index(s, open)
			if i < 0 {
				break
			}
			j := strings.Index(s[i:], close)
			if j < 0 {
				break
			}
			s = s[:i] + s[i+j+len(close):]
		}
	}
	return strings.TrimSpace(s)
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}

func normalizeCWD(cwd string) string {
	return strings.TrimRight(strings.TrimSpace(cwd), string(filepath.Separator))
}
