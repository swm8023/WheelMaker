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
// A Client holds multiple Sessions, routed by IM routeKey.
type Session struct {
	ID     string
	Status SessionStatus

	// instance is the AgentInstance bound to this Session.
	// Created lazily by ensureInstance(). Nil means no agent connected yet.
	instance *AgentInstance

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

// ensureInstance connects the active agent via AgentFactory and sets up the
// AgentInstance if not already running. Connect is executed outside s.mu.
func (s *Session) ensureInstance(ctx context.Context) error {
	s.mu.Lock()
	if s.instance != nil {
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

	fac := s.registry.getV2(name)
	if fac == nil {
		return fmt.Errorf("no agent registered for %q", name)
	}

	inst, err := fac.CreateInstance(ctx, s, dw)
	if err != nil {
		return err
	}

	s.mu.Lock()
	if s.instance != nil {
		s.mu.Unlock()
		_ = inst.Close()
		return nil
	}
	s.instance = inst
	s.ready = false
	s.acpSessionID = savedSID
	s.mu.Unlock()
	return nil
}
