package client

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type sessionHistoryTurn struct {
	TurnIndex int64  `json:"turnIndex"`
	Method    string `json:"method"`
	Finished  bool   `json:"finished"`
	Content   string `json:"content"`
}

type sessionHistoryPrompt struct {
	SchemaVersion int64                `json:"schemaVersion"`
	SessionID     string               `json:"sessionId"`
	PromptIndex   int64                `json:"promptIndex"`
	Title         string               `json:"title,omitempty"`
	ModelName     string               `json:"modelName,omitempty"`
	StartedAt     time.Time            `json:"startedAt,omitempty"`
	UpdatedAt     time.Time            `json:"updatedAt,omitempty"`
	StopReason    string               `json:"stopReason,omitempty"`
	TurnIndex     int64                `json:"turnIndex"`
	Turns         []sessionHistoryTurn `json:"turns"`
}

type fileSessionHistoryStore struct {
	root string
}

func newFileSessionHistoryStore(root string) *fileSessionHistoryStore {
	return &fileSessionHistoryStore{root: root}
}

func (s *fileSessionHistoryStore) WritePrompt(ctx context.Context, projectName string, prompt sessionHistoryPrompt) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s == nil {
		return fmt.Errorf("session history store is required")
	}
	prompt.SessionID = strings.TrimSpace(prompt.SessionID)
	if prompt.SessionID == "" {
		return fmt.Errorf("session id is required")
	}
	if prompt.PromptIndex <= 0 {
		return fmt.Errorf("prompt index is required")
	}
	prompt.SchemaVersion = 1
	prompt.TurnIndex = int64(len(prompt.Turns))
	dir := filepath.Join(s.root, safeHistoryPathPart(projectName), safeHistoryPathPart(prompt.SessionID), "prompts")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir prompt history: %w", err)
	}
	path := filepath.Join(dir, fmt.Sprintf("p%06d.json", prompt.PromptIndex))
	tmp := path + ".tmp"
	raw, err := json.MarshalIndent(prompt, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal prompt history: %w", err)
	}
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return fmt.Errorf("write prompt history temp: %w", err)
	}
	if err := replaceFile(tmp, path); err != nil {
		return fmt.Errorf("replace prompt history: %w", err)
	}
	return nil
}

func (s *fileSessionHistoryStore) ReadPrompt(ctx context.Context, projectName, sessionID string, promptIndex int64) (sessionHistoryPrompt, error) {
	if err := ctx.Err(); err != nil {
		return sessionHistoryPrompt{}, err
	}
	if s == nil {
		return sessionHistoryPrompt{}, fmt.Errorf("session history store is required")
	}
	path := filepath.Join(s.root, safeHistoryPathPart(projectName), safeHistoryPathPart(sessionID), "prompts", fmt.Sprintf("p%06d.json", promptIndex))
	raw, err := os.ReadFile(path)
	if err != nil {
		return sessionHistoryPrompt{}, fmt.Errorf("read prompt history: %w", err)
	}
	var prompt sessionHistoryPrompt
	if err := json.Unmarshal(raw, &prompt); err != nil {
		return sessionHistoryPrompt{}, fmt.Errorf("decode prompt history: %w", err)
	}
	return prompt, nil
}

func replaceFile(tmp, path string) error {
	if err := os.Rename(tmp, path); err == nil {
		return nil
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.Rename(tmp, path)
}

func safeHistoryPathPart(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "_"
	}
	replacer := strings.NewReplacer(
		"\\", "_",
		"/", "_",
		":", "_",
		"*", "_",
		"?", "_",
		`"`, "_",
		"<", "_",
		">", "_",
		"|", "_",
	)
	return replacer.Replace(value)
}
