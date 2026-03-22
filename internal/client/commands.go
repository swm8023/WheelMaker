package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	acp "github.com/swm8023/wheelmaker/internal/acp"
	"github.com/swm8023/wheelmaker/internal/im"
)

// handleCommand processes recognized "/" commands.
func (c *Client) handleCommand(msg im.Message, cmd, args string) {
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()

	switch cmd {
	case "/use":
		if args == "" {
			c.reply(msg.ChatID, "Usage: /use <agent-name> [--continue]  (e.g. /use claude)")
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
		if err := c.switchAgent(ctx, msg.ChatID, name, mode); err != nil {
			c.reply(msg.ChatID, fmt.Sprintf("Switch error: %v", err))
		}

	case "/cancel":
		c.mu.Lock()
		active := c.currentAgent != nil
		c.mu.Unlock()
		if !active {
			c.reply(msg.ChatID, "No active session.")
			return
		}
		if err := c.cancelPrompt(); err != nil {
			c.reply(msg.ChatID, fmt.Sprintf("Cancel error: %v", err))
			return
		}
		c.reply(msg.ChatID, "Cancelled.")

	case "/status":
		c.mu.Lock()
		agentName := ""
		if c.currentAgent != nil {
			agentName = c.currentAgent.Name()
		}
		sid := c.sessionID
		active := c.currentAgent != nil
		c.mu.Unlock()
		if !active {
			c.reply(msg.ChatID, "No active session.")
			return
		}
		status := fmt.Sprintf("Active agent: %s", agentName)
		if sid != "" {
			status += fmt.Sprintf("\nACP session: %s", sid)
		}
		c.reply(msg.ChatID, status)

	case "/list":
		c.promptMu.Lock()
		defer c.promptMu.Unlock()
		lines, err := c.listSessions(ctx)
		if err != nil {
			c.reply(msg.ChatID, fmt.Sprintf("List error: %v", err))
			return
		}
		c.reply(msg.ChatID, strings.Join(lines, "\n"))

	case "/new":
		c.promptMu.Lock()
		defer c.promptMu.Unlock()
		sid, err := c.createNewSession(ctx)
		if err != nil {
			c.reply(msg.ChatID, fmt.Sprintf("New error: %v", err))
			return
		}
		c.reply(msg.ChatID, fmt.Sprintf("Created new session: %s", sid))

	case "/load":
		idxStr := strings.TrimSpace(args)
		if idxStr == "" {
			c.reply(msg.ChatID, "Usage: /load <index>  (see /list)")
			return
		}
		idx, err := strconv.Atoi(idxStr)
		if err != nil || idx <= 0 {
			c.reply(msg.ChatID, "Load error: index must be a positive integer")
			return
		}
		c.promptMu.Lock()
		defer c.promptMu.Unlock()
		sid, err := c.loadSessionByIndex(ctx, idx)
		if err != nil {
			c.reply(msg.ChatID, fmt.Sprintf("Load error: %v", err))
			return
		}
		c.reply(msg.ChatID, fmt.Sprintf("Loaded session: %s", sid))

	case "/mode":
		c.handleConfigCommand(ctx, msg.ChatID, args, "Usage: /mode <mode-id-or-name>", "Mode", resolveModeArg)

	case "/model":
		c.handleConfigCommand(ctx, msg.ChatID, args, "Usage: /model <model-id-or-name>", "Model", resolveModelArg)

	case "/debug":
		if err := c.handleDebugCommand(msg.ChatID, args); err != nil {
			c.reply(msg.ChatID, fmt.Sprintf("Debug error: %v", err))
		}
	}
}

func (c *Client) handleConfigCommand(
	ctx context.Context,
	chatID string,
	args string,
	usage string,
	label string,
	resolve func(input string, st *SessionState) (configID, value string, err error),
) {
	input := strings.TrimSpace(args)
	if input == "" {
		c.reply(chatID, usage)
		return
	}

	c.promptMu.Lock()
	defer c.promptMu.Unlock()

	if err := c.ensureForwarder(ctx); err != nil {
		c.reply(chatID, fmt.Sprintf("No active session: %v. Use /use <agent> to connect.", err))
		return
	}

	c.mu.Lock()
	agentName := ""
	if c.currentAgent != nil {
		agentName = c.currentAgent.Name()
	}
	var sessionState *SessionState
	if c.state != nil && c.state.Agents != nil {
		if as := c.state.Agents[agentName]; as != nil {
			sessionState = as.Session
		}
	}
	c.mu.Unlock()

	if err := c.ensureReadyAndNotify(ctx, chatID); err != nil {
		c.reply(chatID, fmt.Sprintf("%s error: %v", label, err))
		return
	}

	configID, value, err := resolve(input, sessionState)
	if err != nil {
		c.reply(chatID, fmt.Sprintf("%s error: %v", label, err))
		return
	}

	c.mu.Lock()
	fwd := c.forwarder
	sid := c.sessionID
	c.mu.Unlock()
	if fwd == nil {
		c.reply(chatID, fmt.Sprintf("%s error: no active forwarder", label))
		return
	}

	if _, err := fwd.SessionSetConfigOption(ctx, acp.SessionSetConfigOptionParams{
		SessionID: sid,
		ConfigID:  configID,
		Value:     value,
	}); err != nil {
		c.reply(chatID, fmt.Sprintf("%s error: %v", label, err))
		return
	}

	c.saveSessionState()
	c.reply(chatID, fmt.Sprintf("%s set to: %s", label, value))
}

func resolveModeArg(input string, st *SessionState) (configID, value string, err error) {
	return resolveConfigSelectArg("mode", "mode", input, st)
}

func resolveModelArg(input string, st *SessionState) (configID, value string, err error) {
	return resolveConfigSelectArg("model", "model", input, st)
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
	fwd := c.forwarder
	cwd := c.cwd
	curSID := c.sessionID
	agentName := c.currentAgentName
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
	fwd := c.forwarder
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
	agentName := c.currentAgentName
	fwd := c.forwarder
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
	hasForwarder := c.forwarder != nil
	c.mu.Unlock()
	if !hasForwarder {
		_ = c.ensureForwarder(ctx)
	}
	_ = c.ensureReady(ctx)

	c.mu.Lock()
	opts := append([]acp.ConfigOption(nil), c.sessionMeta.ConfigOptions...)
	commands := append([]acp.AvailableCommand(nil), c.sessionMeta.AvailableCommands...)
	c.mu.Unlock()

	model := im.HelpModel{
		Title: "WheelMaker Help",
		Body:  "Choose a quick action below. Advanced commands can still be typed manually.",
	}
	model.Options = append(model.Options, im.HelpOption{Label: "Status", Command: "/status"})
	model.Options = append(model.Options, im.HelpOption{Label: "Cancel", Command: "/cancel"})
	model.Options = append(model.Options, im.HelpOption{Label: "List Sessions", Command: "/list"})
	model.Options = append(model.Options, im.HelpOption{Label: "New Session", Command: "/new"})
	model.Options = append(model.Options, im.HelpOption{Label: "Project Debug Status", Command: "/debug"})
	c.mu.Lock()
	agentNames := make([]string, 0, len(c.agentFacs))
	for name := range c.agentFacs {
		agentNames = append(agentNames, name)
	}
	c.mu.Unlock()
	for _, name := range agentNames {
		model.Options = append(model.Options, im.HelpOption{
			Label:   "Agent: " + name,
			Command: "/use",
			Value:   name,
		})
	}

	for _, opt := range opts {
		switch opt.ID {
		case "mode":
			for _, v := range opt.Options {
				model.Options = append(model.Options, im.HelpOption{
					Label:   "Mode: " + firstNonEmpty(v.Name, v.Value),
					Command: "/mode",
					Value:   v.Value,
				})
			}
		case "model":
			for _, v := range opt.Options {
				model.Options = append(model.Options, im.HelpOption{
					Label:   "Model: " + firstNonEmpty(v.Name, v.Value),
					Command: "/model",
					Value:   v.Value,
				})
			}
		}
	}
	if len(commands) > 0 {
		names := make([]string, 0, len(commands))
		for _, cmd := range commands {
			if strings.TrimSpace(cmd.Name) != "" {
				names = append(names, cmd.Name)
			}
		}
		if len(names) > 0 {
			model.Body += "\nAvailable slash commands: " + strings.Join(names, ", ")
		}
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
