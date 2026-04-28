package client

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/swm8023/wheelmaker/internal/im"
	acp "github.com/swm8023/wheelmaker/internal/protocol"
)

type HelpOption = im.HelpOption

type HelpMenu = im.HelpMenu

type HelpModel = im.HelpModel

// handleCommand processes recognized "/" commands.
func (c *Client) handleCommand(sess *Session, routeKey, cmd, args string) {
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()

	switch cmd {
	case "/cancel":
		sess.mu.Lock()
		active := sess.instance != nil
		sess.mu.Unlock()
		if !active {
			sess.reply("No active session.")
			return
		}
		if err := sess.cancelPrompt(); err != nil {
			sess.reply(fmt.Sprintf("Cancel error: %v", err))
			return
		}
		sess.reply("Cancelled.")

	case "/status":
		sess.mu.Lock()
		active := sess.instance != nil
		sess.mu.Unlock()
		if !active {
			sess.reply("No active session.")
			return
		}
		sess.reply(sess.sessionInfoLine())

	case "/list":
		c.handleListCommand(sess)

	case "/new":
		c.handleNewCommand(sess, routeKey, args)

	case "/load":
		c.handleLoadCommand(sess, routeKey, args)

	case "/mode":
		sess.handleConfigCommand(ctx, args, "Usage: /mode <mode-id-or-name>", "Mode", resolveModeArg)

	case "/model":
		sess.handleConfigCommand(ctx, args, "Usage: /model <model-id-or-name>", "Model", resolveModelArg)

	case "/config":
		sess.handleConfigCommand(ctx, args, "Usage: /config <config-id> <value>", "Config", resolveConfigArg)

	}
}

// handleNewCommand creates a new Client-level session and rebinds the route.
func (c *Client) handleNewCommand(sess *Session, routeKey, args string) {
	agentType := strings.TrimSpace(args)
	if agentType == "" {
		if _, source, ok := sess.imContext(); ok {
			model, err := sess.resolveHelpModel(context.Background(), source.ChatID)
			if err != nil {
				sess.reply(fmt.Sprintf("New error: %v", err))
				return
			}
			if err := c.sendHelpCard(context.Background(), source, model, "menu:new", 0); err != nil {
				sess.reply(fmt.Sprintf("New error: %v", err))
			}
			return
		}
		sess.reply("Usage: /new <agent-name>")
		return
	}

	newSess, err := c.ClientNewSession(routeKey, agentType)
	if err != nil {
		sess.reply(fmt.Sprintf("New error: %v", err))
		return
	}
	newSess.reply(fmt.Sprintf("Created new session: %s", newSess.acpSessionID))
}

// handleLoadCommand loads a session by index and rebinds the route.
func (c *Client) handleLoadCommand(sess *Session, routeKey, args string) {
	idxStr := strings.TrimSpace(args)
	if idxStr == "" {
		sess.reply("Usage: /load <index>  (see /list)")
		return
	}
	idx, err := parsePositiveIndex(idxStr)
	if err != nil {
		sess.reply(fmt.Sprintf("Load error: %v", err))
		return
	}
	loaded, err := c.ClientLoadSession(routeKey, idx)
	if err != nil {
		sess.reply(fmt.Sprintf("Load error: %v", err))
		return
	}
	// Reply from the NEW session so the message goes to the right context.
	loaded.reply(fmt.Sprintf("Loaded session: %s", loaded.acpSessionID))
}

func parsePositiveIndex(value string) (int, error) {
	idx, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || idx <= 0 {
		return 0, fmt.Errorf("index must be a positive integer")
	}
	return idx, nil
}

// handleListCommand lists all sessions (in-memory + persisted).
func (c *Client) handleListCommand(sess *Session) {
	body, err := c.formatSessionList(sess.acpSessionID)
	if err != nil {
		sess.reply(fmt.Sprintf("List error: %v", err))
		return
	}
	sess.reply(body)
}

func (c *Client) formatSessionList(currentID string) (string, error) {
	entries, err := c.clientListSessions()
	if err != nil {
		return "", err
	}

	if len(entries) == 0 {
		return "No sessions.", nil
	}

	lines := make([]string, 0, len(entries)+1)
	lines = append(lines, fmt.Sprintf("Sessions (%d):", len(entries)))
	for i, e := range entries {
		marker := " "
		if e.ID == currentID {
			marker = "*"
		}
		title := strings.TrimSpace(e.Title)
		if title == "" {
			title = "(no title)"
		}
		agent := e.Agent
		if agent == "" {
			agent = "-"
		}
		statusStr := "active"
		switch e.Status {
		case SessionSuspended:
			statusStr = "suspended"
		case SessionPersisted:
			statusStr = "persisted"
		}
		loc := "mem"
		if !e.InMemory {
			loc = "disk"
		}
		lines = append(lines, fmt.Sprintf("%s %d. [%s] %s  agent=%s  %s (%s)",
			marker, i+1, statusStr, e.ID, agent, title, loc))
	}
	return strings.Join(lines, "\n"), nil
}

