package client

import (
	"fmt"
	"io"
	"strings"
)

// agentDebugSink routes ACP JSON debug output to the IM debug channel.
// It no longer tracks per-agent chatIDs; routing uses imBridge.activeChatID.
type agentDebugSink struct {
	client *Client
}

func newAgentDebugSink(c *Client) *agentDebugSink {
	return &agentDebugSink{client: c}
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
		w.sink.client.replyDebug(fmt.Sprintf("[debug][%s] %s", w.agentName, line))
	}
	return len(p), nil
}

func (c *Client) resolveCurrentAgentName() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn != nil && strings.TrimSpace(c.conn.name) != "" {
		return c.conn.name
	}
	if c.state != nil && strings.TrimSpace(c.state.ActiveAgent) != "" {
		return c.state.ActiveAgent
	}
	return defaultAgentName
}

func (c *Client) composeDebugWriter(agentName string, base io.Writer) io.Writer {
	var ws []io.Writer
	if base != nil {
		ws = append(ws, base)
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

func (c *Client) handleDebugCommand(args string) error {
	if strings.TrimSpace(args) != "" {
		return fmt.Errorf("usage: /debug")
	}
	c.reply(c.renderDebugStatus())
	return nil
}

func (c *Client) renderDebugStatus() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.debugLog != nil {
		return "Debug status: on (project)"
	}
	return "Debug status: off (project)"
}
