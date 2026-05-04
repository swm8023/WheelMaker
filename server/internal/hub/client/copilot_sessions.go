package client

import (
	"context"
	"strings"

	acp "github.com/swm8023/wheelmaker/internal/protocol"
	"github.com/swm8023/wheelmaker/internal/hub/agent"
)

// scanACPUnmanagedSessions spawns a temporary ACP process to list resumable
// sessions via session/list, filtered by CWD and excluding managed IDs.
func scanACPUnmanagedSessions(ctx context.Context, provider agent.ACPProvider, projectCWD string, managedIDs map[string]bool) ([]ClaudeSessionInfo, error) {
	conn, err := agent.NewOwnedProviderConn(provider, projectCWD)
	if err != nil {
		return nil, nil // agent not available, no sessions to return
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
	if !initResult.AgentCapabilities.LoadSession {
		return nil, nil
	}

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

// verifyACPSessionExists checks that a session ID is known to the agent via
// session/list and returns its metadata.
func verifyACPSessionExists(ctx context.Context, provider agent.ACPProvider, projectCWD, targetSessionID string) (*ClaudeSessionInfo, error) {
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

func acpProviderForAgentType(agentType string) agent.ACPProvider {
	switch strings.TrimSpace(agentType) {
	case "codex":
		return agent.NewCodexProvider()
	case "copilot":
		return agent.NewCopilotProvider()
	default:
		return nil
	}
}
