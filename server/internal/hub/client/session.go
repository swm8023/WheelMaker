package client

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/swm8023/wheelmaker/internal/hub/agent"
	"github.com/swm8023/wheelmaker/internal/im"
	acp "github.com/swm8023/wheelmaker/internal/protocol"
	logger "github.com/swm8023/wheelmaker/internal/shared"
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
// prompt lifecycle and callback handling.
// A Client holds multiple Sessions, routed by IM routeKey.
type Session struct {
	ID     string
	Status SessionStatus

	// instance is the agent runtime bound to this Session.
	// Created lazily by ensureInstance(). Nil means no agent connected yet.
	instance agent.Instance

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

	permRouter *permissionRouter

	// Back-references to Client-owned resources needed by Session methods.
	cwd         string
	yolo        bool
	registry    *agent.ACPFactory
	persistence ClientStateStore
	state       *ProjectState
	imRouter    IMRouter
	imSource    *im.ChatRef

	createdAt    time.Time
	lastActiveAt time.Time

	mu       sync.Mutex
	promptMu sync.Mutex
}

// newSession creates a Session with sensible defaults.
func newSession(id string, cwd string) *Session {
	if strings.TrimSpace(id) == "" {
		id = newSessionID()
	}
	s := &Session{
		ID:        id,
		Status:    SessionActive,
		agents:    make(map[string]*SessionAgentState),
		cwd:       cwd,
		createdAt: time.Now(),
		prompt: promptState{
			activeTCs: make(map[string]struct{}),
		},
	}
	s.initCond = sync.NewCond(&s.mu)
	return s
}

func newSessionID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("sess-fallback-%d", time.Now().UnixNano())
	}
	buf[6] = (buf[6] & 0x0f) | 0x40
	buf[8] = (buf[8] & 0x3f) | 0x80

	hexBuf := hex.EncodeToString(buf)
	return fmt.Sprintf("sess-%s-%s-%s-%s-%s",
		hexBuf[0:8],
		hexBuf[8:12],
		hexBuf[12:16],
		hexBuf[16:20],
		hexBuf[20:32],
	)
}

// reply sends a text response to the active chat via the IM channel.
func (s *Session) reply(text string) {
	if router, source, ok := s.imContext(); ok {
		_ = router.SystemNotify(context.Background(), im.SendTarget{SessionID: s.ID, Source: &source}, im.SystemPayload{
			Kind: "message",
			Body: text,
		})
		return
	}
	fmt.Println(text)
}

func (s *Session) setIMSource(source im.ChatRef) {
	source = im.ChatRef{ChannelID: strings.TrimSpace(source.ChannelID), ChatID: strings.TrimSpace(source.ChatID)}
	s.mu.Lock()
	s.imSource = &source
	s.mu.Unlock()
}

func (s *Session) imContext() (IMRouter, im.ChatRef, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.imRouter == nil || s.imSource == nil || s.imSource.ChannelID == "" || s.imSource.ChatID == "" {
		return nil, im.ChatRef{}, false
	}
	return s.imRouter, *s.imSource, true
}

// ensureInstance connects the active agent via AgentFactory and sets up the
// runtime instance if not already running. Connect is executed outside s.mu.
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
	savedSID := ""
	if s.state.Agents != nil {
		if as := s.state.Agents[name]; as != nil && as.LastSessionID != "" {
			savedSID = as.LastSessionID
		}
	}
	s.mu.Unlock()

	creator := s.registry.CreatorByName(name)
	if creator == nil {
		return fmt.Errorf("no agent registered for %q", name)
	}

	inst, err := creator(ctx)
	if err != nil {
		return err
	}
	inst.SetCallbacks(s)

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
	s.syncRuntimeToProjectState() // save outgoing agent state before reset

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
		s.syncRuntimeToProjectState()
	}

	// Update ActiveAgent and save.
	s.mu.Lock()
	if s.state != nil {
		s.state.ActiveAgent = name
	}
	st := s.state
	s.mu.Unlock()
	if st != nil {
		s.persistProjectState(st)
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
		s.syncAndPersistProjectState()
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
	s.syncAndPersistProjectState()
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

// syncRuntimeToProjectState snapshots current runtime metadata into ProjectState.
func (s *Session) syncRuntimeToProjectState() bool {
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

func (s *Session) persistProjectState(st *ProjectState) {
	if st == nil || s.persistence == nil {
		return
	}
	_ = s.persistence.SaveProjectState(st)
}

// syncAndPersistProjectState syncs runtime state into ProjectState and persists when changed.
func (s *Session) syncAndPersistProjectState() {
	if !s.syncRuntimeToProjectState() {
		return
	}
	s.mu.Lock()
	st := s.state
	s.mu.Unlock()
	if st != nil {
		s.persistProjectState(st)
	}
}

// cloneSessionAgentState deep-copies SessionAgentState and its slice fields.
func cloneSessionAgentState(src *SessionAgentState) *SessionAgentState {
	if src == nil {
		return nil
	}
	cp := *src
	cp.ConfigOptions = append([]acp.ConfigOption(nil), src.ConfigOptions...)
	cp.Commands = append([]acp.AvailableCommand(nil), src.Commands...)
	return &cp
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
func (s *Session) Suspend(ctx context.Context, projectName string) error {
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

	if s.persistence != nil && s.persistence.SessionStoreEnabled() {
		snap := s.Snapshot(projectName)
		return s.persistence.SaveSession(ctx, snap)
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
func (s *Session) SessionRequestPermission(ctx context.Context, requestID int64, params acp.PermissionRequestParams) (acp.PermissionResult, error) {
	s.mu.Lock()
	pCtx := s.prompt.ctx
	snap := acp.SessionConfigSnapshotFromOptions(s.sessionMeta.ConfigOptions)
	s.mu.Unlock()
	if pCtx != nil {
		ctx = pCtx
	}
	return s.permRouter.decide(ctx, requestID, params, snap.Mode)
}
