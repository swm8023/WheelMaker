package client

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	acp "github.com/swm8023/wheelmaker/internal/protocol"
)

type recoverySession struct {
	SessionID    string `json:"sessionId"`
	AgentType    string `json:"agentType,omitempty"`
	Title        string `json:"title"`
	Preview      string `json:"preview,omitempty"`
	UpdatedAt    string `json:"updatedAt"`
	MessageCount int    `json:"messageCount"`
	CWD          string `json:"cwd"`
}

type recoverySource interface {
	AgentType() string
	List(projectCWD string, managedIDs map[string]bool) ([]recoverySession, error)
	Find(projectCWD, sessionID string, managedIDs map[string]bool) (*recoverySession, error)
}

type sessionRecovery struct {
	client *Client
}

func (c *Client) recovery() *sessionRecovery {
	return &sessionRecovery{client: c}
}

func (r *sessionRecovery) ListResumableSessions(ctx context.Context, agentType string) (map[string]any, error) {
	agentType, err := normalizeRecoveryAgentType(agentType)
	if err != nil {
		return nil, err
	}
	managed, err := r.managedSessionIDs(ctx)
	if err != nil {
		return nil, err
	}
	source, err := r.sourceFor(agentType)
	if err != nil {
		return nil, err
	}
	sessions, err := source.List(r.client.cwd, managed)
	if err != nil {
		hubLogger(r.client.projectName).Warn("session.resume.list scan failed agent=%s err=%v", agentType, err)
		return nil, err
	}
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].UpdatedAt > sessions[j].UpdatedAt
	})
	hubLogger(r.client.projectName).Info("session.resume.list agent=%s count=%d", agentType, len(sessions))
	return map[string]any{"sessions": sessions}, nil
}

func (r *sessionRecovery) ImportResumableSession(ctx context.Context, agentType, sessionID string) (map[string]any, error) {
	agentType, err := normalizeRecoveryAgentType(agentType)
	if err != nil {
		return nil, err
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, fmt.Errorf("sessionId is required")
	}
	managed, err := r.managedSessionIDs(ctx)
	if err != nil {
		return nil, err
	}
	if managed[sessionID] {
		return nil, fmt.Errorf("session already managed")
	}
	source, err := r.sourceFor(agentType)
	if err != nil {
		return nil, err
	}
	info, err := source.Find(r.client.cwd, sessionID, managed)
	if err != nil {
		return nil, err
	}
	if info == nil {
		return nil, fmt.Errorf("session not found")
	}
	lastActiveAt := parseRecoveryUpdatedAt(info.UpdatedAt)
	rec := &SessionRecord{
		ID:           sessionID,
		ProjectName:  r.client.projectName,
		Status:       SessionPersisted,
		AgentType:    agentType,
		Title:        strings.TrimSpace(info.Title),
		LastActiveAt: lastActiveAt,
	}
	if err := r.client.store.SaveSession(ctx, rec); err != nil {
		return nil, fmt.Errorf("save session: %w", err)
	}
	if err := r.client.store.SaveProjectDefaultAgent(ctx, r.client.projectName, rec.AgentType); err != nil {
		hubLogger(r.client.projectName).Warn("save project default agent failed agent=%s err=%v", rec.AgentType, err)
	}
	summary := sessionViewSummary{
		SessionID: sessionID,
		Title:     strings.TrimSpace(info.Title),
		UpdatedAt: lastActiveAt.UTC().Format(time.RFC3339),
		AgentType: rec.AgentType,
	}
	return map[string]any{"ok": true, "session": summary}, nil
}

func (r *sessionRecovery) ReloadSession(ctx context.Context, sessionID string) (map[string]any, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, fmt.Errorf("sessionId is required")
	}
	rec, err := r.client.store.LoadSession(ctx, r.client.projectName, sessionID)
	if err != nil {
		return nil, fmt.Errorf("load session: %w", err)
	}
	if rec != nil {
		rec.SessionSyncJSON = sessionSyncJSON(0)
		if err := r.client.store.SaveSession(ctx, rec); err != nil {
			return nil, fmt.Errorf("reset session sync: %w", err)
		}
	}
	if r.client.sessionRecorder != nil {
		if err := r.client.sessionRecorder.ResetSessionTurns(ctx, sessionID); err != nil {
			return nil, fmt.Errorf("reset session turns: %w", err)
		}
	}
	sess, err := r.client.SessionByID(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("resolve session: %w", err)
	}
	if err := sess.Suspend(ctx); err != nil {
		return nil, fmt.Errorf("suspend session: %w", err)
	}
	updates, err := sess.captureReplay(ctx)
	if err != nil {
		return nil, fmt.Errorf("capture replay: %w", err)
	}
	r.feedReplayToRecorder(ctx, sessionID, updates)
	return map[string]any{"ok": true, "sessionId": sessionID}, nil
}

