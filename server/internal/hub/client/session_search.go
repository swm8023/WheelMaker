package client

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	acp "github.com/swm8023/wheelmaker/internal/protocol"
)

const (
	sessionSearchIdleTTL      = 10 * time.Minute
	sessionSearchMaxTasks     = 8
	sessionSearchMaxQuerySize = 200
	sessionSearchIdleForCap   = 30 * time.Second
)

type sessionSearchRequest struct {
	Action   string `json:"action"`
	SearchID string `json:"searchId"`
	Query    string `json:"query,omitempty"`
}

type sessionSearchResponse struct {
	SearchID string                `json:"searchId"`
	Done     bool                  `json:"done"`
	Results  []sessionSearchResult `json:"results,omitempty"`
	Errors   []sessionSearchError  `json:"errors,omitempty"`
}

type sessionSearchResult struct {
	ProjectID string `json:"projectId"`
	SessionID string `json:"sessionId"`
	Source    string `json:"source"`
	TurnIndex int64  `json:"turnIndex,omitempty"`
}

type sessionSearchError struct {
	ProjectID string `json:"projectId"`
	SessionID string `json:"sessionId,omitempty"`
	Message   string `json:"message"`
}

type sessionSearchSnapshot struct {
	SessionID       string
	Title           string
	LatestTurnIndex int64
}

type sessionSearchTask struct {
	searchID    string
	projectID   string
	query       string
	queryFold   string
	sessions    []sessionSearchSnapshot
	ctx         context.Context
	cancel      context.CancelFunc
	results     []sessionSearchResult
	errors      []sessionSearchError
	done        bool
	lastTouched time.Time
	startedAt   time.Time
}

type sessionSearchManager struct {
	client *Client
	now    func() time.Time

	mu    sync.Mutex
	tasks map[string]*sessionSearchTask
}

func newSessionSearchManager(client *Client) *sessionSearchManager {
	return &sessionSearchManager{
		client: client,
		now: func() time.Time {
			return time.Now().UTC()
		},
		tasks: map[string]*sessionSearchTask{},
	}
}

func (m *sessionSearchManager) Close() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, task := range m.tasks {
		task.cancel()
		delete(m.tasks, id)
	}
}

func (m *sessionSearchManager) Handle(ctx context.Context, projectID string, payload json.RawMessage) (sessionSearchResponse, error) {
	var req sessionSearchRequest
	if err := decodeSessionRequestPayload(payload, &req); err != nil {
		return sessionSearchResponse{}, fmt.Errorf("invalid session.search payload: %w", err)
	}
	req.Action = strings.TrimSpace(req.Action)
	req.SearchID = strings.TrimSpace(req.SearchID)
	if req.SearchID == "" {
		return sessionSearchResponse{}, fmt.Errorf("searchId is required")
	}
	projectID = strings.TrimSpace(projectID)
	if projectID == "" && m != nil && m.client != nil {
		projectID = m.client.projectName
	}

	switch req.Action {
	case "start":
		return m.start(ctx, projectID, req.SearchID, req.Query)
	case "query":
		return m.query(req.SearchID)
	case "cancel":
		return m.cancel(req.SearchID), nil
	default:
		return sessionSearchResponse{}, fmt.Errorf("unsupported session.search action: %s", req.Action)
	}
}

func (m *sessionSearchManager) start(ctx context.Context, projectID, searchID, query string) (sessionSearchResponse, error) {
	if m == nil || m.client == nil || m.client.sessionRecorder == nil {
		return sessionSearchResponse{}, fmt.Errorf("session search manager is required")
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return sessionSearchResponse{}, fmt.Errorf("query is required")
	}
	if len([]rune(query)) > sessionSearchMaxQuerySize {
		return sessionSearchResponse{}, fmt.Errorf("query is too long")
	}
	sessions, err := m.client.sessionRecorder.ListSessionViews(ctx)
	if err != nil {
		return sessionSearchResponse{}, err
	}
	snapshot := make([]sessionSearchSnapshot, 0, len(sessions))
	for _, session := range sessions {
		sessionID := strings.TrimSpace(session.SessionID)
		if sessionID == "" {
			continue
		}
		snapshot = append(snapshot, sessionSearchSnapshot{
			SessionID:       sessionID,
			Title:           session.Title,
			LatestTurnIndex: session.LatestTurnIndex,
		})
	}

	taskCtx, cancel := context.WithCancel(context.Background())
	now := m.now()
	task := &sessionSearchTask{
		searchID:    searchID,
		projectID:   projectID,
		query:       query,
		queryFold:   strings.ToLower(query),
		sessions:    snapshot,
		ctx:         taskCtx,
		cancel:      cancel,
		results:     []sessionSearchResult{},
		errors:      []sessionSearchError{},
		lastTouched: now,
		startedAt:   now,
	}

	m.mu.Lock()
	m.cleanupLocked(now)
	if existing := m.tasks[searchID]; existing != nil {
		existing.cancel()
		delete(m.tasks, searchID)
	}
	if err := m.ensureTaskCapacityLocked(now); err != nil {
		m.mu.Unlock()
		cancel()
		return sessionSearchResponse{}, err
	}
	m.tasks[searchID] = task
	m.mu.Unlock()

	go m.run(task)
	return sessionSearchResponse{SearchID: searchID, Done: false}, nil
}

