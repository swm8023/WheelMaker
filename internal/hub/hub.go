package hub

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/swm8023/wheelmaker/internal/agent"
	"github.com/swm8023/wheelmaker/internal/agent/codex"
	"github.com/swm8023/wheelmaker/internal/im"
)

// Hub is the central dispatcher that connects an IM adapter to one or more agents.
// It manages agent lifecycle, command parsing, and state persistence.
type Hub struct {
	store Store
	im    im.Adapter // may be nil in CLI/test mode

	mu     sync.Mutex
	state  *State
	agents map[string]agent.Agent // name → live agent instance
}

// New creates a Hub with the given store and IM adapter.
// im may be nil; in that case, Hub can still be used programmatically via HandleMessage.
func New(store Store, adapter im.Adapter) *Hub {
	return &Hub{
		store:  store,
		im:     adapter,
		agents: make(map[string]agent.Agent),
	}
}

// Start loads persisted state and registers the IM message handler.
// It does not block; call im.Adapter.Run separately to start the event loop.
func (h *Hub) Start(_ context.Context) error {
	state, err := h.store.Load()
	if err != nil {
		return fmt.Errorf("hub: load state: %w", err)
	}
	h.mu.Lock()
	h.state = state
	h.mu.Unlock()

	if h.im != nil {
		h.im.OnMessage(h.HandleMessage)
	}
	return nil
}

// HandleMessage routes an incoming IM message to the appropriate handler.
// Special commands (prefixed with "/") are handled directly; other text is sent to the active agent.
func (h *Hub) HandleMessage(msg im.Message) {
	text := strings.TrimSpace(msg.Text)
	if text == "" {
		return
	}

	if strings.HasPrefix(text, "/") {
		h.handleCommand(msg, text)
		return
	}

	h.handlePrompt(msg, text)
}

// Close saves state and shuts down all running agents.
func (h *Hub) Close() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Persist any live session IDs.
	for name, a := range h.agents {
		if ca, ok := a.(*codex.Agent); ok {
			if sid := ca.SessionID(); sid != "" {
				h.state.ACPSessionIDs[name] = sid
			}
		}
		_ = a.Close()
	}
	h.agents = make(map[string]agent.Agent)

	return h.store.Save(h.state)
}

// --- internal ---

// handleCommand processes a "/" prefixed command.
func (h *Hub) handleCommand(msg im.Message, text string) {
	parts := strings.Fields(text)
	cmd := strings.ToLower(parts[0])

	switch cmd {
	case "/use":
		if len(parts) < 2 {
			h.reply(msg.ChatID, "Usage: /use <agent-name>  (e.g. /use codex)")
			return
		}
		name := strings.ToLower(parts[1])
		h.mu.Lock()
		h.state.ActiveAgent = name
		h.mu.Unlock()
		_ = h.store.Save(h.state)
		h.reply(msg.ChatID, fmt.Sprintf("Switched to agent: %s", name))

	case "/cancel":
		h.mu.Lock()
		a := h.agents[h.state.ActiveAgent]
		h.mu.Unlock()
		if a == nil {
			h.reply(msg.ChatID, "No active agent session.")
			return
		}
		if err := a.Cancel(); err != nil {
			h.reply(msg.ChatID, fmt.Sprintf("Cancel error: %v", err))
			return
		}
		h.reply(msg.ChatID, "Cancelled.")

	case "/status":
		h.mu.Lock()
		active := h.state.ActiveAgent
		sid := h.state.ACPSessionIDs[active]
		h.mu.Unlock()
		status := fmt.Sprintf("Active agent: %s", active)
		if sid != "" {
			status += fmt.Sprintf("\nACP session: %s", sid)
		}
		h.reply(msg.ChatID, status)

	default:
		h.reply(msg.ChatID, fmt.Sprintf("Unknown command: %s\nAvailable: /use <agent>, /cancel, /status", cmd))
	}
}

// handlePrompt sends the text to the active agent and streams the response back.
func (h *Hub) handlePrompt(msg im.Message, text string) {
	a, err := h.getOrCreateAgent()
	if err != nil {
		h.reply(msg.ChatID, fmt.Sprintf("Error starting agent: %v", err))
		return
	}

	ctx := context.Background()
	updates, err := a.Prompt(ctx, text)
	if err != nil {
		h.reply(msg.ChatID, fmt.Sprintf("Prompt error: %v", err))
		return
	}

	// Stream response — accumulate text chunks and send as one message.
	// TODO(phase2): send incremental updates for better UX.
	var buf strings.Builder
	for u := range updates {
		if u.Err != nil {
			h.reply(msg.ChatID, fmt.Sprintf("Agent error: %v", u.Err))
			return
		}
		if u.Type == "text" {
			buf.WriteString(u.Content)
		}
		if u.Done {
			break
		}
	}

	if buf.Len() > 0 {
		h.reply(msg.ChatID, buf.String())
	}
}

// getOrCreateAgent returns the active agent, creating it if necessary.
func (h *Hub) getOrCreateAgent() (agent.Agent, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	name := h.state.ActiveAgent
	if name == "" {
		name = "codex"
	}

	if a, ok := h.agents[name]; ok {
		return a, nil
	}

	a, err := h.createAgent(name)
	if err != nil {
		return nil, err
	}
	h.agents[name] = a
	return a, nil
}

// createAgent instantiates an agent by name using the persisted config.
func (h *Hub) createAgent(name string) (agent.Agent, error) {
	cfg := h.state.Agents[name] // zero value if not configured
	sessionID := h.state.ACPSessionIDs[name]

	switch name {
	case "codex":
		return codex.New(codex.Config{
			ExePath:   cfg.ExePath,
			Env:       cfg.Env,
			SessionID: sessionID,
		}), nil
	default:
		return nil, fmt.Errorf("unknown agent: %q (supported: codex)", name)
	}
}

// reply sends a text response back to the IM chat.
// If no IM adapter is configured, the response is printed to stdout.
func (h *Hub) reply(chatID, text string) {
	if h.im != nil {
		_ = h.im.SendText(chatID, text)
		return
	}
	// CLI mode: print to stdout.
	fmt.Println(text)
}
