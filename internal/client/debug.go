package client

import (
	"fmt"
	"io"
	"strings"
	"sync"
)

type agentDebugSink struct {
	client *Client

	mu          sync.Mutex
	chatByAgent map[string]string
}

func newAgentDebugSink(c *Client) *agentDebugSink {
	return &agentDebugSink{
		client:      c,
		chatByAgent: map[string]string{},
	}
}

func (s *agentDebugSink) bindChat(agentName, chatID string) {
	agentName = strings.TrimSpace(agentName)
	chatID = strings.TrimSpace(chatID)
	if agentName == "" || chatID == "" {
		return
	}
	s.mu.Lock()
	s.chatByAgent[agentName] = chatID
	s.mu.Unlock()
}

func (s *agentDebugSink) resolveChat(agentName string) string {
	s.mu.Lock()
	chatID := s.chatByAgent[agentName]
	s.mu.Unlock()
	if strings.TrimSpace(chatID) != "" {
		return chatID
	}
	return s.client.projectName
}

func (s *agentDebugSink) writer(agentName string) io.Writer {
	return &agentDebugWriter{sink: s, agentName: agentName}
}

type agentDebugWriter struct {
	sink      *agentDebugSink
	agentName string
}

func (w *agentDebugWriter) Write(p []byte) (int, error) {
	raw := strings.TrimSpace(string(p))
	if raw == "" {
		return len(p), nil
	}

	lines := strings.Split(raw, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		chatID := w.sink.resolveChat(w.agentName)
		w.sink.client.reply(chatID, fmt.Sprintf("[debug][%s] %s", w.agentName, line))
	}
	return len(p), nil
}

func (c *Client) resolveCurrentAgentName() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if strings.TrimSpace(c.currentAgentName) != "" {
		return c.currentAgentName
	}
	if c.state != nil && strings.TrimSpace(c.state.ActiveAgent) != "" {
		return c.state.ActiveAgent
	}
	return "claude"
}

func (c *Client) bindDebugChat(agentName, chatID string) {
	if c.debugSink == nil {
		return
	}
	c.debugSink.bindChat(agentName, chatID)
}

func (c *Client) composeDebugWriter(agentName string, base io.Writer, debugEnabled bool) io.Writer {
	var ws []io.Writer
	if base != nil {
		ws = append(ws, base)
	}
	if debugEnabled && c.debugSink != nil {
		ws = append(ws, c.debugSink.writer(agentName))
	}
	if len(ws) == 0 {
		return nil
	}
	if len(ws) == 1 {
		return ws[0]
	}
	return io.MultiWriter(ws...)
}

func (c *Client) handleDebugCommand(chatID, args string) error {
	parts := strings.Fields(strings.TrimSpace(args))
	current := c.resolveCurrentAgentName()

	var target string
	var mode *bool

	switch len(parts) {
	case 0:
		c.reply(chatID, c.renderDebugStatus())
		return nil
	case 1:
		if v, ok := parseDebugOnOff(parts[0]); ok {
			target = current
			mode = &v
		} else {
			target = parts[0]
			c.reply(chatID, c.renderAgentDebugStatus(target))
			return nil
		}
	case 2:
		target = parts[0]
		v, ok := parseDebugOnOff(parts[1])
		if !ok {
			return fmt.Errorf("usage: /debug [agent] <on|off>")
		}
		mode = &v
	default:
		return fmt.Errorf("usage: /debug [agent] <on|off>")
	}

	target = strings.TrimSpace(target)
	if target == "" {
		return fmt.Errorf("usage: /debug [agent] <on|off>")
	}

	c.bindDebugChat(target, chatID)
	changed := c.setAgentDebugSetting(target, *mode)
	c.refreshActiveDebugLogger()

	word := "enabled"
	if !*mode {
		word = "disabled"
	}
	if changed {
		c.reply(chatID, fmt.Sprintf("Debug %s for agent: %s", word, target))
		return nil
	}
	c.reply(chatID, fmt.Sprintf("Debug already %s for agent: %s", word, target))
	return nil
}

func parseDebugOnOff(v string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "on", "true", "1":
		return true, true
	case "off", "false", "0":
		return false, true
	default:
		return false, false
	}
}

func (c *Client) renderDebugStatus() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.state == nil || len(c.state.Agents) == 0 {
		return "Debug status: no known agents"
	}
	lines := []string{"Debug status:"}
	for name, as := range c.state.Agents {
		flag := "off"
		if as != nil && as.DebugIM {
			flag = "on"
		}
		lines = append(lines, fmt.Sprintf("- %s: %s", name, flag))
	}
	return strings.Join(lines, "\n")
}

func (c *Client) renderAgentDebugStatus(agentName string) string {
	agentName = strings.TrimSpace(agentName)
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.state == nil || c.state.Agents == nil {
		return fmt.Sprintf("Debug status for %s: off", agentName)
	}
	as := c.state.Agents[agentName]
	if as != nil && as.DebugIM {
		return fmt.Sprintf("Debug status for %s: on", agentName)
	}
	return fmt.Sprintf("Debug status for %s: off", agentName)
}

func (c *Client) setAgentDebugSetting(agentName string, enabled bool) bool {
	c.mu.Lock()
	if c.state == nil {
		c.state = defaultProjectState()
	}
	if c.state.Agents == nil {
		c.state.Agents = map[string]*AgentState{}
	}
	as := c.state.Agents[agentName]
	if as == nil {
		as = &AgentState{}
		c.state.Agents[agentName] = as
	}
	changed := as.DebugIM != enabled
	as.DebugIM = enabled
	s := c.state
	c.mu.Unlock()

	if changed && s != nil {
		_ = c.store.Save(s)
	}
	return changed
}

func (c *Client) refreshActiveDebugLogger() {
	c.mu.Lock()
	name := c.currentAgentName
	fwd := c.forwarder
	base := c.debugLog
	enabled := false
	if c.state != nil && c.state.Agents != nil {
		if as := c.state.Agents[name]; as != nil {
			enabled = as.DebugIM
		}
	}
	c.mu.Unlock()

	if strings.TrimSpace(name) == "" || fwd == nil {
		return
	}
	fwd.SetDebugLogger(c.composeDebugWriter(name, base, enabled))
}
