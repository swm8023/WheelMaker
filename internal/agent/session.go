package agent

import (
	"context"
	"fmt"
	"log"

	"github.com/swm8023/wheelmaker/internal/agent/acp"
)

// ensureReady performs the ACP handshake if the agent is not yet ready:
//  1. Register the callback handler on conn so requests arriving during handshake are handled.
//  2. Send "initialize" and store the agent capabilities.
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
	// The pre-seeded sessionID is intentionally preserved so that a transient
	// error (e.g. subprocess crash, network glitch) does not discard the only
	// in-memory copy of the persisted session ID. If a subsequent retry succeeds
	// it can still attempt session/load with the original ID; if session/load then
	// fails, ensureReady falls through to session/new as usual.
	notifyDone := func() {
		a.mu.Lock()
		a.initializing = false
		a.mu.Unlock()
		a.initCond.Broadcast()
	}

	// Step 1: register the callback handler before sending initialize so that
	// any ACP backend that fires requests during startup is handled correctly.
	conn.OnRequest(a.handleCallback)

	// Step 2: initialize handshake.
	clientCaps := acp.ClientCapabilities{
		FS: &acp.FSCapabilities{
			ReadTextFile:  true,
			WriteTextFile: true,
		},
		Terminal: true,
	}
	clientInfo := &acp.AgentInfo{Name: "wheelmaker", Version: "0.1"}
	const clientProtocolVersion = 1 // B3 fix: integer per spec (was string "0.1")

	var initResult acp.InitializeResult
	if err := conn.Send(ctx, "initialize", acp.InitializeParams{
		ProtocolVersion:    clientProtocolVersion,
		ClientCapabilities: clientCaps,
		ClientInfo:         clientInfo,
	}, &initResult); err != nil {
		notifyDone()
		return fmt.Errorf("ensureReady: initialize: %w", err)
	}

	newInitMeta := InitMeta{
		ProtocolVersion:       initResult.ProtocolVersion.String(),
		AgentCapabilities:     initResult.AgentCapabilities,
		AgentInfo:             initResult.AgentInfo,
		AuthMethods:           initResult.AuthMethods,
		ClientProtocolVersion: clientProtocolVersion,
		ClientCapabilities:    clientCaps,
		ClientInfo:            clientInfo,
	}

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
			a.initMeta = newInitMeta
			// sessionID is already set (savedSessionID)
			a.ready = true
			a.initializing = false
			a.mu.Unlock()
			a.initCond.Broadcast()
			log.Printf("[agent] connected: adapter=%s session=%s (resumed)", a.name, savedSessionID)
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
	a.initMeta = newInitMeta
	a.sessionID = newResult.SessionID
	a.sessionMeta = SessionMeta{
		Modes:         newResult.Modes,
		Models:        newResult.Models,
		ConfigOptions: newResult.ConfigOptions,
	}
	a.ready = true
	a.initializing = false
	a.mu.Unlock()
	a.initCond.Broadcast()

	modeID := ""
	if newResult.Modes != nil {
		modeID = newResult.Modes.CurrentModeID
	}
	modelID := ""
	if newResult.Models != nil {
		modelID = newResult.Models.CurrentModelID
	}
	log.Printf("[agent] connected: adapter=%s session=%s mode=%s model=%s",
		a.name, newResult.SessionID, modeID, modelID)
	return nil
}