func (m *sessionSearchManager) query(searchID string) (sessionSearchResponse, error) {
	if m == nil {
		return sessionSearchResponse{}, fmt.Errorf("session search manager is required")
	}
	now := m.now()
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cleanupLocked(now)
	task := m.tasks[searchID]
	if task == nil {
		return sessionSearchResponse{}, fmt.Errorf("session search not found or expired: %s", searchID)
	}
	task.lastTouched = now
	return sessionSearchResponse{
		SearchID: task.searchID,
		Done:     task.done,
		Results:  append([]sessionSearchResult(nil), task.results...),
		Errors:   append([]sessionSearchError(nil), task.errors...),
	}, nil
}

func (m *sessionSearchManager) cancel(searchID string) sessionSearchResponse {
	if m == nil {
		return sessionSearchResponse{SearchID: searchID, Done: true}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if task := m.tasks[searchID]; task != nil {
		task.cancel()
		delete(m.tasks, searchID)
	}
	return sessionSearchResponse{SearchID: searchID, Done: true}
}

func (m *sessionSearchManager) run(task *sessionSearchTask) {
	defer m.markDone(task.searchID)
	for _, session := range task.sessions {
		if err := task.ctx.Err(); err != nil {
			return
		}
		if sessionSearchContains(resolveSessionSearchTitle(session.Title), task.queryFold) {
			m.appendResult(task.searchID, sessionSearchResult{
				ProjectID: task.projectID,
				SessionID: session.SessionID,
				Source:    "title",
			})
			continue
		}
		turnIndex, matched, err := m.searchPromptTurns(task.ctx, session.SessionID, session.LatestTurnIndex, task.queryFold)
		if err != nil {
			if task.ctx.Err() != nil {
				return
			}
			m.appendError(task.searchID, sessionSearchError{
				ProjectID: task.projectID,
				SessionID: session.SessionID,
				Message:   err.Error(),
			})
			continue
		}
		if matched {
			m.appendResult(task.searchID, sessionSearchResult{
				ProjectID: task.projectID,
				SessionID: session.SessionID,
				Source:    "prompt",
				TurnIndex: turnIndex,
			})
		}
	}
}

func (m *sessionSearchManager) searchPromptTurns(ctx context.Context, sessionID string, latestTurnIndex int64, queryFold string) (int64, bool, error) {
	if m == nil || m.client == nil || m.client.sessionRecorder == nil {
		return 0, false, fmt.Errorf("session recorder is required")
	}
	recorder := m.client.sessionRecorder
	if recorder.turnStore != nil {
		var matchedTurn int64
		err := recorder.turnStore.scanTurnsNewestFirst(ctx, recorder.projectName, sessionID, latestTurnIndex, func(turn sessionViewTurn) (bool, error) {
			if sessionSearchContains(sessionSearchTurnVisibleText(turn.Content), queryFold) {
				matchedTurn = turn.TurnIndex
				return true, nil
			}
			return false, nil
		})
		if err != nil {
			return 0, false, err
		}
		return matchedTurn, matchedTurn > 0, nil
	}

	_, turns, err := recorder.ReadSessionTurns(ctx, sessionID, 0)
	if err != nil {
		return 0, false, err
	}
	for index := len(turns) - 1; index >= 0; index-- {
		turn := turns[index]
		if sessionSearchContains(sessionSearchTurnVisibleText(turn.Content), queryFold) {
			return turn.TurnIndex, true, nil
		}
	}
	return 0, false, nil
}

func (m *sessionSearchManager) appendResult(searchID string, result sessionSearchResult) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if task := m.tasks[searchID]; task != nil {
		task.results = append(task.results, result)
	}
}

func (m *sessionSearchManager) appendError(searchID string, searchErr sessionSearchError) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if task := m.tasks[searchID]; task != nil {
		task.errors = append(task.errors, searchErr)
	}
}

func (m *sessionSearchManager) markDone(searchID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if task := m.tasks[searchID]; task != nil {
		task.done = true
	}
}

func (m *sessionSearchManager) cleanupLocked(now time.Time) {
	for id, task := range m.tasks {
		if now.Sub(task.lastTouched) >= sessionSearchIdleTTL {
			task.cancel()
			delete(m.tasks, id)
		}
	}
}

