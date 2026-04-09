package client

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/swm8023/wheelmaker/internal/hub/agent"
	"github.com/swm8023/wheelmaker/internal/im"
	acp "github.com/swm8023/wheelmaker/internal/protocol"
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

type SessionSummary struct {
	ID        string `json:"id"`
	Title     string `json:"title,omitempty"`
	UpdatedAt string `json:"updatedAt,omitempty"`
}

// SessionAgentState holds all persisted per-agent metadata within one Session.
type SessionAgentState struct {
	ACPSessionID      string                 `json:"acpSessionId,omitempty"`
	ConfigOptions     []acp.ConfigOption     `json:"configOptions,omitempty"`
	Commands          []acp.AvailableCommand `json:"commands,omitempty"`
	Title             string                 `json:"title,omitempty"`
	UpdatedAt         string                 `json:"updatedAt,omitempty"`
	ProtocolVersion   string                 `json:"protocolVersion,omitempty"`
	AgentCapabilities acp.AgentCapabilities  `json:"agentCapabilities,omitempty"`
	AgentInfo         *acp.AgentInfo         `json:"agentInfo,omitempty"`
	AuthMethods       []acp.AuthMethod       `json:"authMethods,omitempty"`
	Sessions          []SessionSummary       `json:"sessions,omitempty"`
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
	agents      map[string]*SessionAgentState
	activeAgent string

	// Runtime ACP session state (moved from Client.session / Client.sessionMeta / Client.initMeta).
	acpSessionID string
	ready        bool
	initializing bool
	lastReply    string

	prompt   promptState
	initCond *sync.Cond

	permissionMu         sync.Mutex
	permissionDecisionCh chan struct{}
	permissionPending    map[int64]chan acp.PermissionResult
	timeoutLimiter       *timeoutNotifyLimiter

	// Back-references to Client-owned resources needed by Session methods.
	projectName string
	cwd         string
	yolo        bool
	registry    *agent.ACPFactory
	store       Store
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
		permissionDecisionCh: make(chan struct{}, 1),
		permissionPending:    make(map[int64]chan acp.PermissionResult),
		timeoutLimiter:       newTimeoutNotifyLimiter(timeoutNotifyCooldown),
	}
	s.initCond = sync.NewCond(&s.mu)
	s.permissionDecisionCh <- struct{}{}
	return s
}

func cloneAgentInfo(src *acp.AgentInfo) *acp.AgentInfo {
	if src == nil {
		return nil
	}
	cp := *src
	return &cp
}

func cloneSessionSummaries(src []SessionSummary) []SessionSummary {
	return append([]SessionSummary(nil), src...)
}

func cloneSessionAgentState(src *SessionAgentState) *SessionAgentState {
	if src == nil {
		return nil
	}
	cp := *src
	cp.ConfigOptions = append([]acp.ConfigOption(nil), src.ConfigOptions...)
	cp.Commands = append([]acp.AvailableCommand(nil), src.Commands...)
	cp.AuthMethods = append([]acp.AuthMethod(nil), src.AuthMethods...)
	cp.Sessions = cloneSessionSummaries(src.Sessions)
	cp.AgentInfo = cloneAgentInfo(src.AgentInfo)
	return &cp
}

func inferActiveAgent(acpSessionID string, agents map[string]*SessionAgentState) string {
	acpSessionID = strings.TrimSpace(acpSessionID)
	if acpSessionID != "" {
		for name, state := range agents {
			if state != nil && strings.TrimSpace(state.ACPSessionID) == acpSessionID {
				return name
			}
		}
	}
	if len(agents) == 1 {
		for name := range agents {
			return name
		}
	}
	return ""
}

func (s *Session) agentStateLocked(name string) *SessionAgentState {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}
	if s.agents == nil {
		s.agents = map[string]*SessionAgentState{}
	}
	state := s.agents[name]
	if state == nil {
		state = &SessionAgentState{}
		s.agents[name] = state
	}
	return state
}

func (s *Session) currentAgentNameLocked() string {
	if s.instance != nil && strings.TrimSpace(s.instance.Name()) != "" {
		return strings.TrimSpace(s.instance.Name())
	}
	if strings.TrimSpace(s.activeAgent) != "" {
		return strings.TrimSpace(s.activeAgent)
	}
	return inferActiveAgent(s.acpSessionID, s.agents)
}

