package console

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/swm8023/wheelmaker/internal/hub/im"
)

// Channel implements im.Channel for local testing via stdin/stdout.
// It reads stdin line by line and dispatches each line as an im.Message.
// Responses are printed to stdout prefixed with the project name.
type Channel struct {
	projectName string
	debug       bool
	handler     im.MessageHandler
}

// New creates a Channel for the given project.
// If debug is true, the client will enable ACP JSON debug logging.
func New(projectName string, debug bool) *Channel {
	return &Channel{projectName: projectName, debug: debug}
}

// Debug returns whether debug logging is enabled.
func (c *Channel) Debug() bool { return c.debug }

// OnMessage registers the handler called for each received message.
func (c *Channel) OnMessage(handler im.MessageHandler) {
	c.handler = handler
}

// Send prints a text message to stdout; kind is indicated by a prefix for debug/system.
func (c *Channel) Send(_ string, text string, kind im.TextKind) error {
	switch kind {
	case im.TextThought:
		fmt.Printf("[%s][thinking] %s\n", c.projectName, text)
	case im.TextDebug:
		fmt.Printf("[%s][debug] %s\n", c.projectName, strings.TrimSpace(text))
	case im.TextSystem:
		fmt.Printf("[%s][system] %s\n", c.projectName, text)
	default:
		fmt.Printf("[%s] %s\n", c.projectName, text)
	}
	return nil
}

// SendCard prints the card payload to stdout. Dispatches by card type.
func (c *Channel) SendCard(_ string, _ string, card im.Card) error {
	switch cd := card.(type) {
	case im.OptionsCard:
		var b strings.Builder
		if strings.TrimSpace(cd.Title) != "" {
			b.WriteString(strings.TrimSpace(cd.Title))
		}
		if strings.TrimSpace(cd.Body) != "" {
			if b.Len() > 0 {
				b.WriteString("\n")
			}
			b.WriteString(strings.TrimSpace(cd.Body))
		}
		for i, opt := range cd.Options {
			if b.Len() > 0 && i == 0 {
				b.WriteString("\n")
			}
			b.WriteString(fmt.Sprintf("%d. %s (id=%s)\n", i+1, opt.Label, opt.ID))
		}
		fmt.Printf("[%s] %s\n", c.projectName, strings.TrimSpace(b.String()))
	case im.ToolCallCard:
		if msg := im.RenderToolCallMessage(cd.Update); msg != "" {
			fmt.Printf("[%s] %s\n", c.projectName, msg)
		}
	default:
		data, _ := json.Marshal(card)
		fmt.Printf("[%s] card: %s\n", c.projectName, string(data))
	}
	return nil
}

// SendReaction is a no-op for the console IM.
func (c *Channel) SendReaction(_, _ string) error { return nil }

// Run reads lines from os.Stdin until ctx is cancelled or EOF.
// Each non-empty line is dispatched as an im.Message to the registered handler.
func (c *Channel) Run(ctx context.Context) error {
	fmt.Fprintf(os.Stderr, "[%s] ready. Type a message or /status, /use <agent>, /cancel. Ctrl+C to quit.\n", c.projectName)

	lines := make(chan string)
	go func() {
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			lines <- scanner.Text()
		}
		close(lines)
	}()

	for {
		fmt.Fprintf(os.Stderr, "[%s] > ", c.projectName)
		select {
		case <-ctx.Done():
			return nil
		case line, ok := <-lines:
			if !ok {
				return nil // EOF
			}
			if line == "" {
				continue
			}
			if c.handler != nil {
				c.handler(im.Message{
					ChatID:    c.projectName,
					MessageID: fmt.Sprintf("console-%d", time.Now().UnixNano()),
					Text:      line,
					RouteKey:  "console",
				})
			}
		}
	}
}

// OnCardAction is a no-op for the console; it has no interactive card UI.
func (c *Channel) OnCardAction(_ func(im.CardActionEvent)) {}

// MarkDone is a no-op for the console.
func (c *Channel) MarkDone(_ string) error { return nil }

var _ im.Channel = (*Channel)(nil)
