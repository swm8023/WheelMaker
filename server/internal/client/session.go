package client

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/swm8023/wheelmaker/internal/acp"
	"github.com/swm8023/wheelmaker/internal/logger"
)

// emptyMCPServers returns an empty MCP server list for session/new and session/load calls.
// Replace this helper when MCP config support is added.
func emptyMCPServers() []acp.MCPServer {
	return []acp.MCPServer{}
}

// SwitchMode controls how an agent switch affects session context.
type SwitchMode int

const (
	// SwitchClean discards the current session; new conn is lazily initialized on next prompt.
	SwitchClean SwitchMode = iota
	// SwitchWithContext passes the last reply as bootstrap context to the new session.
	SwitchWithContext
)

// clientInitMeta holds agent-level metadata from the initialize handshake.
type clientInitMeta struct {
	ProtocolVersion       string
	AgentCapabilities     acp.AgentCapabilities
	AgentInfo             *acp.AgentInfo
	AuthMethods           []acp.AuthMethod
	ClientProtocolVersion int
	ClientCapabilities    acp.ClientCapabilities
	ClientInfo            *acp.AgentInfo
}

// clientSessionMeta holds session-level metadata updated by session/update notifications.
type clientSessionMeta struct {
	ConfigOptions     []acp.ConfigOption
	AvailableCommands []acp.AvailableCommand
	Title             string
	UpdatedAt         string
}

// ensureReady performs the ACP handshake if the client is not yet connected:
//  1. Send "initialize" and store agent capabilities.
//  2. If caps.LoadSession and a sessionID is stored, attempt session/load.
//  3. Otherwise, create a new session via session/new.
//
// Single-flight: if concurrent callers race here, only one performs the I/O;
// others wait on c.initCond and return once ready is set.
func (c *Client) ensureReady(ctx context.Context) error {
	c.mu.Lock()
	for c.session.initializing {
		c.initCond.Wait()
	}
	if c.session.ready {
		c.mu.Unlock()
		return nil
	}
	c.session.initializing = true
	fwd := c.conn.forwarder
	savedSID := c.session.id
	cwd := c.cwd
	c.mu.Unlock()

	notifyDone := func() {
		c.mu.Lock()
		c.session.initializing = false
		c.mu.Unlock()
		c.initCond.Broadcast()
	}

	// Step 1: initialize handshake.
	clientCaps := acp.ClientCapabilities{
		FS: &acp.FSCapabilities{
			ReadTextFile:  true,
			WriteTextFile: true,
		},
		Terminal: true,
	}
	initResult, err := fwd.Initialize(ctx, acp.InitializeParams{
		ProtocolVersion:    acpClientProtocolVersion,
		ClientCapabilities: clientCaps,
		ClientInfo:         acpClientInfo,
	})
	if err != nil {
		notifyDone()
		return fmt.Errorf("ensureReady: initialize: %w", err)
	}

	newInitMeta := clientInitMeta{
		ProtocolVersion:       initResult.ProtocolVersion.String(),
		AgentCapabilities:     initResult.AgentCapabilities,
		AgentInfo:             initResult.AgentInfo,
		AuthMethods:           initResult.AuthMethods,
		ClientProtocolVersion: acpClientProtocolVersion,
		ClientCapabilities:    clientCaps,
		ClientInfo:            acpClientInfo,
	}

	// Step 2: attempt session/load if possible.
	if savedSID != "" && initResult.AgentCapabilities.LoadSession {
		var replayMu sync.Mutex
		var replay []acp.Update
		replayMeta := clientSessionMeta{}

		c.mu.Lock()
		c.session.replayH = func(p acp.SessionUpdateParams) {
			if p.SessionID != savedSID {
				return
			}
			derived := acp.ParseSessionUpdateParams(p)
			replayMu.Lock()
			replay = append(replay, derived.Update)
			if len(derived.AvailableCommands) > 0 {
				replayMeta.AvailableCommands = derived.AvailableCommands
			}
			if len(derived.ConfigOptions) > 0 {
				replayMeta.ConfigOptions = derived.ConfigOptions
			}
			if derived.Title != "" {
				replayMeta.Title = derived.Title
			}
			if derived.UpdatedAt != "" {
				replayMeta.UpdatedAt = derived.UpdatedAt
			}
			replayMu.Unlock()
		}
		c.mu.Unlock()

		var loadResult acp.SessionLoadResult
		loadErr := func() error {
			res, err := fwd.SessionLoad(ctx, acp.SessionLoadParams{
				SessionID:  savedSID,
				CWD:        cwd,
				MCPServers: emptyMCPServers(),
			})
			if err == nil {
				loadResult = res
			}
			return err
		}()
		c.mu.Lock()
		c.session.replayH = nil
		c.mu.Unlock()

		if loadErr == nil {
			replayMu.Lock()
			replayUpdates := replay
			meta := replayMeta
			replayMu.Unlock()
			if len(meta.ConfigOptions) == 0 && len(loadResult.ConfigOptions) > 0 {
				meta.ConfigOptions = loadResult.ConfigOptions
			}

			c.mu.Lock()
			c.initMeta = newInitMeta
			c.sessionMeta = meta
			c.session.ready = true
			c.session.initializing = false
			c.mu.Unlock()
			c.initCond.Broadcast()
			logger.Debug("[client] connected: agent=%s session=%s (resumed, %d history updates)",
				c.conn.name, savedSID, len(replayUpdates))
			return nil
		}
		// session/load failed — fall through to session/new.
	}

	// Step 3: create a new session.
	newResult, err := fwd.SessionNew(ctx, acp.SessionNewParams{
		CWD:        cwd,
		MCPServers: emptyMCPServers(),
	})
	if err != nil {
		notifyDone()
		return fmt.Errorf("ensureReady: session/new: %w", err)
	}

	c.mu.Lock()
	c.initMeta = newInitMeta
	c.session.id = newResult.SessionID
	c.sessionMeta = clientSessionMeta{
		ConfigOptions: newResult.ConfigOptions,
	}
	c.session.ready = true
	c.session.initializing = false
	c.mu.Unlock()
	c.initCond.Broadcast()

	modeID := ""
	for _, opt := range newResult.ConfigOptions {
		if opt.ID == "mode" || opt.Category == "mode" {
			modeID = opt.CurrentValue
			break
		}
	}
	logger.Debug("[client] connected: agent=%s session=%s mode=%s",
		c.conn.name, newResult.SessionID, modeID)
	return nil
}