func (r *sessionRecovery) managedSessionIDs(ctx context.Context) (map[string]bool, error) {
	managed := make(map[string]bool)
	if r.client.store == nil {
		return managed, nil
	}
	recs, err := r.client.store.ListSessions(ctx, r.client.projectName)
	if err != nil {
		return nil, err
	}
	for _, rec := range recs {
		managed[rec.ID] = true
	}
	return managed, nil
}

func (r *sessionRecovery) sourceFor(agentType string) (recoverySource, error) {
	switch agentType {
	case "claude":
		return claudeRecoverySource{}, nil
	case "codex":
		return codexRecoverySource{}, nil
	case "codexapp":
		return codexappRecoverySource{}, nil
	case "copilot":
		return copilotRecoverySource{}, nil
	default:
		return nil, fmt.Errorf("unsupported recovery agent: %s", agentType)
	}
}

func normalizeRecoveryAgentType(agentType string) (string, error) {
	agentType = strings.ToLower(strings.TrimSpace(agentType))
	if agentType == "" {
		return "", fmt.Errorf("agentType is required")
	}
	return agentType, nil
}

func parseRecoveryUpdatedAt(value string) time.Time {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return time.Now().UTC()
	}
	parsed, err := time.Parse(time.RFC3339, trimmed)
	if err != nil {
		return time.Now().UTC()
	}
	return parsed.UTC()
}

func findRecoverySession(items []recoverySession, sessionID string) *recoverySession {
	sessionID = strings.TrimSpace(sessionID)
	for i := range items {
		if items[i].SessionID == sessionID {
			cp := items[i]
			return &cp
		}
	}
	return nil
}

func normalizeRecoveryCWD(cwd string) string {
	s := strings.TrimSpace(cwd)
	s = strings.TrimRight(s, string(filepath.Separator))
	s = strings.TrimRight(s, "/")
	return strings.ToLower(s)
}

func truncateRecoveryText(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}

func (s *Session) captureReplay(ctx context.Context) ([]acp.SessionUpdateParams, error) {
	s.promptMu.Lock()
	defer s.promptMu.Unlock()

	if err := s.ensureInstance(ctx); err != nil {
		return nil, err
	}

	captureCh := make(chan acp.SessionUpdateParams, 2048)
	s.mu.Lock()
	s.prompt.updatesCh = captureCh
	s.mu.Unlock()

	if err := s.ensureReadyAndNotify(ctx); err != nil {
		s.mu.Lock()
		s.prompt.updatesCh = nil
		s.mu.Unlock()
		return nil, err
	}

	s.mu.Lock()
	s.prompt.updatesCh = nil
	s.mu.Unlock()

	var updates []acp.SessionUpdateParams
	for {
		select {
		case u := <-captureCh:
			updates = append(updates, u)
		default:
			return updates, nil
		}
	}
}

