package client

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	acp "github.com/swm8023/wheelmaker/internal/protocol"
)

// ClaudeSessionInfo represents a resumable Claude session discovered on disk.
type ClaudeSessionInfo struct {
	SessionID    string `json:"sessionId"`
	Title        string `json:"title"`
	UpdatedAt    string `json:"updatedAt"`    // ISO 8601
	MessageCount int    `json:"messageCount"` // jsonl lines
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

		// Capture title from first assistant message
		if firstTitle == "" && ev.Type == "assistant" {
			var msg scanClaudeMessage
			if json.Unmarshal(ev.Message, &msg) == nil {
				if text, ok := extractAssistantText(msg.Content); ok && text != "" {
					firstTitle = cleanClaudeReply(text)
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

	return &ClaudeSessionInfo{
		SessionID:    sessionID,
		Title:        truncateString(firstTitle, 200),
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

// reloadClaudeSessionPrompts reads a Claude jsonl session file and converts
// user/assistant exchanges into SessionPromptRecords for the session_prompts table.
func reloadClaudeSessionPrompts(ctx context.Context, store Store, projectName, sessionID string) error {
	jsonlPath, err := findClaudeSessionFile(sessionID)
	if err != nil {
		return err
	}

	data, err := os.ReadFile(jsonlPath)
	if err != nil {
		return err
	}

	// Parse events and group into exchanges (user → assistant pairs)
	type exchange struct {
		userText      string
		assistantText string
	}
	var exchanges []exchange
	var currentUserText string

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var ev scanClaudeEvent
		if json.Unmarshal([]byte(line), &ev) != nil {
			continue
		}

		if ev.Type == "user" {
			var msg scanClaudeMessage
			if json.Unmarshal(ev.Message, &msg) != nil {
				continue
			}
			content, _ := extractAssistantText(msg.Content) // extracts string content
			if content == "" {
				continue
			}
			if strings.HasPrefix(content, "/model") || strings.HasPrefix(content, "/quit") ||
				strings.HasPrefix(content, "/config") || strings.HasPrefix(content, "/exit") ||
				strings.HasPrefix(content, "/effort") {
				continue
			}
			// If there's a pending exchange, flush it
			if currentUserText != "" {
				exchanges = append(exchanges, exchange{userText: currentUserText})
				currentUserText = ""
			}
			currentUserText = content
		}

		if ev.Type == "assistant" && currentUserText != "" {
			var msg scanClaudeMessage
			if json.Unmarshal(ev.Message, &msg) != nil {
				continue
			}
			text, ok := extractAssistantText(msg.Content)
			if !ok || text == "" {
				continue
			}
			text = cleanClaudeReply(text)
			if text == "" {
				continue
			}
			exchanges = append(exchanges, exchange{userText: currentUserText, assistantText: text})
			currentUserText = ""
		}
	}
	// Flush remaining user message (no assistant reply)
	if currentUserText != "" {
		exchanges = append(exchanges, exchange{userText: currentUserText})
	}

	for i, ex := range exchanges {
		if err := saveExchangeAsPrompt(ctx, store, projectName, sessionID, int64(i+1), ex); err != nil {
			return fmt.Errorf("save prompt %d: %w", i+1, err)
		}
	}
	return nil
}

// findClaudeSessionFile locates a session jsonl file under ~/.claude/projects/*/*.jsonl.
func findClaudeSessionFile(sessionID string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	projectsDir := filepath.Join(home, ".claude", "projects")
	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		return "", err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		candidate := filepath.Join(projectsDir, entry.Name(), sessionID+".jsonl")
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("session file not found for %s", sessionID)
}

// saveExchangeAsPrompt converts a user/assistant exchange into a SessionPromptRecord and upserts it.
func saveExchangeAsPrompt(ctx context.Context, store Store, projectName, sessionID string, promptIndex int64, ex struct{ userText, assistantText string }) error {
	now := time.Now().UTC()

	// Build turns: prompt_request → agent_message → prompt_done
	var turns []string

	// Turn 1: prompt_request
	promptParam := acp.IMPromptRequest{
		ContentBlocks: []acp.ContentBlock{
			{Type: acp.ContentBlockTypeText, Text: ex.userText},
		},
	}
	turns = append(turns, buildReloadTurn("prompt_request", promptParam))

	// Turn 2: agent_message (if assistant replied)
	if ex.assistantText != "" {
		turns = append(turns, buildReloadTurn("agent_message", acp.IMTextResult{Text: ex.assistantText}))
	}

	// Turn 3: prompt_done
	turns = append(turns, buildReloadTurn("prompt_done", acp.IMPromptResult{StopReason: "end_turn"}))

	// Encode turns
	turnsJSON, err := json.Marshal(turns)
	if err != nil {
		return err
	}

	title := truncateString(ex.userText, 200)

	rec := SessionPromptRecord{
		SessionID:   sessionID,
		PromptIndex: promptIndex,
		Title:       title,
		StopReason:  "end_turn",
		TurnsJSON:   string(turnsJSON),
		TurnIndex:   int64(len(turns)),
		StartedAt:   now,
		UpdatedAt:   now,
	}
	return store.UpsertSessionPrompt(ctx, rec)
}

func buildReloadTurn(method string, param any) string {
	raw, _ := json.Marshal(param)
	turn := acp.IMTurnMessage{
		Method: method,
		Param:  raw,
	}
	out, _ := json.Marshal(turn)
	return string(out)
}
