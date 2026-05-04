package client

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	acp "github.com/swm8023/wheelmaker/internal/protocol"
	"github.com/swm8023/wheelmaker/internal/hub/agent"
)

// -- Codex disk scanning (~/.codex/session_index.jsonl + sessions/) --

type codexIndexEntry struct {
	ID        string `json:"id"`
	Title     string `json:"thread_name"`
	UpdatedAt string `json:"updated_at"`
}

func scanUnmanagedCodexSessions(projectCWD string, managedIDs map[string]bool) ([]ClaudeSessionInfo, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, nil
	}

	// Load session_index for title/updated_at lookup.
	indexPath := filepath.Join(home, ".codex", "session_index.jsonl")
	indexData, _ := os.ReadFile(indexPath)
	indexEntries := make(map[string]codexIndexEntry)
	for _, line := range strings.Split(string(indexData), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var e codexIndexEntry
		if json.Unmarshal([]byte(line), &e) == nil && e.ID != "" {
			indexEntries[e.ID] = e
		}
	}

	normalizedCWD := normalizeCWD(projectCWD)
	var results []ClaudeSessionInfo

	// Walk session files directly — read first line for session ID + CWD.
	sessionsDir := filepath.Join(home, ".codex", "sessions")
	_ = filepath.WalkDir(sessionsDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(d.Name(), ".jsonl") {
			return nil
		}
		sessionID, cwd := readCodexSessionMeta(path)
		if sessionID == "" {
			return nil
		}
		if managedIDs[sessionID] {
			return nil
		}
		if normalizeCWD(cwd) != normalizedCWD {
			return nil
		}
		entry := indexEntries[sessionID]
		updatedAt := entry.UpdatedAt
		if updatedAt == "" {
			if fi, err := d.Info(); err == nil {
				updatedAt = fi.ModTime().UTC().Format(time.RFC3339)
			}
		}
		title := firstNonEmpty(strings.TrimSpace(entry.Title), sessionID)
		preview := readCodexSessionPreview(path, title)
		results = append(results, ClaudeSessionInfo{
			SessionID: sessionID,
			Title:     title,
			Preview:   preview,
			UpdatedAt: updatedAt,
			CWD:       cwd,
		})
		return nil
	})
	return results, nil
}

// readCodexSessionMeta reads the first line of a Codex session file and returns sessionID + CWD.
func readCodexSessionMeta(path string) (sessionID, cwd string) {
	f, err := os.Open(path)
	if err != nil {
		return "", ""
	}
	defer f.Close()
	// First line can be very long (includes base_instructions text).
	// Use a bufio.Scanner to handle arbitrary line length.
	scanner := bufio.NewScanner(f)
	if !scanner.Scan() {
		return "", ""
	}
	firstLine := scanner.Bytes()
	var ev struct {
		Payload struct {
			ID  string `json:"id"`
			CWD string `json:"cwd"`
		} `json:"payload"`
	}
	if json.Unmarshal(firstLine, &ev) != nil {
		return "", ""
	}
	return ev.Payload.ID, ev.Payload.CWD
}

// readCodexSessionPreview scans a Codex session file backwards for a preview.
func readCodexSessionPreview(path, fallback string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return fallback
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	type evMsg struct {
		Type    string `json:"type"`
		Payload struct {
			Type             string `json:"type"`
			LastAgentMessage string `json:"last_agent_message"`
		} `json:"payload"`
	}
	for i := len(lines) - 1; i >= 0; i-- {
		var msg evMsg
		if json.Unmarshal([]byte(lines[i]), &msg) != nil || msg.Type != "event_msg" {
			continue
		}
		if msg.Payload.Type == "task_complete" && msg.Payload.LastAgentMessage != "" {
			return truncateString(strings.TrimSpace(msg.Payload.LastAgentMessage), 200)
		}
	}
	return fallback
}

// -- Copilot disk scanning (~/.copilot/session-state/<id>/events.jsonl) --

func scanUnmanagedCopilotDiskSessions(projectCWD string, managedIDs map[string]bool) ([]ClaudeSessionInfo, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, nil
	}
	stateDir := filepath.Join(home, ".copilot", "session-state")
	entries, err := os.ReadDir(stateDir)
	if err != nil {
		return nil, nil
	}

	normalizedCWD := normalizeCWD(projectCWD)
	var results []ClaudeSessionInfo

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		sessionID := entry.Name()
		if managedIDs[sessionID] {
			continue
		}
		eventsPath := filepath.Join(stateDir, sessionID, "events.jsonl")
		info := readCopilotSessionInfo(eventsPath, normalizedCWD, sessionID)
		if info != nil {
			results = append(results, *info)
		}
	}
	return results, nil
}