// ensureReadyAndNotify calls ensureReady and sends a "Session ready" message
// to chatID when this call is the one that first transitions to ready.
func (c *Client) ensureReadyAndNotify(ctx context.Context, chatID string) error {
	c.mu.Lock()
	wasReady := c.session.ready
	c.mu.Unlock()

	if err := c.ensureReady(ctx); err != nil {
		return err
	}

	if !wasReady {
		snap := c.sessionConfigSnapshot()
		if snap.Mode != "" || snap.Model != "" {
			c.reply(chatID, fmt.Sprintf("Session ready: mode=%s model=%s",
				renderUnknown(snap.Mode), renderUnknown(snap.Model)))
		} else {
			c.reply(chatID, "Session ready.")
		}
		c.saveSessionState()
	}
	return nil
}

// sessionConfigSnapshot returns the current mode/model values.
func (c *Client) sessionConfigSnapshot() acp.SessionConfigSnapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	return acp.SessionConfigSnapshotFromOptions(c.sessionMeta.ConfigOptions)
}

// promptStream sends a prompt and returns a channel of streaming updates.
// The caller must drain the channel until a Done update is received.
func (c *Client) promptStream(ctx context.Context, text string) (<-chan acp.Update, error) {
	c.mu.Lock()
	c.session.lastReply = ""
	c.mu.Unlock()

	if err := c.ensureReady(ctx); err != nil {
		return nil, err
	}

	c.mu.Lock()
	sessID := c.session.id
	promptCtx, promptCancel := context.WithCancel(ctx)
	c.prompt.ctx = promptCtx
	c.prompt.cancel = promptCancel
	c.mu.Unlock()

	updates := make(chan acp.Update, 32)
	interceptCh := make(chan acp.Update, 32)

	c.mu.Lock()
	c.prompt.updatesCh = interceptCh
	c.mu.Unlock()

	var replyMu sync.Mutex
	var replyBuf strings.Builder

	go func() {
		defer func() {
			c.mu.Lock()
			c.prompt.ctx = nil
			c.prompt.cancel = nil
			c.prompt.activeTCs = make(map[string]struct{})
			c.prompt.updatesCh = nil
			c.mu.Unlock()
			promptCancel()
		}()

		result, err := c.conn.forwarder.SessionPrompt(promptCtx, acp.SessionPromptParams{
			SessionID: sessID,
			Prompt:    []acp.ContentBlock{{Type: "text", Text: text}},
		})

		// Drain interceptCh into updates, accumulating text as we go.
		draining := true
		for draining {
			select {
			case u, ok := <-interceptCh:
				if !ok {
					draining = false
					break
				}
				if u.Type == acp.UpdateText {
					replyMu.Lock()
					replyBuf.WriteString(u.Content)
					replyMu.Unlock()
				}
				select {
				case updates <- u:
				case <-ctx.Done():
					draining = false
				}
			default:
				draining = false
			}
		}

		replyMu.Lock()
		reply := replyBuf.String()
		replyMu.Unlock()
		c.mu.Lock()
		c.session.lastReply = reply
		c.mu.Unlock()

		var finalUpdate acp.Update
		if err != nil {
			finalUpdate = acp.Update{Type: acp.UpdateError, Err: err, Done: true}
		} else {
			finalUpdate = acp.Update{Type: acp.UpdateDone, Content: result.StopReason, Done: true}
		}
		select {
		case updates <- finalUpdate:
		case <-ctx.Done():
		}
		close(updates)
	}()

	return updates, nil
}

