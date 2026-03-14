package client

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/swm8023/wheelmaker/internal/adapter"
	"github.com/swm8023/wheelmaker/internal/agent"
	"github.com/swm8023/wheelmaker/internal/im"
)

// Client is the top-level coordinator for WheelMaker.
// It holds a pool of stateless Adapter factories and two references to the active Agent:
//   - session agent.Session  — narrow interface for Prompt/Cancel/SetMode, mockable in tests.
//   - ag      *agent.Agent   — concrete type for Switch (to avoid type assertion on mock).
//
// Switching adapters is done via c.ag.Switch() (never via c.session), so
// injecting a mock Session in tests does not break the Switch code path.
type Client struct {
	adapters map[string]adapter.Adapter
	session  agent.Session // narrow interface, can be mock in tests
	ag       *agent.Agent  // concrete type, used for Switch only; nil when mock injected

	store Store
	state *State
	imRun im.Adapter // nil in CLI/test mode

	mu              sync.Mutex
	currentPromptCh <-chan agent.Update // tracked for draining during switchAdapter
}

// New creates a Client with the given store and optional IM adapter.
// imAdapter may be nil; in that case Run() drives the stdin loop.
func New(store Store, imAdapter im.Adapter) *Client {
	return &Client{
		adapters: make(map[string]adapter.Adapter),
		store:    store,
		imRun:    imAdapter,
	}
}

// RegisterAdapter registers an adapter factory under its Name().
func (c *Client) RegisterAdapter(a adapter.Adapter) {
	c.mu.Lock()
	c.adapters[a.Name()] = a
	c.mu.Unlock()
}

// Start loads persisted state and eagerly connects the active adapter.
// After Start returns, the subprocess is running and the first Prompt will be fast.
func (c *Client) Start(ctx context.Context) error {
	state, err := c.store.Load()
	if err != nil {
		return fmt.Errorf("client: load state: %w", err)
	}
	c.mu.Lock()
	c.state = state
	c.mu.Unlock()

	// Determine the active adapter name.
	name := state.ActiveAdapter
	if name == "" {
		name = "codex"
	}

	c.mu.Lock()
	adpt := c.adapters[name]
	savedSessionID := state.SessionIDs[name]
	c.mu.Unlock()

	if adpt == nil {
		return fmt.Errorf("client: no adapter registered for %q", name)
	}

	// Start the subprocess eagerly.
	conn, err := adpt.Connect(ctx)
	if err != nil {
		return fmt.Errorf("client: connect %q: %w", name, err)
	}

	cwd, _ := os.Getwd()
	var ag *agent.Agent
	if savedSessionID != "" {
		ag = agent.NewWithSessionID(name, conn, cwd, savedSessionID)
	} else {
		ag = agent.New(name, conn, cwd)
	}

	c.mu.Lock()
	c.ag = ag
	c.session = ag
	c.mu.Unlock()

	if c.imRun != nil {
		c.imRun.OnMessage(c.HandleMessage)
	}
	return nil
}

// Run blocks until ctx is cancelled.
// With an IM adapter it delegates to im.Adapter.Run; otherwise it drives the stdin loop.
func (c *Client) Run(ctx context.Context) error {
	if c.imRun != nil {
		return c.imRun.Run(ctx)
	}
	// CLI mode: read messages from stdin.
	fmt.Fprintln(os.Stderr, "WheelMaker ready. Type a message or /status, /use <adapter>, /cancel. Ctrl+C to quit.")
	scanner := bufio.NewScanner(os.Stdin)
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		fmt.Fprint(os.Stderr, "> ")
		if !scanner.Scan() {
			return nil
		}
		text := scanner.Text()
		if text == "" {
			continue
		}
		c.HandleMessage(im.Message{
			ChatID:    "cli",
			MessageID: "cli-msg",
			UserID:    "local",
			Text:      text,
		})
	}
}

// HandleMessage routes an incoming IM (or CLI) message to the appropriate handler.
func (c *Client) HandleMessage(msg im.Message) {
	text := strings.TrimSpace(msg.Text)
	if text == "" {
		return
	}
	if strings.HasPrefix(text, "/") {
		c.handleCommand(msg, text)
		return
	}
	c.handlePrompt(msg, text)
}

// Close saves state and shuts down the active agent.
func (c *Client) Close() error {
	c.mu.Lock()
	ag := c.ag
	state := c.state
	c.mu.Unlock()

	if ag != nil {
		// Persist the final session ID before closing.
		if sid := ag.SessionID(); sid != "" && state != nil {
			c.mu.Lock()
			if state.SessionIDs == nil {
				state.SessionIDs = map[string]string{}
			}
			state.SessionIDs[ag.AdapterName()] = sid
			c.mu.Unlock()
		}
		_ = ag.Close()
	}

	c.mu.Lock()
	s := c.state
	c.mu.Unlock()
	if s != nil {
		return c.store.Save(s)
	}
	return nil
}

// --- internal ---

