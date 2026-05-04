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
	type codexIndexEntry struct {
		ID        string `json:"id"`
		Title     string `json:"thread_name"`
		UpdatedAt string `json:"updated_at"`
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

	normalizedCWD := normalizeCWD(projectCWD)
	var results []ClaudeSessionInfo

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
		entry, ok := entries[sessionID]
		if !ok || managedIDs[sessionID] {
			return nil
		}
		cwd := readCodexSessionCWD(path)
		if normalizeCWD(cwd) != normalizedCWD {
			return nil
		}
		results = append(results, ClaudeSessionInfo{
			SessionID: sessionID,
			Title:     firstNonEmpty(strings.TrimSpace(entry.Title), sessionID),
			UpdatedAt: entry.UpdatedAt,
			CWD:       cwd,
		})
		return nil
	})
	return results, nil
}

func readCodexSessionCWD(jsonlPath string) string {
	f, err := os.Open(jsonlPath)
	if err != nil {
		return ""
	}
	defer f.Close()
	var buf [2048]byte
	n, _ := f.Read(buf[:])
	firstLine := string(buf[:n])
	if idx := strings.IndexByte(firstLine, '\n'); idx >= 0 {
		firstLine = firstLine[:idx]
	}
	var ev struct {
		Payload struct {
			CWD string `json:"cwd"`
		} `json:"payload"`
	}
	if json.Unmarshal([]byte(firstLine), &ev) == nil {
		return ev.Payload.CWD
	}
	return ""
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

type copilotEv struct {
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
	f, err := os.Open(eventsPath)
	if err != nil {
		return nil
	}
	defer f.Close()

	var buf [4096]byte
	n, _ := f.Read(buf[:])
	raw := string(buf[:n])

	// Parse first line: session.start event with CWD.
	var ev copilotEv
	firstEnd := strings.IndexByte(raw, '\n')
	if firstEnd < 0 {
		firstEnd = len(raw)
	}
	if err := json.Unmarshal([]byte(raw[:firstEnd]), &ev); err != nil || ev.Type != "session.start" {
		return nil
	}
	if normalizeCWD(ev.Data.Context.CWD) != normalizedCWD {
		return nil
	}

	sessionID := firstNonEmpty(ev.Data.SessionID, fallbackSessionID)
	updatedAt := ev.Data.StartTime
	if updatedAt == "" {
		if fi, err := os.Stat(eventsPath); err == nil {
			updatedAt = fi.ModTime().UTC().Format(time.RFC3339)
		}
	}
	title := sessionID
	// Try second line for first user message as title.
	if firstEnd+1 < len(raw) {
		secondEnd := strings.IndexByte(raw[firstEnd+1:], '\n')
		secondLine := raw[firstEnd+1:]
		if secondEnd >= 0 {
			secondLine = raw[firstEnd+1 : firstEnd+1+secondEnd]
		}
		var msg copilotEv
		if json.Unmarshal([]byte(secondLine), &msg) == nil && msg.Type == "session.message" && msg.Data.Content != "" {
			title = firstNonEmpty(strings.TrimSpace(msg.Data.Content), sessionID)
		}
	}

	return &ClaudeSessionInfo{
		SessionID: sessionID,
		Title:     title,
		UpdatedAt: updatedAt,
		CWD:       ev.Data.Context.CWD,
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

// -- ACP session/list fallback (for agents without disk storage) --

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
