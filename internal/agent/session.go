package agent

import (
	"context"
	"fmt"

	"github.com/swm8023/wheelmaker/internal/agent/acp"
)

// ensureReady performs the ACP handshake if the agent is not yet ready:
//  1. Send "initialize" and store the agent capabilities.
//  2. Register the callback handler on conn.
//  3. If caps.LoadSession and a sessionID is stored, attempt session/load.
//  4. Otherwise, create a new session via session/new.
//
// Single-flight: if concurrent callers race here, only one performs the I/O;
// others wait on initCond and return immediately once ready is set.
func (a *Agent) ensureReady(ctx context.Context) error {
	a.mu.Lock()
	// Wait out any concurrent initialization in progress.
	for a.initializing {
		a.initCond.Wait() // atomically releases a.mu and waits; re-acquires on signal
	}
	if a.ready {
		a.mu.Unlock()
		return nil
	}
	// We are the initializer: claim the slot.
	a.initializing = true
	conn := a.conn
	savedSessionID := a.sessionID
	cwd := a.cwd
	mcpServers := a.mcpServers
	a.mu.Unlock()

	// notifyDone releases the initializing slot and wakes up any waiters.
	notifyDone := func() {
		a.mu.Lock()
		a.initializing = false
		a.mu.Unlock()
		a.initCond.Broadcast()
	}

	// Step 1: initialize handshake.
	var initResult acp.InitializeResult
	if err := conn.Send(ctx, "initialize", acp.InitializeParams{
		ProtocolVersion: "0.1",
		ClientCapabilities: acp.ClientCapabilities{
			FS: &acp.FSCapabilities{
				ReadTextFile:  true,
				WriteTextFile: true,
			},
			Terminal: true,
		},
		ClientInfo: &acp.AgentInfo{Name: "wheelmaker", Version: "0.1"},
	}, &initResult); err != nil {
		notifyDone()
		return fmt.Errorf("ensureReady: initialize: %w", err)
	}

	// Step 2: register the callback handler.
	conn.OnRequest(a.handleCallback)

	// Step 3: attempt session/load if possible.
	if savedSessionID != "" && initResult.AgentCapabilities.LoadSession {
		var loadResult acp.SessionLoadResult
		err := conn.Send(ctx, "session/load", acp.SessionLoadParams{
			SessionID:  savedSessionID,
			CWD:        cwd,
			MCPServers: mcpServers,
		}, &loadResult)
		if err == nil {
			a.mu.Lock()
			a.caps = initResult.AgentCapabilities
			// sessionID is already set (savedSessionID)
			a.ready = true
			a.initializing = false
			a.mu.Unlock()
			a.initCond.Broadcast()
			return nil
		}
		// session/load failed — fall through to session/new.
	}

	// Step 4: create a new session.
	var newResult acp.SessionNewResult
	if err := conn.Send(ctx, "session/new", acp.SessionNewParams{
		CWD:        cwd,
		MCPServers: mcpServers,
	}, &newResult); err != nil {
		notifyDone()
		return fmt.Errorf("ensureReady: session/new: %w", err)
	}

	a.mu.Lock()
	a.caps = initResult.AgentCapabilities
	a.sessionID = newResult.SessionID
	a.ready = true
	a.initializing = false
	a.mu.Unlock()
	a.initCond.Broadcast()
	return nil
}

