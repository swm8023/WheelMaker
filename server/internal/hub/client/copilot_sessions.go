package client

import (
	"context"
	"strings"

	acp "github.com/swm8023/wheelmaker/internal/protocol"
	"github.com/swm8023/wheelmaker/internal/hub/agent"
)

// scanUnmanagedCopilotSessions spawns a temporary Copilot ACP process to list
// resumable sessions via session/list, filtered by CWD and excluding managed IDs.
func scanUnmanagedCopilotSessions(ctx context.Context, projectCWD string, managedIDs map[string]bool) ([]ClaudeSessionInfo, error) {
	provider := agent.NewCopilotProvider()
	conn, err := agent.NewOwnedProviderConn(provider, projectCWD)
	if err != nil {
		return nil, nil // copilot not available, no sessions to return
	}
	defer conn.Close()

	// Initialize
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

	// List sessions (no CWD filter at protocol level, we filter client-side)
	var listResult struct {
		Sessions []acp.SessionInfo `json:"sessions"`
	}
	if err := conn.Send(ctx, acp.MethodSessionList, acp.SessionListParams{}, &listResult); err != nil {
		return nil, nil
	}

	normalizedCWD := normalizeCWD(projectCWD)
	var results []ClaudeSessionInfo
	for _, s := range listResult.Sessions {
		sessionID := strings.TrimSpace(s.SessionID)
		if sessionID == "" {
			continue
		}
		if managedIDs[sessionID] {
			continue
		}
		if normalizeCWD(s.CWD) != normalizedCWD {
			continue
		}
		title := firstNonEmpty(strings.TrimSpace(s.Title), sessionID)
		results = append(results, ClaudeSessionInfo{
			SessionID: sessionID,
			Title:     title,
			UpdatedAt: s.UpdatedAt,
			CWD:       s.CWD,
		})
	}
	return results, nil
}

// verifyCopilotSessionExists checks that a session ID is known to Copilot via
// session/list and returns its metadata.
func verifyCopilotSessionExists(ctx context.Context, projectCWD, targetSessionID string) (*ClaudeSessionInfo, error) {
	provider := agent.NewCopilotProvider()
	conn, err := agent.NewOwnedProviderConn(provider, projectCWD)
	if err != nil {
		return nil, err
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
		return nil, err
	}

	var listResult struct {
		Sessions []acp.SessionInfo `json:"sessions"`
	}
	if err := conn.Send(ctx, acp.MethodSessionList, acp.SessionListParams{}, &listResult); err != nil {
		return nil, err
	}
	for _, s := range listResult.Sessions {
		if strings.TrimSpace(s.SessionID) == strings.TrimSpace(targetSessionID) {
			return &ClaudeSessionInfo{
				SessionID: strings.TrimSpace(s.SessionID),
				Title:     firstNonEmpty(strings.TrimSpace(s.Title), s.SessionID),
				UpdatedAt: s.UpdatedAt,
				CWD:       s.CWD,
			}, nil
		}
	}
	return nil, nil
}
