package client

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	acp "github.com/swm8023/wheelmaker/internal/protocol"
	logger "github.com/swm8023/wheelmaker/internal/shared"
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

// switchAgent cancels any in-progress prompt, waits for it to finish via
// promptMu, connects a new agent via AgentFactory, and replaces the instance.
func (s *Session) switchAgent(ctx context.Context, name string, mode SwitchMode) error {
	creator := s.registry.CreatorByName(name)
	if creator == nil {
		return fmt.Errorf("unknown agent: %q (registered: %v)", name, s.registry.Names())
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
	oldInst := s.instance
	savedLastReply := s.lastReply
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

	// Connect new agent via factory.
	newInst, err := creator(ctx)
	if err != nil {
		return fmt.Errorf("connect %q: %w", name, err)
	}
	newInst.SetCallbacks(s)

	// Replace instance atomically and close old instance.
	s.mu.Lock()
	s.instance = newInst
	s.initializing = false
	s.prompt.updatesCh = nil
	s.initMeta = clientInitMeta{}
	s.resetSessionFields(savedSID, nil) // sets ready=true — override below:
	s.ready = false
	s.mu.Unlock()
	s.initCond.Broadcast() // MUST be outside s.mu — never inside the lock

	if oldInst != nil {
		_ = oldInst.Close()
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

// ensureReady performs the ACP handshake if the session is not yet connected:
//  1. Send "initialize" and store agent capabilities.
//  2. If caps.LoadSession and a sessionID is stored, attempt session/load.
//  3. Otherwise, create a new session via session/new.
func (s *Session) ensureReady(ctx context.Context) error {
	s.mu.Lock()
	for s.initializing {
		s.initCond.Wait()
	}
	if s.ready {
		s.mu.Unlock()
		return nil
	}
	s.initializing = true
	inst := s.instance
	if inst == nil {
		s.initializing = false
		s.mu.Unlock()
		s.initCond.Broadcast()
		return errors.New("ensureReady: instance is nil")
	}
	agentName := inst.Name()
	savedSID := s.acpSessionID
	cwd := s.cwd
	s.mu.Unlock()

	notifyDone := func() {
		s.mu.Lock()
		s.initializing = false
		s.mu.Unlock()
		s.initCond.Broadcast()
	}

	// Step 1: initialize handshake.
	clientCaps := acp.ClientCapabilities{
		FS: &acp.FSCapabilities{
			ReadTextFile:  true,
			WriteTextFile: true,
		},
		Terminal: true,
	}
	initResult, err := inst.Initialize(ctx, acp.InitializeParams{
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
	if savedSID != "" && initResult.AgentCapabilities.LoadSession && !isCopilotAgent(agentName) {
		var replayMu sync.Mutex
		var replay []acp.Update
		replayMeta := clientSessionMeta{}

		s.mu.Lock()
		s.replayH = func(p acp.SessionUpdateParams) {
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
		s.mu.Unlock()

		var loadResult acp.SessionLoadResult
		loadErr := func() error {
			res, err := inst.SessionLoad(ctx, acp.SessionLoadParams{
				SessionID:  savedSID,
				CWD:        cwd,
				MCPServers: emptyMCPServers(),
			})
			if err == nil {
				loadResult = res
			}
			return err
		}()
		s.mu.Lock()
		s.replayH = nil
		s.mu.Unlock()

		if loadErr == nil {
			replayMu.Lock()
			replayUpdates := replay
			meta := replayMeta
			replayMu.Unlock()
			if len(meta.ConfigOptions) == 0 && len(loadResult.ConfigOptions) > 0 {
				meta.ConfigOptions = loadResult.ConfigOptions
			}

			s.mu.Lock()
			s.initMeta = newInitMeta
			s.sessionMeta = meta
			s.ready = true
			s.initializing = false
			s.mu.Unlock()
			s.initCond.Broadcast()
			logger.Info("[client] connected: agent=%s session=%s (resumed, %d history updates)",
				inst.Name(), savedSID, len(replayUpdates))
			return nil
		}
	}

	// Step 3: create a new session.
	newResult, err := inst.SessionNew(ctx, acp.SessionNewParams{
		CWD:        cwd,
		MCPServers: emptyMCPServers(),
	})
	if err != nil {
		notifyDone()
		return fmt.Errorf("ensureReady: session/new: %w", err)
	}

	s.mu.Lock()
	s.initMeta = newInitMeta
	s.acpSessionID = newResult.SessionID
	s.sessionMeta = clientSessionMeta{
		ConfigOptions: newResult.ConfigOptions,
	}
	s.ready = true
	s.initializing = false
	s.mu.Unlock()
	s.initCond.Broadcast()

	modeID := ""
	for _, opt := range newResult.ConfigOptions {
		if opt.ID == "mode" || opt.Category == "mode" {
			modeID = opt.CurrentValue
			break
		}
	}
	logger.Info("[client] connected: agent=%s session=%s mode=%s",
		inst.Name(), newResult.SessionID, modeID)
	return nil
}

// ensureReadyAndNotify calls ensureReady and sends a "Session ready" message
// when this call is the one that first transitions to ready.
func (s *Session) ensureReadyAndNotify(ctx context.Context) error {
	s.mu.Lock()
	wasReady := s.ready
	s.mu.Unlock()

	if err := s.ensureReady(ctx); err != nil {
		return err
	}

	if !wasReady {
		snap := s.sessionConfigSnapshot()
		if snap.Mode != "" || snap.Model != "" {
			s.reply(fmt.Sprintf("Session ready: mode=%s model=%s",
				renderUnknown(snap.Mode), renderUnknown(snap.Model)))
		} else {
			s.reply("Session ready.")
		}
		s.saveSessionState()
	}
	return nil
}

// sessionConfigSnapshot returns the current mode/model values.
func (s *Session) sessionConfigSnapshot() acp.SessionConfigSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return acp.SessionConfigSnapshotFromOptions(s.sessionMeta.ConfigOptions)
}

// promptStream sends a prompt and returns a channel of streaming updates.
func (s *Session) promptStream(ctx context.Context, text string) (<-chan acp.Update, error) {
	s.mu.Lock()
	s.lastReply = ""
	s.mu.Unlock()

	if err := s.ensureReady(ctx); err != nil {
		return nil, err
	}

	s.mu.Lock()
	sessID := s.acpSessionID
	promptCtx, promptCancel := context.WithCancel(ctx)
	s.prompt.ctx = promptCtx
	s.prompt.cancel = promptCancel
	s.mu.Unlock()

	updates := make(chan acp.Update, 32)
	interceptCh := make(chan acp.Update, 32)

	s.mu.Lock()
	s.prompt.updatesCh = interceptCh
	s.mu.Unlock()

	var replyMu sync.Mutex
	var replyBuf strings.Builder

	go func() {
		defer func() {
			s.mu.Lock()
			s.prompt.ctx = nil
			s.prompt.cancel = nil
			s.prompt.activeTCs = make(map[string]struct{})
			s.prompt.updatesCh = nil
			s.mu.Unlock()
			promptCancel()
		}()

		type promptResult struct {
			result acp.SessionPromptResult
			err    error
		}
		resultCh := make(chan promptResult, 1)
		go func() {
			res, err := s.instance.SessionPrompt(promptCtx, acp.SessionPromptParams{
				SessionID: sessID,
				Prompt:    []acp.ContentBlock{{Type: "text", Text: text}},
			})
			resultCh <- promptResult{result: res, err: err}
		}()

		drain := func(u acp.Update) bool {
			if u.Type == acp.UpdateText {
				replyMu.Lock()
				replyBuf.WriteString(u.Content)
				replyMu.Unlock()
			}
			select {
			case updates <- u:
				return true
			case <-ctx.Done():
				return false
			}
		}

		pr := promptResult{}
		for {
			select {
			case u := <-interceptCh:
				if !drain(u) {
					return
				}
			case pr = <-resultCh:
				s.mu.Lock()
				if s.prompt.updatesCh == interceptCh {
					s.prompt.updatesCh = nil
				}
				s.mu.Unlock()
				for {
					select {
					case u := <-interceptCh:
						if !drain(u) {
							return
						}
					default:
						goto drained
					}
				}
			drained:
				goto done
			}
		}

	done:
		result, err := pr.result, pr.err

		replyMu.Lock()
		reply := replyBuf.String()
		replyMu.Unlock()
		s.mu.Lock()
		s.lastReply = reply
		s.mu.Unlock()

		var finalUpdate acp.Update
		if err != nil {
			if isCopilotReasoningEffortError(err) {
				s.invalidateSessionForRetry()
			}
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

func isCopilotAgent(name string) bool {
	return strings.EqualFold(strings.TrimSpace(name), "copilot")
}

func isCopilotReasoningEffortError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "reasoning_effort")
}

// invalidateSessionForRetry clears current session identity so the next prompt
// forces a fresh session/new instead of reusing potentially incompatible config.
func (s *Session) invalidateSessionForRetry() {
	s.mu.Lock()
	agentName := ""
	if s.instance != nil {
		agentName = s.instance.Name()
	}
	s.acpSessionID = ""
	s.ready = false
	s.lastReply = ""
	s.sessionMeta = clientSessionMeta{}
	if agentName != "" && s.state != nil && s.state.Agents != nil {
		if st := s.state.Agents[agentName]; st != nil {
			st.LastSessionID = ""
			st.Session = nil
		}
	}
	s.mu.Unlock()
	s.saveSessionState()
}

// cancelPrompt emits tool_call_cancelled updates then sends session/cancel.
func (s *Session) cancelPrompt() error {
	s.mu.Lock()
	sessID := s.acpSessionID
	ready := s.ready
	cancel := s.prompt.cancel
	ch := s.prompt.updatesCh
	var cancelIDs []string
	for id := range s.prompt.activeTCs {
		cancelIDs = append(cancelIDs, id)
	}
	s.mu.Unlock()

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
	return s.instance.SessionCancel(sessID)
}

// persistMeta snapshots current session metadata into in-memory state.
func (s *Session) persistMeta() bool {
	s.mu.Lock()
	if s.instance == nil {
		s.mu.Unlock()
		return false
	}
	agentName := s.instance.Name()
	sessionID := s.acpSessionID
	initMeta := s.initMeta
	sessMeta := s.sessionMeta
	s.mu.Unlock()

	if agentName == "" {
		return false
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state == nil {
		return false
	}
	if s.state.Agents == nil {
		s.state.Agents = map[string]*AgentState{}
	}
	as := s.state.Agents[agentName]
	if as == nil {
		as = &AgentState{}
		s.state.Agents[agentName] = as
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
		if s.state.Connection == nil {
			s.state.Connection = &ConnectionConfig{}
		}
		s.state.Connection.ProtocolVersion = initMeta.ClientProtocolVersion
		s.state.Connection.ClientCapabilities = initMeta.ClientCapabilities
		s.state.Connection.ClientInfo = initMeta.ClientInfo
		changed = true
	}

	hasSessionData := len(sessMeta.AvailableCommands) > 0 || len(sessMeta.ConfigOptions) > 0 ||
		sessMeta.Title != "" || sessMeta.UpdatedAt != ""
	if hasSessionData {
		if as.Session == nil {
			as.Session = &SessionState{}
		}
		as.Session.ConfigOptions = append([]acp.ConfigOption(nil), sessMeta.ConfigOptions...)
		as.Session.AvailableCommands = append([]acp.AvailableCommand(nil), sessMeta.AvailableCommands...)
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

// resetSessionFields resets session-level fields. Callers MUST hold s.mu.
func (s *Session) resetSessionFields(sid string, configOpts []acp.ConfigOption) {
	s.acpSessionID = sid
	s.ready = true
	s.lastReply = ""
	s.prompt.activeTCs = make(map[string]struct{})
	s.sessionMeta = clientSessionMeta{ConfigOptions: configOpts}
}

// saveSessionState calls persistMeta and writes to disk if changed.
func (s *Session) saveSessionState() {
	if !s.persistMeta() {
		return
	}
	s.mu.Lock()
	st := s.state
	s.mu.Unlock()
	if st != nil {
		_ = s.store.Save(st)
	}
}
func cloneSessionAgentState(src *SessionAgentState) *SessionAgentState {
	if src == nil {
		return nil
	}
	cp := *src
	cp.ConfigOptions = append([]acp.ConfigOption(nil), src.ConfigOptions...)
	cp.Commands = append([]acp.AvailableCommand(nil), src.Commands...)
	return &cp
}

func cloneClientSessionMeta(src clientSessionMeta) clientSessionMeta {
	return clientSessionMeta{
		ConfigOptions:     append([]acp.ConfigOption(nil), src.ConfigOptions...),
		AvailableCommands: append([]acp.AvailableCommand(nil), src.AvailableCommands...),
		Title:             src.Title,
		UpdatedAt:         src.UpdatedAt,
	}
}

// Snapshot captures the full state of this Session into a SessionSnapshot.
func (s *Session) Snapshot(projectName string) *SessionSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()

	agentName := ""
	if s.instance != nil {
		agentName = s.instance.Name()
	}

	// Deep copy agents map.
	agents := make(map[string]*SessionAgentState, len(s.agents))
	for k, v := range s.agents {
		agents[k] = cloneSessionAgentState(v)
	}

	return &SessionSnapshot{
		ID:           s.ID,
		ProjectName:  projectName,
		Status:       s.Status,
		ActiveAgent:  agentName,
		LastReply:    s.lastReply,
		ACPSessionID: s.acpSessionID,
		CreatedAt:    s.createdAt,
		LastActiveAt: s.lastActiveAt,
		Agents:       agents,
		SessionMeta:  cloneClientSessionMeta(s.sessionMeta),
		InitMeta:     s.initMeta,
	}
}

// Suspend cancels any in-progress prompt, closes the agent, and marks
// this session as suspended. If a SessionStore is provided, the session
// state is persisted.
func (s *Session) Suspend(ctx context.Context, store SessionStore, projectName string) error {
	_ = s.cancelPrompt()

	s.mu.Lock()
	inst := s.instance
	s.instance = nil
	s.ready = false
	s.initializing = false
	s.Status = SessionSuspended
	s.lastActiveAt = time.Now()
	s.mu.Unlock()

	if inst != nil {
		_ = inst.Close()
	}

	if store != nil {
		snap := s.Snapshot(projectName)
		return store.Save(ctx, snap)
	}
	return nil
}

// RestoreFromSnapshot re-hydrates a Session from a persisted snapshot.
// The agent connection is NOT restored — it will be lazily initialized
// on the next prompt via ensureInstance().
func RestoreFromSnapshot(snap *SessionSnapshot, cwd string) *Session {
	s := newSession(snap.ID, cwd)
	s.Status = SessionActive
	s.lastReply = snap.LastReply
	s.acpSessionID = snap.ACPSessionID
	s.createdAt = snap.CreatedAt
	s.lastActiveAt = time.Now()
	s.sessionMeta = cloneClientSessionMeta(snap.SessionMeta)
	s.initMeta = snap.InitMeta

	if snap.Agents != nil {
		for k, v := range snap.Agents {
			s.agents[k] = cloneSessionAgentState(v)
		}
	}
	return s
}

// SessionUpdate routes session/update notifications to the active session.
func (c *Client) SessionUpdate(params acp.SessionUpdateParams) {
	c.mu.Lock()
	sess := c.activeSession
	c.mu.Unlock()
	if sess != nil {
		sess.SessionUpdate(params)
	}
}

// SessionRequestPermission handles agent permission requests via active session state.
func (c *Client) SessionRequestPermission(ctx context.Context, params acp.PermissionRequestParams) (acp.PermissionResult, error) {
	c.mu.Lock()
	sess := c.activeSession
	c.mu.Unlock()
	if sess != nil {
		return sess.SessionRequestPermission(ctx, params)
	}
	return acp.PermissionResult{Outcome: "cancelled"}, nil
}

// SessionUpdate receives session/update notifications from the agent.
func (s *Session) SessionUpdate(params acp.SessionUpdateParams) {
	s.mu.Lock()
	sessID := s.acpSessionID
	ch := s.prompt.updatesCh
	promptCtx := s.prompt.ctx
	replayH := s.replayH
	s.mu.Unlock()

	if replayH != nil {
		replayH(params)
	}

	if params.SessionID != sessID {
		return
	}

	derived := acp.ParseSessionUpdateParams(params)

	if len(derived.AvailableCommands) > 0 || len(derived.ConfigOptions) > 0 || derived.Title != "" || derived.UpdatedAt != "" {
		s.mu.Lock()
		if len(derived.AvailableCommands) > 0 {
			s.sessionMeta.AvailableCommands = append([]acp.AvailableCommand(nil), derived.AvailableCommands...)
		}
		if len(derived.ConfigOptions) > 0 {
			s.sessionMeta.ConfigOptions = append([]acp.ConfigOption(nil), derived.ConfigOptions...)
		}
		if derived.Title != "" {
			s.sessionMeta.Title = derived.Title
		}
		if derived.UpdatedAt != "" {
			s.sessionMeta.UpdatedAt = derived.UpdatedAt
		}
		s.mu.Unlock()
	}

	if derived.TrackAddToolCall != "" || derived.TrackDoneToolCall != "" {
		s.mu.Lock()
		if derived.TrackAddToolCall != "" {
			s.prompt.activeTCs[derived.TrackAddToolCall] = struct{}{}
		}
		if derived.TrackDoneToolCall != "" {
			delete(s.prompt.activeTCs, derived.TrackDoneToolCall)
		}
		s.mu.Unlock()
	}

	if ch == nil {
		return
	}
	if promptCtx == nil {
		select {
		case ch <- derived.Update:
		default:
		}
		return
	}
	select {
	case ch <- derived.Update:
	case <-promptCtx.Done():
	}
}

// SessionRequestPermission responds to session/request_permission agent requests.
func (s *Session) SessionRequestPermission(ctx context.Context, params acp.PermissionRequestParams) (acp.PermissionResult, error) {
	s.mu.Lock()
	pCtx := s.prompt.ctx
	snap := acp.SessionConfigSnapshotFromOptions(s.sessionMeta.ConfigOptions)
	s.mu.Unlock()
	if pCtx != nil {
		ctx = pCtx
	}
	return s.permRouter.decide(ctx, params, snap.Mode)
}
