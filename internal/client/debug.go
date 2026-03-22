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
		w.sink.client.replyDebug(chatID, fmt.Sprintf("[debug][%s] %s", w.agentName, line))
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
	return defaultAgentName
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
	if strings.TrimSpace(args) != "" {
		return fmt.Errorf("usage: /debug")
	}
	c.reply(chatID, c.renderDebugStatus())
	return nil
}

func (c *Client) renderDebugStatus() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.debugEnabled {
		return "Debug status: on (project)"
	}
	return "Debug status: off (project)"
}