func (r *sessionRecovery) feedReplayToRecorder(ctx context.Context, sessionID string, updates []acp.SessionUpdateParams) {
	if len(updates) == 0 || r.client.sessionRecorder == nil {
		return
	}

	finishPrompt := func() {
		_ = r.client.sessionRecorder.RecordEvent(ctx, SessionViewEvent{
			Type:      SessionViewEventTypeACP,
			SessionID: sessionID,
			Content: acp.BuildACPContentJSON(acp.MethodSessionPrompt, map[string]any{
				"result": acp.SessionPromptResult{StopReason: "end_turn"},
			}),
		})
	}

	startPrompt := func(text string) {
		_ = r.client.sessionRecorder.RecordEvent(ctx, SessionViewEvent{
			Type:      SessionViewEventTypeACP,
			SessionID: sessionID,
			Content: acp.BuildACPContentJSON(acp.MethodSessionPrompt, map[string]any{
				"params": acp.SessionPromptParams{
					SessionID: sessionID,
					Prompt:    []acp.ContentBlock{{Type: acp.ContentBlockTypeText, Text: text}},
				},
			}),
		})
	}

	recordUpdate := func(u acp.SessionUpdateParams) {
		_ = r.client.sessionRecorder.RecordEvent(ctx, SessionViewEvent{
			Type:      SessionViewEventTypeACP,
			SessionID: sessionID,
			Content: acp.BuildACPContentJSON(acp.MethodSessionUpdate, map[string]any{
				"params": u,
			}),
		})
	}

	hasPending := false
	recordedAny := false
	for _, u := range updates {
		switch u.Update.SessionUpdate {
		case acp.SessionUpdateConfigOptionUpdate,
			acp.SessionUpdateAvailableCommandsUpdate,
			acp.SessionUpdateSessionInfoUpdate,
			acp.SessionUpdateCurrentModeUpdate,
			acp.SessionUpdateUsageUpdate:
			continue
		}

		if u.Update.SessionUpdate == acp.SessionUpdateUserMessageChunk {
			if hasPending || recordedAny {
				finishPrompt()
				recordedAny = false
			}
			startPrompt(extractRecoveryUpdateText(u.Update.Content))
			hasPending = true
			continue
		}

		if !hasPending && !recordedAny {
			if err := r.ensureReplayPromptState(ctx, sessionID); err != nil {
				hubLogger(r.client.projectName).Warn("session.reload replay prompt init failed session=%s err=%v", sessionID, err)
				return
			}
		}
		recordUpdate(u)
		recordedAny = true
	}
	if hasPending || recordedAny {
		finishPrompt()
	}
}

func (r *sessionRecovery) ensureReplayPromptState(ctx context.Context, sessionID string) error {
	recorder := r.client.sessionRecorder
	if recorder == nil {
		return nil
	}
	recorder.writeMu.Lock()
	defer recorder.writeMu.Unlock()
	_, err := recorder.ensurePromptStateLocked(ctx, sessionID)
	return err
}

func extractRecoveryUpdateText(raw json.RawMessage) string {
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text
	}
	block := struct {
		Text string `json:"text,omitempty"`
	}{}
	if err := json.Unmarshal(raw, &block); err == nil {
		return block.Text
	}
	return ""
}

type claudeRecoverySource struct{}

func (claudeRecoverySource) AgentType() string { return "claude" }

func (claudeRecoverySource) List(projectCWD string, managedIDs map[string]bool) ([]recoverySession, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	projectsDir := filepath.Join(home, ".claude", "projects")
	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	normalizedCWD := normalizeRecoveryCWD(projectCWD)
	var results []recoverySession
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		jsonlFiles, err := filepath.Glob(filepath.Join(projectsDir, entry.Name(), "*.jsonl"))
		if err != nil {
			continue
		}
		for _, jsonlPath := range jsonlFiles {
			info, err := readClaudeRecoverySession(jsonlPath, normalizedCWD, managedIDs)
			if err != nil || info == nil {
				continue
			}
			results = append(results, *info)
		}
	}
	return results, nil
}

func (s claudeRecoverySource) Find(projectCWD, sessionID string, managedIDs map[string]bool) (*recoverySession, error) {
	items, err := s.List(projectCWD, managedIDs)
	if err != nil {
		return nil, err
	}
	return findRecoverySession(items, sessionID), nil
}

type scanClaudeRecoveryEvent struct {
	Type    string          `json:"type"`
	Message json.RawMessage `json:"message"`
	CWD     string          `json:"cwd"`
}

type scanClaudeRecoveryMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type scanClaudeRecoveryContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func readClaudeRecoverySession(jsonlPath, normalizedCWD string, managedIDs map[string]bool) (*recoverySession, error) {
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
		var ev scanClaudeRecoveryEvent
		if json.Unmarshal([]byte(line), &ev) != nil {
			continue
		}
		if firstCWD == "" && ev.CWD != "" {
			firstCWD = ev.CWD
			if normalizeRecoveryCWD(firstCWD) != normalizedCWD {
				return nil, nil
			}
		}
		if firstTitle == "" && ev.Type == "user" {
			var msg scanClaudeRecoveryMessage
			if json.Unmarshal(ev.Message, &msg) == nil && msg.Role == "user" {
				if text, ok := extractClaudeRecoveryText(msg.Content); ok && text != "" {
					text = strings.TrimSpace(text)
					if text != "" && !strings.HasPrefix(text, "<local-command") && !strings.HasPrefix(text, "<command-name") && !strings.HasPrefix(text, "<system-") {
						firstTitle = text
					}
				}
			}
		}
		if firstCWD != "" && firstTitle != "" {
			break
		}
	}
	if firstCWD == "" {
		return nil, nil
	}
	if firstTitle == "" {
		firstTitle = sessionID
	}

	var lastText string
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		var ev scanClaudeRecoveryEvent
		if json.Unmarshal([]byte(line), &ev) != nil {
			continue
		}
		if ev.Type != "assistant" {
			continue
		}
		var msg scanClaudeRecoveryMessage
		if json.Unmarshal(ev.Message, &msg) == nil {
			if text, ok := extractClaudeRecoveryText(msg.Content); ok && text != "" {
				lastText = text
				break
			}
		}
	}

	return &recoverySession{
		SessionID:    sessionID,
		AgentType:    "claude",
		Title:        truncateRecoveryText(firstTitle, 200),
		Preview:      truncateRecoveryText(cleanClaudeRecoveryReply(lastText), 200),
		UpdatedAt:    fileInfo.ModTime().UTC().Format(time.RFC3339),
		MessageCount: messageCount,
		CWD:          firstCWD,
	}, nil
}

func extractClaudeRecoveryText(raw json.RawMessage) (string, bool) {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s, true
	}
	var blocks []scanClaudeRecoveryContentBlock
	if json.Unmarshal(raw, &blocks) != nil {
		return "", false
	}
	for _, block := range blocks {
		if block.Type == "text" && block.Text != "" {
			return block.Text, true
		}
	}
	return "", false
}

func cleanClaudeRecoveryReply(s string) string {
	for _, tag := range []string{"thinking", "tool_calls", "tool_result", "antml:function_calls", "antml:function_results"} {
		for {
			open := "<" + tag + ">"
			closeTag := "</" + tag + ">"
			i := strings.Index(s, open)
			if i < 0 {
				break
			}
			j := strings.Index(s[i:], closeTag)
			if j < 0 {
				break
			}
			s = s[:i] + s[i+j+len(closeTag):]
		}
	}
	return strings.TrimSpace(s)
}

type codexRecoverySource struct{}

func (codexRecoverySource) AgentType() string { return "codex" }

func (codexRecoverySource) List(projectCWD string, managedIDs map[string]bool) ([]recoverySession, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, nil
	}
	indexEntries := make(map[string]struct {
		Title     string `json:"thread_name"`
		UpdatedAt string `json:"updated_at"`
	})
	indexPath := filepath.Join(home, ".codex", "session_index.jsonl")
	indexData, _ := os.ReadFile(indexPath)
	for _, line := range strings.Split(string(indexData), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var entry struct {
			ID        string `json:"id"`
			Title     string `json:"thread_name"`
			UpdatedAt string `json:"updated_at"`
		}
		if json.Unmarshal([]byte(line), &entry) == nil && entry.ID != "" {
			indexEntries[entry.ID] = struct {
				Title     string `json:"thread_name"`
				UpdatedAt string `json:"updated_at"`
			}{Title: entry.Title, UpdatedAt: entry.UpdatedAt}
		}
	}
	normalizedCWD := normalizeRecoveryCWD(projectCWD)
	var results []recoverySession
	sessionsDir := filepath.Join(home, ".codex", "sessions")
	_ = filepath.WalkDir(sessionsDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(d.Name(), ".jsonl") {
			return nil
		}
		sessionID, cwd := readCodexRecoverySessionMeta(path)
		if sessionID == "" || managedIDs[sessionID] || normalizeRecoveryCWD(cwd) != normalizedCWD {
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
		results = append(results, recoverySession{
			SessionID: sessionID,
			AgentType: "codex",
			Title:     title,
			Preview:   readCodexRecoverySessionPreview(path, title),
			UpdatedAt: updatedAt,
			CWD:       cwd,
		})
		return nil
	})
	return results, nil
}

func (s codexRecoverySource) Find(projectCWD, sessionID string, managedIDs map[string]bool) (*recoverySession, error) {
	items, err := s.List(projectCWD, managedIDs)
	if err != nil {
		return nil, err
	}
	return findRecoverySession(items, sessionID), nil
}

