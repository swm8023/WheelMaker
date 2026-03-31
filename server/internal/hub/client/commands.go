package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	acp "github.com/swm8023/wheelmaker/internal/hub/acp"
	"github.com/swm8023/wheelmaker/internal/hub/im"
)

// handleCommand processes recognized "/" commands.
func (c *Client) handleCommand(msg im.Message, cmd, args string) {
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()

	switch cmd {
	case "/use":
		if args == "" {
			c.reply("Usage: /use <agent-name> [--continue]  (e.g. /use claude, /use copilot)")
			return
		}
		parts := strings.Fields(args)
		name := strings.ToLower(parts[0])
		mode := SwitchClean
		for _, p := range parts[1:] {
			if p == "--continue" {
				mode = SwitchWithContext
			}
		}
		if err := c.switchAgent(ctx, name, mode); err != nil {
			c.reply(fmt.Sprintf("Switch error: %v", err))
		}

	case "/cancel":
		c.mu.Lock()
		active := c.conn != nil
		c.mu.Unlock()
		if !active {
			c.reply("No active session.")
			return
		}
		if err := c.cancelPrompt(); err != nil {
			c.reply(fmt.Sprintf("Cancel error: %v", err))
			return
		}
		c.reply("Cancelled.")

	case "/status":
		c.mu.Lock()
		agentName := ""
		if c.conn != nil {
			agentName = c.conn.name
		}
		sid := c.session.id
		active := c.conn != nil
		c.mu.Unlock()
		if !active {
			c.reply("No active session.")
			return
		}
		status := fmt.Sprintf("Active agent: %s", agentName)
		if sid != "" {
			status += fmt.Sprintf("\nACP session: %s", sid)
		}
		c.reply(status)

	case "/list":
		c.promptMu.Lock()
		defer c.promptMu.Unlock()
		lines, err := c.listSessions(ctx)
		if err != nil {
			c.reply(fmt.Sprintf("List error: %v", err))
			return
		}
		c.reply(strings.Join(lines, "\n"))

	case "/new":
		c.promptMu.Lock()
		defer c.promptMu.Unlock()
		sid, err := c.createNewSession(ctx)
		if err != nil {
			c.reply(fmt.Sprintf("New error: %v", err))
			return
		}
		c.reply(fmt.Sprintf("Created new session: %s", sid))

	case "/load":
		idxStr := strings.TrimSpace(args)
		if idxStr == "" {
			c.reply("Usage: /load <index>  (see /list)")
			return
		}
		idx, err := strconv.Atoi(idxStr)
		if err != nil || idx <= 0 {
			c.reply("Load error: index must be a positive integer")
			return
		}
		c.promptMu.Lock()
		defer c.promptMu.Unlock()
		sid, err := c.loadSessionByIndex(ctx, idx)
		if err != nil {
			c.reply(fmt.Sprintf("Load error: %v", err))
			return
		}
		c.reply(fmt.Sprintf("Loaded session: %s", sid))

	case "/mode":
		c.handleConfigCommand(ctx, args, "Usage: /mode <mode-id-or-name>", "Mode", resolveModeArg)

	case "/model":
		c.handleConfigCommand(ctx, args, "Usage: /model <model-id-or-name>", "Model", resolveModelArg)

	case "/config":
		c.handleConfigCommand(ctx, args, "Usage: /config <config-id> <value>", "Config", resolveConfigArg)

	}
}

func (c *Client) handleConfigCommand(
	ctx context.Context,
	args string,
	usage string,
	label string,
	resolve func(input string, st *SessionState) (configID, value string, err error),
) {
	input := strings.TrimSpace(args)
	if input == "" {
		c.reply(usage)
		return
	}

	c.promptMu.Lock()
	defer c.promptMu.Unlock()

	if err := c.ensureForwarder(ctx); err != nil {
		c.reply(fmt.Sprintf("No active session: %v. Use /use <agent> to connect.", err))
		return
	}

	// Lock section 1: read agentName and sessionState for config resolution.
	c.mu.Lock()
	agentName := ""
	if c.conn != nil {
		agentName = c.conn.name
	}
	var sessionState *SessionState
	if c.state != nil && c.state.Agents != nil {
		if as := c.state.Agents[agentName]; as != nil {
			sessionState = as.Session
		}
	}
	c.mu.Unlock()

	if err := c.ensureReadyAndNotify(ctx); err != nil {
		c.reply(fmt.Sprintf("%s error: %v", label, err))
		return
	}

	configID, value, err := resolve(input, sessionState)
	if err != nil {
		c.reply(fmt.Sprintf("%s error: %v", label, err))
		return
	}

	// Lock section 2: read fwd and sid after ensureReady has set c.session.id.
	c.mu.Lock()
	fwd := c.conn.forwarder
	sid := c.session.id
	c.mu.Unlock()

	updatedOpts, err := fwd.SessionSetConfigOption(ctx, acp.SessionSetConfigOptionParams{
		SessionID: sid,
		ConfigID:  configID,
		Value:     value,
	})
	if err != nil {
		c.reply(fmt.Sprintf("%s error: %v", label, err))
		return
	}
	// Apply returned config options immediately so the help menu reflects the new value.
	if len(updatedOpts) > 0 {
		c.mu.Lock()
		c.sessionMeta.ConfigOptions = updatedOpts
		c.mu.Unlock()
	}

	c.saveSessionState()
	c.reply(fmt.Sprintf("%s set to: %s", label, value))
}

