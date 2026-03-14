// Package codex implements an Agent adapter for the Codex CLI via the ACP protocol.
package codex

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"sync/atomic"

	"github.com/swm8023/wheelmaker/internal/acp"
	"github.com/swm8023/wheelmaker/internal/agent"
	"github.com/swm8023/wheelmaker/internal/tools"
)

const agentName = "codex"

// Config holds the configuration for the Codex agent.
type Config struct {
	// ExePath is the path to the codex-acp binary.
	// If empty, tools.ResolveBinary("codex-acp", "") is used.
	ExePath string

	// Env is extra environment variables for the subprocess (e.g. OPENAI_API_KEY).
	Env map[string]string

	// CWD is the working directory passed to ACP session/new.
	// Defaults to the current process working directory.
	CWD string

	// SessionID is a previously saved ACP sessionId to attempt session/load.
	// If empty or if session/load fails, session/new is used.
	SessionID string
}

// Agent implements agent.Agent for the Codex CLI via codex-acp.
type Agent struct {
	cfg    Config
	client *acp.Client

	mu        sync.Mutex
	sessionID string // active ACP session ID
	ready     bool   // true after initialize + session/new or session/load

	// Terminal management (for Agent→Client terminal/* callbacks).
	termsMu     sync.Mutex
	terminals   map[string]*managedTerminal
	termCounter atomic.Int64
}

// New creates a new Codex Agent with the given config.
func New(cfg Config) *Agent {
	return &Agent{
		cfg:       cfg,
		terminals: make(map[string]*managedTerminal),
	}
}

// Name returns the agent's identifier.
func (a *Agent) Name() string { return agentName }

// Prompt sends a prompt to Codex and streams the response as agent.Updates.
func (a *Agent) Prompt(ctx context.Context, text string) (<-chan agent.Update, error) {
	if err := a.ensureSession(ctx); err != nil {
		return nil, fmt.Errorf("codex: %w", err)
	}

	updates := make(chan agent.Update, 32)

	// Subscribe to session/update notifications before sending prompt.
	cancel := a.client.Subscribe(func(n acp.Notification) {
		if n.Method != "session/update" {
			return
		}
		var p acp.SessionUpdateParams
		if err := json.Unmarshal(n.Params, &p); err != nil {
			return
		}
		if p.SessionID != a.sessionID {
			return
		}
		u := a.toUpdate(p.Update)
		select {
		case updates <- u:
		case <-ctx.Done():
		}
	})

	// Send session/prompt asynchronously; close channel when done.
	go func() {
		defer cancel()
		defer close(updates)

		var result acp.SessionPromptResult
		err := a.client.Send(ctx, "session/prompt", acp.SessionPromptParams{
			SessionID: a.sessionID,
			Prompt:    text,
		}, &result)

		if err != nil {
			select {
			case updates <- agent.Update{Type: "error", Err: err, Done: true}:
			case <-ctx.Done():
			}
			return
		}

		select {
		case updates <- agent.Update{Type: "done", Content: result.StopReason, Done: true}:
		case <-ctx.Done():
		}
	}()

	return updates, nil
}

// Cancel sends a session/cancel notification to abort the current prompt.
func (a *Agent) Cancel() error {
	a.mu.Lock()
	sessID := a.sessionID
	a.mu.Unlock()
	if sessID == "" {
		return nil
	}
	return a.client.Notify("session/cancel", acp.SessionCancelParams{SessionID: sessID})
}

// SetMode switches the agent's operating mode.
func (a *Agent) SetMode(modeID string) error {
	a.mu.Lock()
	sessID := a.sessionID
	a.mu.Unlock()
	if sessID == "" {
		return fmt.Errorf("codex: no active session")
	}
	return a.client.Send(context.Background(), "session/set_mode",
		map[string]string{"sessionId": sessID, "modeId": modeID}, nil)
}

// SessionID returns the current ACP session ID (for persistence).
func (a *Agent) SessionID() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.sessionID
}

// Close shuts down the ACP client and the codex-acp subprocess.
func (a *Agent) Close() error {
	a.killAllTerminals()
	if a.client != nil {
		return a.client.Close()
	}
	return nil
}

// ensureSession starts the subprocess and establishes an ACP session if not already done.
func (a *Agent) ensureSession(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.ready {
		return nil
	}

	exePath, err := tools.ResolveBinary("codex-acp", a.cfg.ExePath)
	if err != nil {
		return fmt.Errorf("resolve binary: %w", err)
	}

	env := buildEnv(a.cfg.Env)
	client := acp.New(exePath, env)
	// Register Agent→Client request handler before starting the subprocess.
	// This handles fs/read_text_file, fs/write_text_file, terminal/*, session/request_permission.
	client.OnRequest(a.handleRequest)
	if err := client.Start(); err != nil {
		return fmt.Errorf("start process: %w", err)
	}
	a.client = client

	// Handshake: initialize.
	var initResult acp.InitializeResult
	if err := client.Send(ctx, "initialize", acp.InitializeParams{
		ProtocolVersion: "0.1",
		ClientCapabilities: acp.ClientCapabilities{
			FS: &acp.FSCapabilities{
				ReadTextFile:  true,
				WriteTextFile: true,
			},
			Terminal: true,
		},
		ClientInfo: &acp.AgentInfo{Name: "wheelmaker", Version: "0.1"},
	}, &initResult); err != nil {
		return fmt.Errorf("initialize: %w", err)
	}

	// Attempt session/load if we have a saved sessionId.
	if a.cfg.SessionID != "" && initResult.AgentCapabilities.LoadSession {
		var loadResult acp.SessionLoadResult // session/load does NOT return sessionId
		err := client.Send(ctx, "session/load", acp.SessionLoadParams{
			SessionID:  a.cfg.SessionID,
			CWD:        a.cwd(),
			MCPServers: []acp.MCPServer{},
		}, &loadResult)
		if err == nil {
			a.sessionID = a.cfg.SessionID
			a.ready = true
			return nil
		}
		// session/load failed — fall through to session/new.
	}

	// Create a new session.
	var newResult acp.SessionNewResult
	if err := client.Send(ctx, "session/new", acp.SessionNewParams{
		CWD:        a.cwd(),
		MCPServers: []acp.MCPServer{},
	}, &newResult); err != nil {
		return fmt.Errorf("session/new: %w", err)
	}

	a.sessionID = newResult.SessionID
	a.ready = true
	return nil
}

// toUpdate converts an ACP SessionUpdate to an agent.Update.
func (a *Agent) toUpdate(u acp.SessionUpdate) agent.Update {
	switch u.SessionUpdate {
	case "agent_message_chunk":
		text := ""
		if u.Content != nil && u.Content.Type == "text" {
			text = u.Content.Text
		}
		return agent.Update{Type: "text", Content: text}
	case "agent_thought_chunk":
		text := ""
		if u.Content != nil {
			text = u.Content.Text
		}
		return agent.Update{Type: "thought", Content: text}
	case "tool_call", "tool_call_update":
		return agent.Update{Type: "tool_call", Content: u.SessionUpdate}
	default:
		return agent.Update{Type: u.SessionUpdate}
	}
}

// cwd returns the configured CWD or falls back to the process working directory.
func (a *Agent) cwd() string {
	if a.cfg.CWD != "" {
		return a.cfg.CWD
	}
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return wd
}

// buildEnv converts a map of env vars to "KEY=VALUE" strings.
func buildEnv(m map[string]string) []string {
	env := make([]string, 0, len(m))
	for k, v := range m {
		env = append(env, k+"="+v)
	}
	return env
}