func (s *Session) handleConfigCommand(
	ctx context.Context,
	args string,
	usage string,
	label string,
	resolve func(input string, st *SessionAgentState) (configID, value string, err error),
) {
	input := strings.TrimSpace(args)
	if input == "" {
		s.reply(usage)
		return
	}

	s.promptMu.Lock()
	defer s.promptMu.Unlock()

	if err := s.ensureInstance(ctx); err != nil {
		s.reply(fmt.Sprintf("No active session: %v. %s", err, s.connectHint()))
		return
	}

	// Lock section 1: read session state for config resolution.
	s.mu.Lock()
	agentName := s.currentAgentNameLocked()
	sessionState := cloneSessionAgentState(s.agentStateLocked(agentName))
	s.mu.Unlock()

	if err := s.ensureReadyAndNotify(ctx); err != nil {
		s.reply(fmt.Sprintf("%s error: %v", label, err))
		return
	}

	configID, value, err := resolve(input, sessionState)
	if err != nil {
		s.reply(fmt.Sprintf("%s error: %v", label, err))
		return
	}

	// Lock section 2: read sid after ensureReady has set acpSessionID.
	s.mu.Lock()
	sid := s.acpSessionID
	s.mu.Unlock()

	updatedOpts, err := s.instance.SessionSetConfigOption(ctx, acp.SessionSetConfigOptionParams{
		SessionID: sid,
		ConfigID:  configID,
		Value:     value,
	})
	if err != nil {
		s.reply(fmt.Sprintf("%s error: %v", label, err))
		return
	}
	// Apply returned config options immediately so the help menu reflects the new value.
	agentName = ""
	commands := []acp.AvailableCommand(nil)
	if len(updatedOpts) > 0 {
		s.mu.Lock()
		agentName = s.currentAgentNameLocked()
		if state := s.agentStateLocked(agentName); state != nil {
			state.ConfigOptions = append([]acp.ConfigOption(nil), updatedOpts...)
			updatedOpts = append([]acp.ConfigOption(nil), state.ConfigOptions...)
			commands = append([]acp.AvailableCommand(nil), state.Commands...)
		}
		s.mu.Unlock()
		s.persistAgentPreferenceState(agentName, updatedOpts, commands)
	}

	s.persistSessionBestEffort()
	s.reply(fmt.Sprintf("%s set to: %s", label, value))
}

func resolveModeArg(input string, st *SessionAgentState) (configID, value string, err error) {
	return resolveConfigSelectArg(acp.ConfigOptionIDMode, acp.ConfigOptionCategoryMode, input, st)
}

func resolveModelArg(input string, st *SessionAgentState) (configID, value string, err error) {
	return resolveConfigSelectArg(acp.ConfigOptionIDModel, acp.ConfigOptionCategoryModel, input, st)
}

func resolveConfigArg(input string, st *SessionAgentState) (configID, value string, err error) {
	parts := strings.Fields(strings.TrimSpace(input))
	if len(parts) < 2 {
		return "", "", fmt.Errorf("usage: /config <config-id> <value>")
	}
	configID = strings.TrimSpace(parts[0])
	if configID == "" {
		return "", "", fmt.Errorf("empty config id")
	}
	value = strings.TrimSpace(strings.Join(parts[1:], " "))
	if value == "" {
		return "", "", fmt.Errorf("empty config value")
	}
	if st == nil {
		return configID, value, nil
	}
	for _, opt := range st.ConfigOptions {
		if !strings.EqualFold(opt.ID, configID) {
			continue
		}
		if len(opt.Options) == 0 {
			return opt.ID, value, nil
		}
		for _, candidate := range opt.Options {
			if value == candidate.Value || strings.EqualFold(value, candidate.Name) {
				return opt.ID, candidate.Value, nil
			}
		}
		values := make([]string, 0, len(opt.Options))
		for _, candidate := range opt.Options {
			values = append(values, candidate.Value)
		}
		slices.Sort(values)
		return "", "", fmt.Errorf("unknown config value %q for %q (available: %s)", value, opt.ID, strings.Join(values, ", "))
	}
	return configID, value, nil
}

func resolveConfigSelectArg(kind string, defaultConfigID string, input string, st *SessionAgentState) (configID, value string, err error) {
	v := strings.TrimSpace(input)
	if v == "" {
		return "", "", fmt.Errorf("empty %s", kind)
	}

	configID = defaultConfigID
	var targetOpt *acp.ConfigOption
	if st != nil {
		for i := range st.ConfigOptions {
			opt := &st.ConfigOptions[i]
			if opt.ID == defaultConfigID || strings.EqualFold(opt.Category, kind) {
				targetOpt = opt
				configID = opt.ID
				break
			}
		}
	}

	if targetOpt != nil && len(targetOpt.Options) > 0 {
		for _, opt := range targetOpt.Options {
			if v == opt.Value || strings.EqualFold(v, opt.Name) {
				return configID, opt.Value, nil
			}
		}
		values := make([]string, 0, len(targetOpt.Options))
		for _, opt := range targetOpt.Options {
			values = append(values, opt.Value)
		}
		slices.Sort(values)
		return "", "", fmt.Errorf("unknown %s %q (available: %s)", kind, v, strings.Join(values, ", "))
	}

	return configID, v, nil
}

