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
	instance   agent.Instance
	agentType  string
	agentState SessionAgentState

	// Runtime ACP session state (moved from Client.session / Client.sessionMeta / Client.initMeta).
	acpSessionID string
	ready        bool
	initializing bool
	lastReply    string

	prompt   promptState
	initCond *sync.Cond

	timeoutLimiter *timeoutNotifyLimiter

	// Back-references to Client-owned resources needed by Session methods.
	projectName string
	cwd         string
	registry    *agent.ACPFactory
	store       Store
	imRouter    IMRouter
	viewSink    SessionViewSink
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
		cwd:       cwd,
		createdAt: time.Now(),
		prompt: promptState{
			activeTCs: make(map[string]struct{}),
		},
		timeoutLimiter: newTimeoutNotifyLimiter(timeoutNotifyCooldown),
	}
	s.initCond = sync.NewCond(&s.mu)
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

func (s *Session) agentStateLocked(name string) *SessionAgentState {
	name = strings.TrimSpace(name)
	if name == "" {
		name = strings.TrimSpace(s.agentType)
	}
	if name == "" {
		return nil
	}
	if strings.TrimSpace(s.agentType) == "" {
		s.agentType = name
	}
	return &s.agentState
}

func (s *Session) currentAgentNameLocked() string {
	if s.instance != nil && strings.TrimSpace(s.instance.Name()) != "" {
		return strings.TrimSpace(s.instance.Name())
	}
	if strings.TrimSpace(s.agentType) != "" {
		return strings.TrimSpace(s.agentType)
	}
	return ""
}