type copilotEvent struct {
	Type string `json:"type"`
	Data struct {
		SessionID string `json:"sessionId"`
		StartTime string `json:"startTime"`
		Content   string `json:"content"`
		Context   struct {
			CWD string `json:"cwd"`
		} `json:"context"`
	} `json:"data"`
}

func readCopilotSessionInfo(eventsPath, normalizedCWD, fallbackSessionID string) *ClaudeSessionInfo {
	data, err := os.ReadFile(eventsPath)
	if err != nil || len(data) == 0 {
		return nil
	}

	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) == 0 {
		return nil
	}

	// First line must be session.start with CWD.
	var start copilotEvent
	if json.Unmarshal([]byte(lines[0]), &start) != nil || start.Type != "session.start" {
		return nil
	}
	if normalizeCWD(start.Data.Context.CWD) != normalizedCWD {
		return nil
	}

	sessionID := firstNonEmpty(start.Data.SessionID, fallbackSessionID)
	updatedAt := start.Data.StartTime
	if updatedAt == "" {
		if fi, err := os.Stat(eventsPath); err == nil {
			updatedAt = fi.ModTime().UTC().Format(time.RFC3339)
		}
	}

	// Scan forward for first user.message as title.
	title := sessionID
	preview := ""
	for i := 1; i < len(lines); i++ {
		var ev copilotEvent
		if json.Unmarshal([]byte(lines[i]), &ev) != nil {
			continue
		}
		if title == sessionID && ev.Type == "user.message" && ev.Data.Content != "" {
			title = firstNonEmpty(strings.TrimSpace(ev.Data.Content), sessionID)
		}
		// Collect last meaningful content as preview.
		switch ev.Type {
		case "user.message", "assistant.message":
			if c := strings.TrimSpace(ev.Data.Content); c != "" {
				preview = c
			}
		}
	}
	if preview == "" {
		preview = title
	}

	return &ClaudeSessionInfo{
		SessionID: sessionID,
		Title:     title,
		Preview:   truncateString(preview, 200),
		UpdatedAt: updatedAt,
		CWD:       start.Data.Context.CWD,
	}
}

// -- Unified disk scanner (preferred: no process spawn needed) --

func scanUnmanagedDiskSessions(agentType, projectCWD string, managedIDs map[string]bool) ([]ClaudeSessionInfo, error) {
	switch strings.TrimSpace(agentType) {
	case "codex":
		return scanUnmanagedCodexSessions(projectCWD, managedIDs)
	case "copilot":
		return scanUnmanagedCopilotDiskSessions(projectCWD, managedIDs)
	default:
		return nil, nil
	}
}

// -- ACP session/list fallback --

func scanACPUnmanagedSessions(ctx context.Context, provider agent.ACPProvider, projectCWD string, managedIDs map[string]bool) ([]ClaudeSessionInfo, error) {
	conn, err := agent.NewOwnedProviderConn(provider, projectCWD)
	if err != nil {
		return nil, nil
	}
	defer conn.Close()

	var initResult struct {
		AgentCapabilities struct {
			LoadSession bool `json:"loadSession"`
		} `json:"agentCapabilities"`
	}
	if err := conn.Send(ctx, acp.MethodInitialize, acp.ClientCapabilities{
		FS:       &acp.FSCapabilities{ReadTextFile: true, WriteTextFile: true},
		Terminal: true,
	}, &initResult); err != nil {
		return nil, nil
	}
	if !initResult.AgentCapabilities.LoadSession {
		return nil, nil
	}

	var listResult struct {
		Sessions []acp.SessionInfo `json:"sessions"`
	}
	if err := conn.Send(ctx, acp.MethodSessionList, acp.SessionListParams{CWD: projectCWD}, &listResult); err != nil {
		return nil, nil
	}

	normalizedCWD := normalizeCWD(projectCWD)
	var results []ClaudeSessionInfo
	for _, s := range listResult.Sessions {
		sessionID := strings.TrimSpace(s.SessionID)
		if sessionID == "" || managedIDs[sessionID] {
			continue
		}
		if normalizeCWD(s.CWD) != normalizedCWD {
			continue
		}
		results = append(results, ClaudeSessionInfo{
			SessionID: sessionID,
			Title:     firstNonEmpty(strings.TrimSpace(s.Title), sessionID),
			UpdatedAt: s.UpdatedAt,
			CWD:       s.CWD,
		})
	}
	return results, nil
}
