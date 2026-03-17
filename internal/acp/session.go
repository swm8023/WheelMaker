package acp

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
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
	// any ACP agent that fires requests during startup is handled correctly.
	a.forwarder.OnRequest(a.handleCallback)

	// Step 2: initialize handshake.
	clientCaps := ClientCapabilities{
		FS: &FSCapabilities{
			ReadTextFile:  true,
			WriteTextFile: true,
		},
		Terminal: true,
	}
	clientInfo := &AgentInfo{Name: "wheelmaker", Version: "0.1"}
	const clientProtocolVersion = 1 // B3 fix: integer per spec (was string "0.1")

	var initResult InitializeResult
	if err := conn.Send(ctx, "initialize", InitializeParams{
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
		// Subscribe before sending session/load so that history-replay session/update
		// notifications (dispatched synchronously by readLoop before the RPC response)
		// are captured. By the time conn.Send returns, all replayed notifications are
		// already processed by this handler.
		var replayMu sync.Mutex
		var replay []Update
		replayMeta := SessionMeta{}
		cancelReplaySub := a.forwarder.Subscribe(func(n Notification) {
			if n.Method != "session/update" {
				return
			}
			normalized := n.Params
			var p SessionUpdateParams
			if err := json.Unmarshal(normalized, &p); err != nil || p.SessionID != savedSessionID {
				return
			}
			u := sessionUpdateToUpdate(p.Update, normalized)
			replayMu.Lock()
			replay = append(replay, u)
			switch p.Update.SessionUpdate {
			case "available_commands_update":
				if len(p.Update.AvailableCommands) > 0 {
					replayMeta.AvailableCommands = p.Update.AvailableCommands
				}
			case "config_option_update":
				if len(p.Update.ConfigOptions) > 0 {
					replayMeta.ConfigOptions = p.Update.ConfigOptions
				}
			case "session_info_update":
				if p.Update.Title != "" {
					replayMeta.Title = p.Update.Title
				}
				if p.Update.UpdatedAt != "" {
					replayMeta.UpdatedAt = p.Update.UpdatedAt
				}
			}
			replayMu.Unlock()
		})

		var loadResult SessionLoadResult
		err := a.forwarder.Send(ctx, "session/load", SessionLoadParams{
			SessionID:  savedSessionID,
			CWD:        cwd,
			MCPServers: mcpServers,
		}, &loadResult)
		cancelReplaySub()

		if err == nil {
			a.mu.Lock()
			a.caps = initResult.AgentCapabilities
			a.initMeta = newInitMeta
			// sessionID is already set (savedSessionID)
			a.loadHistory = replay
			a.sessionMeta = replayMeta
			a.ready = true
			a.initializing = false
			a.mu.Unlock()
			a.initCond.Broadcast()
			log.Printf("[agent] connected: agent=%s session=%s (resumed, %d history updates)",
				a.name, savedSessionID, len(replay))
			return nil
		}
		// session/load failed — fall through to session/new.
	}

	// Step 4: create a new session.
	var newResult SessionNewResult
	if err := a.forwarder.Send(ctx, "session/new", SessionNewParams{
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
		ConfigOptions: newResult.ConfigOptions,
	}
	a.ready = true
	a.initializing = false
	a.mu.Unlock()
	a.initCond.Broadcast()

	modeID := ""
	for _, opt := range newResult.ConfigOptions {
		if opt.ID == "mode" || opt.Category == "mode" {
			modeID = opt.CurrentValue
			break
		}
	}
	log.Printf("[agent] connected: agent=%s session=%s mode=%s",
		a.name, newResult.SessionID, modeID)
	return nil
}
