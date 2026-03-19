package console

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/swm8023/wheelmaker/internal/im"
)

// ConsoleIM implements im.Channel for local testing via stdin/stdout.
// It reads stdin line by line and dispatches each line as an im.Message.
// Responses are printed to stdout prefixed with the project name.
type ConsoleIM struct {
	projectName string
	debug       bool
	handler     im.MessageHandler
}

// New creates a ConsoleIM for the given project.
// If debug is true, the client will enable ACP JSON debug logging.
func New(projectName string, debug bool) *ConsoleIM {
	return &ConsoleIM{projectName: projectName, debug: debug}
}

// Debug returns whether debug logging is enabled.
func (c *ConsoleIM) Debug() bool { return c.debug }

// OnMessage registers the handler called for each received message.
func (c *ConsoleIM) OnMessage(handler im.MessageHandler) {
	c.handler = handler
}

// SendText prints a plain text reply to stdout with a project-name prefix.
func (c *ConsoleIM) SendText(_ string, text string) error {
	fmt.Printf("[%s] %s\n", c.projectName, text)
	return nil
}

// SendCard prints the card as JSON to stdout.
func (c *ConsoleIM) SendCard(_ string, card im.Card) error {
	data, _ := json.Marshal(card)
	fmt.Printf("[%s] card: %s\n", c.projectName, string(data))
	return nil
}

// SendOptions renders options as plain text in console.
func (c *ConsoleIM) SendOptions(_ string, title, body string, options []im.DecisionOption, _ map[string]string) error {
	var b strings.Builder
	if strings.TrimSpace(title) != "" {
		b.WriteString(strings.TrimSpace(title))
	}
	if strings.TrimSpace(body) != "" {
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString(strings.TrimSpace(body))
	}
	if len(options) > 0 {
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		for i, opt := range options {
			b.WriteString(fmt.Sprintf("%d. %s (id=%s)\n", i+1, opt.Label, opt.ID))
		}
	}
	return c.SendText(c.projectName, strings.TrimSpace(b.String()))
}

// SendReaction is a no-op for the console IM.
func (c *ConsoleIM) SendReaction(_, _ string) error { return nil }

// Run reads lines from os.Stdin until ctx is cancelled or EOF.
// Each non-empty line is dispatched as an im.Message to the registered handler.
func (c *ConsoleIM) Run(ctx context.Context) error {
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
				})
			}
		}
	}
}

var _ im.Channel = (*ConsoleIM)(nil)
var _ im.OptionSender = (*ConsoleIM)(nil)
