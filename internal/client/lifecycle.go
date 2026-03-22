package client

import (
	"context"
	"fmt"
	"log"

	acp "github.com/swm8023/wheelmaker/internal/acp"
)

// ensureForwarder connects the active agent and sets up the Forwarder if not already running.
// Connect() is intentionally executed outside c.mu to avoid blocking unrelated operations.
func (c *Client) ensureForwarder(ctx context.Context) error {
	c.mu.Lock()
	if c.forwarder != nil {
		c.mu.Unlock()
		return nil
	}
	if c.state == nil {
		c.mu.Unlock()
		return fmt.Errorf("state not loaded")
	}
	name := c.state.ActiveAgent
	if name == "" {
		name = defaultAgentName
	}
	fac := c.agentFacs[name]
	if fac == nil {
		c.mu.Unlock()
		return fmt.Errorf("no agent registered for %q", name)
	}
	dw := c.debugLog
	debugEnabled := c.debugEnabled
	savedSID := ""
	if c.state.Agents != nil {
		if as := c.state.Agents[name]; as != nil && as.LastSessionID != "" {
			savedSID = as.LastSessionID
		}
	}
	c.mu.Unlock()

	baseAgent := fac("", nil)
	conn, err := baseAgent.Connect(ctx)
	if err != nil {
		return fmt.Errorf("connect %q: %w", name, err)
	}
	if debugWriter := c.composeDebugWriter(name, dw, debugEnabled); debugWriter != nil {
		conn.SetDebugLogger(debugWriter)
	}
	fwd := acp.NewForwarder(conn, nil)
	fwd.SetCallbacks(c)

	c.mu.Lock()
	if c.forwarder != nil {
		c.mu.Unlock()
		_ = fwd.Close()
		return nil
	}
	c.forwarder = fwd
	c.currentAgent = baseAgent
	c.currentAgentName = name
	c.ready = false
	c.sessionID = savedSID
	c.mu.Unlock()
	return nil
}

// switchAgent cancels any in-progress prompt, waits for it to finish via
// promptMu, connects a new agent binary, and replaces the forwarder.
func (c *Client) switchAgent(ctx context.Context, chatID, name string, mode SwitchMode) error {
	c.mu.Lock()
	fac := c.agentFacs[name]
	c.mu.Unlock()
	if fac == nil {
		return fmt.Errorf("unknown agent: %q (registered: %v)", name, c.registeredAgentNames())
	}

	// Cancel in-progress prompt, wait for handlePrompt to release promptMu.
	_ = c.cancelPrompt()
	c.promptMu.Lock()
	defer c.promptMu.Unlock()
	// Belt-and-suspenders: drain any channel published between cancelPrompt and promptMu.Lock.
	c.mu.Lock()
	promptCh := c.currentPromptCh
	c.mu.Unlock()
	if promptCh != nil {
		for range promptCh {
		}
		c.mu.Lock()
		c.currentPromptCh = nil
		c.mu.Unlock()
	}

	// Capture outgoing state.
	c.mu.Lock()
	oldFwd := c.forwarder
	savedLastReply := c.lastReply
	c.mu.Unlock()
	c.persistMeta() // save outgoing agent state before reset

	// Read saved session ID for incoming agent.
	c.mu.Lock()
	var savedSID string
	if c.state != nil && c.state.Agents != nil {
		if as := c.state.Agents[name]; as != nil {
			savedSID = as.LastSessionID
		}
	}
	dw := c.debugLog
	debugEnabled := c.debugEnabled
	c.mu.Unlock()

	// Connect new agent.
	baseAgent := fac("", nil)
	newConn, err := baseAgent.Connect(ctx)
	if err != nil {
		return fmt.Errorf("connect %q: %w", name, err)
	}
	if debugWriter := c.composeDebugWriter(name, dw, debugEnabled); debugWriter != nil {
		newConn.SetDebugLogger(debugWriter)
	}
	newFwd := acp.NewForwarder(newConn, nil)
	newFwd.SetCallbacks(c)

	// Replace forwarder atomically; kill terminals; close old conn.
	c.mu.Lock()
	c.terminals.KillAll()
	c.forwarder = newFwd
	c.currentAgent = baseAgent
	c.currentAgentName = name
	c.ready = false
	c.initializing = false
	c.sessionID = savedSID
	c.lastReply = ""
	c.loadHistory = nil
	c.activeToolCalls = make(map[string]struct{})
	c.promptUpdatesCh = nil
	c.initMeta = clientInitMeta{}
	c.sessionMeta = clientSessionMeta{}
	c.mu.Unlock()
	c.initCond.Broadcast()

	if oldFwd != nil {
		_ = oldFwd.Close()
	}

	// SwitchWithContext — bootstrap new session with previous reply.
	if mode == SwitchWithContext && savedLastReply != "" {
		ch, err := c.promptStream(ctx, "[context] "+savedLastReply)
		if err != nil {
			log.Printf("client: SwitchWithContext bootstrap prompt failed: %v", err)
		} else {
			for u := range ch {
				if u.Err != nil {
					log.Printf("client: SwitchWithContext bootstrap prompt failed: %v", u.Err)
				}
			}
		}
		c.persistMeta()
	}

	// Update ActiveAgent and save.
	c.mu.Lock()
	if c.state != nil {
		c.state.ActiveAgent = name
	}
	s := c.state
	c.mu.Unlock()
	if s != nil {
		_ = c.store.Save(s)
	}

	c.reply(chatID, fmt.Sprintf("Switched to agent: %s", name))
	snap := c.sessionConfigSnapshot()
	if snap.Mode != "" || snap.Model != "" {
		c.reply(chatID, fmt.Sprintf("Session ready: mode=%s model=%s",
			renderUnknown(snap.Mode), renderUnknown(snap.Model)))
	}
	return nil
}

// registeredAgentNames returns all registered agent names (for error messages).
func (c *Client) registeredAgentNames() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	names := make([]string, 0, len(c.agentFacs))
	for n := range c.agentFacs {
		names = append(names, n)
	}
	return names
}