// cancelPrompt emits tool_call_cancelled updates then sends session/cancel.
func (c *Client) cancelPrompt() error {
	c.mu.Lock()
	sessID := c.session.id
	ready := c.session.ready
	cancel := c.prompt.cancel
	ch := c.prompt.updatesCh
	var cancelIDs []string
	for id := range c.prompt.activeTCs {
		cancelIDs = append(cancelIDs, id)
	}
	c.mu.Unlock()

	if cancel != nil {
		cancel()
	}

	for _, id := range cancelIDs {
		u := acp.Update{Type: acp.UpdateToolCallCancelled, Content: id}
		if ch != nil {
			select {
			case ch <- u:
			default:
			}
		}
	}

	if sessID == "" || !ready {
		return nil
	}
	return c.conn.forwarder.SessionCancel(sessID)
}

// persistMeta snapshots current session metadata into in-memory state and
// returns true if anything changed. Must be called while NOT holding c.mu.
//
// Concurrency safety: this function acquires c.mu twice (once to read, once
// to write). This looks like a TOCTOU window but is safe in practice because
// every caller is serialized by promptMu:
//   - saveSessionState is called only from handlePrompt, ensureReadyAndNotify,
//     switchAgent, createNewSession, and loadSessionByIndex — all under promptMu.
//   - Close is a known exception: it calls saveSessionState during shutdown
//     without promptMu, which is acceptable because Close is not concurrent
//     with prompt operations by contract.
//
// c.store.Save (file I/O) is intentionally kept outside c.mu to avoid
// stalling ACP callback goroutines during disk writes.
func (c *Client) persistMeta() bool {
	c.mu.Lock()
	if c.conn == nil {
		c.mu.Unlock()
		return false
	}
	agentName := c.conn.name
	sessionID := c.session.id
	initMeta := c.initMeta
	sessMeta := c.sessionMeta
	c.mu.Unlock()

	if agentName == "" {
		return false
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.state == nil {
		return false
	}
	if c.state.Agents == nil {
		c.state.Agents = map[string]*AgentState{}
	}
	as := c.state.Agents[agentName]
	if as == nil {
		as = &AgentState{}
		c.state.Agents[agentName] = as
	}

	changed := false

	if sessionID != "" && as.LastSessionID != sessionID {
		as.LastSessionID = sessionID
		changed = true
	}

	if initMeta.ProtocolVersion != "" {
		as.ProtocolVersion = initMeta.ProtocolVersion
		as.AgentCapabilities = initMeta.AgentCapabilities
		as.AgentInfo = initMeta.AgentInfo
		as.AuthMethods = initMeta.AuthMethods
		if c.state.Connection == nil {
			c.state.Connection = &ConnectionConfig{}
		}
		c.state.Connection.ProtocolVersion = initMeta.ClientProtocolVersion
		c.state.Connection.ClientCapabilities = initMeta.ClientCapabilities
		c.state.Connection.ClientInfo = initMeta.ClientInfo
		changed = true
	}

	hasSessionData := len(sessMeta.AvailableCommands) > 0 || len(sessMeta.ConfigOptions) > 0 ||
		sessMeta.Title != "" || sessMeta.UpdatedAt != ""
	if hasSessionData {
		if as.Session == nil {
			as.Session = &SessionState{}
		}
		as.Session.ConfigOptions = sessMeta.ConfigOptions
		as.Session.AvailableCommands = sessMeta.AvailableCommands
		if sessMeta.Title != "" {
			as.Session.Title = sessMeta.Title
		}
		if sessMeta.UpdatedAt != "" {
			as.Session.UpdatedAt = sessMeta.UpdatedAt
		}
		changed = true
	}

	return changed
}

// resetSessionFields resets the 6 session-level fields common to session/new,
// session/load, and agent-switch operations. Callers MUST hold c.mu.
func (c *Client) resetSessionFields(sid string, configOpts []acp.ConfigOption) {
	c.session.id = sid
	c.session.ready = true
	c.session.lastReply = ""
	c.prompt.activeTCs = make(map[string]struct{})
	c.sessionMeta = clientSessionMeta{ConfigOptions: configOpts}
}

// saveSessionState calls persistMeta and writes to disk if changed.
func (c *Client) saveSessionState() {
	if !c.persistMeta() {
		return
	}
	c.mu.Lock()
	s := c.state
	c.mu.Unlock()
	if s != nil {
		_ = c.store.Save(s)
	}
}
