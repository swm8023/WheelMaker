package client

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	acp "github.com/swm8023/wheelmaker/internal/hub/acp"
	"github.com/swm8023/wheelmaker/internal/hub/im"
)

// SessionStatus defines the lifecycle state of a Session.
type SessionStatus int

const (
	// SessionActive means the session is accepting messages.
	SessionActive SessionStatus = iota
	// SessionSuspended means the session is idle but still in memory.
	SessionSuspended
	// SessionPersisted means the session has been saved to disk and released from memory.
	SessionPersisted
)

// SessionAgentState holds per-agent metadata within one Session.
// Preserved across agent switches so that switching back restores previous state.
type SessionAgentState struct {
	ACPSessionID  string                 `json:"acpSessionId,omitempty"`
	ConfigOptions []acp.ConfigOption     `json:"configOptions,omitempty"`
	Commands      []acp.AvailableCommand `json:"commands,omitempty"`
	Title         string                 `json:"title,omitempty"`
	UpdatedAt     string                 `json:"updatedAt,omitempty"`
}

// Session is the business session object that owns ACP session state,
// prompt lifecycle, terminal management, and callback handling.
// In Phase 1 a Client holds exactly one Session.
type Session struct {
	ID     string
	Status SessionStatus

	// conn bundles the active agent subprocess and forwarder.
	conn *agentConn

	// Per-agent state indexed by agent name.
	agents map[string]*SessionAgentState

	// Runtime ACP session state (moved from Client.session / Client.sessionMeta / Client.initMeta).
	acpSessionID string
	ready        bool
	initializing bool
	lastReply    string
	replayH      func(acp.SessionUpdateParams)
	initMeta     clientInitMeta
	sessionMeta  clientSessionMeta

	prompt   promptState
	initCond *sync.Cond

	terminals  *terminalManager
	permRouter *permissionRouter

	// Back-references to Client-owned resources needed by Session methods.
	cwd              string
	yolo             bool
	debugLog         io.Writer
	registry         *agentRegistry
	store            Store
	state            *ProjectState
	imBridge         *im.ImAdapter
	imBlockedUpdates map[string]struct{}

	createdAt    time.Time
	lastActiveAt time.Time

	mu       sync.Mutex
	promptMu sync.Mutex
}

// newSession creates a Session with sensible defaults.
func newSession(id string, cwd string) *Session {
	s := &Session{
		ID:        id,
		Status:    SessionActive,
		agents:    make(map[string]*SessionAgentState),
		cwd:       cwd,
		createdAt: time.Now(),
		prompt: promptState{
			activeTCs: make(map[string]struct{}),
		},
		imBlockedUpdates: make(map[string]struct{}),
	}
	s.initCond = sync.NewCond(&s.mu)
	s.terminals = newTerminalManager()
	return s
}

// reply sends a text response to the active chat via the IM channel.
func (s *Session) reply(text string) {
	if s.imBridge != nil {
		chatID := s.imBridge.ActiveChatID()
		if chatID == "" {
			chatID = s.ID
		}
		_ = s.imBridge.SendSystem(chatID, text)
		return
	}
	fmt.Println(text)
}

// ensureForwarder connects the active agent and sets up the Forwarder if not already running.
func (s *Session) ensureForwarder(ctx context.Context) error {
	s.mu.Lock()
	if s.conn != nil && s.conn.forwarder != nil {
		s.mu.Unlock()
		return nil
	}
	if s.state == nil {
		s.mu.Unlock()
		return fmt.Errorf("state not loaded")
	}
	name := s.state.ActiveAgent
	if name == "" {
		name = defaultAgentName
	}
	dw := s.debugLog
	savedSID := ""
	if s.state.Agents != nil {
		if as := s.state.Agents[name]; as != nil && as.LastSessionID != "" {
			savedSID = as.LastSessionID
		}
	}
	s.mu.Unlock()

	fac := s.registry.get(name)
	if fac == nil {
		return fmt.Errorf("no agent registered for %q", name)
	}

	baseAgent := fac("", nil)
	conn, err := baseAgent.Connect(ctx)
	if err != nil {
		return fmt.Errorf("connect %q: %w", name, err)
	}
	if dw != nil {
		conn.SetDebugLogger(dw)
	}
	fwd := acp.NewForwarder(conn, nil)
	// SetCallbacks wired after Session implements acp.ClientCallbacks.
	// fwd.SetCallbacks(s)

	s.mu.Lock()
	if s.conn != nil && s.conn.forwarder != nil {
		s.mu.Unlock()
		_ = fwd.Close()
		return nil
	}
	s.conn = &agentConn{name: name, agent: baseAgent, forwarder: fwd}
	s.ready = false
	s.acpSessionID = savedSID
	s.mu.Unlock()
	return nil
}