func (s *Session) currentAgentStateSnapshot() (*SessionAgentState, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	name := s.currentAgentNameLocked()
	if name == "" {
		return nil, ""
	}
	return cloneSessionAgentState(s.agents[name]), name
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
	name := s.currentAgentNameLocked()
	if name == "" && s.registry != nil {
		name = strings.TrimSpace(s.registry.PreferredName())
	}
	name = strings.TrimSpace(name)
	savedSID := ""
	if as := s.agents[name]; as != nil && strings.TrimSpace(as.ACPSessionID) != "" {
		savedSID = strings.TrimSpace(as.ACPSessionID)
	}
	s.mu.Unlock()

	if name == "" {
		return fmt.Errorf("no available ACP provider")
	}

	creator := s.registry.CreatorByName(name)
	if creator == nil {
		fallback := ""
		if s.registry != nil {
			fallback = strings.TrimSpace(s.registry.PreferredName())
		}
		if fallback == "" {
			return fmt.Errorf("no available ACP provider")
		}
		if !strings.EqualFold(fallback, name) {
			hubLogger(s.projectName).Warn("requested agent unavailable requested=%s fallback=%s", name, fallback)
		}
		name = fallback
		creator = s.registry.CreatorByName(name)
		if creator == nil {
			return fmt.Errorf("no agent registered for %q", name)
		}
	}
	s.mu.Lock()
	if as := s.agents[name]; as != nil && strings.TrimSpace(as.ACPSessionID) != "" {
		savedSID = strings.TrimSpace(as.ACPSessionID)
	}
	s.mu.Unlock()

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
	s.activeAgent = name
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

	// Read saved session ID for incoming agent.
	s.mu.Lock()
	var savedSID string
	if as := s.agents[name]; as != nil {
		savedSID = strings.TrimSpace(as.ACPSessionID)
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
	s.activeAgent = name
	s.acpSessionID = savedSID
	s.lastReply = ""
	s.prompt.activeTCs = make(map[string]struct{})
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
			hubLogger(s.projectName).Warn("switch with context bootstrap prompt failed err=%v", err)
		} else {
			for u := range ch {
				if u.Err != nil {
					hubLogger(s.projectName).Warn("switch with context bootstrap prompt failed err=%v", u.Err)
				}
			}
		}
		s.persistSessionBestEffort()
	}
	s.persistSessionBestEffort()

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

	// Step 2: attempt session/load if possible.
	if savedSID != "" && initResult.AgentCapabilities.LoadSession {
		loadResult, loadErr := inst.SessionLoad(ctx, acp.SessionLoadParams{
			SessionID:  savedSID,
			CWD:        cwd,
			MCPServers: emptyMCPServers(),
		})
		if loadErr == nil {
			s.mu.Lock()
			state := s.agentStateLocked(agentName)
			state.ACPSessionID = savedSID
			state.ConfigOptions = loadResult.ConfigOptions
			state.ProtocolVersion = initResult.ProtocolVersion.String()
			state.AgentCapabilities = initResult.AgentCapabilities
			state.AgentInfo = cloneAgentInfo(initResult.AgentInfo)
			state.AuthMethods = initResult.AuthMethods
			s.activeAgent = agentName
			s.acpSessionID = savedSID
			s.ready = true
			s.initializing = false
			s.mu.Unlock()
			s.initCond.Broadcast()
			hubLogger(s.projectName).Info("connected agent=%s session=%s resumed",
				inst.Name(), savedSID)
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
	state := s.agentStateLocked(agentName)
	state.ACPSessionID = newResult.SessionID
	state.ConfigOptions = newResult.ConfigOptions
	state.Commands = nil
	state.Title = ""
	state.UpdatedAt = ""
	state.ProtocolVersion = initResult.ProtocolVersion.String()
	state.AgentCapabilities = initResult.AgentCapabilities
	state.AgentInfo = cloneAgentInfo(initResult.AgentInfo)
	state.AuthMethods = initResult.AuthMethods
	s.activeAgent = agentName
	s.acpSessionID = newResult.SessionID
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
	hubLogger(s.projectName).Info("connected agent=%s session=%s mode=%s",
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
		s.persistSessionBestEffort()
	}
	return nil
}

// sessionConfigSnapshot returns the current mode/model values.
func (s *Session) sessionConfigSnapshot() acp.SessionConfigSnapshot {
	state, _ := s.currentAgentStateSnapshot()
	if state == nil {
		return acp.SessionConfigSnapshot{}
	}
	return acp.SessionConfigSnapshotFromOptions(state.ConfigOptions)
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
	interceptCh := make(chan acp.SessionUpdateParams, 32)

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

		drain := func(params acp.SessionUpdateParams) bool {
			update := params.Update
			u := acp.Update{}
			switch update.SessionUpdate {
			case acp.SessionUpdateAgentMessageChunk:
				var content acp.ContentBlock
				if len(update.Content) > 0 && json.Unmarshal(update.Content, &content) == nil && content.Type == acp.ContentBlockTypeText {
					u = acp.Update{Type: acp.UpdateText, Content: content.Text}
				} else {
					u = acp.Update{Type: acp.UpdateText}
				}
			case acp.SessionUpdateUserMessageChunk:
				u = acp.Update{Type: acp.UpdateUserChunk}
			case acp.SessionUpdateAgentThoughtChunk:
				var content acp.ContentBlock
				if len(update.Content) > 0 && json.Unmarshal(update.Content, &content) == nil {
					u = acp.Update{Type: acp.UpdateThought, Content: content.Text}
				} else {
					u = acp.Update{Type: acp.UpdateThought}
				}
			case acp.SessionUpdateToolCall, acp.SessionUpdateToolCallUpdate:
				raw, _ := json.Marshal(update)
				u = acp.Update{Type: acp.UpdateToolCall, Raw: raw}
			case acp.SessionUpdatePlan:
				raw, _ := json.Marshal(update)
				u = acp.Update{Type: acp.UpdatePlan, Raw: raw}
			case acp.SessionUpdateConfigOptionUpdate:
				raw, _ := json.Marshal(update)
				u = acp.Update{Type: acp.UpdateConfigOption, Raw: raw}
			case acp.SessionUpdateCurrentModeUpdate:
				raw, _ := json.Marshal(update)
				u = acp.Update{Type: acp.UpdateModeChange, Raw: raw}
			default:
				raw, _ := json.Marshal(update)
				u = acp.Update{Type: acp.UpdateType(update.SessionUpdate), Raw: raw}
			}
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
		u := acp.SessionUpdateParams{
			SessionID: sessID,
			Update: acp.SessionUpdate{
				SessionUpdate: acp.SessionUpdateToolCallUpdate,
				ToolCallID:    id,
				Status:        "cancelled",
			},
		}
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

func (s *Session) toRecord() (*SessionRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if name := s.currentAgentNameLocked(); name != "" {
		if state := s.agentStateLocked(name); state != nil {
			state.ACPSessionID = s.acpSessionID
		}
	}

	agentsJSON, err := json.Marshal(s.agents)
	if err != nil {
		return nil, fmt.Errorf("marshal agents: %w", err)
	}

	return &SessionRecord{
		ID:           s.ID,
		ProjectName:  s.projectName,
		Status:       s.Status,
		LastReply:    s.lastReply,
		ACPSessionID: s.acpSessionID,
		AgentsJSON:   string(agentsJSON),
		CreatedAt:    s.createdAt,
		LastActiveAt: s.lastActiveAt,
	}, nil
}

func sessionFromRecord(rec *SessionRecord, cwd string) (*Session, error) {
	s := newSession(rec.ID, cwd)
	s.Status = rec.Status
	s.lastReply = rec.LastReply
	s.acpSessionID = rec.ACPSessionID
	s.createdAt = rec.CreatedAt
	s.lastActiveAt = rec.LastActiveAt
	if strings.TrimSpace(rec.AgentsJSON) != "" {
		if err := json.Unmarshal([]byte(rec.AgentsJSON), &s.agents); err != nil {
			return nil, fmt.Errorf("unmarshal agents_json: %w", err)
		}
	}
	if s.agents == nil {
		s.agents = map[string]*SessionAgentState{}
	}
	s.activeAgent = inferActiveAgent(rec.ACPSessionID, s.agents)
	return s, nil
}

func (s *Session) persistSession(ctx context.Context) error {
	if s.store == nil {
		return nil
	}
	rec, err := s.toRecord()
	if err != nil {
		return err
	}
	return s.store.SaveSession(ctx, rec)
}

func (s *Session) persistSessionBestEffort() {
	if err := s.persistSession(context.Background()); err != nil {
		hubLogger(s.projectName).Warn("persist session failed session=%s err=%v", s.ID, err)
	}
}

// Suspend cancels any in-progress prompt, closes the agent, and marks
// this session as suspended.
func (s *Session) Suspend(ctx context.Context) error {
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
	return s.persistSession(ctx)
}

// SessionUpdate receives session/update notifications from the agent.
func (s *Session) SessionUpdate(params acp.SessionUpdateParams) {
	s.mu.Lock()
	sessID := s.acpSessionID
	ch := s.prompt.updatesCh
	promptCtx := s.prompt.ctx
	s.mu.Unlock()

	if params.SessionID != sessID {
		return
	}

	update := params.Update

	changed := false
	if update.SessionUpdate == acp.SessionUpdateAvailableCommandsUpdate ||
		update.SessionUpdate == acp.SessionUpdateConfigOptionUpdate ||
		update.SessionUpdate == acp.SessionUpdateSessionInfoUpdate {
		s.mu.Lock()
		state := s.agentStateLocked(s.currentAgentNameLocked())
		if state == nil {
			state = s.agentStateLocked(s.activeAgent)
		}
		if state != nil {
			state.ACPSessionID = s.acpSessionID
		}
		switch update.SessionUpdate {
		case acp.SessionUpdateAvailableCommandsUpdate:
			if len(update.AvailableCommands) > 0 {
				state.Commands = update.AvailableCommands
				changed = true
			}
		case acp.SessionUpdateConfigOptionUpdate:
			if len(update.ConfigOptions) > 0 {
				state.ConfigOptions = update.ConfigOptions
				changed = true
			}
		case acp.SessionUpdateSessionInfoUpdate:
			if update.Title != "" {
				state.Title = update.Title
				changed = true
			}
			if update.UpdatedAt != "" {
				state.UpdatedAt = update.UpdatedAt
				changed = true
			}
		}
		s.mu.Unlock()
	}

	addToolCallID, doneToolCallID := trackToolCallUpdate(update)
	if addToolCallID != "" || doneToolCallID != "" {
		s.mu.Lock()
		if addToolCallID != "" {
			s.prompt.activeTCs[addToolCallID] = struct{}{}
		}
		if doneToolCallID != "" {
			delete(s.prompt.activeTCs, doneToolCallID)
		}
		s.mu.Unlock()
	}
	if changed {
		s.persistSessionBestEffort()
	}

	if ch == nil {
		return
	}
	if promptCtx == nil {
		select {
		case ch <- params:
		default:
		}
		return
	}
	select {
	case ch <- params:
	case <-promptCtx.Done():
	}
}

func trackToolCallUpdate(update acp.SessionUpdate) (addToolCallID string, doneToolCallID string) {
	toolCallID := strings.TrimSpace(update.ToolCallID)
	if toolCallID == "" {
		return "", ""
	}

	isDoneStatus := update.Status == acp.ToolCallStatusCompleted || update.Status == acp.ToolCallStatusFailed
	switch update.SessionUpdate {
	case acp.SessionUpdateToolCall:
		if isDoneStatus {
			return "", toolCallID
		}
		return toolCallID, ""
	case acp.SessionUpdateToolCallUpdate:
		if isDoneStatus {
			return "", toolCallID
		}
	}
	return "", ""
}

// SessionRequestPermission responds to session/request_permission agent requests.
func (s *Session) SessionRequestPermission(ctx context.Context, requestID int64, params acp.PermissionRequestParams) (acp.PermissionResult, error) {
	s.mu.Lock()
	pCtx := s.prompt.ctx
	name := s.currentAgentNameLocked()
	options := []acp.ConfigOption(nil)
	if state := s.agents[name]; state != nil {
		options = append(options, state.ConfigOptions...)
	}
	s.mu.Unlock()
	if pCtx != nil {
		ctx = pCtx
	}
	snap := acp.SessionConfigSnapshotFromOptions(options)
	return s.decidePermission(ctx, requestID, params, snap.Mode)
}