func (s *Session) currentAgentStateSnapshot() (*SessionAgentState, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	name := s.currentAgentNameLocked()
	if name == "" {
		return nil, ""
	}
	return cloneSessionAgentState(&s.agentState), name
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

// shortSessionID returns a compact display form of a session ID.
// e.g. "sess-12345678-1234-1234-1234-123456789012" -> "sess-12345678"
func shortSessionID(id string) string {
	const prefix = "sess-"
	if strings.HasPrefix(id, prefix) {
		rest := id[len(prefix):]
		if idx := strings.Index(rest, "-"); idx > 0 {
			return prefix + rest[:idx]
		}
	}
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

// sessionInfoLine returns a multi-line summary of the current session state.
func (s *Session) sessionInfoLine() string {
	s.mu.Lock()
	agentName := s.currentAgentNameLocked()
	clientSID := s.ID
	var configOpts []acp.ConfigOption
	if state := s.agentStateLocked(agentName); state != nil {
		configOpts = append([]acp.ConfigOption(nil), state.ConfigOptions...)
	}
	s.mu.Unlock()

	snap := acp.SessionConfigSnapshotFromOptions(configOpts)
	sid := shortSessionID(clientSID)
	if sid == "" {
		sid = "none"
	}
	return fmt.Sprintf("session: %s\nagent: %s\nmode: %s\nmodel: %s",
		sid,
		renderUnknown(agentName),
		renderUnknown(snap.Mode),
		renderUnknown(snap.Model),
	)
}

// reply sends a text response to the active chat via the IM channel.
func (s *Session) reply(text string) {
	s.replyWithTitle("", text)
}

// replyWithTitle sends a system message with an optional card title.
func (s *Session) replyWithTitle(title, body string) {
	messageText := strings.TrimSpace(body)
	if strings.TrimSpace(title) != "" && strings.TrimSpace(body) != "" {
		messageText = strings.TrimSpace(title) + "\n" + strings.TrimSpace(body)
	} else if strings.TrimSpace(title) != "" {
		messageText = strings.TrimSpace(title)
	}
	if messageText != "" {
		s.recordSessionViewEvent(SessionViewEvent{
			Type:    SessionViewEventTypeSystem,
			Content: messageText,
		})
	}
	if router, source, ok := s.imContext(); ok {
		_ = router.SystemNotify(context.Background(), im.SendTarget{SessionID: s.ID, Source: &source}, im.SystemPayload{
			Kind:  "message",
			Title: title,
			Body:  body,
		})
		return
	}
	if title != "" {
		fmt.Println(title)
	}
	fmt.Println(body)
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

func (s *Session) recordSessionViewEvent(event SessionViewEvent) {
	if s.viewSink == nil {
		return
	}
	if strings.TrimSpace(event.SessionID) == "" {
		event.SessionID = s.ID
	}
	if event.UpdatedAt.IsZero() {
		event.UpdatedAt = time.Now().UTC()
	}
	if router, source, ok := s.imContext(); ok && router != nil {
		event.SourceChannel = source.ChannelID
		event.SourceChatID = source.ChatID
	}
	_ = s.viewSink.RecordEvent(context.Background(), event)
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
	if state := s.agentStateLocked(name); state != nil && strings.TrimSpace(state.ACPSessionID) != "" {
		savedSID = strings.TrimSpace(state.ACPSessionID)
	}
	cwd := s.cwd
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
	if state := s.agentStateLocked(name); state != nil && strings.TrimSpace(state.ACPSessionID) != "" {
		savedSID = strings.TrimSpace(state.ACPSessionID)
	}
	s.mu.Unlock()

	inst, err := creator(ctx, cwd)
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
	s.agentType = name
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
	var persistedConfigOptions []acp.ConfigOption
	var persistedCommands []acp.AvailableCommand
	if state := s.agentStateLocked(agentName); state != nil {
		persistedConfigOptions = append([]acp.ConfigOption(nil), state.ConfigOptions...)
		persistedCommands = append([]acp.AvailableCommand(nil), state.Commands...)
	}
	s.mu.Unlock()

	notifyDone := func() {
		s.mu.Lock()
		s.initializing = false
		s.mu.Unlock()
		s.initCond.Broadcast()
	}

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

	baseline := s.loadAgentPreferenceState(agentName)
	baselineSnap := acp.SessionConfigSnapshotFromOptions(baseline.ConfigOptions)
	baselineCommands := append([]acp.AvailableCommand(nil), baseline.AvailableCommands...)

	if savedSID != "" && initResult.AgentCapabilities.LoadSession {
		loadResult, loadErr := inst.SessionLoad(ctx, acp.SessionLoadParams{
			SessionID:  savedSID,
			CWD:        cwd,
			MCPServers: emptyMCPServers(),
		})
		if loadErr == nil {
			resolved := append([]acp.ConfigOption(nil), loadResult.ConfigOptions...)
			savedSessionSnap := acp.SessionConfigSnapshotFromOptions(persistedConfigOptions)
			if len(resolved) > 0 {
				resolved = applyReplayableConfigBaseline(ctx, s.projectName, inst, savedSID, resolved, savedSessionSnap)
			} else if len(persistedConfigOptions) > 0 {
				resolved = append([]acp.ConfigOption(nil), persistedConfigOptions...)
			}
			resolvedCommands := append([]acp.AvailableCommand(nil), persistedCommands...)
			if len(resolvedCommands) == 0 && len(baselineCommands) > 0 {
				resolvedCommands = append([]acp.AvailableCommand(nil), baselineCommands...)
			}

			resolvedSnap := acp.SessionConfigSnapshotFromOptions(resolved)
			missing := acp.SessionConfigSnapshot{}
			if strings.TrimSpace(resolvedSnap.Mode) == "" {
				missing.Mode = strings.TrimSpace(baselineSnap.Mode)
			}
			if strings.TrimSpace(resolvedSnap.Model) == "" {
				missing.Model = strings.TrimSpace(baselineSnap.Model)
			}
			if strings.TrimSpace(resolvedSnap.ThoughtLevel) == "" {
				missing.ThoughtLevel = strings.TrimSpace(baselineSnap.ThoughtLevel)
			}
			if missing.Mode != "" || missing.Model != "" || missing.ThoughtLevel != "" {
				resolved = mergeConfigOptions(resolved, baseline.ConfigOptions)
				resolved = applyReplayableConfigBaseline(ctx, s.projectName, inst, savedSID, resolved, missing)
			}

			s.mu.Lock()
			state := s.agentStateLocked(agentName)
			state.ACPSessionID = savedSID
			if len(resolved) > 0 {
				state.ConfigOptions = append([]acp.ConfigOption(nil), resolved...)
			}
			state.ProtocolVersion = initResult.ProtocolVersion.String()
			state.AgentCapabilities = initResult.AgentCapabilities
			state.AgentInfo = cloneAgentInfo(initResult.AgentInfo)
			state.AuthMethods = initResult.AuthMethods
			state.Commands = append([]acp.AvailableCommand(nil), resolvedCommands...)
			commands := append([]acp.AvailableCommand(nil), state.Commands...)
			s.agentType = agentName
			s.acpSessionID = savedSID
			s.ready = true
			s.initializing = false
			s.mu.Unlock()
			s.initCond.Broadcast()
			s.persistAgentPreferenceState(agentName, resolved, commands)
			hubLogger(s.projectName).Info("connected agent=%s session=%s resumed", inst.Name(), savedSID)
			return nil
		}
		hubLogger(s.projectName).Warn("session/load failed agent=%s session=%s err=%v; fallback to session/new", inst.Name(), savedSID, loadErr)
	}

	newResult, err := inst.SessionNew(ctx, acp.SessionNewParams{
		CWD:        cwd,
		MCPServers: emptyMCPServers(),
	})
	if err != nil {
		notifyDone()
		return fmt.Errorf("ensureReady: session/new: %w", err)
	}

	resolved := append([]acp.ConfigOption(nil), newResult.ConfigOptions...)
	desiredSnap := acp.SessionConfigSnapshotFromOptions(persistedConfigOptions)
	if strings.TrimSpace(desiredSnap.Mode) == "" {
		desiredSnap.Mode = strings.TrimSpace(baselineSnap.Mode)
	}
	if strings.TrimSpace(desiredSnap.Model) == "" {
		desiredSnap.Model = strings.TrimSpace(baselineSnap.Model)
	}
	if strings.TrimSpace(desiredSnap.ThoughtLevel) == "" {
		desiredSnap.ThoughtLevel = strings.TrimSpace(baselineSnap.ThoughtLevel)
	}
	if desiredSnap.Mode != "" || desiredSnap.Model != "" || desiredSnap.ThoughtLevel != "" {
		resolved = applyReplayableConfigBaseline(ctx, s.projectName, inst, newResult.SessionID, resolved, desiredSnap)
	}
	resolvedCommands := append([]acp.AvailableCommand(nil), persistedCommands...)
	if len(resolvedCommands) == 0 && len(baselineCommands) > 0 {
		resolvedCommands = append([]acp.AvailableCommand(nil), baselineCommands...)
	}

	s.mu.Lock()
	state := s.agentStateLocked(agentName)
	state.ACPSessionID = newResult.SessionID
	state.ConfigOptions = append([]acp.ConfigOption(nil), resolved...)
	state.Commands = append([]acp.AvailableCommand(nil), resolvedCommands...)
	state.Title = ""
	state.UpdatedAt = ""
	state.ProtocolVersion = initResult.ProtocolVersion.String()
	state.AgentCapabilities = initResult.AgentCapabilities
	state.AgentInfo = cloneAgentInfo(initResult.AgentInfo)
	state.AuthMethods = initResult.AuthMethods
	commands := append([]acp.AvailableCommand(nil), state.Commands...)
	s.agentType = agentName
	s.ID = newResult.SessionID
	s.acpSessionID = newResult.SessionID
	s.ready = true
	s.initializing = false
	s.mu.Unlock()
	s.initCond.Broadcast()

	modeID := ""
	for _, opt := range resolved {
		if opt.ID == acp.ConfigOptionIDMode || strings.EqualFold(opt.Category, acp.ConfigOptionCategoryMode) {
			modeID = opt.CurrentValue
			break
		}
	}
	hubLogger(s.projectName).Info("connected agent=%s session=%s mode=%s", inst.Name(), newResult.SessionID, modeID)
	s.persistAgentPreferenceState(agentName, resolved, commands)
	return nil
}
func findConfigOptionID(options []acp.ConfigOption, id, category string) string {
	id = strings.TrimSpace(id)
	category = strings.TrimSpace(category)
	for _, opt := range options {
		if strings.EqualFold(strings.TrimSpace(opt.ID), id) && strings.TrimSpace(opt.ID) != "" {
			return strings.TrimSpace(opt.ID)
		}
	}
	for _, opt := range options {
		if strings.EqualFold(strings.TrimSpace(opt.Category), category) && strings.TrimSpace(opt.ID) != "" {
			return strings.TrimSpace(opt.ID)
		}
	}
	return ""
}

func mergeConfigOptions(current []acp.ConfigOption, updated []acp.ConfigOption) []acp.ConfigOption {
	if len(updated) == 0 {
		return append([]acp.ConfigOption(nil), current...)
	}
	merged := append([]acp.ConfigOption(nil), current...)
	for _, next := range updated {
		replaced := false
		for i, existing := range merged {
			sameID := strings.TrimSpace(next.ID) != "" && strings.EqualFold(strings.TrimSpace(existing.ID), strings.TrimSpace(next.ID))
			sameCategory := strings.TrimSpace(next.ID) == "" && strings.TrimSpace(next.Category) != "" && strings.EqualFold(strings.TrimSpace(existing.Category), strings.TrimSpace(next.Category))
			if sameID || sameCategory {
				merged[i] = next
				replaced = true
				break
			}
		}
		if !replaced {
			merged = append(merged, next)
		}
	}
	return merged
}

type replayableConfigTarget struct {
	label    string
	id       string
	category string
	value    string
}

func replayableTargetsFromSnapshot(snap acp.SessionConfigSnapshot) []replayableConfigTarget {
	return []replayableConfigTarget{
		{label: "mode", id: acp.ConfigOptionIDMode, category: acp.ConfigOptionCategoryMode, value: strings.TrimSpace(snap.Mode)},
		{label: "model", id: acp.ConfigOptionIDModel, category: acp.ConfigOptionCategoryModel, value: strings.TrimSpace(snap.Model)},
		{label: "thought_level", id: acp.ConfigOptionIDThoughtLevel, category: acp.ConfigOptionCategoryThoughtLv, value: strings.TrimSpace(snap.ThoughtLevel)},
	}
}

func currentReplayableValue(label string, snap acp.SessionConfigSnapshot) string {
	switch label {
	case "mode":
		return strings.TrimSpace(snap.Mode)
	case "model":
		return strings.TrimSpace(snap.Model)
	case "thought_level":
		return strings.TrimSpace(snap.ThoughtLevel)
	default:
		return ""
	}
}

func applyReplayableConfigBaseline(
	ctx context.Context,
	projectName string,
	inst agent.Instance,
	sessionID string,
	current []acp.ConfigOption,
	desired acp.SessionConfigSnapshot,
) []acp.ConfigOption {
	options := append([]acp.ConfigOption(nil), current...)
	for _, target := range replayableTargetsFromSnapshot(desired) {
		if target.value == "" {
			continue
		}
		currentSnap := acp.SessionConfigSnapshotFromOptions(options)
		if strings.EqualFold(currentReplayableValue(target.label, currentSnap), target.value) {
			continue
		}

		configID := findConfigOptionID(options, target.id, target.category)
		if configID == "" {
			hubLogger(projectName).Warn("skip reapply %s: config option not found", target.label)
			continue
		}

		updated, err := inst.SessionSetConfigOption(ctx, acp.SessionSetConfigOptionParams{
			SessionID: sessionID,
			ConfigID:  configID,
			Value:     target.value,
		})
		if err != nil {
			hubLogger(projectName).Warn("reapply %s failed session=%s value=%s err=%v",
				target.label, sessionID, target.value, err)
			continue
		}
		if len(updated) > 0 {
			options = mergeConfigOptions(options, updated)
		}
	}
	return options
}

// ensureReadyAndNotify calls ensureReady and only emits a ready message when
// transitioning from not-ready to ready.
func (s *Session) ensureReadyAndNotify(ctx context.Context) error {
	s.mu.Lock()
	wasReady := s.ready
	s.mu.Unlock()

	if err := s.ensureReady(ctx); err != nil {
		return err
	}

	if !wasReady {
		s.replyWithTitle("Session ready", s.sessionInfoLine())
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
func (s *Session) promptStream(ctx context.Context, blocks []acp.ContentBlock) (<-chan promptStreamEvent, error) {
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

	updates := make(chan promptStreamEvent, 32)
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
				Prompt:    blocks,
			})
			resultCh <- promptResult{result: res, err: err}
		}()

		drain := func(params acp.SessionUpdateParams) bool {
			copied := params
			if copied.Update.SessionUpdate == acp.SessionUpdateAgentMessageChunk {
				var content acp.ContentBlock
				if len(copied.Update.Content) > 0 && json.Unmarshal(copied.Update.Content, &content) == nil && content.Type == acp.ContentBlockTypeText {
					replyMu.Lock()
					replyBuf.WriteString(content.Text)
					replyMu.Unlock()
				}
			}
			select {
			case updates <- promptStreamEvent{update: &copied}:
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

		if err != nil {
			select {
			case updates <- promptStreamEvent{err: err}:
			case <-ctx.Done():
			}
		} else {
			final := result
			select {
			case updates <- promptStreamEvent{result: &final}:
			case <-ctx.Done():
			}
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
				Status:        acp.ToolCallStatusCancelled,
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

	agentType := strings.TrimSpace(s.agentType)
	if agentType == "" {
		agentType = s.currentAgentNameLocked()
	}
	if agentType == "" {
		return nil, fmt.Errorf("session agent type is required")
	}

	state := cloneSessionAgentState(s.agentStateLocked(agentType))
	title := ""
	if state != nil {
		title = strings.TrimSpace(state.Title)
	}

	agentJSON := "{}"
	if state != nil {
		state.ACPSessionID = ""
		raw, err := json.Marshal(state)
		if err != nil {
			return nil, fmt.Errorf("marshal agent_json: %w", err)
		}
		agentJSON = string(raw)
	}

	return &SessionRecord{
		ID:           s.ID,
		ProjectName:  s.projectName,
		Status:       s.Status,
		AgentType:    agentType,
		AgentJSON:    agentJSON,
		Title:        title,
		CreatedAt:    s.createdAt,
		LastActiveAt: s.lastActiveAt,
	}, nil
}

func sessionFromRecord(rec *SessionRecord, cwd string) (*Session, error) {
	s := newSession(rec.ID, cwd)
	s.Status = rec.Status
	s.agentType = strings.TrimSpace(rec.AgentType)
	if s.agentType == "" {
		return nil, fmt.Errorf("session %q missing agent type", rec.ID)
	}
	s.acpSessionID = strings.TrimSpace(rec.ID)
	s.createdAt = rec.CreatedAt
	s.lastActiveAt = rec.LastActiveAt
	if strings.TrimSpace(rec.AgentJSON) != "" {
		if err := json.Unmarshal([]byte(rec.AgentJSON), &s.agentState); err != nil {
			return nil, fmt.Errorf("unmarshal agent_json: %w", err)
		}
	}
	s.agentState.ACPSessionID = s.acpSessionID
	if strings.TrimSpace(rec.Title) != "" {
		if state := s.agentStateLocked(s.agentType); state != nil && strings.TrimSpace(state.Title) == "" {
			state.Title = strings.TrimSpace(rec.Title)
		}
	}
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

func (s *Session) loadAgentPreferenceState(agentName string) ProjectAgentState {
	if s.store == nil || strings.TrimSpace(agentName) == "" {
		return ProjectAgentState{}
	}
	rec, err := s.store.LoadAgentPreference(context.Background(), s.projectName, strings.TrimSpace(agentName))
	if err != nil || rec == nil || strings.TrimSpace(rec.PreferenceJSON) == "" {
		return ProjectAgentState{}
	}
	var pref ProjectAgentState
	if err := json.Unmarshal([]byte(rec.PreferenceJSON), &pref); err != nil {
		hubLogger(s.projectName).Warn("decode agent preference failed agent=%s err=%v", agentName, err)
		return ProjectAgentState{}
	}
	return pref
}

func (s *Session) persistAgentPreferenceState(agentName string, configOptions []acp.ConfigOption, commands []acp.AvailableCommand) {
	if s.store == nil || strings.TrimSpace(agentName) == "" {
		return
	}
	next := s.loadAgentPreferenceState(agentName)
	if configOptions != nil {
		next.ConfigOptions = append([]acp.ConfigOption(nil), configOptions...)
	}
	if commands != nil {
		next.AvailableCommands = append([]acp.AvailableCommand(nil), commands...)
	}
	next.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	raw, err := json.Marshal(next)
	if err != nil {
		hubLogger(s.projectName).Warn("encode agent preference failed agent=%s err=%v", agentName, err)
		return
	}
	if err := s.store.SaveAgentPreference(context.Background(), AgentPreferenceRecord{
		ProjectName:    s.projectName,
		AgentType:      strings.TrimSpace(agentName),
		PreferenceJSON: string(raw),
	}); err != nil {
		hubLogger(s.projectName).Warn("save agent preference failed agent=%s err=%v", agentName, err)
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
		s.mu.Lock()
		agentName := s.currentAgentNameLocked()
		state := cloneSessionAgentState(s.agentStateLocked(agentName))
		s.mu.Unlock()
		if state != nil {
			s.persistAgentPreferenceState(agentName, state.ConfigOptions, state.Commands)
		}
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
	_ = ctx
	_ = requestID

	// Prefer persistent allow semantics, then one-shot allow semantics.
	fallback := ""
	for _, option := range params.Options {
		kind := strings.ToLower(strings.TrimSpace(option.Kind))
		optionID := strings.TrimSpace(option.OptionID)
		name := strings.ToLower(strings.TrimSpace(option.Name))
		if optionID == "" {
			continue
		}
		if kind == "allow_always" || kind == "always" {
			return acp.PermissionResult{Outcome: "selected", OptionID: optionID}, nil
		}
		if fallback == "" && (kind == "allow_once" || kind == "allow" || kind == "once" || strings.HasPrefix(kind, "allow") || strings.EqualFold(optionID, "allow") || strings.Contains(name, "allow")) {
			fallback = optionID
		}
	}
	if fallback != "" {
		return acp.PermissionResult{Outcome: "selected", OptionID: fallback}, nil
	}
	return acp.PermissionResult{Outcome: "cancelled"}, nil
}

func normalizeSessionPromptBlocks(blocks []acp.ContentBlock) []acp.ContentBlock {
	if len(blocks) == 0 {
		return nil
	}
	out := make([]acp.ContentBlock, 0, len(blocks))
	for _, block := range blocks {
		kind := strings.TrimSpace(block.Type)
		if kind == "" {
			continue
		}
		if kind == acp.ContentBlockTypeText {
			text := strings.TrimSpace(block.Text)
			if text == "" {
				continue
			}
			block.Text = text
		}
		block.Type = kind
		out = append(out, block)
	}
	return out
}

// handlePrompt sends text to the active (or lazily initialized) session and streams the reply.
// promptMu is held for the full duration, serializing with switchAgent.
func (s *Session) handlePrompt(text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	s.handlePromptBlocks([]acp.ContentBlock{{Type: acp.ContentBlockTypeText, Text: text}})
}

// handlePromptBlocks sends content blocks to the active (or lazily initialized) session.
// promptMu is held for the full duration, serializing with switchAgent.
func (s *Session) handlePromptBlocks(blocks []acp.ContentBlock) {
	blocks = normalizeSessionPromptBlocks(blocks)
	if len(blocks) == 0 {
		return
	}
	s.recordSessionViewEvent(SessionViewEvent{
		Type:      SessionViewEventTypeACP,
		SessionID: s.ID,
		Content: acp.BuildACPContentJSON(acp.MethodSessionPrompt, map[string]any{
			"params": acp.SessionPromptParams{
				SessionID: s.ID,
				Prompt:    cloneSessionContentBlocks(blocks),
			},
		}),
	})
	s.promptMu.Lock()
	defer s.promptMu.Unlock()
	ctx := context.Background()
	for attempt := 1; attempt <= 2; attempt++ {
		if err := s.ensureInstance(ctx); err != nil {
			s.reply(fmt.Sprintf("No active session: %v. %s", err, s.connectHint()))
			return
		}

		if err := s.ensureReadyAndNotify(ctx); err != nil {
			if attempt == 1 && s.shouldReconnectOnRecoverableErr(err) {
				s.reply("Agent init failed transiently, reconnecting and retrying once...")
				s.forceReconnect()
				continue
			}
			s.reply(fmt.Sprintf("No active session: %v. %s", err, s.connectHint()))
			return
		}

		updates, err := s.promptStream(ctx, blocks)
		if err != nil {
			if attempt == 1 && s.shouldReconnectOnRecoverableErr(err) {
				s.reply("Agent disconnected, reconnecting and retrying once...")
				s.forceReconnect()
				continue
			}
			if isAgentExitError(err) && !s.agentProcessAlive() {
				_ = s.resetDeadConnection(err)
			}
			s.reply(fmt.Sprintf("Prompt error: %v", err))
			return
		}

		s.mu.Lock()
		s.prompt.currentCh = updates
		s.mu.Unlock()

		var buf strings.Builder
		imRouter, imSource, hasIMEmitter := s.imContext()

		sawSandboxRefresh := false
		sawText := false
		observe := newPromptObserveState(time.Now())
		observeTicker := time.NewTicker(promptObserveInterval)

		retryStream := false
		streamDone := false
		for !streamDone {
			select {
			case ev, ok := <-updates:
				if !ok {
					streamDone = true
					break
				}
				observe.MarkActivity(time.Now(), true)
				if ev.err != nil {
					if attempt == 1 && isUnsupportedReasoningEffortError(ev.err) && s.tryCopilotReasoningFallback(ctx) {
						s.reply("Detected incompatible reasoning settings, switched model and retrying once...")
						retryStream = true
						break
					}
					if attempt == 1 && !sawText && s.shouldReconnectOnRecoverableErr(ev.err) {
						s.reply("Agent disconnected during stream, reconnecting and retrying once...")
						s.forceReconnect()
						retryStream = true
						break
					}
					recovered := false
					if !s.agentProcessAlive() && s.resetDeadConnection(ev.err) {
						if recErr := s.ensureInstance(ctx); recErr == nil {
							_ = s.ensureReadyAndNotify(ctx)
							recovered = true
						}
					}
					if recovered {
						s.reply("Agent process exited and was reconnected. Please resend if this reply was interrupted.")
					} else {
						s.reply(fmt.Sprintf("Agent error: %v", ev.err))
					}
					s.mu.Lock()
					s.prompt.currentCh = nil
					s.mu.Unlock()
					observeTicker.Stop()
					return
				}
				if ev.update != nil {
					params := *ev.update
					s.recordSessionViewEvent(SessionViewEvent{
						Type:      SessionViewEventTypeACP,
						SessionID: s.ID,
						Content: acp.BuildACPContentJSON(acp.MethodSessionUpdate, map[string]any{
							"params": params,
						}),
					})
					if hasIMEmitter {
						target := im.SendTarget{SessionID: s.ID, Source: &imSource}
						if emitErr := imRouter.PublishSessionUpdate(ctx, target, params); emitErr != nil {
							s.reply(fmt.Sprintf("IM emit error: %v", emitErr))
						}
					}
					if hasSandboxRefreshUpdate(params.Update) {
						sawSandboxRefresh = true
					}
					if params.Update.SessionUpdate == acp.SessionUpdateAgentMessageChunk {
						text := extractTextChunk(params.Update.Content)
						if strings.TrimSpace(text) != "" {
							sawText = true
							if !hasIMEmitter {
								buf.WriteString(text)
							}
						}
					}
					if params.Update.SessionUpdate == acp.SessionUpdateConfigOptionUpdate && !hasIMEmitter {
						raw, _ := json.Marshal(params.Update)
						s.reply(formatConfigOptionUpdateMessage(raw))
						s.persistSessionBestEffort()
					}
				}
				if ev.result != nil {
					s.recordSessionViewEvent(SessionViewEvent{
						Type:      SessionViewEventTypeACP,
						SessionID: s.ID,
						Content: acp.BuildACPContentJSON(acp.MethodSessionPrompt, map[string]any{
							"result": *ev.result,
						}),
					})
					if hasIMEmitter {
						target := im.SendTarget{SessionID: s.ID, Source: &imSource}
						if emitErr := imRouter.PublishPromptResult(ctx, target, *ev.result); emitErr != nil {
							s.reply(fmt.Sprintf("IM emit error: %v", emitErr))
						}
					}
					streamDone = true
				}
			case <-observeTicker.C:
				ev := observe.Eval(time.Now(), observe.Started())
				if ev.WarnFirstWait {
					hubLogger(s.projectName).Warn("timeout warn category=timeout stage=stream kind=first_wait session=%s", s.ID)
				}
				if ev.ErrorFirstWait {
					s.reportTimeoutError("stream", "first_wait")
				}
				if ev.WarnSilence {
					hubLogger(s.projectName).Warn("timeout warn category=timeout stage=stream kind=silence session=%s", s.ID)
				}
				if ev.ErrorSilence {
					s.reportTimeoutError("stream", "silence")
				}
			}
		}
		observeTicker.Stop()
		if retryStream {
			continue
		}

		s.mu.Lock()
		s.prompt.currentCh = nil
		s.mu.Unlock()

		s.persistSessionBestEffort()

		if !hasIMEmitter && buf.Len() > 0 {
			s.reply(buf.String())
		}

		if attempt == 1 && sawSandboxRefresh && !sawText && !s.agentProcessAlive() {
			s.reply("Detected sandbox refresh failure, reconnecting and retrying once...")
			s.forceReconnect()
			continue
		}
		return
	}
}

func extractTextChunk(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var anyValue any
	if err := json.Unmarshal(raw, &anyValue); err != nil {
		return ""
	}
	return extractTextFromAny(anyValue)
}

func extractTextFromAny(v any) string {
	switch value := v.(type) {
	case string:
		return value
	case map[string]any:
		if text, ok := value["text"].(string); ok && strings.TrimSpace(text) != "" {
			return text
		}
		if delta, ok := value["delta"].(string); ok && strings.TrimSpace(delta) != "" {
			return delta
		}
		if content, ok := value["content"]; ok {
			if text := extractTextFromAny(content); strings.TrimSpace(text) != "" {
				return text
			}
		}
		if resource, ok := value["resource"]; ok {
			if text := extractTextFromAny(resource); strings.TrimSpace(text) != "" {
				return text
			}
		}
		return ""
	case []any:
		var builder strings.Builder
		for _, item := range value {
			builder.WriteString(extractTextFromAny(item))
		}
		return builder.String()
	default:
		return ""
	}
}

func renderSessionToolStatus(update acp.SessionUpdate) string {
	title := strings.TrimSpace(update.Title)
	if title == "" {
		title = strings.TrimSpace(update.Kind)
	}
	status := strings.TrimSpace(update.Status)
	if title == "" {
		return status
	}
	if status == "" {
		return title
	}
	return title + " - " + status
}

func renderUnknown(v string) string {
	if strings.TrimSpace(v) == "" {
		return "unknown"
	}
	return v
}

func (s *Session) reportTimeoutError(stage string, kind string) {
	now := time.Now()
	s.mu.Lock()
	agent := s.currentAgentNameLocked()
	sid := s.acpSessionID
	allow := true
	if s.timeoutLimiter != nil {
		allow = s.timeoutLimiter.Allow(kind, now)
	}
	s.mu.Unlock()

	hubLogger(s.projectName).Error("timeout error category=timeout stage=%s kind=%s agent=%s session=%s",
		stage, kind, renderUnknown(agent), renderUnknown(sid))
	if !allow {
		return
	}

	router, source, ok := s.imContext()
	if !ok {
		return
	}
	body := fmt.Sprintf(
		"category=timeout stage=%s agent=%s sessionID=%s action=check /status then retry",
		stage,
		renderUnknown(agent),
		renderUnknown(sid),
	)
	if err := router.SystemNotify(
		context.Background(),
		im.SendTarget{SessionID: s.ID, Source: &source},
		im.SystemPayload{Kind: "message", Body: body},
	); err != nil {
		hubLogger(s.projectName).Error("timeout im notify failed stage=%s kind=%s session=%s err=%v",
			stage, kind, s.ID, err)
	}
}

func (s *Session) connectHint() string {
	agentName := s.preferredAgentName()
	if agentName == "" {
		if s.registry != nil {
			names := s.registry.Names()
			if len(names) > 0 {
				return fmt.Sprintf("Run `/new <agent>` to connect. Available: %s", strings.Join(names, ", "))
			}
		}
		return "No available ACP provider. Check environment and restart wheelmaker."
	}
	return fmt.Sprintf("Run `%s` to connect.", "/new "+agentName)
}

func (s *Session) preferredAgentName() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.instance != nil && strings.TrimSpace(s.instance.Name()) != "" {
		return strings.TrimSpace(s.instance.Name())
	}
	if strings.TrimSpace(s.agentType) != "" {
		return strings.TrimSpace(s.agentType)
	}
	if s.registry != nil {
		return strings.TrimSpace(s.registry.PreferredName())
	}
	return ""
}

type instanceLivenessProbe interface {
	Alive() bool
}

func (s *Session) agentProcessAlive() bool {
	s.mu.Lock()
	inst := s.instance
	s.mu.Unlock()
	if inst == nil {
		return false
	}
	probe, ok := inst.(instanceLivenessProbe)
	if !ok {
		return true
	}
	return probe.Alive()
}

func (s *Session) shouldReconnectOnRecoverableErr(err error) bool {
	if !isAgentRecoverableRuntimeErr(err) {
		return false
	}
	return !s.agentProcessAlive()
}

func isAgentExitError(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(strings.TrimSpace(err.Error()))
	if s == "" {
		return false
	}
	if strings.Contains(s, "tls handshake eof") {
		return false
	}
	if s == "eof" || strings.HasSuffix(s, ": eof") {
		return true
	}
	return strings.Contains(s, "agent process exited") ||
		strings.Contains(s, "process exited") ||
		strings.Contains(s, "conn is closed") ||
		strings.Contains(s, "broken pipe") ||
		strings.Contains(s, "connection reset") ||
		strings.Contains(s, "process stdout closed")
}

func isAgentRecoverableRuntimeErr(err error) bool {
	if err == nil {
		return false
	}
	return isAgentExitError(err) || isSandboxRefreshErr(err)
}

func isUnsupportedReasoningEffortError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	if msg == "" || !strings.Contains(msg, "reasoning_effort") {
		return false
	}
	hints := []string{"unrecognized request argument", "unknown field", "not supported", "unsupported", "invalid request"}
	for _, hint := range hints {
		if strings.Contains(msg, hint) {
			return true
		}
	}
	return false
}

func pickAlternativeModelValue(opt *acp.ConfigOption) string {
	if opt == nil || len(opt.Options) == 0 {
		return ""
	}
	current := strings.TrimSpace(opt.CurrentValue)
	for _, candidate := range opt.Options {
		value := strings.TrimSpace(candidate.Value)
		if value == "" || strings.EqualFold(value, current) {
			continue
		}
		return value
	}
	return ""
}

func (s *Session) tryCopilotReasoningFallback(ctx context.Context) bool {
	s.mu.Lock()
	if s.instance == nil || !strings.EqualFold(strings.TrimSpace(s.instance.Name()), string(acp.ACPProviderCopilot)) {
		s.mu.Unlock()
		return false
	}
	sid := strings.TrimSpace(s.acpSessionID)
	configOptions := []acp.ConfigOption(nil)
	if state := s.agentStateLocked(s.currentAgentNameLocked()); state != nil {
		configOptions = append(configOptions, state.ConfigOptions...)
	}
	s.mu.Unlock()

	if sid == "" {
		return false
	}

	var modelOpt *acp.ConfigOption
	for i := range configOptions {
		opt := &configOptions[i]
		if strings.EqualFold(strings.TrimSpace(opt.ID), acp.ConfigOptionIDModel) || strings.EqualFold(strings.TrimSpace(opt.Category), acp.ConfigOptionCategoryModel) {
			modelOpt = opt
			break
		}
	}
	target := pickAlternativeModelValue(modelOpt)
	if target == "" {
		return false
	}

	updatedOpts, err := s.instance.SessionSetConfigOption(ctx, acp.SessionSetConfigOptionParams{SessionID: sid, ConfigID: modelOpt.ID, Value: target})
	if err != nil {
		return false
	}

	s.mu.Lock()
	if len(updatedOpts) > 0 {
		if state := s.agentStateLocked(s.currentAgentNameLocked()); state != nil {
			state.ConfigOptions = append([]acp.ConfigOption(nil), updatedOpts...)
		}
	}
	s.mu.Unlock()
	s.persistSessionBestEffort()
	return true
}

func (s *Session) resetDeadConnection(err error) bool {
	if !isAgentExitError(err) {
		return false
	}
	s.mu.Lock()
	old := s.instance
	s.instance = nil
	s.ready = false
	s.initializing = false
	s.prompt.ctx = nil
	s.prompt.cancel = nil
	s.prompt.updatesCh = nil
	s.prompt.currentCh = nil
	s.prompt.activeTCs = make(map[string]struct{})
	s.mu.Unlock()
	if old != nil {
		_ = old.Close()
	}
	return true
}

func (s *Session) forceReconnect() {
	s.mu.Lock()
	old := s.instance
	s.instance = nil
	s.ready = false
	s.initializing = false
	s.prompt.ctx = nil
	s.prompt.cancel = nil
	s.prompt.updatesCh = nil
	s.prompt.currentCh = nil
	s.prompt.activeTCs = make(map[string]struct{})
	s.mu.Unlock()
	if old != nil {
		_ = old.Close()
	}
}

func hasSandboxRefreshUpdate(update acp.SessionUpdate) bool {
	raw, _ := json.Marshal(update)
	s := strings.ToLower(string(raw))
	return strings.Contains(s, "windows sandbox: spawn setup refresh")
}

func isSandboxRefreshErr(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "windows sandbox: spawn setup refresh")
}