func (m *sessionSearchManager) ensureTaskCapacityLocked(now time.Time) error {
	if len(m.tasks) < sessionSearchMaxTasks {
		return nil
	}
	if id, ok := oldestSearchTaskID(m.tasks, func(task *sessionSearchTask) bool {
		return task.done
	}); ok {
		m.tasks[id].cancel()
		delete(m.tasks, id)
		return nil
	}
	if id, ok := oldestSearchTaskID(m.tasks, func(task *sessionSearchTask) bool {
		return now.Sub(task.lastTouched) >= sessionSearchIdleForCap
	}); ok {
		m.tasks[id].cancel()
		delete(m.tasks, id)
		return nil
	}
	return fmt.Errorf("too many active session searches")
}

func oldestSearchTaskID(tasks map[string]*sessionSearchTask, include func(*sessionSearchTask) bool) (string, bool) {
	type candidate struct {
		id          string
		lastTouched time.Time
	}
	candidates := []candidate{}
	for id, task := range tasks {
		if include(task) {
			candidates = append(candidates, candidate{id: id, lastTouched: task.lastTouched})
		}
	}
	if len(candidates) == 0 {
		return "", false
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].lastTouched.Before(candidates[j].lastTouched)
	})
	return candidates[0].id, true
}

func sessionSearchContains(text, queryFold string) bool {
	text = strings.TrimSpace(text)
	if text == "" || queryFold == "" {
		return false
	}
	return strings.Contains(strings.ToLower(text), queryFold)
}

func resolveSessionSearchTitle(rawTitle string) string {
	rawTitle = strings.TrimSpace(rawTitle)
	if !strings.HasPrefix(rawTitle, "{") {
		return rawTitle
	}
	var facts sessionTitleFacts
	if err := json.Unmarshal([]byte(rawTitle), &facts); err != nil {
		return rawTitle
	}
	first := strings.TrimSpace(facts.First)
	last := strings.TrimSpace(facts.Last)
	manual := strings.TrimSpace(facts.Manual)
	if manual != "" {
		return manual
	}
	if first != "" {
		return first
	}
	if last != "" {
		return last
	}
	return rawTitle
}

func sessionSearchTurnVisibleText(content string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}
	var turn acp.IMTurnMessage
	if err := json.Unmarshal([]byte(content), &turn); err != nil {
		return ""
	}
	method := strings.TrimSpace(turn.Method)
	switch method {
	case acp.IMMethodPromptRequest:
		var payload acp.IMPromptRequest
		if err := json.Unmarshal(turn.Param, &payload); err != nil {
			return ""
		}
		return sessionSearchContentBlockText(payload.ContentBlocks)
	case acp.IMMethodPromptDone:
		var payload acp.IMPromptResult
		if err := json.Unmarshal(turn.Param, &payload); err != nil {
			return ""
		}
		stopReason := strings.ToLower(strings.TrimSpace(payload.StopReason))
		switch stopReason {
		case "cancelled", "canceled", "interrupted", "failed", "error":
			return strings.Join(nonEmptySessionSearchParts(payload.StopReason, payload.Message), "\n")
		default:
			return strings.TrimSpace(payload.Message)
		}
	case acp.IMMethodAgentMessage, acp.IMMethodAgentThought, acp.SessionUpdateUserMessageChunk, acp.IMMethodSystem:
		var payload acp.IMTextResult
		if err := json.Unmarshal(turn.Param, &payload); err != nil {
			return ""
		}
		return strings.TrimSpace(payload.Text)
	case acp.IMMethodToolCall:
		return ""
	case acp.IMMethodAgentPlan:
		var payload []acp.IMPlanResult
		if err := json.Unmarshal(turn.Param, &payload); err != nil {
			return ""
		}
		parts := make([]string, 0, len(payload)*2)
		for _, entry := range payload {
			parts = append(parts, strings.TrimSpace(entry.Content), strings.TrimSpace(entry.Status))
		}
		return strings.Join(nonEmptySessionSearchParts(parts...), "\n")
	default:
		return sessionSearchGenericVisibleText(turn.Param)
	}
}

func sessionSearchContentBlockText(blocks []acp.ContentBlock) string {
	parts := make([]string, 0, len(blocks))
	for _, block := range blocks {
		if strings.TrimSpace(block.Type) == acp.ContentBlockTypeText {
			parts = append(parts, strings.TrimSpace(block.Text))
		}
	}
	return strings.Join(nonEmptySessionSearchParts(parts...), "\n")
}

func sessionSearchGenericVisibleText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return ""
	}
	parts := []string{}
	var walk func(any)
	walk = func(input any) {
		switch typed := input.(type) {
		case map[string]any:
			for key, value := range typed {
				switch key {
				case "text", "output", "cmd", "content", "message", "status":
					if text, ok := value.(string); ok {
						parts = append(parts, strings.TrimSpace(text))
						continue
					}
				}
				if key == "contentBlocks" {
					if blocks, ok := value.([]any); ok {
						for _, block := range blocks {
							walk(block)
						}
					}
				}
			}
		case []any:
			for _, item := range typed {
				walk(item)
			}
		}
	}
	walk(value)
	return strings.Join(nonEmptySessionSearchParts(parts...), "\n")
}

func nonEmptySessionSearchParts(parts ...string) []string {
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