func (s *Session) resolveHelpModel(ctx context.Context, _ string) (HelpModel, error) {
	s.mu.Lock()
	hasInstance := s.instance != nil
	s.mu.Unlock()
	if !hasInstance {
		_ = s.ensureInstance(ctx)
	}
	_ = s.ensureReady(ctx)

	state, _ := s.currentAgentStateSnapshot()
	opts := []acp.ConfigOption(nil)
	if state != nil {
		opts = append(opts, state.ConfigOptions...)
	}

	model := HelpModel{
		Title:    "WheelMaker",
		Body:     "",
		RootMenu: "root",
		Menus:    map[string]HelpMenu{},
	}

	// 1. New Conversation
	newMenuID := "menu:new"
	model.Options = append(model.Options, HelpOption{
		Label:  "New Conversation",
		MenuID: newMenuID,
	})
	newMenu := HelpMenu{
		Title:  "New Conversation",
		Body:   "Choose an agent for the new conversation.",
		Parent: model.RootMenu,
	}
	agentNames := []string(nil)
	if s.registry != nil {
		agentNames = s.registry.Names()
	}
	for _, name := range agentNames {
		newMenu.Options = append(newMenu.Options, HelpOption{
			Label:   "Agent: " + name,
			Command: "/new",
			Value:   name,
		})
	}
	if len(newMenu.Options) == 0 {
		newMenu.Body = "No agents available."
	}
	model.Menus[newMenuID] = newMenu

	// 2. Session List (client-level behavior)
	sessionMenuID := "menu:sessions"
	model.Options = append(model.Options, HelpOption{Label: "Session List", MenuID: sessionMenuID})
	model.Menus[sessionMenuID] = HelpMenu{
		Title:  "Sessions",
		Body:   "Use /list to view sessions, then /load <index> to load one.",
		Parent: model.RootMenu,
		Options: []HelpOption{
			{Label: "Show sessions", Command: "/list"},
		},
	}

	// 3. Status
	model.Options = append(model.Options, HelpOption{Label: "Status", Command: "/status"})

	// 4. Config options
	for _, opt := range opts {
		cfgID := strings.TrimSpace(opt.ID)
		if cfgID == "" {
			continue
		}
		label := "Config: " + cfgID
		if cur := strings.TrimSpace(opt.CurrentValue); cur != "" {
			label += " (" + cur + ")"
		}
		menuID := "menu:config:" + cfgID
		model.Options = append(model.Options, HelpOption{
			Label:  label,
			MenuID: menuID,
		})
		cfgMenu := HelpMenu{
			Title:  "Config: " + cfgID,
			Body:   "Select a value.",
			Parent: model.RootMenu,
		}
		for _, v := range opt.Options {
			display := firstNonEmpty(v.Name, v.Value)
			if display == "" {
				continue
			}
			cfgMenu.Options = append(cfgMenu.Options, HelpOption{
				Label:   display,
				Command: "/config",
				Value:   cfgID + " " + v.Value,
			})
		}
		if len(cfgMenu.Options) == 0 {
			cfgMenu.Body = "No predefined values. Use /config " + cfgID + " <value> manually."
		}
		model.Menus[menuID] = cfgMenu
	}

	return model, nil
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

func formatConfigOptionUpdateMessage(raw []byte) string {
	if len(raw) == 0 {
		return "Config options updated."
	}
	var u acp.SessionUpdate
	var opts []acp.ConfigOption
	if err := json.Unmarshal(raw, &u); err == nil {
		opts = u.ConfigOptions
	}
	if len(opts) == 0 {
		return "Config options updated."
	}
	mode := ""
	model := ""
	for _, opt := range opts {
		if mode == "" && (opt.ID == acp.ConfigOptionIDMode || strings.EqualFold(opt.Category, acp.ConfigOptionCategoryMode)) {
			mode = strings.TrimSpace(opt.CurrentValue)
		}
		if model == "" && (opt.ID == acp.ConfigOptionIDModel || strings.EqualFold(opt.Category, acp.ConfigOptionCategoryModel)) {
			model = strings.TrimSpace(opt.CurrentValue)
		}
	}
	if mode == "" && model == "" {
		return "Config options updated."
	}
	return fmt.Sprintf("Config options updated: mode=%s model=%s", renderUnknown(mode), renderUnknown(model))
}

