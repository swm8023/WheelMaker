package client

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

type sessionProjectionAggregateState struct {
	message SessionMessageRecord
}

type sessionProjectionAggregator struct {
	mu     sync.Mutex
	nextID int64
	active map[string]*sessionProjectionAggregateState
}

func newSessionProjectionAggregator() *sessionProjectionAggregator {
	return &sessionProjectionAggregator{active: map[string]*sessionProjectionAggregateState{}}
}

func sessionProjectionAggregateKey(sessionID, kind string) string {
	return strings.TrimSpace(sessionID) + ":" + strings.TrimSpace(kind)
}

func (a *sessionProjectionAggregator) appendChunk(projectName string, event SessionViewEvent) SessionMessageRecord {
	a.mu.Lock()
	defer a.mu.Unlock()

	key := strings.TrimSpace(event.AggregateKey)
	if key == "" {
		key = sessionProjectionAggregateKey(event.SessionID, event.Kind)
	}
	state := a.active[key]
	now := event.UpdatedAt
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if state == nil {
		a.nextID += 1
		messageID := fmt.Sprintf("msg-agg-%d", a.nextID)
		state = &sessionProjectionAggregateState{message: SessionMessageRecord{
			MessageID:     messageID,
			ProjectName:   projectName,
			SessionID:     strings.TrimSpace(event.SessionID),
			Role:          strings.TrimSpace(event.Role),
			Kind:          strings.TrimSpace(event.Kind),
			Status:        firstNonEmpty(strings.TrimSpace(event.Status), "streaming"),
			AggregateKey:  key,
			SourceChannel: strings.TrimSpace(event.SourceChannel),
			SourceChatID:  strings.TrimSpace(event.SourceChatID),
			RequestID:     event.RequestID,
			CreatedAt:     now,
			UpdatedAt:     now,
		}}
		a.active[key] = state
	}
	state.message.Body += event.Text
	state.message.UpdatedAt = now
	state.message.Status = firstNonEmpty(strings.TrimSpace(event.Status), state.message.Status)
	if state.message.Role == "" {
		state.message.Role = strings.TrimSpace(event.Role)
	}
	if state.message.Kind == "" {
		state.message.Kind = strings.TrimSpace(event.Kind)
	}
	return state.message
}

func (a *sessionProjectionAggregator) flushSession(sessionID string) []SessionMessageRecord {
	a.mu.Lock()
	defer a.mu.Unlock()
	var out []SessionMessageRecord
	for key, state := range a.active {
		if state == nil || strings.TrimSpace(state.message.SessionID) != strings.TrimSpace(sessionID) {
			continue
		}
		state.message.Status = "done"
		state.message.UpdatedAt = time.Now().UTC()
		out = append(out, state.message)
		delete(a.active, key)
	}
	return out
}