func resolveModeArg(input string, st *SessionState) (configID, value string, err error) {
	return resolveConfigSelectArg("mode", "mode", input, st)
}

func resolveModelArg(input string, st *SessionState) (configID, value string, err error) {
	return resolveConfigSelectArg("model", "model", input, st)
}

func resolveConfigArg(input string, st *SessionState) (configID, value string, err error) {
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

func resolveConfigSelectArg(kind string, defaultConfigID string, input string, st *SessionState) (configID, value string, err error) {
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

func (c *Client) listSessions(ctx context.Context) ([]string, error) {
	if err := c.ensureForwarder(ctx); err != nil {
		return nil, err
	}

	if err := c.ensureReady(ctx); err != nil {
		return nil, err
	}

	c.mu.Lock()
	fwd := c.conn.forwarder
	cwd := c.cwd
	curSID := c.session.id
	agentName := c.conn.name
	caps := c.initMeta.AgentCapabilities
	c.mu.Unlock()

	if fwd == nil {
		return nil, errors.New("no active forwarder")
	}
	if caps.SessionCapabilities == nil || caps.SessionCapabilities.List == nil {
		return nil, errors.New("agent does not support session/list")
	}

	all := make([]acp.SessionInfo, 0, 16)
	cursor := ""
	for page := 0; page < 20; page++ {
		res, err := fwd.SessionList(ctx, acp.SessionListParams{
			CWD:    cwd,
			Cursor: cursor,
		})
		if err != nil {
			return nil, err
		}
		all = append(all, res.Sessions...)
		if strings.TrimSpace(res.NextCursor) == "" || res.NextCursor == cursor {
			break
		}
		cursor = res.NextCursor
	}

	summaries := make([]SessionSummary, 0, len(all))
	lines := make([]string, 0, len(all)+1)
	lines = append(lines, fmt.Sprintf("Sessions (%d):", len(all)))
	for i, s := range all {
		summaries = append(summaries, SessionSummary{
			ID:        s.SessionID,
			Title:     s.Title,
			UpdatedAt: s.UpdatedAt,
		})
		marker := " "
		if s.SessionID == curSID {
			marker = "*"
		}
		title := strings.TrimSpace(s.Title)
		if title == "" {
			title = "(no title)"
		}
		lines = append(lines, fmt.Sprintf("%s %d. %s  %s", marker, i+1, s.SessionID, title))
	}

	c.persistSessionSummaries(agentName, summaries)
	return lines, nil
}

func (c *Client) createNewSession(ctx context.Context) (string, error) {
	if err := c.ensureForwarder(ctx); err != nil {
		return "", err
	}
	c.mu.Lock()
	fwd := c.conn.forwarder
	cwd := c.cwd
	c.mu.Unlock()
	if fwd == nil {
		return "", errors.New("no active forwarder")
	}
	if err := c.ensureReady(ctx); err != nil {
		return "", err
	}

	res, err := fwd.SessionNew(ctx, acp.SessionNewParams{
		CWD:        cwd,
		MCPServers: emptyMCPServers(),
	})
	if err != nil {
		return "", err
	}

	c.mu.Lock()
	c.resetSessionFields(res.SessionID, res.ConfigOptions)
	c.mu.Unlock()
	c.saveSessionState()
	return res.SessionID, nil
}

func (c *Client) loadSessionByIndex(ctx context.Context, index int) (string, error) {
	lines, err := c.listSessions(ctx)
	if err != nil {
		return "", err
	}
	_ = lines // listSessions already refreshes and persists state

	c.mu.Lock()
	agentName := c.conn.name
	fwd := c.conn.forwarder
	cwd := c.cwd
	loadCap := c.initMeta.AgentCapabilities.LoadSession
	var sessions []SessionSummary
	if c.state != nil && c.state.Agents != nil {
		if as := c.state.Agents[agentName]; as != nil {
			sessions = append(sessions, as.Sessions...)
		}
	}
	c.mu.Unlock()

	if !loadCap {
		return "", errors.New("agent does not support session/load")
	}
	if fwd == nil {
		return "", errors.New("no active forwarder")
	}
	if index < 1 || index > len(sessions) {
		return "", fmt.Errorf("index out of range (1-%d)", len(sessions))
	}
	target := sessions[index-1].ID
	if strings.TrimSpace(target) == "" {
		return "", errors.New("invalid session id")
	}

	_, err = fwd.SessionLoad(ctx, acp.SessionLoadParams{
		SessionID:  target,
		CWD:        cwd,
		MCPServers: emptyMCPServers(),
	})
	if err != nil {
		return "", err
	}

	c.mu.Lock()
	c.resetSessionFields(target, nil)
	c.mu.Unlock()
	c.saveSessionState()
	return target, nil
}

func (c *Client) persistSessionSummaries(agentName string, sessions []SessionSummary) {
	if strings.TrimSpace(agentName) == "" {
		return
	}
	c.mu.Lock()
	if c.state == nil {
		c.mu.Unlock()
		return
	}
	if c.state.Agents == nil {
		c.state.Agents = map[string]*AgentState{}
	}
	as := c.state.Agents[agentName]
	if as == nil {
		as = &AgentState{}
		c.state.Agents[agentName] = as
	}
	as.Sessions = sessions
	s := c.state
	c.mu.Unlock()
	_ = c.store.Save(s)
}

func (c *Client) resolveHelpModel(ctx context.Context, _ string) (im.HelpModel, error) {
	c.mu.Lock()
	hasForwarder := c.conn != nil && c.conn.forwarder != nil
	c.mu.Unlock()
	if !hasForwarder {
		_ = c.ensureForwarder(ctx)
	}
	_ = c.ensureReady(ctx)

	c.mu.Lock()
	opts := append([]acp.ConfigOption(nil), c.sessionMeta.ConfigOptions...)
	currentAgent := ""
	if c.conn != nil {
		currentAgent = c.conn.name
	}
	var cachedSessions []SessionSummary
	if c.state != nil && c.state.Agents != nil && currentAgent != "" {
		if as := c.state.Agents[currentAgent]; as != nil {
			cachedSessions = append([]SessionSummary(nil), as.Sessions...)
		}
	}
	c.mu.Unlock()

	model := im.HelpModel{
		Title:    "WheelMaker",
		Body:     "",
		RootMenu: "root",
		Menus:    map[string]im.HelpMenu{},
	}

	// 1. Agent switch (show current agent in label)
	agentLabel := "Agent Switch"
	if currentAgent != "" {
		agentLabel = "Agent: " + currentAgent
	}
	agentMenuID := "menu:agents"
	model.Options = append(model.Options, im.HelpOption{
		Label:  agentLabel,
		MenuID: agentMenuID,
	})
	agentMenu := im.HelpMenu{
		Title:  "Agent Switch",
		Body:   "Choose an agent to switch to.",
		Parent: model.RootMenu,
	}
	agentNames := c.registry.names()
	for _, name := range agentNames {
		agentMenu.Options = append(agentMenu.Options, im.HelpOption{
			Label:   "Agent: " + name,
			Command: "/use",
			Value:   name,
		})
	}
	model.Menus[agentMenuID] = agentMenu

	// 2. Config options
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
		model.Options = append(model.Options, im.HelpOption{
			Label:  label,
			MenuID: menuID,
		})
		cfgMenu := im.HelpMenu{
			Title:  "Config: " + cfgID,
			Body:   "Select a value.",
			Parent: model.RootMenu,
		}
		for _, v := range opt.Options {
			display := firstNonEmpty(v.Name, v.Value)
			if display == "" {
				continue
			}
			cfgMenu.Options = append(cfgMenu.Options, im.HelpOption{
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

	// 3. Session List — submenu from cached sessions, clicking loads the session
	sessionMenuID := "menu:sessions"
	model.Options = append(model.Options, im.HelpOption{
		Label:  "Session List",
		MenuID: sessionMenuID,
	})
	sessionMenu := im.HelpMenu{
		Title:  "Sessions",
		Body:   "Select a session to load.",
		Parent: model.RootMenu,
	}
	for i, s := range cachedSessions {
		title := strings.TrimSpace(s.Title)
		if title == "" {
			title = "(no title)"
		}
		label := fmt.Sprintf("%d. %s", i+1, title)
		sessionMenu.Options = append(sessionMenu.Options, im.HelpOption{
			Label:   label,
			Command: "/load",
			Value:   strconv.Itoa(i + 1),
		})
	}
	if len(sessionMenu.Options) == 0 {
		sessionMenu.Body = "No cached sessions. Send a message first to populate the list."
	}
	model.Menus[sessionMenuID] = sessionMenu

	// 4. Status
	model.Options = append(model.Options, im.HelpOption{Label: "Status", Command: "/status"})

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
		if mode == "" && (opt.ID == "mode" || strings.EqualFold(opt.Category, "mode")) {
			mode = strings.TrimSpace(opt.CurrentValue)
		}
		if model == "" && (opt.ID == "model" || strings.EqualFold(opt.Category, "model")) {
			model = strings.TrimSpace(opt.CurrentValue)
		}
	}
	if mode == "" && model == "" {
		return "Config options updated."
	}
	return fmt.Sprintf("Config options updated: mode=%s model=%s", renderUnknown(mode), renderUnknown(model))
}
