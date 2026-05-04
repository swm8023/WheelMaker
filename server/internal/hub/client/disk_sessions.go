package client

import (
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
	indexPath := filepath.Join(home, ".codex", "session_index.jsonl")
	indexData, err := os.ReadFile(indexPath)
	if err != nil {
		return nil, nil
	}

	entries := make(map[string]codexIndexEntry)
	for _, line := range strings.Split(string(indexData), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var e codexIndexEntry
		if json.Unmarshal([]byte(line), &e) == nil && e.ID != "" {
			entries[e.ID] = e
		}
	}
	if len(entries) == 0 {
		return nil, nil
	}

	// Build a set of session files by their session ID (extracted from filename).
	// Format: sessions/YYYY/MM/DD/rollout-...-<uuid>.jsonl
	sessionFiles := map[string][]string{} // sessionID -> paths
	sessionsDir := filepath.Join(home, ".codex", "sessions")
	_ = filepath.WalkDir(sessionsDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(d.Name(), ".jsonl") {
			return nil
		}
		base := strings.TrimSuffix(d.Name(), ".jsonl")
		lastDash := strings.LastIndex(base, "-")
		if lastDash < 0 {
			return nil
		}
		sessionID := base[lastDash+1:]
		sessionFiles[sessionID] = append(sessionFiles[sessionID], path)
		return nil
	})

	normalizedCWD := normalizeCWD(projectCWD)
	var results []ClaudeSessionInfo

	for _, entry := range entries {
		if managedIDs[entry.ID] {
			continue
		}
		paths := sessionFiles[entry.ID]
		if len(paths) == 0 {
			continue // can't verify CWD without a session file
		}
		// Use the first (usually only) session file.
		info := readCodexSessionFile(paths[0], normalizedCWD, entry)
		if info != nil {
			results = append(results, *info)
		}
	}
	return results, nil
}

type codexLineMeta struct {
	Payload struct {
		CWD string `json:"cwd"`
	} `json:"payload"`
}

type codexLineMsg struct {
	Type    string `json:"type"`
	Payload struct {
		Type             string `json:"type"`
		LastAgentMessage string `json:"last_agent_message"`
	} `json:"payload"`
}

func readCodexSessionFile(path, normalizedCWD string, entry codexIndexEntry) *ClaudeSessionInfo {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	// First line: session_meta with CWD.
	var firstLine string
	raw := string(data)
	if idx := strings.IndexByte(raw, '\n'); idx >= 0 {
		firstLine = raw[:idx]
	} else {
		firstLine = raw
	}
	var meta codexLineMeta
	if json.Unmarshal([]byte(firstLine), &meta) != nil {
		return nil
	}
	if normalizeCWD(meta.Payload.CWD) != normalizedCWD {
		return nil
	}

	// Last few lines: look for event_msg with a preview.
	preview := firstNonEmpty(strings.TrimSpace(entry.Title), entry.ID)
	lines := strings.Split(strings.TrimRight(raw, "\n"), "\n")
	// Scan backwards for a meaningful preview.
	for i := len(lines) - 1; i >= 0; i-- {
		var msg codexLineMsg
		if json.Unmarshal([]byte(lines[i]), &msg) != nil {
			continue
		}
		if msg.Type != "event_msg" {
			continue
		}
		switch msg.Payload.Type {
		case "task_complete":
			if t := strings.TrimSpace(msg.Payload.LastAgentMessage); t != "" {
				preview = truncateString(t, 200)
			}
		case "assistant_response", "user_prompt":
			// These have more detail but are harder to parse generically.
			// Fall through to use the index title.
		}
		if preview != entry.Title && preview != entry.ID {
			break // found a good preview
		}
	}

	return &ClaudeSessionInfo{
		SessionID: entry.ID,
		Title:     firstNonEmpty(strings.TrimSpace(entry.Title), entry.ID),
		Preview:   preview,
		UpdatedAt: entry.UpdatedAt,
		CWD:       meta.Payload.CWD,
	}
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