func readCodexRecoverySessionMeta(path string) (sessionID, cwd string) {
	f, err := os.Open(path)
	if err != nil {
		return "", ""
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	buf := make([]byte, 0, 1024*64)
	scanner.Buffer(buf, 1024*1024*8)
	if !scanner.Scan() {
		return "", ""
	}
	var ev struct {
		Payload struct {
			ID  string `json:"id"`
			CWD string `json:"cwd"`
		} `json:"payload"`
	}
	if json.Unmarshal(scanner.Bytes(), &ev) != nil {
		return "", ""
	}
	return ev.Payload.ID, ev.Payload.CWD
}

func readCodexRecoverySessionPreview(path, fallback string) string {
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
			return truncateRecoveryText(strings.TrimSpace(msg.Payload.LastAgentMessage), 200)
		}
	}
	return fallback
}

type codexappRecoverySource struct{}

func (codexappRecoverySource) AgentType() string { return "codexapp" }

func (codexappRecoverySource) List(projectCWD string, managedIDs map[string]bool) ([]recoverySession, error) {
	items, err := codexRecoverySource{}.List(projectCWD, managedIDs)
	if err != nil {
		return nil, err
	}
	for i := range items {
		items[i].AgentType = "codexapp"
	}
	return items, nil
}

func (s codexappRecoverySource) Find(projectCWD, sessionID string, managedIDs map[string]bool) (*recoverySession, error) {
	items, err := s.List(projectCWD, managedIDs)
	if err != nil {
		return nil, err
	}
	return findRecoverySession(items, sessionID), nil
}

type copilotRecoverySource struct{}

func (copilotRecoverySource) AgentType() string { return "copilot" }

func (copilotRecoverySource) List(projectCWD string, managedIDs map[string]bool) ([]recoverySession, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, nil
	}
	stateDir := filepath.Join(home, ".copilot", "session-state")
	entries, err := os.ReadDir(stateDir)
	if err != nil {
		return nil, nil
	}
	normalizedCWD := normalizeRecoveryCWD(projectCWD)
	var results []recoverySession
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		sessionID := entry.Name()
		if managedIDs[sessionID] {
			continue
		}
		info := readCopilotRecoverySession(filepath.Join(stateDir, sessionID, "events.jsonl"), normalizedCWD, sessionID)
		if info != nil {
			results = append(results, *info)
		}
	}
	return results, nil
}

func (s copilotRecoverySource) Find(projectCWD, sessionID string, managedIDs map[string]bool) (*recoverySession, error) {
	items, err := s.List(projectCWD, managedIDs)
	if err != nil {
		return nil, err
	}
	return findRecoverySession(items, sessionID), nil
}

type copilotRecoveryEvent struct {
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

func readCopilotRecoverySession(eventsPath, normalizedCWD, fallbackSessionID string) *recoverySession {
	data, err := os.ReadFile(eventsPath)
	if err != nil || len(data) == 0 {
		return nil
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) == 0 {
		return nil
	}
	var start copilotRecoveryEvent
	if json.Unmarshal([]byte(lines[0]), &start) != nil || start.Type != "session.start" {
		return nil
	}
	if normalizeRecoveryCWD(start.Data.Context.CWD) != normalizedCWD {
		return nil
	}
	sessionID := firstNonEmpty(start.Data.SessionID, fallbackSessionID)
	updatedAt := start.Data.StartTime
	if updatedAt == "" {
		if fi, err := os.Stat(eventsPath); err == nil {
			updatedAt = fi.ModTime().UTC().Format(time.RFC3339)
		}
	}
	title := sessionID
	preview := ""
	messageCount := 0
	for i := 1; i < len(lines); i++ {
		var ev copilotRecoveryEvent
		if json.Unmarshal([]byte(lines[i]), &ev) != nil {
			continue
		}
		if title == sessionID && ev.Type == "user.message" && ev.Data.Content != "" {
			title = firstNonEmpty(strings.TrimSpace(ev.Data.Content), sessionID)
		}
		switch ev.Type {
		case "user.message", "assistant.message":
			if content := strings.TrimSpace(ev.Data.Content); content != "" {
				preview = content
				messageCount++
			}
		}
	}
	if preview == "" {
		preview = title
	}
	return &recoverySession{
		SessionID:    sessionID,
		AgentType:    "copilot",
		Title:        title,
		Preview:      truncateRecoveryText(preview, 200),
		UpdatedAt:    updatedAt,
		MessageCount: messageCount,
		CWD:          start.Data.Context.CWD,
	}
}
