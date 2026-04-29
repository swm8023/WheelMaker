package client

import (
	"context"
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

// SessionAgentState holds all persisted per-agent metadata within one Session.
type SessionAgentState struct {
	ConfigOptions     []acp.ConfigOption     `json:"configOptions,omitempty"`
	Commands          []acp.AvailableCommand `json:"commands,omitempty"`
	Title             string                 `json:"title,omitempty"`
	UpdatedAt         string                 `json:"updatedAt,omitempty"`
	AgentCapabilities acp.AgentCapabilities  `json:"agentCapabilities,omitempty"`
	AgentInfo         *acp.AgentInfo         `json:"agentInfo,omitempty"`
	AuthMethods       []acp.AuthMethod       `json:"authMethods,omitempty"`
}

type createdSessionState struct {
	sessionID string
	agentType string
	state     SessionAgentState
	instance  agent.Instance
	createdAt time.Time
}

// Session is the business session object that owns ACP session state,
// prompt lifecycle and callback handling.
// A Client holds multiple Sessions, routed by IM routeKey.
type Session struct {
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
// id, cwd, and agentType are all required and immutable after creation.
func newSession(id, cwd, agentType string) (*Session, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, fmt.Errorf("session id is required")
	}
	agentType = strings.TrimSpace(agentType)
	if agentType == "" {
		return nil, fmt.Errorf("agent type is required")
	}
	s := &Session{
		acpSessionID:   id,
		agentType:      agentType,
		Status:         SessionActive,
		cwd:            cwd,
		createdAt:      time.Now(),
		prompt:         promptState{},
		timeoutLimiter: newTimeoutNotifyLimiter(timeoutNotifyCooldown),
	}
	s.initCond = sync.NewCond(&s.mu)
	return s, nil
}

func cloneAgentInfo(src *acp.AgentInfo) *acp.AgentInfo {
	if src == nil {
		return nil
	}
	cp := *src
	return &cp
}

func cloneSessionAgentState(src *SessionAgentState) *SessionAgentState {
	if src == nil {
		return nil
	}
	cp := *src
	cp.ConfigOptions = append([]acp.ConfigOption(nil), src.ConfigOptions...)
	cp.Commands = append([]acp.AvailableCommand(nil), src.Commands...)
	cp.AuthMethods = append([]acp.AuthMethod(nil), src.AuthMethods...)
	cp.AgentInfo = cloneAgentInfo(src.AgentInfo)
	return &cp
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
	agentType := s.agentType
	clientSID := s.acpSessionID
	configOpts := append([]acp.ConfigOption(nil), s.agentState.ConfigOptions...)
	s.mu.Unlock()

	snap := acp.SessionConfigSnapshotFromOptions(configOpts)
	sid := shortSessionID(clientSID)
	if sid == "" {
		sid = "none"
	}
	return fmt.Sprintf("session: %s\nagent: %s\nmode: %s\nmodel: %s",
		sid,
		renderUnknown(agentType),
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
	title = strings.TrimSpace(title)
	body = strings.TrimSpace(body)
	var messageText string
	switch {
	case title != "" && body != "":
		messageText = title + "\n" + body
	case title != "":
		messageText = title
	default:
		messageText = body
	}
	if messageText != "" {
		s.recordSessionViewEvent(SessionViewEvent{
			Type:    SessionViewEventTypeSystem,
			Content: messageText,
		})
	}
	if router, source, ok := s.imContext(); ok {
		_ = router.SystemNotify(context.Background(), im.SendTarget{SessionID: s.acpSessionID, Source: &source}, im.SystemPayload{
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
		event.SessionID = s.acpSessionID
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
	s.mu.Unlock()

	creator := s.registry.CreatorByName(s.agentType)
	if creator == nil {
		return fmt.Errorf("no agent registered for %q", s.agentType)
	}
	inst, err := creator(ctx, s.cwd)
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
	s.mu.Unlock()
	return nil
}

// emptyMCPServers returns an empty MCP server list for session/new and session/load calls.
// Replace this helper when MCP config support is added.
func emptyMCPServers() []acp.MCPServer {
	return []acp.MCPServer{}
}

// ensureReady performs ACP initialize + session/load for an existing ACP session ID.
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
	agentName := s.agentType
	savedSID := s.acpSessionID
	cwd := s.cwd
	persistedConfigOptions := append([]acp.ConfigOption(nil), s.agentState.ConfigOptions...)
	persistedCommands := append([]acp.AvailableCommand(nil), s.agentState.Commands...)
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

	if savedSID == "" {
		notifyDone()
		return errors.New("ensureReady: session id is required")
	}
	if !initResult.AgentCapabilities.LoadSession {
		notifyDone()
		return fmt.Errorf("ensureReady: agent %q does not support session/load", agentName)
	}

	loadResult, loadErr := inst.SessionLoad(ctx, acp.SessionLoadParams{
		SessionID:  savedSID,
		CWD:        cwd,
		MCPServers: emptyMCPServers(),
	})
	if loadErr != nil {
		notifyDone()
		return fmt.Errorf("ensureReady: session/load: %w", loadErr)
	}

	resolved := append([]acp.ConfigOption(nil), loadResult.ConfigOptions...)
	targetConfig := configPreferenceFromACPOptions(persistedConfigOptions)
	if len(resolved) > 0 {
		if len(targetConfig) > 0 {
			resolved = applyStoredConfigOptions(ctx, s.projectName, inst, savedSID, resolved, targetConfig)
		}
	} else if len(persistedConfigOptions) > 0 {
		resolved = append([]acp.ConfigOption(nil), persistedConfigOptions...)
	}
	resolvedCommands := append([]acp.AvailableCommand(nil), persistedCommands...)

	s.mu.Lock()
	state := &s.agentState
	if len(resolved) > 0 {
		state.ConfigOptions = append([]acp.ConfigOption(nil), resolved...)
	}
	state.Commands = append([]acp.AvailableCommand(nil), resolvedCommands...)
	state.AgentCapabilities = initResult.AgentCapabilities
	state.AgentInfo = cloneAgentInfo(initResult.AgentInfo)
	state.AuthMethods = initResult.AuthMethods
	s.ready = true
	s.initializing = false
	s.mu.Unlock()
	s.initCond.Broadcast()

	s.persistAgentPreferenceState(agentName, resolved)
	hubLogger(s.projectName).Info("connected agent=%s session=%s resumed", inst.Name(), savedSID)
	return nil
}
func configPreferenceFromACPOptions(options []acp.ConfigOption) []PreferenceConfigOption {
	out := make([]PreferenceConfigOption, 0, len(options))
	seen := make(map[string]struct{}, len(options))
	for _, opt := range options {
		optID := strings.TrimSpace(opt.ID)
		if optID == "" {
			continue
		}
		key := strings.ToLower(optID)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, PreferenceConfigOption{
			ID:           optID,
			CurrentValue: opt.CurrentValue,
		})
	}
	return out
}

func mergeConfigOptions(current []acp.ConfigOption, updated []acp.ConfigOption) []acp.ConfigOption {
	if len(updated) == 0 {
		return append([]acp.ConfigOption(nil), current...)
	}
	merged := append([]acp.ConfigOption(nil), current...)
	for _, next := range updated {
		nextID := strings.TrimSpace(next.ID)
		nextCategory := strings.TrimSpace(next.Category)
		replaced := false
		for i, existing := range merged {
			existingID := strings.TrimSpace(existing.ID)
			sameID := nextID != "" && strings.EqualFold(existingID, nextID)
			sameCategory := nextID == "" && nextCategory != "" && strings.EqualFold(strings.TrimSpace(existing.Category), nextCategory)
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

func applyStoredConfigOptions(
	ctx context.Context,
	projectName string,
	inst agent.Instance,
	sessionID string,
	current []acp.ConfigOption,
	target []PreferenceConfigOption,
) []acp.ConfigOption {
	options := append([]acp.ConfigOption(nil), current...)
	for _, next := range target {
		targetID := strings.TrimSpace(next.ID)
		if targetID == "" {
			continue
		}
		configID := ""
		currentValue := ""
		for _, opt := range options {
			optID := strings.TrimSpace(opt.ID)
			if optID == "" || !strings.EqualFold(optID, targetID) {
				continue
			}
			configID = optID
			currentValue = opt.CurrentValue
			break
		}
		if configID == "" {
			hubLogger(projectName).Warn("skip reapply config id=%s: config option not found", targetID)
			continue
		}
		if currentValue == next.CurrentValue {
			continue
		}
		updated, err := inst.SessionSetConfigOption(ctx, acp.SessionSetConfigOptionParams{
			SessionID: sessionID,
			ConfigID:  configID,
			Value:     next.CurrentValue,
		})
		if err != nil {
			hubLogger(projectName).Warn("reapply config failed session=%s id=%s value=%s err=%v",
				sessionID, configID, next.CurrentValue, err)
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

// promptStream sends a prompt and returns a channel of streaming updates.
func (s *Session) promptStream(ctx context.Context, blocks []acp.ContentBlock) (<-chan promptStreamEvent, error) {
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

	go func() {
		defer func() {
			s.mu.Lock()
			s.prompt.ctx = nil
			s.prompt.cancel = nil
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
	s.mu.Unlock()

	if cancel != nil {
		cancel()
	}

	if sessID == "" || !ready {
		return nil
	}
	return s.instance.SessionCancel(sessID)
}

func (s *Session) toRecord() (*SessionRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.agentType == "" {
		return nil, fmt.Errorf("session agent type is required")
	}

	state := cloneSessionAgentState(&s.agentState)
	title := ""
	if state != nil {
		title = strings.TrimSpace(state.Title)
	}

	agentJSON := "{}"
	if state != nil {
		raw, err := json.Marshal(state)
		if err != nil {
			return nil, fmt.Errorf("marshal agent_json: %w", err)
		}
		agentJSON = string(raw)
	}

	return &SessionRecord{
		ID:           s.acpSessionID,
		ProjectName:  s.projectName,
		Status:       s.Status,
		AgentType:    s.agentType,
		AgentJSON:    agentJSON,
		Title:        title,
		CreatedAt:    s.createdAt,
		LastActiveAt: s.lastActiveAt,
	}, nil
}

func sessionFromRecord(rec *SessionRecord, cwd string) (*Session, error) {
	s, err := newSession(rec.ID, cwd, rec.AgentType)
	if err != nil {
		return nil, fmt.Errorf("session %q: %w", rec.ID, err)
	}
	s.Status = rec.Status
	s.createdAt = rec.CreatedAt
	s.lastActiveAt = rec.LastActiveAt
	if strings.TrimSpace(rec.AgentJSON) != "" {
		if err := json.Unmarshal([]byte(rec.AgentJSON), &s.agentState); err != nil {
			return nil, fmt.Errorf("unmarshal agent_json: %w", err)
		}
	}
	if strings.TrimSpace(rec.Title) != "" {
		if strings.TrimSpace(s.agentState.Title) == "" {
			state := &s.agentState
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
		hubLogger(s.projectName).Warn("persist session failed session=%s err=%v", s.acpSessionID, err)
	}
}

func (s *Session) persistAgentPreferenceState(agentName string, options []acp.ConfigOption) {
	if s.store == nil || strings.TrimSpace(agentName) == "" {
		return
	}
	configOptions := configPreferenceFromACPOptions(options)
	next := PreferenceState{
		ConfigOptions: configOptions,
		UpdatedAt:     time.Now().UTC().Format(time.RFC3339Nano),
	}
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
		state := &s.agentState
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
	if changed {
		s.mu.Lock()
		agentType := s.agentType
		state := cloneSessionAgentState(&s.agentState)
		s.mu.Unlock()
		if state != nil {
			s.persistAgentPreferenceState(agentType, state.ConfigOptions)
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
	if len(blocks) == 0 {
		return
	}
	s.recordSessionViewEvent(SessionViewEvent{
		Type:      SessionViewEventTypeACP,
		SessionID: s.acpSessionID,
		Content: acp.BuildACPContentJSON(acp.MethodSessionPrompt, map[string]any{
			"params": acp.SessionPromptParams{
				SessionID: s.acpSessionID,
				Prompt:    cloneSessionContentBlocks(blocks),
			},
		}),
	})
	s.promptMu.Lock()
	defer s.promptMu.Unlock()
	ctx := context.Background()
	if err := s.ensureInstance(ctx); err != nil {
		s.reply(fmt.Sprintf("No active session: %v. %s", err, s.connectHint()))
		return
	}

	if err := s.ensureReadyAndNotify(ctx); err != nil {
		s.reply(fmt.Sprintf("No active session: %v. %s", err, s.connectHint()))
		return
	}

	updates, err := s.promptStream(ctx, blocks)
	if err != nil {
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
	observe := newPromptObserveState(time.Now())
	observeTicker := time.NewTicker(promptObserveInterval)

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
					SessionID: s.acpSessionID,
					Content: acp.BuildACPContentJSON(acp.MethodSessionUpdate, map[string]any{
						"params": params,
					}),
				})
				if hasIMEmitter {
					target := im.SendTarget{SessionID: s.acpSessionID, Source: &imSource}
					if emitErr := imRouter.PublishSessionUpdate(ctx, target, params); emitErr != nil {
						s.reply(fmt.Sprintf("IM emit error: %v", emitErr))
					}
				}
				if params.Update.SessionUpdate == acp.SessionUpdateAgentMessageChunk {
					text := extractTextChunk(params.Update.Content)
					if strings.TrimSpace(text) != "" && !hasIMEmitter {
						buf.WriteString(text)
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
					SessionID: s.acpSessionID,
					Content: acp.BuildACPContentJSON(acp.MethodSessionPrompt, map[string]any{
						"result": *ev.result,
					}),
				})
				if hasIMEmitter {
					target := im.SendTarget{SessionID: s.acpSessionID, Source: &imSource}
					if emitErr := imRouter.PublishPromptResult(ctx, target, *ev.result); emitErr != nil {
						s.reply(fmt.Sprintf("IM emit error: %v", emitErr))
					}
				}
				streamDone = true
			}
		case <-observeTicker.C:
			ev := observe.Eval(time.Now(), observe.Started())
			if ev.WarnFirstWait {
				hubLogger(s.projectName).Warn("timeout warn category=timeout stage=stream kind=first_wait session=%s", s.acpSessionID)
			}
			if ev.ErrorFirstWait {
				s.reportTimeoutError("stream", "first_wait")
			}
			if ev.WarnSilence {
				hubLogger(s.projectName).Warn("timeout warn category=timeout stage=stream kind=silence session=%s", s.acpSessionID)
			}
			if ev.ErrorSilence {
				s.reportTimeoutError("stream", "silence")
			}
		}
	}
	observeTicker.Stop()

	s.mu.Lock()
	s.prompt.currentCh = nil
	s.mu.Unlock()

	s.persistSessionBestEffort()

	if !hasIMEmitter && buf.Len() > 0 {
		s.reply(buf.String())
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

func renderUnknown(v string) string {
	if strings.TrimSpace(v) == "" {
		return "unknown"
	}
	return v
}

func (s *Session) reportTimeoutError(stage string, kind string) {
	now := time.Now()
	s.mu.Lock()
	agent := s.agentType
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
	body := fmt.Sprintf(
		"category=timeout stage=%s agent=%s sessionID=%s action=check /status then retry",
		stage,
		renderUnknown(agent),
		renderUnknown(sid),
	)
	s.reply(body)
}

func (s *Session) connectHint() string {
	s.mu.Lock()
	agentName := strings.TrimSpace(s.agentType)
	s.mu.Unlock()
	if agentName == "" && s.registry != nil {
		agentName = strings.TrimSpace(s.registry.PreferredName())
	}
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
	s.mu.Unlock()
	if old != nil {
		_ = old.Close()
	}
	return true
}