// handleCommand processes "/" prefixed commands.
func (c *Client) handleCommand(msg im.Message, text string) {
	parts := strings.Fields(text)
	cmd := strings.ToLower(parts[0])

	switch cmd {
	case "/use":
		if len(parts) < 2 {
			c.reply(msg.ChatID, "Usage: /use <adapter-name> [--continue]  (e.g. /use codex)")
			return
		}
		name := strings.ToLower(parts[1])
		mode := agent.SwitchClean
		for _, p := range parts[2:] {
			if p == "--continue" {
				mode = agent.SwitchWithContext
			}
		}
		if err := c.switchAdapter(context.Background(), msg.ChatID, name, mode); err != nil {
			c.reply(msg.ChatID, fmt.Sprintf("Switch error: %v", err))
		}

	case "/cancel":
		c.mu.Lock()
		sess := c.session
		c.mu.Unlock()
		if sess == nil {
			c.reply(msg.ChatID, "No active session.")
			return
		}
		if err := sess.Cancel(); err != nil {
			c.reply(msg.ChatID, fmt.Sprintf("Cancel error: %v", err))
			return
		}
		c.reply(msg.ChatID, "Cancelled.")

	case "/status":
		c.mu.Lock()
		sess := c.session
		c.mu.Unlock()
		if sess == nil {
			c.reply(msg.ChatID, "No active session.")
			return
		}
		status := fmt.Sprintf("Active adapter: %s", sess.AdapterName())
		if sid := sess.SessionID(); sid != "" {
			status += fmt.Sprintf("\nACP session: %s", sid)
		}
		c.reply(msg.ChatID, status)

	default:
		c.reply(msg.ChatID, fmt.Sprintf("Unknown command: %s\nAvailable: /use <adapter>, /cancel, /status", cmd))
	}
}

// handlePrompt sends text to the active session and streams the response.
func (c *Client) handlePrompt(msg im.Message, text string) {
	c.mu.Lock()
	sess := c.session
	c.mu.Unlock()
	if sess == nil {
		c.reply(msg.ChatID, "No active session. Use /use <adapter> to start.")
		return
	}

	ctx := context.Background()
	updates, err := sess.Prompt(ctx, text)
	if err != nil {
		c.reply(msg.ChatID, fmt.Sprintf("Prompt error: %v", err))
		return
	}

	c.mu.Lock()
	c.currentPromptCh = updates
	c.mu.Unlock()

	var buf strings.Builder
	for u := range updates {
		if u.Err != nil {
			c.reply(msg.ChatID, fmt.Sprintf("Agent error: %v", u.Err))
			c.mu.Lock()
			c.currentPromptCh = nil
			c.mu.Unlock()
			return
		}
		if u.Type == agent.UpdateText {
			buf.WriteString(u.Content)
		}
		if u.Done {
			break
		}
	}

	c.mu.Lock()
	c.currentPromptCh = nil
	c.mu.Unlock()

	if buf.Len() > 0 {
		c.reply(msg.ChatID, buf.String())
	}
}

// switchAdapter cancels any in-progress prompt, connects a new binary,
// and calls ag.Switch() to replace the connection.
// Always uses c.ag (concrete type) for Switch, never c.session (interface),
// to avoid type assertion panics when a mock is injected in tests.
func (c *Client) switchAdapter(ctx context.Context, chatID, name string, mode agent.SwitchMode) error {
	c.mu.Lock()
	adpt := c.adapters[name]
	sess := c.session
	ag := c.ag
	promptCh := c.currentPromptCh
	c.mu.Unlock()

	if adpt == nil {
		return fmt.Errorf("unknown adapter: %q (registered: %v)", name, c.registeredAdapterNames())
	}

	// Step 1: cancel and drain any in-progress prompt.
	if sess != nil {
		_ = sess.Cancel()
	}
	if promptCh != nil {
		for range promptCh {
		}
		c.mu.Lock()
		c.currentPromptCh = nil
		c.mu.Unlock()
	}

	// Step 2: connect the new binary.
	newConn, err := adpt.Connect(ctx)
	if err != nil {
		return fmt.Errorf("connect %q: %w", name, err)
	}

	// Step 3: replace the connection via the concrete Agent type.
	if ag != nil {
		if err := ag.Switch(ctx, name, newConn, mode); err != nil {
			return fmt.Errorf("switch %q: %w", name, err)
		}
	}

	// Persist the new active adapter name.
	c.mu.Lock()
	if c.state != nil {
		c.state.ActiveAdapter = name
	}
	c.mu.Unlock()
	_ = c.store.Save(c.state)

	c.reply(chatID, fmt.Sprintf("Switched to adapter: %s", name))
	return nil
}

// registeredAdapterNames returns all registered adapter names (for error messages).
func (c *Client) registeredAdapterNames() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	names := make([]string, 0, len(c.adapters))
	for n := range c.adapters {
		names = append(names, n)
	}
	return names
}

// reply sends a text response to the chat, or prints to stdout in CLI mode.
func (c *Client) reply(chatID, text string) {
	if c.imRun != nil {
		_ = c.imRun.SendText(chatID, text)
		return
	}
	fmt.Println(text)
}
