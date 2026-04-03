package client

import (
	"context"
	"fmt"

	acp "github.com/swm8023/wheelmaker/internal/hub/acp"
	logger "github.com/swm8023/wheelmaker/internal/shared"
)

// switchAgent cancels any in-progress prompt, waits for it to finish via
// promptMu, connects a new agent binary, and replaces the forwarder.
func (s *Session) switchAgent(ctx context.Context, name string, mode SwitchMode) error {
	fac := s.registry.get(name)
	if fac == nil {
		return fmt.Errorf("unknown agent: %q (registered: %v)", name, s.registry.names())
	}

	// Cancel in-progress prompt, wait for handlePrompt to release promptMu.
	_ = s.cancelPrompt()
	s.promptMu.Lock()
	defer s.promptMu.Unlock()
	// Belt-and-suspenders: drain any channel published between cancelPrompt and promptMu.Lock.
	s.mu.Lock()
	promptCh := s.prompt.currentCh
	s.mu.Unlock()
	if promptCh != nil {
		for range promptCh {
		}
		s.mu.Lock()
		s.prompt.currentCh = nil
		s.mu.Unlock()
	}

	// Capture outgoing state.
	s.mu.Lock()
	oldConn := s.conn
	savedLastReply := s.lastReply
	dw := s.debugLog
	s.mu.Unlock()
	s.persistMeta() // save outgoing agent state before reset

	// Read saved session ID for incoming agent.
	s.mu.Lock()
	var savedSID string
	if s.state != nil && s.state.Agents != nil {
		if as := s.state.Agents[name]; as != nil {
			savedSID = as.LastSessionID
		}
	}
	s.mu.Unlock()

	// Connect new agent.
	baseAgent := fac("", nil)
	newConn, err := baseAgent.Connect(ctx)
	if err != nil {
		return fmt.Errorf("connect %q: %w", name, err)
	}
	if dw != nil {
		newConn.SetDebugLogger(dw)
	}
	newFwd := acp.NewForwarder(newConn, nil)
	newFwd.SetCallbacks(s)

	// Replace connection atomically; kill terminals; close old conn.
	s.mu.Lock()
	s.terminals.KillAll()
	s.conn = &agentConn{name: name, agent: baseAgent, forwarder: newFwd}
	s.initializing = false
	s.prompt.updatesCh = nil
	s.initMeta = clientInitMeta{}
	s.resetSessionFields(savedSID, nil) // sets ready=true — override below:
	s.ready = false
	s.mu.Unlock()
	s.initCond.Broadcast() // MUST be outside s.mu — never inside the lock

	if oldConn != nil {
		_ = oldConn.close()
	}

	// SwitchWithContext — bootstrap new session with previous reply.
	if mode == SwitchWithContext && savedLastReply != "" {
		ch, err := s.promptStream(ctx, "[context] "+savedLastReply)
		if err != nil {
			logger.Warn("client: SwitchWithContext bootstrap prompt failed: %v", err)
		} else {
			for u := range ch {
				if u.Err != nil {
					logger.Warn("client: SwitchWithContext bootstrap prompt failed: %v", u.Err)
				}
			}
		}
		s.persistMeta()
	}

	// Update ActiveAgent and save.
	s.mu.Lock()
	if s.state != nil {
		s.state.ActiveAgent = name
	}
	st := s.state
	s.mu.Unlock()
	if st != nil {
		_ = s.store.Save(st)
	}

	s.reply(fmt.Sprintf("Switched to agent: %s", name))
	snap := s.sessionConfigSnapshot()
	if snap.Mode != "" || snap.Model != "" {
		s.reply(fmt.Sprintf("Session ready: mode=%s model=%s",
			renderUnknown(snap.Mode), renderUnknown(snap.Model)))
	}
	return nil
}
