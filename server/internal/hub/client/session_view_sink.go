package client

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/swm8023/wheelmaker/internal/im"
	acp "github.com/swm8023/wheelmaker/internal/protocol"
)

type SessionViewEventType string

const (
	SessionViewEventSessionCreated      SessionViewEventType = "session_created"
	SessionViewEventUserMessageAccepted SessionViewEventType = "user_message_accepted"
	SessionViewEventAssistantChunk      SessionViewEventType = "assistant_chunk"
	SessionViewEventThoughtChunk        SessionViewEventType = "thought_chunk"
	SessionViewEventToolUpdated         SessionViewEventType = "tool_updated"
	SessionViewEventPermissionRequested SessionViewEventType = "permission_requested"
	SessionViewEventPermissionResolved  SessionViewEventType = "permission_resolved"
	SessionViewEventPromptFinished      SessionViewEventType = "prompt_finished"
	SessionViewEventSystemMessage       SessionViewEventType = "system_message"
)

type SessionViewEvent struct {
	Type          SessionViewEventType
	SessionID     string
	Title         string
	Role          string
	Kind          string
	Text          string
	Blocks        []acp.ContentBlock
	Options       []acp.PermissionOption
	Status        string
	AggregateKey  string
	RequestID     int64
	SourceChannel string
	SourceChatID  string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type SessionViewSink interface {
	RecordEvent(ctx context.Context, event SessionViewEvent) error
}

func (c *Client) SetSessionViewSink(sink SessionViewSink) {
	if sink == nil {
		sink = c
	}
	c.mu.Lock()
	c.viewSink = sink
	for _, sess := range c.sessions {
		sess.viewSink = sink
	}
	c.mu.Unlock()
}

func (c *Client) CreateSession(ctx context.Context, title string) (*Session, error) {
	sess := c.newWiredSession("")
	if strings.TrimSpace(title) != "" {
		sess.mu.Lock()
		state := sess.agentStateLocked(sess.currentAgentNameLocked())
		if state != nil {
			state.Title = strings.TrimSpace(title)
		}
		sess.mu.Unlock()
	}
	if err := sess.persistSession(ctx); err != nil {
		return nil, fmt.Errorf("save session: %w", err)
	}
	c.mu.Lock()
	c.sessions[sess.ID] = sess
	c.mu.Unlock()
	return sess, nil
}

func (c *Client) SessionByID(ctx context.Context, sessionID string) (*Session, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, fmt.Errorf("session id is required")
	}
	c.mu.Lock()
	if sess := c.sessions[sessionID]; sess != nil {
		c.mu.Unlock()
		return sess, nil
	}
	store := c.store
	c.mu.Unlock()

	rec, err := store.LoadSession(ctx, c.projectName, sessionID)
	if err != nil {
		return nil, fmt.Errorf("load session %q: %w", sessionID, err)
	}
	if rec == nil {
		return nil, fmt.Errorf("session %q not found", sessionID)
	}
	restored, err := sessionFromRecord(rec, c.cwd)
	if err != nil {
		return nil, err
	}
	c.wireSession(restored)
	restored.Status = SessionActive
	restored.lastActiveAt = time.Now().UTC()
	c.mu.Lock()
	c.sessions[restored.ID] = restored
	c.mu.Unlock()
	return restored, nil
}

func (c *Client) SendToSession(ctx context.Context, sessionID string, source im.ChatRef, blocks []acp.ContentBlock) error {
	sess, err := c.SessionByID(ctx, sessionID)
	if err != nil {
		return err
	}
	if strings.TrimSpace(source.ChannelID) != "" && strings.TrimSpace(source.ChatID) != "" {
		sess.setIMSource(source)
		if err := c.bindIM(ctx, source, sess.ID); err != nil {
			return err
		}
		if err := c.store.SaveRouteBinding(ctx, c.projectName, imRouteKey(source), sess.ID); err != nil {
			return fmt.Errorf("save route binding: %w", err)
		}
	}
	sess.handlePromptBlocks(blocks)
	return nil
}

func flattenPromptText(blocks []acp.ContentBlock) string {
	parts := make([]string, 0, len(blocks))
	for _, block := range blocks {
		if strings.TrimSpace(block.Type) == acp.ContentBlockTypeText && strings.TrimSpace(block.Text) != "" {
			parts = append(parts, strings.TrimSpace(block.Text))
		}
	}
	return strings.Join(parts, "\n")
}

func PromptPreview(blocks []acp.ContentBlock) string {
	text := flattenPromptText(blocks)
	if text != "" {
		return text
	}
	for _, block := range blocks {
		if strings.TrimSpace(block.Type) == acp.ContentBlockTypeImage {
			return "Sent an image"
		}
	}
	return ""
}

func cloneSessionContentBlocks(blocks []acp.ContentBlock) []acp.ContentBlock {
	if len(blocks) == 0 {
		return nil
	}
	out := make([]acp.ContentBlock, len(blocks))
	copy(out, blocks)
	return out
}

func cloneSessionPermissionOptions(options []acp.PermissionOption) []acp.PermissionOption {
	if len(options) == 0 {
		return nil
	}
	out := make([]acp.PermissionOption, len(options))
	copy(out, options)
	return out
}
