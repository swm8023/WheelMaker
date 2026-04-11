package sessionview

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/swm8023/wheelmaker/internal/hub/client"
)

type aggregateState struct {
	message client.SessionMessageRecord
}

type aggregator struct {
	mu       sync.Mutex
	nextID   int64
	active   map[string]*aggregateState
}

func newAggregator() *aggregator {
	return &aggregator{active: map[string]*aggregateState{}}
}

func aggregateKey(sessionID, kind string) string {
	return strings.TrimSpace(sessionID) + ":" + strings.TrimSpace(kind)
}

func (a *aggregator) appendChunk(projectName string, event client.SessionViewEvent) client.SessionMessageRecord {
	a.mu.Lock()
	defer a.mu.Unlock()

	key := strings.TrimSpace(event.AggregateKey)
	if key == "" {
		key = aggregateKey(event.SessionID, event.Kind)
	}
	state := a.active[key]
	now := event.UpdatedAt
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if state == nil {
		a.nextID += 1
		messageID := fmt.Sprintf("msg-agg-%d", a.nextID)
		state = &aggregateState{message: client.SessionMessageRecord{
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
	return cloneMessage(state.message)
}

func (a *aggregator) flushSession(sessionID string) []client.SessionMessageRecord {
	a.mu.Lock()
	defer a.mu.Unlock()
	var out []client.SessionMessageRecord
	for key, state := range a.active {
		if state == nil || strings.TrimSpace(state.message.SessionID) != strings.TrimSpace(sessionID) {
			continue
		}
		state.message.Status = "done"
		state.message.UpdatedAt = time.Now().UTC()
		out = append(out, cloneMessage(state.message))
		delete(a.active, key)
	}
	return out
}

func cloneMessage(in client.SessionMessageRecord) client.SessionMessageRecord {
	return in
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}