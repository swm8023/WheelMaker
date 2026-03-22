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
	if c.conn != nil && c.conn.forwarder != nil {
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
	dw := c.debugLog
	savedSID := ""
	if c.state.Agents != nil {
		if as := c.state.Agents[name]; as != nil && as.LastSessionID != "" {
			savedSID = as.LastSessionID
		}
	}
	c.mu.Unlock()

	fac := c.registry.get(name)
	if fac == nil {
		return fmt.Errorf("no agent registered for %q", name)
	}

	baseAgent := fac("", nil)
	conn, err := baseAgent.Connect(ctx)
	if err != nil {
		return fmt.Errorf("connect %q: %w", name, err)
	}
	if debugWriter := c.composeDebugWriter(name, dw); debugWriter != nil {
		conn.SetDebugLogger(debugWriter)
	}
	fwd := acp.NewForwarder(conn, nil)
	fwd.SetCallbacks(c)

	c.mu.Lock()
	if c.conn != nil && c.conn.forwarder != nil {
		c.mu.Unlock()
		_ = fwd.Close()
		return nil
	}
	c.conn = &agentConn{name: name, agent: baseAgent, forwarder: fwd}
	c.session.ready = false
	c.session.id = savedSID
	c.mu.Unlock()
	return nil
}

// switchAgent cancels any in-progress prompt, waits for it to finish via
// promptMu, connects a new agent binary, and replaces the forwarder.
func (c *Client) switchAgent(ctx context.Context, chatID, name string, mode SwitchMode) error {
	fac := c.registry.get(name)
	if fac == nil {
		return fmt.Errorf("unknown agent: %q (registered: %v)", name, c.registry.names())
	}

	// Cancel in-progress prompt, wait for handlePrompt to release promptMu.
	_ = c.cancelPrompt()
	c.promptMu.Lock()
	defer c.promptMu.Unlock()
	// Belt-and-suspenders: drain any channel published between cancelPrompt and promptMu.Lock.
	c.mu.Lock()
	promptCh := c.prompt.currentCh
	c.mu.Unlock()
	if promptCh != nil {
		for range promptCh {
		}
		c.mu.Lock()
		c.prompt.currentCh = nil
		c.mu.Unlock()
	}

	// Capture outgoing state.
	c.mu.Lock()
	oldConn := c.conn
	savedLastReply := c.session.lastReply
	dw := c.debugLog
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
	c.mu.Unlock()

	// Connect new agent.
	baseAgent := fac("", nil)
	newConn, err := baseAgent.Connect(ctx)
	if err != nil {
		return fmt.Errorf("connect %q: %w", name, err)
	}
	if debugWriter := c.composeDebugWriter(name, dw); debugWriter != nil {
		newConn.SetDebugLogger(debugWriter)
	}
	newFwd := acp.NewForwarder(newConn, nil)
	newFwd.SetCallbacks(c)

	// Replace connection atomically; kill terminals; close old conn.
	c.mu.Lock()
	c.terminals.KillAll()
	c.conn = &agentConn{name: name, agent: baseAgent, forwarder: newFwd}
	c.session.initializing = false
	c.prompt.updatesCh = nil
	c.initMeta = clientInitMeta{}
	c.resetSessionFields(savedSID, nil) // sets ready=true — override below:
	c.session.ready = false
	c.mu.Unlock()
	c.initCond.Broadcast() // MUST be outside c.mu — never inside the lock

	if oldConn != nil {
		_ = oldConn.close()
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
