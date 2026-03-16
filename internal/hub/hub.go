package hub

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"sync"

	"github.com/swm8023/wheelmaker/internal/agent/provider"
	"github.com/swm8023/wheelmaker/internal/agent/provider/claude"
	"github.com/swm8023/wheelmaker/internal/agent/provider/codex"
	"github.com/swm8023/wheelmaker/internal/agent/provider/mock"
	"github.com/swm8023/wheelmaker/internal/client"
	"github.com/swm8023/wheelmaker/internal/im"
	"github.com/swm8023/wheelmaker/internal/im/console"
)

// Hub orchestrates one or more WheelMaker project clients.
// Each project has its own IM adapter, agent session, and state partition.
type Hub struct {
	cfg       *Config
	statePath string
	clients   []*client.Client
}

// New creates a Hub from the given config and state file path.
// hub.Start() must be called before hub.Run().
func New(cfg *Config, statePath string) *Hub {
	return &Hub{cfg: cfg, statePath: statePath}
}

// Start validates config, creates one client.Client per project, and starts each client.
// Returns an error if more than one project uses type "console", or if any project
// has an unsupported IM type.
func (h *Hub) Start(ctx context.Context) error {
	// Validate: at most one console IM project.
	consoleCount := 0
	for _, pc := range h.cfg.Projects {
		if pc.IM.Type == "console" {
			consoleCount++
		}
	}
	if consoleCount > 1 {
		return errors.New("hub: at most one project may use im.type = \"console\"")
	}

	for _, pc := range h.cfg.Projects {
		c, err := h.buildClient(ctx, pc)
		if err != nil {
			return fmt.Errorf("hub: project %q: %w", pc.Name, err)
		}
		h.clients = append(h.clients, c)
	}
	return nil
}

// buildClient creates, configures, and starts a client.Client for one project.
func (h *Hub) buildClient(ctx context.Context, pc ProjectConfig) (*client.Client, error) {
	// Resolve working directory.
	cwd := pc.Client.Path
	if cwd == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			cwd = "."
		}
	}

	// Create IM provider.
	imAdapter, err := h.buildIM(pc)
	if err != nil {
		return nil, err
	}

	// Create per-project state store.
	store := client.NewProjectJSONStore(h.statePath, pc.Name)

	// Create the client.
	c := client.New(store, imAdapter, pc.Name, cwd)

	// Enable ACP JSON debug logging for console projects with debug=true.
	if pc.IM.Type == "console" && pc.IM.Debug {
		c.SetDebugLogger(log.Writer())
	}

	// Register all known adapter factories so users can switch between them at runtime.
	c.RegisterProvider("codex", func(_ string, _ map[string]string) provider.Provider {
		return codex.NewAdapter(codex.Config{})
	})
	c.RegisterProvider("claude", func(_ string, _ map[string]string) provider.Provider {
		return claude.NewAdapter(claude.Config{})
	})
	c.RegisterProvider("mock", func(_ string, _ map[string]string) provider.Provider {
		return mock.NewAdapter()
	})

	if err := c.Start(ctx); err != nil {
		return nil, fmt.Errorf("start: %w", err)
	}
	return c, nil
}

// buildIM creates the im.Adapter for a project's IM config.
func (h *Hub) buildIM(pc ProjectConfig) (im.Adapter, error) {
	switch pc.IM.Type {
	case "console":
		return console.New(pc.Name, pc.IM.Debug), nil
	case "feishu":
		return nil, fmt.Errorf("feishu IM adapter not yet implemented")
	default:
		return nil, fmt.Errorf("unknown im.type %q (supported: console)", pc.IM.Type)
	}
}

// Run starts each project client in a goroutine and blocks until ctx is done
// or all goroutines have exited. Individual project errors are logged to stderr;
// only ctx cancellation terminates the run.
func (h *Hub) Run(ctx context.Context) error {
	var wg sync.WaitGroup
	for _, c := range h.clients {
		wg.Add(1)
		go func(c *client.Client) {
			defer wg.Done()
			if err := c.Run(ctx); err != nil && ctx.Err() == nil {
				log.Printf("wheelmaker: project run error: %v", err)
			}
		}(c)
	}
	wg.Wait()
	return nil
}

// Close calls Close() on all project clients, collecting any errors.
func (h *Hub) Close() error {
	var errs []error
	for _, c := range h.clients {
		if err := c.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("hub close errors: %v", errs)
	}
	return nil
}
